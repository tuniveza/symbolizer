"""Tests for the window words (bitwords.py): brick's own windows, from plain English.

Three kinds of test:
  - many ways of saying things build (with the real bitwin runtime);
  - what a phrase is read as (the fields it sets), without running anything;
  - end to end: English programs run under the fake compositor (tests/wayland_stub.py), checking pixels and
    what the program writes.
Programs without window words must not change: they never get the window routines.

Usage: python3 tests/test_bitwords.py        (SAVE=folder also saves the end-to-end frames as PNG there)
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
os.environ["BRICK_MEMORY"] = "off"
os.environ["BRICK_TRAINER"] = "off"

import bitwords  # noqa: E402
from bitwords import Toks  # noqa: E402
from compiler import Compiler  # noqa: E402
from expressions import Problem  # noqa: E402
from reader import words_of  # noqa: E402
from wayland_stub import Stub  # noqa: E402

FOLDER = tempfile.mkdtemp(prefix="bitwords-")
SAVE = os.environ.get("SAVE")
SETUP = """open a window called Test, 640 by 480
make a canvas called board at 20 80, 300 by 200
make a button called start saying Start at 20 20
put a text called score at the top saying Score: 0
make a menu called file with New, Open and Quit
make tabs called pages with Home, Settings and About
make a box called panel at 10% 10%, 80% by 80%
make an input box called name at 20 400
"""


def compiler():
    return Compiler(os.path.join(FOLDER, "mywords.txt"), ask_user=lambda question: "no")


def build(text, name="program"):
    c = compiler()
    program = c.compile_text(text)
    path = os.path.join(FOLDER, name)
    with open(path, "wb") as f:
        f.write(program)
    os.chmod(path, 0o755)
    return path, c


def refusal(text):
    try:
        compiler().compile_text(text)
    except Problem as problem:
        return str(problem)
    raise AssertionError(f"{text!r} should have been refused")


def read(line, setup=SETUP, kind=None):
    """The effects a phrase about an element is read as (after the setup made the elements)."""
    c = compiler()
    c.compile_text(setup)
    t = Toks(words_of(line))
    return c.gui_resolve(kind, c.gui_mods(t, 0, t.n, {"kind": kind, "creating": True}))


def fields(effects):
    return {e[1]: e[2] for e in effects if e[0] == "set"}


# ---------------- it builds, many ways ----------------

WAYS = """add a button
make a button
put a button called go
create a button called go
a button called go
button go saying Go
make a button named go
make a button labelled Go
make a button that says Go
make a button with the text Go
make a button called go at 20, 20
make a button called go at x 20 y 20
make a button called go at 20 px across and 40 down
make a button called go at 10% 20%
make a button called go in the middle
make a button called go at the top right
make a button called go 20 from the right
make a button called go 80x60
make a button called go 80 wide and 60 tall
make a button called go width 80 height 60
make a button called go half the window wide
make a button called go as wide as board
make a button called go 2 cm wide and 1 cm tall
make a button called go 10 mm wide and 20 pt tall
make a button called go 8 em wide
make a red button called go
make a button called go in light blue
make a button called go with a navy background
make a button called go with a radial gradient from white to blue
make a button called go with a gradient from red to orange going right
make a box called b1 with a dotted texture in white
make a box called b2 with big checks in grey
make a box called b3 that is grainy
make a box called b4 with bricks
make a box called b5 with a grid in white 20 px apart
make a box called b7 50% transparent
make a box called b9 with an opacity of 80%
make a button called go with rounded corners
make a button called go with a 3 px red border
make a button called go with a shadow
make a button called go with white text
make a button called go that turns yellow when the mouse is over it
make a button called go at depth 3
make a button called go below start
make a button called go next to start with a gap of 20
make a button called go on the Settings tab
make a button called go inside panel
make a button called go hidden
make start 200 by 50
make start bigger
put start 20 from the right and 20 from the bottom
move start down 2 cm
move start to the middle
move start forward
send start behind board
bring start in front of panel
set the depth of start to 3
set the width of start to 50%
set the colour of start to green
set start's text to "Go"
make start say Go
start is red
colour start green
paint board with a gradient from red to orange
give start a thick black border
give board a grainy texture
round the corners of start to 20
make panel fully opaque
grey out start
draw a green square on board at 10 10
draw a big blue circle on board at 100 100
draw a circle on board at 100 100 with a radius of 20 in yellow
draw an oval on board at 10 10, 80 by 40 with a gradient from red to blue
draw a thick line on board from 10 10 to 200 100
draw a white triangle on board with corners at 0 0, 50 0 and 25 40
draw a dot on board at 30 30
write hi on board at 5 5 in red, size 30
clear the window
add Save to file
put text in the window saying hello
write hi in the window at the top in gold
write start's x
write the mouse x on board
write the chosen item
when start is tapped, write hi
when someone clicks start, write hi
when the start button is clicked, write hi
when Quit is selected in the file menu, close the window
when Open is chosen, write opening
when tab Home is picked, write home
when someone presses a key, write the key
when the window is resized, write the width of the window
every half a second, write tick
when start is clicked, if the mouse x is more than 50, write right
wait for a click on start
wait for a click
after 2 seconds hide start
shut the window
when name changes, write the text of name
when enter is pressed in name, write the text of name
make the window 800 by 600
name the window Hello
give the window a gradient from black to navy
make a 3d view called v2 200 by 200"""


def test_many_ways_build():
    failed = []
    for line in WAYS.splitlines():
        try:
            compiler().compile_text(SETUP + line)
        except Problem as problem:
            failed.append(f"{line}: {str(problem).splitlines()[-1].strip()}")
    assert not failed, "\n".join(failed)


def test_the_contract_examples_build():
    text = """open a window called My Game, 800 by 600, in navy
