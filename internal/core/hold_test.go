package core

import "testing"

func TestHoldPolicyMergesAndCounts(t *testing.T) {
	policy := NewHoldPolicy("")
	if policy.Inspect("⏸") != true || policy.Inspect("⏸ is the marker") {
		t.Fatal("only a lone ⏸ is the Hold marker")
	}

	held := policy.MarkHeld("part one", 1000, 1000, &Direction{Src: "zh", Tgt: "ja"}, 1000)
	if held != policy.Held() || held.merges != 0 || !held.held {
		t.Fatalf("held = %+v", held)
	}

	policy.Merge("part two", 1200, 1200)
	merged := policy.Merge("part three", 1400, 1400)
	if merged.source != "part one part two part three" || merged.merges != 2 || merged.speechEnd != 1400 {
		t.Fatalf("merged = %+v", merged)
	}
}

func TestHoldPolicyExpiresAt2500ms(t *testing.T) {
	policy := NewHoldPolicy("")
	policy.MarkHeld("x", 1000, 1000, &Direction{}, 1000)

	if policy.MSLeft(1000) != 2500 || policy.MSLeft(3000) != 500 || policy.MSLeft(4000) != 0 {
		t.Fatal("msLeft does not match the 2.5 s limit")
	}
	if policy.Expired(3499) || !policy.Expired(3500) {
		t.Fatal("expiry must be exactly 2500 ms after HeldAt")
	}

	policy.Force()
	if policy.Expired(9000) {
		t.Fatal("a Forced Segment never expires again")
	}
}

func TestHoldPolicyAppendsTheForcedInstruction(t *testing.T) {
	policy := NewHoldPolicy("FORCE NOW")
	request := policy.WithInstruction(TranslatorPrompt{System: "SYSTEM", User: "USER"})
	if request.System != "SYSTEM\nFORCE NOW" || request.User != "USER" {
		t.Fatalf("request = %+v", request)
	}

	empty := NewHoldPolicy("").WithInstruction(TranslatorPrompt{System: "SYSTEM"})
	if empty.System != "SYSTEM" {
		t.Fatalf("empty instruction changed the system prompt: %+v", empty)
	}

	policy.Reset()
	if policy.Held() != nil || policy.Merge("x", 0, 0) != nil {
		t.Fatal("Reset must forget the Held Segment")
	}
}
