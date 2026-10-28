#!/usr/bin/env node
// Live acceptance runner for ticket 20. Three subcommands:
//
//   node scripts/acceptance.mjs demo
//     Seam 1 replay of samples/zh-numbers.json through real Session + mock
//     Providers: T3 number fidelity and the T1 timing fields.
//
//   node scripts/acceptance.mjs live --url ws://127.0.0.1:3000/ws \
//     --sample samples/zh-numbers.wav --src zh --tgt ja [--packs conference] \
//     [--expect 127.5,42] [--token X] [--insecure] [--out tmp/t20/events.json]
//     Replays a WAV as real-time 16 kHz PCM into a running Server Host and
//     records every event. Judge the server log afterwards with `judge`.
//
//   node scripts/acceptance.mjs judge <server.log> [--require-segment] [--t5]
//     Reads the host's JSON-line Segment log, prints T1 deltas and a T5/T7
//     verdict aid. T1 is asr_final_at -> tts_first_chunk; the speech_end and
//     first_token deltas are printed alongside for comparison.

import { mkdir, readFile, readdir, writeFile } from 'node:fs/promises';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { WebSocket } from 'ws';

import { createRegistry } from '../core/packs-registry.js';
import { createSession } from '../core/session.js';
import { create as createMockAsr } from '../core/providers/asr-mock.js';
import { create as createMockTranslator } from '../core/providers/translator-mock.js';
import { create as createMockTts } from '../core/providers/tts-mock.js';

const ROOT = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
const sleep = (ms) => new Promise((resolve) => setTimeout(resolve, ms));

function arg(name, fallback = '') {
  const index = process.argv.indexOf(`--${name}`);
  return index === -1 ? fallback : process.argv[index + 1] ?? fallback;
}

function assert(condition, message) {
  if (!condition) throw new Error(message);
}

// --- WAV / PCM -------------------------------------------------------------

function readWav(bytes) {
  assert(bytes.toString('ascii', 0, 4) === 'RIFF' && bytes.toString('ascii', 8, 12) === 'WAVE', 'not a WAV file');
  let offset = 12;
  let fmt = null;
  let data = null;
  while (offset + 8 <= bytes.length) {
    const id = bytes.toString('ascii', offset, offset + 4);
    const size = bytes.readUInt32LE(offset + 4);
    const body = offset + 8;
    if (id === 'fmt ') {
      fmt = {
        channels: bytes.readUInt16LE(body + 2),
        sampleRate: bytes.readUInt32LE(body + 4),
        bits: bytes.readUInt16LE(body + 14),
      };
    } else if (id === 'data') {
      data = bytes.subarray(body, body + size);
    }
    offset = body + size + (size % 2);
  }
  assert(fmt && data, 'WAV has no fmt/data chunk');
  assert(fmt.channels === 1 && fmt.bits === 16, `need mono PCM16, got ${fmt.channels}ch/${fmt.bits}bit`);
  return { sampleRate: fmt.sampleRate, pcm: new Int16Array(data.buffer, data.byteOffset, data.length / 2) };
}

function resampleTo16k(pcm, from) {
  if (from === 16000) return pcm;
  const ratio = from / 16000;
  const out = new Int16Array(Math.floor(pcm.length / ratio));
  for (let i = 0; i < out.length; i++) {
    const at = i * ratio;
    const left = Math.floor(at);
    const right = Math.min(left + 1, pcm.length - 1);
    const mix = pcm[left] + (pcm[right] - pcm[left]) * (at - left);
    out[i] = Math.max(-32768, Math.min(32767, Math.round(mix)));
  }
  return out;
}

function rms(frame) {
  let sum = 0;
  for (let i = 0; i < frame.length; i++) sum += (frame[i] / 32768) ** 2;
  return Math.sqrt(sum / frame.length);
}

function frames(pcm, ms = 20) {
  const size = (16000 * ms) / 1000;
  const list = [];
  for (let at = 0; at < pcm.length; at += size) {
    const chunk = pcm.subarray(at, Math.min(at + size, pcm.length));
    const padded = new Int16Array(size);
    padded.set(chunk);
    list.push(padded);
  }
  return list;
}

