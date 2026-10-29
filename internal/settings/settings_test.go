package settings_test

import (
	"encoding/json"
	"errors"
	"os"
	"testing"

	"github.com/wray-lee/parley/internal/settings"
)

// The fixture is the one case table the Dart client reads too.
type fixture struct {
	BaseURL []struct {
		Value  string  `json:"value"`
		Preset string  `json:"preset"`
		Expect *string `json:"expect"`
		Error  string  `json:"error"`
	} `json:"baseUrl"`
	Model []struct {
		Value  string  `json:"value"`
		Preset string  `json:"preset"`
		Expect *string `json:"expect"`
		Error  string  `json:"error"`
	} `json:"model"`
	ExtraBody []struct {
		Value  string  `json:"value"`
		Expect *string `json:"expect"`
		Error  string  `json:"error"`
	} `json:"extraBody"`
	LocalNetwork struct {
		Base  map[string]string `json:"base"`
		Cases []struct {
			Set    map[string]string `json:"set"`
			Expect bool              `json:"expect"`
		} `json:"cases"`
	} `json:"localNetwork"`
}

func loadFixture(t *testing.T) fixture {
	t.Helper()
	data, err := os.ReadFile("../../spec/fixtures/settings-validation.json")
	if err != nil {
		t.Fatal(err)
	}
	var f fixture
	if err := json.Unmarshal(data, &f); err != nil {
		t.Fatal(err)
	}
	return f
}

func errorCode(err error) string {
	var code settings.Error
	if errors.As(err, &code) {
		return string(code)
	}
	if err != nil {
		return err.Error()
	}
	return ""
}

func TestBaseURLCases(t *testing.T) {
	f := loadFixture(t)
	for _, c := range f.BaseURL {
		got, err := settings.NormalizeBaseURL(c.Value, c.Preset)
		if c.Error != "" {
			if errorCode(err) != c.Error {
				t.Errorf("%q/%s: error = %q, want %q", c.Value, c.Preset, errorCode(err), c.Error)
			}
			continue
		}
		if err != nil {
			t.Errorf("%q/%s: unexpected error %v", c.Value, c.Preset, err)
			continue
		}
		if c.Expect != nil && got != *c.Expect {
			t.Errorf("%q/%s: got %q, want %q", c.Value, c.Preset, got, *c.Expect)
		}
	}
}

func TestModelCases(t *testing.T) {
	f := loadFixture(t)
	for _, c := range f.Model {
		got, err := settings.NormalizeModel(c.Value, c.Preset)
		if c.Error != "" {
			if errorCode(err) != c.Error {
				t.Errorf("%q/%s: error = %q, want %q", c.Value, c.Preset, errorCode(err), c.Error)
			}
			continue
		}
		if err != nil || c.Expect == nil || got != *c.Expect {
			t.Errorf("%q/%s: got %q, %v, want %q", c.Value, c.Preset, got, err, *c.Expect)
		}
	}
}

func TestExtraBodyCases(t *testing.T) {
	f := loadFixture(t)
	for _, c := range f.ExtraBody {
		got, err := settings.NormalizeExtraBody(c.Value)
		if c.Error != "" {
			if errorCode(err) != c.Error {
				t.Errorf("%q: error = %q, want %q", c.Value, errorCode(err), c.Error)
			}
			continue
		}
		if err != nil || c.Expect == nil || got != *c.Expect {
			t.Errorf("%q: got %q, %v, want %q", c.Value, got, err, *c.Expect)
		}
	}
}

func TestLocalNetworkCases(t *testing.T) {
	f := loadFixture(t)
	for _, c := range f.LocalNetwork.Cases {
		values := map[string]string{}
		for key, value := range f.LocalNetwork.Base {
			values[key] = value
		}
		for key, value := range c.Set {
			values[key] = value
		}
		if got := settings.NeedsLocalNetworkPermission(values); got != c.Expect {
			t.Errorf("%v: got %v, want %v", c.Set, got, c.Expect)
		}
	}
}
