package index

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/cucumber/godog"
	"github.com/petervdpas/formidable2/internal/modules/storage"
	"github.com/petervdpas/formidable2/internal/modules/template"
)

func TestFeatures(t *testing.T) {
	suite := godog.TestSuite{
		ScenarioInitializer: initIndexScenario,
		Options: &godog.Options{
			Format:   "pretty",
			Paths:    []string{"features"},
			TestingT: t,
		},
	}
	if suite.Run() != 0 {
		t.Fatal("non-zero status returned, failed to run feature tests")
	}
}

// indexProfile keeps everything one profile owns: its on-disk root,
// its DB path, the live Manager, the EventHandler, and the canned
// loader maps the test populates as templates/forms get added.
//
// Switching profiles in a scenario rotates the active *indexProfile.
type indexProfile struct {
	root  string
	db    string
	mgr   *Manager
	hand  *EventHandler
	tpls  map[string]*TemplateRecord
	forms map[string]*FormRecord
}

func newIndexProfile(rootBase, dbBase string) (*indexProfile, error) {
	root, err := os.MkdirTemp(rootBase, "ctx-")
	if err != nil {
		return nil, err
	}
	dbDir, err := os.MkdirTemp(dbBase, "db-")
	if err != nil {
		return nil, err
	}
	dbPath := filepath.Join(dbDir, "i.db")
	m, err := NewManager(dbPath)
	if err != nil {
		return nil, err
	}
	tpls := map[string]*TemplateRecord{}
	forms := map[string]*FormRecord{}
	h := NewEventHandler(m,
		&fakeTemplateLoader{tpls: tpls},
		&fakeFormStore{forms: forms},
	)
	h.SetRoot(root)
	return &indexProfile{
		root: root, db: dbPath, mgr: m, hand: h,
		tpls: tpls, forms: forms,
	}, nil
}

// indexWorld holds the per-scenario state, including the registry of
// remembered profiles for "switch back to X" scenarios.
type indexWorld struct {
	rootBase  string
	dbBase    string
	active    *indexProfile
	remembered map[string]*indexProfile
	lastErr   error // populated by tolerant-rescan steps for later assertions
}

