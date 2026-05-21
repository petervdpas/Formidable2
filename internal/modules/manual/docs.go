// Package manual ships the in-app user manual as a set of per-topic,
// per-locale markdown files embedded into the binary. The Information
// workspace exposes them under the "Manual" group; the frontend pipes
// the raw markdown returned here through the render module so syntax
// highlighting and the rest of the rendering pipeline match the wiki.
package manual

import (
	"embed"
	"errors"
	"fmt"
	"path"
	"regexp"
	"strings"
)

//go:embed docs/*.md
var manualFS embed.FS

const (
	manualDir      = "docs"
	manualFallback = "en"
)

// ErrUnknownTopic is returned when no markdown file exists for the
// requested topic in any locale, including the english fallback.
var ErrUnknownTopic = errors.New("manual: unknown topic")

var (
	topicPattern  = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]*$`)
	localePattern = regexp.MustCompile(`^[a-z]{2,8}(?:[-_][A-Za-z0-9]{2,8})?$`)
)

// manualDoc returns the embedded markdown for the (topic, locale)
// pair. Unknown locale, missing translation, or empty locale all fall
// back to english. Unknown topic returns ErrUnknownTopic.
// Both inputs are validated against a tight pattern so an embed path
// cannot be escaped via traversal.
func manualDoc(topic, locale string) (string, error) {
	topic = strings.ToLower(strings.TrimSpace(topic))
	if !topicPattern.MatchString(topic) {
		return "", fmt.Errorf("manual: invalid topic %q", topic)
	}
	locale = strings.ToLower(strings.TrimSpace(locale))
	if locale != "" && !localePattern.MatchString(locale) {
		return "", fmt.Errorf("manual: invalid locale %q", locale)
	}
	if locale == "" {
		locale = manualFallback
	}

	primary := path.Join(manualDir, topic+"."+locale+".md")
	if b, err := manualFS.ReadFile(primary); err == nil {
		return string(b), nil
	}
	fallback := path.Join(manualDir, topic+"."+manualFallback+".md")
	b, err := manualFS.ReadFile(fallback)
	if err != nil {
		return "", fmt.Errorf("%w: %s", ErrUnknownTopic, topic)
	}
	return string(b), nil
}
