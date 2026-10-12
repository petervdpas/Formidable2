package plugin

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const tinyDoc = `{"openapi": "3.0.3", "info": {"title": "Tiny"}}`

// shippedManager returns a manager with one plugin "demo" declaring clients, and its folder.
func shippedManager(t *testing.T, inst *fakeInstaller, clients string) (*Manager, string, *int) {
	t.Helper()
	m, pluginsDir := newClientManager(t, inst)
	fired := 0
	m.deps.ClientNoticeOut = func() { fired++ }
	dir := writePlugin(t, pluginsDir, "demo", clientManifest("demo", clients), "function run() end")
	if err := m.Refresh(); err != nil {
		t.Fatal(err)
	}
	m.TakeClientNotices()
	return m, dir, &fired
}

func TestSaveShippedClient_StoresAndDeclaresButCreatesNothing(t *testing.T) {
	inst := newFakeInstaller()
	m, dir, fired := shippedManager(t, inst, `[]`)

	rows, err := m.SaveShippedClient("demo", "northwind", tinyDoc)
	if err != nil {
		t.Fatalf("SaveShippedClient: %v", err)
	}
	got, err := os.ReadFile(filepath.Join(dir, "api-clients", "specs", "northwind.json"))
	if err != nil || string(got) != tinyDoc {
		t.Fatalf("stored document = %q, %v", got, err)
	}
	mf, err := LoadManifest(dir)
	if err != nil || len(mf.APIClients) != 1 || mf.APIClients[0] != "northwind" {
		t.Fatalf("api_clients on disk = %v, %v", mf.APIClients, err)
	}
	want := ShippedClient{ID: "northwind", Document: "northwind.json", Installed: false, Title: "Tiny", Operations: 3, Server: "https://doc.example.com"}
	if len(rows) != 1 || rows[0] != want {
		t.Errorf("rows = %+v, want %+v", rows, want)
	}
	if inst.calls != 0 || *fired != 0 || len(m.TakeClientNotices()) != 0 {
		t.Error("saving a document created or announced a client; only the create action may")
	}
}

func TestSaveShippedClient_KeepsTheRestOfTheManifest(t *testing.T) {
	m, dir, _ := shippedManager(t, newFakeInstaller(), `["crm-prod"]`)
	if _, err := m.SaveShippedClient("demo", "northwind", tinyDoc); err != nil {
		t.Fatal(err)
	}
	mf, _ := LoadManifest(dir)
	if mf.Name != "Demo" || len(mf.Commands) != 1 || len(mf.APIClients) != 2 || mf.APIClients[0] != "crm-prod" {
		t.Errorf("manifest = %+v", mf)
	}
}

func TestSaveShippedClient_ReplacingADocumentLeavesOneFileAndOneDeclaration(t *testing.T) {
	inst := newFakeInstaller()
	m, dir, _ := shippedManager(t, inst, `[]`)
	if _, err := m.SaveShippedClient("demo", "northwind", tinyDoc); err != nil {
		t.Fatal(err)
	}
	rows, err := m.SaveShippedClient("demo", "northwind", "openapi: 3.0.3\ninfo:\n  title: Tiny YAML\n")
	if err != nil {
		t.Fatal(err)
	}
	specs, _ := os.ReadDir(filepath.Join(dir, "api-clients", "specs"))
	if len(specs) != 1 || specs[0].Name() != "northwind.yaml" {
		t.Errorf("specs = %v, want only the yaml copy", specs)
	}
	if len(rows) != 1 || rows[0].Document != "northwind.yaml" || rows[0].Title != "Tiny YAML" {
		t.Errorf("rows = %+v", rows)
	}
	if inst.calls != 0 {
		t.Errorf("install ran %d times; saving never creates a client", inst.calls)
	}
}

