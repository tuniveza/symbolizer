"""Tests for brick's own Wayland client (wayland.py), run against the fake compositor in wayland_stub.py.

The programs are built straight from machine code (asm.Asm), so these tests don't depend on the English.
Each program opens a window, fills it with a colour, presents it, then reports every event on its
output as "ev KIND RCX RDX R8" and reacts: a button press paints the second colour, a resize repaints,
"p" presents three frames in a row, "q" ends it, close ends it.

Usage: python3 tests/test_wayland.py            (LIVE=1 also opens a real window on the desktop for 2 s)
"""
import os
import queue
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

import elf  # noqa: E402
import runtime  # noqa: E402
import wayland  # noqa: E402
from asm import EQUAL, NOT_EQUAL, R8, RAX, RCX, RDI, RDX, RSI, Asm  # noqa: E402
from wayland_stub import Stub  # noqa: E402

BLUE, ORANGE = 0x3366CC, 0xFF8800
PRESENT_COLOURS = (0x112233, 0x445566, 0x778899)
FOLDER = tempfile.mkdtemp(prefix="bricks-wayland-")
BUILT = {}


def fill(a, colour):
    """Paint the whole gui_target one colour."""
    top, done = a.new_label("t"), a.new_label("t")
    a.address(RDI, "gui_target")
    a.load(RSI, RDI, 0)
    a.load(RCX, RDI, 16)
    a.load(RDX, RDI, 24)
    a.multiply(RCX, RDX)
    a.shift_right(RCX, 2)
    a.set(RAX, colour)
    a.label(top)
    a.op("test", RCX, RCX)
    a.jump_if(EQUAL, done)
    a.store32(RSI, 0, RAX)
    a.op_n("add", RSI, 4)
    a.op_n("sub", RCX, 1)
    a.jump(top)
    a.label(done)


def say(a, s):
    a.address(RAX, a.text(s))
    a.call("write_text")


def say_number_at(a, block, offset):
    a.address(RSI, block)
    a.load(RAX, RSI, offset)
    a.call("number_to_text")
    a.call("write_text")


