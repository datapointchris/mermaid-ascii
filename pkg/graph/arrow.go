package graph

import (
	"slices"

	"github.com/mattn/go-runewidth"

	log "github.com/sirupsen/logrus"
)

func (g *graph) isFreeInGrid(c gridCoord) bool {
	// We'll fix it later if we overshoot the grid size
	if c.x < 0 || c.y < 0 {
		return false
	}
	return g.grid[c] == nil
}

func (g *graph) drawArrow(from gridCoord, to gridCoord, e *edge) (*drawing, *drawing, *drawing, *drawing, *drawing) {
	if len(e.path) == 0 {
		return nil, nil, nil, nil, nil
	}
	log.Debugf("Drawing arrow from %v to %v with path %v", from, to, e.path)
	dLabel := g.drawArrowLabel(e)
	dPath, linesDrawn, lineDirs := g.drawPath(e)
	dBoxStart := g.drawBoxStart(e.path, linesDrawn[0], e.stroke)
	skipHead := e.head == headNone
	if skipHead && len(linesDrawn) > 0 {
		// headNone (open links like ---) has no arrowhead glyph to mark the
		// connection at the destination, unlike every other head; touch up
		// the destination box's own border the same way drawBoxStart does
		// for the source, so the join is visible there too.
		dBoxEnd := g.drawBoxEnd(linesDrawn[len(linesDrawn)-1], lineDirs[len(lineDirs)-1], e.stroke)
		dBoxStart = g.mergeDrawings(dBoxStart, drawingCoord{0, 0}, dBoxEnd)
	}
	var dArrowHead *drawing
	if skipHead {
		dArrowHead = copyCanvas(g.drawing)
	} else {
		dArrowHead = g.drawArrowHead(linesDrawn[len(linesDrawn)-1], lineDirs[len(lineDirs)-1], e.head)
	}
	if e.isBidirectional && !skipHead && len(linesDrawn) > 0 {
		dStartArrowHead := g.drawArrowHead(reverseDrawingLine(linesDrawn[0]), lineDirs[0].getOpposite(), e.head)
		dArrowHead = g.mergeDrawings(dArrowHead, drawingCoord{0, 0}, dStartArrowHead)
	}
	dCorners := g.drawCorners(e)
	return dPath, dBoxStart, dArrowHead, dCorners, dLabel
}

func reverseDrawingLine(line []drawingCoord) []drawingCoord {
	if len(line) == 0 {
		return line
	}
	reversed := make([]drawingCoord, len(line))
	for i, coord := range line {
		reversed[len(line)-1-i] = coord
	}
	return reversed
}

func mergePath(path []gridCoord) []gridCoord {
	// If two steps are in the same direction, merge them to one step.
	if len(path) <= 2 {
		return path
	}
	indexToRemove := []int{}
	step0 := path[0]
	step1 := path[1]
	for idx, step2 := range path[2:] {
		prevDir := determineDirection(genericCoord(step0), genericCoord(step1))
		dir := determineDirection(genericCoord(step1), genericCoord(step2))
		if prevDir == dir {
			indexToRemove = append(indexToRemove, idx+1) // +1 because we skip the initial step
		}
		step0 = step1
		step1 = step2
	}
	newPath := []gridCoord{}
	for idx, step := range path {
		if !slices.Contains(indexToRemove, idx) {
			newPath = append(newPath, step)
		}
	}
	log.Debugf("Reduced path from %v to %v", path, newPath)
	return newPath
}

