//go:build linux && cgo

package main

// ---- the room's finishes on the graphics card (design_3d_finish.go) -----------------------------
//
// Each piece follows the Go it is written from (named beside it): change
// one and change the other (TestDesign3DGPUFinishes compares them). It
// comes after gpuHead, whose sums it uses, and before gpuFS2, which calls
// wallFinish and floorFinish.

const gpuFinish = `
uniform int wallFin, floorFin;

// fracF is fract; modF is gmod.
// shows
float shows(float size, float foot) { return clamp01(size / foot / 2.0 - 0.5); }
// seam
float seam(float d, float w, float foot) { return cover(d, -w, w, foot); }
// cellRand
float cellRand(float i, float j) { return noise(i + 0.03, j + 0.03); }
// noiseSmooth
float noiseFineAt(int x, int y) { return texelFetch(noiseTex, ivec2(x & 1023, y & 1023), 0).r; }
float noiseSmooth(float x, float y) {
	x *= 16.0;
	y *= 16.0;
	float fx = floor(x), fy = floor(y);
	float u = x - fx, v = y - fy;
	int ix = int(fx), iy = int(fy);
	float a = noiseFineAt(ix, iy), b = noiseFineAt(ix + 1, iy);
	float c = noiseFineAt(ix, iy + 1), d = noiseFineAt(ix + 1, iy + 1);
	float top = a + (b - a) * u, bot = c + (d - c) * u;
	return top + (bot - top) * v;
}
// filteredChecker
float filteredChecker(float x, float y, float w) {
	w = max(w, 1e-4);
	float ix = 2.0 * (abs(fract((x - 0.5 * w) * 0.5) - 0.5) - abs(fract((x + 0.5 * w) * 0.5) - 0.5)) / w;
	float iy = 2.0 * (abs(fract((y - 0.5 * w) * 0.5) - 0.5) - abs(fract((y + 0.5 * w) * 0.5) - 0.5)) / w;
	return 0.5 - 0.5 * ix * iy;
}

// The finishes' own colours.
const uint colBrick = 0x9c4f3au, colMortar = 0xc4beb2u, colWood = 0x8a5a36u, colConcr = 0xa29e96u,
	colTile = 0xf0eee8u, colGrout = 0x8f8d88u, colFelt = 0x2a3440u, colParquet = 0x9a6a40u,
	colCarpet = 0x4b5563u, colMarbleL = 0xe9e5ddu, colMarbleD = 0x3a3a40u, colOak = 0xc9a77cu, colSlate = 0x3f474au;

// brickAt
uint brickAt(uint base, float u, float h, float foot, float pl) {
	const float bw = 8.0, bh = 3.0, mortar = 0.22;
	float row = floor(h / bh);
	float uu = u + 0.5 * bw * gmod(row, 2.0);
	float col = floor(uu / bw);
	float fu = uu - col * bw, fh = h - row * bh;
	float d = gk * (3.0 * pl + 7.0 * shows(bh, foot) * cellRand(col + 7.0, row) + 4.0 * shows(0.7, foot) * noise(u / 0.7 + 5.1, h / 0.7 + 2.3));
	uint c = shade32(mix32(base, colBrick, 0.6), d);
	float mx = seam(min(fu, bw - fu), mortar, foot);
	float my = seam(min(fh, bh - fh), mortar, foot);
	return mix32(c, mix32(base, colMortar, 0.45), mx + my - mx * my);
}

// panelsAt
uint panelsAt(uint base, float u, float h, float foot, float pl) {
	const float bw = 4.5, groove = 0.13;
	float b = floor(u / bw);
	float fu = u - b * bw;
	float g = noise(u / 0.45 + b * 13.1, h / 9.0);
	float d = gk * (2.0 * pl + 6.0 * shows(bw, foot) * cellRand(b, 11.0) + 7.0 * shows(0.45, foot) * g + 4.0 * shows(1.6, foot) * noise(u / 1.6, h / 20.0 + 3.7));
	uint c = shade32(mix32(base, colWood, 0.6), d);
	c = mix32(c, 0u, 0.45 * seam(min(fu, bw - fu), groove, foot));
	if (h < 3.3 + foot) {
		c = mix32(c, mix32(base, colWood, 0.85), cover(h, -1.0, 3.0, foot));
		c = mix32(c, 0u, 0.25 * cover(h, 3.0, 3.3, foot));
	}
	return c;
}

// concreteWallAt
uint concreteWallAt(uint base, float u, float h, float foot, float pl) {
	float d = gk * (6.0 * pl + 5.0 * noise(u / 31.0 + 3.3, h / 31.0 + 8.1));
	uint c = shade32(mix32(base, colConcr, 0.35), d);
	float su = min(gmod(u, 24.0), 24.0 - gmod(u, 24.0));
	float sh = min(gmod(h, 12.0), 12.0 - gmod(h, 12.0));
	c = mix32(c, 0u, 0.12 * max(seam(su, 0.08, foot), seam(sh, 0.08, foot)));
	float du = gmod(u, 12.0) - 6.0, dh = gmod(h, 6.0) - 3.0;
	float r = sqrt(du * du + dh * dh);
	return mix32(c, 0u, 0.3 * shows(0.6, foot) * seam(r, 0.3, foot));
}

// tilesAt
uint tilesAt(uint base, float u, float h, float foot, float pl) {
	const float tw = 4.0, th = 2.0, grout = 0.08;
	float row = floor(h / th);
	float uu = u + 0.5 * tw * gmod(row, 2.0);
	float col = floor(uu / tw);
	float fu = uu - col * tw, fh = h - row * th;
	float eu = min(fu, tw - fu), eh = min(fh, th - fh);
	float d = gk * (1.5 * pl + 3.0 * shows(th, foot) * cellRand(col + 19.0, row + 5.0));
	float vis = shows(0.6, foot);
	d += vis * (10.0 * seam(th - fh, 0.25, foot) - 9.0 * seam(fh, 0.25, foot) - 5.0 * seam(eu, 0.25, foot));
	uint c = shade32(mix32(base, colTile, 0.7), d);
	float gu = seam(eu, grout, foot), gh = seam(eh, grout, foot);
	return mix32(c, mix32(base, colGrout, 0.6), gu + gh - gu * gh);
}

// trellisPaper
uint trellisPaper(uint col, uint base, uint text, float u, float h, float foot) {
	const float cw = 5.0, ch = 7.0;
	float fu = (fract(u / cw) - 0.5) * cw, fh = (fract(h / ch) - 0.5) * ch;
	float q = abs(fu) / (cw / 2.0) + abs(fh) / (ch / 2.0);
	float vis = shows(1.2, foot);
	col = mix32(col, mix32(base, text, 0.15), vis * seam((1.0 - q) * 1.9, 0.11, foot));
	return mix32(col, mix32(base, text, 0.12), vis * seam(sqrt(fu * fu + fh * fh), 0.35, foot));
}

// feltAt
uint feltAt(uint base, float u, float h, float foot, float pl) {
	const float pw = 10.0, gap = 0.25;
	float bu = floor(u / pw), bh = floor(h / pw);
	float eu = min(u - bu * pw, pw - (u - bu * pw)), eh = min(h - bh * pw, pw - (h - bh * pw));
	float d = gk * (2.0 * pl + 3.0 * shows(pw, foot) * cellRand(bu + 31.0, bh + 3.0) + 5.0 * shows(0.5, foot) * noise(u / 0.5 + 1.7, h / 0.5 + 9.2) +
		3.0 * shows(2.0, foot) * noise(u / 2.0 + 4.4, h / 2.0));
	uint c = shade32(mix32(base, colFelt, 0.7), d);
	float e = min(eu, eh);
	c = mix32(c, 0u, 0.2 * shows(1.2, foot) * (1.0 - clamp01(e / 1.2)));
	return mix32(c, 0u, 0.5 * seam(e, gap, foot));
}

// wallFinish
uint wallFinish(uint base, uint text, float u, float y, float foot, float pl) {
	float h = y - rmFy;
	if (wallFin == 1) return brickAt(base, u, h, foot, pl);
	if (wallFin == 2) return panelsAt(base, u, h, foot, pl);
	if (wallFin == 3) return concreteWallAt(base, u, h, foot, pl);
	if (wallFin == 4) return tilesAt(base, u, h, foot, pl);
	if (wallFin == 5) return paperWall(base, text, u, y, foot, pl, true);
	if (wallFin == 6) return feltAt(base, u, h, foot, pl);
	return paperWall(base, text, u, y, foot, pl, false);
}

// parquetAt
uint parquetAt(uint base, float x, float z, float foot) {
	const float w = 2.5, n = 4.0;
	const float k = 0.70710678 / w;
	float a = (x + z) * k, b = (z - x) * k;
	float along, across, id1, id2, tone;
	float row = floor(b);
	if (gmod(a - row, 2.0 * n) < n) {
		id1 = row;
		id2 = floor((a - row) / (2.0 * n));
		along = a - row - 2.0 * n * id2;
		across = b - row;
		tone = 3.0;
	} else {
		float cl = floor(a);
		id1 = cl;
		id2 = floor((b - cl - 1.0) / (2.0 * n));
		along = b - cl - 1.0 - 2.0 * n * id2;
		across = a - cl;
		tone = -3.0;
	}
	float ea = min(along, n - along) * w, ec = min(across, 1.0 - across) * w;
	float vis = shows(w, foot);
	float g = noise(along * w / 9.0 + id1 * 3.1, across * w / 0.4 + id2 * 7.3);
	float d = vis * (tone + 8.0 * cellRand(id1 + 41.0, id2 + 13.0)) + 6.0 * shows(0.4, foot) * g;
	uint c = shade32(mix32(base, colParquet, 0.5), d);
	return mix32(c, 0u, 0.3 * seam(min(ea, ec), 0.1, foot));
}

// carpetAt
uint carpetAt(uint base, float x, float z, float foot) {
	const float tw = 12.0;
	float chk = 0.5 + shows(tw, foot) * (filteredChecker(x / tw, z / tw, foot / tw) - 0.5);
	float fuzz = 5.0 * shows(0.4, foot) * noise(x / 0.4 + 2.2, z / 0.4 + 6.6) + 3.0 * shows(1.5, foot) * noise(x / 1.5 + 8.8, z / 1.5);
	uint c = shade32(mix32(base, colCarpet, 0.6), 6.0 * (chk - 0.5) + fuzz);
	float ex = min(gmod(x, tw), tw - gmod(x, tw)), ez = min(gmod(z, tw), tw - gmod(z, tw));
	return mix32(c, 0u, 0.12 * seam(min(ex, ez), 0.06, foot));
}

// concreteFloorAt
uint concreteFloorAt(uint base, float x, float z, float foot) {
	float cloud = 7.0 * noise(x / 26.0 + 1.3, z / 26.0 + 4.9) + 4.0 * shows(8.0, foot) * noise(x / 8.0 + 6.1, z / 8.0 + 2.2);
	float speck = 6.0 * shows(0.5, foot) * noise(x / 0.5 + 3.9, z / 0.5 + 7.4);
	uint c = shade32(mix32(base, colConcr, 0.45), cloud + speck);
	float ex = min(gmod(x, 40.0), 40.0 - gmod(x, 40.0)), ez = min(gmod(z, 40.0), 40.0 - gmod(z, 40.0));
	return mix32(c, 0u, 0.3 * seam(min(ex, ez), 0.1, foot));
}

// marbleAt
uint marbleAt(uint base, float x, float z, float foot) {
	const float tw = 8.0;
	float chk = 0.5 + shows(tw, foot) * (filteredChecker(x / tw, z / tw, foot / tw) - 0.5);
	uint c = mix32(mix32(base, colMarbleL, 0.6), mix32(base, colMarbleD, 0.6), chk);
	float ti = floor(x / tw), tj = floor(z / tw);
	float ph = (x + 0.6 * z) / 3.2 + 3.0 * noiseSmooth(x / 9.0 + 11.3, z / 9.0 + 4.1) + 1.2 * noiseSmooth(x / 3.0 + 2.7, z / 3.0 + 9.9) + 9.0 * cellRand(ti + 71.0, tj + 23.0);
	float vein = shows(0.6, foot) * (1.0 - clamp01(abs(sin(ph)) / 0.12));
	c = shade32(c, 4.0 * noise(x / 3.0 + 5.5, z / 3.0 + 1.1) + vein * (28.0 * chk - 14.0));
	float ex = min(gmod(x, tw), tw - gmod(x, tw)), ez = min(gmod(z, tw), tw - gmod(z, tw));
	return mix32(c, mix32(base, colMortar, 0.4), seam(min(ex, ez), 0.06, foot));
}

// oakAt
uint oakAt(uint base, float x, float z, float foot) {
	const float bw = 9.0, bl = 60.0;
	float p = floor(x / bw);
	float along = z + gmod(p * 23.0, bl);
	float q = floor(along / bl);
	float fx = x - p * bw, fz = along - q * bl;
	float g = 6.0 * shows(0.5, foot) * noise(x / 0.5 + p * 7.7, z / 12.0) + 4.0 * shows(1.5, foot) * noise(x / 1.5 + 3.1, z / 25.0 + p);
	uint c = shade32(mix32(base, colOak, 0.55), 6.0 * shows(bw, foot) * cellRand(p + 53.0, q) + g);
	float sx = seam(min(fx, bw - fx), 0.1, foot), sz = seam(min(fz, bl - fz), 0.1, foot);
	return mix32(c, 0u, 0.22 * (sx + sz - sx * sz));
}

// slateAt
uint slateAt(uint base, float x, float z, float foot) {
	const float tw = 10.0;
	float row = floor(z / tw);
	float xx = x + 0.5 * tw * gmod(row, 2.0);
	float col = floor(xx / tw);
	float vis = shows(tw, foot);
	uint c = mix32(base, colSlate, 0.75);
	c = mix32(c, 0x4a5a48u, 0.25 * vis * (cellRand(col + 61.0, row + 17.0) + 1.0) / 2.0);
	float riven = 5.0 * shows(3.0, foot) * noise(x / 3.0 + 7.2, z / 3.0 + 0.6) + 4.0 * shows(1.0, foot) * noise(x + 3.3, z + 5.5);
	c = shade32(c, 9.0 * vis * cellRand(col + 3.0, row + 29.0) + riven);
	float ex = min(xx - col * tw, tw - (xx - col * tw)), ez = min(z - row * tw, tw - (z - row * tw));
	float gx = seam(ex, 0.15, foot), gz = seam(ez, 0.15, foot);
	return mix32(c, mix32(base, 0x1c1f22u, 0.8), gx + gz - gx * gz);
}

// floorFinish
uint floorFinish(uint base, float x, float z, float pxs, float foot) {
	if (floorFin == 1) return parquetAt(base, x, z, foot);
	if (floorFin == 2) return carpetAt(base, x, z, foot);
	if (floorFin == 3) return concreteFloorAt(base, x, z, foot);
	if (floorFin == 4) return marbleAt(base, x, z, foot);
	if (floorFin == 5) return oakAt(base, x, z, foot);
	if (floorFin == 6) return slateAt(base, x, z, foot);
	return floorAt(base, x, z, pxs);
}
`
