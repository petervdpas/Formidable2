package connection

import (
	"io"
	"net/http"
	"strings"
	"testing"
)

func docsFixture(t *testing.T) *DocsServer {
	t.Helper()
	m := NewManager(newMemFS(), nil)
	if _, err := m.ImportSpec("crm", []byte(specV3JSON)); err != nil {
		t.Fatal(err)
	}
	d, err := NewDocsServer(m, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = d.Close() })
	return d
}

func get(t *testing.T, url string) (int, string, http.Header) {
	t.Helper()
	resp, err := http.Get(url)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	return resp.StatusCode, string(body), resp.Header
}

func TestDocsServer_ServesTheShellForASpec(t *testing.T) {
	d := docsFixture(t)
	url := d.URLFor("crm.json")
	if url == "" {
		t.Fatal("URLFor returned nothing")
	}

	status, body, hdr := get(t, url)
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}
	if !strings.Contains(body, "swagger-ui") {
		t.Errorf("shell does not mount swagger-ui:\n%s", body)
	}
	if !strings.Contains(hdr.Get("Content-Type"), "text/html") {
		t.Errorf("Content-Type = %q, want html", hdr.Get("Content-Type"))
	}
}

func TestDocsServer_ShellPointsAtASameOriginSpecURL(t *testing.T) {
	// The whole reason this server exists: swagger-ui's resolver fetches the
	// document, and that fetch has to succeed for an operation to expand.
	d := docsFixture(t)
	_, body, _ := get(t, d.URLFor("crm.json"))
	if !strings.Contains(body, `"/spec/crm.json"`) {
		t.Fatalf("shell does not name a same-origin spec URL:\n%s", body)
	}
}

func TestDocsServer_ServesTheDocumentAsJSON(t *testing.T) {
	d := docsFixture(t)
	status, body, hdr := get(t, d.BaseURL()+"/spec/crm.json")
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}
	if !strings.Contains(hdr.Get("Content-Type"), "application/json") {
		t.Errorf("Content-Type = %q, want json", hdr.Get("Content-Type"))
	}
	if !strings.Contains(body, "listCustomers") {
		t.Errorf("body is not the document:\n%s", body)
	}
}

func TestDocsServer_ServesTheVendoredAssets(t *testing.T) {
	d := docsFixture(t)
	for name, wantType := range map[string]string{
		"swagger-ui.css":                  "text/css",
		"swagger-ui-bundle.js":            "javascript",
		"swagger-ui-standalone-preset.js": "javascript",
	} {
		status, body, hdr := get(t, d.BaseURL()+"/docs/"+name)
		if status != http.StatusOK {
			t.Errorf("%s status = %d, want 200", name, status)
			continue
		}
		if len(body) < 1000 {
			t.Errorf("%s is %d bytes, want the real asset", name, len(body))
		}
		if !strings.Contains(hdr.Get("Content-Type"), wantType) {
			t.Errorf("%s Content-Type = %q, want %s", name, hdr.Get("Content-Type"), wantType)
		}
	}
}

func TestDocsServer_RefusesAPathOutsideTheSpecsFolder(t *testing.T) {
	d := docsFixture(t)
	for _, bad := range []string{"/spec/../../secrets.yaml", "/spec/sub/crm.json", "/spec/.hidden"} {
		status, _, _ := get(t, d.BaseURL()+bad)
		if status == http.StatusOK {
			t.Errorf("GET %s = 200, want a refusal", bad)
		}
	}
}

func TestDocsServer_UnknownSpecAndAssetAre404(t *testing.T) {
	d := docsFixture(t)
	if status, _, _ := get(t, d.BaseURL()+"/spec/nope.json"); status != http.StatusNotFound {
		t.Errorf("unknown spec = %d, want 404", status)
	}
	if status, _, _ := get(t, d.BaseURL()+"/docs/nope.js"); status != http.StatusNotFound {
		t.Errorf("unknown asset = %d, want 404", status)
	}
}

func TestDocsServer_BindsLoopbackOnly(t *testing.T) {
	// The documents are a local authoring aid, not something to publish onto
	// whatever network the machine happens to be on.
	d := docsFixture(t)
	if !strings.HasPrefix(d.Addr(), "127.0.0.1:") {
		t.Fatalf("Addr = %q, want a loopback bind", d.Addr())
	}
}

func TestDocsServer_ClosedServerHandsOutNoURL(t *testing.T) {
	d := docsFixture(t)
	if err := d.Close(); err != nil {
		t.Fatal(err)
	}
	if got := d.URLFor("crm.json"); got != "" {
		t.Fatalf("URLFor after Close = %q, want empty", got)
	}
	// Closing twice is what a shutdown path does when something else already
	// closed it, so it must not panic or error.
	if err := d.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}
}

func TestDocsServer_NilIsSafe(t *testing.T) {
	var d *DocsServer
	if d.URLFor("crm.json") != "" || d.Addr() != "" || d.BaseURL() != "" {
		t.Fatal("a nil server must answer empty rather than panic")
	}
	if err := d.Close(); err != nil {
		t.Fatalf("Close on nil: %v", err)
	}
}

func TestDocsServer_AvoidsAnExcludedPort(t *testing.T) {
	// Squatting the wiki port while the wiki is off would block it starting
	// later, which is the same guard the PDF asset server carries.
	m := NewManager(newMemFS(), nil)
	first, err := NewDocsServer(m, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()

	taken := portOf(t, first.Addr())
	second, err := NewDocsServer(m, nil, taken)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()

	if portOf(t, second.Addr()) == taken {
		t.Fatalf("second server bound the excluded port %d", taken)
	}
}

func portOf(t *testing.T, addr string) int {
	t.Helper()
	idx := strings.LastIndex(addr, ":")
	if idx < 0 {
		t.Fatalf("addr %q has no port", addr)
	}
	n := 0
	for _, r := range addr[idx+1:] {
		n = n*10 + int(r-'0')
	}
	return n
}

func TestService_DocsURLStartsTheServerOnDemand(t *testing.T) {
	// Most sessions never open this tab, so nothing binds a port until one
	// actually does.
	s, _ := newService(t)
	url, err := s.DocsURL(Connection{ID: "crm-prod", SpecFile: "crm-prod.json"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(url, "http://127.0.0.1:") {
		t.Fatalf("URL = %q, want a loopback URL", url)
	}

	status, body, _ := get(t, url)
	if status != http.StatusOK || !strings.Contains(body, "swagger-ui") {
		t.Fatalf("shell not served: %d\n%s", status, body)
	}
	t.Cleanup(func() { _ = s.CloseDocs() })
}

func TestService_DocsURLNeedsASpec(t *testing.T) {
	s, _ := newService(t)
	t.Cleanup(func() { _ = s.CloseDocs() })
	if _, err := s.DocsURL(Connection{ID: "crm-prod"}); err == nil {
		t.Fatal("want an error when the client names no document")
	}
}
