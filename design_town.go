package main

// ---- windows in the walls, with the town outside --------------------------------------
//
// Every room has a few windows (1 to 4, picked at random once for the room,
// the same every time), each on one of its walls, high up over the middle
// of the board. Through each you see buildings across the street, with
// little people made of letters going about: walking past, at their own
// windows, a cat on a ledge (design_windows_art.go draws them). The floor
// has none. Structures stand in front of a window, as on a real wall.
//
// The people move a step every windowsEvery while a window is on the screen;
// with none on the screen nothing ticks.

import (
	"image/color"
	"math"
	"time"

	tea "charm.land/bubbletea/v2"
	uv "github.com/charmbracelet/ultraviolet"
)

const (
	windowW, windowH = 44, 15                 // a window's size on the board, at 100%
	windowGap        = 14                     // between two windows on one wall
	windowTop        = 3                      // its top edge: near the top of a new board's first view
	windowMid        = 100                    // the middle of them, across: about the middle of that view
	windowsEvery     = 220 * time.Millisecond // how often the people outside move on
)

// townShown is whether walls have windows (tests turn it off: most check
// the board alone).
var townShown = true

// windowsStart is when the people outside started (their clock).
var windowsStart = time.Now()

// windowsTickMsg: the people outside take their next step.
type windowsTickMsg struct{}

// roomSeed is room id's own random number: its windows come from it.
func roomSeed(id int) uint64 {
	x := uint64(id)*0x9E3779B97F4A7C15 + 0x5EED
	x ^= x >> 31
	x *= 0xBF58476D1CE4E5B9
	return x ^ x>>29
}

// wallWindow is a window on the wall on screen: where it is on the board
// (100% units) and its seed.
type wallWindow struct {
	x, y int
	seed uint64
}

// windowsHere are the windows on the wall you are looking at (none on the floor).
func (m *Model) windowsHere() []wallWindow {
	h := m.hs()
	r := h.room(h.Current)
	if r == nil || r.Down || !townShown {
		return nil
	}
	seed := roomSeed(r.ID)
	n, walls := winCount(seed), r.walls()
	var mine []uint64
	for i := range n {
		s := winSeed(seed, i)
		if int(s>>40)%walls == r.at() {
			mine = append(mine, s)
		}
	}
	if len(mine) == 0 && r.at() == 0 && n > 0 {
		mine = append(mine, winSeed(seed, 0)) // the first wall always has one
	}
	out := make([]wallWindow, len(mine))
	total := len(mine)*windowW + (len(mine)-1)*windowGap
	for i, s := range mine {
		out[i] = wallWindow{x: windowMid - total/2 + i*(windowW+windowGap), y: windowTop, seed: s}
	}
	return out
}

// paintWindows draws the windows of the wall on screen onto the empty
// board, under the structures.
func (m *Model) paintWindows(c *boardCanvas) {
	d := m.ds()
	z := d.z()
	t := time.Since(windowsStart).Seconds()
	if !designAnims {
		t = 0 // tests: the same picture every time
	}
	light := isLightTheme()
	cw, ch := c.Width(), c.Height()
	colours := map[string]color.Color{}
	rgba := func(hex string) color.Color {
		col, ok := colours[hex]
		if !ok {
			r, g, b := rgb(hex)
			col = color.RGBA{uint8(r), uint8(g), uint8(b), 255}
			colours[hex] = col
		}
		return col
	}
	for _, w := range m.windowsHere() {
		sx := int(math.Round((float64(w.x) - d.camX) * z))
		sy := int(math.Round((float64(w.y) - d.camY) * z))
		ww, wh := int(math.Round(windowW*z)), int(math.Round(windowH*z))
		if ww < 3 || wh < 2 || sx >= cw || sy >= ch || sx+ww <= 0 || sy+wh <= 0 {
			continue
		}
		art := winArt(w.seed, ww, wh, t, light)
		for y, row := range art {
			py := sy + y
			if py < 0 || py >= ch {
				continue
			}
			line := c.Lines[py]
			for x, cell := range row {
				px := sx + x
				if px < 0 || px >= cw {
					continue
				}
				st := line[px].Style
				if cell.Fg != "" {
					st.Fg = rgba(cell.Fg)
				}
				if cell.Bg != "" {
					st.Bg = rgba(cell.Bg)
				}
				ch := string(cell.R)
				if cell.R == 0 {
					ch = " "
				}
				line[px] = uv.Cell{Content: ch, Width: 1, Style: st}
			}
		}
	}
}

// windowsOnScreen is whether a window of the wall on screen shows.
func (m *Model) windowsOnScreen() bool {
	d := m.design
	if d == nil {
		return false
	}
	z := d.z()
	for _, w := range m.windowsHere() {
		sx := (float64(w.x) - d.camX) * z
		sy := (float64(w.y) - d.camY) * z
		if sx < float64(m.width) && sy < float64(m.boardH()) && sx+windowW*z > 0 && sy+windowH*z > 0 && windowW*z >= 3 {
			return true
		}
	}
	return false
}

// windowsCmd asks for the people's next step while a window shows.
func (m *Model) windowsCmd() tea.Cmd {
	if !designAnims || !m.inDesign() || m.design == nil || m.windowsTick || m.help || !m.windowsOnScreen() {
		return nil
	}
	m.windowsTick = true
	return tea.Tick(windowsEvery, func(time.Time) tea.Msg { return windowsTickMsg{} })
}

// windowsTicked is the tick: the next View draws the people further on.
func (m *Model) windowsTicked() {
	m.windowsTick = false
	if m.design != nil && m.spin == nil {
		m.design.frameAt = time.Time{}
	}
}
