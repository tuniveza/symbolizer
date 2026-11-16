package main

// codex_fx.go: the look-and-move kit for Codex pages.
// It has 16 letter styles ("fonts"), big block letters (sizes),
// 16 ways a thing can move (animations) and a set of small animated symbols.
// Nothing here draws to the screen. It only says what to draw,
// so the page code can use it for text, pictures, drawings and symbols alike.

import (
	"math"
	"strings"
	"sync"

	"github.com/mattn/go-runewidth"
)

// ---------------------------------------------------------------- fonts

// codexFonts names the 16 letter styles.
// A terminal cannot change its font, so each style swaps letters for
// Unicode letters that look like that style.
var codexFonts = [16]string{
	"plain", "bold", "italic", "bold italic",
	"script", "bold script", "fraktur", "double",
	"mono", "sans", "sans bold", "small caps",
	"wide", "circled", "squared", "upside down",
}

const (
	fxFontPlain      = 0
	fxFontWide       = 12
	fxFontUpsideDown = 15
)

var (
	fxFontOnce sync.Once
	fxFontMaps [16]map[rune]rune
)

// fxMathMap builds a style from one of the Unicode "math letter" blocks.
// capA, lowA and zero are where A, a and 0 start (0 means the style has none).
// holes are letters that live somewhere else in Unicode for that style.
func fxMathMap(capA, lowA, zero rune, holes map[rune]rune) map[rune]rune {
	m := map[rune]rune{}
	for i := rune(0); i < 26; i++ {
		if capA != 0 {
			m['A'+i] = capA + i
		}
		if lowA != 0 {
			m['a'+i] = lowA + i
		}
	}
	if zero != 0 {
		for i := rune(0); i < 10; i++ {
			m['0'+i] = zero + i
		}
	}
	for k, v := range holes {
		m[k] = v
	}
	return m
}

// fxPairMap builds a style from two strings of the same length in runes:
// each rune of from turns into the rune at the same place in to.
func fxPairMap(from, to string) map[rune]rune {
	m := map[rune]rune{}
	f, t := []rune(from), []rune(to)
	for i := range f {
		if i < len(t) {
			m[f[i]] = t[i]
		}
	}
	return m
}

