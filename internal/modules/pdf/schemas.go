package pdf

import (
	"reflect"
	"strings"
)

// Schemas returns the structural map every codeformatter-style repair
// pass needs: top-level frontmatter key → the set of child keys
// recognised under it. Derived by reflecting on the typed Frontmatter
// struct so the keyset can't drift from input.go - adding a field to
// CoverFM (or any other sub-block) auto-registers it here.
//
// Returned map is fresh on each call (cheap; called once at app boot).
// Only fields whose type is a struct or pointer-to-struct become
// parents; scalars and slices at the Frontmatter level (e.g. Style,
// Keywords) are absent because they are not nesting parents.
func Schemas() map[string]map[string]bool {
	out := map[string]map[string]bool{}
	fmType := reflect.TypeFor[Frontmatter]()
	for i := 0; i < fmType.NumField(); i++ {
		f := fmType.Field(i)
		parentKey := yamlKeyOf(f.Tag.Get("yaml"))
		if parentKey == "" {
			continue
		}
		childType := f.Type
		if childType.Kind() == reflect.Pointer {
			childType = childType.Elem()
		}
		if childType.Kind() != reflect.Struct {
			continue
		}
		children := map[string]bool{}
		for j := 0; j < childType.NumField(); j++ {
			childKey := yamlKeyOf(childType.Field(j).Tag.Get("yaml"))
			if childKey != "" {
				children[childKey] = true
			}
		}
		if len(children) > 0 {
			out[parentKey] = children
		}
	}
	return out
}

// yamlKeyOf parses a `yaml:"..."` tag value and returns the key name
// without any modifiers (`,omitempty`, `,inline`, etc.). Empty string
// means "no yaml tag" or "explicitly ignored" (`yaml:"-"`).
func yamlKeyOf(tag string) string {
	if tag == "" || tag == "-" {
		return ""
	}
	if comma := strings.Index(tag, ","); comma >= 0 {
		tag = tag[:comma]
	}
	return tag
}
