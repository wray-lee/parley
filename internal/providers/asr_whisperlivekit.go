// WhisperLiveKit v0.2.26 local ASR, ported from
// core/providers/asr-whisperlivekit.js. PCM16 mono 16 kHz goes up over /asr;
// the server answers with full-mode JSON. `lines` is resent in full on every
// update. Its last entry is still being extended, so a line is committed (and
// emitted as Stable Prefix exactly once) only after a later line or silence
// follows it; the interim tail is a display-only Partial, as is
// `buffer_transcription` when present. End of stream is an empty binary frame,
// then the server answers ready_to_stop and the last line is committed.
// `url` is config data (WHISPERLIVEKIT_URL); no language is hardcoded here.

package providers

import (
	"encoding/json"
	"math"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/coder/websocket"

	"github.com/wray-lee/parley/internal/core"
	"github.com/wray-lee/parley/internal/protocol"
)

const (
	whisperLiveKitDefaultURL = "ws://127.0.0.1:8000"
	wlkFallbackStack         = "Check WHISPERLIVEKIT_URL and that the local stack is up " +
		"(docker compose -f deploy/compose.yaml -f deploy/compose.local.yaml --profile local up -d), " +
		fallbackMock
	wlkFallbackLogs = "Check the WhisperLiveKit server logs, " + fallbackMock
)

// WhisperLiveKitOptions are the config data of one local connection.
type WhisperLiveKitOptions struct {
	URL         string
	StopTimeout time.Duration // default 5 s for ready_to_stop after end-of-audio
	dial        wsDial
}

type wlkLine struct {
	Text    string   `json:"text"`
	Start   any      `json:"start"`
	End     any      `json:"end"`
	Speaker *float64 `json:"speaker"`
}

type wlkUpdate struct {
	Error               json.RawMessage `json:"error"`
	Lines               *[]wlkLine      `json:"lines"`
	BufferTranscription *string         `json:"buffer_transcription"`
}

type whisperLiveKitASR struct {
	socketASR
	opts          WhisperLiveKitOptions
	stopped       bool
	pending       [][]byte
	lastAudioMS   int64
	lastPartial   string
	lastLineEndMS int64
	latestLines   []wlkLine
	committed     []bool
	stopTimer     *time.Timer
}

// NewWhisperLiveKitASR builds the local adapter.
func NewWhisperLiveKitASR(options WhisperLiveKitOptions) *whisperLiveKitASR {
	if options.URL == "" {
		options.URL = whisperLiveKitDefaultURL
	}
	if options.StopTimeout <= 0 {
		options.StopTimeout = 5 * time.Second
	}
	return &whisperLiveKitASR{socketASR: newSocketASR(options.dial), opts: options}
}

func (a *whisperLiveKitASR) Name() string      { return "whisperlivekit" }
func (a *whisperLiveKitASR) AcceptsLive() bool { return true }
func (a *whisperLiveKitASR) Revisable() bool   { return false }

func (a *whisperLiveKitASR) Open(direction protocol.Direction) {
	ctx, mine := a.begin()
	a.mu.Lock()
	a.stopped = false
	a.pending = nil
	a.lastAudioMS = 0
	a.lastPartial = ""
	a.lastLineEndMS = 0
	a.latestLines = nil
	a.committed = nil
	a.stopTimer = nil
	if direction.Src == "" {
		a.raiseLocked(asrError("protocol", "whisperlivekit ASR needs the Direction source language", fallbackMock))
		a.mu.Unlock()
		return
	}
	target, err := whisperLiveKitEndpoint(a.opts.URL, direction.Src)
	if err != nil {
		a.raiseLocked(asrError("unreachable", "WhisperLiveKit URL is invalid: "+err.Error(), wlkFallbackStack))
		a.mu.Unlock()
		return
	}
	a.mu.Unlock()
	go a.run(ctx, mine, target, nil, nil, nil, true, a.handleLocked,
		whisperLiveKitDialError, whisperLiveKitClosedError, whisperLiveKitSendError)
}

func whisperLiveKitEndpoint(base, src string) (string, error) {
	target, err := url.Parse(base)
	if err != nil {
		return "", err
	}
	switch target.Scheme {
	case "http":
		target.Scheme = "ws"
	case "https":
		target.Scheme = "wss"
	}
	pathname := strings.TrimRight(target.Path, "/")
	if !strings.HasSuffix(pathname, "/asr") {
		pathname += "/asr"
	}
	target.Path = pathname
	query := target.Query()
	query.Set("language", src)
	query.Set("mode", "full")
	target.RawQuery = query.Encode()
	return target.String(), nil
}

