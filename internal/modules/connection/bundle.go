package connection

import (
	"errors"
	"fmt"
	"reflect"
	"strings"

	"gopkg.in/yaml.v3"
)

// A shipped definition may be no more than the settings a document cannot
// carry for a given deployment. These helpers read and write that file without
// the shipper knowing the definition's shape.

// ErrInvalidBaseURL is returned for a base URL that is not absolute http(s).
var ErrInvalidBaseURL = errors.New("connection: base URL must be an absolute http or https URL")

// BundleSettings is what a shipped definition says beyond the document: the
// display name and the base URL. Custom is true when the definition also
// carries bindings or names its own document, so it is a hand-written
// definition rather than settings alone.
type BundleSettings struct {
	Name    string
	BaseURL string
	Custom  bool
}

// settingsOnly is the on-disk form of a definition that holds nothing but
// settings, so the file stays as small as what it says.
type settingsOnly struct {
	ID      string `yaml:"id"`
	Name    string `yaml:"name,omitempty"`
	BaseURL string `yaml:"base_url,omitempty"`
}

func decodeBundleDefinition(def []byte, id string) (Connection, error) {
	c := Connection{}
	if len(strings.TrimSpace(string(def))) > 0 {
		if err := yaml.Unmarshal(def, &c); err != nil {
			return Connection{}, fmt.Errorf("connection: %q is not a valid definition: %w", id, err)
		}
	}
	return c, nil
}

// ReadBundleSettings reads a shipped definition; empty input is no settings.
func ReadBundleSettings(def []byte) (BundleSettings, error) {
	c, err := decodeBundleDefinition(def, "")
	if err != nil {
		return BundleSettings{}, err
	}
	rest := c
	rest.ID, rest.Name, rest.BaseURL = "", "", ""
	return BundleSettings{Name: c.Name, BaseURL: c.BaseURL, Custom: !reflect.DeepEqual(rest, Connection{})}, nil
}

// SetBundleSettings returns the shipped definition for id with its name and
// base URL set from s, keeping everything else def already says. An empty
// value clears that setting, and the document supplies it again. The result is
// nil when nothing is left worth a file, which tells the caller to ship the
// document alone. s.Custom is ignored.
func SetBundleSettings(def []byte, id string, s BundleSettings) ([]byte, error) {
	name, baseURL := strings.TrimSpace(s.Name), strings.TrimSpace(s.BaseURL)
	if baseURL != "" && !ValidBaseURL(baseURL) {
		return nil, fmt.Errorf("%w: %q", ErrInvalidBaseURL, baseURL)
	}
	c, err := decodeBundleDefinition(def, id)
	if err != nil {
		return nil, err
	}
	if c.ID != "" && c.ID != id {
		return nil, fmt.Errorf("connection: shipped definition %q declares id %q", id, c.ID)
	}
	c.ID, c.Name, c.BaseURL = id, name, baseURL

	switch {
	case reflect.DeepEqual(c, Connection{ID: id}):
		return nil, nil
	case reflect.DeepEqual(c, Connection{ID: id, Name: name, BaseURL: baseURL}):
		return yaml.Marshal(settingsOnly{ID: id, Name: name, BaseURL: baseURL})
	}
	return yaml.Marshal(&c)
}
