"""Tests for brick's imagination (imagine.py): imagined pictures and their recipes, the eye's read-back tally,
what a thing could be, learning it safely, imagining a named thing, dream rounds, reacting to what was learned,
the terminal pictures, and the history. Everything happens in temporary folders: never the real neurons.

Usage: python3 tests/test_imagine.py
"""
import contextlib
import io
import os
import random
import sys
import time
from collections import Counter

from terminal import folder
import eye  # noqa: E402 (terminal finds brick)
import imagine  # noqa: E402
import senses  # noqa: E402
import sight  # noqa: E402
import symbols  # noqa: E402

KNOWLEDGE = """# a small, made-up ConceptNet
apple\tRelatedTo\tred\t9.3
apple\tRelatedTo\tround\t4.1
apple\tIsA\tfruit\t2
tomato\tRelatedTo\tred\t9
tomato\tRelatedTo\tround\t4
banana\tRelatedTo\tyellow\t5
banana\tRelatedTo\tlong\t3
fire truck\tRelatedTo\tred\t3
plate\tRelatedTo\tround\t6
""" + "".join(f"apple\tRelatedTo\tthing {n}\t1\n" for n in range(30))     # apple is well known


class Apart:
    """The neurons, the imagination's files and the eye's tuned settings in a temporary folder; knowledge=True
    swaps ConceptNet for the small one above and WordNet for nothing."""

    def __init__(self, knowledge=True):
        self.knowledge = knowledge

    def __enter__(self):
        self.where = folder()
        self.saved = (symbols.FOLDER, symbols.CONCEPTNET, symbols.WORDNET, imagine.TALLY, imagine.SAMPLES,
                      imagine.PICTURES, imagine.MAX_PICTURES, eye.TUNED, senses.FOLDER, senses.WATCHING, senses.SEEN)
        symbols.FOLDER = os.path.join(self.where, "universes")
        imagine.TALLY = os.path.join(self.where, "imagination.txt")
        imagine.SAMPLES = os.path.join(self.where, "imagination-samples.txt")
        imagine.PICTURES = os.path.join(self.where, "imagination")
        eye.TUNED = os.path.join(self.where, "eye-settings.txt")
        senses.FOLDER, senses.WATCHING, senses.SEEN = (self.where, os.path.join(self.where, "watching.txt"),
                                                      os.path.join(self.where, "senses.json"))
        if self.knowledge:
            symbols.CONCEPTNET = os.path.join(self.where, "conceptnet.tsv")
            with open(symbols.CONCEPTNET, "w", encoding="utf-8") as f:
                f.write(KNOWLEDGE)
            symbols.WORDNET = os.path.join(self.where, "no-wordnet")
        return self

    def __exit__(self, *problem):
        (symbols.FOLDER, symbols.CONCEPTNET, symbols.WORDNET, imagine.TALLY, imagine.SAMPLES, imagine.PICTURES,
         imagine.MAX_PICTURES, eye.TUNED, senses.FOLDER, senses.WATCHING, senses.SEEN) = self.saved


def neuron(name, text):
    os.makedirs(symbols.FOLDER, exist_ok=True)
    with open(symbols.path_of(name), "w", encoding="utf-8") as f:
        f.write(text)


def red_circle():
    recipe = {"kind": "one", "width": 128, "height": 128, "background": "white", "background rgb": (255, 255, 255),
              "things": [], "scene": None}
    t = imagine.new_thing(random.Random(3), shape="circle", colour="red", texture="plain")
    recipe["things"].append(imagine.place(t, 128, 128, (1, 1), random.Random(3), 0.2))
    return recipe


def test_imagined_shapes_keep_true_recipes():
    rng = random.Random(11)
    for shape in imagine.SHAPES:
        t = imagine.place(imagine.new_thing(rng, shape=shape, texture="plain", avoid=("white",)), 128, 128, (1, 1),
                          rng, 0.15)
        recipe = {"kind": "one", "width": 128, "height": 128, "background": "white", "things": [t], "scene": None,
                  "background rgb": (255, 255, 255)}
        w, h, pixels = imagine.draw(recipe)
        assert t["shape"] == shape and t["area"] > 100, (shape, t.get("area"))
        drawn = Counter(sight.colour_name(*p) for p in pixels if p != (255, 255, 255))
        assert drawn.most_common(1)[0][0] == t["colour"], (shape, t["colour"], drawn)     # the colour it says
        assert t["position"] == "middle" and t["size"] in ("medium-sized", "small"), (shape, t)
        if shape in imagine.CORNERS:
            assert t["corners"] == imagine.CORNERS[shape]
    star = imagine.place(imagine.new_thing(rng, shape="star"), 128, 128, (1, 1), rng, 0.2)
    assert 5 <= star["points"] <= 8
    for _ in range(25):                                     # compositions: every thing drawn, said in words
        recipe = imagine.random_recipe(rng)
        imagine.draw(recipe)
        assert all(t["area"] > 0 for t in recipe["things"]), imagine.describe(recipe)
        assert imagine.describe(recipe) and imagine.truths(recipe)
    triangle = {"kind": "one", "width": 128, "height": 128, "background": "white", "scene": None,
                "background rgb": (255, 255, 255),
                "things": [imagine.place(dict(imagine.new_thing(rng, shape="triangle", colour="blue", texture="plain"),
                                              angle=0), 128, 128, (1, 1), rng, 0.2)]}
    w, h, pixels = imagine.draw(triangle)
    with Apart():
        path = os.path.join(symbols.FOLDER + "-pic.png")
        sight.write_png(path, w, h, pixels)
        assert eye.see(path, model=False, memory=False, read=False)["things"][0].shape == "triangle"


