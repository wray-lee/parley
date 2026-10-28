// Generates the bundled sample audio with Kokoro and writes one WAV plus one
// sidecar JSON per sample. Kokoro must be listening (deploy/compose.kokoro.yaml):
//   docker compose -f deploy/compose.kokoro.yaml up -d
//   node scripts/generate-samples.mjs
// The sidecar's timed transcript is computed from the synthesised clause
// boundaries, so it stays aligned with the audio without manual timing.

import { createHash } from 'node:crypto';
import { readFile, writeFile } from 'node:fs/promises';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const ROOT = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
const SAMPLES = path.join(ROOT, 'samples');
const KOKORO_URL = (process.env.KOKORO_URL || 'http://127.0.0.1:8880').replace(/\/+$/, '');
const SILENCE_THRESHOLD = 328; // 1% of full scale; trims Kokoro's lead-in/lead-out

const manifest = JSON.parse(await readFile(path.join(SAMPLES, 'manifest.json'), 'utf8'));

for (const sample of manifest.samples) {
  const clips = [];
  for (const clause of sample.clauses) {
    const clip = await synthesize(sample, clause.text);
    clips.push({ ...clause, pcm: trimSilence(clip.pcm), sampleRate: clip.sampleRate });
  }

  const sampleRate = clips[0].sampleRate;
  const placed = [];
  let cursor = 0;
  for (const [index, clip] of clips.entries()) {
    if (clip.sampleRate !== sampleRate) {
      throw new Error(`${sample.id}: Kokoro returned mixed sample rates`);
    }
    const start = cursor;
    const end = start + clip.pcm.length;
    placed.push({
      ...clip,
      startSample: start,
      start_ms: ms(start, sampleRate),
      end_ms: ms(end, sampleRate),
    });
    cursor = end;
    const pauseMs = index === clips.length - 1 ? sample.trailing_ms : clip.pause_after_ms;
    cursor += Math.round((pauseMs * sampleRate) / 1000);
  }

  const pcm = new Int16Array(cursor);
  for (const clip of placed) pcm.set(clip.pcm, clip.startSample);

  const id = sample.id;
  const wavName = `${id}.wav`;
  await writeFile(path.join(SAMPLES, wavName), wav(pcm, sampleRate));

  const references = {};
  for (const clip of placed) references[clip.text] = clip.translation;
  Object.assign(references, sample.references ?? {});

  const sidecar = {
    id,
    src: sample.src,
    tgt: sample.tgt,
    audio_file: wavName,
    duration_ms: ms(pcm.length, sampleRate),
    audio: placed.map((clip) => ({
      start_ms: clip.start_ms,
      end_ms: clip.end_ms,
      amplitude: 0.2,
    })),
    timed_transcript: placed.map((clip) => ({
      start_ms: clip.start_ms,
      end_ms: clip.end_ms,
      text: clip.text,
    })),
    reference_translations: references,
  };
  await writeFile(path.join(SAMPLES, `${id}.json`), `${JSON.stringify(sidecar, null, 2)}\n`);

  check(sidecar, pcm.length, sampleRate);
  console.log(
    `ok ${id} ${sample.src}->${sample.tgt} ${sidecar.duration_ms} ms ` +
      `${sha256(await readFile(path.join(SAMPLES, wavName)))} ${wavName}`,
  );
}

function ms(samples, sampleRate) {
  return Math.round((samples * 1000) / sampleRate);
}

async function synthesize(sample, text) {
  const response = await fetch(`${KOKORO_URL}/v1/audio/speech`, {
    method: 'POST',
    headers: { 'content-type': 'application/json' },
    body: JSON.stringify({
      model: 'kokoro',
      input: text,
      voice: sample.voice,
      response_format: 'wav',
      stream: false,
    }),
  });
  if (!response.ok) {
    const detail = await response.text();
    throw new Error(
      `Kokoro at ${KOKORO_URL} refused ${JSON.stringify(text)} (${response.status}): ${detail}`,
    );
  }
  const body = Buffer.from(await response.arrayBuffer());
  return parseWav(body, sample.id);
}

