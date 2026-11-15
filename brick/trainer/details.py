"""The brain decides the details: what each word of a direction is for, and the direction said in brick's words.

trainer/picker.py picks the brick (write, set, wait ...). This picks the rest: for every word, its role
  W  words to show        C  a colour        P  a place          N  how many times    T  a time
  F  the frame of words to show ('text that says', 'the words', 'saying')
  B  the background       K  the console     Q  the text itself ('make the writing red')
  V  the action word      X  chatter and small words
and then says the direction again from those roles, exactly ('after 2 seconds write hi in red at the top 3 times'),
which brick builds, to check.

It learns from the scrambler (trainer/scrambler.py): it writes each messy direction from known parts, so it knows
exactly which word is which. Fresh seeds, never the bench's paragraphs. A structured perceptron (like the cutter):
each word's role is weighed by the word itself, its neighbours, its shape, the ideas brick's universe knows for it,
and the role before it; the best roles for the whole direction win (Viterbi). It learns from its own mistakes.
The links are kept in trainer/data/brain-details.txt.

    python3 -m trainer.details learn [count]     learn the roles (about a minute)
    python3 -m trainer.details try "a line"      each word's role, and the direction said again
    python3 -m trainer.details score [count]     the brain's whole readings against fresh scrambled paragraphs and
                                                 the bench: same program as the meant one, or not
"""

import os
import random
import re
import sys
import tempfile

HERE = os.path.dirname(os.path.abspath(__file__))
sys.path.insert(0, os.path.dirname(HERE))

LINKS = os.path.join(HERE, "data", "brain-details.txt")
ROLES = ["W", "F", "C", "P", "N", "T", "B", "K", "Q", "V", "X"]
VERBS = {"write", "say", "show", "print", "display", "type", "put", "make", "turn", "set", "paint", "move", "clear",
         "wipe", "cls", "wait", "pause", "sleep", "hold"}
SIDES = {"left", "right", "top", "bottom", "middle", "centre", "center", "top-left", "top-right", "bottom-left",
         "bottom-right"}


# ---------------- examples with known roles, from the scrambler ----------------

def words_roles(text, role):
    """A piece of a direction -> [(word, role)]: inside a mixed piece ('make the bg red') each word gets its own."""
    out = []
    for w in text.split():
        low = w.lower().strip(".,!?")
        if role != "mixed":
            out.append((w, role))
        elif low in SIDES:
            out.append((w, "P"))
        elif low in ("bg", "background"):
            out.append((w, "B"))
        elif low == "console":
            out.append((w, "K"))
        elif low in ("text", "writing"):
            out.append((w, "Q"))
        elif re.fullmatch(r"\d+", low):
            out.append((w, "T"))
        elif low in VERBS:
            out.append((w, "V"))
        else:
            out.append((w, "C" if low in COLOURS else "X"))
    return out


COLOURS = set()


def mess(pairs):
    """The scrambler's damage (trainer/scrambler.mess), keeping each word's role."""
    from trainer import scrambler
    out = []
    for word, role in pairs:
        low = word.lower()
        if role not in ("W",):
            if low in ("the", "a", "of") and random.random() < 0.5:
                continue
            if low in scrambler.SHORT and random.random() < 0.4:
                word = scrambler.SHORT[low]
            if random.random() < 0.04:
                word = scrambler.typo(word)
            if random.random() < 0.03:
                out.append((word, role))
            word = word.upper() if random.random() < 0.06 else word
        out.append((word, role))
    return out


def example():
    """One messy direction from the scrambler with every word's role, its brick and its tidy words."""
    from trainer import scrambler
    COLOURS.update(scrambler.COLOURS)
    part = random.choice(scrambler.MAKERS)()
    chosen = {name: random.choice(ways) for name, ways in part.pieces.items()}
    names = list(chosen)
    random.shuffle(names)
    pairs = []
    for name in names:
        said = chosen[name]
        if not said:
            continue
        if name == "words":
            m = re.match(r'(text that says |the words |saying )?(.*)$', said)
            frame, shown = m.group(1) or "", m.group(2)
            pairs += [(w, "F") for w in frame.split()] + \
                ([(shown, "W")] if shown.startswith('"') else [(w, "W") for w in shown.split()])
        elif name == "verb":
            pairs += mess([(w, "V") for w in said.split()])
        elif name == "colour":
            pairs += mess([(w, "C" if w in scrambler.COLOURS else "X") for w in said.split()])
        elif name == "place":
            pairs += mess([(w, "P" if w in SIDES else "X") for w in said.split()])
        elif name == "count":
            pairs += mess([(w, "N" if re.search(r"\d", w) else "X") for w in said.split()])
        elif name == "later":
            pairs += mess([(w, "T" if re.search(r"\d", w) else "X") for w in said.split()])
        else:
            pairs += mess(words_roles(said, "mixed"))
    if random.random() < 0.25:
        pairs = [(w, "X") for w in random.choice(scrambler.CHATTER_BEFORE).split()] + pairs
    if random.random() < 0.15:
        pairs += [(w, "X") for w in random.choice(scrambler.CHATTER_AFTER).split()]
    return pairs, part.tidy


