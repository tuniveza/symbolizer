package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"
)

func screenOf(m *Model) string { return ansi.Strip(m.View().Content) }

// writePNG makes a small picture file: a colour gradient.
func writePNG(t *testing.T, dir string) string {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, 40, 20))
	for y := range 20 {
		for x := range 40 {
			img.Set(x, y, color.NRGBA{uint8(x * 6), uint8(y * 12), 200, 255})
		}
	}
	p := filepath.Join(dir, "pic.png")
	f, err := os.Create(p)
	if err != nil {
		t.Fatal(err)
	}
	png.Encode(f, img)
	f.Close()
	return p
}

// runCmd runs a command and feeds what it sends back into the model.
func runCmd(m *Model, c tea.Cmd) {
	if c != nil {
		if msg := c(); msg != nil {
			m.Update(msg)
		}
	}
}

func TestDesignPagesAndNotes(t *testing.T) {
	m := designTest(t)
	d := m.ds()

	// P: a page. It takes text of any length and scrolls.
	press(m, "p")
	typeKeys(m, "Plan")
	press(m, "tab")
	for i := range 40 {
		typeKeys(m, "line ")
		typeKeys(m, string(rune('a'+i%26)))
		press(m, "enter")
	}
	press(m, "esc")
	pg := d.byID(d.sel)
	if pg == nil || pg.Kind != kPage || pg.Header != "Plan" || strings.Count(pg.Body, "\n") != 40 {
		t.Fatalf("page: %+v", pg)
	}
	s := screenOf(m)
	if !strings.Contains(s, "≡ Plan") || !strings.Contains(s, "41 lines") {
		t.Fatalf("page not drawn:\n%s", s)
	}
	press(m, "pgdown")
	if pg.Scroll == 0 {
		t.Fatal("PgDn did not scroll the page")
	}

	// S: a note. It holds one page and no more.
	press(m, "s")
	typeKeys(m, "Remember")
	press(m, "tab")
	for range noteRows + 5 {
		typeKeys(m, "x")
		press(m, "enter")
	}
	note := d.byID(d.sel)
	if note.Kind != kNote || !noteFits(note.Body) || strings.Count(note.Body, "\n") != noteRows-1 {
		t.Fatalf("note holds %d lines: %q", strings.Count(note.Body, "\n")+1, note.Body)
	}
	if !strings.Contains(m.msg, "full") {
		t.Fatalf("no 'full' message: %q", m.msg)
	}
	press(m, "esc")

	// Text-like only: control codes and colour codes are dropped.
	if got := textLike("a\x1b[31mred\x1b[0m\tb\x07 ✓ 🙂"); got != "ared    b ✓ 🙂" {
		t.Fatalf("textLike: %q", got)
	}

	// A picture dropped while a page is chosen does not go into the page
	// (pages hold text): it gets an octagon of its own.
	press(m, "esc")
	d.sel = pg.ID
	n := len(d.octs)
	dndRun(m, m.update(tea.PasteMsg{Content: writePNG(t, t.TempDir())}))
	if pg.Image != "" || len(d.octs) != n+1 || d.octs[n].Image == "" || d.octs[n].Kind != kOct {
		t.Fatalf("a picture dropped with a page chosen: page image %q, %d new structures", pg.Image, len(d.octs)-n)
	}
}

func TestDesignCabinetAndDrawers(t *testing.T) {
	m := designTest(t)
	d := m.ds()
	d.camX, d.camY = 0, 0

	cab := d.newKind(kCabinet, 0, 0)
	cab.X, cab.Y, cab.Header = 60, 1, "Archive"
	pg := d.newKind(kPage, 0, 0)
	pg.X, pg.Y, pg.W, pg.H, pg.Header = 2, 2, 20, 8, "Draft"
	d.sel, d.editing = -1, partNone
	s := screenOf(m)
	for _, w := range []string{"≣ Archive", "Pages", "Books", "Slides", "Notes", "Drawings"} {
		if !strings.Contains(s, w) {
			t.Fatalf("cabinet is missing %q:\n%s", w, s)
		}
	}

	// Drag the page onto the cabinet: it is filed in the Pages stack.
	px, py, _, _ := d.rect(pg)
	cx, cy, _, _ := d.rect(cab)
	dragTo(m, px+5, py+textTop+1, cx+8, cy+textTop+5, tea.MouseLeft)
	if d.byID(pg.ID) != nil || len(cab.Items) != 1 || cab.Items[0] != pg || cab.OpenStack != kPage {
		t.Fatalf("not filed: board %d, cabinet %d", len(d.octs), len(cab.Items))
	}
	if s := screenOf(m); !strings.Contains(s, "≡ Draft") {
		t.Fatalf("the open stack does not show the page:\n%s", s)
	}

	// Drag it out again by its row.
	rows := stackRows(cab, false)
	row := -1
	for i, r := range rows {
		if r.act == "item:0" {
			row = i
		}
	}
	dragTo(m, cx+5, cy+textTop+3+row, 20, 20, tea.MouseLeft)
	if d.byID(pg.ID) == nil || len(cab.Items) != 0 {
		t.Fatalf("not taken out: cabinet %d", len(cab.Items))
	}

	// Every structure has a drawer: its ▾ pulls it out.
	oc := d.newKind(kOct, 0, 0)
	oc.X, oc.Y = 2, 14
	d.editing = partNone
	x, y, w, h := d.rect(oc)
	hx, hy, _, ok := d.handleSpot(oc, x, y, w, h)
	if !ok {
		t.Fatal("no drawer handle")
	}
	click(m, hx+1, hy+textTop, tea.MouseLeft)
	if !oc.DrawerOpen || !strings.Contains(screenOf(m), "drawer") {
		t.Fatal("drawer did not open")
	}
	// Drop the page into the open drawer.
	pg.X, pg.Y = 40, 4
	px, py, _, _ = d.rect(pg)
	dx, dy, _, _ := d.drawerRect(oc)
	dragTo(m, px+3, py+textTop+1, dx+4, dy+textTop+2, tea.MouseLeft)
	if len(oc.Items) != 1 || oc.Items[0] != pg {
		t.Fatalf("page not in the drawer: %d items", len(oc.Items))
	}
	d.sel = oc.ID
	press(m, "d") // D closes the chosen structure's drawer
	if oc.DrawerOpen {
		t.Fatal("D did not close the drawer")
	}

	// It all comes back after a save.
	d.dirty = true
	d.save()
	d2 := loadDesign(d.loadedFrom)
	var got *dOct
	for _, o := range d2.octs {
		if o.ID == oc.ID {
			got = o
		}
	}
	if got == nil || len(got.Items) != 1 || got.Items[0].Header != "Draft" || got.Items[0].Kind != kPage {
		t.Fatalf("drawer not saved: %+v", got)
	}
	if d2.nextID <= pg.ID {
		t.Fatalf("nextID %d does not count things in drawers (%d)", d2.nextID, pg.ID)
	}

	// Removing a structure that holds things asks first.
	d.sel = oc.ID
	press(m, "delete")
	if d.byID(oc.ID) == nil || m.prompt != pConfirm {
		t.Fatal("removed without asking")
	}
	typeKeys(m, "y")
	if d.byID(oc.ID) != nil {
		t.Fatal("not removed after yes")
	}
}

func TestDesignFromDiary(t *testing.T) {
	m := designTest(t)
	d := m.ds()
	dir := filepath.Join(diaryDir(), "2026-11-05")
	os.MkdirAll(dir, 0o700)
	os.WriteFile(filepath.Join(dir, "01.txt"), []byte("Dear diary"), 0o600)
	cab := d.newKind(kCabinet, 0, 0)
	m.designRow(cab, "diary", 0, 0)
	m.designRow(cab, "diary", 0, 0) // a second time: not twice
	if len(cab.Items) != 1 || cab.Items[0].Body != "Dear diary" || cab.Items[0].Kind != kPage {
		t.Fatalf("diary pages: %+v", cab.Items)
	}
	m.designRow(cab, "files", 0, 0)
	if len(cab.Items) != 2 || cab.Items[1].Body != "text stays" {
		t.Fatalf("open files: %d items", len(cab.Items))
	}
}

