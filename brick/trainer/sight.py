"""brick's sight: look at a picture of a thing, and say what it is like as symbols, for its universe.

    look(path, name=None, learn=True) -> dict    the stable way in, for anything that brings brick a picture:
      the eye (trainer/eye.py) looks with all its filters; name given ("told"), or named from memory, or by the vision
      model when memory isn't sure; learn=True feeds the neurons (the thing's universe, its contexts, the
      words read in it with their letters). Never raises: problems go in "errors". Keys: name, sure (0-1),
      how (told / memory / model / unknown), symbols [(symbol, facet, weight)], channels {channel: (symbols,
      ms)}, objects [{name, shape, colour, position, share, box}], text, errors, contexts, summary, learned,
      ms, decode ms.

The older, simpler measuring below (measure, symbols) is kept for the tests that use it:

    look(path, name)   -> what was found, in three parts, each a source and its {facet: [(symbol, strength)]}:
      measured from the pixels (standard library only): the thing apart from its background (found by the
          border's colour), how wide it is for how tall, how round (how close its outline is to an ellipse,
          and how much of its box it fills), whether it's the same left and right, its main colours by brick's
          colour names, its size in pixels -> "round", "symmetric", "red", "ratio 1.04" ...
      what the vision model on this computer sees (optional): a local model served by Ollama, started and
          stopped with brick like the neural reader (trainer/neural.py); skipped, and said so, when there isn't one
      what brick already knows about the name: WordNet and ConceptNet (symbols.lookup)
PNG (8 or 16 bits, any colour type, not interlaced) and PPM/PGM are read directly; anything else (JPEG,
WebP, GIF ...) is turned into a PNG first with ImageMagick (magick), when it's installed.
Nothing is sent off this computer.

    brick trainer sight          is the vision model there?
    brick trainer sight setup    download it (once; about 3 GB): qwen2.5vl:3b
"""
import base64
import colorsys
import json
import math
import os
import re
import shutil
import struct
import subprocess
import tempfile
import zlib
from collections import Counter, deque

VISION = "qwen2.5vl:3b"
VISION_FAMILIES = ("qwen2.5vl", "qwen2-vl", "qwen3-vl", "llava", "llama3.2-vision", "gemma3", "minicpm-v",
                   "moondream", "bakllava", "granite3.2-vision", "mistral-small3.1")
LARGEST = 500               # pictures bigger than this (in pixels across) are shrunk first, when magick is here
IMAGES = r"png|jpe?g|webp|gif|bmp|ppm|pgm|pnm|pbm|tiff?|heic|heif|avif"


class CantSee(Exception):
    pass


# ---------------- reading pictures ----------------

