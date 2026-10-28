package main

import (
	"fmt"
	"math/rand/v2"
	"strings"

	"charm.land/lipgloss/v2"
)

// The emblem: a small symbol made fresh each time sy starts, mirrored round a
// centre and lit like an ember: dim at the edges, brightest in the middle.
// It is built from one family of shapes so the parts match in size and
// weight: a diamond or four-point star in the middle, with a ring of dots,
// chevrons or small arrows pointing in, and sometimes thin brackets round
// that, e.g. ( · ◆ · ), › ✦ ‹. It sits either side of the title, and its
// halves wrap the SY blocks.

// emblemShapes is one family of shapes: Nerd Font ones in kitty, plain
// Unicode ones (which most fonts have) elsewhere.
type emblemShapes struct {
	centres []rune
	inner   [][2]rune // left, right: pointing in, toward the centre
	outer   [][2]rune
}

var nerdEmblem = emblemShapes{
	centres: []rune{0xF0A10, 0xF14DC, 0xF0AE2, 0xF0AE3, 0xF08CF, 0xF101D}, // ◆ ◇ ✦ ✧, a tall ◆ and ◇
	inner: [][2]rune{
		{0xF09DF, 0xF09DF}, // · ·
		{0xF0142, 0xF0141}, // › ‹
		{0xF035F, 0xF035E}, // ▸ ◂
	},
	outer: [][2]rune{
		{0xE0B7, 0xE0B5},   // ( )  thin half circles
		{0xF09DF, 0xF09DF}, // · ·
	},
}

var plainEmblem = emblemShapes{
	centres: []rune("◆◇"),
	inner:   [][2]rune{{'·', '·'}, {'❯', '❮'}, {'▶', '◀'}},
	outer:   [][2]rune{{'⟨', '⟩'}, {'·', '·'}},
}

type emblem struct {
	left, right []rune // read outwards-in on the left, inwards-out on the right
	centre      rune
}

func newEmblem() emblem {
	r := rand.New(rand.NewPCG(rand.Uint64(), rand.Uint64()))
	sh := plainEmblem
	if nerdIcons {
		sh = nerdEmblem
	}
	e := emblem{centre: sh.centres[r.IntN(len(sh.centres))]}
	if r.IntN(4) == 0 { // now and then the centre alone
		return e
	}
	in := sh.inner[r.IntN(len(sh.inner))]
	e.left, e.right = []rune{in[0]}, []rune{in[1]}
	if r.IntN(2) == 0 { // sometimes a second ring, never the same shape twice
		out := sh.outer[r.IntN(len(sh.outer))]
		if out != in {
			e.left = append([]rune{out[0]}, e.left...)
			e.right = append(e.right, out[1])
		}
	}
	return e
}

// ringColour lights the emblem like an ember: the centre in the accent, the
// next ring in the ember colour, the outer ring dimmer, toward the bar.
func ringColour(dist int) string {
	switch dist {
	case 0:
		return cAccent
	case 1:
		return cEmber
	}
	c := toRGB(cGradient[0]).mix(toRGB(cDim), 0.45)
	return fmt.Sprintf("#%02x%02x%02x", int(c.r), int(c.g), int(c.b))
}

// paint draws one shape, dist rings out from the centre.
func (e emblem) paint(ch rune, dist int, bg string) string {
	fg := ringColour(dist)
	if bg == blockFill() { // inside a block: sunk a little into the fill, so it does not shout
		c := toRGB(fg).mix(toRGB(bg), 0.4)
		fg = fmt.Sprintf("#%02x%02x%02x", int(c.r), int(c.g), int(c.b))
	}
	return lipgloss.NewStyle().Foreground(hex(fg)).Background(hex(bg)).Render(string(ch))
}

// halves gives the left and right parts (without the centre).
func (e emblem) halves(bg string) (string, string) {
	var l, r strings.Builder
	for i, ch := range e.left {
		l.WriteString(e.paint(ch, len(e.left)-i, bg))
	}
	for i, ch := range e.right {
		r.WriteString(e.paint(ch, i+1, bg))
	}
	return l.String(), r.String()
}

// full is the whole emblem.
func (e emblem) full(bg string) string {
	l, r := e.halves(bg)
	return l + e.paint(e.centre, 0, bg) + r
}

// wrap puts text in the middle of the emblem, where its centre would be.
func (e emblem) wrap(text, bg string, textStyle lipgloss.Style) string {
	l, r := e.halves(bg)
	sp := lipgloss.NewStyle().Background(hex(bg)).Render(" ")
	return l + sp + textStyle.Background(hex(bg)).Render(text) + sp + r
}

// blockFill is the soft fill of the SY and mode blocks: the bar colour with a
// little ember in it (calmer than a solid orange pill).
func blockFill() string {
	c := toRGB(cStatusBG).mix(toRGB(cEmber), 0.20)
	return fmt.Sprintf("#%02x%02x%02x", int(c.r), int(c.g), int(c.b))
}

// block puts content in a block with rounded ends (Nerd Font caps), sitting
// on a bar of colour outer.
func block(content, outer string) string {
	fill := blockFill()
	cap := lipgloss.NewStyle().Foreground(hex(fill)).Background(hex(outer))
	l, r := icon("\ue0b6", ""), icon("\ue0b4", "")
	pad := lipgloss.NewStyle().Background(hex(fill)).Render(" ")
	if l == "" {
		return pad + content + pad
	}
	return cap.Render(l) + content + cap.Render(r)
}

// syBlock is "SY" inside the emblem, in a block.
func (m *Model) syBlock(outer string) string {
	text := lipgloss.NewStyle().Foreground(hex(cAccent)).Bold(true)
	return block(m.emblem.wrap("SY", blockFill(), text), outer)
}
