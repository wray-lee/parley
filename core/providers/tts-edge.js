// Microsoft Edge read-aloud: keyless, unofficial and unstable. This is an
// independent Node implementation of the publicly observable WebSocket
// protocol; no code was copied from the LGPL edge-tts project. Protocol notes
// and sources: .scratch/parley-v1/edge-tts-implementation-notes.md.
//
// Frames: a JSON speech.config text frame, an SSML text frame, then binary
// frames of [2-byte big-endian header length][header][mp3 payload]; a text
// frame with Path:turn.end closes the response.
//
// shortcut: the Sec-MS-GEC version pin ages with Edge, update it when the
// handshake starts returning 403.

import { createHash, randomUUID } from 'node:crypto';
import { WebSocket } from 'ws';

import voiceConfig from '../../config/voices.json' with { type: 'json' };

const TRUSTED_CLIENT_TOKEN = '6A5AA1D4EAFF4E9FB37E23D68491D6F4';
const ENDPOINT =
  'wss://speech.platform.bing.com/consumer/speech/synthesize/readaloud/edge/v1';
const OUTPUT_FORMAT = 'audio-24khz-48kbitrate-mono-mp3';
const SEC_MS_GEC_VERSION = '2-143.0.3650.96';
const COPYRIGHT = 'Copyright (C) Microsoft Corporation. All rights reserved.';
const TIMEOUT_MS = 30_000;
const FALLBACK = 'Set TTS_PROVIDER=kokoro or system.';

const HEADERS = {
  Pragma: 'no-cache',
  'Cache-Control': 'no-cache',
  Origin: 'chrome-extension://jdiccldimpdaibmpdkjnbmckianbfold',
  'User-Agent':
    'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) ' +
    'Chrome/143.0.0.0 Safari/537.36 Edg/143.0.0.0',
  'Accept-Encoding': 'gzip, deflate, br',
  'Accept-Language': 'en-US,en;q=0.9',
};

// Sec-MS-GEC v2 is CS64(copyright + ticks + token) + "-" + SHA-256(same input).
const SWAP = [
  { word: 0xef0569fb, sum: 0x76f50f34, product: 0xc3efea97 },
  { word: 0xc31713db, sum: 0x1881ce3c, product: 0x35bd1ec9 },
];
const MIX = [
  [0xcf98b111, 0x87085b9f, 0x12ceb96d, 0x257e1d83],
  [0xa27416f5, 0xd38396ff, 0x7c932b89, 0xbfa49f69],
];

const swap16 = (value) => ((value << 16) | (value >>> 16)) >>> 0;

function swapTerm(word, key, multipliers) {
  const sum = Math.imul(word, key) + Math.imul(swap16(word), multipliers.word);
  const product = Math.imul(swap16(sum), multipliers.sum) >>> 0;
  return (Math.imul(swap16(product), multipliers.product) + product) >>> 0;
}

function mixWord(value, multipliers) {
  for (const multiplier of multipliers) value = Math.imul(swap16(value), multiplier) >>> 0;
  return value;
}

function cs64(message, digest) {
  const keys = [
    (digest.readUInt32LE(0) | 1) >>> 0,
    (digest.readUInt32LE(4) | 1) >>> 0,
  ];
  const padded = Buffer.alloc((message.length + 7) & ~7);
  message.copy(padded);

  let swapSum = 0;
  let lastSwap = 0;
  let mix = 0;
  let mixSum = 0;
  for (let offset = 0; offset < padded.length; offset += 8) {
    const words = [padded.readUInt32LE(offset), padded.readUInt32LE(offset + 4)];
    swapSum =
      (swapSum +
        swapTerm(words[0], keys[0], SWAP[0]) +
        swapTerm(words[1], keys[1], SWAP[1])) >>>
      0;
    lastSwap = swapTerm(words[1], keys[1], SWAP[1]);
    for (let i = 0; i < 2; i++) {
      mix = mixWord(Math.imul(mix + words[i], keys[i]), MIX[i]);
      mixSum = (mixSum + mix) >>> 0;
    }
  }

  const out = Buffer.alloc(8);
  out.writeUInt32LE((lastSwap ^ mix) >>> 0, 0);
  out.writeUInt32LE((swapSum ^ mixSum) >>> 0, 4);
  return out;
}

export function secMsGec(nowMs) {
  const seconds = Math.floor(nowMs / 1000);
  const rounded = seconds - (seconds % 300);
  const ticks = BigInt(rounded + 11644473600) * 10000000n;
  const message = Buffer.from(`${COPYRIGHT}${ticks}${TRUSTED_CLIENT_TOKEN}`, 'utf8');
  const digest = createHash('sha256').update(message).digest();
  return `${cs64(message, digest).toString('hex').toUpperCase()}-${digest
    .toString('hex')
    .toUpperCase()}`;
}

