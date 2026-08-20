package connection

import (
	"context"
	"net/http"
	"strings"
	"testing"
)

func tryReq(op string) TryRequest {
	return TryRequest{Connection: "shop", Operation: op}
}

func mustTry(t *testing.T, iv *Invoker, req TryRequest) *TryResult {
	t.Helper()
	res, err := iv.Try(context.Background(), req)
	if err != nil {
		t.Fatalf("Try: %v", err)
	}
	return res
}

func TestTry_RunsAnOperationNoResourceBinds(t *testing.T) {
	// The whole point: listCustomers is bound, createCustomer is not, and the
	// console has to reach an operation nothing binds.
	iv, rec, _ := shopFixture(t, jsonHandler(200, listBody))

	res := mustTry(t, iv, TryRequest{
		Connection: "shop",
		Operation:  "getCustomer",
		Params:     map[string]string{"customerId": "7", "tenant": "acme"},
	})
	if res.Status != 200 {
		t.Fatalf("status = %d, want 200", res.Status)
	}
	if rec.path != "/customers/7" {
		t.Errorf("path = %q, want the path parameter substituted", rec.path)
	}
	if rec.header.Get("tenant") != "acme" {
		t.Errorf("tenant header = %q, want a header param sent as a header", rec.header.Get("tenant"))
	}
	if !strings.Contains(res.Body, "Ada") {
		t.Errorf("body = %q, want the raw payload", res.Body)
	}
	if res.URL == "" || !strings.HasSuffix(res.URL, "/customers/7") {
		t.Errorf("URL = %q, want the URL that was actually called", res.URL)
	}
}

func TestTry_QueryParamsGoOnTheURL(t *testing.T) {
	iv, rec, _ := shopFixture(t, jsonHandler(200, listBody))

	req := tryReq("listCustomers")
	req.Params = map[string]string{"q": "ada", "limit": "5", "tenant": "acme"}
	mustTry(t, iv, req)

	if rec.query.Get("q") != "ada" || rec.query.Get("limit") != "5" {
		t.Fatalf("query = %v, want the query params sent", rec.query)
	}
	// A header param must not leak onto the query string.
	if rec.query.Has("tenant") {
		t.Errorf("query = %v, want tenant sent as a header only", rec.query)
	}
}

func TestTry_FailingStatusIsAResultNotAnError(t *testing.T) {
	// A console that swallows a 404 is useless: the status and the body are
	// exactly what the author is trying to see.
	iv, _, _ := shopFixture(t, jsonHandler(404, `{"error": "no such customer"}`))

	req := tryReq("getCustomer")
	req.Params = map[string]string{"customerId": "nope", "tenant": "acme"}
	res := mustTry(t, iv, req)

	if res.Status != 404 {
		t.Fatalf("status = %d, want 404", res.Status)
	}
	if !res.Failed {
		t.Error("Failed = false, want a 404 marked as a failure")
	}
	if !strings.Contains(res.Body, "no such customer") {
		t.Errorf("body = %q, want the remote's own error payload", res.Body)
	}
}

func TestTry_NonJSONBodyComesBackVerbatim(t *testing.T) {
	// decodeJSON refuses an HTML login page, which is right for a binding and
	// wrong here: seeing that page is how the author diagnoses the redirect.
	iv, _, _ := shopFixture(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte("<html><body>Please sign in</body></html>"))
	})

	req := tryReq("getCustomer")
	req.Params = map[string]string{"customerId": "7", "tenant": "acme"}
	res := mustTry(t, iv, req)

	if res.JSON {
		t.Error("JSON = true, want an HTML body reported as not JSON")
	}
	if !strings.Contains(res.Body, "Please sign in") {
		t.Errorf("body = %q, want the HTML kept", res.Body)
	}
	if !strings.HasPrefix(res.ContentType, "text/html") {
		t.Errorf("ContentType = %q, want the remote's own type", res.ContentType)
	}
}

