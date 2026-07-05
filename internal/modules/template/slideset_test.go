package template

import "testing"

// slideset is the deck-selector singleton: forced read-only key, one per
// template, reserved key, requires collection. Unlike guid/sequence it carries
// free-form author options (the decks) like dropdown, so no fixed OptionsShape.
func TestSlidesetFieldDescriptor_IsSingletonWithFreeOptions(t *testing.T) {
	got, ok := fieldDescriptors["slideset"]
	if !ok {
		t.Fatalf("slideset descriptor missing")
	}
	a := got.Abilities
	if !a.Key || !a.Type {
		t.Errorf("slideset must keep Key + Type")
	}
	if !a.Options {
		t.Errorf("slideset must advertise author-editable options (the decks)")
	}
	if got.OptionsShape != nil {
		t.Errorf("slideset options are free-form (no fixed shape); got %+v", got.OptionsShape)
	}
	if a.Label || a.Description || a.TwoColumn || a.Default ||
		a.PrimaryKey || a.UseInStatistics {
		t.Errorf("slideset modal must stay lean apart from options; got %+v", a)
	}
	if !a.ExpressionItem {
		t.Errorf("slideset value (the selected deck) is a scalar usable as an expression input")
	}
	if !got.KeyReadonly {
		t.Errorf("slideset key must be read-only (forced singleton)")
	}
	if !got.RequiresCollection {
		t.Errorf("slideset groups a collection, so it requires collection mode")
	}
}

func TestNormalize_ForcesSlidesetKey(t *testing.T) {
	tpl := &Template{Fields: []Field{{Key: "whatever", Type: "slideset"}}}
	Normalize(tpl)
	if got := tpl.Fields[0].Key; got != "slideset" {
		t.Errorf("slideset key = %q, want forced to \"slideset\"", got)
	}
}

func TestValidate_MultipleSlidesetFields_Flagged(t *testing.T) {
	errs := Validate(&Template{EnableCollection: true, Fields: []Field{
		{Key: "slideset", Type: "slideset"},
		{Key: "slideset2", Type: "slideset"},
	}})
	if !hasErr(errs, "multiple-slideset-fields") {
		t.Errorf("expected multiple-slideset-fields; got %+v", errs)
	}
}

func TestValidate_SlidesetReservedKey(t *testing.T) {
	if errs := Validate(&Template{EnableCollection: true, Fields: []Field{{Key: "slideset", Type: "text"}}}); !hasErr(errs, "reserved-key") {
		t.Errorf("text field keyed \"slideset\" should be reserved-key; got %+v", errs)
	}
	if errs := Validate(&Template{EnableCollection: true, Fields: []Field{{Key: "slideset", Type: "slideset"}}}); hasErr(errs, "reserved-key") {
		t.Errorf("the slideset field may use key \"slideset\"; got %+v", errs)
	}
}

func TestValidate_SlidesetNeedsSlide(t *testing.T) {
	// A slideset without a slide field is flagged (decks group slides).
	if errs := Validate(&Template{EnableCollection: true, Fields: []Field{
		{Key: "slideset", Type: "slideset"},
	}}); !hasErr(errs, "slideset-needs-slide") {
		t.Errorf("slideset without a slide field should be flagged; got %+v", errs)
	}
	// With a slide field present, no such error.
	if errs := Validate(&Template{EnableCollection: true, Fields: []Field{
		{Key: "slide", Type: "slide"},
		{Key: "slideset", Type: "slideset"},
	}}); hasErr(errs, "slideset-needs-slide") {
		t.Errorf("slideset with a slide field should be fine; got %+v", errs)
	}
}

func TestValidate_SlidesetNeedsCollection(t *testing.T) {
	// Without collection, a slideset field is flagged.
	if errs := Validate(&Template{Fields: []Field{{Key: "slideset", Type: "slideset"}}}); !hasErr(errs, "slideset-needs-collection") {
		t.Errorf("slideset without collection should be flagged; got %+v", errs)
	}
	// With collection, no such error.
	if errs := Validate(&Template{EnableCollection: true, Fields: []Field{{Key: "slideset", Type: "slideset"}}}); hasErr(errs, "slideset-needs-collection") {
		t.Errorf("slideset with collection should be fine; got %+v", errs)
	}
}