func (g *graph) drawPath(e *edge) (*drawing, [][]drawingCoord, []direction) {
	path, stroke := e.path, e.stroke
	d := copyCanvas(g.drawing)
	previousCoord := path[0]
	linesDrawn := make([][]drawingCoord, 0)
	lineDirs := make([]direction, 0)
	var previousDrawingCoord drawingCoord
	for _, nextCoord := range path[1:] {
		previousDrawingCoord = g.edgeDrawingCoord(e, previousCoord)
		nextDrawingCoord := g.edgeDrawingCoord(e, nextCoord)
		if previousDrawingCoord.Equals(nextDrawingCoord) {
			log.Debugf("Skipping drawing identical line on %v", nextCoord)
			continue
		}
		dir := determineDirection(genericCoord(previousCoord), genericCoord(nextCoord))
		s := g.drawLine(d, previousDrawingCoord, nextDrawingCoord, 1, -1, stroke)
		if len(s) == 0 {
			// drawLine returns no coords when a one-cell corridor leaves no room between the offsets. The first segment
			// then records the cell past the source border, so drawBoxStart puts its junction on the border itself.
			if len(linesDrawn) == 0 {
				s = append(s, nextDrawingCoord)
			} else {
				s = append(s, previousDrawingCoord)
			}
		}
		linesDrawn = append(linesDrawn, s)
		lineDirs = append(lineDirs, dir)
		previousCoord = nextCoord
	}
	return d, linesDrawn, lineDirs
}

func (g *graph) drawBoxStart(path []gridCoord, firstLine []drawingCoord, stroke edgeStroke) *drawing {
	d := *(copyCanvas(g.drawing))
	from := firstLine[0]
	dir := determineDirection(genericCoord(path[0]), genericCoord(path[1]))
	log.Debugf("Drawing box start at %v with direction %v for line %v", from, dir, path)

	if g.useAscii {
		return &d
	}

	// A heavy line meeting a light box border looks mismatched through a
	// light T-junction glyph (e.g. "├"); leave the box's own light border
	// character in place instead.
	if stroke == strokeThick {
		return &d
	}

	switch dir {
	case Up:
		d[from.x][from.y+1] = "┴"
	case Down:
		d[from.x][from.y-1] = "┬"
	case Left:
		d[from.x+1][from.y] = "┤"
	case Right:
		d[from.x-1][from.y] = "├"
	}
	return &d
}

// drawBoxEnd is drawBoxStart's mirror for the destination end of a path.
func (g *graph) drawBoxEnd(lastLine []drawingCoord, dir direction, stroke edgeStroke) *drawing {
	d := *(copyCanvas(g.drawing))
	if len(lastLine) == 0 {
		return &d
	}
	to := lastLine[len(lastLine)-1]

	if g.useAscii || stroke == strokeThick {
		return &d
	}

	switch dir {
	case Up:
		d[to.x][to.y-1] = "┬"
	case Down:
		d[to.x][to.y+1] = "┴"
	case Left:
		d[to.x-1][to.y] = "├"
	case Right:
		d[to.x+1][to.y] = "┤"
	}
	return &d
}

func (g *graph) drawArrowHead(line []drawingCoord, fallback direction, head edgeHead) *drawing {
	d := *(copyCanvas(g.drawing))
	if len(line) == 0 {
		return &d
	}
	from := line[0]
	lastPos := line[len(line)-1]
	dir := determineDirection(genericCoord(from), genericCoord(lastPos))
	if len(line) == 1 || dir == Middle {
		dir = fallback
	}

	// Circle and cross heads are a single glyph regardless of travel direction.
	if head == headCircle || head == headCross {
		char := "o"
		if head == headCross {
			char = "x"
		}
		if !g.useAscii {
			char = "●"
			if head == headCross {
				char = "✕"
			}
		}
		d[lastPos.x][lastPos.y] = char
		return &d
	}

	var char string
	if !g.useAscii {
		switch dir {
		case Up:
			char = "▲"
		case Down:
			char = "▼"
		case Left:
			char = "◄"
		case Right:
			char = "►"
		case UpperRight:
			char = "◥"
		case UpperLeft:
			char = "◤"
		case LowerRight:
			char = "◢"
		case LowerLeft:
			char = "◣"
		default:
			switch fallback {
			case Up:
				char = "▲"
			case Down:
				char = "▼"
			case Left:
				char = "◄"
			case Right:
				char = "►"
			case UpperRight:
				char = "◥"
			case UpperLeft:
				char = "◤"
			case LowerRight:
				char = "◢"
			case LowerLeft:
				char = "◣"
			default:
				char = "●"
			}
		}
	} else {
		switch dir {
		case Up:
			char = "^"
		case Down:
			char = "v"
		case Left:
			char = "<"
		case Right:
			char = ">"
		default:
			switch fallback {
			case Up:
				char = "^"
			case Down:
				char = "v"
			case Left:
				char = "<"
			case Right:
				char = ">"
			default:
				char = "*"
			}
		}
	}

	d[lastPos.x][lastPos.y] = char
	return &d
}

