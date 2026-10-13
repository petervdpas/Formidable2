package app

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/petervdpas/formidable2/internal/modules/connection"
	"github.com/petervdpas/formidable2/internal/modules/plugin"
	"github.com/petervdpas/formidable2/internal/modules/system"
)

// clientStoreFS is an in-memory api-client store, so the test owns exactly
// what is "already installed".
type clientStoreFS struct {
	mu    sync.Mutex
	files map[string]string
}

func (f *clientStoreFS) FileExists(p string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	_, ok := f.files[p]
	return ok
}

func (f *clientStoreFS) LoadFile(p string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	v, ok := f.files[p]
	if !ok {
		return "", os.ErrNotExist
	}
	return v, nil
}

func (f *clientStoreFS) SaveFile(p, content string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.files[p] = content
	return nil
}

func (f *clientStoreFS) DeleteFile(p string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.files, p)
	return nil
}

func (f *clientStoreFS) ListDir(dir string) ([]string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	prefix := strings.TrimSuffix(dir, "/") + "/"
	var out []string
	for p := range f.files {
		if rest, ok := strings.CutPrefix(p, prefix); ok && !strings.Contains(rest, "/") {
			out = append(out, rest)
		}
	}
	return out, nil
}

type fixedSecret struct {
	value string
	err   error
}

func (s fixedSecret) Secret(string) (string, error) { return s.value, s.err }

const shopSpec = `{
  "openapi": "3.0.3",
  "info": {"title": "Shop", "version": "1.0.0"},
  "paths": {
    "/products": {"get": {"operationId": "listProducts",
      "parameters": [{"name": "q", "in": "query", "schema": {"type": "string"}}]}},
    "/products/{id}": {"get": {"operationId": "getProduct",
      "parameters": [{"name": "id", "in": "path", "required": true, "schema": {"type": "string"}}]}}
  }
}`

func shopDefinition(baseURL string) string {
	return `id: shop
name: Shop
base_url: ` + baseURL + `
spec_file: shop.json
auth:
  kind: bearer
resources:
  - key: products
    label: Products
    list:
      operation: listProducts
    get:
      operation: getProduct
    items_path: /data
    id_path: /sku
    label_path: /name
    search_param: q
    fields:
      - key: price
        label: Price
        pointer: /price
`
}

const shopPluginLua = `
function listing(ctx)
  local res = formidable.client.resources("shop")[1]
  local page = formidable.client.items("shop", "products", { search = "tea", select = { "price" } })
  local one = formidable.client.fetch("shop", "products", page.items[1].id)
  return {
    clients = #formidable.client.list(),
    resource = res.key, can_fetch = res.can_fetch, can_search = res.can_search, field = res.fields[1].key,
    count = #page.items, first = page.items[1].label, price = page.items[1].fields.price,
    fetched = one.label,
  }
end

function snap(ctx)
  local page = formidable.client.items("shop", "products")
  local all = formidable.client.snapshot(page.items[1])
  local none = formidable.client.snapshot(page.items[1], {})
  local ok, err = pcall(formidable.client.snapshot, { label = "nameless" })
  return {
    id = all.id, label = all.label, price = all.fields.price, stamped = all.fetched ~= "",
    none = next(none.fields) == nil, refused = not ok, why = tostring(err),
  }
end

function empty(ctx)
  return #formidable.client.items("shop", "products", { search = "none" }).items
end
`

