// App-side settings screen. Tauri-free on purpose: the shell injects it before
// the web client, which stays unmodified. Non-secret choices live in
// localStorage; API keys and the server token go only through the keystore
// adapter. There is deliberately no localStorage fallback for secrets.

export const SETTINGS_KEY = 'parley.settings.v1';

export const SECRET_KEYS = Object.freeze({
  llm: 'llm.apiKey',
  deepgram: 'asr.deepgram.apiKey',
  paraformer: 'asr.paraformer.apiKey',
  azure: 'tts.azure.apiKey',
  server: 'server.token',
});

export const DEFAULT_SETTINGS = Object.freeze({
  mode: 'device',
  translatorPreset: 'groq',
  translatorBaseUrl: '',
  translatorModel: '',
  translatorExtraBody: '',
  asrProvider: 'deepgram',
  ttsProvider: 'system',
  azureRegion: '',
  kokoroUrl: '',
  serverUrl: '',
});

const NON_SECRET = Object.keys(DEFAULT_SETTINGS);

function defaultStorage() {
  try {
    return globalThis.localStorage ?? null;
  } catch {
    return null;
  }
}

export function readSettings(store = defaultStorage()) {
  const settings = { ...DEFAULT_SETTINGS };
  try {
    const raw = store?.getItem(SETTINGS_KEY);
    if (raw) {
      const saved = JSON.parse(raw);
      for (const key of NON_SECRET) {
        if (saved?.[key] != null) settings[key] = String(saved[key]);
      }
    }
  } catch {
    // corrupt or unavailable storage: defaults are safer than throwing
  }
  if (!['device', 'server'].includes(settings.mode)) settings.mode = 'device';
  return settings;
}

export function writeSettings(values, store = defaultStorage()) {
  const clean = {};
  for (const key of NON_SECRET) {
    clean[key] = values?.[key] == null ? DEFAULT_SETTINGS[key] : String(values[key]);
  }
  try {
    store?.setItem(SETTINGS_KEY, JSON.stringify(clean));
  } catch {
    // a read-only WebView still gets a working in-memory session
  }
  return clean;
}

function isPrivateHttpHost(hostname) {
  if (hostname === 'localhost') return true;
  const match = /^(\d{1,3})\.(\d{1,3})\.(\d{1,3})\.(\d{1,3})$/.exec(hostname);
  if (!match) return false;
  const octets = match.slice(1).map(Number);
  if (octets.some((octet) => octet > 255)) return false;
  const [a, b] = octets;
  return (
    a === 10 ||
    a === 127 ||
    (a === 169 && b === 254) ||
    (a === 192 && b === 168) ||
    (a === 172 && b >= 16 && b <= 31)
  );
}

// Android 17's local-network definition lists RFC 1918, link-local, CGNAT and
// IPv6 local addresses but not 127/8 or ::1, so localhost stays prompt-free.
function isLocalNetworkUrl(value) {
  let parsed;
  try {
    parsed = new URL(String(value ?? '').trim());
  } catch {
    return false;
  }
  const host = parsed.hostname.replace(/^\[|\]$/g, '').toLowerCase();
  if (host === 'localhost' || host === '::1' || host.startsWith('127.')) return false;
  // mDNS and home-network names only resolve on the LAN.
  if (/\.(local|lan|home\.arpa)$/.test(host)) return true;
  return (
    isPrivateHttpHost(host) ||
    /^100\.(6[4-9]|[7-9]\d|1[01]\d|12[0-7])\./.test(host) ||
    /^(fe[89ab][0-9a-f]|f[cd][0-9a-f]{2}):/.test(host)
  );
}

// True when saving or connecting would reach a Provider over the LAN (Kokoro, a
// custom translator Base URL, or a Server Host), which Android 17 gates behind
// the ACCESS_LOCAL_NETWORK runtime permission.
export function needsLocalNetworkPermission(settings) {
  if (settings?.mode === 'server') return isLocalNetworkUrl(settings.serverUrl);
  if (isLocalNetworkUrl(settings?.translatorBaseUrl)) return true;
  return settings?.ttsProvider === 'kokoro' && isLocalNetworkUrl(settings?.kokoroUrl);
}