def test_the_eye_read_back_is_tallied():
    with Apart():
        tally = imagine.Tally()
        item = imagine.imagine_picture(red_circle(), "you", "from you: a test", tally, think=False)
        assert item["reads"]["shape"] == "round" and item["reads"]["colour"] == "red", item["reads"]
        tally.save()
        with open(imagine.TALLY, encoding="utf-8") as f:
            text = f.read()
        assert "shape: circle read as round 1 of 1" in text and "colour: red read as red 1 of 1" in text, text
        again = imagine.Tally()                                    # read back from the plain text
        assert again.counts["shape"]["circle"]["round"] == 1 and again.totals["pictures"] == 1
        assert again.accuracy("shape") == (1, 1)
        line, items = imagine.dream(2.0, 3, rng=random.Random(5))
        assert items and line.startswith("dreamt"), line
        assert imagine.Tally().totals["pictures"] == 1 + len(items)
        assert os.path.isfile(imagine.SAMPLES)                     # shape measures kept for tuning


def test_a_red_circle_could_be_an_apple():
    with Apart():
        found = imagine.associate([imagine.feature("circle"), imagine.feature("red", imagine.BASE["red"])])
        names = [c["name"] for c in found]
        assert names[0] in ("apple", "tomato") and "apple" in names[:2], found
        apple = next(c for c in found if c["name"] == "apple")
        assert apple["reason"].startswith("apple: round (shape) + red (colour); ConceptNet"), apple["reason"]
        assert "banana" not in names                               # yellow and long: nothing like it
    with Apart(knowledge=False):                                  # the real ConceptNet and WordNet, if here
        if os.path.isfile(symbols.CONCEPTNET) and os.path.getsize(symbols.CONCEPTNET) > 1000:
            found = imagine.associate([imagine.feature("circle"), imagine.feature("red", imagine.BASE["red"])])
            assert "apple" in [c["name"] for c in found[:3]], found


def test_it_learns_into_the_neuron_but_never_over_what_you_wrote():
    with Apart():
        neuron("apple", "is: fruit\n# picked in autumn\n")
        neuron("tomato", "colour: green\n")                        # you said what colour a tomato is
        features = [imagine.feature("circle"), imagine.feature("red", imagine.BASE["red"])]
        found = imagine.associate(features)
        said, used = imagine.learn(found, features)
        assert said and 0 < used <= imagine.LINKS_PER_ROUND, (said, found)
        apple = symbols.Neuron("apple")
        assert "imagination" in apple.facets["colour"]["red"].sources, apple.render()
        assert "imagination" in apple.facets["shape"]["round"].sources
        assert apple.facets["colour"]["red"].strength <= imagine.IMAGINED, apple.render()   # weaker than seeing
        assert any(s.startswith("imagined round+red") for s in apple.facets["colour"]["red"].sources)
        assert apple.facets["is"]["fruit"].yours                  # the hand-written line survives
        with open(symbols.path_of("apple"), encoding="utf-8") as f:
            assert "# picked in autumn" in f.read()
        tomato = symbols.Neuron("tomato")
        assert set(tomato.facets["colour"]) == {"green"} and tomato.facets["colour"]["green"].yours


def test_imagine_a_name_and_read_it_back():
    with Apart():
        w, h, pixels = imagine.draw(red_circle())                 # brick show apple.png as apple, first
        shown = os.path.join(imagine.PICTURES + "-apple.png")
        sight.write_png(shown, w, h, pixels)
        assert sight.look(shown, "apple", learn=True, model=False)["name"] == "apple"
        item = imagine.imagine_named("apple")
        assert item["words"].startswith("apple: red circle"), item["words"]
        assert item["eye_name"] == "apple" and item["matched"], (item["eye"], item["found"]["matches"])
        assert os.path.isfile(item["path"]) and os.path.dirname(item["path"]) == imagine.PICTURES
        last = imagine.entries(most=1)[0]
        assert last["origin"] == "you" and last["exists"] and "apple" in last["recipe"], last
        assert imagine.Tally().counts["names"]["apple"]["apple"] == 1


def test_a_dream_round_stays_within_its_budget():
    with Apart():
        started = time.monotonic()
        first = imagine.dream_round(0.5)
        cold = time.monotonic() - started
        started = time.monotonic()
        second = imagine.dream_round(0.5)
        warm = time.monotonic() - started
        assert first.startswith("dreamt") and second.startswith("dreamt"), (first, second)
        assert cold < 1.5 and warm < 0.8, (cold, warm)
        assert all(e["origin"] == "own" for e in imagine.entries(most=20))


