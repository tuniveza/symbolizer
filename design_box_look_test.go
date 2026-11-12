package main

import (
	"strings"
	"testing"
)

// lumOf is how light a colour "#rrggbb" is, 0 to 255.
func lumOf(hex string) float64 {
	r, g, b := rgb(hex)
	return 0.299*float64(r) + 0.587*float64(g) + 0.114*float64(b)
}

// Open, each design is drawn as a box seen from a little above: a rim
// round it (painted, part of the box: a drag there moves it), the inside
// of its far wall under the rim, darker than the rim, a floor darker
// still, and the near wall's outside face along its bottom. Shut, its lid
// covers it, with a front edge darker than its top, and the box's front
// under it.
func TestBoxLooksLikeABox(t *testing.T) {
	m, d, b := boxTest(t)
	_ = m
	b.Items = nil
	w, h := 46, 17
	for des := range boxDesigns {
		b.Layout, b.DrawerOpen = des, true
		name := boxDesignNames[des]
		s := boxShapeOf(des, w, h)
		g := d.boxGridOf(b, w, h)
		mid := h / 2
		side := g.c[mid][s.cut[mid]] // its left wall, half way down
		if side.bg == cBG || side.act != "" {
			t.Fatalf("%s: its left wall should be painted and move the box: %+v", name, side)
		}
		rows := s.insideRows(w, h)
		cx := w / 2
		rim := g.c[rows[0]-1][cx+6]                   // the far rim, beside the middle
		far := g.c[rows[0]][cx+6]                     // the inside of the far wall, under it
		floor := g.c[rows[len(rows)/2+2]][cx+6]       // the floor
		front := g.c[h-1][cx+6]                       // the near wall's outside face
		rimLum := (lumOf(rim.fg) + lumOf(rim.bg)) / 2 // its two pixels
		if lumOf(far.fg) >= rimLum || lumOf(floor.bg) >= rimLum || lumOf(front.bg) >= rimLum {
			t.Fatalf("%s: the far wall's inside (%s), the floor (%s) and the front (%s) should be darker than the rim (%s %s)",
				name, far.fg, floor.bg, front.bg, rim.fg, rim.bg)
		}
		if lumOf(far.bg) >= lumOf(far.fg) {
			t.Fatalf("%s: the far wall's inside should get darker toward the floor: %s over %s", name, far.fg, far.bg)
		}
		if floor.act != "boxfloor" {
			t.Fatalf("%s: a click on its floor should be on its floor, not %q", name, floor.act)
		}

		b.DrawerOpen = false
		g = d.boxGridOf(b, w, h)
		lid := 0.0 // how light its top is, on the average
		for x := cx + 2; x < cx+8; x++ {
			c := g.c[rows[len(rows)/2]+3][x]
			lid += (lumOf(c.fg) + lumOf(c.bg)) / 12
		}
		edge := g.c[h-s.bottom-1][cx+5] // the lid's last row
		under := g.c[h-1][cx+5]
		if edge.r != "▀" || lumOf(edge.bg) >= lumOf(edge.fg) || lumOf(edge.bg) >= lid {
			t.Fatalf("%s shut: the lid's last row should show its front edge, darker: %+v (lid %.0f)", name, edge, lid)
		}
		if lumOf(under.bg) >= lid {
			t.Fatalf("%s shut: the box's front under the lid (%s) should be darker than the lid (%.0f)", name, under.bg, lid)
		}
		for y := range h {
			for x := range w {
				if strings.HasPrefix(g.c[y][x].act, "boxthing") || g.c[y][x].act == "boxfloor" {
					t.Fatalf("%s shut: cell %d,%d of the lid does %q", name, x, y, g.c[y][x].act)
				}
			}
		}
	}
}

// At every zoom from 25% to 300% a box shows its name, and its things'
// names show where they are not covered.
func TestBoxEveryZoomReads(t *testing.T) {
	m, d, b := boxTest(t)
	for zi := range designZooms {
		d.zoomTo(zi, 0, 0)
		d.camX, d.camY = 0, 0
		for des := range boxDesigns {
			b.Layout = des
			d.dirty = true
			s := screenOf(m)
			want := "Toys"
			if designZooms[zi] < 0.5 {
				want = "To" // a small box's name may be cut short
			}
			if !strings.Contains(s, want) {
				t.Fatalf("zoom %d, %s: the box's name should show:\n%s", zi, boxDesignNames[des], s)
			}
			if designZooms[zi] >= 1 && !strings.Contains(s, "Idea") {
				t.Fatalf("zoom %d, %s: the top thing's name should show:\n%s", zi, boxDesignNames[des], s)
			}
		}
	}
}
