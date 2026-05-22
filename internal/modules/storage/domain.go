package storage

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"
	"time"

	"github.com/petervdpas/formidable2/internal/modules/auth"
	"github.com/petervdpas/formidable2/internal/modules/sfr"
	"github.com/petervdpas/formidable2/internal/modules/template"
)

// fs is the narrow filesystem surface storage needs (beyond sfr).
type fs interface {
	ResolvePath(segments ...string) string
	JoinPath(segments ...string) string
	EnsureDirectory(path string) error
	FileExists(path string) bool
	LoadFile(path string) (string, error)
	SaveFile(path string, content string) error
	DeleteFile(path string) error
	ListFiles(dir string) ([]string, error)
}

// templateLoader is the narrow interface the storage module needs from
// template - load by filename. Mirrors template.Manager.LoadTemplate.
type templateLoader interface {
	LoadTemplate(name string) (*template.Template, error)
}

const (
	formExt   = ".meta.json"
	imagesDir = "images"
)

// Indexer is the post-write hook surface a downstream cache plugs
// into for forms (the SQLite index used by the wiki/API). Failures
// are logged at the manager and never propagated - the index is a
// derived view, never authoritative.
type Indexer interface {
	OnFormChanged(templateFilename, datafile string) error
	OnFormDeleted(templateFilename, datafile string) error
}

// FormReader is the symmetric read-side surface for the same cache:
// when installed, ExtendedListForms consults it instead of walking
// disk per record. The composition root supplies an adapter around
// *index.Manager. Implementations should return summaries in the
// filename-ascending order the original disk path used, so existing
// frontends see no observable change.
//
// A reader-side error is non-fatal: the manager logs it and falls
// back to the disk path. The index is a derived view, never
// authoritative - same posture as the Indexer write hook.
type FormReader interface {
	ListSummaries(templateFilename string) ([]FormSummary, error)
}

// Manager owns CRUD over the per-template storage tree.
type Manager struct {
	fs         fs
	sfr        *sfr.Manager
	templates  templateLoader
	log        *slog.Logger
	storageDir string // base storage path (absolute or relative to fs root)
	indexer    Indexer
	reader     FormReader
	author     AuthorProvider
}

// NewManager builds the manager. storageDir is the storage root
// (e.g. "<context>/storage" - the composition root resolves it).
// log may be nil.
func NewManager(filesystem fs, sfrM *sfr.Manager, templates templateLoader, storageDir string, log *slog.Logger) *Manager {
	if log == nil {
		log = slog.Default()
	}
	if storageDir == "" {
		storageDir = "storage"
	}
	return &Manager{
		fs:         filesystem,
		sfr:        sfrM,
		templates:  templates,
		log:        log,
		storageDir: storageDir,
	}
}

// SetIndexer installs the post-write hook for form save/delete.
// Composition root calls this after building the index event handler.
// Pass nil to disable.
func (m *Manager) SetIndexer(i Indexer) { m.indexer = i }

// SetReader installs the read-side surface for ExtendedListForms.
// Symmetric with SetIndexer: the composition root supplies an adapter
// after building the index manager; passing nil disables the
// fast path and reverts to disk reads.
func (m *Manager) SetReader(r FormReader) { m.reader = r }

// SetAuthorProvider installs the active-profile identity source. Every
// SaveForm stamps the returned (name, email) onto Updated, and onto
// Created on first save. Pass nil to fall back to "Unknown".
func (m *Manager) SetAuthorProvider(p AuthorProvider) { m.author = p }

// stamp returns a fresh AuditEntry for the current actor. Resolution
// order: ctx-scoped auth.Identity (request-scoped - HTTP API + future
// subscriptions) > AuthorProvider (process-scoped - Wails IPC + plugin
// host) > ("Unknown", "unknown@example.com") fallback.
func (m *Manager) stamp(ctx context.Context) AuditEntry {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if id, ok := auth.IdentityFromContext(ctx); ok && id.Valid() {
		name, email := id.Name, id.Email
		if name == "" {
			name = "Unknown"
		}
		if email == "" {
			email = "unknown@example.com"
		}
		return AuditEntry{At: now, Name: name, Email: email}
	}
	name, email := "", ""
	if m.author != nil {
		name, email = m.author()
	}
	if name == "" {
		name = "Unknown"
	}
	if email == "" {
		email = "unknown@example.com"
	}
	return AuditEntry{At: now, Name: name, Email: email}
}

// StorageDir returns the absolute storage root, used by the
// composition root to stat form files (mtime + size for the index
// loader adapter).
func (m *Manager) StorageDir() string { return m.storageDir }

