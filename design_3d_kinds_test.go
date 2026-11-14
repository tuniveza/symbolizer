//go:build linux && cgo

package main

import (
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

// kindsScene is a 200×64 Design mode with one of every kind of structure,
// each with words on it, its containers holding things.
func kindsScene(t testing.TB) (*Model, map[string]*dOct) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	m, _ := NewModel("", 0)
	m.Update(tea.WindowSizeMsg{Width: 200, Height: 64})
	m.setMode(designMode)
	d := m.ds()
	d.camX, d.camY, d.zoom = 0, 0, designZoom100
	all := map[string]*dOct{}
	add := func(name, kind string, x, y, ow, oh int, title, body string) *dOct {
		o := d.newKind(kind, 0, 0)
		o.X, o.Y, o.Header, o.Body = x, y, title, body
		if ow > 0 {
			o.W, o.H = ow, oh
		}
		all[name] = o
		return o
	}
	add("page", kPage, 2, 1, 26, 11, "Notes", "Words to read\nin the room.")
	add("note", kNote, 30, 1, 24, 9, "Keys", "Front door\nand garage")
	add("slide", kSlide, 56, 1, 26, 11, "Slide", "Big words")
	add("book", kBook, 84, 1, 22, 11, "Atlas", "Maps of the world")
	add("drawing", kDrawing, 108, 1, 22, 9, "Plan", " /\\\n/  \\\n|[]|")
	add("shape", kOct, 132, 1, 24, 9, "Idea", "Round it off")
	cab := add("cabinet", kCabinet, 2, 15, 30, 0, "Shelf", "")
	cab.Items = []*dOct{{ID: 901, Kind: kNote, Header: "Jam", Color: "#e5c07b"}}
	dr := add("drawer", kDrawer, 36, 15, 30, 0, "Tools", "")
	dr.Drawers = 3
	dr.Items = []*dOct{{ID: 903, Kind: kNote, Header: "Saw", Color: "#98c379"}}
	rk := add("rack", kRack, 70, 15, 30, 0, "Records", "")
	rk.Items = []*dOct{{ID: 904, Kind: kPage, Header: "Blues", Color: "#61afef"}, {ID: 905, Kind: kPage, Header: "Jazz", Color: "#c678dd"}}
	rk.H = rackLines(len(rk.Items))
	bd := add("board", kBoard, 104, 15, 0, 0, "Pins", "")
	bd.Items = []*dOct{{ID: 906, Kind: kNote, Header: "Milk", Color: "#e5c07b"}, {ID: 907, Kind: kNote, Header: "Call Bob", Color: "#98c379"}}
	bx := add("box", kBox, 2, 33, 34, 14, "Card box", "")
	bx.DrawerOpen = true
	bx.Items = []*dOct{
		{ID: 908, Kind: kNote, Header: "Stamps", Color: "#e5c07b", X: 2, Y: 1, W: 26, H: 9},
		{ID: 909, Kind: kPage, Header: "Letters", Color: "#c678dd", X: 8, Y: 3, W: 26, H: 9, Slot: 1},
	}
	lab := add("label", kPage, 40, 33, 26, 10, "Recipes", "Bread\nSoup")
	lab.LabelStyle, lab.LabelTitle = 1, "Kitchen"
	pl := add("plate", kNote, 70, 33, 24, 9, "Garden", "Seeds")
	pl.Plate = "Outside"
	ly := add("layer", kPage, 98, 33, 26, 10, "Upstairs", "On a layer")
	ly.Layer = 2
	oc := add("cabinet-open", kCabinet, 130, 33, 30, 0, "Pantry", "")
	oc.Items = []*dOct{{ID: 910, Kind: kNote, Header: "Rice", Color: "#e5c07b"}, {ID: 911, Kind: kPage, Header: "Tea", Color: "#61afef"}}
	oc.DrawerOpen = true
	pd := add("drawer-pulled", kDrawer, 164, 1, 30, 0, "Desk", "")
	pd.Drawers = 2
	pd.Items = []*dOct{{ID: 912, Kind: kNote, Header: "Pens", Color: "#98c379"}}
	pd.DrawerOpen, pd.Pulled = true, []bool{true, false}
	tb := add("board-turned", kBoard, 164, 33, 34, 18, "Turned", "")
	tb.Items = []*dOct{
		{ID: 913, Kind: kNote, Header: "Tilted", Body: "Read me", Color: "#e5c07b", X: 1, Y: 1, Tf: &dTf{Rot: 20}},
		{ID: 914, Kind: kNote, Header: "Big", Color: "#98c379", X: 12, Y: 6, Tf: &dTf{SX: 1.3, SY: 1.3}},
	}
	d.sel = -1
	return m, all
}

