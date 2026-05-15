package pdf

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	picoloom "github.com/alnah/picoloom/v2"
	"github.com/petervdpas/formidable2/internal/modules/template"
)

// ---------- template loader stub ----------

type fakeTemplateLoader struct {
	tpls map[string]*template.Template
	err  error
}

func newFakeTemplateLoader() *fakeTemplateLoader {
	return &fakeTemplateLoader{tpls: map[string]*template.Template{}}
}

func (f *fakeTemplateLoader) LoadTemplate(name string) (*template.Template, error) {
	if f.err != nil {
		return nil, f.err
	}
	t, ok := f.tpls[name]
	if !ok {
		return nil, errors.New("template: not found: " + name)
	}
	return t, nil
}

// ---------- test doubles ----------

type fakeRenderer struct {
	md  map[string]string // key = "<tpl>|<datafile>"
	err error
}

func newFakeRenderer() *fakeRenderer { return &fakeRenderer{md: map[string]string{}} }

func (f *fakeRenderer) RenderMarkdown(tpl, df string) (string, error) {
	if f.err != nil {
		return "", f.err
	}
	return f.md[tpl+"|"+df], nil
}

type fakeStorage struct {
	dirs map[string]string
}

func (f *fakeStorage) TemplateStorageDir(tpl string) string { return f.dirs[tpl] }

type fakeConverter struct {
	pdfBytes []byte
	convErr  error
	closeErr error

	seen    picoloom.Input
	seenCtx context.Context
	closed  bool

	mu sync.Mutex
}

func (f *fakeConverter) Convert(ctx context.Context, in picoloom.Input) (*picoloom.ConvertResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.seen = in
	f.seenCtx = ctx
	if f.convErr != nil {
		return nil, f.convErr
	}
	return &picoloom.ConvertResult{HTML: []byte("<html/>"), PDF: f.pdfBytes}, nil
}

func (f *fakeConverter) Close() error { f.closed = true; return f.closeErr }

type fakeConverterFactory struct {
	mu      sync.Mutex
	last    *fakeConverter
	calls   int
	bin     string
	style   string
	coverTS *picoloom.TemplateSet
	err     error
}

func (f *fakeConverterFactory) build(browserBin, style string, coverTS *picoloom.TemplateSet) (converter, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	f.bin = browserBin
	f.style = style
	f.coverTS = coverTS
	if f.err != nil {
		return nil, f.err
	}
	c := &fakeConverter{pdfBytes: []byte("%PDF-1.4\n%fake\n")}
	f.last = c
	return c, nil
}

// ---------- helpers ----------

func newActiveManager(t *testing.T) (*Manager, *memFS, *fakeRenderer, *fakeStorage, *fakeConverterFactory) {
	t.Helper()
	mem := newMemFS()
	fs := fakeFS{}
	vers := fakeVersions{}
	rdr := newFakeRenderer()
	stg := &fakeStorage{dirs: map[string]string{}}
	cf := &fakeConverterFactory{}

	m := &Manager{
		log:       slog.Default(),
		store:     &store{fs: mem, log: slog.Default()},
		prober:    &prober{fs: fs, versions: vers, goos: "linux", cacheRoot: "/cache/rod/browser"},
		nowFn:     func() time.Time { return time.Date(2026, 5, 15, 12, 0, 0, 0, time.UTC) },
		dirOK:     func(p string) bool { return false },
		renderer:  rdr,
		storage:   stg,
		convertFn: cf.build,
		status:    Status{Source: SourceUnset},
	}

	// Seed activation directly without going through Activate (avoids
	// the prober step).
	fs["/usr/bin/chromium"] = true
	vers["/usr/bin/chromium"] = struct {
		version string
		err     error
	}{version: "Chromium 148", err: nil}
	if _, err := m.Activate(ActivateOpts{BrowserBin: "/usr/bin/chromium"}); err != nil {
		t.Fatalf("seed Activate: %v", err)
	}
	return m, mem, rdr, stg, cf
}

