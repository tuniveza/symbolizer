"""Tests for glspace: brick's own software 3D. Each test builds a small program with Asm that makes views and
objects, renders them into a pixel buffer, writes the pixels to a file, and checks them.

Usage: python3 tests/test_glspace.py            (BRICK_GL_PNG=folder also saves a few scenes as PNG files)
"""
import os
import struct
import subprocess
import sys
import tempfile
import time
import zlib

sys.path.insert(0, os.path.dirname(os.path.dirname(os.path.abspath(__file__))))
os.environ.setdefault("BRICK_MEMORY", "off")
os.environ.setdefault("BRICK_FACTORY", "off")

import elf  # noqa: E402
import glspace  # noqa: E402
import runtime  # noqa: E402
from asm import RAX, RCX, RDI, RDX, RSI, Asm  # noqa: E402
from reader import words_of  # noqa: E402

FOLDER = tempfile.mkdtemp(prefix="glspace_")
PNG_FOLDER = os.environ.get("BRICK_GL_PNG")


# ---------------- building and running test programs ----------------

class Scene:
    """Emits the code of a test program: views, objects, renders, then the pixels written to a file."""
    def __init__(self, width, height, clip=None):
        self.a = a = Asm()
        self.width, self.height = width, height
        runtime.start(a)
        a.space.setdefault("gui_target", 32)
        a.space.setdefault("gui_clip", 32)
        a.space["test_pixels"] = width * height * 4
        a.address(RCX, "gui_target")
        a.address(RAX, "test_pixels")
        a.store(RCX, 0, RAX)
        for offset, value in ((8, width), (16, height), (24, width * 4)):
            a.set(RAX, value)
            a.store(RCX, offset, RAX)
        if clip:
            a.address(RCX, "gui_clip")
            for n, value in enumerate(clip):
                a.set(RAX, value)
                a.store(RCX, 8 * n, RAX)
        self.slots = 0

    def slot(self):
        self.slots += 1
        return self.a.reserve(8, "test_slot")

    def fields(self, reg_label, values):
        """values: {offset: number}; ints are whole numbers, floats decimals."""
        a = self.a
        for offset, value in values.items():
            a.address(RDX, reg_label)
            a.load(RDX, RDX)
            a.set(RCX, value if isinstance(value, int) else glspace.bits(value))
            a.store(RDX, offset, RCX)

    def view(self, x=0, y=0, w=None, h=None, **named):
        a = self.a
        slot = self.slot()
        a.call("gl_new_view")
        a.address(RCX, slot)
        a.store(RCX, 0, RAX)
        values = {glspace.V_X: x, glspace.V_Y: y, glspace.V_W: self.width if w is None else w,
                  glspace.V_H: self.height if h is None else h}
        for name, value in named.items():
            values[getattr(glspace, "V_" + name.upper())] = value
        self.fields(slot, values)
        return slot

    def add(self, view, kind, colour=None, at=None, size=None, turn=None, spin=None, visible=None, name=None):
        a = self.a
        slot = self.slot()
        a.address(RAX, view)
        a.load(RAX, RAX)
        a.set(RCX, glspace.KINDS[kind])
        a.call("gl_add")
        a.address(RCX, slot)
        a.store(RCX, 0, RAX)
        values = {}
        if colour is not None:
            values[glspace.O_COLOUR] = colour
        if visible is not None:
            values[glspace.O_VISIBLE] = visible
        for triple, offset in ((at, glspace.O_X), (turn, glspace.O_RX), (spin, glspace.O_SPX)):
            if triple is not None:
                for n, value in enumerate(triple):
                    values[offset + 8 * n] = float(value)
        if size is not None:
            size = (size,) * 3 if isinstance(size, (int, float)) else size
            for n, value in enumerate(size):
                values[glspace.O_SX + 8 * n] = float(value)
        self.fields(slot, values)
        if name:
            a.address(RDX, slot)
            a.load(RDX, RDX)
            a.address(RCX, a.text(name))
            a.store(RDX, glspace.O_NAME, RCX)
        return slot

    def render(self, view, times=1):
        a = self.a
        for _ in range(times):
            a.address(RAX, view)
            a.load(RAX, RAX)
            a.call("gl_render")

    def sleep_ms(self, ms):
        a = self.a
        a.space.setdefault("test_nap", 16)
        a.address(RDI, "test_nap")
        a.set(RAX, ms * 1_000_000)
        a.store(RDI, 8, RAX)
        a.set(RAX, 0)
        a.store(RDI, 0, RAX)
        a.set(RSI, 0)
        a.set(RAX, 35)                       # nanosleep
        a.syscall()

    def save(self, path):
        """Write the pixels to path (open, write, close), as raw 4-byte XRGB."""
        a = self.a
        a.address(RDI, a.text(path))
        a.op_n("add", RDI, 8)
        a.set(RSI, 0o1101)                   # write only, make it, empty it
        a.set(RDX, 0o644)
        a.set(RAX, 2)
        a.syscall()
        a.mov(RDI, RAX)
        a.push(RAX)
        a.address(RSI, "test_pixels")
        a.set(RDX, self.width * self.height * 4)
        a.set(RAX, 1)
        a.syscall()
        a.pop(RDI)
        a.set(RAX, 3)
        a.syscall()

    def build(self, name):
        runtime.finish(self.a)
        runtime.add_all(self.a)
        glspace.add_all(self.a)
        path = os.path.join(FOLDER, name)
        with open(path, "wb") as f:
            f.write(elf.wrap(self.a))
        os.chmod(path, 0o755)
        return path


