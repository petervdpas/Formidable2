package datacore

// Loader is the seam between the tensor and wherever records live (storage,
// a fixture, a remote source).
type Loader interface {
	Records() ([]Record, error)
}

// Build ingests every record the loader yields into a fresh tensor.
func Build(l Loader) (*Tensor, error) {
	recs, err := l.Records()
	if err != nil {
		return nil, err
	}
	return buildFromRecords(recs), nil
}