def build(title="brick test", width=200, height=100, resizable=0, wait=-1, timeouts=0):
    """wait: ms for wl_event. timeouts: end after this many "nothing happened" answers (0 = never)."""
    key = (title, width, height, resizable, wait, timeouts)
    if key in BUILT:
        return BUILT[key]
    a = Asm()
    runtime.start(a)
    ev = a.reserve(40, "t_event")
    a.address(RAX, a.text(title))
    a.set(RCX, width)
    a.set(RDX, height)
    a.set(R8, resizable)
    a.call("wl_open")
    a.address(RSI, ev)
    a.store(RSI, 0, RAX)
    say(a, "open ")
    say_number_at(a, ev, 0)
    say(a, "\n")
    opened = a.new_label("t")
    a.address(RSI, ev)
    a.load(RAX, RSI, 0)
    a.op("test", RAX, RAX)
    a.jump_if(NOT_EQUAL, opened)
    runtime.finish(a, 3)
    a.label(opened)
    say(a, "scale ")
    say_number_at(a, "gui_scale", 0)
    say(a, "\n")
    fill(a, BLUE)
    a.call("wl_present")

    loop, report, not_kind = a.new_label("t"), a.new_label("t"), {}
    a.label(loop)
    a.set(RAX, wait)
    a.call("wl_event")
    a.address(RSI, ev)
    for n, reg in enumerate((RAX, RCX, RDX, R8)):
        a.store(RSI, 8 * n, reg)
    a.op("test", RAX, RAX)
    a.jump_if(NOT_EQUAL, report)
    if timeouts:                                  # count the quiet answers
        a.load(RCX, RSI, 32)
        a.op_n("add", RCX, 1)
        a.store(RSI, 32, RCX)
        a.op_n("cmp", RCX, timeouts)
        ended = a.new_label("t")
        a.jump_if(NOT_EQUAL, ended)
        say(a, "done\n")
        a.call("wl_close")
        runtime.finish(a, 0)
        a.label(ended)
    a.label(report)
    say(a, "ev")
    for n in range(4):
        say(a, " ")
        say_number_at(a, ev, 8 * n)
    say(a, "\n")

    def when(kind, value=None):
        skip = a.new_label("t")
        a.address(RSI, ev)
        a.load(RAX, RSI, 0)
        a.op_n("cmp", RAX, kind)
        a.jump_if(NOT_EQUAL, skip)
        if value is not None:
            a.load(RAX, RSI, 8)
            a.op_n("cmp", RAX, value)
            a.jump_if(NOT_EQUAL, skip)
        return skip

    skip = when(2)                                # button pressed: the second colour
    fill(a, ORANGE)
    a.call("wl_present")
    a.label(skip)
    skip = when(5)                                # resized: draw again at the new size
    say(a, "scale ")
    say_number_at(a, "gui_scale", 0)
    say(a, "\n")
    fill(a, BLUE)
    a.call("wl_present")
    a.label(skip)
    skip = when(4, ord("p"))                      # three frames in a row
    for colour in PRESENT_COLOURS:
        fill(a, colour)
        a.call("wl_present")
    say(a, "presented\n")
    a.label(skip)
    skip = when(4, ord("q"))
    a.call("wl_close")
    runtime.finish(a, 0)
    a.label(skip)
    skip = when(6)
    a.call("wl_close")
    say(a, "closed\n")
    runtime.finish(a, 0)
    a.label(skip)
    a.jump(loop)

    runtime.add_all(a)
    wayland.add_all(a)
    path = os.path.join(FOLDER, f"program{len(BUILT)}")
    with open(path, "wb") as f:
        f.write(elf.wrap(a))
    os.chmod(path, 0o755)
    BUILT[key] = path
    return path


class Run:
    """A program running under a stub, with its output lines collected as they come."""

    def __init__(self, path, env):
        self.process = subprocess.Popen([path], env=env, stdout=subprocess.PIPE, stderr=subprocess.PIPE)
        self.lines = queue.Queue()
        self.seen = []
        threading.Thread(target=self._read, daemon=True).start()

    def _read(self):
        for line in self.process.stdout:
            self.lines.put(line.decode().rstrip("\n"))

    def expect(self, wanted, timeout=5.0):
        """Wait for a line starting with wanted; returns it."""
        end = time.time() + timeout
        while True:
            try:
                line = self.lines.get(timeout=max(0.01, end - time.time()))
            except queue.Empty:
                raise AssertionError(f"waited for {wanted!r}; saw {self.seen}") from None
            self.seen.append(line)
            if line.startswith(wanted):
                return line

    def events(self, n, timeout=5.0):
        return [tuple(int(v) for v in self.expect("ev ", timeout).split()[1:]) for _ in range(n)]

    def finish(self, timeout=5.0):
        try:
            return self.process.wait(timeout)
        except subprocess.TimeoutExpired:
            self.process.kill()
            raise AssertionError("the program didn't end") from None

    def stop(self):
        if self.process.poll() is None:
            self.process.kill()
            self.process.wait()


def session(stub=None, env=None, **program):
    stub = (stub or Stub()).start()
    run = Run(build(**program), dict(stub.env, **(env or {})))
    return stub, run


def no_errors(stub):
    assert not stub.errors, stub.errors


# ---- the tests ----

def test_no_wayland_means_no_window():
    path = build()
    empty = tempfile.mkdtemp(prefix="bricks-nowl-")
    for env in [{}, {"XDG_RUNTIME_DIR": empty}, {"XDG_RUNTIME_DIR": empty, "WAYLAND_DISPLAY": "wayland-9"},
                {"WAYLAND_DISPLAY": os.path.join(empty, "nothing-here")}, {"WAYLAND_DISPLAY": "wayland-0"}]:
        done = subprocess.run([path], env=env, capture_output=True, timeout=10)
        assert done.returncode == 3 and done.stdout == b"open 0\n", (env, done)


