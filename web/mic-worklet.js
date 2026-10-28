// Batches microphone input into 20 ms PCM16 frames at the context sample rate
// (16 kHz is requested by main.js, so no resampling is needed).
// shortcut: assumes the host honours the 16 kHz AudioContext; add worklet
// resampling when real ASR lands and browsers ignore sampleRate.

const FRAME_SAMPLES = 320;

class MicFrames extends AudioWorkletProcessor {
  constructor() {
    super();
    this.buffer = new Int16Array(FRAME_SAMPLES);
    this.length = 0;
  }

  process(inputs) {
    const channel = inputs[0]?.[0];
    if (!channel) return true;
    for (let i = 0; i < channel.length; i++) {
      const clamped = Math.max(-1, Math.min(1, channel[i]));
      this.buffer[this.length++] = Math.round(clamped * 32767);
      if (this.length === FRAME_SAMPLES) {
        this.port.postMessage(this.buffer.buffer, [this.buffer.buffer]);
        this.buffer = new Int16Array(FRAME_SAMPLES);
        this.length = 0;
      }
    }
    return true;
  }
}

registerProcessor('mic-frames', MicFrames);
