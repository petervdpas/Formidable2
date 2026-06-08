package git

import (
	"errors"
	"testing"
)

// ── parseStatusPorcelain ────────────────────────────────────────────────

func TestParseStatusPorcelain_BranchAheadBehind(t *testing.T) {
	st := parseStatusPorcelain("## master...origin/master [ahead 2, behind 3]\n")
	if st.Branch != "master" {
		t.Errorf("Branch = %q, want master", st.Branch)
	}
	if st.Tracking != "refs/remotes/origin/master" {
		t.Errorf("Tracking = %q", st.Tracking)
	}
	if st.Ahead != 2 || st.Behind != 3 {
		t.Errorf("ahead/behind = %d/%d, want 2/3", st.Ahead, st.Behind)
	}
	if !st.Clean {
		t.Error("no file lines should be Clean")
	}
}

func TestParseStatusPorcelain_BranchNoUpstream(t *testing.T) {
	st := parseStatusPorcelain("## feature\n")
	if st.Branch != "feature" || st.Tracking != "" {
		t.Errorf("got branch=%q tracking=%q, want feature/empty", st.Branch, st.Tracking)
	}
	if st.Ahead != 0 || st.Behind != 0 {
		t.Errorf("ahead/behind should be 0 with no upstream")
	}
}

func TestParseStatusPorcelain_BehindOnly(t *testing.T) {
	st := parseStatusPorcelain("## main...origin/main [behind 1]\n")
	if st.Behind != 1 || st.Ahead != 0 {
		t.Errorf("ahead/behind = %d/%d, want 0/1", st.Ahead, st.Behind)
	}
}

func TestParseStatusPorcelain_Detached(t *testing.T) {
	st := parseStatusPorcelain("## HEAD (no branch)\n")
	if !st.Detached {
		t.Error("expected Detached")
	}
	if st.Branch != "" {
		t.Errorf("Branch = %q, want empty when detached", st.Branch)
	}
}

func TestParseStatusPorcelain_NewbornRepo(t *testing.T) {
	st := parseStatusPorcelain("## No commits yet on master\n")
	if st.Branch != "master" {
		t.Errorf("Branch = %q, want master for newborn", st.Branch)
	}
	if st.Detached {
		t.Error("newborn is not detached")
	}
}

func TestParseStatusPorcelain_FileClassification(t *testing.T) {
	raw := "## master\n" +
		" M modified.txt\n" + // worktree-modified
		"?? untracked.txt\n" + // untracked
		"A  staged-add.txt\n" + // staged
		" D worktree-deleted.txt\n" + // worktree delete
		"D  index-deleted.txt\n" + // staged delete
		"UU conflict.txt\n" // unmerged
	st := parseStatusPorcelain(raw)

	want := map[string][]string{
		"modified":   {"modified.txt"},
		"untracked":  {"untracked.txt"},
		"deleted":    {"index-deleted.txt", "worktree-deleted.txt"},
		"conflicted": {"conflict.txt"},
	}
	if !sliceEq(st.Modified, want["modified"]) {
		t.Errorf("Modified = %v", st.Modified)
	}
	if !sliceEq(st.Untracked, want["untracked"]) {
		t.Errorf("Untracked = %v", st.Untracked)
	}
	if !sliceEq(st.Deleted, want["deleted"]) {
		t.Errorf("Deleted = %v, want %v", st.Deleted, want["deleted"])
	}
	if !sliceEq(st.Conflicted, want["conflicted"]) {
		t.Errorf("Conflicted = %v", st.Conflicted)
	}
	// staged-add + index-deleted both land in Staged.
	if !sliceEq(st.Staged, []string{"index-deleted.txt", "staged-add.txt"}) {
		t.Errorf("Staged = %v", st.Staged)
	}
	if st.Clean {
		t.Error("a dirty status must not be Clean")
	}
}

func TestParseStatusPorcelain_Rename(t *testing.T) {
	st := parseStatusPorcelain("## master\nR  old.txt -> new.txt\n")
	if !sliceEq(st.Renamed, []string{"new.txt"}) {
		t.Errorf("Renamed = %v, want [new.txt]", st.Renamed)
	}
	if !sliceEq(st.Staged, []string{"new.txt"}) {
		t.Errorf("Staged = %v, want [new.txt]", st.Staged)
	}
}

func TestParseStatusPorcelain_EmptyIsClean(t *testing.T) {
	st := parseStatusPorcelain("")
	if !st.Clean {
		t.Error("empty porcelain must be Clean")
	}
	if st.Modified == nil || st.Untracked == nil {
		t.Error("slices must be non-nil (JSON-friendly), not nil")
	}
}

func TestParseStatusPorcelain_IgnoresTooShortLines(t *testing.T) {
	// A stray short line must not panic on the line[:2]/line[3:] slicing.
	st := parseStatusPorcelain("## master\nX\n")
	if !st.Clean {
		t.Errorf("malformed short line should be ignored, got %+v", st)
	}
}

// ── sysgitBackend wiring ────────────────────────────────────────────────

func TestSysgitBackend_DiscardCallsRestore(t *testing.T) {
	fake := &fakeSysgit{available: true}
	b := &sysgitBackend{run: fake}
	if err := b.Discard(DiscardOptions{Path: "/repo", File: "a.txt"}); err != nil {
		t.Fatalf("Discard: %v", err)
	}
	if fake.restoreCalls != 1 || fake.restoreFile != "a.txt" {
		t.Errorf("Restore calls=%d file=%q, want 1/a.txt", fake.restoreCalls, fake.restoreFile)
	}
}

func TestSysgitBackend_DiscardPropagatesError(t *testing.T) {
	fake := &fakeSysgit{available: true, restoreErr: errors.New("clean failed")}
	b := &sysgitBackend{run: fake}
	if err := b.Discard(DiscardOptions{Path: "/repo", File: "a.txt"}); err == nil {
		t.Error("Restore failure should propagate")
	}
}

func TestSysgitBackend_StatusParsesPorcelain(t *testing.T) {
	fake := &fakeSysgit{available: true, statusOut: "## main...origin/main [behind 2]\n M x.txt\n"}
	b := &sysgitBackend{run: fake}
	st, err := b.Status("/repo")
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if st.Branch != "main" || st.Behind != 2 || !sliceEq(st.Modified, []string{"x.txt"}) {
		t.Errorf("parsed status wrong: %+v", st)
	}
}

func TestSysgitBackend_StatusPropagatesError(t *testing.T) {
	fake := &fakeSysgit{available: true, statusErr: errors.New("not a repo")}
	b := &sysgitBackend{run: fake}
	if _, err := b.Status("/repo"); err == nil {
		t.Error("StatusPorcelain failure should propagate")
	}
}

func TestSysgitBackend_FetchErrorPropagates(t *testing.T) {
	fake := &fakeSysgit{available: true, err: errors.New("auth required")}
	b := &sysgitBackend{run: fake}
	if _, err := b.Fetch(FetchOptions{Path: "/repo"}); err == nil {
		t.Error("Fetch failure should propagate")
	}
}

func sliceEq(a, b []string) bool {
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
