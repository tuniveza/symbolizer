package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	hexenc "encoding/hex"
	"fmt"
	"image"
	"image/color"
	_ "image/gif"
	_ "image/jpeg"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"
)

// Pictures in octagons, and the octagons' layouts.
//
// A picture of any format goes in with I (a file name), or by dropping the
// file on the terminal. PNG, JPEG and GIF are read here; anything else
// (WebP, AVIF, HEIC, BMP, TIFF, SVG, ICO, PSD, RAW, a video's first frame…)
// is turned into a PNG by ImageMagick, rsvg-convert or ffmpeg when the
// computer has them. The picture is copied (made smaller, as a PNG) to
//
//	~/.local/share/symbolizer/design/images/
//
// so the board keeps it when the original moves. It is drawn with ▀
// half-blocks: each letter cell shows two pixels, one above the other.

// octLayouts are the ways an octagon lays out its middle. The toggle on
// its top edge shows the icon of the one in use; a click shifts it on.
var octLayouts = []struct{ icon, name string }{
	{"¶", "text"},
	{"▀", "picture above the text"},
	{"◧", "picture on the left, text on the right"},
	{"◨", "text on the left, picture on the right"},
	{"■", "picture only"},
	{"‖", "text in two columns"},
}

const (
	layoutText = iota
	layoutAbove
	layoutLeft
	layoutRight
	layoutPicture
	layoutColumns
)

// layoutToggleSpot is where an octagon's layout toggle is: on its top edge,
// near the right corner. Small octagons have none.
func layoutToggleSpot(x, y, w, h int) (int, int, bool) {
	if w < octMinW || h < octMinH {
		return 0, 0, false
	}
	return x + w - 7, y, true
}

func (m *Model) designLayout(o *dOct, step int) {
	n := len(octLayouts)
	o.Layout = ((o.Layout+step)%n + n) % n
	m.ds().dirty = true
	l := octLayouts[o.Layout]
	msg := "Layout " + l.icon + " " + l.name + "."
	if o.Image == "" && o.Layout != layoutText && o.Layout != layoutColumns {
		msg += " No picture yet: I adds one (or drop a file on the terminal)."
	}
	m.say(msg)
}

// ---- the middle of an octagon -------------------------------------------------------

// octContent is an octagon's middle, cw wide and rows high, in its layout.
func (d *designState) octContent(o *dOct, editing, cw, rows int, fill, dim string) string {
	if rows <= 0 || cw <= 0 {
		return ""
	}
	switch o.Layout % len(octLayouts) {
	case layoutPicture:
		return d.pictureAlone(o, cw, rows, fill, dim)
	case layoutAbove:
		if rows < 3 {
			return d.pictureAlone(o, cw, rows, fill, dim)
		}
		ir := rows / 2
		if o.Body == "" && editing != partBody {
			ir = rows - 1
		}
		return d.pictureAlone(o, cw, ir, fill, dim) + "\n" + fitBlock(d.octBody(o, editing, cw, rows-ir, fill, dim), cw, rows-ir, fill)
	case layoutLeft, layoutRight:
		if cw < 9 {
			return d.octBody(o, editing, cw, rows, fill, dim)
		}
		iw := cw / 2
		tw := cw - iw - 1
		spots := len(d.picSpots)
		pic := strings.Split(d.pictureArea(o, iw, rows, fill, dim), "\n")
		if o.Layout%len(octLayouts) == layoutRight {
			for i := spots; i < len(d.picSpots); i++ {
				d.picSpots[i].dx += tw + 1
			}
		}
		txt := strings.Split(fitBlock(d.octBody(o, editing, tw, rows, fill, dim), tw, rows, fill), "\n")
		gap := st(cText, fill).Render(" ")
		out := make([]string, rows)
		for i := range rows {
			if o.Layout%len(octLayouts) == layoutLeft {
				out[i] = pic[i] + gap + txt[i]
			} else {
				out[i] = txt[i] + gap + pic[i]
			}
		}
		return strings.Join(out, "\n")
	case layoutColumns:
		return d.octColumns(o, editing, cw, rows, fill, dim)
	}
	return d.octBody(o, editing, cw, rows, fill, dim)
}