export const LOCAL_NETWORK_DENIED =
  'Android blocked local-network access. Allow Nearby devices for Parley in Android Settings, then try again.';

// Mirrors the Tauri HTTP scope: https anywhere, plain http only on loopback/LAN.
export function normalizeBaseUrl(value, presetId = '') {
  const baseUrl = String(value ?? '')
    .trim()
    .replace(/\/+$/, '');
  if (!baseUrl) {
    if (presetId === 'custom') throw new Error('The custom Preset needs a Base URL.');
    return '';
  }
  let parsed;
  try {
    parsed = new URL(baseUrl);
  } catch {
    throw new Error('Base URL must be a full http:// or https:// URL.');
  }
  if (!['http:', 'https:'].includes(parsed.protocol)) {
    throw new Error('Base URL must start with http:// or https://.');
  }
  if (parsed.username || parsed.password) {
    throw new Error('Base URL must not include credentials.');
  }
  if (parsed.search || parsed.hash || /[?#]/.test(baseUrl)) {
    throw new Error('Base URL must not include a query or fragment.');
  }
  if (parsed.protocol === 'http:' && !isPrivateHttpHost(parsed.hostname)) {
    throw new Error(
      'Plain http is allowed only for localhost or private LAN IPv4 addresses; use https:// for other hosts.',
    );
  }
  return baseUrl;
}

export function normalizeModel(value, presetId = '') {
  const model = String(value ?? '').trim();
  if (!model && presetId === 'custom') throw new Error('The custom Preset needs a Model.');
  return model;
}

export function normalizeExtraBody(value) {
  const text = String(value ?? '').trim();
  if (!text) return '';
  let parsed;
  try {
    parsed = JSON.parse(text);
  } catch (error) {
    throw new Error(`Extra request fields must be valid JSON: ${error.message}`);
  }
  if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed)) {
    throw new Error('Extra request fields must be a JSON object.');
  }
  return text;
}

const STYLE = `
.parley-settings-button {
  position: fixed; right: 12px; bottom: 12px; z-index: 20;
  font: 600 14px system-ui, sans-serif; padding: 10px 14px; border-radius: 8px;
  border: 1px solid #888; background: #1f2430; color: #fff; cursor: pointer;
}
.parley-settings { max-width: 420px; width: calc(100vw - 32px); padding: 0; }
.parley-settings::backdrop { background: rgba(0,0,0,.45); }
.parley-settings form { padding: 18px; display: grid; gap: 10px; }
.parley-settings h2 { margin: 0 0 4px; font: 700 18px system-ui, sans-serif; }
.parley-settings label { display: grid; gap: 4px; font: 13px system-ui, sans-serif; }
.parley-settings input, .parley-settings select {
  font: 14px system-ui, sans-serif; padding: 7px; border-radius: 6px;
  border: 1px solid #999; background: #fff; color: #111;
}
.parley-settings .parley-row { display: grid; gap: 10px; }
.parley-settings .parley-actions { display: flex; gap: 8px; justify-content: flex-end; }
.parley-settings button { font: 600 14px system-ui, sans-serif; padding: 8px 14px; border-radius: 6px; }
.parley-settings .parley-note { margin: 0; font: 12px system-ui, sans-serif; color: #9a3412; }
.parley-settings .parley-status { margin: 0; font: 12px system-ui, sans-serif; }
`;

function resolveStore(keyStore) {
  return typeof keyStore?.then === 'function' ? keyStore : Promise.resolve(keyStore);
}

function field(labelText, input) {
  const label = document.createElement('label');
  label.textContent = labelText;
  label.append(input);
  return label;
}

function select(values, selected) {
  const element = document.createElement('select');
  for (const [value, label] of values) {
    const option = document.createElement('option');
    option.value = value;
    option.textContent = label;
    if (value === selected) option.selected = true;
    element.append(option);
  }
  return element;
}

function textInput(value, { type = 'text', placeholder = '', autocomplete = 'off' } = {}) {
  const input = document.createElement('input');
  input.type = type;
  input.value = value ?? '';
  input.placeholder = placeholder;
  input.autocomplete = autocomplete;
  input.spellcheck = false;
  return input;
}

