"""The brain picks the bricks: which action (brick) a line becomes, read by the brain from the line's own words.

The hand-written readers (compiler.py, actions.py, bitwords.py) are the checker here, not the reader. They label
the sentence pool once: each line is built, and the brick every direction became is noted (write, wait, set,
a window phrase, one of your own words ...). The brain then links what it sees in a line (its words, where each
word stands, the words on either side, the shape of each word) to the bricks those lines became, with the brain's
own machinery (trainer/brain.py: weighted links, a broad link is weak evidence, a rare one strong). It keeps those
links in trainer/data/brain-bricks.txt, plain text like brain.txt.

To pick, the brain weighs every brick by the links the line's symbols have to it and takes the strongest.

    python3 -m trainer.picker label [lines]     build the pool's lines and note their bricks (data/brick-labels.tsv)
    python3 -m trainer.picker learn             the brain learns the links from the labels
    python3 -m trainer.picker try "a line"      which brick the brain picks, and why
    python3 -m trainer.picker score             the brain's picks against the checker's, on lines it never saw,
                                                tidy and messed up (scrambler.mess), and on the bench
"""

import json
import math
import os
import random
import re
import sys
import tempfile

HERE = os.path.dirname(os.path.abspath(__file__))
sys.path.insert(0, os.path.dirname(HERE))

POOL = os.path.join(HERE, "data", "sentences.txt")
LABELS = os.path.join(HERE, "data", "brick-labels.tsv")
LINKS = os.path.join(HERE, "data", "brain-bricks.txt")
HELD_OUT = 0.15            # lines the brain never learns from, for the score (chosen by a fixed hash)


# ---------------- the checker labels the pool ----------------

def held_out(line):
    """A fixed fifth-ish of the lines, the same every run: the brain never learns from these."""
    return (sum(map(ord, line)) * 2654435761 % 1000) / 1000 < HELD_OUT


def label_one(line):
    import compiler
    from expressions import Problem
    folder = tempfile.mkdtemp(prefix="picks-")
    c = compiler.Compiler(os.path.join(folder, "mywords.txt"), ask_user=lambda question: "no")
    got = []
    try:
        c.compile(line)                                  # the whole reader, as in real use (re-reads and all)
        ok = True
        got = [b for n in sorted(c.bricks_on) for b in c.bricks_on[n]]   # the bricks of the reading that built
    except Problem:
        ok = False
    except Exception:
        ok = False
    finally:
        try:
            os.remove(os.path.join(folder, "mywords.txt"))
        except OSError:
            pass
        os.rmdir(folder) if not os.listdir(folder) else None
    return line, ok, got


def label(count=None, workers=8):
    from multiprocessing import Pool
    os.environ.setdefault("BRICK_NEURAL", "off")
    os.environ["BRICK_PICKER"] = "off"                 # the labels are the hand-written readers' alone
    from trainer import neural
    neural.patient = False
    with open(POOL, encoding="utf-8") as f:
        lines = [l.strip() for l in f if l.strip() and "\t" not in l]
    lines = sorted(set(lines))
    if count:
        random.Random(1).shuffle(lines)
        lines = lines[:count]
    done = 0
    with Pool(workers) as pool, open(LABELS + ".new", "w", encoding="utf-8") as out:
        for line, ok, got in pool.imap_unordered(label_one, lines, chunksize=8):
            if ok and got:
                out.write(f"{line}\t{' | '.join(got)}\n")
            done += 1
            if done % 500 == 0:
                print(f"  {done} of {len(lines)}", flush=True)
    os.replace(LABELS + ".new", LABELS)
    print(f"Labelled {done} lines -> {LABELS}")


def labels():
    """[(line, [bricks])] from data/brick-labels.tsv."""
    found = []
    with open(LABELS, encoding="utf-8") as f:
        for row in f:
            line, _, bricks = row.rstrip("\n").partition("\t")
            if bricks:
                found.append((line, bricks.split(" | ")))
    return found


# ---------------- what the brain sees in a line ----------------

def tokens(line):
    return re.findall(r"[a-z0-9']+|\"", line.lower())


_known = None


