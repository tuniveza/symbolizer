package main

import (
	"testing"

	"github.com/mattn/go-runewidth"
)

// Every font gives letters of the right width: 1 cell, or 2 for "wide".
func TestCodexFxFontWidths(t *testing.T) {
	in := "Hello World 123"
	for f, name := range codexFonts {
		out := codexFont(in, f)
		want := 1
		if name == "wide" {
			want = 2
		}
		for _, r := range out {
			w := runewidth.RuneWidth(r)
			if r == ' ' {
				w = want // spaces stay plain spaces
				if runewidth.RuneWidth(r) != 1 {
					t.Errorf("font %q: space is not 1 cell", name)
				}
				continue
			}
			if w != want {
				t.Errorf("font %q: rune %q is %d cells, want %d", name, r, w, want)
			}
		}
		spaces := 2
		if got, wantW := codexWidth(out), (len([]rune(in))-spaces)*want+spaces; got != wantW {
			t.Errorf("font %q: codexWidth = %d, want %d", name, got, wantW)
		}
		if f > 0 && name != "squared" && out == in {
			t.Errorf("font %q changed nothing", name)
		}
	}
	if codexFont(in, -1) != in || codexFont(in, 99) != in {
		t.Error("font out of range should be plain")
	}
	if got := codexFont("ab\ncd", 15); got != "qɐ\npɔ" {
		t.Errorf("upside down = %q", got)
	}
}

// Big letters: rows all the same width, and size n is (n-1) times size 2.
func TestCodexFxBig(t *testing.T) {
	s := "Hi, Codex! 0-9 @#%&*()"
	if got := codexBig(s, 1); len(got) != 1 || got[0] != s {
		t.Fatalf("size 1 should be unchanged, got %q", got)
	}
	base := codexBig(s, 2)
	if len(base) != 3 {
		t.Fatalf("size 2 rows = %d, want 3", len(base))
	}
	baseW := codexWidth(base[0])
	for size := 2; size <= codexSizes; size++ {
		rows := codexBig(s, size)
		if len(rows) != len(base)*(size-1) {
			t.Errorf("size %d: %d rows, want %d", size, len(rows), len(base)*(size-1))
		}
		w := codexWidth(rows[0])
		if w != baseW*(size-1) {
			t.Errorf("size %d: width %d, want %d", size, w, baseW*(size-1))
		}
		for i, r := range rows {
			if codexWidth(r) != w || len([]rune(r)) != w {
				t.Errorf("size %d row %d: width %d, want %d", size, i, codexWidth(r), w)
			}
		}
	}
	// Two lines stack; unknown runes still draw something.
	if got := codexBig("A\nB", 2); len(got) != 6 {
		t.Errorf("two lines: %d rows, want 6", len(got))
	}
	if got := codexBig("€", 2); got[1] == "   " {
		t.Errorf("unknown rune drew nothing: %q", got)
	}
}

// Poses are the same for the same inputs and stay in sane ranges.
func TestCodexFxPose(t *testing.T) {
	sizes := [][2]int{{12, 3}, {3, 1}, {40, 8}}
	for anim := range codexAnims {
		for _, wh := range sizes {
			w, h := wh[0], wh[1]
			for step := 0; step < 240; step++ {
				tm := float64(step) / 12
				for y := 0; y < h; y++ {
					for x := 0; x < w; x++ {
						p := codexPose(anim, x, y, w, h, tm, 42)
						if q := codexPose(anim, x, y, w, h, tm, 42); p != q {
							t.Fatalf("%s: not the same twice", codexAnims[anim])
						}
						if p.DX < -w || p.DX > w || p.DY < -w || p.DY > w {
							t.Fatalf("%s: move %d,%d too big for w=%d", codexAnims[anim], p.DX, p.DY, w)
						}
						if p.Fade < 0 || p.Fade > 1 || p.TintAmt < 0 || p.TintAmt > 1 {
							t.Fatalf("%s: fade %v tint %v out of 0..1", codexAnims[anim], p.Fade, p.TintAmt)
						}
						if p.Glyph != "" && runewidth.StringWidth(p.Glyph) != 1 {
							t.Fatalf("%s: glyph %q not 1 cell", codexAnims[anim], p.Glyph)
						}
						if p.Tint != "" && (len(p.Tint) != 7 || p.Tint[0] != '#') {
							t.Fatalf("%s: bad tint %q", codexAnims[anim], p.Tint)
						}
						if anim == 6 { // marquee wraps inside the element
							if nx := x + p.DX; nx < 0 || nx >= w {
								t.Fatalf("marquee: x+DX = %d outside 0..%d", nx, w-1)
							}
						}
					}
				}
			}
		}
	}
	if codexPose(0, 1, 1, 5, 5, 3, 1) != (fxPose{}) {
		t.Error("none should do nothing")
	}
}

