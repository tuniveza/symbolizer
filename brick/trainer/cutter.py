"""The brain's cutter: where one direction ends and the next begins in a messy paragraph.

Between separators (a comma, 'then', a full stop ...) a messy paragraph can still hold several directions run
together: 'ok so to right in the color navy after 9 sec lets go can you in yellow to top ...'. Rules that cut left
to right get the ends wrong: a colour or a place goes to the wrong direction. The cutter learns where directions
really end instead, from the scrambler's paragraphs (trainer/scrambler.py), which know where each direction they
wrote begins. Each piece of a line (an action, the words to show, a colour, a place, a time, a count, chatter ...)
is one step; the cutter weighs cutting or not before each piece, with what the direction holds so far, and keeps
the best whole cut of the stretch (a beam search, trained as a structured perceptron).

It never sees the bench's paragraphs (tools/bench40.txt): it learns from fresh ones, made with their own seeds.

    python3 -m trainer.cutter learn [paragraphs]     learn again (about a minute for 6000)
    python3 -m trainer.cutter try "messy text"       show how it cuts a line
"""
import json
import os
import random
import sys

HERE = os.path.dirname(os.path.abspath(__file__))
SAVED = os.path.join(HERE, "data", "cutter.json")
SEEDS = range(1000, 1000 + 64)          # its own seeds: never the bench's paragraphs
BEAM = 12
CONTROL = {"if", "when", "while", "until", "repeat", "each", "unless", "otherwise", "do"}

_weights = None


# ---------------- a line as pieces ----------------

def label(tokens, i):
    """(the piece's label for the cutter, the slot it fills, where it ends)."""
    from trainer import universe as u
    slot, end = u.piece(tokens, i)
    t = tokens[i]
    if slot == "action":
        kind = next((k for k in ("wait", "clear", "show", "placing", "make") if k in t.ideas), "other")
        return "V" + kind, "action", end
    if slot == "words":
        return ("Wq" if t.quoted else "Ws" if "saying" in t.ideas else "W"), "words", end
    if slot == "skip":
        for idea, name in (("chatter", "chat"), ("filler", "chat"), ("toward", "to"), ("it", "it"),
                           ("by means of", "with"), ("becoming", "into")):
            if idea in t.ideas:
                return name, None, end
        return "g:" + t.word, None, end
    if slot == "thing":
        return ("window" if "window" in t.ideas else "bg"), "thing", end
    if slot == "text":
        return ("screen" if "screen" in t.ideas else "text"), "text", end
    if slot == "time":
        return ("after" if "later" in t.ideas else "time"), "time", end
    return slot, slot, end


def pieces(tokens):
    """[(label, slot, the piece's tokens)] for one stretch between separators."""
    out, i = [], 0
    while i < len(tokens):
        name, slot, end = label(tokens, i)
        end = max(end, i + 1)
        out.append((name, slot, tokens[i:end]))
        i = end
    return out


def is_end(token):
    from trainer import universe as u
    return token.word in u.ENDS or "end" in token.ideas


# ---------------- the features ----------------

def gap_features(ps, g, filled, has_action):
    """What the cutter looks at before piece g, given the slots the direction holds so far."""
    L = [p[0] for p in ps]
    at = lambda k: L[k] if 0 <= k < len(L) else "<>"
    slot = ps[g][1]
    rest_action = any(p[1] == "action" for p in ps[g:])
    return ["b", "p1n1:" + at(g - 1) + "|" + at(g), "p2p1n1:" + at(g - 2) + "|" + at(g - 1) + "|" + at(g),
            "p1n1n2:" + at(g - 1) + "|" + at(g) + "|" + at(g + 1), "n1:" + at(g),
            "dup:" + str(slot in filled and slot is not None) + "|" + str(slot),
            "state:" + ",".join(sorted(filled)) + "|" + at(g),
            "act:" + str(has_action) + "|" + str(rest_action) + "|" + at(g),
            "act2:" + str(has_action) + "|" + at(g - 1) + "|" + at(g)]


