# One JS orchestration core, hosted on a server or inside the mobile app

The original spec made the product a phone-browser web app backed by a self-hosted orchestration server, and it explicitly forbade native apps. We reversed that: Parley also ships an APK and an IPA, so that someone can install it, paste a free-tier API key, and use it without owning a server. To make that possible, the orchestration core (segmentation, hold/commit, rolling context, TTS queue) is written once as plain JavaScript ES modules with no build step. It runs in two hosts:

- **Server host:** Node, deployed via Docker or bare metal and configured with environment variables. It serves plain browsers over the WebSocket protocol, and it is the only home for the local model stack (WhisperLiveKit, Ollama, Kokoro).
- **Device host:** inside the app shell, calling cloud providers directly. Keys are stored in the OS keystore. In this host the OS system TTS is the default.

The app can also act as a thin client of a server host.

The app shell wraps the same vanilla web client. There is no native UI code beyond shell plugins.

## Considered options

- **Thin-client app only.** Simplest, but useless without a server, which defeats "install and use".
- **Python server, with the app as a thin client.** Fits the ASR/TTS ecosystem, but the core could not run on-device without a second implementation.
- **Separate native implementations.** Two or three codebases for one product.

## Consequences

- The backend is Node rather than Python. Python-only components (WhisperLiveKit, Kokoro) are consumed as network services, never in-process.
- In-app network calls must bypass WebView CORS and WebSocket-header limits through the shell's native networking.
- IPA builds need a macOS CI runner. Until an Apple Developer account exists, releases ship an unsigned IPA for sideloading and a signed APK.
