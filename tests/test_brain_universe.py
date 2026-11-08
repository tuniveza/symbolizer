"""Tests for brick's brain in the universe: a word nothing else knows, read as the word it's used like.

Usage: python3 tests/test_brain_universe.py
"""
import os
import sys

from terminal import build, folder, run
from compiler import Compiler  # noqa: E402 (terminal finds brick)
from expressions import Problem  # noqa: E402
import brain  # noqa: E402

RED, RESET = "\x1b[38;2;255;0;0m", "\x1b[0m"


def output(text):
    screen, code, _ = run(build(text, folder()), (), size=(6, 40))
    assert code == 0, (code, screen)
    return screen


def read_as(line):
    compiler = Compiler(folder() + "/mywords.txt", ask_user=lambda question: "no")
    compiler.compile(line)
    return [note.split(" read as: ")[1] for note in compiler.notes if " read as: " in note]


def small_brain():
    """A small brain that has seen 'zorp' used just like 'red', and 'flurb' only once, like a word to show."""
    where = folder()
    mind = brain.Brain(os.path.join(where, "brain.txt"))
    for word in ("red", "zorp"):
        for line in ("write hi in %s", "say hello in %s at the top", "set the background colour to %s",
                     "bg %s", "text %s", "write well done in %s 3 times"):
            mind.learn_line(line % word)
    mind.learn_line("write flurb")
    mind.learn_ideas()
    mind.save()
    return mind, where


class BrainOn:
    """The brain on, with this brain file (and an empty memory, so nothing else learned counts, and no
    neural reader, whose readings would come before the brain's)."""

    def __init__(self, path, where):
        self.changes = {"BRICK_BRAIN": path, "BRICK_MEMORY": os.path.join(where, "empty.txt"),
                        "BRICK_BRAIN_THRESHOLD": str(brain.THRESHOLD), "BRICK_NEURAL": "off"}

    def __enter__(self):
        self.saved = {k: os.environ.get(k) for k in self.changes}
        os.environ.update(self.changes)

    def __exit__(self, *problem):
        for k, v in self.saved.items():
            if v is None:
                os.environ.pop(k, None)
            else:
                os.environ[k] = v


def test_weighted_overlap():
    assert brain.overlap({"x": 2, "y": 1}, {"x": 1, "z": 1}) == 1 / 4      # min 1 / max (2 + 1 + 1)
    mind = brain.Brain("")
    mind.associate("a", "x", 2)
    mind.associate("a", "y")
    mind.associate("b", "x")
    mind.associate("b", "a", 5)                  # a and b themselves are left out
    assert mind.similarity("a", "a") == 1.0
    assert mind.similarity("a", "nothing") == 0.0
    assert mind.breadth("x") == 2 and mind.breadth("y") == 1     # how many symbols have it
    # comparing a and b, a link's breadth leaves them out: 1 + others having it. x: 1 + 0, y: 1 + 0
    # a: x 2/1, y 1/1 -> shares 2/3, 1/3; b: x 1/1 -> share 1
    assert abs(mind.similarity("a", "b") - brain.overlap({"x": 2 / 3, "y": 1 / 3}, {"x": 1.0})) < 1e-9
    mind.associate("c", "x")                     # x is now broader: weaker evidence that a and b are alike
    assert abs(mind.similarity("a", "b") - brain.overlap({"x": 1 / 2, "y": 1 / 2}, {"x": 1.0})) < 1e-9
    assert abs(dict(mind.like("a"))["b"] - mind.similarity("a", "b")) < 1e-9      # the fast way: the same score


def test_unknown_word_used_like_a_colour_is_that_colour():
    mind, where = small_brain()
    assert mind.similarity("zorp", "red", route="usage") >= brain.THRESHOLD
    with BrainOn(mind.path, where):
        assert read_as("hi in zorp") == ["write hi in red ('zorp' is used like 'red')"]
        assert output("hi in zorp") == output("write hi in red")
        assert f"{RED}hi{RESET}" in output("hi in zorp")
        assert read_as("bg zorp") == ["set the background colour to red ('zorp' is used like 'red')"]
        assert read_as("write hi in zorp") == []              # brick reads that itself: it shows 'hi in zorp'
        assert "hi in zorp" in output("write hi in zorp")
        assert read_as('write "zorp" in red') == []           # quoted words are never touched


def test_below_the_threshold_is_still_refused():
    mind, where = small_brain()
    assert mind.similarity("flurb", "red", route="usage") < brain.THRESHOLD
    with BrainOn(mind.path, where):
        for line in ("hi in flurb", "hi in qwxv"):          # seen once, used otherwise; never seen at all
            try:
                build(line, folder())
            except Problem:
                pass
            else:
                raise AssertionError(f"{line!r} should have been refused")
    try:                                                  # BRICK_MEMORY=off, as in every other test: no brain
        assert brain.off()
        build("hi in zorp", folder())
    except Problem:
        pass
    else:
        raise AssertionError("'hi in zorp' should be refused with the brain off")


