package main

// ---- each drawer's colour and texture ----------------------------------------------------
//
// In a unit of several drawers each drawer has a colour and a texture of its
// own: its front is painted in it, cell by cell (wood, brushed metal, linen,
// paint or marble), and its tray is tinted with its colour. By default each
// drawer gets the next colour and texture, so no two neighbours look alike;
// a one-drawer unit keeps the unit's colour, in wood. With a unit chosen,
// each front shows a dot (its colour) and a swatch (its texture) at its
// right end: a click on either, or C and Shift+C (for the drawer that is
// out, else the first), changes it to the next one. Colours and textures
// are changes (undo takes them back), saved in Fronts.

import (
	"fmt"
	"math"
	"strings"

	"github.com/charmbracelet/x/ansi"
)

// dFront is how one drawer looks: its colour ("" the default) and texture
// (-1 the default, else drawerTextures' index).
type dFront struct {
	Color   string `json:"color,omitempty"`
	Texture int    `json:"texture,omitempty"` // stored plus one: 0 is the default
}

// drawerPalette are the colours a drawer's front goes through.
var drawerPalette = []string{"#c8834a", "#6f9fc4", "#8fbf73", "#d4a84a", "#b97fb5", "#d9786a", "#5fb3a8", "#a3a3ad"}

// drawerTextures are the textures a drawer's front goes through.
var drawerTextures = []string{"wood", "brushed metal", "linen", "paint", "marble"}

// drawerColor is drawer i's colour.
func drawerColor(o *dOct, i int) string {
	if i >= 0 && i < len(o.Fronts) && o.Fronts[i].Color != "" {
		return o.Fronts[i].Color
	}
	if drawerCount(o) == 1 {
		return o.Color
	}
	return mixHex(drawerPalette[i%len(drawerPalette)], o.Color, 0.12)
}

// drawerTexture is drawer i's texture (drawerTextures' index).
func drawerTexture(o *dOct, i int) int {
	if i >= 0 && i < len(o.Fronts) && o.Fronts[i].Texture > 0 {
		return (o.Fronts[i].Texture - 1) % len(drawerTextures)
	}
	if drawerCount(o) == 1 {
		return 0 // wood
	}
	return i % len(drawerTextures)
}

// setFront changes drawer i's look.
func setFront(o *dOct, i int, f func(*dFront)) {
	for len(o.Fronts) <= i {
		o.Fronts = append(o.Fronts, dFront{})
	}
	f(&o.Fronts[i])
}

// shadeRGB lightens (v > 0) or darkens (v < 0) c by v.
func shadeRGB(c rgbf, v float64) string {
	if v > 0 {
		c = c.mix(rgbf{255, 255, 255}, v)
	} else {
		c = c.mix(rgbf{0, 0, 0}, -v)
	}
	cl := func(v float64) int { return max(0, min(255, int(math.Round(v)))) }
	return fmt.Sprintf("#%02x%02x%02x", cl(c.r), cl(c.g), cl(c.b))
}

// texAt is the colour of pixel x (of w) on pixel row y (two to a cell's
// row) of a front of colour base in texture tex. It depends only on where
// the pixel is in the drawer, so it never flickers and moves with it.
func texAt(base rgbf, tex, x, y, w int) string {
	g := grain(x, y)
	fx, fy := float64(x), float64(y)
	var v float64
	switch tex {
	case 0: // wood: wavy lines of grain along it, and a knot now and then
		v = 0.15*math.Sin(fy*1.9+fx*0.07+2.4*grain(x/7, y/3)) + 0.03*g
		if k := grain(x/9, y/2+11); k > 0.86 {
			v -= 0.14
		}
	case 1: // brushed metal: a sheen across it, fine streaks along it
		t := (fx + 0.5) / float64(max(w, 1))
		v = 0.2*math.Exp(-math.Pow((t-0.3)/0.22, 2)) - 0.06 + 0.035*grain(x/4, y)
	case 2: // linen: a fine weave, over and under
		v = 0.02 * g
		if (x+y)%2 == 0 {
			v += 0.07
		} else {
			v -= 0.05
		}
	case 3: // paint: flat, a faint speckle
		v = 0.03 * g
	case 4: // marble: light veins winding across it
		vein := math.Abs(math.Sin(fx*0.21 + fy*0.8 + 2.8*grain(x/5, y/2)))
		v = 0.025*g - 0.03
		if vein < 0.22 {
			v = 0.3 - vein
		}
	}
	return shadeRGB(base, v)
}

