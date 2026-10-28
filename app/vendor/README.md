# Vendored Tauri plugin JS

The shell has no bundler. These files are copied verbatim from the exact npm
packages below, with their bare `@tauri-apps/api/*` imports rewritten to
`./api/*` so they load as plain ES modules from `dist/vendor`. Update them by
re-copying the package `dist-js/index.js` files; keep the import rewrite.

| File | Source package | Version | License |
| --- | --- | --- | --- |
| `plugin-http.js` | `@tauri-apps/plugin-http` | 2.8.3 | Apache-2.0 OR MIT |
| `plugin-websocket.js` | `@tauri-apps/plugin-websocket` | 2.5.0 | Apache-2.0 OR MIT |
| `plugin-tts.js` | `tauri-plugin-tts-api` | 0.2.1 | MIT |
| `plugin-keystore.js` | `tauri-plugin-secure-keystore` | 0.0.3-beta.1 | Apache-2.0 OR MIT |
| `api/core.js`, `api/event.js`, `api/external/tslib/*` | `@tauri-apps/api` | 2.12.4 | Apache-2.0 OR MIT |

License texts are in `licenses/`. The Rust sides are pinned in
`../src-tauri/Cargo.toml`.
