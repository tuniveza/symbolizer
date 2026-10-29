"""Run the real sy program in a pretend terminal and press real key codes.

    python3 tests/e2e.py            (needs: pip install pyte)

Each check starts sy in a pseudo-terminal, sends the exact bytes a terminal
sends for each key, then reads the screen and the saved file.
"kitty" checks also answer sy's keyboard question the way kitty does, so
sy gets key-release events, as it does in your real kitty.
"""
import os
import pty
import re
import select
import signal
import subprocess
import sys
import tempfile
import time

import pyte

HERE = os.path.dirname(os.path.abspath(__file__))
SY = os.path.join(os.path.dirname(HERE), "sy")
ROWS, COLS = 30, 100
FAKE_HOME = tempfile.mkdtemp(prefix="sy-e2e-home-")

KEYS = {
    "up": "\x1b[A", "down": "\x1b[B", "right": "\x1b[C", "left": "\x1b[D",
    "alt+up": "\x1b[1;3A", "alt+down": "\x1b[1;3B",
    "alt+right": "\x1b[1;3C", "alt+left": "\x1b[1;3D",
    "shift+right": "\x1b[1;2C", "shift+left": "\x1b[1;2D", "shift+down": "\x1b[1;2B",
    "ctrl+right": "\x1b[1;5C", "ctrl+left": "\x1b[1;5D", "ctrl+up": "\x1b[1;5A", "ctrl+down": "\x1b[1;5B",
    "ctrl+shift+right": "\x1b[1;6C", "ctrl+shift+left": "\x1b[1;6D",
    "alt+shift+up": "\x1b[1;4A", "alt+shift+down": "\x1b[1;4B",
    "enter": "\r", "esc": "\x1b", "backspace": "\x7f", "tab": "\t",
    "f1": "\x1bOP", "f3": "\x1bOR",
    "alt+l": "\x1bl", "alt+b": "\x1bb", "alt+e": "\x1be", "alt+s": "\x1bs",
    "ctrl+home": "\x1b[1;5H",
    "alt+a": "\x1ba",
}
for letter in "acdefgklnopqrsvxyz":
    KEYS["ctrl+" + letter] = chr(ord(letter) - 96)


class FullScreen(pyte.Screen):
    """pyte plus two codes real terminals (kitty) know and Bubble Tea uses:
    CBT, ESC[n Z (back n tab stops), and REP, ESC[n b (repeat the last letter)."""

    last_char = " "

    def draw(self, data):
        if data:
            self.last_char = data[-1]
        super().draw(data)

    def cursor_back_tab(self, count=None):
        for _ in range(max(count or 1, 1)):
            x = self.cursor.x
            stops = [s for s in sorted(self.tabstops) if s < x]
            self.cursor.x = stops[-1] if stops else 0

    def repeat_last(self, count=None):
        self.draw(self.last_char * max(count or 1, 1))


class FullStream(pyte.ByteStream):
    csi = dict(pyte.ByteStream.csi, Z="cursor_back_tab", b="repeat_last")


class Term:
    def __init__(self, args, kitty=False, env=None, cwd=None):
        self.screen = FullScreen(COLS, ROWS)
        self.stream = FullStream(self.screen)
        self.kitty = kitty
        pid, fd = pty.fork()
        if pid == 0:
            e = dict(os.environ, TERM="xterm-256color", COLORTERM="truecolor",
                     LINES=str(ROWS), COLUMNS=str(COLS))
            e.pop("WAYLAND_DISPLAY", None)   # keep the real clipboard untouched
            e.pop("DISPLAY", None)
            e["HOME"] = FAKE_HOME                # sessions never touch the real home
            e["XDG_DATA_HOME"] = os.path.join(FAKE_HOME, "data")
            e["SY_NO_NETWORK"] = "1"                 # weather only from the test's own file
            e.update(env or {})
            if cwd:
                os.chdir(cwd)
            os.execve(SY, [SY] + args, e)
        self.pid, self.fd = pid, fd
        import fcntl, struct, termios
        fcntl.ioctl(fd, termios.TIOCSWINSZ, struct.pack("HHHH", ROWS, COLS, 0, 0))
        os.kill(pid, signal.SIGWINCH)
        self.raw = b""
        self.pump(0.6)

    def pump(self, wait=0.15):
        end = time.time() + wait
        while time.time() < end:
            r, _, _ = select.select([self.fd], [], [], max(0, end - time.time()))
            if not r:
                continue
            try:
                data = os.read(self.fd, 65536)
            except OSError:
                return
            self.raw += data
            if self.kitty and b"\x1b[?u" in data:
                # kitty's answer: "I support the keyboard protocol"
                os.write(self.fd, b"\x1b[?3u")
            self.stream.feed(data)

    def send(self, s, wait=0.12):
        os.write(self.fd, s.encode() if isinstance(s, str) else s)
        self.pump(wait)

    def key(self, *names, wait=0.12):
        for n in names:
            self.send(KEYS[n], wait)

    def type(self, text):
        for ch in text:
            self.send(ch, 0.02)
        self.pump(0.1)

    def text(self):
        return "\n".join(self.screen.display)

    def row(self, y):
        return self.screen.display[y]

    def close(self):
        try:
            os.kill(self.pid, signal.SIGKILL)
        except ProcessLookupError:
            pass
        try:
            os.waitpid(self.pid, 0)
        except ChildProcessError:
            pass

    def exited(self, wait=1.0):
        end = time.time() + wait
        while time.time() < end:
            self.pump(0.05)
            pid, status = os.waitpid(self.pid, os.WNOHANG)
            if pid:
                return True
        return False


