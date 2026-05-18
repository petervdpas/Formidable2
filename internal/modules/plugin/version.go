package plugin

import (
	"strconv"
	"strings"
)

// compareVersions orders two dotted-numeric plugin version strings.
// Returns -1 if a < b, 0 if equal, +1 if a > b.
//
// Leading 'v' is tolerated. Missing components default to 0 so
// "1.2" == "1.2.0". Each component is parsed up to its first
// non-digit; any pre-release or build suffix is dropped, so
// "1.0.0-rc1" compares equal to "1.0.0". That is deliberately
// looser than strict semver — the version field's sole purpose
// here is downgrade-protection on import, and the simple form
// covers the practical cases without dragging in a semver dep.
func compareVersions(a, b string) int {
	pa := splitVersion(a)
	pb := splitVersion(b)
	n := len(pa)
	if len(pb) > n {
		n = len(pb)
	}
	for i := 0; i < n; i++ {
		va, vb := 0, 0
		if i < len(pa) {
			va = pa[i]
		}
		if i < len(pb) {
			vb = pb[i]
		}
		if va < vb {
			return -1
		}
		if va > vb {
			return 1
		}
	}
	return 0
}

func splitVersion(s string) []int {
	s = strings.TrimPrefix(strings.TrimSpace(s), "v")
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ".")
	out := make([]int, len(parts))
	for i, p := range parts {
		end := 0
		for end < len(p) && p[end] >= '0' && p[end] <= '9' {
			end++
		}
		if end == 0 {
			out[i] = 0
			continue
		}
		n, _ := strconv.Atoi(p[:end])
		out[i] = n
	}
	return out
}
