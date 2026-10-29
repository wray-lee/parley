import test from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';

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

// Shared with the Dart client and the Go core: the cases are data, the error
// code -> message mapping is each implementation's own.
const VALIDATION = JSON.parse(
  readFileSync(new URL('../spec/fixtures/settings-validation.json', import.meta.url), 'utf8'),
);

const ERROR_PATTERNS = {
  needsBaseUrl: /Base URL/,
  invalidUrl: /full http/,
  notHttp: /start with http/,
  plainHttp: /https/,
  credentials: /credentials/,
  query: /query/,
  needsModel: /Model/,
  invalidJson: /valid JSON/,
  notObject: /JSON object/,
};

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
  for (const entry of VALIDATION.baseUrl) {
    const label = JSON.stringify(entry);
    if (entry.error) {
      assert.throws(
        () => normalizeBaseUrl(entry.value, entry.preset),
        ERROR_PATTERNS[entry.error],
        label,
      );
    } else {
      assert.equal(normalizeBaseUrl(entry.value, entry.preset), entry.expect, label);
    }
  }
});

test('model is optional except for custom', () => {
  for (const entry of VALIDATION.model) {
    const label = JSON.stringify(entry);
    if (entry.error) {
      assert.throws(
        () => normalizeModel(entry.value, entry.preset),
        ERROR_PATTERNS[entry.error],
        label,
      );
    } else {
      assert.equal(normalizeModel(entry.value, entry.preset), entry.expect, label);
    }
  }
});

test('extra request fields must be a JSON object', () => {
  for (const entry of VALIDATION.extraBody) {
    const label = JSON.stringify(entry);
    if (entry.error) {
      assert.throws(() => normalizeExtraBody(entry.value), ERROR_PATTERNS[entry.error], label);
    } else {
      assert.equal(normalizeExtraBody(entry.value), entry.expect, label);
    }
  }
});

test('only LAN targets require Android local-network access', () => {
  const { base, cases } = VALIDATION.localNetwork;
  for (const entry of cases) {
    assert.equal(
      needsLocalNetworkPermission({ ...base, ...entry.set }),
      entry.expect,
      JSON.stringify(entry),
    );
  }
});
