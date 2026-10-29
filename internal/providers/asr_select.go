// ASR Provider selection: ASR_PROVIDER names one adapter, exactly like the
// JavaScript providers/index.js convention. Unknown names fail loudly.

package providers

import (
	"fmt"

	"github.com/wray-lee/parley/internal/core"
)

// ASROptions carries the real ASR Provider settings of the Server Host.
type ASROptions struct {
	WhisperLiveKitURL string
	DeepgramAPIKey    string
	DashscopeAPIKey   string
}

// SelectASR builds the ASR Provider named by ASR_PROVIDER.
func SelectASR(name string, options ASROptions, sample Sample) (core.ASR, error) {
	switch name {
	case "mock":
		return NewMockASR(sample), nil
	case "deepgram":
		return NewDeepgramASR(DeepgramOptions{APIKey: options.DeepgramAPIKey}), nil
	case "paraformer":
		return NewParaformerASR(ParaformerOptions{APIKey: options.DashscopeAPIKey}), nil
	case "whisperlivekit":
		return NewWhisperLiveKitASR(WhisperLiveKitOptions{URL: options.WhisperLiveKitURL}), nil
	}
	return nil, fmt.Errorf("ASR_PROVIDER %q is not supported; use mock, deepgram, paraformer or whisperlivekit", name)
}
