// Mock Providers: the keyless Demo Mode shipped by the Bun host. A bundled
// sample's timed transcript drives ASR, a reference translation is prefixed
// with the DEMO marker, and TTS is a short generated tone.

package providers

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io/fs"
	"math"

	"github.com/wray-lee/parley/internal/core"
	"github.com/wray-lee/parley/internal/protocol"
)

// TranscriptEntry is one timed transcript line of a bundled sample.
type TranscriptEntry struct {
	StartMS int64  `json:"start_ms"`
	EndMS   int64  `json:"end_ms"`
	Text    string `json:"text"`
}

// Sample is one bundled sample under samples/<id>.json.
type Sample struct {
	ID                    string            `json:"id"`
	Src                   string            `json:"src"`
	Tgt                   string            `json:"tgt"`
	AudioFile             string            `json:"audio_file"`
	DurationMS            int64             `json:"duration_ms"`
	TimedTranscript       []TranscriptEntry `json:"timed_transcript"`
	ReferenceTranslations map[string]string `json:"reference_translations"`
}

// LoadSample reads samples/<id>.json from fsys.
func LoadSample(fsys fs.FS, id string) (Sample, error) {
	data, err := fs.ReadFile(fsys, "samples/"+id+".json")
	if err != nil {
		return Sample{}, fmt.Errorf("read sample %s: %w", id, err)
	}
	var sample Sample
	if err := json.Unmarshal(data, &sample); err != nil {
		return Sample{}, fmt.Errorf("parse sample %s: %w", id, err)
	}
	return sample, nil
}

// MockASR replays a sample's timed transcript. It refuses live microphone
// audio so the demo never pretends to listen.
type MockASR struct {
	transcript []TranscriptEntry
	partial    []bool
	stable     []bool
}

// NewMockASR builds a mock ASR for one sample.
func NewMockASR(sample Sample) *MockASR {
	return &MockASR{transcript: sample.TimedTranscript}
}

func (m *MockASR) Name() string      { return "mock" }
func (m *MockASR) AcceptsLive() bool { return false }
func (m *MockASR) Revisable() bool   { return false }
func (m *MockASR) Close()            {}

// Open resets the transcript playback for a new Session or sample replay.
func (m *MockASR) Open(protocol.Direction) {
	m.partial = make([]bool, len(m.transcript))
	m.stable = make([]bool, len(m.transcript))
}

// Push emits transcript lines whose start or end time the audio has reached.
func (m *MockASR) Push(_ []int16, audioMS int64) ([]core.ASREvent, error) {
	if m.partial == nil {
		m.Open(protocol.Direction{})
	}
	events := []core.ASREvent{}
	for index, entry := range m.transcript {
		if !m.partial[index] && audioMS >= entry.StartMS {
			m.partial[index] = true
			events = append(events, core.ASREvent{Type: "partial", Text: entry.Text})
		}
		if !m.stable[index] && audioMS >= entry.EndMS {
			m.stable[index] = true
			events = append(events, core.ASREvent{Type: "stable", Text: entry.Text})
		}
	}
	return events, nil
}

// MockTranslator returns the sample's reference translation with a DEMO marker.
type MockTranslator struct {
	references map[string]string
}

const demoMarker = "[DEMO] "

// NewMockTranslator builds a mock translator for one sample's references.
func NewMockTranslator(references map[string]string) *MockTranslator {
	return &MockTranslator{references: references}
}

func (m *MockTranslator) Name() string { return "mock" }

// Translate yields the whole answer as one chunk.
func (m *MockTranslator) Translate(request protocol.TranslatorRequest, onChunk func(string)) error {
	text, ok := m.references[request.Text]
	if !ok {
		text = request.Text
	}
	onChunk(demoMarker + text)
	return nil
}

// MockTTS synthesizes a short WAV tone per Segment, without dependencies.
type MockTTS struct {
	DurationMS int
	Frequency  float64
	SampleRate int
}

// NewMockTTS builds the default 120 ms 440 Hz tone.
func NewMockTTS() *MockTTS {
	return &MockTTS{DurationMS: 120, Frequency: 440, SampleRate: 16000}
}

func (m *MockTTS) Name() string { return "mock" }

// Synthesize returns the tone WAV.
func (m *MockTTS) Synthesize(protocol.TTSRequest) (protocol.Audio, error) {
	return protocol.Audio{Format: "wav", Data: toneWAV(m.DurationMS, m.Frequency, m.SampleRate)}, nil
}

func toneWAV(durationMS int, frequency float64, sampleRate int) []byte {
	samples := int(math.Round(float64(sampleRate*durationMS) / 1000))
	dataBytes := samples * 2
	buffer := make([]byte, 44+dataBytes)
	copy(buffer[0:], "RIFF")
	binary.LittleEndian.PutUint32(buffer[4:], uint32(36+dataBytes))
	copy(buffer[8:], "WAVE")
	copy(buffer[12:], "fmt ")
	binary.LittleEndian.PutUint32(buffer[16:], 16)
	binary.LittleEndian.PutUint16(buffer[20:], 1) // PCM
	binary.LittleEndian.PutUint16(buffer[22:], 1) // mono
	binary.LittleEndian.PutUint32(buffer[24:], uint32(sampleRate))
	binary.LittleEndian.PutUint32(buffer[28:], uint32(sampleRate*2))
	binary.LittleEndian.PutUint16(buffer[32:], 2)
	binary.LittleEndian.PutUint16(buffer[34:], 16)
	copy(buffer[36:], "data")
	binary.LittleEndian.PutUint32(buffer[40:], uint32(dataBytes))
	for i := 0; i < samples; i++ {
		value := math.Sin(2*math.Pi*frequency*float64(i)/float64(sampleRate)) * 0.25
		binary.LittleEndian.PutUint16(buffer[44+i*2:], uint16(int16(math.Round(value*32767))))
	}
	return buffer
}

// Config selects Providers by name. Unset names mean mock, so a fresh clone
// with no .env is Demo Mode.
type Config struct {
	ASR        string
	Translator string
	TTS        string
	Sample     Sample
}

// Select builds the three Providers or fails loudly on an unsupported name.
func Select(config Config) (core.ASR, core.Translator, core.TTS, error) {
	if config.ASR != "mock" {
		return nil, nil, nil, fmt.Errorf("ASR_PROVIDER %q is not supported by this build; use mock", config.ASR)
	}
	if config.Translator != "mock" {
		return nil, nil, nil, fmt.Errorf("translator %q is not supported by this build; use a mock setup (unset LLM_* variables)", config.Translator)
	}
	if config.TTS != "mock" {
		return nil, nil, nil, fmt.Errorf("TTS_PROVIDER %q is not supported by this build; use mock", config.TTS)
	}
	return NewMockASR(config.Sample), NewMockTranslator(config.Sample.ReferenceTranslations), NewMockTTS(), nil
}
