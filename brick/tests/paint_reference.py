"""A pure-Python painter of the paint.py contract, doing the same sums as the machine code.

Used by tests/test_paint.py: the machine-code painter and this one must give the same pixels.
Pixels are 32-bit XRGB values in a list (row after row, `width` a row; `stride` in pixels).
A pixel is written only when what is painted over it is at least a little opaque (alpha > 0),
and every written pixel has 0 in its top byte, so the tests can see exactly which pixels were touched.
"""
import math
import os
import sys

sys.path.insert(0, os.path.join(os.path.dirname(os.path.abspath(__file__)), ".."))
import font  # noqa: E402

MAXW = 16384                 # widest/tallest area painted (row buffers in the machine code)
LIM = 1 << 28                # positions are kept within +-LIM, sizes within 2 * LIM
SHADOW_ALPHA = 110           # how dark the middle of a shadow is (0..255)


def div255(x):
    t = x + 128
    return (t + (t >> 8)) >> 8


def clampi(v, lo=0, hi=255):
    return lo if v < lo else hi if v > hi else v


def mix(c1, c2, a):
    """c1 with c2 over it at alpha a (0..255), per channel."""
    out = 0
    for shift in (0, 8, 16):
        x, y = (c1 >> shift) & 255, (c2 >> shift) & 255
        out |= div255(x * (255 - a) + y * a) << shift
    return out


