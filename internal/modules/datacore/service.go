package datacore

// LoaderFactory builds a Loader for one template. The composition root wires
// it to whatever holds the live data (template + storage), so the service
// stays dependency-free.
type LoaderFactory func(template string) Loader

// Service is the Wails-facing, read-only layer over the tensor. Each call
// builds a fresh tensor from the template's live forms and runs one
// perspective over it. It is purely additive: it reads form data the same way
// query does and changes nothing the index or stat path relies on.
//
// The optional follow argument steps one reference hop before reducing, so a
// table or loop column is reachable (follow the loop field, then reduce its
// rows) without a full spec language. Empty follow reduces over the root
// records.
type Service struct {
	factory LoaderFactory
}

func NewService(factory LoaderFactory) *Service { return &Service{factory: factory} }

func (s *Service) view(template, follow string) (*Perspective, error) {
	t, err := Build(s.factory(template))
	if err != nil {
		return nil, err
	}
	v := t.View()
	if follow != "" {
		v = v.Follow(follow)
	}
	return v, nil
}

// Count is the number of records (follow empty) or loop rows (follow set).
func (s *Service) Count(template, follow string) (int, error) {
	v, err := s.view(template, follow)
	if err != nil {
		return 0, err
	}
	return v.Count(), nil
}

// Distribution is the rank-1 marginal of field over the (optionally followed)
// working set.
func (s *Service) Distribution(template, follow, field string) ([]Bucket, error) {
	v, err := s.view(template, follow)
	if err != nil {
		return nil, err
	}
	return v.Distribution(field), nil
}

// Aggregate is the numeric reduction of field (sum/min/max/mean + anomalies),
// coercing on demand.
func (s *Service) Aggregate(template, follow, field string) (Aggregate, error) {
	v, err := s.view(template, follow)
	if err != nil {
		return Aggregate{}, err
	}
	return v.Aggregate(field), nil
}

// Cross is the rank-2 contingency of two fields over the working set.
func (s *Service) Cross(template, follow, rowField, colField string) (CrossTab, error) {
	v, err := s.view(template, follow)
	if err != nil {
		return CrossTab{}, err
	}
	return v.Cross(rowField, colField), nil
}

// DateSeries is the date histogram of field over the (optionally followed)
// working set, bucketed by period (year/month/day).
func (s *Service) DateSeries(template, follow, field, period string) (Series, error) {
	v, err := s.view(template, follow)
	if err != nil {
		return Series{}, err
	}
	return v.DateSeries(field, period), nil
}

// Graph projects the template's tensor as a node-link graph (records and loop
// rows as nodes, refs as edges) for the visual explorer. limit caps the node
// count (0 = no cap); roots are kept before rows and dangling edges dropped.
func (s *Service) Graph(template string, limit int) (Graph, error) {
	t, err := Build(s.factory(template))
	if err != nil {
		return Graph{}, err
	}
	return t.Graph(limit), nil
}

// GraphFrom projects the subgraph reachable from one record (rootID, a node
// id) up to depth hops, for the per-record flower and click-to-unfold.
func (s *Service) GraphFrom(template, rootID string, depth int) (Graph, error) {
	t, err := Build(s.factory(template))
	if err != nil {
		return Graph{}, err
	}
	return t.GraphFrom(rootID, depth), nil
}

// AggregateRaw produces the raw grid rows (form + dim values + numeric
// measures) that feed statistical aggregation, over root fields, facets, and
// date-bucketed dims. The caller groups and reduces.
func (s *Service) AggregateRaw(template string, dims []GridDim, nums []GridNum, filters []GridFilter) ([]GridRow, error) {
	t, err := Build(s.factory(template))
	if err != nil {
		return nil, err
	}
	return t.View().Grid(dims, nums, filters), nil
}

// Summarize lands a per-record loop summary on each root: linkField's rows
// reduced by valueField (empty valueField gives a plain count summary).
func (s *Service) Summarize(template, linkField, valueField string) ([]RootSummary, error) {
	t, err := Build(s.factory(template))
	if err != nil {
		return nil, err
	}
	return t.View().Summarize(linkField, valueField), nil
}
