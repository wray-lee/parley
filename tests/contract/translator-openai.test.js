import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import path from 'node:path';

import { create } from '../../core/providers/translator-openai.js';
import { buildTranslatorRequest } from '../../core/translator-request.js';

const FIXTURES = path.join(import.meta.dirname, '..', '..', 'spec', 'fixtures', 'translator');
const NAMES = ['groq', 'siliconflow', 'ollama'];

async function loadFixture(name) {
  const raw = await readFile(path.join(FIXTURES, `${name}.json`), 'utf8');
  return { raw, fixture: JSON.parse(raw) };
}

function responseFor({ response }) {
  const bytes = new TextEncoder().encode(response.sse);
  const cuts = [];
  const firstMultibyte = bytes.findIndex((byte) => byte >= 0x80);
  if (firstMultibyte >= 0) cuts.push(firstMultibyte + 1); // splits one UTF-8 character
  for (let i = 17; i < bytes.length; i += 17) cuts.push(i);
  cuts.sort((a, b) => a - b);

  let start = 0;
  const stream = new ReadableStream({
    start(controller) {
      for (const cut of [...cuts, bytes.length]) {
        if (cut > start) controller.enqueue(bytes.subarray(start, cut));
        start = cut;
      }
      controller.close();
    },
  });
  return new Response(stream, {
    status: response.status ?? 200,
    headers: response.headers,
  });
}

async function readAll(translator, request) {
  const chunks = [];
  for await (const chunk of translator.translate(request)) chunks.push(chunk);
  return chunks.join('');
}

const sampleRequest = { system: 's', user: 'u', text: 't', direction: {} };

for (const name of NAMES) {
  test(`contract: replays the ${name} SSE fixture with split lines and split UTF-8`, async () => {
    const { fixture } = await loadFixture(name);
    const { system, user } = buildTranslatorRequest(fixture.input);
    assert.equal(
      user,
      fixture.request.body.messages[1].content,
      'the fixed user-message format must reach the wire byte for byte',
    );

    const calls = [];
    const translator = create({
      preset: fixture.preset,
      apiKey: fixture.request.headers.authorization ? 'test-key' : '',
      fetch: async (url, init) => {
        calls.push({ url, init });
        return responseFor(fixture);
      },
    });

    const output = await readAll(translator, {
      system,
      user,
      text: fixture.input.text,
      direction: {},
    });
    assert.equal(output, fixture.expected);

    assert.equal(calls.length, 1);
    assert.equal(calls[0].url, fixture.request.url);
    assert.equal(calls[0].init.method, fixture.request.method);

    const headers = Object.fromEntries(new Headers(calls[0].init.headers));
    const expectedHeaders = Object.fromEntries(
      Object.entries(fixture.request.headers).map(([key, value]) => [
        key.toLowerCase(),
        value.replace('<redacted>', 'test-key'),
      ]),
    );
    assert.deepEqual(headers, expectedHeaders);

    const body = JSON.parse(calls[0].init.body);
    assert.deepEqual(body, fixture.request.body);
  });
}

test('contract: LLM_EXTRA_BODY is shallow-merged and cannot override model, messages or stream', async () => {
  const { fixture } = await loadFixture('ollama');
  let sent;
  const translator = create({
    preset: 'ollama',
    model: 'llama3.1',
    extraBody: JSON.stringify({
      temperature: 0.2,
      chat_template_kwargs: { reasoning_effort: 'low' },
      model: 'not-the-model',
      stream: false,
      messages: [],
    }),
    fetch: async (url, init) => {
      sent = JSON.parse(init.body);
      return responseFor(fixture);
    },
  });

  await readAll(translator, sampleRequest);
  assert.equal(sent.temperature, 0.2);
  assert.deepEqual(sent.chat_template_kwargs, { reasoning_effort: 'low' });
  assert.equal(sent.model, 'llama3.1');
  assert.equal(sent.stream, true);
  assert.equal(sent.messages.length, 2);
  assert.equal(sent.messages[0].role, 'system');
});

