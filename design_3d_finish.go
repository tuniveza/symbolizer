package main

// ---- the room's finishes: what the walls and the floor are made of ------------------------
//
// In 3D, T picks the next finish for the walls (Shift+T the one before),
// and F the next for the floor (Shift+F the one before). The first of
// each is the room as it always was: papered walls with rails, and
// planks. The choice is kept with the board (design_store.go), not as an
// undo step.
//
// Every finish follows the walls' rule (design_3d_walls.go): it is worked
// out where the pixel's ray meets the surface, in the room's units, so it
// stays put when the camera moves, and each detail fades into its average
// once it is smaller than a pixel or two (foot is how many units a pixel
// spans there), so nothing flickers. The lamp lights it as before. Each is
// written twice, here and in the card's shader (design_3d_gpu_glsl.go):
// change one and change the other.

import (
	"math"
	"strings"
)

// The walls' finishes, in the order T goes through them.
const (
	wallPaper    = iota // striped wallpaper, a painted lower part, rails and a skirting board
	wallBrick           // bare brick
	wallPanels          // wooden boards, upright
	wallConcrete        // smooth painted concrete with tie holes
	wallTiles           // white tiles laid like bricks
	wallTrellis         // wallpaper with a diamond pattern, and the rails
	wallFelt            // dark felt panels that soak up sound
)

// wallNames are the walls' finishes as the status bar says them and the
// board file keeps them.
var wallNames = []string{"paper", "brick", "wood panels", "concrete", "tiles", "trellis paper", "felt panels"}

// The floor's finishes, in the order F goes through them.
const (
	floorPlanks   = iota // planks running out from the wall
	floorParquet         // small wood blocks in a herringbone
	floorCarpet          // office carpet tiles
	floorConcrete        // polished concrete with saw cuts
	floorMarble          // light and dark marble squares
	floorOak             // wide light oak boards
	floorSlate           // slate flags
)

var floorNames = []string{"planks", "parquet", "carpet tiles", "concrete", "marble", "oak", "slate"}

// finishIndex is name's place in names, or 0 (the first) when it is not
// one of them: a board from an older or newer sy still opens.
func finishIndex(names []string, name string) int {
	for i, n := range names {
		if n == name {
			return i
		}
	}
	return 0
}

// finishName is finish i as kept in the board file: "" for the first, so
// a board that never chose one is written as before.
func finishName(names []string, i int) string {
	if i <= 0 || i >= len(names) {
		return ""
	}
	return names[i]
}

// design3DFinishKey changes the walls (T, Shift+T) or the floor (F,
// Shift+F) in 3D. It says whether it took the key. Label mode keeps T for
// its own.
func (m *Model) design3DFinishKey(name string) bool {
	d := m.ds()
	if d.labelMode {
		return false
	}
	step := 1
	if strings.HasPrefix(name, "shift+") || name == "T" || name == "F" {
		step = -1
	}
	switch strings.ToLower(strings.TrimPrefix(name, "shift+")) {
	case "t":
		d.walls = (d.walls + step + len(wallNames)) % len(wallNames)
		m.say("Walls: " + wallNames[d.walls] + ".  T: next · Shift+T: back")
	case "f":
		d.floor = (d.floor + step + len(floorNames)) % len(floorNames)
		m.say("Floor: " + floorNames[d.floor] + ".  F: next · Shift+F: back")
	default:
		return false
	}
	d.dirty = true    // saved with the board (not an undo step: the structures did not change)
	d.room.frame = "" // drawn afresh
	return true
}

// wallGloss and floorGloss are how shiny each finish is (r3face.gloss).
func wallGloss(fin int) float64 {
	return [...]float64{0.1, 0.04, 0.25, 0.12, 0.5, 0.1, 0}[clampFin(fin, len(wallNames))]
}

func floorGloss(fin int) float64 {
	return [...]float64{0.6, 0.5, 0.04, 0.7, 0.85, 0.45, 0.3}[clampFin(fin, len(floorNames))]
}

func clampFin(fin, n int) int { return max(0, min(n-1, fin)) }

// wallHasRails is whether a finish has the dado and picture rails (the
// letters' drawing of the room draws them, design_3d_text.go).
func wallHasRails(fin int) bool { return fin == wallPaper || fin == wallTrellis }

// ---- helpers, each in the shader too --------------------------------------------------------

