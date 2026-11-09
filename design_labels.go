package main

// ---- labels and name plates --------------------------------------------------------
//
// Alt+L turns label mode on (and off; Alt tapped twice or Esc turn it off
// too). In label mode, on the chosen structure:
//
//	L  its label: icon + title → title + icon → icon → title → none
//	C  the label's colour, round a set
//	I  the label's icon: symbols, then the pictures in design/images
//	T  the label's own words (else the structure's title)
//	G  its name plate: everything with the same name plate is a group
//
// A label sits on top of the structure's top edge, at its left; a picture
// icon is drawn small in half cells. A name plate sits under the bottom
// edge. AltGr+P lists the name plates and what has each.

import (
	"hash/fnv"
	"image"
	"image/color"
	"os"
	"path/filepath"
	"sort"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"
)

const (
	labelNone = iota
	labelIconTitle
	labelTitleIcon
	labelIcon
	labelTitle
	labelStyles
)

var labelStyleNames = []string{"no label", "icon + title", "title + icon", "icon", "title"}

// labelIcons are the symbols I goes round (all one cell wide in Agave).
var labelIcons = []string{"★", "●", "◆", "■", "▲", "♥", "✓", "☺", "♪", "☀", "☁", "▶", "→"}

// labelColors are the colours C goes round.
var labelColors = []string{"#e5c07b", "#e06c75", "#98c379", "#61afef", "#c678dd", "#56b6c2", "#d19a66", "#abb2bf"}

// picIconW, picIconH: a picture icon's size in cells.
const picIconW, picIconH = 4, 2

// designLabelMode turns label mode on or off.
func (m *Model) designLabelMode() {
	d := m.ds()
	d.labelMode = !d.labelMode
	d.dirty = true
	if d.labelMode {
		d.editing = partNone
		m.say("Label mode: L label · C colour · I icon · T words · G name plate · Alt twice: leave")
	} else {
		m.say("Label mode off.")
	}
}

// designLabelKey handles a key in label mode. It says whether it did.
func (m *Model) designLabelKey(name string) (bool, tea.Cmd) {
	d := m.ds()
	o := d.byID(d.sel)
	switch strings.ToLower(strings.TrimPrefix(name, "shift+")) {
	case "l", "c", "i", "t", "g":
	default:
		return false, nil
	}
	if o == nil {
		m.say("Label mode: choose a structure first (the arrows, or a click).")
		return true, nil
	}
	d.dirty = true
	switch strings.ToLower(strings.TrimPrefix(name, "shift+")) {
	case "l":
		o.LabelStyle = (o.LabelStyle + 1) % labelStyles
		if o.LabelStyle != labelNone {
			if o.LabelIcon == "" {
				o.LabelIcon = labelIcons[0]
			}
			if o.LabelColor == "" {
				o.LabelColor = labelColors[0]
			}
		}
		m.say("Label: " + labelStyleNames[o.LabelStyle] + ".")
	case "c":
		o.LabelColor = labelColors[(strIndex(labelColors, o.LabelColor)+1)%len(labelColors)]
		if o.LabelStyle == labelNone {
			o.LabelStyle = labelIconTitle
		}
	case "i":
		icons := append(append([]string{}, labelIcons...), d.pictureIcons()...)
		o.LabelIcon = icons[(strIndex(icons, o.LabelIcon)+1)%len(icons)]
		if o.LabelStyle == labelNone || o.LabelStyle == labelTitle {
			o.LabelStyle = labelIconTitle
		}
		if o.LabelColor == "" {
			o.LabelColor = labelColors[0]
		}
		if strings.HasPrefix(o.LabelIcon, "img:") {
			m.say("Label icon: the picture " + strings.TrimPrefix(o.LabelIcon, "img:") + ".")
		}
	case "t":
		return true, m.ask(pDesignLabel, "Label words for "+octName(o)+" (empty: its title): ", o.LabelTitle)
	case "g":
		return true, m.ask(pDesignPlate, "Name plate for "+octName(o)+" (empty: none): ", o.Plate)
	}
	return true, nil
}

func strIndex(list []string, s string) int {
	for i, v := range list {
		if v == s {
			return i
		}
	}
	return -1
}

// pictureIcons are the pictures in the board's images folder, as icons.
func (d *designState) pictureIcons() []string {
	ents, err := os.ReadDir(d.imagesDir())
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range ents {
		if !e.IsDir() {
			out = append(out, "img:"+e.Name())
		}
	}
	sort.Strings(out)
	return out
}

// designSetLabelWords and designSetPlate take what was typed at the prompt.
func (m *Model) designSetLabelWords(s string) {
	d := m.ds()
	if o := d.byID(d.sel); o != nil {
		o.LabelTitle = strings.TrimSpace(s)
		if o.LabelStyle == labelNone || o.LabelStyle == labelIcon {
			o.LabelStyle = labelIconTitle
		}
		if o.LabelIcon == "" {
			o.LabelIcon = labelIcons[0]
		}
		if o.LabelColor == "" {
			o.LabelColor = labelColors[0]
		}
		d.dirty = true
		d.save()
	}
}