// fxFontSetup fills fxFontMaps once. Any swap that is not the right width
// (1 cell, or 2 cells for "wide") is dropped, so that letter stays plain.
func fxFontSetup() {
	m := &fxFontMaps
	m[0] = map[rune]rune{}
	m[1] = fxMathMap(0x1D400, 0x1D41A, 0x1D7CE, nil)
	m[2] = fxMathMap(0x1D434, 0x1D44E, 0, map[rune]rune{'h': 0x210E})
	m[3] = fxMathMap(0x1D468, 0x1D482, 0, nil)
	m[4] = fxMathMap(0x1D49C, 0x1D4B6, 0, map[rune]rune{
		'B': 0x212C, 'E': 0x2130, 'F': 0x2131, 'H': 0x210B, 'I': 0x2110,
		'L': 0x2112, 'M': 0x2133, 'R': 0x211B, 'e': 0x212F, 'g': 0x210A, 'o': 0x2134,
	})
	m[5] = fxMathMap(0x1D4D0, 0x1D4EA, 0, nil)
	m[6] = fxMathMap(0x1D504, 0x1D51E, 0, map[rune]rune{
		'C': 0x212D, 'H': 0x210C, 'I': 0x2111, 'R': 0x211C, 'Z': 0x2128,
	})
	m[7] = fxMathMap(0x1D538, 0x1D552, 0x1D7D8, map[rune]rune{
		'C': 0x2102, 'H': 0x210D, 'N': 0x2115, 'P': 0x2119, 'Q': 0x211A, 'R': 0x211D, 'Z': 0x2124,
	})
	m[8] = fxMathMap(0x1D670, 0x1D68A, 0x1D7F6, nil)
	m[9] = fxMathMap(0x1D5A0, 0x1D5BA, 0x1D7E2, nil)
	m[10] = fxMathMap(0x1D5D4, 0x1D5EE, 0x1D7EC, nil)
	// Small caps: little capital letters for the small ones; big ones stay.
	// q and x have no good small capital, so they stay as they are.
	m[11] = fxPairMap("abcdefghijklmnoprstuvwyz", "ᴀʙᴄᴅᴇꜰɢʜɪᴊᴋʟᴍɴᴏᴘʀꜱᴛᴜᴠᴡʏᴢ")
	// Wide: the full-width forms, 2 cells each, for all plain signs.
	wide := map[rune]rune{}
	for r := rune(0x21); r <= 0x7E; r++ {
		wide[r] = 0xFF01 + (r - 0x21)
	}
	m[12] = wide
	circ := fxMathMap(0x24B6, 0x24D0, 0, nil)
	for i := rune(1); i <= 9; i++ {
		circ['0'+i] = 0x2460 + i - 1
	}
	circ['0'] = 0x24EA
	m[13] = circ
	// Squared: there are only squared capitals, so small letters use them too.
	sq := fxMathMap(0x1F130, 0x1F130, 0, nil)
	m[14] = sq
	m[15] = fxPairMap(
		"abcdefghijklmnopqrstuvwxyz"+"ABCDEFGHIJKLMNOPQRSTUVWXYZ"+"0123456789"+".,'!?()[]{}<>_&\"",
		"ɐqɔpǝɟƃɥᴉɾʞlɯuodbɹsʇnʌʍxʎz"+"∀ᗺƆᗡƎℲ⅁HIſʞ˥WNOԀὉᴚS⊥∩ΛMX⅄Z"+"0ƖᘔƐᔭϛ9Ꞁ86"+"˙',¡¿)(][}{><‾⅋„",
	)
	for i := range m {
		want := 1
		if i == fxFontWide {
			want = 2
		}
		for k, v := range m[i] {
			if runewidth.RuneWidth(v) != want {
				delete(m[i], k)
			}
		}
	}
}

// codexFont turns the letters and digits of s into the given style.
// Runes the style has no swap for stay as they are.
// A font number out of range means plain.
// "upside down" also turns each line back to front, so it reads when flipped.
func codexFont(s string, font int) string {
	if font <= fxFontPlain || font >= len(codexFonts) {
		return s
	}
	fxFontOnce.Do(fxFontSetup)
	m := fxFontMaps[font]
	if font == fxFontUpsideDown {
		lines := strings.Split(s, "\n")
		for li, line := range lines {
			rs := []rune(line)
			out := make([]rune, len(rs))
			for i, r := range rs {
				if v, ok := m[r]; ok {
					r = v
				}
				out[len(rs)-1-i] = r
			}
			lines[li] = string(out)
		}
		return strings.Join(lines, "\n")
	}
	var b strings.Builder
	b.Grow(len(s) * 3)
	for _, r := range s {
		if v, ok := m[r]; ok {
			r = v
		}
		b.WriteRune(r)
	}
	return b.String()
}

// codexWidth reports how many terminal cells s takes.
func codexWidth(s string) int {
	return runewidth.StringWidth(s)
}

// ---------------------------------------------------------------- sizes

// codexSizes is how many sizes there are: 1 normal, 2 big, 3 large, 4 huge.
const codexSizes = 4

// fxPixRows is how tall a letter in the pixel font is.
// One more empty row sits under each letter, so a line is 6 pixels tall,
// which is 3 terminal rows with half blocks.
const fxPixRows = 5

