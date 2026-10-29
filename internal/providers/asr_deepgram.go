// Deepgram Nova-3 live streaming ASR, ported from core/providers/asr-deepgram.js.
// PCM16 mono 16 kHz goes up, Results and UtteranceEnd frames come down. Stable
// Prefix is LocalAgreement-2 over consecutive interim hypotheses; is_final text
// is stable outright. Stable text is sliced from Deepgram's own transcript, so
// its spacing and punctuation are the provider's. The adapter emits both event
// kinds itself, so it must NOT report itself revisable.
// model/endpoint/auth are config data; no language is hardcoded here.

package providers

import (
	"encoding/json"
	"math"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/coder/websocket"

	"github.com/wray-lee/parley/internal/core"
	"github.com/wray-lee/parley/internal/protocol"
)

const (
	deepgramDefaultURL = "wss://api.deepgram.com/v1/listen"
	fallbackMock       = "or set ASR_PROVIDER=mock to run in Demo Mode."
)

// DeepgramOptions are the config data of one connection.
type DeepgramOptions struct {
	APIKey         string
	Model          string
	Endpointing    int
	UtteranceEndMS int
	URL            string
	Auth           string // "header" (default) or "subprotocol"
	dial           wsDial
}

type dgWord struct {
	Word           *string  `json:"word"`
	PunctuatedWord *string  `json:"punctuated_word"`
	Start          *float64 `json:"start"`
	End            *float64 `json:"end"`
}

type dgAlternative struct {
	Transcript string   `json:"transcript"`
	Words      []dgWord `json:"words"`
}

type dgMessage struct {
	Type     string `json:"type"`
	Start    *float64
	Duration *float64
	IsFinal  bool `json:"is_final"`
	Channel  struct {
		Alternatives []dgAlternative `json:"alternatives"`
	} `json:"channel"`
	ErrCode     any `json:"err_code"`
	ErrMsg      any `json:"err_msg"`
	Description any `json:"description"`
}

type deepgramASR struct {
	socketASR
	opts           DeepgramOptions
	previous       []dgWord
	emitted        int
	emittedChars   int
	utteranceStart *float64
}

// NewDeepgramASR builds the adapter; a missing key fails on Open like the
// JavaScript oracle, so the host can start and report the error to the client.
func NewDeepgramASR(options DeepgramOptions) *deepgramASR {
	if options.Model == "" {
		options.Model = "nova-3"
	}
	if options.Endpointing == 0 {
		options.Endpointing = 300
	}
	if options.UtteranceEndMS == 0 {
		options.UtteranceEndMS = 1000
	}
	if options.URL == "" {
		options.URL = deepgramDefaultURL
	}
	return &deepgramASR{socketASR: newSocketASR(options.dial), opts: options}
}

func (a *deepgramASR) Name() string      { return "deepgram" }
func (a *deepgramASR) AcceptsLive() bool { return true }
func (a *deepgramASR) Revisable() bool   { return false }

func (a *deepgramASR) Open(direction protocol.Direction) {
	ctx, mine := a.begin()
	a.mu.Lock()
	a.resetAgreementLocked()
	if direction.Src == "" {
		a.raiseLocked(asrError("protocol", "deepgram ASR needs the Direction source language", fallbackMock))
		a.mu.Unlock()
		return
	}
	if a.opts.APIKey == "" {
		a.raiseLocked(asrError("auth", "DEEPGRAM_API_KEY is not set",
			"Set DEEPGRAM_API_KEY, "+fallbackMock))
		a.mu.Unlock()
		return
	}
	target, err := deepgramTarget(a.opts, direction.Src)
	if err != nil {
		a.raiseLocked(asrError("unreachable", "Deepgram URL is invalid: "+err.Error(), fallbackMock))
		a.mu.Unlock()
		return
	}
	var header http.Header
	var subprotocols []string
	if a.opts.Auth == "subprotocol" {
		subprotocols = []string{"token", a.opts.APIKey}
	} else {
		header = http.Header{"Authorization": {"Token " + a.opts.APIKey}}
	}
	a.mu.Unlock()
	go a.run(ctx, mine, target, header, subprotocols, nil, true, a.handleLocked,
		deepgramDialError, deepgramClosedError, deepgramSendError)
}

func deepgramTarget(options DeepgramOptions, src string) (string, error) {
	target, err := url.Parse(options.URL)
	if err != nil {
		return "", err
	}
	query := target.Query()
	query.Set("model", options.Model)
	query.Set("language", src) // fixed per connection: no auto-detection
	query.Set("interim_results", "true")
	query.Set("endpointing", strconv.Itoa(options.Endpointing))
	query.Set("utterance_end_ms", strconv.Itoa(options.UtteranceEndMS))
	query.Set("encoding", "linear16")
	query.Set("sample_rate", "16000")
	query.Set("channels", "1")
	query.Set("punctuate", "true")
	query.Set("smart_format", "true")
	target.RawQuery = query.Encode()
	return target.String(), nil
}

