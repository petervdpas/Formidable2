package connection

import (
	"encoding/base64"
	"errors"
	"strings"
	"testing"
)

// memKeys is a stand-in secret store. The real one is a vault resolver; this
// keeps the facade tests free of crypto and of a master password.
type memKeys struct {
	values map[string]string
	locked bool
}

func newMemKeys() *memKeys { return &memKeys{values: map[string]string{}} }

func (k *memKeys) Store(id, secret string) error {
	if k.locked {
		return errors.New("locked")
	}
	k.values[id] = secret
	return nil
}

func (k *memKeys) Forget(id string) error {
	if k.locked {
		return errors.New("locked")
	}
	delete(k.values, id)
	return nil
}

// Has needs the store open, mirroring the vault: identity lives inside the
// ciphertext, so a locked store cannot answer.
func (k *memKeys) Has(id string) bool {
	if k.locked {
		return false
	}
	_, ok := k.values[id]
	return ok
}

func (k *memKeys) Secret(id string) (string, error) {
	if k.locked {
		return "", errors.New("locked")
	}
	v, ok := k.values[id]
	if !ok {
		return "", errors.New("not found")
	}
	return v, nil
}

func newService(t *testing.T) (*Service, *memKeys) {
	t.Helper()
	mgr := NewManager(newMemFS(), nil)
	if _, err := mgr.ImportSpec("crm-prod", []byte(specV3JSON)); err != nil {
		t.Fatal(err)
	}
	keys := newMemKeys()
	return NewService(mgr, NewInvoker(mgr, keys), keys), keys
}

func TestService_SaveListGet(t *testing.T) {
	s, _ := newService(t)
	c := validConn()
	if err := s.SaveClient(c); err != nil {
		t.Fatal(err)
	}

	list, err := s.ListClients()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].ID != "crm-prod" || !list[0].OK {
		t.Fatalf("list = %+v", list)
	}

	detail, err := s.GetClient("crm-prod")
	if err != nil {
		t.Fatal(err)
	}
	if detail.Client.Name != "CRM Production" {
		t.Errorf("client = %+v", detail.Client)
	}
	if detail.Catalog == nil || len(detail.Catalog.Operations) == 0 {
		t.Error("detail must carry the catalog for the operation pickers")
	}
	if detail.HasCredential {
		t.Error("no credential stored yet")
	}
}

func TestService_SaveRefusesABrokenBinding(t *testing.T) {
	s, _ := newService(t)
	c := validConn()
	c.Resources[0].List.Operation = "noSuchOperation"

	err := s.SaveClient(c)
	var vfe *ValidationFailedError
	if !errors.As(err, &vfe) || !hasType(vfe.Errors, "unknown-operation") {
		t.Fatalf("err = %v, want a validation failure", err)
	}
}

func TestService_ImportSpecAcceptsBase64AndDataURI(t *testing.T) {
	s, _ := newService(t)
	encoded := base64.StdEncoding.EncodeToString([]byte(specV3YAML))

	info, err := s.ImportSpec("tiny", encoded)
	if err != nil {
		t.Fatal(err)
	}
	if info.File != "tiny.yaml" || info.Catalog.Title != "Tiny" {
		t.Fatalf("info = %+v", info)
	}

	info, err = s.ImportSpec("tiny2", "data:application/yaml;base64,"+encoded)
	if err != nil {
		t.Fatalf("a data URI must be accepted unchanged: %v", err)
	}
	if info.Catalog.Title != "Tiny" {
		t.Fatalf("info = %+v", info)
	}
}

func TestService_ImportSpecRejectsGarbage(t *testing.T) {
	s, _ := newService(t)
	if _, err := s.ImportSpec("bad", "not base64 at all!!"); err == nil {
		t.Fatal("want an error")
	}
	if _, err := s.ImportSpec("bad", base64.StdEncoding.EncodeToString([]byte("hello"))); err == nil {
		t.Fatal("a decodable body that is not a spec must still fail")
	}
}

func TestService_ValidateAnUnsavedDraft(t *testing.T) {
	s, _ := newService(t)
	draft := validConn()
	draft.ID = "never-saved"
	draft.Resources[0].SearchParam = "nope"

	errs, err := s.ValidateClient(draft)
	if err != nil {
		t.Fatalf("a draft must validate against the spec it names: %v", err)
	}
	if !hasType(errs, "unknown-param") {
		t.Fatalf("errs = %s", types(errs))
	}
}

