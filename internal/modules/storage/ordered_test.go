package storage

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/petervdpas/formidable2/internal/modules/template"
)

// dataKeyOrder returns the top-level key order of the "data" object as it
// appears in the raw JSON (a decoded map would lose order).
func dataKeyOrder(t *testing.T, raw []byte) []string {
	t.Helper()
	var doc orderedProbe
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("probe unmarshal: %v", err)
	}
	return doc.order
}

// orderedProbe captures the key order of the top-level "data" object by
// streaming tokens: it walks to the "data" object, then reads alternating
// key/value pairs, skipping each value (recursively for nested objects and
// arrays) so only keys are recorded.
type orderedProbe struct {
	order []string
}

func (p *orderedProbe) UnmarshalJSON(b []byte) error {
	dec := json.NewDecoder(strings.NewReader(string(b)))
	// Advance to the value following the top-level "data" key.
	depth := 0
	for {
		tok, err := dec.Token()
		if err != nil {
			return nil
		}
		if d, ok := tok.(json.Delim); ok {
			if d == '{' || d == '[' {
				depth++
			} else {
				depth--
			}
			continue
		}
		if s, ok := tok.(string); ok && s == "data" && depth == 1 {
			break
		}
	}
	// Consume the opening '{' of the data object.
	if _, err := dec.Token(); err != nil {
		return nil
	}
	// Read key, skip value, until the closing '}'.
	for dec.More() {
		keyTok, err := dec.Token()
		if err != nil {
			return nil
		}
		p.order = append(p.order, keyTok.(string))
		if err := skipValue(dec); err != nil {
			return nil
		}
	}
	return nil
}

// skipValue consumes exactly one JSON value from dec, descending through
// nested objects/arrays by depth.
func skipValue(dec *json.Decoder) error {
	tok, err := dec.Token()
	if err != nil {
		return err
	}
	d, ok := tok.(json.Delim)
	if !ok {
		return nil // scalar
	}
	if d != '{' && d != '[' {
		return nil
	}
	depth := 1
	for depth > 0 {
		tok, err := dec.Token()
		if err != nil {
			return err
		}
		if dd, ok := tok.(json.Delim); ok {
			if dd == '{' || dd == '[' {
				depth++
			} else {
				depth--
			}
		}
	}
	return nil
}

func TestSaveForm_DataFollowsTemplateFieldOrder(t *testing.T) {
	m, sys, tplM, root := newTestStack(t)
	_ = sys
	fields := []template.Field{
		{Key: "zebra", Type: "text"},
		{Key: "alpha", Type: "text"},
		{Key: "rows", Type: "loopstart"},
		{Key: "inner_z", Type: "text"},
		{Key: "inner_a", Type: "text"},
		{Key: "rows", Type: "loopstop"},
		{Key: "middle", Type: "text"},
	}
	if err := tplM.SaveTemplate("ord.yaml", &template.Template{
		Name: "ord", Filename: "ord.yaml", Fields: fields,
	}); err != nil {
		t.Fatalf("save template: %v", err)
	}

	data := map[string]any{
		"alpha":  "a",
		"middle": "m",
		"zebra":  "z",
		"rows": []any{
			map[string]any{"inner_a": "1", "inner_z": "2"},
		},
	}
	if r := m.SaveForm(context.Background(), "ord.yaml", "f1", data); !r.Success {
		t.Fatalf("save: %s", r.Error)
	}

	raw, err := os.ReadFile(filepath.Join(root, "storage", "ord", "f1.meta.json"))
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	got := dataKeyOrder(t, raw)
	want := []string{"zebra", "alpha", "rows", "middle"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("top-level data order = %v, want %v\n%s", got, want, raw)
	}

	// Inner loop-item order should mirror inner field order too.
	zIdx := strings.Index(string(raw), `"inner_z"`)
	aIdx := strings.Index(string(raw), `"inner_a"`)
	if zIdx < 0 || aIdx < 0 || zIdx > aIdx {
		t.Errorf("inner loop order wrong: inner_z at %d, inner_a at %d\n%s", zIdx, aIdx, raw)
	}
}

func TestOrderData_AppendsOrphanKeysSorted(t *testing.T) {
	fields := []template.Field{{Key: "b", Type: "text"}, {Key: "a", Type: "text"}}
	data := map[string]any{"a": 1, "b": 2, "zzz": 3, "mmm": 4}
	o := orderData(data, fields)
	want := []string{"b", "a", "mmm", "zzz"}
	if strings.Join(o.keys, ",") != strings.Join(want, ",") {
		t.Errorf("order = %v, want %v", o.keys, want)
	}
}
