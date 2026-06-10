package storage

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/petervdpas/formidable2/internal/modules/template"
)

// A template with a guid field ("id") plus a plain text field.
func guidHealStack(t *testing.T) (*Manager, string) {
	t.Helper()
	m, _, tplM, root := newTestStack(t)
	if err := tplM.SaveTemplate("g.yaml", &template.Template{
		Name: "g", Filename: "g.yaml",
		Fields: []template.Field{
			{Key: "id", Type: "guid"},
			{Key: "title", Type: "text"},
		},
	}); err != nil {
		t.Fatalf("seed template: %v", err)
	}
	return m, root
}

// seedRaw stages an on-disk envelope verbatim (SaveFormExact does not sanitize or
// mint), so we can reproduce guid-less and drifted disk states the heal must fix.
func seedRaw(t *testing.T, m *Manager, df, dataID, metaID, updatedAt string) {
	t.Helper()
	data := map[string]any{"title": "hello"}
	if dataID != "" {
		data["id"] = dataID
	}
	r := m.SaveFormExact(context.Background(), "g.yaml", df, Form{
		Meta: FormMeta{ID: metaID, Updated: AuditEntry{At: updatedAt, Name: "Seed"}},
		Data: data,
	})
	if !r.Success {
		t.Fatalf("seed %s: %+v", df, r)
	}
}

func diskFile(t *testing.T, m *Manager, root, stem, df string) string {
	t.Helper()
	return filepath.Join(root, m.StorageDir(), stem, df+".meta.json")
}

func readFile(t *testing.T, path string) []byte {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return b
}

// The lifecycle the user specified: a record under a freshly added guid field has
// no id; the first load mints one into data, mirrors it to meta, and persists it.
func TestLoadForm_MintsIntoDataMirrorsToMetaAndPersists(t *testing.T) {
	m, _ := guidHealStack(t)
	seedRaw(t, m, "r1.json", "", "", "")

	out := m.LoadForm("g.yaml", "r1.json")
	if out == nil || out.Meta.ID == "" {
		t.Fatalf("load minted no id: %+v", out)
	}
	if out.Data["id"] != out.Meta.ID {
		t.Errorf("data id %v != meta id %v (data leads, meta mirrors)", out.Data["id"], out.Meta.ID)
	}

	// Persisted to disk, not just in memory.
	raw := m.LoadFormRaw("g.yaml", "r1.json")
	if raw.Meta.ID != out.Meta.ID || raw.Data["id"] != out.Meta.ID {
		t.Errorf("not persisted: disk meta.id=%q data.id=%v, want %q", raw.Meta.ID, raw.Data["id"], out.Meta.ID)
	}
}

// THE regression test for the drift bug: a guid-less record must resolve to the
// SAME id on every load. Before the heal, each load minted a fresh random uuid,
// so api references and relation edges latched onto throwaway guids that drifted.
func TestLoadForm_GuidStableAcrossLoads(t *testing.T) {
	m, _ := guidHealStack(t)
	seedRaw(t, m, "r1.json", "", "", "")

	first := m.LoadForm("g.yaml", "r1.json").Meta.ID
	second := m.LoadForm("g.yaml", "r1.json").Meta.ID
	third := m.LoadForm("g.yaml", "r1.json").Meta.ID
	if first == "" {
		t.Fatal("no id minted")
	}
	if first != second || second != third {
		t.Errorf("guid drifted across loads: %q, %q, %q", first, second, third)
	}
}

// Data is the source of truth: if data carries an id but meta is empty (drift),
// meta copies the data id rather than minting a new one.
func TestLoadForm_DataIdLeads_MetaCopies(t *testing.T) {
	m, _ := guidHealStack(t)
	seedRaw(t, m, "r1.json", "DATA-X", "", "")

	out := m.LoadForm("g.yaml", "r1.json")
	if out.Meta.ID != "DATA-X" {
		t.Errorf("meta.id = %q, want DATA-X (data leads)", out.Meta.ID)
	}
	raw := m.LoadFormRaw("g.yaml", "r1.json")
	if raw.Meta.ID != "DATA-X" || raw.Data["id"] != "DATA-X" {
		t.Errorf("not synced on disk: meta.id=%q data.id=%v", raw.Meta.ID, raw.Data["id"])
	}
}

