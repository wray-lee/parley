// Alibaba DashScope Paraformer real-time ASR (Beijing region), ported from
// core/providers/asr-paraformer.js. The client sends run-task, waits for
// task-started, then streams raw PCM16 mono 16 kHz; the server returns
// result-generated frames whose sentence text is a cumulative hypothesis.
// Stable Prefix is LocalAgreement-2 over consecutive word lists; sentence_end
// text is stable outright. The adapter emits both event kinds itself, so it
// must NOT report itself revisable.
// language_hints is fixed from the Direction's src: no auto-detection.
// Config data: model (default paraformer-realtime-v2) and url.
// Auth: Authorization: Bearer <DASHSCOPE_API_KEY> on the WebSocket handshake.

package providers

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"regexp"
	"strings"
	"time"
	"unicode"

	"github.com/coder/websocket"

	"github.com/wray-lee/parley/internal/core"
	"github.com/wray-lee/parley/internal/protocol"
)

const (
	paraformerDefaultURL   = "wss://dashscope.aliyuncs.com/api-ws/v1/inference"
	paraformerDefaultModel = "paraformer-realtime-v2"
)

// ParaformerOptions are the config data of one connection.
type ParaformerOptions struct {
	APIKey string
	Model  string
	URL    string
	dial   wsDial
}

type pfWord struct {
	Text        string   `json:"text"`
	Punctuation string   `json:"punctuation"`
	BeginTime   *float64 `json:"begin_time"`
	EndTime     *float64 `json:"end_time"`
}

type pfSentence struct {
	Text        string   `json:"text"`
	Heartbeat   bool     `json:"heartbeat"`
	SentenceEnd bool     `json:"sentence_end"`
	BeginTime   *float64 `json:"begin_time"`
	EndTime     *float64 `json:"end_time"`
	Words       []pfWord `json:"words"`
}

type pfHeader struct {
	Event        string `json:"event"`
	ErrorCode    any    `json:"error_code"`
	ErrorMessage any    `json:"error_message"`
}

type pfMessage struct {
	Header  *pfHeader `json:"header"`
	Payload *struct {
		Output *struct {
			Sentence *pfSentence `json:"sentence"`
		} `json:"output"`
	} `json:"payload"`
}

type paraformerASR struct {
	socketASR
	opts         ParaformerOptions
	taskID       string
	started      bool
	finished     bool
	previous     []pfWord
	emitted      int
	emittedChars int
}

// NewParaformerASR builds the adapter; a missing key fails on Open like the
// JavaScript oracle.
func NewParaformerASR(options ParaformerOptions) *paraformerASR {
	if options.Model == "" {
		options.Model = paraformerDefaultModel
	}
	if options.URL == "" {
		options.URL = paraformerDefaultURL
	}
	return &paraformerASR{socketASR: newSocketASR(options.dial), opts: options}
}

func (a *paraformerASR) Name() string      { return "paraformer" }
func (a *paraformerASR) AcceptsLive() bool { return true }
func (a *paraformerASR) Revisable() bool   { return false }
func (a *paraformerASR) NewStream() core.ASR {
	return NewParaformerASR(a.opts)
}
func (a *paraformerASR) Discard() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.resetAgreementLocked()
	a.events = nil
	a.pendingAudio = nil
	// failure is kept: an error queued but not yet surfaced must still reach
	// the next Push (external failures never vanish).
}

func (a *paraformerASR) Open(direction protocol.Direction) {
	ctx, mine := a.begin()
	a.mu.Lock()
	a.started = false
	a.finished = false
	a.resetAgreementLocked()
	if direction.Src == "" {
		a.raiseLocked(asrError("protocol", "paraformer ASR needs the Direction source language", fallbackMock))
		a.mu.Unlock()
		return
	}
	if a.opts.APIKey == "" {
		a.raiseLocked(asrError("auth", "DASHSCOPE_API_KEY is not set",
			"Set DASHSCOPE_API_KEY (Beijing region), "+fallbackMock))
		a.mu.Unlock()
		return
	}
	a.taskID = newTaskID()
	header := http.Header{"Authorization": {"Bearer " + a.opts.APIKey}}
	url := a.opts.URL
	runTask := paraformerRunTask(a.taskID, a.opts.Model, direction.Src)
	a.mu.Unlock()
	go func() {
		a.run(ctx, mine, url, header, nil, runTask, false, a.handleLocked,
			paraformerDialError, a.closedError, paraformerSendError)
	}()
}