func whisperLiveKitDialError(status int, err error) error {
	if status != 0 {
		return whisperLiveKitStatusError(status)
	}
	return asrError("unreachable", "WhisperLiveKit connection failed", wlkFallbackStack)
}

func whisperLiveKitClosedError(code string) error {
	return asrError("unreachable", "WhisperLiveKit connection closed (code "+code+")", wlkFallbackStack)
}

func whisperLiveKitSendError(err error) error {
	return asrError("unreachable", "WhisperLiveKit send failed: "+err.Error(), wlkFallbackLogs)
}

func whisperLiveKitStatusError(status int) error {
	if status == 404 {
		return asrError("unreachable", "WhisperLiveKit has no /asr endpoint (HTTP 404)", wlkFallbackStack)
	}
	return asrError("unreachable", "WhisperLiveKit handshake failed (HTTP "+strconv.Itoa(status)+")", wlkFallbackStack)
}

func (a *whisperLiveKitASR) handleLocked(data []byte) {
	var message map[string]any
	if err := json.Unmarshal(data, &message); err != nil {
		a.failLocked(asrError("protocol", "WhisperLiveKit sent a frame that is not JSON", wlkFallbackLogs))
		return
	}
	if message == nil {
		return
	}
	if message["type"] == "config" {
		if use, ok := message["useAudioWorklet"].(bool); ok && !use {
			a.failLocked(asrError("config", "WhisperLiveKit is not decoding raw PCM (useAudioWorklet=false)",
				"Start WhisperLiveKit with --pcm-input (the local Compose profile does), "+fallbackMock))
		}
		return
	}
	if message["type"] == "ready_to_stop" {
		// Flush updates that arrived before the stop, then the stream is final.
		queued := a.pending
		a.pending = nil
		for _, raw := range queued {
			a.handleUpdateLocked(raw)
		}
		a.commitLastLineLocked()
		a.closeSocketLocked()
		return
	}
	a.pending = append(a.pending, data)
}

func (a *whisperLiveKitASR) handleUpdateLocked(raw []byte) {
	var update wlkUpdate
	if err := json.Unmarshal(raw, &update); err != nil {
		return
	}
	if len(update.Error) > 0 {
		var failure any
		_ = json.Unmarshal(update.Error, &failure)
		if truthy(failure) {
			a.failLocked(asrError("protocol", "WhisperLiveKit error: "+stringify(failure), wlkFallbackLogs))
			return
		}
	}

	lines := []wlkLine{}
	if update.Lines != nil {
		lines = *update.Lines
		a.latestLines = lines
	}
	for index, line := range lines {
		if end, ok := parseWLKTime(line.End); ok && end > a.lastLineEndMS {
			a.lastLineEndMS = end
		}
		// Only a line followed by another line is committed; the last entry is
		// still being extended and must not reach the Stable Prefix yet.
		if index >= len(lines)-1 || a.lineCommitted(index) {
			continue
		}
		text := strings.TrimSpace(line.Text)
		if (line.Speaker != nil && *line.Speaker == -2) || text == "" {
			a.setCommitted(index, true)
			continue
		}
		a.emitLineLocked(index, line, text)
	}

	var last *wlkLine
	if len(lines) > 0 {
		last = &lines[len(lines)-1]
	}
	lastText := ""
	if last != nil && !(last.Speaker != nil && *last.Speaker == -2) {
		lastText = strings.TrimSpace(last.Text)
	}
	buffer := ""
	if update.BufferTranscription != nil {
		buffer = strings.TrimSpace(*update.BufferTranscription)
	}
	partial := buffer
	if partial == "" {
		partial = lastText
	}
	if partial != "" && partial != a.lastPartial {
		start, end := a.lastLineEndMS, maxInt64(a.lastLineEndMS, a.lastAudioMS)
		if buffer == "" && last != nil {
			if at, ok := parseWLKTime(last.Start); ok {
				start = at
			}
			if at, ok := parseWLKTime(last.End); ok {
				end = at
			}
		}
		a.deliverLocked(core.ASREvent{Type: "partial", Text: partial, StartMS: start, EndMS: end})
	}
	a.lastPartial = partial
}

