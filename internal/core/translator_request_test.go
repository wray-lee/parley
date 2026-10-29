package core_test

import (
	"testing"

	"github.com/wray-lee/parley/internal/core"
)

func TestBuildTranslatorRequestMatchesOracle(t *testing.T) {
	request := core.BuildTranslatorRequest(
		"SYSTEM {{GLOSSARY}}",
		nil,
		"one",
		[]core.Term{{Src: "预算", Tgt: "予算"}, {Src: "决议", Tgt: "決議"}},
	)
	if request.System != "SYSTEM 预算 → 予算\n决议 → 決議" {
		t.Fatalf("system = %q", request.System)
	}
	if request.User != "[已译上下文]\n[新段落]\none" {
		t.Fatalf("user = %q", request.User)
	}

	withContext := core.BuildTranslatorRequest("SYSTEM ", []core.ContextPair{{Src: "one", Tgt: "translated-one"}}, "two", nil)
	if withContext.User != "[已译上下文]\n原文：one\n译文：translated-one\n[新段落]\ntwo" {
		t.Fatalf("context user = %q", withContext.User)
	}
}