func initIndexScenario(ctx *godog.ScenarioContext) {
	w := &indexWorld{remembered: map[string]*indexProfile{}}

	ctx.Before(func(c context.Context, _ *godog.Scenario) (context.Context, error) {
		base, err := os.MkdirTemp("", "index-godog-")
		if err != nil {
			return c, err
		}
		w.rootBase = filepath.Join(base, "ctx")
		w.dbBase = filepath.Join(base, "db")
		if err := os.MkdirAll(w.rootBase, 0o755); err != nil {
			return c, err
		}
		if err := os.MkdirAll(w.dbBase, 0o755); err != nil {
			return c, err
		}
		w.active = nil
		w.remembered = map[string]*indexProfile{}
		w.lastErr = nil
		return c, nil
	})

	ctx.After(func(c context.Context, _ *godog.Scenario, _ error) (context.Context, error) {
		if w.active != nil {
			_ = w.active.mgr.Close()
		}
		for _, p := range w.remembered {
			if p != w.active {
				_ = p.mgr.Close()
			}
		}
		_ = os.RemoveAll(w.rootBase)
		_ = os.RemoveAll(w.dbBase)
		return c, nil
	})

	// ── Setup steps ───────────────────────────────────────────────

	ctx.Step(`^a fresh index for a temp profile$`, func() error {
		p, err := newIndexProfile(w.rootBase, w.dbBase)
		if err != nil {
			return err
		}
		w.active = p
		return nil
	})

	ctx.Step(`^I switch to a fresh profile$`, func() error {
		// "Drain" the active profile by closing its handle. Any future
		// reads on the old *Manager will error gracefully (see
		// TestClosedManager_ErrorsGracefully). New profile gets a fresh
		// root, fresh DB, fresh loaders.
		if w.active != nil && !inRemembered(w, w.active) {
			_ = w.active.mgr.Close()
		}
		p, err := newIndexProfile(w.rootBase, w.dbBase)
		if err != nil {
			return err
		}
		w.active = p
		return nil
	})

	ctx.Step(`^I remember the current profile as "([^"]*)"$`, func(name string) error {
		if w.active == nil {
			return fmt.Errorf("no active profile to remember")
		}
		w.remembered[name] = w.active
		return nil
	})

	ctx.Step(`^I switch back to profile "([^"]*)"$`, func(name string) error {
		p, ok := w.remembered[name]
		if !ok {
			return fmt.Errorf("no remembered profile %q", name)
		}
		// Close the current profile if it's not also remembered.
		if w.active != nil && w.active != p && !inRemembered(w, w.active) {
			_ = w.active.mgr.Close()
		}
		// The remembered profile's DB is still open from before; just
		// flip the pointer. (The active profile in this test setup
		// never had its DB closed because we only close on switch.)
		// If the remembered profile was implicitly closed, reopen.
		if err := pingMgr(p); err != nil {
			m, err := NewManager(p.db)
			if err != nil {
				return err
			}
			p.mgr = m
			p.hand = NewEventHandler(m,
				&fakeTemplateLoader{tpls: p.tpls},
				&fakeFormStore{forms: p.forms},
			)
			p.hand.SetRoot(p.root)
		}
		w.active = p
		return nil
	})

	// ── Disk + loader plumbing ────────────────────────────────────

	ctx.Step(`^a template "([^"]*)" on disk with fields:$`,
		func(filename string, table *godog.Table) error {
			fields := parseFieldsTable(table)
			path := filepath.Join(w.active.root, "templates", filename)
			if err := writeFile(path, "name: "+strings.TrimSuffix(filename, ".yaml")+"\n", time.Now().Unix()); err != nil {
				return err
			}
			rec := &TemplateRecord{
				Template: &template.Template{
					Name:             strings.TrimSuffix(filename, ".yaml"),
					Filename:         filename,
					MarkdownTemplate: "# x",
					Fields:           fields,
				},
				Mtime: time.Now().UnixNano(),
			}
			w.active.tpls[filename] = rec
			return nil
		})

	ctx.Step(`^the template "([^"]*)" is removed from disk$`, func(filename string) error {
		if err := os.Remove(filepath.Join(w.active.root, "templates", filename)); err != nil {
			return err
		}
		delete(w.active.tpls, filename)
		return nil
	})

	addOrRewriteForm := func(stem, datafile string, table *godog.Table, t time.Time) error {
		data := parseValuesTable(table)
		path := filepath.Join(w.active.root, "storage", stem, datafile)
		if err := writeFile(path, `{"meta":{}}`, t.Unix()); err != nil {
			return err
		}
		w.active.forms[stem+".yaml/"+datafile] = &FormRecord{
			Form:  &storage.Form{Meta: storage.FormMeta{}, Data: data},
			Mtime: t.UnixNano(),
		}
		return nil
	}

	ctx.Step(`^a form "([^"]*)" under "([^"]*)" with values:$`,
		func(datafile, tplFilename string, table *godog.Table) error {
			stem := strings.TrimSuffix(tplFilename, ".yaml")
			return addOrRewriteForm(stem, datafile, table, time.Now())
		})

	ctx.Step(`^the form "([^"]*)" under "([^"]*)" is rewritten with values:$`,
		func(datafile, tplFilename string, table *godog.Table) error {
			stem := strings.TrimSuffix(tplFilename, ".yaml")
			// Force a strictly later mtime so the diff sees "changed"
			// regardless of disk granularity on the host.
			return addOrRewriteForm(stem, datafile, table, time.Now().Add(2*time.Second))
		})

	ctx.Step(`^the form "([^"]*)" under "([^"]*)" is removed from disk$`,
		func(datafile, tplFilename string) error {
			stem := strings.TrimSuffix(tplFilename, ".yaml")
			if err := os.Remove(filepath.Join(w.active.root, "storage", stem, datafile)); err != nil {
				return err
			}
			delete(w.active.forms, tplFilename+"/"+datafile)
			return nil
		})

	// Drops a form file on disk WITHOUT registering it with the fake
	// loader, simulating a malformed *.meta.json (the real storage
	// manager returns nil on parse error, which the adapter surfaces
	// as a load error).
	ctx.Step(`^a malformed form "([^"]*)" exists under "([^"]*)"$`,
		func(datafile, tplFilename string) error {
			stem := strings.TrimSuffix(tplFilename, ".yaml")
			path := filepath.Join(w.active.root, "storage", stem, datafile)
			return writeFile(path, `not json`, time.Now().Unix())
		})

	// ── Action ───────────────────────────────────────────────────

	ctx.Step(`^I run RescanAll$`, func() error {
		return w.active.hand.RescanAll(context.Background())
	})

	// Run RescanAll but stash the error on the world instead of failing
	// the scenario - used by scenarios that expect a non-nil error
	// (malformed file) AND want to assert the rest of the index still
	// populated.
	ctx.Step(`^I run RescanAll tolerating load errors$`, func() error {
		w.lastErr = w.active.hand.RescanAll(context.Background())
		return nil
	})

	ctx.Step(`^the last RescanAll error mentions "([^"]*)"$`, func(needle string) error {
		if w.lastErr == nil {
			return fmt.Errorf("expected an error from RescanAll, got nil")
		}
		if !strings.Contains(w.lastErr.Error(), needle) {
			return fmt.Errorf("error %q does not mention %q", w.lastErr.Error(), needle)
		}
		return nil
	})

	// ── Assertions ───────────────────────────────────────────────

	ctx.Step(`^the index lists templates "([^"]*)"$`, func(csv string) error {
		want := splitCSV(csv)
		rows, err := w.active.mgr.ListTemplates()
		if err != nil {
			return err
		}
		got := make([]string, len(rows))
		for i, r := range rows {
			got[i] = r.Filename
		}
		sort.Strings(got)
		if !equalStrings(got, want) {
			return fmt.Errorf("templates = %v, want %v", got, want)
		}
		return nil
	})

	ctx.Step(`^the index has (\d+) templates$`, func(n int) error {
		rows, err := w.active.mgr.ListTemplates()
		if err != nil {
			return err
		}
		if len(rows) != n {
			return fmt.Errorf("templates = %d, want %d", len(rows), n)
		}
		return nil
	})

	ctx.Step(`^the index has (\d+) forms for template "([^"]*)"$`,
		func(n int, tpl string) error {
			rows, err := w.active.mgr.ListForms(tpl, QueryOpts{})
			if err != nil {
				return err
			}
			if len(rows) != n {
				return fmt.Errorf("forms for %q = %d, want %d", tpl, len(rows), n)
			}
			return nil
		})

	ctx.Step(`^form "([^"]*)" under "([^"]*)" has tags "([^"]*)"$`,
		func(datafile, tpl, csv string) error {
			row, ok, err := w.active.mgr.GetForm(tpl, datafile)
			if err != nil {
				return err
			}
			if !ok {
				return fmt.Errorf("form %q/%q not found", tpl, datafile)
			}
			got := append([]string(nil), row.Tags...)
			sort.Strings(got)
			want := splitCSV(csv)
			if !equalStrings(got, want) {
				return fmt.Errorf("tags = %v, want %v", got, want)
			}
			return nil
		})

	ctx.Step(`^the index rev is (\d+)$`, func(n int) error {
		got, err := w.active.mgr.Rev()
		if err != nil {
			return err
		}
		if int(got) != n {
			return fmt.Errorf("rev = %d, want %d", got, n)
		}
		return nil
	})
}