def close_features(seg):
    """What the cutter looks at when a direction closes: is it a whole direction?"""
    slots = sorted({p[1] for p in seg if p[1]})
    words = sum(1 for p in seg if p[1] == "words")
    first = seg[0][0] if seg else "<>"
    last = seg[-1][0] if seg else "<>"
    return ["close:" + ",".join(slots), "close-w:" + str(min(words, 3)) + "|" + str("action" in slots),
            "close-ends:" + first + "|" + last]


def score(w, feats, y):
    return sum(w.get(y + f, 0.0) for f in feats)


# ---------------- cutting ----------------

def decode(w, ps):
    """The best cut of one stretch of pieces -> the indices where a new direction starts."""
    beams = [(0.0, (), [ps[0]], frozenset({ps[0][1]} - {None}), ps[0][1] == "action")]
    for g in range(1, len(ps)):
        nxt = []
        for total, cuts, seg, filled, has_action in beams:
            f = gap_features(ps, g, filled, has_action)
            slot = ps[g][1]
            # join: piece g goes with the direction so far
            nxt.append((total + score(w, f, "J:"), cuts, seg + [ps[g]], filled | ({slot} - {None}),
                        has_action or slot == "action"))
            # cut: the direction so far is whole, and piece g starts the next one
            nxt.append((total + score(w, f, "C:") + score(w, close_features(seg), "E:"), cuts + (g,), [ps[g]],
                        frozenset({slot} - {None}), slot == "action"))
        nxt.sort(key=lambda b: -b[0])
        beams = nxt[:BEAM]
    best = max(beams, key=lambda b: b[0] + score(w, close_features(b[2]), "E:"))
    return list(best[1])


def all_features(ps, cuts):
    """Every feature of one cut of a stretch, counted (for learning)."""
    from collections import Counter
    out = Counter()
    seg, filled, has_action = [ps[0]], {ps[0][1]} - {None}, ps[0][1] == "action"
    for g in range(1, len(ps)):
        f = gap_features(ps, g, filled, has_action)
        slot = ps[g][1]
        if g in cuts:
            out.update("C:" + x for x in f)
            out.update("E:" + x for x in close_features(seg))
            seg, filled, has_action = [ps[g]], {slot} - {None}, slot == "action"
        else:
            out.update("J:" + x for x in f)
            seg.append(ps[g])
            filled |= {slot} - {None}
            has_action = has_action or slot == "action"
    out.update("E:" + x for x in close_features(seg))
    return out


def weights():
    global _weights
    if _weights is None:
        try:
            with open(SAVED, encoding="utf-8") as f:
                _weights = json.load(f)["weights"]
        except (OSError, ValueError, KeyError):
            _weights = {}
    return _weights


def stretches(tokens):
    """The tokens cut at the separators -> [stretch of tokens]."""
    out, current = [], []
    for k, t in enumerate(tokens):
        after = tokens[k + 1] if k + 1 < len(tokens) else None
        if is_end(t) or t.word == "and" and after and after.ideas & {"action", "other action"}:
            out.append(current)
            current = []
        else:
            current.append(t)
    return [s for s in out + [current] if s]


def cut(tokens):
    """The tokens cut into directions where the brain learned they end -> [[token]], or None when it has
    nothing learned yet or the line holds ifs, loops and the like (brick's own rules read those)."""
    w = weights()
    if not w or any(t.word in CONTROL for t in tokens) or os.environ.get("BRICK_CUTTER") == "off":
        return None
    from trainer import universe as u
    frames = []
    for stretch in stretches(tokens):
        ps = pieces(stretch)
        if sum(p[1] == "words" for p in ps) < 2 and sum(p[1] == "action" for p in ps) < 2:
            frames += u.by_slots(stretch)   # one direction's worth ('write hi on a blue background'): the rules read it
            continue
        starts = [0] + decode(w, ps) + [len(ps)]
        for a, b in zip(starts, starts[1:]):
            frames.append([t for p in ps[a:b] for t in p[2]])
    return frames