func TestDesignPictureAndLayouts(t *testing.T) {
	m := designTest(t)
	d := m.ds()
	d.camX, d.camY = 0, 0
	o := d.newKind(kOct, 0, 0)
	o.X, o.Y, o.W, o.H = 4, 2, 30, 13
	d.editing = partNone

	// A dropped picture file (the terminal pastes its path) goes in.
	p := writePNG(t, t.TempDir())
	runCmd(m, m.update(tea.PasteMsg{Content: "'" + p + "'"}))
	if o.Image == "" || o.Layout != layoutAbove {
		t.Fatalf("picture: %q layout %d (%s)", o.Image, o.Layout, m.msg)
	}
	if _, err := os.Stat(filepath.Join(d.imagesDir(), o.Image)); err != nil {
		t.Fatal("the picture was not kept:", err)
	}
	if v := m.View().Content; !strings.Contains(v, "▀") || !strings.Contains(v, "38;2;") {
		t.Fatal("the picture is not drawn")
	}

	// The toggle on the top edge shifts the layout, its icon with it.
	x, y, w, h := d.rect(o)
	tx, ty, _ := layoutToggleSpot(x, y, w, h)
	click(m, tx+1, ty+textTop, tea.MouseLeft)
	if o.Layout != layoutLeft || !strings.Contains(screenOf(m), "[◧]") {
		t.Fatalf("toggle: layout %d", o.Layout)
	}
	for i := range len(octLayouts) {
		press(m, "l")
		if s := screenOf(m); !strings.Contains(s, "["+octLayouts[o.Layout].icon+"]") {
			t.Fatalf("layout %d (%d) icon missing", o.Layout, i)
		}
	}

	// Other formats go through ImageMagick, rsvg-convert or ffmpeg, when
	// the computer has them (tests run with no PATH; these tools only read).
	t.Setenv("PATH", "/usr/local/bin:/usr/bin:/bin")
	if _, err := exec.LookPath("magick"); err != nil {
		t.Log("ImageMagick not installed: other formats not tried")
		return
	}
	dir := t.TempDir()
	svg := filepath.Join(dir, "pic.svg")
	os.WriteFile(svg, []byte(`<svg xmlns="http://www.w3.org/2000/svg" width="40" height="20"><rect width="40" height="20" fill="red"/></svg>`), 0o600)
	files := []string{svg}
	for _, ext := range []string{"webp", "bmp", "tiff", "avif", "ico"} {
		out := filepath.Join(dir, "pic."+ext)
		if err := exec.Command("magick", p, out).Run(); err != nil {
			t.Logf("magick cannot write %s here: %v", ext, err)
			continue
		}
		files = append(files, out)
	}
	if _, err := exec.LookPath("ffmpeg"); err == nil {
		mp4 := filepath.Join(dir, "clip.mp4")
		if exec.Command("ffmpeg", "-v", "error", "-f", "lavfi", "-i", "testsrc=size=64x32:rate=1", "-t", "1", "-y", mp4).Run() == nil {
			files = append(files, mp4)
		}
	}
	for _, f := range files {
		data, _ := os.ReadFile(f)
		name, err := importPicture(f, data, t.TempDir())
		if err != nil || name == "" {
			t.Fatalf("%s: %v", filepath.Base(f), err)
		}
	}
	if len(files) < 6 {
		t.Fatalf("only %d formats tried", len(files))
	}
}

// A page's text wrapped paragraph by paragraph (kept between frames) is
// the same as the whole text wrapped at once, also after edits.
func TestDesignWrapCache(t *testing.T) {
	o := &dOct{}
	texts := []string{
		"",
		"one",
		"short\n\nwords and a much longer line that has to wrap round more than once here\n",
		"trailing spaces   \n   leading spaces\nAveryveryverylongwordwithnospacesthatmustbreak\n日本語のテキストと絵文字 🎉🎉 mixed in",
		"words and a much longer line that has to wrap round more than once here\nwords and a much longer line that has to wrap round more than once here",
		"words and a much longer line that has to wrap round more than once here!\nwords and a much longer line that has to wrap round more than once here",
	}
	for _, w := range []int{20, 7, 20} {
		for _, s := range texts {
			o.Body = s
			want := strings.Split(ansi.Wrap(s, w, ""), "\n")
			got := o.wrapped(w)
			if strings.Join(got, "\n") != strings.Join(want, "\n") {
				t.Fatalf("width %d, %q:\n got %q\nwant %q", w, s, got, want)
			}
		}
	}
}

// Pictures laid in as cells look exactly as they did drawn as styled text:
// every layout, small and big structures, at several zooms, half off the
// screen too.
func TestDesignPictureCellsMatchText(t *testing.T) {
	m := designTest(t)
	d := m.ds()
	dir := d.imagesDir()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	img := image.NewNRGBA(image.Rect(0, 0, 60, 40))
	for y := 0; y < 40; y++ {
		for x := 0; x < 60; x++ {
			img.Set(x, y, color.NRGBA{uint8(x * 4), uint8(y * 6), uint8(x * y), uint8(128 + x*2)})
		}
	}
	f, err := os.Create(filepath.Join(dir, "p.png"))
	if err != nil {
		t.Fatal(err)
	}
	png.Encode(f, img)
	f.Close()
	t.Cleanup(func() { picturesDirect, middlesDirect = true, true })

	for layout := range octLayouts {
		for _, size := range [][2]int{{12, 7}, {30, 16}} {
			for _, zoom := range []int{2, 5, 8} {
				for _, cam := range [][2]float64{{-3, -2}, {10, 5}, {-40, 0}} {
					d.octs, d.drawn = nil, nil
					o := d.newKind(kOct, 0, 0)
					o.X, o.Y, o.W, o.H, o.Image, o.Layout, o.Body = 0, 0, size[0], size[1], "p.png", layout, "Some words here"
					d.sel, d.editing, d.zoom = -1, partNone, zoom // chosen, a picture has a ✕ (TestDesignDelete)
					d.camX, d.camY = cam[0], cam[1]
					picturesDirect, middlesDirect = false, false
					want := m.designCanvas().Render()
					picturesDirect, middlesDirect = true, true
					d.drawn = nil
					got := m.designCanvas().Render()
					if got != want {
						gl, wl := strings.Split(got, "\n"), strings.Split(want, "\n")
						for i := range wl {
							if i < len(gl) && gl[i] != wl[i] {
								t.Fatalf("layout %d size %v zoom %d cam %v: row %d differs\n got %q\nwant %q\n got %q\nwant %q", layout, size, zoom, cam, i,
									ansi.Strip(gl[i]), ansi.Strip(wl[i]), gl[i], wl[i])
							}
						}
					}
				}
			}
		}
	}
}

// The empty board laid straight onto the canvas looks exactly as its styled
// text did: dots, the centre and links, at several zooms and places.
func TestDesignBoardPaintMatchesText(t *testing.T) {
	m := designTest(t)
	d := m.ds()
	a, b, c := d.newKind(kOct, 0, 0), d.newKind(kOct, 0, 0), d.newKind(kOct, 0, 0)
	a.X, a.Y, b.X, b.Y, c.X, c.Y = 0, 0, 40, 6, 10, 30
	d.links = []dLink{{a.ID, b.ID}, {a.ID, c.ID}}
	for zoom := range designZooms {
		for _, cam := range [][2]float64{{-3, -2}, {10.4, 5.7}, {-40, 0}} {
			d.zoom, d.camX, d.camY = zoom, cam[0], cam[1]
			w, h := 90, 24
			want := lipgloss.NewCanvas(w, h)
			want.Compose(lipgloss.NewLayer(d.boardLayer(w, h)))
			got := newBoardCanvas(w, h)
			d.paintBoard(got)
			if got.Render() != want.Render() {
				t.Fatalf("zoom %d cam %v: painted board differs\n got %q\nwant %q", zoom, cam, ansi.Strip(got.Render()), ansi.Strip(want.Render()))
			}
		}
	}
}

// The copy saved in the background holds every saved field: each one is
// filled in here (by reflection, so a field added later is checked too).
func TestDesignSnapshotKeepsEveryField(t *testing.T) {
	fill := func(o *dOct, n int) {
		v := reflect.ValueOf(o).Elem()
		for i := 0; i < v.NumField(); i++ {
			f := v.Field(i)
			if !v.Type().Field(i).IsExported() {
				continue
			}
			switch f.Kind() {
			case reflect.String:
				f.SetString(fmt.Sprint("v", i, n))
			case reflect.Int, reflect.Int64, reflect.Int8:
				f.SetInt(int64(i + n + 1))
			case reflect.Bool:
				f.SetBool(true)
			}
		}
	}
	a, b := &dOct{}, &dOct{}
	fill(a, 1)
	fill(b, 2)
	a.Items = []*dOct{b}
	want, _ := json.Marshal([]*dOct{a})
	got, _ := json.Marshal(copyOcts([]*dOct{a}))
	if string(got) != string(want) {
		t.Fatalf("the snapshot loses fields:\n got %s\nwant %s", got, want)
	}
	if copyOcts([]*dOct{a})[0] == a || copyOcts([]*dOct{a})[0].Items[0] == b {
		t.Fatal("the snapshot shares structures with the board")
	}
}

// With saving later, changes reach the disk after the wait, and quitting
// writes whatever is still waiting.
func TestDesignSaveLater(t *testing.T) {
	m := designTest(t)
	d := m.ds()
	designSaveLater = true
	defer func() { designSaveLater = false }()
	o := d.newKind(kOct, 0, 0)
	o.Header = "First"
	d.dirty = true
	d.save()
	if !d.savePending {
		t.Fatal("nothing waits to be saved")
	}
	cmd := m.designSaveCmd()
	if cmd == nil || m.designSaveCmd() != nil {
		t.Fatal("want exactly one wait")
	}
	write := m.designSaveNow()
	done := make(chan designSavedMsg)
	go func() { done <- write().(designSavedMsg) }() // as Bubble Tea runs it
	o.Header = "Changed after the snapshot"          // the board changes meanwhile
	o.Body += " and more"
	if msg := <-done; msg.err != "" {
		t.Fatal(msg.err)
	}
	if got := savedHeaders(t, d.loadedFrom); got != "First" {
		t.Fatalf("written: %q", got)
	}
	d.dirty = true
	d.save()
	d.flushSave()
	if got := savedHeaders(t, d.loadedFrom); got != "Changed after the snapshot" {
		t.Fatalf("quitting did not write the last change: %q", got)
	}
	// An older write landing late must not undo a newer one.
	if err := d.writeSnapshot(designFile{}, 1); err != "" {
		t.Fatal(err)
	}
	if got := savedHeaders(t, d.loadedFrom); got != "Changed after the snapshot" {
		t.Fatal("an older snapshot overwrote a newer one")
	}
}

