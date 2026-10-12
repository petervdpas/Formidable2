package plugin

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fakeInstaller stands in for the api-client store: have is what already
// exists, fail forces an install error, and got records what each install read.
type fakeInstaller struct {
	have  map[string]bool
	fail  map[string]error
	got   map[string]string
	calls int

	created     map[string]any
	createdSpec string
}

func newFakeInstaller() *fakeInstaller {
	return &fakeInstaller{have: map[string]bool{}, fail: map[string]error{}, got: map[string]string{}}
}

func (f *fakeInstaller) HasClient(id string) bool { return f.have[id] }

func (f *fakeInstaller) InstallClient(id string, read func(name string) ([]byte, error)) (bool, error) {
	f.calls++
	if err := f.fail[id]; err != nil {
		return false, err
	}
	def, err := read(id + ".yaml")
	if errors.Is(err, os.ErrNotExist) {
		// Document only: the store derives the definition.
		if _, err := read("specs/" + id + ".json"); err != nil {
			return false, err
		}
		f.have[id] = true
		return true, nil
	}
	if err != nil {
		return false, err
	}
	f.got[id] = string(def)
	f.have[id] = true
	return false, nil
}

func (f *fakeInstaller) CreateClient(def map[string]any, spec []byte) (bool, error) {
	id, _ := def["id"].(string)
	if f.have[id] {
		return false, nil
	}
	if err := f.fail[id]; err != nil {
		return false, err
	}
	f.created, f.createdSpec = def, string(spec)
	f.have[id] = true
	return true, nil
}

func clientManifest(id string, clients string) string {
	return `{
		"manifest_version": 1, "id": "` + id + `", "name": "Demo",
		"version": "0.1.0", "api_clients": ` + clients + `,
		"commands": [{"id": "run", "label": "Run"}]
	}`
}

// writeShippedClient puts a definition where a plugin ships it.
func writeShippedClient(t *testing.T, pluginDir, clientID, body string) {
	t.Helper()
	dir := filepath.Join(pluginDir, "api-clients")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(dir, clientID+".yaml"), body)
}

func newClientManager(t *testing.T, inst ClientInstaller) (*Manager, string) {
	t.Helper()
	m, pluginsDir := newTestManager(t)
	m.deps.Clients = inst
	m.deps.Editor = kvTestFS{}
	return m, pluginsDir
}

func TestLoadManifest_APIClientsRoundtrip(t *testing.T) {
	dir := writePlugin(t, t.TempDir(), "demo", clientManifest("demo", `["northwind", "crm-prod"]`), "function run() end")
	m, err := LoadManifest(dir)
	if err != nil {
		t.Fatalf("LoadManifest: %v", err)
	}
	if len(m.APIClients) != 2 || m.APIClients[0] != "northwind" || m.APIClients[1] != "crm-prod" {
		t.Errorf("APIClients = %v", m.APIClients)
	}
}

func TestLoadManifest_APIClientsRejectsBadAndDuplicate(t *testing.T) {
	for name, clients := range map[string]string{
		"empty":     `[""]`,
		"traversal": `["../northwind"]`,
		"uppercase": `["Northwind"]`,
		"duplicate": `["northwind", "northwind"]`,
	} {
		t.Run(name, func(t *testing.T) {
			dir := writePlugin(t, t.TempDir(), "demo", clientManifest("demo", clients), "function run() end")
			if _, err := LoadManifest(dir); !errors.Is(err, ErrManifestInvalid) {
				t.Fatalf("err = %v, want ErrManifestInvalid", err)
			}
		})
	}
}

func TestRefresh_InstallsDeclaredClient(t *testing.T) {
	inst := newFakeInstaller()
	m, pluginsDir := newClientManager(t, inst)
	dir := writePlugin(t, pluginsDir, "demo", clientManifest("demo", `["northwind"]`), "function run() end")
	writeShippedClient(t, dir, "northwind", "id: northwind\n")

	if err := m.Refresh(); err != nil {
		t.Fatal(err)
	}
	if inst.got["northwind"] != "id: northwind\n" {
		t.Errorf("installer read %q", inst.got["northwind"])
	}
	notices := m.TakeClientNotices()
	if len(notices) != 1 || notices[0] != (ClientInstall{Plugin: "demo", ID: "northwind", Status: ClientInstalled}) {
		t.Fatalf("notices = %+v", notices)
	}
	if again := m.TakeClientNotices(); len(again) != 0 {
		t.Errorf("notices not drained: %+v", again)
	}
}

