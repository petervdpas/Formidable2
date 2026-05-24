package stat

// Service is the Wails-bound facade for the stat module. Vue calls
// these to drive the statistics dialog; each returns a chart-neutral
// Result the frontend (or a plugin) shapes into a chart spec. The
// service stays thin - all aggregation lives on Manager and, beneath
// it, the index module's SQL.
//
// col is the table-column index for table-field stats; pass nil for a
// scalar field. percentile is in [0,100]; pass nil to skip it. Wails
// surfaces both as `number | null` on the TypeScript side.
type Service struct{ m *Manager }

// NewService wraps a Manager.
func NewService(m *Manager) *Service { return &Service{m: m} }

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
