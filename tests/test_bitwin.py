"""Tests for bitwin's element runtime (bitwin.py), run against the fake compositor in wayland_stub.py.

The programs are built straight from machine code (asm.Asm): runtime + wayland + paint (+ glspace) + bitwin,
calling the bw_ routines directly, so these tests don't depend on the English. Handlers write a word on the
program's output ("clicked start"), so a test can see what ran.

Usage: python3 tests/test_bitwin.py            (SAVE=folder also saves every checked frame as PNG there)
"""
import os
import queue
import struct
import subprocess
import sys
import tempfile
import threading
import time

HERE = os.path.dirname(os.path.abspath(__file__))
sys.path.insert(0, os.path.dirname(HERE))
sys.path.insert(0, HERE)
os.environ.setdefault("BRICK_MEMORY", "off")
os.environ.setdefault("BRICK_TRAINER", "off")

import bitwin  # noqa: E402
import elf  # noqa: E402
import paint  # noqa: E402
import runtime  # noqa: E402
import wayland  # noqa: E402
from asm import R8, R12, RAX, RCX, RDI, RDX, Asm  # noqa: E402
from wayland_stub import Stub  # noqa: E402

try:
    import glspace  # noqa: E402
except ImportError:
    glspace = None

FOLDER = tempfile.mkdtemp(prefix="bricks-bitwin-")
SAVE = os.environ.get("SAVE")
B = bitwin


class Program:
    """A program built from machine code, step by step. Element numbers are kept in named slots."""

    def __init__(self, gl=False):
        self.a = a = Asm()
        self.gl = gl and glspace is not None
        runtime.start(a)
        self.slots = {}
        self.handlers = []

    # -- helpers that emit code --
    def slot(self, name):
        if name not in self.slots:
            self.slots[name] = self.a.reserve(8, "slot")
        return self.slots[name]

    def open(self, title="test", w=400, h=300, resizable=0):
        a = self.a
        a.address(RAX, a.text(title))
        a.set(RCX, w)
        a.set(RDX, h)
        a.set(R8, resizable)
        a.call("bw_open")

    def new(self, name, kind, text=None):
        a = self.a
        a.set(RAX, kind)
        a.address(RCX, a.text(name))
        a.call("bw_new")
        a.address(RCX, self.slot(name))
        a.store(RCX, 0, RAX)
        if text is not None:
            self.text(name, text)

    def elem(self, name, reg=RDI):
        """reg = the record of the named element (changes rax, r11)."""
        a = self.a
        a.address(RAX, self.slot(name))
        a.load(RAX, RAX)
        a.call("bw_elem")
        if reg != RAX:
            a.mov(reg, RAX)

    def index(self, name, reg=RAX):
        self.a.address(reg, self.slot(name))
        self.a.load(reg, reg)

    def text(self, name, text):
        a = self.a
        self.index(name)
        a.address(RCX, a.text(text))
        a.call("bw_set_text")

    def set(self, name, **fields):
        """Record fields by offset name, e.g. set("start", XV=20, XU=0, STYLE_COL=0xFF0000)."""
        a = self.a
        self.elem(name)
        for field, value in fields.items():
            off = sum(getattr(B, part) for part in field.split("_"))
            a.set(RAX, value)
            a.store(RDI, off, RAX)

    def place(self, name, x, y, w=None, h=None, units=(0, 0, 0, 0)):
        fields = dict(XV=x, XU=units[0], YV=y, YU=units[1])
        if w is not None:
            fields.update(WV=w, WU=units[2])
        if h is not None:
            fields.update(HV=h, HU=units[3])
        self.set(name, **fields)

    def handler(self, word):
        """-> the label of a routine that writes word and a newline."""
        label = self.a.new_label("handler")
        self.handlers.append((label, word))
        return label

    def on(self, name, field, word):
        a = self.a
        self.elem(name)
        a.address(RAX, self.handler(word))
        a.store(RDI, getattr(B, field), RAX)

    def window_handler(self, label_name, word):
        a = self.a
        a.address(RAX, self.handler(word))
        a.address(RCX, label_name)
        a.store(RCX, 0, RAX)

    def item(self, name, text, word=None):
        a = self.a
        self.index(name)
        a.address(RCX, a.text(text))
        a.call("bw_add_item")
        if word:
            a.mov(RCX, RAX)
            self.index(name)
            a.address(RDX, self.handler(word))
            a.call("bw_item_handler")

    def call(self, routine, *names):
        a = self.a
        for reg, name in zip((RAX, RCX), names):
            if isinstance(name, int):
                a.set(reg, name)
            else:
                a.address(reg, self.slot(name))
                a.load(reg, reg)
        a.call(routine)

    def find(self, name):
        """rax = bw_find(name text), with the capitals changed to show they don't matter."""
        self.a.address(RAX, self.a.text(name.upper()))
        self.a.call("bw_find")

    def code_handler(self, emit):
        """-> the label of a routine whose body emit(a) writes (it gets a ret after)."""
        label = self.a.new_label("handler")
        self.handlers.append((label, emit))
        return label

    def say(self, word):
        self.a.address(RAX, self.a.text(word + "\n"))
        self.a.call("write_text")

    def build(self, name):
        a = self.a
        a.set(RAX, 60)
        a.set(RDI, 0)
        a.syscall()
        for label, word in self.handlers:
            a.label(label)
            if callable(word):
                word(a)
            else:
                a.address(RAX, a.text(word + "\n"))
                a.call("write_text")
            a.ret()
        runtime.add_all(a)
        wayland.add_all(a)
        paint.add_all(a)
        if self.gl:
            glspace.add_all(a)
        bitwin.add_all(a)
        path = os.path.join(FOLDER, name)
        with open(path, "wb") as f:
            f.write(elf.wrap(a))
        os.chmod(path, 0o755)
        return path


