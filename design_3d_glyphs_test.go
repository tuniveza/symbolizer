package main

import (
	"github.com/charmbracelet/x/ansi"
	"strings"
	"testing"
)

// In kitty the room is a picture by default, its words painted on the
// faces: no letters of the terminal over it, and the pixels where a
// card's title is are partly the words' colour.
func TestPaintedWords(t *testing.T) {
	m := wordsScene(t, 140, 40)
	withPixels(t, m, 10, 20)
	r := &m.ds().room
	r.yaw, r.pitch = 20, 10
	rows := boardRows(m)
	if !r.pix || !r.paint {
		t.Fatalf("want the picture with painted words: pix %v, paint %v", r.pix, r.paint)
	}
	for y, row := range rows {
		for _, c := range []string{"Shopping", "Notes", "Bread"} {
			if strings.Contains(row, c) {
				t.Fatalf("row %d has %q as terminal letters: %q", y, c, row)
			}
		}
	}
	painted := 0
	for i := range r.tex {
		if r.tex[i].gl != nil {
			painted++
		}
	}
	if painted < 20 {
		t.Fatalf("%d painted letters, want the cards' words", painted)
	}
	// The picture has many more colours than plain fills: the letters'
	// edges are smooth (between the card's colour and the words').
	seen := map[uint32]bool{}
	for i := range r.base {
		seen[r.base[i]] = true
	}
	if len(seen) < 500 {
		t.Fatalf("%d colours in the picture", len(seen))
	}
}

// In 3D every structure's words are its flat drawing's, painted on its
// face: the cards' words, the drawers', boxes' and cabinets' counts and a
// box's layer tabs all show, as on the flat board.
func TestCardTexelsMatchFlat(t *testing.T) {
	m := wordsScene(t, 140, 40)
	flat := strings.ReplaceAll(ansi.Strip(strings.Join(boardRows(m), "")), " ", "")
	withPixels(t, m, 10, 20)
	r := &m.ds().room
	r.yaw, r.pitch = 20, 10
	boardRows(m)
	var text strings.Builder
	for i := range r.tex {
		tx := &r.tex[i]
		if tx.arms != 0 && tx.gl != nil {
			t.Fatal("a box line painted as a letter")
		}
		text.WriteString(tx.glyph)
	}
	s := text.String()
	for _, want := range []string{"Shoppinglist", "Breadandmilk", "top1", "Tools·1"} {
		if !strings.Contains(s, want) {
			t.Errorf("no %q in the 3D faces' words", want)
		}
		if !strings.Contains(flat, want) {
			t.Errorf("no %q on the flat board", want)
		}
	}
}

// A letter's coverage: a solid stroke inside, nothing at the cell's
// corner, the same mean seen from far off.
func TestGlyphCover(t *testing.T) {
	g := glyphOf("H", false, false)
	if g == nil {
		t.Fatal("no H in the font")
	}
	if c := g.cover(0.02, 0.02, 0.01, 0.01); c > 0.05 {
		t.Errorf("corner covered %.2f", c)
	}
	if c := g.cover(0.5, 0.5, 10, 10); c != g.mean {
		t.Errorf("far off %.3f, want the mean %.3f", c, g.mean)
	}
	most := 0.0
	for u := 0.1; u < 0.9; u += 0.02 {
		most = max(most, g.cover(u, 0.5, 0.01, 0.01))
	}
	if most < 0.9 {
		t.Errorf("the crossbar covers only %.2f", most)
	}
}

// The flat board keeps its counts, and pins3D is only on while the 3D
// picture is drawn.
func TestFlatKeepsCounts(t *testing.T) {
	m := wordsScene(t, 140, 40)
	rows := strings.Join(boardRows(m), "\n")
	if !strings.Contains(rows, "Tools · 1") {
		t.Fatal("the flat board lost the drawer's count")
	}
	if pins3D {
		t.Fatal("pins3D left on")
	}
}
