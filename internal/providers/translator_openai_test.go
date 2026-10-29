package providers_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/wray-lee/parley/internal/core"
	"github.com/wray-lee/parley/internal/protocol"
	"github.com/wray-lee/parley/internal/providers"
)

// The fixture replay mirrors tests/contract/translator-openai.test.js: the
// frozen SSE bytes arrive in 17-byte cuts with one cut inside a UTF-8 rune.

type translatorFixture struct {
	Preset string `json:"preset"`
	Input  struct {
		Prompt  string             `json:"prompt"`
		Context []core.ContextPair `json:"context"`
		Text    string             `json:"text"`
	} `json:"input"`
	Request struct {
		URL     string            `json:"url"`
		Method  string            `json:"method"`
		Headers map[string]string `json:"headers"`
		Body    map[string]any    `json:"body"`
	} `json:"request"`
	Response struct {
		Status int    `json:"status"`
		SSE    string `json:"sse"`
	} `json:"response"`
	Expected string `json:"expected"`
}

type translatorCapture struct {
	URL    string
	Method string
	Header http.Header
	Body   map[string]any
}

type translatorRoundTrip func(*http.Request) (*http.Response, error)

func (f translatorRoundTrip) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

type translatorChunkReader struct {
	chunks [][]byte
	index  int
}

func (c *translatorChunkReader) Read(p []byte) (int, error) {
	if c.index >= len(c.chunks) {
		return 0, io.EOF
	}
	n := copy(p, c.chunks[c.index])
	c.index++
	return n, nil
}

func (c *translatorChunkReader) Close() error { return nil }

func translatorSplitLikeJS(data []byte) [][]byte {
	var cuts []int
	for i, b := range data {
		if b >= 0x80 {
			cuts = append(cuts, i+1)
			break
		}
	}
	for i := 17; i < len(data); i += 17 {
		cuts = append(cuts, i)
	}
	sort.Ints(cuts)
	cuts = append(cuts, len(data))
	var chunks [][]byte
	start := 0
	for _, cut := range cuts {
		if cut > start {
			chunks = append(chunks, data[start:cut])
			start = cut
		}
	}
	return chunks
}

func translatorRespond(status int, sse string, capture *translatorCapture) *http.Client {
	return &http.Client{Transport: translatorRoundTrip(func(request *http.Request) (*http.Response, error) {
		if capture != nil {
			capture.URL = request.URL.String()
			capture.Method = request.Method
			capture.Header = request.Header.Clone()
			body, err := io.ReadAll(request.Body)
			if err != nil {
				return nil, err
			}
			capture.Body = map[string]any{}
			if err := json.Unmarshal(body, &capture.Body); err != nil {
				return nil, fmt.Errorf("request body is not JSON: %w", err)
			}
		}
		return &http.Response{
			StatusCode: status,
			Header:     http.Header{"Content-Type": {"text/event-stream"}},
			Body:       &translatorChunkReader{chunks: translatorSplitLikeJS([]byte(sse))},
		}, nil
	})}
}

func translatorFailingClient(err error) *http.Client {
	return &http.Client{Transport: translatorRoundTrip(func(*http.Request) (*http.Response, error) {
		return nil, err
	})}
}

func loadTranslatorPresets(t *testing.T) *providers.Presets {
	t.Helper()
	presets, err := providers.LoadPresets(os.DirFS("../.."))
	if err != nil {
		t.Fatal(err)
	}
	return presets
}

func newTestTranslator(t *testing.T, options providers.TranslatorOptions) *providers.OpenAITranslator {
	t.Helper()
	translator, err := providers.NewOpenAITranslator(loadTranslatorPresets(t), options)
	if err != nil {
		t.Fatal(err)
	}
	return translator
}

func translatorReadAll(translator *providers.OpenAITranslator, request protocol.TranslatorRequest) (string, error) {
	var output strings.Builder
	err := translator.Translate(request, func(chunk string) { output.WriteString(chunk) })
	return output.String(), err
}

func loadTranslatorFixture(t *testing.T, name string) translatorFixture {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "spec", "fixtures", "translator", name+".json"))
	if err != nil {
		t.Fatal(err)
	}
	var value translatorFixture
	if err := json.Unmarshal(data, &value); err != nil {
		t.Fatal(err)
	}
	return value
}

