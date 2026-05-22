package credential

// Service is the Wails-bound surface of the credential Manager.
// Same shape as Manager - the split exists for parity with other
// modules and to give us a place to hang request-scoped concerns
// (logging, audit trail) later without leaking them into domain
// code.
type Service struct{ m *Manager }

func NewService(m *Manager) *Service { return &Service{m: m} }

// Set stores secret under account. Empty values are rejected.
func (s *Service) Set(account, secret string) error { return s.m.Set(account, secret) }

// Has returns whether a non-empty secret is present for account.
// Bound through a struct so future metadata (created_at etc.) can
// land without breaking the Wails signature.
func (s *Service) Has(account string) LookupResult {
	return LookupResult{Found: s.m.Has(account)}
}

// Delete removes the entry for account. Idempotent.
func (s *Service) Delete(account string) error { return s.m.Delete(account) }

// Get is intentionally NOT exposed via Wails - secrets stay on the
// backend. The frontend can ask "does this exist" via Has and write
// new values via Set; reading is reserved for backend ops (clone,
// pull, push) that need the secret to talk to the remote.
