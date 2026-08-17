package connection

import (
	"strings"
	"testing"
)

const specTwoPathParams = `{
  "openapi": "3.0.0",
  "info": {"title": "Nested", "version": "1.0"},
  "paths": {
    "/orgs/{orgId}/users/{userId}": {
      "get": {
        "operationId": "getOrgUser",
        "parameters": [
          {"name": "orgId", "in": "path", "required": true, "schema": {"type": "string"}},
          {"name": "userId", "in": "path", "required": true, "schema": {"type": "string"}}
        ]
      }
    },
    "/orgs": {"get": {"operationId": "listOrgs"}}
  }
}`

const specNoServers = `{
  "openapi": "3.0.0",
  "info": {"title": "Serverless", "version": "1.0"},
  "paths": {"/x": {"get": {"operationId": "listX"}}}
}`

// validConn is the baseline every unhappy case mutates: it must validate clean
// against specV3JSON, so a new rule that breaks it shows up immediately.
func validConn() Connection {
	return Connection{
		ID:       "crm-prod",
		Name:     "CRM Production",
		BaseURL:  "https://crm.example.com/v2",
		SpecFile: "crm-prod.json",
		Auth:     Auth{Kind: AuthBearer},
		Resources: []Resource{{
			Key:         "customers",
			Label:       "Customers",
			List:        OpRef{Operation: "listCustomers", Params: map[string]string{"tenant": "acme"}},
			Get:         OpRef{Operation: "getCustomer", Params: map[string]string{"tenant": "acme"}},
			IDPath:      "/id",
			LabelPath:   "/name",
			SearchParam: "q",
		}},
	}
}

func types(errs []ValidationError) string {
	out := make([]string, len(errs))
	for i, e := range errs {
		out[i] = e.Type
	}
	return strings.Join(out, ",")
}

func hasType(errs []ValidationError, want string) bool {
	for _, e := range errs {
		if e.Type == want {
			return true
		}
	}
	return false
}

func TestValidate_Baseline(t *testing.T) {
	cat := mustParse(t, specV3JSON)
	c := validConn()
	if errs := Validate(&c, cat); len(errs) != 0 {
		t.Fatalf("baseline connection must validate clean, got: %s", types(errs))
	}
}

func TestValidate_ListOnlyResourceIsAllowed(t *testing.T) {
	cat := mustParse(t, specV3JSON)
	c := validConn()
	c.Resources[0].Get = OpRef{}
	if errs := Validate(&c, cat); len(errs) != 0 {
		t.Fatalf("a resource without a get binding is legal, got: %s", types(errs))
	}
}

func TestValidate_EmptyPointersMeanTheItemItself(t *testing.T) {
	cat := mustParse(t, specV3JSON)
	c := validConn()
	c.Resources[0].IDPath = ""
	c.Resources[0].LabelPath = ""
	if errs := Validate(&c, cat); len(errs) != 0 {
		t.Fatalf("empty pointers are legal, got: %s", types(errs))
	}
}

func TestValidate_BaseURLFallsBackToSpecServer(t *testing.T) {
	cat := mustParse(t, specV3JSON)
	c := validConn()
	c.BaseURL = ""
	if errs := Validate(&c, cat); len(errs) != 0 {
		t.Fatalf("empty base URL with a spec server is legal, got: %s", types(errs))
	}
}

