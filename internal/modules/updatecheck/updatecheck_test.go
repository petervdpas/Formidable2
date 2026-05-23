package updatecheck

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCompareVersions(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"2.4.9", "2.4.8", 1},
		{"2.4.8", "2.4.9", -1},
		{"2.4.8", "2.4.8", 0},
		{"v2.4.8", "2.4.8", 0},
		{"2.4.8-dirty", "2.4.8", 0},
		{"v2.4.8-dirty", "2.4.8", 0},
		{"2.5.0", "2.4.99", 1},
		{"3.0.0", "2.99.99", 1},
		{"2.4", "2.4.0", 0},
		{"2.4.1", "2.4", 1},
		{"10.0.0", "9.9.9", 1},
		{"", "2.4.8", -1},
		{"2.4.8", "", 1},
	}
	for _, c := range cases {
		if got := compareVersions(c.a, c.b); got != c.want {
			t.Errorf("compareVersions(%q,%q) = %d, want %d", c.a, c.b, got, c.want)
		}
	}
}

func TestVersionCore(t *testing.T) {
	cases := map[string]string{
		"v2.4.8-dirty": "2.4.8",
		"2.4.8":        "2.4.8",
		"v2.4":         "2.4",
		"weird":        "",
		"":             "",
		"v1.2.3+meta":  "1.2.3",
	}
	for in, want := range cases {
		if got := versionCore(in); got != want {
			t.Errorf("versionCore(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestIsDevVersion(t *testing.T) {
	for _, v := range []string{"0.1.0", "v0.1.0", "weird", ""} {
		if !isDevVersion(v) {
			t.Errorf("isDevVersion(%q) = false, want true", v)
		}
	}
	if isDevVersion("2.4.8") {
		t.Error("isDevVersion(2.4.8) = true, want false")
	}
}

// serve spins up a throwaway endpoint returning the given status/body.
func serve(t *testing.T, status int, body string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv
}

func newMgrAt(current, endpoint string) *Manager {
	m := NewManager(current, nil) // nil enabled => always on
	m.endpoint = endpoint
	return m
}

func TestRefresh_UpdateAvailable(t *testing.T) {
	srv := serve(t, 200, `{"version":"2.4.9","url":"https://example.test/r/2.4.9"}`)
	m := newMgrAt("2.4.8", srv.URL)

	st, err := m.Refresh(context.Background())
	if err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	if !st.Checked {
		t.Error("Checked = false, want true")
	}
	if !st.UpdateAvailable {
		t.Error("UpdateAvailable = false, want true")
	}
	if st.Latest != "2.4.9" || st.Current != "2.4.8" {
		t.Errorf("got Latest=%q Current=%q", st.Latest, st.Current)
	}
	if st.URL != "https://example.test/r/2.4.9" {
		t.Errorf("URL = %q", st.URL)
	}
	// Cached for the next reader.
	if got := m.GetStatus(); got != st {
		t.Errorf("GetStatus mismatch: %+v vs %+v", got, st)
	}
}

func TestRefresh_UpToDate(t *testing.T) {
	srv := serve(t, 200, `{"version":"2.4.8"}`)
	m := newMgrAt("2.4.8", srv.URL)
	st, err := m.Refresh(context.Background())
	if err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	if !st.Checked {
		t.Error("Checked = false, want true")
	}
	if st.UpdateAvailable {
		t.Error("UpdateAvailable = true, want false (equal versions)")
	}
}

func TestRefresh_DirtyCurrentSameCore(t *testing.T) {
	srv := serve(t, 200, `{"version":"2.4.8"}`)
	m := newMgrAt("2.4.8-dirty", srv.URL)
	st, _ := m.Refresh(context.Background())
	if st.UpdateAvailable {
		t.Error("dirty build of same version must not report an update")
	}
}

func TestRefresh_DevVersionNeverNags(t *testing.T) {
	srv := serve(t, 200, `{"version":"2.4.9"}`)
	m := newMgrAt("0.1.0", srv.URL)
	st, _ := m.Refresh(context.Background())
	if st.UpdateAvailable {
		t.Error("untagged dev build (0.1.0) must not report an update")
	}
	if !st.Checked {
		t.Error("Checked should still be true even when we suppress the nag")
	}
}

func TestRefresh_Non200_SilentlyUnchecked(t *testing.T) {
	srv := serve(t, 502, `{"error":"github responded 403"}`)
	m := newMgrAt("2.4.8", srv.URL)
	st, err := m.Refresh(context.Background())
	if err == nil {
		t.Error("expected an error for non-200")
	}
	if st.Checked {
		t.Error("Checked = true, want false on non-200")
	}
}

func TestRefresh_MalformedJSON(t *testing.T) {
	srv := serve(t, 200, `{not json`)
	m := newMgrAt("2.4.8", srv.URL)
	st, err := m.Refresh(context.Background())
	if err == nil {
		t.Error("expected a parse error")
	}
	if st.Checked {
		t.Error("Checked = true, want false on malformed JSON")
	}
}

func TestRefresh_EmptyVersion(t *testing.T) {
	srv := serve(t, 200, `{"version":"  "}`)
	m := newMgrAt("2.4.8", srv.URL)
	_, err := m.Refresh(context.Background())
	if err == nil {
		t.Error("expected an error when version is blank")
	}
}

func TestRefresh_NetworkDown_SilentFail(t *testing.T) {
	// Closed server: connection refused. Mirrors "net down / site
	// unavailable" - must fail silently with Checked=false.
	srv := serve(t, 200, `{"version":"2.4.9"}`)
	url := srv.URL
	srv.Close()

	m := newMgrAt("2.4.8", url)
	st, err := m.Refresh(context.Background())
	if err == nil {
		t.Error("expected a transport error when the site is unreachable")
	}
	if st.Checked || st.UpdateAvailable {
		t.Errorf("unreachable site must leave status blank: %+v", st)
	}
}

func TestRefresh_DisabledSkipsNetwork(t *testing.T) {
	hit := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hit = true
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`{"version":"9.9.9"}`))
	}))
	t.Cleanup(srv.Close)

	m := NewManager("2.4.8", func() bool { return false })
	m.endpoint = srv.URL

	st, err := m.Refresh(context.Background())
	if err != nil {
		t.Fatalf("disabled Refresh should not error: %v", err)
	}
	if hit {
		t.Error("disabled probe must not make a network call")
	}
	if st.Checked || st.UpdateAvailable {
		t.Errorf("disabled probe must report nothing: %+v", st)
	}
}

