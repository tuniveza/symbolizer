"""glspace: brick's own software 3D (the "context (gl)" of bitwin). No GPU, no libraries: machine code that
turns shapes into pixels in gui_target, with a depth buffer, a light and real-time spin.

How a picture is made (gl_render):
  1. clip = the view's rect, cut to gui_clip and to the target (and to 2048 x 2048, the depth buffer's size).
     gui_clip all zeros (nobody set it yet) counts as the whole target.
  2. spin: every object turns by (degrees per second) x (seconds since this view was last drawn), real time
     from the stopwatch clock, so it turns at the same speed on every machine. The camera orbits the same way.
  3. background: a vertical gradient from the top colour to the bottom colour (the same colour twice = plain),
     or nothing at all (background 1: the 3D is drawn over whatever is there).
  4. each visible object: its points are turned (x, then y, then z), sized, moved, and seen from the camera;
     light per corner (ambient + diffuse + a small shine), triangles cut at the near plane, projected,
     back faces dropped, and filled with a depth test, the shade blended smoothly across each triangle.

Numbers: "decimal" fields are 8-byte IEEE doubles (like brick's decimals); "whole" fields are 64-bit integers.
Angles are in degrees. The world: x to the right, y up, z towards the viewer when the camera has turned 0.
A size of 1 is 1 unit across (the cube is 1 x 1 x 1, the sphere 1 wide).

GLVIEW (VIEW_SIZE bytes, from alloc):
  0 x, 8 y, 16 w, 24 h      whole: the rect in pixels (bitwin sets it before each render)
  32 camera distance        decimal, from the point it looks at (default 4)
  40 camera yaw             decimal degrees around the y axis (default 30; 0 = looking along -z)
  48 camera pitch           decimal degrees up (default 20; kept between -89 and 89 by gl_turn_camera)
  56, 64, 72                decimal: the point the camera looks at (default 0 0 0)
  80 field of view          decimal degrees across the smaller side of the rect (default 50)
  88, 96, 104 light         decimal: direction the light comes FROM, seen from the camera: x right, y up,
                            z towards the viewer (default -0.5 0.7 0.6: upper left, in front)
  112 ambient               decimal 0..1 (default 0.28)
  120 diffuse               decimal 0..1 (default 0.72)
  128 background top        whole 0xRRGGBB (default 0x2B3445)
  136 background bottom     whole 0xRRGGBB (default 0x0F131A)
  144 background kind       whole: 0 gradient (plain if both colours match), 1 none (draw over what is there)
  152 orbit                 decimal degrees per second the camera goes round (yaw); 0 = still
  160 last drawn            whole: stopwatch nanoseconds of the last render (0 = never)
  168 object count, 176 room, 184 list (address of `room` 8-byte object addresses; grows, objects never move)
  192 near                  decimal: nearest distance drawn (default 0.05; at least 0.01)
  200 moving                whole: 1 if something in this view spins or orbits (set by gl_render)
  208 next view             whole: the view made before this one (all views are chained from gl_views)
  216 shine                 decimal 0..1: strength of the white highlight (default 0.35)

OBJECT (OBJECT_SIZE bytes, from alloc; gl_add fills in the defaults):
  0 kind                    whole: 1 cube, 2 sphere, 3 pyramid, 4 cylinder, 5 cone, 6 plane, 7 torus
  8 visible                 whole: 1 shown, 0 hidden
  16 name                   whole: address of a brick text, or 0 (gl_find looks for it)
  24 colour                 whole 0xRRGGBB (default 0xC8C8C8)
  32, 40, 48 position       decimal x, y, z (default 0 0 0)
  56, 64, 72 size           decimal x, y, z (default 1 1 1)
  80, 88, 96 rotation       decimal degrees around x, y, z (turned in that order)
  104, 112, 120 spin        decimal degrees per second around x, y, z

Routines (brick's calling convention: inputs rax, rcx, rdx; answer in rax; any register may change):
  gl_new_view      -> rax = a new GLVIEW with the defaults above, no objects
  gl_add           rax = view, rcx = kind -> rax = the new object
  gl_find          rax = view, rcx = name text -> rax = the newest object with that name (capitals don't matter), or 0
  gl_find_kind     rax = view, rcx = kind -> rax = the newest object of that kind, or 0
  gl_last          rax = view -> rax = the newest object, or 0
  gl_turn_camera   rax = view, rcx = degrees round (decimal), rdx = degrees up (decimal)
  gl_zoom          rax = view, rcx = factor (decimal): distance x factor (0.8 = closer), kept within 0.01 .. 1e6
  gl_advance       rax = view, rcx = seconds (decimal): turn everything that spins by that much time
  gl_check_animating   sets gl_animating from every view (call after changing a spin by hand)
  gl_render        rax = view: draws it (see above)
  gl_sincos        xmm0 = degrees -> xmm0 = sine, xmm1 = cosine (uses rax, rcx and xmm0-xmm5 only)
Labels: gl_animating (8 bytes, non-zero while any visible object spins or any camera orbits), gl_views (the
newest view), gl_depth (the depth buffer, 2048 x 2048 4-byte places).

The English: read(words, names) turns a direction into a small dict (an "op"); emit(compiler, op, view_slot) writes
its code. See read() for the ops.
"""
import math
import re
import struct

from asm import (BELOW, EQUAL, GREATER, LESS, NEGATIVE, NOT_ABOVE, NOT_BELOW, NOT_EQUAL, NOT_GREATER,
                 NOT_LESS, R8, R9, R10, R11, R12, R13, R14, R15, RAX, RBX, RCX, RDI, RDX, RSI)

KINDS = {"cube": 1, "sphere": 2, "pyramid": 3, "cylinder": 4, "cone": 5, "plane": 6, "torus": 7}
KIND_NAMES = {n: k for k, n in KINDS.items()}

VIEW_SIZE = 256
V_X, V_Y, V_W, V_H = 0, 8, 16, 24
V_DIST, V_YAW, V_PITCH = 32, 40, 48
V_TX, V_TY, V_TZ = 56, 64, 72
V_FOV = 80
V_LX, V_LY, V_LZ = 88, 96, 104
V_AMBIENT, V_DIFFUSE = 112, 120
V_TOP, V_BOTTOM, V_BACKGROUND = 128, 136, 144
V_ORBIT, V_LAST = 152, 160
V_COUNT, V_ROOM, V_LIST = 168, 176, 184
V_NEAR, V_MOVING, V_NEXT, V_SHINE = 192, 200, 208, 216

OBJECT_SIZE = 128
O_KIND, O_VISIBLE, O_NAME, O_COLOUR = 0, 8, 16, 24
O_X, O_Y, O_Z = 32, 40, 48
O_SX, O_SY, O_SZ = 56, 64, 72
O_RX, O_RY, O_RZ = 80, 88, 96
O_SPX, O_SPY, O_SPZ = 104, 112, 120

DEFAULT_COLOUR = 0xC8C8C8
DEPTH_SIDE = 2048                 # the depth buffer covers at most this many pixels each way
DEPTH_SCALE = float(1 << 24)      # depth = (1 / distance) x this, as a 32-bit whole number; 0 = nothing yet
POINT_SCALE = 16384               # mesh points are stored as 16-bit whole numbers: 1 unit = 16384
SHADES = 512                      # shade table: 0..255 dark to the colour, 256..511 the colour to white

DEFAULTS = {V_DIST: 4.0, V_YAW: 30.0, V_PITCH: 20.0, V_FOV: 50.0, V_LX: -0.5, V_LY: 0.7, V_LZ: 0.6,
            V_AMBIENT: 0.28, V_DIFFUSE: 0.72, V_NEAR: 0.05, V_SHINE: 0.35}
DEFAULT_TOP, DEFAULT_BOTTOM = 0x2B3445, 0x0F131A
FIRST_ROOM = 16


def bits(value):
    """The 8 bytes of a decimal, as a signed whole number (for a.set)."""
    return struct.unpack("<q", struct.pack("<d", float(value)))[0]


# ---------------- the shapes, made here in Python, stored as fixed bytes ----------------

def _cube():
    points, tris = [], []
    for axis in range(3):
        for sign in (-1, 1):
            n = [0.0] * 3
            n[axis] = sign
            u, w = [0.0] * 3, [0.0] * 3
            u[(axis + 1) % 3], w[(axis + 2) % 3] = 1, 1
            base = len(points)
            for su, sw in ((-1, -1), (1, -1), (1, 1), (-1, 1)):
                points.append(([0.5 * (n[k] + u[k] * su + w[k] * sw) for k in range(3)], n))
            tris += [(base, base + 1, base + 2), (base, base + 2, base + 3)]
    return points, tris


def _sphere(slices=20, stacks=14):
    points, tris = [([0, 0.5, 0], [0, 1, 0])], []
    for j in range(1, stacks):
        phi = math.pi * j / stacks
        for i in range(slices):
            theta = 2 * math.pi * i / slices
            n = [math.sin(phi) * math.cos(theta), math.cos(phi), math.sin(phi) * math.sin(theta)]
            points.append(([0.5 * c for c in n], n))
    points.append(([0, -0.5, 0], [0, -1, 0]))
    ring = lambda j, i: 1 + (j - 1) * slices + i % slices      # noqa: E731
    for i in range(slices):
        tris.append((0, ring(1, i), ring(1, i + 1)))
        for j in range(1, stacks - 1):
            tris += [(ring(j, i), ring(j + 1, i), ring(j + 1, i + 1)), (ring(j, i), ring(j + 1, i + 1), ring(j, i + 1))]
        tris.append((len(points) - 1, ring(stacks - 1, i), ring(stacks - 1, i + 1)))
    return points, tris


def _pyramid():
    corners = [(-0.5, -0.5), (0.5, -0.5), (0.5, 0.5), (-0.5, 0.5)]
    points, tris = [], []
    for x, z in corners:
        points.append(([x, -0.5, z], [0, -1, 0]))
    tris += [(0, 1, 2), (0, 2, 3)]
    for k in range(4):
        (x1, z1), (x2, z2) = corners[k], corners[(k + 1) % 4]
        a, b, c = [x1, -0.5, z1], [x2, -0.5, z2], [0, 0.5, 0]
        n = _cross(_sub(b, a), _sub(c, a))
        if n[0] * (x1 + x2) + n[2] * (z1 + z2) < 0:          # make it point out
            n = [-c for c in n]
        base = len(points)
        points += [(a, n), (b, n), (c, n)]
        tris.append((base, base + 1, base + 2))
    return points, tris


def _cylinder(sides=24):
    points, tris = [], []
    for i in range(sides):
        t = 2 * math.pi * i / sides
        n = [math.cos(t), 0, math.sin(t)]
        points += [([0.5 * n[0], -0.5, 0.5 * n[2]], n), ([0.5 * n[0], 0.5, 0.5 * n[2]], n)]
    for i in range(sides):
        j = (i + 1) % sides
        tris += [(2 * i, 2 * j, 2 * j + 1), (2 * i, 2 * j + 1, 2 * i + 1)]
    for y in (-0.5, 0.5):
        centre = len(points)
        points.append(([0, y, 0], [0, 2 * y, 0]))
        for i in range(sides):
            t = 2 * math.pi * i / sides
            points.append(([0.5 * math.cos(t), y, 0.5 * math.sin(t)], [0, 2 * y, 0]))
        for i in range(sides):
            tris.append((centre, centre + 1 + i, centre + 1 + (i + 1) % sides))
    return points, tris


