package main

// ---- 3D: layers, the lens, and placing where the cross is ------------------------------
//
// Every structure stands on a layer: 0 is on the wall, and each layer out
// stands layerStep further into the room. Structures on the same layer
// share it (a new one is moved off the others on its layer only); Shift
// + = brings the chosen one a layer out, Shift + − takes it a layer back.
// On the flat board the layer does not show.
//
// 3D always shows the board at 100%: the flat board's zoom would shrink
// every block and its words. Going into 3D, the camera stands as far back
// as the flat board's zoom showed the wall, so there is no jump, and the
// flat board's zoom comes back on leaving. In 3D, = and − zoom the
// camera's lens instead, and 0 sets it back.
//
// Alt + N places the new structure centred on the cross: on the wall, in
// front of the structure the cross is on (a layer out from it), or out in
// the room where the cross meets the floor, a side wall or the ceiling,
// on the nearest layer (and not far off: at most placeReach in front of
// the camera). It is never moved aside: where its spot is taken,
// it goes on the first free layer in front.

import (
	"fmt"
	"math"
)

const (
	layerStep = 13.0 // units between layers: deeper than the deepest block (12, a cabinet)
	maxLayer  = 400
	lensStep  = 1.25 // = and − in 3D zoom the lens this much a press
	lensMin   = 0.4
	lensMax   = 8.0
	// placeReach is how far in front of the camera (the screen's bigger
	// side) a new structure goes at most, when the cross is on the floor,
	// a side wall or the ceiling far off.
	placeReach = 0.35
)

// design3DAt100 sets the flat board to 100% for 3D, keeping the board
// point in the middle of the screen where it is, and remembers its own
// zoom for design3DFlatZoom. It gives the zoom it was at.
func (m *Model) design3DAt100() float64 {
	d := m.ds()
	r := &d.room
	z := d.z()
	if r.free || d.zoom == designZoom100 {
		return z
	}
	w, h := float64(m.width), float64(m.boardH())
	r.flatZoom, r.flatCam, r.flatSaved = d.zoom, [2]float64{d.camX, d.camY}, true
	cx, cy := d.camX+w/2/z, d.camY+h/2/z
	d.zoom = designZoom100
	d.camX, d.camY = cx-w/2, cy-h/2
	return z
}

// design3DFlatZoom gives the flat board its own zoom and place back.
func (m *Model) design3DFlatZoom() {
	d := m.ds()
	r := &d.room
	if !r.flatSaved {
		return
	}
	d.zoom, d.camX, d.camY = r.flatZoom, r.flatCam[0], r.flatCam[1]
	r.flatSaved = false
}

// pullBack stands the camera (seeing as v) further back from the wall, by
// as much as zoom z (under 1) showed more of it.
func (r *room3D) pullBack(v r3view, z float64) {
	if z >= 1 || z <= 0 || v.fwd.z > -1e-6 {
		return
	}
	t := -r.pos.z / v.fwd.z // to the wall, straight ahead
	at := r.pos.add(v.fwd.mul(t))
	r.pos = at.sub(v.fwd.mul(t / z))
}

// keepCamera keeps the first-person camera on the same spot of the board
// when the board area changes size (the window, or the terminal's font
// zoomed) or the board under the room moves: the room is laid out round
// the board area, so the camera is moved with it, and its lens widened or
// narrowed so the picture keeps its width.
func (r *room3D) keepCamera(d *designState, w, h int) {
	cam := [2]float64{d.camX, d.camY}
	if !r.free || r.rm.w == 0 || r.rm.w == w && r.rm.h == h && r.anchor == cam {
		r.anchor = cam
		return
	}
	ohw, ohh := r.rm.hw, r.rm.hh
	nhw, nhh := float64(w)/2, float64(h)
	move := func(p r3vec) r3vec {
		bx, by := p.x+ohw+r.anchor[0], (ohh-p.y)/2+r.anchor[1] // the board point, in board cells
		return r3vec{bx - nhw - cam[0], nhh - 2*(by-cam[1]), p.z}
	}
	r.pos, r.startPos = move(r.pos), move(r.startPos)
	if r.rm.w != w {
		k := float64(w) / float64(r.rm.w)
		r.ff, r.ff0 = r.ff*k, r.ff0*k
	}
	r.anchor = cam
	r.rm.w, r.rm.h, r.rm.hw, r.rm.hh = w, h, nhw, nhh
}

