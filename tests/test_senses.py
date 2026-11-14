"""Tests for brick trainer learn (trainer/senses.py): learning from any file, folder, picture or words, and watched folders.
Everything happens in temporary folders: never the real brain or the real neurons.

Usage: python3 tests/test_senses.py
"""
import os
import struct
import sys
import zlib

from terminal import folder
from trainer import brain  # noqa: E402 (terminal finds brick)
from trainer import senses  # noqa: E402
from trainer import sight  # noqa: E402
from trainer import symbols  # noqa: E402


class Apart:
    """The neurons, the watch list and the 'already learned' list in a temporary folder."""

    def __enter__(self):
        self.where = folder()
        self.saved = symbols.FOLDER, senses.WATCHING, senses.SEEN, senses.FOLDER
        symbols.FOLDER = os.path.join(self.where, "universes")
        senses.FOLDER = self.where
        senses.WATCHING = os.path.join(self.where, "watching.txt")
        senses.SEEN = os.path.join(self.where, "senses.json")
        self.mind = brain.Brain(os.path.join(self.where, "brain.txt"))
        return self

    def __exit__(self, *problem):
        symbols.FOLDER, senses.WATCHING, senses.SEEN, senses.FOLDER = self.saved


def png(path, width, height, colour_at):
    rows = b"".join(b"\0" + b"".join(bytes(colour_at(x, y)) for x in range(width)) for y in range(height))

    def chunk(kind, data):
        return struct.pack(">I", len(data)) + kind + data + struct.pack(">I", zlib.crc32(kind + data))
    with open(path, "wb") as f:
        f.write(b"\x89PNG\r\n\x1a\n" + chunk(b"IHDR", struct.pack(">IIBBBBB", width, height, 8, 2, 0, 0, 0)) +
                chunk(b"IDAT", zlib.compress(rows)) + chunk(b"IEND", b""))


def red_circle(x, y):
    return (220, 20, 20) if (x - 30) ** 2 + (y - 30) ** 2 < 400 else (255, 255, 255)


def a_folder(where):
    stuff = os.path.join(where, "stuff")
    os.makedirs(os.path.join(stuff, "inside"))
    with open(os.path.join(stuff, "fruit.txt"), "w") as f:
        f.write("Apples are sweet and round.\nA pear is sweet and green!\n")
    with open(os.path.join(stuff, "inside", "cars.md"), "w") as f:
        f.write("cars have wheels and engines\n")
    with open(os.path.join(stuff, "noise.bin"), "wb") as f:
        f.write(bytes(range(256)) * 8)
    with open(os.path.join(stuff, ".hidden.txt"), "w") as f:
        f.write("secret words\n")
    return stuff


def test_words_teach_the_brain_and_the_neurons():
    with Apart() as apart:
        done = senses.Senses(apart.mind).learn("zorp is a sweet round fruit")
        assert done["text"] == 1
        assert apart.mind.links["zorp"]["after:is"] >= 1 and apart.mind.links["zorp"]["before:(start)"] >= 1
        with open(os.path.join(symbols.FOLDER, "_waiting.txt"), encoding="utf-8") as f:
            assert "zorp /" in f.read()                     # its words fired together, little words too


def test_a_folder_reads_text_skips_the_rest():
    with Apart() as apart:
        stuff = a_folder(apart.where)
        done = senses.Senses(apart.mind).learn(stuff)
        assert done["text"] == 2 and done["skipped"] == 1, done      # fruit.txt, cars.md; noise.bin skipped
        assert "pear" in apart.mind.links and "engines" in apart.mind.links
        assert "secret" not in apart.mind.links                       # hidden files are left alone


def test_pictures_go_to_the_eye_and_odd_ones_never_stop_it():
    with Apart() as apart:
        stuff = os.path.join(apart.where, "pics")
        os.makedirs(stuff)
        png(os.path.join(stuff, "dot.png"), 60, 60, red_circle)
        with open(os.path.join(stuff, "broken.png"), "wb") as f:
            f.write(b"\x89PNG\r\n\x1a\nnot really")
        real_model, sight.model_sees = sight.model_sees, lambda *a, **k: None    # no vision model in tests
        try:
            done = senses.Senses(apart.mind).learn(stuff)
        finally:
            sight.model_sees = real_model
        assert done["picture"] + done["skipped"] == 2, done
        assert any(line.startswith("broken.png: skipped") for line in done["said"]), done["said"]


def test_watched_folders_learn_only_what_is_new():
    with Apart() as apart:
        stuff = a_folder(apart.where)
        assert senses.watch_round(apart.mind) == ""                   # nothing watched: nothing happens
        senses.set_watched([stuff])
        assert senses.watched() == [stuff]
        assert senses.watch_round(apart.mind).startswith("learned from 2 texts")
        assert senses.watch_round(apart.mind) == ""                   # already learned
        with open(os.path.join(stuff, "new.txt"), "w") as f:
            f.write("bananas are long and yellow\n")
        assert senses.watch_round(apart.mind) == "learned from 1 text, 0 pictures"
        assert "bananas" in apart.mind.links


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