// octColumns is the text in two columns: it runs down the left one, then
// the right one.
func (d *designState) octColumns(o *dOct, editing, cw, rows int, fill, dim string) string {
	colW := (cw - 3) / 2
	if colW < 4 {
		return d.octBody(o, editing, cw, rows, fill, dim)
	}
	if o.Body == "" && editing != partBody {
		return fitBlock(st(dim, fill).Italic(true).Render(ansi.Truncate("double-click to write", cw, "…")), cw, rows, fill)
	}
	lines := strings.Split(ansi.Wrap(o.Body, colW, ""), "\n")
	if editing == partBody {
		if lipgloss.Width(lines[len(lines)-1]) >= colW {
			lines = append(lines, "")
		}
		if len(lines) > 2*rows {
			lines = lines[len(lines)-2*rows:]
		}
	} else if len(lines) > 2*rows {
		lines = lines[:2*rows]
		lines[2*rows-1] = ansi.Truncate(lines[2*rows-1], colW-1, "") + "…"
	}
	text := st(cText, fill)
	cell := func(i int) string {
		if i >= len(lines) {
			return fitBlockLine("", colW, fill)
		}
		s := text.Render(lines[i])
		if editing == partBody && i == len(lines)-1 {
			s += st(cAccent, fill).Render("▏")
		}
		return fitBlockLine(s, colW, fill)
	}
	sep := st(mixHex(cDim, fill, 0.3), fill).Render(" │ ")
	out := make([]string, rows)
	for i := range rows {
		out[i] = fitBlockLine(cell(i)+sep+cell(rows+i), cw, fill)
	}
	return strings.Join(out, "\n")
}

// fitBlock makes drawn text exactly w wide and h high.
func fitBlock(s string, w, h int, fill string) string {
	lines := strings.Split(s, "\n")
	out := make([]string, h)
	for i := range h {
		l := ""
		if i < len(lines) {
			l = lines[i]
		}
		out[i] = fitBlockLine(l, w, fill)
	}
	return strings.Join(out, "\n")
}

// pictureAlone is pictureArea on rows of its own. When the structure's
// middle is built as cells (middleCells), its hole can be empty lines: the
// middle is filled already, and there is nothing to read.
func (d *designState) pictureAlone(o *dOct, w, h int, fill, dim string) string {
	spots := len(d.picSpots)
	s := d.pictureArea(o, w, h, fill, dim)
	if d.emptyHoles && len(d.picSpots) > spots {
		d.holesLeft = true
		return strings.Repeat("\n", max(h-1, 0))
	}
	return s
}

// pictureArea is the octagon's picture, w×h cells, or a place for one.
func (d *designState) pictureArea(o *dOct, w, h int, fill, dim string) string {
	if o.Image != "" {
		path := filepath.Join(d.imagesDir(), o.Image)
		if picturesDirect && !d.picsAsText {
			if _, ok := loadPicture(path); ok {
				// A hole the size of the picture; its cells are laid in
				// afterwards, straight from its colours (see picSpot).
				d.picSpots = append(d.picSpots, picSpot{path: path, w: w, h: h, fill: fill})
				return fitBlock("", w, h, fill)
			}
		} else if s, ok := pictureBlock(path, w, h, fill); ok {
			return s
		}
		return fitBlock(st(cError, fill).Render(ansi.Truncate("■ picture missing: "+o.Image, w, "…")), w, h, fill)
	}
	lines := make([]string, h)
	for i := range lines {
		lines[i] = st(mixHex(cDim, fill, 0.55), fill).Render(strings.Repeat("░", w))
	}
	label := ansi.Truncate(" ■ I adds a picture ", w, "")
	lines[h/2] = lipgloss.PlaceHorizontal(w, lipgloss.Center, st(dim, fill).Italic(true).Render(label),
		lipgloss.WithWhitespaceChars("░"), lipgloss.WithWhitespaceStyle(st(mixHex(cDim, fill, 0.55), fill)))
	return strings.Join(lines, "\n")
}

// ---- drawing a picture --------------------------------------------------------------

// picLoaded is whether the picture at path has been read already.
func picLoaded(path string) bool {
	picMu.Lock()
	defer picMu.Unlock()
	_, ok := picFiles[path]
	return ok
}

var (
	picMu    sync.Mutex
	picFiles = map[string]image.Image{} // decoded pictures, by path
	picDrawn = map[string]string{}      // drawn pictures, by path, size and fill
)

