#!/usr/bin/env bun
// PENDING-USER live check for the real translator. It needs a real key and
// never writes the key anywhere: the result lands in gitignored .scratch/.
//
//   export LLM_PRESET=groq
//   export LLM_API_KEY=...            # paste in your shell; never commit it
//   bun scripts/live-check-translator.mjs
//
// With a local .env instead (gitignored):
//   bun scripts/live-check-translator.mjs

import { mkdir, readFile, writeFile } from 'node:fs/promises';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

import { PRESETS } from '../config/presets.js';
import { create } from '../core/providers/translator-openai.js';
import { buildTranslatorRequest } from '../core/translator-request.js';

const ROOT = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
const PRESET = process.env.LLM_PRESET || 'groq';
const API_KEY = process.env.LLM_API_KEY || '';

const preset = PRESETS[PRESET];
if (!preset) {
  console.error(`unknown LLM_PRESET="${PRESET}"; known presets: ${Object.keys(PRESETS).join(', ')}`);
  process.exit(2);
}
if (preset.requiresKey && !API_KEY) {
  console.error(
    [
      `LLM_API_KEY is not set (preset "${PRESET}" needs a key).`,
      '',
      'Exact steps:',
      '  1. export LLM_PRESET=groq',
      '  2. export LLM_API_KEY=<your key>   # paste in the shell; never commit it',
      '  3. bun scripts/live-check-translator.mjs',
      '',
      'Or put both variables in .env (gitignored) and run:',
      '  bun scripts/live-check-translator.mjs',
    ].join('\n'),
  );
  process.exit(2);
}

const prompt = await readFile(path.join(ROOT, 'prompts', 'system_zh2ja.txt'), 'utf8');
const text = '今天天气很好。';
const { system, user } = buildTranslatorRequest({
  prompt,
  context: [{ src: '你好', tgt: 'こんにちは' }],
  text,
});

try {
  const translator = create({
    preset: PRESET,
    baseUrl: process.env.LLM_BASE_URL || '',
    apiKey: API_KEY,
    model: process.env.LLM_MODEL || '',
    extraBody: process.env.LLM_EXTRA_BODY || '',
  });

  const started = Date.now();
  let firstTokenMs = null;
  let output = '';
  for await (const chunk of translator.translate({ system, user, text, direction: { src: 'zh', tgt: 'ja' } })) {
    if (firstTokenMs === null) firstTokenMs = Date.now() - started;
    output += chunk;
  }

  const result = {
    checked_at: new Date().toISOString(),
    preset: translator.preset,
    model: translator.model,
    base_url: translator.baseUrl,
    trains_on_data: translator.trainsOnData,
    docs: preset.docs,
    direction: 'zh->ja',
    system_prompt_file: 'prompts/system_zh2ja.txt',
    input: text,
    output,
    first_token_ms: firstTokenMs,
    total_ms: Date.now() - started,
    note: 'Live check result. Contains no API key; .scratch/ is gitignored.',
  };

  const target = path.join(ROOT, '.scratch', 'live-check-translator.json');
  await mkdir(path.dirname(target), { recursive: true });
  await writeFile(target, `${JSON.stringify(result, null, 2)}\n`, 'utf8');
  console.log(JSON.stringify(result, null, 2));
  console.error(`Result written to ${target} (gitignored).`);
} catch (error) {
  console.error(`LIVE CHECK FAILED [${error.kind ?? 'unknown'}]: ${error.message}`);
  if (error.fallback) console.error(`Fallback: ${error.fallback}`);
  console.error(`Preset docs: ${preset.docs}`);
  process.exitCode = 1;
}