// layerOut moves block b's faces (faces f0 on, corners p0 on) out to its
// layer, and closes its back, which is seen once it stands off the wall.
func (r *room3D) layerOut(b *r3block, f0, p0 int, hw, hh float64) {
	for i := f0; i < len(r.faces); i++ {
		r.faces[i].tdy = 2 * b.shift // (even flat against the wall, a block on a layer shows its kept drawing)
	}
	if b.off <= 0 {
		return
	}
	for i := p0; i < len(r.pts); i++ {
		r.pts[i].z += b.off
	}
	for i := f0; i < len(r.faces); i++ {
		f := &r.faces[i]
		f.k = f.n.dot(r.pts[f.p0])
		f.o.z += b.off
		f.back = b.off
		f.tdy = 2 * b.shift
	}
	X0, X1 := float64(b.x0)-hw, float64(b.x1)-hw
	Yt, Yb := hh-float64(2*b.y0), hh-float64(2*b.y1)
	col := hex32(cText)
	if b.o.Color != "" {
		col = r.hex(b.o.Color)
	}
	r.quadFace(r3face{n: r3vec{0, 0, -1}, mat: matSolid, own: b.o.ID, col: mix32(col, 0, 0.35), back: b.off,
		cx0: b.x0, cx1: b.x1, py0: 2 * b.y0, py1: 2 * b.y1},
		r3vec{X0, Yb, b.off}, r3vec{0, Yt - Yb, 0}, r3vec{X1 - X0, 0, 0})
}

// design3DLayerKey takes the 3D view's lens and layer keys: = and − zoom
// the lens (0: back to normal), Shift + = and Shift + − bring the chosen
// structure a layer out or take it a layer back. It says whether it took
// the key.
func (m *Model) design3DLayerKey(name string) bool {
	d := m.ds()
	r := &d.room
	switch name {
	case "=", "-", "minus", "kp+", "kp-", "ctrl+=", "ctrl+-":
		in := name == "=" || name == "kp+" || name == "ctrl+="
		if !r.free {
			m.design3DFree()
		}
		lens := r.lens * map[bool]float64{true: lensStep, false: 1 / lensStep}[in]
		r.lens = clampF(lens, lensMin, lensMax)
		m.say(fmt.Sprintf("Lens: %.0f%%.  0: back to normal.", 100*r.lens))
		return true
	case "0":
		r.lens = 1
		m.say("Lens: normal.")
		return true
	case "+", "shift+=", "plus", "shift+plus":
		m.design3DLayerBy(1)
		return true
	case "_", "shift+-", "shift+minus", "shift+_":
		m.design3DLayerBy(-1)
		return true
	}
	return false
}

// design3DLayerBy moves the chosen structures by n layers (out from the
// wall: n > 0).
func (m *Model) design3DLayerBy(n int) {
	d := m.ds()
	var them []*dOct
	if len(d.multi) > 0 {
		for _, o := range d.octs {
			if d.multi[o.ID] {
				them = append(them, o)
			}
		}
	} else if o := d.byID(d.sel); o != nil {
		them = append(them, o)
	}
	if len(them) == 0 {
		m.say("Put the cross on a structure first: Shift + = and Shift + − move it out and back.")
		return
	}
	moved := false
	for _, o := range them {
		if l := max(0, min(maxLayer, o.Layer+n)); l != o.Layer {
			o.Layer, moved = l, true
		}
	}
	if !moved {
		m.say(map[bool]string{true: "It is on the wall: it cannot go further back.", false: "It is as far out as it goes."}[n < 0])
		return
	}
	d.dirty = true
	if len(them) == 1 {
		m.say(fmt.Sprintf("%s: %s.", itemTitle(them[0]), layerName(them[0].Layer)))
	} else {
		m.say(fmt.Sprintf("%d structures moved %s a layer.", len(them), map[bool]string{true: "out", false: "back"}[n > 0]))
	}
}