def _cone(sides=24):
    points, tris = [], []
    for i in range(sides):
        t = 2 * math.pi * i / sides
        points.append(([0.5 * math.cos(t), -0.5, 0.5 * math.sin(t)], [math.cos(t), 0.5, math.sin(t)]))
    for i in range(sides):
        t = 2 * math.pi * (i + 0.5) / sides
        points.append(([0, 0.5, 0], [math.cos(t), 0.5, math.sin(t)]))
        tris.append((sides + i, i, (i + 1) % sides))
    centre = len(points)
    points.append(([0, -0.5, 0], [0, -1, 0]))
    for i in range(sides):
        t = 2 * math.pi * i / sides
        points.append(([0.5 * math.cos(t), -0.5, 0.5 * math.sin(t)], [0, -1, 0]))
        tris.append((centre, centre + 1 + i, centre + 1 + (i + 1) % sides))
    return points, tris


def _plane():
    points = [([x, 0, z], [0, 1, 0]) for x, z in ((-0.5, -0.5), (0.5, -0.5), (0.5, 0.5), (-0.5, 0.5))]
    points += [(p, [0, -1, 0]) for p, _ in points]
    return points, [(0, 1, 2), (0, 2, 3), (4, 5, 6), (4, 6, 7)]


def _torus(around=24, tube=12, big=0.35, small=0.15):
    points, tris = [], []
    for i in range(around):
        u = 2 * math.pi * i / around
        for j in range(tube):
            v = 2 * math.pi * j / tube
            n = [math.cos(v) * math.cos(u), math.sin(v), math.cos(v) * math.sin(u)]
            r = big + small * math.cos(v)
            points.append(([r * math.cos(u), small * math.sin(v), r * math.sin(u)], n))
    at = lambda i, j: (i % around) * tube + j % tube      # noqa: E731
    for i in range(around):
        for j in range(tube):
            tris += [(at(i, j), at(i + 1, j), at(i + 1, j + 1)), (at(i, j), at(i + 1, j + 1), at(i, j + 1))]
    return points, tris


def _sub(p, q):
    return [p[k] - q[k] for k in range(3)]


def _cross(p, q):
    return [p[1] * q[2] - p[2] * q[1], p[2] * q[0] - p[0] * q[2], p[0] * q[1] - p[1] * q[0]]


def mesh_bytes(points, tris):
    """Header (4-byte point count, 4-byte triangle count), points (6 x 2 bytes: x y z, then the direction
    straight out of the surface), triangles (3 x 2-byte point numbers), all turned to face out the same way
    (counter-clockwise seen from outside)."""
    out = []
    for t in tris:
        p = [points[k][0] for k in t]
        face = _cross(_sub(p[1], p[0]), _sub(p[2], p[0]))
        if sum(c * c for c in face) < 1e-12:
            continue
        outward = [sum(points[k][1][c] for k in t) for c in range(3)]
        out.append(t if sum(face[c] * outward[c] for c in range(3)) > 0 else (t[0], t[2], t[1]))
    raw = struct.pack("<II", len(points), len(out))
    for p, n in points:
        length = math.sqrt(sum(c * c for c in n)) or 1
        raw += struct.pack("<6h", *(round(c * POINT_SCALE) for c in p), *(round(c / length * 16384) for c in n))
    for t in out:
        raw += struct.pack("<3H", *t)
    return raw


MESHES = {1: _cube, 2: _sphere, 3: _pyramid, 4: _cylinder, 5: _cone, 6: _plane, 7: _torus}
MOST_POINTS = max(len(make()[0]) for make in MESHES.values())


# ---------------- small helpers for writing the machine code ----------------

class Slots:
    """Named places in a writable block (gl_vars), 8 bytes each unless asked."""
    def __init__(self):
        self.at, self.size = {}, 0

    def __call__(self, name, size=8):
        if name not in self.at:
            self.at[name] = self.size
            self.size += size
        return self.at[name]


class Consts:
    """Fixed decimals in one block (gl_k)."""
    def __init__(self):
        self.at, self.values = {}, []

    def __call__(self, value):
        key = struct.pack("<d", float(value))
        if key not in self.at:
            self.at[key] = len(self.values) * 8
            self.values.append(key)
        return self.at[key]

    def raw(self):
        return b"".join(self.values)


def _set_decimal(a, base, offset, value):
    a.set(RCX, bits(value))
    a.store(base, offset, RCX)


# ---------------- the routines ----------------

def add_all(a):
    """Add every glspace routine (call once, after the program's last instruction, like runtime.add_all)."""
    if "gl_render" in a.labels:
        return
    a.space.setdefault("gui_target", 32)
    a.space.setdefault("gui_clip", 32)
    a.space["gl_animating"] = 8
    a.space["gl_views"] = 8
    a.space["gl_depth"] = DEPTH_SIDE * DEPTH_SIDE * 4
    a.space["gl_points"] = MOST_POINTS * 32
    a.space["gl_shades"] = SHADES * 4
    a.space["gl_cut"] = 4 * 32
    a.space["gl_seen"] = 4 * 32
    s, k = Slots(), Consts()
    for kind, make in MESHES.items():
        a.data[f"gl_mesh_{kind}"] = mesh_bytes(*make())
    g = Gen(a, s, k)
    routines = (g.new_view, g.add, g.find, g.find_kind, g.last, g.turn_camera, g.zoom, g.advance,
                g.check_animating, g.sincos, g.matmul, g.render, g.object, g.project, g.triangle)
    a.piece(lambda a: [routine() for routine in routines], split=True)     # each left out when unused (Asm.prune)
    a.space["gl_vars"] = max(s.size, 8)
    a.data["gl_k"] = k.raw()