def respell(ws):
    """Words the brain has never seen in a line ('winddow', 'moev') -> the known word they're nearest to,
    by letters, when one is close enough."""
    global _known
    import difflib
    if _known is None:
        _known = set()
        if os.path.isfile(LINKS):
            with open(LINKS, encoding="utf-8") as f:
                for row in f:
                    parts = row.split("\t")
                    if len(parts) == 4 and re.fullmatch(r"[a-z']+", parts[1]):
                        _known.add(parts[1])
    out = []
    for w in ws:
        if len(w) > 3 and w.isalpha() and _known and w not in _known:
            near = difflib.get_close_matches(w, _known, n=1, cutoff=0.8)
            w = near[0] if near else w
        out.append(w)
    return out


def symbols(line):
    """The symbols the brain sees in a line: each word, the first words, each word with its neighbours, and
    the shape of each word (its start and end), so a slip ('wirte') still looks like its word."""
    ws = respell(tokens(line))
    quoted, seen = False, []
    for k, w in enumerate(ws):
        if w == '"':
            quoted = not quoted
            continue
        if quoted:
            seen.append("(quoted)")
            continue
        seen.append(w)
    found = {}

    def add(s, weight=1.0):
        found[s] = found.get(s, 0.0) + weight

    for k, w in enumerate(seen[:12]):
        add(w)
        if k < 3:
            add(f"at{k}:{w}", 1.5)
        before = seen[k - 1] if k else "(start)"
        add(f"{before}+{w}")
        if len(w) > 3 and not w.startswith("("):
            add(f"starts:{w[:3]}", 0.5)
            add(f"ends:{w[-3:]}", 0.5)
            if k < 3:
                add(f"at{k}:starts:{w[:3]}", 0.5)
    add(f"length:{min(len(seen), 8)}", 0.5)
    content = [w for w in seen[:12] if w not in SMALL and not w.startswith("(")]
    for a in range(len(content)):                      # words that mean something together: 'pen' and 'window'
        for b in range(a + 1, len(content)):
            if content[a] != content[b]:
                add(f"{content[a]}&{content[b]}", 0.3)
    return found


SMALL = {"the", "a", "an", "of", "to", "in", "on", "at", "and", "then", "it", "is", "also", "first", "now", "next",
         "finally", "after", "that", "please", "pls", "just", "so", "ok", "can", "you"}


# ---------------- the brain learns and picks ----------------

class Picker:
    def __init__(self, path=LINKS):
        from trainer.brain import Brain
        self.brain = Brain(path)

    def learn(self, examples, rounds=10):
        """The brain learns from its mistakes: it picks a brick for each line; when the pick is wrong, every
        symbol of the line gets a stronger link to the right brick and a weaker one to the wrong pick. Rounds
        over all the lines, and the links kept are the average over the whole practice (steadier than the last
        round's)."""
        links, sums, step = {}, {}, 1
        bricks = sorted(set(b for _, b in examples))
        seen = [(symbols(line), brick) for line, brick in examples]
        for r in range(rounds):
            wrong = 0
            order = list(seen)
            random.Random(r).shuffle(order)
            for found, brick in order:
                pick = self.best(found, links, bricks)
                if pick != brick:
                    wrong += 1
                    for s, w in found.items():
                        for to, sign in ((f"brick:{brick}", 1), (f"brick:{pick}", -1)):
                            links.setdefault(s, {})[to] = links.get(s, {}).get(to, 0.0) + sign * w
                            sums.setdefault(s, {})[to] = sums.get(s, {}).get(to, 0.0) + sign * w * step
                step += 1
            print(f"  practice round {r + 1}: {wrong} of {len(order)} picked wrong", flush=True)
        b = self.brain
        b.links = {s: {to: w - sums[s].get(to, 0.0) / step for to, w in linked.items()}
                   for s, linked in links.items()}
        b.total = sum(abs(w) for to in b.links.values() for w in to.values())
        b.changed = True

    @staticmethod
    def best(found, links, bricks):
        score = {}
        for s, w in found.items():
            for to, v in links.get(s, {}).items():
                score[to[6:]] = score.get(to[6:], 0.0) + w * v
        return max(bricks, key=lambda x: score.get(x, 0.0))

    def scores(self, line):
        score = {}
        for s, w in symbols(line).items():
            for to, v in self.brain.links.get(s, {}).items():
                score[to[6:]] = score.get(to[6:], 0.0) + w * v
        return score

    def pick(self, line, top=3):
        """[(brick, how sure, 0 to 1)], strongest first: every symbol of the line pushes for or against each
        brick by the strength of its links, and the bricks are weighed against each other."""
        score = self.scores(line)
        if not score:
            return []
        high = max(score.values())
        weight = {b: math.exp(min(0.0, v - high) / 4) for b, v in score.items()}
        total = sum(weight.values())
        return sorted(((b, w / total) for b, w in weight.items()), key=lambda x: -x[1])[:top]

    def why(self, line, brick):
        """The symbols that pushed hardest for this brick."""
        out = []
        for s, w in symbols(line).items():
            v = self.brain.links.get(s, {}).get(f"brick:{brick}", 0.0)
            if v > 0:
                out.append((s, w * v))
        return sorted(out, key=lambda x: -x[1])[:6]

    def save(self):
        b = self.brain
        with open(b.path + ".new", "w", encoding="utf-8") as f:
            f.write("# brick's brain, the bricks: which symbols in a line point to which brick (action), and how\n"
                    "# strongly (below 0: a sign against it). Learned from its own mistakes on the sentence pool,\n"
                    "# each line labelled by building it (python3 -m trainer.picker learn). Delete it to start again.\n")
            for s in sorted(b.links):
                for brick, w in sorted(b.links[s].items()):
                    if abs(w) > 0.001:
                        f.write(f"link\t{s}\t{brick}\t{w:.4f}\n")
        os.replace(b.path + ".new", b.path)