// ---------- tests ----------

func TestExport_InactiveStillReturnsNotActivated(t *testing.T) {
	mem := newMemFS()
	m := &Manager{
		log:       slog.Default(),
		store:     &store{fs: mem, log: slog.Default()},
		prober:    &prober{fs: fakeFS{}, versions: fakeVersions{}, goos: "linux", cacheRoot: "/x"},
		nowFn:     func() time.Time { return time.Now() },
		dirOK:     func(p string) bool { return false },
		renderer:  newFakeRenderer(),
		storage:   &fakeStorage{dirs: map[string]string{}},
		convertFn: (&fakeConverterFactory{}).build,
		status:    Status{Source: SourceUnset},
	}

	_, err := m.Export("tpl.yaml", "form-1.meta.json", ExportOpts{})
	if !errors.Is(err, ErrPDFNotActivated) {
		t.Errorf("err = %v, want ErrPDFNotActivated", err)
	}
}

func TestExport_HappyPathWritesPDF(t *testing.T) {
	m, mem, rdr, stg, cf := newActiveManager(t)
	stg.dirs["tpl.yaml"] = "/storage/tpl"
	rdr.md["tpl.yaml|form-1.meta.json"] = "---\nstyle: technical\ncover:\n  title: Hello\n---\n# Body\n"

	res, err := m.Export("tpl.yaml", "form-1.meta.json", ExportOpts{})
	if err != nil {
		t.Fatalf("Export err = %v", err)
	}
	if res.Bytes <= 0 {
		t.Errorf("result.Bytes = %d, want > 0", res.Bytes)
	}
	if res.Path == "" {
		t.Errorf("result.Path empty")
	}
	if !strings.HasSuffix(res.Path, ".pdf") {
		t.Errorf("result.Path = %q, want suffix .pdf", res.Path)
	}
	if !mem.FileExists(res.Path) {
		t.Errorf("PDF not written to %q", res.Path)
	}
	if cf.last == nil {
		t.Fatalf("converter factory not called")
	}
	if cf.last.seen.Markdown != "# Body\n" {
		t.Errorf("converter saw markdown = %q, want frontmatter-stripped body", cf.last.seen.Markdown)
	}
	if cf.last.seen.Cover == nil || cf.last.seen.Cover.Title != "Hello" {
		t.Errorf("converter saw cover = %+v, want title=Hello", cf.last.seen.Cover)
	}
	if cf.style != "technical" {
		t.Errorf("converter factory style = %q, want technical", cf.style)
	}
	if !cf.last.closed {
		t.Errorf("converter was not closed")
	}
}

func TestExport_OutputPath_AbsoluteOptsWins(t *testing.T) {
	m, mem, rdr, stg, _ := newActiveManager(t)
	stg.dirs["tpl.yaml"] = "/storage/tpl"
	rdr.md["tpl.yaml|form-1.meta.json"] = "# body"

	res, err := m.Export("tpl.yaml", "form-1.meta.json", ExportOpts{OutputPath: "/custom/abs/out.pdf"})
	if err != nil {
		t.Fatalf("Export: %v", err)
	}
	if res.Path != "/custom/abs/out.pdf" {
		t.Errorf("path = %q, want /custom/abs/out.pdf", res.Path)
	}
	if !mem.FileExists("/custom/abs/out.pdf") {
		t.Errorf("output not written to absolute opts path")
	}
}

