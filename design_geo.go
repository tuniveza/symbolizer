package main

// ---- the shape letters: a font for the room in 3D ------------------------------------------
//
// A terminal cell holds one letter in two colours. Box drawing and block
// letters only split a cell straight across, down or in quarters, so a
// sloping edge of a block comes out as a staircase, and a sloping line as
// ╱ ╲ ╳ that never quite meet. Sy Geometry is a font of shape letters
// made for the room: each cell can be split by a straight edge at any of
// 64 angles, at any of 7 depths into the cell, or crossed by a thin line
// at any of 32 angles, at any of 9 places across it. With it, every edge
// of the room is drawn as the straight line it is, smoothly from cell to
// cell, in letters.
//
// The letters are in the Supplementary Private Use Area (U+FB000 on, where
// Nerd Fonts put nothing). sy --font writes the font (fitted to the
// terminal's own font, so a letter fills its cell exactly) and tells kitty
// to use it for them; see design_geo_font.go.
//
// Shapes are worked out in a cell's own units: u from 0 (left) to 1
// (right), v from 0 (top) to 2 (bottom), a cell being twice as tall as it
// is wide, so an angle here is an angle on the screen.

import (
	"math"
	"math/bits"
	"sync"
)

const (
	geoBase    = 0xFB000 // the first shape letter
	geoAngles  = 64      // the angles an edge can face, all the way round
	geoDepths  = 7       // how much of a cell an edge's side covers: 1/8 … 7/8
	geoLineAng = 32      // the angles a line can run at, half way round
	geoLinePos = 9       // the places across a cell a line can be, its middle the 5th
	geoLines   = geoAngles * geoDepths
	geoCount   = geoLines + geoLineAng*geoLinePos
	geoStroke  = 0.12 // how thick a line is: an eighth of a cell's width, as box drawing letters are
)

// geoPoint is a point in a cell: u across (0 to 1), v down (0 to 2).
type geoPoint struct{ u, v float64 }

// geoBox is a whole cell.
var geoBox = []geoPoint{{0, 0}, {1, 0}, {1, 2}, {0, 2}}

// clipHalf is the part of polygon p where n·q ≥ t.
func clipHalf(p []geoPoint, nu, nv, t float64) []geoPoint {
	var out []geoPoint
	for i := range p {
		a, b := p[i], p[(i+1)%len(p)]
		da, db := nu*a.u+nv*a.v-t, nu*b.u+nv*b.v-t
		if da >= 0 {
			out = append(out, a)
		}
		if (da >= 0) != (db >= 0) {
			s := da / (da - db)
			out = append(out, geoPoint{a.u + (b.u-a.u)*s, a.v + (b.v-a.v)*s})
		}
	}
	return out
}

// polyArea is the area of polygon p (positive however it goes round).
func polyArea(p []geoPoint) float64 {
	s := 0.0
	for i := range p {
		a, b := p[i], p[(i+1)%len(p)]
		s += a.u*b.v - b.u*a.v
	}
	return math.Abs(s) / 2
}

// wedgeNormal is the way edge angle k faces (out of the covered side's
// way: the covered side is where n·q is large).
func wedgeNormal(k int) (nu, nv float64) {
	a := 2 * math.Pi * float64(k) / geoAngles
	return math.Cos(a), math.Sin(a)
}

// wedgeShape is the shape of the wedge letter with edge angle k and depth
// j (1 to 7): the side of a straight edge facing n, covering j/8 of the
// cell.
func wedgeShape(k, j int) []geoPoint {
	nu, nv := wedgeNormal(k)
	want := 2 * float64(j) / 8 // a cell's area is 2
	lo, hi := -3.0, 3.0        // n·q over a cell is within this
	for range 60 {
		t := (lo + hi) / 2
		if polyArea(clipHalf(geoBox, nu, nv, t)) > want {
			lo = t
		} else {
			hi = t
		}
	}
	return clipHalf(geoBox, nu, nv, (lo+hi)/2)
}

// lineFrame is how line letter k runs: its direction, the way across it,
// and how far a cell reaches across it from the middle (so place p is at
// lineOffset(p, e) across).
func lineFrame(k int) (du, dv, nu, nv, e float64) {
	a := math.Pi * float64(k) / geoLineAng
	du, dv = math.Cos(a), math.Sin(a)
	nu, nv = -dv, du
	e = (math.Abs(nu) + 2*math.Abs(nv)) / 2
	return
}

// lineOffset is how far place p (0 to 8) of a line is from the cell's
// middle, across it, in a cell that reaches e either way.
func lineOffset(p int, e float64) float64 {
	return float64(p-geoLinePos/2) * e / 5
}

// lineShape is the shape of the line letter at angle k and place p.
func lineShape(k, p int) []geoPoint {
	_, _, nu, nv, e := lineFrame(k)
	t := nu*0.5 + nv*1 + lineOffset(p, e) // n·q at the line's middle
	s := clipHalf(geoBox, nu, nv, t-geoStroke/2)
	return clipHalf(s, -nu, -nv, -(t + geoStroke/2))
}

