package storage

import (
	"testing"

	"github.com/petervdpas/formidable2/internal/modules/template"
)

func TestSanitize_FlagStateFromInjectedMeta(t *testing.T) {
	fields := []template.Field{{Key: "title", Type: "text"}}
	raw := map[string]any{
		"title": "Hello",
		"_meta": map[string]any{"flag_state": "FLASH"},
	}
	out := Sanitize(raw, fields, SanitizeOptions{})
	if out.Meta.FlagState != "FLASH" {
		t.Errorf("FlagState = %q, want %q", out.Meta.FlagState, "FLASH")
	}
}

func TestSanitize_FlagStateFromRawMetaEnvelope(t *testing.T) {
	fields := []template.Field{{Key: "title", Type: "text"}}
	envelope := map[string]any{
		"data": map[string]any{"title": "Hi"},
		"meta": map[string]any{"flag_state": "IMMEDIATE"},
	}
	out := Sanitize(envelope, fields, SanitizeOptions{})
	if out.Meta.FlagState != "IMMEDIATE" {
		t.Errorf("FlagState = %q, want %q", out.Meta.FlagState, "IMMEDIATE")
	}
}

func TestSanitize_FlagStateFromOptions(t *testing.T) {
	fields := []template.Field{{Key: "title", Type: "text"}}
	out := Sanitize(map[string]any{"title": "X"}, fields, SanitizeOptions{FlagState: "PRIORITY"})
	if out.Meta.FlagState != "PRIORITY" {
		t.Errorf("FlagState = %q, want %q", out.Meta.FlagState, "PRIORITY")
	}
}

func TestSanitize_FlagStateRawMetaWinsOverOptions(t *testing.T) {
	fields := []template.Field{{Key: "title", Type: "text"}}
	envelope := map[string]any{
		"data": map[string]any{"title": "X"},
		"meta": map[string]any{"flag_state": "FLASH"},
	}
	out := Sanitize(envelope, fields, SanitizeOptions{FlagState: "ROUTINE"})
	if out.Meta.FlagState != "FLASH" {
		t.Errorf("FlagState = %q, want raw meta to win (FLASH)", out.Meta.FlagState)
	}
}

func TestSanitize_FlagStateDefaultsEmpty(t *testing.T) {
	fields := []template.Field{{Key: "title", Type: "text"}}
	out := Sanitize(map[string]any{"title": "X"}, fields, SanitizeOptions{})
	if out.Meta.FlagState != "" {
		t.Errorf("FlagState = %q, want empty", out.Meta.FlagState)
	}
}

func TestSanitize_LegacyFlaggedTrueSurvivesWithEmptyState(t *testing.T) {
	fields := []template.Field{{Key: "title", Type: "text"}}
	raw := map[string]any{
		"title": "Hello",
		"_meta": map[string]any{"flagged": true},
	}
	out := Sanitize(raw, fields, SanitizeOptions{})
	if !out.Meta.Flagged {
		t.Errorf("Flagged = false, want true (legacy bool must survive)")
	}
	if out.Meta.FlagState != "" {
		t.Errorf("FlagState = %q, want empty (no implicit state)", out.Meta.FlagState)
	}
}

func TestSanitize_FlaggedAndStateAreIndependent(t *testing.T) {
	fields := []template.Field{{Key: "title", Type: "text"}}
	raw := map[string]any{
		"title": "X",
		"_meta": map[string]any{
			"flagged":    false,
			"flag_state": "FLASH",
		},
	}
	out := Sanitize(raw, fields, SanitizeOptions{})
	if out.Meta.Flagged {
		t.Errorf("Flagged = true, want false (storage doesn't infer)")
	}
	if out.Meta.FlagState != "FLASH" {
		t.Errorf("FlagState = %q, want FLASH", out.Meta.FlagState)
	}
}

func TestSanitize_FlagStateNonStringIgnored(t *testing.T) {
	fields := []template.Field{{Key: "title", Type: "text"}}
	raw := map[string]any{
		"title": "X",
		"_meta": map[string]any{"flag_state": 123},
	}
	out := Sanitize(raw, fields, SanitizeOptions{})
	if out.Meta.FlagState != "" {
		t.Errorf("FlagState = %q, want empty (non-string ignored)", out.Meta.FlagState)
	}
}
