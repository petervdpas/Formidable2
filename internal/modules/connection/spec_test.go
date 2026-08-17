package connection

import (
	"strings"
	"testing"
)

const specV3JSON = `{
  "openapi": "3.0.3",
  "info": {"title": "CRM", "version": "2.1.0"},
  "servers": [{"url": "https://crm.example.com/v2"}],
  "components": {
    "parameters": {
      "Limit": {"name": "limit", "in": "query", "schema": {"type": "integer"}}
    }
  },
  "paths": {
    "/customers": {
      "parameters": [{"name": "tenant", "in": "header", "required": true, "schema": {"type": "string"}}],
      "get": {
        "operationId": "listCustomers",
        "summary": "List customers",
        "parameters": [
          {"name": "q", "in": "query", "schema": {"type": "string"}},
          {"$ref": "#/components/parameters/Limit"}
        ]
      },
      "post": {"operationId": "createCustomer"}
    },
    "/customers/{customerId}": {
      "parameters": [{"name": "tenant", "in": "header", "required": true, "schema": {"type": "string"}}],
      "get": {
        "operationId": "getCustomer",
        "parameters": [
          {"name": "customerId", "in": "path", "required": true, "schema": {"type": "string"}}
        ]
      }
    }
  }
}`

const specV3YAML = `
openapi: 3.0.0
info:
  title: Tiny
  version: "1.0"
paths:
  /things:
    get:
      operationId: listThings
`

const specV2JSON = `{
  "swagger": "2.0",
  "info": {"title": "Legacy", "version": "1.4"},
  "host": "legacy.example.com",
  "basePath": "/api",
  "schemes": ["https"],
  "paths": {
    "/orders": {
      "get": {
        "operationId": "listOrders",
        "parameters": [{"name": "since", "in": "query", "type": "string"}],
        "responses": {"200": {"description": "ok"}}
      }
    }
  }
}`

const specNoOperationID = `{
  "openapi": "3.0.0",
  "info": {"title": "Bare", "version": "1.0"},
  "paths": {
    "/widgets": {"get": {"summary": "list widgets"}}
  }
}`

const specDuplicateOperationID = `{
  "openapi": "3.0.0",
  "info": {"title": "Dup", "version": "1.0"},
  "paths": {
    "/a": {"get": {"operationId": "same"}},
    "/b": {"get": {"operationId": "same"}}
  }
}`

func mustParse(t *testing.T, src string) *Catalog {
	t.Helper()
	cat, err := ParseSpec([]byte(src))
	if err != nil {
		t.Fatalf("ParseSpec: %v", err)
	}
	return cat
}

func TestParseSpec_V3_InfoAndServers(t *testing.T) {
	cat := mustParse(t, specV3JSON)
	if cat.Title != "CRM" || cat.Version != "2.1.0" {
		t.Fatalf("info = %q/%q, want CRM/2.1.0", cat.Title, cat.Version)
	}
	if cat.SpecFormat != FormatOpenAPI3 {
		t.Fatalf("SpecFormat = %q, want %q", cat.SpecFormat, FormatOpenAPI3)
	}
	if len(cat.Servers) != 1 || cat.Servers[0] != "https://crm.example.com/v2" {
		t.Fatalf("Servers = %v", cat.Servers)
	}
}

func TestParseSpec_V3_OperationsAndIDs(t *testing.T) {
	cat := mustParse(t, specV3JSON)
	for _, id := range []string{"listCustomers", "createCustomer", "getCustomer"} {
		if _, ok := cat.Op(id); !ok {
			t.Errorf("missing operation %q", id)
		}
	}
	op, _ := cat.Op("listCustomers")
	if op.Method != "GET" || op.Path != "/customers" {
		t.Errorf("listCustomers = %s %s", op.Method, op.Path)
	}
	if op.Summary != "List customers" {
		t.Errorf("summary = %q", op.Summary)
	}
	if op.Synthetic {
		t.Error("operation with an operationId must not be marked synthetic")
	}
}

func TestParseSpec_V3_ResolvesParamRef(t *testing.T) {
	cat := mustParse(t, specV3JSON)
	op, _ := cat.Op("listCustomers")
	p, ok := op.Param("limit")
	if !ok {
		t.Fatal("$ref parameter not resolved")
	}
	if p.In != InQuery || p.Type != "integer" {
		t.Fatalf("limit param = %+v", p)
	}
}

func TestParseSpec_V3_MergesPathLevelParams(t *testing.T) {
	cat := mustParse(t, specV3JSON)
	for _, id := range []string{"listCustomers", "createCustomer"} {
		op, _ := cat.Op(id)
		p, ok := op.Param("tenant")
		if !ok {
			t.Errorf("%s: path-level param not merged", id)
			continue
		}
		if p.In != InHeader || !p.Required {
			t.Errorf("%s: tenant = %+v", id, p)
		}
	}
}