func TestValidate_Mutations(t *testing.T) {
	cases := []struct {
		name   string
		spec   string
		mutate func(*Connection)
		want   string
	}{
		{"empty id", specV3JSON, func(c *Connection) { c.ID = "" }, "invalid-id"},
		{"id with spaces", specV3JSON, func(c *Connection) { c.ID = "CRM Prod" }, "invalid-id"},
		{"id with slash", specV3JSON, func(c *Connection) { c.ID = "../escape" }, "invalid-id"},
		{"empty name", specV3JSON, func(c *Connection) { c.Name = "" }, "missing-name"},
		{"empty spec file", specV3JSON, func(c *Connection) { c.SpecFile = "" }, "missing-spec"},

		{"base url is not a url", specV3JSON, func(c *Connection) { c.BaseURL = "not a url" }, "invalid-base-url"},
		{"base url is relative", specV3JSON, func(c *Connection) { c.BaseURL = "/v2" }, "invalid-base-url"},
		{"base url scheme is not http", specV3JSON, func(c *Connection) { c.BaseURL = "ftp://crm.example.com" }, "invalid-base-url"},
		{"no base url and no spec server", specNoServers, func(c *Connection) {
			c.BaseURL = ""
			c.Resources[0] = Resource{Key: "x", List: OpRef{Operation: "listX"}}
		}, "no-server"},

		{"unknown auth kind", specV3JSON, func(c *Connection) { c.Auth = Auth{Kind: "magic"} }, "invalid-auth-kind"},
		{"apikey without a name", specV3JSON, func(c *Connection) {
			c.Auth = Auth{Kind: AuthAPIKey, In: InHeader}
		}, "incomplete-auth"},
		{"apikey in an illegal place", specV3JSON, func(c *Connection) {
			c.Auth = Auth{Kind: AuthAPIKey, In: "body", Name: "X-Key"}
		}, "incomplete-auth"},
		{"basic without a user", specV3JSON, func(c *Connection) { c.Auth = Auth{Kind: AuthBasic} }, "incomplete-auth"},

		{"duplicate resource keys", specV3JSON, func(c *Connection) {
			c.Resources = append(c.Resources, c.Resources[0])
		}, "duplicate-resource-key"},
		{"empty resource key", specV3JSON, func(c *Connection) { c.Resources[0].Key = "" }, "invalid-resource-key"},
		{"resource key with spaces", specV3JSON, func(c *Connection) { c.Resources[0].Key = "my customers" }, "invalid-resource-key"},

		{"unknown list operation", specV3JSON, func(c *Connection) {
			c.Resources[0].List.Operation = "nope"
		}, "unknown-operation"},
		{"unknown get operation", specV3JSON, func(c *Connection) {
			c.Resources[0].Get.Operation = "nope"
		}, "unknown-operation"},
		{"missing list operation", specV3JSON, func(c *Connection) {
			c.Resources[0].List = OpRef{}
		}, "unknown-operation"},

		{"id pointer without a leading slash", specV3JSON, func(c *Connection) {
			c.Resources[0].IDPath = "id"
		}, "invalid-pointer"},
		{"label pointer with a bad escape", specV3JSON, func(c *Connection) {
			c.Resources[0].LabelPath = "/a/~5"
		}, "invalid-pointer"},
		{"items pointer malformed", specV3JSON, func(c *Connection) {
			c.Resources[0].ItemsPath = "data/items"
		}, "invalid-pointer"},

		{"search param is not in the spec", specV3JSON, func(c *Connection) {
			c.Resources[0].SearchParam = "nope"
		}, "unknown-param"},
		{"search param is not a query param", specV3JSON, func(c *Connection) {
			c.Resources[0].SearchParam = "tenant"
		}, "unknown-param"},
		{"fixed param is not in the spec", specV3JSON, func(c *Connection) {
			c.Resources[0].List.Params["bogus"] = "1"
		}, "unknown-param"},

		{"list leaves a required param unbound", specV3JSON, func(c *Connection) {
			c.Resources[0].List.Params = nil
		}, "unsatisfied-required-param"},
		{"get leaves a required header unbound", specV3JSON, func(c *Connection) {
			c.Resources[0].Get.Params = nil
		}, "unsatisfied-required-param"},

		{"get has no path param to carry the id", specV3JSON, func(c *Connection) {
			c.Resources[0].Get = OpRef{Operation: "listCustomers", Params: map[string]string{"tenant": "acme"}}
		}, "no-id-param"},
		{"get has two open path params", specTwoPathParams, func(c *Connection) {
			c.Resources[0] = Resource{
				Key:  "orgusers",
				List: OpRef{Operation: "listOrgs"},
				Get:  OpRef{Operation: "getOrgUser"},
			}
		}, "ambiguous-id-param"},

		{"unknown pagination style", specV3JSON, func(c *Connection) {
			c.Resources[0].Pagination = Pagination{Style: "weird"}
		}, "invalid-pagination"},
		{"offset paging without an offset param", specV3JSON, func(c *Connection) {
			c.Resources[0].Pagination = Pagination{Style: PageOffset, LimitParam: "limit"}
		}, "invalid-pagination"},
		{"offset paging naming a param the spec lacks", specV3JSON, func(c *Connection) {
			c.Resources[0].Pagination = Pagination{Style: PageOffset, LimitParam: "limit", OffsetParam: "offset"}
		}, "unknown-param"},
		{"cursor paging without a cursor path", specV3JSON, func(c *Connection) {
			c.Resources[0].Pagination = Pagination{Style: PageCursor, CursorParam: "q"}
		}, "invalid-pagination"},
		{"cursor paging with a malformed cursor path", specV3JSON, func(c *Connection) {
			c.Resources[0].Pagination = Pagination{Style: PageCursor, CursorParam: "q", CursorPath: "next"}
		}, "invalid-pointer"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cat := mustParse(t, tc.spec)
			c := validConn()
			tc.mutate(&c)
			errs := Validate(&c, cat)
			if !hasType(errs, tc.want) {
				t.Fatalf("want a %q error, got: %s", tc.want, types(errs))
			}
		})
	}
}