func TestTranslatorContractFixturesReplay(t *testing.T) {
	for _, name := range []string{"groq", "siliconflow", "ollama"} {
		t.Run(name, func(t *testing.T) {
			value := loadTranslatorFixture(t, name)
			prompt := core.BuildTranslatorRequest(value.Input.Prompt, value.Input.Context, value.Input.Text, nil)
			if got := value.Request.Body["messages"].([]any)[1].(map[string]any)["content"]; got != prompt.User {
				t.Fatalf("user message = %q, want the fixed format %q", got, prompt.User)
			}

			apiKey := ""
			if _, ok := value.Request.Headers["authorization"]; ok {
				apiKey = "test-key"
			}
			var capture translatorCapture
			translator := newTestTranslator(t, providers.TranslatorOptions{
				Preset: value.Preset, APIKey: apiKey, HTTPClient: translatorRespond(value.Response.Status, value.Response.SSE, &capture),
			})

			output, err := translatorReadAll(translator, protocol.TranslatorRequest{
				System: prompt.System, User: prompt.User, Text: value.Input.Text,
			})
			if err != nil {
				t.Fatal(err)
			}
			if output != value.Expected {
				t.Fatalf("output = %q, want %q", output, value.Expected)
			}
			if capture.URL != value.Request.URL || capture.Method != value.Request.Method {
				t.Fatalf("request = %s %s, want %s %s", capture.Method, capture.URL, value.Request.Method, value.Request.URL)
			}
			for key, want := range value.Request.Headers {
				want = strings.ReplaceAll(want, "<redacted>", "test-key")
				if got := capture.Header.Get(key); got != want {
					t.Fatalf("header %s = %q, want %q", key, got, want)
				}
			}
			if _, ok := value.Request.Headers["authorization"]; !ok && capture.Header.Get("Authorization") != "" {
				t.Fatalf("unexpected authorization header %q", capture.Header.Get("Authorization"))
			}
			if !reflect.DeepEqual(capture.Body, value.Request.Body) {
				got, _ := json.Marshal(capture.Body)
				want, _ := json.Marshal(value.Request.Body)
				t.Fatalf("body = %s, want %s", got, want)
			}
		})
	}
}

func TestTranslatorExtraBodyCannotOverrideRequiredFields(t *testing.T) {
	value := loadTranslatorFixture(t, "ollama")
	var capture translatorCapture
	translator := newTestTranslator(t, providers.TranslatorOptions{
		Preset: "ollama",
		Model:  "llama3.1",
		ExtraBody: `{"temperature":0.2,"chat_template_kwargs":{"reasoning_effort":"low"},` +
			`"model":"not-the-model","stream":false,"messages":[]}`,
		HTTPClient: translatorRespond(200, value.Response.SSE, &capture),
	})
	if _, err := translatorReadAll(translator, protocol.TranslatorRequest{System: "s", User: "u"}); err != nil {
		t.Fatal(err)
	}
	if capture.Body["temperature"] != 0.2 {
		t.Fatalf("temperature = %v", capture.Body["temperature"])
	}
	if !reflect.DeepEqual(capture.Body["chat_template_kwargs"], map[string]any{"reasoning_effort": "low"}) {
		t.Fatalf("chat_template_kwargs = %v", capture.Body["chat_template_kwargs"])
	}
	if capture.Body["model"] != "llama3.1" || capture.Body["stream"] != true {
		t.Fatalf("required fields were overridden: %v", capture.Body)
	}
	if messages := capture.Body["messages"].([]any); len(messages) != 2 {
		t.Fatalf("messages = %v", messages)
	}
}

func TestTranslatorStreamsEachDeltaAsOneChunk(t *testing.T) {
	sse := "data: {\"choices\":[{\"delta\":{\"role\":\"assistant\",\"content\":\"\"}}]}\n\n" +
		"data: []\n\ndata: null\n\ndata: \"ignored\"\n\n" +
		"data: {\"choices\":[{\"delta\":{\"content\":\"⏸\"}}]}\n\n" +
		"data: {\"choices\":[{\"delta\":{\"content\":\"ok\"}}]}\n\n" +
		"data: [DONE]\n\n"
	translator := newTestTranslator(t, providers.TranslatorOptions{
		Preset: "groq", APIKey: "k", HTTPClient: translatorRespond(200, sse, nil),
	})
	var chunks []string
	if err := translator.Translate(protocol.TranslatorRequest{}, func(chunk string) { chunks = append(chunks, chunk) }); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(chunks, []string{"⏸", "ok"}) {
		t.Fatalf("chunks = %q, want one call per non-empty delta", chunks)
	}
}

