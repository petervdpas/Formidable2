package connection

import (
	"errors"
	"os"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// bundle is a shipped client as a plugin folder would hold it: one definition
// and the spec it names, keyed by path relative to the api-clients folder.
func bundle(t *testing.T, c Connection, spec string) map[string]string {
	t.Helper()
	def, err := yaml.Marshal(c)
	if err != nil {
		t.Fatal(err)
	}
	return map[string]string{
		c.ID + ".yaml":        string(def),
		"specs/" + c.SpecFile: spec,
	}
}

func readerOf(files map[string]string) BundleReader {
	return func(name string) ([]byte, error) {
		v, ok := files[name]
		if !ok {
			return nil, os.ErrNotExist
		}
		return []byte(v), nil
	}
}

func TestInstall_WritesDefinitionAndSpec(t *testing.T) {
	fs := newMemFS()
	m := NewManager(fs, nil)
	if _, err := m.Install("crm-prod", readerOf(bundle(t, validConn(), specV3JSON))); err != nil {
		t.Fatalf("Install: %v", err)
	}
	c, cat, err := m.Get("crm-prod")
	if err != nil {
		t.Fatalf("Get after install: %v", err)
	}
	if c.BaseURL != "https://crm.example.com/v2" || cat.Title != "CRM" {
		t.Errorf("installed client = %+v, catalog title %q", c, cat.Title)
	}
	if got, _ := fs.LoadFile("api-clients/specs/crm-prod.json"); got != specV3JSON {
		t.Error("spec must be stored byte-identical to the bundled copy")
	}
	if !m.Has("crm-prod") {
		t.Error("Has = false after install")
	}
}

func TestInstall_StoresSpecUnderTheClientID(t *testing.T) {
	fs := newMemFS()
	m := NewManager(fs, nil)
	c := validConn()
	c.SpecFile = "vendor-doc.json"
	if _, err := m.Install("crm-prod", readerOf(bundle(t, c, specV3JSON))); err != nil {
		t.Fatalf("Install: %v", err)
	}
	got, _, err := m.Get("crm-prod")
	if err != nil {
		t.Fatal(err)
	}
	if got.SpecFile != "crm-prod.json" {
		t.Errorf("spec_file = %q, want crm-prod.json", got.SpecFile)
	}
	if fs.FileExists("api-clients/specs/vendor-doc.json") {
		t.Error("bundled spec name must not leak into the store")
	}
}

func TestInstall_IsCreateOnly(t *testing.T) {
	m, fs := seeded(t)
	before, _ := fs.LoadFile("api-clients/crm-prod.yaml")

	incoming := validConn()
	incoming.BaseURL = "https://evil.example.com"
	_, err := m.Install("crm-prod", readerOf(bundle(t, incoming, specV3JSON)))
	if !errors.Is(err, ErrExists) {
		t.Fatalf("err = %v, want ErrExists", err)
	}
	if after, _ := fs.LoadFile("api-clients/crm-prod.yaml"); after != before {
		t.Error("an existing definition must be left untouched")
	}
}

func TestInstall_RefusesSpecNameAnotherClientUses(t *testing.T) {
	m, fs := seeded(t)
	if _, err := m.ImportSpec("billing", []byte(specV3JSON)); err != nil {
		t.Fatal(err)
	}
	other := validConn()
	other.ID, other.SpecFile = "other", "billing.json"
	if err := m.Save(&other); err != nil {
		t.Fatal(err)
	}
	fs.files["api-clients/specs/billing.json"] = specV3JSON + "\n"

	incoming := validConn()
	incoming.ID, incoming.SpecFile = "billing", "billing.json"
	_, err := m.Install("billing", readerOf(bundle(t, incoming, specV3JSON)))
	if !errors.Is(err, ErrExists) {
		t.Fatalf("err = %v, want ErrExists", err)
	}
	if got, _ := fs.LoadFile("api-clients/specs/billing.json"); got != specV3JSON+"\n" {
		t.Error("a spec another client uses must not be overwritten")
	}
	if m.Has("billing") {
		t.Error("definition written despite the refused spec")
	}
}

func TestInstall_Unhappy(t *testing.T) {
	good := validConn()

	mismatched := bundle(t, good, specV3JSON)
	mismatched["other.yaml"] = mismatched["crm-prod.yaml"]

	noSpec := bundle(t, good, specV3JSON)
	delete(noSpec, "specs/crm-prod.json")

	escaping := validConn()
	escaping.SpecFile = "../../secrets.json"

	unbound := validConn()
	unbound.Resources[0].List.Operation = "noSuchOperation"

	cases := []struct {
		name  string
		id    string
		files map[string]string
	}{
		{"bad id", "../crm", bundle(t, good, specV3JSON)},
		{"no document", "absent", bundle(t, good, specV3JSON)},
		{"definition is not yaml", "crm-prod", map[string]string{"crm-prod.yaml": "\t: ["}},
		{"id differs from the file name", "other", mismatched},
		{"missing spec", "crm-prod", noSpec},
		{"spec is not a document", "crm-prod", bundle(t, good, "not a spec")},
		{"spec file escapes the bundle", "crm-prod", bundle(t, escaping, specV3JSON)},
		{"binding not in the spec", "crm-prod", bundle(t, unbound, specV3JSON)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fs := newMemFS()
			m := NewManager(fs, nil)
			if _, err := m.Install(tc.id, readerOf(tc.files)); err == nil {
				t.Fatal("Install succeeded, want an error")
			}
			if len(fs.files) != 0 {
				t.Errorf("a refused install must write nothing; files = %v", fs.files)
			}
		})
	}
}

