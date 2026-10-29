//go:build live

// Live ASR checks, ported from scripts/live-check-asr.mjs and
// scripts/live-check-paraformer.mjs. They need a real key or a local
// WhisperLiveKit service and never run in CI: every test skips cleanly when the
// environment is not configured.

package providers_test

import (
	"encoding/binary"
	"io"
	"os"
	"testing"
	"time"

	"github.com/wray-lee/parley/internal/core"
	"github.com/wray-lee/parley/internal/protocol"
	"github.com/wray-lee/parley/internal/providers"
)

const liveSample = "../../samples/ja-context.wav"

// readWAV16Mono reads a 16-bit mono PCM WAV file and returns its sample rate.
func readWAV16Mono(t *testing.T, path string) ([]int16, int) {
	t.Helper()
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	data, err := io.ReadAll(file)
	if err != nil {
		t.Fatal(err)
	}
	if len(data) < 12 || string(data[0:4]) != "RIFF" || string(data[8:12]) != "WAVE" {
		t.Fatalf("%s is not a WAV file", path)
	}
	var (
		channels int
		bits     int
		rate     int
		pcm      []int16
		sawData  bool
	)
	for offset := 12; offset+8 <= len(data); {
		id := string(data[offset : offset+4])
		size := int(binary.LittleEndian.Uint32(data[offset+4 : offset+8]))
		body := offset + 8
		switch id {
		case "fmt ":
			channels = int(binary.LittleEndian.Uint16(data[body+2 : body+4]))
			rate = int(binary.LittleEndian.Uint32(data[body+4 : body+8]))
			bits = int(binary.LittleEndian.Uint16(data[body+14 : body+16]))
		case "data":
			sawData = true
			for index := body; index+1 < body+size && index+1 < len(data); index += 2 {
				pcm = append(pcm, int16(binary.LittleEndian.Uint16(data[index:index+2])))
			}
		}
		offset = body + size
		if size%2 == 1 {
			offset++
		}
	}
	if !sawData || channels != 1 || bits != 16 || len(pcm) == 0 {
		t.Fatalf("%s: channels=%d bits=%d samples=%d", path, channels, bits, len(pcm))
	}
	return pcm, rate
}

// resample16k linearly resamples to the fixed 16 kHz wire rate.
func resample16k(pcm []int16, from int) []int16 {
	if from == 16000 {
		return pcm
	}
	count := len(pcm) * 16000 / from
	out := make([]int16, count)
	for index := range out {
		position := float64(index) * float64(from) / 16000
		left := int(position)
		right := left + 1
		if right >= len(pcm) {
			right = len(pcm) - 1
		}
		fraction := position - float64(left)
		out[index] = int16(float64(pcm[left])*(1-fraction) + float64(pcm[right])*fraction)
	}
	return out
}

// streamLive feeds the sample at 5x realtime and returns the stable text the
// adapter emitted while the Session drained.
func streamLive(t *testing.T, asr core.ASR, pcm []int16) (string, []error) {
	t.Helper()
	var (
		stable []string
		errs   []error
	)
	collect := func() {
		events, err := asr.Push(nil, 0)
		if err != nil {
			errs = append(errs, err)
		}
		for _, event := range events {
			if event.Type != "stable" {
				continue
			}
			if event.Glue != nil {
				stable = append(stable, *event.Glue)
			}
			stable = append(stable, event.Text)
		}
	}
	asr.Open(protocol.Direction{Src: "ja", Tgt: "zh"})
	const chunkMS = 100
	chunk := 16 * chunkMS
	for offset := 0; offset < len(pcm); offset += chunk {
		end := offset + chunk
		if end > len(pcm) {
			end = len(pcm)
		}
		if _, err := asr.Push(pcm[offset:end], int64(offset/16)); err != nil {
			errs = append(errs, err)
		}
		time.Sleep(20 * time.Millisecond) // 5x realtime
	}
	asr.Close()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		collect()
		time.Sleep(50 * time.Millisecond)
	}
	return joinText(stable), errs
}

func joinText(parts []string) string {
	joined := ""
	for _, part := range parts {
		joined += part
	}
	return joined
}

func TestLiveDeepgram(t *testing.T) {
	apiKey := os.Getenv("DEEPGRAM_API_KEY")
	if apiKey == "" {
		t.Skip("DEEPGRAM_API_KEY is not set")
	}
	pcm, rate := readWAV16Mono(t, liveSample)
	asr := providers.NewDeepgramASR(providers.DeepgramOptions{APIKey: apiKey})
	text, errs := streamLive(t, asr, resample16k(pcm, rate))
	if len(errs) > 0 {
		t.Fatalf("live Deepgram errors: %v", errs)
	}
	if text == "" {
		t.Fatal("live Deepgram returned no Stable Prefix")
	}
	t.Logf("deepgram stable: %s", text)
}

func TestLiveParaformer(t *testing.T) {
	apiKey := os.Getenv("DASHSCOPE_API_KEY")
	if apiKey == "" {
		t.Skip("DASHSCOPE_API_KEY is not set")
	}
	pcm, rate := readWAV16Mono(t, liveSample)
	asr := providers.NewParaformerASR(providers.ParaformerOptions{
		APIKey: apiKey, URL: os.Getenv("DASHSCOPE_URL"),
	})
	text, errs := streamLive(t, asr, resample16k(pcm, rate))
	if len(errs) > 0 {
		t.Fatalf("live Paraformer errors: %v", errs)
	}
	if text == "" {
		t.Fatal("live Paraformer returned no Stable Prefix")
	}
	t.Logf("paraformer stable: %s", text)
}

func TestLiveWhisperLiveKit(t *testing.T) {
	url := os.Getenv("WHISPERLIVEKIT_URL")
	if url == "" {
		t.Skip("WHISPERLIVEKIT_URL is not set")
	}
	pcm, rate := readWAV16Mono(t, liveSample)
	asr := providers.NewWhisperLiveKitASR(providers.WhisperLiveKitOptions{URL: url})
	text, errs := streamLive(t, asr, resample16k(pcm, rate))
	if len(errs) > 0 {
		t.Skipf("WhisperLiveKit is not reachable at %s: %v", url, errs[0])
	}
	if text == "" {
		t.Fatal("live WhisperLiveKit returned no Stable Prefix")
	}
	t.Logf("whisperlivekit stable: %s", text)
}
