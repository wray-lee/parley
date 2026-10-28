// CosyVoice is not part of v1. Fail at load time so the Server Host reports
// one clear error instead of an error for every Segment.

export function create() {
  throw new Error(
    'CosyVoice TTS is not implemented in v1. ' +
      'Set TTS_PROVIDER=kokoro, edge, azure, system or mock.',
  );
}
