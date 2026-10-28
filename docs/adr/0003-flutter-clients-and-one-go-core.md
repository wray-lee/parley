# Flutter clients and one Go core, in two hosts

Status: Accepted. Supersedes ADR-0001; amends ADR-0002.

## Context

v0.0.2 ships a JavaScript orchestration core in two hosts: a Bun server and a Tauri WebView shell. Two limits block the next release. The WebView cannot give the app the audio behaviour it needs — platform audio-session ownership, AEC and Bluetooth routing on phones, and reliable TTS completion events. And the product's main surface is its least capable one: a vanilla web client tuned for desktop browsers. Keeping the core in JavaScript while adding a native app would mean two cores and guaranteed divergence.

The v0.1 research (design systems, interpretation UX, Flutter audio, Go core/mobile) concluded that the core should move to Go and the client to Flutter, with the JavaScript implementation kept as the parity oracle until the port is proven.

## Decision

- **One core, in Go.** Pure Go with cgo disabled; standard library plus `github.com/coder/websocket`; no `os/exec` and no vendor SDKs. One goroutine per Session runs as an event loop with an injected clock and scheduler, so a recorded Session Trace replays deterministically. Adapters are ported 1:1 from the JavaScript providers.
- **Server Host** is that core as a static binary in a distroless image. It is a drop-in for the Bun image: same environment variables, port 3000, `/ws?token=`, `/healthz` and `/samples`. It serves the Flutter web build at `/` and the vanilla client at `/lite`.
- **Device Host** is the same host code compiled with `gomobile bind` into an Android AAR and an iOS XCFramework. It exports only `Start(configJSON) -> {url, token}` and `Stop()`, and serves the existing wire protocol on a **Loopback Host**: loopback interface only, random port, a 128-bit per-launch token, and every request carrying an `Origin` header rejected. It lives only while the app is in the foreground in v0.1. If the loopback spike fails, the fallback is a MethodChannel/EventChannel facade carrying the same messages behind the same Dart transport seam.
- **Flutter everywhere.** One client for Android, iOS and web. Material 3 through `material_ui` 1.8.x with one custom "Caption" theme: ink and paper, one tally-red live signal, dark-first. No adaptive Cupertino, no Liquid Glass; M3 Expressive only for the mic-pad shape morph.
- **Converse is a face-to-face split.** On a phone in portrait, the screen splits into two halves, each in its speaker's language, with the far half rotated 180 degrees. Tap takes the Turn; hold is push-to-talk. The stacked layout replaces the split above 1.5x text scale, in landscape, on web and desktop, and with a screen reader.
- **Gating moves into the core** (amends ADR-0002). The client reports playback start and end for every spoken Segment, system TTS included. The core gates Converse for every TTS kind: 500 ms tail, and a watchdog of `estimateSpeechMs(text) + 1.5 s` when no end arrives. **Listen never gates**; its voice defaults to earphones only, and the echo filter stays.
- **Flutter Web is a client of the Server Host only.** No Go WASM in the browser. The web build is served same-origin by the Go server, and it replaces the vanilla client only after its release gates pass; until then the vanilla client stays at `/lite`.
- **Protocol and environment stay byte-compatible.** The only additions are `hello.protocol: 1` and `hello.tts: "system" | "audio"`. Every intentional delta, including these two fields, is recorded in `spec/DIVERGENCES.md` with the trace or comparison that proves it.
- **The JavaScript implementation is the oracle until it is deleted.** After v0.0.2 it takes bug fixes only, each with a trace. Parity is proven by Session Traces replayed against both implementations, Go fixture tests over the recorded provider fixtures, and a black-box Conformance Suite run against the Bun server, the Go server and the Device Host.
- **One repo.** Root `go.mod` with the root package embedding `config/ packs/ prompts/ samples/`; `internal/core`, `internal/providers`, `internal/protocol`, `internal/host`, `cmd/parley-server`, `mobile/`, `client/` (Flutter) and `spec/` (fixtures, traces, conformance, divergences).

## Consequences

- The deployed Bun server host is removed when the Go server reaches parity; the JavaScript core and its tests stay as the oracle until the final gate; the Tauri shell is deleted with the v0.1.0 app. Only the Go core and the Flutter client remain, plus the vanilla `/lite` client if the Flutter web gates fail.
- v0.1.0 is an in-place upgrade: same application id and signing key, versionCode 1000, arm64. Keys and settings are re-entered once because Tauri storage is not migrated.
- The Android app must declare and request `ACCESS_LOCAL_NETWORK` (targetSdk 37) when a LAN URL is saved; the current APK probably cannot reach LAN providers on Android 17.
- Go becomes a build requirement for CI and for anyone building the app. gomobile is pinned; the toolchain is experimental and needs Go 1.26 or newer and NDK r28 or newer.
- Flutter Web has real limits: no background capture, iOS Safari reroutes output when the mic opens, CJK fallback glyphs are fetched from a configurable base URL, and semantics are opt-in. Background and locked-screen listening are deferred to v0.2; v0.1 is foreground-only.
- Exact package pins (`material_ui`, `record`, `flutter_soloud`, `flutter_tts`, `audio_session`, …) are part of the architecture; they are re-tested on devices before any bump.

## Alternatives

- **Keep the JavaScript core and vanilla web UI, fix audio in the Tauri shell.** Rejected: the same WebView audio limits remain, and the main surface keeps the weakest experience.
- **Go compiled to WASM in the browser.** Rejected: about 2.82 MB gzip on top of Flutter's payload, browsers cannot set provider authentication headers on WebSockets, CORS blocks many LLM endpoints, and keys would live in the browser.
- **`c-shared`/`c-archive` with `dart:ffi` and ffigen.** Kept as the escape hatch: a direct C ABI and no platform channel, but we would own the C ABI, iOS symbol stripping and XCFramework packaging. The exported facade stays at two functions so a switch is one file plus build scripts.
- **MethodChannel/EventChannel streaming facade for the Device Host.** Kept as the fallback if the loopback spike fails; it adds a second transport and main-thread hops at 50 frames per second.
- **Flutter Web plus a keyless static demo instead of a self-hosted web client.** Deferred to v0.2 as a recorded event-log replay; it is not a substitute for the server-hosted client.
