package template

import (
	"fmt"
	"strings"
)

const maxLoopDepth = 2

// Validate runs all template-level checks and returns the accumulated
// errors. Mirrors the rules in `controls/templateManager.js`.
func Validate(t *Template) []ValidationError {
	if t == nil || t.Fields == nil {
		return []ValidationError{{
			Type:    "invalid-template",
			Message: "Missing or invalid fields array",
		}}
	}

	var errs []ValidationError

	if dups := duplicateKeys(t.Fields); len(dups) > 0 {
		errs = append(errs, ValidationError{
			Type:    "duplicate-keys",
			Keys:    dups,
			Message: fmt.Sprintf("Duplicate field keys: %s", strings.Join(dups, ", ")),
		})
	}

	if e := primaryKeyError(t.Fields); e != nil {
		errs = append(errs, *e)
	}

	errs = append(errs, loopPairingErrors(t.Fields)...)
	errs = append(errs, loopNestingErrors(t.Fields, maxLoopDepth)...)

	if e := collectionGuidError(t); e != nil {
		errs = append(errs, *e)
	}
	if e := singleTagsError(t.Fields); e != nil {
		errs = append(errs, *e)
	}
	errs = append(errs, apiFieldErrors(t.Fields)...)

	return errs
}

// duplicateKeys returns keys that appear more than once, ignoring matched
// loopstart/loopstop pairs (those legally share a key).
func duplicateKeys(fields []Field) []string {
	seen := map[string]string{}
	var dups []string
	for _, f := range fields {
		if f.Key == "" {
			continue
		}
		if existing, ok := seen[f.Key]; ok {
			isLoopPair :=
				(f.Type == "loopstart" && existing == "loopstop") ||
					(f.Type == "loopstop" && existing == "loopstart")
			if !isLoopPair {
				dups = append(dups, f.Key)
			}
		} else {
			seen[f.Key] = f.Type
		}
	}
	return dups
}

func primaryKeyError(fields []Field) *ValidationError {
	var pkKeys []string
	for _, f := range fields {
		if f.PrimaryKey {
			pkKeys = append(pkKeys, f.Key)
		}
	}
	if len(pkKeys) > 1 {
		return &ValidationError{
			Type:    "multiple-primary-keys",
			Keys:    pkKeys,
			Message: fmt.Sprintf("Multiple primary keys found: %s", strings.Join(pkKeys, ", ")),
		}
	}
	return nil
}

// loopPairingErrors reports unmatched loopstart/loopstop and key mismatches.
func loopPairingErrors(fields []Field) []ValidationError {
	var errs []ValidationError
	type frame struct {
		field Field
		index int
	}
	var stack []frame

	for i := range fields {
		f := fields[i]
		switch f.Type {
		case "loopstart":
			stack = append(stack, frame{field: f, index: i})
		case "loopstop":
			if len(stack) == 0 {
				ff := f
				errs = append(errs, ValidationError{
					Type:    "unmatched-loopstop",
					Field:   &ff,
					Index:   i,
					Message: "Unmatched loopstop without preceding loopstart",
				})
				continue
			}
			top := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			if f.Key != top.field.key() {
				ff := f
				errs = append(errs, ValidationError{
					Type:  "loop-key-mismatch",
					Field: &ff,
					Index: i,
					Detail: map[string]any{
						"expectedKey": top.field.Key,
						"actualKey":   f.Key,
					},
					Message: fmt.Sprintf(
						"Loopstop key %q does not match loopstart key %q",
						f.Key, top.field.Key,
					),
				})
			}
		}
	}

	for _, top := range stack {
		ff := top.field
		errs = append(errs, ValidationError{
			Type:    "unmatched-loopstart",
			Field:   &ff,
			Index:   top.index,
			Message: "Unmatched loopstart without corresponding loopstop",
		})
	}

	return errs
}

func loopNestingErrors(fields []Field, maxDepth int) []ValidationError {
	var errs []ValidationError
	var stack []string
	for _, f := range fields {
		switch f.Type {
		case "loopstart":
			stack = append(stack, f.Key)
			if len(stack) > maxDepth {
				errs = append(errs, ValidationError{
					Type: "excessive-loop-nesting",
					Key:  f.Key,
					Detail: map[string]any{
						"depth":    len(stack),
						"maxDepth": maxDepth,
						"path":     strings.Join(stack, " → "),
					},
					Message: fmt.Sprintf(
						"Loop nesting exceeds %d at key %q (depth %d)",
						maxDepth, f.Key, len(stack),
					),
				})
			}
		case "loopstop":
			if len(stack) > 0 && stack[len(stack)-1] == f.Key {
				stack = stack[:len(stack)-1]
			}
		}
	}
	return errs
}

func collectionGuidError(t *Template) *ValidationError {
	if !t.EnableCollection {
		return nil
	}
	for _, f := range t.Fields {
		if f.Type == "guid" {
			return nil
		}
	}
	return &ValidationError{
		Type:    "missing-guid-for-collection",
		Message: "`Enable Collection` is active but no GUID field found. Add a GUID field or disable this option.",
	}
}

func singleTagsError(fields []Field) *ValidationError {
	var keys []string
	for _, f := range fields {
		if f.Type == "tags" {
			k := f.Key
			if k == "" {
				k = "(no key)"
			}
			keys = append(keys, k)
		}
	}
	if len(keys) > 1 {
		return &ValidationError{
			Type:    "multiple-tags-fields",
			Keys:    keys,
			Message: fmt.Sprintf("Only one 'tags' field is allowed per template (found: %s)", strings.Join(keys, ", ")),
		}
	}
	return nil
}

func apiFieldErrors(fields []Field) []ValidationError {
	var errs []ValidationError
	for _, f := range fields {
		if f.Type != "api" {
			continue
		}
		ff := f
		key := f.Key
		if key == "" {
			key = "(no key)"
		}
		if strings.TrimSpace(f.Collection) == "" {
			errs = append(errs, ValidationError{
				Type:    "api-collection-required",
				Field:   &ff,
				Key:     key,
				Message: "API field requires a non-empty collection name.",
			})
			continue
		}
		if f.Map != nil {
			seen := map[string]bool{}
			for _, m := range f.Map {
				k := strings.TrimSpace(m.Key)
				if k == "" {
					errs = append(errs, ValidationError{
						Type:    "api-map-key-required",
						Field:   &ff,
						Key:     key,
						Message: "Each API map entry must have a non-empty 'key'.",
					})
					break
				}
				kl := strings.ToLower(k)
				if seen[kl] {
					errs = append(errs, ValidationError{
						Type:    "api-map-duplicate-keys",
						Field:   &ff,
						Key:     key,
						Detail:  map[string]any{"dup": k},
						Message: fmt.Sprintf("Duplicate API map key: %s", k),
					})
					break
				}
				seen[kl] = true
			}
		}
	}
	return errs
}

// key is a tiny accessor used by loopPairingErrors so the same struct
// can be ranged whether passed by value or pointer.
func (f Field) key() string { return f.Key }
