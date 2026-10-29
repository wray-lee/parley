// Seam 2: replay an Azure Speech REST response through the adapter. The
// request carries the key and region that loadConfig reads from
// AZURE_SPEECH_KEY / AZURE_SPEECH_REGION. The response body is opaque audio;
// the fixture is a real 24 kHz mono mp3 because no Azure key was available to
// record one (the live check is PENDING-USER).

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';

import { create } from '../../core/providers/tts-azure.js';
import { loadConfig } from '../../hosts/node/config.js';

const fixture = () =>
  new Uint8Array(readFileSync(new URL('../../spec/fixtures/azure/ja.mp3', import.meta.url)));

function recorder(handler) {
  const requests = [];
  return {
    requests,
    fetch: async (url, init) => {
      requests.push({ url, init });
      return handler(requests.at(-1));
    },
  };
}

const config = loadConfig({
  TTS_PROVIDER: 'azure',
  AZURE_SPEECH_KEY: 'test-key',
  AZURE_SPEECH_REGION: 'japaneast',
});

test('env key and region produce the documented Azure REST request', async () => {
  const expected = fixture();
  const { requests, fetch } = recorder(
    () => new Response(expected, { status: 200, headers: { 'content-type': 'audio/mpeg' } }),
  );
  const tts = create({ ...config.ttsOptions, fetch });

  const audio = await tts.synthesize({
    text: '天気 & <ok>',
    seg_id: 1,
    direction: { src: 'zh', tgt: 'ja' },
  });

  assert.equal(tts.name, 'azure');
  assert.equal(audio.format, 'mp3');
  assert.deepEqual(audio.data, expected);

  const { url, init } = requests[0];
  assert.equal(url, 'https://japaneast.tts.speech.microsoft.com/cognitiveservices/v1');
  assert.equal(init.method, 'POST');
  assert.equal(init.headers['Ocp-Apim-Subscription-Key'], 'test-key');
  assert.equal(init.headers['Content-Type'], 'application/ssml+xml');
  assert.equal(init.headers['X-Microsoft-OutputFormat'], 'audio-24khz-48kbitrate-mono-mp3');
  assert.match(init.body, /<voice name='ja-JP-NanamiNeural'>/);
  assert.match(init.body, /xml:lang='ja-JP'/);
  assert.match(init.body, /天気 &amp; &lt;ok&gt;/);
});

test('a rejected key says which env variables to check and the alternatives', async () => {
  const { fetch } = recorder(() => new Response('bad key', { status: 401 }));
  const tts = create({ ...config.ttsOptions, fetch });
  await assert.rejects(
    tts.synthesize({ text: 'x', direction: { src: 'zh', tgt: 'ja' } }),
    /HTTP 401.*AZURE_SPEECH_KEY and AZURE_SPEECH_REGION.*system or mock/,
  );
});

test('a rate limit names the fallback', async () => {
  const { fetch } = recorder(() => new Response('', { status: 429 }));
  const tts = create({ ...config.ttsOptions, fetch });
  await assert.rejects(
    tts.synthesize({ text: 'x', direction: { src: 'zh', tgt: 'ja' } }),
    /rate limited.*system or mock/,
  );
});

test('an unreachable endpoint names Azure and the alternatives', async () => {
  const tts = create({
    ...config.ttsOptions,
    fetch: async () => {
      throw new Error('ECONNREFUSED');
    },
  });
  await assert.rejects(
    tts.synthesize({ text: 'x', direction: { src: 'zh', tgt: 'ja' } }),
    /Azure TTS is unreachable.*ECONNREFUSED.*system or mock/,
  );
});

test('a missing key or region fails at create with exact variable names', () => {
  assert.throws(() => create({ azureRegion: 'japaneast' }), /AZURE_SPEECH_KEY/);
  assert.throws(
    () => create({ azureKey: 'k', azureRegion: 'not a region' }),
    /AZURE_SPEECH_REGION/,
  );
});

test('a missing voice falls back to the base language and warns', async () => {
  const warnings = [];
  const { fetch } = recorder(
    () => new Response(fixture(), { status: 200, headers: { 'content-type': 'audio/mpeg' } }),
  );
  const tts = create({
    ...config.ttsOptions,
    fetch,
    voices: { ja: ['ja-JP-NanamiNeural'] },
    warn: (line) => warnings.push(line),
  });

  await tts.synthesize({ text: 'x', direction: { src: 'zh', tgt: 'ja-JP' } });
  assert.equal(warnings.length, 1);
  assert.match(warnings[0], /no voice configured for "ja-JP"/);
});

test('a language with no configured voice fails with the alternatives', async () => {
  const tts = create({ ...config.ttsOptions, fetch: async () => new Response('') });
  await assert.rejects(
    tts.synthesize({ text: 'x', direction: { src: 'ja', tgt: 'fr' } }),
    /no voice configured for target language "fr".*system or mock/,
  );
});
