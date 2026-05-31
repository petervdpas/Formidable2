package app

import (
	"context"
	"errors"

	"github.com/petervdpas/formidable2/internal/modules/storage"
)

// integrityStorageAdapter implements integrity.StorageWriter via
// storage.Manager.SaveFormExact, which bypasses the "preserve prev meta"
// merge in the everyday SaveForm path. The Fix pipeline owns the meta block
// on writes (mint UUID, re-stamp timestamps), so exact-write is correct.
type integrityStorageAdapter struct {
	sto *storage.Manager
}

func (a integrityStorageAdapter) SaveForm(ctx context.Context, templateFilename, datafile string, form *storage.Form) error {
	if form == nil {
		return errors.New("integrity adapter: nil form")
	}
	r := a.sto.SaveFormExact(ctx, templateFilename, datafile, *form)
	if !r.Success {
		return errors.New(r.Error)
	}
	return nil
}
