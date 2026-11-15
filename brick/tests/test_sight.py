"""Tests for brick's sight and the universe of each symbol (its neuron): pictures measured from their pixels,
universes written and merged (hand-written lines kept), loose 'show' wordings, the vision model missing,
and neurons that learn together and by themselves. Pictures are made here, with the standard library.
Nothing here needs Ollama or the graphics card: the vision model is always pretended.

Usage: python3 tests/test_sight.py
"""
import contextlib
import io
import os
import shutil
import struct
import sys
import tempfile
import zlib

sys.path.insert(0, os.path.dirname(os.path.dirname(os.path.abspath(__file__))))
os.environ["BRICK_SIGHT"] = "off"         # the vision model is only ever pretended here
os.environ["BRICK_EYE_TEXT"] = "off"      # and tesseract too, so the tests are the same everywhere
from trainer import eye  # noqa: E402
from trainer import sight  # noqa: E402
from trainer import symbols  # noqa: E402

PICTURES = tempfile.mkdtemp(prefix="brick-sight-")
WHITE, RED, GREEN = (255, 255, 255), (220, 20, 30), (0, 160, 0)


def picture(name, test, size=(120, 120)):
    width, height = size
    path = os.path.join(PICTURES, name)
    sight.write_png(path, width, height, [test(x, y) for y in range(height) for x in range(width)])
    return path


def red_circle():
    return picture("circle.png", lambda x, y: RED if (x - 60) ** 2 + (y - 60) ** 2 <= 40 ** 2 else WHITE)


def tall_green_rectangle():
    return picture("tall.png", lambda x, y: GREEN if 45 <= x < 75 and 10 <= y < 110 else WHITE)


def seen(path):
    width, height, pixels, _ = sight.read_image(path)
    measures = sight.measure(width, height, pixels)
    return measures, sight.symbols(measures, os.path.basename(path))[0]


def values(facets, facet):
    return [v for v, _ in facets.get(facet, [])]


def fresh():
    """An empty folder of universes for one test."""
    symbols.FOLDER = tempfile.mkdtemp(prefix="brick-universes-")
    symbols.now = lambda: 1_000_000.0
    return symbols.FOLDER


def text_of(name):
    with open(symbols.path_of(name), encoding="utf-8") as f:
        return f.read()


def strength(text, facet, value):
    line = next(line for line in text.splitlines() if line.startswith(facet + ":"))
    return next(link.strength for v, link in map(symbols.parse_item, symbols.split_items(line.split(":", 1)[1]))
                if v == value)


def quietly(function, *args):
    out = io.StringIO()
    with contextlib.redirect_stdout(out):
        result = function(*args)
    return result, out.getvalue()


# ---------------- pictures ----------------

def test_red_circle_is_round_and_red():
    measures, facets = seen(red_circle())
    assert "round" in values(facets, "shape"), facets
    assert "symmetric" in values(facets, "shape"), facets
    assert "red" in values(facets, "colour"), facets
    assert "circle" in values(facets, "mathematics"), facets
    assert abs(measures["ratio"] - 1.0) < 0.03, measures["ratio"]
    assert (measures["width"], measures["height"]) == (81, 81), measures


def test_tall_green_rectangle_is_not_round():
    measures, facets = seen(tall_green_rectangle())
    shape = values(facets, "shape")
    assert "round" not in shape and "oval" not in shape, shape
    assert "rectangular" in shape and "tall" in shape, shape
    assert values(facets, "colour")[0] == "green", facets
    assert abs(measures["ratio"] - 0.30) < 0.02, measures["ratio"]
    assert "ratio 0.30" in values(facets, "mathematics"), facets


