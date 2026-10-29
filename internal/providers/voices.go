// config/voices.json is data: the per-language voice lists of the audio TTS
// adapters. Keeping it out of code is what lets a new language ship without a
// code change.

package providers

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"regexp"
	"strings"
)

// Voices is the parsed config/voices.json.
type Voices struct {
	Kokoro map[string][]string `json:"kokoro"`
	Azure  map[string][]string `json:"azure"`
	Edge   map[string][]string `json:"edge"`
}

// LoadVoices reads config/voices.json from fsys.
func LoadVoices(fsys fs.FS) (Voices, error) {
	data, err := fs.ReadFile(fsys, "config/voices.json")
	if err != nil {
		return Voices{}, fmt.Errorf("read config/voices.json: %w", err)
	}
	var voices Voices
	if err := json.Unmarshal(data, &voices); err != nil {
		return Voices{}, fmt.Errorf("parse config/voices.json: %w", err)
	}
	return voices, nil
}

// voiceList picks the voices of language, falling back to its base language
// exactly like the JavaScript adapters.
func voiceList(provider string, voices map[string][]string, language, fallback string, warn func(string)) ([]string, error) {
	if language == "" {
		return nil, fmt.Errorf("%s TTS needs a target language (direction.tgt).%s", provider, fallbackSuffix(fallback))
	}
	if list := voices[language]; len(list) > 0 {
		return list, nil
	}
	base := strings.SplitN(language, "-", 2)[0]
	if list := voices[base]; len(list) > 0 {
		warn(fmt.Sprintf(`%s TTS has no voice configured for %q; using %q.`, provider, language, base))
		return list, nil
	}
	return nil, fmt.Errorf(`%s TTS has no voice configured for target language %q.%s`, provider, language, fallbackSuffix(fallback))
}

func fallbackSuffix(fallback string) string {
	if fallback == "" {
		return ""
	}
	return " " + fallback
}

var voiceLanguagePattern = regexp.MustCompile(`^([a-z]{2,3}-[A-Z]{2})-`)

// voiceLanguage is the SSML language tag of a voice name, e.g. ja-JP.
func voiceLanguage(voice string) string {
	if match := voiceLanguagePattern.FindStringSubmatch(voice); match != nil {
		return match[1]
	}
	return ""
}