export function mountSettings({
  keyStore,
  presets = {},
  onSaved = () => {},
  requestLocalNetwork = async () => true,
} = {}) {
  if (typeof document === 'undefined' || !document.body) return null;

  const style = document.createElement('style');
  style.textContent = STYLE;
  document.head.append(style);

  const button = document.createElement('button');
  button.type = 'button';
  button.className = 'parley-settings-button';
  button.textContent = 'Settings';
  button.setAttribute('aria-haspopup', 'dialog');

  const dialog = document.createElement('dialog');
  dialog.className = 'parley-settings';
  dialog.setAttribute('aria-label', 'Parley settings');
  const form = document.createElement('form');
  dialog.append(form);

  const heading = document.createElement('h2');
  heading.textContent = 'Providers and keys';
  form.append(heading);

  const mode = select(
    [
      ['device', 'This device (direct Providers)'],
      ['server', 'Connect to my server'],
    ],
    DEFAULT_SETTINGS.mode,
  );
  const serverUrl = textInput('', { placeholder: 'https://parley.example.com' });
  const serverToken = textInput('', { type: 'password', placeholder: 'Access Token (optional)' });
  const translatorPreset = select(
    Object.values(presets).map((preset) => [preset.id, `${preset.id} — ${preset.note ?? ''}`]),
    DEFAULT_SETTINGS.translatorPreset,
  );
  const translatorBaseUrl = textInput('', { placeholder: 'Preset default; required for custom' });
  const translatorModel = textInput('', { placeholder: 'Preset default' });
  const llmKey = textInput('', { type: 'password', placeholder: 'Groq / OpenAI-compatible key' });
  const translatorExtraBody = textInput('', { placeholder: '{"top_k":20}' });
  const asrProvider = select(
    [
      ['deepgram', 'Deepgram (cloud)'],
      ['paraformer', 'Paraformer (cloud)'],
      ['mock', 'Mock (bundled sample only)'],
    ],
    DEFAULT_SETTINGS.asrProvider,
  );
  const deepgramKey = textInput('', { type: 'password', placeholder: 'Deepgram API key' });
  const paraformerKey = textInput('', { type: 'password', placeholder: 'Paraformer API key' });
  const ttsProvider = select(
    [
      ['system', 'System TTS (default)'],
      ['azure', 'Azure Speech'],
      ['kokoro', 'Kokoro server'],
      ['mock', 'Mock (Demo Mode)'],
    ],
    DEFAULT_SETTINGS.ttsProvider,
  );
  const azureKey = textInput('', { type: 'password', placeholder: 'Azure Speech key' });
  const azureRegion = textInput('', { placeholder: 'e.g. eastasia' });
  const kokoroUrl = textInput('', { placeholder: 'http://192.168.1.10:8880' });
  const note = document.createElement('p');
  note.className = 'parley-note';
  const status = document.createElement('p');
  status.className = 'parley-status';

  function syncPresetHelp() {
    const preset = presets[translatorPreset.value];
    translatorBaseUrl.placeholder =
      preset?.baseUrl ||
      (translatorPreset.value === 'custom'
        ? 'https://gateway.example.com/v1'
        : 'Optional override');
    translatorModel.placeholder =
      preset?.model ||
      (translatorPreset.value === 'custom' ? 'Required for custom' : 'Optional override');
  }
  translatorPreset.addEventListener('change', syncPresetHelp);

  const providerRow = document.createElement('div');
  providerRow.className = 'parley-row';
  providerRow.append(
    field('Mode', mode),
    field('Server URL', serverUrl),
    field('Access Token', serverToken),
    field('Translator Preset', translatorPreset),
    field('Base URL', translatorBaseUrl),
    field('Model', translatorModel),
    field('Translator API Key', llmKey),
    field('Extra request fields (JSON)', translatorExtraBody),
    field('ASR Provider', asrProvider),
    field('Deepgram API Key', deepgramKey),
    field('Paraformer API Key', paraformerKey),
    field('TTS Provider', ttsProvider),
    field('Azure Speech Key', azureKey),
    field('Azure Region', azureRegion),
    field('Kokoro URL', kokoroUrl),
    note,
    status,
  );
  form.append(providerRow);

  const actions = document.createElement('div');
  actions.className = 'parley-actions';
  const cancel = document.createElement('button');
  cancel.type = 'button';
  cancel.textContent = 'Cancel';
  const save = document.createElement('button');
  save.type = 'submit';
  save.textContent = 'Save';
  actions.append(cancel, save);
  form.append(actions);

  let store = null;

  async function open() {
    const settings = readSettings();
    mode.value = settings.mode;
    serverUrl.value = settings.serverUrl;
    translatorPreset.value = settings.translatorPreset;
    translatorBaseUrl.value = settings.translatorBaseUrl;
    translatorModel.value = settings.translatorModel;
    translatorExtraBody.value = settings.translatorExtraBody;
    syncPresetHelp();
    asrProvider.value = settings.asrProvider;
    ttsProvider.value = settings.ttsProvider;
    azureRegion.value = settings.azureRegion;
    kokoroUrl.value = settings.kokoroUrl;
    status.textContent = '';
    store = await resolveStore(keyStore);
    const [llm, dg, pf, az, token] = await Promise.all([
      store.get(SECRET_KEYS.llm),
      store.get(SECRET_KEYS.deepgram),
      store.get(SECRET_KEYS.paraformer),
      store.get(SECRET_KEYS.azure),
      store.get(SECRET_KEYS.server),
    ]);
    llmKey.value = llm ?? '';
    deepgramKey.value = dg ?? '';
    paraformerKey.value = pf ?? '';
    azureKey.value = az ?? '';
    serverToken.value = token ?? '';
    note.textContent = store.persistent
      ? ''
      : 'Secure storage is unavailable. Keys stay in memory for this session only; they are never written to browser storage.';
    try {
      dialog.showModal();
    } catch {
      dialog.setAttribute('open', '');
    }
  }

  cancel.addEventListener('click', () => dialog.close());

  form.addEventListener('submit', async (event) => {
    event.preventDefault();
    status.textContent = '';
    const values = {
      mode: mode.value,
      translatorPreset: translatorPreset.value,
      translatorBaseUrl: '',
      translatorModel: '',
      translatorExtraBody: '',
      asrProvider: asrProvider.value,
      ttsProvider: ttsProvider.value,
      azureRegion: azureRegion.value.trim(),
      kokoroUrl: kokoroUrl.value.trim(),
      serverUrl: serverUrl.value.trim(),
    };
    if (values.mode === 'server') {
      try {
        const parsed = new URL(values.serverUrl);
        if (!['http:', 'https:', 'ws:', 'wss:'].includes(parsed.protocol)) throw new Error();
      } catch {
        status.textContent = 'Server URL must start with http://, https://, ws:// or wss://.';
        return;
      }
      values.translatorBaseUrl = translatorBaseUrl.value.trim();
      values.translatorModel = translatorModel.value.trim();
      values.translatorExtraBody = translatorExtraBody.value.trim();
    } else {
      try {
        values.translatorBaseUrl = normalizeBaseUrl(
          translatorBaseUrl.value,
          values.translatorPreset,
        );
        values.translatorModel = normalizeModel(translatorModel.value, values.translatorPreset);
        values.translatorExtraBody = normalizeExtraBody(translatorExtraBody.value);
      } catch (error) {
        status.textContent = error.message;
        return;
      }
    }
    if (needsLocalNetworkPermission(values) && !(await requestLocalNetwork())) {
      status.textContent = LOCAL_NETWORK_DENIED;
      return;
    }
    writeSettings(values);
    if (!store) store = await resolveStore(keyStore);
    const secrets = [
      [SECRET_KEYS.llm, llmKey.value],
      [SECRET_KEYS.deepgram, deepgramKey.value],
      [SECRET_KEYS.paraformer, paraformerKey.value],
      [SECRET_KEYS.azure, azureKey.value],
      [SECRET_KEYS.server, serverToken.value],
    ];
    for (const [key, value] of secrets) {
      const trimmed = value.trim();
      if (trimmed) await store.set(key, trimmed);
      else await store.remove(key);
    }
    status.textContent = store.persistent
      ? 'Saved. Reloading…'
      : 'Saved for this session only. Reloading…';
    save.disabled = true;
    setTimeout(() => {
      dialog.close();
      onSaved();
    }, 250);
  });

  button.addEventListener('click', open);
  document.body.append(button, dialog);
  return { button, dialog, open };
}
