"""Tests for paint.py: machine-code painting compared pixel for pixel with tests/paint_reference.py.

Each scene becomes a tiny program (runtime + paint, no libc) that fills a buffer with 0xAB bytes,
points gui_target at the middle of it (with padding at the end of each row and guard bytes around it),
paints, and writes the whole buffer to stdout. The Python reference paints the same scene.
They must match exactly, and the guard bytes and row padding must be untouched.

Usage: python3 tests/test_paint.py              (BRICK_PAINT_PNG=folder also saves showcase pictures)
       python3 tests/test_paint.py --time       (also measures speed)
"""
import os
import struct
import subprocess
import sys
import tempfile
import time

HERE = os.path.dirname(os.path.abspath(__file__))
sys.path.insert(0, os.path.join(HERE, ".."))
sys.path.insert(0, HERE)
os.environ.setdefault("BRICK_MEMORY", "off")
os.environ.setdefault("BRICK_TRAINER", "off")

import elf  # noqa: E402
import font  # noqa: E402
import paint  # noqa: E402
import runtime  # noqa: E402
from asm import RAX, RCX, RDI, RDX, RSI, Asm, NOT_EQUAL, GREATER  # noqa: E402
from paint_reference import Painter, png  # noqa: E402

GUARD = 4096
RESULTS = 256             # answers kept (from paint_text and paint_measure)
FOLDER = tempfile.mkdtemp(prefix="brick-paint-")


def style(fill=0, c1=0, c2=0, direction=0, tex=0, tcol=0, tsize=0, transparency=0, radius=0, border=0,
          bcol=0, shadow=0):
    return [fill, c1, c2, direction, tex, tcol, tsize, transparency, radius, border, bcol, shadow, 0, 0, 0, 0]