function parseWav(buffer, id) {
  if (buffer.toString('ascii', 0, 4) !== 'RIFF' || buffer.toString('ascii', 8, 12) !== 'WAVE') {
    throw new Error(`${id}: Kokoro did not return a RIFF/WAVE body`);
  }
  let offset = 12;
  let format = null;
  let data = null;
  while (offset + 8 <= buffer.length) {
    const chunk = buffer.toString('ascii', offset, offset + 4);
    const size = buffer.readUInt32LE(offset + 4);
    const body = offset + 8;
    if (chunk === 'fmt ') {
      format = {
        encoding: buffer.readUInt16LE(body),
        channels: buffer.readUInt16LE(body + 2),
        sampleRate: buffer.readUInt32LE(body + 4),
        bits: buffer.readUInt16LE(body + 14),
      };
    } else if (chunk === 'data') {
      data = buffer.subarray(body, body + size);
      break;
    }
    offset = body + size + (size % 2);
  }
  if (!format || !data) throw new Error(`${id}: Kokoro WAV has no fmt or data chunk`);
  if (format.encoding !== 1 || format.channels !== 1 || format.bits !== 16) {
    throw new Error(`${id}: expected mono 16-bit PCM, got ${JSON.stringify(format)}`);
  }
  const pcm = new Int16Array(data.length / 2);
  for (let i = 0; i < pcm.length; i++) pcm[i] = data.readInt16LE(i * 2);
  return { sampleRate: format.sampleRate, pcm };
}

function trimSilence(pcm) {
  let first = 0;
  let last = pcm.length - 1;
  while (first <= last && Math.abs(pcm[first]) <= SILENCE_THRESHOLD) first++;
  while (last >= first && Math.abs(pcm[last]) <= SILENCE_THRESHOLD) last--;
  if (first > last) throw new Error('Kokoro returned silence');
  return pcm.subarray(first, last + 1);
}

function wav(pcm, sampleRate) {
  const dataBytes = pcm.length * 2;
  const buffer = Buffer.alloc(44 + dataBytes);
  buffer.write('RIFF', 0, 'ascii');
  buffer.writeUInt32LE(36 + dataBytes, 4);
  buffer.write('WAVE', 8, 'ascii');
  buffer.write('fmt ', 12, 'ascii');
  buffer.writeUInt32LE(16, 16);
  buffer.writeUInt16LE(1, 20);
  buffer.writeUInt16LE(1, 22);
  buffer.writeUInt32LE(sampleRate, 24);
  buffer.writeUInt32LE(sampleRate * 2, 28);
  buffer.writeUInt16LE(2, 32);
  buffer.writeUInt16LE(16, 34);
  buffer.write('data', 36, 'ascii');
  buffer.writeUInt32LE(dataBytes, 40);
  for (let i = 0; i < pcm.length; i++) buffer.writeInt16LE(pcm[i], 44 + i * 2);
  return buffer;
}

function sha256(buffer) {
  return createHash('sha256').update(buffer).digest('hex');
}

function check(sidecar, totalSamples, sampleRate) {
  if (Math.abs(ms(totalSamples, sampleRate) - sidecar.duration_ms) > 1) {
    throw new Error(`${sidecar.id}: duration mismatch`);
  }
  let previous = 0;
  for (const entry of sidecar.timed_transcript) {
    if (!entry.text || entry.start_ms < previous || entry.end_ms <= entry.start_ms) {
      throw new Error(`${sidecar.id}: bad timed transcript ${JSON.stringify(entry)}`);
    }
    previous = entry.end_ms;
  }
  if (sidecar.timed_transcript.at(-1).end_ms > sidecar.duration_ms) {
    throw new Error(`${sidecar.id}: transcript ends after the audio`);
  }
  for (const text of sidecar.timed_transcript.map((entry) => entry.text)) {
    if (!sidecar.reference_translations[text]) {
      throw new Error(`${sidecar.id}: no reference translation for ${JSON.stringify(text)}`);
    }
  }
}
