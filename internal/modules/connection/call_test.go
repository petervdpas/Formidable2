package connection

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

func customerCall(id string) CallRequest {
	return CallRequest{
		Connection: "shop",
		Operation:  "getCustomer",
		Params:     map[string]string{"customerId": id, "tenant": "acme"},
	}
}

func TestCall_ReturnsParsedJSON(t *testing.T) {
	iv, rec, _ := shopFixture(t, jsonHandler(200, `{"id": 7, "name": "Ada", "tags": ["a", "b"]}`))
	res, err := iv.Call(context.Background(), customerCall("7"))
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	data, ok := res.Data.(map[string]any)
	if !ok || data["name"] != "Ada" || data["id"] != float64(7) {
		t.Fatalf("Data = %#v", res.Data)
	}
	if res.Status != 200 || res.Body != "" {
		t.Errorf("status = %d, body = %q", res.Status, res.Body)
	}
	if rec.path != "/customers/7" || rec.header.Get("tenant") != "acme" {
		t.Errorf("request = %s, tenant %q", rec.path, rec.header.Get("tenant"))
	}
}

func TestCall_NonJSONBodyComesBackAsText(t *testing.T) {
	iv, _, _ := shopFixture(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/csv")
		_, _ = w.Write([]byte("id,name\n7,Ada\n"))
	})
	res, err := iv.Call(context.Background(), customerCall("7"))
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if res.Data != nil || res.Body != "id,name\n7,Ada\n" || !strings.HasPrefix(res.ContentType, "text/csv") {
		t.Errorf("result = %+v", res)
	}
}

func TestCall_EmptyBodyIsNeitherDataNorAnError(t *testing.T) {
	iv, _, _ := shopFixture(t, func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) })
	res, err := iv.Call(context.Background(), customerCall("7"))
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if res.Status != 204 || res.Data != nil || res.Body != "" {
		t.Errorf("result = %+v", res)
	}
}

func TestCall_FailingStatusIsAnErrorWithItsCode(t *testing.T) {
	for status, want := range map[int]InvokeErrorCode{
		401: CodeUnauthorized, 403: CodeForbidden, 404: CodeRemoteNotFound, 429: CodeRateLimited, 500: CodeRemoteError,
	} {
		iv, _, _ := shopFixture(t, jsonHandler(status, `{"error": "x"}`))
		_, err := iv.Call(context.Background(), customerCall("7"))
		if got := codeOf(t, err); got != want {
			t.Errorf("status %d: code = %s, want %s", status, got, want)
		}
	}
}

func TestCall_RefusedBeforeLeavingTheMachine(t *testing.T) {
	cases := map[string]struct {
		req  CallRequest
		want InvokeErrorCode
	}{
		"unknown operation": {CallRequest{Connection: "shop", Operation: "nope"}, CodeOperationNotFound},
		"unknown client":    {CallRequest{Connection: "absent", Operation: "getCustomer"}, CodeConnectionNotFound},
		"missing param":     {CallRequest{Connection: "shop", Operation: "getCustomer", Params: map[string]string{"tenant": "acme"}}, CodeMissingParam},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			iv, rec, _ := shopFixture(t, jsonHandler(200, `{}`))
			_, err := iv.Call(context.Background(), tc.req)
			if got := codeOf(t, err); got != tc.want {
				t.Errorf("code = %s, want %s", got, tc.want)
			}
			if rec.path != "" {
				t.Errorf("a request went out to %s", rec.path)
			}
		})
	}
}

func TestCall_OversizedBodyIsRefusedNotCut(t *testing.T) {
	big := `{"pad": "` + strings.Repeat("x", 2048) + `"}`
	iv, _, _ := shopFixture(t, jsonHandler(200, big), WithMaxResponseBytes(512))
	_, err := iv.Call(context.Background(), customerCall("7"))
	if got := codeOf(t, err); got != CodeTooLarge {
		t.Errorf("code = %s, want too_large", got)
	}
}

func TestCall_CancelledContext(t *testing.T) {
	iv, _, _ := shopFixture(t, jsonHandler(200, `{}`))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := iv.Call(ctx, customerCall("7"))
	if got := codeOf(t, err); got != CodeCanceled {
		t.Errorf("code = %s, want canceled", got)
	}
}

