// Per-Segment JSON log record. The host supplies the sink (file, stdout, test).

export function createSegmentLog(write = () => {}) {
  return function record(segment) {
    write({
      seg_id: segment.seg_id,
      direction: segment.direction,
      speech_end: segment.speech_end,
      asr_final: segment.asr_final,
      first_token: segment.first_token,
      tts_first_chunk: segment.tts_first_chunk,
      held: segment.held ?? false,
      merges: segment.merges ?? 0,
      forced: segment.forced ?? false,
      forced_fail: segment.forced_fail ?? false,
      echo_suspect: segment.echo_suspect ?? 0,
    });
  };
}
