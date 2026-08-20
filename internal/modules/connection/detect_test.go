package connection

import (
	"reflect"
	"slices"
	"testing"
)

const specDetectREST = `{
  "openapi": "3.0.3",
  "info": {"title": "CRM", "version": "1.0"},
  "servers": [{"url": "https://crm.example.com/v2"}],
  "paths": {
    "/customers": {
      "get": {"operationId": "listCustomers", "parameters": [
        {"name": "q", "in": "query", "schema": {"type": "string"}},
        {"name": "limit", "in": "query", "schema": {"type": "integer"}},
        {"name": "offset", "in": "query", "schema": {"type": "integer"}}],
        "responses": {"200": {"description": "ok", "content": {"application/json": {"schema": {
          "type": "object", "properties": {
            "total": {"type": "integer"},
            "items": {"type": "array", "items": {"$ref": "#/components/schemas/Customer"}}}}}}}}},
      "post": {"operationId": "createCustomer"}
    },
    "/customers/{customerId}": {
      "get": {"operationId": "getCustomer", "parameters": [
        {"name": "customerId", "in": "path", "required": true, "schema": {"type": "string"}}],
        "responses": {"200": {"description": "ok", "content": {"application/json": {
          "schema": {"$ref": "#/components/schemas/Customer"}}}}}}
    },
    "/customers/{customerId}/orders": {
      "get": {"operationId": "listCustomerOrders", "parameters": [
        {"name": "customerId", "in": "path", "required": true, "schema": {"type": "string"}}],
        "responses": {"200": {"description": "ok", "content": {"application/json": {"schema": {
          "type": "array", "items": {"type": "object", "properties": {"id": {"type": "string"}}}}}}}}}
    },
    "/health": {
      "get": {"operationId": "health", "responses": {"200": {"description": "ok",
        "content": {"application/json": {"schema": {"type": "object",
          "properties": {"status": {"type": "string"}}}}}}}}
    }
  },
  "components": {"schemas": {
    "Customer": {"type": "object", "properties": {
      "id": {"type": "string"},
      "name": {"type": "string"},
      "credit": {"type": "number"},
      "address": {"type": "object", "properties": {"city": {"type": "string"}}}}}
  }}
}`

func drafts(t *testing.T, src string, c *Connection) []ResourceDraft {
	t.Helper()
	return DetectResources(mustParse(t, src), c)
}

func draftFor(t *testing.T, list []ResourceDraft, key string) ResourceDraft {
	t.Helper()
	for _, d := range list {
		if d.Resource.Key == key {
			return d
		}
	}
	t.Fatalf("no draft for %q; got %v", key, draftKeys(list))
	return ResourceDraft{}
}

func draftKeys(list []ResourceDraft) []string {
	out := make([]string, 0, len(list))
	for _, d := range list {
		out = append(out, d.Resource.Key)
	}
	return out
}

func TestDetect_PairsAListWithItsEntityOperation(t *testing.T) {
	got := drafts(t, specDetectREST, nil)
	if keys := draftKeys(got); !reflect.DeepEqual(keys, []string{"customers"}) {
		t.Fatalf("drafts = %v, want just customers", keys)
	}
	r := draftFor(t, got, "customers").Resource
	if r.List.Operation != "listCustomers" || r.Get.Operation != "getCustomer" {
		t.Errorf("bindings = %q/%q, want listCustomers/getCustomer", r.List.Operation, r.Get.Operation)
	}
	if r.Label != "Customers" {
		t.Errorf("Label = %q, want Customers", r.Label)
	}
}

func TestDetect_SkipsNestedCollectionsAndNonCollections(t *testing.T) {
	got := draftKeys(drafts(t, specDetectREST, nil))
	for _, unwanted := range []string{"orders", "health"} {
		if slices.Contains(got, unwanted) {
			t.Errorf("drafts = %v, did not want %q", got, unwanted)
		}
	}
}

func TestDetect_ReadsItemsPathFromTheResponse(t *testing.T) {
	r := draftFor(t, drafts(t, specDetectREST, nil), "customers").Resource
	if r.ItemsPath != "/items" {
		t.Errorf("ItemsPath = %q, want /items", r.ItemsPath)
	}
}

func TestDetect_IDFromAPropertyNamedIDIsNotAGuess(t *testing.T) {
	d := draftFor(t, drafts(t, specDetectREST, nil), "customers")
	if d.Resource.IDPath != "/id" {
		t.Errorf("IDPath = %q, want /id", d.Resource.IDPath)
	}
	if slices.Contains(d.Guessed, "id_path") {
		t.Errorf("Guessed = %v, want id_path derived", d.Guessed)
	}
}