// layerName says where layer l is.
func layerName(l int) string {
	if l == 0 {
		return "on the wall"
	}
	return fmt.Sprintf("layer %d out from the wall", l)
}

// design3DPlace places a new structure of kind centred on the cross (see
// the top of this file). It says whether it did: false when the room is
// not in 3D or not drawn yet.
func (m *Model) design3DPlace(kind string) bool {
	d := m.ds()
	r := &d.room
	if !r.on() {
		return false
	}
	v, ok := r.nowView()
	if !ok {
		return false
	}
	hit := r.pickAim()
	if !hit.ok {
		return false
	}
	dir := v.ray(v.px, v.py)
	k := r.outK()
	layer, p := 0, hit.p
	switch {
	case hit.own >= 0: // on a structure: a layer out from it
		if o := d.byID(hit.own); o != nil {
			layer = min(maxLayer, o.Layer+1)
		}
	case r.faces[hit.face].mat == matBoard: // on the wall
	default: // out in the room: the nearest layer to where the cross meets it, no further than placeReach in front
		if t := p.sub(v.cam).len(); t > placeReach*r.rm.big {
			p = v.cam.add(dir.norm().mul(placeReach * r.rm.big))
		}
		if k > 0.2 {
			layer = max(0, min(maxLayer, int(math.Round(p.z/(layerStep*k)))))
		}
	}
	// Where the cross's ray meets the new layer's plane (the front of what
	// stands there is near enough).
	if zl := float64(layer) * layerStep * k; dir.z < -1e-9 && v.cam.z > zl {
		p = v.cam.add(dir.mul((zl - v.cam.z) / dir.z))
	}
	rm := &r.rm
	sx, sy := int(math.Floor(p.x+rm.hw)), int(math.Floor((rm.hh-p.y)/2))
	wx, wy := d.world(sx, sy)
	o := d.newKind(kind, int(math.Floor(wx)), int(math.Floor(wy)))
	o.Layer = layer
	for o.Layer < maxLayer && d.coversOnLayer(o) {
		o.Layer++ // the spot is taken on that layer: in front of it, still at the cross
	}
	d.sel = o.ID
	d.dirty = true
	return true
}

// coversOnLayer is whether o covers another structure on its layer (with
// a cell between, as clearSpot keeps).
func (d *designState) coversOnLayer(o *dOct) bool {
	w, h := d.size(o)
	for _, p := range d.octs {
		if p == nil || p.ID == o.ID || p.Layer != o.Layer {
			continue
		}
		pw, ph := d.size(p)
		if o.X < p.X+pw+1 && p.X-1 < o.X+w && o.Y < p.Y+ph+1 && p.Y-1 < o.Y+h {
			return true
		}
	}
	return false
}

// layerCopies makes room in the room's picture of the wall for the
// drawings of the blocks out on layers: the wall behind such a block is
// drawn without it (design3DRows leaves it out of the flat board read
// for the wall), so its words do not show on the wall behind it as well,
// and its own drawing is kept in rows added below the board, which the
// wall never shows (it ends at the floor); its front shows them there.
func (r *room3D) layerCopies(d *designState, rm *r3room) {
	const gap = 8 // rows between, so the smaller pictures (mipmap) do not blend one into another
	base := rm.ty0 + rm.th + gap
	for i := range r.blocks {
		b := &r.blocks[i]
		b.shift = 0
		if b.o.Layer <= 0 {
			continue
		}
		_, y, _, oh := d.rect(b.o)
		b.shift = base - y
		base += oh + gap
	}
	if base > rm.ty0+rm.th+gap {
		rm.th = base - rm.ty0
	}
}

// layered is whether any structure stands out on a layer.
func (d *designState) layered() bool {
	for _, o := range d.octs {
		if o != nil && o.Layer > 0 {
			return true
		}
	}
	return false
}

// onWall is the structures on the wall (layer 0): the flat board the wall
// shows (see layerCopies).
func (d *designState) onWall() []*dOct {
	var them []*dOct
	for _, o := range d.octs {
		if o != nil && o.Layer == 0 {
			them = append(them, o)
		}
	}
	return them
}
