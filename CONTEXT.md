# Parley

Self-hosted simultaneous interpretation on a phone and in a browser. Speech is cut into segments, translated with rolling context, and committed segment by segment; committed output is never revised.

## Language

**Direction**:
One translation path from a source language to a target language (src→tgt), defined by exactly one system prompt file.
_Avoid_: pair (when a single src→tgt path is meant)

**Language Pair**:
The user-facing pairing of "my language" and "their language". Listen uses the their→mine Direction; Converse uses both Directions.
_Avoid_: direction, locale

**Mode**:
How a Session uses a Language Pair — **Listen** (one Direction, user only listens) or **Converse** (both Directions, user speaks and listens).

**Turn**:
In Converse, who is currently speaking — **me** or **them**. Turns are chosen explicitly by the user and select the Direction that receives audio.

**Gating**:
Withholding microphone audio from ASR while the app's own speech is playing, plus a short tail. It applies in Converse only; Listen never gates.

**Face-to-Face split**:
The Converse layout for two people sharing one phone: two halves, each in its speaker's language, with the far half rotated 180 degrees and each half carrying its own Turn control.

**Session**:
One start-to-stop run of a Mode on a Language Pair, with its own Rolling Context and selected Domain Packs.

**Partial**:
An unstable ASR hypothesis, shown greyed and never translated.

**Stable Prefix**:
The part of the ASR hypothesis confirmed unchanged across consecutive hypotheses; the only text that may become a Segment.

**Segment**:
A span of stable source text cut for translation. A Segment is either **Held** or **Committed**.
_Avoid_: chunk, utterance, sentence

**Cut**:
The act of closing a Segment (by silence, accumulated duration, clause length, or push-to-talk release).

**Hold**:
The translator's ⏸ answer meaning "this Segment cannot be translated until more arrives"; a Held Segment is merged with the next one and retried.

**Forced Translation**:
The retry of a Held Segment after the hold limit expires, in which the translator may no longer Hold. If it still fails, only the source text is committed.

**Commit**:
The moment a Segment receives its translation and a seg_id; from then on it is spoken and never revised.

**Rolling Context**:
The most recent Committed source/translation pairs of the Session, given to the translator for consistency of reference and terminology.

**Language Pack**:
The files that define a Direction: its system prompt and optional base Glossary. Adding a language is adding a Language Pack — no code.

**Domain Pack**:
An optional, per-Session, user-selected Glossary layered over the base Glossary for a domain (e.g. conference). Never enabled by default.

**Glossary**:
Source-term → target-term entries injected into the system prompt; Domain Pack entries override base entries for the same source term.

**Echo Suspect**:
An ASR hypothesis judged to be the app's own playback picked up by the microphone; it is discarded, never committed.

**Server Host**:
A deployment where the core runs on a server the user operates; browsers and the app (as a thin client) connect to it.

**Device Host**:
The core running inside the installed app on the phone, calling providers directly with keys held on the device. The app reaches it through a Loopback Host.

**Loopback Host**:
The Device Host's transport: a WebSocket on the loopback interface with a per-launch credential, so the app speaks the same wire protocol it uses to a Server Host.

**Lite client**:
The vanilla JavaScript web client kept at `/lite` as the fallback until the Flutter web client passes its release gates.

**Provider**:
A replaceable ASR, translator, or TTS service.

**Preset**:
A named, ready-made Provider configuration (endpoint and recommended model), so that a user supplies only a key.
_Avoid_: platform, vendor

**Access Token**:
An optional shared passphrase guarding a Server Host. It is not an account.

**Demo Mode**:
Any Session where at least one of ASR, translator, or TTS is a mock; always visibly labelled DEMO.

**Session Trace**:
A recorded Session — its inputs (client messages, microphone frames, clock advances, Provider responses) and its outputs (host events, Provider requests) — replayed on a fake clock to compare implementations.

**Conformance Suite**:
A black-box test suite that runs the wire protocol against any host and asserts the same observable behaviour; it is the parity gate between hosts.

## Relationships

- A **Language Pair** has two **Directions**; **Listen** needs one, **Converse** needs both
- A **Direction** is defined by one **Language Pack** and may have any number of **Domain Packs**
- A **Session** produces **Segments**; each **Segment** is **Held** zero or more times, then **Committed** exactly once
- Only **Committed** Segments enter the **Rolling Context**
- A **Device Host** is reached through exactly one **Loopback Host**
- A **Session Trace** replays one **Session**; a **Conformance Suite** exercises a host over the wire

## Example dialogue

> **Dev:** "The speaker paused mid-sentence before the verb — do we send what we have?"
> **Domain expert:** "Yes, it's a **Cut**. If the translator answers with a **Hold**, the **Segment** waits and merges with the next one. After 2.5 s we do a **Forced Translation**, and whatever comes back gets **Committed**."

## Flagged ambiguities

- The spec used "语言对" for both a single src→tgt path and the mine/theirs pairing — resolved: **Direction** vs **Language Pair**.
- The spec's acceptance test T4 named 「决算」, which is not in the conference **Domain Pack** — resolved: typo for 「预算」; pack content is unchanged.