func (m *Model) designSetPlate(s string) {
	d := m.ds()
	if o := d.byID(d.sel); o != nil {
		o.Plate = strings.TrimSpace(s)
		d.dirty = true
		d.save()
		if o.Plate == "" {
			m.say(octName(o) + " has no name plate now.")
			return
		}
		n := len(d.plates()[o.Plate])
		m.say(octName(o) + " is in the group “" + o.Plate + "” (" + plural(n, "structure") + "). AltGr+P lists them.")
	}
}

func plural(n int, what string) string {
	if n == 1 {
		return "1 " + what
	}
	return itoa(n) + " " + what + "s"
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	if neg {
		b = append([]byte{'-'}, b...)
	}
	return string(b)
}

// plates are the name plates on the board (in containers too) and what has each.
func (d *designState) plates() map[string][]*dOct {
	out := map[string][]*dOct{}
	var walk func(os []*dOct)
	walk = func(os []*dOct) {
		for _, o := range os {
			if o == nil {
				continue
			}
			if o.Plate != "" {
				out[o.Plate] = append(out[o.Plate], o)
			}
			walk(o.Items)
		}
	}
	walk(d.octs)
	return out
}

// plateColor is a name plate's colour: the same name, the same colour.
func plateColor(name string) string {
	h := fnv.New32a()
	h.Write([]byte(name))
	return hslHex(float64(h.Sum32()%360), 0.45, 0.62)
}

// labelWords is what a label says: its own words, else the structure's title.
func labelWords(o *dOct) string {
	if o.LabelTitle != "" {
		return o.LabelTitle
	}
	return itemTitle(o)
}

// ---- drawing labels and plates ----------------------------------------------------------

// paintLabels draws each structure's label (on its top edge) and name
// plate (under it) onto the canvas, over the structures.
func (m *Model) paintLabels(c *boardCanvas) {
	d := m.ds()
	cw, ch := c.Width(), c.Height()
	put := func(x, y int, s string, fg, bg string, bold bool) {
		if x < 0 || y < 0 || x >= cw || y >= ch {
			return
		}
		st := uv.Style{Fg: rgbaOf(fg), Bg: rgbaOf(bg)}
		if bold {
			st.Attrs = uv.AttrBold
		}
		c.Lines[y][x] = uv.Cell{Content: s, Width: 1, Style: st}
	}
	text := func(x, y int, s, fg, bg string, bold bool) int {
		for _, r := range s {
			put(x, y, string(r), fg, bg, bold)
			x++
		}
		return x
	}
	for _, o := range d.octs {
		if o == nil || (o.LabelStyle == labelNone && o.Plate == "") || !d.onScreen(o, cw, ch) {
			continue
		}
		x, y, w, h := d.rect(o)
		if o.LabelStyle != labelNone {
			col := o.LabelColor
			if col == "" {
				col = labelColors[0]
			}
			fg := mixHex(col, "#101010", 0.82)
			words := ansi.Truncate(labelWords(o), max(4, w-6), "…")
			pic := strings.HasPrefix(o.LabelIcon, "img:")
			icon := o.LabelIcon
			if icon == "" {
				icon = labelIcons[0]
			}
			showIcon := o.LabelStyle != labelTitle
			showWords := o.LabelStyle != labelIcon
			lx, ly := x+1, y // on the top edge, at the left
			if pic && showIcon {
				ly = y - picIconH + 1 // a picture is taller: it rises above the edge
			}
			cx := lx
			drawIcon := func() {
				if pic {
					m.paintPicIcon(c, cx, ly, strings.TrimPrefix(icon, "img:"), col)
					cx += picIconW
				} else {
					cx = text(cx, ly+boolInt(pic)*(picIconH-1), " "+icon, fg, col, true)
				}
			}
			row := ly + boolInt(pic)*(picIconH-1) // the words' row: the edge
			if o.LabelStyle == labelTitleIcon {
				if showWords {
					cx = text(cx, row, " "+words, fg, col, true)
				}
				if showIcon {
					drawIcon()
				}
			} else {
				if showIcon {
					drawIcon()
				}
				if showWords {
					cx = text(cx, row, " "+words, fg, col, true)
				}
			}
			text(cx, row, " ", fg, col, false)
		}
		if o.Plate != "" {
			col := plateColor(o.Plate)
			name := " ▪ " + ansi.Truncate(o.Plate, max(4, w-6), "…") + " "
			px := x + max(0, (w-lipgloss.Width(name))/2)
			text(px, y+h, name, mixHex(col, "#101010", 0.85), col, true)
		}
	}
}

