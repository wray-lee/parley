#!/usr/bin/env bun
// Re-records the frozen Session Traces under spec/traces/. Run only for an
// intentional JavaScript bug fix; see spec/traces/README.md.

import { spawnSync } from 'node:child_process';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const ROOT = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
const result = spawnSync(
  process.execPath,
  ['test', 'tests/session.test.js', 'tests/converse.test.js', '-t', 'Session Trace'],
  {
    cwd: ROOT,
    env: { ...process.env, PARLEY_RECORD_TRACES: '1' },
    stdio: 'inherit',
  },
);
process.exit(result.status ?? 1);
