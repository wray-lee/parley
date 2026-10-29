# Conformance Suite (Seam 3)

Black-box checks that speak only the WebSocket protocol. They run unchanged
against the Bun Server Host today, the v0.1 Go Server Host in CI, and the
Device Host loopback later. The suite is the seed of the Go test that will take
`-url`; `cases.mjs` is the implementation until the Go host exists.

## Run against a Demo Mode host

```bash
bun hosts/node/server.js            # terminal 1; keyless = Demo Mode
bun spec/conformance/run.mjs --url ws://127.0.0.1:8787/ws
```

`tests/conformance.test.js` boots the Bun server itself on a free port, so the
suite also runs green under `bun test`.

The host must be in Demo Mode (mock Providers): the demo flow cases rely on the
bundled sample. A keyed server is a different fixture, not a conformance
failure.

## Cases

- `hello` — `demo: true`, the converse-capable zh/ja pair and packs; tolerates
  the additive `hello.protocol` / `hello.tts` fields per `spec/DIVERGENCES.md`.
- `start listen` — a listen Session reaches `state: listening`.
- `demo segment` — the bundled sample commits one DEMO-labelled Segment in the
  requested Direction with `speak: true`.
- `tts ordering` — `tts_begin`, at least one binary audio chunk, `tts_end`.
- `commit immutability` — a committed `seg_id` appears exactly once.
- `unknown direction error` — an unsupported direction yields a `protocol`
  error instead of a Session.

Parked until the Go host exists: token rejection, Origin rejection, Hold and
Forced Translation, Gating, and Device Host loopback. Add them as cases here
first, then port the file; the suite is the comparison that proves
`spec/DIVERGENCES.md`.
