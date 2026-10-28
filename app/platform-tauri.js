// The tauri platform adapter. It is the only place that touches Tauri APIs:
// native streaming HTTP (no WebView CORS) and header-authenticated WebSockets
// (no browser header limit). The Session core receives both by injection.
// The plugin modules are vendored under ./vendor with their bare imports
// rewritten so the shell keeps working without a bundler.

// shortcut: one hard 35 s deadline per request; make it per-Provider when a
// slow local model needs a longer stream than the 30 s Provider timeout.
const DEFAULT_TIMEOUT_MS = 35_000;
const DEFAULT_BATCH_MS = 150; // 100–250 ms of PCM per WebSocket send
const CONNECT_TIMEOUT_MS = 15_000;

function namedError(name, message) {
  const error = new Error(message);
  error.name = name;
  return error;
}

// Wraps the Tauri HTTP plugin's fetch. The plugin's abort support can leave a
// body read hanging, so this adds a hard deadline and drops every chunk that
// arrives after an abort instead of waiting for it.
export function createTauriFetch({ fetch: nativeFetch, timeoutMs = DEFAULT_TIMEOUT_MS } = {}) {
  if (typeof nativeFetch !== 'function') {
    throw new Error('createTauriFetch needs the Tauri HTTP plugin fetch()');
  }
  const inflight = new Map();

  function abortAll() {
    for (const stop of [...inflight.values()]) stop();
  }

  async function platformFetch(url, init = {}) {
    const controller = new AbortController();
    let reason = '';
    let timer = null;
    let rejectAborted = () => {};
    const aborted = new Promise((_, reject) => {
      rejectAborted = reject;
    });
    aborted.catch(() => {});

    const stop = (name) => {
      if (reason) return;
      reason = name;
      controller.abort();
      rejectAborted(
        namedError(name, name === 'TimeoutError' ? 'request timed out' : 'request aborted'),
      );
    };
    inflight.set(controller, () => stop('AbortError'));
    const onAbort = () => stop('AbortError');
    if (init.signal?.aborted) stop('AbortError');
    else init.signal?.addEventListener?.('abort', onAbort, { once: true });
    timer = setTimeout(() => stop('TimeoutError'), timeoutMs);

    const release = () => {
      if (timer !== null) clearTimeout(timer);
      timer = null;
      init.signal?.removeEventListener?.('abort', onAbort);
      inflight.delete(controller);
    };

    try {
      const pending = nativeFetch(url, { ...init, signal: controller.signal });
      // A response that arrives after the deadline has no reader left; close it.
      pending.then(
        (response) => {
          if (reason) response.body?.cancel?.().catch?.(() => {});
        },
        () => {},
      );
      const response = await Promise.race([pending, aborted]);
      if (!response.body) {
        release();
        return response;
      }
      const reader = response.body.getReader();
      const stream = new ReadableStream(
        {
          async pull(out) {
            if (reason) {
              release();
              out.error(namedError(reason, reason === 'TimeoutError' ? 'request timed out' : 'request aborted'));
              return;
            }
            let result;
            try {
              result = await Promise.race([reader.read(), aborted]);
            } catch (error) {
              release();
              if (reason) {
                try {
                  reader.cancel().catch(() => {});
                  out.error(namedError(reason, reason === 'TimeoutError' ? 'request timed out' : 'request aborted'));
                } catch {
                  // the consumer is already gone
                }
              } else {
                out.error(error);
              }
              return;
            }
            if (reason) {
              release();
              return; // a chunk that raced the abort is intentionally dropped
            }
            if (result.done) {
              release();
              out.close();
              return;
            }
            out.enqueue(result.value);
          },
          cancel() {
            stop('AbortError');
            release();
            return reader.cancel().catch(() => {});
          },
        },
        { highWaterMark: 0 },
      );
      return new Response(stream, {
        status: response.status,
        statusText: response.statusText,
        headers: response.headers,
      });
    } catch (error) {
      release();
      throw error;
    }
  }

  platformFetch.abortAll = abortAll;
  return platformFetch;
}