func TestDetect_LabelPathIsAlwaysAGuess(t *testing.T) {
	d := draftFor(t, drafts(t, specDetectREST, nil), "customers")
	if d.Resource.LabelPath != "/name" {
		t.Errorf("LabelPath = %q, want /name", d.Resource.LabelPath)
	}
	if !slices.Contains(d.Guessed, "label_path") {
		t.Errorf("Guessed = %v, want label_path flagged", d.Guessed)
	}
}

func TestDetect_FieldsAreTheItemScalars(t *testing.T) {
	r := draftFor(t, drafts(t, specDetectREST, nil), "customers").Resource
	var keys, pointers []string
	for _, f := range r.Fields {
		keys = append(keys, f.Key)
		pointers = append(pointers, f.Pointer)
	}
	wantKeys := []string{"address_city", "credit", "id", "name"}
	wantPointers := []string{"/address/city", "/credit", "/id", "/name"}
	if !reflect.DeepEqual(keys, wantKeys) {
		t.Errorf("field keys = %v, want %v", keys, wantKeys)
	}
	if !reflect.DeepEqual(pointers, wantPointers) {
		t.Errorf("field pointers = %v, want %v", pointers, wantPointers)
	}
	if r.Fields[1].Type != "number" {
		t.Errorf("credit type = %q, want number", r.Fields[1].Type)
	}
}

func TestDetect_SearchAndPagingComeFromTheListParams(t *testing.T) {
	d := draftFor(t, drafts(t, specDetectREST, nil), "customers")
	if d.Resource.SearchParam != "q" {
		t.Errorf("SearchParam = %q, want q", d.Resource.SearchParam)
	}
	want := Pagination{Style: PageOffset, LimitParam: "limit", OffsetParam: "offset"}
	if d.Resource.Pagination != want {
		t.Errorf("Pagination = %+v, want %+v", d.Resource.Pagination, want)
	}
	for _, attr := range []string{"search_param", "pagination"} {
		if !slices.Contains(d.Guessed, attr) {
			t.Errorf("Guessed = %v, want %q flagged", d.Guessed, attr)
		}
	}
}

func TestDetect_DraftsValidateAgainstTheirSpec(t *testing.T) {
	cat := mustParse(t, specDetectREST)
	c := &Connection{ID: "crm", Name: "CRM", SpecFile: "crm.json"}
	for _, d := range DetectResources(cat, c) {
		c.Resources = append(c.Resources, d.Resource)
	}
	if errs := Validate(c, cat); len(errs) > 0 {
		t.Fatalf("Validate = %+v, want a clean draft", errs)
	}
}

func TestDetect_SkipsOperationsAlreadyBound(t *testing.T) {
	c := &Connection{Resources: []Resource{{Key: "klanten", List: OpRef{Operation: "listCustomers"}}}}
	if got := drafts(t, specDetectREST, c); len(got) != 0 {
		t.Fatalf("drafts = %v, want nothing left to propose", draftKeys(got))
	}
}

func TestDetect_KeyCollisionGetsASuffix(t *testing.T) {
	c := &Connection{Resources: []Resource{{Key: "customers", List: OpRef{Operation: "createCustomer"}}}}
	got := drafts(t, specDetectREST, c)
	if keys := draftKeys(got); !reflect.DeepEqual(keys, []string{"customers-2"}) {
		t.Fatalf("drafts = %v, want customers-2", keys)
	}
}

func TestDetect_IsDeterministic(t *testing.T) {
	cat := mustParse(t, specDetectREST)
	if !reflect.DeepEqual(DetectResources(cat, nil), DetectResources(cat, nil)) {
		t.Fatal("two runs over the same catalog disagree")
	}
}

const specDetectPathKey = `{
  "openapi": "3.0.3",
  "info": {"title": "Reg", "version": "1.0"},
  "servers": [{"url": "https://reg.example.com"}],
  "paths": {
    "/providers": {
      "get": {"operationId": "getProviders", "responses": {"200": {"description": "ok",
        "content": {"application/json": {"schema": {"type": "array", "items": {
          "type": "object", "properties": {
            "providerId": {"type": "string"},
            "title": {"type": "string"}}}}}}}}}
    },
    "/providers/{providerId}": {
      "get": {"operationId": "getProvider", "parameters": [
        {"name": "providerId", "in": "path", "required": true, "schema": {"type": "string"}}],
        "responses": {"200": {"description": "ok", "content": {"application/json": {"schema": {
          "type": "object", "properties": {"providerId": {"type": "string"}}}}}}}}
    }
  }
}`