func TestRefresh_ToggleOnAfterOff(t *testing.T) {
	srv := serve(t, 200, `{"version":"2.4.9","url":"u"}`)
	on := false
	m := NewManager("2.4.8", func() bool { return on })
	m.endpoint = srv.URL

	if st, _ := m.Refresh(context.Background()); st.UpdateAvailable {
		t.Fatal("should be silent while off")
	}
	on = true
	st, _ := m.Refresh(context.Background())
	if !st.UpdateAvailable {
		t.Error("flipping the toggle on should let the next probe run")
	}
}

func TestService_CheckNow_SwallowsError(t *testing.T) {
	srv := serve(t, 500, `boom`)
	m := newMgrAt("2.4.8", srv.URL)
	svc := NewService(m, nil)
	st := svc.CheckNow() // must not panic or surface the error
	if st.Checked {
		t.Error("CheckNow on failure should yield Checked=false")
	}
}

func TestService_OpenLatest(t *testing.T) {
	srv := serve(t, 200, `{"version":"2.4.9","url":"https://example.test/rel"}`)
	m := newMgrAt("2.4.8", srv.URL)
	m.Refresh(context.Background())

	var opened string
	svc := NewService(m, func(u string) error { opened = u; return nil })
	if err := svc.OpenLatest(); err != nil {
		t.Fatalf("OpenLatest: %v", err)
	}
	if opened != "https://example.test/rel" {
		t.Errorf("opened %q", opened)
	}
}

func TestService_OpenLatest_NoURL(t *testing.T) {
	m := newMgrAt("2.4.8", "http://unused.test")
	svc := NewService(m, func(string) error { return nil })
	if err := svc.OpenLatest(); err == nil {
		t.Error("OpenLatest with no known URL should error")
	}
}