class Picture:
    def __init__(self, raw, width, height):
        self.raw, self.width, self.height = raw, width, height

    def __getitem__(self, xy):
        x, y = xy
        value = struct.unpack_from("<I", self.raw, (y * self.width + x) * 4)[0]
        return value >> 16 & 255, value >> 8 & 255, value & 255

    def png(self, path):
        rows = b"".join(b"\0" + bytes(c for x in range(self.width) for c in self[x, y]) for y in range(self.height))
        chunk = lambda kind, data: (struct.pack(">I", len(data)) + kind + data +      # noqa: E731
                                    struct.pack(">I", zlib.crc32(kind + data) & 0xFFFFFFFF))
        with open(path, "wb") as f:
            f.write(b"\x89PNG\r\n\x1a\n" + chunk(b"IHDR", struct.pack(">IIBBBBB", self.width, self.height, 8, 2, 0, 0, 0))
                    + chunk(b"IDAT", zlib.compress(rows, 9)) + chunk(b"IEND", b""))

    def brightness(self, x, y):
        return sum(self[x, y])

    def count(self, test, box=None):
        x0, y0, x1, y1 = box or (0, 0, self.width, self.height)
        return sum(1 for y in range(y0, y1) for x in range(x0, x1) if test(self[x, y]))


def shoot(scene, name, png=False):
    """Save the pixels, build, run; returns the Picture."""
    out = os.path.join(FOLDER, name + ".raw")
    scene.save(out)
    program = scene.build(name)
    done = subprocess.run([program], capture_output=True, timeout=60)
    assert done.returncode == 0, (name, done.returncode, done.stdout[-300:], done.stderr[-300:])
    with open(out, "rb") as f:
        picture = Picture(f.read(), scene.width, scene.height)
    if png and PNG_FOLDER:
        os.makedirs(PNG_FOLDER, exist_ok=True)
        picture.png(os.path.join(PNG_FOLDER, name + ".png"))
    return picture


RED, GREEN, BLUE, WHITE = 0xFF0000, 0x00C000, 0x0000FF, 0xFFFFFF


def reddish(p):
    return p[0] > 60 and p[0] > 2 * p[1] + 20 and p[0] > 2 * p[2] + 20


def greenish(p):
    return p[1] > 60 and p[1] > 2 * p[0] + 20 and p[1] > 2 * p[2] + 20


def blueish(p):
    return p[2] > 60 and p[2] > 2 * p[0] + 20 and p[2] > 2 * p[1] + 20


# ---------------- the tests ----------------

def test_sincos():
    """gl_sincos against Python's maths, angles from -1000 to 1000 degrees."""
    s = Scene(4, 4)
    a = s.a
    angles = [-1000.0, -720.0, -450.0, -90.0, -45.0, -30.0, 0.0, 1.0, 30.0, 44.9, 45.0, 89.0, 90.0, 135.0,
              180.0, 225.0, 270.0, 300.0, 359.0, 360.0, 361.0, 1000.0, 12345.6]
    a.space["test_sc"] = 16 * len(angles)
    a.space["test_pixels"] = max(a.space["test_pixels"], 16 * len(angles))
    for n, angle in enumerate(angles):
        a.set(RAX, glspace.bits(angle))
        a.to_xmm(0, RAX)
        a.call("gl_sincos")
        a.address(RAX, "test_pixels")
        a.store_decimal(RAX, 16 * n, 0)
        a.store_decimal(RAX, 16 * n + 8, 1)
    s.width, s.height = 4 * len(angles), 1
    picture = shoot(s, "sincos")
    import math
    for n, angle in enumerate(angles):
        sin, cos = struct.unpack_from("<dd", picture.raw, 16 * n)
        assert abs(sin - math.sin(math.radians(angle))) < 1e-8, (angle, sin)
        assert abs(cos - math.cos(math.radians(angle))) < 1e-8, (angle, cos)


