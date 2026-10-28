import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import path from 'node:path';

import { create } from '../../core/providers/asr-whisperlivekit.js';

// Replays WhisperLiveKit full-mode JSON into the adapter; no network is touched.
const FIXTURES = path.join(import.meta.dirname, 'fixtures', 'whisperlivekit');

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
    this.onclose?.({ code: 1000 });
  }

  open() {
    this.onopen?.();
  }

  receive(data) {
    this.onmessage?.({ data: JSON.stringify(data) });
  }

  fail() {
    this.onerror?.(new Error('connection refused'));
    this.onclose?.({ code: 1006 });
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
  const asr = create({ url: fixture.url, WebSocket: FakeSocket, ...overrides });
  asr.open(fixture.direction);
  return { asr, socket: FakeSocket.instances.at(-1) };
}

test('contract: replays recorded full-mode JSON as Partial and Stable Prefix events', async () => {
  const fixture = await loadFixture('full-mode');
  const { asr, socket } = start(fixture);

  const url = new URL(socket.url);
  assert.equal(`${url.protocol}//${url.host}${url.pathname}`, 'ws://127.0.0.1:8000/asr');
  assert.equal(url.searchParams.get('language'), fixture.direction.src);
  assert.equal(url.searchParams.get('mode'), 'full');

  socket.open();
  const stable = [];
  for (const frame of fixture.frames) {
    socket.receive(frame.data);
    const pcm = new Int16Array(160);
    const events = asr.push({ pcm, audioMs: frame.atMs });
    assert.deepEqual(events, frame.events, `frame at ${frame.atMs} ms`);
    for (const event of events) if (event.type === 'stable') stable.push(event.text);
    if (!socket.closed) assert.equal(socket.sent.at(-1), pcm);
  }
  // The full lines list is resent on every update but each committed line
  // becomes exactly one Stable Prefix event.
  assert.equal(stable.join(' '), fixture.expectedStableText);
});

test('contract: end of stream sends an empty binary frame and waits for ready_to_stop', () => {
  const { asr, socket } = start({ url: 'ws://wlk.test:8000', direction: { src: 'ja' } });
  socket.open();
  asr.push({ pcm: new Int16Array(160), audioMs: 10 });

  asr.close();
  const frame = socket.sent.at(-1);
  assert.ok(frame instanceof Uint8Array, 'end-of-audio frame is binary');
  assert.equal(frame.byteLength, 0);
  assert.equal(socket.closed, false, 'socket stays open until ready_to_stop');

  socket.receive({ type: 'ready_to_stop' });
  assert.equal(socket.closed, true);
});

test('contract: audio frames wait for the socket to open and are sent as-is', () => {
  FakeSocket.reset();
  const asr = create({ url: 'ws://wlk.test:8000', WebSocket: FakeSocket });
  asr.open({ src: 'zh' });
  const socket = FakeSocket.instances.at(-1);
  const pcm = new Int16Array([1, 2, 3]);

  asr.push({ pcm, audioMs: 20 });
  assert.equal(socket.sent.length, 0);
  socket.open();
  assert.equal(socket.sent.length, 1);
  assert.equal(socket.sent[0], pcm);
});

test('contract: replays the in-band error frame as an error event with a fallback', async () => {
  const fixture = await loadFixture('in-band-error');
  const { asr, socket } = start(fixture);
  socket.open();

  socket.receive(fixture.frame);
  const [error] = asr.push({ pcm: new Int16Array(0), audioMs: fixture.frameAtMs });
  assert.equal(error.type, 'error');
  assert.equal(error.provider, 'asr');
  assert.equal(error.kind, fixture.expected.kind);
  assert.match(error.message, new RegExp(fixture.expected.messageContains));
  for (const text of fixture.expected.fallbackContains) {
    assert.match(error.fallback, new RegExp(text));
  }
});

test('contract: open refuses a Direction without a source language', () => {
  FakeSocket.reset();
  const asr = create({ url: 'ws://wlk.test:8000', WebSocket: FakeSocket });
  assert.throws(() => asr.open({}), /source language/);
  assert.equal(FakeSocket.instances.length, 0);
});

test('contract: an unreachable WhisperLiveKit names the URL and the local stack', () => {
  const { asr, socket } = start({ url: 'ws://wlk.test:8000', direction: { src: 'ja' } });
  socket.fail();

  const [error] = asr.push({ pcm: new Int16Array(0), audioMs: 0 });
  assert.equal(error.type, 'error');
  assert.equal(error.kind, 'unreachable');
  assert.match(error.message, /WhisperLiveKit/);
  assert.match(error.fallback, /WHISPERLIVEKIT_URL/);
  assert.match(error.fallback, /--profile local/);
  assert.match(error.fallback, /ASR_PROVIDER=mock/);
});

test('contract: a server without --pcm-input fails with an actionable error', () => {
  const { asr, socket } = start({ url: 'ws://wlk.test:8000', direction: { src: 'ja' } });
  socket.open();
  socket.receive({ type: 'config', useAudioWorklet: false, mode: 'full' });

  const [error] = asr.push({ pcm: new Int16Array(0), audioMs: 0 });
  assert.equal(error.type, 'error');
  assert.equal(error.kind, 'config');
  assert.match(error.message, /PCM/);
  assert.match(error.fallback, /--pcm-input/);
  assert.match(error.fallback, /ASR_PROVIDER=mock/);
});