class Gen:
    def __init__(self, a, s, k):
        self.a, self.s, self.k = a, s, k

    # -- tiny shorthands --
    def vars(self, reg=R15):
        self.a.address(reg, "gl_vars")

    def consts(self, reg=R14):
        self.a.address(reg, "gl_k")

    def ld(self, x, base, off):
        self.a.load_decimal(x, base, off)

    def st(self, base, off, x):
        self.a.store_decimal(base, off, x)

    def kd(self, x, value, base=R14):
        """xmm x = a fixed decimal (base = gl_k)."""
        self.a.load_decimal(x, base, self.k(value))

    def f(self, name, d, src):
        self.a.decimal_maths(name, d, src)

    def v(self, name, size=8):
        return self.s(name, size)

    # ---- views and objects ----

    def new_view(self):
        a = self.a
        a.label("gl_new_view")
        a.set(RCX, VIEW_SIZE)
        a.call("alloc")
        a.push(RAX)
        for offset, value in DEFAULTS.items():
            _set_decimal(a, RAX, offset, value)
        a.set(RCX, DEFAULT_TOP)
        a.store(RAX, V_TOP, RCX)
        a.set(RCX, DEFAULT_BOTTOM)
        a.store(RAX, V_BOTTOM, RCX)
        a.set(RCX, FIRST_ROOM)
        a.store(RAX, V_ROOM, RCX)
        a.set(RCX, FIRST_ROOM * 8)
        a.call("alloc")
        a.pop(RDX)
        a.store(RDX, V_LIST, RAX)
        a.address(RCX, "gl_views")              # chain it: every view can be found for gl_animating
        a.load(R8, RCX)
        a.store(RDX, V_NEXT, R8)
        a.store(RCX, 0, RDX)
        a.mov(RAX, RDX)
        a.ret()

    def add(self):
        a = self.a
        a.label("gl_add")
        a.push(RAX)
        a.push(RCX)
        a.set(RCX, OBJECT_SIZE)
        a.call("alloc")
        a.pop(RCX)
        a.store(RAX, O_KIND, RCX)
        a.set(RCX, 1)
        a.store(RAX, O_VISIBLE, RCX)
        a.set(RCX, DEFAULT_COLOUR)
        a.store(RAX, O_COLOUR, RCX)
        for offset in (O_SX, O_SY, O_SZ):
            _set_decimal(a, RAX, offset, 1.0)
        a.pop(RDX)                              # the view
        a.push(RAX)
        a.load(R8, RDX, V_COUNT)
        a.load(R9, RDX, V_ROOM)
        a.op("cmp", R8, R9)
        a.jump_if(LESS, "gl_add_room")
        a.push(RDX)                             # full: a list twice as long, the addresses copied over
        a.mov(RCX, R9)
        a.shift_left(RCX, 4)
        a.call("alloc")
        a.pop(RDX)
        a.load(RCX, RDX, V_ROOM)
        a.mov(R9, RCX)
        a.op("add", R9, R9)
        a.store(RDX, V_ROOM, R9)
        a.shift_left(RCX, 3)
        a.load(RSI, RDX, V_LIST)
        a.mov(RDI, RAX)
        a.copy_bytes()
        a.store(RDX, V_LIST, RAX)
        a.label("gl_add_room")
        a.pop(RAX)
        a.load(R8, RDX, V_COUNT)
        a.load(R9, RDX, V_LIST)
        a.mov(R10, R8)
        a.shift_left(R10, 3)
        a.op("add", R9, R10)
        a.store(R9, 0, RAX)
        a.op_n("add", R8, 1)
        a.store(RDX, V_COUNT, R8)
        a.ret()

    def newest(self, label, check):
        """A routine that walks a view's objects newest first: check(object in rax, what in r13) jumps to
        `found` when it fits."""
        a = self.a
        a.label(label)
        a.load(R12, RAX, V_COUNT)
        a.load(R14, RAX, V_LIST)
        a.mov(R13, RCX)
        look, found, none = a.new_label("gl_look"), a.new_label("gl_found"), a.new_label("gl_none")
        a.label(look)
        a.op_n("sub", R12, 1)
        a.jump_if(LESS, none)
        a.mov(RAX, R12)
        a.shift_left(RAX, 3)
        a.op("add", RAX, R14)
        a.load(RAX, RAX)
        check(found)
        a.jump(look)
        a.label(found)
        a.ret()
        a.label(none)
        a.set(RAX, 0)
        a.ret()

    def find(self):
        a = self.a

        def by_name(found):
            a.load(RCX, RAX, O_NAME)
            skip = a.new_label("gl_unnamed")
            a.op("test", RCX, RCX)
            a.jump_if(EQUAL, skip)
            a.push(RAX)
            a.mov(RAX, R13)
            a.call("text_equal")
            a.mov(RCX, RAX)
            a.pop(RAX)
            a.op("test", RCX, RCX)
            a.jump_if(NOT_EQUAL, found)
            a.label(skip)
        self.newest("gl_find", by_name)

    def find_kind(self):
        a = self.a

        def by_kind(found):
            a.load(RCX, RAX, O_KIND)
            a.op("cmp", RCX, R13)
            a.jump_if(EQUAL, found)
        self.newest("gl_find_kind", by_kind)

    def last(self):
        a = self.a
        a.label("gl_last")
        a.load(RCX, RAX, V_COUNT)
        a.op("test", RCX, RCX)
        none = a.new_label("gl_no_last")
        a.jump_if(EQUAL, none)
        a.load(RAX, RAX, V_LIST)
        a.shift_left(RCX, 3)
        a.op("add", RAX, RCX)
        a.load(RAX, RAX, -8)
        a.ret()
        a.label(none)
        a.set(RAX, 0)
        a.ret()

    def turn_camera(self):
        a = self.a
        a.label("gl_turn_camera")
        self.consts()
        a.to_xmm(1, RCX)
        self.ld(0, RAX, V_YAW)
        self.f("add", 0, 1)
        self.st(RAX, V_YAW, 0)
        a.to_xmm(1, RDX)
        self.ld(0, RAX, V_PITCH)
        self.f("add", 0, 1)
        self.kd(1, 89.0)
        a.decimal_min(0, 1)
        self.kd(1, -89.0)
        a.decimal_max(0, 1)
        self.st(RAX, V_PITCH, 0)
        a.ret()

    def zoom(self):
        a = self.a
        a.label("gl_zoom")
        self.consts()
        a.to_xmm(1, RCX)
        self.ld(0, RAX, V_DIST)
        self.f("mul", 0, 1)
        self.kd(1, 1e6)
        a.decimal_min(0, 1)
        self.kd(1, 0.01)
        a.decimal_max(0, 1)
        self.st(RAX, V_DIST, 0)
        a.ret()

    def advance(self):
        """rax = view, rcx = seconds: angle += spin x seconds, kept within one turn."""
        a = self.a
        a.label("gl_advance")
        self.consts()
        a.to_xmm(6, RCX)                        # xmm6 = seconds
        self.kd(7, 1 / 360)
        self.kd(8, 360.0)

        def turn(base, angle, speed):
            self.ld(0, base, speed)
            self.f("mul", 0, 6)
            self.ld(1, base, angle)
            self.f("add", 1, 0)
            a.copy_decimal(2, 1)                # angle - 360 x (whole turns)
            self.f("mul", 2, 7)
            a.round_decimal(2, 2, 3)
            self.f("mul", 2, 8)
            self.f("sub", 1, 2)
            self.st(base, angle, 1)
        turn(RAX, V_YAW, V_ORBIT)
        a.load(R12, RAX, V_COUNT)
        a.load(R13, RAX, V_LIST)
        loop, done = a.new_label("gl_adv"), a.new_label("gl_adv_done")
        a.label(loop)
        a.op_n("sub", R12, 1)
        a.jump_if(LESS, done)
        a.load(RDX, R13)
        a.op_n("add", R13, 8)
        for angle, speed in ((O_RX, O_SPX), (O_RY, O_SPY), (O_RZ, O_SPZ)):
            turn(RDX, angle, speed)
        a.jump(loop)
        a.label(done)
        a.ret()

    def check_animating(self):
        a = self.a
        a.label("gl_check_animating")
        a.address(R8, "gl_views")
        a.load(R8, R8)
        a.set(R11, 0)                           # anything moving anywhere
        views, objects, view_done, done = (a.new_label(h) for h in ("gl_ca_view", "gl_ca_obj", "gl_ca_vdone", "gl_ca_done"))
        a.label(views)
        a.op("test", R8, R8)
        a.jump_if(EQUAL, done)
        a.load(R10, R8, V_ORBIT)
        a.shift_left(R10, 1)                    # -0.0 counts as still
        a.load(R12, R8, V_COUNT)
        a.load(R13, R8, V_LIST)
        a.label(objects)
        a.op_n("sub", R12, 1)
        a.jump_if(LESS, view_done)
        a.load(RDX, R13)
        a.op_n("add", R13, 8)
        a.load(RAX, RDX, O_VISIBLE)
        a.op("test", RAX, RAX)
        a.jump_if(EQUAL, objects)
        for offset in (O_SPX, O_SPY, O_SPZ):
            a.load(RAX, RDX, offset)
            a.shift_left(RAX, 1)
            a.op("or", R10, RAX)
        a.jump(objects)
        a.label(view_done)
        a.set(RAX, 0)
        a.set(RCX, 1)
        a.op("test", R10, R10)
        a.move_if(NOT_EQUAL, RAX, RCX)
        a.store(R8, V_MOVING, RAX)
        a.op("or", R11, RAX)
        a.load(R8, R8, V_NEXT)
        a.jump(views)
        a.label(done)
        a.address(RAX, "gl_animating")
        a.store(RAX, 0, R11)
        a.ret()

    # ---- maths ----

    def sincos(self):
        """xmm0 = degrees -> xmm0 = sine, xmm1 = cosine. A quarter turn is cut off (the nearest whole number of
        them), what is left (at most 45 degrees) goes through the sine and cosine series, and the quarter
        turns swap and flip the two."""
        a = self.a
        k = self.k
        a.label("gl_sincos")
        a.address(RAX, "gl_k")
        a.load_decimal(1, RAX, k(1 / 90))
        self.f("mul", 0, 1)                      # quarter turns
        a.round_decimal(2, 0, 0)
        self.f("sub", 0, 2)
        a.load_decimal(1, RAX, k(math.pi / 2))
        self.f("mul", 0, 1)                      # r: radians, -pi/4 .. pi/4
        a.copy_decimal(3, 0)
        self.f("mul", 3, 0)                      # r squared
        # sine: r (1 + r2 (-1/6 + r2 (1/120 + r2 (-1/5040 + r2 / 362880))))
        a.load_decimal(1, RAX, k(1 / 362880))
        for c in (-1 / 5040, 1 / 120, -1 / 6, 1.0):
            self.f("mul", 1, 3)
            a.load_decimal(4, RAX, k(c))
            self.f("add", 1, 4)
        self.f("mul", 1, 0)                      # xmm1 = sin r
        a.load_decimal(4, RAX, k(-1 / 3628800))
        for c in (1 / 40320, -1 / 720, 1 / 24, -1 / 2, 1.0):
            self.f("mul", 4, 3)
            a.load_decimal(5, RAX, k(c))
            self.f("add", 4, 5)                  # xmm4 = cos r
        a.load_decimal(5, RAX, k(0.0))
        a.decimal_to_whole(RCX, 2)
        a.op_n("and", RCX, 3)
        q1, q2, q3 = (a.new_label(h) for h in ("gl_q1", "gl_q2", "gl_q3"))
        a.op_n("cmp", RCX, 1)
        a.jump_if(EQUAL, q1)
        a.op_n("cmp", RCX, 2)
        a.jump_if(EQUAL, q2)
        a.op_n("cmp", RCX, 3)
        a.jump_if(EQUAL, q3)
        a.copy_decimal(0, 1)                     # (sin r, cos r)
        a.copy_decimal(1, 4)
        a.ret()
        a.label(q1)                              # (cos r, -sin r)
        a.copy_decimal(0, 4)
        self.f("sub", 5, 1)
        a.copy_decimal(1, 5)
        a.ret()
        a.label(q2)                              # (-sin r, -cos r)
        a.copy_decimal(0, 5)
        self.f("sub", 0, 1)
        self.f("sub", 5, 4)
        a.copy_decimal(1, 5)
        a.ret()
        a.label(q3)                              # (-cos r, sin r)
        self.f("sub", 5, 4)
        a.copy_decimal(0, 5)
        a.ret()

    def matmul(self):
        """rax, rcx = two 3 x 3 grids of decimals (row by row) -> the grid at rdx = rax x rcx."""
        a = self.a
        a.label("gl_matmul")
        for i in range(3):
            for j in range(3):
                for m in range(3):
                    self.ld(1, RAX, (i * 3 + m) * 8)
                    self.ld(2, RCX, (m * 3 + j) * 8)
                    self.f("mul", 1, 2)
                    if m == 0:
                        a.copy_decimal(0, 1)
                    else:
                        self.f("add", 0, 1)
                self.st(RDX, (i * 3 + j) * 8, 0)
        a.ret()

    # ---- drawing ----

    def render(self):
        a, v, kd, ld, st, f = self.a, self.v, self.kd, self.ld, self.st, self.f
        a.label("gl_render")
        self.vars()
        self.consts()
        a.store(R15, v("view"), RAX)
        a.mov(RBX, RAX)
        # the meshes' addresses (only known once linked): written into gl_vars every time, it costs nothing
        for kind in MESHES:
            a.address(RCX, f"gl_mesh_{kind}")
            a.store(R15, v("meshes", 64) + (kind - 1) * 8, RCX)

        # -- time: spin by the real seconds since this view was last drawn --
        a.set(RDI, 1)
        a.call("now")
        self.vars()
        self.consts()
        a.load(RBX, R15, v("view"))
        a.load(RCX, RBX, V_LAST)
        a.store(RBX, V_LAST, RAX)
        first = a.new_label("gl_first")
        a.set(RDX, 0)
        a.op("test", RCX, RCX)
        a.jump_if(EQUAL, first)
        a.op("sub", RAX, RCX)
        a.whole_to_decimal(0, RAX)
        kd(1, 1e-9)
        f("mul", 0, 1)
        a.from_xmm(RDX, 0)
        a.label(first)
        a.mov(RCX, RDX)
        a.mov(RAX, RBX)
        a.call("gl_advance")
        a.call("gl_check_animating")
        self.vars()
        self.consts()
        a.load(RBX, R15, v("view"))

        # -- the clip rect: view rect, gui_clip, target, depth buffer size --
        a.address(RSI, "gui_target")
        a.address(RDI, "gui_clip")
        a.load(RAX, RSI, 0)
        a.store(R15, v("pixels"), RAX)
        a.load(RAX, RSI, 24)
        a.store(R15, v("stride"), RAX)
        a.load(R8, RDI, 0)
        a.load(R9, RDI, 8)
        a.load(R10, RDI, 16)
        a.load(R11, RDI, 24)
        clip_set = a.new_label("gl_clip_set")
        a.mov(RAX, R10)
        a.op("or", RAX, R11)
        a.jump_if(NOT_EQUAL, clip_set)
        a.set(R8, 0)                            # never set: the whole target
        a.set(R9, 0)
        a.load(R10, RSI, 8)
        a.load(R11, RSI, 16)
        a.label(clip_set)

        def bigger(reg, src):
            a.op("cmp", reg, src)
            a.move_if(LESS, reg, src)

        def smaller(reg, src):
            a.op("cmp", reg, src)
            a.move_if(GREATER, reg, src)
        a.set(RAX, 0)
        bigger(R8, RAX)
        bigger(R9, RAX)
        a.load(RAX, RSI, 8)
        smaller(R10, RAX)
        a.load(RAX, RSI, 16)
        smaller(R11, RAX)
        a.load(RAX, RBX, V_X)
        bigger(R8, RAX)
        a.load(RCX, RBX, V_W)
        a.op("add", RAX, RCX)
        smaller(R10, RAX)
        a.load(RAX, RBX, V_Y)
        bigger(R9, RAX)
        a.load(RCX, RBX, V_H)
        a.op("add", RAX, RCX)
        smaller(R11, RAX)
        a.lea(RAX, R8, DEPTH_SIDE)
        smaller(R10, RAX)
        a.lea(RAX, R9, DEPTH_SIDE)
        smaller(R11, RAX)
        nothing = a.new_label("gl_nothing")
        a.op("cmp", R10, R8)
        a.jump_if(NOT_GREATER, nothing)
        a.op("cmp", R11, R9)
        a.jump_if(NOT_GREATER, nothing)
        a.store(R15, v("cx0"), R8)
        a.store(R15, v("cy0"), R9)
        a.store(R15, v("cx1"), R10)
        a.store(R15, v("cy1"), R11)
        a.mov(RAX, R10)
        a.op("sub", RAX, R8)
        a.store(R15, v("cw"), RAX)
        for name, reg in (("cx0", R8), ("cy0", R9), ("cx1", R10), ("cy1", R11)):
            a.whole_to_decimal(0, reg)
            st(R15, v(name + "d"), 0)

        # -- background, row by row --
        a.load(RAX, RBX, V_BACKGROUND)
        a.op_n("cmp", RAX, 1)
        no_background = a.new_label("gl_no_bg")
        a.jump_if(EQUAL, no_background)
        a.mov(R12, R9)                          # row
        rows, row_done = a.new_label("gl_bg_row"), a.new_label("gl_bg_done")
        a.label(rows)
        a.load(RAX, R15, v("cy1"))
        a.op("cmp", R12, RAX)
        a.jump_if(NOT_LESS, row_done)
        # t = (row - y) x 256 / (h - 1)
        a.mov(RAX, R12)
        a.load(RCX, RBX, V_Y)
        a.op("sub", RAX, RCX)
        a.shift_left(RAX, 8)
        a.load(RCX, RBX, V_H)
        a.op_n("sub", RCX, 1)
        a.set(RDX, 1)
        a.op("cmp", RCX, RDX)
        a.move_if(LESS, RCX, RDX)
        a.sign_extend()
        a.divide(RCX, signed=True)
        a.mov(R13, RAX)                         # t, 0 .. 256
        a.set(R14, 0)                           # the row's colour
        for shift in (16, 8, 0):
            a.load(RAX, RBX, V_TOP)
            a.shift_right(RAX, shift)
            a.op_n("and", RAX, 255)
            a.load(RCX, RBX, V_BOTTOM)
            a.shift_right(RCX, shift)
            a.op_n("and", RCX, 255)
            a.op("sub", RCX, RAX)
            a.multiply(RCX, R13)
            a.shift_right_signed(RCX, 8)
            a.op("add", RAX, RCX)
            a.shift_left(R14, 8)
            a.op("or", R14, RAX)
        a.mov(RAX, R12)
        a.load(RCX, R15, v("stride"))
        a.multiply(RAX, RCX)
        a.load(RDI, R15, v("pixels"))
        a.op("add", RDI, RAX)
        a.load(RAX, R15, v("cx0"))
        a.shift_left(RAX, 2)
        a.op("add", RDI, RAX)
        a.load(RCX, R15, v("cw"))
        a.mov(RAX, R14)
        a.fill_dwords()
        a.op_n("add", R12, 1)
        a.jump(rows)
        a.label(row_done)
        a.label(no_background)
        self.consts()

        # -- clear the depth buffer, only as much as the clip needs --
        a.address(RDI, "gl_depth")
        a.load(RCX, R15, v("cw"))
        a.load(RAX, R15, v("cy1"))
        a.load(RDX, R15, v("cy0"))
        a.op("sub", RAX, RDX)
        a.multiply(RCX, RAX)
        a.set(RAX, 0)
        a.fill_dwords()

        # -- the camera --
        ld(0, RBX, V_YAW)
        a.call("gl_sincos")
        st(R15, v("sin_yaw"), 0)
        st(R15, v("cos_yaw"), 1)
        ld(0, RBX, V_PITCH)
        a.call("gl_sincos")
        st(R15, v("sin_pitch"), 0)
        st(R15, v("cos_pitch"), 1)
        # d = (cos p sin y, sin p, cos p cos y): from the target to the camera
        cam = v("cam", 72)                      # rows: right, up, forward
        ld(0, R15, v("cos_pitch"))
        ld(1, R15, v("sin_yaw"))
        f("mul", 1, 0)                          # dx
        ld(2, R15, v("sin_pitch"))              # dy
        ld(3, R15, v("cos_yaw"))
        f("mul", 3, 0)                          # dz
        ld(4, RBX, V_DIST)
        for n, (reg, target) in enumerate(((1, V_TX), (2, V_TY), (3, V_TZ))):
            a.copy_decimal(5, reg)
            f("mul", 5, 4)
            ld(6, RBX, target)
            f("add", 5, 6)
            st(R15, v("eye", 24) + 8 * n, 5)        # camera position (eye, eye+8, eye+16)
            kd(5, 0.0)
            f("sub", 5, reg)
            st(R15, cam + 48 + 8 * n, 5)        # forward = -d
        ld(0, R15, v("cos_yaw"))                # right = (cos y, 0, -sin y)
        st(R15, cam, 0)
        kd(0, 0.0)
        st(R15, cam + 8, 0)
        ld(1, R15, v("sin_yaw"))
        f("sub", 0, 1)
        st(R15, cam + 16, 0)
        ld(2, R15, v("sin_pitch"))              # up = (-sin p sin y, cos p, -sin p cos y)
        kd(0, 0.0)
        ld(1, R15, v("sin_yaw"))
        f("mul", 1, 2)
        f("sub", 0, 1)
        st(R15, cam + 24, 0)
        ld(0, R15, v("cos_pitch"))
        st(R15, cam + 32, 0)
        kd(0, 0.0)
        ld(1, R15, v("cos_yaw"))
        f("mul", 1, 2)
        f("sub", 0, 1)
        st(R15, cam + 40, 0)
        # focal length in pixels: (smaller side / 2) / tan(fov / 2)
        ld(0, RBX, V_FOV)
        kd(1, 0.5)
        f("mul", 0, 1)
        kd(1, 0.5)                              # keep the field of view sensible: 0.5 .. 170 degrees
        a.decimal_max(0, 1)
        kd(1, 85.0)
        a.decimal_min(0, 1)
        a.call("gl_sincos")
        f("div", 1, 0)                          # 1 / tan
        a.load(RAX, RBX, V_W)
        a.load(RCX, RBX, V_H)
        a.op("cmp", RAX, RCX)
        a.move_if(GREATER, RAX, RCX)
        a.whole_to_decimal(0, RAX)
        kd(2, 0.5)
        f("mul", 0, 2)
        f("mul", 0, 1)
        st(R15, v("focal"), 0)
        a.load(RAX, RBX, V_W)                   # the centre of the rect
        a.whole_to_decimal(0, RAX)
        f("mul", 0, 2)
        a.load(RAX, RBX, V_X)
        a.whole_to_decimal(1, RAX)
        f("add", 0, 1)
        st(R15, v("centre_x"), 0)
        a.load(RAX, RBX, V_H)
        a.whole_to_decimal(0, RAX)
        f("mul", 0, 2)
        a.load(RAX, RBX, V_Y)
        a.whole_to_decimal(1, RAX)
        f("add", 0, 1)
        st(R15, v("centre_y"), 0)
        ld(0, RBX, V_NEAR)
        kd(1, 0.01)
        a.decimal_max(0, 1)
        st(R15, v("near"), 0)
        # light, in the camera's terms (x right, y up, z forward): (lx, ly, -lz), made 1 long
        ld(0, RBX, V_LX)
        ld(1, RBX, V_LY)
        kd(2, 0.0)
        ld(3, RBX, V_LZ)
        f("sub", 2, 3)
        self.normalise(0, 1, 2, v("light", 24))
        # the half-way direction between the light and the eye (0, 0, -1), for the shine
        ld(0, R15, v("light"))
        ld(1, R15, v("light") + 8)
        ld(2, R15, v("light") + 16)
        kd(3, 1.0)
        f("sub", 2, 3)
        self.normalise(0, 1, 2, v("half", 24))
        for name, offset in (("ambient", V_AMBIENT), ("diffuse", V_DIFFUSE), ("shine", V_SHINE)):
            ld(0, RBX, offset)
            st(R15, v(name), 0)

        # -- every visible object --
        a.set(RAX, 0)
        a.store(R15, v("index"), RAX)
        objects, objects_done = a.new_label("gl_objects"), a.new_label("gl_objects_done")
        a.label(objects)
        self.vars()
        a.load(RBX, R15, v("view"))
        a.load(RAX, R15, v("index"))
        a.load(RCX, RBX, V_COUNT)
        a.op("cmp", RAX, RCX)
        a.jump_if(NOT_LESS, objects_done)
        a.lea(RCX, RAX, 1)
        a.store(R15, v("index"), RCX)
        a.load(RCX, RBX, V_LIST)
        a.shift_left(RAX, 3)
        a.op("add", RCX, RAX)
        a.load(RAX, RCX)
        a.call("gl_object")
        a.jump(objects)
        a.label(objects_done)
        a.label(nothing)
        a.ret()

    def normalise(self, x, y, z, out):
        """xmm x, y, z -> made 1 long, stored at gl_vars + out (0 0 0 if it has no length). Uses xmm3, xmm4."""
        a, f = self.a, self.f
        a.copy_decimal(3, x)
        f("mul", 3, x)
        a.copy_decimal(4, y)
        f("mul", 4, y)
        f("add", 3, 4)
        a.copy_decimal(4, z)
        f("mul", 4, z)
        f("add", 3, 4)
        a.square_root(3, 3)
        self.kd(4, 1e-12)
        zero = a.new_label("gl_nolength")
        a.compare_decimals(3, 4)
        self.kd(4, 0.0)
        a.jump_if(NOT_ABOVE, zero)
        self.kd(4, 1.0)
        f("div", 4, 3)
        a.label(zero)
        for n, reg in enumerate((x, y, z)):
            f("mul", reg, 4)
            self.st(R15, out + 8 * n, reg)

    def object(self):
        """rax = an object: turn its points, light them, cut, project and fill its triangles.
        Expects r15 = gl_vars, r14 = gl_k and the camera worked out by gl_render."""
        a, v, kd, ld, st, f = self.a, self.v, self.kd, self.ld, self.st, self.f
        a.label("gl_object")
        a.mov(R12, RAX)
        a.load(RAX, R12, O_VISIBLE)
        skip = a.new_label("gl_obj_skip")
        a.op("test", RAX, RAX)
        a.jump_if(EQUAL, skip)
        a.load(RAX, R12, O_KIND)
        a.op_n("sub", RAX, 1)
        a.op_n("cmp", RAX, len(MESHES))
        a.jump_if(NOT_BELOW, skip)              # unknown kinds (and 0) draw nothing
        a.shift_left(RAX, 3)
        a.op("add", RAX, R15)
        a.load(R13, RAX, v("meshes", 64))
        a.store(R15, v("mesh"), R13)
        a.store(R15, v("object"), R12)

        # rotation grids around x, y, z
        grids = {}
        for name, angle, layout in (("rx", O_RX, ((1, 0, 0), (0, "c", "-s"), (0, "s", "c"))),
                                    ("ry", O_RY, (("c", 0, "s"), (0, 1, 0), ("-s", 0, "c"))),
                                    ("rz", O_RZ, (("c", "-s", 0), ("s", "c", 0), (0, 0, 1)))):
            grids[name] = v(name, 72)
            ld(0, R12, angle)
            a.call("gl_sincos")
            kd(2, 0.0)
            a.copy_decimal(3, 2)
            f("sub", 3, 0)                      # -sin
            kd(4, 1.0)
            regs = {"s": 0, "c": 1, "-s": 3, 0: 2, 1: 4}
            for i, row in enumerate(layout):
                for j, item in enumerate(row):
                    st(R15, grids[name] + (i * 3 + j) * 8, regs[item])
        for left, right, out in (("ry", "rx", "t1"), ("rz", "t1", "rot"), ("cam", "rot", "vr")):
            a.lea(RAX, R15, v(left, 72))
            a.lea(RCX, R15, v(right, 72))
            a.lea(RDX, R15, v(out, 72))
            a.call("gl_matmul")
            self.consts()
        # A = vr x size / POINT_SCALE (for points), N = vr / size (for the out-directions; 0 where size is 0)
        for j, size in enumerate((O_SX, O_SY, O_SZ)):
            ld(5, R12, size)
            a.copy_decimal(6, 5)
            kd(7, 1 / POINT_SCALE)
            f("mul", 6, 7)
            kd(7, 0.0)
            zero = a.new_label("gl_flat")
            a.compare_decimals(5, 7)
            a.jump_if(EQUAL, zero)
            kd(7, 1.0)
            f("div", 7, 5)
            a.label(zero)
            for i in range(3):
                ld(0, R15, v("vr", 72) + (i * 3 + j) * 8)
                a.copy_decimal(1, 0)
                f("mul", 0, 6)
                st(R15, v("A", 72) + (i * 3 + j) * 8, 0)
                f("mul", 1, 7)
                st(R15, v("N", 72) + (i * 3 + j) * 8, 1)
        # b = cam x (position - eye)
        for n, offset in enumerate((O_X, O_Y, O_Z)):
            ld(n, R12, offset)
            ld(3, R15, v("eye") + 8 * n)
            f("sub", n, 3)
        for i in range(3):
            for m in range(3):
                ld(4, R15, v("cam", 72) + (i * 3 + m) * 8)
                f("mul", 4, m)
                if m == 0:
                    a.copy_decimal(5, 4)
                else:
                    f("add", 5, 4)
            st(R15, v("b", 24) + 8 * i, 5)

        # the shade table: 0..255 black to the colour, 256..511 the colour to white
        a.load(RAX, R12, O_COLOUR)
        a.address(RDI, "gl_shades")
        a.set(RCX, 0)
        shades = a.new_label("gl_shade")
        a.label(shades)
        a.set(R11, 0)
        for shift in (16, 8, 0):
            a.mov(RDX, RAX)
            a.shift_right(RDX, shift)
            a.op_n("and", RDX, 255)             # the channel
            a.mov(R8, RCX)
            a.op_n("and", R8, 255)              # how far along
            whiter, mixed = a.new_label("gl_whiter"), a.new_label("gl_mixed")
            a.op_n("cmp", RCX, 256)
            a.jump_if(NOT_LESS, whiter)
            a.multiply(R8, RDX)                 # channel x i / 255
            a.mov(R9, R8)
            a.shift_right(R9, 8)
            a.op("add", R8, R9)
            a.op_n("add", R8, 1)
            a.shift_right(R8, 8)
            a.jump(mixed)
            a.label(whiter)                     # channel + (255 - channel) x i / 255
            a.set(R9, 255)
            a.op("sub", R9, RDX)
            a.multiply(R8, R9)
            a.mov(R9, R8)
            a.shift_right(R9, 8)
            a.op("add", R8, R9)
            a.op_n("add", R8, 1)
            a.shift_right(R8, 8)
            a.op("add", R8, RDX)
            a.label(mixed)
            a.shift_left(R11, 8)
            a.op("or", R11, R8)
        a.store32(RDI, 0, R11)
        a.op_n("add", RDI, 4)
        a.op_n("add", RCX, 1)
        a.op_n("cmp", RCX, SHADES)
        a.jump_if(LESS, shades)

        # -- every point: where the camera sees it, and how much light it gets --
        a.load32(R10, R13, 0)                   # point count
        a.lea(R8, R13, 8)
        a.address(R9, "gl_points")
        points = a.new_label("gl_point")
        a.label(points)
        for n in range(3):
            a.load16_signed(RAX, R8, 2 * n)
            a.whole_to_decimal(n, RAX)
        for i in range(3):
            ld(3, R15, v("A", 72) + i * 24)
            f("mul", 3, 0)
            ld(4, R15, v("A", 72) + i * 24 + 8)
            f("mul", 4, 1)
            f("add", 3, 4)
            ld(4, R15, v("A", 72) + i * 24 + 16)
            f("mul", 4, 2)
            f("add", 3, 4)
            ld(4, R15, v("b", 24) + 8 * i)
            f("add", 3, 4)
            st(R9, 8 * i, 3)
        for n in range(3):
            a.load16_signed(RAX, R8, 6 + 2 * n)
            a.whole_to_decimal(n, RAX)
        for i in range(3):                      # xmm5, 6, 7 = the out-direction, seen from the camera
            ld(5 + i, R15, v("N", 72) + i * 24)
            f("mul", 5 + i, 0)
            ld(4, R15, v("N", 72) + i * 24 + 8)
            f("mul", 4, 1)
            f("add", 5 + i, 4)
            ld(4, R15, v("N", 72) + i * 24 + 16)
            f("mul", 4, 2)
            f("add", 5 + i, 4)
        a.copy_decimal(3, 5)
        f("mul", 3, 5)
        a.copy_decimal(4, 6)
        f("mul", 4, 6)
        f("add", 3, 4)
        a.copy_decimal(4, 7)
        f("mul", 4, 7)
        f("add", 3, 4)
        ld(8, R15, v("ambient"))
        unlit = a.new_label("gl_unlit")
        kd(4, 1e-30)
        a.compare_decimals(3, 4)
        a.jump_if(NOT_ABOVE, unlit)
        a.square_root(3, 3)
        kd(9, 1.0)
        f("div", 9, 3)                          # xmm9 = 1 / length
        for name, out in (("light", 10), ("half", 11)):
            ld(out, R15, v(name))
            f("mul", out, 5)
            ld(4, R15, v(name) + 8)
            f("mul", 4, 6)
            f("add", out, 4)
            ld(4, R15, v(name) + 16)
            f("mul", 4, 7)
            f("add", out, 4)
            f("mul", out, 9)
            kd(4, 0.0)
            a.decimal_max(out, 4)
        ld(4, R15, v("diffuse"))
        f("mul", 10, 4)
        f("add", 8, 10)
        for _ in range(5):                      # shine: (cos of the half-way angle) to the power 32
            f("mul", 11, 11)
        ld(4, R15, v("shine"))
        f("mul", 11, 4)
        f("add", 8, 11)
        a.label(unlit)
        kd(4, 255.0)
        f("mul", 8, 4)
        kd(4, 0.5)
        a.decimal_max(8, 4)
        kd(4, SHADES - 1.5)
        a.decimal_min(8, 4)
        st(R9, 24, 8)
        a.op_n("add", R8, 12)
        a.op_n("add", R9, 32)
        a.op_n("sub", R10, 1)
        a.jump_if(GREATER, points)

        # -- every triangle --
        ld(0, R15, v("near"))
        a.load32(RAX, R13, 0)
        a.load32(R10, R13, 4)                   # triangle count
        a.op("test", R10, R10)
        a.jump_if(EQUAL, skip)
        a.lea(R8, R13, 8)
        a.multiply_n(RAX, RAX, 12)
        a.op("add", R8, RAX)                    # the triangles start after the points
        tris, tri_next = a.new_label("gl_tri_loop"), a.new_label("gl_tri_next")
        a.label(tris)
        a.store(R15, v("tri_at"), R8)
        a.store(R15, v("tri_left"), R10)
        a.address(R9, "gl_points")
        a.set(R11, 0)                           # which corners are in front of the near plane: bits 0, 1, 2
        ld(0, R15, v("near"))
        for n in range(3):
            a.load16(RAX, R8, 2 * n)
            a.shift_left(RAX, 5)
            a.op("add", RAX, R9)
            a.store(R15, v("corner%d" % n), RAX)
            ld(1, RAX, 16)
            behind = a.new_label("gl_behind")
            a.compare_decimals(1, 0)
            a.jump_if(BELOW, behind)
            a.op_n("or", R11, 1 << n)
            a.label(behind)
        a.op("test", R11, R11)
        a.jump_if(EQUAL, tri_next)
        a.address(RDI, "gl_seen")
        cut = a.new_label("gl_cut")
        a.op_n("cmp", R11, 7)
        a.jump_if(NOT_EQUAL, cut)
        for n in range(3):                      # all in front: project the three corners
            a.load(RSI, R15, v("corner%d" % n))
            a.call("gl_project")
        a.address(R9, "gl_seen")
        a.lea(R10, R9, 32)
        a.lea(R11, R9, 64)
        a.call("gl_triangle")
        a.jump(tri_next)
        # partly behind: cut the triangle at the near plane (3 or 4 corners left), project, fill 1 or 2
        a.label(cut)
        a.address(RDI, "gl_cut")
        for ka, kb in ((0, 1), (1, 2), (2, 0)):
            keep, same = a.new_label("gl_keep"), a.new_label("gl_same")
            a.mov(RAX, R11)
            a.op_n("and", RAX, 1 << ka)
            a.jump_if(EQUAL, keep)
            a.load(RSI, R15, v("corner%d" % ka))
            a.set(RCX, 32)
            a.copy_bytes()                      # rdi moves on by 32
            a.label(keep)
            a.mov(RAX, R11)
            a.shift_right(RAX, ka)
            a.mov(RCX, R11)
            a.shift_right(RCX, kb)
            a.op("xor", RAX, RCX)
            a.op_n("and", RAX, 1)
            a.jump_if(EQUAL, same)
            # rsi = the corner in front, rdx = the one behind: always measured from the one in front
            a.load(RSI, R15, v("corner%d" % ka))
            a.load(RDX, R15, v("corner%d" % kb))
            a.mov(RAX, R11)
            a.op_n("and", RAX, 1 << ka)
            in_front = a.new_label("gl_infront")
            a.jump_if(NOT_EQUAL, in_front)
            a.mov(RAX, RSI)
            a.mov(RSI, RDX)
            a.mov(RDX, RAX)
            a.label(in_front)
            ld(0, R15, v("near"))               # t = (near - in.z) / (out.z - in.z)
            ld(1, RSI, 16)
            f("sub", 0, 1)
            ld(2, RDX, 16)
            f("sub", 2, 1)
            f("div", 0, 2)
            for field in range(4):
                ld(1, RSI, field * 8)
                ld(2, RDX, field * 8)
                f("sub", 2, 1)
                f("mul", 2, 0)
                f("add", 1, 2)
                st(RDI, field * 8, 1)
            a.op_n("add", RDI, 32)
            a.label(same)
        a.address(RAX, "gl_cut")
        a.op("sub", RDI, RAX)
        a.shift_right(RDI, 5)
        a.store(R15, v("cut_count"), RDI)
        a.address(RSI, "gl_cut")
        a.address(RDI, "gl_seen")
        three = a.new_label("gl_three")
        for n in range(4):
            if n == 3:
                a.load(RAX, R15, v("cut_count"))
                a.op_n("cmp", RAX, 4)
                a.jump_if(LESS, three)
            a.call("gl_project")
            a.op_n("add", RSI, 32)
        a.label(three)
        a.address(R9, "gl_seen")
        a.lea(R10, R9, 32)
        a.lea(R11, R9, 64)
        a.call("gl_triangle")
        a.load(RAX, R15, v("cut_count"))
        a.op_n("cmp", RAX, 4)
        a.jump_if(LESS, tri_next)
        a.address(R9, "gl_seen")
        a.lea(R10, R9, 64)
        a.lea(R11, R9, 96)
        a.call("gl_triangle")
        a.label(tri_next)
        a.load(R8, R15, v("tri_at"))
        a.load(R10, R15, v("tri_left"))
        a.op_n("add", R8, 6)
        a.op_n("sub", R10, 1)
        a.jump_if(GREATER, tris)
        a.label(skip)
        a.ret()

    def project(self):
        """rsi = a point seen from the camera (x, y, z, shade) -> at rdi: screen x, screen y, depth, shade;
        rdi moves on by 32. Expects r15 = gl_vars, r14 = gl_k."""
        a, v, kd, ld, st, f = self.a, self.v, self.kd, self.ld, self.st, self.f
        a.label("gl_project")
        ld(0, RSI, 16)
        kd(1, 1.0)
        f("div", 1, 0)                          # 1 / z
        ld(2, R15, v("focal"))
        f("mul", 2, 1)
        ld(0, RSI, 0)
        f("mul", 0, 2)
        ld(3, R15, v("centre_x"))
        f("add", 0, 3)
        st(RDI, 0, 0)
        ld(0, RSI, 8)
        f("mul", 0, 2)
        ld(3, R15, v("centre_y"))
        f("sub", 3, 0)
        st(RDI, 8, 3)
        kd(0, DEPTH_SCALE)
        f("mul", 1, 0)
        st(RDI, 16, 1)
        ld(0, RSI, 24)
        st(RDI, 24, 0)
        a.op_n("add", RDI, 32)
        a.ret()

    def triangle(self):
        """r9, r10, r11 = three projected corners (screen x, y, depth, shade). Fills the triangle if it faces
        the camera, inside the clip, nearer than what is there. Expects r15 = gl_vars, r14 = gl_k."""
        a, v, kd, ld, st, f = self.a, self.v, self.kd, self.ld, self.st, self.f
        a.label("gl_triangle")
        done = a.new_label("gl_tri_done")
        x0, y0, x1, y1, x2, y2 = 0, 1, 2, 3, 4, 5

        def corners():
            ld(x0, R9, 0)
            ld(y0, R9, 8)
            ld(x1, R10, 0)
            ld(y1, R10, 8)
            ld(x2, R11, 0)
            ld(y2, R11, 8)
        corners()
        # area = (x1-x0)(y2-y0) - (x2-x0)(y1-y0); facing the camera means negative here (y grows downwards)
        a.copy_decimal(6, x1)
        f("sub", 6, x0)
        a.copy_decimal(7, y2)
        f("sub", 7, y0)
        f("mul", 6, 7)
        a.copy_decimal(7, x2)
        f("sub", 7, x0)
        a.copy_decimal(8, y1)
        f("sub", 8, y0)
        f("mul", 7, 8)
        f("sub", 6, 7)
        kd(7, 0.0)
        a.compare_decimals(7, 6)
        a.jump_if(NOT_ABOVE, done)              # back face, no area, or not a number
        a.mov(RAX, R10)                         # swap corners 1 and 2 so the area is positive
        a.mov(R10, R11)
        a.mov(R11, RAX)
        f("sub", 7, 6)
        kd(6, 1.0)
        f("div", 6, 7)
        st(R15, v("inv_area"), 6)
        corners()
        # the box around it, cut to the clip: whole pixels xs .. xe-1, ys .. ye-1
        for lo, hi, a0, a1, a2, name in ((6, 7, x0, x1, x2, "x"), (8, 9, y0, y1, y2, "y")):
            a.copy_decimal(lo, a0)
            a.decimal_min(lo, a1)
            a.decimal_min(lo, a2)
            a.copy_decimal(hi, a0)
            a.decimal_max(hi, a1)
            a.decimal_max(hi, a2)
            ld(10, R15, v("c%s0d" % name))
            a.decimal_max(lo, 10)
            ld(10, R15, v("c%s1d" % name))
            a.decimal_min(hi, 10)
            a.decimal_to_whole(RAX, lo)
            a.decimal_to_whole(RCX, hi)
            a.op_n("add", RCX, 1)
            a.load(RDX, R15, v("c%s1" % name))
            a.op("cmp", RCX, RDX)
            a.move_if(GREATER, RCX, RDX)
            a.load(RDX, R15, v("c%s0" % name))
            a.op("cmp", RAX, RDX)
            a.move_if(LESS, RAX, RDX)
            a.op("cmp", RAX, RCX)
            a.jump_if(NOT_LESS, done)
            a.store(R15, v(name + "s"), RAX)
            a.store(R15, v(name + "e"), RCX)
        # the first pixel's middle
        a.load(RAX, R15, v("xs"))
        a.whole_to_decimal(6, RAX)
        kd(7, 0.5)
        f("add", 6, 7)                          # px
        a.load(RAX, R15, v("ys"))
        a.whole_to_decimal(8, RAX)
        f("add", 8, 7)                          # py
        # edges: e(a->b)(p) = (bx-ax)(py-ay) - (by-ay)(px-ax); e12 weighs corner 0, e20 corner 1, e01 corner 2
        for n, (pa, pb) in enumerate((((x1, y1), (x2, y2)), ((x2, y2), (x0, y0)), ((x0, y0), (x1, y1)))):
            (ax, ay), (bx, by) = pa, pb
            a.copy_decimal(9, bx)
            f("sub", 9, ax)                     # bx - ax: step down a row
            st(R15, v("edy%d" % n), 9)
            a.copy_decimal(10, ay)
            f("sub", 10, by)                    # ay - by: step right a pixel
            st(R15, v("edx%d" % n), 10)
            a.copy_decimal(11, 8)
            f("sub", 11, ay)
            f("mul", 9, 11)
            a.copy_decimal(11, 6)
            f("sub", 11, ax)
            f("mul", 10, 11)
            f("add", 9, 10)
            st(R15, v("erow%d" % n), 9)
        # depth and shade change evenly across the screen: value = v0 + dx (px - x0) + dy (py - y0)
        for field, name in ((16, "z"), (24, "i")):
            ld(9, R9, field)
            ld(10, R10, field)
            ld(11, R11, field)
            ld(12, R15, v("inv_area"))
            # d/dx = (a0 (y1-y2) + a1 (y2-y0) + a2 (y0-y1)) / area
            for out, (p, q), (r, s_), (t, u) in ((13, (y1, y2), (y2, y0), (y0, y1)), (14, (x2, x1), (x0, x2), (x1, x0))):
                a.copy_decimal(out, p)
                f("sub", out, q)
                f("mul", out, 9)
                a.copy_decimal(15, r)
                f("sub", 15, s_)
                f("mul", 15, 10)
                f("add", out, 15)
                a.copy_decimal(15, t)
                f("sub", 15, u)
                f("mul", 15, 11)
                f("add", out, 15)
                f("mul", out, 12)
            st(R15, v(name + "dx"), 13)
            st(R15, v(name + "dy"), 14)
            a.copy_decimal(15, 6)
            f("sub", 15, x0)
            f("mul", 13, 15)
            a.copy_decimal(15, 8)
            f("sub", 15, y0)
            f("mul", 14, 15)
            f("add", 9, 13)
            f("add", 9, 14)
            st(R15, v(name + "row"), 9)
        # the rows
        ld(3, R15, v("edx0"))
        ld(4, R15, v("edx1"))
        ld(5, R15, v("edx2"))
        ld(7, R15, v("zdx"))
        ld(9, R15, v("idx"))
        a.address(RBX, "gl_shades")
        a.load(R12, R15, v("ys"))
        row, pixel, outside, step, row_done = (a.new_label(h) for h in
                                               ("gl_row", "gl_px", "gl_out", "gl_step", "gl_row_done"))
        a.label(row)
        a.load(RDI, R15, v("stride"))
        a.multiply(RDI, R12)
        a.load(RAX, R15, v("pixels"))
        a.op("add", RDI, RAX)
        a.load(RAX, R15, v("xs"))
        a.mov(RDX, RAX)
        a.shift_left(RAX, 2)
        a.op("add", RDI, RAX)
        a.mov(RSI, R12)                         # depth place: ((y - cy0) x cw + (x - cx0)) x 4
        a.load(RAX, R15, v("cy0"))
        a.op("sub", RSI, RAX)
        a.load(RAX, R15, v("cw"))
        a.multiply(RSI, RAX)
        a.op("add", RSI, RDX)
        a.load(RAX, R15, v("cx0"))
        a.op("sub", RSI, RAX)
        a.shift_left(RSI, 2)
        a.address(RAX, "gl_depth")
        a.op("add", RSI, RAX)
        a.load(RCX, R15, v("xe"))
        a.op("sub", RCX, RDX)
        ld(0, R15, v("erow0"))
        ld(1, R15, v("erow1"))
        ld(2, R15, v("erow2"))
        ld(6, R15, v("zrow"))
        ld(8, R15, v("irow"))
        a.set(R8, 0)
        a.label(pixel)
        a.from_xmm(RAX, 0)                      # inside when no edge value is negative
        a.from_xmm(RDX, 1)
        a.op("or", RAX, RDX)
        a.from_xmm(RDX, 2)
        a.op("or", RAX, RDX)
        a.jump_if(NEGATIVE, outside)
        a.set(R8, 1)
        a.decimal_to_whole(RAX, 6)
        a.load32(RDX, RSI)
        a.op("cmp", RAX, RDX)
        a.jump_if(NOT_GREATER, step)            # something nearer is already there
        a.store32(RSI, 0, RAX)
        a.decimal_to_whole(RAX, 8)
        a.shift_left(RAX, 2)
        a.op("add", RAX, RBX)
        a.load32(RAX, RAX)
        a.store32(RDI, 0, RAX)
        a.jump(step)
        a.label(outside)
        a.op("test", R8, R8)
        a.jump_if(NOT_EQUAL, row_done)          # been inside and left: nothing more on this row
        a.label(step)
        f("add", 0, 3)
        f("add", 1, 4)
        f("add", 2, 5)
        f("add", 6, 7)
        f("add", 8, 9)
        a.op_n("add", RDI, 4)
        a.op_n("add", RSI, 4)
        a.op_n("sub", RCX, 1)
        a.jump_if(GREATER, pixel)
        a.label(row_done)
        for n in range(3):
            ld(10, R15, v("erow%d" % n))
            ld(11, R15, v("edy%d" % n))
            f("add", 10, 11)
            st(R15, v("erow%d" % n), 10)
        for name in ("z", "i"):
            ld(10, R15, v(name + "row"))
            ld(11, R15, v(name + "dy"))
            f("add", 10, 11)
            st(R15, v(name + "row"), 10)
        a.op_n("add", R12, 1)
        a.load(RAX, R15, v("ye"))
        a.op("cmp", R12, RAX)
        a.jump_if(LESS, row)
        a.label(done)
        a.ret()