def test_it_reacts_to_what_was_learned():
    with Apart():
        lines = imagine.react([{"what": "dot.png", "name": None, "symbols": [("round", "shape"), ("red", "colour")]}])
        assert len(lines) >= 2 and lines[0].startswith("same shape, another colour"), lines
        made = imagine.entries(most=10)
        assert made and all(e["origin"] == "learned" and e["label"] == "after learning dot.png" for e in made), made
        assert all(e["picture"].startswith("learned-") for e in made)
        neuron("apple", "shape: round (5)\ncolour: red (5)\n")
        lines = imagine.react([{"what": "notes.txt", "words": ["red", "apples", "are", "sweet"]}])
        assert lines and lines[0].startswith("imagined apple"), lines
        import brain
        learner = senses.Senses(brain.Brain(os.path.join(symbols.FOLDER, "brain.txt")))
        learner.learn("red apples are sweet")                  # brick learn keeps what the imagination reacts to
        assert learner.recent and "apples" in learner.recent[-1]["words"], learner.recent
        imagine.later([{"what": "x.txt", "words": ["apple"]}])
        assert imagine._queue
        imagine.dream(1.0, 1, rng=random.Random(2))             # the next dream round reacts to it
        assert not imagine._queue


def test_half_blocks_show_the_right_colours():
    pixels = [(255, 0, 0), (0, 255, 0), (0, 0, 255), (255, 255, 255)]    # top: red, green; bottom: blue, white
    lines = imagine.halfblock(2, 2, pixels, columns=2)
    assert lines == ["\x1b[38;2;255;0;0m\x1b[48;2;0;0;255m▀\x1b[38;2;0;255;0m\x1b[48;2;255;255;255m▀\x1b[0m"], lines
    big = imagine.halfblock(128, 128, [(10, 20, 30)] * (128 * 128), columns=40)
    assert len(big) == 20 and big[0].count("▀") == 40


def test_the_history_its_filters_and_the_cap():
    with Apart():
        imagine.MAX_PICTURES = 5
        tally = imagine.Tally()
        for origin, label in [("own", "on its own")] * 4 + [("you", "from you: brick imagine")] * 2 + \
                [("learned", "after learning a.png")] * 2:
            imagine.imagine_picture(red_circle(), origin, label, tally, think=False)
        pictures = sorted(os.listdir(imagine.PICTURES))
        assert len([p for p in pictures if p.endswith(".png")]) == 5, pictures
        assert sum(p.startswith("own-") for p in pictures) == 1, pictures   # its own went first, oldest first
        assert len(imagine.entries(most=50)) == 8                  # the log keeps every line
        assert [e["origin"] for e in imagine.entries("you", most=50)] == ["you", "you"]
        assert len(imagine.entries("own", most=50)) == 4 and len(imagine.entries(today=True, most=50)) == 8
        assert sum(e["exists"] for e in imagine.entries("own", most=50)) == 1
        item = imagine.imagine_picture(red_circle(), "own", "on its own", tally, think=False)
        assert os.path.isfile(item["path"])                       # the one just made is never the one dropped
        assert len([p for p in os.listdir(imagine.PICTURES) if p.endswith(".png")]) == 5
        out = io.StringIO()
        with contextlib.redirect_stdout(out):
            imagine.history_command(["learned", "1"])
        assert "after learning a.png" in out.getvalue() and "picture:" in out.getvalue(), out.getvalue()


def test_tuning_keeps_the_eye_within_its_safe_range():
    with Apart():
        rows = ["hexagon | fit 0.84 solidity 0.97 corners 6 points 0 elongation 1.05 holes 0 hole share 0 sides 1.1"] * 40
        rows += ["circle | fit 0.95 solidity 0.99 corners 9 points 0 elongation 1.02 holes 0 hole share 0 sides 1.2"] * 30
        with open(imagine.SAMPLES, "w", encoding="utf-8") as f:
            f.write("\n".join(rows) + "\n")
        assert eye.classify({"fit": 0.84, "solidity": 0.97, "corners": 6, "points": 0, "elongation": 1.05,
                             "holes": 0, "hole share": 0, "sides": 1.1}) == "round"
        changed = imagine.tune()
        assert changed and changed[0].startswith("round fit 0.8 ->"), changed
        tuned = eye.tuned()
        low, high = eye.TUNABLE["round fit"][1:]
        assert 0.84 < tuned["round fit"] <= high, tuned
        assert eye.classify({"fit": 0.84, "solidity": 0.97, "corners": 6, "points": 0, "elongation": 1.05,
                             "holes": 0, "hole share": 0, "sides": 1.1}) == "hexagon"
        os.remove(eye.TUNED)                                       # deleting the file: back to the defaults
        assert eye.tuned() == eye.DEFAULTS
        imagine.add_sample("seen round", {"fit": 0.82, "solidity": 0.96, "corners": 7, "points": 0,
                                          "elongation": 1.05, "holes": 0, "hole share": 0, "sides": 1.3})
        kept = imagine.tune()                                      # a real round picture would turn roundish
        assert kept and kept[0].startswith("kept round fit 0.8") and not os.path.isfile(eye.TUNED), kept


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
