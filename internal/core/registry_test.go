package core_test

import (
	"strings"
	"testing"
	"testing/fstest"

	"github.com/wray-lee/parley"
	"github.com/wray-lee/parley/internal/core"
)

func loadAssets(t *testing.T) *core.Registry {
	t.Helper()
	var warnings []string
	registry, err := core.LoadRegistry(parley.Assets, func(line string) { warnings = append(warnings, line) })
	if err != nil {
		t.Fatalf("LoadRegistry: %v", err)
	}
	for _, warning := range warnings {
		t.Errorf("unexpected warning: %s", warning)
	}
	return registry
}

func TestRegistryDirectionsAndPacks(t *testing.T) {
	registry := loadAssets(t)

	if len(registry.Directions) != 2 {
		t.Fatalf("directions = %d, want 2", len(registry.Directions))
	}
	zh := registry.FindDirection("zh", "ja")
	if zh == nil || zh.Src != "zh" || zh.Tgt != "ja" {
		t.Fatalf("zh->ja direction missing: %+v", zh)
	}
	if zh.File != "system_zh2ja.txt" || zh.Prompt == "" {
		t.Fatalf("zh->ja prompt not loaded: %+v", zh)
	}
	if len(registry.Packs) != 2 {
		t.Fatalf("packs = %d, want 2", len(registry.Packs))
	}
	if !registry.HasPack("conference") {
		t.Fatal("conference pack missing")
	}
	if len(registry.Pairs) != 1 || registry.Pairs[0].A != "ja" || registry.Pairs[0].B != "zh" || !registry.Pairs[0].Converse {
		t.Fatalf("pairs = %+v, want one converse ja↔zh pair", registry.Pairs)
	}
}

func TestSelectGlossaryMergesPacks(t *testing.T) {
	registry := loadAssets(t)
	terms := core.SelectGlossary(registry, "zh", "ja", []string{"conference"})
	if len(terms) != 6 {
		t.Fatalf("terms = %d, want 6", len(terms))
	}
	found := false
	for _, term := range terms {
		if term.Src == "预算" && term.Tgt == "予算" {
			found = true
		}
	}
	if !found {
		t.Fatalf("conference glossary missing 预算: %+v", terms)
	}
	if got := core.SelectGlossary(registry, "zh", "ja", nil); len(got) != 0 {
		t.Fatalf("unselected packs leaked terms: %+v", got)
	}
}

func TestRegistryWarnsAndSkipsMalformedFiles(t *testing.T) {
	files := fstest.MapFS{
		"prompts/system_zh2ja.txt": {Data: []byte("SYSTEM")},
		"prompts/system_bad.txt":   {Data: []byte("x")},
		"packs/good_zh2ja.tsv":     {Data: []byte("预算\t予算\nbad row\n")},
	}
	var warnings []string
	registry, err := core.LoadRegistry(files, func(line string) { warnings = append(warnings, line) })
	if err != nil {
		t.Fatalf("LoadRegistry: %v", err)
	}
	if len(registry.Directions) != 1 || len(registry.Packs) != 1 {
		t.Fatalf("registry = %+v", registry)
	}
	joined := strings.Join(warnings, "\n")
	if !strings.Contains(joined, "malformed prompt filename: system_bad.txt") {
		t.Errorf("missing prompt warning: %s", joined)
	}
	if !strings.Contains(joined, "good_zh2ja.tsv:2: malformed glossary row ignored") {
		t.Errorf("missing glossary warning: %s", joined)
	}
}