// WhisperLiveKit's pause segmentation needs roughly 0.7 s of silence; the
// generated samples have 0.7-0.9 s gaps and the 0.7 s one can land under the
// threshold after resampling. `--min-silence 1200` stretches every silent run
// to at least that long before streaming; the WAV itself is untouched.
function stretchSilence(list, minMs, minRunMs = 300) {
  if (!(minMs > 0)) return list;
  const out = [];
  let run = [];
  // Only a real pause (300 ms+) between speech stretches; the short silences
  // inside a word must stay short or the ASR hears chopped audio.
  const flush = (stretch) => {
    if (!run.length) return;
    out.push(...run);
    if (stretch && run.length * 20 >= minRunMs) {
      const needed = Math.ceil(minMs / 20) - run.length;
      for (let i = 0; i < needed; i++) out.push(new Int16Array(run[0].length));
    }
    run = [];
  };
  for (const frame of list) {
    if (rms(frame) <= 0.01) {
      run.push(frame);
    } else {
      flush(true);
      out.push(frame);
    }
  }
  flush(false);
  return out;
}

// A tone under each sample `audio` span, silence elsewhere; the mock ASR only
// uses audioMs, the cut policy needs real energy to see speech and silence.
function demoPcm(sample) {
  const pcm = new Int16Array((sample.duration_ms * 16000) / 1000);
  for (const span of sample.audio ?? []) {
    for (let i = Math.floor((span.start_ms * 16000) / 1000); i < (span.end_ms * 16000) / 1000; i++) {
      pcm[i] = Math.round(Math.sin((2 * Math.PI * 220 * i) / 16000) * (span.amplitude ?? 0.2) * 32767);
    }
  }
  return pcm;
}

async function registryFromDisk() {
  const promptFiles = (await readdir(path.join(ROOT, 'prompts')))
    .filter((name) => name.startsWith('system_') && name.endsWith('.txt'))
    .sort();
  const prompts = [];
  for (const name of promptFiles) {
    prompts.push({ name, text: await readFile(path.join(ROOT, 'prompts', name), 'utf8') });
  }
  const packFiles = (await readdir(path.join(ROOT, 'packs'))).filter((name) => name.endsWith('.tsv')).sort();
  const packs = [];
  for (const name of packFiles) {
    packs.push({ name, text: await readFile(path.join(ROOT, 'packs', name), 'utf8') });
  }
  return createRegistry({ promptFiles: prompts, packFiles: packs, warn: (line) => console.warn(`warn: ${line}`) });
}

// --- demo (Seam 1) ---------------------------------------------------------

async function demo() {
  const sample = JSON.parse(await readFile(path.join(ROOT, 'samples', 'zh-numbers.json'), 'utf8'));
  const registry = await registryFromDisk();
  const pairConfig = JSON.parse(await readFile(path.join(ROOT, 'config', 'pairs.json'), 'utf8'));
  const records = [];
  const events = [];
  const session = createSession({
    providers: {
      asr: createMockAsr({ sample }),
      translator: createMockTranslator({ references: sample.reference_translations }),
      tts: createMockTts(),
    },
    clock: { now: () => Date.now() },
    registry,
    pairConfig,
    emit: (event) => events.push(event),
    log: (record) => records.push(record),
  });

  session.handleMessage({ type: 'start', mode: 'listen', src: sample.src, tgt: sample.tgt });
  session.handleMessage({ type: 'sample' });
  for (const frame of frames(demoPcm(sample))) session.handleAudio(frame);
  for (let i = 0; i < 30; i++) session.handleAudio(new Int16Array(320)); // 600 ms silence
  await session.idle();

  const segments = events.filter((event) => event.type === 'segment');
  const src = segments.map((event) => event.src).join(' ');
  const tgt = segments.map((event) => event.tgt).join(' ');
  assert(segments.length >= 2, `expected >= 2 Segments, got ${segments.length}`);
  assert(src.includes('127.5') && src.includes('42'), `T3 FAIL source lost a number: ${src}`);
  assert(tgt.includes('127.5') && tgt.includes('42'), `T3 FAIL translation lost a number: ${tgt}`);

  console.log(`ok - T3 demo: ${segments.length} Segments, 127.5 and 42 verbatim in source and translation`);
  console.log(`     src: ${src}`);
  console.log(`     tgt: ${tgt}`);
  for (const record of records) {
    console.log(
      `     seg ${record.seg_id} ${record.direction?.src}->${record.direction?.tgt} ` +
        `speech_end->tts=${record.tts_first_chunk - record.speech_end} ms ` +
        `first_token->tts=${record.tts_first_chunk - record.first_token} ms`,
    );
  }
  const out = path.join(ROOT, 'tmp', 't20', 'demo.json');
  await mkdir(path.dirname(out), { recursive: true });
  await writeFile(out, JSON.stringify({ sample: sample.id, segments, records }, null, 2));
  console.log(`ok - demo evidence ${out}`);
}

// --- live (running host) ---------------------------------------------------

function connect(url, insecure) {
  return new Promise((resolve, reject) => {
    const ws = new WebSocket(url, insecure ? { rejectUnauthorized: false } : undefined);
    ws.once('open', () => resolve(ws));
    ws.once('error', reject);
  });
}

