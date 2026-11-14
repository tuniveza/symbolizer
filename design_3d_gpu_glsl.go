//go:build linux && cgo

package main

import "fmt"

// ---- the 3D room's shaders (design_3d_gpu.go) --------------------------------------------------
//
// Each piece follows the Go it is written from (named beside it): change
// one and change the other, or the picture on the card and the one on the
// processor drift apart (TestDesign3DGPUMatches compares them).

// gpuVS1 and gpuFS1: step 1, the nearest face at each pixel (depthRows).
// The outlines come in the picture's pixels (x right, y down); row 0 of
// the picture is the framebuffer's row 0, so it reads back top row first.
const gpuVS1 = `#version 330 core
layout(location = 0) in vec2 pos;
layout(location = 1) in int face;
flat out int vFace;
uniform float pw, ph;
void main() {
	vFace = face;
	gl_Position = vec4(pos.x / pw * 2.0 - 1.0, pos.y / ph * 2.0 - 1.0, 0.0, 1.0);
}
`

const gpuFS1 = `#version 330 core
flat in int vFace;
layout(location = 0) out int outFace;
uniform sampler2D faces;
uniform vec3 fwd, right, up;
uniform float f, px, py, kx, ky;
void main() {
	int i = vFace * 6;
	vec4 a = texelFetch(faces, ivec2(i & 1023, i >> 10), 0);
	vec3 dir = fwd + right * ((gl_FragCoord.x / kx - px) / f) + up * (-(gl_FragCoord.y / ky - py) / f);
	float nd = dot(a.xyz, dir);
	if (nd >= -1e-12) discard;
	float t = a.w / nd;
	if (t <= 0.0) discard;
	gl_FragDepth = min(t / 16777216.0, 1.0); // a power of two: the depth is t, as the processor's float32
	outFace = vFace;
}
`

// gpuVS2: step 2 covers the picture with one triangle.
const gpuVS2 = `#version 330 core
void main() {
	vec2 p = vec2(gl_VertexID == 1 ? 3.0 : -1.0, gl_VertexID == 2 ? 3.0 : -1.0);
	gl_Position = vec4(p, 0.0, 1.0);
}
`

