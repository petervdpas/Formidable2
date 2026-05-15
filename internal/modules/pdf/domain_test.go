package pdf

import (
	"errors"
	"log/slog"
	"testing"
	"time"
)

// newTestManager builds a Manager whose prober + store are wired
// against in-memory stubs. The returned trio (Manager, memFS, fakeFS
// + fakeVersions) lets each test set up "what binaries exist" and
// "what state file is on disk" independently.
func newTestManager(t *testing.T) (*Manager, *memFS, fakeFS, fakeVersions) {
	t.Helper()
	mem := newMemFS()
	fs := fakeFS{}
	vers := fakeVersions{}
	m := &Manager{
		log:    slog.Default(),
		store:  &store{fs: mem, log: slog.Default()},
		prober: &prober{fs: fs, versions: vers, goos: "linux", cacheRoot: "/cache/rod/browser"},
		nowFn:  func() time.Time { return time.Date(2026, 5, 15, 12, 0, 0, 0, time.UTC) },
		status: Status{Source: SourceUnset},
	}
	return m, mem, fs, vers
}

func TestNewManagerDefaultStatus(t *testing.T) {
	m := NewManager(nil, nil, nil, nil, nil, nil)
	s := m.Status()
	if s.Active {
		t.Errorf("fresh manager active=true, want false")
	}
	if s.Source != SourceUnset {
		t.Errorf("fresh manager source=%q, want %q", s.Source, SourceUnset)
	}
}

func TestActivate_ExplicitPathSuccess(t *testing.T) {
	m, mem, fs, vers := newTestManager(t)
	fs["/usr/bin/chromium"] = true
	vers["/usr/bin/chromium"] = struct {
		version string
		err     error
	}{version: "Chromium 148.0", err: nil}

	st, err := m.Activate(ActivateOpts{BrowserBin: "/usr/bin/chromium"})
	if err != nil {
		t.Fatalf("Activate err = %v", err)
	}
	if !st.Active || st.BrowserBin != "/usr/bin/chromium" || st.Source != SourceSystem ||
		st.Version != "Chromium 148.0" {
		t.Errorf("status after Activate = %+v", st)
	}
	if !mem.FileExists(stateFilePath) {
		t.Errorf("Activate did not persist state file")
	}
}

func TestActivate_ExplicitPathMissing(t *testing.T) {
	m, mem, _, _ := newTestManager(t)
	_, err := m.Activate(ActivateOpts{BrowserBin: "/no/such/chrome"})
	if !errors.Is(err, ErrInvalidBrowserBin) {
		t.Errorf("err = %v, want ErrInvalidBrowserBin", err)
	}
	if mem.FileExists(stateFilePath) {
		t.Errorf("failed Activate persisted state — should not have")
	}
	if m.Status().Active {
		t.Errorf("failed Activate flipped status active")
	}
}

func TestActivate_ExplicitPathVersionFailure(t *testing.T) {
	m, _, fs, vers := newTestManager(t)
	fs["/usr/bin/chromium"] = true
	vers["/usr/bin/chromium"] = struct {
		version string
		err     error
	}{version: "", err: errors.New("permission denied")}

	_, err := m.Activate(ActivateOpts{BrowserBin: "/usr/bin/chromium"})
	if !errors.Is(err, ErrInvalidBrowserBin) {
		t.Errorf("err = %v, want ErrInvalidBrowserBin (wrap)", err)
	}
}

func TestActivate_AutoPickFirstCandidate(t *testing.T) {
	m, _, fs, vers := newTestManager(t)
	fs["/usr/bin/google-chrome"] = true
	fs["/usr/bin/chromium"] = true
	vers["/usr/bin/google-chrome"] = struct {
		version string
		err     error
	}{version: "Chrome 99", err: nil}
	vers["/usr/bin/chromium"] = struct {
		version string
		err     error
	}{version: "Chromium 148", err: nil}

	st, err := m.Activate(ActivateOpts{})
	if err != nil {
		t.Fatalf("auto-pick Activate err = %v", err)
	}
	if st.BrowserBin != "/usr/bin/google-chrome" {
		t.Errorf("auto-pick chose %q, want first system candidate", st.BrowserBin)
	}
}

func TestActivate_NoCandidatesReturnsNoBrowserFound(t *testing.T) {
	m, _, _, _ := newTestManager(t)
	_, err := m.Activate(ActivateOpts{})
	if !errors.Is(err, ErrNoBrowserFound) {
		t.Errorf("err = %v, want ErrNoBrowserFound", err)
	}
	if m.Status().Active {
		t.Errorf("Activate w/o candidates flipped status active")
	}
}

func TestActivate_ManagedCachePathInfersSourceManaged(t *testing.T) {
	managedBin := "/cache/rod/browser/chromium-1234/chrome"
	m, _, fs, vers := newTestManager(t)
	fs[managedBin] = true
	vers[managedBin] = struct {
		version string
		err     error
	}{version: "Chromium 137", err: nil}

	st, err := m.Activate(ActivateOpts{BrowserBin: managedBin})
	if err != nil {
		t.Fatalf("Activate err = %v", err)
	}
	if st.Source != SourceManaged {
		t.Errorf("source = %q, want managed (path under cacheRoot)", st.Source)
	}
}

