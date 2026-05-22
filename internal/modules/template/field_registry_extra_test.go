package template

import (
	"bytes"
	"errors"
	"log/slog"
	"strings"
	"testing"

	"github.com/petervdpas/formidable2/internal/modules/system"
)

// ─────────────────────────────────────────────────────────────────────
// guid mirrors original Formidable's fieldTypes.js: label, description,
// primary_key, default, options, summary_field, expression_item,
// two_column, collapsible, readonly, format and the api group are all
// disabled. Normalize strips any value present in a field's disabled
// list so existing YAML with extra attributes self-heals on save and
// the validator doesn't trip on stale data.
// ─────────────────────────────────────────────────────────────────────

func TestNormalize_GuidStripsDisabledAttributes(t *testing.T) {
	tpl := &Template{
		Fields: []Field{{
			Key:         "id",
			Type:        "guid",
			Label:       "Custom Label",
			Description: "Some description",
			PrimaryKey:  true,
			Default:     "abc",
		}},
	}
	Normalize(tpl)
	f := tpl.Fields[0]
	if f.Label != "" {
		t.Errorf("guid Label should be stripped; got %q", f.Label)
	}
	if f.Description != "" {
		t.Errorf("guid Description should be stripped; got %q", f.Description)
	}
	if f.PrimaryKey {
		t.Errorf("guid PrimaryKey should be stripped to false")
	}
	if f.Default != nil {
		t.Errorf("guid Default should be stripped; got %#v", f.Default)
	}
	if f.Key != "id" {
		t.Errorf("guid Key should be forced to \"id\"; got %q", f.Key)
	}
}

func TestValidate_GuidPassesAfterNormalize(t *testing.T) {
	tpl := &Template{
		Fields: []Field{{
			Key:         "id",
			Type:        "guid",
			Label:       "GUID",
			Description: "x",
			PrimaryKey:  true,
		}},
	}
	Normalize(tpl)
	errs := Validate(tpl)
	if anyForbiddenFor(errs, "id") {
		t.Errorf("guid should validate after Normalize; got %+v", errs)
	}
}

func TestValidate_GuidStillForbidsCollapsibleAndDefault(t *testing.T) {
	// Regression guard for the still-forbidden attrs. Default is split
	// out below - collapsible is the simple one.
	errs := Validate(&Template{
		Fields: []Field{{Key: "g", Type: "guid", Collapsible: boolPtr(true)}},
	})
	if !hasForbidden(errs, "g", "collapsible") {
		t.Errorf("guid must still forbid collapsible; got %+v", errs)
	}
}

func TestValidate_GuidStillForbidsPopulatedDefault(t *testing.T) {
	errs := Validate(&Template{
		Fields: []Field{{Key: "g", Type: "guid", Default: "abc"}},
	})
	if !hasForbidden(errs, "g", "default") {
		t.Errorf("guid with a populated default must still be flagged; got %+v", errs)
	}
}

// ─────────────────────────────────────────────────────────────────────
// Empty / zero defaults are not "set"
//
// YAML round-trip can leave `default: ""` on a guid/loopstart/loopstop
// even when the user never typed one. Treat the quiet zero as "not set"
// so the seed Examples templates validate cleanly.
// ─────────────────────────────────────────────────────────────────────

func TestValidate_EmptyDefaultOnGuidIsNotFlagged(t *testing.T) {
	errs := Validate(&Template{
		Fields: []Field{{Key: "id", Type: "guid", Default: ""}},
	})
	if hasForbidden(errs, "id", "default") {
		t.Errorf("empty-string default on guid must not be flagged; got %+v", errs)
	}
}

func TestValidate_EmptyDefaultOnLoopstartIsNotFlagged(t *testing.T) {
	errs := Validate(&Template{
		Fields: []Field{
			{Key: "items", Type: "loopstart", Default: ""},
			{Key: "name", Type: "text"},
			{Key: "items", Type: "loopstop", Default: ""},
		},
	})
	if hasForbidden(errs, "items", "default") {
		t.Errorf("empty default on loop fields must not be flagged; got %+v", errs)
	}
}

func TestDefaultIsPopulated_AllZeroVariantsAreFalse(t *testing.T) {
	cases := map[string]any{
		"nil":         nil,
		"empty":       "",
		"zero-int":    0,
		"zero-int64":  int64(0),
		"zero-float":  float64(0),
		"false-bool":  false,
		"empty-slice": []any{},
		"empty-map":   map[string]any{},
	}
	for name, v := range cases {
		if defaultIsPopulated(v) {
			t.Errorf("%s should not count as populated; got true for %#v", name, v)
		}
	}
}

func TestDefaultIsPopulated_TruthyVariantsAreTrue(t *testing.T) {
	cases := map[string]any{
		"non-empty-string": "x",
		"nonzero-int":      5,
		"nonzero-int64":    int64(5),
		"nonzero-float":    1.5,
		"true-bool":        true,
		"populated-slice":  []any{"a"},
		"populated-map":    map[string]any{"k": "v"},
	}
	for name, v := range cases {
		if !defaultIsPopulated(v) {
			t.Errorf("%s should count as populated; got false for %#v", name, v)
		}
	}
}

// ─────────────────────────────────────────────────────────────────────
// SaveTemplate logs validation rejections to the manager logger
//
// formidable.log gets every rejection so a failed save isn't a black box
// for whoever's debugging from the log file. The frontend pre-validates
// too - this branch is hit by HTTP / sync / scripted callers.
// ─────────────────────────────────────────────────────────────────────

func TestSaveTemplate_LogsValidationErrors(t *testing.T) {
	root := t.TempDir()
	sys := system.NewManager(root, nil)

	var buf bytes.Buffer
	log := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	m := NewManager(sys, "templates", log)

	tmpl := &Template{
		Name:     "Bad",
		Filename: "bad.yaml",
		Fields: []Field{
			{Key: "id", Type: "guid"},
			{Key: "alt", Type: "guid"},
		},
	}
	err := m.SaveTemplate("bad.yaml", tmpl)
	if err == nil {
		t.Fatal("expected validation error, got nil")
	}
	var verr *ValidationFailedError
	if !errors.As(err, &verr) {
		t.Fatalf("expected *ValidationFailedError, got %T", err)
	}

	logged := buf.String()
	if !strings.Contains(logged, `level=WARN`) ||
		!strings.Contains(logged, `msg="template validation rejected save"`) {
		t.Errorf("expected a WARN line about validation; got:\n%s", logged)
	}
	if !strings.Contains(logged, `name=bad.yaml`) {
		t.Errorf("log line should carry the template name; got:\n%s", logged)
	}
	if !strings.Contains(logged, `type=multiple-guid-fields`) {
		t.Errorf("log line should carry the validation error type; got:\n%s", logged)
	}
}
