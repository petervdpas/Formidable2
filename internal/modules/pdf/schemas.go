package pdf

import (
	"reflect"
	"strings"
)

// Schemas maps each top-level frontmatter key to its recognised child
// keys, reflected off the Frontmatter struct so the keyset can't drift
// from input.go. Only struct / pointer-to-struct fields become parents;
// scalars and slices (Style, Keywords) are absent.
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

// yamlKeyOf returns the key name from a yaml tag without modifiers;
// empty for no tag or `yaml:"-"`.
func yamlKeyOf(tag string) string {
	if tag == "" || tag == "-" {
		return ""
	}
	if comma := strings.Index(tag, ","); comma >= 0 {
		tag = tag[:comma]
	}
	return tag
}
