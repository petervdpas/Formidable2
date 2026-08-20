package connection

import (
	"encoding/json"
	"strconv"
	"strings"
)

// ValidPointer reports whether s is well-formed RFC 6901. A pointer is either
// empty (the whole document) or a run of "/"-prefixed tokens, where "~" is only
// legal as the escape "~0" (a literal ~) or "~1" (a literal /).
func ValidPointer(s string) bool {
	if s == "" {
		return true
	}
	if !strings.HasPrefix(s, "/") {
		return false
	}
	for tok := range strings.SplitSeq(s[1:], "/") {
		for i := 0; i < len(tok); i++ {
			if tok[i] != '~' {
				continue
			}
			if i+1 >= len(tok) || (tok[i+1] != '0' && tok[i+1] != '1') {
				return false
			}
			i++
		}
	}
	return true
}

// escapeToken writes a literal property name as an RFC 6901 token, so a key
// containing a slash addresses the key rather than a nesting level.
func escapeToken(tok string) string {
	tok = strings.ReplaceAll(tok, "~", "~0")
	return strings.ReplaceAll(tok, "/", "~1")
}

// unescapeToken reverses the RFC 6901 escapes. Order matters: ~1 first, so a
// literal "~1" written as "~01" does not decode into a slash.
func unescapeToken(tok string) string {
	tok = strings.ReplaceAll(tok, "~1", "/")
	return strings.ReplaceAll(tok, "~0", "~")
}

// ResolvePointer walks ptr through a decoded JSON value and returns the target
// as a string. ok is false when the pointer is malformed or does not resolve,
// which is how a caller tells "the field is absent" from "the field is empty".
//
// Decode with json.Decoder.UseNumber when large integer ids matter; float64
// input is accepted but loses precision past 2^53, which is json's problem
// rather than this walker's.
func ResolvePointer(doc any, ptr string) (string, bool) {
	node, ok := ResolveNode(doc, ptr)
	if !ok {
		return "", false
	}
	return stringify(node), true
}

// ResolveNode is ResolvePointer without the stringification, for callers that
// need the container itself rather than a value: the items array of a list
// response, or one item to project fields out of.
func ResolveNode(doc any, ptr string) (any, bool) {
	if !ValidPointer(ptr) {
		return nil, false
	}
	cur := doc
	if ptr != "" {
		for raw := range strings.SplitSeq(ptr[1:], "/") {
			tok := unescapeToken(raw)
			switch node := cur.(type) {
			case map[string]any:
				v, ok := node[tok]
				if !ok {
					return nil, false
				}
				cur = v
			case []any:
				i, err := strconv.Atoi(tok)
				if err != nil || i < 0 || i >= len(node) {
					return nil, false
				}
				cur = node[i]
			default:
				return nil, false
			}
		}
	}
	return cur, true
}

// stringify renders a resolved node the way a form value wants it: scalars
// plain, null empty, and a container as its compact JSON rather than a guess.
func stringify(v any) string {
	switch t := v.(type) {
	case nil:
		return ""
	case string:
		return t
	case bool:
		return strconv.FormatBool(t)
	case json.Number:
		return t.String()
	case float64:
		return strconv.FormatFloat(t, 'f', -1, 64)
	default:
		b, err := json.Marshal(t)
		if err != nil {
			return ""
		}
		return string(b)
	}
}
