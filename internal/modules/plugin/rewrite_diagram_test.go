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
