package plugin

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// fakeClients stands in for the api-client store's read side and records what
// each call was asked for.
type fakeClients struct {
	items     map[string]any
	item      map[string]any
	err       error
	calls     int
	gotClient string
	gotRes    string
	gotID     string
	gotQuery  ClientQuery
	gotCtx    context.Context
	gotItem   map[string]any
	gotKeys   []string
	gotOp     string
	gotParams map[string]string
}

func (f *fakeClients) ListClients() ([]map[string]any, error) {
	return []map[string]any{
		{"id": "crm-prod", "name": "CRM"},
		{"id": "northwind", "name": "Northwind"},
		{"id": "private", "name": "Not for this plugin"},
	}, f.err
}

func (f *fakeClients) ClientResources(id string) ([]map[string]any, error) {
	f.calls++
	f.gotClient = id
	return []map[string]any{{"key": "customers", "can_fetch": true}}, f.err
}

func (f *fakeClients) ClientItems(ctx context.Context, client, resource string, q ClientQuery) (map[string]any, error) {
	f.calls++
	f.gotCtx, f.gotClient, f.gotRes, f.gotQuery = ctx, client, resource, q
	return f.items, f.err
}

func (f *fakeClients) ClientFetch(ctx context.Context, client, resource, id string, q ClientQuery) (map[string]any, error) {
	f.calls++
	f.gotCtx, f.gotClient, f.gotRes, f.gotID, f.gotQuery = ctx, client, resource, id, q
	return f.item, f.err
}

func (f *fakeClients) ClientOperations(id string) ([]map[string]any, error) {
	f.calls++
	f.gotClient = id
	return []map[string]any{{"id": "listPets", "method": "GET", "path": "/pets"}}, f.err
}

func (f *fakeClients) ClientCall(ctx context.Context, client, operation string, params map[string]string) (map[string]any, error) {
	f.calls++
	f.gotCtx, f.gotClient, f.gotOp, f.gotParams = ctx, client, operation, params
	return f.item, f.err
}

func (f *fakeClients) ClientSnapshot(item map[string]any, keys []string) (map[string]any, error) {
	f.calls++
	f.gotItem, f.gotKeys = item, keys
	return f.item, f.err
}

func runClientScript(t *testing.T, f ClientAccess, declared []string, body string) (RunResult, error) {
	t.Helper()
	return runScript(scriptOpts{
		Source:    "function run(ctx)\n" + body + "\nend",
		Fn:        "run",
		Ctx:       context.Background(),
		Client:    f,
		ClientIDs: declared,
	})
}

func TestClient_ListShowsOnlyDeclaredClients(t *testing.T) {
	res, err := runClientScript(t, &fakeClients{}, []string{"northwind", "crm-prod"}, `
		local out = {}
		for _, c in ipairs(formidable.client.list()) do out[#out + 1] = c.id end
		return out`)
	if err != nil {
		t.Fatal(err)
	}
	got := res.Value.([]any)
	if len(got) != 2 || got[0] != "crm-prod" || got[1] != "northwind" {
		t.Errorf("list = %v, want the two declared clients", got)
	}
}

