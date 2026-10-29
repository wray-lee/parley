package core_test

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/wray-lee/parley"
	"github.com/wray-lee/parley/internal/core"
	"github.com/wray-lee/parley/internal/protocol"
	"github.com/wray-lee/parley/internal/providers"
)

// The byte-level hello comparison: the v0.1 Go hello may add only `protocol`
// and `tts` to the frozen JavaScript oracle hello. The fixture is the JSON the
// JS core emitted for the canonical registry (spec/fixtures/hello-v0.0.3.json).
func TestHelloAddsOnlyProtocolAndTTS(t *testing.T) {
	fixture, err := os.ReadFile("../../spec/fixtures/hello-v0.0.3.json")
	if err != nil {
		t.Fatal(err)
	}
	oracle := strings.TrimSpace(string(fixture))

	registry, err := core.LoadRegistry(parley.Assets, func(string) {})
	if err != nil {
		t.Fatal(err)
	}
	session := core.NewSession(core.SessionConfig{
		Providers: core.Providers{
			ASR:        &scriptedASR{name: "mock"},
			Translator: &scriptedTranslator{name: "mock"},
			TTS:        providers.NewMockTTS(),
		},
		Registry: registry,
	})
	hello := session.Hello()
	if hello.Protocol != 1 {
		t.Fatalf("hello.protocol = %d, want 1", hello.Protocol)
	}
	if hello.TTS != "audio" {
		t.Fatalf("hello.tts = %q, want audio for the mock TTS", hello.TTS)
	}

	got, err := json.Marshal(hello)
	if err != nil {
		t.Fatal(err)
	}
	want := strings.TrimSuffix(oracle, "}") + `,"protocol":1,"tts":"audio"}`
	if string(got) != want {
		t.Fatalf("hello bytes differ from the oracle plus the two additive fields\n got: %s\nwant: %s", got, want)
	}

	systemSession := core.NewSession(core.SessionConfig{
		Providers: core.Providers{
			ASR:        &scriptedASR{name: "mock"},
			Translator: &scriptedTranslator{name: "mock"},
			TTS:        providers.NewSystemTTS(),
		},
		Registry: registry,
	})
	if hello := systemSession.Hello(); hello.TTS != "system" {
		t.Fatalf("hello.tts = %q, want system for the text-only Provider", hello.TTS)
	}
}

// hello's TTS field is the only provider-dependent difference.
func TestHelloTTSIsSystemOrAudio(t *testing.T) {
	registry, err := core.LoadRegistry(parley.Assets, func(string) {})
	if err != nil {
		t.Fatal(err)
	}
	for _, testCase := range []struct {
		tts  core.TTS
		want string
	}{
		{providers.NewMockTTS(), "audio"},
		{providers.NewSystemTTS(), "system"},
	} {
		session := core.NewSession(core.SessionConfig{
			Providers: core.Providers{ASR: &scriptedASR{name: "s"}, Translator: &scriptedTranslator{name: "s"}, TTS: testCase.tts},
			Registry:  registry,
		})
		var hello protocol.Hello = session.Hello()
		if hello.TTS != testCase.want {
			t.Fatalf("hello.tts = %q, want %q", hello.TTS, testCase.want)
		}
	}
}
