package main

// ---- Sy Geometry, the font file ----------------------------------------------------------
//
// sy --font writes the shape letters of design_geo.go as a TrueType font,
// SyGeometry.ttf, in the user's fonts folder, and tells kitty to draw
// U+FB000 on with it. The font is made to fit the terminal's own font:
// the same units to the em, the same ascent and descent, and every letter
// as wide as the terminal font's M, so a letter fills its cell.
//
// How kitty (0.50) sizes a cell: its height is the font's height (ascent
// + descent + line gap, in pixels, rounded up) and the baseline is the
// ascent, rounded up; its width is the letters' width, rounded up. A
// letter is put on the baseline and cut off at the cell's bottom and
// right side, but one that reaches above the cell's top is moved down,
// and one more than two pixels wider than the cell is drawn smaller. So
// a shape's top is the ascent exactly and its left side the cell's, and
// its bottom and right side reach a little past the cell's (under a
// pixel and a half at usual sizes: cut, not shrunk), so a cell rounded
// up shows no seam.
//
// The writer is plain Go: the tables head, hhea, maxp, OS/2, hmtx, cmap
// (format 4, and 12 for the letters past U+FFFF), loca, glyf, name and
// post, each with its checksum.

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"unicode/utf16"
)

// geoFontMetrics is the terminal font's size, in its own units.
type geoFontMetrics struct {
	upm      int // units to the em
	asc      int // ascent, above the baseline
	desc     int // descent, below it (a positive number)
	gap      int // the line gap, under the descent
	adv      int // a letter's width
	fromFile string
}

// geoDefaultMetrics is a usual monospace font's (Agave's).
var geoDefaultMetrics = geoFontMetrics{upm: 2048, asc: 1536, desc: 512, gap: 0, adv: 1024}

const (
	geoFontFile   = "SyGeometry.ttf"
	geoFontFamily = "Sy Geometry"
)

// geoFontsDir is the user's own fonts folder.
func geoFontsDir() string {
	base := os.Getenv("XDG_DATA_HOME")
	if base == "" {
		home, _ := os.UserHomeDir()
		base = filepath.Join(home, ".local", "share")
	}
	return filepath.Join(base, "fonts")
}

// geoKittyConf is kitty's own settings file.
func init() { geoFontReady = geoFontSetUp }

// geoFontSetUp is whether the shape letters can be used here: in kitty
// (not over SSH, where the terminal is another computer's), with the font
// written by sy --font and kitty.conf using it.
func geoFontSetUp() bool {
	if os.Getenv("SSH_CONNECTION") != "" || os.Getenv("SSH_TTY") != "" {
		return false
	}
	if os.Getenv("KITTY_WINDOW_ID") == "" && !strings.Contains(os.Getenv("TERM"), "kitty") {
		return false
	}
	if _, err := os.Stat(filepath.Join(geoFontsDir(), geoFontFile)); err != nil {
		return false
	}
	b, err := os.ReadFile(geoKittyConf())
	return err == nil && bytes.Contains(b, []byte(geoFontFamily))
}

func geoKittyConf() string {
	if d := os.Getenv("KITTY_CONFIG_DIRECTORY"); d != "" {
		return filepath.Join(d, "kitty.conf")
	}
	base := os.Getenv("XDG_CONFIG_HOME")
	if base == "" {
		home, _ := os.UserHomeDir()
		base = filepath.Join(home, ".config")
	}
	return filepath.Join(base, "kitty", "kitty.conf")
}

// geoFontInstalled says whether sy --font has put the font in the fonts
// folder (looked at once).
var geoFontInstalled = sync.OnceValue(func() bool {
	_, err := os.Stat(filepath.Join(geoFontsDir(), geoFontFile))
	return err == nil
})

