package wiki

import (
	"archive/zip"
	"bytes"
	"context"
	"io"
	"strings"
	"testing"

	"github.com/petervdpas/formidable2/internal/modules/bundle"
	"github.com/petervdpas/formidable2/internal/modules/dataprovider"
	tpl "github.com/petervdpas/formidable2/internal/modules/template"
)

func unzipMap(t *testing.T, b []byte) map[string][]byte {
	t.Helper()
	zr, err := zip.NewReader(bytes.NewReader(b), int64(len(b)))
	if err != nil {
		t.Fatalf("open zip: %v", err)
	}
	out := map[string][]byte{}
	for _, f := range zr.File {
		rc, err := f.Open()
		if err != nil {
			t.Fatalf("open %s: %v", f.Name, err)
		}
		data, _ := io.ReadAll(rc)
		rc.Close()
		out[f.Name] = data
	}
	return out
}

func exportProvider() *stubProvider {
	return &stubProvider{
		templates: []dataprovider.TemplateSummary{
			{Stem: "basic", Filename: "basic.yaml", Name: "Basic Form"},
		},
		forms: map[string][]dataprovider.FormSummary{
			"basic.yaml": {
				{Template: "basic.yaml", Filename: "x.meta.json", Title: "X"},
				{Template: "basic.yaml", Filename: "y.meta.json", Title: "Y"},
			},
		},
		render: func(_, datafile string) (*dataprovider.RenderedPage, error) {
			return &dataprovider.RenderedPage{
				Title: "Title-" + datafile,
				HTML:  `<p>body ` + datafile + `</p><img src="/storage/basic/images/logo.png">`,
			}, nil
		},
	}
}

func TestExportBundle_DocumentTemplate(t *testing.T) {
	h := NewHandler(exportProvider(), newStubStorage(), &stubExpressioner{})

	res, err := h.ExportBundle(context.Background(), map[string][]string{"basic.yaml": nil})
	if err != nil {
		t.Fatalf("ExportBundle: %v", err)
	}
	if len(res.Skipped) != 0 {
		t.Errorf("nothing should be skipped, got %v", res.Skipped)
	}
	files := unzipMap(t, res.Zip)

	for _, want := range []string{
		"index.html", "template-basic.html",
		"form-basic-x-meta-json.html", "form-basic-y-meta-json.html",
		"_/css/base.css", "_/css/header.css", "_/css/facets.css",
		"_/css/formidable-prose.css", "_/js/filter.js", "_/js/crumbs.js",
		"_/js/mermaid.min.js", "_/img/logo.png",
	} {
		if _, ok := files[want]; !ok {
			t.Errorf("missing zip entry %q", want)
		}
	}

	// No absolute server URLs may leak into any page.
	for name, data := range files {
		if !strings.HasSuffix(name, ".html") {
			continue
		}
		s := string(data)
		if strings.Contains(s, "/_/") {
			t.Errorf("%s: leftover /_/ asset URL", name)
		}
		if strings.Contains(s, "/template/") {
			t.Errorf("%s: leftover /template/ link", name)
		}
		if strings.Contains(s, "/storage/") {
			t.Errorf("%s: leftover /storage/ URL", name)
		}
	}

	if !strings.Contains(string(files["index.html"]), "template-basic.html") {
		t.Errorf("index.html should link to the collection page")
	}
	if !strings.Contains(string(files["template-basic.html"]), "form-basic-x-meta-json.html") {
		t.Errorf("collection page should link to a record page")
	}
	if !strings.Contains(string(files["form-basic-x-meta-json.html"]), "data:image/png;base64,") {
		t.Errorf("record image should be inlined as a data URI")
	}

	hdr := string(files["_/css/header.css"])
	if strings.Contains(hdr, "/_/img/logo.png") {
		t.Errorf("header.css logo url should not stay absolute")
	}
	if !strings.Contains(hdr, "../img/logo.png") {
		t.Errorf("header.css logo url should be rewritten to ../img/logo.png")
	}
}

