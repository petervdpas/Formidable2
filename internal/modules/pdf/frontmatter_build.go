package pdf

import (
	"fmt"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// InjectConfig is the typed input BuildFrontmatter renders into a
// picoloom v2 YAML scaffold. A nil block pointer means the user's
// toggle for that block is OFF, so it is omitted from the output.
// Watermark and PageBreaks are intentionally absent (deferred).
type InjectConfig struct {
	Style     string                 `json:"style,omitempty"`
	Keywords  []string               `json:"keywords,omitempty"`
	Page      *InjectPageConfig      `json:"page,omitempty"`
	Cover     *InjectCoverConfig     `json:"cover,omitempty"`
	TOC       *InjectTOCConfig       `json:"toc,omitempty"`
	Footer    *InjectFooterConfig    `json:"footer,omitempty"`
	Signature *InjectSignatureConfig `json:"signature,omitempty"`
}

// InjectPageConfig is the page-layout block; empty fields fall to
// picoloom defaults.
type InjectPageConfig struct {
	Size        string  `json:"size,omitempty"`
	Orientation string  `json:"orientation,omitempty"`
	Margin      float64 `json:"margin,omitempty"`
}

// InjectCoverConfig holds every cover field except Enabled / TemplatePath.
type InjectCoverConfig struct {
	Template     string `json:"template,omitempty"`
	Title        string `json:"title,omitempty"`
	Subtitle     string `json:"subtitle,omitempty"`
	Author       string `json:"author,omitempty"`
	AuthorTitle  string `json:"author_title,omitempty"`
	Organization string `json:"organization,omitempty"`
	Date         string `json:"date,omitempty"`
	Version      string `json:"version,omitempty"`
	ClientName   string `json:"client_name,omitempty"`
	ProjectName  string `json:"project_name,omitempty"`
	DocumentType string `json:"document_type,omitempty"`
	DocumentID   string `json:"document_id,omitempty"`
	Description  string `json:"description,omitempty"`
	Department   string `json:"department,omitempty"`
	Logo         string `json:"logo,omitempty"`
}

// InjectTOCConfig is the table-of-contents block.
type InjectTOCConfig struct {
	Title    string `json:"title,omitempty"`
	MinDepth int    `json:"min_depth,omitempty"`
	MaxDepth int    `json:"max_depth,omitempty"`
}

// InjectFooterConfig is the footer block. ShowPageNumber is a value
// (not pointer): the dialog always supplies on/off, so nil-vs-false
// is moot at scaffold-emit time.
type InjectFooterConfig struct {
	Position       string `json:"position,omitempty"`
	ShowPageNumber bool   `json:"show_page_number"`
	Date           string `json:"date,omitempty"`
	Status         string `json:"status,omitempty"`
	Text           string `json:"text,omitempty"`
	DocumentID     string `json:"document_id,omitempty"`
}

// InjectSignatureConfig is the signature block; Links are deferred.
type InjectSignatureConfig struct {
	Name         string `json:"name,omitempty"`
	Title        string `json:"title,omitempty"`
	Email        string `json:"email,omitempty"`
	Organization string `json:"organization,omitempty"`
	ImagePath    string `json:"image_path,omitempty"`
	Phone        string `json:"phone,omitempty"`
	Address      string `json:"address,omitempty"`
	Department   string `json:"department,omitempty"`
}

// BuildFrontmatter renders the InjectConfig into a `---`-fenced YAML
// block. Disabled blocks and empty fields are omitted. Output is
// deterministic so tests can compare literal strings. Each enabled
// sub-block carries an explicit `enabled: true` so the emitted YAML
// is self-documenting.
func BuildFrontmatter(cfg InjectConfig) (string, error) {
	fm := Frontmatter{Style: cfg.Style}
	keywords := append([]string(nil), cfg.Keywords...)
	on := true

	if cfg.Page != nil {
		fm.Page = &PageFM{
			Size:        cfg.Page.Size,
			Orientation: cfg.Page.Orientation,
			Margin:      cfg.Page.Margin,
		}
	}
	if cfg.Cover != nil {
		fm.Cover = &CoverFM{
			Enabled:      &on,
			Template:     cfg.Cover.Template,
			Title:        cfg.Cover.Title,
			Subtitle:     cfg.Cover.Subtitle,
			Logo:         cfg.Cover.Logo,
			Author:       cfg.Cover.Author,
			AuthorTitle:  cfg.Cover.AuthorTitle,
			Organization: cfg.Cover.Organization,
			Date:         cfg.Cover.Date,
			Version:      cfg.Cover.Version,
			ClientName:   cfg.Cover.ClientName,
			ProjectName:  cfg.Cover.ProjectName,
			DocumentType: cfg.Cover.DocumentType,
			DocumentID:   cfg.Cover.DocumentID,
			Description:  cfg.Cover.Description,
			Department:   cfg.Cover.Department,
		}
	}
	if cfg.TOC != nil {
		fm.TOC = &TOCFM{
			Enabled:  &on,
			Title:    cfg.TOC.Title,
			MinDepth: cfg.TOC.MinDepth,
			MaxDepth: cfg.TOC.MaxDepth,
		}
	}
	if cfg.Footer != nil {
		showPage := cfg.Footer.ShowPageNumber
		fm.Footer = &FooterFM{
			Enabled:        &on,
			Position:       cfg.Footer.Position,
			ShowPageNumber: &showPage,
			Date:           cfg.Footer.Date,
			Status:         cfg.Footer.Status,
			Text:           cfg.Footer.Text,
			DocumentID:     cfg.Footer.DocumentID,
		}
	}
	if cfg.Signature != nil {
		fm.Signature = &SignatureFM{
			Enabled:      &on,
			Name:         cfg.Signature.Name,
			Title:        cfg.Signature.Title,
			Email:        cfg.Signature.Email,
			Organization: cfg.Signature.Organization,
			ImagePath:    cfg.Signature.ImagePath,
			Phone:        cfg.Signature.Phone,
			Address:      cfg.Signature.Address,
			Department:   cfg.Signature.Department,
		}
	}

	body, err := yaml.Marshal(&fm)
	if err != nil {
		return "", fmt.Errorf("pdf: build frontmatter: %w", err)
	}
	emptyBody := strings.TrimSpace(string(body)) == "{}"
	if emptyBody && len(keywords) == 0 {
		return "---\n---\n", nil
	}
	out := string(body)
	if emptyBody {
		out = ""
	}
	if len(keywords) > 0 {
		out = insertKeywordsBlock(out, buildKeywordsBlock(keywords))
	}
	return "---\n" + out + "---\n", nil
}

// wholeHandlebarsRe matches a string that is entirely one Handlebars
// expression, so it lands at raw-line position instead of being
// single-quoted as a scalar.
var wholeHandlebarsRe = regexp.MustCompile(`^\s*\{\{.+?\}\}\s*$`)

// buildKeywordsBlock emits the column-0 `keywords:` block. Wholly-
// handlebars elements drop the `- ` prefix and quoting so the helper
// expansion plugs into the block sequence at render time.
func buildKeywordsBlock(keywords []string) string {
	items := make([]any, 0, len(keywords))
	for _, k := range keywords {
		if k == "" {
			continue
		}
		if wholeHandlebarsRe.MatchString(k) {
			items = append(items, yamlRawLinePrefix+strings.TrimSpace(k))
			continue
		}
		items = append(items, k)
	}
	if len(items) == 0 {
		return ""
	}
	return marshalKeywordsBlock(items)
}

// insertKeywordsBlock splices the keywords block in after `style:` (or
// at the top), keeping the canonical block order.
func insertKeywordsBlock(yamlBody, kwBlock string) string {
	if kwBlock == "" {
		return yamlBody
	}
	if strings.HasPrefix(yamlBody, "style:") {
		if idx := strings.Index(yamlBody, "\n"); idx >= 0 {
			return yamlBody[:idx+1] + kwBlock + yamlBody[idx+1:]
		}
	}
	return kwBlock + yamlBody
}

// ---------- enum registries ----------

// PageSizeDescriptor is a dropdown entry; labels come from frontend
// i18n. Slice ordering IS the dropdown ordering.
type PageSizeDescriptor struct {
	Name string `json:"name"`
}

type OrientationDescriptor struct {
	Name string `json:"name"`
}

type FooterPositionDescriptor struct {
	Name string `json:"name"`
}

// Canonical picoloom v2 enum sets. Picoloom doesn't expose these as
// registries (same situation as themes - see builtinThemes), so the
// Go side keeps the one-source-of-truth copy. Keep in sync with
// picoloom's PageSize* / Orientation* / Position* constants in
// `github.com/alnah/picoloom/v2/types.go`. When picoloom adds a value,
// add it here and (if applicable) the i18n label key.
var builtinPageSizes = []PageSizeDescriptor{
	{Name: "a4"},
	{Name: "letter"},
	{Name: "legal"},
}

var builtinOrientations = []OrientationDescriptor{
	{Name: "portrait"},
	{Name: "landscape"},
}

var builtinFooterPositions = []FooterPositionDescriptor{
	{Name: "left"},
	{Name: "center"},
	{Name: "right"},
}
