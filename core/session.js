// The deep module: one Session is driven by protocol client messages, audio
// frames, an injected clock and injected Providers, and emits protocol events.
// No DOM and no host built-ins here.

import { createCutPolicy } from './cut.js';
import { createEchoFilter } from './echo.js';
import { createGating } from './gating.js';
import { createHoldPolicy } from './hold.js';
import { selectGlossary } from './packs-registry.js';
import { buildTranslatorRequest } from './translator-request.js';

const CONTEXT_WINDOW = 8;
// Spec amendment: when silence Cuts but the ASR has not confirmed the latest
// Partial yet, wait up to 600 ms for the Stable Prefix to catch up (a late
// final). Other Cut triggers never wait.
const CUT_CATCHUP_MS = 600;
// shortcut: 3 s bounded ASR drain after stop; raise it if a Provider needs
// longer to flush its final line.
const ASR_DRAIN_MS = 3000;
const ASR_DRAIN_POLL_MS = 50;

// Layer III bitrate tables (kbps) for MPEG1 and MPEG2/2.5.
const MP3_BITRATES = {
  1: [0, 32, 40, 48, 56, 64, 80, 96, 112, 128, 160, 192, 224, 256, 320, 0],
  2: [0, 8, 16, 24, 32, 40, 48, 56, 64, 80, 96, 112, 128, 144, 160, 0],
};

// shortcut: the first frame's bitrate; accurate for the CBR mp3 produced by
// Kokoro/Azure/Edge. Scan every frame header when a VBR Provider is added.
function mp3Kbps(data, bytes) {
  let i = 0;
  if (bytes > 10 && data[0] === 0x49 && data[1] === 0x44 && data[2] === 0x33) {
    i = 10 + (((data[6] & 0x7f) << 21) | ((data[7] & 0x7f) << 14) | ((data[8] & 0x7f) << 7) | (data[9] & 0x7f));
  }
  for (; i + 4 <= bytes; i++) {
    if (data[i] !== 0xff || (data[i + 1] & 0xe0) !== 0xe0) continue;
    const version = (data[i + 1] >> 3) & 3;
    const layer = (data[i + 1] >> 1) & 3;
    if (version === 1 || layer !== 1) continue;
    const kbps = MP3_BITRATES[version === 3 ? 1 : 2][(data[i + 2] >> 4) & 0xf];
    if (kbps > 0) return kbps;
  }
  return 0;
}

function commonPrefix(a, b) {
  const left = [...a];
  const right = [...b];
  let i = 0;
  while (i < left.length && i < right.length && left[i] === right[i]) i++;
  return left.slice(0, i).join('');
}

