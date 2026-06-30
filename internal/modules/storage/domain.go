package storage

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/petervdpas/formidable2/internal/modules/auth"
	"github.com/petervdpas/formidable2/internal/modules/sfr"
	"github.com/petervdpas/formidable2/internal/modules/template"
)

// fs is the filesystem surface storage needs (beyond sfr).
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

// templateLoader is what storage needs from template: load by filename.
type templateLoader interface {
	LoadTemplate(name string) (*template.Template, error)
}

const (
	formExt   = ".meta.json"
	imagesDir = "images"
)

// Indexer is the post-write hook for forms (the SQLite index). Failures
// are logged and never propagated; the index is a derived view.
type Indexer interface {
	OnFormChanged(templateFilename, datafile string) error
	OnFormDeleted(templateFilename, datafile string) error
}

// FormReader is the index read-side; ListSummaries returns filename-ascending and falls back to disk on error.
type FormReader interface {
	ListSummaries(templateFilename string) ([]FormSummary, error)
	// LoadSummary returns one form's summary from the index, the single source
	// of harvested ExpressionItems (fields + facets + formulas). The per-record
	// path (ExtendedLoadForm) reads it so it never diverges from the list path.
	LoadSummary(templateFilename, datafile string) (FormSummary, bool, error)
	// SearchSummaries has no disk fallback: FTS is index-only, so a missing/erroring reader surfaces as an error.
	SearchSummaries(templateFilename, query string) ([]FormSummary, error)
	// MaxValue returns the greatest scalar numeric value for fieldKey across the
	// template's records (ok=false when none yet). Backs sequence auto-assign;
	// index-only, no disk fallback.
	MaxValue(templateFilename, fieldKey string) (float64, bool, error)
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
	formulas   FormulaFiller
	scales     ScaleFiller
	author     AuthorProvider
}

// FormulaFiller computes a form's formula field values, so the disk-read
// fallback (when no index reader is wired) still produces complete expression
// context. Storage owns no calculator; the composition root supplies one (the
// same one the index harvest uses, so the values match).
type FormulaFiller interface {
	FormulaValues(t *template.Template, f *Form) map[string]any
}

// ScaleFiller computes a form's scaling factors (keyed by scaling name) for the
// disk-read fallback, folded into the expression context under the S namespace.
// Supplied by the composition root, like FormulaFiller.
type ScaleFiller interface {
	ScaleValues(t *template.Template, f *Form) map[string]any
}

// NewManager builds the manager rooted at storageDir (the composition root resolves it); log may be nil.
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

// SetIndexer installs the post-write hook for form save/delete (nil disables).
func (m *Manager) SetIndexer(i Indexer) { m.indexer = i }

// SetFormulaFiller installs the formula calculator for the disk-read fallback
// (nil leaves disk reads with fields + facets only).
func (m *Manager) SetFormulaFiller(ff FormulaFiller) { m.formulas = ff }

// SetScaleFiller installs the scaling calculator for the disk-read fallback
// (nil leaves disk reads without an S namespace).
func (m *Manager) SetScaleFiller(sf ScaleFiller) { m.scales = sf }

// SetReader installs the index read-side for ExtendedListForms (nil reverts to disk reads).
func (m *Manager) SetReader(r FormReader) { m.reader = r }

// SetAuthorProvider installs the active-profile identity source (nil falls back to "Unknown").
func (m *Manager) SetAuthorProvider(p AuthorProvider) { m.author = p }

// stamp returns a fresh AuditEntry. Resolution order: ctx auth.Identity (request-scoped) >
// AuthorProvider (process-scoped) > ("Unknown", "unknown@example.com").
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

// StorageDir returns the absolute storage root.
func (m *Manager) StorageDir() string { return m.storageDir }

// EnsureFormDir creates <storage>/<template-name>/ if missing.
func (m *Manager) EnsureFormDir(templateFilename string) error {
	dir := m.templateDir(templateFilename)
	return m.fs.EnsureDirectory(dir)
}

// ListForms returns the .meta.json filenames in the template's storage folder; missing folder yields an empty slice.
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