# ---------------- learning, from the scrambler's paragraphs ----------------

def examples(count, seed):
    """[(pieces of a stretch, the gold cuts)] from fresh scrambled paragraphs."""
    from trainer import scrambler
    from trainer import universe as u
    random.seed(seed)
    out = []
    for _ in range(count):
        parts = [random.choice(scrambler.MAKERS)() for _ in range(random.randint(2, 4))]
        flat = []                                   # (piece, starts a direction?)
        for k, part in enumerate(parts):
            said = scrambler.say(part)
            if random.random() < 0.25:
                said = f"{random.choice(scrambler.CHATTER_BEFORE)} {said}"
            if random.random() < 0.15:
                said = f"{said} {random.choice(scrambler.CHATTER_AFTER)}"
            join = random.choice(scrambler.JOINS) if k else ""
            lower = random.random() < 0.3
            if join.strip():
                for t in u.tokens_of(join.strip() if join.strip() not in (".", "...", "-") else ";"):
                    flat.append((("sep",), False, t))
            first = True
            for p in pieces(u.tokens_of(said.lower() if lower else said)):
                flat.append((p, first, None))
                first = False
        stretch, cuts = [], []
        for item in flat + [(("sep",), False, None)]:
            p, starts, tok = item
            if p == ("sep",) or (tok is not None and is_end(tok)):
                if stretch:
                    out.append((stretch, cuts))
                stretch, cuts = [], []
                continue
            if p[2] and is_end(p[2][0]):
                if stretch:
                    out.append((stretch, cuts))
                stretch, cuts = [], []
                continue
            if starts and stretch:
                cuts.append(len(stretch))
            stretch.append(p)
    return out


def learn(count=6000, rounds=6, say=print):
    """Learn the cutter's weights from fresh scrambled paragraphs (an averaged structured perceptron)."""
    global _weights
    data = []
    per = max(1, count // len(SEEDS))
    for seed in SEEDS:
        data += examples(per, seed)
    say(f"cutter: {len(data)} stretches from {per * len(SEEDS)} paragraphs")
    w, total, stamp, t = {}, {}, {}, 1
    for r in range(rounds):
        random.seed(r)
        random.shuffle(data)
        wrong = 0
        for ps, gold in data:
            guess = decode(w, ps)
            if guess != gold:
                wrong += 1
                delta = all_features(ps, set(gold))
                delta.subtract(all_features(ps, set(guess)))
                for f, d in delta.items():
                    if d:
                        total[f] = total.get(f, 0.0) + (t - stamp.get(f, 0)) * w.get(f, 0.0)
                        stamp[f] = t
                        w[f] = w.get(f, 0.0) + d
            t += 1
        say(f"  round {r + 1}: {len(data) - wrong}/{len(data)} stretches cut right")
    averaged = {}
    for f, v in w.items():
        a = (total.get(f, 0.0) + (t - stamp.get(f, 0)) * v) / t
        if abs(a) > 1e-6:
            averaged[f] = round(a, 5)
    os.makedirs(os.path.dirname(SAVED), exist_ok=True)
    with open(SAVED + ".tmp", "w", encoding="utf-8") as f:
        json.dump({"weights": averaged, "paragraphs": per * len(SEEDS), "rounds": rounds}, f)
    os.replace(SAVED + ".tmp", SAVED)
    _weights = averaged
    held = examples(400, 7777)
    right = sum(decode(averaged, ps) == gold for ps, gold in held)
    say(f"cutter: learned {len(averaged)} weights; on 400 new paragraphs' stretches: {right}/{len(held)} cut right")
    return right / max(1, len(held))


if __name__ == "__main__":
    if len(sys.argv) > 1 and sys.argv[1] == "learn":
        learn(int(sys.argv[2]) if len(sys.argv) > 2 else 6000)
    elif len(sys.argv) > 2 and sys.argv[1] == "try":
        from trainer import universe as u
        for d in cut(u.tokens_of(sys.argv[2])) or []:
            print(" ".join(t.text for t in d))
    else:
        print(__doc__)
