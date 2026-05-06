package index

import "time"

// atimeNoop is reused across chtimes calls — we don't care about
// access times, just keep them stable.
var atimeNoop = time.Unix(0, 0)

// secsToTime turns a unix-second value into time.Time. Convenience for
// scan tests that pin specific mtimes.
func secsToTime(secs int64) time.Time { return time.Unix(secs, 0) }

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