# ---------------- what the brain sees in each word ----------------

_world = None


def ideas(word):
    global _world
    from trainer import universe as u
    if _world is None:
        _world = u.universe()
    return sorted(_world.words.get(word, ()))[:4]


def shape(w):
    if re.fullmatch(r"\d+", w):
        return "num"
    if re.fullmatch(r"x\d+", w):
        return "xnum"
    if w.startswith('"'):
        return "quote"
    return "upper" if w.isupper() and len(w) > 1 else "word"


def seen(words):
    """Each word, lower case and spelled right by the brain where it's a clear slip."""
    from trainer import picker
    low = [w.lower().strip(".,!?;:") for w in words]
    return [picker.respell([w])[0] if w.isalpha() and len(w) > 3 else w for w in low]


def features(words):
    """[feature list] for each word of a direction."""
    ws = seen(words)
    out = []
    for i, w in enumerate(ws):
        prev = ws[i - 1] if i else "(start)"
        after = ws[i + 1] if i + 1 < len(ws) else "(end)"
        prev2 = ws[i - 2] if i > 1 else "(start)"
        after2 = ws[i + 2] if i + 2 < len(ws) else "(end)"
        f = [f"w:{w}", f"p:{prev}", f"n:{after}", f"pw:{prev}|{w}", f"wn:{w}|{after}", f"p2:{prev2}", f"n2:{after2}",
             f"s:{shape(words[i])}", f"ps:{shape(words[i - 1]) if i else '-'}", f"first:{i == 0}"]
        f += [f"i:{x}" for x in ideas(w)] + [f"pi:{x}" for x in (ideas(prev) if i else [])] + \
            [f"ni:{x}" for x in (ideas(after) if i + 1 < len(ws) else [])]
        if len(w) > 3:
            f += [f"end:{w[-3:]}", f"start:{w[:3]}"]
        out.append(f)
    return out


# ---------------- the brain learns and reads ----------------

class Details:
    def __init__(self, path=LINKS):
        self.path = path
        self.w = {}
        if os.path.isfile(path):
            with open(path, encoding="utf-8") as f:
                for row in f:
                    parts = row.rstrip("\n").split("\t")
                    if parts[0] == "link" and len(parts) == 4:
                        self.w.setdefault(parts[1], {})[parts[2]] = float(parts[3])

    def emit(self, fs, w=None):
        w = self.w if w is None else w
        score = {}
        for f in fs:
            for role, v in w.get(f, {}).items():
                score[role] = score.get(role, 0.0) + v
        return score

    def decode(self, feats, w=None):
        """The best roles for the whole direction (Viterbi over each word's role and the role before it)."""
        w = self.w if w is None else w
        best = [{}]
        for i, fs in enumerate(feats):
            e = self.emit(fs, w)
            col = {}
            for r in ROLES:
                if i == 0:
                    col[r] = (e.get(r, 0.0) + w.get("from:(start)", {}).get(r, 0.0), None)
                else:
                    col[r] = max(((best[i][q][0] + w.get(f"from:{q}", {}).get(r, 0.0) + e.get(r, 0.0), q)
                                  for q in ROLES), key=lambda x: x[0])
            best.append(col)
        r = max(ROLES, key=lambda x: best[-1][x][0])
        path = [r]
        for i in range(len(feats), 1, -1):
            r = best[i][r][1]
            path.append(r)
        return path[::-1]

    def learn(self, examples, rounds=8):
        w, total, step = {}, {}, 1

        def push(f, r, v):
            w.setdefault(f, {})[r] = w.get(f, {}).get(r, 0.0) + v
            total.setdefault(f, {})[r] = total.get(f, {}).get(r, 0.0) + v * step

        prepared = [(features([x for x, _ in pairs]), [r for _, r in pairs]) for pairs in examples]
        for k in range(rounds):
            wrong = 0
            random.Random(k).shuffle(prepared)
            for feats, gold in prepared:
                got = self.decode(feats, w)
                if got != gold:
                    wrong += 1
                    for i, (g, p) in enumerate(zip(gold, got)):
                        if g != p:
                            for f in feats[i]:
                                push(f, g, 1.0)
                                push(f, p, -1.0)
                        gp = gold[i - 1] if i else "(start)"
                        pp = got[i - 1] if i else "(start)"
                        if (g, gp) != (p, pp):
                            push(f"from:{gp}", g, 1.0)
                            push(f"from:{pp}", p, -1.0)
                step += 1
            print(f"  practice round {k + 1}: {wrong} of {len(prepared)} directions with a word read wrong", flush=True)
        self.w = {f: {r: v - total[f].get(r, 0.0) / step for r, v in rs.items()} for f, rs in w.items()}

    def save(self):
        with open(self.path + ".new", "w", encoding="utf-8") as f:
            f.write("# brick's brain, the details: which things about a word (itself, its neighbours, its shape, its ideas)\n"
                    "# point to which role it plays in a direction (W words to show, C colour, P place, N count,\n"
                    "# T time ...). Learned from its own mistakes (python3 -m trainer.details learn).\n")
            for feat in sorted(self.w):
                for role, v in sorted(self.w[feat].items()):
                    if abs(v) > 0.001:
                        f.write(f"link\t{feat}\t{role}\t{v:.4f}\n")
        os.replace(self.path + ".new", self.path)

    def roles(self, words):
        return self.decode(features(words)) if words else []


