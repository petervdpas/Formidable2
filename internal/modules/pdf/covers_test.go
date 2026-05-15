package pdf

import (
	"errors"
	"strings"
	"testing"
)

func TestEmbeddedCoverNames_ContainsLibrary(t *testing.T) {
	got := EmbeddedCoverNames()
	want := map[string]bool{"classic": true, "banner": true, "corporate": true}
	for w := range want {
		found := false
		for _, n := range got {
			if n == w {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("embedded library missing %q; got %v", w, got)
		}
	}
}

func TestEmbeddedCoverNames_ExcludesSignature(t *testing.T) {
	for _, n := range EmbeddedCoverNames() {
		if n == "signature" {
			t.Errorf("signature should not be in the user-pickable list")
		}
	}
}

func TestEmbeddedCover_Classic_HasPicoloomPlaceholders(t *testing.T) {
	html, err := embeddedCover("classic")
	if err != nil {
		t.Fatalf("embeddedCover(classic) err = %v", err)
	}
	for _, ph := range []string{"{{.Title}}", "{{if .Logo}}", "{{if .Date}}"} {
		if !strings.Contains(html, ph) {
			t.Errorf("classic.html missing placeholder %q", ph)
		}
	}
	if !strings.Contains(html, "data-cover-end") {
		t.Errorf("classic.html missing picoloom data-cover-end sentinel")
	}
}

func TestEmbeddedCover_AllLibraryFilesParseable(t *testing.T) {
	for _, name := range EmbeddedCoverNames() {
		html, err := embeddedCover(name)
		if err != nil {
			t.Errorf("embeddedCover(%q) err = %v", name, err)
			continue
		}
		if !strings.Contains(html, "data-cover-end") {
			t.Errorf("%s.html missing picoloom data-cover-end sentinel", name)
		}
		if !strings.Contains(html, "{{.Title}}") {
			t.Errorf("%s.html missing {{.Title}} placeholder", name)
		}
	}
}

func TestEmbeddedCover_UnknownNameReturnsNotFound(t *testing.T) {
	_, err := embeddedCover("nope-not-real")
	if !errors.Is(err, ErrCoverNotFound) {
		t.Errorf("err = %v, want ErrCoverNotFound", err)
	}
}

func TestEmbeddedCover_EmptyNameReturnsNotFound(t *testing.T) {
	_, err := embeddedCover("")
	if !errors.Is(err, ErrCoverNotFound) {
		t.Errorf("err = %v, want ErrCoverNotFound", err)
	}
}

func TestBundledSignature_LoadsAndContainsPlaceholders(t *testing.T) {
	html, err := bundledSignature()
	if err != nil {
		t.Fatalf("bundledSignature err = %v", err)
	}
	for _, ph := range []string{"{{.Name}}", "{{.Email}}", "signature-block"} {
		if !strings.Contains(html, ph) {
			t.Errorf("signature.html missing %q", ph)
		}
	}
}

func TestResolveCoverTemplateSet_NilCoverReturnsNil(t *testing.T) {
	ts, err := ResolveCoverTemplateSet(nil, "/storage/tpl", newMemFS())
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if ts != nil {
		t.Errorf("ts = %+v, want nil for nil cover", ts)
	}
}

func TestResolveCoverTemplateSet_NeitherFieldSetReturnsNil(t *testing.T) {
	cv := &CoverFM{Title: "T"}
	ts, err := ResolveCoverTemplateSet(cv, "/storage/tpl", newMemFS())
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if ts != nil {
		t.Errorf("ts = %+v, want nil when no Template/TemplatePath set", ts)
	}
}

func TestResolveCoverTemplateSet_EmbeddedTemplate(t *testing.T) {
	cv := &CoverFM{Template: "banner"}
	ts, err := ResolveCoverTemplateSet(cv, "/storage/tpl", newMemFS())
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if ts == nil {
		t.Fatalf("ts = nil, want non-nil")
	}
	if ts.Name != "banner" {
		t.Errorf("Name = %q, want banner", ts.Name)
	}
	if !strings.Contains(ts.Cover, "cover-banner") {
		t.Errorf("Cover did not contain expected design marker")
	}
	if !strings.Contains(ts.Signature, "signature-block") {
		t.Errorf("Signature missing — bundled signature must always be set")
	}
}

func TestResolveCoverTemplateSet_UnknownEmbeddedTemplate(t *testing.T) {
	cv := &CoverFM{Template: "no-such-design"}
	_, err := ResolveCoverTemplateSet(cv, "/storage/tpl", newMemFS())
	if !errors.Is(err, ErrCoverNotFound) {
		t.Errorf("err = %v, want ErrCoverNotFound", err)
	}
}

func TestResolveCoverTemplateSet_TemplatePathRelative(t *testing.T) {
	fs := newMemFS()
	fs.files["/storage/tpl/assets/my-cover.html"] = "<section class=\"cover\">CUSTOM {{.Title}}</section><span data-cover-end></span>"
	cv := &CoverFM{TemplatePath: "assets/my-cover.html"}

	ts, err := ResolveCoverTemplateSet(cv, "/storage/tpl", fs)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if ts == nil {
		t.Fatalf("ts = nil")
	}
	if !strings.Contains(ts.Cover, "CUSTOM") {
		t.Errorf("Cover did not load user file content; got %q", ts.Cover)
	}
}

func TestResolveCoverTemplateSet_TemplatePathAbsolute(t *testing.T) {
	fs := newMemFS()
	fs.files["/abs/path/cover.html"] = "ABS"
	cv := &CoverFM{TemplatePath: "/abs/path/cover.html"}

	ts, err := ResolveCoverTemplateSet(cv, "/storage/tpl", fs)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if ts == nil || ts.Cover != "ABS" {
		t.Errorf("absolute path not honored verbatim; got %+v", ts)
	}
}

func TestResolveCoverTemplateSet_TemplatePathOverridesTemplate(t *testing.T) {
	// When both are set, TemplatePath wins.
	fs := newMemFS()
	fs.files["/storage/tpl/custom.html"] = "USER-CUSTOM"
	cv := &CoverFM{TemplatePath: "custom.html", Template: "banner"}

	ts, err := ResolveCoverTemplateSet(cv, "/storage/tpl", fs)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if ts == nil || !strings.Contains(ts.Cover, "USER-CUSTOM") {
		t.Errorf("TemplatePath should have won; got %+v", ts)
	}
}

func TestResolveCoverTemplateSet_TemplatePathMissingFile(t *testing.T) {
	fs := newMemFS()
	cv := &CoverFM{TemplatePath: "missing.html"}
	_, err := ResolveCoverTemplateSet(cv, "/storage/tpl", fs)
	if !errors.Is(err, ErrCoverPathInvalid) {
		t.Errorf("err = %v, want ErrCoverPathInvalid", err)
	}
}