func TestInstall_InvalidBindingIsAValidationError(t *testing.T) {
	c := validConn()
	c.Resources[0].List.Operation = "noSuchOperation"
	m := NewManager(newMemFS(), nil)
	_, err := m.Install("crm-prod", readerOf(bundle(t, c, specV3JSON)))
	var vf *ValidationFailedError
	if !errors.As(err, &vf) {
		t.Fatalf("err = %v, want *ValidationFailedError", err)
	}
}

func TestInstall_RollsBackSpecWhenDefinitionWriteFails(t *testing.T) {
	fs := newMemFS()
	fs.failOn = "crm-prod.yaml"
	m := NewManager(fs, nil)
	if _, err := m.Install("crm-prod", readerOf(bundle(t, validConn(), specV3JSON))); err == nil {
		t.Fatal("Install succeeded, want the write error")
	}
	for p := range fs.files {
		if strings.HasPrefix(p, "api-clients/") {
			t.Errorf("left behind after a failed install: %s", p)
		}
	}
}

func TestInstall_NilReaderAndNoFilesystem(t *testing.T) {
	if _, err := NewManager(newMemFS(), nil).Install("crm-prod", nil); err == nil {
		t.Error("nil reader accepted")
	}
	if _, err := NewManager(nil, nil).Install("crm-prod", readerOf(bundle(t, validConn(), specV3JSON))); err == nil {
		t.Error("no filesystem accepted")
	}
	if NewManager(nil, nil).Has("crm-prod") {
		t.Error("Has = true without a filesystem")
	}
}

// derivableSpec has one collection the document fully describes (products, with
// its entity operation and a response schema), one that needs a parameter
// nothing can supply (orders), a top-level bearer requirement and a server.
const derivableSpec = `{
  "openapi": "3.0.3",
  "info": {"title": "Shop API", "version": "1.0.0"},
  "servers": [{"url": "https://shop.example.com/v1"}],
  "security": [{"token": []}],
  "components": {
    "securitySchemes": {"token": {"type": "http", "scheme": "bearer"}},
    "schemas": {"Product": {"type": "object", "properties": {
      "id": {"type": "string"}, "name": {"type": "string"}, "price": {"type": "number"}}}}
  },
  "paths": {
    "/orders": {"get": {"operationId": "listOrders",
      "parameters": [{"name": "tenant", "in": "query", "required": true, "schema": {"type": "string"}}],
      "responses": {"200": {"description": "ok", "content": {"application/json": {"schema": {
        "type": "array", "items": {"type": "object", "properties": {"id": {"type": "string"}, "name": {"type": "string"}}}}}}}}}},
    "/products": {"get": {"operationId": "listProducts",
      "responses": {"200": {"description": "ok", "content": {"application/json": {"schema": {
        "type": "array", "items": {"$ref": "#/components/schemas/Product"}}}}}}}},
    "/products/{id}": {"get": {"operationId": "getProduct",
      "parameters": [{"name": "id", "in": "path", "required": true, "schema": {"type": "string"}}],
      "responses": {"200": {"description": "ok", "content": {"application/json": {"schema": {
        "$ref": "#/components/schemas/Product"}}}}}}}
  }
}`

