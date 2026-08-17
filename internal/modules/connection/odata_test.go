package connection

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

// specOData mirrors what the OASIS OData-to-OpenAPI mapping emits: system query
// options declared as real parameters, and entity access through a
// parenthesised key segment rather than a path segment.
const specOData = `{
  "openapi": "3.0.0",
  "info": {"title": "Northwind", "version": "4.0"},
  "paths": {
    "/Customers": {
      "get": {
        "operationId": "listCustomers",
        "parameters": [
          {"name": "$top", "in": "query", "schema": {"type": "integer"}},
          {"name": "$skip", "in": "query", "schema": {"type": "integer"}},
          {"name": "$filter", "in": "query", "schema": {"type": "string"}},
          {"name": "$select", "in": "query", "schema": {"type": "string"}}
        ]
      }
    },
    "/Customers({CustomerID})": {
      "get": {
        "operationId": "getCustomer",
        "parameters": [
          {"name": "CustomerID", "in": "path", "required": true, "schema": {"type": "string"}},
          {"name": "$select", "in": "query", "schema": {"type": "string"}}
        ]
      }
    },
    "/Orders({OrderID})": {
      "get": {
        "operationId": "getOrder",
        "parameters": [
          {"name": "OrderID", "in": "path", "required": true, "schema": {"type": "integer"}}
        ]
      }
    },
    "/Orders": {"get": {"operationId": "listOrders"}}
  }
}`

// specODataBare is the same service described by a document that does not
// enumerate the system query options, which is common for hand-written specs.
const specODataBare = `{
  "openapi": "3.0.0",
  "info": {"title": "Northwind", "version": "4.0"},
  "paths": {
    "/Customers": {"get": {"operationId": "listCustomers"}},
    "/Customers({CustomerID})": {
      "get": {
        "operationId": "getCustomer",
        "parameters": [
          {"name": "CustomerID", "in": "path", "required": true, "schema": {"type": "string"}}
        ]
      }
    }
  }
}`

// odataFixture stands up a service that answers like OData: a value envelope,
// an absolute @odata.nextLink, and a record shape with PascalCase properties.
func odataFixture(t *testing.T, spec string, mutate func(*Connection)) (*Invoker, *remote) {
	t.Helper()
	rm := &remote{status: 200, contentType: "application/json"}
	srv := httptest.NewServer(http.HandlerFunc(rm.handler))
	t.Cleanup(srv.Close)
	rm.base = srv.URL

	fs := newMemFS()
	mgr := NewManager(fs, nil)
	if _, err := mgr.ImportSpec("northwind", []byte(spec)); err != nil {
		t.Fatal(err)
	}

	conn := &Connection{
		ID:       "northwind",
		Name:     "Northwind",
		BaseURL:  srv.URL,
		SpecFile: "northwind.json",
		Dialect:  DialectOData,
		Auth:     Auth{Kind: AuthNone},
		Resources: []Resource{{
			Key:         "customers",
			List:        OpRef{Operation: "listCustomers"},
			Get:         OpRef{Operation: "getCustomer"},
			ItemsPath:   "/value",
			IDPath:      "/CustomerID",
			LabelPath:   "/CompanyName",
			SearchParam: "$filter",
			Pagination:  Pagination{Style: PageOffset, LimitParam: "$top", OffsetParam: "$skip"},
			Fields: []FieldMap{
				{Key: "city", Pointer: "/Address/City"},
				{Key: "phone", Pointer: "/Phone"},
			},
		}},
	}
	if mutate != nil {
		mutate(conn)
	}
	if err := mgr.Save(conn); err != nil {
		t.Fatalf("Save: %v", err)
	}
	return NewInvoker(mgr, staticSecret{value: "s3cret"}), rm
}

// Entity addressing -----------------------------------------------------

