package manual

// Service is the Wails-bound surface for the in-app manual. Each
// method returns the raw markdown for one topic in one locale; the
// frontend renders via the existing render module so the manual gets
// the same syntax highlighting and link handling as the wiki.
type Service struct{}

// NewService returns the Wails-bound surface. Stateless — the embed
// FS lives at package scope, so a single instance is shared by every
// IPC caller.
func NewService() *Service {
	return &Service{}
}

// GetTopic returns the embedded markdown for (topic, locale). Unknown
// locale or missing translation falls back to english. Unknown topic
// returns an error so the frontend can show "topic not found" rather
// than an empty pane.
func (s *Service) GetTopic(topic, locale string) (string, error) {
	return manualDoc(topic, locale)
}
