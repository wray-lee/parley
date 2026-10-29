package providers

import (
	"encoding/json"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"unicode"

	"github.com/wray-lee/parley/internal/core"
	"github.com/wray-lee/parley/internal/protocol"
)

// Mirrors tests/contract/asr-deepgram.test.js: replay the recorded wire frames
// through the adapter and assert the normalised events it emits.

type deepgramFixture struct {
	Options struct {
		Model          string `json:"model"`
		Endpointing    int    `json:"endpointing"`
		UtteranceEndMS int    `json:"utteranceEndMs"`
	} `json:"options"`
	URLParams map[string]string    `json:"urlParams"`
	Auth      struct{ Key string } `json:"auth"`
	Direction protocol.Direction   `json:"direction"`
	Frames    []struct {
		AtMS int64           `json:"atMs"`
		Data json.RawMessage `json:"data"`
	} `json:"frames"`
	ExpectedStableText     string     `json:"expectedStableText"`
	SampleTranscript       string     `json:"sampleTranscript"`
	RecognitionCorrections [][]string `json:"recognitionCorrections"`
	CloseAfterFrame        int        `json:"closeAfterFrame"`
	Handshake              struct {
		Status int `json:"status"`
	} `json:"handshake"`
	Expected struct {
		Kind             string   `json:"kind"`
		MessageContains  string   `json:"messageContains"`
		FallbackContains []string `json:"fallbackContains"`
	} `json:"expected"`
}

func loadDeepgramFixture(t *testing.T, name string) deepgramFixture {
	t.Helper()
	var fixture deepgramFixture
	readFixture(t, filepath.Join("..", "..", "spec", "fixtures", "deepgram", name+".json"), &fixture)
	return fixture
}

func startDeepgram(t *testing.T, fixture deepgramFixture, transport *fakeTransport) *deepgramASR {
	t.Helper()
	asr := NewDeepgramASR(DeepgramOptions{
		APIKey:         fixture.Auth.Key,
		Model:          fixture.Options.Model,
		Endpointing:    fixture.Options.Endpointing,
		UtteranceEndMS: fixture.Options.UtteranceEndMS,
		dial:           transport.dial,
	})
	asr.Open(fixture.Direction)
	waitOpened(t, &asr.socketASR)
	return asr
}

// deepgramStableText concatenates exactly like the JavaScript contract test:
// a missing glue contributes nothing (the adapter always sets one).
func deepgramStableText(events []core.ASREvent) string {
	var builder strings.Builder
	for _, event := range events {
		if event.Type != "stable" {
			continue
		}
		if event.Glue != nil {
			builder.WriteString(*event.Glue)
		}
		builder.WriteString(event.Text)
	}
	return builder.String()
}

// normalizeDeepgram mirrors the JavaScript test: apply the recorded recognition
// corrections, then drop whitespace and CJK punctuation.
func normalizeDeepgram(text string, corrections [][]string) string {
	for _, correction := range corrections {
		if len(correction) == 2 {
			text = strings.ReplaceAll(text, correction[0], correction[1])
		}
	}
	var builder strings.Builder
	for _, symbol := range text {
		if unicode.IsSpace(symbol) || strings.ContainsRune("。、，！？；：", symbol) {
			continue
		}
		builder.WriteRune(symbol)
	}
	return builder.String()
}