results = []


def check(name, ok, detail=""):
    results.append((name, ok))
    mark = "PASS" if ok else "FAIL"
    print(f"{mark}  {name}" + (f"   ({detail})" if detail and not ok else ""))


def cursor(t):
    return t.screen.cursor.y, t.screen.cursor.x



def near(got, want, tol=3):
    """Colours match within tol per channel (the margins have a fine grain)."""
    try:
        return all(abs(int(got[i:i + 2], 16) - int(want[i:i + 2], 16)) <= tol for i in (0, 2, 4))
    except ValueError:
        return False

def main():
    tmp = tempfile.mkdtemp(prefix="sy-e2e-")
    path = os.path.join(tmp, "demo.py")
    with open(path, "w") as f:
        f.write("def greet(name):\n    return 'hi ' + name  # say hi\n\nprint(greet('ember'))\n")

    # ---- screen and colours ----
    t = Term([path])
    top, status = t.row(0), t.row(ROWS - 2)
    check("top panel shows SYMBOLIZER and the mode at both ends",
          "S Y M B O L I Z E R" in top and top.strip().startswith("✎ Text") and top.strip().endswith("Text ✎"), top)
    check("tab row shows the file, without the old title", "demo.py" in t.row(1) and "Symbolizer" not in t.row(1), t.row(1))
    check("line numbers and text drawn", "1 def greet(name):" in t.row(2), t.row(2))
    check("status bar shows language and position", "Python" in status and "Ln 1, Col 1" in status, status)
    bg = t.screen.buffer[6][30].bg
    check("Emberistal background #161616", bg.lower() == "161616", bg)
    fg = t.screen.buffer[2][5].fg            # the 'def' keyword
    check("keyword coloured Emberistal #ff3704", fg.lower() == "ff3704", fg)
    fgs = t.screen.buffer[3][17].fg          # inside the 'hi ' string
    check("string coloured Emberistal #ff8a1a", fgs.lower() == "ff8a1a", fgs)

    # ---- your navigation keys ----
    t.send("\x1b[F")                                # End
    check("End = end of line", cursor(t) == (2, 5 + 16), cursor(t))
    t.key("alt+b")
    check("Alt+B = start of line", cursor(t) == (2, 5), cursor(t))
    t.key("alt+e")
    check("Alt+E = end of file", cursor(t) == (5, 5 + 21), cursor(t))
    t.key("alt+e")
    t.key("alt+s")
    check("Alt+S = start of file", cursor(t) == (2, 5), cursor(t))
    t.key("ctrl+e")
    check("Ctrl+E does nothing", cursor(t) == (2, 5), cursor(t))
    t.key("ctrl+home")
    check("Ctrl+Home = start of file", cursor(t) == (2, 5), cursor(t))
    t.key("alt+right")
    check("Alt+Right moves the word right", "greet def(name):" in t.row(2) and cursor(t) == (2, 5 + 6),
          (t.row(2), cursor(t)))
    t.key("alt+left")
    check("Alt+Left moves it back", "def greet(name):" in t.row(2) and cursor(t) == (2, 5), (t.row(2), cursor(t)))
    check("status bar: an Alt key shows movement", "movement" in t.row(ROWS - 2), t.row(ROWS - 2))
    time.sleep(0.2)
    t.key("alt+down")
    check("Alt+Down moves the line down", "def greet" in t.row(3) and "return" in t.row(2)
          and cursor(t) == (3, 5), (t.row(2), t.row(3), cursor(t)))
    time.sleep(0.2)
    t.key("alt+up")
    check("Alt+Up moves the line back up", "def greet" in t.row(2) and "return" in t.row(3)
          and cursor(t) == (2, 5), (t.row(2), t.row(3), cursor(t)))
    t.key("down")
    check("Down = cursor down a line", cursor(t) == (3, 5), cursor(t))
    check("status bar: a plain key shows text", "text" in t.row(ROWS - 2), t.row(ROWS - 2))
    t.key("up")
    check("Up = cursor up a line", cursor(t) == (2, 5), cursor(t))
    t.key("right", "shift+right")                 # " greet" highlighted
    t.key("alt+down")
    check("Alt+Down moves only the highlighted text down", "def(name):" in t.row(2)
          and "    greet return" in t.row(3), (t.row(2), t.row(3)))
    t.key("alt+up")
    check("Alt+Up moves it back", "def greet(name):" in t.row(2) and "    return 'hi '" in t.row(3),
          (t.row(2), t.row(3)))
    t.key("esc", "ctrl+home")

    # diagonal without key-release support: two keys pressed together
    t.key("down", "ctrl+right", "ctrl+right", "ctrl+right")      # line 2, col 3
    time.sleep(0.8)
    t.send(KEYS["ctrl+left"] + KEYS["ctrl+up"])     # both at once
    check("Ctrl+Left+Up together = up a line, left a letter", cursor(t) == (2, 5 + 2), cursor(t))
    time.sleep(0.8)
    t.send(KEYS["ctrl+right"] + KEYS["ctrl+down"])
    check("Ctrl+Right+Down together = down a line, right a letter", cursor(t) == (3, 5 + 3), cursor(t))

    # ---- highlighting ----
    t.key("ctrl+home")
    t.key("shift+right")
    check("Shift+Right highlights a word", "3 selected" in t.row(ROWS - 2), t.row(ROWS - 2))
    t.key("esc", "ctrl+home", "right", "right")
    check("Right moves one word, highlighting nothing", cursor(t) == (2, 5 + 9) and "selected" not in t.row(ROWS - 2),
          (cursor(t), t.row(ROWS - 2)))
    t.key("left")
    check("Left moves back one word", cursor(t) == (2, 5 + 4), cursor(t))
    t.key("ctrl+right")
    check("Ctrl+Right moves one character", cursor(t) == (2, 5 + 5) and "selected" not in t.row(ROWS - 2),
          (cursor(t), t.row(ROWS - 2)))
    t.key("ctrl+shift+right", "ctrl+shift+right")
    check("Ctrl+Shift+Right highlights one character at a time", "2 selected" in t.row(ROWS - 2), t.row(ROWS - 2))
    t.key("esc")
    t.key("esc")

    # ---- typing, copy, paste, save ----
    t.key("alt+e", "enter")
    t.type("x = 1")
    t.key("ctrl+c")                       # nothing selected: copies the line
    t.key("ctrl+p")                       # paste (fallback clipboard here)
    t.send("\x1b[200~# pasted\x1b[201~")  # what kitty's Ctrl+V sends
    t.key("ctrl+s", wait=0.3)
    saved = open(path).read()
    want = ("def greet(name):\n    return 'hi ' + name  # say hi\n\nprint(greet('ember'))\n"
            "x = 1\nx = 1# pasted\n")
    check("typing + Ctrl+C + Ctrl+P + paste + Ctrl+S saved right", saved == want, repr(saved))
    check("save message shown", "Saved demo.py" in t.row(ROWS - 1), t.row(ROWS - 1))

    # ---- moving text ----
    t.key("alt+up")
    t.key("ctrl+s", wait=0.3)
    lines = open(path).read().splitlines()
    check("Alt+Up moves the line up (saved to the file)", lines[-2] == "x = 1# pasted" and lines[-1] == "x = 1", lines[-2:])

    # ---- find ----
    t.key("ctrl+home", "ctrl+f")
    t.type("greet")
    check("Ctrl+F finds and counts", "Match 1 of 2" in t.row(ROWS - 1), t.row(ROWS - 1))
    t.key("enter")
    t.key("f3")
    check("F3 goes to the next match", cursor(t)[0] == 5, cursor(t))

    # ---- undo ----
    t.key("ctrl+z", "ctrl+z")
    t.key("ctrl+s", wait=0.3)
    check("Ctrl+Z undoes", open(path).read().endswith("print(greet('ember'))\nx = 1\nx = 1\n"), repr(open(path).read()[-40:]))

    # ---- autosave: no Ctrl+S ----
    t.key("alt+e", "enter")
    t.type("# autosaved")
    time.sleep(0.4)
    check("saves by itself after a change (no Ctrl+S)", open(path).read().endswith("# autosaved\n"),
          repr(open(path).read()[-30:]))
    check("status bar shows the saving tick (words when there is room)", "✓" in t.row(ROWS - 2), t.row(ROWS - 2))

    # ---- load list (Alt+L) after the file changes on disk ----
    with open(path, "a") as f:
        f.write("# added outside\n")
    t.key("alt+l", wait=0.3)
    check("Alt+L shows the load list", "this file again from disk" in t.text(), t.text()[:300])
    t.key("enter", wait=0.3)
    check("loads the file again from disk", "# added outside" in t.text(), t.text()[-200:])

    # ---- help ----
    t.key("f1", wait=0.3)
    check("F1 shows the key list", "try me" in t.text() and "Ctrl + Left + Up" in t.text() and "highlighter" in t.text())
    t.key("esc")

    # ---- quit: saved already, so no question ----
    t.type("zz")
    t.key("ctrl+q")
    check("Ctrl+Q no longer quits", not t.exited(0.3))
    t.send("\x1bq")
    check("Alt+Q quits at once", t.exited(), t.row(ROWS - 1))
    check("the last letters were saved before quitting", "zz" in open(path).read())
    t.close()

    # ---- sessions: sy with no file ----
    s1 = Term([])
    check("sy alone starts a new session (status bar), with a made-up name on its tab",
          "new session" in s1.row(ROWS - 2) and ".txt" in s1.row(1), (s1.row(1), s1.row(ROWS - 2)))
    s1.type("#include <stdio.h>\nint main(void) {\nprintf(\"hi\");\n}")
    time.sleep(0.4)
    check("session coloured as C while typing", " C " in s1.row(ROWS - 2) or "·  C  ·" in s1.row(ROWS - 2), s1.row(ROWS - 2))
    s1.send("\x1bq")
    check("Alt+Q quits a session at once", s1.exited())
    sessions = os.listdir(os.path.join(FAKE_HOME, "data", "symbolizer", "sessions"))
    check("the session was kept on disk", len(sessions) == 1, sessions)
    s2 = Term([], cwd=FAKE_HOME)
    check("sy again carries on with today's session", "printf" in s2.text(), s2.text()[:200])
    # Ctrl+S names it; .c is added because it looks like C
    s2.key("ctrl+s")
    check("Ctrl+S asks for a name and says .c", "Name it (C, .c is added)" in s2.row(ROWS - 1), s2.row(ROWS - 1))
    s2.type("hello")
    s2.key("enter", wait=0.4)
    named = os.path.join(FAKE_HOME, "hello.c")
    check("saved as hello.c, with its line breaks", os.path.exists(named) and "printf" in open(named).read()
          and open(named).read().count("\n") >= 3,
          os.listdir(FAKE_HOME))
    check("the session moved into hello.c", os.listdir(os.path.join(FAKE_HOME, "data", "symbolizer", "sessions")) == [])
    s2.close()

    # ---- a folder as a project: file manager and tabs (on a copy) ----
    proj = os.path.join(tmp, "proj")
    os.makedirs(os.path.join(proj, "src"))
    open(os.path.join(proj, "src", "main.go"), "w").write("package main\n\nfunc main() {}\n")
    open(os.path.join(proj, "README.md"), "w").write("# Project\n")
    p = Term([proj])
    check("sy FOLDER shows the file manager", "src/" in p.text() and "README.md" in p.text(), p.text()[:400])
    p.type("main")
    p.key("enter", wait=0.4)
    check("type to find + Enter opens the file", "main.go" in p.row(1) and "Go" in p.row(ROWS - 2), p.row(ROWS - 2))
    p.send("\x1bo", 0.2)                            # Alt+O
    p.type("read")
    p.key("enter", wait=0.4)
    check("second file opens in a second tab", "1 main.go" in p.row(1) and "2 README.md" in p.row(1), p.row(1))
    p.send("\x1b1", 0.3)                            # Alt+1
    check("Alt+1 goes to the first tab", "Go" in p.row(ROWS - 2), p.row(ROWS - 2))
    p.send("\x1b[<0;30;2M\x1b[<0;30;2m", 0.3)       # mouse click on the tab bar
    p.close()
    check("tabs and file manager worked on a copy only", open(os.path.join(proj, "README.md")).read() == "# Project\n")

    # ---- kitty: real held keys ----
    path2 = os.path.join(tmp, "grid.go")                # code: left-aligned, so columns are fixed
    with open(path2, "w") as f:
        f.write("a0cdef\nb1cdef\nc2cdef\nd3cdef\n")
    k = Term([path2], kitty=True)
    asked = b"\x1b[?u" in k.raw
    check("sy asks the terminal for key-release events (kitty protocol)", asked)
    k.key("alt+e")                                  # line 4, col 6
    k.send("\x1b[1;5D")                             # Ctrl+Left pressed (held)
    time.sleep(0.4)                                 # a slow second key
    k.send("\x1b[1;5A")                             # Ctrl+Up pressed while Left held
    check("kitty: hold Ctrl+Left, then Up = up a line + left a letter",
          cursor(k) == (4, 5 + 5), cursor(k))
    k.send("\x1b[1;5:2A")                           # key repeat of Up
    check("kitty: keep holding = keeps moving diagonally", cursor(k) == (3, 5 + 4), cursor(k))
    k.send("\x1b[1;5:3A\x1b[1;5:3D")                # both let go
    time.sleep(0.2)
    k.send("\x1b[1;5A")                             # Ctrl+Up alone
    check("kitty: after letting go, Ctrl+Up alone moves up a line",
          cursor(k) == (2, 5 + 4) and "a0cdef" in k.row(2), (cursor(k), k.row(2)))
    k.send("\x1b[1;5:3A")
    for code, mod, word in ((57441, 2, "highlighter"), (57442, 5, "position"), (57443, 3, "movement")):
        k.send(f"\x1b[{code};{mod}u", 0.2)           # Left Shift / Ctrl / Alt pressed on its own
        check(f"kitty: holding the key shows {word}", word in k.row(ROWS - 2), k.row(ROWS - 2))
        k.send(f"\x1b[{code};1:3u", 0.2)             # let go
        check(f"kitty: letting go shows text again", word not in k.row(ROWS - 2) and "text" in k.row(ROWS - 2),
              k.row(ROWS - 2))
    k.send("\x1b[99;5u")                            # Ctrl+C (kitty's code) must not quit
    check("Ctrl+C does not close the editor", not k.exited(0.3))
    k.send("\x1b[113;3u")                           # Alt+Q (kitty's code)
    check("kitty: Alt+Q quits to the terminal", k.exited())
    k.close()

    # ---- UK keyboard: ß saves, ø opens (sent as typed text) ----
    uk = os.path.join(tmp, "uk.txt")
    u = Term([uk])
    u.type("hello")
    u.send("ß".encode(), 0.4)
    check("ß saves and is not typed (Right Alt+S on a UK keyboard)",
          os.path.exists(uk) and open(uk).read() == "hello\n")
    u.send("ø".encode(), 0.3)
    check("ø opens the file manager and is not typed (Right Alt+O)", "type letters to find" in u.text()
          and "ø" not in u.text(), u.text()[:300])
    u.close()

    # ---- tabs: tab mode (AltGr+Tab), Alt+N, Alt+C (kitty's codes) ----
    proj2 = os.path.join(tmp, "proj2")
    os.makedirs(proj2)
    for n in ("a.txt", "b.txt", "c.txt"):
        open(os.path.join(proj2, n), "w").write(n + "\n")
    k2 = Term([proj2], kitty=True)
    for i, n in enumerate(("a.txt", "b.txt", "c.txt")):
        if i:                                          # a project starts in the file manager
            k2.send("\x1b[111;3u", 0.2)               # Alt+O
        k2.type(n)
        k2.key("enter", wait=0.3)
    on = lambda: k2.row(ROWS - 2)
    check("three files open in three tabs", "c.txt" in on() and "3 c.txt" in k2.row(1), k2.row(1))
    altgr_tab = "\x1b[57453u\t\x1b[57453;1:3u"         # AltGr down, Tab, AltGr up
    k2.send(altgr_tab, 0.5)
    check("AltGr+Tab opens tab mode", "tabs" in on(), on())
    k2.send("\x1b[C", 0.5)                           # Right
    check("tab mode: Right goes round to the first tab", "a.txt" in on(), on())
    k2.send("\x1b[D\x1b[D", 0.5)                     # Left, Left
    check("tab mode: Left goes back", "b.txt" in on(), on())
    k2.send("\r", 0.5)                               # Enter
    check("Enter keeps the file and leaves tab mode", "b.txt" in on() and "tabs" not in on(), on())
    k2.send(altgr_tab + "\x1b[D\x1b[27u", 0.8)       # AltGr+Tab, Left, Esc
    check("Esc goes back to where tab mode started", "b.txt" in on(), on())
    k2.send("\x1b[9;17u", 0.5)                       # Hyper+Tab, as the Hyprland helper sends it
    check("Hyper+Tab opens tab mode too", "tabs" in on(), on())
    k2.send("\x1b[C\r", 0.5)                         # Right, Enter
    check("and its arrows switch tabs", "c.txt" in on() and "tabs" not in on(), on())
    k2.send("\x1b[9;17u\x1b[D\r", 0.5)               # back to b.txt for the checks below
    k2.send("\x1b[9;3u", 0.3)                        # Alt+Tab
    time.sleep(0.3)
    check("Alt+Tab types a real Tab (indent)", open(os.path.join(proj2, "b.txt")).read() == "    b.txt\n",
          repr(open(os.path.join(proj2, "b.txt")).read()))
    k2.send("\x1b[9;4u", 0.5)                        # Shift+Alt+Tab
    check("Shift+Alt+Tab un-indents", open(os.path.join(proj2, "b.txt")).read() == "b.txt\n",
          repr(open(os.path.join(proj2, "b.txt")).read()))
    k2.send("\x1b[99;3u", 0.3)                       # Alt+C
    check("Alt+C closes the file", "3 " not in k2.row(1) and "b.txt" not in k2.row(1), k2.row(1))
    k2.send("\x1b[110;3u", 0.3)                      # Alt+N
    check("Alt+N opens a new tab with a made-up ember name", re.search(r"3 [a-z]+-[a-z]+\.txt", k2.row(1)) is not None, k2.row(1))
    k2.close()

    # ---- modes (Alt+M) ----
    md = Term([shot_file := os.path.join(tmp, "modes.txt")])
    md.type("keep me")
    md.send("\x1bm", 0.4)                             # Alt+M
    check("Alt+M shows the modes box", "M O D E S" in md.text() and "Design" in md.text() and "Draw" in md.text())
    md.send("2", 0.4)
    check("2 switches to Design (empty for now)", "Design mode" in md.text() and "Design" in md.row(0), md.row(0))
    md.type("zzz")
    md.send("\x1bm", 0.3)
    md.send("1", 0.4)
    time.sleep(0.3)
    check("back in Text; typing in Design changed nothing", open(shot_file).read() == "keep me\n" and "Text" in md.row(0),
          repr(open(shot_file).read()))
    md.close()

    # ---- sy design / slide / diary start in that mode ----
    for word, name in (("design", "Design"), ("slide", "Slides"), ("diary", "Diary"), ("book", "Book"), ("draw", "Draw")):
        w = Term([word])
        check(f"sy {word} starts in {name} mode", f"{name} mode" in w.text() and name in w.row(0), w.row(0))
        w.send("\x1bq", 0.2)
        w.close()
    w = Term([])
    check("sy alone starts in Text mode", "Text" in w.row(0) and "mode" not in w.row(ROWS // 2), w.row(0))
    w.close()

    # ---- the line from the screenshot: Left / Right steps ----
    shot = os.path.join(tmp, "shot.c")
    open(shot, "w").write("char *parts[MAX_PARTS];\n")
    c = Term([shot])
    got = []
    for _ in range(9):
        c.key("right", wait=0.08)
        got.append(cursor(c)[1] - 5)
    check("Right on char *parts[MAX_PARTS]; stops after char * parts [ MAX _ PARTS ] ;",
          got == [4, 6, 11, 12, 15, 16, 21, 22, 23], got)
    back = []
    for _ in range(9):
        c.key("left", wait=0.08)
        back.append(cursor(c)[1] - 5)
    check("Left goes back to the start of each step", back == [22, 21, 16, 15, 12, 11, 6, 5, 0], back)
    c.close()

    # ---- big file: colours follow the screen ----
    big = os.path.join(tmp, "big.py")
    with open(big, "w") as f:
        for i in range(20000):
            f.write(f"def f{i}(x):\n    return x + {i}  # line\n")
    b = Term([big])
    b.key("alt+e", wait=0.6)
    row = next((y for y in range(ROWS) if "def f19999" in b.row(y)), 0)
    line = b.row(row)
    kw = b.screen.buffer[row][line.index("def")].fg if "def" in line else ""
    check("big file: jump to the end, colours follow", kw.lower() == "ff3704", f"{line!r} {kw}")

    # ---- window resize ----
    import fcntl, struct, termios
    fcntl.ioctl(b.fd, termios.TIOCSWINSZ, struct.pack("HHHH", 20, 70, 0, 0))
    b.screen.resize(20, 70)
    os.kill(b.pid, signal.SIGWINCH)
    b.pump(0.5)
    check("redraws after the window is resized", "Ln 40000" in b.row(18), b.row(18))
    b.close()

    # ---- terminals with fewer colours ----
    for term, colorterm, label in (("xterm-256color", "", "256 colours"), ("linux", "", "16 colours (Linux console)")):
        c = Term([path], env={"TERM": term, "COLORTERM": colorterm})
        ok = "S Y M B O L I Z E R" in c.row(0) and "def greet" in c.row(2) and "Ln 1, Col 1" in c.row(ROWS - 2)
        c.send("\x1b[F")                            # End
        ok = ok and cursor(c)[1] > 5
        check(f"works in a {label} terminal", ok, c.row(0))
        c.send("\x1bq")                              # Alt+Q, older terminals' code
        check(f"Alt+Q quits at once when saved ({label})", c.exited())
        c.close()

    # ---- the central zone: drag a margin handle with the mouse ----
    zf = os.path.join(tmp, "zone.go")
    open(zf, "w").write("package main\n\nfunc main() {}\n")
    zt = Term([zf])
    start = zt.row(2).index("1 package")
    zt.screen.resize(ROWS, 160)
    import fcntl, struct, termios
    fcntl.ioctl(zt.fd, termios.TIOCSWINSZ, struct.pack("HHHH", ROWS, 160, 0, 0))
    os.kill(zt.pid, signal.SIGWINCH)
    zt.pump(0.5)
    wide_start = zt.row(2).index("1 package")
    check("on a wide window the text sits in a centred zone", wide_start > 20, (start, wide_start))
    handle = max(i for i, ch in enumerate(zt.row(3)[:wide_start]) if ch in "│┃")
    zt.send(f"\x1b[<0;{handle+1};4M\x1b[<32;{handle+11};4M\x1b[<0;{handle+11};4m", 0.5)
    moved = zt.row(2).index("1 package")
    check("dragging the left handle 10 columns in moves the text 10 columns", moved in (wide_start + 9, wide_start + 10), (wide_start, moved))
    check("the new width is shown and remembered", "Central zone:" in zt.text() and "Remembered" in zt.text(), zt.row(ROWS - 1))
    zt.close()

    # ---- lock-key lamps in the status bar (outside kitty) ----
    run = tempfile.mkdtemp()
    open(os.path.join(run, "emberline-locks.json"), "w").write('{"caps": false, "num": true, "scroll": false, "fn": true}')
    lp = Term([os.path.join(tmp, "lamps.txt")], env={"XDG_RUNTIME_DIR": run, "KITTY_WINDOW_ID": ""})
    time.sleep(1.2); lp.pump(0.2)
    row = lp.row(ROWS - 2)
    check("status bar shows the lock lamps", "A 1 ↕ ƒ" in row, row)
    i = row.index("A 1 ↕ ƒ")
    colours = [lp.screen.buffer[ROWS - 2][i + k].fg for k in (0, 2)]
    check("Caps off is ash, Num on is gold", colours == ["6a2814", "ff8a1a"], colours)
    open(os.path.join(run, "emberline-locks.json"), "w").write('{"caps": true, "num": true, "scroll": false, "fn": true}')
    time.sleep(1.3); lp.pump(0.2)
    check("pressing Caps Lock lights its lamp within a second", lp.screen.buffer[ROWS - 2][i].fg == "ff8a1a",
          lp.screen.buffer[ROWS - 2][i].fg)
    lp.close()

    # ---- the lock plate, when sy fills the screen ----
    import json, socket, threading
    run = tempfile.mkdtemp()
    answer = {"title": "plate.txt — Symbolizer", "fullscreen": 2, "pid": os.getpid()}
    os.makedirs(os.path.join(run, "hypr", "e2e"))
    hypr = socket.socket(socket.AF_UNIX)
    hypr.bind(os.path.join(run, "hypr", "e2e", ".socket.sock"))
    hypr.listen()

    def pretend_hyprland():
        while True:
            try:
                c, _ = hypr.accept()
            except OSError:
                return
            if c.recv(64) == b"j/activewindow":
                c.sendall(json.dumps(answer).encode())
            c.close()
    threading.Thread(target=pretend_hyprland, daemon=True).start()

    def locks(caps):   # written the way emberline-locks writes it: a new file renamed over the old
        state = os.path.join(run, "emberline-locks.json")
        open(state + ".tmp", "w").write(json.dumps({"caps": caps, "num": True, "scroll": False, "fn": True}))
        os.replace(state + ".tmp", state)
    locks(False)
    pl = Term([os.path.join(tmp, "plate.txt")], env={"XDG_RUNTIME_DIR": run, "HYPRLAND_INSTANCE_SIGNATURE": "e2e"})
    pl.pump(0.3)
    check("lock plate: not there at the start", "SCROLL" not in pl.text())
    locks(True)
    pl.pump(0.3)
    rows = [y for y in range(ROWS) if "SCROLL" in pl.row(y)]
    check("fullscreen: Caps Lock brings the plate up at once", rows != [] and "CAPS" in pl.row(rows[0]), pl.text())
    if rows:
        y = rows[0]
        check("it sits at the bottom right, just above the info line", y == ROWS - 3 - 4 and pl.row(y).rstrip().endswith("│"),
              (y, pl.row(y)))
        x = pl.row(y).index("CAPS")
        lit, dark = pl.screen.buffer[y][x].fg, pl.screen.buffer[y][pl.row(y).index("SCROLL")].fg
        check("Caps is lit gold, Scroll (off) is ash", int(lit[:2], 16) > 200 and int(dark[:2], 16) < 120, (lit, dark))
    pl.pump(2.3)
    check("the plate fades away by itself", "SCROLL" not in pl.text(), pl.text())
    answer["fullscreen"] = 0
    locks(False)
    pl.pump(0.4)
    check("not fullscreen: no plate in sy (the desktop shows its own)", "SCROLL" not in pl.text())
    answer["fullscreen"], answer["title"] = 2, "zsh"
    locks(True)
    pl.pump(0.4)
    check("another window fullscreen: no plate in sy", "SCROLL" not in pl.text())
    pl.close()
    hypr.close()

    # ---- info line, weather and colours (at the end: themes are remembered) ----
    os.makedirs(os.path.join(FAKE_HOME, ".cache", "emberline"), exist_ok=True)
    open(os.path.join(FAKE_HOME, ".cache", "emberline", "weather.json"), "w").write(
        '{"latitude": 51.5, "longitude": -0.13, "_fetched": 1, "current": {"temperature_2m": 12, '
        '"apparent_temperature": 8, "relative_humidity_2m": 75, "is_day": 0, "weather_code": 51, '
        '"wind_speed_10m": 17, "wind_direction_10m": 266, "wind_gusts_10m": 36}, '
        '"daily": {"temperature_2m_max": [13], "temperature_2m_min": [9]}, "hourly": {"time": [], "precipitation_probability": []}}')
    e2e_wide = Term([os.path.join(tmp, "colour.txt")])
    time.sleep(0.5)
    e2e_wide.pump(0.3)
    info = e2e_wide.row(ROWS - 3)
    check("info line: time and date in words (the mode is in the top panel)", "Text mode" not in info and any(
        w in info for w in ("past", "to", "o'clock", "MIDNIGHT", "NOON")), info)
    check("info line: weather from the shared cache", "Light drizzle 12°" in info and "feels 8°" in info, info)
    e2e_wide.send("ŧ".encode(), 0.4)
    check("ŧ opens the colour switcher", "C O L O U R S" in e2e_wide.text())
    e2e_wide.send("\x1b[C", 0.4)
    check("moving shows the colours at once", near(e2e_wide.screen.buffer[4][2].bg, "1c1c1c"),
          e2e_wide.screen.buffer[4][2].bg)
    e2e_wide.send("\x1b", 0.4)
    check("Esc puts the old colours back", near(e2e_wide.screen.buffer[4][2].bg, "161616"),
          e2e_wide.screen.buffer[4][2].bg)
    e2e_wide.close()

    failed = [n for n, ok in results if not ok]
    print(f"\n{len(results) - len(failed)} of {len(results)} checks passed.")
    sys.exit(1 if failed else 0)


if __name__ == "__main__":
    main()
