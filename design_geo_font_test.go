package main

import (
	"bytes"
	"encoding/binary"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The font file reads back: every table's checksum and the whole file's,
// the space and every shape letter in the cmap, one outline each, all
// as wide as the terminal font's letters, within their cells.
func TestGeoFontFile(t *testing.T) {
	fm := geoFontMetrics{upm: 2048, asc: 1500, desc: 480, gap: 68, adv: 1100}
	b := geoFontTTF(fm)
	if !bytes.Equal(b, geoFontTTF(fm)) {
		t.Fatal("the same metrics should make the same file")
	}
	tabs, err := sfntTables(b)
	if err != nil {
		t.Fatal(err)
	}
	u16 := func(o int) int { return int(binary.BigEndian.Uint16(b[o:])) }
	s16 := func(o int) int { return int(int16(binary.BigEndian.Uint16(b[o:]))) }
	u32 := func(o int) int { return int(binary.BigEndian.Uint32(b[o:])) }
	for _, tag := range []string{"OS/2", "cmap", "glyf", "head", "hhea", "hmtx", "loca", "maxp", "name", "post"} {
		if _, ok := tabs[tag]; !ok {
			t.Fatalf("no %s table", tag)
		}
	}
	n := u16(4)
	for i := range n {
		r := 12 + 16*i
		tag, sum := string(b[r:r+4]), uint32(u32(r+4))
		at := tabs[tag]
		data := append([]byte(nil), b[at[0]:at[0]+at[1]]...)
		if tag == "head" {
			binary.BigEndian.PutUint32(data[8:], 0)
		}
		if at[0]%4 != 0 || sfntSum(data) != sum {
			t.Fatalf("table %s: offset %d, checksum %x, want %x", tag, at[0], sum, sfntSum(data))
		}
		if i > 0 && string(b[r-16:r-12]) >= tag {
			t.Fatalf("tables out of order at %s", tag)
		}
	}
	if s := sfntSum(b); s != 0xB1B0AFBA {
		t.Fatalf("whole file checksum %x", s)
	}
	head, hhea, maxp := tabs["head"][0], tabs["hhea"][0], tabs["maxp"][0]
	if u16(head+18) != fm.upm || u16(head+50) != 1 || u32(head+12) != 0x5F0F3CF5 {
		t.Fatalf("head: upm %d, loca format %d", u16(head+18), u16(head+50))
	}
	if s16(hhea+4) != fm.asc || s16(hhea+6) != -fm.desc || s16(hhea+8) != fm.gap {
		t.Fatal("hhea's ascent, descent and gap should be the terminal font's")
	}
	glyphs := u16(maxp + 4)
	if glyphs != 2+geoCount || u16(hhea+34) != glyphs {
		t.Fatalf("%d glyphs, %d widths, want %d", glyphs, u16(hhea+34), 2+geoCount)
	}
	os2 := tabs["OS/2"][0]
	if u16(os2) != 4 || u16(os2+62)&0x80 == 0 || s16(os2+68) != fm.asc {
		t.Fatal("OS/2 should be version 4 and use its typo metrics")
	}
	cm := tabs["cmap"]
	for c, want := range map[rune]int{' ': 1, geoBase: 2, geoBase + 100: 102, geoBase + geoCount - 1: geoCount + 1, geoBase + geoCount: 0, 'M': 0} {
		if g := sfntGlyph(b, cm, c); g != want {
			t.Fatalf("U+%X is glyph %d, want %d", c, g, want)
		}
	}
	// Its format 4 table alone (as an older reader takes it) has the space.
	only4 := append([]byte{0, 0, 0, 1, 0, 3, 0, 1, 0, 0, 0, 12}, b[cm[0]+20:cm[0]+52]...)
	if g, h := sfntGlyph(only4, [2]int{0, len(only4)}, ' '), sfntGlyph(only4, [2]int{0, len(only4)}, 'a'); g != 1 || h != 0 {
		t.Fatalf("format 4: space %d, a %d", g, h)
	}
	name := tabs["name"][0]
	if !strings.Contains(string(b[name:name+tabs["name"][1]]), "\x00S\x00y\x00 \x00G\x00e\x00o") {
		t.Fatal("the name table should say Sy Geometry")
	}
	loca, glyf, hmtx := tabs["loca"][0], tabs["glyf"][0], tabs["hmtx"][0]
	cellH := fm.asc + fm.desc + fm.gap
	for g := range glyphs {
		if u16(hmtx+4*g) != fm.adv {
			t.Fatalf("glyph %d is %d wide", g, u16(hmtx+4*g))
		}
		start, end := u32(loca+4*g), u32(loca+4*g+4)
		if g < 2 {
			if end != start {
				t.Fatalf("glyph %d should be empty", g)
			}
			continue
		}
		if end <= start {
			t.Fatalf("shape letter %d has no outline", g-2)
		}
		o := glyf + start
		if s16(o) != 1 {
			t.Fatalf("shape letter %d: %d contours", g-2, s16(o))
		}
		x0, y0, x1, y1 := s16(o+2), s16(o+4), s16(o+6), s16(o+8)
		if x0 < 0 || x1 > fm.adv+fm.adv/10 || y1 > fm.asc || y0 < fm.asc-cellH-cellH/9 {
			t.Fatalf("shape letter %d is outside its cell: %d %d %d %d", g-2, x0, y0, x1, y1)
		}
		// Clockwise (y up), as TrueType wants: the points, read back.
		np := u16(o+10) + 1
		p := o + 12 + 2 + np // past the flags (all on the curve, one byte each)
		xs, ys := make([]int, np), make([]int, np)
		x, y := 0, 0
		for k := range np {
			x += s16(p + 2*k)
			xs[k] = x
		}
		for k := range np {
			y += s16(p + 2*np + 2*k)
			ys[k] = y
		}
		area := 0
		for k := range np {
			area += xs[k]*ys[(k+1)%np] - xs[(k+1)%np]*ys[k]
		}
		if area >= 0 {
			t.Fatalf("shape letter %d goes round the wrong way (area %d)", g-2, area)
		}
	}
	// Read back as a terminal font, it has the same size.
	got, err := sfntMetrics(b)
	if err != nil {
		t.Fatal(err)
	}
	if got.upm != fm.upm || got.asc != fm.asc || got.desc != fm.desc || got.gap != fm.gap || got.adv != fm.adv {
		t.Fatalf("read back %+v, want %+v", got, fm)
	}
}

// A half cell letter covers half its cell: the wedge at depth 4 facing
// down is the bottom half, from the cell's middle to its bottom.
func TestGeoFontHalf(t *testing.T) {
	fm := geoDefaultMetrics
	g := geoOutline(16*geoDepths+3, fm) // angle 16 of 64: facing straight down (v grows down)
	mid := fm.asc - (fm.asc+fm.desc)/2
	if g.yMax != mid || g.yMin >= fm.asc-(fm.asc+fm.desc) || g.xMin != 0 || g.xMax <= fm.adv {
		t.Fatalf("the bottom half: %+v, its top should be %d", g, mid)
	}
}

// With no fc-match, or no kitty.conf, the font is sized as a usual
// monospace font; font_family is read as kitty reads it.
func TestGeoTermMetrics(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("KITTY_CONFIG_DIRECTORY", "")
	if fm := geoTermMetrics(); fm != geoDefaultMetrics {
		t.Fatalf("no fc-match: %+v", fm)
	}
	for conf, want := range map[string]string{
		"font_family      Agave Nerd Font\nfont_size 13":    "Agave Nerd Font",
		"# font_family Hack\n":                              "",
		"font_family Hack\nfont_family  Iosevka Term \n":    "Iosevka Term",
		`font_family family="JetBrains Mono" style=Regular`: "JetBrains Mono",
		"font_family family=Hack features=+zero":            "Hack",
		"font_family auto\n":                                "",
		"font_family_x Nope\nbold_font auto":                "",
	} {
		if got := kittyFamily(conf); got != want {
			t.Fatalf("%q: %q, want %q", conf, got, want)
		}
	}
	if _, err := sfntMetrics([]byte("not a font at all")); err == nil {
		t.Fatal("rubbish should not read as a font")
	}
}

// sy --font writes the font where fontconfig looks, and adds its lines
// to kitty.conf once.
func TestGeoInstall(t *testing.T) {
	data, cfg := t.TempDir(), t.TempDir()
	t.Setenv("PATH", t.TempDir()) // no fc-match, no fc-cache
	t.Setenv("XDG_DATA_HOME", data)
	t.Setenv("XDG_CONFIG_HOME", cfg)
	t.Setenv("KITTY_CONFIG_DIRECTORY", "")
	var out bytes.Buffer
	if err := geoInstall(&out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "No kitty.conf") || !strings.Contains(out.String(), "symbol_map U+FB000-U+FB2DF Sy Geometry") {
		t.Fatalf("without kitty.conf it should say what to add:\n%s", out.String())
	}
	b, err := os.ReadFile(filepath.Join(data, "fonts", geoFontFile))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := sfntTables(b); err != nil {
		t.Fatal(err)
	}
	conf := filepath.Join(cfg, "kitty", "kitty.conf")
	os.MkdirAll(filepath.Dir(conf), 0o755)
	os.WriteFile(conf, []byte("font_size 13"), 0o644)
	for range 2 {
		out.Reset()
		if err := geoInstall(&out); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(out.String(), "Restart kitty") {
			t.Fatalf("it should say to restart kitty:\n%s", out.String())
		}
	}
	got, _ := os.ReadFile(conf)
	if strings.Count(string(got), "symbol_map") != 1 || strings.Count(string(got), "narrow_symbols") != 1 ||
		!strings.HasPrefix(string(got), "font_size 13\n\n# sy:") {
		t.Fatalf("kitty.conf:\n%s", got)
	}
	if fmt := geoKittyLines(); !strings.Contains(fmt, "U+FB2DF") {
		t.Fatalf("the range should end at the last shape letter: %s", fmt)
	}
}

// The shape letters are used in kitty once sy --font has written the font
// and told kitty.conf; not over SSH, not elsewhere, not before.
func TestGeoFontSetUp(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_DATA_HOME", filepath.Join(dir, "data"))
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, "config"))
	t.Setenv("SSH_CONNECTION", "")
	t.Setenv("SSH_TTY", "")
	t.Setenv("KITTY_WINDOW_ID", "1")
	if geoFontSetUp() {
		t.Fatal("set up before sy --font")
	}
	if err := os.MkdirAll(filepath.Join(dir, "config", "kitty"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "config", "kitty", "kitty.conf"), []byte("font_size 12\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := geoInstall(io.Discard); err != nil {
		t.Fatal(err)
	}
	if !geoFontSetUp() {
		t.Fatal("not set up after sy --font")
	}
	t.Setenv("SSH_CONNECTION", "1.2.3.4 5 6.7.8.9 22")
	if geoFontSetUp() {
		t.Fatal("over SSH it should be off")
	}
	t.Setenv("SSH_CONNECTION", "")
	t.Setenv("KITTY_WINDOW_ID", "")
	t.Setenv("TERM", "xterm-256color")
	if geoFontSetUp() {
		t.Fatal("outside kitty it should be off")
	}
}
