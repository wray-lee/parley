// Device Host: runs the Session core inside the WebView with real Providers
// and exposes it through the WebSocket interface the unmodified web client
// speaks. All Tauri API use lives in ./platform-tauri.js, ./keystore.js and
// ./tts-speaker.js; core/ and web/ are used exactly as the Server Host uses
// them. Settings either build Providers here or redirect the client to a
// Server Host.

import presetData from './config/presets.json' with { type: 'json' };
import { createRegistry } from './core/packs-registry.js';
import { createProvider } from './core/providers/index.js';
import { createSession } from './core/session.js';
import { loadKeyStore } from './keystore.js';
import { loadTauriPlatform, requestLocalNetworkAccess } from './platform-tauri.js';
import { createServerWebSocket } from './server-socket.js';
import {
  LOCAL_NETWORK_DENIED,
  mountSettings,
  needsLocalNetworkPermission,
  readSettings,
  SECRET_KEYS,
} from './settings.js';
import { createSpeechGate } from './speech-gate.js';
import { loadSystemSpeaker } from './tts-speaker.js';

const { DEFAULT_PRESET, PRESETS } = presetData;

async function readAsset(path) {
  const response = await fetch(path);
  if (!response.ok) throw new Error(`${path}: HTTP ${response.status}`);
  return response;
}

let hostPromise = null;

// Prompts, packs, pair tuning and the bundled sample ship as static files, so
// the shell needs no Rust-side file API.
function loadHost() {
  hostPromise ??= (async () => {
    const manifest = await (await readAsset('/device-assets.json')).json();
    const readAll = (dir, names) =>
      Promise.all(
        names.map(async (name) => ({
          name,
          text: await (await readAsset(`/${dir}/${name}`)).text(),
        })),
      );
    const [promptFiles, packFiles, pairConfig, sample] = await Promise.all([
      readAll('prompts', manifest.prompts),
      readAll('packs', manifest.packs),
      readAsset('/config/pairs.json').then((response) => response.json()),
      readAsset('/samples/placeholder.json').then((response) => response.json()),
    ]);
    return {
      registry: createRegistry({ promptFiles, packFiles, warn: (line) => console.warn(line) }),
      pairConfig,
      sample,
    };
  })();
  return hostPromise;
}

const settings = readSettings();
const keyStorePromise = loadKeyStore({ warn: (line) => console.warn(line) });

mountSettings({
  keyStore: keyStorePromise,
  presets: PRESETS,
  onSaved: () => globalThis.location?.reload(),
  requestLocalNetwork: requestLocalNetworkAccess,
});

async function mockProviders(sample) {
  return {
    asr: await createProvider('asr', 'mock', { sample }),
    translator: await createProvider('translator', 'mock', {
      references: sample.reference_translations ?? {},
    }),
    tts: await createProvider('tts', 'mock'),
  };
}

