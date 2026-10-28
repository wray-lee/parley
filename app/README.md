# Parley app shell (Tauri v2)

The shell wraps the same vanilla web client the Server Host serves. There is no
native UI and no bundler: `build-frontend.mjs` copies `web/`, `core/`,
`config/`, `prompts/`, `packs/` and `samples/` into `dist/`, adds
`device-host.js` before `main.js`, and writes `device-assets.json` so the
Device Host can load the Language Packs without a filesystem API.

`device-host.js` runs the unmodified Session core in the WebView and exposes it
through the WebSocket surface the web client already uses. With no keys it runs
the mock Providers, so Demo Mode (the DEMO banner, sample playback, `[DEMO]`
translations, per-Segment JSON logs) still works without a server. With keys it
calls the real Providers through the `tauri` platform adapter. The app can also
switch to "connect to my server" and act as a protocol client.

All Tauri API use is confined to three thin adapters:

- `platform-tauri.js` — streaming HTTP through the official Tauri HTTP plugin
  (no WebView CORS) and header-authenticated WebSockets through the official
  Tauri WebSocket plugin (no browser header limit). PCM is batched 150 ms per
  send (100–250 ms window). Fetch has a 35 s hard deadline, and Stop aborts
  every in-flight stream; chunks that arrive after an abort are dropped.
- `keystore.js` — `tauri-plugin-secure-keystore` (0.0.3-beta.1).
- `tts-speaker.js` — `tauri-plugin-tts` (0.2.1), whose `speech:start` /
  `speech:finish` events drive the Converse microphone gate.

## Layout

- `device-host.js` — in-page Device Host; wires Providers, TTS speech and the
  WebSocket shim, or redirects to a Server Host.
- `platform-tauri.js` — Tauri HTTP/WebSocket platform adapter.
- `settings.js` — injected settings screen (Presets, Base URL/model/extra
  JSON, keys, server mode).
- `keystore.js` — secure-keystore adapter with the memory fallback.
- `tts-speaker.js` — system-TTS plugin adapter with the Web Speech fallback.
- `speech-gate.js` — app-side mic gate for system TTS in Converse.
- `server-socket.js` — WebSocket redirect for "connect to my server".
- `vendor/` — vendored plugin ES modules and their licenses (see
  `vendor/README.md`).
- `build-frontend.mjs` — assembles `dist/` from the repo's static files.
- `src-tauri/` — Tauri v2 shell (Rust side has no app logic).
- `src-tauri/gen/android/` — generated Android project, committed; the manifest
  declares `android.permission.RECORD_AUDIO` and
  `android.permission.ACCESS_LOCAL_NETWORK`.
- `src-tauri/Info.ios.plist` — iOS microphone usage string (stub; IPA is
  human-gated in ticket 18).
- `android-builder.Dockerfile` — SDK/NDK/Rust/Bun build image.
- `../scripts/build-apk` — WSL + Docker APK build.

## Settings, keys and fallbacks

The **Settings** button (bottom-right) chooses the translator Preset, the ASR
Provider (Deepgram, Paraformer, mock) and the TTS Provider (system, Azure,
Kokoro, mock), and switches between Device mode and a Server Host URL plus
Access Token. The translator section also takes an optional **Base URL**
override (required for the `custom` Preset, which also needs a **Model**), a
**Model** override for any Preset, and **Extra request fields (JSON)**, which
follows the Server Host's `LLM_EXTRA_BODY` rules. Saving reloads the app.
On Android 17+, saving a LAN URL (Kokoro URL, custom Base URL, or a LAN Server
Host) asks for the Nearby devices permission (`ACCESS_LOCAL_NETWORK`); denial
keeps the dialog open with a specific error. `localhost` needs no grant.

- API keys and the server token are written **only** through the secure
  keystore. Non-secret choices (Presets, provider names, server URL, Base URL,
  model and extra request JSON) are the only values in `localStorage`; secret
  values never touch WebView storage.