func TestDeepgramContractLiveCapturesReplayToTheProvidersTranscript(t *testing.T) {
	for _, name := range []string{"live-ja-context", "live-zh-numbers"} {
		t.Run(name, func(t *testing.T) {
			fixture := loadDeepgramFixture(t, name)
			transport := &fakeTransport{}
			asr := startDeepgram(t, fixture, transport)

			target, header, _ := transport.attempt(t)
			parsed, err := url.Parse(target)
			if err != nil {
				t.Fatal(err)
			}
			if parsed.Scheme+"://"+parsed.Host+parsed.Path != "wss://api.deepgram.com/v1/listen" {
				t.Fatalf("url = %s", target)
			}
			for key, want := range fixture.URLParams {
				if got := parsed.Query().Get(key); got != want {
					t.Fatalf("url param %s = %q, want %q", key, got, want)
				}
			}
			if parsed.Query().Has("detect_language") {
				t.Fatal("language must be fixed per connection, not auto-detected")
			}
			if got := header.Get("Authorization"); got != "Token "+fixture.Auth.Key {
				t.Fatalf("authorization = %q", got)
			}

			var events []core.ASREvent
			for _, frame := range fixture.Frames {
				asr.receive(frame.Data)
				batch, err := asr.Push(nil, frame.AtMS)
				if err != nil {
					t.Fatalf("push at %d ms: %v", frame.AtMS, err)
				}
				events = append(events, batch...)
			}

			text := deepgramStableText(events)
			if text != fixture.ExpectedStableText {
				t.Fatalf("stable text = %q, want %q", text, fixture.ExpectedStableText)
			}
			if strings.ContainsAny(text, " \t\n") {
				t.Fatal("the provider transcript has no whitespace here")
			}
			if normalizeDeepgram(text, fixture.RecognitionCorrections) != normalizeDeepgram(fixture.SampleTranscript, nil) {
				t.Fatal("stable text does not cover the sample transcript modulo recorded recognition errors")
			}
			partials := 0
			for _, event := range events {
				if event.Type == "partial" {
					partials++
				}
				if event.Type != "stable" {
					continue
				}
				if event.StartMS > event.EndMS {
					t.Fatalf("span %d..%d is not audio time", event.StartMS, event.EndMS)
				}
			}
			if partials == 0 {
				t.Fatal("interims must be display-only Partials")
			}
		})
	}
}

