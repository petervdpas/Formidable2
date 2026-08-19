package connection

import (
	"context"
	"testing"
)

// A resource binding carries the params that are always the same (a tenant
// header, an api version). A field asks for specific data at fetch time, so the
// request carries its own params on top: filters driven by the record being
// edited, not baked into the client.

func TestList_RuntimeParamReachesTheQuery(t *testing.T) {
	iv, rec, _ := shopFixture(t, jsonHandler(200, listBody))
	req := listReq()
	req.Params = map[string]string{"q": "acme"}
	mustList(t, iv, req)
	if got := rec.query.Get("q"); got != "acme" {
		t.Errorf("q = %q, want acme (raw %s)", got, rec.rawURL)
	}
}

func TestList_RuntimeParamReachesTheHeader(t *testing.T) {
	iv, rec, _ := shopFixture(t, jsonHandler(200, listBody))
	req := listReq()
	req.Params = map[string]string{"tenant": "globex"}
	mustList(t, iv, req)
	if got := rec.header.Get("tenant"); got != "globex" {
		t.Errorf("tenant header = %q, want globex", got)
	}
}

// The binding's value is the default; the caller's is the specific ask.
func TestList_RuntimeParamOverridesTheBinding(t *testing.T) {
	iv, rec, _ := shopFixture(t, jsonHandler(200, listBody))
	req := listReq()
	req.Params = map[string]string{"tenant": "globex"}
	mustList(t, iv, req)
	if got := rec.header.Get("tenant"); got == "acme" {
		t.Error("binding param won over the runtime param")
	}
}

// A param the operation does not declare is a broken field config. Failing
// loudly beats silently returning the unfiltered list.
func TestList_UnknownRuntimeParamIsRejected(t *testing.T) {
	iv, _, _ := shopFixture(t, jsonHandler(200, listBody))
	req := listReq()
	req.Params = map[string]string{"nosuch": "x"}
	_, err := iv.List(context.Background(), req)
	if err == nil {
		t.Fatal("expected an error for an undeclared param")
	}
	if code := codeOf(t, err); code != CodeUnknownField {
		t.Errorf("code = %q, want %q", code, CodeUnknownField)
	}
}

// Search stays the resource's declared search param: an explicit param of the
// same name is the more specific instruction and wins.
func TestList_ExplicitParamBeatsSearch(t *testing.T) {
	iv, rec, _ := shopFixture(t, jsonHandler(200, listBody))
	req := listReq()
	req.Search = "ignored"
	req.Params = map[string]string{"q": "explicit"}
	mustList(t, iv, req)
	if got := rec.query.Get("q"); got != "explicit" {
		t.Errorf("q = %q, want explicit", got)
	}
}

func TestFetch_RuntimeParamReachesTheHeader(t *testing.T) {
	iv, rec, _ := shopFixture(t, jsonHandler(200, oneCustomer))
	_, err := iv.Fetch(context.Background(), FetchRequest{
		Connection: "shop", Resource: "customers", ID: "7",
		Params: map[string]string{"tenant": "globex"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := rec.header.Get("tenant"); got != "globex" {
		t.Errorf("tenant header = %q, want globex", got)
	}
}

// The path id is the field's stored id, so a runtime param must not be able to
// hijack the path placeholder and fetch a different record.
func TestFetch_RuntimeParamCannotOverrideTheID(t *testing.T) {
	iv, rec, _ := shopFixture(t, jsonHandler(200, oneCustomer))
	_, err := iv.Fetch(context.Background(), FetchRequest{
		Connection: "shop", Resource: "customers", ID: "7",
		Params: map[string]string{"customerId": "9"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if rec.path != "/customers/7" {
		t.Errorf("path = %q, want /customers/7", rec.path)
	}
}
