// WhisperLiveKit v0.2.26 local ASR. PCM16 mono 16 kHz goes up over /asr; the
// server answers with full-mode JSON. `lines` is resent in full on every
// update. Its last entry is still being extended, so a line is committed (and
// emitted as Stable Prefix exactly once) only after a later line or silence
// follows it; the interim tail is a display-only Partial, as is
// `buffer_transcription` when present. The adapter emits both event kinds
// itself, so it must NOT set revisable (the Session would derive a second,
// duplicate prefix). End of stream is an empty binary frame, then the server
// answers ready_to_stop and the last line is committed.
// `url` is config data (WHISPERLIVEKIT_URL); no language is hardcoded here.

const DEFAULT_URL = 'ws://127.0.0.1:8000';
const FALLBACK_MOCK = 'or set ASR_PROVIDER=mock to run in Demo Mode.';
const FALLBACK_STACK =
  'Check WHISPERLIVEKIT_URL and that the local stack is up ' +
  '(docker compose -f deploy/compose.yaml -f deploy/compose.local.yaml --profile local up -d), ' +
  FALLBACK_MOCK;
const FALLBACK_LOGS = `Check the WhisperLiveKit server logs, ${FALLBACK_MOCK}`;

function errorEvent(kind, message, fallback) {
  return { type: 'error', provider: 'asr', kind, message, fallback };
}

function statusError(status) {
  return status === 404
    ? errorEvent('unreachable', 'WhisperLiveKit has no /asr endpoint (HTTP 404)', FALLBACK_STACK)
    : errorEvent('unreachable', `WhisperLiveKit handshake failed (HTTP ${status})`, FALLBACK_STACK);
}

// "H:MM:SS.cc" from timed_objects.format_time().
function parseTime(value) {
  const match = /^(\d+):(\d{2}):(\d{2}(?:\.\d+)?)$/.exec(String(value ?? ''));
  if (!match) return null;
  return Math.round(((Number(match[1]) * 60 + Number(match[2])) * 60 + Number(match[3])) * 1000);
}

function endpoint(base, src) {
  const target = new URL(base);
  if (target.protocol === 'http:') target.protocol = 'ws:';
  else if (target.protocol === 'https:') target.protocol = 'wss:';
  const pathname = target.pathname.replace(/\/+$/, '');
  if (!pathname.endsWith('/asr')) target.pathname = `${pathname}/asr`;
  target.searchParams.set('language', src);
  target.searchParams.set('mode', 'full');
  return target.toString();
}