class Run:
    """The program running under the stub; its output lines arrive in a queue."""

    def __init__(self, path, **stub_options):
        self.stub = Stub(**stub_options).start()
        env = dict(self.stub.env, BRICK_APP_ID="brick-test")
        self.process = subprocess.Popen([path], env=env, stdout=subprocess.PIPE, stderr=subprocess.STDOUT)
        self.lines = queue.Queue()
        self.seen = []
        threading.Thread(target=self._read, daemon=True).start()

    def _read(self):
        for line in self.process.stdout:
            self.lines.put(line.decode(errors="replace").rstrip("\n"))
        self.lines.put(None)

    def expect(self, word, timeout=5.0):
        end = time.time() + timeout
        while time.time() < end:
            try:
                line = self.lines.get(timeout=max(0.01, end - time.time()))
            except queue.Empty:
                break
            if line is None:
                break
            self.seen.append(line)
            if line == word:
                return
        raise AssertionError(f"never saw {word!r}; saw {self.seen}")

    def quiet(self, seconds=0.3):
        """-> the lines written in the next moment (none expected, usually)."""
        time.sleep(seconds)
        got = []
        while not self.lines.empty():
            line = self.lines.get()
            if line is not None:
                got.append(line)
        return got

    def frames(self, n=1, timeout=5.0):
        return self.stub.wait_frames(n, timeout)

    def settle(self, timeout=3.0):
        """Wait until no new frame has come for a moment; -> the frame count."""
        end = time.time() + timeout
        count = len(self.stub.frames)
        while time.time() < end:
            time.sleep(0.12)
            if len(self.stub.frames) == count:
                return count
            count = len(self.stub.frames)
        return count

    def pixel(self, x, y):
        return self.stub.pixel(x, y)

    def save(self, name):
        if SAVE:
            os.makedirs(SAVE, exist_ok=True)
            self.stub.save_png(os.path.join(SAVE, name + ".png"))

    def finish(self, timeout=5.0):
        try:
            return self.process.wait(timeout)
        finally:
            self.stub.stop()

    def __enter__(self):
        return self

    def __exit__(self, *problem):
        if self.process.poll() is None:
            self.process.kill()
            self.process.wait()
        self.stub.stop()


def near(colour, expected, slack=24):
    return all(abs((colour >> s & 255) - (expected >> s & 255)) <= slack for s in (16, 8, 0))


# ---- the tests ----

RED, GREEN, BLUE, GOLD, PINK, TEAL = 0xE04040, 0x40C060, 0x4060E0, 0xE0B040, 0xE070C0, 0x40C0C0


def box(p, name, colour, x, y, w, h, units=(0, 0, 0, 0)):
    p.new(name, B.BOX)
    p.place(name, x, y, w, h, units)
    p.set(name, STYLE_COL=colour, STYLE_RADIUS=0, STYLE_BORDER=0)


