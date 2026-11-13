//go:build linux && cgo

package main

// ---- the 3D room on the graphics card --------------------------------------------------------
//
// The kitty picture of the room (room3D.paint) is drawn here on the
// graphics card, not on the processor, when there is one: the same faces,
// the same colours and the same light as draw and shadePixel work out,
// written once more as shaders. The processor still makes the scene (the
// faces, their outlines on the screen, the back wall's cells as pictures,
// the shadows): that is small. The card does the millions of pixels:
//
//  1. Each face's outline is filled, and each pixel keeps the face whose
//     plane its ray meets nearest (as depthRows does).
//  2. Each pixel is coloured and lit for the face it shows (shadePixel).
//  3. Its red, green and blue bytes are packed four to a pixel, in the
//     order kitty takes them, and read back (reading RGB straight, the
//     driver converts every pixel on the processor, and slowly).
//
// The light is worked out at every pixel (shadePixel shares it over 2×2
// pixels to save time), which is a hair smoother. Anything that goes wrong
// (no card, a shader the driver will not take) and the processor draws it,
// as before. SY_GPU=0 (or cpu) always draws on the processor; SY_GPU=
// discrete tries the cards EGL lists first (this laptop's NVIDIA card)
// rather than the one that shows the screen.

import (
	"os"
	"strings"
	"sync"
	"unsafe"
)

// room3DGPU: the kitty picture may be drawn on the graphics card. Set in
// main.go; off in tests, which look at the processor's pixels.
var room3DGPU = false

// faceTexels is how many texels of the faces' texture a face takes.
const faceTexels = 6

// gpuRoom is the graphics card's side of the room: its programs, its
// pictures and the pixels it draws into.
var gpuRoom gpuState

type gpuState struct {
	once     sync.Once
	ok       bool
	why      string // why it is not used, when it is not
	frameWhy string // why the last frame asked for was not drawn on it
	p1       glUniforms
	p2       glUniforms
	p3       glUniforms
	vao, vbo uint32
	fbo1     uint32 // the faces' numbers and depth (step 1)
	fbo2     uint32 // the picture (step 2)
	fbo3     uint32 // the picture packed (step 3)
	pbo      uint32 // where it is read back through
	w, h     int    // the size the pictures below have now
	// The pixels drawn into, and the textures sent each frame.
	idTex, zTex, picTex, packTex                           uint32
	faceTex, mipTex, flatTex, cellTex, shadowTex, noiseTex uint32
	glyphTex, glyphInfoTex                                 uint32
	glyphAt                                                map[*r3glyph]int32
	glyphs                                                 []*r3glyph
	glyphCap                                               int // layers the letters' texture has room for
	glyphSent                                              int // letters sent to it
	// Buffers used again each frame.
	verts    []byte
	faces    []float32
	cells    []uint32
	mips     []uint32
	info     []float32
	maxTex   int
	maxLayer int
}

// gpuReady is whether the graphics card can draw the room (it is set up
// the first time this is asked).
func gpuReady() bool {
	if !room3DGPU {
		return false
	}
	g := &gpuRoom
	g.once.Do(func() {
		switch v := strings.ToLower(os.Getenv("SY_GPU")); v {
		case "0", "off", "cpu", "no":
			g.why = "SY_GPU=" + v
			return
		default:
			if !gpuStart(v == "discrete" || v == "nvidia" || v == "dgpu") {
				g.why = "no graphics card to draw on"
				return
			}
		}
		gpuRun(func() { g.ok, g.why = gpuSetUp() })
	})
	return g.ok
}

