// OpenAI-compatible chat-completions translator. One adapter serves every
// Preset; config/presets.js holds the endpoints and recommended models.
// Options come from the Server Host env: preset, baseUrl, apiKey, model,
// extraBody. Errors carry a kind (unreachable|auth|rate_limit|timeout|protocol)
// and a suggested fallback so the host can surface them instead of failing
// silently.

import { DEFAULT_PRESET, PRESETS } from '../../config/presets.js';

// shortcut: one whole-stream 30 s timeout, add a first-token timeout if slow local models need it.
const DEFAULT_TIMEOUT_MS = 30_000;
const BODY_LIMIT = 200;

export class ProviderError extends Error {
  constructor(kind, message, fallback) {
    super(message);
    this.name = 'ProviderError';
    this.kind = kind;
    this.fallback = fallback;
  }
}

export function create({
  preset: presetId = '',
  baseUrl = '',
  apiKey = '',
  model = '',
  extraBody = '',
  fetch: fetchImpl = globalThis.fetch,
  timeoutMs = DEFAULT_TIMEOUT_MS,
} = {}) {
  const id = presetId || DEFAULT_PRESET;
  const preset = PRESETS[id];
  if (!preset) {
    throw new Error(`unknown preset "${id}"; known presets: ${Object.keys(PRESETS).join(', ')}`);
  }

  const base = trimSlash(baseUrl || preset.baseUrl);
  if (!base) throw new Error(`preset "${id}" requires LLM_BASE_URL`);
  const parsed = parseHttpUrl(base, id);
  const endpoint = parsed.endsWith('/chat/completions')
    ? parsed
    : `${parsed}/chat/completions`;
  const selectedModel = model || preset.model;
  if (!selectedModel) throw new Error(`preset "${id}" requires LLM_MODEL`);
  const extra = parseExtraBody(extraBody);

  const headers = { 'content-type': 'application/json' };
  if (apiKey) headers.authorization = `Bearer ${apiKey}`;

  return {
    name: 'openai',
    preset: preset.id,
    model: selectedModel,
    baseUrl: parsed,
    trainsOnData: preset.trainsOnData,
    async *translate({ system = '', user = '' } = {}) {
      if (preset.requiresKey && !apiKey) {
        throw new ProviderError(
          'auth',
          `LLM_API_KEY is not set for preset "${preset.id}"`,
          'Set LLM_API_KEY, or unset the LLM_* variables to use Demo Mode.',
        );
      }

      let timedOut = false;
      const controller = new AbortController();
      const timer = setTimeout(() => {
        timedOut = true;
        controller.abort();
      }, timeoutMs);

      try {
        let response;
        try {
          response = await fetchImpl(endpoint, {
            method: 'POST',
            headers,
            // Extra fields merge first; Parley's required fields always win,
            // so model/messages/stream cannot be overridden.
            body: JSON.stringify({
              ...extra,
              model: selectedModel,
              stream: true,
              messages: [
                { role: 'system', content: String(system) },
                { role: 'user', content: String(user) },
              ],
            }),
            signal: controller.signal,
          });
        } catch (error) {
          throw transportError(error, timedOut, preset.id, timeoutMs);
        }

        if (!response.ok) {
          const detail = await response.text().catch(() => '');
          throw statusError(response.status, detail, preset.id, apiKey);
        }
        if (!response.body) {
          throw new ProviderError(
            'protocol',
            `translator sent no response body (${preset.id})`,
            'Check LLM_BASE_URL and LLM_MODEL for this Preset.',
          );
        }

        let emitted = false;
        try {
          for await (const content of readSse(response.body)) {
            emitted = true;
            yield content;
          }
        } catch (error) {
          if (error instanceof ProviderError) throw error;
          if (timedOut) throw timeoutError(preset.id, timeoutMs);
          throw new ProviderError(
            'protocol',
            `translator sent a malformed stream (${preset.id}): ${error?.message ?? error}`,
            'Switch LLM_PRESET, or check the endpoint and model with LLM_BASE_URL / LLM_MODEL.',
          );
        }
        if (!emitted) {
          throw new ProviderError(
            'protocol',
            `translator returned no text (${preset.id})`,
            'Switch LLM_PRESET, or check the endpoint and model with LLM_BASE_URL / LLM_MODEL.',
          );
        }
      } finally {
        clearTimeout(timer);
      }
    },
  };
}

