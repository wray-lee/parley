// Package protocol defines the wire vocabulary of the Session core: client
// messages the host passes in, events the core emits and the hello handshake.
// Field names are fixed by the frozen JavaScript oracle (spec/traces).
package protocol

// Direction is one translation path src→tgt.
type Direction struct {
	Src string `json:"src"`
	Tgt string `json:"tgt"`
}

// ClientMessage is one JSON message from the client.
type ClientMessage struct {
	Type   string   `json:"type"`
	Mode   string   `json:"mode,omitempty"`
	Src    string   `json:"src,omitempty"`
	Tgt    string   `json:"tgt,omitempty"`
	Packs  []string `json:"packs,omitempty"`
	Name   string   `json:"name,omitempty"`
	Source string   `json:"source,omitempty"`
	Who    string   `json:"who,omitempty"`
	SegID  int      `json:"seg_id,omitempty"`
	State  string   `json:"state,omitempty"`
}

// Event is one JSON event the core emits. TTSChunkEvent is sent as a binary
// WebSocket frame by the host; every other event is JSON text.
type Event interface{ isEvent() }

// StateEvent reports the Session lifecycle state.
type StateEvent struct {
	Type  string `json:"type"`
	State string `json:"state"`
}

// ErrorEvent surfaces a Provider or protocol failure with a fallback path.
type ErrorEvent struct {
	Type     string `json:"type"`
	Provider string `json:"provider,omitempty"`
	Kind     string `json:"kind"`
	Message  string `json:"message"`
	Fallback string `json:"fallback,omitempty"`
}

// TurnEvent reports the Converse Turn that receives audio.
type TurnEvent struct {
	Type string `json:"type"`
	Who  string `json:"who"`
	Auto bool   `json:"auto"`
}

// HoldEvent reports that a Segment is Held for a later Cut to merge into.
type HoldEvent struct {
	Type      string    `json:"type"`
	Held      bool      `json:"held"`
	Merges    int       `json:"merges"`
	Direction Direction `json:"direction"`
}

// ASAPartialEvent is a display-only ASR hypothesis.
type ASAPartialEvent struct {
	Type      string    `json:"type"`
	Text      string    `json:"text"`
	Direction Direction `json:"direction"`
}

// SegmentEvent is one committed Segment. Committed output is never revised.
type SegmentEvent struct {
	Type      string    `json:"type"`
	SegID     int       `json:"seg_id"`
	Src       string    `json:"src"`
	Tgt       string    `json:"tgt"`
	Direction Direction `json:"direction"`
	LatencyMS int64     `json:"latency_ms"`
	Speak     bool      `json:"speak"`
	Demo      bool      `json:"demo"`
}

// TTSBeginEvent announces synthesized audio for a Segment.
type TTSBeginEvent struct {
	Type   string `json:"type"`
	SegID  int    `json:"seg_id"`
	Format string `json:"format"`
}

// TTSChunkEvent carries audio bytes; the host sends them as a binary frame.
type TTSChunkEvent struct {
	Type  string `json:"type"`
	SegID int    `json:"seg_id"`
	Data  []byte `json:"data"`
}

// TTSEndEvent closes the audio of a Segment.
type TTSEndEvent struct {
	Type  string `json:"type"`
	SegID int    `json:"seg_id"`
}

func (*StateEvent) isEvent()      {}
func (*TurnEvent) isEvent()       {}
func (*HoldEvent) isEvent()       {}
func (*ErrorEvent) isEvent()      {}
func (*ASAPartialEvent) isEvent() {}
func (*SegmentEvent) isEvent()    {}
func (*TTSBeginEvent) isEvent()   {}
func (*TTSChunkEvent) isEvent()   {}
func (*TTSEndEvent) isEvent()     {}

// SegmentLog is one JSON line of the host's Segment log.
type SegmentLog struct {
	SegID         int       `json:"seg_id"`
	Direction     Direction `json:"direction"`
	SpeechEnd     int64     `json:"speech_end"`
	ASRFinal      string    `json:"asr_final"`
	ASRFinalAt    *int64    `json:"asr_final_at"`
	FirstToken    *int64    `json:"first_token"`
	TTSFirstChunk *int64    `json:"tts_first_chunk"`
	Held          bool      `json:"held"`
	Merges        int       `json:"merges"`
	Forced        bool      `json:"forced"`
	ForcedFail    bool      `json:"forced_fail"`
	EchoSuspect   int       `json:"echo_suspect"`
}

// TranslatorRequest is the fixed request shape both cores send to a translator.
type TranslatorRequest struct {
	System    string    `json:"system"`
	User      string    `json:"user"`
	Text      string    `json:"text"`
	Direction Direction `json:"direction"`
}

// TTSRequest is the request shape the core sends to a TTS Provider.
type TTSRequest struct {
	Text      string    `json:"text"`
	SegID     int       `json:"seg_id"`
	Direction Direction `json:"direction"`
}

// Audio is one synthesized Segment: format name and encoded bytes.
type Audio struct {
	Format string
	Data   []byte
}

// Pair is a Language Pair as the client sees it.
type Pair struct {
	A          string      `json:"a"`
	B          string      `json:"b"`
	Directions []Direction `json:"directions"`
	Converse   bool        `json:"converse"`
}

// PackRef names one Domain Pack for one Direction.
type PackRef struct {
	Name string `json:"name"`
	Src  string `json:"src"`
	Tgt  string `json:"tgt"`
}

// Preset is the client-visible part of an LLM Preset.
type Preset struct {
	ID           string `json:"id"`
	TrainsOnData bool   `json:"trainsOnData"`
}

// Hello is the first message on every WebSocket connection.
type Hello struct {
	Type         string    `json:"type"`
	Pairs        []Pair    `json:"pairs"`
	Packs        []PackRef `json:"packs"`
	PresetsInUse []Preset  `json:"presetsInUse"`
	Demo         bool      `json:"demo"`
	// Protocol is the additive v0.1 field: 1 names the core-owned Gating
	// protocol. Every other byte stays as v0.0.3 sent it.
	Protocol int `json:"protocol"`
	// TTS is the additive v0.1 field: "system" when the client must speak
	// Segment text with OS TTS, "audio" when the host streams chunks.
	TTS string `json:"tts"`
}

// ProviderError carries the kind and fallback the ErrorEvent exposes.
type ProviderError struct {
	Kind     string
	Message  string
	Fallback string
}

func (e *ProviderError) Error() string { return e.Message }
