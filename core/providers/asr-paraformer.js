// Alibaba DashScope Paraformer real-time ASR (Beijing region). The client sends
// run-task, waits for task-started, then streams raw PCM16 mono 16 kHz; the
// server returns result-generated frames whose sentence text is a cumulative
// hypothesis. Stable Prefix is LocalAgreement-2 over consecutive word lists;
// sentence_end text is stable outright. Partials are display-only. The adapter
// emits both event kinds itself, so it must NOT set revisable (the Session
// would derive a second, duplicate prefix).
// language_hints is fixed from the Direction's src: no auto-detection.
// Config data: model (default paraformer-realtime-v2) and url (default the
// Beijing DashScope endpoint; a workspace domain also works). Free quota:
// 10 h/month in the Beijing region, documented at
// https://help.aliyun.com/en/model-studio/websocket-for-paraformer-real-time-service
// and subject to change.
// Auth: Authorization: Bearer <DASHSCOPE_API_KEY> on the WebSocket handshake.
// The injected WebSocket implementation must support request headers (Node ws,
// the Tauri WebSocket plugin, or a host proxy); a browser's native WebSocket
// cannot set them.

const DEFAULT_URL = 'wss://dashscope.aliyuncs.com/api-ws/v1/inference';
const DEFAULT_MODEL = 'paraformer-realtime-v2';
const SAMPLE_RATE = 16000;
const FALLBACK_MOCK = 'or set ASR_PROVIDER=mock to run in Demo Mode.';

function errorEvent(kind, message, fallback) {
  return { type: 'error', provider: 'asr', kind, message, fallback };
}

function statusError(status) {
  if (status === 401 || status === 403) {
    return errorEvent(
      'auth',
      `Paraformer rejected the credentials (HTTP ${status})`,
      `Check DASHSCOPE_API_KEY and use a Beijing-region key, ${FALLBACK_MOCK}`,
    );
  }
  if (status === 402) {
    return errorEvent(
      'quota',
      `Paraformer has insufficient credit (HTTP ${status})`,
      `Top up in Model Studio; the free quota is 10 h/month in the Beijing region and may change, ${FALLBACK_MOCK}`,
    );
  }
  if (status === 429) {
    return errorEvent(
      'rate_limit',
      `Paraformer rate limit hit (HTTP ${status})`,
      `Wait and retry, ${FALLBACK_MOCK}`,
    );
  }
  return errorEvent(
    'unreachable',
    `Paraformer handshake failed (HTTP ${status})`,
    `Check the DashScope endpoint (Beijing region) and the network, ${FALLBACK_MOCK}`,
  );
}

function inBandError({ error_code: code = 'unknown', error_message: message = '' } = {}) {
  const name = String(code);
  const detail = message ? `: ${message}` : '';
  if (/arrear|overdue|payment|funds|allocation.?quota|insufficient|quota/i.test(name)) {
    return errorEvent(
      'quota',
      `Paraformer error ${name}${detail}`,
      `The Paraformer free quota (10 h/month, Beijing region; it may change) may be used up; top up in Model Studio, ${FALLBACK_MOCK}`,
    );
  }
  if (/throttl|rate.?quota|rate.?limit|too.?many/i.test(name)) {
    return errorEvent(
      'rate_limit',
      `Paraformer error ${name}${detail}`,
      `Wait and retry, ${FALLBACK_MOCK}`,
    );
  }
  if (/invalid.?api.?key|unauthor|access.?denied|authentication/i.test(name)) {
    return errorEvent(
      'auth',
      `Paraformer error ${name}${detail}`,
      `Check DASHSCOPE_API_KEY and use a Beijing-region key, ${FALLBACK_MOCK}`,
    );
  }
  if (
    /region|model.?not.?found|not.?available|unsupported|endpoint/i.test(name) ||
    /region/i.test(message)
  ) {
    return errorEvent(
      'region',
      `Paraformer error ${name}${detail}`,
      `Paraformer is served only from the Beijing region; check the endpoint and use a Beijing-region key, ${FALLBACK_MOCK}`,
    );
  }
  return errorEvent(
    'protocol',
    `Paraformer error ${name}${detail}`,
    `Check the Paraformer model and parameters, ${FALLBACK_MOCK}`,
  );
}

function languageHint(src) {
  return String(src).toLowerCase().split(/[-_]/)[0];
}

function tokenOf(word) {
  return String(word?.text ?? '');
}