class Scene:
    """A list of painting steps, run both ways."""

    def __init__(self, width, height, pad=3, sizes=None):
        self.width, self.height, self.pad = width, height, pad
        self.stride = width + pad
        self.steps = []
        self.sizes = sizes

    def __getattr__(self, name):
        if name in ("clip_all", "clip", "box", "ellipse", "text", "measure", "line", "triangle", "repeat"):
            return lambda *args: self.steps.append((name,) + args)
        raise AttributeError(name)

    # ---- the machine-code way ----

    def program(self, repeat=1):
        a = Asm()
        runtime.start(a)
        total = GUARD * 2 + self.stride * self.height * 4
        results = a.reserve(8 * RESULTS, "results")
        a.set(RCX, total)
        a.call("alloc")
        a.address(RSI, "scene_buffer")
        a.store(RSI, 0, RAX)
        a.mov(RDI, RAX)
        a.set(RCX, total)
        a.set(RAX, 0xAB)
        a.fill_bytes()
        a.address(RSI, "scene_buffer")
        a.load(RAX, RSI, 0)
        a.op_n("add", RAX, GUARD)
        a.address(RDI, "gui_target")
        a.store(RDI, 0, RAX)
        for offset, value in ((8, self.width), (16, self.height), (24, self.stride * 4)):
            a.set(RAX, value)
            a.store(RDI, offset, RAX)
        a.space["scene_buffer"] = 8
        a.space["scene_count"] = 8
        a.set(RAX, repeat)
        a.address(RDI, "scene_count")
        a.store(RDI, 0, RAX)
        again = a.new_label("again")
        a.label(again)
        result = 0
        for step in self.steps:
            kind, args = step[0], step[1:]
            if kind == "clip_all":
                a.call("paint_clip_all")
            elif kind == "measure":
                assert result + 2 <= RESULTS
                a.address(RAX, a.text(args[0]))
                a.set(RCX, args[1])
                a.call("paint_measure")
                a.address(RDI, results)
                a.store(RDI, 8 * result, RAX)
                a.store(RDI, 8 * result + 8, RCX)
                result += 2
            else:
                fields = list(args)
                if kind in ("box", "ellipse"):
                    fields = fields[:4] + [("style", fields[4])]
                if kind == "text":
                    fields[2] = ("text", fields[2])
                block = self.block(a, fields)
                a.address(RAX, block)
                a.call("paint_" + kind)
                if kind == "text" and result < RESULTS:
                    a.address(RDI, results)
                    a.store(RDI, 8 * result, RAX)
                    result += 1
        a.address(RDI, "scene_count")
        a.load(RAX, RDI, 0)
        a.op_n("sub", RAX, 1)
        a.store(RDI, 0, RAX)
        a.jump_if(NOT_EQUAL, again)
        # write the buffer, then the results
        a.address(RSI, "scene_buffer")
        a.load(RSI, RSI, 0)
        a.set(RDX, total)
        self.write_all(a)
        a.address(RSI, results)
        a.set(RDX, 8 * RESULTS)
        self.write_all(a)
        runtime.finish(a)
        runtime.add_all(a)
        paint.add_all(a, self.sizes)
        return elf.wrap(a)

    @staticmethod
    def write_all(a):
        """write(1, rsi, rdx) until all is written."""
        loop = a.new_label("write")
        a.label(loop)
        a.push(RSI)
        a.push(RDX)
        a.set(RAX, 1)
        a.set(RDI, 1)
        a.syscall()
        a.pop(RDX)
        a.pop(RSI)
        a.op("add", RSI, RAX)
        a.op("sub", RDX, RAX)
        a.op_n("cmp", RDX, 0)
        a.jump_if(GREATER, loop)

    @staticmethod
    def block(a, fields):
        label = a.reserve(8 * len(fields), "block")
        a.address(RDI, label)
        for i, field in enumerate(fields):
            if isinstance(field, tuple) and field[0] == "style":
                st = a.new_label("style")
                a.data[st] = struct.pack("<16q", *field[1])
                a.address(RAX, st)
            elif isinstance(field, tuple) and field[0] == "text":
                a.address(RAX, a.text(field[1]))
            else:
                a.set(RAX, field)
            a.address(RDI, label)
            a.store(RDI, 8 * i, RAX)
        return label

    def run_machine(self):
        path = os.path.join(FOLDER, "scene")
        with open(path, "wb") as f:
            f.write(self.program())
        os.chmod(path, 0o755)
        out = subprocess.run([path], capture_output=True, timeout=60)
        assert out.returncode == 0, f"the program ended with {out.returncode}"
        total = GUARD * 2 + self.stride * self.height * 4
        assert len(out.stdout) == total + 8 * RESULTS, len(out.stdout)
        buffer, results = out.stdout[:total], out.stdout[total:]
        assert buffer[:GUARD] == b"\xab" * GUARD, "wrote before the buffer"
        assert buffer[-GUARD:] == b"\xab" * GUARD, "wrote after the buffer"
        pixels = list(struct.unpack(f"<{self.stride * self.height}I", buffer[GUARD:-GUARD]))
        return pixels, list(struct.unpack(f"<{RESULTS}q", results))

    # ---- the reference way ----

    def run_reference(self):
        p = Painter(self.width, self.height, self.stride)
        results = []
        sizes = font.sizes_for(self.sizes)
        for step in self.steps:
            kind, args = step[0], step[1:]
            if kind == "clip_all":
                p.clip_all()
            elif kind == "clip":
                p.set_clip(*args)
            elif kind == "box":
                p.box(*args)
            elif kind == "ellipse":
                p.ellipse(*args)
            elif kind == "line":
                p.line(*args)
            elif kind == "triangle":
                p.triangle(*args)
            elif kind == "text":
                results.append(p.text(*args, sizes=sizes))
            elif kind == "measure":
                results.extend(p.measure(args[0], args[1], sizes))
        return p.pixels, results

    def check(self, name=None):
        mine, my_results = self.run_machine()
        ref, ref_results = self.run_reference()
        if name and os.environ.get("BRICK_PAINT_PNG"):
            png(os.path.join(os.environ["BRICK_PAINT_PNG"], name + ".png"), mine, self.width, self.height, self.stride)
            png(os.path.join(os.environ["BRICK_PAINT_PNG"], name + "-ref.png"), ref, self.width, self.height,
                self.stride)
        wrong = [i for i in range(len(ref)) if mine[i] != ref[i]]
        if wrong:
            shown = ", ".join(f"({i % self.stride},{i // self.stride}) got {mine[i]:08x} want {ref[i]:08x}"
                              for i in wrong[:6])
            raise AssertionError(f"{len(wrong)} pixels differ: {shown}")
        assert my_results[:len(ref_results)] == ref_results, (my_results[:len(ref_results)], ref_results)
        for y in range(self.height):                 # row padding untouched
            for x in range(self.width, self.stride):
                assert mine[y * self.stride + x] == 0xABABABAB, f"wrote in the row padding at ({x},{y})"
        return mine


