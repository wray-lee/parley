// OpenAI-compatible chat-completions translator, ported 1:1 from
// core/providers/translator-openai.js. One adapter serves every Preset;
// options come from the Server Host's LLM_* variables. Errors carry a kind
// (unreachable|auth|rate_limit|timeout|protocol) and a suggested fallback so
// the host can surface them instead of failing silently.

package providers

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/wray-lee/parley/internal/protocol"
)

const (
	// shortcut: one whole-stream 30 s timeout, add a first-token timeout if
	// slow local models need it.
	defaultTranslatorTimeout = 30 * time.Second
	bodyLimit                = 200
)

// TranslatorOptions are the Server Host's LLM_* settings plus test seams.
type TranslatorOptions struct {
	Preset    string
	BaseURL   string
	APIKey    string
	Model     string
	ExtraBody string
	// HTTPClient and Timeout are injected by tests; nil and 0 mean
	// http.DefaultClient and 30 s.
	HTTPClient *http.Client
	Timeout    time.Duration
}

// OpenAITranslator is the streaming translator Provider.
type OpenAITranslator struct {
	preset   Preset
	model    string
	endpoint string
	apiKey   string
	extra    map[string]any
	headers  map[string]string
	client   *http.Client
	timeout  time.Duration
}

// NewOpenAITranslator validates the options exactly like the JavaScript
// adapter: same defaults, same error messages, same order of checks.
func NewOpenAITranslator(presets *Presets, options TranslatorOptions) (*OpenAITranslator, error) {
	id := options.Preset
	if id == "" {
		id = presets.Default
	}
	preset, ok := presets.ByID[id]
	if !ok {
		return nil, fmt.Errorf("unknown preset %q; known presets: %s", id, strings.Join(presets.Order, ", "))
	}

	base := strings.TrimRight(firstNonEmptyTranslatorOption(options.BaseURL, preset.BaseURL), "/")
	if base == "" {
		return nil, fmt.Errorf("preset %q requires LLM_BASE_URL", id)
	}
	parsed, err := parseTranslatorBaseURL(base, id)
	if err != nil {
		return nil, err
	}
	endpoint := parsed
	if !strings.HasSuffix(parsed, "/chat/completions") {
		endpoint = parsed + "/chat/completions"
	}

	model := firstNonEmptyTranslatorOption(options.Model, preset.Model)
	if model == "" {
		return nil, fmt.Errorf("preset %q requires LLM_MODEL", id)
	}
	extra, err := parseTranslatorExtraBody(options.ExtraBody)
	if err != nil {
		return nil, err
	}

	timeout := options.Timeout
	if timeout <= 0 {
		timeout = defaultTranslatorTimeout
	}
	return &OpenAITranslator{
		preset:   preset,
		model:    model,
		endpoint: endpoint,
		apiKey:   options.APIKey,
		extra:    extra,
		headers:  map[string]string{"content-type": "application/json"},
		client:   options.HTTPClient,
		timeout:  timeout,
	}, nil
}

// Name is the Provider name used by hello.demo and error events.
func (t *OpenAITranslator) Name() string { return "openai" }

// Preset reports the resolved Preset so the Host can publish hello.presetsInUse.
func (t *OpenAITranslator) Preset() Preset { return t.preset }

// Translate streams the answer through onChunk. A required key is checked at
// request time, not at creation, so a keyless host still starts.
func (t *OpenAITranslator) Translate(request protocol.TranslatorRequest, onChunk func(string)) error {
	if t.preset.RequiresKey && t.apiKey == "" {
		return &protocol.ProviderError{
			Kind:     "auth",
			Message:  fmt.Sprintf("LLM_API_KEY is not set for preset %q", t.preset.ID),
			Fallback: "Set LLM_API_KEY, or unset the LLM_* variables to use Demo Mode.",
		}
	}

	payload, err := t.requestBody(request)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), t.timeout)
	defer cancel()
	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodPost, t.endpoint, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	for key, value := range t.headers {
		httpRequest.Header.Set(key, value)
	}
	if t.apiKey != "" {
		httpRequest.Header.Set("Authorization", "Bearer "+t.apiKey)
	}

	response, err := t.httpClient().Do(httpRequest)
	if err != nil {
		return t.transportError(ctx, err)
	}
	if response.StatusCode < 200 || response.StatusCode > 299 {
		var detail []byte
		if response.Body != nil {
			detail, _ = io.ReadAll(response.Body)
			_ = response.Body.Close()
		}
		return t.statusError(response.StatusCode, string(detail))
	}
	// fetch has a null body only for 204/205; net/http also reports
	// http.NoBody for an empty 200, which must reach the "no text" path.
	if response.StatusCode == http.StatusNoContent || response.StatusCode == http.StatusResetContent {
		return &protocol.ProviderError{
			Kind:     "protocol",
			Message:  fmt.Sprintf("translator sent no response body (%s)", t.preset.ID),
			Fallback: "Check LLM_BASE_URL and LLM_MODEL for this Preset.",
		}
	}
	if response.Body == nil {
		response.Body = http.NoBody
	}
	defer response.Body.Close()

	emitted, err := t.stream(response.Body, ctx, onChunk)
	if err != nil {
		return err
	}
	if !emitted {
		return &protocol.ProviderError{
			Kind:     "protocol",
			Message:  fmt.Sprintf("translator returned no text (%s)", t.preset.ID),
			Fallback: "Switch LLM_PRESET, or check the endpoint and model with LLM_BASE_URL / LLM_MODEL.",
		}
	}
	return nil
}

