package connection

import (
	"context"
	"net/http"
	"strings"
	"testing"
)

func customerCall(id string) TryRequest {
	return TryRequest{
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
		req  TryRequest
		want InvokeErrorCode
	}{
		"unknown operation": {TryRequest{Connection: "shop", Operation: "nope"}, CodeOperationNotFound},
		"unknown client":    {TryRequest{Connection: "absent", Operation: "getCustomer"}, CodeConnectionNotFound},
		"missing param":     {TryRequest{Connection: "shop", Operation: "getCustomer", Params: map[string]string{"tenant": "acme"}}, CodeMissingParam},
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

func TestCall_WriteMethodIsRefused(t *testing.T) {
	iv, rec, _ := shopFixture(t, jsonHandler(200, `{}`))
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
	c.Resources = nil
	if err := iv.mgr.Save(c); err != nil {
		t.Fatal(err)
	}
	_, err := iv.Call(context.Background(), TryRequest{Connection: "shop", Operation: "wipeCustomers"})
	if got := codeOf(t, err); got != CodeMethodNotAllowed {
		t.Errorf("code = %s, want the write refused", got)
	}
	if rec.path != "" {
		t.Errorf("a request went out to %s", rec.path)
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
	req := tryReq("listCustomers")
	req.Params = map[string]string{"q": "ada", "tenant": "acme"}
	res, err := iv.Call(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if res.Method != "GET" || !strings.Contains(res.URL, "/customers?q=ada") || res.DurationMS < 0 {
		t.Errorf("round trip = %s %s (%d ms)", res.Method, res.URL, res.DurationMS)
	}
}
