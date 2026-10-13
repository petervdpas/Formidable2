package plugin

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fakeTemplates stands in for the template store: have is what exists in the
// context folder, by file name and YAML.
type fakeTemplates struct {
	have    map[string]string
	creates int
}

func newFakeTemplates() *fakeTemplates { return &fakeTemplates{have: map[string]string{}} }

func (f *fakeTemplates) HasTemplate(name string) bool { _, ok := f.have[name]; return ok }

func (f *fakeTemplates) InspectTemplate(name string, yaml []byte) (TemplateInfo, error) {
	s := string(yaml)
	if strings.Contains(s, "INVALID") || strings.TrimSpace(s) == "" {
		return TemplateInfo{}, errors.New("template says INVALID")
	}
	info := TemplateInfo{Fields: strings.Count(s, "- key:")}
	for _, line := range strings.Split(s, "\n") {
		if n, ok := strings.CutPrefix(line, "name: "); ok {
			info.Name = n
		}
	}
	return info, nil
}

func (f *fakeTemplates) CreateTemplate(name string, yaml []byte) (bool, error) {
	if _, err := f.InspectTemplate(name, yaml); err != nil {
		return false, err
	}
	if f.HasTemplate(name) {
		return false, nil
	}
	f.creates++
	f.have[name] = string(yaml)
	return true, nil
}

func (f *fakeTemplates) TemplateSource(name string) (string, error) {
	src, ok := f.have[name]
	if !ok {
		return "", errors.New("template: file not found: " + name)
	}
	return src, nil
}

const heldYAML = "name: Invoice\nfields:\n  - key: number\n    type: text\n  - key: amount\n    type: number\n"

// templateManager returns a manager with one plugin "demo", its folder, the fake store and a count of notice signals.
func templateManager(t *testing.T, lua string) (*Manager, string, *fakeTemplates, *int) {
	t.Helper()
	m, pluginsDir := newTestManager(t)
	store := newFakeTemplates()
	fired := 0
	m.deps.Editor = kvTestFS{}
	m.deps.Templates = store
	m.deps.ClientNoticeOut = func() { fired++ }
	if lua == "" {
		lua = "function run() end"
	}
	dir := writePlugin(t, pluginsDir, "demo", clientManifest("demo", `[]`), lua)
	if err := m.Refresh(); err != nil {
		t.Fatal(err)
	}
	return m, dir, store, &fired
}

func TestSaveShippedTemplate_HoldsTheYAMLButCreatesNothing(t *testing.T) {
	m, dir, store, fired := templateManager(t, "")
	rows, err := m.SaveShippedTemplate("demo", "invoice.yaml", heldYAML)
	if err != nil {
		t.Fatalf("SaveShippedTemplate: %v", err)
	}
	got, err := os.ReadFile(filepath.Join(dir, "templates", "invoice.yaml"))
	if err != nil || string(got) != heldYAML {
		t.Fatalf("held copy = %q, %v", got, err)
	}
	want := ShippedTemplate{File: "invoice.yaml", Name: "Invoice", Fields: 2, Installed: false}
	if len(rows) != 1 || rows[0] != want {
		t.Errorf("rows = %+v, want %+v", rows, want)
	}
	if store.creates != 0 || *fired != 0 {
		t.Error("saving into the plugin created a template in the context folder")
	}
}

func TestCreateShippedTemplate_CreatesOnceAndNeverReplaces(t *testing.T) {
	m, _, store, _ := templateManager(t, "")
	if _, err := m.SaveShippedTemplate("demo", "invoice.yaml", heldYAML); err != nil {
		t.Fatal(err)
	}
	rows, err := m.CreateShippedTemplate("demo", "invoice.yaml")
	if err != nil || len(rows) != 1 || !rows[0].Installed {
		t.Fatalf("rows = %+v, err = %v", rows, err)
	}
	if store.have["invoice.yaml"] != heldYAML {
		t.Errorf("created template = %q", store.have["invoice.yaml"])
	}

	// The user edits the template; a newer copy in the plugin must not undo that.
	store.have["invoice.yaml"] = "name: Edited by the user\n"
	if _, err := m.SaveShippedTemplate("demo", "invoice.yaml", heldYAML+"  - key: extra\n    type: text\n"); err != nil {
		t.Fatal(err)
	}
	if _, err := m.CreateShippedTemplate("demo", "invoice.yaml"); err != nil {
		t.Fatal(err)
	}
	if store.have["invoice.yaml"] != "name: Edited by the user\n" || store.creates != 1 {
		t.Errorf("an existing template was replaced: %q, creates = %d", store.have["invoice.yaml"], store.creates)
	}
}