async function createProviders(sample) {
  const current = readSettings();
  const keyStore = await keyStorePromise;
  const [llmKey, deepgramKey, paraformerKey, azureKey] = await Promise.all([
    keyStore.get(SECRET_KEYS.llm),
    keyStore.get(SECRET_KEYS.deepgram),
    keyStore.get(SECRET_KEYS.paraformer),
    keyStore.get(SECRET_KEYS.azure),
  ]);
  const preset = PRESETS[current.translatorPreset] ?? PRESETS[DEFAULT_PRESET];
  if (preset.id === 'custom' && !current.translatorBaseUrl) {
    throw new Error('The custom Preset needs a Base URL. Open Settings and set one.');
  }

  // No translator key means Demo Mode, exactly as the Server Host behaves.
  if (preset.requiresKey && !llmKey) {
    return { providers: await mockProviders(sample), abort: null };
  }

  // Android 17 gates LAN sockets; ask before the first Provider request.
  if (needsLocalNetworkPermission(current) && !(await requestLocalNetworkAccess())) {
    throw new Error(LOCAL_NETWORK_DENIED);
  }

  let platformPromise = null;
  const platform = () => (platformPromise ??= loadTauriPlatform());

  const translator = await createProvider('translator', 'openai', {
    preset: preset.id,
    baseUrl: current.translatorBaseUrl,
    apiKey: llmKey || '',
    model: current.translatorModel,
    extraBody: current.translatorExtraBody,
    fetch: (await platform()).fetch,
  });

  let asr;
  if (current.asrProvider === 'deepgram' && deepgramKey) {
    asr = await createProvider('asr', 'deepgram', {
      apiKey: deepgramKey,
      WebSocket: (await platform()).WebSocket,
      auth: 'header',
    });
  } else if (current.asrProvider === 'paraformer' && paraformerKey) {
    asr = await createProvider('asr', 'paraformer', {
      apiKey: paraformerKey,
      WebSocket: (await platform()).WebSocket,
    });
  } else {
    asr = await createProvider('asr', 'mock', { sample });
  }

  let tts;
  if (current.ttsProvider === 'system') {
    tts = await createProvider('tts', 'system');
  } else if (current.ttsProvider === 'azure' && azureKey && current.azureRegion) {
    tts = await createProvider('tts', 'azure', {
      azureKey,
      azureRegion: current.azureRegion,
      fetch: (await platform()).fetch,
      warn: (line) => console.warn(line),
    });
  } else if (current.ttsProvider === 'kokoro') {
    tts = await createProvider('tts', 'kokoro', {
      url: current.kokoroUrl,
      fetch: (await platform()).fetch,
      warn: (line) => console.warn(line),
    });
  } else {
    tts = await createProvider('tts', 'mock');
  }

  const loaded = platformPromise ? await platformPromise : null;
  return {
    providers: { asr, translator, tts },
    abort: loaded?.fetch.abortAll ?? null,
  };
}

// ASR adapters queue typed errors for `push()`; the Session only forwards
// partial/stable, so the Device Host forwards the errors itself.
function surfaceAsrErrors(asr, emit) {
  const push = asr.push?.bind(asr);
  if (!push) return asr;
  asr.push = (args) => {
    const events = push(args);
    for (const event of events) if (event.type === 'error') emit(event);
    return events;
  };
  return asr;
}

// Minimal browser-WebSocket surface: the web client only uses these members.
class DeviceWebSocket {
  static CONNECTING = 0;
  static OPEN = 1;
  static CLOSING = 2;
  static CLOSED = 3;

  readyState = DeviceWebSocket.CONNECTING;
  binaryType = 'arraybuffer';
  url;
  #listeners = new Map();
  #session = null;
  #abortStreams = null;
  #speaker = null;
  #systemTts = false;
  #speakerWarned = false;
  #mode = 'listen';
  #gate = createSpeechGate();
  #speechSession = 0;
  #stopped = false;

  constructor(url) {
    this.url = String(url);
    this.#connect();
  }