# ---------------- saying it again ----------------

def number(words):
    for w in words:
        m = re.search(r"\d+", w)
        if m:
            return int(m.group())
        if w.lower() in ("half", "a", "one"):
            return {"half": None, "a": 1, "one": 1}[w.lower()]
    return None


def colour_of(words):
    from trainer import picker
    from actions import colour_words
    known = set(colour_words())
    for w in words:
        low = w.lower().strip(".,!?;:")
        if low in known:
            return low
        near = picker.respell([low])[0]
        if near in known:
            return near
        import difflib
        close = difflib.get_close_matches(low, known, n=1, cutoff=0.8)
        if close:
            return close[0]
    return None


def place_of(words):
    import difflib
    for w in words:
        low = w.lower().strip(".,!?;:")
        if low in SIDES:
            return low
        close = difflib.get_close_matches(low, SIDES, n=1, cutoff=0.8)
        if close:
            return close[0]
    return None


def say_again(brick, words, roles):
    """The direction in brick's words, from its brick and each word's role -> text, or None if a part it needs
    is missing."""
    part = {r: [w for w, x in zip(words, roles) if x == r] for r in ROLES}
    later = number(part["T"]) if part["T"] else None
    if brick in ("put", "place", "move", "write") and part["K"] and not part["W"]:
        place = place_of(part["P"])
        return f"put the console on the {place} side of the screen" if place else None
    if brick in ("write", "put", "place") and part["W"]:
        shown = " ".join(w.strip('"') for w in part["W"]).strip(".,!?;:")
        places = [w for w in part["P"] if w.lower().strip(".,!?;:") != "right"] or part["P"] or \
            [w for w, x in zip(words, roles) if x == "X" and w.lower().strip(".,!?;:") == "right"]
        # 'right' is a place or just chatter ('right, ...'): a place only when no other place is said
        colour, place, count = colour_of(part["C"]), place_of(places), number(part["N"]) if part["N"] else None
        text = f"write {shown}" + (f" in {colour}" if colour else "") + (f" at the {place}" if place else "") + \
            (f" {count} times" if count else "")
        return (f"after {later} seconds " if later else "") + text
    if brick in ("put", "place", "move") and part["K"]:
        place = place_of(part["P"])
        return f"put the console on the {place} side of the screen" if place else None
    if brick == "set":
        colour = colour_of(part["C"])
        if not colour:
            return None
        return f"set the background colour to {colour}" if part["B"] else f"set the colour to {colour}"
    if brick == "clear":
        return "clear the screen"
    if brick == "wait" and later:
        return f"wait {later} {'second' if later == 1 else 'seconds'}"
    return None