make a button called start saying Start at 20 20, 120 by 40, in orange with round corners
add a button saying Quit at the bottom right
put a text called score at the top saying Score: 0, in gold, size 24
make a canvas called board at 20 80, 400 by 300 with a gradient from black to navy going down
make a menu called file with New, Open and Quit
make tabs called pages with Home, Settings and About
put start on the Home tab
make a gl context called view 300 by 300 to the right of board
make an input box called name below start
make a box called panel at 10% 10%, 80% by 80%, with a striped texture in grey, 50% transparent
start is 20 from the right and 20 from the bottom
put start below board with a gap of 10
make start 2 cm wide and 1 cm tall
put start in front of board
put start behind board
bring start to the front
send panel to the back
set the depth of start to 5
make start red
give start a gradient from red to orange
make board checked in grey
give start a 2 px gold border
give panel a shadow
make panel half see-through
round the corners of start by 12
make the text of start white and size 20
make start turn yellow when the mouse is over it
when start is clicked, set the text of score to "Score: 1"
when start is clicked:
    write clicked
    hide start
when New is chosen from file, clear board
when the Settings tab is picked, write settings
when a key is pressed, write the key
when board is clicked, draw a red dot on board at the mouse x and the mouse y
every 100 milliseconds, move start right 5
when the window is closed, write bye
draw a circle on board at 50 50, 30 by 30, in red
draw a line on board from 0 0 to 100 100 in white, 3 px thick
draw a triangle on board at 0 0, 50 0 and 25 40
write hello on board at 10 10 in gold
clear board
move start to 100 100
move start right 20
resize start to 200 by 50
hide start
show start
disable start
set points to 3
set the text of score to "Score: " joined with points
write the text of name
write the x of start
write the width of the window
wait until start is clicked
put a red cube in view
spin the cube
close the window"""
    compiler().compile_text(text)


# ---------------- what phrases are read as ----------------

def test_places_and_units():
    f = fields(read("at 20 40"))
    assert (f[bitwords.X], f[bitwords.XU], f[bitwords.Y], f[bitwords.YU]) == (20, 0, 40, 0)
    f = fields(read("at x 20 y 40"))
    assert (f[bitwords.X], f[bitwords.Y]) == (20, 40)
    f = fields(read("at 10% 20%"))
    assert (f[bitwords.X], f[bitwords.XU], f[bitwords.Y], f[bitwords.YU]) == (10, 1, 20, 1)
    f = fields(read("at 12.5% 0"))
    assert (f[bitwords.X], f[bitwords.XU]) == (125, 8)                 # per mille
    f = fields(read("20 from the right and 30 from the bottom"))
    assert (f[bitwords.X], f[bitwords.XU], f[bitwords.Y], f[bitwords.YU]) == (20, 2, 30, 2)
    f = fields(read("in the middle"))
    assert (f[bitwords.XU], f[bitwords.YU]) == (3, 3)
    f = fields(read("at the top right"))
    assert (f[bitwords.XU], f[bitwords.YU]) == (2, 0)
    f = fields(read("2 cm wide and 1 cm tall"))
    assert (f[bitwords.W], f[bitwords.H]) == (76, 38)
    f = fields(read("1 inch wide and 12 pt tall"))
    assert (f[bitwords.W], f[bitwords.H]) == (96, 16)
    f = fields(read("10 mm wide"))
    assert f[bitwords.W] == 38
    f = fields(read("800x600"))
    assert (f[bitwords.W], f[bitwords.H]) == (800, 600)
    f = fields(read("width 80 height 60"))
    assert (f[bitwords.W], f[bitwords.H]) == (80, 60)
    f = fields(read("half the window wide"))
    assert (f[bitwords.W], f[bitwords.WU]) == (50, 1)
    effects = read("below start with a gap of 12")
    f = fields(effects)
    assert (f[bitwords.YU], f[bitwords.XU], f[bitwords.GAP]) == (4, 6, 12)
    assert any(e[0] == "other" for e in effects)
    f = fields(read("to the left of board"))
    assert f[bitwords.XU] == 5


def test_looks():
    f = fields(read("in orange with round corners"))
    assert (f[bitwords.S + bitwords.FILL], f[bitwords.S + bitwords.COLOUR], f[bitwords.S + bitwords.RADIUS]) == \
        (1, 0xFFA500, 12)
    f = fields(read("with a gradient from black to navy going down"))
    assert (f[bitwords.S + bitwords.FILL], f[bitwords.S + bitwords.COLOUR], f[bitwords.S + bitwords.DIRECTION]) == \
        (2, 0, 0)
    assert f[bitwords.S + bitwords.COLOUR2] == 0x000080
    f = fields(read("with a radial gradient from white to blue"))
    assert f[bitwords.S + bitwords.FILL] == 3
    f = fields(read("fading from red to orange left to right"))
    assert f[bitwords.S + bitwords.DIRECTION] == 2
    f = fields(read("with a striped texture in grey, 50% transparent"))
    assert (f[bitwords.S + bitwords.TEXTURE], f[bitwords.S + bitwords.TEXTURE_COLOUR]) == (1, 0x808080)
    assert f[bitwords.S + bitwords.TRANSPARENCY] == 128
    for said, number in (("checked", 5), ("dotted", 4), ("grainy", 6), ("with bricks", 7), ("with a grid", 8),
                         ("with horizontal lines", 2), ("with vertical lines", 3), ("noisy", 6)):
        assert fields(read(said))[bitwords.S + bitwords.TEXTURE] == number, said
    assert fields(read("half see-through"))[bitwords.S + bitwords.TRANSPARENCY] == 128
    assert fields(read("with an opacity of 80%"))[bitwords.S + bitwords.TRANSPARENCY] == 51
    f = fields(read("with a 2 px gold border"))
    assert (f[bitwords.S + bitwords.BORDER], f[bitwords.S + bitwords.BORDER_COLOUR]) == (2, 0xFFD700)
    assert fields(read("with a shadow"))[bitwords.S + bitwords.SHADOW] == 4
    f = fields(read("with white text, text size 20"))
    assert (f[bitwords.S + bitwords.TEXT_COLOUR], f[bitwords.S + bitwords.TEXT_SIZE]) == (0xFFFFFF, 20)
    effects = read("that turns yellow when the mouse is over it")
    assert fields(effects)[bitwords.HOVER] == 0xFFFF00 and ("flag", bitwords.HAS_HOVER) in effects
    assert fields(read("in gold", kind=bitwords.TEXTKIND))[bitwords.S + bitwords.TEXT_COLOUR] == 0xFFD700
    assert fields(read("in crimson"))[bitwords.S + bitwords.COLOUR] == 0xDC143C          # a dictionary colour


def test_depth_words():
    assert ("call", "bw_front", None) in read("in front")
    effects = read("behind board")
    assert effects[0][:2] == ("call", "bw_behind")
    assert fields(read("at depth 3"))[bitwords.DEPTH] == 3


def test_refusals_say_what_to_do():
    assert "You can say" in refusal(SETUP + "make start wobbly")
    assert "can't move itself" in refusal(SETUP + "put the window on the left")
    assert "full screen" in refusal(SETUP + "make the window full screen")
    assert "Make it first" in refusal(SETUP + "when stop is clicked, write hi")
    assert "Make a canvas first" in refusal("open a window\ndraw a circle at 20 20")
    assert "What should happen" in refusal(SETUP + "when start is clicked")
    assert "Which colour" in refusal(SETUP + "set the text colour of start to bananas")


def test_programs_without_window_words_stay_the_same():
    for text in ("write hi", "set x to 5\nwrite x in red at the top right", 'ask "Name?" as name\nwrite hi name',
                 "repeat 3 times: write hi\nwait 1 second", "put the console on the left side of the screen"):
        _, c = build(text)
        assert not c.gui_on and not c.uses_gui, text
        assert not any(label.startswith(("bw_", "bww_", "wl_", "paint_")) for label in c.a.labels), text


def test_the_console_is_still_the_console():
    from terminal import run
    for line, shown in (("write hi to the console", "hi\n"), ("write hi in the console", "hi\n"),
                        ("write the width of the console", "40\n")):
        path, _ = build(line)
        screen, code, _ = run(path, size=(20, 40))
        assert code == 0 and screen == shown, (line, screen)
    _, c = build("set the title of the console to Demo\nput the console on the left side of the screen")
    assert c.uses_window and not c.uses_gui


# ---------------- end to end, under the fake compositor ----------------

class Run:
    """An English program running under the stub; what it writes arrives line by line."""

    def __init__(self, text, size=None):
        self.path, self.compiler = build(text, f"e2e{time.monotonic_ns()}")
        self.stub = Stub(size=size).start()
        env = dict(self.stub.env, BRICK_APP_ID="brick-test")
        self.process = subprocess.Popen([self.path], env=env, stdout=subprocess.PIPE, stderr=subprocess.STDOUT)
        self.lines, self.seen = queue.Queue(), []
        threading.Thread(target=self._read, daemon=True).start()
        self.stub.wait_frames(1, 10)
        self.settle()

    def _read(self):
        for line in self.process.stdout:
            self.lines.put(line.decode(errors="replace").rstrip("\n"))
        self.lines.put(None)

    def settle(self, seconds=0.25):
        """Wait until no new frame has come for a moment."""
        end = time.time() + 5
        count = -1
        while time.time() < end and count != len(self.stub.frames):
            count = len(self.stub.frames)
            time.sleep(seconds)

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

    def pixel(self, x, y):
        return self.stub.pixel(x, y)

    def save(self, name):
        if SAVE:
            os.makedirs(SAVE, exist_ok=True)
            self.stub.save_png(os.path.join(SAVE, name + ".png"))

    def click(self, x, y):
        self.stub.wait_input()
        self.stub.click(x, y)
        self.settle()

    def finish(self):
        self.stub.close()
        try:
            self.process.wait(timeout=5)
        except subprocess.TimeoutExpired:
            self.process.kill()
            self.process.wait()
        self.stub.stop()
        return self.process.returncode


def near(colour, wanted, slack=24):
    return all(abs((colour >> s & 255) - (wanted >> s & 255)) <= slack for s in (16, 8, 0))


def test_a_window_its_size_title_and_colour():
    run = Run("open a window called My Game, 320 by 200, in navy\nwrite ready")
    run.expect("ready")
    width, height, _ = run.stub.frames[-1]
    assert (width, height) == (320, 200) and run.stub.title == "My Game"
    assert run.pixel(160, 150) == 0x000080
    assert run.finish() == 0


def test_the_window_opens_by_itself():
    run = Run("make the window 300 by 200\nset the title of the window to Hello\nmake the window teal")
    width, height, _ = run.stub.frames[-1]
    assert (width, height) == (300, 200) and run.stub.title == "Hello"
    assert run.pixel(150, 150) == 0x008080
    run.finish()


def test_clicking_a_button_changes_a_text():
    run = Run("""open a window called Clicks, 400 by 300