// EnsureFormDir creates <storage>/<template-name>/ if missing.
func (m *Manager) EnsureFormDir(templateFilename string) error {
	dir := m.templateDir(templateFilename)
	return m.fs.EnsureDirectory(dir)
}

// ListForms returns the .meta.json filenames inside the template's
// storage folder. Missing folder → empty slice.
func (m *Manager) ListForms(templateFilename string) ([]string, error) {
	dir := m.templateDir(templateFilename)
	if !m.fs.FileExists(dir) {
		return []string{}, nil
	}
	files, err := m.sfr.ListFiles(dir, formExt)
	if err != nil {
		return nil, fmt.Errorf("storage: list %q: %w", templateFilename, err)
	}
	return files, nil
}

// LoadForm reads + sanitizes a form. Returns nil if the file is missing
// or malformed (mirrors JS - frontend treats null as "not found").
func (m *Manager) LoadForm(templateFilename, datafile string) *Form {
	if datafile == "" {
		return nil
	}
	dir := m.templateDir(templateFilename)
	raw, err := m.sfr.LoadFromBase(dir, datafile, sfr.Options{})
	if err != nil {
		return nil
	}
	rawMap, ok := raw.(map[string]any)
	if !ok {
		return nil
	}
	fields := m.fieldsFor(templateFilename)
	out := Sanitize(rawMap, fields, SanitizeOptions{
		TemplateName: strings.TrimSuffix(templateFilename, filepath.Ext(templateFilename)),
	})
	return &out
}

