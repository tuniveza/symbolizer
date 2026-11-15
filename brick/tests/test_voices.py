"""Archetype voices: the language model's messy rewrites are kept only when they mean exactly what was meant.
The model is stubbed, so these run without Ollama.

Usage: python3 tests/test_voices.py
"""
import json
import sys

import terminal  # noqa: F401 (finds brick)
from trainer import neural, voices  # noqa: E402

ORACLE = {"name": "The Mirror Oracle", "type": "INFJ", "bias": "strict", "yours": True,
          "line": "Sees itself in others and the future in both."}
TIDY = ["write hello there in red at the top", "wait 3 seconds"]


def answering(parts):
    """Stand in for the model: it answers these parts, whatever it's asked."""
    asked = []

    def call(path, body=None, seconds=60):
        asked.append(body)
        return {"message": {"content": json.dumps({"parts": parts})}}
    neural.call, neural.ready, neural.off = call, (lambda: True), (lambda: False)
    return asked


def stubbed(test):
    def run():
        saved = neural.call, neural.ready, neural.off
        try:
            test()
        finally:
            neural.call, neural.ready, neural.off = saved
    run.__name__ = test.__name__
    return run


@stubbed
def test_good_rewrite_is_kept():
    asked = answering(["ok so TOP of screen hello there in red pls", "then like 3 secs wait"])
    found = voices.voiced(TIDY, ORACLE)
    assert found is not None
    mess, name = found
    assert name == "The Mirror Oracle"
    assert "hello there" in mess and "3 secs" in mess
    assert "Mirror Oracle" in asked[0]["messages"][0]["content"]       # the voice steers the model
    assert asked[0]["model"] == neural.settings()["model"]


@stubbed
def test_dropped_words_to_show_are_refused():
    answering(["ok so TOP of screen hello in red pls", "then like 3 secs wait"])
    assert voices.voiced(TIDY, ORACLE) is None
    answering(["ok so TOP of screen Hello There in red pls", "then like 3 secs wait"])
    assert voices.voiced(TIDY, ORACLE) is None                           # changed: not exactly the same


@stubbed
def test_dropped_or_changed_numbers_are_refused():
    answering(["top hello there in red", "then wait a bit"])
    assert voices.voiced(TIDY, ORACLE) is None
    answering(["top hello there in red", "then wait 5 secs"])
    assert voices.voiced(TIDY, ORACLE) is None


@stubbed
def test_lost_or_new_colours_places_and_words_are_refused():
    for parts in (["top hello there pls", "wait 3 secs"],                     # colour dropped
                  ["top hello there in red and blue", "wait 3 secs"],         # a new colour
                  ["hello there in red", "wait 3 secs"],                      # the place dropped
                  ['top hello there in red then say "bye"', "wait 3 secs"],   # new words to show
                  ["top hello there in red, game over", "wait 3 secs"],       # new words to show
                  ["top hello there in red (show: hello there)", "wait 3 secs"],  # the notes copied
                  ["top hello there in red then wait 3 secs"]):               # a direction short
        answering(parts)
        assert voices.voiced(TIDY, ORACLE) is None, parts


def test_problem_explains_itself():
    assert voices.problem("hello there in red at top", TIDY[0]) is None
    assert "hello there" in voices.problem("hi there in red at top", TIDY[0])
    assert "numbers" in voices.problem("wait 2 secs", TIDY[1])
    assert voices.shows("after 4 seconds write game over in teal at the left 3 times") == ["game over"]


@stubbed
def test_no_model_means_no_voice():
    answering(["anything"])
    neural.off = lambda: True
    assert voices.voiced(TIDY, ORACLE) is None


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