func TestExportBundle_ExportsSelectedDecks(t *testing.T) {
	sp := exportProvider()
	sp.templates = append(sp.templates,
		dataprovider.TemplateSummary{Stem: "talk", Filename: "talk.yaml", Name: "Talk 2026"})
	h := NewHandler(sp, newStubStorage(), &stubExpressioner{})
	h.SetTemplates(&stubTemplates{byName: map[string]*tpl.Template{"talk.yaml": {Presentation: true}}})
	h.SetDecks(twoDecks())

	// Pick the document plus only the "intro" deck of the presentation.
	res, err := h.ExportBundle(context.Background(), map[string][]string{
		"basic.yaml": nil,
		"talk.yaml":  {"intro"},
	})
	if err != nil {
		t.Fatalf("ExportBundle: %v", err)
	}
	if len(res.Skipped) != 0 {
		t.Errorf("nothing should be skipped, got %v", res.Skipped)
	}
	files := unzipMap(t, res.Zip)

	if _, ok := files["deck-talk-intro.html"]; !ok {
		t.Errorf("selected deck page missing")
	}
	if _, ok := files["deck-talk-deep.html"]; ok {
		t.Errorf("unselected deck should not be exported")
	}
	// Deck assets pulled in only because a deck was exported.
	for _, a := range []string{"_/js/reveal.js", "_/css/reveal.css", "_/css/deck.css", "_/katex/katex.min.css"} {
		if _, ok := files[a]; !ok {
			t.Errorf("deck asset %q missing", a)
		}
	}
	// The document still exports and the index links the deck as a presentation.
	if _, ok := files["template-basic.html"]; !ok {
		t.Errorf("document template should still be exported")
	}
	idx := string(files["index.html"])
	if !strings.Contains(idx, "deck-talk-intro.html") {
		t.Errorf("index should link the exported deck page")
	}
	// Deck page must not leak the ?v= cache-buster or absolute /_/ asset URLs.
	deck := string(files["deck-talk-intro.html"])
	if strings.Contains(deck, "?v=") {
		t.Errorf("deck page should have the ?v= cache-buster stripped")
	}
	if strings.Contains(deck, "/_/") {
		t.Errorf("deck page should not reference absolute /_/ assets")
	}
}

func TestExportBundle_DocOnlyOmitsDeckAssets(t *testing.T) {
	h := NewHandler(exportProvider(), newStubStorage(), &stubExpressioner{})
	res, err := h.ExportBundle(context.Background(), map[string][]string{"basic.yaml": nil})
	if err != nil {
		t.Fatalf("ExportBundle: %v", err)
	}
	files := unzipMap(t, res.Zip)
	if _, ok := files["_/js/reveal.js"]; ok {
		t.Errorf("a document-only bundle should not carry reveal.js")
	}
}

// twoDocProvider serves two document templates so the closure can pull the
// second into a bundle that only selected the first.
func twoDocProvider() *stubProvider {
	return &stubProvider{
		templates: []dataprovider.TemplateSummary{
			{Stem: "aanpak", Filename: "aanpak.yaml", Name: "Aanpak"},
			{Stem: "controls", Filename: "controls.yaml", Name: "Controls"},
		},
		forms: map[string][]dataprovider.FormSummary{
			"aanpak.yaml":   {{Template: "aanpak.yaml", Filename: "a.meta.json", Title: "A"}},
			"controls.yaml": {{Template: "controls.yaml", Filename: "c.meta.json", Title: "C"}},
		},
		render: func(_, datafile string) (*dataprovider.RenderedPage, error) {
			return &dataprovider.RenderedPage{Title: "T-" + datafile, HTML: `<p>body ` + datafile + `</p>`}, nil
		},
	}
}

// Selecting only aanpak still bundles controls, because aanpak links to it. The
// zip must open self-contained: the related template's collection and record
// pages are present even though the caller never picked it.
func TestExportBundle_PullsInRelatedTemplate(t *testing.T) {
	h := NewHandler(twoDocProvider(), newStubStorage(), &stubExpressioner{})
	h.SetDependencyGraph(newFakeGraph(map[string][]string{"aanpak.yaml": {"controls.yaml"}}))

	res, err := h.ExportBundle(context.Background(), map[string][]string{"aanpak.yaml": nil})
	if err != nil {
		t.Fatalf("ExportBundle: %v", err)
	}
	files := unzipMap(t, res.Zip)
	for _, want := range []string{
		"template-aanpak.html", "form-aanpak-a-meta-json.html",
		"template-controls.html", "form-controls-c-meta-json.html",
	} {
		if _, ok := files[want]; !ok {
			t.Errorf("missing zip entry %q (closure should have added controls)", want)
		}
	}
	if !strings.Contains(string(files["index.html"]), "template-controls.html") {
		t.Errorf("index should list the auto-included related template")
	}
}

// With no dependency graph wired, the export includes exactly what was selected
// (identity), so behavior is unchanged for callers that opt out.
func TestExportBundle_NoGraphNoAutoInclude(t *testing.T) {
	h := NewHandler(twoDocProvider(), newStubStorage(), &stubExpressioner{})
	res, err := h.ExportBundle(context.Background(), map[string][]string{"aanpak.yaml": nil})
	if err != nil {
		t.Fatalf("ExportBundle: %v", err)
	}
	files := unzipMap(t, res.Zip)
	if _, ok := files["template-controls.html"]; ok {
		t.Errorf("controls must not appear without a dependency graph")
	}
}

