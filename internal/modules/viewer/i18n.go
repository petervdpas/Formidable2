package viewer

import (
	"embed"
	"encoding/json"
	"os"
	"strings"
)

//go:embed locales/en.json locales/nl.json
var localesFS embed.FS

// SupportedLanguages are the viewer UI languages, in display order. English is
// the fallback and must stay first.
var SupportedLanguages = []string{"en", "nl"}

// ResolveLanguage turns a config language ("system" | "en" | "nl") into a
// concrete supported language. "system" (or anything unknown) is resolved from
// the OS locale, falling back to English.
func ResolveLanguage(configLang string) string {
	switch configLang {
	case "en", "nl":
		return configLang
	}
	return detectOSLanguage()
}

// detectOSLanguage reads the usual locale env vars and matches a supported
// language by prefix, defaulting to English.
func detectOSLanguage() string {
	for _, key := range []string{"LC_ALL", "LC_MESSAGES", "LANG"} {
		v := strings.ToLower(os.Getenv(key))
		if v == "" {
			continue
		}
		for _, lang := range SupportedLanguages {
			if strings.HasPrefix(v, lang) {
				return lang
			}
		}
	}
	return "en"
}

// Messages returns the flattened UI strings for a concrete language, falling
// back to English for an unknown one. Keys are dotted (e.g. "settings.title").
func Messages(lang string) map[string]string {
	switch lang {
	case "en", "nl":
	default:
		lang = "en"
	}
	nested := loadLocale(lang)
	if nested == nil {
		nested = loadLocale("en")
	}
	flat := map[string]string{}
	flatten("", nested, flat)
	return flat
}

func loadLocale(lang string) map[string]any {
	data, err := localesFS.ReadFile("locales/" + lang + ".json")
	if err != nil {
		return nil
	}
	var out map[string]any
	if err := json.Unmarshal(data, &out); err != nil {
		return nil
	}
	return out
}

func flatten(prefix string, in map[string]any, out map[string]string) {
	for k, v := range in {
		key := k
		if prefix != "" {
			key = prefix + "." + k
		}
		switch t := v.(type) {
		case map[string]any:
			flatten(key, t, out)
		case string:
			out[key] = t
		}
	}
}
