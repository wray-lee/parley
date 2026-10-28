import { test } from 'node:test';
import assert from 'node:assert/strict';

import { loadConfig } from '../hosts/node/config.js';

test('no environment means every Provider is mock (Demo Mode)', () => {
  const config = loadConfig({});
  assert.equal(config.asr, 'mock');
  assert.equal(config.translator, 'mock');
  assert.equal(config.tts, 'mock');
  assert.equal(config.port, 3000);
});

test('LLM_EXTRA_BODY reaches the adapter untouched for validation', () => {
  const config = loadConfig({ LLM_API_KEY: 'k', LLM_EXTRA_BODY: '{"top_k":20}' });
  assert.equal(config.llm.extraBody, '{"top_k":20}');
});

test('environment variables select Providers and the port', () => {
  const config = loadConfig({
    ASR_PROVIDER: 'deepgram',
    TTS_PROVIDER: 'kokoro',
    LLM_API_KEY: 'k',
    PORT: '8080',
  });
  assert.equal(config.asr, 'deepgram');
  assert.equal(config.tts, 'kokoro');
  assert.equal(config.translator, 'openai');
  assert.equal(config.port, 8080);
});
