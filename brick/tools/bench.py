#!/usr/bin/env python3
"""Precision bench: how exactly does brick turn MESSY English into the program that was meant?

40 hard misread paragraphs from trainer/data/scrambled.txt (the scrambler's '✗' lines, each with a
'meant:' line in brick's tidy words). For each: build the messy text the way `brick build` does, build the
meant text by itself, and compare the program bytes. Same bytes = pass. Nothing else counts.

    python3 tools/bench.py              brain only (language model off)      -> "brain: N/40 exact"
    python3 tools/bench.py --model      the language model (trainer/neural.py) on, brick builds as usual
    python3 tools/bench.py --pieces     the model reads each paragraph one direction at a time (neural.read_pieces)
                                        and brick builds that reading
    python3 tools/bench.py -v           also every failure: messy, meant, what brick understood instead
    python3 tools/bench.py --pick       choose the 40 again from scrambled.txt (rewrites tools/bench40.txt)
    python3 tools/bench.py --fresh      (with --model/--pieces) ask the model everything again, not its saved answers

The 40 are kept in tools/bench40.txt, so every run uses the same set. How they were picked (--pick):
  - only '✗' pairs (brick misread them when the scrambler wrote them down);
  - not ones that meant moving the window: back then 'window' meant the console, now it means brick's own
    window, so the right program for them isn't clear any more;
  - the meant text must build by itself;
  - hardest first: most directions in the meant text, then most words in the messy text, then file order.

Builds run in a scratch folder with an empty mywords.txt. Nothing is written into the repo: the model's saved
answers are read from a scratch copy, and any write into the repo is refused (and counted) while it runs.
"""
import argparse
import builtins
import os
import re
import shutil
import sys
import tempfile
import time

HERE = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
SCRAMBLED = os.path.join(HERE, "trainer", "data", "scrambled.txt")
SET = os.path.join(HERE, "tools", "bench40.txt")
COUNT = 40


# ---------------- the pairs ----------------

def read_pairs(path):
    """'✗ messy' followed by '    meant: tidy' -> [(messy, meant)]. '#' lines are notes."""
    with open(path, encoding="utf-8") as f:
        lines = f.read().split("\n")
    pairs = []
    for k, line in enumerate(lines):
        if line.startswith("✗ ") and k + 1 < len(lines) and lines[k + 1].strip().startswith("meant:"):
            pairs.append((line[2:].strip(), lines[k + 1].split("meant:", 1)[1].strip()))
    return pairs


def pick(build):
    """Choose the 40 from scrambled.txt (see the top of this file) and write them to tools/bench40.txt."""
    candidates = []
    for k, (messy, meant) in enumerate(read_pairs(SCRAMBLED)):
        if re.search(r"\bwindow\b", meant):
            continue
        if build(meant, alone=True) is None:
            continue
        candidates.append((-(meant.count(" then ") + 1), -len(messy.split()), k, messy, meant))
    chosen = sorted(candidates)[:COUNT]
    with open(SET, "w", encoding="utf-8") as f:
        f.write("# brick precision bench: 40 hard misread paragraphs from trainer/data/scrambled.txt\n"
                "# (made by python3 tools/bench.py --pick; see the top of tools/bench.py for how they were chosen)\n")
        for _, _, k, messy, meant in chosen:
            f.write(f"✗ {messy}\n    meant: {meant}\n")
    return [(messy, meant) for *_, messy, meant in chosen]


# ---------------- no writes into the repo ----------------

blocked = []


def guard(scratch):
    """Refuse every write inside the repo while the bench runs (the scratch folder is fine)."""
    real_open, real_replace = builtins.open, os.replace

    def inside(path):
        try:
            full = os.path.realpath(os.fspath(path))
        except TypeError:
            return False
        return full.startswith(HERE + os.sep) and not full.startswith(scratch + os.sep)

    def safe_open(file, mode="r", *args, **kwargs):
        if isinstance(file, (str, bytes, os.PathLike)) and any(c in mode for c in "wax+") and inside(file):
            blocked.append(os.fspath(file))
            raise PermissionError(f"bench: not writing {file}")
        return real_open(file, mode, *args, **kwargs)

    def safe_replace(src, dst, *args, **kwargs):
        if inside(dst):
            blocked.append(os.fspath(dst))
            raise PermissionError(f"bench: not writing {dst}")
        return real_replace(src, dst, *args, **kwargs)

    builtins.open, os.replace = safe_open, safe_replace


# ---------------- building ----------------

class Builder:
    def __init__(self, folder):
        self.words = os.path.join(folder, "mywords.txt")

    def build(self, text, alone=False, watch=False):
        """The program's bytes (None if brick refuses). alone: just the tidy text, no rereading.
        watch: -> (bytes or None, what brick understood)."""
        from compiler import Compiler
        from kiln import Watching
        compiler = (Watching if watch else Compiler)(self.words, ask_user=lambda question: "no")
        program, why = None, ""
        try:
            program = compiler.compile_text(text) if alone else compiler.compile(text)
        except Exception as problem:
            why = str(problem).strip().split("\n")
            why = "refused: " + (why[1].strip() if len(why) > 1 else why[0])
        finally:
            if os.path.exists(self.words):
                os.remove(self.words)
        if not watch:
            return program
        return program, why or understood(compiler)