// fxPixFont is the small pixel font. Each letter is 5 rows; '#' is on.
// Most letters are 3 wide; a few (M, N, W, @) need more room.
// Small letters use the capitals.
var fxPixFont = map[rune][fxPixRows]string{
	'A':  {".#.", "#.#", "###", "#.#", "#.#"},
	'B':  {"##.", "#.#", "##.", "#.#", "##."},
	'C':  {".##", "#..", "#..", "#..", ".##"},
	'D':  {"##.", "#.#", "#.#", "#.#", "##."},
	'E':  {"###", "#..", "##.", "#..", "###"},
	'F':  {"###", "#..", "##.", "#..", "#.."},
	'G':  {".##", "#..", "#.#", "#.#", ".##"},
	'H':  {"#.#", "#.#", "###", "#.#", "#.#"},
	'I':  {"###", ".#.", ".#.", ".#.", "###"},
	'J':  {"..#", "..#", "..#", "#.#", ".#."},
	'K':  {"#.#", "#.#", "##.", "#.#", "#.#"},
	'L':  {"#..", "#..", "#..", "#..", "###"},
	'M':  {"#...#", "##.##", "#.#.#", "#...#", "#...#"},
	'N':  {"#..#", "##.#", "#.##", "#..#", "#..#"},
	'O':  {".#.", "#.#", "#.#", "#.#", ".#."},
	'P':  {"##.", "#.#", "##.", "#..", "#.."},
	'Q':  {".#.", "#.#", "#.#", "##.", ".##"},
	'R':  {"##.", "#.#", "##.", "#.#", "#.#"},
	'S':  {".##", "#..", ".#.", "..#", "##."},
	'T':  {"###", ".#.", ".#.", ".#.", ".#."},
	'U':  {"#.#", "#.#", "#.#", "#.#", "###"},
	'V':  {"#.#", "#.#", "#.#", "#.#", ".#."},
	'W':  {"#...#", "#...#", "#.#.#", "##.##", "#...#"},
	'X':  {"#.#", "#.#", ".#.", "#.#", "#.#"},
	'Y':  {"#.#", "#.#", ".#.", ".#.", ".#."},
	'Z':  {"###", "..#", ".#.", "#..", "###"},
	'0':  {"###", "#.#", "#.#", "#.#", "###"},
	'1':  {".#.", "##.", ".#.", ".#.", "###"},
	'2':  {"##.", "..#", ".#.", "#..", "###"},
	'3':  {"##.", "..#", ".#.", "..#", "##."},
	'4':  {"#.#", "#.#", "###", "..#", "..#"},
	'5':  {"###", "#..", "##.", "..#", "##."},
	'6':  {".##", "#..", "###", "#.#", "###"},
	'7':  {"###", "..#", ".#.", ".#.", ".#."},
	'8':  {"###", "#.#", "###", "#.#", "###"},
	'9':  {"###", "#.#", "###", "..#", "##."},
	' ':  {"..", "..", "..", "..", ".."},
	'.':  {".", ".", ".", ".", "#"},
	',':  {"..", "..", "..", ".#", "#."},
	'!':  {"#", "#", "#", ".", "#"},
	'?':  {"##.", "..#", ".#.", "...", ".#."},
	':':  {".", "#", ".", "#", "."},
	';':  {"..", ".#", "..", ".#", "#."},
	'\'': {"#", "#", ".", ".", "."},
	'"':  {"#.#", "#.#", "...", "...", "..."},
	'-':  {"...", "...", "###", "...", "..."},
	'+':  {"...", ".#.", "###", ".#.", "..."},
	'=':  {"...", "###", "...", "###", "..."},
	'/':  {"..#", "..#", ".#.", "#..", "#.."},
	'(':  {".#", "#.", "#.", "#.", ".#"},
	')':  {"#.", ".#", ".#", ".#", "#."},
	'[':  {"##", "#.", "#.", "#.", "##"},
	']':  {"##", ".#", ".#", ".#", "##"},
	'<':  {"..#", ".#.", "#..", ".#.", "..#"},
	'>':  {"#..", ".#.", "..#", ".#.", "#.."},
	'_':  {"...", "...", "...", "...", "###"},
	'&':  {".#.", "#.#", ".#.", "#.#", ".##"},
	'#':  {"#.#", "###", "#.#", "###", "#.#"},
	'@':  {".##.", "#..#", "#.##", "#...", ".##."},
	'*':  {"...", "#.#", ".#.", "#.#", "..."},
	'%':  {"#.#", "..#", ".#.", "#..", "#.#"},
}