func TestClient_ItemsPassesOptionsAndReturnsPage(t *testing.T) {
	f := &fakeClients{items: map[string]any{
		"items":       []any{map[string]any{"id": "ALFKI", "label": "Alfreds", "fields": map[string]any{"city": "Berlin"}}},
		"next_cursor": "page-2",
	}}
	res, err := runClientScript(t, f, []string{"northwind"}, `
		local page = formidable.client.items("northwind", "customers", {
			search = "alf", limit = 25, offset = 50, cursor = "page-1",
			select = { "city", "country" },
			params = { tenant = "acme", version = 2, active = true },
		})
		return { id = page.items[1].id, city = page.items[1].fields.city, next = page.next_cursor }`)
	if err != nil {
		t.Fatal(err)
	}
	got := res.Value.(map[string]any)
	if got["id"] != "ALFKI" || got["city"] != "Berlin" || got["next"] != "page-2" {
		t.Errorf("result = %v", got)
	}
	q := f.gotQuery
	if f.gotClient != "northwind" || f.gotRes != "customers" || q.Search != "alf" || q.Limit != 25 || q.Offset != 50 || q.Cursor != "page-1" {
		t.Errorf("call = %s/%s %+v", f.gotClient, f.gotRes, q)
	}
	if len(q.Select) != 2 || q.Select[0] != "city" || q.Select[1] != "country" {
		t.Errorf("select = %v", q.Select)
	}
	if q.Params["tenant"] != "acme" || q.Params["version"] != "2" || q.Params["active"] != "true" {
		t.Errorf("params = %v", q.Params)
	}
	if f.gotCtx == nil {
		t.Error("the run context must reach the call, so Stop cancels it")
	}
}

func TestClient_ItemsWithoutOptions(t *testing.T) {
	f := &fakeClients{items: map[string]any{"items": []any{}}}
	if _, err := runClientScript(t, f, []string{"northwind"}, `
		return formidable.client.items("northwind", "customers")`); err != nil {
		t.Fatal(err)
	}
	if f.gotQuery.Limit != 0 || f.gotQuery.Select != nil || f.gotQuery.Params != nil {
		t.Errorf("query = %+v, want the zero value", f.gotQuery)
	}
}

func TestClient_FetchResolvesOneItem(t *testing.T) {
	f := &fakeClients{item: map[string]any{"id": "ALFKI", "label": "Alfreds"}}
	res, err := runClientScript(t, f, []string{"northwind"}, `
		return formidable.client.fetch("northwind", "customers", "ALFKI", { select = { "city" } }).label`)
	if err != nil {
		t.Fatal(err)
	}
	if res.Value != "Alfreds" || f.gotID != "ALFKI" || len(f.gotQuery.Select) != 1 {
		t.Errorf("value = %v, id = %q, query = %+v", res.Value, f.gotID, f.gotQuery)
	}
}

func TestClient_ResourcesDescribesAClient(t *testing.T) {
	f := &fakeClients{}
	res, err := runClientScript(t, f, []string{"northwind"}, `
		return formidable.client.resources("northwind")[1].key`)
	if err != nil {
		t.Fatal(err)
	}
	if res.Value != "customers" || f.gotClient != "northwind" {
		t.Errorf("value = %v, client = %q", res.Value, f.gotClient)
	}
}

func TestClient_UndeclaredClientIsRefusedBeforeAnyCall(t *testing.T) {
	for name, body := range map[string]string{
		"items":      `return formidable.client.items("private", "customers")`,
		"fetch":      `return formidable.client.fetch("private", "customers", "1")`,
		"resources":  `return formidable.client.resources("private")`,
		"operations": `return formidable.client.operations("private")`,
		"call":       `return formidable.client.call("private", "listPets")`,
	} {
		t.Run(name, func(t *testing.T) {
			f := &fakeClients{}
			_, err := runClientScript(t, f, []string{"northwind"}, body)
			if err == nil || !strings.Contains(err.Error(), "api_clients") {
				t.Fatalf("err = %v, want a pointer at api_clients in plugin.json", err)
			}
			if f.calls != 0 {
				t.Errorf("an undeclared client was called %d times", f.calls)
			}
		})
	}
}

func TestClient_BadOptionsAreRefused(t *testing.T) {
	for name, opts := range map[string]string{
		"unknown key":       `{ serach = "x" }`,
		"not a table":       `"alf"`,
		"limit not number":  `{ limit = "ten" }`,
		"negative limit":    `{ limit = -1 }`,
		"fractional offset": `{ offset = 1.5 }`,
		"select not table":  `{ select = "city" }`,
		"select not string": `{ select = { 1, 2 } }`,
		"params not table":  `{ params = "tenant=acme" }`,
		"param is a table":  `{ params = { tenant = {} } }`,
	} {
		t.Run(name, func(t *testing.T) {
			f := &fakeClients{}
			_, err := runClientScript(t, f, []string{"northwind"},
				`return formidable.client.items("northwind", "customers", `+opts+`)`)
			if err == nil || !strings.Contains(err.Error(), "client.items") {
				t.Fatalf("err = %v, want a client.items error", err)
			}
			if f.calls != 0 {
				t.Error("a call went out despite bad options")
			}
		})
	}
}

