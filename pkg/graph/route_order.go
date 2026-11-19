package graph

import (
	"fmt"
	"slices"
)

// layoutAttempts bounds how many routing orders bestLayout lays out. Edges
// are routed one at a time, so an edge routed early can take the only clear
// route of one routed later, and another order can keep both apart.
const layoutAttempts = 8

func (a layoutAttempt) betterThan(b layoutAttempt) bool {
	if len(a.findings) != len(b.findings) {
		return len(a.findings) < len(b.findings)
	}
	return len(*a.d) < len(*b.d)
}

// routingOrder returns the indexes of g.edges in the order they are routed:
// the order they were written in, unless a layout attempt set another.
func (g *graph) routingOrder() []int {
	if g.routeOrder != nil {
		return g.routeOrder
	}
	order := make([]int, len(g.edges))
	for i := range order {
		order[i] = i
	}
	return order
}

// bestLayout lays the graph out in the written order, then, while findings
// remain, in orders that route the edges they name first, or reversed, and
// keeps the layout with the fewest findings, then the narrowest.
func bestLayout(properties *Properties) layoutAttempt {
	best := layOut(properties, nil)
	var queue [][]int
	seen := map[string]bool{fmt.Sprint(best.g.routingOrder()): true}
	enqueue := func(a layoutAttempt) {
		if len(a.findings) == 0 {
			return
		}
		order := a.g.routingOrder()
		reversed := slices.Clone(order)
		slices.Reverse(reversed)
		for _, next := range [][]int{a.g.namedFirst(order, a.findings), reversed} {
			if key := fmt.Sprint(next); !seen[key] {
				seen[key] = true
				queue = append(queue, next)
			}
		}
	}
	enqueue(best)
	for tries := 1; tries < layoutAttempts && len(queue) > 0 && len(best.findings) > 0; tries++ {
		attempt := layOut(properties, queue[0])
		queue = queue[1:]
		if attempt.betterThan(best) {
			best = attempt
		}
		enqueue(attempt)
	}
	return best
}

// namedFirst moves the edges the findings name to the front, the last routed
// first, since the edge routed later is the one that found no clear route.
func (g *graph) namedFirst(order []int, findings []layoutFinding) []int {
	named := map[*edge]bool{}
	for _, f := range findings {
		for _, e := range f.edges {
			named[e] = true
		}
	}
	var front, rest []int
	for _, i := range order {
		if named[g.edges[i]] {
			front = append(front, i)
		} else {
			rest = append(rest, i)
		}
	}
	slices.Reverse(front)
	return append(front, rest...)
}
