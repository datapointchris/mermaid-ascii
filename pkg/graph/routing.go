package graph

import (
	"container/heap"
	"fmt"
)

// Costs the router charges on top of one per cell. Running along another
// edge, or turning on its line, draws the two as one line, so it is priced
// to be taken only when no detour exists.
const (
	turnCost      = 2
	crossingCost  = 3
	overlapCost   = 50
	portCost      = 60
	labelRoomCost = 30
	sidePairBias  = 6
)

var sides = []direction{Up, Down, Left, Right}

var routeSteps = []gridCoord{{1, 0}, {-1, 0}, {0, 1}, {0, -1}}

type cellUse struct {
	e              *edge
	horizontal     bool
	vertical       bool
	head           bool
	headHorizontal bool
}

type portUse struct {
	e        *edge
	outgoing bool
}

type sidePair struct {
	start direction
	end   direction
	bias  int
}

// bundled reports whether two edges may share cells: a fan-out from one
// source or a fan-in to one target. Two edges joining the same pair share
// both ends, and drawn on one line their labels would collide.
func bundled(a, b *edge) bool {
	return (a.from == b.from) != (a.to == b.to)
}

// shareKind is the one way an edge's line may join others: with edges from
// its source or with edges into its target. A line joined both ways draws a
// path from a source to a target that no edge joins.
type shareKind int

const (
	shareNone shareKind = iota
	shareFanOut
	shareFanIn
)

func shareKindOf(a, b *edge) shareKind {
	switch {
	case !bundled(a, b):
		return shareNone
	case a.from == b.from:
		return shareFanOut
	}
	return shareFanIn
}

// mayShare reports whether e, already sharing as kind, may also join f's
// line, and the kind e shares as once it does.
func (g *graph) mayShare(e, f *edge, kind shareKind) (bool, shareKind) {
	k := shareKindOf(e, f)
	if k == shareNone || (kind != shareNone && kind != k) {
		return false, kind
	}
	if fk := g.shareKinds[f]; fk != shareNone && fk != k {
		return false, kind
	}
	return true, k
}

func (g *graph) sidePairs(e *edge) []sidePair {
	preferred, preferredEnd, alternative, alternativeEnd := g.determineStartAndEndDir(e)
	pairs := []sidePair{{preferred, preferredEnd, 0}}
	if alternative != preferred || alternativeEnd != preferredEnd {
		pairs = append(pairs, sidePair{alternative, alternativeEnd, 1})
	}
	for _, start := range sides {
		for _, end := range sides {
			seen := false
			for _, p := range pairs {
				seen = seen || (p.start == start && p.end == end)
			}
			if !seen {
				pairs = append(pairs, sidePair{start, end, sidePairBias})
			}
		}
	}
	return pairs
}

func (g *graph) portPenalty(e *edge, port gridCoord, outgoing bool) int {
	cost := 0
	for _, use := range g.portUses[port] {
		if use.e != e && use.outgoing != outgoing {
			cost += portCost
		}
	}
	return cost
}

// routeEdge picks the side pair whose route costs least, counting the cells
// it walks, its turns, what it crosses or runs along, and the ports it shares.
func (g *graph) routeEdge(e *edge, pairs []sidePair) bool {
	bestCost := -1
	limit := g.routeLimit()
	for _, pair := range pairs {
		from := e.from.gridCoord.Direction(pair.start)
		to := e.to.gridCoord.Direction(pair.end)
		if !reachablePort(from, pair.start) || !reachablePort(to, pair.end) {
			continue
		}
		fixed := pair.bias + g.portPenalty(e, from, true) + g.portPenalty(e, to, false)
		cutoff := -1
		if bestCost >= 0 {
			if cutoff = bestCost - fixed; cutoff <= 0 {
				continue
			}
		}
		path, cost, err := g.cheapestPath(e, from, to, limit, cutoff)
		if err != nil {
			continue
		}
		cost += fixed
		if e.text != "" && !g.hasLabelRoom(e, path) {
			cost += labelRoomCost
		}
		if bestCost < 0 || cost < bestCost {
			bestCost = cost
			e.startDir, e.endDir = pair.start, pair.end
			e.path = mergePath(path)
		}
	}
	return bestCost >= 0
}