FILLER = {"secs", "sec", "seconds", "second", "s", "times", "time", "screen", "side", "color", "colour", "of", "the",
          "a", "with", "at", "to", "on", "in", "rn", "thx", "thanks", "pls", "please", "lol", "yeah", "ok", "so", "um",
          "hey", "just", "now", "right", "and", "then", "can", "you", "u", "real", "quick", "also", "&", "x", "for"}


def filler(word):
    """Chatter and small words a direction can leave out without losing anything ('ok so', 'side of screen')."""
    from trainer import picker, scrambler
    low = word.lower().strip(".,!?;:-")
    if not low or not re.search(r"[a-z0-9]", low):
        return True
    chatter = {w for c in scrambler.CHATTER_BEFORE + scrambler.CHATTER_AFTER for w in c.split()}
    return low in FILLER or low in chatter or low in picker.SMALL or picker.respell([low])[0] in FILLER


TIME_MARKS = {"after", "sec", "secs", "second", "seconds", "s", "minute", "minutes", "min", "mins"}
JOINERS = {"but", "and", "or", "so", "then", "that", "which"}
_actions = None


def consistent(words, roles):
    """Do the roles hang together the way a direction does?
      the words to show are one stretch ('write she was saying hi' isn't 'she' and 'hi');
      one action word (said twice is fine: 'type type'), not two different ones;
      a number is a time or a count only with its mark ('after 2 secs', '3 times', 'x3': not 'the x 20');
      the words to show don't start with an action word brick knows ('state hello in red'), nor end on 'but'."""
    global _actions
    from trainer import picker
    import dictionary
    if _actions is None:
        from actions import STARTERS
        _actions = set(STARTERS) | set(dictionary.first_words("action", "loop"))
    low = [w.lower().strip(".,!?;:") for w in words]
    shown = [i for i, r in enumerate(roles) if r == "W"]
    if shown and shown != list(range(shown[0], shown[-1] + 1)):
        return False
    if shown and (low[shown[0]] in _actions and not words[shown[0]].startswith('"') or low[shown[-1]] in JOINERS):
        return False
    verbs = {picker.respell([low[i]])[0] for i, r in enumerate(roles) if r == "V"}
    if len(verbs) > 1:
        return False
    marks = set(low)
    for i, r in enumerate(roles):
        if r == "T" and re.search(r"\d", low[i]) and not (marks & TIME_MARKS or re.fullmatch(r"\d+s", low[i])):
            return False
        if r == "N" and re.fullmatch(r"\d+", low[i]) and not marks & {"times", "time"}:
            return False
    return True


def fits(word, role):
    """Does the word really fit the role the brain read for it (a place is a place, a count has a number ...)?"""
    from trainer import picker
    low = word.lower().strip(".,!?;:")
    near = picker.respell([low])[0] if low.isalpha() and len(low) > 3 else low
    if role == "C":
        return colour_of([word]) is not None
    if role == "P":
        return place_of([word]) is not None
    if role in ("N", "T"):
        return bool(re.search(r"\d", low)) or low in ("half", "a", "one") or filler(word)
    if role == "B":
        return {low, near} & {"bg", "background"} != set()
    if role == "K":
        return "console" in (low, near)
    if role == "Q":
        return {low, near} & {"text", "writing"} != set()
    if role == "F":
        return {low, near} & {"text", "that", "says", "the", "words", "saying"} != set()
    if role == "V":
        return bool(picker._picker and picker._picker.pick(low)) and not filler(word) or near in VERBS
    return True


_details = None


def read(line):
    """The brain's whole reading of a line: cut into directions (the cutter), each one's brick (the picker), each
    word's role, said again in brick's words -> text, or None where it can't say one direction."""
    global _details
    from trainer import cutter, picker, universe as u
    if _details is None:
        _details = Details()
    if picker._picker is None:
        picker._picker = picker.Picker()
    frames = cutter.cut(u.tokens_of(line, gaps=True))
    pieces = [" ".join(t.text for t in fr) for fr in frames] if frames else [line]
    said = []
    for piece in pieces:
        picked = picker._picker.pick(piece)
        words = re.findall(r'"[^"]*"|\S+', piece)
        if not picked or picked[0][0] == "none":
            if all(filler(w) for w in words):
                continue                         # chatter
            return None
        roles = _details.roles(words)
        if all(r == "X" for r in roles) and all(filler(w) for w in words):
            continue                             # only chatter
        if any(r == "X" and not filler(w) for w, r in zip(words, roles)) or \
                not all(fits(w, r) for w, r in zip(words, roles)):
            return None                          # a word it has no role for, or one that doesn't fit its role
        if not consistent(words, roles):
            return None                          # the roles don't hang together: it isn't sure
        action = " ".join(w for w, r in zip(words, roles) if r == "V")
        again = None
        for k, (brick, share) in enumerate(picked):   # the brick it's sure of, or the one the action word itself
            if brick == "none":                       # is (the roles didn't fit the first: 'pls hold on 4 secs')
                break
            agrees = bool(action) and bool(picker._picker.pick(action)) and picker._picker.pick(action)[0][0] == brick
            if not (k == 0 and share >= 0.5 or agrees):     # every word already fits its role
                continue
            again = say_again(brick, words, roles)
            if again:
                break
        if again is None:
            return None
        said.append(again)
    return " then ".join(said) if said else None


