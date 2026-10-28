// Deepgram Nova-3 live streaming ASR. PCM16 mono 16 kHz goes up, Results and
// UtteranceEnd frames come down. Stable Prefix is LocalAgreement-2 over
// consecutive interim hypotheses; is_final text is stable outright. Stable
// text is sliced from Deepgram's own transcript, so its spacing and punctuation
// are the provider's. Partials are display-only. The adapter emits both event
// kinds itself, so it must NOT set revisable (the Session would derive a
// second, duplicate prefix).
// Auth: Authorization header on node/tauri, token subprotocol on web.
// model/endpoint/auth are config data; no language is hardcoded here.

const DEFAULT_URL = 'wss://api.deepgram.com/v1/listen';
const SAMPLE_RATE = 16000;
const FALLBACK_MOCK = 'or set ASR_PROVIDER=mock to run in Demo Mode.';

function errorEvent(kind, message, fallback) {
  return { type: 'error', provider: 'asr', kind, message, fallback };
}

function statusError(status) {
  if (status === 401 || status === 403) {
    return errorEvent(
      'auth',
      `Deepgram rejected the credentials (HTTP ${status})`,
      `Check DEEPGRAM_API_KEY, ${FALLBACK_MOCK}`,
    );
  }
  if (status === 402) {
    return errorEvent(
      'quota',
      `Deepgram has insufficient credit (HTTP ${status})`,
      `Deepgram credit is exhausted; top up, ${FALLBACK_MOCK} whisperlivekit is a local alternative.`,
    );
  }
  if (status === 429) {
    return errorEvent(
      'rate_limit',
      `Deepgram rate limit hit (HTTP ${status})`,
      `Wait and retry, ${FALLBACK_MOCK}`,
    );
  }
  return errorEvent(
    'unreachable',
    `Deepgram handshake failed (HTTP ${status})`,
    `Check DEEPGRAM_URL and the network, ${FALLBACK_MOCK}`,
  );
}

function inBandError(message) {
  const code = String(message.err_code ?? message.description ?? 'unknown');
  const kind = /auth/i.test(code)
    ? 'auth'
    : /payment|funds|quota/i.test(code)
      ? 'quota'
      : 'protocol';
  return errorEvent(
    kind,
    `Deepgram error ${code}: ${message.err_msg ?? message.description ?? ''}`.trim(),
    kind === 'auth'
      ? `Check DEEPGRAM_API_KEY, ${FALLBACK_MOCK}`
      : `Check the Deepgram model/language settings, ${FALLBACK_MOCK}`,
  );
}

function tokenOf(word) {
  return word?.word ?? word?.punctuated_word ?? '';
}

function textOf(word) {
  return word?.punctuated_word ?? word?.word ?? '';
}

function commonWords(previous, current) {
  let count = 0;
  while (
    count < previous.length &&
    count < current.length &&
    tokenOf(previous[count]) === tokenOf(current[count])
  ) {
    count++;
  }
  return count;
}

// Character offset in the provider transcript just after word `count - 1`, so
// emitted text keeps Deepgram's own spacing and punctuation. Null when the word
// list and transcript disagree (e.g. smart_format rewrote them).
function charsThrough(text, words, count) {
  let at = 0;
  for (let index = 0; index < count; index++) {
    const token = textOf(words[index]);
    if (!token) continue;
    const found = text.indexOf(token, at);
    if (found < 0) return null;
    at = found + token.length;
  }
  return at;
}

function milliseconds(seconds, fallback) {
  return Number.isFinite(seconds) ? Math.round(seconds * 1000) : fallback;
}

function spanOf(words, fallback) {
  return {
    startMs: milliseconds(words[0]?.start, fallback.startMs),
    endMs: milliseconds(words.at(-1)?.end, fallback.endMs),
  };
}

