// Builds the Direction / Language Pair / Domain Pack registry from asset file
// names. File reading is the caller's job; this module only parses.

package core

import (
	"fmt"
	"io/fs"
	"regexp"
	"sort"
	"strings"

	"github.com/wray-lee/parley/internal/protocol"
)

var (
	directionPattern = regexp.MustCompile(`^system_([A-Za-z0-9_-]+)2([A-Za-z0-9_-]+)\.txt$`)
	glossaryPattern  = regexp.MustCompile(`^glossary_([A-Za-z0-9_-]+)2([A-Za-z0-9_-]+)\.tsv$`)
	packPattern      = regexp.MustCompile(`^([A-Za-z0-9-]+)_([A-Za-z0-9-]+)2([A-Za-z0-9-]+)\.tsv$`)
)

// Term is one glossary entry.
type Term struct {
	Src string `json:"src"`
	Tgt string `json:"tgt"`
}

// Direction is one translation path and its system prompt.
type Direction struct {
	Src      string `json:"src"`
	Tgt      string `json:"tgt"`
	File     string `json:"file"`
	Prompt   string `json:"prompt"`
	Glossary []Term `json:"glossary"`
}

// Pack is one Domain Pack for one Direction.
type Pack struct {
	Name     string `json:"name"`
	Src      string `json:"src"`
	Tgt      string `json:"tgt"`
	File     string `json:"file"`
	Glossary []Term `json:"glossary"`
}

// Registry is the resolved set of Directions, Packs and Language Pairs. Its
// JSON shape is also the trace meta.registry shape.
type Registry struct {
	Directions []Direction     `json:"directions"`
	Packs      []Pack          `json:"packs"`
	Pairs      []protocol.Pair `json:"pairs"`
}

// LoadRegistry reads system prompts, glossaries and packs from fsys/prompts
// and fsys/packs. Malformed files are reported through warn and skipped.
func LoadRegistry(fsys fs.FS, warn func(string)) (*Registry, error) {
	promptEntries, err := fs.ReadDir(fsys, "prompts")
	if err != nil {
		return nil, fmt.Errorf("read prompts: %w", err)
	}
	packEntries, err := fs.ReadDir(fsys, "packs")
	if err != nil {
		return nil, fmt.Errorf("read packs: %w", err)
	}

	registry := &Registry{}
	for _, entry := range sortedNames(promptEntries) {
		if !strings.HasPrefix(entry, "system_") || !strings.HasSuffix(entry, ".txt") {
			continue
		}
		match := directionPattern.FindStringSubmatch(entry)
		if match == nil {
			warn("ignoring malformed prompt filename: " + entry)
			continue
		}
		text, err := fs.ReadFile(fsys, "prompts/"+entry)
		if err != nil {
			return nil, fmt.Errorf("read prompts/%s: %w", entry, err)
		}
		registry.Directions = append(registry.Directions, Direction{
			Src: match[1], Tgt: match[2], File: entry, Prompt: string(text),
		})
	}
	for _, entry := range sortedNames(promptEntries) {
		if !strings.HasPrefix(entry, "glossary_") || !strings.HasSuffix(entry, ".tsv") {
			continue
		}
		match := glossaryPattern.FindStringSubmatch(entry)
		if match == nil {
			warn("ignoring malformed glossary filename: " + entry)
			continue
		}
		direction := registry.FindDirection(match[1], match[2])
		if direction == nil {
			warn("glossary " + entry + " has no matching Direction")
			continue
		}
		text, err := fs.ReadFile(fsys, "prompts/"+entry)
		if err != nil {
			return nil, fmt.Errorf("read prompts/%s: %w", entry, err)
		}
		direction.Glossary = parseGlossary(string(text), entry, warn)
	}
	for _, entry := range sortedNames(packEntries) {
		if !strings.HasSuffix(entry, ".tsv") {
			continue
		}
		match := packPattern.FindStringSubmatch(entry)
		if match == nil {
			warn("ignoring malformed pack filename: " + entry)
			continue
		}
		text, err := fs.ReadFile(fsys, "packs/"+entry)
		if err != nil {
			return nil, fmt.Errorf("read packs/%s: %w", entry, err)
		}
		registry.Packs = append(registry.Packs, Pack{
			Name: match[1], Src: match[2], Tgt: match[3], File: entry,
			Glossary: parseGlossary(string(text), entry, warn),
		})
	}
	registry.Pairs = derivePairs(registry.Directions)
	return registry, nil
}