// reachablePort reports whether the cell outside a port is on the grid.
// The grid has no negative cells, so a port on the top or left edge of
// the layout cannot be entered from outside.
func reachablePort(port gridCoord, side direction) bool {
	switch side {
	case Up:
		return port.y > 0
	case Left:
		return port.x > 0
	}
	return true
}

// routeLimit bounds the search to the placed nodes plus room for one detour
// lane on each side, since the grid itself grows without end.
func (g *graph) routeLimit() gridCoord {
	limit := gridCoord{}
	for _, n := range g.nodes {
		if n.gridCoord == nil {
			continue
		}
		limit.x = Max(limit.x, n.gridCoord.x+2)
		limit.y = Max(limit.y, n.gridCoord.y+2)
	}
	return gridCoord{x: limit.x + 3, y: limit.y + 3}
}

func (g *graph) recordEdgeCells(e *edge) {
	cells := expandPath(e.path)
	if len(cells) < 2 {
		return
	}
	g.portUses[cells[0]] = append(g.portUses[cells[0]], portUse{e, true})
	last := cells[len(cells)-1]
	g.portUses[last] = append(g.portUses[last], portUse{e, false})
	for i := 1; i < len(cells)-1; i++ {
		for _, use := range g.cellUses[cells[i]] {
			if ok, k := g.mayShare(e, use.e, g.shareKinds[e]); ok {
				g.shareKinds[e], g.shareKinds[use.e] = k, k
			}
		}
		inHorizontal := cells[i-1].y == cells[i].y
		outHorizontal := cells[i].y == cells[i+1].y
		g.cellUses[cells[i]] = append(g.cellUses[cells[i]], cellUse{
			e:              e,
			horizontal:     inHorizontal || outHorizontal,
			vertical:       !inHorizontal || !outHorizontal,
			head:           i == len(cells)-2,
			headHorizontal: cells[i].y == cells[i+1].y,
		})
	}
}

func expandPath(path []gridCoord) []gridCoord {
	if len(path) == 0 {
		return nil
	}
	cells := []gridCoord{path[0]}
	for _, next := range path[1:] {
		current := cells[len(cells)-1]
		for !current.Equals(next) {
			current = gridCoord{x: current.x + sign(next.x-current.x), y: current.y + sign(next.y-current.y)}
			cells = append(cells, current)
		}
	}
	return cells
}

func sign(v int) int {
	switch {
	case v > 0:
		return 1
	case v < 0:
		return -1
	}
	return 0
}

// foreignUse reports how a cell is already used by edges e may not share it
// with, given the kind e shares as so far, and the kind it shares as after
// the cell. A cell holding another edge's label or arrowhead is blocked
// outright, since any line through it hides what it carries.
func (g *graph) foreignUse(e *edge, c gridCoord, kind shareKind) (horizontal, vertical, blocked bool, after shareKind) {
	after = kind
	if owner := g.labelCells[c]; owner != nil && owner != e {
		blocked = true
	}
	for _, use := range g.cellUses[c] {
		if use.e == e {
			continue
		}
		if use.head && use.e.to != e.to && !g.roomyHead(c, use.headHorizontal) {
			blocked = true
		}
		if ok, k := g.mayShare(e, use.e, after); ok {
			after = k
			continue
		}
		horizontal = horizontal || use.horizontal
		vertical = vertical || use.vertical
	}
	return horizontal, vertical, blocked, after
}

// holdsOtherTarget reports whether an edge bound for another node runs
// through c, where e's arrowhead would be drawn over it.
func (g *graph) holdsOtherTarget(e *edge, c gridCoord, horizontal bool) bool {
	if g.labelCells[c] != nil && g.labelCells[c] != e {
		return true
	}
	if g.roomyHead(c, horizontal) {
		return false
	}
	for _, use := range g.cellUses[c] {
		if use.e != e && use.e.to != e.to {
			return true
		}
	}
	return false
}

// roomyHead reports whether an arrowhead's cell is at least three characters
// along its axis. The arrowhead is drawn at the cell's far end and a crossing
// or branch at its center, so in a roomy cell they stay apart.
func (g *graph) roomyHead(c gridCoord, horizontal bool) bool {
	if horizontal {
		return g.columnWidth[c.x] >= 3
	}
	return g.rowHeight[c.y] >= 3
}