// savedHeaders reads the board on disk and gives its structures' headers.
func savedHeaders(t *testing.T, path string) string {
	t.Helper()
	f, _, err := readDesignFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var hs []string
	for _, o := range f.Octs {
		hs = append(hs, o.Header)
	}
	return strings.Join(hs, "|")
}

// tintSlow is the pen tint as it was first written, cell by cell: the
// reference the faster tint must match.
func (d *designState) tintSlow(c *boardCanvas) {
	z := d.z()
	for y := range c.Height() {
		wy := int(math.Floor(d.camY + (float64(y)+0.5)/z))
		for x := range c.Width() {
			mk, ok := d.marks[[2]int{int(math.Floor(d.camX + (float64(x)+0.5)/z)), wy}]
			if !ok {
				continue
			}
			cell := c.CellAt(x, y)
			if cell == nil || cell.Width == 0 {
				continue
			}
			pen := penColors[mk.Pen]
			bg, fg := colorHex(cell.Style.Bg, cBG), colorHex(cell.Style.Fg, cText)
			if !mk.Over {
				cell.Style.Bg = hex(mixHex(bg, pen, 0.5))
				continue
			}
			nbg := mixHex(bg, pen, 0.38)
			cell.Style.Bg = hex(nbg)
			if strings.TrimSpace(cell.Content) == "" {
				cell.Content, cell.Width = "░", 1
				cell.Style.Fg = hex(mixHex(nbg, pen, 0.55))
			} else {
				cell.Style.Fg = hex(mixHex(fg, pen, 0.45))
			}
		}
	}
}

// The fast pen tint gives exactly the old result: ink over and under,
// on the empty board, on text and on a structure, at several zooms.
func TestDesignTintMatchesSlow(t *testing.T) {
	m := designTest(t)
	d := m.ds()
	o := d.newKind(kOct, 0, 0)
	o.X, o.Y, o.W, o.H, o.Header, o.Body = 2, 2, 30, 14, "Inked", "Words under the pen"
	for i := 0; i < 600; i++ {
		d.marks[[2]int{(i * 7) % 60, (i * 13) % 30}] = dMark{Pen: int8(i % 8), Over: i%3 == 0}
	}
	for zoom := range designZooms {
		d.zoom, d.camX, d.camY, d.drawn = zoom, -1.3, -0.6, nil
		marks := d.marks
		d.marks = nil // draw without ink, then ink it both ways
		base := m.designCanvas()
		d.marks = marks
		fast, slow := newBoardCanvas(base.Width(), base.Height()), newBoardCanvas(base.Width(), base.Height())
		for y := range base.Lines {
			copy(fast.Lines[y], base.Lines[y])
			copy(slow.Lines[y], base.Lines[y])
		}
		d.tint(fast)
		d.tintSlow(slow)
		if fast.Render() != slow.Render() {
			t.Fatalf("zoom %d: fast tint differs\n got %q\nwant %q", zoom, fast.Render(), slow.Render())
		}
	}
}

// The board drawn the fast way (cached cells copied in, the board painted
// straight on) looks exactly as drawn the original way: every structure as
// styled text, put together by Lip Gloss's compositor. Overlaps, wide
// letters, open drawers and structures half off the screen included.
func TestDesignCanvasMatchesCompositor(t *testing.T) {
	m := designTest(t)
	d := m.ds()
	a := d.newKind(kOct, 0, 0)
	a.X, a.Y, a.W, a.H, a.Header, a.Body = 0, 0, 30, 14, "日本 header", "Wide 漢字 and emoji 🎉 inside the body text"
	b := d.newKind(kPage, 0, 0)
	b.X, b.Y, b.W, b.H, b.Header, b.Body = 20, 6, 28, 12, "Page", strings.Repeat("漢字かな mixed with words 🎉\n", 30)
	c := d.newKind(kCabinet, 0, 0)
	c.X, c.Y, c.Header = 44, 2, "Archive"
	c.Items = []*dOct{{Kind: kPage, Header: "One"}, {Kind: kNote, Body: "Two"}}
	n := d.newKind(kNote, 0, 0)
	n.X, n.Y, n.Body = 5, 18, "A note 🎉"
	a.Items, a.DrawerOpen = []*dOct{{Kind: kPage, Header: "Kept"}}, true
	d.links = []dLink{{a.ID, c.ID}, {b.ID, n.ID}}
	t.Cleanup(func() { picturesDirect, middlesDirect = true, true })
	for zoom := range designZooms {
		for _, cam := range [][2]float64{{-2, -1}, {7.5, 3.25}, {-31, -9}, {13, 0}} {
			for _, sel := range []int{-1, b.ID} {
				d.zoom, d.camX, d.camY, d.sel, d.editing, d.drawn = zoom, cam[0], cam[1], sel, partNone, nil
				w, h := m.width, m.textHeight()
				picturesDirect, middlesDirect = false, false
				ref := lipgloss.NewCanvas(w, h)
				ref.Compose(lipgloss.NewLayer(d.boardLayer(w, h)))
				var layers []*lipgloss.Layer
				for i, o := range d.octs {
					for _, l := range d.structLayers(o) {
						layers = append(layers, l.Z(i+1))
					}
				}
				ref.Compose(lipgloss.NewCompositor(layers...))
				picturesDirect, middlesDirect = true, true
				d.drawn = nil
				got := m.designCanvas().Render()
				if got != ref.Render() {
					gl, wl := strings.Split(got, "\n"), strings.Split(ref.Render(), "\n")
					for i := range wl {
						if i < len(gl) && gl[i] != wl[i] {
							t.Fatalf("zoom %d cam %v sel %d: row %d differs\n got %q\nwant %q\n got %q\nwant %q", zoom, cam, sel, i, ansi.Strip(gl[i]), ansi.Strip(wl[i]), gl[i], wl[i])
						}
					}
					t.Fatalf("zoom %d cam %v: differs in length", zoom, cam)
				}
			}
		}
	}
}

// The board's own text writer gives the very cells uv's rendering gives,
// once the terminal side reads them back: pictures, wide letters, bold and
// italic words, ink over and under.
func TestDesignRenderToMatchesUV(t *testing.T) {
	m := designTest(t)
	d := m.ds()
	dir := d.imagesDir()
	os.MkdirAll(dir, 0o700)
	img := image.NewNRGBA(image.Rect(0, 0, 40, 30))
	for y := 0; y < 30; y++ {
		for x := 0; x < 40; x++ {
			img.Set(x, y, color.NRGBA{uint8(x * 6), uint8(y * 8), uint8(x * y), 255})
		}
	}
	f, _ := os.Create(filepath.Join(dir, "r.png"))
	png.Encode(f, img)
	f.Close()
	a := d.newKind(kOct, 0, 0)
	a.X, a.Y, a.W, a.H, a.Image, a.Layout, a.Header = 0, 0, 30, 16, "r.png", layoutPicture, "Picture"
	b := d.newKind(kPage, 0, 0)
	b.X, b.Y, b.W, b.H, b.Header, b.Body = 26, 4, 30, 14, "日本語 page", strings.Repeat("漢字 and 🎉 words\n", 20)
	d.sel = b.ID
	for i := 0; i < 300; i++ {
		d.marks[[2]int{(i * 7) % 70, (i * 11) % 28}] = dMark{Pen: int8(i % 8), Over: i%2 == 0}
	}
	read := func(s string, w, h int) *boardCanvas {
		c := newBoardCanvas(w, h)
		uv.NewStyledString(s).Draw(c, c.Bounds())
		return c
	}
	for zoom := range designZooms {
		d.zoom, d.camX, d.camY, d.drawn = zoom, -2.5, -1.5, nil
		c := m.designCanvas()
		var sb strings.Builder
		c.renderTo(&sb)
		w, h := c.Width(), c.Height()
		got, want := read(sb.String(), w, h), read(c.Render(), w, h)
		for y := range h {
			for x := range w {
				g, wnt := got.CellAt(x, y), want.CellAt(x, y)
				if g.Content != wnt.Content || g.Width != wnt.Width || !g.Style.Equal(&wnt.Style) || g.Link != wnt.Link {
					t.Fatalf("zoom %d: cell %d,%d is %+v, uv gives %+v", zoom, x, y, *g, *wnt)
				}
			}
		}
	}
}

// screenCell is the letter at screen cell (x, y), board rows counted from
// the top of the window.
func screenCell(m *Model, x, y int) string {
	rows := strings.Split(screenOf(m), "\n")
	if y >= len(rows) {
		return ""
	}
	r := []rune(rows[y])
	if x >= len(r) {
		return ""
	}
	return string(r[x])
}

// findItem is the thing with this ID anywhere on the board, or nil.
func findItem(d *designState, id int) *dOct {
	var in func([]*dOct) *dOct
	in = func(os []*dOct) *dOct {
		for _, o := range os {
			if o == nil {
				continue
			}
			if o.ID == id {
				return o
			}
			if f := in(o.Items); f != nil {
				return f
			}
		}
		return nil
	}
	return in(d.octs)
}