def test_cube_in_the_middle():
    s = Scene(200, 150)
    view = s.view()
    s.add(view, "cube", colour=RED, turn=(0, 0, 0))
    s.render(view)
    p = shoot(s, "cube", png=True)
    assert reddish(p[100, 75]), p[100, 75]
    assert not reddish(p[5, 5]) and not reddish(p[195, 145])
    red = p.count(reddish)
    assert 1500 < red < 12000, red


def test_object_appears_where_expected():
    """Camera straight on (yaw 0, pitch 0): x to the right, y up on the screen."""
    s = Scene(200, 200)
    view = s.view(yaw=0.0, pitch=0.0)
    s.add(view, "cube", colour=RED, at=(1, 0, 0), size=0.5)
    s.add(view, "sphere", colour=BLUE, at=(0, 1, 0), size=0.5)
    s.render(view)
    p = shoot(s, "placed")
    # focal = 100 / tan(25) = 214.5 px; 1 unit at distance 4 -> 53.6 px from the centre
    assert reddish(p[154, 100]), p[154, 100]
    assert blueish(p[100, 46]), p[100, 46]
    assert not reddish(p[100, 100]) and not blueish(p[100, 100])
    assert p.count(reddish, (0, 0, 100, 200)) == 0
    assert p.count(blueish, (0, 100, 200, 200)) == 0


def test_depth_order():
    """A near object hides a far one whatever order they were added in."""
    for order in ("near first", "far first"):
        s = Scene(160, 120)
        view = s.view(yaw=0.0, pitch=0.0)
        objects = [("cube", RED, (0, 0, 1.0)), ("cube", GREEN, (0, 0, -1.0))]
        for kind, colour, at in (objects if order == "near first" else objects[::-1]):
            s.add(view, kind, colour=colour, at=at, turn=(0, 0, 0))
        s.render(view)
        p = shoot(s, "depth_" + order.replace(" ", "_"))
        assert reddish(p[80, 60]), (order, p[80, 60])
        assert p.count(greenish) == 0, order        # the green cube is exactly behind the bigger-looking red one


def test_intersecting_objects():
    """Two cubes pushed through each other: the depth test is per pixel, both show."""
    s = Scene(200, 150)
    view = s.view(yaw=30.0, pitch=20.0)
    s.add(view, "cube", colour=RED, at=(-0.25, 0, 0))
    s.add(view, "cube", colour=GREEN, at=(0.25, 0, 0), turn=(0, 45, 0))
    s.render(view)
    p = shoot(s, "intersect", png=True)
    assert p.count(reddish) > 500 and p.count(greenish) > 500


def test_back_faces_culled():
    """Inside a big cube with the camera inside it, every face points away: nothing is drawn."""
    s = Scene(100, 100)
    view = s.view(yaw=0.0, pitch=0.0, dist=0.5)
    s.add(view, "cube", colour=RED, size=10)
    s.render(view)
    p = shoot(s, "inside")
    assert p.count(reddish) == 0
    # and from outside the faces facing the camera are drawn
    s = Scene(100, 100)
    view = s.view(yaw=0.0, pitch=0.0, dist=10.0)
    s.add(view, "cube", colour=RED, size=2)
    s.render(view)
    assert shoot(s, "outside").count(reddish) > 100


def test_lit_side_is_brighter():
    """The default light comes from the upper left: the left of a sphere is brighter than the right."""
    s = Scene(200, 200)
    view = s.view(yaw=0.0, pitch=0.0)
    s.add(view, "sphere", colour=WHITE, size=2.5)
    s.render(view)
    p = shoot(s, "lit", png=True)
    left, right = p.brightness(60, 90), p.brightness(140, 110)
    assert left > right + 150, (left, right)
    top, bottom = p.brightness(100, 50), p.brightness(100, 150)
    assert top > bottom + 100, (top, bottom)
    # light from the right instead
    s = Scene(200, 200)
    view = s.view(yaw=0.0, pitch=0.0, lx=1.0, ly=0.0, lz=0.3)
    s.add(view, "sphere", colour=WHITE, size=2.5)
    s.render(view)
    p = shoot(s, "lit_right")
    assert p.brightness(140, 100) > p.brightness(60, 100) + 150


def test_cube_faces_shaded_differently():
    s = Scene(200, 200)
    view = s.view()
    s.add(view, "cube", colour=0xC0C0C0, size=1.5)
    s.render(view)
    p = shoot(s, "cube_faces")
    shades = {p[x, y] for x, y in ((100, 40), (70, 110), (130, 110))}      # top, left front, right front
    assert len(shades) == 3, shades