// fracF is the part of x after the point (from 0 up to 1, below 0 too).
func fracF(x float64) float64 { return x - ffloor(x) }

// modF is a modulo b, from 0 up to b, below 0 too.
func modF(a, b float64) float64 { return a - b*ffloor(a/b) }

// shows is how much a detail size units big shows where a pixel spans
// foot: all of it from three pixels up, none below one.
func shows(size, foot float64) float64 { return clamp01(size/foot/2 - 0.5) }

// seam is how much of a pixel a line w units either side of its middle
// covers, d units away from it.
func seam(d, w, foot float64) float64 { return cover(d, -w, w, foot) }

// cellRand is a fixed random value (about −1 to 1) for the whole numbers
// i and j: one brick's, one tile's or one board's own shade.
func cellRand(i, j float64) float64 { return noise(i+0.03, j+0.03) }

// noiseSmooth is noise blended between its sixteenths too: for a
// pattern that a step in noise would show in (the marble's veins).
func noiseSmooth(x, y float64) float64 {
	const n = 64 * noiseK
	x, y = x*noiseK, y*noiseK
	fx, fy := ffloor(x), ffloor(y)
	u, v := x-fx, y-fy
	ix, iy := int(fx)&(n-1), int(fy)&(n-1)
	ix1, iy1 := (ix+1)&(n-1), (iy+1)&(n-1)
	a, b := float64(noiseFine[iy*n+ix]), float64(noiseFine[iy*n+ix1])
	c, d := float64(noiseFine[iy1*n+ix]), float64(noiseFine[iy1*n+ix1])
	top, bot := a+(b-a)*u, c+(d-c)*u
	return top + (bot-top)*v
}

// The finishes' own colours, each mixed into the theme's wall or floor
// colour (so a dark theme keeps a dark room).
const (
	colBrick   = 0x9c4f3a
	colMortar  = 0xc4beb2
	colWood    = 0x8a5a36
	colConcr   = 0xa29e96
	colTile    = 0xf0eee8
	colGrout   = 0x8f8d88
	colFelt    = 0x2a3440
	colParquet = 0x9a6a40
	colCarpet  = 0x4b5563
	colMarbleL = 0xe9e5dd
	colMarbleD = 0x3a3a40
	colOak     = 0xc9a77c
	colSlate   = 0x3f474a
)

// ---- the walls -------------------------------------------------------------------------------

// wallFinish is a wall's colour at u units along it and y up (pl: the
// plaster there, see wallWith), in finish fin.
func wallFinish(fin int, base, text uint32, u, y, foot, gk, pl float64, rm *r3room) uint32 {
	h := y - rm.fy // how high above the floor
	switch fin {
	case wallBrick:
		return brickAt(base, u, h, foot, gk, pl)
	case wallPanels:
		return panelsAt(base, u, h, foot, gk, pl)
	case wallConcrete:
		return concreteWallAt(base, u, h, foot, gk, pl)
	case wallTiles:
		return tilesAt(base, u, h, foot, gk, pl)
	case wallTrellis:
		return paperWall(base, text, u, y, foot, gk, pl, rm, true) // the paper's rails and lower part, diamonds above
	case wallFelt:
		return feltAt(base, u, h, foot, gk, pl)
	}
	return wallWith(base, text, u, y, foot, gk, pl, rm)
}

// brickAt: bricks 8 units long and 3 high, each row half a brick along
// from the one under it, each brick a shade of its own, in pale mortar.
func brickAt(base uint32, u, h, foot, gk, pl float64) uint32 {
	const bw, bh, mortar = 8.0, 3.0, 0.22
	row := ffloor(h / bh)
	uu := u + 0.5*bw*modF(row, 2)
	col := ffloor(uu / bw)
	fu, fh := uu-col*bw, h-row*bh
	d := gk * (3*pl + 7*shows(bh, foot)*cellRand(col+7, row) + 4*shows(0.7, foot)*noise(u/0.7+5.1, h/0.7+2.3))
	c := shade32(mix32(base, colBrick, 0.6), d)
	mx := seam(min(fu, bw-fu), mortar, foot)
	my := seam(min(fh, bh-fh), mortar, foot)
	return mix32(c, mix32(base, colMortar, 0.45), mx+my-mx*my)
}