// fxPixBox is drawn for any rune the pixel font does not know.
var fxPixBox = [fxPixRows]string{"...", "###", "#.#", "###", "..."}

func fxPixGlyph(r rune) [fxPixRows]string {
	if r == '\t' {
		r = ' '
	}
	if r >= 'a' && r <= 'z' {
		r -= 'a' - 'A'
	}
	if g, ok := fxPixFont[r]; ok {
		return g
	}
	return fxPixBox
}

// codexBig draws s in big block letters and gives back the rows.
// Size 2 is the pixel font as it is (3 rows a line), size 3 doubles it
// (6 rows) and size 4 triples it (9 rows). The gap between letters grows
// with the size too (1, 2, 3 columns). Each line of s gives its own block
// of rows. All rows are padded with spaces to the same width.
// Size 1 or less gives s back unchanged, so fonts still show at size 1.
func codexBig(s string, size int) []string {
	if size <= 1 {
		return []string{s}
	}
	if size > codexSizes {
		size = codexSizes
	}
	k := size - 1
	var rows []string
	for _, line := range strings.Split(s, "\n") {
		// px[y] is one pixel row of the whole line, already scaled.
		px := make([][]bool, (fxPixRows+1)*k)
		first := true
		for _, r := range line {
			g := fxPixGlyph(r)
			if !first {
				for y := range px {
					for i := 0; i < k; i++ {
						px[y] = append(px[y], false)
					}
				}
			}
			first = false
			for gy := 0; gy < fxPixRows; gy++ {
				for _, c := range g[gy] {
					on := c == '#'
					for sy := 0; sy < k; sy++ {
						for sx := 0; sx < k; sx++ {
							px[gy*k+sy] = append(px[gy*k+sy], on)
						}
					}
				}
			}
			// The empty row under the letter.
			for y := fxPixRows * k; y < len(px); y++ {
				for i := 0; i < len([]rune(g[0]))*k; i++ {
					px[y] = append(px[y], false)
				}
			}
		}
		w := len(px[0])
		for y := 0; y+1 < len(px); y += 2 {
			var b strings.Builder
			for x := 0; x < w; x++ {
				top, bot := px[y][x], px[y+1][x]
				switch {
				case top && bot:
					b.WriteRune('█')
				case top:
					b.WriteRune('▀')
				case bot:
					b.WriteRune('▄')
				default:
					b.WriteByte(' ')
				}
			}
			rows = append(rows, b.String())
		}
	}
	return fxPadRows(rows)
}

// fxPadRows pads every row with spaces to the width of the widest one.
func fxPadRows(rows []string) []string {
	w := 0
	for _, r := range rows {
		if n := runewidth.StringWidth(r); n > w {
			w = n
		}
	}
	for i, r := range rows {
		if n := runewidth.StringWidth(r); n < w {
			rows[i] = r + strings.Repeat(" ", w-n)
		}
	}
	return rows
}

// ---------------------------------------------------------------- animations

// codexAnims names the 16 ways an element can move.
var codexAnims = [16]string{"none", "blink", "pulse", "wave", "rainbow", "typewriter", "marquee", "bounce", "shake", "fade in", "glow", "sparkle", "flicker", "slide in", "jelly", "glitch"}

const (
	fxAnimNone = iota
	fxAnimBlink
	fxAnimPulse
	fxAnimWave
	fxAnimRainbow
	fxAnimTypewriter
	fxAnimMarquee
	fxAnimBounce
	fxAnimShake
	fxAnimFadeIn
	fxAnimGlow
	fxAnimSparkle
	fxAnimFlicker
	fxAnimSlideIn
	fxAnimJelly
	fxAnimGlitch
)

// How fast the settling animations go.
const (
	fxTypeRate   = 25.0 // typewriter: cells shown each second
	fxFadeInTime = 1.2  // fade in: seconds until fully there
	fxSlideTime  = 0.8  // slide in: seconds until in place
)

