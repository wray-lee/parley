import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';

import { createSession } from '../core/session.js';
import { createRegistry } from '../core/packs-registry.js';
import { makeClock, pcmFrame, scriptedAsr, scriptedTranslator, scriptedTts } from './helpers/fakes.js';
import { checkTrace, createTraceRecorder } from './helpers/trace.js';

// The Forced Translation instruction is a prompt asset, never a code string.
const FORCED_INSTRUCTION = readFileSync(
  new URL('../prompts/forced_translation.txt', import.meta.url),
  'utf8',
).trim();

function registry() {
  return createRegistry({
    promptFiles: [
      { name: 'system_zh2ja.txt', text: 'SYSTEM {{GLOSSARY}}' },
      { name: 'system_ja2zh.txt', text: 'SYSTEM {{GLOSSARY}}' },
    ],
    packFiles: ['conference_zh2ja.tsv', 'conference_ja2zh.tsv'],
  });
}

function build(overrides = {}) {
  const clock = overrides.clock ?? makeClock();
  const asr = overrides.asr ?? scriptedAsr([
    { at: 20, events: [{ type: 'partial', text: 'one' }, { type: 'stable', text: 'one' }] },
  ]);
  const translator = overrides.translator ?? scriptedTranslator(['translated-one']);
  const tts = overrides.tts ?? scriptedTts();
  const registryValue = overrides.registry ?? registry();
  const pairConfig = overrides.pairConfig ?? {};
  const forcedInstruction = overrides.forcedInstruction ?? FORCED_INSTRUCTION;
  const trace = overrides.trace
    ? createTraceRecorder({
        name: overrides.trace,
        clock,
        asr,
        translator,
        tts,
        registry: registryValue,
        pairConfig,
        forcedInstruction,
      })
    : null;
  const events = [];
  const records = [];
  const session = createSession({
    providers: { asr, translator, tts },
    clock,
    registry: registryValue,
    pairConfig,
    forcedInstruction,
    emit: (event) => {
      events.push(event);
      trace?.output('event', { event });
    },
    log: (record) => {
      records.push(record);
      trace?.output('log', { record });
    },
  });
  trace?.attachSession(session);
  return { session, clock, asr, translator, tts, events, records, trace };
}

function start(session, src = 'zh', tgt = 'ja', mode = 'listen') {
  session.handleMessage({ type: 'start', mode, src, tgt });
  session.handleMessage({ type: 'sample', name: 'placeholder' });
}

function feedSpeech(session, clock, ms) {
  for (let elapsed = 0; elapsed < ms; elapsed += 20) {
    session.handleAudio(pcmFrame(20, 0.2));
    clock.advance(20);
  }
}

function feedSilence(session, clock, ms) {
  for (let elapsed = 0; elapsed < ms; elapsed += 20) {
    session.handleAudio(pcmFrame(20, 0));
    clock.advance(20);
  }
}

async function speechThenSilence(session, clock, silenceFrames = 18) {
  session.handleAudio(pcmFrame(20, 0.2));
  clock.advance(20);
  for (let i = 0; i < silenceFrames; i++) {
    session.handleAudio(pcmFrame(20, 0));
    clock.advance(20);
  }
  await session.idle();
}

test('hello carries the demo flag and the discovered pairs', () => {
  const { session } = build();
  const hello = session.hello();
  assert.equal(hello.type, 'hello');
  assert.equal(hello.demo, false);
  assert.deepEqual(hello.pairs, [
    {
      a: 'ja',
      b: 'zh',
      directions: [{ src: 'zh', tgt: 'ja' }, { src: 'ja', tgt: 'zh' }],
      converse: true,
    },
  ]);
  assert.deepEqual(hello.packs, [
    { name: 'conference', src: 'zh', tgt: 'ja' },
    { name: 'conference', src: 'ja', tgt: 'zh' },
  ]);
});

test('any mock Provider puts the Session in Demo Mode', () => {
  const fake = scriptedAsr([], { name: 'mock' });
  const { session } = build({ asr: fake });
  assert.equal(session.hello().demo, true);
});

test('350 ms of silence cuts a Segment, which commits and speaks in order', async () => {
  const { session, clock, events } = build();
  start(session);
  await speechThenSilence(session, clock, 17);
  assert.equal(events.filter((e) => e.type === 'segment').length, 0, '349 ms of silence must not cut');

  session.handleAudio(pcmFrame(20, 0));
  clock.advance(20);
  await session.idle();

  const types = events.map((e) => e.type);
  assert.ok(types.includes('asr_partial'), 'ASR partial was emitted');
  const segment = events.find((e) => e.type === 'segment');
  assert.equal(segment.seg_id, 1);
  assert.equal(segment.src, 'one');
  assert.equal(segment.tgt, 'translated-one');
  assert.deepEqual(segment.direction, { src: 'zh', tgt: 'ja' });
  assert.equal(segment.speak, true);
  assert.equal(typeof segment.latency_ms, 'number');
  assert.deepEqual(types.slice(types.indexOf('segment')), ['segment', 'tts_begin', 'tts_chunk', 'tts_end']);
});

test('a silence Cut waits for a late final inside 600 ms and keeps the whole clause', async () => {
  const asr = scriptedAsr([
    { at: 20, events: [{ type: 'partial', text: '127.5' }] },
    { at: 440, events: [{ type: 'stable', text: '127.5' }] },
  ]);
  const { session, clock, events } = build({ asr });
  start(session);
  feedSpeech(session, clock, 20);
  feedSilence(session, clock, 360); // the 350 ms trigger fires on the last frame
  await session.idle();
  assert.equal(
    events.filter((e) => e.type === 'segment').length,
    0,
    'must wait for the Stable Prefix to catch up',
  );

  feedSilence(session, clock, 60); // the late final arrives during the wait
  await session.idle();
  const segments = events.filter((e) => e.type === 'segment');
  assert.equal(segments.length, 1);
  assert.equal(segments[0].src, '127.5');
  assert.ok(segments[0].latency_ms < 950, 'cut as soon as the final arrived, not at the deadline');
});

