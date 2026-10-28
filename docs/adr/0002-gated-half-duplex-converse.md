# Converse mode is gated half-duplex with explicit turns, not full-duplex AEC

The original spec kept the microphone streaming while the phone speaker plays translated speech, relying on browser echo cancellation. Research showed this does not hold on phones. Android WebView/Chrome AEC references only WebRTC playout, so neither Web Audio nor system TTS is cancelled. iOS WebKit behaves the same way until device-tested otherwise. No mobile page can route one stream to the speaker and another to earphones at the same time.

So Converse mode works as follows:

1. **Explicit turns.** The user taps "me" / "them"; audio goes only to the current turn's Direction. There is no language auto-detection.
2. **Spoken output for "my" Direction only.** Its translation is spoken through the loudspeaker. "Their" Direction shows subtitles only.
3. **Mic gating during playback.** The mic track stays live, but no frames are sent while own speech plays, plus a ~500 ms tail.
4. **Echo filter as a second line of defence.** A text-similarity check (> 0.8) still drops Echo Suspects.

## Considered options

- **Full-duplex with AEC.** This is what the spec described. It is not achievable on Android or iOS WebViews today.
- **Loopback through RTCPeerConnection so that AEC sees the playback.** It works on desktop Chromium but is unproven on phones and on iOS 26+. Revisit once device tests prove it.
- **Constrained language auto-detect.** It is only possible with some ASR providers, and running two parallel streams doubles compute.