func TestValidate_NilArgs(t *testing.T) {
	cat := mustParse(t, specV3JSON)
	if errs := Validate(nil, cat); len(errs) == 0 {
		t.Error("a nil connection must not validate")
	}
	c := validConn()
	if errs := Validate(&c, nil); len(errs) == 0 {
		t.Error("a connection without a catalog must not validate")
	}
}

func TestValidate_ReportsTheOffendingResource(t *testing.T) {
	cat := mustParse(t, specV3JSON)
	c := validConn()
	c.Resources[0].List.Operation = "nope"
	errs := Validate(&c, cat)
	if len(errs) == 0 || errs[0].Resource != "customers" {
		t.Fatalf("errors must name their resource, got %+v", errs)
	}
}

// A spec may declare a relative server such as "/api/v3", which says where the
// service sits but not on which host. Accepting that as a fallback would let a
// connection validate clean and then fail every call.
func TestValidate_RelativeServerIsNotAUsableFallback(t *testing.T) {
	const relativeServer = `{
	  "openapi": "3.0.0",
	  "info": {"title": "Relative", "version": "1"},
	  "servers": [{"url": "/api/v3"}],
	  "paths": {"/things": {"get": {"operationId": "listThings"}}}
	}`
	cat, err := ParseSpec([]byte(relativeServer))
	if err != nil {
		t.Fatal(err)
	}
	if got := FirstAbsoluteServer(cat); got != "" {
		t.Fatalf("FirstAbsoluteServer = %q, want none", got)
	}

	c := Connection{
		ID: "rel", Name: "Relative", SpecFile: "rel.json",
		Resources: []Resource{{Key: "things", List: OpRef{Operation: "listThings"}}},
	}
	if errs := Validate(&c, cat); !hasType(errs, "no-server") {
		t.Fatalf("errors = %s; a relative server cannot be dialled", types(errs))
	}

	// With a base URL of its own it is fine.
	c.BaseURL = "https://example.com/api/v3"
	if errs := Validate(&c, cat); hasType(errs, "no-server") {
		t.Fatalf("errors = %s; an explicit base URL resolves it", types(errs))
	}
}

func TestFirstAbsoluteServer_SkipsRelativeAndNonHTTP(t *testing.T) {
	cat := &Catalog{Servers: []string{"/api/v3", "ftp://files.example.com", "https://api.example.com/v2/"}}
	if got := FirstAbsoluteServer(cat); got != "https://api.example.com/v2" {
		t.Fatalf("got %q", got)
	}
	if got := FirstAbsoluteServer(nil); got != "" {
		t.Fatalf("nil catalog = %q", got)
	}
}