func TestExport_OutputPath_ExportDirDefault(t *testing.T) {
	m, mem, rdr, stg, _ := newActiveManager(t)
	stg.dirs["tpl.yaml"] = "/storage/tpl"
	rdr.md["tpl.yaml|adapter-eum.meta.json"] = "# body"
	// Configure ExportDir
	m.dirOK = func(p string) bool { return p == "/home/peter/PDFs" }
	if _, err := m.SetExportDir("/home/peter/PDFs"); err != nil {
		t.Fatalf("seed SetExportDir: %v", err)
	}

	res, err := m.Export("tpl.yaml", "adapter-eum.meta.json", ExportOpts{})
	if err != nil {
		t.Fatalf("Export: %v", err)
	}
	want := "/home/peter/PDFs/adapter-eum.pdf"
	if res.Path != want {
		t.Errorf("path = %q, want %q", res.Path, want)
	}
	if !mem.FileExists(want) {
		t.Errorf("PDF not written to ExportDir default")
	}
}

func TestExport_OutputPath_FallsBackToStorageDir(t *testing.T) {
	m, mem, rdr, stg, _ := newActiveManager(t)
	stg.dirs["tpl.yaml"] = "/abs/storage/tpl"
	rdr.md["tpl.yaml|form-1.meta.json"] = "# body"

	res, err := m.Export("tpl.yaml", "form-1.meta.json", ExportOpts{})
	if err != nil {
		t.Fatalf("Export: %v", err)
	}
	want := "/abs/storage/tpl/form-1.pdf"
	if res.Path != want {
		t.Errorf("path = %q, want %q", res.Path, want)
	}
	if !mem.FileExists(want) {
		t.Errorf("PDF not written to template storage dir")
	}
}

func TestExport_Style_OptsOverridesFrontmatter(t *testing.T) {
	m, _, rdr, stg, cf := newActiveManager(t)
	stg.dirs["tpl.yaml"] = "/x"
	rdr.md["tpl.yaml|f.meta.json"] = "---\nstyle: technical\n---\n# body"

	_, err := m.Export("tpl.yaml", "f.meta.json", ExportOpts{Style: "academic"})
	if err != nil {
		t.Fatalf("Export: %v", err)
	}
	if cf.style != "academic" {
		t.Errorf("converter style = %q, want academic (opts override)", cf.style)
	}
}

func TestExport_Style_DefaultsToEmptyWhenNoneSet(t *testing.T) {
	m, _, rdr, stg, cf := newActiveManager(t)
	stg.dirs["tpl.yaml"] = "/x"
	rdr.md["tpl.yaml|f.meta.json"] = "# body"

	_, err := m.Export("tpl.yaml", "f.meta.json", ExportOpts{})
	if err != nil {
		t.Fatalf("Export: %v", err)
	}
	if cf.style != "" {
		t.Errorf("converter style = %q, want empty (no style → picoloom default)", cf.style)
	}
}

func TestExport_SourceDirDefaultsToStorageDir(t *testing.T) {
	m, _, rdr, stg, cf := newActiveManager(t)
	stg.dirs["tpl.yaml"] = "/abs/storage/tpl"
	rdr.md["tpl.yaml|f.meta.json"] = "# body"

	_, err := m.Export("tpl.yaml", "f.meta.json", ExportOpts{})
	if err != nil {
		t.Fatalf("Export: %v", err)
	}
	if cf.last.seen.SourceDir != "/abs/storage/tpl" {
		t.Errorf("Input.SourceDir = %q, want template storage dir", cf.last.seen.SourceDir)
	}
}

func TestExport_RendererError_Wrapped(t *testing.T) {
	m, _, rdr, stg, _ := newActiveManager(t)
	stg.dirs["tpl.yaml"] = "/x"
	rdr.err = errors.New("template not found")

	_, err := m.Export("tpl.yaml", "f.meta.json", ExportOpts{})
	if err == nil {
		t.Fatalf("Export err = nil, want non-nil")
	}
	if !strings.Contains(err.Error(), "template not found") {
		t.Errorf("err = %v, want wrapped renderer error", err)
	}
}