func TestDetect_IDFromTheEntityPathParameter(t *testing.T) {
	d := draftFor(t, drafts(t, specDetectPathKey, nil), "providers")
	if d.Resource.IDPath != "/providerId" {
		t.Errorf("IDPath = %q, want /providerId", d.Resource.IDPath)
	}
	if slices.Contains(d.Guessed, "id_path") {
		t.Errorf("Guessed = %v, want the path parameter treated as derived", d.Guessed)
	}
	if d.Resource.LabelPath != "/title" {
		t.Errorf("LabelPath = %q, want /title", d.Resource.LabelPath)
	}
	if d.Resource.ItemsPath != "" {
		t.Errorf("ItemsPath = %q, want empty for a root array", d.Resource.ItemsPath)
	}
	if got := d.Resource.Pagination.Style; got != "" && got != PageNone {
		t.Errorf("Pagination style = %q, want none when the spec declares no paging params", got)
	}
}

const specDetectOData = `{
  "openapi": "3.0.3",
  "info": {"title": "Northwind", "version": "1.0"},
  "servers": [{"url": "https://services.odata.org/V4/Northwind/Northwind.svc"}],
  "paths": {
    "/Customers": {
      "get": {"operationId": "listCustomers", "responses": {"200": {"description": "ok",
        "content": {"application/json": {"schema": {"type": "object", "properties": {
          "value": {"type": "array", "items": {"type": "object", "properties": {
            "CustomerID": {"type": "string"},
            "CompanyName": {"type": "string"}}}}}}}}}}}
    },
    "/Customers({CustomerID})": {
      "get": {"operationId": "getCustomer", "parameters": [
        {"name": "CustomerID", "in": "path", "required": true, "schema": {"type": "string"}}],
        "responses": {"200": {"description": "ok", "content": {"application/json": {"schema": {
          "type": "object", "properties": {"CustomerID": {"type": "string"}}}}}}}}
    }
  }
}`

func TestDetect_ODataEntityPathAndDialectDefaults(t *testing.T) {
	got := DetectResources(mustParse(t, specDetectOData), &Connection{Dialect: DialectOData})
	d := draftFor(t, got, "customers")
	r := d.Resource
	if r.Get.Operation != "getCustomer" {
		t.Errorf("Get = %q, want getCustomer for a parenthesised key path", r.Get.Operation)
	}
	if r.IDPath != "/CustomerID" {
		t.Errorf("IDPath = %q, want /CustomerID", r.IDPath)
	}
	if r.ItemsPath != "/value" {
		t.Errorf("ItemsPath = %q, want /value", r.ItemsPath)
	}
	if r.Pagination.Style != PageLink || r.Pagination.LimitParam != "$top" {
		t.Errorf("Pagination = %+v, want link paging with $top", r.Pagination)
	}
	if r.SelectParam != "$select" {
		t.Errorf("SelectParam = %q, want $select", r.SelectParam)
	}
	if r.SearchParam != "$filter" || r.SearchTemplate != "contains(CompanyName,'{q}')" {
		t.Errorf("search = %q / %q, want a $filter contains template", r.SearchParam, r.SearchTemplate)
	}
	if !slices.Contains(d.Guessed, "search_template") {
		t.Errorf("Guessed = %v, want search_template flagged", d.Guessed)
	}
}

func TestDetect_ODataDraftsValidate(t *testing.T) {
	cat := mustParse(t, specDetectOData)
	c := &Connection{ID: "nw", Name: "Northwind", SpecFile: "nw.json", Dialect: DialectOData}
	for _, d := range DetectResources(cat, c) {
		c.Resources = append(c.Resources, d.Resource)
	}
	if errs := Validate(c, cat); len(errs) > 0 {
		t.Fatalf("Validate = %+v, want a clean draft", errs)
	}
}

func TestDetect_UndescribedResponseStillProposesThePair(t *testing.T) {
	got := drafts(t, specV3JSON, nil)
	d := draftFor(t, got, "customers")
	if d.Resource.List.Operation != "listCustomers" || d.Resource.Get.Operation != "getCustomer" {
		t.Fatalf("bindings = %+v, want the structural pair", d.Resource)
	}
	if d.Resource.IDPath != "/customerId" {
		t.Errorf("IDPath = %q, want the path parameter name as the fallback", d.Resource.IDPath)
	}
	for _, attr := range []string{"id_path", "label_path"} {
		if !slices.Contains(d.Guessed, attr) {
			t.Errorf("Guessed = %v, want %q flagged when the spec describes no response", d.Guessed, attr)
		}
	}
}

func TestDetect_NilCatalogYieldsNothing(t *testing.T) {
	if got := DetectResources(nil, nil); got != nil {
		t.Fatalf("DetectResources(nil) = %v, want nil", got)
	}
}

