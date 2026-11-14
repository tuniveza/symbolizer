//go:build linux && cgo

package main

// ---- the windows, the town and the extra lights on the graphics card ---------------------------
//
// The same sums as design_3d_world.go, line for line (each function is
// named as the Go it follows): change one and change the other. The
// tables (the figures, the signs, the colours) are written in from the
// Go ones, so they cannot drift apart.

import (
	"fmt"
	"strings"
)

// gpuWorld goes between gpuFinish and gpuFS2 in step 2's program.
var gpuWorld = worldGLSL()

func worldGLSL() string {
	ints := func(name string, v []int) string {
		s := make([]string, len(v))
		for i, x := range v {
			s[i] = fmt.Sprint(x)
		}
		return fmt.Sprintf("const int %s[%d] = int[](%s);\n", name, len(v), strings.Join(s, ", "))
	}
	cols := func(name string, v []string) string {
		s := make([]string, len(v))
		for i, x := range v {
			s[i] = fmt.Sprintf("%du", hex32(x))
		}
		return fmt.Sprintf("const uint %s[%d] = uint[](%s);\n", name, len(v), strings.Join(s, ", "))
	}
	floats := func(name string, f func(l outLayer) float64) string {
		s := make([]string, len(outLayers))
		for i, l := range outLayers {
			s[i] = fmt.Sprintf("%g", f(l))
			if !strings.ContainsAny(s[i], ".e") {
				s[i] += ".0"
			}
		}
		return fmt.Sprintf("const float %s[%d] = float[](%s);\n", name, len(outLayers), strings.Join(s, ", "))
	}
	var b strings.Builder
	fmt.Fprintf(&b, "const int matWindow = %d;\nconst int NGLYPH = %d, NWALK = %d, WALKLANE = %d, SIGNMAX = %d, MAXX = %d;\n",
		matWindow, len(outChars), 3*worldWalkers, walkLane, signMax, maxXLights)
	fmt.Fprintf(&b, "const float laneNear = %g, laneFar = %g, postAt = %g, postEvery = %g, kerbAt = %g, roadAt = %g, roadEnd = %g, middleAt = %g;\n",
		laneNear, laneFar, postAt, postEvery, kerbAt, roadAt, roadEnd, middleAt)
	b.WriteString(ints("figIdx", figIdx))
	b.WriteString(ints("wordIdx", wordIdx))
	b.WriteString(ints("wordLen", wordLen))
	b.WriteString(ints("winIdx", winIdx))
	b.WriteString(ints("starIdx", starIdx))
	b.WriteString(ints("mirrorIdx", mirrorIdx))
	b.WriteString(cols("facadeCols", facadeCols))
	b.WriteString(cols("neonCols", neonCols))
	b.WriteString(cols("skinCols", skinCols))
	b.WriteString(floats("layS", func(l outLayer) float64 { return l.s }))
	b.WriteString(floats("laySlot", func(l outLayer) float64 { return l.slot }))
	b.WriteString(floats("layLo", func(l outLayer) float64 { return l.lo }))
	b.WriteString(floats("layHi", func(l outLayer) float64 { return l.hi }))
	b.WriteString(floats("layGap", func(l outLayer) float64 { return l.gap }))
	b.WriteString(floats("layHaze", func(l outLayer) float64 { return l.haze }))
	fmt.Fprintf(&b, "const int NFACADE = %d, NNEON = %d, NSIGN = %d, NWIN = %d, NSTAR = %d, NFIG = %d;\n",
		len(facadeCols), len(neonCols), len(signWords), len(winChars), len(starChars), len(walkerFigs)/2)
	b.WriteString(gpuWorldCode)
	return b.String()
}

