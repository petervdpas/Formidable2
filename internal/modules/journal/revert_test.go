package journal

import (
	"path/filepath"
	"sync"
	"testing"

	"github.com/petervdpas/formidable2/internal/modules/system"
)

// pendingOp returns the pending op recorded for rel under backend, or "" if the
// path is not pending.
func pendingOp(pr PendingResult, rel string) string {
	for _, p := range pr.Paths {
		if p.Path == rel {
			return p.Op
		}
	}
	return ""
}

func TestRecordRevert_ClearsPendingCreate(t *testing.T) {
	m, _, root := newTestManager(t)
	if err := m.Configure(root, "git"); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "storage/adapters/test.meta.json")
	m.RecordOp("create", path, nil)
	if pendingOp(m.Pending("git"), "storage/adapters/test.meta.json") != "create" {
		t.Fatalf("setup: expected pending create, got %+v", m.Pending("git"))
	}

	m.RecordRevert(path)

	if c := m.Pending("git").Count; c != 0 {
		t.Errorf("after revert: pending count = %d, want 0 (%+v)", c, m.Pending("git"))
	}
}

func TestRecordRevert_ClearsPendingUpdate(t *testing.T) {
	m, _, root := newTestManager(t)
	_ = m.Configure(root, "git")
	path := filepath.Join(root, "templates/a.yaml")
	m.RecordOp("update", path, nil)
	m.RecordRevert(path)
	if c := m.Pending("git").Count; c != 0 {
		t.Errorf("pending count = %d, want 0", c)
	}
}

func TestRecordRevert_ClearsPendingDelete(t *testing.T) {
	m, _, root := newTestManager(t)
	_ = m.Configure(root, "git")
	path := filepath.Join(root, "storage/x/y.json")
	m.RecordOp("delete", path, nil)
	m.RecordRevert(path)
	if c := m.Pending("git").Count; c != 0 {
		t.Errorf("pending count = %d, want 0", c)
	}
}

// The clear must survive a restart: rebuildPending replays the log, and the
// revert marker has to cancel the earlier op or the entry would resurrect.
func TestRecordRevert_DurableAcrossReload(t *testing.T) {
	m, _, root := newTestManager(t)
	_ = m.Configure(root, "git")
	path := filepath.Join(root, "templates/a.yaml")
	m.RecordOp("create", path, nil)
	m.RecordRevert(path)

	// Fresh manager on the same root: pending is rebuilt purely from disk.
	sys2 := system.NewManager(root, nil)
	m2 := NewManager(sys2, nil, nil)
	if err := m2.Configure(root, "git"); err != nil {
		t.Fatal(err)
	}
	if c := m2.Pending("git").Count; c != 0 {
		t.Errorf("reloaded pending count = %d, want 0 (revert not durable: %+v)", c, m2.Pending("git"))
	}
}

// A revert with nothing pending is harmless: count stays 0, no panic, and a
// marker is still appended (so the clear is durable even if the op preceded
// this process).
func TestRecordRevert_NoPendingIsHarmless(t *testing.T) {
	m, _, root := newTestManager(t)
	_ = m.Configure(root, "git")
	path := filepath.Join(root, "templates/never-touched.yaml")

	m.RecordRevert(path)

	if c := m.Pending("git").Count; c != 0 {
		t.Errorf("pending count = %d, want 0", c)
	}
	entries := m.RecentEntries(0)
	if len(entries) != 1 || entries[0].Op != OpRevert {
		t.Errorf("expected one revert entry on disk, got %+v", entries)
	}
}

// Revert is not a permanent tombstone: re-creating the same path after a revert
// makes it pending again.
func TestRecordRevert_RecreateAfterRevertReappears(t *testing.T) {
	m, _, root := newTestManager(t)
	_ = m.Configure(root, "git")
	path := filepath.Join(root, "templates/a.yaml")
	m.RecordOp("create", path, nil)
	m.RecordRevert(path)
	m.RecordOp("create", path, nil)

	if op := pendingOp(m.Pending("git"), "templates/a.yaml"); op != "create" {
		t.Errorf("after re-create: pending op = %q, want create", op)
	}
}

