package connection

import "testing"

const specShapes = `{
  "openapi": "3.0.3",
  "info": {"title": "Shapes", "version": "1.0"},
  "paths": {
    "/rootarray": {"get": {"operationId": "rootArray", "responses": {"200": {"description": "ok",
      "content": {"application/json": {"schema": {"type": "array", "items": {"$ref": "#/components/schemas/Customer"}}}}}}}},
    "/wrapped": {"get": {"operationId": "wrapped", "responses": {"200": {"description": "ok",
      "content": {"application/json": {"schema": {"type": "object", "properties": {
        "total": {"type": "integer"},
        "items": {"type": "array", "items": {"$ref": "#/components/schemas/Customer"}}}}}}}}}},
    "/twoarrays": {"get": {"operationId": "twoArrays", "responses": {"200": {"description": "ok",
      "content": {"application/json": {"schema": {"type": "object", "properties": {
        "warnings": {"type": "array", "items": {"type": "object", "properties": {"code": {"type": "string"}}}},
        "value": {"type": "array", "items": {"$ref": "#/components/schemas/Customer"}}}}}}}}}},
    "/single": {"get": {"operationId": "single", "responses": {"200": {"description": "ok",
      "content": {"application/json": {"schema": {"$ref": "#/components/schemas/Customer"}}}}}}},
    "/inherited": {"get": {"operationId": "inherited", "responses": {"200": {"description": "ok",
      "content": {"application/json": {"schema": {"allOf": [
        {"$ref": "#/components/schemas/Customer"},
        {"type": "object", "properties": {"vip": {"type": "boolean"}}}]}}}}}}},
    "/deep": {"get": {"operationId": "deep", "responses": {"200": {"description": "ok",
      "content": {"application/json": {"schema": {"type": "object", "properties": {
        "a": {"type": "object", "properties": {
          "b": {"type": "object", "properties": {"c": {"type": "string"}}}}}}}}}}}}},
    "/cyclic": {"get": {"operationId": "cyclic", "responses": {"200": {"description": "ok",
      "content": {"application/json": {"schema": {"$ref": "#/components/schemas/Node"}}}}}}},
    "/silent": {"get": {"operationId": "silent", "responses": {"204": {"description": "gone"}}}},
    "/binary": {"get": {"operationId": "binary", "responses": {"200": {"description": "ok",
      "content": {"application/pdf": {"schema": {"type": "string"}}}}}}},
    "/patterned": {"get": {"operationId": "patterned", "responses": {"2XX": {"description": "ok",
      "content": {"application/json": {"schema": {"type": "array", "items": {"$ref": "#/components/schemas/Customer"}}}}}}}}
  },
  "components": {"schemas": {
    "Customer": {"type": "object", "properties": {
      "id": {"type": "string"},
      "name": {"type": "string"},
      "credit": {"type": "number"},
      "tags": {"type": "array", "items": {"type": "string"}},
      "address": {"type": "object", "properties": {"city": {"type": "string"}, "zip": {"type": "string"}}}}},
    "Node": {"type": "object", "properties": {
      "id": {"type": "string"},
      "parent": {"$ref": "#/components/schemas/Node"}}}
  }}
}`

func resultOf(t *testing.T, cat *Catalog, opID string) *Result {
	t.Helper()
	op, ok := cat.Op(opID)
	if !ok {
		t.Fatalf("operation %q missing from catalog", opID)
	}
	return op.Result
}

func pointers(r *Result) []string {
	if r == nil {
		return nil
	}
	out := make([]string, 0, len(r.Properties))
	for _, p := range r.Properties {
		out = append(out, p.Pointer)
	}
	return out
}

func hasPointer(r *Result, ptr string) bool {
	for _, p := range pointers(r) {
		if p == ptr {
			return true
		}
	}
	return false
}

func TestResult_RootArrayIsACollection(t *testing.T) {
	res := resultOf(t, mustParse(t, specShapes), "rootArray")
	if res == nil || !res.Collection {
		t.Fatalf("Result = %+v, want a collection", res)
	}
	if res.ItemsPath != "" {
		t.Errorf("ItemsPath = %q, want empty for a root array", res.ItemsPath)
	}
	if res.Ambiguous {
		t.Error("Ambiguous = true, want false for a root array")
	}
	if !hasPointer(res, "/name") {
		t.Errorf("properties = %v, want the item's own properties", pointers(res))
	}
}