func (t *OpenAITranslator) httpClient() *http.Client {
	if t.client != nil {
		return t.client
	}
	return http.DefaultClient
}

func (t *OpenAITranslator) requestBody(request protocol.TranslatorRequest) ([]byte, error) {
	// Extra fields merge first; Parley's required fields always win, so
	// model/messages/stream cannot be overridden.
	body := make(map[string]any, len(t.extra)+3)
	for key, value := range t.extra {
		body[key] = value
	}
	body["model"] = t.model
	body["stream"] = true
	body["messages"] = []map[string]string{
		{"role": "system", "content": request.System},
		{"role": "user", "content": request.User},
	}
	return json.Marshal(body)
}

// stream parses the SSE body line by line; chunks may split anywhere.
func (t *OpenAITranslator) stream(body io.Reader, ctx context.Context, onChunk func(string)) (bool, error) {
	reader := bufio.NewReader(body)
	emitted := false
	for {
		line, err := reader.ReadString('\n')
		if line != "" {
			content, done, parseErr := t.parseLine(line)
			if parseErr != nil {
				return emitted, parseErr
			}
			if content != "" {
				emitted = true
				onChunk(content)
			}
			if done {
				return emitted, nil
			}
		}
		if err != nil {
			if err == io.EOF {
				return emitted, nil
			}
			if errors.Is(ctx.Err(), context.DeadlineExceeded) {
				return emitted, t.timeoutError()
			}
			return emitted, &protocol.ProviderError{
				Kind:     "protocol",
				Message:  fmt.Sprintf("translator sent a malformed stream (%s): %v", t.preset.ID, err),
				Fallback: "Switch LLM_PRESET, or check the endpoint and model with LLM_BASE_URL / LLM_MODEL.",
			}
		}
	}
}

func (t *OpenAITranslator) parseLine(rawLine string) (content string, done bool, err error) {
	line := strings.TrimSuffix(rawLine, "\r")
	if !strings.HasPrefix(line, "data:") {
		return "", false, nil
	}
	payload := strings.TrimSpace(line[len("data:"):])
	if payload == "" {
		return "", false, nil
	}
	if payload == "[DONE]" {
		return "", true, nil
	}

	var event any
	if err := json.Unmarshal([]byte(payload), &event); err != nil {
		return "", false, &protocol.ProviderError{
			Kind:     "protocol",
			Message:  "translator sent malformed SSE JSON: " + truncateTranslatorText(payload, bodyLimit),
			Fallback: "Switch LLM_PRESET, or check the endpoint with LLM_BASE_URL.",
		}
	}
	object, _ := event.(map[string]any)
	if object == nil {
		return "", false, nil
	}
	if value, present := object["error"]; present && translatorTruthy(value) {
		var raw map[string]json.RawMessage
		_ = json.Unmarshal([]byte(payload), &raw)
		message := translatorErrorMessage(value, raw["error"])
		return "", false, &protocol.ProviderError{
			Kind:     "protocol",
			Message:  "translator error event: " + truncateTranslatorText(message, bodyLimit),
			Fallback: "Switch LLM_PRESET, or check the endpoint and model.",
		}
	}
	if choices, ok := object["choices"].([]any); ok && len(choices) > 0 {
		if choice, ok := choices[0].(map[string]any); ok {
			if delta, ok := choice["delta"].(map[string]any); ok {
				if value, ok := delta["content"].(string); ok && value != "" {
					return value, false, nil
				}
			}
		}
	}
	return "", false, nil
}

func (t *OpenAITranslator) transportError(ctx context.Context, err error) error {
	var netError net.Error
	if errors.Is(ctx.Err(), context.DeadlineExceeded) || (errors.As(err, &netError) && netError.Timeout()) {
		return t.timeoutError()
	}
	return &protocol.ProviderError{
		Kind:     "unreachable",
		Message:  fmt.Sprintf("translator unreachable (%s): %v", t.preset.ID, err),
		Fallback: "Check the network and LLM_BASE_URL; for a local model, start Ollama or set LLM_BASE_URL.",
	}
}

func (t *OpenAITranslator) timeoutError() *protocol.ProviderError {
	return &protocol.ProviderError{
		Kind:     "timeout",
		Message:  fmt.Sprintf("translator timed out after %d ms (%s)", t.timeout.Milliseconds(), t.preset.ID),
		Fallback: "Retry, switch LLM_PRESET, or choose a lighter LLM_MODEL.",
	}
}

