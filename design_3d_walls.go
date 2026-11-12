package main

// ---- the room's walls: plaster, wallpaper and rails ------------------------------------
//
// The walls, the ceiling and the empty parts of the board are not flat
// colour: each has a soft plaster grain, and the side walls (and the wall
// behind the camera) are papered, with a skirting board, a painted lower
// part under a dado rail, and a picture rail near the ceiling. Every
// pattern is worked out where the pixel's ray meets the surface, in the
// room's own units, so it stays put on the wall when the camera moves; and
// each is told how many units one pixel spans there (foot), so a detail
// smaller than a pixel or two fades into its average instead of
// flickering from frame to frame. Nothing here counts pixels a cell.

import "math"

// How strong the plaster grain is on each surface, on a light board: out
// of 255, either way (see grainFor for a dark one). The board's is the
// softest, so words on it stay easy to read.
const (
	boardGrain   = 4.0
	ceilingGrain = 5.0
	wallGrain    = 4.5
)

// grainFor is how much stronger every pattern is drawn on a board this
// light (0 dark to 1 white): on a dark board the light makes the same
// grain a few shades fainter still, and it would not show at all.
func grainFor(lightness float64) float64 { return 2 - 1.2*lightness }

// noiseTile is a 64×64 square of fixed random values (from grain), which
// noise blends between; it repeats, but the plaster's three sizes of bumps
// repeat at different lengths, so no repeat shows.
var noiseTile = func() (t [64 * 64]float32) {
	for y := range 64 {
		for x := range 64 {
			t[y*64+x] = float32(grain(x+101, y+37))
		}
	}
	return t
}()

// noise is a smooth random value at (x, y), from about −1 to 1: the
// tile's values sit on whole numbers, blended softly between them.
func noise(x, y float64) float64 {
	fx, fy := math.Floor(x), math.Floor(y)
	u, v := x-fx, y-fy
	u, v = u*u*(3-2*u), v*v*(3-2*v)
	ix, iy := int(fx)&63, int(fy)&63
	ix1, iy1 := (ix+1)&63, (iy+1)&63
	a, b := float64(noiseTile[iy*64+ix]), float64(noiseTile[iy*64+ix1])
	c, d := float64(noiseTile[iy1*64+ix]), float64(noiseTile[iy1*64+ix1])
	top, bot := a+(b-a)*u, c+(d-c)*u
	return top + (bot-top)*v
}

// plaster is a wall's soft unevenness at (u, v) (units across and up),
// with a pixel spanning foot units: three sizes of bumps, each fading out
// as it gets smaller than about two pixels. From about −1 to 1.
func plaster(u, v, foot float64) float64 {
	s := 0.0
	for k, o := range [3]struct{ size, w float64 }{{13, 0.5}, {4.5, 0.32}, {1.6, 0.25}} {
		fade := clamp01(o.size/foot - 1.5)
		if fade == 0 {
			break // the smaller ones are finer still
		}
		off := 23.7 * float64(k) // each size its own part of the tile
		s += o.w * fade * noise(u/o.size+off, v/o.size+off)
	}
	return s
}

// clamp01 is v kept within 0 to 1 (plain sums: it runs for every pixel).
func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

// plasterAt is colour base with the plaster at (u, v), amp strong.
func plasterAt(base uint32, u, v, foot, amp float64) uint32 {
	return shade32(base, amp*plaster(u, v, foot))
}

// shade32 is colour c with d added to each of red, green and blue.
func shade32(c uint32, d float64) uint32 {
	r, g, b := chans(c)
	return rgb32(byteOf(r+d+0.5), byteOf(g+d+0.5), byteOf(b+d+0.5))
}

// cover is how much of a pixel spanning foot units round h a band from lo
// to hi covers (0 to 1): a band thinner than a pixel is a faint line, not a
// line that comes and goes.
func cover(h, lo, hi, foot float64) float64 {
	return clamp01((min(h+foot/2, hi) - max(h-foot/2, lo)) / foot)
}

// wallAt is a papered wall's colour at u units along it and y up, with a
// pixel spanning foot units: from the floor up, a skirting board, a
// painted lower part, a dado rail, soft striped wallpaper, a picture rail
// and plaster to the ceiling. The rails are in a shade toward the words'
// colour text, as the skirting board always was; the paper's patterns are
// gk times as strong as on a light board (see grainFor).
func wallAt(base, text uint32, u, y, foot, gk float64, rm *r3room) uint32 {
	h := y - rm.fy // how high above the floor
	const skirt, rail = 3.0, 1.2
	dado := max(9, 0.14*rm.big)
	pic := rm.cy - rm.fy - max(3, 0.05*rm.big)
	if pic < dado+8 {
		pic = math.Inf(1) // a low room: no picture rail
	}
	d := gk * wallGrain * plaster(u, y, foot)
	switch {
	case h < dado:
		d -= 3 * gk // the lower part: a shade darker
	case h < pic:
		// The paper: wide soft stripes 6 units apart (a curve close to a
		// wave, cheaper), a fine line between them; both fade where they
		// would be too fine to show.
		const period = 6.0
		if a := clamp01(period/foot/3 - 0.5); a > 0 {
			k := u/period - math.Floor(u/period+0.5) // from the nearest line, in periods: −½ to ½
			d += 2.5 * gk * a * (1 - 8*k*k)
			d -= 6 * gk * a * cover(k*period, -0.15, 0.15, foot)
		}
	}
	col := shade32(base, d)
	// The skirting board, the dado rail and the picture rail, each with a
	// thin shadow under it.
	near := rail + foot + 1
	if h < skirt+near {
		trim := mix32(base, text, 0.13)
		if c := cover(h, -1, skirt, foot); c > 0 {
			col = mix32(col, trim, c)
		}
		if c := cover(h, skirt, skirt+0.4, foot); c > 0 {
			col = mix32(col, 0, 0.1*c)
		}
	}
	for _, at := range [2]float64{dado, pic} {
		if h < at-near || h > at+near {
			continue
		}
		col = mix32(col, 0, 0.12*cover(h, at-0.45, at, foot))
		col = mix32(col, mix32(base, text, 0.13), cover(h, at, at+rail, foot))
	}
	return col
}

// backWallAt is the back wall's colour round the board at (x, y): papered
// as the side walls are, with a frame round the board (a band in the
// rails' shade), so the board hangs on it like a panel.
func backWallAt(base, text uint32, x, y, foot, gk float64, rm *r3room) uint32 {
	col := wallAt(base, text, x, y, foot, gk, rm)
	out := max(rm.x0-x, x-rm.x1, rm.y0-y, y-rm.y1) // how far outside the board
	if out < 0.8+foot {
		if c := cover(out, 0, 0.8, foot); c > 0 {
			col = mix32(col, mix32(base, text, 0.22), c)
		}
	}
	return col
}
