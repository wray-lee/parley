// Black-box Conformance Suite (Seam 3), skeleton. Every case talks only the
// WebSocket protocol, so the same cases can run against the Bun Server Host
// today, the Go Server Host in CI and the Device Host loopback later.
// See spec/DIVERGENCES.md for the hello differences this suite tolerates.

import { WebSocket } from 'ws';

const MESSAGE_MS = 10_000;

function assert(condition, message) {
  if (!condition) throw new Error(message);
}

function pcmFrame(amplitude, ms = 20) {
  const samples = (16000 * ms) / 1000;
  const pcm = new Int16Array(samples);
  if (amplitude) {
    for (let i = 0; i < samples; i++) {
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
    if (isBinary) {
      messages.push({ type: 'binary', bytes: data.length });
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
  const opened = new Promise((resolve, reject) => {
    ws.once('open', resolve);
    ws.once('error', reject);
  });
  return {
    ws,
    messages,
    opened,
    send: (message) => ws.send(JSON.stringify(message)),
    sendBinary: (data) => ws.send(data),
    close: () => ws.close(),
    waitFor(predicate, label, timeoutMs = MESSAGE_MS) {
      const found = messages.find(predicate);
      if (found) return Promise.resolve(found);
      return new Promise((resolve, reject) => {
        const waiter = { predicate, resolve };
        waiters.push(waiter);
        setTimeout(() => {
          const index = waiters.indexOf(waiter);
          if (index !== -1) waiters.splice(index, 1);
          reject(new Error(`timed out waiting for ${label}; got ${JSON.stringify(messages)}`));
        }, timeoutMs);
      });
    },
  };
}

// Runs every case against a Demo Mode host and returns the case names.
export async function runConformance({ url, log = () => {} }) {
  const conn = connect(url);
  const passed = [];
  const ok = (name) => {
    passed.push(name);
    log(`ok - ${name}`);
  };

  try {
    await conn.opened;
    const hello = await conn.waitFor((message) => message.type === 'hello', 'hello');
    assert(hello.demo === true, 'conformance expects Demo Mode (mock Providers)');
    const pair = (hello.pairs ?? []).find(
      (candidate) => [candidate.a, candidate.b].sort().join('-') === 'ja-zh',
    );
    assert(pair?.converse === true, 'hello must expose the converse-capable zh/ja pair');
    assert(Array.isArray(hello.packs), 'hello must list packs');
    if ('protocol' in hello) assert(hello.protocol === 1, `hello.protocol: ${hello.protocol}`);
    if ('tts' in hello) assert(['system', 'audio'].includes(hello.tts), `hello.tts: ${hello.tts}`);
    ok('hello');

    conn.send({ type: 'start', mode: 'listen', src: 'zh', tgt: 'ja' });
    const state = await conn.waitFor((message) => message.type === 'state', 'state');
    assert(state.state === 'listening', `unexpected state ${state.state}`);
    ok('start listen');

    conn.send({ type: 'sample' });
    // The bundled sample drives the mock ASR: 1.6 s of speech, then 0.5 s of
    // silence so the VAD Cut fires.
    for (let i = 0; i < 80; i++) conn.sendBinary(pcmFrame(0.2));
    for (let i = 0; i < 25; i++) conn.sendBinary(pcmFrame(0));
    const segment = await conn.waitFor((message) => message.type === 'segment', 'segment');
    assert(Number.isInteger(segment.seg_id) && segment.seg_id > 0, 'seg_id must be a positive integer');
    assert(typeof segment.src === 'string' && segment.src.length > 0, 'segment.src');
    assert(typeof segment.tgt === 'string' && segment.tgt.length > 0, 'segment.tgt');
    assert(segment.direction?.src === 'zh' && segment.direction?.tgt === 'ja', 'direction');
    assert(segment.demo === true, 'demo Segment must be DEMO-labelled');
    assert(segment.speak === true, 'Listen must speak its Direction');
    ok('demo segment');

    await conn.waitFor(
      (message) => message.type === 'tts_end' && message.seg_id === segment.seg_id,
      'tts_end',
    );
    assert(conn.messages.some((message) => message.type === 'tts_begin'), 'tts_begin');
    assert(conn.messages.some((message) => message.type === 'binary'), 'binary TTS chunk');
    ok('tts ordering');

    const copies = conn.messages.filter(
      (message) => message.type === 'segment' && message.seg_id === segment.seg_id,
    );
    assert(copies.length === 1, 'a committed Segment is never revised');
    ok('commit immutability');

    conn.send({ type: 'start', mode: 'listen', src: 'zh', tgt: 'xx' });
    const error = await conn.waitFor((message) => message.type === 'error', 'error');
    assert(error.kind === 'protocol', `unexpected error kind ${error.kind}`);
    ok('unknown direction error');
  } finally {
    conn.close();
  }

  return passed;
}
