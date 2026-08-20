package connection

import (
	"reflect"
	"testing"
)

func infoFor(t *testing.T, list []OperationInfo, id string) OperationInfo {
	t.Helper()
	for _, i := range list {
		if i.Operation.ID == id {
			return i
		}
	}
	t.Fatalf("operation %q is not listed", id)
	return OperationInfo{}
}

func TestOperations_ShapePerResponse(t *testing.T) {
	list := Operations(mustParse(t, specMapShapes), nil)
	want := map[string]string{
		"registry":   ShapeKeyed,
		"wrappedMap": ShapeKeyed,
		"flags":      ShapeKeyedValues,
		"names":      ShapeValues,
		"bare":       ShapeValues,
	}
	for id, shape := range want {
		if got := infoFor(t, list, id).Shape; got != shape {
			t.Errorf("%s shape = %q, want %q", id, got, shape)
		}
	}
}

func TestOperations_RecordAndUndeclaredShapes(t *testing.T) {
	list := Operations(mustParse(t, specShapes), nil)
	if got := infoFor(t, list, "single").Shape; got != ShapeRecord {
		t.Errorf("single shape = %q, want %q", got, ShapeRecord)
	}
	if got := infoFor(t, list, "silent").Shape; got != ShapeUnknown {
		t.Errorf("silent shape = %q, want %q", got, ShapeUnknown)
	}
	if got := infoFor(t, list, "rootArray").Shape; got != ShapeRecords {
		t.Errorf("rootArray shape = %q, want %q", got, ShapeRecords)
	}
}

func TestOperations_ListsEveryMethodNotJustGET(t *testing.T) {
	list := Operations(mustParse(t, specDetectREST), nil)
	if len(list) != 5 {
		t.Fatalf("listed %d operations, want every one in the document", len(list))
	}
	if info := infoFor(t, list, "createCustomer"); info.Operation.Method != "POST" {
		t.Errorf("createCustomer method = %q, want POST", info.Operation.Method)
	}
}

func TestOperations_ReportsWhoBindsWhat(t *testing.T) {
	c := &Connection{Resources: []Resource{{
		Key:  "customers",
		List: OpRef{Operation: "listCustomers"},
		Get:  OpRef{Operation: "getCustomer"},
	}}}
	list := Operations(mustParse(t, specDetectREST), c)

	got := infoFor(t, list, "listCustomers").BoundBy
	if !reflect.DeepEqual(got, []OperationBinding{{Resource: "customers", Role: RoleList}}) {
		t.Errorf("listCustomers bound by %+v", got)
	}
	got = infoFor(t, list, "getCustomer").BoundBy
	if !reflect.DeepEqual(got, []OperationBinding{{Resource: "customers", Role: RoleGet}}) {
		t.Errorf("getCustomer bound by %+v", got)
	}
	if got := infoFor(t, list, "createCustomer").BoundBy; got != nil {
		t.Errorf("createCustomer bound by %+v, want nothing", got)
	}
}

func TestOperations_OneOperationCanBackSeveralResources(t *testing.T) {
	// Reusing a list under two resources is legitimate: different field maps
	// over the same collection. The editor has to show both.
	c := &Connection{Resources: []Resource{
		{Key: "customers", List: OpRef{Operation: "listCustomers"}},
		{Key: "leads", List: OpRef{Operation: "listCustomers"}},
	}}
	got := infoFor(t, Operations(mustParse(t, specDetectREST), c), "listCustomers").BoundBy
	if len(got) != 2 || got[0].Resource != "customers" || got[1].Resource != "leads" {
		t.Fatalf("bound by %+v, want both resources in resource order", got)
	}
}

func TestOperations_CollectionFlagAgreesWithDetection(t *testing.T) {
	cat := mustParse(t, specDetectRegistry)
	proposed := map[string]bool{}
	for _, d := range DetectResources(cat, nil) {
		proposed[d.Resource.List.Operation] = true
	}
	for _, info := range Operations(cat, nil) {
		if proposed[info.Operation.ID] && !info.Collection {
			t.Errorf("%s was proposed but is not marked a collection", info.Operation.ID)
		}
	}
}

func TestOperations_NilCatalogYieldsAnEmptySlice(t *testing.T) {
	if got := Operations(nil, nil); got == nil || len(got) != 0 {
		t.Fatalf("Operations(nil) = %v, want an empty slice", got)
	}
}