func deepgramDialError(status int, err error) error {
	if status != 0 {
		return deepgramStatusError(status)
	}
	return asrError("unreachable", "Deepgram connection failed", fallbackMock)
}

func deepgramClosedError(code string) error {
	return asrError("unreachable", "Deepgram connection closed (code "+code+")", fallbackMock)
}

func deepgramSendError(err error) error {
	return asrError("unreachable", "Deepgram send failed: "+err.Error(), fallbackMock)
}

func deepgramStatusError(status int) error {
	switch status {
	case 401, 403:
		return asrError("auth", "Deepgram rejected the credentials (HTTP "+strconv.Itoa(status)+")",
			"Check DEEPGRAM_API_KEY, "+fallbackMock)
	case 402:
		return asrError("quota", "Deepgram has insufficient credit (HTTP 402)",
			"Deepgram credit is exhausted; top up, "+fallbackMock+" whisperlivekit is a local alternative.")
	case 429:
		return asrError("rate_limit", "Deepgram rate limit hit (HTTP 429)", "Wait and retry, "+fallbackMock)
	}
	return asrError("unreachable", "Deepgram handshake failed (HTTP "+strconv.Itoa(status)+")",
		"Check DEEPGRAM_URL and the network, "+fallbackMock)
}

var (
	dgAuthPattern  = regexp.MustCompile(`(?i)auth`)
	dgQuotaPattern = regexp.MustCompile(`(?i)payment|funds|quota`)
)

func deepgramInBandError(message *dgMessage) error {
	code := "unknown"
	for _, value := range []any{message.ErrCode, message.Description} {
		if value != nil {
			code = stringify(value)
			break
		}
	}
	detail := ""
	for _, value := range []any{message.ErrMsg, message.Description} {
		if value != nil {
			detail = stringify(value)
			break
		}
	}
	kind := "protocol"
	fallback := "Check the Deepgram model/language settings, " + fallbackMock
	if dgAuthPattern.MatchString(code) {
		kind = "auth"
		fallback = "Check DEEPGRAM_API_KEY, " + fallbackMock
	} else if dgQuotaPattern.MatchString(code) {
		kind = "quota"
	}
	return asrError(kind, strings.TrimSpace("Deepgram error "+code+": "+detail), fallback)
}

// stringify mirrors JavaScript String(value) for the JSON scalars Deepgram sends.
func stringify(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	case float64:
		return strconv.FormatFloat(typed, 'f', -1, 64)
	case bool:
		return strconv.FormatBool(typed)
	}
	return ""
}

func (a *deepgramASR) handleLocked(data []byte) {
	if a.failed {
		return
	}
	var message dgMessage
	if err := json.Unmarshal(data, &message); err != nil {
		a.raiseLocked(asrError("protocol", "Deepgram sent a frame that is not JSON", fallbackMock))
		return
	}
	switch message.Type {
	case "Results":
		a.handleResultsLocked(&message)
	case "Error":
		a.raiseLocked(deepgramInBandError(&message))
	case "UtteranceEnd":
		a.resetWordsLocked()
	}
}

func (a *deepgramASR) handleResultsLocked(message *dgMessage) {
	var alternative dgAlternative
	if len(message.Channel.Alternatives) > 0 {
		alternative = message.Channel.Alternatives[0]
	}
	words := alternative.Words
	transcript := alternative.Transcript
	if transcript == "" && len(words) == 0 {
		return
	}

	// Deepgram's `start` is absolute stream time: a later start is a new utterance.
	if message.Start != nil && (a.utteranceStart == nil || *message.Start > *a.utteranceStart) {
		a.resetAgreementLocked()
		start := *message.Start
		a.utteranceStart = &start
	}

	fallback := core.ASREvent{
		StartMS: milliseconds(message.Start, 0),
		EndMS:   milliseconds(sumStartDuration(message.Start, message.Duration), 0),
	}

	if message.IsFinal {
		// is_final text is stable outright. Emit the provider transcript's
		// not-yet-emitted tail, so a final that lands after UtteranceEnd or
		// CloseStream cannot repeat words interims already made stable.
		if a.emittedChars < len(transcript) {
			glue := ""
			event := deepgramSpan(dgFrom(words, a.emitted), fallback)
			event.Type, event.Text, event.Glue = "stable", transcript[a.emittedChars:], &glue
			a.deliverLocked(event)
			a.emittedChars = len(transcript)
		}
		a.resetWordsLocked()
		return
	}

	partial := deepgramSpan(words, fallback)
	partial.Type, partial.Text = "partial", transcript
	a.deliverLocked(partial)

	agreed := dgCommonWords(a.previous, words)
	if agreed > a.emitted {
		a.emitStableLocked(words, transcript, agreed, fallback)
	}
	a.previous = words
}