// panelsAt: upright boards 4.5 units wide with a dark groove between,
// each its own shade, a grain running up them, and a darker skirting
// board.
func panelsAt(base uint32, u, h, foot, gk, pl float64) uint32 {
	const bw, groove = 4.5, 0.13
	b := ffloor(u / bw)
	fu := u - b*bw
	g := noise(u/0.45+b*13.1, h/9) // the grain: thin across, long up
	d := gk * (2*pl + 6*shows(bw, foot)*cellRand(b, 11) + 7*shows(0.45, foot)*g + 4*shows(1.6, foot)*noise(u/1.6, h/20+3.7))
	c := shade32(mix32(base, colWood, 0.6), d)
	c = mix32(c, 0, 0.45*seam(min(fu, bw-fu), groove, foot))
	if h < 3.3+foot {
		c = mix32(c, mix32(base, colWood, 0.85), cover(h, -1, 3, foot))
		c = mix32(c, 0, 0.25*cover(h, 3, 3.3, foot))
	}
	return c
}

// concreteWallAt: smooth painted concrete, cast in panels 24 units wide
// and 12 high with faint seams, a tie hole every 12 across and 6 up.
func concreteWallAt(base uint32, u, h, foot, gk, pl float64) uint32 {
	d := gk * (6*pl + 5*noise(u/31+3.3, h/31+8.1))
	c := shade32(mix32(base, colConcr, 0.35), d)
	su := min(modF(u, 24), 24-modF(u, 24))
	sh := min(modF(h, 12), 12-modF(h, 12))
	c = mix32(c, 0, 0.12*max(seam(su, 0.08, foot), seam(sh, 0.08, foot)))
	du, dh := modF(u, 12)-6, modF(h, 6)-3
	r := math.Sqrt(du*du + dh*dh)
	return mix32(c, 0, 0.3*shows(0.6, foot)*seam(r, 0.3, foot))
}

// tilesAt: white tiles 4 units wide and 2 high, each row half a tile
// along, with grey grout and edges a little darker where they round off.
func tilesAt(base uint32, u, h, foot, gk, pl float64) uint32 {
	const tw, th, grout = 4.0, 2.0, 0.08
	row := ffloor(h / th)
	uu := u + 0.5*tw*modF(row, 2)
	col := ffloor(uu / tw)
	fu, fh := uu-col*tw, h-row*th
	eu, eh := min(fu, tw-fu), min(fh, th-fh)
	d := gk * (1.5*pl + 3*shows(th, foot)*cellRand(col+19, row+5))
	// The glaze rounds off at the edges: lighter along the top, darker
	// along the bottom and the sides, as the light from above catches it.
	vis := shows(0.6, foot)
	d += vis * (10*seam(th-fh, 0.25, foot) - 9*seam(fh, 0.25, foot) - 5*seam(eu, 0.25, foot))
	c := shade32(mix32(base, colTile, 0.7), d)
	gu, gh := seam(eu, grout, foot), seam(eh, grout, foot)
	return mix32(c, mix32(base, colGrout, 0.6), gu+gh-gu*gh)
}

// trellisPaper is the trellis paper's pattern over col, the paper at u
// along and h above the floor (see paperWall): a lattice of diamonds 5
// units wide and 7 high, a dot in each.
func trellisPaper(col, base, text uint32, u, h, foot float64) uint32 {
	const cw, ch = 5.0, 7.0
	fu, fh := (fracF(u/cw)-0.5)*cw, (fracF(h/ch)-0.5)*ch
	q := absF(fu)/(cw/2) + absF(fh)/(ch/2) // 1 on the diamond's edge
	vis := shows(1.2, foot)
	col = mix32(col, mix32(base, text, 0.15), vis*seam((1-q)*1.9, 0.11, foot))
	return mix32(col, mix32(base, text, 0.12), vis*seam(math.Sqrt(fu*fu+fh*fh), 0.35, foot))
}

// feltAt: dark felt panels 10 units square, a shadowed gap between them,
// a soft fuzz, and their edges a little darker where they round off.
func feltAt(base uint32, u, h, foot, gk, pl float64) uint32 {
	const pw, gap = 10.0, 0.25
	bu, bh := ffloor(u/pw), ffloor(h/pw)
	eu, eh := min(u-bu*pw, pw-(u-bu*pw)), min(h-bh*pw, pw-(h-bh*pw))
	d := gk * (2*pl + 3*shows(pw, foot)*cellRand(bu+31, bh+3) + 5*shows(0.5, foot)*noise(u/0.5+1.7, h/0.5+9.2) +
		3*shows(2, foot)*noise(u/2+4.4, h/2))
	c := shade32(mix32(base, colFelt, 0.7), d)
	e := min(eu, eh)
	c = mix32(c, 0, 0.2*shows(1.2, foot)*(1-clamp01(e/1.2))) // softly darker toward the edge
	return mix32(c, 0, 0.5*seam(e, gap, foot))
}