// SaveForm sanitizes the input against the template's fields and writes
// the resulting envelope as JSON. ctx is consulted by stamp() for the
// auth.Identity that attributes Updated (and Created on first save) -
// HTTP API handlers thread the request context here; non-HTTP callers
// (Wails IPC, plugin host) pass context.Background() and fall back to
// the AuthorProvider.
func (m *Manager) SaveForm(ctx context.Context, templateFilename, datafile string, data map[string]any) SaveResult {
	if datafile == "" {
		return SaveResult{Success: false, Error: "empty datafile"}
	}
	if strings.ContainsAny(datafile, `/\`) || strings.Contains(datafile, "..") {
		return SaveResult{Success: false, Error: fmt.Sprintf("invalid datafile %q", datafile)}
	}
	dir := m.templateDir(templateFilename)
	if err := m.fs.EnsureDirectory(dir); err != nil {
		return SaveResult{Success: false, Error: err.Error()}
	}

	fields := m.fieldsFor(templateFilename)
	templateName := strings.TrimSuffix(templateFilename, filepath.Ext(templateFilename))

	// Preserve previously-set id + Created (creator identity is locked
	// for the lifetime of the record). Updated is always re-stamped
	// with the current profile so the audit trail reflects "who saved
	// this last", even when a different profile edits a record.
	prev := m.LoadForm(templateFilename, datafile)
	stamp := m.stamp(ctx)
	opts := SanitizeOptions{
		TemplateName: templateName,
		Updated:      stamp,
	}
	if prev != nil {
		opts.ID = prev.Meta.ID
		opts.Created = prev.Meta.Created
	} else {
		opts.Created = stamp
	}

	envelope := Sanitize(data, fields, opts)
	r := m.sfr.SaveFromBase(dir, datafile, envelope, sfr.Options{})
	if r.Success && m.indexer != nil {
		if err := m.indexer.OnFormChanged(templateFilename, datafile); err != nil {
			m.log.Warn("storage indexer save hook failed",
				"template", templateFilename, "datafile", datafile, "err", err)
		}
	}
	return SaveResult{Success: r.Success, Path: r.Path, Error: r.Error}
}

// SaveFormExact writes a fully-formed envelope as-is, without
// consulting the previously-stored meta. SaveForm is the everyday
// path - it preserves prev.Meta.ID / Created so editing a form can't
// accidentally re-generate identity. SaveFormExact is the escape
// hatch for callers that are deliberately mutating the meta block -
// the integrity repair pipeline mints UUIDs and re-stamps timestamps
// and needs its updated meta to land on disk verbatim. ctx is carried
// for parity with SaveForm but currently unused (the form's Meta is
// already fully resolved).
func (m *Manager) SaveFormExact(ctx context.Context, templateFilename, datafile string, form Form) SaveResult {
	_ = ctx
	if datafile == "" {
		return SaveResult{Success: false, Error: "empty datafile"}
	}
	if strings.ContainsAny(datafile, `/\`) || strings.Contains(datafile, "..") {
		return SaveResult{Success: false, Error: fmt.Sprintf("invalid datafile %q", datafile)}
	}
	dir := m.templateDir(templateFilename)
	if err := m.fs.EnsureDirectory(dir); err != nil {
		return SaveResult{Success: false, Error: err.Error()}
	}
	r := m.sfr.SaveFromBase(dir, datafile, form, sfr.Options{})
	if r.Success && m.indexer != nil {
		if err := m.indexer.OnFormChanged(templateFilename, datafile); err != nil {
			m.log.Warn("storage indexer save hook failed",
				"template", templateFilename, "datafile", datafile, "err", err)
		}
	}
	return SaveResult{Success: r.Success, Path: r.Path, Error: r.Error}
}

// DeleteForm removes the form file. Missing is a no-op.
func (m *Manager) DeleteForm(templateFilename, datafile string) error {
	if datafile == "" {
		return errors.New("storage: empty datafile")
	}
	dir := m.templateDir(templateFilename)
	if err := m.sfr.DeleteFromBase(dir, datafile, sfr.Options{}); err != nil {
		return err
	}
	if m.indexer != nil {
		if err := m.indexer.OnFormDeleted(templateFilename, datafile); err != nil {
			m.log.Warn("storage indexer delete hook failed",
				"template", templateFilename, "datafile", datafile, "err", err)
		}
	}
	return nil
}

// LoadImageFile reads <storage>/<template-name>/images/<name> and returns
// it as a base64 data URL ("data:image/png;base64,…") suitable for direct
// use as an <img src=""> on the frontend. Missing file → empty string +
// nil error (mirrors LoadForm's "missing isn't an error" semantics).
func (m *Manager) LoadImageFile(templateFilename, name string) (string, error) {
	raw, mime, err := m.OpenImageFile(templateFilename, name)
	if err != nil {
		return "", err
	}
	if raw == nil {
		return "", nil
	}
	return "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(raw), nil
}

// OpenImageFile reads the raw image bytes + MIME type without the
// data-URL framing. Used by the wiki HTTP server which streams the
// bytes directly through `/storage/<tpl>/images/<name>` - encoding to
// base64 and decoding in the browser would just bloat the response.
// Missing file → nil bytes + nil error (mirrors LoadImageFile).
func (m *Manager) OpenImageFile(templateFilename, name string) ([]byte, string, error) {
	if name == "" {
		return nil, "", errors.New("storage: empty image name")
	}
	if strings.ContainsAny(name, `/\`) || strings.Contains(name, "..") {
		return nil, "", fmt.Errorf("storage: invalid image name %q", name)
	}
	full := filepath.Join(m.templateDir(templateFilename), imagesDir, name)
	if !m.fs.FileExists(full) {
		return nil, "", nil
	}
	raw, err := m.fs.LoadFile(full)
	if err != nil {
		return nil, "", fmt.Errorf("storage: read image %q: %w", name, err)
	}
	return []byte(raw), imageMIMEFromName(name), nil
}

// imageMIMEFromName maps an image filename's extension to a MIME type.
// Falls back to application/octet-stream for unknown extensions.
func imageMIMEFromName(name string) string {
	ext := strings.ToLower(filepath.Ext(name))
	switch ext {
	case ".png":
		return "image/png"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".gif":
		return "image/gif"
	case ".webp":
		return "image/webp"
	case ".svg":
		return "image/svg+xml"
	default:
		return "application/octet-stream"
	}
}

// DeleteImageFile removes <storage>/<template-name>/images/<name>.
// Missing file is a no-op (mirrors DeleteForm). Used when the user
// clears an image field.
func (m *Manager) DeleteImageFile(templateFilename, name string) error {
	if name == "" {
		return errors.New("storage: empty image name")
	}
	if strings.ContainsAny(name, `/\`) || strings.Contains(name, "..") {
		return fmt.Errorf("storage: invalid image name %q", name)
	}
	full := filepath.Join(m.templateDir(templateFilename), imagesDir, name)
	return m.fs.DeleteFile(full)
}

// SaveImageFile writes raw bytes to <storage>/<template-name>/images/<name>.
// Returns SaveResult with the absolute path on success.
func (m *Manager) SaveImageFile(templateFilename, name string, content []byte) SaveResult {
	if name == "" {
		return SaveResult{Success: false, Error: "empty image name"}
	}
	if strings.ContainsAny(name, `/\`) || strings.Contains(name, "..") {
		return SaveResult{Success: false, Error: fmt.Sprintf("invalid image name %q", name)}
	}
	dir := filepath.Join(m.templateDir(templateFilename), imagesDir)
	if err := m.fs.EnsureDirectory(dir); err != nil {
		return SaveResult{Success: false, Error: err.Error()}
	}
	full := filepath.Join(dir, name)
	if err := m.fs.SaveFile(full, string(content)); err != nil {
		return SaveResult{Success: false, Error: err.Error()}
	}
	return SaveResult{Success: true, Path: full}
}

// ExtendedListForms returns each form summary with title resolved from
// item_field (if set) and any expression-flagged data carried for the
// sidebar mini-expression evaluator.
//
// Fast path: when a FormReader is installed (composition root wires
// one over the SQLite index), summaries come straight off the index
// - one query instead of one disk read per record. Reader errors are
// logged and the disk path runs as a safety net so a transient index
// problem can't make the studio list go blank.
func (m *Manager) ExtendedListForms(templateFilename string) ([]FormSummary, error) {
	if m.reader != nil {
		out, err := m.reader.ListSummaries(templateFilename)
		if err == nil {
			return out, nil
		}
		m.log.Warn("storage: form reader failed, falling back to disk",
			"template", templateFilename, "err", err)
	}
	return m.extendedListFormsFromDisk(templateFilename)
}

// extendedListFormsFromDisk is the original walk-every-file path.
// Kept as a fallback for when the reader isn't installed or errors -
// the index is a derived view, never authoritative.
func (m *Manager) extendedListFormsFromDisk(templateFilename string) ([]FormSummary, error) {
	files, err := m.ListForms(templateFilename)
	if err != nil {
		return nil, err
	}
	tpl, _ := m.templates.LoadTemplate(templateFilename)
	out := make([]FormSummary, 0, len(files))
	for _, filename := range files {
		s, ok := m.summaryFor(templateFilename, filename, tpl)
		if !ok {
			continue
		}
		out = append(out, s)
	}
	return out, nil
}

// ExtendedLoadForm is the single-record analogue of ExtendedListForms.
// Returns nil when the file is missing - same posture as LoadForm -
// so the expression module's per-row "this one form changed" path
// doesn't need to walk every record on disk.
func (m *Manager) ExtendedLoadForm(templateFilename, datafile string) (*FormSummary, error) {
	tpl, _ := m.templates.LoadTemplate(templateFilename)
	s, ok := m.summaryFor(templateFilename, datafile, tpl)
	if !ok {
		return nil, nil
	}
	return &s, nil
}

func (m *Manager) summaryFor(templateFilename, filename string, tpl *template.Template) (FormSummary, bool) {
	f := m.LoadForm(templateFilename, filename)
	if f == nil {
		return FormSummary{}, false
	}
	itemFieldKey := ""
	var fields []template.Field
	if tpl != nil {
		itemFieldKey = tpl.ItemField
		fields = tpl.Fields
	}
	title := filename
	if itemFieldKey != "" {
		if v, ok := f.Data[itemFieldKey]; ok {
			if s, ok := v.(string); ok && s != "" {
				title = s
			}
		}
	}
	expressionItems := map[string]any{}
	for _, fld := range fields {
		if !fld.ExpressionItem {
			continue
		}
		if v, ok := f.Data[fld.Key]; ok && v != nil && v != "" {
			expressionItems[fld.Key] = v
		}
	}
	return FormSummary{
		Filename:        filename,
		Meta:            f.Meta,
		Title:           title,
		ExpressionItems: expressionItems,
	}, true
}

// ─────────────────────────────────────────────────────────────────────
// Helpers
// ─────────────────────────────────────────────────────────────────────

func (m *Manager) templateDir(templateFilename string) string {
	name := strings.TrimSuffix(templateFilename, filepath.Ext(templateFilename))
	return filepath.Join(m.storageDir, name)
}

// TemplateStorageDir is the exported name for templateDir - returns
// the absolute path of `<storage>/<template-stem>/`. Used by the
// Cleanup Storage utility to "Open Storage Folder" for the currently
// selected template via OpenExternal.
func (m *Manager) TemplateStorageDir(templateFilename string) string {
	return m.templateDir(templateFilename)
}

// TemplateImageDir returns the absolute filesystem path of the
// `<storage>/<template>/images/` folder. Public so the render module
// (and future internal HTTP server) can build URLs without re-deriving
// storage layout.
func (m *Manager) TemplateImageDir(templateFilename string) string {
	return filepath.Join(m.templateDir(templateFilename), imagesDir)
}

func (m *Manager) fieldsFor(templateFilename string) []template.Field {
	tpl, err := m.templates.LoadTemplate(templateFilename)
	if err != nil || tpl == nil {
		return nil
	}
	return tpl.Fields
}
