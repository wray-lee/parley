// Keyless Demo Mode smoke: branch like a clean clone (no .env, no Provider
// keys), boot the real Server Host, drive the bundled sample over the real
// WebSocket protocol, and fail unless every Segment is DEMO-labelled.

import { spawn } from 'node:child_process';
import { createServer as createNetServer } from 'node:net';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { WebSocket } from 'ws';

const ROOT = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
const STARTUP_MS = 30_000;
const MESSAGE_MS = 10_000;

// Every environment key that could take the Server Host out of Demo Mode.
const PROVIDER_KEYS = [
  'ACCESS_TOKEN', 'PUBLIC_DOMAIN',
  'ASR_PROVIDER', 'TTS_PROVIDER',
  'LLM_PRESET', 'LLM_BASE_URL', 'LLM_API_KEY', 'LLM_MODEL', 'LLM_EXTRA_BODY',
  'WHISPERLIVEKIT_URL', 'DEEPGRAM_API_KEY', 'DASHSCOPE_API_KEY',
  'KOKORO_URL', 'AZURE_SPEECH_KEY', 'AZURE_SPEECH_REGION',
];

function ok(line) {
  console.log(`ok - ${line}`);
}

function assert(condition, message) {
  if (!condition) throw new Error(message);
}

function freePort() {
  return new Promise((resolve, reject) => {
    const probe = createNetServer();
    probe.once('error', reject);
    probe.listen(0, '127.0.0.1', () => {
      const { port } = probe.address();
      probe.close(() => resolve(port));
    });
  });
}

function demoEnv(port) {
  const env = { ...process.env };
  for (const key of PROVIDER_KEYS) delete env[key];
  env.PORT = String(port);
  return env;
}

function pcmFrame(amplitude, ms = 20) {
  const pcm = new Int16Array((16000 * ms) / 1000);
  if (amplitude) {
    for (let i = 0; i < pcm.length; i++) {
      pcm[i] = Math.round(Math.sin((2 * Math.PI * 220 * i) / 16000) * amplitude * 32767);
    }
  }
  return Buffer.from(pcm.buffer);
}

function collect(ws) {
  const messages = [];
  const waiters = [];
  let binaryChunks = 0;
  ws.on('message', (data, isBinary) => {
    const message = isBinary ? { type: 'binary', bytes: data.length } : JSON.parse(data.toString());
    if (isBinary) binaryChunks++;
    messages.push(message);
    for (const waiter of [...waiters]) {
      if (waiter.predicate(message)) {
        waiters.splice(waiters.indexOf(waiter), 1);
        waiter.resolve(message);
      }
    }
  });
  return {
    messages,
    get binaryChunks() { return binaryChunks; },
    waitFor(predicate, label) {
      const found = messages.find(predicate);
      if (found) return Promise.resolve(found);
      return new Promise((resolve, reject) => {
        const waiter = { predicate, resolve };
        waiters.push(waiter);
        setTimeout(() => {
          const i = waiters.indexOf(waiter);
          if (i !== -1) waiters.splice(i, 1);
          reject(new Error(`timed out waiting for ${label}; got ${JSON.stringify(messages)}`));
        }, MESSAGE_MS);
      });
    },
  };
}

const port = await freePort();
const startedAt = Date.now();
const server = spawn(process.execPath, ['hosts/node/server.js'], {
  cwd: ROOT,
  env: demoEnv(port),
  stdio: ['ignore', 'pipe', 'pipe'],
});
const serverExit = new Promise((resolve) => server.once('exit', resolve));

let stdout = '';
let stderr = '';
let exited = null;
server.stdout.on('data', (data) => { stdout += data; });
server.stderr.on('data', (data) => { stderr += data; });
server.on('exit', (code) => { exited = code ?? 1; });

let ws;
try {
  for (;;) {
    if (exited !== null) throw new Error(`server exited (${exited})\n${stdout}${stderr}`);
    if (stdout.includes('Parley listening')) break;
    if (Date.now() > startedAt + STARTUP_MS) {
      throw new Error(`server did not listen in ${STARTUP_MS} ms\n${stdout}${stderr}`);
    }
    await new Promise((resolve) => setTimeout(resolve, 50));
  }
  ok(`keyless server listening on port ${port} (no .env, no Provider keys)`);

  const health = await fetch(`http://127.0.0.1:${port}/healthz`).then((res) => res.json());
  assert(health?.ok === true, `healthz failed: ${JSON.stringify(health)}`);
  ok(`healthz ${JSON.stringify(health)}`);

  ws = new WebSocket(`ws://127.0.0.1:${port}/ws`);
  await new Promise((resolve, reject) => {
    ws.once('open', resolve);
    ws.once('error', reject);
  });
  const conn = collect(ws);

  const hello = await conn.waitFor((m) => m.type === 'hello', 'hello');
  assert(hello.demo === true, `hello.demo must be true in Demo Mode: ${JSON.stringify(hello)}`);
  const pair = hello.pairs?.find((p) => [p.a, p.b].includes('zh') && [p.a, p.b].includes('ja'));
  assert(pair, `hello did not list the zh/ja pair: ${JSON.stringify(hello.pairs)}`);
  ok(`hello demo=true pairs=${hello.pairs.map((p) => `${p.a}<->${p.b}`).join(',')}`);

  ws.send(JSON.stringify({ type: 'start', src: 'zh', tgt: 'ja', mode: 'listen' }));
  await conn.waitFor((m) => m.type === 'state' && m.state === 'listening', 'listening state');
  ws.send(JSON.stringify({ type: 'sample' }));

  // 1.6 s of speech (mock ASR emits the stable transcript) then 0.5 s of
  // silence so the VAD Cut fires.
  for (let i = 0; i < 80; i++) ws.send(pcmFrame(0.2));
  for (let i = 0; i < 25; i++) ws.send(pcmFrame(0));

  const segment = await conn.waitFor((m) => m.type === 'segment', 'segment');
  await conn.waitFor((m) => m.type === 'tts_end', 'tts_end');

  const segments = conn.messages.filter((m) => m.type === 'segment');
  for (const item of segments) {
    assert(item.demo === true, `segment ${item.seg_id} is not DEMO-labelled: ${JSON.stringify(item)}`);
    assert(
      typeof item.tgt === 'string' && item.tgt.startsWith('[DEMO] '),
      `segment ${item.seg_id} lacks the [DEMO] marker: ${JSON.stringify(item.tgt)}`,
    );
  }
  assert(conn.binaryChunks > 0, 'no TTS audio chunks arrived');
  ok(`segment seg_id=${segment.seg_id} src=${JSON.stringify(segment.src)} tgt=${JSON.stringify(segment.tgt)} demo=${segment.demo}`);
  ok(`tts audio chunks=${conn.binaryChunks}; all ${segments.length} segment(s) DEMO-labelled`);

  // The Server Host logs one JSON record per Segment, just after tts_end.
  const logDeadline = Date.now() + 2000;
  while (!/"seg_id":\s*1/.test(stdout)) {
    assert(Date.now() < logDeadline, `no Segment log record on stdout\n${stdout}`);
    await new Promise((resolve) => setTimeout(resolve, 20));
  }
  ok('segment log record written to stdout');

  console.log('demo smoke passed');
} catch (error) {
  console.error(`demo smoke failed: ${error.message}`);
  process.exitCode = 1;
} finally {
  ws?.terminate();
  server.kill();
  await serverExit;
}
