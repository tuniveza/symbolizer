"""Tests for the brain words: linking symbols, learning links from text, how alike two symbols are,
'is like', the closest match. Each builds and runs a real program in a pseudo-terminal.

Usage: python3 tests/test_brain_words.py
"""
import os
import sys

from terminal import build, folder, run

RESET = "\x1b[0m"
FRUIT = "link apple with fruit, sweet and tree\nlink pear with fruit, sweet and tree\nlink car with wheels and road\n"


def output(text, typing=(), where=None, code=0):
    screen, ended, _ = run(build(text, where or folder()), typing)
    assert ended == code, (ended, screen)
    return screen


def refusal(text):
    from expressions import Problem
    try:
        build(text, folder())
    except Problem as problem:
        return str(problem)
    raise AssertionError(f"{text!r} should have been refused")


def test_link_and_likeness():
    assert output(FRUIT + "write how alike apple and pear are\nwrite the likeness of apple and car\n"
                  "write how alike apple and apple are\nwrite how alike apple and banana are\n"
                  "write the links of fruit\n"
                  "link apple to red\nwrite how alike apple and pear are\n"
                  "link pear to fruit\nwrite how alike apple and pear are") == \
        "100\n0\n100\n0\napple, pear\n60\n60\n"        # shares: 9/15 (apple has red too), then 12/20


def test_is_like_and_is_not_like():
    assert output(FRUIT + "if apple is like pear, write yum\nif apple is not like car, write not a car\n"
                  "if apple is like car, write wrong\nif apple isn't like pear, write wrong too\n"
                  "set thing to \"pear\"\nif thing is like apple write yes otherwise write no") == \
        "yum\nnot a car\nyes\n"


def test_changing_the_likeness_needed():
    assert output(FRUIT + "link apple to red\nwrite the likeness needed\n"
                  "if apple is like pear write like at 80 otherwise write not like at 80\n"
                  "set the likeness needed to 70\nif apple is like pear write wrong at 70\n"
                  "set the likeness needed to 60\nif apple is like pear write like at 60\n"
                  "change the likeness needed to 100%\nwrite the likeness needed\n"
                  "if apple is like pear write wrong") == "80\nnot like at 80\nlike at 60\n100\n"


def test_closest_match():
    assert output(FRUIT + "link apple to red\nwrite the closest match to apple\nwrite the closest match to car\n"
                  "write the closest match to banana\nset thing to \"pear\"\nwrite what thing is like\n"
                  "set m to the best match for thing, then write m in capitals") == \
        "pear\n\n\napple\nAPPLE\n"


def test_learning_from_a_text():
    assert output('learn "Red apples are sweet."\nlearn "red cherries are SWEET, too!"\n'
                  "write the links of apples\nwrite the links of red\n"
                  "write how alike apples and cherries are\n"
                  "learn \"don't stop, it's 'fine' - ok ok\"\nwrite the links of don't\n"
                  "set s to \"dogs chase cats\"\nlearn from the words in s\nwrite the links of cats") == \
        "red, are, sweet\napples, are, sweet, cherries, too\n60\nstop, it's, fine, ok\ndogs, chase\n"


def test_links_with_variables_from_ask():
    screen = output("link apple with fruit and sweet\n"
                    "ask a word as w, then link w to fruit and link w to sweet\n"
                    "write the links of fruit\nwrite how alike w and apple are\n"
                    "if w is like apple write w is like apple\n"
                    "ask \"Say something:\" and learn from answer\nwrite the links of cats",
                    [("Word?", "Kiwi\n"), ("something:", "Cats chase mice\n")])
    assert screen == ("Word? Kiwi\napple, Kiwi\n100\nKiwi is like apple\n"
                      "Say something: Cats chase mice\nchase, mice\n"), repr(screen)


def test_linking_a_symbol_with_each_word():
    screen = output("ask \"Describe a cat:\" and link cat with the words in answer\n"
                    "ask \"Describe a dog:\" and link dog to every word in answer\n"
                    "write the links of cat\nwrite the links of furry\nwrite how alike cat and dog are",
                    [("cat:", "Small and FURRY, with whiskers\n"), ("dog:", "furry and loyal; it barks\n")])
    assert screen == ("Describe a cat: Small and FURRY, with whiskers\nDescribe a dog: furry and loyal; it barks\n"
                      "small, and, furry, with, whiskers\ncat, dog\n25\n"), repr(screen)


def test_the_brain_full_message():
    screen = output("set n to 0\nrepeat 2001 times: add 1 to n and link n to hub\nwrite never shown", code=1)
    assert screen == f"{RESET}\n(The brain is full: 2,000 symbols is the most it can hold.)\n", repr(screen)


def test_loose_phrasings():
    assert output("associate apple with fruit and sweet\nconnect pear to fruit, sweet\n"
                  "link sun, moon and star together\nlink up dog with bark\n"
                  "write how similar apple and pear are\nwrite how alike are apple and pear\n"
                  "write the similarity between sun and moon\nif apple looks like pear, write looks alike\n"
                  "write what apple is linked to\nwrite the links of moon\nwrite the links of dog\n"
                  "please link cat to purr then write the links of cat\n"
                  "write \"apple v pear:\" how alike apple and pear are") == \
        "100\n100\n100\nlooks alike\nfruit, sweet\nsun, star\nbark\npurr\napple v pear: 100\n"


def test_old_readings_kept():
    """Words that start brain actions stay plain words where they don't act on anything."""
    assert output("write live, laugh, learn\nwrite we connect people\nwrite I know what it is like\n"
                  "write don't forget") == "live, laugh, learn\nwe connect people\nI know what it is like\n" \
                                           "don't forget\n"


def test_unlink_and_forget():
    assert output(FRUIT + "unlink apple from tree\nwrite the links of apple\nwrite the links of tree\n"
                  "forget the link between pear and sweet\nwrite the links of pear\n"
                  "forget apple\nwrite the links of apple\nwrite the links of fruit") == \
        "fruit, sweet\npear\nfruit, tree\n\npear\n"


def test_saving_and_loading_links():
    where = folder()
    assert output("load the links from brain.txt\nwrite the links of apple\n"
                  "link apple with fruit and sweet, link apple to fruit\nsave the links to brain.txt", where=where) == "\n"
    with open(os.path.join(where, "brain.txt")) as f:
        assert f.read() == "apple\tfruit\t2\napple\tsweet\t1\n"
    assert output("load the links from brain.txt\nwrite the links of apple\nwrite how alike fruit and sweet are\n"
                  "link pear to fruit\nwrite how alike apple and pear are", where=where) == \
        "fruit, sweet\n100\n50\n"           # fruit (apple 2) v sweet (apple 1): the same share; pear v apple: 2/4


def test_only_programs_with_brain_words_carry_the_brain():
    from compiler import Compiler
    plain = Compiler(os.path.join(folder(), "mywords.txt")).compile("write hi")
    brainy = Compiler(os.path.join(folder(), "mywords.txt")).compile("link a to b")
    assert b"The brain is full" not in plain and b"The brain is full" in brainy


def test_refusals():
    assert "Link it to what?" in refusal("link apple")
    assert "Learn from what?" in refusal("learn")
    assert "Unlink what from what?" in refusal("unlink apple")
    assert "Did you mean 'link'?" in refusal("lnik apple to fruit")


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