func TestOData_StringKeyIsQuoted(t *testing.T) {
	iv, rm := odataFixture(t, specOData, nil)
	rm.body = `{"CustomerID": "ALFKI", "CompanyName": "Alfreds"}`

	if _, err := iv.Fetch(context.Background(), FetchRequest{
		Connection: "northwind", Resource: "customers", ID: "ALFKI",
	}); err != nil {
		t.Fatal(err)
	}
	if rm.path != "/Customers('ALFKI')" {
		t.Fatalf("path = %q, want /Customers('ALFKI')", rm.path)
	}
}

func TestOData_QuoteInsideAKeyIsDoubled(t *testing.T) {
	iv, rm := odataFixture(t, specOData, nil)
	rm.body = `{"CustomerID": "O'Brien", "CompanyName": "OB"}`

	if _, err := iv.Fetch(context.Background(), FetchRequest{
		Connection: "northwind", Resource: "customers", ID: "O'Brien",
	}); err != nil {
		t.Fatal(err)
	}
	if rm.path != "/Customers('O''Brien')" {
		t.Fatalf("path = %q; an embedded quote must be doubled, not left to truncate the literal", rm.path)
	}
}

func TestOData_NumericKeyIsNotQuoted(t *testing.T) {
	iv, rm := odataFixture(t, specOData, func(c *Connection) {
		c.Resources = append(c.Resources, Resource{
			Key:       "orders",
			List:      OpRef{Operation: "listOrders"},
			Get:       OpRef{Operation: "getOrder"},
			ItemsPath: "/value",
			IDPath:    "/OrderID",
			LabelPath: "/OrderID",
		})
	})
	rm.body = `{"OrderID": 10248}`

	if _, err := iv.Fetch(context.Background(), FetchRequest{
		Connection: "northwind", Resource: "orders", ID: "10248",
	}); err != nil {
		t.Fatal(err)
	}
	if rm.path != "/Orders(10248)" {
		t.Fatalf("path = %q; an integer key must stay bare", rm.path)
	}
}

func TestKeyStyle_RawIsTheRESTDefault(t *testing.T) {
	iv, rec, _ := shopFixture(t, jsonHandler(200, `{"id": "ALFKI", "name": "Alfreds"}`))
	if _, err := iv.Fetch(context.Background(), FetchRequest{
		Connection: "shop", Resource: "customers", ID: "ALFKI",
	}); err != nil {
		t.Fatal(err)
	}
	if rec.path != "/customers/ALFKI" {
		t.Fatalf("path = %q; plain REST must not gain quotes", rec.path)
	}
}

func TestKeyStyle_ExplicitOverridesTheDialect(t *testing.T) {
	iv, rm := odataFixture(t, specOData, func(c *Connection) {
		c.Resources[0].KeyStyle = KeyRaw
	})
	rm.body = `{"CustomerID": "ALFKI", "CompanyName": "Alfreds"}`

	if _, err := iv.Fetch(context.Background(), FetchRequest{
		Connection: "northwind", Resource: "customers", ID: "ALFKI",
	}); err != nil {
		t.Fatal(err)
	}
	if rm.path != "/Customers(ALFKI)" {
		t.Fatalf("path = %q; an explicit key_style must win over the dialect", rm.path)
	}
}

// Paging by follow-this-URL ---------------------------------------------

