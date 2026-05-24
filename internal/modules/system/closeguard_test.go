package system

import "testing"

func TestCloseGuard_UnsavedChangesRoundTrip(t *testing.T) {
	t.Cleanup(func() {
		unsavedChanges.Store(false)
		allowClose.Store(false)
	})

	s := NewService(NewManager(t.TempDir(), nil))

	if UnsavedChanges() {
		t.Fatal("unsaved changes should start false")
	}

	s.SetUnsavedChanges(true)
	if !UnsavedChanges() {
		t.Fatal("SetUnsavedChanges(true) should flip the flag")
	}

	s.SetUnsavedChanges(false)
	if UnsavedChanges() {
		t.Fatal("SetUnsavedChanges(false) should clear the flag")
	}
}

func TestCloseGuard_AllowCloseStartsFalse(t *testing.T) {
	t.Cleanup(func() { allowClose.Store(false) })

	if AllowClose() {
		t.Fatal("allowClose should start false so the hook vetoes unsaved closes")
	}
}
