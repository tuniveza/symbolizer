package main

import (
	"strings"
	"testing"
)

// Every font has its own big letters (the user asked, 19 Nov 2026): at sizes
// 2–4 each font's rows differ from plain, keep the same height, and are as
// wide as codexMeasure says (so lining up and wrapping stay right).
func TestCodexBigFonts(t *testing.T) {
	words := "Hello Ab 42"
	for size := 2; size <= codexSizes; size++ {
		plain := strings.Join(codexBigF(words, size, fxFontPlain), "\n")
		for f := 0; f < 16; f++ {
			rows := codexBigF(words, size, f)
			if len(rows) != 3*(size-1) {
				t.Fatalf("%s size %d: %d rows, want %d", codexFonts[f], size, len(rows), 3*(size-1))
			}
			if f != fxFontPlain && strings.Join(rows, "\n") == plain {
				t.Fatalf("%s size %d looks just like plain", codexFonts[f], size)
			}
			if got, want := codexWidth(rows[0]), codexMeasure([]rune(words), f, size); got != want {
				t.Fatalf("%s size %d: %d cells wide, codexMeasure says %d", codexFonts[f], size, got, want)
			}
		}
	}
	if strings.Join(codexBig(words, 3), "\n") != strings.Join(codexBigF(words, 3, fxFontPlain), "\n") {
		t.Fatal("codexBig is plain big letters")
	}
}

// A big text on a page is drawn in its font.
func TestCodexBigTextFont(t *testing.T) {
	m, c := openLook(t)
	e := c.el(1)
	e.Text, e.Size, e.Font = "Hi", 3, fxFontDouble
	m.codexSave()
	want := codexBigF("Hi", 3, fxFontDouble)[0]
	if !strings.Contains(strings.Join(canvasRows(m), "\n"), strings.TrimRight(want, " ")) {
		t.Fatalf("a size 3 text in the double font should show its big double letters:\n%s", want)
	}
}

// The cover's title shows big letters in any font now (it was plain only).
func TestCodexCoverTitleBigFont(t *testing.T) {
	m, c := openCover(t)
	c.cf.Title = "Hi"
	c.cf.TitleFont = fxFontBold
	m.codexSave()
	canvasRows(m)
	if got := m.codexCoverLayNow().sizes[cpTitle]; got < 2 {
		t.Fatalf("a bold title on a big cover should be in big letters: size %d", got)
	}
	want := strings.TrimRight(codexBigF("Hi", 2, fxFontBold)[0], " ")
	if !strings.Contains(screenOf(m), want) {
		t.Fatalf("the cover should show the big bold title %q", want)
	}
}