func TestClient_FetchRequiresAnID(t *testing.T) {
	f := &fakeClients{}
	if _, err := runClientScript(t, f, []string{"northwind"},
		`return formidable.client.fetch("northwind", "customers", "")`); err == nil {
		t.Fatal("an empty id was accepted")
	}
	if f.calls != 0 {
		t.Error("a call went out without an id")
	}
}

func TestClient_RemoteErrorIsCatchableWithPcall(t *testing.T) {
	f := &fakeClients{err: errors.New("unauthorized: GET /customers returned 401")}
	res, err := runClientScript(t, f, []string{"northwind"}, `
		local ok, msg = pcall(formidable.client.items, "northwind", "customers")
		return { ok = ok, msg = tostring(msg) }`)
	if err != nil {
		t.Fatal(err)
	}
	got := res.Value.(map[string]any)
	if got["ok"] != false || !strings.Contains(got["msg"].(string), "client.items: unauthorized") {
		t.Errorf("pcall = %v", got)
	}
}

func TestClient_NotConfigured(t *testing.T) {
	for _, fn := range []string{"list()", `resources("x")`, `items("x", "y")`, `fetch("x", "y", "1")`, `snapshot({ id = "1" })`, `operations("x")`, `call("x", "op")`} {
		_, err := runClientScript(t, nil, []string{"x"}, `return formidable.client.`+fn)
		if err == nil || !strings.Contains(err.Error(), "client: not configured") {
			t.Errorf("%s: err = %v", fn, err)
		}
	}
}

func TestManagerRun_HandsDeclaredClientsToTheScript(t *testing.T) {
	inst := newFakeInstaller()
	inst.have["northwind"] = true
	m, pluginsDir := newClientManager(t, inst)
	f := &fakeClients{items: map[string]any{"items": []any{}}}
	m.deps.Client = f
	writePlugin(t, pluginsDir, "demo", clientManifest("demo", `["northwind"]`), `function run()
		formidable.client.items("northwind", "customers")
		return pcall(formidable.client.items, "private", "customers")
	end`)
	if err := m.Refresh(); err != nil {
		t.Fatal(err)
	}
	res, err := m.Run("demo", "run", nil)
	if err != nil {
		t.Fatal(err)
	}
	if res.Value != false || f.calls != 1 || f.gotClient != "northwind" {
		t.Errorf("value = %v, calls = %d, client = %q", res.Value, f.calls, f.gotClient)
	}
}

func TestClient_SnapshotBuildsTheStoredPickFromAnItem(t *testing.T) {
	f := &fakeClients{item: map[string]any{"id": "ALFKI", "label": "Alfreds", "fields": map[string]any{"city": "Berlin"}, "fetched": "2026-10-12T10:00:00Z"}}
	res, err := runClientScript(t, f, nil, `
		local item = { id = "ALFKI", label = "Alfreds", fields = { city = "Berlin", country = "DE" } }
		return formidable.client.snapshot(item, { "city" }).fetched`)
	if err != nil {
		t.Fatal(err)
	}
	if res.Value != "2026-10-12T10:00:00Z" {
		t.Errorf("value = %v", res.Value)
	}
	if f.gotItem["id"] != "ALFKI" || len(f.gotKeys) != 1 || f.gotKeys[0] != "city" {
		t.Errorf("item = %v, keys = %v", f.gotItem, f.gotKeys)
	}
}

