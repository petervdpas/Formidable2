package wiki

import (
	"errors"
	"testing"
)

// stubPortFunc is a tiny constructor used by service tests to feed
// the desired port for a scenario. Returning 0 means "ask the OS".
func stubPortFunc(p int) func() int { return func() int { return p } }

func TestService_Status_Idle(t *testing.T) {
	m := NewManager(nil)
	svc := NewService(m, stubPortFunc(0), nil, nil)
	s := svc.GetServerStatus()
	if s.Running {
		t.Error("expected not running")
	}
}

func TestService_StartUsesConfiguredPort(t *testing.T) {
	m := NewManager(nil)
	svc := NewService(m, stubPortFunc(0), nil, nil)
	if err := svc.StartServer(); err != nil {
		t.Fatalf("start: %v", err)
	}
	defer svc.StopServer()
	if !svc.GetServerStatus().Running {
		t.Error("expected running after StartServer")
	}
	if svc.GetServerStatus().Port == 0 {
		t.Error("expected non-zero bound port")
	}
}

func TestService_StopReleasesServer(t *testing.T) {
	m := NewManager(nil)
	svc := NewService(m, stubPortFunc(0), nil, nil)
	if err := svc.StartServer(); err != nil {
		t.Fatal(err)
	}
	if err := svc.StopServer(); err != nil {
		t.Fatalf("stop: %v", err)
	}
	if svc.GetServerStatus().Running {
		t.Error("expected not running after StopServer")
	}
}

func TestService_OpenInBrowser_DelegatesToOpener(t *testing.T) {
	m := NewManager(nil)
	if err := m.Start(0); err != nil {
		t.Fatal(err)
	}
	defer m.Stop()

	var got string
	open := func(url string) error {
		got = url
		return nil
	}
	svc := NewService(m, stubPortFunc(0), open, nil)
	if err := svc.OpenInBrowser(); err != nil {
		t.Fatalf("open: %v", err)
	}
	want := "http://127.0.0.1:" + intStr(m.Status().Port) + "/"
	if got != want {
		t.Errorf("opener url = %q, want %q", got, want)
	}
}

func TestService_OpenInBrowser_ServerNotRunning(t *testing.T) {
	m := NewManager(nil)
	svc := NewService(m, stubPortFunc(0), func(string) error { return nil }, nil)
	if err := svc.OpenInBrowser(); err == nil {
		t.Error("expected error when server not running")
	}
}

func TestService_OpenInBrowser_PropagatesOpenerError(t *testing.T) {
	m := NewManager(nil)
	if err := m.Start(0); err != nil {
		t.Fatal(err)
	}
	defer m.Stop()
	svc := NewService(m, stubPortFunc(0), func(string) error { return errors.New("nope") }, nil)
	if err := svc.OpenInBrowser(); err == nil {
		t.Error("expected opener error to propagate")
	}
}

func TestService_OpenInternalWiki_DelegatesToWindowOpener(t *testing.T) {
	m := NewManager(nil)
	if err := m.Start(0); err != nil {
		t.Fatal(err)
	}
	defer m.Stop()

	var got string
	winOpen := func(url string) error {
		got = url
		return nil
	}
	svc := NewService(m, stubPortFunc(0), nil, winOpen)
	if err := svc.OpenInternalWiki(); err != nil {
		t.Fatal(err)
	}
	want := "http://127.0.0.1:" + intStr(m.Status().Port) + "/"
	if got != want {
		t.Errorf("window-opener url = %q, want %q", got, want)
	}
}

func TestService_OpenInternalWiki_NoOpenerErrors(t *testing.T) {
	m := NewManager(nil)
	if err := m.Start(0); err != nil {
		t.Fatal(err)
	}
	defer m.Stop()
	svc := NewService(m, stubPortFunc(0), nil, nil) // nil winOpen
	if err := svc.OpenInternalWiki(); err == nil {
		t.Error("expected error when window opener is unset")
	}
}

func TestService_InstallWindowOpener(t *testing.T) {
	// main.go uses InstallWindowOpener to wire the Wails-aware
	// window spawner after construction. Verify it actually
	// updates the field used by OpenInternalWiki.
	m := NewManager(nil)
	if err := m.Start(0); err != nil {
		t.Fatal(err)
	}
	defer m.Stop()
	svc := NewService(m, stubPortFunc(0), nil, nil)
	if err := svc.OpenInternalWiki(); err == nil {
		t.Fatal("expected error before opener is installed")
	}
	var seen string
	InstallWindowOpener(svc, func(url string) error { seen = url; return nil })
	if err := svc.OpenInternalWiki(); err != nil {
		t.Fatal(err)
	}
	if seen == "" {
		t.Error("opener was not invoked")
	}
}

func TestService_InstallWindowOpener_NilSafe(t *testing.T) {
	// nil service and nil function should both be safe — no panics.
	InstallWindowOpener(nil, func(string) error { return nil })

	m := NewManager(nil)
	if err := m.Start(0); err != nil {
		t.Fatal(err)
	}
	defer m.Stop()
	svc := NewService(m, stubPortFunc(0), nil, func(string) error { return nil })
	InstallWindowOpener(svc, nil)
	if err := svc.OpenInternalWiki(); err == nil {
		t.Error("clearing opener should re-disable the action")
	}
}

func TestService_OpenAPIDocsInBrowser(t *testing.T) {
	m := NewManager(nil)
	if err := m.Start(0); err != nil {
		t.Fatal(err)
	}
	defer m.Stop()

	var got string
	svc := NewService(m, stubPortFunc(0), func(url string) error { got = url; return nil }, nil)
	if err := svc.OpenAPIDocsInBrowser(); err != nil {
		t.Fatal(err)
	}
	want := "http://127.0.0.1:" + intStr(m.Status().Port) + "/api/docs/"
	if got != want {
		t.Errorf("opener url = %q, want %q", got, want)
	}
}

func TestService_OpenAPIDocsInBrowser_NotRunning(t *testing.T) {
	m := NewManager(nil)
	svc := NewService(m, stubPortFunc(0), func(string) error { return nil }, nil)
	if err := svc.OpenAPIDocsInBrowser(); err == nil {
		t.Error("expected error when server not running")
	}
}

func TestService_OpenAPIDocsInWindow(t *testing.T) {
	m := NewManager(nil)
	if err := m.Start(0); err != nil {
		t.Fatal(err)
	}
	defer m.Stop()

	var got string
	svc := NewService(m, stubPortFunc(0), nil, func(url string) error { got = url; return nil })
	if err := svc.OpenAPIDocsInWindow(); err != nil {
		t.Fatal(err)
	}
	want := "http://127.0.0.1:" + intStr(m.Status().Port) + "/api/docs/"
	if got != want {
		t.Errorf("window-opener url = %q, want %q", got, want)
	}
}

func TestService_OpenAPIDocsInWindow_NoOpener(t *testing.T) {
	m := NewManager(nil)
	if err := m.Start(0); err != nil {
		t.Fatal(err)
	}
	defer m.Stop()
	svc := NewService(m, stubPortFunc(0), nil, nil)
	if err := svc.OpenAPIDocsInWindow(); err == nil {
		t.Error("expected error when window opener is unset")
	}
}

func TestService_NilManagerSafe(t *testing.T) {
	// Defense-in-depth: NewService should refuse a nil manager rather
	// than panicking deep in a method.
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic on nil manager")
		}
	}()
	_ = NewService(nil, stubPortFunc(0), nil, nil)
}