func TestInstall_DerivesADefinitionFromTheDocumentAlone(t *testing.T) {
	fs := newMemFS()
	m := NewManager(fs, nil)
	derived, err := m.Install("shop", readerOf(map[string]string{"specs/shop.json": derivableSpec}))
	if err != nil {
		t.Fatalf("Install: %v", err)
	}
	if !derived {
		t.Error("derived = false for a bundle with no definition")
	}
	c, cat, err := m.Get("shop")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if c.Name != "Shop API" || c.BaseURL != "" || c.Auth.Kind != AuthBearer || c.SpecFile != "shop.json" {
		t.Errorf("derived client = %+v", c)
	}
	if len(c.Resources) != 1 {
		t.Fatalf("resources = %+v, want only the one the document fully describes", c.Resources)
	}
	r := c.Resources[0]
	if r.List.Operation != "listProducts" || r.Get.Operation != "getProduct" || r.IDPath != "/id" || r.LabelPath != "/name" {
		t.Errorf("resource = %+v", r)
	}
	if errs := Validate(c, cat); len(errs) != 0 {
		t.Errorf("a derived client must validate: %v", errs)
	}
	if got, _ := fs.LoadFile("api-clients/specs/shop.json"); got != derivableSpec {
		t.Error("document must be stored byte-identical")
	}
}

func TestInstall_ShippedDefinitionIsNotDerived(t *testing.T) {
	m := NewManager(newMemFS(), nil)
	derived, err := m.Install("crm-prod", readerOf(bundle(t, validConn(), specV3JSON)))
	if err != nil || derived {
		t.Errorf("derived = %v, err = %v", derived, err)
	}
}

func TestInstall_DerivesFromAYAMLDocument(t *testing.T) {
	doc := "openapi: 3.0.3\ninfo:\n  title: Tiny\n  version: '1'\nservers:\n  - url: https://tiny.example.com\npaths: {}\n"
	fs := newMemFS()
	m := NewManager(fs, nil)
	if _, err := m.Install("tiny", readerOf(map[string]string{"specs/tiny.yml": doc})); err != nil {
		t.Fatalf("Install: %v", err)
	}
	if !fs.FileExists("api-clients/specs/tiny.yaml") {
		t.Errorf("yaml document not stored under the client id; files = %v", fs.files)
	}
}

func TestInstall_DerivedClientWithoutAnAbsoluteServerIsRefused(t *testing.T) {
	doc := `{"openapi": "3.0.3", "info": {"title": "Rel", "version": "1"}, "servers": [{"url": "/api"}], "paths": {}}`
	fs := newMemFS()
	m := NewManager(fs, nil)
	_, err := m.Install("rel", readerOf(map[string]string{"specs/rel.json": doc}))
	var vf *ValidationFailedError
	if !errors.As(err, &vf) {
		t.Fatalf("err = %v, want a validation failure: no document can supply the host", err)
	}
	if len(fs.files) != 0 {
		t.Errorf("a refused install must write nothing; files = %v", fs.files)
	}
}