def test_png_filters_and_ppm():
    """Rows filtered every PNG way (sub, up, average, paeth), an RGB PNG with no alpha, and a PPM, read back."""
    width, height = 9, 5
    pixels = [((x * 29) % 256, (y * 53) % 256, (x * y * 7) % 256) for y in range(height) for x in range(width)]
    raw, previous = b"", bytes(width * 3)
    for y in range(height):
        row = bytes(c for p in pixels[y * width:(y + 1) * width] for c in p)
        kind = y % 5
        out = bytearray()
        for k, value in enumerate(row):
            a = row[k - 3] if k >= 3 else 0
            b = previous[k]
            c = previous[k - 3] if k >= 3 else 0
            guess = [0, a, b, (a + b) // 2,
                     min((abs(a + b - c - a), 0, a), (abs(a + b - c - b), 1, b), (abs(a + b - c - c), 2, c))[2]][kind]
            out.append((value - guess) & 255)
        raw += bytes([kind]) + bytes(out)
        previous = row

    def chunk(tag, body):
        return struct.pack(">I", len(body)) + tag + body + struct.pack(">I", zlib.crc32(tag + body) & 0xffffffff)

    png = b"\x89PNG\r\n\x1a\n" + chunk(b"IHDR", struct.pack(">IIBBBBB", width, height, 8, 2, 0, 0, 0)) + \
        chunk(b"IDAT", zlib.compress(raw)) + chunk(b"IEND", b"")
    assert sight.read_png(png) == (width, height, [p + (255,) for p in pixels])
    ppm = f"P6\n# made in a test\n{width} {height}\n255\n".encode() + bytes(c for p in pixels for c in p)
    assert sight.read_ppm(ppm) == (width, height, [p + (255,) for p in pixels])


def test_jpeg_goes_through_imagemagick():
    if not (shutil.which("magick") or shutil.which("convert")):
        return                                    # not installed here: brick says so when it's needed
    jpeg = os.path.join(PICTURES, "circle.jpg")
    os.system(f"{shutil.which('magick') or shutil.which('convert')} '{red_circle()}' -quality 90 '{jpeg}'")
    measures, facets = seen(jpeg)
    assert "round" in values(facets, "shape") and "red" in values(facets, "colour"), facets


def test_loose_show_wordings():
    for said in ("show apple.png as apple", "show apple.png as an apple", "this is an apple: apple.png",
                 "apple.png is an apple", "apple.png is apple", "this is an apple apple.png",
                 "look at apple.png it's an apple", "show me apple.png, that's an apple"):
        assert sight.show_request(said) == ("apple.png", "apple"), (said, sight.show_request(said))
    assert sight.show_request("show photos/red-apple.JPG as a green apple") == ("photos/red-apple.JPG", "green apple")
    assert sight.show_request('show "my pics/apple.png" as apple') == ("my pics/apple.png", "apple")
    for said in ("what is this.png", "look at this.png", "look this.png", "brick trainer look this.png",
                 "what do you see in this.png"):
        assert sight.show_request(said) == ("this.png", ""), (said, sight.show_request(said))
    assert sight.show_request("brick trainer show shoe.png as a trainer") == ("shoe.png", "trainer")
    assert sight.show_request("this is a trainer: shoe.png") == ("shoe.png", "trainer")
    assert sight.show_request("show game.txt") is None
    assert sight.show_request("write hello in red") is None


# ---------------- the vision model, pretended ----------------

def test_no_ollama_or_no_vision_model_is_said_plainly():
    from trainer import neural
    saved = (os.environ.pop("BRICK_SIGHT"), sight.shutil.which, neural.start, neural.call)
    switched = os.environ.pop("BRICK_NEURAL", None)     # the learning daemon runs with the model switched off;
    try:                                                # here every engine is stubbed, so it's on
        sight.shutil.which = lambda tool: None
        facets, note = sight.model_sees(b"png", "apple")
        assert facets is None and "Ollama isn't installed" in note, note
        sight.shutil.which = lambda tool: "/usr/bin/" + tool
        neural.start = lambda: False
        facets, note = sight.model_sees(b"png", "apple")
        assert facets is None and "wouldn't start" in note, note
        neural.start = lambda: True
        neural.call = lambda path, body=None, seconds=60: {"models": [{"name": "qwen2.5:3b"}]}
        facets, note = sight.model_sees(b"png", "apple")
        assert facets is None and "no vision model" in note and "brick trainer sight setup" in note, note
        neural.call = lambda path, body=None, seconds=60: (
            {"models": [{"name": "qwen2.5vl:3b"}]} if path == "/api/tags" else
            {"message": {"content": '{"shape": ["round"], "color": ["red", "Green"], "parts": ["stem"], "is": []}'}})
        facets, note = sight.model_sees(b"png", "apple")
        assert facets == {"shape": [("round", 1.5)], "colour": [("red", 1.5), ("green", 1.5)],
                          "parts": [("stem", 1.5)]}, facets
        assert note.startswith("qwen2.5vl:3b saw 4 things"), note
        neural.call = lambda path, body=None, seconds=60: (
            {"models": [{"name": "qwen2.5vl:3b"}]} if path == "/api/tags" else 1 / 0)
        facets, note = sight.model_sees(b"png", "apple")
        assert facets is None and "didn't answer" in note, note
    finally:
        os.environ["BRICK_SIGHT"], sight.shutil.which, neural.start, neural.call = saved
        if switched is not None:
            os.environ["BRICK_NEURAL"] = switched