def lerp(c1, c2, n, d):
    if d <= 0:
        return c1 & 0xFFFFFF
    out = 0
    for shift in (0, 8, 16):
        x, y = (c1 >> shift) & 255, (c2 >> shift) & 255
        out |= ((x * (d - n) + y * n + d // 2) // d) << shift
    return out


def grain_hash(u, v):
    h = (u * 374761393 + v * 668265263) & 0xFFFFFFFF
    h = ((h ^ (h >> 13)) * 1274126177) & 0xFFFFFFFF
    return h ^ (h >> 16)


def texture_mark(tex, s, u, v):
    """How strongly the texture colour covers the pixel at (u, v) inside the element: 0..255."""
    lw = max(1, s // 8)
    if tex == 1:
        return 255 if (u + v) % s < max(1, s // 3) else 0
    if tex == 2:
        return 255 if v % s < lw else 0
    if tex == 3:
        return 255 if u % s < lw else 0
    if tex == 4:
        du, dv = 2 * (u % s) + 1 - s, 2 * (v % s) + 1 - s
        return 255 if du * du + dv * dv <= (s * s) // 4 else 0
    if tex == 5:
        return 255 if ((u // s) + (v // s)) & 1 else 0
    if tex == 6:
        g = max(1, s // 8)
        return grain_hash(u // g, v // g) & 127
    if tex == 7:
        if v % s < lw:
            return 255
        return 255 if (u + ((v // s) & 1) * s) % (2 * s) < lw else 0
    if tex == 8:
        return 255 if (u % s < lw or v % s < lw) else 0
    return 0


class Painter:
    def __init__(self, width, height, stride=None, fill=0xABABABAB):
        self.width, self.height = width, height
        self.stride = stride or width
        self.pixels = [fill] * (self.stride * height)
        self.clip = (0, 0, 0, 0)
        self.clip_set = False

    # ---- clip ----

    def clip_all(self):
        self.clip = (0, 0, self.width, self.height)
        self.clip_set = True

    def set_clip(self, x0, y0, x1, y1):
        self.clip = (max(x0, 0), max(y0, 0), min(x1, self.width), min(y1, self.height))
        self.clip_set = True

    def eclip(self):
        x0, y0, x1, y1 = self.clip if self.clip_set else (0, 0, self.width, self.height)
        return max(x0, 0), max(y0, 0), min(x1, self.width, MAXW), min(y1, self.height, MAXW)

    # ---- pixels ----

    def blend(self, px, py, colour, a):
        if a <= 0:
            return
        i = py * self.stride + px
        if a >= 255:
            self.pixels[i] = colour & 0xFFFFFF
        else:
            self.pixels[i] = mix(self.pixels[i] & 0xFFFFFF, colour, a)

    # ---- boxes and ellipses ----

    def box(self, x, y, w, h, style, ellipse=False):
        st = list(style) + [0] * (16 - len(style))
        if w <= 0 or h <= 0:
            return
        x, y = clampi(x, -LIM, LIM), clampi(y, -LIM, LIM)
        w, h = min(w, 2 * LIM), min(h, 2 * LIM)
        fill, c1, c2, direction, tex, tcol, tsize, transparency, radius, bw, bcol, shadow = st[:12]
        op = 255 - clampi(transparency)
        if op == 0:
            return
        ex0, ey0, ex1, ey1 = self.eclip()
        if shadow > 0:
            self.shadow(x, y, w, h, min(shadow, 1024), radius, op, ellipse, (ex0, ey0, ex1, ey1))
        cxa, cxb = max(x, ex0), min(x + w, ex1)
        rya, ryb = max(y, ey0), min(y + h, ey1)
        if cxa >= cxb or rya >= ryb:
            return
        tsize = 8 if tsize <= 0 else clampi(tsize, 2, 4096)
        if not 1 <= tex <= 8:
            tex = 0
        bw = max(bw, 0)
        bcol &= 0xFFFFFF
        tcol &= 0xFFFFFF
        af = 255 if fill in (1, 2, 3) else 0
        r = min(max(radius, 0), w // 2, h // 2)
        if fill == 2 and not 0 <= direction <= 5:
            direction = 0
        rtab = [lerp(c1, c2, i, 255) for i in range(256)]
        rcx, rcy = float(2 * x + w) * 0.5, float(2 * y + h) * 0.5
        irx, iry = 2.0 / float(w), 2.0 / float(h)
        if ellipse:
            ea, eb = float(w) * 0.5, float(h) * 0.5
            ia, ib = 1.0 / ea, 1.0 / eb
            ia2, ib2 = ia * ia, ib * ib
            big, small = max(ea, eb), min(ea, eb)
            k = (float(bw) + 1.0) * big / small
            ai, bi = ea - k, eb - k
        bwf = float(bw)
        for py in range(rya, ryb):
            v = py - y
            row_c, row_a = {}, {}
            for px in range(cxa, cxb):
                u = px - x
                if fill == 1:
                    c = c1 & 0xFFFFFF
                elif fill == 2:
                    n, d = [(v, h - 1), (h - 1 - v, h - 1), (u, w - 1), (w - 1 - u, w - 1),
                            (u + v, w + h - 2), (u + h - 1 - v, w + h - 2)][direction]
                    c = lerp(c1, c2, n, d)
                elif fill == 3:
                    dy = ((float(py) + 0.5) - rcy) * iry
                    dx = ((float(px) + 0.5) - rcx) * irx
                    t = int(math.sqrt(dx * dx + dy * dy) * 255.0)
                    c = rtab[min(t, 255)]
                else:
                    c = 0
                a = af
                if tex:
                    m = texture_mark(tex, tsize, u, v)
                    if af:
                        c = mix(c, tcol, m)
                    else:
                        c, a = tcol, m
                row_c[px], row_a[px] = c, a
            # the shape: which pixels, border, soft edges
            if ellipse:
                ey = (float(py) + 0.5) - rcy
                yy0, yy1 = ey * ib, ey * ib2
                yy0, yy1 = yy0 * yy0, yy1 * yy1
                pia = pib = cxa
                if ai > 0.0 and bi > 0.0 and ey * ey < bi * bi:
                    t = ey / bi
                    hi = ai * math.sqrt(1.0 - t * t)
                    pia = clampi(math.ceil((rcx - hi) - 0.5), cxa, cxb)
                    pib = clampi(math.floor((rcx + hi) - 0.5) + 1, pia, cxb)
                for px in list(range(cxa, pia)) + list(range(pib, cxb)):
                    ex = (float(px) + 0.5) - rcx
                    k0 = math.sqrt((ex * ia) * (ex * ia) + yy0)
                    k1 = math.sqrt((ex * ia2) * (ex * ia2) + yy1)
                    d = (k0 * (k0 - 1.0)) / k1 if k1 > 0.0 else 0.0 - small
                    self.edge_pixel(row_c, row_a, px, d, bw, bwf, bcol)
            else:
                corner_row = v < r or v >= h - r
                for px in range(cxa, cxb):
                    u = px - x
                    if corner_row and (u < r or u >= w - r):
                        ccx = x + r if u < r else x + w - r
                        ccy = y + r if v < r else y + h - r
                        dx = (float(px) + 0.5) - float(ccx)
                        dy = (float(py) + 0.5) - float(ccy)
                        d = math.sqrt(dx * dx + dy * dy) - float(r)
                        self.edge_pixel(row_c, row_a, px, d, bw, bwf, bcol)
                    elif bw > 0 and (u < bw or u >= w - bw or v < bw or v >= h - bw):
                        row_c[px], row_a[px] = bcol, 255
            for px in range(cxa, cxb):
                self.blend(px, py, row_c[px], div255(row_a[px] * op))

    def edge_pixel(self, row_c, row_a, px, d, bw, bwf, bcol):
        e = 0.5 - d
        cov = clampi(int(e * 255.0 + 0.5))
        b = 255 - clampi(int((e - bwf) * 255.0 + 0.5)) if bw > 0 else 0
        c, a = row_c[px], row_a[px]
        if b == 255:
            c, a = bcol, 255
        elif b > 0:
            wa = div255(a * (255 - b))
            a2 = b + wa
            out = 0
            for shift in (0, 8, 16):
                out |= (((bcol >> shift) & 255) * b + ((c >> shift) & 255) * wa + a2 // 2) // a2 << shift
            c, a = out, a2
        row_c[px], row_a[px] = c, div255(a * cov)

    def shadow(self, x, y, w, h, s, radius, op, ellipse, clip):
        ex0, ey0, ex1, ey1 = clip
        sx, sy = x + s, y + s
        xa, xb = max(sx - s, ex0), min(sx + w + s, ex1)
        ya, yb = max(sy - s, ey0), min(sy + h + s, ey1)
        r = float(min(max(radius, 0), w // 2, h // 2))
        scx, scy = float(2 * sx + w) * 0.5, float(2 * sy + h) * 0.5
        hw, hh = float(w) * 0.5, float(h) * 0.5
        half, inv = float(s), 1.0 / float(2 * s)
        sa = float(SHADOW_ALPHA * op) / 255.0
        ia, ib = 1.0 / hw, 1.0 / hh
        ia2, ib2 = ia * ia, ib * ib
        for py in range(ya, yb):
            ey = (float(py) + 0.5) - scy
            qy = abs(ey) - (hh - r)
            yy0, yy1 = ey * ib, ey * ib2
            yy0, yy1 = yy0 * yy0, yy1 * yy1
            for px in range(xa, xb):
                ex = (float(px) + 0.5) - scx
                if ellipse:
                    k0 = math.sqrt((ex * ia) * (ex * ia) + yy0)
                    k1 = math.sqrt((ex * ia2) * (ex * ia2) + yy1)
                    d = (k0 * (k0 - 1.0)) / k1 if k1 > 0.0 else 0.0 - min(hw, hh)
                else:
                    qx = abs(ex) - (hw - r)
                    mx, my = max(qx, 0.0), max(qy, 0.0)
                    d = math.sqrt(mx * mx + my * my) + min(max(qx, qy), 0.0) - r
                t = min(max((half - d) * inv, 0.0), 1.0)
                f = t * t * (3.0 - 2.0 * t)
                a = int(f * sa + 0.5)
                self.blend(px, py, 0, a)

    def ellipse(self, x, y, w, h, style):
        self.box(x, y, w, h, style, ellipse=True)

    # ---- lines and triangles ----

    def line(self, x1, y1, x2, y2, colour, thickness, transparency):
        op = 255 - clampi(transparency)
        if op == 0:
            return
        x1, y1, x2, y2 = (clampi(n, -LIM, LIM) for n in (x1, y1, x2, y2))
        t = 1 if thickness <= 0 else min(thickness, 4096)
        rr = float(t) * 0.5 + 0.5
        vx, vy = float(x2 - x1), float(y2 - y1)
        l2 = vx * vx + vy * vy
        inv = 1.0 / l2 if l2 > 0.0 else 0.0
        ex0, ey0, ex1, ey1 = self.eclip()
        m = t // 2 + 2
        for py in range(max(min(y1, y2) - m, ey0), min(max(y1, y2) + m + 1, ey1)):
            for px in range(max(min(x1, x2) - m, ex0), min(max(x1, x2) + m + 1, ex1)):
                fx, fy = float(px - x1), float(py - y1)
                tt = min(max((fx * vx + fy * vy) * inv, 0.0), 1.0)
                qx, qy = fx - tt * vx, fy - tt * vy
                d = math.sqrt(qx * qx + qy * qy)
                cov = clampi(int((rr - d) * 255.0 + 0.5))
                self.blend(px, py, colour, div255(cov * op))

    def triangle(self, x1, y1, x2, y2, x3, y3, colour, transparency):
        op = 255 - clampi(transparency)
        if op == 0:
            return
        x1, y1, x2, y2, x3, y3 = (clampi(n, -LIM, LIM) for n in (x1, y1, x2, y2, x3, y3))
        area = (x2 - x1) * (y3 - y1) - (y2 - y1) * (x3 - x1)
        if area == 0:
            return
        if area < 0:
            x2, y2, x3, y3 = x3, y3, x2, y2
        edges = [(x1, y1, x2, y2), (x2, y2, x3, y3), (x3, y3, x1, y1)]
        ex0, ey0, ex1, ey1 = self.eclip()
        for py in range(max(min(y1, y2, y3), ey0), min(max(y1, y2, y3), ey1)):
            lo, hi = ex0, ex1
            for ax, ay, bx, by in edges:
                topleft = (ay == by and bx > ax) or by < ay
                a = -4 * (by - ay)
                c = 2 * (bx - ax) * (2 * py + 1 - 2 * ay) - 2 * (by - ay) * (1 - 2 * ax) - (0 if topleft else 1)
                if a > 0:
                    lo = max(lo, -((c) // a))        # px >= ceil(-c / a)
                elif a < 0:
                    hi = min(hi, c // (-a) + 1)       # px <= floor(c / -a)
                elif c < 0:
                    hi = lo
            for px in range(lo, hi):
                self.blend(px, py, colour, op)

    # ---- text ----

    def text_font(self, s, sizes):
        b = font.pick(s, sizes)
        return b, font.baked(b)

    def measure(self, text, s, sizes=None):
        s = 16 if s <= 0 else min(s, 1024)
        b, data = self.text_font(s, sizes or font.DEFAULT_SIZES)
        pen = sum(data[8 + 8 * g] for g in glyphs(text))
        lh = int.from_bytes(data[2:4], "little")
        return (pen * s + b // 2) // b, (lh * s + b // 2) // b

    def text(self, x, y, text, colour, s, transparency, sizes=None):
        s = 16 if s <= 0 else min(s, 1024)
        x, y = clampi(x, -LIM, LIM), clampi(y, -LIM, LIM)
        b, data = self.text_font(s, sizes or font.DEFAULT_SIZES)
        op = 255 - clampi(transparency)
        step = (b << 16) // s
        ex0, ey0, ex1, ey1 = self.eclip()
        pen = 0
        for g in glyphs(text):
            adv, gx, gy, gw, gh = data[8 + 8 * g:13 + 8 * g]
            gx, gy = gx - 256 if gx > 127 else gx, gy - 256 if gy > 127 else gy
            off = 776 + int.from_bytes(data[14 + 8 * g:16 + 8 * g], "little")
            if gw and gh and op:
                pad = [[0] * (gw + 2) for _ in range(gh + 2)]
                for i in range(gw * gh):
                    nib = (data[off + i // 2] >> (4 * (i & 1))) & 15
                    pad[i // gw + 1][i % gw + 1] = nib * 17
                ox, oy = pen + gx, gy
                xa = ((ox - 1) * s) // b - 1
                xb = ((ox + gw + 1) * s) // b + 2
                ya = ((oy - 1) * s) // b - 1
                yb = ((oy + gh + 1) * s) // b + 2
                for yy in range(ya, yb):
                    py = y + yy
                    if not ey0 <= py < ey1:
                        continue
                    lv = yy * step + (step >> 1) - 32768 - (oy << 16)
                    iv, fv = lv >> 16, (lv >> 8) & 255
                    if iv < -1 or iv > gh - 1:
                        continue
                    for xx in range(xa, xb):
                        px = x + xx
                        if not ex0 <= px < ex1:
                            continue
                        lu = xx * step + (step >> 1) - 32768 - (ox << 16)
                        iu, fu = lu >> 16, (lu >> 8) & 255
                        if iu < -1 or iu > gw - 1:
                            continue
                        r0, r1 = pad[iv + 1], pad[iv + 2]
                        top = r0[iu + 1] * (256 - fu) + r0[iu + 2] * fu
                        bot = r1[iu + 1] * (256 - fu) + r1[iu + 2] * fu
                        cov = (top * (256 - fv) + bot * fv + 32768) >> 16
                        if cov:
                            self.blend(px, py, colour, div255(cov * op))
            pen += adv
        return (pen * s + b // 2) // b


def glyphs(text):
    """Glyph numbers for the bytes of a text (UTF-8): 32..126 as themselves - 32, anything else the box (95)."""
    raw = text.encode() if isinstance(text, str) else text
    out = []
    for byte in raw:
        if 0x80 <= byte < 0xC0:
            continue
        out.append(byte - 32 if 32 <= byte <= 126 else 95)
    return out


def png(path, pixels, width, height, stride=None):
    """Save XRGB pixels as a PNG (standard library only)."""
    import struct
    import zlib
    stride = stride or width
    rows = bytearray()
    for y in range(height):
        rows.append(0)
        for x in range(width):
            p = pixels[y * stride + x]
            rows += bytes(((p >> 16) & 255, (p >> 8) & 255, p & 255))

    def chunk(kind, body):
        return struct.pack(">I", len(body)) + kind + body + struct.pack(">I", zlib.crc32(kind + body))
    with open(path, "wb") as f:
        f.write(b"\x89PNG\r\n\x1a\n" + chunk(b"IHDR", struct.pack(">IIBBBBB", width, height, 8, 2, 0, 0, 0))
                + chunk(b"IDAT", zlib.compress(bytes(rows), 6)) + chunk(b"IEND", b""))
