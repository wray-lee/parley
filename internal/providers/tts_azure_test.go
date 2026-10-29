package providers

import (
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

// Mirrors tests/contract/tts-azure.test.js: the env key and region produce the
// documented Azure REST request, and failures name the fallback.

func azureFixture(t *testing.T) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "spec", "fixtures", "azure", "ja.mp3"))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	return data
}

func TestAzureRequestShapeAndAudio(t *testing.T) {
	expected := azureFixture(t)
	var got *http.Request
	var body string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r
		data, _ := io.ReadAll(r.Body)
		body = string(data)
		w.Header().Set("content-type", "audio/mpeg")
		_, _ = w.Write(expected)
	}))
	defer server.Close()

	tts, err := NewAzureTTS(AzureOptions{
		AzureKey:    "test-key",
		AzureRegion: "japaneast",
		Endpoint:    server.URL,
		Voices:      map[string][]string{"ja": {"ja-JP-NanamiNeural"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	audio, err := tts.Synthesize(protocol.TTSRequest{
		Text: "天気 & <ok>", SegID: 1, Direction: protocol.Direction{Src: "zh", Tgt: "ja"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if tts.Name() != "azure" || audio.Format != "mp3" || !reflect.DeepEqual(audio.Data, expected) {
		t.Fatalf("audio = %s %d bytes", audio.Format, len(audio.Data))
	}
	if got.Method != http.MethodPost {
		t.Fatalf("method = %s", got.Method)
	}
	if got.Header.Get("Ocp-Apim-Subscription-Key") != "test-key" ||
		got.Header.Get("Content-Type") != "application/ssml+xml" ||
		got.Header.Get("X-Microsoft-OutputFormat") != "audio-24khz-48kbitrate-mono-mp3" {
		t.Fatalf("headers = %v", got.Header)
	}
	for _, want := range []string{
		`<voice name='ja-JP-NanamiNeural'>`,
		`xml:lang='ja-JP'`,
		`天気 &amp; &lt;ok&gt;`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("body = %q, missing %q", body, want)
		}
	}
}

func TestAzureDefaultEndpointComesFromTheRegion(t *testing.T) {
	tts, err := NewAzureTTS(AzureOptions{AzureKey: "k", AzureRegion: "japaneast"})
	if err != nil {
		t.Fatal(err)
	}
	if tts.endpoint != "https://japaneast.tts.speech.microsoft.com/cognitiveservices/v1" {
		t.Fatalf("endpoint = %q", tts.endpoint)
	}
}

func TestAzureErrorsNameTheKeyAndTheFallback(t *testing.T) {
	cases := []struct {
		name   string
		status int
		want   []string
	}{
		{"rejected key", http.StatusUnauthorized, []string{"HTTP 401", "AZURE_SPEECH_KEY and AZURE_SPEECH_REGION", "system or mock"}},
		{"rate limit", http.StatusTooManyRequests, []string{"rate limited", "system or mock"}},
		{"other", http.StatusBadGateway, []string{"HTTP 502", "system or mock"}},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				http.Error(w, "nope", test.status)
			}))
			defer server.Close()
			tts, err := NewAzureTTS(AzureOptions{
				AzureKey: "k", AzureRegion: "japaneast", Endpoint: server.URL,
				Voices: map[string][]string{"ja": {"ja-JP-NanamiNeural"}},
			})
			if err != nil {
				t.Fatal(err)
			}
			_, err = tts.Synthesize(protocol.TTSRequest{Text: "x", Direction: protocol.Direction{Tgt: "ja"}})
			if err == nil {
				t.Fatal("expected an error")
			}
			for _, want := range test.want {
				if !strings.Contains(err.Error(), want) {
					t.Fatalf("error = %v, missing %q", err, want)
				}
			}
		})
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	url := server.URL
	server.Close()
	tts, err := NewAzureTTS(AzureOptions{
		AzureKey: "k", AzureRegion: "japaneast", Endpoint: url,
		Voices: map[string][]string{"ja": {"ja-JP-NanamiNeural"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = tts.Synthesize(protocol.TTSRequest{Text: "x", Direction: protocol.Direction{Tgt: "ja"}})
	if err == nil || !strings.Contains(err.Error(), "Azure TTS is unreachable") ||
		!strings.Contains(err.Error(), "system or mock") {
		t.Fatalf("unreachable error = %v", err)
	}
}

func TestAzureRequiresKeyAndRegionAtCreate(t *testing.T) {
	if _, err := NewAzureTTS(AzureOptions{AzureRegion: "japaneast"}); err == nil ||
		!strings.Contains(err.Error(), "AZURE_SPEECH_KEY") {
		t.Fatalf("missing key = %v", err)
	}
	if _, err := NewAzureTTS(AzureOptions{AzureKey: "k", AzureRegion: "not a region"}); err == nil ||
		!strings.Contains(err.Error(), "AZURE_SPEECH_REGION") {
		t.Fatalf("bad region = %v", err)
	}
}

func TestAzureFallsBackToTheBaseLanguageVoice(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(azureFixture(t))
	}))
	defer server.Close()
	var warnings []string
	tts, err := NewAzureTTS(AzureOptions{
		AzureKey: "k", AzureRegion: "japaneast", Endpoint: server.URL,
		Voices: map[string][]string{"ja": {"ja-JP-NanamiNeural"}},
		Warn:   func(line string) { warnings = append(warnings, line) },
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tts.Synthesize(protocol.TTSRequest{Text: "x", Direction: protocol.Direction{Tgt: "ja-JP"}}); err != nil {
		t.Fatal(err)
	}
	if len(warnings) != 1 || !strings.Contains(warnings[0], `no voice configured for "ja-JP"`) {
		t.Fatalf("warnings = %q", warnings)
	}
}

func TestAzureMissingVoiceNamesTheFallback(t *testing.T) {
	tts, err := NewAzureTTS(AzureOptions{
		AzureKey: "k", AzureRegion: "japaneast",
		Voices: map[string][]string{"ja": {"ja-JP-NanamiNeural"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = tts.Synthesize(protocol.TTSRequest{Text: "x", Direction: protocol.Direction{Tgt: "fr"}})
	if err == nil || !strings.Contains(err.Error(), `no voice configured for target language "fr"`) ||
		!strings.Contains(err.Error(), "system or mock") {
		t.Fatalf("missing voice = %v", err)
	}
}