test('contract: invalid LLM_EXTRA_BODY fails provider creation with a clear message', () => {
  const base = { preset: 'ollama', model: 'llama3.1' };
  assert.throws(() => create({ ...base, extraBody: '{not json' }), /LLM_EXTRA_BODY/);
  for (const value of ['[]', 'null', '"text"', '42']) {
    assert.throws(() => create({ ...base, extraBody: value }), /LLM_EXTRA_BODY.*object/);
  }
});

test('contract: fixtures carry no API keys', async () => {
  for (const name of NAMES) {
    const { raw } = await loadFixture(name);
    assert.doesNotMatch(raw, /(?:sk|gsk|sk-or|sk-proj)-[A-Za-z0-9_-]{10,}/, name);
    if (raw.includes('authorization')) assert.match(raw, /<redacted>/, name);
  }
});

test('contract: 401 and 403 surface as auth errors with a fallback', async () => {
  for (const status of [401, 403]) {
    const translator = create({
      preset: 'groq',
      apiKey: 'test-key',
      fetch: async () =>
        new Response('{"error":{"message":"Invalid API Key: test-key"}}', { status }),
    });
    await assert.rejects(readAll(translator, sampleRequest), (error) => {
      assert.equal(error.kind, 'auth');
      assert.match(error.message, new RegExp(String(status)));
      assert.ok(error.fallback);
      assert.doesNotMatch(error.message, /test-key/);
      return true;
    });
  }
});

test('contract: 429 surfaces as a rate_limit error with a fallback', async () => {
  const translator = create({
    preset: 'groq',
    apiKey: 'test-key',
    fetch: async () => new Response('{"error":{"message":"rate limited"}}', { status: 429 }),
  });
  await assert.rejects(readAll(translator, sampleRequest), (error) => {
    assert.equal(error.kind, 'rate_limit');
    assert.ok(error.fallback);
    return true;
  });
});

test('contract: an unreachable endpoint surfaces as an unreachable error', async () => {
  const translator = create({
    preset: 'groq',
    apiKey: 'test-key',
    fetch: async () => {
      throw new TypeError('fetch failed');
    },
  });
  await assert.rejects(readAll(translator, sampleRequest), (error) => {
    assert.equal(error.kind, 'unreachable');
    assert.ok(error.fallback);
    return true;
  });
});

test('contract: a timeout surfaces as a timeout error', async () => {
  const translator = create({
    preset: 'groq',
    apiKey: 'test-key',
    timeoutMs: 5,
    fetch: async (url, init) =>
      new Promise((resolve, reject) => {
        init.signal.addEventListener('abort', () =>
          reject(Object.assign(new Error('aborted'), { name: 'TimeoutError' })),
        );
      }),
  });
  await assert.rejects(readAll(translator, sampleRequest), (error) => {
    assert.equal(error.kind, 'timeout');
    assert.ok(error.fallback);
    return true;
  });
});

test('contract: a non-auth HTTP failure or malformed stream is a protocol error', async () => {
  const badStatus = create({
    preset: 'groq',
    apiKey: 'test-key',
    fetch: async () => new Response('upstream exploded', { status: 500 }),
  });
  await assert.rejects(readAll(badStatus, sampleRequest), (error) => {
    assert.equal(error.kind, 'protocol');
    assert.ok(error.fallback);
    return true;
  });

  const badStream = create({
    preset: 'groq',
    apiKey: 'test-key',
    fetch: async () =>
      new Response('data: {"choices":[{"delta":{"content":oops}}]}\n\n', {
        status: 200,
        headers: { 'content-type': 'text/event-stream' },
      }),
  });
  await assert.rejects(readAll(badStream, sampleRequest), (error) => {
    assert.equal(error.kind, 'protocol');
    return true;
  });
});

test('contract: an empty stream is a protocol error, never a silent empty Segment', async () => {
  const translator = create({
    preset: 'groq',
    apiKey: 'test-key',
    fetch: async () => new Response('data: [DONE]\n\n', { status: 200 }),
  });
  await assert.rejects(readAll(translator, sampleRequest), (error) => {
    assert.equal(error.kind, 'protocol');
    return true;
  });
});
