# Third-party notices

Parley itself is licensed under the Apache License 2.0 (see `LICENSE`). This page lists third-party components that are bundled in this repository, plus services the documented deployment downloads. It is not legal advice; the upstream licences govern.

## Bundled JavaScript

| Component | Version | Licence | Location |
|---|---|---|---|
| `ws` (Node WebSocket client/server) | 8.22.0 | MIT | runtime dependency, `node_modules/ws` |
| `@tauri-apps/api` | 2.12.4 | Apache-2.0 OR MIT | vendored JS under `app/vendor/api/` |
| `@tauri-apps/plugin-http` | 2.8.3 | Apache-2.0 OR MIT | `app/vendor/plugin-http.js` |
| `@tauri-apps/plugin-websocket` | 2.5.0 | Apache-2.0 OR MIT | `app/vendor/plugin-websocket.js` |
| `tauri-plugin-tts-api` | 0.2.1 | MIT | `app/vendor/plugin-tts.js` |
| `tauri-plugin-secure-keystore` | 0.0.3-beta.1 | Apache-2.0 OR MIT | `app/vendor/plugin-keystore.js` |

The vendored files are copied from the exact npm packages with bare `@tauri-apps/api/*` imports rewritten; see `app/vendor/README.md`. Their full licence texts are in `app/vendor/licenses/`.

## Bundled Rust dependencies

The Tauri shell pins its Rust dependencies in `app/src-tauri/Cargo.toml` and `app/src-tauri/Cargo.lock`. In particular:

| Component | Version | Licence |
|---|---|---|
| `tauri` | 2.12.5 | Apache-2.0 OR MIT |
| `tauri-build` | 2.7.4 | Apache-2.0 OR MIT |
| `tauri-plugin-http` | 2.8.3 | Apache-2.0 OR MIT |
| `tauri-plugin-websocket` | 2.5.0 | Apache-2.0 OR MIT |
| `tauri-plugin-tts` | 0.2.1 | MIT |
| `tauri-plugin-secure-keystore` | 0.0.3-beta.1 | Apache-2.0 OR MIT |

Transitive crates and their licence texts are resolved by `Cargo.lock` and the packages' own metadata.

## Sample audio and transcripts

All WAV files in `samples/` and their timed transcripts and reference translations were generated with [Kokoro-FastAPI](https://github.com/remsky/Kokoro-FastAPI) using the [Kokoro-82M](https://huggingface.co/hexgrad/Kokoro-82M) weights, which are Apache-2.0. The generated audio, transcripts, translations and sample text are released under CC0 1.0 Universal. See `samples/LICENSE.txt`.

## Downloaded by the documented deployment (not vendored)

These run as separate services or images and keep their own licences and notices:

| Component | Licence | Role |
|---|---|---|
| Caddy | Apache-2.0 | HTTPS reverse proxy in `deploy/compose.yaml` |
| Node.js (`node:24-alpine`) | MIT | Server Host runtime |
| WhisperLiveKit | Apache-2.0 (SimulStreaming MIT since 2025-10-22) | optional local ASR sidecar |
| Kokoro-FastAPI | Apache-2.0 | local TTS sidecar; Kokoro-82M weights Apache-2.0 |
| Ollama | MIT | optional local LLM runtime (external) |
| Deepgram, DashScope/Paraformer, Groq, Gemini, SiliconFlow, OpenRouter, Mistral, DeepSeek, Zhipu, Moonshot, Cerebras, Azure Speech, Microsoft Edge TTS | provider terms | cloud Providers selected by the user |

Parley contains no code copied from PolyTalk or any AGPL/GPL/LGPL project. The Edge TTS client is an independent implementation of the public service protocol and is labelled unofficial.
