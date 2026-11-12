package main

// ---- the room in shape letters (Sy Geometry, design_geo.go) ------------------------------
//
// With the shape letters' font set up, the whole room is drawn in
// letters: each cell is looked at as 8 × 16 points of the picture, split
// into its two colours, and drawn as the shape letter whose straight edge
// splits it most like that (a cell of about one colour is a space in it;
// one no straight edge splits well, quarter blocks). A sloping edge of a
// block is then a straight edge from cell to cell, not a staircase, and the
// text layer's sloping lines are thin lines at their own angle, not ╱ ╲.
// P goes on to the text over the picture, then the plain picture.

import (
	"os"
	"strconv"
	"strings"
	"sync"
)

// shapesWanted is whether the terminal has the shape letters' font:
// SY_3D_FONT=1 or =0 says so; otherwise sy --font has put it in place.
var shapesWanted = sync.OnceValue(func() bool {
	switch os.Getenv("SY_3D_FONT") {
	case "1":
		return true
	case "0":
		return false
	}
	return geoFontReady()
})

// geoFontReady is whether the font is set up for this terminal (see
// design_geo_font.go); over SSH it is the other computer's terminal, so
// only SY_3D_FONT=1 says it is.
var geoFontReady = func() bool { return false }

// shapes is whether the room is drawn in shape letters now.
func (r *room3D) shapes() bool { return shapesWanted() && !r.noShapes && !r.plain }

// writeShapes turns the pixels into h lines of w cells of shape letters
// (the words and the text layer's letters over them).
func (r *room3D) writeShapes(w, h int, sb *strings.Builder) {
	buf := make([]byte, 0, w*40)
	colour := func(buf []byte, lead string, c uint32) []byte {
		buf = append(buf, lead...)
		buf = strconv.AppendUint(buf, uint64(c>>16&255), 10)
		buf = append(buf, ';')
		buf = strconv.AppendUint(buf, uint64(c>>8&255), 10)
		buf = append(buf, ';')
		buf = strconv.AppendUint(buf, uint64(c&255), 10)
		return append(buf, 'm')
	}
	for row := 0; row < h; row++ {
		buf = buf[:0]
		fg, bg := int64(-1), int64(-1)
		for col := 0; col < w; col++ {
			var top, bot uint32
			var glyph string
			if ch := &r.chars[row*w+col]; ch.stamp == r.stamp {
				if ch.cont {
					continue
				}
				top, bot, glyph = ch.fg, ch.bg, ch.s
			} else if k := r.inkAt(row*w + col); k != nil {
				top, bot, glyph = k.fg, k.bg, k.s
			} else {
				glyph, top, bot = r.fitCell(col, row)
			}
			if glyph != " " && int64(top) != fg {
				buf = colour(buf, "\x1b[38;2;", top)
				fg = int64(top)
			}
			if int64(bot) != bg {
				buf = colour(buf, "\x1b[48;2;", bot)
				bg = int64(bot)
			}
			buf = append(buf, glyph...)
		}
		buf = append(buf, "\x1b[m\n"...)
		sb.Write(buf)
	}
}

// fitCell is the shape letter and its two colours for screen cell (col,
// row) of the picture.
func (r *room3D) fitCell(col, row int) (glyph string, fg, bg uint32) {
	x0, y0, x1, y1 := r.cellBox(col, row)
	var px [geoSX * geoSY][3]float64
	lo, hi, li, hiI := 1e9, -1e9, 0, 0
	var tr, tg, tb float64
	for b := range geoSY {
		y := y0 + (2*b+1)*(y1-y0)/(2*geoSY)
		for a := range geoSX {
			x := x0 + (2*a+1)*(x1-x0)/(2*geoSX)
			cr, cg, cb := chans(r.pixel(y*r.pw + x))
			i := b*geoSX + a
			px[i] = [3]float64{cr, cg, cb}
			tr, tg, tb = tr+cr, tg+cg, tb+cb
			if l := 0.299*cr + 0.587*cg + 0.114*cb; l < lo {
				lo, li = l, i
			} else if l > hi {
				hi, hiI = l, i
			}
		}
	}
	const n = geoSX * geoSY
	mean := rgb32(tr/n+0.5, tg/n+0.5, tb/n+0.5)
	if hi-lo <= smoothSpread {
		return " ", mean, mean
	}
	// Two colours: the darkest and lightest points, each point going to
	// the nearer, a few times over.
	a, b := px[li], px[hiI]
	var m geoMask
	for range 3 {
		m = geoMask{}
		var sa, sb [3]float64
		na, nb := 0.0, 0.0
		for i := range px {
			p := px[i]
			da := (p[0]-a[0])*(p[0]-a[0]) + (p[1]-a[1])*(p[1]-a[1]) + (p[2]-a[2])*(p[2]-a[2])
			db := (p[0]-b[0])*(p[0]-b[0]) + (p[1]-b[1])*(p[1]-b[1]) + (p[2]-b[2])*(p[2]-b[2])
			if db < da {
				m.set(i)
				sb[0], sb[1], sb[2], nb = sb[0]+p[0], sb[1]+p[1], sb[2]+p[2], nb+1
			} else {
				sa[0], sa[1], sa[2], na = sa[0]+p[0], sa[1]+p[1], sa[2]+p[2], na+1
			}
		}
		if na == 0 || nb == 0 {
			return " ", mean, mean
		}
		a = [3]float64{sa[0] / na, sa[1] / na, sa[2] / na}
		b = [3]float64{sb[0] / nb, sb[1] / nb, sb[2] / nb}
	}
	if i := geoFit(m); i >= 0 {
		gm := geoWedgeMasks()[i]
		var s [2][3]float64
		var c [2]float64
		for j := range px {
			k := 0
			if gm.has(j) {
				k = 1
			}
			s[k][0], s[k][1], s[k][2], c[k] = s[k][0]+px[j][0], s[k][1]+px[j][1], s[k][2]+px[j][2], c[k]+1
		}
		if c[0] > 0 && c[1] > 0 {
			return string(rune(geoBase + i)), rgb32(s[1][0]/c[1]+0.5, s[1][1]/c[1]+0.5, s[1][2]/c[1]+0.5),
				rgb32(s[0][0]/c[0]+0.5, s[0][1]/c[0]+0.5, s[0][2]/c[0]+0.5)
		}
	}
	// No straight edge splits it: quarter blocks of its four quarters.
	var q [4]uint32
	for k := range 4 {
		var sr, sg, sb float64
		for b := (k / 2) * geoSY / 2; b < (k/2+1)*geoSY/2; b++ {
			for a := (k % 2) * geoSX / 2; a < (k%2+1)*geoSX/2; a++ {
				p := px[b*geoSX+a]
				sr, sg, sb = sr+p[0], sg+p[1], sb+p[2]
			}
		}
		const qn = n / 4
		q[k] = rgb32(sr/qn+0.5, sg/qn+0.5, sb/qn+0.5)
	}
	mask, fa, fb := quads(&q)
	if mask == 0 {
		return " ", fa, fa
	}
	return quadGlyph[mask], fa, fb
}
