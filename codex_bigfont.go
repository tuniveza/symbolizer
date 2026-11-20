package main

// ---- Fonts in big letters ---------------------------------------------------------------------
//
// Big letters (sizes 2–4) are drawn from one small pixel font (fxPixFont).
// So that a font shows at every size (the user asked, 19 Nov 2026), each of
// the 16 fonts reshapes those pixel letters the way the font looks: bold
// strokes are thicker, italic leans, script leans and joins its letters,
// double has two lines per stroke, wide is twice as wide, circled and
// squared are light letters on a dark block, upside down is turned round …
// Every font keeps the letters 5 pixel rows tall, so a line of big letters
// takes the same rows in any font.

import "strings"

// The font numbers (codexFonts) the shapes below need.
const (
	fxFontBold = 1 + iota
	fxFontItalic
	fxFontBoldItalic
	fxFontScript
	fxFontBoldScript
	fxFontFraktur
	fxFontDouble
	fxFontMono
	fxFontSans
	fxFontSansBold
	fxFontSmallCaps
	_ // wide: fxFontWide
	fxFontCircled
	fxFontSquared
)

// fxLeans is whether a font's big letters lean (italic, script).
func fxLeans(font int) bool {
	switch font {
	case fxFontItalic, fxFontBoldItalic, fxFontScript, fxFontBoldScript:
		return true
	}
	return false
}

// fxJoins is whether a font's big letters are joined at the bottom (script).
func fxJoins(font int) bool { return font == fxFontScript || font == fxFontBoldScript }

// fxLean is how many columns the top of a line of big letters at size
// leans over to the right (0: the font stands up).
func fxLean(font, size int) int {
	if size <= 1 || !fxLeans(font) {
		return 0
	}
	return size - 1
}

// fxGrid is a pixel letter as rows of on (true) and off.
type fxGrid [fxPixRows][]bool

func fxGridOf(g [fxPixRows]string) fxGrid {
	var out fxGrid
	for y, row := range g {
		for _, c := range row {
			out[y] = append(out[y], c == '#')
		}
	}
	return out
}

func (g fxGrid) w() int { return len(g[0]) }

func (g fxGrid) at(x, y int) bool {
	return y >= 0 && y < fxPixRows && x >= 0 && x < len(g[y]) && g[y][x]
}

// fxRemap makes a new letter w wide whose pixel x, y is on when on says.
func fxRemap(w int, on func(x, y int) bool) fxGrid {
	var out fxGrid
	for y := range out {
		out[y] = make([]bool, w)
		for x := 0; x < w; x++ {
			out[y][x] = on(x, y)
		}
	}
	return out
}

// fxStyledGlyph is the pixel letter for r in font.
func fxStyledGlyph(r rune, font int) fxGrid {
	small := r >= 'a' && r <= 'z'
	space := r == ' ' || r == '\t'
	g := fxGridOf(fxPixGlyph(r))
	if space {
		if font == fxFontWide {
			return fxRemap(g.w()*2, func(int, int) bool { return false })
		}
		return g
	}
	bold := func(g fxGrid) fxGrid { // each upright stroke two pixels wide (the letter's holes stay open)
		var cols []int // the old column each new one shows
		for x := 0; x < g.w(); x++ {
			n := 0
			for y := 0; y < fxPixRows; y++ {
				if g.at(x, y) {
					n++
				}
			}
			cols = append(cols, x)
			if n >= 3 {
				cols = append(cols, x)
			}
		}
		return fxRemap(len(cols), func(x, y int) bool { return g.at(cols[x], y) })
	}
	round := func(g fxGrid) fxGrid { // the outside corners taken off
		return fxRemap(g.w(), func(x, y int) bool {
			if !g.at(x, y) {
				return false
			}
			ends := 0
			for _, d := range [][2]int{{-1, 0}, {1, 0}, {0, -1}, {0, 1}} {
				if g.at(x+d[0], y+d[1]) {
					ends++
				}
			}
			corner := (x == 0 || x == g.w()-1) && (y == 0 || y == fxPixRows-1)
			return !(corner && ends == 2 && (g.at(x+1, y) || g.at(x-1, y)) && (g.at(x, y+1) || g.at(x, y-1)))
		})
	}
	switch font {
	case fxFontBold, fxFontBoldItalic, fxFontBoldScript:
		return bold(g)
	case fxFontSans:
		return round(g)
	case fxFontSansBold:
		return round(bold(g))
	case fxFontFraktur: // heavy, with the top left of each stroke cut on a slant
		b := bold(g)
		return fxRemap(b.w(), func(x, y int) bool {
			return b.at(x, y) && !(y == 0 && !b.at(x-1, y) && b.at(x+1, y) && !b.at(x, y+1)) &&
				!(x == 0 && y == 0)
		})
	case fxFontDouble: // two lines for each stroke
		return fxRemap(g.w()+2, func(x, y int) bool { return g.at(x, y) || g.at(x-2, y) })
	case fxFontMono: // every letter as wide as the widest
		pad := 5 - g.w()
		l := pad / 2
		return fxRemap(5, func(x, y int) bool { return g.at(x-l, y) })
	case fxFontSmallCaps: // small letters: capitals, a row shorter
		if !small {
			return g
		}
		return fxRemap(g.w(), func(x, y int) bool {
			switch y {
			case 0:
				return false
			case 1:
				return g.at(x, 0)
			case 2:
				return g.at(x, 1) || g.at(x, 2)
			}
			return g.at(x, y)
		})
	case fxFontWide:
		return fxRemap(g.w()*2, func(x, y int) bool { return g.at(x/2, y) })
	case fxFontCircled: // the letter between two round sides: (A)
		w := g.w() + 4
		return fxRemap(w, func(x, y int) bool {
			if x == 0 || x == w-1 {
				return y >= 1 && y <= 3
			}
			return g.at(x-2, y)
		})
	case fxFontSquared: // a light letter cut out of a dark block
		return fxRemap(g.w()+2, func(x, y int) bool { return !g.at(x-1, y) })
	case fxFontUpsideDown: // turned round (the letters' order is turned in codexBigF)
		return fxRemap(g.w(), func(x, y int) bool { return g.at(g.w()-1-x, fxPixRows-1-y) })
	}
	return g
}