def main(bricks):
    """A direction's main brick: 'after 2 seconds write hi' is a wait, then a write; the write is what it's
    about (the time is read on its own). Waits only: it's a wait."""
    rest = [b for b in bricks if b != "wait"]
    return rest if rest else bricks[:1]


def single(found):
    """The lines that became one brick (with or without a wait for a time said in it)."""
    return [(line, main(bricks)[0]) for line, bricks in found if len(main(bricks)) == 1]


def practice(count=4000, seed=12345):
    """More lines to learn from, the way people really type: the scrambler's messy directions (each labelled by
    building its tidy version), and chatter on its own ('lol', 'ok so', 'and'), which is no brick at all. Fresh
    seeds: never the bench's paragraphs."""
    import tempfile
    from trainer import scrambler
    rng_state = random.getstate()
    random.seed(seed)
    folder = tempfile.mkdtemp(prefix="picks-")
    words_file = os.path.join(folder, "mywords.txt")
    found, built = [], {}
    try:
        for _ in range(count):
            from trainer import details
            part = random.choice(details.makers())()
            said = scrambler.say(part)
            if random.random() < 0.25:
                said = f"{random.choice(scrambler.CHATTER_BEFORE)} {said}"
            if random.random() < 0.15:
                said = f"{said} {random.choice(scrambler.CHATTER_AFTER)}"
            if part.tidy not in built:
                bricks = bricks_of(part.tidy, words_file)
                built[part.tidy] = (main(bricks) if bricks else None)
            bricks = built[part.tidy]
            if bricks and len(bricks) == 1:
                found.append((said, bricks[0]))
        chatter = scrambler.CHATTER_BEFORE + scrambler.CHATTER_AFTER + ["and", "then", "lol", "ok", "yeah"]
        for _ in range(count // 8):
            found.append((" ".join(random.sample(chatter, random.randint(1, 2))), "none"))
    finally:
        random.setstate(rng_state)
        import shutil
        shutil.rmtree(folder, ignore_errors=True)
    return found


def learn():
    examples = [(l, b) for l, b in single(labels()) if not held_out(l)] + practice()
    p = Picker()
    print(f"Learning from {len(examples)} one-brick lines ({len(set(b for _, b in examples))} different bricks)")
    p.learn(examples)
    p.save()
    print(f"Saved {sum(len(t) for t in p.brain.links.values())} links -> {LINKS}")


# ---------------- the score ----------------

def checker_pick(line):
    """What the hand-written readers build a line as: its one brick, or None (refused, or several)."""
    _, ok, got = label_one(line)
    return main(got)[0] if ok and got and len(main(got)) == 1 else None


def score(limit=None, workers=8):
    from multiprocessing import Pool
    from trainer import scrambler
    os.environ.setdefault("BRICK_NEURAL", "off")
    p = Picker()
    test = [(l, b) for l, b in single(labels()) if held_out(l)]
    if limit:
        test = test[:limit]
    rng = random.Random(7)
    random.seed(7)
    messed = [(scrambler.mess(l), b) for l, b in test]
    tidy_right = sum(p.pick(l)[0][0] == b for l, b in test if p.pick(l))
    brain_messy = [bool(p.pick(m)) and p.pick(m)[0][0] == b for m, b in messed]
    with Pool(workers) as pool:
        checker = pool.map(checker_pick, [m for m, _ in messed], chunksize=8)
    checker_messy = [c == b for c, (_, b) in zip(checker, messed)]
    n = len(test)
    print(f"Lines the brain never saw: {n}")
    print(f"  tidy lines:   brain picks the right brick in {tidy_right} ({100 * tidy_right / n:.1f}%)")
    print(f"  messed lines: brain {sum(brain_messy)} ({100 * sum(brain_messy) / n:.1f}%), "
          f"hand-written readers {sum(checker_messy)} ({100 * sum(checker_messy) / n:.1f}%)")
    both = sum(a or c for a, c in zip(brain_messy, checker_messy))
    print(f"  messed lines either one gets: {both} ({100 * both / n:.1f}%)")
    wrong = [(m, b, p.pick(m)[:2]) for (m, b), ok in zip(messed, brain_messy) if not ok]
    with open(os.path.join(HERE, "data", "brick-picks-wrong.txt"), "w", encoding="utf-8") as f:
        for m, b, got in wrong:
            f.write(f"{m}\n    should be: {b}   brain: {got}\n")
    return tidy_right, sum(brain_messy), sum(checker_messy), n


def bench(workers=8):
    """The 40 hard paragraphs of tools/bench40.txt: the cutter cuts each into directions, the brain picks each
    direction's brick, and the bricks in order are checked against what the meant paragraph builds as. The hand-
    written readers are scored the same way on the messy paragraph. Waits are left out of both (a time said in a
    direction is read on its own)."""
    from multiprocessing import Pool
    from trainer import cutter, universe as u
    os.environ.setdefault("BRICK_NEURAL", "off")
    rows, messy = [], None
    with open(os.path.join(os.path.dirname(HERE), "tools", "bench40.txt"), encoding="utf-8") as f:
        for row in f:
            if row.startswith(("\u2717 ", "\u2713 ")):
                messy = row[2:].strip()
            elif row.strip().startswith("meant:") and messy:
                rows.append((messy, row.strip()[6:].strip()))
                messy = None
    p = Picker()
    with Pool(workers) as pool:
        truth = pool.map(label_one, [m for _, m in rows])
        old = pool.map(label_one, [m for m, _ in rows])
    brain_right = old_right = 0
    report = []
    for (messy, meant), (_, ok_t, want), (_, ok_o, got_o) in zip(rows, truth, old):
        want = [b for b in want if b != "wait"]
        frames = cutter.cut(u.tokens_of(messy, gaps=True)) or [u.tokens_of(messy)]
        pieces = [" ".join(t.text for t in fr) for fr in frames]
        picks = [p.pick(x)[0][0] if p.pick(x) else "none" for x in pieces]
        picks = [b for b in picks if b not in ("wait", "none")]
        olds = [b for b in got_o if b != "wait"] if ok_o else ["(refused)"]
        brain_right += picks == want
        old_right += olds == want
        report.append(f"{messy}\n    meant: {' '.join(want)}\n    brain: {' '.join(picks)}"
                      f"{'' if picks == want else '   WRONG'}\n    old:   {' '.join(olds)}"
                      f"{'' if olds == want else '   WRONG'}\n    pieces: {pieces}\n")
    with open(os.path.join(HERE, "data", "brick-picks-bench.txt"), "w", encoding="utf-8") as f:
        f.write("\n".join(report))
    print(f"Bench paragraphs: {len(rows)}")
    print(f"  every brick right, in order: brain {brain_right}, hand-written readers {old_right}")
    return brain_right, old_right, len(rows)


# ---------------- in use: the brain says which bricks a line should be ----------------

_picker = None


def off():
    """Off with BRICK_PICKER=off, or with all learned memory off (BRICK_MEMORY=off, as in the tests)."""
    return os.environ.get("BRICK_PICKER") == "off" or os.environ.get("BRICK_MEMORY") == "off" or \
        not os.path.isfile(LINKS)


def line_bricks(line, sure=0.9):
    """The bricks the brain says a line should be, in order (waits left out: a time is read on its own), or None
    when it isn't sure of every one (or it's switched off with BRICK_PICKER=off)."""
    global _picker
    if off() or not line.strip():
        return None
    from trainer import cutter, universe as u
    if _picker is None:
        _picker = Picker()
    frames = cutter.cut(u.tokens_of(line, gaps=True))
    pieces = [" ".join(t.text for t in fr) for fr in frames] if frames else [line]
    found = []
    for piece in pieces:
        picked = _picker.pick(piece)
        if not picked:
            continue                                     # nothing the brain knows: chatter
        if picked[0][1] < sure:
            return None
        if picked[0][0] not in ("wait", "none"):
            found.append(picked[0][0])
    return found


_verbs = None


def verbs():
    """brick -> the words lines of that brick most often start with (from the labels), best first."""
    global _verbs
    if _verbs is None:
        counts = {}
        if os.path.isfile(LABELS):
            for line, brick in single(labels()):
                ws = [w for w in tokens(line) if w not in SMALL and w != '"']
                if ws and ws[0].isalpha():
                    counts.setdefault(brick, {}).setdefault(ws[0], 0)
                    counts[brick][ws[0]] += 1
        _verbs = {b: [w for w, _ in sorted(c.items(), key=lambda x: -x[1])[:3]] for b, c in counts.items()}
    return _verbs


def spelled(words):
    """The words with slips spelled right by the brain (quoted words stay as typed)."""
    out, quoted = [], False
    for w in words:
        core = w.strip('.,!?;:').lower()
        if w.count('"') % 2:
            quoted = not quoted
        elif not quoted and core.isalpha() and len(core) > 3:
            fixed = respell([core])[0]
            if fixed != core:
                w = w.lower().replace(core, fixed)
        out.append(w)
    return out


def resay(line, want, words_file, whole=False):
    """The brain's own reading of a line it's sure of: each direction said again so it starts with a word for the
    brick the brain picked (slips spelled right, chatter before it dropped), checked by building it -> the
    reading, or None when no reading builds the brain's bricks."""
    from trainer import cutter, universe as u
    frames = None if whole else cutter.cut(u.tokens_of(line, gaps=True))
    pieces = [" ".join(t.text for t in fr) for fr in frames] if frames else [line]
    said = []
    for piece in pieces:
        picked = _picker.pick(piece) if _picker else []
        brick = picked[0][0] if picked else "none"
        if brick == "none":
            continue
        words = spelled(piece.split())
        tries = [" ".join(words)]
        for k in range(min(3, len(words))):
            for v in verbs().get(brick, [])[:2] if not brick.startswith("window") else ["write"]:
                tries.append(" ".join([v] + words[k + 1:]))
                tries.append(" ".join(words[:k] + [v] + words[k + 1:]))
        found = None
        for way in dict.fromkeys(tries):
            got = bricks_of(way, words_file)
            if got is not None and [b for b in got if b != "wait"] == ([] if brick == "wait" else [brick]):
                found = way
                break
        if found is None:
            return None
        said.append(found)
    if not said:
        return None
    joined = " then ".join(said)
    got = bricks_of(joined, words_file)
    return joined if same(got, want) else None


def whole_pick(line, sure=0.0):
    """The brick the brain picks for the whole line, read as one direction (None if it's less sure than sure)."""
    global _picker
    if off():
        return None
    if _picker is None:
        _picker = Picker()
    picked = _picker.pick(line)
    return picked[0][0] if picked and picked[0][1] >= sure else None


ALIKE = {"put": "write", "place": "write"}     # bricks that can each show words, or place things: which one a
                                               # line goes through isn't the brain's call


def same(a, b):
    """Two lists of bricks that are the same, counting put and place as write."""
    if a is None or b is None:
        return a is b
    return [ALIKE.get(x, x) for x in a] == [ALIKE.get(x, x) for x in b]


def bricks_of(text, words_file):
    """The bricks brick's own readers build a one-line text as (no re-reading), or None if they refuse it."""
    import compiler
    try:
        c = compiler.Compiler(words_file, ask_user=lambda question: "no")
        c.compile_text(text)
    except Exception:
        return None
    return [b for b in c.bricks_on.get(1, []) if b != "wait"]


def command(args):
    if args[:1] == ["bench"]:
        return bench()
    if args[:1] == ["label"]:
        return label(int(args[1]) if args[1:] else None)
    if args[:1] == ["learn"]:
        return learn()
    if args[:1] == ["score"]:
        return score(int(args[1]) if args[1:] else None)
    if args[:1] == ["try"] and args[1:]:
        p = Picker()
        line = " ".join(args[1:])
        for brick, share in p.pick(line):
            print(f"{brick:24} {100 * share:5.1f}%   because of: "
                  + ", ".join(s for s, _ in p.why(line, brick)[:4]))
        return
    print(__doc__)


if __name__ == "__main__":
    command(sys.argv[1:])