// gpuSetUp makes the programs and the pictures that stay (on the card's thread).
func gpuSetUp() (bool, string) {
	g := &gpuRoom
	p1, err := glProgram(gpuVS1, gpuFS1)
	if err != nil {
		return false, err.Error()
	}
	p2, err := glProgram(gpuVS2, gpuHead+gpuFinish+gpuFS2)
	if err != nil {
		return false, err.Error()
	}
	p3, err := glProgram(gpuVS2, gpuFS3)
	if err != nil {
		return false, err.Error()
	}
	g.p1, g.p2, g.p3 = glUniforms{prog: p1}, glUniforms{prog: p2}, glUniforms{prog: p3}
	g.fbo3, g.packTex, g.pbo = glGenFramebuffer(), glGenTexture(), glGenBuffer()
	plainTexture(glTexture2D, g.packTex)
	g.maxTex, g.maxLayer = glGetInteger(glMaxTextureSize), glGetInteger(glMaxArrayLayers)
	g.vao, g.vbo = glGenVertexArray(), glGenBuffer()
	g.fbo1, g.fbo2 = glGenFramebuffer(), glGenFramebuffer()
	g.idTex, g.zTex, g.picTex = glGenTexture(), glGenTexture(), glGenTexture()
	g.faceTex, g.mipTex, g.flatTex, g.cellTex = glGenTexture(), glGenTexture(), glGenTexture(), glGenTexture()
	g.shadowTex, g.noiseTex, g.glyphTex, g.glyphInfoTex = glGenTexture(), glGenTexture(), glGenTexture(), glGenTexture()
	g.glyphAt = map[*r3glyph]int32{}
	for _, t := range []uint32{g.idTex, g.zTex, g.picTex, g.faceTex, g.mipTex, g.flatTex, g.cellTex, g.shadowTex, g.noiseTex, g.glyphInfoTex} {
		plainTexture(glTexture2D, t)
	}
	// The plaster's noise, once (see noiseFine).
	glBindTexture(glTexture2D, g.noiseTex)
	n := 64 * noiseK
	glTexImage2D(glTexture2D, 0, glR32F, n, n, glRed, glFloat, unsafe.Pointer(&noiseFine[0]))
	glPixelStorei(glUnpackAlignment, 1)
	if e := glGetError(); e != 0 {
		return false, "OpenGL error while setting up"
	}
	return true, ""
}

// plainTexture makes texture t read pixel for pixel, never blended.
func plainTexture(target, t uint32) {
	glBindTexture(target, t)
	glTexParameteri(target, glTextureMinFilter, glNearest)
	glTexParameteri(target, glTextureMagFilter, glNearest)
	glTexParameteri(target, glTextureWrapS, glClampToEdge)
	glTexParameteri(target, glTextureWrapT, glClampToEdge)
}

// gpuDraw draws the picture's pixels (r.pw × r.ph) on the graphics card
// into r.rgbOut, lit, for the faces draw has outlined. False when it could
// not (the processor draws them then).
func (r *room3D) gpuDraw(c *r3shade, rm *r3room) bool {
	if !gpuReady() || len(r.rgbOut) != 3*r.pw*r.ph {
		return false
	}
	g := &gpuRoom
	if r.pw > g.maxTex || r.ph > g.maxTex || 2*rm.tw > g.maxTex || len(r.mips) == 0 || len(r.flat0) != r.mipW[0]*r.mipH[0] {
		g.frameWhy = "too big for the card, or no back wall pictures"
		return false
	}
	r.gpuData(rm) // on this goroutine: the card's thread only sends it
	ok := false
	gpuRun(func() { ok = r.gpuFrame(c, rm) })
	if !ok {
		g.frameWhy = "the card failed to draw"
	}
	return ok
}