// geoKittyLines is what sy --font adds to kitty.conf.
func geoKittyLines() string {
	r := fmt.Sprintf("U+%X-U+%X", geoBase, geoBase+geoCount-1)
	return "# sy: shape letters for Design mode's 3D room (written by sy --font)\n" +
		"symbol_map " + r + " " + geoFontFamily + "\n" +
		"narrow_symbols " + r + " 1\n"
}

// geoInstall writes the font, has fontconfig find it, and tells kitty to
// use it, saying what it did on out.
func geoInstall(out io.Writer) error {
	fm := geoTermMetrics()
	dir := geoFontsDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	path := filepath.Join(dir, geoFontFile)
	if err := os.WriteFile(path, geoFontTTF(fm), 0o644); err != nil {
		return err
	}
	from := "a usual monospace font's size"
	if fm.fromFile != "" {
		from = filepath.Base(fm.fromFile)
	}
	fmt.Fprintf(out, "Wrote %s (sized to fit %s).\n", path, from)
	if _, err := exec.LookPath("fc-cache"); err == nil {
		if err := exec.Command("fc-cache", "-f", dir).Run(); err == nil {
			fmt.Fprintln(out, "Fontconfig knows the font now.")
		} else {
			fmt.Fprintln(out, "fc-cache failed: run fc-cache -f yourself.")
		}
	}
	conf := geoKittyConf()
	had, err := os.ReadFile(conf)
	switch {
	case err != nil:
		fmt.Fprintf(out, "No kitty.conf at %s. For kitty, add these lines to it:\n%s", conf, geoKittyLines())
		return nil
	case bytes.Contains(had, []byte(geoFontFamily)):
		fmt.Fprintf(out, "%s already uses the font.\n", conf)
	default:
		f, err := os.OpenFile(conf, os.O_APPEND|os.O_WRONLY, 0)
		if err != nil {
			return err
		}
		lead := "\n"
		if len(had) > 0 && had[len(had)-1] != '\n' {
			lead = "\n\n"
		}
		_, err = f.WriteString(lead + geoKittyLines())
		if cerr := f.Close(); err == nil {
			err = cerr
		}
		if err != nil {
			return err
		}
		fmt.Fprintf(out, "Added 3 lines to %s.\n", conf)
	}
	fmt.Fprintln(out, "Restart kitty to use it.")
	return nil
}

// ---- the terminal font's size -------------------------------------------------------------

// geoTermMetrics is the size of kitty's font (its font_family, found with
// fc-match), or a usual one's when it cannot be read.
func geoTermMetrics() geoFontMetrics {
	family := "monospace"
	if b, err := os.ReadFile(geoKittyConf()); err == nil {
		if f := kittyFamily(string(b)); f != "" {
			family = f
		}
	}
	out, err := exec.Command("fc-match", "-f", "%{file}", family).Output()
	if err != nil || len(out) == 0 {
		return geoDefaultMetrics
	}
	b, err := os.ReadFile(string(out))
	if err != nil {
		return geoDefaultMetrics
	}
	fm, err := sfntMetrics(b)
	if err != nil {
		return geoDefaultMetrics
	}
	fm.fromFile = string(out)
	return fm
}

// kittyFamily is the font_family set in kitty.conf's text ("" for none or
// auto): the last one set, as kitty takes it, plain or as family="…".
func kittyFamily(conf string) string {
	family := ""
	for _, line := range strings.Split(conf, "\n") {
		line = strings.TrimSpace(line)
		rest, ok := strings.CutPrefix(line, "font_family")
		if !ok || (rest != "" && rest[0] != ' ' && rest[0] != '\t') {
			continue
		}
		v := strings.TrimSpace(rest)
		if i := strings.Index(v, "family="); i >= 0 {
			v = v[i+len("family="):]
			if q := v[:min(1, len(v))]; q == `"` || q == "'" {
				if j := strings.Index(v[1:], q); j >= 0 {
					v = v[1 : 1+j]
				}
			} else if j := strings.IndexAny(v, " \t"); j >= 0 {
				v = v[:j]
			}
		}
		if v == "auto" || v == "monospace" {
			v = ""
		}
		family = v
	}
	return family
}