func TestExport_ConverterFactoryError_Wrapped(t *testing.T) {
	m, _, rdr, stg, cf := newActiveManager(t)
	stg.dirs["tpl.yaml"] = "/x"
	rdr.md["tpl.yaml|f.meta.json"] = "# body"
	cf.err = errors.New("chrome refused to boot")

	_, err := m.Export("tpl.yaml", "f.meta.json", ExportOpts{})
	if err == nil {
		t.Fatalf("Export err = nil, want non-nil")
	}
	if !strings.Contains(err.Error(), "chrome refused to boot") {
		t.Errorf("err = %v, want wrapped factory error", err)
	}
}

func TestExport_ConvertError_Wrapped(t *testing.T) {
	m, _, rdr, stg, cf := newActiveManager(t)
	stg.dirs["tpl.yaml"] = "/x"
	rdr.md["tpl.yaml|f.meta.json"] = "# body"
	// Replace the converter factory with one that errors on Convert.
	m.convertFn = func(bin, style string, _ *picoloom.TemplateSet) (converter, error) {
		c := &fakeConverter{convErr: errors.New("page load timeout")}
		cf.last = c
		return c, nil
	}

	_, err := m.Export("tpl.yaml", "f.meta.json", ExportOpts{})
	if err == nil {
		t.Fatalf("Export err = nil, want non-nil")
	}
	if !strings.Contains(err.Error(), "page load timeout") {
		t.Errorf("err = %v, want wrapped convert error", err)
	}
	if cf.last == nil || !cf.last.closed {
		t.Errorf("converter not closed on convert failure")
	}
}

func TestExport_SaveError_Wrapped(t *testing.T) {
	m, mem, rdr, stg, _ := newActiveManager(t)
	stg.dirs["tpl.yaml"] = "/x"
	rdr.md["tpl.yaml|f.meta.json"] = "# body"
	mem.saveErr = errors.New("disk full")

	_, err := m.Export("tpl.yaml", "f.meta.json", ExportOpts{})
	if err == nil {
		t.Fatalf("Export err = nil, want non-nil")
	}
	if !strings.Contains(err.Error(), "disk full") {
		t.Errorf("err = %v, want wrapped save error", err)
	}
}

func TestExport_PerFormSerialization(t *testing.T) {
	m, _, rdr, stg, cf := newActiveManager(t)
	stg.dirs["tpl.yaml"] = "/x"
	rdr.md["tpl.yaml|same.meta.json"] = "# body"

	// Replace the factory with one that records concurrency.
	var active int32
	var maxActive int32
	mu := &sync.Mutex{}
	m.convertFn = func(bin, style string, _ *picoloom.TemplateSet) (converter, error) {
		mu.Lock()
		active++
		if active > maxActive {
			maxActive = active
		}
		mu.Unlock()
		return &fakeBlockingConverter{
			pdf:    []byte("%PDF-1.4 fake"),
			finish: func() {
				time.Sleep(20 * time.Millisecond)
				mu.Lock()
				active--
				mu.Unlock()
			},
		}, nil
	}
	_ = cf // unused

	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = m.Export("tpl.yaml", "same.meta.json", ExportOpts{})
		}()
	}
	wg.Wait()
	if maxActive > 1 {
		t.Errorf("max concurrent renders for same form = %d, want 1", maxActive)
	}
}

func TestExport_DifferentFormsParallelizable(t *testing.T) {
	m, _, rdr, stg, _ := newActiveManager(t)
	stg.dirs["tpl.yaml"] = "/x"
	for _, df := range []string{"a.meta.json", "b.meta.json", "c.meta.json"} {
		rdr.md["tpl.yaml|"+df] = "# body"
	}

	var active int32
	var maxActive int32
	mu := &sync.Mutex{}
	m.convertFn = func(bin, style string, _ *picoloom.TemplateSet) (converter, error) {
		mu.Lock()
		active++
		if active > maxActive {
			maxActive = active
		}
		mu.Unlock()
		return &fakeBlockingConverter{
			pdf: []byte("%PDF-1.4 fake"),
			finish: func() {
				time.Sleep(30 * time.Millisecond)
				mu.Lock()
				active--
				mu.Unlock()
			},
		}, nil
	}

	var wg sync.WaitGroup
	for _, df := range []string{"a.meta.json", "b.meta.json", "c.meta.json"} {
		wg.Add(1)
		go func(df string) {
			defer wg.Done()
			_, _ = m.Export("tpl.yaml", df, ExportOpts{})
		}(df)
	}
	wg.Wait()
	if maxActive < 2 {
		t.Errorf("max concurrent renders across forms = %d, want >= 2 (forms should parallelize)", maxActive)
	}
}

