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
    const text = readFileSync(path.join(TRACE_DIR, file), 'utf8');
    const meta = JSON.parse(text.slice(0, text.indexOf('\n')));
    // Go-recorded traces cover behaviour v0.1 adds; the frozen JS core cannot
    // reproduce them (see spec/DIVERGENCES.md and spec/traces/README.md).
    if (meta.oracle === 'go') continue;
    const replayed = await replayTrace(text);
    names.add(replayed.meta.name);
  }
  assert.ok(names.has('listen-basic'), 'a Listen trace is recorded');
  assert.ok(names.has('converse-turns'), 'a Converse trace is recorded');
});
