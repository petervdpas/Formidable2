package formwidget

import (
	"errors"
	"testing"
)

func TestWidget_Validate_HappyPath(t *testing.T) {
	cases := []Widget{
		{ID: "bar1", Kind: KindProgressBar},
		{ID: "status_1", Kind: KindStatusMessage},
		{ID: "current-file", Kind: KindStatusMessage, Label: "Current file"},
		{ID: "p", Kind: KindProgressBar},
	}
	for _, w := range cases {
		if err := w.Validate(); err != nil {
			t.Errorf("Widget(%+v).Validate(): %v", w, err)
		}
	}
}

func TestWidget_Validate_EmptyID(t *testing.T) {
	w := Widget{ID: "", Kind: KindProgressBar}
	if err := w.Validate(); !errors.Is(err, ErrWidgetInvalid) {
		t.Errorf("err = %v, want ErrWidgetInvalid", err)
	}
}

func TestWidget_Validate_BadID(t *testing.T) {
	cases := []string{
		"With Space",
		"UpperCase",
		"-leading-dash",
		"trailing.dot",
		"has/slash",
	}
	for _, id := range cases {
		w := Widget{ID: id, Kind: KindProgressBar}
		if err := w.Validate(); !errors.Is(err, ErrWidgetInvalid) {
			t.Errorf("id %q: err = %v, want ErrWidgetInvalid", id, err)
		}
	}
}

func TestWidget_Validate_EmptyKind(t *testing.T) {
	w := Widget{ID: "x", Kind: ""}
	if err := w.Validate(); !errors.Is(err, ErrWidgetInvalid) {
		t.Errorf("err = %v, want ErrWidgetInvalid", err)
	}
}

func TestWidget_Validate_UnknownKind(t *testing.T) {
	w := Widget{ID: "x", Kind: "spinner"}
	if err := w.Validate(); !errors.Is(err, ErrWidgetInvalid) {
		t.Errorf("err = %v, want ErrWidgetInvalid", err)
	}
}

func TestValidateAll_DetectsDuplicateID(t *testing.T) {
	ws := []Widget{
		{ID: "bar", Kind: KindProgressBar},
		{ID: "msg", Kind: KindStatusMessage},
		{ID: "bar", Kind: KindStatusMessage},
	}
	if err := ValidateAll(ws); !errors.Is(err, ErrWidgetInvalid) {
		t.Errorf("err = %v, want ErrWidgetInvalid (duplicate)", err)
	}
}

func TestValidateAll_EmptyListIsValid(t *testing.T) {
	if err := ValidateAll(nil); err != nil {
		t.Errorf("nil list: err = %v, want nil", err)
	}
	if err := ValidateAll([]Widget{}); err != nil {
		t.Errorf("empty list: err = %v, want nil", err)
	}
}