// shopPlugin writes a plugin that ships the shop client and returns the wired managers.
func shopPlugin(t *testing.T, baseURL string, secrets connection.SecretResolver) (*plugin.Manager, *connection.Manager) {
	t.Helper()
	pluginsDir := filepath.Join(t.TempDir(), "plugins")
	dir := filepath.Join(pluginsDir, "shopper")
	for name, body := range map[string]string{
		"plugin.json": `{"manifest_version": 1, "id": "shopper", "name": "Shopper", "version": "0.1.0",
			"api_clients": ["shop"],
			"commands": [{"id": "listing", "label": "Listing"}, {"id": "empty", "label": "Empty"}, {"id": "snap", "label": "Snap"}]}`,
		"main.lua":                    shopPluginLua,
		"api-clients/shop.yaml":       shopDefinition(baseURL),
		"api-clients/specs/shop.json": shopSpec,
	} {
		full := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	clientM := connection.NewManager(&clientStoreFS{files: map[string]string{}}, nil)
	adapter := pluginClientAdapter{m: clientM, inv: connection.NewInvoker(clientM, secrets)}
	pluginM := plugin.NewManager(plugin.ManagerDeps{PluginsDir: pluginsDir, Client: adapter, Clients: adapter,
		Editor: system.NewManager(t.TempDir(), nil)})
	if err := pluginM.Refresh(); err != nil {
		t.Fatal(err)
	}
	if clientM.Has("shop") {
		t.Fatal("a refresh created the shipped client")
	}
	if _, err := pluginM.GenerateShippedClient("shopper", "shop"); err != nil {
		t.Fatalf("GenerateShippedClient: %v", err)
	}
	return pluginM, clientM
}

func TestPluginClient_ShippedClientIsInstalledThenCalled(t *testing.T) {
	var gotAuth, gotSearch string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/products" && r.URL.Query().Get("q") == "none":
			_, _ = w.Write([]byte(`{"data": []}`))
		case r.URL.Path == "/products":
			gotSearch = r.URL.Query().Get("q")
			_, _ = w.Write([]byte(`{"data": [{"sku": "T1", "name": "Green tea", "price": "4.50"}]}`))
		case r.URL.Path == "/products/T1":
			_, _ = w.Write([]byte(`{"sku": "T1", "name": "Green tea", "price": "4.50"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	pluginM, clientM := shopPlugin(t, srv.URL, fixedSecret{value: "s3cret"})
	if !clientM.Has("shop") {
		t.Fatalf("shipped client not installed; notices = %+v", pluginM.TakeClientNotices())
	}

	res, err := pluginM.Run("shopper", "listing", nil)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	got := res.Value.(map[string]any)
	want := map[string]any{
		"clients": float64(1), "resource": "products", "can_fetch": true, "can_search": true, "field": "price",
		"count": float64(1), "first": "Green tea", "price": "4.50", "fetched": "Green tea",
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s = %v, want %v", k, got[k], v)
		}
	}
	if gotAuth != "Bearer s3cret" || gotSearch != "tea" {
		t.Errorf("remote saw auth %q and search %q", gotAuth, gotSearch)
	}

	res, err = pluginM.Run("shopper", "empty", nil)
	if err != nil {
		t.Fatalf("Run empty: %v", err)
	}
	if res.Value != float64(0) {
		t.Errorf("empty page length = %v, want 0", res.Value)
	}
}

func TestPluginClient_MissingCredentialReadsAsConfiguration(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("a call went out without a credential")
	}))
	defer srv.Close()

	pluginM, _ := shopPlugin(t, srv.URL, fixedSecret{err: errors.New("vault is locked")})
	_, err := pluginM.Run("shopper", "listing", nil)
	if err == nil || !strings.Contains(err.Error(), "client.items: not_configured: ") {
		t.Fatalf("err = %v, want a flat not_configured error", err)
	}
	if strings.Contains(err.Error(), `{"code"`) {
		t.Errorf("error still JSON shaped: %v", err)
	}
}

func TestPluginClient_RemoteFailureCarriesItsCode(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "nope", http.StatusUnauthorized)
	}))
	defer srv.Close()

	pluginM, _ := shopPlugin(t, srv.URL, fixedSecret{value: "wrong"})
	_, err := pluginM.Run("shopper", "listing", nil)
	if err == nil || !strings.Contains(err.Error(), "client.items: unauthorized: ") {
		t.Fatalf("err = %v, want an unauthorized error", err)
	}
}

func TestPluginClient_SnapshotMatchesTheStoredFieldShape(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data": [{"sku": "T1", "name": "Green tea", "price": "4.50"}]}`))
	}))
	defer srv.Close()

	pluginM, _ := shopPlugin(t, srv.URL, fixedSecret{value: "s3cret"})
	res, err := pluginM.Run("shopper", "snap", nil)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	got := res.Value.(map[string]any)
	for k, v := range map[string]any{
		"id": "T1", "label": "Green tea", "price": "4.50", "stamped": true, "none": true, "refused": true,
	} {
		if got[k] != v {
			t.Errorf("%s = %v, want %v", k, got[k], v)
		}
	}
	if !strings.Contains(got["why"].(string), "client.snapshot: item has no id") {
		t.Errorf("why = %v", got["why"])
	}
}

