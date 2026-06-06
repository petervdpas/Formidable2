package app

import (
	"context"

	"github.com/petervdpas/formidable2/internal/modules/api"
	"github.com/petervdpas/formidable2/internal/modules/dataprovider"
	"github.com/petervdpas/formidable2/internal/modules/relation"
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
