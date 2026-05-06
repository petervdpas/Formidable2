package storage

import (
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"

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
// template — load by filename. Mirrors template.Manager.LoadTemplate.
type templateLoader interface {
	LoadTemplate(name string) (*template.Template, error)
}

const (
	formExt   = ".meta.json"
	imagesDir = "images"
)

// Indexer is the post-write hook surface a downstream cache plugs
// into for forms (the SQLite index used by the wiki/API). Failures
// are logged at the manager and never propagated — the index is a
// derived view, never authoritative.
type Indexer interface {
	OnFormChanged(templateFilename, datafile string) error
	OnFormDeleted(templateFilename, datafile string) error
}

// Manager owns CRUD over the per-template storage tree.
type Manager struct {
	fs        fs
	sfr       *sfr.Manager
	templates templateLoader
	log       *slog.Logger
	storageDir string // base storage path (absolute or relative to fs root)
	indexer   Indexer
}

// NewManager builds the manager. storageDir is the storage root
// (e.g. "<context>/storage" — the composition root resolves it).
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
// or malformed (mirrors JS — frontend treats null as "not found").
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
// the resulting envelope as JSON.
func (m *Manager) SaveForm(templateFilename, datafile string, data map[string]any) SaveResult {
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

	// Preserve previously-set id / created across edits by reading the
	// existing form (if any) and feeding its meta into Sanitize options.
	prev := m.LoadForm(templateFilename, datafile)
	opts := SanitizeOptions{TemplateName: templateName}
	if prev != nil {
		opts.ID = prev.Meta.ID
		opts.Created = prev.Meta.Created
		opts.AuthorName = prev.Meta.AuthorName
		opts.AuthorEmail = prev.Meta.AuthorEmail
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
	if name == "" {
		return "", errors.New("storage: empty image name")
	}
	if strings.ContainsAny(name, `/\`) || strings.Contains(name, "..") {
		return "", fmt.Errorf("storage: invalid image name %q", name)
	}
	full := filepath.Join(m.templateDir(templateFilename), imagesDir, name)
	if !m.fs.FileExists(full) {
		return "", nil
	}
	raw, err := m.fs.LoadFile(full)
	if err != nil {
		return "", fmt.Errorf("storage: read image %q: %w", name, err)
	}
	mime := imageMIMEFromName(name)
	return "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString([]byte(raw)), nil
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
func (m *Manager) ExtendedListForms(templateFilename string) ([]FormSummary, error) {
	files, err := m.ListForms(templateFilename)
	if err != nil {
		return nil, err
	}
	tpl, _ := m.templates.LoadTemplate(templateFilename)
	itemFieldKey := ""
	var fields []template.Field
	if tpl != nil {
		itemFieldKey = tpl.ItemField
		fields = tpl.Fields
	}

	out := make([]FormSummary, 0, len(files))
	for _, filename := range files {
		f := m.LoadForm(templateFilename, filename)
		if f == nil {
			continue
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
		out = append(out, FormSummary{
			Filename:        filename,
			Meta:            f.Meta,
			Title:           title,
			ExpressionItems: expressionItems,
		})
	}
	return out, nil
}

// ─────────────────────────────────────────────────────────────────────
// Helpers
// ─────────────────────────────────────────────────────────────────────

func (m *Manager) templateDir(templateFilename string) string {
	name := strings.TrimSuffix(templateFilename, filepath.Ext(templateFilename))
	return filepath.Join(m.storageDir, name)
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
