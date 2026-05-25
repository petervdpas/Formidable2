package stat

import "fmt"

// StatObject is a named statistical object defined on a template: its
// identifier, optional human label, and the DSL the engine evaluates.
// Mirrors template.Statistic without the template dependency, so the
// catalog can travel to Vue and Lua via the Service.
type StatObject struct {
	Name  string `json:"name"`
	Label string `json:"label,omitempty"`
	DSL   string `json:"dsl"`
}

// StatisticSource resolves a template's named statistical objects. The
// app implements it over the template manager; keeping it an interface
// lets the stat package stay free of a template dependency.
type StatisticSource interface {
	StatisticDSL(template, name string) (dsl string, ok bool, err error)
	// ListStatistics returns every named statistical object on the
	// template in definition order. Empty when the template has none;
	// error only on a load failure.
	ListStatistics(template string) ([]StatObject, error)
}

// Service is the Wails-bound facade for the stat module. Vue calls
// these to drive the statistics dialog; each returns a chart-neutral
// Result the frontend (or a plugin) shapes into a chart spec. The
// service stays thin - all aggregation lives on Manager and, beneath
// it, the index module's SQL.
//
// col is the table-column index for table-field stats; pass nil for a
// scalar field. percentile is in [0,100]; pass nil to skip it. Wails
// surfaces both as `number | null` on the TypeScript side.
type Service struct {
	m   *Manager
	src StatisticSource
}

// NewService wraps a Manager and a statistic-name resolver (for
// EvaluateObject). src may be nil if name resolution isn't needed.
func NewService(m *Manager, src StatisticSource) *Service {
	return &Service{m: m, src: src}
}

// Distribution counts forms by a field's (or table column's) value.
func (s *Service) Distribution(template, fieldKey string, col *int) (*Result, error) {
	return s.m.Distribution(template, fieldKey, col)
}

// FacetDistribution counts set forms by a facet's selected option.
func (s *Service) FacetDistribution(template, facetKey string) (*Result, error) {
	return s.m.FacetDistribution(template, facetKey)
}

// CrossTab is the pair-combination matrix between two facets.
func (s *Service) CrossTab(template, keyA, keyB string) (*Result, error) {
	return s.m.CrossTab(template, keyA, keyB)
}

// NumericStats reduces a numeric field / table column to summary
// scalars (min/max/sum/avg/median/stddev[/percentile]).
func (s *Service) NumericStats(template, fieldKey string, col *int, percentile *float64) (*Result, error) {
	return s.m.NumericStats(template, fieldKey, col, percentile)
}

// TimeSeries buckets a date field / column by period ("year" | "month"
// | "day") and counts forms per bucket.
func (s *Service) TimeSeries(template, fieldKey string, col *int, period string) (*Result, error) {
	return s.m.TimeSeries(template, fieldKey, col, period)
}

// CompileDSL serializes a StatConfig into the canonical statistical-DSL
// string. The Statistical Insight builder dialog calls this on save; the
// stored string is what evaluation (step 4) later parses + runs.
func (s *Service) CompileDSL(cfg StatConfig) (string, error) {
	return Compile(cfg)
}

// ParseDSL turns a stored statistical-DSL string back into a StatConfig
// so the builder can re-open it for editing. Strict: an unrecognised
// string is an error, letting the dialog show a clean "couldn't load"
// flow rather than silently misreading.
func (s *Service) ParseDSL(dsl string) (StatConfig, error) {
	return Parse(dsl)
}

// BuilderMeasureOps is the measure catalog (op + input rules) the builder
// renders its op picker from - backend owns the vocabulary, not the UI.
func (s *Service) BuilderMeasureOps() []MeasureOpDescriptor { return MeasureOps() }

// BuilderBins is the date-bin catalog for the dimension binning picker.
func (s *Service) BuilderBins() []Bin { return Bins() }

// BuilderFilterOps is the where-clause operator catalog for the filter
// picker (op + whether its value is numeric).
func (s *Service) BuilderFilterOps() []FilterOpDescriptor { return FilterOps() }

// EvaluateDSL evaluates a raw statistical-DSL string against the index.
// The builder uses it to preview a statistic's output before it is saved
// (EvaluateObject needs the object persisted to resolve it by name).
func (s *Service) EvaluateDSL(template, dsl string) (*Grid, error) {
	return s.m.EvaluateDSL(template, dsl)
}

// ListObjects returns the catalog of named statistical objects defined
// on a template (name, label, DSL). The frontend lists them and the
// Lua binding enumerates them; either then calls EvaluateObject(name)
// to run one. DSL is exposed for display, not required for evaluation.
func (s *Service) ListObjects(template string) ([]StatObject, error) {
	if s.src == nil {
		return nil, fmt.Errorf("stat: no statistic source configured")
	}
	return s.src.ListStatistics(template)
}

// EvaluateObject resolves a template's named statistical object to its
// DSL, evaluates it against the index, and returns the rank-N Grid. This
// is the surface the frontend renderer and the Lua binding consume.
func (s *Service) EvaluateObject(template, name string) (*Grid, error) {
	if s.src == nil {
		return nil, fmt.Errorf("stat: no statistic source configured")
	}
	dsl, ok, err := s.src.StatisticDSL(template, name)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, fmt.Errorf("stat: no statistic %q on template %q", name, template)
	}
	return s.m.EvaluateDSL(template, dsl)
}
