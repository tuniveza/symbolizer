package main

// The flat board and the 3D room move together: one openness, one clock
// (design_anim.go). These tests stop the clock, step it through each move,
// and at every step draw the board flat and then in 3D (turning between
// them mid-move), checking the 3D model is the 3D form of the flat frame.

import (
	"math"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
)

// animClock stops the opening and shutting clock for a test (and turns
// moving on); the function it gives moves the clock on.
func animClock(t *testing.T) func(time.Duration) {
	now := time.Unix(1_000_000, 0)
	animNow = func() time.Time { return now }
	designAnims = true
	t.Cleanup(func() { animNow, designAnims = time.Now, false })
	return func(d time.Duration) { now = now.Add(d) }
}

// animSteps are the moments (parts of openAnimFor) the tests look at.
var animSteps = []float64{0, 0.05, 0.15, 0.3, 0.5, 0.7, 0.9, 1.05}

// A move turned round part way goes back from exactly where it is drawn,
// and one started again the same way goes on as it was.
func TestAnimTurnsRoundWhereItIs(t *testing.T) {
	step := animClock(t)
	d := &designState{}
	o := &dOct{ID: 7, Kind: kCabinet, DrawerOpen: true}
	d.startOpenAnim(o)
	step(openAnimFor / 4)
	was := d.openness(o)
	d.startOpenAnim(o) // the same way again: nothing changes
	if got := d.openness(o); got != was {
		t.Fatalf("started again the same way, it jumped from %.3f to %.3f", was, got)
	}
	o.DrawerOpen = false
	d.startOpenAnim(o)
	if got := d.openness(o); math.Abs(got-was) > 1e-6 {
		t.Fatalf("turned round at %.3f, it jumped to %.3f", was, got)
	}
	last := was
	for range 40 {
		step(openAnimFor / 32)
		p := d.openness(o)
		if p > last+1e-12 {
			t.Fatalf("shutting, it opened again: %.3f after %.3f", p, last)
		}
		last = p
	}
	if last != 0 {
		t.Fatalf("shut at the end, it is %.3f open", last)
	}
}

// knobFrac is how much of the cabinet's left door faces the front in the
// flat drawing (1 shut, 0 edge on), read off where its knob ● is drawn.
func knobFrac(t *testing.T, rows []string, d *designState, cab *dOct) float64 {
	t.Helper()
	x, y, w, h := d.rect(cab)
	line := rows[y+3+(h-4)/2]
	col := 0
	for _, r := range ansi.Strip(line) {
		if r == '●' && col > x && col < x+w/2 {
			dw := col - x + 1 // the door's width: its knob is its last cell but one
			return float64(dw-cabDoorW) / float64((w-2)/2-cabDoorW)
		}
		col += ansi.StringWidth(string(r))
	}
	t.Fatalf("no knob on the cabinet's left door: %q", ansi.Strip(line))
	return 0
}

// boxLidFlat is how many inside rows the flat drawing's lid covers: the
// rows from the top where none of the box's floor shows.
func boxLidFlat(g *boxGrid, rows []int) int {
	n := 0
	for _, row := range rows {
		for _, c := range g.c[row] {
			if c.act == "boxfloor" {
				return n
			}
		}
		n++
	}
	return n
}

// ---- every kind together, every way of starting a move ----------------------------------
