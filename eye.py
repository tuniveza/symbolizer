"""brick's eye: many quick filters, each turning a picture's pixels into symbols, then memory naming it.

Like the channels of the visual cortex, each filter looks at one thing, on a small copy of the picture
(LONGEST pixels across at most), with the standard library only:

  objects    the thing(s) apart from the background: colours sorted into a few groups (k-means), the groups
             along the border taken as background, the rest split into connected pieces -> how many, where
             each is (top-left, middle ...), how big, and which is left of / above which
  colour     the main colours and their shares, warm or cool, bright or dark, vivid / muted / pale, contrast
  shape      roundness (outline against an ellipse), how long and which way it points, how solid (area
             against its convex hull), holes, corners (the hull simplified) -> round, oval, triangle,
             square, rectangle, star, ring, blob, long, curved; mirror and turning symmetry; a moment
             signature (Hu) that stays the same however it's moved, turned or scaled
  edges      how many edges (Sobel), which way the lines run, straight or curved
  texture    smooth, grainy or rough (local binary patterns), and repeats along rows and columns ->
             striped, dotted, checked
  layout     a subject in the middle or a spread-out scene, sky-like top, ground-like bottom, a horizon,
             how full the picture is; the scene itself ("sky above ground") is a context, a symbol of its own
  text       words written in the picture, read by tesseract when it's installed (and there are edges to read)
  memory     the picture's symbols compared with every universe that has been seen or knows how it looks
             (likeness = sum of min / sum of max of the two patterns as shares, as in brain.py; each symbol
             weighted by 1 / (1 + floor(log2(its breadth))), so broad symbols don't drown out specific ones):
             "looks like apple (0.82), ball (0.41)". Sure and well ahead of the next: that's the answer.
  model      only when memory isn't sure: the vision model on this computer is asked for a name and facets

    see(path, name=None, model=True) -> everything found (a dict; summary(found) puts it in words)
      (memory=False / read=False leave out the memory and text channels: the imagination's quick read-back)

The shape channel's thresholds (how round is round, how hollow a star ...) are DEFAULTS below. brick's
imagination (imagine.py) draws pictures whose shapes it knows, sees how often the eye reads them right, and,
where a threshold within its safe range (TUNABLE) reads them better, writes it to texts/factory/
eye-settings.txt ("star solidity: 0.84"), which the eye reads here. Delete that file to go back to DEFAULTS.
"""
import base64
import colorsys
import json
import math
import os
import re
import shutil
import subprocess
import tempfile
import time
from collections import Counter, deque

import sight

LONGEST = 160
VISUAL = ("shape", "colour", "tone", "texture", "pattern", "edges", "mathematics", "layout", "position", "objects",
          "size", "text")
WEIGHT = {"shape": 1.5, "colour": 1.5, "pattern": 1.2, "text": 1.5, "tone": 1.0, "texture": 1.0,
          "mathematics": 1.0, "edges": 0.7, "size": 0.5, "layout": 0.5, "position": 0.3, "objects": 0.3}
SURE = 0.3                  # memory's best likeness must reach this,
AHEAD = 1.25                # and be this many times the next one's, to name a picture without the model
HERE = os.path.dirname(os.path.abspath(__file__))
TUNED = os.path.join(HERE, "texts", "factory", "eye-settings.txt")
TUNABLE = {                 # name: (default, lowest, highest) -- the imagination tunes these within the range
    "ring hole": (0.08, 0.04, 0.16), "ring fit": (0.6, 0.45, 0.75), "ring elongation": (1.3, 1.15, 1.5),
    "round fit": (0.8, 0.72, 0.88), "round solidity": (0.9, 0.85, 0.96), "round elongation": (1.15, 1.06, 1.3),
    "star solidity": (0.8, 0.65, 0.9), "star points": (4, 3, 6), "star elongation": (1.5, 1.3, 1.8),
    "polygon solidity": (0.9, 0.82, 0.96), "square sides": (1.2, 1.08, 1.4), "roundish fit": (0.6, 0.45, 0.75),
    "roundish solidity": (0.85, 0.75, 0.92), "long": (2.5, 2.0, 3.2)}
DEFAULTS = {name: value[0] for name, value in TUNABLE.items()}
POSITIONS = [["top-left", "top", "top-right"], ["left", "middle", "right"], ["bottom-left", "bottom", "bottom-right"]]
NOUNS = {"round": "round thing", "oval": "oval", "triangle": "triangle", "square": "square", "rectangle": "rectangle",
         "star": "star", "ring": "ring", "pentagon": "pentagon", "hexagon": "hexagon", "long": "long thing",
         "blob": "blob", "roundish": "roundish thing"}


# ---------------- the picture, small ----------------

