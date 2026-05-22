package pdf

import (
	"fmt"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// InjectConfig is the typed input that BuildFrontmatter renders into
// a picoloom v2 frontmatter YAML scaffold. The Inject dialog
// collects values into this shape - toggles per block, dropdowns per
// enum field, text inputs per cover/footer/signature field - and the
// backend renders the YAML deterministically.
//
// Block pointers are nil when the user's toggle for that block is
// OFF, so the emitted scaffold contains only the blocks the user
// asked for. Style is a top-level scalar (no block to gate); empty
// means "no theme override".
//
// Watermark and PageBreaks are intentionally absent - they're
// power-user features deferred until there's demand.
type InjectConfig struct {
	Style     string                 `json:"style,omitempty"`
	Keywords  []string               `json:"keywords,omitempty"`
	Page      *InjectPageConfig      `json:"page,omitempty"`
	Cover     *InjectCoverConfig     `json:"cover,omitempty"`
	TOC       *InjectTOCConfig       `json:"toc,omitempty"`
	Footer    *InjectFooterConfig    `json:"footer,omitempty"`
	Signature *InjectSignatureConfig `json:"signature,omitempty"`
}

// InjectPageConfig - page layout. Values from ListPageSizes() and
// ListPageOrientations() drive the dropdowns; the user can also leave
// fields empty for picoloom defaults.
type InjectPageConfig struct {
	Size        string  `json:"size,omitempty"`
	Orientation string  `json:"orientation,omitempty"`
	Margin      float64 `json:"margin,omitempty"`
}

// InjectCoverConfig - every cover field except the picoloom-internal
// ones (Enabled / TemplatePath). Empty fields are skipped on emit.
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

// InjectTOCConfig - table-of-contents block.
type InjectTOCConfig struct {
	Title    string `json:"title,omitempty"`
	MinDepth int    `json:"min_depth,omitempty"`
	MaxDepth int    `json:"max_depth,omitempty"`
}

// InjectFooterConfig - footer block. ShowPageNumber is a value type
// because the dialog always has a position for the toggle (on/off);
// nil-vs-false distinction isn't needed at scaffold-emit time.
type InjectFooterConfig struct {
	Position       string `json:"position,omitempty"`
	ShowPageNumber bool   `json:"show_page_number"`
	Date           string `json:"date,omitempty"`
	Status         string `json:"status,omitempty"`
	Text           string `json:"text,omitempty"`
	DocumentID     string `json:"document_id,omitempty"`
}

// InjectSignatureConfig - signature block. Links (signature.links
// array) are deferred - out of scope for the v1 wizard.
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

// BuildFrontmatter renders the typed InjectConfig into a YAML
// frontmatter block (surrounded by `---` fences, terminated with a
// newline). Blocks the user disabled are omitted entirely; empty
// optional fields are skipped. The output is deterministic - same
// config in produces the same YAML out - so tests can compare
// against literal strings.
//
// Each enabled sub-block carries an explicit `enabled: true` for
// readability; this matches what picoloom expects and makes the
// emitted YAML self-documenting when the user opens the editor later.
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

// wholeHandlebarsRe matches a string consisting entirely of a single
// Handlebars expression. Used by the wizard so a user-typed
// `{{yamlList (fieldRaw "x")}}` lands at raw-line position instead
// of getting single-quoted as a scalar.
var wholeHandlebarsRe = regexp.MustCompile(`^\s*\{\{.+?\}\}\s*$`)

// buildKeywordsBlock emits the column-0 `keywords:` block for the
// wizard's BuildFrontmatter path. Wholly-handlebars elements drop
// the `- ` prefix and the single-quoting so the helper expansion
// plugs into the block sequence at render time.
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

// insertKeywordsBlock splices the column-0 keywords block into a
// yaml.Marshal'd Frontmatter body right after the `style:` line, or
// at the top when no style is set. The block sits at the canonical
// position (style → keywords → page → cover → …) so the wizard
// output stays readable.
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

// PageSizeDescriptor / OrientationDescriptor / FooterPositionDescriptor
// follow the same shape as TableColumnTypeDescriptor / ThemeDescriptor:
// just a Name string. Human labels come from frontend i18n keys
// (`pdf.export.dialog.page_size.<name>` etc.). Display order is
// significant - the slice ordering IS the dropdown ordering.
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
