package gigot

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

type recordingEmitter struct{ names []string }

func (r *recordingEmitter) Emit(name string, _ any) { r.names = append(r.names, name) }
func (r *recordingEmitter) has(n string) bool {
	for _, x := range r.names {
		if x == n {
			return true
		}
	}
	return false
}

func emitTestService(t *testing.T, handler http.HandlerFunc) (*Service, string) {
	t.Helper()
	ctxDir := t.TempDir()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	m := NewManager(newFakeFS(), WithHTTPClient(srv.Client()))
	cfg := &fakeConfig{baseURL: srv.URL, repoName: "r", context: ctxDir}
	creds := &fakeCreds{store: map[string]string{"default.json:gigot:r": "tok"}}
	profile := &fakeProfile{name: "default.json"}
	return NewService(m, creds, profile, cfg, nil), ctxDir
}

// servesOneFile returns a handler whose tree carries a single new file.
func servesOneFile() http.HandlerFunc {
	body := []byte("x")
	sha := GitBlobSha(body)
	return func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/repos/r/tree":
			_ = json.NewEncoder(w).Encode(TreeResponse{
				Version: "v1",
				Files:   []TreeEntry{{Path: "templates/one.yaml", Blob: sha}},
			})
		case "/api/repos/r/files/templates/one.yaml":
			_ = json.NewEncoder(w).Encode(FileResponse{
				Path: "templates/one.yaml", ContentB64: base64.StdEncoding.EncodeToString(body), Blob: sha,
			})
		default:
			http.NotFound(w, r)
		}
	}
}

// A pull that wrote a file changed the working set, so it must emit context:reloaded.
func TestService_PullLocal_EmitsWhenFilesChanged(t *testing.T) {
	s, _ := emitTestService(t, servesOneFile())
	em := &recordingEmitter{}
	AttachEmitter(s, em)

	res, err := s.PullLocal()
	if err != nil {
		t.Fatal(err)
	}
	if res.Files == 0 {
		t.Fatalf("expected a written file, got %+v", res)
	}
	if !em.has("context:reloaded") {
		t.Errorf("changed PullLocal must emit context:reloaded, got %v", em.names)
	}
}

// A pull with an empty tree changed nothing, so it must NOT emit.
func TestService_PullLocal_NoEmitWhenNothingChanged(t *testing.T) {
	s, _ := emitTestService(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/repos/r/tree" {
			_ = json.NewEncoder(w).Encode(TreeResponse{Version: "v1"})
		}
	})
	em := &recordingEmitter{}
	AttachEmitter(s, em)

	res, err := s.PullLocal()
	if err != nil {
		t.Fatal(err)
	}
	if res.Files != 0 || res.Deleted != 0 {
		t.Fatalf("expected no change, got %+v", res)
	}
	if em.has("context:reloaded") {
		t.Errorf("unchanged PullLocal must not emit, got %v", em.names)
	}
}

// Reclone wrote fresh files, so it must emit context:reloaded.
func TestService_Reclone_EmitsWhenChanged(t *testing.T) {
	s, _ := emitTestService(t, servesOneFile())
	em := &recordingEmitter{}
	AttachEmitter(s, em)

	res, err := s.Reclone()
	if err != nil {
		t.Fatal(err)
	}
	if res.Files == 0 && res.Deleted == 0 {
		t.Fatalf("expected a change, got %+v", res)
	}
	if !em.has("context:reloaded") {
		t.Errorf("Reclone must emit context:reloaded, got %v", em.names)
	}
}
