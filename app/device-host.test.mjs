// Self-check for the Device Host adapter: assemble the real frontend, boot the
// in-page socket, drive the bundled sample, and require a DEMO-labelled Segment
// plus a Segment log.

import test from 'node:test';
import assert from 'node:assert/strict';
import { execFileSync } from 'node:child_process';
import { mkdtemp, readFile, rm } from 'node:fs/promises';
import os from 'node:os';
import path from 'node:path';
import { fileURLToPath, pathToFileURL } from 'node:url';

const APP = path.dirname(fileURLToPath(import.meta.url));
const DIST = await mkdtemp(path.join(os.tmpdir(), 'parley-dist-'));

execFileSync(process.execPath, [path.join(APP, 'build-frontend.mjs')], {
  stdio: 'ignore',
  env: { ...process.env, PARLEY_DIST: DIST },
});

globalThis.fetch = async (url) => {
  const pathname = String(url).replace(/^https?:\/\/[^/]+/, '').split('?')[0];
  try {
    return new Response(await readFile(path.join(DIST, pathname)));
  } catch {
    return new Response('not found', { status: 404 });
  }
};

await import(pathToFileURL(path.join(DIST, 'device-host.js')).href);

const sample = JSON.parse(await readFile(path.join(DIST, 'samples', 'placeholder.json'), 'utf8'));

function connect() {
  const ws = new globalThis.WebSocket('ws://device-host/ws');
  const messages = [];
  ws.addEventListener('message', (event) => {
    messages.push(typeof event.data === 'string' ? JSON.parse(event.data) : { type: 'binary' });
  });
  return { ws, messages };
}

async function waitFor(predicate, timeoutMs = 5000) {
  const started = Date.now();
  while (!predicate()) {
    if (Date.now() - started > timeoutMs) throw new Error('timed out');
    await new Promise((resolve) => setTimeout(resolve, 20));
  }
}

test('Device Host runs the bundled sample to a DEMO Segment', async (t) => {
  t.after(() => rm(DIST, { recursive: true, force: true }));
  const { ws, messages } = connect();
  await waitFor(() => messages.some((message) => message.type === 'hello'));
  const hello = messages.find((message) => message.type === 'hello');
  assert.equal(hello.demo, true);
  assert.ok(
    hello.pairs.some((pair) =>
      pair.directions.some((direction) => direction.src === sample.src && direction.tgt === sample.tgt),
    ),
  );

  ws.send(JSON.stringify({ type: 'start', mode: 'listen', src: sample.src, tgt: sample.tgt, packs: [] }));
  ws.send(JSON.stringify({ type: 'sample', name: sample.id }));

  const chunkMs = 100;
  const chunkSamples = (16000 * chunkMs) / 1000;
  for (let offset = 0; offset < sample.duration_ms; offset += chunkMs) {
    const pcm = new Int16Array(chunkSamples);
    const amplitude =
      sample.audio.find((span) => offset >= span.start_ms && offset < span.end_ms)?.amplitude ?? 0;
    for (let i = 0; i < pcm.length && amplitude; i++) {
      pcm[i] = Math.round(Math.sin((2 * Math.PI * 220 * (offset + i / 16)) / 1000) * amplitude * 32767);
    }
    ws.send(pcm.buffer);
    await new Promise((resolve) => setImmediate(resolve));
  }

  await waitFor(() => messages.some((message) => message.type === 'segment'));
  const segment = messages.find((message) => message.type === 'segment');
  assert.match(segment.tgt, /^\[DEMO\] /);
  assert.ok(messages.some((message) => message.type === 'binary'), 'TTS chunk is delivered');
  assert.equal(globalThis.__parleyLog?.[0]?.seg_id, 1);
  ws.close();
});
