"""The digital brain reads messy lines (trainer/mind.py: the universal linkage, learned from the archetypes)."""
import os
import sys
import tempfile

HERE = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
sys.path.insert(0, HERE)
os.environ["BRICK_NEURAL"] = "off"
os.environ["BRICK_MEMORY"] = os.path.join(tempfile.mkdtemp(prefix="mind-test-"), "empty-memory.txt")

from trainer import links, mind  # noqa: E402


def test_the_brain_reads_and_builds():
    """Lines the brain is sure of come back in brick's words, and build."""
    if mind.off() or not os.path.isfile(links.PATH):
        return                        # nothing learned yet (python3 -m trainer.mind learn)
    for line, meant in (("save you win to notes.txt", "save you win to notes.txt"),
                        ("bg blue", "set the background colour to blue"),
                        ("write hello in brown at window", "write hello in the window in brown")):
        assert mind.read(line) in (meant, None) and mind.read_line(line)[0] == meant, (line, mind.read_line(line))


def test_links_hold_a_known_thing_together():
    """'orange juice' stays together as words to show; 'juice in orange' is juice in the colour orange."""
    if mind.off() or not os.path.isfile(links.PATH):
        return
    brick, roles, gap, words = links.reading("write orange juice at the top")
    assert roles[1:3] == ["W", "W"], roles
    brick, roles, gap, words = links.reading("write juice in orange")
    assert roles[1] == "W" and roles[3] == "C", roles


def test_every_word_is_accounted_for():
    """A reading can't drop a word the brain linked to a role (a time can't vanish into 'clear the screen')."""
    from trainer import details
    assert not details.accounts_for("clear the screen", ["V", "T", "X"])
    assert details.accounts_for("wait 4 seconds", ["V", "T", "X"])


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
