// Seam 1 Session Trace recorder and replayer. Format: spec/traces/README.md.
// The recorder mutates the fake clock, Providers and Session it is attached to
// so an ordinary `build()` call records the exact inputs and outputs it saw.

import assert from 'node:assert/strict';
import { mkdirSync, readFileSync, writeFileSync } from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

import { createSession } from '../../core/session.js';
import { makeClock, pcmFrame } from './fakes.js';

export const TRACE_FORMAT = 2;
export const TRACE_DIR = fileURLToPath(new URL('../../spec/traces/', import.meta.url));

const base64 = (data) => Buffer.from(data).toString('base64');
const replacer = (key, value) =>
  value instanceof Uint8Array ? { $b64: base64(value) } : value;

// Provider errors are inputs, not outputs: the fake Provider throws the same
// error the JavaScript adapter did on the recorded run.
const providerError = (provider, error, call) => ({
  kind: 'provider_error',
  provider,
  ...(call === undefined ? {} : { call }),
  error: {
    message: error?.message ?? String(error),
    ...(error?.kind ? { kind: error.kind } : {}),
    ...(error?.fallback ? { fallback: error.fallback } : {}),
  },
});

const errorFrom = (spec) => {
  const error = new Error(spec.message);
  if (spec.kind) error.kind = spec.kind;
  if (spec.fallback) error.fallback = spec.fallback;
  return error;
};

export function toJsonValue(value) {
  return JSON.parse(JSON.stringify(value, replacer));
}

function fromJsonValue(value) {
  if (Array.isArray(value)) return value.map(fromJsonValue);
  if (value && typeof value === 'object') {
    if (typeof value.$b64 === 'string') return new Uint8Array(Buffer.from(value.$b64, 'base64'));
    for (const key of Object.keys(value)) value[key] = fromJsonValue(value[key]);
  }
  return value;
}

// The wire protocol fixes PCM16 at 16 kHz, so a frame round-trips as the
// amplitude the tests generated it with.
function frameSpec(pcm) {
  let peak = 0;
  for (let i = 0; i < pcm.length; i++) peak = Math.max(peak, Math.abs(pcm[i]));
  return { ms: pcm.length / 16, amplitude: peak / 32767 };
}

export function createTraceRecorder({
  name,
  clock,
  asr,
  translator,
  tts,
  registry,
  pairConfig = {},
  forcedInstruction = '',
}) {
  const lines = [
    {
      type: 'meta',
      format: TRACE_FORMAT,
      name,
      clockStart: clock.now(),
      registry,
      pairConfig,
      forcedInstruction,
      providers: {
        asr: { name: asr.name, acceptsLive: asr.acceptsLive ?? true, revisable: asr.revisable === true },
        translator: { name: translator.name },
        tts: { name: tts.name },
      },
    },
  ];
  const input = (record) => lines.push({ type: 'input', ...record });
  const output = (kind, payload) => lines.push({ type: 'output', kind, ...payload });

  const advance = clock.advance.bind(clock);
  clock.advance = (ms) => {
    input({ kind: 'advance', ms });
    return advance(ms);
  };

  if (asr.push) {
    const push = asr.push.bind(asr);
    asr.push = (frame) => {
      try {
        const events = push(frame);
        input({ kind: 'asr', events: events ?? [] });
        return events;
      } catch (error) {
        input(providerError('asr', error));
        throw error;
      }
    };
  }

  const translate = translator.translate.bind(translator);
  let call = 0;
  translator.translate = async function* (request) {
    const id = call++;
    output('request', { provider: 'translator', request });
    try {
      for await (const chunk of translate(request)) {
        input({ kind: 'translator', call: id, chunk });
        yield chunk;
      }
    } catch (error) {
      input(providerError('translator', error, id));
      throw error;
    }
  };

  if (tts.synthesize) {
    const synthesize = tts.synthesize.bind(tts);
    tts.synthesize = async (request) => {
      output('request', { provider: 'tts', request });
      try {
        const response = await synthesize(request);
        input({ kind: 'tts', response });
        return response;
      } catch (error) {
        input(providerError('tts', error));
        throw error;
      }
    };
  }

  function attachSession(session) {
    const handleMessage = session.handleMessage;
    session.handleMessage = (message) => {
      input({ kind: 'client', message });
      return handleMessage(message);
    };
    const handleAudio = session.handleAudio;
    session.handleAudio = (pcm) => {
      input({ kind: 'audio', ...frameSpec(pcm) });
      return handleAudio(pcm);
    };
    const idle = session.idle;
    session.idle = () => {
      input({ kind: 'idle' });
      return idle();
    };
  }

  return {
    name,
    attachSession,
    output,
    finish: () => `${lines.map((line) => JSON.stringify(line, replacer)).join('\n')}\n`,
  };
}