def step(p):
    """The program says "ready" and waits for a click anywhere (the test's cue to go on)."""
    p.say("ready")
    p.call("bw_wait_click", -1)


class Steps(Run):
    def next(self, click=(2, 298)):
        """After checking a step: click (on the bare window) to let the program go on."""
        self.stub.click(*click)

    def ready(self):
        count = len(self.stub.frames)
        self.expect("ready")
        self.stub.wait_until(lambda: len(self.stub.frames) > count, 3)
        self.settle()


def colour_at(r, x, y, expected, name):
    got = r.pixel(x, y)
    assert near(got, expected), f"{name}: at {x},{y} expected {expected:06x}, got {got:06x}"


def test_smoke():
    p = Program()
    p.open("smoke", 400, 300)
    p.new("start", B.BUTTON, "Start")
    p.new("hello", B.TEXTK, "Hello, bitwin!")
    p.new("board", B.CANVAS)
    p.call("bw_run")
    p.say("after run")
    with Run(p.build("smoke")) as r:
        r.frames(1)
        r.settle()
        r.save("smoke")
        assert near(r.pixel(395, 295), B.WIN_BG, 2), hex(r.pixel(395, 295))
        # the button (flowing at 20 20) is there, with a lighter top than bottom
        top, bottom = r.pixel(30, 24), r.pixel(30, 50)
        assert top != B.WIN_BG and sum(top.to_bytes(3, "big")) > sum(bottom.to_bytes(3, "big")), (hex(top), hex(bottom))
        r.stub.close()
        r.expect("after run")
        assert r.finish() == 0


def test_units():
    p = Program()
    p.open("units", 400, 300)
    box(p, "a", RED, 10, 10, 40, 30)                                  # px
    box(p, "b", GREEN, 50, 0, 10, 10, (1, 0, 1, 1))                   # percent: 200,0 40x30
    box(p, "c", BLUE, 10, 10, 40, 30, (2, 2, 0, 0))                   # far edge: 350..390, 260..290
    box(p, "d", GOLD, 0, 0, 40, 30, (3, 3, 0, 0))                     # centred: 180..220, 135..165
    box(p, "e", PINK, 0, 0, 20, 30, (4, 6, 0, 6))                     # after a, same y, same h
    p.set("e", OTHER=0, GAP=5)
    p.find("a")
    p.a.mov(R12, RAX)
    p.elem("e", RDX)
    p.a.store(RDX, B.OTHER, R12)
    box(p, "f", TEAL, 0, 0, 125, 100, (0, 0, 8, 8))                   # per mille: 50 x 30 at 0,0 ... moved below
    p.set("f", YV=200)
    box(p, "g", PINK, 0, 0, 30, 10, (5, 6, 0, 0))                     # before c, aligned with c's y + 0
    p.find("c")
    p.a.mov(R12, RAX)
    p.elem("g", RDX)
    p.a.store(RDX, B.OTHER, R12)
    p.set("g", GAP=10)
    box(p, "panel", 0x303030, 100, 180, 120, 80)                      # a parent
    box(p, "inner", GREEN, 10, 10, 50, 50, (0, 0, 2, 2))              # inside it: 10 in, far edges 50 in
    p.find("panel")
    p.a.mov(R12, RAX)
    p.elem("inner", RDX)
    p.a.store(RDX, B.PARENT, R12)
    p.new("fit", B.BUTTON, "A much longer label")
    p.place("fit", 230, 200)
    p.call("bw_run")
    with Run(p.build("units")) as r:
        r.frames(1)
        r.settle()
        r.save("units")
        colour_at(r, 12, 12, RED, "px")
        colour_at(r, 48, 38, RED, "px far corner")
        colour_at(r, 202, 2, GREEN, "percent")
        colour_at(r, 238, 28, GREEN, "percent far corner")
        assert not near(r.pixel(241, 2), GREEN), "percent too wide"
        colour_at(r, 352, 262, BLUE, "far edge")
        colour_at(r, 388, 288, BLUE, "far edge corner")
        assert not near(r.pixel(391, 289), BLUE), "far edge too far"
        colour_at(r, 181, 136, GOLD, "centred")
        colour_at(r, 218, 163, GOLD, "centred corner")
        colour_at(r, 56, 12, PINK, "after")
        colour_at(r, 74, 38, PINK, "after, same height")
        assert not near(r.pixel(53, 12), PINK), "after: no gap"
        colour_at(r, 2, 202, TEAL, "per mille")
        colour_at(r, 48, 228, TEAL, "per mille corner")
        assert not near(r.pixel(52, 202), TEAL), "per mille too wide"
        colour_at(r, 312, 262, PINK, "before")
        colour_at(r, 338, 268, PINK, "before, right edge")
        assert not near(r.pixel(342, 262), PINK), "before: no gap"
        colour_at(r, 112, 192, GREEN, "inside a parent")
        colour_at(r, 178, 218, GREEN, "inside, far edges")
        assert not near(r.pixel(182, 218), GREEN) and not near(r.pixel(150, 222), GREEN), "inside: far edge"
        # fit: the button is wider than its long label
        row = [r.pixel(x, 210) for x in range(230, 400)]
        width = next((i for i, c in enumerate(row) if near(c, B.WIN_BG, 3)), len(row))
        assert width > 140, width