// docOnlySpec is everything the plugin below ships: no definition, only the
// document. It declares its server, so the derived client has a host, but the
// test server's address is not known when the constant is written, hence %s.
const docOnlySpec = `{
  "openapi": "3.0.3",
  "info": {"title": "Kennel", "version": "1.0.0"},
  "servers": [{"url": "%s"}],
  "security": [{"token": []}],
  "components": {"securitySchemes": {"token": {"type": "http", "scheme": "bearer"}}},
  "paths": {
    "/dogs": {"get": {"operationId": "findDogs",
      "parameters": [{"name": "status", "in": "query", "required": true, "schema": {"type": "string"}}]},
      "post": {"operationId": "addDog", "requestBody": {"required": true,
        "content": {"application/json": {"schema": {"type": "object"}}}}}},
    "/dogs/{id}": {"delete": {"operationId": "removeDog",
      "parameters": [{"name": "id", "in": "path", "required": true, "schema": {"type": "integer"}}]}}
  }
}`

func TestPluginClient_DocumentOnlyPluginDerivesAClientAndCallsIt(t *testing.T) {
	var gotAuth, gotStatus string
	var writes []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.Method {
		case http.MethodPost:
			raw, _ := io.ReadAll(r.Body)
			writes = append(writes, "POST "+r.URL.Path+" "+r.Header.Get("Content-Type")+" "+string(raw)+" "+r.Header.Get("Authorization"))
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"id": 3, "name": "Bo"}`))
		case http.MethodDelete:
			writes = append(writes, "DELETE "+r.URL.Path)
			w.WriteHeader(http.StatusNoContent)
		default:
			gotAuth, gotStatus = r.Header.Get("Authorization"), r.URL.Query().Get("status")
			_, _ = w.Write([]byte(`[{"id": 1, "name": "Rex"}, {"id": 2, "name": "Fido"}]`))
		}
	}))
	defer srv.Close()

	pluginsDir := filepath.Join(t.TempDir(), "plugins")
	dir := filepath.Join(pluginsDir, "kennel")
	for name, body := range map[string]string{
		"plugin.json": `{"manifest_version": 1, "id": "kennel", "name": "Kennel", "version": "0.1.0",
			"api_clients": ["kennel-api"],
			"commands": [{"id": "dogs", "label": "Dogs"}, {"id": "write", "label": "Write"}, {"id": "nobody", "label": "Nobody"}]}`,
		"main.lua": `
function dogs(ctx)
  local ops = formidable.client.operations("kennel-api")
  local r = formidable.client.call("kennel-api", "findDogs", { status = "available" })
  return { ops = #ops, status = r.status, count = #r.data, second = r.data[2].name, id = r.data[1].id }
end
function write(ctx)
  local made = formidable.client.call("kennel-api", "addDog", nil, { name = "Bo", tags = { "new" } })
  local gone = formidable.client.call("kennel-api", "removeDog", { id = made.data.id })
  local body
  for _, op in ipairs(formidable.client.operations("kennel-api")) do
    if op.id == "addDog" then body = op.body end
  end
  return {
    made = made.status, id = made.data.id, gone = gone.status,
    required = body.required, accepts = body.content_types[1],
  }
end
function nobody(ctx)
  return formidable.client.call("kennel-api", "addDog")
end`,
		"api-clients/specs/kennel-api.json": fmt.Sprintf(docOnlySpec, srv.URL),
	} {
		full := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	clientM := connection.NewManager(&clientStoreFS{files: map[string]string{}}, nil)
	adapter := pluginClientAdapter{m: clientM, inv: connection.NewInvoker(clientM, fixedSecret{value: "s3cret"})}
	pluginM := plugin.NewManager(plugin.ManagerDeps{PluginsDir: pluginsDir, Client: adapter, Clients: adapter,
		Editor: system.NewManager(t.TempDir(), nil)})
	if err := pluginM.Refresh(); err != nil {
		t.Fatal(err)
	}
	if _, err := pluginM.Run("kennel", "dogs", nil); !errors.Is(err, plugin.ErrClientMissing) {
		t.Fatalf("Run before the client is created: err = %v, want ErrClientMissing", err)
	}
	if _, err := pluginM.GenerateShippedClient("kennel", "kennel-api"); err != nil {
		t.Fatalf("GenerateShippedClient: %v", err)
	}

	notices := pluginM.TakeClientNotices()
	want := plugin.ClientInstall{Plugin: "kennel", ID: "kennel-api", Status: plugin.ClientInstalled, Derived: true}
	if len(notices) != 1 || notices[0] != want {
		t.Fatalf("notices = %+v, want %+v", notices, want)
	}
	c, _, err := clientM.Get("kennel-api")
	if err != nil {
		t.Fatal(err)
	}
	if c.Name != "Kennel" || c.Auth.Kind != connection.AuthBearer || len(c.Resources) != 0 {
		t.Errorf("derived client = %+v", c)
	}

	res, err := pluginM.Run("kennel", "dogs", nil)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	got := res.Value.(map[string]any)
	for k, v := range map[string]any{"ops": float64(3), "status": float64(200), "count": float64(2), "second": "Fido", "id": float64(1)} {
		if got[k] != v {
			t.Errorf("%s = %v, want %v", k, got[k], v)
		}
	}
	if gotAuth != "Bearer s3cret" || gotStatus != "available" {
		t.Errorf("remote saw auth %q, status %q", gotAuth, gotStatus)
	}

	res, err = pluginM.Run("kennel", "write", nil)
	if err != nil {
		t.Fatalf("Run write: %v", err)
	}
	wrote := res.Value.(map[string]any)
	for k, v := range map[string]any{"made": float64(201), "id": float64(3), "gone": float64(204), "required": true, "accepts": "application/json"} {
		if wrote[k] != v {
			t.Errorf("%s = %v, want %v", k, wrote[k], v)
		}
	}
	want2 := []string{`POST /dogs application/json {"name":"Bo","tags":["new"]} Bearer s3cret`, "DELETE /dogs/3"}
	if len(writes) != 2 || writes[0] != want2[0] || writes[1] != want2[1] {
		t.Errorf("remote saw %q, want %q", writes, want2)
	}

	if _, err := pluginM.Run("kennel", "nobody", nil); err == nil || !strings.Contains(err.Error(), "client.call: missing_param") {
		t.Errorf("a required body left out: err = %v", err)
	}
	if len(writes) != 2 {
		t.Error("the call without its required body still went out")
	}
}

func TestPluginClient_ScriptCreatesAClientAndCallsItInTheSameRun(t *testing.T) {
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("X-Key")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"id": 1, "name": "Rex"}]`))
	}))
	defer srv.Close()

	pluginsDir := filepath.Join(t.TempDir(), "plugins")
	dir := filepath.Join(pluginsDir, "kennel")
	for name, body := range map[string]string{
		"plugin.json": `{"manifest_version": 1, "id": "kennel", "name": "Kennel", "version": "0.1.0",
			"api_clients": ["kennel-api"],
			"commands": [{"id": "setup", "label": "Setup"}, {"id": "typo", "label": "Typo"}]}`,
		"main.lua": `
function setup(ctx)
  local created = formidable.client.create("kennel-api", {
    spec = "openapi/kennel.json",
    base_url = ctx.url,
    auth = { kind = "apikey", ["in"] = "header", name = "X-Key" },
    resources = { {
      key = "dogs", label = "Dogs",
      list = { operation = "findDogs", params = { status = "available" } },
      id_path = "/id", label_path = "/name",
    } },
  })
  local again = formidable.client.create("kennel-api", { spec = "openapi/kennel.json", base_url = "https://evil.example.com" })
  local page = formidable.client.items("kennel-api", "dogs")
  return { created = created, again = again, first = page.items[1].label }
end
function typo(ctx)
  return formidable.client.create("kennel-api", { spec = "openapi/kennel.json", base_ur = "https://x.example.com" })
end`,
		// A relative server: only the script's base_url makes this client valid.
		"openapi/kennel.json": fmt.Sprintf(docOnlySpec, "/api"),
	} {
		full := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	store := &clientStoreFS{files: map[string]string{}}
	clientM := connection.NewManager(store, nil)
	adapter := pluginClientAdapter{m: clientM, inv: connection.NewInvoker(clientM, fixedSecret{value: "k3y"})}
	signals := 0
	pluginM := plugin.NewManager(plugin.ManagerDeps{PluginsDir: pluginsDir, Client: adapter, Clients: adapter,
		ClientNoticeOut: func() { signals++ }})
	if err := pluginM.Refresh(); err != nil {
		t.Fatal(err)
	}
	if clientM.Has("kennel-api") || len(pluginM.TakeClientNotices()) != 0 {
		t.Fatal("a client the plugin does not ship under api-clients was installed at refresh")
	}

	if _, err := pluginM.Run("kennel", "typo", nil); err == nil || !strings.Contains(err.Error(), "client.create: bad option") {
		t.Fatalf("typo err = %v, want the misspelt option refused", err)
	}
	if len(store.files) != 0 {
		t.Fatalf("a refused create wrote %v", store.files)
	}

	res, err := pluginM.Run("kennel", "setup", map[string]any{"url": srv.URL})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	got := res.Value.(map[string]any)
	if got["created"] != true || got["again"] != false || got["first"] != "Rex" {
		t.Errorf("result = %v", got)
	}
	if gotAuth != "k3y" {
		t.Errorf("remote saw key %q", gotAuth)
	}
	c, _, err := clientM.Get("kennel-api")
	if err != nil {
		t.Fatal(err)
	}
	if c.BaseURL != srv.URL || c.Name != "Kennel" || c.SpecFile != "kennel-api.json" {
		t.Errorf("client = %+v; the second create must not have re-pointed it", c)
	}
	notices := pluginM.TakeClientNotices()
	if len(notices) != 1 || notices[0].ID != "kennel-api" || notices[0].Status != plugin.ClientInstalled || signals != 1 {
		t.Errorf("notices = %+v, signals = %d", notices, signals)
	}
}

