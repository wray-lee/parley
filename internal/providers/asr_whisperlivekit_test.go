package providers

import (
	"encoding/json"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"

	"github.com/wray-lee/parley/internal/protocol"
)

// Mirrors tests/contract/asr-whisperlivekit.test.js.

type whisperLiveKitFixture struct {
	URL       string             `json:"url"`
	Direction protocol.Direction `json:"direction"`
	Frames    []struct {
		AtMS   int64           `json:"atMs"`
		Data   json.RawMessage `json:"data"`
		Events []fixtureEvent  `json:"events"`
	} `json:"frames"`
	ExpectedStableText string          `json:"expectedStableText"`
	FrameAtMS          int64           `json:"frameAtMs"`
	Frame              json.RawMessage `json:"frame"`
	Expected           struct {
		Kind             string   `json:"kind"`
		MessageContains  string   `json:"messageContains"`
		FallbackContains []string `json:"fallbackContains"`
	} `json:"expected"`
}

func loadWhisperLiveKitFixture(t *testing.T, name string) whisperLiveKitFixture {
	t.Helper()
	var fixture whisperLiveKitFixture
	readFixture(t, filepath.Join("..", "..", "spec", "fixtures", "whisperlivekit", name+".json"), &fixture)
	return fixture
}

func startWhisperLiveKit(t *testing.T, fixture whisperLiveKitFixture, transport *fakeTransport) *whisperLiveKitASR {
	t.Helper()
	asr := NewWhisperLiveKitASR(WhisperLiveKitOptions{
		URL: fixture.URL, StopTimeout: 30 * time.Millisecond, dial: transport.dial,
	})
	asr.Open(fixture.Direction)
	waitOpened(t, &asr.socketASR)
	return asr
}

func TestWhisperLiveKitContractFullModeReplaysToPartialsAndStables(t *testing.T) {
	fixture := loadWhisperLiveKitFixture(t, "full-mode")
	transport := &fakeTransport{}
	asr := startWhisperLiveKit(t, fixture, transport)

	target, _, _ := transport.attempt(t)
	parsed, err := url.Parse(target)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Scheme+"://"+parsed.Host+parsed.Path != "ws://127.0.0.1:8000/asr" {
		t.Fatalf("url = %s", target)
	}
	if parsed.Query().Get("language") != fixture.Direction.Src || parsed.Query().Get("mode") != "full" {
		t.Fatalf("query = %s", parsed.RawQuery)
	}

	socket := transport.socket(t, 0)
	pcm := make([]int16, 160)
	var stable []string
	for _, frame := range fixture.Frames {
		asr.receive(frame.Data)
		batch, pushErr := asr.Push(pcm, frame.AtMS)
		if pushErr != nil {
			t.Fatalf("push at %d ms: %v", frame.AtMS, pushErr)
		}
		compareFixtureEvents(t, batch, frame.Events)
		for _, event := range batch {
			if event.Type == "stable" {
				stable = append(stable, event.Text)
			}
		}
		if !socket.isClosed() {
			writes := socket.sent()
			if len(writes) == 0 || writes[len(writes)-1].typ != websocket.MessageBinary ||
				string(writes[len(writes)-1].data) != string(pcmBytes(pcm)) {
				t.Fatalf("audio frame at %d ms was not sent", frame.AtMS)
			}
		}
	}
	if strings.Join(stable, " ") != fixture.ExpectedStableText {
		t.Fatalf("stable text = %q, want %q", strings.Join(stable, " "), fixture.ExpectedStableText)
	}
}

