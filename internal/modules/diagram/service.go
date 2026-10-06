package diagram

import (
	"fmt"

	"github.com/petervdpas/formidable2/internal/modules/template"
)

type TemplateLoader interface {
	LoadTemplate(name string) (*template.Template, error)
}

// Service is the Wails facade. Render takes the record's live data (not the
// saved file) so the drawing follows unsaved table edits.
type Service struct {
	tpl TemplateLoader
}

func NewService(tpl TemplateLoader) *Service { return &Service{tpl: tpl} }

// Render returns the SVG for diagram field fieldKey of templateFile drawn from
// data; "" when the source table is empty.
func (s *Service) Render(templateFile, fieldKey string, data map[string]any) (string, error) {
	t, err := s.tpl.LoadTemplate(templateFile)
	if err != nil {
		return "", err
	}
	svg, ok := RenderField(t, fieldKey, data)
	if !ok {
		return "", fmt.Errorf("diagram: %q is not a bound diagram field of %s", fieldKey, templateFile)
	}
	return svg, nil
}