// Every element can be deleted, by key and by its ✕, and Ctrl+Z brings it
// back: structures (with their links), links, things in a cabinet or a
// drawer, an octagon's picture. (Undo puts back copies: look things up.)
func TestDesignDelete(t *testing.T) {
	m := designTest(t)
	d := m.ds()
	d.camX, d.camY = 0, 0
	a := d.newKind(kOct, 0, 0)
	a.X, a.Y, a.W, a.H, a.Header = 2, 1, 24, 12, "Alpha"
	b := d.newKind(kOct, 0, 0)
	b.X, b.Y, b.W, b.H, b.Header = 60, 1, 24, 12, "Beta"
	aID, bID := a.ID, b.ID
	d.links = []dLink{{aID, bID}}
	d.sel, d.editing = -1, partNone
	d.dirty = true
	d.save() // the board so far is where undo starts

	// A structure, by Delete: gone with its link; Ctrl+Z brings both back.
	d.sel = aID
	press(m, "delete")
	if d.byID(aID) != nil || len(d.links) != 0 {
		t.Fatalf("Delete: %d structures, %d links", len(d.octs), len(d.links))
	}
	press(m, "ctrl+z")
	if d.byID(aID) == nil || len(d.links) != 1 {
		t.Fatalf("Ctrl+Z: structure back %v, %d links", d.byID(aID) != nil, len(d.links))
	}

	// The same by its ✕, which shows on the chosen structure.
	d.sel = bID
	m.View()
	b = d.byID(bID)
	x, y, w, h := d.rect(b)
	dx, dy, ok := d.deleteSpot(b, x, y, w, h)
	if !ok || screenCell(m, dx, dy+textTop) != "✕" {
		t.Fatalf("no ✕ on the chosen structure (at %d,%d: %q)", dx, dy, screenCell(m, dx, dy+textTop))
	}
	click(m, dx, dy+textTop, tea.MouseLeft)
	if d.byID(bID) != nil {
		t.Fatal("the ✕ did not delete the structure")
	}
	press(m, "ctrl+z")
	if d.byID(bID) == nil || len(d.links) != 1 {
		t.Fatal("Ctrl+Z did not bring back the structure and its link")
	}

	// A link: click its line, Delete deletes it, Ctrl+Z brings it back.
	d.sel = -1
	m.View()
	lx, ly := -1, -1
	for cell := range d.linkCells(m.width, m.textHeight()) {
		if d.hit(cell[0], cell[1]).o == nil {
			lx, ly = cell[0], cell[1]
			break
		}
	}
	click(m, lx, ly+textTop, tea.MouseLeft)
	if d.selLink != 0 {
		t.Fatalf("clicking the link at %d,%d did not choose it", lx, ly)
	}
	press(m, "backspace")
	if len(d.links) != 0 || d.byID(aID) == nil || d.byID(bID) == nil {
		t.Fatalf("Delete on a link: %d links, structures kept %v %v", len(d.links), d.byID(aID) != nil, d.byID(bID) != nil)
	}
	press(m, "ctrl+z")
	if len(d.links) != 1 {
		t.Fatal("Ctrl+Z did not bring back the link")
	}

	// A thing in a cabinet, by the ✕ on its row.
	cab := d.newKind(kCabinet, 0, 0)
	cab.X, cab.Y, cab.Header = 30, 14, "Archive"
	cabID := cab.ID
	cab.Items, cab.OpenStack = []*dOct{{ID: 101, Kind: kPage, Header: "First"}, {ID: 102, Kind: kPage, Header: "Second"}}, kPage
	d.sel = -1
	d.dirty = true
	d.save()
	rowX := func(word string) (int, int) {
		for i, r := range strings.Split(screenOf(m), "\n") {
			if strings.Contains(r, word) {
				rr := []rune(r)
				for j := len(rr) - 1; j >= 0; j-- {
					if rr[j] == '✕' {
						return j, i
					}
				}
			}
		}
		return -1, -1
	}
	cx, cy := rowX("First")
	if cx < 0 {
		t.Fatalf("no ✕ on the cabinet's row:\n%s", screenOf(m))
	}
	click(m, cx, cy, tea.MouseLeft)
	if c := d.byID(cabID); len(c.Items) != 1 || c.Items[0].ID != 102 {
		t.Fatalf("the row's ✕ left %d things", len(c.Items))
	}
	press(m, "esc")
	press(m, "ctrl+z")
	if c := d.byID(cabID); len(c.Items) != 2 || c.Items[0].ID != 101 {
		t.Fatal("Ctrl+Z did not put the page back in its place")
	}

	// A structure holding things asks first; y deletes, Ctrl+Z brings it all back.
	d.sel = cabID
	press(m, "delete")
	if d.byID(cabID) == nil || m.prompt != pConfirm {
		t.Fatal("deleting a full cabinet did not ask")
	}
	m.Update(tea.KeyPressMsg(tea.Key{Code: 'y', Text: "y"}))
	if d.byID(cabID) != nil {
		t.Fatal("y did not delete the cabinet")
	}
	press(m, "ctrl+z")
	if got := d.byID(cabID); got == nil || len(got.Items) != 2 {
		t.Fatal("Ctrl+Z did not bring back the cabinet with its pages")
	}

	// An octagon's picture: the ✕ on it takes out just the picture.
	os.MkdirAll(d.imagesDir(), 0o700)
	a = d.byID(aID)
	a.Image, a.Layout = filepath.Base(writePNG(t, d.imagesDir())), layoutPicture
	d.sel, d.drawn = aID, nil
	d.dirty = true
	d.save()
	m.View()
	ax, ay, _, _ := d.rect(a)
	px, py, ok := d.pictureDeleteSpot(a, ax, ay)
	if !ok || screenCell(m, px, py+textTop) != "✕" {
		t.Fatalf("no ✕ on the chosen picture (at %d,%d: %q)", px, py, screenCell(m, px, py+textTop))
	}
	click(m, px, py+textTop, tea.MouseLeft)
	if d.byID(aID) == nil || d.byID(aID).Image != "" {
		t.Fatal("the picture's ✕ did not take out just the picture")
	}
	press(m, "ctrl+z")
	if d.byID(aID).Image == "" {
		t.Fatal("Ctrl+Z did not bring back the picture")
	}
	d.sel = aID
	press(m, "shift+delete")
	if d.byID(aID) == nil || d.byID(aID).Image != "" {
		t.Fatal("Shift+Delete did not take out just the picture")
	}

	// A thing in an octagon's open drawer, by the ✕ on its row.
	b = d.byID(bID)
	b.Items, b.DrawerOpen, b.OpenStack = []*dOct{{ID: 201, Kind: kNote, Body: "In the drawer"}}, true, kNote
	d.sel, d.drawn = -1, nil
	d.dirty = true
	d.save()
	cx, cy = rowX("In the drawer")
	if cx < 0 {
		t.Fatalf("no ✕ on the drawer's row:\n%s", screenOf(m))
	}
	click(m, cx, cy, tea.MouseLeft)
	if len(d.byID(bID).Items) != 0 {
		t.Fatal("the drawer row's ✕ did not delete the note")
	}
	press(m, "ctrl+z")
	if it := d.byID(bID).Items; len(it) != 1 || it[0].ID != 201 {
		t.Fatal("Ctrl+Z did not put the note back in the drawer")
	}
}

// Undo and redo: every finished change is a step, writing in one part is
// one step, looking around is none, and a new change ends what can be redone.
func TestDesignUndoRedo(t *testing.T) {
	m := designTest(t)
	d := m.ds()
	d.camX, d.camY = 0, 0
	before := len(d.octs)

	// A new structure, written in: placing it is one step, its words another.
	click(m, 30, 10, tea.MouseLeft)
	click(m, 30, 10, tea.MouseLeft)
	typeKeys(m, "Plan")
	press(m, "esc")
	o := d.byID(d.sel)
	if o == nil || o.Header != "Plan" {
		t.Fatal("no new structure called Plan")
	}
	id := o.ID

	// A move with the arrows: one step per press.
	d.sel = id
	press(m, "alt+right")
	press(m, "alt+right")
	x := d.byID(id).X

	// Looking around is not a step.
	press(m, "=") // zoom in (the key with +)
	press(m, "-")

	press(m, "ctrl+z")
	if d.byID(id).X != x-1 {
		t.Fatalf("Ctrl+Z after two moves: x %d, want %d", d.byID(id).X, x-1)
	}
	press(m, "ctrl+z")
	if d.byID(id).X != x-2 {
		t.Fatal("the second Ctrl+Z did not take back the first move")
	}
	press(m, "ctrl+z") // placed and named at once: one step
	if d.byID(id) != nil || len(d.octs) != before {
		t.Fatal("Ctrl+Z did not take back the new structure with its title")
	}

	// Words written later into a part are a step of their own.
	press(m, "ctrl+y")
	if got := d.byID(id); got == nil || got.Header != "Plan" {
		t.Fatalf("Ctrl+Y did not bring back the structure and its title: %+v", got)
	}
	d.sel = id
	press(m, "enter") // write in its header again
	typeKeys(m, " B")
	press(m, "esc")
	if d.byID(id).Header != "Plan B" {
		t.Fatalf("header %q", d.byID(id).Header)
	}
	press(m, "ctrl+z")
	if d.byID(id).Header != "Plan" {
		t.Fatalf("Ctrl+Z did not take back the added words as one step: %q", d.byID(id).Header)
	}
	press(m, "ctrl+shift+z")
	if d.byID(id).Header != "Plan B" {
		t.Fatal("Ctrl+Shift+Z did not do them again")
	}

	// A new change ends the redo.
	press(m, "ctrl+z")
	d.sel = id
	press(m, "alt+down")
	n := len(d.redo)
	press(m, "ctrl+y")
	if n != 0 || d.byID(id).Header != "Plan" {
		t.Fatalf("redo after a new change: %d to redo, header %q", n, d.byID(id).Header)
	}

	// Ink is a step: a stroke with the pen, then undone.
	marks := len(d.marks)
	press(m, "1")
	dragTo(m, 10, 20, 20, 20, tea.MouseLeft)
	press(m, "esc")
	if len(d.marks) == marks {
		t.Fatal("the pen drew nothing")
	}
	press(m, "ctrl+z")
	if len(d.marks) != marks {
		t.Fatalf("Ctrl+Z left %d of the stroke's marks", len(d.marks)-marks)
	}

	// The toolbar's ↷ and ↶ do the same.
	var undoX, redoX int
	m.View()
	for _, s := range d.toolbar {
		switch s.act {
		case "undo":
			undoX = s.x0
		case "redo":
			redoX = s.x0
		}
	}
	click(m, redoX, m.height-3, tea.MouseLeft)
	if len(d.marks) == marks {
		t.Fatal("↷ did not bring the stroke back")
	}
	click(m, undoX, m.height-3, tea.MouseLeft)
	if len(d.marks) != marks {
		t.Fatal("↶ did not take the stroke back")
	}
}