// LoadForm reads + sanitizes a form, returning nil when the file is missing or malformed.
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
	// Autofill: the data guid field is the source of truth and meta.id mirrors it
	// (Sanitize resolves data first, then mints when both are empty). Persist once
	// whenever disk is out of sync with that resolved id: a freshly minted guid, a
	// data id meta never copied, or any data/meta drift. Without this the id stays
	// unstable, so the index, link picker, and relation edges latch onto throwaway
	// guids that drift apart across loads, scans, and clones.
	if gk := guidFieldKey(fields); gk != "" && out.Meta.ID != "" {
		rawData, rawMeta := splitEnvelope(rawMap)
		if stringOrEmpty(rawData[gk]) != out.Meta.ID || stringOrEmpty(rawMeta["id"]) != out.Meta.ID {
			m.SaveFormExact(context.Background(), templateFilename, datafile, out)
		}
	}
	if tpl, err := m.templates.LoadTemplate(templateFilename); err == nil {
		m.applyFormulaFields(tpl, &out, "load")
	}
	return &out
}

// applyFormulaFields writes each formula virtual field's source value into its
// target data slot, for fields whose Trigger matches (load|save). The target is
// an ordinary data field, so the computed value persists like a typed entry. A
// nil FormulaFiller (or no matching field) is a no-op. The same calculator the
// index harvest uses computes the values, so the persisted value matches what
// charts and the sidebar see.
func (m *Manager) applyFormulaFields(tpl *template.Template, f *Form, trigger string) {
	if m.formulas == nil || tpl == nil || f == nil {
		return
	}
	matched := false
	for _, fld := range tpl.Fields {
		if fld.Type == "formula" && fld.Trigger == trigger && fld.FormulaKey != "" && fld.TargetKey != "" {
			matched = true
			break
		}
	}
	if !matched {
		return
	}
	vals := m.formulas.FormulaValues(tpl, f)
	if len(vals) == 0 {
		return
	}
	if f.Data == nil {
		f.Data = map[string]any{}
	}
	for _, fld := range tpl.Fields {
		if fld.Type != "formula" || fld.Trigger != trigger || fld.FormulaKey == "" || fld.TargetKey == "" {
			continue
		}
		if v, ok := vals[fld.FormulaKey]; ok {
			f.Data[fld.TargetKey] = v
		}
	}
}

// LoadFormRaw parses the on-disk envelope WITHOUT sanitizing, so the integrity doctor can detect a
// guid field that is empty on disk while meta.id is set (LoadForm would mask the drift). Meta carries only id.
func (m *Manager) LoadFormRaw(templateFilename, datafile string) *Form {
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
	data, meta := splitEnvelope(rawMap)
	fm := FormMeta{ID: stringOrEmpty(meta["id"])}
	// Carry the on-disk facets verbatim (no seeding), so the integrity doctor
	// can see a facet a form is actually missing instead of the seeded default.
	if f, ok := facetsFromAny(meta["facets"]); ok {
		fm.Facets = f
	}
	return &Form{
		Meta: fm,
		Data: data,
	}
}

// SaveForm sanitizes input against the template's fields and writes the envelope as JSON.
// ctx is consulted by stamp() for the Identity attributing Updated (and Created on first save).
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

	// Created (creator identity) is locked for the record's lifetime; Updated is always re-stamped with the current profile.
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
	// Guard guid uniqueness within the collection: if this guid already belongs
	// to a DIFFERENT record, mint a fresh one. A duplicate (usually a copied or
	// imported file carrying another record's id) would make relation edges and
	// api references ambiguous. The data guid field is the identity source;
	// meta.id mirrors it. Best-effort; with no index reader we can't check.
	if gk := guidFieldKey(fields); gk != "" && envelope.Meta.ID != "" &&
		m.guidCollides(templateFilename, datafile, envelope.Meta.ID) {
		fresh := uuid.NewString()
		envelope.Data[gk] = fresh // source of truth
		envelope.Meta.ID = fresh  // mirror
	}
	// On save the meta facets sync to the template: an undeclared facet key is
	// dropped, and a selection that is no longer an option falls back to the
	// field default or empties (Set stays). The doctor backstops any leftover.
	if tpl, err := m.templates.LoadTemplate(templateFilename); err == nil && tpl != nil {
		syncFormFacets(envelope.Meta.Facets, tpl)
		m.applyFormulaFields(tpl, &envelope, "save")
	}
	ordered := orderedForm{Meta: envelope.Meta, Data: orderData(envelope.Data, fields)}
	r := m.sfr.SaveFromBase(dir, datafile, ordered, sfr.Options{})
	if r.Success && m.indexer != nil {
		if err := m.indexer.OnFormChanged(templateFilename, datafile); err != nil {
			m.log.Warn("storage indexer save hook failed",
				"template", templateFilename, "datafile", datafile, "err", err)
		}
	}
	return SaveResult{Success: r.Success, Path: r.Path, Error: r.Error}
}

