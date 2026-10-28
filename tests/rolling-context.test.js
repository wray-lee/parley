// Seam 1: the Session keeps at most 8 Committed pairs per Direction. The 9th
// Commit must drop the oldest pair from the next translator request.

import { test } from 'node:test';
import assert from 'node:assert/strict';

import { createSession } from '../core/session.js';
import { createRegistry } from '../core/packs-registry.js';
import { makeClock, pcmFrame, scriptedAsr, scriptedTranslator, scriptedTts } from './helpers/fakes.js';

const COMMITS = 10;

test('the 9th Commit drops the oldest Rolling Context pair', async () => {
  const marks = Array.from({ length: COMMITS }, (_, index) => ({
    at: 20 + index * 380,
    events: [{ type: 'stable', text: `source-${index + 1}` }],
  }));
  const asr = scriptedAsr(marks);
  const translator = scriptedTranslator(
    Array.from({ length: COMMITS }, (_, index) => `target-${index + 1}`),
  );
  const clock = makeClock();
  const session = createSession({
    providers: { asr, translator, tts: scriptedTts() },
    clock,
    registry: createRegistry({
      promptFiles: [{ name: 'system_zh2ja.txt', text: 'SYSTEM' }],
      packFiles: [],
    }),
    emit: () => {},
    log: () => {},
  });

  session.handleMessage({ type: 'start', mode: 'listen', src: 'zh', tgt: 'ja' });
  session.handleMessage({ type: 'sample', name: 'placeholder' });

  for (let i = 0; i < COMMITS; i++) {
    session.handleAudio(pcmFrame(20, 0.2));
    clock.advance(20);
    for (let frame = 0; frame < 18; frame++) {
      session.handleAudio(pcmFrame(20, 0));
      clock.advance(20);
    }
    await session.idle();
  }

  assert.equal(translator.requests.length, COMMITS);

  const ninth = translator.requests[8].user;
  assert.equal((ninth.match(/原文：/g) ?? []).length, 8);
  assert.match(ninth, /原文：source-1\n译文：target-1/);

  const tenth = translator.requests[9].user;
  assert.equal((tenth.match(/原文：/g) ?? []).length, 8);
  assert.doesNotMatch(tenth, /source-1\n/);
  assert.match(tenth, /原文：source-2\n译文：target-2/);
  assert.match(tenth, /原文：source-9\n译文：target-9/);
});