// pictureBlock draws the picture at path in w×h cells: it fits inside,
// keeping its shape, centred on fill. Each cell is "▀": its top half is the
// letter's colour, its bottom half the background.
func pictureBlock(path string, w, h int, fill string) (string, bool) {
	key := fmt.Sprintf("%s|%d|%d|%s", path, w, h, fill)
	picMu.Lock()
	defer picMu.Unlock()
	if s, ok := picDrawn[key]; ok {
		return s, true
	}
	img, ok := loadPictureLocked(path)
	if !ok {
		return "", false
	}
	if len(picDrawn) > 300 {
		clear(picDrawn)
	}
	s := drawPicture(img, w, h, fill)
	picDrawn[key] = s
	return s, true
}

// loadPicture reads the picture at path once; later calls reuse it.
func loadPicture(path string) (image.Image, bool) {
	picMu.Lock()
	defer picMu.Unlock()
	return loadPictureLocked(path)
}

func loadPictureLocked(path string) (image.Image, bool) {
	if img, ok := picFiles[path]; ok {
		return img, true
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, false
	}
	img, _, err := image.Decode(f)
	f.Close()
	if err != nil {
		return nil, false
	}
	picFiles[path] = img
	return img, true
}

// picturesDirect: pictures are laid on the board as cells made straight
// from their colours, not as styled text that has to be read back. Zoomed
// in, a picture is hundreds of cells across, and the text way took a tenth
// of a second a frame. (Off only in the test that checks both ways match.)
var picturesDirect = true

// picSpot is a picture waiting to be laid into a structure's drawing: its
// size in cells and where it starts, from the corner of the structure's text.
type picSpot struct {
	path   string
	w, h   int
	fill   string
	dx, dy int
}

var (
	picCells     = map[string]*boardCanvas{} // pictures as cells, by path, size and fill (under picMu)
	picCellsKept int                         // how many cells picCells holds
)

// pictureCanvas is the picture at path as w×h cells, the same cells
// pictureBlock's text would give.
func pictureCanvas(path string, w, h int, fill string) *boardCanvas {
	key := fmt.Sprintf("%s|%d|%d|%s", path, w, h, fill)
	picMu.Lock()
	defer picMu.Unlock()
	if c, ok := picCells[key]; ok {
		return c
	}
	c := newBoardCanvas(w, h)
	if img, ok := loadPictureLocked(path); ok {
		cell := uv.Cell{Content: "▀", Width: 1}
		forPictureCells(img, w, h, fill, func(col, row int, t, bo [3]int) {
			cell.Style.Fg = color.RGBA{uint8(t[0]), uint8(t[1]), uint8(t[2]), 255}
			cell.Style.Bg = color.RGBA{uint8(bo[0]), uint8(bo[1]), uint8(bo[2]), 255}
			c.SetCell(col, row, &cell)
		})
	}
	// A picture zoomed in is hundreds of thousands of cells, about a hundred
	// bytes each: keep at most a million cells (some 100 MB) of them.
	if picCellsKept += w * h; picCellsKept > 1_000_000 {
		clear(picCells)
		picCellsKept = w * h
	}
	picCells[key] = c
	return c
}

func drawPicture(img image.Image, w, h int, fill string) string {
	b := img.Bounds()
	if b.Dx() <= 0 || b.Dy() <= 0 || w <= 0 || h <= 0 {
		return fitBlock("", w, h, fill)
	}
	var sb strings.Builder
	var lastT, lastB [3]int
	forPictureCells(img, w, h, fill, func(col, row int, t, bo [3]int) {
		if col == 0 && row > 0 {
			sb.WriteString("\x1b[0m\n")
		}
		if col == 0 || t != lastT || bo != lastB {
			fmt.Fprintf(&sb, "\x1b[38;2;%d;%d;%d;48;2;%d;%d;%dm", t[0], t[1], t[2], bo[0], bo[1], bo[2])
			lastT, lastB = t, bo
		}
		sb.WriteString("▀")
	})
	sb.WriteString("\x1b[0m")
	return sb.String()
}

