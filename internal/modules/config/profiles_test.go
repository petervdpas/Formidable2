package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestProfileDisplayName_MalformedFileShowsUnknown verifies a corrupt
// sibling profile does not break the picker: it surfaces as "(unknown)"
// and the good profile still lists with its real name.
func TestProfileDisplayName_MalformedFileShowsUnknown(t *testing.T) {
	m, sys, _ := newTestManager(t)
	if err := sys.SaveFile("config/broken.json", "{not valid json"); err != nil {
		t.Fatalf("seed broken: %v", err)
	}

	profiles, err := m.ListAvailableProfiles()
	if err != nil {
		t.Fatalf("ListAvailableProfiles: %v", err)
	}
	got := map[string]string{}
	for _, p := range profiles {
		got[p.Value] = p.Display
	}
	if got["broken.json"] != "(unknown)" {
		t.Errorf("broken.json display = %q, want (unknown)", got["broken.json"])
	}
	if got["user.json"] != "Default Profile" {
		t.Errorf("user.json display = %q, want Default Profile", got["user.json"])
	}
}

func TestSwitchUserProfile_EmptyNameRejected(t *testing.T) {
	m, _, _ := newTestManager(t)
	_, err := m.SwitchUserProfile("")
	if err == nil {
		t.Fatal("expected error for empty profile name")
	}
	if !strings.Contains(err.Error(), "invalid profile filename") {
		t.Errorf("err = %v, want invalid-filename message", err)
	}
	if got := m.CurrentProfileFilename(); got != defaultProfileName {
		t.Errorf("active profile changed to %q after failed switch, want %q", got, defaultProfileName)
	}
}

func TestSwitchUserProfile_DotPrefixRejectedBeforeFormatCheck(t *testing.T) {
	m, _, _ := newTestManager(t)
	_, err := m.SwitchUserProfile(".boot.json")
	if err == nil {
		t.Fatal("expected error for dot-prefixed profile")
	}
	if !strings.Contains(err.Error(), "cannot start with '.'") {
		t.Errorf("err = %v, want dot-prefix rejection", err)
	}
}

func TestDeleteUserProfile_EmptyNameMissingFilenameCode(t *testing.T) {
	m, _, _ := newTestManager(t)
	r := m.DeleteUserProfile("")
	if r.Success {
		t.Fatal("empty delete must fail")
	}
	if r.Code != "missing_filename" {
		t.Errorf("code = %q, want missing_filename", r.Code)
	}
}

func TestDeleteUserProfile_MissingFileNotFoundCode(t *testing.T) {
	m, _, _ := newTestManager(t)
	r := m.DeleteUserProfile("ghost.json")
	if r.Success {
		t.Fatal("deleting a non-existent profile must fail")
	}
	if r.Code != "not_found" {
		t.Errorf("code = %q, want not_found", r.Code)
	}
}

func TestExportUserProfile_MissingArgsNoCode(t *testing.T) {
	m, _, _ := newTestManager(t)
	r := m.ExportUserProfile("", "/tmp/whatever.json", true)
	if r.Success {
		t.Fatal("empty profileFilename export must fail")
	}
	if r.Code != "" {
		t.Errorf("code = %q, want empty (validation case sets only Error)", r.Code)
	}
	if r.Error != "Missing profileFilename or targetPath." {
		t.Errorf("Error = %q", r.Error)
	}

	r = m.ExportUserProfile("user.json", "", true)
	if r.Success || r.Error != "Missing profileFilename or targetPath." {
		t.Errorf("empty targetPath: got %+v", r)
	}
}

func TestImportUserProfile_EmptySourceRejected(t *testing.T) {
	m, _, _ := newTestManager(t)
	r := m.ImportUserProfile("", "dest.json", false)
	if r.Success {
		t.Fatal("empty sourcePath import must fail")
	}
	if r.Error != "Missing sourcePath." {
		t.Errorf("Error = %q, want Missing sourcePath.", r.Error)
	}
}

// TestImportUserProfile_MalformedSourceUndoesCopy exercises the rollback:
// a copied-but-unparseable source must be deleted and reported as
// invalid_config, leaving no orphan in config/.
func TestImportUserProfile_MalformedSourceUndoesCopy(t *testing.T) {
	m, _, root := newTestManager(t)
	src := filepath.Join(root, "bad-source.json")
	if err := os.WriteFile(src, []byte("{ not json"), 0o644); err != nil {
		t.Fatalf("seed bad source: %v", err)
	}

	r := m.ImportUserProfile(src, "imp.json", false)
	if r.Success {
		t.Fatal("malformed source import must fail")
	}
	if r.Code != "invalid_config" {
		t.Errorf("code = %q, want invalid_config", r.Code)
	}
	if _, err := os.Stat(filepath.Join(root, "config", "imp.json")); !os.IsNotExist(err) {
		t.Errorf("orphan target must be removed, stat err = %v", err)
	}
}

// TestImportUserProfile_NonObjectSourceUndoesCopy covers a source that is valid
// JSON but the wrong shape (an array). parseUserConfig's probe-decode into a map
// rejects it, so the copy is rolled back and no orphan remains.
func TestImportUserProfile_NonObjectSourceUndoesCopy(t *testing.T) {
	m, _, root := newTestManager(t)
	src := filepath.Join(root, "array-source.json")
	if err := os.WriteFile(src, []byte(`[1,2,3]`), 0o644); err != nil {
		t.Fatalf("seed array source: %v", err)
	}
	r := m.ImportUserProfile(src, "arr.json", false)
	if r.Success {
		t.Fatal("array-shaped source import must fail")
	}
	if r.Code != "invalid_config" {
		t.Errorf("code = %q, want invalid_config", r.Code)
	}
	if _, err := os.Stat(filepath.Join(root, "config", "arr.json")); !os.IsNotExist(err) {
		t.Errorf("orphan target must be removed, stat err = %v", err)
	}
}