func TestService_ValidateReturnsAnEmptySliceNotNull(t *testing.T) {
	s, _ := newService(t)
	errs, err := s.ValidateClient(validConn())
	if err != nil {
		t.Fatal(err)
	}
	if errs == nil {
		t.Fatal("a clean result must marshal as [] rather than null")
	}
	if len(errs) != 0 {
		t.Fatalf("errs = %s", types(errs))
	}
}

func TestService_CredentialsAreWriteOnly(t *testing.T) {
	s, _ := newService(t)
	if err := s.SaveClient(validConn()); err != nil {
		t.Fatal(err)
	}

	if err := s.SetCredential("crm-prod", "s3cret"); err != nil {
		t.Fatal(err)
	}
	if !s.HasCredential("crm-prod") {
		t.Fatal("HasCredential is false after SetCredential")
	}
	detail, err := s.GetClient("crm-prod")
	if err != nil {
		t.Fatal(err)
	}
	if !detail.HasCredential {
		t.Error("detail must report the credential as present")
	}

	if err := s.ForgetCredential("crm-prod"); err != nil {
		t.Fatal(err)
	}
	if s.HasCredential("crm-prod") {
		t.Fatal("still present after ForgetCredential")
	}
}

// A locked store cannot say whether a credential exists, so the panel must
// treat "locked" as unknown rather than as "not configured".
func TestService_HasCredentialIsFalseWhileTheStoreIsLocked(t *testing.T) {
	s, keys := newService(t)
	if err := s.SetCredential("crm-prod", "s3cret"); err != nil {
		t.Fatal(err)
	}
	keys.locked = true

	if s.HasCredential("crm-prod") {
		t.Fatal("a locked store must not claim to know what it holds")
	}

	keys.locked = false
	if !s.HasCredential("crm-prod") {
		t.Fatal("the credential should reappear once the store is open")
	}
}

func TestService_DeleteAlsoForgetsTheCredential(t *testing.T) {
	s, _ := newService(t)
	if err := s.SaveClient(validConn()); err != nil {
		t.Fatal(err)
	}
	if err := s.SetCredential("crm-prod", "s3cret"); err != nil {
		t.Fatal(err)
	}

	if err := s.DeleteClient("crm-prod"); err != nil {
		t.Fatal(err)
	}
	if s.HasCredential("crm-prod") {
		t.Fatal("a credential outlived the client it belonged to")
	}
}

func TestService_DeleteSucceedsEvenWhenTheStoreIsLocked(t *testing.T) {
	s, keys := newService(t)
	if err := s.SaveClient(validConn()); err != nil {
		t.Fatal(err)
	}
	keys.locked = true

	if err := s.DeleteClient("crm-prod"); err != nil {
		t.Fatalf("a locked secret store must not block deleting a client: %v", err)
	}
}

func TestService_EnumerationsForTheEditor(t *testing.T) {
	s, _ := newService(t)
	if strings.Join(s.ListDialects(), ",") != "rest,odata" {
		t.Errorf("dialects = %v", s.ListDialects())
	}
	if strings.Join(s.ListKeyStyles(), ",") != "raw,quoted,typed" {
		t.Errorf("key styles = %v", s.ListKeyStyles())
	}
	if strings.Join(s.ListPaginationStyles(), ",") != "none,offset,page,cursor,link" {
		t.Errorf("pagination styles = %v", s.ListPaginationStyles())
	}
}

func TestService_GetCatalogAndReload(t *testing.T) {
	s, _ := newService(t)
	if err := s.SaveClient(validConn()); err != nil {
		t.Fatal(err)
	}
	cat, err := s.GetCatalog("crm-prod")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := cat.Op("listCustomers"); !ok {
		t.Error("catalog is missing an operation")
	}
	s.ReloadSpecs()
	if _, err := s.GetCatalog("crm-prod"); err != nil {
		t.Fatalf("catalog unavailable after a reload: %v", err)
	}
}

