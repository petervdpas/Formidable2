package wiki

import (
	"maps"
	"sort"
)

// DependencyGraph reports the direct OUTGOING template dependencies of a
// template: the other templates whose record pages it links to. Two declared,
// deterministic edge kinds feed this in the composition root: a template's
// non-inverse relations and its api-field target collections. Both name the
// target by template filename (e.g. "audit-controls.yaml"), so the closure runs
// over a single identity. Incoming edges (templates that point AT this one) are
// deliberately NOT followed: a reader of A's pages needs what A links to, not
// everything that links to A.
type DependencyGraph interface {
	// DirectDeps returns the filenames a template directly depends on.
	DirectDeps(filename string) ([]string, error)
	// TemplateExists reports whether a filename is a known template, so a
	// dangling reference is reported instead of pulling a non-existent page
	// into the bundle.
	TemplateExists(filename string) bool
}

// DependencyResult is the outcome of expanding a user's explicit template picks
// into the full set an offline bundle needs for uninterrupted reading.
type DependencyResult struct {
	// Required is the full closure (the picks plus every transitive
	// dependency), sorted and deduped. The export must include exactly this.
	Required []string `json:"required"`
	// Added is Required minus the explicit picks, sorted: the templates the
	// export pulls in on the author's behalf. The frontend force-toggles these.
	Added []string `json:"added"`
	// Because maps each added filename to the explicit picks that pulled it in
	// (sorted), so the UI can explain why a switch is locked on.
	Because map[string][]string `json:"because"`
	// Missing lists referenced filenames that are not known templates (dangling
	// relation/api targets), sorted. They are left out of Required, never
	// fabricated.
	Missing []string `json:"missing"`
}

// resolveDeps expands an explicit set of picks into the full closure the bundle
// needs, following outgoing edges transitively. It is pure over the graph:
//   - a pick with no dependencies carries only itself;
//   - a dependency that is not a known template goes to Missing, not Required;
//   - cycles terminate (each template is visited once per originating pick).
//
// Each pick is walked independently so a dependency can be attributed to the
// pick(s) that reached it (Because), which drives the "included by X" UI.
func resolveDeps(seeds []string, g DependencyGraph) (DependencyResult, error) {
	picks := map[string]bool{}
	for _, s := range seeds {
		if s != "" {
			picks[s] = true
		}
	}

	required := map[string]bool{}
	missing := map[string]bool{}
	because := map[string]map[string]bool{}

	for seed := range picks {
		seen := map[string]bool{}
		queue := []string{seed}
		for len(queue) > 0 {
			cur := queue[0]
			queue = queue[1:]
			if seen[cur] {
				continue
			}
			seen[cur] = true
			required[cur] = true

			deps, err := g.DirectDeps(cur)
			if err != nil {
				return DependencyResult{}, err
			}
			for _, d := range deps {
				if d == "" || d == cur {
					continue
				}
				if !g.TemplateExists(d) {
					missing[d] = true
					continue
				}
				if because[d] == nil {
					because[d] = map[string]bool{}
				}
				because[d][seed] = true
				queue = append(queue, d)
			}
		}
	}

	res := DependencyResult{
		Required: sortedKeys(required),
		Added:    []string{},
		Because:  map[string][]string{},
		Missing:  sortedKeys(missing),
	}
	for _, fn := range res.Required {
		if picks[fn] {
			continue // an explicit pick is never "added"
		}
		res.Added = append(res.Added, fn)
		res.Because[fn] = sortedKeys(because[fn])
	}
	return res, nil
}

// expandSelections adds every auto-included dependency to the export selection,
// so the produced zip is self-contained. An auto-added template joins with a nil
// deck slice, which the exporter reads as "the whole document / all decks". With
// no graph wired, the selection is returned unchanged. Tolerant: a graph error
// falls back to the selection as-given rather than failing the export.
func (h *Handler) expandSelections(selections map[string][]string) map[string][]string {
	if h.deps == nil {
		return selections
	}
	seeds := make([]string, 0, len(selections))
	for fn := range selections {
		if fn != "" {
			seeds = append(seeds, fn)
		}
	}
	res, err := resolveDeps(seeds, h.deps)
	if err != nil {
		return selections
	}
	out := make(map[string][]string, len(selections)+len(res.Added))
	maps.Copy(out, selections)
	for _, add := range res.Added {
		if _, ok := out[add]; !ok {
			out[add] = nil
		}
	}
	return out
}

// sortedKeys returns a set's keys sorted; a nil/empty set yields an empty slice
// (never nil) so JSON emits [] rather than null for the frontend.
func sortedKeys(set map[string]bool) []string {
	out := make([]string, 0, len(set))
	for k := range set {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
