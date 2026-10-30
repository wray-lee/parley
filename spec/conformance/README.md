# Conformance Suite (Seam 3)

Black-box checks that speak only the WebSocket protocol. The normative suite is
`conformance_test.go` (Go); the same cases run against the Bun Server Host, the
Go Server Host, and the Device Host's loopback URL (`device_host_test.go` starts
it in-process through `mobile.Start()`). `cases.mjs` is the frozen v0.0.3
oracle's copy of the first cases and stays until P4, run by
`tests/conformance.test.js` under `bun test`.

## Run against a running host

```sh
go test ./spec/conformance -url ws://127.0.0.1:3000/ws
go test ./spec/conformance -url "ws://127.0.0.1:3000/ws?token=$ACCESS_TOKEN"
go test ./spec/conformance -url ws://127.0.0.1:<port>/ws?token=<token> -origin reject
```

The Go test exits non-zero on any failure and prints `ok - <case>`,
`skip - <case> (<reason>)` or `FAIL - <case>: <detail>`.

For a Device Host running in an Android app, forward its loopback port and use
the URL and token from the debug `PARLEY host` log line (release builds do not
log it):

```sh
adb forward tcp:<port> tcp:<port>
go test ./spec/conformance -url "ws://127.0.0.1:<port>/ws?token=<token>" -origin reject
```

Without `-url` it starts the Go host in process and, when Bun and `node_modules`
are available, boots the Bun host too and requires the same result for every
case. `go test ./...` runs those in-process checks.

`-origin reject` is the Device Host's Loopback Host policy (ADR-0003): any
request with an `Origin` header is refused. The default, `allow`, is the Server
Host contract: it accepts every Origin like the Bun oracle (tunnels and CDNs may
rewrite `Host`, so a same-origin check would break deploys) and `ACCESS_TOKEN`
is the guard. The `allow` case asserts that the web client's own Origin is
accepted.

The host must be in Demo Mode (mock Providers) for the demo flow cases: the
bundled sample drives the mock ASR. A keyed server is the same fixture plus
`?token=`; `token rejection` is skipped when the URL carries no token.

## Cases

- `hello` — `demo: true`, the converse-capable ja/zh pair and packs; tolerates
  the additive `hello.protocol` / `hello.tts` fields per `spec/DIVERGENCES.md`.
- `start listen` — a listen Session reaches `state: listening`.
- `demo segment` — the bundled sample commits one DEMO-labelled Segment in the
  requested Direction with `speak: true`.
- `segment ordering` — replaying the sample on the same Session keeps `seg_id`
  increasing and each Segment well formed.
- `commit immutability` — a committed `seg_id` appears exactly once.
- `tts ordering` — `tts_begin`, at least one binary audio chunk, `tts_end`.
- `unknown direction error` — an unsupported direction yields a `protocol`
  error instead of a Session.
- `invalid JSON error` — a non-JSON text frame yields a `protocol` error.
- `converse gating` — after the TTS ends, a live microphone frame is withheld
  from ASR while the playback is pending and for the 500 ms tail; after
  `playback end` the same frame reaches ASR. The Demo ASR refuses live audio,
  so its error event is the sentinel.
- `listen gating delta` — encodes the `gating-scope` row of
  `spec/DIVERGENCES.md`: the v0.0.3 oracle withholds Listen frames too, the
  v0.1 core never gates Listen. Both pass; `hello.protocol` selects which
  expectation applies.
- `hold merge` / `hold forced` — the ja-held sample's first line translates to
  `⏸`, so the Segment is Held, then merged with the next line or force-
  translated after 2.5 s. Demo Mode cannot produce the mark (its mock
  translator prefixes `[DEMO]`), so the case skips on a Demo host unless a
  translator that answers `⏸` is configured; `TestConformanceHoldWithFakeTranslator`
  runs both against a fake OpenAI-compatible endpoint. The traces
  `spec/traces/listen-hold-*` remain the Demo-mode proof.
- `token rejection` — a missing or wrong `?token=` handshake gets HTTP 401.
  Skipped when `-url` has no token.
- `origin policy` — the host's Origin policy: 403 for every Origin-bearing
  handshake with `-origin reject`; HTTP 101 for the web client's own Origin
  with the default `allow`.
