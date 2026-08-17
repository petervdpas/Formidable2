package connection

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

const specShop = `{
  "openapi": "3.0.0",
  "info": {"title": "Shop", "version": "1"},
  "paths": {
    "/customers": {
      "get": {
        "operationId": "listCustomers",
        "parameters": [
          {"name": "q", "in": "query", "schema": {"type": "string"}},
          {"name": "limit", "in": "query", "schema": {"type": "integer"}},
          {"name": "offset", "in": "query", "schema": {"type": "integer"}},
          {"name": "cursor", "in": "query", "schema": {"type": "string"}},
          {"name": "tenant", "in": "header", "required": true, "schema": {"type": "string"}}
        ]
      }
    },
    "/customers/{customerId}": {
      "get": {
        "operationId": "getCustomer",
        "parameters": [
          {"name": "customerId", "in": "path", "required": true, "schema": {"type": "string"}},
          {"name": "tenant", "in": "header", "required": true, "schema": {"type": "string"}}
        ]
      }
    }
  }
}`

const listBody = `{
  "data": [
    {"id": 7, "name": "Ada", "email": "ada@example.com", "address": {"city": "Delft"}, "vip": true},
    {"id": 8, "name": "Grace", "email": "grace@example.com", "address": {"city": "Utrecht"}, "vip": false}
  ],
  "paging": {"next": "cur-2"}
}`

// staticSecret is the stand-in for the key vault the connection module will
// eventually read from.
type staticSecret struct {
	value string
	err   error
}

func (s staticSecret) Secret(string) (string, error) { return s.value, s.err }

// recorder captures what the interpreter actually put on the wire.
type recorder struct {
	path   string
	rawURL string
	query  url.Values
	header http.Header
}

// shopFixture wires a Manager and an Invoker at a test server, and returns the
// recorder for the last request the server saw.
func shopFixture(t *testing.T, handler http.HandlerFunc, opts ...InvokerOption) (*Invoker, *recorder, func(*Connection)) {
	t.Helper()
	rec := &recorder{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec.path = r.URL.Path
		rec.rawURL = r.URL.String()
		rec.query = r.URL.Query()
		rec.header = r.Header.Clone()
		handler(w, r)
	}))
	t.Cleanup(srv.Close)

	fs := newMemFS()
	mgr := NewManager(fs, nil)
	if _, err := mgr.ImportSpec("shop", []byte(specShop)); err != nil {
		t.Fatal(err)
	}

	conn := &Connection{
		ID:       "shop",
		Name:     "Shop",
		BaseURL:  srv.URL,
		SpecFile: "shop.json",
		Auth:     Auth{Kind: AuthNone},
		Resources: []Resource{{
			Key:         "customers",
			List:        OpRef{Operation: "listCustomers", Params: map[string]string{"tenant": "acme"}},
			Get:         OpRef{Operation: "getCustomer", Params: map[string]string{"tenant": "acme"}},
			ItemsPath:   "/data",
			IDPath:      "/id",
			LabelPath:   "/name",
			SearchParam: "q",
			Pagination:  Pagination{Style: PageOffset, LimitParam: "limit", OffsetParam: "offset"},
			Fields: []FieldMap{
				{Key: "email", Pointer: "/email"},
				{Key: "city", Pointer: "/address/city"},
				{Key: "vip", Pointer: "/vip"},
			},
		}},
	}

	// reconfigure lets a test bend the connection before it is written.
	reconfigure := func(mutate *Connection) {
		if err := mgr.Save(mutate); err != nil {
			t.Fatalf("Save: %v", err)
		}
	}
	reconfigure(conn)

	iv := NewInvoker(mgr, staticSecret{value: "s3cret"}, opts...)
	return iv, rec, reconfigure
}

func jsonHandler(status int, body string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}
}

func listReq() ListRequest {
	return ListRequest{Connection: "shop", Resource: "customers"}
}

