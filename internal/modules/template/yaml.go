package template

import (
	"bytes"

	"gopkg.in/yaml.v3"
)

// marshalYAML serializes a template with deterministic field order
// (same key sequence as the JS `saveTemplate`) and 2-space indent.
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
