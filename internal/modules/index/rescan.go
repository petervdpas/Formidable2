package index

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// RescanTemplate force-reindexes one template's collection regardless of mtime, re-deriving every item's
// rows and dropping rows for items no longer on disk. Unlike RescanAll it ignores mtime (so it picks up
// projection-logic changes). A missing template is a delete; per-item failures accumulate, never aborting.
func (h *EventHandler) RescanTemplate(ctx context.Context, templateFilename string) error {
	if h.root == "" {
		return errors.New("index: RescanTemplate: root not set (call SetRoot)")
	}

	tplPath := filepath.Join(h.root, "templates", templateFilename)
	info, err := os.Stat(tplPath)
	if errors.Is(err, fs.ErrNotExist) {
		return h.OnTemplateDeleted(templateFilename)
	}
	if err != nil {
		return fmt.Errorf("index: stat template %q: %w", templateFilename, err)
	}

	tplRow, err := h.loadTemplateRow(FileEntry{
		Filename: templateFilename,
		Mtime:    info.ModTime().UnixNano(),
		Size:     info.Size(),
	})
	if err != nil {
		return err
	}
	batch := ReconcileBatch{UpsertTemplates: []TemplateRow{tplRow}}
	var loadErrs []error

	stem := strings.TrimSuffix(templateFilename, ".yaml")
	diskForms, err := listFilesBySuffix(filepath.Join(h.root, "storage", stem), ".meta.json")
	if err != nil {
		return fmt.Errorf("index: scan storage for %q: %w", templateFilename, err)
	}
	onDisk := make(map[string]bool, len(diskForms))
	for _, e := range diskForms {
		onDisk[e.Filename] = true
		row, err := h.loadFormRow(templateFilename, e)
		if err != nil {
			loadErrs = append(loadErrs, err)
			continue
		}
		batch.UpsertForms = append(batch.UpsertForms, row)
	}

	indexed, err := h.listIndexedFormFiles(templateFilename)
	if err != nil {
		return fmt.Errorf("index: list indexed forms for %q: %w", templateFilename, err)
	}
	for _, name := range indexed {
		if !onDisk[name] {
			batch.DeleteForms = append(batch.DeleteForms, FormRef{Template: templateFilename, Filename: name})
		}
	}

	if err := Reconcile(h.m.DB(), batch); err != nil {
		loadErrs = append(loadErrs, err)
	}
	return errors.Join(loadErrs...)
}

// RescanAll diffs disk under h.root against the index and applies the (added, changed, removed) sets in one
// transaction. It's the recovery path after sync or an external editor; an empty diff is a no-op (no rev bump).
// Per-file load failures accumulate and return joined, never aborting the rest of the rebuild.
func (h *EventHandler) RescanAll(ctx context.Context) error {
	if h.root == "" {
		return errors.New("index: RescanAll: root not set (call SetRoot)")
	}

	disk, err := scanDisk(h.root)
	if err != nil {
		return fmt.Errorf("index: scan disk: %w", err)
	}
	idx, err := scanIndexState(h.m.DB())
	if err != nil {
		return fmt.Errorf("index: scan db: %w", err)
	}

	batch := ReconcileBatch{}
	var loadErrs []error

	tplDiff := diffEntries(disk.Templates, idx.templates)
	for _, e := range tplDiff.Added {
		row, err := h.loadTemplateRow(e)
		if err != nil {
			loadErrs = append(loadErrs, err)
			continue
		}
		batch.UpsertTemplates = append(batch.UpsertTemplates, row)
	}
	for _, e := range tplDiff.Changed {
		row, err := h.loadTemplateRow(e)
		if err != nil {
			loadErrs = append(loadErrs, err)
			continue
		}
		batch.UpsertTemplates = append(batch.UpsertTemplates, row)
	}
	batch.DeleteTemplates = append(batch.DeleteTemplates, tplDiff.Removed...)

	// Skip orphan storage dirs (no template on disk): their rows would FK-violate and the cascade handles cleanup.
	tplFilenamesOnDisk := make(map[string]bool, len(disk.Templates))
	for _, e := range disk.Templates {
		tplFilenamesOnDisk[e.Filename] = true
	}

	allStems := mergedStems(disk.Forms, idx.forms)
	for _, stem := range allStems {
		tplFilename := stem + ".yaml"
		diskBucket := disk.Forms[stem]
		idxBucket := idx.forms[stem]

		if !tplFilenamesOnDisk[tplFilename] {
			continue
		}

		formDiff := diffEntries(diskBucket, idxBucket)
		for _, e := range formDiff.Added {
			row, err := h.loadFormRow(tplFilename, e)
			if err != nil {
				loadErrs = append(loadErrs, err)
				continue
			}
			batch.UpsertForms = append(batch.UpsertForms, row)
		}
		for _, e := range formDiff.Changed {
			row, err := h.loadFormRow(tplFilename, e)
			if err != nil {
				loadErrs = append(loadErrs, err)
				continue
			}
			batch.UpsertForms = append(batch.UpsertForms, row)
		}
		for _, name := range formDiff.Removed {
			batch.DeleteForms = append(batch.DeleteForms,
				FormRef{Template: tplFilename, Filename: name})
		}
	}

	allImageStems := mergedStems(disk.Images, idx.images)
	for _, stem := range allImageStems {
		tplFilename := stem + ".yaml"
		if !tplFilenamesOnDisk[tplFilename] {
			continue
		}
		imgDiff := diffEntries(disk.Images[stem], idx.images[stem])
		for _, e := range imgDiff.Added {
			batch.UpsertImages = append(batch.UpsertImages,
				ImageRow{Template: tplFilename, Filename: e.Filename, Mtime: e.Mtime, Size: e.Size})
		}
		for _, e := range imgDiff.Changed {
			batch.UpsertImages = append(batch.UpsertImages,
				ImageRow{Template: tplFilename, Filename: e.Filename, Mtime: e.Mtime, Size: e.Size})
		}
		for _, name := range imgDiff.Removed {
			batch.DeleteImages = append(batch.DeleteImages,
				ImageRef{Template: tplFilename, Filename: name})
		}
	}

	if err := Reconcile(h.m.DB(), batch); err != nil {
		loadErrs = append(loadErrs, err)
	}
	return errors.Join(loadErrs...)
}

