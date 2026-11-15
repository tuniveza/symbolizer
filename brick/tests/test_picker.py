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
