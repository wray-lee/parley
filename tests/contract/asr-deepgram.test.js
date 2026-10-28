import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import path from 'node:path';

import { create } from '../../core/providers/asr-deepgram.js';

// Replays the Deepgram wire protocol into the adapter; no network is touched.
// live-*.json are real captures with the key stripped (see each `recorded`).
const FIXTURES = path.join(import.meta.dirname, 'fixtures', 'deepgram');

class FakeSocket {
  static instances = [];

  static reset() {
    FakeSocket.instances = [];
  }

  constructor(url, init) {
    this.url = url;
    this.init = init;
    this.sent = [];
    this.closed = false;
    this.listeners = {};
    FakeSocket.instances.push(this);
  }

  on(type, handler) {
    this.listeners[type] = handler;
  }

  send(chunk) {
    this.sent.push(chunk);
  }

  close() {
    this.closed = true;
  }

  open() {
    this.onopen?.();
  }

  receive(data) {
    this.onmessage?.({ data: JSON.stringify(data) });
  }

  rejectHandshake(status) {
    this.listeners['unexpected-response']?.(
      { destroy() {} },
      { statusCode: status, resume() {}, destroy() {} },
    );
  }
}

async function loadFixture(name) {
  const raw = await readFile(path.join(FIXTURES, `${name}.json`), 'utf8');
  return JSON.parse(raw);
}

function start(fixture, overrides = {}) {
  FakeSocket.reset();
  const asr = create({
    ...fixture.options,
    apiKey: fixture.auth?.key ?? '<redacted>',
    WebSocket: FakeSocket,
    ...overrides,
  });
  asr.open(fixture.direction);
  return { asr, socket: FakeSocket.instances.at(-1) };
}

// Stable text as the adapter produces it: each event carries the provider's own
// spacing, so nothing is inserted between events.
function stableText(events) {
  return events
    .filter((event) => event.type === 'stable')
    .reduce((text, event) => `${text}${event.glue ?? ''}${event.text}`, '');
}

// Provider recognition errors are data in the fixture, never language rules.
function normalised(text, corrections = []) {
  let out = text;
  for (const [from, to] of corrections) out = out.split(from).join(to);
  return out.replace(/[\s。、，！？；：]/gu, '');
}

for (const name of ['live-ja-context', 'live-zh-numbers']) {
  test(`contract: the live ${name} capture replays to the provider's transcript`, async () => {
    const fixture = await loadFixture(name);
    const { asr, socket } = start(fixture);

    const url = new URL(socket.url);
    assert.equal(`${url.protocol}//${url.host}${url.pathname}`, 'wss://api.deepgram.com/v1/listen');
    for (const [key, value] of Object.entries(fixture.urlParams)) {
      assert.equal(url.searchParams.get(key), value, key);
    }
    // Language is fixed per connection: no auto-detection parameter.
    assert.equal(url.searchParams.has('detect_language'), false);
    assert.deepEqual(socket.init, {
      headers: { authorization: `Token ${fixture.auth.key}` },
    });

    socket.open();
    const events = [];
    for (const frame of fixture.frames) {
      socket.receive(frame.data);
      events.push(...asr.push({ pcm: new Int16Array(0), audioMs: frame.atMs }));
    }

    const text = stableText(events);
    assert.equal(text, fixture.expectedStableText, 'no duplication, dropped words or invented spaces');
    assert.equal(/\s/.test(text), false, 'the provider transcript has no whitespace here');
    assert.equal(
      normalised(text, fixture.recognitionCorrections),
      normalised(fixture.sampleTranscript),
      'covers the sample transcript modulo the recorded recognition errors',
    );
    for (const event of events.filter((e) => e.type === 'stable')) {
      assert.ok(Number.isFinite(event.startMs) && Number.isFinite(event.endMs), 'span is audio time');
      assert.ok(event.startMs <= event.endMs);
    }
    assert.ok(events.some((e) => e.type === 'partial'), 'interims are display-only Partials');
  });
}

