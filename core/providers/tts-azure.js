// Azure Speech REST TTS (F0 free tier): POST SSML, receive an mp3 body.
// Voices are per-language data in config/voices.json; AZURE_SPEECH_KEY and
// AZURE_SPEECH_REGION reach this adapter through the Server Host config.

import voiceConfig from '../../config/voices.json' with { type: 'json' };

const OUTPUT_FORMAT = 'audio-24khz-48kbitrate-mono-mp3';
const REGION = /^[a-z0-9-]+$/;
// shortcut: one whole-request timeout, add a first-audio timeout if Azure stalls.
const TIMEOUT_MS = 30_000;
const FALLBACK = 'Set TTS_PROVIDER=system or mock to run without Azure.';

const escapeXml = (value) =>
  String(value).replace(
    /[&<>"']/g,
    (char) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&apos;' })[char],
  );

function voiceLanguage(voice) {
  const match = /^([a-z]{2,3}-[A-Z]{2})-/.exec(voice);
  return match ? match[1] : '';
}

export function create({
  azureKey = '',
  azureRegion = '',
  voices = voiceConfig.azure,
  fetch: fetchImpl = globalThis.fetch,
  warn = (line) => console.warn(line),
  timeoutMs = TIMEOUT_MS,
} = {}) {
  const key = String(azureKey).trim();
  const region = String(azureRegion).trim();
  if (!key) throw new Error(`Azure TTS requires AZURE_SPEECH_KEY. ${FALLBACK}`);
  if (!REGION.test(region)) {
    throw new Error(
      `Azure TTS requires AZURE_SPEECH_REGION (for example eastus). ${FALLBACK}`,
    );
  }
  const endpoint = `https://${region}.tts.speech.microsoft.com/cognitiveservices/v1`;

  function voiceFor(language) {
    if (!language) throw new Error(`Azure TTS needs a target language (direction.tgt). ${FALLBACK}`);
    const exact = voices?.[language];
    if (Array.isArray(exact) && exact.length) return exact[0];
    const base = String(language).split('-')[0];
    const short = voices?.[base];
    if (Array.isArray(short) && short.length) {
      warn(`Azure TTS has no voice configured for "${language}"; using "${base}".`);
      return short[0];
    }
    throw new Error(
      `Azure TTS has no voice configured for target language "${language}". ${FALLBACK}`,
    );
  }

  return {
    name: 'azure',
    async synthesize({ text, direction }) {
      const voice = voiceFor(direction?.tgt);
      const lang = voiceLanguage(voice) || direction.tgt;
      const ssml =
        `<speak version='1.0' xmlns='http://www.w3.org/2001/10/synthesis' xml:lang='${escapeXml(lang)}'>` +
        `<voice name='${escapeXml(voice)}'>${escapeXml(text)}</voice></speak>`;

      let response;
      try {
        response = await fetchImpl(endpoint, {
          method: 'POST',
          headers: {
            'Ocp-Apim-Subscription-Key': key,
            'Content-Type': 'application/ssml+xml',
            'X-Microsoft-OutputFormat': OUTPUT_FORMAT,
            'User-Agent': 'parley',
          },
          body: ssml,
          signal: AbortSignal.timeout(timeoutMs),
        });
      } catch (cause) {
        throw new Error(
          `Azure TTS is unreachable at ${endpoint}: ${cause?.message ?? cause}. ${FALLBACK}`,
          { cause },
        );
      }

      if (!response.ok) {
        const detail = await response.text().catch(() => '');
        const suffix = detail ? `: ${detail.trim().slice(0, 200)}` : '';
        if (response.status === 401 || response.status === 403) {
          throw new Error(
            `Azure TTS rejected the key or region (HTTP ${response.status}). ` +
              `Check AZURE_SPEECH_KEY and AZURE_SPEECH_REGION. ${FALLBACK}`,
          );
        }
        if (response.status === 429) {
          throw new Error(`Azure TTS is rate limited (HTTP 429). Wait and retry. ${FALLBACK}`);
        }
        throw new Error(`Azure TTS failed (HTTP ${response.status})${suffix}. ${FALLBACK}`);
      }

      const data = new Uint8Array(await response.arrayBuffer());
      if (!data.length) throw new Error(`Azure TTS returned no audio. ${FALLBACK}`);
      return { format: 'mp3', data };
    },
  };
}