// paintPicIcon draws the picture file (in design/images) picIconW × picIconH
// cells big at (x, y), in half cells; bg shows where it is clear.
func (m *Model) paintPicIcon(c *boardCanvas, x, y int, file, bg string) {
	d := m.ds()
	img, ok := loadPicture(filepath.Join(d.imagesDir(), file))
	if !ok {
		return
	}
	b := img.Bounds()
	if b.Dx() <= 0 || b.Dy() <= 0 {
		return
	}
	pix := func(cx, sub int) string {
		px := b.Min.X + (2*cx+1)*b.Dx()/(2*picIconW)
		py := b.Min.Y + (2*sub+1)*b.Dy()/(2*picIconH*2)
		return colorHex(colorAt(img, px, py), bg)
	}
	for cy := 0; cy < picIconH; cy++ {
		for cx := 0; cx < picIconW; cx++ {
			if x+cx < 0 || y+cy < 0 || x+cx >= c.Width() || y+cy >= c.Height() {
				continue
			}
			top, bot := pix(cx, cy*2), pix(cx, cy*2+1)
			c.Lines[y+cy][x+cx] = uv.Cell{Content: "▀", Width: 1, Style: uv.Style{Fg: rgbaOf(top), Bg: rgbaOf(bot)}}
		}
	}
}

// colorAt is the colour of img at (x, y), or nil where it is clear.
func colorAt(img image.Image, x, y int) color.Color {
	c := img.At(x, y)
	if _, _, _, a := c.RGBA(); a < 0x4000 {
		return nil
	}
	return c
}

// ---- the list of name plates (AltGr+P) -------------------------------------------------

// designPlatesToggle shows or hides the list of name plates.
func (m *Model) designPlatesToggle() {
	d := m.ds()
	d.platesOpen = !d.platesOpen
	d.dirty = true
	if d.platesOpen && len(d.plates()) == 0 {
		m.say("No name plates yet: in label mode (Alt+L), G gives the chosen structure one.")
	}
}

// paintPlates draws the list of name plates in the middle of the board.
func (m *Model) paintPlates(c *boardCanvas) {
	d := m.ds()
	if !d.platesOpen {
		return
	}
	plates := d.plates()
	names := make([]string, 0, len(plates))
	for n := range plates {
		names = append(names, n)
	}
	sort.Strings(names)
	var lines [][2]string // name, members
	for _, n := range names {
		var who []string
		for _, o := range plates[n] {
			who = append(who, kindIcons[o.Kind]+" "+itemTitle(o))
		}
		lines = append(lines, [2]string{n, strings.Join(who, ", ")})
	}
	cw, ch := c.Width(), c.Height()
	bw := min(cw-4, 76)
	bh := min(ch-2, max(len(lines), 1)+4)
	x0, y0 := (cw-bw)/2, (ch-bh)/2
	bg := mixHex(cBG, cBarBG, 0.7)
	edge := mixHex(cDim, cBG, 0.1)
	put := func(x, y int, s string, fg, back string, bold bool) {
		if x < 0 || y < 0 || x >= cw || y >= ch {
			return
		}
		st := uv.Style{Fg: rgbaOf(fg), Bg: rgbaOf(back)}
		if bold {
			st.Attrs = uv.AttrBold
		}
		c.Lines[y][x] = uv.Cell{Content: s, Width: 1, Style: st}
	}
	text := func(x, y int, s, fg, back string, bold bool) int {
		for _, r := range s {
			if x >= x0+bw-1 {
				break
			}
			put(x, y, string(r), fg, back, bold)
			x++
		}
		return x
	}
	for y := 0; y < bh; y++ {
		for x := 0; x < bw; x++ {
			s := " "
			switch {
			case y == 0 && x == 0:
				s = "╭"
			case y == 0 && x == bw-1:
				s = "╮"
			case y == bh-1 && x == 0:
				s = "╰"
			case y == bh-1 && x == bw-1:
				s = "╯"
			case y == 0 || y == bh-1:
				s = "─"
			case x == 0 || x == bw-1:
				s = "│"
			}
			put(x0+x, y0+y, s, edge, bg, false)
		}
	}
	title := " Name plates · AltGr+P closes "
	text(x0+(bw-len([]rune(title)))/2, y0, title, mixHex(cDim, cText, 0.5), bg, true)
	if len(lines) == 0 {
		msg := "none yet: label mode (Alt+L), then G"
		text(x0+(bw-len([]rune(msg)))/2, y0+2, msg, mixHex(cDim, cText, 0.3), bg, false)
		return
	}
	for i, l := range lines {
		y := y0 + 2 + i
		if y >= y0+bh-1 {
			break
		}
		col := plateColor(l[0])
		x := text(x0+2, y, " ▪ "+ansi.Truncate(l[0], 18, "…")+" ", mixHex(col, "#101010", 0.85), col, true)
		text(x+1, y, ansi.Truncate(l[1], max(0, x0+bw-2-(x+1)), "…"), mixHex(cText, bg, 0.1), bg, false)
	}
}