func TestTry_PrettyPrintsJSON(t *testing.T) {
	iv, _, _ := shopFixture(t, jsonHandler(200, `{"a":1,"b":[2,3]}`))
	req := tryReq("getCustomer")
	req.Params = map[string]string{"customerId": "7", "tenant": "acme"}
	res := mustTry(t, iv, req)

	if !res.JSON {
		t.Fatal("JSON = false, want a JSON body recognised")
	}
	if !strings.Contains(res.Body, "\n") {
		t.Errorf("body = %q, want it indented for reading", res.Body)
	}
}

func TestTry_MissingRequiredParamNeverLeavesTheMachine(t *testing.T) {
	called := false
	iv, _, _ := shopFixture(t, func(w http.ResponseWriter, r *http.Request) {
		called = true
		_, _ = w.Write([]byte("{}"))
	})

	// customerId is a path param, so there is no request to make without it.
	_, err := iv.Try(context.Background(), tryReq("getCustomer"))
	if got := codeOf(t, err); got != CodeMissingParam {
		t.Fatalf("code = %v, want a missing-param refusal", got)
	}
	if called {
		t.Error("the remote was called despite an unfilled parameter")
	}
}

func TestTry_UndeclaredParamIsRefused(t *testing.T) {
	iv, _, _ := shopFixture(t, jsonHandler(200, "{}"))
	req := tryReq("listCustomers")
	req.Params = map[string]string{"tenant": "acme", "nonsense": "1"}

	_, err := iv.Try(context.Background(), req)
	if got := codeOf(t, err); got != CodeUnknownField {
		t.Fatalf("code = %v, want the undeclared param refused", got)
	}
}

func TestTry_UnknownOperation(t *testing.T) {
	iv, _, _ := shopFixture(t, jsonHandler(200, "{}"))
	_, err := iv.Try(context.Background(), tryReq("noSuchOperation"))
	if got := codeOf(t, err); got != CodeOperationNotFound {
		t.Fatalf("code = %v, want an unknown-operation error", got)
	}
}

func TestTry_RefusesAMutatingMethod(t *testing.T) {
	// A console button that can fire DELETE at production is a hazard, not a
	// feature. Reading is safe and idempotent; writing needs a real intent.
	called := false
	iv, _, _ := shopFixture(t, func(w http.ResponseWriter, r *http.Request) {
		called = true
		_, _ = w.Write([]byte("{}"))
	})

	const specWithWrites = `{
      "openapi": "3.0.0",
      "info": {"title": "Shop", "version": "1"},
      "paths": {"/customers": {
        "get": {"operationId": "listCustomers"},
        "delete": {"operationId": "wipeCustomers"}}}
    }`
	if _, err := iv.mgr.ImportSpec("writes", []byte(specWithWrites)); err != nil {
		t.Fatal(err)
	}
	c := shopConn(t, iv)
	c.SpecFile = "writes.json"
	c.Resources = []Resource{{
		Key: "customers", List: OpRef{Operation: "listCustomers"},
		IDPath: "/id", LabelPath: "/name",
	}}
	if err := iv.mgr.Save(c); err != nil {
		t.Fatal(err)
	}

	_, err := iv.Try(context.Background(), tryReq("wipeCustomers"))
	if got := codeOf(t, err); got != CodeMethodNotAllowed {
		t.Fatalf("code = %v, want the write refused", got)
	}
	if called {
		t.Error("a DELETE reached the remote")
	}
}

func TestTry_BodyIsTruncatedRatherThanRefused(t *testing.T) {
	// A binding treats an oversized payload as fatal, since it cannot project
	// half a document. The console only displays it, so a prefix is useful.
	big := `{"padding":"` + strings.Repeat("x", 4000) + `"}`
	iv, _, _ := shopFixture(t, jsonHandler(200, big), WithMaxResponseBytes(512))

	req := tryReq("getCustomer")
	req.Params = map[string]string{"customerId": "7", "tenant": "acme"}
	res := mustTry(t, iv, req)

	if !res.Truncated {
		t.Fatal("Truncated = false, want the oversized body marked")
	}
	if len(res.Body) > 1024 {
		t.Errorf("body is %d bytes, want it cut to the limit", len(res.Body))
	}
}