func TestAddShippedTemplateFrom_CopiesAnExistingTemplateIntoThePlugin(t *testing.T) {
	m, dir, store, _ := templateManager(t, "")
	store.have["invoice.yaml"] = heldYAML

	rows, err := m.AddShippedTemplateFrom("demo", "invoice.yaml")
	if err != nil {
		t.Fatalf("AddShippedTemplateFrom: %v", err)
	}
	if got, _ := os.ReadFile(filepath.Join(dir, "templates", "invoice.yaml")); string(got) != heldYAML {
		t.Errorf("held copy = %q", got)
	}
	if len(rows) != 1 || !rows[0].Installed || rows[0].Name != "Invoice" {
		t.Errorf("rows = %+v", rows)
	}
	if _, err := m.AddShippedTemplateFrom("demo", "absent.yaml"); err == nil {
		t.Error("a template that does not exist was added")
	}
}

func TestRemoveShippedTemplate_LeavesTheCreatedTemplateAlone(t *testing.T) {
	m, dir, store, _ := templateManager(t, "")
	if _, err := m.SaveShippedTemplate("demo", "invoice.yaml", heldYAML); err != nil {
		t.Fatal(err)
	}
	if _, err := m.CreateShippedTemplate("demo", "invoice.yaml"); err != nil {
		t.Fatal(err)
	}
	rows, err := m.RemoveShippedTemplate("demo", "invoice.yaml")
	if err != nil || len(rows) != 0 {
		t.Fatalf("rows = %+v, err = %v", rows, err)
	}
	if _, err := os.Stat(filepath.Join(dir, "templates", "invoice.yaml")); err == nil {
		t.Error("the plugin still holds the template")
	}
	if !store.HasTemplate("invoice.yaml") {
		t.Error("the template in the context folder was removed with the plugin's copy")
	}
	if _, err := m.RemoveShippedTemplate("demo", "never-there.yaml"); err != nil {
		t.Errorf("removing what is not held: %v", err)
	}
}

func TestShippedTemplates_Unhappy(t *testing.T) {
	m, dir, store, _ := templateManager(t, "")
	for _, bad := range []string{"../invoice.yaml", "sub/invoice.yaml", "invoice", ".yaml", ".hidden.yaml", ""} {
		if _, err := m.SaveShippedTemplate("demo", bad, heldYAML); !errors.Is(err, ErrManifestInvalid) {
			t.Errorf("save %q: err = %v", bad, err)
		}
		if _, err := m.GetShippedTemplate("demo", bad); !errors.Is(err, ErrManifestInvalid) {
			t.Errorf("get %q: err = %v", bad, err)
		}
		if _, err := m.CreateShippedTemplate("demo", bad); !errors.Is(err, ErrManifestInvalid) {
			t.Errorf("create %q: err = %v", bad, err)
		}
		if _, err := m.RemoveShippedTemplate("demo", bad); !errors.Is(err, ErrManifestInvalid) {
			t.Errorf("remove %q: err = %v", bad, err)
		}
	}
	if _, err := m.SaveShippedTemplate("demo", "invoice.yaml", "INVALID"); !errors.Is(err, ErrTemplateInvalid) {
		t.Errorf("invalid yaml: err = %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "templates")); err == nil {
		t.Error("a refused save wrote into the plugin folder")
	}
	if _, err := m.CreateShippedTemplate("demo", "invoice.yaml"); !errors.Is(err, ErrTemplateNotShipped) {
		t.Errorf("create of a template the plugin does not hold: err = %v", err)
	}
	if _, err := m.ListShippedTemplates("absent"); !errors.Is(err, ErrPluginNotFound) {
		t.Errorf("unknown plugin: err = %v", err)
	}
	if store.creates != 0 {
		t.Error("something was created")
	}
	m.deps.Templates = nil
	if _, err := m.ListShippedTemplates("demo"); err == nil {
		t.Error("no template store wired, yet the list answered")
	}
}

func TestListShippedTemplates_ReportsABrokenHeldCopy(t *testing.T) {
	m, dir, _, _ := templateManager(t, "")
	if err := os.MkdirAll(filepath.Join(dir, "templates"), 0o755); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(dir, "templates", "broken.yaml"), "INVALID")
	mustWrite(t, filepath.Join(dir, "templates", "notes.txt"), "not a template")
	rows, err := m.ListShippedTemplates("demo")
	if err != nil || len(rows) != 1 || rows[0].File != "broken.yaml" || !strings.Contains(rows[0].Error, "INVALID") {
		t.Errorf("rows = %+v, err = %v", rows, err)
	}
}

