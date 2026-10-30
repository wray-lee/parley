// Mirror the Noto CJK fallback slices that the Flutter engine would otherwise
// download from fonts.gstatic.com into the release build, so the Go server
// serves them same-origin at /fonts/ (fontFallbackBaseUrl in
// client/web/flutter_bootstrap.js). Run after `flutter build web`.
//
// The URL list is read from the compiled engine, so it always matches the
// pinned Flutter version instead of a hand-maintained font list.
//
// Usage: node scripts/self-host-fallback-fonts.mjs [BUILD_DIR]
import { existsSync, mkdirSync, readFileSync, statSync, writeFileSync } from 'node:fs';
import { dirname, join } from 'node:path';

const dir = process.argv[2] ?? 'client/build/web';
const mainJs = join(dir, 'main.dart.js');
if (!existsSync(mainJs)) {
  console.error(`missing ${mainJs}; run flutter build web first`);
  process.exit(1);
}

const families =
  /(?:notosans(?:sc|tc|hk|jp|kr)|roboto)\/[A-Za-z0-9._/-]+\.woff2/g;
const paths = [...new Set(readFileSync(mainJs, 'utf8').match(families) ?? [])].sort();
if (paths.length === 0) {
  console.error(`no CJK fallback slices found in ${mainJs}`);
  process.exit(1);
}

const base = 'https://fonts.gstatic.com/s/';
const queue = [...paths];
let downloaded = 0;
let skipped = 0;
let bytes = 0;

async function fetchOne(path) {
  const target = join(dir, 'fonts', path);
  if (existsSync(target) && statSync(target).size > 0) {
    skipped++;
    bytes += statSync(target).size;
    return;
  }
  let lastError;
  for (let attempt = 0; attempt < 3; attempt++) {
    try {
      const response = await fetch(base + path);
      if (!response.ok) throw new Error(`HTTP ${response.status}`);
      const data = Buffer.from(await response.arrayBuffer());
      mkdirSync(dirname(target), { recursive: true });
      writeFileSync(target, data);
      downloaded++;
      bytes += data.length;
      return;
    } catch (error) {
      lastError = error;
      await new Promise((resolve) => setTimeout(resolve, 500 * (attempt + 1)));
    }
  }
  throw new Error(`${path}: ${lastError?.message ?? lastError}`);
}

const failures = [];
await Promise.all(
  Array.from({ length: 8 }, async () => {
    for (let path = queue.shift(); path !== undefined; path = queue.shift()) {
      try {
        await fetchOne(path);
      } catch (error) {
        failures.push(error.message);
      }
    }
  }),
);

if (failures.length > 0) {
  console.error(`could not mirror ${failures.length} fallback slices:\n${failures.join('\n')}`);
  process.exit(1);
}
console.log(
  `self-hosted ${downloaded} fallback slices (${skipped} already present) ` +
    `for ${new Set(paths.map((path) => path.split('/')[0])).size} families, ` +
    `${(bytes / 1024 / 1024).toFixed(1)} MiB under ${join(dir, 'fonts')}`,
);
