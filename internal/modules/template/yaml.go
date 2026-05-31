package template

import (
	"bytes"

	"gopkg.in/yaml.v3"
)

// marshalYAML serializes a template with deterministic field order and 2-space indent.
func marshalYAML(t *Template) ([]byte, error) {
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(t); err != nil {
		_ = enc.Close()
		return nil, err
	}
	if err := enc.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// unmarshalYAML parses YAML bytes into a Template. Exposed for tests.
func unmarshalYAML(b []byte, t *Template) error {
	return yaml.Unmarshal(b, t)
}

func yamlMissingLevelScope(raw []byte) bool {
	var doc map[string]any
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		return false
	}
	fieldsAny, ok := doc["fields"].([]any)
	if !ok {
		return false
	}
	for _, fAny := range fieldsAny {
		fMap, ok := fAny.(map[string]any)
		if !ok {
			continue
		}
		if _, has := fMap["level_scope"]; !has {
			return true
		}
	}
	return false
}
