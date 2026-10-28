import { test } from 'node:test';
import assert from 'node:assert/strict';

import { createSession } from '../core/session.js';
import { createRegistry } from '../core/packs-registry.js';
import { create as createMockTts } from '../core/providers/tts-mock.js';
import { makeClock, pcmFrame, scriptedTranslator, scriptedTts } from './helpers/fakes.js';

// Two Directions: zh->ja is the user's ("me") Direction in Converse.
function registry() {
  return createRegistry({
    promptFiles: [
      { name: 'system_zh2ja.txt', text: 'PROMPT_ME' },
      { name: 'system_ja2zh.txt', text: 'PROMPT_THEM' },
    ],
  });
}

// One script per open, in open order. A script is [{ at: audioMs, text }].
function scriptedStreams(scripts) {
  const opens = [];
  let marks = [];
  let fired = [];
  let audioMs = 0;
  return {
    name: 'scripted',
    acceptsLive: true,
    opens,
    pushCount: 0,
    open(direction) {
      opens.push(direction.src);
      marks = scripts[opens.length - 1] ?? [];
      fired = marks.map(() => false);
      audioMs = 0;
    },
    close() {},
    push(frame) {
      this.pushCount++;
      audioMs = frame.audioMs;
      const out = [];
      marks.forEach((mark, i) => {
        if (!fired[i] && audioMs >= mark.at) {
          fired[i] = true;
          out.push({ type: 'stable', text: mark.text });
        }
      });
      return out;
    },
  };
}

function build(overrides = {}) {
  const clock = overrides.clock ?? makeClock();
  const asr = overrides.asr ?? scriptedStreams([]);
  const translator = overrides.translator ?? scriptedTranslator([]);
  const tts = overrides.tts ?? scriptedTts();
  const events = [];
  const records = [];
  const session = createSession({
    providers: { asr, translator, tts },
    clock,
    registry: overrides.registry ?? registry(),
    pairConfig: overrides.pairConfig ?? {},
    forcedInstruction: '',
    emit: (event) => events.push(event),
    log: (record) => records.push(record),
  });
  return { session, clock, asr, translator, tts, events, records };
}

function startConverse(session, { src = 'zh', tgt = 'ja' } = {}) {
  session.handleMessage({ type: 'start', mode: 'converse', src, tgt });
}

// One speech frame supplies the ASR mark at audioMs 20; 18 silent frames exceed
// the 350 ms VAD window and Cut the Segment.
async function speakAndCut(session, clock, speechFrames = 1) {
  for (let i = 0; i < speechFrames; i++) {
    session.handleAudio(pcmFrame(20, 0.2));
    clock.advance(20);
  }
  for (let i = 0; i < 18; i++) {
    session.handleAudio(pcmFrame(20, 0));
    clock.advance(20);
  }
  await session.idle();
}

async function feedLive(session, clock, { speechFrames = 0, silenceFrames = 0 } = {}) {
  for (let i = 0; i < speechFrames; i++) {
    session.handleAudio(pcmFrame(20, 0.2));
    clock.advance(20);
  }
  for (let i = 0; i < silenceFrames; i++) {
    session.handleAudio(pcmFrame(20, 0));
    clock.advance(20);
  }
  await session.idle();
}

test('a one-Direction Language Pair is listen-only and cannot start Converse', () => {
  const threeLanguages = createRegistry({
    promptFiles: [
      { name: 'system_zh2ja.txt', text: 'PROMPT_ME' },
      { name: 'system_ja2zh.txt', text: 'PROMPT_THEM' },
      { name: 'system_zh2fr.txt', text: 'PROMPT_THIRD' },
    ],
  });
  const { session, events } = build({ registry: threeLanguages });
  const pairs = session.hello().pairs;
  assert.equal(pairs.find((pair) => pair.a === 'fr' || pair.b === 'fr').converse, false);
  assert.equal(pairs.find((pair) => pair.a === 'ja' || pair.b === 'ja').converse, true);

  session.handleMessage({ type: 'start', mode: 'converse', src: 'zh', tgt: 'fr' });
  assert.match(events.find((event) => event.type === 'error').message, /both directions/);
  assert.equal(events.some((event) => event.type === 'state'), false);
});

