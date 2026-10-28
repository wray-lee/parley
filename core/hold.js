// Hold policy: ⏸ detection and the Held Segment's state. The Session owns
// translation, timers and logging; this module only tracks what is Held and
// builds the Forced Translation request from the prompt-asset instruction.

const HOLD_MARK = '⏸';
const HOLD_LIMIT_MS = 2500;

export function createHoldPolicy({ instruction = '', limitMs = HOLD_LIMIT_MS } = {}) {
  let held = null;

  return {
    inspect(text) {
      return String(text).trim() === HOLD_MARK;
    },

    get held() {
      return held;
    },

    markHeld(source, speechEnd, now) {
      if (!held) held = { held: true, source, speechEnd, heldAt: now, merges: 0, forced: false };
      return held;
    },

    merge(source, speechEnd) {
      if (!held) return null;
      held.source = `${held.source} ${source}`;
      held.speechEnd = speechEnd;
      held.merges += 1;
      return held;
    },

    msLeft(now) {
      return held ? Math.max(0, held.heldAt + limitMs - now) : 0;
    },

    expired(now) {
      return Boolean(held && !held.forced && now - held.heldAt >= limitMs);
    },

    force() {
      if (!held) return null;
      held.forced = true;
      return held;
    },

    withInstruction({ system, user }) {
      return { system: instruction ? `${system}\n${instruction}` : system, user };
    },

    reset() {
      held = null;
    },
  };
}