// A step notices a change to every saved field of a structure (by
// reflection, so a field added later is not forgotten), except what is
// only looking around.
func TestDesignUndoSeesEveryField(t *testing.T) {
	looking := map[string]bool{"Scroll": true, "DrawerOpen": true, "OpenStack": true, "Items": true}
	typ := reflect.TypeOf(dOct{})
	for i := 0; i < typ.NumField(); i++ {
		f := typ.Field(i)
		if !f.IsExported() || looking[f.Name] {
			continue
		}
		a := &dOct{}
		b := &dOct{}
		v := reflect.ValueOf(b).Elem().Field(i)
		switch v.Kind() {
		case reflect.String:
			v.SetString("changed")
		case reflect.Int, reflect.Int64:
			v.SetInt(7)
		case reflect.Bool:
			v.SetBool(true)
		default:
			t.Fatalf("field %s: a kind this test does not know", f.Name)
		}
		if sameOct(a, b) {
			t.Errorf("a change to %s is not a step", f.Name)
		}
	}
}

// The board kept in parts: an old whole board is read, kept as
// board.v1.json, and written in parts; a save writes only what changed; a
// deleted structure's part goes; a missing part does not lose the rest.
func TestDesignStoreInParts(t *testing.T) {
	m := designTest(t)
	d := m.ds()
	path := d.loadedFrom
	os.MkdirAll(filepath.Dir(path), 0o700)
	old := `{"structures":[{"id":1,"x":0,"y":0,"w":26,"h":9,"header":"One","body":"","footer":"","color":"#ff0000"},` +
		`{"id":2,"x":40,"y":0,"w":26,"h":9,"header":"Two","body":"","footer":"","color":"#00ff00"}],` +
		`"links":[{"a":1,"b":2}],"marks":[{"x":3,"y":4,"pen":2,"over":false}],"camX":0,"camY":0,"zoom":5,"nextID":3}`
	if err := os.WriteFile(path, []byte(old), 0o600); err != nil {
		t.Fatal(err)
	}
	d = loadDesign(path)
	if len(d.octs) != 2 || len(d.links) != 1 || len(d.marks) != 1 {
		t.Fatalf("old board read: %d structures, %d links, %d marks", len(d.octs), len(d.links), len(d.marks))
	}
	d.dirty = true
	d.save()
	if b, err := os.ReadFile(filepath.Join(filepath.Dir(path), "board.v1.json")); err != nil || string(b) != old {
		t.Fatalf("the old board was not kept as board.v1.json: %v", err)
	}
	for _, id := range []int{1, 2} {
		if _, err := os.Stat(partPath(path, id)); err != nil {
			t.Fatalf("part %d not written: %v", id, err)
		}
	}
	again := loadDesign(path)
	if len(again.octs) != 2 || again.octs[0].Header != "One" || len(again.links) != 1 || len(again.marks) != 1 {
		t.Fatalf("read back in parts: %d structures, %d links, %d marks", len(again.octs), len(again.links), len(again.marks))
	}

	// Only what changed is written: part 2 keeps its time.
	d = again
	before, _ := os.Stat(partPath(path, 2))
	time.Sleep(20 * time.Millisecond)
	d.byID(1).Header = "One, changed"
	d.dirty = true
	d.save()
	after, _ := os.Stat(partPath(path, 2))
	if !after.ModTime().Equal(before.ModTime()) {
		t.Fatal("an unchanged structure was written again")
	}
	if got := loadDesign(path).byID(1); got == nil || got.Header != "One, changed" {
		t.Fatal("the changed structure was not written")
	}

	// A deleted structure's part goes, after the index without it.
	d.deleteStruct(d.byID(2))
	d.dirty = true
	d.save()
	if _, err := os.Stat(partPath(path, 2)); !os.IsNotExist(err) {
		t.Fatal("the deleted structure's part is still there")
	}
	if got := loadDesign(path); len(got.octs) != 1 || len(got.links) != 0 {
		t.Fatalf("after deleting: %d structures, %d links", len(got.octs), len(got.links))
	}

	// A part gone missing: the rest of the board still opens.
	n := d.newKind(kOct, 0, 0)
	n.Header = "Three"
	d.dirty = true
	d.save()
	os.Remove(partPath(path, 1))
	got := loadDesign(path)
	if len(got.octs) != 1 || got.octs[0].Header != "Three" || got.saveErr == "" {
		t.Fatalf("with a part missing: %d structures, error %q", len(got.octs), got.saveErr)
	}
}

// dndRun runs cmd as Bubble Tea would: what it writes to the terminal is
// gathered, and messages go back into the model (and their commands run).
func dndRun(m *Model, cmd tea.Cmd) []string {
	var out []string
	var run func(tea.Cmd)
	run = func(c tea.Cmd) {
		if c == nil {
			return
		}
		switch msg := c().(type) {
		case nil:
		case tea.BatchMsg:
			for _, c := range msg {
				run(c)
			}
		case tea.RawMsg:
			out = append(out, fmt.Sprint(msg.Msg))
		case designSaveMsg, designFrameMsg:
		default:
			_, next := m.Update(msg)
			run(next)
		}
	}
	run(cmd)
	return out
}

// dndSend gives sy one of kitty's drag and drop codes and gathers its answer.
func dndSend(m *Model, code string) string {
	_, cmd := m.Update(uv.UnknownOscEvent("\x1b]72;" + code + "\x1b\\"))
	return strings.Join(dndRun(m, cmd), "")
}

// dndDrop drops data of the first offered type at screen cell (x, y), sent
// in pieces as kitty sends it.
func dndDrop(t *testing.T, m *Model, x, y int, offered, data string) {
	t.Helper()
	got := dndSend(m, fmt.Sprintf("t=M:x=%d:y=%d:X=0:Y=0;%s", x, y, offered))
	if !strings.Contains(got, "t=r:x=1") {
		t.Fatalf("the drop was not asked for: %q", got)
	}
	enc := base64.StdEncoding.EncodeToString([]byte(data))
	half := len(enc) / 2
	dndSend(m, "t=r:x=1:m=1;"+enc[:half])
	dndSend(m, "t=r:x=1:m=1;"+enc[half:])
	if got := dndSend(m, "t=r:x=1:m=0;"); !strings.Contains(got, "t=r:o=1") {
		t.Fatalf("the drop was not finished: %q", got)
	}
}

