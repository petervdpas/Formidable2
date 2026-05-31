package stat

import "fmt"

// StatObject is a named statistical object defined on a template: a DSL (a
// plain object the engine evaluates), a Composite spec (a hop route), or a
// Scaling. Exactly one is set. Mirrors template.Statistic without the template
// dependency so the catalog can travel to Vue and Lua via the Service.
type StatObject struct {
	Name      string         `json:"name"`
	Label     string         `json:"label,omitempty"`
	DSL       string         `json:"dsl"`
	Composite *CompositeSpec `json:"composite,omitempty"`
	// Scaling is set when this object is a scaling (a reusable weighting),
	// in which case DSL is empty. Other objects reference it by name through
	// their DSL `scale "<name>"` clause.
	Scaling *Scaling `json:"scaling,omitempty"`
}

// CompositeSpec is the stored form of a composite (hop route): a parent
// object name plus per-branch child object names. Name-based (not inlined
// configs) so parent and children stay single sources of truth.
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

// StatisticSource resolves a template's named statistical objects.
type StatisticSource interface {
	StatisticDSL(template, name string) (dsl string, ok bool, err error)
	// ListStatistics returns every named statistical object on the
	// template in definition order. Empty when the template has none;
	// error only on a load failure.
	ListStatistics(template string) ([]StatObject, error)
}

// Service is the Wails-bound facade for the stat module. Each method
// returns a chart-neutral Grid the frontend (or a plugin) shapes into a
// chart spec; all aggregation lives on Manager and the index module's SQL.
// Throughout, col is the table-column index (nil for a scalar field) and
// percentile is in [0,100] (nil to skip).
type Service struct {
	m   *Manager
	src StatisticSource
}

// NewService wraps a Manager and a statistic-name resolver; src may be nil
// when name resolution isn't needed.
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
// string (called by the builder dialog on save).
func (s *Service) CompileDSL(cfg StatConfig) (string, error) {
	return Compile(cfg)
}

// ParseDSL turns a stored statistical-DSL string back into a StatConfig so
// the builder can re-open it. Strict: an unrecognised string errors rather
// than silently misreading.
func (s *Service) ParseDSL(dsl string) (StatConfig, error) {
	return Parse(dsl)
}

// BuilderMeasureOps is the measure catalog (op + input rules) the builder
// renders its op picker from.
func (s *Service) BuilderMeasureOps() []MeasureOpDescriptor { return MeasureOps() }

// BuilderBins is the date-bin catalog for the dimension binning picker.
func (s *Service) BuilderBins() []Bin { return Bins() }

// BuilderPercentBases is the percentage-denominator catalog for the builder's
// percent-base picker (distribution / forms / none).
func (s *Service) BuilderPercentBases() []PercentBase { return PercentBases() }

// BuilderFilterOps is the where-clause operator catalog for the filter
// picker (op + whether its value is numeric).
func (s *Service) BuilderFilterOps() []FilterOpDescriptor { return FilterOps() }

// EvaluateDSL evaluates a raw statistical-DSL string against the index, for
// previewing a statistic before it is saved. A `scale "<name>"` clause is
// resolved against the template's saved objects, so the referenced scaling
// must already exist.
func (s *Service) EvaluateDSL(template, dsl string) (*Grid, error) {
	cfg, err := Parse(dsl)
	if err != nil {
		return nil, err
	}
	if cfg.Scale == "" {
		return s.m.Evaluate(template, cfg)
	}
	catalog, err := s.loadCatalog(template)
	if err != nil {
		return nil, err
	}
	return s.evaluateResolved(template, cfg, catalog)
}

// ListObjects returns the catalog of named statistical objects defined on a
// template. Callers then invoke EvaluateObject(name) to run one.
func (s *Service) ListObjects(template string) ([]StatObject, error) {
	if s.src == nil {
		return nil, fmt.Errorf("stat: no statistic source configured")
	}
	return s.src.ListStatistics(template)
}

// CompositeOptions reports the composites (hop routes) buildable from the
// template's named objects: each rank-1 parent and, per branch, the existing
// children that can drill it. The builder renders only these.
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

// EvaluateComposite resolves a template's named composite object and
// evaluates it: the parent rank-1 grid plus a child grid per drilled branch
// (nil for solid leaves). Plain objects use EvaluateObject instead.
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
// template's saved objects (referenced parent and children must already
// exist), for previewing a composite before it is saved.
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

// catalogConfigs adapts a template's object catalog to the ObjectConfigs the
// composite resolver needs: it parses a plain object's DSL and rejects
// unknown names and composites (no nesting).
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

func (c catalogConfigs) Scaling(name string) (*Scaling, error) {
	o, ok := c[name]
	if !ok {
		return nil, fmt.Errorf("scaling %q not found", name)
	}
	if o.Scaling == nil {
		return nil, fmt.Errorf("object %q is not a scaling", name)
	}
	return o.Scaling, nil
}

// evaluateResolved is the single path behind EvaluateObject and the builder
// preview, resolving an optional scale-clause reference through the catalog
// so both apply weighting the same way.
func (s *Service) evaluateResolved(template string, cfg StatConfig, catalog catalogConfigs) (*Grid, error) {
	if cfg.Scale == "" {
		return s.m.Evaluate(template, cfg)
	}
	sc, err := catalog.Scaling(cfg.Scale)
	if err != nil {
		return nil, err
	}
	return s.m.EvaluateScaled(template, cfg, sc)
}

// loadCatalog reads the template's objects into a name->object catalog.
func (s *Service) loadCatalog(template string) (catalogConfigs, error) {
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
	return catalog, nil
}

// EvaluateObject resolves a template's named statistical object to its
// DSL, evaluates it against the index, and returns the rank-N Grid. This
// is the surface the frontend renderer and the Lua binding consume.
func (s *Service) EvaluateObject(template, name string) (*Grid, error) {
	catalog, err := s.loadCatalog(template)
	if err != nil {
		return nil, err
	}
	obj, ok := catalog[name]
	if !ok {
		return nil, fmt.Errorf("stat: no statistic %q on template %q", name, template)
	}
	if obj.Composite != nil {
		return nil, fmt.Errorf("stat: statistic %q is a composite; use EvaluateComposite", name)
	}
	if obj.Scaling != nil {
		return nil, fmt.Errorf("stat: statistic %q is a scaling and has no grid of its own", name)
	}
	cfg, err := Parse(obj.DSL)
	if err != nil {
		return nil, err
	}
	return s.evaluateResolved(template, cfg, catalog)
}