- The keystore plugin is 0.0.3-beta.1. If it fails, the adapter falls back to an
  in-memory store for the current session and the settings screen says so.
  Keys are then lost on exit and are still never written to browser storage.
- The TTS plugin may be unavailable (for example on a desktop without an
  engine). The adapter then falls back to Web Speech; if neither works the app
  shows a notice and keeps subtitles only.
- No keys means Demo Mode, exactly like the Server Host: the DEMO banner stays
  visible until the translator and ASR Providers have keys.

## Provider transport details

- Translator, Azure TTS and Kokoro calls go through the Tauri HTTP plugin. The
  capability allows `https://*` and plain HTTP only to loopback and private LAN
  IPv4 ranges (10/8, 127/8, 169.254/16, 172.16/12, 192.168/16), so an Ollama or
  other gateway at e.g. `http://192.168.1.10:11434/v1` works. Android release
  builds keep `usesCleartextTraffic=false`: the plugin's requests run in Rust
  over raw sockets, not through the WebView/Java networking stack, so that
  policy does not apply. The local model stack stays on the Server Host
  (ADR-0001).
- Deepgram uses the Tauri WebSocket plugin with an
  `Authorization: Token …` header. The plugin has no subprotocol support, which
  is why the Device Host uses header auth rather than the browser's `token`
  subprotocol.
- System TTS is text-only in the core. The Device Host speaks each Segment with
  the TTS plugin and, in Converse, drops microphone frames while it speaks plus
  a 500 ms tail. Stop cancels speech, aborts streams and resets the gate.
- The server Access Token is stored in the keystore and appended as `?token=`
  when connecting. Server-side enforcement is still pending in this build.

## Security notes

- Release builds set `devtools: false` and keep the strict CSP
  (`script-src 'self'`, no `unsafe-eval`) from ticket 16.
- The HTTP capability is deliberately broad over HTTPS because the user picks
  the Preset; it cannot be narrowed to Groq without breaking the others.
- `scripts/build-apk` never copies `.env`, `tmp/` or `.git` into the build.

## Pinned versions (spec Further Notes, verified 2026-10-28)

| Component | Version |
| --- | --- |
| Tauri CLI / `@tauri-apps/api` | 2.12.4 |
| `tauri` crate | 2.12.5 (2.12.x pin) |
| `tauri-build` | 2.7.4 |
| `tauri-plugin-http` / `@tauri-apps/plugin-http` | 2.8.3 |
| `tauri-plugin-websocket` / `@tauri-apps/plugin-websocket` | 2.5.0 |
| `tauri-plugin-tts` | 0.2.1 |
| `tauri-plugin-secure-keystore` | 0.0.3-beta.1 |
| Android SDK | platform android-37.0, build-tools 37.0.0 |
| Android NDK | 30.0.16248370 |
| Bun (build image and runtime) | 1.4.3 |
| JDK (build image) | 17 |

## Self-check

`bun test app/` (also picked up by the root `bun run test`) covers the adapter
seams without a phone: fetch timeout/abort/late-chunk handling, WebSocket header
auth and PCM batching, keystore fallback, TTS event matching and Web Speech
fallback, the Converse gate, settings storage (no secrets), and the server-mode
URL rewrite. `app/device-host.test.mjs` additionally assembles a temporary
frontend, boots the Device Host with the bundled sample, and asserts a
DEMO-labelled Segment plus the Segment log. It writes outside the source tree
via `PARLEY_DIST`.

## Build the APK

From Windows (any shell), at the repo root:

```sh
bun run build:apk
```

Equivalent, from WSL:

```sh
wsl.exe -d arch -- bash /mnt/f/Projects/translation/scripts/build-apk
```

The script copies the repo to `/home/<user>/work/parley` (no secrets, no
`.git`), builds `parley-android-builder:local` on first use, copies the repo to
the container, runs `bun install --frozen-lockfile` and `bunx tauri android
build --apk --debug -t aarch64`, and deletes the work directory afterwards. The
APK and its sha256
land in `tmp/`:

```
tmp/parley-0.0.3-arm64-debug.apk
tmp/parley-0.0.3-arm64-debug.apk.sha256
```

Docker images pulled by the build: `oven/bun:1.4.3` (Bun, copied into the
builder) and `rust:1.98-bookworm` (base, only if it is not already present). The
Android SDK and NDK are downloaded by the Dockerfile from Google; the JDK
comes from Debian packages and Bun from the `oven/bun` image. Remove the local
builder image when it is no longer needed:

```sh
docker rmi parley-android-builder:local
```

## Keystore

`tmp/parley-debug.keystore` (gitignored) is generated on first build with
`keytool`:

- alias `androiddebugkey`, store/key password `android`, RSA 2048, 10000 days;
- mounted into the container as `~/.android/debug.keystore`, so Gradle
  debug-signs every APK with it;
- never committed and never leaves `tmp/`.

Release signing (repository secrets) is ticket 18. If the file is deleted, the
next build generates a new key; Android will refuse to update an installed app
signed with the old one until it is uninstalled.

## Phone install (PENDING-USER)

No Android device is attached to the build host, so none of these steps has
been run. They are the acceptance procedure for tickets 16 and 17.

### Demo Mode (ticket 16)

1. Transfer `tmp/parley-0.0.3-arm64-debug.apk` to the Android phone (USB,
   Tailscale, or `adb install -r F:\Projects\translation\tmp\parley-0.0.3-arm64-debug.apk`).
2. Allow install from unknown sources for the file manager, then install.
3. Open **Parley**. The header shows `ready` and the DEMO banner is visible
   because no keys are stored yet.
4. Press **Start** with pair `ja ↔ zh`. The bundled sample plays; within a few
   seconds a Segment appears with source above and `[DEMO]` translation below.
5. Optional microphone declaration check: press **Mic**. Android shows the
   microphone permission prompt; in Demo Mode, mock ASR then reports that only
   bundled samples are accepted (by design).

### Real Providers: Groq + Deepgram + system TTS (ticket 17)

1. Install the same APK, open **Settings** (bottom-right).
2. Keep Mode **This device**. Choose the **groq** Translator Preset and paste a
   free-tier Groq key from <https://console.groq.com/keys>. Choose **Deepgram**
   ASR and paste a key from <https://console.deepgram.com>. Keep TTS **System**.
   Press **Save**; the app reloads.
3. Open Settings again and confirm both key fields still show a stored value
   (this proves the OS keystore round-trip; the values are never in
   `localStorage`).
4. Choose pair `ja ↔ zh`, Mode **Listen**, press **Start**, press **Mic**, grant
   the microphone permission, and speak one sentence. Expected: the DEMO banner
   is gone, a live Deepgram Partial appears, a Segment from Groq appears, and
   the phone speaks the translation with the system voice. `adb logcat | grep
   -i tauri` shows no HTTP/WebSocket permission denial.
5. Converse gating: switch Mode to **Converse**, press **Start**, and speak as
   `me`. While the system voice speaks, the microphone frames are dropped; the
   next Segment starts only after speech ends plus the 500 ms tail. Turn
   switching still works and no Segment transcribes the spoken translation.
6. Failure paths: clear the Groq key and Save; the DEMO banner returns. Enter a
   wrong Deepgram key and press Mic; an `auth` error toast names Deepgram and
   suggests checking the key.
7. Optional server mode: start a Server Host (for example
   `PORT=3100 bun hosts/node/server.js` on the LAN), Settings → Mode
   **Connect to my server** → URL and Save. The app must show the Server Host's
   pairs and `ready`; Access Token enforcement is pending server-side.

iOS: `src-tauri/Info.ios.plist` declares `NSMicrophoneUsageDescription`; no IPA
is built here (needs macOS, ticket 18).