// The inverse drift: data empty, meta carries an id. Sanitize falls back to meta,
// so data is filled from it and both end up equal on disk.
func TestLoadForm_MetaOnlyId_FillsData(t *testing.T) {
	m, _ := guidHealStack(t)
	seedRaw(t, m, "r1.json", "", "META-Y", "")

	out := m.LoadForm("g.yaml", "r1.json")
	if out.Meta.ID != "META-Y" || out.Data["id"] != "META-Y" {
		t.Errorf("meta-only id not propagated: meta=%q data=%v", out.Meta.ID, out.Data["id"])
	}
	raw := m.LoadFormRaw("g.yaml", "r1.json")
	if raw.Data["id"] != "META-Y" {
		t.Errorf("disk data.id = %v, want META-Y", raw.Data["id"])
	}
}

// "Never touch it ever again": once data.id == meta.id, loading must not rewrite
// the file (no guid change, no mtime churn that would spam git and the indexer).
func TestLoadForm_SyncedGuid_NotRewritten(t *testing.T) {
	m, root := guidHealStack(t)
	seedRaw(t, m, "r1.json", "SAME-Z", "SAME-Z", "2025-01-01T00:00:00Z")
	path := diskFile(t, m, root, "g", "r1.json")
	before := readFile(t, path)
	beforeMtime := statMtime(t, path)

	time.Sleep(10 * time.Millisecond) // any write would land a later mtime

	out := m.LoadForm("g.yaml", "r1.json")
	if out.Meta.ID != "SAME-Z" {
		t.Errorf("guid changed: %q, want SAME-Z", out.Meta.ID)
	}
	after := readFile(t, path)
	if string(before) != string(after) {
		t.Errorf("synced record was rewritten on load:\nbefore=%s\nafter=%s", before, after)
	}
	if got := statMtime(t, path); !got.Equal(beforeMtime) {
		t.Errorf("file mtime changed (%v -> %v): a synced record must not be rewritten", beforeMtime, got)
	}
}

// Healing a record must not bump the Updated audit stamp: it is a structural id
// fix, not a user edit.
func TestLoadForm_HealPreservesUpdatedStamp(t *testing.T) {
	m, _ := guidHealStack(t)
	seedRaw(t, m, "r1.json", "", "", "2024-07-01T12:00:00Z")

	m.LoadForm("g.yaml", "r1.json") // heals + persists
	raw := m.LoadFormRaw("g.yaml", "r1.json")
	if raw.Meta.ID == "" {
		t.Fatal("not healed")
	}
	reloaded := m.LoadForm("g.yaml", "r1.json")
	if reloaded.Meta.Updated.At != "2024-07-01T12:00:00Z" {
		t.Errorf("Updated stamp changed by heal: %q, want 2024-07-01T12:00:00Z", reloaded.Meta.Updated.At)
	}
}

// A template without a guid field never heals and never rewrites on load.
func TestLoadForm_NoGuidField_NoHeal(t *testing.T) {
	m, _, tplM, root := newTestStack(t)
	if err := tplM.SaveTemplate("p.yaml", &template.Template{
		Name: "p", Filename: "p.yaml",
		Fields: []template.Field{{Key: "title", Type: "text"}},
	}); err != nil {
		t.Fatalf("seed template: %v", err)
	}
	r := m.SaveFormExact(context.Background(), "p.yaml", "r1.json", Form{
		Meta: FormMeta{ID: ""},
		Data: map[string]any{"title": "hello"},
	})
	if !r.Success {
		t.Fatalf("seed: %+v", r)
	}
	path := filepath.Join(root, m.StorageDir(), "p", "r1.json.meta.json")
	before := readFile(t, path)

	out := m.LoadForm("p.yaml", "r1.json")
	if out.Meta.ID != "" {
		t.Errorf("guid minted without a guid field: %q", out.Meta.ID)
	}
	if string(before) != string(readFile(t, path)) {
		t.Error("record without a guid field was rewritten on load")
	}
}

func statMtime(t *testing.T, path string) time.Time {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat %s: %v", path, err)
	}
	return info.ModTime()
}
