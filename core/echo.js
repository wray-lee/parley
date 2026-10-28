// Echo-filter seam: drop ASR hypotheses that match our own played speech.
// The Session owns each spoken Segment's playback lifetime; this module only
// matches normalised text and forgets an entry one tail after it ends.

import { GATING_TAIL_MS } from './gating.js';

const THRESHOLD = 0.8;

function norm(text) {
  return String(text ?? '')
    .normalize('NFKC')
    .toLowerCase()
    .replace(/[^\p{L}\p{N}]+/gu, '');
}

function editDistance(a, b) {
  let prev = Array.from({ length: b.length + 1 }, (_, i) => i);
  for (let i = 1; i <= a.length; i++) {
    const row = [i];
    for (let j = 1; j <= b.length; j++) {
      row[j] = Math.min(prev[j] + 1, row[j - 1] + 1, prev[j - 1] + (a[i - 1] === b[j - 1] ? 0 : 1));
    }
    prev = row;
  }
  return prev[b.length];
}

// Best-matching-substring ratio: the highest 1 - distance / longer length over
// windows of roughly the hypothesis's length.
function bestRatio(hypothesis, speech) {
  if (!hypothesis || !speech) return 0;
  if (speech.includes(hypothesis)) return 1;
  const shortest = Math.max(1, hypothesis.length - 2);
  const longest = Math.min(speech.length, hypothesis.length + 2);
  let best = 0;
  for (let length = Math.min(shortest, speech.length); length <= longest; length++) {
    for (let i = 0; i + length <= speech.length; i++) {
      const ratio =
        1 - editDistance(hypothesis, speech.slice(i, i + length)) / Math.max(hypothesis.length, length);
      if (ratio > best) best = ratio;
    }
  }
  return best;
}

export function createEchoFilter({ threshold = THRESHOLD, tailMs = GATING_TAIL_MS } = {}) {
  const entries = new Map(); // seg_id -> { text, until }

  return {
    remember(segId, text) {
      entries.set(segId, { text: norm(text), until: Infinity });
    },
    end(segId, now) {
      const entry = entries.get(segId);
      if (entry) entry.until = now + tailMs;
    },
    isEcho(text, now) {
      const hypothesis = norm(text);
      if (!hypothesis) return false;
      for (const [segId, entry] of entries) {
        if (entry.until <= now) {
          entries.delete(segId);
          continue;
        }
        if (bestRatio(hypothesis, entry.text) > threshold) return true;
      }
      return false;
    },
    reset() {
      entries.clear();
    },
  };
}
