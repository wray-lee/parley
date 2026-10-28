import { test } from 'node:test';
import assert from 'node:assert/strict';
import { createServer as createHttpServer } from 'node:http';
import { mkdir, mkdtemp, rm, writeFile } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import path from 'node:path';
import { WebSocket } from 'ws';

import { createServer, loadRegistry } from '../hosts/node/server.js';
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
  ws.on('message', (data, isBinary) => {
    if (isBinary) return;
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

function handshake(url) {
  return new Promise((resolve, reject) => {
    const ws = new WebSocket(url);
    ws.once('open', () => resolve({ ws, status: 101, body: '' }));
    ws.once('unexpected-response', (request, response) => {
      const chunks = [];
      response.on('data', (chunk) => chunks.push(Buffer.from(chunk)));
      response.on('end', () =>
        resolve({ ws, status: response.statusCode, body: Buffer.concat(chunks).toString() }));
      response.on('error', reject);
    });
    ws.once('error', reject);
  });
}

async function startServer(env = {}) {
  const records = [];
  const warnings = [];
  const server = await createServer({
    config: loadConfig({ PORT: '0', ...env }),
    logger: { segment: (record) => records.push(record), warn: (line) => warnings.push(line) },
  });
  await server.listen(0);
  return { server, port: server.address().port, records, warnings };
}

async function startFakeTranslator() {
  const requests = [];
  const http = createHttpServer((req, res) => {
    let body = '';
    req.on('data', (chunk) => { body += chunk; });
    req.on('end', () => {
      requests.push(JSON.parse(body));
      res.writeHead(200, { 'content-type': 'text/event-stream' });
      res.end('data: {"choices":[{"delta":{"content":"予算です"}}]}\n\ndata: [DONE]\n\n');
    });
  });
  await new Promise((resolve) => http.listen(0, '127.0.0.1', resolve));
  return { http, requests, port: http.address().port };
}

test('loadRegistry gives the registry base glossary and pack text', async () => {
  const root = await mkdtemp(path.join(tmpdir(), 'parley-registry-'));
  try {
    await mkdir(path.join(root, 'prompts'));
    await mkdir(path.join(root, 'packs'));
    await writeFile(path.join(root, 'prompts', 'system_zh2ja.txt'), 'SYSTEM {{GLOSSARY}}');
    await writeFile(path.join(root, 'prompts', 'glossary_zh2ja.tsv'), '预算\t予算\n');
    await writeFile(path.join(root, 'packs', 'conference_zh2ja.tsv'), '议程\tアジェンダ\n');
    const warnings = [];
    const registry = await loadRegistry(root, (line) => warnings.push(line));
    assert.deepEqual(registry.directions[0].glossary, [{ src: '预算', tgt: '予算' }]);
    assert.deepEqual(registry.packs[0].glossary, [{ src: '议程', tgt: 'アジェンダ' }]);
    assert.deepEqual(warnings, []);
  } finally {
    await rm(root, { recursive: true, force: true });
  }
});

test('an unknown pack is a protocol error over a real WebSocket', async () => {
  const { server, port } = await startServer();
  const conn = connect(`ws://127.0.0.1:${port}/ws`);
  try {
    const hello = await conn.waitFor((m) => m.type === 'hello');
    assert.deepEqual(hello.presetsInUse, []);
    assert.ok(
      hello.packs.some((pack) => pack.name === 'conference' && pack.src === 'zh' && pack.tgt === 'ja'),
      'hello lists the conference pack',
    );

    conn.ws.send(JSON.stringify({
      type: 'start', mode: 'listen', src: 'zh', tgt: 'ja', packs: ['conference', 'missing-pack'],
    }));
    const error = await conn.waitFor((m) => m.type === 'error');
    assert.equal(error.kind, 'protocol');
    assert.equal(error.message, 'unknown pack: missing-pack');
  } finally {
    conn.ws.close();
    await server.close();
  }
});

test('the conference glossary reaches the live translator request', async () => {
  const fake = await startFakeTranslator();
  const { server, port } = await startServer({
    LLM_BASE_URL: `http://127.0.0.1:${fake.port}/v1`,
    LLM_MODEL: 'fake-model',
    LLM_API_KEY: 'fake-key-for-test',
  });
  const conn = connect(`ws://127.0.0.1:${port}/ws`);
  try {
    await conn.waitFor((m) => m.type === 'hello');
    conn.ws.send(JSON.stringify({
      type: 'start', mode: 'listen', src: 'zh', tgt: 'ja', packs: ['conference'],
    }));
    conn.ws.send(JSON.stringify({ type: 'sample', name: 'placeholder' }));
    for (let i = 0; i < 16; i++) conn.ws.send(pcmChunk(100, 0.2));
    for (let i = 0; i < 10; i++) conn.ws.send(pcmChunk(100, 0));

    await conn.waitFor((m) => m.type === 'segment');
    assert.equal(fake.requests.length, 1);
    const system = fake.requests[0].messages[0].content;
    assert.match(system, /预算 → 予算/);
    assert.match(system, /议事录 → 議事録/);
  } finally {
    conn.ws.close();
    await server.close();
    await new Promise((resolve) => fake.http.close(resolve));
  }
});

test('ACCESS_TOKEN rejects a missing or wrong token without echoing it', async () => {
  const token = 'fake-token-not-real';
  const { server, port } = await startServer({ ACCESS_TOKEN: token });
  try {
    const missing = await handshake(`ws://127.0.0.1:${port}/ws`);
    assert.equal(missing.status, 401);
    assert.ok(!missing.body.includes(token));

    const wrong = await handshake(`ws://127.0.0.1:${port}/ws?token=wrong-token`);
    assert.equal(wrong.status, 401);
    assert.ok(!wrong.body.includes(token));

    const conn = connect(`ws://127.0.0.1:${port}/ws?token=${encodeURIComponent(token)}`);
    const hello = await conn.waitFor((m) => m.type === 'hello');
    assert.equal(hello.type, 'hello');
    conn.ws.close();
  } finally {
    await server.close();
  }
});

test('a missing ACCESS_TOKEN prints one startup warning naming it', async () => {
  const without = await startServer();
  const withToken = await startServer({ ACCESS_TOKEN: 'fake-token-not-real' });
  try {
    const warning = without.warnings.filter((line) => line.includes('ACCESS_TOKEN'));
    assert.equal(warning.length, 1);
    assert.match(warning[0], /warning/);
    assert.ok(!withToken.warnings.some((line) => line.includes('ACCESS_TOKEN')));
  } finally {
    await without.server.close();
    await withToken.server.close();
  }
});

test('hello reports the configured preset and carries no key material', async () => {
  const apiKey = 'fake-secret-key-not-real';
  const { server, port } = await startServer({
    LLM_PRESET: 'gemini',
    LLM_API_KEY: apiKey,
    LLM_MODEL: 'fake-model',
  });
  const conn = connect(`ws://127.0.0.1:${port}/ws`);
  try {
    const hello = await conn.waitFor((m) => m.type === 'hello');
    assert.deepEqual(hello.presetsInUse, [{ id: 'gemini', trainsOnData: true }]);
    const serialized = JSON.stringify(hello);
    assert.ok(!serialized.includes(apiKey), 'hello must not carry the API key');
    assert.doesNotMatch(serialized, /api[-_]?key|authorization|bearer|secret/i);
  } finally {
    conn.ws.close();
    await server.close();
  }
});
