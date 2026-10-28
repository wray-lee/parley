// Seam 2: the CosyVoice stub must fail loudly instead of pretending to speak.

import { test } from 'node:test';
import assert from 'node:assert/strict';

import { create } from '../../core/providers/tts-cosyvoice.js';

test('the CosyVoice stub names the not-implemented status and the alternatives', () => {
  assert.throws(
    () => create({}),
    /CosyVoice TTS is not implemented in v1.*kokoro.*edge.*azure.*system.*mock/,
  );
});
