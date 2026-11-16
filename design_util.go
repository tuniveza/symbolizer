package main

// Small helpers the board uses, kept when the 3D room was taken out
// (16 Nov 2026).

import "unicode"

func clampF(v, lo, hi float64) float64 { return max(lo, min(hi, v)) }

func isWordRune(c rune) bool { return unicode.IsLetter(c) || unicode.IsDigit(c) }

// quadGlyph is the quarter-block letter for 4 bits: top left 1, top right
// 2, bottom left 4, bottom right 8.
var quadGlyph = [16]string{" ", "▘", "▝", "▀", "▖", "▌", "▞", "▛", "▗", "▚", "▐", "▜", "▄", "▙", "▟", "█"}

// cabSwing is how far, in degrees, a cabinet's doors swing open.
const cabSwing = 105

// screenMiddle is the board area's middle, as a board-area cell.
func (m *Model) screenMiddle() (sx, sy int) { return m.width / 2, m.boardH() / 2 }