func TestRefresh_LeavesExistingClientAlone(t *testing.T) {
	inst := newFakeInstaller()
	inst.have["northwind"] = true
	m, pluginsDir := newClientManager(t, inst)
	dir := writePlugin(t, pluginsDir, "demo", clientManifest("demo", `["northwind"]`), "function run() end")
	writeShippedClient(t, dir, "northwind", "id: northwind\n")

	if err := m.Refresh(); err != nil {
		t.Fatal(err)
	}
	if inst.calls != 0 {
		t.Errorf("install called %d times for an existing client", inst.calls)
	}
	if notices := m.TakeClientNotices(); len(notices) != 0 {
		t.Errorf("an untouched client must not notify: %+v", notices)
	}
}

func TestRefresh_FailedInstallIsReportedAndPluginStillLoads(t *testing.T) {
	inst := newFakeInstaller()
	inst.fail["northwind"] = errors.New("binding not in the spec")
	inst.fail["crm-prod"] = errors.New("not a document")
	m, pluginsDir := newClientManager(t, inst)
	dir := writePlugin(t, pluginsDir, "demo", clientManifest("demo", `["northwind", "crm-prod"]`), "function run() end")
	writeShippedClient(t, dir, "northwind", "id: northwind\n")
	writeShippedClient(t, dir, "crm-prod", "id: crm-prod\n")

	if err := m.Refresh(); err != nil {
		t.Fatal(err)
	}
	if _, ok := m.Get("demo"); !ok {
		t.Fatal("plugin dropped because its client failed to install")
	}
	notices := m.TakeClientNotices()
	if len(notices) != 2 {
		t.Fatalf("notices = %+v", notices)
	}
	for _, n := range notices {
		if n.Status != ClientFailed || n.Error == "" {
			t.Errorf("notice = %+v, want a failure with its reason", n)
		}
	}
}

func TestRefresh_RepeatedFailureKeepsOneNotice(t *testing.T) {
	inst := newFakeInstaller()
	inst.fail["northwind"] = errors.New("boom")
	m, pluginsDir := newClientManager(t, inst)
	dir := writePlugin(t, pluginsDir, "demo", clientManifest("demo", `["northwind"]`), "function run() end")
	writeShippedClient(t, dir, "northwind", "id: northwind\n")

	for range 3 {
		if err := m.Refresh(); err != nil {
			t.Fatal(err)
		}
	}
	if notices := m.TakeClientNotices(); len(notices) != 1 {
		t.Errorf("notices = %+v, want one", notices)
	}
}

func TestRefresh_NoInstallerIsInert(t *testing.T) {
	m, pluginsDir := newClientManager(t, nil)
	writePlugin(t, pluginsDir, "demo", clientManifest("demo", `["northwind"]`), "function run() end")
	if err := m.Refresh(); err != nil {
		t.Fatal(err)
	}
	if notices := m.TakeClientNotices(); len(notices) != 0 {
		t.Errorf("notices = %+v", notices)
	}
}

func TestBundleReader_StaysInsideTheShippedFolder(t *testing.T) {
	root := t.TempDir()
	dir := writePlugin(t, root, "demo", clientManifest("demo", `[]`), "function run() end")
	writeShippedClient(t, dir, "northwind", "id: northwind\n")
	read := bundleReader(dir)

	if got, err := read("northwind.yaml"); err != nil || string(got) != "id: northwind\n" {
		t.Fatalf("read = %q, %v", got, err)
	}
	for _, name := range []string{"../plugin.json", "/etc/passwd", "specs/../../main.lua", ""} {
		if _, err := read(name); err == nil {
			t.Errorf("read(%q) escaped the shipped folder", name)
		}
	}
}

func TestImportArchive_ReportsInstalledAndExistingClients(t *testing.T) {
	inst := newFakeInstaller()
	inst.have["crm-prod"] = true
	m, pluginsDir := newClientManager(t, inst)
	zipPath := filepath.Join(filepath.Dir(pluginsDir), "incoming.zip")
	buildArchive(t, kvTestFS{}, zipPath, map[string]string{
		"plugin.json":                clientManifest("imported", `["northwind", "crm-prod"]`),
		"main.lua":                   "function run(ctx) end\n",
		"api-clients/northwind.yaml": "id: northwind\n",
		"api-clients/crm-prod.yaml":  "id: crm-prod\n",
	})

	res, err := m.ImportArchive(zipPath, false)
	if err != nil {
		t.Fatalf("ImportArchive: %v", err)
	}
	want := []ClientInstall{
		{Plugin: "imported", ID: "northwind", Status: ClientInstalled},
		{Plugin: "imported", ID: "crm-prod", Status: ClientExists},
	}
	if len(res.Clients) != len(want) {
		t.Fatalf("Clients = %+v", res.Clients)
	}
	for i := range want {
		if res.Clients[i] != want[i] {
			t.Errorf("Clients[%d] = %+v, want %+v", i, res.Clients[i], want[i])
		}
	}
	if pending := m.TakeClientNotices(); len(pending) != 0 {
		t.Errorf("import result already carries these; pending = %+v", pending)
	}
}

