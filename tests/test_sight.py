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
import sight  # noqa: E402
import symbols  # noqa: E402

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
    assert sight.show_request("show game.txt") is None
    assert sight.show_request("write hello in red") is None


# ---------------- the vision model, pretended ----------------

def test_no_ollama_or_no_vision_model_is_said_plainly():
    import neural
    saved = (os.environ.pop("BRICK_SIGHT"), sight.shutil.which, neural.start, neural.call)
    try:
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
        assert facets is None and "no vision model" in note and "brick sight setup" in note, note
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


def test_new_neurons_are_born_a_few_at_a_time():
    fresh()
    for _ in range(4):
        symbols.experience(["kiwi", "mango", "plum", "lime"], "a direction")
    assert symbols.names() == []                                     # waiting, no files yet
    said = symbols.practise()
    assert len(symbols.names()) == symbols.MAX_BIRTHS, (symbols.names(), said)
    assert said.count("got a neuron") == symbols.MAX_BIRTHS, said
    symbols.practise()
    symbols.practise()
    assert len(symbols.names()) == 4, symbols.names()


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