// The settling animations finish, and then they look plain.
func TestCodexFxSettle(t *testing.T) {
	for _, name := range []string{"typewriter", "fade in", "slide in"} {
		anim := -1
		for i, n := range codexAnims {
			if n == name {
				anim = i
			}
		}
		if codexAnimLoops(anim) {
			t.Errorf("%s should not loop", name)
		}
		if codexAnimDone(anim, 40, 10, 0.01) {
			t.Errorf("%s done too soon", name)
		}
		if !codexAnimDone(anim, 40, 10, 60) {
			t.Errorf("%s not done by t=60", name)
		}
		for y := 0; y < 10; y++ {
			for x := 0; x < 40; x++ {
				if p := codexPose(anim, x, y, 40, 10, 60, 7); p != (fxPose{}) {
					t.Fatalf("%s: still moving at t=60: %+v", name, p)
				}
			}
		}
	}
	if codexAnimLoops(0) || !codexAnimDone(0, 1, 1, 0) {
		t.Error("none should not loop and should be done")
	}
	for i, n := range codexAnims {
		if n != "none" && n != "typewriter" && n != "fade in" && n != "slide in" {
			if !codexAnimLoops(i) || codexAnimDone(i, 5, 5, 60) {
				t.Errorf("%s should loop", n)
			}
		}
	}
}

// Symbols: enough of them, every frame 1 cell, big ones are neat blocks.
func TestCodexFxSymbols(t *testing.T) {
	if len(codexSymbols) < 24 {
		t.Fatalf("only %d symbols", len(codexSymbols))
	}
	seen := map[string]bool{}
	for i, s := range codexSymbols {
		if seen[s.Name] {
			t.Errorf("symbol name %q used twice", s.Name)
		}
		seen[s.Name] = true
		if len(s.Frames) == 0 || s.FPS <= 0 || len(s.Color) != 7 {
			t.Errorf("symbol %q: bad fields", s.Name)
		}
		if _, ok := fxSymbolShape[s.Name]; !ok {
			t.Errorf("symbol %q has no big shape", s.Name)
		}
		for _, f := range s.Frames {
			if runewidth.StringWidth(f) != 1 || len([]rune(f)) != 1 {
				t.Errorf("symbol %q: frame %q is not 1 cell", s.Name, f)
			}
		}
		for step := 0; step < 30; step++ {
			tm := float64(step) / 12
			if f := codexSymbolFrame(i, tm); runewidth.StringWidth(f) != 1 {
				t.Errorf("symbol %q: frame at %v is %q", s.Name, tm, f)
			}
			if codexSymbolFrame(i, tm) != codexSymbolFrame(i+len(codexSymbols), tm) {
				t.Errorf("symbol index should wrap")
			}
			for size := 1; size <= codexSizes; size++ {
				rows := codexSymbolBig(i, tm, size)
				wantH := size*2 - 1
				if len(rows) != wantH {
					t.Errorf("symbol %q size %d: %d rows, want %d", s.Name, size, len(rows), wantH)
				}
				w := codexWidth(rows[0])
				for _, r := range rows {
					if codexWidth(r) != w {
						t.Errorf("symbol %q size %d: rows differ in width", s.Name, size)
					}
				}
				again := codexSymbolBig(i, tm, size)
				for k := range rows {
					if rows[k] != again[k] {
						t.Errorf("symbol %q: big not the same twice", s.Name)
					}
				}
			}
		}
	}
}
