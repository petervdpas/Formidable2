package optrack

// Service is the Wails-bound surface of the op registry. The frontend calls
// Active() on mount/reload to reflect or resume any running long op.
type Service struct {
	reg *Registry
}

func NewService(reg *Registry) *Service { return &Service{reg: reg} }

// Active returns the currently running ops across the whole system.
func (s *Service) Active() []Status {
	if s.reg == nil {
		return nil
	}
	return s.reg.List()
}
