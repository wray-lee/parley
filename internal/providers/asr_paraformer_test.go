package providers

import (
	"encoding/json"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"

	"github.com/wray-lee/parley/internal/core"
	"github.com/wray-lee/parley/internal/protocol"
)

// Mirrors tests/contract/asr-paraformer.test.js.

type fixtureEvent struct {
	Type    string  `json:"type"`
	Text    string  `json:"text"`
	Glue    *string `json:"glue"`
	StartMS *int64  `json:"startMs"`
	EndMS   *int64  `json:"endMs"`
}

type paraformerFixture struct {
	Options struct {
		Model string `json:"model"`
		URL   string `json:"url"`
	} `json:"options"`
	Auth      struct{ Key string } `json:"auth"`
	Direction protocol.Direction   `json:"direction"`
	Frames    []struct {
		AtMS   int64           `json:"atMs"`
		Data   json.RawMessage `json:"data"`
		Events []fixtureEvent  `json:"events"`
	} `json:"frames"`
	Handshake struct {
		Status int `json:"status"`
	} `json:"handshake"`
	Expected struct {
		Kind             string   `json:"kind"`
		MessageContains  string   `json:"messageContains"`
		FallbackContains []string `json:"fallbackContains"`
	} `json:"expected"`
}

func loadParaformerFixture(t *testing.T, name string) paraformerFixture {
	t.Helper()
	var fixture paraformerFixture
	readFixture(t, filepath.Join("..", "..", "spec", "fixtures", "paraformer", name+".json"), &fixture)
	return fixture
}

func startParaformer(t *testing.T, fixture paraformerFixture, transport *fakeTransport) *paraformerASR {
	t.Helper()
	asr := NewParaformerASR(ParaformerOptions{
		APIKey: fixture.Auth.Key, Model: fixture.Options.Model, URL: fixture.Options.URL,
		dial: transport.dial,
	})
	asr.Open(fixture.Direction)
	waitOpened(t, &asr.socketASR)
	return asr
}

func compareFixtureEvents(t *testing.T, got []core.ASREvent, want []fixtureEvent) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("got %d events, want %d: %+v", len(got), len(want), got)
	}
	for index := range want {
		actual, expected := got[index], want[index]
		if actual.Type != expected.Type || actual.Text != expected.Text {
			t.Fatalf("event %d = %+v, want %+v", index, actual, expected)
		}
		if (actual.Glue == nil) != (expected.Glue == nil) {
			t.Fatalf("event %d glue = %v, want %v", index, actual.Glue, expected.Glue)
		}
		if expected.StartMS != nil && actual.StartMS != *expected.StartMS {
			t.Fatalf("event %d startMs = %d, want %d", index, actual.StartMS, *expected.StartMS)
		}
		if expected.EndMS != nil && actual.EndMS != *expected.EndMS {
			t.Fatalf("event %d endMs = %d, want %d", index, actual.EndMS, *expected.EndMS)
		}
	}
}

var uuidPattern = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

func TestParaformerContractOpensTaskAndStreamsPCMAfterTaskStarted(t *testing.T) {
	fixture := loadParaformerFixture(t, "interim-final")
	transport := &fakeTransport{}
	asr := NewParaformerASR(ParaformerOptions{
		APIKey: "<redacted>", Model: fixture.Options.Model, URL: fixture.Options.URL,
		dial: transport.dial,
	})
	asr.Open(fixture.Direction)
	waitOpened(t, &asr.socketASR)

	_, header, _ := transport.attempt(t)
	if got := header.Get("Authorization"); got != "Bearer <redacted>" {
		t.Fatalf("authorization = %q", got)
	}
	socket := transport.socket(t, 0)
	writes := socket.sent()
	if len(writes) != 1 || writes[0].typ != websocket.MessageText {
		t.Fatalf("first frame = %+v", writes)
	}
	var runTask struct {
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
			Input map[string]any `json:"input"`
		} `json:"payload"`
	}
	if err := json.Unmarshal(writes[0].data, &runTask); err != nil {
		t.Fatal(err)
	}
	if runTask.Header.Action != "run-task" || runTask.Header.Streaming != "duplex" || !uuidPattern.MatchString(runTask.Header.TaskID) {
		t.Fatalf("run-task header = %+v", runTask.Header)
	}
	if runTask.Payload.TaskGroup != "audio" || runTask.Payload.Task != "asr" ||
		runTask.Payload.Function != "recognition" || runTask.Payload.Model != "paraformer-realtime-v2" ||
		runTask.Payload.Parameters.Format != "pcm" || runTask.Payload.Parameters.SampleRate != 16000 ||
		len(runTask.Payload.Parameters.LanguageHints) != 1 || runTask.Payload.Parameters.LanguageHints[0] != "ja" ||
		len(runTask.Payload.Input) != 0 {
		t.Fatalf("run-task payload = %+v", runTask.Payload)
	}

	pcm := []int16{1, 2, 3, 4}
	if _, err := asr.Push(pcm, 20); err != nil {
		t.Fatal(err)
	}
	if got := len(socket.sent()); got != 1 {
		t.Fatalf("audio must wait for task-started, writes = %d", got)
	}
	asr.receive(fixture.Frames[0].Data) // task-started
	writes = socket.sent()
	if len(writes) != 2 || writes[1].typ != websocket.MessageBinary || string(writes[1].data) != string(pcmBytes(pcm)) {
		t.Fatalf("audio frame after task-started = %+v", writes)
	}

	asr.Close()
	writes = socket.sent()
	var finish struct {
		Header struct {
			Action    string `json:"action"`
			TaskID    string `json:"task_id"`
			Streaming string `json:"streaming"`
		} `json:"header"`
		Payload struct {
			Input map[string]any `json:"input"`
		} `json:"payload"`
	}
	if err := json.Unmarshal(writes[len(writes)-1].data, &finish); err != nil {
		t.Fatal(err)
	}
	if finish.Header.Action != "finish-task" || finish.Header.TaskID != runTask.Header.TaskID ||
		finish.Header.Streaming != "duplex" || len(finish.Payload.Input) != 0 {
		t.Fatalf("finish-task = %+v", finish)
	}
}

