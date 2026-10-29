// Server Host: static web client, /healthz, and the WebSocket protocol bridge
// into the Session core. The browser never runs the core (ADR-0001).

import { createServer as createHttpServer } from 'node:http';
import { readFile, readdir } from 'node:fs/promises';
import { readFileSync } from 'node:fs';
import path from 'node:path';
import { fileURLToPath, pathToFileURL } from 'node:url';
import { WebSocketServer } from 'ws';

import { createRegistry } from '../../core/packs-registry.js';
import { loadProvider } from '../../core/providers/index.js';
import { createSession } from '../../core/session.js';
import presetData from '../../config/presets.json' with { type: 'json' };
import { loadConfig } from './config.js';

const { PRESETS } = presetData;

const ROOT = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '../..');

const MIME = {
  '.html': 'text/html; charset=utf-8',
  '.js': 'text/javascript; charset=utf-8',
  '.css': 'text/css; charset=utf-8',
  '.json': 'application/json; charset=utf-8',
  '.wav': 'audio/wav',
  '.mp3': 'audio/mpeg',
};

const defaultLogger = {
  segment: (record) => console.log(JSON.stringify(record)),
  warn: (line) => console.warn(line),
};

export async function createServer({ config = loadConfig(), logger = defaultLogger, root = ROOT } = {}) {
  const segmentLog = logger.segment ?? (() => {});
  const warn = logger.warn ?? (() => {});
  if (!config.accessToken) {
    warn('warning: ACCESS_TOKEN is not set; anyone who finds this URL can spend your API quota');
  }

  const registry = await loadRegistry(root, warn);
  const pairConfig = loadPairConfig(root, warn);
  const forcedInstruction = loadForcedInstruction(root, warn);
  const sample = JSON.parse(readFileSync(path.join(root, 'samples', 'placeholder.json'), 'utf8'));

  const factories = {
    asr: await loadProvider('asr', config.asr),
    translator: await loadProvider('translator', config.translator),
    tts: await loadProvider('tts', config.tts),
  };
  // ASR adapters take {url} / {apiKey}; config exposes provider-specific names.
  const asrOptions =
    config.asr === 'deepgram' ? { apiKey: config.asrOptions.deepgramApiKey }
      : config.asr === 'paraformer' ? { apiKey: config.asrOptions.dashscopeApiKey }
        : config.asr === 'whisperlivekit' && config.asrOptions.whisperlivekitUrl
          ? { url: config.asrOptions.whisperlivekitUrl }
          : {};

  const http = createHttpServer(async (req, res) => {
    try {
      if (req.method !== 'GET' && req.method !== 'HEAD') {
        res.writeHead(405).end();
        return;
      }
      const { pathname } = new URL(req.url, 'http://localhost');
      if (pathname === '/healthz') {
        res.writeHead(200, { 'content-type': 'application/json; charset=utf-8' });
        res.end(JSON.stringify({ ok: true }));
        return;
      }
      const file = resolveStatic(root, pathname);
      if (!file) {
        res.writeHead(404).end('not found');
        return;
      }
      const body = await readFile(file);
      res.writeHead(200, {
        'content-type': MIME[path.extname(file)] ?? 'application/octet-stream',
        'cache-control': 'no-store',
      });
      res.end(req.method === 'HEAD' ? undefined : body);
    } catch (error) {
      if (error?.code === 'ENOENT' || error?.code === 'EISDIR') {
        res.writeHead(404).end('not found');
        return;
      }
      res.writeHead(400).end('bad request');
    }
  });

  const wss = new WebSocketServer({ noServer: true });

  http.on('upgrade', (req, socket, head) => {
    const { pathname, searchParams } = new URL(req.url, 'http://localhost');
    if (pathname !== '/ws') {
      socket.destroy();
      return;
    }
    if (config.accessToken && searchParams.get('token') !== config.accessToken) {
      const body = 'access token required';
      socket.end(
        'HTTP/1.1 401 Unauthorized\r\n' +
          'content-type: text/plain; charset=utf-8\r\n' +
          `content-length: ${Buffer.byteLength(body)}\r\n` +
          'connection: close\r\n\r\n' +
          body,
      );
      return;
    }
    wss.handleUpgrade(req, socket, head, (ws) => wss.emit('connection', ws, req));
  });

  wss.on('connection', (ws) => {
    let session;
    const emit = (event) => {
      if (ws.readyState !== 1) return;
      if (event.type === 'tts_chunk') {
        ws.send(Buffer.from(event.data));
        return;
      }
      ws.send(JSON.stringify(event));
    };

    try {
      const translator = factories.translator({
        references: sample.reference_translations ?? {},
        ...config.llm,
      });
      const providers = {
        asr: factories.asr({ sample, ...asrOptions }),
        translator,
        tts: factories.tts({ ...config.ttsOptions }),
      };
      session = createSession({
        providers,
        clock: { now: () => Date.now() },
        registry,
        pairConfig,
        forcedInstruction,
        emit,
        log: (record) => segmentLog(record),
      });
      const preset = PRESETS[translator.preset];
      // The Session core does not know the Host's Preset configuration.
      ws.send(JSON.stringify({
        ...session.hello(),
        presetsInUse: preset ? [{ id: preset.id, trainsOnData: preset.trainsOnData }] : [],
      }));
    } catch (error) {
      emit({
        type: 'error',
        kind: 'protocol',
        message: `could not start the Session: ${error.message}`,
      });
      ws.close();
      return;
    }

    ws.on('message', (data, isBinary) => {
      if (isBinary) {
        session.handleAudio(toPcm(data));
        return;
      }
      try {
        session.handleMessage(JSON.parse(data.toString()));
      } catch {
        emit({ type: 'error', kind: 'protocol', message: 'invalid JSON message' });
      }
    });
    ws.on('close', () => session.close());
  });

  return {
    listen(port = config.port) {
      return new Promise((resolve, reject) => {
        const onError = (error) => reject(error);
        http.once('error', onError);
        http.listen(port, () => {
          http.off('error', onError);
          resolve(http.address());
        });
      });
    },
    address: () => http.address(),
    async close() {
      for (const client of wss.clients) client.terminate();
      await new Promise((resolve) => wss.close(resolve));
      await new Promise((resolve) => http.close(resolve));
    },
  };
}