// ---- the floor -------------------------------------------------------------------------------

// floorFinish is the floor's colour at (x, z) in finish fin: px units to
// a pixel seen square on (the planks' measure, see floorAt) and foot at
// the slant it is seen at.
func floorFinish(fin int, base uint32, x, z, px, foot float64) uint32 {
	switch fin {
	case floorParquet:
		return parquetAt(base, x, z, foot)
	case floorCarpet:
		return carpetAt(base, x, z, foot)
	case floorConcrete:
		return concreteFloorAt(base, x, z, foot)
	case floorMarble:
		return marbleAt(base, x, z, foot)
	case floorOak:
		return oakAt(base, x, z, foot)
	case floorSlate:
		return slateAt(base, x, z, foot)
	}
	return floorAt(base, x, z, px)
}

// parquetAt: wood blocks 2.5 units wide and four times as long, laid in a
// herringbone that points out from the wall, each block its own shade.
func parquetAt(base uint32, x, z, foot float64) uint32 {
	const w, n = 2.5, 4.0
	const k = 0.70710678 / w // turned an eighth of a turn, in blocks' widths
	a, b := (x+z)*k, (z-x)*k
	// The blocks lying one way are at mod(a − floor(b), 2n) < n; the
	// others fill the gaps between them (worked out as a staircase of
	// pairs).
	var along, across, id1, id2, tone float64
	row := ffloor(b)
	if modF(a-row, 2*n) < n {
		id1, id2 = row, ffloor((a-row)/(2*n))
		along, across = a-row-2*n*id2, b-row
		tone = 3
	} else {
		cl := ffloor(a)
		id1, id2 = cl, ffloor((b-cl-1)/(2*n))
		along, across = b-cl-1-2*n*id2, a-cl
		tone = -3
	}
	ea, ec := min(along, n-along)*w, min(across, 1-across)*w // to the block's ends and sides, in units
	vis := shows(w, foot)
	g := noise(along*w/9+id1*3.1, across*w/0.4+id2*7.3) // the grain, along the block
	d := vis*(tone+8*cellRand(id1+41, id2+13)) + 6*shows(0.4, foot)*g
	c := shade32(mix32(base, colParquet, 0.5), d)
	return mix32(c, 0, 0.3*seam(min(ea, ec), 0.1, foot))
}

// carpetAt: carpet tiles 12 units square, each laid a quarter turn from
// its neighbours so the pile catches the light a shade apart, a fine
// fuzz, and faint seams.
func carpetAt(base uint32, x, z, foot float64) uint32 {
	const tw = 12.0
	chk := 0.5 + shows(tw, foot)*(filteredChecker(x/tw, z/tw, foot/tw)-0.5) // 0 or 1, the pile's way; a half far off
	fuzz := 5*shows(0.4, foot)*noise(x/0.4+2.2, z/0.4+6.6) + 3*shows(1.5, foot)*noise(x/1.5+8.8, z/1.5)
	c := shade32(mix32(base, colCarpet, 0.6), 6*(chk-0.5)+fuzz)
	ex, ez := min(modF(x, tw), tw-modF(x, tw)), min(modF(z, tw), tw-modF(z, tw))
	return mix32(c, 0, 0.12*seam(min(ex, ez), 0.06, foot))
}

// concreteFloorAt: polished concrete, cloudy in big soft patches, a fine
// speckle of stone in it, and a saw cut every 40 units each way.
func concreteFloorAt(base uint32, x, z, foot float64) uint32 {
	cloud := 7*noise(x/26+1.3, z/26+4.9) + 4*shows(8, foot)*noise(x/8+6.1, z/8+2.2)
	speck := 6 * shows(0.5, foot) * noise(x/0.5+3.9, z/0.5+7.4)
	c := shade32(mix32(base, colConcr, 0.45), cloud+speck)
	ex, ez := min(modF(x, 40), 40-modF(x, 40)), min(modF(z, 40), 40-modF(z, 40))
	return mix32(c, 0, 0.3*seam(min(ex, ez), 0.1, foot))
}