// pdfFixture is a complete one-page PDF with the bytes a text-minded
// transport would mangle: the binary marker line, a NUL, a CRLF and 0xFF.
var pdfFixture = []byte("%PDF-1.4\n%\xe2\xe3\xcf\xd3\n% \x00 \r\n \xff\xfe\n" +
	"1 0 obj\n<< /Type /Catalog /Pages 2 0 R >>\nendobj\n" +
	"2 0 obj\n<< /Type /Pages /Kids [3 0 R] /Count 1 >>\nendobj\n" +
	"3 0 obj\n<< /Type /Page /Parent 2 0 R /MediaBox [0 0 200 200] >>\nendobj\n" +
	"trailer\n<< /Root 1 0 R >>\n%%EOF\n")

const archiveSpec = `{
  "openapi": "3.0.3",
  "info": {"title": "Archive", "version": "1.0.0"},
  "servers": [{"url": "%s"}],
  "paths": {
    "/entries/{guid}/document": {"put": {"operationId": "putDocument",
      "parameters": [{"name": "guid", "in": "path", "required": true, "schema": {"type": "string"}}],
      "requestBody": {"required": true,
        "content": {"application/pdf": {"schema": {"type": "string", "format": "binary"}}}}}}
  }
}`

func TestPluginClient_SendsAFileFromDiskByteForByte(t *testing.T) {
	var gotBody []byte
	var gotType, gotPath string
	hits := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotBody, _ = io.ReadAll(r.Body)
		gotType, gotPath = r.Header.Get("Content-Type"), r.URL.Path
		hits++
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"bytes": ` + fmt.Sprint(len(gotBody)) + `}`))
	}))
	defer srv.Close()

	root := t.TempDir()
	pdfPath := filepath.Join(root, "invoice-7.pdf")
	if err := os.WriteFile(pdfPath, pdfFixture, 0o644); err != nil {
		t.Fatal(err)
	}
	pluginsDir := filepath.Join(root, "plugins")
	dir := filepath.Join(pluginsDir, "filer")
	for name, body := range map[string]string{
		"plugin.json": `{"manifest_version": 1, "id": "filer", "name": "Filer", "version": "0.1.0",
			"api_clients": ["archive"], "commands": [{"id": "send", "label": "Send"}]}`,
		"main.lua": `