def test_clipped_to_the_view_rect():
    """A view in the middle of the target: nothing drawn outside its rect (not even background)."""
    s = Scene(200, 200)
    view = s.view(x=50, y=60, w=100, h=80)
    s.add(view, "cube", colour=RED, size=3)
    s.render(view)
    p = shoot(s, "rect", png=True)
    outside = p.count(lambda c: c != (0, 0, 0), (0, 0, 200, 60)) + p.count(lambda c: c != (0, 0, 0), (0, 140, 200, 200))
    outside += p.count(lambda c: c != (0, 0, 0), (0, 60, 50, 140)) + p.count(lambda c: c != (0, 0, 0), (150, 60, 200, 140))
    assert outside == 0, outside
    assert p.count(reddish, (50, 60, 150, 140)) > 3000
    assert p[50, 60] != (0, 0, 0) and p[149, 139] != (0, 0, 0)


def test_clipped_to_gui_clip():
    s = Scene(200, 200, clip=(0, 0, 100, 200))
    view = s.view()
    s.add(view, "sphere", colour=RED, size=2)
    s.render(view)
    p = shoot(s, "guiclip")
    assert p.count(lambda c: c != (0, 0, 0), (100, 0, 200, 200)) == 0
    assert p.count(reddish, (0, 0, 100, 200)) > 1000


def test_view_partly_off_the_target():
    s = Scene(100, 100)
    view = s.view(x=-150, y=40, w=300, h=200)
    s.add(view, "cube", colour=RED, size=2)
    s.render(view)
    p = shoot(s, "offtarget")
    assert p.count(reddish) > 1000
    assert p.count(lambda c: c != (0, 0, 0), (0, 0, 100, 40)) == 0


def test_behind_and_through_the_camera():
    """Objects behind the camera draw nothing, an object the camera is half inside is cut at the near plane,
    a huge plane under the camera stretches to the horizon: no crash, no smearing."""
    s = Scene(160, 120)
    view = s.view(yaw=0.0, pitch=0.0, dist=3.0)
    s.add(view, "cube", colour=RED, at=(0, 0, 6))            # behind the camera (camera at z = 3)
    s.add(view, "sphere", colour=BLUE, at=(0, 0, 3), size=1)   # the camera is inside it
    s.render(view)
    p = shoot(s, "behind")
    assert p.count(reddish) == 0
    s = Scene(160, 120)
    view = s.view(yaw=0.0, pitch=10.0, dist=3.0)
    s.add(view, "plane", colour=GREEN, at=(0, -1, 0), size=200)
    s.add(view, "cube", colour=RED, at=(0, 0, 2.9), size=0.6)   # the camera pokes through the front face
    s.add(view, "cube", colour=RED, at=(0, -0.5, 0), size=1)
    s.render(view)
    p = shoot(s, "through", png=True)
    assert p.count(greenish, (0, 75, 160, 100)) > 2500           # the floor fills the bottom
    assert p.count(greenish, (0, 0, 160, 30)) == 0               # but not the sky


def test_spin_changes_the_picture():
    """Spin follows real time: a cube spinning 90 degrees a second looks different 150 ms later, and
    gl_animating says so; a still one doesn't."""
    s = Scene(120, 120)
    view = s.view()
    s.add(view, "cube", colour=RED, spin=(0, 90, 0))
    s.render(view)
    a = s.a
    a.address(RSI, "gl_animating")
    a.load(RAX, RSI)
    a.space["test_flag"] = 8
    a.address(RSI, "test_flag")
    a.store(RSI, 0, RAX)
    first = shoot(s, "spin_a")
    s2 = Scene(120, 120)
    view = s2.view()
    s2.add(view, "cube", colour=RED, spin=(0, 90, 0))
    s2.render(view)
    s2.sleep_ms(150)
    s2.render(view)
    second = shoot(s2, "spin_b")
    assert first.raw != second.raw
    s3 = Scene(120, 120)
    view = s3.view()
    s3.add(view, "cube", colour=RED)
    s3.render(view)
    s3.sleep_ms(50)
    s3.render(view)
    assert shoot(s3, "still").raw == first.raw


def test_spin_speed_is_real_time():
    """gl_advance with 0.5 s at 90 degrees a second turns a cube 45 degrees: same picture as turning it by hand."""
    s = Scene(120, 120)
    view = s.view()
    cube = s.add(view, "cube", colour=RED, spin=(0, 90, 0))
    s.a.address(RAX, view)
    s.a.load(RAX, RAX)
    s.a.set(RCX, glspace.bits(0.5))
    s.a.call("gl_advance")
    s.fields(cube, {glspace.O_SPY: 0.0})
    s.render(view)
    spun = shoot(s, "advanced")
    s = Scene(120, 120)
    view = s.view()
    s.add(view, "cube", colour=RED, turn=(0, 45, 0))
    s.render(view)
    assert shoot(s, "turned").raw == spun.raw