// hasLabelRoom reports whether a route has a cell determineLabelLine could
// give e's label.
func (g *graph) hasLabelRoom(e *edge, path []gridCoord) bool {
	cells := expandPath(path)
	for i := 1; i < len(cells)-1; i++ {
		prev, c, next := cells[i-1], cells[i], cells[i+1]
		vertical := prev.x == c.x && c.x == next.x
		horizontal := prev.y == c.y && c.y == next.y
		if (vertical || horizontal) && g.labelCellFree(e, c, vertical) {
			return true
		}
	}
	return false
}

func (g *graph) labelCellFree(e *edge, c gridCoord, vertical bool) bool {
	return g.labelCells[c] == nil && !g.cellShared(e, c) && !g.isNodeBorderColumn(c.x) && (!vertical || !g.isNodeBorderRow(c.y))
}

func (g *graph) recordFanoutLabelCells(e *edge) {
	if e.text == "" || len(e.labelLine) < 2 {
		return
	}
	cells := expandPath(e.labelLine)
	for _, c := range cells[:len(cells)-1] {
		g.labelCells[c] = e
	}
}

type routeState struct {
	c    gridCoord
	dir  int
	kind shareKind
}

type routeItem struct {
	state    routeState
	priority int
}

type routeQueue []routeItem

func (q routeQueue) Len() int            { return len(q) }
func (q routeQueue) Less(i, j int) bool  { return q[i].priority < q[j].priority }
func (q routeQueue) Swap(i, j int)       { q[i], q[j] = q[j], q[i] }
func (q *routeQueue) Push(x interface{}) { *q = append(*q, x.(routeItem)) }
func (q *routeQueue) Pop() interface{} {
	old := *q
	item := old[len(old)-1]
	*q = old[:len(old)-1]
	return item
}

func routeHeuristic(a, b gridCoord) int {
	dx, dy := Abs(a.x-b.x), Abs(a.y-b.y)
	if dx != 0 && dy != 0 {
		return dx + dy + turnCost
	}
	return dx + dy
}

// cheapestPath is A* over grid cells, with the heading in the state so turns
// are priced, and with cells other edges hold priced by foreignUse. It gives
// up once no route can cost less than cutoff, unless cutoff is negative.
func (g *graph) cheapestPath(e *edge, from, to, limit gridCoord, cutoff int) ([]gridCoord, int, error) {
	start := routeState{c: from, dir: -1}
	cost := map[routeState]int{start: 0}
	cameFrom := map[routeState]routeState{}
	q := &routeQueue{{start, 0}}
	for q.Len() > 0 {
		item := heap.Pop(q).(routeItem)
		if cutoff >= 0 && item.priority >= cutoff {
			break
		}
		current := item.state
		if current.c.Equals(to) {
			path := []gridCoord{current.c}
			for s := current; s != start; {
				s = cameFrom[s]
				path = append([]gridCoord{s.c}, path...)
			}
			return path, cost[current], nil
		}
		for dir, step := range routeSteps {
			next := gridCoord{x: current.c.x + step.x, y: current.c.y + step.y}
			if !next.Equals(to) && (!g.isFreeInGrid(next) || next.x > limit.x || next.y > limit.y) {
				continue
			}
			horizontal := step.y == 0
			stepCost := 1
			if current.dir >= 0 && current.dir != dir {
				stepCost += turnCost
				if h, v, blocked, _ := g.foreignUse(e, current.c, current.kind); h || v || blocked {
					stepCost += overlapCost
				}
			}
			if next.Equals(to) && g.holdsOtherTarget(e, current.c, horizontal) {
				stepCost += overlapCost
			}
			kind := current.kind
			if !next.Equals(to) {
				var h, v, blocked bool
				h, v, blocked, kind = g.foreignUse(e, next, current.kind)
				switch {
				case blocked || (horizontal && h) || (!horizontal && v):
					stepCost += overlapCost
				case h || v:
					stepCost += crossingCost
				}
			}
			nextState := routeState{c: next, dir: dir, kind: kind}
			newCost := cost[current] + stepCost
			if known, ok := cost[nextState]; !ok || newCost < known {
				cost[nextState] = newCost
				cameFrom[nextState] = current
				heap.Push(q, routeItem{nextState, newCost + routeHeuristic(next, to)})
			}
		}
	}
	return nil, 0, fmt.Errorf("no path found")
}