func TestParaformerContractFramesReplayToPartialsAndStables(t *testing.T) {
	fixture := loadParaformerFixture(t, "interim-final")
	transport := &fakeTransport{}
	asr := startParaformer(t, fixture, transport)

	var events []core.ASREvent
	var want []fixtureEvent
	for _, frame := range fixture.Frames {
		asr.receive(frame.Data)
		batch, err := asr.Push(nil, frame.AtMS)
		if err != nil {
			t.Fatalf("push at %d ms: %v", frame.AtMS, err)
		}
		events = append(events, batch...)
		want = append(want, frame.Events...)
	}
	compareFixtureEvents(t, events, want)
}

func TestParaformerContractLanguageHintComesFromTheDirectionSource(t *testing.T) {
	transport := &fakeTransport{}
	asr := NewParaformerASR(ParaformerOptions{APIKey: "k", dial: transport.dial})
	asr.Open(protocol.Direction{Src: "zh-Hans", Tgt: "ja"})
	waitOpened(t, &asr.socketASR)

	socket := transport.socket(t, 0)
	var runTask struct {
		Payload struct {
			Parameters struct {
				LanguageHints []string `json:"language_hints"`
			} `json:"parameters"`
		} `json:"payload"`
	}
	if err := json.Unmarshal(socket.sent()[0].data, &runTask); err != nil {
		t.Fatal(err)
	}
	if len(runTask.Payload.Parameters.LanguageHints) != 1 || runTask.Payload.Parameters.LanguageHints[0] != "zh" {
		t.Fatalf("language hints = %v", runTask.Payload.Parameters.LanguageHints)
	}
}

func TestParaformerContractRejectedHandshakeMapsToAuthError(t *testing.T) {
	fixture := loadParaformerFixture(t, "auth-failure")
	transport := &fakeTransport{status: fixture.Handshake.Status, err: errHandshake}
	asr := NewParaformerASR(ParaformerOptions{APIKey: "<redacted>", dial: transport.dial})
	asr.Open(fixture.Direction)
	waitFor(t, "the handshake failure", func() bool {
		asr.mu.Lock()
		defer asr.mu.Unlock()
		return asr.failure != nil
	})

	_, err := asr.Push(nil, 0)
	requireProviderError(t, err, "auth", "HTTP 401", "DASHSCOPE_API_KEY")
	requireProviderError(t, err, "auth", "HTTP 401", "Beijing")
}

func TestParaformerContractTaskFailedFramesMapToQuotaAndRegion(t *testing.T) {
	cases := []struct {
		name             string
		kind             string
		message          string
		fallbackContains []string
	}{
		{"quota-exhaustion", "quota", "Arrearage", []string{"10 h/month", "Beijing"}},
		{"region-error", "region", "ModelNotFound", []string{"Beijing", "endpoint"}},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			fixture := loadParaformerFixture(t, testCase.name)
			transport := &fakeTransport{}
			asr := startParaformer(t, fixture, transport)

			var (
				err       error
				collected []core.ASREvent
			)
			for _, frame := range fixture.Frames {
				asr.receive(frame.Data)
				batch, pushErr := asr.Push(nil, frame.AtMS)
				collected = append(collected, batch...)
				if pushErr != nil {
					err = pushErr
				}
			}
			if len(collected) != 0 {
				t.Fatalf("events = %+v", collected)
			}
			providerError := requireProviderError(t, err, testCase.kind, testCase.message, "")
			for _, hint := range testCase.fallbackContains {
				if !strings.Contains(providerError.Fallback, hint) {
					t.Fatalf("fallback %q does not contain %q", providerError.Fallback, hint)
				}
			}
		})
	}
}

