//go:build linux && cgo

package main

import "fmt"

// The uniforms' names for the town's arrays, made once.
var (
	walkNames  = arrayNames("oWalk", 3*worldWalkers)
	glyphNames = arrayNames("oGlyph", 64)
	xPosNames  = arrayNames("xPos", maxXLights)
	xColNames  = arrayNames("xCol", maxXLights)
	xInvNames  = arrayNames("xInv", maxXLights)
)

func arrayNames(name string, n int) []string {
	s := make([]string, n)
	for i := range s {
		s[i] = fmt.Sprintf("%s[%d]", name, i)
	}
	return s
}

// worldSlots gives the town's letters their layers in the card's letters
// (before gpuFrame sends the ones it does not have yet).
func (r *room3D) worldSlots() {
	w := &r.world
	for i, g := range outGlyphsOnce() {
		w.slots[i] = -1
		if g != nil {
			w.slots[i] = r.glyphSlot(g)
		}
	}
}

// worldUniforms gives step 2 the sky, the people and the extra lights of this frame.
func (r *room3D) worldUniforms(u *glUniforms) {
	w := &r.world
	sk := &w.sky
	u.f("oH", w.h)
	u.f("oG", w.g)
	u.f("oOu", w.ou)
	u.f("oCloud", w.cloud)
	u.f("oDay", sk.day)
	u.f("oStars", sk.stars)
	u.f("oLit", sk.lit)
	u.f("lGlowE", w.glowE)
	u.v3("oMid", w.mid)
	u.v3("oSun", sk.sun)
	u.v3("oMoon", sk.moon)
	u.ui("oTop", sk.top)
	u.ui("oHorizon", sk.horizon)
	u.ui("oCloudCol", sk.cloud)
	u.ui("oHaze", sk.haze)
	u.ui("oSunCol", sk.sunCol)
	u.i("oTwink", int(w.twink))
	for i, wk := range w.walk {
		u.v3(walkNames[i], r3vec{wk.x, float64(wk.code), float64(wk.coat)})
	}
	for i := range outChars {
		u.i(glyphNames[i], int(w.slots[i]))
	}
	u.i("nX", len(w.lamps))
	for i, l := range w.lamps {
		u.v3(xPosNames[i], l.at)
		u.v3(xColNames[i], r3vec{l.col[0], l.col[1], l.col[2]})
		u.f(xInvNames[i], l.inv)
	}
}
