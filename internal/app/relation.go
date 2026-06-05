package app

import (
	"context"

	"github.com/petervdpas/formidable2/internal/modules/dataprovider"
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