// ── small parsing helpers (godog-only) ────────────────────────────────

func parseFieldsTable(table *godog.Table) []template.Field {
	if table == nil || len(table.Rows) < 2 {
		return nil
	}
	out := make([]template.Field, 0, len(table.Rows)-1)
	for _, row := range table.Rows[1:] {
		f := template.Field{}
		if len(row.Cells) > 0 {
			f.Key = row.Cells[0].Value
		}
		if len(row.Cells) > 1 {
			f.Type = row.Cells[1].Value
		}
		out = append(out, f)
	}
	return out
}

func parseValuesTable(table *godog.Table) map[string]any {
	out := map[string]any{}
	if table == nil || len(table.Rows) < 2 {
		return out
	}
	for _, row := range table.Rows[1:] {
		if len(row.Cells) < 2 {
			continue
		}
		key := row.Cells[0].Value
		val := row.Cells[1].Value
		// CSV-as-list convention so the .feature stays readable.
		if strings.Contains(val, ",") {
			parts := strings.Split(val, ",")
			arr := make([]any, len(parts))
			for i, p := range parts {
				arr[i] = strings.TrimSpace(p)
			}
			out[key] = arr
		} else {
			out[key] = val
		}
	}
	return out
}

func splitCSV(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, len(parts))
	for i, p := range parts {
		out[i] = strings.TrimSpace(p)
	}
	sort.Strings(out)
	return out
}

func writeFile(path, content string, mtimeUnix int64) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return err
	}
	if mtimeUnix > 0 {
		t := time.Unix(mtimeUnix, 0)
		return os.Chtimes(path, t, t)
	}
	return nil
}

func inRemembered(w *indexWorld, p *indexProfile) bool {
	for _, r := range w.remembered {
		if r == p {
			return true
		}
	}
	return false
}

func pingMgr(p *indexProfile) error {
	_, err := p.mgr.Rev()
	return err
}