// fxPose says how one cell looks at one moment.
type fxPose struct {
	Hide    bool    // this cell is not drawn this moment
	DX, DY  int     // the cell is drawn moved by this much (cells)
	Tint    string  // "" none; else a colour to mix into the cell's foreground
	TintAmt float64 // how much of Tint (0..1)
	Fade    float64 // 0 as it is … 1 fully faded into the background
	Glyph   string  // "" keep the cell's letter; else draw this 1-cell letter instead
}

// fxHash mixes some numbers into one random-looking number.
// The same numbers always give the same answer.
func fxHash(vals ...int) uint32 {
	h := uint32(2166136261)
	for _, v := range vals {
		h ^= uint32(v)
		h *= 16777619
		h ^= h >> 13
	}
	h ^= h >> 16
	h *= 0x85ebca6b
	h ^= h >> 13
	h *= 0xc2b2ae35
	h ^= h >> 16
	return h
}

// fxRand is fxHash turned into a number from 0 up to (not including) 1.
func fxRand(vals ...int) float64 {
	return float64(fxHash(vals...)) / 4294967296.0
}

// fxSlot counts which step of length 1/rate seconds t falls in.
func fxSlot(t, rate float64) int {
	return int(math.Floor(t * rate))
}

func fxClamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

func fxClampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func fxPick(list string, n uint32) string {
	rs := []rune(list)
	return string(rs[int(n%uint32(len(rs)))])
}

var fxSparkleSteps = []string{"·", "✧", "✦", "✧", "⋆"}