func TestDetect_ReportsWhyNothingWasProposed(t *testing.T) {
	cat := mustParse(t, specDetectREST)

	// Bound and unbindable are opposite answers, and an empty proposal list
	// says neither on its own.
	fresh := Detect(cat, nil)
	if len(fresh.Drafts) != 1 || fresh.Bound != 0 {
		t.Fatalf("fresh = %+v, want one draft and nothing bound", fresh)
	}
	if fresh.NoCollection != 1 {
		t.Errorf("NoCollection = %d, want the health operation counted", fresh.NoCollection)
	}

	done := Detect(cat, &Connection{Resources: []Resource{
		{Key: "klanten", List: OpRef{Operation: "listCustomers"}},
	}})
	if len(done.Drafts) != 0 || done.Bound != 1 {
		t.Fatalf("done = %+v, want nothing proposed and one bound", done)
	}
}

func TestDetect_EmptyDetectionCarriesAnEmptySlice(t *testing.T) {
	// The frontend renders drafts directly; a nil slice crosses the boundary
	// as null and would need a guard at every use.
	got := Detect(nil, nil)
	if got.Drafts == nil {
		t.Fatal("Drafts = nil, want an empty slice")
	}
}

const specDetectRegistry = `{
  "openapi": "3.0.3",
  "info": {"title": "Registry", "version": "1.0"},
  "servers": [{"url": "https://registry.example.com"}],
  "paths": {
    "/list.json": {
      "get": {"operationId": "listAPIs", "responses": {"200": {"description": "ok",
        "content": {"application/json": {"schema": {"type": "object",
          "additionalProperties": {"type": "object", "properties": {
            "added": {"type": "string"},
            "title": {"type": "string"}}}}}}}}}
    },
    "/providers.json": {
      "get": {"operationId": "getProviders", "responses": {"200": {"description": "ok",
        "content": {"application/json": {"schema": {"type": "object", "properties": {
          "data": {"type": "array", "items": {"type": "string"}}}}}}}}}
    },
    "/metrics.json": {
      "get": {"operationId": "getMetrics", "responses": {"200": {"description": "ok",
        "content": {"application/json": {"schema": {"type": "object", "properties": {
          "numAPIs": {"type": "integer"}}}}}}}}
    }
  }
}`

func TestDetect_KeyedCollectionNeedsNoIDPointer(t *testing.T) {
	d := draftFor(t, drafts(t, specDetectRegistry, nil), "list")
	r := d.Resource
	if r.ItemsMode != ItemsMap {
		t.Fatalf("ItemsMode = %q, want %q", r.ItemsMode, ItemsMap)
	}
	if r.IDPath != "" {
		t.Errorf("IDPath = %q, want empty: the key is the id", r.IDPath)
	}
	if slices.Contains(d.Guessed, "id_path") {
		t.Errorf("Guessed = %v, want the key treated as derived", d.Guessed)
	}
	if r.LabelPath != "/title" {
		t.Errorf("LabelPath = %q, want the value's label property", r.LabelPath)
	}
}

func TestDetect_ScalarArrayIsItsOwnIDAndLabel(t *testing.T) {
	d := draftFor(t, drafts(t, specDetectRegistry, nil), "providers")
	r := d.Resource
	if r.ItemsPath != "/data" || r.ItemsMode != "" {
		t.Fatalf("container = %q/%q, want the sole array property in array mode", r.ItemsPath, r.ItemsMode)
	}
	if r.IDPath != "" || r.LabelPath != "" {
		t.Errorf("pointers = %q/%q, want none: the value is the record", r.IDPath, r.LabelPath)
	}
	if len(r.Fields) != 0 {
		t.Errorf("Fields = %+v, want none for plain values", r.Fields)
	}
	if len(d.Guessed) != 0 {
		t.Errorf("Guessed = %v, want nothing to check", d.Guessed)
	}
}

func TestDetect_KeysDropAPathExtension(t *testing.T) {
	keys := draftKeys(drafts(t, specDetectRegistry, nil))
	if !slices.Contains(keys, "list") || !slices.Contains(keys, "providers") {
		t.Fatalf("keys = %v, want the .json suffix off the key", keys)
	}
	if slices.Contains(keys, "metrics") {
		t.Errorf("keys = %v, want a single counters object left alone", keys)
	}
}

func TestDetect_RegistryDraftsValidate(t *testing.T) {
	cat := mustParse(t, specDetectRegistry)
	c := &Connection{ID: "reg", Name: "Registry", SpecFile: "reg.json"}
	for _, d := range DetectResources(cat, c) {
		c.Resources = append(c.Resources, d.Resource)
	}
	if errs := Validate(c, cat); len(errs) > 0 {
		t.Fatalf("Validate = %+v, want a clean draft", errs)
	}
}