def test_animating_flag():
    s = Scene(8, 8)
    a = s.a
    a.space["test_flags"] = 32
    view = s.view()
    cube = s.add(view, "cube")

    def keep(n):
        a.call("gl_check_animating")
        a.address(RSI, "gl_animating")
        a.load(RAX, RSI)
        a.address(RSI, "test_flags")
        a.store(RSI, 8 * n, RAX)
    keep(0)
    s.fields(cube, {glspace.O_SPZ: 30.0})
    keep(1)
    s.fields(cube, {glspace.O_VISIBLE: 0})
    keep(2)
    s.fields(view, {glspace.V_ORBIT: 10.0})
    keep(3)
    a.address(RSI, "test_flags")
    a.address(RDI, "test_pixels")
    a.set(RCX, 32)
    a.copy_bytes()
    flags = struct.unpack_from("<4q", shoot(s, "flags").raw)
    assert flags == (0, 1, 0, 1), flags


def test_many_objects():
    """200 objects in one view (the list grows past its first 16 places), every kind."""
    s = Scene(320, 240)
    view = s.view(dist=14.0, pitch=35.0)
    kinds = list(glspace.KINDS)
    colours = [0xE02020, 0x20C020, 0x2040F0, 0xE0C040, 0xC040C0, 0x40C0C0, 0xF08030]
    for n in range(200):
        x, z = n % 20 - 9.5, n // 20 - 4.5
        s.add(view, kinds[n % 7], colour=colours[n % 7], at=(x * 0.55, 0, z * 1.1), size=0.45,
              turn=(n * 7 % 90, n * 13 % 360, 0))
    s.render(view)
    p = shoot(s, "many", png=True)
    lit = p.count(lambda c: max(c) > 90)
    assert lit > 6000, lit
    for test in (reddish, greenish, blueish):
        assert p.count(test) > 200


def test_every_kind():
    """Each kind, side by side seen from straight on: seven separate shapes, each lit (brighter on the left)."""
    s = Scene(960, 300)
    view = s.view(dist=8.0, yaw=0.0, pitch=20.0)
    for n, kind in enumerate(glspace.KINDS):
        s.add(view, kind, colour=0xD08040, at=(n * 1.6 - 4.8, 0, 0), size=1.0, turn=(0, 20, 0))
    s.render(view)
    p = shoot(s, "kinds", png=True)
    orange = lambda c: c[0] > 60 and c[0] > c[2] + 25      # noqa: E731
    columns = [any(orange(p[x, y]) for y in range(60, 240)) for x in range(960)]
    shapes = [x for x in range(1, 960) if columns[x] and not columns[x - 1]]
    assert len(shapes) == 7, shapes
    ends = [x for x in range(1, 960) if columns[x - 1] and not columns[x]]
    for start, end in zip(shapes, ends):
        lit = [max(p[x, y][0] for y in range(60, 240) if orange(p[x, y])) for x in (start + 3, end - 3)]
        assert lit[0] >= lit[1], (start, lit)


def test_tiny_and_huge_views():
    s = Scene(1, 1)
    view = s.view(w=1, h=1)
    s.add(view, "cube", colour=RED)
    s.render(view)
    assert reddish(shoot(s, "one")[0, 0])
    s = Scene(2400, 2200)                      # bigger than the depth buffer: drawn up to 2048 x 2048
    view = s.view()
    s.add(view, "sphere", colour=RED, size=0.5)
    s.render(view)
    p = shoot(s, "huge")
    assert reddish(p[1200, 1100])
    assert p[2047, 2047] != (0, 0, 0) and p[2100, 10] == (0, 0, 0)
    s = Scene(50, 50)
    view = s.view(w=0, h=0)                   # an empty view draws nothing and doesn't crash
    s.add(view, "cube")
    s.render(view)
    assert shoot(s, "empty").count(lambda c: c != (0, 0, 0)) == 0


def test_background():
    s = Scene(40, 100)
    view = s.view(top=0xFF0000, bottom=0x0000FF)
    s.render(view)
    p = shoot(s, "gradient")
    assert p[20, 0] == (255, 0, 0) and p[20, 99][2] > 250 and p[20, 99][0] < 5
    assert 100 < p[20, 50][0] < 160 and 100 < p[20, 50][2] < 160
    s = Scene(40, 40)
    view = s.view(background=1)
    s.add(view, "cube", colour=RED)
    s.render(view)
    p = shoot(s, "nobackground")
    assert p[0, 0] == (0, 0, 0) and reddish(p[20, 20])