function send(ctx)
  local bytes = formidable.fs.read(ctx.path)
  local res = formidable.client.call("archive", "putDocument", { guid = "abc-123" }, bytes)
  return { read = #bytes, status = res.status, stored = res.data.bytes }
end`,
		"api-clients/specs/archive.json": fmt.Sprintf(archiveSpec, srv.URL),
	} {
		full := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	clientM := connection.NewManager(&clientStoreFS{files: map[string]string{}}, nil)
	adapter := pluginClientAdapter{m: clientM, inv: connection.NewInvoker(clientM, nil)}
	pluginM := plugin.NewManager(plugin.ManagerDeps{PluginsDir: pluginsDir, Client: adapter, Clients: adapter,
		FS: plugin.OSFS{}, Editor: system.NewManager(t.TempDir(), nil)})
	if err := pluginM.Refresh(); err != nil {
		t.Fatal(err)
	}
	if _, err := pluginM.GenerateShippedClient("filer", "archive"); err != nil {
		t.Fatalf("GenerateShippedClient: %v", err)
	}

	res, err := pluginM.Run("filer", "send", map[string]any{"path": pdfPath})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	got := res.Value.(map[string]any)
	n := float64(len(pdfFixture))
	if got["read"] != n || got["status"] != float64(201) || got["stored"] != n {
		t.Errorf("result = %v, want %v bytes read and stored", got, n)
	}
	if !bytes.Equal(gotBody, pdfFixture) {
		t.Errorf("%d bytes arrived, %d were on disk; the file was altered on the way", len(gotBody), len(pdfFixture))
	}
	if gotType != "application/pdf" || gotPath != "/entries/abc-123/document" || hits != 1 {
		t.Errorf("remote saw %q on %s, %d times", gotType, gotPath, hits)
	}
}

func TestPluginClient_AttemptReadsARefusalAndRecoversAfterAnUnknownOutcome(t *testing.T) {
	// The remote books an entry once per guid, as Themis does: a repeat is a 409 with the reason in the body.
	booked := map[string]bool{}
	posts := 0
	slowOnce := true
	var mu sync.Mutex
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method != http.MethodPost {
			_, _ = w.Write([]byte(`[]`))
			return
		}
		var in map[string]any
		_ = json.NewDecoder(r.Body).Decode(&in)
		guid, _ := in["guid"].(string)
		// The slow first answer is still pending when the script asks again, so two handlers overlap.
		mu.Lock()
		posts++
		repeat, slow := booked[guid], slowOnce
		booked[guid] = true
		slowOnce = false
		mu.Unlock()
		if repeat {
			w.WriteHeader(http.StatusConflict)
			_, _ = w.Write([]byte(`{"error": "ledger.error.duplicate_guid"}`))
			return
		}
		if slow {
			// Booked, but the answer never makes it back in time.
			time.Sleep(400 * time.Millisecond)
			return
		}
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"guid": "` + guid + `"}`))
	}))
	defer srv.Close()

	pluginsDir := filepath.Join(t.TempDir(), "plugins")
	dir := filepath.Join(pluginsDir, "kennel")
	for name, body := range map[string]string{
		"plugin.json": `{"manifest_version": 1, "id": "kennel", "name": "Kennel", "version": "0.1.0",
			"api_clients": ["kennel-api"], "commands": [{"id": "send", "label": "Send"}]}`,
		"main.lua": `
function send(ctx)
  local body = { guid = "g-1", name = "Bo" }
  local res, err = formidable.client.attempt("kennel-api", "addDog", nil, body)
  local first = err and err.delivery or "ok"
  local outcome = "sent"
  if err and err.delivery == "unknown" then
    -- Not known whether it arrived: ask by sending the same guid again.
    res, err = formidable.client.attempt("kennel-api", "addDog", nil, body)
    if err and err.status == 409 and err.data and err.data.error == "ledger.error.duplicate_guid" then
      outcome = "it had arrived the first time"
    end
  end
  return { first = first, outcome = outcome, status = err and err.status, reason = err and err.data and err.data.error }
end`,
		"api-clients/specs/kennel-api.json": fmt.Sprintf(docOnlySpec, srv.URL),
	} {
		full := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	clientM := connection.NewManager(&clientStoreFS{files: map[string]string{}}, nil)
	inv := connection.NewInvoker(clientM, fixedSecret{value: "s3cret"}, connection.WithTimeout(100*time.Millisecond))
	adapter := pluginClientAdapter{m: clientM, inv: inv}
	pluginM := plugin.NewManager(plugin.ManagerDeps{PluginsDir: pluginsDir, Client: adapter, Clients: adapter,
		Editor: system.NewManager(t.TempDir(), nil)})
	if err := pluginM.Refresh(); err != nil {
		t.Fatal(err)
	}
	if _, err := pluginM.GenerateShippedClient("kennel", "kennel-api"); err != nil {
		t.Fatal(err)
	}

	res, err := pluginM.Run("kennel", "send", nil)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	got := res.Value.(map[string]any)
	want := map[string]any{
		"first": "unknown", "outcome": "it had arrived the first time",
		"status": float64(409), "reason": "ledger.error.duplicate_guid",
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s = %v, want %v", k, got[k], v)
		}
	}
	mu.Lock()
	defer mu.Unlock()
	if posts != 2 || len(booked) != 1 {
		t.Errorf("posts = %d, booked = %d; the script sent twice on purpose and the remote booked once", posts, len(booked))
	}
}