func mustList(t *testing.T, iv *Invoker, req ListRequest) *Page {
	t.Helper()
	page, err := iv.List(context.Background(), req)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	return page
}

func codeOf(t *testing.T, err error) InvokeErrorCode {
	t.Helper()
	if err == nil {
		t.Fatal("want an error")
	}
	var ie *InvokeError
	if !errors.As(err, &ie) {
		t.Fatalf("want an *InvokeError, got %T: %v", err, err)
	}
	return ie.Code
}

// Reading the payload ---------------------------------------------------

func TestList_ReturnsIDAndLabel(t *testing.T) {
	iv, _, _ := shopFixture(t, jsonHandler(200, listBody))
	page := mustList(t, iv, listReq())

	if len(page.Items) != 2 {
		t.Fatalf("items = %+v", page.Items)
	}
	if page.Items[0].ID != "7" || page.Items[0].Label != "Ada" {
		t.Errorf("item 0 = %+v", page.Items[0])
	}
	if page.Items[1].ID != "8" || page.Items[1].Label != "Grace" {
		t.Errorf("item 1 = %+v", page.Items[1])
	}
}

func TestList_LargeIntegerIDKeepsItsDigits(t *testing.T) {
	body := `{"data": [{"id": 90071992547409911, "name": "Big"}]}`
	iv, _, _ := shopFixture(t, jsonHandler(200, body))
	page := mustList(t, iv, listReq())
	if page.Items[0].ID != "90071992547409911" {
		t.Fatalf("id = %q; a float round-trip corrupted it", page.Items[0].ID)
	}
}

func TestList_SelectProjectsOnlyTheRequestedFields(t *testing.T) {
	iv, _, _ := shopFixture(t, jsonHandler(200, listBody))
	req := listReq()
	req.Select = []string{"email"}
	page := mustList(t, iv, req)

	got := page.Items[0].Fields
	if len(got) != 1 || got["email"] != "ada@example.com" {
		t.Fatalf("fields = %+v, want only email", got)
	}
}

func TestList_EmptySelectReturnsEveryDeclaredField(t *testing.T) {
	iv, _, _ := shopFixture(t, jsonHandler(200, listBody))
	page := mustList(t, iv, listReq())

	got := page.Items[0].Fields
	want := map[string]string{"email": "ada@example.com", "city": "Delft", "vip": "true"}
	if len(got) != len(want) {
		t.Fatalf("fields = %+v, want %+v", got, want)
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("field %q = %q, want %q", k, got[k], v)
		}
	}
}

func TestList_SelectingAnUndeclaredFieldIsRefused(t *testing.T) {
	iv, _, _ := shopFixture(t, jsonHandler(200, listBody))
	req := listReq()
	req.Select = []string{"email", "salary"}
	_, err := iv.List(context.Background(), req)
	if got := codeOf(t, err); got != CodeUnknownField {
		t.Fatalf("code = %q, want %q", got, CodeUnknownField)
	}
}

func TestList_AbsentFieldIsOmittedRatherThanBlank(t *testing.T) {
	body := `{"data": [{"id": 1, "name": "NoEmail", "address": {"city": "Delft"}}]}`
	iv, _, _ := shopFixture(t, jsonHandler(200, body))
	page := mustList(t, iv, listReq())

	if _, present := page.Items[0].Fields["email"]; present {
		t.Error("a pointer that does not resolve must not produce an empty string")
	}
	if page.Items[0].Fields["city"] != "Delft" {
		t.Error("sibling fields must still resolve")
	}
}

func TestList_ItemWithoutAnIDIsSkipped(t *testing.T) {
	body := `{"data": [{"name": "Anonymous"}, {"id": 8, "name": "Grace"}]}`
	iv, _, _ := shopFixture(t, jsonHandler(200, body))
	page := mustList(t, iv, listReq())

	if len(page.Items) != 1 || page.Items[0].ID != "8" {
		t.Fatalf("an item with no id cannot be referenced and must be dropped: %+v", page.Items)
	}
}

