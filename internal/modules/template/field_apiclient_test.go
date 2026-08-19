package template

import "testing"

// The api-client field references a record on a remote service reached through
// an app-level API client. Unlike api (id-only, target lives in this repo) it
// caches what it fetches, because the client definition, the spec file and the
// vault secret all sit outside the synced tree.

func apiClientField() Field {
	return Field{
		Key: "customer", Type: "api-client",
		ClientID: "crm", Resource: "customers",
		Map: []APIMap{{Key: "name", Label: "Name"}},
	}
}

func TestValidate_ApiClientIsKnownType(t *testing.T) {
	errs := Validate(&Template{Fields: []Field{apiClientField()}})
	if hasErr(errs, "unknown-field-type") {
		t.Errorf("api-client should be a known type; got %+v", errs)
	}
}

func TestValidate_ApiClientCleanFieldHasNoErrors(t *testing.T) {
	errs := Validate(&Template{Fields: []Field{apiClientField()}})
	for _, e := range errs {
		t.Errorf("clean api-client field should validate; got %+v", e)
	}
}

func TestValidate_ApiClientRequiresClientID(t *testing.T) {
	f := apiClientField()
	f.ClientID = "  "
	errs := Validate(&Template{Fields: []Field{f}})
	if !hasErr(errs, "api-client-required") {
		t.Errorf("expected api-client-required; got %+v", errs)
	}
}

func TestValidate_ApiClientRequiresResource(t *testing.T) {
	f := apiClientField()
	f.Resource = ""
	errs := Validate(&Template{Fields: []Field{f}})
	if !hasErr(errs, "api-client-resource-required") {
		t.Errorf("expected api-client-resource-required; got %+v", errs)
	}
}

func TestValidate_ApiClientMapKeyRequired(t *testing.T) {
	f := apiClientField()
	f.Map = []APIMap{{Key: " ", Label: "Blank"}}
	errs := Validate(&Template{Fields: []Field{f}})
	if !hasErr(errs, "api-client-map-key-required") {
		t.Errorf("expected api-client-map-key-required; got %+v", errs)
	}
}

func TestValidate_ApiClientMapDuplicateKeys(t *testing.T) {
	f := apiClientField()
	f.Map = []APIMap{{Key: "name"}, {Key: "NAME"}}
	errs := Validate(&Template{Fields: []Field{f}})
	if !hasErr(errs, "api-client-map-duplicate-keys") {
		t.Errorf("expected api-client-map-duplicate-keys; got %+v", errs)
	}
}

// An empty Map is legal: the field then stores the id and label only.
func TestValidate_ApiClientEmptyMapIsFine(t *testing.T) {
	f := apiClientField()
	f.Map = nil
	errs := Validate(&Template{Fields: []Field{f}})
	for _, e := range errs {
		t.Errorf("empty map is legal; got %+v", e)
	}
}

// client_id / resource / multiple are dead data anywhere else.
func TestValidate_ApiClientGroupForbiddenOnOtherTypes(t *testing.T) {
	for _, ty := range []string{"text", "api"} {
		f := Field{Key: "x", Type: ty, ClientID: "crm"}
		if ty == "api" {
			f.Collection = "c"
		}
		errs := Validate(&Template{Fields: []Field{f}})
		if !hasForbidden(errs, "x", "api_client") {
			t.Errorf("type %q: expected forbidden-attribute(api_client); got %+v", ty, errs)
		}
	}
}

// Collection and Filter belong to api, never to api-client.
func TestValidate_ApiCollectionForbiddenOnApiClient(t *testing.T) {
	f := apiClientField()
	f.Collection = "notes.yaml"
	errs := Validate(&Template{Fields: []Field{f}})
	if !hasForbidden(errs, "customer", "api") {
		t.Errorf("expected forbidden-attribute(api) on api-client; got %+v", errs)
	}
}

