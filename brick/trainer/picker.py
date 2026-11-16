"""The digital brain's Ni, first half: which brick (action) each direction is, read from the line's own words.

It learns strictly from its own imagination: the archetypes voice directions in their own personalities
(trainer/voices.py, trainer/data/voiced-practice.jsonl), each with its meaning kept, and brick's builder only says
which brick each meaning builds as. The brain links what it sees in a direction (its words, where each stands, the
words beside it, pairs of words, each word's shape) to those bricks, learning from its own mistakes. The links are
kept in trainer/data/brain-bricks.txt. trainer/mind.py is where brick asks the brain.

    python3 -m trainer.picker learn             learn from the archetypes' practice
    python3 -m trainer.picker try "a line"      which brick the brain picks, and why
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

LINKS = os.path.join(HERE, "data", "brain-bricks.txt")
NEWEST = 40000             # paragraphs of practice it learns from at most (the newest)


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


def voiced(holdout=0.1):
    """The digital brain's own practice: each direction an archetype voiced, and the brick its meaning builds as
    (brick's builder only says which brick that is). Chatter on its own is no brick. -> (learn from, held out)."""
    import json
    from trainer import voices
    if not os.path.isfile(voices.PRACTICE):
        return [], []
    folder = tempfile.mkdtemp(prefix="picks-")
    words_file, built = os.path.join(folder, "mywords.txt"), {}
    learn_from, held = [], []
    try:
        with open(voices.PRACTICE, encoding="utf-8") as f:
            rows = [json.loads(r) for r in f if r.strip()][-NEWEST:]     # the newest practice
        for k, d in enumerate(rows):
            for part, tidy in zip(d["parts"], d["tidy"]):
                if tidy not in built:
                    bricks = bricks_of(tidy, words_file)
                    built[tidy] = main(bricks) if bricks else None
                bricks = built[tidy]
                if bricks and len(bricks) == 1:
                    (held if (k * 2654435761) % 1000 < holdout * 1000 else learn_from).append((part, bricks[0]))
    finally:
        import shutil
        shutil.rmtree(folder, ignore_errors=True)
    from trainer import scrambler
    rng = random.Random(5)
    chatter = scrambler.CHATTER_BEFORE + scrambler.CHATTER_AFTER + ["and", "then", "lol", "ok", "yeah"]
    learn_from += [(" ".join(rng.sample(chatter, rng.randint(1, 2))), "none") for _ in range(len(learn_from) // 10)]
    return learn_from, held


def learn():
    """The brain learns which brick a direction is, strictly from its archetypes' voiced practice."""
    examples, held = voiced()
    p = Picker()
    print(f"Learning from {len(examples)} directions the archetypes voiced ({len(set(b for _, b in examples))} bricks)")
    p.learn(examples)
    p.save()
    print(f"Saved {sum(len(t) for t in p.brain.links.values())} links -> {LINKS}")
    if held:
        right = sum(bool(p.pick(l)) and same([p.pick(l)[0][0]], [b]) for l, b in held)
        print(f"On {len(held)} voiced directions it never saw: {right} right ({100 * right / len(held):.1f}%)")


# ---------------- the score ----------------

def off():
    """Off with BRICK_PICKER=off, or with all learned memory off (BRICK_MEMORY=off, as in the tests)."""
    return os.environ.get("BRICK_PICKER") == "off" or os.environ.get("BRICK_MEMORY") == "off" or \
        not os.path.isfile(LINKS)


ALIKE = {"put": "write", "place": "write"}     # bricks that can each show words, or place things: which one a
                                               # line goes through isn't the brain's call


def same(a, b):
    """Two lists of bricks that are the same, counting put and place as write."""
    if a is None or b is None:
        return a is b
    return [ALIKE.get(x, x) for x in a] == [ALIKE.get(x, x) for x in b]


def all_bricks_of(text, words_file):
    """Every brick brick's own language builds a one-line text as, waits too (or None if it refuses it)."""
    import compiler
    try:
        c = compiler.Compiler(words_file, ask_user=lambda question: "no")
        c.compile_text(text)
    except Exception:
        return None
    return c.bricks_on.get(1, [])


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
    if args[:1] == ["learn"]:
        return learn()
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
