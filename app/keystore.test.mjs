import test from 'node:test';
import assert from 'node:assert/strict';

import { createKeyStore, loadKeyStore } from './keystore.js';

test('plugin keystore round-trips and removes', async () => {
  const values = new Map();
  const store = createKeyStore({
    setItem: async (key, value) => values.set(key, value),
    getItem: async (key) => values.get(key) ?? null,
    deleteItem: async (key) => values.delete(key),
    warn: () => {},
  });
  assert.equal(store.persistent, true);
  await store.set('llm.apiKey', 'secret');
  assert.equal(values.get('llm.apiKey'), 'secret');
  assert.equal(await store.get('llm.apiKey'), 'secret');
  assert.equal(await store.get('missing'), null);
  await store.remove('llm.apiKey');
  assert.equal(await store.get('llm.apiKey'), null);
});

test('a failing beta plugin falls back to memory without losing the session', async () => {
  const warnings = [];
  const store = createKeyStore({
    getItem: async () => {
      throw new Error('keystore not ready');
    },
    setItem: async () => {
      throw new Error('keystore not ready');
    },
    deleteItem: async () => {},
    warn: (line) => warnings.push(line),
  });
  assert.equal(await store.get('llm.apiKey'), null);
  assert.equal(store.persistent, false);
  await store.set('llm.apiKey', 'typed key');
  assert.equal(await store.get('llm.apiKey'), 'typed key');
  assert.equal(warnings.length, 1);
  assert.match(warnings[0], /memory/);
});

test('outside Tauri the store is memory-only', async () => {
  const store = await loadKeyStore();
  assert.equal(store.persistent, false);
  await store.set('a', 'b');
  assert.equal(await store.get('a'), 'b');
});