func TestDeepgramContractLateFinalAppendsItsTailWithoutRepeatingStableWords(t *testing.T) {
	fixture := loadDeepgramFixture(t, "live-ja-context-late-final")
	transport := &fakeTransport{}
	asr := startDeepgram(t, fixture, transport)

	var events []core.ASREvent
	collect := func() {
		batch, err := asr.Push(nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		events = append(events, batch...)
	}
	for _, frame := range fixture.Frames[:fixture.CloseAfterFrame] {
		asr.receive(frame.Data)
		collect()
	}
	asr.Close()
	for _, frame := range fixture.Frames[fixture.CloseAfterFrame:] {
		asr.receive(frame.Data)
		collect()
	}

	text := deepgramStableText(events)
	if text != fixture.ExpectedStableText {
		t.Fatalf("stable text = %q, want %q", text, fixture.ExpectedStableText)
	}
	if strings.Count(text, "議事録が固まったら") != 1 {
		t.Fatal("the clause is repeated after CloseStream")
	}
}

func TestDeepgramContractStableTextKeepsTheProviderSpacing(t *testing.T) {
	transport := &fakeTransport{}
	asr := NewDeepgramASR(DeepgramOptions{APIKey: "<redacted>", dial: transport.dial})
	asr.Open(protocol.Direction{Src: "en", Tgt: "zh"})
	waitOpened(t, &asr.socketASR)

	frame := func(transcript string, words []string, final bool) []byte {
		type word struct {
			Word           string  `json:"word"`
			PunctuatedWord string  `json:"punctuated_word"`
			Start          float64 `json:"start"`
			End            float64 `json:"end"`
		}
		payload := struct {
			Type     string  `json:"type"`
			Start    float64 `json:"start"`
			Duration float64 `json:"duration"`
			IsFinal  bool    `json:"is_final"`
			Channel  struct {
				Alternatives []struct {
					Transcript string `json:"transcript"`
					Words      []word `json:"words"`
				} `json:"alternatives"`
			} `json:"channel"`
		}{Type: "Results", Start: 0, Duration: 1, IsFinal: final}
		alternative := struct {
			Transcript string `json:"transcript"`
			Words      []word `json:"words"`
		}{Transcript: transcript}
		for index, text := range words {
			alternative.Words = append(alternative.Words, word{
				Word: text, PunctuatedWord: text,
				Start: float64(index) * 0.4, End: float64(index)*0.4 + 0.3,
			})
		}
		payload.Channel.Alternatives = append(payload.Channel.Alternatives, alternative)
		data, err := json.Marshal(payload)
		if err != nil {
			t.Fatal(err)
		}
		return data
	}

	var events []core.ASREvent
	for _, data := range [][]byte{
		frame("hello", []string{"hello"}, false),
		frame("hello world", []string{"hello", "world"}, false),
		frame("hello world", []string{"hello", "world"}, true),
	} {
		asr.receive(data)
		batch, err := asr.Push(nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		events = append(events, batch...)
	}
	if text := deepgramStableText(events); text != "hello world" {
		t.Fatalf("stable text = %q", text)
	}
}

func TestDeepgramContractWebAuthRidesTheSubprotocol(t *testing.T) {
	transport := &fakeTransport{}
	asr := NewDeepgramASR(DeepgramOptions{
		APIKey: "test-key", Auth: "subprotocol", dial: transport.dial,
	})
	asr.Open(protocol.Direction{Src: "ja", Tgt: "zh"})
	waitOpened(t, &asr.socketASR)

	target, header, subprotocols := transport.attempt(t)
	if header.Get("Authorization") != "" {
		t.Fatal("subprotocol auth must not send an Authorization header")
	}
	if len(subprotocols) != 2 || subprotocols[0] != "token" || subprotocols[1] != "test-key" {
		t.Fatalf("subprotocols = %v", subprotocols)
	}
	parsed, _ := url.Parse(target)
	if got := parsed.Query().Get("language"); got != "ja" {
		t.Fatalf("language = %q", got)
	}
}

func TestDeepgramContractReopenClosesThePreviousSocket(t *testing.T) {
	transport := &fakeTransport{}
	asr := NewDeepgramASR(DeepgramOptions{APIKey: "k", dial: transport.dial})
	asr.Open(protocol.Direction{Src: "ja", Tgt: "zh"})
	waitOpened(t, &asr.socketASR)
	old := transport.socket(t, 0)

	asr.Open(protocol.Direction{Src: "zh", Tgt: "ja"})
	waitFor(t, "the second socket", func() bool {
		transport.mu.Lock()
		defer transport.mu.Unlock()
		return len(transport.sockets) == 2
	})
	next := transport.socket(t, 1)
	if !old.isClosed() {
		t.Fatal("reopening must close the previous socket")
	}
	if next.isClosed() {
		t.Fatal("the new socket must stay open")
	}
	parsed, _ := url.Parse(transport.urls[1])
	if parsed.Query().Get("language") != "zh" {
		t.Fatalf("reopened language = %q", parsed.Query().Get("language"))
	}
}

func TestDeepgramContractAMissingKeyFailsWithoutConnecting(t *testing.T) {
	transport := &fakeTransport{}
	asr := NewDeepgramASR(DeepgramOptions{dial: transport.dial})
	asr.Open(protocol.Direction{Src: "ja", Tgt: "zh"})

	_, err := asr.Push(nil, 0)
	requireProviderError(t, err, "auth", "DEEPGRAM_API_KEY is not set", "ASR_PROVIDER=mock")
	if transport.calls != 0 {
		t.Fatal("a missing key must not dial")
	}
}

func TestDeepgramContractAudioFramesWaitForTheSocketToOpen(t *testing.T) {
	fixture := loadDeepgramFixture(t, "live-ja-context")
	transport := &fakeTransport{hold: make(chan struct{})}
	asr := NewDeepgramASR(DeepgramOptions{APIKey: "k", dial: transport.dial})
	asr.Open(fixture.Direction)
	waitFor(t, "the dial attempt", func() bool {
		transport.mu.Lock()
		defer transport.mu.Unlock()
		return transport.calls == 1
	})

	pcm := []int16{1, 2, 3}
	if _, err := asr.Push(pcm, 20); err != nil {
		t.Fatal(err)
	}
	close(transport.hold)
	socket := waitSocket(t, transport, 0)
	waitFor(t, "the queued audio", func() bool { return len(socket.sent()) == 1 })
	writes := socket.sent()
	if want := pcmBytes(pcm); string(writes[0].data) != string(want) {
		t.Fatalf("audio frame = %v, want %v", writes[0].data, want)
	}
}

func TestDeepgramContractRejectedHandshakeMapsToAnErrorEvent(t *testing.T) {
	for _, name := range []string{"auth-failure", "quota-exhaustion"} {
		t.Run(name, func(t *testing.T) {
			fixture := loadDeepgramFixture(t, name)
			transport := &fakeTransport{status: fixture.Handshake.Status, err: errHandshake}
			asr := NewDeepgramASR(DeepgramOptions{APIKey: "<redacted>", dial: transport.dial})
			asr.Open(fixture.Direction)
			waitFor(t, "the handshake failure", func() bool {
				asr.mu.Lock()
				defer asr.mu.Unlock()
				return asr.failure != nil
			})

			_, err := asr.Push(nil, 0)
			providerError := requireProviderError(t, err, fixture.Expected.Kind, fixture.Expected.MessageContains, "")
			for _, hint := range fixture.Expected.FallbackContains {
				if !strings.Contains(providerError.Fallback, hint) {
					t.Fatalf("fallback %q does not contain %q", providerError.Fallback, hint)
				}
			}
		})
	}
}

func TestDeepgramContractConnectionCloseSurfacesAnUnreachableError(t *testing.T) {
	transport := &fakeTransport{}
	asr := NewDeepgramASR(DeepgramOptions{APIKey: "k", dial: transport.dial})
	asr.Open(protocol.Direction{Src: "ja", Tgt: "zh"})
	waitOpened(t, &asr.socketASR)
	transport.socket(t, 0).CloseNow()

	waitFor(t, "the close error", func() bool {
		asr.mu.Lock()
		defer asr.mu.Unlock()
		return asr.failure != nil
	})
	_, err := asr.Push(nil, 0)
	requireProviderError(t, err, "unreachable", "Deepgram connection closed", "ASR_PROVIDER=mock")
}

func TestDeepgramContractInBandErrorsMapByCode(t *testing.T) {
	transport := &fakeTransport{}
	asr := NewDeepgramASR(DeepgramOptions{APIKey: "k", dial: transport.dial})
	asr.Open(protocol.Direction{Src: "ja", Tgt: "zh"})
	waitOpened(t, &asr.socketASR)

	asr.receive([]byte(`{"type":"Error","err_code":"ASR_AUTH_FAILED","err_msg":"Invalid credentials."}`))
	_, err := asr.Push(nil, 0)
	requireProviderError(t, err, "auth", "ASR_AUTH_FAILED", "DEEPGRAM_API_KEY")

	// A soft in-band error must leave the stream usable.
	asr.receive([]byte(`{"type":"Results","is_final":true,"channel":{"alternatives":[{"transcript":"ok"}]}}`))
	events, err := asr.Push(nil, 0)
	if err != nil || len(events) != 1 || events[0].Text != "ok" {
		t.Fatalf("events after a soft error = %+v, %v", events, err)
	}
}

func TestDeepgramContractCloseAbortsAPendingDial(t *testing.T) {
	transport := &fakeTransport{hold: make(chan struct{})}
	asr := NewDeepgramASR(DeepgramOptions{APIKey: "k", dial: transport.dial})
	asr.Open(protocol.Direction{Src: "en", Tgt: "zh"})
	asr.Close()
	waitDialReturned(t, transport)
}

func TestDeepgramContractStableFallbackTreatsAllWhitespaceAsSpace(t *testing.T) {
	for _, space := range []string{string(rune(0x3000)), string(rune(0x00A0))} {
		text := "x" + space + "y"
		asr := NewDeepgramASR(DeepgramOptions{})
		first, second := "p", "q"
		asr.emitStableLocked([]dgWord{{Word: &first}, {Word: &second}}, text, 2, core.ASREvent{})
		events, _ := asr.take()
		if len(events) != 1 || events[0].Text != "p q" {
			t.Fatalf("stable text for transcript %q = %+v, want %q", text, events, "p q")
		}
	}
}