func TestList_LabelFallsBackToTheID(t *testing.T) {
	body := `{"data": [{"id": 7}]}`
	iv, _, _ := shopFixture(t, jsonHandler(200, body))
	page := mustList(t, iv, listReq())
	if page.Items[0].Label != "7" {
		t.Fatalf("label = %q, want the id as a fallback", page.Items[0].Label)
	}
}

func TestList_BareArrayResponse(t *testing.T) {
	iv, _, reconfigure := shopFixture(t, jsonHandler(200, `[{"id": 1, "name": "Solo"}]`))
	c := shopConn(t, iv)
	c.Resources[0].ItemsPath = ""
	reconfigure(c)

	page := mustList(t, iv, listReq())
	if len(page.Items) != 1 || page.Items[0].Label != "Solo" {
		t.Fatalf("an empty items_path means the body is the array: %+v", page.Items)
	}
}

func TestList_ItemsPathPointingAtANonArray(t *testing.T) {
	iv, _, _ := shopFixture(t, jsonHandler(200, `{"data": {"id": 1}}`))
	_, err := iv.List(context.Background(), listReq())
	if got := codeOf(t, err); got != CodeShapeMismatch {
		t.Fatalf("code = %q, want %q", got, CodeShapeMismatch)
	}
}

func TestList_EmptyResultIsNotAnError(t *testing.T) {
	iv, _, _ := shopFixture(t, jsonHandler(200, `{"data": []}`))
	page := mustList(t, iv, listReq())
	if len(page.Items) != 0 {
		t.Fatalf("items = %+v", page.Items)
	}
}

// Building the request --------------------------------------------------

func TestList_SendsSearchAndPagingAndFixedParams(t *testing.T) {
	iv, rec, _ := shopFixture(t, jsonHandler(200, listBody))
	req := listReq()
	req.Search = "ada & co"
	req.Limit = 25
	req.Offset = 50
	mustList(t, iv, req)

	if got := rec.query.Get("q"); got != "ada & co" {
		t.Errorf("q = %q", got)
	}
	if rec.query.Get("limit") != "25" || rec.query.Get("offset") != "50" {
		t.Errorf("paging = %v", rec.query)
	}
	if got := rec.header.Get("tenant"); got != "acme" {
		t.Errorf("fixed header param not sent: %q", got)
	}
	if rec.path != "/customers" {
		t.Errorf("path = %q", rec.path)
	}
}

func TestList_OmitsPagingWhenTheResourceDoesNotPage(t *testing.T) {
	iv, rec, reconfigure := shopFixture(t, jsonHandler(200, listBody))
	c := shopConn(t, iv)
	c.Resources[0].Pagination = Pagination{}
	reconfigure(c)

	req := listReq()
	req.Limit = 25
	mustList(t, iv, req)

	if _, present := rec.query["limit"]; present {
		t.Error("a resource without paging must not invent paging params")
	}
}

func TestList_CursorPaging(t *testing.T) {
	iv, rec, reconfigure := shopFixture(t, jsonHandler(200, listBody))
	c := shopConn(t, iv)
	c.Resources[0].Pagination = Pagination{
		Style: PageCursor, CursorParam: "cursor", CursorPath: "/paging/next", LimitParam: "limit",
	}
	reconfigure(c)

	req := listReq()
	req.Cursor = "cur-1"
	page := mustList(t, iv, req)

	if rec.query.Get("cursor") != "cur-1" {
		t.Errorf("cursor not sent: %v", rec.query)
	}
	if page.NextCursor != "cur-2" {
		t.Errorf("next cursor = %q, want cur-2", page.NextCursor)
	}
}

func TestList_SearchIsIgnoredWhenTheResourceHasNoSearchParam(t *testing.T) {
	iv, rec, reconfigure := shopFixture(t, jsonHandler(200, listBody))
	c := shopConn(t, iv)
	c.Resources[0].SearchParam = ""
	reconfigure(c)

	req := listReq()
	req.Search = "ada"
	mustList(t, iv, req)

	if len(rec.query["q"]) != 0 {
		t.Errorf("search leaked into the query: %v", rec.query)
	}
}