// geoShape is the shape of shape letter i (0 ≤ i < geoCount).
func geoShape(i int) []geoPoint {
	if i < geoLines {
		return wedgeShape(i/geoDepths, i%geoDepths+1)
	}
	i -= geoLines
	return lineShape(i/geoLinePos, i%geoLinePos)
}

// geoWedge is the letter of edge angle k, depth j (1 to 7).
func geoWedge(k, j int) string {
	return string(rune(geoBase + ((k%geoAngles+geoAngles)%geoAngles)*geoDepths + j - 1))
}

// geoLine is the letter of line angle k (0 to 31), place p (0 to 8).
func geoLine(k, p int) string {
	return string(rune(geoBase + geoLines + ((k%geoLineAng+geoLineAng)%geoLineAng)*geoLinePos + p))
}

// ---- fitting a cell -----------------------------------------------------------------------

// A cell is looked at as geoSX × geoSY points, one bit each of a geoMask.
const geoSX, geoSY = 8, 16

type geoMask [2]uint64 // bit y*geoSX+x: the point in column x, row y

func (m *geoMask) set(i int)     { m[i>>6] |= 1 << (i & 63) }
func (m geoMask) has(i int) bool { return m[i>>6]&(1<<(i&63)) != 0 }
func (m geoMask) not() geoMask   { return geoMask{^m[0], ^m[1]} }
func (m geoMask) count() int     { return bits.OnesCount64(m[0]) + bits.OnesCount64(m[1]) }
func (m geoMask) apart(o geoMask) int {
	return bits.OnesCount64(m[0]^o[0]) + bits.OnesCount64(m[1]^o[1])
}

// inPoly says whether point q is inside convex polygon p.
func inPoly(p []geoPoint, q geoPoint) bool {
	if len(p) < 3 {
		return false
	}
	sign := 0.0
	for i := range p {
		a, b := p[i], p[(i+1)%len(p)]
		c := (b.u-a.u)*(q.v-a.v) - (b.v-a.v)*(q.u-a.u)
		if c != 0 {
			if sign == 0 {
				sign = c
			} else if (c > 0) != (sign > 0) {
				return false
			}
		}
	}
	return true
}

// geoWedgeMasks are the wedge letters' points, worked out once.
var geoWedgeMasks = sync.OnceValue(func() (ms [geoLines]geoMask) {
	for i := range ms {
		s := geoShape(i)
		for y := range geoSY {
			for x := range geoSX {
				if inPoly(s, geoPoint{(float64(x) + 0.5) / geoSX, 2 * (float64(y) + 0.5) / geoSY}) {
					ms[i].set(y*geoSX + x)
				}
			}
		}
	}
	return
})

// geoFit is the wedge letter (its number, from 0) that splits a cell
// most like mask m (the points of one of its two colours), or −1 when
// none is alike enough (more than geoFitMin points wrong) or the cell is
// one colour.
func geoFit(m geoMask) int {
	n := m.count()
	if n == 0 || n == geoSX*geoSY {
		return -1
	}
	ms := geoWedgeMasks()
	best, bi := geoSX*geoSY+1, -1
	inv := m.not()
	for i := range ms {
		if d := min(ms[i].apart(m), ms[i].apart(inv)); d < best {
			best, bi = d, i
		}
	}
	if best > geoFitMin {
		return -1
	}
	return bi
}

// geoFitMin is how many of a cell's 128 points a shape letter may get
// wrong and still stand for it (more: the cell is not split by one edge).
const geoFitMin = 22

// geoLineFor is the line letter for a line through screen point (x, y)
// (cells across, rows down) running (dx, dy), drawn in the cell (col,
// row): the angle nearest its own and the place nearest where it crosses.
// It is "" when the line misses the cell.
func geoLineFor(x, y, dx, dy float64, col, row int) string {
	du, dv := dx, 2*dy // in a cell's own units a row is 2
	if du == 0 && dv == 0 {
		return ""
	}
	a := math.Atan2(dv, du)
	if a < 0 {
		a += math.Pi
	}
	if a >= math.Pi {
		a -= math.Pi
	}
	k := int(math.Round(a/(math.Pi/geoLineAng))) % geoLineAng
	_, _, nu, nv, e := lineFrame(k)
	pu, pv := x-float64(col), 2*(y-float64(row))
	off := nu*(pu-0.5) + nv*(pv-1) // how far across the line is from the cell's middle
	p := int(math.Round(off/(e/5))) + geoLinePos/2
	if p < 0 || p >= geoLinePos {
		return ""
	}
	return geoLine(k, p)
}

// isGeo says whether letter s is a shape letter.
func isGeo(s string) bool {
	for _, c := range s {
		return c >= geoBase && c < geoBase+geoCount
	}
	return false
}
