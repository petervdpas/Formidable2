package i18n

import "testing"

func TestListLocales_SortedWithEndonyms(t *testing.T) {
	m, err := NewManager(nil)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	got := m.ListLocales()
	if len(got) < 2 {
		t.Fatalf("ListLocales returned %d entries; want >= 2 (en, nl)", len(got))
	}
	if got[0].Code > got[len(got)-1].Code {
		t.Errorf("ListLocales not sorted: %+v", got)
	}
	have := map[string]string{}
	for _, l := range got {
		have[l.Code] = l.Endonym
	}
	if have["en"] != "English" {
		t.Errorf("en endonym = %q, want %q", have["en"], "English")
	}
	if have["nl"] != "Nederlands" {
		t.Errorf("nl endonym = %q, want %q", have["nl"], "Nederlands")
	}
}

func TestListLocales_FallsBackToCodeWhenEndonymMissing(t *testing.T) {
	m := &Manager{
		bundles: map[string]map[string]any{
			"xx": {"some.other.key": "value"},
		},
	}
	got := m.ListLocales()
	if len(got) != 1 || got[0].Code != "xx" {
		t.Fatalf("ListLocales = %+v", got)
	}
	if got[0].Endonym != "xx" {
		t.Errorf("missing-endonym fallback = %q, want %q", got[0].Endonym, "xx")
	}
}

func TestListLocales_IgnoresNonStringEndonym(t *testing.T) {
	m := &Manager{
		bundles: map[string]map[string]any{
			"xx": {"language.endonym": 42},
		},
	}
	got := m.ListLocales()
	if got[0].Endonym != "xx" {
		t.Errorf("non-string endonym should fall back to code; got %q", got[0].Endonym)
	}
}

func TestService_ListLocales_DelegatesToManager(t *testing.T) {
	m, err := NewManager(nil)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	svc := NewService(m)
	got := svc.ListLocales()
	if len(got) < 2 {
		t.Fatalf("service.ListLocales returned %d entries; want >= 2", len(got))
	}
}
