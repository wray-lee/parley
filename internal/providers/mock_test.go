package providers_test

import (
	"encoding/binary"
	"strings"
	"testing"

	"github.com/wray-lee/parley/internal/protocol"
	"github.com/wray-lee/parley/internal/providers"
)

func sample() providers.Sample {
	return providers.Sample{
		TimedTranscript:       []providers.TranscriptEntry{{StartMS: 0, EndMS: 100, Text: "hello"}},
		ReferenceTranslations: map[string]string{"hello": "bonjour"},
	}
}

func TestSelectResolvesProvidersByKindAndName(t *testing.T) {
	asr, translator, tts, err := providers.Select(providers.Config{
		ASR: "mock", Translator: "mock", TTS: "mock", Sample: sample(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if asr.Name() != "mock" || translator.Name() != "mock" || tts.Name() != "mock" {
		t.Fatalf("providers = %s/%s/%s", asr.Name(), translator.Name(), tts.Name())
	}
}

func TestSelectRefusesUnknownProviderNames(t *testing.T) {
	for _, config := range []providers.Config{
		{ASR: "../evil", Translator: "mock", TTS: "mock"},
		{ASR: "no-such-provider", Translator: "mock", TTS: "mock"},
		{ASR: "mock", Translator: "no-such-provider", TTS: "mock"},
		{ASR: "mock", Translator: "mock", TTS: "no-such-provider"},
	} {
		if _, _, _, err := providers.Select(config); err == nil {
			t.Fatalf("Select(%+v) accepted an unknown Provider", config)
		}
	}
}

func TestMockASRReplaysTheTimedTranscript(t *testing.T) {
	asr := providers.NewMockASR(sample())
	asr.Open(protocol.Direction{Src: "zh", Tgt: "ja"})
	if asr.AcceptsLive() || asr.Revisable() {
		t.Fatal("mock ASR accepts bundled samples only and is not revisable")
	}
	if events, _ := asr.Push(nil, 20); len(events) != 1 || events[0].Type != "partial" || events[0].Text != "hello" {
		t.Fatalf("partial = %+v", events)
	}
	if events, _ := asr.Push(nil, 120); len(events) != 1 || events[0].Type != "stable" || events[0].Text != "hello" {
		t.Fatalf("stable = %+v", events)
	}
	if events, _ := asr.Push(nil, 140); len(events) != 0 {
		t.Fatalf("extra events = %+v", events)
	}
}

func TestMockTranslatorReturnsTheReferenceWithADemoMarker(t *testing.T) {
	translator := providers.NewMockTranslator(sample().ReferenceTranslations)
	var chunks []string
	err := translator.Translate(protocol.TranslatorRequest{Text: "hello"}, func(chunk string) {
		chunks = append(chunks, chunk)
	})
	if err != nil || strings.Join(chunks, "") != "[DEMO] bonjour" {
		t.Fatalf("chunks = %q err = %v", chunks, err)
	}
}

func TestMockTTSReturnsAShortDecodableTone(t *testing.T) {
	audio, err := providers.NewMockTTS().Synthesize(protocol.TTSRequest{Text: "x"})
	if err != nil || audio.Format != "wav" {
		t.Fatalf("audio = %+v err = %v", audio, err)
	}
	if len(audio.Data) < 44 || string(audio.Data[0:4]) != "RIFF" || string(audio.Data[8:12]) != "WAVE" {
		t.Fatalf("not a WAV: %x", audio.Data[:min(12, len(audio.Data))])
	}
	if size := int(binary.LittleEndian.Uint32(audio.Data[4:])); size+8 != len(audio.Data) {
		t.Fatalf("RIFF size %d does not match %d bytes", size, len(audio.Data))
	}
}
