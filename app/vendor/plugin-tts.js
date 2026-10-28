import { invoke } from './api/core.js';
import { listen } from './api/event.js';

/** Maximum `text` length in UTF-8 bytes. Longer input is rejected with `TEXT_TOO_LONG`. */
const MAX_TEXT_LENGTH = 10_000;
/** Accepted `rate`, where 1.0 is the platform's normal speed. */
const RATE_RANGE = [0.1, 4.0];
/** Accepted `pitch`, where 1.0 is the platform's normal pitch. */
const PITCH_RANGE = [0.5, 2.0];
/** Accepted `volume`. */
const VOLUME_RANGE = [0.0, 1.0];
const DEFAULT_QUEUE_MODE = "flush";
function isTtsError(error) {
    return (typeof error === "object" &&
        error !== null &&
        "code" in error &&
        "message" in error);
}

/**
 * Native text-to-speech for Tauri 2.
 *
 * @module
 */
/**
 * `JSON.stringify` turns `NaN` and `Infinity` into `null`, which would read as "unset". A
 * slider that momentarily reads `NaN` should fall back to the documented default here,
 * where the intent is still visible.
 */
function finiteOr(value, fallback) {
    return typeof value === "number" && Number.isFinite(value) ? value : fallback;
}
let relayRegistration = null;
/**
 * Registers the native event relay exactly once per page load. On mobile this hands a
 * channel to the native plugin; on desktop it resolves immediately.
 */
function ensureRelayRegistered() {
    relayRegistration ??= invoke("plugin:tts|register_listener").catch((error) => {
        relayRegistration = null;
        throw error;
    });
    return relayRegistration;
}
/**
 * Speaks `text`, resolving once the engine accepts the utterance.
 *
 * Check `warning` on the result: an uninstalled voice still speaks, using the system
 * default. Use `utteranceId` to match this call against its lifecycle events.
 *
 * @throws A {@link TtsError} if validation fails or the engine refuses the request.
 *
 * @example
 * ```typescript
 * await speak({ text: "Hello, world!" });
 * await speak({ text: "Olá!", language: "pt-BR", rate: 1.2 });
 *
 * const { warning } = await speak({ text: "Hi", voiceId: savedId });
 * if (warning) console.warn(warning);
 * ```
 */
async function speak(options) {
    return invoke("plugin:tts|speak", {
        payload: {
            text: options.text,
            language: options.language ?? null,
            voiceId: options.voiceId ?? null,
            rate: finiteOr(options.rate, 1.0),
            pitch: finiteOr(options.pitch, 1.0),
            volume: finiteOr(options.volume, 1.0),
            queueMode: options.queueMode ?? DEFAULT_QUEUE_MODE,
        },
    });
}
/** Stops the current utterance and clears anything queued behind it. */
async function stop() {
    await invoke("plugin:tts|stop");
}
/**
 * Lists installed voices, optionally filtered by locale prefix: `"pt"` matches both `pt-BR`
 * and `pt-PT`.
 */
async function getVoices(language) {
    const { voices } = await invoke("plugin:tts|get_voices", {
        payload: { language: language ?? null },
    });
    return voices;
}
/**
 * Speaks a short sample in `voiceId`, at default rate, pitch and volume.
 *
 * Resolves with `success: false` when the voice is not installed. Unlike {@link speak} it
 * does not fall back to another voice, since hearing this one is the point of the call.
 */
async function previewVoice(options) {
    return invoke("plugin:tts|preview_voice", {
        payload: {
            voiceId: options.voiceId,
            text: options.text ?? null,
        },
    });
}
async function isSpeaking() {
    const { speaking } = await invoke("plugin:tts|is_speaking");
    return speaking;
}
/**
 * Reports whether the engine is ready. Mobile initializes asynchronously, so poll this
 * before calling {@link getVoices} on a cold start.
 *
 * @example
 * ```typescript
 * async function waitForTts(attempts = 10): Promise<boolean> {
 *   for (let i = 0; i < attempts; i++) {
 *     const { initialized, voiceCount } = await isInitialized();
 *     if (initialized && voiceCount > 0) return true;
 *     await new Promise((resolve) => setTimeout(resolve, 500));
 *   }
 *   return false;
 * }
 * ```
 */
async function isInitialized() {
    return invoke("plugin:tts|is_initialized");
}
/**
 * Pauses the current utterance. iOS only; desktop and Android resolve with
 * `success: false` and a `reason`, since neither engine can pause.
 */
async function pauseSpeaking() {
    return invoke("plugin:tts|pause_speaking");
}
/** Resumes a paused utterance. iOS only — see {@link pauseSpeaking}. */
async function resumeSpeaking() {
    return invoke("plugin:tts|resume_speaking");
}
/**
 * Controls what happens when the app backgrounds or the screen locks.
 *
 * With `false`, speech stops and `speech:backgroundPause` is emitted. Mobile only; desktop
 * resolves without doing anything.
 */
async function setBackgroundBehavior(options) {
    await invoke("plugin:tts|set_background_behavior", {
        payload: { continueInBackground: options.continueInBackground },
    });
}
/**
 * Subscribes to a lifecycle event. Call the returned function to stop listening.
 *
 * @example
 * ```typescript
 * const unlisten = await onSpeechEvent("speech:finish", (event) => {
 *   console.log("finished", event.id);
 * });
 * ```
 */
async function onSpeechEvent(eventType, handler) {
    await ensureRelayRegistered();
    return listen(`tts://${eventType}`, (event) => handler(event.payload));
}

export { DEFAULT_QUEUE_MODE, MAX_TEXT_LENGTH, PITCH_RANGE, RATE_RANGE, VOLUME_RANGE, getVoices, isInitialized, isSpeaking, isTtsError, onSpeechEvent, pauseSpeaking, previewVoice, resumeSpeaking, setBackgroundBehavior, speak, stop };