// sfntTables is where each table of a font file is: offset and length.
func sfntTables(b []byte) (map[string][2]int, error) {
	if len(b) < 12 {
		return nil, errors.New("too short")
	}
	at := 0
	if string(b[:4]) == "ttcf" { // a collection: its first font
		if len(b) < 16 {
			return nil, errors.New("too short")
		}
		at = int(binary.BigEndian.Uint32(b[12:]))
	}
	if at+12 > len(b) {
		return nil, errors.New("bad offset")
	}
	n := int(binary.BigEndian.Uint16(b[at+4:]))
	t := map[string][2]int{}
	for i := range n {
		r := at + 12 + 16*i
		if r+16 > len(b) {
			return nil, errors.New("bad table directory")
		}
		off, ln := int(binary.BigEndian.Uint32(b[r+8:])), int(binary.BigEndian.Uint32(b[r+12:]))
		if off+ln > len(b) {
			return nil, errors.New("table past the end")
		}
		t[string(b[r:r+4])] = [2]int{off, ln}
	}
	return t, nil
}

// sfntMetrics reads a font file's units to the em, ascent, descent, line
// gap (OS/2's when it says to use them, else hhea's) and the width of M.
func sfntMetrics(b []byte) (geoFontMetrics, error) {
	t, err := sfntTables(b)
	if err != nil {
		return geoFontMetrics{}, err
	}
	for _, need := range []string{"head", "hhea", "hmtx", "cmap"} {
		if _, ok := t[need]; !ok {
			return geoFontMetrics{}, errors.New("no " + need + " table")
		}
	}
	u16 := func(o int) int { return int(binary.BigEndian.Uint16(b[o:])) }
	s16 := func(o int) int { return int(int16(binary.BigEndian.Uint16(b[o:]))) }
	head, hhea, hmtx := t["head"][0], t["hhea"][0], t["hmtx"]
	fm := geoFontMetrics{upm: u16(head + 18), asc: s16(hhea + 4), desc: -s16(hhea + 6), gap: s16(hhea + 8)}
	if os2, ok := t["OS/2"]; ok && os2[1] >= 78 && u16(os2[0]+62)&0x80 != 0 {
		fm.asc, fm.desc, fm.gap = s16(os2[0]+68), -s16(os2[0]+70), s16(os2[0]+72)
	}
	nh := u16(hhea + 34)
	g := sfntGlyph(b, t["cmap"], 'M')
	if nh == 0 {
		return geoFontMetrics{}, errors.New("no widths")
	}
	if g >= nh {
		g = nh - 1
	}
	if 4*g+2 > hmtx[1] {
		return geoFontMetrics{}, errors.New("short hmtx")
	}
	fm.adv = u16(hmtx[0] + 4*g)
	if fm.upm <= 0 || fm.asc <= 0 || fm.adv <= 0 || fm.desc < 0 || fm.gap < 0 {
		return geoFontMetrics{}, errors.New("odd metrics")
	}
	return fm, nil
}

