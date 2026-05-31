package index

import (
	"database/sql"
	"fmt"
)

// Reconcile applies a ReconcileBatch atomically: either everything lands and meta.rev bumps once, or
// nothing does. An empty batch is a no-op (no rev bump). Tag/facet/value/search rows re-sync inside form upsert.
func Reconcile(db *sql.DB, batch ReconcileBatch) error {
	if isEmpty(batch) {
		return nil
	}
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("index: begin: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck // Commit clears it

	if err := upsertTemplates(tx, batch.UpsertTemplates); err != nil {
		return err
	}
	if err := upsertImages(tx, batch.UpsertImages); err != nil {
		return err
	}
	if err := upsertFormsWithChildren(tx, batch.UpsertForms); err != nil {
		return err
	}
	var deleted int64
	if n, err := deleteForms(tx, batch.DeleteForms); err != nil {
		return err
	} else {
		deleted += n
	}
	if n, err := deleteImages(tx, batch.DeleteImages); err != nil {
		return err
	} else {
		deleted += n
	}
	if n, err := deleteTemplates(tx, batch.DeleteTemplates); err != nil {
		return err
	} else {
		deleted += n
	}
	// Upserts always write; deletes only count when they hit a row. A batch of
	// deletes that matched nothing must not churn the ETag.
	hasUpserts := len(batch.UpsertTemplates) > 0 ||
		len(batch.UpsertImages) > 0 ||
		len(batch.UpsertForms) > 0
	if hasUpserts || deleted > 0 {
		if err := bumpRev(tx); err != nil {
			return err
		}
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

func upsertFormsWithChildren(tx *sql.Tx, rows []FormRow) error {
	if len(rows) == 0 {
		return nil
	}
	formStmt, err := tx.Prepare(`
		INSERT INTO forms
		    (template, filename, id, title, fm_title,
		     created, created_name, created_email,
		     updated, updated_name, updated_email,
		     expression_items, rev, mtime, size)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?,
		        COALESCE((SELECT rev FROM forms WHERE template = ? AND filename = ?), 0) + 1,
		        ?, ?)
		ON CONFLICT(template, filename) DO UPDATE SET
		    id = excluded.id,
		    title = excluded.title,
		    fm_title = excluded.fm_title,
		    created = excluded.created,
		    created_name = excluded.created_name,
		    created_email = excluded.created_email,
		    updated = excluded.updated,
		    updated_name = excluded.updated_name,
		    updated_email = excluded.updated_email,
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

	clearFacets, err := tx.Prepare(`DELETE FROM form_facets WHERE template = ? AND filename = ?`)
	if err != nil {
		return fmt.Errorf("index: prepare clear facets: %w", err)
	}
	defer clearFacets.Close()

	insertFacet, err := tx.Prepare(`
		INSERT INTO form_facets (template, filename, facet_key, set_flag, selected)
		VALUES (?, ?, ?, ?, ?)
	`)
	if err != nil {
		return fmt.Errorf("index: prepare insert facet: %w", err)
	}
	defer insertFacet.Close()

	clearValues, err := tx.Prepare(`DELETE FROM form_values WHERE template = ? AND filename = ?`)
	if err != nil {
		return fmt.Errorf("index: prepare clear values: %w", err)
	}
	defer clearValues.Close()

	insertValue, err := tx.Prepare(`
		INSERT INTO form_values (template, filename, field_key, col, value_type, num_value, text_value)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return fmt.Errorf("index: prepare insert value: %w", err)
	}
	defer insertValue.Close()

	clearSearch, err := tx.Prepare(`DELETE FROM form_search WHERE template = ? AND filename = ?`)
	if err != nil {
		return fmt.Errorf("index: prepare clear search: %w", err)
	}
	defer clearSearch.Close()

	insertSearch, err := tx.Prepare(`
		INSERT INTO form_search (template, filename, title, body)
		VALUES (?, ?, ?, ?)
	`)
	if err != nil {
		return fmt.Errorf("index: prepare insert search: %w", err)
	}
	defer insertSearch.Close()

	for _, r := range rows {
		if _, err := formStmt.Exec(
			r.Template, r.Filename, r.ID, r.Title, r.FmTitle,
			r.Created, r.CreatedName, r.CreatedEmail,
			r.Updated, r.UpdatedName, r.UpdatedEmail,
			r.ExpressionItems,
			r.Template, r.Filename, r.Mtime, r.Size,
		); err != nil {
			return fmt.Errorf("index: upsert form %q/%q: %w", r.Template, r.Filename, err)
		}

		// Tag re-sync: replace-all rather than diff (cheap, and the caller carries no diff burden).
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

		// Facet re-sync, replace-all (so a stale facet set from a previous save disappears).
		if _, err := clearFacets.Exec(r.Template, r.Filename); err != nil {
			return fmt.Errorf("index: clear facets for %q/%q: %w", r.Template, r.Filename, err)
		}
		for _, ff := range r.Facets {
			if ff.Key == "" {
				continue
			}
			if _, err := insertFacet.Exec(r.Template, r.Filename, ff.Key, boolToInt(ff.Set), ff.Selected); err != nil {
				return fmt.Errorf("index: insert facet %q for %q/%q: %w", ff.Key, r.Template, r.Filename, err)
			}
		}

		// Value re-sync, replace-all. col/num are nullable pointers mapping to SQL NULL for scalars / non-numeric cells.
		if _, err := clearValues.Exec(r.Template, r.Filename); err != nil {
			return fmt.Errorf("index: clear values for %q/%q: %w", r.Template, r.Filename, err)
		}
		for _, v := range r.Values {
			if v.FieldKey == "" {
				continue
			}
			if _, err := insertValue.Exec(
				r.Template, r.Filename, v.FieldKey, v.Col, v.ValueType, v.Num, v.Text,
			); err != nil {
				return fmt.Errorf("index: insert value %q for %q/%q: %w", v.FieldKey, r.Template, r.Filename, err)
			}
		}

		// Search re-sync via direct DELETE+INSERT on form_search, which fires the triggers that keep FTS5 in step.
		if _, err := clearSearch.Exec(r.Template, r.Filename); err != nil {
			return fmt.Errorf("index: clear search for %q/%q: %w", r.Template, r.Filename, err)
		}
		if _, err := insertSearch.Exec(r.Template, r.Filename, r.Title, r.SearchBody); err != nil {
			return fmt.Errorf("index: insert search for %q/%q: %w", r.Template, r.Filename, err)
		}
	}
	return nil
}

// deleteForms returns the number of form rows actually removed (zero when the
// refs matched nothing), so the caller can skip a no-op rev bump.
func deleteForms(tx *sql.Tx, refs []FormRef) (int64, error) {
	if len(refs) == 0 {
		return 0, nil
	}
	// Drop the search row with a direct DELETE so its trigger fires (a bare FK cascade isn't guaranteed to).
	clearSearch, err := tx.Prepare(`DELETE FROM form_search WHERE template = ? AND filename = ?`)
	if err != nil {
		return 0, fmt.Errorf("index: prepare delete search: %w", err)
	}
	defer clearSearch.Close()

	stmt, err := tx.Prepare(`DELETE FROM forms WHERE template = ? AND filename = ?`)
	if err != nil {
		return 0, fmt.Errorf("index: prepare delete form: %w", err)
	}
	defer stmt.Close()
	var affected int64
	for _, r := range refs {
		if _, err := clearSearch.Exec(r.Template, r.Filename); err != nil {
			return 0, fmt.Errorf("index: clear search %q/%q: %w", r.Template, r.Filename, err)
		}
		res, err := stmt.Exec(r.Template, r.Filename)
		if err != nil {
			return 0, fmt.Errorf("index: delete form %q/%q: %w", r.Template, r.Filename, err)
		}
		affected += rowsAffected(res)
	}
	return affected, nil
}

func deleteImages(tx *sql.Tx, refs []ImageRef) (int64, error) {
	if len(refs) == 0 {
		return 0, nil
	}
	stmt, err := tx.Prepare(`DELETE FROM images WHERE template = ? AND filename = ?`)
	if err != nil {
		return 0, fmt.Errorf("index: prepare delete image: %w", err)
	}
	defer stmt.Close()
	var affected int64
	for _, r := range refs {
		res, err := stmt.Exec(r.Template, r.Filename)
		if err != nil {
			return 0, fmt.Errorf("index: delete image %q/%q: %w", r.Template, r.Filename, err)
		}
		affected += rowsAffected(res)
	}
	return affected, nil
}

func deleteTemplates(tx *sql.Tx, names []string) (int64, error) {
	if len(names) == 0 {
		return 0, nil
	}
	stmt, err := tx.Prepare(`DELETE FROM templates WHERE filename = ?`)
	if err != nil {
		return 0, fmt.Errorf("index: prepare delete template: %w", err)
	}
	defer stmt.Close()
	var affected int64
	for _, name := range names {
		res, err := stmt.Exec(name)
		if err != nil {
			return 0, fmt.Errorf("index: delete template %q: %w", name, err)
		}
		affected += rowsAffected(res)
	}
	return affected, nil
}

// rowsAffected reads RowsAffected, treating a driver that cannot report it as
// "something changed" so a real delete is never silently skipped.
func rowsAffected(res sql.Result) int64 {
	n, err := res.RowsAffected()
	if err != nil {
		return 1
	}
	return n
}

// bumpRev increments meta.rev (missing row treated as 0); the HTTP layer publishes it as an ETag.
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
