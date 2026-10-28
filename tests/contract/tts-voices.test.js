// config/voices.json is data, not code: every voice of a language must belong
// to that language, so adding a language never silently speaks the wrong one.

import { test } from 'node:test';
import assert from 'node:assert/strict';

import voices from '../../config/voices.json' with { type: 'json' };

test('every Provider voice map is coherent per language', () => {
  const languages = Object.keys(voices.kokoro).sort();
  assert.deepEqual(languages, ['ja', 'zh']);

  for (const provider of ['azure', 'edge']) {
    assert.ok(voices[provider], `missing ${provider} voices`);
    assert.deepEqual(Object.keys(voices[provider]).sort(), languages);
    for (const [language, list] of Object.entries(voices[provider])) {
      assert.ok(Array.isArray(list) && list.length > 0, `${provider}.${language} is empty`);
      assert.equal(new Set(list).size, list.length, `${provider}.${language} has duplicates`);
      for (const voice of list) {
        assert.match(voice, new RegExp(`^${language}-`), `${voice} is not a ${language} voice`);
      }
    }
  }
});
