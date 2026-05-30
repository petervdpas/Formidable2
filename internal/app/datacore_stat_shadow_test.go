package app

import (
	"context"
	"log/slog"
	"testing"

	"github.com/petervdpas/formidable2/internal/modules/datacore"
	"github.com/petervdpas/formidable2/internal/modules/index"
	"github.com/petervdpas/formidable2/internal/modules/stat"
)

// capturingHandler records the divergence/error log lines the shadow wrapper
// emits, so a test can assert exactly what was (and was not) logged.
type capturingHandler struct{ msgs []string }

func (h *capturingHandler) Enabled(context.Context, slog.Level) bool { return true }
func (h *capturingHandler) Handle(_ context.Context, r slog.Record) error {
	h.msgs = append(h.msgs, r.Message)
	return nil
}
func (h *capturingHandler) WithAttrs([]slog.Attr) slog.Handler { return h }
func (h *capturingHandler) WithGroup(string) slog.Handler       { return h }

// stubStatIndex is a fixed-return stat.Index for testing the shadow wrapper's
// compare-and-log logic without involving real engines.
type stubStatIndex struct {
	total   int
	buckets []index.Bucket
}

func (s stubStatIndex) TotalForms(string) (int, error) { return s.total, nil }
func (s stubStatIndex) ValueDistribution(string, string, *int) ([]index.Bucket, error) {
	return s.buckets, nil
}
func (s stubStatIndex) NumericValues(string, string, *int) ([]float64, error) { return nil, nil }
func (s stubStatIndex) FacetDistribution(string, string) ([]index.Bucket, error) {
	return s.buckets, nil
}
func (s stubStatIndex) FacetCross(string, string, string) ([]index.CrossCell, error) { return nil, nil }
func (s stubStatIndex) DateSeries(string, string, *int, string) ([]index.Bucket, error) {
	return nil, nil
}
func (s stubStatIndex) AggregateRaw(string, []index.AggDim, []index.AggNum, []index.AggFilter) ([]index.StatRawRow, error) {
	return nil, nil
}

func divergenceCount(h *capturingHandler) int {
	n := 0
	for _, m := range h.msgs {
		if m == "stat shadow divergence" || m == "stat shadow error" {
			n++
		}
	}
	return n
}

func TestShadow_ReturnsPrimaryAndLogsRealDivergence(t *testing.T) {
	h := &capturingHandler{}
	primary := stubStatIndex{total: 3, buckets: []index.Bucket{{Label: "A", Count: 2}, {Label: "B", Count: 1}}}
	shadow := stubStatIndex{total: 5, buckets: []index.Bucket{{Label: "A", Count: 2}, {Label: "B", Count: 9}}}
	s := newShadowStatIndex(primary, shadow, slog.New(h))

	// The wrapper always returns the primary (index) result.
	if n, _ := s.TotalForms("t"); n != 3 {
		t.Fatalf("TotalForms returned %d, want primary 3", n)
	}
	got, _ := s.ValueDistribution("t", "x", nil)
	if len(got) != 2 || got[1].Count != 1 {
		t.Fatalf("ValueDistribution returned %v, want primary", got)
	}
	// Both calls diverged, so both should have logged.
	if divergenceCount(h) != 2 {
		t.Fatalf("divergence logs = %d, want 2 (total + distribution); msgs=%v", divergenceCount(h), h.msgs)
	}
}

func TestShadow_QuietWhenEnginesAgree(t *testing.T) {
	h := &capturingHandler{}
	same := []index.Bucket{{Label: "A", Count: 2}}
	s := newShadowStatIndex(stubStatIndex{total: 1, buckets: same}, stubStatIndex{total: 1, buckets: same}, slog.New(h))
	s.TotalForms("t")
	s.ValueDistribution("t", "x", nil)
	if divergenceCount(h) != 0 {
		t.Fatalf("agreeing engines logged %d divergences: %v", divergenceCount(h), h.msgs)
	}
}

