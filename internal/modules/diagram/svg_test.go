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

func TestSVG_EdgeColourFollowsFirstRuleCode(t *testing.T) {
	if EdgeColour("Q-01") != EdgeColour("Q-01, C-02") || EdgeColour(" q-01 ") != EdgeColour("Q-01") {
		t.Fatal("same first code should share a colour")
	}
	if EdgeColour("") != edgeNeutral {
		t.Fatalf("unlabelled colour = %q, want neutral", EdgeColour(""))
	}
	seen := map[string]bool{}
	for _, c := range []string{"Q-01", "Q-02", "Q-03", "C-01", "C-02", "E-01", "F-01", "V-01"} {
		seen[EdgeColour(c)] = true
	}
	if len(seen) < 4 {
		t.Fatalf("only %d distinct colours across 8 codes", len(seen))
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