make a button called start saying Start at 20 20, 120 by 40, in orange
put a text called score at 20 100 saying Score: 0, in white, size 20
set points to 0
when start is clicked:
    add 1 to points
    set the text of score to "Score: " joined with points
    write the text of score""")
    assert near(run.pixel(80, 30), 0xFFA500)
    before = run.stub.frames[-1][2]
    run.click(60, 40)
    run.expect("Score: 1")
    run.click(60, 40)
    run.expect("Score: 2")
    run.save("clicks")
    assert run.stub.frames[-1][2] != before          # the text was drawn again, changed
    run.finish()


def test_depth_in_front_and_behind():
    setup = """open a window 300 by 300 in white
make a box called red at 50 50, 100 by 100 in red
make a box called blue at 100 100, 100 by 100 in blue
"""
    run = Run(setup)
    assert run.pixel(120, 120) == 0x0000FF          # made later: in front
    run.finish()
    run = Run(setup + "put red in front of blue")
    assert run.pixel(120, 120) == 0xFF0000
    run.finish()
    run = Run(setup + "send blue to the back\nwhen red is clicked, bring blue to the front")
    assert run.pixel(120, 120) == 0xFF0000
    run.click(70, 70)
    assert run.pixel(120, 120) == 0x0000FF
    run.save("depth")
    run.finish()


def test_real_world_units():
    run = Run("open a window 400 by 200 in white\nmake a box called ruler at 1 cm 1 cm, 1 inch wide and 1 cm tall in red")
    assert run.pixel(42, 57) == 0xFF0000 and run.pixel(38 + 92, 57) == 0xFF0000      # 96 px wide (it has a border)
    assert run.pixel(38 + 99, 57) == 0xFFFFFF and run.pixel(35, 57) == 0xFFFFFF
    run.finish()
    run = Run("open a window 400 by 200 in white\nmake a box called half at 50% 0, 50% by 100% in green")
    assert run.pixel(196, 100) == 0xFFFFFF and run.pixel(205, 100) == 0x00C000 and run.pixel(395, 190) == 0x00C000
    run.finish()


def test_drawing_on_a_canvas_where_it_is_clicked():
    run = Run("""open a window 400 by 300 in white
