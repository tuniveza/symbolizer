package main

// ---- a board's turned pins in the 3D picture: cards, their words turned with them ---------
//
// On the flat board a pin turned by other than quarter turns (or skewed)
// is a fine line of dots round its turned shape, its words upright inside
// it, cut to the room each row has (design_board.go): letters of the
// terminal cannot turn. In the 3D picture (kitty) they can: such a pin is
// left off its board's flat drawing and is a card of its own, a hair in
// front of the board, turned and skewed as it is, showing its card drawn
// upright (pinCard: its outline, title and text, and the pin through its
// top), so its words turn with it, whole, as print on a card does. The
// upright card is kept in rows of the picture below the board (as a
// structure on a layer keeps its drawing, layerCopies), and the card's
// face takes its pixels from there through its turn (r3face.aff). A click
// on the card is a click on the board cell behind it, as on the flat
// board, so it chooses and opens the same pin.

import (
	"image/color"
	"math"

	"charm.land/lipgloss/v2"
	uv "github.com/charmbracelet/ultraviolet"
)

// pinCarded is whether pin it is a card of its own in the 3D picture,
// not part of its board's flat drawing: turned by other than quarter
// turns, or skewed, and with no picture.
func pinCarded(it *dOct) bool {
	if it == nil || it.Image != "" {
		return false
	}
	_, ok := pinQuarter(it.Tf)
	return !ok
}

// r3pinCard is a turned pin shown as a card: block blk's (a board's)
// thing j, how it is turned (pinMap), and where its upright card is kept
// in the picture (cells tx, ty: gw × gh of them).
type r3pinCard struct {
	blk, j int
	p      pinMap
	tx, ty int
}

// pinCopies finds each board's turned pins and keeps room for their
// upright cards in rows below everything else in the picture.
func (r *room3D) pinCopies(d *designState, rm *r3room) {
	r.pinCards = r.pinCards[:0]
	if !r.paint {
		return // in letters the flat drawing shows them (design_3d_text.go)
	}
	const gap = 8 // rows between, so the smaller pictures do not blend one into another
	base := rm.ty0 + rm.th + gap
	for i := range r.blocks {
		o := r.blocks[i].o
		if o.Kind != kBoard {
			continue
		}
		for j, it := range o.Items {
			if !pinCarded(it) {
				continue
			}
			p := d.pinMapOf(o, it)
			if !p.ok || p.gw > rm.tw {
				continue
			}
			r.pinCards = append(r.pinCards, r3pinCard{blk: i, j: j, p: p, tx: rm.tx0, ty: base})
			base += p.gh + gap
		}
	}
	if len(r.pinCards) > 0 {
		rm.th = base - rm.ty0
	}
}

// pinTexels puts each turned pin's upright card in its rows of the picture.
func (r *room3D) pinTexels(d *designState, rm *r3room, defFG, defBG uint32) {
	for _, pc := range r.pinCards {
		o := r.blocks[pc.blk].o
		it := o.Items[pc.j]
		sel := d.pin.active && d.pin.board == o.ID && d.pin.at == pc.j
		g := pinCard(it, pc.p.gw, pc.p.gh, sel)
		if len(g) > 0 && len(g[0]) > 0 {
			fill, _ := miniColours(it, sel)
			g[0][len(g[0])/2] = miniCell{"●", "#e06c75", fill} // the pin, through its top
		}
		for y, row := range g {
			for x, mc := range row {
				cx, cy := pc.tx+x-rm.tx0, pc.ty+y-rm.ty0
				if cx < 0 || cy < 0 || cx >= rm.tw || cy >= rm.th {
					continue
				}
				cell := uv.Cell{Content: mc.r, Width: 1, Style: uv.Style{Fg: hexColor(mc.fg), Bg: hexColor(mc.bg)}}
				if mc.r == "●" {
					cell.Style.Attrs |= uv.AttrBold
				}
				r.tex[cy*rm.tw+cx] = texelOf(&cell, defFG, defBG)
			}
		}
	}
}

// hexColor is colour h ("#rrggbb"), or nil for none.
func hexColor(h string) color.Color {
	if h == "" {
		return nil
	}
	return lipgloss.Color(h)
}

// pinFaces adds the cards of board block i's turned pins, dep out from
// the wall (its front), each a hair further than the one before, so the
// last pinned is on top as on the flat board.
func (r *room3D) pinFaces(i int, dep, hw, hh float64) {
	b := &r.blocks[i]
	x, y := b.o.X, b.o.Y
	if r.ds != nil {
		x, y, _, _ = r.ds.rect(b.o)
	}
	n := 0
	for _, pc := range r.pinCards {
		if pc.blk != i {
			continue
		}
		n++
		p := pc.p
		gw, gh := float64(p.gw), float64(p.gh)
		z := dep + 0.01*float64(n)
		// A point of the card on the screen (from the board's corner, in
		// cells) is X = x + sx − hw across and Y = hh − 2(y + sy) up.
		at := func(u, v float64) r3vec {
			sx, sy := p.toScreen(u, v)
			return r3vec{float64(x) + sx - hw, hh - 2*(float64(y)+sy), z}
		}
		// Where on the upright card (columns, and half rows down) the point
		// (X, Y) is: toMini, written out for X and Y.
		ox, oy := hw-float64(x)-p.cx, hh-2*float64(y)-2*p.cy
		f := r3face{n: r3vec{0, 0, 1}, mat: matFront, own: b.o.ID, gloss: 0.3,
			cx0: b.x0, cx1: b.x1, py0: 2 * b.y0, py1: 2 * b.y1, aff: true,
			au: r3vec{p.ia, -p.ib, 0}, a0: p.ia*ox + p.ib*oy + gw/2 + float64(pc.tx),
			av: r3vec{p.ic, -p.id, 0}, b0: p.ic*ox + p.id*oy + gh + 2*float64(pc.ty),
			tcx0: pc.tx, tcx1: pc.tx + p.gw, tpy0: 2 * pc.ty, tpy1: 2 * (pc.ty + p.gh)}
		pts := []r3vec{at(0, 0), at(gw, 0), at(gw, gh), at(0, gh)}
		// Clockwise on the screen whatever the flip, as every face is.
		if (pts[1].x-pts[0].x)*(pts[3].y-pts[0].y)-(pts[1].y-pts[0].y)*(pts[3].x-pts[0].x) > 0 {
			pts[1], pts[3] = pts[3], pts[1]
		}
		r.addFace(f, pts...)
	}
}

// affScale is how many of the picture's cells a unit of an affine face
// spans (1 for a card only turned).
func (f *r3face) affScale() float64 {
	return math.Max(f.au.len(), f.av.len())
}
