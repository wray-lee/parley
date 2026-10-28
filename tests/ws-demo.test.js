import { test } from 'node:test';
import assert from 'node:assert/strict';
import { WebSocket } from 'ws';

import { createServer } from '../hosts/node/server.js';
import { loadConfig } from '../hosts/node/config.js';

function pcmChunk(ms, amplitude) {
  const n = (16000 * ms) / 1000;
  const pcm = new Int16Array(n);
  if (amplitude !== 0) {
    for (let i = 0; i < n; i++) {
      pcm[i] = Math.round(Math.sin((2 * Math.PI * 220 * i) / 16000) * amplitude * 32767);
    }
  }
  return Buffer.from(pcm.buffer);
}

function connect(url) {
  const ws = new WebSocket(url);
  const messages = [];
  const waiters = [];
  let binaryCount = 0;
  ws.on('message', (data, isBinary) => {
    if (isBinary) {
      binaryCount++;
      messages.push({ type: 'binary' });
      return;
    }
    const message = JSON.parse(data.toString());
    messages.push(message);
    for (const waiter of [...waiters]) {
      if (waiter.predicate(message)) {
        waiters.splice(waiters.indexOf(waiter), 1);
        waiter.resolve(message);
      }
    }
  });
  return {
    ws,
    messages,
    get binaryCount() { return binaryCount; },
    waitFor(predicate, timeoutMs = 3000) {
      const found = messages.find(predicate);
      if (found) return Promise.resolve(found);
      return new Promise((resolve, reject) => {
        const waiter = { predicate, resolve };
        waiters.push(waiter);
        setTimeout(() => {
          const i = waiters.indexOf(waiter);
          if (i !== -1) waiters.splice(i, 1);
          reject(new Error(`timed out waiting for message; got ${JSON.stringify(messages)}`));
        }, timeoutMs);
      });
    },
  };
}

async function startServer() {
  const records = [];
  const warnings = [];
  const server = await createServer({
    config: loadConfig({ PORT: '0' }),
    logger: { segment: (record) => records.push(record), warn: (line) => warnings.push(line) },
  });
  await server.listen(0);
  return { server, port: server.address().port, records, warnings };
}

test('Server Host serves the demo page and healthz', async () => {
  const { server, port } = await startServer();
  try {
    const health = await fetch(`http://127.0.0.1:${port}/healthz`);
    assert.equal(health.status, 200);
    assert.deepEqual(await health.json(), { ok: true });

    const page = await (await fetch(`http://127.0.0.1:${port}/`)).text();
    assert.match(page, /id="demo-banner"/);
    assert.match(page, /\/main\.js/);

    const sample = await (await fetch(`http://127.0.0.1:${port}/samples/placeholder.json`)).json();
    assert.equal(sample.id, 'placeholder');
  } finally {
    await server.close();
  }
});

test('the full mock flow runs over a real WebSocket', async () => {
  const { server, port, records } = await startServer();
  const conn = connect(`ws://127.0.0.1:${port}/ws`);
  try {
    const hello = await conn.waitFor((m) => m.type === 'hello');
    assert.equal(hello.demo, true);
    assert.ok(
      hello.pairs.some((pair) => pair.directions.some((d) => d.src === 'zh' && d.tgt === 'ja')),
      'hello lists the zh->ja Direction',
    );

    conn.ws.send(JSON.stringify({ type: 'start', mode: 'listen', src: 'zh', tgt: 'ja' }));
    conn.ws.send(JSON.stringify({ type: 'sample', name: 'placeholder' }));
    for (let i = 0; i < 16; i++) conn.ws.send(pcmChunk(100, 0.2));
    for (let i = 0; i < 10; i++) conn.ws.send(pcmChunk(100, 0));

    const segment = await conn.waitFor((m) => m.type === 'segment');
    assert.equal(segment.seg_id, 1);
    assert.equal(segment.src, '你好，这是演示。');
    assert.match(segment.tgt, /^\[DEMO\] /);
    assert.equal(segment.demo, true);

    await conn.waitFor((m) => m.type === 'tts_end');
    assert.ok(conn.binaryCount > 0, 'mock TTS sent at least one binary chunk');

    const ttsBegin = conn.messages.find((m) => m.type === 'tts_begin');
    assert.equal(ttsBegin.seg_id, 1);
    assert.equal(ttsBegin.format, 'wav');

    assert.equal(records.length, 1);
    assert.equal(records[0].seg_id, 1);
    assert.equal(records[0].asr_final, '你好，这是演示。');
    assert.equal(records[0].direction.src, 'zh');
    assert.equal(records[0].direction.tgt, 'ja');
  } finally {
    conn.ws.close();
    await server.close();
  }
});

test('microphone frames over the socket yield the mock-ASR error', async () => {
  const { server, port } = await startServer();
  const conn = connect(`ws://127.0.0.1:${port}/ws`);
  try {
    await conn.waitFor((m) => m.type === 'hello');
    conn.ws.send(JSON.stringify({ type: 'start', mode: 'listen', src: 'zh', tgt: 'ja' }));
    conn.ws.send(pcmChunk(100, 0.2));

    const error = await conn.waitFor((m) => m.type === 'error');
    assert.equal(error.provider, 'asr');
    assert.match(error.message, /bundled samples/);
  } finally {
    conn.ws.close();
    await server.close();
  }
});