# ---------------- universes ----------------

def test_show_writes_a_universe_and_keeps_hand_edits():
    fresh()
    code, said = quietly(symbols.main, ["show", red_circle(), "as", "an", "apple"])
    assert code == 0, said
    assert "switched off" in said, said                   # the vision model's part says why it's missing
    text = text_of("apple")
    assert text.startswith("# apple: brick's neuron"), text
    first = strength(text, "shape", "round")
    assert first >= 2 and "round (%s, seen in circle.png" % symbols.number(first) in text, text
    assert "colour: red (" in text and "seen in: circle.png" in text, text
    assert text.rstrip().endswith("seen in: circle.png\nlooked up: WordNet, ConceptNet"), text   # once
    assert "edible fruit (3, WordNet)" in text.split("\nis: ")[1].split("\n")[0], text
    # the owner's edits: a link of their own, a line of their own, and a word they never want back
    lines = []
    for line in text.splitlines():
        if line.startswith("shape:"):
            line += ", sphere"
        if line.startswith("colour:"):
            line = "colour: red (2, seen in circle.png), " + ", ".join(
                item for item in symbols.split_items(line.split(":", 1)[1]) if item.split(" (")[0] == "green")
        lines.append(line)
    lines += ["taste: sweet", "# my own note", "apple is a pome", "not: doctor"]
    with open(symbols.path_of("apple"), "w", encoding="utf-8") as f:
        f.write("\n".join(lines) + "\n")
    quietly(symbols.main, ["this", "is", "an", "apple:", red_circle()])
    text = text_of("apple")
    assert "sphere (5, yours)" in text and "sweet (5, yours" in text, text
    assert "# my own note" in text and "apple is a pome" in text, text
    assert strength(text, "shape", "round") == first + 2, text       # seen twice: stronger
    assert "doctor" not in text.replace("not: doctor", ""), text
    assert ("pome", "is", 1.0) in symbols.links_for("apple")
    assert ("round", "shape", min(1.0, round((first + 2) / 5, 3))) in symbols.links_for("apple")
    assert ("sphere", "shape", 1.0) in symbols.links_for("apple")


def test_universe_command_fills_and_forgets():
    fresh()
    code, said = quietly(symbols.main, ["universe", "round"])
    assert code == 0 and "# round: brick's neuron" in said, said
    assert "circular" in text_of("round") and "opposite: square" in text_of("round"), text_of("round")
    code, said = quietly(symbols.main, ["universe", "round", "forget"])
    assert not symbols.exists("round") and "Forgot round" in said, said
    assert symbols.handles(["universe", "round"]) and symbols.handles(["apple.png", "is", "an", "apple"])
    assert not symbols.handles(["game.txt"]) and not symbols.handles(["build", "game.txt"])


# ---------------- neurons learning ----------------

def test_neurons_that_fire_together_wire_together():
    fresh()
    symbols.learn("apple", {"is": ["fruit"]}, "test")
    symbols.learn("pie", {"is": ["food"]}, "test")
    new = symbols.experience(["apple", "pie", "the", "red"], "a direction")
    assert ("apple", "goes with", "pie") in new and ("pie", "goes with", "apple") in new, new
    assert ("apple", "colour", "red") in new, new                    # filed under a facet when one is known
    symbols.experience(["apple", "pie"], "a direction")
    assert "pie (2, a direction)" in text_of("apple"), text_of("apple")
    assert "red" in open(os.path.join(symbols.FOLDER, "_waiting.txt")).read()   # red has no neuron yet


