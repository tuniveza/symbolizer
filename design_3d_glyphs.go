package main

// ---- words painted on the faces (the picture, kitty) --------------------------------------
//
// In the plain picture the words are not letters of the terminal laid over
// it: each letter is painted into the picture where its cell of the board
// is on its face, from the terminal's own font (kitty's font_family, else
// Go Mono). So words turn, tilt and shrink with the face they are on, the
// way print on a card does. Each letter is drawn once, big, with smaller
// copies of it (a mipmap), and a pixel of the picture takes the copy about
// as fine as it is, so far or slanting words stay smooth.

import (
	"image"
	"image/draw"
	"math"
	"os"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/charmbracelet/x/ansi"
	"github.com/rivo/uniseg"
	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/gomono"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
)

// clean3D is set while the structures are drawn for the 3D picture: their
// drawings leave out what is only for working on the flat board (how many
// things a drawer, cabinet or box holds, a box's layer tabs).
var clean3D bool

// glyphW, glyphH: a cell's letter is drawn this many pixels wide and tall
// at its finest (a terminal cell is about twice as tall as wide).
const glyphW, glyphH = 48, 96

// r3glyph is one letter, drawn: its coverage (0 to 255) at each size, the
// first glyphW × glyphH (twice as wide for a wide letter), each after it
// half as big each way; and its mean coverage.
type r3glyph struct {
	lv   [][]uint8
	w, h []int
	mean float64
}

// glyphKey is a letter and whether it is bold.
type glyphKey struct {
	s    string
	bold bool
}

var glyphCache = struct {
	sync.Mutex
	m map[glyphKey]*r3glyph
}{m: map[glyphKey]*r3glyph{}}

// glyphFaces is the terminal's font as two faces, for one cell and for a
// letter two cells wide, and where its baseline is (pixels from the top).
var glyphFaces = sync.OnceValues(func() (font.Face, int) {
	b := gomono.TTF
	if p := geoTermMetrics().fromFile; p != "" {
		if fb, err := os.ReadFile(p); err == nil {
			b = fb
		}
	}
	f, err := opentype.Parse(b)
	if err != nil {
		f, _ = opentype.Parse(gomono.TTF)
	}
	// The size whose line (ascent and descent) is the cell's height, a
	// little less, and whose letters fit its width.
	try := func(size float64) (font.Face, font.Metrics, fixed.Int26_6) {
		face, _ := opentype.NewFace(f, &opentype.FaceOptions{Size: size, DPI: 72, Hinting: font.HintingNone})
		adv, _ := face.GlyphAdvance('M')
		return face, face.Metrics(), adv
	}
	// As the terminal does: a letter's advance is the cell's width (so the
	// letters are as big and as close as on the flat board), unless its
	// line would then be taller than the cell.
	size := float64(glyphH)
	_, m, adv := try(size)
	if a := float64(adv) / 64; a > 0 {
		size *= glyphW / a
	}
	if line := float64(m.Ascent+m.Descent) / 64 * size / glyphH; line > glyphH {
		size *= glyphH / line
	}
	face, m, _ := try(size)
	line := float64(m.Ascent+m.Descent) / 64
	base := int(math.Round((glyphH-line)/2 + float64(m.Ascent)/64))
	return face, base
})

// glyphOf is letter s drawn (bold: thicker), or nil when the font has no
// such letter. wide: it is two cells wide.
func glyphOf(s string, bold, wide bool) *r3glyph {
	k := glyphKey{s, bold}
	glyphCache.Lock()
	defer glyphCache.Unlock()
	if g, ok := glyphCache.m[k]; ok {
		return g
	}
	g := drawGlyph(s, bold, wide)
	glyphCache.m[k] = g
	return g
}

func drawGlyph(s string, bold, wide bool) *r3glyph {
	face, base := glyphFaces()
	ru, _ := utf8.DecodeRuneInString(s)
	if _, ok := face.GlyphAdvance(ru); !ok {
		return nil
	}
	w := glyphW
	if wide {
		w *= 2
	}
	img := image.NewAlpha(image.Rect(0, 0, w, glyphH))
	adv := font.MeasureString(face, s)
	x := fixed.I(w)/2 - adv/2
	dr := font.Drawer{Dst: img, Src: image.Opaque, Face: face, Dot: fixed.Point26_6{X: x, Y: fixed.I(base)}}
	dr.DrawString(s)
	if bold {
		// Bold: each stroke a little wider (the letter drawn again, moved
		// by a few pixels).
		cp := image.NewAlpha(img.Rect)
		draw.Draw(cp, cp.Rect, img, image.Point{}, draw.Src)
		for dx := 1; dx <= glyphW/16; dx++ {
			draw.Draw(img, img.Rect.Add(image.Pt(dx, 0)), cp, image.Point{}, draw.Over)
		}
	}
	g := &r3glyph{}
	lv, lw, lh := img.Pix, w, glyphH
	sum := 0
	for _, a := range lv {
		sum += int(a)
	}
	g.mean = float64(sum) / float64(255*len(lv))
	for {
		g.lv, g.w, g.h = append(g.lv, lv), append(g.w, lw), append(g.h, lh)
		if lw <= 2 || lh <= 2 {
			break
		}
		nw, nh := lw/2, lh/2
		next := make([]uint8, nw*nh)
		for y := range nh {
			for x := range nw {
				i := 2*y*lw + 2*x
				next[y*nw+x] = uint8((int(lv[i]) + int(lv[i+1]) + int(lv[i+lw]) + int(lv[i+lw+1]) + 2) / 4)
			}
		}
		lv, lw, lh = next, nw, nh
	}
	return g
}