export function create({
  url = DEFAULT_URL,
  WebSocket: WebSocketImpl = globalThis.WebSocket,
  // shortcut: close waits 5 s for ready_to_stop; raise it if a cold model load
  // makes the server flush slower than that.
  stopTimeoutMs = 5000,
} = {}) {
  let socket = null;
  let generation = 0;
  let opened = false;
  let closing = false;
  let stopped = false;
  let failed = false;
  let events = [];
  let pending = [];
  let pendingAudio = [];
  let lastAudioMs = 0;
  let lastPartial = '';
  let lastLineEndMs = 0;
  let latestLines = [];
  let committed = [];
  let stopTimer = null;

  function deliver(event) {
    events.push(event);
  }

  function fail(event) {
    if (failed || closing) return;
    failed = true;
    deliver(event);
  }

  function send(chunk) {
    if (!socket || closing) return;
    try {
      socket.send(chunk);
    } catch (error) {
      fail(errorEvent('unreachable', `WhisperLiveKit send failed: ${error.message}`, FALLBACK_LOGS));
    }
  }

  function closeSocket() {
    stopped = true;
    closing = true;
    clearTimeout(stopTimer);
    stopTimer = null;
    try {
      socket?.close();
    } catch {
      // the socket is being discarded anyway
    }
  }

  function lineText(line) {
    return typeof line?.text === 'string' ? line.text.trim() : '';
  }

  function emitLine(index, line, text) {
    committed[index] = true;
    // shortcut: a line shares one audio span; WLK gives no per-token times.
    deliver({
      type: 'stable',
      text,
      startMs: parseTime(line.start) ?? lastLineEndMs,
      endMs: parseTime(line.end) ?? lastAudioMs,
    });
  }

  function commitLastLine() {
    const index = latestLines.length - 1;
    const line = latestLines[index];
    const text = lineText(line);
    if (!line || committed[index] || line.speaker === -2 || !text) return;
    emitLine(index, line, text);
  }

  function handleUpdate(message) {
    if (message.error) {
      fail(
        errorEvent('protocol', `WhisperLiveKit error: ${message.error}`, FALLBACK_LOGS),
      );
      return;
    }
    const lines = Array.isArray(message.lines) ? message.lines : [];
    if (Array.isArray(message.lines)) latestLines = lines;
    for (let index = 0; index < lines.length; index++) {
      const line = lines[index] ?? {};
      const endMs = parseTime(line.end);
      if (endMs !== null) lastLineEndMs = Math.max(lastLineEndMs, endMs);
      // Only a line followed by another line is committed; the last entry is
      // still being extended and must not reach the Stable Prefix yet.
      if (index >= lines.length - 1 || committed[index]) continue;
      const text = lineText(line);
      if (line.speaker === -2 || !text) {
        committed[index] = true;
        continue;
      }
      emitLine(index, line, text);
    }

    const last = lines.at(-1);
    const lastText = last && last.speaker !== -2 ? lineText(last) : '';
    const buffer =
      typeof message.buffer_transcription === 'string' ? message.buffer_transcription.trim() : '';
    const partial = buffer || lastText;
    if (partial && partial !== lastPartial) {
      deliver({
        type: 'partial',
        text: partial,
        ...(buffer || !last
          ? { startMs: lastLineEndMs, endMs: Math.max(lastLineEndMs, lastAudioMs) }
          : {
              startMs: parseTime(last.start) ?? lastLineEndMs,
              endMs: parseTime(last.end) ?? lastAudioMs,
            }),
      });
    }
    lastPartial = partial;
  }

  function handleMessage(raw) {
    if (typeof raw !== 'string') return;
    let message;
    try {
      message = JSON.parse(raw);
    } catch {
      fail(errorEvent('protocol', 'WhisperLiveKit sent a frame that is not JSON', FALLBACK_LOGS));
      return;
    }
    if (message?.type === 'config') {
      if (message.useAudioWorklet === false) {
        fail(
          errorEvent(
            'config',
            'WhisperLiveKit is not decoding raw PCM (useAudioWorklet=false)',
            `Start WhisperLiveKit with --pcm-input (the local Compose profile does), ${FALLBACK_MOCK}`,
          ),
        );
      }
      return;
    }
    if (message?.type === 'ready_to_stop') {
      // Flush updates that arrived before the stop, then the stream is final.
      for (const queued of pending) handleUpdate(queued);
      pending = [];
      commitLastLine();
      closeSocket();
      return;
    }
    if (!message || typeof message !== 'object') return;
    pending.push(message);
  }

  function open({ src } = {}) {
    if (!src) throw new Error('whisperlivekit ASR needs the Direction source language');
    try {
      socket?.close();
    } catch {
      // the old socket is being discarded anyway
    }
    socket = null;
    generation++;
    const mine = generation;
    events = [];
    pending = [];
    pendingAudio = [];
    opened = false;
    closing = false;
    stopped = false;
    failed = false;
    lastAudioMs = 0;
    lastPartial = '';
    lastLineEndMs = 0;
    latestLines = [];
    committed = [];

    if (!WebSocketImpl) {
      deliver(
        errorEvent('unreachable', 'No WebSocket implementation is available', FALLBACK_STACK),
      );
      return;
    }

    let target;
    try {
      target = endpoint(url || DEFAULT_URL, src);
      socket = new WebSocketImpl(target);
    } catch (error) {
      fail(errorEvent('unreachable', `WhisperLiveKit URL is invalid: ${error.message}`, FALLBACK_STACK));
      return;
    }
    const current = () => mine === generation;

    socket.onopen = () => {
      if (!current()) return;
      opened = true;
      const queued = pendingAudio;
      pendingAudio = [];
      for (const chunk of queued) send(chunk);
    };
    socket.onmessage = (event) => {
      if (current()) handleMessage(event?.data);
    };
    socket.onerror = () => {
      if (current()) fail(errorEvent('unreachable', 'WhisperLiveKit connection failed', FALLBACK_STACK));
    };
    socket.onclose = (event) => {
      if (!current()) return;
      fail(
        errorEvent(
          'unreachable',
          `WhisperLiveKit connection closed (code ${event?.code ?? 'unknown'})`,
          FALLBACK_STACK,
        ),
      );
    };
    // The ws package exposes the HTTP handshake status; browsers do not.
    socket.on?.('unexpected-response', (request, response) => {
      if (!current()) return;
      fail(statusError(response?.statusCode));
      response?.resume?.();
      request?.destroy?.();
    });
  }

  function push({ pcm, audioMs } = {}) {
    if (Number.isFinite(audioMs)) lastAudioMs = audioMs;
    if (pcm?.length && !closing) {
      if (opened) send(pcm);
      else pendingAudio.push(pcm);
    }
    if (!stopped) {
      const queued = pending;
      pending = [];
      for (const message of queued) handleUpdate(message);
    }
    const out = events;
    events = [];
    return out;
  }

  function close() {
    if (closing) return;
    pendingAudio = [];
    if (socket && opened && !stopped) {
      try {
        socket.send(new Uint8Array(0)); // end-of-audio: the server flushes and answers ready_to_stop
      } catch (error) {
        fail(errorEvent('unreachable', `WhisperLiveKit send failed: ${error.message}`, FALLBACK_LOGS));
      }
      stopTimer = setTimeout(closeSocket, stopTimeoutMs);
      stopTimer.unref?.();
    } else {
      closeSocket();
    }
    closing = true;
  }

  return { name: 'whisperlivekit', acceptsLive: true, open, push, close };
}
