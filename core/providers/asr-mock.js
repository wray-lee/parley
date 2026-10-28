// Replays a bundled sample's timed transcript. Live microphone audio is refused
// by the Session (acceptsLive = false) so the demo never pretends to listen.

export function create({ sample = {} } = {}) {
  const transcript = sample.timed_transcript ?? [];
  let fired = [];

  return {
    name: 'mock',
    acceptsLive: false,
    open() {
      fired = transcript.map(() => ({ partial: false, stable: false }));
    },
    close() {},
    push({ audioMs }) {
      const events = [];
      transcript.forEach((entry, index) => {
        const state = fired[index];
        if (!state) return;
        if (!state.partial && audioMs >= entry.start_ms) {
          state.partial = true;
          events.push({ type: 'partial', text: entry.text });
        }
        if (!state.stable && audioMs >= entry.end_ms) {
          state.stable = true;
          events.push({ type: 'stable', text: entry.text });
        }
      });
      return events;
    },
  };
}