func TestSaveShippedClient_Unhappy(t *testing.T) {
	cases := map[string]struct {
		plugin, client, doc string
		want                error
	}{
		"unknown plugin": {"absent", "northwind", tinyDoc, ErrPluginNotFound},
		"bad client id":  {"demo", "../northwind", tinyDoc, ErrManifestInvalid},
		"uppercase id":   {"demo", "Northwind", tinyDoc, ErrManifestInvalid},
		"empty document": {"demo", "northwind", "  \n", ErrClientDocumentInvalid},
		"not a document": {"demo", "northwind", "INVALID", ErrClientDocumentInvalid},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			inst := newFakeInstaller()
			m, dir, fired := shippedManager(t, inst, `[]`)
			if _, err := m.SaveShippedClient(tc.plugin, tc.client, tc.doc); !errors.Is(err, tc.want) {
				t.Fatalf("err = %v, want %v", err, tc.want)
			}
			if _, err := os.Stat(filepath.Join(dir, "api-clients")); err == nil {
				t.Error("a refused save wrote into the plugin folder")
			}
			if mf, _ := LoadManifest(dir); len(mf.APIClients) != 0 || *fired != 0 {
				t.Errorf("api_clients = %v, fired = %d", mf.APIClients, *fired)
			}
		})
	}
}

func TestRemoveShippedClient_LeavesTheInstalledClientAlone(t *testing.T) {
	inst := newFakeInstaller()
	m, dir, _ := shippedManager(t, inst, `["crm-prod"]`)
	if _, err := m.SaveShippedClient("demo", "northwind", tinyDoc); err != nil {
		t.Fatal(err)
	}
	writeShippedClient(t, dir, "northwind", "id: northwind\n")
	inst.have["northwind"] = true

	rows, err := m.RemoveShippedClient("demo", "northwind")
	if err != nil {
		t.Fatalf("RemoveShippedClient: %v", err)
	}
	if len(rows) != 1 || rows[0].ID != "crm-prod" {
		t.Errorf("rows = %+v", rows)
	}
	for _, f := range []string{"api-clients/northwind.yaml", "api-clients/specs/northwind.json"} {
		if _, err := os.Stat(filepath.Join(dir, filepath.FromSlash(f))); err == nil {
			t.Errorf("%s still shipped", f)
		}
	}
	if mf, _ := LoadManifest(dir); len(mf.APIClients) != 1 || mf.APIClients[0] != "crm-prod" {
		t.Errorf("api_clients = %v", mf.APIClients)
	}
	if !inst.have["northwind"] {
		t.Error("the installed client was removed along with the plugin's copy")
	}
}

func TestRemoveShippedClient_UnknownIDIsANoOp(t *testing.T) {
	m, _, _ := shippedManager(t, newFakeInstaller(), `["crm-prod"]`)
	rows, err := m.RemoveShippedClient("demo", "never-there")
	if err != nil || len(rows) != 1 {
		t.Errorf("rows = %+v, err = %v", rows, err)
	}
	if _, err := m.RemoveShippedClient("demo", "../x"); !errors.Is(err, ErrManifestInvalid) {
		t.Errorf("err = %v, want a bad id refused", err)
	}
}

func TestListShippedClients_ReportsEachKind(t *testing.T) {
	inst := newFakeInstaller()
	inst.have["by-hand"] = true
	m, dir, _ := shippedManager(t, inst, `["by-hand", "scripted", "broken"]`)
	specs := filepath.Join(dir, "api-clients", "specs")
	if err := os.MkdirAll(specs, 0o755); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(specs, "broken.json"), "INVALID")

	rows, err := m.ListShippedClients("demo")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 3 {
		t.Fatalf("rows = %+v", rows)
	}
	if !rows[0].Installed || rows[0].Document != "" {
		t.Errorf("by-hand = %+v", rows[0])
	}
	if rows[1].Installed || rows[1].Document != "" || rows[1].Error != "" {
		t.Errorf("scripted = %+v", rows[1])
	}
	if rows[2].Document != "broken.json" || !strings.Contains(rows[2].Error, "INVALID") {
		t.Errorf("broken = %+v", rows[2])
	}
	if _, err := m.ListShippedClients("absent"); !errors.Is(err, ErrPluginNotFound) {
		t.Errorf("err = %v", err)
	}
}