func TestFetch_SubstitutesAndEscapesThePathParam(t *testing.T) {
	iv, rec, _ := shopFixture(t, jsonHandler(200, `{"id": "a/b c", "name": "Odd"}`))
	item, err := iv.Fetch(context.Background(), FetchRequest{
		Connection: "shop", Resource: "customers", ID: "a/b c",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(rec.rawURL, "a%2Fb%20c") {
		t.Errorf("id was not escaped into the path: %q", rec.rawURL)
	}
	if rec.path != "/customers/a/b c" {
		t.Errorf("decoded path = %q", rec.path)
	}
	if item.Label != "Odd" {
		t.Errorf("item = %+v", item)
	}
}

func TestFetch_ProjectsSelectedFields(t *testing.T) {
	iv, _, _ := shopFixture(t, jsonHandler(200,
		`{"id": 7, "name": "Ada", "email": "ada@example.com", "address": {"city": "Delft"}}`))
	item, err := iv.Fetch(context.Background(), FetchRequest{
		Connection: "shop", Resource: "customers", ID: "7", Select: []string{"city"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(item.Fields) != 1 || item.Fields["city"] != "Delft" {
		t.Fatalf("fields = %+v", item.Fields)
	}
}

func TestFetch_RequiresAGetBinding(t *testing.T) {
	iv, _, reconfigure := shopFixture(t, jsonHandler(200, `{}`))
	c := shopConn(t, iv)
	c.Resources[0].Get = OpRef{}
	reconfigure(c)

	_, err := iv.Fetch(context.Background(), FetchRequest{Connection: "shop", Resource: "customers", ID: "7"})
	if got := codeOf(t, err); got != CodeBindingInvalid {
		t.Fatalf("code = %q, want %q", got, CodeBindingInvalid)
	}
}

func TestFetch_EmptyIDIsRefusedBeforeAnyCall(t *testing.T) {
	iv, rec, _ := shopFixture(t, jsonHandler(200, `{}`))
	_, err := iv.Fetch(context.Background(), FetchRequest{Connection: "shop", Resource: "customers", ID: ""})
	if got := codeOf(t, err); got != CodeBindingInvalid {
		t.Fatalf("code = %q, want %q", got, CodeBindingInvalid)
	}
	if rec.path != "" {
		t.Error("no request should have gone out")
	}
}

func TestList_BaseURLPathPrefixIsKept(t *testing.T) {
	iv, rec, reconfigure := shopFixture(t, jsonHandler(200, listBody))
	c := shopConn(t, iv)
	c.BaseURL = strings.TrimSuffix(c.BaseURL, "/") + "/v2"
	reconfigure(c)

	mustList(t, iv, listReq())
	if rec.path != "/v2/customers" {
		t.Fatalf("path = %q, want the base prefix kept", rec.path)
	}
}

// Authentication --------------------------------------------------------

func TestAuth_Modes(t *testing.T) {
	cases := []struct {
		name  string
		auth  Auth
		check func(*testing.T, *recorder)
	}{
		{"none", Auth{Kind: AuthNone}, func(t *testing.T, r *recorder) {
			if r.header.Get("Authorization") != "" {
				t.Errorf("unexpected Authorization: %q", r.header.Get("Authorization"))
			}
		}},
		{"bearer", Auth{Kind: AuthBearer}, func(t *testing.T, r *recorder) {
			if got := r.header.Get("Authorization"); got != "Bearer s3cret" {
				t.Errorf("Authorization = %q", got)
			}
		}},
		{"apikey header", Auth{Kind: AuthAPIKey, In: InHeader, Name: "X-Api-Key"}, func(t *testing.T, r *recorder) {
			if got := r.header.Get("X-Api-Key"); got != "s3cret" {
				t.Errorf("X-Api-Key = %q", got)
			}
		}},
		{"apikey query", Auth{Kind: AuthAPIKey, In: InQuery, Name: "api_key"}, func(t *testing.T, r *recorder) {
			if got := r.query.Get("api_key"); got != "s3cret" {
				t.Errorf("api_key = %q", got)
			}
		}},
		{"basic", Auth{Kind: AuthBasic, User: "ada"}, func(t *testing.T, r *recorder) {
			if !strings.HasPrefix(r.header.Get("Authorization"), "Basic ") {
				t.Errorf("Authorization = %q", r.header.Get("Authorization"))
			}
		}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			iv, rec, reconfigure := shopFixture(t, jsonHandler(200, listBody))
			c := shopConn(t, iv)
			c.Auth = tc.auth
			reconfigure(c)
			mustList(t, iv, listReq())
			tc.check(t, rec)
		})
	}
}

func TestAuth_MissingSecretIsReportedAsUnconfigured(t *testing.T) {
	iv, _, reconfigure := shopFixture(t, jsonHandler(200, listBody))
	c := shopConn(t, iv)
	c.Auth = Auth{Kind: AuthBearer}
	reconfigure(c)
	iv.secrets = staticSecret{err: errors.New("no such keychain entry")}

	_, err := iv.List(context.Background(), listReq())
	if got := codeOf(t, err); got != CodeNotConfigured {
		t.Fatalf("code = %q, want %q", got, CodeNotConfigured)
	}
}

func TestAuth_NoResolverWithAuthConfigured(t *testing.T) {
	iv, _, reconfigure := shopFixture(t, jsonHandler(200, listBody))
	c := shopConn(t, iv)
	c.Auth = Auth{Kind: AuthBearer}
	reconfigure(c)
	iv.secrets = nil

	_, err := iv.List(context.Background(), listReq())
	if got := codeOf(t, err); got != CodeNotConfigured {
		t.Fatalf("code = %q, want %q", got, CodeNotConfigured)
	}
}

func TestKeychainAccount_IsStableAndAppScoped(t *testing.T) {
	if got := KeychainAccount("shop"); got != "app:connection:shop" {
		t.Fatalf("account = %q", got)
	}
}

// Failure modes ---------------------------------------------------------

func TestInvoke_HTTPStatusMapping(t *testing.T) {
	cases := []struct {
		status int
		want   InvokeErrorCode
	}{
		{400, CodeRemoteError},
		{401, CodeUnauthorized},
		{403, CodeForbidden},
		{404, CodeRemoteNotFound},
		{429, CodeRateLimited},
		{500, CodeRemoteError},
		{503, CodeRemoteError},
	}
	for _, tc := range cases {
		t.Run(fmt.Sprint(tc.status), func(t *testing.T) {
			iv, _, _ := shopFixture(t, jsonHandler(tc.status, `{"error": "nope"}`))
			_, err := iv.List(context.Background(), listReq())
			if got := codeOf(t, err); got != tc.want {
				t.Fatalf("status %d gave %q, want %q", tc.status, got, tc.want)
			}
		})
	}
}

func TestInvoke_StatusErrorCarriesTheStatus(t *testing.T) {
	iv, _, _ := shopFixture(t, jsonHandler(503, `service down`))
	_, err := iv.List(context.Background(), listReq())
	var ie *InvokeError
	if !errors.As(err, &ie) || ie.Status != 503 {
		t.Fatalf("error = %+v, want Status 503", ie)
	}
}

func TestInvoke_MalformedJSON(t *testing.T) {
	iv, _, _ := shopFixture(t, jsonHandler(200, `{"data": [`))
	_, err := iv.List(context.Background(), listReq())
	if got := codeOf(t, err); got != CodeBadResponse {
		t.Fatalf("code = %q, want %q", got, CodeBadResponse)
	}
}

func TestInvoke_NonJSONContentType(t *testing.T) {
	iv, _, _ := shopFixture(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte("<html>login please</html>"))
	})
	_, err := iv.List(context.Background(), listReq())
	if got := codeOf(t, err); got != CodeBadResponse {
		t.Fatalf("code = %q, want %q", got, CodeBadResponse)
	}
}

func TestInvoke_ResponseSizeIsCapped(t *testing.T) {
	big := `{"data": [{"id": 1, "name": "` + strings.Repeat("x", 4096) + `"}]}`
	iv, _, _ := shopFixture(t, jsonHandler(200, big), WithMaxResponseBytes(512))
	_, err := iv.List(context.Background(), listReq())
	if got := codeOf(t, err); got != CodeTooLarge {
		t.Fatalf("code = %q, want %q", got, CodeTooLarge)
	}
}

func TestInvoke_Timeout(t *testing.T) {
	iv, _, _ := shopFixture(t, func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(300 * time.Millisecond)
		_, _ = w.Write([]byte(listBody))
	}, WithTimeout(40*time.Millisecond))

	_, err := iv.List(context.Background(), listReq())
	if got := codeOf(t, err); got != CodeTimeout {
		t.Fatalf("code = %q, want %q", got, CodeTimeout)
	}
}

func TestInvoke_ContextCancellation(t *testing.T) {
	iv, _, _ := shopFixture(t, func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(300 * time.Millisecond)
		_, _ = w.Write([]byte(listBody))
	})
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(20 * time.Millisecond)
		cancel()
	}()
	if _, err := iv.List(ctx, listReq()); err == nil {
		t.Fatal("want an error when the caller cancels")
	}
}

