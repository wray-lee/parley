import { test } from 'node:test';
import assert from 'node:assert/strict';

import { createProvider, loadProvider } from '../core/providers/index.js';

const sample = {
  timed_transcript: [{ start_ms: 0, end_ms: 100, text: 'hello' }],
  reference_translations: { hello: 'bonjour' },
};

test('the loader resolves Providers by kind-name convention', async () => {
  const asr = await createProvider('asr', 'mock', { sample });
  assert.equal(asr.name, 'mock');
  assert.equal(typeof asr.push, 'function');

  const create = await loadProvider('translator', 'mock');
  const translator = await create({ references: {} });
  assert.equal(translator.name, 'mock');
});

test('the loader refuses names that are not plain identifiers', async () => {
  await assert.rejects(createProvider('asr', '../evil'), /unknown provider/);
  await assert.rejects(createProvider('asr', 'no-such-provider'), /unknown provider/);
});

test('mock ASR replays the timed transcript as Partial then Stable Prefix', async () => {
  const asr = await createProvider('asr', 'mock', { sample });
  asr.open({ src: 'zh', tgt: 'ja' });

  assert.deepEqual(asr.push({ audioMs: 20 }), [{ type: 'partial', text: 'hello' }]);
  assert.deepEqual(asr.push({ audioMs: 120 }), [{ type: 'stable', text: 'hello' }]);
  assert.deepEqual(asr.push({ audioMs: 140 }), []);
  assert.equal(asr.acceptsLive, false);
});

test('mock translator returns the reference translation with a DEMO marker', async () => {
  const translator = await createProvider('translator', 'mock', {
    references: sample.reference_translations,
  });
  const chunks = [];
  for await (const chunk of translator.translate({ text: 'hello' })) chunks.push(chunk);
  assert.equal(chunks.join(''), '[DEMO] bonjour');
});

test('mock TTS returns a short decodable tone', async () => {
  const tts = await createProvider('tts', 'mock');
  const { format, data } = await tts.synthesize({ text: 'x' });
  assert.equal(format, 'wav');
  assert.equal(String.fromCharCode(...data.slice(0, 4)), 'RIFF');
  assert.equal(String.fromCharCode(...data.slice(8, 12)), 'WAVE');
});