def test_depth():
    p = Program()
    p.open("depth", 300, 200)
    box(p, "red", RED, 20, 20, 100, 100)
    box(p, "blue", BLUE, 60, 60, 100, 100)
    box(p, "green", GREEN, 100, 30, 100, 100)
    step(p)
    p.call("bw_front", "red")
    step(p)
    p.call("bw_back", "red")
    step(p)
    p.call("bw_in_front_of", "red", "blue")
    step(p)
    p.call("bw_behind", "green", "blue")
    step(p)
    with Steps(p.build("depth")) as r:
        def check(name, red_blue, blue_green, red_blue_green):
            colour_at(r, 80, 80, red_blue, name + " (red/blue)")
            colour_at(r, 140, 110, blue_green, name + " (blue/green)")
            colour_at(r, 110, 90, red_blue_green, name + " (all three)")
        r.ready()
        check("creation order", BLUE, GREEN, GREEN)
        r.next()
        r.ready()
        check("red to the front", RED, GREEN, RED)
        r.save("depth-front")
        r.next()
        r.ready()
        check("red to the back", BLUE, GREEN, GREEN)
        r.next()
        r.ready()
        check("red just in front of blue", RED, GREEN, GREEN)
        r.next()
        r.ready()
        check("green just behind blue", RED, BLUE, RED)


def test_hover_press_click():
    p = Program()
    p.open("click", 300, 200)
    p.new("go", B.BUTTON, "Go")
    p.place("go", 20, 20, 100, 40)
    p.on("go", "CLICK", "clicked go")
    p.call("bw_run")
    with Run(p.build("click")) as r:
        r.frames(1)
        r.settle()
        plain = r.pixel(30, 50)
        r.stub.move(30, 50)
        r.settle()
        hover = r.pixel(30, 50)
        r.save("hover")
        r.stub.button(1, True)
        r.settle()
        pressed = r.pixel(30, 50)
        r.save("pressed")
        assert sum(hover.to_bytes(3, "big")) > sum(plain.to_bytes(3, "big")), (hex(plain), hex(hover))
        assert sum(pressed.to_bytes(3, "big")) < sum(plain.to_bytes(3, "big")), (hex(plain), hex(pressed))
        assert r.quiet() == []
        r.stub.button(1, False)
        r.expect("clicked go")
        r.settle()
        assert r.pixel(30, 50) == hover
        # press on the button, let go somewhere else: no click
        r.stub.button(1, True)
        r.stub.move(200, 150)
        r.stub.button(1, False)
        assert r.quiet() == []
        r.settle()
        assert r.pixel(30, 50) == plain


def test_click_frontmost():
    p = Program()
    p.open("front", 300, 200)
    p.new("back", B.BUTTON, "Back")
    p.place("back", 20, 20, 150, 80)
    p.on("back", "CLICK", "clicked back")
    p.new("front", B.BUTTON, "Front")
    p.place("front", 100, 60, 150, 80)
    p.on("front", "CLICK", "clicked front")
    p.new("label", B.TEXTK, "a text over it")       # texts don't catch clicks unless they do something
    p.place("label", 110, 100)
    p.call("bw_run")
    with Run(p.build("front")) as r:
        r.frames(1)
        r.settle()
        r.stub.click(120, 80)                        # both: the front one only
        r.expect("clicked front")
        assert r.quiet() == []
        r.stub.click(40, 30)
        r.expect("clicked back")
        r.stub.click(130, 105)                       # on the text, over front
        r.expect("clicked front")
        r.stub.click(280, 190)                       # nothing there
        assert r.quiet() == []


