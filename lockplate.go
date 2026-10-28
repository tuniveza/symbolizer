package main

import (
	"fmt"
	"math"
	"math/rand/v2"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// The lock plate: four little keycaps on a dark resin plate, drawn in letters, like the desktop's
// emberline-locks plate. It flashes up in a corner of the text when Caps, Num, Scroll or Fn lock
// changes while sy fills the screen (lamps.go decides that). A lock that is on has its lamp lit and
// its letter glowing like a coal; one that is off sits dark and ash-grey. The key just pressed
// flares: turning on, it throws a few sparks up; turning off, a couple of drops fall off it.
// A small window gets a one-line plate instead.

var plateKeys = []struct{ key, glyph, label string }{
	{"caps", "A", "CAPS"}, {"num", "1", "NUM"}, {"scroll", "⇕", "SCROLL"}, {"fn", "Fn", "FN"},
}

const (
	capW, capH      = 10, 5 // one keycap, border included
	capGap          = 1
	platePadX       = 2
	platePadY       = 1
	plateW          = 2 + 2*platePadX + 4*capW + 3*capGap
	plateH          = 2 + 2*platePadY + capH
	plateIn         = 160 * time.Millisecond  // fade and rise in
	plateHold       = 1600 * time.Millisecond // stays this long after the last change
	plateOut        = 350 * time.Millisecond  // fade out
	plateFlare      = 750 * time.Millisecond  // the pressed key's flare
	plateFrameEvery = 30 * time.Millisecond
	plateSmallH     = 3 // rows of the one-line plate
)

// The Emberline colours, the same on every theme (the plate is an object lying on the page).
var (
	pHot  = rgbf{255, 184, 107} // white-hot core
	pGold = rgbf{255, 138, 26}  // ff8a1a
	pLava = rgbf{255, 84, 20}   // ff5414
	pDeep = rgbf{204, 36, 8}    // red ember
	pAsh  = rgbf{94, 43, 18}    // 5e2c12
	pCoal = rgbf{26, 18, 15}
)

type spark struct {
	x, y, vx, vy, g float64 // cells and cells a second
	born            time.Time
	life            float64
	rising          bool
}

type lockPlate struct {
	shown, changed time.Time // came up / the latest change (it holds from then, then fades)
	flared         string
	flareT         time.Time
	glow           map[string]float64 // drawn heat per key, easing to its state
	sparks         []spark
	last           time.Time
	x, y           int
	small          bool
	gen            int // frames of an older chain are dropped
}

type plateFrame int

func easeOut(p float64) float64 {
	p = min(max(p, 0), 1)
	return 1 - math.Pow(1-p, 3)
}

func (p *lockPlate) alpha(now time.Time) float64 {
	a := easeOut(float64(now.Sub(p.shown)) / float64(plateIn))
	if out := p.changed.Add(plateIn + plateHold); now.After(out) {
		a = min(a, 1-easeOut(float64(now.Sub(out))/float64(plateOut)))
	}
	return a
}

func (p *lockPlate) over(now time.Time) bool {
	return !now.Before(p.changed.Add(plateIn + plateHold + plateOut))
}

// showPlate brings the plate up (or keeps it up) for a change of key.
func (m *Model) showPlate(key string) tea.Cmd {
	now := time.Now()
	p := m.plate
	if p == nil || p.over(now) {
		p = &lockPlate{shown: now, last: now, glow: map[string]float64{}}
		for _, k := range plateKeys {
			on := lockOn(m.lamps, k.key)
			if k.key == key {
				on = !on // it starts from where it was and warms up or cools down
			}
			p.glow[k.key] = b2f(on)
		}
		if !m.placePlate(p) {
			return nil
		}
		m.plate = p
	} else if a := p.alpha(now); now.After(p.changed.Add(plateIn + plateHold)) {
		p.shown = now.Add(-time.Duration(a * float64(plateIn))) // back from fading out: from where it got to
	}
	p.changed, p.flared, p.flareT = now, key, now
	if !p.small {
		p.throwSparks(key, lockOn(m.lamps, key), now)
	}
	p.gen++
	gen := p.gen
	return tea.Tick(plateFrameEvery, func(time.Time) tea.Msg { return plateFrame(gen) })
}

func b2f(b bool) float64 {
	if b {
		return 1
	}
	return 0
}

// placePlate finds a corner of the text for it, away from the cursor. False when the window is
// too small even for the one-line plate.
func (m *Model) placePlate(p *lockPlate) bool {
	w, h := plateW, plateH
	bottom := m.height - 3 - h // just above the info line
	if bottom < textTop || m.width < w+4 {
		p.small = true
		w, h = smallPlateW(), plateSmallH
		bottom = m.height - 3 - h
		if bottom < textTop || m.width < w+2 {
			return false
		}
	}
	right, centre, left := max(m.width-w-2, 0), max((m.width-w)/2, 0), min(2, max(m.width-w, 0))
	cx, cy := -10, -10
	if c := m.cursor(); c != nil {
		cx, cy = c.X, c.Y
	}
	for _, s := range [][2]int{{right, bottom}, {centre, bottom}, {right, textTop}, {left, bottom}, {left, textTop}} {
		if cx < s[0]-1 || cx > s[0]+w || cy < s[1]-1 || cy > s[1]+h {
			p.x, p.y = s[0], s[1]
			return true
		}
	}
	p.x, p.y = right, bottom
	return true
}

func (p *lockPlate) throwSparks(key string, on bool, now time.Time) {
	i := 0
	for n, k := range plateKeys {
		if k.key == key {
			i = n
		}
	}
	x0, y0 := float64(1+platePadX+i*(capW+capGap)), float64(1+platePadY)
	n := 3
	if on {
		n = 7
	}
	for range n {
		s := spark{born: now.Add(time.Duration(rand.Float64() * float64(120*time.Millisecond))), rising: on}
		if on {
			s.x, s.y = x0+1.5+rand.Float64()*(capW-3), y0+0.2+rand.Float64()*1.2
			s.vx, s.vy, s.g = (rand.Float64()-0.5)*5, -3-rand.Float64()*3.5, 2.5
			s.life = 0.45 + rand.Float64()*0.35
		} else {
			s.x, s.y = x0+2+rand.Float64()*(capW-4), y0+capH-1
			s.vx, s.vy, s.g = (rand.Float64()-0.5)*1, 0.4+rand.Float64()*1.2, 9
			s.life = 0.4 + rand.Float64()*0.3
		}
		p.sparks = append(p.sparks, s)
	}
}

// plateStep moves it on one frame and asks for the next one.
func (m *Model) plateStep(gen int) tea.Cmd {
	p := m.plate
	if p == nil || gen != p.gen {
		return nil
	}
	now := time.Now()
	if p.over(now) {
		m.plate = nil
		return nil
	}
	dt := now.Sub(p.last).Seconds() * 60 // in sixtieths of a second, as the desktop plate counts
	p.last = now
	settled := true
	for _, k := range plateKeys {
		target, g := b2f(lockOn(m.lamps, k.key)), p.glow[k.key]
		rate := 0.14 // slower to cool
		if target > g {
			rate = 0.28 // quick to catch
		}
		g += (target - g) * (1 - math.Pow(1-rate, dt))
		if math.Abs(target-g) < 0.01 {
			g = target
		}
		settled = settled && g == target
		p.glow[k.key] = g
	}
	live := p.sparks[:0]
	for _, s := range p.sparks {
		if now.Sub(s.born).Seconds() < s.life {
			live = append(live, s)
		}
	}
	p.sparks = live
	wait := plateFrameEvery
	out := p.changed.Add(plateIn + plateHold)
	if settled && len(p.sparks) == 0 && now.Sub(p.flareT) > plateFlare && now.Sub(p.shown) > plateIn && now.Before(out) {
		wait = out.Sub(now) // nothing moves until the fade-out
	}
	return tea.Tick(wait, func(time.Time) tea.Msg { return plateFrame(gen) })
}

// ---- drawing ---------------------------------------------------------------------

type pcell struct {
	ch     string
	fg, bg rgbf
	bold   bool
}

type pgrid struct {
	w, h  int
	cells []pcell
}

func newGrid(w, h int) *pgrid { return &pgrid{w, h, make([]pcell, w*h)} }

func (g *pgrid) at(x, y int) *pcell {
	if x < 0 || y < 0 || x >= g.w || y >= g.h {
		return nil
	}
	return &g.cells[y*g.w+x]
}

func (g *pgrid) text(x, y int, s string, fg rgbf, bold bool) {
	for _, r := range s {
		if c := g.at(x, y); c != nil {
			c.ch, c.fg, c.bold = string(r), fg, bold
		}
		x++
	}
}

func (a rgbf) plus(b rgbf, t float64) rgbf { return rgbf{a.r + b.r*t, a.g + b.g*t, a.b + b.b*t} }

func clampByte(v float64) int { return max(0, min(255, int(math.Round(v)))) }

// render turns the grid into terminal text, every colour faded towards the page by alpha.
func (g *pgrid) render(alpha float64) string {
	page := toRGB(cBG)
	var sb strings.Builder
	for y := 0; y < g.h; y++ {
		last := ""
		for x := 0; x < g.w; x++ {
			c := g.cells[y*g.w+x]
			fg, bg := page.mix(c.fg, alpha), page.mix(c.bg, alpha)
			code := fmt.Sprintf("\x1b[0;38;2;%d;%d;%d;48;2;%d;%d;%d", clampByte(fg.r), clampByte(fg.g), clampByte(fg.b),
				clampByte(bg.r), clampByte(bg.g), clampByte(bg.b))
			if c.bold {
				code += ";1"
			}
			code += "m"
			if code != last {
				sb.WriteString(code)
				last = code
			}
			if c.ch == "" {
				sb.WriteByte(' ')
			} else {
				sb.WriteString(c.ch)
			}
		}
		sb.WriteString(ansiReset)
		if y < g.h-1 {
			sb.WriteByte('\n')
		}
	}
	return sb.String()
}

// resin is the plate at cell (x, y): dark, lighter at the top, warm light rising from below, grain.
func resin(x, y, w, h int) rgbf {
	t := float64(y) / float64(max(h-1, 1))
	c := rgbf{33, 19, 14}.mix(rgbf{14, 13, 13}, t)
	// cells are about twice as tall as wide, so rows count double
	d := math.Hypot(float64(x)+0.5-float64(w)/2, (float64(y)+0.5-float64(h)*1.35)*2) / (float64(w) * 0.6)
	c = c.mix(pDeep, 0.30*max(0, 1-d))
	return c.add(1.6 * grain(x+7, y+3))
}

// box draws a rounded outline; fg gives the colour along it (t from 0 at the left to 1 at the right).
func (g *pgrid) box(x0, y0, w, h int, fg func(t float64) rgbf) {
	for x := x0; x < x0+w; x++ {
		t := float64(x-x0) / float64(max(w-1, 1))
		top, bot := "─", "─"
		switch x {
		case x0:
			top, bot = "╭", "╰"
		case x0 + w - 1:
			top, bot = "╮", "╯"
		}
		g.text(x, y0, top, fg(t), false)
		g.text(x, y0+h-1, bot, fg(t), false)
	}
	for y := y0 + 1; y < y0+h-1; y++ {
		g.text(x0, y, "│", fg(0), false)
		g.text(x0+w-1, y, "│", fg(1), false)
	}
}

func (m *Model) plateBox(now time.Time) string {
	p := m.plate
	if p.small {
		return m.smallPlate(now).render(p.alpha(now))
	}
	page := toRGB(cBG)
	g := newGrid(plateW, plateH)
	for y := 0; y < plateH; y++ {
		for x := 0; x < plateW; x++ {
			bg := resin(x, y, plateW, plateH)
			if y == 0 || y == plateH-1 || x == 0 || x == plateW-1 {
				bg = page // the rounded rim sits on the page
			}
			g.cells[y*plateW+x] = pcell{ch: " ", bg: bg}
		}
	}
	// the hot rim, brightest in the middle of the top
	g.box(0, 0, plateW, plateH, func(t float64) rgbf {
		hot := 1 - math.Abs(2*t-1)
		return page.mix(pDeep.mix(pLava, hot), 0.35+0.5*hot)
	})
	// screw heads in the corners
	for _, s := range [][2]int{{1, 1}, {plateW - 2, 1}, {1, plateH - 2}, {plateW - 2, plateH - 2}} {
		g.text(s[0], s[1], "◦", pAsh.mix(rgbf{150, 90, 60}, 0.35), false)
	}
	for i, k := range plateKeys {
		f := 0.0
		if k.key == p.flared {
			f = math.Pow(max(0, 1-float64(now.Sub(p.flareT))/float64(plateFlare)), 2)
		}
		heat := p.glow[k.key]
		if !lockThere(m.lamps, k.key) {
			heat = 0 // no such light on this machine: greyed out
		}
		g.drawCap(1+platePadX+i*(capW+capGap), 1+platePadY, k.glyph, k.label, heat, f, lockThere(m.lamps, k.key))
	}
	for _, s := range p.sparks {
		g.drawSpark(s, now)
	}
	return g.render(p.alpha(now))
}

func (g *pgrid) drawCap(x0, y0 int, glyph, label string, heat, f float64, there bool) {
	// the press: light spreading off the cap onto its outline and the plate around it, fading with distance
	if f > 0 {
		for y := y0 - 2; y <= y0+capH+1; y++ {
			for x := x0 - 4; x <= x0+capW+3; x++ {
				c := g.at(x, y)
				if c == nil || y <= 0 || y >= g.h-1 || (x > x0 && x < x0+capW-1 && y > y0 && y < y0+capH-1) {
					continue
				}
				dx := max(x0-x, x-(x0+capW-1), 0)
				dy := max(y0-y, y-(y0+capH-1), 0) * 2 // rows count double
				d := math.Hypot(float64(dx), float64(dy))
				c.bg = c.bg.mix(pGold, 0.16*f*max(0, 1-d/4))
			}
		}
	}
	// the keycap: dark coal, warm and lit from below when on
	for y := y0 + 1; y < y0+capH-1; y++ {
		t := (float64(y-y0-1) + 0.5) / float64(capH-2)
		for x := x0 + 1; x < x0+capW-1; x++ {
			c := g.at(x, y)
			top := rgbf{38, 26, 20}.mix(rgbf{61, 26, 13}, heat)
			bot := rgbf{22, 17, 15}.mix(rgbf{40, 17, 9}, heat)
			bg := top.mix(bot, t)
			d := math.Hypot(float64(x)+0.5-float64(x0)-float64(capW)/2, (float64(y)+0.5-float64(y0)-float64(capH)*1.05)*2) / float64(capH*2)
			bg = bg.mix(pLava, 0.38*heat*max(0, 1-d))
			c.bg = bg.add(1.2 * grain(x+11, y+5))
		}
	}
	edge := func(t float64) rgbf {
		c := pAsh.mix(pLava, heat)
		c = g.at(x0, y0).bg.mix(c, min(1, 0.55+0.35*heat+0.1*f))
		return c.mix(pHot, 0.8*f)
	}
	g.box(x0, y0, capW, capH, edge)
	for x := x0 + 2; x < x0+capW-2; x++ { // the top bevel catches a little light
		if c := g.at(x, y0); c != nil {
			c.fg = c.fg.mix(rgbf{255, 191, 140}, 0.10+0.15*heat)
		}
	}
	// the lamp, top right, like a real keyboard light
	lx, ly := x0+capW-3, y0+1
	for dx := -1; dx <= 1; dx++ {
		if c := g.at(lx+dx, ly); c != nil {
			k := 0.24
			if dx != 0 {
				k = 0.06 * (1 + 0.8*f)
			}
			c.bg = c.bg.mix(pGold, k*heat)
		}
	}
	g.text(lx, ly, "●", pCoal.mix(pAsh, 0.55).mix(pHot, heat), false)
	// the glyph: a glowing coal when on, carved ash when off
	gy := y0 + 2
	gx := x0 + 1 + (capW-2-len([]rune(glyph)))/2
	g.text(gx, gy, glyph, pAsh.mix(pGold.mix(pHot, f*0.8), heat), true)
	// the label underneath
	lbl := pAsh.mix(pGold, heat)
	if !there {
		lbl = pAsh.mix(pCoal, 0.4)
	}
	ly2 := y0 + 3
	lxs := x0 + 1 + (capW-2-len(label))/2
	g.text(lxs, ly2, label, g.at(lxs, ly2).bg.mix(lbl, 0.75+0.25*heat), false)
}

// drawSpark puts a hot fleck (or a cooling drop) where it has got to, on an empty cell or a line.
func (g *pgrid) drawSpark(s spark, now time.Time) {
	t := now.Sub(s.born).Seconds()
	if t < 0 || t > s.life {
		return
	}
	q := t / s.life
	x := s.x + s.vx*t
	y := s.y + s.vy*t + 0.5*s.g*t*t
	c := g.at(int(math.Floor(x)), int(math.Floor(y)))
	if c == nil || (c.ch != " " && c.ch != "─") {
		return
	}
	col := pHot.mix(pDeep, q)
	if !s.rising {
		col = pLava.mix(pAsh, q)
	}
	a := math.Pow(1-q, 1.4)
	frac := y - math.Floor(y) // where in the cell: a dot high, in the middle or low
	ch := "·"
	switch {
	case frac < 0.34:
		ch = "˙"
	case frac > 0.67:
		ch = "."
	}
	if q < 0.25 && s.rising {
		ch = "•"
		if frac < 0.34 || frac > 0.67 {
			ch = "·"
		}
	}
	c.ch, c.fg, c.bold = ch, c.bg.mix(col, a), true
	c.bg = c.bg.mix(col, 0.07*a)
}

// ---- the one-line plate, for small windows ------------------------------------------

func smallPlateW() int {
	w := 2 + 2 // rim and a space each side
	for i, k := range plateKeys {
		w += 2 + len(k.label) + 2 // " ● LABEL " with a space either side
		if i > 0 {
			w++
		}
	}
	return w
}

func (m *Model) smallPlate(now time.Time) *pgrid {
	p := m.plate
	page := toRGB(cBG)
	w := smallPlateW()
	g := newGrid(w, plateSmallH)
	for y := 0; y < g.h; y++ {
		for x := 0; x < w; x++ {
			bg := resin(x, y, w, g.h)
			if y != 1 || x == 0 || x == w-1 {
				bg = page
			}
			g.cells[y*w+x] = pcell{ch: " ", bg: bg}
		}
	}
	g.box(0, 0, w, g.h, func(t float64) rgbf {
		hot := 1 - math.Abs(2*t-1)
		return page.mix(pDeep.mix(pLava, hot), 0.35+0.5*hot)
	})
	x := 2
	for _, k := range plateKeys {
		f := 0.0
		if k.key == p.flared {
			f = math.Pow(max(0, 1-float64(now.Sub(p.flareT))/float64(plateFlare)), 2)
		}
		heat := p.glow[k.key]
		if !lockThere(m.lamps, k.key) {
			heat = 0
		}
		seg := 2 + len(k.label) + 2
		for i := 0; i < seg; i++ {
			c := g.at(x+i, 1)
			c.bg = rgbf{38, 26, 20}.mix(rgbf{61, 26, 13}, heat).mix(pLava, 0.18*heat).mix(pGold, 0.22*f)
		}
		g.text(x+1, 1, "●", pCoal.mix(pAsh, 0.55).mix(pHot, heat), false)
		g.text(x+3, 1, k.label, pAsh.mix(pGold.mix(pHot, f*0.8), heat), heat > 0.5)
		x += seg + 1
	}
	return g
}

// overlayPlate puts the plate over the finished screen.
func (m *Model) overlayPlate(screen string) string {
	now := time.Now()
	p := m.plate
	if p == nil || p.over(now) {
		return screen
	}
	y := p.y
	if now.Sub(p.shown) < plateIn { // it rises a row as it fades in
		y += int(math.Round(1 - p.alpha(now)))
	}
	return lipgloss.NewCompositor(
		lipgloss.NewLayer(screen),
		lipgloss.NewLayer(m.plateBox(now)).X(p.x).Y(y).Z(5),
	).Render()
}
