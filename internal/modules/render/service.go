package render

// Service is the Wails-bound facade for the render module. Vue calls
// these to drive the Storage workspace's Render button.
type Service struct{ m *Manager }

func NewService(m *Manager) *Service { return &Service{m: m} }

// RenderForm - Handlebars markdown + sanitized HTML for one
// (template, datafile) pair. Empty datafile renders defaults.
func (s *Service) RenderForm(templateName, datafile string) (*Result, error) {
	return s.m.RenderForm(templateName, datafile)
}

// RenderMarkdown - Handlebars stage only. Useful when Vue wants to let
// the user edit the intermediate markdown before HTML rendering.
func (s *Service) RenderMarkdown(templateName, datafile string) (string, error) {
	return s.m.RenderMarkdown(templateName, datafile)
}

// RenderHTML - re-render arbitrary markdown (Vue's editor preview).
func (s *Service) RenderHTML(markdown string) (string, error) {
	return s.m.RenderHTMLOnly(markdown)
}

// RenderSlideBlockHTML renders one slide block's content to HTML, so the canvas
// editor shows the same output the deck will render. content is the block's
// value (string, 2D array, etc.); templateName scopes image URLs.
func (s *Service) RenderSlideBlockHTML(templateName, kind string, content any) string {
	return s.m.RenderSlideBlockHTML(templateName, kind, content)
}

// SanitizeSVG cleans imported SVG markup (e.g. an Inkscape export) into the safe
// subset a slide shape block renders: script/foreignObject/event-handlers and
// external references are stripped, geometry and safe styling kept. Returns ""
// when the input is empty, too large, or not valid SVG. The same sanitizer runs
// again at render time, so this is for storing clean data and previewing on
// import.
func (s *Service) SanitizeSVG(raw string) string {
	clean, ok := sanitizeSVG(raw)
	if !ok {
		return ""
	}
	return clean
}

// BuildDeck renders ordered records into reveal.js slide sections for the deck
// previewer. datafiles come from form.DeckOrder / SequenceOrder (deck order).
func (s *Service) BuildDeck(templateName string, datafiles []string) (RevealDeck, error) {
	return s.m.BuildDeck(templateName, datafiles)
}

// BuildBoard lays one plan-board record onto its project time axis for the board
// previewer: axis ticks (from/to by time-block) plus each event mapped to the
// tick range it spans. Unlike a deck, a board is a single record, so it takes
// one datafile, not a deck-ordered list.
func (s *Service) BuildBoard(templateName, datafile string) (Board, error) {
	return s.m.BuildBoard(templateName, datafile)
}

// BuildBoardLive lays in-progress events onto the template's project axis so the
// form editor can render a live board as the user edits, without saving first.
func (s *Service) BuildBoardLive(templateName, name string, events any, resourceOrder []string, timeBlock string) (Board, error) {
	return s.m.BuildBoardLive(templateName, name, events, resourceOrder, timeBlock)
}

// RenderFullHTML - self-contained HTML document (DOCTYPE + head + body)
// with the formidable-prose stylesheet inlined. Used by the storage
// workspace's "Copy HTML" action so what the user pastes into a .html
// file renders identically to the in-app preview.
func (s *Service) RenderFullHTML(templateName, datafile string) (string, error) {
	return s.m.RenderFullHTML(templateName, datafile)
}

// ListHelpers - the catalog of every Handlebars helper this module
// registers, for the Information panel's "Render helpers reference".
// Static data (no Manager state read); returned as a fresh slice the
// frontend can sort/filter freely.
func (s *Service) ListHelpers() []HelperDescriptor {
	return Catalog()
}

// ValidateMarkdownTemplate parses the given Handlebars source and
// reports parse errors plus helper-name warnings. Cheap enough to call
// on every keystroke (debounced from the frontend).
func (s *Service) ValidateMarkdownTemplate(src string) ValidationReport {
	return ValidateMarkdownTemplate(src)
}