def understood(compiler):
    """What brick read the line as, in its own words."""
    pieces = []
    for number in sorted(compiler.tidied):
        pieces += [after for _, after in compiler.tidied[number]]
    if not pieces:
        for number in sorted(compiler.splits):
            pieces += [p["words"] for p in compiler.splits[number]]
    return " then ".join(pieces)


def directions(text):
    return [p.strip() for p in re.split(r"\bthen\b", text) if p.strip()]


def differences(builder, meant, read):
    """Which meant directions brick didn't make, and what it made instead: each direction built by itself and
    compared by its bytes, so 'on bottom' and 'at the bottom' count as the same. -> (missing, instead)."""
    built = {}

    def bits(text):
        if text not in built:
            built[text] = builder.build(text, alone=True)
        return built[text]

    want, got = directions(meant), directions(read)
    have = {bits(d) for d in got} - {None}
    wanted = {bits(d) for d in want} - {None}
    return [d for d in want if bits(d) not in have], [d for d in got if bits(d) not in wanted]


# ---------------- main ----------------

def main():
    parser = argparse.ArgumentParser(description="brick precision bench: messy English -> the exact meant bits")
    mode = parser.add_mutually_exclusive_group()
    mode.add_argument("--brain", action="store_true", help="language model off (the default)")
    mode.add_argument("--model", action="store_true", help="the language model reads too (brick's own build)")
    mode.add_argument("--pieces", action="store_true",
                      help="the model reads each paragraph one direction at a time; brick builds its reading")
    parser.add_argument("-v", "--verbose", action="store_true", help="show every failure")
    parser.add_argument("--pick", action="store_true", help="choose the 40 again (rewrites tools/bench40.txt)")
    parser.add_argument("--fresh", action="store_true", help="with --model/--pieces: don't use the model's saved answers")
    args = parser.parse_args()

    if os.environ.get("PYTHONHASHSEED") != "0":           # same order of sets every run: same results every run
        os.execve(sys.executable, [sys.executable] + sys.argv, dict(os.environ, PYTHONHASHSEED="0"))

    use_model = args.model or args.pieces
    os.environ["BRICK_NEURAL"] = "off"                   # the set is chosen with the model off, always

    scratch = os.path.realpath(tempfile.mkdtemp(prefix="brick-bench-"))
    sys.path.insert(0, HERE)
    os.chdir(scratch)
    try:
        from trainer import neural
        builder = Builder(scratch)
        if args.pick or not os.path.exists(SET):
            pairs = pick(builder.build)
            print(f"Picked {len(pairs)} paragraphs into {os.path.relpath(SET, HERE)}.")
        else:
            pairs = read_pairs(SET)
        if use_model:
            os.environ.pop("BRICK_NEURAL", None)
            copy = os.path.join(scratch, "neural.json")
            if not args.fresh and os.path.exists(neural.CACHE):
                shutil.copy(neural.CACHE, copy)
            neural.CACHE = copy
            if not neural.ready():
                print(f"model: not run (Ollama or {neural.settings()['model']} isn't ready, or it's switched off)")
                return 2
        guard(scratch)

        name = "brain" if not use_model else \
            f"{'pieces' if args.pieces else 'model'} ({neural.settings()['model']})"
        started, passed, failures = time.perf_counter(), 0, []
        for messy, meant in pairs:
            want = builder.build(meant, alone=True)
            reading = neural.read_pieces(messy) if args.pieces else None     # None: brick reads it itself
            got, read = builder.build(reading or messy, watch=True)
            if want is not None and got == want:
                passed += 1
            else:
                failures.append((messy, meant, reading, read if want is not None else "(the meant text doesn't build)"))
        took = time.perf_counter() - started

        if args.verbose:
            for n, (messy, meant, reading, read) in enumerate(failures, 1):
                print(f"\n{n}. messy:      {messy}\n   meant:      {meant}")
                if reading:
                    print(f"   model read: {reading}")
                print(f"   understood: {read}")
                if read.startswith(("refused", "(")):
                    continue
                lost, extra = differences(builder, meant, read)
                if lost:
                    print(f"   missing:    {' | '.join(lost)}")
                if extra:
                    print(f"   instead:    {' | '.join(extra)}")
                if not lost and not extra:
                    print("   (the same directions, but not the same program: order or how they're joined)")
            print()
        print(f"{name}: {passed}/{len(pairs)} exact  ({took:.0f}s)")
        if blocked:
            print(f"(refused {len(blocked)} writes into the repo: {', '.join(sorted(set(blocked)))})")
        return 0
    finally:
        shutil.rmtree(scratch, ignore_errors=True)


if __name__ == "__main__":
    sys.exit(main())
