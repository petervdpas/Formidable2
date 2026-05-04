package handlers

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/petervdpas/formidable2/internal/modules/config"
	"github.com/petervdpas/formidable2/internal/modules/sfr"
	"github.com/petervdpas/formidable2/internal/modules/storage"
	"github.com/petervdpas/formidable2/internal/modules/system"
	"github.com/petervdpas/formidable2/internal/modules/template"
)

// newTestStack builds the smallest viable {system, config, template,
// storage} cluster rooted at a temp dir. Used to drive the handler in
// isolation from main.go.
func newTestStack(t *testing.T) (Deps, *system.Manager) {
	t.Helper()
	root := t.TempDir()
	log := slog.New(slog.NewTextHandler(nil, &slog.HandlerOptions{Level: slog.LevelError + 1}))
	_ = log

	sys := system.NewManager(root, nil)
	cfgM, err := config.NewManager(sys, nil)
	if err != nil {
		t.Fatalf("config: %v", err)
	}
	// Use root-relative context so seed scenarios don't fall under
	// ./Examples (which would require additional fs setup).
	if _, err := cfgM.UpdateUserConfig(map[string]any{"context_folder": "./"}); err != nil {
		t.Fatalf("override context: %v", err)
	}

	templatesPath, _ := cfgM.GetContextTemplatesPath()
	storagePath, _ := cfgM.GetContextStoragePath()

	tplM := template.NewManager(sys, templatesPath, nil)
	sfrM := sfr.NewManager(sys, nil)
	stoM := storage.NewManager(sys, sfrM, tplM, storagePath, nil)

	return Deps{
		Template: template.NewService(tplM, nil),
		Storage:  storage.NewService(stoM),
		Config:   config.NewService(cfgM),
		AppName:  "Formidable2-Test",
		Version:  "0.0.0",
	}, sys
}

// ── Happy paths ──────────────────────────────────────────────────────

func TestIndex_RendersWithoutTemplates(t *testing.T) {
	d, _ := newTestStack(t)
	h := New(d)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, "No templates yet") {
		t.Errorf("expected empty-state, got: %s", body)
	}
	if !strings.Contains(body, "Formidable2-Test") {
		t.Errorf("brand missing: %s", body)
	}
}

func TestIndex_ListsExistingTemplates(t *testing.T) {
	d, sys := newTestStack(t)
	_ = sys.SaveFile("templates/basic.yaml", "name: Basic Form\nfilename: basic.yaml\nfields: []\n")

	h := New(d)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, "Basic Form") {
		t.Errorf("display name missing: %s", body)
	}
	if !strings.Contains(body, "basic.yaml") {
		t.Errorf("filename missing: %s", body)
	}
	if !strings.Contains(body, "0 forms") {
		t.Errorf("form count badge missing: %s", body)
	}
}

func TestIndex_FallsBackWhenTemplateNameEmpty(t *testing.T) {
	d, sys := newTestStack(t)
	_ = sys.SaveFile("templates/x.yaml", "name: \"\"\nfilename: x.yaml\nfields: []\n")

	h := New(d)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, ">x<") && !strings.Contains(body, "x.yaml") {
		t.Errorf("expected fallback to filename, got: %s", body)
	}
}

// ── Asset serving ────────────────────────────────────────────────────

func TestAssets_ServesEmbeddedCSS(t *testing.T) {
	d, _ := newTestStack(t)
	h := New(d)

	req := httptest.NewRequest(http.MethodGet, "/assets/app.css", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), ":root") {
		t.Errorf("expected CSS body, got: %s", rec.Body.String())
	}
}

func TestAssets_MissingFileReturns404(t *testing.T) {
	d, _ := newTestStack(t)
	h := New(d)

	req := httptest.NewRequest(http.MethodGet, "/assets/nope.css", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rec.Code)
	}
}

// ── Unhappy paths ────────────────────────────────────────────────────

func TestUnknownRoute_Returns404(t *testing.T) {
	d, _ := newTestStack(t)
	h := New(d)

	req := httptest.NewRequest(http.MethodGet, "/totally/unknown", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rec.Code)
	}
}

func TestPostToIndex_NotAllowed(t *testing.T) {
	d, _ := newTestStack(t)
	h := New(d)

	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(""))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code == http.StatusOK {
		t.Errorf("POST / should not return 200, got %d", rec.Code)
	}
}

func TestFavicon_Returns204(t *testing.T) {
	d, _ := newTestStack(t)
	h := New(d)

	req := httptest.NewRequest(http.MethodGet, "/favicon.ico", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Errorf("status = %d, want 204", rec.Code)
	}
}
