package stat

import "testing"

// TestService_ParseDSLForTemplate_DropsDanglingScale proves the backend, not
// the builder, resolves scale references: a known scaling survives, a renamed
// or deleted one is dropped, and with no template nothing is pruned.
func TestService_ParseDSLForTemplate_DropsDanglingScale(t *testing.T) {
	src := fakeSource{list: []StatObject{
		{Name: "by-name", DSL: `count() by F["name"]`},
		{Name: "urgency", Scaling: &Scaling{Source: SourceRef{Kind: SourceFacet, Key: "fcdm"}, Default: 1}},
	}}
	svc := NewService(NewManager(&fakeIndex{}), src)

	// A reference to an existing scaling is kept.
	cfg, err := svc.ParseDSLForTemplate("t", `count() by F["name"] scale "urgency"`)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Scales) != 1 || cfg.Scales[0] != "urgency" {
		t.Fatalf("known scale should survive, got %+v", cfg.Scales)
	}

	// A reference to a renamed/deleted scaling is dropped (no error).
	cfg, err = svc.ParseDSLForTemplate("t", `count() by F["name"] scale "gone"`)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Scales) != 0 {
		t.Fatalf("dangling scale should be dropped, got %+v", cfg.Scales)
	}

	// With no template (a brand-new statistic) nothing is pruned.
	cfg, err = svc.ParseDSLForTemplate("", `count() by F["name"] scale "gone"`)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Scales) != 1 {
		t.Fatalf("no-template parse should not prune, got %+v", cfg.Scales)
	}

	// A genuine parse error still surfaces.
	if _, err := svc.ParseDSLForTemplate("t", "this is not a dsl"); err == nil {
		t.Fatal("expected a parse error for a malformed DSL")
	}
}