test('when the final never comes, the silence Cut fires at 350 + 600 ms with stable text only', async () => {
  const asr = scriptedAsr([
    { at: 20, events: [{ type: 'partial', text: 'hello world' }] },
    { at: 40, events: [{ type: 'stable', text: 'hello' }] },
  ]);
  const { session, clock, events } = build({ asr });
  start(session);
  feedSpeech(session, clock, 40);
  feedSilence(session, clock, 360); // wait starts at clock 1380
  await session.idle();
  assert.equal(events.filter((e) => e.type === 'segment').length, 0, 'waiting for the final');

  feedSilence(session, clock, 560); // clock 1960: still inside the 600 ms wait
  await session.idle();
  assert.equal(events.filter((e) => e.type === 'segment').length, 0, '600 ms not elapsed yet');

  feedSilence(session, clock, 20); // clock 1980: the wait expires
  await session.idle();
  const segments = events.filter((e) => e.type === 'segment');
  assert.equal(segments.length, 1);
  assert.equal(segments[0].src, 'hello', 'never Partial text');
  assert.equal(segments[0].latency_ms, 960);
});

test('speech resuming during the wait cancels the silence Cut', async () => {
  const asr = scriptedAsr([
    { at: 20, events: [{ type: 'partial', text: 'hello world' }] },
    { at: 40, events: [{ type: 'stable', text: 'hello' }] },
  ]);
  const { session, clock, events } = build({ asr });
  start(session);
  feedSpeech(session, clock, 40);
  feedSilence(session, clock, 360); // wait starts at clock 1380
  feedSpeech(session, clock, 20); // speech resumes inside the wait
  await session.idle();
  assert.equal(events.filter((e) => e.type === 'segment').length, 0, 'no Cut while speech resumes');

  feedSilence(session, clock, 560); // clock 1980, past the cancelled deadline
  await session.idle();
  assert.equal(
    events.filter((e) => e.type === 'segment').length,
    0,
    'the cancelled wait must not fire',
  );
});

test('closing the Session cancels a pending silence Cut', async () => {
  const asr = scriptedAsr([
    { at: 20, events: [{ type: 'partial', text: 'hello world' }] },
    { at: 40, events: [{ type: 'stable', text: 'hello' }] },
  ]);
  const { session, clock, events } = build({ asr });
  start(session);
  feedSpeech(session, clock, 40);
  feedSilence(session, clock, 360);
  session.close();
  clock.advance(600);
  await session.idle();
  assert.equal(events.filter((e) => e.type === 'segment').length, 0);
});

test('no pending Partial cuts on silence at 360 ms as before', async () => {
  const asr = scriptedAsr([
    { at: 20, events: [{ type: 'partial', text: 'one' }, { type: 'stable', text: 'one' }] },
  ]);
  const { session, clock, events } = build({ asr });
  start(session);
  feedSpeech(session, clock, 20);
  feedSilence(session, clock, 340);
  await session.idle();
  assert.equal(events.filter((e) => e.type === 'segment').length, 0, '340 ms must not cut');
  feedSilence(session, clock, 20);
  await session.idle();
  const segments = events.filter((e) => e.type === 'segment');
  assert.equal(segments.length, 1, 'the 350 ms trigger cuts immediately');
  assert.equal(segments[0].src, 'one');
});

test('seg_id is monotonic across Cuts and Direction', async () => {
  const asr = scriptedAsr([
    { at: 20, events: [{ type: 'stable', text: 'one' }] },
    { at: 400, events: [{ type: 'stable', text: 'two' }] },
  ]);
  const translator = scriptedTranslator(['translated-one', 'translated-two']);
  const { session, clock, events } = build({ asr, translator });
  start(session);

  await speechThenSilence(session, clock, 18);
  await speechThenSilence(session, clock, 18);

  const segments = events.filter((e) => e.type === 'segment');
  assert.deepEqual(segments.map((s) => s.seg_id), [1, 2]);
  assert.deepEqual(segments.map((s) => s.src), ['one', 'two']);
});

test('translator output is trimmed before Commit, Rolling Context and TTS', async () => {
  const asr = scriptedAsr([
    { at: 20, events: [{ type: 'stable', text: 'one' }] },
    { at: 400, events: [{ type: 'stable', text: 'two' }] },
  ]);
  const translator = scriptedTranslator(['\n  translated-one \n', ' translated-two\n']);
  const tts = scriptedTts();
  const { session, clock, events } = build({ asr, translator, tts });
  start(session);

  await speechThenSilence(session, clock, 18);
  await speechThenSilence(session, clock, 18);

  assert.deepEqual(
    events.filter((e) => e.type === 'segment').map((e) => e.tgt),
    ['translated-one', 'translated-two'],
  );
  assert.deepEqual(tts.calls, ['translated-one', 'translated-two']);
  assert.match(translator.requests[1].user, /原文：one\n译文：translated-one\n/);
});

test('Segment audio plays in seg_id order and never waits for later Segments', async () => {
  const asr = scriptedAsr([
    { at: 20, events: [{ type: 'stable', text: 'one' }] },
    { at: 400, events: [{ type: 'stable', text: 'two' }] },
  ]);
  const translator = scriptedTranslator(['translated-one', 'translated-two']);
  const ttsCalls = [];
  let releaseFirst;
  const firstGate = new Promise((resolve) => {
    releaseFirst = resolve;
  });
  const tts = {
    name: 'scripted',
    async synthesize({ seg_id }) {
      ttsCalls.push(seg_id);
      if (seg_id === 1) await firstGate;
      return { format: 'wav', data: new Uint8Array([seg_id]) };
    },
  };
  const { session, clock, events } = build({ asr, translator, tts });
  start(session);

  for (let segment = 0; segment < 2; segment++) {
    session.handleAudio(pcmFrame(20, 0.2));
    clock.advance(20);
    for (let i = 0; i < 18; i++) {
      session.handleAudio(pcmFrame(20, 0));
      clock.advance(20);
    }
  }

  await new Promise((resolve) => setImmediate(resolve));
  assert.deepEqual(ttsCalls, [1], 'Segment 2 must not start before Segment 1 finishes');

  releaseFirst();
  await session.idle();

  const spoken = events
    .filter((event) => ['tts_begin', 'tts_chunk', 'tts_end'].includes(event.type))
    .map((event) => `${event.type}:${event.seg_id}`);
  assert.deepEqual(spoken, [
    'tts_begin:1',
    'tts_chunk:1',
    'tts_end:1',
    'tts_begin:2',
    'tts_chunk:2',
    'tts_end:2',
  ]);
});