func TestWhisperLiveKitContractEndpointNormalisesHTTPAndTheASRPath(t *testing.T) {
	for _, testCase := range []struct {
		base string
		want string
	}{
		{"http://wlk.test:8000", "ws://wlk.test:8000/asr"},
		{"http://wlk.test:8000/", "ws://wlk.test:8000/asr"},
		{"https://wlk.test/asr/", "wss://wlk.test/asr"},
		{"ws://wlk.test:8000/prefix", "ws://wlk.test:8000/prefix/asr"},
	} {
		transport := &fakeTransport{}
		asr := NewWhisperLiveKitASR(WhisperLiveKitOptions{URL: testCase.base, dial: transport.dial})
		asr.Open(protocol.Direction{Src: "ja", Tgt: "zh"})
		waitOpened(t, &asr.socketASR)
		target, _, _ := transport.attempt(t)
		parsed, _ := url.Parse(target)
		got := parsed.Scheme + "://" + parsed.Host + parsed.Path
		if got != testCase.want {
			t.Fatalf("%s: endpoint = %s, want %s", testCase.base, got, testCase.want)
		}
		if parsed.Query().Get("language") != "ja" || parsed.Query().Get("mode") != "full" {
			t.Fatalf("%s: query = %s", testCase.base, parsed.RawQuery)
		}
	}
}

func TestWhisperLiveKitContractEndOfStreamWaitsForReadyToStop(t *testing.T) {
	transport := &fakeTransport{}
	asr := NewWhisperLiveKitASR(WhisperLiveKitOptions{URL: "ws://wlk.test:8000", StopTimeout: 30 * time.Millisecond, dial: transport.dial})
	asr.Open(protocol.Direction{Src: "ja", Tgt: "zh"})
	waitOpened(t, &asr.socketASR)
	if _, err := asr.Push(make([]int16, 160), 10); err != nil {
		t.Fatal(err)
	}

	asr.Close()
	socket := transport.socket(t, 0)
	writes := socket.sent()
	last := writes[len(writes)-1]
	if last.typ != websocket.MessageBinary || len(last.data) != 0 {
		t.Fatalf("end-of-audio frame = %+v", last)
	}
	if socket.isClosed() {
		t.Fatal("the socket must stay open until ready_to_stop")
	}

	asr.receive([]byte(`{"type":"ready_to_stop"}`))
	if !socket.isClosed() {
		t.Fatal("ready_to_stop must close the socket")
	}
}

func TestWhisperLiveKitContractAShortStopTimeoutClosesTheSocket(t *testing.T) {
	transport := &fakeTransport{}
	asr := NewWhisperLiveKitASR(WhisperLiveKitOptions{URL: "ws://wlk.test:8000", StopTimeout: 20 * time.Millisecond, dial: transport.dial})
	asr.Open(protocol.Direction{Src: "ja", Tgt: "zh"})
	waitOpened(t, &asr.socketASR)
	if _, err := asr.Push(make([]int16, 160), 10); err != nil {
		t.Fatal(err)
	}
	asr.Close()
	socket := transport.socket(t, 0)
	waitFor(t, "the stop timeout", socket.isClosed)
}

func TestWhisperLiveKitContractAudioWaitsForTheSocketToOpen(t *testing.T) {
	transport := &fakeTransport{hold: make(chan struct{})}
	asr := NewWhisperLiveKitASR(WhisperLiveKitOptions{URL: "ws://wlk.test:8000", dial: transport.dial})
	asr.Open(protocol.Direction{Src: "zh", Tgt: "ja"})
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
	if string(socket.sent()[0].data) != string(pcmBytes(pcm)) {
		t.Fatal("queued audio was not sent as-is")
	}
}

func TestWhisperLiveKitContractInBandErrorFrameMapsToProtocolError(t *testing.T) {
	fixture := loadWhisperLiveKitFixture(t, "in-band-error")
	transport := &fakeTransport{}
	asr := startWhisperLiveKit(t, fixture, transport)

	asr.receive(fixture.Frame)
	_, err := asr.Push(nil, fixture.FrameAtMS)
	providerError := requireProviderError(t, err, fixture.Expected.Kind, fixture.Expected.MessageContains, "")
	for _, hint := range fixture.Expected.FallbackContains {
		if !strings.Contains(providerError.Fallback, hint) {
			t.Fatalf("fallback %q does not contain %q", providerError.Fallback, hint)
		}
	}
}

