// Seam 1 test doubles: fake clock and scripted Providers.

export function makeClock(start = 1000) {
  let t = start;
  let nextId = 0;
  const timers = new Map();
  return {
    now: () => t,
    setTimeout(fn, ms) {
      const id = ++nextId;
      timers.set(id, { at: t + Math.max(0, ms), fn });
      return id;
    },
    clearTimeout(id) {
      timers.delete(id);
    },
    advance(ms) {
      const target = t + ms;
      for (;;) {
        let dueId = null;
        let due = null;
        for (const [id, timer] of timers) {
          if (timer.at <= target && (!due || timer.at < due.at || (timer.at === due.at && id < dueId))) {
            dueId = id;
            due = timer;
          }
        }
        if (!due) break;
        timers.delete(dueId);
        t = due.at;
        due.fn();
      }
      t = target;
    },
  };
}

export function pcmFrame(ms = 20, amplitude = 0.2, sampleRate = 16000) {
  const n = Math.round((sampleRate * ms) / 1000);
  const pcm = new Int16Array(n);
  if (amplitude !== 0) {
    for (let i = 0; i < n; i++) {
      pcm[i] = Math.round(Math.sin((2 * Math.PI * 220 * i) / sampleRate) * amplitude * 32767);
    }
  }
  return pcm;
}

// marks: [{ at: ms, events: [{ type: 'partial'|'stable', text }] }]
export function scriptedAsr(marks = [], { name = 'scripted', acceptsLive = false } = {}) {
  let fired = [];
  let audioMs = 0;
  return {
    name,
    acceptsLive,
    open() {
      fired = marks.map(() => false);
      audioMs = 0;
    },
    close() {},
    push(frame) {
      audioMs = frame.audioMs;
      const out = [];
      marks.forEach((mark, i) => {
        if (!fired[i] && audioMs >= mark.at) {
          fired[i] = true;
          out.push(...mark.events);
        }
      });
      return out;
    },
  };
}

export function scriptedTranslator(texts = [], { name = 'scripted' } = {}) {
  const requests = [];
  return {
    name,
    requests,
    async *translate(request) {
      requests.push(request);
      yield texts.shift() ?? 'translated';
    },
  };
}

export function scriptedTts({ name = 'scripted', data = new Uint8Array([82, 73, 70, 70]) } = {}) {
  const calls = [];
  return {
    name,
    calls,
    async synthesize({ text }) {
      calls.push(text);
      return { format: 'wav', data };
    },
  };
}