func TestService_InvokeWithoutAnInvoker(t *testing.T) {
	mgr := NewManager(newMemFS(), nil)
	s := NewService(mgr, nil, nil)

	if _, err := s.ListItems(ListRequest{Connection: "x", Resource: "y"}); err == nil {
		t.Error("want an error rather than a panic")
	}
	if _, err := s.FetchItem(FetchRequest{Connection: "x", Resource: "y", ID: "1"}); err == nil {
		t.Error("want an error rather than a panic")
	}
	if err := s.SetCredential("x", "y"); err == nil {
		t.Error("want an error when no secret store is configured")
	}
	if s.HasCredential("x") {
		t.Error("no store means no credentials")
	}
}

func TestService_DetectResourcesReadsTheDraftsSpec(t *testing.T) {
	s, _ := newService(t)

	// Nothing saved yet: detection has to reach the spec through the file the
	// draft names, the same way ValidateClient does.
	got, err := s.DetectResources(Connection{ID: "crm-prod", Name: "CRM", SpecFile: "crm-prod.json"})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Drafts) != 1 || got.Drafts[0].Resource.Key != "customers" {
		t.Fatalf("drafts = %+v, want one customers proposal", got)
	}
	if got.Drafts[0].Resource.List.Operation != "listCustomers" {
		t.Errorf("list = %q, want listCustomers", got.Drafts[0].Resource.List.Operation)
	}
}

func TestService_DetectResourcesHonoursWhatTheDraftAlreadyBinds(t *testing.T) {
	s, _ := newService(t)

	got, err := s.DetectResources(validConn())
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Drafts) != 0 {
		t.Fatalf("drafts = %+v, want nothing left to propose", got)
	}
	if got.Bound != 1 {
		t.Errorf("Bound = %d, want the already-bound list operation counted", got.Bound)
	}
}

func TestService_DetectResourcesWithoutASpecFails(t *testing.T) {
	s, _ := newService(t)
	if _, err := s.DetectResources(Connection{ID: "nope", SpecFile: "missing.json"}); err == nil {
		t.Fatal("want an error when the spec cannot be read")
	}
}

func TestService_ListItemsModes(t *testing.T) {
	s, _ := newService(t)
	got := s.ListItemsModes()
	if len(got) != 2 || got[0] != ItemsArray || got[1] != ItemsMap {
		t.Fatalf("modes = %v, want array then map", got)
	}
}

func TestService_ListOperationsAnnotatesTheCatalog(t *testing.T) {
	s, _ := newService(t)

	got, err := s.ListOperations(validConn())
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Fatalf("listed %d operations, want all three in the document", len(got))
	}
	if info := infoFor(t, got, "listCustomers"); len(info.BoundBy) != 1 {
		t.Errorf("listCustomers bound by %+v, want the customers resource", info.BoundBy)
	}
	// Every method, not only the ones a resource can use.
	if info := infoFor(t, got, "createCustomer"); info.Operation.Method != "POST" {
		t.Errorf("createCustomer = %+v, want the POST listed too", info.Operation)
	}
}

func TestService_ListShapesCoversEveryShapeName(t *testing.T) {
	s, _ := newService(t)
	got := s.ListShapes()
	want := []string{ShapeRecords, ShapeKeyed, ShapeValues, ShapeKeyedValues, ShapeRecord, ShapeUnknown}
	if len(got) != len(want) {
		t.Fatalf("shapes = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("shapes = %v, want %v", got, want)
		}
	}
}

func TestService_TryOperationFormReadsTheDraft(t *testing.T) {
	s, _ := newService(t)

	form, err := s.TryOperationForm(validConn(), "listCustomers")
	if err != nil {
		t.Fatal(err)
	}
	if !form.Runnable || len(form.Params) == 0 {
		t.Fatalf("form = %+v, want a runnable form with parameters", form)
	}
	// The draft's own fixed params, not the saved file's.
	for _, p := range form.Params {
		if p.Name == "tenant" && p.Value != "acme" {
			t.Errorf("tenant = %+v, want the draft's fixed value", p)
		}
	}
}

func TestService_TryOperationNeedsAnInvoker(t *testing.T) {
	s := NewService(NewManager(newMemFS(), nil), nil, nil)
	if _, err := s.TryOperation(TryRequest{Connection: "crm-prod", Operation: "listCustomers"}); err == nil {
		t.Fatal("want an error when no invoker is configured")
	}
}