func TestRefresh_NeverCreatesAHeldTemplate(t *testing.T) {
	m, _, store, _ := templateManager(t, "")
	if _, err := m.SaveShippedTemplate("demo", "invoice.yaml", heldYAML); err != nil {
		t.Fatal(err)
	}
	for range 3 {
		if err := m.Refresh(); err != nil {
			t.Fatal(err)
		}
	}
	if store.creates != 0 {
		t.Error("a refresh created the template")
	}
}

func TestImportArchive_CreatesTheTemplatesThePluginBrings(t *testing.T) {
	m, pluginsDir := newTestManager(t)
	store := newFakeTemplates()
	store.have["existing.yaml"] = "name: Mine\n"
	m.deps.Editor = kvTestFS{}
	m.deps.Templates = store

	zipPath := filepath.Join(filepath.Dir(pluginsDir), "incoming.zip")
	buildArchive(t, kvTestFS{}, zipPath, map[string]string{
		"plugin.json":             validManifest("imported"),
		"main.lua":                "function run(ctx) end\n",
		"templates/invoice.yaml":  heldYAML,
		"templates/existing.yaml": "name: Theirs\n",
		"templates/broken.yaml":   "INVALID",
	})
	res, err := m.ImportArchive(zipPath, false)
	if err != nil {
		t.Fatalf("ImportArchive: %v", err)
	}
	status := map[string]string{}
	for _, r := range res.Templates {
		status[r.File] = r.Status
	}
	if status["invoice.yaml"] != ClientInstalled || status["existing.yaml"] != ClientExists || status["broken.yaml"] != ClientFailed {
		t.Errorf("Templates = %+v", res.Templates)
	}
	if store.have["existing.yaml"] != "name: Mine\n" {
		t.Error("an existing template was replaced by the import")
	}
	if store.have["invoice.yaml"] != heldYAML {
		t.Error("the new template was not created")
	}
}

func TestTemplateInstall_ScriptCallIsHarmlessOnceTheTemplateExists(t *testing.T) {
	m, _, store, _ := templateManager(t, `function run()
		local first = formidable.template.install("invoice.yaml")
		local second = formidable.template.install("invoice.yaml")
		return { first = first, second = second }
	end`)
	if _, err := m.SaveShippedTemplate("demo", "invoice.yaml", heldYAML); err != nil {
		t.Fatal(err)
	}
	res, err := m.Run("demo", "run", nil)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	got := res.Value.(map[string]any)
	if got["first"] != false || got["second"] != false || store.creates != 1 {
		t.Errorf("result = %v, creates = %d; the start of the run made it, the script calls find it there", got, store.creates)
	}
}

func TestTemplateAdmin_InstallCreatesAndAnnouncesWhenTheTemplateIsMissing(t *testing.T) {
	m, _, store, fired := templateManager(t, "")
	if _, err := m.SaveShippedTemplate("demo", "invoice.yaml", heldYAML); err != nil {
		t.Fatal(err)
	}
	p, _ := m.Get("demo")
	created, err := templateAdmin{m: m, p: p}.Install("invoice.yaml")
	if err != nil || !created || store.creates != 1 {
		t.Fatalf("created = %v, err = %v", created, err)
	}
	notices := m.TakeClientNotices()
	want := ClientInstall{Kind: NoticeTemplate, Plugin: "demo", ID: "invoice.yaml", Status: ClientInstalled}
	if len(notices) != 1 || notices[0] != want || *fired != 1 {
		t.Errorf("notices = %+v, fired = %d", notices, *fired)
	}
}

func TestTemplateInstall_Unhappy(t *testing.T) {
	for name, call := range map[string]string{
		"not held":    `formidable.template.install("absent.yaml")`,
		"escapes":     `formidable.template.install("../../templates/x.yaml")`,
		"no argument": `formidable.template.install()`,
	} {
		t.Run(name, func(t *testing.T) {
			m, _, store, fired := templateManager(t, "function run() return "+call+" end")
			if _, err := m.Run("demo", "run", nil); err == nil {
				t.Fatal("accepted")
			}
			if store.creates != 0 || *fired != 0 {
				t.Error("something was created or announced")
			}
		})
	}
}

