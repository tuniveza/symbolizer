package main

import (
	"reflect"
	"strings"
	"testing"
)

var winTestSymbols = map[rune]bool{
	'☻': true, '♪': true, '✦': true, '·': true, '°': true, '∙': true,
	'⌂': true, '♙': true, '●': true, 'Λ': true, 'ô': true,
}

// winTestNarrow: only runes that render one cell wide in normal monospace fonts.
func winTestNarrow(r rune) bool {
	switch {
	case r >= 0x20 && r < 0x7f:
		return true
	case r >= 0x2500 && r <= 0x259f: // box drawing + block elements
		return true
	}
	return winTestSymbols[r]
}

func winTestHex(s string) bool {
	if len(s) != 7 || s[0] != '#' {
		return false
	}
	return strings.Trim(s[1:], "0123456789abcdef") == ""
}

func winTestKey(rows [][]winCell) string {
	var b strings.Builder
	for _, row := range rows {
		for _, c := range row {
			b.WriteRune(c.R)
			b.WriteString(c.Fg)
			b.WriteString(c.Bg)
		}
		b.WriteByte('\n')
	}
	return b.String()
}

func TestWinArtSizes(t *testing.T) {
	ws := []int{0, 1, 2, 3, 4, 5, 6, 7, 8, 10, 13, 16, 20, 31, 40, 57, 80, 120}
	hs := []int{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 12, 15, 16, 17, 20, 24, 30, 40}
	ts := []float64{0, 1.3, 77.7, -5, 123456.78}
	for _, w := range ws {
		for _, h := range hs {
			for _, tt := range ts {
				for seed := uint64(1); seed <= 4; seed++ {
					for _, light := range []bool{false, true} {
						rows := winArt(seed, w, h, tt, light)
						if len(rows) != h {
							t.Fatalf("%dx%d: %d rows", w, h, len(rows))
						}
						for y, row := range rows {
							if len(row) != w {
								t.Fatalf("%dx%d: row %d has %d cells", w, h, y, len(row))
							}
							for x, c := range row {
								if !winTestNarrow(c.R) {
									t.Fatalf("%dx%d seed %d: rune %q (%U) at %d,%d not allowed", w, h, seed, c.R, c.R, x, y)
								}
								if !winTestHex(c.Bg) {
									t.Fatalf("%dx%d: bad Bg %q at %d,%d", w, h, c.Bg, x, y)
								}
								if c.R != ' ' && !winTestHex(c.Fg) {
									t.Fatalf("%dx%d: bad Fg %q for %q at %d,%d", w, h, c.Fg, c.R, x, y)
								}
							}
						}
					}
				}
			}
		}
	}
}

func TestWinArtDeterministic(t *testing.T) {
	for seed := uint64(0); seed < 20; seed++ {
		for _, tt := range []float64{0, 2.5, 99.1} {
			a := winArt(seed, 80, 30, tt, seed%2 == 0)
			b := winArt(seed, 80, 30, tt, seed%2 == 0)
			if !reflect.DeepEqual(a, b) {
				t.Fatalf("seed %d t %v: not deterministic", seed, tt)
			}
		}
	}
}

func TestWinArtSeedsDiffer(t *testing.T) {
	for _, light := range []bool{false, true} {
		seen := map[string]uint64{}
		for seed := uint64(1); seed <= 40; seed++ {
			k := winTestKey(winArt(seed, 40, 14, 3, light))
			if prev, ok := seen[k]; ok {
				t.Fatalf("seeds %d and %d look identical", prev, seed)
			}
			seen[k] = seed
		}
		// tiny windows should still vary across seeds
		tiny := map[string]bool{}
		for seed := uint64(1); seed <= 40; seed++ {
			tiny[winTestKey(winArt(seed, 6, 4, 0, light))] = true
		}
		if len(tiny) < 10 {
			t.Fatalf("6x4: only %d distinct looks over 40 seeds", len(tiny))
		}
	}
}

func TestWinArtFrame(t *testing.T) {
	frame := map[rune]bool{'┏': true, '┓': true, '━': true, '┃': true, '▀': true, '┳': true, '┣': true, '┫': true}
	for _, w := range []int{4, 5, 9, 14, 40, 80, 120} {
		for _, h := range []int{3, 4, 7, 14, 30, 40} {
			for seed := uint64(1); seed <= 6; seed++ {
				rows := winArt(seed, w, h, 1, seed%2 == 1)
				check := func(x, y int) {
					c := rows[y][x]
					if !frame[c.R] || c.Fg == "" || c.Bg == "" {
						t.Fatalf("%dx%d seed %d: edge %d,%d is %q", w, h, seed, x, y, c.R)
					}
				}
				for x := 0; x < w; x++ {
					check(x, 0)
					check(x, h-1)
				}
				for y := 0; y < h; y++ {
					check(0, y)
					check(w-1, y)
				}
				if rows[0][0].R != '┏' || rows[0][w-1].R != '┓' {
					t.Fatalf("%dx%d: missing top corners", w, h)
				}
			}
		}
	}
}

func TestWinArtMoves(t *testing.T) {
	sizes := [][2]int{{20, 8}, {40, 14}, {70, 24}, {80, 30}}
	for _, sz := range sizes {
		w, h := sz[0], sz[1]
		streetMoves := 0
		const n = 200
		for seed := uint64(1); seed <= n; seed++ {
			light := seed%3 == 0
			a := winArt(seed, w, h, 0, light)
			b := winArt(seed, w, h, 2, light)
			any, street := false, false
			for y := range a {
				for x := range a[y] {
					if a[y][x] != b[y][x] {
						any = true
						if y >= h*2/3 { // pavement / road area: the walkers
							street = true
						}
					}
				}
			}
			if !any {
				t.Fatalf("%dx%d seed %d: nothing changed between t=0 and t=2", w, h, seed)
			}
			if street {
				streetMoves++
			}
		}
		if streetMoves < n*8/10 {
			t.Fatalf("%dx%d: walkers moved in only %d/%d windows", w, h, streetMoves, n)
		}
	}
}

func TestWinCountAndSeed(t *testing.T) {
	seen := map[int]int{}
	for s := uint64(0); s < 5000; s++ {
		c := winCount(s)
		if c < 1 || c > 4 {
			t.Fatalf("winCount(%d) = %d", s, c)
		}
		if c != winCount(s) {
			t.Fatal("winCount not deterministic")
		}
		seen[c]++
	}
	for c := 1; c <= 4; c++ {
		if seen[c] < 800 {
			t.Fatalf("winCount value %d appears only %d/5000 times", c, seen[c])
		}
	}
	ids := map[uint64]bool{}
	for room := uint64(0); room < 50; room++ {
		for i := 0; i < 4; i++ {
			s := winSeed(room, i)
			if s != winSeed(room, i) {
				t.Fatal("winSeed not deterministic")
			}
			if ids[s] {
				t.Fatalf("winSeed collision room %d i %d", room, i)
			}
			ids[s] = true
		}
	}
}

func BenchmarkWinArt80x30(b *testing.B) {
	for i := 0; i < b.N; i++ {
		winArt(uint64(i%8), 80, 30, float64(i)*0.13, i%2 == 0)
	}
}

func BenchmarkWinArt40x14(b *testing.B) {
	for i := 0; i < b.N; i++ {
		winArt(uint64(i%8), 40, 14, float64(i)*0.13, i%2 == 0)
	}
}
