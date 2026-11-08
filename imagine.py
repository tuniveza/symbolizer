"""brick's imagination: it makes things up, looks at them, and works out what they could be.

Imagining (everything it imagines keeps its true recipe, so brick knows what it really drew):
  pictures   polygons, stars, rings, ovals, blobs, spirals, lines, crosses (and crescents for named things),
             in random colours, sizes and places, plain or striped, dotted or checked, alone or together
             (left of, above, one inside another, a few at once, a horizon scene), drawn to PNG with the
             standard library (sight.write_png)
  names      brick imagine apple: apple drawn from what brick knows about it (its universe, WordNet's
             definition, ConceptNet): its shape, colour, parts (stem, leaf, wheels), how wide for how tall
  blends     two symbols mixed: apple + ball -> a round red thing, and what that could be (tomato ...)
  contexts   new ways to fill a context it knows: 'a _ is' filled with apple and pear -> 'a cherry is'

Dreaming (self-supervised seeing): the eye (eye.py) looks at each imagined picture without being told, and
what it reads is compared with the recipe, channel by channel (shape, colour, texture, count, position, size,
corners, points, layout). The tallies are plain text in texts/factory/imagination.txt ("star read as star 12 of
30, as blob 18 of 30"). The shape channel's measures go to texts/factory/imagination-samples.txt, and every
TUNE_EVERY pictures the eye's shape thresholds are tried within their safe ranges (eye.TUNABLE); a value that
reads the drawn shapes clearly better, and no one shape much worse, is written to texts/factory/
eye-settings.txt, which the eye reads. Delete that file to go back to the eye's defaults.

Associating ("what could it be?"): the imagined thing's symbols (round, red, striped ...) are matched against
  the neurons      (texts/universes: a neuron that links round and red),
  ConceptNet       (texts/sources/conceptnet.tsv: RelatedTo, HasA, IsA, PartOf, MadeOf, UsedFor, SimilarTo,
                   both ways, by their weights),
  WordNet          (the definitions: apple is "fruit with red or yellow or green skin ..."),
  the dictionary   (colours named after things: tomato, salmon, lemon).
Each symbol counts by how specific it is (1 / (1 + floor(log2 b)), b = 1 + how many things ConceptNet links
to it, as in brain.py), each source by how much it knows (a link's strength or weight), and a thing whose
known colour or shape is something else counts less. -> "apple 0.71: round (shape) + red (colour); ConceptNet
RelatedTo red 9.3, round 4.1; WordNet 'fruit with red or yellow or green skin'".
Strong, well-supported answers (two sources, or one about a well-known thing; two symbols matched exactly)
are learned into the neurons with source "imagination", at IMAGINED (0.3) of
the strength of being told, with the reason as a second source. A link you wrote by hand is never touched,
and a facet you wrote by hand (colour: green) gets nothing new. Imagined links fade like every other.

When it happens:
  while brick runs   dream_round(budget): the factory calls it every round (a few pictures, well under a second,
                     no vision model)
  brick learn ...    react(recent, source): a few variations or blends of what was just learned
  you ask            brick imagine [N] | brick imagine NAME | brick imagine apple + ball | brick dream [N] |
                     brick imaginations [own | mine | learned | today] [N]   (--open opens the picture)
Everything it imagined is in texts/imagination/: the pictures (at most MAX_PICTURES; when there are more, the
oldest ones it imagined on its own go first) and log.txt, one line each: when, where it came from (on its own,
from you, after learning ...), the picture, the recipe, what the eye saw, and what it could be.
Nothing is sent off this computer.
"""
import math
import os
import random
import re
import shutil
import subprocess
import sys
import textwrap
import time
from collections import Counter

import symbols

HERE = os.path.dirname(os.path.abspath(__file__))
FACTORY = os.path.join(HERE, "texts", "factory")
TALLY = os.path.join(FACTORY, "imagination.txt")
SAMPLES = os.path.join(FACTORY, "imagination-samples.txt")
PICTURES = os.path.join(HERE, "texts", "imagination")

SIZE = 128                  # pixels across an imagined picture (the eye looks at up to 160)
MAX_PICTURES = 60           # pictures kept in texts/imagination
MAX_LOG = 2000              # lines kept in texts/imagination/log.txt
MAX_SAMPLES = 400           # shape measures kept for tuning
PER_ROUND = 3               # pictures per dream round
ROUND_BUDGET = 0.5          # seconds per dream round
LINKS_PER_ROUND = 6         # links learned per round (or per picture you ask for), at most
IMAGINED = 0.3              # how strongly imagination teaches: being told is 1, a guess from a picture 0.5
LEARN_AT = 0.45             # an answer must score this, with two symbols supported, to be learned
TUNE_EVERY = 25             # pictures between tries at tuning the eye
TUNE_FROM = 60              # shape samples needed before tuning
FADE_AT = 2000              # a channel's counts halve when they pass this, so recent dreams count most
KEPT_NOTES = 20             # 'learned:' and 'tuned:' lines kept in the tally
ORIGINS = {"own": "on its own", "you": "from you", "learned": "after learning"}
CHANNELS = ("shape", "colour", "texture", "count", "position", "size", "corners", "points", "layout", "names")

BASE = {"red": (215, 30, 35), "orange": (240, 140, 20), "yellow": (240, 215, 30), "green": (40, 160, 60),
        "cyan": (30, 200, 210), "blue": (35, 80, 210), "purple": (120, 50, 180), "pink": (245, 160, 190),
        "magenta": (220, 30, 170), "brown": (120, 70, 30), "grey": (128, 128, 128), "black": (20, 20, 20),
        "white": (250, 250, 250)}
DRAWN = ("red", "orange", "yellow", "green", "cyan", "blue", "purple", "pink", "magenta", "brown", "grey", "black")
SHAPES = ("circle", "oval", "ring", "triangle", "square", "rectangle", "pentagon", "hexagon", "star", "blob",
          "spiral", "line", "cross")
EYE_WORD = {"circle": "round", "oval": "oval", "ring": "ring", "triangle": "triangle", "square": "square",
            "rectangle": "rectangle", "pentagon": "pentagon", "hexagon": "hexagon", "star": "star", "line": "long"}
FROM_EYE = {"round": "circle", "roundish": "circle", "oval": "oval", "ring": "ring", "triangle": "triangle",
            "square": "square", "rectangle": "rectangle", "pentagon": "pentagon", "hexagon": "hexagon",
            "star": "star", "long": "line", "blob": "blob"}
CORNERS = {"triangle": 3, "square": 4, "rectangle": 4, "pentagon": 5, "hexagon": 6}
TEXTURES = ("plain", "striped", "dotted", "checked")
COUNT = {1: "one thing", 2: "two things", 3: "three things"}
POSITIONS = [["top-left", "top", "top-right"], ["left", "middle", "right"], ["bottom-left", "bottom", "bottom-right"]]
# what each imagined symbol is called in the knowledge, with how well each word stands for it
FEATURES = {
    "circle": ("shape", {"round": 1.0, "circle": 0.8, "circular": 0.8, "sphere": 0.6, "spherical": 0.6}),
    "oval": ("shape", {"oval": 1.0, "elliptical": 0.8, "oblong": 0.6, "egg shaped": 0.8}),
    "ring": ("shape", {"ring": 1.0, "hoop": 0.7, "round": 0.5}),
    "triangle": ("shape", {"triangle": 1.0, "triangular": 1.0, "pointed": 0.5}),
    "square": ("shape", {"square": 1.0, "cube": 0.5}),
    "rectangle": ("shape", {"rectangular": 1.0, "rectangle": 1.0, "flat": 0.4}),
    "pentagon": ("shape", {"pentagon": 1.0, "pentagonal": 1.0}),
    "hexagon": ("shape", {"hexagon": 1.0, "hexagonal": 1.0}),
    "star": ("shape", {"star": 1.0, "star shaped": 1.0, "pointed": 0.6}),
    "blob": ("shape", {"blob": 1.0, "irregular": 0.6, "lumpy": 0.6}),
    "spiral": ("shape", {"spiral": 1.0, "coil": 0.7, "coiled": 0.7, "swirl": 0.6}),
    "line": ("shape", {"long": 1.0, "thin": 0.7, "straight": 0.6, "stick": 0.5}),
    "cross": ("shape", {"cross": 1.0, "plus": 0.5}),
    "crescent": ("shape", {"crescent": 1.0, "curved": 0.8}),
    "striped": ("pattern", {"striped": 1.0, "stripes": 1.0, "stripe": 0.8}),
    "dotted": ("pattern", {"spotted": 1.0, "spots": 1.0, "dotted": 1.0, "dots": 0.8}),
    "checked": ("pattern", {"checkered": 1.0, "chequered": 1.0, "checked": 0.8}),
    "scene": ("scene", {"sky": 1.0, "landscape": 0.8, "horizon": 0.8}),
    "green ground": ("scene", {"grass": 1.0, "field": 0.8, "meadow": 0.8, "lawn": 0.6}),
    "brown ground": ("scene", {"earth": 1.0, "soil": 0.8, "dirt": 0.8, "desert": 0.5}),
    "big": ("size", {"big": 1.0, "large": 1.0, "huge": 0.6}),
    "small": ("size", {"small": 1.0, "little": 0.8, "tiny": 0.6})}
FACET_WEIGHT = {"shape": 1.5, "colour": 1.5, "pattern": 1.2, "scene": 1.0, "size": 0.4}
PROPERTY = {w for _, words in FEATURES.values() for w in words} | {"color", "colour", "shape", "size", "texture",
                                                                  "pattern", "thing", "picture", "image"}
# a word for a shape, as brick can draw it
DRAWABLE = {"round": "circle", "circular": "circle", "circle": "circle", "sphere": "circle", "spherical": "circle",
            "ball": "circle", "globe": "circle", "globular": "circle", "disc": "circle", "disk": "circle",
            "roundish": "circle", "oval": "oval", "elliptical": "oval", "oblong": "oval", "egg": "oval",
            "ovoid": "oval", "long": "long", "elongated": "long", "cylindrical": "long", "cylinder": "long",
            "straight": "long", "slender": "long", "crescent": "crescent", "curved": "crescent",
            "triangle": "triangle", "triangular": "triangle", "pointed": "triangle", "cone": "triangle",
            "conical": "triangle", "square": "square", "cube": "square", "rectangular": "rectangle",
            "rectangle": "rectangle", "flat": "rectangle", "box": "rectangle", "ring": "ring", "hoop": "ring",
            "star": "star", "spiral": "spiral", "coiled": "spiral", "hexagonal": "hexagon", "hexagon": "hexagon",
            "pentagon": "pentagon"}
PATTERN_WORDS = {"striped": "striped", "stripes": "striped", "stripy": "striped", "spotted": "dotted",
                 "spots": "dotted", "dotted": "dotted", "speckled": "dotted", "checkered": "checked",
                 "chequered": "checked", "checked": "checked"}
PARTS = {"stem": "stem", "stalk": "stem", "leaf": "leaf", "leaves": "leaf", "wheel": "wheels", "wheels": "wheels"}
CN_RELATIONS = {"RelatedTo", "HasA", "IsA", "PartOf", "MadeOf", "UsedFor", "SimilarTo", "HasProperty", "Synonym"}
now = time.time
_queue = []                 # what brick learned while watching folders, for the next dream round to react to
_made = [0]


def log_path():
    return os.path.join(PICTURES, "log.txt")


# ---------------- colours ----------------

def colour_name(rgb):
    import sight
    return sight.colour_name(*rgb[:3])


