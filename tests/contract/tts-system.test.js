// Seam 2 contract plus the Seam 1 system-mode guarantee: text-only TTS emits
// no tts_* frames, and the client's speechSynthesis path speaks Segments in
// order. The Server Host sends `segment {speak:true}` and the browser speaks it.

import { test } from 'node:test';
import assert from 'node:assert/strict';

import { create } from '../../core/providers/tts-system.js';
import { createSession } from '../../core/session.js';
import { createRegistry } from '../../core/packs-registry.js';
import { createAudioOut } from '../../web/audio-out.js';
import { makeClock, pcmFrame, scriptedAsr, scriptedTranslator } from '../helpers/fakes.js';

test('system TTS returns text, never audio', async () => {
  const tts = create();
  assert.equal(tts.name, 'system');
  const result = await tts.synthesize({
    text: 'こんにちは。',
    seg_id: 1,
    direction: { src: 'zh', tgt: 'ja' },
  });
  assert.equal(result.format, 'text');
  assert.equal(result.data, null);
});

function registry() {
  return createRegistry({
    promptFiles: [
      { name: 'system_zh2ja.txt', text: 'SYSTEM {{GLOSSARY}}' },
      { name: 'system_ja2zh.txt', text: 'SYSTEM {{GLOSSARY}}' },
    ],
    packFiles: ['conference_zh2ja.tsv', 'conference_ja2zh.tsv'],
  });
}

function fakeSpeech() {
  const spoken = [];
  class FakeUtterance {
    constructor(text) {
      this.text = text;
      this.lang = '';
    }
  }
  return {
    spoken,
    speechSynthesis: {
      speak(utterance) {
        spoken.push(utterance.text);
        queueMicrotask(() => utterance.onend?.());
      },
      cancel() {},
    },
    SpeechSynthesisUtterance: FakeUtterance,
  };
}

test('a system Session emits no tts_begin and the client speaks Segments in order', async () => {
  const clock = makeClock();
  const asr = scriptedAsr([
    { at: 20, events: [{ type: 'stable', text: 'one' }] },
    { at: 400, events: [{ type: 'stable', text: 'two' }] },
  ]);
  const translator = scriptedTranslator(['translated-one', 'translated-two']);
  const events = [];
  const records = [];
  const session = createSession({
    providers: { asr, translator, tts: create() },
    clock,
    registry: registry(),
    emit: (event) => events.push(event),
    log: (record) => records.push(record),
  });

  session.handleMessage({ type: 'start', mode: 'listen', src: 'zh', tgt: 'ja' });
  session.handleMessage({ type: 'sample', name: 'placeholder' });
  for (let segment = 0; segment < 2; segment++) {
    session.handleAudio(pcmFrame(20, 0.2));
    clock.advance(20);
    for (let i = 0; i < 18; i++) {
      session.handleAudio(pcmFrame(20, 0));
      clock.advance(20);
    }
  }
  await session.idle();

  const ttsEvents = events.filter((event) => event.type.startsWith('tts_'));
  assert.deepEqual(ttsEvents, [], 'system TTS must emit no tts_begin/chunk/end');

  const segments = events.filter((event) => event.type === 'segment');
  assert.deepEqual(
    segments.map((segment) => [segment.seg_id, segment.tgt, segment.speak]),
    [
      [1, 'translated-one', true],
      [2, 'translated-two', true],
    ],
  );
  assert.equal(records.length, 2);
  assert.equal(records[0].tts_first_chunk, null, 'no server-side first chunk');

  const { spoken, speechSynthesis, SpeechSynthesisUtterance } = fakeSpeech();
  const audioOut = createAudioOut({
    Audio: null,
    URL: null,
    speechSynthesis,
    SpeechSynthesisUtterance,
    controls: false,
  });
  for (const segment of segments) await audioOut.speak(segment.tgt, segment.direction.tgt);
  assert.deepEqual(spoken, ['translated-one', 'translated-two']);
});