function wordText(word) {
  return `${word?.text ?? ''}${word?.punctuation ?? ''}`;
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

function finite(value, fallback) {
  return Number.isFinite(value) ? value : fallback;
}

function spanOf(words, fallback) {
  return {
    startMs: finite(words[0]?.begin_time, fallback.startMs),
    endMs: finite(words.at(-1)?.end_time, fallback.endMs),
  };
}

// Character offset in the sentence text just after word `count - 1`, so the
// emitted text keeps Paraformer's own spacing and punctuation. Null when the
// word list and sentence text disagree.
function charsThrough(text, words, count) {
  let at = 0;
  for (let index = 0; index < count; index++) {
    const token = wordText(words[index]);
    if (!token) continue;
    const found = text.indexOf(token, at);
    if (found < 0) return null;
    at = found + token.length;
  }
  return at;
}

export function create({
  apiKey = '',
  model = DEFAULT_MODEL,
  url = DEFAULT_URL,
  WebSocket: WebSocketImpl = globalThis.WebSocket,
} = {}) {
  let socket = null;
  let generation = 0;
  let taskId = '';
  let started = false;
  let closing = false;
  let finished = false;
  let failed = false;
  let events = [];
  let pending = [];
  let previous = [];
  let emitted = 0;
  let emittedChars = 0;

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
  }

  function sendAudio(chunk) {
    try {
      socket.send(chunk);
    } catch (error) {
      fail(errorEvent('unreachable', `Paraformer send failed: ${error.message}`, FALLBACK_MOCK));
    }
  }

  function deltaText(text, words, from, to) {
    const end = charsThrough(text, words, to);
    if (end !== null && end >= emittedChars) return text.slice(emittedChars, end);
    const fresh = words.slice(from, to).map(wordText);
    return fresh.join(/\s/.test(text) ? ' ' : '');
  }

  function emitStable(words, text, count, fallback) {
    if (count <= emitted) return;
    const fresh = words.slice(emitted, count);
    const delta = deltaText(text, words, emitted, count);
    if (!delta) return;
    deliver({ type: 'stable', text: delta, ...spanOf(fresh, fallback) });
    emitted = count;
    emittedChars = charsThrough(text, words, count) ?? emittedChars + delta.length;
  }

  function handleSentence(sentence) {
    if (!sentence || sentence.heartbeat === true) return;
    const text = String(sentence.text ?? '');
    const words = Array.isArray(sentence.words) ? sentence.words : [];
    const fallback = {
      startMs: finite(sentence.begin_time, 0),
      endMs: finite(sentence.end_time, finite(sentence.begin_time, 0)),
    };
    const span = spanOf(words, fallback);

    if (sentence.sentence_end === true) {
      // Sentence-end text is stable outright: emit only the not-yet-stable tail.
      emitStable(words, text, words.length, fallback);
      if (words.length === 0 && text.length > emittedChars) {
        deliver({ type: 'stable', text: text.slice(emittedChars), ...span });
      }
      resetAgreement();
      return;
    }

    if (!text && words.length === 0) return;
    deliver({ type: 'partial', text, ...span });

    const agreed = commonWords(previous, words);
    if (agreed > emitted) emitStable(words, text, agreed, fallback);
    previous = words;
  }

  function handleMessage(raw) {
    if (failed || typeof raw !== 'string') return;
    let message;
    try {
      message = JSON.parse(raw);
    } catch {
      deliver(
        errorEvent('protocol', 'Paraformer sent a frame that is not JSON', FALLBACK_MOCK),
      );
      return;
    }
    const event = message.header?.event;
    if (event === 'task-started') {
      started = true;
      const queued = pending;
      pending = [];
      for (const chunk of queued) sendAudio(chunk);
    } else if (event === 'result-generated') {
      handleSentence(message.payload?.output?.sentence);
    } else if (event === 'task-failed') {
      fail(inBandError(message.header));
    } else if (event === 'task-finished') {
      finished = true;
    }
  }

  function open({ src } = {}) {
    if (!src) throw new Error('paraformer ASR needs the Direction source language');
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
    started = false;
    closing = false;
    finished = false;
    failed = false;
    resetAgreement();
    taskId = globalThis.crypto.randomUUID();

    if (!apiKey) {
      deliver(
        errorEvent(
          'auth',
          'DASHSCOPE_API_KEY is not set',
          `Set DASHSCOPE_API_KEY (Beijing region), ${FALLBACK_MOCK}`,
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

    try {
      socket = new WebSocketImpl(url, { headers: { Authorization: `Bearer ${apiKey}` } });
    } catch (error) {
      fail(
        errorEvent('unreachable', `Paraformer connection failed: ${error.message}`, FALLBACK_MOCK),
      );
      return;
    }
    const current = () => mine === generation;

    socket.onopen = () => {
      if (!current()) return;
      const frame = {
        header: { action: 'run-task', task_id: taskId, streaming: 'duplex' },
        payload: {
          task_group: 'audio',
          task: 'asr',
          function: 'recognition',
          model,
          parameters: {
            format: 'pcm',
            sample_rate: SAMPLE_RATE,
            language_hints: [languageHint(src)], // fixed per connection: no detection
          },
          input: {},
        },
      };
      try {
        socket.send(JSON.stringify(frame));
      } catch (error) {
        fail(
          errorEvent('unreachable', `Paraformer send failed: ${error.message}`, FALLBACK_MOCK),
        );
      }
    };
    socket.onmessage = (event) => {
      if (current()) handleMessage(event?.data);
    };
    socket.onerror = () => {
      if (current()) {
        fail(errorEvent('unreachable', 'Paraformer connection failed', FALLBACK_MOCK));
      }
    };
    socket.onclose = (event) => {
      if (!current() || finished) return;
      fail(
        errorEvent(
          'unreachable',
          `Paraformer connection closed (code ${event?.code ?? 'unknown'})`,
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
      if (started && !closing) sendAudio(pcm);
      else if (!closing) pending.push(pcm); // task-started must arrive first
    }
    const out = events;
    events = [];
    return out;
  }

  function close() {
    closing = true;
    pending = [];
    resetAgreement();
    if (!socket) return;
    const currentSocket = socket;
    if (started) {
      try {
        currentSocket.send(
          JSON.stringify({
            header: { action: 'finish-task', task_id: taskId, streaming: 'duplex' },
            payload: { input: {} },
          }),
        );
      } catch {
        // the socket is already unusable
      }
      // Let Paraformer flush final results before the socket goes away. Capture
      // the socket: a fast stop/start must not let this timer close the next one.
      const timer = setTimeout(() => {
        try {
          currentSocket.close();
        } catch {
          // already closed
        }
      }, 1000);
      timer.unref?.();
      return;
    }
    try {
      socket.close();
    } catch {
      // already closed
    }
  }

  return {
    name: 'paraformer',
    acceptsLive: true,
    open,
    close,
    push,
  };
}