# ---------------- the English ----------------
#
# read(words, names=()) -> an op (a dict) or None when the words aren't about 3D. names: names of 3D objects the
# program has made so far ("spin the tower"). A value in an op is a number, a list of reader.Word (a name or
# expression the compiler works out, like ["angle"]), or ("-", words) for minus that.
# A target is {"kind": "cube"}, {"name": "ball", "kind": "sphere"} (name first, then the newest of that kind),
# {"name": "tower"}, or {"last": True} (it: the newest object).
#
#   {"op": "add", "kind": "cube", "colour": 0xFF0000 or None, "size": value or None, "name": text or None,
#    "at": [x, y, z] or None, "spin": degrees a second or None, "view": the word after in/to ("view") or None}
#   {"op": "spin", "target": T, "speed": value, "axis": "x" | "y" | "z"}      {"op": "stop", "target": T}
#   {"op": "rotate", "target": T, "angle": value, "axis": a, "relative": True (by) | False (to)}
#   {"op": "move", "target": T, "to": [x, y, z]}  or  {"op": "move", "target": T, "by": [dx, dy, dz]}
#   {"op": "colour", "target": T, "colour": 0xRRGGBB}
#   {"op": "scale", "target": T, "factor": value}   {"op": "size", "target": T, "size": value}
#   {"op": "hide", "target": T}   {"op": "show", "target": T}
#   {"op": "camera", "yaw": value, "pitch": value}  (turn it, degrees)    {"op": "zoom", "factor": number}
#   {"op": "distance", "distance": value}           {"op": "orbit", "speed": value (degrees a second; 0 stops)}
#   {"op": "light", "from": [x, y, z]}  (seen from the camera: x right, y up, z towards the viewer)
#   {"op": "background", "top": 0xRRGGBB, "bottom": 0xRRGGBB}