// paraformerRunTask is the DashScope recognition setup frame.
func paraformerRunTask(taskID, model, src string) []byte {
	frame := struct {
		Header struct {
			Action    string `json:"action"`
			TaskID    string `json:"task_id"`
			Streaming string `json:"streaming"`
		} `json:"header"`
		Payload struct {
			TaskGroup  string `json:"task_group"`
			Task       string `json:"task"`
			Function   string `json:"function"`
			Model      string `json:"model"`
			Parameters struct {
				Format        string   `json:"format"`
				SampleRate    int      `json:"sample_rate"`
				LanguageHints []string `json:"language_hints"`
			} `json:"parameters"`
			Input struct{} `json:"input"`
		} `json:"payload"`
	}{}
	frame.Header.Action = "run-task"
	frame.Header.TaskID = taskID
	frame.Header.Streaming = "duplex"
	frame.Payload.TaskGroup = "audio"
	frame.Payload.Task = "asr"
	frame.Payload.Function = "recognition"
	frame.Payload.Model = model
	frame.Payload.Parameters.Format = "pcm"
	frame.Payload.Parameters.SampleRate = 16000
	frame.Payload.Parameters.LanguageHints = []string{languageHint(src)}
	data, _ := json.Marshal(frame)
	return data
}

func (a *paraformerASR) handleLocked(data []byte) {
	if a.failed {
		return
	}
	var message pfMessage
	if err := json.Unmarshal(data, &message); err != nil {
		a.raiseLocked(asrError("protocol", "Paraformer sent a frame that is not JSON", fallbackMock))
		return
	}
	if message.Header == nil {
		return
	}
	switch message.Header.Event {
	case "task-started":
		a.started = true
		pending := a.pendingAudio
		a.pendingAudio = nil
		conn, closing, sendCtx := a.conn, a.closing, a.ctx
		for _, chunk := range pending {
			if conn == nil || closing {
				break
			}
			if err := conn.Write(sendCtx, websocket.MessageBinary, chunk); err != nil {
				a.failLocked(paraformerSendError(err))
				break
			}
		}
	case "result-generated":
		if message.Payload != nil && message.Payload.Output != nil {
			a.handleSentenceLocked(message.Payload.Output.Sentence)
		}
	case "task-failed":
		a.failLocked(paraformerInBandError(message.Header))
	case "task-finished":
		a.finished = true
	}
}

func (a *paraformerASR) handleSentenceLocked(sentence *pfSentence) {
	if sentence == nil || sentence.Heartbeat {
		return
	}
	text := sentence.Text
	words := sentence.Words
	fallback := core.ASREvent{
		StartMS: paraformerMilliseconds(sentence.BeginTime, 0),
		EndMS:   paraformerMilliseconds(sentence.EndTime, paraformerMilliseconds(sentence.BeginTime, 0)),
	}
	span := paraformerSpan(words, fallback)

	if sentence.SentenceEnd {
		// Sentence-end text is stable outright: emit only the not-yet-stable tail.
		a.emitStableLocked(words, text, len(words), fallback)
		if len(words) == 0 && len(text) > a.emittedChars {
			event := span
			event.Type, event.Text = "stable", text[a.emittedChars:]
			a.deliverLocked(event)
		}
		a.resetAgreementLocked()
		return
	}
	if text == "" && len(words) == 0 {
		return
	}
	partial := span
	partial.Type, partial.Text = "partial", text
	a.deliverLocked(partial)

	agreed := pfCommonWords(a.previous, words)
	if agreed > a.emitted {
		a.emitStableLocked(words, text, agreed, fallback)
	}
	a.previous = words
}