def test_missing_globals_means_no_window():
    for missing in ("wl_compositor", "wl_shm", "xdg_wm_base"):
        offer = [g for g in Stub().offer if g[0] != missing]
        stub, run = session(Stub(offer=offer))
        try:
            run.expect("open 0")
            assert run.finish() == 3
        finally:
            run.stop()
            stub.stop()


def test_window_opens_with_its_size_title_and_colour():
    stub, run = session()
    try:
        run.expect("open 1")
        width, height, _ = stub.wait_frames(1)
        assert (width, height) == (200, 100)
        assert stub.pixel(0, 0) == BLUE and stub.pixel(199, 99) == BLUE
        assert stub.title == "brick test" and stub.app_id == "brick"
        assert stub.min_size == stub.max_size == (200, 100)
        assert stub.versions == {"wl_compositor": 6, "wl_shm": 1, "xdg_wm_base": 2, "wl_seat": 5,
                                 "wp_viewporter": 1, "wp_fractional_scale_manager_v1": 1}, stub.versions
        assert stub.destination == (200, 100)
        png = os.path.join(FOLDER, "frame.png")
        stub.save_png(png)
        assert open(png, "rb").read(8) == b"\x89PNG\r\n\x1a\n"
        no_errors(stub)
    finally:
        run.stop()
        stub.stop()


def test_absolute_socket_path_app_id_and_resizable():
    stub = Stub().start()
    env = {"WAYLAND_DISPLAY": stub.path, "BRICK_APP_ID": "brick-tests"}
    run = Run(build(title="Grow ✓", resizable=1), env)
    try:
        run.expect("open 1")
        stub.wait_frames(1)
        assert stub.app_id == "brick-tests" and stub.title == "Grow ✓"
        assert stub.min_size is None and stub.max_size is None
        no_errors(stub)
    finally:
        run.stop()
        stub.stop()


def test_the_compositor_can_choose_the_size():
    stub, run = session(Stub(size=(320, 240)))
    try:
        run.expect("open 1")
        width, height, _ = stub.wait_frames(1)
        assert (width, height) == (320, 240)
        assert stub.min_size == (200, 100)            # what the program asked for
        no_errors(stub)
    finally:
        run.stop()
        stub.stop()


def test_pointer():
    stub, run = session()
    try:
        run.expect("open 1")
        stub.wait_frames(1)
        stub.wait_input()
        stub.click(10.5, 20.9)
        assert run.events(3) == [(1, 10, 20, 0), (2, 10, 20, 1), (3, 10, 20, 1)]
        stub.wait_frames(2)
        assert stub.pixel(5, 5) == ORANGE
        stub.click(30, 40, button=3)
        assert run.events(3) == [(1, 30, 40, 0), (2, 30, 40, 3), (3, 30, 40, 3)]
        stub.button(2)
        assert run.events(1) == [(2, 30, 40, 2)]
        stub.scroll(15)
        stub.scroll(-30)
        stub.scroll(0.25)
        assert run.events(3) == [(7, 30, 40, 15), (7, 30, 40, -30), (7, 30, 40, 1)]
        stub.leave()
        assert run.events(1) == [(8, 30, 40, 0)]
        no_errors(stub)
    finally:
        run.stop()
        stub.stop()


def test_moves_in_a_row_become_one():
    stub, run = session()
    try:
        run.expect("open 1")
        stub.wait_frames(1)
        stub.wait_input()
        stub.send_raw(b"".join(stub.message(stub.pointer, 2, bytes(4) + (x * 256).to_bytes(4, "little") +
                                            (7 * 256).to_bytes(4, "little")) for x in range(1, 40)))
        stub.button(1)
        assert run.events(2) == [(1, 39, 7, 0), (2, 39, 7, 1)]
    finally:
        run.stop()
        stub.stop()


