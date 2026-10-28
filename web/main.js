// Protocol client of the Server Host. The core never runs in this browser.

import { createAudioOut } from './audio-out.js';
import { createCutRing } from './cut-ring.js';
import { createHoldIndicator } from './hold-indicator.js';
import { streamSample } from './demo-sample.js';
import { createSubtitles } from './subtitles.js';
import { createTurns } from './turns.js';

const $ = (id) => document.getElementById(id);

const TOKEN_KEY = 'parley.token.v1';

const subtitles = createSubtitles($('subtitles'), $('demo-banner'));
const audioOut = createAudioOut({
  onPlayback: (seg_id, state) => send({ type: 'playback', seg_id, state }),
});
const cutRing = createCutRing($('cut-ring'));
const holdIndicator = createHoldIndicator($('hold'));
const turns = createTurns($('turns'), (who) => send({ type: 'turn', who }));

const pairs = new Map();
const voiceNoticed = new Set();
let ws = null;
let sample = null;
let stopStreaming = null;
let mic = null;
let ttsFormat = 'wav';
let ttsSeg = null;
let audioTts = false; // the Server Host sends tts_chunk audio, not system TTS

$('start').addEventListener('click', start);
$('stop').addEventListener('click', stop);
$('mic').addEventListener('click', toggleMic);
$('ptt').addEventListener('pointerdown', () => send({ type: 'ptt', state: 'down' }));
$('ptt').addEventListener('pointerup', () => send({ type: 'ptt', state: 'up' }));
$('ptt').addEventListener('pointerleave', () => send({ type: 'ptt', state: 'up' }));

connect();
loadSample();

// One prompt per tab; an empty answer is remembered so a server without a
// token is not asked again. sessionStorage only: the token is never persisted.
function accessToken() {
  let token = sessionStorage.getItem(TOKEN_KEY);
  if (token === null) {
    token = prompt('Access token for this Parley server (leave empty if it has none):') ?? '';
    sessionStorage.setItem(TOKEN_KEY, token);
  }
  return token;
}

function connect() {
  const scheme = location.protocol === 'https:' ? 'wss' : 'ws';
  const token = accessToken();
  const query = token ? `?token=${encodeURIComponent(token)}` : '';
  ws = new WebSocket(`${scheme}://${location.host}/ws${query}`);
  ws.binaryType = 'arraybuffer';
  ws.addEventListener('open', () => setStatus('connected'));
  ws.addEventListener('close', () => setStatus('disconnected'));
  ws.addEventListener('error', (event) => toast(event?.message ?? 'WebSocket connection failed'));
  ws.addEventListener('message', (event) => {
    if (typeof event.data === 'string') {
      onMessage(JSON.parse(event.data));
      return;
    }
    audioOut.play(ttsFormat, event.data, ttsSeg).catch((error) => toast(`audio: ${error.message}`));
  });
}

function onMessage(message) {
  switch (message.type) {
    case 'hello':
      onHello(message);
      break;
    case 'asr_partial':
      cutRing.set(message); // ticket 02 renders Partials
      break;
    case 'segment':
      cutRing.set(message);
      holdIndicator.set(message);
      subtitles.add(message);
      setStatus('listening');
      if (message.speak && !audioTts) speakSegment(message);
      break;
    case 'tts_begin':
      audioTts = true;
      ttsSeg = message.seg_id;
      ttsFormat = message.format;
      audioOut.cancelSpeech(); // provider audio wins over a wrongly guessed system voice
      audioOut.begin(message.seg_id);
      setStatus('speaking');
      break;
    case 'tts_end':
      audioOut.end(message.seg_id);
      ttsSeg = null;
      setStatus('listening');
      break;
    case 'state':
      holdIndicator.set(message);
      setStatus(message.state);
      break;
    case 'hold':
      holdIndicator.set(message);
      break;
    case 'turn':
      turns.set(message.who);
      break;
    case 'error': {
      const kind = message.kind ? `${message.kind}: ` : '';
      const fallback = message.fallback ? ` — fallback: ${message.fallback}` : '';
      toast(`${kind}${message.message}${fallback}`);
      if (message.provider === 'asr') stopMic();
      break;
    }
    default:
      break;
  }
}

// System TTS: the Segment carries text only, so the browser speaks it and
// reports playback so gating and the Converse auto-switch use real timing.
// shortcut: the first spoken Segment of an audio-TTS session may briefly use
// the browser voice until tts_begin reveals the mode; add hello.tts to close it.
function speakSegment(segment) {
  const lang = segment.direction?.tgt ?? '';
  send({ type: 'playback', seg_id: segment.seg_id, state: 'start' });
  audioOut.speak(segment.tgt, lang).then((spoken) => {
    if (audioTts) return; // provider audio took over; its playback events end this Segment
    if (!spoken && !voiceNoticed.has(lang)) {
      voiceNoticed.add(lang);
      toast(`No ${lang || 'target-language'} voice installed — subtitles only`);
    }
    send({ type: 'playback', seg_id: segment.seg_id, state: 'end' });
  });
}

