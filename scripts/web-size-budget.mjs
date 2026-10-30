// Web release gate: a cold load must be <= 3.5 MiB brotli at quality 11,
// fonts included. The Flutter bundle ships one renderer per browser plus
// lazily-loaded pieces, so the gate counts what one cold app start fetches:
// the entry files, main.dart.js, the largest CanvasKit pair, fonts and the
// assets manifest. Alternate renderers, debug symbols, source maps, PWA icons
// and the TTS engine (loaded on the first playback) are not cold-load bytes.
//
// Usage: node scripts/web-size-budget.mjs [DIR] [BUDGET_MIB]
import { existsSync, readFileSync, readdirSync } from 'node:fs';
import { join, relative, sep } from 'node:path';
import { brotliCompressSync, constants } from 'node:zlib';

const dir = process.argv[2] ?? 'client/build/web';
const budgetMiB = Number(process.argv[3] ?? process.env.WEB_BROTLI_BUDGET_MIB ?? 3.5);
if (!Number.isFinite(budgetMiB) || budgetMiB <= 0) {
  console.error(`invalid budget ${process.argv[3] ?? process.env.WEB_BROTLI_BUDGET_MIB}: expected a positive number of MiB`);
  process.exit(2);
}

const notColdLoad = [
  /^canvaskit\/(chromium|webparagraph|skwasm|wimp)/,
  /\.symbols$/,
  /\.map$/,
  /^icons\//,
  /^favicon\.png$/,
  /^assets\/NOTICES$/,
  /^assets\/packages\/flutter_soloud\//,
];

const required = ['index.html', 'flutter_bootstrap.js', 'main.dart.js', 'canvaskit/canvaskit.wasm'];
const missing = required.filter((name) => !existsSync(join(dir, name)));
if (missing.length > 0) {
  console.error(`missing from ${dir}: ${missing.join(', ')}`);
  process.exit(1);
}

/** @type {Array<[number, string]>} */
const rows = [];
let total = 0;
const walk = (current) => {
  for (const entry of readdirSync(current, { withFileTypes: true })) {
    const path = join(current, entry.name);
    if (entry.isDirectory()) {
      walk(path);
      continue;
    }
    const name = relative(dir, path).split(sep).join('/');
    if (notColdLoad.some((pattern) => pattern.test(name))) continue;
    const size = brotliCompressSync(readFileSync(path), {
      params: { [constants.BROTLI_PARAM_QUALITY]: 11 },
    }).length;
    rows.push([size, name]);
    total += size;
  }
};
walk(dir);

rows.sort((a, b) => b[0] - a[0]);
for (const [size, name] of rows.slice(0, 8)) {
  console.log(`${(size / 1024).toFixed(0).padStart(7)} KiB  ${name}`);
}
const mib = total / 1024 / 1024;
console.log(`cold-load brotli total ${mib.toFixed(2)} MiB across ${rows.length} files (budget ${budgetMiB} MiB)`);
if (mib > budgetMiB) {
  console.error(`over budget by ${(mib - budgetMiB).toFixed(2)} MiB`);
  process.exit(1);
}
