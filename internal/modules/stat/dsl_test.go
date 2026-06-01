package stat

import (
	"reflect"
	"testing"
)

func fptr(f float64) *float64 { return &f }

// ── Compile: Config -> canonical string ──────────────────────────────

func TestCompile_Canonical(t *testing.T) {
	cases := []struct {
		name string
		cfg  StatConfig
		want string
	}{
		{
			name: "count by field",
			cfg: StatConfig{
				Measures:   []Measure{{Op: OpCount}},
				Dimensions: []Dimension{{Source: SourceRef{Kind: SourceField, Key: "status"}}},
			},
			want: `count() by F["status"]`,
		},
		{
			name: "count by two facets",
			cfg: StatConfig{
				Measures: []Measure{{Op: OpCount}},
				Dimensions: []Dimension{
					{Source: SourceRef{Kind: SourceFacet, Key: "priority"}},
					{Source: SourceRef{Kind: SourceFacet, Key: "stage"}},
				},
			},
			want: `count() by Facet["priority"], Facet["stage"]`,
		},
		{
			name: "date bin",
			cfg: StatConfig{
				Measures:   []Measure{{Op: OpCount}},
				Dimensions: []Dimension{{Source: SourceRef{Kind: SourceField, Key: "due"}, Bin: BinMonth}},
			},
			want: `count() by F["due"]@month`,
		},
		{
			name: "avg field",
			cfg: StatConfig{
				Measures:   []Measure{{Op: OpAvg, Source: &SourceRef{Kind: SourceField, Key: "amount"}}},
				Dimensions: []Dimension{{Source: SourceRef{Kind: SourceField, Key: "status"}}},
			},
			want: `avg(F["amount"]) by F["status"]`,
		},
		{
			name: "table column source + date bin",
			cfg: StatConfig{
				Measures: []Measure{{Op: OpSum, Source: &SourceRef{Kind: SourceField, Key: "items", Column: "qty"}}},
				Dimensions: []Dimension{
					{Source: SourceRef{Kind: SourceField, Key: "region"}},
					{Source: SourceRef{Kind: SourceField, Key: "due"}, Bin: BinYear},
				},
			},
			want: `sum(F["items"]["qty"]) by F["region"], F["due"]@year`,
		},
		{
			name: "multiple measures",
			cfg: StatConfig{
				Measures: []Measure{
					{Op: OpCount},
					{Op: OpAvg, Source: &SourceRef{Kind: SourceField, Key: "amount"}},
				},
				Dimensions: []Dimension{{Source: SourceRef{Kind: SourceField, Key: "status"}}},
			},
			want: `count(), avg(F["amount"]) by F["status"]`,
		},
		{
			name: "percentile",
			cfg: StatConfig{
				Measures: []Measure{{Op: OpPercentile, Source: &SourceRef{Kind: SourceField, Key: "amount"}, Arg: fptr(90)}},
			},
			want: `percentile(F["amount"], 90)`,
		},
		{
			name: "rank-0 scalar count",
			cfg:  StatConfig{Measures: []Measure{{Op: OpCount}}},
			want: `count()`,
		},
		{
			name: "distinct-form count by table column",
			cfg: StatConfig{
				Measures:   []Measure{{Op: OpRecords}},
				Dimensions: []Dimension{{Source: SourceRef{Kind: SourceField, Key: "components", Column: "item"}, Top: 10}},
			},
			want: `records() by F["components"]["item"] top 10`,
		},
		{
			name: "count and records together",
			cfg: StatConfig{
				Measures:   []Measure{{Op: OpCount}, {Op: OpRecords}},
				Dimensions: []Dimension{{Source: SourceRef{Kind: SourceField, Key: "status"}}},
			},
			want: `count(), records() by F["status"]`,
		},
		{
			name: "percent base forms",
			cfg: StatConfig{
				Measures:   []Measure{{Op: OpCount}},
				Dimensions: []Dimension{{Source: SourceRef{Kind: SourceField, Key: "status"}}},
				Percent:    PctForms,
			},
			want: `count() by F["status"] pct forms`,
		},
		{
			name: "percent base distribution is the default and omitted",
			cfg: StatConfig{
				Measures:   []Measure{{Op: OpCount}},
				Dimensions: []Dimension{{Source: SourceRef{Kind: SourceField, Key: "status"}}},
				Percent:    PctDistribution,
			},
			want: `count() by F["status"]`,
		},
		{
			name: "top-N on a dimension",
			cfg: StatConfig{
				Measures:   []Measure{{Op: OpCount}},
				Dimensions: []Dimension{{Source: SourceRef{Kind: SourceField, Key: "base-table"}, Top: 10}},
			},
			want: `count() by F["base-table"] top 10`,
		},
		{
			name: "top-N after a date bin",
			cfg: StatConfig{
				Measures:   []Measure{{Op: OpCount}},
				Dimensions: []Dimension{{Source: SourceRef{Kind: SourceField, Key: "due"}, Bin: BinMonth, Top: 12}},
			},
			want: `count() by F["due"]@month top 12`,
		},
		{
			name: "where equality filter (table column)",
			cfg: StatConfig{
				Measures:   []Measure{{Op: OpCount}},
				Dimensions: []Dimension{{Source: SourceRef{Kind: SourceField, Key: "base-table"}}},
				Filters:    []Filter{{Source: SourceRef{Kind: SourceField, Key: "datasets", Column: "entry"}, Op: FilterEq, Value: "Item1"}},
			},
			want: `count() by F["base-table"] where F["datasets"]["entry"] eq "Item1"`,
		},
		{
			name: "where numeric comparison, AND-chained with a facet ne",
			cfg: StatConfig{
				Measures: []Measure{{Op: OpCount}},
				Filters: []Filter{
					{Source: SourceRef{Kind: SourceField, Key: "amount"}, Op: FilterGt, Value: "100"},
					{Source: SourceRef{Kind: SourceFacet, Key: "qzm"}, Op: FilterNe, Value: "ZONNIG"},
				},
			},
			want: `count() where F["amount"] gt 100 and Facet["qzm"] ne "ZONNIG"`,
		},
		{
			name: "scale reference",
			cfg: StatConfig{
				Measures:   []Measure{{Op: OpRecords}},
				Dimensions: []Dimension{{Source: SourceRef{Kind: SourceField, Key: "components", Column: "item"}, Top: 10}},
				Scales:     []string{"qzm-urgency"},
			},
			want: `records() by F["components"]["item"] top 10 scale "qzm-urgency"`,
		},
		{
			name: "scale before pct (canonical order)",
			cfg: StatConfig{
				Measures: []Measure{{Op: OpCount}},
				Scales:   []string{"w"},
				Percent:  PctForms,
			},
			want: `count() scale "w" pct forms`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Compile(tc.cfg)
			if err != nil {
				t.Fatalf("Compile: %v", err)
			}
			if got != tc.want {
				t.Errorf("Compile = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestCompile_Rejects(t *testing.T) {
	cases := []struct {
		name string
		cfg  StatConfig
	}{
		{"no measures", StatConfig{}},
		{"count with source", StatConfig{Measures: []Measure{{Op: OpCount, Source: &SourceRef{Kind: SourceField, Key: "x"}}}}},
		{"records with source", StatConfig{Measures: []Measure{{Op: OpRecords, Source: &SourceRef{Kind: SourceField, Key: "x"}}}}},
		{"avg without source", StatConfig{Measures: []Measure{{Op: OpAvg}}}},
		{"avg over facet", StatConfig{Measures: []Measure{{Op: OpAvg, Source: &SourceRef{Kind: SourceFacet, Key: "p"}}}}},
		{"percentile without arg", StatConfig{Measures: []Measure{{Op: OpPercentile, Source: &SourceRef{Kind: SourceField, Key: "x"}}}}},
		{"unknown op", StatConfig{Measures: []Measure{{Op: "bogus"}}}},
		{"top below range", StatConfig{Measures: []Measure{{Op: OpCount}}, Dimensions: []Dimension{{Source: SourceRef{Kind: SourceField, Key: "x"}, Top: 0 - 1}}}},
		{"top above range", StatConfig{Measures: []Measure{{Op: OpCount}}, Dimensions: []Dimension{{Source: SourceRef{Kind: SourceField, Key: "x"}, Top: 21}}}},
		{"comparison filter with non-numeric value", StatConfig{Measures: []Measure{{Op: OpCount}}, Filters: []Filter{{Source: SourceRef{Kind: SourceField, Key: "x"}, Op: FilterGt, Value: "abc"}}}},
		{"unknown filter op", StatConfig{Measures: []Measure{{Op: OpCount}}, Filters: []Filter{{Source: SourceRef{Kind: SourceField, Key: "x"}, Op: "bogus", Value: "y"}}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := Compile(tc.cfg); err == nil {
				t.Errorf("Compile(%+v) = nil error, want error", tc.cfg)
			}
		})
	}
}

// ── Parse: string -> Config ──────────────────────────────────────────

func TestParse_Shapes(t *testing.T) {
	got, err := Parse(`count(), avg(F["amount"]) by F["status"], F["due"]@month`)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	want := StatConfig{
		Measures: []Measure{
			{Op: OpCount},
			{Op: OpAvg, Source: &SourceRef{Kind: SourceField, Key: "amount"}},
		},
		Dimensions: []Dimension{
			{Source: SourceRef{Kind: SourceField, Key: "status"}},
			{Source: SourceRef{Kind: SourceField, Key: "due"}, Bin: BinMonth},
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Parse mismatch:\n got %+v\nwant %+v", got, want)
	}
}

func TestParse_TableColumnAndPercentile(t *testing.T) {
	got, err := Parse(`percentile(F["items"]["qty"], 95)`)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(got.Measures) != 1 {
		t.Fatalf("measures = %d", len(got.Measures))
	}
	m := got.Measures[0]
	if m.Op != OpPercentile || m.Source == nil || m.Source.Key != "items" || m.Source.Column != "qty" {
		t.Errorf("source wrong: %+v", m.Source)
	}
	if m.Arg == nil || *m.Arg != 95 {
		t.Errorf("arg = %v, want 95", m.Arg)
	}
}

func TestParse_Rejects(t *testing.T) {
	bad := []string{
		``,                               // empty
		`count`,                          // missing parens
		`count(`,                         // unterminated
		`count() by`,                     // dangling by
		`count() by F["x"`,               // unterminated bracket
		`avg(Facet["p"])`,                // reduce over facet
		`records(F["x"])`,                // records takes no source
		`bogus()`,                        // unknown measure
		`count() by F["due"]@decade`,     // bad bin
		`count() extra`,                  // trailing tokens
		`F["x"]`,                         // no measure
		`count() by F[status]`,           // unquoted key
		`count() by F["x"] top`,          // top without a number
		`count() by F["x"] top abc`,      // top with a non-number
		`count() where`,                  // dangling where
		`count() where F["x"]`,           // filter missing op + value
		`count() where F["x"] eq 5`,      // eq needs a quoted string
		`count() where F["x"] gt "y"`,    // gt needs a number
		`count() where F["x"] bogus "y"`, // unknown operator
	}
	for _, src := range bad {
		t.Run(src, func(t *testing.T) {
			if _, err := Parse(src); err == nil {
				t.Errorf("Parse(%q) = nil error, want error", src)
			}
		})
	}
}

// ── Round-trip identity: Compile(Parse(Compile(x))) == Compile(x) ─────

func TestRoundTrip_Identity(t *testing.T) {
	configs := []StatConfig{
		{Measures: []Measure{{Op: OpCount}}},
		{Measures: []Measure{{Op: OpCount}}, Dimensions: []Dimension{{Source: SourceRef{Kind: SourceField, Key: "status"}}}},
		{
			Measures: []Measure{{Op: OpCount}},
			Dimensions: []Dimension{
				{Source: SourceRef{Kind: SourceFacet, Key: "priority"}},
				{Source: SourceRef{Kind: SourceFacet, Key: "stage"}},
			},
		},
		{
			Measures: []Measure{
				{Op: OpCount},
				{Op: OpAvg, Source: &SourceRef{Kind: SourceField, Key: "amount"}},
				{Op: OpPercentile, Source: &SourceRef{Kind: SourceField, Key: "amount"}, Arg: fptr(90)},
			},
			Dimensions: []Dimension{
				{Source: SourceRef{Kind: SourceField, Key: "region"}},
				{Source: SourceRef{Kind: SourceField, Key: "due"}, Bin: BinYear},
			},
		},
		{
			Measures:   []Measure{{Op: OpSum, Source: &SourceRef{Kind: SourceField, Key: "items", Column: "qty"}}},
			Dimensions: []Dimension{{Source: SourceRef{Kind: SourceField, Key: "hyphen-key"}, Bin: BinDay}},
		},
		{
			Measures:   []Measure{{Op: OpCount}},
			Dimensions: []Dimension{{Source: SourceRef{Kind: SourceField, Key: "base-table"}, Top: 10}},
		},
		{
			Measures:   []Measure{{Op: OpCount}, {Op: OpRecords}},
			Dimensions: []Dimension{{Source: SourceRef{Kind: SourceField, Key: "components", Column: "item"}, Top: 10}},
		},
		{
			Measures:   []Measure{{Op: OpCount}},
			Dimensions: []Dimension{{Source: SourceRef{Kind: SourceField, Key: "status"}}},
			Percent:    PctForms,
		},
		{
			Measures:   []Measure{{Op: OpCount}},
			Dimensions: []Dimension{{Source: SourceRef{Kind: SourceField, Key: "status"}}},
			Percent:    PctNone,
		},
		{
			Measures:   []Measure{{Op: OpCount}},
			Dimensions: []Dimension{{Source: SourceRef{Kind: SourceField, Key: "base-table"}, Top: 10}},
			Filters: []Filter{
				{Source: SourceRef{Kind: SourceField, Key: "grp", Column: "entry"}, Op: FilterEq, Value: "P1"},
				{Source: SourceRef{Kind: SourceField, Key: "amount"}, Op: FilterGe, Value: "5"},
			},
		},
		{
			Measures:   []Measure{{Op: OpRecords}},
			Dimensions: []Dimension{{Source: SourceRef{Kind: SourceField, Key: "components", Column: "item"}, Top: 10}},
			Filters:    []Filter{{Source: SourceRef{Kind: SourceFacet, Key: "flag"}, Op: FilterEq, Value: "IN OMLOOP"}},
			Scales:     []string{"qzm-urgency"},
		},
		{
			Measures:   []Measure{{Op: OpRecords}},
			Dimensions: []Dimension{{Source: SourceRef{Kind: SourceField, Key: "components", Column: "item"}, Top: 10}},
			Scales:     []string{"qzm-urgency"},
			Percent:    PctNone,
		},
		{
			Measures:   []Measure{{Op: OpCount}},
			Dimensions: []Dimension{{Source: SourceRef{Kind: SourceField, Key: "app"}, Top: 10}},
			Scales:     []string{"tshirt-impact", "qzm-urgency"},
			Percent:    PctForms,
		},
	}
	for i, cfg := range configs {
		src1, err := Compile(cfg)
		if err != nil {
			t.Fatalf("[%d] Compile: %v", i, err)
		}
		parsed, err := Parse(src1)
		if err != nil {
			t.Fatalf("[%d] Parse(%q): %v", i, src1, err)
		}
		src2, err := Compile(parsed)
		if err != nil {
			t.Fatalf("[%d] re-Compile: %v", i, err)
		}
		if src1 != src2 {
			t.Errorf("[%d] round-trip drift:\n first %q\nsecond %q", i, src1, src2)
		}
	}
}