export async function loadRegistry(root, warn) {
  const readFiles = (dir, names) =>
    names.map((name) => ({ name, text: readFileSync(path.join(dir, name), 'utf8') }));
  const promptDir = path.join(root, 'prompts');
  const promptEntries = await readdir(promptDir);
  const promptFiles = readFiles(
    promptDir,
    promptEntries.filter((name) => name.startsWith('system_') && name.endsWith('.txt')).sort(),
  );
  const glossaryFiles = readFiles(
    promptDir,
    promptEntries.filter((name) => name.startsWith('glossary_') && name.endsWith('.tsv')).sort(),
  );
  const packDir = path.join(root, 'packs');
  const packFiles = readFiles(
    packDir,
    (await readdir(packDir)).filter((name) => name.endsWith('.tsv')).sort(),
  );
  return createRegistry({ promptFiles, glossaryFiles, packFiles, warn });
}

function loadForcedInstruction(root, warn) {
  try {
    return readFileSync(path.join(root, 'prompts', 'forced_translation.txt'), 'utf8').trim();
  } catch (error) {
    if (error.code !== 'ENOENT') warn(`prompts/forced_translation.txt ignored: ${error.message}`);
    return '';
  }
}

function loadPairConfig(root, warn) {
  try {
    return JSON.parse(readFileSync(path.join(root, 'config', 'pairs.json'), 'utf8'));
  } catch (error) {
    if (error.code !== 'ENOENT') warn(`config/pairs.json ignored: ${error.message}`);
    return {};
  }
}

function resolveStatic(root, pathname) {
  let base = path.join(root, 'web');
  let relative = pathname;
  if (pathname.startsWith('/samples/')) {
    base = path.join(root, 'samples');
    relative = pathname.slice('/samples/'.length);
  }
  relative = relative.replace(/^\/+/, '') || 'index.html';
  const file = path.resolve(base, relative);
  if (file !== base && !file.startsWith(`${base}${path.sep}`)) return null;
  return file;
}

function toPcm(data) {
  if (data instanceof ArrayBuffer) return new Int16Array(data);
  return new Int16Array(data.buffer.slice(data.byteOffset, data.byteOffset + data.byteLength));
}

const invoked = process.argv[1] ? pathToFileURL(path.resolve(process.argv[1])).href : '';
if (import.meta.url === invoked) {
  const config = loadConfig();
  createServer({ config })
    .then((server) => server.listen())
    .then(() => console.log(`Parley listening on http://localhost:${config.port}`))
    .catch((error) => {
      console.error(error);
      process.exitCode = 1;
    });
}
