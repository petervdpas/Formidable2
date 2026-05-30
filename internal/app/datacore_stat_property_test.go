package app

import (
	"fmt"
	"math/rand"
	"strconv"
	"testing"

	"github.com/petervdpas/formidable2/internal/modules/index"
	"github.com/petervdpas/formidable2/internal/modules/stat"
)

// Layer 3 of the parity gate: randomized, seeded property parity. For many
// random fixtures, every non-fanning stat.Index method must agree between the
// index and the datacore adapter. The seed is the loop index, so a failure
// names the exact fixture to reproduce. The generator deliberately avoids the
// two settled divergences: facets are always set with a non-empty option (no
// "(unset)" bucket), and no operation crosses two table columns (the engine
// would reject it anyway). What's left is where the engines must be identical,
// so any disagreement here is a real bug, not an intended difference.

// randomStatForms builds a random but well-typed fixture: a status text field,
// an amount number (sometimes blank, sometimes junk), a due date (sometimes
// absent), two always-set facets, and an items table of 0..3 cost rows.
func randomStatForms(rng *rand.Rand) []statForm {
	statuses := []string{"high", "low", "mid"}
	tiers := []string{"GOLD", "SILVER", "BRONZE"}
	stages := []string{"draft", "live"}

	n := 3 + rng.Intn(12)
	forms := make([]statForm, n)
	for i := range forms {
		f := statForm{
			id:   fmt.Sprintf("f%02d.meta.json", i),
			text: map[string]string{"status": statuses[rng.Intn(len(statuses))]},
			facets: map[string]string{
				"tier":  tiers[rng.Intn(len(tiers))],
				"stage": stages[rng.Intn(len(stages))],
			},
		}
		switch r := rng.Intn(5); {
		case r < 3:
			f.num = map[string]string{"amount": strconv.Itoa(rng.Intn(100))}
		case r == 3:
			f.num = map[string]string{"amount": ""} // blank: absence both engines
		default:
			f.num = map[string]string{"amount": "oops"} // anomaly both engines
		}
		if rng.Intn(5) != 0 {
			f.date = map[string]string{"due": fmt.Sprintf("%04d-%02d-%02d", 2025+rng.Intn(3), 1+rng.Intn(12), 1+rng.Intn(28))}
		}
		for k := 0; k < rng.Intn(4); k++ {
			f.costs = append(f.costs, strconv.Itoa(1+rng.Intn(500)))
		}
		forms[i] = f
	}
	return forms
}

func TestStatProperty_RandomParity(t *testing.T) {
	col0 := 0
	for seed := 1; seed <= 60; seed++ {
		t.Run(fmt.Sprintf("seed-%02d", seed), func(t *testing.T) {
			rng := rand.New(rand.NewSource(int64(seed)))
			forms := randomStatForms(rng)
			a, idxM := newDatacoreStatAdapter(t, forms)

			it, _ := idxM.TotalForms("basic.yaml")
			dt, err := a.TotalForms("basic.yaml")
			if err != nil {
				t.Fatalf("TotalForms: %v", err)
			}
			if it != dt {
				t.Fatalf("TotalForms index=%d datacore=%d", it, dt)
			}

			si, sd := mustDist(t, idxM, a, "status", nil)
			assertBucketsEqual(t, "status", si, sd)
			ti, td := mustDist(t, idxM, a, "items", &col0)
			assertBucketsEqual(t, "items.cost", ti, td)
			ai, ad := mustNumeric(t, idxM, a, "amount", nil)
			assertFloatsEqual(t, "amount", ai, ad)
			ii, id := mustNumeric(t, idxM, a, "items", &col0)
			assertFloatsEqual(t, "items.cost", ii, id)

			fi, _ := idxM.FacetDistribution("basic.yaml", "tier")
			fd, err := a.FacetDistribution("basic.yaml", "tier")
			if err != nil {
				t.Fatalf("FacetDistribution: %v", err)
			}
			assertBucketsEqual(t, "facet tier", fi, fd)

			ci, _ := idxM.FacetCross("basic.yaml", "tier", "stage")
			cd, err := a.FacetCross("basic.yaml", "tier", "stage")
			if err != nil {
				t.Fatalf("FacetCross: %v", err)
			}
			assertCrossEqual(t, "tier x stage", ci, cd)

			for _, period := range []string{"year", "month", "day"} {
				di, _ := idxM.DateSeries("basic.yaml", "due", nil, period)
				dd, err := a.DateSeries("basic.yaml", "due", nil, period)
				if err != nil {
					t.Fatalf("DateSeries %s: %v", period, err)
				}
				assertBucketsEqual(t, "due "+period, di, dd)
			}

			dims := []index.AggDim{{Kind: "field", Key: "status"}, {Kind: "facet", Key: "tier"}}
			nums := []index.AggNum{{Key: "amount"}}
			filters := []index.AggFilter{{Kind: "field", Key: "status", Op: "eq", Value: "high"}}
			ri, _ := idxM.AggregateRaw("basic.yaml", dims, nums, filters)
			rd, err := a.AggregateRaw("basic.yaml", dims, nums, filters)
			if err != nil {
				t.Fatalf("AggregateRaw: %v", err)
			}
			if wi, wd := indexRawKeys(ri), indexRawKeys(rd); !equalStrings(wi, wd) {
				t.Fatalf("AggregateRaw rows: index=%v datacore=%v", wi, wd)
			}
		})
	}
}