def some_colour(rng, name):
    """A colour the eye calls name, a little different each time."""
    base = BASE[name]
    for _ in range(40):
        c = tuple(max(0, min(255, int(v + rng.uniform(-35, 35)))) for v in base)
        if colour_name(c) == name:
            return c
    return base


def rgb_for(word):
    """A colour word -> (r, g, b), or None: brick's colour names, the dictionary's finer ones, 'dark red'."""
    word = symbols.tidy(word)
    if word in BASE:
        return BASE[word]
    if word == "gray":
        return BASE["grey"]
    try:
        import sight
        found = sight.palette().get(word)
    except Exception:
        found = None
    if found:
        return tuple(found)
    shade = re.fullmatch(r"(light|pale|dark|deep|bright) (.+)", word)
    if shade and rgb_for(shade.group(2)):
        r, g, b = rgb_for(shade.group(2))
        if shade.group(1) in ("light", "pale"):
            return tuple(int(v + (255 - v) * 0.45) for v in (r, g, b))
        if shade.group(1) in ("dark", "deep"):
            return tuple(int(v * 0.55) for v in (r, g, b))
        return (r, g, b)
    return None


def colour_thing(word):
    """Is this a colour named after a thing (tomato, salmon, lemon)? The dictionary knows."""
    try:
        import dictionary
        found = dictionary.colour_table().get(word)
    except Exception:
        return False
    return bool(found and found[1])


# ---------------- drawing ----------------

def rotate(points, angle):
    c, s = math.cos(angle), math.sin(angle)
    return [(u * c - v * s, u * s + v * c) for u, v in points]


def ellipse(a, b, n=96, dx=0.0, dy=0.0):
    return [(dx + a * math.cos(2 * math.pi * k / n), dy + b * math.sin(2 * math.pi * k / n)) for k in range(n)]


def outline(t):
    """A thing's outline around its centre, at radius r: (polygons to fill, polygons to cut out, discs to stamp)."""
    shape, r, ratio, angle = t["shape"], t["r"], t.get("ratio", 1.0), math.radians(t.get("angle", 0))
    add, cut, stamps = [], [], []
    if shape == "circle":
        add.append(ellipse(r, r))
    elif shape == "oval":
        add.append(rotate(ellipse(r, r / ratio), angle))
    elif shape == "ring":
        add.append(ellipse(r, r))
        cut.append(ellipse(r * t.get("inner", 0.55), r * t.get("inner", 0.55)))
    elif shape == "crescent":
        add.append(rotate(ellipse(r, r), angle))
        cut.append(rotate(ellipse(0.85 * r, 0.85 * r, dx=0.45 * r, dy=-0.15 * r), angle))
    elif shape in ("triangle", "pentagon", "hexagon"):
        n = CORNERS[shape]
        add.append(rotate([(r * math.cos(2 * math.pi * k / n - math.pi / 2),
                            r * math.sin(2 * math.pi * k / n - math.pi / 2)) for k in range(n)], angle))
    elif shape == "square":
        h = r / math.sqrt(2)
        add.append(rotate([(-h, -h), (h, -h), (h, h), (-h, h)], angle))
    elif shape in ("rectangle", "line"):
        b = r / ratio
        add.append(rotate([(-r, -b), (r, -b), (r, b), (-r, b)], angle))
    elif shape == "cross":
        b = r / 4
        add.append(rotate([(-r, -b), (r, -b), (r, b), (-r, b)], angle))
        add.append(rotate([(-b, -r), (b, -r), (b, r), (-b, r)], angle))
    elif shape == "star":
        p, inner = t.get("points", 5), t.get("inner", 0.45)
        add.append(rotate([((r if k % 2 == 0 else r * inner) * math.cos(math.pi * k / p - math.pi / 2),
                            (r if k % 2 == 0 else r * inner) * math.sin(math.pi * k / p - math.pi / 2))
                           for k in range(2 * p)], angle))
    elif shape == "blob":
        waves = t.get("waves", [(2, 0.15, 0.0), (3, 0.1, 1.0)])
        points = []
        for k in range(64):
            a = 2 * math.pi * k / 64
            rr = r * (1 + sum(amp * math.sin(n * a + phase) for n, amp, phase in waves)) / (1 + sum(w[1] for w in waves))
            points.append((rr * math.cos(a), rr * math.sin(a)))
        add.append(points)
    elif shape == "spiral":
        turns, thick = t.get("turns", 2.5), max(2.0, 0.1 * r)
        for k in range(0, 361):
            f = k / 360
            a = f * turns * 2 * math.pi + angle
            stamps.append(((0.12 + 0.88 * f) * (r - thick) * math.cos(a), (0.12 + 0.88 * f) * (r - thick) * math.sin(a),
                           thick))
    return add, cut, stamps


def fill_polygon(poly, cx, cy, w, h, out):
    """Every pixel whose middle is inside the polygon (moved to cx, cy) -> added to out (scanlines)."""
    pts = [(cx + x, cy + y) for x, y in poly]
    ys = [p[1] for p in pts]
    for y in range(max(0, int(min(ys))), min(h - 1, int(max(ys)) + 1) + 1):
        yc = y + 0.5
        xs = []
        for i in range(len(pts)):
            (ax, ay), (bx, by) = pts[i - 1], pts[i]
            if (ay <= yc < by) or (by <= yc < ay):
                xs.append(ax + (yc - ay) * (bx - ax) / (by - ay))
        xs.sort()
        for k in range(0, len(xs) - 1, 2):
            for x in range(max(0, math.ceil(xs[k] - 0.5)), min(w - 1, math.floor(xs[k + 1] - 0.5)) + 1):
                out.add(y * w + x)


def mask(t, w, h):
    """The pixels a thing covers -> set of indices."""
    add, cut, stamps = outline(t)
    inside = set()
    for poly in add:
        fill_polygon(poly, t["cx"], t["cy"], w, h, inside)
    for poly in cut:
        hole = set()
        fill_polygon(poly, t["cx"], t["cy"], w, h, hole)
        inside -= hole
    for x, y, rad in stamps:
        x, y = t["cx"] + x, t["cy"] + y
        for py in range(max(0, int(y - rad)), min(h, int(y + rad) + 2)):
            for px in range(max(0, int(x - rad)), min(w, int(x + rad) + 2)):
                if (px + 0.5 - x) ** 2 + (py + 0.5 - y) ** 2 <= rad * rad:
                    inside.add(py * w + px)
    return inside


def reach(t):
    """How far the thing reaches from its centre: (left, right, up, down)."""
    add, _, stamps = outline(t)
    points = [p for poly in add for p in poly] + [(x + s * r, y + s * r) for x, y, r in stamps for s in (-1, 1)]
    if not points:
        return t["r"], t["r"], t["r"], t["r"]
    return (-min(p[0] for p in points), max(p[0] for p in points), -min(p[1] for p in points),
            max(p[1] for p in points))


def unit_area(t):
    """The thing's area at radius 1 (to size it by how much of the picture it should fill)."""
    if t["shape"] == "spiral":
        return 2.2
    add, cut, _ = outline(dict(t, r=1.0))

    def area(poly):
        return abs(sum(poly[i][0] * poly[i - 1][1] - poly[i - 1][0] * poly[i][1] for i in range(len(poly)))) / 2
    return max(0.05, sum(area(p) for p in add) - sum(area(p) for p in cut))


def place(t, w, h, cell=None, rng=None, share=None):
    """Size the thing (share of the picture) and put it in a cell of the 3 by 3 grid, inside the picture."""
    rng = rng or random
    if share is not None:
        t["r"] = math.sqrt(share * w * h / unit_area(t))
    col, row = cell if cell else (1, 1)
    for _ in range(8):
        left, right, up, down = reach(t)
        if left + right <= w - 6 and up + down <= h - 6:
            break
        t["r"] *= 0.85
    left, right, up, down = reach(t)
    cx = (col + 0.5) * w / 3 + rng.uniform(-w / 14, w / 14)
    cy = (row + 0.5) * h / 3 + rng.uniform(-h / 14, h / 14)
    t["cx"] = min(max(cx, left + 3), w - 3 - right)
    t["cy"] = min(max(cy, up + 3), h - 3 - down)
    return t