def test_keyboard():
    stub, run = session()
    try:
        run.expect("open 1")
        stub.wait_frames(1)
        stub.wait_input()
        stub.type_text("aZ1!?~ \n\t\b\x1b")
        expected = [ord(c) for c in "aZ1!?~ "] + [10, 9, 8, 27]
        assert [e[1] for e in run.events(len(expected))] == expected
        for name, code in [("left", 1001), ("right", 1002), ("up", 1003), ("down", 1004), ("home", 1005),
                           ("end", 1006), ("delete", 1007)]:
            stub.key(name)
            assert run.events(1) == [(4, code, 0, 0)], name
        stub.key("c", ctrl=True)
        assert run.events(1) == [(4, ord("c"), 1, 0)]
        stub.key("shift")                              # keys without a character say nothing
        stub.key("ctrl")
        stub.modifiers(locked=2)                       # caps lock
        stub.type_text("aA1")
        stub.modifiers(locked=0)
        stub.type_text("b")
        assert [e[1] for e in run.events(4)] == [ord("A"), ord("a"), ord("1"), ord("b")]
        no_errors(stub)
    finally:
        run.stop()
        stub.stop()


def test_messages_split_or_bunched_up():
    stub, run = session()
    try:
        run.expect("open 1")
        stub.wait_frames(1)
        stub.wait_input()
        stub.key("x")                                  # enter the keyboard first
        run.events(1)

        def key(code, pressed):
            return stub.message(stub.keyboard, 3, (1).to_bytes(4, "little") + bytes(4) +
                                code.to_bytes(4, "little") + pressed.to_bytes(4, "little"))
        unknown = (stub.message(stub.surface, 0, (1).to_bytes(4, "little")) +    # wl_surface.enter: ignored
                   stub.message(200, 3, bytes(12)) + stub.message(stub.keyboard, 9, bytes(20)))
        raw = unknown + key(30, 1) + key(30, 0) + key(48, 1) + key(48, 0)
        stub.send_raw(raw[:5])                         # pieces that end mid-header and mid-message
        time.sleep(0.05)
        stub.send_raw(raw[5:len(unknown) + 7])
        time.sleep(0.05)
        stub.send_raw(raw[len(unknown) + 7:])
        assert run.events(2) == [(4, ord("a"), 0, 0), (4, ord("b"), 0, 0)]
        no_errors(stub)
    finally:
        run.stop()
        stub.stop()


def test_resize():
    stub, run = session(resizable=1)
    try:
        run.expect("open 1")
        stub.wait_frames(1)
        stub.resize(300, 150)
        assert run.events(1) == [(5, 300, 150, 0)]
        width, height, _ = stub.wait_frames(2)
        assert (width, height) == (300, 150) and stub.pixel(299, 149) == BLUE
        stub.resize(300, 150)                          # the same size again: nothing to say
        stub.resize(0, 0)                              # the program chooses: keeps its size
        stub.resize(120, 80)
        assert run.events(1) == [(5, 120, 80, 0)]
        width, height, _ = stub.wait_frames(3)
        assert (width, height) == (120, 80)
        assert stub.acks == [c[0] for c in stub.configures], (stub.acks, stub.configures)
        no_errors(stub)
    finally:
        run.stop()
        stub.stop()