// SaveFormExact writes a fully-formed envelope verbatim, without consulting prior meta. It is the escape
// hatch for callers deliberately mutating the meta block (the integrity repair pipeline). ctx is unused here.
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
	fields := m.fieldsFor(templateFilename)
	ordered := orderedForm{Meta: form.Meta, Data: orderData(form.Data, fields)}
	r := m.sfr.SaveFromBase(dir, datafile, ordered, sfr.Options{})
	if r.Success && m.indexer != nil {
		if err := m.indexer.OnFormChanged(templateFilename, datafile); err != nil {
			m.log.Warn("storage indexer save hook failed",
				"template", templateFilename, "datafile", datafile, "err", err)
		}
	}
	return SaveResult{Success: r.Success, Path: r.Path, Error: r.Error}
}

// DeleteForm removes the form file (missing is a no-op).
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

// LoadImageFile returns the image as a base64 data URL; missing file yields "" + nil error.
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

// OpenImageFile returns raw image bytes + MIME (no data-URL framing) for the wiki to stream directly;
// missing file yields nil bytes + nil error.
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

// imageMIMEFromName maps an extension to a MIME type, defaulting to application/octet-stream.
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

// DeleteImageFile removes an image (missing file is a no-op).
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

// SaveImageFile writes raw bytes to the template's images folder, returning the absolute path on success.
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

// ExtendedListForms returns form summaries (title from item_field, plus expression-flagged data).
// With a FormReader installed it reads off the index; reader errors fall back to disk so the list can't go blank.
func (m *Manager) ExtendedListForms(templateFilename string) ([]FormSummary, error) {
	if m.reader != nil {
		out, err := m.reader.ListSummaries(templateFilename)
		if err == nil {
			m.sortSummaries(templateFilename, out)
			return out, nil
		}
		m.log.Warn("storage: form reader failed, falling back to disk",
			"template", templateFilename, "err", err)
	}
	out, err := m.extendedListFormsFromDisk(templateFilename)
	if err != nil {
		return nil, err
	}
	m.sortSummaries(templateFilename, out)
	return out, nil
}

// sortSummaries reorders the list by the item-field value (Title) when the
// template opts in via SortByItemField; otherwise the reader/disk filename
// order is left untouched. Title falls back to the filename, so empty item
// fields still sort stably. Case-insensitive, filename as tie-breaker.
func (m *Manager) sortSummaries(templateFilename string, out []FormSummary) {
	tpl, err := m.templates.LoadTemplate(templateFilename)
	if err != nil || tpl == nil || !tpl.SortByItemField {
		return
	}
	sort.SliceStable(out, func(i, j int) bool {
		ti, tj := strings.ToLower(out[i].Title), strings.ToLower(out[j].Title)
		if ti != tj {
			return ti < tj
		}
		return out[i].Filename < out[j].Filename
	})
}

// SearchForms returns full-text matches ranked by relevance, backed entirely by the FTS index;
// no reader means an error (not a disk walk), and an empty query matches nothing.
func (m *Manager) SearchForms(templateFilename, query string) ([]FormSummary, error) {
	if m.reader == nil {
		return nil, errors.New("storage: full-text search requires the index (no reader installed)")
	}
	return m.reader.SearchSummaries(templateFilename, query)
}

// extendedListFormsFromDisk is the walk-every-file fallback for when the reader is absent or errors.
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

