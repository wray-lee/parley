// Presets are the OpenAI-compatible endpoints and recommended models.
// config/presets.json is the single data source: JavaScript imports it and Go
// reads it through the embedded assets, so adding a Preset is not a code change.

package providers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/fs"
)

// Preset is one OpenAI-compatible endpoint.
type Preset struct {
	ID           string `json:"id"`
	BaseURL      string `json:"baseUrl"`
	Model        string `json:"model"`
	RequiresKey  bool   `json:"requiresKey"`
	TrainsOnData bool   `json:"trainsOnData"`
}

// Presets is the parsed config/presets.json. Order is the file order, so error
// messages list the Presets exactly like the JavaScript adapter.
type Presets struct {
	Default string
	ByID    map[string]Preset
	Order   []string
}

// LoadPresets reads config/presets.json from fsys.
func LoadPresets(fsys fs.FS) (*Presets, error) {
	data, err := fs.ReadFile(fsys, "config/presets.json")
	if err != nil {
		return nil, fmt.Errorf("read config/presets.json: %w", err)
	}
	var presets Presets
	if err := json.Unmarshal(data, &presets); err != nil {
		return nil, fmt.Errorf("parse config/presets.json: %w", err)
	}
	return &presets, nil
}

// UnmarshalJSON walks the object so PRESETS keeps its file order; a plain map
// loses it and the unknown-preset error would not match JavaScript.
func (p *Presets) UnmarshalJSON(data []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	if token != json.Delim('{') {
		return fmt.Errorf("presets must be a JSON object")
	}
	p.ByID = map[string]Preset{}
	for decoder.More() {
		token, err := decoder.Token()
		if err != nil {
			return err
		}
		switch token {
		case "DEFAULT_PRESET":
			if err := decoder.Decode(&p.Default); err != nil {
				return err
			}
		case "PRESETS":
			token, err := decoder.Token()
			if err != nil {
				return err
			}
			if token != json.Delim('{') {
				return fmt.Errorf("PRESETS must be a JSON object")
			}
			for decoder.More() {
				id, err := decoder.Token()
				if err != nil {
					return err
				}
				var preset Preset
				if err := decoder.Decode(&preset); err != nil {
					return err
				}
				name, _ := id.(string)
				p.ByID[name] = preset
				p.Order = append(p.Order, name)
			}
			if _, err := decoder.Token(); err != nil { // closing '}'
				return err
			}
		default:
			var skip json.RawMessage
			if err := decoder.Decode(&skip); err != nil {
				return err
			}
		}
	}
	return nil
}