def test_fractional_scale_means_real_pixels():
    stub, run = session(Stub(scale=192), resizable=1)
    try:
        run.expect("open 1")
        assert run.expect("scale ") == "scale 192"
        width, height, _ = stub.wait_frames(1)
        assert (width, height) == (320, 160) and stub.destination == (200, 100)
        assert stub.pixel(319, 159) == BLUE
        stub.wait_input()
        stub.click(10, 20.5)                           # logical 10, 20.5 = real 16, 32.8
        assert run.events(3) == [(1, 16, 32, 0), (2, 16, 32, 1), (3, 16, 32, 1)]
        stub.scroll(10)
        assert run.events(1) == [(7, 16, 32, 16)]
        stub.set_scale(240)                            # moved to a screen at 2.0
        assert run.events(1) == [(5, 400, 200, 0)]
        assert run.expect("scale ") == "scale 240"
        stub.wait_until(lambda: stub.frames[-1][0] == 400)
        assert stub.frames[-1][:2] == (400, 200) and stub.destination == (200, 100)
        stub.set_scale(240)                            # the same again: nothing to say
        stub.resize(300, 150)                          # logical size from the compositor
        assert run.events(1) == [(5, 600, 300, 0)]
        stub.wait_until(lambda: stub.frames[-1][0] == 600)
        assert stub.frames[-1][:2] == (600, 300) and stub.destination == (300, 150)
        stub.set_scale(120)
        assert run.events(1) == [(5, 300, 150, 0)]
        no_errors(stub)
    finally:
        run.stop()
        stub.stop()


def test_whole_scale_when_there_is_no_fractional_scale():
    stub, run = session(Stub(scale=192, fractional=False))
    try:
        run.expect("open 1")
        assert run.expect("scale ") == "scale 240"     # 1.6 rounded up to 2 by the compositor
        width, height, _ = stub.wait_frames(1)
        assert (width, height) == (400, 200) and stub.buffer_scale == 2 and stub.destination is None
        assert stub.min_size == stub.max_size == (200, 100)
        stub.wait_input()
        stub.click(10, 20)
        assert run.events(3) == [(1, 20, 40, 0), (2, 20, 40, 1), (3, 20, 40, 1)]
        stub.set_scale(120)
        assert run.events(1) == [(5, 200, 100, 0)]
        stub.wait_until(lambda: stub.frames[-1][0] == 200)
        assert stub.buffer_scale == 1
        no_errors(stub)
    finally:
        run.stop()
        stub.stop()


def test_no_scale_known_means_one():
    stub, run = session(Stub(scale=192, fractional=False, compositor_version=4))
    try:
        run.expect("open 1")
        assert run.expect("scale ") == "scale 120"
        width, height, _ = stub.wait_frames(1)
        assert (width, height) == (200, 100) and stub.buffer_scale == 1
        assert stub.versions["wl_compositor"] == 4
        no_errors(stub)
    finally:
        run.stop()
        stub.stop()


def test_double_buffering_waits_for_release():
    stub, run = session(Stub(release=0.3))
    try:
        run.expect("open 1")
        stub.wait_frames(1)
        stub.wait_input()
        started = time.time()
        stub.key("p")
        time.sleep(0.1)
        serial = stub.ping()                           # answered while wl_present waits
        assert stub.wait_until(lambda: serial in stub.pongs, 0.3), stub.pongs
        run.expect("presented")
        took = time.time() - started
        assert len(stub.frames) == 4
        assert [stub.pixel(1, 1, n) for n in (1, 2, 3)] == list(PRESENT_COLOURS)
        b = stub.frame_buffers
        assert b[0] != b[1] and b[0] == b[2] and b[1] == b[3], b
        assert took >= 0.5, took                       # it really waited for each release
        no_errors(stub)
    finally:
        run.stop()
        stub.stop()


def test_a_compositor_that_never_releases_only_slows_it_down():
    stub, run = session(Stub(release="never"))
    try:
        run.expect("open 1")
        stub.wait_frames(1)
        stub.wait_input()
        stub.key("p")
        run.expect("presented", timeout=10)
        stub.key("q")
        assert run.finish() == 0
    finally:
        run.stop()
        stub.stop()


def test_ping_and_close():
    stub, run = session()
    try:
        run.expect("open 1")
        stub.wait_frames(1)
        serial = stub.ping()
        assert stub.wait_until(lambda: serial in stub.pongs, 3), stub.pongs
        stub.close()
        assert run.events(1) == [(6, 0, 0, 0)]
        run.expect("closed")
        assert run.finish() == 0
        assert stub.wait_closed(3)
        no_errors(stub)
    finally:
        run.stop()
        stub.stop()