make a canvas called board at 20 80, 300 by 200 in black
draw a circle on board at 10 10, 40 by 40 in yellow
when board is clicked, draw a red dot on board at the mouse x and the mouse y""")
    assert run.pixel(20 + 30, 80 + 30) == 0xFFFF00
    run.click(200, 150)
    assert near(run.pixel(200, 150), 0xFF0000) and run.pixel(250, 250) == 0
    run.save("canvas")
    run.finish()


def test_hover_colour():
    run = Run("""open a window 300 by 200 in white
make a button called go saying Go at 20 20, 120 by 40, in blue
make go turn yellow when the mouse is over it""")
    assert near(run.pixel(30, 50), 0x0000FF, 40)
    run.stub.wait_input()
    run.stub.move(60, 40)
    run.settle()
    assert near(run.pixel(30, 50), 0xFFFF00, 40)
    run.finish()


def test_keys_timers_and_closing():
    run = Run("""open a window 300 by 200
make a button called ball saying o at 10 10
when a key is pressed, write the key
every 100 milliseconds, move ball right 2
when the window is closed, write bye""")
    run.stub.wait_input()
    run.stub.type_text("q")
    run.expect("q")
    run.stub.key("left")
    run.expect("left")
    x = run.compiler and None
    time.sleep(0.5)
    run.stub.close()
    run.expect("bye")
    run.process.wait(timeout=5)
    run.stub.stop()
    assert x is None


def test_menus_and_tabs():
    run = Run("""open a window 400 by 300