func (t *OpenAITranslator) statusError(status int, detail string) error {
	snippet := truncateTranslatorText(strings.Join(strings.Fields(detail), " "), bodyLimit)
	if t.apiKey != "" {
		snippet = strings.ReplaceAll(snippet, t.apiKey, "<redacted>")
	}
	suffix := ""
	if snippet != "" {
		suffix = ": " + snippet
	}
	switch status {
	case http.StatusUnauthorized, http.StatusForbidden:
		return &protocol.ProviderError{
			Kind:     "auth",
			Message:  fmt.Sprintf("translator auth failed (%d) from %s%s", status, t.preset.ID, suffix),
			Fallback: "Check LLM_API_KEY; to run without a key, unset the LLM_* variables for Demo Mode.",
		}
	case http.StatusTooManyRequests:
		return &protocol.ProviderError{
			Kind:     "rate_limit",
			Message:  fmt.Sprintf("translator rate limited (429) from %s%s", t.preset.ID, suffix),
			Fallback: "Wait a few seconds, switch LLM_PRESET, or choose a lighter LLM_MODEL.",
		}
	default:
		return &protocol.ProviderError{
			Kind:     "protocol",
			Message:  fmt.Sprintf("translator protocol error (%d) from %s%s", status, t.preset.ID, suffix),
			Fallback: "Check LLM_BASE_URL and LLM_MODEL for this Preset.",
		}
	}
}

func firstNonEmptyTranslatorOption(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func parseTranslatorExtraBody(value string) (map[string]any, error) {
	if value == "" {
		return map[string]any{}, nil
	}
	var parsed any
	if err := json.Unmarshal([]byte(value), &parsed); err != nil {
		return nil, fmt.Errorf("LLM_EXTRA_BODY is not valid JSON: %v", err)
	}
	extra, ok := parsed.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("LLM_EXTRA_BODY must be a JSON object")
	}
	return extra, nil
}

func parseTranslatorBaseURL(raw, id string) (string, error) {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "", fmt.Errorf("preset %q has an invalid LLM_BASE_URL", id)
	}
	parsed.Scheme = strings.ToLower(parsed.Scheme)
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", fmt.Errorf("preset %q LLM_BASE_URL must be http or https", id)
	}
	if parsed.User != nil {
		return "", fmt.Errorf("preset %q LLM_BASE_URL must not embed credentials; use LLM_API_KEY", id)
	}
	return strings.TrimRight(parsed.String(), "/"), nil
}

// translatorErrorMessage mirrors the JavaScript adapter's
// `event.error.message ?? JSON.stringify(event.error)` followed by String().
func translatorErrorMessage(value any, raw json.RawMessage) string {
	if object, ok := value.(map[string]any); ok {
		if message, present := object["message"]; present && message != nil {
			return translatorJSString(message)
		}
	}
	return translatorJSONStringify(raw)
}

// translatorJSString mirrors JavaScript String(value) for parsed JSON.
func translatorJSString(value any) string {
	switch typed := value.(type) {
	case nil:
		return "null"
	case string:
		return typed
	case bool:
		return strconv.FormatBool(typed)
	case float64:
		return translatorJSNumber(typed)
	case []any:
		// String(array) joins String(element), with null/undefined as "".
		parts := make([]string, len(typed))
		for index, element := range typed {
			if element != nil {
				parts[index] = translatorJSString(element)
			}
		}
		return strings.Join(parts, ",")
	}
	return "[object Object]"
}

// translatorJSNumber mirrors JavaScript Number#toString: plain decimal for
// 1e-6 <= |x| < 1e21, exponential outside it, and no leading zero in the
// exponent ("1e-7", not Go's "1e-07").
func translatorJSNumber(value float64) string {
	if value == 0 {
		return "0" // JS String(-0) is "0"
	}
	absolute := math.Abs(value)
	if absolute >= 1e-6 && absolute < 1e21 {
		return strconv.FormatFloat(value, 'f', -1, 64)
	}
	return strings.Replace(strconv.FormatFloat(value, 'g', -1, 64), "e-0", "e-", 1)
}

// translatorJSONStringify mirrors JSON.stringify for an error value: it drops
// insignificant whitespace but keeps the wire order of object keys, which a
// map marshal would sort.
// shortcut: numbers and escapes keep their wire spelling (JSON.parse would
// normalize 1.0 to 1); revisit only if an upstream sends that.
func translatorJSONStringify(raw json.RawMessage) string {
	var compact bytes.Buffer
	if err := json.Compact(&compact, raw); err != nil {
		return string(raw)
	}
	return compact.String()
}

// translatorTruthy matches JavaScript's treatment of a JSON `error` value.
func translatorTruthy(value any) bool {
	switch typed := value.(type) {
	case nil:
		return false
	case bool:
		return typed
	case string:
		return typed != ""
	case float64:
		return typed != 0
	}
	return true
}

func truncateTranslatorText(text string, limit int) string {
	if utf8.RuneCountInString(text) <= limit {
		return text
	}
	return string([]rune(text)[:limit])
}
