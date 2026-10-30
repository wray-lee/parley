// Release gate for the Flutter web client: serves the release build from the
// real Go Server Host on loopback, then drives headless Chrome through
// Playwright to check that
//   - `/` is the Flutter app and `/lite/` is still the vanilla client,
//   - every request is same-origin (CanvasKit, fonts and slices included),
//   - no request reaches fonts.gstatic.com / www.gstatic.com,
//   - the strict CSP header is present and produces no violations,
//   - the cold-load font URLs are recorded for the brotli size budget,
//   - first interactive time is measured under Fast 4G throttling.
//
// Usage: node scripts/web-release-gate.mjs [BUILD_DIR]
// Requires Go and a Chrome/Chromium binary (CHROME_PATH, Playwright's cache,
// or the system install). Fast 4G = 9 Mbit/s down, 2 Mbit/s up, 170 ms RTT.
import { execFileSync, spawn } from 'node:child_process';
import { existsSync, mkdtempSync, readdirSync, rmSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join, resolve } from 'node:path';
import { chromium } from 'playwright-core';

const buildDir = resolve(process.argv[2] ?? 'client/build/web');
if (!existsSync(join(buildDir, 'index.html'))) {
  console.error(`missing Flutter build at ${buildDir}`);
  process.exit(1);
}

function findChrome() {
  const candidates = [
    process.env.CHROME_PATH,
    chromium.executablePath(),
    join(process.env.LOCALAPPDATA ?? '', 'ms-playwright'),
    '/usr/bin/google-chrome',
    '/usr/bin/chromium',
    '/usr/bin/chromium-browser',
    '/Applications/Google Chrome.app/Contents/MacOS/Google Chrome',
    'C:/Program Files/Google/Chrome/Application/chrome.exe',
    'C:/Program Files (x86)/Microsoft/Edge/Application/msedge.exe',
  ];
  for (const candidate of candidates) {
    if (!candidate) continue;
    if (existsSync(candidate) && candidate.endsWith('.exe')) return candidate;
    if (existsSync(candidate) && candidate.startsWith('/')) return candidate;
    if (existsSync(candidate) && candidate.endsWith('Application/msedge.exe')) return candidate;
    if (candidate.endsWith('ms-playwright') && existsSync(candidate)) {
      for (const entry of readdirSync(candidate)) {
        if (!entry.startsWith('chromium-')) continue;
        for (const exe of ['chrome-win64/chrome.exe', 'chrome-win/chrome.exe', 'chrome-linux/chrome']) {
          const path = join(candidate, entry, exe);
          if (existsSync(path)) return path;
        }
      }
    }
  }
  throw new Error('no Chrome/Chromium found; set CHROME_PATH');
}

function startServer() {
  const temp = mkdtempSync(join(tmpdir(), 'parley-gate-'));
  const binary = join(temp, process.platform === 'win32' ? 'parley-server.exe' : 'parley-server');
  execFileSync('go', ['build', '-o', binary, './cmd/parley-server'], { stdio: 'inherit' });
  const env = {
    ...process.env,
    PORT: '0',
    HOST: '127.0.0.1',
    PARLEY_WEB_DIR: buildDir,
    // Deterministic Demo Mode regardless of the caller's shell.
    ACCESS_TOKEN: '',
    ASR_PROVIDER: '',
    TTS_PROVIDER: '',
    LLM_PRESET: '',
    LLM_BASE_URL: '',
    LLM_API_KEY: '',
    WHISPERLIVEKIT_URL: '',
    DEEPGRAM_API_KEY: '',
    DASHSCOPE_API_KEY: '',
    AZURE_SPEECH_KEY: '',
    KOKORO_URL: '',
  };
  const child = spawn(binary, [], { env, stdio: ['ignore', 'pipe', 'pipe'] });
  const logs = [];
  const ready = new Promise((resolvePromise, reject) => {
    let buffer = '';
    const onData = (chunk) => {
      buffer += chunk;
      logs.push(chunk.toString());
      const match = buffer.match(/Parley listening on http:\/\/localhost:(\d+)/);
      if (match) resolvePromise(Number(match[1]));
    };
    child.stdout.on('data', onData);
    child.stderr.on('data', (chunk) => logs.push(chunk.toString()));
    child.on('exit', (code) => reject(new Error(`server exited ${code}: ${logs.join('')}`)));
    setTimeout(() => reject(new Error(`server did not listen: ${logs.join('')}`)), 15000);
  });
  return { child, ready, temp, logs };
}

const failures = [];
const check = (ok, message) => {
  if (!ok) failures.push(message);
  console.log(`${ok ? 'ok  ' : 'FAIL'} ${message}`);
};

