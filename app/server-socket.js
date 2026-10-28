// "Connect to my server" mode: the app is a protocol client of a Server Host.
// shortcut: the token goes in the query string because Server Host handshake
// enforcement is still pending; switch to an Authorization header once it is.
// The unmodified web client opens ws(s)://<webview-host>/ws, so this facade
// redirects that socket to the configured server and appends the Access Token
// once the keystore read resolves. Browser WebSockets cannot send headers.

export function serverSocketUrl(serverUrl, token = '') {
  const target = new URL(serverUrl);
  const secure = target.protocol === 'https:' || target.protocol === 'wss:';
  target.protocol = secure ? 'wss:' : 'ws:';
  const path = target.pathname.replace(/\/+$/, '');
  target.pathname = path.endsWith('/ws') ? path : `${path}/ws`;
  target.search = '';
  target.hash = '';
  if (token) target.searchParams.set('token', token);
  return target.toString();
}

export function createServerWebSocket({
  Native = globalThis.WebSocket,
  serverUrl,
  token = Promise.resolve(''),
  target = serverSocketUrl,
} = {}) {
  if (typeof Native !== 'function') {
    throw new Error('createServerWebSocket needs a native WebSocket implementation');
  }

  return class ServerWebSocket {
    static CONNECTING = 0;
    static OPEN = 1;
    static CLOSING = 2;
    static CLOSED = 3;

    readyState = ServerWebSocket.CONNECTING;
    binaryType = 'arraybuffer';
    url;
    #native = null;
    #queue = [];
    #listeners = new Map();
    #closed = false;
    #protocols;

    constructor(url, protocols) {
      this.url = String(url);
      this.#protocols = protocols;
      this.#connect();
    }

    addEventListener(type, listener) {
      if (!this.#listeners.has(type)) this.#listeners.set(type, new Set());
      this.#listeners.get(type).add(listener);
    }

    removeEventListener(type, listener) {
      this.#listeners.get(type)?.delete(listener);
    }

    send(data) {
      if (this.#closed) return;
      if (this.#native?.readyState === ServerWebSocket.OPEN) this.#native.send(data);
      else this.#queue.push(data);
    }

    close() {
      if (this.#closed) return;
      this.#closed = true;
      this.readyState = ServerWebSocket.CLOSED;
      try {
        this.#native?.close();
      } catch {
        // already gone
      }
      this.#fire('close', {});
    }

    async #connect() {
      let secret = '';
      try {
        secret = (await token) || '';
      } catch {
        // no token: connect anyway, the server prompt is not implemented yet
      }
      if (this.#closed) return;
      let native;
      try {
        native = new Native(target(serverUrl, secret), this.#protocols);
      } catch (error) {
        this.#fail(error);
        return;
      }
      this.#native = native;
      this.url = native.url ?? this.url;
      native.binaryType = 'arraybuffer';
      native.addEventListener('open', (event) => {
        this.readyState = ServerWebSocket.OPEN;
        const queued = this.#queue;
        this.#queue = [];
        for (const data of queued) native.send(data);
        this.#fire('open', event);
      });
      native.addEventListener('message', (event) => this.#fire('message', event));
      native.addEventListener('error', (event) => this.#fire('error', event));
      native.addEventListener('close', (event) => {
        this.readyState = ServerWebSocket.CLOSED;
        this.#closed = true;
        this.#fire('close', event);
      });
    }

    #fail(error) {
      if (this.#closed) return;
      this.#closed = true;
      this.readyState = ServerWebSocket.CLOSED;
      this.#fire('error', { error });
      this.#fire('close', {});
    }

    #fire(type, event) {
      for (const listener of this.#listeners.get(type) ?? []) listener(event);
    }
  };
}
