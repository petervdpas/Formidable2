package gigot

import (
	"errors"
	"testing"
)

func TestManager_LedgerSummary_BlankContextErrors(t *testing.T) {
	m := NewManager(newFakeFS())
	if _, err := m.LedgerSummary(""); !errors.Is(err, ErrMissingContext) {
		t.Fatalf("want ErrMissingContext, got %v", err)
	}
}

func TestManager_LedgerSummary_EmptyContext_ZeroValues(t *testing.T) {
	m := NewManager(newFakeFS())
	dir := t.TempDir()

	sum, err := m.LedgerSummary(dir)
	if err != nil {
		t.Fatal(err)
	}
	if sum.Version != "" || sum.LastSync != "" {
		t.Errorf("expected empty version/lastSync on fresh context, got %+v", sum)
	}
	if sum.Scanned != 0 {
		t.Errorf("scanned = %d, want 0", sum.Scanned)
	}
	if len(sum.Changed) != 0 || len(sum.Deleted) != 0 {
		t.Errorf("expected empty diff, got changed=%v deleted=%v", sum.Changed, sum.Deleted)
	}
}

func TestManager_LedgerSummary_FirstSync_NoLedger_AllChanged(t *testing.T) {
	// On first sync the ledger is empty. Every managed file shows up as
	// Changed (so the UI surfaces "N pending pushes"). Deleted stays
	// empty - DiffAgainstRecord suppresses deletes when rec.Version is "".
	dir := t.TempDir()
	writeFile(t, dir, "templates/a.yaml", "x")
	writeFile(t, dir, "templates/b.yaml", "y")

	m := NewManager(newFakeFS())
	sum, err := m.LedgerSummary(dir)
	if err != nil {
		t.Fatal(err)
	}
	if sum.Scanned != 2 {
		t.Errorf("scanned = %d, want 2", sum.Scanned)
	}
	if len(sum.Changed) != 2 {
		t.Errorf("changed = %v, want both files pending", sum.Changed)
	}
	if len(sum.Deleted) != 0 {
		t.Errorf("first-sync deleted must be empty, got %v", sum.Deleted)
	}
}

func TestManager_LedgerSummary_LedgerInSync_NoChanges(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "templates/a.yaml", "stable\n")
	sha := GitBlobSha([]byte("stable\n"))

	fs := newFakeFS()
	m := NewManager(fs)
	if err := m.WriteTrackRecord(dir, TrackRecord{
		Version:  "v1",
		LastSync: "2026-05-14T12:00:00Z",
		Files:    map[string]string{"templates/a.yaml": sha},
	}); err != nil {
		t.Fatal(err)
	}

	sum, err := m.LedgerSummary(dir)
	if err != nil {
		t.Fatal(err)
	}
	if sum.Version != "v1" || sum.LastSync != "2026-05-14T12:00:00Z" {
		t.Errorf("ledger fields not surfaced: %+v", sum)
	}
	if len(sum.Changed) != 0 || len(sum.Deleted) != 0 {
		t.Errorf("clean ledger should report no pending, got %+v", sum)
	}
	if sum.Scanned != 1 {
		t.Errorf("scanned = %d, want 1", sum.Scanned)
	}
}

func TestManager_LedgerSummary_ModifiedFile_ListedAsChanged(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "templates/a.yaml", "edited\n")

	fs := newFakeFS()
	m := NewManager(fs)
	if err := m.WriteTrackRecord(dir, TrackRecord{
		Version: "v1",
		Files:   map[string]string{"templates/a.yaml": "stale-sha"},
	}); err != nil {
		t.Fatal(err)
	}

	sum, err := m.LedgerSummary(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(sum.Changed) != 1 || sum.Changed[0] != "templates/a.yaml" {
		t.Errorf("changed = %v, want [templates/a.yaml]", sum.Changed)
	}
}

func TestManager_LedgerSummary_DeletedFile_ListedAsDeleted(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "templates/kept.yaml", "k")
	keptSha := GitBlobSha([]byte("k"))

	fs := newFakeFS()
	m := NewManager(fs)
	if err := m.WriteTrackRecord(dir, TrackRecord{
		Version: "v1",
		Files: map[string]string{
			"templates/kept.yaml": keptSha,
			"templates/gone.yaml": "anything",
		},
	}); err != nil {
		t.Fatal(err)
	}

	sum, err := m.LedgerSummary(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(sum.Changed) != 0 {
		t.Errorf("expected no changed, got %v", sum.Changed)
	}
	if len(sum.Deleted) != 1 || sum.Deleted[0] != "templates/gone.yaml" {
		t.Errorf("deleted = %v, want [templates/gone.yaml]", sum.Deleted)
	}
}

func TestService_LedgerSummary_NoContextConfigured(t *testing.T) {
	m := NewManager(newFakeFS())
	cfg := &fakeConfig{} // empty context
	s := NewService(m, nil, nil, cfg, nil)

	if _, err := s.LedgerSummary(); !errors.Is(err, ErrMissingContext) {
		t.Fatalf("want ErrMissingContext, got %v", err)
	}
}

func TestService_LedgerSummary_NoCfgInjected(t *testing.T) {
	m := NewManager(newFakeFS())
	s := NewService(m, nil, nil, nil, nil)
	if _, err := s.LedgerSummary(); !errors.Is(err, ErrMissingContext) {
		t.Fatalf("nil cfg must surface ErrMissingContext, got %v", err)
	}
}

func TestService_LedgerSummary_PassesContextFolderThrough(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "templates/a.yaml", "x")

	m := NewManager(newFakeFS())
	cfg := &fakeConfig{context: dir}
	s := NewService(m, nil, nil, cfg, nil)

	sum, err := s.LedgerSummary()
	if err != nil {
		t.Fatal(err)
	}
	if sum.Scanned != 1 {
		t.Errorf("service should walk the configured context: scanned=%d", sum.Scanned)
	}
}

func TestManager_LedgerSummary_DoesNotMutateLedger(t *testing.T) {
	// Pure read - calling LedgerSummary repeatedly does not write any
	// new ledger version, so a Push that runs afterwards sees the same
	// pending diff. Without this contract the UI's "preview" call would
	// silently advance the ledger.
	dir := t.TempDir()
	writeFile(t, dir, "templates/a.yaml", "edited\n")

	fs := newFakeFS()
	m := NewManager(fs)
	original := TrackRecord{
		Version:  "v1",
		LastSync: "2026-05-14T11:00:00Z",
		Files:    map[string]string{"templates/a.yaml": "stale"},
	}
	if err := m.WriteTrackRecord(dir, original); err != nil {
		t.Fatal(err)
	}

	for range 3 {
		if _, err := m.LedgerSummary(dir); err != nil {
			t.Fatal(err)
		}
	}

	after := m.ReadTrackRecord(dir)
	if after.Version != original.Version || after.LastSync != original.LastSync {
		t.Errorf("ledger mutated by LedgerSummary: %+v vs %+v", after, original)
	}
}