func TestInstall_UnreadableDefinitionIsNotMistakenForDocumentOnly(t *testing.T) {
	fs := newMemFS()
	m := NewManager(fs, nil)
	read := func(name string) ([]byte, error) {
		if name == "shop.yaml" {
			return nil, errors.New("permission denied")
		}
		return []byte(derivableSpec), nil
	}
	if _, err := m.Install("shop", read); err == nil {
		t.Fatal("an unreadable definition was silently replaced by a derived one")
	}
	if len(fs.files) != 0 {
		t.Errorf("files = %v", fs.files)
	}
}

func TestDerive_FallsBackToTheIDForAName(t *testing.T) {
	c := Derive("shop", &Catalog{})
	if c.Name != "shop" || len(c.Resources) != 0 {
		t.Errorf("derived = %+v", c)
	}
	if got := Derive("shop", nil); got.ID != "shop" {
		t.Errorf("nil catalog: %+v", got)
	}
}

func TestParseSpec_ReadsTheSecurityRequirement(t *testing.T) {
	for name, tc := range map[string]struct {
		security string
		schemes  string
		want     Auth
	}{
		"bearer":            {`[{"t": []}]`, `{"t": {"type": "http", "scheme": "bearer"}}`, Auth{Kind: AuthBearer}},
		"api key in header": {`[{"k": []}]`, `{"k": {"type": "apiKey", "in": "header", "name": "X-Key"}}`, Auth{Kind: AuthAPIKey, In: "header", Name: "X-Key"}},
		"api key in cookie": {`[{"k": []}]`, `{"k": {"type": "apiKey", "in": "cookie", "name": "sid"}}`, Auth{}},
		"basic":             {`[{"b": []}]`, `{"b": {"type": "http", "scheme": "basic"}}`, Auth{}},
		"none required":     {`[]`, `{"t": {"type": "http", "scheme": "bearer"}}`, Auth{}},
		"first usable wins": {`[{"b": [], "k": []}]`, `{"b": {"type": "http", "scheme": "basic"}, "k": {"type": "apiKey", "in": "query", "name": "key"}}`, Auth{Kind: AuthAPIKey, In: "query", Name: "key"}},
	} {
		t.Run(name, func(t *testing.T) {
			doc := `{"openapi": "3.0.3", "info": {"title": "T", "version": "1"}, "paths": {},
				"security": ` + tc.security + `, "components": {"securitySchemes": ` + tc.schemes + `}}`
			cat, err := ParseSpec([]byte(doc))
			if err != nil {
				t.Fatal(err)
			}
			if cat.Auth != tc.want {
				t.Errorf("Auth = %+v, want %+v", cat.Auth, tc.want)
			}
		})
	}
}

func TestCreate_FillsWhatTheDraftLeavesOutFromTheDocument(t *testing.T) {
	fs := newMemFS()
	m := NewManager(fs, nil)
	if err := m.Create(Connection{ID: "shop", BaseURL: "https://acc.shop.example.com/v1"}, []byte(derivableSpec)); err != nil {
		t.Fatalf("Create: %v", err)
	}
	c, _, err := m.Get("shop")
	if err != nil {
		t.Fatal(err)
	}
	if c.Name != "Shop API" || c.Auth.Kind != AuthBearer || c.BaseURL != "https://acc.shop.example.com/v1" || c.SpecFile != "shop.json" {
		t.Errorf("client = %+v", c)
	}
	if len(c.Resources) != 1 || c.Resources[0].List.Operation != "listProducts" {
		t.Errorf("resources = %+v, want the detected one", c.Resources)
	}
}

func TestCreate_KeepsWhatTheDraftStates(t *testing.T) {
	m := NewManager(newMemFS(), nil)
	draft := Connection{
		ID: "shop", Name: "My shop",
		Auth: Auth{Kind: AuthAPIKey, In: InHeader, Name: "X-Key"},
		Resources: []Resource{{
			Key: "orders", List: OpRef{Operation: "listOrders", Params: map[string]string{"tenant": "acme"}},
			IDPath: "/id", LabelPath: "/name",
		}},
	}
	if err := m.Create(draft, []byte(derivableSpec)); err != nil {
		t.Fatalf("Create: %v", err)
	}
	c, _, _ := m.Get("shop")
	if c.Name != "My shop" || c.Auth.Kind != AuthAPIKey || len(c.Resources) != 1 || c.Resources[0].Key != "orders" {
		t.Errorf("client = %+v", c)
	}
}