def read_png(data):
    """PNG bytes -> (width, height, [(r, g, b, a)] row by row)."""
    if data[:8] != b"\x89PNG\r\n\x1a\n":
        raise CantSee("it isn't a PNG")
    at, idat, palette, alpha_table = 8, [], None, None
    width = height = depth = kind = interlace = None
    while at < len(data):
        size, tag = struct.unpack(">I4s", data[at:at + 8])
        body = data[at + 8:at + 8 + size]
        at += 12 + size
        if tag == b"IHDR":
            width, height, depth, kind, _, _, interlace = struct.unpack(">IIBBBBB", body)
        elif tag == b"PLTE":
            palette = [tuple(body[k:k + 3]) for k in range(0, len(body), 3)]
        elif tag == b"tRNS":
            alpha_table = body
        elif tag == b"IDAT":
            idat.append(body)
        elif tag == b"IEND":
            break
    if width is None:
        raise CantSee("the PNG has no header")
    if interlace:
        raise CantSee("it's an interlaced PNG")
    channels = {0: 1, 2: 3, 3: 1, 4: 2, 6: 4}.get(kind)
    if channels is None or depth not in (1, 2, 4, 8, 16):
        raise CantSee(f"it's a kind of PNG brick can't read (colour type {kind}, {depth} bits)")
    raw = zlib.decompress(b"".join(idat))
    bpp = max(1, channels * depth // 8)
    stride = (width * channels * depth + 7) // 8
    rows, previous, at = [], bytearray(stride), 0
    for _ in range(height):
        kind_of_filter, row = raw[at], bytearray(raw[at + 1:at + 1 + stride])
        at += 1 + stride
        if kind_of_filter == 1:
            for k in range(bpp, stride):
                row[k] = (row[k] + row[k - bpp]) & 255
        elif kind_of_filter == 2:
            row = bytearray((a + b) & 255 for a, b in zip(row, previous))
        elif kind_of_filter == 3:
            for k in range(stride):
                row[k] = (row[k] + ((row[k - bpp] if k >= bpp else 0) + previous[k]) // 2) & 255
        elif kind_of_filter == 4:
            for k in range(stride):
                a = row[k - bpp] if k >= bpp else 0
                b = previous[k]
                c = previous[k - bpp] if k >= bpp else 0
                p = a + b - c
                pa, pb, pc = abs(p - a), abs(p - b), abs(p - c)
                row[k] = (row[k] + (a if pa <= pb and pa <= pc else b if pb <= pc else c)) & 255
        rows.append(row)
        previous = row
    pixels = []
    for row in rows:
        if depth == 16:
            samples = row[0::2]
        elif depth == 8:
            samples = row
        else:
            per, mask = 8 // depth, (1 << depth) - 1
            samples = [(row[k // per] >> (8 - depth * (k % per + 1))) & mask for k in range(width * channels)]
            if kind != 3:
                samples = [s * 255 // mask for s in samples]
        for x in range(width):
            s = samples[x * channels:(x + 1) * channels]
            if kind == 0:
                pixels.append((s[0], s[0], s[0], 255))
            elif kind == 2:
                pixels.append((s[0], s[1], s[2], 255))
            elif kind == 3:
                r, g, b = palette[s[0]] if palette and s[0] < len(palette) else (0, 0, 0)
                a = alpha_table[s[0]] if alpha_table and s[0] < len(alpha_table) else 255
                pixels.append((r, g, b, a))
            elif kind == 4:
                pixels.append((s[0], s[0], s[0], s[1]))
            else:
                pixels.append((s[0], s[1], s[2], s[3]))
    return width, height, pixels


def read_ppm(data):
    """PPM / PGM (P2, P3, P5, P6) bytes -> (width, height, pixels)."""
    tokens, at = [], 0
    while len(tokens) < 4:
        found = re.compile(rb"\s*(#[^\n]*\n\s*)*(\S+)").match(data, at)
        if not found:
            raise CantSee("the PPM header is cut short")
        tokens.append(found.group(2))
        at = found.end()
    magic, width, height, top = tokens[0], int(tokens[1]), int(tokens[2]), int(tokens[3])
    grey = magic in (b"P2", b"P5")
    count = width * height * (1 if grey else 3)
    if magic in (b"P5", b"P6"):
        at += 1
        size = 2 if top > 255 else 1
        body = data[at:at + count * size]
        values = list(body[0::2]) if size == 2 else list(body)
    elif magic in (b"P2", b"P3"):
        values = [int(v) for v in data[at:].split()[:count]]
    else:
        raise CantSee("it's a kind of PPM brick can't read")
    if len(values) < count:
        raise CantSee("the PPM is cut short")
    if top != 255:
        values = [v * 255 // top for v in values]
    if grey:
        return width, height, [(v, v, v, 255) for v in values]
    return width, height, [(values[k], values[k + 1], values[k + 2], 255) for k in range(0, count, 3)]


def png_size(data):
    return struct.unpack(">II", data[16:24]) if data[:8] == b"\x89PNG\r\n\x1a\n" else (0, 0)


def converted(path, folder):
    """The picture as an 8-bit RGBA PNG made by ImageMagick (shrunk to LARGEST across), or CantSee."""
    tool = shutil.which("magick") or shutil.which("convert")
    if not tool:
        raise CantSee("brick reads PNG and PPM itself; for other pictures it needs ImageMagick, which isn't "
                      "installed (sudo pacman -S imagemagick), or save the picture as a PNG")
    out = os.path.join(folder, "picture.png")
    done = subprocess.run([tool, path + "[0]", "-auto-orient", "-resize", f"{LARGEST}x{LARGEST}>",
                           "-interlace", "none", "-depth", "8", "PNG32:" + out],
                          capture_output=True, text=True, timeout=60)
    if done.returncode or not os.path.isfile(out):
        raise CantSee(f"ImageMagick couldn't read it ({done.stderr.strip()[:120] or 'no reason given'})")
    return out


def read_image(path):
    """-> (width, height, pixels, the PNG or JPEG bytes a vision model can be given)."""
    with open(path, "rb") as f:
        data = f.read()
    with tempfile.TemporaryDirectory() as folder:
        big = max(png_size(data)) > LARGEST
        if data[:8] == b"\x89PNG\r\n\x1a\n" and not (big and (shutil.which("magick") or shutil.which("convert"))):
            try:
                return (*read_png(data), data)
            except CantSee as problem:
                if "interlaced" not in str(problem) and "kind of PNG" not in str(problem):
                    raise
        if data[:2] in (b"P2", b"P3", b"P5", b"P6"):
            width, height, pixels = read_ppm(data)
            return width, height, pixels, write_png(None, width, height, pixels)
        with open(converted(path, folder), "rb") as f:
            png = f.read()
        shown = data if data[:3] == b"\xff\xd8\xff" else png       # JPEG as it is, the rest as PNG
        return (*read_png(png), shown)


def write_png(path, width, height, pixels):
    """Pixels [(r, g, b) or (r, g, b, a)] -> an RGBA PNG (written to path, and returned as bytes)."""
    rows = b"".join(b"\x00" + bytes(c for p in pixels[y * width:(y + 1) * width]
                                    for c in (tuple(p) + (255,))[:4]) for y in range(height))

    def chunk(tag, body):
        return struct.pack(">I", len(body)) + tag + body + struct.pack(">I", zlib.crc32(tag + body) & 0xffffffff)

    data = b"\x89PNG\r\n\x1a\n" + chunk(b"IHDR", struct.pack(">IIBBBBB", width, height, 8, 6, 0, 0, 0)) + \
        chunk(b"IDAT", zlib.compress(rows)) + chunk(b"IEND", b"")
    if path:
        with open(path, "wb") as f:
            f.write(data)
    return data


# ---------------- measuring ----------------

def colour_name(r, g, b):
    """One of brick's plain colour names for a pixel, by its hue, so a dark red apple is still red."""
    h, s, v = colorsys.rgb_to_hsv(r / 255, g / 255, b / 255)
    deg = h * 360
    if v < 0.16:
        return "black"
    if s < 0.18 or v < 0.25 and s < 0.35:
        return "white" if v > 0.86 else "black" if v < 0.3 else "grey"
    if 10 <= deg < 45 and v < 0.6:
        return "brown"
    if deg < 12 or deg >= 340:
        return "pink" if s < 0.45 and v > 0.75 else "red"
    if deg < 45:
        return "orange"
    if deg < 68:
        return "yellow"
    if deg < 165:
        return "green"
    if deg < 200:
        return "cyan"
    if deg < 255:
        return "blue"
    if deg < 290:
        return "purple"
    return "pink" if s < 0.6 else "magenta"


_palette = None


def palette():
    """brick's finer colour names (the dictionary's: crimson, sea green ...) -> (r, g, b)."""
    global _palette
    if _palette is None:
        _palette = {}
        try:
            import dictionary
            from actions import COLOURS
            table = {**{k: (v, False) for k, v in COLOURS.items()}, **dictionary.colour_table()}
        except Exception:
            table = {}
        spaced = {k.replace(" ", "") for k in table if " " in k}
        for name, (code, _) in table.items():
            if " " not in name and name in spaced or name == "gray":
                continue                                  # 'seagreen': 'sea green' says it better
            parts = code.split(";")
            if len(parts) == 5:
                _palette[name] = tuple(int(c) for c in parts[2:])
    return _palette


def fine_name(r, g, b):
    return min(palette().items(), key=lambda kv: sum((a - c) ** 2 for a, c in zip(kv[1], (r, g, b))),
               default=(None, None))[0]


def measure(width, height, pixels):
    """Find the thing apart from its background, and measure it -> a dict (CantSee if there's no thing)."""
    border = [pixels[x] for x in range(width)] + [pixels[(height - 1) * width + x] for x in range(width)] + \
             [pixels[y * width] for y in range(height)] + [pixels[y * width + width - 1] for y in range(height)]
    see_through = sum(1 for p in border if p[3] < 128) > len(border) / 2
    if see_through:
        solid = [p[3] >= 128 for p in pixels]
    else:
        back = tuple(sorted(p[k] for p in border)[len(border) // 2] for k in range(3))
        spread = sorted(math.dist(p[:3], back) for p in border)
        limit = max(40.0, spread[int(len(spread) * 0.9)] * 1.5 + 10)
        solid = [p[3] >= 128 and math.dist(p[:3], back) > limit for p in pixels]
    # outside: background reachable from the edge; everything else (holes, highlights) belongs to the thing
    outside = [False] * len(pixels)
    queue = deque(k for k in [x for x in range(width)] + [(height - 1) * width + x for x in range(width)] +
                  [y * width for y in range(height)] + [y * width + width - 1 for y in range(height)]
                  if not solid[k])
    for k in queue:
        outside[k] = True
    while queue:
        k = queue.popleft()
        y, x = divmod(k, width)
        for n in ((k - 1) if x else -1, (k + 1) if x < width - 1 else -1, k - width, k + width):
            if 0 <= n < len(pixels) and not outside[n] and not solid[n]:
                outside[n] = True
                queue.append(n)
    # the biggest piece that isn't outside
    piece, best = [0] * len(pixels), (0, 0)
    label = 0
    for start in range(len(pixels)):
        if outside[start] or piece[start]:
            continue
        label += 1
        piece[start], size, queue = label, 0, deque([start])
        while queue:
            k = queue.popleft()
            size += 1
            y, x = divmod(k, width)
            for n in ((k - 1) if x else -1, (k + 1) if x < width - 1 else -1, k - width, k + width):
                if 0 <= n < len(pixels) and not outside[n] and not piece[n]:
                    piece[n] = label
                    queue.append(n)
        best = max(best, (size, label))
    area, label = best
    if area < 16 or area > 0.98 * width * height:
        raise CantSee("I couldn't tell the thing apart from its background (a plain background helps)")
    inside = [piece[k] == label for k in range(len(pixels))]
    xs = [k % width for k in range(len(pixels)) if inside[k]]
    ys = [k // width for k in range(len(pixels)) if inside[k]]
    x0, x1, y0, y1 = min(xs), max(xs), min(ys), max(ys)
    w, h = x1 - x0 + 1, y1 - y0 + 1
    # the ellipse with the same spread as the thing (a stem or a leaf hardly moves it)
    cx, cy = sum(xs) / area, sum(ys) / area
    a = 2 * math.sqrt(max(sum((x - cx) ** 2 for x in xs) / area, 0.25))
    b = 2 * math.sqrt(max(sum((y - cy) ** 2 for y in ys) / area, 0.25))
    edge = []
    for x, y in zip(xs, ys):
        k = y * width + x
        if x in (0, width - 1) or y in (0, height - 1) or not (inside[k - 1] and inside[k + 1] and
                                                               inside[k - width] and inside[k + width]):
            edge.append(math.hypot((x - cx) / a, (y - cy) / b))
    middle = sorted(edge)[len(edge) // 2]
    fit = sum(1 for e in edge if abs(e / middle - 1) < 0.07) / len(edge)     # 1: the outline is an ellipse
    left_right = sum(1 for x, y in zip(xs, ys) if inside[y * width + (x0 + x1 - x)])
    top_bottom = sum(1 for x, y in zip(xs, ys) if inside[(y0 + y1 - y) * width + x])
    names = Counter(colour_name(*pixels[y * width + x][:3]) for x, y in zip(xs, ys))
    colours = [(name, count / area) for name, count in names.most_common(3) if count / area >= 0.15]
    top = colours[0][0] if colours else names.most_common(1)[0][0]
    chosen = [pixels[y * width + x][:3] for x, y in zip(xs, ys) if colour_name(*pixels[y * width + x][:3]) == top]
    average = tuple(round(sum(p[k] for p in chosen) / len(chosen)) for k in range(3))
    return {"width": w, "height": h, "area": area, "ratio": a / b, "fill": area / (w * h), "fit": fit,
            "left right": left_right / (2 * area - left_right), "top bottom": top_bottom / (2 * area - top_bottom),
            "colours": colours, "average": average, "fine colour": fine_name(*average)}


def symbols(m, where):
    """Measures -> {facet: [(symbol, strength)]} and one line in words."""
    out, s = {}, 2.0

    def put(facet, *values):
        for value in values:
            if value not in [v for v, _ in out.setdefault(facet, [])]:
                out[facet].append((value, s))

    ratio = m["ratio"]
    even = 0.87 <= ratio <= 1.15
    if m["fill"] > 0.92:
        put("shape", "square" if 0.92 <= ratio <= 1.08 else "rectangular")
        put("mathematics", "square" if 0.92 <= ratio <= 1.08 else "rectangle")
    elif m["fit"] >= 0.8:
        put("shape", "round" if even else "oval")
        put("mathematics", "circle" if even else "ellipse")
    elif m["fit"] >= 0.6:
        put("shape", "roundish")
        put("mathematics", "near circle" if even else "near ellipse")
    else:
        put("shape", "irregular")
    if ratio < 0.8:
        put("shape", "tall")
    elif ratio > 1.25:
        put("shape", "wide")
    if m["left right"] >= 0.9:
        put("shape", "symmetric")
        put("mathematics", "mirror symmetry")
    if m["top bottom"] >= 0.9:
        put("mathematics", "top-bottom symmetry")
    put("mathematics", f"ratio {ratio:.2f}")
    wording = f"about {ratio:.2f} times as wide as tall" if ratio >= 1 else f"about {1 / ratio:.2f} times as tall as wide"
    put("size", wording, f"{m['width']} by {m['height']} pixels in {where}")
    put("colour", *[name for name, _ in m["colours"]])
    if m["fine colour"] and m["fine colour"] not in [n for n, _ in m["colours"]]:
        put("colour", m["fine colour"])
    return out, wording


# ---------------- the vision model (optional) ----------------

def vision_model(installed):
    """The vision model to ask: BRICK_SIGHT_MODEL, else qwen2.5vl:3b, else any known vision model installed."""
    want = os.environ.get("BRICK_SIGHT_MODEL") or VISION
    for name in installed:
        if name in (want, want + ":latest"):
            return name
    return next((name for name in installed if name.split(":")[0] in VISION_FAMILIES), None)


def installed_models():
    from trainer import neural
    try:
        return [m.get("name", "") for m in neural.call("/api/tags", seconds=3).get("models", [])]
    except Exception:
        return []


ASK = """You describe one thing in a picture as short lists of plain English words.
Reply with JSON only, with these keys, each a list of up to 5 short words or phrases (1 to 3 words each):
"is", "shape", "colour", "size", "parts", "texture", "material", "goes with".
Only say what you can see in the picture or know for sure about the thing. Use [] when unsure."""


def model_sees(image, name):
    """-> ({facet: [(symbol, strength)]} or None, a note in words for the person)."""
    if os.environ.get("BRICK_NEURAL") == "off" or os.environ.get("BRICK_SIGHT") == "off":
        return None, "the vision model is switched off here"
    if not shutil.which("ollama"):
        return None, "skipped: Ollama isn't installed, so no vision model looked (sudo pacman -S ollama-cuda)"
    from trainer import neural
    if not neural.start():
        return None, "skipped: Ollama is installed but wouldn't start (try: ollama serve)"
    model = vision_model(installed_models())
    if model is None:
        return None, f"skipped: no vision model in Ollama yet (brick trainer sight setup downloads {VISION}, about 3 GB)"
    try:
        answer = neural.call("/api/chat", {
            "model": model, "stream": False, "format": "json",
            "messages": [{"role": "system", "content": ASK},
                         {"role": "user", "content": f"The picture shows {article(name)} {name}. Describe it.",
                          "images": [base64.b64encode(image).decode()]}],
            "options": {"temperature": 0.1, "num_predict": 300}}, seconds=300)
        said = json.loads(answer.get("message", {}).get("content", "{}"))
    except Exception as problem:
        return None, f"skipped: {model} didn't answer ({type(problem).__name__})"
    from trainer import symbols as brain_symbols
    found = {}
    for key, values in (said.items() if isinstance(said, dict) else ()):
        facet = brain_symbols.ALIASES.get(str(key).lower().strip(), str(key).lower().strip())
        for value in (values if isinstance(values, list) else [values])[:5]:
            value = brain_symbols.tidy(value) if isinstance(value, (str, int, float)) else ""
            if value and len(value) <= 40 and value != name:
                if facet in ("is", "material", "goes with"):        # 'is: red' is a colour
                    facet = brain_symbols.guess_facet(value) or facet
                found.setdefault(facet, [])
                if value not in [v for v, _ in found[facet]]:
                    found[facet].append((value, 1.5))
    count = sum(len(v) for v in found.values())
    return found, f"{model} saw {count} things" + (": " + ", ".join(v for vs in found.values() for v, _ in vs[:1])
                                                    if count else "")


def article(name):
    return "an" if name[:1] in "aeiou" else "a"


# ---------------- looking ----------------

def look(path, name=None, learn=True, model=True):
    """Look at a picture with the eye (trainer/eye.py), name it, and (learn=True) feed the neurons what was seen.
    Never raises: anything that goes wrong is listed in "errors".
    -> {"name", "sure" (0-1), "how" ("told" / "memory" / "model" / "unknown"), "symbols" [(symbol, facet,
        weight)], "channels" {channel: ([(symbol, facet)], ms)}, "objects" [{...}], "text" (words read, or ""),
        "errors" [], "contexts" [], "summary" (in words), "learned" [], "ms" (all), "decode ms"}"""
    from trainer import eye
    from trainer import symbols as brain
    result = {"name": None, "sure": 0.0, "how": "unknown", "symbols": [], "channels": {}, "objects": [], "text": "",
              "errors": [], "contexts": [], "summary": "", "learned": [], "ms": 0, "decode ms": 0, "path": path}
    told = brain.clean(name) if name else None
    try:
        found = eye.see(path, name=told, model=model)
    except CantSee as problem:
        result["errors"].append(f"couldn't look at {path}: {problem}")
        result["summary"] = result["errors"][-1]
        return result
    except Exception as problem:                       # an odd file must never stop brick
        result["errors"].append(f"couldn't look at {path}: {type(problem).__name__}: {problem}")
        result["summary"] = result["errors"][-1]
        return result
    where = os.path.basename(path)
    model_facets, model_note = found["model facets"], found["model note"]
    if told and model:
        try:
            model_facets, model_note = model_sees(eye.model_image(found["picture"]), told)
            model_facets = model_facets or {}
        except Exception as problem:
            result["errors"].append(f"the vision model: {type(problem).__name__}: {problem}")
        found["channels"].append(("model", 0.0, model_note, model_facets))
    result.update(name=found["name"], sure=1.0 if told else found["sure"],
                  how={"given": "told", None: "unknown"}.get(found["how"], found["how"]),
                  symbols=[(value, facet, round(min(1.0, strength / 2), 2))
                           for facet, values in found["facets"].items() for value, strength in values],
                  channels={channel: ([(value, facet) for facet, values in facets.items() for value, _ in values],
                                      round(seconds * 1000)) for channel, seconds, _, facets in found["channels"]},
                  objects=[{"name": t.name(), "shape": t.shape, "colour": t.colour, "position": t.position,
                            "share": round(t.share, 3), "box": t.box} for t in found["things"]],
                  text=" ".join(found["words"]), contexts=found["contexts"],
                  ms=round(found["seconds"] * 1000), **{"decode ms": round(found["channels"][0][1] * 1000)})
    if result["how"] == "model":
        result["sure"] = 0.6
    if learn:
        try:
            result["learned"] = teach(found, result["name"], result["how"], model_facets, where)
        except Exception as problem:
            result["errors"].append(f"learning from it: {type(problem).__name__}: {problem}")
    if model_note and found["how"] == "given":
        found["model note"] = model_note
    result["summary"] = eye.summary(found)
    return result


def teach(found, name, how, model_facets, where):
    """Feed the neurons what the eye saw -> [lines saying what was learned]."""
    from trainer import symbols as brain
    said = []
    facets = found["facets"]
    source = f"seen in {where}" if how == "told" else f"looked at {where}"
    scale = 1.0 if how == "told" else 0.5          # a guess teaches half as much as being told
    if found["words"]:
        brain.experience(found["words"], f"written in {where}")          # its words, letters and contexts
        said.append(f"read {len(found['words'])} words (their letters and contexts learned)")
    main = [v for f in ("colour", "shape") for v, _ in facets.get(f, [])[:1]]
    if not name:
        if found["contexts"]:
            brain.contextualise("", found["contexts"], source, appears=main)
        return said
    brain.learn(name, {f: [(v, round(s * scale, 2)) for v, s in values] for f, values in facets.items()}, source)
    if model_facets:
        model = (vision_model(installed_models()) or "the vision model").split(":")[0]
        brain.learn(name, model_facets, f"seen by {model} in {where}")
    neuron = brain.Neuron(name)
    neuron.add("seen in", where, 0, "")
    neuron.save()
    measured = [(v, f) for f, values in facets.items() for v, _ in values]
    brain.experience([(name, None)] + measured, source, centre=name, centre_learns=False, spelling=False)
    if found["contexts"]:
        brain.contextualise(name, found["contexts"], source, appears=main)
    known = brain.fill(name)
    said.append(f"the universe of {name} learned what was seen" + (f", and {len(known)} links from WordNet and "
                                                               f"ConceptNet" if known else ""))
    return said


FRONT = {"show", "look", "at", "see", "teach", "learn", "this", "is", "it", "it's", "its", "that", "that's",
         "identify", "recognise", "recognize", "name", "guess", "tell", "do", "think", "in", "there", "who",
         "thats", "here", "here's", "heres", "picture", "image", "photo", "pic", "of", "as", "a", "an", "the",
         "brick", "please", "called", "named", "means", "me", "you", "what", "what's", "whats", "shows", "showing",
         "file", "and", "to", "be", "are", "these", "those", "i", "my"}


def show_request(text):
    """'show apple.png as apple', 'this is an apple: apple.png', 'apple.png is an apple' -> (path, name);
    'look at photo.jpg', 'what is this.png', 'look photo.jpg' -> (path, ''): the eye names it.
    None if there's no picture in it. A typed 'brick trainer ' in front is the command, not part of the name,
    so 'brick trainer look this.png' is (this.png, '') while 'show shoe.png as a trainer' keeps 'trainer'."""
    text = re.sub(r"^\s*brick\s+trainer(?=\s|$)", "", text, flags=re.I)
    found = re.search(rf"\"([^\"]+)\"|'([^']+\.(?:{IMAGES}))'|(\S+\.(?:{IMAGES}))(?=[\s:,.;!?]|$)", text, re.I)
    if not found:
        return None
    path = next(g for g in found.groups() if g)
    if not re.search(rf"\.(?:{IMAGES})$", path, re.I) and not os.path.isfile(path):
        return None
    rest = (text[:found.start()] + " " + text[found.end():]).lower()
    words = re.findall(r"[a-z0-9][a-z0-9'\-]*", rest)
    while words and words[0] in FRONT:
        words.pop(0)
    while words and words[-1] in FRONT:
        words.pop()
    if len(words) > 2 and ("as" in words or "is" in words):     # 'apple, this is it' and the like
        cut = max(k for k, w in enumerate(words) if w in ("as", "is"))
        words = [w for w in words[cut + 1:] if w not in FRONT] or words
    return path, " ".join(words)


# ---------------- brick trainer sight ----------------

def command(args):
    from trainer import neural
    if not shutil.which("ollama"):
        print("brick's sight measures pictures by itself. To also have a vision model look, it needs Ollama:\n"
              "    sudo pacman -S ollama-cuda\nthen: brick trainer sight setup   (brick starts and stops Ollama itself)")
        return 0
    if not neural.start():
        print("Ollama is installed, but wouldn't start. Try running 'ollama serve' to see why.")
        return 1
    model = vision_model(installed_models())
    if args[:1] == ["setup"] and model is None:
        print(f"Downloading {VISION} (once; about 3 GB)...", flush=True)
        import urllib.request
        request = urllib.request.Request(neural.SERVER + "/api/pull", json.dumps({"model": VISION}).encode(),
                                         {"Content-Type": "application/json"})
        shown = ""
        with urllib.request.urlopen(request, timeout=3600) as response:
            for raw in response:
                step = json.loads(raw or b"{}")
                note = step.get("status", "") + (f" {100 * step.get('completed', 0) // step['total']}%"
                                                 if step.get("total") else "")
                if note != shown:
                    print("  " + note, flush=True)
                    shown = note
        model = vision_model(installed_models())
    if model is None:
        print(f"No vision model yet. brick trainer sight setup downloads {VISION} (about 3 GB, once).")
    else:
        print(f"brick's sight uses {model}, on this computer, when you show it a picture.")
    return 0
