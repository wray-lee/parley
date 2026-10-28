// Seam for the tauri platform adapter: fake plugin fetch/connect stand in for
// the native side, so timeout, abort, header auth and PCM batching are
// exercised without a phone.
import test from 'node:test';
import assert from 'node:assert/strict';

import { createTauriFetch, createTauriWebSocket } from './platform-tauri.js';

const tick = (ms = 0) => new Promise((resolve) => setTimeout(resolve, ms));

async function waitFor(predicate, timeoutMs = 1000) {
  const started = Date.now();
  while (!predicate()) {
    if (Date.now() - started > timeoutMs) throw new Error('timed out');
    await tick(10);
  }
}

test('fetch hard timeout rejects and ignores the late response', async () => {
  let resolveLate;
  let calls = 0;
  const native = () => {
    calls += 1;
    return new Promise((resolve) => {
      resolveLate = () => resolve(new Response('late'));
    });
  };
  const platformFetch = createTauriFetch({ fetch: native, timeoutMs: 20 });
  await assert.rejects(platformFetch('https://api.example/v1/chat'), (error) => {
    assert.equal(error.name, 'TimeoutError');
    return true;
  });
  assert.equal(calls, 1);
  resolveLate();
  await tick(20); // a late resolution must not become an unhandled rejection
});

test('abort ends the body promptly and drops a racing chunk', async () => {
  let bodyController;
  const body = new ReadableStream({
    start(controller) {
      bodyController = controller;
      controller.enqueue(new TextEncoder().encode('first'));
    },
  });
  const platformFetch = createTauriFetch({
    fetch: async () => new Response(body),
    timeoutMs: 5000,
  });
  const abort = new AbortController();
  const response = await platformFetch('https://api.example/v1/chat', { signal: abort.signal });
  const reader = response.body.getReader();
  const first = await reader.read();
  assert.equal(new TextDecoder().decode(first.value), 'first');

  abort.abort();
  await assert.rejects(reader.read(), (error) => error.name === 'AbortError');

  try {
    bodyController.enqueue(new TextEncoder().encode('late'));
  } catch {
    // the native body was already cancelled: the late chunk cannot arrive
  }
  await tick(10); // the chunk arrives after the abort and is never delivered
});

test('abortAll stops every in-flight request', async () => {
  const platformFetch = createTauriFetch({
    fetch: () => new Promise(() => {}),
    timeoutMs: 5000,
  });
  const pending = platformFetch('https://api.example/slow');
  platformFetch.abortAll();
  await assert.rejects(pending, (error) => error.name === 'AbortError');
});

test('WebSocket facade authenticates with headers and batches PCM', async () => {
  const connections = [];
  const sent = [];
  let listener = null;
  const connect = async (url, config) => {
    connections.push({ url, config });
    return {
      addListener(callback) {
        listener = callback;
      },
      async send(payload) {
        sent.push(payload);
      },
      async disconnect() {},
    };
  };
  const WS = createTauriWebSocket({
    connect,
    batchMs: 40,
    maxBatchBytes: 10_000,
  });
  const ws = new WS('wss://api.deepgram.com/v1/listen', {
    headers: { authorization: 'Token test-key' },
  });
  let opened = false;
  let closed = null;
  let received = null;
  ws.onopen = () => {
    opened = true;
  };
  ws.onmessage = (event) => {
    received = event.data;
  };
  ws.onclose = (event) => {
    closed = event;
  };

  await waitFor(() => opened);
  assert.equal(connections[0].url, 'wss://api.deepgram.com/v1/listen');
  assert.equal(connections[0].config.headers.authorization, 'Token test-key');

  ws.send(new Uint8Array(320).fill(1));
  ws.send(new Uint8Array(320).fill(2));
  assert.equal(sent.length, 0, 'frames wait for the batch window');
  await tick(70);
  assert.equal(sent.length, 1);
  assert.ok(Array.isArray(sent[0]));
  assert.equal(sent[0].length, 640);
  assert.equal(sent[0][0], 1);
  assert.equal(sent[0][639], 2);

  listener({ type: 'Text', data: '{"type":"Results"}' });
  assert.equal(received, '{"type":"Results"}');
  listener({ type: 'Close', data: { code: 1000, reason: 'bye' } });
  assert.deepEqual(closed, { code: 1000, reason: 'bye' });
  assert.equal(ws.readyState, WS.CLOSED);
});

test('WebSocket facade fails closed when the plugin cannot connect', async () => {
  const WS = createTauriWebSocket({
    connect: async () => {
      throw new Error('handshake refused');
    },
  });
  const ws = new WS('wss://api.deepgram.com/v1/listen');
  let failure = null;
  let close = null;
  ws.onerror = (event) => {
    failure = event.error;
  };
  ws.onclose = (event) => {
    close = event;
  };
  await waitFor(() => close !== null);
  assert.match(failure.message, /handshake refused/);
  assert.equal(close.code, 1006);
  assert.equal(ws.readyState, WS.CLOSED);
});