def test_menus():
    p = Program()
    p.open("menus", 400, 300)
    p.new("file", B.MENU, "File")
    p.item("file", "New", "chose new")
    p.item("file", "Open", "chose open")
    p.item("file", "Quit", "chose quit")
    p.new("edit", B.MENU, "Edit")
    p.item("edit", "Copy", "chose copy")
    box(p, "under", GREEN, 0, 40, 300, 200)             # the list opens in front of this
    p.call("bw_run")
    with Run(p.build("menus")) as r:
        r.frames(1)
        r.settle()
        r.save("menus-closed")
        bar = r.pixel(200, 5)
        assert near(bar, B.MENU_BG, 3), hex(bar)
        r.stub.click(15, 10)                         # open File
        r.settle()
        r.save("menus-open")
        assert near(r.pixel(60, 60), B.LIST_BG, 6), hex(r.pixel(60, 60))
        r.stub.move(60, 80)                          # hover the second item
        r.settle()
        r.save("menus-hover")
        assert near(r.pixel(100, 80), B.ACCENT, 10), hex(r.pixel(100, 80))
        r.stub.click(60, 80)
        r.expect("chose open")
        r.settle()
        colour_at(r, 60, 60, GREEN, "list closed")
        # Edit sits right of File
        r.stub.click(70, 10)
        r.settle()
        r.stub.click(80, 45)
        r.expect("chose copy")
        # open, then press elsewhere: closes without running anything
        r.stub.click(15, 10)
        r.settle()
        r.stub.click(350, 280)
        assert r.quiet() == []
        r.settle()
        colour_at(r, 60, 60, GREEN, "closed by a press elsewhere")


def test_tabs():
    p = Program()
    p.open("tabs", 400, 300)
    p.new("pages", B.TABS)
    for name in ("Home", "Settings", "About"):
        p.item("pages", name)
    p.on("pages", "CHANGE", "page changed")
    box(p, "home", RED, 20, 60, 100, 100)
    box(p, "settings", BLUE, 20, 60, 100, 100)
    p.set("home", PAGE=0)
    p.set("settings", PAGE=0)
    p.find("pages")                       # page fields: tabs index * 65536 + page + 1
    p.a.mov(RCX, RAX)
    p.a.shift_left(RCX, 16)
    p.a.op_n("add", RCX, 1)
    p.elem("home", RDX)
    p.a.store(RDX, B.PAGE, RCX)
    p.a.op_n("add", RCX, 1)
    p.elem("settings", RDX)
    p.a.store(RDX, B.PAGE, RCX)
    p.call("bw_run")
    with Run(p.build("tabs")) as r:
        r.frames(1)
        r.settle()
        r.save("tabs-home")
        colour_at(r, 50, 100, RED, "the first page")
        r.stub.click(110, 15)                        # "Settings" (Home is about 75 wide)
        r.expect("page changed")
        r.settle()
        r.save("tabs-settings")
        colour_at(r, 50, 100, BLUE, "the second page")
        r.stub.click(110, 15)                        # the same tab again: no change
        assert r.quiet() == []


def test_input_box():
    p = Program()
    p.open("input", 400, 200)
    p.new("name", B.INPUT)
    p.place("name", 20, 20)
    shout = p.a.new_label("shout")
    p.elem("name")
    p.a.address(RAX, shout)
    p.a.store(RDI, B.CHANGE, RAX)
    p.call("bw_run")
    # the on change handler writes the box's text
    a = p.a
    after = a.new_label("after")
    a.jump(after)
    a.label(shout)
    p.elem("name")
    a.load(RAX, RDI, B.TEXT)
    a.call("write_text")
    p.say("")
    a.ret()
    a.label(after)
    with Run(p.build("input")) as r:
        r.frames(1)
        r.settle()
        r.stub.click(60, 35)
        r.settle()
        r.stub.type_text("Hellp")
        r.stub.key("backspace")
        r.stub.type_text("o wrld")
        for _ in range(3):
            r.stub.key("left")
        r.stub.type_text("o")
        r.stub.key("end")
        r.stub.type_text("!")
        r.stub.key("home")
        r.stub.key("delete")
        r.stub.type_text("J")
        r.settle()
        r.save("input")
        assert near(r.pixel(20, 30), B.ACCENT, 10), hex(r.pixel(20, 30))   # focused: accent border
        r.stub.key("enter")
        r.expect("Jello world!")