// gpuData packs what the card needs that changes each frame.
func (r *room3D) gpuData(rm *r3room) {
	g := &gpuRoom
	// The faces: their plane as seen (from draw), what they are made of,
	// their edges and the board cells they show.
	nf := len(r.faces)
	rows := (nf*faceTexels + 1023) / 1024
	g.faces = growF(g.faces, 4*1024*max(rows, 1))
	for fi := range r.faces {
		fc, fd := &r.faces[fi], &r.fdraw[fi]
		n, num := fc.n, 0.0
		if fc.seen {
			n, num = fd.n, fd.num
		}
		flags := 0
		if fd.inside && fc.seen {
			flags |= 1
		}
		if fc.edge {
			flags |= 2
		}
		v := g.faces[4*faceTexels*fi:]
		put4(v[0:], n.x, n.y, n.z, num)
		put4(v[4:], float64(fc.mat), float64(fc.col), fc.gloss, float64(flags))
		put4(v[8:], fc.o.x, fc.o.y, fc.o.z, fc.lu)
		put4(v[12:], fc.eu.x, fc.eu.y, fc.eu.z, fc.lv)
		put4(v[16:], fc.ev.x, fc.ev.y, fc.ev.z, float64(fc.tdy))
		put4(v[20:], float64(fc.cx0), float64(fc.cx1), float64(fc.py0), float64(fc.py1))
	}
	// The faces' outlines on the screen, as triangles: x, y and the face.
	g.verts = g.verts[:0]
	for fi := range r.faces {
		if !r.faces[fi].seen {
			continue
		}
		fd := &r.fdraw[fi]
		poly := r.polys[fd.p0 : fd.p0+fd.pn]
		for i := 1; i+1 < len(poly); i++ {
			for _, p := range [3][2]float64{poly[0], poly[i], poly[i+1]} {
				g.verts = putVert(g.verts, float32(p[0]), float32(p[1]), int32(fi))
			}
		}
	}
	// The back wall's cells: colours, painted letter and box line.
	g.cells = growU(g.cells, 4*len(r.tex))
	for i := range r.tex {
		t := &r.tex[i]
		gi := uint32(0)
		if t.gl != nil {
			gi = uint32(r.glyphSlot(t.gl)+1) | uint32(t.gx&1)<<16
		}
		c := g.cells[4*i:]
		c[0], c[1], c[2], c[3] = t.fg, t.bg, gi, uint32(uint8(t.arms))
	}
	// The back wall's pictures, one under another.
	total := 0
	for k := range r.mips {
		total += r.mipW[0] * r.mipH[k]
	}
	g.mips = growU(g.mips, total)
	at := 0
	for k, m := range r.mips {
		w, h := r.mipW[k], r.mipH[k]
		for y := range h {
			copy(g.mips[at+y*r.mipW[0]:], m[y*w:y*w+w])
		}
		at += r.mipW[0] * h
	}
}

// glyphSlot is the layer of the card's letters letter g is in (it is
// given one, and sent, in gpuFrame).
func (r *room3D) glyphSlot(gl *r3glyph) int32 {
	g := &gpuRoom
	if i, ok := g.glyphAt[gl]; ok {
		return i
	}
	i := int32(len(g.glyphs))
	g.glyphAt[gl] = i
	g.glyphs = append(g.glyphs, gl)
	return i
}

