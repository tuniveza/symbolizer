package main

import (
	"image"
	"image/color"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
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

	// A picture is refused by a page.
	press(m, "esc")
	d.sel = pg.ID
	m.Update(tea.PasteMsg{Content: writePNG(t, t.TempDir())})
	if !m.msgErr || !strings.Contains(m.msg, "only text") || pg.Image != "" {
		t.Fatalf("page took a picture: %q", m.msg)
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
	for _, w := range []string{"▦ Archive", "Pages", "Books", "Slides", "Notes", "Drawings"} {
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