def test_canvas_shapes():
    p = Program()
    p.open("canvas", 400, 300)
    p.new("board", B.CANVAS)
    p.place("board", 50, 50, 300, 200)
    a = p.a
    shape = a.reserve(64, "shape")

    def draw(*fields):
        a.address(RDI, shape)
        for k, value in enumerate(fields + (0,) * (8 - len(fields))):
            a.set(RAX, value)
            a.store(RDI, 8 * k, RAX)
        p.index("board")
        a.address(RCX, shape)
        a.call("bw_draw")

    draw(1, 10, 10, 60, 40, 0, RED)                  # box at 60,60
    draw(2, 100, 10, 60, 60, 0, GREEN)               # ellipse
    draw(3, 0, 190, 300, 190, BLUE, 4, 0)            # line along the bottom
    draw(4 + 256 * 0, 200, 20, 280, 20, 240, 100, GOLD)   # triangle
    draw(6, 30, 150, 12, PINK)                       # dot
    draw(1, 290, 100, 100, 50, 0, TEAL)              # sticks out: clipped to the canvas
    for k in range(30):                              # more than the first list holds
        draw(6, 20 + k * 8, 120, 4, GOLD)
    p.new("hi", B.TEXTK, "x")
    step(p)
    p.call("bw_clear", "board")
    step(p)
    with Steps(p.build("canvas")) as r:
        r.ready()
        r.save("canvas")
        colour_at(r, 62, 62, RED, "box")
        colour_at(r, 180, 90, GREEN, "ellipse centre")
        assert not near(r.pixel(152, 62), GREEN), "ellipse corner"
        colour_at(r, 200, 240, BLUE, "line")
        colour_at(r, 290, 80, GOLD, "triangle")
        colour_at(r, 80, 200, PINK, "dot")
        colour_at(r, 345, 170, TEAL, "clipped box inside")
        colour_at(r, 360, 170, B.WIN_BG, "clipped box outside")
        colour_at(r, 70 + 29 * 8, 170, GOLD, "the 36th shape")
        r.next()
        r.ready()
        colour_at(r, 62, 62, B.CANVAS_BG, "cleared")


def test_gl_view():
    if glspace is None:
        print("(no glspace.py: skipped)")
        return
    p = Program(gl=True)
    p.open("gl", 300, 250)
    p.new("view", B.GL)
    p.place("view", 20, 20, 200, 200)
    p.call("bw_run")
    with Run(p.build("gl")) as r:
        r.frames(1)
        r.settle()
        r.save("gl")
        inside = r.pixel(120, 120)
        assert not near(inside, B.WIN_BG, 2), hex(inside)
        colour_at(r, 250, 120, B.WIN_BG, "outside the view")


def test_resize():
    p = Program()
    p.open("resize", 400, 300, resizable=1)
    box(p, "half", RED, 0, 0, 50, 100, (0, 0, 1, 1))         # left half
    box(p, "corner", BLUE, 10, 10, 40, 40, (2, 2, 0, 0))     # bottom right corner
    p.window_handler("bw_on_resize", "resized")
    p.call("bw_run")
    with Run(p.build("resize")) as r:
        r.frames(1)
        r.settle()
        colour_at(r, 198, 150, RED, "half")
        colour_at(r, 380, 280, BLUE, "corner")
        r.stub.resize(600, 400)
        r.expect("resized")
        r.settle()
        r.save("resized")
        w, h, _ = r.stub.frames[-1]
        assert (w, h) == (600, 400), (w, h)
        colour_at(r, 298, 350, RED, "half after")
        assert not near(r.pixel(302, 350), RED)
        colour_at(r, 580, 380, BLUE, "corner after")


