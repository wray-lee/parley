// A short WAV tone per Segment, generated without dependencies.

export function create({ durationMs = 120, frequency = 440, sampleRate = 16000 } = {}) {
  return {
    name: 'mock',
    async synthesize() {
      return { format: 'wav', data: toneWav(durationMs, frequency, sampleRate) };
    },
  };
}

function toneWav(durationMs, frequency, sampleRate) {
  const samples = Math.round((sampleRate * durationMs) / 1000);
  const dataBytes = samples * 2;
  const buffer = new ArrayBuffer(44 + dataBytes);
  const view = new DataView(buffer);

  ascii(view, 0, 'RIFF');
  view.setUint32(4, 36 + dataBytes, true);
  ascii(view, 8, 'WAVE');
  ascii(view, 12, 'fmt ');
  view.setUint32(16, 16, true);
  view.setUint16(20, 1, true);
  view.setUint16(22, 1, true);
  view.setUint32(24, sampleRate, true);
  view.setUint32(28, sampleRate * 2, true);
  view.setUint16(32, 2, true);
  view.setUint16(34, 16, true);
  ascii(view, 36, 'data');
  view.setUint32(40, dataBytes, true);

  for (let i = 0; i < samples; i++) {
    const value = Math.sin((2 * Math.PI * frequency * i) / sampleRate) * 0.25;
    view.setInt16(44 + i * 2, Math.round(value * 32767), true);
  }
  return new Uint8Array(buffer);
}

function ascii(view, offset, text) {
  for (let i = 0; i < text.length; i++) view.setUint8(offset + i, text.charCodeAt(i));
}