async function live() {
  const url = arg('url');
  const sampleFile = arg('sample', 'samples/zh-numbers.wav');
  const src = arg('src', 'zh');
  const tgt = arg('tgt', 'ja');
  const packs = arg('packs') ? arg('packs').split(',').map((name) => name.trim()) : [];
  const expect = arg('expect') ? arg('expect').split(',').map((token) => token.trim()) : [];
  const token = arg('token');
  const insecure = process.argv.includes('--insecure');
  const out = arg('out', path.join('tmp', 't20', `live-${src}-${tgt}.json`));
  assert(url, 'live needs --url ws://host:port/ws');

  const wav = readWav(await readFile(path.resolve(sampleFile)));
  const pcm = resampleTo16k(wav.pcm, wav.sampleRate);
  const framesList = stretchSilence(frames(pcm), Number(arg('min-silence', '0')));

  const events = [];
  let binary = [];
  const audio = [];
  let currentTts = null;
  let lastSpeechAt = 0;
  const open = await connect(token ? `${url}?token=${encodeURIComponent(token)}` : url, insecure);
  open.on('message', (data, isBinary) => {
    if (isBinary) {
      binary.push({ at: Date.now(), bytes: data.length, seg_id: currentTts });
      audio.push(Buffer.from(data));
      return;
    }
    let message;
    try {
      message = JSON.parse(data.toString());
    } catch {
      return;
    }
    if (message.type === 'tts_begin') currentTts = message.seg_id;
    if (message.type === 'tts_end') currentTts = null;
    events.push({ at: Date.now(), ...message });
  });

  const hello = waitForEvent(events, (event) => event.type === 'hello', 10_000);
  const helloEvent = await hello;
  console.log(`hello demo=${helloEvent.demo} pairs=${JSON.stringify(helloEvent.pairs)}`);
  open.send(JSON.stringify({ type: 'start', mode: 'listen', src, tgt, packs }));

  const start = Date.now();
  let index = 0;
  for (const frame of framesList) {
    const deadline = start + index * 20;
    await sleep(Math.max(0, deadline - Date.now()));
    if (rms(frame) > 0.01) lastSpeechAt = Date.now();
    open.send(Buffer.from(frame.buffer));
    index++;
  }
  console.log(`played ${framesList.length * 20} ms of ${sampleFile} in ${Date.now() - start} ms`);

  // Silence lets the VAD and WhisperLiveKit's pause segmentation commit.
  const silenceUntil = Date.now() + 2000;
  while (Date.now() < silenceUntil) {
    open.send(Buffer.from(new Int16Array(320).buffer));
    await sleep(20);
  }

  const settleMs = Number(arg('settle', '5000'));
  const timeoutAt = Date.now() + Number(arg('timeout', '45000'));
  let lastAt = Date.now();
  while (Date.now() < timeoutAt) {
    open.send(Buffer.from(new Int16Array(320).buffer));
    await sleep(20);
    if (events.length) lastAt = events.at(-1).at;
    if (events.some((event) => event.type === 'segment') && Date.now() - lastAt >= settleMs) break;
  }
  open.send(JSON.stringify({ type: 'stop' }));
  await sleep(500);
  open.close();

  const segments = events.filter((event) => event.type === 'segment');
  const errors = events.filter((event) => event.type === 'error');
  const text = segments.map((event) => `${event.src} ${event.tgt}`).join(' ');
  const verdict = [];
  if (expect.length) {
    const missing = expect.filter((item) => !text.includes(item));
    verdict.push(missing.length ? `FAIL expect missing: ${missing.join(', ')}` : `PASS expect ${expect.join(', ')}`);
    if (missing.length) process.exitCode = 1;
  }
  for (const segment of segments) {
    const first = binary.find((chunk) => chunk.seg_id === segment.seg_id && chunk.at >= segment.at);
    console.log(
      `segment ${segment.seg_id} latency_ms=${segment.latency_ms} ` +
        `segment_event->first_audio=${first ? first.at - segment.at : 'none'} ms ` +
        `src=${JSON.stringify(segment.src)} tgt=${JSON.stringify(segment.tgt)}`,
    );
  }
  for (const error of errors) console.log(`error ${JSON.stringify(error)}`);
  for (const line of verdict) console.log(line);
  const result = { sample: sampleFile, src, tgt, packs, segments, errors, binary, events, lastSpeechAt };
  const file = path.resolve(out);
  await mkdir(path.dirname(file), { recursive: true });
  await writeFile(file, JSON.stringify(result, null, 2));
  const audioFile = arg('save-audio');
  if (audioFile) {
    const bytes = Buffer.concat(audio);
    await writeFile(path.resolve(audioFile), bytes);
    console.log(`saved TTS audio ${path.resolve(audioFile)} bytes=${bytes.length}`);
  }
  console.log(`events ${events.length}, TTS chunks ${binary.length}, TTS bytes ${binary.reduce((sum, c) => sum + c.bytes, 0)}`);
  console.log(`ok - live evidence ${file}`);
  if (!segments.length) {
    console.log('FAIL no Segments');
    process.exitCode = 1;
  }
}

