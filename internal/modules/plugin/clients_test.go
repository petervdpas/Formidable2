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
	locked      map[string]bool
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

// ReadSettings and WithSettings model the settings file as "name=<name>" and "base_url=<url>" lines plus an optional "custom" marker line.
func (f *fakeInstaller) ReadSettings(def []byte) (ClientSettings, error) {
	s := string(def)
	if strings.Contains(s, "UNREADABLE") {
		return ClientSettings{}, errors.New("definition unreadable")
	}
	out := ClientSettings{Custom: strings.Contains(s, "custom")}
	for _, line := range strings.Split(s, "\n") {
		if url, ok := strings.CutPrefix(line, "base_url="); ok {
			out.BaseURL = url
		}
		if name, ok := strings.CutPrefix(line, "name="); ok {
			out.Name = name
		}
	}
	return out, nil
}

func (f *fakeInstaller) WithSettings(def []byte, id string, s ClientSettings) ([]byte, error) {
	if s.BaseURL != "" && !strings.HasPrefix(s.BaseURL, "https://") {
		return nil, errors.New("bad base url")
	}
	var lines []string
	if strings.Contains(string(def), "custom") {
		lines = append(lines, "custom")
	}
	if s.Name != "" {
		lines = append(lines, "name="+s.Name)
	}
	if s.BaseURL != "" {
		lines = append(lines, "base_url="+s.BaseURL)
	}
	if len(lines) == 0 {
		return nil, nil
	}
	return []byte(strings.Join(lines, "\n")), nil
}

func (f *fakeInstaller) CredentialLocked(id string) bool { return f.locked[id] }

// InspectDocument accepts anything but the literal INVALID, and reads a title out of the test documents.
func (f *fakeInstaller) InspectDocument(spec []byte) (DocumentInfo, error) {
	s := string(spec)
	if strings.Contains(s, "INVALID") {
		return DocumentInfo{}, errors.New("document says INVALID")
	}
	info := DocumentInfo{Operations: 3, Ext: ".yaml", Server: "https://doc.example.com"}
	if strings.HasPrefix(strings.TrimSpace(s), "{") {
		info.Ext = ".json"
	}
	switch {
	case strings.Contains(s, "Tiny YAML"):
		info.Title = "Tiny YAML"
	case strings.Contains(s, "Tiny"):
		info.Title = "Tiny"
	}
	return info, nil
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

func TestRefresh_NeverCreatesAClient(t *testing.T) {
	inst := newFakeInstaller()
	m, pluginsDir := newClientManager(t, inst)
	dir := writePlugin(t, pluginsDir, "demo", clientManifest("demo", `["northwind"]`), "function run() end")
	writeShippedClient(t, dir, "northwind", "id: northwind\n")

	for range 3 {
		if err := m.Refresh(); err != nil {
			t.Fatal(err)
		}
	}
	if inst.calls != 0 || inst.have["northwind"] {
		t.Errorf("a refresh created the shipped client; calls = %d", inst.calls)
	}
	if notices := m.TakeClientNotices(); len(notices) != 0 {
		t.Errorf("notices = %+v", notices)
	}
}

func TestImportArchive_CreatesOnlyTheImportedPluginsClients(t *testing.T) {
	inst := newFakeInstaller()
	m, pluginsDir := newClientManager(t, inst)
	local := writePlugin(t, pluginsDir, "local", clientManifest("local", `["crm-prod"]`), "function run() end")
	writeShippedClient(t, local, "crm-prod", "id: crm-prod\n")

	zipPath := filepath.Join(filepath.Dir(pluginsDir), "incoming.zip")
	buildArchive(t, kvTestFS{}, zipPath, map[string]string{
		"plugin.json":                      clientManifest("imported", `["northwind"]`),
		"main.lua":                         "function run(ctx) end\n",
		"api-clients/specs/northwind.json": `{"openapi": "3.0.3"}`,
	})
	res, err := m.ImportArchive(zipPath, false)
	if err != nil {
		t.Fatal(err)
	}
	want := ClientInstall{Plugin: "imported", ID: "northwind", Status: ClientInstalled, Derived: true}
	if len(res.Clients) != 1 || res.Clients[0] != want {
		t.Errorf("Clients = %+v, want %+v", res.Clients, want)
	}
	if inst.have["crm-prod"] {
		t.Error("importing one plugin created another plugin's client")
	}
	if pending := m.TakeClientNotices(); len(pending) != 0 {
		t.Errorf("pending = %+v; the import result already carries its outcomes", pending)
	}
}

func TestImportArchive_ReportsAClientThatCouldNotBeCreated(t *testing.T) {
	inst := newFakeInstaller()
	inst.fail["northwind"] = errors.New("no-server")
	m, pluginsDir := newClientManager(t, inst)
	zipPath := filepath.Join(filepath.Dir(pluginsDir), "incoming.zip")
	buildArchive(t, kvTestFS{}, zipPath, map[string]string{
		"plugin.json":                clientManifest("imported", `["northwind"]`),
		"main.lua":                   "function run(ctx) end\n",
		"api-clients/northwind.yaml": "id: northwind\n",
	})
	res, err := m.ImportArchive(zipPath, false)
	if err != nil {
		t.Fatalf("the plugin itself must still import: %v", err)
	}
	if len(res.Clients) != 1 || res.Clients[0].Status != ClientFailed || res.Clients[0].Error != "no-server" {
		t.Errorf("Clients = %+v", res.Clients)
	}
	if _, ok := m.Get("imported"); !ok {
		t.Error("plugin not registered")
	}
}

func TestRun_AsksForTheVaultWhenADeclaredClientsCredentialIsLocked(t *testing.T) {
	inst := newFakeInstaller()
	inst.have["northwind"] = true
	inst.locked = map[string]bool{"northwind": true}
	m, pluginsDir := newClientManager(t, inst)
	writePlugin(t, pluginsDir, "demo", clientManifest("demo", `["northwind"]`), "function run() return 1 end")
	if err := m.Refresh(); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Run("demo", "run", nil); !errors.Is(err, ErrVaultLocked) {
		t.Fatalf("err = %v, want ErrVaultLocked", err)
	}
	if got := NewService(m).Run("demo", "run", nil); got.Kind != "vault_locked" {
		t.Errorf("Kind = %q, want vault_locked", got.Kind)
	}
	// Unlocked: the same run goes through.
	inst.locked["northwind"] = false
	if res, err := m.Run("demo", "run", nil); err != nil || res.Value != float64(1) {
		t.Fatalf("after unlocking: %v, %v", res.Value, err)
	}
}

func TestRun_LockedVaultDoesNotStopAPluginThatNeedsNoCredential(t *testing.T) {
	inst := newFakeInstaller()
	inst.locked = map[string]bool{"elsewhere": true, "not-created-yet": true}
	inst.have["elsewhere"] = true
	m, pluginsDir := newClientManager(t, inst)
	// One plugin declares no client; the other declares a client that does not exist yet (its script creates it).
	writePlugin(t, pluginsDir, "plain", clientManifest("plain", `[]`), "function run() return 1 end")
	writePlugin(t, pluginsDir, "maker", clientManifest("maker", `["not-created-yet"]`), "function run() return 2 end")
	if err := m.Refresh(); err != nil {
		t.Fatal(err)
	}
	for id, want := range map[string]float64{"plain": 1, "maker": 2} {
		if res, err := m.Run(id, "run", nil); err != nil || res.Value != want {
			t.Errorf("%s: %v, %v; a locked vault must only stop a plugin whose own client needs it", id, res.Value, err)
		}
	}
}
