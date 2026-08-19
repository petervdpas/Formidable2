package connection

import (
	"net/http"
	"testing"
	"time"
)

var snapAt = time.Date(2026, 8, 18, 9, 30, 0, 0, time.UTC)

func TestSnapshot_CarriesIDLabelAndStamp(t *testing.T) {
	got := Snapshot(Item{ID: "cust-42", Label: "Acme BV"}, nil, snapAt)
	if got["id"] != "cust-42" || got["label"] != "Acme BV" {
		t.Errorf("identity lost: %#v", got)
	}
	if got["fetched"] != "2026-08-18T09:30:00Z" {
		t.Errorf("fetched = %#v, want RFC 3339 UTC", got["fetched"])
	}
}

// No declared projection means the field stores identity only.
func TestSnapshot_NoKeysKeepsNoFields(t *testing.T) {
	item := Item{ID: "a", Fields: map[string]string{"city": "Utrecht"}}
	fields, _ := Snapshot(item, nil, snapAt)["fields"].(map[string]any)
	if len(fields) != 0 {
		t.Errorf("fields = %#v, want empty", fields)
	}
}

func TestSnapshot_KeepsOnlyDeclaredKeys(t *testing.T) {
	item := Item{ID: "a", Fields: map[string]string{
		"city": "Utrecht", "country": "NL", "secret": "nope",
	}}
	fields, _ := Snapshot(item, []string{"city", "country"}, snapAt)["fields"].(map[string]any)
	if len(fields) != 2 || fields["city"] != "Utrecht" || fields["country"] != "NL" {
		t.Errorf("projection wrong: %#v", fields)
	}
	if _, leaked := fields["secret"]; leaked {
		t.Error("undeclared field leaked into the snapshot")
	}
}

// A key the remote did not return is absent, not blank: a consumer can then tell
// "the service stopped sending this" from "the value is an empty string".
func TestSnapshot_MissingKeyIsAbsentNotBlank(t *testing.T) {
	item := Item{ID: "a", Fields: map[string]string{"city": "Utrecht"}}
	fields, _ := Snapshot(item, []string{"city", "country"}, snapAt)["fields"].(map[string]any)
	if _, has := fields["country"]; has {
		t.Errorf("absent key materialised: %#v", fields)
	}
}

// A remote that returns an empty string for a declared key is keeping it.
func TestSnapshot_EmptyValueIsKept(t *testing.T) {
	item := Item{ID: "a", Fields: map[string]string{"city": ""}}
	fields, _ := Snapshot(item, []string{"city"}, snapAt)["fields"].(map[string]any)
	if v, has := fields["city"]; !has || v != "" {
		t.Errorf("empty value dropped: %#v", fields)
	}
}

func TestSnapshot_NoIDIsNotAPick(t *testing.T) {
	if got := Snapshot(Item{Label: "orphan"}, nil, snapAt); got != nil {
		t.Errorf("got %#v, want nil", got)
	}
}

// The key names are the on-disk contract, shared with storage.coerceAPIClientRef.
func TestSnapshot_ShapeIsTheStorageContract(t *testing.T) {
	got := Snapshot(Item{ID: "a"}, nil, snapAt)
	if len(got) != 4 {
		t.Fatalf("snapshot has %d keys, want exactly id/label/fields/fetched: %#v", len(got), got)
	}
	for _, k := range []string{"id", "label", "fields", "fetched"} {
		if _, has := got[k]; !has {
			t.Errorf("missing %q: %#v", k, got)
		}
	}
}

// The whole point of the field: a failed refresh keeps the good copy.
func TestRefresh_FailedFetchKeepsTheStoredSnapshot(t *testing.T) {
	stored := Snapshot(Item{ID: "a", Label: "Acme BV"}, nil, snapAt)
	got := Refresh(stored, nil, nil, snapAt)
	if got["label"] != "Acme BV" {
		t.Errorf("stored snapshot lost on a failed refresh: %#v", got)
	}
}

func TestRefresh_SuccessReplacesTheSnapshot(t *testing.T) {
	stored := Snapshot(Item{ID: "a", Label: "Old"}, nil, snapAt)
	later := snapAt.Add(time.Hour)
	got := Refresh(stored, &Item{ID: "a", Label: "New"}, nil, later)
	if got["label"] != "New" {
		t.Errorf("label = %#v, want New", got["label"])
	}
	if got["fetched"] != "2026-08-18T10:30:00Z" {
		t.Errorf("fetched not restamped: %#v", got["fetched"])
	}
}

// ── Service facade ────────────────────────────────────────────────────

const oneCustomer = `{"id":"c-1","name":"Acme BV","email":"hi@acme.example",
  "address":{"city":"Utrecht"},"vip":true}`

func snapshotService(t *testing.T, handler http.HandlerFunc) *Service {
	t.Helper()
	iv, _, _ := shopFixture(t, handler)
	return NewService(iv.mgr, iv, newMemKeys())
}

func TestService_FetchSnapshotProjectsSelectedFields(t *testing.T) {
	s := snapshotService(t, jsonHandler(200, oneCustomer))
	got, err := s.FetchSnapshot(FetchRequest{
		Connection: "shop", Resource: "customers", ID: "c-1",
		Select: []string{"city", "email"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got["id"] != "c-1" || got["label"] != "Acme BV" {
		t.Errorf("identity wrong: %#v", got)
	}
	fields, _ := got["fields"].(map[string]any)
	if fields["city"] != "Utrecht" || fields["email"] != "hi@acme.example" {
		t.Errorf("projection wrong: %#v", fields)
	}
	if _, leaked := fields["vip"]; leaked {
		t.Errorf("unselected field stored: %#v", fields)
	}
	if got["fetched"] == "" {
		t.Error("snapshot is not stamped")
	}
}

// A failing remote returns the error and no snapshot, so the caller keeps
// whatever is already on disk rather than writing a blank over it.
func TestService_FetchSnapshotFailsWithoutASnapshot(t *testing.T) {
	s := snapshotService(t, jsonHandler(500, `{"error":"boom"}`))
	got, err := s.FetchSnapshot(FetchRequest{
		Connection: "shop", Resource: "customers", ID: "c-1",
	})
	if err == nil {
		t.Fatal("expected an error")
	}
	if got != nil {
		t.Errorf("got %#v, want no snapshot on failure", got)
	}
}
