package host_test

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"

	"github.com/wray-lee/parley"
	"github.com/wray-lee/parley/internal/host"
	"github.com/wray-lee/parley/internal/protocol"
)

func startServer(t *testing.T, values map[string]string) (*host.Server, int, *logSink) {
	t.Helper()
	return startServerWith(t, values, host.Options{})
}

func startServerWith(t *testing.T, values map[string]string, options host.Options) (*host.Server, int, *logSink) {
	t.Helper()
	options.Loopback = true // test servers stay off the host firewall
	values["PORT"] = "0"
	sink := &logSink{records: make(chan protocol.SegmentLog, 16)}
	options.SegmentLog = sink.log
	options.Warn = sink.warn
	server, err := host.New(host.LoadConfig(env(values)), parley.Assets, options)
	if err != nil {
		t.Fatalf("host.New: %v", err)
	}
	if err := server.Listen(); err != nil {
		t.Fatalf("Listen: %v", err)
	}
	t.Cleanup(func() { _ = server.Close() })
	return server, server.Port(), sink
}

type logSink struct {
	mu       sync.Mutex
	records  chan protocol.SegmentLog
	warnings []string
}

func (s *logSink) log(record protocol.SegmentLog) { s.records <- record }

func (s *logSink) warn(line string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.warnings = append(s.warnings, line)
}

func (s *logSink) warningLines() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string{}, s.warnings...)
}

func get(t *testing.T, url string) (*http.Response, string) {
	t.Helper()
	response, err := http.Get(url)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read %s: %v", url, err)
	}
	return response, string(body)
}

func TestHealthzAndStaticClient(t *testing.T) {
	_, port, _ := startServer(t, map[string]string{})
	base := fmt.Sprintf("http://127.0.0.1:%d", port)

	response, body := get(t, base+"/healthz")
	if response.StatusCode != 200 || body != `{"ok":true}` || response.Header.Get("content-type") != "application/json; charset=utf-8" {
		t.Fatalf("healthz = %d %q %v", response.StatusCode, body, response.Header)
	}

	response, body = get(t, base+"/")
	if response.StatusCode != 200 || !strings.Contains(body, "DEMO — mock providers") || !strings.HasPrefix(response.Header.Get("content-type"), "text/html") {
		t.Fatalf("index = %d %q", response.StatusCode, body[:min(len(body), 80)])
	}

	response, body = get(t, base+"/samples/placeholder.json")
	if response.StatusCode != 200 || !strings.Contains(body, `"id": "placeholder"`) {
		t.Fatalf("sample = %d %q", response.StatusCode, body)
	}
	if response, _ = get(t, base+"/samples//placeholder.json"); response.StatusCode != 200 {
		t.Fatalf("double-slash sample = %d, want 200 like the Bun host", response.StatusCode)
	}
	if response, _ = get(t, base+"//index.html"); response.StatusCode != 200 {
		t.Fatalf("double-slash index = %d, want 200 like the Bun host", response.StatusCode)
	}

	if response, _ = get(t, base+"/samples"); response.StatusCode != 404 {
		t.Fatalf("/samples = %d, want 404", response.StatusCode)
	}
	if response, body = get(t, base+"/nope.js"); response.StatusCode != 404 || body != "not found" {
		t.Fatalf("missing = %d %q", response.StatusCode, body)
	}
	if response, _ = get(t, base+"/samples/../config/pairs.json"); response.StatusCode != 404 {
		t.Fatalf("traversal = %d, want 404", response.StatusCode)
	}

	request, _ := http.NewRequest(http.MethodPost, base+"/healthz", nil)
	post, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	post.Body.Close()
	if post.StatusCode != 405 {
		t.Fatalf("POST healthz = %d, want 405", post.StatusCode)
	}
}