const { child, ready, temp, logs } = startServer();
let browser;
try {
  const port = await ready;
  const base = `http://127.0.0.1:${port}`;
  console.log(`serving ${buildDir} at ${base}`);

  browser = await chromium.launch({
    executablePath: findChrome(),
    headless: true,
    args: ['--no-sandbox', '--disable-dev-shm-usage'],
  });

  const context = await browser.newContext({
    locale: 'zh-CN',
    viewport: { width: 1280, height: 800 },
  });
  const page = await context.newPage();
  const requests = [];
  const consoleErrors = [];
  page.on('request', (request) => requests.push(request.url()));
  page.on('console', (message) => {
    if (message.type() === 'error') consoleErrors.push(message.text());
  });
  page.on('pageerror', (error) => consoleErrors.push(String(error)));
  await page.addInitScript(() => {
    window.__parleyFirstFrame = null;
    window.addEventListener('flutter-first-frame', () => {
      window.__parleyFirstFrame = performance.now();
    });
  });

  const appResponse = await page.goto(base + '/', { waitUntil: 'load', timeout: 60000 });
  const csp = appResponse?.headers()['content-security-policy'] ?? '';
  check(csp.includes("font-src 'self'"), 'CSP keeps fonts same-origin');
  check(csp.includes("connect-src 'self'"), 'CSP keeps connections same-origin');
  check(!csp.includes('gstatic'), 'CSP names no gstatic host');

  await page.waitForSelector('flutter-view', { timeout: 30000 });
  await page.waitForFunction(() => window.__parleyFirstFrame !== null, null, { timeout: 30000 });
  check(true, 'Flutter app rendered its first frame');

  // main.dart calls ensureSemantics(); without it the DOM only has the
  // "enable accessibility" placeholder and VoiceOver/TalkBack read nothing.
  const semantics = await page.evaluate(() => ({
    host: document.querySelector('flt-semantics-host') !== null,
    placeholder: document.querySelector('flt-semantics-placeholder') !== null,
  }));
  check(semantics.host && !semantics.placeholder, 'semantics tree is on (screen readers)');

  const origin = new URL(base).origin;
  const crossOrigin = requests.filter((url) => {
    const parsed = new URL(url);
    return parsed.protocol.startsWith('http') && parsed.origin !== origin;
  });
  check(crossOrigin.length === 0, `no cross-origin requests (${crossOrigin.join(', ')})`);
  const gstatic = requests.filter((url) => new URL(url).hostname.endsWith('gstatic.com'));
  check(gstatic.length === 0, `no fonts.gstatic.com / gstatic.com requests (${gstatic.length})`);
  const fontRequests = requests.filter((url) => new URL(url).pathname.startsWith('/fonts/'));
  check(fontRequests.length > 0, `fallback slices fetched from the server (${fontRequests.length})`);
  const cspViolations = consoleErrors.filter((line) =>
    line.toLowerCase().includes('content security policy'),
  );
  check(cspViolations.length === 0, `no CSP violations (${cspViolations.join(' | ')})`);
  check(consoleErrors.length === 0, `no console errors (${consoleErrors.join(' | ')})`);

  const fontsUsed = [...new Set(fontRequests.map((url) => new URL(url).pathname.slice(1)))];
  const manifest = join(buildDir, '..', 'web-fonts-used.json');
  writeFileSync(manifest, JSON.stringify(fontsUsed, null, 2) + '\n');
  console.log(`recorded ${fontsUsed.length} cold-load font files in ${manifest}`);

  // The vanilla client remains reachable at /lite/ and its module scripts
  // resolve under the prefix (no leading-slash asset URLs).
  const lite = await context.newPage();
  const liteErrors = [];
  lite.on('pageerror', (error) => liteErrors.push(String(error)));
  lite.on('response', (response) => {
    if (response.status() >= 400) liteErrors.push(`${response.status()} ${response.url()}`);
  });
  const liteResponse = await lite.goto(base + '/lite/', { waitUntil: 'load', timeout: 30000 });
  check(liteResponse?.status() === 200, '/lite/ answers 200');
  check((await lite.textContent('h1')) === 'Parley', '/lite/ is the vanilla Parley client');
  // 'connected' is transient; the hello handler lands on 'ready'.
  await lite.waitForFunction(() => document.querySelector('#status')?.textContent === 'ready', null, {
    timeout: 15000,
  });
  check(liteErrors.length === 0, `/lite/ scripts run (${liteErrors.join(' | ')})`);
  const redirect = await fetch(base + '/lite', { redirect: 'manual' });
  check(redirect.status === 301 && redirect.headers.get('location') === '/lite/', '/lite redirects to /lite/');
  await lite.close();

  // Fast 4G cold load: first frame under throttling, cache disabled.
  const timingContext = await browser.newContext({
    locale: 'zh-CN',
    viewport: { width: 390, height: 844 },
  });
  const timingPage = await timingContext.newPage();
  await timingPage.addInitScript(() => {
    window.__parleyFirstFrame = null;
    window.addEventListener('flutter-first-frame', () => {
      window.__parleyFirstFrame = performance.now();
    });
  });
  const cdp = await timingContext.newCDPSession(timingPage);
  await cdp.send('Network.enable');
  await cdp.send('Network.setCacheDisabled', { cacheDisabled: true });
  await cdp.send('Network.emulateNetworkConditions', {
    offline: false,
    latency: 170,
    downloadThroughput: (9 * 1024 * 1024) / 8,
    uploadThroughput: (2 * 1024 * 1024) / 8,
    connectionType: 'cellular4g',
  });
  await timingPage.goto(base + '/', { waitUntil: 'load', timeout: 120000 });
  await timingPage.waitForFunction(() => window.__parleyFirstFrame !== null, null, { timeout: 120000 });
  const firstFrame = await timingPage.evaluate(() => window.__parleyFirstFrame);
  console.log(`measured Fast 4G first frame: ${(firstFrame / 1000).toFixed(2)} s (budget 4.00 s)`);
  check(firstFrame <= 4000, 'Fast 4G first-frame budget');
  await timingContext.close();
  await context.close();
} catch (error) {
  failures.push(String(error?.stack ?? error));
  console.error(error);
} finally {
  if (browser) await browser.close();
  child.kill();
  await new Promise((resolveExit) => {
    child.once('exit', resolveExit);
    setTimeout(resolveExit, 2000);
  });
  try {
    rmSync(temp, { recursive: true, force: true, maxRetries: 5 });
  } catch {
    // Windows may hold the binary for a moment; TEMP is disposable.
  }
}

if (failures.length > 0) {
  console.error(`\nweb release gate failed (${failures.length}):\n${failures.join('\n')}`);
  if (logs.length > 0) console.error(`server log:\n${logs.join('')}`);
  process.exit(1);
}
console.log('\nweb release gate passed');
