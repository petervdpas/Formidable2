package template

import "strings"

// A `reference` table column holds plain codes ("Q-01, C-01"). Its option map
// may name a target: a field inside a top-level loop. Codes are matched against
// that field's values at render time (nothing is stored but the code), so a
// matching code becomes an in-page link to the loop item.
const ReferenceTargetKey = "target"

// SplitReferenceCodes splits a reference cell on commas/semicolons, deduped
// case-insensitively with the first spelling kept.
func SplitReferenceCodes(s string) []string {
	var out []string
	seen := map[string]bool{}
	for _, part := range strings.FieldsFunc(s, func(r rune) bool { return r == ',' || r == ';' }) {
		code := strings.TrimSpace(part)
		k := strings.ToLower(code)
		if code == "" || seen[k] {
			continue
		}
		seen[k] = true
		out = append(out, code)
	}
	return out
}

// ReferenceColumns maps a table field's column index to its reference target,
// for reference columns that name one.
func ReferenceColumns(f Field) map[int]string {
	out := map[int]string{}
	if f.Type != "table" {
		return out
	}
	for i, o := range f.Options {
		m, ok := o.(map[string]any)
		if !ok || m["type"] != "reference" {
			continue
		}
		if t, _ := m[ReferenceTargetKey].(string); strings.TrimSpace(t) != "" {
			out[i] = strings.TrimSpace(t)
		}
	}
	return out
}

// referenceLoopOf returns the top-level loop that directly holds field key, or
// "" when key is not a data field inside one.
func referenceLoopOf(fields []Field, key string) string {
	loop, depth := "", 0
	for _, f := range fields {
		switch f.Type {
		case "loopstart":
			depth++
			if depth == 1 {
				loop = f.Key
			}
			continue
		case "loopstop":
			depth--
			continue
		}
		if f.Key == key && depth == 1 && !IsVirtualFieldType(f.Type) {
			return loop
		}
	}
	return ""
}

// ReferenceTargetCandidates lists the fields a reference column may target: data
// fields directly inside a top-level loop, labelled "Loop: Field".
func ReferenceTargetCandidates(fields []Field) []ItemField {
	out := []ItemField{}
	loopLabel := map[string]string{}
	for _, f := range fields {
		if f.Type == "loopstart" {
			if _, seen := loopLabel[f.Key]; !seen {
				loopLabel[f.Key] = labelOrKey(f)
			}
		}
	}
	for _, f := range fields {
		if f.Type == "loopstart" || f.Type == "loopstop" || f.Key == "" {
			continue
		}
		if loop := referenceLoopOf(fields, f.Key); loop != "" {
			out = append(out, ItemField{Key: f.Key, Label: loopLabel[loop] + ": " + labelOrKey(f)})
		}
	}
	return out
}

func labelOrKey(f Field) string {
	if f.Label != "" {
		return f.Label
	}
	return f.Key
}

// ReferenceTargets maps every valid reference target used by fields' tables to
// the loop that holds it.
func ReferenceTargets(fields []Field) map[string]string {
	out := map[string]string{}
	for _, f := range fields {
		for _, target := range ReferenceColumns(f) {
			if loop := referenceLoopOf(fields, target); loop != "" {
				out[target] = loop
			}
		}
	}
	return out
}

// ReferenceAnchor is the in-page id for code under target ("ref-tr_code-q-01");
// "" when the code has nothing slug-worthy.
func ReferenceAnchor(target, code string) string {
	c := refSlug(code)
	if c == "" {
		return ""
	}
	return "ref-" + refSlug(target) + "-" + c
}

func refSlug(s string) string {
	var b strings.Builder
	dash := false
	for _, r := range strings.ToLower(strings.TrimSpace(s)) {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '_' {
			b.WriteRune(r)
			dash = false
		} else if !dash && b.Len() > 0 {
			b.WriteByte('-')
			dash = true
		}
	}
	return strings.TrimRight(b.String(), "-")
}

// referenceColumnErrors flags reference targets that name no field, or a field
// that is not a data field directly inside a top-level loop.
func referenceColumnErrors(fields []Field) []ValidationError {
	keys := map[string]bool{}
	for _, f := range fields {
		keys[f.Key] = true
	}
	var errs []ValidationError
	for i, f := range fields {
		for _, target := range ReferenceColumns(f) {
			ff := f
			detail := map[string]any{"target": target}
			switch {
			case !keys[target]:
				errs = append(errs, ValidationError{Type: "table-reference-unknown-target", Field: &ff, Index: i, Key: f.Key, Detail: detail,
					Message: "Reference column target is not a field: " + target})
			case referenceLoopOf(fields, target) == "":
				errs = append(errs, ValidationError{Type: "table-reference-target-not-in-loop", Field: &ff, Index: i, Key: f.Key, Detail: detail,
					Message: "Reference column target must be a field inside a top-level loop: " + target})
			}
		}
	}
	return errs
}