func TestAccessTokenRejectsHandshake(t *testing.T) {
	token := "fake-token-not-real"
	_, port, _ := startServer(t, map[string]string{"ACCESS_TOKEN": token})
	url := fmt.Sprintf("ws://127.0.0.1:%d/ws", port)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	conn, response, err := websocket.Dial(ctx, url, nil)
	if conn != nil {
		conn.CloseNow()
	}
	if err == nil || response == nil || response.StatusCode != 401 {
		t.Fatalf("missing token: err=%v response=%v", err, response)
	}
	body, _ := io.ReadAll(response.Body)
	response.Body.Close()
	if strings.Contains(string(body), token) {
		t.Fatal("the response must not echo the token")
	}
	if strings.TrimSpace(string(body)) != "access token required" {
		t.Fatalf("body = %q", body)
	}

	conn, response, err = websocket.Dial(ctx, url+"?token=wrong", nil)
	if conn != nil {
		conn.CloseNow()
	}
	if err == nil || response == nil || response.StatusCode != 401 {
		t.Fatalf("wrong token: err=%v response=%v", err, response)
	}
	response.Body.Close()

	conn, _, err = websocket.Dial(ctx, url+"?token="+token, nil)
	if err != nil {
		t.Fatalf("valid token: %v", err)
	}
	defer conn.CloseNow()
	var hello protocol.Hello
	readJSON(t, ctx, conn, &hello)
	if hello.Type != "hello" {
		t.Fatalf("hello = %+v", hello)
	}
}

// The Device Host's Loopback Host sets RejectOrigin: a request from a browser
// page must not reach the core, while a native client without Origin does.
func TestRejectOriginBlocksRequestsCarryingOne(t *testing.T) {
	_, port, _ := startServerWith(t, map[string]string{}, host.Options{RejectOrigin: true})
	base := fmt.Sprintf("http://127.0.0.1:%d", port)

	request, err := http.NewRequest(http.MethodGet, base+"/healthz", nil)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Origin", "https://evil.example")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusForbidden {
		t.Fatalf("healthz with Origin = %d, want 403", response.StatusCode)
	}

	if response, body := get(t, base+"/healthz"); response.StatusCode != 200 || body != `{"ok":true}` {
		t.Fatalf("healthz without Origin = %d %q", response.StatusCode, body)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	url := fmt.Sprintf("ws://127.0.0.1:%d/ws", port)
	conn, response, err := websocket.Dial(ctx, url, &websocket.DialOptions{
		HTTPHeader: http.Header{"Origin": []string{"https://evil.example"}},
	})
	if conn != nil {
		conn.CloseNow()
	}
	if err == nil || response == nil || response.StatusCode != http.StatusForbidden {
		t.Fatalf("websocket with Origin: err=%v response=%v", err, response)
	}
	response.Body.Close()

	conn, _, err = websocket.Dial(ctx, url, nil)
	if err != nil {
		t.Fatalf("websocket without Origin: %v", err)
	}
	conn.CloseNow()
}

