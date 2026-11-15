"""The teacher (trainer/teacher.py): the model's cached readings teach brick's own brain. The model is never asked
here: its cache is stubbed, and asking it fails the test. Runs under pytest or on its own (no pytest needed)."""
import contextlib
import json
import os
import shutil
import sys
import tempfile

HERE = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
if HERE not in sys.path:
    sys.path.insert(0, HERE)
os.environ.setdefault("BRICK_TRAINER", "off")

import dictionary  # noqa: E402
from trainer import core, neural, teacher  # noqa: E402


@contextlib.contextmanager
def lessons_from(cache):
    """The teacher sees only these cached readings, keeps its notes and learns into scratch copies, and the
    model can't be asked."""
    folder = tempfile.mkdtemp(prefix="brick-teacher-test-")
    saved = (neural._cache, neural.ask, teacher.TAUGHT, teacher._trainer, dictionary.PATH, teacher.check)
    words = os.path.join(folder, "dictionary.txt")
    with open(dictionary.PATH, encoding="utf-8") as f:       # without anything the lessons here could teach
        kept = [x for x in f if not x.startswith(("splash the whole screen", "make the screen go"))]
    with open(words, "w", encoding="utf-8") as f:
        f.writelines(kept)

    def no_model(*_):
        raise AssertionError("the teacher asked the model")

    class Pupil:                          # a Trainer without the brain: brick's real builder, nothing else learns
        def __init__(self):
            self.builder, self.added = core.Builder(), []

        def learn_toward(self, *_):
            return False

        def steer(self, *_, **__):
            return False

        def check(self):
            self.added = []

        def save(self):
            pass

    neural._cache, neural.ask = dict(cache), no_model
    teacher.check = lambda t, say=None: t.added.clear() or []     # brick's tests aren't run from inside a test
    teacher.TAUGHT, teacher._trainer, dictionary.PATH = os.path.join(folder, "taught.json"), Pupil(), words
    dictionary.clear()
    try:
        yield words
    finally:
        neural._cache, neural.ask, teacher.TAUGHT, teacher._trainer, dictionary.PATH, teacher.check = saved
        dictionary.clear()
        shutil.rmtree(folder, ignore_errors=True)


def key(line):
    return f"qwen2.5:7b|1||{line}"


def test_pattern_generalises_colours_numbers_and_words():
    assert teacher.pattern("set background to purple", "set the background colour to purple") == \
        "set background to a colour = set the background colour to a colour"
    assert teacher.pattern("write text that says happy days 3 times right",
                           "write happy days 3 times on the right") == \
        "write text that says something a number times right = write something a number times on the right"
    assert teacher.pattern("print cyan saying well done bottom", "write well done in cyan at the bottom") == \
        "print a colour saying something bottom = write something in a colour at the bottom"


def test_pattern_words_that_arent_colours_stay_words_to_show():
    # 'mocha' isn't a colour brick knows: 'in mocha' is part of what's written, not a fixed word of the pattern
    assert teacher.pattern("bottom side screen demo afraid function in mocha",
                           "write demo afraid function in mocha at the bottom") == \
        "bottom side screen something = write something at the bottom"


def test_pattern_refuses_what_it_cant_say_safely():
    refused = [
        ("clear the screen", "wait 2 seconds"),                       # no gap: a wrong reading of one line
        ("console", "wait 1 second"),
        ("save the links to links.txt", "write links to links.txt"),  # 'the' dropped from the words
        ("announce it in scarlet", "make it say txt in scarlet after 2 seconds"),   # a number made up
        ("link creature with the words in fact", "set the colour to green"),       # a colour made up
        ("the words lets go at the bottom screen white", "write see you soon in cyan at the top"),
        ("type good morning to the TOP X5", "write good morning in the top"),     # a count it can't gap
        ("print the words nice one in tal colour", "write nice one on the right"),  # a place made up
        ("print violent console in canary", "write violent console"),             # 'in canary' left out
        ("speak to me in", "make it say in"),                                     # shows only 'in'
        ("paint red and blue", "set the background colour to red then write blue"),   # two colours
    ]
    for messy, meant in refused:
        assert teacher.pattern(messy, meant) is None, (messy, teacher.pattern(messy, meant))


def test_meaning_check_refuses_dropped_or_made_up_words():
    assert teacher.changes_meaning("put official go in the corner of the screen", "write official go in the middle")
    assert teacher.changes_meaning("write game over in pink", "set the background colour to red")
    assert teacher.changes_meaning("flash haste", "wait 1 second")
    assert teacher.changes_meaning("clear the screen", "wait 2 seconds")


def test_meaning_check_accepts_slips_counts_and_same_words():
    assert teacher.changes_meaning("print the words nice one in tal colour", "write nice one in teal") is None
    assert teacher.changes_meaning("wait for 3 secs then make bg navy x5",
                                   "wait 3 seconds then set the background colour to navy 5 times") is None
    assert teacher.changes_meaning("splash the whole screen purple", "set the background colour to purple") is None
    assert teacher.changes_meaning("write hi at the centre", "write hi in the middle") is None