// kindNames3D are kindsScene's structures, in order.
var kindNames3D = []string{"page", "note", "slide", "book", "drawing", "shape", "cabinet", "drawer", "rack", "board", "box",
	"label", "plate", "layer", "cabinet-open", "drawer-pulled", "board-turned"}

// kindCams are the cameras each kind is seen from: turned, tilted, how
// much of the screen's width it fills, and the lens (= and − in 3D).
var kindCams = []struct{ yaw, pitch, fill, lens float64 }{
	{0, 0, 0.5, 1}, {35, 10, 0.5, 1}, {50, -15, 0.7, 2.5}, {0, 0, 0.15, 0.4}, {20, 5, 0.3, 6}, {0, 0, 0.08, 1}, {-60, 20, 0.4, 1},
}

// aimFree puts the first-person camera in front of structure o's front,
// turned yaw degrees and tilted pitch, with lens lens, as far off as makes
// it fill that much of the screen's width.
func aimFree(m *Model, o *dOct, yaw, pitch, fill, lens float64) {
	d := m.ds()
	r := &d.room
	if !r.free {
		press(m, "ctrl+f")
		boardRows(m)
	}
	x, y, w, h := d.rect(o)
	hw, hh := float64(m.width)/2, float64(m.boardH())
	z := float64(o.Layer)*layerStep + 1
	at := r3vec{float64(x) + float64(w)/2 - hw, hh - 2*(float64(y)+float64(h)/2), z}
	ly, lp := yaw*math.Pi/180, pitch*math.Pi/180
	fwd := r3vec{math.Sin(ly) * math.Cos(lp), -math.Sin(lp), -math.Cos(ly) * math.Cos(lp)}
	dist := r.ff * lens * float64(w) / (fill * float64(m.width))
	r.pos = at.sub(fwd.mul(dist))
	r.look = [2]float64{yaw, pitch}
	r.lens = lens
	r.freeK = 1
}

