// Cut policy: closes a Segment on energy-VAD silence, accumulated confirmed
// speech, or the Direction's clause length. Session feeds it audio, Stable
// Prefix text and the clock; PTT suspension is the Session's job.
// shortcut: energy VAD only; upgrade to Silero via onnxruntime if false Cuts become frequent.

const SILENCE_MS = 350;
const ACCUMULATION_MS = 2500;
const SPEECH_RMS = 0.01;
const SAMPLES_PER_MS = 16; // PCM16 16 kHz, fixed by the protocol

export function createCutPolicy({
  silenceMs = SILENCE_MS,
  accumulationMs = ACCUMULATION_MS,
  clauseLength,
  unit,
} = {}) {
  let silence = 0;
  let heardSpeech = false;
  let stableSince = null;

  function reset() {
    silence = 0;
    heardSpeech = false;
    stableSince = null;
  }

  function clauseReached(text) {
    if (!(clauseLength > 0) || (unit !== 'char' && unit !== 'word')) return false;
    const length = unit === 'word'
      ? (text.trim().match(/\S+/g) ?? []).length
      : [...text].length;
    return length >= clauseLength;
  }

  return {
    feed(pcm) {
      const ms = pcm.length / SAMPLES_PER_MS;
      if (rms(pcm) > SPEECH_RMS) {
        silence = 0;
        heardSpeech = true;
        return { speech: true, cut: false };
      }
      if (!heardSpeech) return { speech: false, cut: false };

      silence += ms;
      if (silence >= silenceMs) {
        reset();
        return { speech: false, cut: true };
      }
      return { speech: false, cut: false };
    },

    // `text` is the whole pending Stable Prefix. Starts the accumulation clock
    // on the first confirmed text after the last Cut.
    stable(text, now) {
      if (stableSince === null) stableSince = now;
      return { cut: clauseReached(text) };
    },

    accumulated(now) {
      return stableSince !== null && now - stableSince >= accumulationMs;
    },

    reset,
  };
}

function rms(pcm) {
  let sum = 0;
  for (let i = 0; i < pcm.length; i++) {
    const sample = pcm[i] / 32768;
    sum += sample * sample;
  }
  return Math.sqrt(sum / Math.max(1, pcm.length));
}