func TestCreate_IsCreateOnly(t *testing.T) {
	m, fs := seeded(t)
	before, _ := fs.LoadFile("api-clients/crm-prod.yaml")
	err := m.Create(Connection{ID: "crm-prod", BaseURL: "https://evil.example.com"}, []byte(specV3JSON))
	if !errors.Is(err, ErrExists) {
		t.Fatalf("err = %v, want ErrExists", err)
	}
	if after, _ := fs.LoadFile("api-clients/crm-prod.yaml"); after != before {
		t.Error("an existing definition must be left untouched")
	}
}

func TestCreate_Unhappy(t *testing.T) {
	cases := map[string]struct {
		c    Connection
		spec string
	}{
		"bad id":            {Connection{ID: "../shop"}, derivableSpec},
		"not a document":    {Connection{ID: "shop"}, "nope"},
		"bad base url":      {Connection{ID: "shop", BaseURL: "ftp://x"}, derivableSpec},
		"unknown auth kind": {Connection{ID: "shop", Auth: Auth{Kind: "magic"}}, derivableSpec},
		"unbound resource":  {Connection{ID: "shop", Resources: []Resource{{Key: "x", List: OpRef{Operation: "nope"}, IDPath: "/id", LabelPath: "/name"}}}, derivableSpec},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			fs := newMemFS()
			if err := NewManager(fs, nil).Create(tc.c, []byte(tc.spec)); err == nil {
				t.Fatal("Create succeeded, want an error")
			}
			if len(fs.files) != 0 {
				t.Errorf("a refused create must write nothing; files = %v", fs.files)
			}
		})
	}
	if err := NewManager(nil, nil).Create(Connection{ID: "shop"}, []byte(derivableSpec)); err == nil {
		t.Error("no filesystem accepted")
	}
}

func TestInstall_PartialDefinitionKeepsItsBaseURLAndTakesTheRestFromTheDocument(t *testing.T) {
	// The document's only server is relative, so the shipped base URL is what
	// makes this client valid at all.
	doc := strings.Replace(derivableSpec, `"https://shop.example.com/v1"`, `"/v1"`, 1)
	m := NewManager(newMemFS(), nil)
	derived, err := m.Install("shop", readerOf(map[string]string{
		"shop.yaml":       "id: shop\nbase_url: https://acc.shop.example.com/v1\n",
		"specs/shop.json": doc,
	}))
	if err != nil {
		t.Fatalf("Install: %v", err)
	}
	if !derived {
		t.Error("derived = false although the resources came from the document")
	}
	c, _, err := m.Get("shop")
	if err != nil {
		t.Fatal(err)
	}
	if c.BaseURL != "https://acc.shop.example.com/v1" || c.Name != "Shop API" || c.Auth.Kind != AuthBearer || len(c.Resources) != 1 {
		t.Errorf("client = %+v", c)
	}
}

func TestInstall_PartialDefinitionWithABadBaseURLIsRefused(t *testing.T) {
	fs := newMemFS()
	_, err := NewManager(fs, nil).Install("shop", readerOf(map[string]string{
		"shop.yaml":       "base_url: not-a-url\n",
		"specs/shop.json": derivableSpec,
	}))
	var vf *ValidationFailedError
	if !errors.As(err, &vf) || len(fs.files) != 0 {
		t.Fatalf("err = %v, files = %v", err, fs.files)
	}
}

func TestValidBaseURL(t *testing.T) {
	for in, want := range map[string]bool{
		"https://api.example.com/v2": true, "http://localhost:8080": true,
		"": false, "/v2": false, "api.example.com": false, "ftp://x.example.com": false, "https://": false, "::": false,
	} {
		if got := ValidBaseURL(in); got != want {
			t.Errorf("ValidBaseURL(%q) = %v, want %v", in, got, want)
		}
	}
}
