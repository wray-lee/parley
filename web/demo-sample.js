// Streams a bundled sample as real-time PCM16 16 kHz chunks. The sample file
// describes its energy spans; the mock ASR ignores the waveform and replays
// its timed transcript, while the core VAD sees the silence gaps.
// shortcut: placeholder audio is synthesised from energy spans; bundle real
// sample WAVs when ticket 15 lands.

export function streamSample(sample, send, { chunkMs = 100, sampleRate = 16000 } = {}) {
  let offset = 0;
  const timer = setInterval(() => {
    if (offset >= sample.duration_ms) {
      clearInterval(timer);
      return;
    }
    send(pcmChunk(sample, offset, chunkMs, sampleRate).buffer);
    offset += chunkMs;
  }, chunkMs);
  return () => clearInterval(timer);
}

function pcmChunk(sample, offsetMs, chunkMs, sampleRate) {
  const count = Math.round((sampleRate * chunkMs) / 1000);
  const pcm = new Int16Array(count);
  for (let i = 0; i < count; i++) {
    const timeMs = offsetMs + (i * 1000) / sampleRate;
    const amplitude = amplitudeAt(sample.audio ?? [], timeMs);
    if (amplitude) {
      pcm[i] = Math.round(Math.sin((2 * Math.PI * 220 * timeMs) / 1000) * amplitude * 32767);
    }
  }
  return pcm;
}

function amplitudeAt(spans, timeMs) {
  for (const span of spans) {
    if (timeMs >= span.start_ms && timeMs < span.end_ms) return span.amplitude;
  }
  return 0;
}
