// Mic gate for Converse: the web client keeps its microphone live while the
// system TTS plugin speaks, so the Device Host drops its audio frames until
// speech:finish plus a 500 ms tail. The core's own gate is inactive for the
// text-only system Provider, hence this app-side gate.

export function createSpeechGate({
  tailMs = 500,
  now = () => Date.now(),
  schedule = setTimeout,
  cancel = clearTimeout,
} = {}) {
  let speaking = 0;
  let tailUntil = 0;
  let timer = null;

  return {
    get blocked() {
      return speaking > 0 || now() < tailUntil;
    },
    get speaking() {
      return speaking > 0;
    },
    begin() {
      speaking += 1;
      tailUntil = 0;
    },
    end() {
      speaking = Math.max(0, speaking - 1);
      if (speaking > 0) return;
      tailUntil = now() + tailMs;
      if (timer !== null) cancel(timer);
      timer = schedule(() => {
        timer = null;
        tailUntil = 0;
      }, tailMs);
    },
    reset() {
      speaking = 0;
      tailUntil = 0;
      if (timer !== null) cancel(timer);
      timer = null;
    },
  };
}
