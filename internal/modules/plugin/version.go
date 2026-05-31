package plugin

import (
	"strconv"
	"strings"
)

// compareVersions orders two dotted-numeric versions (-1/0/+1). Tolerates a leading 'v', pads missing components with 0,
// and drops pre-release/build suffixes ("1.0.0-rc1" == "1.0.0"). Deliberately looser than semver: its only job is import downgrade-protection.
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
