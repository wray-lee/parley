import test from 'node:test';
import assert from 'node:assert/strict';

import { createTauriTts, createWebSpeechTts, estimateSpeechMs } from './tts-speaker.js';

const tick = () => new Promise((resolve) => setImmediate(resolve));

test('plugin TTS resolves on the matching speech:finish event', async () => {
  const handlers = {};
  const calls = [];
  const tts = createTauriTts(
    {
      speak: async (options) => {
        calls.push(options);
        return { utteranceId: 'u1' };
      },
      stop: () => {},
      onSpeechEvent: (name, handler) => {
        handlers[name] = handler;
        return Promise.resolve(() => {});
      },
    },
    { warn: () => {}, estimateMs: () => 10_000 },
  );
  const speaking = tts.speak('こんにちは', 'ja');
  await tick();
  assert.deepEqual(calls[0], { text: 'こんにちは', language: 'ja', queueMode: 'add' });
  handlers['speech:finish']({ id: 'u1' });
  await speaking;
});

test('a finish event that beats speak() still releases the gate', async () => {
  const handlers = {};
  let releaseSpeak;
  const tts = createTauriTts(
    {
      speak: () =>
        new Promise((resolve) => {
          releaseSpeak = () => resolve({ utteranceId: 'u9' });
        }),
      stop: () => {},
      onSpeechEvent: (name, handler) => {
        handlers[name] = handler;
        return Promise.resolve(() => {});
      },
    },
    { warn: () => {}, estimateMs: () => 50 },
  );
  const speaking = tts.speak('hi', 'en');
  await tick();
  handlers['speech:finish']({ id: 'u9' });
  releaseSpeak();
  await speaking;
});

test('plugin stop settles a pending utterance and calls the engine', async () => {
  const handlers = {};
  let stopped = 0;
  const tts = createTauriTts(
    {
      speak: async () => ({ utteranceId: 'u2' }),
      stop: () => {
        stopped += 1;
      },
      onSpeechEvent: (name, handler) => {
        handlers[name] = handler;
        return Promise.resolve(() => {});
      },
    },
    { warn: () => {}, estimateMs: () => 10_000 },
  );
  const speaking = tts.speak('hello', 'en');
  await tick();
  tts.stop();
  await speaking;
  assert.equal(stopped, 1);
});

test('Web Speech fallback speaks with the target language and stops', async () => {
  const spoken = [];
  const synthesis = {
    cancelled: false,
    speak(utterance) {
      spoken.push(utterance);
      utterance.onend();
    },
    cancel() {
      this.cancelled = true;
    },
  };
  class Utterance {
    constructor(text) {
      this.text = text;
    }
  }
  const tts = createWebSpeechTts({ synthesis, Utterance, estimateMs: () => 1000 });
  await tts.speak('hello', 'ja');
  assert.equal(spoken[0].text, 'hello');
  assert.equal(spoken[0].lang, 'ja');
  tts.stop();
  assert.equal(synthesis.cancelled, true);
});

test('duration estimate grows with the text and is capped', () => {
  assert.ok(estimateSpeechMs('hello') > 1000);
  assert.ok(estimateSpeechMs('x'.repeat(1000)) <= 30_000);
});