// sfntGlyph is the glyph of character c in a font's cmap table (0 for
// none), from its format 12 or format 4 subtable.
func sfntGlyph(b []byte, cmap [2]int, c rune) int {
	base := cmap[0]
	u16 := func(o int) int { return int(binary.BigEndian.Uint16(b[o:])) }
	u32 := func(o int) int { return int(binary.BigEndian.Uint32(b[o:])) }
	n := u16(base + 2)
	var f4, f12 int
	for i := range n {
		r := base + 4 + 8*i
		pid, eid, off := u16(r), u16(r+2), u32(r+4)
		st := base + off
		if st+2 > len(b) || (pid != 3 && pid != 0) {
			continue
		}
		switch u16(st) {
		case 12:
			if pid == 0 || eid == 10 {
				f12 = st
			}
		case 4:
			if pid == 0 || eid == 1 || eid == 0 {
				f4 = st
			}
		}
	}
	if f12 != 0 {
		groups := u32(f12 + 12)
		for i := range groups {
			g := f12 + 16 + 12*i
			if s, e := u32(g), u32(g+4); int(c) >= s && int(c) <= e {
				return u32(g+8) + int(c) - s
			}
		}
		return 0
	}
	if f4 == 0 || c > 0xFFFF {
		return 0
	}
	seg := u16(f4+6) / 2
	ends, starts := f4+14, f4+16+2*seg
	deltas, ranges := starts+2*seg, starts+4*seg
	for i := range seg {
		if int(c) > u16(ends+2*i) {
			continue
		}
		s := u16(starts + 2*i)
		if int(c) < s {
			return 0
		}
		d, ro := u16(deltas+2*i), u16(ranges+2*i)
		if ro == 0 {
			return (int(c) + d) & 0xFFFF
		}
		gi := u16(ranges + 2*i + ro + 2*(int(c)-s))
		if gi == 0 {
			return 0
		}
		return (gi + d) & 0xFFFF
	}
	return 0
}

// ---- writing the font ----------------------------------------------------------------------

// geoGlyph is one letter's outline in font units, clockwise (y up).
type geoGlyph struct {
	pts                    [][2]int
	xMin, yMin, xMax, yMax int
}

// geoOutline is shape letter i in font units for metrics fm: its top on
// the ascent, its bottom a little below the line gap, its sides a little
// past the cell's (see the top of the file).
func geoOutline(i int, fm geoFontMetrics) geoGlyph {
	cellH := float64(fm.asc + fm.desc + fm.gap)
	padX := float64(fm.adv) / 11 // past the right side: the cell is the letter's width rounded up
	padY := cellH / 10           // below the bottom: the cell is the font's height rounded up
	const on = 1e-9              // a point this close to a side is on it
	var g geoGlyph
	for _, p := range geoShape(i) {
		x := p.u * float64(fm.adv)
		switch {
		case p.u <= on:
			x = 0 // the left side exactly: a letter starting left of its cell is moved, not cut
		case p.u >= 1-on:
			x = float64(fm.adv) + padX
		}
		y := float64(fm.asc) - p.v/2*cellH
		if p.v >= 2-on {
			y -= padY
		}
		q := [2]int{int(math.Round(x)), int(math.Round(y))}
		if n := len(g.pts); n > 0 && g.pts[n-1] == q {
			continue
		}
		g.pts = append(g.pts, q)
	}
	if n := len(g.pts); n > 1 && g.pts[0] == g.pts[n-1] {
		g.pts = g.pts[:n-1]
	}
	area := 0
	for k := range g.pts {
		a, b := g.pts[k], g.pts[(k+1)%len(g.pts)]
		area += a[0]*b[1] - b[0]*a[1]
	}
	if len(g.pts) < 3 || area == 0 {
		return geoGlyph{}
	}
	if area > 0 { // anticlockwise: TrueType wants an outline clockwise
		for l, r := 0, len(g.pts)-1; l < r; l, r = l+1, r-1 {
			g.pts[l], g.pts[r] = g.pts[r], g.pts[l]
		}
	}
	g.xMin, g.yMin, g.xMax, g.yMax = g.pts[0][0], g.pts[0][1], g.pts[0][0], g.pts[0][1]
	for _, q := range g.pts {
		g.xMin, g.xMax = min(g.xMin, q[0]), max(g.xMax, q[0])
		g.yMin, g.yMax = min(g.yMin, q[1]), max(g.yMax, q[1])
	}
	return g
}