func TestExport_MalformedFrontmatterUsesDefaults(t *testing.T) {
	m, _, rdr, stg, cf := newActiveManager(t)
	stg.dirs["tpl.yaml"] = "/x"
	// Malformed YAML — missing closing delimiter
	rdr.md["tpl.yaml|f.meta.json"] = "---\nstyle: technical\nbody never closes"

	_, err := m.Export("tpl.yaml", "f.meta.json", ExportOpts{})
	if err != nil {
		t.Fatalf("Export should tolerate malformed frontmatter; got %v", err)
	}
	if cf.style != "" {
		t.Errorf("malformed frontmatter let style leak through: %q", cf.style)
	}
}

// fakeBlockingConverter holds Convert open for a tick so we can
// measure concurrent calls.
type fakeBlockingConverter struct {
	pdf    []byte
	finish func()
}

func (f *fakeBlockingConverter) Convert(ctx context.Context, in picoloom.Input) (*picoloom.ConvertResult, error) {
	if f.finish != nil {
		f.finish()
	}
	return &picoloom.ConvertResult{PDF: f.pdf}, nil
}

func (f *fakeBlockingConverter) Close() error { return nil }

// ---------- Stage 6: manifest layer + cover resolution ----------

func TestExport_ManifestStyleFromTemplate(t *testing.T) {
	m, _, rdr, stg, cf := newActiveManager(t)
	stg.dirs["tpl.yaml"] = "/storage/tpl"
	rdr.md["tpl.yaml|f.meta.json"] = "# body" // no doc frontmatter style
	loader := newFakeTemplateLoader()
	loader.tpls["tpl.yaml"] = &template.Template{
		PDF: &template.PDFConfig{Style: "academic"},
	}
	m.templates = loader

	if _, err := m.Export("tpl.yaml", "f.meta.json", ExportOpts{}); err != nil {
		t.Fatalf("Export: %v", err)
	}
	if cf.style != "academic" {
		t.Errorf("style = %q, want academic (from template manifest)", cf.style)
	}
}

func TestExport_DocFMOverridesManifestStyle(t *testing.T) {
	m, _, rdr, stg, cf := newActiveManager(t)
	stg.dirs["tpl.yaml"] = "/storage/tpl"
	rdr.md["tpl.yaml|f.meta.json"] = "---\nstyle: technical\n---\n# body"
	loader := newFakeTemplateLoader()
	loader.tpls["tpl.yaml"] = &template.Template{
		PDF: &template.PDFConfig{Style: "academic"},
	}
	m.templates = loader

	if _, err := m.Export("tpl.yaml", "f.meta.json", ExportOpts{}); err != nil {
		t.Fatalf("Export: %v", err)
	}
	if cf.style != "technical" {
		t.Errorf("style = %q, want technical (doc wins over manifest)", cf.style)
	}
}

func TestExport_OptsOverridesManifestStyle(t *testing.T) {
	m, _, rdr, stg, cf := newActiveManager(t)
	stg.dirs["tpl.yaml"] = "/storage/tpl"
	rdr.md["tpl.yaml|f.meta.json"] = "# body"
	loader := newFakeTemplateLoader()
	loader.tpls["tpl.yaml"] = &template.Template{
		PDF: &template.PDFConfig{Style: "academic"},
	}
	m.templates = loader

	if _, err := m.Export("tpl.yaml", "f.meta.json", ExportOpts{Style: "corporate"}); err != nil {
		t.Fatalf("Export: %v", err)
	}
	if cf.style != "corporate" {
		t.Errorf("style = %q, want corporate (opts wins)", cf.style)
	}
}