// TestShadow_WhitelistsFacetUnset proves the settled "(unset)" divergence is not
// logged: the primary (index) carries an "" bucket the shadow drops, and that
// difference alone must stay quiet.
func TestShadow_WhitelistsFacetUnset(t *testing.T) {
	h := &capturingHandler{}
	primary := stubStatIndex{buckets: []index.Bucket{{Label: "", Count: 2}, {Label: "live", Count: 1}}}
	shadow := stubStatIndex{buckets: []index.Bucket{{Label: "live", Count: 1}}}
	s := newShadowStatIndex(primary, shadow, slog.New(h))

	if _, err := s.FacetDistribution("t", "stage"); err != nil {
		t.Fatalf("FacetDistribution: %v", err)
	}
	if divergenceCount(h) != 0 {
		t.Fatalf("facet (unset) divergence was logged but is whitelisted: %v", h.msgs)
	}
}

// TestShadow_RealEnginesNoUnexpectedDivergence runs the shadow over the real
// index and datacore adapter on a fixture that includes a set-but-unselected
// facet. The only difference between the engines is the whitelisted "(unset)"
// bucket, so a full sweep of Index calls must log nothing.
func TestShadow_RealEnginesNoUnexpectedDivergence(t *testing.T) {
	forms := []statForm{
		{id: "a.meta.json", text: map[string]string{"status": "high"}, num: map[string]string{"amount": "10"}, date: map[string]string{"due": "2026-01-15"}, facets: map[string]string{"tier": "GOLD", "stage": "live"}, costs: []string{"100", "50"}},
		{id: "b.meta.json", text: map[string]string{"status": "low"}, num: map[string]string{"amount": "30"}, date: map[string]string{"due": "2026-02-20"}, facets: map[string]string{"tier": "GOLD", "stage": ""}},   // set, unselected
		{id: "c.meta.json", text: map[string]string{"status": "high"}, num: map[string]string{"amount": "oops"}, date: map[string]string{"due": "2027-03-05"}, facets: map[string]string{"tier": "SILVER", "stage": ""}}, // set, unselected
	}
	adapter, idxM := newDatacoreStatAdapter(t, forms)

	h := &capturingHandler{}
	s := newShadowStatIndex(idxM, adapter, slog.New(h))
	col0 := 0

	s.TotalForms("basic.yaml")
	s.ValueDistribution("basic.yaml", "status", nil)
	s.ValueDistribution("basic.yaml", "items", &col0)
	s.NumericValues("basic.yaml", "amount", nil)
	s.NumericValues("basic.yaml", "items", &col0)
	s.FacetDistribution("basic.yaml", "tier")
	s.FacetDistribution("basic.yaml", "stage") // the unset case
	s.FacetCross("basic.yaml", "tier", "stage")
	for _, p := range []string{"year", "month", "day"} {
		s.DateSeries("basic.yaml", "due", nil, p)
	}
	s.AggregateRaw("basic.yaml",
		[]index.AggDim{{Kind: "field", Key: "status"}, {Kind: "facet", Key: "stage"}},
		[]index.AggNum{{Key: "amount"}}, nil)

	if divergenceCount(h) != 0 {
		t.Fatalf("real-engine shadow logged unexpected divergences: %v", h.msgs)
	}
}

func TestChooseStatIndex_Modes(t *testing.T) {
	idxM, err := index.NewManager(t.TempDir() + "/x.db")
	if err != nil {
		t.Fatalf("index.NewManager: %v", err)
	}
	t.Cleanup(func() { idxM.Close() })
	dc := datacore.NewService(func(string) datacore.Loader { return dcRecordLoader{} })

	cases := map[string]string{
		statEngineIndex:    statEngineIndex,
		statEngineDatacore: statEngineDatacore,
		statEngineShadow:   statEngineShadow,
		"garbage":          statEngineIndex, // unknown falls back
		"":                 statEngineIndex,
	}
	for mode, wantEngine := range cases {
		impl, engine := chooseStatIndex(mode, idxM, dc, nil, slog.Default())
		if engine != wantEngine {
			t.Fatalf("mode %q: engine = %q, want %q", mode, engine, wantEngine)
		}
		if impl == nil {
			t.Fatalf("mode %q: nil stat.Index", mode)
		}
	}

	// index mode returns the index manager itself (the unchanged default path).
	if impl, _ := chooseStatIndex(statEngineIndex, idxM, dc, nil, slog.Default()); impl != stat.Index(idxM) {
		t.Fatal("index mode must return the index manager unchanged")
	}
}
