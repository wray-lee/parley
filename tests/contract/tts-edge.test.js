// Seam 2: replay a recorded Edge read-aloud WebSocket session through the
// adapter. The fixture holds only response frames; request ids are stripped.
// The session was recorded from the live service on 2026-10-28.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';

import { create } from '../../core/providers/tts-edge.js';

const VECTOR_TIME = Date.parse('2026-10-13T10:00:30Z');
const VECTOR_TOKEN =
  '461B5A82FDC12AF5-A89B0EBF2C158E7B404FF47B1F12D5579CB3C7D7B18EC8ADE4A4AD0A8036C8DE';

function frames(name) {
  return JSON.parse(
    readFileSync(new URL(`../../spec/fixtures/edge/${name}.frames.json`, import.meta.url), 'utf8'),
  );
}

function audioFrom(framesList) {
  const parts = [];
  for (const frame of framesList) {
    if (frame.binary === undefined) continue;
    const buffer = Buffer.from(frame.binary, 'base64');
    const headerLength = buffer.readUInt16BE(0);
    const header = buffer.subarray(2, 2 + headerLength).toString();
    if (header.split('\r\n').includes('Path:audio')) parts.push(buffer.subarray(2 + headerLength));
  }
  return new Uint8Array(Buffer.concat(parts));
}

class FakeSocket {
  static sockets = [];

  constructor(url, options) {
    this.url = url;
    this.options = options;
    this.sent = [];
    this.handlers = new Map();
    FakeSocket.sockets.push(this);
    queueMicrotask(() => this.emit('open'));
  }

  on(type, handler) {
    const list = this.handlers.get(type) ?? [];
    list.push(handler);
    this.handlers.set(type, list);
  }

  emit(type, ...args) {
    for (const handler of [...(this.handlers.get(type) ?? [])]) handler(...args);
  }

  send(data) {
    this.sent.push(String(data));
  }

  close() {}

  terminate() {}
}

async function replay(socket, framesList) {
  while (socket.sent.length < 2) await new Promise((resolve) => setTimeout(resolve, 0));
  for (const frame of framesList) {
    if (frame.text !== undefined) socket.emit('message', Buffer.from(frame.text), false);
    else socket.emit('message', Buffer.from(frame.binary, 'base64'), true);
  }
}

function makeTts(options = {}) {
  FakeSocket.sockets = [];
  return create({ WebSocket: FakeSocket, warn: () => {}, ...options });
}

const request = { text: 'こんにちは。', seg_id: 1, direction: { src: 'zh', tgt: 'ja' } };

test('a recorded Edge session replays into one mp3 chunk', async () => {
  const recorded = frames('ja');
  const expected = audioFrom(recorded);
  const tts = makeTts({ now: () => VECTOR_TIME });
  const pending = tts.synthesize(request);
  const socket = FakeSocket.sockets[0];
  await replay(socket, recorded);
  const audio = await pending;

  assert.equal(tts.name, 'edge');
  assert.equal(audio.format, 'mp3');
  assert.deepEqual(audio.data, expected);

  assert.match(socket.url, /^wss:\/\/speech\.platform\.bing\.com\//);
  assert.ok(socket.url.includes(`Sec-MS-GEC=${VECTOR_TOKEN}`));
  assert.ok(socket.url.includes('Sec-MS-GEC-Version=2-'));
  assert.ok(socket.url.includes('TrustedClientToken=6A5AA1D4EAFF4E9FB37E23D68491D6F4'));
  assert.equal(
    socket.options.headers.Origin,
    'chrome-extension://jdiccldimpdaibmpdkjnbmckianbfold',
  );

  assert.ok(socket.sent[0].includes('Path:speech.config'));
  assert.ok(socket.sent[0].includes('audio-24khz-48kbitrate-mono-mp3'));
  assert.ok(socket.sent[1].includes('Path:ssml'));
  assert.ok(socket.sent[1].includes("voice name='ja-JP-NanamiNeural'"));
  assert.ok(socket.sent[1].includes("xml:lang='ja-JP'"));
  assert.ok(socket.sent[1].includes('こんにちは。'));
});

test('a zh session records and replays the same way', async () => {
  const recorded = frames('zh');
  const tts = makeTts();
  const pending = tts.synthesize({
    text: '你好。',
    seg_id: 2,
    direction: { src: 'ja', tgt: 'zh' },
  });
  const socket = FakeSocket.sockets[0];
  await replay(socket, recorded);
  const audio = await pending;

  assert.deepEqual(audio.data, audioFrom(recorded));
  assert.ok(socket.sent[1].includes("voice name='zh-CN-XiaoxiaoNeural'"));
});

test('a rejected handshake names kokoro or system', async () => {
  const tts = makeTts();
  const pending = tts.synthesize(request);
  const socket = FakeSocket.sockets[0];
  await new Promise((resolve) => setTimeout(resolve, 0));

  socket.emit('unexpected-response', {}, { statusCode: 403 });
  await assert.rejects(pending, /HTTP 403/);
  await assert.rejects(pending, /kokoro or system/);
});

test('a socket error before the session starts names kokoro or system', async () => {
  const tts = makeTts();
  const pending = tts.synthesize(request);
  const socket = FakeSocket.sockets[0];
  const rejection = assert.rejects(pending, /kokoro or system/);
  socket.emit('error', new Error('ECONNRESET'));
  await rejection;
});

test('a close before turn.end fails instead of returning a truncated clip', async () => {
  const tts = makeTts();
  const pending = tts.synthesize(request);
  const socket = FakeSocket.sockets[0];
  await new Promise((resolve) => setTimeout(resolve, 0));
  const rejection = assert.rejects(pending, /closed the connection/);
  socket.emit('close', 1006, Buffer.alloc(0));
  await rejection;
});

test('a malformed audio frame is ignored instead of crashing', async () => {
  const tts = makeTts();
  const pending = tts.synthesize(request);
  const socket = FakeSocket.sockets[0];
  await new Promise((resolve) => setTimeout(resolve, 0));

  const rejection = assert.rejects(pending, /returned no audio/);
  socket.emit('message', Buffer.from([1]), true);
  socket.emit('message', Buffer.from('Path:turn.end\r\n\r\n'), false);
  await rejection;
});

test('a timeout fails with the alternatives', async () => {
  const tts = makeTts({ timeoutMs: 5 });
  await assert.rejects(tts.synthesize(request), /timed out after 5 ms.*kokoro or system/);
});

test('a missing voice falls back to the base language and warns', async () => {
  const recorded = frames('ja');
  const warnings = [];
  const tts = makeTts({
    voices: { ja: ['ja-JP-NanamiNeural'] },
    warn: (line) => warnings.push(line),
  });
  const pending = tts.synthesize({ ...request, direction: { src: 'zh', tgt: 'ja-JP' } });
  const socket = FakeSocket.sockets[0];
  await replay(socket, recorded);
  await pending;

  assert.equal(warnings.length, 1);
  assert.match(warnings[0], /no voice configured for "ja-JP"/);
  assert.ok(socket.sent[1].includes("voice name='ja-JP-NanamiNeural'"));
});

test('a language with no configured voice fails with the alternatives', async () => {
  const tts = makeTts();
  await assert.rejects(
    tts.synthesize({ ...request, direction: { src: 'zh', tgt: 'fr' } }),
    /no voice configured for target language "fr".*kokoro or system/,
  );
  assert.equal(FakeSocket.sockets.length, 0);
});