// cover is how much of the letter covers the point (u, v) of its cell (0
// to 1 across and down; u up to 2 for a wide letter), seen from a pixel du
// of a cell wide and dv of it tall.
func (g *r3glyph) cover(u, v, du, dv float64) float64 {
	fp := math.Max(du*glyphW, dv*glyphH) // the finest copy's pixels one of the screen's takes
	k := 0
	for ; k < len(g.lv)-1 && fp > 1.5; fp /= 2 {
		k++
	}
	if fp > 1.5 {
		return g.mean // smaller than the smallest copy
	}
	lw, lh := g.w[k], g.h[k]
	cw := float64(lw) // pixels a cell across
	if g.w[0] > glyphW {
		cw /= 2 // a wide letter's copy is two cells wide
	}
	x := u*cw - 0.5
	y := v*float64(lh) - 0.5
	x0, y0 := int(math.Floor(x)), int(math.Floor(y))
	fx, fy := x-float64(x0), y-float64(y0)
	at := func(x, y int) float64 {
		if x < 0 || y < 0 || x >= lw || y >= lh {
			return 0
		}
		return float64(g.lv[k][y*lw+x])
	}
	a := at(x0, y0)*(1-fx) + at(x0+1, y0)*fx
	b := at(x0, y0+1)*(1-fx) + at(x0+1, y0+1)*fx
	return (a*(1-fy) + b*fy) / 255
}

// paintGlyphs gives each letter of the back wall's cells its painted
// letter, and its cells the letter's mean colour for the smaller pictures
// (a face seen far off shows its words as a soft tint, not as nothing).
func (r *room3D) paintGlyphs(rm *r3room) {
	for i := range r.tex {
		t := &r.tex[i]
		if t.glyph == "" || t.gx != 0 {
			continue
		}
		g := glyphOf(t.glyph, t.bold, t.wide)
		if g == nil {
			continue // not in the font: its tint, as before
		}
		t.gl = g
		m := mix32(t.bg, t.fg, g.mean)
		t.q = [4]uint32{m, m, m, m}
		if t.wide && (i+1)%rm.tw != 0 {
			n := &r.tex[i+1]
			*n = *t
			n.gx, n.glyph = 1, ""
		}
	}
}

// cleanCard is whether structure o is shown in 3D as a clean card made
// from its words (cleanCards), not as its flat drawing.
func cleanCard(o *dOct) bool {
	switch o.Kind {
	case kOct, kPage, kNote, kSlide, kBook, kDrawing:
		return o.Image == ""
	}
	return false
}