func (a *paraformerASR) emitStableLocked(words []pfWord, text string, count int, fallback core.ASREvent) {
	if count <= a.emitted || a.emitted > len(words) || count > len(words) {
		return
	}
	fresh := words[a.emitted:count]
	delta := a.deltaTextLocked(text, words, a.emitted, count)
	if delta == "" {
		return
	}
	event := paraformerSpan(fresh, fallback)
	event.Type, event.Text = "stable", delta
	a.deliverLocked(event)
	a.emitted = count
	if end, ok := pfCharsThrough(text, words, count); ok {
		a.emittedChars = end
	} else {
		a.emittedChars += len(delta)
	}
}

func (a *paraformerASR) deltaTextLocked(text string, words []pfWord, from, to int) string {
	if end, ok := pfCharsThrough(text, words, to); ok && end >= a.emittedChars {
		return text[a.emittedChars:end]
	}
	space := ""
	if strings.ContainsFunc(text, unicode.IsSpace) {
		space = " "
	}
	parts := make([]string, 0, to-from)
	for _, word := range words[from:to] {
		parts = append(parts, pfWordText(word))
	}
	return strings.Join(parts, space)
}

func (a *paraformerASR) resetAgreementLocked() {
	a.previous = nil
	a.emitted = 0
	a.emittedChars = 0
}

func pfTokenOf(word pfWord) string { return word.Text }

func pfWordText(word pfWord) string { return word.Text + word.Punctuation }

func pfCommonWords(previous, current []pfWord) int {
	count := 0
	for count < len(previous) && count < len(current) && pfTokenOf(previous[count]) == pfTokenOf(current[count]) {
		count++
	}
	return count
}

