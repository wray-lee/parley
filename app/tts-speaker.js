// The Device Host speaks `segment {speak:true}` itself because the core's
// system TTS Provider is text-only. The third-party TTS plugin's speech:start
// and speech:finish events close the gate; a duration estimate is the timeout
// backstop when a platform never emits them. Web Speech is the fallback, and
// subtitles-only is the last resort (the Device Host shows a notice).

// shortcut: a duration estimate is the timeout backstop when a platform never
// emits speech:finish; replace it with engine callbacks if one is found.
const MIN_WAIT_MS = 1500;
const PER_CHAR_MS = 180;

export function estimateSpeechMs(text) {
  return Math.min(30_000, MIN_WAIT_MS + String(text ?? '').length * PER_CHAR_MS);
}

export function createTauriTts(
  { speak, stop, onSpeechEvent } = {},
  { warn = () => {}, estimateMs = estimateSpeechMs } = {},
) {
  if (typeof speak !== 'function' || typeof onSpeechEvent !== 'function') return null;
  const pending = new Map();
  const settled = new Set();
  let untracked = null;

  const settle = (id) => {
    if (id != null && pending.has(id)) {
      const entry = pending.get(id);
      pending.delete(id);
      clearTimeout(entry.timer);
      entry.resolve();
      return;
    }
    if (untracked) {
      const entry = untracked;
      untracked = null;
      clearTimeout(entry.timer);
      entry.resolve();
      return;
    }
    // The engine can finish a very short utterance before speak() resolves.
    if (id != null) settled.add(id);
  };

  for (const name of ['speech:finish', 'speech:cancel', 'speech:error']) {
    Promise.resolve(
      onSpeechEvent(name, (event) => settle(event?.id ?? event?.utteranceId)),
    ).catch((error) => warn(`TTS event listener failed (${error?.message ?? error})`));
  }

  return {
    name: 'system-plugin',
    async speak(text, language) {
      const body = String(text ?? '').trim();
      if (!body) return;
      let result;
      try {
        result = await speak({ text: body, language: language || undefined, queueMode: 'add' });
      } catch (error) {
        throw new Error(`system TTS failed: ${error?.message ?? error}`);
      }
      const id = result?.utteranceId ?? null;
      if (id != null && settled.delete(id)) return;
      await new Promise((resolve) => {
        const entry = { resolve, timer: null };
        entry.timer = setTimeout(() => {
          if (id != null) pending.delete(id);
          else untracked = null;
          resolve();
        }, estimateMs(body));
        if (id != null) pending.set(id, entry);
        else untracked = entry;
      });
    },
    stop() {
      try {
        stop?.();
      } catch {
        // the engine is being discarded anyway
      }
      for (const entry of pending.values()) {
        clearTimeout(entry.timer);
        entry.resolve();
      }
      pending.clear();
      if (untracked) {
        clearTimeout(untracked.timer);
        untracked.resolve();
        untracked = null;
      }
    },
  };
}

export function createWebSpeechTts({
  synthesis = globalThis.speechSynthesis,
  Utterance = globalThis.SpeechSynthesisUtterance,
  estimateMs = estimateSpeechMs,
} = {}) {
  if (!synthesis || typeof Utterance !== 'function') return null;
  let cancelCurrent = null;

  return {
    name: 'web-speech',
    speak(text, language) {
      const body = String(text ?? '').trim();
      if (!body) return Promise.resolve();
      return new Promise((resolve) => {
        const utterance = new Utterance(body);
        if (language) utterance.lang = language;
        let done = false;
        const finish = () => {
          if (done) return;
          done = true;
          clearTimeout(timer);
          if (cancelCurrent === finish) cancelCurrent = null;
          resolve();
        };
        const timer = setTimeout(finish, estimateMs(body));
        utterance.onend = finish;
        utterance.onerror = finish;
        cancelCurrent = finish;
        try {
          synthesis.speak(utterance);
        } catch {
          finish();
        }
      });
    },
    stop() {
      try {
        synthesis.cancel();
      } catch {
        // no engine to cancel
      }
      cancelCurrent?.();
    },
  };
}

function createFallbackSpeaker(primary, fallback, warn) {
  return {
    name: primary.name,
    async speak(text, language) {
      try {
        await primary.speak(text, language);
      } catch (error) {
        if (!fallback) throw error;
        warn(`TTS plugin failed (${error?.message ?? error}); using Web Speech`);
        await fallback.speak(text, language);
      }
    },
    stop() {
      primary.stop();
      fallback?.stop();
    },
  };
}

export async function loadSystemSpeaker({ warn = () => {} } = {}) {
  const web = createWebSpeechTts({ estimateMs: estimateSpeechMs });
  if (globalThis.__TAURI_INTERNALS__) {
    try {
      const api = await import('./vendor/plugin-tts.js');
      const plugin = createTauriTts(api, { warn });
      if (plugin) return createFallbackSpeaker(plugin, web, warn);
    } catch (error) {
      warn(`TTS plugin unavailable (${error?.message ?? error}); using Web Speech`);
    }
  }
  return web;
}
