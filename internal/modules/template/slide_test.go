package template

import "testing"

// slide is the third structural singleton (guid, sequence, slide): minimal
// modal, forced read-only key, one per template, reserved key.
func TestSlideFieldDescriptor_IsMinimalSingleton(t *testing.T) {
	got, ok := fieldDescriptors["slide"]
	if !ok {
		t.Fatalf("slide descriptor missing")
	}
	a := got.Abilities
	if !a.Key || !a.Type {
		t.Errorf("slide must keep Key + Type")
	}
	if a.Label || a.Description || a.Options || a.TwoColumn || a.Default ||
		a.PrimaryKey || a.ExpressionItem || a.UseInStatistics {
		t.Errorf("slide modal must be minimal; got %+v", a)
	}
	if !got.KeyReadonly {
		t.Errorf("slide key must be read-only (forced singleton)")
	}
	if got.RequiresCollection {
		t.Errorf("slide is independent of collection")
	}
}

func TestNormalize_ForcesSlideKey(t *testing.T) {
	tpl := &Template{Fields: []Field{{Key: "whatever", Type: "slide"}}}
	Normalize(tpl)
	if got := tpl.Fields[0].Key; got != "slide" {
		t.Errorf("slide key = %q, want forced to \"slide\"", got)
	}
}

func TestValidate_MultipleSlideFields_Flagged(t *testing.T) {
	errs := Validate(&Template{Fields: []Field{
		{Key: "slide", Type: "slide"},
		{Key: "slide2", Type: "slide"},
	}})
	if !hasErr(errs, "multiple-slide-fields") {
		t.Errorf("expected multiple-slide-fields; got %+v", errs)
	}
}

func TestValidate_SlideReservedKey(t *testing.T) {
	// A plain field keyed "slide" is flagged; the slide field itself is fine.
	if errs := Validate(&Template{Fields: []Field{{Key: "slide", Type: "text"}}}); !hasErr(errs, "reserved-key") {
		t.Errorf("text field keyed \"slide\" should be reserved-key; got %+v", errs)
	}
	if errs := Validate(&Template{Fields: []Field{{Key: "slide", Type: "slide"}}}); hasErr(errs, "reserved-key") {
		t.Errorf("the slide field may use key \"slide\"; got %+v", errs)
	}
}

func TestSlideBlockKinds_RegistryAndMembership(t *testing.T) {
	kinds := SlideBlockKinds()
	want := []string{"textarea", "mermaid", "image", "table", "list"}
	if len(kinds) != len(want) {
		t.Fatalf("got %d kinds, want %d", len(kinds), len(want))
	}
	for i, w := range want {
		if kinds[i].Name != w {
			t.Errorf("kind %d = %q, want %q", i, kinds[i].Name, w)
		}
		if kinds[i].LabelKey == "" {
			t.Errorf("kind %q has no label key", w)
		}
		if !IsSlideBlockKind(w) {
			t.Errorf("IsSlideBlockKind(%q) should be true", w)
		}
	}
	if IsSlideBlockKind("formula") {
		t.Errorf("formula is not a slide block kind")
	}
	// Defensive copy: mutating the result must not affect the registry.
	kinds[0].Name = "mutated"
	if SlideBlockKinds()[0].Name != "textarea" {
		t.Errorf("SlideBlockKinds must return a defensive copy")
	}
}

func TestParseSlideDoc_RoundTripsNestedContent(t *testing.T) {
	raw := map[string]any{
		"blocks": []any{
			map[string]any{
				"id": "b1", "kind": "mermaid", "content": "graph TD; A-->B",
				"x": float64(40), "y": float64(60), "w": float64(600), "h": float64(300),
			},
			map[string]any{
				"id": "b2", "kind": "table",
				"content": []any{[]any{"a", "b"}, []any{"c", "d"}},
				"x":       float64(700), "y": float64(60), "w": float64(500), "h": float64(300),
			},
		},
	}
	doc, err := ParseSlideDoc(raw)
	if err != nil {
		t.Fatalf("ParseSlideDoc: %v", err)
	}
	if len(doc.Blocks) != 2 {
		t.Fatalf("got %d blocks, want 2", len(doc.Blocks))
	}
	if doc.Blocks[0].Kind != "mermaid" || doc.Blocks[0].W != 600 {
		t.Errorf("block 0 = %+v", doc.Blocks[0])
	}
	// Nested table content survives as a 2D array.
	rows, ok := doc.Blocks[1].Content.([]any)
	if !ok || len(rows) != 2 {
		t.Errorf("block 1 table content not preserved: %#v", doc.Blocks[1].Content)
	}
	// nil decodes to an empty doc.
	if d, err := ParseSlideDoc(nil); err != nil || len(d.Blocks) != 0 {
		t.Errorf("nil should be an empty doc; got %+v err=%v", d, err)
	}
}