func TestCall_ReportsTheRoundTripForLogging(t *testing.T) {
	iv, _, _ := shopFixture(t, jsonHandler(200, `{}`))
	res, err := iv.Call(context.Background(), CallRequest{
		Connection: "shop", Operation: "listCustomers", Params: map[string]string{"q": "ada", "tenant": "acme"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Method != "GET" || !strings.Contains(res.URL, "/customers?q=ada") || res.DurationMS < 0 {
		t.Errorf("round trip = %s %s (%d ms)", res.Method, res.URL, res.DurationMS)
	}
}

// writesSpec declares one operation per method the document format knows, and
// the body shapes a caller may have to send.
const writesSpec = `{
  "openapi": "3.0.0",
  "info": {"title": "Shop", "version": "1"},
  "paths": {
    "/customers": {
      "get": {"operationId": "listCustomers"},
      "post": {"operationId": "addCustomer", "requestBody": {"required": true,
        "content": {"application/json": {"schema": {"type": "object"}}}}},
      "delete": {"operationId": "wipeCustomers"}},
    "/customers/{id}": {
      "parameters": [{"name": "id", "in": "path", "required": true, "schema": {"type": "string"}}],
      "put": {"operationId": "replaceCustomer", "requestBody": {
        "content": {"application/json": {"schema": {"type": "object"}}}}},
      "patch": {"operationId": "patchCustomer", "requestBody": {
        "content": {"application/merge-patch+json": {"schema": {"type": "object"}}}}},
      "delete": {"operationId": "removeCustomer"},
      "head": {"operationId": "probeCustomer"},
      "options": {"operationId": "customerOptions"}},
    "/login": {"post": {"operationId": "login", "requestBody": {
      "content": {"application/x-www-form-urlencoded": {"schema": {"type": "object"}}}}}},
    "/notes": {"post": {"operationId": "addNote", "requestBody": {
      "content": {"text/markdown": {"schema": {"type": "string"}}}}}},
    "/files": {"post": {"operationId": "upload", "requestBody": {
      "content": {"multipart/form-data": {"schema": {"type": "object"}}}}}},
    "/documents/{id}": {"put": {"operationId": "putDocument",
      "parameters": [{"name": "id", "in": "path", "required": true, "schema": {"type": "string"}}],
      "requestBody": {"required": true, "content": {"application/pdf": {"schema": {"type": "string", "format": "binary"}}}}}},
    "/ping": {"post": {"operationId": "ping"}}
  }
}`

type sent struct {
	method, path, contentType, body string
	hits                            int
}

// writesFixture swaps the shop client onto writesSpec and records each request.
func writesFixture(t *testing.T, status int, answer string) (*Invoker, *sent) {
	t.Helper()
	got := &sent{}
	iv, _, _ := shopFixture(t, func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		got.method, got.path, got.contentType, got.body = r.Method, r.URL.Path, r.Header.Get("Content-Type"), string(raw)
		got.hits++
		if answer != "" {
			w.Header().Set("Content-Type", "application/json")
		}
		w.WriteHeader(status)
		_, _ = w.Write([]byte(answer))
	})
	if _, err := iv.mgr.ImportSpec("writes", []byte(writesSpec)); err != nil {
		t.Fatal(err)
	}
	c := shopConn(t, iv)
	c.SpecFile = "writes.json"
	c.Resources = nil
	if err := iv.mgr.Save(c); err != nil {
		t.Fatal(err)
	}
	return iv, got
}

func TestCall_SendsEveryMethodTheDocumentDeclares(t *testing.T) {
	cases := []struct {
		op, method, path string
		params           map[string]string
		body             any
	}{
		{"listCustomers", "GET", "/customers", nil, nil},
		{"addCustomer", "POST", "/customers", nil, map[string]any{"name": "Ada"}},
		{"replaceCustomer", "PUT", "/customers/7", map[string]string{"id": "7"}, map[string]any{"name": "Ada"}},
		{"patchCustomer", "PATCH", "/customers/7", map[string]string{"id": "7"}, map[string]any{"name": "Ada"}},
		{"removeCustomer", "DELETE", "/customers/7", map[string]string{"id": "7"}, nil},
		{"wipeCustomers", "DELETE", "/customers", nil, nil},
		{"probeCustomer", "HEAD", "/customers/7", map[string]string{"id": "7"}, nil},
		{"customerOptions", "OPTIONS", "/customers/7", map[string]string{"id": "7"}, nil},
	}
	for _, tc := range cases {
		t.Run(tc.op, func(t *testing.T) {
			iv, got := writesFixture(t, 200, `{"ok": true}`)
			res, err := iv.Call(context.Background(), CallRequest{Connection: "shop", Operation: tc.op, Params: tc.params, Body: tc.body})
			if err != nil {
				t.Fatalf("Call: %v", err)
			}
			if got.method != tc.method || got.path != tc.path || res.Method != tc.method || res.Status != 200 {
				t.Errorf("sent %s %s, result %s %d", got.method, got.path, res.Method, res.Status)
			}
		})
	}
}

func TestCall_EncodesTheBodyByWhatTheOperationAccepts(t *testing.T) {
	cases := map[string]struct {
		op          string
		params      map[string]string
		body        any
		contentType string
		wantType    string
		wantBody    string
	}{
		"json object":            {"addCustomer", nil, map[string]any{"name": "Ada", "age": float64(36)}, "", "application/json", `{"age":36,"name":"Ada"}`},
		"json array":             {"addCustomer", nil, []any{"a", float64(1), true}, "", "application/json", `["a",1,true]`},
		"json scalar":            {"addCustomer", nil, float64(42), "", "application/json", `42`},
		"json given as text":     {"addCustomer", nil, `{"name":"Ada"}`, "", "application/json", `{"name":"Ada"}`},
		"vendor json type":       {"patchCustomer", map[string]string{"id": "7"}, map[string]any{"name": "Ada"}, "", "application/merge-patch+json", `{"name":"Ada"}`},
		"form from the document": {"login", nil, map[string]any{"user": "ada", "remember": true, "n": float64(2)}, "", "application/x-www-form-urlencoded", "n=2&remember=true&user=ada"},
		"text from the document": {"addNote", nil, "# hello", "", "text/markdown", "# hello"},
		"caller's type wins":     {"addNote", nil, "<p>hello</p>", "text/html", "text/html", "<p>hello</p>"},
		"undeclared body, table": {"ping", nil, map[string]any{"a": float64(1)}, "", "application/json", `{"a":1}`},
		"undeclared body, text":  {"ping", nil, "hello", "", "text/plain", "hello"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			iv, got := writesFixture(t, 201, `{"id": 1}`)
			_, err := iv.Call(context.Background(), CallRequest{
				Connection: "shop", Operation: tc.op, Params: tc.params, Body: tc.body, ContentType: tc.contentType,
			})
			if err != nil {
				t.Fatalf("Call: %v", err)
			}
			if got.contentType != tc.wantType || got.body != tc.wantBody {
				t.Errorf("sent %q as %q, want %q as %q", got.body, got.contentType, tc.wantBody, tc.wantType)
			}
		})
	}
}

func TestCall_NoBodySendsNoContentType(t *testing.T) {
	iv, got := writesFixture(t, 204, "")
	res, err := iv.Call(context.Background(), CallRequest{Connection: "shop", Operation: "removeCustomer", Params: map[string]string{"id": "7"}})
	if err != nil {
		t.Fatal(err)
	}
	if got.contentType != "" || got.body != "" || res.Status != 204 || res.Data != nil {
		t.Errorf("sent type %q body %q; result %+v", got.contentType, got.body, res)
	}
}

func TestCall_BadBodiesNeverLeaveTheMachine(t *testing.T) {
	cases := map[string]struct {
		req  CallRequest
		want InvokeErrorCode
	}{
		"required body missing": {CallRequest{Connection: "shop", Operation: "addCustomer"}, CodeMissingParam},
		"multipart declared":    {CallRequest{Connection: "shop", Operation: "upload", Body: "x"}, CodeBindingInvalid},
		"multipart requested":   {CallRequest{Connection: "shop", Operation: "ping", Body: "x", ContentType: "multipart/form-data"}, CodeBindingInvalid},
		"form from a list":      {CallRequest{Connection: "shop", Operation: "login", Body: []any{"a"}}, CodeBindingInvalid},
		"form with a nested":    {CallRequest{Connection: "shop", Operation: "login", Body: map[string]any{"a": map[string]any{}}}, CodeBindingInvalid},
		"table as plain text":   {CallRequest{Connection: "shop", Operation: "ping", Body: map[string]any{"a": 1.0}, ContentType: "text/csv"}, CodeBindingInvalid},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			iv, got := writesFixture(t, 200, `{}`)
			_, err := iv.Call(context.Background(), tc.req)
			if code := codeOf(t, err); code != tc.want {
				t.Errorf("code = %s, want %s", code, tc.want)
			}
			if got.hits != 0 {
				t.Error("a request went out")
			}
		})
	}
}