func mustDist(t *testing.T, idxM *index.Manager, a *datacoreStatIndex, field string, col *int) ([]index.Bucket, []index.Bucket) {
	t.Helper()
	i, _ := idxM.ValueDistribution("basic.yaml", field, col)
	d, err := a.ValueDistribution("basic.yaml", field, col)
	if err != nil {
		t.Fatalf("ValueDistribution %q: %v", field, err)
	}
	return i, d
}

func mustNumeric(t *testing.T, idxM *index.Manager, a *datacoreStatIndex, field string, col *int) ([]float64, []float64) {
	t.Helper()
	i, _ := idxM.NumericValues("basic.yaml", field, col)
	d, err := a.NumericValues("basic.yaml", field, col)
	if err != nil {
		t.Fatalf("NumericValues %q: %v", field, err)
	}
	return i, d
}

func assertCrossEqual(t *testing.T, label string, idx, dc []index.CrossCell) {
	t.Helper()
	want := map[[2]string]int{}
	for _, c := range idx {
		want[[2]string{c.A, c.B}] = c.Count
	}
	got := map[[2]string]int{}
	for _, c := range dc {
		got[[2]string{c.A, c.B}] = c.Count
	}
	if len(want) != len(got) {
		t.Fatalf("%s: cross cell sets differ: index=%v datacore=%v", label, want, got)
	}
	for k, n := range want {
		if got[k] != n {
			t.Fatalf("%s: cross cell %v index=%d datacore=%d", label, k, n, got[k])
		}
	}
}

// Layer 5: concurrency. Fire many stat queries through the datacore-backed
// manager at once. Each call builds its own tensor and reads the index for
// nothing shared mutably, so under -race this proves the engine is safe for the
// real concurrent-request pattern; results are checked against the index's
// answer computed up front.
func TestStatProperty_ConcurrentDatacoreManager(t *testing.T) {
	idxMgr, dcMgr := twoStatManagers(t)
	queries := []string{
		`count() by F["status"]`,
		`count() by Facet["tier"]`,
		`sum(F["amount"]), avg(F["amount"]) by Facet["tier"]`,
		`count() by F["due"]@month`,
		`count() by F["items"]["cost"]`,
	}
	want := make(map[string]*stat.Grid, len(queries))
	for _, q := range queries {
		g, err := idxMgr.EvaluateDSL("basic.yaml", q)
		if err != nil {
			t.Fatalf("index %q: %v", q, err)
		}
		want[q] = g
	}

	const workers = 32
	errs := make(chan error, workers)
	for w := range workers {
		q := queries[w%len(queries)]
		go func(q string) {
			g, err := dcMgr.EvaluateDSL("basic.yaml", q)
			if err != nil {
				errs <- fmt.Errorf("%q: %w", q, err)
				return
			}
			errs <- gridsEqual(want[q], g)
		}(q)
	}
	for range workers {
		if err := <-errs; err != nil {
			t.Fatalf("concurrent datacore query: %v", err)
		}
	}
}
