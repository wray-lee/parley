// Plays each TTS chunk through an <audio> element. Chunks are independently
// decodable mp3, so there is no Web Audio graph and no MediaSource.
// Segments play strictly in seg_id order; a later Segment waits for an
// earlier one even when its audio arrives first.
// shortcut: main.js calls play(format, data) without seg ids, so that path
// uses arrival order; wire begin()/end() to tts_begin/tts_end to order by id.
// shortcut: a Segment with no audio leaves a gap until Skip/Stop.

const MIME = {
  mp3: 'audio/mpeg',
  wav: 'audio/wav',
  opus: 'audio/ogg',
  flac: 'audio/flac',
  aac: 'audio/aac',
};

const CACHE_SEGMENTS = 20;

export function createAudioOut({
  Audio: AudioCtor = globalThis.Audio,
  URL: urlApi = globalThis.URL,
  speechSynthesis: speech = globalThis.speechSynthesis,
  SpeechSynthesisUtterance: Utterance = globalThis.SpeechSynthesisUtterance,
  controls = true,
  onPlayback = () => {},
} = {}) {
  const segments = new Map(); // seg_id -> { chunks, ended }
  const cache = new Map(); // seg_id -> { format, chunks } for playOne()
  const flat = []; // chunks with no seg_id (no begin() has been seen)
  let structured = false;
  let nextSeg = 1;
  let oneShot = 0;
  let oneShotMode = false;
  let draining = false;
  let current = null; // { audio, finish }
  let oneButton = null;

  function remember(segId, format, data) {
    const chunks = cache.get(segId) ?? [];
    chunks.push({ format, data });
    cache.set(segId, chunks);
    while (cache.size > CACHE_SEGMENTS) cache.delete(cache.keys().next().value);
  }

  function entryFor(segId) {
    let entry = segments.get(segId);
    if (!entry) {
      entry = { chunks: [], ended: false };
      segments.set(segId, entry);
    }
    return entry;
  }

  function playChunk({ format, data }) {
    if (!AudioCtor || !urlApi) return Promise.resolve();
    const url = urlApi.createObjectURL(new Blob([data], { type: MIME[format] ?? 'audio/mpeg' }));
    const audio = new AudioCtor(url);
    return new Promise((resolve) => {
      let settled = false;
      const finish = () => {
        if (settled) return;
        settled = true;
        audio.pause?.();
        if (current?.audio === audio) current = null;
        urlApi.revokeObjectURL(url);
        resolve();
      };
      current = { audio, finish };
      audio.addEventListener('ended', finish, { once: true });
      audio.addEventListener('error', finish, { once: true });
      Promise.resolve(audio.play()).catch(finish);
    });
  }

  async function drain() {
    if (draining) return;
    draining = true;
    try {
      for (;;) {
        if (structured) {
          const segId = nextSeg;
          const entry = segments.get(segId);
          if (!entry || (!entry.chunks.length && !entry.ended)) break;

          if (!entry.chunks.length) {
            segments.delete(segId);
            nextSeg += 1;
            if (entry.started) onPlayback(segId, 'end');
            if (oneShot === segId) {
              endOneShot();
              break;
            }
            continue;
          }

          const chunk = entry.chunks.shift();
          if (!entry.started) {
            entry.started = true;
            onPlayback(segId, 'start');
          }
          await playChunk(chunk);
          chunk.resolve();
          if (segments.get(segId) !== entry) continue; // skipped or stopped

          if (entry.ended && !entry.chunks.length) {
            segments.delete(segId);
            nextSeg += 1;
            onPlayback(segId, 'end');
            if (oneShot === segId) {
              endOneShot();
              break;
            }
          }
          continue;
        }

        const chunk = flat.shift();
        if (!chunk) break;
        await playChunk(chunk);
        chunk.resolve();
      }
    } finally {
      draining = false;
    }
  }

  function begin(segId) {
    structured = true;
    // Spoken Segments are not contiguous: Converse has subtitle-only ones
    // between them, so jump the queue to the first Segment with audio.
    if (segId != null && !segments.has(nextSeg)) nextSeg = segId;
    entryFor(segId);
    if (oneShotMode && !oneShot) oneShot = segId;
    drain();
  }

  function play(format, data, segId) {
    return new Promise((resolve) => {
      const chunk = { format, data, resolve };
      if (segId != null) {
        structured = true;
        remember(segId, format, data);
        entryFor(segId).chunks.push(chunk);
      } else if (structured) {
        entryFor(nextSeg).chunks.push(chunk);
      } else {
        flat.push(chunk);
      }
      drain();
    });
  }

  function end(segId) {
    if (segId == null) return;
    entryFor(segId).ended = true;
    drain();
  }

  function drop(entry) {
    for (const chunk of entry.chunks.splice(0)) chunk.resolve();
  }

  function skip() {
    if (current) current.finish();
    if (structured) {
      const entry = segments.get(nextSeg);
      if (entry) {
        drop(entry);
        segments.delete(nextSeg);
        nextSeg += 1;
      }
    } else {
      for (const chunk of flat.splice(0)) chunk.resolve();
    }
    drain();
  }

  function next() {
    if (current) skip();
    else drain();
  }

  function endOneShot() {
    oneShot = 0;
    oneShotMode = false;
    oneButton?.setAttribute('aria-pressed', 'false');
  }

  function stop() {
    if (current) current.finish();
    for (const entry of segments.values()) drop(entry);
    segments.clear();
    for (const chunk of flat.splice(0)) chunk.resolve();
    speech?.cancel?.();
    structured = false;
    nextSeg = 1;
    endOneShot();
  }

  // Replays a Segment that is still in the bounded cache.
  function playOne(segId) {
    const cached = cache.get(segId);
    if (!cached) return false;
    stop();
    structured = true;
    nextSeg = segId;
    oneShot = segId;
    const entry = entryFor(segId);
    entry.ended = true;
    for (const chunk of cached) {
      entry.chunks.push({ format: chunk.format, data: chunk.data, resolve: () => {} });
    }
    drain();
    return true;
  }

  // Generic tag match (ja matches ja-JP, zh matches zh-CN): no per-language map.
  function voiceFor(voices, lang) {
    const tag = String(lang ?? '').toLowerCase().split(/[-_]/)[0];
    if (!tag) return null;
    return voices.find(
      (voice) => String(voice.lang ?? '').toLowerCase().split(/[-_]/)[0] === tag,
    ) ?? null;
  }

  // Resolves true when the text was spoken; false when speech is unavailable or
  // the target language has no installed voice (subtitles continue either way).
  async function speak(text, lang) {
    if (!speech || !Utterance) return false;
    const voices = speech.getVoices?.() ?? [];
    const voice = voiceFor(voices, lang);
    // An empty list means voices are still loading; let the browser choose.
    if (voices.length && !voice) return false;
    await new Promise((resolve) => {
      const utterance = new Utterance(text);
      utterance.lang = lang ?? '';
      if (voice) utterance.voice = voice;
      utterance.onend = resolve;
      utterance.onerror = resolve;
      speech.speak(utterance);
    });
    return true;
  }

  function cancelSpeech() {
    speech?.cancel?.();
  }

  async function resume() {
    // <audio> needs no AudioContext unlock; kept for the client's start path.
  }

  function mountControls(parent = typeof document === 'undefined' ? null : document.body) {
    if (!parent) return null;
    const doc = parent.ownerDocument ?? parent;
    const bar = doc.createElement('div');
    bar.dataset.audioControls = '';
    bar.style.cssText = 'display:flex;gap:.5rem;padding:.5rem 1rem;';

    const button = (label, action) => {
      const element = doc.createElement('button');
      element.type = 'button';
      element.textContent = label;
      element.addEventListener('click', action);
      bar.append(element);
      return element;
    };

    button('Play next', next);
    button('Skip', skip);
    button('Stop', stop);
    const buttonLater = button('Play one', () => {
      oneShotMode = !oneShotMode;
      buttonLater.setAttribute('aria-pressed', String(oneShotMode));
    });
    buttonLater.setAttribute('aria-pressed', 'false');
    oneButton = buttonLater;
    parent.append(bar);
    return bar;
  }

  if (controls) mountControls();

  return {
    play, begin, end, next, skip, stop, playOne, speak, cancelSpeech, resume, mountControls,
  };
}