def test_find():
    """gl_find by name (capitals don't matter), gl_find_kind, gl_last, and 0 when there is none."""
    s = Scene(8, 8)
    a = s.a
    view = s.view()
    ball = s.add(view, "sphere", name="Ball")
    box = s.add(view, "cube", name="box")
    a.space["test_found"] = 64
    checks = [("gl_find", "ball", ball), ("gl_find", "BOX", box), ("gl_find", "nothing", None),
              ("gl_find_kind", 2, ball), ("gl_find_kind", 3, None), ("gl_last", None, box)]
    for n, (routine, what, expected) in enumerate(checks):
        a.address(RAX, view)
        a.load(RAX, RAX)
        if isinstance(what, str):
            a.address(RCX, a.text(what))
        elif what is not None:
            a.set(RCX, what)
        a.call(routine)
        if expected:
            a.address(RCX, expected)
            a.load(RCX, RCX)
            a.op("sub", RAX, RCX)               # 0 when it found the right one
            a.op_n("add", RAX, 7)
        a.address(RCX, "test_found")
        a.store(RCX, 8 * n, RAX)
    a.address(RSI, "test_found")
    a.address(RDI, "test_pixels")
    a.set(RCX, 8 * len(checks))
    a.copy_bytes()
    s.width, s.height = 2 * len(checks), 1
    found = struct.unpack_from("<%dq" % len(checks), shoot(s, "find").raw)
    assert found == (7, 7, 0, 7, 0, 7), found


def test_camera_controls():
    s = Scene(8, 8)
    a = s.a
    view = s.view()
    a.address(RAX, view)
    a.load(RAX, RAX)
    a.set(RCX, glspace.bits(20.0))
    a.set(RDX, glspace.bits(100.0))
    a.call("gl_turn_camera")
    a.address(RAX, view)
    a.load(RAX, RAX)
    a.set(RCX, glspace.bits(0.5))
    a.call("gl_zoom")
    a.address(RSI, view)
    a.load(RSI, RSI)
    a.op_n("add", RSI, glspace.V_DIST)
    a.address(RDI, "test_pixels")
    a.set(RCX, 24)
    a.copy_bytes()
    raw = shoot(s, "camera").raw
    assert struct.unpack_from("<3d", raw) == (2.0, 50.0, 89.0)


# ---------------- the English ----------------

def read(text, **known):
    return glspace.read(words_of(text), **known)


def test_read_adding():
    op = read("put a red cube in view")
    assert op["op"] == "add" and op["kind"] == "cube" and op["colour"] == 0xFF0000, op
    op = read("add a big blue sphere called ball to view at 1 0 0")
    assert (op["kind"], op["colour"], op["name"], op["size"], op["at"]) == ("sphere", 0x0000FF, "ball", 1.5, [1, 0, 0])
    op = read("draw a small green pyramid at 1, -2, 0.5")
    assert (op["kind"], op["size"], op["at"]) == ("pyramid", 0.6, [1, -2, 0.5]), op
    assert read("make a torus")["kind"] == "torus"
    assert read("put a donut in the scene")["kind"] == "torus"
    assert read("create a yellow cylinder named tower")["name"] == "tower"
    assert read("add a cone")["kind"] == "cone"
    assert read("add a light blue box")["colour"] == 0x6666FF
    assert read("put a sea green ball in the view")["kind"] == "sphere"
    op = read("add a cube at px 2 py")
    assert [w.word for w in op["at"][0]] == ["px"] and op["at"][1] == 2 and [w.word for w in op["at"][2]] == ["py"]