test('a TTS failure becomes an error event that names the Provider and fallbacks', async () => {
  const tts = {
    name: 'kokoro',
    async synthesize() {
      throw new Error(
        'Kokoro TTS is unreachable at http://127.0.0.1:8880: connect ECONNREFUSED. ' +
          'Set TTS_PROVIDER=system or mock to run without Kokoro.',
      );
    },
  };
  const { session, clock, events } = build({ tts });
  start(session);
  await speechThenSilence(session, clock, 18);

  const error = events.find((event) => event.type === 'error');
  assert.equal(error.provider, 'tts');
  assert.match(error.message, /Kokoro/);
  assert.match(error.message, /TTS_PROVIDER=system or mock/);
});

test('each Commit writes the spec JSON log record', async () => {
  const { session, clock, records } = build();
  start(session);
  await speechThenSilence(session, clock, 18);

  assert.equal(records.length, 1);
  assert.deepEqual(Object.keys(records[0]).sort(), [
    'asr_final',
    'asr_final_at',
    'direction',
    'echo_suspect',
    'first_token',
    'forced',
    'forced_fail',
    'held',
    'merges',
    'seg_id',
    'speech_end',
    'tts_first_chunk',
  ]);
  assert.equal(records[0].seg_id, 1);
  assert.equal(records[0].asr_final, 'one');
  assert.equal(typeof records[0].asr_final_at, 'number');
  assert.ok(records[0].asr_final_at >= records[0].speech_end);
  assert.equal(records[0].held, false);
  assert.equal(records[0].merges, 0);
  assert.equal(records[0].forced, false);
  assert.equal(records[0].forced_fail, false);
  assert.equal(records[0].echo_suspect, 0);
  assert.equal(typeof records[0].speech_end, 'number');
  assert.equal(typeof records[0].first_token, 'number');
  assert.equal(typeof records[0].tts_first_chunk, 'number');
});

test('rolling context carries the previous Segment into the next request', async () => {
  const asr = scriptedAsr([
    { at: 20, events: [{ type: 'stable', text: 'one' }] },
    { at: 400, events: [{ type: 'stable', text: 'two' }] },
  ]);
  const translator = scriptedTranslator(['translated-one', 'translated-two']);
  const { session, clock } = build({ asr, translator });
  start(session);

  await speechThenSilence(session, clock, 18);
  await speechThenSilence(session, clock, 18);

  const second = translator.requests[1];
  assert.match(second.user, /\[已译上下文\]\n原文：one\n译文：translated-one\n\[新段落\]\ntwo$/);
  assert.equal(second.system, 'SYSTEM ');
});

test('live microphone audio with mock ASR emits one explanatory error and nothing else', async () => {
  const asr = scriptedAsr([], { name: 'mock', acceptsLive: false });
  const { session, events } = build({ asr });
  session.handleMessage({ type: 'start', mode: 'listen', src: 'zh', tgt: 'ja' });

  session.handleAudio(pcmFrame(20, 0.2));
  session.handleAudio(pcmFrame(20, 0.2));
  await session.idle();

  const errors = events.filter((e) => e.type === 'error');
  assert.equal(errors.length, 1);
  assert.equal(errors[0].provider, 'asr');
  assert.match(errors[0].message, /bundled samples/);
  assert.equal(events.filter((e) => e.type === 'segment').length, 0);
});

test('switching from a bundled sample back to live input triggers the mock-ASR error', async () => {
  const asr = scriptedAsr([], { name: 'mock', acceptsLive: false });
  const { session, clock, events } = build({ asr });
  start(session);
  session.handleAudio(pcmFrame(20, 0.2));
  clock.advance(20);
  await session.idle();
  assert.equal(events.filter((e) => e.type === 'error').length, 0, 'sample input is accepted');

  session.handleMessage({ type: 'input', source: 'live' });
  session.handleAudio(pcmFrame(20, 0.2));
  await session.idle();

  const errors = events.filter((e) => e.type === 'error');
  assert.equal(errors.length, 1);
  assert.match(errors[0].message, /bundled samples/);
});

test('unknown Direction is rejected with a protocol error', () => {
  const { session, events } = build();
  session.handleMessage({ type: 'start', mode: 'listen', src: 'xx', tgt: 'zz' });
  const error = events.find((e) => e.type === 'error');
  assert.equal(error.kind, 'protocol');
  assert.match(error.message, /unknown direction/);
});

test('2.5 s of confirmed but untranslated speech Cuts the Segment', async () => {
  const asr = scriptedAsr([{ at: 20, events: [{ type: 'stable', text: 'one' }] }]);
  const { session, clock, events } = build({ asr });
  start(session);

  feedSpeech(session, clock, 2400);
  await session.idle();
  assert.equal(events.filter((e) => e.type === 'segment').length, 0, '2.4 s must not Cut');

  feedSpeech(session, clock, 140);
  await session.idle();
  const segments = events.filter((e) => e.type === 'segment');
  assert.equal(segments.length, 1);
  assert.equal(segments[0].src, 'one');
});