def paint(t, x, y):
    texture, p = t.get("texture", "plain"), t.get("period", 8)
    if texture == "plain":
        return t["rgb"]
    if texture == "striped":
        on = ((y if t.get("way", "horizontal") == "horizontal" else x) // p) % 2
    elif texture == "checked":
        on = ((x // p) + (y // p)) % 2
    else:
        dx, dy = x % p - p / 2 + 0.5, y % p - p / 2 + 0.5
        on = dx * dx + dy * dy <= (0.3 * p) ** 2
    return t["second rgb"] if on else t["rgb"]


def draw(recipe):
    """Draw a recipe -> (width, height, [(r, g, b)]); fills in each thing's area, position and size as drawn."""
    w, h = recipe["width"], recipe["height"]
    pixels = [recipe["background rgb"]] * (w * h)
    scene = recipe.get("scene")
    if scene:
        line = int(scene["at"] * h)
        sky, ground = scene["sky rgb"], scene["ground rgb"]
        pixels = [tuple(int(c * (0.88 + 0.12 * y / max(1, line))) for c in sky) if y < line else ground
                  for y in range(h) for _ in range(w)]
    owner = [-1] * (w * h)
    for i, t in enumerate(recipe["things"]):
        for k in mask(t, w, h):
            pixels[k] = paint(t, k % w, k // w)
            owner[k] = i
    for i, t in enumerate(recipe["things"]):
        mine = [k for k in range(w * h) if owner[k] == i]
        t["area"] = len(mine)
        if mine:
            cx = sum(k % w for k in mine) / len(mine)
            cy = sum(k // w for k in mine) / len(mine)
            t["position"] = POSITIONS[min(2, int(3 * cy / h))][min(2, int(3 * cx / w))]
            share = len(mine) / (w * h)
            t["size"] = "big" if share >= 0.3 else "medium-sized" if share >= 0.06 else "small"
    return w, h, pixels


# ---------------- recipes ----------------

def new_thing(rng, shape=None, colour=None, texture=None, avoid=()):
    shape = shape or rng.choice(SHAPES)
    colour = colour or rng.choice([c for c in DRAWN if c not in avoid] or DRAWN)
    t = {"shape": shape, "colour": colour, "rgb": some_colour(rng, colour), "angle": rng.uniform(0, 360),
         "texture": texture or rng.choices(TEXTURES, weights=[64, 14, 11, 11])[0], "r": 20.0}
    if shape in ("triangle", "pentagon", "hexagon", "square"):
        t["angle"] = rng.choice([0, 0, rng.uniform(0, 360)])
    if shape == "oval":
        t["ratio"] = rng.uniform(1.5, 2.4)
    elif shape == "rectangle":
        t["ratio"], t["angle"] = rng.uniform(1.6, 2.4), rng.choice([0, 90, rng.uniform(0, 180)])
    elif shape == "line":
        t["ratio"] = rng.uniform(5, 8)
    elif shape == "ring":
        t["inner"] = rng.uniform(0.45, 0.65)
    elif shape == "star":
        t["points"], t["inner"] = rng.randint(5, 8), rng.uniform(0.38, 0.5)
    elif shape == "blob":
        t["waves"] = [(n, rng.uniform(0.05, 0.22), rng.uniform(0, 6.3)) for n in rng.sample([2, 3, 4, 5], 2)]
    elif shape == "spiral":
        t["turns"] = rng.uniform(2, 3.2)
    if shape in CORNERS:
        t["corners"] = CORNERS[shape]
    if t["texture"] != "plain":
        t["second"] = rng.choice([c for c in DRAWN + ("white",) if c not in (colour,) + tuple(avoid)])
        t["second rgb"] = some_colour(rng, t["second"])
        t["period"] = rng.randint(6, 10)
        t["way"] = rng.choice(["horizontal", "vertical"])
    return t


def random_recipe(rng=None, size=SIZE):
    """A random picture: what's in it and where, before it's drawn."""
    rng = rng or random
    kind = rng.choices(["one", "pair", "inside", "several", "scene"], weights=[50, 16, 10, 12, 12])[0]
    back = rng.choices(["white", "pale", "colour"], weights=[70, 15, 15])[0]
    if back == "white":
        background, back_rgb = "white", (255, 255, 255)
    elif back == "pale":
        background, back_rgb = "white", rng.choice([(245, 240, 228), (240, 240, 240), (236, 242, 248)])
    else:
        background = rng.choice(DRAWN)
        back_rgb = some_colour(rng, background)
    avoid = (background,) if background != "white" else ("white",)
    recipe = {"kind": kind, "width": size, "height": size, "background": background, "background rgb": back_rgb,
              "things": [], "scene": None}
    if kind == "one":
        share = rng.choice([rng.uniform(0.025, 0.05), rng.uniform(0.08, 0.22), rng.uniform(0.32, 0.42)])
        cell = (1, 1) if share > 0.3 else (rng.randint(0, 2), rng.randint(0, 2))
        recipe["things"].append(place(new_thing(rng, avoid=avoid), size, size, cell, rng, share))
    elif kind == "pair":
        way = rng.choice(["left of", "above"])
        a = new_thing(rng, avoid=avoid, texture="plain")
        b = new_thing(rng, avoid=avoid + (a["colour"],), texture="plain")
        cells = [(0, 1), (2, 1)] if way == "left of" else [(1, 0), (1, 2)]
        for t, cell in zip((a, b), cells):
            recipe["things"].append(place(t, size, size, cell, rng, rng.uniform(0.04, 0.09)))
        recipe["way"] = way
    elif kind == "inside":
        outer = new_thing(rng, shape=rng.choice(["circle", "square", "hexagon", "pentagon"]), avoid=avoid,
                          texture="plain")
        place(outer, size, size, (1, 1), rng, rng.uniform(0.3, 0.4))
        inner = new_thing(rng, shape=rng.choice(["circle", "square", "triangle", "star"]),
                          avoid=avoid + (outer["colour"],), texture="plain")
        inner["r"] = math.sqrt(0.035 * size * size / unit_area(inner))
        inner["cx"], inner["cy"] = outer["cx"], outer["cy"]
        recipe["things"] += [outer, inner]
    elif kind == "several":
        cells = rng.sample([(c, r) for c in range(3) for r in range(3)], 3)
        used = list(avoid)
        for cell in cells:
            t = new_thing(rng, avoid=tuple(used), texture="plain")
            used.append(t["colour"])
            recipe["things"].append(place(t, size, size, cell, rng, rng.uniform(0.025, 0.045)))
    else:
        at = rng.uniform(0.35, 0.65)
        sky = rng.choice([(110, 170, 230), (90, 150, 225), (150, 195, 240)])
        ground_name = rng.choice(["green", "brown"])
        ground = (60, 150, 60) if ground_name == "green" else (130, 90, 50)
        recipe["scene"] = {"at": at, "sky": "blue", "ground": ground_name, "sky rgb": sky, "ground rgb": ground,
                           "horizon": "high" if at < 0.4 else "low" if at > 0.6 else "in the middle"}
        recipe["background"] = "blue"
        if rng.random() < 0.5:
            sun = new_thing(rng, shape="circle", colour="yellow", texture="plain")
            recipe["things"].append(place(sun, size, size, (rng.choice([0, 2]), 0), rng, 0.02))
    return recipe


def recipe_from_traits(found, title, rng=None, size=SIZE):
    """A picture of one thing from what is known about it: {shape, colour, pattern, parts, ratio}."""
    rng = rng or random.Random(title)
    shape_word = found.get("shape") or ""
    shape = DRAWABLE.get(shape_word, "blob")
    t = {"shape": "oval" if shape == "long" else shape, "angle": 0.0, "texture": found.get("pattern") or "plain",
         "colour": found.get("colour") or "grey", "rgb": rgb_for(found.get("colour") or "grey") or BASE["grey"],
         "r": 20.0}
    ratio = found.get("ratio")
    if shape == "long":
        t["ratio"] = max(2.6, ratio or 3.0)
    elif shape == "oval":
        t["ratio"] = max(1.3, ratio if ratio and ratio > 1 else 1.6)
    elif shape == "rectangle":
        t["ratio"] = max(1.5, ratio or 1.8)
    elif shape == "circle" and ratio and abs(ratio - 1) > 0.12:
        t["shape"], t["ratio"] = "oval", max(ratio, 1 / ratio)
    if ratio and ratio < 1 and t["shape"] in ("oval", "rectangle"):
        t["angle"] = 90.0                                  # taller than wide: stand it up
    if t["shape"] == "star":
        t["points"], t["inner"] = 5, 0.45
    if t["shape"] == "ring":
        t["inner"] = 0.55
    if t["shape"] == "blob":
        t["waves"] = [(3, 0.12, 0.5), (2, 0.08, 1.7)]
    if t["texture"] != "plain":
        t["second"] = "white" if t["colour"] != "white" else "black"
        t["second rgb"], t["period"], t["way"] = BASE[t["second"]], 8, "horizontal"
    if t["shape"] in CORNERS:
        t["corners"] = CORNERS[t["shape"]]
    pale = t["colour"] in ("white", "grey") or sum(t["rgb"]) > 640       # a white thing on pale blue, not white
    recipe = {"kind": "named", "width": size, "height": size, "background": "blue" if pale else "white",
              "background rgb": (170, 200, 235) if pale else (255, 255, 255), "things": [], "scene": None,
              "title": title,
              "parts": list(found.get("parts") or [])}
    place(t, size, size, (1, 1), rng, 0.2)
    t["cx"], t["cy"] = size / 2, size / 2 + (4 if "stem" in recipe["parts"] else 0)
    recipe["things"].append(t)
    left, right, up, down = reach(t)
    if "stem" in recipe["parts"]:
        stem = {"shape": "rectangle", "ratio": 4.0, "angle": 80.0, "r": max(4.0, 0.3 * up), "colour": "brown",
                "rgb": (100, 60, 25), "texture": "plain"}
        stem["cx"], stem["cy"] = t["cx"] + 1, t["cy"] - up - stem["r"] * 0.55
        recipe["things"].append(stem)
        if "leaf" in recipe["parts"]:
            leaf = {"shape": "oval", "ratio": 2.2, "angle": -25.0, "r": max(5.0, 0.32 * up), "colour": "green",
                    "rgb": (50, 150, 50), "texture": "plain"}
            leaf["cx"], leaf["cy"] = stem["cx"] + leaf["r"] * 0.95, stem["cy"] - 1
            recipe["things"].append(leaf)
    elif "leaf" in recipe["parts"]:
        leaf = {"shape": "oval", "ratio": 2.2, "angle": -25.0, "r": max(5.0, 0.35 * up), "colour": "green",
                "rgb": (50, 150, 50), "texture": "plain"}
        leaf["cx"], leaf["cy"] = t["cx"] + leaf["r"] * 0.6, t["cy"] - up - leaf["r"] * 0.2
        recipe["things"].append(leaf)
    if "wheels" in recipe["parts"]:
        for side in (-1, 1):
            wheel = {"shape": "circle", "r": max(4.0, 0.22 * (left + right)), "colour": "black", "rgb": BASE["black"],
                     "texture": "plain"}
            wheel["cx"], wheel["cy"] = t["cx"] + side * 0.55 * right, t["cy"] + down
            recipe["things"].append(wheel)
    return recipe


def thing_words(t):
    texture = "" if t.get("texture", "plain") == "plain" else f"{t['texture']} "
    extra = f" ({t['points']} points)" if t["shape"] == "star" else ""
    return f"{t['colour']} {texture}{t['shape']}{extra}"


def describe(recipe):
    """A recipe in words: 'red striped star (5 points), medium-sized, at the top-left, on white'."""
    things = recipe["things"]
    if recipe.get("title"):
        parts = f" with {', '.join(recipe['parts'])}" if recipe.get("parts") else ""
        return f"{recipe['title']}: {thing_words(things[0])}{parts}"
    if recipe.get("scene"):
        s = recipe["scene"]
        sun = ", a yellow sun" if things else ""
        return f"a scene: sky above {s['ground']} ground, horizon {s['horizon']}{sun}"
    if recipe["kind"] == "pair":
        return f"{thing_words(things[0])} {recipe['way']} {thing_words(things[1])}, on {recipe['background']}"
    if recipe["kind"] == "inside":
        return f"{thing_words(things[1])} inside {thing_words(things[0])}, on {recipe['background']}"
    if recipe["kind"] == "several":
        return ", ".join(f"{thing_words(t)} at the {t.get('position', '?')}" for t in things) + \
            f", on {recipe['background']}"
    t = things[0]
    return f"{thing_words(t)}, {t.get('size', '?')}, at the {t.get('position', '?')}, on {recipe['background']}"


def main_thing(recipe):
    things = [t for t in recipe["things"] if t.get("area")]
    return max(things, key=lambda t: t["area"]) if things else None


def truths(recipe):
    """What is really in the picture, per channel, in the eye's words where the eye has a word."""
    if recipe.get("scene"):
        return {"layout": "sky above ground"}
    main = main_thing(recipe)
    if main is None:
        return {}
    out = {"shape": main["shape"], "colour": main["colour"], "texture": main.get("texture", "plain"),
           "position": main.get("position", "?"), "size": main.get("size", "?"), "layout": "no scene"}
    if recipe["kind"] != "named":
        out["count"] = COUNT.get(len(recipe["things"]), "many things")
        if recipe["kind"] == "inside":
            out["count"] = "two things, one inside the other"
    if main["shape"] in CORNERS:
        out["corners"] = f"{CORNERS[main['shape']]} corners"
    if main["shape"] == "star":
        out["points"] = f"{main['points']}-pointed"
    return out


# ---------------- the eye reading it back ----------------

def eye_reads(found, recipe=None):
    """What the eye saw, per channel (the same channels as truths())."""
    facets, things = found["facets"], found["things"]
    first = (facets.get("objects") or [("nothing", 0)])[0][0]
    out = {"count": first if first in COUNT.values() or first in ("many things", "no single subject") else "?"}
    if things:
        t = things[0]
        out.update(shape=t.shape, colour=t.colour, position=t.position)
        out["size"] = next((v for v, _ in facets.get("size", []) if v in ("big", "medium-sized", "small")), "?")
        out["measures"] = getattr(t, "measures", None)
    else:
        out.update(shape="no single thing", colour="no single thing", position="nowhere", size="?")
    pattern = [v for v, _ in facets.get("pattern", [])]
    out["texture"] = next((p for p in ("striped", "dotted", "checked") if p in pattern), "plain")
    maths = [v for v, _ in facets.get("mathematics", [])]
    out["corners"] = next((v for v in maths if v.endswith(" corners")), "no corners")
    out["points"] = next((v for v, _ in facets.get("shape", []) if v.endswith("-pointed")), "no points")
    contexts = found.get("contexts") or []
    out["layout"] = "sky above ground" if "sky above ground" in contexts else \
        "under the sky" if "under the sky" in contexts else "no scene"
    if recipe:
        main = main_thing(recipe)
        if main and main.get("texture", "plain") != "plain" and out["colour"] == main.get("second"):
            out["colour"] = main["colour"]          # either colour of a patterned thing is its colour
    return out


ALSO_RIGHT = {"line": {"rectangle"}}         # a long thin bar is a rectangle too


def right(channel, truth, seen):
    if channel == "shape":
        return EYE_WORD.get(truth) == seen or seen in ALSO_RIGHT.get(truth, ())
    return truth == seen


def has_word(channel, truth):
    """Can the eye say this at all? (it has no word for a spiral, a cross or a blob it should call a blob)"""
    if channel == "shape":
        return truth in EYE_WORD
    return not truth.startswith("two things, one inside")


def said(reads):
    """What the eye saw, in a few words."""
    if reads.get("shape") == "no single thing":
        return f"no single thing; {reads.get('layout', '')}".strip("; ")
    extra = f" {reads['texture']}" if reads.get("texture", "plain") != "plain" else ""
    scene = f", {reads['layout']}" if reads.get("layout", "no scene") != "no scene" else ""
    return f"{reads['colour']}{extra} {reads['shape']}, {reads['count']}, at the {reads['position']}{scene}"


# ---------------- the tally: how well the eye reads what brick imagines ----------------

class Tally:
    def __init__(self):
        self.counts = {}             # channel -> truth -> Counter(what the eye read)
        self.totals = Counter()      # pictures, names, blends, contexts, dreams
        self.notes = {"tuned": [], "learned": []}
        try:
            with open(TALLY, encoding="utf-8", errors="replace") as f:
                lines = f.read().splitlines()
        except OSError:
            lines = []
        for line in lines:
            total = re.match(r"^(pictures|names|blends|contexts|dreams): (\d+)$", line)
            if total:
                self.totals[total.group(1)] = int(total.group(2))
                continue
            note = re.match(r"^(tuned|learned): (.+)$", line)
            if note:
                self.notes[note.group(1)].append(note.group(2))
                continue
            read = re.match(r"^(\w+): (.+?) read as (.+)$", line)
            if read and read.group(1) in CHANNELS:
                seen = self.counts.setdefault(read.group(1), {}).setdefault(read.group(2), Counter())
                for part in read.group(3).split(", as "):
                    each = re.match(r"^(.+) (\d+) of \d+$", part.strip())
                    if each:
                        seen[each.group(1)] += int(each.group(2))

    def add(self, channel, truth, seen):
        self.counts.setdefault(channel, {}).setdefault(truth, Counter())[seen] += 1

    def accuracy(self, channel):
        """-> (right, out of): only for what the eye has a word for."""
        good = total = 0
        for truth, seen in self.counts.get(channel, {}).items():
            if has_word(channel, truth):
                total += sum(seen.values())
                good += sum(n for s, n in seen.items() if right(channel, truth, s))
        return good, total

    def note(self, kind, text):
        self.notes[kind] = ([f"{time.strftime('%Y-%m-%d %H:%M')} {text}"] + self.notes[kind])[:KEPT_NOTES]

    def fade(self):
        for channel, truths_seen in self.counts.items():
            if sum(sum(c.values()) for c in truths_seen.values()) > FADE_AT:
                for truth in list(truths_seen):
                    kept = Counter({s: n // 2 for s, n in truths_seen[truth].items() if n // 2})
                    if kept:
                        truths_seen[truth] = kept
                    else:
                        del truths_seen[truth]

    def summary(self):
        """['shape 86% (95 of 110)', ...] for the channels seen so far."""
        out = []
        for channel in CHANNELS:
            good, total = self.accuracy(channel)
            if total:
                out.append(f"{channel} {round(100 * good / total)}% ({good} of {total})")
        return out

    def worst(self, channel, most=3):
        """The truths the eye gets wrong most: 'star read as blob 18 of 30'."""
        found = []
        for truth, seen in self.counts.get(channel, {}).items():
            total = sum(seen.values())
            for s, n in seen.items():
                if not right(channel, truth, s) and has_word(channel, truth):
                    found.append((n / total, f"{truth} read as {s} {n} of {total}"))
        return [text for _, text in sorted(found, reverse=True)[:most]]

    def save(self):
        self.fade()
        lines = ["# brick's imagination: how well brick's eye reads back the pictures brick imagines (imagine.py).",
                 "# Each picture is drawn from a recipe brick knows; the eye looks without being told and is right",
                 "# or wrong per channel. A channel's counts halve when they pass "
                 f"{FADE_AT}, so recent dreams count most.",
                 "# 'tuned:' is what changed in the eye's shape thresholds (texts/factory/eye-settings.txt);",
                 "# 'learned:' is what imagination taught the neurons, and why. Delete this file to start again."]
        lines += [f"{k}: {self.totals[k]}" for k in ("pictures", "names", "blends", "contexts", "dreams") if self.totals[k]]
        for channel in CHANNELS:
            truths_seen = self.counts.get(channel)
            if not truths_seen:
                continue
            good, total = self.accuracy(channel)
            lines.append("")
            lines.append(f"# {channel}: right {good} of {total}" + (f" ({round(100 * good / total)}%)" if total else ""))
            for truth in sorted(truths_seen, key=lambda t: -sum(truths_seen[t].values())):
                seen = truths_seen[truth]
                n = sum(seen.values())
                lines.append(f"{channel}: {truth} read as " +
                             ", as ".join(f"{s} {c} of {n}" for s, c in seen.most_common()))
        for kind in ("tuned", "learned"):
            if self.notes[kind]:
                lines.append("")
                lines += [f"{kind}: {text}" for text in self.notes[kind]]
        os.makedirs(os.path.dirname(TALLY), exist_ok=True)
        with open(TALLY + ".new", "w", encoding="utf-8") as f:
            f.write("\n".join(lines) + "\n")
        os.replace(TALLY + ".new", TALLY)


def add_sample(truth, measures):
    """Keep the eye's shape measures: of a shape brick drew (truth: what it is), or of a real picture it was
    shown (truth: 'seen round', what the eye said then). The last MAX_SAMPLES drawn and MAX_SAMPLES / 4 seen."""
    if not measures or truth not in EYE_WORD and not truth.startswith("seen "):
        return
    keys = ("fit", "solidity", "corners", "points", "elongation", "holes", "hole share", "sides")
    line = f"{truth} | " + " ".join(f"{k} {round(float(measures.get(k, 0)), 3):g}" for k in keys)
    try:
        with open(SAMPLES, encoding="utf-8") as f:
            kept = [l.rstrip("\n") for l in f if "|" in l and not l.startswith("#")]
    except OSError:
        kept = []
    seen = [l for l in kept if l.startswith("seen ")]
    drawn = [l for l in kept if not l.startswith("seen ")]
    if truth.startswith("seen "):
        seen = (seen + [line])[-(MAX_SAMPLES // 4):]
    else:
        drawn = (drawn + [line])[-MAX_SAMPLES:]
    os.makedirs(os.path.dirname(SAMPLES), exist_ok=True)
    with open(SAMPLES + ".new", "w", encoding="utf-8") as f:
        f.write("# The eye's shape measures, for tuning it: 'star | ...' a shape brick drew itself; 'seen round | ...'\n"
                "# a real picture it was shown and what the eye called it (tuning must never change that).\n")
        f.write("\n".join(seen + drawn) + "\n")
    os.replace(SAMPLES + ".new", SAMPLES)


def read_samples(seen=False):
    """[(truth, measures)]: the drawn shapes, or (seen=True) the real pictures with what the eye called them."""
    rows = []
    try:
        with open(SAMPLES, encoding="utf-8") as f:
            lines = f.read().splitlines()
    except OSError:
        return rows
    for line in lines:
        if line.startswith("#") or "|" not in line:
            continue
        truth, _, rest = line.partition("|")
        truth = truth.strip()
        if truth.startswith("seen ") != seen:
            continue
        m = {k.strip(): float(v) for k, v in re.findall(r"([a-z][a-z ]*?) (-?\d+(?:\.\d+)?)", rest)}
        if (seen or truth in EYE_WORD) and {"fit", "solidity", "corners", "points", "elongation"} <= set(m):
            m.setdefault("holes", 0)
            m.setdefault("hole share", 0)
            m.setdefault("sides", 1)
            rows.append((truth[5:] if seen else truth, m))
    return rows


def remember_seen(path):
    """A real picture brick was shown: keep the eye's shape measures and what it called it, so tuning on
    drawn shapes never changes how the eye reads real things."""
    try:
        found = look_back(path)
    except Exception:
        return
    if found["things"] and getattr(found["things"][0], "measures", None):
        add_sample(f"seen {found['things'][0].shape}", found["things"][0].measures)


def tune(write=True):
    """Try the eye's shape thresholds within their safe ranges on the drawn shapes; keep a value only if the
    shapes are read clearly better, no one shape much worse, and no real picture you showed it read differently.
    -> lines saying what changed (and 'kept ...' when only a real picture stood in the way)."""
    import eye
    rows, real = read_samples(), read_samples(seen=True)
    if len(rows) < TUNE_FROM:
        return []
    current = eye.tuned()
    sizes = Counter(t for t, _ in rows)

    def score(settings):
        per = Counter(t for t, m in rows if right("shape", t, eye.classify(m, settings)))
        return sum(per.values()), per

    base, base_per = score(current)
    changed = []
    for name, (_, low, high) in eye.TUNABLE.items():
        values = [3, 4, 5, 6] if name == "star points" else [round(low + (high - low) * k / 12, 3) for k in range(13)]
        best_total, best_value = base, current[name]
        blocked = None                                   # the best value only a real picture stood in the way of
        for value in values:
            trial = dict(current, **{name: value})
            total, per = score(trial)
            if any(per[t] < base_per[t] - max(1, 0.1 * sizes[t]) for t in sizes):
                continue                                 # never much worse at any one shape
            if any(eye.classify(m, trial) != said_then for said_then, m in real):
                if total > (blocked or (best_total, 0))[0]:
                    blocked = (total, value)
                continue                                 # never a different reading of a real picture
            if total > best_total:
                best_total, best_value = total, value
        enough = base + max(2, 0.02 * len(rows))
        if blocked and blocked[0] >= enough and blocked[0] > best_total:
            changes = sum(eye.classify(m, dict(current, **{name: blocked[1]})) != said for said, m in real)
            changed.append(f"kept {name} {current[name]:g}: {blocked[1]:g} would read {blocked[0] - base} more drawn "
                           f"shapes right, but {changes} real picture{'s' * (changes != 1)} you showed it would "
                           "read differently")
        if best_total >= enough and best_value != current[name]:
            changed.append(f"{name} {current[name]:g} -> {best_value:g} (drawn shapes read right {base} -> "
                           f"{best_total} of {len(rows)})")
            current[name] = best_value
            base, base_per = score(current)
    if any(" -> " in c for c in changed) and write:
        eye.save_tuned(current, [f"{time.strftime('%Y-%m-%d %H:%M')}: {c}" for c in changed if " -> " in c])
    return changed


# ---------------- knowledge: what could it be? ----------------

_reverse, _starting, _wn_reverse, _glosses, _memory, _things, _degrees = {}, {}, {}, {}, {}, {}, {}


def degrees(words):
    """{word: how many ConceptNet lines start with it}: how much is known about each (one quick pass)."""
    path = symbols.CONCEPTNET
    need = [w for w in dict.fromkeys(words) if (path, w) not in _degrees]
    if need:
        data = symbols._bytes(path)
        counts = Counter()
        if data:
            pattern = rb"\n(" + b"|".join(re.escape(w.encode()) for w in sorted(need, key=len, reverse=True)) + rb")\t"
            counts = Counter(m.group(1).decode() for m in re.finditer(pattern, data))
        for w in need:
            _degrees[(path, w)] = counts[w]
    return {w: _degrees[(path, w)] for w in words}


def familiar(degree):
    """How much is known about a thing, from its ConceptNet lines: 0 -> 0.55, 15 -> 0.95, 30 or more -> 1."""
    return min(1.0, 0.55 + 0.1 * math.log2(1 + degree))


def cn_e(weight):
    """A ConceptNet weight as evidence from 0 to 1: weight 1 -> 0.3, 4 -> 0.7, 9 and more -> 1."""
    return min(1.0, math.log2(1 + weight) / math.log2(10)) if weight > 0 else 0.0


def cn_reverse(word):
    """{thing: (weight, relation)} for every ConceptNet line 'thing relation word' (C-speed search)."""
    key = (symbols.CONCEPTNET, word)
    if key not in _reverse:
        data, found, at = symbols._bytes(symbols.CONCEPTNET), {}, 0
        needle = b"\t" + word.encode() + b"\t"
        while data:
            at = data.find(needle, at)
            if at < 0:
                break
            start, end = data.rfind(b"\n", 0, at) + 1, data.find(b"\n", at)
            end = len(data) if end < 0 else end
            parts = data[start:end].decode("utf-8", "replace").split("\t")
            at = end
            if len(parts) == 4 and parts[1] in CN_RELATIONS and parts[2] == word:
                try:
                    weight = float(parts[3])
                except ValueError:
                    continue
                thing = symbols.tidy(parts[0])
                if thing and weight > found.get(thing, (0,))[0]:
                    found[thing] = (weight, parts[1])
        _reverse[key] = found
    return _reverse[key]


def cn_from(word):
    """{other: (weight, relation)} for every ConceptNet line 'word relation other'."""
    key = (symbols.CONCEPTNET, word)
    if key not in _starting:
        found = {}
        for line in symbols._lines_starting(symbols.CONCEPTNET, (word + "\t").encode()):
            parts = line.split("\t")
            if len(parts) == 4 and parts[1] in CN_RELATIONS:
                try:
                    weight = float(parts[3])
                except ValueError:
                    continue
                other = symbols.tidy(parts[2])
                if other and weight > found.get(other, (0,))[0]:
                    found[other] = (weight, parts[1])
        _starting[key] = found
    return _starting[key]


def wn_reverse(word):
    """{noun: its definition} for WordNet nouns whose definition uses word ('red' -> apple, tomato ...)."""
    key = (symbols.WORDNET, word)
    if key not in _wn_reverse:
        found = {}
        data = symbols._bytes(os.path.join(symbols.WORDNET, "data.noun"))
        if data and re.fullmatch(r"[a-z][a-z ]*", word):
            for hit in re.finditer(rb"[ (]" + re.escape(word.encode()) + rb"[ ,;)]", data):
                p = hit.start()
                start = data.rfind(b"\n", 0, p) + 1
                bar = data.find(b" | ", start, p)
                if bar < 0:
                    continue
                end = data.find(b"\n", p)
                gloss = data[bar + 3:end if end >= 0 else len(data)].decode("utf-8", "replace")
                gloss = gloss.split('"')[0].split(";")[0].strip()
                if not re.search(rf"\b{re.escape(word)}\b", gloss):
                    continue
                fields = data[start:bar].split()
                if len(fields) < 5:
                    continue
                noun = fields[4].decode("utf-8", "replace")
                if noun[:1].isupper():
                    continue
                found.setdefault(re.sub(r"\(.*?\)$", "", noun).replace("_", " "), gloss)
                if len(found) >= 150:
                    break
        _wn_reverse[key] = found
    return _wn_reverse[key]


def wn_glosses(name):
    """[(definition, rank)] of name's first WordNet noun meanings."""
    key = (symbols.WORDNET, name)
    if key not in _glosses:
        out = []
        if os.path.isdir(symbols.WORDNET):
            known = symbols.wn_index(name, "n")
            for rank, offset in enumerate((known[1] if known else [])[:3]):
                try:
                    meaning = symbols.wn_synset(offset, "n")[2]
                except (ValueError, IndexError):
                    continue
                out.append((meaning.split('"')[0].split(";")[0].strip(), rank))
        _glosses[key] = out
    return _glosses[key]


def memory(deadline=None):
    """{name: {value: (facet, strength, yours)}} for every neuron (re-read only when its file changes)."""
    folder = symbols.FOLDER
    if not os.path.isdir(folder):
        return {}
    seen = set()
    for entry in os.scandir(folder):
        if not entry.name.endswith(".txt") or entry.name.startswith(("_", "letter-")):
            continue
        seen.add(entry.path)
        try:
            when = entry.stat().st_mtime
        except OSError:
            continue
        if _memory.get(entry.path, (None,))[0] == when or deadline and time.monotonic() > deadline:
            continue
        neuron = symbols.Neuron(symbols.Neuron.read_name(entry.path) or entry.name[:-4].replace("-", " "))
        links = {value: (facet, link.strength, link.yours) for value, facet, link in neuron.links()
                 if facet not in symbols.FORM and facet not in ("before", "after", "in words", "place")}
        _memory[entry.path] = (when, neuron.name, links)
    for path in [p for p in _memory if p not in seen]:
        del _memory[path]
    return {name: links for _, name, links in _memory.values()}


def is_thing(word, avoid=()):
    """Could this word be a thing you can see (a noun WordNet knows, not a colour or shape word)?"""
    if word in avoid or set(word.split()) & PROPERTY:            # not 'regular hexagon' for a hexagon
        return False
    key = (symbols.WORDNET, word)
    if key not in _things:
        ok = bool(re.fullmatch(r"[a-z][a-z '\-]{1,30}", word)) and len(word.split()) <= 2 and \
            word not in symbols.STOP and word not in symbols.TOO_WIDE and \
            (symbols.guess_facet(word) not in ("colour", "taste", "texture") and not rgb_for(word) or
             colour_thing(word)) and word not in symbols.SHAPES - {"ball", "cube", "disc", "disk", "cone", "ring",
                                                                   "cylinder", "sphere"} and \
            (not os.path.isdir(symbols.WORDNET) or bool(symbols.wn_index(word, "n")))
        _things[key] = ok
    return _things[key]


def feature(key, colour_rgb=None):
    """One imagined symbol, as the knowledge says it: {facet, say, words: {word: how well}, weight}."""
    if key in FEATURES:
        facet, words = FEATURES[key]
        say = next(iter(words))
    else:                                               # a colour
        import sight
        facet, words, say = "colour", {key: 1.0}, key
        if key == "grey":
            words["gray"] = 1.0
        fine = sight.fine_name(*colour_rgb) if colour_rgb else None
        return {"facet": facet, "say": say, "words": words, "fine": fine if fine and colour_thing(fine) else None,
                "weight": FACET_WEIGHT[facet] / (1 + len(cn_reverse(say))).bit_length()}
    return {"facet": facet, "say": say, "words": dict(words), "fine": None,
            "weight": FACET_WEIGHT[facet] / (1 + len(cn_reverse(say))).bit_length()}


def features_of(recipe):
    """The imagined picture's symbols (its main thing, or its scene) -> [feature]."""
    if recipe.get("scene"):
        out = [feature("scene"), feature(f"{recipe['scene']['ground']} ground")]
        if recipe["things"]:
            out += [feature("circle"), feature("yellow", BASE["yellow"])]
        return out
    main = main_thing(recipe) or recipe["things"][0]
    out = [feature(main["shape"]), feature(main["colour"], main["rgb"])]
    if main.get("texture", "plain") != "plain":
        out.append(feature(main["texture"]))
    if main.get("size") in ("big", "small") and recipe["kind"] != "named":
        out.append(feature(main["size"]))
    return out


def features_seen(facets):
    """What the eye saw (or a picture you showed) -> [feature]."""
    out = []
    shapes = [v for v, _ in facets.get("shape", []) if v in FROM_EYE]
    if shapes:
        out.append(feature(FROM_EYE[shapes[0]]))
    colours = [v for v, _ in facets.get("colour", []) if v in BASE]
    if colours:
        out.append(feature(colours[0], BASE[colours[0]]))
    patterns = [v for v, _ in facets.get("pattern", []) if v in ("striped", "dotted", "checked")]
    if patterns:
        out.append(feature(patterns[0]))
    return out


def evidence(name, f, known):
    """How strongly the knowledge says name has this symbol -> (0..1, {source: [why]})."""
    best, why = 0.0, {}
    links = known.get(name, {})
    glosses = wn_glosses(name)
    for word, how_well in f["words"].items():
        reasons = {}
        e_memory = min(1.0, links[word][1] / symbols.YOURS) if word in links else 0.0
        if e_memory:
            reasons["memory"] = [f"{links[word][0]} {word} {round(links[word][1], 1):g}"]
        rev, fwd = cn_reverse(word).get(name), cn_from(word).get(name)
        hit = max([x for x in (rev, fwd) if x], default=None)
        e_cn = cn_e(hit[0]) if hit else 0.0
        if hit:
            reasons["ConceptNet"] = [f"{hit[1]} {word} {round(hit[0], 1):g}"]
        e_wn = 0.0
        for gloss, rank in glosses:
            if re.search(rf"\b{re.escape(word)}\b", gloss):
                e_wn = max(e_wn, 0.8 if rank == 0 else 0.5)
                reasons.setdefault("WordNet", [f"'{gloss[:60]}'"])
        e_dict = 0.6 if f["facet"] == "colour" and f.get("fine") == name else 0.0
        if e_dict:
            reasons["dictionary"] = [f"the colour {name}"]
        e = (1 - (1 - e_memory) * (1 - e_cn) * (1 - e_wn) * (1 - e_dict)) * how_well
        if e > best:
            best, why = e, dict(reasons, word=(word, how_well))
    return best, why


def contradicts(name, f, known):
    """Does the knowledge give name another colour (or shape), and not this one?"""
    if f["facet"] not in ("colour", "shape"):
        return False
    said_so = {v for v, (facet, strength, _) in known.get(name, {}).items() if facet == f["facet"] and strength >= 1}
    glosses = wn_glosses(name)
    if glosses:
        said_so |= {w for w in re.findall(r"[a-z]+", glosses[0][0]) if symbols.guess_facet(w) == f["facet"]}
    return bool(said_so) and not said_so & set(f["words"])


def associate(features, avoid=(), most=5, deadline=None):
    """What could a thing with these symbols be? -> [{name, score, reason, supported, sources}], best first.
    Candidates come from every source (neurons, ConceptNet both ways, WordNet definitions, the dictionary's
    colours named after things); each is then scored on every symbol."""
    if not features:
        return []
    known = memory(deadline)
    pre = Counter()
    complete = True
    for f in features:
        if deadline and time.monotonic() > deadline:
            complete = False
            break
        for word, how_well in f["words"].items():
            for thing, (weight, _) in cn_reverse(word).items():
                pre[thing] += f["weight"] * how_well * cn_e(weight)
            for thing, (weight, _) in cn_from(word).items():
                pre[thing] += f["weight"] * how_well * cn_e(weight) * 0.5
            if how_well >= 0.8 and not (deadline and time.monotonic() > deadline):
                for thing in wn_reverse(word):
                    pre[thing] += f["weight"] * how_well * 0.5
            for name, links in known.items():
                if word in links:
                    pre[name] += f["weight"] * how_well * min(1.0, links[word][1] / symbols.YOURS) * 1.5
        if f.get("fine"):
            pre[f["fine"]] += f["weight"] * 0.6
    avoid = set(avoid) | {w for f in features for w in f["words"]}
    candidates = []
    for name, _ in pre.most_common(300):
        if deadline and time.monotonic() > deadline:
            complete = False
            break
        if is_thing(name, avoid):
            candidates.append(name)
            if len(candidates) >= 24:
                break
    total = sum(f["weight"] for f in features)
    known_about = degrees(candidates)
    found = []
    for name in candidates:
        got, supported, sources, why_all = 0.0, [], set(), {}
        penalty = 1.0
        for f in features:
            e, why = evidence(name, f, known)
            got += f["weight"] * e
            if e >= 0.25:
                word, how_well = why.pop("word")
                supported.append(dict(f, evidence=round(e, 2), matched=word, exact=how_well >= 0.8))
                for source, reasons in why.items():
                    sources.add(source)
                    why_all.setdefault(source, []).extend(reasons)
            elif contradicts(name, f, known):
                penalty *= 0.7
        score = round(penalty * familiar(known_about[name]) * got / total, 2) if total else 0.0
        if not supported:
            continue
        reason = f"{name}: " + " + ".join(f"{f['say']} ({f['facet']})" for f in supported)
        for source in ("memory", "ConceptNet", "WordNet", "dictionary"):
            if source in why_all:
                bits = list(dict.fromkeys(why_all[source]))[:3]
                reason += f"; {source} " + ", ".join(bits)
        found.append({"name": name, "score": score, "reason": reason, "supported": supported,
                      "sources": sorted(sources), "complete": complete, "known": familiar(known_about[name])})
    found.sort(key=lambda c: -c["score"])
    return found[:most]


def could_be(found, most=3):
    """'apple 0.71 (round + red; ConceptNet, WordNet), tomato 0.6 (...)'"""
    return ", ".join(f"{c['name']} {c['score']:.2f} ({' + '.join(f['say'] for f in c['supported'])}; "
                     f"{', '.join(c['sources'])})" for c in found[:most])


# ---------------- learning what it could be ----------------

def link_safely(linker, owner, facet, value, amount, reason):
    """Strengthen owner -> value with source 'imagination' (and the reason), unless you wrote it, or you wrote
    what owner's facet is, or it's on owner's 'not:' line. -> True if linked."""
    owner, value = symbols.clean(owner), symbols.tidy(value)
    if not owner or not value or owner == value:
        return False
    if symbols.exists(owner) or owner in linker.neurons:
        neuron = linker.neuron(owner)
        if value in neuron.meta.get("not", ()):
            return False
        where = neuron.find(value)
        if where and neuron.facets[where][value].yours:
            return False                                   # yours: never touched
        yours = [v for v, link in neuron.facets.get(facet, {}).items() if link.yours]
        if yours and value not in yours:
            return False                                   # you said what its colour (or shape) is
        linker.link(owner, facet, value, amount, "imagination", given_facet=True)
        link = neuron.facets.get(facet, {}).get(value)
    else:
        linker.link(owner, facet, value, amount, "imagination", given_facet=True)
        link = (linker.waiting or {}).get(owner, {}).get(facet, {}).get(value)
    if link is not None:
        why = symbols.plain(reason)[:80]
        link.sources = [s for s in link.sources if s not in ("imagination", why)][-4:] + ["imagination", why]
    return link is not None


def learn(found, features, room=LINKS_PER_ROUND):
    """Learn the strong, well-supported answers into their neurons. -> (lines saying what and why, links used)."""
    if not found or room <= 0 or not found[0].get("complete", True):
        return [], 0
    top = found[0]["score"]
    linker, said, used = symbols.Linker(), [], 0
    shape = next((f["say"] for f in features if f["facet"] == "shape"), None)
    colour = next((f["say"] for f in features if f["facet"] == "colour"), None)
    for c in found[:2]:
        sure = [f for f in c["supported"] if f["exact"] and f["evidence"] >= 0.5]   # not 'round' for a ring
        if len(c["sources"]) < 2 and c.get("known", 1) < 0.95:
            continue                                       # one source about a little-known thing: not enough
        if c["score"] < LEARN_AT or c["score"] < 0.75 * top or \
                sum(f["facet"] in ("shape", "colour", "pattern", "scene") for f in sure) < 2:
            continue
        amount = round(IMAGINED * c["score"], 2)
        reason = "imagined " + "+".join(f["say"] for f in sure) + " by " + "+".join(c["sources"])
        linked = []
        for f in sure:
            if used >= room or f["facet"] not in ("shape", "colour", "pattern"):
                continue
            if link_safely(linker, c["name"], f["facet"], f["say"], amount, reason):
                linked.append(f["say"])
                used += 1
        if used < room and {shape, colour} <= {f["say"] for f in sure}:
            frame = f"a picture with {colour} and {shape}"
            if link_safely(linker, c["name"], "contexts", frame, amount / 2, reason):
                linker.link(f"context:{frame}", "fills", c["name"], amount / 2, "imagination", given_facet=True)
                used += 1
        if linked:
            said.append(f"{c['reason']} ({c['score']:.2f}): learned {', '.join(linked)}")
    linker.save()
    return said, used


# ---------------- what is known about a named thing ----------------

def ratio_of(links):
    for value, (facet, _, _) in links.items():
        found = re.match(r"^ratio (\d+(?:\.\d+)?)$", value)
        if found:
            return float(found.group(1))
        found = re.match(r"^about (\d+(?:\.\d+)?) times as (wide as tall|tall as wide)$", value)
        if found:
            n = float(found.group(1))
            return n if found.group(2).startswith("wide") else 1 / n
    return None


def traits(name):
    """What brick knows about how name looks -> {shape, colour, pattern, parts, ratio, why: [...]}: from its
    neuron first, then WordNet's definition, then ConceptNet."""
    name = symbols.clean(name)
    scores = {"shape": Counter(), "colour": Counter(), "pattern": Counter(), "parts": Counter()}
    why = {}

    def put(facet, value, strength, source):
        if strength > 0:
            scores[facet][value] += strength
            why.setdefault((facet, value), source)

    links = memory().get(name, {})
    for value, (facet, strength, _) in links.items():
        if facet == "shape" and value in DRAWABLE:
            put("shape", value, strength + 1, f"its universe {strength:g}")
        elif facet == "colour" and rgb_for(value):
            put("colour", value, strength + 1, f"its universe {strength:g}")
        elif facet in ("texture", "pattern") and value in PATTERN_WORDS:
            put("pattern", PATTERN_WORDS[value], strength + 1, f"its universe {strength:g}")
        elif facet == "parts" and value in PARTS:
            put("parts", PARTS[value], strength + 1, f"its universe {strength:g}")
    for gloss, rank in wn_glosses(name)[:2]:
        strength = 1.5 if rank == 0 else 0.7
        for word in re.findall(r"[a-z]+(?:-[a-z]+)?", gloss.lower()):
            word = word.replace("-shaped", "").replace("-", " ")
            if word in DRAWABLE:
                put("shape", word, strength, f"WordNet '{gloss[:50]}'")
            elif symbols.guess_facet(word) == "colour" and rgb_for(word):
                put("colour", word, strength, f"WordNet '{gloss[:50]}'")
            elif word in PATTERN_WORDS:
                put("pattern", PATTERN_WORDS[word], strength, "WordNet")
            elif word in PARTS:
                put("parts", PARTS[word], strength * 0.8, "WordNet")
    for other, (weight, relation) in cn_from(name).items():
        strength = min(3.0, 0.5 + weight / 4)
        if other in DRAWABLE and relation in ("RelatedTo", "HasProperty", "SimilarTo"):
            put("shape", other, strength, f"ConceptNet {relation} {weight:g}")
        elif symbols.guess_facet(other) == "colour" and rgb_for(other) and relation in ("RelatedTo", "HasProperty"):
            put("colour", other, strength, f"ConceptNet {relation} {weight:g}")
        elif other in PATTERN_WORDS:
            put("pattern", PATTERN_WORDS[other], strength, f"ConceptNet {relation}")
        elif other in PARTS and relation in ("HasA", "RelatedTo", "PartOf"):
            put("parts", PARTS[other], strength, f"ConceptNet {relation}")
    out = {facet: (counts.most_common(1)[0][0] if counts else None) for facet, counts in scores.items()
           if facet != "parts"}
    out["parts"] = [p for p, s in scores["parts"].most_common(3) if s >= 1.0]
    out["ratio"] = ratio_of(links)
    out["why"] = [f"{facet} {out[facet]} ({why[(facet, out[facet])]})" for facet in ("shape", "colour", "pattern")
                  if out[facet]] + [f"parts {p} ({why[('parts', p)]})" for p in out["parts"]]
    return out


def singular(word):
    """'apples' -> 'apple', when brick knows apple (a neuron, or WordNet) and not apples."""
    wordnet = os.path.isdir(symbols.WORDNET)
    if symbols.exists(word) or wordnet and symbols.wn_index(word, "n"):
        return word
    for cut in ("es", "s"):
        stem = word[:-len(cut)]
        if word.endswith(cut) and (symbols.exists(stem) or wordnet and symbols.wn_index(stem, "n")):
            return stem
    return word


# ---------------- the pictures and the log ----------------

def tidy_field(text):
    return re.sub(r"\s+", " ", str(text).replace("|", "/")).strip() or "-"


def save_picture(origin, words, w, h, pixels):
    import sight
    os.makedirs(PICTURES, exist_ok=True)
    _made[0] += 1
    slug = re.sub(r"[^a-z0-9]+", "-", words.lower()).strip("-")[:40] or "thing"
    name = f"{origin}-{time.strftime('%Y%m%d-%H%M%S')}-{_made[0] % 1000:03d}-{slug}.png"
    path = os.path.join(PICTURES, name)
    sight.write_png(path, w, h, pixels)
    trim_pictures(keep=name)
    return path


def trim_pictures(keep=None):
    """At most MAX_PICTURES pictures: the oldest imagined on its own go first, then the oldest of the rest
    (never keep, the one just made)."""
    try:
        files = [f for f in os.listdir(PICTURES) if f.endswith(".png") and f != keep]
    except OSError:
        return
    extra = len(files) + (keep is not None) - MAX_PICTURES
    if extra <= 0:
        return
    when = lambda f: f.split("-", 1)[1] if "-" in f else f          # noqa: E731 (the time in the name)
    order = sorted([f for f in files if f.startswith("own-")], key=when) + \
        sorted([f for f in files if not f.startswith("own-")], key=when)
    for f in order[:extra]:
        try:
            os.remove(os.path.join(PICTURES, f))
        except OSError:
            pass


def record(label, picture, recipe_text, eye_text, could_text):
    """One line in texts/imagination/log.txt: when | where it came from | picture | recipe | eye saw | could be."""
    os.makedirs(PICTURES, exist_ok=True)
    line = " | ".join([time.strftime("%Y-%m-%d %H:%M:%S", time.localtime(now())), tidy_field(label),
                       os.path.basename(picture) if picture else "-", tidy_field(recipe_text), tidy_field(eye_text),
                       tidy_field(could_text)])
    with open(log_path(), "a", encoding="utf-8") as f:
        f.write(line + "\n")
    try:
        if os.path.getsize(log_path()) > MAX_LOG * 400:          # roughly: trim now and then, not every time
            with open(log_path(), encoding="utf-8") as f:
                kept = f.read().splitlines()[-MAX_LOG:]
            with open(log_path() + ".new", "w", encoding="utf-8") as f:
                f.write("\n".join(kept) + "\n")
            os.replace(log_path() + ".new", log_path())
    except OSError:
        pass
    return line


def origin_of(label):
    for key, words in ORIGINS.items():
        if label.startswith(words):
            return key
    return "you"


def entries(origin=None, today=False, most=10):
    """The log, newest first -> [{time, origin, label, picture, path, exists, recipe, eye, could be}]."""
    try:
        with open(log_path(), encoding="utf-8", errors="replace") as f:
            lines = f.read().splitlines()
    except OSError:
        return []
    day = time.strftime("%Y-%m-%d", time.localtime(now()))
    out = []
    for line in reversed(lines):
        parts = line.split(" | ")
        if len(parts) != 6:
            continue
        key = origin_of(parts[1])
        if origin and key != origin or today and not parts[0].startswith(day):
            continue
        path = os.path.join(PICTURES, parts[2]) if parts[2] != "-" else None
        out.append({"time": parts[0], "origin": key, "label": parts[1], "picture": parts[2] if path else None,
                    "path": path, "exists": bool(path and os.path.isfile(path)), "recipe": parts[3],
                    "eye": parts[4], "could be": parts[5]})
        if len(out) >= most:
            break
    return out


# ---------------- imagining ----------------

def look_back(path, memory_too=False):
    import eye
    return eye.see(path, model=False, memory=memory_too, read=False)


def imagine_picture(recipe, origin, label, tally, rng=None, think=True, teach=True, room=LINKS_PER_ROUND,
                    deadline=None, avoid=()):
    """Draw a recipe, have the eye read it back (tallied), and say what it could be. -> an item dict."""
    w, h, pixels = draw(recipe)
    words = describe(recipe)
    path = save_picture(origin, recipe.get("title") or thing_words(main_thing(recipe) or recipe["things"][0])
                        if recipe["things"] else "scene", w, h, pixels)
    found = look_back(path, memory_too=recipe.get("kind") == "named")
    reads = eye_reads(found, recipe)
    truth = truths(recipe)
    scored = []
    for channel, value in truth.items():
        if channel in reads:
            tally.add(channel, value, reads[channel])
            if has_word(channel, value):
                scored.append(right(channel, value, reads[channel]))
    if "shape" in truth:
        add_sample(truth["shape"], reads.get("measures"))
    tally.totals["pictures"] += 1
    found_could, learned, used = [], [], 0
    if think and not (deadline and time.monotonic() > deadline):
        features = features_of(recipe)
        found_could = associate(features, avoid=avoid, deadline=deadline)
        if teach:
            learned, used = learn(found_could, features, room)
            for line in learned:
                tally.note("learned", line)
    could_text = could_be(found_could)
    entry = record(label, path, words, said(reads), could_text)
    return {"path": path, "recipe": recipe, "words": words, "reads": reads, "eye": said(reads), "found": found,
            "could": found_could, "could text": could_text, "learned": learned, "used": used,
            "right": sum(scored), "scored": len(scored), "pixels": (w, h, pixels), "entry": entry}


def imagine_named(name, origin="you", label=None, tally=None, rng=None):
    """Draw name from what brick knows about it, and see whether the eye reads it back as name."""
    name = symbols.clean(name)
    own_tally = tally is None
    tally = tally or Tally()
    found = traits(name)
    recipe = recipe_from_traits(found, name, rng)
    item = imagine_picture(recipe, origin, label or f"{ORIGINS[origin]}: imagine {name}", tally, rng, teach=False)
    eye_name = item["found"].get("name")
    seen_could = associate(features_seen(item["found"]["facets"]), most=5)
    seen_names = [c["name"] for c in seen_could[:3]]
    matched = eye_name == name or name in seen_names          # the eye's reading, not the recipe, must say it
    tally.add("names", name, eye_name or (seen_names[0] if seen_names else "nothing"))
    tally.totals["names"] += 1
    if own_tally:
        tally.save()
    how = ("memory named it" if eye_name == name else
           "it's in what the eye's symbols could be" if name in seen_names else "")
    item.update(name=name, traits=found, eye_name=eye_name, matched=matched, how=how, seen_could=seen_could)
    return item


def imagine_blend(a, b, origin="you", label=None, tally=None, rng=None, teach=True, room=LINKS_PER_ROUND):
    """Mix two symbols: what they share, then shape from b and colour from a -> drawn, and what it could be."""
    a, b = symbols.clean(a), symbols.clean(b)
    own_tally = tally is None
    tally = tally or Tally()
    ta, tb = traits(a), traits(b)
    mixed = {"shape": tb["shape"] or ta["shape"], "colour": ta["colour"] or tb["colour"],
             "pattern": ta["pattern"] or tb["pattern"], "parts": [], "ratio": None}
    if ta["shape"] and DRAWABLE.get(ta["shape"]) == DRAWABLE.get(tb["shape"] or ""):
        mixed["shape"] = ta["shape"]
    recipe = recipe_from_traits(mixed, f"{a} + {b}", rng)
    item = imagine_picture(recipe, origin, label or f"{ORIGINS[origin]}: blend {a} + {b}", tally, rng,
                           teach=teach, room=room, avoid=(a, b))
    tally.totals["blends"] += 1
    if own_tally:
        tally.save()
    looks = " ".join(w for w in (mixed["colour"], mixed["pattern"], mixed["shape"]) if w) or "something"
    item.update(blend=(a, b), mixed=mixed, looks=f"{looks} thing")
    return item


def imagine_context(rng=None, origin="own", tally=None, room=2):
    """A new way to fill a context brick knows: 'a _ is' filled by apple and pear -> 'a cherry is'.
    -> (line, learned) or None."""
    rng = rng or random
    folder = symbols.FOLDER
    try:
        files = [f for f in os.listdir(folder) if f.startswith("context-") and f.endswith(".txt")]
    except OSError:
        return None
    rng.shuffle(files)
    for file in files[:6]:
        neuron = symbols.Neuron(symbols.Neuron.read_name(os.path.join(folder, file)) or file[:-4])
        fillers = [v for v in neuron.facets.get("fills", {}) if re.fullmatch(r"[a-z]+", v)]
        if len(fillers) < 2:
            continue
        kinds = Counter()
        for filler in fillers[:12]:
            known = set(symbols.Neuron(filler).facets.get("is", {})) if symbols.exists(filler) else set()
            known |= {o for o, (w, rel) in cn_from(filler).items() if rel == "IsA" and w >= 1}
            for kind in known:
                if kind not in symbols.TOO_WIDE:
                    kinds[kind] += 1
        shared = [(n, k) for k, n in kinds.most_common(5) if n >= 2]
        if not shared:
            continue
        n, kind = shared[0]
        options = [(w, t) for t, (w, rel) in cn_reverse(kind).items()
                   if rel == "IsA" and t not in fillers and re.fullmatch(r"[a-z]+", t) and is_thing(t)]
        if not options:
            continue
        options.sort(reverse=True)
        weight, new = rng.choice(options[:5])
        frame = neuron.name.split(":", 1)[1]
        example = [f for f in fillers if kind in cn_from(f) or symbols.exists(f)][:2] or fillers[:2]
        line = f"'{frame.replace('_', new)}': {new} is a {kind}, like {' and '.join(example)} " \
               f"({n} of the words that fill '{frame}' are; ConceptNet IsA {weight:g})"
        learned = False
        if room > 0:
            linker = symbols.Linker()
            learned = link_safely(linker, neuron.name, "fills", new, 0.2, f"imagined {new} is a {kind} like "
                                                                           f"{'+'.join(example)}")
            linker.save()
        record(f"{ORIGINS[origin]}: context", None, f"context '{frame}' filled with {new}", "-", line)
        if tally is not None:
            tally.totals["contexts"] += 1
        return line, learned
    return None


# ---------------- when it happens ----------------

def dream(budget=ROUND_BUDGET, most=PER_ROUND, origin="own", rng=None, label=None):
    """One dream round -> (one line saying what happened, [items])."""
    rng = rng or random.Random()
    started = time.monotonic()
    deadline = started + budget
    tally, items, room, extra = Tally(), [], LINKS_PER_ROUND, []
    label = label or ORIGINS[origin]
    if _queue and origin == "own":
        recent, source = _queue.pop(0)
        for line in react(recent, source, budget=budget / 2, most=1, rng=rng, tally=tally):
            extra.append(line)
    for k in range(most):
        if items and time.monotonic() - started > 0.6 * budget:
            break
        think = k == 0 and time.monotonic() < started + 0.5 * budget
        item = imagine_picture(random_recipe(rng), origin, label, tally, rng, think=think, room=room,
                               deadline=deadline)
        room -= item["used"]
        items.append(item)
    if time.monotonic() < started + 0.6 * budget and rng.random() < 0.25:
        got = imagine_context(rng, origin, tally, room=min(1, room))
        if got:
            extra.append(f"imagined {got[0]}")
    tally.totals["dreams"] += 1
    tuned = []
    if tally.totals["pictures"] // TUNE_EVERY != (tally.totals["pictures"] - len(items)) // TUNE_EVERY:
        tuned = tune()
        for line in tuned:
            tally.note("tuned", line)
    tally.save()
    good = sum(i["right"] for i in items)
    out = sum(i["scored"] for i in items)
    line = f"dreamt {len(items)}: " + "; ".join(f"{thing_words(main_thing(i['recipe']))} (eye: {i['reads']['shape']})"
                                                if main_thing(i["recipe"]) and not i["recipe"].get("scene")
                                                else "a scene" for i in items)
    line += f"; eye right {good} of {out}"
    thought = next((i for i in items if i["could"]), None)
    if thought:
        line += f"; could be {thought['could'][0]['name']} {thought['could'][0]['score']:.2f}"
    learned = sum(len(i["learned"]) for i in items)
    if learned:
        line += f"; learned {learned}"
    if tuned:
        line += "; tuned the eye: " + "; ".join(tuned)
    line += "".join(f"; {x}" for x in extra)
    return line + f" ({time.monotonic() - started:.2f}s)", items


def dream_round(budget=ROUND_BUDGET, most=PER_ROUND):
    """The factory's round: a few imagined pictures, read back and tallied. Never raises. -> a line, or ''."""
    try:
        return dream(budget, most)[0]
    except Exception as problem:                      # imagining must never stop the factory
        return f"problem: {type(problem).__name__}: {problem}"


def later(recent, source="watched folders"):
    """Remember what was just learned (while watching folders), for the next dream round to react to."""
    if recent:
        _queue.append((list(recent)[-20:], source))
        del _queue[:-5]


def react(recent, source="brick learn", budget=4.0, most=3, rng=None, tally=None):
    """After learning: imagine a few variations or blends of what was just learned. recent: [{"what", and
    "symbols" [(value, facet)] and "name" for a picture, or "words" for a text}]. -> lines saying what."""
    rng = rng or random.Random()
    started = time.monotonic()
    own_tally = tally is None
    tally = tally or Tally()
    lines, done = [], 0
    for item in reversed(list(recent or [])):
        if done >= most or time.monotonic() - started > budget:
            break
        label = f"{ORIGINS['learned']} {item.get('what') or source}"
        if item.get("path") and os.path.isfile(item["path"]):
            remember_seen(item["path"])
        if item.get("symbols"):
            facets = {}
            for value, facet in item["symbols"]:
                facets.setdefault(facet, []).append((value, 1.0))
            shape = next((FROM_EYE[v] for v, _ in facets.get("shape", []) if v in FROM_EYE), None)
            colour = next((v for v, _ in facets.get("colour", []) if v in DRAWN), None)
            tries = []
            if shape:
                tries.append(("same shape, another colour", dict(shape=shape, avoid=(colour,) if colour else ())))
            if colour:
                tries.append(("same colour, another shape",
                              dict(colour=colour, shape=rng.choice([s for s in SHAPES if s != shape]))))
            for why, how in tries:
                if done >= most or time.monotonic() - started > budget:
                    break
                recipe = random_recipe(rng)
                recipe.update(kind="one", scene=None, background="white", **{"background rgb": (255, 255, 255)})
                t = new_thing(rng, shape=how.get("shape"), colour=how.get("colour"), texture="plain",
                              avoid=how.get("avoid", ()) + ("white",))
                recipe["things"] = [place(t, recipe["width"], recipe["height"], (1, 1), rng, rng.uniform(0.1, 0.2))]
                got = imagine_picture(recipe, "learned", label, tally, rng)
                lines.append(f"{why}: {got['words']}; eye: {got['eye']}" +
                             (f"; could be {got['could text']}" if got["could"] else ""))
                done += 1
            if item.get("name") and done < most and time.monotonic() - started < budget:
                got = imagine_named(item["name"], "learned", label, tally, rng)
                lines.append(f"drew {got['name']} back from what it knows: {got['words']}; eye: {got['eye']}; "
                             f"{'matched' if got['matched'] else 'not matched'}")
                done += 1
        elif item.get("words"):
            seen = []
            for word in item["words"]:
                word = singular(symbols.clean(word))
                if word in seen or not is_thing(word):
                    continue
                found = traits(word)
                if found["shape"] or found["colour"]:
                    seen.append(word)
                if len(seen) >= 2:
                    break
            if seen and done < most:
                got = imagine_named(seen[0], "learned", label, tally, rng)
                lines.append(f"imagined {got['name']}: {got['words']}; eye: {got['eye']}; "
                             f"{'matched' if got['matched'] else 'not matched'}")
                done += 1
            if len(seen) >= 2 and done < most and time.monotonic() - started < budget:
                got = imagine_blend(seen[0], seen[1], "learned", label, tally, rng)
                lines.append(f"blended {seen[0]} + {seen[1]}: a {got['looks']}" +
                             (f"; could be {got['could text']}" if got["could"] else ""))
                done += 1
    if own_tally:
        tally.save()
    return lines


def react_to_show(args):
    """After brick show apple.png as apple (or brick look ...): the imagination reacts to that picture."""
    try:
        import brain
        import sight
        if brain.off():
            return
        request = sight.show_request(" ".join(args))
        if not request or not os.path.isfile(request[0]):
            return
        path, name = request
        seen = sight.look(path, symbols.clean(name) or None, learn=False, model=False)
        if not seen["symbols"]:
            return
        lines = react([{"what": os.path.basename(path), "name": seen["name"], "path": path,
                        "symbols": [(v, f) for v, f, _ in seen["symbols"]]}], "brick show")
        if lines:
            print("Imagined after seeing it (brick imaginations learned draws them):")
            for line in lines:
                print("  " + line)
    except Exception as problem:                      # imagining must never spoil a show
        print(f"  (the imagination couldn't react: {type(problem).__name__}: {problem})")


# ---------------- showing it: half-block pictures in the terminal ----------------

def halfblock(width, height, pixels, columns=40):
    """A picture as terminal lines: each character is two pixels, the top one in the foreground colour of '▀'
    and the bottom one in the background colour (24-bit colour escapes)."""
    columns = max(1, min(columns, width))
    sx = width / columns
    rows = max(2, round(height / sx))
    rows += rows % 2
    sy = height / rows

    def at(c, r):
        x0, y0 = int(c * sx), int(r * sy)
        x1, y1 = max(x0 + 1, int((c + 1) * sx)), max(y0 + 1, int((r + 1) * sy))
        box = [pixels[y * width + x] for y in range(y0, min(y1, height)) for x in range(x0, min(x1, width))]
        return tuple(round(sum(p[k] for p in box) / len(box)) for k in range(3))
    lines = []
    for r in range(0, rows, 2):
        cells = []
        for c in range(columns):
            (a, b, d), (e, f, g) = at(c, r), at(c, r + 1)
            cells.append(f"\x1b[38;2;{a};{b};{d}m\x1b[48;2;{e};{f};{g}m▀")
        lines.append("".join(cells) + "\x1b[0m")
    return lines


def read_picture(path):
    import sight
    with open(path, "rb") as f:
        return sight.read_png(f.read())


def side_by_side(art, text, columns):
    width = shutil.get_terminal_size((100, 24)).columns
    room = width - columns - 3
    wrapped = []
    for line in text:
        wrapped += textwrap.wrap(line, max(20, room), subsequent_indent="  ") or [""]
    if room < 24:
        return art + wrapped
    return [(art[k] if k < len(art) else " " * columns) + "   " + (wrapped[k] if k < len(wrapped) else "")
            for k in range(max(len(art), len(wrapped)))]


def show(path, text, columns=40, open_it=False, pixels=None):
    """Draw the picture in the terminal with its text beside it (or just the text and the path when the output
    isn't a terminal); --open opens the PNG too."""
    if sys.stdout.isatty() and (pixels or (path and os.path.isfile(path))):
        w, h, px = pixels or read_picture(path)
        print("\n".join(side_by_side(halfblock(w, h, px, columns), text, columns)))
    else:
        print("\n".join(text))
        if path:
            print(f"  picture: {path}")
    if open_it and path and shutil.which("xdg-open"):
        subprocess.Popen(["xdg-open", path], stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)


def sheet(items, columns=32):
    """Several pictures in a row, with a caption under each (brick dream)."""
    if not sys.stdout.isatty():
        for item in items:
            print(f"  {item['words']} -> eye: {item['eye']}  ({item['path']})")
        return
    width = shutil.get_terminal_size((100, 24)).columns
    per_row = max(1, width // (columns + 2))
    for k in range(0, len(items), per_row):
        row = items[k:k + per_row]
        arts = [halfblock(*i["pixels"], columns) for i in row]
        for n in range(max(len(a) for a in arts)):
            print("  ".join(a[n] if n < len(a) else " " * columns for a in arts))
        for caption in ("words", "eye"):
            print("  ".join((("eye: " if caption == "eye" else "") + i[caption])[:columns].ljust(columns) for i in row))


# ---------------- brick imagine / brick dream / brick imaginations ----------------

def learning_on():
    import brain
    return not brain.off()


def main(args):
    if not args:
        print(__doc__)
        return 1
    open_it = "--open" in args
    args = [a for a in args if a != "--open"]
    command, rest = args[0], args[1:]
    if command == "dream":
        return dream_command(rest, open_it)
    if command == "imaginations":
        return history_command(rest, open_it)
    teach = learning_on()
    rng = random.Random()
    if not rest or re.fullmatch(r"\d+", rest[0]) and len(rest) == 1:
        count = min(50, int(rest[0])) if rest else 5
        tally = Tally()
        for _ in range(count):
            item = imagine_picture(random_recipe(rng), "you", f"{ORIGINS['you']}: brick imagine", tally, rng,
                                   teach=teach)
            text = [f"imagined: {item['words']}", f"eye saw:  {item['eye']}  ({item['right']} of {item['scored']} right)",
                    "could be: " + (item["could text"] or "nothing brick knows")]
            text += [f"learned:  {line}" for line in item["learned"]]
            show(item["path"], text, 40, open_it, item["pixels"])
            print()
        tally.save()
        print("The eye so far: " + ", ".join(tally.summary()))
        print(f"(pictures in {PICTURES}; tallies in {TALLY})")
        return 0
    words = " ".join(rest)
    if "+" in words:
        a, b = (part.strip() for part in words.split("+", 1))
        if not symbols.slug(a) or not symbols.slug(b):
            print("Blend which two? For example: brick imagine apple + ball")
            return 1
        item = imagine_blend(a, b, "you", teach=teach, rng=rng)
        text = [f"blended:  {a} + {b} -> a {item['looks']}", f"drew:     {item['words']}",
                f"eye saw:  {item['eye']}", "could be: " + (item["could text"] or "nothing brick knows")]
        text += [f"learned:  {line}" for line in item["learned"]]
        show(item["path"], text, 40, open_it, item["pixels"])
        return 0
    name = symbols.clean(words)
    if not symbols.slug(name):
        print("Imagine what? For example: brick imagine apple")
        return 1
    item = imagine_named(name, "you", rng=rng)
    why = item["traits"]["why"]
    eye_name = item["eye_name"]
    text = [f"drew:     {item['words']}",
            "from:     " + ("; ".join(why) if why else f"nothing known about how {name} looks (drew a grey blob)"),
            f"eye saw:  {item['eye']}",
            "memory:   " + (", ".join(f"{n} ({s})" for n, s in item["found"].get("matches", [])) or "nothing seen before"),
            "could be: " + (could_be(item["seen_could"]) or "nothing brick knows"),
            (f"matched:  yes, {item['how']}" if item["matched"] else
             f"matched:  no, the eye didn't read it back as {name}" + (f" (memory said {eye_name})" if eye_name else ""))]
    show(item["path"], text, 40, open_it, item["pixels"])
    return 0


def dream_command(rest, open_it=False):
    count = min(500, int(rest[0])) if rest and rest[0].isdigit() else 10
    started = time.monotonic()
    rng = random.Random()
    pictures = 0
    for k in range(count):
        line, items = dream(ROUND_BUDGET, PER_ROUND, origin="you", rng=rng,
                            label=f"{ORIGINS['you']}: brick dream")
        pictures += len(items)
        print(f"round {k + 1}: {line}")
        sheet(items)
        if open_it and items:
            show(items[-1]["path"], [], open_it=True)
    tally = Tally()
    print(f"\nDreamt {pictures} pictures in {count} rounds ({time.monotonic() - started:.1f}s).")
    print("The eye, right per channel: " + (", ".join(tally.summary()) or "nothing yet"))
    worst = tally.worst("shape")
    if worst:
        print("It finds hardest: " + "; ".join(worst))
    if tally.notes["tuned"]:
        print("Tuned: " + tally.notes["tuned"][0])
    if tally.notes["learned"]:
        print("Learned lately: " + tally.notes["learned"][0])
    print(f"(tallies: {TALLY}; pictures and log: {PICTURES})")
    return 0


def history_command(rest, open_it=False):
    picks = {"own": "own", "mine": "you", "you": "you", "learned": "learned", "learning": "learned"}
    origin = next((picks[w] for w in rest if w in picks), None)
    today = "today" in rest
    most = next((int(w) for w in rest if w.isdigit()), 10)
    found = entries(origin, today, most)
    if not found:
        print("Nothing imagined yet" + (" like that" if origin or today else "") +
              ". brick imagine makes some; brick dreams by itself while it runs.")
        return 0
    for e in reversed(found):
        text = [f"{e['time']}  {e['label']}", f"imagined: {e['recipe']}", f"eye saw:  {e['eye']}",
                f"could be: {e['could be']}"]
        if e["exists"]:
            show(e["path"], text, 40, open_it)
        else:
            print("\n".join(text) + ("\n  (picture no longer kept)" if e["picture"] else ""))
        print()
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]) or 0)