make a menu called file with New, Old
when New is chosen from file, write new
when Old is chosen from file, write old""")
    run.click(15, 10)                                # the menu at the top left opens its list
    run.save("menu")
    found = None
    for y in range(30, 120, 4):                      # the items are listed under it
        run.click(25, y)
        try:
            run.expect("new", 0.5)
            found = y
            break
        except AssertionError:
            run.click(15, 10)
    assert found is not None, run.seen
    run.finish()
    run = Run("""open a window 400 by 300
make tabs called pages with Home, About
make a button called hello saying Hello at 20 100 in red
put hello on the About tab
when the About tab is picked, write about""")
    assert not any(near(run.pixel(x, 110), 0xFF0000) for x in range(20, 60))       # not on the Home page
    for x in range(60, 200, 10):
        run.click(x, 15)
        try:
            run.expect("about", 0.4)
            break
        except AssertionError:
            pass
    assert any(near(run.pixel(x, 110), 0xFF0000) for x in range(20, 60))
    run.save("tabs")
    run.finish()


def test_typing_in_an_input_box():
    run = Run("""open a window 400 by 200
make an input box called name at 20 20
when enter is pressed in name, write the text of name""")
    run.click(40, 30)
    run.stub.type_text("Ada\n")
    run.expect("Ada")
    run.finish()


def test_waiting_keeps_the_window_alive():
    run = Run("""open a window 300 by 200