test('Stable Prefix reaching the clause-length threshold Cuts (char unit)', async () => {
  const asr = scriptedAsr([
    { at: 20, events: [{ type: 'stable', text: 'abc' }] },
    { at: 100, events: [{ type: 'stable', text: 'defg' }] },
  ]);
  const config = { 'zh->ja': { clauseLength: 8, unit: 'char' } };
  const { session, clock, events } = build({ asr, pairConfig: config });
  start(session);

  feedSpeech(session, clock, 400);
  await session.idle();
  const segments = events.filter((e) => e.type === 'segment');
  assert.equal(segments.length, 1);
  assert.equal(segments[0].src, 'abc defg');
});

test('the clause-length threshold honours the word unit', async () => {
  const asr = scriptedAsr([{ at: 20, events: [{ type: 'stable', text: 'hello world' }] }]);
  const config = { 'ja->zh': { clauseLength: 2, unit: 'word' } };
  const { session, clock, events } = build({ asr, pairConfig: config });
  start(session, 'ja', 'zh');

  feedSpeech(session, clock, 200);
  await session.idle();
  assert.equal(events.filter((e) => e.type === 'segment' && e.src === 'hello world').length, 1);
});

test('a Direction with no pair config never Cuts on clause length', async () => {
  const asr = scriptedAsr([{ at: 20, events: [{ type: 'stable', text: 'x'.repeat(60) }] }]);
  const config = { 'ja->zh': { clauseLength: 2, unit: 'word' } };
  const { session, clock, events } = build({ asr, pairConfig: config });
  start(session, 'zh', 'ja');

  feedSpeech(session, clock, 1000);
  await session.idle();
  assert.equal(events.filter((e) => e.type === 'segment').length, 0, 'long text alone must not Cut');

  feedSilence(session, clock, 360);
  await session.idle();
  assert.equal(events.filter((e) => e.type === 'segment').length, 1, 'silence still Cuts');
});

test('PTT release Cuts immediately even mid-word', async () => {
  const asr = scriptedAsr([
    { at: 20, events: [{ type: 'partial', text: 'hel' }, { type: 'stable', text: 'hel' }] },
  ]);
  const { session, clock, events } = build({ asr });
  start(session);
  session.handleMessage({ type: 'ptt', state: 'down' });

  feedSpeech(session, clock, 200);
  await session.idle();
  assert.equal(events.filter((e) => e.type === 'segment').length, 0);

  session.handleMessage({ type: 'ptt', state: 'up' });
  await session.idle();
  const segments = events.filter((e) => e.type === 'segment');
  assert.equal(segments.length, 1);
  assert.equal(segments[0].src, 'hel');
});

test('while PTT is held every automatic Cut is suspended', async () => {
  const asr = scriptedAsr([{ at: 20, events: [{ type: 'stable', text: 'abcdef' }] }]);
  const config = { 'zh->ja': { clauseLength: 3, unit: 'char' } };
  const { session, clock, events, translator } = build({ asr, pairConfig: config });
  start(session);
  session.handleMessage({ type: 'ptt', state: 'down' });

  feedSpeech(session, clock, 2600); // clause length and accumulation would Cut
  feedSilence(session, clock, 400); // silence would Cut
  await session.idle();
  assert.equal(events.filter((e) => e.type === 'segment').length, 0);
  assert.equal(translator.requests.length, 0);

  session.handleMessage({ type: 'ptt', state: 'up' });
  await session.idle();
  const segments = events.filter((e) => e.type === 'segment');
  assert.equal(segments.length, 1);
  assert.equal(segments[0].src, 'abcdef');
});

test('LocalAgreement-2 makes a prefix stable only after two identical hypotheses', async () => {
  const asr = scriptedAsr([
    { at: 20, events: [{ type: 'partial', text: 'hello' }] },
    { at: 40, events: [{ type: 'partial', text: 'hello' }] },
  ]);
  asr.revisable = true;
  const { session, clock, events, translator } = build({ asr });
  start(session);

  feedSpeech(session, clock, 200);
  await session.idle();
  assert.equal(events.filter((e) => e.type === 'segment').length, 0, 'stable text waits for a Cut');
  assert.equal(translator.requests.length, 0, 'Partials are never translated');

  session.handleMessage({ type: 'ptt', state: 'up' });
  await session.idle();
  const segments = events.filter((e) => e.type === 'segment');
  assert.equal(segments.length, 1);
  assert.equal(segments[0].src, 'hello');
  assert.equal(translator.requests.length, 1);
});

test('a single revisable hypothesis never becomes stable', async () => {
  const asr = scriptedAsr([{ at: 20, events: [{ type: 'partial', text: 'hello' }] }]);
  asr.revisable = true;
  const { session, clock, events } = build({ asr });
  start(session);

  feedSpeech(session, clock, 200);
  session.handleMessage({ type: 'ptt', state: 'up' });
  await session.idle();
  assert.equal(events.filter((e) => e.type === 'segment').length, 0);
});

test('a Partial alone is never translated or committed', async () => {
  const asr = scriptedAsr([{ at: 20, events: [{ type: 'partial', text: 'hello' }] }]);
  const { session, clock, events, translator } = build({ asr });
  start(session);

  feedSpeech(session, clock, 200);
  feedSilence(session, clock, 360);
  await session.idle();
  assert.equal(events.filter((e) => e.type === 'segment').length, 0);
  assert.equal(translator.requests.length, 0);
});

test('a clause Cut and a silence Cut close separate Segments', async () => {
  const asr = scriptedAsr([
    { at: 20, events: [{ type: 'stable', text: 'abc' }] },
    { at: 40, events: [{ type: 'stable', text: 'de' }] },
    { at: 500, events: [{ type: 'stable', text: 'xyz' }] },
  ]);
  const config = { 'zh->ja': { clauseLength: 5, unit: 'char' } };
  const { session, clock, events } = build({ asr, pairConfig: config });
  start(session);

  feedSpeech(session, clock, 700);
  await session.idle();
  assert.equal(events.filter((e) => e.type === 'segment').length, 1, 'clause Cut while speech continues');

  feedSilence(session, clock, 360);
  await session.idle();
  const segments = events.filter((e) => e.type === 'segment');
  assert.deepEqual(segments.map((e) => e.src), ['abc de', 'xyz']);
});