// Kitty's drag and drop: the board takes drops while it is shown, lights up
// where a drop would go, and puts each file where it was let go.
func TestDesignDragAndDrop(t *testing.T) {
	dndTerminal = true
	defer func() { dndTerminal = false }()
	m := designTest(t)
	d := m.ds()
	d.camX, d.camY = 0, 0
	_, cmd := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	if got := strings.Join(dndRun(m, cmd), ""); !strings.Contains(got, dndAccept) {
		t.Fatalf("the board did not ask for drops: %q", got)
	}

	// Dragged over the empty board: accepted, and a ✚ shows where.
	got := dndSend(m, "t=m:x=40:y=12:X=400:Y=240:o=3;text/uri-list text/plain")
	if !strings.Contains(got, "t=m:o=1;text/uri-list") {
		t.Fatalf("the drag was not accepted: %q", got)
	}
	if c := screenCell(m, 40, 12); c != "✚" {
		t.Fatalf("no ✚ under the drag: %q", c)
	}

	// Files dropped there: a picture gets a new octagon, a text file a new
	// page, a step apart; a name with a space, sent as %20, is found.
	dir := t.TempDir()
	pic := writePNG(t, dir)
	txt := filepath.Join(dir, "my notes.txt")
	os.WriteFile(txt, []byte("Line one\nLine two\n"), 0o600)
	uris := "file://" + pic + "\r\nfile://" + strings.ReplaceAll(txt, " ", "%20") + "\r\n"
	n := len(d.octs)
	dndDrop(t, m, 40, 12, "text/uri-list text/plain", uris)
	if len(d.octs) != n+2 {
		t.Fatalf("dropping 2 files made %d structures", len(d.octs)-n)
	}
	var picO, page *dOct
	for _, o := range d.octs[n:] {
		if o.Image != "" {
			picO = o
		}
		if o.Kind == kPage {
			page = o
		}
	}
	if picO == nil || page == nil || !strings.Contains(page.Body, "Line two") || page.Header != "my notes.txt" {
		t.Fatalf("dropped: picture %v, page %+v", picO != nil, page)
	}
	wx, wy := d.world(40, 12-textTop)
	if cx := picO.X + picO.W/2; abs(cx-int(wx)) > 1 || abs(picO.Y+picO.H/2-int(wy)) > 1 {
		t.Fatalf("the picture's octagon is at %d,%d, not where it was dropped (%v,%v)", picO.X, picO.Y, wx, wy)
	}

	// Dragged over a structure, it lights up; a picture dropped on an
	// octagon goes into it.
	oct := d.newKind(kOct, 0, 0)
	oct.X, oct.Y, oct.W, oct.H, oct.Header = 2, 2, 24, 12, "Frame"
	d.sel, d.editing = -1, partNone
	m.View()
	ox, oy, _, _ := d.rect(oct)
	dndSend(m, fmt.Sprintf("t=m:x=%d:y=%d:o=3;text/uri-list", ox+5, oy+5+textTop))
	if d.dropOn != oct.ID {
		t.Fatal("the octagon under the drag did not light up")
	}
	n = len(d.octs)
	dndDrop(t, m, ox+5, oy+5+textTop, "text/uri-list", "file://"+pic+"\r\n")
	if oct.Image == "" || len(d.octs) != n {
		t.Fatalf("a picture dropped on an octagon: image %q, %d new structures", oct.Image, len(d.octs)-n)
	}

	// A text file dropped on a cabinet is filed in it as a page.
	cab := d.newKind(kCabinet, 0, 0)
	cab.X, cab.Y, cab.Header = 50, 2, "Archive"
	d.sel, d.editing = -1, partNone
	cx, cy, _, _ := d.rect(cab)
	dndDrop(t, m, cx+3, cy+1+textTop, "text/uri-list", "file://"+txt+"\r\n")
	if len(cab.Items) != 1 || cab.Items[0].Kind != kPage {
		t.Fatalf("a text file dropped on a cabinet: %d things in it", len(cab.Items))
	}

	// Dragged text (no files) becomes a page there.
	n = len(d.octs)
	dndDrop(t, m, 60, 20, "text/plain", "Words from a browser")
	if len(d.octs) != n+1 || d.octs[len(d.octs)-1].Body != "Words from a browser" {
		t.Fatal("dropped text did not become a page")
	}

	// Leaving: the light goes out; out of Design mode, no more drops.
	dndSend(m, "t=m:x=-1:y=-1")
	if d.drop.over || d.dropOn >= 0 {
		t.Fatal("the drag left, but the board still shows it")
	}
	m.setMode(0) // Text mode
	_, cmd = m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	if got := strings.Join(dndRun(m, cmd), ""); !strings.Contains(got, dndStop) {
		t.Fatalf("leaving the board did not stop drops: %q", got)
	}
}

// Without kitty's protocol, a terminal pastes dropped files as paths, one
// a line: each file is put in, not pasted as text.
func TestDesignPasteSeveralFiles(t *testing.T) {
	m := designTest(t)
	d := m.ds()
	dir := t.TempDir()
	a := filepath.Join(dir, "a.txt")
	b := filepath.Join(dir, "b c.txt")
	os.WriteFile(a, []byte("alpha"), 0o600)
	os.WriteFile(b, []byte("beta"), 0o600)
	n := len(d.octs)
	dndRun(m, m.update(tea.PasteMsg{Content: a + "\n" + b + "\n"}))
	if len(d.octs) != n+2 {
		t.Fatalf("pasting 2 dropped files made %d structures", len(d.octs)-n)
	}
	for _, o := range d.octs[n:] {
		if strings.Contains(o.Body, dir) {
			t.Fatalf("a path was pasted as text: %q", o.Body)
		}
	}
}

// linkGridSlow is the link drawing as it was first written: the reference.
func (d *designState) linkGridSlow(w, h int, owner map[[2]int]int) map[[2]int]struct{ s, fg string } {
	ways := map[[2]int]int{}
	colour := map[[2]int]string{}
	current := 0
	add := func(x, y, way int, c string) {
		if x < 0 || y < 0 || x >= w || y >= h {
			return
		}
		ways[[2]int{x, y}] |= way
		colour[[2]int{x, y}] = c
		if owner != nil {
			owner[[2]int{x, y}] = current
		}
	}
	hline := func(x0, x1, y int, c string) {
		if y < 0 || y >= h {
			return
		}
		if x0 > x1 {
			x0, x1 = x1, x0
		}
		for x := max(x0, -1); x < min(x1, w); x++ {
			add(x, y, goRight, c)
			add(x+1, y, goLeft, c)
		}
	}
	vline := func(x, y0, y1 int, c string) {
		if x < 0 || x >= w {
			return
		}
		if y0 > y1 {
			y0, y1 = y1, y0
		}
		for y := max(y0, -1); y < min(y1, h); y++ {
			add(x, y, goDown, c)
			add(x, y+1, goUp, c)
		}
	}
	for i, l := range d.links {
		a, b := d.byIDSlow(l.A), d.byIDSlow(l.B)
		if a == nil || b == nil {
			continue
		}
		current = i
		c := mixHex(mixHex(a.Color, b.Color, 0.5), cBG, 0.2)
		if i == d.selLink {
			c = cAccent // the chosen link
		}
		ax, ay, aw, ah := d.rect(a)
		bx, by, bw, bh := d.rect(b)
		x0, y0, x1, y1 := ax+aw/2, ay+ah/2, bx+bw/2, by+bh/2
		if abs(x1-x0) >= 2*abs(y1-y0) { // side by side: across, down, across
			mx := (x0 + x1) / 2
			hline(x0, mx, y0, c)
			vline(mx, y0, y1, c)
			hline(mx, x1, y1, c)
		} else { // one above the other: down, across, down
			my := (y0 + y1) / 2
			vline(x0, y0, my, c)
			hline(x0, x1, my, c)
			vline(x1, my, y1, c)
		}
	}
	out := map[[2]int]struct{ s, fg string }{}
	for at, way := range ways {
		if s, ok := boxLetters[way]; ok {
			out[at] = struct{ s, fg string }{s, colour[at]}
		}
	}
	return out
}

// byIDSlow is finding a structure by looking at each: the reference.
func (d *designState) byIDSlow(id int) *dOct {
	for _, o := range d.octs {
		if o.ID == id {
			return o
		}
	}
	return nil
}

// The fast ways give what the first ways gave: links drawn (with the
// chosen one, links off the screen and crossing), a cabinet's row count,
// and finding a structure while the board's list changes in every way.
func TestDesignManyElementsMatchSlow(t *testing.T) {
	m := designTest(t)
	d := m.ds()
	rnd := 3
	next := func(k int) int { rnd = (rnd*1103515245 + 12345) & 0x7fffffff; return rnd % k }
	for i := 0; i < 60; i++ {
		o := d.newKind([]string{kOct, kPage, kNote, kCabinet}[i%4], 0, 0)
		o.X, o.Y = next(400)-200, next(200)-100
	}
	for i := 0; i < 80; i++ {
		d.links = append(d.links, dLink{d.octs[next(60)].ID, d.octs[next(60)].ID})
	}
	for _, sel := range []int{-1, 5} {
		d.selLink = sel
		for zoom := range designZooms {
			for _, cam := range [][2]float64{{-50, -20}, {0, 0}, {120, 40}} {
				d.zoom, d.camX, d.camY = zoom, cam[0], cam[1]
				fo, so := map[[2]int]int{}, map[[2]int]int{}
				fast, slow := d.linkGrid(100, 24, fo), d.linkGridSlow(100, 24, so)
				if len(fast) != len(slow) {
					t.Fatalf("zoom %d cam %v: %d link cells, first way %d", zoom, cam, len(fast), len(slow))
				}
				for k, v := range slow {
					if fast[k] != v || fo[k] != so[k] {
						t.Fatalf("zoom %d cam %v: cell %v is %v (link %d), first way %v (link %d)", zoom, cam, k, fast[k], fo[k], v, so[k])
					}
				}
			}
		}
	}

	// A cabinet's rows, counted and made, in every state.
	cab := &dOct{Kind: kCabinet}
	for _, k := range stackKinds {
		for j := 0; j < 3; j++ {
			cab.Items = append(cab.Items, &dOct{Kind: k, Header: "x"})
		}
	}
	cab.Items = append(cab.Items, nil)
	for _, open := range append([]string{""}, stackKinds...) {
		cab.OpenStack = open
		for _, compact := range []bool{false, true} {
			if a, b := stackRowCount(cab, compact), len(stackRows(cab, compact)); a != b {
				t.Fatalf("open %q compact %v: counted %d rows, made %d", open, compact, a, b)
			}
		}
	}

	// Finding structures while the list is reordered, cut, grown, swapped.
	check := func(what string) {
		t.Helper()
		for id := -2; id < d.nextID+3; id++ {
			if d.byID(id) != d.byIDSlow(id) {
				t.Fatalf("%s: byID(%d) is wrong", what, id)
			}
		}
	}
	check("at first")
	d.toFront(d.octs[3])
	check("brought to the front")
	d.remove(d.octs[10].ID)
	check("one removed")
	d.newKind(kOct, 0, 0)
	check("one added")
	d.octs[0], d.octs[1] = d.octs[1], d.octs[0]
	check("two swapped in place")
	d.octs[2] = &dOct{ID: 9999, Kind: kOct}
	check("one replaced in place")
	d.restore(d.state())
	check("after undo puts a copy back")
}