func TestDeactivate_ClearsState(t *testing.T) {
	m, mem, fs, vers := newTestManager(t)
	fs["/usr/bin/chromium"] = true
	vers["/usr/bin/chromium"] = struct {
		version string
		err     error
	}{version: "v1", err: nil}
	if _, err := m.Activate(ActivateOpts{BrowserBin: "/usr/bin/chromium"}); err != nil {
		t.Fatalf("seed Activate: %v", err)
	}
	if !mem.FileExists(stateFilePath) {
		t.Fatalf("seed Activate did not persist")
	}

	if err := m.Deactivate(); err != nil {
		t.Fatalf("Deactivate err = %v", err)
	}
	if m.Status().Active {
		t.Errorf("status active after Deactivate")
	}
	// Clear writes {} — file remains, but loaded state is unset.
	if got, _ := m.store.Load(); got.Source != SourceUnset {
		t.Errorf("store source after Clear = %q, want unset", got.Source)
	}
}

func TestDeactivate_WhileInactiveIsNoOp(t *testing.T) {
	m, _, _, _ := newTestManager(t)
	if err := m.Deactivate(); err != nil {
		t.Errorf("Deactivate while inactive err = %v", err)
	}
}

func TestRestore_LoadsPersistedActivation(t *testing.T) {
	m, _, fs, _ := newTestManager(t)
	// Seed persisted state independent of Activate.
	fs["/usr/bin/chromium"] = true
	_ = m.store.Save(state{
		BrowserBin:  "/usr/bin/chromium",
		Source:      SourceSystem,
		Version:     "Chromium 148",
		ActivatedAt: time.Date(2026, 5, 14, 10, 0, 0, 0, time.UTC),
	})

	if err := m.Restore(); err != nil {
		t.Fatalf("Restore: %v", err)
	}
	st := m.Status()
	if !st.Active || st.BrowserBin != "/usr/bin/chromium" || st.Source != SourceSystem {
		t.Errorf("status after Restore = %+v", st)
	}
}

func TestRestore_DropsStalePersistedPath(t *testing.T) {
	m, mem, _, _ := newTestManager(t)
	// Persisted path doesn't exist on the fakeFS — Restore should
	// detect it, log, and clear.
	_ = m.store.Save(state{
		BrowserBin:  "/gone/forever/chrome",
		Source:      SourceSystem,
		ActivatedAt: time.Now(),
	})

	if err := m.Restore(); err != nil {
		t.Fatalf("Restore: %v", err)
	}
	if m.Status().Active {
		t.Errorf("stale path made manager active")
	}
	// Store should now hold the unset state.
	loaded, _ := m.store.Load()
	if loaded.Source != SourceUnset {
		t.Errorf("after Restore w/ stale path, store source = %q", loaded.Source)
	}
	// And the file should still exist (Clear writes empty, doesn't delete).
	if !mem.FileExists(stateFilePath) {
		t.Errorf("Clear should leave the file in place with {} contents")
	}
}

func TestExport_InactiveReturnsNotActivated(t *testing.T) {
	for _, tc := range []struct {
		name string
		tpl  string
		df   string
		opts ExportOpts
	}{
		{"defaults", "tpl.yaml", "form-1.meta.json", ExportOpts{}},
		{"empty datafile", "tpl.yaml", "", ExportOpts{}},
		{"with options", "tpl.yaml", "form-1.meta.json", ExportOpts{OutputPath: "/tmp/x.pdf", Style: "technical"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m, _, _, _ := newTestManager(t)
			res, err := m.Export(tc.tpl, tc.df, tc.opts)
			if !errors.Is(err, ErrPDFNotActivated) {
				t.Errorf("Export err = %v, want ErrPDFNotActivated", err)
			}
			if res != (Result{}) {
				t.Errorf("Export result = %+v, want zero value", res)
			}
		})
	}
}

func TestNewServiceNilManagerPanics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Errorf("NewService(nil) did not panic")
		}
	}()
	_ = NewService(nil)
}

func TestServiceMirrorsManager(t *testing.T) {
	svc := NewService(NewManager(nil, nil, nil, nil, nil, nil))
	if svc.GetStatus().Active {
		t.Errorf("fresh service status active=true")
	}
	// Auto-pick activation on a real filesystem with no Chrome present
	// should fail with ErrNoBrowserFound. Tests run on a box that
	// might have /usr/bin/chromium installed (dev) or might not (CI),
	// so accept either ErrNoBrowserFound (clean machine) or a success
	// (dev box). The point is the service forwards to the manager
	// without flagging ErrPDFNotActivated anymore.
	_, err := svc.Activate(ActivateOpts{})
	if errors.Is(err, ErrPDFNotActivated) {
		t.Errorf("Activate still returning legacy ErrPDFNotActivated")
	}
	// Deactivate is now idempotent + safe — always returns nil.
	if err := svc.Deactivate(); err != nil {
		t.Errorf("Deactivate err = %v, want nil", err)
	}
}
