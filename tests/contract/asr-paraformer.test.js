import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import path from 'node:path';

import { create } from '../../core/providers/asr-paraformer.js';

// Replays the DashScope Paraformer wire protocol into the adapter; no network
// is touched. Fixtures marked "recorded" in each file say where they came from.
const FIXTURES = path.join(import.meta.dirname, '..', '..', 'spec', 'fixtures', 'paraformer');
const UUID = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;

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

function start(fixture) {
  FakeSocket.reset();
  const asr = create({
    ...fixture.options,
    apiKey: fixture.auth?.key ?? '<redacted>',
    WebSocket: FakeSocket,
  });
  asr.open(fixture.direction);
  const socket = FakeSocket.instances.at(-1);
  socket.open();
  return { asr, socket };
}

function replay(asr, socket, frames) {
  const events = [];
  for (const frame of frames) {
    socket.receive(frame.data);
    events.push(...asr.push({}));
  }
  return events;
}

test('contract: opens a DashScope task with the Direction hint and streams PCM only after task-started', async () => {
  const fixture = await loadFixture('interim-final');
  FakeSocket.reset();
  const asr = create({
    ...fixture.options,
    apiKey: '<redacted>',
    WebSocket: FakeSocket,
  });
  asr.open({ src: 'ja' });
  const socket = FakeSocket.instances.at(-1);

  assert.equal(socket.init.headers.Authorization, 'Bearer <redacted>');
  socket.open();
  const runTask = JSON.parse(socket.sent[0]);
  assert.equal(runTask.header.action, 'run-task');
  assert.match(runTask.header.task_id, UUID);
  assert.equal(runTask.header.streaming, 'duplex');
  assert.deepEqual(runTask.payload, {
    task_group: 'audio',
    task: 'asr',
    function: 'recognition',
    model: 'paraformer-realtime-v2',
    parameters: {
      format: 'pcm',
      sample_rate: 16000,
      language_hints: ['ja'],
    },
    input: {},
  });

  const pcm = new Int16Array([1, 2, 3, 4]);
  assert.deepEqual(asr.push({ pcm }), []);
  assert.equal(socket.sent.length, 1, 'audio must wait for task-started');

  socket.receive({
    header: { task_id: runTask.header.task_id, event: 'task-started', attributes: {} },
    payload: {},
  });
  assert.equal(socket.sent.length, 2);
  assert.equal(socket.sent[1], pcm);

  asr.close();
  const finish = JSON.parse(socket.sent.at(-1));
  assert.equal(finish.header.action, 'finish-task');
  assert.equal(finish.header.task_id, runTask.header.task_id);
  assert.equal(finish.header.streaming, 'duplex');
  assert.deepEqual(finish.payload, { input: {} });
});

test('contract: replays Paraformer result frames as Partial and Stable Prefix events with audio spans', async () => {
  const fixture = await loadFixture('interim-final');
  const { asr, socket } = start(fixture);

  const events = replay(asr, socket, fixture.frames);

  assert.deepEqual(events, fixture.frames.flatMap((frame) => frame.events ?? []));
});

test('contract: fixes the language hint from the active Direction source', async () => {
  FakeSocket.reset();
  const asr = create({ apiKey: '<redacted>', WebSocket: FakeSocket });
  asr.open({ src: 'zh-Hans' });
  const socket = FakeSocket.instances.at(-1);
  socket.open();

  const runTask = JSON.parse(socket.sent[0]);
  assert.deepEqual(runTask.payload.parameters.language_hints, ['zh']);
});

test('contract: a rejected handshake surfaces an auth error with a fallback', async () => {
  const fixture = await loadFixture('auth-failure');
  const { asr, socket } = start(fixture);

  socket.rejectHandshake(fixture.handshake.status);
  const events = asr.push({});

  assert.equal(events.length, 1);
  assert.equal(events[0].type, 'error');
  assert.equal(events[0].provider, 'asr');
  assert.equal(events[0].kind, 'auth');
  assert.match(events[0].message, /HTTP 401/);
  assert.match(events[0].fallback, /DASHSCOPE_API_KEY/);
  assert.match(events[0].fallback, /Beijing/);
});

test('contract: a task-failed Arrearage frame surfaces a quota error with the free-quota fallback', async () => {
  const fixture = await loadFixture('quota-exhaustion');
  const { asr, socket } = start(fixture);

  const events = replay(asr, socket, fixture.frames);

  assert.equal(events.length, 1);
  assert.equal(events[0].type, 'error');
  assert.equal(events[0].provider, 'asr');
  assert.equal(events[0].kind, 'quota');
  assert.match(events[0].message, /Arrearage/);
  assert.match(events[0].fallback, /10 h\/month/);
  assert.match(events[0].fallback, /Beijing/);
});

test('contract: a task-failed model/region error surfaces a region error with a fallback', async () => {
  const fixture = await loadFixture('region-error');
  const { asr, socket } = start(fixture);

  const events = replay(asr, socket, fixture.frames);

  assert.equal(events.length, 1);
  assert.equal(events[0].type, 'error');
  assert.equal(events[0].provider, 'asr');
  assert.equal(events[0].kind, 'region');
  assert.match(events[0].message, /ModelNotFound/);
  assert.match(events[0].fallback, /Beijing/);
  assert.match(events[0].fallback, /endpoint/);
});

test('contract: a rapid stop/start does not let the finish-task timer close the new socket', async (t) => {
  t.mock.timers.enable({ apis: ['setTimeout'] });
  const fixture = await loadFixture('interim-final');
  const { asr, socket } = start(fixture);
  socket.receive(fixture.frames[0].data); // task-started
  asr.push({});

  asr.close();
  const old = socket;
  asr.open({ src: 'zh' });
  const next = FakeSocket.instances.at(-1);
  next.open();

  t.mock.timers.tick(1000);

  assert.equal(old.closed, true);
  assert.equal(next.closed, false);
});

test('contract: a missing DASHSCOPE_API_KEY surfaces an auth error without opening a socket', () => {
  FakeSocket.reset();
  const asr = create({ WebSocket: FakeSocket });
  asr.open({ src: 'ja' });

  const events = asr.push({});

  assert.equal(FakeSocket.instances.length, 0);
  assert.equal(events.length, 1);
  assert.equal(events[0].kind, 'auth');
  assert.match(events[0].message, /DASHSCOPE_API_KEY/);
  assert.match(events[0].fallback, /DASHSCOPE_API_KEY/);
});
