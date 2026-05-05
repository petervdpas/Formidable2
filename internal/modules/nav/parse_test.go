package nav

import "testing"

func TestParseFormidableHref(t *testing.T) {
	cases := []struct {
		name    string
		in      string
		want    *Target
	}{
		{
			"basic",
			"formidable://basic.yaml:test.meta.json",
			&Target{Template: "basic.yaml", Datafile: "test.meta.json"},
		},
		{
			"with fragment",
			"formidable://recepten.yaml:groentenschotel.meta.json#ingredients",
			&Target{Template: "recepten.yaml", Datafile: "groentenschotel.meta.json", Fragment: "ingredients"},
		},
		{
			"no scheme",
			"https://example.com",
			nil,
		},
		{
			"empty",
			"",
			nil,
		},
		{
			"missing colon between template and datafile",
			"formidable://no-colon-here",
			nil,
		},
		{
			"empty template",
			"formidable://:datafile.meta.json",
			nil,
		},
		{
			"empty datafile",
			"formidable://tpl.yaml:",
			nil,
		},
		{
			"datafile with colon",
			"formidable://tpl.yaml:weird:name.meta.json",
			// lastIndexOf-style split: weirder filenames stay on the
			// datafile side. Mirrors the JS parser exactly.
			&Target{Template: "tpl.yaml:weird", Datafile: "name.meta.json"},
		},
		{
			"path traversal in template",
			"formidable://../escape:foo.meta.json",
			nil,
		},
		{
			"path traversal in datafile",
			"formidable://tpl.yaml:../escape.meta.json",
			nil,
		},
		{
			"slash in template",
			"formidable://sub/tpl.yaml:foo.meta.json",
			nil,
		},
		{
			"slash in datafile",
			"formidable://tpl.yaml:sub/foo.meta.json",
			nil,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := ParseFormidableHref(c.in)
			if (got == nil) != (c.want == nil) {
				t.Fatalf("got %v, want %v", got, c.want)
			}
			if got == nil {
				return
			}
			if *got != *c.want {
				t.Errorf("got %+v, want %+v", *got, *c.want)
			}
		})
	}
}