// gpuFrame draws the frame on the card (on its thread).
func (r *room3D) gpuFrame(c *r3shade, rm *r3room) bool {
	g := &gpuRoom
	pw, ph := r.pw, r.ph
	if g.w != pw || g.h != ph {
		glBindTexture(glTexture2D, g.idTex)
		glTexImage2D(glTexture2D, 0, glR32I, pw, ph, glRedInteger, glInt, nil)
		glBindTexture(glTexture2D, g.zTex)
		glTexImage2D(glTexture2D, 0, glDepth32F, pw, ph, glDepthComponent, glFloat, nil)
		glBindTexture(glTexture2D, g.picTex)
		glTexImage2D(glTexture2D, 0, glRGBA8, pw, ph, glRGBA, glUnsignedByte, nil)
		glBindFramebuffer(g.fbo1)
		glFramebufferTexture(glColorAttachment0, g.idTex)
		glFramebufferTexture(glDepthAttachment, g.zTex)
		if !glFramebufferComplete() {
			return false
		}
		glBindFramebuffer(g.fbo2)
		glFramebufferTexture(glColorAttachment0, g.picTex)
		if !glFramebufferComplete() {
			return false
		}
		glBindTexture(glTexture2D, g.packTex)
		glTexImage2D(glTexture2D, 0, glRGBA8, pw, packRows(pw, ph), glRGBA, glUnsignedByte, nil)
		glBindFramebuffer(g.fbo3)
		glFramebufferTexture(glColorAttachment0, g.packTex)
		if !glFramebufferComplete() {
			return false
		}
		g.w, g.h = pw, ph
	}
	if !g.sendGlyphs() {
		return false
	}
	// This frame's textures.
	glPixelStorei(glUnpackAlignment, 4)
	glBindTexture(glTexture2D, g.faceTex)
	glTexImage2D(glTexture2D, 0, glRGBA32F, 1024, len(g.faces)/4096, glRGBA, glFloat, unsafe.Pointer(&g.faces[0]))
	glBindTexture(glTexture2D, g.cellTex)
	glTexImage2D(glTexture2D, 0, glRGBA32UI, rm.tw, rm.th, glRGBAInteger, glUnsignedInt, unsafe.Pointer(&g.cells[0]))
	glBindTexture(glTexture2D, g.mipTex)
	glTexImage2D(glTexture2D, 0, glR32UI, r.mipW[0], len(g.mips)/r.mipW[0], glRedInteger, glUnsignedInt, unsafe.Pointer(&g.mips[0]))
	glBindTexture(glTexture2D, g.flatTex)
	glTexImage2D(glTexture2D, 0, glR32UI, r.mipW[0], r.mipH[0], glRedInteger, glUnsignedInt, unsafe.Pointer(&r.flat0[0]))
	glBindTexture(glTexture2D, g.shadowTex)
	glTexImage2D(glTexture2D, 0, glR32F, rm.tw, 2*rm.th, glRed, glFloat, unsafe.Pointer(&r.shadow[0]))
	glPixelStorei(glUnpackAlignment, 1)

	v := &r.view
	view := func(u *glUniforms) {
		u.v3("fwd", v.fwd)
		u.v3("right", v.right)
		u.v3("up", v.up)
		u.f("f", v.f)
		u.f("px", v.px)
		u.f("py", v.py)
		u.f("kx", c.kx)
		u.f("ky", c.ky)
		u.i("faces", 0)
	}
	glBindVertexArray(g.vao)
	glActiveTexture(0)
	glBindTexture(glTexture2D, g.faceTex)

	// 1: the nearest face at each pixel.
	glBindFramebuffer(g.fbo1)
	glViewport(pw, ph)
	glClearInt(-1)
	glClearDepth(1)
	if len(g.verts) > 0 {
		glEnable(glDepthTest)
		glDepthFunc(glLess)
		glUseProgram(g.p1.prog)
		view(&g.p1)
		g.p1.f("pw", float64(pw))
		g.p1.f("ph", float64(ph))
		glVertices(g.vbo, g.verts)
		glDrawTriangles(len(g.verts) / 12)
		glDisable(glDepthTest)
	}

	// 2: each pixel coloured and lit.
	glBindFramebuffer(g.fbo2)
	glUseProgram(g.p2.prog)
	u := &g.p2
	view(u)
	for i, t := range []uint32{g.faceTex, g.idTex, g.mipTex, g.flatTex, g.cellTex, g.glyphTex, g.glyphInfoTex, g.shadowTex, g.noiseTex} {
		glActiveTexture(i)
		target := uint32(glTexture2D)
		if t == g.glyphTex {
			target = glTexture2DArray
		}
		glBindTexture(target, t)
	}
	for i, name := range []string{"faces", "ids", "mips", "flat0", "cells", "glyphs", "glyphInfo", "shadowTex", "noiseTex"} {
		u.i(name, i)
	}
	u.v3("cam", v.cam)
	u.v3("lamp", c.lamp)
	u.f("kpx", c.kpx)
	u.f("invR2", c.invR2)
	u.f("lAmbient", c.light.ambient)
	u.f("lFill", c.light.fill)
	u.f("lLamp", c.light.lamp)
	u.f("lGlow", c.light.glow)
	u.f("lSpec", c.light.spec)
	u.f("hw", c.hw)
	u.f("hh", c.hh)
	u.f("minU", c.minU)
	u.f("minV", c.minV)
	u.f("gk", c.gk)
	u.i("tx0", c.tx0)
	u.i("ty0", c.ty0)
	u.i("tw", rm.tw)
	u.i("th", rm.th)
	u.ui("textCol", c.textCol)
	u.ui("outer", c.outer)
	u.ui("bgPlain", c.bgPlain)
	u.ui("wallCol", c.wallCol)
	u.f("rmFy", rm.fy)
	u.f("rmCy", rm.cy)
	u.f("rmBig", rm.big)
	u.i("wallFin", rm.walls) // the finishes (design_3d_finish.go)
	u.i("floorFin", rm.floor)
	var mw, mh, mo [6]int32
	off := 0
	for k := range min(len(r.mips), 6) {
		mw[k], mh[k], mo[k] = int32(r.mipW[k]), int32(r.mipH[k]), int32(off)
		off += r.mipH[k]
	}
	u.i("nMips", min(len(r.mips), 6))
	u.iv("mipW", mw[:])
	u.iv("mipH", mh[:])
	u.iv("mipOff", mo[:])
	glDrawTriangles(3)

	// 3: packed for kitty, and read back.
	glBindFramebuffer(g.fbo3)
	glViewport(pw, packRows(pw, ph))
	glUseProgram(g.p3.prog)
	glActiveTexture(0)
	glBindTexture(glTexture2D, g.picTex)
	g.p3.i("pic", 0)
	g.p3.i("pw", pw)
	g.p3.i("ph", ph)
	glDrawTriangles(3)
	if !glReadPBO(g.pbo, pw, packRows(pw, ph), r.rgbOut) {
		return false
	}
	return glGetError() == 0
}