// marbleAt: marble squares 8 units across, light and dark by turns, with
// soft veins and a thin grout.
func marbleAt(base uint32, x, z, foot float64) uint32 {
	const tw = 8.0
	chk := 0.5 + shows(tw, foot)*(filteredChecker(x/tw, z/tw, foot/tw)-0.5) // squares under 3 pixels fade to grey: no shimmer
	c := mix32(mix32(base, colMarbleL, 0.6), mix32(base, colMarbleD, 0.6), chk)
	// The veins: a wave across the stone, bent by soft noise, each
	// square's its own; a vein is where the wave crosses 0.
	ti, tj := ffloor(x/tw), ffloor(z/tw)
	ph := (x+0.6*z)/3.2 + 3*noiseSmooth(x/9+11.3, z/9+4.1) + 1.2*noiseSmooth(x/3+2.7, z/3+9.9) + 9*cellRand(ti+71, tj+23)
	vein := shows(0.6, foot) * (1 - clamp01(absF(math.Sin(ph))/0.12))
	c = shade32(c, 4*noise(x/3+5.5, z/3+1.1)+vein*(28*chk-14)) // darker on the light squares, lighter on the dark
	ex, ez := min(modF(x, tw), tw-modF(x, tw)), min(modF(z, tw), tw-modF(z, tw))
	return mix32(c, mix32(base, colMortar, 0.4), seam(min(ex, ez), 0.06, foot))
}

// oakAt: light oak boards 9 units wide and 60 long, the ends staggered,
// each board its own shade, with a grain along it.
func oakAt(base uint32, x, z, foot float64) uint32 {
	const bw, bl = 9.0, 60.0
	p := ffloor(x / bw)
	along := z + modF(p*23, bl)
	q := ffloor(along / bl)
	fx, fz := x-p*bw, along-q*bl
	g := 6*shows(0.5, foot)*noise(x/0.5+p*7.7, z/12) + 4*shows(1.5, foot)*noise(x/1.5+3.1, z/25+p)
	c := shade32(mix32(base, colOak, 0.55), 6*shows(bw, foot)*cellRand(p+53, q)+g)
	sx, sz := seam(min(fx, bw-fx), 0.1, foot), seam(min(fz, bl-fz), 0.1, foot)
	return mix32(c, 0, 0.22*(sx+sz-sx*sz))
}

// slateAt: slate flags 10 units square, each row half a flag along, each
// its own shade and a little green or brown, with a riven surface and
// dark grout.
func slateAt(base uint32, x, z, foot float64) uint32 {
	const tw = 10.0
	row := ffloor(z / tw)
	xx := x + 0.5*tw*modF(row, 2)
	col := ffloor(xx / tw)
	vis := shows(tw, foot)
	c := mix32(base, colSlate, 0.75)
	c = mix32(c, 0x4a5a48, 0.25*vis*(cellRand(col+61, row+17)+1)/2)
	riven := 5*shows(3, foot)*noise(x/3+7.2, z/3+0.6) + 4*shows(1, foot)*noise(x+3.3, z+5.5)
	c = shade32(c, 9*vis*cellRand(col+3, row+29)+riven)
	ex, ez := min(xx-col*tw, tw-(xx-col*tw)), min(z-row*tw, tw-(z-row*tw))
	gx, gz := seam(ex, 0.15, foot), seam(ez, 0.15, foot)
	return mix32(c, mix32(base, 0x1c1f22, 0.8), gx+gz-gx*gz)
}

// filteredChecker is a checkerboard of squares a unit across at (x, y),
// 0 on one colour and 1 on the other, blurred over a pixel w units wide:
// far off, where the squares are smaller than a pixel, it is a half.
func filteredChecker(x, y, w float64) float64 {
	w = max(w, 1e-4)
	ix := 2 * (absF(fracF((x-0.5*w)*0.5)-0.5) - absF(fracF((x+0.5*w)*0.5)-0.5)) / w
	iy := 2 * (absF(fracF((y-0.5*w)*0.5)-0.5) - absF(fracF((y+0.5*w)*0.5)-0.5)) / w
	return 0.5 - 0.5*ix*iy
}

func absF(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}