// cleanCards draws each plain structure's front as a card: its colour,
// a strip of its own colour along the top, its title, its text and its
// footer (when it has one), the words in the middle. None of the flat
// board's frame (lines, dotted rules, cut corners) nor its placeholders
// ("footer", "3 lines"): those are for working on the flat board.
func (r *room3D) cleanCards(rm *r3room) {
	d := r.ds
	if d == nil {
		return
	}
	for _, b := range r.blocks {
		o := b.o
		if !cleanCard(o) {
			continue
		}
		x, y, w, h := d.rect(o)
		if w < 6 || h < 3 {
			continue
		}
		y += b.shift // on a layer: its drawing is kept below the board (layerCopies)
		fill := hex32(mixHex(boardBG(), o.Color, 0.2))
		edge := hex32(o.Color)
		head := hex32(mixHex(o.Color, cText, 0.35))
		text := hex32(cText)
		dim := hex32(mixHex(cText, mixHex(boardBG(), o.Color, 0.2), 0.35))
		at := func(cx, cy int) *r3texel {
			if cx < rm.tx0 || cy < rm.ty0 || cx >= rm.tx0+rm.tw || cy >= rm.ty0+rm.th {
				return nil
			}
			return &r.tex[(cy-rm.ty0)*rm.tw+cx-rm.tx0]
		}
		for cy := y; cy < y+h; cy++ {
			for cx := x; cx < x+w; cx++ {
				if t := at(cx, cy); t != nil {
					*t = r3texel{fg: text, bg: fill, q: [4]uint32{fill, fill, fill, fill}, blank: true}
					if cy == y {
						t.q[0], t.q[1] = edge, edge // the strip along the top
					}
					if o.Kind == kBook && cx == x {
						t.q[0], t.q[2] = edge, edge // a book: its spine down the left
					}
				}
			}
		}
		// line writes s in the middle of row cy, in colour fg.
		line := func(s string, cy int, fg uint32, bold bool) {
			n := ansi.StringWidth(s)
			cx := x + (w-n)/2
			for _, gr := range graphemes(s) {
				gw := ansi.StringWidth(gr)
				if t := at(cx, cy); t != nil && gw > 0 {
					t.fg, t.bold, t.blank = fg, bold, gr == " "
					if !t.blank {
						t.glyph, t.wide = gr, gw == 2
					}
				}
				cx += max(gw, 1)
			}
		}
		room := w - 4
		top, bottom := y+1, y+h-1 // rows for words: top ≤ row < bottom
		editing := partNone
		if d.sel == o.ID {
			editing = d.editing
		}
		// typed is part s while it is being written: its end, with the caret.
		typed := func(s string, part int) string {
			if editing != part {
				return ansi.Truncate(s, room, "…")
			}
			return ansi.TruncateLeft(s, max(0, ansi.StringWidth(s)-(room-1)), "…") + "▏"
		}
		if o.Header != "" || editing == partHeader {
			line(typed(o.Header, partHeader), top, head, true)
			top += 2
		}
		if (o.Footer != "" || editing == partFooter) && bottom-top >= 2 {
			bottom--
			line(typed(o.Footer, partFooter), bottom, dim, false)
			bottom-- // a blank row above it
		}
		if editing == partBody && bottom > top {
			// Being written: the end of the text, where the typing is, and the caret.
			lines := strings.Split(ansi.Wrap(o.Body, room-1, ""), "\n")
			lines = lines[max(0, len(lines)-(bottom-top)):]
			lines[len(lines)-1] += "▏"
			for i, l := range lines {
				line(l, top+i, text, false)
			}
		} else if o.Kind == kDrawing && o.Body != "" && bottom > top {
			// A drawing keeps its lines as they are, one under another,
			// the whole of it in the middle.
			lines := strings.Split(o.Body, "\n")
			lines = lines[:min(len(lines), bottom-top)]
			widest := 0
			for _, l := range lines {
				widest = max(widest, ansi.StringWidth(l))
			}
			x0 := x + max(0, (w-min(widest, room))/2)
			for i, l := range lines {
				cx := x0
				for _, gr := range graphemes(ansi.Truncate(l, room, "")) {
					gw := ansi.StringWidth(gr)
					if t := at(cx, top+i); t != nil && gw > 0 && gr != " " {
						t.fg, t.blank, t.glyph, t.wide = text, false, gr, gw == 2
					}
					cx += max(gw, 1)
				}
			}
		} else if o.Body != "" && bottom > top {
			lines := strings.Split(ansi.Wrap(o.Body, room, ""), "\n")
			if o.Kind == kPage && o.Scroll > 0 && o.Scroll < len(lines) {
				lines = lines[o.Scroll:]
			}
			rows := bottom - top
			if len(lines) > rows {
				lines = lines[:rows]
				lines[rows-1] = ansi.Truncate(lines[rows-1], room-1, "") + "…"
			}
			for i, l := range lines {
				line(strings.TrimRight(l, " "), top+i, text, false)
			}
		}
	}
}

// graphemes is s cut into what the terminal shows as one letter each.
func graphemes(s string) []string {
	var out []string
	g := uniseg.NewGraphemes(s)
	for g.Next() {
		out = append(out, g.Str())
	}
	return out
}

// smooth is the back wall's finest picture at (x, y) (its pixels, two a
// cell across, two a row down), between its four nearest pixels.
func (r *room3D) smooth(x, y, s float64) uint32 {
	l, w, h := r.flat0, r.mipW[0], r.mipH[0]
	x, y = x-0.5, y-0.5
	x0, y0 := int(ffloor(x)), int(ffloor(y))
	fx, fy := x-float64(x0), y-float64(y0)
	// Sharp, not soft: each of the wall's pixels is a clean square, and
	// only the screen pixel on the edge between two is blended, so the
	// edge is smooth but not blurred however close the camera comes. (s
	// is how many of the wall's pixels a screen pixel spans.)
	cl := func(v, n int) int { return max(0, min(n-1, v)) }
	if s = max(s, 1e-3); s < 1 {
		fx, fy = clamp01((fx-0.5)/s+0.5), clamp01((fy-0.5)/s+0.5)
		if (fx == 0 || fx == 1) && (fy == 0 || fy == 1) {
			// Inside one of the wall's pixels, away from its edges: that
			// pixel's colour, with no blend (most of the screen's pixels).
			return l[cl(y0+int(fy), h)*w+cl(x0+int(fx), w)]
		}
	}
	xa, xb, ya, yb := cl(x0, w), cl(x0+1, w), cl(y0, h), cl(y0+1, h)
	a, b, c, d := l[ya*w+xa], l[ya*w+xb], l[yb*w+xa], l[yb*w+xb]
	if a == b && b == c && c == d {
		return a
	}
	ch := func(sh uint) uint32 {
		v := (float64(a>>sh&255)*(1-fx)+float64(b>>sh&255)*fx)*(1-fy) +
			(float64(c>>sh&255)*(1-fx)+float64(d>>sh&255)*fx)*fy
		return uint32(v + 0.5)
	}
	return ch(16)<<16 | ch(8)<<8 | ch(0)
}