func TestTranslatorInvalidExtraBody(t *testing.T) {
	presets := loadTranslatorPresets(t)
	_, err := providers.NewOpenAITranslator(presets, providers.TranslatorOptions{
		Preset: "ollama", Model: "llama3.1", ExtraBody: "{not json",
	})
	if err == nil || !strings.HasPrefix(err.Error(), "LLM_EXTRA_BODY is not valid JSON: ") {
		t.Fatalf("invalid JSON error = %v", err)
	}
	for _, value := range []string{"[]", "null", `"text"`, "42"} {
		_, err := providers.NewOpenAITranslator(presets, providers.TranslatorOptions{
			Preset: "ollama", Model: "llama3.1", ExtraBody: value,
		})
		if err == nil || err.Error() != "LLM_EXTRA_BODY must be a JSON object" {
			t.Fatalf("ExtraBody %q error = %v", value, err)
		}
	}
}

func TestTranslatorUnknownPreset(t *testing.T) {
	presets := loadTranslatorPresets(t)
	_, err := providers.NewOpenAITranslator(presets, providers.TranslatorOptions{Preset: "not-a-preset", APIKey: "k"})
	want := `unknown preset "not-a-preset"; known presets: groq, gemini, siliconflow, dashscope, openrouter, ollama, custom, mistral, deepseek, zhipu, moonshot, cerebras`
	if err == nil || err.Error() != want {
		t.Fatalf("error = %v, want %q", err, want)
	}
}

func TestTranslatorBaseURLAndModelOverrideThePreset(t *testing.T) {
	var capture translatorCapture
	translator := newTestTranslator(t, providers.TranslatorOptions{
		Preset: "groq", APIKey: "test-key",
		BaseURL: "https://gateway.example/v1/", Model: "my-model",
		HTTPClient: translatorRespond(200, "data: {\"choices\":[{\"delta\":{\"content\":\"ok\"}}]}\n\ndata: [DONE]\n\n", &capture),
	})
	output, err := translatorReadAll(translator, protocol.TranslatorRequest{})
	if err != nil || output != "ok" {
		t.Fatalf("output = %q, err = %v", output, err)
	}
	if capture.URL != "https://gateway.example/v1/chat/completions" || capture.Body["model"] != "my-model" {
		t.Fatalf("request = %s body model = %v", capture.URL, capture.Body["model"])
	}
}

func TestTranslatorPresetWithoutKeyFailsAtRequestTime(t *testing.T) {
	translator, err := providers.NewOpenAITranslator(loadTranslatorPresets(t), providers.TranslatorOptions{
		Preset:     "groq",
		HTTPClient: translatorFailingClient(errors.New("the test must not reach the network")),
	})
	if err != nil {
		t.Fatalf("creation without a key must succeed: %v", err)
	}
	if translator.Name() != "openai" {
		t.Fatalf("name = %q", translator.Name())
	}
	_, err = translatorReadAll(translator, protocol.TranslatorRequest{})
	var providerError *protocol.ProviderError
	if !errors.As(err, &providerError) || providerError.Kind != "auth" || !strings.Contains(providerError.Message, "LLM_API_KEY") || providerError.Fallback == "" {
		t.Fatalf("error = %#v", err)
	}
}

func TestTranslatorCustomPresetNeedsBaseURLAndModel(t *testing.T) {
	presets := loadTranslatorPresets(t)
	if _, err := providers.NewOpenAITranslator(presets, providers.TranslatorOptions{Preset: "custom"}); err == nil || !strings.Contains(err.Error(), "LLM_BASE_URL") {
		t.Fatalf("missing base URL error = %v", err)
	}
	if _, err := providers.NewOpenAITranslator(presets, providers.TranslatorOptions{
		Preset: "custom", BaseURL: "https://gateway.example/v1",
	}); err == nil || !strings.Contains(err.Error(), "LLM_MODEL") {
		t.Fatalf("missing model error = %v", err)
	}
	if _, err := providers.NewOpenAITranslator(presets, providers.TranslatorOptions{
		Preset: "custom", BaseURL: "https://user:password@gateway.example/v1", Model: "m",
	}); err == nil || !strings.Contains(err.Error(), "credentials") {
		t.Fatalf("credentials error = %v", err)
	}
	if _, err := providers.NewOpenAITranslator(presets, providers.TranslatorOptions{
		Preset: "custom", BaseURL: "ftp://gateway.example/v1", Model: "m",
	}); err == nil || !strings.Contains(err.Error(), "http") {
		t.Fatalf("protocol error = %v", err)
	}
	if _, err := providers.NewOpenAITranslator(presets, providers.TranslatorOptions{
		Preset: "custom", BaseURL: "not a url", Model: "m",
	}); err == nil || !strings.Contains(err.Error(), "invalid LLM_BASE_URL") {
		t.Fatalf("invalid URL error = %v", err)
	}
}