func TestInvoke_Unreachable(t *testing.T) {
	iv, _, reconfigure := shopFixture(t, jsonHandler(200, listBody))
	c := shopConn(t, iv)
	c.BaseURL = "http://127.0.0.1:1"
	reconfigure(c)

	_, err := iv.List(context.Background(), listReq())
	if got := codeOf(t, err); got != CodeUnreachable {
		t.Fatalf("code = %q, want %q", got, CodeUnreachable)
	}
}

func TestInvoke_CrossHostRedirectIsRefused(t *testing.T) {
	elsewhere := httptest.NewServer(jsonHandler(200, listBody))
	t.Cleanup(elsewhere.Close)

	iv, _, _ := shopFixture(t, func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, elsewhere.URL+"/customers", http.StatusFound)
	})
	_, err := iv.List(context.Background(), listReq())
	if err == nil {
		t.Fatal("a redirect to another host must not carry the credentials along")
	}
}

func TestInvoke_UnknownConnectionAndResource(t *testing.T) {
	iv, _, _ := shopFixture(t, jsonHandler(200, listBody))

	_, err := iv.List(context.Background(), ListRequest{Connection: "nope", Resource: "customers"})
	if got := codeOf(t, err); got != CodeConnectionNotFound {
		t.Errorf("code = %q, want %q", got, CodeConnectionNotFound)
	}

	_, err = iv.List(context.Background(), ListRequest{Connection: "shop", Resource: "nope"})
	if got := codeOf(t, err); got != CodeResourceNotFound {
		t.Errorf("code = %q, want %q", got, CodeResourceNotFound)
	}
}

func TestInvokeError_SerializesAsJSON(t *testing.T) {
	e := &InvokeError{Code: CodeUnauthorized, Message: "denied", Status: 401}
	s := e.Error()
	if !strings.Contains(s, `"code":"unauthorized"`) || !strings.Contains(s, `"status":401`) {
		t.Fatalf("Error() = %s", s)
	}
}

// shopConn reloads the fixture connection so a test can mutate and re-save it.
func shopConn(t *testing.T, iv *Invoker) *Connection {
	t.Helper()
	c, _, err := iv.mgr.Get("shop")
	if err != nil {
		t.Fatal(err)
	}
	return c
}