# ---------------- learning and the score ----------------

def learn(count=8000, seed=4242):
    random.seed(seed)
    examples = [example()[0] for _ in range(count)]
    d = Details(path=LINKS)
    d.w = {}
    print(f"Learning the roles of words from {count} scrambled directions")
    d.learn(examples)
    d.save()
    print(f"Saved {sum(len(v) for v in d.w.values())} links -> {LINKS}")


def build(text, folder, alone):
    from compiler import Compiler
    words = os.path.join(folder, "mywords.txt")
    try:
        c = Compiler(words, ask_user=lambda question: "no")
        return c.compile_text(text) if alone else c.compile(text)
    except Exception:
        return None
    finally:
        if os.path.exists(words):
            os.remove(words)


def score_one(job):
    messy, meant = job
    folder = tempfile.mkdtemp(prefix="details-")
    try:
        want = build(meant, folder, True)
        if want is None:
            return None
        reading = read(messy)
        brain = reading is not None and build(reading, folder, True) == want
        brick = build(messy, folder, False) == want
        return brain, brick, messy, meant, reading
    finally:
        import shutil
        shutil.rmtree(folder, ignore_errors=True)


def score(count=300, seed=777, workers=10):
    from multiprocessing import Pool
    from trainer import scrambler
    os.environ.setdefault("BRICK_NEURAL", "off")
    random.seed(seed)
    fresh = []
    for _ in range(count):
        mess_text, _, tidy = scrambler.paragraph()
        fresh.append((mess_text, " then ".join(tidy)))
    bench, messy = [], None
    with open(os.path.join(os.path.dirname(HERE), "tools", "bench40.txt"), encoding="utf-8") as f:
        for row in f:
            if row.startswith(("✗ ", "✓ ")):
                messy = row[2:].strip()
            elif row.strip().startswith("meant:") and messy:
                bench.append((messy, row.strip()[6:].strip()))
                messy = None
    report = []
    with Pool(workers) as pool:
        for name, jobs in (("fresh scrambled paragraphs", fresh), ("bench paragraphs", bench)):
            done = [r for r in pool.map(score_one, jobs, chunksize=4) if r]
            brain = sum(r[0] for r in done)
            brick = sum(r[1] for r in done)
            either = sum(r[0] or r[1] for r in done)
            print(f"{name}: {len(done)}   brain's own reading: {brain} exact ({100 * brain / len(done):.1f}%)   "
                  f"brick: {brick} ({100 * brick / len(done):.1f}%)   either: {either}")
            report += [f"[{name}] brain {'RIGHT' if r[0] else 'WRONG'}  brick {'RIGHT' if r[1] else 'WRONG'}\n"
                       f"    messy: {r[2]}\n    meant: {r[3]}\n    brain: {r[4]}\n" for r in done if not (r[0] and r[1])]
    with open(os.path.join(HERE, "data", "brain-details-wrong.txt"), "w", encoding="utf-8") as f:
        f.write("\n".join(report))


def command(args):
    if args[:1] == ["learn"]:
        return learn(int(args[1]) if args[1:] else 8000)
    if args[:1] == ["score"]:
        return score(int(args[1]) if args[1:] else 300)
    if args[:1] == ["try"] and args[1:]:
        line = " ".join(args[1:])
        d = Details()
        words = re.findall(r'"[^"]*"|\S+', line)
        print("  ".join(f"{w}/{r}" for w, r in zip(words, d.roles(words))))
        print("said again:", read(line))
        return
    print(__doc__)


if __name__ == "__main__":
    command(sys.argv[1:])