func TestExportPack_EncryptedRoundTrips(t *testing.T) {
	h := NewHandler(exportProvider(), newStubStorage(), &stubExpressioner{})
	meta := bundle.Manifest{Title: "Basic Pack", Author: "Peter", Kind: "wiki"}

	packed, skipped, err := h.ExportPack(context.Background(), map[string][]string{"basic.yaml": nil}, "s3cret", meta)
	if err != nil {
		t.Fatalf("ExportPack: %v", err)
	}
	if len(skipped) != 0 {
		t.Errorf("nothing should be skipped, got %v", skipped)
	}

	man, err := bundle.ReadManifest(packed)
	if err != nil {
		t.Fatalf("ReadManifest: %v", err)
	}
	if !man.Encrypted || man.Title != "Basic Pack" || man.Author != "Peter" {
		t.Fatalf("manifest wrong: %+v", man)
	}

	if _, err := bundle.Unpack(packed, "wrong"); err == nil {
		t.Error("wrong password must not unpack")
	}

	zipBytes, err := bundle.Unpack(packed, "s3cret")
	if err != nil {
		t.Fatalf("Unpack: %v", err)
	}
	files := unzipMap(t, zipBytes)
	for _, want := range []string{"index.html", "template-basic.html", "form-basic-x-meta-json.html"} {
		if _, ok := files[want]; !ok {
			t.Errorf("decrypted bundle missing %q", want)
		}
	}
}

func TestExportPack_PlainIsBrandedNotRawZip(t *testing.T) {
	h := NewHandler(exportProvider(), newStubStorage(), &stubExpressioner{})

	packed, _, err := h.ExportPack(context.Background(), map[string][]string{"basic.yaml": nil}, "", bundle.Manifest{Title: "Open Pack"})
	if err != nil {
		t.Fatalf("ExportPack: %v", err)
	}

	// A plain bundle leads with the brand marker, not the zip magic: the file
	// identifies as a Formidable bundle. This is branding, not protection (a
	// prefix-tolerant zip tool can still read the payload); only a password
	// protects. The manifest reads and the payload unpacks.
	if bytes.HasPrefix(packed, []byte("PK\x03\x04")) {
		t.Error("plain bundle should lead with the brand marker, not the zip magic")
	}
	man, err := bundle.ReadManifest(packed)
	if err != nil {
		t.Fatalf("ReadManifest: %v", err)
	}
	if man.Encrypted {
		t.Error("no-password bundle should report not encrypted")
	}
	zipBytes, err := bundle.Unpack(packed, "")
	if err != nil {
		t.Fatalf("Unpack: %v", err)
	}
	if _, ok := unzipMap(t, zipBytes)["index.html"]; !ok {
		t.Error("plain bundle payload should unzip to the wiki")
	}
}

type fakePacker struct {
	db   []byte
	spec []byte
	got  []string
}

func (f *fakePacker) BuildDataPack(_ context.Context, filenames []string) (DataPack, error) {
	f.got = append([]string(nil), filenames...)
	return DataPack{DB: f.db, OpenAPI: f.spec}, nil
}

func TestExportBundle_EmbedsDataDB(t *testing.T) {
	h := NewHandler(exportProvider(), newStubStorage(), &stubExpressioner{})
	fp := &fakePacker{db: []byte("SQLITE-IMAGE-BYTES"), spec: []byte(`{"openapi":"3.0.3"}`)}
	h.SetDataPacker(fp)

	res, err := h.ExportBundle(context.Background(), map[string][]string{"basic.yaml": nil})
	if err != nil {
		t.Fatalf("ExportBundle: %v", err)
	}
	files := unzipMap(t, res.Zip)
	if string(files["_/data.db"]) != "SQLITE-IMAGE-BYTES" {
		t.Fatalf("_/data.db missing or wrong: %q", files["_/data.db"])
	}
	if string(files["_/openapi.json"]) != `{"openapi":"3.0.3"}` {
		t.Fatalf("_/openapi.json missing or wrong: %q", files["_/openapi.json"])
	}
	// The packer is handed the selected templates (it filters to collections).
	found := false
	for _, fn := range fp.got {
		if fn == "basic.yaml" {
			found = true
		}
	}
	if !found {
		t.Fatalf("packer got %v, want it to include basic.yaml", fp.got)
	}
}

func TestExportBundle_NoPackerNoDataDB(t *testing.T) {
	h := NewHandler(exportProvider(), newStubStorage(), &stubExpressioner{})
	res, err := h.ExportBundle(context.Background(), map[string][]string{"basic.yaml": nil})
	if err != nil {
		t.Fatalf("ExportBundle: %v", err)
	}
	if _, ok := unzipMap(t, res.Zip)["_/data.db"]; ok {
		t.Error("no data.db should be packed without a DataPacker")
	}
}

func TestExportBundle_EmptyPresentationSkipped(t *testing.T) {
	sp := &stubProvider{
		templates: []dataprovider.TemplateSummary{{Stem: "deck", Filename: "deck.yaml", Name: "Deck"}},
	}
	h := NewHandler(sp, newStubStorage(), &stubExpressioner{})
	h.SetTemplates(&stubTemplates{byName: map[string]*tpl.Template{"deck.yaml": {Presentation: true}}})
	h.SetDecks(&fakeDeckProvider{}) // no decks, no sequence -> nothing to export

	if _, err := h.ExportBundle(context.Background(), map[string][]string{"deck.yaml": nil}); err == nil {
		t.Errorf("expected an error when a slideless presentation is the only selection")
	}
}