// inkOn is a colour for words on bg: dark on a light colour, light on a dark one.
func inkOn(bg string) string {
	c := toRGB(bg)
	if 0.299*c.r+0.587*c.g+0.114*c.b > 140 {
		return mixHex(bg, "#101010", 0.78)
	}
	return mixHex(bg, "#f4f4f4", 0.8)
}

// frontRow draws a row of drawer i's front, w cells: textured (each cell
// two pixels tall, "▀"), with label at its left, the handle (hw wide, 0:
// none) in the middle, and, when tools is set, its colour dot and texture
// swatch at its right end. y is the row's place in the unit.
func frontRow(o *dOct, i int, front string, y, w int, label, labelFG string, hw int, tools bool) string {
	base := toRGB(front)
	tex := drawerTexture(o, i)
	lw := max((w-hw)/2, 0)
	labelR := []rune(label)
	if len(labelR) > max(lw-1, 0) {
		labelR = labelR[:max(lw-1, 0)]
	}
	py := 2 * (y + 7*i)
	top := func(x int) string { return texAt(base, tex, x, py, w) }
	bottom := func(x int) string { return texAt(base, tex, x, py+1, w) }
	var sb strings.Builder
	cell := func(x int, r rune, fg string, bold bool) {
		s := st(fg, bottom(x))
		if bold {
			s = s.Bold(true)
		}
		sb.WriteString(s.Render(string(r)))
	}
	dot, swatch := w-4, w-2 // its colour dot and its texture swatch, with a cell between
	for x := 0; x < w; x++ {
		switch {
		case hw > 0 && x == lw: // the handle, its mounts on the texture
			sb.WriteString(drawerHandleOn(hw, bottom(x), bottom(min(x+hw-1, w-1)), o.Color))
			x += hw - 1
		case tools && x == dot && dot > lw+hw:
			cell(x, '●', mixHex(drawerColor(o, i), "#ffffff", 0.3), true)
		case tools && x == swatch && dot > lw+hw:
			cell(x, '▒', mixHex(front, "#ffffff", 0.4), false)
		case x > 0 && x < len(labelR):
			cell(x, labelR[x], labelFG, true)
		default:
			sb.WriteString(st(top(x), bottom(x)).Render("▀")) // two pixels of texture
		}
	}
	return sb.String()
}

// drawerHandleOn is drawerHandle with its two mounts on the front's
// texture where they stand (left and right).
func drawerHandleOn(n int, left, right, tint string) string {
	n = max(n, 4)
	h := drawerHandle(n, left, tint)
	base := mixHex("#aeb4ba", tint, 0.12)
	mount := mixHex(base, "#20242a", 0.3)
	return ansi.Cut(h, 0, n-1) + st(mount, right).Render("▌")
}

// designDrawerLook changes the look of drawer i of o: its colour (tex
// false) or its texture to the next one.
func (m *Model) designDrawerLook(o *dOct, i int, tex bool) {
	n := drawerCount(o)
	if i < 0 || i >= n {
		return
	}
	if tex {
		next := (drawerTexture(o, i) + 1) % len(drawerTextures)
		setFront(o, i, func(f *dFront) { f.Texture = next + 1 })
		m.say(drawerName(o, i) + ": " + drawerTextures[next] + ". Shift+C: the next texture.")
	} else {
		cur, next := drawerColor(o, i), 0
		for k, c := range drawerPalette {
			if c == cur {
				next = (k + 1) % len(drawerPalette)
			}
		}
		setFront(o, i, func(f *dFront) { f.Color = drawerPalette[next] })
		m.say(drawerName(o, i) + ": a new colour. C: the next colour.")
	}
	m.ds().dirty = true
}

// lookSlot is the drawer C and Shift+C change: the first one out, else the first.
func lookSlot(o *dOct) int {
	for i := range drawerCount(o) {
		if slotOpen(o, i) {
			return i
		}
	}
	return 0
}