func (a *whisperLiveKitASR) emitLineLocked(index int, line wlkLine, text string) {
	a.setCommitted(index, true)
	start, end := a.lastLineEndMS, a.lastAudioMS
	if at, ok := parseWLKTime(line.Start); ok {
		start = at
	}
	if at, ok := parseWLKTime(line.End); ok {
		end = at
	}
	a.deliverLocked(core.ASREvent{Type: "stable", Text: text, StartMS: start, EndMS: end})
}

func (a *whisperLiveKitASR) commitLastLineLocked() {
	index := len(a.latestLines) - 1
	if index < 0 || a.lineCommitted(index) {
		return
	}
	line := a.latestLines[index]
	text := strings.TrimSpace(line.Text)
	if (line.Speaker != nil && *line.Speaker == -2) || text == "" {
		return
	}
	a.emitLineLocked(index, line, text)
}

func (a *whisperLiveKitASR) lineCommitted(index int) bool {
	return index < len(a.committed) && a.committed[index]
}

func (a *whisperLiveKitASR) setCommitted(index int, value bool) {
	for len(a.committed) <= index {
		a.committed = append(a.committed, false)
	}
	a.committed[index] = value
}

func (a *whisperLiveKitASR) closeSocketLocked() {
	a.stopped = true
	a.closing = true
	a.abortPendingDialLocked()
	if a.stopTimer != nil {
		a.stopTimer.Stop()
		a.stopTimer = nil
	}
	if a.conn != nil {
		_ = a.conn.CloseNow()
	}
}

func (a *whisperLiveKitASR) Push(pcm []int16, audioMS int64) ([]core.ASREvent, error) {
	a.mu.Lock()
	a.lastAudioMS = audioMS
	a.mu.Unlock()
	a.pushAudio(pcm, func() bool { return a.opened }, whisperLiveKitSendError)
	a.mu.Lock()
	if !a.stopped {
		queued := a.pending
		a.pending = nil
		for _, raw := range queued {
			a.handleUpdateLocked(raw)
		}
	}
	a.mu.Unlock()
	return a.take()
}

func (a *whisperLiveKitASR) Close() {
	a.mu.Lock()
	if a.closing {
		a.mu.Unlock()
		return
	}
	a.pendingAudio = nil
	conn, opened, stopped := a.conn, a.opened, a.stopped
	if conn != nil && opened && !stopped {
		ctx, mine := a.ctx, a.generation
		a.mu.Unlock()
		if err := conn.Write(ctx, websocket.MessageBinary, []byte{}); err != nil {
			a.mu.Lock()
			if mine == a.generation {
				a.failLocked(whisperLiveKitSendError(err))
			}
			a.mu.Unlock()
		}
		a.mu.Lock()
		a.stopTimer = time.AfterFunc(a.opts.StopTimeout, func() {
			a.mu.Lock()
			a.closeSocketLocked()
			a.mu.Unlock()
		})
		a.mu.Unlock()
	} else {
		a.closeSocketLocked()
		a.mu.Unlock()
	}
	a.mu.Lock()
	a.closing = true
	a.mu.Unlock()
}

func (a *whisperLiveKitASR) receive(data []byte) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.handleLocked(data)
}

var wlkTimePattern = regexp.MustCompile(`^(\d+):(\d{2}):(\d{2}(?:\.\d+)?)$`)

// parseWLKTime parses WhisperLiveKit's "H:MM:SS.cc" timed_objects.format_time().
func parseWLKTime(value any) (int64, bool) {
	text, ok := value.(string)
	if !ok {
		return 0, false
	}
	match := wlkTimePattern.FindStringSubmatch(text)
	if match == nil {
		return 0, false
	}
	hours, _ := strconv.Atoi(match[1])
	minutes, _ := strconv.Atoi(match[2])
	seconds, _ := strconv.ParseFloat(match[3], 64)
	return int64(math.Round(((float64(hours)*60+float64(minutes))*60 + seconds) * 1000)), true
}

func truthy(value any) bool {
	switch typed := value.(type) {
	case nil:
		return false
	case bool:
		return typed
	case float64:
		return typed != 0
	case string:
		return typed != ""
	}
	return true
}

func maxInt64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}
