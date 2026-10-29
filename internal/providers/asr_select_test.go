package providers_test

import (
	"testing"

	"github.com/wray-lee/parley/internal/providers"
)

func TestSelectASRResolvesTheThreeAdapters(t *testing.T) {
	options := providers.ASROptions{
		DeepgramAPIKey: "dg", DashscopeAPIKey: "ds", WhisperLiveKitURL: "ws://wlk.test:8000",
	}
	for _, testCase := range []struct {
		name string
		want string
	}{
		{"mock", "mock"},
		{"deepgram", "deepgram"},
		{"paraformer", "paraformer"},
		{"whisperlivekit", "whisperlivekit"},
	} {
		asr, err := providers.SelectASR(testCase.name, options, providers.Sample{})
		if err != nil {
			t.Fatalf("SelectASR(%q): %v", testCase.name, err)
		}
		if asr.Name() != testCase.want {
			t.Fatalf("SelectASR(%q).Name() = %q", testCase.name, asr.Name())
		}
		if !asr.AcceptsLive() && testCase.name != "mock" {
			t.Fatalf("%s must accept live audio", testCase.name)
		}
		if asr.Revisable() {
			t.Fatalf("%s emits its own Stable Prefix", testCase.name)
		}
	}
	if _, err := providers.SelectASR("../evil", options, providers.Sample{}); err == nil {
		t.Fatal("unknown ASR names must fail")
	}
}
