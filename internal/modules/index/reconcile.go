package index

import (
	"database/sql"
	"fmt"
)

// Reconcile applies a ReconcileBatch atomically to db. Either every
// upsert/delete lands and meta.rev bumps once, or nothing does. An
// empty batch returns nil without bumping rev (callers can run a
// speculative diff and only commit when something actually moved).
//
// Tag synchronization happens *inside* form upsert: any prior
// (template, filename) tag rows are wiped, then the FormRow's current
// tag set is re-inserted. This keeps the inverted index in lock-step
// with the form row without the caller having to compute the diff.
func Reconcile(db *sql.DB, batch ReconcileBatch) error {
	if isEmpty(batch) {
		return nil
	}
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("index: begin: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck // Commit clears it; harmless on success

	if err := upsertTemplates(tx, batch.UpsertTemplates); err != nil {
		return err
	}
	if err := upsertImages(tx, batch.UpsertImages); err != nil {
		return err
	}
	if err := upsertFormsWithTags(tx, batch.UpsertForms); err != nil {
		return err
	}
	if err := deleteForms(tx, batch.DeleteForms); err != nil {
		return err
	}
	if err := deleteImages(tx, batch.DeleteImages); err != nil {
		return err
	}
	if err := deleteTemplates(tx, batch.DeleteTemplates); err != nil {
		return err
	}
	if err := bumpRev(tx); err != nil {
		return err
	}
	return tx.Commit()
}

func isEmpty(b ReconcileBatch) bool {
	return len(b.UpsertTemplates) == 0 &&
		len(b.DeleteTemplates) == 0 &&
		len(b.UpsertForms) == 0 &&
		len(b.DeleteForms) == 0 &&
		len(b.UpsertImages) == 0 &&
		len(b.DeleteImages) == 0
}

func upsertTemplates(tx *sql.Tx, rows []TemplateRow) error {
	if len(rows) == 0 {
		return nil
	}
	stmt, err := tx.Prepare(`
		INSERT INTO templates
		    (filename, name, item_field, guid_field, tags_field,
		     has_markdown_template, enable_collection, rev, mtime, size)
		VALUES (?, ?, ?, ?, ?, ?, ?, COALESCE((SELECT rev FROM templates WHERE filename = ?), 0) + 1, ?, ?)
		ON CONFLICT(filename) DO UPDATE SET
		    name = excluded.name,
		    item_field = excluded.item_field,
		    guid_field = excluded.guid_field,
		    tags_field = excluded.tags_field,
		    has_markdown_template = excluded.has_markdown_template,
		    enable_collection = excluded.enable_collection,
		    rev = templates.rev + 1,
		    mtime = excluded.mtime,
		    size = excluded.size
	`)
	if err != nil {
		return fmt.Errorf("index: prepare upsert template: %w", err)
	}
	defer stmt.Close()

	for _, r := range rows {
		if _, err := stmt.Exec(
			r.Filename, r.Name, r.ItemField, r.GuidField, r.TagsField,
			boolToInt(r.HasMarkdownTemplate), boolToInt(r.EnableCollection),
			r.Filename, r.Mtime, r.Size,
		); err != nil {
			return fmt.Errorf("index: upsert template %q: %w", r.Filename, err)
		}
	}
	return nil
}

func upsertImages(tx *sql.Tx, rows []ImageRow) error {
	if len(rows) == 0 {
		return nil
	}
	stmt, err := tx.Prepare(`
		INSERT INTO images (template, filename, mtime, size) VALUES (?, ?, ?, ?)
		ON CONFLICT(template, filename) DO UPDATE SET
		    mtime = excluded.mtime,
		    size = excluded.size
	`)
	if err != nil {
		return fmt.Errorf("index: prepare upsert image: %w", err)
	}
	defer stmt.Close()
	for _, r := range rows {
		if _, err := stmt.Exec(r.Template, r.Filename, r.Mtime, r.Size); err != nil {
			return fmt.Errorf("index: upsert image %q/%q: %w", r.Template, r.Filename, err)
		}
	}
	return nil
}

func upsertFormsWithTags(tx *sql.Tx, rows []FormRow) error {
	if len(rows) == 0 {
		return nil
	}
	formStmt, err := tx.Prepare(`
		INSERT INTO forms
		    (template, filename, id, title, fm_title, author,
		     created, updated, expression_items, rev, mtime, size)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?,
		        COALESCE((SELECT rev FROM forms WHERE template = ? AND filename = ?), 0) + 1,
		        ?, ?)
		ON CONFLICT(template, filename) DO UPDATE SET
		    id = excluded.id,
		    title = excluded.title,
		    fm_title = excluded.fm_title,
		    author = excluded.author,
		    created = excluded.created,
		    updated = excluded.updated,
		    expression_items = excluded.expression_items,
		    rev = forms.rev + 1,
		    mtime = excluded.mtime,
		    size = excluded.size
	`)
	if err != nil {
		return fmt.Errorf("index: prepare upsert form: %w", err)
	}
	defer formStmt.Close()

	clearTags, err := tx.Prepare(`DELETE FROM form_tags WHERE template = ? AND filename = ?`)
	if err != nil {
		return fmt.Errorf("index: prepare clear tags: %w", err)
	}
	defer clearTags.Close()

	insertTag, err := tx.Prepare(`INSERT INTO form_tags (template, filename, tag) VALUES (?, ?, ?)`)
	if err != nil {
		return fmt.Errorf("index: prepare insert tag: %w", err)
	}
	defer insertTag.Close()

	for _, r := range rows {
		if _, err := formStmt.Exec(
			r.Template, r.Filename, r.ID, r.Title, r.FmTitle, r.Author,
			r.Created, r.Updated, r.ExpressionItems,
			r.Template, r.Filename, r.Mtime, r.Size,
		); err != nil {
			return fmt.Errorf("index: upsert form %q/%q: %w", r.Template, r.Filename, err)
		}

		// Tag re-sync. Wipe whatever was there, re-insert the current
		// set. Cheap (a form rarely has more than a handful of tags)
		// and removes the diff burden from the caller.
		if _, err := clearTags.Exec(r.Template, r.Filename); err != nil {
			return fmt.Errorf("index: clear tags for %q/%q: %w", r.Template, r.Filename, err)
		}
		seen := make(map[string]struct{}, len(r.Tags))
		for _, tag := range r.Tags {
			if tag == "" {
				continue
			}
			if _, dup := seen[tag]; dup {
				continue
			}
			seen[tag] = struct{}{}
			if _, err := insertTag.Exec(r.Template, r.Filename, tag); err != nil {
				return fmt.Errorf("index: insert tag %q for %q/%q: %w", tag, r.Template, r.Filename, err)
			}
		}
	}
	return nil
}

func deleteForms(tx *sql.Tx, refs []FormRef) error {
	if len(refs) == 0 {
		return nil
	}
	stmt, err := tx.Prepare(`DELETE FROM forms WHERE template = ? AND filename = ?`)
	if err != nil {
		return fmt.Errorf("index: prepare delete form: %w", err)
	}
	defer stmt.Close()
	for _, r := range refs {
		if _, err := stmt.Exec(r.Template, r.Filename); err != nil {
			return fmt.Errorf("index: delete form %q/%q: %w", r.Template, r.Filename, err)
		}
	}
	return nil
}

func deleteImages(tx *sql.Tx, refs []ImageRef) error {
	if len(refs) == 0 {
		return nil
	}
	stmt, err := tx.Prepare(`DELETE FROM images WHERE template = ? AND filename = ?`)
	if err != nil {
		return fmt.Errorf("index: prepare delete image: %w", err)
	}
	defer stmt.Close()
	for _, r := range refs {
		if _, err := stmt.Exec(r.Template, r.Filename); err != nil {
			return fmt.Errorf("index: delete image %q/%q: %w", r.Template, r.Filename, err)
		}
	}
	return nil
}

func deleteTemplates(tx *sql.Tx, names []string) error {
	if len(names) == 0 {
		return nil
	}
	stmt, err := tx.Prepare(`DELETE FROM templates WHERE filename = ?`)
	if err != nil {
		return fmt.Errorf("index: prepare delete template: %w", err)
	}
	defer stmt.Close()
	for _, name := range names {
		if _, err := stmt.Exec(name); err != nil {
			return fmt.Errorf("index: delete template %q: %w", name, err)
		}
	}
	return nil
}

// bumpRev increments meta.rev by 1, treating a missing row as 0.
// The HTTP layer publishes this as an ETag, so a per-batch monotonic
// counter is exactly the semantic we want.
func bumpRev(tx *sql.Tx) error {
	_, err := tx.Exec(`
		INSERT INTO meta (key, value) VALUES ('rev', '1')
		ON CONFLICT(key) DO UPDATE SET value = CAST((CAST(meta.value AS INTEGER) + 1) AS TEXT)
	`)
	if err != nil {
		return fmt.Errorf("index: bump rev: %w", err)
	}
	return nil
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
