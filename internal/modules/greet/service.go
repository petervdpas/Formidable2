package greet

type Service struct{}

func New() *Service { return &Service{} }

func (s *Service) Greet(name string) string {
	return "Hello " + name + "!"
}
