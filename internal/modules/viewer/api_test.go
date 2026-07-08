package viewer

import (
	"encoding/base64"
	"net/http"
	"strings"
	"testing"

	"github.com/petervdpas/formidable2/internal/modules/bundle"
	"github.com/petervdpas/formidable2/internal/modules/datadb"
)

func dataImage(t *testing.T) []byte {
	t.Helper()
	img, err := datadb.Build([]datadb.Record{
		{Template: "t.yaml", GUID: "g1", Title: "One", Payload: map[string]any{"a": 1}, Text: "one alpha"},
	})
	if err != nil {
		t.Fatalf("datadb.Build: %v", err)
	}
	return img
}

// zipWithData is an export zip carrying a data image alongside the home page.
func zipWithData(t *testing.T) []byte {
	t.Helper()
	return makeZip(t, map[string]string{
		"index.html": "<h1>HOME</h1>",
		dataEntry:    string(dataImage(t)),
	})
}

func TestBundleServesAPIWhenEnabled(t *testing.T) {
	b, err := BundleFromBytes(zipWithData(t), "d.bundle")
	if err != nil {
		t.Fatalf("BundleFromBytes: %v", err)
	}
	if !b.HasData() {
		t.Fatal("bundle with a data image should report HasData")
	}
	s := NewServer()
	s.SetBundle(b)
	s.SetAPIEnabled(true)

	if res, body := get(t, s, "/api/templates"); res.StatusCode != http.StatusOK || !strings.Contains(body, "t.yaml") {
		t.Fatalf("GET /api/templates = %d %q", res.StatusCode, body)
	}
	// HTML is still served for non-/api paths.
	if res, body := get(t, s, "/"); res.StatusCode != http.StatusOK || !strings.Contains(body, "HOME") {
		t.Fatalf("GET / = %d %q", res.StatusCode, body)
	}
}

func TestServerAPIGatedOffIs404(t *testing.T) {
	b, err := BundleFromBytes(zipWithData(t), "d.bundle")
	if err != nil {
		t.Fatalf("BundleFromBytes: %v", err)
	}
	s := NewServer()
	s.SetBundle(b) // ServeAPI not enabled
	if res, _ := get(t, s, "/api/templates"); res.StatusCode != http.StatusNotFound {
		t.Fatalf("API off should 404, got %d", res.StatusCode)
	}
}

func TestServerAPINoDataImageIs404(t *testing.T) {
	b, err := BundleFromBytes(makeZip(t, map[string]string{"index.html": "x"}), "d.bundle")
	if err != nil {
		t.Fatalf("BundleFromBytes: %v", err)
	}
	if b.HasData() {
		t.Fatal("bundle without a data image should not report HasData")
	}
	s := NewServer()
	s.SetBundle(b)
	s.SetAPIEnabled(true)
	if res, _ := get(t, s, "/api/templates"); res.StatusCode != http.StatusNotFound {
		t.Fatalf("no data image should 404, got %d", res.StatusCode)
	}
}

func TestServiceAPIStatus(t *testing.T) {
	s := newService(t)
	packed, err := bundle.Pack(bundle.Manifest{Title: "P"}, zipWithData(t), "")
	if err != nil {
		t.Fatalf("pack: %v", err)
	}
	if _, err := s.OpenBytes("p.bundle", base64.StdEncoding.EncodeToString(packed), ""); err != nil {
		t.Fatalf("OpenBytes: %v", err)
	}

	// Data is present, but the API is opt-in and off by default.
	if st := s.APIStatus(); st.Enabled || !st.Available || len(st.URLs) != 0 {
		t.Fatalf("default status = %+v, want disabled but available with no urls", st)
	}

	cfg := s.GetConfig()
	cfg.ServeAPI = true
	if _, err := s.SetConfig(cfg); err != nil {
		t.Fatalf("SetConfig: %v", err)
	}
	st := s.APIStatus()
	if !st.Enabled || !st.Available || len(st.URLs) == 0 {
		t.Fatalf("enabled status = %+v, want enabled + available + urls", st)
	}
}
