package trace_test

import (
	"os"
	"strings"
	"testing"

	"github.com/wray-lee/parley/internal/trace"
)

const listenTrace = "../../spec/traces/listen-basic.jsonl"

var listenTraces = []string{
	"../../spec/traces/listen-basic.jsonl",
	"../../spec/traces/listen-live-gating.jsonl",
}

func TestListenTracesReplayDeterministically(t *testing.T) {
	for _, name := range listenTraces {
		for run := 0; run < 2; run++ {
			if err := trace.ReplayFile(name); err != nil {
				t.Fatalf("%s run %d: %v", name, run, err)
			}
		}
	}
}

func TestReplayFailsOnAnyOutputDifference(t *testing.T) {
	data, err := os.ReadFile(listenTrace)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(string(data), "\n")
	changed := false
	for index, line := range lines {
		if strings.Contains(line, `"type":"output"`) && strings.Contains(line, `"event":{"type":"segment"`) {
			lines[index] = strings.Replace(line, `"src":"one"`, `"src":"ONE"`, 1)
			changed = true
		}
	}
	if !changed {
		t.Fatal("trace has no segment output line to corrupt")
	}
	if err := trace.Replay(strings.Join(lines, "\n")); err == nil {
		t.Fatal("a changed expected output must fail the replay")
	}
}