func TestImportArchive_KeepsOtherPluginsNoticesPending(t *testing.T) {
	inst := newFakeInstaller()
	m, pluginsDir := newClientManager(t, inst)
	dir := writePlugin(t, pluginsDir, "local", clientManifest("local", `["northwind"]`), "function run() end")
	writeShippedClient(t, dir, "northwind", "id: northwind\n")

	zipPath := filepath.Join(filepath.Dir(pluginsDir), "incoming.zip")
	buildArchive(t, kvTestFS{}, zipPath, map[string]string{
		"plugin.json": validManifest("imported"),
		"main.lua":    "function run(ctx) end\n",
	})
	res, err := m.ImportArchive(zipPath, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Clients) != 0 {
		t.Errorf("Clients = %+v, want none for a plugin that ships none", res.Clients)
	}
	pending := m.TakeClientNotices()
	if len(pending) != 1 || pending[0].Plugin != "local" {
		t.Errorf("pending = %+v, want the hand-copied plugin's install", pending)
	}
}

func TestRun_RefusesWhenDeclaredClientIsMissing(t *testing.T) {
	inst := newFakeInstaller()
	inst.fail["northwind"] = errors.New("boom")
	m, pluginsDir := newClientManager(t, inst)
	dir := writePlugin(t, pluginsDir, "demo", clientManifest("demo", `["northwind"]`), "function run() return 1 end")
	writeShippedClient(t, dir, "northwind", "id: northwind\n")
	if err := m.Refresh(); err != nil {
		t.Fatal(err)
	}

	if _, err := m.Run("demo", "run", nil); !errors.Is(err, ErrClientMissing) {
		t.Fatalf("err = %v, want ErrClientMissing", err)
	}
	if got := NewService(m).Run("demo", "run", nil); got.Kind != "client_missing" {
		t.Errorf("Kind = %q, want client_missing", got.Kind)
	}

	inst.have["northwind"] = true
	if _, err := m.Run("demo", "run", nil); err != nil {
		t.Fatalf("Run with the client present: %v", err)
	}
}

func TestRefresh_DocumentOnlyClientIsReportedAsDerived(t *testing.T) {
	inst := newFakeInstaller()
	m, pluginsDir := newClientManager(t, inst)
	dir := writePlugin(t, pluginsDir, "demo", clientManifest("demo", `["northwind"]`), "function run() end")
	specs := filepath.Join(dir, "api-clients", "specs")
	if err := os.MkdirAll(specs, 0o755); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(specs, "northwind.json"), `{"openapi": "3.0.3"}`)

	if err := m.Refresh(); err != nil {
		t.Fatal(err)
	}
	notices := m.TakeClientNotices()
	want := ClientInstall{Plugin: "demo", ID: "northwind", Status: ClientInstalled, Derived: true}
	if len(notices) != 1 || notices[0] != want {
		t.Fatalf("notices = %+v, want %+v", notices, want)
	}
}

func TestBundleReader_MissingFileReportsNotExist(t *testing.T) {
	dir := writePlugin(t, t.TempDir(), "demo", clientManifest("demo", `[]`), "function run() end")
	if _, err := bundleReader(dir)("northwind.yaml"); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("err = %v, want fs.ErrNotExist so a document-only bundle is recognised", err)
	}
}

func TestRefresh_DeclaredButNotShippedClientIsLeftToTheScript(t *testing.T) {
	inst := newFakeInstaller()
	m, pluginsDir := newClientManager(t, inst)
	writePlugin(t, pluginsDir, "demo", clientManifest("demo", `["northwind"]`), "function run() return 1 end")
	if err := m.Refresh(); err != nil {
		t.Fatal(err)
	}
	if inst.calls != 0 || len(m.TakeClientNotices()) != 0 {
		t.Errorf("a client the plugin does not ship was installed or reported; calls = %d", inst.calls)
	}
	if _, err := m.Run("demo", "run", nil); err != nil {
		t.Fatalf("Run must not wait for a client the script is to create: %v", err)
	}
}

