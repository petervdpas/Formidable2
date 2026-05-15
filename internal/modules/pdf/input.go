package pdf

import (
	"errors"
	"fmt"
	"regexp"
	"strings"

	picoloom "github.com/alnah/picoloom/v2"
	"gopkg.in/yaml.v3"
)

// Frontmatter is the typed YAML schema for PDF export configuration.
// The shape mirrors picoloom.Input 1:1, with one Formidable-specific
// addition: each major block carries an Enabled flag that gates
// whether the block contributes to the final picoloom.Input. picoloom
// itself uses nil pointers for "no cover / no toc / etc"; the Enabled
// flag lets all four merge layers reason about presence in a uniform
// way (a higher layer can say "explicitly no cover" against a lower
// layer that asserts one).
//
// Sub-blocks are pointers so "no opinion" (nil) cleanly differs from
// "explicitly empty" (non-nil zero). Within a non-nil sub-block,
// scalar zero values cascade to the next layer; *bool fields carry
// the same semantics for booleans (where false vs unset must be
// distinguishable).
type Frontmatter struct {
	Style      string        `yaml:"style,omitempty"`
	Page       *PageFM       `yaml:"page,omitempty"`
	Cover      *CoverFM      `yaml:"cover,omitempty"`
	TOC        *TOCFM        `yaml:"toc,omitempty"`
	Footer     *FooterFM     `yaml:"footer,omitempty"`
	Signature  *SignatureFM  `yaml:"signature,omitempty"`
	Watermark  *WatermarkFM  `yaml:"watermark,omitempty"`
	PageBreaks *PageBreaksFM `yaml:"pageBreaks,omitempty"`
}

// PageFM mirrors picoloom.PageSettings.
type PageFM struct {
	Size        string  `yaml:"size,omitempty"`
	Orientation string  `yaml:"orientation,omitempty"`
	Margin      float64 `yaml:"margin,omitempty"`
}

// CoverFM mirrors picoloom.Cover plus the Formidable Enabled gate.
type CoverFM struct {
	Enabled      *bool  `yaml:"enabled,omitempty"`
	Title        string `yaml:"title,omitempty"`
	Subtitle     string `yaml:"subtitle,omitempty"`
	Logo         string `yaml:"logo,omitempty"`
	Author       string `yaml:"author,omitempty"`
	AuthorTitle  string `yaml:"authorTitle,omitempty"`
	Organization string `yaml:"organization,omitempty"`
	Date         string `yaml:"date,omitempty"`
	Version      string `yaml:"version,omitempty"`
	ClientName   string `yaml:"clientName,omitempty"`
	ProjectName  string `yaml:"projectName,omitempty"`
	DocumentType string `yaml:"documentType,omitempty"`
	DocumentID   string `yaml:"documentID,omitempty"`
	Description  string `yaml:"description,omitempty"`
	Department   string `yaml:"department,omitempty"`
}

// TOCFM mirrors picoloom.TOC plus Enabled gate.
type TOCFM struct {
	Enabled  *bool  `yaml:"enabled,omitempty"`
	Title    string `yaml:"title,omitempty"`
	MinDepth int    `yaml:"minDepth,omitempty"`
	MaxDepth int    `yaml:"maxDepth,omitempty"`
}

// FooterFM mirrors picoloom.Footer plus Enabled gate.
type FooterFM struct {
	Enabled        *bool  `yaml:"enabled,omitempty"`
	Position       string `yaml:"position,omitempty"`
	ShowPageNumber *bool  `yaml:"showPageNumber,omitempty"`
	Date           string `yaml:"date,omitempty"`
	Status         string `yaml:"status,omitempty"`
	Text           string `yaml:"text,omitempty"`
	DocumentID     string `yaml:"documentID,omitempty"`
}

// SignatureFM mirrors picoloom.Signature plus Enabled gate.
type SignatureFM struct {
	Enabled      *bool    `yaml:"enabled,omitempty"`
	Name         string   `yaml:"name,omitempty"`
	Title        string   `yaml:"title,omitempty"`
	Email        string   `yaml:"email,omitempty"`
	Organization string   `yaml:"organization,omitempty"`
	ImagePath    string   `yaml:"imagePath,omitempty"`
	Phone        string   `yaml:"phone,omitempty"`
	Address      string   `yaml:"address,omitempty"`
	Department   string   `yaml:"department,omitempty"`
	Links        []LinkFM `yaml:"links,omitempty"`
}

// LinkFM mirrors picoloom.Link.
type LinkFM struct {
	Label string `yaml:"label"`
	URL   string `yaml:"url"`
}