func TestCall_FailedWriteCarriesItsCode(t *testing.T) {
	iv, got := writesFixture(t, 403, `{"error": "read only"}`)
	_, err := iv.Call(context.Background(), CallRequest{Connection: "shop", Operation: "wipeCustomers"})
	if code := codeOf(t, err); code != CodeForbidden || got.hits != 1 {
		t.Errorf("code = %s, hits = %d", code, got.hits)
	}
}

func TestTry_StillRefusesWritesWhileCallSendsThem(t *testing.T) {
	iv, got := writesFixture(t, 200, `{}`)
	_, err := iv.Try(context.Background(), TryRequest{Connection: "shop", Operation: "wipeCustomers"})
	if code := codeOf(t, err); code != CodeMethodNotAllowed || got.hits != 0 {
		t.Errorf("console: code = %s, hits = %d; the console must stay read-only", code, got.hits)
	}
}

func TestParseSpec_DescribesRequestBodies(t *testing.T) {
	cat, err := ParseSpec([]byte(writesSpec))
	if err != nil {
		t.Fatal(err)
	}
	add, _ := cat.Op("addCustomer")
	if add.Body == nil || !add.Body.Required || len(add.Body.ContentTypes) != 1 || add.Body.ContentTypes[0] != "application/json" {
		t.Errorf("addCustomer body = %+v", add.Body)
	}
	if list, _ := cat.Op("listCustomers"); list.Body != nil {
		t.Errorf("listCustomers body = %+v, want none", list.Body)
	}
	// Swagger 2 states a body as a parameter; it must surface the same way.
	v2 := `{"swagger": "2.0", "info": {"title": "T", "version": "1"}, "host": "x.example.com", "paths": {"/pet": {"post": {
		"operationId": "addPet", "consumes": ["application/json"],
		"parameters": [{"in": "body", "name": "body", "required": true, "schema": {"type": "object"}}],
		"responses": {"200": {"description": "ok"}}}}}}`
	cat2, err := ParseSpec([]byte(v2))
	if err != nil {
		t.Fatal(err)
	}
	pet, _ := cat2.Op("addPet")
	if pet.Body == nil || !pet.Body.Required || len(pet.Body.ContentTypes) == 0 {
		t.Errorf("swagger 2 addPet body = %+v", pet.Body)
	}
	if _, isParam := pet.Param("body"); isParam {
		t.Error("the body leaked into the parameter list")
	}
}