const createScript = `function run(ctx)
	local before = formidable.client.exists("northwind")
	local first = formidable.client.create("northwind", {
		spec = "openapi/northwind.json", base_url = ctx.url, name = "Northwind",
		auth = { kind = "bearer" },
	})
	local second = formidable.client.create("northwind", { spec = "openapi/northwind.json" })
	return { before = before, first = first, second = second, after = formidable.client.exists("northwind") }
end`

// scriptedPlugin writes a plugin that declares northwind, ships its document outside api-clients, and creates the client from a script.
func scriptedPlugin(t *testing.T, inst *fakeInstaller, lua string) (*Manager, *int) {
	t.Helper()
	m, pluginsDir := newClientManager(t, inst)
	fired := 0
	m.deps.ClientNoticeOut = func() { fired++ }
	dir := writePlugin(t, pluginsDir, "demo", clientManifest("demo", `["northwind"]`), lua)
	if err := os.MkdirAll(filepath.Join(dir, "openapi"), 0o755); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(dir, "openapi", "northwind.json"), `{"openapi": "3.0.3"}`)
	if err := m.Refresh(); err != nil {
		t.Fatal(err)
	}
	return m, &fired
}

func TestClientCreate_ScriptCreatesOnceAndTheUserIsNotified(t *testing.T) {
	inst := newFakeInstaller()
	m, fired := scriptedPlugin(t, inst, createScript)

	res, err := m.Run("demo", "run", map[string]any{"url": "https://acc.example.com"})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	got := res.Value.(map[string]any)
	if got["before"] != false || got["first"] != true || got["second"] != false || got["after"] != true {
		t.Errorf("result = %v", got)
	}
	def := inst.created
	if def["id"] != "northwind" || def["base_url"] != "https://acc.example.com" || def["name"] != "Northwind" {
		t.Errorf("definition = %v", def)
	}
	if _, leaked := def["spec"]; leaked {
		t.Error("the spec path is not part of the definition")
	}
	if auth, _ := def["auth"].(map[string]any); auth["kind"] != "bearer" {
		t.Errorf("auth = %v", def["auth"])
	}
	if inst.createdSpec != `{"openapi": "3.0.3"}` {
		t.Errorf("spec = %q", inst.createdSpec)
	}
	notices := m.TakeClientNotices()
	want := ClientInstall{Plugin: "demo", ID: "northwind", Status: ClientInstalled, Derived: true}
	if len(notices) != 1 || notices[0] != want || *fired != 1 {
		t.Errorf("notices = %+v, fired = %d; want one notice and one signal", notices, *fired)
	}
}

func TestClientCreate_Unhappy(t *testing.T) {
	for name, call := range map[string]string{
		"undeclared client": `formidable.client.create("private", { spec = "openapi/northwind.json" })`,
		"no options":        `formidable.client.create("northwind")`,
		"no spec":           `formidable.client.create("northwind", { name = "x" })`,
		"spec escapes":      `formidable.client.create("northwind", { spec = "../../secrets.json" })`,
		"spec absolute":     `formidable.client.create("northwind", { spec = "/etc/passwd" })`,
		"spec missing":      `formidable.client.create("northwind", { spec = "openapi/absent.json" })`,
		"exists undeclared": `formidable.client.exists("private")`,
	} {
		t.Run(name, func(t *testing.T) {
			inst := newFakeInstaller()
			m, fired := scriptedPlugin(t, inst, "function run() return "+call+" end")
			_, err := m.Run("demo", "run", nil)
			if err == nil || !strings.Contains(err.Error(), "client.") {
				t.Fatalf("err = %v, want a client error", err)
			}
			if inst.created != nil || *fired != 0 || len(m.TakeClientNotices()) != 0 {
				t.Error("a refused create still reached the store or notified")
			}
		})
	}
}

func TestClientCreate_StoreErrorIsRaisedAndNothingIsNotified(t *testing.T) {
	inst := newFakeInstaller()
	inst.fail["northwind"] = errors.New("no-server")
	m, fired := scriptedPlugin(t, inst, `function run() return formidable.client.create("northwind", { spec = "openapi/northwind.json" }) end`)
	_, err := m.Run("demo", "run", nil)
	if err == nil || !strings.Contains(err.Error(), "client.create: no-server") {
		t.Fatalf("err = %v", err)
	}
	if *fired != 0 || len(m.TakeClientNotices()) != 0 {
		t.Error("a failed create must not notify")
	}
}