func TestWhisperLiveKitContractReopenClosesThePreviousSocket(t *testing.T) {
	transport := &fakeTransport{}
	asr := NewWhisperLiveKitASR(WhisperLiveKitOptions{URL: "ws://wlk.test:8000", dial: transport.dial})
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

func TestWhisperLiveKitContractConnectionCloseSurfacesAnUnreachableError(t *testing.T) {
	transport := &fakeTransport{}
	asr := NewWhisperLiveKitASR(WhisperLiveKitOptions{URL: "ws://wlk.test:8000", dial: transport.dial})
	asr.Open(protocol.Direction{Src: "ja", Tgt: "zh"})
	waitOpened(t, &asr.socketASR)
	transport.socket(t, 0).CloseNow()

	waitFor(t, "the close error", func() bool {
		asr.mu.Lock()
		defer asr.mu.Unlock()
		return asr.failure != nil
	})
	_, err := asr.Push(nil, 0)
	requireProviderError(t, err, "unreachable", "WhisperLiveKit connection closed", "WHISPERLIVEKIT_URL")
}

func TestWhisperLiveKitContractConfigWithoutRawPCMFails(t *testing.T) {
	transport := &fakeTransport{}
	asr := NewWhisperLiveKitASR(WhisperLiveKitOptions{URL: "ws://wlk.test:8000", dial: transport.dial})
	asr.Open(protocol.Direction{Src: "ja", Tgt: "zh"})
	waitOpened(t, &asr.socketASR)

	asr.receive([]byte(`{"type":"config","useAudioWorklet":false}`))
	_, err := asr.Push(nil, 0)
	requireProviderError(t, err, "config", "useAudioWorklet=false", "--pcm-input")
}

func TestWhisperLiveKitContractOpenRefusesADirectionWithoutSource(t *testing.T) {
	transport := &fakeTransport{}
	asr := NewWhisperLiveKitASR(WhisperLiveKitOptions{URL: "ws://wlk.test:8000", dial: transport.dial})
	asr.Open(protocol.Direction{})

	_, err := asr.Push(nil, 0)
	requireProviderError(t, err, "protocol", "source language", "")
	if transport.calls != 0 {
		t.Fatal("a missing source language must not dial")
	}
}

func TestWhisperLiveKitContractAnUnreachableServerNamesTheURLAndFallbacks(t *testing.T) {
	transport := &fakeTransport{err: errHandshake}
	asr := NewWhisperLiveKitASR(WhisperLiveKitOptions{URL: "ws://wlk.test:8000", dial: transport.dial})
	asr.Open(protocol.Direction{Src: "ja", Tgt: "zh"})
	waitFor(t, "the dial failure", func() bool {
		asr.mu.Lock()
		defer asr.mu.Unlock()
		return asr.failure != nil
	})

	_, err := asr.Push(nil, 0)
	providerError := requireProviderError(t, err, "unreachable", "WhisperLiveKit connection failed", "")
	for _, hint := range []string{"WHISPERLIVEKIT_URL", "--profile local", "ASR_PROVIDER=mock"} {
		if !strings.Contains(providerError.Fallback, hint) {
			t.Fatalf("fallback %q does not contain %q", providerError.Fallback, hint)
		}
	}
}

func TestWhisperLiveKitContractCloseAbortsAPendingDial(t *testing.T) {
	transport := &fakeTransport{hold: make(chan struct{})}
	asr := NewWhisperLiveKitASR(WhisperLiveKitOptions{URL: "ws://wlk.test:8000", dial: transport.dial})
	asr.Open(protocol.Direction{Src: "ja", Tgt: "zh"})
	asr.Close()
	waitDialReturned(t, transport)
}
