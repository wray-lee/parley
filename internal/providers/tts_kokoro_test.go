package providers

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/wray-lee/parley/internal/protocol"
)

// Mirrors tests/contract/tts-kokoro.test.js: replay the recorded Kokoro wire
// traffic through the adapter and assert the request shape and mp3 bytes.

func kokoroFixture(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "spec", "fixtures", "kokoro", name))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	return data
}

type testKokoroRequest struct {
	Model          string `json:"model"`
	Input          string `json:"input"`
	Voice          string `json:"voice"`
	ResponseFormat string `json:"response_format"`
	Stream         bool   `json:"stream"`
}

func TestKokoroReassemblesTheRecordedChunkedFixture(t *testing.T) {
	expected := kokoroFixture(t, "ja.mp3")
	var requests []testKokoroRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var request testKokoroRequest
		if err := json.Unmarshal(body, &request); err != nil {
			t.Errorf("request body: %v", err)
		}
		requests = append(requests, request)
		w.Header().Set("content-type", "audio/mpeg")
		flusher, _ := w.(http.Flusher)
		for offset := 0; offset < len(expected); offset += 97 {
			end := offset + 97
			if end > len(expected) {
				end = len(expected)
			}
			_, _ = w.Write(expected[offset:end])
			if flusher != nil {
				flusher.Flush()
			}
		}
	}))
	defer server.Close()

	tts := NewKokoroTTS(KokoroOptions{
		KokoroURL: server.URL,
		Voices:    map[string][]string{"ja": {"jf_alpha"}},
	})
	audio, err := tts.Synthesize(protocol.TTSRequest{
		Text: "こんにちは。", SegID: 1, Direction: protocol.Direction{Src: "zh", Tgt: "ja"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if audio.Format != "mp3" || !reflect.DeepEqual(audio.Data, expected) {
		t.Fatalf("audio = %s, %d bytes", audio.Format, len(audio.Data))
	}
	if len(requests) != 1 {
		t.Fatalf("requests = %d", len(requests))
	}
	want := testKokoroRequest{
		Model: "kokoro", Input: "こんにちは。", Voice: "jf_alpha", ResponseFormat: "mp3", Stream: true,
	}
	if !reflect.DeepEqual(requests[0], want) {
		t.Fatalf("request = %+v", requests[0])
	}
}

func TestKokoroSplitsSentencesAtUnicodeSentenceTerminals(t *testing.T) {
	cases := []struct {
		input string
		want  []string
	}{
		{"你好。你好吗？好的！", []string{"你好。", "你好吗？", "好的！"}},
		{"こんにちは。元気ですか？", []string{"こんにちは。", "元気ですか？"}},
		{"no terminal here", []string{"no terminal here"}},
		{"  spaced out.  ", []string{"spaced out."}},
		{"", nil},
	}
	for _, test := range cases {
		if got := splitSentences(test.input); !reflect.DeepEqual(got, test.want) {
			t.Fatalf("splitSentences(%q) = %q, want %q", test.input, got, test.want)
		}
	}
}

func TestKokoroSynthesizesSentenceBySentenceAndKeepsOnlyTheFirstID3Tag(t *testing.T) {
	clip := kokoroFixture(t, "zh.mp3")
	var inputs []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var request testKokoroRequest
		if err := json.Unmarshal(body, &request); err != nil {
			t.Errorf("request body: %v", err)
		}
		inputs = append(inputs, request.Input)
		_, _ = w.Write(clip)
	}))
	defer server.Close()

	tts := NewKokoroTTS(KokoroOptions{
		KokoroURL: server.URL,
		Voices:    map[string][]string{"zh": {"zf_xiaobei"}},
	})
	audio, err := tts.Synthesize(protocol.TTSRequest{
		Text: "第一句。第二句！", SegID: 3, Direction: protocol.Direction{Src: "en", Tgt: "zh"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(inputs, []string{"第一句。", "第二句！"}) {
		t.Fatalf("inputs = %q", inputs)
	}
	tagLength := 10 + (int(clip[6]&0x7f)<<21 | int(clip[7]&0x7f)<<14 | int(clip[8]&0x7f)<<7 | int(clip[9]&0x7f))
	want := append(append([]byte{}, clip...), clip[tagLength:]...)
	if !reflect.DeepEqual(audio.Data, want) {
		t.Fatalf("audio = %d bytes, want %d", len(audio.Data), len(want))
	}
}

func TestKokoroFallsBackToAnotherVoiceOfTheLanguage(t *testing.T) {
	clip := kokoroFixture(t, "ja.mp3")
	var voices []string
	var warnings []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var request testKokoroRequest
		_ = json.Unmarshal(body, &request)
		voices = append(voices, request.Voice)
		if request.Voice == "jf_missing" {
			http.Error(w, "voice not found", http.StatusBadRequest)
			return
		}
		_, _ = w.Write(clip)
	}))
	defer server.Close()

	tts := NewKokoroTTS(KokoroOptions{
		KokoroURL: server.URL,
		Voices:    map[string][]string{"ja": {"jf_missing", "jf_alpha"}},
		Warn:      func(line string) { warnings = append(warnings, line) },
	})
	audio, err := tts.Synthesize(protocol.TTSRequest{
		Text: "こんにちは。", Direction: protocol.Direction{Src: "en", Tgt: "ja"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(voices, []string{"jf_missing", "jf_alpha"}) {
		t.Fatalf("voices = %q", voices)
	}
	if !reflect.DeepEqual(audio.Data, clip) {
		t.Fatal("fallback audio differs from the fixture")
	}
	if len(warnings) != 1 || !strings.Contains(warnings[0], "jf_missing") || !strings.Contains(warnings[0], "jf_alpha") {
		t.Fatalf("warnings = %q", warnings)
	}
}

func TestKokoroErrorsNameTheProviderAndTheFallback(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	url := server.URL
	server.Close()

	tts := NewKokoroTTS(KokoroOptions{KokoroURL: url, Voices: map[string][]string{"ja": {"jf_alpha"}}})
	_, err := tts.Synthesize(protocol.TTSRequest{Text: "x", Direction: protocol.Direction{Tgt: "ja"}})
	if err == nil || !strings.Contains(err.Error(), "Kokoro") || !strings.Contains(err.Error(), "TTS_PROVIDER=system or mock") {
		t.Fatalf("unreachable error = %v", err)
	}

	statusServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusServiceUnavailable)
	}))
	defer statusServer.Close()
	tts = NewKokoroTTS(KokoroOptions{KokoroURL: statusServer.URL, Voices: map[string][]string{"ja": {"jf_alpha"}}})
	_, err = tts.Synthesize(protocol.TTSRequest{Text: "x", Direction: protocol.Direction{Tgt: "ja"}})
	if err == nil || !strings.Contains(err.Error(), "HTTP 503") || !strings.Contains(err.Error(), "boom") ||
		!strings.Contains(err.Error(), "TTS_PROVIDER=system or mock") {
		t.Fatalf("HTTP error = %v", err)
	}
}

func TestKokoroRequiresATargetLanguageWithAVoice(t *testing.T) {
	tts := NewKokoroTTS(KokoroOptions{Voices: map[string][]string{"ja": {"jf_alpha"}}})
	if _, err := tts.Synthesize(protocol.TTSRequest{Text: "x"}); err == nil ||
		!strings.Contains(err.Error(), "needs a target language") {
		t.Fatalf("missing language error = %v", err)
	}
	if _, err := tts.Synthesize(protocol.TTSRequest{Text: "x", Direction: protocol.Direction{Tgt: "fr"}}); err == nil ||
		!strings.Contains(err.Error(), `no voice configured for target language "fr"`) {
		t.Fatalf("missing voice error = %v", err)
	}
}