export function checkTrace(trace) {
  const file = path.join(TRACE_DIR, `${trace.name}.jsonl`);
  const actual = trace.finish();
  if (process.env.PARLEY_RECORD_TRACES === '1') {
    mkdirSync(TRACE_DIR, { recursive: true });
    writeFileSync(file, actual);
    return;
  }
  let expected;
  try {
    expected = readFileSync(file, 'utf8').replace(/\r\n/g, '\n');
  } catch {
    assert.fail(`missing Session Trace ${trace.name}.jsonl; run bun run traces:record`);
  }
  assert.equal(
    actual,
    expected,
    `Session Trace ${trace.name}.jsonl is stale; run bun run traces:record`,
  );
}

export async function replayTrace(text) {
  const records = text.trim().split(/\r?\n/).map((line) => JSON.parse(line));
  const [meta] = records;
  assert.equal(meta.type, 'meta', 'trace must start with a meta record');
  assert.equal(meta.format, TRACE_FORMAT, `unsupported trace format ${meta.format}`);
  const inputs = records.filter((record) => record.type === 'input');
  const expected = records
    .filter((record) => record.type === 'output')
    .map(({ type, ...record }) => record);

  // Provider calls consume tts/asr responses and provider_error inputs in
  // trace order; a translator call's chunks may end in an error.
  const asrEntries = [];
  const ttsEntries = [];
  const translatorCalls = [];
  for (const entry of inputs) {
    if (entry.kind === 'asr') asrEntries.push({ events: entry.events });
    else if (entry.kind === 'tts') ttsEntries.push({ response: fromJsonValue(entry.response) });
    else if (entry.kind === 'translator') {
      (translatorCalls[entry.call] ??= []).push({ chunk: entry.chunk });
    } else if (entry.kind === 'provider_error') {
      const failed = { error: errorFrom(entry.error) };
      if (entry.provider === 'asr') asrEntries.push(failed);
      else if (entry.provider === 'tts') ttsEntries.push(failed);
      else (translatorCalls[entry.call] ??= []).push(failed);
    }
  }

  const clock = makeClock(meta.clockStart);
  const outputs = [];
  const push = (kind, payload) => outputs.push({ kind, ...payload });

  const asr = {
    name: meta.providers.asr.name,
    acceptsLive: meta.providers.asr.acceptsLive,
    revisable: meta.providers.asr.revisable,
    open() {},
    close() {},
    push() {
      const entry = asrEntries.shift() ?? { events: [] };
      if (entry.error) throw entry.error;
      return entry.events;
    },
  };
  let call = 0;
  const translator = {
    name: meta.providers.translator.name,
    async *translate(request) {
      push('request', { provider: 'translator', request: toJsonValue(request) });
      for (const entry of translatorCalls[call++] ?? []) {
        if (entry.error) throw entry.error;
        yield entry.chunk;
      }
    },
  };
  const tts = {
    name: meta.providers.tts.name,
    async synthesize(request) {
      push('request', { provider: 'tts', request: toJsonValue(request) });
      const entry = ttsEntries.shift() ?? {};
      if (entry.error) throw entry.error;
      return entry.response;
    },
  };

  const session = createSession({
    providers: { asr, translator, tts },
    clock,
    registry: meta.registry,
    pairConfig: meta.pairConfig,
    forcedInstruction: meta.forcedInstruction,
    emit: (event) => push('event', { event: toJsonValue(event) }),
    log: (record) => push('log', { record: toJsonValue(record) }),
  });

  for (const entry of inputs) {
    if (entry.kind === 'client') session.handleMessage(entry.message);
    else if (entry.kind === 'audio') session.handleAudio(pcmFrame(entry.ms, entry.amplitude));
    else if (entry.kind === 'advance') clock.advance(entry.ms);
    else if (entry.kind === 'idle') await session.idle();
  }

  assert.deepEqual(outputs, expected, `trace ${meta.name}: replayed outputs differ`);
  return { meta, outputs };
}