// savePNG writes picture rgb (w×h) to dir/name.png.
func savePNG(t testing.TB, dir, name string, rgb []byte, w, h int) {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for i := 0; i < w*h; i++ {
		img.Set(i%w, i/w, color.RGBA{rgb[3*i], rgb[3*i+1], rgb[3*i+2], 255})
	}
	f, err := os.Create(filepath.Join(dir, name+".png"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	png.Encode(f, img)
}

// SY_3D_PNG=dir go test -run TestDesign3DKindsPictures writes every kind
// of structure seen at a few angles, distances and lenses, on the card
// and on the processor, to look at.
func TestDesign3DKindsPictures(t *testing.T) {
	dir := os.Getenv("SY_3D_PNG")
	if dir == "" {
		t.Skip("set SY_3D_PNG to a folder")
	}
	m, all := kindsScene(t)
	flat := ansi.Strip(strings.Join(boardRows(m), "\n"))
	os.WriteFile(filepath.Join(dir, "flat.txt"), []byte(flat), 0o644)
	withPixels(t, m, 10, 20)
	for _, name := range kindNames3D {
		o := all[name]
		for _, c := range kindCams {
			aimFree(m, o, c.yaw, c.pitch, c.fill, c.lens)
			for _, gpu := range []bool{false, true} {
				if gpu && !gpuOK() {
					continue
				}
				pic, w, h, _ := gpuPicture(t, m, gpu)
				way := "cpu"
				if gpu {
					way = "gpu"
				}
				savePNG(t, dir, fmt.Sprintf("%s_y%g_p%g_f%g_l%g_%s", name, c.yaw, c.pitch, c.fill, c.lens, way), pic, w, h)
			}
		}
	}
}

// gpuOK is whether the graphics card can draw the room here.
func gpuOK() bool {
	was := room3DGPU
	room3DGPU = true
	ok := gpuReady()
	room3DGPU = was
	return ok
}

// kindWords are words each structure of kindsScene shows, as the flat
// board has them: titles, text, labels, drawer numbers, rack rows, pins,
// things on shelves and in boxes, name plates, and their icons.
var kindWords = map[string][]string{
	"page": {"Notes", "Words to read", "in the room."}, "note": {"Keys", "Front door", "and garage"},
	"slide": {"Slide", "Big words"}, "book": {"Atlas", "Maps of the world"}, "drawing": {"Plan", "/  \\", "|[]|"},
	"shape": {"Idea", "Round it off"}, "cabinet": {"≣ Shelf"}, "drawer": {"⊔ Tools", "1", "2", "3"},
	"rack": {"Records", "≡ Blues", "≡ Jazz", "✕"}, "board": {"⊞ Pins", "✎ Call Bob"},
	"box":   {"⊡ Card box", "Stamps", "✎", "Letters", "top 1", "middle 1", "bottom 0"},
	"label": {"★ Kitchen", "Recipes", "Bread", "Soup"}, "plate": {"Garden", "Seeds", "▪ Outside"},
	"layer": {"Upstairs", "On a layer"}, "cabinet-open": {"≣ Pantry", "≡ Tea", "✎ Rice", "≡ Pages", "◫ Books", "⇣ diary", "⇣ files"},
	"drawer-pulled": {"⊔ Desk", "1", "Pens", "2"}, "board-turned": {"⊞ Turned", "✎ Big"},
}

// paintedText is the painted letters of cells x0 ≤ x < x1, y0 ≤ y < y1 of
// the room's picture, row by row (a space where no letter is painted), and
// how many letters there are not painted (not in any font).
func paintedText(r *room3D, x0, y0, x1, y1 int) (string, int) {
	rm := &r.rm
	var sb strings.Builder
	missing := 0
	for y := y0; y < y1; y++ {
		for x := x0; x < x1; x++ {
			if x < rm.tx0 || y < rm.ty0 || x >= rm.tx0+rm.tw || y >= rm.ty0+rm.th {
				continue
			}
			t := &r.tex[(y-rm.ty0)*rm.tw+x-rm.tx0]
			switch {
			case t.gx != 0:
			case t.gl != nil && t.glyph != "":
				sb.WriteString(t.glyph)
			case t.glyph != "" && t.glyph != " ":
				missing++
				sb.WriteString("?")
			default:
				sb.WriteByte(' ')
			}
		}
		sb.WriteByte('\n')
	}
	return sb.String(), missing
}

// structText is the painted letters of structure o (its kept drawing, on
// a layer) and of its turned pins' cards.
func structText(r *room3D, d *designState, o *dOct) (string, int) {
	x, y, w, h := d.rect(o)
	for bi, b := range r.blocks {
		if b.o == o {
			y += b.shift
			s, miss := paintedText(r, x, y, x+w, y+h)
			for _, pc := range r.pinCards {
				if pc.blk == bi {
					c, m := paintedText(r, pc.tx, pc.ty, pc.tx+pc.p.gw, pc.ty+pc.p.gh)
					s, miss = s+c, miss+m
				}
			}
			return s, miss
		}
	}
	return "", 0
}

// Every kind of structure's words are painted on it in 3D, letter by
// letter from the fonts (the terminal's, else a font that has the letter:
// icons too), the same words as on the flat board, from every camera and
// lens; a turned pin's words are whole on its own card.
func TestDesign3DKindsWordsPainted(t *testing.T) {
	m, all := kindsScene(t)
	withPixels(t, m, 6, 12)
	d := m.ds()
	r := &d.room
	for ci, c := range kindCams {
		for _, name := range kindNames3D {
			o := all[name]
			aimFree(m, o, c.yaw, c.pitch, c.fill, c.lens)
			boardRows(m)
			if !r.paint {
				t.Fatal("not the painted picture")
			}
			text, miss := structText(r, d, o)
			if miss > 0 && ci == 0 {
				t.Errorf("%s: %d letters not painted:\n%s", name, miss, text)
			}
			for _, w := range kindWords[name] {
				if !strings.Contains(text, w) {
					t.Errorf("%s, camera %+v: %q not painted:\n%s", name, c, w, text)
				}
			}
		}
	}
	// The turned pin: its whole words on its card, none on the board.
	press(m, "esc")
	aimFree(m, all["board-turned"], 0, 0, 0.5, 1)
	boardRows(m)
	if len(r.pinCards) != 1 {
		t.Fatalf("%d turned pins' cards, want 1", len(r.pinCards))
	}
	pc := r.pinCards[0]
	card, _ := paintedText(r, pc.tx, pc.ty, pc.tx+pc.p.gw, pc.ty+pc.p.gh)
	for _, w := range []string{"✎ Tilted", "Read me", "●"} {
		if !strings.Contains(card, w) {
			t.Errorf("the turned pin's card has no %q:\n%s", w, card)
		}
	}
	x, y, w, h := d.rect(all["board-turned"])
	if board, _ := paintedText(r, x, y, x+w, y+h); strings.Contains(board, "Tilted") || strings.Contains(board, "Read") {
		t.Errorf("the turned pin's words are on the board too (doubled):\n%s", board)
	}
}

// frontStats is, over the pixels of the last drawing that show structure
// own's front (or its cards), how many there are and their colours'
// spread (the words' contrast), from the colours before the light.
func frontStats(r *room3D, own int) (n int, spread float64) {
	sum, sq := 0.0, 0.0
	for i, fi := range r.face {
		if fi < 0 || r.faces[fi].own != own || r.faces[fi].mat != matFront {
			continue
		}
		cr, cg, cb := chans(r.base[i])
		l := 0.3*cr + 0.59*cg + 0.11*cb
		sum, sq, n = sum+l, sq+l*l, n+1
	}
	if n == 0 {
		return 0, 0
	}
	m := sum / float64(n)
	return n, math.Sqrt(math.Max(0, sq/float64(n)-m*m))
}

// frontDiff is the mean difference (0 to 255) between two drawings'
// colours over the pixels that show the same face of structure own's front
// in both.
func frontDiff(r *room3D, own int, faceA []int32, baseA []uint32) float64 {
	sum, n := 0.0, 0
	for i, fi := range r.face {
		if fi < 0 || faceA[i] != fi || r.faces[fi].own != own || r.faces[fi].mat != matFront {
			continue // not its front, or the edge of one of its faces (the faces' edges are not what is checked)
		}
		ar, ag, ab := chans(baseA[i])
		br, bg, bb := chans(r.base[i])
		sum += math.Max(math.Abs(ar-br), math.Max(math.Abs(ag-bg), math.Abs(ab-bb)))
		n++
	}
	return sum / float64(max(n, 1))
}

// steadyRun brings the camera (with lens lens) to structure o from far to
// near in small steps
// and gives the biggest jump in its words' contrast from one step to the
// next (as a ratio, 0: none), and the biggest mean change a hair's zoom
// makes (0 to 255).
func steadyRun(t *testing.T, m *Model, o *dOct, yaw, pitch, lens float64) (jump, hair float64) {
	r := &m.ds().room
	draw := func(fill float64) (int, float64) {
		aimFree(m, o, yaw, pitch, fill, lens)
		gpuPicture(t, m, false)
		return frontStats(r, o.ID)
	}
	last := -1.0
	step := 0
	for fill := 0.015; fill < 0.5; fill *= 1.12 {
		n, sp := draw(fill)
		if n < 400 {
			continue // too few pixels to say: its edges are most of them
		}
		step++
		if last > 0 {
			jump = math.Max(jump, math.Abs(math.Log(sp/last)))
		}

		last = sp
		if step%3 == 1 {
			face, base := append([]int32(nil), r.face...), append([]uint32(nil), r.base...)
			draw(fill * 1.002)
			hair = math.Max(hair, frontDiff(r, o.ID, face, base))
		}
	}
	return jump, hair
}

// Every kind's words stay steady as the camera comes near from far off,
// at a wide lens, the normal one and zoomed in: no step changes how much
// they stand out suddenly (a letter popping into a tint, or a picture
// into the next smaller one), and a hair's move changes the picture only
// a hair (no shimmer). On the processor; TestDesign3DKindsGPU checks the
// card draws the same.
func TestDesign3DKindsSteady(t *testing.T) {
	m, all := kindsScene(t)
	withPixels(t, m, 6, 12)
	for _, c := range []struct {
		name             string
		yaw, pitch, lens float64
	}{
		{"page", 25, 8, 1}, {"rack", 25, 8, 1}, {"drawer", 25, 8, 1}, {"cabinet-open", 25, 8, 1}, {"box", 25, 8, 1},
		{"board-turned", 25, 8, 1}, {"layer", 25, 8, 1}, {"rack", -40, -10, 0.4}, {"cabinet-open", 10, 5, 6}, {"label", -30, 12, 2.5},
	} {
		jump, hair := steadyRun(t, m, all[c.name], c.yaw, c.pitch, c.lens)
		t.Logf("%s %+v: biggest jump %.3f, hair %.2f", c.name, c, jump, hair)
		if jump > 0.2 || hair > 6 {
			t.Errorf("%s, camera %+v: its words jump %.3f (want ≤ 0.2) or shimmer %.2f (want ≤ 6)", c.name, c, jump, hair)
		}
	}
}

// The card draws every kind as the processor does, at every lens: the
// words (painted letters, the blended smaller pictures, the turned pins'
// cards) the same, pixel for pixel but for a little rounding.
func TestDesign3DKindsGPU(t *testing.T) {
	if !gpuOK() {
		t.Skip("no graphics card: " + gpuRoom.why)
	}
	m, all := kindsScene(t)
	withPixels(t, m, 6, 12)
	for _, name := range kindNames3D {
		for _, c := range []struct{ yaw, pitch, fill, lens float64 }{{35, 10, 0.5, 1}, {0, 0, 0.04, 0.4}, {20, 5, 0.3, 6}, {-60, 20, 0.12, 2.5}} {
			aimFree(m, all[name], c.yaw, c.pitch, c.fill, c.lens)
			cpu, _, _, _ := gpuPicture(t, m, false)
			gpu, _, _, _ := gpuPicture(t, m, true)
			if len(cpu) != len(gpu) || len(cpu) == 0 {
				t.Fatalf("%s %+v: sizes cpu %d gpu %d", name, c, len(cpu), len(gpu))
			}
			far, sum := 0, 0
			for i := 0; i < len(cpu); i += 3 {
				dd := 0
				for k := range 3 {
					dd = max(dd, abs(int(cpu[i+k])-int(gpu[i+k])))
				}
				sum += dd
				if dd > 40 {
					far++
				}
			}
			n := len(cpu) / 3
			if mean := float64(sum) / float64(n); mean > 2 || float64(far) > 0.005*float64(n) {
				t.Errorf("%s %+v: the card's picture differs: mean %.2f, %d of %d pixels far apart", name, c, mean, far, n)
			}
		}
	}
}

// A letter seen smaller and smaller changes smoothly: no jump from one of
// its copies to the next, nor from the smallest to its mean tint.
func TestGlyphCoverSmooth(t *testing.T) {
	for _, s := range []string{"H", "a", "≡", "▏"} {
		g := glyphOf(s, false, false)
		if g == nil {
			t.Fatalf("no %q", s)
		}
		for _, p := range [][2]float64{{0.5, 0.5}, {0.3, 0.4}, {0.15, 0.7}, {0.62, 0.33}, {0.06, 0.5}} {
			last := g.cover(p[0], p[1], 0.005, 0.0025)
			for du := 0.005; du < 8; du *= 1.02 {
				c := g.cover(p[0], p[1], du, du/2)
				if math.Abs(c-last) > 0.06 {
					t.Fatalf("%q at %v: cover jumps %.3f → %.3f as it shrinks to %.4f of a cell a pixel", s, p, last, c, du)
				}
				last = c
			}
			if last != g.mean {
				t.Errorf("%q far off: %.3f, want its mean %.3f", s, last, g.mean)
			}
		}
	}
}

// The back wall's picture seen smaller and smaller changes smoothly too
// (wallAt blends the smaller pictures, it does not jump between them).
func TestWallAtSmooth(t *testing.T) {
	m, _ := kindsScene(t)
	withPixels(t, m, 6, 12)
	press(m, "ctrl+f")
	boardRows(m)
	r := &m.ds().room
	for _, p := range [][2]float64{{60.3, 10.7}, {130.5, 44.2}, {90.9, 70.1}, {210.2, 36.6}} {
		last := r.wallAt(p[0], p[1], 0.3)
		for s := 0.3; s < 60; s *= 1.02 {
			c := r.wallAt(p[0], p[1], s)
			lr, lg, lb := chans(last)
			cr, cg, cb := chans(c)
			if d := math.Max(math.Abs(lr-cr), math.Max(math.Abs(lg-cg), math.Abs(lb-cb))); d > 12 {
				t.Fatalf("at %v: the wall jumps by %.0f at %.2f of its pixels a screen pixel", p, d, s)
			}
			last = c
		}
	}
}

// A turned pin's card stands for its board: the cross on it chooses the
// board's cell behind it, as the flat board's pin there.
func TestDesign3DPinCardPicks(t *testing.T) {
	m, all := kindsScene(t)
	withPixels(t, m, 6, 12)
	b := all["board-turned"]
	d := m.ds()
	r := &d.room
	aimFree(m, b, 0, 0, 0.5, 1)
	boardRows(m)
	if len(r.pinCards) != 1 {
		t.Fatalf("%d cards", len(r.pinCards))
	}
	pc := r.pinCards[0]
	// The card's middle on the screen, from the board's corner.
	x, y, _, _ := d.rect(b)
	sx, sy := pc.p.toScreen(float64(pc.p.gw)/2, float64(pc.p.gh)/2)
	cx, cy := x+int(sx), y+int(sy)
	// Aim the camera straight at it.
	hw, hh := float64(m.width)/2, float64(m.boardH())
	r.pos = r3vec{float64(cx) + 0.5 - hw, hh - 2*float64(cy) - 1, r.pos.z}
	r.look = [2]float64{0, 0}
	boardRows(m)
	h := r.pickAim()
	if !h.ok || h.own != b.ID || !r.faces[h.face].aff {
		t.Fatalf("the cross on the card: own %d (want the board %d), card %v", h.own, b.ID, h.ok && r.faces[h.face].aff)
	}
	if h.sx != cx || h.sy != cy {
		t.Errorf("the cross on the card is on cell %d,%d, want %d,%d behind it", h.sx, h.sy, cx, cy)
	}
	if j := d.pinAt(b, h.sx-x, h.sy-y); j != pc.j {
		t.Errorf("the flat board's pin there is %d, want the card's %d", j, pc.j)
	}
}