test('Stable Prefix events that carry their own spacing are joined without an inserted space', async () => {
  const asr = scriptedAsr([
    { at: 20, events: [{ type: 'stable', text: '会議', glue: '' }] },
    { at: 40, events: [{ type: 'stable', text: '録です。', glue: '' }] },
  ]);
  const config = { 'zh->ja': { clauseLength: 5, unit: 'char' } };
  const { session, clock, events } = build({ asr, pairConfig: config });
  start(session);

  feedSpeech(session, clock, 700);
  await session.idle();
  const segments = events.filter((e) => e.type === 'segment');
  assert.equal(segments.length, 1);
  assert.equal(segments[0].src, '会議録です。');
});

test('LocalAgreement-2 joins successive stable prefixes without inventing spaces', async () => {
  const asr = scriptedAsr([
    { at: 20, events: [{ type: 'partial', text: 'hello wor' }] },
    { at: 40, events: [{ type: 'partial', text: 'hello world' }] },
    { at: 60, events: [{ type: 'partial', text: 'hello world' }] },
  ]);
  asr.revisable = true;
  const { session, clock, events } = build({ asr });
  start(session);

  feedSpeech(session, clock, 100);
  session.handleMessage({ type: 'ptt', state: 'up' });
  await session.idle();
  const segments = events.filter((e) => e.type === 'segment');
  assert.equal(segments.length, 1);
  assert.equal(segments[0].src, 'hello world');
});

test('a lone ⏸ after trimming Holds the Segment: no seg_id, output, or speech', async () => {
  const translator = scriptedTranslator(['  ⏸\n']);
  const { session, clock, events, records, tts } = build({ translator });
  start(session);

  await speechThenSilence(session, clock, 18);

  assert.equal(translator.requests.length, 1);
  assert.equal(events.filter((e) => e.type === 'segment').length, 0);
  assert.equal(records.length, 0, 'a Held Segment has no seg_id and no log record yet');
  assert.equal(tts.calls.length, 0);
  assert.ok(events.some((e) => e.type === 'hold' && e.held === true));
});

test('text that merely contains ⏸ is a normal translation', async () => {
  const translator = scriptedTranslator(['⏸ is the hold marker']);
  const { session, clock, events } = build({ translator });
  start(session);

  await speechThenSilence(session, clock, 18);

  const segments = events.filter((e) => e.type === 'segment');
  assert.equal(segments.length, 1);
  assert.equal(segments[0].tgt, '⏸ is the hold marker');
});

test('a Held Segment merges with the next Cut and retries with both parts', async () => {
  const asr = scriptedAsr([
    { at: 20, events: [{ type: 'stable', text: 'この報告書は、確認が終わってから' }] },
    { at: 400, events: [{ type: 'stable', text: '提出されます' }] },
  ]);
  const translator = scriptedTranslator(['⏸', 'この報告書は、確認が終わってから提出されます。']);
  const { session, clock, events, records } = build({ asr, translator });
  start(session, 'ja', 'zh');

  await speechThenSilence(session, clock, 18);
  assert.equal(events.filter((e) => e.type === 'segment').length, 0, 'nothing before the next Cut arrives');
  assert.equal(translator.requests.length, 1);
  assert.match(translator.requests[0].user, /この報告書は、確認が終わってから$/);

  await speechThenSilence(session, clock, 18);
  const segments = events.filter((e) => e.type === 'segment');
  assert.equal(segments.length, 1);
  assert.ok(translator.requests[1].user.includes('この報告書は、確認が終わってから'));
  assert.ok(translator.requests[1].user.includes('提出されます'));
  assert.equal(records[0].held, true);
  assert.equal(records[0].merges, 1);
  assert.equal(records[0].forced, false);
  assert.equal(records[0].forced_fail, false);

  clock.advance(5000); // the stale Hold timer must not force a committed Segment
  await session.idle();
  assert.equal(translator.requests.length, 2);
  assert.equal(events.filter((e) => e.type === 'segment').length, 1);
});

test('the Hold limit issues a Forced Translation with the prompt-asset instruction', async () => {
  const translator = scriptedTranslator(['⏸', 'forced translation']);
  const { session, clock, events, records, tts } = build({ translator });
  start(session);

  await speechThenSilence(session, clock, 18);
  assert.equal(translator.requests.length, 1);

  clock.advance(2400);
  await session.idle();
  assert.equal(translator.requests.length, 1, 'must not force before the 2.5 s limit');

  clock.advance(100); // the timer fires with no further speech
  await session.idle();

  assert.equal(translator.requests.length, 2);
  const forced = translator.requests[1];
  assert.equal(forced.user, translator.requests[0].user, 'Forced Translation reuses the same request');
  assert.equal(forced.system, `${translator.requests[0].system}\n${FORCED_INSTRUCTION}`);

  const segments = events.filter((e) => e.type === 'segment');
  assert.equal(segments.length, 1);
  assert.equal(segments[0].seg_id, 1, 'seg_ids stay gap-free');
  assert.equal(segments[0].tgt, 'forced translation');
  assert.equal(segments[0].speak, true);
  assert.equal(tts.calls.length, 1);
  assert.equal(records[0].held, true);
  assert.equal(records[0].merges, 0);
  assert.equal(records[0].forced, true);
  assert.equal(records[0].forced_fail, false);
});

