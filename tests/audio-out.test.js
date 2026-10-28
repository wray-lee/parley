import { test } from 'node:test';
import assert from 'node:assert/strict';

import { createAudioOut } from '../web/audio-out.js';

const flush = () => new Promise((resolve) => setTimeout(resolve, 0));

function fakePlatform() {
  const instances = [];
  const blobs = [];
  class FakeAudio {
    constructor(url) {
      this.url = url;
      this.listeners = new Map();
      this.paused = false;
      instances.push(this);
    }
    addEventListener(type, fn) {
      this.listeners.set(type, fn);
    }
    play() {
      return Promise.resolve();
    }
    pause() {
      this.paused = true;
    }
    finish() {
      this.listeners.get('ended')?.();
    }
  }
  const urls = {
    createObjectURL(blob) {
      blobs.push(blob);
      return `blob:${blobs.length - 1}`;
    },
    revokeObjectURL() {},
  };
  const spoken = [];
  class FakeUtterance {
    constructor(text) {
      this.text = text;
      this.lang = '';
    }
  }
  const speech = {
    speak(utterance) {
      spoken.push(utterance);
      queueMicrotask(() => utterance.onend?.());
    },
    cancel() {},
  };
  const audioOut = createAudioOut({
    Audio: FakeAudio,
    URL: urls,
    speechSynthesis: speech,
    SpeechSynthesisUtterance: FakeUtterance,
    controls: false,
  });
  return { audioOut, instances, blobs, spoken };
}

const bytes = (value) => new Uint8Array([value, value, value]);

test('audio chunks play strictly in Segment order even when a later Segment arrives first', async () => {
  const { audioOut, instances, blobs } = fakePlatform();

  audioOut.begin(1, 'mp3');
  audioOut.begin(2, 'mp3');
  const second = audioOut.play('mp3', bytes(2), 2);
  await flush();
  assert.equal(instances.length, 0, 'Segment 2 must wait for Segment 1');

  const first = audioOut.play('mp3', bytes(1), 1);
  await flush();
  assert.equal(instances.length, 1);
  assert.equal(blobs[0].type, 'audio/mpeg');

  audioOut.end(1);
  instances[0].finish();
  await flush();
  assert.equal(instances.length, 2, 'Segment 2 starts after Segment 1 ends');

  audioOut.end(2);
  instances[1].finish();
  await Promise.all([first, second]);
});

test('playback does not wait for later Segments to be ready', async () => {
  const { audioOut, instances } = fakePlatform();

  audioOut.begin(1, 'mp3');
  audioOut.begin(2, 'mp3');
  const first = audioOut.play('mp3', bytes(1), 1);
  await flush();
  assert.equal(instances.length, 1, 'Segment 1 starts before Segment 2 has audio');

  audioOut.end(1);
  instances[0].finish();
  await first;
});

test('stop clears the queue and pauses the current chunk', async () => {
  const { audioOut, instances } = fakePlatform();

  audioOut.begin(1, 'mp3');
  audioOut.play('mp3', bytes(1), 1);
  await flush();
  audioOut.stop();

  assert.equal(instances[0].paused, true);
  audioOut.begin(1, 'mp3');
  const after = audioOut.play('mp3', bytes(9), 1);
  await flush();
  assert.equal(instances.length, 2);
  audioOut.end(1);
  instances[1].finish();
  await after;
});

test('system mode speaks the text with the target language', async () => {
  const { audioOut, spoken } = fakePlatform();
  await audioOut.speak('hello there', 'ja');
  assert.equal(spoken.length, 1);
  assert.equal(spoken[0].text, 'hello there');
  assert.equal(spoken[0].lang, 'ja');
});