def test_close_and_timer():
    p = Program()
    p.open("timer", 200, 100)
    p.window_handler("bw_on_close", "bye")
    p.window_handler("bw_every", "tick")
    p.a.set(RAX, 100)
    p.a.address(RCX, "bw_every_ms")
    p.a.store(RCX, 0, RAX)
    p.call("bw_run")
    p.say("after run")
    with Run(p.build("timer")) as r:
        r.frames(1)
        start = time.time()
        for _ in range(5):
            r.expect("tick")
        took = time.time() - start
        assert 0.35 < took < 0.9, took
        r.stub.close()
        r.expect("bye")
        r.expect("after run")
        assert r.finish() == 0


def test_wait_and_reentry():
    """bw_wait keeps the window alive; a handler may wait itself; bw_wait_click waits for one element."""
    p = Program()
    p.open("wait", 300, 200)
    p.new("a", B.BUTTON, "A")
    p.place("a", 20, 20, 80, 40)
    p.new("b", B.BUTTON, "B")
    p.place("b", 150, 20, 80, 40)
    slow = p.a.new_label("slow")
    p.elem("a")
    p.a.address(RAX, slow)
    p.a.store(RDI, B.CLICK, RAX)
    p.on("b", "CLICK", "b clicked")
    p.a.set(RAX, 300)
    p.a.call("bw_wait")
    p.say("waited")
    p.call("bw_wait_click", "b")
    p.say("b was clicked")
    p.call("bw_run")
    a = p.a
    after = a.new_label("after")
    a.jump(after)
    a.label(slow)                                   # a's handler waits 200 ms, then speaks
    a.set(RAX, 200)
    a.call("bw_wait")
    p.say("slow done")
    a.ret()
    a.label(after)
    with Run(p.build("wait")) as r:
        r.expect("waited")
        r.settle()
        r.stub.click(40, 40)                        # a: not the one waited for
        r.stub.click(170, 40)                       # b, clicked while a's handler still waits
        r.expect("b clicked")
        r.expect("slow done")
        r.expect("b was clicked")
        r.stub.close()
        assert r.finish() == 0


def test_text_wrapping():
    p = Program()
    p.open("wrap", 300, 200)
    p.new("story", B.TEXTK, "The quick brown fox jumps over the lazy dog, again and again.")
    p.place("story", 20, 20, 120, None)
    p.new("after", B.BOX)
    p.set("after", WV=100, HV=10, STYLE_COL=RED, STYLE_RADIUS=0, STYLE_BORDER=0)
    p.call("bw_run")
    with Run(p.build("wrap")) as r:
        r.frames(1)
        r.settle()
        r.save("wrap")
        rows = [y for y in range(20, 200) if near(r.pixel(22, y), RED)]
        assert rows and rows[0] > 20 + 3 * 18, rows[:3]           # the box flows below 4+ lines
        lit = [x for x in range(140, 300) for y in range(20, 120) if near(r.pixel(x, y), B.TEXT_COL, 60)]
        assert not lit, "text past its width"


def test_scale():
    """On a 1.6 screen everything is 1.6 times bigger in real pixels; the program's numbers stay logical."""
    p = Program()
    p.open("scale", 300, 200)
    box(p, "a", RED, 10, 10, 50, 50)
    p.on("a", "CLICK", "clicked a")
    p.call("bw_run")
    with Run(p.build("scale"), scale=192, fractional=True) as r:
        r.frames(1)
        r.settle()
        w, h, _ = r.stub.frames[-1]
        assert (w, h) == (480, 320), (w, h)
        colour_at(r, 17, 17, RED, "scaled start")
        colour_at(r, 94, 94, RED, "scaled end")
        assert not near(r.pixel(97, 50), RED)
        r.stub.click(55, 55)                        # logical, as the compositor says it
        r.expect("clicked a")


def write_number(label):
    """Handler body: write the 8-byte number at label, and a newline."""
    def emit(a):
        a.address(RAX, label)
        a.load(RAX, RAX)
        a.call("number_to_text")
        a.call("write_text")
        a.address(RAX, a.text("\n"))
        a.call("write_text")
    return emit


def write_text_at(label):
    def emit(a):
        a.address(RAX, label)
        a.load(RAX, RAX)
        a.call("write_text")
        a.address(RAX, a.text("\n"))
        a.call("write_text")
    return emit