// loadTemplateRow loads and projects a template into a TemplateRow; mtime/size come from the disk scan
// (not the loader) so the stored row matches the next stale-detect's stat().
func (h *EventHandler) loadTemplateRow(entry FileEntry) (TemplateRow, error) {
	rec, err := h.templates.LoadTemplate(entry.Filename)
	if err != nil {
		return TemplateRow{}, fmt.Errorf("index: load template %q: %w", entry.Filename, err)
	}
	if rec == nil || rec.Template == nil {
		return TemplateRow{}, fmt.Errorf("index: template %q loader returned nil", entry.Filename)
	}
	row := buildTemplateRow(rec.Template, entry.Mtime, entry.Filename)
	row.Size = entry.Size
	return row, nil
}

// loadFormRow loads the template (for guid/tags/item field) and form data, projecting them into a FormRow.
func (h *EventHandler) loadFormRow(templateFilename string, entry FileEntry) (FormRow, error) {
	tplRec, err := h.templates.LoadTemplate(templateFilename)
	if err != nil {
		return FormRow{}, fmt.Errorf("index: load template %q: %w", templateFilename, err)
	}
	formRec, err := h.forms.LoadForm(templateFilename, entry.Filename)
	if err != nil {
		return FormRow{}, fmt.Errorf("index: load form %q/%q: %w", templateFilename, entry.Filename, err)
	}
	if tplRec == nil || tplRec.Template == nil || formRec == nil || formRec.Form == nil {
		return FormRow{}, fmt.Errorf("index: nil load result for %q/%q", templateFilename, entry.Filename)
	}
	row := buildFormRow(tplRec.Template, formRec.Form, templateFilename, entry.Filename, entry.Mtime)
	row.Size = entry.Size
	return row, nil
}

// indexState mirrors ScanResult but is read from the SQLite index; used by RescanAll to compute the diff.
type indexState struct {
	templates []FileEntry
	forms     map[string][]FileEntry // template-stem -> entries
	images    map[string][]FileEntry
}

// scanIndexState reads (filename, mtime, size) for templates/forms/images, bucketing forms/images by stem.
func scanIndexState(db *sql.DB) (*indexState, error) {
	out := &indexState{
		forms:  map[string][]FileEntry{},
		images: map[string][]FileEntry{},
	}

	tpls, err := db.Query(`SELECT filename, mtime, size FROM templates`)
	if err != nil {
		return nil, err
	}
	for tpls.Next() {
		var name string
		var mtime, size int64
		if err := tpls.Scan(&name, &mtime, &size); err != nil {
			tpls.Close()
			return nil, err
		}
		out.templates = append(out.templates, FileEntry{Filename: name, Mtime: mtime, Size: size})
	}
	if err := tpls.Err(); err != nil {
		tpls.Close()
		return nil, err
	}
	tpls.Close()

	formRows, err := db.Query(`SELECT template, filename, mtime, size FROM forms`)
	if err != nil {
		return nil, err
	}
	for formRows.Next() {
		var tpl, file string
		var mtime, size int64
		if err := formRows.Scan(&tpl, &file, &mtime, &size); err != nil {
			formRows.Close()
			return nil, err
		}
		stem := strings.TrimSuffix(tpl, ".yaml")
		out.forms[stem] = append(out.forms[stem], FileEntry{Filename: file, Mtime: mtime, Size: size})
	}
	if err := formRows.Err(); err != nil {
		formRows.Close()
		return nil, err
	}
	formRows.Close()

	imgRows, err := db.Query(`SELECT template, filename, mtime, size FROM images`)
	if err != nil {
		return nil, err
	}
	for imgRows.Next() {
		var tpl, file string
		var mtime, size int64
		if err := imgRows.Scan(&tpl, &file, &mtime, &size); err != nil {
			imgRows.Close()
			return nil, err
		}
		stem := strings.TrimSuffix(tpl, ".yaml")
		out.images[stem] = append(out.images[stem], FileEntry{Filename: file, Mtime: mtime, Size: size})
	}
	if err := imgRows.Err(); err != nil {
		imgRows.Close()
		return nil, err
	}
	imgRows.Close()

	return out, nil
}

// mergedStems returns the union of map keys from two stem→entries
// maps so we iterate every stem that appears in either side of the
// diff. Order doesn't matter for correctness.
func mergedStems(a, b map[string][]FileEntry) []string {
	seen := make(map[string]struct{}, len(a)+len(b))
	for k := range a {
		seen[k] = struct{}{}
	}
	for k := range b {
		seen[k] = struct{}{}
	}
	out := make([]string, 0, len(seen))
	for k := range seen {
		out = append(out, k)
	}
	return out
}