// A browser-WebSocket-shaped facade over the Tauri WebSocket plugin. Provider
// adapters construct it with `new`, set on* callbacks and call send/close.
// Binary PCM is buffered and flushed every batchMs (100–250 ms) because the
// core emits one 20 ms microphone frame at a time.
export function createTauriWebSocket({
  connect,
  batchMs = DEFAULT_BATCH_MS,
  maxBatchBytes = Math.round(batchMs * 32), // 16 kHz mono PCM16 = 32 bytes/ms
  connectTimeoutMs = CONNECT_TIMEOUT_MS,
} = {}) {
  if (typeof connect !== 'function') {
    throw new Error('createTauriWebSocket needs the Tauri WebSocket plugin connect()');
  }

  return class TauriWebSocket {
    static CONNECTING = 0;
    static OPEN = 1;
    static CLOSING = 2;
    static CLOSED = 3;

    readyState = TauriWebSocket.CONNECTING;
    binaryType = 'arraybuffer';
    url;
    onopen = null;
    onmessage = null;
    onerror = null;
    onclose = null;
    #socket = null;
    #closed = false;
    #batch = [];
    #bytes = 0;
    #timer = null;

    constructor(url, options) {
      this.url = String(url);
      this.#connect(options);
    }

    async #connect(options) {
      let timer = null;
      try {
        // The plugin has no subprotocol support; providers use header auth.
        const config = Array.isArray(options) ? undefined : options;
        const connecting = connect(this.url, config);
        connecting.catch(() => {}); // losing the timeout race must not be unhandled
        const timeout = new Promise((_, reject) => {
          timer = setTimeout(
            () => reject(new Error('WebSocket connect timed out')),
            connectTimeoutMs,
          );
        });
        const socket = await Promise.race([connecting, timeout]);
        if (this.#closed) {
          Promise.resolve(socket.disconnect()).catch(() => {});
          return;
        }
        this.#socket = socket;
        socket.addListener((message) => this.#receive(message));
        this.readyState = TauriWebSocket.OPEN;
        this.onopen?.({});
      } catch (error) {
        this.#fail(error);
      } finally {
        if (timer !== null) clearTimeout(timer);
      }
    }

    send(data) {
      if (this.#closed || this.readyState >= TauriWebSocket.CLOSING) return;
      if (typeof data === 'string') {
        this.#send(data);
        return;
      }
      const bytes =
        data instanceof ArrayBuffer
          ? new Uint8Array(data)
          : new Uint8Array(data.buffer, data.byteOffset ?? 0, data.byteLength);
      this.#batch.push(bytes);
      this.#bytes += bytes.byteLength;
      if (this.#timer === null) this.#timer = setTimeout(() => this.#flush(), batchMs);
      if (this.#bytes >= maxBatchBytes) this.#flush();
    }

    close() {
      if (this.readyState === TauriWebSocket.CLOSED) return;
      this.readyState = TauriWebSocket.CLOSING;
      this.#closed = true;
      this.#dropBatch();
      const socket = this.#socket;
      this.#socket = null;
      Promise.resolve(socket?.disconnect()).catch(() => {});
      this.readyState = TauriWebSocket.CLOSED;
      this.onclose?.({ code: 1000, reason: 'client closed' });
    }

    #flush() {
      if (this.#timer !== null) clearTimeout(this.#timer);
      this.#timer = null;
      if (this.#batch.length === 0) return;
      const merged = new Uint8Array(this.#bytes);
      let at = 0;
      for (const chunk of this.#batch) {
        merged.set(chunk, at);
        at += chunk.byteLength;
      }
      this.#batch = [];
      this.#bytes = 0;
      this.#send(Array.from(merged));
    }

    #dropBatch() {
      if (this.#timer !== null) clearTimeout(this.#timer);
      this.#timer = null;
      this.#batch = [];
      this.#bytes = 0;
    }

    #send(payload) {
      if (!this.#socket || this.readyState !== TauriWebSocket.OPEN) return;
      Promise.resolve(this.#socket.send(payload)).catch((error) => this.#fail(error));
    }

    #receive(message) {
      if (!message) return;
      if (message.type === 'Text') {
        this.onmessage?.({ data: message.data });
        return;
      }
      if (message.type === 'Binary') {
        this.onmessage?.({ data: new Uint8Array(message.data).buffer });
        return;
      }
      if (message.type === 'Close') {
        if (this.readyState === TauriWebSocket.CLOSED) return;
        this.readyState = TauriWebSocket.CLOSED;
        this.#closed = true;
        this.#dropBatch();
        this.onclose?.(message.data ?? {});
      }
    }

    #fail(error) {
      if (this.readyState === TauriWebSocket.CLOSED) return;
      this.readyState = TauriWebSocket.CLOSED;
      this.#closed = true;
      this.#dropBatch();
      this.onerror?.({ error });
      this.onclose?.({ code: 1006, reason: String(error?.message ?? error) });
    }
  };
}

export async function loadTauriPlatform(options = {}) {
  const [http, websocket] = await Promise.all([
    import('./vendor/plugin-http.js'),
    import('./vendor/plugin-websocket.js'),
  ]);
  return {
    fetch: createTauriFetch({ fetch: http.fetch, ...options }),
    WebSocket: createTauriWebSocket({ connect: websocket.default.connect, ...options }),
  };
}