// gpuHead: step 2's inputs and the colour sums (design_3d_draw.go).
const gpuHead = `#version 330 core
out vec4 outCol;
uniform sampler2D faces;
uniform isampler2D ids;
uniform usampler2D mips, flat0, cells;
uniform sampler2DArray glyphs;
uniform sampler2D glyphInfo, shadowTex, noiseTex;

uniform vec3 cam, fwd, right, up, lamp;
uniform float f, px, py, kx, ky, kpx, invR2;
uniform float lAmbient, lFill, lLamp, lGlow, lSpec;
uniform float hw, hh, minU, minV, gk;
uniform int tx0, ty0, tw, th;
uniform uint textCol, outer, bgPlain, wallCol;
uniform float rmFy, rmCy, rmBig;
uniform int nMips;
uniform int mipW[6], mipH[6], mipOff[6];

const int matBoard = 0, matFront = 1, matSolid = 2, matFloor = 3, matWall = 4,
	matBack = 5, matCeiling = 6, matLamp = 7, matShade = 8;

vec4 faceRow(int fi, int j) {
	int i = fi * 6 + j;
	return texelFetch(faces, ivec2(i & 1023, i >> 10), 0);
}

// chans, rgb32, mix32, shade32
vec3 chans(uint c) { return vec3(float((c >> 16) & 255u), float((c >> 8) & 255u), float(c & 255u)); }
uint rgb32(vec3 v) { uvec3 u = uvec3(max(v, 0.0)); return (u.r << 16) | (u.g << 8) | u.b; }
uint mix32(uint a, uint b, float t) { vec3 A = chans(a), B = chans(b); return rgb32(A + (B - A) * t + 0.5); }
uint shade32(uint c, float d) { return rgb32(clamp(chans(c) + d + 0.5, 0.0, 255.0)); }
float clamp01(float v) { return clamp(v, 0.0, 1.0); }
float gmod(float a, float b) { return a - b * floor(a / b); }

// noise (noiseFine)
float noise(float x, float y) {
	int ix = int(floor(x * 16.0)) & 1023, iy = int(floor(y * 16.0)) & 1023;
	return texelFetch(noiseTex, ivec2(ix, iy), 0).r;
}

// plaster
float plaster(float u, float v, float foot) {
	const float size[3] = float[3](13.0, 4.5, 1.6);
	const float w[3] = float[3](0.5, 0.32, 0.25);
	float s = 0.0;
	for (int k = 0; k < 3; k++) {
		float fade = clamp01(size[k] / foot - 1.5);
		if (fade == 0.0) break;
		float off = 23.7 * float(k);
		s += w[k] * fade * noise(u / size[k] + off, v / size[k] + off);
	}
	return s;
}

// cover
float cover(float h, float lo, float hi, float foot) {
	return clamp01((min(h + foot / 2.0, hi) - max(h - foot / 2.0, lo)) / foot);
}

// paperWall (wallWith; trellisPaper is in gpuFinish, design_3d_finish_glsl.go)
uint trellisPaper(uint col, uint base, uint text, float u, float h, float foot);
uint paperWall(uint base, uint text, float u, float y, float foot, float pl, bool trellis) {
	float h = y - rmFy;
	const float skirt = 3.0, rail = 1.2;
	float dado = max(9.0, 0.14 * rmBig);
	float pic = rmCy - rmFy - max(3.0, 0.05 * rmBig);
	bool hasPic = !(pic < dado + 8.0);
	float d = gk * 4.5 * pl;
	if (h < dado) {
		d -= 3.0 * gk;
	} else if (!trellis && (!hasPic || h < pic)) {
		const float period = 6.0;
		float a = clamp01(period / foot / 3.0 - 0.5);
		if (a > 0.0) {
			float k = u / period - floor(u / period + 0.5);
			d += 2.5 * gk * a * (1.0 - 8.0 * k * k);
			d -= 6.0 * gk * a * cover(k * period, -0.15, 0.15, foot);
		}
	}
	uint col = shade32(base, d);
	if (trellis && h >= dado && (!hasPic || h < pic)) col = trellisPaper(col, base, text, u, h, foot);
	float near = rail + foot + 1.0;
	if (h < skirt + near) {
		uint trim = mix32(base, text, 0.13);
		float c = cover(h, -1.0, skirt, foot);
		if (c > 0.0) col = mix32(col, trim, c);
		c = cover(h, skirt, skirt + 0.4, foot);
		if (c > 0.0) col = mix32(col, 0u, 0.1 * c);
	}
	for (int i = 0; i < 2; i++) {
		if (i == 1 && !hasPic) break;
		float at = i == 0 ? dado : pic;
		if (h < at - near || h > at + near) continue;
		col = mix32(col, 0u, 0.12 * cover(h, at - 0.45, at, foot));
		col = mix32(col, mix32(base, text, 0.13), cover(h, at, at + rail, foot));
	}
	return col;
}
uint wallWith(uint base, uint text, float u, float y, float foot, float pl) { return paperWall(base, text, u, y, foot, pl, false); }

// floorAt
uint floorAt(uint base, float x, float z, float pxs) {
	const float plankW = 7.0, plankL = 34.0;
	float p = floor(x / plankW);
	float along = z + gmod(p * 13.0, plankL);
	float hash = gmod(p * 7.31 + floor(along / plankL) * 3.17, 1.0);
	float dark = 0.04 + 0.03 * hash;
	float d = min(x - p * plankW, plankW - (x - p * plankW));
	float j = clamp01(1.0 - (d - 0.25) / max(pxs, 0.3));
	dark += 0.1 * j * clamp01(1.6 - pxs);
	return mix32(base, 0u, dark);
}

// mips (room3D.mips), one under another
uint mipAt(int k, int x, int y) {
	x = clamp(x, 0, mipW[k] - 1);
	y = clamp(y, 0, mipH[k] - 1);
	return texelFetch(mips, ivec2(x, mipOff[k] + y), 0).r;
}

// smooth
uint flatAt(int x, int y) {
	return texelFetch(flat0, ivec2(clamp(x, 0, mipW[0] - 1), clamp(y, 0, mipH[0] - 1)), 0).r;
}
uint smoothAt(float x, float y, float s) {
	x -= 0.5;
	y -= 0.5;
	int x0 = int(floor(x)), y0 = int(floor(y));
	float fx = x - float(x0), fy = y - float(y0);
	s = max(s, 1e-3);
	if (s < 1.0) {
		fx = clamp01((fx - 0.5) / s + 0.5);
		fy = clamp01((fy - 0.5) / s + 0.5);
		if ((fx == 0.0 || fx == 1.0) && (fy == 0.0 || fy == 1.0)) return flatAt(x0 + int(fx), y0 + int(fy));
	}
	uint a = flatAt(x0, y0), b = flatAt(x0 + 1, y0), c = flatAt(x0, y0 + 1), d = flatAt(x0 + 1, y0 + 1);
	if (a == b && b == c && c == d) return a;
	vec3 v = (chans(a) * (1.0 - fx) + chans(b) * fx) * (1.0 - fy) + (chans(c) * (1.0 - fx) + chans(d) * fx) * fy;
	return rgb32(v + 0.5);
}

// room3D.mipAt and wallAt
uint mipLin(int k, float x, float y) {
	float sc = 1.0 / float(1 << k);
	x = x * sc - 0.5;
	y = y * sc - 0.5;
	int x0 = int(floor(x)), y0 = int(floor(y));
	float fx = x - float(x0), fy = y - float(y0);
	uint a = mipAt(k, x0, y0), b = mipAt(k, x0 + 1, y0), c = mipAt(k, x0, y0 + 1), d = mipAt(k, x0 + 1, y0 + 1);
	if (a == b && b == c && c == d) return a;
	vec3 v = (chans(a) * (1.0 - fx) + chans(b) * fx) * (1.0 - fy) + (chans(c) * (1.0 - fx) + chans(d) * fx) * fy;
	return rgb32(v + 0.5);
}
uint wallAt(float x, float y, float s) {
	float l = log2(max(s, 1e-9) / wallSharp);
	if (l <= 0.0) return smoothAt(x, y, s);
	int k = int(l);
	float t = l - float(k);
	if (k >= nMips - 1) return mipLin(nMips - 1, x, y);
	uint a = k == 0 ? smoothAt(x, y, s) : mipLin(k, x, y);
	return mix32(a, mipLin(k + 1, x, y), t);
}

// r3glyph.cover
float glyphAt(int layer, int k, int lw, int lh, int x, int y) {
	if (x < 0 || y < 0 || x >= lw || y >= lh) return 0.0;
	return texelFetch(glyphs, ivec3(x, y, layer), k).r;
}
float glyphCoverAt(int layer, int k, bool wide, float u, float v) {
	int lw = wide ? (96 >> k) : (48 >> k), lh = 96 >> k;
	float cw = float(lw);
	if (wide) cw /= 2.0;
	float x = u * cw - 0.5, y = v * float(lh) - 0.5;
	int x0 = int(floor(x)), y0 = int(floor(y));
	float fx = x - float(x0), fy = y - float(y0);
	float a = glyphAt(layer, k, lw, lh, x0, y0) * (1.0 - fx) + glyphAt(layer, k, lw, lh, x0 + 1, y0) * fx;
	float b = glyphAt(layer, k, lw, lh, x0, y0 + 1) * (1.0 - fx) + glyphAt(layer, k, lw, lh, x0 + 1, y0 + 1) * fx;
	return a * (1.0 - fy) + b * fy;
}
float glyphCover(int layer, float u, float v, float du, float dv) {
	vec4 info = texelFetch(glyphInfo, ivec2(layer & 1023, layer >> 10), 0);
	bool wide = info.g > 0.5;
	int nl = wide ? 7 : 6;
	float l = log2(max(max(du * 48.0, dv * 96.0), 1e-9) * glyphSharp);
	if (l <= 0.0) return glyphCoverAt(layer, 0, wide, u, v);
	int k = int(l);
	if (k >= nl) return info.r;
	float t = l - float(k);
	float c = glyphCoverAt(layer, k, wide, u, v);
	float next = k + 1 < nl ? glyphCoverAt(layer, k + 1, wide, u, v) : info.r;
	return c + (next - c) * t;
}

// lineCoverUV
float over(float c, float lo, float hi, float p) {
	return clamp01((min(c + p / 2.0, hi) - max(c - p / 2.0, lo)) / p);
}
float lineCoverUV(int arms, float u, float v, float pu, float pv, float mU, float mV) {
	float hu = max(mU, 0.5 * pu), hv = max(mV, 0.5 * pv);
	float cov = 0.0;
	if ((arms & 12) != 0) {
		float lo = 0.5 - hu, hi = 0.5 + hu;
		if ((arms & 4) != 0) lo = -1.0;
		if ((arms & 8) != 0) hi = 2.0;
		cov = over(u, lo, hi, pu) * over(v, 0.5 - hv, 0.5 + hv, pv);
	}
	if ((arms & 3) != 0) {
		float lo = 0.5 - hv, hi = 0.5 + hv;
		if ((arms & 1) != 0) lo = -1.0;
		if ((arms & 2) != 0) hi = 2.0;
		cov = max(cov, over(v, lo, hi, pv) * over(u, 0.5 - hu, 0.5 + hu, pu));
	}
	return cov;
}

// shadowAt
float shadowAt(float u, float v) {
	int w = tw, ph = 2 * th;
	u = clamp(u - 0.5, 0.0, float(w - 1));
	v = clamp(v - 0.5, 0.0, float(ph - 1));
	int i = int(u), j = int(v);
	int i1 = min(i + 1, w - 1), j1 = min(j + 1, ph - 1);
	float fu = u - float(i), fv = v - float(j);
	float a = texelFetch(shadowTex, ivec2(i, j), 0).r, b = texelFetch(shadowTex, ivec2(i1, j), 0).r;
	float c = texelFetch(shadowTex, ivec2(i, j1), 0).r, d = texelFetch(shadowTex, ivec2(i1, j1), 0).r;
	float top = a + (b - a) * fu, bot = c + (d - c) * fu;
	return top + (bot - top) * fv;
}
`