test('Turn me/them routes audio to its Direction; only my Direction is spoken', async () => {
  const asr = scriptedStreams([
    [{ at: 20, text: 'from me' }],
    [{ at: 20, text: 'from them' }],
  ]);
  const translator = scriptedTranslator(['translated-me', 'translated-them']);
  const { session, clock, tts, events } = build({ asr, translator });
  startConverse(session);

  await speakAndCut(session, clock);
  session.handleMessage({ type: 'turn', who: 'them' });
  session.handleMessage({ type: 'playback', seg_id: 1, state: 'end' });
  clock.advance(500); // the gating tail
  await speakAndCut(session, clock);

  const segments = events.filter((event) => event.type === 'segment');
  assert.equal(segments.length, 2);
  assert.deepEqual(segments[0].direction, { src: 'zh', tgt: 'ja' });
  assert.equal(segments[0].speak, true, 'my Direction is spoken');
  assert.deepEqual(segments[1].direction, { src: 'ja', tgt: 'zh' });
  assert.equal(segments[1].speak, false, 'their Direction is subtitle-only');

  assert.deepEqual(asr.opens.slice(0, 2), ['zh', 'ja'], 'the stream follows the Turn');
  assert.equal(translator.requests[0].system, 'PROMPT_ME');
  assert.equal(translator.requests[1].system, 'PROMPT_THEM');
  assert.deepEqual(tts.calls, ['translated-me']);
  assert.deepEqual(
    events.filter((event) => event.type === 'tts_begin').map((event) => event.seg_id),
    [1],
  );
});

test('Gating withholds live mic frames while own speech plays and through the 500 ms tail', async () => {
  const asr = scriptedStreams([[{ at: 20, text: 'question' }]]);
  const translator = scriptedTranslator(['translated-one']);
  const { session, clock, events } = build({ asr, translator });
  startConverse(session);
  await speakAndCut(session, clock);

  const pushes = asr.pushCount;
  session.handleAudio(pcmFrame(20, 0.2));
  assert.equal(asr.pushCount, pushes, 'mic frames are withheld while own speech plays');

  session.handleMessage({ type: 'playback', seg_id: 1, state: 'end' });
  assert.ok(
    events.some((event) => event.type === 'turn' && event.who === 'them' && event.auto === true),
    'my spoken output finishing switches the Turn to them',
  );

  clock.advance(499);
  session.handleAudio(pcmFrame(20, 0.2));
  assert.equal(asr.pushCount, pushes, 'the tail still blocks the mic');

  clock.advance(1);
  session.handleAudio(pcmFrame(20, 0.2));
  assert.equal(asr.pushCount, pushes + 1, 'the mic reopens after the tail');
});

test('without a playback report the estimate is audio duration + 1.5 s, then the tail', async () => {
  const asr = scriptedStreams([[{ at: 20, text: 'question' }]]);
  const translator = scriptedTranslator(['translated-one']);
  const tts = createMockTts({ durationMs: 1000 });
  const { session, clock, events } = build({ asr, translator, tts });
  startConverse(session);
  await speakAndCut(session, clock);
  const pushes = asr.pushCount;

  session.handleAudio(pcmFrame(20, 0.2));
  assert.equal(asr.pushCount, pushes, 'gated before the estimate fires');

  clock.advance(2499); // 1000 ms audio + 1500 ms estimate, minus 1
  session.handleAudio(pcmFrame(20, 0.2));
  assert.equal(asr.pushCount, pushes, 'still gated during the estimated playback');

  clock.advance(1);
  assert.ok(
    events.some((event) => event.type === 'turn' && event.who === 'them' && event.auto === true),
    'the estimate also finishes the spoken output and switches the Turn',
  );
  session.handleAudio(pcmFrame(20, 0.2));
  assert.equal(asr.pushCount, pushes, 'the tail follows the estimate');

  clock.advance(499);
  session.handleAudio(pcmFrame(20, 0.2));
  assert.equal(asr.pushCount, pushes, 'still in the tail');

  clock.advance(1);
  session.handleAudio(pcmFrame(20, 0.2));
  assert.equal(asr.pushCount, pushes + 1, 'the mic reopens after estimate + tail');
});

test('the fallback estimate reads the mp3 bitrate (Azure/Edge are 48 kbit/s)', async () => {
  const mp3 = new Uint8Array(6000); // 48 kbit/s for 1000 ms
  mp3.set([0xff, 0xf3, 0x6c, 0x00]); // MPEG2 Layer III, bitrate index 6
  const tts = {
    name: 'edge',
    async synthesize() {
      return { format: 'mp3', data: mp3 };
    },
  };
  const asr = scriptedStreams([[{ at: 20, text: 'question' }]]);
  const { session, clock } = build({ asr, translator: scriptedTranslator(['translated-one']), tts });
  startConverse(session);
  await speakAndCut(session, clock);
  const pushes = asr.pushCount;

  clock.advance(2499); // 1000 ms audio + 1500 ms estimate, minus 1
  session.handleAudio(pcmFrame(20, 0.2));
  assert.equal(asr.pushCount, pushes, 'still gated');

  clock.advance(1); // estimate ends, tail starts
  clock.advance(499);
  session.handleAudio(pcmFrame(20, 0.2));
  assert.equal(asr.pushCount, pushes, 'tail still gated');

  clock.advance(1);
  session.handleAudio(pcmFrame(20, 0.2));
  assert.equal(asr.pushCount, pushes + 1, 'released after estimate + tail');
});