SHAPE_WORDS = {"cube": "cube", "cubes": "cube", "box": "cube", "block": "cube", "dice": "cube", "die": "cube",
               "sphere": "sphere", "ball": "sphere", "globe": "sphere", "orb": "sphere", "planet": "sphere",
               "pyramid": "pyramid", "cylinder": "cylinder", "tube": "cylinder", "pillar": "cylinder",
               "cone": "cone", "plane": "plane", "floor": "plane", "ground": "plane", "tile": "plane",
               "torus": "torus", "donut": "torus", "doughnut": "torus", "ring": "torus"}
FILLER = {"please", "now", "then", "also", "just"}
ARTICLES = {"a", "an", "another", "one", "new", "some"}
ADD_VERBS = {"put", "add", "place", "draw", "create", "make", "insert", "spawn", "build", "show", "drop"}
SIZES = {"big": 1.5, "large": 1.5, "huge": 2.5, "giant": 2.5, "enormous": 2.5, "small": 0.6, "little": 0.6,
         "tiny": 0.3, "medium": 1.0, "normal": 1.0}
PLACES = {"in", "into", "to", "inside", "on", "onto"}
UNITS = {"degrees", "degree", "times", "turns", "turn", "revolutions", "rotations", "spins", "units", "unit",
         "steps", "step"}