// realPDF is a complete one-page PDF. Its second line is the binary marker
// every PDF writer emits (bytes above 0x7F), and a NUL and a CRLF are added in
// a comment: the bytes a text-minded transport would mangle.
var realPDF = "%PDF-1.4\n%\xe2\xe3\xcf\xd3\n% \x00 \r\n \xff\xfe\n" +
	"1 0 obj\n<< /Type /Catalog /Pages 2 0 R >>\nendobj\n" +
	"2 0 obj\n<< /Type /Pages /Kids [3 0 R] /Count 1 >>\nendobj\n" +
	"3 0 obj\n<< /Type /Page /Parent 2 0 R /MediaBox [0 0 200 200] >>\nendobj\n" +
	"trailer\n<< /Root 1 0 R >>\n%%EOF\n"

func TestCall_BinaryBodyArrivesByteForByte(t *testing.T) {
	cases := map[string]struct {
		op          string
		params      map[string]string
		contentType string
	}{
		"operation declares application/pdf": {"putDocument", map[string]string{"id": "inv-7"}, ""},
		"caller names the type":              {"ping", nil, "application/pdf"},
		"caller overrides a JSON operation":  {"addCustomer", nil, "application/pdf"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			iv, got := writesFixture(t, 201, `{"stored": true}`)
			_, err := iv.Call(context.Background(), CallRequest{
				Connection: "shop", Operation: tc.op, Params: tc.params, Body: realPDF, ContentType: tc.contentType,
			})
			if err != nil {
				t.Fatalf("Call: %v", err)
			}
			if got.contentType != "application/pdf" {
				t.Errorf("content type = %q", got.contentType)
			}
			if got.body != realPDF {
				t.Errorf("%d bytes arrived, %d were sent; the body was altered in transit", len(got.body), len(realPDF))
			}
			if got.hits != 1 {
				t.Errorf("the write was sent %d times", got.hits)
			}
		})
	}
}