// codexPose says how the cell at (x, y) of a w×h element looks
// t seconds after its animation started.
// seed changes the random-looking ones (sparkle, flicker, glitch) for each
// element, but the same inputs always give the same pose.
//
// Marquee: DX is set so that x+DX, taken modulo w, lands back inside 0..w-1,
// so the text scrolls left and comes back in on the right.
func codexPose(anim, x, y, w, h int, t float64, seed int) fxPose {
	var p fxPose
	if w < 1 {
		w = 1
	}
	if h < 1 {
		h = 1
	}
	if t < 0 {
		t = 0
	}
	switch anim {
	case fxAnimBlink:
		// Shown for the first half of each 1.2 s, hidden for the rest.
		p.Hide = math.Mod(t, 1.2) >= 0.6

	case fxAnimPulse:
		p.Fade = 0.3 * (1 - math.Cos(2*math.Pi*t/1.6))

	case fxAnimWave:
		p.DY = int(math.Round(math.Sin(float64(x)*0.6 - t*6)))

	case fxAnimRainbow:
		hue := math.Mod(float64(x)*18+float64(y)*10-t*120, 360)
		if hue < 0 {
			hue += 360
		}
		p.Tint = hslHex(hue, 0.85, 0.62)
		p.TintAmt = 0.85

	case fxAnimTypewriter:
		shown := int(t * fxTypeRate)
		idx := y*w + x
		switch {
		case idx < shown:
		case idx == shown && shown < w*h:
			p.Glyph = "▌" // the cursor, just ahead of the text
			p.Tint = "#ffffff"
			p.TintAmt = 0.5
		default:
			p.Hide = true
		}

	case fxAnimMarquee:
		off := fxSlot(t, 8) % w
		nx := ((x-off)%w + w) % w
		p.DX = nx - x

	case fxAnimBounce:
		// A ball arc for 0.75 s, then a short rest on the ground.
		ph := math.Mod(t, 1.0) / 0.75
		if ph < 1 {
			p.DY = -int(math.Round(2 * 4 * ph * (1 - ph)))
		}

	case fxAnimShake:
		// A 0.4 s shake every 2 s; the whole element moves as one.
		cyc := math.Mod(t+float64(seed%7)*0.13, 2.0)
		if cyc < 0.4 {
			p.DX = []int{-1, 1, 0, 1, -1, 0, 1, -1}[fxSlot(t, 20)%8]
		}

	case fxAnimFadeIn:
		q := fxClamp01(t / fxFadeInTime)
		p.Fade = 1 - q*q*(3-2*q)

	case fxAnimGlow:
		p.Tint = "#ffffff"
		p.TintAmt = 0.25 * (1 - math.Cos(2*math.Pi*t/2.0-float64(x)*0.15))

	case fxAnimSparkle:
		// A cell lights up now and then and runs through a few star steps.
		slot := fxSlot(t, 10)
		for k := range fxSparkleSteps {
			if fxRand(11, seed, x, y, slot-k) < 0.018 {
				p.Glyph = fxSparkleSteps[k]
				p.Tint = "#ffffff"
				p.TintAmt = 0.9
				break
			}
		}

	case fxAnimFlicker:
		// Like a broken light: some seconds are calm, some stutter.
		sec := fxSlot(t, 1)
		slot := fxSlot(t, 12)
		chance := 0.03
		if fxRand(12, seed, sec) < 0.45 {
			chance = 0.35
		}
		r := fxRand(13, seed, slot)
		if r < chance {
			p.Hide = true
		} else if r < chance*1.6 {
			p.Fade = 0.45
		}

	case fxAnimSlideIn:
		q := fxClamp01(t / fxSlideTime)
		e := 1 - (1-q)*(1-q)*(1-q)
		p.DX = int(math.Round(float64(w) * (1 - e)))

	case fxAnimJelly:
		// The bottom row stays put; rows higher up sway more.
		amp := 1.0
		if h > 1 {
			amp = 1.5 * float64(h-1-y) / float64(h-1)
		}
		p.DX = int(math.Round(amp * math.Sin(t*5-float64(y)*0.9)))
		if y == 0 {
			p.DY = int(math.Round(0.6 * math.Sin(t*10+float64(x)*0.4)))
		}

	case fxAnimGlitch:
		// Mostly calm. Now and then a short burst: rows jump sideways and
		// some cells turn into noise blocks in red or cyan.
		half := fxSlot(t, 2)
		if fxRand(15, seed, half) < 0.3 && t-float64(half)*0.5 < 0.22 {
			slot := fxSlot(t, 12)
			if fxRand(16, seed, y, slot) < 0.4 {
				n := 1 + int(fxRand(17, seed, y, slot)*3)
				if fxHash(18, seed, y, slot)&1 == 0 {
					n = -n
				}
				p.DX = n
			}
			if fxRand(19, seed, x, y, slot) < 0.12 {
				p.Glyph = fxPick("░▒▓█▚▞", fxHash(20, seed, x, y, slot))
				p.Tint = "#ff3355"
				if fxHash(21, seed, x, y, slot)&1 == 0 {
					p.Tint = "#33ffee"
				}
				p.TintAmt = 0.7
			}
		}
	}
	p.DX = fxClampInt(p.DX, -w, w)
	p.DY = fxClampInt(p.DY, -w, w)
	p.Fade = fxClamp01(p.Fade)
	p.TintAmt = fxClamp01(p.TintAmt)
	return p
}

// codexAnimLoops reports whether anim keeps moving forever.
// Typewriter, fade in and slide in settle after a while; "none" never moves.
func codexAnimLoops(anim int) bool {
	switch anim {
	case fxAnimNone, fxAnimTypewriter, fxAnimFadeIn, fxAnimSlideIn:
		return false
	}
	return anim > 0 && anim < len(codexAnims)
}

// codexAnimDone reports whether a settling animation is over by time t
// for a w×h element, so the caller can stop redrawing it.
// Looping ones are never done; "none" (or an unknown number) is done at once.
func codexAnimDone(anim, w, h int, t float64) bool {
	switch anim {
	case fxAnimTypewriter:
		if w < 1 {
			w = 1
		}
		if h < 1 {
			h = 1
		}
		return int(t*fxTypeRate) >= w*h
	case fxAnimFadeIn:
		return t >= fxFadeInTime
	case fxAnimSlideIn:
		return t >= fxSlideTime
	}
	return !codexAnimLoops(anim)
}