make a button called go saying Go at 20 20
write waiting
wait until go is clicked
write clicked
wait 1 second
write done
close the window""")
    run.expect("waiting")
    run.click(40, 30)
    run.expect("clicked")
    run.expect("done")
    run.process.wait(timeout=5)
    run.stub.stop()


def test_a_spinning_cube_in_a_3d_view():
    run = Run("""open a window called Cube, 360 by 360
make a gl view called view at 30 30, 300 by 300
put a red cube in view
spin the cube""")
    first = len(run.stub.frames)
    time.sleep(0.5)
    assert len(run.stub.frames) > first                  # it keeps turning
    red = run.pixel(180, 190)
    assert red >> 16 > 100 and (red >> 8 & 255) < 60, hex(red)
    run.save("cube")
    run.finish()


def test_a_whole_english_program():
    run = Run("""open a window called Painter, 500 by 360, in navy
put a text called title at the top saying Brick paints, in gold, size 24
make a canvas called board at 20 60, 300 by 220 with a gradient from black to blue going down
make a box called panel at 340 60, 140 by 220 with a striped texture in grey, 50% transparent, with round corners
make a button called clear saying Clear at 20 300, 120 by 40 in orange with round corners
make clear turn yellow when the mouse is over it
draw a red circle on board at 40 40, 60 by 60
draw a line on board from 0 200 to 300 120 in white, 3 px thick
draw a triangle on board at 200 30, 260 30 and 230 90 in green
write hello on board at 20 180 in gold
when clear is clicked, clear board""")
    run.save("painter")
    assert near(run.pixel(20 + 70, 60 + 70), 0xFF0000)
    run.click(60, 320)
    assert not near(run.pixel(20 + 70, 60 + 70), 0xFF0000)
    run.finish()


def main():
    tests = [(name, f) for name, f in globals().items() if name.startswith("test_") and callable(f)]
    only = sys.argv[1:]
    passed = 0
    for name, test in tests:
        if only and not any(o in name for o in only):
            continue
        try:
            test()
            print(f"passed: {name}")
            passed += 1
        except Exception as problem:
            import traceback
            traceback.print_exc()
            print(f"FAILED: {name}: {type(problem).__name__}: {problem}")
    total = len([n for n, _ in tests if not only or any(o in n for o in only)])
    print(f"{passed} of {total} tests passed.")
    sys.exit(0 if passed == total else 1)


if __name__ == "__main__":
    main()
