# Session Traces (Seam 1)

A Session Trace is the executable oracle for the Session core. It records every
input a Session receives and every observable output it produces on a fake
clock, so the same run can be replayed against the JavaScript core (now) and the
Go core (during the port). The JavaScript implementation is frozen at v0.0.3:
behaviour changes are bug fixes only, and each fix re-records the traces it
touches.

## Files

One JSONL file per scenario, `spec/traces/<name>.jsonl`, one JSON object per
line. Line order is significant. Three record types exist:

| `type` | Shape |
|---|---|
| `meta` | `{type, format, name, oracle?, clockStart, registry, pairConfig, forcedInstruction, providers}` — exactly one, first line |
| `input` | `{type, kind, ...}` — everything the core receives |
| `output` | `{type, kind, ...}` — everything the core emits |

`format` is `2`. A change to this grammar bumps the number and re-records every
trace. Format 2 adds the `provider_error` input kind; format-1 traces have no
Provider errors and stay replayable. Binary data (TTS audio) is encoded as
`{"$b64": "<base64>"}`.

`meta.registry` is the resolved registry (`directions`, `packs`, `pairs`), not
file names, so a replay needs no filesystem. `meta.providers` carries only the
names and flags a replay needs to reconstruct the fake Providers. An Auto
Turn-capable ASR also carries `meta.providers.asr.languageCapability`
(`streamingLanguageId`, `languageGranularity`, `languageConfidence`,
`languages`, `validatedPairs`); without it the replay has no Auto Turn.

`meta.oracle` is `"js"` when absent and `"go"` for traces that record behaviour
v0.1 deliberately adds and the frozen JavaScript oracle cannot produce (see
`spec/DIVERGENCES.md`). A `go` trace is recorded from the Go core and replayed
against it; it is a regression golden, not a parity oracle.

## Inputs

| `kind` | Fields | Meaning |
|---|---|---|
| `client` | `message` | one client protocol message passed to `handleMessage` |
| `audio` | `ms`, `amplitude` | one 16 kHz PCM16 microphone frame; the replay regenerates it |
| `advance` | `ms` | move the fake clock forward, firing due timers in order |
| `idle` | — | await the Session queue (`session.idle()`), i.e. flush pending commit work |
| `asr` | `events` | the scripted ASR response to the preceding `push`; a Stable event may carry per-word `tags: [{language, confidence}]` |
| `translator` | `call`, `chunk` | one chunk yielded by translator call `call` |
| `tts` | `response` | the scripted TTS response (`format`, `data`) |
| `provider_error` | `provider`, `error`, `call` (translator only) | the named Provider throws this error on its next call; a translator error follows that call's chunks |

`idle` is a real input, not a convenience: it fixes the points at which async
commit work is allowed to settle. A replay that flushes at different points can
change `first_token` and event order, so it must use the recorded `idle` lines.

The fake clock starts at `meta.clockStart`. `advance` fires every timer due at or
before the new time in chronological order; timers with equal deadlines fire in
the order they were scheduled. The Go fake clock must use the same rule.

`provider_error.error` carries `message`, and optionally `kind` and `fallback`
so a Provider error event replays exactly.

## Outputs

| `kind` | Fields | Meaning |
|---|---|---|
| `event` | `event` | one host event emitted by the Session |
| `log` | `record` | one Segment log record |
| `request` | `provider`, `request` | one Provider request; translator requests carry the exact `system` and `user` messages |

## Replaying

1. Build a fake clock at `meta.clockStart`, the fake Providers whose scripted
   responses are the `asr` / `translator` / `tts` inputs, and a Session with
   `meta.registry`, `meta.pairConfig` and `meta.forcedInstruction`.
2. Apply the executable inputs (`client`, `audio`, `advance`, `idle`) in file
   order. Provider-response inputs are consumed by the fakes at the matching
   call, not applied directly.
3. Assert the emitted `event`, `log` and `request` outputs equal the recorded
   `output` lines in order. Differences are failures unless the field appears in
   `spec/DIVERGENCES.md`.

The Go runner implements the same rules. It must reproduce the outputs exactly,
including `latency_ms`, `speech_end`, `first_token` and `tts_first_chunk`, which
is why the clock and the flush points are recorded.

## Recording

`bun run traces:record` runs the JavaScript trace tests in record mode and
rewrites the `oracle: js` files. Run it only when a JS bug fix intentionally
changes behaviour; never hand-edit a trace. Review the resulting diff.

