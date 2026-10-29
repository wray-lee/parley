// Kokoro-FastAPI: OpenAI-style /v1/audio/speech returning mp3. The voice map is
// data in config/voices.json; the target language is the Direction's tgt.
// Ported from core/providers/tts-kokoro.js.

package providers

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"unicode"

	"github.com/wray-lee/parley/internal/protocol"
)

const (
	kokoroDefaultURL = "http://127.0.0.1:8880"
	kokoroFallback   = "Set TTS_PROVIDER=system or mock to run without Kokoro."
)

// KokoroOptions are the Server Host's KOKORO_URL and the voices data.
type KokoroOptions struct {
	KokoroURL string
	Voices    map[string][]string
	Model     string
	// HTTPClient and Timeout are injected by tests; nil means http.DefaultClient.
	HTTPClient *http.Client
	Warn       func(string)
}

// KokoroTTS is the Kokoro-FastAPI Provider.
type KokoroTTS struct {
	base     string
	endpoint string
	voices   map[string][]string
	model    string
	client   *http.Client
	warn     func(string)
}

// NewKokoroTTS builds the adapter from config data.
func NewKokoroTTS(options KokoroOptions) *KokoroTTS {
	base := strings.TrimRight(strings.TrimSpace(options.KokoroURL), "/")
	if base == "" {
		base = kokoroDefaultURL
	}
	endpoint := base + "/v1/audio/speech"
	if strings.HasSuffix(strings.ToLower(base), "/v1") {
		endpoint = base + "/audio/speech"
	}
	model := options.Model
	if model == "" {
		model = "kokoro"
	}
	client := options.HTTPClient
	if client == nil {
		client = http.DefaultClient
	}
	warn := options.Warn
	if warn == nil {
		warn = func(string) {}
	}
	return &KokoroTTS{base: base, endpoint: endpoint, voices: options.Voices, model: model, client: client, warn: warn}
}

// Name is the Provider name.
func (k *KokoroTTS) Name() string { return "kokoro" }

type kokoroRequest struct {
	Model          string `json:"model"`
	Input          string `json:"input"`
	Voice          string `json:"voice"`
	ResponseFormat string `json:"response_format"`
	Stream         bool   `json:"stream"`
}

// httpStatusError keeps the status so a missing voice can fall back to another.
type httpStatusError struct {
	status  int
	message string
}

func (e *httpStatusError) Error() string { return e.message }

func (k *KokoroTTS) post(input, voice string) ([]byte, error) {
	payload, err := json.Marshal(kokoroRequest{
		Model: k.model, Input: input, Voice: voice, ResponseFormat: "mp3", Stream: true,
	})
	if err != nil {
		return nil, err
	}
	request, err := http.NewRequest(http.MethodPost, k.endpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	request.Header.Set("content-type", "application/json")
	response, err := k.client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("Kokoro TTS is unreachable at %s: %v. %s", k.base, err, kokoroFallback)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode > 299 {
		detail, _ := io.ReadAll(io.LimitReader(response.Body, 4096))
		suffix := ""
		if trimmed := strings.TrimSpace(string(detail)); trimmed != "" {
			if runes := []rune(trimmed); len(runes) > 200 {
				trimmed = string(runes[:200])
			}
			suffix = ": " + trimmed
		}
		return nil, &httpStatusError{
			status: response.StatusCode,
			message: fmt.Sprintf("Kokoro TTS at %s returned HTTP %d%s. %s",
				k.base, response.StatusCode, suffix, kokoroFallback),
		}
	}
	data, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, fmt.Errorf("Kokoro TTS at %s: %v. %s", k.base, err, kokoroFallback)
	}
	return data, nil
}

// Synthesize sends one request per sentence and joins the clips.
func (k *KokoroTTS) Synthesize(request protocol.TTSRequest) (protocol.Audio, error) {
	candidates, err := voiceList("Kokoro", k.voices, request.Direction.Tgt, "", k.warn)
	if err != nil {
		return protocol.Audio{}, err
	}
	sentences := splitSentences(request.Text)
	if len(sentences) == 0 {
		return protocol.Audio{Format: "mp3", Data: []byte{}}, nil
	}

	voice := 0
	first := true
	var parts [][]byte
	for _, sentence := range sentences {
		for {
			clip, err := k.post(sentence, candidates[voice])
			if err == nil {
				if first {
					parts = append(parts, clip)
					first = false
				} else {
					parts = append(parts, withoutID3(clip))
				}
				break
			}
			var status *httpStatusError
			retryable := errors.As(err, &status) && (status.status == http.StatusBadRequest || status.status == http.StatusNotFound)
			if !retryable || voice+1 >= len(candidates) {
				return protocol.Audio{}, err
			}
			k.warn(fmt.Sprintf("Kokoro voice %s is unavailable; falling back to %s", candidates[voice], candidates[voice+1]))
			voice++
		}
	}
	return protocol.Audio{Format: "mp3", Data: concatBytes(parts)}, nil
}

// splitSentences splits after a Unicode Sentence_Terminal, keeping trailing
// closing punctuation.
func splitSentences(text string) []string {
	input := strings.TrimSpace(text)
	var out []string
	if input == "" {
		return out
	}
	runes := []rune(input)
	start := 0
	for i := 0; i < len(runes); {
		if unicode.Is(unicode.Sentence_Terminal, runes[i]) {
			end := i + 1
			for end < len(runes) && unicode.Is(unicode.Pe, runes[end]) {
				end++
			}
			pushSentence(&out, string(runes[start:end]))
			start, i = end, end
			continue
		}
		i++
	}
	pushSentence(&out, string(runes[start:]))
	return out
}

func pushSentence(out *[]string, sentence string) {
	if trimmed := strings.TrimSpace(sentence); trimmed != "" {
		*out = append(*out, trimmed)
	}
}

// withoutID3 drops the ID3v2 tag so joined clips stay one decodable mp3 chunk.
func withoutID3(data []byte) []byte {
	if len(data) < 10 || data[0] != 0x49 || data[1] != 0x44 || data[2] != 0x33 {
		return data
	}
	size := int(data[6]&0x7f)<<21 | int(data[7]&0x7f)<<14 | int(data[8]&0x7f)<<7 | int(data[9]&0x7f)
	offset := 10 + size
	if data[5]&0x10 != 0 {
		offset += 10
	}
	if offset > len(data) {
		offset = len(data)
	}
	return data[offset:]
}

func concatBytes(parts [][]byte) []byte {
	total := 0
	for _, part := range parts {
		total += len(part)
	}
	data := make([]byte, 0, total)
	for _, part := range parts {
		data = append(data, part...)
	}
	return data
}
