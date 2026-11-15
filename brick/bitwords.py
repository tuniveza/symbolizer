"""bitwin words: brick's own windows, in plain English (see docs/bitwin-contract.md).

A Compiler mixin, like window.Window. It reads the window words and turns them into calls to the bitwin
runtime (bw_ routines) and stores into ELEMENT records. "the window" always means brick's own window; the
console words (window.py) say "console" or "terminal".

  open a window called My Game, 800 by 600, in navy           (or: make the window 800 by 600)
  make a button called start saying Start at 20 20, 120 by 40, in orange with round corners
  put a text called score at the top saying Score: 0, in gold, size 24
  make a canvas called board at 20 80, 400 by 300 with a gradient from black to navy going down
  make a menu called file with New, Open and Quit     /  make tabs called pages with Home and About
  when start is clicked, set the text of score to "Score: 1"     (or an indented block under "when ...:")
  draw a red circle on board at 50 50, 30 by 30   /   move start right 20   /   hide start

Programs that never use these words are built byte for byte as before: everything here only starts
when the program's text mentions a window word (gui_on), and only adds routines when one is used (uses_gui).
"""
import re

from actions import colour_at, list_items
from asm import EQUAL, NOT_EQUAL, R8, RAX, RCX, RDX
from expressions import Problem, read_value
from reader import Word, group_parts, read_number, split_line, words_of

# ---- ELEMENT record offsets (docs/bitwin-contract.md) ----
KIND, NAME, PARENT, TAB = 0, 8, 16, 24
X, XU, Y, YU, W, WU, H, HU = 32, 40, 48, 56, 64, 72, 80, 88
OTHER, GAP, AX, AY, AW, AH, DEPTH, FLAGS = 96, 104, 112, 120, 128, 136, 144, 152
TEXT, CLICK, ITEMS, PICKED, DATA, CHANGE = 160, 168, 176, 184, 192, 200
S = 256                                         # the STYLE block inside the record
FILL, COLOUR, COLOUR2, DIRECTION, TEXTURE, TEXTURE_COLOUR, TEXTURE_SIZE = 0, 8, 16, 24, 32, 40, 48
TRANSPARENCY, RADIUS, BORDER, BORDER_COLOUR, SHADOW, TEXT_COLOUR, TEXT_SIZE, ALIGN = 56, 64, 72, 80, 88, 96, 104, 112
HOVER, HOVER2, HOVER_TEXT = 384, 392, 400
HIDDEN, DISABLED, OPEN, HAS_HOVER = 1, 2, 32, 64
PX, PERCENT, FAR, CENTRED, AFTER, BEFORE, ALIGNED, FIT, PER_MILLE = range(9)

WINDOW, BUTTON, CANVAS, MENU, TABS, TEXTKIND, GLVIEW, BOX, INPUT = range(1, 10)
KIND_NAMES = {WINDOW: "window", BUTTON: "button", CANVAS: "canvas", MENU: "menu", TABS: "tabs", TEXTKIND: "text",
              GLVIEW: "3D view", BOX: "box", INPUT: "input box"}
KINDS = sorted([(p.split(), k) for k, ps in {
    INPUT: ["input box", "text box", "input field", "text field", "textbox", "input", "entry box", "entry field",
            "edit box", "typing box"],
    GLVIEW: ["gl context", "gl view", "gl window", "gl canvas", "3d view", "3d scene", "3d space", "3d canvas",
             "opengl view", "opengl context", "context", "gl", "3d"],
    TABS: ["tab bar", "tab strip", "set of tabs", "row of tabs", "tabs"],
    MENU: ["drop down menu", "dropdown menu", "drop-down menu", "dropdown", "menu"],
    CANVAS: ["drawing area", "drawing board", "sketch pad", "canvas"],
    BUTTON: ["push button", "button", "btn"],
    TEXTKIND: ["text", "label", "heading", "caption"],
    BOX: ["box", "panel", "rectangle", "card"],
    WINDOW: ["window"],
}.items() for p in ps], key=lambda pk: -len(pk[0]))
KIND_NOUNS = {p[-1] for p, _ in KINDS} | {"buttons", "menus", "canvases", "boxes", "panels", "tab", "view"}

GUI_HINT = re.compile(r"\b(?:windows?|buttons?|canvas(?:es)?|menus?|tabs?|gl|3d|context|input box|text box|"
                      r"textbox|panels?|boxes|box)\b", re.IGNORECASE)
GUI_STARTERS = {"open", "draw", "hide", "disable", "enable", "bring", "send", "give", "round", "close", "shut",
                "spin", "paint", "grey", "gray", "unhide", "rotate"}
CREATE = {"make", "add", "put", "create", "place", "insert", "open", "build", "draw", "give", "spawn", "pop"}
WRITE = {"write", "say", "show", "print", "display", "output", "put", "add", "place", "type"}
ARTICLES = {"a", "an", "the", "my", "your", "some", "another", "new", "one", "our", "this"}
FILLER = {"a", "an", "the", "with", "and", "it", "its", "that", "which", "is", "are", "has", "have", "having",
          "also", "too", "then", "plus", "some", "well", "be", "should", "please", "so", "looks", "look", "now",
          "new", "nice", "pretty", "lovely", "little", "real", "proper", "also", "make", "let", "lets", "let's",
          "gets", "get", "given", "giving", "being", "both", "all", "to", "can", "you", "of", "one", "just"}
PEOPLE = ("someone", "somebody", "you", "i", "we", "they", "the user", "the player", "anyone", "a person", "the person",
          "people", "user", "player")
MARGIN = 10                     # px from the edge for "at the top right"
SCREEN = (1920, 1080)          # what "50% by 50%" of the screen means for a window (wl_open takes px)
REAL_UNITS = {"pt": 4 / 3, "mm": 3.78, "cm": 37.8, "in": 96}
UNIT_WORDS = {"px": "px", "pixels": "px", "pixel": "px", "pxs": "px", "%": "%", "percent": "%", "pc": "%",
              "pct": "%", "pt": "pt", "pts": "pt", "points": "pt", "point": "pt", "mm": "mm", "millimetres": "mm",
              "millimeters": "mm", "millimetre": "mm", "millimeter": "mm", "cm": "cm", "centimetres": "cm",
              "centimeters": "cm", "centimetre": "cm", "centimeter": "cm", "inch": "in", "inches": "in", "in": "in",
              "em": "em", "ems": "em"}
TEXTURES = {"stripes": 1, "striped": 1, "stripy": 1, "stripey": 1, "diagonal stripes": 1, "diagonal lines": 1,
            "lines": 2, "lined": 2, "horizontal lines": 2, "horizontal stripes": 2, "ruled": 2, "ruled lines": 2,
            "vertical lines": 3, "vertical stripes": 3, "pinstripes": 3, "pinstriped": 3, "pinstripe": 3,
            "dots": 4, "dotted": 4, "spots": 4, "spotted": 4, "spotty": 4, "polka dots": 4, "polka dot": 4,
            "polka dotted": 4, "dotty": 4, "checks": 5, "checked": 5, "checkered": 5, "chequered": 5,
            "checkerboard": 5, "chequerboard": 5, "check": 5, "checkers": 5, "chequers": 5, "checker": 5,
            "grain": 6, "grainy": 6, "noise": 6, "noisy": 6, "speckled": 6, "speckles": 6, "sandy": 6,
            "gritty": 6, "grit": 6, "film grain": 6, "grained": 6, "bricks": 7, "brick": 7, "bricky": 7,
            "brickwork": 7, "brick wall": 7, "brick pattern": 7, "grid": 8, "gridded": 8, "grid lines": 8,
            "squared": 8, "graph paper": 8, "gridlines": 8}
TEXTURE_SIZES = {"small": 4, "fine": 4, "thin": 4, "tiny": 3, "narrow": 4, "big": 16, "large": 16, "thick": 16,
                 "wide": 16, "chunky": 16, "huge": 24, "fat": 16, "medium": 8}
DIRECTIONS = [("from top to bottom", 0), ("top to bottom", 0), ("from the top to the bottom", 0), ("going down", 0),
              ("downwards", 0), ("downward", 0), ("down", 0), ("vertically", 0), ("vertical", 0),
              ("from bottom to top", 1), ("bottom to top", 1), ("going up", 1), ("upwards", 1), ("upward", 1),
              ("up", 1), ("from left to right", 2), ("left to right", 2), ("going right", 2), ("going across", 2),
              ("across", 2), ("horizontally", 2), ("horizontal", 2), ("sideways", 2), ("to the right", 2),
              ("rightwards", 2), ("right", 2), ("left", 3), ("from right to left", 3), ("right to left", 3), ("going left", 3),
              ("to the left", 3), ("leftwards", 3), ("going down right", 4), ("down right", 4),
              ("diagonally down", 4), ("diagonally", 4), ("diagonal", 4), ("going diagonally", 4),
              ("going up right", 5), ("up right", 5), ("diagonally up", 5)]
DIRECTIONS.sort(key=lambda d: -len(d[0].split()))
HOVER_WORDS = {"mouse", "hover", "hovered", "hovering", "hovers", "pointer", "cursor", "moused", "mouseover", "over"}
SHAPES = {"circle": 2, "ellipse": 2, "oval": 2, "disc": 2, "disk": 2, "dot": 6, "point": 6, "spot": 6, "blob": 6,
          "box": 1, "square": 1, "rectangle": 1, "rect": 1, "block": 1, "line": 3, "triangle": 4}
GL_SHAPES = {"cube", "cubes", "sphere", "spheres", "ball", "pyramid", "cone", "cylinder", "torus", "donut",
             "doughnut", "teapot", "prism", "plane", "floor", "camera", "light", "shape", "shapes", "block"}
GUI_EXAMPLES = {
    "window": ["open a window called My Game, 800 by 600, in navy", "open a resizable window 50% by 50%",
               "make the window 800 by 600", "set the title of the window to My Game", "close the window"],
    "button": ["make a button called start saying Start at 20 20, 120 by 40, in orange with round corners",
               "add a button saying Quit at the bottom right", "make start turn yellow when the mouse is over it",
               "when start is clicked, write hi"],
    "text": ["put a text called score at the top saying Score: 0, in gold, size 24",
             "set the text of score to \"Score: \" joined with points", "write hi in the window"],
    "canvas": ["make a canvas called board at 20 80, 400 by 300 with a gradient from black to navy going down",
               "draw a red circle on board at 50 50, 30 by 30", "draw a line on board from 0 0 to 100 100 in white",
               "write hello on board at 10 10 in gold", "clear board"],
    "menu": ["make a menu called file with New, Open and Quit", "when New is chosen from file, clear board"],
    "tabs": ["make tabs called pages with Home, Settings and About", "put start on the Home tab",
             "when the Settings tab is picked, write settings"],
    "place": ["put start at 20 20", "start is 20 from the right and 20 from the bottom", "put start below board",
              "move start to the top right", "move start right 20", "make start 2 cm wide and 1 cm tall"],
    "depth": ["put start in front of board", "put start behind board", "bring start to the front",
              "send panel to the back", "set the depth of start to 5"],
    "style": ["make start red", "give start a gradient from red to orange", "make board checked in grey",
              "give start a 2 px gold border", "give panel a shadow", "make panel half see-through",
              "round the corners of start by 12", "make the text of start white and size 20"],
    "when": ["when start is clicked, write hi", "when a key is pressed, write the key",
             "when the mouse moves, move ball to the mouse x and the mouse y", "every 100 milliseconds, move ball right 5",
             "when the window is closed, write bye", "wait until start is clicked"],
}


def joined(words):
    """'"Score: " joined with points' -> joined to: the value reader's way of putting texts together."""
    out = []
    for k, w in enumerate(words):
        if w.word in ("with", "onto", "and") and k and words[k - 1].word in ("joined", "join", "added", "stuck",
                                                                           "glued"):
            out.append(Word("to"))
            continue
        out.append(w)
    return out


def colour_value(code):
    """'38;2;255;165;0' -> 0xFFA500."""
    r, g, b = (int(c) for c in code.split(";")[2:5])
    return r << 16 | g << 8 | b


def split_glued(words):
    """'800x600' -> 800 x 600, '10%' -> 10 %, '20px' -> 20 px, 'x=20' -> x = 20, 'top-right' -> top right."""
    out = []
    for w in words:
        if w.quoted:
            out.append(w)
            continue
        parts = re.split(r"(?<=\d)(px|%|pt|mm|cm|em|x|×)(?=\d|[^\w]|$)|(=)|(?<=[a-z])-(?=to-)|(?<=-to)-", w.raw,
                         flags=re.IGNORECASE)
        parts = [p for p in parts if p]
        pieces = []
        for p in parts:
            m = re.fullmatch(r"(top|bottom|upper|lower|up|down|middle)-(left|right|centre|center)([,.;:]?)", p, re.I)
            if m:
                pieces += [m[1], m[2] + m[3]]
            elif re.fullmatch(r"[a-z]+-to", p, re.I):
                pieces += p.split("-")
            else:
                pieces.append(p)
        out += [Word(p) for p in pieces]
    return out


class Elem:
    """An element the program made: its kind, its name, and the slot that holds its index."""

    def __init__(self, name, kind, slot):
        self.name, self.kind, self.slot = name, kind, slot
        self.items = {}              # item text (lower case) -> slot holding its item number
        self.item_names = []
        self.view = None             # gl views: slot holding the GLVIEW address
        self.text_size = 16


class Toks:
    """The words of a phrase, with units and sizes unglued, read from any position."""

    def __init__(self, words):
        self.words = split_glued(words)
        self.w = [x.word for x in self.words] + [None] * 8
        self.n = len(self.words)

    def at(self, i, *phrases):
        """How many words one of the phrases covers at i (0 if none). The longest one wins."""
        best = 0
        for phrase in phrases:
            parts = phrase.split()
            if len(parts) > best and self.w[i:i + len(parts)] == parts:
                best = len(parts)
        return best

    def comma(self, i, colon=True):
        return 0 <= i < self.n and self.words[i].after[-1:] in ((",", ";", ":") if colon else (",", ";"))

    def raw(self, i, j):
        """The words i..j as typed (quotes kept as the text inside), without a comma at the end."""
        shown = []
        for w in self.words[i:j]:
            shown.append(w.text + w.after if w.quoted else w.raw)
        text = " ".join(shown)
        return re.sub(r"[,;:]$", "", text).strip()


