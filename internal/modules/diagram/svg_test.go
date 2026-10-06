package diagram

import (
	"encoding/xml"
	"io"
	"strings"
	"testing"
)

func wellFormed(t *testing.T, s string) {
	t.Helper()
	d := xml.NewDecoder(strings.NewReader(s))
	for {
		_, err := d.Token()
		if err == io.EOF {
			return
		}
		if err != nil {
			t.Fatalf("not well-formed XML: %v\n%s", err, s)
		}
	}
}

func TestSVG_EmptyLayoutEmitsNothing(t *testing.T) {
	if got := SVG(Place(Graph{})); got != "" {
		t.Fatalf("SVG = %q, want empty", got)
	}
}

func TestSVG_WellFormedWithViewBoxAndLabels(t *testing.T) {
	l := Place(Lineage([][]string{
		{"Basisvariant", "StudielinkVariantCode", "AanmeldingInschrijving", "OpleidingvariantBk", "Q-01"},
		{"", "", "AanmeldingInschrijving", "Bk", "C-01"},
	}, testCols))
	s := SVG(l)
	wellFormed(t, s)
	if !strings.HasPrefix(s, `<svg xmlns="http://www.w3.org/2000/svg"`) || !strings.Contains(s, `viewBox="0 0 `) {
		t.Fatalf("missing svg root/viewBox: %.120s", s)
	}
	for _, want := range []string{"Basisvariant", "StudielinkVariantCode", "AanmeldingInschrijving", "OpleidingvariantBk", "Bk", "C-01"} {
		if !strings.Contains(s, ">"+want+"<") {
			t.Fatalf("label %q not emitted", want)
		}
	}
	if n := strings.Count(s, "<path data-edge="); n != len(l.Edges) {
		t.Fatalf("paths = %d, want %d", n, len(l.Edges))
	}
}

func TestSVG_EscapesText(t *testing.T) {
	s := SVG(Place(Lineage([][]string{{`A<&>"'`, "x<y", "T", "a&b", `Q"1`}}, testCols)))
	wellFormed(t, s)
	if strings.Contains(s, "x<y") || !strings.Contains(s, "x&lt;y") {
		t.Fatal("port label not escaped")
	}
}

func TestSVG_HasNoActiveContent(t *testing.T) {
	s := strings.ToLower(SVG(Place(Lineage([][]string{{"<script>alert(1)</script>", "onload=x", "T", "javascript:a", ""}}, testCols))))
	for _, bad := range []string{"<script", "<style", "<foreignobject", " onload=", "href="} {
		if strings.Contains(s, bad) {
			t.Fatalf("found %q in output", bad)
		}
	}
}

func TestSVG_EdgeColourFollowsCategoryOfFirstCode(t *testing.T) {
	if EdgeColour("Q-01") != EdgeColour("Q-07, C-02") || EdgeColour(" q-01 ") != EdgeColour("Q-01") {
		t.Fatal("codes of one category share a colour; the first listed code decides")
	}
	if EdgeColour("") != edgeNeutral || EdgeColour(" , ") != edgeNeutral || EdgeColour("123") != edgeNeutral {
		t.Fatal("no category letter -> neutral")
	}
	seen := map[string]bool{}
	for _, c := range []string{"C-01", "E-01", "F-01", "M-01", "Q-01", "V-01"} {
		seen[EdgeColour(c)] = true
	}
	if len(seen) != 6 {
		t.Fatalf("the six TR categories need six distinct colours, got %d", len(seen))
	}
	if EdgeColour("X-01") == edgeNeutral {
		t.Fatal("an unknown category still gets a palette colour")
	}
}

func TestCategory(t *testing.T) {
	cases := map[string]string{"Q-01": "Q", "q-01": "Q", "CX12": "CX", " E-1": "E", "": "", "12": "", "-Q": ""}
	for in, want := range cases {
		if got := category(in); got != want {
			t.Errorf("category(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSVG_LegendListsCategoriesPresent(t *testing.T) {
	s := SVG(Place(Lineage([][]string{
		{"", "", "T", "a", "Q-01, C-01"},
		{"A", "x", "T", "b", "Q-02"},
	}, testCols)))
	wellFormed(t, s)
	if !strings.Contains(s, `data-legend="C"`) || !strings.Contains(s, `data-legend="Q"`) || strings.Count(s, "data-legend=") != 2 {
		t.Fatalf("legend wrong in %s", s)
	}
	if strings.Contains(SVG(Place(Lineage([][]string{{"A", "x", "T", "y"}}, testCols))), "data-legend=") {
		t.Fatal("no codes, no legend")
	}
}

func TestSVG_RulePillTakesItsCategoryColour(t *testing.T) {
	s := SVG(Place(Lineage([][]string{{"", "", "T", "a", "C-01"}}, testCols)))
	if !strings.Contains(s, `data-node="rule:c-01"`) || !strings.Contains(s, `stroke="`+EdgeColour("C-01")+`"`) {
		t.Fatalf("pill not tinted: %s", s)
	}
}

func TestSVG_TooltipCarriesExampleValue(t *testing.T) {
	s := SVG(Place(Lineage([][]string{{"A", "x", "T", "y", "Q-01", "!!NULL"}}, testCols)))
	if !strings.Contains(s, "<title>A.x -&gt; T.y (Q-01) = !!NULL</title>") {
		t.Fatalf("tooltip missing example in %s", s)
	}
}

func TestSVG_LiteralPillIsDrawn(t *testing.T) {
	s := SVG(Place(Lineage([][]string{{"", "", "T", "kind", "", "3"}}, testCols)))
	wellFormed(t, s)
	if !strings.Contains(s, `data-node="literal:target:t:kind"`) || !strings.Contains(s, ">3<") {
		t.Fatalf("literal missing in %s", s)
	}
}

func TestSVG_MarksUnmappedAndUnsourcedPorts(t *testing.T) {
	s := SVG(Place(Lineage([][]string{
		{"A", "joinkey", "", "", ""},
		{"", "", "T", "orphan", ""},
	}, testCols)))
	if !strings.Contains(s, `data-class="unmapped"`) || !strings.Contains(s, `data-class="unsourced"`) {
		t.Fatal("port classes not emitted")
	}
}

func TestSVG_EdgeTooltipNamesBothEnds(t *testing.T) {
	s := SVG(Place(Lineage([][]string{{"A", "x", "T", "y", "Q-01"}}, testCols)))
	if !strings.Contains(s, "<title>A.x -&gt; T.y (Q-01)</title>") {
		t.Fatalf("tooltip missing in %s", s)
	}
}

func TestSVG_IsDeterministic(t *testing.T) {
	rows := [][]string{{"A", "x", "T", "c", "R1"}, {"", "", "U", "d", "R2"}, {"C", "w", "", "", "R2"}}
	first := SVG(Place(Lineage(rows, testCols)))
	for range 10 {
		if SVG(Place(Lineage(rows, testCols))) != first {
			t.Fatal("output differs between runs")
		}
	}
}

func TestSVG_OnlyInPageLinksAreEmitted(t *testing.T) {
	g := Lineage([][]string{{"", "", "T", "a", "Q-01"}}, testCols)
	for i := range g.Nodes {
		if g.Nodes[i].Kind == KindRule {
			g.Nodes[i].Link = "javascript:alert(1)"
		}
	}
	if s := SVG(Place(g)); strings.Contains(s, "href=") {
		t.Fatalf("non-fragment link emitted: %s", s)
	}
}
