package emmet

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func actWriteFile(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
}

func actPNG(t *testing.T, w, h int) []byte {
	var b bytes.Buffer
	if err := png.Encode(&b, image.NewRGBA(image.Rect(0, 0, w, h))); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func actRIFF(chunk string, data []byte) []byte {
	var b bytes.Buffer
	b.WriteString("RIFF")
	binary.Write(&b, binary.LittleEndian, uint32(4+8+len(data)))
	b.WriteString("WEBP")
	b.WriteString(chunk)
	binary.Write(&b, binary.LittleEndian, uint32(len(data)))
	b.Write(data)
	return b.Bytes()
}

// actImageDir makes a folder with one image of each kind.
func actImageDir(t *testing.T) string {
	dir := t.TempDir()
	actWriteFile(t, filepath.Join(dir, "a.png"), actPNG(t, 3, 2))
	actWriteFile(t, filepath.Join(dir, "img", "logo@2x.png"), actPNG(t, 6, 4))
	var g bytes.Buffer
	if err := gif.Encode(&g, image.NewPaletted(image.Rect(0, 0, 4, 5), color.Palette{color.Black, color.White}), nil); err != nil {
		t.Fatal(err)
	}
	actWriteFile(t, filepath.Join(dir, "a.gif"), g.Bytes())
	var j bytes.Buffer
	if err := jpeg.Encode(&j, image.NewRGBA(image.Rect(0, 0, 6, 7)), nil); err != nil {
		t.Fatal(err)
	}
	actWriteFile(t, filepath.Join(dir, "a.jpg"), j.Bytes())
	// VP8L: signature 0x2f, then 14 bits width-1, 14 bits height-1
	bits := uint32(8-1) | uint32(9-1)<<14
	vp8l := []byte{0x2f, byte(bits), byte(bits >> 8), byte(bits >> 16), byte(bits >> 24), 0, 0, 0, 0, 0}
	actWriteFile(t, filepath.Join(dir, "l.webp"), actRIFF("VP8L", vp8l))
	// VP8: frame tag, start code, 16-bit width and height
	vp8 := []byte{0, 0, 0, 0x9d, 0x01, 0x2a, 10, 0, 11, 0}
	actWriteFile(t, filepath.Join(dir, "v.webp"), actRIFF("VP8 ", vp8))
	// VP8X: flags, 3 reserved, 24-bit width-1 and height-1
	vp8x := []byte{0, 0, 0, 0, 299 & 0xff, 299 >> 8, 0, 149, 0, 0}
	actWriteFile(t, filepath.Join(dir, "x.webp"), actRIFF("VP8X", vp8x))
	actWriteFile(t, filepath.Join(dir, "s.svg"), []byte(`<?xml version="1.0"?><svg xmlns="http://www.w3.org/2000/svg" width="40px" height="30"></svg>`))
	actWriteFile(t, filepath.Join(dir, "vb.svg"), []byte(`<svg viewBox="0 0 24 12"><path d="M0 0"/></svg>`))
	actWriteFile(t, filepath.Join(dir, "wv.svg"), []byte(`<svg width="48" viewBox="0 0 24 12"></svg>`))
	actWriteFile(t, filepath.Join(dir, "bad.png"), []byte("not an image"))
	return dir
}

func TestActImageFileSize(t *testing.T) {
	dir := actImageDir(t)
	cases := []struct {
		src  string
		w, h int
		err  string
	}{
		{"a.png", 3, 2, ""},
		{"a.gif", 4, 5, ""},
		{"a.jpg", 6, 7, ""},
		{"l.webp", 8, 9, ""},
		{"v.webp", 10, 11, ""},
		{"x.webp", 300, 150, ""},
		{"s.svg", 40, 30, ""},
		{"vb.svg", 24, 12, ""},
		{"wv.svg", 48, 24, ""},
		{"img/logo@2x.png", 3, 2, ""},
		{"a.png?v=3", 3, 2, ""},
		{"missing.png", 0, 0, "not found"},
		{"bad.png", 0, 0, "not a PNG"},
		{"https://example.com/a.png", 0, 0, "web or data"},
		{"data:image/png;base64,AAAA", 0, 0, "web or data"},
		{"//cdn.example.com/a.png", 0, 0, "web or data"},
	}
	for _, c := range cases {
		w, h, err := actImageFileSize(c.src, dir)
		if c.err != "" {
			if err == nil || !strings.Contains(err.Error(), c.err) {
				t.Errorf("%s: err %v, want %q", c.src, err, c.err)
			}
			continue
		}
		if err != nil || w != c.w || h != c.h {
			t.Errorf("%s: %d×%d %v, want %d×%d", c.src, w, h, err, c.w, c.h)
		}
	}
}

func TestActUpdateImageSize(t *testing.T) {
	dir := actImageDir(t)
	cases := []struct{ syn, in, want, err string }{
		{"html", `<img src="a.png"|>`, `<img src="a.png"| width="3" height="2">`, ""},
		{"html", `<img width="100" sr|c='a.gif'>`, `<img width="4" sr|c='a.gif' height='5'>`, ""},
		{"html", `<img src="a.png" width height=9|>`, `<img src="a.png" width="3" height="2"|>`, ""},
		{"jsx", `<img src="a.png" |/>`, `<img src="a.png" width={3} height={2} |/>`, ""},
		{"jsx", `<img src="a.png" width={10}| />`, `<img src="a.png" width={3}| height={2} />`, ""},
		{"css", ".a {\n  background: url(a.png);|\n}", ".a {\n  background: url(a.png);|\n  width: 3px;\n  height: 2px;\n}", ""},
		{"css", `.a { width: 1px; background: url("a.gif")| }`, `.a { width: 4px; background: url("a.gif")|; height: 5px; }`, ""},
		{"scss", ".a { b|ackground: url('x.webp') no-repeat; height: 1px; }", ".a { b|ackground: url('x.webp') no-repeat; width: 300px; height: 150px; }", ""},
		{"html", `<img src="https://x.org/a.png"|>`, "", "web or data"},
		{"html", `<p>|</p>`, "", "no <img>"},
		{"html", `<img alt="x"|>`, "", "no src"},
		{"css", `.a { color: red;| }`, "", "no url"},
		{"html", `<img src="nope.png"|>`, "", "not found"},
	}
	for _, c := range cases {
		text, sel := actMarks(c.in)
		ch, err := UpdateImageSize(text, sel.Start, Options{Syntax: c.syn}, dir)
		if c.err != "" {
			if err == nil || !strings.Contains(err.Error(), c.err) {
				t.Errorf("UpdateImageSize(%q): err %v, want %q", c.in, err, c.err)
			}
			continue
		}
		if err != nil {
			t.Errorf("UpdateImageSize(%q): %v", c.in, err)
			continue
		}
		if got := actResult(t, text, ch, true); got != c.want {
			t.Errorf("UpdateImageSize(%q) =\n%q, want\n%q", c.in, got, c.want)
		}
		if !strings.HasPrefix(ch.Note, "image size: ") {
			t.Errorf("note %q", ch.Note)
		}
	}
}