func TestClient_SnapshotWithoutKeysPassesNil(t *testing.T) {
	f := &fakeClients{item: map[string]any{"id": "1"}}
	if _, err := runClientScript(t, f, nil, `return formidable.client.snapshot({ id = "1", label = "One" })`); err != nil {
		t.Fatal(err)
	}
	if f.gotKeys != nil {
		t.Errorf("keys = %v, want nil so the store keeps every field the item carries", f.gotKeys)
	}
}

func TestClient_SnapshotRefusesBadArguments(t *testing.T) {
	for name, args := range map[string]string{
		"item not a table": `"ALFKI"`,
		"item is a list":   `{ "a", "b" }`,
		"keys not a table": `{ id = "1" }, "city"`,
		"keys not strings": `{ id = "1" }, { 1 }`,
	} {
		t.Run(name, func(t *testing.T) {
			f := &fakeClients{}
			_, err := runClientScript(t, f, nil, `return formidable.client.snapshot(`+args+`)`)
			if err == nil || !strings.Contains(err.Error(), "client.snapshot") {
				t.Fatalf("err = %v, want a client.snapshot error", err)
			}
			if f.calls != 0 {
				t.Error("the store was reached despite bad arguments")
			}
		})
	}
}

func TestClient_SnapshotStoreErrorIsRaised(t *testing.T) {
	f := &fakeClients{err: errors.New("item has no id")}
	_, err := runClientScript(t, f, nil, `return formidable.client.snapshot({ label = "nameless" })`)
	if err == nil || !strings.Contains(err.Error(), "client.snapshot: item has no id") {
		t.Fatalf("err = %v", err)
	}
}

func TestClient_CallRunsAnOperationByID(t *testing.T) {
	f := &fakeClients{item: map[string]any{"status": float64(200), "data": []any{map[string]any{"name": "Rex"}}}}
	res, err := runClientScript(t, f, []string{"petstore"}, `
		local r = formidable.client.call("petstore", "findPetsByStatus", { status = "available", limit = 5, strict = true })
		return { status = r.status, first = r.data[1].name }`)
	if err != nil {
		t.Fatal(err)
	}
	got := res.Value.(map[string]any)
	if got["status"] != float64(200) || got["first"] != "Rex" {
		t.Errorf("result = %v", got)
	}
	if f.gotClient != "petstore" || f.gotOp != "findPetsByStatus" || f.gotCtx == nil {
		t.Errorf("call = %s/%s", f.gotClient, f.gotOp)
	}
	if f.gotParams["status"] != "available" || f.gotParams["limit"] != "5" || f.gotParams["strict"] != "true" {
		t.Errorf("params = %v", f.gotParams)
	}
}

func TestClient_CallWithoutParams(t *testing.T) {
	f := &fakeClients{item: map[string]any{"status": float64(200)}}
	if _, err := runClientScript(t, f, []string{"petstore"}, `return formidable.client.call("petstore", "getInventory")`); err != nil {
		t.Fatal(err)
	}
	if f.gotParams != nil {
		t.Errorf("params = %v, want nil", f.gotParams)
	}
}

func TestClient_CallRefusesBadParams(t *testing.T) {
	for name, params := range map[string]string{"not a table": `"status=available"`, "nested table": `{ status = { "a" } }`} {
		t.Run(name, func(t *testing.T) {
			f := &fakeClients{}
			_, err := runClientScript(t, f, []string{"petstore"}, `return formidable.client.call("petstore", "findPetsByStatus", `+params+`)`)
			if err == nil || !strings.Contains(err.Error(), "client.call") || f.calls != 0 {
				t.Fatalf("err = %v, calls = %d", err, f.calls)
			}
		})
	}
}

func TestClient_OperationsListsTheDocument(t *testing.T) {
	f := &fakeClients{}
	res, err := runClientScript(t, f, []string{"petstore"}, `return formidable.client.operations("petstore")[1].id`)
	if err != nil {
		t.Fatal(err)
	}
	if res.Value != "listPets" {
		t.Errorf("value = %v", res.Value)
	}
}