func TestCall_LargeBinaryBodyIsNotCappedByTheResponseLimit(t *testing.T) {
	big := strings.Repeat(realPDF, 20000) // several megabytes, far over the 512-byte answer cap below
	got := &sent{}
	iv, _, _ := shopFixture(t, func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		got.body = string(raw)
		got.hits++
		w.WriteHeader(204)
	}, WithMaxResponseBytes(512))
	if _, err := iv.mgr.ImportSpec("writes", []byte(writesSpec)); err != nil {
		t.Fatal(err)
	}
	c := shopConn(t, iv)
	c.SpecFile, c.Resources = "writes.json", nil
	if err := iv.mgr.Save(c); err != nil {
		t.Fatal(err)
	}
	if _, err := iv.Call(context.Background(), CallRequest{Connection: "shop", Operation: "putDocument", Params: map[string]string{"id": "x"}, Body: big}); err != nil {
		t.Fatalf("Call: %v", err)
	}
	if got.body != big {
		t.Errorf("%d of %d bytes arrived", len(got.body), len(big))
	}
}

func invokeError(t *testing.T, err error) *InvokeError {
	t.Helper()
	var ie *InvokeError
	if !errors.As(err, &ie) {
		t.Fatalf("err = %v, want an *InvokeError", err)
	}
	return ie
}

func TestCall_RefusalCarriesWhatTheRemoteAnswered(t *testing.T) {
	iv, _ := writesFixture(t, 409, `{"error": "ledger.error.duplicate_guid"}`)
	_, err := iv.Call(context.Background(), CallRequest{Connection: "shop", Operation: "addCustomer", Body: map[string]any{"guid": "g1"}})
	ie := invokeError(t, err)
	if ie.Status != 409 || ie.Body != `{"error": "ledger.error.duplicate_guid"}` || ie.Delivery() != DeliveryAnswered {
		t.Errorf("status = %d, body = %q, delivery = %s", ie.Status, ie.Body, ie.Delivery())
	}
}

func TestCall_RefusalBodyIsCapped(t *testing.T) {
	iv, _, _ := shopFixture(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(422)
		_, _ = w.Write([]byte(strings.Repeat("x", maxErrorBody+500)))
	})
	_, err := iv.Call(context.Background(), customerCall("7"))
	if ie := invokeError(t, err); len(ie.Body) != maxErrorBody {
		t.Errorf("kept %d bytes of the refusal, want %d", len(ie.Body), maxErrorBody)
	}
}

func TestCall_WriteIsNeverSentTwiceThroughARedirect(t *testing.T) {
	for _, status := range []int{301, 302, 303, 307, 308} {
		hits := 0
		iv, _, _ := shopFixture(t, func(w http.ResponseWriter, r *http.Request) {
			hits++
			if r.URL.Path == "/moved" {
				t.Errorf("status %d: the write was sent on to the redirect target as %s", status, r.Method)
				return
			}
			http.Redirect(w, r, "/moved", status)
		})
		if _, err := iv.mgr.ImportSpec("writes", []byte(writesSpec)); err != nil {
			t.Fatal(err)
		}
		c := shopConn(t, iv)
		c.SpecFile, c.Resources = "writes.json", nil
		if err := iv.mgr.Save(c); err != nil {
			t.Fatal(err)
		}
		_, err := iv.Call(context.Background(), CallRequest{Connection: "shop", Operation: "addCustomer", Body: map[string]any{"name": "Ada"}})
		ie := invokeError(t, err)
		if ie.Code != CodeRedirected || ie.Status != status || ie.Delivery() != DeliveryAnswered || hits != 1 {
			t.Errorf("status %d: code = %s, delivery = %s, requests = %d", status, ie.Code, ie.Delivery(), hits)
		}
	}
}

