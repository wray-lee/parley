import test from 'node:test';
import assert from 'node:assert/strict';

import { createSpeechGate } from './speech-gate.js';

test('the gate holds during speech and for the tail, then opens', () => {
  let now = 1000;
  let timer = null;
  let armed = 0;
  const gate = createSpeechGate({
    tailMs: 500,
    now: () => now,
    schedule: (fn) => {
      armed += 1;
      timer = fn;
      return armed;
    },
    cancel: () => {},
  });

  assert.equal(gate.blocked, false);
  gate.begin();
  assert.equal(gate.blocked, true);
  now = 1200;
  gate.end();
  assert.equal(gate.blocked, true, 'tail after the last utterance');

  now = 1800;
  assert.equal(gate.blocked, false, 'clock wins even if the timer is late');

  gate.begin();
  gate.end();
  timer();
  assert.equal(gate.blocked, false);
  assert.equal(armed, 2);
});

test('overlapping utterances keep the gate closed until the last ends', () => {
  let timer = null;
  const gate = createSpeechGate({
    tailMs: 500,
    now: () => 1000,
    schedule: (fn) => {
      timer = fn;
      return 1;
    },
    cancel: () => {},
  });
  gate.begin();
  gate.begin();
  gate.end();
  assert.equal(gate.blocked, true);
  gate.end();
  assert.equal(gate.blocked, true);
  timer();
  assert.equal(gate.blocked, false);
});

test('reset opens the gate immediately', () => {
  const gate = createSpeechGate({ schedule: () => 1, cancel: () => {} });
  gate.begin();
  gate.reset();
  assert.equal(gate.blocked, false);
});