// gpuTextConsts are the words' and lines' sharpness and fades, as the
// processor has them (design_3d_glyphs.go).
func gpuTextConsts() string {
	return fmt.Sprintf("const float glyphSharp = %g, wallSharp = %g, glyphFull = %g, glyphGone = %g, lineFull = %g, lineGone = %g;\n",
		glyphSharp, wallSharp, glyphFull, glyphGone, lineFull, lineGone)
}

// gpuFS2: step 2's main, shadePixel and lit (after gpuFinish, design_3d_finish_glsl.go).
const gpuFS2 = `
void main() {
	ivec2 pix = ivec2(gl_FragCoord.xy);
	int fi = texelFetch(ids, pix, 0).r;
	if (fi < 0) {
		outCol = vec4(chans(outer) / 255.0, 1.0);
		return;
	}
	vec3 dir = fwd + right * ((gl_FragCoord.x / kx - px) / f) + up * (-(gl_FragCoord.y / ky - py) / f);
	vec4 r0 = faceRow(fi, 0), r1 = faceRow(fi, 1);
	vec3 n = r0.xyz;
	int mat = int(r1.x);
	uint fcol = uint(r1.y);
	float gloss = r1.z;
	int flags = int(r1.w);
	bool inside = (flags & 1) != 0, edge = (flags & 2) != 0;
	float nd = dot(n, dir);
	float t = r0.w / nd;
	vec3 p = cam + dir * t;
	float dl = 1.0 / sqrt(dot(dir, dir));
	float foot = t / f * kpx / max(-nd * dl, 0.12);
	uint col = fcol;
	float shade = 1.0;
	bool shadowOn = false;
	if (mat == matBoard || mat == matFront) {
		vec4 r5 = faceRow(fi, 5);
		int cx0 = int(r5.x), cx1 = int(r5.y), py0 = int(r5.z), py1 = int(r5.w);
		int tdy = int(faceRow(fi, 4).w); // a block out on a layer: its drawing is kept below the board
		float fx = p.x + hw;
		float hp = hh - p.y + float(tdy);
		int cx = max(cx0, min(cx1 - 1, int(floor(fx))));
		int hy = max(py0, min(py1 - 1, int(floor(hh - p.y)))) + tdy;
		float sc = 1.0;
		if ((flags & 4) != 0) { // a turned card (design_3d_pins.go)
			vec4 au = faceRow(fi, 2), av = faceRow(fi, 3), tr = faceRow(fi, 4);
			fx = dot(au.xyz, p) + au.w;
			hp = dot(av.xyz, p) + av.w;
			cx = max(int(tr.x), min(int(tr.y) - 1, int(floor(fx))));
			hy = max(int(tr.z), min(int(tr.w) - 1, int(floor(hp))));
			sc = max(length(au.xyz), length(av.xyz));
		}
		int tx = 2 * (cx - tx0) + int(2.0 * (fx - float(cx)));
		int ty = hy - 2 * ty0;
		int k = 0;
		float s0 = t / f * sc;
		float s = s0 * kpx;
		if (s > 1.25 || nd > -0.5) {
			float q = sqrt(max(-nd / sqrt(dot(dir, dir)), 0.03));
			s /= q;
			s0 /= q;
		}
		col = wallAt(2.0 * (fx - float(tx0)), hp - float(2 * ty0), s);
		int cy = hy >> 1;
		uvec4 cell = texelFetch(cells, ivec2(clamp(cx - tx0, 0, tw - 1), clamp(cy - ty0, 0, th - 1)), 0);
		uint gi = cell.z & 0xffffu;
		if (s < glyphGone && gi != 0u) {
			float u = fx - float(cx) + float((cell.z >> 16) & 1u);
			float v = hp / 2.0 - float(cy);
			uint g = mix32(cell.y, cell.x, glyphCover(int(gi) - 1, u, v, s0 / kx, s0 / (2.0 * ky)));
			col = mix32(col, g, clamp01((glyphGone - s) / (glyphGone - glyphFull)));
		}
		if (s < lineGone && cell.w != 0u) {
			float cov = lineCoverUV(int(cell.w), fx - float(cx), hp / 2.0 - float(cy), s0 / kx, s0 / (2.0 * ky), minU, minV);
			col = mix32(col, mix32(cell.y, cell.x, 0.85 * cov), clamp01((lineGone - s) / (lineGone - lineFull)));
		}
		if (mat == matBoard) {
			if (col == bgPlain) col = wallFinish(wallCol, textCol, p.x, p.y, foot, plaster(p.x, p.y, foot));
			shadowOn = true;
		}
	} else if (mat == matSolid) {
		if (edge) {
			vec4 r2 = faceRow(fi, 2), r3 = faceRow(fi, 3), r4 = faceRow(fi, 4);
			vec3 q = p - r2.xyz;
			float u = dot(q, r3.xyz), v = dot(q, r4.xyz), lu = r2.w, lv = r3.w;
			float e = min(0.45, 0.25 * min(lu, lv));
			float pxs = max(t / f * kpx, 0.1 * kpx);
			float d = min(min(u, v), min(lu - u, lv - v));
			float cov = clamp01((e - d) / pxs + 0.5);
			if (cov > 0.0) col = mix32(col, 0u, 0.3 * cov * min(1.0, 2.0 * e / pxs));
		}
	} else if (mat == matFloor) {
		col = floorFinish(fcol, p.x + hw, p.z, t / f * kpx, foot);
	} else if (mat == matWall) {
		float u = n.z != 0.0 ? p.x : p.z;
		col = wallFinish(fcol, textCol, u, p.y, foot, plaster(u, p.y, foot));
	} else if (mat == matBack) {
		col = wallFinish(fcol, textCol, p.x, p.y, foot, plaster(p.x, p.y, foot));
	} else if (mat == matCeiling) {
		col = shade32(fcol, 5.0 * gk * plaster(p.x, p.z, foot));
	} else if (mat == matWindow) {
		bool glass;
		col = windowAt(fi, n, p, dir, t, fcol, glass); // design_3d_world_glsl.go
		if (glass) {
			outCol = vec4(chans(col) / 255.0, 1.0);
			return;
		}
	}

	// The light (shadePixel), worked out at every pixel.
	if (shadowOn) shade = 1.0 - 0.7 * shadowAt(p.x + hw - float(tx0), hh - p.y - float(2 * ty0));
	vec3 vv = -dir * dl;
	float fill = dot(n, vv);
	float mul, add;
	if (mat == matLamp) {
		mul = 0.85 + 0.15 * fill;
		add = 0.0;
	} else {
		vec3 L = lamp - p;
		float d2 = dot(L, L);
		L *= 1.0 / sqrt(d2);
		float att = 1.0 / (1.0 + d2 * invR2);
		float ndl = dot(n, L);
		if (mat == matShade && inside) ndl = 1.0;
		float diff = 0.0, spec = 0.0;
		if (ndl > 0.0) {
			diff = ndl * att * shade;
			vec3 H = L + vv;
			float hn = dot(n, H) / sqrt(dot(H, H));
			if (hn > 0.0) {
				float s = hn * hn;
				s *= s;
				s *= s;
				s *= s;
				s *= s;
				spec = s * att * gloss * shade;
			}
		}
		mul = lAmbient + lFill * fill + lLamp * diff;
		add = lGlow * diff + lSpec * spec;
	}
	// lit, with the extra lights (design_3d_world_glsl.go)
	vec3 E = mat == matLamp ? vec3(0.0) : xlightAt(n, p);
	vec3 o = floor(clamp(chans(col) * (mul + E) + add * vec3(1.0, 0.9, 0.72) + lGlowE * E, 0.0, 255.0));
	outCol = vec4(o / 255.0, 1.0);
}
`

// gpuFS3: step 3, the picture's red, green and blue bytes, row by row as
// kitty takes them (f=24), four to a pixel of an RGBA picture pw wide.
const gpuFS3 = `#version 330 core
out vec4 outCol;
uniform sampler2D pic;
uniform int pw, ph;
void main() {
	int b0 = 4 * (int(gl_FragCoord.y) * pw + int(gl_FragCoord.x));
	vec4 o;
	for (int c = 0; c < 4; c++) {
		int b = b0 + c, p = b / 3;
		ivec2 q = ivec2(p % pw, min(p / pw, ph - 1));
		o[c] = texelFetch(pic, q, 0)[b - 3 * p];
	}
	outCol = o;
}
`
