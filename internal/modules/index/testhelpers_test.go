package index

import (
	"path/filepath"
	"testing"
	"time"
)

// atimeNoop is reused across chtimes calls - we don't care about
// access times, just keep them stable.
var atimeNoop = time.Unix(0, 0)

// newEmptyManager returns a Manager with only the schema migrated, no rows.
func newEmptyManager(t *testing.T) *Manager {
	t.Helper()
	m, err := NewManager(filepath.Join(t.TempDir(), "empty.db"))
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}
	t.Cleanup(func() { m.Close() })
	return m
}

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
