package providers

import (
	"context"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"

	"github.com/wray-lee/parley/internal/protocol"
)

// Mirrors tests/contract/tts-edge.test.js: replay a recorded Edge read-aloud
// session through the adapter and assert the handshake, frames and mp3 bytes.

const edgeVectorToken = "461B5A82FDC12AF5-A89B0EBF2C158E7B404FF47B1F12D5579CB3C7D7B18EC8ADE4A4AD0A8036C8DE"

var edgeVectorTime = time.Date(2026, 10, 13, 10, 0, 30, 0, time.UTC)

type edgeFixtureFrame struct {
	Text   *string `json:"text"`
	Binary *string `json:"binary"`
}

func loadEdgeFrames(t *testing.T, name string) []edgeFrame {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "spec", "fixtures", "edge", name+".frames.json"))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	var fixture []edgeFixtureFrame
	if err := json.Unmarshal(data, &fixture); err != nil {
		t.Fatalf("parse fixture: %v", err)
	}
	frames := make([]edgeFrame, 0, len(fixture))
	for _, entry := range fixture {
		if entry.Text != nil {
			frames = append(frames, edgeFrame{typ: websocket.MessageText, data: []byte(*entry.Text)})
			continue
		}
		payload, err := base64.StdEncoding.DecodeString(*entry.Binary)
		if err != nil {
			t.Fatalf("decode binary frame: %v", err)
		}
		frames = append(frames, edgeFrame{typ: websocket.MessageBinary, data: payload})
	}
	return frames
}

func edgeAudioFrom(frames []edgeFrame) []byte {
	var parts [][]byte
	for _, frame := range frames {
		if frame.typ != websocket.MessageBinary || len(frame.data) < 2 {
			continue
		}
		headerLength := int(binary.BigEndian.Uint16(frame.data))
		if len(frame.data) < 2+headerLength {
			continue
		}
		if strings.Contains(string(frame.data[2:2+headerLength]), "Path:audio") {
			parts = append(parts, frame.data[2+headerLength:])
		}
	}
	return concatBytes(parts)
}

type edgeFrame struct {
	typ  websocket.MessageType
	data []byte
}

type scriptedEdgeConn struct {
	mu      sync.Mutex
	writes  []fakeWrite
	frames  []edgeFrame
	readErr error
	block   bool
}

func (c *scriptedEdgeConn) Read(ctx context.Context) (websocket.MessageType, []byte, error) {
	c.mu.Lock()
	if len(c.frames) > 0 {
		frame := c.frames[0]
		c.frames = c.frames[1:]
		c.mu.Unlock()
		return frame.typ, frame.data, nil
	}
	readErr, block := c.readErr, c.block
	c.mu.Unlock()
	if !block {
		return 0, nil, readErr
	}
	<-ctx.Done()
	return 0, nil, ctx.Err()
}

func (c *scriptedEdgeConn) Write(_ context.Context, typ websocket.MessageType, data []byte) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.writes = append(c.writes, fakeWrite{typ: typ, data: append([]byte(nil), data...)})
	return nil
}

func (c *scriptedEdgeConn) Close(websocket.StatusCode, string) error { return nil }

func (c *scriptedEdgeConn) sent() []fakeWrite {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]fakeWrite(nil), c.writes...)
}

type edgeDialCall struct {
	url     string
	options *websocket.DialOptions
}

type fakeEdgeDial struct {
	calls    []edgeDialCall
	conn     edgeConn
	response *http.Response
	err      error
}

func (f *fakeEdgeDial) dial(_ context.Context, url string, options *websocket.DialOptions) (edgeConn, *http.Response, error) {
	f.calls = append(f.calls, edgeDialCall{url: url, options: options})
	return f.conn, f.response, f.err
}

func newEdgeForTest(t *testing.T, frames []edgeFrame, options EdgeOptions) (*EdgeTTS, *scriptedEdgeConn, *fakeEdgeDial) {
	t.Helper()
	conn := &scriptedEdgeConn{frames: frames, readErr: io.EOF}
	dial := &fakeEdgeDial{conn: conn}
	options.Dial = dial.dial
	if options.Now == nil {
		options.Now = func() time.Time { return edgeVectorTime }
	}
	if options.Warn == nil {
		options.Warn = func(string) {}
	}
	return NewEdgeTTS(options), conn, dial
}