// forPictureCells works out each of the w×h cells of the picture fitted
// inside them, keeping its shape, centred on fill: t is the colour of the
// cell's top half, bo of its bottom half. Cells go row by row.
func forPictureCells(img image.Image, w, h int, fill string, cell func(col, row int, t, bo [3]int)) {
	b := img.Bounds()
	iw, ih := b.Dx(), b.Dy()
	pw, ph := w, h*2 // a cell is about twice as tall as wide: two square pixels
	if iw <= 0 || ih <= 0 || pw <= 0 || ph <= 0 {
		return
	}
	scale := min(float64(pw)/float64(iw), float64(ph)/float64(ih))
	tw, th := max(1, int(float64(iw)*scale+0.5)), max(1, int(float64(ih)*scale+0.5))
	ox, oy := (pw-tw)/2, (ph-th)/2
	fr, fg, fb := rgb(fill)
	px := func(x, y int) [3]int {
		x, y = x-ox, y-oy
		if x < 0 || y < 0 || x >= tw || y >= th {
			return [3]int{fr, fg, fb}
		}
		// The average of the picture's pixels under this one, laid on the fill.
		x0, x1 := b.Min.X+x*iw/tw, b.Min.X+max((x+1)*iw/tw, x*iw/tw+1)
		y0, y1 := b.Min.Y+y*ih/th, b.Min.Y+max((y+1)*ih/th, y*ih/th+1)
		stepX, stepY := max(1, (x1-x0)/4), max(1, (y1-y0)/4) // at most 16 samples
		var r, g, bl, n float64
		for sy := y0; sy < y1; sy += stepY {
			for sx := x0; sx < x1; sx += stepX {
				c := color.NRGBAModel.Convert(img.At(sx, sy)).(color.NRGBA)
				a := float64(c.A) / 255
				r += float64(c.R)*a + float64(fr)*(1-a)
				g += float64(c.G)*a + float64(fg)*(1-a)
				bl += float64(c.B)*a + float64(fb)*(1-a)
				n++
			}
		}
		return [3]int{int(r/n + 0.5), int(g/n + 0.5), int(bl/n + 0.5)}
	}
	for row := range h {
		for col := range w {
			cell(col, row, px(col, row*2), px(col, row*2+1))
		}
	}
}

// ---- putting a picture in -------------------------------------------------------------

func (d *designState) imagesDir() string { return filepath.Join(filepath.Dir(d.loadedFrom), "images") }

// designAskFile asks for a file: a picture for the chosen octagon (or a new
// one), a text file for the chosen page or note.
func (m *Model) designAskFile() tea.Cmd {
	d := m.ds()
	label := "Picture (any format) for a new structure: "
	if o := d.byID(d.sel); o != nil {
		switch {
		case o.Kind == kOct:
			label = "Picture (any format) for " + octName(o) + ": "
		case o.Kind == kCabinet:
			label = "Text file to put in " + octName(o) + " as a page: "
		default:
			label = "Text file to put in " + octName(o) + ": "
		}
	}
	return m.ask(pDesignFile, label, "")
}

// designUseFile puts a file where it belongs: a picture into the chosen
// octagon (or a new one), text into the chosen page or note (a new page,
// or a cabinet's Pages stack).
func (m *Model) designUseFile(path string) tea.Cmd {
	d := m.ds()
	o := d.byID(d.sel)
	data, err := os.ReadFile(path)
	if err != nil {
		m.sayErr("Can't read " + filepath.Base(path) + ": " + err.Error())
		return nil
	}
	if isText(data) && !strings.HasSuffix(strings.ToLower(path), ".svg") {
		switch {
		case o == nil || o.Kind == kOct:
			cx, cy := d.world(m.width/2, m.textHeight()/2)
			o = d.newKind(kPage, int(cx), int(cy))
			o.Header = filepath.Base(path)
			o.From = path
			m.designAddText(o, textLike(string(data)))
		case o.Kind == kCabinet:
			m.designPutFrom(d, o, path, filepath.Base(path), string(data))
			m.say(filepath.Base(path) + " put in " + octName(o) + " as a page.")
		default:
			m.designAddText(o, textLike(string(data)))
		}
		d.save()
		return nil
	}
	if o != nil && o.Kind != kOct {
		m.sayErr(kindNames[o.Kind] + "s hold only text and text-like media: " + filepath.Base(path) + " is not text. Pictures go in octagon structures.")
		return nil
	}
	id := -1
	if o != nil {
		id = o.ID
	}
	m.say("Reading the picture " + filepath.Base(path) + "…")
	dir := d.imagesDir()
	return func() tea.Msg {
		name, err := importPicture(path, data, dir)
		return designPictureMsg{id: id, name: name, path: path, err: err}
	}
}

type designPictureMsg struct {
	id   int // the octagon, or -1 for a new one
	name string
	path string
	err  error
}