func TestParaformerContractConnectionCloseMapsToUnreachable(t *testing.T) {
	transport := &fakeTransport{}
	asr := NewParaformerASR(ParaformerOptions{APIKey: "k", dial: transport.dial})
	asr.Open(protocol.Direction{Src: "ja", Tgt: "zh"})
	waitOpened(t, &asr.socketASR)
	transport.socket(t, 0).CloseNow()

	waitFor(t, "the close error", func() bool {
		asr.mu.Lock()
		defer asr.mu.Unlock()
		return asr.failure != nil
	})
	_, err := asr.Push(nil, 0)
	requireProviderError(t, err, "unreachable", "Paraformer connection closed", "ASR_PROVIDER=mock")
}

func TestParaformerContractTaskFinishedCloseIsNotAnError(t *testing.T) {
	transport := &fakeTransport{}
	asr := NewParaformerASR(ParaformerOptions{APIKey: "k", dial: transport.dial})
	asr.Open(protocol.Direction{Src: "ja", Tgt: "zh"})
	waitOpened(t, &asr.socketASR)
	asr.receive([]byte(`{"header":{"event":"task-finished"},"payload":{}}`))
	transport.socket(t, 0).CloseNow()

	time.Sleep(50 * time.Millisecond) // let the read loop observe the close
	if _, err := asr.Push(nil, 0); err != nil {
		t.Fatalf("a task-finished close must stay quiet: %v", err)
	}
}

func TestParaformerContractRapidStopStartKeepsTheNewSocket(t *testing.T) {
	fixture := loadParaformerFixture(t, "interim-final")
	transport := &fakeTransport{}
	asr := startParaformer(t, fixture, transport)
	asr.receive(fixture.Frames[0].Data) // task-started
	if _, err := asr.Push(nil, 0); err != nil {
		t.Fatal(err)
	}

	asr.Close()
	old := transport.socket(t, 0)
	asr.Open(protocol.Direction{Src: "zh", Tgt: "ja"})
	waitOpened(t, &asr.socketASR)
	next := transport.socket(t, 1)

	time.Sleep(1200 * time.Millisecond) // the finish-task close timer is 1 s
	if !old.isClosed() {
		t.Fatal("the old socket must be closed")
	}
	if next.isClosed() {
		t.Fatal("the finish-task timer must not close the new socket")
	}
}

func TestParaformerContractAMissingKeyDoesNotDial(t *testing.T) {
	transport := &fakeTransport{}
	asr := NewParaformerASR(ParaformerOptions{dial: transport.dial})
	asr.Open(protocol.Direction{Src: "ja", Tgt: "zh"})

	_, err := asr.Push(nil, 0)
	requireProviderError(t, err, "auth", "DASHSCOPE_API_KEY is not set", "ASR_PROVIDER=mock")
	if transport.calls != 0 {
		t.Fatal("a missing key must not dial")
	}
}

func TestParaformerContractCloseAbortsAPendingDial(t *testing.T) {
	transport := &fakeTransport{hold: make(chan struct{})}
	asr := NewParaformerASR(ParaformerOptions{APIKey: "k", dial: transport.dial})
	asr.Open(protocol.Direction{Src: "zh-Hans", Tgt: "ja"})
	asr.Close()
	waitDialReturned(t, transport)
}

func TestParaformerContractLanguageHintSplitsOnDashOrUnderscore(t *testing.T) {
	cases := []struct{ src, want string }{
		{"zh-Hans", "zh"},
		{"zh_Hans", "zh"},
		{"EN_us", "en"},
		{"ja", "ja"},
		{"", ""},
	}
	for _, testCase := range cases {
		if got := languageHint(testCase.src); got != testCase.want {
			t.Fatalf("languageHint(%q) = %q, want %q", testCase.src, got, testCase.want)
		}
	}
}

func TestParaformerContractOnlyRegionInTheMessageMapsToRegion(t *testing.T) {
	cases := []struct{ code, message, kind string }{
		{"InvalidParameter", "model not available", "protocol"},
		{"InvalidParameter", "the service is not available in this region", "region"},
	}
	for _, testCase := range cases {
		err := paraformerInBandError(&pfHeader{ErrorCode: testCase.code, ErrorMessage: testCase.message})
		providerError, ok := err.(*protocol.ProviderError)
		if !ok || providerError.Kind != testCase.kind {
			t.Fatalf("error %q / %q = %v, want kind %q", testCase.code, testCase.message, err, testCase.kind)
		}
	}
}

func TestParaformerContractStableFallbackTreatsAllWhitespaceAsSpace(t *testing.T) {
	for _, space := range []string{string(rune(0x3000)), string(rune(0x00A0))} {
		asr := NewParaformerASR(ParaformerOptions{})
		words := []pfWord{{Text: "p"}, {Text: "q"}}
		if got := asr.deltaTextLocked("x"+space+"y", words, 0, 2); got != "p q" {
			t.Fatalf("stable delta for whitespace U+%04X = %q, want %q", []rune(space)[0], got, "p q")
		}
	}
}