// A file dropped while writing in a page goes in as a file: its name is
// not typed into the page.
func TestDesignDropWhileWriting(t *testing.T) {
	m := designTest(t)
	d := m.ds()
	pg := d.newKind(kPage, 0, 0)
	d.sel, d.editing = pg.ID, partBody
	dir := t.TempDir()
	txt := filepath.Join(dir, "more.txt")
	os.WriteFile(txt, []byte("More words"), 0o600)
	dndRun(m, m.update(tea.PasteMsg{Content: txt}))
	if strings.Contains(pg.Body, dir) {
		t.Fatalf("the path was typed into the page: %q", pg.Body)
	}
	if !strings.Contains(pg.Body, "More words") {
		t.Fatalf("the file's words did not go into the page: %q", pg.Body)
	}
	// Words that are not file names are still typed in.
	d.editing = partBody
	dndRun(m, m.update(tea.PasteMsg{Content: "just words"}))
	if !strings.HasSuffix(pg.Body, "just words") {
		t.Fatalf("pasted words: %q", pg.Body)
	}
}

// The board by keyboard: the arrows move the view and choose what is in
// the middle, E writes in it, Alt + arrows move it, R is the rubber, + and
// - (and the keypad's) zoom, and AltGr + K shows the keys.
func TestDesignKeyboard(t *testing.T) {
	m := designTest(t)
	d := m.ds()
	d.camX, d.camY, d.zoom = 0, 0, designZoom100
	w, h := m.width, m.textHeight()
	left := d.newKind(kOct, 0, 0)
	left.X, left.Y, left.W, left.H, left.Header = w/2-13, h/2-6, 26, 12, "Middle"
	right := d.newKind(kPage, 0, 0)
	right.X, right.Y, right.W, right.H, right.Header = w/2+60, h/2-5, 24, 10, "Far right"
	d.sel, d.editing = -1, partNone

	// An arrow moves the view (not a structure) and chooses the one in the middle.
	press(m, "right")
	if d.camX <= 0 || left.X != w/2-13 {
		t.Fatalf("Right: view at %v, structure at %d", d.camX, left.X)
	}
	if d.sel != left.ID {
		t.Fatalf("Right did not choose the structure in the middle (chosen %d)", d.sel)
	}
	for i := 0; i < 4; i++ { // the view goes right (16 cells a press) until the page is in the middle
		press(m, "shift+right")
	}
	if d.sel != right.ID {
		t.Fatalf("after moving the view right, the page in the middle is not chosen (chosen %d)", d.sel)
	}

	// E writes in it: a page's text.
	press(m, "e")
	if d.editing != partBody {
		t.Fatalf("E on a page: writing in part %d", d.editing)
	}
	typeKeys(m, "Hello")
	press(m, "esc")
	if right.Body != "Hello" {
		t.Fatalf("written: %q", right.Body)
	}
	// An octagon: its header.
	for i := 0; i < 4; i++ {
		press(m, "shift+left")
	}
	if d.sel != left.ID {
		t.Fatalf("back left, the octagon is not chosen (chosen %d)", d.sel)
	}
	press(m, "e")
	if d.editing != partHeader {
		t.Fatalf("E on an octagon: writing in part %d", d.editing)
	}
	press(m, "esc")

	// Alt + arrows move the chosen structure, not the view.
	x, cam := left.X, d.camX
	press(m, "alt+right")
	press(m, "alt+shift+right")
	if left.X != x+5 || d.camX != cam {
		t.Fatalf("Alt + arrows: structure moved %d, view moved %v", left.X-x, d.camX-cam)
	}

	// R is the rubber now.
	press(m, "r")
	if d.pen != rubber {
		t.Fatalf("R: pen %d, want the rubber", d.pen)
	}
	press(m, "esc")

	// + and - zoom, the keypad's too.
	z := d.zoom
	press(m, "=")
	m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyKpPlus}))
	if d.zoom != z+2 {
		t.Fatalf("+ and keypad +: zoom %d, want %d", d.zoom, z+2)
	}
	press(m, "-")
	m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyKpMinus}))
	if d.zoom != z {
		t.Fatalf("- and keypad -: zoom %d, want %d", d.zoom, z)
	}

	// AltGr + K: the keys, at Design mode's; again closes them.
	m.Update(tea.KeyPressMsg(tea.Key{Code: 'ĸ', Text: "ĸ"}))
	if !m.help || m.helpTop != helpLineOf("Design mode") || helpLineOf("Design mode") == 0 {
		t.Fatalf("AltGr + K: help %v at line %d, Design mode's keys at %d", m.help, m.helpTop, helpLineOf("Design mode"))
	}
	if s := screenOf(m); !strings.Contains(s, "Design mode") || !strings.Contains(s, "move the view") {
		t.Fatalf("the keys page does not show Design mode's keys:\n%s", s)
	}
	m.Update(tea.KeyPressMsg(tea.Key{Code: 'ĸ', Text: "ĸ"}))
	if m.help {
		t.Fatal("AltGr + K again did not close the keys")
	}
}

// AltGr + K in Text mode opens the keys at the top.
func TestKeysPageInText(t *testing.T) {
	m := newTest(t, "hello")
	m.Update(tea.KeyPressMsg(tea.Key{Code: 'ĸ', Text: "ĸ"}))
	if !m.help || m.helpTop != 0 {
		t.Fatalf("AltGr + K in Text mode: help %v at %d", m.help, m.helpTop)
	}
	press(m, "esc")
	if m.help || string(m.buf.Lines[0]) != "hello" {
		t.Fatal("Esc did not close the keys, or the text changed")
	}
}

// Two arrows held together move the view (or, with Alt, the chosen
// structure) diagonally: in kitty, which says when keys are let go, and in
// other terminals, where two quick presses count as held together.
func TestDesignDiagonals(t *testing.T) {
	m := designTest(t)
	d := m.ds()
	d.camX, d.camY, d.zoom = 0, 0, designZoom100
	m.releases = true
	press(m, "up")
	x0, y0 := d.camX, d.camY
	press(m, "right") // Up still held
	if d.camX <= x0 || d.camY >= y0 {
		t.Fatalf("Up held, Right: the view moved %v, %v (want right and up)", d.camX-x0, d.camY-y0)
	}
	m.Update(tea.KeyReleaseMsg(tea.Key{Code: tea.KeyUp}))
	x0, y0 = d.camX, d.camY
	press(m, "right")
	if d.camX <= x0 || d.camY != y0 {
		t.Fatalf("Up let go, Right: the view moved %v, %v (want right only)", d.camX-x0, d.camY-y0)
	}
	m.Update(tea.KeyReleaseMsg(tea.Key{Code: tea.KeyRight}))

	// Alt: the chosen structure goes diagonally.
	o := d.newKind(kOct, 0, 0)
	d.sel, d.editing = o.ID, partNone
	x, y := o.X, o.Y
	press(m, "alt+down")
	press(m, "alt+left") // Down still held
	if o.X != x-1 || o.Y != y+2 {
		t.Fatalf("Alt + Down held, Alt + Left: moved %d, %d (want -1, 2)", o.X-x, o.Y-y)
	}
	m.Update(tea.KeyReleaseMsg(tea.Key{Code: tea.KeyDown}))
	m.Update(tea.KeyReleaseMsg(tea.Key{Code: tea.KeyLeft}))

	// Without key releases: two arrows pressed together.
	m.releases = false
	d.lastArrow = ""
	press(m, "left")
	x0, y0 = d.camX, d.camY
	press(m, "down")
	if d.camX >= x0 || d.camY <= y0 {
		t.Fatalf("Left then Down at once: the view moved %v, %v (want left and down)", d.camX-x0, d.camY-y0)
	}
}

// Tab on a chosen structure goes from part to part: header, text, footer
// (Shift+Tab back); on a cabinet it pulls out the next stack.
func TestDesignTab(t *testing.T) {
	m := designTest(t)
	d := m.ds()
	o := d.newKind(kOct, 0, 0)
	d.sel, d.editing = o.ID, partNone
	for _, want := range []int{partHeader, partBody, partFooter, partHeader} {
		press(m, "tab")
		if d.editing != want {
			t.Fatalf("Tab: writing in part %d, want %d", d.editing, want)
		}
	}
	press(m, "shift+tab")
	if d.editing != partFooter {
		t.Fatalf("Shift+Tab: part %d, want the footer", d.editing)
	}
	press(m, "esc")
	press(m, "shift+tab")
	if d.editing != partFooter {
		t.Fatalf("Shift+Tab on a chosen structure: part %d, want its last (the footer)", d.editing)
	}
	press(m, "esc")

	cab := d.newKind(kCabinet, 0, 0)
	d.sel, d.editing = cab.ID, partNone
	cab.OpenStack = ""
	var seen []string
	for range stackKinds {
		press(m, "tab")
		seen = append(seen, cab.OpenStack)
	}
	if strings.Join(seen, ",") != strings.Join(stackKinds, ",") {
		t.Fatalf("Tab on a cabinet pulled out %v, want each stack in turn %v", seen, stackKinds)
	}
}

