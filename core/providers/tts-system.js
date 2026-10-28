// System TTS: the OS or browser speaks, so the Server Host sends only text.
// The Session recognises `name: 'system'` and emits only
// `segment {speak:true}` (no tts_* frames); the client's speechSynthesis path
// (web/audio-out.js speak()) is the whole playback side.

export function create() {
  return {
    name: 'system',
    async synthesize() {
      return { format: 'text', data: null };
    },
  };
}
