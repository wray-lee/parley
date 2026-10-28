import { test } from 'node:test';
import assert from 'node:assert/strict';

import { create } from '../../core/providers/translator-openai.js';
import { createProvider, loadProvider } from '../../core/providers/index.js';
import { createRegistry } from '../../core/packs-registry.js';
import { createSession } from '../../core/session.js';
import { DEFAULT_PRESET, PRESETS } from '../../config/presets.js';
import { loadConfig } from '../../hosts/node/config.js';

async function readAll(translator, request = { system: 's', user: 'u' }) {
  const chunks = [];
  for await (const chunk of translator.translate(request)) chunks.push(chunk);
  return chunks.join('');
}

test('presets carry the required data and a conservative free-tier flag', () => {
  assert.equal(DEFAULT_PRESET, 'groq');
  const required = [
    'groq',
    'gemini',
    'siliconflow',
    'dashscope',
    'openrouter',
    'ollama',
    'custom',
    'mistral',
    'deepseek',
    'zhipu',
    'moonshot',
    'cerebras',
  ];
  assert.deepEqual(Object.keys(PRESETS).sort(), [...required].sort());

  for (const [id, preset] of Object.entries(PRESETS)) {
    assert.equal(preset.id, id);
    assert.equal(typeof preset.trainsOnData, 'boolean', id);
    assert.equal(typeof preset.region, 'string', id);
    assert.match(preset.docs, /^https:\/\//, id);
    assert.equal(typeof preset.requiresKey, 'boolean', id);
    if (id === 'custom') continue;
    assert.match(preset.baseUrl, /^https?:\/\//, id);
    assert.ok(preset.model, id);
  }
  assert.equal(PRESETS.groq.trainsOnData, false);
  assert.equal(PRESETS.gemini.trainsOnData, true);
  assert.equal(PRESETS.ollama.requiresKey, false);
});

test('an unknown Preset fails with a clear message naming the known ones', () => {
  assert.throws(
    () => create({ preset: 'not-a-preset', apiKey: 'k' }),
    (error) => {
      assert.match(error.message, /unknown preset "not-a-preset"/);
      assert.match(error.message, /groq/);
      return true;
    },
  );
});

test('LLM_BASE_URL and LLM_MODEL override the Preset', async () => {
  const calls = [];
  const translator = create({
    preset: 'groq',
    apiKey: 'test-key',
    baseUrl: 'https://gateway.example/v1/',
    model: 'my-model',
    fetch: async (url, init) => {
      calls.push({ url, init });
      return new Response('data: {"choices":[{"delta":{"content":"ok"}}]}\n\ndata: [DONE]\n\n', {
        status: 200,
      });
    },
  });

  assert.equal(await readAll(translator), 'ok');
  assert.equal(calls[0].url, 'https://gateway.example/v1/chat/completions');
  const body = JSON.parse(calls[0].init.body);
  assert.equal(body.model, 'my-model');
});

test('a Preset without a key still resolves; the failure is an auth error at request time', async () => {
  const translator = create({ preset: 'groq' });
  assert.equal(translator.name, 'openai');
  assert.equal(translator.preset, 'groq');
  await assert.rejects(readAll(translator), (error) => {
    assert.equal(error.kind, 'auth');
    assert.match(error.message, /LLM_API_KEY/);
    assert.ok(error.fallback);
    return true;
  });
});

test('custom needs an explicit base URL and model', () => {
  assert.throws(() => create({ preset: 'custom' }), /LLM_BASE_URL/);
  assert.throws(
    () => create({ preset: 'custom', baseUrl: 'https://gateway.example/v1' }),
    /LLM_MODEL/,
  );
});

test('a base URL must not embed credentials', () => {
  assert.throws(
    () =>
      create({
        preset: 'custom',
        baseUrl: 'https://user:secret@gateway.example/v1',
        model: 'm',
      }),
    /must not embed credentials/,
  );
});

test('LLM_PRESET=groq without a key resolves Providers and keeps Demo Mode on', async () => {
  const config = loadConfig({ LLM_PRESET: 'groq' });
  assert.equal(config.translator, 'openai');
  assert.equal(config.asr, 'mock');
  assert.equal(config.tts, 'mock');

  const factory = await loadProvider('translator', config.translator);
  const translator = await factory({ ...config.llm, references: {} });
  assert.equal(translator.name, 'openai');

  const session = createSession({
    providers: {
      asr: await createProvider('asr', 'mock', { sample: {} }),
      translator,
      tts: await createProvider('tts', 'mock'),
    },
    clock: { now: () => 0 },
    registry: createRegistry(),
  });
  assert.equal(session.hello().demo, true);
});

test('the adapter never exposes the key on its public surface', () => {
  const translator = create({ preset: 'groq', apiKey: 'super-secret-key' });
  assert.doesNotMatch(JSON.stringify(translator), /super-secret-key/);
});
