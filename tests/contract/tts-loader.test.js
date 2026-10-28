// Every new TTS adapter is reachable through the convention loader, so
// TTS_PROVIDER selects it without a registry edit.

import { test } from 'node:test';
import assert from 'node:assert/strict';

import { createProvider, loadProvider } from '../../core/providers/index.js';

test('TTS_PROVIDER names resolve to the four new adapters by convention', async () => {
  const system = await createProvider('tts', 'system');
  assert.equal(system.name, 'system');

  const edge = await createProvider('tts', 'edge');
  assert.equal(edge.name, 'edge');
  assert.equal(edge.unofficial, true);

  await assert.rejects(
    createProvider('tts', 'azure'),
    /AZURE_SPEECH_KEY/,
    'azure loads and asks for its env key',
  );

  await assert.rejects(
    createProvider('tts', 'cosyvoice'),
    /not implemented in v1/,
    'cosyvoice loads and fails clearly',
  );

  assert.equal(typeof (await loadProvider('tts', 'mock')), 'function');
});
