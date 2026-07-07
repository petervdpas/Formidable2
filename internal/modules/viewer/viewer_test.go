package viewer

import (
	"archive/zip"
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func makeZip(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, body := range files {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatalf("create %s: %v", name, err)
		}
		if _, err := io.WriteString(w, body); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("close zip: %v", err)
	}
	return buf.Bytes()
}

func get(t *testing.T, h http.Handler, path string) (*http.Response, string) {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	res := rec.Result()
	body, _ := io.ReadAll(res.Body)
	_ = res.Body.Close()
	return res, string(body)
}

func sampleBundle(t *testing.T) *Bundle {
	t.Helper()
	zb := makeZip(t, map[string]string{
		"index.html":         "<h1>HOME</h1>",
		"template-book.html": "<h1>BOOK COLLECTION</h1>",
		"form-book-abc.html": "<h1>ONE BOOK</h1>",
		"deck-talk.html":     "<section>SLIDES</section>",
		"_/css/reveal.css":   ".reveal{}",
		"_/img/logo.png":     "PNGDATA",
	})
	b, err := BundleFromBytes(zb, "sample.zip")
	if err != nil {
		t.Fatalf("BundleFromBytes: %v", err)
	}
	return b
}

func TestBundleServesIndexAtRoot(t *testing.T) {
	b := sampleBundle(t)
	res, body := get(t, b, "/")
	if res.StatusCode != http.StatusOK {
		t.Fatalf("GET / status = %d, want 200", res.StatusCode)
	}
	if !strings.Contains(body, "HOME") {
		t.Fatalf("GET / body = %q, want index.html content", body)
	}
}

func TestBundleServesPages(t *testing.T) {
	b := sampleBundle(t)
	cases := map[string]string{
		"/template-book.html": "BOOK COLLECTION",
		"/form-book-abc.html": "ONE BOOK",
		"/deck-talk.html":     "SLIDES",
		"/_/css/reveal.css":   ".reveal{}",
		"/_/img/logo.png":     "PNGDATA",
	}
	for path, want := range cases {
		res, body := get(t, b, path)
		if res.StatusCode != http.StatusOK {
			t.Errorf("GET %s status = %d, want 200", path, res.StatusCode)
			continue
		}
		if !strings.Contains(body, want) {
			t.Errorf("GET %s body = %q, want %q", path, body, want)
		}
	}
}

func TestBundleMissingReturns404(t *testing.T) {
	b := sampleBundle(t)
	res, _ := get(t, b, "/does-not-exist.html")
	if res.StatusCode != http.StatusNotFound {
		t.Fatalf("GET missing status = %d, want 404", res.StatusCode)
	}
}

func TestBundleHasIndex(t *testing.T) {
	b := sampleBundle(t)
	if !b.HasIndex() {
		t.Fatal("HasIndex = false, want true for a bundle with index.html")
	}

	noIndex, err := BundleFromBytes(makeZip(t, map[string]string{"other.html": "x"}), "n.zip")
	if err != nil {
		t.Fatalf("BundleFromBytes: %v", err)
	}
	if noIndex.HasIndex() {
		t.Fatal("HasIndex = true, want false for a bundle without index.html")
	}
}

func TestBundleName(t *testing.T) {
	b := sampleBundle(t)
	if b.Name() != "sample.zip" {
		t.Fatalf("Name = %q, want sample.zip", b.Name())
	}
}

func TestOpenBundleFromDiskAndClose(t *testing.T) {
	zb := makeZip(t, map[string]string{"index.html": "<h1>DISK</h1>"})
	path := filepath.Join(t.TempDir(), "export.zip")
	if err := os.WriteFile(path, zb, 0o644); err != nil {
		t.Fatalf("write temp zip: %v", err)
	}
	b, err := OpenBundle(path)
	if err != nil {
		t.Fatalf("OpenBundle: %v", err)
	}
	if b.Name() != "export.zip" {
		t.Fatalf("Name = %q, want export.zip", b.Name())
	}
	res, body := get(t, b, "/")
	if res.StatusCode != http.StatusOK || !strings.Contains(body, "DISK") {
		t.Fatalf("GET / = %d %q", res.StatusCode, body)
	}
	if err := b.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
}

func TestBundleFromBytesCloseIsNoop(t *testing.T) {
	if err := sampleBundle(t).Close(); err != nil {
		t.Fatalf("Close on byte bundle = %v, want nil", err)
	}
}

func TestOpenBundleBadZip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bad.zip")
	if err := os.WriteFile(path, []byte("not a zip"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if _, err := OpenBundle(path); err == nil {
		t.Fatal("OpenBundle on garbage = nil error, want failure")
	}
}

func TestServerLandingWhenNoBundle(t *testing.T) {
	s := NewServer()
	res, body := get(t, s, "/")
	if res.StatusCode != http.StatusOK {
		t.Fatalf("landing status = %d, want 200", res.StatusCode)
	}
	if ct := res.Header.Get("Content-Type"); !strings.Contains(ct, "text/html") {
		t.Fatalf("landing content-type = %q, want html", ct)
	}
	if !strings.Contains(body, landingTitle) {
		t.Fatalf("landing body missing title %q: %q", landingTitle, body)
	}
}

func TestServerServesBundleAndSwaps(t *testing.T) {
	s := NewServer()

	first := sampleBundle(t)
	if prev := s.SetBundle(first); prev != nil {
		t.Fatalf("first SetBundle returned prev = %v, want nil", prev)
	}
	if s.Current() != first {
		t.Fatal("Current did not return the set bundle")
	}
	if res, body := get(t, s, "/"); res.StatusCode != http.StatusOK || !strings.Contains(body, "HOME") {
		t.Fatalf("serve first bundle = %d %q", res.StatusCode, body)
	}

	second, err := BundleFromBytes(makeZip(t, map[string]string{"index.html": "<h1>SECOND</h1>"}), "second.zip")
	if err != nil {
		t.Fatalf("BundleFromBytes: %v", err)
	}
	prev := s.SetBundle(second)
	if prev != first {
		t.Fatal("SetBundle did not return the previously-loaded bundle")
	}
	if res, body := get(t, s, "/"); res.StatusCode != http.StatusOK || !strings.Contains(body, "SECOND") {
		t.Fatalf("serve second bundle = %d %q", res.StatusCode, body)
	}
}