func TestTry_ReportsWhatItSent(t *testing.T) {
	iv, _, _ := shopFixture(t, jsonHandler(200, "{}"))
	req := tryReq("listCustomers")
	req.Params = map[string]string{"tenant": "acme", "q": "ada"}
	res := mustTry(t, iv, req)

	if res.Method != "GET" {
		t.Errorf("Method = %q, want GET", res.Method)
	}
	// The URL is the one thing an author cannot reconstruct by hand once
	// dialect defaults and auth params are folded in.
	if !strings.Contains(res.URL, "q=ada") {
		t.Errorf("URL = %q, want the query it actually sent", res.URL)
	}
}

func TestTryForm_ListsEveryParameterWithItsLocation(t *testing.T) {
	cat := mustParse(t, specV3JSON)
	form, err := BuildTryForm(cat, nil, "listCustomers")
	if err != nil {
		t.Fatal(err)
	}
	if !form.Runnable {
		t.Fatalf("form = %+v, want a GET runnable", form)
	}

	byName := map[string]TryParam{}
	for _, p := range form.Params {
		byName[p.Name] = p
	}
	if got := byName["tenant"]; got.In != InHeader || !got.Required {
		t.Errorf("tenant = %+v, want a required header param", got)
	}
	if got := byName["q"]; got.In != InQuery || got.Required {
		t.Errorf("q = %+v, want an optional query param", got)
	}
}

func TestTryForm_PrefillsWhatAResourceAlreadyFixes(t *testing.T) {
	// Trying a bound operation should reproduce the call the field makes, not
	// a bare one the author then has to reconstruct by hand.
	cat := mustParse(t, specV3JSON)
	c := validConn()
	form, err := BuildTryForm(cat, &c, "listCustomers")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range form.Params {
		if p.Name == "tenant" && p.Value != "acme" {
			t.Fatalf("tenant value = %q, want the resource's fixed value", p.Value)
		}
	}
}

func TestTryForm_MarksAMutatingOperationUnrunnable(t *testing.T) {
	cat := mustParse(t, specV3JSON)
	form, err := BuildTryForm(cat, nil, "createCustomer")
	if err != nil {
		t.Fatal(err)
	}
	if form.Runnable {
		t.Fatal("Runnable = true, want a POST refused before the click")
	}
	if form.Reason != "method_not_allowed" {
		t.Errorf("Reason = %q, want a stable key the frontend can translate", form.Reason)
	}
}

func TestTryForm_UnknownOperation(t *testing.T) {
	if _, err := BuildTryForm(mustParse(t, specV3JSON), nil, "nope"); err == nil {
		t.Fatal("want an error for an operation the document does not declare")
	}
}

func TestTry_RedactsAQueryPlacedAPIKey(t *testing.T) {
	// The vault protects this secret; a console screenshot must not leak it.
	iv, _, _ := shopFixture(t, jsonHandler(200, "{}"))
	c := shopConn(t, iv)
	c.Auth = Auth{Kind: AuthAPIKey, In: InQuery, Name: "api_key"}
	if err := iv.mgr.Save(c); err != nil {
		t.Fatal(err)
	}

	req := tryReq("getCustomer")
	req.Params = map[string]string{"customerId": "7", "tenant": "acme"}
	res := mustTry(t, iv, req)

	if strings.Contains(res.URL, "s3cret") {
		t.Fatalf("URL = %q, want the credential redacted", res.URL)
	}
	if !strings.Contains(res.URL, "api_key=%2A%2A%2A") && !strings.Contains(res.URL, "api_key=***") {
		t.Errorf("URL = %q, want the key present but masked", res.URL)
	}
}
