// Kokoro-FastAPI: OpenAI-style /v1/audio/speech returning mp3. The voice map
// is data in config/voices.json; target language is the Direction's tgt.

import voiceConfig from '../../config/voices.json' with { type: 'json' };

const DEFAULT_URL = 'http://127.0.0.1:8880';
const SENTENCE_TERMINAL = /\p{Sentence_Terminal}/u;
const CLOSE_PUNCTUATION = /\p{Close_Punctuation}/u;

// Split after Unicode Sentence_Terminal, keeping trailing closing punctuation.
export function splitSentences(text) {
  const input = String(text ?? '').trim();
  const out = [];
  if (!input) return out;

  let start = 0;
  let i = 0;
  while (i < input.length) {
    const char = String.fromCodePoint(input.codePointAt(i));
    if (SENTENCE_TERMINAL.test(char)) {
      let end = i + char.length;
      while (end < input.length) {
        const next = String.fromCodePoint(input.codePointAt(end));
        if (!CLOSE_PUNCTUATION.test(next)) break;
        end += next.length;
      }
      push(out, input.slice(start, end));
      start = end;
      i = end;
      continue;
    }
    i += char.length;
  }
  push(out, input.slice(start));
  return out;
}

function push(out, sentence) {
  const trimmed = sentence.trim();
  if (trimmed) out.push(trimmed);
}

function baseUrl(value) {
  return String(value || DEFAULT_URL).trim().replace(/\/+$/, '');
}

function speechUrl(base) {
  return /\/v1$/i.test(base) ? `${base}/audio/speech` : `${base}/v1/audio/speech`;
}

// Kokoro puts an ID3v2 tag at the start of every response; only the first is
// valid when clips are joined into one independently decodable mp3 chunk.
function withoutId3(data) {
  if (data.length < 10 || data[0] !== 0x49 || data[1] !== 0x44 || data[2] !== 0x33) {
    return data;
  }
  const size =
    ((data[6] & 0x7f) << 21) |
    ((data[7] & 0x7f) << 14) |
    ((data[8] & 0x7f) << 7) |
    (data[9] & 0x7f);
  let offset = 10 + size;
  if (data[5] & 0x10) offset += 10;
  return data.subarray(Math.min(offset, data.length));
}

function concat(parts) {
  const total = parts.reduce((sum, part) => sum + part.length, 0);
  const data = new Uint8Array(total);
  let offset = 0;
  for (const part of parts) {
    data.set(part, offset);
    offset += part.length;
  }
  return data;
}

export function create({
  kokoroUrl = '',
  voices = voiceConfig.kokoro,
  fetch: fetchImpl = globalThis.fetch,
  warn = (line) => console.warn(line),
  model = 'kokoro',
} = {}) {
  const base = baseUrl(kokoroUrl);
  const endpoint = speechUrl(base);

  function voicesFor(language) {
    if (!language) throw new Error('Kokoro TTS needs a target language (direction.tgt).');
    const exact = voices?.[language];
    if (Array.isArray(exact) && exact.length) return exact;
    const baseTag = String(language).split('-')[0];
    const short = voices?.[baseTag];
    if (Array.isArray(short) && short.length) return short;
    throw new Error(`Kokoro TTS has no voice configured for target language "${language}".`);
  }

  async function post(input, voice) {
    let response;
    try {
      response = await fetchImpl(endpoint, {
        method: 'POST',
        headers: { 'content-type': 'application/json' },
        body: JSON.stringify({
          model,
          input,
          voice,
          response_format: 'mp3',
          stream: true,
        }),
      });
    } catch (cause) {
      throw new Error(
        `Kokoro TTS is unreachable at ${base}: ${cause?.message ?? cause}. ` +
          'Set TTS_PROVIDER=system or mock to run without Kokoro.',
        { cause },
      );
    }

    if (!response.ok) {
      const detail = await response.text().catch(() => '');
      const suffix = detail ? `: ${detail.trim().slice(0, 200)}` : '';
      const error = new Error(
        `Kokoro TTS at ${base} returned HTTP ${response.status}${suffix}. ` +
          'Set TTS_PROVIDER=system or mock to run without Kokoro.',
      );
      error.status = response.status;
      throw error;
    }

    return new Uint8Array(await response.arrayBuffer());
  }

  return {
    name: 'kokoro',
    async synthesize({ text, direction }) {
      const candidates = voicesFor(direction?.tgt);
      const sentences = splitSentences(text);
      if (!sentences.length) return { format: 'mp3', data: new Uint8Array(0) };

      let voice = 0;
      let first = true;
      const parts = [];
      for (const sentence of sentences) {
        for (;;) {
          try {
            const clip = await post(sentence, candidates[voice]);
            parts.push(first ? clip : withoutId3(clip));
            first = false;
            break;
          } catch (error) {
            const retryable = error.status === 400 || error.status === 404;
            if (!retryable || voice + 1 >= candidates.length) throw error;
            warn(
              `Kokoro voice ${candidates[voice]} is unavailable; ` +
                `falling back to ${candidates[voice + 1]}`,
            );
            voice += 1;
          }
        }
      }
      return { format: 'mp3', data: concat(parts) };
    },
  };
}
