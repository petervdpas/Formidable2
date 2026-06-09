package app

import (
	"context"

	"github.com/petervdpas/formidable2/internal/modules/api"
	"github.com/petervdpas/formidable2/internal/modules/dataprovider"
	"github.com/petervdpas/formidable2/internal/modules/relation"
	"github.com/petervdpas/formidable2/internal/modules/template"
)

// relationCatalog implements relation.Catalog over the dataprovider, so the relation module never
// imports template/storage. Two questions only: is a template a collection, does a record exist.
type relationCatalog struct{ dp *dataprovider.Manager }

func (c relationCatalog) IsCollection(template string) bool {
	return c.dp.IsCollectionEnabled(context.Background(), template)
}

func (c relationCatalog) RecordExists(template, id string) bool {
	_, ok, err := c.dp.ResolveCollectionByID(context.Background(), template, id)
	return err == nil && ok
}

// apiRelations adapts relationM onto api.Relations so the api module exposes
// record relations without importing the relation package (it maps the types).
type apiRelations struct{ rel *relation.Manager }

func (a apiRelations) GetRelations(template string) ([]api.RelationDef, error) {
	rels, err := a.rel.GetRelations(template)
	if err != nil {
		return nil, err
	}
	out := make([]api.RelationDef, 0, len(rels))
	for _, r := range rels {
		edges := make([]api.RelationEdgePair, 0, len(r.Edges))
		for _, e := range r.Edges {
			edges = append(edges, api.RelationEdgePair{From: e.From, To: e.To})
		}
		out = append(out, api.RelationDef{
			To:          r.To,
			Cardinality: string(r.Cardinality),
			Inverse:     r.Inverse,
			Edges:       edges,
		})
	}
	return out, nil
}

// referenceEdgeSyncer reconciles a host record's api-field references into the
// relation edge graph (implements form.ReferenceEdgeSyncer). An api-field's
// target template is owned by the field(s): host->target edges are made to match
// the union of those fields' selected ids, so picking writes an edge and clearing
// removes it. Targets with no api-field are left untouched, so manual links from
// the relation panel to other targets are safe. Best-effort: a per-edge error
// (undeclared relation, cardinality breach, missing record) is skipped, not fatal.
type referenceEdgeSyncer struct{ rel *relation.Manager }

func (s referenceEdgeSyncer) SyncReferenceEdges(hostTemplate, hostGuid string, fields []template.Field, data map[string]any) error {
	// Every api-field's target collection is "owned" so reconciliation runs for
	// it even when nothing is referenced (a fully-cleared collection must drain
	// its edges). The flat fields slice already contains loop-nested fields, so
	// this captures them whatever their nesting.
	desired := map[string]map[string]bool{}
	for _, f := range fields {
		if f.Type == "api" && f.Collection != "" && desired[f.Collection] == nil {
			desired[f.Collection] = map[string]bool{}
		}
	}
	if len(desired) == 0 {
		return nil
	}

	// Union every referenced guid across the whole form - top-level fields AND
	// every loop row - into the desired set. An edge survives when its target
	// guid is referenced anywhere in the form; it drains only when that guid is
	// absent everywhere. Clearing one api-field instance never removes a relation
	// while the same guid still appears elsewhere.
	collectAPIReferences(fields, data, func(collection, id string) {
		if set := desired[collection]; set != nil {
			set[id] = true
		}
	})

	rels, err := s.rel.GetRelations(hostTemplate)
	if err != nil {
		return err
	}
	current := map[string]map[string]bool{}
	for _, r := range rels {
		if _, owned := desired[r.To]; !owned {
			continue
		}
		set := map[string]bool{}
		for _, e := range r.Edges {
			if e.From == hostGuid {
				set[e.To] = true
			}
		}
		current[r.To] = set
	}

	for target, want := range desired {
		have := current[target]
		for id := range want {
			if have[id] {
				continue
			}
			// Tolerate: undeclared relation, cardinality, or a not-yet-saved target.
			_ = s.rel.AddEdge(hostTemplate, target, relation.Edge{From: hostGuid, To: id})
		}
		for id := range have {
			if want[id] {
				continue
			}
			_ = s.rel.RemoveEdge(hostTemplate, target, relation.Edge{From: hostGuid, To: id})
		}
	}
	return nil
}

// collectAPIReferences walks fields alongside their data, descending into loop
// rows, calling add(collection, id) for every api-field reference it finds. A
// loop is a loopstart/loopstop pair whose inner fields' data lives as an array
// under the loopstart key; each row is the data map for those inner fields,
// which may themselves contain loops, so the walk recurses.
func collectAPIReferences(fields []template.Field, data map[string]any, add func(collection, id string)) {
	for i := 0; i < len(fields); i++ {
		f := fields[i]
		switch f.Type {
		case "loopstart":
			loopKey := f.Key
			depth := 1
			inner := []template.Field{}
			i++
			for i < len(fields) && depth > 0 {
				ff := fields[i]
				switch ff.Type {
				case "loopstart":
					depth++
				case "loopstop":
					depth--
				}
				if depth > 0 {
					inner = append(inner, ff)
				}
				i++
			}
			i-- // the outer loop's i++ steps past the matching loopstop
			rows, _ := data[loopKey].([]any)
			for _, r := range rows {
				if rm, ok := r.(map[string]any); ok {
					collectAPIReferences(inner, rm, add)
				}
			}
		case "loopstop":
			// Unbalanced marker; nothing to do.
		case "api":
			if f.Collection != "" {
				for _, id := range referenceIDs(data[f.Key]) {
					add(f.Collection, id)
				}
			}
		}
	}
}

// referenceIDs pulls the target id(s) from an api-field value: a bare id string
// (single) or a list of id strings (to-many). Empty entries are dropped.
func referenceIDs(v any) []string {
	switch t := v.(type) {
	case string:
		if t != "" {
			return []string{t}
		}
	case []any:
		out := make([]string, 0, len(t))
		for _, e := range t {
			if s, ok := e.(string); ok && s != "" {
				out = append(out, s)
			}
		}
		return out
	}
	return nil
}