SPIN_WORDS = {"spin", "spins", "spinning", "twirl", "twirling", "whirl", "whirling"}
ROTATE_VERBS = {"rotate", "turn", "tilt", "twist", "roll", "flip", "angle"}
MOVE_VERBS = {"move", "shift", "slide", "put", "place", "push", "bring", "send", "raise", "lower", "lift", "drop"}
HIDE_VERBS = {"hide", "remove", "delete", "erase"}
SHOW_VERBS = {"show", "unhide", "reveal"}
DIRECTIONS = {"up": (0, 1, 0), "upwards": (0, 1, 0), "down": (0, -1, 0), "downwards": (0, -1, 0),
              "left": (-1, 0, 0), "right": (1, 0, 0), "forward": (0, 0, 1), "forwards": (0, 0, 1),
              "closer": (0, 0, 1), "nearer": (0, 0, 1), "back": (0, 0, -1), "backward": (0, 0, -1),
              "backwards": (0, 0, -1), "away": (0, 0, -1), "further": (0, 0, -1), "farther": (0, 0, -1)}
LIGHT_FROM = {"left": (-1.0, 0.3, 0.4), "right": (1.0, 0.3, 0.4), "above": (0.0, 1.0, 0.3), "top": (0.0, 1.0, 0.3),
              "overhead": (0.0, 1.0, 0.3), "up": (0.0, 1.0, 0.3), "below": (0.0, -1.0, 0.3),
              "bottom": (0.0, -1.0, 0.3), "underneath": (0.0, -1.0, 0.3), "under": (0.0, -1.0, 0.3),
              "front": (0.0, 0.2, 1.0), "camera": (0.0, 0.2, 1.0), "viewer": (0.0, 0.2, 1.0),
              "behind": (0.0, 0.2, -1.0), "back": (0.0, 0.2, -1.0)}
