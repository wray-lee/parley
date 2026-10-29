//go:build live

package providers_test

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/wray-lee/parley"
	"github.com/wray-lee/parley/internal/core"
	"github.com/wray-lee/parley/internal/protocol"
	"github.com/wray-lee/parley/internal/providers"
)

// One real OpenAI-compatible endpoint, driven by the same LLM_* variables as
// the Server Host. Skipped without a key, so CI never touches the network.
func TestLiveOpenAICompatibleTranslator(t *testing.T) {
	apiKey := os.Getenv("LLM_API_KEY")
	if apiKey == "" {
		t.Skip("LLM_API_KEY is not set")
	}
	presets, err := providers.LoadPresets(parley.Assets)
	if err != nil {
		t.Fatal(err)
	}
	translator, err := providers.NewOpenAITranslator(presets, providers.TranslatorOptions{
		Preset:    os.Getenv("LLM_PRESET"),
		BaseURL:   os.Getenv("LLM_BASE_URL"),
		APIKey:    apiKey,
		Model:     os.Getenv("LLM_MODEL"),
		ExtraBody: os.Getenv("LLM_EXTRA_BODY"),
		Timeout:   45 * time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	prompt := core.BuildTranslatorRequest("Translate the user's English into Chinese. Output only the translation.", nil, "Hello, world.", nil)
	var output strings.Builder
	err = translator.Translate(protocol.TranslatorRequest{
		System: prompt.System, User: prompt.User, Text: "Hello, world.",
	}, func(chunk string) { output.WriteString(chunk) })
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(output.String()) == "" {
		t.Fatal("live translator returned no text")
	}
	t.Logf("translation: %s", output.String())
}