// ---------------------------------------------------------------- symbols

// codexSymbol is a small animated icon: a few frames of one text cell.
type codexSymbol struct {
	Name   string
	Frames []string
	FPS    float64
	Color  string // a nice default "#rrggbb"
}

func fxFrames(s string) []string {
	var out []string
	for _, r := range s {
		out = append(out, string(r))
	}
	return out
}

// codexSymbols is the list of animated symbols. Every frame is one cell wide.
var codexSymbols = []codexSymbol{
	{"spinner", fxFrames("⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏"), 12, "#7fd1ff"},
	{"dots", fxFrames("⣾⣽⣻⢿⡿⣟⣯⣷"), 10, "#9be37a"},
	{"pulse", fxFrames("·•●•"), 5, "#ff6b6b"},
	{"heart", fxFrames("♡♥♥♡"), 3, "#ff4d6d"},
	{"star", fxFrames("✧✦★✦"), 4, "#ffd166"},
	{"moon", fxFrames("◐◓◑◒"), 3, "#e8e4c9"},
	{"clock", fxFrames("◷◶◵◴"), 2, "#c9b8ff"},
	{"wave", fxFrames("▁▂▃▄▅▆▇█▇▆▅▄▃▂"), 12, "#4cc9f0"},
	{"hourglass", fxFrames("⧗⧖"), 1, "#e9c46a"},
	{"arrows", fxFrames("←↖↑↗→↘↓↙"), 6, "#90e0ef"},
	{"music", fxFrames("♪♫"), 2, "#f72585"},
	{"snow", fxFrames("❄❅❆❅"), 3, "#caf0f8"},
	{"flame", fxFrames("^▴▲▴"), 8, "#ff7b25"},
	{"eye", fxFrames("◉◉◉◉◉◡◉◉"), 4, "#80ffdb"},
	{"bounce", fxFrames("⠁⠂⠄⡀⠄⠂"), 8, "#ffafcc"},
	{"sun", fxFrames("☀☼"), 2, "#ffbe0b"},
	{"lightning", fxFrames("ϟϟ ϟ   "), 8, "#fff275"},
	{"gear", fxFrames("⚙✲"), 3, "#adb5bd"},
	{"flower", fxFrames("✿❀❁❀"), 3, "#ff8fab"},
	{"diamond", fxFrames("◇◈◆◈"), 4, "#72efdd"},
	{"chevron", fxFrames(" ›»›"), 5, "#ffd60a"},
	{"loading", fxFrames("▏▎▍▌▋▊▉█"), 8, "#06d6a0"},
	{"cross", fxFrames("✚✛✜✛"), 4, "#ef476f"},
	{"arc", fxFrames("◜◝◞◟"), 8, "#a0c4ff"},
	{"triangle", fxFrames("◢◣◤◥"), 4, "#bdb2ff"},
	{"square", fxFrames("□▣■▣"), 4, "#ffc6ff"},
	{"quadrant", fxFrames("▖▘▝▗"), 6, "#fdffb6"},
	{"dice", fxFrames("⚀⚁⚂⚃⚄⚅"), 4, "#f1faee"},
	{"ripple", fxFrames("·∘○◯○∘"), 6, "#48cae4"},
	{"sparkle", fxFrames("·✧✦✧"), 6, "#fff3b0"},
	{"fill", fxFrames("⠀⠁⠉⠋⠛⠟⠿⡿⣿"), 8, "#b5e48c"},
	{"toggle", fxFrames("⊶⊷"), 2, "#cdb4db"},
}

