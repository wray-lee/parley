import test from 'node:test';
import assert from 'node:assert/strict';

import { createServerWebSocket, serverSocketUrl } from './server-socket.js';

const tick = () => new Promise((resolve) => setImmediate(resolve));

class FakeNative {
  static made = [];

  constructor(url, protocols) {
    this.url = url;
    this.protocols = protocols;
    this.listeners = new Map();
    this.sent = [];
    this.readyState = 0;
    this.closed = false;
    FakeNative.made.push(this);
  }

  addEventListener(type, listener) {
    if (!this.listeners.has(type)) this.listeners.set(type, []);
    this.listeners.get(type).push(listener);
  }

  send(data) {
    this.sent.push(data);
  }

  close() {
    this.closed = true;
  }

  emit(type, event) {
    for (const listener of this.listeners.get(type) ?? []) listener(event);
  }
}

test('server URL becomes wss and carries the token as a query parameter', () => {
  assert.equal(serverSocketUrl('https://parley.example.com'), 'wss://parley.example.com/ws');
  assert.equal(
    serverSocketUrl('https://parley.example.com/base', 'a b&c'),
    'wss://parley.example.com/base/ws?token=a+b%26c',
  );
  assert.equal(serverSocketUrl('http://192.168.1.5:3000/ws'), 'ws://192.168.1.5:3000/ws');
});

test('server socket redirects the web client and queues until open', async () => {
  FakeNative.made = [];
  const WS = createServerWebSocket({
    Native: FakeNative,
    serverUrl: 'https://parley.example.com',
    token: Promise.resolve('secret-token'),
  });
  const ws = new WS('ws://tauri.localhost/ws');
  let opened = 0;
  const messages = [];
  ws.addEventListener('open', () => {
    opened += 1;
  });
  ws.addEventListener('message', (event) => messages.push(event.data));

  await tick();
  const native = FakeNative.made[0];
  assert.equal(native.url, 'wss://parley.example.com/ws?token=secret-token');

  ws.send('{"type":"start"}');
  assert.equal(native.sent.length, 0, 'queued while connecting');
  native.emit('open', {});
  assert.equal(opened, 1);
  assert.deepEqual(native.sent, ['{"type":"start"}']);
  assert.equal(ws.readyState, WS.OPEN);

  native.emit('message', { data: '{"type":"hello"}' });
  assert.deepEqual(messages, ['{"type":"hello"}']);

  ws.close();
  assert.equal(native.closed, true);
  assert.equal(ws.readyState, WS.CLOSED);
});

test('a bad server URL fails the socket instead of hanging', async () => {
  const WS = createServerWebSocket({
    Native: FakeNative,
    serverUrl: 'not a url',
    token: Promise.resolve(''),
  });
  const ws = new WS('ws://tauri.localhost/ws');
  let failed = 0;
  let closed = 0;
  ws.addEventListener('error', () => {
    failed += 1;
  });
  ws.addEventListener('close', () => {
    closed += 1;
  });
  await tick();
  assert.equal(failed, 1);
  assert.equal(closed, 1);
  assert.equal(ws.readyState, WS.CLOSED);
});