// WatermarkFM mirrors picoloom.Watermark plus Enabled gate.
type WatermarkFM struct {
	Enabled *bool   `yaml:"enabled,omitempty"`
	Text    string  `yaml:"text,omitempty"`
	Color   string  `yaml:"color,omitempty"`
	Opacity float64 `yaml:"opacity,omitempty"`
	Angle   float64 `yaml:"angle,omitempty"`
}

// PageBreaksFM mirrors picoloom.PageBreaks plus Enabled gate.
type PageBreaksFM struct {
	Enabled  *bool `yaml:"enabled,omitempty"`
	BeforeH1 *bool `yaml:"beforeH1,omitempty"`
	BeforeH2 *bool `yaml:"beforeH2,omitempty"`
	BeforeH3 *bool `yaml:"beforeH3,omitempty"`
	Orphans  int   `yaml:"orphans,omitempty"`
	Widows   int   `yaml:"widows,omitempty"`
}

// ErrFrontmatterMalformed wraps every parse failure surfaced from
// ParseFrontmatter. Stage 4 (the renderer) treats this as "log a
// warning and use the merged-defaults Frontmatter" rather than a
// hard render failure — the body always survives.
var ErrFrontmatterMalformed = errors.New("pdf: frontmatter malformed")

var fmOpenRe = regexp.MustCompile(`(?m)\A---\s*\n`)
var fmCloseRe = regexp.MustCompile(`(?m)^---\s*$`)

// ParseFrontmatter splits a markdown source into a typed Frontmatter
// + the body. When the source has no leading `---\n…\n---` block,
// the returned Frontmatter is the zero value and body is the input
// verbatim. Malformed YAML, type mismatches, and a missing closing
// `---` produce a non-nil ErrFrontmatterMalformed; in that case the
// returned body is the input verbatim so the caller can still render
// the document with default settings.
//
// Stage 4 (render pipeline) passes this through after raymond
// expansion has already replaced `{{form.x}}` placeholders with
// concrete strings, so YAML decoding is the only thing happening
// here.
func ParseFrontmatter(md string) (Frontmatter, string, error) {
	if md == "" {
		return Frontmatter{}, "", nil
	}
	openLoc := fmOpenRe.FindStringIndex(md)
	if openLoc == nil {
		return Frontmatter{}, md, nil
	}
	rest := md[openLoc[1]:]
	closeLoc := fmCloseRe.FindStringIndex(rest)
	if closeLoc == nil {
		return Frontmatter{}, md, fmt.Errorf("%w: missing closing `---`", ErrFrontmatterMalformed)
	}
	raw := rest[:closeLoc[0]]
	body := rest[closeLoc[1]:]
	body = trimOneLeadingNewline(body)

	dec := yaml.NewDecoder(strings.NewReader(raw))
	dec.KnownFields(false)
	var fm Frontmatter
	if err := dec.Decode(&fm); err != nil {
		return Frontmatter{}, md, fmt.Errorf("%w: %v", ErrFrontmatterMalformed, err)
	}
	return fm, body, nil
}

// Merge combines layers into a single Frontmatter. Layers are passed
// in priority order: index 0 has the highest precedence (frontmatter
// from the document), the last index has the lowest (global config).
// Standard project convention is the four-layer call:
//
//	Merge(documentFM, formMetaFM, manifestFM, globalFM)
//
// Empty scalar fields and nil pointer fields cascade to the next
// layer. Sub-blocks merge field-by-field; a nil sub-block in a higher
// layer means "no opinion at this layer" and the lower layer's block
// (if any) is used verbatim where the higher's fields are unset.
//
// Slice fields (currently just Signature.Links) merge atomically: a
// non-empty slice in a higher layer fully replaces the lower's slice.
// Element-by-element merging is intentionally out of scope — links
// are inherently ordered, and partial overrides would surprise users.
func Merge(layers ...Frontmatter) Frontmatter {
	if len(layers) == 0 {
		return Frontmatter{}
	}
	// Walk from lowest priority to highest, overlaying each step.
	out := layers[len(layers)-1]
	for i := len(layers) - 2; i >= 0; i-- {
		out = overlay(layers[i], out)
	}
	return out
}

// overlay applies a higher-priority layer on top of a lower-priority
// base. Where higher has a value the result takes higher's; where
// higher is empty/nil the result keeps base's. Pure function.
func overlay(higher, base Frontmatter) Frontmatter {
	out := base
	if higher.Style != "" {
		out.Style = higher.Style
	}
	out.Page = overlayPage(higher.Page, base.Page)
	out.Cover = overlayCover(higher.Cover, base.Cover)
	out.TOC = overlayTOC(higher.TOC, base.TOC)
	out.Footer = overlayFooter(higher.Footer, base.Footer)
	out.Signature = overlaySignature(higher.Signature, base.Signature)
	out.Watermark = overlayWatermark(higher.Watermark, base.Watermark)
	out.PageBreaks = overlayPageBreaks(higher.PageBreaks, base.PageBreaks)
	return out
}