test('a merged Segment that Holds again past the limit is force-translated with both parts', async () => {
  const asr = scriptedAsr([
    { at: 20, events: [{ type: 'stable', text: 'part-one' }] },
    { at: 400, events: [{ type: 'stable', text: 'part-two' }] },
  ]);
  const translator = scriptedTranslator(['⏸', '⏸', 'merged translation']);
  const { session, clock, events, records } = build({ asr, translator });
  start(session);

  await speechThenSilence(session, clock, 18);
  await speechThenSilence(session, clock, 18);
  assert.equal(translator.requests.length, 2, 'the merged Segment was retried');

  clock.advance(2000); // still before 2.5 s from the first Hold
  await session.idle();
  assert.equal(translator.requests.length, 2, 'the merge must not restart the Hold limit');

  clock.advance(120); // 2.5 s from the first Hold
  await session.idle();
  assert.equal(translator.requests.length, 3);
  assert.match(translator.requests[2].text, /part-one/);
  assert.match(translator.requests[2].text, /part-two/);
  assert.equal(translator.requests[2].system, `${translator.requests[0].system}\n${FORCED_INSTRUCTION}`);

  const segments = events.filter((e) => e.type === 'segment');
  assert.equal(segments.length, 1);
  assert.equal(segments[0].src, 'part-one part-two');
  assert.equal(records[0].merges, 1);
  assert.equal(records[0].forced, true);
  assert.equal(records[0].forced_fail, false);
});

test('a Forced Translation that still Holds commits source only and logs forced_fail', async () => {
  const asr = scriptedAsr([
    { at: 20, events: [{ type: 'stable', text: 'source-one' }] },
    { at: 600, events: [{ type: 'stable', text: 'source-two' }] },
  ]);
  const translator = scriptedTranslator(['⏸', '⏸', 'translated-two']);
  const { session, clock, events, records, tts } = build({ asr, translator });
  start(session);

  await speechThenSilence(session, clock, 18);
  clock.advance(2500);
  await session.idle();

  const segments = events.filter((e) => e.type === 'segment');
  assert.equal(segments.length, 1);
  assert.equal(segments[0].seg_id, 1);
  assert.equal(segments[0].src, 'source-one');
  assert.equal(segments[0].tgt, '');
  assert.equal(segments[0].speak, false);
  assert.equal(tts.calls.length, 0, 'no speech for a forced failure');
  assert.equal(records[0].held, true);
  assert.equal(records[0].forced, true);
  assert.equal(records[0].forced_fail, true);
  assert.equal(records[0].tts_first_chunk, null);

  // The next Segment takes the next seg_id: nothing was burned by the Hold.
  await speechThenSilence(session, clock, 18);
  const next = events.filter((e) => e.type === 'segment')[1];
  assert.equal(next.seg_id, 2);
  assert.equal(next.src, 'source-two');
  assert.equal(next.speak, true);
  assert.equal(records[1].forced_fail, false);
});

test('packs from start reach the translator request as a glossary', async () => {
  const packs = createRegistry({
    promptFiles: [{ name: 'system_zh2ja.txt', text: 'SYSTEM {{GLOSSARY}}' }],
    packFiles: [{ name: 'conference_zh2ja.tsv', text: '预算\t予算\n' }],
  });
  const translator = scriptedTranslator(['translated-one']);
  const { session, clock } = build({ registry: packs, translator });
  session.handleMessage({ type: 'start', mode: 'listen', src: 'zh', tgt: 'ja', packs: ['conference'] });
  session.handleMessage({ type: 'sample', name: 'placeholder' });
  await speechThenSilence(session, clock, 18);

  assert.equal(translator.requests.length, 1);
  assert.match(translator.requests[0].system, /预算 → 予算/);
});

test('an unknown pack is a protocol error and is never used', async () => {
  const packs = createRegistry({
    promptFiles: [{ name: 'system_zh2ja.txt', text: 'SYSTEM {{GLOSSARY}}' }],
    packFiles: [{ name: 'conference_zh2ja.tsv', text: '预算\t予算\n' }],
  });
  const translator = scriptedTranslator(['translated-one']);
  const { session, clock, events } = build({ registry: packs, translator });
  session.handleMessage({
    type: 'start',
    mode: 'listen',
    src: 'zh',
    tgt: 'ja',
    packs: ['nope', 'conference'],
  });
  session.handleMessage({ type: 'sample', name: 'placeholder' });

  const error = events.find((event) => event.type === 'error');
  assert.equal(error.kind, 'protocol');
  assert.equal(error.message, 'unknown pack: nope');

  await speechThenSilence(session, clock, 18);
  assert.equal(translator.requests.length, 1, 'the known pack still starts the Session');
  assert.match(translator.requests[0].system, /预算 → 予算/);
});

test('a Provider error keeps its kind and fallback in the client event', async () => {
  const translator = {
    name: 'scripted',
    async *translate() {
      throw Object.assign(new Error('bad key'), { kind: 'auth', fallback: 'set LLM_API_KEY' });
    },
  };
  const { session, clock, events } = build({ translator });
  start(session);
  await speechThenSilence(session, clock, 18);

  const error = events.find((event) => event.type === 'error');
  assert.equal(error.provider, 'translator');
  assert.equal(error.kind, 'auth');
  assert.equal(error.fallback, 'set LLM_API_KEY');
});

test('stop drains the final ASR line and commits it before release', async () => {
  let closed = false;
  let pushes = 0;
  const asr = {
    name: 'scripted',
    open() {
      closed = false;
      pushes = 0;
    },
    close() {
      closed = true;
    },
    push() {
      if (!closed) return [];
      pushes += 1;
      return pushes === 2 ? [{ type: 'stable', text: 'final-line' }] : [];
    },
  };
  const translator = scriptedTranslator(['translated-final']);
  const { session, clock, events } = build({ asr, translator });
  start(session);

  session.handleMessage({ type: 'stop' });
  clock.advance(100);
  await session.idle();

  const segment = events.find((event) => event.type === 'segment');
  assert.equal(segment.src, 'final-line');
  assert.equal(segment.tgt, 'translated-final');
});

test('leaked reasoning before a closing think marker never reaches the Segment or TTS', async () => {
  const leaked =
    '________\n[新段落] この報告書は、確認が終わってから提出されます。\n</x|think>\n' +
    '確認が終わってから、この報告書を提出します。';
  const translator = scriptedTranslator([leaked]);
  const { session, clock, events, tts } = build({ translator });
  start(session, 'ja', 'zh');

  await speechThenSilence(session, clock, 18);

  const segments = events.filter((event) => event.type === 'segment');
  assert.equal(segments.length, 1);
  assert.equal(segments[0].tgt, '確認が終わってから、この報告書を提出します。');
  assert.deepEqual(tts.calls, ['確認が終わってから、この報告書を提出します。']);
  assert.doesNotMatch(JSON.stringify(events), /提出されます|新段落/);
});

