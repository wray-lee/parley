// Azure Speech REST TTS (F0 free tier): POST SSML, receive an mp3 body.
// Voices are per-language data in config/voices.json; AZURE_SPEECH_KEY and
// AZURE_SPEECH_REGION reach this adapter through the Server Host config.
// Ported from core/providers/tts-azure.js.

package providers

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/wray-lee/parley/internal/protocol"
)

const (
	azureOutputFormat = "audio-24khz-48kbitrate-mono-mp3"
	azureFallback     = "Set TTS_PROVIDER=system or mock to run without Azure."
	// shortcut: one whole-request timeout, add a first-audio timeout if Azure stalls.
	azureTimeout = 30 * time.Second
)

var azureRegionPattern = regexp.MustCompile(`^[a-z0-9-]+$`)

// AzureOptions are the Server Host's AZURE_SPEECH_* settings and the voices data.
type AzureOptions struct {
	AzureKey    string
	AzureRegion string
	Voices      map[string][]string
	// Endpoint and HTTPClient are test seams; empty means the region endpoint.
	Endpoint   string
	HTTPClient *http.Client
	Timeout    time.Duration
	Warn       func(string)
}

// AzureTTS is the Azure Speech REST Provider.
type AzureTTS struct {
	endpoint string
	key      string
	voices   map[string][]string
	client   *http.Client
	timeout  time.Duration
	warn     func(string)
}

// NewAzureTTS validates the key and region exactly like the JavaScript adapter.
func NewAzureTTS(options AzureOptions) (*AzureTTS, error) {
	key := strings.TrimSpace(options.AzureKey)
	region := strings.TrimSpace(options.AzureRegion)
	if key == "" {
		return nil, fmt.Errorf("Azure TTS requires AZURE_SPEECH_KEY. %s", azureFallback)
	}
	if !azureRegionPattern.MatchString(region) {
		return nil, fmt.Errorf("Azure TTS requires AZURE_SPEECH_REGION (for example eastus). %s", azureFallback)
	}
	endpoint := options.Endpoint
	if endpoint == "" {
		endpoint = fmt.Sprintf("https://%s.tts.speech.microsoft.com/cognitiveservices/v1", region)
	}
	client := options.HTTPClient
	if client == nil {
		client = http.DefaultClient
	}
	timeout := options.Timeout
	if timeout == 0 {
		timeout = azureTimeout
	}
	warn := options.Warn
	if warn == nil {
		warn = func(string) {}
	}
	return &AzureTTS{
		endpoint: endpoint, key: key, voices: options.Voices,
		client: client, timeout: timeout, warn: warn,
	}, nil
}

// Name is the Provider name.
func (a *AzureTTS) Name() string { return "azure" }

// Synthesize posts the SSML and returns the mp3 body.
func (a *AzureTTS) Synthesize(request protocol.TTSRequest) (protocol.Audio, error) {
	voices, err := voiceList("Azure", a.voices, request.Direction.Tgt, azureFallback, a.warn)
	if err != nil {
		return protocol.Audio{}, err
	}
	voice := voices[0]
	lang := voiceLanguage(voice)
	if lang == "" {
		lang = request.Direction.Tgt
	}
	ssml := "<speak version='1.0' xmlns='http://www.w3.org/2001/10/synthesis' xml:lang='" +
		escapeXML(lang) + "'><voice name='" + escapeXML(voice) + "'>" +
		escapeXML(request.Text) + "</voice></speak>"

	ctx, cancel := context.WithTimeout(context.Background(), a.timeout)
	defer cancel()
	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodPost, a.endpoint, strings.NewReader(ssml))
	if err != nil {
		return protocol.Audio{}, err
	}
	httpRequest.Header.Set("Ocp-Apim-Subscription-Key", a.key)
	httpRequest.Header.Set("Content-Type", "application/ssml+xml")
	httpRequest.Header.Set("X-Microsoft-OutputFormat", azureOutputFormat)
	httpRequest.Header.Set("User-Agent", "parley")

	response, err := a.client.Do(httpRequest)
	if err != nil {
		return protocol.Audio{}, fmt.Errorf("Azure TTS is unreachable at %s: %v. %s", a.endpoint, err, azureFallback)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode > 299 {
		detail, _ := io.ReadAll(io.LimitReader(response.Body, 4096))
		return protocol.Audio{}, azureStatusError(response.StatusCode, string(detail))
	}
	data, err := io.ReadAll(response.Body)
	if err != nil {
		return protocol.Audio{}, fmt.Errorf("Azure TTS at %s: %v. %s", a.endpoint, err, azureFallback)
	}
	if len(data) == 0 {
		return protocol.Audio{}, fmt.Errorf("Azure TTS returned no audio. %s", azureFallback)
	}
	return protocol.Audio{Format: "mp3", Data: data}, nil
}

func azureStatusError(status int, detail string) error {
	trimmed := strings.TrimSpace(detail)
	if runes := []rune(trimmed); len(runes) > 200 {
		trimmed = string(runes[:200])
	}
	suffix := ""
	if trimmed != "" {
		suffix = ": " + trimmed
	}
	switch status {
	case http.StatusUnauthorized, http.StatusForbidden:
		return fmt.Errorf(
			"Azure TTS rejected the key or region (HTTP %d). "+
				"Check AZURE_SPEECH_KEY and AZURE_SPEECH_REGION. %s", status, azureFallback)
	case http.StatusTooManyRequests:
		return fmt.Errorf("Azure TTS is rate limited (HTTP %d). Wait and retry. %s", status, azureFallback)
	}
	return fmt.Errorf("Azure TTS failed (HTTP %d)%s. %s", status, suffix, azureFallback)
}

var xmlEscaper = strings.NewReplacer(
	"&", "&amp;",
	"<", "&lt;",
	">", "&gt;",
	`"`, "&quot;",
	"'", "&apos;",
)

func escapeXML(value string) string { return xmlEscaper.Replace(value) }
