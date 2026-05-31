package recmerge

import (
	"encoding/json"
	"sort"
	"strings"
	"time"
)

// Meta keys that may not change after creation; divergence emits FieldConflict{Reason:"immutable"} and blocks the merge.
var immutableMetaKeys = []string{"created", "id", "template"}

// MergeMeta applies gigot §10.2 for the meta map. A nil base (first-ever write) degrades immutability checks
// to "take whichever side has the value". The returned map is independent of all inputs.
func MergeMeta(base, theirs, yours map[string]any) (map[string]any, []FieldConflict) {
	merged := map[string]any{}
	var conflicts []FieldConflict

	winner := UpdatedWinner(theirs, yours)

	if base != nil {
		for _, k := range immutableMetaKeys {
			bv, bok := base[k]
			tv, tok := theirs[k]
			yv, yok := yours[k]
			divergent := false
			if bok && tok && !deepEqual(bv, tv) {
				divergent = true
			}
			if bok && yok && !deepEqual(bv, yv) {
				divergent = true
			}
			if divergent {
				conflicts = append(conflicts, FieldConflict{
					Scope:  "meta",
					Key:    k,
					Reason: "immutable",
				})
			}
		}
	}

	// Populate merged even when an immutability conflict fired: the caller chooses merged vs conflicts.
	for _, k := range unionKeys(base, theirs, yours) {
		switch k {
		case "updated":
			merged[k] = mergeUpdated(theirs[k], yours[k])
		case "tags":
			merged[k] = mergeTags(theirs[k], yours[k])
		case "flagged":
			merged[k] = mergeFlagged(theirs[k], yours[k])
		case "created", "id", "template":
			// Immutable: prefer base, else whichever side has it.
			if base != nil {
				if v, ok := base[k]; ok {
					merged[k] = v
					continue
				}
			}
			if v, ok := theirs[k]; ok {
				merged[k] = v
			} else if v, ok := yours[k]; ok {
				merged[k] = v
			}
		default:
			merged[k] = pickWinner(theirs, yours, k, winner)
		}
	}

	return merged, conflicts
}

// UpdatedWinner returns "theirs" or "yours" by max(meta.updated.at); ties and unparseable values resolve to "yours" (stable).
// Accepts both the audit-block object and the legacy flat string form.
func UpdatedWinner(theirs, yours map[string]any) string {
	tt, tok := parseUpdated(theirs)
	yt, yok := parseUpdated(yours)
	switch {
	case tok && yok:
		if tt.After(yt) {
			return "theirs"
		}
		return "yours"
	case tok && !yok:
		return "theirs"
	default:
		return "yours"
	}
}

func parseUpdated(m map[string]any) (time.Time, bool) {
	if m == nil {
		return time.Time{}, false
	}
	v, ok := m["updated"]
	if !ok {
		return time.Time{}, false
	}
	return parseAuditAt(v)
}

// parseAuditAt accepts both the audit-block object ({at, name, email}) and the legacy flat string form.
func parseAuditAt(v any) (time.Time, bool) {
	if obj, ok := v.(map[string]any); ok {
		if s, ok := obj["at"].(string); ok {
			if t, err := time.Parse(time.RFC3339, s); err == nil {
				return t, true
			}
		}
		return time.Time{}, false
	}
	if s, ok := v.(string); ok {
		if t, err := time.Parse(time.RFC3339, s); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

func mergeUpdated(theirs, yours any) any {
	ts, tok := parseAuditAt(theirs)
	ys, yok := parseAuditAt(yours)
	switch {
	case tok && yok:
		if ts.After(ys) {
			return theirs
		}
		return yours
	case tok:
		return theirs
	case yok:
		return yours
	default:
		if yours != nil {
			return yours
		}
		return theirs
	}
}

func mergeTags(theirs, yours any) any {
	set := map[string]struct{}{}
	collect := func(v any) {
		arr, ok := v.([]any)
		if !ok {
			return
		}
		for _, el := range arr {
			s, ok := el.(string)
			if !ok {
				continue
			}
			n := strings.ToLower(strings.TrimSpace(s))
			if n == "" {
				continue
			}
			set[n] = struct{}{}
		}
	}
	collect(theirs)
	collect(yours)

	out := make([]any, 0, len(set))
	keys := make([]string, 0, len(set))
	for k := range set {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		out = append(out, k)
	}
	return out
}

func mergeFlagged(theirs, yours any) any {
	if asBool(theirs) || asBool(yours) {
		return true
	}
	return false
}

func asBool(v any) bool {
	b, _ := v.(bool)
	return b
}

func pickWinner(theirs, yours map[string]any, key, winner string) any {
	tv, tok := theirs[key]
	yv, yok := yours[key]
	switch {
	case tok && yok:
		if winner == "theirs" {
			return tv
		}
		return yv
	case tok:
		return tv
	case yok:
		return yv
	default:
		return nil
	}
}

func unionKeys(maps ...map[string]any) []string {
	seen := map[string]struct{}{}
	for _, m := range maps {
		for k := range m {
			seen[k] = struct{}{}
		}
	}
	out := make([]string, 0, len(seen))
	for k := range seen {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// deepEqual compares decoded JSON values, normalising json.Number to its string form so decoder differences don't matter.
func deepEqual(a, b any) bool {
	ab, errA := json.Marshal(canonicaliseForCompare(a))
	bb, errB := json.Marshal(canonicaliseForCompare(b))
	if errA != nil || errB != nil {
		return false
	}
	return string(ab) == string(bb)
}

func canonicaliseForCompare(v any) any {
	switch t := v.(type) {
	case json.Number:
		return t.String()
	case map[string]any:
		out := map[string]any{}
		for k, val := range t {
			out[k] = canonicaliseForCompare(val)
		}
		return out
	case []any:
		out := make([]any, len(t))
		for i, el := range t {
			out[i] = canonicaliseForCompare(el)
		}
		return out
	default:
		return v
	}
}
