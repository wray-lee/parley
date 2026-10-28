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

export function mountSettings({ keyStore, presets = {}, onSaved = () => {} } = {}) {
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
  const llmKey = textInput('', { type: 'password', placeholder: 'Groq / OpenAI-compatible key' });
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

  const providerRow = document.createElement('div');
  providerRow.className = 'parley-row';
  providerRow.append(
    field('Mode', mode),
    field('Server URL', serverUrl),
    field('Access Token', serverToken),
    field('Translator Preset', translatorPreset),
    field('Translator API Key', llmKey),
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
    const values = {
      mode: mode.value,
      translatorPreset: translatorPreset.value,
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