func TestResult_SingleArrayPropertyIsTheItemsPath(t *testing.T) {
	res := resultOf(t, mustParse(t, specShapes), "wrapped")
	if res == nil || !res.Collection || res.ItemsPath != "/items" {
		t.Fatalf("Result = %+v, want a collection at /items", res)
	}
	if res.Ambiguous {
		t.Error("Ambiguous = true, want false when exactly one property is an array")
	}
}

func TestResult_SeveralArraysPickAKnownNameAndSayItGuessed(t *testing.T) {
	res := resultOf(t, mustParse(t, specShapes), "twoArrays")
	if res == nil || res.ItemsPath != "/value" {
		t.Fatalf("ItemsPath = %+v, want /value", res)
	}
	if !res.Ambiguous {
		t.Error("Ambiguous = false, want true when the choice was by name")
	}
}

func TestResult_ObjectResponseIsNotACollection(t *testing.T) {
	res := resultOf(t, mustParse(t, specShapes), "single")
	if res == nil || res.Collection {
		t.Fatalf("Result = %+v, want a non-collection", res)
	}
	if !hasPointer(res, "/id") {
		t.Errorf("properties = %v, want the object's properties", pointers(res))
	}
}

func TestResult_ScalarsOnlyAndOneLevelOfNesting(t *testing.T) {
	res := resultOf(t, mustParse(t, specShapes), "single")
	if !hasPointer(res, "/address/city") {
		t.Errorf("properties = %v, want a nested scalar", pointers(res))
	}
	if hasPointer(res, "/tags") || hasPointer(res, "/address") {
		t.Errorf("properties = %v, want no arrays and no container itself", pointers(res))
	}
}

func TestResult_PropertiesAreSortedAndStable(t *testing.T) {
	res := resultOf(t, mustParse(t, specShapes), "single")
	got := pointers(res)
	want := []string{"/address/city", "/address/zip", "/credit", "/id", "/name"}
	if len(got) != len(want) {
		t.Fatalf("properties = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("properties = %v, want %v", got, want)
		}
	}
}

func TestResult_NestingStopsAtTheDepthCap(t *testing.T) {
	res := resultOf(t, mustParse(t, specShapes), "deep")
	if hasPointer(res, "/a/b/c") {
		t.Errorf("properties = %v, want nothing past the depth cap", pointers(res))
	}
}

func TestResult_SelfReferentialSchemaTerminates(t *testing.T) {
	res := resultOf(t, mustParse(t, specShapes), "cyclic")
	if !hasPointer(res, "/id") {
		t.Errorf("properties = %v, want the scalar leaves", pointers(res))
	}
	if hasPointer(res, "/parent/parent/id") {
		t.Errorf("properties = %v, want the cycle cut", pointers(res))
	}
}

func TestResult_AllOfMembersAreMerged(t *testing.T) {
	res := resultOf(t, mustParse(t, specShapes), "inherited")
	if !hasPointer(res, "/name") || !hasPointer(res, "/vip") {
		t.Errorf("properties = %v, want both allOf members", pointers(res))
	}
}

func TestResult_NoBodyAndNoJSONYieldNothing(t *testing.T) {
	cat := mustParse(t, specShapes)
	if res := resultOf(t, cat, "silent"); res != nil {
		t.Errorf("silent Result = %+v, want nil", res)
	}
	if res := resultOf(t, cat, "binary"); res != nil {
		t.Errorf("binary Result = %+v, want nil for a non-JSON body", res)
	}
}

func TestResult_PatternedStatusIsRead(t *testing.T) {
	res := resultOf(t, mustParse(t, specShapes), "patterned")
	if res == nil || !res.Collection {
		t.Fatalf("Result = %+v, want the 2XX response read as a collection", res)
	}
}

func TestResult_Swagger2ResponseSchemaSurvivesConversion(t *testing.T) {
	const src = `{
  "swagger": "2.0",
  "info": {"title": "Legacy", "version": "1.0"},
  "host": "legacy.example.com",
  "paths": {"/orders": {"get": {"operationId": "listOrders", "responses": {"200": {
    "description": "ok",
    "schema": {"type": "array", "items": {"type": "object", "properties": {
      "id": {"type": "string"}, "reference": {"type": "string"}}}}}}}}}
}`
	res := resultOf(t, mustParse(t, src), "listOrders")
	if res == nil || !res.Collection {
		t.Fatalf("Result = %+v, want a collection", res)
	}
	if !hasPointer(res, "/reference") {
		t.Errorf("properties = %v, want the converted item properties", pointers(res))
	}
}