func overlayPage(h, b *PageFM) *PageFM {
	if h == nil {
		return b
	}
	if b == nil {
		cp := *h
		return &cp
	}
	out := *b
	if h.Size != "" {
		out.Size = h.Size
	}
	if h.Orientation != "" {
		out.Orientation = h.Orientation
	}
	if h.Margin != 0 {
		out.Margin = h.Margin
	}
	return &out
}

func overlayCover(h, b *CoverFM) *CoverFM {
	if h == nil {
		return b
	}
	if b == nil {
		cp := *h
		return &cp
	}
	out := *b
	if h.Enabled != nil {
		v := *h.Enabled
		out.Enabled = &v
	}
	pickString(&out.Title, h.Title)
	pickString(&out.Subtitle, h.Subtitle)
	pickString(&out.Logo, h.Logo)
	pickString(&out.Author, h.Author)
	pickString(&out.AuthorTitle, h.AuthorTitle)
	pickString(&out.Organization, h.Organization)
	pickString(&out.Date, h.Date)
	pickString(&out.Version, h.Version)
	pickString(&out.ClientName, h.ClientName)
	pickString(&out.ProjectName, h.ProjectName)
	pickString(&out.DocumentType, h.DocumentType)
	pickString(&out.DocumentID, h.DocumentID)
	pickString(&out.Description, h.Description)
	pickString(&out.Department, h.Department)
	return &out
}

func overlayTOC(h, b *TOCFM) *TOCFM {
	if h == nil {
		return b
	}
	if b == nil {
		cp := *h
		return &cp
	}
	out := *b
	if h.Enabled != nil {
		v := *h.Enabled
		out.Enabled = &v
	}
	pickString(&out.Title, h.Title)
	if h.MinDepth != 0 {
		out.MinDepth = h.MinDepth
	}
	if h.MaxDepth != 0 {
		out.MaxDepth = h.MaxDepth
	}
	return &out
}

func overlayFooter(h, b *FooterFM) *FooterFM {
	if h == nil {
		return b
	}
	if b == nil {
		cp := *h
		return &cp
	}
	out := *b
	if h.Enabled != nil {
		v := *h.Enabled
		out.Enabled = &v
	}
	if h.ShowPageNumber != nil {
		v := *h.ShowPageNumber
		out.ShowPageNumber = &v
	}
	pickString(&out.Position, h.Position)
	pickString(&out.Date, h.Date)
	pickString(&out.Status, h.Status)
	pickString(&out.Text, h.Text)
	pickString(&out.DocumentID, h.DocumentID)
	return &out
}

func overlaySignature(h, b *SignatureFM) *SignatureFM {
	if h == nil {
		return b
	}
	if b == nil {
		cp := *h
		return &cp
	}
	out := *b
	if h.Enabled != nil {
		v := *h.Enabled
		out.Enabled = &v
	}
	pickString(&out.Name, h.Name)
	pickString(&out.Title, h.Title)
	pickString(&out.Email, h.Email)
	pickString(&out.Organization, h.Organization)
	pickString(&out.ImagePath, h.ImagePath)
	pickString(&out.Phone, h.Phone)
	pickString(&out.Address, h.Address)
	pickString(&out.Department, h.Department)
	if len(h.Links) > 0 {
		out.Links = append([]LinkFM(nil), h.Links...)
	}
	return &out
}

func overlayWatermark(h, b *WatermarkFM) *WatermarkFM {
	if h == nil {
		return b
	}
	if b == nil {
		cp := *h
		return &cp
	}
	out := *b
	if h.Enabled != nil {
		v := *h.Enabled
		out.Enabled = &v
	}
	pickString(&out.Text, h.Text)
	pickString(&out.Color, h.Color)
	if h.Opacity != 0 {
		out.Opacity = h.Opacity
	}
	if h.Angle != 0 {
		out.Angle = h.Angle
	}
	return &out
}

func overlayPageBreaks(h, b *PageBreaksFM) *PageBreaksFM {
	if h == nil {
		return b
	}
	if b == nil {
		cp := *h
		return &cp
	}
	out := *b
	if h.Enabled != nil {
		v := *h.Enabled
		out.Enabled = &v
	}
	if h.BeforeH1 != nil {
		v := *h.BeforeH1
		out.BeforeH1 = &v
	}
	if h.BeforeH2 != nil {
		v := *h.BeforeH2
		out.BeforeH2 = &v
	}
	if h.BeforeH3 != nil {
		v := *h.BeforeH3
		out.BeforeH3 = &v
	}
	if h.Orphans != 0 {
		out.Orphans = h.Orphans
	}
	if h.Widows != 0 {
		out.Widows = h.Widows
	}
	return &out
}

