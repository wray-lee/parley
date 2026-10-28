#!/usr/bin/env bun
// Live check for the real Deepgram ASR. It needs a real key and a speech WAV
// file, and it never writes the key anywhere: the result lands in gitignored
// .scratch/live-check-deepgram.json.
//
//   export DEEPGRAM_API_KEY=...          # paste in your shell; never commit it
//   bun scripts/live-check-asr.mjs samples/live-check.wav ja
//
// With a local .env instead (gitignored):
//   bun scripts/live-check-asr.mjs samples/live-check.wav ja
//
// The WAV must be 16 kHz mono 16-bit PCM, because the adapter streams raw PCM:
//   ffmpeg -i input.m4a -ar 16000 -ac 1 -c:a pcm_s16le samples/live-check.wav
//
// It uses the ws package on purpose: unlike the browser/undici WebSocket it
// exposes the HTTP handshake status, so a 401/402 is reported as auth/quota
// instead of a generic close, and the close event, so the check can wait for
// Deepgram to flush every final instead of guessing with a sleep.

import { mkdir, readFile, writeFile } from 'node:fs/promises';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { WebSocket } from 'ws';

import { create } from '../core/providers/asr-deepgram.js';

const ROOT = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
const SCRATCH = path.join(ROOT, '.scratch');
const API_KEY = process.env.DEEPGRAM_API_KEY || '';
const FILE = path.resolve(process.argv[2] || path.join(ROOT, 'samples', 'live-check.wav'));
const LANGUAGE = process.argv[3] || 'zh';

const sleep = (ms) => new Promise((resolve) => setTimeout(resolve, ms));

function exactSteps() {
  return [
    'Exact steps:',
    '  1. export DEEPGRAM_API_KEY=<your key>   # paste in the shell; never commit it',
    '  2. ffmpeg -i input.m4a -ar 16000 -ac 1 -c:a pcm_s16le samples/live-check.wav',
    `  3. bun scripts/live-check-asr.mjs samples/live-check.wav ${LANGUAGE}`,
    '',
    'Or put DEEPGRAM_API_KEY in .env (gitignored) and run:',
    `  bun scripts/live-check-asr.mjs samples/live-check.wav ${LANGUAGE}`,
  ].join('\n');
}

// Minimal RIFF/WAVE reader: returns the raw PCM data chunk.
async function readWav(file) {
  const buffer = await readFile(file);
  if (buffer.toString('ascii', 0, 4) !== 'RIFF' || buffer.toString('ascii', 8, 12) !== 'WAVE') {
    throw new Error(`${file} is not a RIFF/WAVE file`);
  }
  let format = null;
  let offset = 12;
  while (offset + 8 <= buffer.length) {
    const id = buffer.toString('ascii', offset, offset + 4);
    const size = buffer.readUInt32LE(offset + 4);
    const body = offset + 8;
    if (id === 'fmt ') {
      format = {
        encoding: buffer.readUInt16LE(body),
        channels: buffer.readUInt16LE(body + 2),
        sampleRate: buffer.readUInt32LE(body + 4),
        bits: buffer.readUInt16LE(body + 14),
      };
    } else if (id === 'data') {
      if (!format) throw new Error('WAV has a data chunk before its fmt chunk');
      if (format.encoding !== 1 || format.channels !== 1 || format.sampleRate !== 16000 || format.bits !== 16) {
        throw new Error(
          `WAV is ${format.sampleRate} Hz / ${format.channels} ch / ${format.bits} bit; ` +
            'the adapter streams 16 kHz mono 16-bit PCM. Convert it with the ffmpeg command above.',
        );
      }
      return buffer.subarray(body, Math.min(body + size, buffer.length));
    }
    offset = body + size + (size % 2);
  }
  throw new Error(`${file} has no fmt or data chunk`);
}

if (!API_KEY) {
  console.error(['DEEPGRAM_API_KEY is not set (this live check cannot be faked).', '', exactSteps()].join('\n'));
  process.exit(2);
}

let pcm;
try {
  pcm = await readWav(FILE);
} catch (error) {
  console.error([error.message, '', exactSteps()].join('\n'));
  process.exit(2);
}

let socketGone = false;
class LiveSocket extends WebSocket {
  constructor(url, init) {
    super(url, init);
    this.once('close', () => {
      socketGone = true;
    });
  }
}

const events = [];
const asr = create({ apiKey: API_KEY, WebSocket: LiveSocket });
function drain() {
  events.push(...asr.push({ pcm: new Int16Array(0) }));
}

asr.open({ src: LANGUAGE });
const CHUNK = 3200; // 100 ms of 16 kHz mono PCM16
for (let offset = 0; offset < pcm.length; offset += CHUNK) {
  // Collect push()'s own return: events delivered since the last drain arrive
  // with this call and would be lost if it were ignored.
  events.push(...asr.push({ pcm: pcm.subarray(offset, offset + CHUNK) }));
  await sleep(100); // real time: streaming faster makes Deepgram lag behind
}
drain();
asr.close();
// CloseStream makes Deepgram emit the final Results and close the socket;
// keep draining until it does, so no flushed final is left behind.
const deadline = Date.now() + 15000;
while (!socketGone && Date.now() < deadline) {
  await sleep(50);
  drain();
}
drain();

// Mirror the Session: each Stable Prefix event carries the provider's spacing.
const stable = events.filter((event) => event.type === 'stable');
const stableText = stable.reduce(
  (text, event) => (text ? `${text}${event.glue ?? ' '}${event.text}` : event.text),
  '',
);
const errors = events.filter((event) => event.type === 'error');
const report = {
  checked_at: new Date().toISOString(),
  file: path.relative(ROOT, FILE),
  language: LANGUAGE,
  model: 'nova-3',
  stable_text: stableText,
  partials: events.filter((event) => event.type === 'partial').length,
  errors,
};
await mkdir(SCRATCH, { recursive: true });
const reportFile = path.join(SCRATCH, 'live-check-deepgram.json');
// ASCII-only JSON: any reader, whatever its default encoding, sees escapes
// instead of mojibake; JSON.parse restores the transcript exactly.
const json = JSON.stringify(report, null, 2)
  .replace(/[\u007f-\uffff]/g, (char) => `\\u${char.charCodeAt(0).toString(16).padStart(4, '0')}`);
await writeFile(reportFile, `${json}\n`, 'utf8');
const roundTrip = JSON.parse(await readFile(reportFile, 'utf8'));
if (roundTrip.stable_text !== report.stable_text) {
  console.error('The report did not read back as written; check the file encoding.');
  process.exit(1);
}

if (errors.length > 0) {
  for (const error of errors) console.error(`${error.kind}: ${error.message}\n  ${error.fallback}`);
  console.error('\nReport: .scratch/live-check-deepgram.json');
  process.exit(1);
}
if (!stable.length) {
  console.error('No Stable Prefix came back. Is the sample speech, and 16 kHz mono PCM?');
  console.error('Report: .scratch/live-check-deepgram.json');
  process.exit(1);
}
console.log(`Stable text: ${report.stable_text}`);
console.log(`Partials: ${report.partials}. Report: .scratch/live-check-deepgram.json`);
