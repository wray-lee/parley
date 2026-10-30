import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync, readdirSync, statSync } from 'node:fs';
import { spawnSync } from 'node:child_process';
import path from 'node:path';

const root = path.resolve(import.meta.dirname, '..');
// Generated outputs (Flutter, Gradle, Tauri) are not project sources.
const SKIP = new Set(['node_modules', '.git', 'tmp', '.scratch', 'build', '.dart_tool']);

// Bun has no `--check`; its transpiler parses in-process. Node uses --check.
const transpiler = globalThis.Bun ? new Bun.Transpiler({ loader: 'js' }) : null;

function check(file) {
  if (!transpiler) return spawnSync(process.execPath, ['--check', file], { encoding: 'utf8' });
  try {
    transpiler.transformSync(readFileSync(file, 'utf8'));
    return { status: 0, stderr: '' };
  } catch (error) {
    return { status: 1, stderr: error.message };
  }
}

function walk(dir, out = []) {
  for (const entry of readdirSync(dir)) {
    if (SKIP.has(entry)) continue;
    const file = path.join(dir, entry);
    if (statSync(file).isDirectory()) walk(file, out);
    else if (/\.m?js$/.test(file)) out.push(file);
  }
  return out;
}

test('every JavaScript file parses (zero-dependency syntax check)', () => {
  const files = walk(root);
  assert.ok(files.length >= 10, `expected project sources, found ${files.length}`);
  for (const file of files) {
    const result = check(file);
    assert.equal(
      result.status,
      0,
      `${path.relative(root, file)} failed to parse:\n${result.stderr}`,
    );
  }
});