BG = style(1, 0x203040)


def test_solid_and_clip_never_set():
    s = Scene(64, 40)
    s.box(-5, -5, 100, 100, style(1, 0x336699))      # no clip set yet: the whole target
    s.box(10, 10, 20, 10, style(1, 0xFFEEDD))
    s.check()


def test_linear_gradients_all_directions():
    s = Scene(160, 120)
    s.clip_all()
    for d in range(6):
        s.box(5 + (d % 3) * 52, 5 + (d // 3) * 58, 48, 52, style(2, 0xFF0000, 0x0000FF, d))
    s.box(-30, 100, 300, 50, style(2, 0x00FF00, 0xFF00FF, 4))   # off the edges: the table follows the clip
    s.box(100, -40, 80, 70, style(2, 0x123456, 0xFEDCBA, 5))
    s.box(70, 70, 1, 1, style(2, 0xFFFFFF, 0, 4))                # one pixel: d = 0
    s.box(80, 70, 1, 30, style(2, 0xFFFFFF, 0, 2))
    s.check("gradients")


def test_radial_gradient():
    s = Scene(140, 100)
    s.clip_all()
    s.box(5, 5, 90, 60, style(3, 0xFFFFC0, 0x802000))
    s.box(80, 40, 100, 100, style(3, 0x00FFFF, 0x000040, radius=30))
    s.check("radial")


def test_textures():
    s = Scene(260, 140)
    s.clip_all()
    s.box(0, 0, 260, 140, BG)
    for t in range(1, 9):
        x, y = 4 + ((t - 1) % 4) * 64, 4 + ((t - 1) // 4) * 68
        s.box(x, y, 60, 30, style(1, 0x3060A0, tex=t, tcol=0xE0F0FF))
        s.box(x + 3, y + 33, 57, 31, style(0, tex=t, tcol=0xFFC040, tsize=5 + t * 2))   # marks with no fill
    s.check("textures")


def test_texture_sizes_and_offsets():
    s = Scene(120, 90)
    s.clip(7, 3, 113, 80)
    for i, (t, size) in enumerate([(1, 3), (4, 2), (5, 1), (6, 40), (7, 13), (8, 4096), (6, 0), (9, 8), (-1, 8)]):
        s.box(-17 + i * 15, -9 + i * 11, 37, 41, style(2, 0x101010, 0xF0F0F0, i % 6, tex=t, tcol=0xFF0000,
                                                      tsize=size))
    s.check()


def test_corners_and_borders():
    s = Scene(220, 130)
    s.clip_all()
    s.box(0, 0, 220, 130, style(1, 0xF0F0F0))
    s.box(5, 5, 60, 40, style(1, 0x4080C0, radius=12))
    s.box(70, 5, 60, 40, style(1, 0x4080C0, radius=12, border=3, bcol=0x102030))
    s.box(135, 5, 60, 40, style(0, radius=15, border=2, bcol=0xC03030))          # outline only
    s.box(5, 50, 41, 41, style(1, 0x30A050, radius=100, border=5, bcol=0xFFFFFF))   # round: radius cut to 20
    s.box(50, 50, 60, 40, style(1, 0x808000, radius=4, border=9, bcol=0x000080))    # border wider than corner
    s.box(115, 50, 30, 20, style(1, 0x804000, border=40, bcol=0x00FFFF))           # all border
    s.box(150, 50, 61, 70, style(2, 0xFFFFFF, 0x000000, 4, tex=1, tcol=0xFF8000, radius=20, border=4,
                                 bcol=0x202020, transparency=60))
    s.box(5, 95, 100, 30, style(0, tex=4, tcol=0x0000FF, radius=10, border=1, bcol=0x0000FF))   # marks + border
    s.check("corners")


def test_shadows():
    s = Scene(200, 140)
    s.clip_all()
    s.box(0, 0, 200, 140, style(1, 0xE8E4DC))
    s.box(10, 10, 70, 40, style(1, 0xFFFFFF, radius=8, shadow=6))
    s.box(100, 10, 70, 50, style(1, 0xFFFFFF, shadow=12, transparency=100))
    s.ellipse(10, 70, 80, 50, style(3, 0xFFFFFF, 0x4080FF, shadow=8))
    s.box(170, 100, 50, 60, style(1, 0x806040, radius=10, shadow=20))           # shadow past the edges
    s.box(-70, 120, 60, 10, style(1, 0x806040, shadow=30))                      # box outside, shadow inside
    s.check("shadows")


def test_ellipses():
    s = Scene(220, 120)
    s.clip_all()
    s.box(0, 0, 220, 120, style(1, 0x202020))
    s.ellipse(5, 5, 60, 60, style(1, 0xFF8040))
    s.ellipse(70, 5, 100, 40, style(2, 0xFFFF00, 0x00FF00, 2, border=3, bcol=0xFFFFFF))
    s.ellipse(175, 5, 21, 80, style(3, 0xFFFFFF, 0x0000FF, border=2, bcol=0xFF0000))
    s.ellipse(5, 70, 120, 45, style(0, tex=5, tcol=0x40C0FF, border=4, bcol=0x40C0FF, transparency=50))
    s.ellipse(130, 50, 3, 3, style(1, 0xFFFFFF))
    s.ellipse(140, 50, 1, 7, style(1, 0xFFFFFF, border=1, bcol=0xFF0000))
    s.ellipse(150, 60, 200, 100, style(1, 0x8080FF, border=6, bcol=0x000000, transparency=128))
    s.ellipse(200, 2, 9, 9, style(1, 0x00FF00, border=10, bcol=0xFF00FF))
    s.check("ellipses")


def test_transparency_stacking():
    s = Scene(120, 90)
    s.clip_all()
    s.box(0, 0, 120, 90, style(2, 0x000000, 0xFFFFFF, 2))
    for i in range(6):
        s.box(5 + i * 12, 5 + i * 9, 50, 40, style(1, [0xFF0000, 0x00FF00, 0x0000FF][i % 3], transparency=40 * i))
    s.ellipse(40, 30, 60, 50, style(1, 0xFFFF00, transparency=200))
    s.box(10, 60, 30, 20, style(1, 0xFFFFFF, transparency=255))        # invisible
    s.box(50, 60, 30, 20, style(1, 0xFFFFFF, transparency=-50))        # below 0 = solid
    s.box(90, 60, 30, 20, style(1, 0xFFFFFF, transparency=999))        # above 255 = invisible
    s.check()


def test_clipping_everywhere():
    s = Scene(100, 80)
    for clip in [(10, 5, 90, 70), (-50, -50, 30, 30), (60, 50, 500, 500), (40, 40, 40, 60), (70, 10, 20, 70)]:
        s.clip(*clip)
        s.box(-20, -20, 140, 120, style(2, 0x102030, 0xA0B0C0, 4, tex=8, tcol=0xFFFFFF, radius=30, border=5,
                                       bcol=0xFF0000, shadow=7))
        s.ellipse(-10, 30, 130, 60, style(3, 0xFFFFFF, 0x000000, border=3, bcol=0x00FF00))
        s.line(-100, -30, 200, 150, 0xFFFF00, 3, 0)
        s.triangle(-40, 90, 50, -30, 140, 85, 0x00FFFF, 128)
        s.text(-7, 30, "Clip me", 0xFFFFFF, 20, 0)
    s.check("clipping")


def test_off_screen_zero_negative_huge():
    s = Scene(60, 50)
    s.clip_all()
    st = style(2, 0xFF0000, 0x00FF00, 3, tex=2, tcol=0, radius=5, border=2, bcol=0xFFFFFF, shadow=4)
    for x, y, w, h in [(10, 10, 0, 20), (10, 10, 20, 0), (10, 10, -5, 20), (10, 10, 20, -5), (100, 10, 20, 20),
                       (-50, 10, 20, 20), (10, -30, 20, 20), (10, 70, 20, 20),
                       (-(1 << 40), -(1 << 40), 1 << 41, 1 << 41), (5, 5, 1 << 62, 10),
                       (-(1 << 62), 30, 1 << 62, 10), (1 << 62, 1 << 62, 1 << 62, 1 << 62)]:
        s.box(x, y, w, h, st)
        s.ellipse(x, y, w, h, st)
    s.line(-(1 << 50), 25, 1 << 50, 26, 0xFFFFFF, 1, 0)
    s.line(30, -(1 << 60), 31, 1 << 60, 0x00FFFF, 2, 0)
    s.line(5, 5, 5, 5, 0xFF00FF, 6, 0)                 # a dot
    s.line(40, 40, 50, 45, 0xFF00FF, -3, 0)            # thickness below 1 = 1
    s.line(40, 10, 55, 20, 0xFF00FF, 1 << 40, 250)     # huge thickness, nearly invisible
    s.triangle(1 << 62, 0, 0, 1 << 62, -(1 << 62), -(1 << 62), 0x808080, 200)
    s.triangle(5, 5, 20, 20, 35, 35, 0xFF0000, 0)      # no area
    s.text(1 << 61, 10, "far", 0xFFFFFF, 16, 0)
    s.text(5, 30, "big", 0xFFFFFF, 1 << 40, 0)
    s.check()


def test_lines():
    s = Scene(160, 120)
    s.clip_all()
    s.box(0, 0, 160, 120, style(1, 0xFAFAFA))
    for i in range(16):
        s.line(80, 60, 80 + [70, 60, 30, 0, -30, -60, -70, -60, -30, 0, 30, 60, 75, -75, 20, -20][i],
               60 + [0, 30, 55, 58, 55, 30, 0, -30, -55, -58, -55, -30, 10, -10, 58, -58][i],
               [0x000000, 0xC02020, 0x2020C0][i % 3], 1 + i % 5, (i % 4) * 50)
    s.line(5, 5, 150, 6, 0x008000, 1, 0)
    s.line(5, 110, 5, 110, 0x008000, 9, 0)
    s.check("lines")


def test_triangles_share_edges():
    s = Scene(120, 100)
    s.clip_all()
    s.box(0, 0, 120, 100, style(1, 0x000000))
    # a fan and a grid of triangles at transparency 128 (alpha 127): any pixel painted twice or missed shows
    centre = (60, 50)
    ring = [(110, 50), (95, 85), (60, 95), (20, 88), (8, 50), (25, 10), (60, 3), (100, 15)]
    for i in range(len(ring)):
        (x1, y1), (x2, y2) = ring[i], ring[(i + 1) % len(ring)]
        s.triangle(centre[0], centre[1], x1, y1, x2, y2, 0xFFFFFF, 128)
    mine = s.check("triangles")
    # every pixel is either untouched black or exactly one blend
    values = set(mine[y * s.stride + x] for y in range(100) for x in range(120))
    assert values <= {0x000000, 0x7F7F7F}, sorted(hex(v) for v in values)
    s2 = Scene(64, 64)
    s2.clip_all()
    s2.box(0, 0, 64, 64, style(1, 0))
    pts = [[(x * 13 + (y * 7) % 5, y * 11 + (x * 3) % 4) for x in range(6)] for y in range(7)]
    for y in range(6):
        for x in range(5):
            a, b, c, d = pts[y][x], pts[y][x + 1], pts[y + 1][x], pts[y + 1][x + 1]
            s2.triangle(*a, *b, *d, 0xFFFFFF, 128)
            s2.triangle(*a, *d, *c, 0xFFFFFF, 128)
    mine = s2.check()
    inside = [mine[y * s2.stride + x] for y in range(14, 50) for x in range(10, 50)]
    assert set(inside) == {0x7F7F7F}, "a gap or a pixel painted twice"


def test_text():
    s = Scene(330, 150)
    s.clip_all()
    s.box(0, 0, 330, 150, style(1, 0xFFFFFF))
    y = 2
    for size, colour in [(12, 0x000000), (16, 0x202060), (20, 0x600000), (28, 0x006000), (14, 0x000000),
                         (40, 0x404040)]:
        s.text(3, y, "Brick letters: Hamburgefonstiv 0123456789 !?{}", colour, size, 0)
        y += size + 2
    s.text(200, 2, "héllo\x01", 0x0000FF, 16, 100)
    s.text(-10, 135, "", 0, 16, 0)
    s.measure("Hello, world", 16)
    s.measure("Hello, world", 33)
    s.measure("", 0)
    s.check("text")


def test_text_sizes_only_needed():
    s = Scene(200, 60, sizes=[16])
    s.clip_all()
    s.box(0, 0, 200, 60, style(2, 0x101820, 0x304050, 0))
    s.text(4, 4, "Only 16 px baked", 0xFFE0A0, 16, 0)
    s.text(4, 24, "but 13 and 24 work", 0xFFFFFF, 24, 50)
    s.text(150, 6, "small", 0xFFFFFF, 13, 0)
    s.measure("Width", 24)
    s.check()
    a = Asm()
    paint.add_all(a, [16])
    assert [k for k in a.data if k.startswith("font_")] == ["font_16"]


def test_programs_without_windows_unchanged():
    a, b = Asm(), Asm()
    for x in (a, b):
        runtime.start(x)
        runtime.finish(x)
        runtime.add_all(x)
    assert elf.wrap(a) == elf.wrap(b)


def showcase():
    s = Scene(640, 420, pad=0)
    s.clip_all()
    s.box(0, 0, 640, 420, style(2, 0xF4EFE6, 0xD9CBB3, 0))
    for d in range(6):
        s.box(20 + d * 100, 20, 80, 60, style(2, 0xFF6A00, 0x2A1A6A, d, radius=10, shadow=4))
    s.box(20, 100, 180, 120, style(3, 0xFFE08A, 0xC0392B, radius=24, border=3, bcol=0x402010, shadow=8))
    for i in range(8):
        s.box(220 + (i % 4) * 100, 100 + (i // 4) * 70, 84, 56,
              style(1, 0x3B5B92, tex=i + 1, tcol=0xDFE8FF, tsize=16 if i == 5 else 10, radius=8, border=2,
                    bcol=0x10204A))
    s.ellipse(20, 240, 160, 100, style(3, 0xA8E6FF, 0x1060A0, border=4, bcol=0xFFFFFF, shadow=6))
    s.ellipse(200, 250, 80, 80, style(1, 0x30A050, transparency=100))
    s.box(240, 280, 90, 60, style(1, 0xE03030, transparency=128, radius=30))
    s.ellipse(300, 250, 120, 40, style(0, tex=1, tcol=0x803080, tsize=6, border=2, bcol=0x803080))
    for i in range(12):
        s.line(440 + i * 15, 250, 500 + i * 8, 340, 0x202020, 1 + i // 3, 0)
    s.triangle(440, 360, 520, 360, 480, 300, 0x2080FF, 0)
    s.triangle(440, 360, 520, 360, 480, 410, 0xFF8020, 100)
    y = 350
    for size in (12, 16, 20):
        s.text(20, y, "Hello brick letters: The quick brown fox 0123456789 {}[]", 0x101010, size, 0)
        y += size + 6
    return s


def test_showcase():
    showcase().check("showcase")


def timing():
    """Milliseconds for one full-window paint, minus the program's own start and buffer writing."""
    def run_time(scene, repeat):
        path = os.path.join(FOLDER, "timed")
        with open(path, "wb") as f:
            f.write(scene.program(repeat))
        os.chmod(path, 0o755)
        best = None
        for _ in range(3):
            start = time.perf_counter()
            subprocess.run([path], stdout=subprocess.DEVNULL, check=True)
            took = time.perf_counter() - start
            best = took if best is None else min(best, took)
        return best

    def per_paint(make):
        empty = Scene(1280, 800, pad=0)
        empty.clip_all()
        base = run_time(empty, 1)
        s = Scene(1280, 800, pad=0)
        s.clip_all()
        make(s)
        return (run_time(s, 51) - base) / 51 * 1000

    results = {
        "gradient box 1280x800": per_paint(lambda s: s.box(0, 0, 1280, 800, style(2, 0xFF0000, 0x0000FF, 0))),
        "diagonal gradient box": per_paint(lambda s: s.box(0, 0, 1280, 800, style(2, 0xFF0000, 0x0000FF, 4))),
        "radial gradient box": per_paint(lambda s: s.box(0, 0, 1280, 800, style(3, 0xFF0000, 0x0000FF))),
        "textured box (stripes)": per_paint(lambda s: s.box(0, 0, 1280, 800,
                                                            style(1, 0x305080, tex=1, tcol=0xFFFFFF))),
        "textured box (grain), round, see-through": per_paint(
            lambda s: s.box(0, 0, 1280, 800, style(1, 0x305080, tex=6, tcol=0xFFFFFF, radius=20, transparency=60))),
        "100 lines of 16 px text": per_paint(lambda s: [s.text(4, 8 * i, "The quick brown fox jumps over the lazy "
                                                               "dog, 0123456789 times!", 0, 16, 0)
                                                        for i in range(100)]),
        "100 lines of 13 px text (scaled)": per_paint(lambda s: [s.text(4, 8 * i, "The quick brown fox jumps over "
                                                                        "the lazy dog, 0123456789 times!", 0, 13, 0)
                                                                 for i in range(100)]),
    }
    for name, ms in results.items():
        print(f"time: {name}: {ms:.2f} ms")


def sizes():
    a, b = Asm(), Asm()
    for x in (a, b):
        runtime.start(x)
        runtime.finish(x)
        runtime.add_all(x)
    paint.add_all(b, [16])
    plain, painted = len(elf.wrap(a)), len(elf.wrap(b))
    letters = len(font.baked(16))
    print(f"size: paint.add_all adds {painted - plain} bytes ({painted - plain - letters} code and constants, "
          f"{letters} letters for 16 px); default letters (sizes unknown): "
          f"{sum(len(font.baked(s)) for s in font.DEFAULT_SIZES)} bytes")


def main():
    tests = [(name, f) for name, f in globals().items() if name.startswith("test_")]
    passed = 0
    for name, f in tests:
        try:
            f()
            print(f"passed: {name}")
            passed += 1
        except Exception as problem:  # noqa: BLE001
            print(f"FAILED: {name}: {type(problem).__name__}: {problem}")
    if "--time" in sys.argv:
        sizes()
        timing()
    print(f"{passed} of {len(tests)} tests passed.")
    sys.exit(0 if passed == len(tests) else 1)


if __name__ == "__main__":
    main()