export function createSession({
  providers,
  clock,
  registry,
  pairConfig = {},
  forcedInstruction = '',
  emit = () => {},
  log = () => {},
}) {
  const { asr, translator, tts } = providers;
  const demo = [asr, translator, tts].some((provider) => provider.name === 'mock');
  // Adapters that emit only revisable hypotheses leave Stable Prefix to us.
  const revisable = asr.revisable === true;
  let cut = null;
  let holdTimer = null;
  let drainTimer = null;
  let cutWaitTimer = null;
  let drainDeadline = 0;
  const hold = createHoldPolicy({ instruction: forcedInstruction });
  const gating = createGating();
  const echo = createEchoFilter();

  const state = {
    running: false,
    mode: 'listen',
    start: null,
    directions: null,
    direction: null,
    packs: [],
    input: 'live',
    audioMs: 0,
    stable: '',
    partial: '',
    lastHypothesis: '',
    committedPrefix: '',
    lastSpeechAt: null,
    ptt: false,
    turn: null,
    segId: 0,
    echoSuspects: 0,
    ownSpeechPending: 0,
    playbacks: new Map(),
    mockMicErrorSent: false,
    contexts: new Map(),
    queue: Promise.resolve(),
  };

  function directionRef(direction = state.direction) {
    return { src: direction.src, tgt: direction.tgt };
  }

  function sameDirection(a, b) {
    return Boolean(a && b && a.src === b.src && a.tgt === b.tgt);
  }

  function converse() {
    return state.mode === 'converse';
  }

  function emitError(provider, error) {
    emit({
      type: 'error',
      provider,
      kind: error?.kind ?? 'protocol',
      message: error?.message ?? String(error),
      fallback: error?.fallback,
    });
  }

  function enqueue(work) {
    state.queue = state.queue.then(work).catch((error) => emitError('session', error));
  }

  const clockTimers = typeof clock.setTimeout === 'function' && typeof clock.clearTimeout === 'function';

  function schedule(fn, ms) {
    return clockTimers ? clock.setTimeout(fn, ms) : setTimeout(fn, ms);
  }

  function clearTimer(id) {
    if (id === null) return null;
    if (clockTimers) clock.clearTimeout(id);
    else clearTimeout(id);
    return null;
  }

  function cancelDrain() {
    drainTimer = clearTimer(drainTimer);
  }

  function cancelCutWait() {
    cutWaitTimer = clearTimer(cutWaitTimer);
  }

  // Provider-agnostic: both adapters emit the whole current hypothesis as a
  // Partial and finalized/agreed text as the Stable Prefix, so the stable text
  // is the covered suffix. Pending speech means the Partial has a tail the
  // Stable Prefix does not cover yet (ignoring spacing and punctuation).
  function asrPending() {
    const strip = (text) => text.replace(/[\p{P}\s]/gu, '').toLowerCase();
    const partial = strip(state.partial);
    return partial !== '' && !strip(state.stable).endsWith(partial);
  }

  function armCutWait() {
    cutWaitTimer = clearTimer(cutWaitTimer);
    cutWaitTimer = schedule(() => {
      cutWaitTimer = null;
      closeSegment(clock.now(), true);
    }, CUT_CATCHUP_MS);
  }

  function clearHoldTimer() {
    holdTimer = clearTimer(holdTimer);
  }

  function releaseHeld() {
    clearHoldTimer();
    hold.reset();
  }

  function armHoldTimer() {
    if (holdTimer !== null || !hold.held) return;
    holdTimer = schedule(() => {
      holdTimer = null;
      enqueue(() => forceHeld());
    }, hold.msLeft(clock.now()));
  }

  function start(message) {
    const direction = registry.directions.find(
      (candidate) => candidate.src === message.src && candidate.tgt === message.tgt,
    );
    if (!direction) {
      emit({
        type: 'error',
        kind: 'protocol',
        message: `unknown direction: ${message.src}->${message.tgt}`,
      });
      return;
    }
    const mode = message.mode ?? 'listen';
    let directions = null;
    if (mode === 'converse') {
      const reverse = registry.directions.find(
        (candidate) => candidate.src === direction.tgt && candidate.tgt === direction.src,
      );
      if (!reverse) {
        emit({
          type: 'error',
          kind: 'protocol',
          message: `converse needs both directions: ${direction.src}->${direction.tgt} and ${direction.tgt}->${direction.src}`,
        });
        return;
      }
      directions = { me: direction, them: reverse };
    }

    const packs = [];
    for (const name of Array.isArray(message.packs) ? message.packs : []) {
      if (registry.packs.some((pack) => pack.name === name)) packs.push(name);
      else emit({ type: 'error', kind: 'protocol', message: `unknown pack: ${name}` });
    }

    cancelDrain();
    state.running = true;
    state.mode = mode;
    state.start = { src: direction.src, tgt: direction.tgt };
    state.packs = packs;
    state.directions = directions;
    state.direction = direction;
    state.turn = mode === 'converse' ? 'me' : null;
    state.input = 'live';
    state.audioMs = 0;
    state.stable = '';
    resetAgreement();
    state.lastSpeechAt = null;
    state.ptt = false;
    state.segId = 0;
    state.echoSuspects = 0;
    state.mockMicErrorSent = false;
    state.contexts.clear();
    releaseHeld();
    resetPlayback();
    cut = createCutPolicy(pairConfig[`${direction.src}->${direction.tgt}`]);
    // shortcut: one ASR stream per Provider instance, re-opened with the Turn's
    // Direction; per-Direction streams need a Provider factory seam.
    asr.open?.(directionRef());
    emit({ type: 'state', state: 'listening' });
    if (mode === 'converse') emit({ type: 'turn', who: 'me', auto: false });
  }

  function handleMessage(message) {
    if (!message || typeof message !== 'object') return;
    switch (message.type) {
      case 'start':
        start(message);
        break;
      case 'sample':
        if (!state.running) return;
        state.input = 'sample';
        state.audioMs = 0;
        resetAgreement();
        asr.open?.(directionRef());
        break;
      case 'input':
        state.input = message.source === 'sample' ? 'sample' : 'live';
        break;
      case 'turn':
        setTurn(message.who, { auto: false });
        break;
      case 'playback':
        handlePlayback(message);
        break;
      case 'ptt':
        state.ptt = message.state === 'down';
        if (state.ptt) cancelCutWait();
        else closeSegment(clock.now(), true);
        break;
      case 'stop':
        state.running = false;
        releaseHeld();
        resetPlayback();
        asr.close?.();
        drainAsrAfterClose();
        break;
      default:
        break;
    }
  }

  function gatingBlocked() {
    if (tts.name === 'system') return false;
    return gating.blocked(clock.now()) || hasGatingPlayback();
  }

  function hasGatingPlayback() {
    for (const entry of state.playbacks.values()) if (entry.gates) return true;
    return false;
  }

  function setTurn(who, { auto = false } = {}) {
    if (!converse() || !state.directions?.[who] || state.turn === who) return;
    closeSegment(clock.now(), true);
    state.turn = who;
    state.direction = state.directions[who];
    state.audioMs = 0;
    resetAgreement();
    cut = createCutPolicy(pairConfig[`${state.direction.src}->${state.direction.tgt}`]);
    // A Held Segment never merges into another Turn's Direction.
    if (hold.held) enqueue(() => forceHeld());
    asr.open?.(directionRef());
    emit({ type: 'turn', who, auto });
  }

  function maybeAutoSwitch() {
    if (!converse() || state.turn !== 'me' || state.ownSpeechPending > 0) return;
    setTurn('them', { auto: true });
  }

  function audioDurationMs({ format, data } = {}) {
    const bytes = data?.byteLength ?? data?.length ?? 0;
    if (format === 'wav' && bytes >= 44) {
      const view = new DataView(data.buffer, data.byteOffset ?? 0, bytes);
      const byteRate = view.getUint32(28, true);
      if (byteRate > 0) return ((bytes - 44) / byteRate) * 1000;
    }
    if (format === 'mp3') {
      const kbps = mp3Kbps(data, bytes);
      return kbps > 0 ? (bytes * 8) / kbps : 0;
    }
    return 0;
  }

  function armPlaybackEstimate(segId, entry) {
    entry.timer = clearTimer(entry.timer);
    entry.timer = schedule(() => {
      entry.timer = null;
      endPlayback(segId);
    }, entry.durationMs + 1500);
  }

  function beginOwnSpeech(segId, text, audio) {
    echo.remember(segId, text);
    state.ownSpeechPending += 1;
    const entry = { gates: tts.name !== 'system', durationMs: audio ? audioDurationMs(audio) : 0, timer: null };
    state.playbacks.set(segId, entry);
    if (entry.gates) armPlaybackEstimate(segId, entry);
  }

  function endPlayback(segId) {
    const entry = state.playbacks.get(segId);
    if (!entry) return;
    state.playbacks.delete(segId);
    entry.timer = clearTimer(entry.timer);
    const now = clock.now();
    echo.end(segId, now);
    if (tts.name !== 'system' && !hasGatingPlayback()) gating.end(now);
    state.ownSpeechPending = Math.max(0, state.ownSpeechPending - 1);
    maybeAutoSwitch();
  }

  function handlePlayback(message) {
    const segId = Number(message.seg_id);
    const entry = state.playbacks.get(segId);
    if (!Number.isInteger(segId) || !entry) return;
    if (message.state === 'start') {
      if (entry.gates) armPlaybackEstimate(segId, entry);
    } else if (message.state === 'end') {
      endPlayback(segId);
    }
  }

  function resetPlayback() {
    for (const entry of state.playbacks.values()) entry.timer = clearTimer(entry.timer);
    state.playbacks.clear();
    state.ownSpeechPending = 0;
    gating.reset();
    echo.reset();
  }

  function handleAudio(pcm) {
    if (!state.running || !state.direction) return;
    if (state.input === 'live' && gatingBlocked()) return;

    if (state.input === 'live' && asr.acceptsLive === false) {
      if (!state.mockMicErrorSent) {
        state.mockMicErrorSent = true;
        emit({
          type: 'error',
          provider: 'asr',
          kind: 'protocol',
          message:
            'Mock ASR only accepts bundled samples. Configure a real ASR provider to use the microphone.',
          fallback: 'Play the bundled sample, or set ASR_PROVIDER to a real provider.',
        });
      }
      return;
    }

    state.audioMs += pcm.length / 16;

    const asrEvents = pushAsr(pcm);
    if (asrEvents === null) return;
    handleAsrEvents(asrEvents);

    const vad = cut.feed(pcm);
    const now = clock.now();
    if (vad.speech) {
      state.lastSpeechAt = now;
      cancelCutWait(); // speech resumed: normal accumulation continues
    }
    if (state.ptt) return; // automatic Cuts are suspended while PTT is held
    if (vad.cut) {
      if (cutWaitTimer !== null) return; // already waiting for the ASR
      if (asrPending()) armCutWait();
      else closeSegment(now, true);
    } else if (cut.accumulated(now)) {
      closeSegment(now, false);
    }
  }

  function pushAsr(pcm = null) {
    try {
      return asr.push?.({ pcm, audioMs: state.audioMs }) ?? [];
    } catch (error) {
      emitError('asr', error);
      return null;
    }
  }

  function handleAsrEvents(asrEvents) {
    for (const event of asrEvents) {
      if (event.type === 'partial') handlePartial(event.text);
      else if (event.type === 'stable') addStable(event.text, event.glue);
    }
  }

  // After stop the Provider flushes its final line only on a later message, so
  // keep polling push() for a bounded window and let the normal Cut commit it.
  function drainAsr() {
    drainTimer = null;
    if (clock.now() >= drainDeadline) {
      closeSegment(clock.now(), true);
      return;
    }
    const events = pushAsr();
    if (events === null) {
      closeSegment(clock.now(), true);
      return;
    }
    handleAsrEvents(events);
    closeSegment(clock.now(), true); // commit each flushed batch; a no-op when empty
    drainTimer = schedule(drainAsr, ASR_DRAIN_POLL_MS);
  }

  function drainAsrAfterClose() {
    cancelDrain();
    drainDeadline = clock.now() + ASR_DRAIN_MS;
    drainAsr();
  }

  function handlePartial(text) {
    state.partial = text;
    if (revisable) {
      const common = commonPrefix(state.lastHypothesis, text);
      const stable = common.slice(state.committedPrefix.length);
      state.lastHypothesis = text;
      if (stable) {
        state.committedPrefix = common;
        addStable(stable, ''); // successive prefixes of one hypothesis
      }
    }
    // A Partial is display-only: never translated and never committed.
    emit({ type: 'asr_partial', text, direction: directionRef() });
  }

  function addStable(text, glue = ' ') {
    if (!text) return;
    if (state.input === 'live' && echo.isEcho(text, clock.now())) {
      state.echoSuspects++;
      return;
    }
    state.stable = state.stable ? `${state.stable}${glue}${text}` : text;
    const now = clock.now();
    const clauseCut = cut.stable(state.stable, now).cut;
    if (!state.ptt && clauseCut) closeSegment(now, false);
    else if (cutWaitTimer !== null && !asrPending()) closeSegment(now, true);
  }

  function closeSegment(now, utteranceEnd) {
    cancelCutWait();
    if (!cut) return;
    const direction = state.direction;
    const source = state.stable.trim();
    const speechEnd = state.lastSpeechAt ?? now;
    cut.reset();
    state.stable = '';
    state.lastSpeechAt = null;
    if (utteranceEnd) resetAgreement();
    if (!source) return;
    enqueue(() => commit(source, speechEnd, direction, now));
  }

  function resetAgreement() {
    state.lastHypothesis = '';
    state.committedPrefix = '';
    state.partial = '';
  }

  async function commit(source, speechEnd, direction, finalizedAt) {
    if (hold.held && sameDirection(hold.held.direction, direction)) {
      // A Held Segment is merged with this Cut and retried.
      const merged = hold.merge(source, speechEnd);
      merged.finalizedAt = finalizedAt;
      await translate(merged, { forced: false });
      return;
    }
    if (hold.held) {
      // A Segment of another Direction never merges into the Held one.
      await translate(hold.force(), { forced: true });
    }
    await translate(
      { held: false, source, speechEnd, merges: 0, forced: false, direction, finalizedAt },
      { forced: false },
    );
  }

  async function forceHeld() {
    if (!hold.held || hold.held.forced) return;
    await translate(hold.force(), { forced: true });
  }

  async function translate(pending, { forced }) {
    const direction = pending.direction ?? state.direction;
    const ref = directionRef(direction);
    const key = `${ref.src}\u0000${ref.tgt}`;
    const context = state.contexts.get(key) ?? [];
    const request = buildTranslatorRequest({
      prompt: direction.prompt,
      context,
      text: pending.source,
      glossary: selectGlossary(registry, { ...direction, packs: state.packs }),
    });
    const { system, user } = forced ? hold.withInstruction(request) : request;

    let text = '';
    let firstToken = null;
    try {
      for await (const chunk of translator.translate({ system, user, text: pending.source, direction: ref })) {
        if (firstToken === null) firstToken = clock.now();
        text += chunk;
      }
    } catch (error) {
      emitError('translator', error);
      return;
    }

    // A reasoning model can leak its scratchpad into the content stream before
    // the answer. Drop everything through the last closing "think" tag before
    // Hold, Commit, TTS or Rolling Context see it. The whole answer is already
    // buffered above, so this adds no latency.
    const leak = text.match(/^[\s\S]*<\/[^>]*think[^>]*>/i);
    if (leak) text = text.slice(leak[0].length);

    // Models may pad the answer; trim once here so Hold, Commit, TTS and
    // Rolling Context all see the same text.
    text = text.trim();
    if (leak && !text) {
      emitError('translator', {
        kind: 'protocol',
        message: `translator returned no text after leaked reasoning (${translator.preset || translator.name || 'translator'})`,
        fallback: 'Switch LLM_PRESET, or check the endpoint and model with LLM_BASE_URL / LLM_MODEL.',
      });
      return;
    }
    if (hold.inspect(text)) {
      return handleHold(pending, { forced, firstToken, direction });
    }
    await commitTranslation(pending, text, firstToken, direction, key, context);
  }

  function handleHold(pending, { forced, firstToken, direction }) {
    if (forced || pending.forced) {
      // A Forced Translation that still returns ⏸ commits the source only.
      commitSourceOnly(pending, firstToken, direction);
      return;
    }
    if (!hold.held) {
      const held = hold.markHeld(pending.source, pending.speechEnd, clock.now());
      held.finalizedAt = pending.finalizedAt;
      held.direction = direction;
      armHoldTimer();
    }
    emit({ type: 'hold', held: true, merges: hold.held.merges, direction: directionRef(direction) });
    if (hold.expired(clock.now())) {
      // The retry returned ⏸ after the limit: force it now.
      clearHoldTimer();
      return translate(hold.force(), { forced: true });
    }
  }

  function commitSourceOnly(pending, firstToken, direction) {
    const segId = ++state.segId;
    const ref = directionRef(direction);
    emit({
      type: 'segment',
      seg_id: segId,
      src: pending.source,
      tgt: '',
      direction: ref,
      latency_ms: clock.now() - pending.speechEnd,
      speak: false,
      demo,
    });
    log({
      seg_id: segId,
      direction: ref,
      speech_end: pending.speechEnd,
      asr_final: pending.source,
      asr_final_at: pending.finalizedAt ?? null,
      first_token: firstToken,
      tts_first_chunk: null,
      held: true,
      merges: pending.merges,
      forced: true,
      forced_fail: true,
      echo_suspect: state.echoSuspects,
    });
    releaseHeld();
  }

  async function commitTranslation(pending, text, firstToken, direction, key, context) {
    const segId = ++state.segId;
    const ref = directionRef(direction);
    const speak = !converse() || sameDirection(ref, state.start);
    const segment = {
      type: 'segment',
      seg_id: segId,
      src: pending.source,
      tgt: text,
      direction: ref,
      latency_ms: clock.now() - pending.speechEnd,
      speak,
      demo,
    };
    emit(segment);

    context.push({ src: pending.source, tgt: text });
    if (context.length > CONTEXT_WINDOW) context.shift();
    state.contexts.set(key, context);
    releaseHeld();

    let ttsFirstChunk = null;
    if (speak) {
      if (tts.name === 'system') {
        // Text-only TTS: the client speaks the Segment itself, so no audio
        // events and no mic gating; the echo filter still guards the text.
        beginOwnSpeech(segId, text, null);
      } else {
        try {
          const audio = await tts.synthesize({ text, seg_id: segId, direction: ref });
          ttsFirstChunk = clock.now();
          emit({ type: 'tts_begin', seg_id: segId, format: audio.format });
          emit({ type: 'tts_chunk', seg_id: segId, data: audio.data });
          emit({ type: 'tts_end', seg_id: segId });
          beginOwnSpeech(segId, text, audio);
        } catch (error) {
          emitError('tts', error);
        }
      }
    }

    log({
      seg_id: segId,
      direction: ref,
      speech_end: pending.speechEnd,
      asr_final: pending.source,
      asr_final_at: pending.finalizedAt ?? null,
      first_token: firstToken,
      tts_first_chunk: ttsFirstChunk,
      held: Boolean(pending.held),
      merges: pending.merges,
      forced: Boolean(pending.forced),
      forced_fail: false,
      echo_suspect: state.echoSuspects,
    });
  }

  function hello() {
    return {
      type: 'hello',
      pairs: registry.pairs,
      packs: registry.packs.map(({ name, src, tgt }) => ({ name, src, tgt })),
      presetsInUse: [],
      demo,
    };
  }

  return {
    hello,
    handleMessage,
    handleAudio,
    idle: () => state.queue,
    close: () => {
      state.running = false;
      cancelDrain();
      cancelCutWait();
      releaseHeld();
      resetPlayback();
      asr.close?.();
    },
  };
}