function waitForEvent(events, predicate, timeoutMs) {
  return new Promise((resolve, reject) => {
    const started = Date.now();
    const timer = setInterval(() => {
      const found = events.find(predicate);
      if (found) {
        clearInterval(timer);
        resolve(found);
      } else if (Date.now() - started > timeoutMs) {
        clearInterval(timer);
        reject(new Error(`timed out after ${timeoutMs} ms waiting for event`));
      }
    }, 25);
  });
}

// --- judge (host log) ------------------------------------------------------

async function judge() {
  const file = process.argv[3];
  assert(file, 'judge needs a log file');
  const lines = (await readFile(path.resolve(file), 'utf8')).split(/\r?\n/).filter(Boolean);
  const records = [];
  const other = [];
  for (const line of lines) {
    try {
      const parsed = JSON.parse(line);
      if (parsed && Object.hasOwn(parsed, 'seg_id')) records.push(parsed);
      else other.push(line);
    } catch {
      other.push(line);
    }
  }
  console.log(`records ${records.length}, non-JSON lines ${other.length}`);
  for (const line of other) console.log(`log: ${line}`);

  for (const record of records) {
    const endToTts = record.tts_first_chunk - record.speech_end;
    const tokenToTts = record.tts_first_chunk - record.first_token;
    const finalToTts = record.tts_first_chunk - record.asr_final_at;
    console.log(
      `seg ${record.seg_id} ${record.direction?.src}->${record.direction?.tgt} ` +
        `asr_final_at->tts=${Number.isFinite(finalToTts) ? finalToTts : 'null'} ms ` +
        `speech_end->tts=${Number.isFinite(endToTts) ? endToTts : 'null'} ms ` +
        `first_token->tts=${Number.isFinite(tokenToTts) ? tokenToTts : 'null'} ms ` +
        `held=${record.held} merges=${record.merges} forced=${record.forced} ` +
        `forced_fail=${record.forced_fail} echo_suspect=${record.echo_suspect} ` +
        `src=${JSON.stringify(record.asr_final)}`,
    );
  }

  const ttsTimes = records.filter((record) => typeof record.tts_first_chunk === 'number');
  if (ttsTimes.length) {
    const endToTts = ttsTimes.map((record) => record.tts_first_chunk - record.speech_end);
    const tokenToTts = ttsTimes.map((record) => record.tts_first_chunk - record.first_token);
    console.log(`T1 aid speech_end->tts_first_chunk: ${endToTts.join(', ')} ms`);
    console.log(`T1 aid first_token->tts_first_chunk: ${tokenToTts.join(', ')} ms`);
  } else {
    console.log('T1 PENDING: no Segment with a numeric tts_first_chunk');
  }

  const t1Times = records.filter(
    (record) => typeof record.tts_first_chunk === 'number' && typeof record.asr_final_at === 'number',
  );
  if (t1Times.length) {
    const finalToTts = t1Times.map((record) => record.tts_first_chunk - record.asr_final_at);
    console.log(`T1 asr_final_at->tts_first_chunk: ${finalToTts.join(', ')} ms`);
    if (finalToTts.some((value) => value > 2500)) {
      console.log('T1 FAIL: an asr_final_at->tts_first_chunk value exceeds 2500 ms');
      process.exitCode = 1;
    } else {
      console.log('T1 PASS for the measured asr_final_at->tts_first_chunk values');
    }
  } else if (ttsTimes.length) {
    console.log('T1 PENDING: records have no numeric asr_final_at (pre-follow-up log?)');
  }

  if (records.length) {
    const suspects = records.reduce((sum, record) => sum + (record.echo_suspect ?? 0), 0);
    console.log(`T5 aid: committed Segments ${records.length}, echo_suspect total ${suspects}`);
    console.log('T5 aid: compare each src above with the translation you heard; any matching src is a committed echo.');
  }
  if (process.argv.includes('--require-segment') && !records.length) {
    console.log('FAIL: no Segment records');
    process.exitCode = 1;
  }
}

const command = process.argv[2];
if (command === 'demo') await demo();
else if (command === 'live') await live();
else if (command === 'judge') await judge();
else {
  console.error('usage: node scripts/acceptance.mjs demo|live|judge ...');
  process.exitCode = 2;
}