func TestDemoSessionOverWebSocket(t *testing.T) {
	_, port, sink := startServer(t, map[string]string{})
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	conn, _, err := websocket.Dial(ctx, fmt.Sprintf("ws://127.0.0.1:%d/ws", port), nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.CloseNow()

	var hello protocol.Hello
	readJSON(t, ctx, conn, &hello)
	if !hello.Demo || len(hello.Pairs) != 1 || len(hello.Packs) != 2 || hello.PresetsInUse == nil {
		t.Fatalf("hello = %+v", hello)
	}
	if hello.Protocol != 1 {
		t.Fatalf("hello.protocol = %d, want 1", hello.Protocol)
	}
	if hello.TTS != "audio" {
		t.Fatalf("hello.tts = %q, want audio for the mock TTS", hello.TTS)
	}
	if hello.Pairs[0].A != "ja" || hello.Pairs[0].B != "zh" || !hello.Pairs[0].Converse {
		t.Fatalf("pairs = %+v", hello.Pairs)
	}

	writeJSON(t, ctx, conn, protocol.ClientMessage{Type: "start", Mode: "listen", Src: "zh", Tgt: "ja", Packs: []string{"conference"}})
	writeJSON(t, ctx, conn, protocol.ClientMessage{Type: "sample", Name: "placeholder"})
	for i := 0; i < 80; i++ {
		writePCM(t, ctx, conn, 6000)
	}
	for i := 0; i < 25; i++ {
		writePCM(t, ctx, conn, 0)
	}

	var segment *protocol.SegmentEvent
	binaryChunks := 0
	ttsEnd := false
	deadline := time.Now().Add(10 * time.Second)
	for (!ttsEnd || segment == nil) && time.Now().Before(deadline) {
		messageType, data, err := conn.Read(ctx)
		if err != nil {
			t.Fatalf("read: %v", err)
		}
		if messageType == websocket.MessageBinary {
			binaryChunks++
			continue
		}
		var event struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal(data, &event); err != nil {
			t.Fatalf("event: %v", err)
		}
		switch event.Type {
		case "segment":
			var candidate protocol.SegmentEvent
			if err := json.Unmarshal(data, &candidate); err != nil {
				t.Fatal(err)
			}
			segment = &candidate
		case "tts_end":
			ttsEnd = true
		}
	}
	if segment == nil {
		t.Fatal("no segment event")
	}
	if segment.SegID != 1 || segment.Src != "你好，这是演示。" || segment.Tgt != "[DEMO] こんにちは、これはデモです。" {
		t.Fatalf("segment = %+v", segment)
	}
	if !segment.Demo || !segment.Speak {
		t.Fatalf("segment flags = %+v", segment)
	}
	select {
	case record := <-sink.records:
		if record.SegID != 1 || record.ASRFinal != "你好，这是演示。" {
			t.Fatalf("log = %+v", record)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("no segment log line")
	}
	if binaryChunks == 0 {
		t.Fatal("no binary tts chunk")
	}
}

func TestHelloDeclaresSystemTTSForTheSystemProvider(t *testing.T) {
	_, port, _ := startServer(t, map[string]string{"TTS_PROVIDER": "system"})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	conn, _, err := websocket.Dial(ctx, fmt.Sprintf("ws://127.0.0.1:%d/ws", port), nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.CloseNow()

	var hello protocol.Hello
	readJSON(t, ctx, conn, &hello)
	if hello.Protocol != 1 {
		t.Fatalf("hello.protocol = %d, want 1", hello.Protocol)
	}
	if hello.TTS != "system" {
		t.Fatalf("hello.tts = %q, want system", hello.TTS)
	}
}

func TestHelloReportsTheConfiguredPreset(t *testing.T) {
	const apiKey = "fake-secret-key-not-real"
	_, port, _ := startServer(t, map[string]string{
		"LLM_PRESET": "gemini", "LLM_API_KEY": apiKey, "LLM_MODEL": "fake-model",
	})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	conn, _, err := websocket.Dial(ctx, fmt.Sprintf("ws://127.0.0.1:%d/ws", port), nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.CloseNow()

	_, data, err := conn.Read(ctx)
	if err != nil {
		t.Fatalf("read hello: %v", err)
	}
	var hello protocol.Hello
	if err := json.Unmarshal(data, &hello); err != nil {
		t.Fatalf("decode hello: %v", err)
	}
	if len(hello.PresetsInUse) != 1 || hello.PresetsInUse[0].ID != "gemini" || !hello.PresetsInUse[0].TrainsOnData {
		t.Fatalf("presetsInUse = %+v, want [{gemini true}]", hello.PresetsInUse)
	}
	if strings.Contains(string(data), apiKey) {
		t.Fatal("hello must not carry the API key")
	}
}

func TestInvalidLLMConfigIsAPerConnectionError(t *testing.T) {
	// The JavaScript host starts listening and sends this error on the
	// connection it cannot serve; it does not exit at startup.
	cases := []struct {
		name string
		env  map[string]string
	}{
		{"unknown preset", map[string]string{"LLM_PRESET": "not-a-preset", "LLM_API_KEY": "fake-key"}},
		{"bad base URL", map[string]string{"LLM_PRESET": "custom", "LLM_BASE_URL": "not a url", "LLM_MODEL": "m"}},
		{"bad extra body", map[string]string{"LLM_PRESET": "custom", "LLM_BASE_URL": "https://gateway.example/v1", "LLM_MODEL": "m", "LLM_EXTRA_BODY": "{not json"}},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			_, port, _ := startServer(t, test.env)
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			conn, _, err := websocket.Dial(ctx, fmt.Sprintf("ws://127.0.0.1:%d/ws", port), nil)
			if err != nil {
				t.Fatalf("dial: %v", err)
			}
			defer conn.CloseNow()
			var event protocol.ErrorEvent
			readJSON(t, ctx, conn, &event)
			if event.Type != "error" || event.Kind != "protocol" ||
				!strings.HasPrefix(event.Message, "could not start the Session: ") {
				t.Fatalf("event = %+v", event)
			}
		})
	}
}

func TestOpenAITranslatorOverWebSocket(t *testing.T) {
	requests := make(chan map[string]any, 4)
	llm := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			http.NotFound(w, r)
			return
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("request body: %v", err)
			return
		}
		requests <- body
		w.Header().Set("content-type", "text/event-stream")
		fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"こんにちは\"}}]}\n\ndata: [DONE]\n\n")
	}))
	defer llm.Close()

	_, port, _ := startServer(t, map[string]string{
		"LLM_PRESET": "custom", "LLM_BASE_URL": llm.URL + "/v1", "LLM_MODEL": "test-model",
	})
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	conn, _, err := websocket.Dial(ctx, fmt.Sprintf("ws://127.0.0.1:%d/ws", port), nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.CloseNow()
	var hello protocol.Hello
	readJSON(t, ctx, conn, &hello)

	writeJSON(t, ctx, conn, protocol.ClientMessage{Type: "start", Mode: "listen", Src: "zh", Tgt: "ja"})
	writeJSON(t, ctx, conn, protocol.ClientMessage{Type: "sample", Name: "placeholder"})
	for i := 0; i < 80; i++ {
		writePCM(t, ctx, conn, 6000)
	}
	for i := 0; i < 25; i++ {
		writePCM(t, ctx, conn, 0)
	}

	var segment *protocol.SegmentEvent
	deadline := time.Now().Add(10 * time.Second)
	for segment == nil && time.Now().Before(deadline) {
		messageType, data, err := conn.Read(ctx)
		if err != nil {
			t.Fatalf("read: %v", err)
		}
		if messageType != websocket.MessageText {
			continue
		}
		var event struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal(data, &event); err != nil {
			t.Fatal(err)
		}
		if event.Type != "segment" {
			continue
		}
		var candidate protocol.SegmentEvent
		if err := json.Unmarshal(data, &candidate); err != nil {
			t.Fatal(err)
		}
		segment = &candidate
	}
	if segment == nil || segment.Tgt != "こんにちは" {
		t.Fatalf("segment = %+v", segment)
	}

	select {
	case body := <-requests:
		if body["model"] != "test-model" || body["stream"] != true {
			t.Fatalf("request body = %v", body)
		}
		messages := body["messages"].([]any)
		if len(messages) != 2 || messages[0].(map[string]any)["role"] != "system" ||
			!strings.HasPrefix(messages[1].(map[string]any)["content"].(string), "[已译上下文]") {
			t.Fatalf("messages = %v", messages)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the translator endpoint was never called")
	}
}

func TestUnsetTokenWarnsOnce(t *testing.T) {
	_, _, sink := startServer(t, map[string]string{})
	warnings := sink.warningLines()
	if len(warnings) != 1 || !strings.Contains(warnings[0], "ACCESS_TOKEN") {
		t.Fatalf("warnings = %+v", warnings)
	}
}

func TestUnsupportedProviderIsAnExplicitStartupError(t *testing.T) {
	_, err := host.New(host.LoadConfig(env(map[string]string{"ASR_PROVIDER": "no-such-provider"})), parley.Assets, host.Options{})
	if err == nil || !strings.Contains(err.Error(), "ASR_PROVIDER") || !strings.Contains(err.Error(), "mock") {
		t.Fatalf("err = %v", err)
	}
}

func TestUnknownSampleIsAProtocolError(t *testing.T) {
	_, port, _ := startServer(t, map[string]string{})
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	conn, _, err := websocket.Dial(ctx, fmt.Sprintf("ws://127.0.0.1:%d/ws", port), nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.CloseNow()

	var hello protocol.Hello
	readJSON(t, ctx, conn, &hello)
	writeJSON(t, ctx, conn, protocol.ClientMessage{Type: "start", Mode: "listen", Src: "zh", Tgt: "ja"})
	writeJSON(t, ctx, conn, protocol.ClientMessage{Type: "sample", Name: "nope"})

	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		messageType, data, err := conn.Read(ctx)
		if err != nil {
			t.Fatalf("read: %v", err)
		}
		if messageType != websocket.MessageText {
			continue
		}
		var event struct {
			Type    string `json:"type"`
			Kind    string `json:"kind"`
			Message string `json:"message"`
		}
		if err := json.Unmarshal(data, &event); err != nil {
			t.Fatal(err)
		}
		if event.Type != "error" {
			continue
		}
		if event.Kind != "protocol" || !strings.Contains(event.Message, `unknown sample "nope"`) {
			t.Fatalf("error = %+v", event)
		}
		return
	}
	t.Fatal("no protocol error for an unknown sample")
}

func TestSampleSelectionDoesNotLeakBetweenSessions(t *testing.T) {
	_, port, _ := startServer(t, map[string]string{})
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	dial := func() *websocket.Conn {
		t.Helper()
		conn, _, err := websocket.Dial(ctx, fmt.Sprintf("ws://127.0.0.1:%d/ws", port), nil)
		if err != nil {
			t.Fatalf("dial: %v", err)
		}
		var hello protocol.Hello
		readJSON(t, ctx, conn, &hello)
		return conn
	}

	first := dial()
	defer first.CloseNow()
	writeJSON(t, ctx, first, protocol.ClientMessage{Type: "start", Mode: "listen", Src: "ja", Tgt: "zh"})
	writeJSON(t, ctx, first, protocol.ClientMessage{Type: "sample", Name: "ja-held"})
	for i := 0; i < 140; i++ {
		writePCM(t, ctx, first, 6000)
	}
	for i := 0; i < 60; i++ {
		writePCM(t, ctx, first, 0)
	}
	// A committed Segment proves the first Session applied its own sample.
	if segment := readSegment(t, ctx, first); !strings.Contains(segment.Src, "この報告書は") {
		t.Fatalf("first session segment = %+v", segment)
	}

	second := dial()
	defer second.CloseNow()
	writeJSON(t, ctx, second, protocol.ClientMessage{Type: "start", Mode: "listen", Src: "zh", Tgt: "ja"})
	// The v0.0.3 path: no name, so this Session keeps its own sample.
	writeJSON(t, ctx, second, protocol.ClientMessage{Type: "sample"})
	for i := 0; i < 80; i++ {
		writePCM(t, ctx, second, 6000)
	}
	for i := 0; i < 25; i++ {
		writePCM(t, ctx, second, 0)
	}
	segment := readSegment(t, ctx, second)
	if segment.Src != "你好，这是演示。" || segment.Tgt != "[DEMO] こんにちは、これはデモです。" {
		t.Fatalf("second session segment = %+v (the first session's sample leaked)", segment)
	}
}

func readSegment(t *testing.T, ctx context.Context, conn *websocket.Conn) *protocol.SegmentEvent {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		messageType, data, err := conn.Read(ctx)
		if err != nil {
			t.Fatalf("read: %v", err)
		}
		if messageType != websocket.MessageText {
			continue
		}
		var event struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal(data, &event); err != nil {
			t.Fatal(err)
		}
		if event.Type != "segment" {
			continue
		}
		var segment protocol.SegmentEvent
		if err := json.Unmarshal(data, &segment); err != nil {
			t.Fatal(err)
		}
		return &segment
	}
	t.Fatal("no segment")
	return nil
}

func readJSON(t *testing.T, ctx context.Context, conn *websocket.Conn, value any) {
	t.Helper()
	messageType, data, err := conn.Read(ctx)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if messageType != websocket.MessageText {
		t.Fatalf("expected text message, got %v", messageType)
	}
	if err := json.Unmarshal(data, value); err != nil {
		t.Fatalf("decode: %v (%s)", err, data)
	}
}

func writeJSON(t *testing.T, ctx context.Context, conn *websocket.Conn, value any) {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	if err := conn.Write(ctx, websocket.MessageText, data); err != nil {
		t.Fatalf("write: %v", err)
	}
}

func writePCM(t *testing.T, ctx context.Context, conn *websocket.Conn, amplitude int16) {
	t.Helper()
	frame := make([]byte, 320*2)
	for i := 0; i < 320; i++ {
		value := amplitude
		if amplitude != 0 {
			value = int16(float64(amplitude) * (0.8 + 0.2*float64(i%10)/10))
		}
		binary.LittleEndian.PutUint16(frame[i*2:], uint16(value))
	}
	if err := conn.Write(ctx, websocket.MessageBinary, frame); err != nil {
		t.Fatalf("write pcm: %v", err)
	}
}