def test_practise_learns_an_indirect_link_and_fades_an_unused_one():
    fresh()
    symbols.learn("apple", {"is": [("fruit", 5)], "goes with": [("doctor", 1)]}, "test")
    symbols.learn("fruit", {"parts": [("seed", 5)]}, "test")
    for name in ("apple", "fruit"):                       # don't look them up: this test is about the links
        neuron = symbols.Neuron(name)
        neuron.add("looked up", "wordnet", 0, "")
        neuron.add("looked up", "conceptnet", 0, "")
        neuron.save()
    with open(symbols.path_of("apple"), "a", encoding="utf-8") as f:
        f.write("taste: sweet (0.3, yours)\n")
    said = symbols.practise()
    assert "apple learned seed (via fruit)" in said, said
    seed = dict((v, w) for v, f, w in symbols.links_for("apple"))["seed"]
    assert 0 < seed < 0.4, seed                                      # weak: only a hint, through fruit
    assert "parts: seed (" in text_of("apple"), text_of("apple")     # parts of a fruit are parts of an apple
    symbols.practise()
    assert dict((v, w) for v, f, w in symbols.links_for("apple"))["seed"] <= 0.4   # practice alone won't grow it
    symbols.now = lambda: 1_000_000.0 + symbols.FADE_AFTER + 60       # days later, nothing used
    said = ""
    for _ in range(20):
        said += symbols.practise() + "; "
    assert "apple let go of doctor" in said, said
    text = text_of("apple")
    assert "doctor" not in text and "taste: sweet (0.3, yours)" in text, text     # yours never fades
    assert "fruit (" in text, text                                   # strong links fade slowly, still there


def words_only():
    return [n for n in symbols.names() if not n.startswith(("letter:", "context:"))]


def test_new_neurons_are_born_a_few_at_a_time():
    fresh()
    for _ in range(4):
        symbols.experience(["kiwi", "mango", "plum", "lime"], "a direction")
    assert words_only() == []                                        # waiting, no files yet (letters have some)
    said = symbols.practise()
    assert said.count("got a neuron") == symbols.MAX_BIRTHS, said
    assert len(words_only()) == symbols.MAX_BIRTHS, (symbols.names(), said)
    symbols.practise()
    symbols.practise()
    assert {"kiwi", "mango", "plum", "lime"} <= set(words_only()), symbols.names()


def test_hand_written_lines_survive_practise_and_relearning():
    fresh()
    with open(symbols.path_of("kettle"), "w", encoding="utf-8") as f:
        f.write("# kettle: brick's neuron for this symbol\nuses: boil water\nshape: round (1, ConceptNet)\n"
                "looked up: WordNet, ConceptNet\n")
    symbols.practise()                                   # brick starts the clock on links it finds
    symbols.now = lambda: 1_000_000.0 + 10 * symbols.FADE_AFTER
    for _ in range(30):
        symbols.practise()
    symbols.learn("kettle", {"uses": [("tea", 1)]}, "a direction")
    text = text_of("kettle")
    assert "uses: boil water (5, yours), tea (1, a direction)" in text, text
    assert "\nshape:" not in text, text                              # learned, unused, faded away


# ---------------- the eye: each filter ----------------

def eyed(name, test, size=(120, 120)):
    return eye.see(picture(name, test, size), model=False)


def said(found, facet):
    return [v for v, _ in found["facets"].get(facet, [])]


def test_eye_shapes():
    import math
    shapes = {
        "triangle": lambda x, y: (255, 160, 0) if 15 <= y < 105 and abs(x - 60) <= (y - 15) * 0.577 else WHITE,
        "square": lambda x, y: (0, 0, 255) if 30 <= x < 90 and 30 <= y < 90 else WHITE,
        "ring": lambda x, y: (0, 0, 0) if 25 ** 2 <= (x - 60) ** 2 + (y - 60) ** 2 <= 45 ** 2 else WHITE,
        "round": lambda x, y: RED if (x - 60) ** 2 + (y - 60) ** 2 <= 40 ** 2 else WHITE,
        "star": lambda x, y: (240, 200, 0) if math.hypot(x - 60, y - 60) <= 25 + 22 * max(
            0, math.cos(5 * (math.atan2(y - 60, x - 60) + math.pi / 2))) ** 3 else WHITE,
    }
    for shape, test in shapes.items():
        found = eyed(f"eye-{shape}.png", test)
        assert said(found, "shape")[0] == shape, (shape, found["facets"])
    ring = eyed("eye-ring.png", shapes["ring"])
    assert "has a hole" in said(ring, "shape") and said(ring, "colour")[0] == "black", ring["facets"]
    assert "5-pointed" in said(eyed("eye-star.png", shapes["star"]), "shape")
    assert "4 corners" in said(eyed("eye-square.png", shapes["square"]), "mathematics")


