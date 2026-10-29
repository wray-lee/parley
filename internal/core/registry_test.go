package core_test

import (
	"io/fs"
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

func loadMapFS(t *testing.T, files fstest.MapFS) (*core.Registry, []string) {
	t.Helper()
	if _, ok := files["packs"]; !ok {
		files["packs"] = &fstest.MapFile{Mode: fs.ModeDir}
	}
	var warnings []string
	registry, err := core.LoadRegistry(files, func(line string) { warnings = append(warnings, line) })
	if err != nil {
		t.Fatalf("LoadRegistry: %v", err)
	}
	return registry, warnings
}

func systemFor(registry *core.Registry, src, tgt string, packs []string) string {
	direction := registry.FindDirection(src, tgt)
	glossary := core.SelectGlossary(registry, src, tgt, packs)
	return core.BuildTranslatorRequest(direction.Prompt, nil, "x", glossary).System
}

func TestPromptFilenamesRegisterDirectionsAndDerivePairs(t *testing.T) {
	registry, warnings := loadMapFS(t, fstest.MapFS{
		"prompts/system_zh2ja.txt": {Data: []byte("zh2ja prompt")},
		"prompts/system_ja2zh.txt": {Data: []byte("ja2zh prompt")},
		"prompts/system_zh2fr.txt": {Data: []byte("zh2fr prompt")},
	})
	if len(warnings) != 0 {
		t.Fatalf("warnings = %v", warnings)
	}
	if len(registry.Directions) != 3 {
		t.Fatalf("directions = %+v", registry.Directions)
	}
	if got := registry.FindDirection("zh", "ja"); got == nil || got.Prompt != "zh2ja prompt" {
		t.Fatalf("zh->ja direction = %+v", got)
	}
	for _, pair := range registry.Pairs {
		has := func(src, tgt string) bool {
			for _, direction := range pair.Directions {
				if direction.Src == src && direction.Tgt == tgt {
					return true
				}
			}
			return false
		}
		switch {
		case has("zh", "ja") && has("ja", "zh"):
			if !pair.Converse || len(pair.Directions) != 2 {
				t.Fatalf("zh↔ja pair = %+v", pair)
			}
		case has("zh", "fr"):
			if pair.Converse || len(pair.Directions) != 1 {
				t.Fatalf("a one-way Direction only supports Listen: %+v", pair)
			}
		}
	}
}

func TestMalformedPromptFilenameIsSkippedWithAWarning(t *testing.T) {
	registry, warnings := loadMapFS(t, fstest.MapFS{
		"prompts/system_zh2ja.txt":  {Data: []byte("ok")},
		"prompts/system_broken.txt": {Data: []byte("bad")},
		"prompts/system_.txt":       {Data: []byte("bad")},
	})
	if len(registry.Directions) != 1 {
		t.Fatalf("directions = %+v", registry.Directions)
	}
	joined := strings.Join(warnings, " ")
	if len(warnings) != 2 || !strings.Contains(joined, "system_broken.txt") ||
		!strings.Contains(joined, "system_.txt") {
		t.Fatalf("warnings = %v", warnings)
	}
}

func TestPackFilenamesRegisterPacksPerDirection(t *testing.T) {
	registry, warnings := loadMapFS(t, fstest.MapFS{
		"prompts/system_zh2ja.txt":   {Data: []byte("ok")},
		"packs/conference_zh2ja.tsv": {Data: []byte("")},
		"packs/conference_ja2zh.tsv": {Data: []byte("")},
		"packs/broken.tsv":           {Data: []byte("")},
	})
	if len(registry.Packs) != 2 {
		t.Fatalf("packs = %+v", registry.Packs)
	}
	found := false
	for _, pack := range registry.Packs {
		if pack.Name == "conference" && pack.Src == "zh" && pack.Tgt == "ja" && pack.File == "conference_zh2ja.tsv" {
			found = true
		}
	}
	if !found {
		t.Fatalf("pack = %+v", registry.Packs)
	}
	if len(warnings) != 1 || !strings.Contains(warnings[0], "broken.tsv") {
		t.Fatalf("warnings = %v", warnings)
	}
}

func TestBaseGlossaryIsInjectedIntoTheTranslatorRequest(t *testing.T) {
	registry, warnings := loadMapFS(t, fstest.MapFS{
		"prompts/system_zh2ja.txt":   {Data: []byte("术语表：\n{{GLOSSARY}}")},
		"prompts/glossary_zh2ja.tsv": {Data: []byte("预算\t予算\n议事录\t議事録\n")},
	})
	if len(warnings) != 0 {
		t.Fatalf("warnings = %v", warnings)
	}
	want := "术语表：\n预算 → 予算\n议事录 → 議事録"
	if got := systemFor(registry, "zh", "ja", nil); got != want {
		t.Fatalf("system = %q", got)
	}
}

func TestConferenceAppliesToBothDirectionsOnlyWhenSelected(t *testing.T) {
	registry := loadAssets(t)
	for _, direction := range []core.Direction{
		{Src: "zh", Tgt: "ja"},
		{Src: "ja", Tgt: "zh"},
	} {
		selected := core.SelectGlossary(registry, direction.Src, direction.Tgt, []string{"conference"})
		if len(selected) == 0 {
			t.Fatalf("conference terms missing for %s->%s", direction.Src, direction.Tgt)
		}
		if got := core.SelectGlossary(registry, direction.Src, direction.Tgt, nil); len(got) != 0 {
			t.Fatalf("unselected packs leaked terms for %s->%s: %+v", direction.Src, direction.Tgt, got)
		}
	}
}

func TestAPackEntryOverridesABaseGlossaryEntry(t *testing.T) {
	registry, warnings := loadMapFS(t, fstest.MapFS{
		"prompts/system_zh2ja.txt":   {Data: []byte("{{GLOSSARY}}")},
		"prompts/glossary_zh2ja.tsv": {Data: []byte("预算\t予算\n")},
		"packs/conference_zh2ja.tsv": {Data: []byte("预算\t予算会議\n")},
	})
	if len(warnings) != 0 {
		t.Fatalf("warnings = %v", warnings)
	}
	if got := systemFor(registry, "zh", "ja", nil); got != "预算 → 予算" {
		t.Fatalf("base glossary = %q", got)
	}
	if got := systemFor(registry, "zh", "ja", []string{"conference"}); got != "预算 → 予算会議" {
		t.Fatalf("pack override = %q", got)
	}
}

func TestMalformedGlossaryRowsAreSkippedWithFileAndLine(t *testing.T) {
	_, warnings := loadMapFS(t, fstest.MapFS{
		"prompts/system_zh2ja.txt":   {Data: []byte("ok")},
		"prompts/glossary_zh2ja.tsv": {Data: []byte("预算\t予算\nbad row\n")},
	})
	if len(warnings) != 1 || !strings.Contains(warnings[0], "glossary_zh2ja.tsv:2: malformed glossary row ignored") {
		t.Fatalf("warnings = %v", warnings)
	}
}
