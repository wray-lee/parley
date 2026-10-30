package core

// White-box checks for the two Auto Turn inputs that cannot reach the public
// Session seam once Gating works: own playback pending, and TTS echo.

import (
	"testing"

	"github.com/wray-lee/parley/internal/protocol"
)

func autoSessionForInternalTest() *Session {
	start := &Direction{Src: "zh", Tgt: "ja"}
	them := &Direction{Src: "ja", Tgt: "zh"}
	return &Session{
		cfg:           SessionConfig{Clock: &echoClock{now: 1000}, Emit: func(protocol.Event) {}},
		mode:          "converse",
		start:         protocol.Direction{Src: "zh", Tgt: "ja"},
		turn:          "me",
		direction:     start,
		directions:    map[string]*Direction{"me": start, "them": them},
		autoEnabled:   true,
		autoAvailable: true,
		gating:        &Gating{},
		echo:          NewEchoFilter(),
		playbacks:     make(map[int]*playback),
		cut:           NewCutPolicy(PairConfig{ClauseLength: 1000, Unit: "char"}),
	}
}

func TestAutoTurnSuppressedWhileOwnSpeechIsPending(t *testing.T) {
	session := autoSessionForInternalTest()
	session.tags = []LanguageTag{{Language: "ja", Confidence: 1}, {Language: "ja", Confidence: 1}}
	session.ownSpeechPending = 1

	if _, who, language, uncertain := session.decideAutoTurn(session.direction); who != "me" || language != "" || uncertain {
		t.Fatalf("decided while playback pending: who=%q language=%q uncertain=%v", who, language, uncertain)
	}

	session.ownSpeechPending = 0
	if _, who, language, _ := session.decideAutoTurn(session.direction); who != "them" || language != "ja" {
		t.Fatalf("after playback: who=%q language=%q", who, language)
	}
}

func TestAutoTurnDropsTagsWithAnEchoSuspect(t *testing.T) {
	session := autoSessionForInternalTest()
	session.input = "live"
	session.beginOwnSpeech(1, "hello world", nil)

	session.addStable("hello world", " ", []LanguageTag{{Language: "ja"}, {Language: "ja"}})
	if len(session.tags) != 0 {
		t.Fatalf("echo tags = %+v, want dropped with the echoed text", session.tags)
	}
	if session.stable != "" {
		t.Fatalf("stable = %q", session.stable)
	}
}