def test_read_changing():
    assert read("spin the cube") == {"op": "spin", "target": {"kind": "cube"}, "speed": 90.0, "axis": "y"}
    op = read("spin the ball 90 degrees a second around y")
    assert op == {"op": "spin", "target": {"name": "ball", "kind": "sphere"}, "speed": 90, "axis": "y"}, op
    assert read("spin the cube around x at 45 degrees per second")["speed"] == 45
    assert read("make the cube spin slowly")["speed"] == 30.0
    assert read("stop spinning the cube") == {"op": "stop", "target": {"kind": "cube"}}
    assert read("stop the pyramid")["op"] == "stop"
    op = read("rotate the cube 30 degrees")
    assert (op["op"], op["angle"], op["axis"], op["relative"]) == ("rotate", 30, "y", True), op
    assert read("tilt the cube 20 degrees")["axis"] == "x"
    assert read("turn the cube to 45 degrees around z")["relative"] is False
    assert read("move the ball to 0 2 0") == {"op": "move", "target": {"name": "ball", "kind": "sphere"},
                                              "to": [0, 2, 0]}
    assert read("move the cube up 2") == {"op": "move", "target": {"kind": "cube"}, "by": [0, 2, 0]}
    assert read("move the cube left")["by"] == [-1, 0, 0]
    assert read("make the cube green")["colour"] == 0x00C000
    assert read("turn the cube red") == {"op": "colour", "target": {"kind": "cube"}, "colour": 0xFF0000}
    assert read("set the colour of the cube to orange")["colour"] == 0xFFA500
    assert read("turn the cube upside down")["angle"] == 180.0
    assert read("spin the cube 2 times a second")["speed"] == 720
    assert read("make the cube disappear")["op"] == "hide"
    assert read("add a spinning green torus")["spin"] == 90.0
    assert read("paint the sphere crimson")["op"] == "colour"
    assert read("make the ball twice as big")["factor"] == 2
    assert read("make the ball half as big")["factor"] == 0.5
    assert read("make the cube 3 times bigger")["factor"] == 3
    assert read("make the cube bigger")["factor"] == 1.25
    assert read("make the cube smaller")["factor"] == 0.8
    assert read("scale the cube by 1.5")["factor"] == 1.5
    assert read("set the size of the cube to 2")["size"] == 2
    assert read("hide the pyramid") == {"op": "hide", "target": {"kind": "pyramid"}}
    assert read("show the pyramid")["op"] == "show"
    assert read("make the tower invisible", names={"tower"}) == {"op": "hide", "target": {"name": "tower"}}
    assert read("spin it")["target"] == {"last": True}
    op = read("rotate the cube angle degrees")
    assert [w.word for w in op["angle"]] == ["angle"]


def test_read_camera_and_light():
    assert read("turn the camera 20 degrees") == {"op": "camera", "yaw": 20, "pitch": 0}
    assert read("turn the camera up 10 degrees") == {"op": "camera", "yaw": 0, "pitch": 10}
    assert read("turn the camera left")["yaw"] == -15
    assert read("move the camera closer") == {"op": "zoom", "factor": 0.8}
    assert read("move the camera further away") == {"op": "zoom", "factor": 1.25}
    assert read("zoom in")["factor"] == 0.8
    assert read("set the camera distance to 8") == {"op": "distance", "distance": 8}
    assert read("move the camera to 10 away") == {"op": "distance", "distance": 10}
    assert read("pull the camera back") == {"op": "zoom", "factor": 1.25}
    assert read("orbit the camera") == {"op": "orbit", "speed": 30.0}
    assert read("spin the camera 45 degrees a second")["speed"] == 45
    assert read("stop the camera") == {"op": "orbit", "speed": 0.0}
    assert read("set the light to come from the left") == {"op": "light", "from": [-1.0, 0.3, 0.4]}
    assert read("light it from above")["from"] == [0.0, 1.0, 0.3]
    assert read("make the light come from the right")["from"] == [1.0, 0.3, 0.4]
    assert read("make the background of the view black")["op"] == "background"


def test_read_ignores_other_things():
    for text in ("write hello", "set x to 5", "make the window red", "turn left", "move to the middle of the screen",
                 "spin the wheel", "add 1 to score", "show the score", "make a list called names",
                 "hide it", "put a red button in the window"):
        assert read(text) is None, text


# ---------------- emit, inside a program the real compiler builds ----------------

def test_emit_with_the_compiler():
    """The ops' code, emitted with brick's Compiler (its value code for names like 'angle')."""
    from compiler import Compiler
    compiler = Compiler(os.path.join(FOLDER, "mywords.txt"), lambda question: "no")
    s = Scene(200, 150)
    s.a = compiler.a                        # the compiler's own assembler: start again in it
    a = s.a
    runtime.start(a)
    a.space.setdefault("gui_target", 32)
    a.space.setdefault("gui_clip", 32)
    a.space["test_pixels"] = 200 * 150 * 4
    a.address(RCX, "gui_target")
    a.address(RAX, "test_pixels")
    a.store(RCX, 0, RAX)
    for offset, value in ((8, 200), (16, 150), (24, 800)):
        a.set(RAX, value)
        a.store(RCX, offset, RAX)
    view = s.view()
    compiler.compile_line("set angle to 45")
    compiler.compile_line("set height to 0.5")
    for text in ("put a red cube called box in view", "add a big blue sphere called ball at 1.2 height 0",
                 "rotate the box angle degrees", "make the ball twice as big", "make the ball smaller",
                 "move the box to -0.8 0 0", "add a green pyramid", "hide the pyramid",
                 "turn the camera 10 degrees", "move the camera closer", "set the light to come from the left",
                 "spin the ball 90 degrees a second around y"):
        op = glspace.read(words_of(text))
        assert op, text
        glspace.emit(compiler, op, view)
    s.render(view)
    p = shoot(s, "emitted", png=True)
    assert p.count(reddish) > 300 and p.count(blueish) > 300 and p.count(greenish) == 0