class BitWords:
    """Mixed into the Compiler: the window words."""

    def gui_setup(self, text=""):
        self.gui_on = bool(GUI_HINT.search(text))   # does this program talk about windows at all?
        self.uses_gui = False                       # has a window word been compiled? Then it needs bitwin.
        self.gui_names = {}                         # name or label (lower case) -> Elem
        self.gui_order = []
        self.gui_window = Elem("window", WINDOW, None)
        self.gui_lines = set()                      # lines read as window words (not for the misread checks)
        self.gui_slots = {}
        self.gl_names = set()
        self.gui_drawn = set()                      # the shapes it draws on canvases (1 box ... 6 dot), for bitwin
        self.gui_looks = set()                      # "shadow", "texture": looks some style may get, for paint
        self.gui_text_sizes = {16}                  # for paint.add_all: the letters the program needs                       # 3D things the program named ("a red cube called box")

    # ---------------- where window words start ----------------

    def gui_starts(self, w):
        return getattr(self, "uses_gui", False) and w in GUI_STARTERS      # once a window word is compiled

    def gui_keeps(self, words):
        """Should these words stay as typed (not respelled by the dictionary: 'close the window' isn't 'stop')?"""
        if not getattr(self, "gui_on", False) or not words:
            return False
        try:
            return self.gui_read(words) is not None
        except Problem:
            return True

    def gui_whole_line(self, line):
        """A whole line that is one window phrase, commas and all ('make a menu called file with New, Open and
        Quit'): its words, else None."""
        if not self.gui_on or re.search(r"\bthen\b|;", line):
            return None
        words = words_of(line)
        if len(words) < 3 or not (any(w.after[-1:] in (",", ":") for w in words) or
                                  any(w.word == "and" for w in words)):
            return None
        try:
            plan = self.gui_read(self.drop_filler(words), whole=True)
        except Problem:
            return None
        return words if plan else None

    # ---------------- the runtime state ----------------

    def gui_slot(self, name, size=8):
        if name not in self.gui_slots:
            self.gui_slots[name] = self.a.reserve(size, "bww_" + name)
        return self.gui_slots[name]

    def gui_use(self):
        self.uses_gui = True
        self.gui_lines.add(self.line_number)
        for name in ("opened", "title", "w", "h", "resizable"):
            self.gui_slot(name)

    def gui_need_window(self):
        self.gui_use()
        self.a.call("bww_window")

    def gui_index(self, elem):
        """rax = the element's index."""
        if elem.kind == WINDOW:
            self.a.set(RAX, 0)
        else:
            self.a.address(RAX, elem.slot)
            self.a.load(RAX, RAX)

    def gui_record(self, elem):
        """rax = the address of the element's record."""
        self.gui_index(elem)
        self.a.call("bw_elem")

    def gui_finish(self):
        """At the end of the program: if it made a window, it stays open until it's closed."""
        if self.uses_gui:
            self.a.call("bww_window")
            self.a.call("bw_run")

    def gui_routines(self):
        if self.uses_gui:
            add_routines(self.a, self.gui_slots, sizes=sorted(self.gui_text_sizes), gl=any(e.kind == GLVIEW for e in self.gui_order) or
                         getattr(self, "uses_glspace", False),
                         kinds={WINDOW} | {e.kind for e in self.gui_order}, drawn=self.gui_drawn,
                         looks=self.gui_looks_used())

    def gui_note_looks(self, effects):
        """Note the looks these style effects may give ("set"/"add" a shadow or texture that isn't 0)."""
        for e in effects:
            if e[0] in ("set", "add") and e[2] != 0:
                if e[1] == S + SHADOW:
                    self.gui_looks.add("shadow")
                elif e[1] == S + TEXTURE:
                    self.gui_looks.add("texture")

    def gui_looks_used(self):
        """What shapes can look like, for paint.add_all: menus' item lists have a shadow; circles and dots
        drawn on canvases are round."""
        looks = set(self.gui_looks)
        if any(e.kind == MENU for e in self.gui_order):
            looks.add("shadow")
        if self.gui_drawn & {2, 6}:
            looks.add("ellipse")
        return looks

    # ---------------- finding elements ----------------

    def gui_find(self, words, kinds=None):
        """The element these words name ('start', 'the start button', 'the window'), else None."""
        said = [w.word if w.word is not None else w.text.lower() for w in words]
        while said and said[0] in ("the", "my", "a", "an", "this", "that", "our", "your"):
            said = said[1:]
        if said and said[-1] in ("'s",):
            said = said[:-1]
        if said and said[0].endswith("'s"):
            said = [said[0][:-2]] + said[1:]
        if said in (["window"], ["windows"]) or said == ["whole", "window"]:
            return self.gui_window if kinds is None or WINDOW in kinds else None
        for cut in (0, 1):
            name = " ".join(said[:len(said) - cut])
            if cut and (not said or said[-1] not in KIND_NOUNS):
                continue
            elem = self.gui_names.get(name)
            if elem and (kinds is None or elem.kind in kinds):
                return elem
        return None

    def gui_element_at(self, t, i, kinds=None):
        """The element named at t[i] (one to four words, longest first) -> (elem, words used), else None."""
        for n in range(min(5, t.n - i), 0, -1):
            if any(t.w[i + k] in (",",) for k in range(n)):
                continue
            elem = self.gui_find(t.words[i:i + n], kinds)
            if elem:
                return elem, n
        return None

    def gui_item(self, words, menu=None):
        """('New', in menu file) -> (the menu or tabs element, item text) or None."""
        text = " ".join(w.text for w in words).strip(" ,.:").lower()
        text = re.sub(r"^(?:the|an?)\s+", "", text)
        holders = [menu] if menu else [e for e in self.gui_order if e.items]
        for elem in holders:
            for item in (text, re.sub(r"\s+(?:item|option|tab|page|entry|choice)$", "", text)):
                if item in elem.items:
                    return elem, item
        return None

    # ---------------- reading amounts and units ----------------

    def gui_amount(self, t, i, size_of=16):
        """A number or value with its unit at t[i] -> (value, unit code, words used), else None.
        value is a whole number or ("node", value tree). Real-world units become px."""
        if i >= t.n or t.words[i].quoted:
            return None
        number, end = read_number(t.words, i)
        if number is not None and t.w[i] in ("one",) and end < t.n and t.w[end] not in UNIT_WORDS:
            number = None                           # "one button": not a number
        node = None
        if number is None:
            node, end = self.gui_value_span(t, i)
            if node is None:
                return None
        unit, k = self.gui_unit(t, end)
        end += k
        if unit == "%" and t.at(end, "of the window", "of the screen", "of the width", "of the height"):
            end += t.at(end, "of the window", "of the screen", "of the width", "of the height")
        if node is not None:
            if unit == "%":
                return ("node", node), PERCENT, end - i
            factor = REAL_UNITS.get(unit) or (size_of if unit == "em" else None)
            if factor:
                node = ("maths", "mul", node, ("decimal", float(factor)))
            return ("node", node), PX, end - i
        if unit == "%":
            if isinstance(number, float) and number != int(number):
                return round(number * 10), PER_MILLE, end - i
            return int(number), PERCENT, end - i
        factor = REAL_UNITS.get(unit) or (size_of if unit == "em" else 1)
        return int(round(number * factor)), PX, end - i

    def gui_unit(self, t, j):
        word = t.w[j]
        if t.at(j, "per cent"):
            return "%", 2
        if word not in UNIT_WORDS or t.words[j].quoted:
            return None, 0
        if word == "in":                             # "2 in" is inches only where a size or place ends
            after = t.w[j + 1]
            if not (after is None or t.comma(j) or after in ("by", "x", "wide", "tall", "high", "across", "down",
                                                             "from", "and", "apart", "thick", "square")
                    or read_number(t.words, j + 1)[0] is not None):
                return None, 0
        return UNIT_WORDS[word], 1

    def gui_value_span(self, t, i):
        """A value like 'the mouse x' or 'the x of start plus 10' at t[i] -> (value tree, where it ends)."""
        stops = {"by", "and", "at", "wide", "tall", "high", "across", "down", "with", "in", "on", "to", "px",
                 "pixels", "%", "percent", "then", "apart", "thick", "from", "or", "x", "×"}
        if t.w[i] in stops | {"a", "an", "the"} and not (t.w[i] == "the" and t.w[i + 1] not in stops):
            return None, i
        for n in range(min(7, t.n - i), 0, -1):
            span = t.words[i:i + n]
            said = [w.word for w in span]
            if any(s in stops and not (s in ("x", "y") and k and said[k - 1] in ("mouse", "pointer", "the"))
                   for k, s in enumerate(said) if k):
                continue
            if any(t.comma(i + k) for k in range(n - 1)):
                continue
            try:
                node = read_value(span, self.kind_of)
            except Problem:
                continue
            if node[0] == "text":
                continue
            return node, i + n
        return None, i

    def gui_point(self, t, i):
        """'20 40', '20, 40', 'x 20 y 40', '20 px across and 40 down', 'the mouse x and the mouse y'
        -> ((x value, x unit), (y value, y unit), words used), else None."""
        j = i
        if t.at(j, "x"):
            j += 1 + t.at(j + 1, "=", "of", "is")
            first = self.gui_amount(t, j)
            if not first:
                return None
            j += first[2]
            j += t.at(j, "and", ",")
            if not t.at(j, "y"):
                return None
            j += 1 + t.at(j + 1, "=", "of", "is")
            second = self.gui_amount(t, j)
            if not second:
                return None
            return (first[0], first[1]), (second[0], second[1]), j + second[2] - i
        first = self.gui_amount(t, j)
        if not first:
            return None
        j += first[2]
        j += t.at(j, "across", "right", "along", "in", "over", "from the left", "to the right")
        if t.w[j] in ("and", ",", "by") and t.w[j] != "by":
            j += 1
        elif t.w[j] == "by":
            return None                              # a size, not a place
        second = self.gui_amount(t, j)
        if not second:
            return None
        j += second[2]
        j += t.at(j, "down", "up", "from the top")
        return (first[0], first[1]), (second[0], second[1]), j - i

    # ---------------- reading what something is like ----------------

    def gui_colour(self, t, i):
        """A colour at t[i] -> (0xRRGGBB, words used), else None."""
        if i >= t.n or t.words[i].quoted:
            return None
        found = colour_at(t.w[:t.n] + [None, None, None, None], i)
        if not found or found[0][:5] != "38;2;":
            return None
        return colour_value(found[0]), found[1]

    def gui_mods(self, t, i, end, ctx, free_from=None):
        """Read what the words from i to end say about an element -> list of effects.
        ctx: {"kind": element kind, "drawing": True for canvas shapes, "creating": True when making one}.
        Raises Problem naming the words it couldn't read."""
        effects, unknown = [], []
        while i < end:
            found = self.gui_mod_at(t, i, end, ctx)
            if found:
                n, more = found
                effects += more
                i += n
                continue
            if t.w[i] in FILLER or t.w[i] in (",", "-") or (t.w[i] or "") == "":
                i += 1
                continue
            unknown.append(t.words[i].raw)
            i += 1
        if unknown:
            raise Problem(f"I don't understand '{' '.join(unknown).strip(',')}' here.")
        return effects

    def gui_mod_at(self, t, i, end, ctx, stop_only=False):
        for reader in (self.read_in_window, self.read_name, self.read_text, self.read_items, self.read_relative, self.read_from_edge,
                       self.read_place, self.read_size, self.read_depth, self.read_tab, self.read_parent,
                       self.read_gradient, self.read_texture, self.read_transparency, self.read_corners,
                       self.read_border, self.read_shadow, self.read_text_style, self.read_hover,
                       self.read_colour_forms, self.read_bare_colour, self.read_resizable, self.read_flags,
                       self.read_thickness):
            if stop_only and reader in (self.read_bare_colour,):
                continue
            found = reader(t, i, end, ctx)
            if found and found[0] > 0 and i + found[0] <= end:
                return found
        return None

    def gui_free(self, t, i, end, ctx, items=False):
        """Free words (a name, a label) from i: up to a comma, the next thing it says, or 'and' + an action
        -> (text, words used). Quoted words are kept exactly."""
        if i < end and t.words[i].quoted:
            return t.words[i].text, 1
        j = i
        while j < end:
            if j > i and self.gui_mod_at(t, j, end, ctx, stop_only=True):
                break
            if j > i and t.w[j] in ("and", "then") and (self.starts_action(t.words, j + 1) or
                                                       self.gui_mod_at(t, j + 1, end, ctx, stop_only=True)):
                break
            j += 1
            if t.comma(j - 1, colon=False):
                break
        return t.raw(i, j), j - i

    def read_in_window(self, t, i, end, ctx):
        """'in the window': where everything goes anyway."""
        n = t.at(i, "in the window", "on the window", "to the window", "into the window", "inside the window",
                 "in my window", "on my window", "in this window", "onto the window")
        if n and not t.at(i + n, "'s"):
            return n, []
        return None

    def read_name(self, t, i, end, ctx):
        n = t.at(i, "called", "named", "labelled", "labeled", "titled", "with the name", "by the name of",
                 "with the title", "known as", "name", "title", "with the label", "with label", "labelled as")
        if not n or not ctx.get("creating") or (t.w[i] in ("name", "title") and ctx.get("stop")):
            return None
        if t.w[i] in ("name", "title") and i + 1 < end and t.w[i + 1] in ("of", "to"):
            return None
        text, m = self.gui_free(t, i + n, end, ctx)
        if not text:
            raise Problem("Called what? Like: make a button called start")
        label = t.w[i] in ("labelled", "labeled", "titled", "title", "with the title", "with the label")
        return n + m, [("name", text)] + ([("text", text)] if label else [])

    def read_text(self, t, i, end, ctx):
        n = t.at(i, "that says", "which says", "saying", "says", "that reads", "which reads", "reading",
                 "with the text", "with text", "with the words", "with the caption", "captioned", "showing",
                 "that shows", "which shows", "that said", "with the message", "with words", "with writing",
                 "that has the text", "text", "with the label text")
        if not n or ctx.get("drawing"):
            return None
        if t.w[i] == "text" and (not ctx.get("creating") or self.read_text_style(t, i, end, ctx)
                                 or t.w[i + 1] in ("in", "colour", "color", "size", "of")):
            return None
        text, m = self.gui_free(t, i + n, end, ctx)
        if not text:
            return None
        return n + m, [("text", text)]

    def read_items(self, t, i, end, ctx):
        if ctx.get("kind") not in (MENU, TABS):
            return None
        n = t.at(i, "with", "containing", "holding", "listing", "offering", "having", "with the items",
                 "with items", "with the options", "with options", "with the choices", "with choices",
                 "with the tabs", "with tabs", "with the pages", "with pages", "items", "options", "choices",
                 "pages", "with the entries", "with entries", "for")
        if not n:
            return None
        j = i + n
        if j < end and t.words[j].quoted is False and t.w[j] in ("a", "an") and self.gui_mod_at(t, j, end, ctx):
            return None                                 # "with a red border": not items
        if self.gui_mod_at(t, j, end, ctx, stop_only=True):
            return None
        k = j
        while k < end and not (k > j and t.w[k] in ("at", "in", "on", "below", "under", "above", "beside")
                               and self.gui_mod_at(t, k, end, ctx, stop_only=True)):
            k += 1
        items = [" ".join(w.text if w.quoted else w.raw for w in item).strip(" ,.:")
                 for item in list_items(t.words[j:k])]
        items = [item for item in items if item]
        if not items:
            return None
        return k - i, [("items", items)]

    def gui_spot_effects(self, vertical, horizontal):
        out = []
        if vertical is None and horizontal is not None:
            vertical = "middle"
        if horizontal is None and vertical is not None:
            horizontal = "centre"
        top = MARGIN + (36 if any(e.kind == MENU for e in self.gui_order) else 0)    # under the menu bar
        out += {"top": [("set", Y, top), ("set", YU, PX)], "bottom": [("set", Y, MARGIN), ("set", YU, FAR)],
                "middle": [("set", Y, 0), ("set", YU, CENTRED)]}[vertical]
        out += {"left": [("set", X, MARGIN), ("set", XU, PX)], "right": [("set", X, MARGIN), ("set", XU, FAR)],
                "centre": [("set", X, 0), ("set", XU, CENTRED)]}[horizontal]
        return out

    def read_place(self, t, i, end, ctx):
        """'at 20 20', 'at 10% 20%', 'at x 20 y 40', 'to 100 100', 'in the middle', 'at the top right',
        'on the left', 'centred', '20 across and 40 down'."""
        if t.at(i, "centred", "centered", "in the centre", "in the center", "in the middle", "at the centre",
                "at the center", "at the middle", "in the very middle", "right in the middle"):
            n = t.at(i, "centred", "centered", "in the centre", "in the center", "in the middle", "at the centre",
                     "at the center", "at the middle", "in the very middle", "right in the middle")
            n += t.at(i + n, "of the window", "of the screen")
            return n, self.gui_spot_effects("middle", "centre")
        lead = t.at(i, "at", "to", "@", "at position", "at the position", "at point", "positioned at", "located at",
                    "placed at", "at the point", "at coordinates", "at the coordinates", "at co-ordinates", "position")
        if lead or t.w[i] == "x":
            found = self.gui_point(t, i + lead)
            if found:
                (xv, xu), (yv, yu), n = found
                return lead + n, [("set", X, xv), ("set", XU, xu), ("set", Y, yv), ("set", YU, yu)]
        if t.w[i] in ("at", "on", "in", "to", "near", "along", "across"):
            for n in range(min(9, end - i), 1, -1):
                spot = self.gui_spot(t.words[i:i + n])
                if spot:
                    return n, self.gui_spot_effects(*spot)
        if lead and t.w[i] == "at" and t.at(i + 1, "x", "y"):
            axis = t.w[i + 1]
            found = self.gui_amount(t, i + 2 + t.at(i + 2, "="))
            if found:
                v, u, n = found
                return 2 + t.at(i + 2, "=") + n, [("set", X if axis == "x" else Y, v), ("set", XU if axis == "x" else YU, u)]
        if t.w[i] in ("x", "y", "left", "top") and (t.at(i + 1, "=", "of", "at", "is") or
                                                    self.gui_amount(t, i + 1)) and t.w[i + 1] != "of" or \
                t.at(i, "x position", "y position"):
            axis = "x" if t.w[i] in ("x", "left") else "y"
            k = 1 + t.at(i + 1, "position") + t.at(i + 1 + t.at(i + 1, "position"), "=", "of", "at", "is")
            found = self.gui_amount(t, i + k)
            if found:
                v, u, n = found
                return k + n, [("set", X if axis == "x" else Y, v), ("set", XU if axis == "x" else YU, u)]
        found = self.gui_amount(t, i)
        if found and t.at(i + found[2], "across", "along", "in from the left", "from the left"):
            n = found[2] + t.at(i + found[2], "across", "along", "in from the left", "from the left")
            out = [("set", X, found[0]), ("set", XU, found[1])]
            j = i + n + t.at(i + n, "and", ",")
            second = self.gui_amount(t, j)
            if second and t.at(j + second[2], "down", "from the top"):
                k = second[2] + t.at(j + second[2], "down", "from the top")
                return j + k - i, out + [("set", Y, second[0]), ("set", YU, second[1])]
            return n, out
        if found and t.at(i + found[2], "down", "from the top", "in from the top"):
            n = found[2] + t.at(i + found[2], "down", "from the top", "in from the top")
            return n, [("set", Y, found[0]), ("set", YU, found[1])]
        return None

    def gui_spot(self, words):
        from window import read_spot
        said = [w.word for w in words]
        if not said or said[-1] in ("of", "the", "and") or any(w.quoted for w in words):
            return None
        if any(s not in ("in", "at", "on", "to", "the", "of", "side", "edge", "corner", "screen", "window", "top",
                         "bottom", "left", "right", "middle", "centre", "center", "upper", "lower", "very", "far",
                         "near", "and", "part", "half", "hand") for s in said):
            return None
        spot = read_spot([Word(w.raw.rstrip(",")) for w in words])
        if not spot or any(isinstance(s, tuple) for s in spot):
            return None
        vertical, horizontal = spot
        return vertical, horizontal

    def read_from_edge(self, t, i, end, ctx):
        """'20 from the right', '20 px from the bottom edge', '10% from the left'."""
        found = self.gui_amount(t, i)
        if not found:
            return None
        v, u, n = found
        j = i + n + t.at(i + n, "in", "away")
        k = t.at(j, "from the right", "from the left", "from the top", "from the bottom", "from right", "from left",
                 "from top", "from bottom", "off the right", "off the left", "off the top", "off the bottom")
        if not k:
            return None
        side = t.w[j + k - 1]
        k += t.at(j + k, "edge", "side", "of the window", "edge of the window", "side of the window")
        axis_x = side in ("right", "left")
        far = side in ("right", "bottom")
        unit = FAR if far and u == PX else u
        if far and u in (PERCENT, PER_MILLE):            # 10% from the right: at 90% minus its own width
            raise Problem("Say how far from the right in px, like: 20 from the right")
        return j + k - i, [("set", X if axis_x else Y, v), ("set", XU if axis_x else YU, unit)]

    def read_relative(self, t, i, end, ctx):
        """'to the right of board', 'below start', 'next to board', '10 px below start', 'with a gap of 10'."""
        gap = None
        j = i
        found = self.gui_amount(t, j)
        if found and found[1] == PX and isinstance(found[0], int):
            gap, j = found[0], j + found[2]
        n = t.at(j, "with a gap of", "with a space of", "gap of", "a gap of", "with a gap", "gap", "spaced by",
                 "with spacing", "spacing")
        if n and gap is None:
            found = self.gui_amount(t, j + n)
            if found and isinstance(found[0], int):
                return n + found[2], [("set", GAP, found[0])]
        if gap is not None and t.at(j, "apart"):
            return j + 1 - i, [("set", GAP, gap)]
        where = [("to the right of", "right"), ("on the right of", "right"), ("right of", "right"),
                 ("next to", "right"), ("beside", "right"), ("alongside", "right"), ("after", "right"),
                 ("to the left of", "left"), ("on the left of", "left"), ("left of", "left"), ("before", "left"),
                 ("below", "below"), ("under", "below"), ("underneath", "below"), ("beneath", "below"),
                 ("just below", "below"), ("just under", "below"), ("above", "above"), ("over", "above"),
                 ("just above", "above"), ("on top of", "above"), ("right below", "below"), ("right above", "above"),
                 ("to the right of the", "right")]
        for phrase, how in sorted(where, key=lambda p: -len(p[0].split())):
            k = t.at(j, phrase)
            if not k:
                continue
            other = self.gui_element_at(t, j + k)
            if not other or other[0].kind == WINDOW:
                return None
            elem, m = other
            out = [("other", elem)]
            if how in ("right", "left"):
                out += [("set", XU, AFTER if how == "right" else BEFORE), ("set", X, 0), ("set", YU, ALIGNED),
                        ("set", Y, 0)]
            else:
                out += [("set", YU, AFTER if how == "below" else BEFORE), ("set", Y, 0), ("set", XU, ALIGNED),
                        ("set", X, 0)]
            out.append(("set", GAP, MARGIN if gap is None else gap))
            return j + k + m - i, out
        return None

    def read_size(self, t, i, end, ctx):
        """'800 by 600', '800x600', '800 wide and 600 tall', 'width 800 height 600', 'half the window wide',
        'as wide as board', '2 cm wide', 'size 200 by 50', 'the same size as board', 'full width'."""
        j = i + t.at(i, "size", "sized", "of size", "a size of", "with a size of", "measuring", "dimensions")
        found = self.gui_amount(t, j)
        if found:
            w, wu, n = found
            k = j + n
            if t.w[k] in ("by", "x", "×", "*") or t.at(k, "wide by"):
                k += 1 + t.at(k, "wide by")
                second = self.gui_amount(t, k)
                if second:
                    h, hu, m = second
                    m += t.at(k + m, "tall", "high", "in size", "big", "large", "px")
                    return k + m - i, [("set", W, w), ("set", WU, wu), ("set", H, h), ("set", HU, hu)]
            side = t.at(k, "wide", "in width", "across", "width", "long") and "w" or \
                t.at(k, "tall", "high", "in height", "height", "deep") and "h"
            if side and not (t.w[k] == "across" and (t.at(k + 1, "and") or self.gui_amount(t, k + 1))):
                m = t.at(k, "wide", "in width", "across", "width", "long", "tall", "high", "in height", "height", "deep")
                return k + m - i, [("set", W if side == "w" else H, w), ("set", WU if side == "w" else HU, wu)]
            if j > i and t.w[i] in ("size", "sized") and not ctx.get("text_size_ok", True):
                return None
        n = t.at(i, "width", "the width", "a width of", "width of", "height", "the height", "a height of",
                 "height of", "w", "h")
        if n:
            side = "w" if t.w[i + n - 1 if t.w[i + n - 1] != "of" else i + n - 2] in ("width", "w") else "h"
            k = i + n + t.at(i + n, "=", "is", "of")
            found = self.gui_amount(t, k)
            if found:
                v, u, m = found
                return k + m - i, [("set", W if side == "w" else H, v), ("set", WU if side == "w" else HU, u)]
        fractions = {"half": 50, "a half": 50, "a third": 33, "third": 33, "a quarter": 25, "quarter": 25,
                     "two thirds": 67, "three quarters": 75, "all": 100, "the whole": 100}
        for phrase, pct in fractions.items():
            k = t.at(i, phrase)
            if not k:
                continue
            k += t.at(i + k, "of", "as")
            m = t.at(i + k, "the window", "the screen", "the window's", "the parent")
            side = t.at(i + k + m, "wide", "width", "tall", "high", "height", "as wide", "as tall") if m else 0
            if m and side:
                which = t.w[i + k + m + side - 1]
                w_side = which in ("wide", "width")
                return k + m + side - i + i, [("set", W if w_side else H, pct), ("set", WU if w_side else HU, PERCENT)]
            if t.at(i + k, "wide", "as wide as the window", "the width", "the width of the window"):
                m = t.at(i + k, "wide", "as wide as the window", "the width", "the width of the window")
                return k + m, [("set", W, pct), ("set", WU, PERCENT)]
            if t.at(i + k, "tall", "high", "as tall as the window", "the height", "the height of the window"):
                m = t.at(i + k, "tall", "high", "as tall as the window", "the height", "the height of the window")
                return k + m, [("set", H, pct), ("set", HU, PERCENT)]
        n = t.at(i, "as wide as", "as tall as", "as high as", "the same width as", "the same height as",
                 "the same size as", "as big as", "the same as", "same size as", "same width as", "same height as")
        if n:
            other = self.gui_element_at(t, i + n)
            if other and other[0].kind != WINDOW:
                elem, m = other
                said = " ".join(t.w[i:i + n])
                out = [("other", elem)]
                if "wide" in said or "width" in said or "size" in said or "big" in said or said == "the same as":
                    out += [("set", W, 0), ("set", WU, ALIGNED)]
                if "tall" in said or "high" in said or "height" in said or "size" in said or "big" in said \
                        or said == "the same as":
                    out += [("set", H, 0), ("set", HU, ALIGNED)]
                return n + m, out
            if other:
                said = " ".join(t.w[i:i + n])
                out = []
                if "wide" in said or "width" in said or "size" in said or "big" in said:
                    out += [("set", W, 100), ("set", WU, PERCENT)]
                if "tall" in said or "high" in said or "height" in said or "size" in said or "big" in said:
                    out += [("set", H, 100), ("set", HU, PERCENT)]
                return n + other[1], out
        n = t.at(i, "full width", "the full width", "full height", "the full height", "full size", "the full size",
                 "fill the window", "filling the window", "full window", "the whole window", "as big as the window")
        if n:
            said = " ".join(t.w[i:i + n])
            out = []
            if "height" not in said:
                out += [("set", X, 0), ("set", XU, PX), ("set", W, 0), ("set", WU, FAR)]
            if "width" not in said:
                out += [("set", Y, 0), ("set", YU, PX), ("set", H, 0), ("set", HU, FAR)]
            return n, out
        scales = {"bigger": 125, "larger": 125, "smaller": 80, "tinier": 80, "wider": 125, "narrower": 80,
                  "taller": 125, "shorter": 80, "twice as big": 200, "twice as wide": 200, "twice as tall": 200,
                  "half as big": 50, "half the size": 50, "double the size": 200, "much bigger": 150,
                  "much smaller": 60, "a bit bigger": 110, "a bit smaller": 90, "a little bigger": 110,
                  "a little smaller": 90}
        n = max((t.at(i, phrase) for phrase in scales), default=0)
        if n and not ctx.get("creating"):
            said = " ".join(t.w[i:i + n])
            pct = scales[said]
            fields = [W] if "wide" in said or said in ("wider", "narrower") else \
                [H] if "tall" in said or said in ("taller", "shorter") else [W, H]
            return n, [("scale", f, pct) for f in fields]
        n = t.at(i, "fit the text", "fitting the text", "fit its text", "as wide as its text", "fit to the text",
                 "fitted to the text", "fit")
        if n:
            return n, [("set", W, 0), ("set", WU, FIT)]
        return None

    def read_depth(self, t, i, end, ctx):
        n = t.at(i, "at depth", "depth", "on layer", "layer", "at layer", "with depth", "with a depth of",
                 "at a depth of", "z", "z index", "z-index")
        if n:
            found = self.gui_amount(t, i + n + t.at(i + n, "=", "of"))
            if found:
                return n + t.at(i + n, "=", "of") + found[2], [("set", DEPTH, found[0])]
        for phrase, how in (("in front of", "front_of"), ("on top of", "front_of"), ("behind", "behind"),
                            ("in back of", "behind"), ("at the back of", "behind")):
            k = t.at(i, phrase)
            if k:
                other = self.gui_element_at(t, i + k)
                if other and other[0].kind != WINDOW:
                    return k + other[1], [("call", "bw_in_front_of" if how == "front_of" else "bw_behind", other[0])]
        k = t.at(i, "in front", "at the front", "to the front", "on top", "in front of everything", "frontmost",
                 "in front of everything else", "on top of everything", "to the top", "forward to the front")
        if k:
            return k, [("call", "bw_front", None)]
        k = t.at(i, "at the back", "to the back", "behind everything", "in the back", "behind everything else",
                 "backmost", "underneath everything", "to the bottom of the pile", "at the very back")
        if k:
            return k, [("call", "bw_back", None)]
        return None

    def read_tab(self, t, i, end, ctx):
        """'on the Home tab', 'in the Settings page', 'on tab Home', 'on the Home tab of pages'."""
        k = t.at(i, "on the", "in the", "on", "in", "into the", "onto the", "to the", "inside the")
        if not k:
            return None
        j = i + k
        if t.at(j, "tab", "page"):
            m = 1
            for n in range(1, 4):
                item = self.gui_item(t.words[j + m:j + m + n])
                if item and item[0].kind == TABS:
                    return k + m + n, [("tab", item[0], item[1])]
            return None
        for n in range(1, 4):
            if t.w[j + n] in ("tab", "page"):
                menu = None
                extra = 1
                if t.at(j + n + 1, "of", "in"):
                    other = self.gui_element_at(t, j + n + 2, (TABS,))
                    if other:
                        menu, extra = other[0], 2 + other[1]
                item = self.gui_item(t.words[j:j + n], menu)
                if item and item[0].kind == TABS:
                    return k + n + extra, [("tab", item[0], item[1])]
        return None

    def read_parent(self, t, i, end, ctx):
        k = t.at(i, "inside", "in", "within", "into", "on")
        if not k or ctx.get("drawing"):
            return None
        other = self.gui_element_at(t, i + k + t.at(i + k, "the"))
        if other and other[0].kind in (BOX, CANVAS, TABS, BUTTON, GLVIEW) and other[0] is not ctx.get("elem"):
            return k + t.at(i + k, "the") + other[1], [("parent", other[0])]
        return None

    def read_gradient(self, t, i, end, ctx):
        j = i + t.at(i, "with", "and", "having")
        j += t.at(j, "a", "an", "some")
        kind = 2
        start = j
        if t.at(j, "radial", "circular", "round", "glowing", "spot"):
            kind, j = 3, j + 1
        j += t.at(j, "linear", "smooth", "soft", "colour", "color", "nice")
        before = self.gui_colour(t, j)                   # "a red to orange gradient"
        if before and t.at(j + before[1], "to", "and", "into"):
            k = j + before[1] + 1
            after = self.gui_colour(t, k)
            if after and t.at(k + after[1], "gradient", "fade", "gradient fill"):
                m = k + after[1] + t.at(k + after[1], "gradient", "fade", "gradient fill")
                d, dn = self.gui_direction(t, m)
                return m + dn - i, self.gradient(kind, before[0], after[0], d)
        g = t.at(j, "gradient", "fade", "gradient fill", "gradient background", "blend")
        if g:
            j += g
            j += t.at(j, "going", "that goes", "running", "that runs", "fading")
            j += t.at(j, "from")
        elif t.at(start, "fading from", "that fades from", "which fades from", "going from", "fading", "fades from",
                  "blending from", "shading from", "graded from", "a fade from"):
            j = start + t.at(start, "fading from", "that fades from", "which fades from", "going from", "fading",
                             "fades from", "blending from", "shading from", "graded from", "a fade from")
        else:
            return None
        first = self.gui_colour(t, j)
        if not first:
            raise Problem("A gradient from which colour to which? Like: with a gradient from black to navy going down")
        j += first[1]
        j += t.at(j, "in the middle", "at the centre", "in the centre", "at the middle", "at the top", "at the left",
                  "on the left", "on top", "in the center", "at the center")
        if not t.at(j, "to", "into", "and", "through"):
            raise Problem("A gradient from which colour to which? Like: with a gradient from black to navy going down")
        j += 1
        second = self.gui_colour(t, j)
        if not second:
            raise Problem("A gradient from which colour to which? Like: with a gradient from black to navy going down")
        j += second[1]
        j += t.at(j, "at the edges", "at the edge", "on the outside", "at the outside", "round the edge",
                  "around the edge", "at the bottom", "at the right", "on the right", "on the bottom")
        d, dn = self.gui_direction(t, j)
        return j + dn - i, self.gradient(kind, first[0], second[0], d)

    def gui_direction(self, t, j):
        k = t.at(j, "going", "that goes", "running", "fading", ",")
        for phrase, d in DIRECTIONS:
            n = t.at(j + k, phrase)
            if n:
                return d, k + n
        return 0, 0

    def gradient(self, kind, first, second, direction):
        return [("set", S + FILL, kind), ("set", S + COLOUR, first), ("set", S + COLOUR2, second),
                ("set", S + DIRECTION, direction)]

    def read_texture(self, t, i, end, ctx):
        j = i + t.at(i, "with", "and", "having", "covered in", "covered with", "in a", "in")
        j += t.at(j, "a", "an", "some")
        size = None
        if t.w[j] in TEXTURE_SIZES and (self.gui_texture_word(t, j + 1) or
                                        (self.gui_colour(t, j + 1) and
                                         self.gui_texture_word(t, j + 1 + self.gui_colour(t, j + 1)[1]))):
            size, j = TEXTURE_SIZES[t.w[j]], j + 1
        colour = None
        found = self.gui_colour(t, j)
        if found and self.gui_texture_word(t, j + found[1]):
            colour, j = found[0], j + found[1]
            if t.at(j, "and") and self.gui_colour(t, j + 1):   # "red and white stripes": the fill and the marks
                pass
        tex = self.gui_texture_word(t, j)
        if not tex and t.at(j, "texture of", "pattern of", "texture", "pattern"):
            k = t.at(j, "texture of", "pattern of", "texture", "pattern")
            k += t.at(j + k, ":", "with")
            tex = self.gui_texture_word(t, j + k)
            if tex:
                j += k
        if not tex:
            return None
        number, n = tex
        if j == i and t.w[i] in ("lines", "check", "brick", "grid", "noise", "spots", "dots", "point") and \
                not ctx.get("creating") and ctx.get("drawing"):
            return None
        j += n
        j += t.at(j, "texture", "pattern", "effect", "finish", "look")
        found = self.gui_colour(t, j + t.at(j, "in", "of", "coloured", "colored", "in the colour"))
        if colour is None and found and t.at(j, "in", "of", "coloured", "colored", "in the colour"):
            colour, j = found[0], j + t.at(j, "in", "of", "coloured", "colored", "in the colour") + found[1]
        amount = self.gui_amount(t, j + t.at(j, "of", "size", "sized", "every"))
        if amount and isinstance(amount[0], int) and t.at(j + t.at(j, "of", "size", "sized", "every") + amount[2],
                                                           "apart", "wide", "big", "across", "in size", "size"):
            k = t.at(j, "of", "size", "sized", "every") + amount[2]
            k += t.at(j + k, "apart", "wide", "big", "across", "in size", "size")
            size, j = amount[0], j + k
        elif t.at(j, "size", "texture size", "with size") and self.gui_amount(t, j + t.at(j, "size", "texture size", "with size")):
            k = t.at(j, "size", "texture size", "with size")
            amount = self.gui_amount(t, j + k)
            size, j = amount[0], j + k + amount[2]
        out = [("set", S + TEXTURE, number), ("set", S + TEXTURE_COLOUR, 0x808080 if colour is None else colour)]
        if size is not None:
            out.append(("set", S + TEXTURE_SIZE, size))
        return j - i, out

    def gui_texture_word(self, t, j):
        for n in (2, 1):
            said = " ".join(w for w in t.w[j:j + n] if w)
            if len(t.w[j:j + n]) == n and None not in t.w[j:j + n] and said in TEXTURES:
                return TEXTURES[said], n
        return None

    def read_transparency(self, t, i, end, ctx):
        see = ("transparent", "see-through", "see through", "seethrough", "translucent", "clear", "transparency",
               "faded", "faint", "ghostly", "see thru", "see-thru")
        found = self.gui_amount(t, i)
        if found and found[1] in (PERCENT, PER_MILLE, PX) and isinstance(found[0], int):
            pct = found[0] / (10 if found[1] == PER_MILLE else 1)
            k = found[2]
            if t.at(i + k, *see):
                return k + t.at(i + k, *see), [("set", S + TRANSPARENCY, round(255 * pct / 100))]
            if t.at(i + k, "opaque", "opacity", "visible", "solid"):
                return k + t.at(i + k, "opaque", "opacity", "visible", "solid"), \
                    [("set", S + TRANSPARENCY, round(255 * (100 - pct) / 100))]
        n = t.at(i, "transparency", "with transparency", "opacity", "with opacity", "with an opacity of",
                 "with a transparency of", "transparency of", "opacity of", "alpha")
        if n:
            found = self.gui_amount(t, i + n + t.at(i + n, "="))
            if found and isinstance(found[0], int):
                pct = found[0] / (10 if found[1] == PER_MILLE else 1)
                k = n + t.at(i + n, "=") + found[2]
                if "opacity" in t.w[i:i + n] or t.w[i] == "alpha":
                    pct = 100 - pct
                return k, [("set", S + TRANSPARENCY, round(255 * pct / 100))]
        amounts = {"half": 50, "partly": 40, "semi": 50, "a bit": 25, "a little": 25, "slightly": 25, "mostly": 75,
                   "very": 75, "fully": 100, "completely": 100, "totally": 100, "somewhat": 40, "a little bit": 20,
                   "quite": 60, "almost": 85, "nearly": 85, "barely": 10}
        k = t.at(i, *amounts)
        j = i + k
        word = t.w[j] or ""
        if word.startswith("semi-") and word[5:] in ("transparent", "see-through", "translucent", "opaque"):
            return k + 1, [("set", S + TRANSPARENCY, 128)]
        m = t.at(j, *see)
        if m and not (t.w[j] == "clear" and not k):
            pct = amounts[" ".join(t.w[i:j])] if k else (60 if t.w[j] in ("faded", "faint", "ghostly") else 50)
            return k + m, [("set", S + TRANSPARENCY, round(255 * pct / 100))]
        if t.at(i, "opaque", "solid", "not transparent", "fully opaque"):
            return t.at(i, "opaque", "solid", "not transparent", "fully opaque"), [("set", S + TRANSPARENCY, 0)]
        return None

    def read_corners(self, t, i, end, ctx):
        j = i + t.at(i, "with", "and", "having")
        size = None
        found = self.gui_amount(t, j + t.at(j, "a", "an"))
        if found and isinstance(found[0], int) and found[1] == PX:
            j2 = j + t.at(j, "a", "an") + found[2]
            if t.at(j2, "round corners", "rounded corners", "corners", "corner radius", "curved corners", "radius"):
                size, j = found[0], j2
        n = t.at(j, "round corners", "rounded corners", "curved corners", "soft corners", "smooth corners",
                 "rounded edges", "round edges", "curved edges", "corners", "corner radius", "a corner radius",
                 "radius", "rounded", "round", "curved", "a corner radius of", "corners rounded", "rounding")
        if not n or (t.w[j] == "round" and t.w[j + 1] in ("the",)):
            return None
        if t.w[j] in ("corners", "radius", "corner") and size is None and not self.gui_amount(t, j + n + t.at(j + n, "of", "by", "=")):
            return None
        j += n
        k = t.at(j, "of", "by", "=", "with a radius of", "radius")
        found = self.gui_amount(t, j + k)
        if size is None and found and isinstance(found[0], int) and found[1] == PX:
            size, j = found[0], j + k + found[2]
        if size is None:
            size = 12
        return j - i, [("set", S + RADIUS, size)]

    def read_border(self, t, i, end, ctx):
        j = i + t.at(i, "with", "and", "having")
        j += t.at(j, "a", "an")
        width = colour = None
        if t.at(i, "no border", "without a border", "no outline", "without border", "borderless"):
            n = t.at(i, "no border", "without a border", "no outline", "without border", "borderless")
            return n, [("set", S + BORDER, 0)]
        for _ in range(3):
            found = self.gui_amount(t, j)
            if width is None and found and isinstance(found[0], int) and found[1] == PX:
                width, j = found[0], j + found[2]
                continue
            if width is None and t.w[j] in ("thin", "thick", "fat", "heavy", "fine", "bold", "light"):
                width, j = {"thin": 1, "fine": 1, "light": 1}.get(t.w[j], 3), j + 1
                continue
            found = self.gui_colour(t, j)
            if colour is None and found:
                colour, j = found[0], j + found[1]
                continue
            break
        n = t.at(j, "border", "outline", "edge", "frame", "rim", "edging", "line around it", "line round it")
        if not n:
            k = t.at(i, "bordered", "outlined", "framed", "edged")
            if not k:
                return None
            j, n = i + k, 0
        j += n
        for _ in range(3):
            k = t.at(j, "in", "of", "coloured", "colored", "that is", "which is")
            found = self.gui_colour(t, j + k)
            if colour is None and found:
                colour, j = found[0], j + k + found[1]
                continue
            found = self.gui_amount(t, j + t.at(j, "of", "that is", "which is"))
            if width is None and found and isinstance(found[0], int):
                k = t.at(j, "of", "that is", "which is") + found[2]
                k += t.at(j + k, "thick", "wide")
                width, j = found[0], j + k
                continue
            break
        return j - i, [("set", S + BORDER, 1 if width is None else width),
                       ("set", S + BORDER_COLOUR, 0 if colour is None else colour)]

    def read_shadow(self, t, i, end, ctx):
        if t.at(i, "no shadow", "without a shadow", "without shadow"):
            return t.at(i, "no shadow", "without a shadow", "without shadow"), [("set", S + SHADOW, 0)]
        j = i + t.at(i, "with", "and", "having", "casting")
        j += t.at(j, "a", "an")
        size = 4
        found = self.gui_amount(t, j)
        if found and isinstance(found[0], int) and t.at(j + found[2], "shadow", "drop shadow"):
            size, j = found[0], j + found[2]
        if t.w[j] in ("soft", "drop", "big", "small", "deep", "dark", "subtle", "large", "long", "short", "little"):
            size = {"big": 8, "large": 8, "deep": 8, "long": 10, "small": 2, "subtle": 2, "short": 2,
                    "little": 2}.get(t.w[j], size)
            j += 1
        n = t.at(j, "shadow", "drop shadow", "shadowed", "shadows")
        if not n:
            return None
        j += n
        found = self.gui_amount(t, j + t.at(j, "of", "=", "size"))
        if found and isinstance(found[0], int):
            size, j = found[0], j + t.at(j, "of", "=", "size") + found[2]
        return j - i, [("set", S + SHADOW, size)]

    def read_text_style(self, t, i, end, ctx):
        """'text size 24', 'size 24', 'with white text', 'text in gold', 'left aligned', 'in big letters'."""
        n = t.at(i, "text size", "font size", "size", "letter size", "with text size", "in size", "with a text size of",
                 "with a font size of", "a font size of", "a text size of", "font", "the size of the text",
                 "letters", "text height")
        if n:
            k = n + t.at(i + n, "=", "of", "to")
            found = self.gui_amount(t, i + k)
            if found and t.w[i + k + found[2]] not in ("by", "x", "×", "wide", "tall", "high"):
                return k + found[2], [("set", S + TEXT_SIZE, found[0])]
        found = self.gui_amount(t, i)
        if found and t.at(i + found[2], "text", "font", "letters", "writing", "type", "high text", "high letters"):
            k = found[2] + t.at(i + found[2], "text", "font", "letters", "writing", "type", "high text", "high letters")
            return k, [("set", S + TEXT_SIZE, found[0])]
        j = i + t.at(i, "with", "and", "in", "using")
        sizes = {"big": 24, "large": 24, "huge": 40, "giant": 48, "small": 12, "tiny": 10, "little": 12,
                 "normal": 16, "medium": 18, "massive": 56}
        if t.w[j] in sizes and t.at(j + 1, "text", "letters", "writing", "font", "type"):
            return j + 2 - i, [("set", S + TEXT_SIZE, sizes[t.w[j]])]
        colour = self.gui_colour(t, j)
        if colour and t.at(j + colour[1], "text", "letters", "writing", "font", "words", "lettering"):
            k = j + colour[1] + 1
            return k - i, [("set", S + TEXT_COLOUR, colour[0])]
        n = t.at(i, "text in", "text colour", "text color", "text coloured", "text colored", "font colour",
                 "font color", "the text", "with the text in", "letters in", "writing in", "with text in",
                 "text that is", "the text colour", "the text color", "lettering in", "words in")
        if n:
            k = n + t.at(i + n, "=", "of", "to", "in", "is")
            colour = self.gui_colour(t, i + k)
            if colour:
                return k + colour[1], [("set", S + TEXT_COLOUR, colour[0])]
        aligns = [("left aligned", 1), ("left-aligned", 1), ("aligned left", 1), ("aligned to the left", 1),
                  ("align left", 1), ("right aligned", 2), ("right-aligned", 2), ("aligned right", 2),
                  ("aligned to the right", 2), ("align right", 2), ("centre aligned", 0), ("center aligned", 0),
                  ("centre-aligned", 0), ("center-aligned", 0), ("aligned centre", 0), ("aligned center", 0),
                  ("with centred text", 0), ("with centered text", 0), ("centred text", 0),
                  ("with the text on the left", 1), ("with the text on the right", 2), ("text on the left", 1),
                  ("text on the right", 2), ("text left", 1), ("text right", 2)]
        for phrase, how in sorted(aligns, key=lambda p: -len(p[0].split())):
            k = t.at(i, phrase)
            if k:
                return k, [("set", S + ALIGN, how)]
        if t.at(i, "in big letters", "in large letters", "in huge letters", "in small letters", "in tiny letters"):
            return 4 if t.w[i + 1] != "letters" else 3, [("set", S + TEXT_SIZE, sizes[t.w[i + 1]])]
        return None

    def read_hover(self, t, i, end, ctx):
        """'that turns yellow when the mouse is over it', 'turning red on hover', 'with a gold hover colour'."""
        j = i + t.at(i, "that", "which", "and", "it", "so it", "so that it")
        k = t.at(j, "turns", "turn", "turning", "goes", "go", "going", "becomes", "become", "becoming", "changes to",
                 "change to", "changing to", "lights up", "light up", "lighting up", "glows", "glow", "glowing",
                 "will turn", "should turn", "is", "gets", "shows", "flashes")
        colour = self.gui_colour(t, j + k)
        if colour:
            m = j + k + colour[1]
            m += t.at(m, "instead")
            n = t.at(m, "on hover", "when hovered", "when hovered over", "while hovered", "under the mouse",
                     "under the pointer", "under the cursor", "on mouse over", "on mouseover", "when moused over")
            if not n and t.w[m] in ("when", "while", "whenever", "if", "as", "once"):
                stop = m + 1
                while stop < end and not t.comma(stop - 1) and t.w[stop] not in ("then",):
                    stop += 1
                if set(t.w[m:stop]) & HOVER_WORDS:
                    n = stop - m
            if n and (k or t.w[i] in ("that", "which", "and") or True):
                return m + n - i, [("set", HOVER, colour[0]), ("flag", HAS_HOVER)]
        n = t.at(i, "with a hover colour of", "with a hover color of", "hover colour", "hover color", "on hover",
                 "a hover colour of", "hover")
        if n:
            colour = self.gui_colour(t, i + n + t.at(i + n, "of", "=", "to"))
            if colour:
                return n + t.at(i + n, "of", "=", "to") + colour[1], [("set", HOVER, colour[0]), ("flag", HAS_HOVER)]
        j = i + t.at(i, "with", "and")
        j += t.at(j, "a", "an")
        colour = self.gui_colour(t, j)
        if colour and t.at(j + colour[1], "hover colour", "hover color", "hover", "hover effect", "highlight"):
            m = j + colour[1] + t.at(j + colour[1], "hover colour", "hover color", "hover", "hover effect", "highlight")
            return m - i, [("set", HOVER, colour[0]), ("flag", HAS_HOVER)]
        return None

    def read_colour_forms(self, t, i, end, ctx):
        """'in orange', 'coloured red', 'with a navy background', 'background blue', 'filled with red'."""
        n = t.at(i, "in", "coloured", "colored", "colour", "color", "the colour", "the color", "in the colour",
                 "in the color", "painted", "filled with", "filled", "in colour", "in color", "colour of",
                 "color of", "with the colour", "with colour", "using", "of colour", "turned", "painted in",
                 "with a fill of", "fill", "fill colour", "fill color", "with fill", "with a colour of")
        if n:
            colour = self.gui_colour(t, i + n)
            if colour:
                k = n + colour[1] + t.at(i + n + colour[1], "colour", "color", "coloured", "colored", "paint")
                if t.at(i + k, "text", "letters", "writing") and t.w[i] == "in":
                    return k + 1, [("set", S + TEXT_COLOUR, colour[0])]
                return k, [("colour", colour[0])]
        j = i + t.at(i, "with", "on", "and", "having")
        j += t.at(j, "a", "an")
        colour = self.gui_colour(t, j)
        if colour and t.at(j + colour[1], "background", "fill", "back", "backdrop", "body"):
            return j + colour[1] + 1 - i, [("bg", colour[0])]
        n = t.at(i, "background", "background colour", "background color", "the background", "bg", "a background of",
                 "with a background of", "on a background of", "backdrop")
        if n:
            colour = self.gui_colour(t, i + n + t.at(i + n, "of", "=", "is", "in"))
            if colour:
                return n + t.at(i + n, "of", "=", "is", "in") + colour[1], [("bg", colour[0])]
        return None

    def read_bare_colour(self, t, i, end, ctx):
        colour = self.gui_colour(t, i)
        if not colour:
            return None
        n = colour[1] + t.at(i + colour[1], "colour", "color", "coloured", "colored")
        return n, [("colour", colour[0])]

    def read_resizable(self, t, i, end, ctx):
        n = t.at(i, "resizable", "resizeable", "that can be resized", "which can be resized", "you can resize",
                 "stretchy", "that resizes", "resizing", "sizable", "that you can resize", "able to resize")
        if n:
            return n, [("resizable", 1)]
        n = t.at(i, "fixed size", "fixed-size", "of fixed size", "not resizable", "that can't be resized",
                 "non-resizable", "unresizable", "fixed")
        if n:
            return n, [("resizable", 0)]
        return None

    def read_flags(self, t, i, end, ctx):
        n = t.at(i, "hidden", "invisible", "out of sight", "unseen")
        if n:
            return n, [("flag", HIDDEN)]
        n = t.at(i, "disabled", "greyed out", "grayed out", "inactive", "switched off", "turned off", "unclickable")
        if n:
            return n, [("flag", DISABLED)]
        n = t.at(i, "visible", "shown", "showing")
        if n and not ctx.get("creating"):
            return n, [("unflag", HIDDEN)]
        n = t.at(i, "enabled", "active", "clickable", "switched on")
        if n:
            return n, [("unflag", DISABLED)]
        return None

    def read_thickness(self, t, i, end, ctx):
        if not ctx.get("drawing"):
            return None
        found = self.gui_amount(t, i + t.at(i, "thickness", "with thickness", "line width", "width"))
        k = t.at(i, "thickness", "with thickness", "line width", "width")
        if found and isinstance(found[0], int) and (k or t.at(i + found[2], "thick", "wide", "thickness")):
            m = k + found[2] + t.at(i + found[2] + k, "thick", "wide", "thickness")
            return m, [("thick", found[0])]
        n = t.at(i, "thick", "thin", "fat", "bold")
        if n:
            return n, [("thick", 1 if t.w[i] == "thin" else 4)]
        return None

    # ---------------- turning effects into code ----------------

    def gui_resolve(self, kind, effects):
        """Effects that depend on the kind of element: 'red' is the fill, or for a text its colour."""
        out = []
        for e in effects:
            if e[0] == "colour":
                if kind == TEXTKIND:
                    out.append(("set", S + TEXT_COLOUR, e[1]))
                else:
                    out += [("set", S + FILL, 1), ("set", S + COLOUR, e[1])]
            elif e[0] == "bg":
                out += [("set", S + FILL, 1), ("set", S + COLOUR, e[1])]
            elif e[0] == "set" and e[1] == S + FILL + 0 and False:
                pass
            else:
                out.append(e)
        if any(e[:2] == ("set", S + TEXTURE) for e in out) and not any(e[:2] == ("set", S + FILL) for e in out) \
                and kind is None:
            out.append(("set", S + FILL, 1))
        return out

    def gui_emit_value(self, value):
        """rax = a value for a field: a whole number, ("node", tree), ("index", elem), ("textlit", s),
        ("textwords", words) or ("label", code label)."""
        a = self.a
        if isinstance(value, int):
            a.set(RAX, value)
        elif value[0] == "node":
            self.emitter.whole(self.emitter.emit(value[1]))
        elif value[0] == "index":
            self.gui_index(value[1])
        elif value[0] == "textlit":
            a.address(RAX, a.text(value[1]))
        elif value[0] == "textwords":
            self.text_value(value[1], None)
        elif value[0] == "textvalue":
            self.emitter.text(self.emitter.emit(self.value_or_text(joined(value[1]))))
        elif value[0] == "label":
            a.address(RAX, value[1])
        else:
            raise ValueError(value)

    def gui_apply(self, elem, effects):
        """Store the effects in the element's record (and call the runtime for depth, items, text)."""
        a = self.a
        effects = self.gui_resolve(elem.kind, effects)
        self.gui_note_looks(effects)
        have = False                                    # does rax hold the record's address?
        for e in effects:
            op = e[0]
            if op in ("set", "add") and isinstance(e[2], int):
                if not have:
                    self.gui_record(elem)
                    have = True
                if op == "set":
                    a.set(RCX, e[2])
                else:
                    a.load(RCX, RAX, e[1])
                    a.set(RDX, e[2])
                    a.op("add", RCX, RDX)
                a.store(RAX, e[1], RCX)
                if e[1] == S + TEXT_SIZE:
                    elem.text_size = e[2]
                    self.gui_text_sizes.add(e[2])
            elif op in ("set", "add"):
                self.gui_emit_value(e[2])
                a.push(RAX)
                self.gui_record(elem)
                a.pop(RCX)
                if op == "add":
                    a.load(RDX, RAX, e[1])
                    a.op("add", RCX, RDX)
                a.store(RAX, e[1], RCX)
                have = True
            elif op in ("flag", "unflag"):
                if not have:
                    self.gui_record(elem)
                    have = True
                a.load(RCX, RAX, FLAGS)
                a.set(RDX, e[1] if op == "flag" else ~e[1])
                a.op("or" if op == "flag" else "and", RCX, RDX)
                a.store(RAX, FLAGS, RCX)
            elif op == "other":
                self.gui_index(e[1])
                a.push(RAX)
                self.gui_record(elem)
                a.pop(RCX)
                a.store(RAX, OTHER, RCX)
                have = True
            elif op == "parent":
                self.gui_index(e[1])
                a.push(RAX)
                self.gui_record(elem)
                a.pop(RCX)
                a.store(RAX, PARENT, RCX)
                have = True
            elif op == "call":
                if e[2] is not None:
                    self.gui_index(e[2])
                    a.mov(RCX, RAX)
                    a.push(RCX)
                    self.gui_index(elem)
                    a.pop(RCX)
                else:
                    self.gui_index(elem)
                a.call(e[1])
                have = False
            elif op == "text":
                self.gui_emit_value(e[1])
                a.push(RAX)
                self.gui_index(elem)
                a.pop(RCX)
                a.call("bw_set_text")
                have = False
            elif op == "items":
                for item in e[1]:
                    self.gui_add_item(elem, item)
                have = False
            elif op == "tab":
                tabs, item = e[1], e[2]
                self.gui_index(tabs)
                a.shift_left(RAX, 16)
                a.address(RCX, tabs.items[item])
                a.load(RCX, RCX)
                a.op("add", RAX, RCX)
                a.op_n("add", RAX, 1)
                a.push(RAX)
                self.gui_record(elem)
                a.pop(RCX)
                a.store(RAX, TAB, RCX)
                have = True
            elif op == "handler":
                a.address(RAX, e[2])
                a.push(RAX)
                self.gui_record(elem)
                a.pop(RCX)
                a.store(RAX, e[1], RCX)
                have = True
            elif op == "scale":                         # "make start bigger": from its size now
                if not have:
                    self.gui_record(elem)
                ready = a.new_label("bww_has_size")
                a.load(RCX, RAX, {W: AW, H: AH}[e[1]])
                a.op("test", RCX, RCX)
                a.jump_if(NOT_EQUAL, ready)
                a.load(RCX, RAX, e[1])
                a.label(ready)
                a.set(RDX, e[2])
                a.multiply(RCX, RDX)
                a.push(RAX)
                a.mov(RAX, RCX)
                a.set(RCX, 100)
                a.sign_extend()
                a.divide(RCX, signed=True)
                a.mov(RCX, RAX)
                a.pop(RAX)
                a.store(RAX, e[1], RCX)
                a.set(RCX, PX)
                a.store(RAX, e[1] + 8, RCX)
                have = True
            elif op in ("name", "resizable", "thick"):
                pass
            else:
                raise ValueError(e)
        a.call("bw_changed")

    def gui_add_item(self, elem, item):
        a = self.a
        if item.lower() in elem.items:
            return
        slot = a.reserve(8, "bww_item")
        elem.items[item.lower()] = slot
        elem.item_names.append(item)
        self.gui_index(elem)
        a.address(RCX, a.text(item))
        a.call("bw_add_item")
        a.address(RCX, slot)
        a.store(RCX, 0, RAX)

    # ---------------- reading a phrase ----------------

    def gui_read(self, words, whole=False):
        """Read a window phrase -> a function that compiles it, else None (not a window phrase).
        Raises Problem when it plainly is one but something in it can't be read.
        whole: the words are a whole line; give None instead of a Problem if any of it is left over."""
        if not getattr(self, "gui_on", False) or not words or words[0].word is None:
            return None
        t = Toks(words)
        try:
            for reader in (self.read_gl, self.read_event_wait, self.read_window_phrase, self.read_make,
                           self.read_draw, self.read_write, self.read_change):
                plan = reader(t)
                if plan:
                    return plan
        except Problem:
            if whole:
                return None
            raise
        return None

    def gui_kind_at(self, t, i):
        for parts, kind in KINDS:
            if t.w[i:i + len(parts)] == parts:
                return kind, len(parts)
        return None

    def read_make(self, t):
        """'make a button called start saying Start at 20 20', 'add a red button', 'a text called score ...',
        'button start saying Go'."""
        verb = 1 if t.w[0] in CREATE or t.w[0] in ("i", "we") else 0
        if t.w[0] in ("i", "we") and t.w[1] in ("want", "need", "would"):
            verb = 2 + t.at(2, "like")
        if t.w[0] in ("give", "get") and t.w[1] in ("me", "us"):
            verb = 2
        if t.w[0] in ("put", "pop") and t.w[1] == "up":
            verb = 2
        if verb == 0 and t.w[0] not in ARTICLES and not self.gui_kind_at(t, 0):
            return None
        # the kind, after articles and words about how it looks ("a big red resizable window")
        j = verb
        found = None
        while j < min(t.n, verb + 7):
            found = self.gui_kind_at(t, j)
            if found:
                break
            if t.w[j] in ARTICLES or t.w[j] in ("big", "small", "large", "little", "tiny", "huge", "wide", "tall",
                                               "simple", "plain", "nice", "fancy", "proper", "basic", "empty",
                                               "blank", "main", "own", "very", "pretty", "cool", "and") \
                    or self.gui_colour(t, j) or self.gui_mod_at(t, j, t.n, {"creating": True, "kind": None}):
                found_mod = self.gui_mod_at(t, j, t.n, {"creating": True, "kind": None})
                j += found_mod[0] if found_mod and t.w[j] not in ARTICLES else 1
                continue
            return None
        if not found:
            return None
        kind, n = found
        if kind != WINDOW and " ".join(t.w[j:j + n]) in self.gui_names and \
                (j == 0 or t.w[j - 1] not in ("a", "an", "another", "new", "one", "some")):
            return None                                 # 'give card a shadow': the card that's there, not a new one
        if verb == 0 and (j + n >= t.n or t.w[j] in ("context", "gl", "3d", "text", "label", "caption", "heading")):
            return None                                 # a word alone isn't a new element: "context:"
        if kind != WINDOW and (t.w[j - 1] in ("the", "this", "its") or t.w[j + n] in ("of", "'s")):
            return None                                 # "make the text of start white": one that's there
        if t.w[0] == "draw" and kind not in (CANVAS, BOX, BUTTON, WINDOW):
            return None
        if kind == WINDOW:
            return self.window_open_plan(t, verb, j, j + n)
        k = j + n
        if kind == TEXTKIND and not (t.at(k, "called", "named", "box", "field", "area") or
                                     self.gui_window_mentioned(t) or self.uses_gui or t.at(k, "labelled")):
            return None                                 # "add red text saying hi": the console's text
        if kind == TEXTKIND and t.w[j + n - 1] == "text" and t.at(k, "saying", "that says") and \
                not self.uses_gui and not self.gui_window_mentioned(t):
            return None
        ctx = {"creating": True, "kind": kind}
        before = self.gui_mods(t, verb, j, ctx) if j > verb else []
        name = None
        # "button start saying Go": a plain word straight after the kind is its name
        if k < t.n and t.w[k] and re.fullmatch(r"[a-z_][\w'-]*", t.w[k]) and not self.gui_mod_at(t, k, t.n, ctx) \
                and t.w[k] not in FILLER and t.w[k] not in ("for", "on", "in", "at", "to", "of", "from") \
                and t.w[k] not in KIND_NOUNS:
            name, k = t.words[k].text.rstrip(","), k + 1
        after = self.gui_mods(t, k, t.n, ctx)
        effects = before + after
        for e in effects:
            if e[0] == "name":
                name = e[1]
        effects = [e for e in effects if e[0] != "name"]
        texts = [e for e in effects if e[0] == "text"]
        if kind in (BUTTON, MENU) and not texts and name:
            effects.append(("text", name))
        if kind == INPUT and texts:
            pass
        if t.w[0] == "draw" and kind == CANVAS and not texts:
            pass

        def plan():
            self.gui_make(kind, name, effects)
        return plan

    def gui_window_mentioned(self, t):
        return "window" in t.w[:t.n]

    def gui_make(self, kind, name, effects):
        a = self.a
        self.gui_need_window()
        texts = [e[1] for e in effects if e[0] == "text"]
        label = name or (texts[-1] if texts else None)
        number = len(self.gui_order) + 1
        shown = label or f"{KIND_NAMES[kind]} {number}"
        if name and name.lower() in self.gui_names and self.gui_names[name.lower()].name.lower() == name.lower():
            raise Problem(f"There's already something called {name}. Give this one another name, like: "
                          f"make a {KIND_NAMES[kind]} called {name}2")
        slot = a.reserve(8, "bww_element")
        elem = Elem(shown, kind, slot)
        a.set(RAX, kind)
        a.address(RCX, a.text(shown))
        a.call("bw_new")
        a.address(RCX, slot)
        a.store(RCX, 0, RAX)
        self.gui_order.append(elem)
        if name:
            self.gui_names[name.lower()] = elem
        for text in texts:
            self.gui_names.setdefault(text.lower(), elem)
        self.gui_names.setdefault(shown.lower(), elem)
        if kind == GLVIEW:
            elem.view = a.reserve(8, "bww_view")
            self.gui_record(elem)
            a.load(RAX, RAX, DATA)
            a.address(RCX, elem.view)
            a.store(RCX, 0, RAX)
        out = []
        for e in effects:
            if e[0] == "text":
                out.append(("text", ("textlit", e[1])))
            elif e[0] in ("resizable",):
                continue
            else:
                out.append(e)
        if kind == INPUT and not texts:
            pass
        self.gui_apply(elem, out)
        self.made_just_now = None
        return elem

    # ---- the window ----

    def window_open_plan(self, t, verb, start, after):
        ctx = {"creating": True, "kind": WINDOW}
        effects = (self.gui_mods(t, verb, start, ctx) if start > verb else []) + self.gui_mods(t, after, t.n, ctx)
        if t.w[0] in ("draw",):
            return None

        def plan():
            self.gui_window_settings(effects, opening=True)
        return plan

    def gui_window_settings(self, effects, opening=False):
        """Title, size and resizable go in the slots bww_window opens it with; the rest is element 0's."""
        a = self.a
        self.gui_use()
        title = [e[1] for e in effects if e[0] in ("name", "text")]
        if title:
            a.address(RAX, a.text(title[-1]))
            a.address(RCX, self.gui_slot("title"))
            a.store(RCX, 0, RAX)
        size = {}
        rest = []
        for e in effects:
            if e[0] == "set" and e[1] in (W, H, WU, HU, X, Y, XU, YU):
                size[e[1]] = e[2]
            elif e[0] == "resizable":
                a.set(RAX, e[1])
                a.address(RCX, self.gui_slot("resizable"))
                a.store(RCX, 0, RAX)
            elif e[0] not in ("name", "text"):
                rest.append(e)
        if any(k in size for k in (X, Y, XU, YU)):
            raise Problem("brick's own window can't place itself on the desktop (Wayland decides where windows go). "
                          "You can say: open a window 800 by 600  |  put the console on the left side of the screen")
        changes = []
        for value, unit, slot, screen in ((W, WU, "w", SCREEN[0]), (H, HU, "h", SCREEN[1])):
            if value not in size:
                continue
            v, u = size[value], size.get(unit, PX)
            if u in (PERCENT, PER_MILLE) and isinstance(v, int):
                v = round(screen * v / (100 if u == PERCENT else 1000))
            elif u not in (PX,):
                raise Problem("Give the window's size in px or %, like: open a window 800 by 600")
            self.gui_emit_value(v)
            a.address(RCX, self.gui_slot(slot))
            a.store(RCX, 0, RAX)
            changes.append((value, slot))
        if changes:                                 # already open: element 0 takes the new size
            skip = a.new_label("bww_not_open")
            a.address(RAX, self.gui_slot("opened"))
            a.load(RAX, RAX)
            a.op("test", RAX, RAX)
            a.jump_if(EQUAL, skip)
            for value, slot in changes:
                a.call("bw_elem") if False else None
                self.gui_record(self.gui_window)
                a.address(RCX, self.gui_slot(slot))
                a.load(RCX, RCX)
                a.store(RAX, value, RCX)
                a.set(RCX, PX)
                a.store(RAX, value + 8, RCX)
            a.call("bw_changed")
            a.label(skip)
        if opening or rest:
            a.call("bww_window")
        if rest:
            self.gui_apply(self.gui_window, rest)

    def read_window_phrase(self, t):
        """The window itself: 'make the window 800 by 600', 'name the window My Game', 'close the window',
        'set the title of the window to X', 'make the window navy', 'make the window full screen'."""
        words = t.w[:t.n]
        if "window" not in words or {"console", "terminal"} & set(words):
            return None
        verb = t.w[0]
        wi = words.index("window")
        if verb in ("close", "shut", "quit", "exit", "end", "stop") and wi <= 3:
            return lambda: (self.gui_use(), self.a.call("bw_close"))
        if verb in ("name", "call", "title", "rename", "retitle", "label"):
            k = wi + 1 + t.at(wi + 1, "to", "as", "with the title", "is")
            title = t.raw(k, t.n)
            if not title:
                raise Problem("Name the window what? Like: name the window My Game")
            return lambda: self.gui_window_settings([("name", title)])
        n = t.at(1, "the title of the window to", "the window title to", "the window's title to",
                 "the title of the window as", "the window title as", "the window name to", "the name of the window to",
                 "the caption of the window to", "the window caption to", "title of the window to", "window title to")
        if verb in ("set", "change", "make") and n:
            title = t.raw(1 + n, t.n)
            if not title:
                raise Problem("Set the title to what? Like: set the title of the window to My Game")
            return lambda: self.gui_window_settings([("name", title)])
        if verb in ("give",) and t.at(1, "the window the title", "the window a title of", "the window the name"):
            n = t.at(1, "the window the title", "the window a title of", "the window the name")
            title = t.raw(1 + n, t.n)
            return lambda: self.gui_window_settings([("name", title)])
        moved = wi <= 2 or t.w[wi - 1] in ("the", "my", "this") and wi <= 3 and t.w[1] not in WRITE
        if t.w[wi - 1] == "of" or t.w[wi - 1] in ("in", "on", "to", "into", "onto", "inside") and wi > 2:
            moved = False                           # "put hi at the bottom of the window": words in it
        if verb in ("put", "move", "place", "centre", "center", "position", "snap", "dock", "push", "drag", "shift",
                    "slide", "nudge", "go", "send") and moved or \
                verb in ("make", "set") and moved and set(words) & {"full", "fullscreen", "maximised", "maximized",
                                                                    "bigger", "smaller", "minimised", "minimized",
                                                                    "wider", "taller", "narrower", "shorter",
                                                                    "larger", "tinier"} or \
                verb in ("maximise", "maximize", "minimise", "minimize", "shrink", "grow", "enlarge", "fullscreen"):
            if set(words) & {"full", "fullscreen", "maximised", "maximized", "maximise", "maximize", "maximum"}:
                raise Problem("brick's own window can't go full screen yet. You can say: open a resizable window "
                              "100% by 100%  |  make the console full screen")
            if set(words) & {"bigger", "smaller", "shrink", "grow", "enlarge", "minimise", "minimize", "minimised",
                             "minimized", "wider", "taller", "narrower", "shorter", "larger", "tinier"}:
                raise Problem("Say the window's new size, like: make the window 800 by 600")
            raise Problem("brick's own window can't move itself: the desktop (Wayland) decides where windows go. "
                          "You can say: make the window 800 by 600  |  put the console on the left side of the screen")
        if verb in ("resize", "make", "set", "change", "give", "colour", "color", "paint", "turn", "fill", "let"):
            start = wi + 1
            if not (t.w[wi - 1] in ("the", "this", "my", "our") and wi <= 2) and not (wi == 1):
                return None
            k = start + t.at(start, "to", "into", "be", "is", "size", "a size of", "the size", "to be")
            n = t.at(1, "the size of the window to", "the window size to", "the window's size to",
                     "the background of the window to", "the background colour of the window to",
                     "the colour of the window to", "the window colour to", "the window background to",
                     "the window's colour to", "the window's background to", "the color of the window to",
                     "the background color of the window to", "the window color to")
            if n:
                k = 1 + n
            elif verb in ("set", "change") and t.w[1] not in ("the", "this", "my"):
                return None
            ctx = {"creating": True, "kind": WINDOW, "stop": True}
            effects = self.gui_mods(t, k, t.n, ctx)
            if not effects:
                raise Problem("What should the window be like? You can say: make the window 800 by 600  |  "
                              "make the window navy  |  give the window a gradient from black to navy")
            return lambda: self.gui_window_settings(effects)
        if verb == "clear" and wi <= 2:
            def clear_all():
                canvases = [e for e in self.gui_order if e.kind == CANVAS]
                if not canvases:
                    raise Problem("There's nothing drawn in the window to clear. To clear the console, say: clear "
                                  "the console")
                for canvas in canvases:
                    self.gui_index(canvas)
                    self.a.call("bw_clear")
                self.a.call("bw_changed")
            return clear_all
        if verb in ("open", "show", "start", "launch") and wi <= 3:
            return self.window_open_plan(t, 1, wi, wi + 1)
        return None

    # ---- drawing on a canvas ----

    def gui_canvas_target(self, t, ctx_kinds=(CANVAS,)):
        """'on board', 'in board', 'onto board' anywhere -> (canvas, where those words are, how many), or None."""
        for i in range(1, t.n):
            if t.w[i] in ("on", "in", "onto", "into", "to", "inside"):
                found = self.gui_element_at(t, i + 1 + t.at(i + 1, "the"), ctx_kinds)
                if found:
                    return found[0], i, 1 + t.at(i + 1, "the") + found[1]
        return None

    def read_draw(self, t):
        """'draw a red circle on board at 50 50, 30 by 30', 'draw a line on board from 0 0 to 100 100 in white,
        3 px thick', 'draw a triangle on board at 0 0, 50 0 and 25 40', 'draw a red dot on board at 10 10'."""
        if t.w[0] not in ("draw", "paint", "sketch", "add", "put", "make", "place", "plot", "stamp"):
            return None
        target = self.gui_canvas_target(t)
        j = 1
        while j < t.n and (t.w[j] in ARTICLES or self.gui_colour(t, j) or t.w[j] in ("big", "small", "large",
                                                                                    "little", "tiny", "filled",
                                                                                    "solid", "thick", "thin", "huge")
                           or self.gui_texture_word(t, j)):
            j += self.gui_colour(t, j)[1] if self.gui_colour(t, j) else 1
        shape = t.w[j] if t.w[j] in SHAPES else None
        if shape is None:
            return None
        if target is None:
            canvases = [e for e in self.gui_order if e.kind == CANVAS]
            if t.w[0] != "draw":
                return None
            if not canvases:
                raise Problem("Draw it on what? Make a canvas first, like: make a canvas called board 400 by 300, "
                              "then: draw a red circle on board at 50 50, 30 by 30")
            canvas, where, size = canvases[-1], t.n, 0
        else:
            canvas, where, size = target
        kind = SHAPES[shape]
        rest = list(range(1, j)) + list(range(j + 1, where)) + list(range(where + size, t.n))
        words = [t.words[k] for k in rest]
        return self.draw_plan(canvas, kind, shape, Toks(words), t.w[1:j])

    def draw_plan(self, canvas, kind, shape, t, adjectives):
        ctx = {"drawing": True, "kind": None}
        points, effects, i = [], [], 0
        sizes = {"big": 2, "large": 2, "huge": 3, "small": 0.5, "little": 0.5, "tiny": 0.3}
        scale = next((sizes[w] for w in adjectives if w in sizes), 1)
        while i < t.n:
            if t.w[i] in sizes or t.w[i] in ("filled", "solid", "a", "an"):
                i += 1
                continue
            k = t.at(i, "at", "from", "between", "to", "and", "with corners at", "with points at", "through",
                     "then", "via", "centred at", "centered at", "with its centre at", "around", "starting at",
                     "ending at")
            found = self.gui_point(t, i + k) if (k or (points and t.w[i] is not None)) else None
            if found and not (t.w[i + k + found[2]] in ("by", "x", "wide", "tall", "high")):
                (xv, xu), (yv, yu), n = found
                if xu not in (PX,) or yu not in (PX,):
                    raise Problem("Shapes on a canvas are placed in px from its top left, like: at 50 50")
                points.append((xv, yv))
                i += k + n
                continue
            n = t.at(i, "radius", "with a radius of", "with radius", "of radius", "a radius of")
            if n:
                found = self.gui_amount(t, i + n)
                if found:
                    effects.append(("radius", found[0]))
                    i += n + found[2]
                    continue
            found = self.gui_mod_at(t, i, t.n, ctx)
            if found:
                effects += found[1]
                i += found[0]
                continue
            if t.w[i] in FILLER or t.w[i] in ("it", "shape"):
                i += 1
                continue
            raise Problem(f"I don't understand '{t.words[i].raw}' in that drawing. You can say: "
                          + "  |  ".join(GUI_EXAMPLES["canvas"][1:4]))
        colour = next((e[1] for e in effects if e[0] in ("colour", "bg")), None)
        thick = next((e[1] for e in effects if e[0] == "thick"), None)
        radius = next((e[1] for e in effects if e[0] == "radius"), None)
        size = [e for e in effects if e[0] == "set" and e[1] in (W, H)]
        style = [e for e in self.gui_resolve(None, effects) if e[0] == "set" and e[1] >= S]
        if not style and colour is None:
            colour = 0xFFFFFF if kind in (3,) else 0x000000
        need = {3: 2, 4: 3}.get(kind, 1)
        if len(points) < need:
            how = {3: "from 0 0 to 100 100", 4: "at 0 0, 50 0 and 25 40"}.get(kind, "at 50 50")
            raise Problem(f"Where should the {shape} go? Like: draw a {shape} on {canvas.name} {how}")

        def plan():
            self.gui_draw(canvas, kind, points, size, radius, colour, thick, style, scale)
        return plan

    def gui_draw(self, canvas, kind, points, size, radius, colour, thick, style, scale=1):
        a = self.a
        self.gui_use()
        shape = self.gui_slot("shape", 64)
        self.gui_drawn.add(kind)
        points = [(self.gui_in_canvas(x, canvas, AX), self.gui_in_canvas(y, canvas, AY)) for x, y in points]

        def put(offset, value):
            self.gui_emit_value(value)
            a.address(RCX, shape)
            a.store(RCX, offset, RAX)
        if kind == 6:                                   # a dot: {x, y (its centre), diameter, colour}
            w = next((e[2] for e in size if e[1] == W), None)
            d = radius * 2 if isinstance(radius, int) else w if w is not None else round(8 * scale)
            x, y = points[0]
            for offset, value in ((0, 6), (8, x), (16, y), (24, d), (32, 0 if colour is None else colour)):
                put(offset, value)
        elif kind == 4:                                 # its transparency rides in the kind field
            transparency = next((e[2] for e in style if e[1] == S + TRANSPARENCY), 0)
            put(0, 4 + 256 * transparency if isinstance(transparency, int) else 4)
        else:
            put(0, kind)
        if kind in (1, 2):
            x, y = points[0]
            w = next((e[2] for e in size if e[1] == W), None)
            h = next((e[2] for e in size if e[1] == H), None)
            if isinstance(radius, int) and w is None:
                w = h = radius * 2
                x, y = self.gui_shift(x, -radius), self.gui_shift(y, -radius)
            if w is None and h is None:
                w = h = round(40 * scale)
            w = h if w is None else w
            h = w if h is None else h
            style = style or []
            if style:                                   # a gradient, texture, border ...: a style block of its own
                style_block = a.reserve(128, "bww_style")
                if colour is not None and not any(e[1] == S + FILL for e in style):
                    style = [("set", S + FILL, 1), ("set", S + COLOUR, colour)] + style
                self.gui_note_looks(style)
                for e in style:
                    self.gui_emit_value(e[2])
                    a.address(RCX, style_block)
                    a.store(RCX, e[1] - S, RAX)
                block = ("label", style_block)
            else:                                       # just a colour: style address 0, then the colour
                block = 0
            for offset, value in ((8, x), (16, y), (24, w), (32, h), (40, block),
                                  (48, 0 if colour is None else colour)):
                put(offset, value)
        elif kind == 3:
            (x1, y1), (x2, y2) = points[:2]
            transparency = next((e[2] for e in style if e[1] == S + TRANSPARENCY), 0)
            for offset, value in ((8, x1), (16, y1), (24, x2), (32, y2), (40, 0xFFFFFF if colour is None else colour),
                                  (48, thick or 1), (56, transparency)):
                put(offset, value)
        elif kind == 4:
            (x1, y1), (x2, y2), (x3, y3) = points[:3]
            for offset, value in ((8, x1), (16, y1), (24, x2), (32, y2), (40, x3), (48, y3),
                                  (56, 0 if colour is None else colour)):
                put(offset, value)
        self.gui_index(canvas)
        a.address(RCX, shape)
        a.call("bw_draw")
        a.call("bw_changed")

    def gui_shift(self, value, by):
        if isinstance(value, int):
            return value + by
        return ("node", ("maths", "add", value[1], ("number", by)))

    def gui_in_canvas(self, value, canvas, field):
        """'the mouse x' on a canvas means from the canvas's left, not the window's."""
        if isinstance(value, int) or value[0] != "node":
            return value

        def swap(node):
            if isinstance(node, tuple) and node[:2] == ("bw", "mouse"):
                return ("bw", "mouse_in", node[2], canvas.name.lower())
            if isinstance(node, tuple):
                return tuple(swap(n) for n in node)
            return node
        return ("node", swap(value[1]))

    # ---- writing words in the window or on a canvas ----

    def read_write(self, t):
        """'write hi in the window', 'write hello on board at 10 10 in gold', 'write hi to the console'."""
        if t.w[0] not in WRITE:
            return None
        canvas = self.gui_canvas_target(t)
        if canvas:
            elem, where, size = canvas
            return self.write_on_canvas(elem, t, where, size)
        if "window" not in t.w[:t.n] or {"console", "terminal"} & set(t.w[:t.n]):
            return None
        wi = t.w.index("window")
        if wi <= 2 and t.w[0] not in ("write", "say", "print", "display", "output", "type"):
            return None                                 # "put the window ...": the window itself
        keep = []
        i = 1
        while i < t.n:
            n = t.at(i, "in the window", "on the window", "to the window", "into the window", "inside the window",
                     "in my window", "on my window", "in this window", "on this window", "onto the window",
                     "in the bitwin window", "in a window", "in window", "on window", "to window")
            if n:
                i += n
                continue
            keep.append(t.words[i])
            i += 1
        return self.write_in_window(Toks(keep))

    def write_in_window(self, t):
        ctx = {"creating": True, "kind": TEXTKIND}
        j = 0
        while j < t.n and not (j and self.gui_mod_at(t, j, t.n, ctx, stop_only=True)):
            j += 1
        if j == 0:
            raise Problem("Write what in the window? Like: write hello in the window at the top in gold")
        said = t.words[:j]
        effects = self.gui_mods(t, j, t.n, ctx)
        texts = [e[1] for e in effects if e[0] == "text"]          # "a message saying hello there"
        if texts and all(w.word in ("a", "an", "the", "some", "message", "text", "words", "note", "label", "sign",
                                    "caption", "writing") or self.gui_colour(t, k) for k, w in enumerate(said)):
            content = ("textlit", texts[-1])
        else:
            content = ("textwords", said)

        def plan():
            elem = self.gui_make(TEXTKIND, None, [e for e in effects if e[0] != "text"])
            self.gui_apply(elem, [("text", content)])
        return plan

    def write_on_canvas(self, canvas, t, where, size):
        words = t.words[1:where] + t.words[where + size:]
        t = Toks(words)
        ctx = {"drawing": True, "kind": TEXTKIND}
        j = 0
        while j < t.n and not (j and (self.gui_mod_at(t, j, t.n, ctx, stop_only=True) or
                                      t.at(j, "at") and self.gui_point(t, j + 1))):
            j += 1
        said = t.words[:j]
        if not said:
            raise Problem(f"Write what on {canvas.name}? Like: write hello on {canvas.name} at 10 10 in gold")
        point, effects, i = (0, 0), [], j
        while i < t.n:
            k = t.at(i, "at", "from")
            found = self.gui_point(t, i + k) if k else None
            if found:
                point = (found[0][0], found[1][0])
                i += k + found[2]
                continue
            found = self.gui_mod_at(t, i, t.n, ctx)
            if found:
                effects += found[1]
                i += found[0]
                continue
            if t.w[i] in FILLER:
                i += 1
                continue
            raise Problem(f"I don't understand '{t.words[i].raw}' there. You can say: write hello on "
                          f"{canvas.name} at 10 10 in gold")
        colour = next((e[1] for e in effects if e[0] in ("colour", "bg")), None)
        colour = next((e[2] for e in effects if e[0] == "set" and e[1] == S + TEXT_COLOUR), colour)
        text_size = next((e[2] for e in effects if e[0] == "set" and e[1] == S + TEXT_SIZE), 16)
        transparency = next((e[2] for e in effects if e[0] == "set" and e[1] == S + TRANSPARENCY), 0)

        def plan():
            a = self.a
            self.gui_use()
            shape = self.gui_slot("shape", 64)
            self.gui_drawn.add(5)                               # text
            x, y = self.gui_in_canvas(point[0], canvas, AX), self.gui_in_canvas(point[1], canvas, AY)
            if isinstance(text_size, int):
                self.gui_text_sizes.add(text_size)
            for offset, value in ((0, 5), (8, x), (16, y), (24, ("textwords", said)),
                                  (32, 0 if colour is None else colour), (40, text_size), (48, transparency)):
                self.gui_emit_value(value)
                a.address(RCX, shape)
                a.store(RCX, offset, RAX)
            self.gui_index(canvas)
            a.address(RCX, shape)
            a.call("bw_draw")
            a.call("bw_changed")
        return plan

    # ---- changing an element that's there ----

    def read_change(self, t):
        a = self.a
        verb = t.w[0]
        # "the x of start", "start's colour": set the FIELD of ELEMENT to ...
        if verb in ("set", "change", "make", "let", "turn") and t.w[1] in ("the", "its") or \
                verb in ("set", "change", "make") and (t.w[1] or "").endswith("'s"):
            plan = self.read_set_field(t)
            if plan:
                return plan
        if verb in ("hide", "show", "unhide", "reveal", "disable", "enable", "grey", "gray", "open", "close",
                    "shut", "fold", "collapse", "expand"):
            k = 1 + t.at(1, "out")
            found = self.gui_element_at(t, k)
            if not found:
                return None
            elem, n = found
            if verb == "show" and self.find(elem.name.lower().replace(" ", "_")):
                return None                             # "show score": the name's value, on the console
            j = k + n + t.at(k + n, "out", "again", "now", "up", "menu", "'s menu")
            if j < t.n:
                return None
            if verb in ("open", "expand"):
                if elem.kind != MENU:
                    return None
                return lambda: (self.gui_use(), self.gui_apply(elem, [("flag", OPEN)]))
            if verb in ("close", "shut", "fold", "collapse"):
                if elem.kind != MENU:
                    raise Problem(f"Only menus close. To make {elem.name} go away, say: hide {elem.name}")
                return lambda: (self.gui_use(), self.gui_apply(elem, [("unflag", OPEN)]))
            flag = {"hide": ("flag", HIDDEN), "show": ("unflag", HIDDEN), "unhide": ("unflag", HIDDEN),
                    "reveal": ("unflag", HIDDEN), "disable": ("flag", DISABLED), "enable": ("unflag", DISABLED),
                    "grey": ("flag", DISABLED), "gray": ("flag", DISABLED)}[verb]
            return lambda: (self.gui_use(), self.gui_apply(elem, [flag]))
        if verb in ("close", "shut") and t.at(1, "the menu", "menus", "the menus", "all menus", "every menu"):
            menus = [e for e in self.gui_order if e.kind == MENU]
            return lambda: (self.gui_use(), [self.gui_apply(m, [("unflag", OPEN)]) for m in menus])
        if verb == "clear" or verb in ("empty", "wipe", "erase") :
            found = self.gui_element_at(t, 1, (CANVAS,))
            if found and 1 + found[1] == t.n:
                elem = found[0]
                return lambda: (self.gui_use(), self.gui_index(elem), a.call("bw_clear"), a.call("bw_changed"))
            return None
        if verb == "bring":
            found = self.gui_element_at(t, 1)
            if not found:
                return None
            elem, n = found
            rest = t.w[1 + n:t.n]
            if rest in (["forward"], ["forwards"], ["up"]):
                return lambda: (self.gui_use(), self.gui_apply(elem, [("add", DEPTH, 1)]))
            effects = self.gui_mods(t, 1 + n, t.n, {"kind": elem.kind})
            return lambda: (self.gui_use(), self.gui_apply(elem, effects or [("call", "bw_front", None)]))
        if verb == "send":
            found = self.gui_element_at(t, 1)
            if not found:
                return None
            elem, n = found
            rest = t.w[1 + n:t.n]
            if rest in (["back"], ["backward"], ["backwards"], ["down"], ["back", "one"]):
                return lambda: (self.gui_use(), self.gui_apply(elem, [("add", DEPTH, -1)]))
            if rest in (["to", "the", "back"], ["to", "back"], ["to", "the", "bottom"], ["behind", "everything"],
                        ["right", "to", "the", "back"]):
                return lambda: (self.gui_use(), self.gui_apply(elem, [("call", "bw_back", None)]))
            effects = self.gui_mods(t, 1 + n, t.n, {"kind": elem.kind})
            return lambda: (self.gui_use(), self.gui_apply(elem, effects))
        if verb == "round" and t.at(1, "the corners of", "off the corners of", "the corners on", "the edges of",
                                    "off", "the corners off"):
            n = t.at(1, "the corners of", "off the corners of", "the corners on", "the edges of", "off",
                     "the corners off")
            found = self.gui_element_at(t, 1 + n)
            if not found:
                return None
            elem, m = found
            k = 1 + n + m
            k += t.at(k, "by", "to", "with", "of", "corners", "'s corners")
            amount = self.gui_amount(t, k)
            size = amount[0] if amount else 12
            if amount and k + amount[2] < t.n and t.w[k + amount[2]] not in ("px", "pixels"):
                pass
            return lambda: (self.gui_use(), self.gui_apply(elem, [("set", S + RADIUS, size)]))
        if verb in ("move", "go", "shift", "nudge", "slide", "push") and self.gui_element_at(t, 1):
            elem, n = self.gui_element_at(t, 1)
            k = 1 + n
            if elem.kind == WINDOW:
                return None
            if t.w[k] in ("forward", "forwards", "back", "backward", "backwards", "backwards") and k + 1 >= t.n:
                step = 1 if t.w[k].startswith("forward") else -1
                return lambda: (self.gui_use(), self.gui_apply(elem, [("add", DEPTH, step)]))
            directions = {"right": (X, 1), "left": (X, -1), "down": (Y, 1), "up": (Y, -1), "across": (X, 1)}
            if t.w[k] in directions:
                field, sign = directions[t.w[k]]
                j = k + 1 + t.at(k + 1, "by")
                amount = self.gui_amount(t, j)
                if amount and amount[1] != PX:
                    raise Problem(f"Move {elem.name} by px, like: move {elem.name} right 20")
                step = amount[0] if amount else 10
                j += amount[2] if amount else 0
                j += t.at(j, "px", "pixels", "steps")
                if j < t.n:
                    more = self.gui_mods(t, j, t.n, {"kind": elem.kind})
                else:
                    more = []
                value = step * sign if isinstance(step, int) else ("node", ("maths", "mul", step[1], ("number", sign)))
                return lambda: (self.gui_use(), self.gui_apply(elem, [("add", field, value)] + more))
            effects = self.gui_mods(t, k, t.n, {"kind": elem.kind})
            if not effects:
                raise Problem(f"Move {elem.name} where? You can say: move {elem.name} to 100 100  |  "
                              f"move {elem.name} right 20  |  move {elem.name} below board")
            return lambda: (self.gui_use(), self.gui_apply(elem, effects))
        if verb == "resize" and self.gui_element_at(t, 1):
            elem, n = self.gui_element_at(t, 1)
            k = 1 + n + t.at(1 + n, "to", "so it is", "to be")
            effects = self.gui_mods(t, k, t.n, {"kind": elem.kind})
            return lambda: (self.gui_use(), self.gui_apply(elem, effects))
        if verb in ("put", "add", "place", "insert", "stick") and not self.gui_element_at(t, 1):
            # "add Save to file": a new item in a menu or a tab
            for i in range(2, t.n):
                if t.w[i] in ("to", "in", "into", "on", "onto"):
                    found = self.gui_element_at(t, i + 1 + t.at(i + 1, "the"), (MENU, TABS))
                    if found and i + 1 + t.at(i + 1, "the") + found[1] + t.at(i + 1 + t.at(i + 1, "the") + found[1],
                                                                             "menu", "tabs") == t.n:
                        items = [" ".join(w.text if w.quoted else w.raw for w in item).strip(" ,.")
                                 for item in list_items(t.words[1:i])]
                        elem = found[0]
                        return lambda: (self.gui_use(), self.gui_apply(elem, [("items", items)]))
            return None
        # "make start red", "give start a shadow", "put start below board", "start is red"
        found = None
        k = 0
        if verb in ("make", "give", "put", "place", "position", "set", "colour", "color", "paint", "turn", "let",
                    "have", "change", "fill", "style", "dress", "draw", "show", "add", "move", "stick"):
            found = self.gui_element_at(t, 1)
            k = 1
        if not found:
            found = self.gui_element_at(t, 0)
            k = 0
            if not found:
                return None
            n = found[1]
            if not t.at(n, "is", "are", "becomes", "should be", "turns", "goes", "gets", "will be", "has", "now is",
                        "is now", "sits", "stays", "looks", "is placed", "is put", "goes to", "moves to", "must be"):
                return None
            k = n + t.at(n, "is", "are", "becomes", "should be", "turns", "goes", "gets", "will be", "has", "now is",
                         "is now", "sits", "stays", "looks", "is placed", "is put", "goes to", "moves to", "must be")
            elem = found[0]
        else:
            elem, n = found
            k = 1 + n
        if elem.kind == WINDOW:
            return None
        k += t.at(k, "to", "into", "so it is", "so that it is", "look", "be", "to be", "to look")
        if t.at(k, "say", "says", "read", "reads", "show", "display"):
            m = t.at(k, "say", "says", "read", "reads", "show", "display")
            words = t.words[k + m:]
            return lambda: (self.gui_use(), self.gui_apply(elem, [("text", ("textvalue", words))]))
        try:
            effects = self.gui_mods(t, k, t.n, {"kind": elem.kind, "stop": True})
        except Problem as problem:
            if verb in ("set", "let", "change", "make", "have", "put", "add", "show") and \
                    self.find(elem.name.lower().replace(" ", "_")):
                return None                             # "make score 0": the name score, not the text
            if verb in ("set", "let", "change") or (verb == "make" and self.gui_plain_value(t.words[k:])):
                return None
            raise Problem(f"{problem} You can say: make {elem.name} red  |  put {elem.name} at 20 20  |  "
                          f"give {elem.name} a shadow  |  make {elem.name} 120 by 40")
        if not effects:
            if verb in ("set", "let", "change", "make", "put", "add", "show"):
                return None
            raise Problem(f"What should {elem.name} be like? You can say: make {elem.name} red  |  "
                          f"give {elem.name} a 2 px gold border")
        return lambda: (self.gui_use(), self.gui_apply(elem, effects))

    def gui_plain_value(self, words):
        try:
            read_value(words, self.kind_of)
            return True
        except Problem:
            return False

    def read_set_field(self, t):
        """'set the text of score to "Score: 1"', 'set the depth of start to 5', 'make the text of start white
        and size 20', 'set start's colour to red', 'change the width of board to 50%'."""
        i = 1 + t.at(1, "the", "its")
        fields = ["text colour", "text color", "font colour", "font color", "text size", "font size", "background colour",
                  "background color", "border colour", "border color", "text", "label", "words", "caption", "colour",
                  "color", "background", "fill", "x", "y", "left", "top", "width", "height", "depth", "layer", "size",
                  "position", "place", "title", "name", "corners", "corner radius", "border", "shadow", "texture",
                  "gradient", "transparency", "opacity", "hover colour", "hover color", "content", "contents", "value",
                  "items", "page", "tab", "message"]
        field = None
        elem = None
        if (t.w[1] or "").endswith("'s"):
            found = self.gui_find([Word(t.words[1].text[:-2])])
            if not found:
                return None
            elem = found
            n = max((t.at(2, f) for f in fields), default=0)
            if not n:
                return None
            field, k = " ".join(t.w[2:2 + n]), 2 + n
        else:
            n = max((t.at(i, f) for f in fields), default=0)
            if not n or not t.at(i + n, "of", "for", "on", "in"):
                return None
            field = " ".join(t.w[i:i + n])
            found = None
            j = i + n + 1
            for m in range(min(5, t.n - j), 0, -1):
                if t.w[j + m] in ("to", "as", "into", "=", "be", "is") or j + m >= t.n or True:
                    candidate = self.gui_find(t.words[j:j + m])
                    if candidate and (j + m >= t.n or t.w[j + m] in ("to", "as", "into", "=", "be", "is") or
                                      t.w[0] in ("make", "turn")):
                        found = (candidate, m)
                        break
            if not found:
                return None
            elem, m = found
            k = j + m
        verb = t.w[0]
        k += t.at(k, "to", "as", "into", "=", "be", "to be", "is")
        rest = t.words[k:]
        if not rest:
            raise Problem(f"Set the {field} of {elem.name} to what? Like: set the {field} of {elem.name} to "
                          + ('"hello"' if field in ("text", "label") else "red" if "colo" in field else "20"))
        if elem.kind == WINDOW:
            if field in ("title", "name", "text", "caption"):
                title = t.raw(k, t.n)
                return lambda: self.gui_window_settings([("name", title)])
            effects = self.gui_mods(t, k, t.n, {"kind": WINDOW, "stop": True})
            return lambda: self.gui_window_settings(effects)
        if field in ("text", "label", "words", "caption", "content", "contents", "value", "message", "title", "name"):
            if verb in ("make", "turn"):
                try:
                    effects = self.gui_mods(t, k, t.n, {"kind": TEXTKIND, "stop": True})
                except Problem:
                    effects = None
                if effects:                             # "make the text of start white and size 20"
                    effects = [("set", S + TEXT_COLOUR, e[1]) if e[0] in ("colour", "bg") else e for e in effects]
                    return lambda: (self.gui_use(), self.gui_apply(elem, effects))
            return lambda: (self.gui_use(), self.gui_apply(elem, [("text", ("textvalue", rest))]))
        if field in ("x", "y", "left", "top", "width", "height", "depth", "layer"):
            found = self.gui_amount(t, k, elem.text_size)
            if not found or k + found[2] != t.n:
                raise Problem(f"Set the {field} of {elem.name} to what number? Like: set the {field} of {elem.name} to 20")
            v, u, _ = found
            off = {"x": X, "left": X, "y": Y, "top": Y, "width": W, "height": H, "depth": DEPTH, "layer": DEPTH}[field]
            effects = [("set", off, v)] + ([("set", off + 8, u)] if off != DEPTH else [])
            return lambda: (self.gui_use(), self.gui_apply(elem, effects))
        ctx = {"kind": elem.kind, "stop": True}
        if field in ("text colour", "text color", "font colour", "font color"):
            colour = self.gui_colour(t, k)
            if not colour:
                raise Problem(f"Which colour? Like: set the text colour of {elem.name} to white")
            return lambda: (self.gui_use(), self.gui_apply(elem, [("set", S + TEXT_COLOUR, colour[0])]))
        if field in ("text size", "font size"):
            found = self.gui_amount(t, k)
            if not found:
                raise Problem(f"How big? Like: set the text size of {elem.name} to 20")
            return lambda: (self.gui_use(), self.gui_apply(elem, [("set", S + TEXT_SIZE, found[0])]))
        if field in ("background", "background colour", "background color", "fill"):
            colour = self.gui_colour(t, k)
            if colour:
                return lambda: (self.gui_use(), self.gui_apply(elem, [("bg", colour[0])]))
        if field in ("hover colour", "hover color"):
            colour = self.gui_colour(t, k)
            if colour:
                return lambda: (self.gui_use(), self.gui_apply(elem, [("set", HOVER, colour[0]), ("flag", HAS_HOVER)]))
        if field in ("border colour", "border color"):
            colour = self.gui_colour(t, k)
            if colour:
                return lambda: (self.gui_use(), self.gui_apply(elem, [("set", S + BORDER_COLOUR, colour[0])]))
        if field in ("corners", "corner radius"):
            found = self.gui_amount(t, k)
            if found:
                return lambda: (self.gui_use(), self.gui_apply(elem, [("set", S + RADIUS, found[0])]))
        if field in ("transparency", "opacity"):
            found = self.gui_amount(t, k)
            if found and isinstance(found[0], int):
                pct = found[0] / (10 if found[1] == PER_MILLE else 1)
                pct = 100 - pct if field == "opacity" else pct
                return lambda: (self.gui_use(), self.gui_apply(elem, [("set", S + TRANSPARENCY, round(255 * pct / 100))]))
        if field in ("page", "tab"):
            item = self.gui_item(rest, elem) if elem.kind == TABS else None
            if item:
                slot = elem.items[item[1]]

                def pick():
                    self.gui_use()
                    a.address(RAX, slot)
                    a.load(RAX, RAX)
                    a.push(RAX)
                    self.gui_record(elem)
                    a.pop(RCX)
                    a.store(RAX, PICKED, RCX)
                    a.call("bw_changed")
                return pick
        effects = self.gui_mods(t, k, t.n, ctx)
        if not effects:
            raise Problem(f"Set the {field} of {elem.name} to what?")
        return lambda: (self.gui_use(), self.gui_apply(elem, effects))

    # ---- waiting ----

    def read_event_wait(self, t):
        """'wait until start is clicked', 'wait for start to be clicked', 'wait for a click on start',
        'wait for a click'."""
        if t.w[0] not in ("wait", "pause", "hold"):
            return None
        k = 1 + t.at(1, "until", "till", "for", "untill", "til", "unless")
        if k == 1:
            return None
        if t.at(k, "a click", "any click", "something to be clicked", "anything to be clicked", "a button to be clicked",
                "someone to click", "the user to click", "a button press", "something is clicked",
                "anything is clicked", "a button is clicked", "any button is clicked", "the user clicks",
                "someone clicks") and not t.at(k + t.at(k, "a click", "any click"), "on"):
            if not self.uses_gui and not self.gui_order:
                return None
            return lambda: (self.gui_need_window(), self.a.set(RAX, -1), self.a.call("bw_wait_click"))
        j = k + t.at(k, "a click on", "someone to click", "the user to click", "someone clicks", "the user clicks",
                     "you click", "a press of", "a click of", "someone to press", "a tap on", "click on")
        j += t.at(j, "on")
        found = self.gui_element_at(t, j)
        if not found or found[0].kind == WINDOW:
            return None
        elem, n = found
        end = j + n + t.at(j + n, "is clicked", "gets clicked", "to be clicked", "is pressed", "to be pressed",
                           "is tapped", "to be tapped", "is hit", "is chosen", "to get clicked", "is clicked on",
                           "has been clicked", "was clicked")
        if end != t.n:
            return None

        def plan():
            self.gui_need_window()
            self.gui_index(elem)
            self.a.call("bw_wait_click")
        return plan

    # ---- 3D (glspace) ----

    def names_2d_thing(self, t):
        """Does the line name one of the program's 2D things (a box called ball), and nothing 3D besides
        (no 3D shape word of its own, like 'sphere' in 'put a blue sphere in view', and no 3D view named)?"""
        mine, i = set(), 0
        while i < t.n:
            found = self.gui_element_at(t, i)
            if found and found[0].kind == GLVIEW:
                return False
            said = " ".join(w or "" for w in t.w[i:i + found[1]] if w not in ("the", "my", "a", "an")) if found else ""
            if found and found[0].name and said == found[0].name.lower() and said not in self.gl_names:
                mine.update(range(i, i + found[1]))       # its own name ('ball'), not its label ('Stop the cube')
                i += found[1]
                continue
            i += 1
        return bool(mine) and not any(t.w[k] in GL_SHAPES or t.w[k] in ("3d", "view")
                                      for k in range(t.n) if k not in mine)

    def makes_2d_thing(self, t):
        """'make a box called zork at 10 10' with no 3D view named in it: a 2D box, even with a 3D view about."""
        if t.w[0] not in CREATE or any(w in ("3d", "view") for w in t.w[:t.n]):
            return False
        for i in range(1, t.n):
            if t.w[i] in ("in", "on", "into", "inside", "to") and self.gui_element_at(t, i + 1, (GLVIEW,)):
                return False
        return self.read_make(t) is not None

    def read_gl(self, t):
        views = [e for e in self.gui_order if e.kind == GLVIEW]
        if not views:
            return None
        words = t.words
        try:
            import glspace
        except ImportError:
            glspace = None
        if glspace is None or not hasattr(glspace, "read"):
            if set(t.w[:t.n]) & GL_SHAPES:
                raise Problem("3D words (cubes, spheres, spinning) need brick's 3D part (glspace.py), which isn't "
                              "ready yet in this brick.")
            return None
        if self.names_2d_thing(t) or self.makes_2d_thing(t):
            return None                                 # your own 2D things come first: a box called ball
        op = glspace.read(words, self.gl_names)
        if not op:
            return None
        view = views[-1]
        for i in range(1, t.n):
            if t.w[i] in ("in", "on", "into", "inside", "to"):
                found = self.gui_element_at(t, i + 1, (GLVIEW,))
                if found:
                    view = found[0]

        def plan():
            self.gui_use()
            if isinstance(op, dict) and op.get("name"):
                self.gl_names.add(str(op["name"]).lower())
            glspace.emit(self, op, view.view)
            self.a.call("bw_changed")
        return plan

    # ---------------- handlers: "when ..." and "every ..." ----------------

    def gui_event(self, line, block):
        """'when start is clicked, write hi' (or with an indented block under 'when start is clicked:'),
        'when New is chosen from file, ...', 'when the Settings tab is picked, ...', 'when a key is pressed, ...',
        'when the mouse moves, ...', 'every 100 milliseconds, ...', 'when the window is closed, ...'.
        Returns True if it was one (and it's compiled), False if it isn't."""
        if not getattr(self, "gui_on", False):
            return False
        words = split_glued(words_of(line))
        w = [x.word for x in words] + [None] * 6
        if not w or w[0] not in ("when", "whenever", "every", "each", "once", "on", "after", "if"):
            return False
        found = self.gui_every(words, w) if w[0] in ("every", "each") else self.gui_when(words, w)
        if not found:
            return False
        event, rest = found
        if rest and rest[0].word in ("then", "do"):
            rest = rest[1:]
        rest_text = " ".join(x.raw for x in rest).strip()
        if rest_text.startswith((",", ":")):
            rest_text = rest_text[1:].strip()
        if not rest_text and not block:
            raise Problem("What should happen then? Put it after a comma, like: when start is clicked, write hi"
                          ". Or put it on the lines below, each one indented with spaces.")
        self.gui_use()

        def body():
            if rest_text:
                self.compile_line(rest_text, block)
            else:
                block()
        label = self.gui_routine(body, event)
        self.gui_store_handler(event, label)
        return True

    def gui_every(self, words, w):
        t = Toks(words)
        i = 1
        amount = None
        if t.at(i, "half a second", "half second"):
            ms, i = 500, i + t.at(i, "half a second", "half second")
        elif t.at(i, "frame", "tick"):
            ms, i = 16, i + 1
        else:
            number, j = read_number(words, i)
            if number is None:
                number, j = 1, i
            unit = t.w[j]
            scale = {"milliseconds": 1, "millisecond": 1, "ms": 1, "seconds": 1000, "second": 1000, "secs": 1000,
                     "sec": 1000, "s": 1000, "minutes": 60000, "minute": 60000, "mins": 60000, "min": 60000,
                     "frames": 16, "frame": 16, "ticks": 16}.get(unit)
            if scale is None:
                return None
            ms, i = round(number * scale), j + 1
            del amount
        if ms <= 0:
            raise Problem("Every how long? Like: every 100 milliseconds, move ball right 5")
        i += t.at(i, "or so")
        return ("every", ms), words[i:]

    def gui_when(self, words, w):
        t = Toks(words)
        start = 1 + t.at(1, "ever")
        verbs = {"clicked", "pressed", "tapped", "hit", "pushed", "chosen", "picked", "selected", "closed", "closes",
                 "shut", "shuts", "resized", "moves", "moved", "changes", "changed", "typed", "opened", "clicks",
                 "presses", "taps", "chooses", "picks", "selects", "types", "click", "press", "tap", "choose", "pick",
                 "select", "type", "edited", "goes", "touched", "touches"}
        k = next((k for k in range(start, min(t.n, start + 9)) if t.w[k] in verbs), None)
        if k is None:
            if t.at(start, "the mouse", "mouse", "the pointer", "the cursor") and \
                    t.at(start + t.at(start, "the mouse", "mouse", "the pointer", "the cursor"), "is over"):
                return None
            return None
        subject = words[start:k]
        said = [x.word for x in subject]
        while said and said[-1] in ("is", "gets", "was", "are", "has", "been", "get", "being", "it's", "its"):
            said, subject = said[:-1], subject[:-1]
        verb = t.w[k]
        after = k + 1
        # who clicks: "when someone clicks start", "when you click on start"
        if verb in ("clicks", "presses", "taps", "chooses", "picks", "selects", "click", "press", "tap", "choose",
                    "pick", "select", "types", "type", "touches") and \
                (not said or " ".join(said) in PEOPLE or said in (["i"], ["we"])):
            j = after + t.at(after, "on", "the button", "")
            if verb in ("types", "type", "presses", "press", "hits") and t.at(j, "a key", "any key", "a letter",
                                                                              "something", "a character", "keys"):
                return ("key",), words[j + t.at(j, "a key", "any key", "a letter", "something", "a character", "keys"):]
            m = self.gui_subject_end(t, j)
            subject, after = words[j:m], m
            said = [x.word for x in subject]
            verb = {"clicks": "clicked", "click": "clicked", "presses": "pressed", "press": "pressed",
                    "taps": "tapped", "tap": "tapped", "chooses": "chosen", "choose": "chosen", "picks": "picked",
                    "pick": "picked", "selects": "selected", "select": "selected", "touches": "clicked"}.get(verb, verb)
        said_text = " ".join(s for s in said if s)
        after += t.at(after, "on", "with the mouse", "by the user", "by someone")
        if said_text in ("a key", "any key", "the key", "a key on the keyboard", "some key", "keys", "a letter",
                         "anything", "something") and verb in ("pressed", "typed", "hit", "pushed", "goes"):
            after += t.at(after, "down")
            return ("key",), words[after:]
        if said_text in ("the mouse", "mouse", "the pointer", "the cursor", "the mouse pointer") and \
                verb in ("moves", "moved", "goes"):
            return ("mouse",), words[after:]
        if said_text in ("the window", "window", "this window", "my window"):
            if verb in ("closed", "closes", "shut", "shuts"):
                return ("window", "bw_on_close"), words[after:]
            if verb in ("resized", "changes", "changed") and (verb == "resized" or t.at(after, "size", "its size")):
                after += t.at(after, "size", "its size")
                return ("window", "bw_on_resize"), words[after:]
            return None
        # "when New is chosen from file", "when the Settings tab is picked"
        menu = None
        if t.at(after, "from", "in", "on", "off"):
            j = after + t.at(after, "from", "in", "on", "off")
            found = self.gui_element_at(t, j + t.at(j, "the"), (MENU, TABS))
            if found:
                menu = found[0]
                after = j + t.at(j, "the") + found[1] + t.at(j + t.at(j, "the") + found[1], "menu", "tabs")
        if said and said[-1] in ("tab", "page") and verb in ("picked", "chosen", "selected", "clicked", "opened",
                                                             "pressed", "shown"):
            item = self.gui_item(subject[:-1] if said[0] != "the" else subject[1:-1], menu)
            if not item:
                raise Problem(f"I don't know a tab called {' '.join(x.text for x in subject[:-1] if x.word != 'the')}"
                              f" yet. Make the tabs first, like: make tabs called pages with Home, Settings and About")
            return ("item", item[0], item[1]), words[after:]
        if said[:2] in (["tab"], ["the", "tab"]) or said[:1] == ["tab"]:
            item = self.gui_item(subject[1 + (said[0] == "the"):], menu)
            if item:
                return ("item", item[0], item[1]), words[after:]
        if verb in ("changes", "changed", "edited", "typed") or (verb in ("pressed", "hit") and said_text in ("enter", "return")):
            target = subject
            if verb in ("pressed", "hit"):
                j = after + t.at(after, "in", "on", "inside")
                m = self.gui_subject_end(t, j)
                target, after = words[j:m], m
            if [x.word for x in target[:3]] in (["the", "text", "of"], ["the", "words", "in"], ["the", "text", "in"]):
                target = target[3:]
            elem = self.gui_find(target, (INPUT, TABS, TEXTKIND))
            if elem:
                return ("change", elem), words[after:]
            name = " ".join(x.text for x in target if x.word != "the")
            raise Problem(f"I don't know an input box called {name} yet. Make it first, like: make an input box "
                          f"called {name}, then: when {name} changes, write the text of {name}")
        if verb in ("chosen", "picked", "selected", "clicked", "pressed", "tapped") and subject:
            if menu:
                item = self.gui_item(subject, menu)
                if not item:
                    raise Problem(f"{menu.name} has no item called {' '.join(x.text for x in subject)}. Its items: "
                                  + ", ".join(menu.item_names))
                return ("item", item[0], item[1]), words[after:]
            elem = self.gui_find(subject)
            if elem and elem.kind != WINDOW:
                return ("click", elem), words[after:]
            item = self.gui_item(subject)
            if item:
                return ("item", item[0], item[1]), words[after:]
            if verb in ("clicked", "pressed", "tapped") and said and said_text not in ("i", "it", "you"):
                name = " ".join(x.text for x in subject if x.word not in ("the",))
                if self.uses_gui or self.gui_order:
                    raise Problem(f"I don't know a button called {name} yet. Make it first, like: make a button "
                                  f"called {name} saying Start, then: when {name} is clicked, write hi")
        return None

    def gui_subject_end(self, t, j):
        m = j
        while m < t.n and not t.comma(m - 1 if m > j else -1) and not (m > j and self.starts_action(t.words, m)):
            m += 1
            if t.comma(m - 1):
                break
        found = None
        for n in range(min(5, m - j), 0, -1):
            if self.gui_find(t.words[j:j + n]) or self.gui_item(t.words[j:j + n]):
                found = j + n
                break
        return found or m

    def gui_routine(self, body, event):
        a = self.a
        label, skip = a.new_label("bww_handler"), a.new_label("bww_after_handler")
        saved = self.scopes, self.loop_ends
        self.scopes, self.loop_ends = [self.scopes[0]], []
        a.jump(skip)
        a.label(label)
        try:
            body()
        finally:
            self.scopes, self.loop_ends = saved
        a.ret()
        a.label(skip)
        return label

    def gui_store_handler(self, event, label):
        a = self.a
        kind = event[0]
        if kind in ("key", "mouse", "window", "every"):
            target = {"key": "bw_on_key", "mouse": "bw_on_move", "every": "bw_every"}.get(kind) or event[1]
            a.address(RAX, label)
            a.address(RCX, target)
            a.store(RCX, 0, RAX)
            if kind == "every":
                a.set(RAX, event[1])
                a.address(RCX, "bw_every_ms")
                a.store(RCX, 0, RAX)
            return
        if kind == "click":
            self.gui_apply(event[1], [("handler", CLICK, label)])
        elif kind == "change":
            self.gui_apply(event[1], [("handler", CHANGE, label)])
        elif kind == "item":
            elem, item = event[1], event[2]
            a.address(RAX, label)
            a.push(RAX)
            a.address(RAX, elem.items[item])
            a.load(RAX, RAX)
            a.push(RAX)
            self.gui_index(elem)
            a.pop(RCX)
            a.pop(RDX)
            a.call("bw_item_handler")

    # ---------------- values: the mouse x, the key, the x of start ... ----------------

    def gui_value(self, p):
        """For the value reader (expressions.Parser.atom): a window value starting here -> its value tree, or None."""
        start = p.at
        if p.kind_of(p.peek()) or p.long_name(0):
            return None
        the = 1 if p.peek() in ("the", "its") else 0
        if p.kind_of(p.peek(the)) and the:
            return None
        if p.take("the mouse x", "mouse x", "the mouse's x", "the x of the mouse", "the pointer x", "the mouse x position",
                  "the x position of the mouse", "the mouse position x", "where the mouse is across"):
            return ("bw", "mouse", "x")
        if p.take("the mouse y", "mouse y", "the mouse's y", "the y of the mouse", "the pointer y", "the mouse y position",
                  "the y position of the mouse", "the mouse position y", "where the mouse is down"):
            return ("bw", "mouse", "y")
        for axis in ("x", "y"):                         # "the mouse x on board": from the canvas's corner
            for lead in ("the mouse " + axis, "mouse " + axis):
                n = len(lead.split())
                if all(p.peek(k) == part for k, part in enumerate(lead.split())) and \
                        p.peek(n) in ("on", "in", "over", "inside", "within"):
                    for m in range(4, 0, -1):
                        names = p.words[p.at + n + 1:p.at + n + 1 + m]
                        elem = self.gui_find(names) if len(names) == m else None
                        if elem and elem.kind != WINDOW:
                            p.at += n + 1 + m
                            return ("bw", "mouse_in", axis, elem.name.lower())
        if p.take("the key code", "the keycode", "the code of the key"):
            return ("bw", "keycode")
        if p.take("the key", "the key pressed", "the key that was pressed", "the last key", "the pressed key",
                  "which key", "the typed key", "the key typed", "the letter typed", "the letter pressed"):
            return ("bw", "key")
        if p.take("the chosen item", "the chosen", "the picked item", "the selected item", "the item chosen",
                  "the item picked", "the choice", "the chosen tab", "the picked tab", "the selected tab",
                  "the chosen option", "the selected option", "what was chosen", "the item selected"):
            return ("bw", "chosen")
        if p.take("the width of the window", "the window width", "the window's width", "how wide the window is",
                  "the width of window", "window width"):
            return ("bw", "field", "window", AW, "number")
        if p.take("the height of the window", "the window height", "the window's height", "how tall the window is",
                  "the height of window", "window height"):
            return ("bw", "field", "window", AH, "number")
        fields = {"x": (AX, "number"), "left": (AX, "number"), "y": (AY, "number"), "top": (AY, "number"),
                  "width": (AW, "number"), "height": (AH, "number"), "depth": (DEPTH, "number"),
                  "layer": (DEPTH, "number"), "text": (TEXT, "text"), "label": (TEXT, "text"),
                  "words": (TEXT, "text"), "caption": (TEXT, "text"), "content": (TEXT, "text"),
                  "contents": (TEXT, "text"), "value": (TEXT, "text"), "picked": (PICKED, "number")}
        word = p.peek(the)
        if word in fields and p.peek(the + 1) in ("of", "in"):
            for n in range(4, 0, -1):
                names = [p.words[k] for k in range(p.at + the + 2, min(len(p.words), p.at + the + 2 + n))]
                if len(names) != n:
                    continue
                elem = self.gui_find(names)
                if elem:
                    p.at += the + 2 + n
                    if elem.kind == WINDOW and fields[word][0] in (AW, AH, AX, AY):
                        return ("bw", "field", "window", fields[word][0], fields[word][1])
                    return ("bw", "field", elem.name.lower(), fields[word][0], fields[word][1])
        if word and word.endswith("'s"):
            elem = self.gui_find([Word(p.words[p.at + the].text[:-2])])
            field = p.peek(the + 1)
            if elem and field in fields:
                p.at += the + 2
                return ("bw", "field", elem.name.lower(), fields[field][0], fields[field][1])
        p.at = start
        return None

    def emit_gui_value(self, what, *args):
        a = self.a
        self.gui_use()
        if what == "mouse":
            a.address(RAX, "bw_mouse_x" if args[0] == "x" else "bw_mouse_y")
            a.load(RAX, RAX)
            return "number"
        if what == "mouse_in":
            elem = self.gui_names.get(args[1])
            self.gui_record(elem)
            a.load(RCX, RAX, AX if args[0] == "x" else AY)
            a.address(RAX, "bw_mouse_x" if args[0] == "x" else "bw_mouse_y")
            a.load(RAX, RAX)
            a.op("sub", RAX, RCX)
            return "number"
        if what == "keycode":
            a.address(RAX, "bw_key")
            a.load(RAX, RAX)
            return "number"
        if what == "key":
            a.call("bww_key_text")
            return "text"
        if what == "chosen":
            a.address(RAX, "bw_chosen")
            a.load(RAX, RAX)
            self.gui_not_empty()
            return "text"
        if what == "field":
            name, offset, kind = args
            elem = self.gui_window if name == "window" else self.gui_names.get(name)
            if elem is None:
                raise Problem(f"I don't know what '{name}' is yet.")
            if elem.kind == WINDOW:
                a.call("bww_window")
            self.gui_record(elem)
            a.load(RAX, RAX, offset)
            if kind == "text":
                self.gui_not_empty()
            return kind
        raise ValueError(what)

    def gui_not_empty(self):
        """A text field that was never set (0) reads as an empty text."""
        a = self.a
        ok = a.new_label("bww_has_text")
        a.op("test", RAX, RAX)
        a.jump_if(NOT_EQUAL, ok)
        a.address(RAX, a.text(""))
        a.label(ok)

    # ---------------- help ----------------

    def gui_help(self, words):
        said = {w.word for w in words}
        for key in ("canvas", "menu", "tabs", "button", "text", "window"):
            if key in said or (key == "tabs" and "tab" in said):
                return "\n    You can say: " + "  |  ".join(GUI_EXAMPLES[key][:3])
        return "\n    You can say: " + "  |  ".join(GUI_EXAMPLES["window"][:3])