  addEventListener(type, listener) {
    if (!this.#listeners.has(type)) this.#listeners.set(type, new Set());
    this.#listeners.get(type).add(listener);
  }

  removeEventListener(type, listener) {
    this.#listeners.get(type)?.delete(listener);
  }

  send(data) {
    if (this.readyState !== DeviceWebSocket.OPEN || !this.#session) return;
    if (typeof data === 'string') {
      let message;
      try {
        message = JSON.parse(data);
      } catch {
        this.#emit({ type: 'error', kind: 'protocol', message: 'invalid JSON message' });
        return;
      }
      if (message?.type === 'start') {
        this.#mode = message.mode === 'converse' ? 'converse' : 'listen';
        this.#stopped = false;
      }
      if (message?.type === 'stop') {
        this.#stopped = true;
        this.#release();
      }
      this.#session.handleMessage(message);
      return;
    }
    if (this.#mode === 'converse' && this.#gate.blocked) return;
    this.#session.handleAudio(new Int16Array(data));
  }

  close() {
    const wasClosed = this.readyState === DeviceWebSocket.CLOSED;
    this.readyState = DeviceWebSocket.CLOSED;
    this.#release();
    this.#session?.close();
    if (!wasClosed) this.#fire('close', {});
  }

  async #connect() {
    try {
      const host = await loadHost();
      if (this.readyState === DeviceWebSocket.CLOSED) return;
      const { providers, abort } = await createProviders(host.sample);
      if (this.readyState === DeviceWebSocket.CLOSED) return;
      this.#abortStreams = abort;
      this.#systemTts = providers.tts.name === 'system';
      if (this.#systemTts) {
        this.#speaker = await loadSystemSpeaker({ warn: (line) => console.warn(line) });
      }
      providers.asr = surfaceAsrErrors(providers.asr, (event) => this.#emit(event));
      this.#session = createSession({
        providers,
        clock: { now: () => Date.now() },
        registry: host.registry,
        pairConfig: host.pairConfig,
        emit: (event) => {
          this.#emit(event);
          if (this.#systemTts && event.type === 'segment' && event.speak) this.#speak(event);
        },
        log: (record) => {
          console.log(JSON.stringify(record));
          (globalThis.__parleyLog ??= []).push(record);
        },
      });
      this.readyState = DeviceWebSocket.OPEN;
      this.#fire('open', {});
      this.#emit(this.#session.hello());
    } catch (error) {
      this.#fire('error', { message: error.message });
      this.close();
    }
  }

  // The core treats system TTS as text-only, so the Device Host speaks each
  // Segment itself and holds the microphone while it plays (Converse only).
  // shortcut: app-side gate; move it into core when system TTS emits tts_*.
  #speak(segment) {
    if (!segment.tgt) return;
    if (!this.#speaker) {
      if (!this.#speakerWarned) {
        this.#speakerWarned = true;
        this.#emit({
          type: 'error',
          provider: 'tts',
          kind: 'protocol',
          message: 'System TTS is unavailable on this device; showing subtitles only.',
        });
      }
      queueMicrotask(() => {
        this.#session?.handleMessage({ type: 'playback', seg_id: segment.seg_id, state: 'end' });
      });
      return;
    }
    const gated = this.#mode === 'converse';
    const session = this.#speechSession;
    if (gated) this.#gate.begin();
    this.#speaker
      .speak(segment.tgt, segment.direction?.tgt)
      .catch((error) => {
        if (this.#speakerWarned) return;
        this.#speakerWarned = true;
        this.#emit({ type: 'error', provider: 'tts', kind: 'protocol', message: error.message });
      })
      .finally(() => {
        if (gated && session === this.#speechSession) this.#gate.end();
        // Lets the Session end its playback/echo entry at the real finish.
        this.#session?.handleMessage({ type: 'playback', seg_id: segment.seg_id, state: 'end' });
      });
  }

  #release() {
    this.#speechSession += 1;
    this.#abortStreams?.();
    this.#gate.reset();
    this.#speaker?.stop();
  }

  #emit(event) {
    if (this.readyState !== DeviceWebSocket.OPEN) return;
    // Aborting an in-flight translation on Stop must not raise a toast.
    if (this.#stopped && event.type === 'error') return;
    if (event.type === 'tts_chunk') {
      this.#fire('message', { data: event.data });
      return;
    }
    this.#fire('message', { data: JSON.stringify(event) });
  }

  #fire(type, event) {
    queueMicrotask(() => {
      for (const listener of this.#listeners.get(type) ?? []) listener.call(this, event);
    });
  }
}

if (settings.mode === 'server' && settings.serverUrl) {
  globalThis.WebSocket = createServerWebSocket({
    Native: globalThis.WebSocket,
    serverUrl: settings.serverUrl,
    token: keyStorePromise.then((store) => store.get(SECRET_KEYS.server)),
  });
} else {
  globalThis.WebSocket = DeviceWebSocket;
}