def test_eye_texture_and_pattern():
    stripes = eyed("eye-stripes.png", lambda x, y: (0, 0, 0) if (x // 6) % 2 else WHITE)
    dots = eyed("eye-dots.png", lambda x, y: (0, 0, 0) if ((x % 12) - 6) ** 2 + ((y % 12) - 6) ** 2 <= 9 else WHITE)
    checks = eyed("eye-checks.png", lambda x, y: (0, 0, 0) if ((x // 10) + (y // 10)) % 2 else WHITE)
    plain = eyed("eye-plain.png", lambda x, y: RED if (x - 60) ** 2 + (y - 60) ** 2 <= 40 ** 2 else WHITE)
    assert "striped" in said(stripes, "pattern") and "vertical stripes" in said(stripes, "pattern"), stripes["facets"]
    assert "dotted" in said(dots, "pattern"), dots["facets"]
    assert "checked" in said(checks, "pattern"), checks["facets"]
    assert not said(plain, "pattern") and "smooth" in said(plain, "texture"), plain["facets"]
    assert "vertical lines" in said(stripes, "edges") and "straight lines" in said(stripes, "edges")


def test_eye_two_things_left_and_right():
    found = eyed("eye-two.png", lambda x, y: RED if (x - 30) ** 2 + (y - 60) ** 2 <= 18 ** 2 else
                 (0, 0, 255) if 75 <= x < 105 and 45 <= y < 75 else WHITE)
    objects = said(found, "objects")
    assert "two things" in objects and "red round thing left of blue square" in objects, objects
    assert [t.position for t in found["things"]] == ["left", "right"]


def test_eye_horizon_scene():
    found = eyed("eye-horizon.png", lambda x, y: (110, 170, 235) if y < 60 else (60, 150, 40))
    layout = said(found, "layout")
    assert "spread-out scene" in layout and "sky-like top" in layout and "ground-like bottom" in layout, layout
    assert "horizon in the middle" in layout, layout
    assert "sky above ground" in found["contexts"]


def test_eye_colour_and_tone():
    found = eyed("eye-pale.png", lambda x, y: (200, 225, 255) if (x - 60) ** 2 + (y - 60) ** 2 <= 40 ** 2 else
                 (40, 40, 40))
    assert said(found, "colour")[0] == "blue" and "pale" in said(found, "tone"), \
        found["facets"]
    assert "vivid" in said(eyed("eye-red.png", lambda x, y: RED if x > 30 and y > 30 and x < 90 and y < 90
                                else WHITE), "tone")


def test_eye_is_fast():
    import random
    random.seed(4)
    noise = [(random.randrange(256), random.randrange(256), random.randrange(256)) for _ in range(160 * 120)]
    path = picture("eye-busy.png", lambda x, y: noise[y * 160 + x] if (x - 80) ** 2 + (y - 60) ** 2 > 900 else RED,
                   (160, 120))
    found = eye.see(path, model=False)
    assert found["seconds"] < 1.5, found["seconds"]                  # usually about 0.1 s


# ---------------- memory: naming without the model ----------------

def test_memory_names_a_new_picture_without_the_model():
    fresh()
    quietly(symbols.main, ["show", picture("apple.png", lambda x, y: RED if (x - 60) ** 2 + (y - 60) ** 2 <= 40 ** 2
                                                   else WHITE), "as", "apple"])
    import math
    quietly(symbols.main, ["show", picture("banana.png", lambda x, y: (240, 220, 40) if 70 <= math.hypot(x - 60, y - 10)
                                                    <= 88 and abs(x - 60) < 55 else WHITE), "as", "banana"])
    asked = []
    saved = eye.ask_model
    os.environ.pop("BRICK_SIGHT")
    try:
        eye.ask_model = lambda pic: asked.append(pic) or (None, {}, "pretend")
        new = picture("another.png", lambda x, y: (200, 30, 40) if (x - 55) ** 2 + (y - 62) ** 2 <= 33 ** 2 else WHITE)
        seen = sight.look(new, learn=True)
        assert seen["name"] == "apple" and seen["how"] == "memory" and not asked, seen
        assert seen["sure"] >= eye.SURE and not seen["errors"], seen
        assert "another.png" in text_of("apple")                     # seeing it taught apple's neuron
        # nothing it knows looks like this: the model is asked, and its name is taken
        eye.ask_model = lambda pic: ("cup", {"shape": [("round", 1.5)]}, "pretend model says: cup")
        odd = sight.look(picture("odd.png", lambda x, y: (0, 0, 0) if ((x // 10) + (y // 10)) % 2 else WHITE))
        assert odd["name"] == "cup" and odd["how"] == "model", odd
        assert symbols.exists("cup")
    finally:
        eye.ask_model = saved
        os.environ["BRICK_SIGHT"] = "off"


def test_look_never_raises_on_odd_files():
    fresh()
    broken = os.path.join(PICTURES, "broken.png")
    with open(broken, "w") as f:
        f.write("not a picture at all")
    for path in (broken, os.path.join(PICTURES, "missing.jpg")):
        seen = sight.look(path)
        assert seen["errors"] and seen["how"] == "unknown" and seen["name"] is None, seen
    seen = sight.look(red_circle(), "ball", learn=False)
    assert seen["how"] == "told" and seen["sure"] == 1.0 and not symbols.exists("ball"), seen
    assert set(seen) >= {"name", "sure", "how", "symbols", "channels", "objects", "text", "errors"}
    assert ("round", "shape", 1.0) in seen["symbols"] and seen["channels"]["shape"][0], seen
    assert seen["objects"][0]["position"] == "middle", seen["objects"]


# ---------------- little words, letters, contexts ----------------

def test_little_words_fire_and_breadth_weighs():
    fresh()
    symbols.learn("the", {"goes with": ["cat", "dog", "sun"]}, "test")
    new = symbols.experience(["the", "apple"], "a direction")
    assert ("the", "goes with", "apple") in new, new                 # little words are symbols too
    assert "# breadth: 4 (a specific symbol)" in text_of("the"), text_of("the")
    assert symbols.breadth("the") == 5 and symbols.weight("the") == 1 / 3      # 1 / (1 + floor(log2 5))
    assert symbols.breadth("the", {"cat", "dog"}) == 3 and symbols.weight("the", {"cat", "dog"}) == 1 / 2
    assert symbols.breadth("nothing here") == 1 and symbols.weight("nothing here") == 1.0


def test_letters_in_context():
    fresh()
    symbols.learn("cat", {"is": ["animal"]}, "test")
    symbols.spell(["cat", "act", "a"], "test")
    c = text_of("letter:c")
    assert os.path.isfile(os.path.join(symbols.FOLDER, "letter-c.txt")) and c.startswith("# letter:c: brick's neuron")
    assert "consonant" in c and "start (1" in c and "middle (1" in c and "cat (1" in c and "act (1" in c, c
    a = text_of("letter:a")
    assert "vowel" in a and "alone (1" in a and "after: t (1, test), c (1, test)" in a and "before: c (1" in a, a
    cat = text_of("cat")
    assert "letters: c (1, spelling), a (1, spelling), t (1, spelling)" in cat, cat
    assert "starts with c" in cat and "ends with t" in cat, cat
    assert ("letter:c", "letters", 0.2) in symbols.links_for("cat")


def test_contexts_are_symbols_of_their_own():
    fresh()
    for colour in ("red", "red", "blue", "red", "green", "red", "red"):
        symbols.experience(["write", "hi", "in", colour, "at", "the", "top"], "a direction")
    waiting = open(os.path.join(symbols.FOLDER, "_waiting.txt")).read()
    assert "context:in _ at / fills: red (5, a direction)" in waiting, waiting
    for _ in range(15):
        symbols.practise()
    assert os.path.isfile(os.path.join(symbols.FOLDER, "context-in-_-at.txt")), os.listdir(symbols.FOLDER)
    frame = text_of("context:in _ at")
    assert "fills: red (5" in frame and "blue (1" in frame, frame
    # the eye's contexts: a thing seen in a scene links to it, and the scene links back
    quietly(symbols.main, ["show", picture("field.png", lambda x, y: (110, 170, 235) if y < 60 else (60, 150, 40)),
                           "as", "field"])
    assert "contexts: sky above ground (" in text_of("field"), text_of("field")
    waiting = open(os.path.join(symbols.FOLDER, "_waiting.txt")).read()
    assert "context:sky above ground / fills: field (1" in waiting, waiting


def test_words_read_in_a_picture_teach_letters():
    fresh()
    saved = eye.Reader.words
    try:
        eye.Reader.words = lambda self, density: ({"text": [("has writing", 2.0), ("stop", 1.5)]}, "read: stop",
                                                   ["stop", "here"])
        seen = sight.look(red_circle(), learn=True)
        assert seen["text"] == "stop here", seen
        assert symbols.exists("letter:s") and "stop (1, written in circle.png)" in text_of("letter:s")
    finally:
        eye.Reader.words = saved


if __name__ == "__main__":
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
