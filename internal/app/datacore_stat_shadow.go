package app

import (
	"fmt"
	"log/slog"
	"math"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/petervdpas/formidable2/internal/modules/datacore"
	"github.com/petervdpas/formidable2/internal/modules/index"
	"github.com/petervdpas/formidable2/internal/modules/stat"
	"github.com/petervdpas/formidable2/internal/modules/template"
)

// Stat engine modes (config.StatEngine). index is authoritative and unchanged;
// shadow runs datacore alongside the index and logs divergences without
// changing results; datacore makes the tensor authoritative. Unknown values
// resolve to index.
const (
	statEngineIndex    = "index"
	statEngineShadow   = "shadow"
	statEngineDatacore = "datacore"
)

// chooseStatIndex picks the stat.Index implementation for the configured mode.
// It is the one wiring decision the flag controls; everything downstream of
// stat.NewManager is identical regardless of which engine computes.
func chooseStatIndex(mode string, idxM *index.Manager, dc *datacore.Service, tpl *template.Manager, log *slog.Logger) (stat.Index, string) {
	cols := templateColumnNamer{tpl: tpl}
	switch mode {
	case statEngineDatacore:
		return newDatacoreStatIndex(dc, cols), statEngineDatacore
	case statEngineShadow:
		return newShadowStatIndex(idxM, newDatacoreStatIndex(dc, cols), log), statEngineShadow
	default:
		return idxM, statEngineIndex
	}
}

// shadowStatIndex runs the datacore adapter alongside the index for every
// stat.Index call, returns the INDEX result (authoritative, behavior unchanged),
// and logs any place the two disagree beyond the settled facet "(unset)"
// divergence. It is the verification step before making datacore authoritative:
// real templates exercise paths no fixture will, and the log tail tells you
// whether the flip is safe. It doubles the work per call (both engines run), so
// it is a temporary diagnostic mode, not a destination.
type shadowStatIndex struct {
	primary stat.Index // index, authoritative result
	shadow  stat.Index // datacore, compared only
	log     *slog.Logger
}

func newShadowStatIndex(primary, shadow stat.Index, log *slog.Logger) *shadowStatIndex {
	if log == nil {
		log = slog.Default()
	}
	return &shadowStatIndex{primary: primary, shadow: shadow, log: log}
}

func (s *shadowStatIndex) report(method, template, detail string) {
	s.log.Warn("stat shadow divergence", "method", method, "template", template, "detail", detail)
}

// shadowErr logs when the datacore side fails a call the index served, since a
// shadow that errors is itself a divergence worth seeing.
func (s *shadowStatIndex) shadowErr(method, template string, err error) {
	s.log.Warn("stat shadow error", "method", method, "template", template, "err", err.Error())
}

func (s *shadowStatIndex) TotalForms(t string) (int, error) {
	got, err := s.primary.TotalForms(t)
	if err == nil {
		if sg, serr := s.shadow.TotalForms(t); serr != nil {
			s.shadowErr("TotalForms", t, serr)
		} else if sg != got {
			s.report("TotalForms", t, fmt.Sprintf("index=%d datacore=%d", got, sg))
		}
	}
	return got, err
}

func (s *shadowStatIndex) ValueDistribution(t, key string, col *int) ([]index.Bucket, error) {
	got, err := s.primary.ValueDistribution(t, key, col)
	if err == nil {
		if sg, serr := s.shadow.ValueDistribution(t, key, col); serr != nil {
			s.shadowErr("ValueDistribution", t, serr)
		} else if d := diffBuckets(got, sg, false); d != "" {
			s.report("ValueDistribution["+key+"]", t, d)
		}
	}
	return got, err
}

func (s *shadowStatIndex) NumericValues(t, key string, col *int) ([]float64, error) {
	got, err := s.primary.NumericValues(t, key, col)
	if err == nil {
		if sg, serr := s.shadow.NumericValues(t, key, col); serr != nil {
			s.shadowErr("NumericValues", t, serr)
		} else if d := diffFloats(got, sg); d != "" {
			s.report("NumericValues["+key+"]", t, d)
		}
	}
	return got, err
}

// FacetDistribution drops the "" (unset) bucket before comparing: that is the
// settled, intended divergence, not something to log on every chart.
func (s *shadowStatIndex) FacetDistribution(t, key string) ([]index.Bucket, error) {
	got, err := s.primary.FacetDistribution(t, key)
	if err == nil {
		if sg, serr := s.shadow.FacetDistribution(t, key); serr != nil {
			s.shadowErr("FacetDistribution", t, serr)
		} else if d := diffBuckets(got, sg, true); d != "" {
			s.report("FacetDistribution["+key+"]", t, d)
		}
	}
	return got, err
}