func TestGetShippedDocument(t *testing.T) {
	m, _, _ := shippedManager(t, newFakeInstaller(), `[]`)
	if _, err := m.SaveShippedClient("demo", "northwind", tinyDoc); err != nil {
		t.Fatal(err)
	}
	if got, err := m.GetShippedDocument("demo", "northwind"); err != nil || got != tinyDoc {
		t.Errorf("document = %q, %v", got, err)
	}
	if got, err := m.GetShippedDocument("demo", "absent"); err != nil || got != "" {
		t.Errorf("absent document = %q, %v", got, err)
	}
	if _, err := m.GetShippedDocument("demo", "../../main"); !errors.Is(err, ErrManifestInvalid) {
		t.Errorf("err = %v, want a bad id refused", err)
	}
}

func TestGenerateShippedClient_CreatesAMissingClientAndAnnouncesIt(t *testing.T) {
	inst := newFakeInstaller()
	m, _, fired := shippedManager(t, inst, `[]`)
	if _, err := m.SaveShippedClient("demo", "northwind", tinyDoc); err != nil {
		t.Fatal(err)
	}
	rows, err := m.GenerateShippedClient("demo", "northwind")
	if err != nil {
		t.Fatalf("GenerateShippedClient: %v", err)
	}
	if len(rows) != 1 || !rows[0].Installed {
		t.Errorf("rows = %+v", rows)
	}
	notices := m.TakeClientNotices()
	if len(notices) != 1 || notices[0].Status != ClientInstalled || !notices[0].Derived || *fired != 1 {
		t.Errorf("notices = %+v, fired = %d", notices, *fired)
	}
}

func TestGenerateShippedClient_ExistingClientIsLeftAlone(t *testing.T) {
	inst := newFakeInstaller()
	m, _, fired := shippedManager(t, inst, `[]`)
	if _, err := m.SaveShippedClient("demo", "northwind", tinyDoc); err != nil {
		t.Fatal(err)
	}
	if _, err := m.GenerateShippedClient("demo", "northwind"); err != nil {
		t.Fatal(err)
	}
	m.TakeClientNotices()
	calls := inst.calls
	*fired = 0

	rows, err := m.GenerateShippedClient("demo", "northwind")
	if err != nil || len(rows) != 1 || !rows[0].Installed {
		t.Fatalf("rows = %+v, err = %v", rows, err)
	}
	if inst.calls != calls || *fired != 0 || len(m.TakeClientNotices()) != 0 {
		t.Error("an existing client was regenerated or announced again")
	}
}

func TestGenerateShippedClient_Unhappy(t *testing.T) {
	inst := newFakeInstaller()
	inst.fail["broken"] = errors.New("no-server")
	m, dir, fired := shippedManager(t, inst, `["scripted", "broken"]`)
	specs := filepath.Join(dir, "api-clients", "specs")
	if err := os.MkdirAll(specs, 0o755); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(specs, "broken.json"), tinyDoc)

	if _, err := m.GenerateShippedClient("demo", "scripted"); !errors.Is(err, ErrClientNotShipped) {
		t.Errorf("no document: err = %v", err)
	}
	if _, err := m.GenerateShippedClient("demo", "broken"); err == nil || !strings.Contains(err.Error(), "no-server") {
		t.Errorf("failing install: err = %v, want the store's reason", err)
	}
	if _, err := m.GenerateShippedClient("demo", "undeclared"); !errors.Is(err, ErrManifestInvalid) {
		t.Errorf("undeclared: err = %v", err)
	}
	if _, err := m.GenerateShippedClient("absent", "broken"); !errors.Is(err, ErrPluginNotFound) {
		t.Errorf("unknown plugin: err = %v", err)
	}
	if *fired != 0 || len(m.TakeClientNotices()) != 0 {
		t.Error("a failed generate must not announce a client")
	}
}

