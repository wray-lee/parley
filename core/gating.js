// Gating seam: microphones are withheld while the app's own audio speech is
// pending and for a short tail after the last Segment finishes. The Session
// owns playback state; this module only remembers when the tail ends.

export const GATING_TAIL_MS = 500;

export function createGating({ tailMs = GATING_TAIL_MS } = {}) {
  let tailUntil = 0;

  return {
    end(now) {
      tailUntil = Math.max(tailUntil, now + tailMs);
    },
    blocked(now) {
      return now < tailUntil;
    },
    reset() {
      tailUntil = 0;
    },
  };
}
