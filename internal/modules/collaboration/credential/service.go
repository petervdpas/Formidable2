package credential

// Service is the Wails-bound surface of the credential Manager.
type Service struct{ m *Manager }

func NewService(m *Manager) *Service { return &Service{m: m} }

// Set stores secret under account.
func (s *Service) Set(account, secret string) error { return s.m.Set(account, secret) }

// Has returns whether a non-empty secret is present for account.
func (s *Service) Has(account string) LookupResult {
	return LookupResult{Found: s.m.Has(account)}
}

// Delete removes the entry for account.
func (s *Service) Delete(account string) error { return s.m.Delete(account) }

// Get is intentionally NOT exposed via Wails: secrets stay on the backend, read only by backend sync ops.
