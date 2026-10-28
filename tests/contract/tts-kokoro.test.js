// Seam 2: replay recorded Kokoro-FastAPI wire traffic through the adapter.
// The fixture is a real mp3 response body; no keys or host names are stored.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';

import { create, splitSentences } from '../../core/providers/tts-kokoro.js';

const fixture = (name) =>
  new Uint8Array(readFileSync(new URL(`./fixtures/kokoro/${name}`, import.meta.url)));

function responseFrom(bytes, { status = 200, body = null, chunkSize = 97 } = {}) {
  if (body !== null) return new Response(body, { status });
  return new Response(
    new ReadableStream({
      start(controller) {
        for (let i = 0; i < bytes.length; i += chunkSize) {
          controller.enqueue(bytes.subarray(i, i + chunkSize));
        }
        controller.close();
      },
    }),
    { status, headers: { 'content-type': 'audio/mpeg' } },
  );
}

function recorder(handler) {
  const requests = [];
  return {
    requests,
    fetch: async (url, init) => {
      requests.push({ url, body: JSON.parse(init.body) });
      return handler(requests.at(-1), requests.length);
    },
  };
}

test('a recorded chunked Kokoro response is reassembled into one mp3 chunk', async () => {
  const expected = fixture('ja.mp3');
  const { requests, fetch } = recorder(() => responseFrom(expected));
  const tts = create({
    kokoroUrl: 'http://kokoro.test:8880',
    voices: { ja: ['jf_alpha'] },
    fetch,
    warn: () => {},
  });

  const audio = await tts.synthesize({
    text: 'こんにちは。',
    seg_id: 1,
    direction: { src: 'en', tgt: 'ja' },
  });

  assert.equal(audio.format, 'mp3');
  assert.deepEqual(audio.data, expected);
  assert.equal(requests.length, 1);
  assert.equal(requests[0].url, 'http://kokoro.test:8880/v1/audio/speech');
  assert.deepEqual(requests[0].body, {
    model: 'kokoro',
    input: 'こんにちは。',
    voice: 'jf_alpha',
    response_format: 'mp3',
    stream: true,
  });
});

test('sentence splitting uses the Unicode Sentence_Terminal property', () => {
  assert.deepEqual(splitSentences('你好。你好吗？好的！'), ['你好。', '你好吗？', '好的！']);
  assert.deepEqual(splitSentences('こんにちは。元気ですか？'), ['こんにちは。', '元気ですか？']);
  assert.deepEqual(splitSentences('no terminal here'), ['no terminal here']);
  assert.deepEqual(splitSentences('  spaced out.  '), ['spaced out.']);
  assert.deepEqual(splitSentences(''), []);
});

test('a long translation starts TTS at the first sentence terminal', async () => {
  const clip = fixture('zh.mp3');
  const { requests, fetch } = recorder(() => responseFrom(clip));
  const tts = create({
    kokoroUrl: 'http://kokoro.test:8880',
    voices: { zh: ['zf_xiaobei'] },
    fetch,
    warn: () => {},
  });

  const audio = await tts.synthesize({
    text: '第一句。第二句！',
    seg_id: 3,
    direction: { src: 'en', tgt: 'zh' },
  });

  assert.deepEqual(requests.map((r) => r.body.input), ['第一句。', '第二句！']);
  const tagLength =
    10 +
    (((clip[6] & 0x7f) << 21) |
      ((clip[7] & 0x7f) << 14) |
      ((clip[8] & 0x7f) << 7) |
      (clip[9] & 0x7f));
  assert.equal(audio.data.length, clip.length * 2 - tagLength);
  assert.deepEqual(audio.data.subarray(0, clip.length), clip);
  assert.deepEqual(audio.data.subarray(clip.length), clip.subarray(tagLength));
});

test('a voice missing from the server falls back to another voice of that language', async () => {
  const clip = fixture('ja.mp3');
  const warnings = [];
  const { requests, fetch } = recorder((request) =>
    request.body.voice === 'jf_missing'
      ? responseFrom(null, { status: 400, body: 'voice not found' })
      : responseFrom(clip),
  );
  const tts = create({
    kokoroUrl: 'http://kokoro.test:8880',
    voices: { ja: ['jf_missing', 'jf_alpha'] },
    fetch,
    warn: (line) => warnings.push(line),
  });

  const audio = await tts.synthesize({
    text: 'こんにちは。',
    direction: { src: 'en', tgt: 'ja' },
  });

  assert.deepEqual(audio.data, clip);
  assert.deepEqual(requests.map((r) => r.body.voice), ['jf_missing', 'jf_alpha']);
  assert.equal(warnings.length, 1);
  assert.match(warnings[0], /jf_missing/);
  assert.match(warnings[0], /jf_alpha/);
});

test('an unreachable Kokoro names itself and suggests system or mock', async () => {
  const tts = create({
    kokoroUrl: 'http://127.0.0.1:8880',
    voices: { ja: ['jf_alpha'] },
    fetch: async () => {
      throw new Error('connect ECONNREFUSED 127.0.0.1:8880');
    },
    warn: () => {},
  });

  await assert.rejects(
    tts.synthesize({ text: 'こんにちは。', direction: { src: 'en', tgt: 'ja' } }),
    (error) => {
      assert.match(error.message, /Kokoro/);
      assert.match(error.message, /TTS_PROVIDER=system or mock/);
      return true;
    },
  );
});

test('the bundled voice map is data keyed by target language', () => {
  const voices = JSON.parse(
    readFileSync(new URL('../../config/voices.json', import.meta.url), 'utf8'),
  );
  assert.ok(voices.kokoro.ja.length >= 2, 'expected preinstalled ja voices');
  assert.ok(voices.kokoro.zh.length >= 2, 'expected preinstalled zh voices');
  for (const list of Object.values(voices.kokoro)) {
    for (const voice of list) assert.match(voice, /^[a-z]{2}_[a-z]+$/);
  }
});