// sendGlyphs gives the card the letters it does not have yet (on its
// thread): each a layer of a texture of 96×96 pixels at its finest, with
// its smaller copies (r3glyph.lv) as the texture's smaller levels.
func (g *gpuState) sendGlyphs() bool {
	if g.glyphSent == len(g.glyphs) && g.glyphCap > 0 {
		return true
	}
	const levels = 7 // 96, 48, 24, 12, 6, 3, 1
	if len(g.glyphs) > g.glyphCap || g.glyphCap == 0 {
		n := max(64, g.glyphCap)
		for n < len(g.glyphs) {
			n *= 2
		}
		if n > g.maxLayer {
			return false
		}
		glBindTexture(glTexture2DArray, g.glyphTex)
		for k := range levels {
			s := 2 * glyphW >> k
			glTexImage3D(glTexture2DArray, k, glR8, s, s, n, glRed, glUnsignedByte, nil)
		}
		plainTexture(glTexture2DArray, g.glyphTex)
		glTexParameteri(glTexture2DArray, glTextureBaseLevel, 0)
		glTexParameteri(glTexture2DArray, glTextureMaxLevel, levels-1)
		g.glyphCap, g.glyphSent = n, 0
	}
	glBindTexture(glTexture2DArray, g.glyphTex)
	glPixelStorei(glUnpackAlignment, 1)
	for i := g.glyphSent; i < len(g.glyphs); i++ {
		gl := g.glyphs[i]
		for k := range min(len(gl.lv), levels) {
			glTexSubImage3D(glTexture2DArray, k, 0, 0, i, gl.w[k], gl.h[k], 1, glRed, glUnsignedByte, unsafe.Pointer(&gl.lv[k][0]))
		}
	}
	g.glyphSent = len(g.glyphs)
	// Each letter's mean cover and whether it is two cells wide.
	rows := (len(g.glyphs) + 1023) / 1024
	g.info = growF(g.info, 4*1024*rows)
	for i, gl := range g.glyphs {
		wide := 0.0
		if gl.w[0] > glyphW {
			wide = 1
		}
		put4(g.info[4*i:], gl.mean, wide, 0, 0)
	}
	glBindTexture(glTexture2D, g.glyphInfoTex)
	glTexImage2D(glTexture2D, 0, glRGBA32F, 1024, rows, glRGBA, glFloat, unsafe.Pointer(&g.info[0]))
	return glGetError() == 0
}

// packRows is how many rows of pw RGBA pixels hold pw × ph pixels' red,
// green and blue bytes.
func packRows(pw, ph int) int { return (3*pw*ph + 4*pw - 1) / (4 * pw) }

func growF(b []float32, n int) []float32 {
	if cap(b) < n {
		return make([]float32, n)
	}
	b = b[:n]
	clear(b)
	return b
}

func growU(b []uint32, n int) []uint32 {
	if cap(b) < n {
		return make([]uint32, n)
	}
	return b[:n]
}

func put4(v []float32, a, b, c, d float64) {
	v[0], v[1], v[2], v[3] = float32(a), float32(b), float32(c), float32(d)
}

func putVert(b []byte, x, y float32, face int32) []byte {
	b = append(b, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0)
	p := b[len(b)-12:]
	*(*float32)(unsafe.Pointer(&p[0])) = x
	*(*float32)(unsafe.Pointer(&p[4])) = y
	*(*int32)(unsafe.Pointer(&p[8])) = face
	return b
}