class Picture:
    def __init__(self, path, longest=LONGEST):
        self.path = path
        started = time.perf_counter()
        self.width, self.height, pixels, self.original = load(path, longest)
        self.rgb = [p[:3] for p in pixels]
        self.alpha = [p[3] for p in pixels]
        self.lum = [(299 * r + 587 * g + 114 * b) // 1000 for r, g, b in self.rgb]
        self.seconds = time.perf_counter() - started

    def border(self):
        w, h = self.width, self.height
        return sorted(set(list(range(w)) + list(range((h - 1) * w, h * w)) + list(range(0, h * w, w)) +
                          list(range(w - 1, h * w, w))))


def magick():
    return shutil.which("magick") or shutil.which("convert")


def load(path, longest):
    """-> (width, height, [(r, g, b, a)], the original size), shrunk to longest across."""
    with open(path, "rb") as f:
        data = f.read()
    png = data[:8] == b"\x89PNG\r\n\x1a\n"
    if png and max(sight.png_size(data)) <= longest:
        try:
            w, h, px = sight.read_png(data)
            return w, h, px, (w, h)
        except sight.CantSee:
            pass
    if data[:2] in (b"P2", b"P3", b"P5", b"P6") and not magick():
        w, h, px = sight.read_ppm(data)
        return (*shrink(w, h, px, longest), (w, h))
    tool = magick()
    if not tool:
        if png:
            w, h, px = sight.read_png(data)
            return (*shrink(w, h, px, longest), (w, h))
        raise sight.CantSee("brick reads PNG and PPM itself; for other pictures it needs ImageMagick, which "
                            "isn't installed (sudo pacman -S imagemagick), or save the picture as a PNG")
    with tempfile.TemporaryDirectory() as folder:
        out = os.path.join(folder, "small.png")
        done = subprocess.run([tool, "-define", f"jpeg:size={2 * longest}x{2 * longest}", path + "[0]", "-auto-orient", "-print", "%w %h\n", "-resize",
                               f"{longest}x{longest}>", "-interlace", "none", "-depth", "8", "PNG32:" + out],
                              capture_output=True, text=True, timeout=60)
        if done.returncode or not os.path.isfile(out):
            raise sight.CantSee(f"ImageMagick couldn't read it ({done.stderr.strip()[:120] or 'no reason given'})")
        size = tuple(int(n) for n in done.stdout.split()[:2]) if len(done.stdout.split()) >= 2 else None
        if data[:3] == b"\xff\xd8\xff":
            real = jpeg_size(data)
            if real and size and (size[0] > size[1]) != (real[0] > real[1]):
                real = real[::-1]                                  # turned upright by its EXIF note
            size = real or size
        with open(out, "rb") as f:
            w, h, px = sight.read_png(f.read())
    return w, h, px, size or (w, h)


def jpeg_size(data):
    """A JPEG's real width and height, from its frame header (magick reads it smaller on purpose)."""
    at = 2
    while at + 9 < len(data):
        if data[at] != 0xFF:
            at += 1
            continue
        marker = data[at + 1]
        if marker in (0xC0, 0xC1, 0xC2, 0xC3, 0xC5, 0xC6, 0xC7, 0xC9, 0xCA, 0xCB, 0xCD, 0xCE, 0xCF):
            h, w = int.from_bytes(data[at + 5:at + 7], "big"), int.from_bytes(data[at + 7:at + 9], "big")
            return w, h
        if marker in (0xD8, 0x01) or 0xD0 <= marker <= 0xD7:
            at += 2
            continue
        at += 2 + int.from_bytes(data[at + 2:at + 4], "big")
    return None


def shrink(w, h, px, longest):
    """Nearest-pixel shrinking, for when there's no ImageMagick."""
    if max(w, h) <= longest:
        return w, h, px
    scale = max(w, h) / longest
    nw, nh = max(1, round(w / scale)), max(1, round(h / scale))
    return nw, nh, [px[min(h - 1, int(y * scale)) * w + min(w - 1, int(x * scale))]
                    for y in range(nh) for x in range(nw)]


_tuned = [None, None]


def tuned():
    """The shape thresholds: DEFAULTS, with any tuned value from texts/factory/eye-settings.txt (kept within
    TUNABLE's range; a line that doesn't read is ignored)."""
    try:
        when = os.path.getmtime(TUNED)
    except OSError:
        return dict(DEFAULTS)
    if _tuned[0] != (TUNED, when):
        values = dict(DEFAULTS)
        try:
            with open(TUNED, encoding="utf-8") as f:
                for line in f:
                    found = re.match(r"^\s*([a-z ]+?)\s*:\s*(-?\d+(?:\.\d+)?)", line)
                    if found and found.group(1) in TUNABLE:
                        _, low, high = TUNABLE[found.group(1)]
                        values[found.group(1)] = min(high, max(low, float(found.group(2))))
        except OSError:
            pass
        _tuned[:] = [(TUNED, when), values]
    return dict(_tuned[1])


def save_tuned(values, notes=()):
    """Write the tuned thresholds (only the ones that differ from DEFAULTS) to texts/factory/eye-settings.txt."""
    lines = ["# The eye's shape thresholds, tuned by brick's imagination (imagine.py): it draws shapes it knows",
             "# and keeps a value here only when the eye reads them better with it. Delete this file to go back to",
             "# the defaults (eye.py, DEFAULTS). You can change a number by hand; it stays within the safe range."]
    lines += [f"# {note}" for note in notes]
    for name, value in values.items():
        if name in TUNABLE and value != DEFAULTS[name]:
            lines.append(f"{name}: {round(value, 3):g}   # default {DEFAULTS[name]:g}")
    os.makedirs(os.path.dirname(TUNED), exist_ok=True)
    with open(TUNED + ".new", "w", encoding="utf-8") as f:
        f.write("\n".join(lines) + "\n")
    os.replace(TUNED + ".new", TUNED)


def put(facets, facet, value, strength=2.0):
    found = facets.setdefault(facet, [])
    if value not in [v for v, _ in found]:
        found.append((value, round(strength, 2)))


# ---------------- objects: the thing(s) apart from the background ----------------

class Thing:
    """One object in the picture: its pixels (holes filled), where it is, and what it looks like."""
    def __init__(self, pixels, width, height, holes, hole_area, solid=None):
        self.pixels, self.holes, self.hole_area = pixels, holes, hole_area
        self.solid = solid or pixels                # without its holes: what its colour is taken from
        xs, ys = [k % width for k in pixels], [k // width for k in pixels]
        self.xs, self.ys = xs, ys
        self.area = len(pixels)
        self.box = (min(xs), min(ys), max(xs), max(ys))
        self.cx, self.cy = sum(xs) / self.area, sum(ys) / self.area
        column = min(2, int(3 * self.cx / width))
        row = min(2, int(3 * self.cy / height))
        self.position = POSITIONS[row][column]
        self.share = self.area / (width * height)
        self.shape, self.colour = "blob", "grey"

    def name(self):
        return f"{self.colour} {NOUNS.get(self.shape, self.shape)}"


def kmeans(pic, k=5, rounds=8):
    """Colours sorted into k groups -> (label of each pixel, the groups' colours)."""
    counts = Counter(((r >> 3) << 10) | ((g >> 3) << 5) | (b >> 3) for r, g, b in pic.rgb)
    keys = list(counts)
    colour = {key: ((key >> 10) * 8 + 4, ((key >> 5) & 31) * 8 + 4, (key & 31) * 8 + 4) for key in keys}
    border = Counter(pic.rgb[i] for i in pic.border())
    centres = [border.most_common(1)[0][0]]
    while len(centres) < min(k, len(keys)):                 # farthest-point start: the same every time
        far = max(keys, key=lambda key: min(sum((a - b) ** 2 for a, b in zip(colour[key], c)) for c in centres))
        if min(sum((a - b) ** 2 for a, b in zip(colour[far], c)) for c in centres) < 400:
            break
        centres.append(colour[far])
    for _ in range(rounds):
        sums = [[0, 0, 0, 0] for _ in centres]
        for key in keys:
            c = colour[key]
            best = min(range(len(centres)), key=lambda j: (c[0] - centres[j][0]) ** 2 + (c[1] - centres[j][1]) ** 2 +
                       (c[2] - centres[j][2]) ** 2)
            n = counts[key]
            s = sums[best]
            s[0] += c[0] * n
            s[1] += c[1] * n
            s[2] += c[2] * n
            s[3] += n
        centres = [(s[0] / s[3], s[1] / s[3], s[2] / s[3]) if s[3] else centres[j] for j, s in enumerate(sums)]
    group = {}
    for key in keys:
        c = colour[key]
        group[key] = min(range(len(centres)), key=lambda j: (c[0] - centres[j][0]) ** 2 + (c[1] - centres[j][1]) ** 2 +
                         (c[2] - centres[j][2]) ** 2)
    labels = [group[((r >> 3) << 10) | ((g >> 3) << 5) | (b >> 3)] for r, g, b in pic.rgb]
    return labels, centres


def pieces(mask, width, height, smallest):
    """Connected pieces (4 neighbours) of a mask -> [[pixel index]], biggest first, at least smallest."""
    seen = bytearray(len(mask))
    found = []
    for start in range(len(mask)):
        if not mask[start] or seen[start]:
            continue
        seen[start] = 1
        queue, piece = deque([start]), []
        while queue:
            k = queue.popleft()
            piece.append(k)
            x = k % width
            for n in (k - 1 if x else -1, k + 1 if x < width - 1 else -1, k - width, k + width):
                if 0 <= n < len(mask) and mask[n] and not seen[n]:
                    seen[n] = 1
                    queue.append(n)
        if len(piece) >= smallest:
            found.append(piece)
    return sorted(found, key=len, reverse=True)


def filled(piece, width, height):
    """A piece with its holes filled -> (pixels, number of holes, hole area)."""
    xs, ys = [k % width for k in piece], [k // width for k in piece]
    x0, y0, x1, y1 = min(xs) - 1, min(ys) - 1, max(xs) + 1, max(ys) + 1
    bw, bh = x1 - x0 + 1, y1 - y0 + 1
    inside = bytearray(bw * bh)
    for x, y in zip(xs, ys):
        inside[(y - y0) * bw + (x - x0)] = 1
    outside = bytearray(bw * bh)
    queue = deque([0])
    outside[0] = 1
    while queue:
        k = queue.popleft()
        x = k % bw
        for n in (k - 1 if x else -1, k + 1 if x < bw - 1 else -1, k - bw, k + bw):
            if 0 <= n < len(inside) and not inside[n] and not outside[n]:
                outside[n] = 1
                queue.append(n)
    holes_mask = [not inside[k] and not outside[k] for k in range(bw * bh)]
    holes = [h for h in pieces(holes_mask, bw, bh, max(3, len(piece) // 100))]
    full = list(piece) + [(k // bw + y0) * width + (k % bw + x0) for k in range(bw * bh) if holes_mask[k]]
    return full, len(holes), sum(len(h) for h in holes)


def objects(pic):
    """-> (facets, note, [Thing], background mask)."""
    w, h, n = pic.width, pic.height, pic.width * pic.height
    labels, centres = kmeans(pic)
    border = pic.border()
    on_border = Counter(labels[i] for i in border)
    back_groups = {g for g, c in on_border.items() if c >= 0.2 * len(border)} or {on_border.most_common(1)[0][0]}
    see_through = sum(1 for i in border if pic.alpha[i] < 128) > len(border) / 2
    if see_through:
        back = [a < 128 for a in pic.alpha]
    else:
        back = [labels[i] in back_groups or pic.alpha[i] < 128 for i in range(n)]
    found = pieces([not b for b in back], w, h, max(12, n // 100))
    facets, things = {}, []
    if sum(len(p) for p in found) < 0.002 * n or sum(back) < 0.15 * n:
        put(facets, "objects", "no single subject")
        return facets, "no thing apart from the background: a spread-out scene", [], back
    for piece in found[:6]:
        pixels, holes, hole_area = filled(piece, w, h)
        things.append(Thing(pixels, w, h, holes, hole_area, piece))
    count = len(things)
    put(facets, "objects", ["one thing", "two things", "three things"][count - 1] if count <= 3 else "many things")
    main = things[0]
    put(facets, "position", main.position)
    put(facets, "size", "big" if main.share >= 0.3 else "medium-sized" if main.share >= 0.06 else "small")
    return facets, f"{count} thing{'s' * (count > 1)}, the biggest at the {main.position} " \
                   f"({round(100 * main.share)}% of the picture)", things, back


def relations(things):
    """'red round thing left of blue square', for the biggest few things."""
    facets = {}
    for thing in things[1:4]:
        a, b = things[0], thing
        dx, dy = b.cx - a.cx, b.cy - a.cy
        if abs(dx) >= abs(dy):
            put(facets, "objects", f"{a.name()} {'left of' if dx > 0 else 'right of'} {b.name()}")
        else:
            put(facets, "objects", f"{a.name()} {'above' if dy > 0 else 'below'} {b.name()}")
        put(facets, "objects", f"{b.name()} at the {b.position}")
    if len(things) > 1:
        put(facets, "objects", f"{things[0].name()} at the {things[0].position}")
        if things[0].area >= 2 * things[1].area:
            put(facets, "objects", f"{things[0].name()} bigger than {things[1].name()}")
    return facets


# ---------------- colour ----------------

def colour(pic, things, back):
    facets = {}
    region = things[0].solid if things else range(len(pic.rgb))
    step = max(1, len(region) // 6000)
    sample = [pic.rgb[k] for k in list(region)[::step]]
    names = Counter(sight.colour_name(*p) for p in sample)
    shares = [(name, count / len(sample)) for name, count in names.most_common(4) if count / len(sample) >= 0.1]
    if not shares:
        shares = [names.most_common(1)[0][0:1] + (1.0,)]
    for name, share in shares:
        put(facets, "colour", name, 1 + 2 * share)
    top = shares[0][0]
    chosen = [p for p in sample if sight.colour_name(*p) == top] or sample
    average = tuple(round(sum(p[k] for p in chosen) / len(chosen)) for k in range(3))
    fine = sight.fine_name(*average)
    if fine and fine not in [n for n, _ in shares]:
        put(facets, "colour", fine, 1.5)
    for thing in things:
        part = [pic.rgb[k] for k in thing.solid[::max(1, len(thing.solid) // 800)]]
        thing.colour = Counter(sight.colour_name(*p) for p in part).most_common(1)[0][0]
    hsv = [colorsys.rgb_to_hsv(r / 255, g / 255, b / 255) for r, g, b in sample]
    lit = [s for h, s, v in hsv if v > 0.2]
    vivid = [h for h, s, v in hsv if s > 0.25 and v > 0.2]
    warm = sum(1 for h in vivid if h < 70 / 360 or h > 300 / 360)
    if vivid and len(vivid) >= 0.2 * len(hsv):
        if warm >= 0.65 * len(vivid):
            put(facets, "tone", "warm")
        elif warm <= 0.35 * len(vivid):
            put(facets, "tone", "cool")
    value = sum(v for _, _, v in hsv) / len(hsv)
    saturation = sum(lit) / len(lit) if lit else 0
    if value > 0.75:
        put(facets, "tone", "bright")
    elif value < 0.3:
        put(facets, "tone", "dark")
    if saturation > 0.5:
        put(facets, "tone", "vivid")
    elif saturation < 0.08:
        put(facets, "tone", "colourless")
    elif saturation < 0.25:
        put(facets, "tone", "pale" if value > 0.6 else "muted")
    lum = pic.lum[::max(1, len(pic.lum) // 6000)]
    mean = sum(lum) / len(lum)
    spread = math.sqrt(sum((v - mean) ** 2 for v in lum) / len(lum)) / 255
    if spread > 0.3:
        put(facets, "tone", "high contrast")
    elif spread < 0.08:
        put(facets, "tone", "low contrast")
    note = ", ".join(f"{name} {round(100 * share)}%" for name, share in shares)
    return facets, note + (", " + ", ".join(v for v, _ in facets.get("tone", [])) if facets.get("tone") else "")


# ---------------- shape ----------------

def hull(points):
    points = sorted(set(points))
    if len(points) < 3:
        return points

    def cross(o, a, b):
        return (a[0] - o[0]) * (b[1] - o[1]) - (a[1] - o[1]) * (b[0] - o[0])

    lower, upper = [], []
    for p in points:
        while len(lower) >= 2 and cross(lower[-2], lower[-1], p) <= 0:
            lower.pop()
        lower.append(p)
    for p in reversed(points):
        while len(upper) >= 2 and cross(upper[-2], upper[-1], p) <= 0:
            upper.pop()
        upper.append(p)
    return lower[:-1] + upper[:-1]


def polygon_area(poly):
    return abs(sum(poly[i][0] * poly[i - 1][1] - poly[i - 1][0] * poly[i][1] for i in range(len(poly)))) / 2


def simplify(poly, eps):
    """Douglas-Peucker on a closed polygon -> its corners."""
    if len(poly) < 4:
        return poly
    far = max(range(len(poly)), key=lambda i: (poly[i][0] - poly[0][0]) ** 2 + (poly[i][1] - poly[0][1]) ** 2)

    def line(points):
        if len(points) < 3:
            return points
        (ax, ay), (bx, by) = points[0], points[-1]
        length = math.hypot(bx - ax, by - ay) or 1
        worst, at = 0, 0
        for i in range(1, len(points) - 1):
            d = abs((bx - ax) * (ay - points[i][1]) - (ax - points[i][0]) * (by - ay)) / length
            if d > worst:
                worst, at = d, i
        if worst <= eps:
            return [points[0], points[-1]]
        return line(points[:at + 1])[:-1] + line(points[at:])

    first = line(poly[:far + 1])
    second = line(poly[far:] + [poly[0]])
    return first[:-1] + second[:-1]


def hu(thing):
    """The seven Hu moments, as -log10 of their size (the same however the thing is moved, turned or scaled)."""
    a, cx, cy = thing.area, thing.cx, thing.cy
    mu = {}
    for p, q in ((2, 0), (0, 2), (1, 1), (3, 0), (0, 3), (2, 1), (1, 2)):
        mu[p, q] = sum((x - cx) ** p * (y - cy) ** q for x, y in zip(thing.xs, thing.ys)) / a ** (1 + (p + q) / 2)
    n20, n02, n11, n30, n03, n21, n12 = (mu[k] for k in ((2, 0), (0, 2), (1, 1), (3, 0), (0, 3), (2, 1), (1, 2)))
    h = [n20 + n02, (n20 - n02) ** 2 + 4 * n11 ** 2, (n30 - 3 * n12) ** 2 + (3 * n21 - n03) ** 2,
         (n30 + n12) ** 2 + (n21 + n03) ** 2,
         (n30 - 3 * n12) * (n30 + n12) * ((n30 + n12) ** 2 - 3 * (n21 + n03) ** 2) +
         (3 * n21 - n03) * (n21 + n03) * (3 * (n30 + n12) ** 2 - (n21 + n03) ** 2),
         (n20 - n02) * ((n30 + n12) ** 2 - (n21 + n03) ** 2) + 4 * n11 * (n30 + n12) * (n21 + n03),
         (3 * n21 - n03) * (n30 + n12) * ((n30 + n12) ** 2 - 3 * (n21 + n03) ** 2) -
         (n30 - 3 * n12) * (n21 + n03) * (3 * (n30 + n12) ** 2 - (n21 + n03) ** 2)]
    return [round(-math.log10(abs(v)), 1) if abs(v) > 1e-30 else 30.0 for v in h]


def measure_shape(thing, width):
    """Fill in thing.shape and its measures -> {name: value}."""
    inside = set(thing.pixels)
    a, cx, cy = thing.area, thing.cx, thing.cy
    m20 = sum((x - cx) ** 2 for x in thing.xs) / a
    m02 = sum((y - cy) ** 2 for y in thing.ys) / a
    m11 = sum((x - cx) * (y - cy) for x, y in zip(thing.xs, thing.ys)) / a
    half = (m20 + m02) / 2
    root = math.sqrt(((m20 - m02) / 2) ** 2 + m11 ** 2)
    l1, l2 = max(half + root, 0.25), max(half - root, 0.25)
    elongation = math.sqrt(l1 / l2)
    angle = math.degrees(0.5 * math.atan2(2 * m11, m20 - m02))     # 0: lying flat, 90: standing up
    edge = [(x, y) for x, y in zip(thing.xs, thing.ys)
            if not ((y * width + x - 1) in inside and (y * width + x + 1) in inside and
                    ((y - 1) * width + x) in inside and ((y + 1) * width + x) in inside)]
    ca, sa = math.cos(math.radians(angle)), math.sin(math.radians(angle))
    big, small = 2 * math.sqrt(l1), 2 * math.sqrt(l2)
    distances = [math.hypot(((x - cx) * ca + (y - cy) * sa) / big, (-(x - cx) * sa + (y - cy) * ca) / small)
                 for x, y in edge]
    middle = sorted(distances)[len(distances) // 2] or 1
    fit = sum(1 for d in distances if abs(d / middle - 1) < 0.07) / len(distances)
    corners_of = [(x + dx, y + dy) for x, y in edge for dx in (0, 1) for dy in (0, 1)]
    outline = hull(corners_of)
    hull_area = polygon_area(outline) or a
    solidity = min(1.0, a / hull_area)
    perimeter = sum(math.dist(outline[i], outline[i - 1]) for i in range(len(outline)))
    corners = simplify(outline, 0.04 * perimeter)
    # points: the outline's distance from the middle, round the circle, counted where it peaks
    radius = [0.0] * 72
    for x, y in edge:
        k = int((math.degrees(math.atan2(y - cy, x - cx)) % 360) / 5)
        radius[k] = max(radius[k], math.hypot(x - cx, y - cy))
    mean_r = sum(radius) / 72 or 1
    points, high = 0, radius[-1] > 1.12 * mean_r
    for r in radius + radius[:1]:
        if not high and r > 1.12 * mean_r:
            high, points = True, points + 1
        elif high and r < 0.92 * mean_r:
            high = False
    mirror = sum(1 for x, y in zip(thing.xs, thing.ys) if (y * width + round(2 * cx - x)) in inside) / a
    flip = sum(1 for x, y in zip(thing.xs, thing.ys) if (round(2 * cy - y) * width + x) in inside) / a
    turn = sum(1 for x, y in zip(thing.xs, thing.ys)
               if (round(2 * cy - y) * width + round(2 * cx - x)) in inside) / a
    quarter = sum(1 for x, y in zip(thing.xs, thing.ys)
                  if (round(cy + (x - cx)) * width + round(cx - (y - cy))) in inside) / a
    sides = [math.dist(corners[i], corners[i - 1]) for i in range(len(corners))] or [1]
    m = {"fit": fit, "solidity": solidity, "corners": len(corners), "points": points, "elongation": elongation,
         "holes": thing.holes, "hole share": thing.hole_area / a, "sides": max(sides) / max(min(sides), 1)}
    shape = classify(m)
    thing.shape = shape
    return {**m, "shape": shape, "angle": angle, "mirror": mirror, "flip": flip,
            "turn": turn, "quarter": quarter, "ratio": math.sqrt(m20 / m02) if m02 else 1.0,
            "width": thing.box[2] - thing.box[0] + 1, "height": thing.box[3] - thing.box[1] + 1}


def classify(m, settings=None):
    """A shape's measures -> its name, with the thresholds in settings (tuned() when not given)."""
    s = settings or tuned()
    fit, solidity, elongation, corners = m["fit"], m["solidity"], m["elongation"], m["corners"]
    if m["holes"] and m["hole share"] >= s["ring hole"] and fit >= s["ring fit"] and elongation < s["ring elongation"]:
        return "ring"
    if fit >= s["round fit"] and solidity >= s["round solidity"]:
        return "round" if elongation < s["round elongation"] else "oval"
    if solidity < s["star solidity"] and m["points"] >= s["star points"] and elongation < s["star elongation"]:
        return "star"
    if solidity >= s["polygon solidity"] and 3 <= corners <= 6:
        if corners == 4:
            return "square" if m["sides"] < s["square sides"] else "rectangle"
        return {3: "triangle", 5: "pentagon", 6: "hexagon"}[corners]
    if fit >= s["roundish fit"] and solidity >= s["roundish solidity"]:
        return "roundish"
    if elongation >= s["long"]:
        return "long"
    return "blob"


def shape(pic, things):
    facets = {}
    if not things:
        return facets, "skipped: no single thing to measure"
    for thing in things[1:]:
        measure_shape(thing, pic.width)
    m = measure_shape(things[0], pic.width)
    s = m["shape"]
    put(facets, "shape", s)
    maths = {"round": "circle", "oval": "ellipse", "ring": "annulus", "triangle": "triangle", "square": "square",
             "rectangle": "rectangle", "pentagon": "pentagon", "hexagon": "hexagon", "star": "star polygon",
             "roundish": "near circle"}
    if s in maths:
        put(facets, "mathematics", maths[s])
    if s == "star":
        put(facets, "shape", f"{m['points']}-pointed")
    if s in ("triangle", "square", "rectangle", "pentagon", "hexagon"):
        put(facets, "shape", "straight sides")
        put(facets, "mathematics", f"{m['corners']} corners")
    if m["elongation"] >= 1.6:
        put(facets, "shape", "long" if m["elongation"] >= 2.5 else "elongated")
        way = abs(m["angle"])
        put(facets, "shape", "lying flat" if way < 22.5 else "standing up" if way > 67.5 else "slanting")
        if m["solidity"] < 0.8:
            put(facets, "shape", "curved")
    if m["ratio"] < 0.8:
        put(facets, "shape", "tall")
    elif m["ratio"] > 1.25:
        put(facets, "shape", "wide")
    if m["holes"]:
        put(facets, "shape", "has a hole" if m["holes"] == 1 else f"has {m['holes']} holes")
        put(facets, "mathematics", f"euler number {1 - m['holes']}")
    if m["solidity"] < 0.75 and s not in ("star", "ring"):
        put(facets, "shape", "concave")
    if m["mirror"] >= 0.9:
        put(facets, "shape", "symmetric")
        put(facets, "mathematics", "mirror symmetry")
    if m["flip"] >= 0.9:
        put(facets, "mathematics", "top-bottom symmetry")
    if m["quarter"] >= 0.9:
        put(facets, "mathematics", "four-way turning symmetry")
    elif m["turn"] >= 0.9:
        put(facets, "mathematics", "half-turn symmetry")
    ratio = m["ratio"]
    put(facets, "mathematics", f"ratio {ratio:.2f}")
    signature = hu(things[0])
    put(facets, "mathematics", f"hu signature {signature[0]}" + (f" {signature[1]}" if signature[1] < 30 else ""), 1.0)
    wording = f"about {ratio:.2f} times as wide as tall" if ratio >= 1 else \
        f"about {1 / ratio:.2f} times as tall as wide"
    put(facets, "size", wording)
    put(facets, "size", f"{m['width']} by {m['height']} pixels in {os.path.basename(pic.path)}"
        + (f" (shrunk from {pic.original[0]} by {pic.original[1]})" if pic.original != (pic.width, pic.height) else ""))
    things[0].measures = m
    return facets, f"{s}, roundness {m['fit']:.2f}, solidity {m['solidity']:.2f}, {m['corners']} corners, " \
                   f"{m['holes']} holes, hu {signature[:3]}"


# ---------------- edges and lines ----------------

def edges(pic):
    w, h, lum = pic.width, pic.height, pic.lum
    count, bins = 0, [0] * 12
    straight_rows = []
    for y in range(1, h - 1):
        r0, r1, r2 = lum[(y - 1) * w:y * w], lum[y * w:(y + 1) * w], lum[(y + 1) * w:(y + 2) * w]
        for x in range(1, w - 1):
            gx = r0[x + 1] + 2 * r1[x + 1] + r2[x + 1] - r0[x - 1] - 2 * r1[x - 1] - r2[x - 1]
            gy = r2[x - 1] + 2 * r2[x] + r2[x + 1] - r0[x - 1] - 2 * r0[x] - r0[x + 1]
            if abs(gx) + abs(gy) >= 160:
                count += 1
                bins[int((math.degrees(math.atan2(gy, gx)) % 180) / 15) % 12] += 1
    del straight_rows
    facets = {}
    inner = max(1, (w - 2) * (h - 2))
    density = count / inner
    put(facets, "edges", "many edges" if density > 0.15 else "some edges" if density > 0.04 else "few edges")
    if count >= 20:
        flat = (bins[5] + bins[6]) / count           # brightness changing up-down: lines lying flat
        upright = (bins[0] + bins[11]) / count       # changing left-right: upright lines
        if flat + upright >= 0.7 and min(flat, upright) >= 0.2:
            put(facets, "edges", "horizontal and vertical lines")
        elif flat >= 0.5:
            put(facets, "edges", "horizontal lines")
        elif upright >= 0.5:
            put(facets, "edges", "vertical lines")
        elif (bins[2] + bins[3] + bins[8] + bins[9]) / count >= 0.5:
            put(facets, "edges", "diagonal lines")
        top_two = sum(sorted(bins)[-2:]) / count
        top_three = sum(sorted(bins)[-3:]) / count
        if top_two >= 0.55 or top_three >= 0.75:
            put(facets, "edges", "straight lines")
        elif top_two <= 0.35:
            put(facets, "edges", "curved lines")
    return facets, f"{round(100 * density)}% of the picture is edges" + \
        (", " + ", ".join(v for v, _ in facets["edges"][1:]) if len(facets["edges"]) > 1 else ""), density


# ---------------- texture and pattern ----------------

def periodic(signal):
    """Does this row of brightness repeat? -> the repeat length, or 0."""
    n = len(signal)
    mean = sum(signal) / n
    s = [v - mean for v in signal]
    energy = sum(v * v for v in s)
    if energy < 25 * n:                           # flat: nothing to repeat
        return None
    dipped = False
    for lag in range(2, min(n // 2, 40)):
        c = sum(s[i] * s[i + lag] for i in range(n - lag)) / energy * n / (n - lag)
        if c <= 0:
            dipped = True
        elif dipped and c >= 0.5:
            return lag
    return 0


def texture(pic, things):
    w, h, lum = pic.width, pic.height, pic.lum
    facets = {}
    region = set(things[0].pixels) if things and things[0].share < 0.85 else None
    flat = uniform = busy = total = 0
    step = 2 if w * h > 6000 else 1
    for y in range(1, h - 1, step):
        for x in range(1, w - 1, step):
            k = y * w + x
            if region is not None and k not in region:
                continue
            c = lum[k]
            ring = (lum[k - w - 1], lum[k - w], lum[k - w + 1], lum[k + 1], lum[k + w + 1], lum[k + w], lum[k + w - 1],
                    lum[k - 1])
            bits = [1 if v > c + 6 else 0 for v in ring]
            total += 1
            if all(abs(v - c) <= 6 for v in ring):
                flat += 1
                continue
            changes = sum(bits[i] != bits[i - 1] for i in range(8))
            if changes <= 2:
                uniform += 1                       # an edge or a corner
            else:
                busy += 1                          # speckle: grain
    if total:
        if flat / total >= 0.7:
            put(facets, "texture", "smooth")
        elif busy / total >= 0.3:
            put(facets, "texture", "rough")
        elif busy / total >= 0.12:
            put(facets, "texture", "grainy")
        else:
            put(facets, "texture", "fairly smooth")
    if things and things[0].share < 0.85:
        x0, y0, x1, y1 = things[0].box
    else:
        x0, y0, x1, y1 = 0, 0, w - 1, h - 1
    rows = [periodic(lum[y * w + x0:y * w + x1 + 1]) for y in range(y0, y1 + 1, max(1, (y1 - y0) // 30))]
    cols = [periodic([lum[y * w + x] for y in range(y0, y1 + 1)]) for x in range(x0, x1 + 1, max(1, (x1 - x0) // 30))]

    def repeats(found):
        busy_lines = [f for f in found if f is not None]
        return len(busy_lines) >= 3 and sum(1 for f in busy_lines if f) >= 0.6 * len(busy_lines)

    across, down = repeats(rows), repeats(cols)
    if across or down:
        part = [lum[y * w + x] for y in range(y0, y1 + 1) for x in range(x0, x1 + 1)]
        mean = sum(part) / len(part)
        dark = sum(1 for v in part if v < mean) / len(part)
        if across and down:
            put(facets, "pattern", "dotted" if min(dark, 1 - dark) < 0.35 else "checked")
        else:
            put(facets, "pattern", "striped")
            put(facets, "pattern", "vertical stripes" if across else "horizontal stripes")
        put(facets, "pattern", "repeating")
    return facets, ", ".join(v for f in facets.values() for v, _ in f) or "nothing clear"


# ---------------- scene layout ----------------

def layout(pic, things, back):
    w, h = pic.width, pic.height
    facets, contexts = {}, []
    if things:
        main = things[0]
        if main.position == "middle" and main.share >= 0.05:
            put(facets, "layout", "centred subject")
        else:
            put(facets, "layout", "subject off-centre")
        behind = [pic.rgb[k] for k in range(len(back)) if back[k]][::7]
        if behind:
            name = Counter(sight.colour_name(*p) for p in behind).most_common(1)[0][0]
            lum = [(299 * r + 587 * g + 114 * b) // 1000 for r, g, b in behind]
            mean = sum(lum) / len(lum)
            plain = math.sqrt(sum((v - mean) ** 2 for v in lum) / len(lum)) < 18
            put(facets, "layout", f"{'plain ' if plain else ''}{name} background")
        share = sum(t.share for t in things)
        if share < 0.1:
            put(facets, "layout", "mostly empty")
        elif share > 0.6:
            put(facets, "layout", "fills the picture")
    else:
        put(facets, "layout", "spread-out scene")
    band = max(1, h // 4)

    def describe(rows):
        part = [pic.rgb[y * w + x] for y in rows for x in range(0, w, 2)]
        hsv = [colorsys.rgb_to_hsv(r / 255, g / 255, b / 255) for r, g, b in part]
        blue = sum(1 for hh, s, v in hsv if 0.5 <= hh <= 0.72 and s > 0.15 and v > 0.35) / len(hsv)
        pale = sum(1 for hh, s, v in hsv if s < 0.15 and v > 0.75) / len(hsv)
        green = sum(1 for hh, s, v in hsv if 0.17 <= hh <= 0.45 and s > 0.2) / len(hsv)
        earth = sum(1 for hh, s, v in hsv if (hh < 0.12 or hh > 0.95) and 0.2 < s < 0.8 and v < 0.7) / len(hsv)
        return blue, pale, green, earth

    blue, pale, _, _ = describe(range(0, band))
    _, _, green, earth = describe(range(h - band, h))
    sky = blue >= 0.5 or blue + pale >= 0.75 and blue >= 0.2
    ground = green >= 0.4 or earth >= 0.4 or green + earth >= 0.5
    if sky:
        put(facets, "layout", "sky-like top")
    if ground:
        put(facets, "layout", "ground-like bottom")
    rows = [tuple(sum(pic.rgb[y * w + x][c] for x in range(0, w, 2)) / len(range(0, w, 2)) for c in range(3))
            for y in range(h)]
    k = max(2, h // 10)
    best, at = 0.0, None
    for y in range(max(k, int(0.15 * h)), min(h - k, int(0.85 * h)) + 1):
        above = [sum(rows[j][c] for j in range(y - k, y)) / k for c in range(3)]
        below = [sum(rows[j][c] for j in range(y, y + k)) / k for c in range(3)]
        d = math.dist(above, below)
        if d > best:
            best, at = d, y
    if at is not None and best >= 60 and not things:
        where = "high" if at < 0.4 * h else "low" if at > 0.6 * h else "in the middle"
        put(facets, "layout", f"horizon {where}")
    if sky and ground:
        contexts.append("sky above ground")
    elif sky:
        contexts.append("under the sky")
    if things:
        contexts.append(f"a picture with {things[0].colour} and {things[0].shape}")
    return facets, ", ".join(v for v, _ in facets.get("layout", [])), contexts


# ---------------- text ----------------

class Reader:
    """tesseract reading the picture's writing in the background, while the other filters work: started as soon
    as the picture is open, stopped if the edges say there's no writing to read."""
    def __init__(self, pic, off=False):
        self.pic, self.processes, self.why = pic, [], None
        if off:
            self.why = "not asked"
        elif os.environ.get("BRICK_EYE_TEXT") == "off":
            self.why = "switched off here"
        elif not shutil.which("tesseract"):
            self.why = "skipped: tesseract isn't installed, so no writing was read (sudo pacman -S tesseract " \
                       "tesseract-data-eng)"
        if self.why:
            return
        try:
            if magick():                        # a 1100-pixel grey copy reads fast, through a pipe
                shrink = subprocess.Popen([magick(), "-define", "jpeg:size=2200x2200", pic.path + "[0]",
                                           "-auto-orient", "-resize", "1100x1100>", "-colorspace", "Gray", "png:-"],
                                          stdout=subprocess.PIPE, stderr=subprocess.DEVNULL)
                read = subprocess.Popen(["tesseract", "stdin", "stdout", "--psm", "11", "-l", "eng", "tsv"],
                                        stdin=shrink.stdout, stdout=subprocess.PIPE, stderr=subprocess.DEVNULL, text=True)
                shrink.stdout.close()
                self.processes = [shrink, read]
            else:
                self.processes = [subprocess.Popen(["tesseract", pic.path, "stdout", "--psm", "11", "-l", "eng", "tsv"],
                                                   stdout=subprocess.PIPE, stderr=subprocess.DEVNULL, text=True)]
        except OSError as problem:
            self.why = f"tesseract wouldn't start ({problem})"

    def stop(self):
        for process in self.processes:
            if process.poll() is None:
                process.kill()
            process.wait()

    def words(self, density):
        """-> (facets, note, words): the words read with confidence that WordNet knows."""
        if self.why:
            return {}, self.why, []
        if density < 0.06:
            self.stop()
            return {}, "skipped: too few edges for writing", []
        try:
            out, _ = self.processes[-1].communicate(timeout=20)
        except subprocess.TimeoutExpired:
            self.stop()
            return {}, "tesseract didn't finish in 20 seconds", []
        finally:
            self.stop()
        import symbols
        words = []
        for line in out.splitlines()[1:]:
            parts = line.split("\t")
            if len(parts) >= 12 and parts[11].strip():
                try:
                    sure = float(parts[10])
                except ValueError:
                    continue
                for word in re.findall(r"[A-Za-z][A-Za-z']{2,}", parts[11]):
                    if sure >= 60 and symbols.real_word(word.lower()):     # sure, and a real word: not 'jfa'
                        words.append(word.lower())
        facets = {}
        if words:
            put(facets, "text", "has writing")
            for word in list(dict.fromkeys(words))[:10]:
                put(facets, "text", word, 1.5)
        return facets, (f"read: {' '.join(list(dict.fromkeys(words))[:12])}" if words else "no writing found"), words


def text(reader, density):
    return reader.words(density)


# ---------------- memory: which universe does this look like? ----------------

def pattern(facets):
    """{(facet, symbol): weight} for the symbols that say how a thing looks (none with numbers or file names)."""
    found = {}
    for facet, values in facets.items():
        if facet not in VISUAL:
            continue
        for value, strength in values:
            if re.search(r"\d|\.(png|jpe?g|gif|webp)", value):
                continue
            found[facet, value] = found.get((facet, value), 0) + strength * WEIGHT.get(facet, 1.0)
    return found


def likeness(a, b, weigh=None):
    """Weighted overlap of two patterns as shares: sum of min / sum of max (brain.py's formula). weigh:
    symbol -> how much it counts (symbols.weight: 1 / (1 + floor(log2(breadth)))), so broad symbols don't drown
    out specific ones."""
    if weigh:
        a = {k: w * weigh(k[1]) for k, w in a.items()}
        b = {k: w * weigh(k[1]) for k, w in b.items()}
    ta, tb = sum(a.values()), sum(b.values())
    if not ta or not tb:
        return 0.0
    keys = set(a) | set(b)
    most = sum(max(a.get(k, 0) / ta, b.get(k, 0) / tb) for k in keys)
    return sum(min(a.get(k, 0) / ta, b.get(k, 0) / tb) for k in keys) / most if most else 0.0


def remembered(facets, leave_out=()):
    """-> [(name, likeness)] best first, against every universe that has been seen or knows how it looks."""
    import symbols
    mine = pattern(facets)
    if not mine or not os.path.isdir(symbols.FOLDER):
        return []
    scores = []
    for file in os.listdir(symbols.FOLDER):
        if not file.endswith(".txt") or file.startswith(("_", "letter-", "context-")):
            continue
        try:
            with open(os.path.join(symbols.FOLDER, file), encoding="utf-8", errors="replace") as f:
                body = f.read()
        except OSError:
            continue
        if "\nseen in:" not in body and not ("\nshape:" in body and "\ncolour:" in body):
            continue
        neuron = symbols.Neuron(symbols.Neuron.read_name(os.path.join(symbols.FOLDER, file)) or file[:-4])
        if neuron.name in leave_out:
            continue
        seen = neuron.meta.get("seen in")                # seen before: compare with what was seen, not read

        def counts(link):
            return not seen or link.yours or any(source.startswith(("seen in", "looked at")) for source in link.sources)

        theirs = pattern({f: [(v, l.strength) for v, l in links.items() if counts(l)]
                          for f, links in neuron.facets.items()})
        score = likeness(mine, theirs, lambda symbol: symbols.weight(symbol, {neuron.name}))
        if score > 0:
            scores.append((neuron.name, round(score, 2)))
    return sorted(scores, key=lambda s: -s[1])[:5]


# ---------------- the vision model, when memory isn't sure ----------------

ASK = """You name the main thing in a picture and describe it as short lists of plain English words.
Reply with JSON only: {"name": "one or two words", and these keys, each a list of up to 5 short words or
phrases: "is", "shape", "colour", "parts", "texture", "material", "goes with"}. Only what you can see or know."""


def model_image(pic):
    """The picture for the model: the file itself if it's a modest JPEG or PNG, else a 768-pixel JPEG copy."""
    with open(pic.path, "rb") as f:
        data = f.read()
    if (data[:3] == b"\xff\xd8\xff" or data[:8] == b"\x89PNG\r\n\x1a\n") and len(data) < 3_000_000 and \
            max(pic.original) <= 1024:                    # a big photo makes the model fail: send a smaller copy
        return data
    tool = magick()
    if not tool:
        return sight.write_png(None, pic.width, pic.height, [p + (a,) for p, a in zip(pic.rgb, pic.alpha)])
    with tempfile.TemporaryDirectory() as folder:
        out = os.path.join(folder, "for-the-model.jpg")
        subprocess.run([tool, pic.path + "[0]", "-auto-orient", "-resize", "768x768>", "-quality", "85", out],
                       capture_output=True, timeout=60)
        with open(out, "rb") as f:
            return f.read()


def ask_model(pic):
    """-> (name or None, {facet: [(symbol, strength)]}, a note in words)."""
    if os.environ.get("BRICK_NEURAL") == "off" or os.environ.get("BRICK_SIGHT") == "off":
        return None, {}, "the vision model is switched off here"
    if not shutil.which("ollama"):
        return None, {}, "skipped: Ollama isn't installed, so no vision model looked"
    import neural
    if not neural.start():
        return None, {}, "skipped: Ollama is installed but wouldn't start (try: ollama serve)"
    model = sight.vision_model(sight.installed_models())
    if model is None:
        return None, {}, f"skipped: no vision model in Ollama yet (brick sight setup downloads {sight.VISION})"
    try:
        answer = neural.call("/api/chat", {
            "model": model, "stream": False, "format": "json",
            "messages": [{"role": "system", "content": ASK},
                         {"role": "user", "content": "What is the main thing in this picture?",
                          "images": [base64.b64encode(model_image(pic)).decode()]}],
            "options": {"temperature": 0.1, "num_predict": 400, "num_ctx": 8192}}, seconds=300)
        content = answer.get("message", {}).get("content", "{}")
    except Exception as problem:
        return None, {}, f"skipped: {model} didn't answer ({type(problem).__name__})"
    try:
        said = json.loads(content)
    except ValueError:                                  # cut short: keep the name at least
        named = re.search(r'"name"\s*:\s*"([^"]+)"', content)
        said = {"name": named.group(1)} if named else None
    import symbols
    if not isinstance(said, dict):
        return None, {}, f"{model} gave no answer brick could read"
    name = symbols.clean(said.pop("name", "") if isinstance(said.get("name", ""), str) else "") or None
    facets = {}
    for key, values in said.items():
        facet = symbols.ALIASES.get(str(key).lower().strip(), str(key).lower().strip())
        for value in (values if isinstance(values, list) else [values])[:5]:
            value = symbols.tidy(value) if isinstance(value, (str, int, float)) else ""
            if value and len(value) <= 40 and value != name:
                if facet in ("is", "material", "goes with"):
                    facet = symbols.guess_facet(value) or facet
                put(facets, facet, value, 1.5)
    return name, facets, f"{model} says: {name or 'it could not say'}"


# ---------------- seeing ----------------

def see(path, name=None, model=True, memory=True, read=True):
    """Look at a picture with every filter, then name it: given, from memory, or (if unsure) by the model.
    -> {"channels": [(channel, seconds, note)], "facets": {facet: [(symbol, strength)]}, "name", "how",
        "sure", "matches", "model facets", "model note", "contexts", "words", "picture", "seconds"}"""
    started = time.perf_counter()
    pic = Picture(path)
    reader = Reader(pic, off=not read)            # reading writing takes longest: it starts first, alongside
    channels = [("picture", pic.seconds, f"{pic.original[0]} by {pic.original[1]}, looked at "
                                         f"{pic.width} by {pic.height}", {})]
    facets = {}

    def run(channel, function, *args):
        t = time.perf_counter()
        result = function(*args)
        channels.append((channel, time.perf_counter() - t, result[1], result[0]))
        for facet, values in result[0].items():
            for value, strength in values:
                put(facets, facet, value, strength)
        return result

    _, _, things, back = run("objects", objects, pic)
    run("colour", colour, pic, things, back)
    run("shape", shape, pic, things)
    if len(things) > 1:
        for facet, values in relations(things).items():
            for value, strength in values:
                put(facets, facet, value, strength)
                put(channels[1][3], facet, value, strength)
    edge = run("edges", edges, pic)
    run("texture", texture, pic, things)
    contexts = run("layout", layout, pic, things, back)[2]
    try:
        words = run("text", text, reader, edge[2])[2]
    finally:
        reader.stop()
    t = time.perf_counter()
    matches = remembered(facets) if memory else []
    how, sure = None, 0.0
    if name:
        how = "given"
    elif matches and matches[0][1] >= SURE and (len(matches) == 1 or matches[0][1] >= AHEAD * matches[1][1]):
        name, how, sure = matches[0][0], "memory", matches[0][1]
    channels.append(("memory", time.perf_counter() - t, ", ".join(f"{n} ({s})" for n, s in matches) or
                     "nothing seen before looks like it", {}))
    model_name, model_facets, model_note = None, {}, "not needed"
    if model and how is None:
        t = time.perf_counter()
        model_name, model_facets, model_note = ask_model(pic)
        channels.append(("model", time.perf_counter() - t, model_note, model_facets))
        if model_name:
            name, how = model_name, "model"
    elif how != "given":
        channels.append(("model", 0.0, "not needed: memory is sure" if how == "memory" else "not asked", {}))
    if not name and matches:
        sure = matches[0][1]
    return {"channels": channels, "facets": facets, "name": name, "how": how, "sure": sure, "matches": matches,
            "model facets": model_facets, "model note": model_note, "contexts": contexts, "words": words,
            "picture": pic, "things": things, "seconds": time.perf_counter() - started}


def summary(found):
    """What the eye saw, in a few lines."""
    pic = found["picture"]
    where = os.path.basename(pic.path)
    if found["how"] == "memory":
        head = f"{where}: looks like {found['name']} (sure {found['sure']:.2f}, from memory: no model needed)"
    elif found["how"] == "model":
        head = f"{where}: {found['name']} (the vision model named it; memory wasn't sure)"
    elif found["how"] == "given":
        head = f"{where}: {found['name']} (as you said)"
    elif found["matches"]:
        head = f"{where}: I'm not sure what it is; closest is {found['matches'][0][0]} ({found['matches'][0][1]:.2f})"
    else:
        head = f"{where}: I don't know what it is yet"
    lines = [head]
    for channel, seconds, note, _ in found["channels"]:
        lines.append(f"  {channel:<8} {seconds:5.2f}s  {note}")
    decode = found["channels"][0][1]
    lines.append(f"  took {found['seconds']:.2f}s in all ({decode:.2f}s reading the file, "
                 f"{found['seconds'] - decode:.2f}s looking)")
    return "\n".join(lines)