KEYWORDS = (set(SHAPE_WORDS) | FILLER | ARTICLES | set(SIZES) | PLACES | UNITS | SPIN_WORDS | set(DIRECTIONS)
            | {"the", "at", "called", "named", "by", "around", "about", "second", "seconds", "per", "each",
               "every", "a", "and", "of", "as", "camera", "light", "view", "scene", "x", "y", "z", "it"})


def _colour(w, j):
    """A colour name at w[j] -> (0xRRGGBB, words used), or None."""
    from actions import colour_at
    found = colour_at(w + [None, None, None], j)
    if not found:
        return None
    red, green, blue = (int(c) for c in found[0].split(";")[2:5])
    return red << 16 | green << 8 | blue, found[1]


def _value(words, w, j, stop=()):
    """A number (digits or words) or a single name at j -> (value, next j), or (None, j)."""
    from reader import read_number
    if j >= len(w):
        return None, j
    number, after = read_number(words, j)
    if number is not None:
        return number, after
    if w[j] and re.fullmatch(r"[a-z_]\w*", w[j]) and w[j] not in KEYWORDS and w[j] not in stop:
        return [words[j]], j + 1
    return None, j


def _amount(words, w, start, end=None):
    """The first number in w[start:end], or a name just before a unit ('angle degrees') -> (value, where) or
    (None, None)."""
    end = len(w) if end is None else end
    for j in range(start, end):
        value, after = _value(words, w, j)
        if value is None:
            continue
        if isinstance(value, list) and not (after < len(w) and w[after] in UNITS):
            continue
        return value, j
    return None, None


def _axis(w, default="y"):
    for j, word in enumerate(w):
        if word in ("around", "about", "on", "along", "in"):
            k = j + 1
            if k < len(w) and w[k] == "the":
                k += 1
            if k < len(w) and w[k] and w[k].split("-")[0] in ("x", "y", "z"):
                return w[k].split("-")[0]
    return default


def _coordinates(words, w, j):
    """'1 0 0', '1, -2, 0.5', 'x 2 y' from j on -> ([x, y, z], next j) or (None, j). Two numbers: z is 0."""
    values = []
    k = j
    while k < len(w) and len(values) < 3:
        value, after = _value(words, w, k, stop={"called", "named", "spinning"})
        if value is None:
            break
        values.append(value)
        k = after
    if len(values) < 2:
        return None, j
    return values + [0] * (3 - len(values)), k


def _target(words, w, names, start=0, it=False):
    """Where the object is named in w -> (target, position) or (None, None)."""
    for j in range(start, len(w)):
        word = w[j]
        if word is None:
            continue
        if word in names:
            target = {"name": word}
            if word in SHAPE_WORDS:
                target["kind"] = SHAPE_WORDS[word]
            return target, j
        if word in SHAPE_WORDS and SHAPE_WORDS[word] != "plane" or word in ("plane", "floor", "ground") and \
                j > 0 and w[j - 1] in ("the", "that", "this"):
            kind = SHAPE_WORDS[word]
            return ({"kind": kind} if word in KINDS else {"name": word, "kind": kind}), j
        if word == "it" and it:
            return {"last": True}, j
    return None, None


def read(words, names=()):
    w = [x.word for x in words]
    start = 0
    while start < len(w) and w[start] in FILLER:
        start += 1
    words, w = words[start:], w[start:]
    if not w or w[0] is None:
        return None
    names = {n.lower() for n in names}
    for reader in (_read_add, _read_camera, _read_light, _read_background, _read_object):
        op = reader(words, w, names)
        if op:
            return op
    return None


def _read_add(words, w, names):
    if w[0] not in ADD_VERBS or len(w) < 2 or w[1] not in ARTICLES:
        return None
    j, colour, size, spin = 2, None, None, None
    while j < len(w):
        found = _colour(w, j) if w[j] not in SHAPE_WORDS else None
        if found:
            colour, j = found[0], j + found[1]
        elif w[j] in SIZES:
            size, j = SIZES[w[j]], j + 1
        elif w[j] in SPIN_WORDS:
            spin, j = 90.0, j + 1
        elif w[j] in ("new", "3d", "solid", "shiny"):
            j += 1
        else:
            break
    if j >= len(w) or w[j] not in SHAPE_WORDS:
        return None
    op = {"op": "add", "kind": SHAPE_WORDS[w[j]], "colour": colour, "size": size, "name": None, "at": None,
          "spin": spin, "view": None}
    j += 1
    while j < len(w):
        if w[j] in ("called", "named") and j + 1 < len(w):
            op["name"] = words[j + 1].text
            j += 2
        elif w[j] == "at":
            op["at"], after = _coordinates(words, w, j + 1)
            j = max(after, j + 1)
        elif w[j] in PLACES and j + 1 < len(w):
            k = j + 1
            while k < len(w) and w[k] in ("the", "3d", "my"):
                k += 1
            op["view"] = w[k] if k < len(w) else None
            j = k + 1
        elif w[j] in SPIN_WORDS or w[j] == "that" and j + 1 < len(w) and w[j + 1] in SPIN_WORDS:
            op["spin"] = 90.0
            j += 1
        else:
            j += 1
    return op


def _read_camera(words, w, names):
    if "camera" not in w and w[0] != "zoom":
        return None
    if w[0] == "stop" or "stop" in w:
        return {"op": "orbit", "speed": 0.0}
    amount, at = _amount(words, w, 1)
    if amount is not None and ("distance" in w or w[0] in MOVE_VERBS | {"set", "pull"} and
                               any(x in w for x in ("away", "units", "distance", "from"))):
        return {"op": "distance", "distance": amount}
    if w[0] == "zoom" or any(d in w for d in ("closer", "nearer", "further", "farther", "away", "back")) \
            and w[0] in MOVE_VERBS | {"pull", "zoom"}:
        out = any(d in w for d in ("out", "further", "farther", "away", "back", "backwards"))
        return {"op": "zoom", "factor": 1.25 if out else 0.8}
    per_second = "second" in w or "sec" in w
    if w[0] in ("orbit", "circle", "spin") or per_second or "round" in w and w[0] in ("make", "send"):
        if amount is None or per_second or w[0] == "spin":
            return {"op": "orbit", "speed": 30.0 if amount is None else amount}
    if w[0] in ROTATE_VERBS | {"orbit", "swing", "pan", "move", "spin", "look"}:
        up = "up" in w or "upwards" in w
        down = "down" in w or "downwards" in w
        left = "left" in w
        if up or down:
            angle = 10.0 if amount is None else amount
            return {"op": "camera", "yaw": 0, "pitch": angle if up else _negative(angle)}
        angle = 15.0 if amount is None else amount
        return {"op": "camera", "yaw": _negative(angle) if left else angle, "pitch": 0}
    return None


def _negative(value):
    if isinstance(value, list):
        return ("-", value)
    if isinstance(value, tuple):
        return value[1]
    return -value