// Reverting one path leaves the rest of the pending set intact.
func TestRecordRevert_OnlyClearsTargetPath(t *testing.T) {
	m, _, root := newTestManager(t)
	_ = m.Configure(root, "git")
	m.RecordOp("create", filepath.Join(root, "templates/keep.yaml"), nil)
	m.RecordOp("create", filepath.Join(root, "templates/drop.yaml"), nil)

	m.RecordRevert(filepath.Join(root, "templates/drop.yaml"))

	pr := m.Pending("git")
	if pr.Count != 1 || pendingOp(pr, "templates/keep.yaml") != "create" {
		t.Errorf("expected only keep.yaml pending, got %+v", pr)
	}
	if pendingOp(pr, "templates/drop.yaml") != "" {
		t.Error("drop.yaml should be gone from pending")
	}
}

// A discard reverts the file for every backend, so the path must drop from both
// the git and gigot pending buckets.
func TestRecordRevert_ClearsBothBackends(t *testing.T) {
	m, _, root := newTestManager(t)
	_ = m.Configure(root, "git")
	path := filepath.Join(root, "storage/x/y.json")
	m.RecordOp("update", path, nil)
	if m.Pending("git").Count != 1 || m.Pending("gigot").Count != 1 {
		t.Fatalf("setup: both backends should see the op")
	}

	m.RecordRevert(path)

	if m.Pending("git").Count != 0 {
		t.Errorf("git pending not cleared: %+v", m.Pending("git"))
	}
	if m.Pending("gigot").Count != 0 {
		t.Errorf("gigot pending not cleared: %+v", m.Pending("gigot"))
	}
}

// ── Unhappy paths ────────────────────────────────────────────────────

func TestRecordRevert_NoOpWhenContextUnset(t *testing.T) {
	m, _, _ := newTestManager(t) // never Configured
	m.RecordRevert("/tmp/anything/templates/a.yaml")
	if pr := m.Pending("git"); pr.Count != 0 {
		t.Errorf("unconfigured revert should be inert, got %+v", pr)
	}
	if got := m.RecentEntries(0); len(got) != 0 {
		t.Errorf("unconfigured revert must not write a log entry, got %+v", got)
	}
}

func TestRecordRevert_NoOpWhenBackendNone(t *testing.T) {
	m, _, root := newTestManager(t)
	_ = m.Configure(root, BackendNone)
	m.RecordRevert(filepath.Join(root, "templates/a.yaml"))
	if got := m.RecentEntries(0); len(got) != 0 {
		t.Errorf("backend=none revert must be inert, got %+v", got)
	}
}

func TestRecordRevert_NoOpForUntrackedPath(t *testing.T) {
	m, _, root := newTestManager(t)
	_ = m.Configure(root, "git")
	// config/ is outside the tracked templates/ + storage/ trees.
	m.RecordRevert(filepath.Join(root, "config/user.json"))
	if got := m.RecentEntries(0); len(got) != 0 {
		t.Errorf("untracked-path revert must not write a log entry, got %+v", got)
	}
}

func TestRecordRevert_NoOpForOutsideContext(t *testing.T) {
	m, _, root := newTestManager(t)
	_ = m.Configure(root, "git")
	m.RecordRevert("/etc/passwd")
	if got := m.RecentEntries(0); len(got) != 0 {
		t.Errorf("outside-context revert must be inert, got %+v", got)
	}
}

func TestRecordRevert_NoOpForEmptyPath(t *testing.T) {
	m, _, root := newTestManager(t)
	_ = m.Configure(root, "git")
	m.RecordRevert("")
	if got := m.RecentEntries(0); len(got) != 0 {
		t.Errorf("empty-path revert must be inert, got %+v", got)
	}
}

// parseLine must accept a well-formed revert and reject one without a path,
// matching the other file-op shapes.
func TestParseLine_RevertShape(t *testing.T) {
	good, err := parseLine(`{"ts":"2026-06-04T00:00:00Z","op":"revert","path":"templates/a.yaml"}`)
	if err != nil || good == nil || good.Op != OpRevert {
		t.Errorf("valid revert line not parsed: entry=%+v err=%v", good, err)
	}
	bad, err := parseLine(`{"ts":"2026-06-04T00:00:00Z","op":"revert"}`) // no path
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if bad != nil {
		t.Errorf("revert without path should be dropped, got %+v", bad)
	}
}

// Concurrent RecordOp / RecordRevert / Pending must be race-free.
func TestRecordRevert_ConcurrentSafety(t *testing.T) {
	m, _, root := newTestManager(t)
	_ = m.Configure(root, "git")
	path := filepath.Join(root, "templates/x.yaml")

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			m.RecordOp("update", path, nil)
			m.RecordRevert(path)
			_ = m.Pending("git")
		}()
	}
	wg.Wait()
	// Final state is order-dependent; the test exists for -race, not the count.
}
