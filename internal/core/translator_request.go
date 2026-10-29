// The fixed translator request format. The four bracket labels are protocol
// markers documented for contributors; do not translate them.

package core

import "strings"

const (
	contextLabel = "[已译上下文]"
	sourceLabel  = "原文："
	targetLabel  = "译文："
	newLabel     = "[新段落]"
)

// ContextPair is one previously committed source/translation pair.
type ContextPair struct {
	Src string `json:"src"`
	Tgt string `json:"tgt"`
}

// TranslatorPrompt is the pure request the Session sends to a translator.
type TranslatorPrompt struct {
	System string
	User   string
}

// BuildTranslatorRequest renders the prompt with glossary and Rolling Context.
func BuildTranslatorRequest(prompt string, context []ContextPair, text string, glossary []Term) TranslatorPrompt {
	terms := make([]string, 0, len(glossary))
	for _, term := range glossary {
		terms = append(terms, term.Src+" → "+term.Tgt)
	}
	lines := []string{contextLabel}
	for _, pair := range context {
		lines = append(lines, sourceLabel+pair.Src, targetLabel+pair.Tgt)
	}
	lines = append(lines, newLabel, text)
	return TranslatorPrompt{
		System: strings.ReplaceAll(prompt, "{{GLOSSARY}}", strings.Join(terms, "\n")),
		User:   strings.Join(lines, "\n"),
	}
}
