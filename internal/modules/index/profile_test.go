package index

import (
	"path/filepath"
	"testing"
)

// TestTwoManagers_IsolatedFiles is the heart of the profile-switch
// model: each profile owns its own SQLite file. Two managers opened
// against different paths must each see only their own data — no
// shared state, no cross-contamination through SQLite's connection
// pooling or the modernc driver.
func TestTwoManagers_IsolatedFiles(t *testing.T) {
	dir := t.TempDir()
	pathA := filepath.Join(dir, "peter.db")
	pathB := filepath.Join(dir, "work.db")

	mA, err := NewManager(pathA)
	if err != nil {
		t.Fatal(err)
	}
	defer mA.Close()
	mB, err := NewManager(pathB)
	if err != nil {
		t.Fatal(err)
	}
	defer mB.Close()

	must(t, Reconcile(mA.DB(), ReconcileBatch{
		UpsertTemplates: []TemplateRow{{Filename: "personal.yaml", Name: "Personal", Mtime: 1}},
	}))
	must(t, Reconcile(mB.DB(), ReconcileBatch{
		UpsertTemplates: []TemplateRow{{Filename: "billing.yaml", Name: "Billing", Mtime: 2}},
	}))

	rowsA, err := mA.ListTemplates()
	if err != nil {
		t.Fatal(err)
	}
	if len(rowsA) != 1 || rowsA[0].Filename != "personal.yaml" {
		t.Errorf("A sees %d rows: %+v; want only personal.yaml", len(rowsA), rowsA)
	}

	rowsB, err := mB.ListTemplates()
	if err != nil {
		t.Fatal(err)
	}
	if len(rowsB) != 1 || rowsB[0].Filename != "billing.yaml" {
		t.Errorf("B sees %d rows: %+v; want only billing.yaml", len(rowsB), rowsB)
	}
}

// TestCloseAndReopen_PreservesData simulates the "swap" half of
// swap-and-drain: closing the active manager and re-opening a fresh
// handle must yield the persisted state intact.
func TestCloseAndReopen_PreservesData(t *testing.T) {
	path := filepath.Join(t.TempDir(), "p.db")

	m1, err := NewManager(path)
	if err != nil {
		t.Fatal(err)
	}
	must(t, Reconcile(m1.DB(), ReconcileBatch{
		UpsertTemplates: []TemplateRow{{Filename: "x.yaml", Name: "X", Mtime: 1}},
	}))
	if err := m1.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	m2, err := NewManager(path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer m2.Close()

	rows, err := m2.ListTemplates()
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].Filename != "x.yaml" || rows[0].Name != "X" {
		t.Errorf("after reopen, got %+v; want one row {x.yaml, X}", rows)
	}

	if rev, err := m2.Rev(); err != nil || rev != 1 {
		t.Errorf("rev across reopen = %d, err=%v; want 1", rev, err)
	}
}

// TestClosedManager_ErrorsGracefully ensures a Close() leaves the
// handle in a state where further reads error rather than panic.
// This matches the swap-and-drain contract: an in-flight goroutine
// holding the old manager pointer must get a clean error after the
// composition root closes it.
func TestClosedManager_ErrorsGracefully(t *testing.T) {
	m, err := NewManager(filepath.Join(t.TempDir(), "p.db"))
	if err != nil {
		t.Fatal(err)
	}
	if err := m.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	// Each read method should error (not panic). We don't assert the
	// specific error string — modernc may surface it differently
	// across versions — only that the call returns an error.
	if _, err := m.ListTemplates(); err == nil {
		t.Errorf("ListTemplates on closed manager: want error, got nil")
	}
	if _, err := m.ListForms("x.yaml", QueryOpts{}); err == nil {
		t.Errorf("ListForms on closed manager: want error, got nil")
	}
	if _, _, err := m.GetForm("x.yaml", "y.meta.json"); err == nil {
		t.Errorf("GetForm on closed manager: want error, got nil")
	}
	if _, err := m.ListByTags([]string{"x"}); err == nil {
		t.Errorf("ListByTags on closed manager: want error, got nil")
	}
	if _, err := m.Rev(); err == nil {
		t.Errorf("Rev on closed manager: want error, got nil")
	}
}

// TestProfileSwap_DataSurvivesPerProfile is the end-to-end shape: open A,
// write A, close A, open B, write B, close B, reopen A — A's data
// must be intact and contain none of B's writes.
func TestProfileSwap_DataSurvivesPerProfile(t *testing.T) {
	dir := t.TempDir()
	pathA := filepath.Join(dir, "a.db")
	pathB := filepath.Join(dir, "b.db")

	// Profile A activity.
	mA1, err := NewManager(pathA)
	must(t, err)
	must(t, Reconcile(mA1.DB(), ReconcileBatch{
		UpsertTemplates: []TemplateRow{{Filename: "ta.yaml", Name: "TA", Mtime: 1}},
	}))
	must(t, mA1.Close())

	// Switch to B.
	mB, err := NewManager(pathB)
	must(t, err)
	must(t, Reconcile(mB.DB(), ReconcileBatch{
		UpsertTemplates: []TemplateRow{{Filename: "tb.yaml", Name: "TB", Mtime: 1}},
	}))
	must(t, mB.Close())

	// Switch back to A.
	mA2, err := NewManager(pathA)
	must(t, err)
	defer mA2.Close()

	rows, err := mA2.ListTemplates()
	must(t, err)
	if len(rows) != 1 || rows[0].Filename != "ta.yaml" {
		t.Errorf("A reopened sees %+v; want one row {ta.yaml}", rows)
	}
}
