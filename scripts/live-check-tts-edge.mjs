#!/usr/bin/env bun
// Keyless live check of the Edge read-aloud adapter. It is unofficial and may
// break at any time. Audio evidence lands in gitignored tmp/.
//
//   bun scripts/live-check-tts-edge.mjs

import { mkdir, writeFile } from 'node:fs/promises';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

import { create } from '../core/providers/tts-edge.js';

const ROOT = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
const OUT = path.join(ROOT, 'tmp');
await mkdir(OUT, { recursive: true });

const cases = [
  { lang: 'ja', text: 'こんにちは。今日はいい天気ですね。' },
  { lang: 'zh', text: '你好。今天天气很好。' },
];

let failed = false;
const tts = create();

for (const { lang, text } of cases) {
  const started = Date.now();
  try {
    const audio = await tts.synthesize({ text, seg_id: 1, direction: { src: 'en', tgt: lang } });
    const elapsed = Date.now() - started;
    const file = path.join(OUT, `edge-live-${lang}.mp3`);
    await writeFile(file, audio.data);
    const sync = audio.data.length > 1 && audio.data[0] === 0xff && (audio.data[1] & 0xe0) === 0xe0;
    console.log(
      `${lang}: format=${audio.format} bytes=${audio.data.length} ms=${elapsed} ` +
        `mp3_sync=${sync} file=${file}`,
    );
    if (audio.format !== 'mp3' || !sync) failed = true;
  } catch (error) {
    failed = true;
    console.error(`${lang}: FAILED after ${Date.now() - started} ms: ${error.message}`);
  }
}

process.exit(failed ? 1 : 0);