func TestEdgeReplaysTheRecordedJaSession(t *testing.T) {
	frames := loadEdgeFrames(t, "ja")
	expected := edgeAudioFrom(frames)
	tts, conn, dial := newEdgeForTest(t, frames, EdgeOptions{
		Voices: map[string][]string{"ja": {"ja-JP-NanamiNeural"}},
	})

	audio, err := tts.Synthesize(protocol.TTSRequest{
		Text: "こんにちは。", SegID: 1, Direction: protocol.Direction{Src: "zh", Tgt: "ja"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if tts.Name() != "edge" || audio.Format != "mp3" || !reflect.DeepEqual(audio.Data, expected) {
		t.Fatalf("audio = %s %d bytes, want %d", audio.Format, len(audio.Data), len(expected))
	}

	if len(dial.calls) != 1 {
		t.Fatalf("dial calls = %d", len(dial.calls))
	}
	call := dial.calls[0]
	for _, want := range []string{
		"wss://speech.platform.bing.com/",
		"Sec-MS-GEC=" + edgeVectorToken,
		"Sec-MS-GEC-Version=2-",
		"TrustedClientToken=6A5AA1D4EAFF4E9FB37E23D68491D6F4",
	} {
		if !strings.Contains(call.url, want) {
			t.Fatalf("url = %q, missing %q", call.url, want)
		}
	}
	if call.options == nil ||
		call.options.HTTPHeader.Get("Origin") != "chrome-extension://jdiccldimpdaibmpdkjnbmckianbfold" {
		t.Fatalf("handshake headers = %+v", call.options)
	}

	sent := conn.sent()
	if len(sent) != 2 || sent[0].typ != websocket.MessageText || sent[1].typ != websocket.MessageText {
		t.Fatalf("sent %d frames", len(sent))
	}
	config := string(sent[0].data)
	if !strings.Contains(config, "Path:speech.config") ||
		!strings.Contains(config, "audio-24khz-48kbitrate-mono-mp3") ||
		!strings.Contains(config, "X-Timestamp:2026-10-13T10:00:30.000Z") {
		t.Fatalf("config frame = %q", config)
	}
	ssml := string(sent[1].data)
	for _, want := range []string{
		"Path:ssml", "voice name='ja-JP-NanamiNeural'", "xml:lang='ja-JP'", "こんにちは。",
	} {
		if !strings.Contains(ssml, want) {
			t.Fatalf("ssml frame = %q, missing %q", ssml, want)
		}
	}
}

func TestEdgeReplaysTheRecordedZhSession(t *testing.T) {
	frames := loadEdgeFrames(t, "zh")
	tts, _, _ := newEdgeForTest(t, frames, EdgeOptions{
		Voices: map[string][]string{"zh": {"zh-CN-XiaoxiaoNeural"}},
	})
	audio, err := tts.Synthesize(protocol.TTSRequest{
		Text: "你好。", SegID: 2, Direction: protocol.Direction{Src: "ja", Tgt: "zh"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(audio.Data, edgeAudioFrom(frames)) {
		t.Fatal("audio differs from the zh fixture")
	}
}

func TestEdgeErrorsNameTheFallback(t *testing.T) {
	t.Run("rejected handshake", func(t *testing.T) {
		tts, _, dial := newEdgeForTest(t, nil, EdgeOptions{Voices: map[string][]string{"ja": {"ja-JP-NanamiNeural"}}})
		dial.conn = nil
		dial.response = &http.Response{StatusCode: http.StatusForbidden}
		dial.err = errors.New("bad handshake")
		_, err := tts.Synthesize(protocol.TTSRequest{Text: "x", Direction: protocol.Direction{Tgt: "ja"}})
		if err == nil || !strings.Contains(err.Error(), "HTTP 403") || !strings.Contains(err.Error(), "kokoro or system") {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("connection failure", func(t *testing.T) {
		tts, _, dial := newEdgeForTest(t, nil, EdgeOptions{Voices: map[string][]string{"ja": {"ja-JP-NanamiNeural"}}})
		dial.conn = nil
		dial.err = errors.New("ECONNRESET")
		_, err := tts.Synthesize(protocol.TTSRequest{Text: "x", Direction: protocol.Direction{Tgt: "ja"}})
		if err == nil || !strings.Contains(err.Error(), "connection failed") || !strings.Contains(err.Error(), "kokoro or system") {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("closed before turn.end", func(t *testing.T) {
		tts, conn, _ := newEdgeForTest(t, nil, EdgeOptions{Voices: map[string][]string{"ja": {"ja-JP-NanamiNeural"}}})
		conn.readErr = websocket.CloseError{Code: websocket.StatusAbnormalClosure}
		_, err := tts.Synthesize(protocol.TTSRequest{Text: "x", Direction: protocol.Direction{Tgt: "ja"}})
		if err == nil || !strings.Contains(err.Error(), "closed the connection") ||
			!strings.Contains(err.Error(), "1006") {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("malformed audio frame", func(t *testing.T) {
		frames := []edgeFrame{
			{typ: websocket.MessageBinary, data: []byte{1}},
			{typ: websocket.MessageText, data: []byte("Path:turn.end\r\n\r\n")},
		}
		tts, _, _ := newEdgeForTest(t, frames, EdgeOptions{Voices: map[string][]string{"ja": {"ja-JP-NanamiNeural"}}})
		_, err := tts.Synthesize(protocol.TTSRequest{Text: "x", Direction: protocol.Direction{Tgt: "ja"}})
		if err == nil || !strings.Contains(err.Error(), "returned no audio") {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("timeout", func(t *testing.T) {
		tts, conn, _ := newEdgeForTest(t, nil, EdgeOptions{
			Voices: map[string][]string{"ja": {"ja-JP-NanamiNeural"}}, Timeout: 5 * time.Millisecond,
		})
		conn.block = true
		_, err := tts.Synthesize(protocol.TTSRequest{Text: "x", Direction: protocol.Direction{Tgt: "ja"}})
		if err == nil || !strings.Contains(err.Error(), "timed out after 5 ms") ||
			!strings.Contains(err.Error(), "kokoro or system") {
			t.Fatalf("error = %v", err)
		}
	})
}

func TestEdgeFallsBackToTheBaseLanguageVoice(t *testing.T) {
	frames := loadEdgeFrames(t, "ja")
	var warnings []string
	tts, conn, _ := newEdgeForTest(t, frames, EdgeOptions{
		Voices: map[string][]string{"ja": {"ja-JP-NanamiNeural"}},
		Warn:   func(line string) { warnings = append(warnings, line) },
	})
	if _, err := tts.Synthesize(protocol.TTSRequest{
		Text: "こんにちは。", Direction: protocol.Direction{Src: "zh", Tgt: "ja-JP"},
	}); err != nil {
		t.Fatal(err)
	}
	if len(warnings) != 1 || !strings.Contains(warnings[0], `no voice configured for "ja-JP"`) {
		t.Fatalf("warnings = %q", warnings)
	}
	sent := conn.sent()
	if len(sent) != 2 || !strings.Contains(string(sent[1].data), "voice name='ja-JP-NanamiNeural'") {
		t.Fatalf("ssml = %v", sent)
	}
}

func TestEdgeMissingVoiceFailsBeforeDialing(t *testing.T) {
	tts, _, dial := newEdgeForTest(t, nil, EdgeOptions{Voices: map[string][]string{"ja": {"ja-JP-NanamiNeural"}}})
	_, err := tts.Synthesize(protocol.TTSRequest{Text: "x", Direction: protocol.Direction{Tgt: "fr"}})
	if err == nil || !strings.Contains(err.Error(), `no voice configured for target language "fr"`) ||
		!strings.Contains(err.Error(), "kokoro or system") {
		t.Fatalf("error = %v", err)
	}
	if len(dial.calls) != 0 {
		t.Fatal("a missing voice must fail before dialing")
	}
}