function onHello(message) {
  subtitles.setDemo(message.demo);
  renderPairs(message.pairs ?? []);
  renderPacks(message.packs ?? []);
  renderPresets(message.presetsInUse ?? []);
  setStatus('ready');
}

// A Preset whose free tier trains on data gets a visible warning label. An
// older Server Host sends no metadata, so the badge simply stays hidden.
function renderPresets(list) {
  const badge = $('preset');
  if (!badge) return;
  const training = list.filter((preset) => preset?.trainsOnData === true);
  badge.hidden = training.length === 0;
  badge.textContent = training.length
    ? `Preset ${training.map((preset) => preset.id ?? preset.name).join(', ')}: free tier trains on data`
    : '';
}

function renderPairs(list) {
  const select = $('pair');
  select.textContent = '';
  pairs.clear();
  for (const pair of list) {
    const value = `${pair.a}|${pair.b}`;
    pairs.set(value, pair);
    const option = document.createElement('option');
    option.value = value;
    option.textContent = `${pair.a} ↔ ${pair.b}${pair.converse ? '' : ' (listen)'}`;
    select.append(option);
  }
  selectSamplePair();
}

function renderPacks(list) {
  const select = $('packs');
  select.textContent = '';
  for (const name of [...new Set(list.map((pack) => pack.name))]) {
    const option = document.createElement('option');
    option.value = name;
    option.textContent = name;
    select.append(option);
  }
}

function selectSamplePair() {
  if (!sample) return;
  for (const [value, pair] of pairs) {
    if (pair.directions.some((d) => d.src === sample.src && d.tgt === sample.tgt)) {
      $('pair').value = value;
      return;
    }
  }
}

async function loadSample() {
  try {
    sample = await (await fetch('/samples/placeholder.json')).json();
    selectSamplePair();
  } catch (error) {
    toast(`sample: ${error.message}`);
  }
}

function sampleDirection() {
  const pair = pairs.get($('pair').value);
  if (!pair) return null;
  return pair.directions.find((d) => d.src === sample?.src && d.tgt === sample?.tgt)
    ?? pair.directions[0]
    ?? null;
}

async function start() {
  if (!sample) {
    toast('the demo sample is still loading');
    return;
  }
  const direction = sampleDirection();
  if (!direction) {
    toast('no Language Pair is available');
    return;
  }
  await audioOut.resume();

  const packs = [...$('packs').selectedOptions].map((option) => option.value);
  send({ type: 'start', mode: $('mode').value, src: direction.src, tgt: direction.tgt, packs });
  send({ type: 'sample', name: sample.id });

  stopStreaming?.();
  stopStreaming = streamSample(sample, (buffer) => {
    if (ws?.readyState === WebSocket.OPEN) ws.send(buffer);
  });
  $('start').disabled = true;
  $('stop').disabled = false;
  setStatus('listening');
}

function stop() {
  stopStreaming?.();
  stopStreaming = null;
  stopMic();
  send({ type: 'stop' });
  $('start').disabled = false;
  $('stop').disabled = true;
  setStatus('stopped');
}

async function toggleMic() {
  if (mic) {
    await stopMic();
    return;
  }
  try {
    send({ type: 'input', source: 'live' });
    const stream = await navigator.mediaDevices.getUserMedia({
      audio: { echoCancellation: true, noiseSuppression: true, autoGainControl: true },
    });
    const context = new AudioContext({ sampleRate: 16000 });
    await context.audioWorklet.addModule('/mic-worklet.js');
    const source = context.createMediaStreamSource(stream);
    const node = new AudioWorkletNode(context, 'mic-frames');
    const mute = context.createGain();
    mute.gain.value = 0;
    source.connect(node).connect(mute).connect(context.destination);
    node.port.onmessage = (event) => {
      if (ws?.readyState === WebSocket.OPEN) ws.send(event.data);
    };
    mic = { stream, context };
    $('mic').textContent = 'Stop mic';
    setStatus('mic on');
  } catch (error) {
    toast(`microphone: ${error.message}`);
  }
}

async function stopMic() {
  if (!mic) return;
  for (const track of mic.stream.getTracks()) track.stop();
  await mic.context.close();
  mic = null;
  $('mic').textContent = 'Mic';
}

function send(message) {
  if (ws?.readyState === WebSocket.OPEN) ws.send(JSON.stringify(message));
}

function setStatus(text) {
  $('status').textContent = text;
}

function toast(text) {
  const element = document.createElement('div');
  element.className = 'toast';
  element.textContent = text;
  $('toasts').append(element);
  setTimeout(() => element.remove(), 6000);
}