# ---------------- routines the window words add ----------------

def add_routines(a, slots, sizes=None, gl=False, kinds=None, drawn=None, looks=None):
    """bww_window (opens the window the first time it's needed), bww_sleep (waiting with a window open),
    bww_key_text (the last key as text), then the bitwin runtime (bitwin.add_all; kinds: the element kinds
    the program makes, drawn: the shapes it draws on canvases, looks: paint.add_all's; None = not known)."""
    def window_routines(a):
        a.label("bww_window")
        done = a.new_label("bww_open_done")
        a.address(RAX, slots["opened"])
        a.load(RCX, RAX)
        a.op("test", RCX, RCX)
        a.jump_if(NOT_EQUAL, done)
        a.set(RCX, 1)
        a.store(RAX, 0, RCX)
        for slot, default, reg in (("title", None, RAX), ("w", 640, RCX), ("h", 480, RDX)):
            ok = a.new_label("bww_given")
            a.address(reg, slots[slot])
            a.load(reg, reg)
            a.op("test", reg, reg)
            a.jump_if(NOT_EQUAL, ok)
            if default is None:
                a.address(reg, a.text("brick"))
            else:
                a.set(reg, default)
            a.label(ok)
            if reg == RAX:
                a.push(RAX)
            elif reg == RCX:
                a.push(RCX)
        a.pop(RCX)
        a.pop(RAX)
        a.address(R8, slots["resizable"])
        a.load(R8, R8)
        a.call("bw_open")
        a.label(done)
        a.ret()

        a.label("bww_sleep")                    # rax = nanoseconds, like sleep
        a.set(RDX, 0)
        a.set(RCX, 1_000_000)
        a.divide(RCX)
        a.push(RAX)
        a.call("bww_window")
        a.pop(RAX)
        a.call("bw_wait")
        a.ret()

        a.label("bww_key_text")
        a.address(RAX, "bw_key")
        a.load(RAX, RAX)
        for code, name in ((8, "backspace"), (9, "tab"), (10, "enter"), (13, "enter"), (27, "escape"), (1001, "left"),
                           (1002, "right"), (1003, "up"), (1004, "down"), (1005, "home"), (1006, "end"), (1007, "delete")):
            other = a.new_label("bww_key")
            a.op_n("cmp", RAX, code)
            a.jump_if(NOT_EQUAL, other)
            a.address(RAX, a.text(name))
            a.ret()
            a.label(other)
        a.push(RAX)
        a.set(RCX, 16)
        a.call("alloc")
        a.pop(RCX)
        a.set(RDX, 0xFF)
        a.op("and", RCX, RDX)
        a.set(RDX, 1)
        a.store(RAX, 0, RDX)
        a.store(RAX, 8, RCX)
        a.ret()
    a.piece(window_routines, split=True)            # each left out when nothing uses it (Asm.prune)

    try:
        import bitwin
        import paint
        import wayland
    except ImportError:
        bitwin = None
    if bitwin is not None:
        wayland.add_all(a)
        paint.add_all(a, sizes, looks)
        if gl:
            import glspace
            glspace.add_all(a)
        bitwin.add_all(a, kinds, drawn)
    else:
        import os
        if os.environ.get("BRICK_BITWIN_STUB") != "1":
            raise Problem("brick's own windows (bitwin.py) aren't built yet in this brick, so this program can't "
                          "open its window. Programs without window words still work.")
        add_stubs(a)


def add_stubs(a):
    """For tests while bitwin.py doesn't exist yet: every bw_ routine does nothing, so programs still build."""
    for name in ("bw_open", "bw_new", "bw_find", "bw_set_text", "bw_add_item", "bw_item_handler", "bw_front",
                 "bw_back", "bw_in_front_of", "bw_behind", "bw_draw", "bw_clear", "bw_changed", "bw_run", "bw_wait",
                 "bw_wait_click", "bw_close"):
        a.label(name)
        a.ret()
    a.label("bw_elem")
    a.address(RAX, "bw_elements")
    a.ret()
    a.space["bw_elements"] = 512
    for name in ("bw_mouse_x", "bw_mouse_y", "bw_key", "bw_last", "bw_chosen", "bw_on_close", "bw_on_key",
                 "bw_on_move", "bw_on_resize", "bw_every", "bw_every_ms"):
        a.space[name] = 8


def gui_lines_of(text):
    return GUI_HINT.search(text) is not None