const escapeXml = (value) =>
  String(value).replace(
    /[&<>"']/g,
    (char) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&apos;' })[char],
  );

function voiceLanguage(voice) {
  const match = /^([a-z]{2,3}-[A-Z]{2})-/.exec(voice);
  return match ? match[1] : '';
}

function synthesisUrl(nowMs) {
  const query = [
    `TrustedClientToken=${TRUSTED_CLIENT_TOKEN}`,
    `Sec-MS-GEC=${secMsGec(nowMs)}`,
    `Sec-MS-GEC-Version=${SEC_MS_GEC_VERSION}`,
    `ConnectionId=${randomUUID().replace(/-/g, '')}`,
  ].join('&');
  return `${ENDPOINT}?${query}`;
}

function configFrame(at) {
  const payload = JSON.stringify({
    context: {
      synthesis: {
        audio: {
          metadataoptions: { sentenceBoundaryEnabled: false, wordBoundaryEnabled: false },
          outputFormat: OUTPUT_FORMAT,
        },
      },
    },
  });
  return (
    `X-Timestamp:${at.toISOString()}\r\nContent-Type:application/json; charset=utf-8\r\n` +
    `Path:speech.config\r\n\r\n${payload}`
  );
}

function ssmlFrame({ text, voice, lang, at }) {
  const ssml =
    `<speak version='1.0' xmlns='http://www.w3.org/2001/10/synthesis' xml:lang='${escapeXml(lang)}'>` +
    `<voice name='${escapeXml(voice)}'>${escapeXml(text)}</voice></speak>`;
  return (
    `X-RequestId:${randomUUID().replace(/-/g, '')}\r\nContent-Type:application/ssml+xml\r\n` +
    `X-Timestamp:${at.toISOString()}\r\nPath:ssml\r\n\r\n${ssml}`
  );
}

function connect({ url, text, voice, lang, WebSocketImpl, now, timeoutMs }) {
  return new Promise((resolve, reject) => {
    const socket = new WebSocketImpl(url, { headers: HEADERS });
    const chunks = [];
    let settled = false;
    let timer;

    const fail = (message, cause) => {
      if (settled) return;
      settled = true;
      clearTimeout(timer);
      try {
        socket.terminate?.();
      } catch {}
      reject(new Error(message, cause ? { cause } : undefined));
    };

    timer = setTimeout(
      () => fail(`Edge TTS timed out after ${timeoutMs} ms. It is unofficial and unstable. ${FALLBACK}`),
      timeoutMs,
    );

    socket.on('open', () => {
      const at = new Date(now());
      socket.send(configFrame(at));
      socket.send(ssmlFrame({ text, voice, lang, at }));
    });

    socket.on('message', (data, isBinary) => {
      const buffer = Buffer.from(data);
      if (!isBinary) {
        if (buffer.toString().includes('Path:turn.end')) {
          const audio = Buffer.concat(chunks);
          if (!audio.length) {
            fail(`Edge TTS returned no audio. ${FALLBACK}`);
            return;
          }
          if (settled) return;
          settled = true;
          clearTimeout(timer);
          try {
            socket.terminate?.();
          } catch {}
          resolve({ format: 'mp3', data: new Uint8Array(audio) });
        }
        return;
      }
      if (buffer.length < 2) return;
      const headerLength = buffer.readUInt16BE(0);
      if (buffer.length < 2 + headerLength) return;
      const header = buffer.subarray(2, 2 + headerLength).toString();
      if (header.split('\r\n').includes('Path:audio')) {
        chunks.push(buffer.subarray(2 + headerLength));
      }
    });

    socket.on('unexpected-response', (_request, response) => {
      fail(
        `Edge TTS handshake was rejected (HTTP ${response.statusCode}). ` +
          `Microsoft changes this unofficial service without notice. ${FALLBACK}`,
      );
    });

    socket.on('error', (error) => {
      fail(
        `Edge TTS connection failed: ${error?.message ?? error}. ` +
          `It is unofficial and unstable. ${FALLBACK}`,
        error,
      );
    });

    socket.on('close', (code) => {
      if (!settled) {
        fail(
          `Edge TTS closed the connection before finishing (code ${code}). ` +
            `It is unofficial and unstable. ${FALLBACK}`,
        );
      }
    });
  });
}

export function create({
  voices = voiceConfig.edge,
  WebSocket: WebSocketImpl = WebSocket,
  now = Date.now,
  timeoutMs = TIMEOUT_MS,
  warn = (line) => console.warn(line),
} = {}) {
  function voiceFor(language) {
    if (!language) throw new Error(`Edge TTS needs a target language (direction.tgt). ${FALLBACK}`);
    const exact = voices?.[language];
    if (Array.isArray(exact) && exact.length) return exact[0];
    const base = String(language).split('-')[0];
    const short = voices?.[base];
    if (Array.isArray(short) && short.length) {
      warn(`Edge TTS has no voice configured for "${language}"; using "${base}".`);
      return short[0];
    }
    throw new Error(
      `Edge TTS has no voice configured for target language "${language}". ${FALLBACK}`,
    );
  }

  return {
    name: 'edge',
    unofficial: true,
    async synthesize({ text, direction }) {
      const voice = voiceFor(direction?.tgt);
      const lang = voiceLanguage(voice) || direction.tgt;
      return connect({
        url: synthesisUrl(now()),
        text,
        voice,
        lang,
        WebSocketImpl,
        now,
        timeoutMs,
      });
    },
  };
}