const gpuWorldCode = `
uniform float oH, oG, oOu, oCloud, oDay, oStars, oLit, lGlowE;
uniform vec3 oMid, oSun, oMoon;
uniform uint oTop, oHorizon, oCloudCol, oHaze, oSunCol;
uniform int oTwink;
uniform vec3 oWalk[NWALK];
uniform int oGlyph[NGLYPH];
uniform int nX;
uniform vec3 xPos[MAXX], xCol[MAXX];
uniform float xInv[MAXX];

// whash, ihash
float ihash(int a, int b, int c) {
	uint x = uint(a) * 0x9E3779B1u ^ uint(b) * 0x85EBCA77u ^ uint(c) * 0xC2B2AE3Du;
	x ^= x >> 15u;
	x *= 0x2C1B3C6Du;
	x ^= x >> 12u;
	x *= 0x297A2D39u;
	x ^= x >> 15u;
	return float(x >> 8u) / 16777216.0;
}

// addCol, shade32k
uint addCol(uint a, uint b, float k) { return rgb32(clamp(chans(a) + chans(b) * k + 0.5, 0.0, 255.0)); }
uint shade32k(uint c, float k) { return rgb32(clamp(chans(c) * k + 0.5, 0.0, 255.0)); }

// r3world.glyph and r3glyph.cover: -1 is no letter
float outCover(int ch, float u, float v, float du, float dv) {
	if (ch <= 0) return -1.0;
	int layer = oGlyph[ch];
	if (layer < 0) return -1.0;
	return glyphCover(layer, u, v, du, dv);
}

// skyCol
uint skyCol(vec3 d) {
	float e = d.y;
	uint col = mix32(oHorizon, oTop, smoothstep(-0.02, 0.55, e));
	if (e > 0.02) {
		float cp = 0.35 / e;
		float cx = d.x * cp + oCloud, cz = d.z * cp;
		float dn = 0.6 * noise(cx, cz) + 0.4 * noise(cx * 2.3 + 5.0, cz * 2.3 + 9.0);
		float cov = smoothstep(0.05, 0.45, dn) * smoothstep(0.02, 0.12, e);
		col = mix32(col, oCloudCol, 0.85 * cov);
		if (oStars > 0.0 && e > 0.03) {
			const float sw = 0.014, sh = 0.028;
			float el = asin(min(e, 1.0));
			float gx = atan(d.x, d.z) / sw, gy = el / sh;
			int i = int(floor(gx)), j = int(floor(gy));
			if (ihash(i, j, 77) < 0.08) {
				float b = (0.55 + 0.45 * ihash(i, j, 79)) * (0.75 + 0.25 * ihash(i, j, oTwink));
				float du = 1.0 / (f * kx) / sw / max(cos(el), 0.2);
				float dv = 1.0 / (f * ky) / sh;
				float g = outCover(starIdx[int(ihash(i, j, 78) * float(NSTAR))], gx - float(i), 1.0 - (gy - float(j)), du, dv);
				if (g >= 0.0) col = mix32(col, 0xfff6dcu, g * oStars * b * (1.0 - cov));
			}
		}
	}
	float cs = dot(d, oSun);
	if (cs > 0.0) {
		float s2 = cs * cs, s4 = s2 * s2, s8 = s4 * s4, s16 = s8 * s8;
		col = addCol(col, oSunCol, 0.4 * s16 * s8);
		col = mix32(col, oSunCol, clamp01((cs - 0.9988) / 0.0005));
	}
	float cm = dot(d, oMoon);
	if (cm > 0.999) {
		vec3 cut = normalize(vec3(oMoon.x + 0.012, oMoon.y + 0.008, oMoon.z));
		float k = clamp01((cm - 0.9993) / 0.0003) * (1.0 - clamp01((dot(d, cut) - 0.9993) / 0.0003));
		col = mix32(col, 0xeef0f7u, k * oStars);
	}
	return col;
}

// r3world.facade
uint facade(int k, int i, int wall, float lx, float wd, float hy, float h, float du, float dv) {
	float day = oDay;
	uint base = facadeCols[int(ihash(i, wall, k + 20) * float(NFACADE))];
	uint col = shade32k(base, 0.2 + 0.8 * day);
	if (hy < oH && day < 1.0) col = addCol(col, 0xffb060u, 0.18 * (1.0 - day) * (1.0 - hy / oH));
	float cw = 1.15 * oOu;
	float ch = 2.0 * cw;
	if (hy > h - 0.35 * ch) return mix32(col, 0u, 0.3);
	int ncol = int(wd / cw);
	int j = int(floor(lx / cw)), q = int(floor(hy / ch));
	float u = lx / cw - float(j), v = 1.0 - (hy / ch - float(q));
	if (j < 1 || j >= ncol - 1) return col;
	if (k == 0 && q == 2) {
		int wi = int(ihash(i, wall, 50) * float(NSIGN));
		int n = wordLen[wi];
		int at = j - (ncol - n) / 2;
		col = mix32(col, 0x15161au, 0.8);
		if (at >= 0 && at < n) {
			float g = outCover(wordIdx[wi * SIGNMAX + at], u, v, du / cw, dv / ch);
			if (g >= 0.0) {
				uint neon = neonCols[int(ihash(i, wall, 51) * float(NNEON))];
				neon = shade32k(neon, 0.75 + 0.25 * (1.0 - day));
				col = mix32(col, neon, g);
			}
		}
		return col;
	}
	if (k == 0 && q < 2) {
		uint glass = mix32(mix32(0x1c2026u, oHorizon, 0.45 * day), 0xffd590u, 0.7 * (1.0 - day));
		return mix32(col, glass, cover(u, 0.1, 0.9, du / cw) * cover(hy, 0.15 * ch, 1.9 * ch, dv));
	}
	int style = int(ihash(i, wall, k + 30) * 3.0);
	int first = k == 0 ? 3 : 1;
	if (q < first || q >= int(h / ch) - 1 || style == 1 && j % 2 == 0 || style == 2 && j % 3 == 0) return col;
	bool lit = ihash(i * 131 + j, q, wall * 7 + k) < oLit;
	uint ink = mix32(mix32(col, 0u, 0.4), oHorizon, 0.4 * day);
	uint bg = mix32(col, 0u, 0.12);
	if (lit) {
		ink = 0xffd98au;
		if (ihash(i, j, q + 900) < 0.25) ink = 0xbfe0ffu;
		bg = mix32(col, ink, 0.22);
	}
	float g = outCover(winIdx[int(ihash(i, wall, k + 40) * float(NWIN))], u, v, du / cw, dv / ch);
	if (g < 0.0) return bg;
	return mix32(bg, ink, g);
}

// r3world.street
uint street(float s, float a, int wall) {
	float H = oH;
	float sh = s / H;
	uint col;
	if (sh < kerbAt) col = 0x8a8a86u;
	else if (sh < roadAt) col = 0xa8a8a2u;
	else if (sh < roadEnd) {
		col = 0x3a3d42u;
		if (abs(sh - middleAt) < 0.04 && gmod(a / H, 1.2) < 0.6) col = 0xe8e0b0u;
	} else if (sh < layS[0]) col = 0x8a8a86u;
	else col = 0x2a2a2cu;
	col = shade32k(col, 0.35 + 0.65 * oDay);
	if (oDay < 1.0) {
		float dx = (gmod(a, postEvery * H) - postEvery * H / 2.0) / H;
		float dz = sh - postAt;
		col = addCol(col, 0xffb060u, 0.5 * (1.0 - oDay) / (1.0 + (dx * dx + dz * dz) * 4.0));
	}
	return mix32(col, oHaze, clamp01(s / (30.0 * H)));
}

// the people of r3world.outside
uint people(uint col, int lane, int wall, float a0, float y0, float ka, float kyy, float sHit, float t, float dOut, float slant, vec3 along) {
	float s = lane == 1 ? laneFar * oH : laneNear * oH;
	if (s >= sHit) return col;
	float hy = y0 + s * kyy - oG;
	if (hy < 0.0 || hy >= 6.0 * oOu) return col;
	float a = a0 + s * ka - dot(oMid, along);
	float z = (t + s / dOut) / f * slant;
	float du = z / kx, dv = z / ky;
	for (int i = 0; i < WALKLANE; i++) {
		vec3 wk = oWalk[wall * 2 * WALKLANE + lane * WALKLANE + i];
		float dx = (a - wk.x) / oOu + 1.5;
		if (dx < 0.0 || dx >= 3.0) continue;
		int c = int(dx), rr = 2 - int(hy / (2.0 * oOu));
		int code = int(wk.y);
		int fig = code & 15;
		int ch = lane == 1 ? mirrorIdx[figIdx[fig * 9 + rr * 3 + 2 - c]] : figIdx[fig * 9 + rr * 3 + c];
		float g = outCover(ch, dx - float(c), 1.0 - (hy / (2.0 * oOu) - float(2 - rr)), du / oOu, dv / (2.0 * oOu));
		if (g < 0.0) continue;
		uint ink = uint(wk.z);
		if (rr == 0) ink = skinCols[(code >> 4) & 3];
		else if (rr == 2) ink = mix32(ink, 0x202020u, 0.55);
		ink = shade32k(ink, 0.4 + 0.6 * oDay + 0.25 * (1.0 - oDay));
		return mix32(col, ink, g);
	}
	return col;
}

// r3world.outside
uint outside(vec3 n, vec3 p, vec3 dir, float t) {
	vec3 outv = vec3(-n.x, 0.0, -n.z), along = vec3(n.z, 0.0, -n.x);
	int wall = n.x > 0.5 ? 0 : (n.x < -0.5 ? 1 : 2);
	float dOut = max(dot(dir, outv), 1e-6);
	float ka = dot(dir, along) / dOut, kyy = dir.y / dOut;
	float a0 = dot(p, along), y0 = p.y;
	float dl = sqrt(dot(dir, dir));
	float H = oH, G = oG;
	float slant = min(dl / dOut, 6.0);
	float sG = 1e30;
	if (dir.y < 0.0) sG = (G - y0) / kyy;
	float sHit = sG;
	uint col = 0u;
	bool hit = false;
	for (int k = 0; k < 3; k++) {
		float s = layS[k] * H;
		if (s >= sG) break;
		float hy = y0 + s * kyy - G;
		float sw = laySlot[k] * H;
		float a = a0 + s * ka + float(wall) * sw * 7.31;
		float fi = floor(a / sw);
		int i = int(fi);
		float fx = a / sw - fi;
		float gap = layGap[k] * ihash(i, wall, k + 10);
		float hb = ihash(i, wall, k);
		float h = (layLo[k] + (layHi[k] - layLo[k]) * hb * hb) * H;
		if (fx < gap || fx > 1.0 - 0.5 * gap || hy >= h) continue;
		float z = (t + s / dOut) / f * slant;
		col = facade(k, i, wall, (fx - gap) * sw, (1.0 - 1.5 * gap) * sw, hy, h, z / kx, z / ky);
		col = mix32(col, oHaze, layHaze[k] * (0.5 + 0.5 * oDay));
		sHit = s;
		hit = true;
		break;
	}
	if (!hit) {
		if (sG < 1e30) col = street(sG, a0 + sG * ka, wall);
		else col = skyCol(dir / dl);
	}
	float day = oDay;
	col = people(col, 1, wall, a0, y0, ka, kyy, sHit, t, dOut, slant, along);
	float s = postAt * H;
	if (s < sHit) {
		float hy = y0 + s * kyy - G;
		float a = a0 + s * ka;
		float pe = postEvery * H;
		float dx = (gmod(a, pe) - pe / 2.0) / H;
		float du = (t + s / dOut) / f * slant / kx;
		const float top = 1.35;
		if (hy >= 0.0 && hy < top * H) col = mix32(col, mix32(0x2a2c30u, oHorizon, 0.15), cover(dx, -0.018, 0.018, du / H));
		float r2 = dx * dx + (hy / H - top) * (hy / H - top);
		if (r2 < 0.0049) col = mix32(col, mix32(0x50545au, 0xffe2a8u, 1.0 - day), clamp01((0.0049 - r2) / 0.002));
		else if (day < 1.0) col = addCol(col, 0xffc070u, 0.35 * (1.0 - day) / (1.0 + r2 * 60.0));
	}
	col = people(col, 0, wall, a0, y0, ka, kyy, sHit, t, dOut, slant, along);
	return col;
}

// room3D.windowAt
uint windowAt(int fi, vec3 n, vec3 p, vec3 dir, float t, uint fcol, out bool glass) {
	vec4 r2 = faceRow(fi, 2), r3 = faceRow(fi, 3), r4 = faceRow(fi, 4);
	vec3 q = p - r2.xyz;
	float u = dot(q, r3.xyz), v = dot(q, r4.xyz), lu = r2.w, lv = r3.w;
	float dl = sqrt(dot(dir, dir));
	float foot = t / f * kpx / max(-dot(n, dir) / dl, 0.12);
	float fw = 0.07 * min(lu, lv);
	float mw = 0.3 * fw;
	float d = min(min(u, lu - u), min(v, lv - v));
	float k = cover(d, -1.0, fw, foot);
	k = max(k, cover(u - lu / 2.0, -mw, mw, foot));
	k = max(k, cover(v - 0.66 * lv, -mw, mw, foot));
	glass = k < 0.5;
	if (!glass) {
		uint col = fcol;
		if (d < fw && d > 0.7 * fw) col = mix32(col, 0u, 0.18);
		return col;
	}
	uint col = outside(n, p, dir, t);
	return mix32(col, mix32(fcol, 0u, 0.5), 2.0 * k);
}

// r3world.xlightAt
vec3 xlightAt(vec3 n, vec3 p) {
	vec3 e = vec3(0.0);
	for (int i = 0; i < nX; i++) {
		vec3 L = xPos[i] - p;
		float d2 = dot(L, L);
		float ndl = dot(n, L) / sqrt(d2);
		if (ndl <= 0.0) continue;
		e += xCol[i] * (ndl / (1.0 + d2 * xInv[i]));
	}
	return e;
}
`