func pickString(dst *string, v string) {
	if v != "" {
		*dst = v
	}
}

// BuildInput projects a merged Frontmatter into a picoloom.Input.
// Pure function: no I/O, no Chrome, no file reads. Style is NOT part
// of picoloom.Input — the caller (Stage 4 render pipeline) reads
// fm.Style and passes it to picoloom.NewConverter via WithStyle().
//
// A sub-block is included in the Input only when the corresponding
// Frontmatter sub-block is non-nil AND its Enabled flag is not
// explicitly false. Block presence with no explicit Enabled defaults
// to opted-in — this matches the design-doc convention "if the
// author wrote `cover:` they probably meant to use it".
func BuildInput(fm Frontmatter, body string) picoloom.Input {
	in := picoloom.Input{Markdown: body}
	if fm.Page != nil {
		in.Page = &picoloom.PageSettings{
			Size:        fm.Page.Size,
			Orientation: fm.Page.Orientation,
			Margin:      fm.Page.Margin,
		}
	}
	if fm.Cover != nil && boolOrTrue(fm.Cover.Enabled) {
		in.Cover = &picoloom.Cover{
			Title:        fm.Cover.Title,
			Subtitle:     fm.Cover.Subtitle,
			Logo:         fm.Cover.Logo,
			Author:       fm.Cover.Author,
			AuthorTitle:  fm.Cover.AuthorTitle,
			Organization: fm.Cover.Organization,
			Date:         fm.Cover.Date,
			Version:      fm.Cover.Version,
			ClientName:   fm.Cover.ClientName,
			ProjectName:  fm.Cover.ProjectName,
			DocumentType: fm.Cover.DocumentType,
			DocumentID:   fm.Cover.DocumentID,
			Description:  fm.Cover.Description,
			Department:   fm.Cover.Department,
		}
	}
	if fm.TOC != nil && boolOrTrue(fm.TOC.Enabled) {
		in.TOC = &picoloom.TOC{
			Title:    fm.TOC.Title,
			MinDepth: fm.TOC.MinDepth,
			MaxDepth: fm.TOC.MaxDepth,
		}
	}
	if fm.Footer != nil && boolOrTrue(fm.Footer.Enabled) {
		in.Footer = &picoloom.Footer{
			Position:       fm.Footer.Position,
			ShowPageNumber: boolOrFalse(fm.Footer.ShowPageNumber),
			Date:           fm.Footer.Date,
			Status:         fm.Footer.Status,
			Text:           fm.Footer.Text,
			DocumentID:     fm.Footer.DocumentID,
		}
	}
	if fm.Signature != nil && boolOrTrue(fm.Signature.Enabled) {
		sig := &picoloom.Signature{
			Name:         fm.Signature.Name,
			Title:        fm.Signature.Title,
			Email:        fm.Signature.Email,
			Organization: fm.Signature.Organization,
			ImagePath:    fm.Signature.ImagePath,
			Phone:        fm.Signature.Phone,
			Address:      fm.Signature.Address,
			Department:   fm.Signature.Department,
		}
		if len(fm.Signature.Links) > 0 {
			sig.Links = make([]picoloom.Link, len(fm.Signature.Links))
			for i, l := range fm.Signature.Links {
				sig.Links[i] = picoloom.Link{Label: l.Label, URL: l.URL}
			}
		}
		in.Signature = sig
	}
	if fm.Watermark != nil && boolOrTrue(fm.Watermark.Enabled) {
		in.Watermark = &picoloom.Watermark{
			Text:    fm.Watermark.Text,
			Color:   fm.Watermark.Color,
			Opacity: fm.Watermark.Opacity,
			Angle:   fm.Watermark.Angle,
		}
	}
	if fm.PageBreaks != nil && boolOrTrue(fm.PageBreaks.Enabled) {
		in.PageBreaks = &picoloom.PageBreaks{
			BeforeH1: boolOrFalse(fm.PageBreaks.BeforeH1),
			BeforeH2: boolOrFalse(fm.PageBreaks.BeforeH2),
			BeforeH3: boolOrFalse(fm.PageBreaks.BeforeH3),
			Orphans:  fm.PageBreaks.Orphans,
			Widows:   fm.PageBreaks.Widows,
		}
	}
	return in
}

func boolOrTrue(b *bool) bool {
	if b == nil {
		return true
	}
	return *b
}

func boolOrFalse(b *bool) bool {
	if b == nil {
		return false
	}
	return *b
}

func trimOneLeadingNewline(s string) string {
	if len(s) > 0 && s[0] == '\n' {
		return s[1:]
	}
	return s
}
