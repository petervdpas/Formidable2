package datacore

// Loader is the seam between the tensor and wherever records live (storage, a
// fixture, a remote source). datacore depends only on this interface, never
// on a concrete store, so the substrate stays self-contained and the mapping
// from a live template lives at the composition root.
type Loader interface {
	Records() ([]Record, error)
}

// Build ingests every record the loader yields into a fresh tensor.
func Build(l Loader) (*Tensor, error) {
	recs, err := l.Records()
	if err != nil {
		return nil, err
	}
	t := New()
	for _, r := range recs {
		t.Ingest(r)
	}
	return t, nil
}
