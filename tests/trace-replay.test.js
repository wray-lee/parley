import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync, readdirSync } from 'node:fs';
import path from 'node:path';

import { replayTrace, TRACE_DIR } from './helpers/trace.js';

test('every committed Session Trace replays to exactly its recorded outputs', async () => {
  const files = readdirSync(TRACE_DIR).filter((file) => file.endsWith('.jsonl')).sort();
  assert.ok(files.length >= 2, `expected recorded traces under ${TRACE_DIR}`);

  const names = new Set();
  for (const file of files) {
    const { meta } = await replayTrace(readFileSync(path.join(TRACE_DIR, file), 'utf8'));
    names.add(meta.name);
  }
  assert.ok(names.has('listen-basic'), 'a Listen trace is recorded');
  assert.ok(names.has('converse-turns'), 'a Converse trace is recorded');
});