// A structure grows to show all it holds while you write (wider, then
// taller, no higher than the screen), the view follows, and F fits it
// exactly; a picture keeps its shape.
func TestDesignFitToContents(t *testing.T) {
	m := designTest(t)
	d := m.ds()
	d.camX, d.camY, d.zoom = 0, 0, designZoom100
	o := d.newKind(kOct, 0, 0)
	o.X, o.Y = 10, 2
	w0, h0 := o.W, o.H
	press(m, "esc")
	d.sel = o.ID
	press(m, "tab")
	press(m, "tab") // its text
	typeKeys(m, strings.Repeat("words that go on ", 12))
	if o.W <= w0 || o.W > fitMaxW {
		t.Fatalf("while writing it did not grow wider (to at most %d): %d → %d", fitMaxW, w0, o.W)
	}
	if o.H <= h0 {
		t.Fatalf("a long text did not make it taller: %d → %d", h0, o.H)
	}
	m.View()
	if x, y, w, h := d.rect(o); x < 0 || y < 0 || x+w > m.width || y+h > m.textHeight() {
		t.Fatalf("it grew off the screen: %d,%d %d×%d on a %d×%d screen", x, y, w, h, m.width, m.textHeight())
	}
	if s := screenOf(m); !strings.Contains(s, "words that go on") || strings.Contains(s, "…") {
		t.Fatalf("not all of the text shows:\n%s", s)
	}
	press(m, "esc")

	// F: fitted exactly, smaller too, once the text is short.
	o.Body = "Short"
	press(m, "f")
	if o.W >= fitMaxW || o.H != octMinH {
		t.Fatalf("F after the text got short: %d × %d", o.W, o.H)
	}

	// A page grows no higher than the screen; beyond, it scrolls.
	pg := d.newKind(kPage, 0, 0)
	d.editing = partBody
	for i := 0; i < 60; i++ {
		typeKeys(m, "line")
		press(m, "enter")
	}
	press(m, "esc")
	if pg.H > m.boardRows() || pg.H < m.boardRows()-1 {
		t.Fatalf("a long page is %d rows high, want about the screen's %d", pg.H, m.boardRows())
	}

	// A picture-only octagon fitted keeps the picture's shape (40×20 pixels:
	// twice as wide as high, a cell being two pixels high).
	os.MkdirAll(d.imagesDir(), 0o700)
	pic := d.newKind(kOct, 0, 0)
	pic.Image, pic.Layout = filepath.Base(writePNG(t, d.imagesDir())), layoutPicture
	d.sel, d.editing = pic.ID, partNone
	press(m, "f")
	cw, rows := pic.W-4, pic.H-6
	if want := cw * 20 / 40 / 2; abs(rows-want) > 1 {
		t.Fatalf("picture fitted to %d rows for %d columns, want about %d", rows, cw, want)
	}
}

// An arrow held glides the view on at once, no pause, until it is let go;
// a quick tap is one step; with Alt a held arrow glides the chosen
// structure, and the glide is one undo step.
func TestDesignGlide(t *testing.T) {
	m := designTest(t)
	d := m.ds()
	d.camX, d.camY, d.zoom = 0, 0, designZoom100
	m.releases = true
	glide := func(n int) {
		for i := 0; i < n; i++ {
			_, cmd := m.Update(designHoldMsg{})
			_ = cmd
		}
	}

	// Press Right: one step now, and a glide started.
	_, cmd := m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyRight}))
	if d.camX != 4 || cmd == nil || !d.holdTick {
		t.Fatalf("Right pressed: view at %v (want 4), glide started %v", d.camX, d.holdTick)
	}
	// The terminal's own repeats do nothing: the glide moves instead.
	m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyRight, IsRepeat: true}))
	if d.camX != 4 {
		t.Fatalf("a repeat moved the view: %v", d.camX)
	}
	glide(5) // still held: five glide steps
	if d.camX != 24 {
		t.Fatalf("held for 5 glide steps: view at %v, want 24", d.camX)
	}
	m.Update(tea.KeyReleaseMsg(tea.Key{Code: tea.KeyRight}))
	glide(1) // let go: the glide ends
	if d.camX != 24 || d.holdTick {
		t.Fatalf("let go: view at %v, glide still going %v", d.camX, d.holdTick)
	}

	// A tap (let go before the glide's first step) is one step.
	m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyRight}))
	m.Update(tea.KeyReleaseMsg(tea.Key{Code: tea.KeyRight}))
	glide(1)
	if d.camX != 28 {
		t.Fatalf("a tap: view at %v, want 28", d.camX)
	}

	// Two held: a diagonal glide.
	m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyDown}))
	m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyLeft}))
	x, y := d.camX, d.camY
	glide(2)
	if d.camX >= x || d.camY <= y {
		t.Fatalf("Down and Left held: the view glided %v, %v", d.camX-x, d.camY-y)
	}
	m.Update(tea.KeyReleaseMsg(tea.Key{Code: tea.KeyDown}))
	m.Update(tea.KeyReleaseMsg(tea.Key{Code: tea.KeyLeft}))
	glide(1)

	// Alt held with an arrow: the chosen structure glides; one undo step.
	o := d.newKind(kOct, 0, 0)
	d.sel, d.editing = o.ID, partNone
	d.dirty = true
	d.save()
	ox := o.X
	m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyRight, Mod: tea.ModAlt}))
	glide(3)
	m.Update(tea.KeyReleaseMsg(tea.Key{Code: tea.KeyRight, Mod: tea.ModAlt}))
	glide(1)
	if o.X != ox+4 {
		t.Fatalf("Alt + Right held for 3 glide steps: moved %d, want 4", o.X-ox)
	}
	press(m, "ctrl+z")
	if got := d.byID(o.ID); got == nil || got.X != ox {
		t.Fatalf("one Ctrl+Z did not take back the whole glide: x %v", got)
	}
}

// All text on the board is centred: an octagon's text, a page's title and
// text, a cabinet's name and the things in a drawer.
func TestDesignTextCentred(t *testing.T) {
	m := designTest(t)
	d := m.ds()
	d.camX, d.camY, d.zoom = 0, 0, designZoom100
	o := d.newKind(kOct, 0, 0)
	o.X, o.Y, o.W, o.H, o.Header, o.Body = 2, 1, 30, 10, "Head", "short\na longer line of words"
	pg := d.newKind(kPage, 0, 0)
	pg.X, pg.Y, pg.W, pg.H, pg.Header, pg.Body = 40, 1, 30, 10, "Plan", "one\nthree words here"
	d.sel, d.editing = -1, partNone
	rows := strings.Split(screenOf(m), "\n")
	centred := func(word string, left, right int) {
		t.Helper()
		for _, r := range rows {
			rr := []rune(r)
			at := strings.Index(r, word)
			if at < 0 || right > len(rr) {
				continue
			}
			i := len([]rune(r[:at]))
			gapL := i - left
			gapR := right - (i + len([]rune(word)))
			if abs(gapL-gapR) > 1 {
				t.Fatalf("%q is not centred between columns %d and %d (%d on the left, %d on the right):\n%s", word, left, right, gapL, gapR, r)
			}
			return
		}
		t.Fatalf("%q not on the screen", word)
	}
	ox, _, ow, _ := d.rect(o)
	centred("short", ox+2, ox+ow-2)
	centred("a longer line of words", ox+2, ox+ow-2)
	px, _, pw, _ := d.rect(pg)
	centred("≡ Plan", px+2, px+pw-2)
	centred("one", px+2, px+pw-2)
	centred("three words here", px+2, px+pw-2)
}

// The wheel zooms, also over the chosen page (toward the pointer); Shift +
// the wheel scrolls the chosen page's text.
func TestDesignWheelZooms(t *testing.T) {
	m := designTest(t)
	d := m.ds()
	d.camX, d.camY, d.zoom = 0, 0, designZoom100
	pg := d.newKind(kPage, 0, 0)
	pg.X, pg.Y, pg.W, pg.H = 10, 2, 30, 10
	pg.Body = strings.Repeat("line\n", 40)
	d.sel, d.editing = pg.ID, partNone
	m.View()
	z := d.zoom
	m.Update(tea.MouseWheelMsg(tea.Mouse{X: 20, Y: 6 + textTop, Button: tea.MouseWheelUp}))
	if d.zoom != z+1 || pg.Scroll != 0 {
		t.Fatalf("the wheel over the chosen page: zoom %d (want %d), scroll %d", d.zoom, z+1, pg.Scroll)
	}
	m.Update(tea.MouseWheelMsg(tea.Mouse{X: 20, Y: 6 + textTop, Button: tea.MouseWheelDown}))
	m.View()
	x, y, _, _ := d.rect(pg)
	m.Update(tea.MouseWheelMsg(tea.Mouse{X: x + 5, Y: y + 5 + textTop, Button: tea.MouseWheelDown, Mod: tea.ModShift}))
	if pg.Scroll == 0 || d.zoom != z {
		t.Fatalf("Shift + wheel over the chosen page: scroll %d, zoom %d", pg.Scroll, d.zoom)
	}
}
