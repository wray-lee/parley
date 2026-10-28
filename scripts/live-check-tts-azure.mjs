#!/usr/bin/env node
// PENDING-USER live check for Azure Speech TTS. It needs a real F0 key and
// never writes the key anywhere: the audio evidence lands in gitignored tmp/.
//
//   export AZURE_SPEECH_KEY=...
//   export AZURE_SPEECH_REGION=japaneast
//   node scripts/live-check-tts-azure.mjs
//
// With a local .env instead (gitignored):
//   node --env-file-if-exists=.env scripts/live-check-tts-azure.mjs

import { mkdir, writeFile } from 'node:fs/promises';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

import { loadConfig } from '../hosts/node/config.js';
import { create } from '../core/providers/tts-azure.js';

const ROOT = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
const config = loadConfig(process.env);

if (!config.ttsOptions.azureKey || !config.ttsOptions.azureRegion) {
  console.error(
    [
      'AZURE_SPEECH_KEY and AZURE_SPEECH_REGION are not both set.',
      '',
      'Exact steps:',
      '  1. Create a Speech resource (F0 free tier) in the Azure portal.',
      '  2. export AZURE_SPEECH_KEY=<key 1 or key 2 from the resource>',
      '  3. export AZURE_SPEECH_REGION=<resource region, e.g. japaneast>',
      '  4. node scripts/live-check-tts-azure.mjs',
      '',
      'Or put both variables in .env (gitignored) and run:',
      '  node --env-file-if-exists=.env scripts/live-check-tts-azure.mjs',
    ].join('\n'),
  );
  process.exit(2);
}

const OUT = path.join(ROOT, 'tmp');
await mkdir(OUT, { recursive: true });

const cases = [
  { lang: 'ja', text: 'こんにちは。今日はいい天気ですね。' },
  { lang: 'zh', text: '你好。今天天气很好。' },
];

let failed = false;
const tts = create(config.ttsOptions);

for (const { lang, text } of cases) {
  const started = Date.now();
  try {
    const audio = await tts.synthesize({ text, seg_id: 1, direction: { src: 'en', tgt: lang } });
    const elapsed = Date.now() - started;
    const file = path.join(OUT, `azure-live-${lang}.mp3`);
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