// pfCharsThrough is the character offset in the sentence text just after word
// count-1, so emitted text keeps Paraformer's own spacing and punctuation.
func pfCharsThrough(text string, words []pfWord, count int) (int, bool) {
	at := 0
	for index := 0; index < count && index < len(words); index++ {
		token := pfWordText(words[index])
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

func paraformerSpan(words []pfWord, fallback core.ASREvent) core.ASREvent {
	event := core.ASREvent{StartMS: fallback.StartMS, EndMS: fallback.EndMS}
	if len(words) > 0 {
		event.StartMS = paraformerMilliseconds(words[0].BeginTime, fallback.StartMS)
		event.EndMS = paraformerMilliseconds(words[len(words)-1].EndTime, fallback.EndMS)
	}
	return event
}

// DashScope begin_time/end_time are already milliseconds.
func paraformerMilliseconds(value *float64, fallback int64) int64 {
	if value == nil || math.IsNaN(*value) || math.IsInf(*value, 0) {
		return fallback
	}
	return int64(math.Round(*value))
}

func paraformerDialError(status int, err error) error {
	if status != 0 {
		return paraformerStatusError(status)
	}
	return asrError("unreachable", "Paraformer connection failed", fallbackMock)
}

// closedError stays quiet after task-finished, which is a normal end of stream.
func (a *paraformerASR) closedError(code string) error {
	if a.finished {
		return nil
	}
	return asrError("unreachable", "Paraformer connection closed (code "+code+")", fallbackMock)
}

func paraformerSendError(err error) error {
	return asrError("unreachable", "Paraformer send failed: "+err.Error(), fallbackMock)
}

func paraformerStatusError(status int) error {
	switch status {
	case 401, 403:
		return asrError("auth", "Paraformer rejected the credentials (HTTP "+fmt.Sprint(status)+")",
			"Check DASHSCOPE_API_KEY and use a Beijing-region key, "+fallbackMock)
	case 402:
		return asrError("quota", "Paraformer has insufficient credit (HTTP 402)",
			"Top up in Model Studio; the free quota is 10 h/month in the Beijing region and may change, "+fallbackMock)
	case 429:
		return asrError("rate_limit", "Paraformer rate limit hit (HTTP 429)", "Wait and retry, "+fallbackMock)
	}
	return asrError("unreachable", "Paraformer handshake failed (HTTP "+fmt.Sprint(status)+")",
		"Check the DashScope endpoint (Beijing region) and the network, "+fallbackMock)
}

var (
	pfQuotaPattern  = regexp.MustCompile(`(?i)arrear|overdue|payment|funds|allocation.?quota|insufficient|quota`)
	pfRatePattern   = regexp.MustCompile(`(?i)throttl|rate.?quota|rate.?limit|too.?many`)
	pfAuthPattern   = regexp.MustCompile(`(?i)invalid.?api.?key|unauthor|access.?denied|authentication`)
	pfRegionPattern = regexp.MustCompile(`(?i)region|model.?not.?found|not.?available|unsupported|endpoint`)
	// Like the oracle: the error_message only maps to region on /region/i.
	pfRegionMessagePattern = regexp.MustCompile(`(?i)region`)
)

func paraformerInBandError(header *pfHeader) error {
	code := "unknown"
	if header.ErrorCode != nil {
		code = stringify(header.ErrorCode)
	}
	detail := ""
	if header.ErrorMessage != nil {
		if message := stringify(header.ErrorMessage); message != "" {
			detail = ": " + message
		}
	}
	switch {
	case pfQuotaPattern.MatchString(code):
		return asrError("quota", "Paraformer error "+code+detail,
			"The Paraformer free quota (10 h/month, Beijing region; it may change) may be used up; top up in Model Studio, "+fallbackMock)
	case pfRatePattern.MatchString(code):
		return asrError("rate_limit", "Paraformer error "+code+detail, "Wait and retry, "+fallbackMock)
	case pfAuthPattern.MatchString(code):
		return asrError("auth", "Paraformer error "+code+detail,
			"Check DASHSCOPE_API_KEY and use a Beijing-region key, "+fallbackMock)
	case pfRegionPattern.MatchString(code) || pfRegionMessagePattern.MatchString(detail):
		return asrError("region", "Paraformer error "+code+detail,
			"Paraformer is served only from the Beijing region; check the endpoint and use a Beijing-region key, "+fallbackMock)
	}
	return asrError("protocol", "Paraformer error "+code+detail,
		"Check the Paraformer model and parameters, "+fallbackMock)
}

func languageHint(src string) string {
	hint := strings.ToLower(src)
	if at := strings.IndexAny(hint, "-_"); at >= 0 {
		return hint[:at]
	}
	return hint
}

func (a *paraformerASR) Push(pcm []int16, audioMS int64) ([]core.ASREvent, error) {
	// task-started must arrive before any audio leaves the client.
	a.pushAudio(pcm, func() bool { return a.started }, paraformerSendError)
	return a.take()
}

func (a *paraformerASR) Close() {
	a.mu.Lock()
	a.closing = true
	a.pendingAudio = nil
	a.resetAgreementLocked()
	a.abortPendingDialLocked()
	conn, started, closingCtx := a.conn, a.started, a.ctx
	taskID := a.taskID
	a.mu.Unlock()
	if conn == nil {
		return
	}
	if !started {
		_ = conn.CloseNow()
		return
	}
	finish := fmt.Sprintf(
		`{"header":{"action":"finish-task","task_id":%q,"streaming":"duplex"},"payload":{"input":{}}}`,
		taskID,
	)
	_ = conn.Write(closingCtx, websocket.MessageText, []byte(finish))
	// Let Paraformer flush final results before the socket goes away. Capture
	// the socket: a fast stop/start must not let this timer close the next one.
	time.AfterFunc(1*time.Second, func() { _ = conn.CloseNow() })
}

func (a *paraformerASR) receive(data []byte) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.handleLocked(data)
}

func newTaskID() string {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		// crypto/rand never fails on supported platforms; a zero id is harmless.
		return "00000000-0000-4000-8000-000000000000"
	}
	raw[6] = (raw[6] & 0x0f) | 0x40
	raw[8] = (raw[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", raw[0:4], raw[4:6], raw[6:8], raw[8:10], raw[10:16])
}
