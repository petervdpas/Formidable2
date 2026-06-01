package stat

import "fmt"

// Composite objects (hop routes): a parent statistical object whose branches
// drill into child objects. See design/statistics-composite.md. The soundness
// constraint is the filter-on-the-base match: a child may expand a parent
// branch only if it filters the parent's group-by dimension (the base) to that
// exact branch value, so the child's record set equals the branch's.

// Edge attaches one parent-branch value to a child object that drills into it.
// Scales carries the child's resolved weightings (multiplied per record) so a
// drilled branch honors the same weighting it would standalone; empty when the
// child has no scale clause.
type Edge struct {
	Branch string
	Child  StatConfig
	Scales []*Scaling
}

// Composite is a rank-1 parent plus per-branch drill edges. A branch with no
// edge is a solid leaf. ParentScales is the parent's resolved weightings,
// empty when the parent has no scale clause.
type Composite struct {
	Parent       StatConfig
	ParentScales []*Scaling
	Edges        []Edge
}

// branchDim returns the parent's single branch dimension. A composite parent
// must be rank-1 (exactly one group-by) for its categories to be branches.
func (c Composite) branchDim() (SourceRef, error) {
	if len(c.Parent.Dimensions) != 1 {
		return SourceRef{}, fmt.Errorf("stat: composite parent must have exactly one dimension, has %d", len(c.Parent.Dimensions))
	}
	return c.Parent.Dimensions[0].Source, nil
}

// Validate checks every edge against the base+filter constraint: the child
// must carry an `eq` filter on the parent's branch dimension whose value is the
// edge's branch, so a bad link is rejected here rather than charted as the
// wrong subset. Structural only; whether the branch is a real parent category
// is a data-time check in EvaluateComposite.
func (c Composite) Validate() error {
	base, err := c.branchDim()
	if err != nil {
		return err
	}
	seen := map[string]bool{}
	for _, e := range c.Edges {
		if e.Branch == "" {
			return fmt.Errorf("stat: composite edge has an empty branch")
		}
		if seen[e.Branch] {
			return fmt.Errorf("stat: composite has a duplicate edge for branch %q", e.Branch)
		}
		seen[e.Branch] = true
		if !hasBranchFilter(e.Child, base, e.Branch) {
			return fmt.Errorf("stat: composite child for branch %q must filter %s eq %q",
				e.Branch, sourceLabel(base, BinNone), e.Branch)
		}
	}
	return nil
}

// hasBranchFilter is true when child carries an eq filter on the base source
// equal to branch. Other filters on the child are allowed alongside it.
func hasBranchFilter(child StatConfig, base SourceRef, branch string) bool {
	for _, f := range child.Filters {
		if f.Op == FilterEq && f.Source == base && f.Value == branch {
			return true
		}
	}
	return false
}

// CompositeOption is one composite the builder may offer: a rank-1 parent
// object and, per branch, the existing child objects that can drill it (those
// whose DSL already filters the parent base to that branch value).
type CompositeOption struct {
	Parent string                `json:"parent"` // parent object name
	Base   string                `json:"base"`   // the base (group-by) source the children must filter
	Edges  []CompositeEdgeOption `json:"edges"`
}

// CompositeEdgeOption is one branch of a parent and the child objects eligible
// to drill it.
type CompositeEdgeOption struct {
	Branch   string   `json:"branch"`   // a parent branch value (a child's eq-filter value on the base)
	Children []string `json:"children"` // names of eligible child objects
}

// CompositeOptions reports the composites buildable from a template's named
// objects. A parent is any object that parses to rank-1 (one group-by); a child
// is eligible for a branch when it carries an eq filter on the parent's base
// equal to that branch value (the same constraint Composite.Validate enforces).
// Parents with no eligible child are omitted; unparseable DSLs are skipped.
func CompositeOptions(objects []StatObject) []CompositeOption {
	type parsed struct {
		obj StatObject
		cfg StatConfig
		ok  bool
	}
	ps := make([]parsed, len(objects))
	for i, o := range objects {
		cfg, err := Parse(o.DSL)
		ps[i] = parsed{obj: o, cfg: cfg, ok: err == nil}
	}

	var out []CompositeOption
	for _, p := range ps {
		if !p.ok || len(p.cfg.Dimensions) != 1 {
			continue
		}
		base := p.cfg.Dimensions[0].Source
		byBranch := map[string][]string{}
		var order []string
		for _, q := range ps {
			if !q.ok || q.obj.Name == p.obj.Name {
				continue
			}
			for _, f := range q.cfg.Filters {
				if f.Op == FilterEq && f.Source == base {
					if _, seen := byBranch[f.Value]; !seen {
						order = append(order, f.Value)
					}
					byBranch[f.Value] = append(byBranch[f.Value], q.obj.Name)
					break
				}
			}
		}
		if len(order) == 0 {
			continue
		}
		edges := make([]CompositeEdgeOption, len(order))
		for i, b := range order {
			edges[i] = CompositeEdgeOption{Branch: b, Children: byBranch[b]}
		}
		out = append(out, CompositeOption{Parent: p.obj.Name, Base: sourceLabel(base, BinNone), Edges: edges})
	}
	return out
}