func sortedNames(entries []fs.DirEntry) []string {
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			names = append(names, entry.Name())
		}
	}
	sort.Strings(names)
	return names
}

// FindDirection returns the Direction src→tgt or nil.
func (r *Registry) FindDirection(src, tgt string) *Direction {
	for i := range r.Directions {
		if r.Directions[i].Src == src && r.Directions[i].Tgt == tgt {
			return &r.Directions[i]
		}
	}
	return nil
}

// HasPack reports whether a pack name exists.
func (r *Registry) HasPack(name string) bool {
	for _, pack := range r.Packs {
		if pack.Name == name {
			return true
		}
	}
	return false
}

// SelectGlossary merges the Direction's base Glossary with the selected Domain
// Packs; a later source term wins.
func SelectGlossary(registry *Registry, src, tgt string, packs []string) []Term {
	selected := make(map[string]bool, len(packs))
	for _, name := range packs {
		selected[name] = true
	}
	merged := make(map[string]string)
	var order []string
	add := func(terms []Term) {
		for _, term := range terms {
			if _, seen := merged[term.Src]; !seen {
				order = append(order, term.Src)
			}
			merged[term.Src] = term.Tgt
		}
	}
	if direction := registry.FindDirection(src, tgt); direction != nil {
		add(direction.Glossary)
	}
	for _, pack := range registry.Packs {
		if pack.Src != src || pack.Tgt != tgt || !selected[pack.Name] {
			continue
		}
		add(pack.Glossary)
	}
	terms := make([]Term, 0, len(order))
	for _, source := range order {
		terms = append(terms, Term{Src: source, Tgt: merged[source]})
	}
	return terms
}

func parseGlossary(text, file string, warn func(string)) []Term {
	entries := make(map[string]string)
	var order []string
	for index, raw := range strings.Split(text, "\n") {
		line := raw
		if index == 0 {
			line = strings.TrimPrefix(line, "\ufeff")
		}
		if strings.TrimSpace(line) == "" {
			continue
		}
		fields := strings.Split(line, "\t")
		for i := range fields {
			fields[i] = strings.TrimSpace(fields[i])
		}
		if len(fields) != 2 || fields[0] == "" || fields[1] == "" {
			warn(fmt.Sprintf("%s:%d: malformed glossary row ignored", file, index+1))
			continue
		}
		if _, seen := entries[fields[0]]; !seen {
			order = append(order, fields[0])
		}
		entries[fields[0]] = fields[1]
	}
	terms := make([]Term, 0, len(order))
	for _, source := range order {
		terms = append(terms, Term{Src: source, Tgt: entries[source]})
	}
	return terms
}

func derivePairs(directions []Direction) []protocol.Pair {
	var pairs []protocol.Pair
	index := make(map[string]int)
	for _, direction := range directions {
		ends := []string{direction.Src, direction.Tgt}
		sort.Strings(ends)
		key := ends[0] + "\x00" + ends[1]
		at, seen := index[key]
		if !seen {
			pairs = append(pairs, protocol.Pair{A: ends[0], B: ends[1]})
			at = len(pairs) - 1
			index[key] = at
		}
		pairs[at].Directions = append(pairs[at].Directions, protocol.Direction{Src: direction.Src, Tgt: direction.Tgt})
	}
	for i := range pairs {
		pairs[i].Converse = len(pairs[i].Directions) > 1
	}
	return pairs
}