// FacetCross drops any cell with a "" axis value (the unset divergence) before
// comparing.
func (s *shadowStatIndex) FacetCross(t, a, b string) ([]index.CrossCell, error) {
	got, err := s.primary.FacetCross(t, a, b)
	if err == nil {
		if sg, serr := s.shadow.FacetCross(t, a, b); serr != nil {
			s.shadowErr("FacetCross", t, serr)
		} else if d := diffCross(got, sg); d != "" {
			s.report("FacetCross["+a+"x"+b+"]", t, d)
		}
	}
	return got, err
}

func (s *shadowStatIndex) DateSeries(t, key string, col *int, period string) ([]index.Bucket, error) {
	got, err := s.primary.DateSeries(t, key, col, period)
	if err == nil {
		if sg, serr := s.shadow.DateSeries(t, key, col, period); serr != nil {
			s.shadowErr("DateSeries", t, serr)
		} else if d := diffBuckets(got, sg, false); d != "" {
			s.report("DateSeries["+key+"@"+period+"]", t, d)
		}
	}
	return got, err
}

// AggregateRaw drops rows carrying a "" dimension value before comparing: a
// blank dim only arises from a set-but-unselected facet (field dims are
// complete-case in both engines), so this is the same settled divergence.
func (s *shadowStatIndex) AggregateRaw(t string, dims []index.AggDim, nums []index.AggNum, filters []index.AggFilter) ([]index.StatRawRow, error) {
	got, err := s.primary.AggregateRaw(t, dims, nums, filters)
	if err == nil {
		if sg, serr := s.shadow.AggregateRaw(t, dims, nums, filters); serr != nil {
			s.shadowErr("AggregateRaw", t, serr)
		} else if d := diffRaw(got, sg); d != "" {
			s.report("AggregateRaw", t, d)
		}
	}
	return got, err
}

// --- comparison helpers: return "" when equal, else a short divergence note ---

func diffBuckets(idxB, dcB []index.Bucket, dropEmpty bool) string {
	im := bucketCounts(idxB, dropEmpty)
	dm := bucketCounts(dcB, dropEmpty)
	return diffIntMaps(im, dm)
}

func bucketCounts(bs []index.Bucket, dropEmpty bool) map[string]int {
	m := make(map[string]int, len(bs))
	for _, b := range bs {
		if dropEmpty && b.Label == "" {
			continue
		}
		m[b.Label] = b.Count
	}
	return m
}

func diffCross(idxC, dcC []index.CrossCell) string {
	im := map[string]int{}
	for _, c := range idxC {
		if c.A == "" || c.B == "" {
			continue
		}
		im[c.A+"\x00"+c.B] = c.Count
	}
	dm := map[string]int{}
	for _, c := range dcC {
		if c.A == "" || c.B == "" {
			continue
		}
		dm[c.A+"\x00"+c.B] = c.Count
	}
	return diffIntMaps(im, dm)
}

func diffIntMaps(a, b map[string]int) string {
	if len(a) != len(b) {
		return fmt.Sprintf("category count index=%d datacore=%d", len(a), len(b))
	}
	for k, n := range a {
		if b[k] != n {
			return fmt.Sprintf("category %q index=%d datacore=%d", k, n, b[k])
		}
	}
	return ""
}

func diffFloats(idx, dc []float64) string {
	a := append([]float64(nil), idx...)
	b := append([]float64(nil), dc...)
	sort.Float64s(a)
	sort.Float64s(b)
	if len(a) != len(b) {
		return fmt.Sprintf("value count index=%d datacore=%d", len(a), len(b))
	}
	for i := range a {
		if math.Abs(a[i]-b[i]) > 1e-9 {
			return fmt.Sprintf("value[%d] index=%g datacore=%g", i, a[i], b[i])
		}
	}
	return ""
}

func diffRaw(idx, dc []index.StatRawRow) string {
	a := rawKeys(idx)
	b := rawKeys(dc)
	if len(a) != len(b) {
		return fmt.Sprintf("row count index=%d datacore=%d", len(a), len(b))
	}
	for i := range a {
		if a[i] != b[i] {
			return fmt.Sprintf("row index=%q datacore=%q", a[i], b[i])
		}
	}
	return ""
}

// rawKeys renders rows to a sorted comparable form, dropping any row with a
// blank dimension (the set-but-unselected facet divergence).
func rawKeys(rows []index.StatRawRow) []string {
	out := make([]string, 0, len(rows))
	for _, r := range rows {
		if slices.Contains(r.Dims, "") {
			continue
		}
		var b strings.Builder
		b.WriteString(r.Form)
		b.WriteByte('|')
		b.WriteString(strings.Join(r.Dims, ","))
		b.WriteByte('|')
		for _, n := range r.Nums {
			if n.Valid {
				b.WriteString(strconv.FormatFloat(n.Float64, 'f', -1, 64))
			} else {
				b.WriteByte('_')
			}
			b.WriteByte(';')
		}
		out = append(out, b.String())
	}
	sort.Strings(out)
	return out
}
