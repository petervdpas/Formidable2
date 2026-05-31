package recmerge

// MergeResult is the outcome of a record-level merge; exactly one of Merged or Conflict is non-nil.
// Conflict is reserved for immutable-meta violations, which short-circuit the data merge.
type MergeResult struct {
	Merged   []byte
	Conflict *RecordConflict
}

// Merge applies gigot §10.2 + §10.3: neither-changed keeps base, one-side-changed takes that side,
// both-changed-differently resolves by last-writer-wins on meta.updated. Missing keys compare as nil.
func Merge(path string, base, theirs, yours Record) (MergeResult, error) {
	mergedMeta, conflicts := MergeMeta(base.Meta, theirs.Meta, yours.Meta)
	if len(conflicts) > 0 {
		return MergeResult{
			Conflict: &RecordConflict{
				Path:           path,
				FieldConflicts: conflicts,
			},
		}, nil
	}

	winner := UpdatedWinner(theirs.Meta, yours.Meta)
	mergedData := map[string]any{}

	for _, key := range unionKeys(base.Data, theirs.Data, yours.Data) {
		bv, bok := base.Data[key]
		tv, tok := theirs.Data[key]
		yv, yok := yours.Data[key]

		switch {
		case tok && yok && deepEqual(tv, yv):
			// Both sides hold the same value (including both-same-changed).
			mergedData[key] = tv
		case tok && !yok:
			// Theirs has it, yours doesn't. If yours matches base
			// (either both absent, or base absent too - a yours-side
			// no-op), theirs is the changed side and its value stands.
			// If yours is a deliberate removal (base had it, yours
			// removed), fall to LWW.
			if !bok {
				mergedData[key] = tv
			} else if deepEqual(bv, tv) {
				// theirs unchanged, yours removed → removal wins, drop.
			} else {
				// theirs changed, yours removed → LWW.
				if winner == "theirs" {
					mergedData[key] = tv
				}
			}
		case yok && !tok:
			if !bok {
				mergedData[key] = yv
			} else if deepEqual(bv, yv) {
				// yours unchanged, theirs removed → removal wins.
			} else {
				if winner == "yours" {
					mergedData[key] = yv
				}
			}
		case tok && yok:
			// Both present and unequal. If one matches base, take the
			// other (that side is the only one that changed).
			switch {
			case bok && deepEqual(bv, tv):
				mergedData[key] = yv
			case bok && deepEqual(bv, yv):
				mergedData[key] = tv
			default:
				// Both changed from base (or base absent) to different
				// values → last-writer-wins.
				if winner == "theirs" {
					mergedData[key] = tv
				} else {
					mergedData[key] = yv
				}
			}
		default:
			// Neither side has it but base does - keep base's value.
			if bok {
				mergedData[key] = bv
			}
		}
	}

	out := Record{Meta: mergedMeta, Data: mergedData}
	bytes, err := out.CanonicalJSON()
	if err != nil {
		return MergeResult{}, err
	}
	return MergeResult{Merged: bytes}, nil
}

// IsRecordPath reports whether p matches storage/<template>/<name>.meta.json (no traversal, not the images dir).
func IsRecordPath(p string) bool {
	if p == "" {
		return false
	}
	if !endsWith(p, ".meta.json") {
		return false
	}
	if !startsWith(p, "storage/") {
		return false
	}
	if containsDoubleDot(p) {
		return false
	}
	parts := splitSlash(p)
	if len(parts) != 3 {
		return false
	}
	if parts[1] == "" || parts[2] == "" || parts[1] == "images" {
		return false
	}
	return true
}

// Inlined to keep this leaf file free of a strings import.

func endsWith(s, suffix string) bool {
	if len(s) < len(suffix) {
		return false
	}
	return s[len(s)-len(suffix):] == suffix
}

func startsWith(s, prefix string) bool {
	if len(s) < len(prefix) {
		return false
	}
	return s[:len(prefix)] == prefix
}

func containsDoubleDot(s string) bool {
	for i := 0; i+1 < len(s); i++ {
		if s[i] == '.' && s[i+1] == '.' {
			return true
		}
	}
	return false
}

func splitSlash(s string) []string {
	out := []string{}
	cur := ""
	for i := 0; i < len(s); i++ {
		if s[i] == '/' {
			out = append(out, cur)
			cur = ""
			continue
		}
		cur += string(s[i])
	}
	out = append(out, cur)
	return out
}
