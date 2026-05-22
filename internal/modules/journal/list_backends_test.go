package journal

import "testing"

func TestService_ListSyncBackends_ReturnsKnownInOrder(t *testing.T) {
	svc := NewService(nil)
	got := svc.ListSyncBackends()
	want := []string{"git", "gigot"}
	if len(got) != len(want) {
		t.Fatalf("len = %d, want %d (got %+v)", len(got), len(want), got)
	}
	for i, w := range want {
		if got[i] != w {
			t.Errorf("got[%d] = %q, want %q", i, got[i], w)
		}
	}
}

func TestService_ListSyncBackends_ReturnsCopy(t *testing.T) {
	svc := NewService(nil)
	first := svc.ListSyncBackends()
	if len(first) == 0 {
		t.Fatalf("empty")
	}
	first[0] = "MUTATED"

	second := svc.ListSyncBackends()
	if second[0] == "MUTATED" {
		t.Errorf("caller mutation leaked into internal slice")
	}
}

func TestService_ListSyncBackends_MatchesKnownBackendsMap(t *testing.T) {
	// Guards against the two registries drifting - if you add an entry
	// to orderedSyncBackends, knownBackends MUST accept it (otherwise
	// the journal silently drops entries from that backend on disk).
	for _, b := range orderedSyncBackends {
		if !knownBackends[b] {
			t.Errorf("orderedSyncBackends contains %q, but knownBackends does not accept it", b)
		}
	}
}