def test_hidden_and_disabled():
    p = Program()
    p.open("hidden", 300, 200)
    box(p, "under", GREEN, 20, 20, 100, 60)
    p.on("under", "CLICK", "clicked under")
    p.new("cover", B.BUTTON, "Cover")
    p.place("cover", 20, 20, 100, 60)
    p.on("cover", "CLICK", "clicked cover")
    p.new("off", B.BUTTON, "Off")
    p.place("off", 150, 20, 100, 60)
    p.on("off", "CLICK", "clicked off")
    p.set("off", FLAGS=B.DISABLED)
    step(p)
    p.set("cover", FLAGS=B.HIDDEN)
    p.call("bw_changed")
    p.call("bw_run")
    with Steps(p.build("hidden")) as r:
        r.ready()
        r.save("disabled")
        assert not near(r.pixel(30, 30), GREEN), hex(r.pixel(30, 30))
        r.stub.click(50, 50)
        r.expect("clicked cover")                    # (and the program hides it)
        r.settle()
        colour_at(r, 30, 30, GREEN, "hidden cover")
        r.stub.click(200, 50)                        # disabled: nothing
        assert r.quiet() == []
        r.stub.click(50, 50)
        r.expect("clicked under")


def test_window_handlers():
    p = Program()
    p.open("handlers", 300, 200)
    p.a.address(RAX, p.code_handler(write_number("bw_key")))
    p.a.address(RCX, "bw_on_key")
    p.a.store(RCX, 0, RAX)
    p.new("pages", B.TABS)
    p.item("pages", "One")
    p.item("pages", "Two")
    p.elem("pages")
    p.a.address(RAX, p.code_handler(write_text_at("bw_chosen")))
    p.a.store(RDI, B.CHANGE, RAX)
    step(p)
    p.a.address(RAX, p.code_handler(write_number("bw_mouse_x")))
    p.a.address(RCX, "bw_on_move")
    p.a.store(RCX, 0, RAX)
    p.call("bw_run")
    with Steps(p.build("handlers"), scale=192, fractional=True) as r:
        r.ready()
        r.stub.key("a")
        r.expect("97")
        r.stub.key("left")
        r.expect("1001")
        r.stub.click(80, 15)                         # "Two" (One is about 60 wide)
        r.expect("Two")                              # (a click: the program goes on to its on move)
        r.settle()
        r.stub.move(123, 150)
        r.expect("123")                              # logical, even on a 1.6 screen


def test_wait_without_window():
    p = Program()
    p.a.set(RAX, 300)
    p.a.call("bw_wait")
    p.say("done")
    path = p.build("nowindow")
    start = time.time()
    done = subprocess.run([path], capture_output=True, timeout=5, env={"PATH": "/usr/bin:/bin"})
    assert done.stdout == b"done\n" and done.returncode == 0, done
    assert time.time() - start >= 0.3


def test_no_desktop():
    p = Program()
    p.open("none", 200, 100)
    p.say("should not get here")
    path = p.build("none")
    empty = tempfile.mkdtemp(prefix="no-wayland-")
    env = {"XDG_RUNTIME_DIR": empty, "PATH": "/usr/bin:/bin"}
    done = subprocess.run([path], env=env, capture_output=True, timeout=5)
    assert done.returncode == 1, done.returncode
    assert done.stdout.decode() == B.MESSAGE_NO_DESKTOP, done.stdout


def test_size_of_bitwin():
    """Programs without windows never get these routines; with them, report how much they add."""
    a = Asm()
    runtime.start(a)
    runtime.add_all(a)
    wayland.add_all(a)
    paint.add_all(a)
    before = len(a.code) + sum(len(v) for v in a.data.values())
    bitwin.add_all(a)
    after = len(a.code) + sum(len(v) for v in a.data.values())
    print(f"(bitwin adds {after - before} bytes)")
    assert after - before < 40000


def main():
    tests = [(name, f) for name, f in globals().items() if name.startswith("test_")]
    only = sys.argv[1:]
    passed = 0
    for name, f in tests:
        if only and name not in only:
            continue
        try:
            f()
            print(f"passed: {name}")
            passed += 1
        except Exception as problem:  # noqa: BLE001
            import traceback
            traceback.print_exc()
            print(f"FAILED: {name}: {problem}")
    total = len([1 for name, _ in tests if not only or name in only])
    print(f"{passed} of {total} tests passed.")
    sys.exit(0 if passed == total else 1)


if __name__ == "__main__":
    main()
