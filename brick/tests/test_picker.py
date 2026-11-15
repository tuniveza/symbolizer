"""The brain picks the bricks (trainer/picker.py): which action each direction of a line becomes."""
import os
import sys
import tempfile

HERE = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
sys.path.insert(0, HERE)
os.environ["BRICK_NEURAL"] = "off"
os.environ["BRICK_MEMORY"] = os.path.join(tempfile.mkdtemp(prefix="picker-test-"), "empty-memory.txt")

from trainer import picker  # noqa: E402


def test_the_brain_picks_bricks():
    if picker.off():
        return                        # nothing learned yet (python3 -m trainer.picker label, then learn)
    for line, brick in (("write hi in red", "write"), ("wipe the screen", "clear"), ("make the bg purple", "set"),
                        ("write red text on window at left side", "window write in window"),
                        ("Moev the console to the left", "put"), ("lol", "none")):
        assert picker.same([picker.whole_pick(line)], [brick]), (line, picker.Picker().pick(line))


def test_a_long_line_is_its_bricks_in_order():
    if picker.off():
        return
    assert picker.line_bricks("make the bg blue then after 2 secs say hi in red lol") == ["set", "write"]


def test_the_brain_has_the_last_word_on_a_misspelled_line():
    """'wirte ... winodw' is a write in brick's own window, not 'red text on winodw' in the console."""
    if picker.off():
        return
    _, ok, got = picker.label_one("wirte red text on winodw at left side")
    assert ok and got == ["window write in window"], got


def test_the_brain_says_the_details():
    """The brain's whole reading: the bricks and every detail (words, colour, place, count, time)."""
    if picker.off():
        return
    from trainer import details
    assert details.read("ok so TOP of screen hello there in red;bg blue   then 2 secs wait lol") == \
        "write hello there in red at the top then set the background colour to blue then wait 2 seconds"
    assert details.read('"ready" in teal colour type 5 times at bottom side of screen rn then put the console at '
                        'left side screen !!') == \
        "write ready in teal at the bottom 5 times then put the console on the left side of the screen"


def test_the_brain_holds_back_when_unsure():
    """Roles that don't hang together, or a word with no role: the brain says nothing, brick reads it."""
    if picker.off():
        return
    from trainer import details
    for line in ("write she was saying hi", "write hello in blue at the x 20 and y 20", "state hello in red",
                 "hi in zorp", "Hide the message after 4 seconds", "pause moment"):
        assert details.read(line) is None, (line, details.read(line))


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