test('an Echo Suspect is dropped, never translated, and expires after the tail', async () => {
  const asr = scriptedStreams([
    [
      { at: 20, text: 'first question' },
      { at: 400, text: 'hello world' },
      { at: 800, text: 'second question' },
    ],
    [],
    [{ at: 20, text: 'hello world' }],
  ]);
  const translator = scriptedTranslator(['hello world', 'translated-two', 'translated-three']);
  const tts = scriptedTts({ name: 'system' });
  const { session, clock, events, records } = build({ asr, translator, tts });
  startConverse(session);

  await speakAndCut(session, clock); // first question -> spoken "hello world"
  await feedLive(session, clock, { speechFrames: 1, silenceFrames: 18 }); // echo "hello world"
  await feedLive(session, clock, { speechFrames: 2, silenceFrames: 18 }); // second question

  const segments = events.filter((event) => event.type === 'segment');
  assert.deepEqual(
    segments.map((segment) => segment.src),
    ['first question', 'second question'],
    'the Echo Suspect never becomes a Segment',
  );
  assert.equal(translator.requests.length, 2, 'the Echo Suspect is never translated');
  assert.equal(records[1].echo_suspect, 1, 'the suspect is logged on the next Commit');
  assert.equal(tts.calls.length, 0, 'system TTS is text-only: no synthesis');
  assert.equal(events.filter((event) => event.type === 'tts_begin').length, 0);

  // Both spoken Segments end, so the Turn auto-switches and the tail starts.
  session.handleMessage({ type: 'playback', seg_id: 1, state: 'end' });
  session.handleMessage({ type: 'playback', seg_id: 2, state: 'end' });
  clock.advance(501); // past the echo tail
  session.handleMessage({ type: 'turn', who: 'me' });
  await speakAndCut(session, clock);

  const afterTail = events.filter((event) => event.type === 'segment');
  assert.equal(afterTail.length, 3);
  assert.equal(afterTail[2].src, 'hello world', 'the same text commits after the tail');
  assert.equal(afterTail[2].tgt, 'translated-three');
});

test('T5: 60 s of own TTS text fed back as mic input commits zero extra Segments', async () => {
  const marks = [{ at: 20, text: 'question' }];
  for (let at = 6000; at < 60000; at += 500) marks.push({ at, text: 'hello world' });
  const asr = scriptedStreams([marks]);
  const translator = scriptedTranslator(['hello world']);
  const tts = scriptedTts({ name: 'system' });
  const { session, clock, events } = build({ asr, translator, tts });
  startConverse(session);

  // Real time interleaves audio with Commit work; yield to the queue so the
  // first Segment is spoken before its echo starts arriving.
  const run = async (ms) => {
    for (let elapsed = 0; elapsed < ms; elapsed += 20) {
      session.handleAudio(pcmFrame(20, 0.2));
      clock.advance(20);
    }
    await session.idle();
  };
  await run(5000);
  for (let elapsed = 5000; elapsed < 60000; elapsed += 500) await run(500);

  const segments = events.filter((event) => event.type === 'segment');
  assert.equal(segments.length, 1, 'only the initial real Segment commits');
  assert.equal(segments[0].src, 'question');
  assert.equal(translator.requests.length, 1);
});

test('interleaved Directions share seg_ids but keep separate Rolling Contexts', async () => {
  const asr = scriptedStreams([
    [{ at: 20, text: 'one' }],
    [{ at: 20, text: 'two' }],
    [{ at: 20, text: 'three' }],
  ]);
  const translator = scriptedTranslator(['translated-one', 'translated-two', 'translated-three']);
  const { session, clock, events, tts } = build({ asr, translator });
  startConverse(session);

  await speakAndCut(session, clock); // me: one
  session.handleMessage({ type: 'playback', seg_id: 1, state: 'end' }); // -> them
  clock.advance(500); // clear the gating tail
  await speakAndCut(session, clock); // them: two
  session.handleMessage({ type: 'turn', who: 'me' });
  await speakAndCut(session, clock); // me: three

  const segments = events.filter((event) => event.type === 'segment');
  assert.deepEqual(segments.map((segment) => segment.seg_id), [1, 2, 3]);
  assert.deepEqual(
    segments.map((segment) => segment.speak),
    [true, false, true],
  );
  assert.deepEqual(
    translator.requests.map((request) => request.direction),
    [
      { src: 'zh', tgt: 'ja' },
      { src: 'ja', tgt: 'zh' },
      { src: 'zh', tgt: 'ja' },
    ],
  );
  assert.doesNotMatch(translator.requests[1].user, /原文：/, 'their context stays empty');
  assert.match(translator.requests[2].user, /原文：one/);
  assert.match(translator.requests[2].user, /译文：translated-one/);
  assert.doesNotMatch(translator.requests[2].user, /two/);

  assert.deepEqual(tts.calls, ['translated-one', 'translated-three']);
  assert.deepEqual(
    events.filter((event) => event.type === 'tts_end').map((event) => event.seg_id),
    [1, 3],
  );
});