def test_the_brain_learns_from_a_proved_direction():
    import factory
    import synchronicity
    where = folder()
    teacher = factory.Factory.__new__(factory.Factory)     # just the parts that prove a direction
    teacher.builder = factory.Builder()
    teacher.memory = synchronicity.Memory(os.path.join(where, "memory.txt"))
    teacher.brain = brain.Brain(os.path.join(where, "brain.txt"))
    import symbols
    real_folder, symbols.FOLDER = symbols.FOLDER, os.path.join(where, "universes")   # never the real neurons
    try:
        teacher.prove("write hi in red at the top")
        with open(os.path.join(where, "universes", "_waiting.txt"), encoding="utf-8") as f:
            waiting = f.read()
    finally:
        symbols.FOLDER = real_folder
    assert "red / place: top" in waiting                  # the words of the proved direction fired together
    teacher.brain.save()
    learned = brain.Brain(os.path.join(where, "brain.txt")).links
    assert learned["red"]["[colour]"] >= 1                # red was read as a colour
    assert learned["top"]["[place]"] >= 1                 # top as a place
    assert learned["red"]["before:in"] >= 1 and learned["red"]["after:at"] >= 1    # and how they're used


def test_alike_universes_are_alike():
    where = folder()
    mind = brain.Brain("")
    os.makedirs(os.path.join(where, "universes"))
    for name, text in (("apple", "# fruit\nshape: round, sphere\nbiology: plant, seed\n"),
                       ("cherry", "shape: round\nbiology: plant, seed, stone\n"),
                       ("car", "shape: boxy\nuse: driving, wheels\n")):
        with open(os.path.join(where, "universes", name + ".txt"), "w") as f:
            f.write(text)
        mind.learn_universe_file(os.path.join(where, "universes", name + ".txt"))
    assert mind.links["apple"]["shape:round"] == 1.0 and mind.links["round"]["apple"] == 1.0
    assert mind.similarity("apple", "cherry") > 0.3 > mind.similarity("apple", "car")
    mind.learn_universe("plum", [("round", "shape", 1.0), ("plant", "biology", 1.0)])
    assert mind.like("plum")[0][0] in ("apple", "cherry")


def test_letters_only_choose_between_words_used_alike():
    where = folder()
    mind = brain.Brain(os.path.join(where, "brain.txt"))
    for word in ("blue", "teal", "blurte"):        # blurte is used just like blue and teal: too close to call
        for line in ("write hi in %s", "say hello in %s at the top", "set the background colour to %s", "bg %s"):
            mind.learn_line(line % word)
    mind.learn_ideas()
    mind.save()
    assert mind.understand("blurte", "in", None)[0::2] == ("blue", "use and letters")    # built like blue
    with BrainOn(mind.path, where):
        assert read_as("hi in blurte") == ["write hi in blue ('blurte' is used like 'blue', and built from letters like it)"]
        try:                                       # a real slip is never the brain's: spelling has it
            build("wirte hello", folder())
        except Problem:
            raise AssertionError("with the memory on, brick reads 'wirte' as write by spelling, as before")
    try:
        build("wirte hello", folder())             # BRICK_MEMORY=off: still refused
    except Problem as problem:
        assert "Did you mean 'write'?" in str(problem)
    else:
        raise AssertionError("'wirte hello' should be refused")


def test_little_words_contexts_and_letters_are_symbols():
    mind = brain.Brain("")
    for line in ("hi in red at the top", "hi in blue at the top", "hi in red on the left", "write a word"):
        mind.learn_line(line)
    assert mind.links["a"]["before:write"] == 1 and mind.links["in"]["before:hi"] == 3    # little words too
    assert mind.links["red"]["in _ at"] == 1                                # the whole slot red fills
    assert dict(mind.strongest("in _ at")) == {"red": 1.0, "blue": 1.0, "before:in": 1.0, "after:at": 1.0}
    assert mind.breadth("in _ at") == 4               # filled by red and blue; appears with before:in, after:at
    assert mind.like("in _ at")[0][0] == "in _ on"                          # filled by red too
    assert "letter:e" not in mind.links                                     # letters aren't kept in the file:
    links = dict(mind.strongest("letter:e"))                                # they're worked out from the words
    assert links["at:middle"] >= 1 and links["before:letter:r"] >= 1        # red: e after r, in the middle
    assert mind.raw("write", "form")["starts:wr"] > 0 and "ends:te" in mind.raw("write", "form")
    assert "starts:wr" not in mind.raw("write", "usage")                    # how it's used and how it's built:
    assert "before:(start)" not in mind.raw("write", "form")                # two separate scores
    assert all(other.startswith("letter:") for other, _ in mind.like("letter:d")) and mind.like("letter:d")


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
