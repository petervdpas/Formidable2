package plugin

import (
	"net/url"
	"testing"

	"github.com/petervdpas/formidable2/internal/modules/template"
)

// A diagram field renders as an image reference to its generated file. Export
// plugins (wikiwonder) find images by the /api/images/<stem>/<file> pattern and
// copy them, so that reference must survive formidable.rewrite.markdown.
func TestBindings_Rewrite_Markdown_PicksUpDiagramImages(t *testing.T) {
	name := url.PathEscape(template.DiagramImageName("datastroom.yaml", "progress-fcdm-cursus.meta.json", "diagram_test"))
	res := run(t, `
		function run()
			local md = "![Data lineage](/api/images/datastroom/`+name+`)"
			local out, images = formidable.rewrite.markdown(md, {
				template_stem = "datastroom",
				image_path_prefix = ".images/",
			})
			local names = {}
			for n, _ in pairs(images) do table.insert(names, n) end
			return { md = out, image = names[1], count = #names }
		end`, scriptOpts{})
	got := res.Value.(map[string]any)
	if got["md"] != "![Data lineage](.images/"+name+")" || got["image"] != name || got["count"] != float64(1) {
		t.Fatalf("got %+v, want the diagram rewritten to .images/%s and collected once", got, name)
	}
}

// In the studio a diagram image URL carries "?v=<hash>" so a redrawn image
// reloads. An export must copy the plain file and link to it without the version.
func TestBindings_Rewrite_Markdown_DropsTheImageVersion(t *testing.T) {
	res := run(t, `
		function run()
			local md = "![d](/api/images/datastroom/a.b.diagram.svg?v=3fa9c1d2) ![p](/api/images/datastroom/cover%20art.png)"
			local out, images = formidable.rewrite.markdown(md, {
				template_stem = "datastroom",
				image_path_prefix = ".images/",
			})
			local names = {}
			for n, _ in pairs(images) do table.insert(names, n) end
			table.sort(names)
			return { md = out, first = names[1], second = names[2], count = #names }
		end`, scriptOpts{})
	got := res.Value.(map[string]any)
	if got["md"] != "![d](.images/a.b.diagram.svg) ![p](.images/cover%20art.png)" {
		t.Fatalf("md = %q", got["md"])
	}
	if got["first"] != "a.b.diagram.svg" || got["second"] != "cover%20art.png" || got["count"] != float64(2) {
		t.Fatalf("collected = %+v, want the two plain file names", got)
	}
}
