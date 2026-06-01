package builder

import (
	"strings"
	"testing"
)

// fixture covering every unique shape we observed in the active
// user workspace. Each row is (name, in, out). out is the EXACT
// expected Convert result - string equality matters because we want
// canonical output the builder dialog can edit.
func TestConvert_ObservedShapes(t *testing.T) {
	fields := []FieldRef{
		{Key: "check", Type: "boolean"},
		{Key: "test", Type: "text"},
		{Key: "is-flags", Type: "boolean"},
		{Key: "name", Type: "text"},
		{Key: "ontology", Type: "dropdown"},
		{Key: "tickertape", Type: "text"},
		{Key: "title", Type: "text"},
		{Key: "unit-number", Type: "text"},
		{Key: "street-address", Type: "text"},
		{Key: "naam", Type: "text"},
		{Key: "processen_naam", Type: "text"},
		{Key: "richtlijnen_naam", Type: "text"},
		{Key: "rol_in_team", Type: "text"},
		{Key: "status", Type: "text"},
		{Key: "datastroom_type", Type: "text"},
		{Key: "adapter-resource-group", Type: "text"},
		{Key: "adr_datum_beslissing", Type: "date"},
		{Key: "control-measure", Type: "text"},
		{Key: "techn_ontwerp_wijzigingsdatum", Type: "date"},
		{Key: "audit-control-datum-evaluatie", Type: "date"},
		{Key: "audit-control-naam", Type: "text"},
	}

	cases := []struct {
		name string
		in   string
		want string // substring expectations, not exact text
	}{
		{
			name: "empty",
			in:   "",
			want: "",
		},
		{
			name: "already canonical map",
			in:   `{text: F["tickertape"], color: "#f5dd5d", classes: ["expr-bold", "expr-scrolling"]}`,
			want: `F["tickertape"]`,
		},
		{
			name: "array-wrapped ternary (audit-controls)",
			in:   `[isExpiredAfter(F["audit-control-datum-evaluatie"], 30) ? { text: F["audit-control-naam"], classes: ["expr-text-red"] } : { text: F["audit-control-naam"], classes: ["expr-text-green"] }]`,
			want: `isExpiredAfter(F["audit-control-datum-evaluatie"], 30)`,
		},
		{
			name: "pipe form with F[] both sides",
			in:   `[ F["datastroom_type"] | { text: F["datastroom_type"], classes: ["expr-text-green", "expr-bold"] } ]`,
			want: `F["datastroom_type"]`,
		},
		{
			name: "pipe form with bare identifier",
			in:   `[ naam | { text: naam, classes: ["expr-text-green", "expr-bold"] } ]`,
			want: `F["naam"]`,
		},
		{
			name: "pipe form with bare string literal in concat",
			in:   `[ F["adapter-resource-group"] | { text: "Resource Group: " + F["adapter-resource-group"], classes: ["expr-text-black", "expr-bold"] } ]`,
			want: `L["Resource Group: "]`,
		},
		{
			name: "pipe form with bare id AND bare string in concat",
			in:   `[ adr_datum_beslissing | { text: "Goedgekeurd op: " + adr_datum_beslissing, classes: ["expr-text-green", "expr-bold"] } ]`,
			want: `str(L["Goedgekeurd op: "]) + str(F["adr_datum_beslissing"])`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Convert(tc.in, fields)
			if err != nil {
				t.Fatalf("Convert returned error: %v", err)
			}
			if tc.want == "" {
				if got != "" {
					t.Errorf("want empty, got %q", got)
				}
				return
			}
			if !strings.Contains(got, tc.want) {
				t.Errorf("Convert(%q)\n  got:  %s\n  want substring: %s", tc.in, got, tc.want)
			}
		})
	}
}

// Convert's output must round-trip through Parse → Compile cleanly.
// This is the load-bearing contract: converted output is something the
// dialog can edit and re-emit without further migration.
func TestConvert_RoundTripsThroughBuilder(t *testing.T) {
	fields := []FieldRef{
		{Key: "check", Type: "boolean"},
		{Key: "test", Type: "text"},
		{Key: "naam", Type: "text"},
		{Key: "title", Type: "text"},
		{Key: "tickertape", Type: "text"},
		{Key: "unit-number", Type: "text"},
		{Key: "street-address", Type: "text"},
		{Key: "audit-control-datum-evaluatie", Type: "date"},
		{Key: "audit-control-naam", Type: "text"},
		{Key: "is-flags", Type: "boolean"},
		{Key: "name", Type: "text"},
		{Key: "adr_datum_beslissing", Type: "date"},
	}

	sources := []string{
		`[ naam | { text: naam, classes: ["expr-text-green", "expr-bold"] } ]`,
		`{text: F["tickertape"], color: "#f5dd5d", classes: ["expr-bold", "expr-scrolling"]}`,
		`{text: F["unit-number"] + L[" "] + F["street-address"], color: "#ff9438", bg: "#000000", classes: ["expr-bold"]}`,
		`[isExpiredAfter(F["audit-control-datum-evaluatie"], 30) ? { text: F["audit-control-naam"], classes: ["expr-text-red", "expr-bold"] } : { text: F["audit-control-naam"], classes: ["expr-text-green", "expr-bold"] }]`,
		`[F["is-flags"] == true ? { text: F["name"], classes: ["expr-text-purple", "expr-bold"] } : { text: F["name"], classes: ["expr-text-purple", "expr-bold"] }]`,
		`[ adr_datum_beslissing | { text: "Goedgekeurd op: " + adr_datum_beslissing, classes: ["expr-text-green", "expr-bold"] } ]`,
	}

	for _, src := range sources {
		t.Run(src, func(t *testing.T) {
			converted, err := Convert(src, fields)
			if err != nil {
				t.Fatalf("Convert: %v", err)
			}
			cfg, err := Parse(converted, fields)
			if err != nil {
				t.Fatalf("Parse(converted): %v\n  converted: %s", err, converted)
			}
			recompiled, err := Compile(cfg, fields)
			if err != nil {
				t.Fatalf("Compile(Parse(converted)): %v", err)
			}
			// Round-trip identity: Compile(Parse(converted)) must equal
			// converted (the F/L/O DSL contract from feedback_round_trip_identity).
			if recompiled != converted {
				t.Errorf("round-trip drift\n  converted:  %s\n  recompiled: %s", converted, recompiled)
			}
		})
	}
}

// Empty input round-trips to empty output without erroring - the
// frontend uses Convert as a fallback when Parse fails, and an empty
// sidebar_expression must not trigger conversion at all.
func TestConvert_Empty(t *testing.T) {
	got, err := Convert("", nil)
	if err != nil {
		t.Fatalf("Convert(\"\"): %v", err)
	}
	if got != "" {
		t.Errorf("Convert(\"\") = %q, want empty", got)
	}
	got, err = Convert("   \n  ", nil)
	if err != nil {
		t.Fatalf("Convert whitespace: %v", err)
	}
	if got != "" {
		t.Errorf("Convert whitespace = %q, want empty", got)
	}
}

// Garbage in: an unparseable source must surface a clear error rather
// than silently emitting the garbage back. The dialog's Convert button
// only fires when Parse already failed, so a Convert failure means
// "really not migratable" - the user should know.
func TestConvert_GarbageReturnsError(t *testing.T) {
	_, err := Convert(`this is not @ valid expression`, nil)
	if err == nil {
		t.Fatal("expected error for unparseable input")
	}
}