func TestParseSpec_V3_PathParamIsRequired(t *testing.T) {
	cat := mustParse(t, specV3JSON)
	op, _ := cat.Op("getCustomer")
	pp := op.PathParams()
	if len(pp) != 1 || pp[0].Name != "customerId" || !pp[0].Required {
		t.Fatalf("path params = %+v", pp)
	}
}

func TestParseSpec_V3_YAML(t *testing.T) {
	cat := mustParse(t, specV3YAML)
	if cat.Title != "Tiny" {
		t.Fatalf("title = %q", cat.Title)
	}
	if _, ok := cat.Op("listThings"); !ok {
		t.Fatal("listThings missing")
	}
}

func TestParseSpec_Swagger2_Converts(t *testing.T) {
	cat := mustParse(t, specV2JSON)
	if cat.SpecFormat != FormatSwagger2 {
		t.Fatalf("SpecFormat = %q, want %q", cat.SpecFormat, FormatSwagger2)
	}
	if cat.Title != "Legacy" {
		t.Fatalf("title = %q", cat.Title)
	}
	op, ok := cat.Op("listOrders")
	if !ok {
		t.Fatal("listOrders missing after 2.0 conversion")
	}
	if op.Method != "GET" || op.Path != "/orders" {
		t.Fatalf("listOrders = %s %s", op.Method, op.Path)
	}
	if _, ok := op.Param("since"); !ok {
		t.Error("since param lost in conversion")
	}
}

func TestParseSpec_Swagger2_DerivesServerFromHostBasePath(t *testing.T) {
	cat := mustParse(t, specV2JSON)
	if len(cat.Servers) == 0 || !strings.Contains(cat.Servers[0], "legacy.example.com") {
		t.Fatalf("servers = %v, want one containing the host", cat.Servers)
	}
	if !strings.HasPrefix(cat.Servers[0], "https://") {
		t.Fatalf("servers[0] = %q, want the https scheme applied", cat.Servers[0])
	}
}

func TestParseSpec_SynthesizesMissingOperationID(t *testing.T) {
	cat := mustParse(t, specNoOperationID)
	op, ok := cat.Op("get:/widgets")
	if !ok {
		t.Fatalf("synthetic id missing; got %+v", cat.Operations)
	}
	if !op.Synthetic {
		t.Error("Synthetic flag not set")
	}
	if op.Method != "GET" || op.Path != "/widgets" {
		t.Errorf("op = %s %s", op.Method, op.Path)
	}
}

func TestParseSpec_DeterministicOrder(t *testing.T) {
	first := mustParse(t, specV3JSON)
	for range 20 {
		next := mustParse(t, specV3JSON)
		if len(next.Operations) != len(first.Operations) {
			t.Fatalf("operation count drifted: %d vs %d", len(next.Operations), len(first.Operations))
		}
		for j := range next.Operations {
			if next.Operations[j].ID != first.Operations[j].ID {
				t.Fatalf("order drifted at %d: %q vs %q", j, next.Operations[j].ID, first.Operations[j].ID)
			}
		}
	}
	want := []string{"listCustomers", "createCustomer", "getCustomer"}
	got := make([]string, len(first.Operations))
	for i, op := range first.Operations {
		got[i] = op.ID
	}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("order = %v, want %v (sorted by path, then canonical method)", got, want)
	}
}

func TestParseSpec_DuplicateOperationIDIsAnError(t *testing.T) {
	if _, err := ParseSpec([]byte(specDuplicateOperationID)); err == nil {
		t.Fatal("want an error for a spec with duplicate operationIds")
	}
}

func TestParseSpec_Unhappy(t *testing.T) {
	cases := []struct {
		name string
		src  string
	}{
		{"empty", ""},
		{"whitespace only", "   \n\t "},
		{"not a document", "this is not a spec"},
		{"json but not a spec", `{"hello": "world"}`},
		{"truncated json", `{"openapi": "3.0.0", "info": {`},
		{"no version marker", `{"info": {"title": "x", "version": "1"}, "paths": {}}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := ParseSpec([]byte(tc.src)); err == nil {
				t.Fatal("want an error")
			}
		})
	}
}

func TestParseSpec_NilCatalogOpIsSafe(t *testing.T) {
	var cat *Catalog
	if _, ok := cat.Op("anything"); ok {
		t.Fatal("nil catalog must not report a hit")
	}
}

func TestSpecExtension(t *testing.T) {
	cases := map[string]string{
		`{"openapi":"3.0.0"}`:   ".json",
		"  \n {\"a\":1}":        ".json",
		"openapi: 3.0.0":        ".yaml",
		"# comment\nopenapi: 3": ".yaml",
		"":                      ".yaml",
	}
	for src, want := range cases {
		if got := SpecExtension([]byte(src)); got != want {
			t.Errorf("SpecExtension(%q) = %q, want %q", src, got, want)
		}
	}
}
