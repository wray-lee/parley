# Intentional divergences and the comparison whitelist

The JavaScript implementation at v0.0.3 is the oracle. A Session Trace replay,
a provider fixture comparison or a Conformance Suite case may ignore **only**
the differences listed here. Anything else is a failure.

Each entry names the trace or comparison that proves it. A trace records the
JavaScript side; the Go side is compared against it when the matching Go ticket
lands.

| ID | Delta | JavaScript oracle (v0.0.3) | v0.1 Go core | Proof |
|---|---|---|---|---|
| `gating-scope` | Gating is core-owned and mode-scoped | `core/session.js` withholds mic frames whenever an audio TTS playback is pending or inside its 500 ms tail, in **both** Listen and Converse; playback is released by the client's `playback end` or a `duration + 1.5 s` estimate. System TTS never gates (`gatingBlocked()` returns false when `tts.name === 'system'`). The Device Host also runs `app/speech-gate.js`. | The Go core owns Gating for **Converse only**, for **every** TTS kind including system, with the 500 ms tail and a watchdog at `estimateSpeechMs(text) + 1.5 s` when no playback end arrives. `app/speech-gate.js` is deleted. | `spec/traces/listen-live-gating.jsonl` proves the JS Listen gate: the second speech burst never reaches ASR. `spec/traces/converse-turns.jsonl` proves the JS Converse gate and its tail. Ticket 09 adds a system-TTS Converse trace for the other half. Only the gating rows (which `audio` inputs reach ASR, and the resulting timing of later events) may differ. |
| `listen-never-gates` | Listen never gates | Gated in Listen when TTS produces audio (see above). | Listen never withholds mic frames; it relies on earphones plus the echo filter. | Same `listen-live-gating.jsonl`: a Go replay must produce the second Segment, while the JS trace stops at one. `spec/traces/listen-basic.jsonl` shows the sample-input Listen path where all 38 `audio` inputs have matching `asr` responses. |
| `hello-protocol` | Additive `hello.protocol` | Field absent. | `1`. | Conformance `hello` case in `spec/conformance/cases.mjs`: the field may be absent or exactly `1`. |
| `hello-tts` | Additive `hello.tts` | Field absent. | `"system"` or `"audio"`. | Same Conformance `hello` case: absent, or one of the two values. |
| `hello-auto-turn` | Additive `hello.autoTurn` and client `autoTurn` | Fields absent; the JS oracle stays Manual. | `hello.autoTurn: {available, enabled}` when the ASR adapter declares pair-capable streaming language tags; the client sends `autoTurn {enabled}`. | Same Conformance `hello` case: the field may be absent, or an object with boolean `available` and `enabled`; ticket 24 adds the Auto Turn traces that prove the client message and the Turn decisions. |
| `asr-provider-errors` | ASR Provider failures reach the client | The ASR adapters return `{type: 'error', provider: 'asr', kind, message, fallback}` from `push()`, but `handleAsrEvents` only reads `partial` and `stable`, so the error is dropped and the client never sees it. | A `Push` error is emitted as a protocol `error` event with the same `provider`, `kind`, `message` and `fallback`; events from the same push are still processed and the Session keeps running. | Adapter fixtures (`spec/fixtures/{deepgram,paraformer,whisperlivekit}` auth/quota/region/in-band errors) assert the error fields at the adapter seam; `internal/core/session_asr_error_test.go` asserts one push delivers its stable event and the error, then a later Cut still commits. |

## Rules for adding an entry

- A divergence is a deliberate product decision, not a shortcut for an
  unimplemented feature. An unported feature is a failing comparison.
- Name the exact trace or conformance case, and the field/row the diff ignores.
- Removing an entry requires re-recording the affected traces and updating the
  Go comparison.