func TestTemplateNoticeDoesNotReplaceAClientNoticeOfTheSameName(t *testing.T) {
	m, _ := newTestManager(t)
	m.queueClientNotices([]ClientInstall{
		{Plugin: "demo", ID: "invoice.yaml", Status: ClientInstalled},
		{Kind: NoticeTemplate, Plugin: "demo", ID: "invoice.yaml", Status: ClientInstalled},
	})
	if got := m.TakeClientNotices(); len(got) != 2 {
		t.Errorf("notices = %+v, want both kept", got)
	}
}

func (f *fakeTemplates) TemplateFiles() ([]string, error) {
	var out []string
	for name := range f.have {
		out = append(out, name)
	}
	return out, nil
}

func TestRun_CreatesHeldTemplatesBeforeTheScriptStarts(t *testing.T) {
	// The script reads the template it depends on; that only works when the template was made before the first line ran.
	m, _, store, fired := templateManager(t, `function run()
		return formidable.template.install("invoice.yaml")
	end`)
	if _, err := m.SaveShippedTemplate("demo", "invoice.yaml", heldYAML); err != nil {
		t.Fatal(err)
	}
	res, err := m.Run("demo", "run", nil)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Value != false {
		t.Errorf("install returned %v; the template should already have existed when the script started", res.Value)
	}
	if store.have["invoice.yaml"] != heldYAML || store.creates != 1 {
		t.Errorf("template = %q, creates = %d", store.have["invoice.yaml"], store.creates)
	}
	notices := m.TakeClientNotices()
	want := ClientInstall{Kind: NoticeTemplate, Plugin: "demo", ID: "invoice.yaml", Status: ClientInstalled}
	if len(notices) != 1 || notices[0] != want || *fired != 1 {
		t.Errorf("notices = %+v, fired = %d; a template made at start must be announced", notices, *fired)
	}

	if _, err := m.Run("demo", "run", nil); err != nil {
		t.Fatal(err)
	}
	if store.creates != 1 || len(m.TakeClientNotices()) != 0 {
		t.Error("a second run created or announced the template again")
	}
}

func TestRun_NeverReplacesAnExistingTemplateAtStart(t *testing.T) {
	m, _, store, _ := templateManager(t, "function run() return 1 end")
	store.have["invoice.yaml"] = "name: Mine\n"
	if _, err := m.SaveShippedTemplate("demo", "invoice.yaml", heldYAML); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Run("demo", "run", nil); err != nil {
		t.Fatal(err)
	}
	if store.have["invoice.yaml"] != "name: Mine\n" || len(m.TakeClientNotices()) != 0 {
		t.Error("the user's template was touched or announced")
	}
}

func TestRun_RefusedWhenAHeldTemplateCannotBeCreated(t *testing.T) {
	m, dir, store, _ := templateManager(t, "function run() return 1 end")
	if err := os.MkdirAll(filepath.Join(dir, "templates"), 0o755); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(dir, "templates", "broken.yaml"), "INVALID")

	_, err := m.Run("demo", "run", nil)
	if !errors.Is(err, ErrTemplateNotCreated) || !strings.Contains(err.Error(), "broken.yaml") {
		t.Fatalf("err = %v, want ErrTemplateNotCreated naming the file", err)
	}
	if got := NewService(m).Run("demo", "run", nil); got.Kind != "template_not_created" {
		t.Errorf("Kind = %q", got.Kind)
	}
	if store.creates != 0 {
		t.Error("something was created")
	}
}

func TestListBindableTemplates_OffersHeldTemplatesBeforeTheyExist(t *testing.T) {
	m, _, store, _ := templateManager(t, "")
	store.have["adr.yaml"] = "name: ADR\n"
	store.have["pets.yaml"] = "name: Pets\n"
	if _, err := m.SaveShippedTemplate("demo", "pets.yaml", heldYAML); err != nil {
		t.Fatal(err)
	}
	if _, err := m.SaveShippedTemplate("demo", "invoice.yaml", heldYAML); err != nil {
		t.Fatal(err)
	}
	rows, err := m.ListBindableTemplates("demo")
	if err != nil {
		t.Fatal(err)
	}
	want := []BindableTemplate{
		{File: "adr.yaml", Held: false, Exists: true},
		{File: "invoice.yaml", Held: true, Exists: false},
		{File: "pets.yaml", Held: true, Exists: true},
	}
	if len(rows) != len(want) {
		t.Fatalf("rows = %+v", rows)
	}
	for i := range want {
		if rows[i] != want[i] {
			t.Errorf("rows[%d] = %+v, want %+v", i, rows[i], want[i])
		}
	}
	if _, err := m.ListBindableTemplates("absent"); !errors.Is(err, ErrPluginNotFound) {
		t.Errorf("err = %v", err)
	}
}
