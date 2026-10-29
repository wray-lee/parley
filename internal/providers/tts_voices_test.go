package providers

import (
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"testing"
)

func loadBundledVoices(t *testing.T) Voices {
	t.Helper()
	voices, err := LoadVoices(os.DirFS(filepath.Join("..", "..")))
	if err != nil {
		t.Fatalf("LoadVoices: %v", err)
	}
	return voices
}

func sortedLanguages(table map[string][]string) []string {
	languages := make([]string, 0, len(table))
	for language := range table {
		languages = append(languages, language)
	}
	sort.Strings(languages)
	return languages
}

// Mirrors tests/contract/tts-voices.test.js: config/voices.json is data, and
// every voice of a language must belong to that language.
func TestVoicesAreCoherentPerLanguage(t *testing.T) {
	voices := loadBundledVoices(t)
	languages := sortedLanguages(voices.Kokoro)
	if !reflect.DeepEqual(languages, []string{"ja", "zh"}) {
		t.Fatalf("kokoro languages = %v", languages)
	}

	for _, provider := range []struct {
		name  string
		table map[string][]string
	}{{"azure", voices.Azure}, {"edge", voices.Edge}} {
		if !reflect.DeepEqual(sortedLanguages(provider.table), languages) {
			t.Fatalf("%s languages = %v, want %v", provider.name, sortedLanguages(provider.table), languages)
		}
		for language, list := range provider.table {
			if len(list) == 0 {
				t.Fatalf("%s.%s is empty", provider.name, language)
			}
			seen := map[string]bool{}
			for _, voice := range list {
				if !strings.HasPrefix(voice, language+"-") {
					t.Fatalf("%s.%s voice %q does not belong to %s", provider.name, language, voice, language)
				}
				if seen[voice] {
					t.Fatalf("%s.%s has duplicate voice %q", provider.name, language, voice)
				}
				seen[voice] = true
			}
		}
	}

	kokoroVoice := regexp.MustCompile(`^[a-z]{2}_[a-z]+$`)
	for language, list := range voices.Kokoro {
		if len(list) < 2 {
			t.Fatalf("kokoro.%s = %v, want at least two preinstalled voices", language, list)
		}
		for _, voice := range list {
			if !kokoroVoice.MatchString(voice) {
				t.Fatalf("kokoro.%s voice %q is not a Kokoro voice id", language, voice)
			}
		}
	}
}
