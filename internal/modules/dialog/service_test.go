package dialog

import "testing"

// The dialog module is a thin facade over Wails' native pickers, which
// require a running Wails application to drive. Functional behaviour
// is therefore verified by hand at the Vue layer; what these tests
// pin down is the small piece of pure-Go shape (constructor, types)
// that can change without anyone noticing in manual QA.

func TestNewService_ReturnsNonNil(t *testing.T) {
	t.Parallel()
	if NewService() == nil {
		t.Fatalf("NewService returned nil")
	}
}

func TestFileFilter_StructShape(t *testing.T) {
	t.Parallel()
	f := FileFilter{DisplayName: "JSON Files", Pattern: "*.json"}
	if f.DisplayName != "JSON Files" || f.Pattern != "*.json" {
		t.Fatalf("FileFilter fields not addressable: %+v", f)
	}
}

// TestServiceMethods_AreCallable just exercises the function-pointer
// shapes so a refactor that drops or renames a method shows up in the
// build. Calling them would require a Wails app — that's manual QA.
func TestServiceMethods_AreCallable(t *testing.T) {
	t.Parallel()
	s := NewService()
	_ = s.ChooseFile
	_ = s.ChooseSaveFile
	_ = s.ChooseDirectory
}