def test_emit_fields():
    """The same emitted code, but the fields are written out instead of a picture."""
    from compiler import Compiler
    compiler = Compiler(os.path.join(FOLDER, "mywords.txt"), lambda question: "no")
    s = Scene(8, 8)
    s.a = a = compiler.a
    runtime.start(a)
    a.space.setdefault("gui_target", 32)
    a.space.setdefault("gui_clip", 32)
    a.space["test_pixels"] = 256
    view = s.view()
    compiler.compile_line("set angle to 45")
    compiler.compile_line("set height to 0.5")
    for text in ("put a red cube called box in view", "add a big blue sphere called ball at 1.2 height 0",
                 "rotate the box angle degrees", "rotate the box 10 degrees", "make the ball twice as big",
                 "move the box to -0.8 0 0", "add a green pyramid", "hide the pyramid",
                 "move the camera closer", "spin the ball 90 degrees a second around y", "make the box green"):
        glspace.emit(compiler, glspace.read(words_of(text)), view)
    a.address(RDI, "test_pixels")
    for n, (name, offset) in enumerate((("box", glspace.O_RY), ("box", glspace.O_X), ("ball", glspace.O_SX),
                                        ("ball", glspace.O_Y), ("ball", glspace.O_SPY), ("box", glspace.O_COLOUR))):
        a.push(RDI)
        a.address(RAX, view)
        a.load(RAX, RAX)
        a.address(RCX, a.text(name))
        a.call("gl_find")
        a.pop(RDI)
        a.load(RCX, RAX, offset)
        a.store(RDI, 8 * n, RCX)
    a.push(RDI)
    a.address(RAX, view)
    a.load(RAX, RAX)
    a.set(RCX, 3)
    a.call("gl_find_kind")
    a.pop(RDI)
    a.load(RCX, RAX, glspace.O_VISIBLE)
    a.store(RDI, 48, RCX)
    a.address(RAX, view)
    a.load(RAX, RAX)
    a.load(RCX, RAX, glspace.V_DIST)
    a.store(RDI, 56, RCX)
    a.address(RAX, "gl_animating")
    a.load(RCX, RAX)
    a.store(RDI, 64, RCX)
    s.width, s.height = 18, 1
    raw = shoot(s, "emit_fields").raw
    ry, x, size, y, spin = struct.unpack_from("<5d", raw)
    colour, visible = struct.unpack_from("<qq", raw, 40)
    dist = struct.unpack_from("<d", raw, 56)[0]
    animating = struct.unpack_from("<q", raw, 64)[0]
    assert (ry, x, size, y, spin) == (55.0, -0.8, 3.0, 0.5, 90.0), (ry, x, size, y, spin)
    assert (colour, visible, animating) == (0x00C000, 0, 1), (colour, visible, animating)
    assert abs(dist - 3.2) < 1e-9, dist


# ---------------- size and speed ----------------

def measure():
    """Render time of an 800 x 600 view with a sphere and a cube, and the bytes glspace adds."""
    def program(renders):
        s = Scene(800, 600)
        view = s.view()
        s.add(view, "sphere", colour=BLUE, at=(-0.6, 0, 0), size=1.2)
        s.add(view, "cube", colour=RED, at=(0.8, 0, 0), turn=(20, 30, 0))
        s.render(view, renders)
        out = os.path.join(FOLDER, "speed.raw")
        s.save(out)
        return s.build("speed%d" % renders)

    def best(path):
        times = []
        for _ in range(5):
            start = time.perf_counter()
            subprocess.run([path], check=True)
            times.append(time.perf_counter() - start)
        return min(times)
    one, many = program(1), program(201)
    per_render = (best(many) - best(one)) / 200
    a = Asm()
    runtime.start(a)
    runtime.finish(a)
    runtime.add_all(a)
    plain = len(elf.wrap(a))
    glspace.add_all(a)
    return per_render, len(elf.wrap(a)) - plain


def main():
    tests = [(name, test) for name, test in globals().items() if name.startswith("test_") and callable(test)]
    only = sys.argv[1:]
    passed = 0
    for name, test in tests:
        if only and not any(o in name for o in only):
            continue
        try:
            test()
            passed += 1
            print(f"passed: {name}")
        except Exception as problem:                  # noqa: BLE001
            import traceback
            traceback.print_exc()
            print(f"FAILED: {name}: {problem!r}")
    total = len([1 for name, _ in tests if not only or any(o in name for o in only)])
    print(f"{passed} of {total} tests passed.")
    if os.environ.get("BRICK_GL_MEASURE"):
        per_render, added = measure()
        print(f"800 x 600, sphere + cube: {per_render * 1000:.2f} ms per render; glspace adds {added} bytes")
    return 0 if passed == total else 1


if __name__ == "__main__":
    sys.exit(main())
