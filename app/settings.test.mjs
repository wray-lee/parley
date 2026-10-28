import test from 'node:test';
import assert from 'node:assert/strict';

import {
  DEFAULT_SETTINGS,
  needsLocalNetworkPermission,
  normalizeBaseUrl,
  normalizeExtraBody,
  normalizeModel,
  readSettings,
  SETTINGS_KEY,
  writeSettings,
} from './settings.js';

function memoryStorage() {
  const data = new Map();
  return {
    data,
    getItem: (key) => data.get(key) ?? null,
    setItem: (key, value) => data.set(key, String(value)),
  };
}

test('only non-secret choices are written to WebView storage', () => {
  const storage = memoryStorage();
  writeSettings(
    {
      ...DEFAULT_SETTINGS,
      mode: 'server',
      serverUrl: 'https://parley.example.com',
      llmKey: 'very-secret-value',
      serverToken: 'access-token-very-secret',
      whisper: 'not-a-setting',
    },
    storage,
  );
  const raw = storage.data.get(SETTINGS_KEY);
  assert.ok(raw.includes('parley.example.com'));
  assert.equal(raw.includes('very-secret-value'), false);
  assert.equal(raw.includes('access-token-very-secret'), false);
  assert.equal(raw.includes('not-a-setting'), false);
  assert.deepEqual(Object.keys(JSON.parse(raw)).sort(), Object.keys(DEFAULT_SETTINGS).sort());
});

test('settings round-trip and unknown modes fall back to device', () => {
  const storage = memoryStorage();
  assert.deepEqual(readSettings(storage), { ...DEFAULT_SETTINGS });
  writeSettings({ ...DEFAULT_SETTINGS, translatorPreset: 'gemini', mode: 'nonsense' }, storage);
  const settings = readSettings(storage);
  assert.equal(settings.translatorPreset, 'gemini');
  assert.equal(settings.mode, 'device');
});

test('corrupt storage yields defaults instead of throwing', () => {
  const storage = memoryStorage();
  storage.data.set(SETTINGS_KEY, '{not json');
  assert.deepEqual(readSettings(storage), { ...DEFAULT_SETTINGS });
});

test('custom translator fields round-trip without leaking the key', () => {
  const storage = memoryStorage();
  writeSettings(
    {
      ...DEFAULT_SETTINGS,
      translatorPreset: 'custom',
      translatorBaseUrl: 'https://gateway.example.com/v1',
      translatorModel: 'my-model',
      translatorExtraBody: '{"top_k":20}',
      llmKey: 'very-secret-value',
    },
    storage,
  );
  const settings = readSettings(storage);
  assert.equal(settings.translatorPreset, 'custom');
  assert.equal(settings.translatorBaseUrl, 'https://gateway.example.com/v1');
  assert.equal(settings.translatorModel, 'my-model');
  assert.equal(settings.translatorExtraBody, '{"top_k":20}');
  assert.equal(storage.data.get(SETTINGS_KEY).includes('very-secret-value'), false);
});

test('base URL is trimmed, validated and required for custom', () => {
  assert.equal(
    normalizeBaseUrl(' https://gateway.example.com/v1/ ', 'groq'),
    'https://gateway.example.com/v1',
  );
  assert.equal(normalizeBaseUrl('', 'groq'), '');
  assert.throws(() => normalizeBaseUrl('', 'custom'), /Base URL/);
  assert.throws(() => normalizeBaseUrl('gateway.example.com/v1', 'custom'), /http/);
  assert.throws(() => normalizeBaseUrl('ftp://gateway.example.com/v1', 'custom'), /http/);
  assert.throws(() => normalizeBaseUrl('http://example.com/v1', 'custom'), /https/);
  assert.throws(
    () => normalizeBaseUrl('https://user:pass@gateway.example.com/v1', 'custom'),
    /credentials/,
  );
  assert.throws(
    () => normalizeBaseUrl('https://gateway.example.com/v1?key=1', 'custom'),
    /query/,
  );
  assert.throws(
    () => normalizeBaseUrl('https://gateway.example.com/v1#', 'custom'),
    /query/,
  );
  assert.equal(
    normalizeBaseUrl('http://localhost:11434/v1', 'custom'),
    'http://localhost:11434/v1',
  );
  assert.equal(
    normalizeBaseUrl('http://192.168.1.10:11434/v1', 'custom'),
    'http://192.168.1.10:11434/v1',
  );
  assert.equal(
    normalizeBaseUrl('http://172.31.5.4:1234/v1', 'custom'),
    'http://172.31.5.4:1234/v1',
  );
  assert.throws(() => normalizeBaseUrl('http://172.32.5.4:1234/v1', 'custom'), /https/);
  assert.throws(() => normalizeBaseUrl('http://192.168.1.10.evil.com/v1', 'custom'), /https/);
});

test('model is optional except for custom', () => {
  assert.equal(normalizeModel('  my-model  ', 'groq'), 'my-model');
  assert.equal(normalizeModel('', 'groq'), '');
  assert.throws(() => normalizeModel('', 'custom'), /Model/);
});

test('extra request fields must be a JSON object', () => {
  assert.equal(normalizeExtraBody(''), '');
  assert.equal(normalizeExtraBody(' {"top_k":20} '), '{"top_k":20}');
  assert.throws(() => normalizeExtraBody('{not json'), /JSON/);
  for (const value of ['[]', 'null', '"x"', '42']) {
    assert.throws(() => normalizeExtraBody(value), /object/);
  }
});

test('only LAN targets require Android local-network access', () => {
  const device = { ...DEFAULT_SETTINGS, translatorBaseUrl: 'https://api.groq.com/openai/v1' };
  assert.equal(needsLocalNetworkPermission(device), false);
  assert.equal(
    needsLocalNetworkPermission({ ...device, translatorBaseUrl: 'http://192.168.1.10:11434/v1' }),
    true,
  );
  assert.equal(
    needsLocalNetworkPermission({ ...device, translatorBaseUrl: 'https://[fd00::1]/v1' }),
    true,
  );
  assert.equal(
    needsLocalNetworkPermission({ ...device, translatorBaseUrl: 'https://parley.local/v1' }),
    true,
  );
  assert.equal(
    needsLocalNetworkPermission({ ...device, translatorBaseUrl: 'https://parley.home.arpa/v1' }),
    true,
  );
  assert.equal(
    needsLocalNetworkPermission({ ...device, translatorBaseUrl: 'http://localhost:11434/v1' }),
    false,
  );
  assert.equal(
    needsLocalNetworkPermission({ ...device, translatorBaseUrl: 'http://127.0.0.1:11434/v1' }),
    false,
  );
  assert.equal(
    needsLocalNetworkPermission({
      ...device,
      ttsProvider: 'kokoro',
      kokoroUrl: 'http://192.168.1.10:8880',
    }),
    true,
  );
  assert.equal(
    needsLocalNetworkPermission({
      ...device,
      ttsProvider: 'system',
      kokoroUrl: 'http://192.168.1.10:8880',
    }),
    false,
  );
  assert.equal(
    needsLocalNetworkPermission({ ...device, mode: 'server', serverUrl: 'ws://10.0.0.5:8787' }),
    true,
  );
  assert.equal(
    needsLocalNetworkPermission({
      ...device,
      mode: 'server',
      serverUrl: 'wss://parley.example.com',
    }),
    false,
  );
});