test('the guard keeps only text after the last closing think tag, case-insensitively', async () => {
  const translator = scriptedTranslator(['DRAFT\n</Think>\nmid\n</|THINK|>\nFINAL']);
  const { session, clock, events, tts } = build({ translator });
  start(session);

  await speechThenSilence(session, clock, 18);

  assert.equal(events.find((event) => event.type === 'segment').tgt, 'FINAL');
  assert.deepEqual(tts.calls, ['FINAL']);
});

test('the guard sees a think marker split across translator chunks', async () => {
  const translator = {
    name: 'scripted',
    async *translate() {
      yield 'DRAFT</x|th';
      yield 'ink>\nFIN';
      yield 'AL';
    },
  };
  const { session, clock, events } = build({ translator });
  start(session);

  await speechThenSilence(session, clock, 18);

  assert.equal(events.find((event) => event.type === 'segment').tgt, 'FINAL');
});

test('a lone ⏸ left after the stripped reasoning still Holds', async () => {
  const translator = scriptedTranslator(['DRAFT\n</think>\n⏸', 'FINAL']);
  const { session, clock, events, tts } = build({ translator });
  start(session);

  await speechThenSilence(session, clock, 18);
  assert.ok(events.some((event) => event.type === 'hold' && event.held === true));
  assert.equal(events.filter((event) => event.type === 'segment').length, 0);
  assert.equal(tts.calls.length, 0);

  clock.advance(2500);
  await session.idle();
  assert.equal(events.find((event) => event.type === 'segment').tgt, 'FINAL');
});

test('an answer that is only leaked reasoning is a protocol error, not an empty Segment', async () => {
  const translator = scriptedTranslator(['DRAFT\n</x|think>\n   ']);
  const { session, clock, events, tts } = build({ translator });
  start(session);

  await speechThenSilence(session, clock, 18);

  assert.equal(events.filter((event) => event.type === 'segment').length, 0);
  assert.equal(tts.calls.length, 0);
  const error = events.find((event) => event.type === 'error');
  assert.equal(error.provider, 'translator');
  assert.equal(error.kind, 'protocol');
  assert.match(error.message, /no text/);
});

test('the stop drain is bounded: a line after the timeout is dropped', async () => {
  let closed = false;
  let pushes = 0;
  const asr = {
    name: 'scripted',
    open() {
      closed = false;
      pushes = 0;
    },
    close() {
      closed = true;
    },
    push() {
      if (!closed) return [];
      pushes += 1;
      return pushes > 62 ? [{ type: 'stable', text: 'too-late' }] : [];
    },
  };
  const { session, clock, events } = build({ asr });
  start(session);

  session.handleMessage({ type: 'stop' });
  clock.advance(4000);
  await session.idle();
  assert.equal(events.filter((event) => event.type === 'segment').length, 0);

  clock.advance(4000);
  await session.idle();
  assert.equal(events.filter((event) => event.type === 'segment').length, 0);
});

test('Listen Session Trace is the frozen oracle', async () => {
  const asr = scriptedAsr([
    { at: 20, events: [{ type: 'partial', text: 'one' }, { type: 'stable', text: 'one' }] },
    { at: 400, events: [{ type: 'partial', text: 'two' }, { type: 'stable', text: 'two' }] },
  ]);
  const translator = scriptedTranslator(['translated-one', 'translated-two']);
  const { session, clock, events, trace } = build({ asr, translator, trace: 'listen-basic' });
  start(session);

  await speechThenSilence(session, clock, 18);
  await speechThenSilence(session, clock, 18);

  assert.deepEqual(
    events.filter((event) => event.type === 'segment').map((event) => event.src),
    ['one', 'two'],
  );
  checkTrace(trace);
});

test('Listen live Session Trace records the v0.0.3 Gating divergence', async () => {
  const asr = scriptedAsr(
    [
      { at: 20, events: [{ type: 'stable', text: 'one' }] },
      { at: 420, events: [{ type: 'stable', text: 'two' }] },
    ],
    { acceptsLive: true },
  );
  const translator = scriptedTranslator(['translated-one', 'translated-two']);
  const { session, clock, events, trace } = build({ asr, translator, trace: 'listen-live-gating' });
  session.handleMessage({ type: 'start', mode: 'listen', src: 'zh', tgt: 'ja' });

  await speechThenSilence(session, clock, 18);
  await speechThenSilence(session, clock, 18);

  assert.equal(
    events.filter((event) => event.type === 'segment').length,
    1,
    'v0.0.3 withholds Listen live mic frames while audio TTS is pending',
  );
  checkTrace(trace);
});

test('Listen Hold-merge Session Trace is the frozen oracle', async () => {
  const asr = scriptedAsr([
    { at: 20, events: [{ type: 'stable', text: 'part one' }] },
    { at: 420, events: [{ type: 'stable', text: 'part two' }] },
    { at: 820, events: [{ type: 'stable', text: 'part three' }] },
  ]);
  const translator = scriptedTranslator(['⏸', '⏸', 'merged-translation']);
  const { session, clock, events, records, trace } = build({ asr, translator, trace: 'listen-hold-merge' });
  start(session);

  await speechThenSilence(session, clock, 18);
  await speechThenSilence(session, clock, 18);
  await speechThenSilence(session, clock, 18);

  const segments = events.filter((event) => event.type === 'segment');
  assert.equal(segments.length, 1);
  assert.equal(segments[0].src, 'part one part two part three');
  assert.deepEqual(
    events.filter((event) => event.type === 'hold').map((event) => event.merges),
    [0, 1],
  );
  assert.equal(translator.requests.length, 3);
  assert.equal(translator.requests[1].text, 'part one part two');
  assert.equal(translator.requests[2].text, 'part one part two part three');
  assert.equal(records.length, 1);
  assert.equal(records[0].held, true);
  assert.equal(records[0].merges, 2);
  assert.equal(records[0].forced, false);
  checkTrace(trace);
});