func (a *deepgramASR) emitStableLocked(words []dgWord, text string, count int, fallback core.ASREvent) {
	if count <= a.emitted || a.emitted > len(words) || count > len(words) {
		return
	}
	fresh := words[a.emitted:count]
	glue := ""
	var delta string
	end, ok := dgCharsThrough(text, words, count)
	if ok {
		if end <= a.emittedChars {
			a.emitted = count
			return
		}
		delta = text[a.emittedChars:end]
	} else {
		// The word list and transcript disagree; use the words and let the
		// transcript's own whitespace (not a language guess) decide the spacing.
		space := ""
		if strings.ContainsFunc(text, unicode.IsSpace) {
			space = " "
		}
		parts := make([]string, len(fresh))
		for index, word := range fresh {
			parts[index] = dgTextOf(word)
		}
		delta = strings.Join(parts, space)
		glue = space
	}
	if delta == "" {
		return
	}
	event := deepgramSpan(fresh, fallback)
	event.Type, event.Text, event.Glue = "stable", delta, &glue
	a.deliverLocked(event)
	a.emitted = count
	if ok {
		a.emittedChars = end
	} else {
		a.emittedChars += len(delta)
	}
}

func (a *deepgramASR) resetAgreementLocked() {
	a.previous = nil
	a.emitted = 0
	a.emittedChars = 0
	a.utteranceStart = nil
}

// Forget the agreement window without forgetting what was already emitted:
// UtteranceEnd and Close can arrive before the final Results for the same
// words, and re-emitting them would duplicate a stable clause.
func (a *deepgramASR) resetWordsLocked() {
	a.previous = nil
	a.emitted = 0
}

func dgTokenOf(word dgWord) string {
	if word.Word != nil {
		return *word.Word
	}
	if word.PunctuatedWord != nil {
		return *word.PunctuatedWord
	}
	return ""
}

func dgTextOf(word dgWord) string {
	if word.PunctuatedWord != nil {
		return *word.PunctuatedWord
	}
	if word.Word != nil {
		return *word.Word
	}
	return ""
}

func dgCommonWords(previous, current []dgWord) int {
	count := 0
	for count < len(previous) && count < len(current) && dgTokenOf(previous[count]) == dgTokenOf(current[count]) {
		count++
	}
	return count
}

// dgCharsThrough is the character offset in the provider transcript just after
// word count-1, so emitted text keeps Deepgram's spacing and punctuation.
func dgCharsThrough(text string, words []dgWord, count int) (int, bool) {
	at := 0
	for index := 0; index < count && index < len(words); index++ {
		token := dgTextOf(words[index])
		if token == "" {
			continue
		}
		found := strings.Index(text[at:], token)
		if found < 0 {
			return 0, false
		}
		at += found + len(token)
	}
	return at, true
}

func dgFrom(words []dgWord, from int) []dgWord {
	if from >= len(words) {
		return nil
	}
	if from < 0 {
		from = 0
	}
	return words[from:]
}

func deepgramSpan(words []dgWord, fallback core.ASREvent) core.ASREvent {
	event := core.ASREvent{StartMS: fallback.StartMS, EndMS: fallback.EndMS}
	if len(words) > 0 {
		event.StartMS = milliseconds(words[0].Start, fallback.StartMS)
		event.EndMS = milliseconds(words[len(words)-1].End, fallback.EndMS)
	}
	return event
}

func milliseconds(seconds *float64, fallback int64) int64 {
	if seconds == nil || math.IsNaN(*seconds) || math.IsInf(*seconds, 0) {
		return fallback
	}
	return int64(math.Round(*seconds * 1000))
}

func sumStartDuration(start, duration *float64) *float64 {
	if start == nil && duration == nil {
		return nil
	}
	sum := 0.0
	if start != nil {
		sum += *start
	}
	if duration != nil {
		sum += *duration
	}
	return &sum
}

func (a *deepgramASR) Push(pcm []int16, audioMS int64) ([]core.ASREvent, error) {
	a.pushAudio(pcm, func() bool { return a.opened }, deepgramSendError)
	return a.take()
}

func (a *deepgramASR) Close() {
	a.mu.Lock()
	a.closing = true
	a.pendingAudio = nil
	a.abortPendingDialLocked()
	conn, opened, ctx := a.conn, a.opened, a.ctx
	a.mu.Unlock()
	if conn == nil {
		return
	}
	if !opened {
		_ = conn.CloseNow()
		return
	}
	// Let Deepgram flush its final Results before the socket goes away: it
	// closes the socket itself right after the last final, and the fallback
	// only fires when the server is stuck.
	_ = conn.Write(ctx, websocket.MessageText, []byte(`{"type":"CloseStream"}`))
	time.AfterFunc(3*time.Second, func() { _ = conn.CloseNow() })
}

// receive delivers one socket frame to the protocol handler; tests use it the
// way the JavaScript contract tests call socket.receive().
func (a *deepgramASR) receive(data []byte) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.handleLocked(data)
}