func (m *Model) designPictureDone(msg designPictureMsg) tea.Cmd {
	if msg.err != nil {
		m.sayErr(msg.err.Error())
		return nil
	}
	d := m.ds()
	o := d.byID(msg.id)
	if o == nil {
		cx, cy := d.world(m.width/2, m.textHeight()/2)
		o = d.newKind(kOct, int(cx), int(cy))
		o.W, o.H = 34, 16
		o.X, o.Y = int(cx)-o.W/2, int(cy)-o.H/2
		d.editing = partNone
		o.Header = strings.TrimSuffix(filepath.Base(msg.path), filepath.Ext(msg.path))
	}
	o.Image = msg.name
	o.W, o.H = max(o.W, 34), max(o.H, 14) // room for the picture to be seen
	if o.Layout == layoutText || o.Layout == layoutColumns {
		o.Layout = layoutAbove
	}
	d.dirty = true
	d.save()
	m.say("Picture " + filepath.Base(msg.path) + " put in " + octName(o) + " · the [" + octLayouts[o.Layout].icon + "] on its top edge (or L) changes the layout.")
	return nil
}

const pictureMaxSide = 480 // pixels kept: plenty for a board of letter cells

// importPicture reads a picture of any format, makes it smaller, and keeps
// it as a PNG in dir. It gives the file's name.
func importPicture(path string, data []byte, dir string) (string, error) {
	img, err := decodeAnyPicture(path, data)
	if err != nil {
		return "", err
	}
	img = shrink(img, pictureMaxSide)
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	name := hexenc.EncodeToString(sum[:8]) + ".png"
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	if err := os.WriteFile(filepath.Join(dir, name), buf.Bytes(), 0o600); err != nil {
		return "", err
	}
	picMu.Lock()
	delete(picFiles, filepath.Join(dir, name))
	picMu.Unlock()
	return name, nil
}

// decodeAnyPicture reads PNG, JPEG and GIF itself; for any other format it
// asks the converters this computer has for a PNG.
func decodeAnyPicture(path string, data []byte) (image.Image, error) {
	if img, _, err := image.Decode(bytes.NewReader(data)); err == nil {
		return img, nil
	}
	tries := [][]string{
		{"magick", path + "[0]", "-auto-orient", "png:-"},
		{"convert", path + "[0]", "-auto-orient", "png:-"},
		{"ffmpeg", "-v", "error", "-i", path, "-frames:v", "1", "-f", "image2pipe", "-c:v", "png", "-"},
	}
	if strings.HasSuffix(strings.ToLower(path), ".svg") || strings.HasSuffix(strings.ToLower(path), ".svgz") {
		tries = append([][]string{{"rsvg-convert", "-w", "480", path}}, tries...)
	}
	found := false
	for _, try := range tries {
		if _, err := exec.LookPath(try[0]); err != nil {
			continue
		}
		found = true
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		out, err := exec.CommandContext(ctx, try[0], try[1:]...).Output()
		cancel()
		if err != nil || len(out) == 0 {
			continue
		}
		if img, err := png.Decode(bytes.NewReader(out)); err == nil {
			return img, nil
		}
	}
	if !found {
		return nil, fmt.Errorf("%s: only PNG, JPEG and GIF can be read without ImageMagick (install the imagemagick package)", filepath.Base(path))
	}
	return nil, fmt.Errorf("%s is not a picture this computer can read", filepath.Base(path))
}

// shrink makes a picture at most side pixels across and down (each new
// pixel the average of the ones it covers).
func shrink(img image.Image, side int) image.Image {
	b := img.Bounds()
	iw, ih := b.Dx(), b.Dy()
	if iw <= side && ih <= side {
		return img
	}
	scale := min(float64(side)/float64(iw), float64(side)/float64(ih))
	tw, th := max(1, int(float64(iw)*scale)), max(1, int(float64(ih)*scale))
	out := image.NewNRGBA(image.Rect(0, 0, tw, th))
	for y := range th {
		y0, y1 := b.Min.Y+y*ih/th, b.Min.Y+max((y+1)*ih/th, y*ih/th+1)
		for x := range tw {
			x0, x1 := b.Min.X+x*iw/tw, b.Min.X+max((x+1)*iw/tw, x*iw/tw+1)
			var r, g, bl, a, n uint64
			for sy := y0; sy < y1; sy++ {
				for sx := x0; sx < x1; sx++ {
					c := color.NRGBAModel.Convert(img.At(sx, sy)).(color.NRGBA)
					r, g, bl, a, n = r+uint64(c.R), g+uint64(c.G), bl+uint64(c.B), a+uint64(c.A), n+1
				}
			}
			out.SetNRGBA(x, y, color.NRGBA{uint8(r / n), uint8(g / n), uint8(bl / n), uint8(a / n)})
		}
	}
	return out
}
