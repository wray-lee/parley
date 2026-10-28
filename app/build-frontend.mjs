// Builds the Tauri frontend by copying the static web client plus the core,
// prompts, packs and sample into app/dist. No bundler: files are served as-is.
// The only edit to the web client's HTML is adding the Device Host entry before
// main.js; web/ itself stays untouched.

import { cp, mkdir, readdir, readFile, rm, writeFile } from 'node:fs/promises';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const APP = path.dirname(fileURLToPath(import.meta.url));
const ROOT = path.resolve(APP, '..');
// The test runner uses PARLEY_DIST so it never mutates the source tree while a
// parallel build is tarring the repo.
const DIST = process.env.PARLEY_DIST
  ? path.resolve(process.env.PARLEY_DIST)
  : path.join(APP, 'dist');
const MAIN_SCRIPT = '<script type="module" src="/main.js"></script>';
const DEVICE_HOST_SCRIPT = '<script type="module" src="/device-host.js"></script>';

await rm(DIST, { recursive: true, force: true });
await mkdir(DIST, { recursive: true });

// The web client is served from the root, exactly as the Server Host serves it.
for (const entry of await readdir(path.join(ROOT, 'web'))) {
  await cp(path.join(ROOT, 'web', entry), path.join(DIST, entry), { recursive: true });
}
for (const dir of ['core', 'config', 'prompts', 'packs', 'samples', 'vendor']) {
  const source = dir === 'vendor' ? path.join(APP, dir) : path.join(ROOT, dir);
  await cp(source, path.join(DIST, dir), { recursive: true });
}
for (const entry of await readdir(APP)) {
  if (entry.endsWith('.js')) await cp(path.join(APP, entry), path.join(DIST, entry));
}

const assets = {
  prompts: (await readdir(path.join(ROOT, 'prompts'))).sort(),
  packs: (await readdir(path.join(ROOT, 'packs'))).sort(),
};
await writeFile(path.join(DIST, 'device-assets.json'), `${JSON.stringify(assets, null, 2)}\n`);

const html = await readFile(path.join(ROOT, 'web', 'index.html'), 'utf8');
if (!html.includes(MAIN_SCRIPT)) throw new Error('web/index.html: main.js script tag not found');
await writeFile(
  path.join(DIST, 'index.html'),
  html.replace(MAIN_SCRIPT, `${DEVICE_HOST_SCRIPT}\n${MAIN_SCRIPT}`),
);

console.log(`frontend: ${DIST} (${assets.prompts.length} prompts, ${assets.packs.length} packs)`);