func TestSetShippedSettings_StoresReportsAndClears(t *testing.T) {
	m, dir, fired := shippedManager(t, newFakeInstaller(), `[]`)
	if _, err := m.SaveShippedClient("demo", "northwind", tinyDoc); err != nil {
		t.Fatal(err)
	}
	def := filepath.Join(dir, "api-clients", "northwind.yaml")

	rows, err := m.SetShippedSettings("demo", "northwind", "", "https://acc.example.com")
	if err != nil {
		t.Fatalf("SetShippedSettings: %v", err)
	}
	if got, _ := os.ReadFile(def); string(got) != "base_url=https://acc.example.com" {
		t.Errorf("definition = %q", got)
	}
	if len(rows) != 1 || rows[0].BaseURL != "https://acc.example.com" || rows[0].Server != "https://doc.example.com" || rows[0].HasDefinition {
		t.Errorf("rows = %+v", rows)
	}

	rows, err = m.SetShippedSettings("demo", "northwind", "", "")
	if err != nil || rows[0].BaseURL != "" {
		t.Fatalf("clear: rows = %+v, err = %v", rows, err)
	}
	if _, err := os.Stat(def); err == nil {
		t.Error("an emptied settings file must be removed, so the plugin ships the document alone")
	}
	if *fired != 0 || len(m.TakeClientNotices()) != 0 {
		t.Error("setting a base URL created or announced a client")
	}
}

func TestSetShippedSettings_KeepsAHandWrittenDefinition(t *testing.T) {
	m, dir, _ := shippedManager(t, newFakeInstaller(), `["northwind"]`)
	writeShippedClient(t, dir, "northwind", "custom")

	rows, err := m.SetShippedSettings("demo", "northwind", "", "")
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(dir, "api-clients", "northwind.yaml"))
	if err != nil || !strings.Contains(string(got), "custom") {
		t.Errorf("hand-written definition lost: %q, %v", got, err)
	}
	if !rows[0].HasDefinition {
		t.Errorf("rows = %+v", rows)
	}
}

func TestSetShippedSettings_Unhappy(t *testing.T) {
	m, dir, _ := shippedManager(t, newFakeInstaller(), `["northwind"]`)
	if _, err := m.SetShippedSettings("demo", "northwind", "", "not-a-url"); err == nil {
		t.Error("a bad base URL was stored")
	}
	if _, err := os.Stat(filepath.Join(dir, "api-clients", "northwind.yaml")); err == nil {
		t.Error("a refused base URL wrote a file")
	}
	if _, err := m.SetShippedSettings("demo", "undeclared", "", "https://x.example.com"); !errors.Is(err, ErrManifestInvalid) {
		t.Errorf("undeclared: err = %v", err)
	}
	if _, err := m.SetShippedSettings("demo", "../x", "", "https://x.example.com"); !errors.Is(err, ErrManifestInvalid) {
		t.Errorf("bad id: err = %v", err)
	}
	if _, err := m.SetShippedSettings("absent", "northwind", "", "https://x.example.com"); !errors.Is(err, ErrPluginNotFound) {
		t.Errorf("unknown plugin: err = %v", err)
	}
}

func TestListShippedClients_UnreadableDefinitionIsReported(t *testing.T) {
	m, dir, _ := shippedManager(t, newFakeInstaller(), `["northwind"]`)
	writeShippedClient(t, dir, "northwind", "UNREADABLE")
	rows, err := m.ListShippedClients("demo")
	if err != nil || len(rows) != 1 || !strings.Contains(rows[0].Error, "unreadable") {
		t.Errorf("rows = %+v, err = %v", rows, err)
	}
}

func TestSetShippedSettings_HoldsADisplayName(t *testing.T) {
	m, dir, _ := shippedManager(t, newFakeInstaller(), `[]`)
	if _, err := m.SaveShippedClient("demo", "northwind", tinyDoc); err != nil {
		t.Fatal(err)
	}
	rows, err := m.SetShippedSettings("demo", "northwind", "Northwind (acceptance)", "https://acc.example.com")
	if err != nil {
		t.Fatal(err)
	}
	if rows[0].Name != "Northwind (acceptance)" || rows[0].Title != "Tiny" || rows[0].BaseURL != "https://acc.example.com" || rows[0].HasDefinition {
		t.Errorf("rows = %+v", rows)
	}
	rows, err = m.SetShippedSettings("demo", "northwind", "Northwind (acceptance)", "")
	if err != nil || rows[0].Name != "Northwind (acceptance)" || rows[0].BaseURL != "" {
		t.Fatalf("clearing the URL must keep the name: rows = %+v, err = %v", rows, err)
	}
	if _, err := os.Stat(filepath.Join(dir, "api-clients", "northwind.yaml")); err != nil {
		t.Error("the settings file was removed while it still holds a name")
	}
}