def test_wait_times_out():
    stub, run = session(wait=100, timeouts=5)
    try:
        run.expect("open 1")
        started = time.time()
        run.expect("done", timeout=5)
        took = time.time() - started
        assert 0.4 <= took < 2, took
        assert run.finish() == 0
        assert stub.wait_closed(3)
    finally:
        run.stop()
        stub.stop()


def test_no_fds_left_open():
    stub, run = session()
    try:
        run.expect("open 1")
        stub.wait_frames(1)
        stub.wait_input()
        stub.type_text("ab")
        run.events(2)
        folder = f"/proc/{run.process.pid}/fd"
        targets = sorted(os.readlink(os.path.join(folder, fd)) for fd in os.listdir(folder) if int(fd) > 2)
        assert len(targets) == 1 and targets[0].startswith("socket:"), targets
        maps = open(f"/proc/{run.process.pid}/maps").read()
        assert maps.count("memfd:brick") == 1, maps            # one pool, mapped once
    finally:
        run.stop()
        stub.stop()


def test_compositor_errors_and_going_away():
    stub, run = session()
    try:
        run.expect("open 1")
        stub.wait_frames(1)
        stub.protocol_error("the stub says no")
        run.expect("ev 6")
        run.expect("closed")
        assert run.finish() == 0
        assert b"the stub says no" in run.process.stderr.read()
    finally:
        run.stop()
        stub.stop()
    stub, run = session()
    try:
        run.expect("open 1")
        stub.wait_frames(1)
        stub.disconnect()                              # the compositor vanishes
        run.expect("ev 6")
        assert run.finish() == 0
    finally:
        run.stop()
        stub.stop()


def test_a_silent_compositor_means_no_window():
    """It accepts the connection but never answers: wl_open gives up (after 5 seconds) instead of hanging."""
    import socket
    folder = tempfile.mkdtemp(prefix="bricks-silent-")
    listener = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
    listener.bind(os.path.join(folder, "wayland-0"))
    listener.listen(1)
    try:
        started = time.time()
        done = subprocess.run([build()], env={"XDG_RUNTIME_DIR": folder}, capture_output=True, timeout=20)
        assert done.returncode == 3 and done.stdout == b"open 0\n", done
        assert 4 < time.time() - started < 10
    finally:
        listener.close()


def test_programs_without_windows_are_unchanged():
    a, b = Asm(), Asm()
    for x in (a, b):
        runtime.start(x)
        runtime.finish(x)
        runtime.add_all(x)
    wayland.add_all(b)
    assert len(elf.wrap(b)) == len(elf.wrap(a))     # routines nothing calls are left out (Asm.prune)
    c = Asm()
    runtime.start(c)
    runtime.finish(c)
    runtime.add_all(c)
    assert elf.wrap(a) == elf.wrap(c)


def live_check():
    """A real window on the desktop: 400 x 300, "brick test", ends by itself after about 2 seconds."""
    path = build(title="brick test", width=400, height=300, wait=100, timeouts=25)
    started = time.time()
    done = subprocess.run([path], capture_output=True, timeout=20)
    print("live:", done.returncode, done.stdout.decode().split("\n")[:3], round(time.time() - started, 2), "s")


if __name__ == "__main__":
    if os.environ.get("LIVE") == "1":
        live_check()
        sys.exit(0)
    tests = [value for name, value in list(globals().items()) if name.startswith("test_")]
    failed = 0
    for test in tests:
        try:
            test()
            print(f"passed: {test.__name__}")
        except Exception as problem:
            failed += 1
            print(f"FAILED: {test.__name__}: {type(problem).__name__}: {problem}")
    print(f"{len(tests) - failed} of {len(tests)} tests passed.")
    sys.exit(1 if failed else 0)
