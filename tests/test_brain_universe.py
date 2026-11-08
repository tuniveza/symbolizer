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
    assert abs(mind.similarity("a", "b") - brain.overlap({"x": 2 / 3, "y": 1 / 3}, {"x": 1.0})) < 1e-9
    assert abs(dict(mind.like("a"))["b"] - mind.similarity("a", "b")) < 1e-9      # the fast way: the same score


def test_unknown_word_used_like_a_colour_is_that_colour():
    mind, where = small_brain()
    assert mind.similarity("zorp", "red", only_usage=True) >= brain.THRESHOLD
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
    assert mind.similarity("flurb", "red", only_usage=True) < brain.THRESHOLD
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
    teacher.prove("write hi in red at the top")
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
    assert mind.similarity("apple", "cherry") > 0.5 > mind.similarity("apple", "car")
    mind.learn_universe("plum", [("round", "shape", 1.0), ("plant", "biology", 1.0)])
    assert mind.like("plum")[0][0] in ("apple", "cherry")


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
