package graph

import (
	"slices"
	"testing"
)

func TestLayoutFindingsNameEdgesDrawnAsOneLine(t *testing.T) {
	properties, err := Parse("graph LR\nA --> B\nC --> D\n", "cli")
	if err != nil {
		t.Fatal(err)
	}
	attempt := layOut(properties, nil)
	if len(attempt.findings) != 0 {
		t.Fatalf("separate edges reported findings: %v", attempt.findings)
	}

	g := attempt.g
	ab, cd := g.edges[0], g.edges[1]
	cd.path = ab.path
	g.recordEdgeCells(cd)
	var texts []string
	for _, f := range g.layoutFindings() {
		texts = append(texts, f.text)
	}
	want := `edges "A" -> "B" and "C" -> "D" share a line`
	if !slices.Contains(texts, want) {
		t.Errorf("findings %q do not include %q", texts, want)
	}
}