// ObjectConfigs resolves a named plain object to its parsed config.
// Errors on an unknown name and on a composite (no DSL to parse).
type ObjectConfigs interface {
	Config(name string) (StatConfig, error)
}

// ResolveComposite turns a stored CompositeSpec (parent + child names) into an
// evaluable Composite, resolving each name through src. The relation
// constraint itself is checked later by Composite.Validate / EvaluateComposite.
func ResolveComposite(spec CompositeSpec, src ObjectConfigs) (Composite, error) {
	parentCfg, err := src.Config(spec.Parent)
	if err != nil {
		return Composite{}, fmt.Errorf("stat: composite parent: %w", err)
	}
	parentScales, err := resolveScales(src, parentCfg)
	if err != nil {
		return Composite{}, fmt.Errorf("stat: composite parent scale: %w", err)
	}
	edges := make([]Edge, 0, len(spec.Edges))
	for _, e := range spec.Edges {
		childCfg, err := src.Config(e.Child)
		if err != nil {
			return Composite{}, fmt.Errorf("stat: composite child for branch %q: %w", e.Branch, err)
		}
		scs, err := resolveScales(src, childCfg)
		if err != nil {
			return Composite{}, fmt.Errorf("stat: composite child scale for branch %q: %w", e.Branch, err)
		}
		edges = append(edges, Edge{Branch: e.Branch, Child: childCfg, Scales: scs})
	}
	return Composite{Parent: parentCfg, ParentScales: parentScales, Edges: edges}, nil
}

// resolveScales turns a config's scale-clause names into their weightings; an
// empty clause is no weighting (nil). A named clause whose source can't resolve
// scalings is an error rather than a silent unweighted run.
func resolveScales(src ObjectConfigs, cfg StatConfig) ([]*Scaling, error) {
	if len(cfg.Scales) == 0 {
		return nil, nil
	}
	ss, ok := src.(ScalingSource)
	if !ok {
		return nil, fmt.Errorf("scale %q requested but source cannot resolve scalings", cfg.Scales[0])
	}
	out := make([]*Scaling, 0, len(cfg.Scales))
	for _, name := range cfg.Scales {
		sc, err := ss.Scaling(name)
		if err != nil {
			return nil, err
		}
		out = append(out, sc)
	}
	return out, nil
}

// CompositeGrid is the evaluated composite: the parent rank-1 grid plus, in
// the parent's axis order, the child grid that drills each category (nil for a
// solid leaf).
type CompositeGrid struct {
	Parent   *Grid        `json:"parent"`
	Branches []BranchGrid `json:"branches"`
}

// BranchGrid is one parent category and the child object drilling it (nil = a
// leaf with no edge).
type BranchGrid struct {
	Branch string `json:"branch"`
	Child  *Grid  `json:"child"`
}

// EvaluateComposite validates the relation, evaluates the parent, and attaches
// each edge's child grid to its parent branch. Branches without an edge are
// leaves (nil child); an edge naming a non-category branch is an error. Branch
// values are matched against the parent's axis labels, which coincide for
// facets and value==label sources (see the design doc for the value!=label
// caveat).
func (m *Manager) EvaluateComposite(template string, c Composite) (*CompositeGrid, error) {
	if err := c.Validate(); err != nil {
		return nil, err
	}
	parent, err := m.EvaluateScaled(template, c.Parent, c.ParentScales...)
	if err != nil {
		return nil, err
	}

	edgeByBranch := make(map[string]Edge, len(c.Edges))
	for _, e := range c.Edges {
		edgeByBranch[e.Branch] = e
	}

	var labels []string
	if len(parent.Axes) == 1 {
		labels = parent.Axes[0].Labels
	}
	known := make(map[string]bool, len(labels))
	for _, l := range labels {
		known[l] = true
	}
	for _, e := range c.Edges {
		if !known[e.Branch] {
			return nil, fmt.Errorf("stat: composite branch %q is not a category of the parent", e.Branch)
		}
	}

	branches := make([]BranchGrid, len(labels))
	for i, l := range labels {
		bg := BranchGrid{Branch: l}
		if e, ok := edgeByBranch[l]; ok {
			cg, err := m.EvaluateScaled(template, e.Child, e.Scales...)
			if err != nil {
				return nil, fmt.Errorf("stat: composite child for branch %q: %w", l, err)
			}
			bg.Child = cg
		}
		branches[i] = bg
	}
	return &CompositeGrid{Parent: parent, Branches: branches}, nil
}
