package graph

import (
	"slices"

	"github.com/mattn/go-runewidth"
	log "github.com/sirupsen/logrus"
)

// edgeStroke is the line style drawn along an edge's path. Only ASCII
// output (g.useAscii) currently distinguishes them; Unicode output always
// renders a solid line regardless of stroke.
type edgeStroke int

const (
	strokeSolid edgeStroke = iota
	strokeDotted
	strokeThick
)

// edgeHead is the glyph drawn at an edge's arrowhead end. headNone means no
// glyph is drawn at all (an open link like ---). Only ASCII output currently
// distinguishes them; Unicode output always draws a directional arrowhead.
type edgeHead int

const (
	headArrow edgeHead = iota
	headCircle
	headCross
	headNone
)

type edge struct {
	from            *node
	to              *node
	text            string
	isBidirectional bool
	stroke          edgeStroke
	head            edgeHead
	path            []gridCoord
	labelLine       []gridCoord
	labelCell       *gridCoord
	labelVertical   bool
	fanout          bool
	startDir        direction
	endDir          direction
}

func (g *graph) determinePath(e *edge) {
	key := newEdgePair(e.from.index, e.to.index)
	duplicateIndex := g.edgeCounts[key]

	pairs := g.sidePairs(e)
	if e.from == e.to {
		pairs = slices.DeleteFunc(pairs, func(p sidePair) bool { return p.start == p.end })
	}
	if startDir, endDir, ok := g.parallelDirections(e, duplicateIndex); ok {
		pairs = append([]sidePair{{startDir, endDir, 0}}, pairs...)
	}
	if !g.routeEdge(e, pairs) {
		log.Warnf("No path found from %v to %v", e.from, e.to)
	}
	log.Debugf("Routed %s -> %s from %v to %v along %v", e.from.name, e.to.name, e.startDir, e.endDir, e.path)
	g.recordEdgeCells(e)
	g.edgeCounts[key]++
}

func (g *graph) parallelDirections(e *edge, duplicateIndex int) (direction, direction, bool) {
	if duplicateIndex == 0 {
		return Middle, Middle, false
	}

	dir := determineDirection(genericCoord(*e.from.gridCoord), genericCoord(*e.to.gridCoord))
	switch {
	case g.graphDirection == "LR" && (dir == Right || dir == Left):
		options := [][2]direction{{Down, Down}, {Up, Up}}
		if duplicateIndex-1 < len(options) {
			return options[duplicateIndex-1][0], options[duplicateIndex-1][1], true
		}
	case g.graphDirection == "TD" && (dir == Down || dir == Up):
		options := [][2]direction{{Right, Right}, {Left, Left}}
		if duplicateIndex-1 < len(options) {
			return options[duplicateIndex-1][0], options[duplicateIndex-1][1], true
		}
	}

	return Middle, Middle, false
}

// determineLabelLine picks the cell on e's path that carries its label. The
// cell is one no other edge uses, so the label names a single line, and it
// lies outside every node's border column, since widening that column
// detaches the box from its ports. The cell's column, and for a vertical
// run its row, grows to fit the label.
func (g *graph) determineLabelLine(e *edge) {
	if e.text == "" || e.fanout || len(e.path) < 2 {
		return
	}
	cells := expandPath(e.path)
	best, bestScore, fallback := -1, 0, -1
	for i := 1; i < len(cells)-1; i++ {
		prev, c, next := cells[i-1], cells[i], cells[i+1]
		vertical := prev.x == c.x && c.x == next.x
		horizontal := prev.y == c.y && c.y == next.y
		if !vertical && !horizontal {
			continue
		}
		if fallback < 0 {
			fallback = i
		}
		if !g.labelCellFree(e, c, vertical) {
			continue
		}
		score := 0
		if vertical != (g.graphDirection == "TD") {
			score++
		}
		if g.isNodeColumn(c.x) {
			score += 2
		}
		if vertical && g.isNodeRow(c.y) {
			score += 2
		}
		if best < 0 || score < bestScore {
			best, bestScore = i, score
		}
	}
	if best < 0 && e.labelCell != nil {
		g.labelCells[*e.labelCell] = e
		return
	}
	if best < 0 {
		log.Debugf("No free cell for the label %q from %v to %v", e.text, e.from, e.to)
		best = fallback
	}
	if best < 0 {
		best = 1
	}
	c := cells[best]
	e.labelCell = &c
	e.labelVertical = cells[best-1].x == c.x
	g.labelCells[c] = e
}

// relocateSharedLabels moves each label whose cell an edge routed after it
// came to share, onto a cell of its own path that no other edge uses.
func (g *graph) relocateSharedLabels() {
	for _, e := range g.edges {
		if e.fanout || e.labelCell == nil || !g.cellShared(e, *e.labelCell) {
			continue
		}
		delete(g.labelCells, *e.labelCell)
		g.determineLabelLine(e)
	}
}

func (g *graph) sizeLabelCell(e *edge) {
	if e.fanout || e.labelCell == nil {
		return
	}
	c := *e.labelCell
	width := runewidth.StringWidth(e.text)
	if !g.isNodeBorderColumn(c.x) {
		if e.labelVertical {
			g.columnWidth[c.x] = Max(g.columnWidth[c.x], width+2)
		} else {
			g.columnWidth[c.x] = Max(g.columnWidth[c.x], width+labelPaddingForEdge(e))
		}
	}
	if e.labelVertical && !g.isNodeBorderRow(c.y) {
		g.rowHeight[c.y] = Max(g.rowHeight[c.y], 3)
	}
}

func rangesOverlap(firstStart, firstEnd, secondStart, secondEnd int) bool {
	firstMin, firstMax := Min(firstStart, firstEnd), Max(firstStart, firstEnd)
	secondMin, secondMax := Min(secondStart, secondEnd), Max(secondStart, secondEnd)
	return firstMin <= secondMax && secondMin <= firstMax
}

func labelPaddingForEdge(e *edge) int {
	if e.isBidirectional {
		return 4 // start arrowhead + dashes + end arrowhead
	}
	return 3 // dashes + end arrowhead
}

// cellShared reports whether any edge other than e runs through c.
func (g *graph) cellShared(e *edge, c gridCoord) bool {
	for _, use := range g.cellUses[c] {
		if use.e != e {
			return true
		}
	}
	return false
}

func (g *graph) isNodeColumn(x int) bool {
	for _, n := range g.nodes {
		if n.gridCoord != nil && x >= n.gridCoord.x && x <= n.gridCoord.x+2 {
			return true
		}
	}
	return false
}

func (g *graph) isNodeRow(y int) bool {
	for _, n := range g.nodes {
		if n.gridCoord != nil && y >= n.gridCoord.y && y <= n.gridCoord.y+2 {
			return true
		}
	}
	return false
}

func (g *graph) isNodeBorderColumn(x int) bool {
	for _, n := range g.nodes {
		if n.gridCoord != nil && (x == n.gridCoord.x || x == n.gridCoord.x+2) {
			return true
		}
	}
	return false
}

func (g *graph) isNodeBorderRow(y int) bool {
	for _, n := range g.nodes {
		if n.gridCoord != nil && (y == n.gridCoord.y || y == n.gridCoord.y+2) {
			return true
		}
	}
	return false
}