export function create({
  apiKey = '',
  model = 'nova-3',
  endpointing = 300,
  utteranceEndMs = 1000,
  url = DEFAULT_URL,
  auth = 'header',
  WebSocket: WebSocketImpl = globalThis.WebSocket,
} = {}) {
  let socket = null;
  let generation = 0;
  let opened = false;
  let closing = false;
  let failed = false;
  let events = [];
  let pending = [];
  let previous = [];
  let emitted = 0;
  let emittedChars = 0;
  let utteranceStart = null;

  function deliver(event) {
    events.push(event);
  }

  function fail(event) {
    if (failed || closing) return;
    failed = true;
    deliver(event);
  }

  function resetAgreement() {
    previous = [];
    emitted = 0;
    emittedChars = 0;
    utteranceStart = null;
  }

  // Forget the agreement window without forgetting what was already emitted:
  // UtteranceEnd and close can arrive before the final Results for the same
  // words, and re-emitting them would duplicate a stable clause.
  function resetWords() {
    previous = [];
    emitted = 0;
  }

  function emitStable(words, text, count, fallback) {
    const end = charsThrough(text, words, count);
    const fresh = words.slice(emitted, count);
    let delta;
    let glue = '';
    if (end !== null) {
      if (end <= emittedChars) {
        emitted = count;
        return;
      }
      delta = text.slice(emittedChars, end);
    } else {
      // The word list and transcript disagree; use the words and let the
      // transcript's own whitespace (not a language guess) decide the spacing.
      const space = /\s/.test(text) ? ' ' : '';
      delta = fresh.map(textOf).join(space);
      glue = space;
    }
    if (!delta) return;
    deliver({ type: 'stable', text: delta, glue, ...spanOf(fresh, fallback) });
    emitted = count;
    emittedChars = end !== null ? end : emittedChars + delta.length;
  }

  function handleResults(message) {
    const alternative = message.channel?.alternatives?.[0];
    const words = Array.isArray(alternative?.words) ? alternative.words : [];
    const transcript = alternative?.transcript ?? '';
    if (!transcript && words.length === 0) return;

    // Deepgram's `start` is absolute stream time: a later start is a new utterance.
    const start = Number.isFinite(message.start) ? message.start : null;
    if (start !== null && (utteranceStart === null || start > utteranceStart)) {
      resetAgreement();
      utteranceStart = start;
    }

    const fallback = {
      startMs: milliseconds(message.start, 0),
      endMs: milliseconds((message.start ?? 0) + (message.duration ?? 0), 0),
    };

    if (message.is_final) {
      // is_final text is stable outright. Emit the provider transcript's
      // not-yet-emitted tail, so a final that lands after UtteranceEnd or
      // CloseStream cannot repeat words interims already made stable.
      const tail = transcript.slice(emittedChars);
      if (tail) {
        deliver({
          type: 'stable',
          text: tail,
          glue: '',
          ...spanOf(words.slice(emitted), fallback),
        });
      }
      emittedChars = Math.max(emittedChars, transcript.length);
      resetWords();
      return;
    }

    deliver({ type: 'partial', text: transcript, ...spanOf(words, fallback) });
    const agreed = commonWords(previous, words);
    if (agreed > emitted) emitStable(words, transcript, agreed, fallback);
    previous = words;
  }

  function handleMessage(raw) {
    if (typeof raw !== 'string') return;
    let message;
    try {
      message = JSON.parse(raw);
    } catch {
      deliver(errorEvent('protocol', 'Deepgram sent a frame that is not JSON', FALLBACK_MOCK));
      return;
    }
    if (message.type === 'Results') handleResults(message);
    else if (message.type === 'Error') deliver(inBandError(message));
    else if (message.type === 'UtteranceEnd') resetWords();
    // Metadata and SpeechStarted carry no text.
  }

  function send(chunk) {
    if (!socket || closing) return;
    try {
      socket.send(chunk);
    } catch (error) {
      fail(errorEvent('unreachable', `Deepgram send failed: ${error.message}`, FALLBACK_MOCK));
    }
  }

  function open({ src } = {}) {
    if (!src) throw new Error('deepgram ASR needs the Direction source language');
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
    opened = false;
    closing = false;
    failed = false;
    resetAgreement();

    if (!apiKey) {
      deliver(
        errorEvent(
          'auth',
          'DEEPGRAM_API_KEY is not set',
          `Set DEEPGRAM_API_KEY, ${FALLBACK_MOCK}`,
        ),
      );
      return;
    }
    if (!WebSocketImpl) {
      deliver(
        errorEvent('unreachable', 'No WebSocket implementation is available', FALLBACK_MOCK),
      );
      return;
    }

    const target = new URL(url);
    target.searchParams.set('model', model);
    target.searchParams.set('language', src); // fixed per connection: no auto-detection
    target.searchParams.set('interim_results', 'true');
    target.searchParams.set('endpointing', String(endpointing));
    target.searchParams.set('utterance_end_ms', String(utteranceEndMs));
    target.searchParams.set('encoding', 'linear16');
    target.searchParams.set('sample_rate', String(SAMPLE_RATE));
    target.searchParams.set('channels', '1');
    target.searchParams.set('punctuate', 'true');
    target.searchParams.set('smart_format', 'true');

    socket = new WebSocketImpl(
      target.toString(),
      auth === 'subprotocol'
        ? ['token', apiKey]
        : { headers: { authorization: `Token ${apiKey}` } },
    );
    const current = () => mine === generation;

    socket.onopen = () => {
      if (!current()) return;
      opened = true;
      const queued = pending;
      pending = [];
      for (const chunk of queued) send(chunk);
    };
    socket.onmessage = (event) => {
      if (current()) handleMessage(event?.data);
    };
    socket.onerror = () => {
      if (!current()) return;
      fail(errorEvent('unreachable', 'Deepgram connection failed', FALLBACK_MOCK));
    };
    socket.onclose = (event) => {
      if (!current()) return;
      fail(
        errorEvent(
          'unreachable',
          `Deepgram connection closed (code ${event?.code ?? 'unknown'})`,
          FALLBACK_MOCK,
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

  function push({ pcm } = {}) {
    if (pcm?.length) {
      if (opened && !closing) send(pcm);
      else if (!closing) pending.push(pcm);
    }
    const out = events;
    events = [];
    return out;
  }

  function close() {
    closing = true;
    pending = [];
    if (socket && opened) {
      // Let Deepgram flush its final Results before the socket goes away: it
      // closes the socket itself right after the last final, and the fallback
      // only fires when the server is stuck.
      try {
        socket.send(JSON.stringify({ type: 'CloseStream' }));
      } catch {
        // the socket is already unusable
      }
      const timer = setTimeout(() => {
        try {
          socket?.close();
        } catch {
          // already closed
        }
      }, 3000);
      timer.unref?.();
    } else {
      try {
        socket?.close();
      } catch {
        // already closed
      }
    }
  }

  return { name: 'deepgram', acceptsLive: true, open, push, close };
}
