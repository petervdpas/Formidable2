package pdf

import (
	"reflect"
	"sort"
	"testing"
)

// extractCoverImageRefs is exercised here in isolation because it's
// the heart of the cover-archive bundling: which assets to ship.
// Coverage targets every gotcha that bit the original Formidable
// (Electron) team-sharing flow: template placeholders, URLs, CSS
// background images, duplicates.
func TestExtractCoverImageRefs(t *testing.T) {
	cases := []struct {
		name string
		html string
		want []string
	}{
		{
			name: "empty",
			html: "",
			want: nil,
		},
		{
			name: "single img src bare filename",
			html: `<img src="formidable.svg">`,
			want: []string{"formidable.svg"},
		},
		{
			name: "single img src nested under images/",
			html: `<img src="images/logo.png">`,
			want: []string{"images/logo.png"},
		},
		{
			name: "css url() in inline style block",
			html: `<style>.header { background-image: url('images/header.jpg'); }</style>`,
			want: []string{"images/header.jpg"},
		},
		{
			name: "double-quoted css url",
			html: `<style>.h{background:url("logo.png")}</style>`,
			want: []string{"logo.png"},
		},
		{
			name: "unquoted css url",
			html: `<style>.h{background:url(banner.svg)}</style>`,
			want: []string{"banner.svg"},
		},
		{
			name: "img + css mixed, deduplicated",
			html: `<img src="logo.png"><style>.h{background:url("logo.png")}</style>`,
			want: []string{"logo.png"},
		},
		{
			name: "skip handlebars template placeholder",
			html: `<img src="{{.Logo}}">`,
			want: nil,
		},
		{
			name: "skip raymond/handlebars expression inside path",
			html: `<img src="{{logoPath}}">`,
			want: nil,
		},
		{
			name: "skip http URL",
			html: `<img src="http://example.com/logo.png">`,
			want: nil,
		},
		{
			name: "skip https URL",
			html: `<img src="https://example.com/logo.png">`,
			want: nil,
		},
		{
			name: "skip data URI",
			html: `<img src="data:image/png;base64,AAAA">`,
			want: nil,
		},
		{
			name: "skip file:// URL",
			html: `<img src="file:///tmp/x.png">`,
			want: nil,
		},
		{
			name: "skip protocol-relative URL",
			html: `<img src="//cdn.example.com/logo.png">`,
			want: nil,
		},
		{
			name: "skip absolute filesystem path",
			html: `<img src="/etc/logo.png">`,
			want: nil,
		},
		{
			name: "skip CSS url with template placeholder",
			html: `<style>.h{background:url("{{logo}}")}</style>`,
			want: nil,
		},
		{
			name: "multiple distinct img refs collected",
			html: `<img src="a.png"><img src="b.png"><img src="images/c.svg">`,
			want: []string{"a.png", "b.png", "images/c.svg"},
		},
		{
			name: "img with whitespace in attribute",
			html: `<img  src = "spaced.png"  alt="x">`,
			want: []string{"spaced.png"},
		},
		{
			name: "single quotes on img src",
			html: `<img src='logo.png'>`,
			want: []string{"logo.png"},
		},
		{
			name: "css url with whitespace around argument",
			html: `<style>.h{background:url( spaced.png )}</style>`,
			want: []string{"spaced.png"},
		},
		{
			name: "ignore srcset (out of scope, picoloom also skips)",
			html: `<img src="logo.png" srcset="logo@2x.png 2x, logo@3x.png 3x">`,
			want: []string{"logo.png"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := extractCoverImageRefs(tc.html)
			sort.Strings(got)
			want := append([]string(nil), tc.want...)
			sort.Strings(want)
			if want == nil && len(got) == 0 {
				return
			}
			if !reflect.DeepEqual(got, want) {
				t.Errorf("got %#v, want %#v", got, want)
			}
		})
	}
}