def _read_light(words, w, names):
    for j, word in enumerate(w):
        if word in ("light", "lamp", "sun", "sunlight", "lighting") and not _colour(w, j + 1) and \
                not (j > 0 and _colour(w, j - 1)):
            direction = [0.0, 0.0, 0.0]
            found = False
            for later in w[j + 1:]:
                if later in LIGHT_FROM:
                    direction = [direction[k] + LIGHT_FROM[later][k] for k in range(3)]
                    found = True
            return {"op": "light", "from": direction} if found else None
    return None


def _read_background(words, w, names):
    if "background" not in w or not any(x in w for x in ("view", "scene", "3d", "space")):
        return None
    colours = []
    j = 0
    while j < len(w):
        found = _colour(w, j) if w[j] not in ("background", "view", "scene") else None
        if found:
            colours.append(found[0])
            j += found[1]
        else:
            j += 1
    if not colours:
        return None
    return {"op": "background", "top": colours[0], "bottom": colours[-1]}


def _read_object(words, w, names):
    verb = w[0]
    it = verb in SPIN_WORDS | ROTATE_VERBS | {"stop"} or any(x in w for x in SPIN_WORDS)
    target, t = _target(words, w, names, 1 if verb in ("make", "set", "change") or verb in KINDS else 0, it)
    if target is None:
        return None
    rest = w[t + 1:]
    if verb == "stop" or "stop" in w[:t]:
        return {"op": "stop", "target": target}
    if any(x in w for x in SPIN_WORDS) and verb not in ("hide", "show"):
        speed = 90.0
        if any(x in rest for x in ("slowly", "slow", "gently")):
            speed = 30.0
        elif any(x in rest for x in ("fast", "quickly", "quick", "rapidly")):
            speed = 180.0
        amount, at = _amount(words, w, t + 1)
        if amount is not None:
            speed = amount
            unit = w[at + 1] if at + 1 < len(w) else None
            if unit in ("times", "turns", "turn", "revolutions", "rotations", "spins"):
                speed = amount * 360 if not isinstance(amount, list) else amount
            if "minute" in w and not isinstance(speed, list):
                speed = speed / 60
        return {"op": "spin", "target": target, "speed": speed, "axis": _axis(rest)}
    colour = None
    for j in range(t + 1, len(w)):
        colour = _colour(w, j)
        if colour:
            colour = colour[0]
            break
    if verb in ROTATE_VERBS and colour is None:
        if "upside" in rest:
            return {"op": "rotate", "target": target, "angle": 180.0, "axis": "x", "relative": True}
        amount, at = _amount(words, w, t + 1)
        default_axis = {"tilt": "x", "roll": "z", "flip": "x"}.get(verb, "y")
        angle = 45.0 if amount is None else amount
        if "right" in rest:
            angle = _negative(angle)
        relative = not (at is not None and at > 0 and w[at - 1] == "to")
        return {"op": "rotate", "target": target, "angle": angle, "axis": _axis(rest, default_axis),
                "relative": relative}
    if verb in HIDE_VERBS or any(x in rest for x in ("invisible", "disappear", "vanish")):
        return {"op": "hide", "target": target}
    if verb in SHOW_VERBS or any(x in rest for x in ("visible", "appear", "reappear")):
        return {"op": "show", "target": target}
    if verb in MOVE_VERBS:
        for j, word in enumerate(rest):
            if word in ("to", "at"):
                to, _ = _coordinates(words, w, t + 2 + j)
                if to:
                    return {"op": "move", "target": target, "to": to}
        for j, word in enumerate(rest):
            if word in DIRECTIONS:
                amount, _ = _amount(words, w, t + 1)
                amount = 1 if amount is None else amount
                by = [0, 0, 0]
                for k, d in enumerate(DIRECTIONS[word]):
                    if d:
                        by[k] = amount if d > 0 else _negative(amount)
                return {"op": "move", "target": target, "by": by}
        return None
    scale = _read_scale(words, w, t, rest, verb)
    if scale:
        scale["target"] = target
        return scale
    if colour is not None and verb in ("make", "colour", "color", "paint", "turn", "set", "change", "tint", "dye"):
        return {"op": "colour", "target": target, "colour": colour}
    return None


def _read_scale(words, w, t, rest, verb):
    amount, at = _amount(words, w, t + 1)
    if "size" in w and amount is not None and not any(x in rest for x in ("times", "twice", "half", "double")):
        return {"op": "size", "size": amount}
    if verb == "scale":
        return {"op": "scale", "factor": 2.0 if amount is None else amount}
    if "twice" in rest or "double" in w:
        return {"op": "scale", "factor": 2}
    if "half" in rest:
        return {"op": "scale", "factor": 0.5}
    if "times" in rest and amount is not None:
        return {"op": "scale", "factor": amount}
    if verb in ("grow", "enlarge") or any(x in rest for x in ("bigger", "larger")):
        return {"op": "scale", "factor": 1.25}
    if verb == "shrink" or "smaller" in rest:
        return {"op": "scale", "factor": 0.8}
    for word in rest:
        if word in SIZES and verb in ("make", "set", "change", "turn"):
            return {"op": "size", "size": SIZES[word]}
    return None


# ---------------- writing an op's code ----------------

def emit(compiler, op, view_slot):
    """The code for op (from read) in compiler.a. view_slot: a writable 8-byte label holding the GLVIEW.
    Names in values are worked out with the compiler's own value code. The program must also get
    glspace.add_all(compiler.a) once (bitwin does that; compiler.uses_glspace says it is needed).
    An op on an object that isn't there does nothing."""
    from asm import XMM0, XMM1
    a = compiler.a
    compiler.uses_glspace = True
    done = a.new_label("gl_op_done")

    def value(v):
        """rax = v as a decimal (its 8 bytes)."""
        if isinstance(v, tuple):
            value(v[1])
            a.to_xmm(XMM1, RAX)
            a.set(RAX, 0)
            a.to_xmm(XMM0, RAX)
            a.decimal_maths("sub", XMM0, XMM1)
            a.from_xmm(RAX, XMM0)
        elif isinstance(v, list):
            compiler.emitter.convert(compiler.value(v), "decimal")
        else:
            a.set(RAX, bits(v))

    def view():
        a.address(RAX, view_slot)
        a.load(RAX, RAX)

    def target(t):
        """rax = the object, or on to the end when there's none."""
        view()
        if t.get("last"):
            a.call("gl_last")
        else:
            if t.get("name"):
                a.address(RCX, a.text(t["name"]))
                a.call("gl_find")
            if t.get("kind"):
                found = a.new_label("gl_got")
                if t.get("name"):
                    a.op("test", RAX, RAX)
                    a.jump_if(NOT_EQUAL, found)
                    view()
                a.set(RCX, KINDS[t["kind"]])
                a.call("gl_find_kind")
                a.label(found)
        a.op("test", RAX, RAX)
        a.jump_if(EQUAL, done)

    def put(offset, v):
        """The object (rax) field = a fixed number, keeping rax."""
        a.set(RCX, v if isinstance(v, int) and not isinstance(v, bool) and offset in (O_COLOUR, O_VISIBLE, O_KIND)
              else bits(v))
        a.store(RAX, offset, RCX)

    kind = op["op"]
    axes = {"x": 0, "y": 1, "z": 2}
    if kind == "add":
        values = list(op["at"] or []) + ([op["size"]] if op["size"] is not None else [])
        for v in values:
            value(v)
            a.push(RAX)
        view()
        a.set(RCX, KINDS[op["kind"]])
        a.call("gl_add")
        if op["size"] is not None:
            a.pop(RCX)
            for offset in (O_SX, O_SY, O_SZ):
                a.store(RAX, offset, RCX)
        if op["at"]:
            for offset in (O_Z, O_Y, O_X):
                a.pop(RCX)
                a.store(RAX, offset, RCX)
        if op["colour"] is not None:
            put(O_COLOUR, op["colour"])
        if op["name"]:
            a.address(RCX, a.text(op["name"]))
            a.store(RAX, O_NAME, RCX)
        if op.get("spin"):
            put(O_SPY, float(op["spin"]))
            a.call("gl_check_animating")
    elif kind in ("spin", "rotate", "scale", "size"):
        value(op.get("speed", op.get("angle", op.get("factor", op.get("size")))))
        a.push(RAX)
        target(op["target"])
        a.pop(RCX)
        a.to_xmm(XMM1, RCX)
        if kind == "spin":
            a.store(RAX, O_SPX + 8 * axes[op["axis"]], RCX)
            a.call("gl_check_animating")
        elif kind == "rotate":
            offset = O_RX + 8 * axes[op["axis"]]
            if op["relative"]:
                a.load_decimal(XMM0, RAX, offset)
                a.decimal_maths("add", XMM1, XMM0)
            a.store_decimal(RAX, offset, XMM1)
        else:
            for offset in (O_SX, O_SY, O_SZ):
                if kind == "scale":
                    a.load_decimal(XMM0, RAX, offset)
                    a.decimal_maths("mul", XMM0, XMM1)
                    a.store_decimal(RAX, offset, XMM0)
                else:
                    a.store_decimal(RAX, offset, XMM1)
    elif kind == "stop":
        target(op["target"])
        for offset in (O_SPX, O_SPY, O_SPZ):
            put(offset, 0.0)
        a.call("gl_check_animating")
    elif kind == "move":
        values = op.get("to") or op.get("by")
        for v in values:
            value(v)
            a.push(RAX)
        target(op["target"])
        for offset in (O_Z, O_Y, O_X):
            a.pop(RCX)
            if "by" in op:
                a.to_xmm(XMM1, RCX)
                a.load_decimal(XMM0, RAX, offset)
                a.decimal_maths("add", XMM0, XMM1)
                a.store_decimal(RAX, offset, XMM0)
            else:
                a.store(RAX, offset, RCX)
    elif kind == "colour":
        target(op["target"])
        put(O_COLOUR, op["colour"])
    elif kind in ("hide", "show"):
        target(op["target"])
        put(O_VISIBLE, 1 if kind == "show" else 0)
        a.call("gl_check_animating")
    elif kind == "camera":
        value(op["yaw"])
        a.push(RAX)
        value(op["pitch"])
        a.mov(RDX, RAX)
        a.pop(RCX)
        a.push(RDX)
        view()
        a.pop(RDX)
        a.call("gl_turn_camera")
    elif kind == "zoom":
        view()
        a.set(RCX, bits(op["factor"]))
        a.call("gl_zoom")
    elif kind in ("distance", "orbit"):
        value(op.get("distance", op.get("speed")))
        a.mov(RCX, RAX)
        a.push(RCX)
        view()
        a.pop(RCX)
        if kind == "distance":
            a.to_xmm(XMM0, RCX)
            a.set(RCX, bits(0.01))
            a.to_xmm(XMM1, RCX)
            a.decimal_max(XMM0, XMM1)
            a.store_decimal(RAX, V_DIST, XMM0)
        else:
            a.store(RAX, V_ORBIT, RCX)
            a.call("gl_check_animating")
    elif kind == "light":
        view()
        for n, offset in enumerate((V_LX, V_LY, V_LZ)):
            a.set(RCX, bits(op["from"][n]))
            a.store(RAX, offset, RCX)
    elif kind == "background":
        view()
        for offset, colour in ((V_TOP, op["top"]), (V_BOTTOM, op["bottom"]), (V_BACKGROUND, 0)):
            a.set(RCX, colour)
            a.store(RAX, offset, RCX)
    else:
        raise ValueError(f"glspace.emit: no such op {kind!r}")
    a.label(done)
