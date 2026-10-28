import test from 'node:test';
import assert from 'node:assert/strict';

import {
  DEFAULT_SETTINGS,
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