func TestTranslatorStatusErrors(t *testing.T) {
	cases := []struct {
		status int
		kind   string
	}{
		{401, "auth"}, {403, "auth"}, {429, "rate_limit"}, {404, "protocol"}, {500, "protocol"},
	}
	for _, test := range cases {
		t.Run(fmt.Sprint(test.status), func(t *testing.T) {
			translator := newTestTranslator(t, providers.TranslatorOptions{
				Preset: "groq", APIKey: "test-key",
				HTTPClient: translatorRespond(test.status, `{"error":{"message":"Invalid API Key: test-key"}}`, nil),
			})
			_, err := translatorReadAll(translator, protocol.TranslatorRequest{})
			var providerError *protocol.ProviderError
			if !errors.As(err, &providerError) || providerError.Kind != test.kind || providerError.Fallback == "" {
				t.Fatalf("error = %#v", err)
			}
			if !strings.Contains(providerError.Message, fmt.Sprint(test.status)) {
				t.Fatalf("message %q does not carry the status", providerError.Message)
			}
			if strings.Contains(providerError.Message, "test-key") {
				t.Fatalf("message leaks the API key: %q", providerError.Message)
			}
		})
	}
}

func TestTranslatorUnreachableAndTimeout(t *testing.T) {
	unreachable := newTestTranslator(t, providers.TranslatorOptions{
		Preset: "groq", APIKey: "k", HTTPClient: translatorFailingClient(errors.New("dial tcp: connection refused")),
	})
	_, err := translatorReadAll(unreachable, protocol.TranslatorRequest{})
	var providerError *protocol.ProviderError
	if !errors.As(err, &providerError) || providerError.Kind != "unreachable" || providerError.Fallback == "" {
		t.Fatalf("unreachable error = %#v", err)
	}

	slow := &http.Client{Transport: translatorRoundTrip(func(request *http.Request) (*http.Response, error) {
		<-request.Context().Done()
		return nil, request.Context().Err()
	})}
	timed := newTestTranslator(t, providers.TranslatorOptions{
		Preset: "groq", APIKey: "k", HTTPClient: slow, Timeout: 20 * time.Millisecond,
	})
	_, err = translatorReadAll(timed, protocol.TranslatorRequest{})
	if !errors.As(err, &providerError) || providerError.Kind != "timeout" || !strings.Contains(providerError.Message, "timed out") {
		t.Fatalf("timeout error = %#v", err)
	}
}

func TestTranslatorMalformedStreams(t *testing.T) {
	cases := []struct {
		name string
		sse  string
		want string
	}{
		{"malformed JSON", "data: {not json\n\n", "malformed SSE JSON"},
		{"error event", "data: {\"error\":{\"message\":\"boom\"}}\n\n", "translator error event: boom"},
		{"empty stream", "data: [DONE]\n\n", "no text"},
		{"ignored non-data lines", "\n: keep-alive\n\n", "no text"},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			translator := newTestTranslator(t, providers.TranslatorOptions{
				Preset: "groq", APIKey: "k", HTTPClient: translatorRespond(200, test.sse, nil),
			})
			_, err := translatorReadAll(translator, protocol.TranslatorRequest{})
			var providerError *protocol.ProviderError
			if !errors.As(err, &providerError) || providerError.Kind != "protocol" || !strings.Contains(providerError.Message, test.want) || providerError.Fallback == "" {
				t.Fatalf("error = %#v", err)
			}
		})
	}
}

func TestSelectBuildsTheOpenAITranslator(t *testing.T) {
	asr, translator, tts, err := providers.Select(providers.Config{
		ASR: "mock", Translator: "openai", TTS: "mock", Sample: sample(),
		Presets: loadTranslatorPresets(t), LLM: providers.TranslatorOptions{Preset: "ollama"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if asr.Name() != "mock" || translator.Name() != "openai" || tts.Name() != "mock" {
		t.Fatalf("providers = %s/%s/%s", asr.Name(), translator.Name(), tts.Name())
	}

	if _, _, _, err := providers.Select(providers.Config{
		ASR: "mock", Translator: "openai", TTS: "mock", Sample: sample(),
	}); err == nil {
		t.Fatal("openai without config/presets.json must fail")
	}
}

func TestTranslatorEmptyResponseBodies(t *testing.T) {
	// net/http reports http.NoBody for an empty 200 as well as for 204/205;
	// fetch has a null body only for 204/205, so a real empty 200 must reach
	// the "returned no text" path.
	cases := []struct {
		name   string
		status int
		want   string
	}{
		{"empty 200", http.StatusOK, "translator returned no text (custom)"},
		{"204", http.StatusNoContent, "translator sent no response body (custom)"},
		{"205", http.StatusResetContent, "translator sent no response body (custom)"},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(test.status)
			}))
			defer server.Close()
			translator := newTestTranslator(t, providers.TranslatorOptions{
				Preset: "custom", BaseURL: server.URL, Model: "m",
			})
			_, err := translatorReadAll(translator, protocol.TranslatorRequest{})
			var providerError *protocol.ProviderError
			if !errors.As(err, &providerError) || providerError.Kind != "protocol" ||
				providerError.Message != test.want || providerError.Fallback == "" {
				t.Fatalf("error = %#v, want message %q", err, test.want)
			}
		})
	}
}