func TestExport_ManifestCoverProjectsThroughToFactory(t *testing.T) {
	m, _, rdr, stg, cf := newActiveManager(t)
	stg.dirs["tpl.yaml"] = "/storage/tpl"
	rdr.md["tpl.yaml|f.meta.json"] = "# body"
	loader := newFakeTemplateLoader()
	loader.tpls["tpl.yaml"] = &template.Template{
		PDF: &template.PDFConfig{
			Cover: &template.PDFCoverConfig{
				Template:     "banner",
				Title:        "Manifest Title",
				Organization: "Fontys",
			},
		},
	}
	m.templates = loader

	if _, err := m.Export("tpl.yaml", "f.meta.json", ExportOpts{}); err != nil {
		t.Fatalf("Export: %v", err)
	}
	if cf.last == nil || cf.last.seen.Cover == nil {
		t.Fatalf("converter saw no cover")
	}
	if cf.last.seen.Cover.Title != "Manifest Title" {
		t.Errorf("cover.Title = %q, want Manifest Title", cf.last.seen.Cover.Title)
	}
	if cf.last.seen.Cover.Organization != "Fontys" {
		t.Errorf("cover.Organization = %q, want Fontys", cf.last.seen.Cover.Organization)
	}
	if cf.coverTS == nil {
		t.Fatalf("factory got nil TemplateSet; manifest cover.template=banner should produce one")
	}
	if cf.coverTS.Name != "banner" {
		t.Errorf("TemplateSet.Name = %q, want banner", cf.coverTS.Name)
	}
	if !strings.Contains(cf.coverTS.Cover, "cover-banner") {
		t.Errorf("TemplateSet.Cover missing banner design marker")
	}
}

func TestExport_DocFMOverridesManifestCoverFields(t *testing.T) {
	m, _, rdr, stg, cf := newActiveManager(t)
	stg.dirs["tpl.yaml"] = "/storage/tpl"
	// Doc frontmatter overrides title only; other manifest fields cascade.
	rdr.md["tpl.yaml|f.meta.json"] = "---\ncover:\n  title: Doc Title\n---\n# body"
	loader := newFakeTemplateLoader()
	loader.tpls["tpl.yaml"] = &template.Template{
		PDF: &template.PDFConfig{
			Cover: &template.PDFCoverConfig{
				Template:     "classic",
				Title:        "Manifest Title",
				Organization: "Fontys",
			},
		},
	}
	m.templates = loader

	if _, err := m.Export("tpl.yaml", "f.meta.json", ExportOpts{}); err != nil {
		t.Fatalf("Export: %v", err)
	}
	c := cf.last.seen.Cover
	if c == nil {
		t.Fatalf("no cover")
	}
	if c.Title != "Doc Title" {
		t.Errorf("Title = %q, want Doc Title (doc wins)", c.Title)
	}
	if c.Organization != "Fontys" {
		t.Errorf("Organization = %q, want Fontys (manifest cascades)", c.Organization)
	}
	if cf.coverTS == nil || cf.coverTS.Name != "classic" {
		t.Errorf("TemplateSet = %+v, want classic from manifest", cf.coverTS)
	}
}