test('contract: a final that lands after CloseStream appends its tail without repeating stable words', async () => {
  const fixture = await loadFixture('live-ja-context-late-final');
  const { asr, socket } = start(fixture);
  socket.open();

  const events = [];
  const collect = () => events.push(...asr.push({ pcm: new Int16Array(0) }));
  for (const frame of fixture.frames.slice(0, fixture.closeAfterFrame)) {
    socket.receive(frame.data);
    collect();
  }
  asr.close();
  for (const frame of fixture.frames.slice(fixture.closeAfterFrame)) {
    socket.receive(frame.data);
    collect();
  }

  assert.equal(stableText(events), fixture.expectedStableText);
  const duplicated = stableText(events).match(/議事録が固まったら/gu) ?? [];
  assert.equal(duplicated.length, 1, 'the clause is not repeated');
});

test('contract: stable text keeps the provider spacing when the transcript has spaces', () => {
  FakeSocket.reset();
  const asr = create({ apiKey: '<redacted>', WebSocket: FakeSocket });
  asr.open({ src: 'en' });
  const socket = FakeSocket.instances.at(-1);
  socket.open();

  const frame = (transcript, words, isFinal) => ({
    type: 'Results',
    start: 0,
    duration: 1,
    is_final: isFinal,
    channel: {
      alternatives: [
        {
          transcript,
          words: words.map((word, index) => ({
            word,
            punctuated_word: word,
            start: index * 0.4,
            end: index * 0.4 + 0.3,
          })),
        },
      ],
    },
  });

  const events = [];
  for (const data of [
    frame('hello', ['hello'], false),
    frame('hello world', ['hello', 'world'], false),
    frame('hello world', ['hello', 'world'], true),
  ]) {
    socket.receive(data);
    events.push(...asr.push({ pcm: new Int16Array(0) }));
  }

  assert.equal(stableText(events), 'hello world');
});

test('contract: web auth rides the subprotocol, node auth rides the header', () => {
  FakeSocket.reset();
  const header = create({ apiKey: 'test-key', WebSocket: FakeSocket });
  header.open({ src: 'zh' });
  assert.deepEqual(FakeSocket.instances.at(-1).init, {
    headers: { authorization: 'Token test-key' },
  });
  assert.equal(new URL(FakeSocket.instances.at(-1).url).searchParams.get('language'), 'zh');

  const subprotocol = create({ apiKey: 'test-key', auth: 'subprotocol', WebSocket: FakeSocket });
  subprotocol.open({ src: 'ja' });
  assert.deepEqual(FakeSocket.instances.at(-1).init, ['token', 'test-key']);
  assert.equal(new URL(FakeSocket.instances.at(-1).url).searchParams.get('language'), 'ja');
});

for (const name of ['auth-failure', 'quota-exhaustion']) {
  test(`contract: replays the ${name} handshake as an error event with a fallback`, async () => {
    const fixture = await loadFixture(name);
    const { asr, socket } = start(fixture);
    socket.rejectHandshake(fixture.handshake.status);

    const events = asr.push({ pcm: new Int16Array(0), audioMs: 0 });
    assert.equal(events.length, 1);
    const [error] = events;
    assert.equal(error.type, 'error');
    assert.equal(error.provider, 'asr');
    assert.equal(error.kind, fixture.expected.kind);
    assert.match(error.message, new RegExp(fixture.handshake.status));
    for (const hint of fixture.expected.fallbackContains) {
      assert.ok(error.fallback.includes(hint), `fallback mentions ${hint}`);
    }
  });
}

test('contract: a missing key fails with an auth error instead of connecting', () => {
  FakeSocket.reset();
  const asr = create({ WebSocket: FakeSocket });
  asr.open({ src: 'ja' });
  assert.equal(FakeSocket.instances.length, 0);

  const [error] = asr.push({ pcm: new Int16Array(0), audioMs: 0 });
  assert.equal(error.type, 'error');
  assert.equal(error.kind, 'auth');
  assert.match(error.fallback, /DEEPGRAM_API_KEY/);
  assert.match(error.fallback, /ASR_PROVIDER=mock/);
});

test('contract: audio frames wait for the socket to open', () => {
  FakeSocket.reset();
  const asr = create({ apiKey: 'k', WebSocket: FakeSocket });
  asr.open({ src: 'ja' });
  const socket = FakeSocket.instances.at(-1);
  const pcm = new Int16Array([1, 2, 3]);

  asr.push({ pcm, audioMs: 20 });
  assert.equal(socket.sent.length, 0);
  socket.open();
  assert.equal(socket.sent.length, 1);
  assert.equal(socket.sent[0], pcm);
});
