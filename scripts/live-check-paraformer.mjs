#!/usr/bin/env node
// PENDING-USER live check for Alibaba DashScope Paraformer real-time ASR. It
// needs a real Beijing-region key and a speech WAV; it never writes the key
// anywhere. The result lands in gitignored .scratch/live-check-paraformer.json.
//
//   export DASHSCOPE_API_KEY=...          # paste in your shell; never commit it
//   node scripts/live-check-paraformer.mjs samples/live-check.wav ja
//
// With a local .env instead (gitignored):
//   node --env-file-if-exists=.env scripts/live-check-paraformer.mjs samples/live-check.wav ja
//
// A workspace-specific endpoint may replace the default Beijing one:
//   export DASHSCOPE_URL=wss://<WorkspaceId>.cn-beijing.maas.aliyuncs.com/api-ws/v1/inference
//
// The WAV must be 16 kHz mono 16-bit PCM, because the adapter streams raw PCM:
//   ffmpeg -i input.m4a -ar 16000 -ac 1 -c:a pcm_s16le samples/live-check.wav
//
// Free quota: 10 h/month in the Beijing region (may change). The check uses the
// ws package on purpose: unlike the browser/undici WebSocket it exposes the HTTP
// handshake status, so a 401/403 is reported as auth instead of a generic close.

import { mkdir, readFile, writeFile } from 'node:fs/promises';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { WebSocket } from 'ws';

import { create } from '../core/providers/asr-paraformer.js';

const ROOT = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
const SCRATCH = path.join(ROOT, '.scratch');
const API_KEY = process.env.DASHSCOPE_API_KEY || '';
const URL = process.env.DASHSCOPE_URL || undefined;
const FILE = path.resolve(process.argv[2] || path.join(ROOT, 'samples', 'live-check.wav'));
const LANGUAGE = process.argv[3] || 'zh';

const sleep = (ms) => new Promise((resolve) => setTimeout(resolve, ms));

function exactSteps() {
  return [
    'Exact steps:',
    '  1. export DASHSCOPE_API_KEY=<Beijing-region key>   # paste in the shell; never commit it',
    '  2. ffmpeg -i input.m4a -ar 16000 -ac 1 -c:a pcm_s16le samples/live-check.wav',
    `  3. node scripts/live-check-paraformer.mjs samples/live-check.wav ${LANGUAGE}`,
    '',
    'Or put DASHSCOPE_API_KEY in .env (gitignored) and run:',
    `  node --env-file-if-exists=.env scripts/live-check-paraformer.mjs samples/live-check.wav ${LANGUAGE}`,
    '',
    'Paraformer is served only from the Beijing region; the free quota is',
    '10 h/month and may change. The check cannot be faked.',
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
      if (
        format.encoding !== 1 ||
        format.channels !== 1 ||
        format.sampleRate !== 16000 ||
        format.bits !== 16
      ) {
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
  console.error(
    ['DASHSCOPE_API_KEY is not set (this live check cannot be faked).', '', exactSteps()].join(
      '\n',
    ),
  );
  process.exit(2);
}

let pcm;
try {
  pcm = await readWav(FILE);
} catch (error) {
  console.error([error.message, '', exactSteps()].join('\n'));
  process.exit(2);
}

const events = [];
const options = URL ? { apiKey: API_KEY, url: URL, WebSocket } : { apiKey: API_KEY, WebSocket };
const asr = create(options);
function drain() {
  events.push(...asr.push({}));
}

asr.open({ src: LANGUAGE });
const CHUNK = 3200; // 100 ms of 16 kHz mono PCM16
for (let offset = 0; offset < pcm.length; offset += CHUNK) {
  asr.push({ pcm: pcm.subarray(offset, offset + CHUNK) });
  drain();
  await sleep(50);
}
await sleep(1000);
drain();
asr.close();
await sleep(1500);
drain();

const stable = events.filter((event) => event.type === 'stable').map((event) => event.text);
const errors = events.filter((event) => event.type === 'error');
const report = {
  checked_at: new Date().toISOString(),
  file: path.relative(ROOT, FILE),
  language: LANGUAGE,
  model: 'paraformer-realtime-v2',
  stable_text: stable.join(' '),
  partials: events.filter((event) => event.type === 'partial').length,
  errors,
};
await mkdir(SCRATCH, { recursive: true });
await writeFile(
  path.join(SCRATCH, 'live-check-paraformer.json'),
  JSON.stringify(report, null, 2),
);

if (errors.length > 0) {
  for (const error of errors) console.error(`${error.kind}: ${error.message}\n  ${error.fallback}`);
  console.error('\nReport: .scratch/live-check-paraformer.json');
  process.exit(1);
}
if (!stable.length) {
  console.error('No Stable Prefix came back. Is the sample speech, and 16 kHz mono PCM?');
  console.error('Report: .scratch/live-check-paraformer.json');
  process.exit(1);
}
console.log(`Stable text: ${report.stable_text}`);
console.log(`Partials: ${report.partials}. Report: .scratch/live-check-paraformer.json`);