func TestExport_DocFMCoverTemplateOverridesManifest(t *testing.T) {
	m, _, rdr, stg, cf := newActiveManager(t)
	stg.dirs["tpl.yaml"] = "/storage/tpl"
	rdr.md["tpl.yaml|f.meta.json"] = "---\ncover:\n  template: corporate\n---\n# body"
	loader := newFakeTemplateLoader()
	loader.tpls["tpl.yaml"] = &template.Template{
		PDF: &template.PDFConfig{
			Cover: &template.PDFCoverConfig{Template: "banner"},
		},
	}
	m.templates = loader

	if _, err := m.Export("tpl.yaml", "f.meta.json", ExportOpts{}); err != nil {
		t.Fatalf("Export: %v", err)
	}
	if cf.coverTS == nil || cf.coverTS.Name != "corporate" {
		t.Errorf("TemplateSet = %+v, want corporate from doc", cf.coverTS)
	}
}

func TestExport_NoCoverNameMeansPicoloomDefault(t *testing.T) {
	// Cover block in doc but no Template/TemplatePath → no override.
	m, _, rdr, stg, cf := newActiveManager(t)
	stg.dirs["tpl.yaml"] = "/storage/tpl"
	rdr.md["tpl.yaml|f.meta.json"] = "---\ncover:\n  title: Hello\n---\n# body"

	if _, err := m.Export("tpl.yaml", "f.meta.json", ExportOpts{}); err != nil {
		t.Fatalf("Export: %v", err)
	}
	if cf.coverTS != nil {
		t.Errorf("TemplateSet = %+v, want nil (no override → picoloom default)", cf.coverTS)
	}
	if cf.last.seen.Cover == nil || cf.last.seen.Cover.Title != "Hello" {
		t.Errorf("Cover data should still flow: got %+v", cf.last.seen.Cover)
	}
}

func TestExport_TemplatePathFromDocFM(t *testing.T) {
	m, mem, rdr, stg, cf := newActiveManager(t)
	stg.dirs["tpl.yaml"] = "/storage/tpl"
	// Seed a user-authored cover HTML on the (in-memory) FS.
	mem.files["/storage/tpl/assets/my-cover.html"] =
		`<section class="cover">USER {{.Title}}</section><span data-cover-end></span>`
	rdr.md["tpl.yaml|f.meta.json"] =
		"---\ncover:\n  template_path: assets/my-cover.html\n  title: Hi\n---\n# body"

	if _, err := m.Export("tpl.yaml", "f.meta.json", ExportOpts{}); err != nil {
		t.Fatalf("Export: %v", err)
	}
	if cf.coverTS == nil {
		t.Fatalf("no TemplateSet, want user-file resolved")
	}
	if !strings.Contains(cf.coverTS.Cover, "USER") {
		t.Errorf("Cover HTML did not load user file content; got %q", cf.coverTS.Cover)
	}
}

func TestExport_UnknownCoverTemplate_SurfacesError(t *testing.T) {
	m, _, rdr, stg, _ := newActiveManager(t)
	stg.dirs["tpl.yaml"] = "/storage/tpl"
	rdr.md["tpl.yaml|f.meta.json"] = "---\ncover:\n  template: nope\n---\n# body"

	_, err := m.Export("tpl.yaml", "f.meta.json", ExportOpts{})
	if err == nil {
		t.Fatalf("Export err = nil, want resolve-cover error")
	}
	if !errors.Is(err, ErrCoverNotFound) {
		t.Errorf("err = %v, want ErrCoverNotFound", err)
	}
}

func TestExport_NilTemplateLoaderDoesntCrash(t *testing.T) {
	// Manager without a templateLoader: manifest layer is skipped, doc
	// frontmatter still works.
	m, _, rdr, stg, cf := newActiveManager(t)
	stg.dirs["tpl.yaml"] = "/storage/tpl"
	rdr.md["tpl.yaml|f.meta.json"] = "---\nstyle: technical\n---\n# body"
	m.templates = nil

	if _, err := m.Export("tpl.yaml", "f.meta.json", ExportOpts{}); err != nil {
		t.Fatalf("Export: %v", err)
	}
	if cf.style != "technical" {
		t.Errorf("style = %q, want technical (doc only)", cf.style)
	}
}
