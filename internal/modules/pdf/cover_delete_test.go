package pdf

import (
	"errors"
	"log/slog"
	"testing"
)

func TestDeleteDiskCover_RemovesExisting(t *testing.T) {
	fs := scaffoldedFS(t)
	key := onDiskCoversDir + "/classic.html"
	if !fs.FileExists(key) {
		t.Fatalf("classic.html missing after scaffold")
	}
	if err := deleteDiskCover(fs, "classic"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if fs.FileExists(key) {
		t.Errorf("classic.html still on disk after delete")
	}
}

func TestDeleteDiskCover_MissingIsNoOp(t *testing.T) {
	fs := scaffoldedFS(t)
	if err := deleteDiskCover(fs, "never-existed"); err != nil {
		t.Errorf("delete of missing cover returned err = %v, want nil (DeleteFile is no-op on missing)", err)
	}
}

func TestDeleteDiskCover_RefusesReservedNames(t *testing.T) {
	cases := []struct {
		name    string
		wantErr error
	}{
		{"", ErrCoverNotFound},
		{"signature", ErrCoverNotFound},
		{"foo/bar", ErrCoverNotFound},
		{"foo\\bar", ErrCoverNotFound},
		{".hidden", ErrCoverNotFound},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fs := scaffoldedFS(t)
			err := deleteDiskCover(fs, tc.name)
			if !errors.Is(err, tc.wantErr) {
				t.Errorf("delete(%q) err = %v, want wrapping %v", tc.name, err, tc.wantErr)
			}
		})
	}
}

func TestDeleteDiskCover_NilFS(t *testing.T) {
	err := deleteDiskCover(nil, "classic")
	if !errors.Is(err, ErrCoverPathInvalid) {
		t.Errorf("nil fs: err = %v, want ErrCoverPathInvalid", err)
	}
}

func TestDeleteDiskCover_PropagatesFSError(t *testing.T) {
	fs := scaffoldedFS(t)
	fs.deleteErr = errors.New("permission denied")
	err := deleteDiskCover(fs, "classic")
	if err == nil {
		t.Errorf("expected fs error to surface")
	}
}

func TestManager_DeleteCover_Logs(t *testing.T) {
	m, _, _, _, _ := newActiveManager(t)
	if err := m.DeleteCover("classic"); err != nil {
		t.Fatalf("DeleteCover: %v", err)
	}
	if m.store.fs.FileExists(onDiskCoversDir + "/classic.html") {
		t.Errorf("classic still on disk after Manager.DeleteCover")
	}
}

func TestManager_DeleteCover_SeedScaffoldsBack(t *testing.T) {
	// Verifies the "delete-a-seed = reset" behavior promised in the
	// public API doc. Delete classic, run scaffold again (what boot
	// does), classic should reappear from the embedded seed.
	m, _, _, _, _ := newActiveManager(t)
	if err := m.DeleteCover("classic"); err != nil {
		t.Fatalf("DeleteCover: %v", err)
	}
	if err := scaffoldCovers(m.store.fs, slog.Default()); err != nil {
		t.Fatalf("scaffold after delete: %v", err)
	}
	if !m.store.fs.FileExists(onDiskCoversDir + "/classic.html") {
		t.Errorf("classic.html did NOT reappear after rescaffold - seed not re-written")
	}
}

func TestLoadDiskCoverRaw_ReturnsBodyWithoutValidating(t *testing.T) {
	fs := scaffoldedFS(t)
	// Corrupt a seed so it would fail ValidateCover.
	fs.files[onDiskCoversDir+"/classic.html"] = "no magic line here"

	html, err := loadDiskCoverRaw(fs, "classic")
	if err != nil {
		t.Fatalf("raw load of broken cover should succeed: %v", err)
	}
	if html != "no magic line here" {
		t.Errorf("body = %q, want raw passthrough", html)
	}
}

func TestLoadDiskCoverRaw_RefusesReserved(t *testing.T) {
	fs := scaffoldedFS(t)
	for _, name := range []string{"", "signature", "foo/bar", "foo\\bar"} {
		if _, err := loadDiskCoverRaw(fs, name); err == nil {
			t.Errorf("loadDiskCoverRaw(%q) err = nil, want refused", name)
		}
	}
}

func TestLoadDiskCoverRaw_MissingFile(t *testing.T) {
	fs := scaffoldedFS(t)
	_, err := loadDiskCoverRaw(fs, "no-such-cover")
	if !errors.Is(err, ErrCoverNotFound) {
		t.Errorf("err = %v, want ErrCoverNotFound", err)
	}
}

func TestService_LoadCover_DelegatesToManager(t *testing.T) {
	m, _, _, _, _ := newActiveManager(t)
	svc := NewService(m)
	html, err := svc.LoadCover("classic")
	if err != nil {
		t.Fatalf("svc.LoadCover: %v", err)
	}
	if html == "" {
		t.Errorf("svc.LoadCover returned empty body")
	}
}

func TestService_DeleteCover_DelegatesToManager(t *testing.T) {
	m, _, _, _, _ := newActiveManager(t)
	svc := NewService(m)

	// Seed a user-authored cover so delete has something distinct to remove.
	if err := svc.SaveCover("custom", testValidCoverHTML("Custom")); err != nil {
		t.Fatalf("seed SaveCover: %v", err)
	}
	if !m.store.fs.FileExists(onDiskCoversDir + "/custom.html") {
		t.Fatalf("custom not seeded")
	}

	if err := svc.DeleteCover("custom"); err != nil {
		t.Fatalf("svc.DeleteCover: %v", err)
	}
	if m.store.fs.FileExists(onDiskCoversDir + "/custom.html") {
		t.Errorf("custom.html still on disk after Service.DeleteCover")
	}
}

// testValidCoverHTML returns a minimal but validation-passing cover
// for tests that need to seed something on disk without depending on
// the embedded seeds.
func testValidCoverHTML(label string) string {
	return `<!--
  formidable-cover: 1
  name: ` + label + `
-->
<section class="cover"><div class="cover-page">
  <p class="cover-title">{{.Title}}</p>
</div></section><span data-cover-end></span>
`
}