`PARLEY_RECORD_TRACES=1 go test ./internal/trace -run TestRecord` does the same
for the `oracle: go` traces; without the variable those tests compare the live
run byte for byte with the committed file.

Current traces:

- `listen-basic.jsonl` — Listen start on the bundled sample, Partial/Stable
  Prefix, two silence Cuts, commit order, TTS, Rolling Context in the second
  translator request.
- `listen-live-gating.jsonl` — Listen with a live mic and audio TTS; records the
  v0.0.3 gate that v0.1 removes (see `spec/DIVERGENCES.md`).
- `converse-turns.jsonl` — Converse start, Turn `me` speech, Turn switch to
  `them`, subtitle-only Segment, Gating playback end plus tail.
- `converse-auto-switch.jsonl` — my spoken output ends; the Turn switches to
  `them` automatically and their Segment is subtitle-only.
- `converse-estimate.jsonl` — no playback report; the watchdog ends my spoken
  output at audio duration + 1.5 s, the Turn switches, and the 500 ms tail
  follows.
- `converse-system-gating.jsonl` (`oracle: go`) — system TTS is Gated in
  Converse: frames are withheld during playback and the 500 ms tail, the
  watchdog releases at `estimateSpeechMs(text) + 1.5 s`, and a client `cancel`
  or `error` also ends the Gate.
- `converse-auto-turn-flip.jsonl` (`oracle: go`) — Auto Turn with per-word
  language tags: the first two agreeing `ja` tags decide the Turn at a Segment
  Cut (`turn {auto: true, language: "ja"}`) with 0 ms added latency, the log
  carries `turn_auto` and `detected_language`, and the next Segment flips back
  to `zh`. The Session keeps one multilingual stream across both Turns.
- `converse-auto-turn-keep.jsonl` (`oracle: go`) — tags that are out of pair,
  below the confidence floor, or alone keep the Turn and emit
  `turn {auto: true, uncertain: true}` (the client pulses the pads); an
  explicit pad tap in Auto emits `turn {override: true}` and logs
  `turn_override`.
- `converse-auto-turn-gating.jsonl` (`oracle: go`) — microphone frames during
  my playback and its 500 ms tail never reach ASR; only the released frame's
  tags flip the Turn, at its Cut, and both Segments log `detected_language`.
- `listen-echo.jsonl` — Listen with system TTS: mic frames keep flowing while it
  plays; the app's own words are dropped as an Echo Suspect and the same text
  commits after the tail.
- `listen-never-gates.jsonl` (`oracle: go`) — Listen with audio TTS: every
  microphone frame reaches ASR during playback, an own-voice phrase is dropped
  as an Echo Suspect while a real phrase commits, and a post-tail repeat is no
  longer a suspect.
- `listen-cuts.jsonl` — one Listen Session exercising silence, accumulated
  duration, clause length and PTT-release Cuts.
- `listen-hold-merge.jsonl` — ⏸ holds a Segment; two later Cuts merge into it
  and one Commit is logged with `merges: 2`.
- `listen-hold-force.jsonl` — the 2.5 s Hold limit forces a translation whose
  request carries the prompt-asset instruction.
- `listen-hold-force-fail.jsonl` — the Forced Translation returns ⏸, so the
  source commits alone with `speak: false` and no TTS.
- `listen-rolling-context.jsonl` — ten Commits; the last request carries exactly
  the eight most recent pairs.
- `listen-glossary.jsonl` — a selected Domain Pack's Glossary replaces
  `{{GLOSSARY}}` in the Direction system prompt; the exact translator messages
  reach the trace.
- `tts-failure.jsonl` — the TTS Provider throws; the Segment and its log are
  emitted as usual (subtitles stay), the `tts` protocol error is emitted, and
  no `tts_*` event follows.
- `asr-deepgram-listen.jsonl`, `asr-paraformer-listen.jsonl`,
  `asr-whisperlivekit-listen.jsonl` — Listen on the recorded ASR contract
  fixture for each adapter (`spec/fixtures/<provider>/`), with system TTS so
  there is no playback gating. The `asr` inputs are the frozen JavaScript
  adapters' own output; they are not produced by `bun run traces:record`, and
  the Go adapter contract tests assert the Go adapters emit the same stream.