async function* readSse(body) {
  const decoder = new TextDecoder();
  let buffer = '';
  let done = false;

  const parseLine = (rawLine) => {
    const line = rawLine.endsWith('\r') ? rawLine.slice(0, -1) : rawLine;
    if (!line.startsWith('data:')) return undefined;
    const payload = line.slice(5).trim();
    if (!payload) return undefined;
    if (payload === '[DONE]') {
      done = true;
      return undefined;
    }
    let event;
    try {
      event = JSON.parse(payload);
    } catch {
      throw new ProviderError(
        'protocol',
        `translator sent malformed SSE JSON: ${payload.slice(0, BODY_LIMIT)}`,
        'Switch LLM_PRESET, or check the endpoint with LLM_BASE_URL.',
      );
    }
    if (event?.error) {
      const message = event.error.message ?? JSON.stringify(event.error);
      throw new ProviderError(
        'protocol',
        `translator error event: ${String(message).slice(0, BODY_LIMIT)}`,
        'Switch LLM_PRESET, or check the endpoint and model.',
      );
    }
    const content = event?.choices?.[0]?.delta?.content;
    return typeof content === 'string' && content !== '' ? content : undefined;
  };

  for await (const chunk of body) {
    buffer += decoder.decode(chunk, { stream: true });
    let newline;
    while (!done && (newline = buffer.indexOf('\n')) >= 0) {
      const line = buffer.slice(0, newline);
      buffer = buffer.slice(newline + 1);
      const content = parseLine(line);
      if (content !== undefined) yield content;
    }
    if (done) return;
  }
  buffer += decoder.decode();
  if (!done && buffer) {
    const content = parseLine(buffer);
    if (content !== undefined) yield content;
  }
}

function parseExtraBody(value) {
  if (value === '' || value == null) return {};
  let parsed = value;
  if (typeof value === 'string') {
    try {
      parsed = JSON.parse(value);
    } catch (error) {
      throw new Error(`LLM_EXTRA_BODY is not valid JSON: ${error.message}`);
    }
  }
  if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed)) {
    throw new Error('LLM_EXTRA_BODY must be a JSON object');
  }
  return parsed;
}

function trimSlash(url) {
  return String(url).replace(/\/+$/, '');
}

function parseHttpUrl(url, id) {
  let parsed;
  try {
    parsed = new URL(url);
  } catch {
    throw new Error(`preset "${id}" has an invalid LLM_BASE_URL`);
  }
  if (parsed.protocol !== 'http:' && parsed.protocol !== 'https:') {
    throw new Error(`preset "${id}" LLM_BASE_URL must be http or https`);
  }
  if (parsed.username || parsed.password) {
    throw new Error(`preset "${id}" LLM_BASE_URL must not embed credentials; use LLM_API_KEY`);
  }
  return trimSlash(parsed.toString());
}

function transportError(error, timedOut, id, timeoutMs) {
  if (timedOut || error?.name === 'TimeoutError') return timeoutError(id, timeoutMs);
  return new ProviderError(
    'unreachable',
    `translator unreachable (${id}): ${error?.message ?? error}`,
    'Check the network and LLM_BASE_URL; for a local model, start Ollama or set LLM_BASE_URL.',
  );
}

function timeoutError(id, timeoutMs) {
  return new ProviderError(
    'timeout',
    `translator timed out after ${timeoutMs} ms (${id})`,
    'Retry, switch LLM_PRESET, or choose a lighter LLM_MODEL.',
  );
}

function statusError(status, detail, id, apiKey) {
  let snippet = String(detail).replace(/\s+/g, ' ').trim().slice(0, BODY_LIMIT);
  if (apiKey) snippet = snippet.replaceAll(apiKey, '<redacted>');
  const suffix = snippet ? `: ${snippet}` : '';
  if (status === 401 || status === 403) {
    return new ProviderError(
      'auth',
      `translator auth failed (${status}) from ${id}${suffix}`,
      'Check LLM_API_KEY; to run without a key, unset the LLM_* variables for Demo Mode.',
    );
  }
  if (status === 429) {
    return new ProviderError(
      'rate_limit',
      `translator rate limited (429) from ${id}${suffix}`,
      'Wait a few seconds, switch LLM_PRESET, or choose a lighter LLM_MODEL.',
    );
  }
  return new ProviderError(
    'protocol',
    `translator protocol error (${status}) from ${id}${suffix}`,
    'Check LLM_BASE_URL and LLM_MODEL for this Preset.',
  );
}