// Map is shared by api and api-client, so it must not trip either group check.
func TestValidate_MapAloneIsNotFlaggedOnApiClient(t *testing.T) {
	errs := Validate(&Template{Fields: []Field{apiClientField()}})
	if anyForbiddenFor(errs, "customer") {
		t.Errorf("api-client owns Map; got %+v", errs)
	}
}

// ── Runtime parameters ────────────────────────────────────────────────

// The client's binding carries what is always true of the service. The field
// carries what THIS field asks for, and a param may read another field of the
// record being edited, so the remote call narrows to the record's own context.

func TestValidate_ApiClientParamsAreOptional(t *testing.T) {
	f := apiClientField()
	f.Params = nil
	errs := Validate(&Template{Fields: []Field{f}})
	for _, e := range errs {
		t.Errorf("params are optional; got %+v", e)
	}
}

func TestValidate_ApiClientParamNameRequired(t *testing.T) {
	f := apiClientField()
	f.Params = []APIParam{{Value: "acme"}}
	errs := Validate(&Template{Fields: []Field{f}})
	if !hasErr(errs, "api-client-param-name-required") {
		t.Errorf("expected api-client-param-name-required; got %+v", errs)
	}
}

func TestValidate_ApiClientParamDuplicateNames(t *testing.T) {
	f := apiClientField()
	f.Params = []APIParam{{Name: "q", Value: "a"}, {Name: "Q", Value: "b"}}
	errs := Validate(&Template{Fields: []Field{f}})
	if !hasErr(errs, "api-client-param-duplicate") {
		t.Errorf("expected api-client-param-duplicate; got %+v", errs)
	}
}

// A literal and a field reference are two different instructions; declaring
// both leaves it ambiguous which one the author meant.
func TestValidate_ApiClientParamCannotBeBothLiteralAndFieldRef(t *testing.T) {
	f := apiClientField()
	f.Params = []APIParam{{Name: "q", Value: "acme", FieldKey: "name"}}
	errs := Validate(&Template{
		Fields: []Field{f, {Key: "name", Type: "text"}},
	})
	if !hasErr(errs, "api-client-param-ambiguous") {
		t.Errorf("expected api-client-param-ambiguous; got %+v", errs)
	}
}

func TestValidate_ApiClientParamFieldRefMustExist(t *testing.T) {
	f := apiClientField()
	f.Params = []APIParam{{Name: "q", FieldKey: "nosuch"}}
	errs := Validate(&Template{Fields: []Field{f}})
	if !hasErr(errs, "api-client-param-unknown-field") {
		t.Errorf("expected api-client-param-unknown-field; got %+v", errs)
	}
}

func TestValidate_ApiClientParamFieldRefResolves(t *testing.T) {
	f := apiClientField()
	f.Params = []APIParam{{Name: "q", FieldKey: "company"}}
	errs := Validate(&Template{
		Fields: []Field{f, {Key: "company", Type: "text"}},
	})
	for _, e := range errs {
		t.Errorf("a resolvable field reference is legal; got %+v", e)
	}
}

// A param may point at nothing at all: an empty literal is a legitimate
// "send this param blank", and the editor writes rows before they are filled.
func TestValidate_ApiClientParamEmptyLiteralIsFine(t *testing.T) {
	f := apiClientField()
	f.Params = []APIParam{{Name: "q"}}
	errs := Validate(&Template{Fields: []Field{f}})
	for _, e := range errs {
		t.Errorf("an empty literal is legal; got %+v", e)
	}
}

// The param group is api-client's own, so it is dead data elsewhere.
func TestValidate_ApiClientParamsForbiddenOnOtherTypes(t *testing.T) {
	errs := Validate(&Template{Fields: []Field{
		{Key: "x", Type: "text", Params: []APIParam{{Name: "q"}}},
	}})
	if !hasForbidden(errs, "x", "api_client") {
		t.Errorf("expected forbidden-attribute(api_client); got %+v", errs)
	}
}
