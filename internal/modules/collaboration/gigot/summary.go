package gigot

// LedgerSummary diffs the context folder against the ledger for the Sync UI; read-only, never writes the ledger.
func (m *Manager) LedgerSummary(contextFolder string) (*LedgerSummary, error) {
	if contextFolder == "" {
		return nil, ErrMissingContext
	}
	local, err := CollectFormidableFiles(contextFolder)
	if err != nil {
		return nil, err
	}
	rec := m.ReadTrackRecord(contextFolder)
	diff := DiffAgainstRecord(local, rec)

	changed := make([]string, 0, len(diff.Changed))
	for _, f := range diff.Changed {
		changed = append(changed, f.Path)
	}
	deleted := diff.Deleted
	if deleted == nil {
		deleted = []string{}
	}
	return &LedgerSummary{
		Version:  rec.Version,
		LastSync: rec.LastSync,
		Changed:  changed,
		Deleted:  deleted,
		Scanned:  len(local),
	}, nil
}