test('Listen forced-translation Session Trace is the frozen oracle', async () => {
  const translator = scriptedTranslator(['⏸', 'forced-translation']);
  const { session, clock, events, records, trace } = build({ translator, trace: 'listen-hold-force' });
  start(session);

  await speechThenSilence(session, clock, 18);
  clock.advance(2500);
  await session.idle();

  const segments = events.filter((event) => event.type === 'segment');
  assert.equal(segments.length, 1);
  assert.equal(segments[0].tgt, 'forced-translation');
  assert.equal(translator.requests.length, 2);
  assert.ok(translator.requests[1].system.endsWith(FORCED_INSTRUCTION));
  assert.equal(records.length, 1);
  assert.equal(records[0].forced, true);
  assert.equal(records[0].merges, 0);
  checkTrace(trace);
});

test('Listen forced-failure Session Trace is the frozen oracle', async () => {
  const translator = scriptedTranslator(['⏸', '⏸']);
  const { session, clock, events, records, tts, trace } = build({
    translator,
    trace: 'listen-hold-force-fail',
  });
  start(session);

  await speechThenSilence(session, clock, 18);
  clock.advance(2500);
  await session.idle();

  const segments = events.filter((event) => event.type === 'segment');
  assert.equal(segments.length, 1);
  assert.equal(segments[0].tgt, '');
  assert.equal(segments[0].speak, false);
  assert.equal(tts.calls.length, 0);
  assert.equal(records.length, 1);
  assert.equal(records[0].forced_fail, true);
  checkTrace(trace);
});

test('Listen Rolling Context Session Trace is the frozen oracle', async () => {
  const marks = Array.from({ length: 10 }, (_, index) => ({
    at: 20 + 400 * index,
    events: [{ type: 'stable', text: `s${index}` }],
  }));
  const translator = scriptedTranslator(Array.from({ length: 10 }, (_, index) => `t${index}`));
  const { session, clock, events, trace } = build({
    asr: scriptedAsr(marks),
    translator,
    trace: 'listen-rolling-context',
  });
  start(session);

  for (let index = 0; index < 10; index++) await speechThenSilence(session, clock, 18);

  assert.equal(events.filter((event) => event.type === 'segment').length, 10);
  assert.equal(translator.requests.length, 10);
  assert.equal(
    translator.requests[1].user,
    '[已译上下文]\n原文：s0\n译文：t0\n[新段落]\ns1',
  );
  assert.equal(
    translator.requests[2].user,
    '[已译上下文]\n原文：s0\n译文：t0\n原文：s1\n译文：t1\n[新段落]\ns2',
  );
  const last = translator.requests[9].user;
  assert.equal(last.match(/原文：/g).length, 8, 'Rolling Context keeps exactly 8 pairs');
  assert.match(last, /原文：s1\n译文：t1\n/);
  assert.doesNotMatch(last, /原文：s0\n/);
  checkTrace(trace);
});

test('Listen Cut-modes Session Trace is the frozen oracle', async () => {
  const asr = scriptedAsr([
    { at: 20, events: [{ type: 'stable', text: 'a' }] },
    { at: 400, events: [{ type: 'stable', text: 'ab' }] },
    { at: 2920, events: [{ type: 'stable', text: 'abc' }] },
    { at: 2940, events: [{ type: 'stable', text: 'abcd' }] },
  ]);
  const translator = scriptedTranslator(['t-a', 't-ab', 't-abc', 't-abcd']);
  const { session, clock, events, trace } = build({
    asr,
    translator,
    pairConfig: { 'zh->ja': { clauseLength: 3, unit: 'char' } },
    trace: 'listen-cuts',
  });
  start(session);

  // Silence Cut.
  await speechThenSilence(session, clock, 18);
  // Accumulated-duration Cut at 2.5 s of confirmed speech.
  feedSpeech(session, clock, 2500);
  await session.idle();
  // Clause-length Cut on the Stable Prefix.
  feedSpeech(session, clock, 40);
  await session.idle();
  // Push-to-talk release Cut.
  session.handleMessage({ type: 'ptt', state: 'down' });
  feedSpeech(session, clock, 200);
  session.handleMessage({ type: 'ptt', state: 'up' });
  await session.idle();

  assert.deepEqual(
    events.filter((event) => event.type === 'segment').map((event) => event.src),
    ['a', 'ab', 'abc', 'abcd'],
  );
  checkTrace(trace);
});

test('Listen glossary Session Trace pins the glossary translator messages', async () => {
  const glossaryRegistry = createRegistry({
    promptFiles: [
      { name: 'system_zh2ja.txt', text: 'SYSTEM {{GLOSSARY}}' },
      { name: 'system_ja2zh.txt', text: 'SYSTEM {{GLOSSARY}}' },
    ],
    packFiles: [
      { name: 'conference_zh2ja.tsv', text: '人工智能\tAI\n神经网络\tneural network' },
    ],
  });
  const translator = scriptedTranslator(['translated-one']);
  const { session, clock, events, trace } = build({
    registry: glossaryRegistry,
    translator,
    trace: 'listen-glossary',
  });
  session.handleMessage({ type: 'start', mode: 'listen', src: 'zh', tgt: 'ja', packs: ['conference'] });
  session.handleMessage({ type: 'sample', name: 'placeholder' });

  await speechThenSilence(session, clock, 18);

  assert.equal(events.filter((event) => event.type === 'segment').length, 1);
  assert.equal(translator.requests[0].system, 'SYSTEM 人工智能 → AI\n神经网络 → neural network');
  assert.equal(translator.requests[0].user, '[已译上下文]\n[新段落]\none');
  checkTrace(trace);
});