// fxSymbolShape says which outline each symbol fills when drawn big.
var fxSymbolShape = map[string]string{
	"heart": "heart", "star": "star", "pulse": "circle", "moon": "circle",
	"clock": "circle", "eye": "circle", "ripple": "ring", "sun": "star",
	"arrows": "ring", "spinner": "ring", "dots": "ring", "arc": "ring",
	"wave": "square", "loading": "square", "fill": "square", "square": "square",
	"quadrant": "square", "dice": "square", "toggle": "square",
	"flame": "triangle", "triangle": "triangle", "hourglass": "hourglass",
	"cross": "plus", "snow": "x", "sparkle": "star", "lightning": "x",
	"gear": "ring", "flower": "circle", "diamond": "diamond",
	"chevron": "chevron", "music": "diamond", "bounce": "diamond",
}

// fxFrameAt picks the frame of sym for time t.
func fxFrameAt(sym codexSymbol, t float64) string {
	n := len(sym.Frames)
	if n == 0 {
		return ""
	}
	f := fxSlot(t, sym.FPS) % n
	if f < 0 {
		f += n
	}
	return sym.Frames[f]
}

func fxSymbolAt(i int) (codexSymbol, bool) {
	n := len(codexSymbols)
	if n == 0 {
		return codexSymbol{}, false
	}
	return codexSymbols[((i%n)+n)%n], true
}

// codexSymbolFrame gives the frame of symbol i at time t (i wraps around).
func codexSymbolFrame(i int, t float64) string {
	sym, ok := fxSymbolAt(i)
	if !ok {
		return ""
	}
	return fxFrameAt(sym, t)
}

// fxInShape says if the point (x, y) is inside the named outline.
// x and y go from -1 to 1; y grows downwards.
func fxInShape(shape string, x, y float64) bool {
	ax, ay := math.Abs(x), math.Abs(y)
	r := math.Hypot(x, y)
	switch shape {
	case "square":
		return true
	case "circle":
		return r <= 1.08
	case "ring":
		return r <= 1.08 && r >= 0.55
	case "plus":
		return ax <= 0.34 || ay <= 0.34
	case "x":
		return math.Abs(ax-ay) <= 0.36
	case "triangle":
		// Point at the top, wide at the bottom.
		return ax <= (y+1)/2+0.1
	case "hourglass":
		return ax <= ay+0.1
	case "chevron":
		// An arrow head that points right.
		d := x + 0.4 - (1 - ay)
		return d <= 0.35 && d >= -0.55
	case "heart":
		hx, hy := x*1.25, -y*1.1+0.1
		v := hx*hx + hy*hy - 1
		return v*v*v-hx*hx*hy*hy*hy <= 0
	case "star":
		// A four-point star: thin arms along the middle lines.
		return math.Sqrt(ax)+math.Sqrt(ay) <= 1.15
	}
	// diamond
	return ax+ay <= 1.05
}

// codexSymbolBig draws symbol i bigger, for sizes 2 to 4: a block
// size*2-1 rows high and twice that wide less one, filled in the symbol's
// own outline with its frame. Cells further from the middle show a slightly
// later frame, so the motion ripples out from the centre.
// Size 1 or less gives just the one frame. All rows are the same width.
func codexSymbolBig(i int, t float64, size int) []string {
	sym, ok := fxSymbolAt(i)
	if !ok {
		return []string{""}
	}
	if size <= 1 {
		return []string{fxFrameAt(sym, t)}
	}
	if size > codexSizes {
		size = codexSizes
	}
	n := size*2 - 1
	w := n*2 - 1
	shape := fxSymbolShape[sym.Name]
	cy, cx := float64(n-1)/2, float64(w-1)/2
	rows := make([]string, n)
	for r := 0; r < n; r++ {
		var b strings.Builder
		for c := 0; c < w; c++ {
			x, y := (float64(c)-cx)/cx, (float64(r)-cy)/cy
			if !fxInShape(shape, x, y) {
				b.WriteByte(' ')
				continue
			}
			d := math.Hypot(x, y)
			f := fxFrameAt(sym, t-d*0.6/math.Max(sym.FPS, 1))
			if runewidth.StringWidth(f) != 1 {
				f = " "
			}
			b.WriteString(f)
		}
		rows[r] = b.String()
	}
	return rows
}
