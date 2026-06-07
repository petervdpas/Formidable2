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
	// Desired host->target edges, keyed by target template, unioned across every
	// api-field. A target present with an empty set means "field cleared", which
	// reconciliation then drains.
	desired := map[string]map[string]bool{}
	for _, f := range fields {
		if f.Type != "api" || f.Collection == "" {
			continue
		}
		set := desired[f.Collection]
		if set == nil {
			set = map[string]bool{}
			desired[f.Collection] = set
		}
		for _, id := range referenceIDs(data[f.Key]) {
			set[id] = true
		}
	}
	if len(desired) == 0 {
		return nil
	}

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