// ExtendedLoadForm is the single-record analogue of ExtendedListForms; nil when the file is missing.
func (m *Manager) ExtendedLoadForm(templateFilename, datafile string) (*FormSummary, error) {
	// Read from the index (single source) when wired, so the per-record path
	// carries the same harvested ExpressionItems (including formulas) the list
	// path does. Disk is the fallback when no reader, mirroring ExtendedListForms.
	if m.reader != nil {
		s, ok, err := m.reader.LoadSummary(templateFilename, datafile)
		if err == nil {
			if !ok {
				return nil, nil
			}
			return &s, nil
		}
		m.log.Warn("storage: form reader LoadSummary failed, falling back to disk",
			"template", templateFilename, "file", datafile, "err", err)
	}
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
		if fld.Type == "facet" {
			// Value lives in meta.facets[key].Selected; harvest only when set with a non-empty Selected.
			if state, ok := f.Meta.Facets[fld.FacetKey]; ok && state.Set && state.Selected != "" {
				expressionItems[fld.Key] = state.Selected
			}
			continue
		}
		if v, ok := f.Data[fld.Key]; ok && v != nil && v != "" {
			expressionItems[fld.Key] = v
		}
	}
	// Expose each set facet under its own key too (F["facet-key"]), matching the
	// formula context and the index harvest.
	for k, st := range f.Meta.Facets {
		if st.Set && st.Selected != "" {
			expressionItems[k] = st.Selected
		}
	}
	// Formulas need a calculator (the composition root supplies it). On the disk
	// path this keeps F["formula"] resolving when the index isn't the source.
	if m.formulas != nil {
		for k, v := range m.formulas.FormulaValues(tpl, f) {
			if v != nil && v != "" {
				expressionItems[k] = v
			}
		}
	}
	// Scaling factors are nested under S so S["name"] resolves on the disk path.
	if m.scales != nil {
		if sv := m.scales.ScaleValues(tpl, f); len(sv) > 0 {
			expressionItems["S"] = sv
		}
	}
	return FormSummary{
		Filename:        filename,
		Meta:            f.Meta,
		Title:           title,
		ExpressionItems: expressionItems,
	}, true
}

func (m *Manager) templateDir(templateFilename string) string {
	name := strings.TrimSuffix(templateFilename, filepath.Ext(templateFilename))
	return filepath.Join(m.storageDir, name)
}

// TemplateStorageDir returns the absolute path of <storage>/<template-stem>/.
func (m *Manager) TemplateStorageDir(templateFilename string) string {
	return m.templateDir(templateFilename)
}

// LoadTemplate exposes the parsed template by filename. The dataprovider needs
// field types to route api columns: a facet field's value lives in meta.facets,
// not data, so the column resolver must know which keys are virtual facets.
func (m *Manager) LoadTemplate(name string) (*template.Template, error) {
	return m.templates.LoadTemplate(name)
}

// TemplateImageDir returns the absolute path of <storage>/<template>/images/.
func (m *Manager) TemplateImageDir(templateFilename string) string {
	return filepath.Join(m.templateDir(templateFilename), imagesDir)
}

// guidFieldKey returns the key of the template's guid-typed field, or "".
func guidFieldKey(fields []template.Field) string {
	for _, f := range fields {
		if f.Type == "guid" {
			return f.Key
		}
	}
	return ""
}

// MaxFieldValue returns the greatest scalar numeric value for fieldKey across
// the collection, best-effort over the index reader (no reader or an error
// yields ok=false, treated by callers as "no siblings yet"). Mirrors
// guidCollides: an index read must never block the caller.
func (m *Manager) MaxFieldValue(templateFilename, fieldKey string) (float64, bool) {
	if m.reader == nil {
		return 0, false
	}
	v, ok, err := m.reader.MaxValue(templateFilename, fieldKey)
	if err != nil {
		return 0, false
	}
	return v, ok
}

// guidCollides reports whether guid already belongs to a record other than
// datafile in this collection. Best-effort over the index reader: a missing or
// erroring reader returns false, so a uniqueness check never blocks a save.
func (m *Manager) guidCollides(templateFilename, datafile, guid string) bool {
	if m.reader == nil {
		return false
	}
	sums, err := m.reader.ListSummaries(templateFilename)
	if err != nil {
		return false
	}
	for _, s := range sums {
		if s.Meta.ID == guid && s.Filename != datafile {
			return true
		}
	}
	return false
}

func (m *Manager) fieldsFor(templateFilename string) []template.Field {
	tpl, err := m.templates.LoadTemplate(templateFilename)
	if err != nil || tpl == nil {
		return nil
	}
	return tpl.Fields
}