const specMapShapes = `{
  "openapi": "3.0.3",
  "info": {"title": "Maps", "version": "1.0"},
  "paths": {
    "/registry": {"get": {"operationId": "registry", "responses": {"200": {"description": "ok",
      "content": {"application/json": {"schema": {"$ref": "#/components/schemas/Entries"}}}}}}},
    "/wrapped": {"get": {"operationId": "wrappedMap", "responses": {"200": {"description": "ok",
      "content": {"application/json": {"schema": {"type": "object", "properties": {
        "entries": {"$ref": "#/components/schemas/Entries"}}}}}}}}},
    "/flags": {"get": {"operationId": "flags", "responses": {"200": {"description": "ok",
      "content": {"application/json": {"schema": {"type": "object",
        "additionalProperties": {"type": "boolean"}}}}}}}},
    "/names": {"get": {"operationId": "names", "responses": {"200": {"description": "ok",
      "content": {"application/json": {"schema": {"type": "object", "properties": {
        "data": {"type": "array", "items": {"type": "string"}}}}}}}}}},
    "/bare": {"get": {"operationId": "bare", "responses": {"200": {"description": "ok",
      "content": {"application/json": {"schema": {"type": "array", "items": {"type": "string"}}}}}}}}
  },
  "components": {"schemas": {
    "Entries": {"type": "object", "additionalProperties": {"type": "object", "properties": {
      "added": {"type": "string"},
      "preferred": {"type": "string"}}}}
  }}
}`

func TestResult_KeyedObjectIsACollection(t *testing.T) {
	res := resultOf(t, mustParse(t, specMapShapes), "registry")
	if res == nil || !res.Collection || res.ItemsMode != ItemsMap {
		t.Fatalf("Result = %+v, want a keyed collection", res)
	}
	if res.ItemsPath != "" {
		t.Errorf("ItemsPath = %q, want empty for a root map", res.ItemsPath)
	}
	if res.Scalar {
		t.Error("Scalar = true, want false for object values")
	}
	if !hasPointer(res, "/preferred") {
		t.Errorf("properties = %v, want the value's own properties", pointers(res))
	}
}

func TestResult_WrappedKeyedObject(t *testing.T) {
	res := resultOf(t, mustParse(t, specMapShapes), "wrappedMap")
	if res == nil || res.ItemsMode != ItemsMap || res.ItemsPath != "/entries" {
		t.Fatalf("Result = %+v, want a keyed collection at /entries", res)
	}
}

func TestResult_KeyedScalarValuesAreStillACollection(t *testing.T) {
	res := resultOf(t, mustParse(t, specMapShapes), "flags")
	if res == nil || !res.Collection || res.ItemsMode != ItemsMap || !res.Scalar {
		t.Fatalf("Result = %+v, want a keyed collection of scalars", res)
	}
	if len(res.Properties) != 0 {
		t.Errorf("properties = %v, want none for scalar values", pointers(res))
	}
}

func TestResult_SolePropertyHoldingAScalarArrayIsAWrapper(t *testing.T) {
	res := resultOf(t, mustParse(t, specMapShapes), "names")
	if res == nil || !res.Collection || res.ItemsPath != "/data" {
		t.Fatalf("Result = %+v, want a collection at /data", res)
	}
	if res.ItemsMode != ItemsArray || !res.Scalar {
		t.Errorf("Result = %+v, want a scalar array", res)
	}
}

func TestResult_RootScalarArrayIsACollection(t *testing.T) {
	res := resultOf(t, mustParse(t, specMapShapes), "bare")
	if res == nil || !res.Collection || !res.Scalar || res.ItemsPath != "" {
		t.Fatalf("Result = %+v, want a bare scalar collection", res)
	}
}

func TestResult_ARecordWithAScalarArrayStaysARecord(t *testing.T) {
	// Customer carries tags. Only a sole array property makes a wrapper, so a
	// record with several properties is never mistaken for one.
	res := resultOf(t, mustParse(t, specShapes), "single")
	if res.Collection {
		t.Fatalf("Result = %+v, want a record, not a collection", res)
	}
}