func TestTranslatorInBandErrorMessagesMatchJavaScript(t *testing.T) {
	cases := []struct {
		name string
		sse  string
		want string
	}{
		// String(event.error.message ?? JSON.stringify(event.error)).
		{"empty message", `{"error":{"message":""}}`, "translator error event: "},
		{"object message", `{"error":{"message":{"code":7,"detail":"x"}}}`, "translator error event: [object Object]"},
		{"array message", `{"error":{"message":["a",null,"b"]}}`, "translator error event: a,,b"},
		{"null message falls back", `{"error":{"message":null,"b":1,"a":2}}`, `translator error event: {"message":null,"b":1,"a":2}`},
		// JSON.stringify keeps the wire order of keys; a map marshal sorts them.
		{"key order", `{"error":{"b":1,"a":2}}`, `translator error event: {"b":1,"a":2}`},
		{"string error", `{"error":"boom"}`, `translator error event: "boom"`},
		{"number message 1e6", `{"error":{"message":1000000}}`, "translator error event: 1000000"},
		{"number message 1e-6", `{"error":{"message":0.000001}}`, "translator error event: 0.000001"},
		{"number message 1e-7", `{"error":{"message":0.0000001}}`, "translator error event: 1e-7"},
		{"number message 1e21", `{"error":{"message":1e21}}`, "translator error event: 1e+21"},
		{"zero message", `{"error":{"message":0}}`, "translator error event: 0"},
		{"false message", `{"error":{"message":false}}`, "translator error event: false"},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			translator := newTestTranslator(t, providers.TranslatorOptions{
				Preset: "groq", APIKey: "k", HTTPClient: translatorRespond(200, "data: "+test.sse+"\n\n", nil),
			})
			_, err := translatorReadAll(translator, protocol.TranslatorRequest{})
			var providerError *protocol.ProviderError
			if !errors.As(err, &providerError) || providerError.Kind != "protocol" ||
				providerError.Message != test.want ||
				providerError.Fallback != "Switch LLM_PRESET, or check the endpoint and model." {
				t.Fatalf("error = %#v, want message %q", err, test.want)
			}
		})
	}
}

func TestPresetsLoadFromTheSharedJSONData(t *testing.T) {
	presets := loadTranslatorPresets(t)
	if presets.Default != "groq" {
		t.Fatalf("default = %q", presets.Default)
	}
	want := []string{"groq", "gemini", "siliconflow", "dashscope", "openrouter", "ollama", "custom", "mistral", "deepseek", "zhipu", "moonshot", "cerebras"}
	if !reflect.DeepEqual(presets.Order, want) {
		t.Fatalf("preset order = %v", presets.Order)
	}
	if len(presets.ByID) != len(want) {
		t.Fatalf("presets = %d, want %d", len(presets.ByID), len(want))
	}
	for _, id := range want {
		preset := presets.ByID[id]
		if preset.ID != id {
			t.Fatalf("preset %q id = %q", id, preset.ID)
		}
		if id != "custom" && preset.BaseURL == "" {
			t.Fatalf("preset %q has no base URL", id)
		}
	}
	if presets.ByID["groq"].TrainsOnData || !presets.ByID["gemini"].TrainsOnData || presets.ByID["ollama"].RequiresKey {
		t.Fatal("preset flags changed")
	}
}

func TestPresetsMatchTheFrozenOracleData(t *testing.T) {
	read := func(path string) any {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		var value any
		if err := json.Unmarshal(data, &value); err != nil {
			t.Fatal(err)
		}
		return value
	}
	current := read(filepath.Join("..", "..", "config", "presets.json"))
	frozen := read(filepath.Join("..", "..", "spec", "fixtures", "presets-v0.0.3.json"))
	if !reflect.DeepEqual(current, frozen) {
		t.Fatal("config/presets.json diverged from the frozen v0.0.3 oracle")
	}
}
