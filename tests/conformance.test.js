import { test } from 'node:test';
import assert from 'node:assert/strict';
import { spawn } from 'node:child_process';
import { createServer as createNetServer } from 'node:net';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

import { runConformance } from '../spec/conformance/cases.mjs';

const ROOT = path.resolve(import.meta.dirname, '..');
const STARTUP_MS = 30_000;

// Every environment key that could take the Server Host out of Demo Mode.
const PROVIDER_KEYS = [
  'ACCESS_TOKEN', 'PUBLIC_DOMAIN',
  'ASR_PROVIDER', 'TTS_PROVIDER',
  'LLM_PRESET', 'LLM_BASE_URL', 'LLM_API_KEY', 'LLM_MODEL', 'LLM_EXTRA_BODY',
  'WHISPERLIVEKIT_URL', 'DEEPGRAM_API_KEY', 'DASHSCOPE_API_KEY',
  'KOKORO_URL', 'AZURE_SPEECH_KEY', 'AZURE_SPEECH_REGION',
];

function freePort() {
  return new Promise((resolve, reject) => {
    const probe = createNetServer();
    probe.once('error', reject);
    probe.listen(0, '127.0.0.1', () => {
      const { port } = probe.address();
      probe.close(() => resolve(port));
    });
  });
}

test('the Conformance Suite runs against the Bun Server Host', { timeout: 30_000 }, async () => {
  const port = await freePort();
  const env = { ...process.env, PORT: String(port) };
  for (const key of PROVIDER_KEYS) delete env[key];
  const runtimeArgs = process.versions.bun ? ['--no-env-file'] : [];
  const server = spawn(process.execPath, [...runtimeArgs, 'hosts/node/server.js'], {
    cwd: ROOT,
    env,
    stdio: ['ignore', 'pipe', 'pipe'],
  });
  let stdout = '';
  let stderr = '';
  server.stdout.on('data', (data) => { stdout += data; });
  server.stderr.on('data', (data) => { stderr += data; });

  try {
    const startedAt = Date.now();
    while (!stdout.includes('Parley listening')) {
      if (server.exitCode !== null) throw new Error(`server exited\n${stdout}${stderr}`);
      if (Date.now() > startedAt + STARTUP_MS) throw new Error('server did not listen');
      await new Promise((resolve) => setTimeout(resolve, 50));
    }
    const passed = await runConformance({ url: `ws://127.0.0.1:${port}/ws` });
    assert.deepEqual(passed, [
      'hello',
      'start listen',
      'demo segment',
      'tts ordering',
      'commit immutability',
      'unknown direction error',
    ]);
  } finally {
    server.kill();
  }
});