func TestCall_ReadStillFollowsASameHostRedirect(t *testing.T) {
	iv, _, _ := shopFixture(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/moved" {
			http.Redirect(w, r, "/moved", 302)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"name": "Ada"}`))
	})
	res, err := iv.Call(context.Background(), customerCall("7"))
	if err != nil || res.Status != 200 {
		t.Fatalf("res = %+v, err = %v; a read may follow a redirect on the same host", res, err)
	}
}

func TestInvokeError_DeliverySaysWhatTheCallerMayConclude(t *testing.T) {
	// Refused before leaving the machine.
	iv, got := writesFixture(t, 200, `{}`)
	_, err := iv.Call(context.Background(), CallRequest{Connection: "shop", Operation: "addCustomer"})
	if d := invokeError(t, err).Delivery(); d != DeliveryNotSent || got.hits != 0 {
		t.Errorf("missing body: delivery = %s, hits = %d", d, got.hits)
	}

	// No connection could be made: nothing was sent.
	iv2, _, _ := shopFixture(t, jsonHandler(200, `{}`))
	c := shopConn(t, iv2)
	c.BaseURL = "http://127.0.0.1:1"
	if err := iv2.mgr.Save(c); err != nil {
		t.Fatal(err)
	}
	_, err = iv2.Call(context.Background(), customerCall("7"))
	if ie := invokeError(t, err); ie.Code != CodeUnreachable || ie.Delivery() != DeliveryNotSent {
		t.Errorf("refused connection: code = %s, delivery = %s", ie.Code, ie.Delivery())
	}

	// The remote took the request and never answered: it may have acted.
	iv3, _, _ := shopFixture(t, func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(300 * time.Millisecond)
	}, WithTimeout(50*time.Millisecond))
	_, err = iv3.Call(context.Background(), customerCall("7"))
	if ie := invokeError(t, err); ie.Code != CodeTimeout || ie.Delivery() != DeliveryUnknown {
		t.Errorf("timeout: code = %s, delivery = %s", ie.Code, ie.Delivery())
	}

	// The remote dropped the connection after taking the request.
	iv4, _, _ := shopFixture(t, func(w http.ResponseWriter, r *http.Request) {
		conn, _, _ := w.(http.Hijacker).Hijack()
		_ = conn.Close()
	})
	_, err = iv4.Call(context.Background(), customerCall("7"))
	if ie := invokeError(t, err); ie.Delivery() != DeliveryUnknown {
		t.Errorf("dropped connection: code = %s, delivery = %s", ie.Code, ie.Delivery())
	}

	// The caller gave up.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = iv.Call(ctx, CallRequest{Connection: "shop", Operation: "listCustomers"})
	if d := invokeError(t, err).Delivery(); d != DeliveryUnknown {
		t.Errorf("cancelled: delivery = %s", d)
	}
}

func TestInvokeError_EveryCodeHasADelivery(t *testing.T) {
	codes := []InvokeErrorCode{
		CodeConnectionNotFound, CodeResourceNotFound, CodeBindingInvalid, CodeUnknownField, CodeNotConfigured,
		CodeOperationNotFound, CodeMissingParam, CodeMethodNotAllowed, CodeUnreachable, CodeTimeout, CodeCanceled,
		CodeUnauthorized, CodeForbidden, CodeRemoteNotFound, CodeRateLimited, CodeRemoteError, CodeRedirected,
		CodeBadResponse, CodeShapeMismatch, CodeTooLarge,
	}
	for _, code := range codes {
		if _, ok := deliveryByCode[code]; !ok && code != CodeUnreachable {
			t.Errorf("code %s has no delivery; a new code must say what a caller may conclude", code)
		}
	}
	var none *InvokeError
	if none.Delivery() != DeliveryUnknown {
		t.Error("a nil error must not claim anything was or was not sent")
	}
}
