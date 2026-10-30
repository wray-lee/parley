# Server Host deployment

The Server Host is one static Go binary (`cmd/parley-server`) on a distroless
image. It serves the Flutter web client at `/` and the vanilla client at
`/lite`, plus `/healthz` and `/samples`; nothing is fetched from a CDN, so
CanvasKit and the Noto CJK fallback slices come from the server. It proxies to
no external service in Demo Mode. Caddy terminates HTTPS: an internal CA by
default, Let's Encrypt when `PUBLIC_DOMAIN` is set.

## Docker quick start

```sh
cp .env.example .env            # optional; no .env is Demo Mode
docker compose -f deploy/compose.yaml up -d --build
```

Open `https://localhost/` (or `https://<server-ip>/` from a phone) and accept
the certificate warning, or trust the root CA below. `/healthz` answers
`{"ok":true}`.

With a domain:

```sh
# PUBLIC_DOMAIN=parley.example.com in .env
docker compose -f deploy/compose.yaml --env-file .env up -d --build
```

Ports 80 and 443 must be reachable from the internet so the ACME challenge
succeeds. Caddy renews certificates on its own.

The optional local model sidecars are stubs owned by tickets 06 and 08 and are
off by default:

```sh
docker compose -f deploy/compose.yaml --profile local up -d
```

Ollama stays external; point `LLM_BASE_URL` at it.

## Trusting the internal root CA (no domain)

Export the CA that Caddy generated:

```sh
docker compose -f deploy/compose.yaml cp caddy:/data/caddy/pki/authorities/local/root.crt ./root.crt
```

- **iOS/iPadOS:** send `root.crt` to the device (AirDrop, Files, email), tap it
  to install the profile, then Settings > General > VPN & Device Management >
  install. Finally Settings > General > About > Certificate Trust Settings and
  enable full trust for the Parley root. Safari only accepts the microphone on
  a trusted HTTPS origin.
- **Android:** copy `root.crt` to the device, then Settings > Security >
  Encryption & credentials > Install a certificate > CA certificate > install.
  Android 11+ shows a warning and some apps ignore user-installed CAs; Chrome
  accepts them after the install.
- **Desktop:** import `root.crt` into the OS/browser trust store, or pass
  `-k` to curl for a quick check.

## Tailscale or Cloudflare Tunnel instead of a CA

If phones cannot reach the host directly, or you do not want to install a CA,
terminate TLS with a tunnel and leave `PUBLIC_DOMAIN` empty:

- **Tailscale:** run the host on loopback (see bare metal below) and
  `tailscale serve --bg 3000`. Tailscale issues a trusted certificate for the
  machine's MagicDNS name, so the browser gets a secure context over the
  tailnet.
- **Cloudflare Tunnel:** `cloudflared tunnel --url http://127.0.0.1:3000`
  (quick tunnel) or a named tunnel to the same origin. Cloudflare terminates
  TLS with a publicly trusted certificate.

Both alternatives assume the Server Host is reachable on `127.0.0.1:3000`; they
replace the Caddy service rather than running in front of it. This paragraph is
the input for ticket 19.

## Bare metal (no Docker)

The same environment variables and one static binary:

```sh
CGO_ENABLED=0 go build -o parley-server ./cmd/parley-server
mkdir -p web-app
tar -xzf parley-<version>-web.tar.gz -C web-app   # the Flutter web build
./parley-server
```

`PARLEY_WEB_DIR` points at another directory. Without it the server looks for
`web-app/index.html` next to its working directory and otherwise keeps the
embedded vanilla client at `/` (and `/lite`).

It listens on `PORT` (default 3000) on all interfaces; set `HOST=127.0.0.1`
to bind loopback only. Put Caddy, nginx or a
tunnel in front for HTTPS; keep `ACCESS_TOKEN` set if the port is reachable by
anyone else. To use Caddy directly with the same certificates, reuse
`deploy/Caddyfile` and change `app:3000` to `127.0.0.1:3000`.

The Docker image is a two-stage build: `golang:1.26.5-alpine` compiles the
binary with `CGO_ENABLED=0`, and `gcr.io/distroless/static-debian12:nonroot`
runs it with no shell. The compose healthcheck therefore calls the binary
itself: `/app/parley-server -healthcheck` probes `/healthz` on `PORT`.

## Access Token

`ACCESS_TOKEN` is one optional shared passphrase for the whole host, not an
account and not a user system. Set it before exposing the host to the
internet; leave it empty on a trusted LAN. The Server Host rejects a WebSocket
handshake whose `/ws?token=` does not match. The Flutter web client asks for
the token after the first failed connect and keeps it in that tab's memory
only; the vanilla `/lite` client uses `sessionStorage`.