// glyf is the glyf table's data for g (empty: none).
func (g geoGlyph) glyf() []byte {
	if len(g.pts) == 0 {
		return nil
	}
	var w bytes.Buffer
	be := func(v any) { binary.Write(&w, binary.BigEndian, v) }
	be(int16(1))
	be([4]int16{int16(g.xMin), int16(g.yMin), int16(g.xMax), int16(g.yMax)})
	be(uint16(len(g.pts) - 1)) // the contour's last point
	be(uint16(0))              // no instructions
	for range g.pts {
		w.WriteByte(1) // on the curve; x and y as 16 bit steps
	}
	px, py := 0, 0
	for _, q := range g.pts {
		be(int16(q[0] - px))
		px = q[0]
	}
	for _, q := range g.pts {
		be(int16(q[1] - py))
		py = q[1]
	}
	for w.Len()%4 != 0 {
		w.WriteByte(0)
	}
	return w.Bytes()
}

// sfntSum is the checksum of a table's bytes.
func sfntSum(b []byte) uint32 {
	var s uint32
	for i := 0; i < len(b); i += 4 {
		var w [4]byte
		copy(w[:], b[i:min(i+4, len(b))])
		s += binary.BigEndian.Uint32(w[:])
	}
	return s
}

// geoFontTTF is the whole font file, sized for metrics fm. Glyph 0 is
// .notdef, 1 the space, 2 + i shape letter i.
func geoFontTTF(fm geoFontMetrics) []byte {
	nGlyphs := 2 + geoCount
	glyphs := make([]geoGlyph, nGlyphs)
	for i := range geoCount {
		glyphs[2+i] = geoOutline(i, fm)
	}
	maxPts, xMin, yMin, xMax, yMax := 0, 0, 0, 0, 0
	minRSB := fm.adv
	for _, g := range glyphs {
		if len(g.pts) == 0 {
			continue
		}
		maxPts = max(maxPts, len(g.pts))
		xMin, yMin, xMax, yMax = min(xMin, g.xMin), min(yMin, g.yMin), max(xMax, g.xMax), max(yMax, g.yMax)
		minRSB = min(minRSB, fm.adv-g.xMax)
	}

	tables := map[string][]byte{}
	put := func(tag string, parts ...any) {
		var w bytes.Buffer
		for _, p := range parts {
			binary.Write(&w, binary.BigEndian, p)
		}
		tables[tag] = w.Bytes()
	}

	// glyf and loca
	var glyf bytes.Buffer
	loca := make([]uint32, 0, nGlyphs+1)
	for _, g := range glyphs {
		loca = append(loca, uint32(glyf.Len()))
		glyf.Write(g.glyf())
	}
	loca = append(loca, uint32(glyf.Len()))
	tables["glyf"] = glyf.Bytes()
	put("loca", loca)

	// head (its checkSumAdjustment is filled in at the end)
	const madeAt = int64(3_900_000_000) // a fixed date, so the same font is the same file
	put("head", uint32(0x00010000), uint32(0x00010000), uint32(0), uint32(0x5F0F3CF5),
		uint16(0x000B), uint16(fm.upm), madeAt, madeAt,
		[4]int16{int16(xMin), int16(yMin), int16(xMax), int16(yMax)},
		uint16(0), uint16(6), int16(2), int16(1), int16(0))

	put("hhea", uint32(0x00010000), int16(fm.asc), int16(-fm.desc), int16(fm.gap),
		uint16(fm.adv), int16(xMin), int16(minRSB), int16(xMax),
		int16(1), int16(0), int16(0), [4]int16{}, int16(0), uint16(nGlyphs))

	put("maxp", uint32(0x00010000), uint16(nGlyphs), uint16(maxPts), uint16(1),
		uint16(0), uint16(0), uint16(2), [8]uint16{})

	// OS/2, version 4: regular, use the typo metrics, monospace.
	panose := [10]byte{2, 0, 5, 9, 0, 0, 0, 0, 0, 0}
	put("OS/2", uint16(4), int16(fm.adv), uint16(400), uint16(5), uint16(0),
		[4]int16{int16(fm.upm * 6 / 10), int16(fm.upm * 7 / 10), 0, int16(fm.upm / 7)},        // subscript
		[4]int16{int16(fm.upm * 6 / 10), int16(fm.upm * 7 / 10), 0, int16(fm.upm * 48 / 100)}, // superscript
		int16(fm.upm/20), int16(fm.upm*26/100), int16(0), panose,
		uint32(0), uint32(1<<28), uint32(1<<26), uint32(0), // private use, and plane 15's
		[4]byte{'S', 'Y', 'G', 'M'}, uint16(0x40|0x80), uint16(0x20), uint16(0xFFFF),
		int16(fm.asc), int16(-fm.desc), int16(fm.gap), uint16(max(fm.asc, yMax)), uint16(max(fm.desc+fm.gap, -yMin)),
		uint32(1), uint32(0), int16(fm.asc/2), int16(fm.asc*7/10), uint16(0), uint16(0x20), uint16(1))

	var hmtx []uint16
	for _, g := range glyphs {
		hmtx = append(hmtx, uint16(fm.adv), uint16(int16(g.xMin)))
	}
	put("hmtx", hmtx)

	// cmap: format 4 for the space, format 12 for it and the shape letters.
	last := uint32(geoBase + geoCount - 1)
	var f4 bytes.Buffer
	binary.Write(&f4, binary.BigEndian, []uint16{4, 32, 0, 4, 4, 1, 0,
		0x20, 0xFFFF, 0, 0x20, 0xFFFF, uint16(1 - 0x20 + 0x10000), 1, 0, 0})
	var f12 bytes.Buffer
	binary.Write(&f12, binary.BigEndian, []uint32{12 << 16, 16 + 2*12, 0, 2,
		0x20, 0x20, 1, geoBase, last, 2})
	put("cmap", uint16(0), uint16(2),
		uint16(3), uint16(1), uint32(4+2*8),
		uint16(3), uint16(10), uint32(4+2*8+uint32(f4.Len())),
		f4.Bytes(), f12.Bytes())

	// name
	names := []struct {
		id int
		s  string
	}{{1, geoFontFamily}, {2, "Regular"}, {3, geoFontFamily + " Regular"}, {4, geoFontFamily},
		{5, "Version 1.0"}, {6, "SyGeometry"}}
	var strs bytes.Buffer
	var recs []uint16
	for _, n := range names {
		u := utf16.Encode([]rune(n.s))
		recs = append(recs, 3, 1, 0x409, uint16(n.id), uint16(2*len(u)), uint16(strs.Len()))
		binary.Write(&strs, binary.BigEndian, u)
	}
	put("name", uint16(0), uint16(len(names)), uint16(6+12*len(names)), recs, strs.Bytes())

	put("post", uint32(0x00030000), int32(0), int16(-fm.desc/2), int16(max(fm.upm/20, 1)),
		uint32(1), [4]uint32{})

	// The file: the table directory, then the tables, each on 4 bytes.
	tags := make([]string, 0, len(tables))
	for t := range tables {
		tags = append(tags, t)
	}
	sort.Strings(tags)
	n := len(tags)
	es := 0
	for 1<<(es+1) <= n {
		es++
	}
	var out bytes.Buffer
	binary.Write(&out, binary.BigEndian, uint32(0x00010000))
	binary.Write(&out, binary.BigEndian, []uint16{uint16(n), uint16(16 << es), uint16(es), uint16(16*n - 16<<es)})
	off := 12 + 16*n
	headAt := 0
	for _, t := range tags {
		b := tables[t]
		if t == "head" {
			headAt = off
		}
		out.WriteString(t)
		binary.Write(&out, binary.BigEndian, []uint32{sfntSum(b), uint32(off), uint32(len(b))})
		off += (len(b) + 3) &^ 3
	}
	for _, t := range tags {
		out.Write(tables[t])
		for out.Len()%4 != 0 {
			out.WriteByte(0)
		}
	}
	file := out.Bytes()
	binary.BigEndian.PutUint32(file[headAt+8:], 0xB1B0AFBA-sfntSum(file))
	return file
}