def test_a_reading_that_changes_the_meaning_is_refused():
    line = "put official go in the corner of the screen"
    with lessons_from({key(line): ["write official go in the middle"]}) as words:
        before = open(words).read()
        got = teacher.teach(5, say=lambda s: None)
        assert got["learned"] == [] and got["refused"] == 1
        assert json.load(open(teacher.TAUGHT))[line].startswith("the model's reading changes the meaning")
        assert open(words).read() == before


def test_align_matches_pieces_to_directions():
    assert teacher.align(["wipe", "everything off", "say hi"], ["clear the screen", "write hi"]) == \
        [("wipe everything off", "clear the screen"), ("say hi", "write hi")]
    assert teacher.align(["wait for 3 seconds", "set background to TEAL"],
                         ["wait 3 seconds", "write hello", "set the background colour to teal"]) == \
        [("wait for 3 seconds", "wait 3 seconds"), ("set background to TEAL", "set the background colour to teal")]


def test_lessons_once_per_line_and_not_while_typing():
    cache = {key("write yellow on con"): ["write yellow on the console"],
             key("write yellow on console"): ["write yellow on the console"],
             "qwen2.5:7b|1|a line before|set bg to red": ["set the colour to red"],
             key("set bg to red"): ["set the background colour to red"],
             "choose|qwen2.5:7b|1||x|a||b": 0}
    with lessons_from(cache):
        assert teacher.lessons() == [("write yellow on console", "write yellow on the console"),
                                     ("set bg to red", "set the background colour to red")]


def test_a_reading_that_doesnt_build_is_refused():
    line = "frobnicate the wibble"
    with lessons_from({key(line): ["zorp the wibble quux"]}) as words:
        before = open(words).read()
        got = teacher.teach(5, say=lambda s: None)
        assert got["learned"] == [] and got["refused"] == 1
        assert json.load(open(teacher.TAUGHT))[line] == "the model's reading doesn't build"
        assert open(words).read() == before                    # nothing learned from it


def test_a_lesson_is_learned_only_when_it_builds_exactly():
    line = "splash the whole screen purple"
    with lessons_from({key(line): ["set the background colour to purple"]}) as words:
        want = teacher._trainer.builder.build("set the background colour to purple", by_itself=True)
        assert teacher._trainer.builder.build(line) != want    # brick can't read it yet
        got = teacher.teach(5, say=lambda s: None)
        assert got["learned"] == [line]
        assert "splash the whole screen a colour = set the background colour to a colour" in open(words).read()
        assert teacher._trainer.builder.build(line) == want
        assert teacher._trainer.builder.build("splash the whole screen green") == \
            teacher._trainer.builder.build("set the background colour to green", by_itself=True)


def test_teaching_never_asks_the_model():
    with lessons_from({key("make the screen go dark blue"): ["set the background colour to dark blue"]}):
        os.environ.pop("BRICK_NEURAL", None)
        teacher.teach(5, say=lambda s: None)                   # no_model would raise if it were asked
        assert "BRICK_NEURAL" not in os.environ                # switched off only while teaching


def test_check_runs_the_tests_once_when_nothing_new_fails():
    from trainer import core
    runs, removed, added = [], [], []
    failing = {"already broken.py"}

    class Pupil:
        builder, added = None, []

        def save(self):
            pass

    saved = (core.broken, core.remove_line, core.add_line, teacher._already)
    core.broken = lambda builder: runs.append(1) or set(failing)
    core.remove_line, core.add_line = (lambda p, l: removed.append(l)), (lambda p, l: added.append(l))
    teacher._already = None
    try:
        pupil = Pupil()
        pupil.added = [("dictionary.txt", "one = write one")]
        assert teacher.check(pupil, say=lambda s: None) == [] and len(runs) == 2   # first time: with and without
        assert added == ["one = write one"]                                         # put back: not to blame
        pupil.added = [("dictionary.txt", "two = write two")]
        assert teacher.check(pupil, say=lambda s: None) == [] and len(runs) == 3   # then once is enough
        failing.add("newly broken.py")
        pupil.added = [("dictionary.txt", "three = write three")]
        core.broken = lambda builder: runs.append(1) or (set(failing) if "three = write three" not in removed
                                                         else {"already broken.py"})
        assert teacher.check(pupil, say=lambda s: None) == ["three = write three"]  # it broke one: taken back
        assert "three = write three" not in added
    finally:
        core.broken, core.remove_line, core.add_line, teacher._already = saved


if __name__ == "__main__":
    failed = 0
    for name, test in [(n, v) for n, v in list(globals().items()) if n.startswith("test_")]:
        try:
            test()
        except Exception as error:
            failed += 1
            print(f"FAILED {name}: {type(error).__name__}: {error}")
    sys.exit(1 if failed else 0)