func TestOData_NextLinkIsReportedAndFollowedVerbatim(t *testing.T) {
	iv, rm := odataFixture(t, specOData, func(c *Connection) {
		c.Resources[0].Pagination = Pagination{Style: PageLink, LimitParam: "$top"}
	})
	rm.bodyFn = func(r *http.Request) string {
		if r.URL.Query().Get("$skiptoken") == "abc" {
			return `{"value": [{"CustomerID": "BLAUS", "CompanyName": "Blauer"}]}`
		}
		return `{"value": [{"CustomerID": "ALFKI", "CompanyName": "Alfreds"}],
		         "@odata.nextLink": "` + rm.base + `/Customers?$skiptoken=abc"}`
	}

	first, err := iv.List(context.Background(), ListRequest{Connection: "northwind", Resource: "customers"})
	if err != nil {
		t.Fatal(err)
	}
	if first.NextCursor == "" {
		t.Fatal("@odata.nextLink was not surfaced")
	}
	if !strings.Contains(first.NextCursor, "$skiptoken=abc") {
		t.Fatalf("next cursor = %q, want the whole link", first.NextCursor)
	}

	second, err := iv.List(context.Background(), ListRequest{
		Connection: "northwind", Resource: "customers", Cursor: first.NextCursor,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(second.Items) != 1 || second.Items[0].ID != "BLAUS" {
		t.Fatalf("following the link did not reach page two: %+v", second.Items)
	}
	if rm.query.Get("$skiptoken") != "abc" {
		t.Errorf("the link was rebuilt instead of followed: %v", rm.query)
	}
}

func TestOData_RelativeNextLinkResolvesAgainstTheBase(t *testing.T) {
	iv, rm := odataFixture(t, specOData, func(c *Connection) {
		c.Resources[0].Pagination = Pagination{Style: PageLink}
	})
	rm.body = `{"value": [{"CustomerID": "BLAUS", "CompanyName": "Blauer"}]}`

	page, err := iv.List(context.Background(), ListRequest{
		Connection: "northwind", Resource: "customers", Cursor: "Customers?$skiptoken=xyz",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 1 {
		t.Fatalf("items = %+v", page.Items)
	}
	if rm.path != "/Customers" || rm.query.Get("$skiptoken") != "xyz" {
		t.Fatalf("relative link resolved to %q?%v", rm.path, rm.query)
	}
}

func TestLinkPaging_RefusesALinkToAnotherHost(t *testing.T) {
	iv, _ := odataFixture(t, specOData, func(c *Connection) {
		c.Resources[0].Pagination = Pagination{Style: PageLink}
	})
	_, err := iv.List(context.Background(), ListRequest{
		Connection: "northwind", Resource: "customers",
		Cursor: "https://evil.example.com/Customers",
	})
	if got := codeOf(t, err); got != CodeBindingInvalid {
		t.Fatalf("code = %q; a next link must not redirect credentials off-host", got)
	}
}

func TestLinkPaging_LastPageReportsNoCursor(t *testing.T) {
	iv, rm := odataFixture(t, specOData, func(c *Connection) {
		c.Resources[0].Pagination = Pagination{Style: PageLink}
	})
	rm.body = `{"value": [{"CustomerID": "ALFKI", "CompanyName": "Alfreds"}]}`

	page, err := iv.List(context.Background(), ListRequest{Connection: "northwind", Resource: "customers"})
	if err != nil {
		t.Fatal(err)
	}
	if page.NextCursor != "" {
		t.Fatalf("next cursor = %q, want empty on the last page", page.NextCursor)
	}
}

// Search as an expression ------------------------------------------------

func TestOData_SearchTemplateBecomesAFilterExpression(t *testing.T) {
	iv, rm := odataFixture(t, specOData, func(c *Connection) {
		c.Resources[0].SearchTemplate = "contains(CompanyName,'{q}')"
	})
	rm.body = `{"value": []}`

	if _, err := iv.List(context.Background(), ListRequest{
		Connection: "northwind", Resource: "customers", Search: "alfr",
	}); err != nil {
		t.Fatal(err)
	}
	if got := rm.query.Get("$filter"); got != "contains(CompanyName,'alfr')" {
		t.Fatalf("$filter = %q", got)
	}
}

func TestOData_SearchTextCannotRewriteTheFilter(t *testing.T) {
	iv, rm := odataFixture(t, specOData, func(c *Connection) {
		c.Resources[0].SearchTemplate = "contains(CompanyName,'{q}')"
	})
	rm.body = `{"value": []}`

	if _, err := iv.List(context.Background(), ListRequest{
		Connection: "northwind", Resource: "customers", Search: "x') or (1 eq 1",
	}); err != nil {
		t.Fatal(err)
	}
	got := rm.query.Get("$filter")
	if got != "contains(CompanyName,'x'') or (1 eq 1')" {
		t.Fatalf("$filter = %q; quotes must be doubled, not stripped", got)
	}
	// An odd number of quotes means the injected text closed the literal and
	// the tail escaped into expression position.
	if strings.Count(got, "'")%2 != 0 {
		t.Fatalf("$filter = %q leaves an unbalanced string literal", got)
	}
}

func TestSearchTemplate_AbsentMeansTheValueGoesThroughBare(t *testing.T) {
	iv, rm := odataFixture(t, specOData, nil)
	rm.body = `{"value": []}`

	if _, err := iv.List(context.Background(), ListRequest{
		Connection: "northwind", Resource: "customers", Search: "alfr",
	}); err != nil {
		t.Fatal(err)
	}
	if got := rm.query.Get("$filter"); got != "alfr" {
		t.Fatalf("$filter = %q, want the raw term when no template is set", got)
	}
}

// Server-side projection -------------------------------------------------

func TestOData_SelectParamPushesTheProjectionToTheServer(t *testing.T) {
	iv, rm := odataFixture(t, specOData, func(c *Connection) {
		c.Resources[0].SelectParam = "$select"
	})
	rm.body = `{"value": [{"CustomerID": "ALFKI", "CompanyName": "Alfreds", "Phone": "030-1234"}]}`

	page, err := iv.List(context.Background(), ListRequest{
		Connection: "northwind", Resource: "customers", Select: []string{"phone"},
	})
	if err != nil {
		t.Fatal(err)
	}
	got := strings.Split(rm.query.Get("$select"), ",")
	assertSameSet(t, got, []string{"CustomerID", "CompanyName", "Phone"})
	if page.Items[0].Fields["phone"] != "030-1234" {
		t.Errorf("fields = %+v", page.Items[0].Fields)
	}
}

func TestOData_SelectIncludesTheIDAndLabelOrTheyWouldBeStripped(t *testing.T) {
	iv, rm := odataFixture(t, specOData, func(c *Connection) {
		c.Resources[0].SelectParam = "$select"
	})
	rm.body = `{"value": []}`

	if _, err := iv.List(context.Background(), ListRequest{
		Connection: "northwind", Resource: "customers", Select: []string{"city"},
	}); err != nil {
		t.Fatal(err)
	}
	got := rm.query.Get("$select")
	for _, want := range []string{"CustomerID", "CompanyName"} {
		if !strings.Contains(got, want) {
			t.Errorf("$select = %q, missing %q", got, want)
		}
	}
}

func TestOData_NestedPointerBecomesAPropertyPath(t *testing.T) {
	iv, rm := odataFixture(t, specOData, func(c *Connection) {
		c.Resources[0].SelectParam = "$select"
	})
	rm.body = `{"value": []}`

	if _, err := iv.List(context.Background(), ListRequest{
		Connection: "northwind", Resource: "customers", Select: []string{"city"},
	}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(rm.query.Get("$select"), "Address/City") {
		t.Fatalf("$select = %q, want the nested property path", rm.query.Get("$select"))
	}
}

func TestSelect_ExplicitRemoteNameWins(t *testing.T) {
	iv, rm := odataFixture(t, specOData, func(c *Connection) {
		c.Resources[0].SelectParam = "$select"
		c.Resources[0].Fields[1].Remote = "Telephone"
	})
	rm.body = `{"value": []}`

	if _, err := iv.List(context.Background(), ListRequest{
		Connection: "northwind", Resource: "customers", Select: []string{"phone"},
	}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(rm.query.Get("$select"), "Telephone") {
		t.Fatalf("$select = %q, want the explicit remote name", rm.query.Get("$select"))
	}
}

func TestSelect_IsDeterministic(t *testing.T) {
	iv, rm := odataFixture(t, specOData, func(c *Connection) {
		c.Resources[0].SelectParam = "$select"
	})
	rm.body = `{"value": []}`

	var first string
	for i := range 10 {
		if _, err := iv.List(context.Background(), ListRequest{
			Connection: "northwind", Resource: "customers", Select: []string{"phone", "city"},
		}); err != nil {
			t.Fatal(err)
		}
		got := rm.query.Get("$select")
		if i == 0 {
			first = got
			continue
		}
		if got != first {
			t.Fatalf("$select drifted: %q vs %q", got, first)
		}
	}
}

func TestSelect_OmittedWhenAPointerCoversTheWholeRecord(t *testing.T) {
	iv, rm := odataFixture(t, specOData, func(c *Connection) {
		c.Resources[0].SelectParam = "$select"
		c.Resources[0].LabelPath = ""
	})
	rm.body = `{"value": []}`

	if _, err := iv.List(context.Background(), ListRequest{Connection: "northwind", Resource: "customers"}); err != nil {
		t.Fatal(err)
	}
	if _, present := rm.query["$select"]; present {
		t.Fatal("a whole-record pointer needs every property, so no projection may be sent")
	}
}

func TestSelect_NotSentWhenTheResourceDeclaresNoSelectParam(t *testing.T) {
	iv, rm := odataFixture(t, specOData, nil)
	rm.body = `{"value": []}`

	if _, err := iv.List(context.Background(), ListRequest{
		Connection: "northwind", Resource: "customers", Select: []string{"phone"},
	}); err != nil {
		t.Fatal(err)
	}
	if _, present := rm.query["$select"]; present {
		t.Fatal("projection must stay local unless the resource opts into a select param")
	}
}

// Validation against a document that omits the system options -------------

func TestOData_ImpliedParamsBindAgainstABareDocument(t *testing.T) {
	fs := newMemFS()
	mgr := NewManager(fs, nil)
	if _, err := mgr.ImportSpec("northwind", []byte(specODataBare)); err != nil {
		t.Fatal(err)
	}
	conn := &Connection{
		ID: "northwind", Name: "Northwind", BaseURL: "https://nw.example.com",
		SpecFile: "northwind.json", Dialect: DialectOData,
		Resources: []Resource{{
			Key:         "customers",
			List:        OpRef{Operation: "listCustomers"},
			Get:         OpRef{Operation: "getCustomer"},
			ItemsPath:   "/value",
			IDPath:      "/CustomerID",
			LabelPath:   "/CompanyName",
			SearchParam: "$filter",
			SelectParam: "$select",
			Pagination:  Pagination{Style: PageOffset, LimitParam: "$top", OffsetParam: "$skip"},
		}},
	}
	if err := mgr.Save(conn); err != nil {
		t.Fatalf("OData system query options are protocol-defined, not typos: %v", err)
	}
}

func TestREST_UndeclaredParamIsStillATypo(t *testing.T) {
	fs := newMemFS()
	mgr := NewManager(fs, nil)
	if _, err := mgr.ImportSpec("northwind", []byte(specODataBare)); err != nil {
		t.Fatal(err)
	}
	conn := &Connection{
		ID: "northwind", Name: "Northwind", BaseURL: "https://nw.example.com",
		SpecFile: "northwind.json",
		Resources: []Resource{{
			Key: "customers", List: OpRef{Operation: "listCustomers"},
			IDPath: "/CustomerID", SearchParam: "$filter",
		}},
	}
	err := mgr.Save(conn)
	var vfe *ValidationFailedError
	if err == nil || !asValidation(err, &vfe) || !hasType(vfe.Errors, "unknown-param") {
		t.Fatalf("without a dialect an undeclared param must still be refused, got %v", err)
	}
}

func TestValidate_UnknownDialectIsRefused(t *testing.T) {
	cat := mustParse(t, specV3JSON)
	c := validConn()
	c.Dialect = "graphql"
	if errs := Validate(&c, cat); !hasType(errs, "invalid-dialect") {
		t.Fatalf("errors = %s", types(errs))
	}
}

func TestValidate_KeyStyleAndTemplateRules(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*Connection)
		want   string
	}{
		{"unknown key style", func(c *Connection) { c.Resources[0].KeyStyle = "sideways" }, "invalid-key-style"},
		{"search template without a param", func(c *Connection) {
			c.Resources[0].SearchParam = ""
			c.Resources[0].SearchTemplate = "contains(name,'{q}')"
		}, "invalid-search-template"},
		{"search template with no placeholder", func(c *Connection) {
			c.Resources[0].SearchTemplate = "contains(name,'x')"
		}, "invalid-search-template"},
		{"select param the spec lacks", func(c *Connection) {
			c.Resources[0].SelectParam = "$select"
		}, "unknown-param"},
		{"link paging with a malformed link path", func(c *Connection) {
			c.Resources[0].Pagination = Pagination{Style: PageLink, LinkPath: "next"}
		}, "invalid-pointer"},
		{"link paging with no link path and no dialect default", func(c *Connection) {
			c.Resources[0].Pagination = Pagination{Style: PageLink}
		}, "invalid-pagination"},
		{"duplicate field keys", func(c *Connection) {
			c.Resources[0].Fields = []FieldMap{
				{Key: "email", Pointer: "/email"},
				{Key: "email", Pointer: "/other"},
			}
		}, "duplicate-field-key"},
		{"field pointer malformed", func(c *Connection) {
			c.Resources[0].Fields = []FieldMap{{Key: "email", Pointer: "email"}}
		}, "invalid-pointer"},
	}
	cat := mustParse(t, specV3JSON)
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := validConn()
			tc.mutate(&c)
			if errs := Validate(&c, cat); !hasType(errs, tc.want) {
				t.Fatalf("want %q, got: %s", tc.want, types(errs))
			}
		})
	}
}

func TestValidate_LinkPagingUsesTheDialectDefault(t *testing.T) {
	cat := mustParse(t, specV3JSON)
	c := validConn()
	c.Dialect = DialectOData
	c.Resources[0].Pagination = Pagination{Style: PageLink}
	if errs := Validate(&c, cat); hasType(errs, "invalid-pagination") {
		t.Fatalf("the odata dialect supplies the link path: %s", types(errs))
	}
}

// Round-tripping a quoted key -------------------------------------------

func TestUnformatKey_RoundTripsQuotedLiterals(t *testing.T) {
	cases := map[string]string{
		"ALFKI":      "ALFKI",
		"'ALFKI'":    "ALFKI",
		"'O''Brien'": "O'Brien",
		"10248":      "10248",
		"''":         "",
		"'it''s ok'": "it's ok",
		"unclosed'":  "unclosed'",
	}
	for in, want := range cases {
		if got := unformatKey(in); got != want {
			t.Errorf("unformatKey(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestPointerToProperty(t *testing.T) {
	cases := map[string]string{
		"/Phone":        "Phone",
		"/Address/City": "Address/City",
		"":              "",
		"/a~1b":         "a/b",
		"/a~0b":         "a~b",
	}
	for in, want := range cases {
		if got := pointerToProperty(in); got != want {
			t.Errorf("pointerToProperty(%q) = %q, want %q", in, got, want)
		}
	}
}

// assertSameSet compares two string slices ignoring order.
func assertSameSet(t *testing.T, got, want []string) {
	t.Helper()
	index := map[string]int{}
	for _, v := range got {
		index[v]++
	}
	for _, v := range want {
		if index[v] == 0 {
			t.Errorf("missing %q in %v", v, got)
		}
		index[v]--
	}
	for v, n := range index {
		if n > 0 {
			t.Errorf("unexpected %q in %v", v, got)
		}
	}
}

// asValidation is errors.As for the validation envelope, kept local so the
// table above reads without an import shuffle.
func asValidation(err error, target **ValidationFailedError) bool {
	v, ok := err.(*ValidationFailedError)
	if ok {
		*target = v
	}
	return ok
}

var _ = json.Marshal
var _ = url.Values{}
