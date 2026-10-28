package main

import (
	"fmt"
	"math"
	"strings"

	"github.com/charmbracelet/x/ansi"
)

// Gradients and texture. Each bar's background is repainted cell by cell: a
// soft glow across its width (warmer in the middle, a touch of ember) plus a
// fine, fixed grain (two layers of hashed noise), like the Molten resin. The
// grain depends only on the cell's position, so it never flickers. The text
// area and its margins stay the plain background: in kitty that colour is
// see-through (its picture shows), and grain there would be solid squares.

// grain is a fixed noise value for a cell, from -1 to 1.
func grain(x, y int) float64 {
	h := uint32(x)*374761393 + uint32(y)*668265263
	h = (h ^ (h >> 13)) * 1274126177
	fine := float64(h&0xffff)/32768 - 1
	// a coarser layer, in blocks of 5x1 cells, for a resin-like unevenness
	cx := x / 5
	k := uint32(cx)*2246822519 + uint32(y)*3266489917
	k = (k ^ (k >> 15)) * 668265263
	coarse := float64(k&0xffff)/32768 - 1
	return fine*0.55 + coarse*0.45
}

type rgbf struct{ r, g, b float64 }

func toRGB(h string) rgbf {
	r, g, b := rgb(h)
	return rgbf{float64(r), float64(g), float64(b)}
}

func (a rgbf) mix(b rgbf, t float64) rgbf {
	return rgbf{a.r + (b.r-a.r)*t, a.g + (b.g-a.g)*t, a.b + (b.b-a.b)*t}
}

func (a rgbf) add(v float64) rgbf { return rgbf{a.r + v, a.g + v, a.b + v} }

func (a rgbf) sgr() string {
	c := func(v float64) int { return max(0, min(255, int(math.Round(v)))) }
	return fmt.Sprintf("\x1b[48;2;%d;%d;%dm", c(a.r), c(a.g), c(a.b))
}

// Painting is remembered: the same cell in the same colour is worked out
// once, and a row that has not changed since the last frame is not painted
// again (typing changes one bar at most).
type texKey struct {
	base        string
	x, y, width int
}

var texCache = map[texKey]string{}

// textured is the background for cell x of a bar of the given width and base
// colour.
func textured(base string, x, y, width int) string {
	k := texKey{base, x, y, width}
	if s, ok := texCache[k]; ok {
		return s
	}
	if len(texCache) > 200000 {
		texCache = map[texKey]string{}
	}
	s := texturedNew(base, x, y, width)
	texCache[k] = s
	return s
}

func texturedNew(base string, x, y, width int) string {
	b := toRGB(base)
	t := 0.5 - 0.5*math.Cos(2*math.Pi*(float64(x)+0.5)/float64(max(width, 1))) // 0 at the ends, 1 in the middle
	lit := b.mix(toRGB(cEmber), 0.07*t).add(6 * t)                             // a soft glow in the middle
	if isLightTheme() {
		lit = b.mix(toRGB(cEmber), 0.05*t).add(-4 * t)
	}
	return lit.add(3.2 * grain(x, y)).sgr()
}

func isLightTheme() bool {
	c := toRGB(cBG)
	return c.r+c.g+c.b > 3*128
}

// paintRow gives every cell of one screen row whose background is one of
// the bar colours its textured, gradient background instead.
type rowMemo struct{ in, out string }

var paintMemo = map[[2]int]rowMemo{}

func paintRow(s string, y, width int) string {
	if m, ok := paintMemo[[2]int{y, width}]; ok && m.in == s {
		return m.out
	}
	out := paintRowNew(s, y, width)
	paintMemo[[2]int{y, width}] = rowMemo{s, out}
	return out
}

var barKeys = map[string]map[string]string{} // theme background -> its "48;2;r;g;b" bar keys

func paintRowNew(s string, y, width int) string {
	bars, ok := barKeys[cBarBG+cStatusBG]
	if !ok {
		bars = map[string]string{}
		for _, c := range []string{cBarBG, cStatusBG} {
			r, g, b := rgb(c)
			bars[fmt.Sprintf("48;2;%d;%d;%d", r, g, b)] = c
		}
		barKeys[cBarBG+cStatusBG] = bars
	}
	var out strings.Builder
	out.Grow(len(s) * 3)
	var state byte
	x := 0
	current := "" // the bar colour the next letters are on, if any
	for len(s) > 0 {
		seq, w, n, newState := ansi.DecodeSequence(s, state, nil)
		state = newState
		if n <= 0 {
			break
		}
		if w == 0 && strings.HasPrefix(seq, "\x1b[") && strings.HasSuffix(seq, "m") {
			params := seq[2 : len(seq)-1]
			switch {
			case params == "" || params == "0":
				current = ""
			default:
				for k, c := range bars {
					if strings.Contains(params, k) {
						current = c
						goto found
					}
				}
				if strings.Contains(params, "48;") || strings.Contains(params, "49") {
					current = ""
				}
			found:
			}
			out.WriteString(seq)
		} else if w > 0 && current != "" {
			out.WriteString(textured(current, x, y, width))
			out.WriteString(seq)
		} else {
			out.WriteString(seq)
		}
		x += w
		s = s[n:]
	}
	return out.String()
}