// codexBigF draws s in big letters in font and gives back the rows (see
// codexBig: size 2 is the pixel font as it is, 3 doubles it, 4 triples it).
func codexBigF(s string, size, font int) []string {
	if size <= 1 {
		return []string{codexFont(s, font)}
	}
	if size > codexSizes {
		size = codexSizes
	}
	font = (font%16 + 16) % 16
	k := size - 1
	lean := fxLean(font, size)
	var rows []string
	for _, line := range strings.Split(s, "\n") {
		rs := []rune(line)
		if font == fxFontUpsideDown {
			for i, j := 0, len(rs)-1; i < j; i, j = i+1, j-1 {
				rs[i], rs[j] = rs[j], rs[i]
			}
		}
		// px[y] is one pixel row of the whole line, already scaled.
		px := make([][]bool, (fxPixRows+1)*k)
		for i, r := range rs {
			g := fxStyledGlyph(r, font)
			if i > 0 { // the gap between letters; script joins them along the bottom
				join := fxJoins(font) && r != ' ' && rs[i-1] != ' '
				for y := range px {
					on := join && y/k == fxPixRows-1
					for n := 0; n < k; n++ {
						px[y] = append(px[y], on)
					}
				}
			}
			for gy := 0; gy < fxPixRows; gy++ {
				for _, on := range g[gy] {
					for sy := 0; sy < k; sy++ {
						for sx := 0; sx < k; sx++ {
							px[gy*k+sy] = append(px[gy*k+sy], on)
						}
					}
				}
			}
			for y := fxPixRows * k; y < len(px); y++ { // the empty row under the letter
				for n := 0; n < g.w()*k; n++ {
					px[y] = append(px[y], false)
				}
			}
		}
		if len(px[0]) == 0 {
			rows = append(rows, make([]string, len(px)/2)...)
			continue
		}
		if lean > 0 { // italic: each row of the screen moved right, the top most (whole rows, so the ▀ ▄ halves stay together)
			last := (fxPixRows*k+1)/2 - 1 // the letters' last screen row
			for y := range px {
				sh := 0
				if cr := y / 2; cr <= last {
					sh = (last - cr) * lean / max(last, 1)
				}
				row := make([]bool, sh, len(px[y])+lean)
				row = append(row, px[y]...)
				for len(row) < len(px[y])+lean {
					row = append(row, false)
				}
				px[y] = row
			}
		}
		w := len(px[0])
		for y := 0; y+1 < len(px); y += 2 {
			var b strings.Builder
			for x := 0; x < w; x++ {
				top, bot := px[y][x], px[y+1][x]
				switch {
				case top && bot:
					b.WriteRune('█')
				case top:
					b.WriteRune('▀')
				case bot:
					b.WriteRune('▄')
				default:
					b.WriteByte(' ')
				}
			}
			rows = append(rows, b.String())
		}
	}
	return fxPadRows(rows)
}

// fxBigWidth is how many cells rs takes in big letters of font at size
// (without the lean: fxLean adds that to a whole line).
func fxBigWidth(rs []rune, font, size int) int {
	k := size - 1
	n := 0
	for i, r := range rs {
		if i > 0 {
			n += k
		}
		n += fxStyledGlyph(r, font).w() * k
	}
	return n
}
