package graph

import (
	"fmt"
	"sort"
	"strings"
)

// layoutFinding is one place the drawing could not keep apart, with the
// edges it names.
type layoutFinding struct {
	text  string
	edges []*edge
}

// layoutFindings lists each place the drawing could not keep apart, where a
// reader could misread the figure: edges drawn as one line, an arrowhead on
// another edge's line, lines that join a source to a target no edge joins, a
// label on a shared line, and a box or label that a later layer overwrote.
func (g *graph) layoutFindings() []layoutFinding {
	found := map[string][]*edge{}
	add := func(edges []*edge, format string, args ...interface{}) {
		text := fmt.Sprintf(format, args...)
		found[text] = append(found[text], edges...)
	}
	for _, e := range g.edges {
		if len(e.path) < 2 {
			add([]*edge{e}, "no route from %s to %s", nodeText(e.from), nodeText(e.to))
		}
	}
	for c, uses := range g.cellUses {
		for i, a := range uses {
			for _, b := range uses[i+1:] {
				if a.e == b.e || bundled(a.e, b.e) {
					continue
				}
				if (a.horizontal && b.horizontal) || (a.vertical && b.vertical) {
					add([]*edge{a.e, b.e}, "edges %s and %s share a line", edgeText(a.e), edgeText(b.e))
				}
			}
			if !a.head || g.roomyHead(c, a.headHorizontal) {
				continue
			}
			for _, other := range uses {
				if other.e != a.e && other.e.to != a.e.to && (!a.e.fanout || !other.e.fanout) {
					add([]*edge{a.e, other.e}, "the arrowhead of %s sits on the line of %s", edgeText(a.e), edgeText(other.e))
				}
			}
		}
	}
	for _, path := range g.falseJoins() {
		a, b, c := path[0], path[1], path[2]
		add(path[:], "%s reads as reaching %s: the lines of %s, %s and %s join",
			nodeText(c.from), nodeText(a.to), edgeText(a), edgeText(b), edgeText(c))
	}
	for c, e := range g.labelCells {
		if !e.fanout && g.cellShared(e, c) {
			edges := []*edge{e}
			for _, use := range g.cellUses[c] {
				edges = append(edges, use.e)
			}
			add(edges, "the label %q sits on a line another edge shares", e.text)
		}
	}
	for _, n := range g.nodes {
		if !g.boxIntact(n) {
			add(nil, "an edge or label overwrites the box %s", nodeText(n))
		}
	}
	for _, e := range g.edges {
		if e.text != "" && !g.labelIntact(e) {
			add([]*edge{e}, "the label %q is overwritten", e.text)
		}
	}
	findings := make([]layoutFinding, 0, len(found))
	for text, edges := range found {
		findings = append(findings, layoutFinding{text, edges})
	}
	sort.Slice(findings, func(i, j int) bool { return findings[i].text < findings[j].text })
	return findings
}

// falseJoins finds each edge b whose line joins both an edge a from its
// source and an edge c into its target, where no edge runs from c's source
// to a's target. Each is returned as {a, b, c}.
func (g *graph) falseJoins() [][3]*edge {
	fanOut := map[*edge]map[*edge]bool{}
	fanIn := map[*edge]map[*edge]bool{}
	for _, uses := range g.cellUses {
		for _, x := range uses {
			for _, y := range uses {
				if x.e == y.e {
					continue
				}
				switch shareKindOf(x.e, y.e) {
				case shareFanOut:
					addEdgeTo(fanOut, x.e, y.e)
				case shareFanIn:
					addEdgeTo(fanIn, x.e, y.e)
				}
			}
		}
	}
	var joins [][3]*edge
	for _, b := range g.edges {
		for a := range fanOut[b] {
			for c := range fanIn[b] {
				if !g.hasEdge(c.from, a.to) {
					joins = append(joins, [3]*edge{a, b, c})
				}
			}
		}
	}
	return joins
}

func addEdgeTo(m map[*edge]map[*edge]bool, key, value *edge) {
	if m[key] == nil {
		m[key] = map[*edge]bool{}
	}
	m[key][value] = true
}

func (g *graph) hasEdge(from, to *node) bool {
	for _, e := range g.edges {
		if e.from == from && e.to == to {
			return true
		}
	}
	return false
}

// boxIntact compares a node's box with the final drawing. A border cell may
// carry the junction of an edge leaving or entering there, and nothing else
// may differ.
func (g *graph) boxIntact(n *node) bool {
	if n.drawing == nil || n.drawingCoord == nil {
		return true
	}
	box := *n.drawing
	width, height := len(box)-1, len(box[0])-1
	for x := 0; x <= width; x++ {
		for y := 0; y <= height; y++ {
			px, py := x+n.drawingCoord.x, y+n.drawingCoord.y
			if px >= len(*g.drawing) || py >= len((*g.drawing)[px]) {
				return false
			}
			want, got := box[x][y], (*g.drawing)[px][py]
			if want == got {
				continue
			}
			border := x == 0 || y == 0 || x == width || y == height
			if !border || !isBorderJunction(got) {
				return false
			}
		}
	}
	return true
}

func isBorderJunction(c string) bool {
	switch c {
	case "┬", "┴", "├", "┤", "+":
		return true
	}
	return false
}

func (g *graph) labelIntact(e *edge) bool {
	var start drawingCoord
	switch {
	case e.fanout:
		x, _, y := g.fanoutLabelBounds(e)
		start = drawingCoord{x: x, y: y}
	case e.labelCell != nil:
		start = g.labelStart(e)
	default:
		return true
	}
	x := start.x
	for _, cell := range textCells(e.text) {
		if x < 0 || x >= len(*g.drawing) || start.y < 0 || start.y >= len((*g.drawing)[x]) {
			return false
		}
		if (*g.drawing)[x][start.y] != cell.text {
			return false
		}
		x += cell.width
	}
	return true
}

func nodeText(n *node) string {
	text := strings.Join(n.label.lines, " ")
	if text == "" {
		text = n.name
	}
	return fmt.Sprintf("%q", text)
}

func edgeText(e *edge) string {
	return nodeText(e.from) + " -> " + nodeText(e.to)
}