func (g *graph) drawCorners(e *edge) *drawing {
	path, stroke := e.path, e.stroke
	d := copyCanvas(g.drawing)
	// Thick edges get the heavy box-drawing corner glyphs to match their
	// heavy line/arrowhead; ASCII has no distinct "thick corner" (still
	// "+"), and dotted corners are left as plain light corners.
	thick := !g.useAscii && stroke == strokeThick
	for idx, coord := range path {
		// Skip the first and last step
		if idx == 0 || idx == len(path)-1 {
			continue
		}
		drawingCoord := g.edgeDrawingCoord(e, coord)

		prevDir := determineDirection(genericCoord(path[idx-1]), genericCoord(coord))
		nextDir := determineDirection(genericCoord(coord), genericCoord(path[idx+1]))

		var corner string
		if !g.useAscii {
			switch {
			case (prevDir == Right && nextDir == Down) || (prevDir == Up && nextDir == Left):
				corner = "┐"
				if thick {
					corner = "┓"
				}
			case (prevDir == Right && nextDir == Up) || (prevDir == Down && nextDir == Left):
				corner = "┘"
				if thick {
					corner = "┛"
				}
			case (prevDir == Left && nextDir == Down) || (prevDir == Up && nextDir == Right):
				corner = "┌"
				if thick {
					corner = "┏"
				}
			case (prevDir == Left && nextDir == Up) || (prevDir == Down && nextDir == Right):
				corner = "└"
				if thick {
					corner = "┗"
				}
			default:
				corner = "+"
			}
		} else {
			corner = "+"
		}

		(*d)[drawingCoord.x][drawingCoord.y] = corner
	}
	return d
}

func (g *graph) drawArrowLabel(e *edge) *drawing {
	d := copyCanvas(g.drawing)
	if e.text == "" {
		return d
	}
	var start drawingCoord
	switch {
	case e.fanout:
		x, _, y := g.fanoutLabelBounds(e)
		start = drawingCoord{x: x, y: y}
	case e.labelCell != nil:
		start = g.labelStart(e)
	default:
		return d
	}
	log.Debugf("Drawing text '%s' at %v", e.text, start)
	d.drawText(start, e.text)
	markLabelBlanks(d, start, e.text)
	return d
}

// labelStart centers the label in its cell. On a horizontal run it keeps
// the text clear of an arrowhead drawn at either end of the cell.
func (g *graph) labelStart(e *edge) drawingCoord {
	c := *e.labelCell
	center := g.gridToDrawingCoord(c, nil)
	width := runewidth.StringWidth(e.text)
	start := drawingCoord{x: center.x - width/2, y: center.y}
	if e.labelVertical {
		return start
	}
	colStart := center.x - g.columnWidth[c.x]/2
	colEnd := colStart + g.columnWidth[c.x] - 1
	left, right := 1, 1
	cells := expandPath(e.path)
	for i := 1; i < len(cells)-1; i++ {
		if !cells[i].Equals(c) {
			continue
		}
		rightward := cells[i+1].x > c.x
		if i == len(cells)-2 && e.head != headNone {
			if rightward {
				right = 2
			} else {
				left = 2
			}
		}
		if i == 1 && e.isBidirectional {
			if rightward {
				left = 2
			} else {
				right = 2
			}
		}
	}
	free := (colEnd - right) - (colStart + left) + 1
	start.x = colStart + left + Max(0, (free-width)/2)
	return start
}

// labelBlank stands in for a space inside a label until the label layer is
// merged, so the space covers the line beneath it instead of showing it.
const labelBlank = "\x00"

func markLabelBlanks(d *drawing, start drawingCoord, text string) {
	x := start.x
	for _, cell := range textCells(text) {
		if cell.text == " " && x >= 0 && x < len(*d) && start.y >= 0 && start.y < len((*d)[x]) {
			(*d)[x][start.y] = labelBlank
		}
		x += cell.width
	}
}
