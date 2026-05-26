package stat

import "fmt"

// StatObject is a named statistical object defined on a template: its
// identifier, optional human label, and either a DSL (a plain object the
// engine evaluates) or a Composite spec (a hop route referencing other
// objects by name). Exactly one of DSL / Composite is set. Mirrors
// template.Statistic without the template dependency, so the catalog can
// travel to Vue and Lua via the Service.
type StatObject struct {
	Name      string         `json:"name"`
	Label     string         `json:"label,omitempty"`
	DSL       string         `json:"dsl"`
	Composite *CompositeSpec `json:"composite,omitempty"`
}

// CompositeSpec is the stored form of a composite (hop route): a parent
// object name plus per-branch child object names. Resolved against the
// template's other objects by ResolveComposite. Kept name-based (not
// inlined configs) so the parent and children stay single sources of truth.
type CompositeSpec struct {
	Parent string              `json:"parent"`
	Edges  []CompositeEdgeSpec `json:"edges"`
}

// CompositeEdgeSpec maps one parent branch value to the child object that
// drills it.
type CompositeEdgeSpec struct {
	Branch string `json:"branch"`
	Child  string `json:"child"`
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

// Distribution counts forms by a field's (or table column's) value, as
// a rank-1 Grid.
func (s *Service) Distribution(template, fieldKey string, col *int) (*Grid, error) {
	return s.m.Distribution(template, fieldKey, col)
}

// FacetDistribution counts set forms by a facet's selected option.
func (s *Service) FacetDistribution(template, facetKey string) (*Grid, error) {
	return s.m.FacetDistribution(template, facetKey)
}

// CrossTab is the pair-combination matrix between two facets (rank-2).
func (s *Service) CrossTab(template, keyA, keyB string) (*Grid, error) {
	return s.m.CrossTab(template, keyA, keyB)
}

// NumericStats reduces a numeric field / table column to summary
// measures (min/max/sum/avg/median/stddev[/percentile]) as a rank-0 Grid.
func (s *Service) NumericStats(template, fieldKey string, col *int, percentile *float64) (*Grid, error) {
	return s.m.NumericStats(template, fieldKey, col, percentile)
}

// TimeSeries buckets a date field / column by period ("year" | "month"
// | "day") and counts forms per bucket, as a rank-1 Grid.
func (s *Service) TimeSeries(template, fieldKey string, col *int, period string) (*Grid, error) {
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

// CompositeOptions reports the composites (hop routes) buildable from the
// template's named objects: each rank-1 parent and, per branch, the existing
// children that can drill it. The builder renders only these, so the structure
// gates what the author can wire. Backend steers the frontend.
func (s *Service) CompositeOptions(template string) ([]CompositeOption, error) {
	if s.src == nil {
		return nil, fmt.Errorf("stat: no statistic source configured")
	}
	objs, err := s.src.ListStatistics(template)
	if err != nil {
		return nil, err
	}
	return CompositeOptions(objs), nil
}

// EvaluateComposite resolves a template's named composite object (a hop
// route) and evaluates it: the parent rank-1 grid plus a child grid per
// drilled branch (nil for solid leaves). Errors if the name is unknown or
// is not a composite. This is the surface the sunburst/drill renderer
// consumes; plain objects use EvaluateObject.
func (s *Service) EvaluateComposite(template, name string) (*CompositeGrid, error) {
	if s.src == nil {
		return nil, fmt.Errorf("stat: no statistic source configured")
	}
	objs, err := s.src.ListStatistics(template)
	if err != nil {
		return nil, err
	}
	catalog := make(catalogConfigs, len(objs))
	for _, o := range objs {
		catalog[o.Name] = o
	}
	obj, ok := catalog[name]
	if !ok {
		return nil, fmt.Errorf("stat: no statistic %q on template %q", name, template)
	}
	if obj.Composite == nil {
		return nil, fmt.Errorf("stat: statistic %q is not a composite", name)
	}
	comp, err := ResolveComposite(*obj.Composite, catalog)
	if err != nil {
		return nil, err
	}
	return s.m.EvaluateComposite(template, comp)
}

// EvaluateCompositeSpec evaluates an inline composite spec against the
// template's saved objects (the parent and children, referenced by name,
// must already exist). The builder uses it to preview a composite before the
// composite object itself is saved, mirroring EvaluateDSL for plain objects.
func (s *Service) EvaluateCompositeSpec(template string, spec CompositeSpec) (*CompositeGrid, error) {
	if s.src == nil {
		return nil, fmt.Errorf("stat: no statistic source configured")
	}
	objs, err := s.src.ListStatistics(template)
	if err != nil {
		return nil, err
	}
	catalog := make(catalogConfigs, len(objs))
	for _, o := range objs {
		catalog[o.Name] = o
	}
	comp, err := ResolveComposite(spec, catalog)
	if err != nil {
		return nil, err
	}
	return s.m.EvaluateComposite(template, comp)
}

// catalogConfigs adapts a template's object catalog (name -> object) to the
// ObjectConfigs the composite resolver needs: it parses a plain object's DSL
// and rejects unknown names and composites (no nesting).
type catalogConfigs map[string]StatObject

func (c catalogConfigs) Config(name string) (StatConfig, error) {
	o, ok := c[name]
	if !ok {
		return StatConfig{}, fmt.Errorf("object %q not found", name)
	}
	if o.Composite != nil {
		return StatConfig{}, fmt.Errorf("object %q is a composite and cannot be nested", name)
	}
	return Parse(o.DSL)
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
