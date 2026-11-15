"""The brain decides the details: what each word of a direction is for, and the direction said in brick's words.

trainer/picker.py picks the brick (write, set, wait ...). This picks the rest: for every word, its role
  W  words to show        C  a colour        P  a place          N  how many times    T  a time
  F  the frame of words to show ('text that says', 'the words', 'saying')
  B  the background       K  the console     Q  the text itself ('make the writing red')
  G  brick's own window   M  a name ('called My Game')                S  a size ('800 by 600')
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
ROLES = ["W", "F", "C", "P", "N", "T", "B", "K", "Q", "G", "M", "S", "V", "X"]
WINDOW_WORDS = {"window", "windows", "win", "windw"}
VERBS = {"write", "say", "show", "print", "display", "type", "put", "make", "turn", "set", "paint", "move", "clear",
         "wipe", "cls", "wait", "pause", "sleep", "hold", "add", "open", "create", "start", "pop", "resize", "size",
         "name", "call", "title", "rename", "label", "colour", "color", "close", "shut", "up"}
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


# ---------------- brick's own window: practice directions with known parts, like the scrambler's ----------------

NAMES = ["Game", "My Game", "Painter", "Hello", "Notes", "Clock", "Snake", "Quiz", "Paint", "Space Race"]
WINDOW_SIDES = ["left", "right", "top", "bottom", "middle", "top-left", "bottom-right"]


def window_write():
    from trainer import scrambler
    words, colour = random.choice(scrambler.WORDS), random.choice(scrambler.COLOURS + [None, None])
    side = random.choice(WINDOW_SIDES + [None, None])
    later, count = random.choice([None, None, None, random.randint(2, 9)]), random.choice([None, None, None,
                                                                                         random.randint(2, 5)])
    tidy = (f"after {later} seconds " if later else "") + f"write {words} in the window" + \
        (f" at the {side}" if side else "") + (f" in {colour}" if colour else "") + (f" {count} times" if count else "")
    pieces = {"verb": ["write", "say", "show", "print", "display", "type", "put", "add", ""],
              "words": [words, f"text that says {words}", f"the words {words}", f"saying {words}", f'"{words}"'],
              "window": ["in the window", "on the window", "in window", "on window", "to the window", "in a window",
                         "on a window", "at window", "inside the window", "in my window", "on that window"]}
    if colour:
        pieces["colour"] = [f"in {colour}", colour, f"with {colour} color", f"in the color {colour}", f"in {colour} colour"]
    if side:
        pieces["place"] = [f"at the {side}", side, f"at the {side} of the window", f"on the {side}", f"to the {side}",
                           f"at {side} side", f"{side} side"]
    if count:
        pieces["count"] = [f"{count} times", f"x{count}"]
    if later:
        pieces["later"] = [f"after {later} seconds", f"after {later} secs", f"after {later} sec"]
    return scrambler.Part(tidy, pieces, keep=["words"])


def window_open():
    from trainer import scrambler
    name, colour = random.choice(NAMES + [None]), random.choice(scrambler.COLOURS + [None, None])
    size = random.choice([None, None, (random.choice([400, 640, 800, 1024]), random.choice([300, 480, 600, 768]))])
    tidy = "open a window" + (f" called {name}" if name else "") + (f" {size[0]} by {size[1]}" if size else "") + \
        (f" in {colour}" if colour else "")
    pieces = {"verb": ["open", "open up", "make", "create", "start", "pop up"],
              "window": ["a window", "window", "a new window", "new window", "the window"]}
    if name:
        pieces["name"] = [f"called {name}", f"named {name}", f"titled {name}", f"with title {name}"]
    if size:
        pieces["size"] = [f"{size[0]} by {size[1]}", f"{size[0]}x{size[1]}", f"size {size[0]} by {size[1]}"]
    if colour:
        pieces["colour"] = [f"in {colour}", colour, f"with a {colour} background", f"in {colour} colour"]
    return scrambler.Part(tidy, pieces, keep=["name"])


def window_size():
    from trainer import scrambler
    a, b = random.choice([300, 400, 640, 800, 1024, 1280]), random.choice([200, 300, 480, 600, 768, 720])
    return scrambler.Part(f"make the window {a} by {b}", {
        "verb": ["make", "resize", "set", "size", ""], "window": ["the window", "window", "my window"],
        "size": [f"{a} by {b}", f"{a}x{b}", f"to {a} by {b}", f"{a} wide and {b} high"]})


def window_name():
    from trainer import scrambler
    name = random.choice(NAMES)
    return scrambler.Part(f"name the window {name}", {
        "verb": ["name", "call", "title", "rename", "label"], "window": ["the window", "window", "my window"],
        "name": [name, f"to {name}"]}, keep=["name"])


def window_colour():
    from trainer import scrambler
    colour = random.choice(scrambler.COLOURS)
    return scrambler.Part(f"make the window {colour}", {
        "verb": ["make", "paint", "colour", "color", "turn"], "window": ["the window", "window", "my window"],
        "colour": [colour, f"in {colour}", f"{colour} colour"]})


def window_close():
    from trainer import scrambler
    return scrambler.Part("close the window", {"verb": ["close", "shut", "close up"],
                                               "window": ["the window", "window", "my window"]})


WINDOW_MAKERS = [window_write, window_write, window_write, window_open, window_size, window_name, window_colour,
                 window_close]


def makers():
    from trainer import scrambler
    return scrambler.MAKERS + WINDOW_MAKERS


def example():
    """One messy direction (the scrambler's kinds, or brick's own window) with every word's role and its tidy
    words."""
    from trainer import scrambler
    COLOURS.update(scrambler.COLOURS)
    part = random.choice(makers())()
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
        elif name == "window":
            pairs += mess([(w, "G" if w.lower() in WINDOW_WORDS else "X") for w in said.split()])
        elif name == "name":
            m = re.match(r"((?:called|named|titled|with title|title|to) )?(.*)$", said)
            pairs += [(w, "F") for w in (m.group(1) or "").split()] + [(w, "M") for w in m.group(2).split()]
        elif name == "size":
            pairs += mess([(w, "S" if re.search(r"\d", w) else "X") for w in said.split()])
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

    def decode(self, feats, w=None, allowed=None):
        """The best roles for the whole direction (Viterbi over each word's role and the role before it), each word
        only in a role it fits when allowed is given ([set of roles] per word)."""
        w = self.w if w is None else w
        best = [{}]
        for i, fs in enumerate(feats):
            e = self.emit(fs, w)
            col = {}
            for r in (allowed[i] if allowed else ROLES):
                if i == 0:
                    col[r] = (e.get(r, 0.0) + w.get("from:(start)", {}).get(r, 0.0), None)
                else:
                    col[r] = max(((best[i][q][0] + w.get(f"from:{q}", {}).get(r, 0.0) + e.get(r, 0.0), q)
                                  for q in best[i]), key=lambda x: x[0])
            best.append(col)
        r = max(best[-1], key=lambda x: best[-1][x][0])
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

    def roles(self, words, forbid=None):
        """Each word's role: the best for the whole direction, each word only in a role it can fit (and not in
        the roles forbid gives it: {word index: roles})."""
        if not words:
            return []
        allowed = [{r for r in ROLES if r in ("X", "W", "M") or fits(w, r)} - (forbid or {}).get(i, set())
                   for i, w in enumerate(words)]
        return self.decode(features(words), allowed=allowed)

    def other_roles(self, words, roles):
        """Readings where one word that fits two roles takes its other one ('bg wite': white, not write), or the
        first of two colours is the words to show ('write yellow in yellow')."""
        for i, (w, r) in enumerate(zip(words, roles)):
            low = w.lower().strip(".,!?;:")
            if r in ("V", "C") and fits(w, "C" if r == "V" else "V") and low not in VERBS and \
                    low not in colour_words_set():            # only a slip can be either: 'write' is write
                yield self.roles(words, {i: {r}})
        colours = [i for i, r in enumerate(roles) if r == "C"]
        if len(colours) > 1:
            yield self.roles(words, {colours[0]: set(ROLES) - {"W"}})


# ---------------- saying it again ----------------

NUMBER_WORDS = {"one": 1, "two": 2, "three": 3, "four": 4, "five": 5, "six": 6, "seven": 7, "eight": 8, "nine": 9,
                "ten": 10, "twice": 2, "once": 1}


def number(words):
    for w in words:
        m = re.search(r"\d+", w)
        if m:
            return int(m.group())
        if w.lower() in NUMBER_WORDS:
            return NUMBER_WORDS[w.lower()]
        if w.lower() in ("half", "a", "one"):
            return {"half": None, "a": 1, "one": 1}[w.lower()]
    return None


def one_slip(a, b):
    """Is a one slip away from b: a letter swapped with the next, missing, doubled or wrong (for words of 4+)?"""
    if a == b:
        return True
    if min(len(a), len(b)) < 3 or max(len(a), len(b)) < 4 or abs(len(a) - len(b)) > 1:
        return False
    if len(a) == len(b) and len(a) < 4:
        return False
    if len(a) == len(b):
        diff = [i for i in range(len(a)) if a[i] != b[i]]
        return len(diff) == 1 or len(diff) == 2 and diff[1] == diff[0] + 1 and a[diff[0]] == b[diff[1]] and \
            a[diff[1]] == b[diff[0]]
    short, long_ = (a, b) if len(a) < len(b) else (b, a)
    return any(long_[:i] + long_[i + 1:] == short for i in range(len(long_)))


def near(word, choices):
    """The choice this word is, or is one slip of -> the choice, or None."""
    if word in choices:
        return word
    return next((c for c in sorted(choices) if one_slip(word, c)), None)


def colour_of(words):
    from trainer import picker
    from actions import colour_words
    known = set(colour_words())
    for w in words:
        low = w.lower().strip(".,!?;:")
        if low in known:
            return low
        from trainer import scrambler                      # the common colours first: 'godl' is gold, not goel
        slip = near(low, set(scrambler.COLOURS) | {"black", "grey", "gray", "brown", "silver"}) if len(low) > 3 \
            else None                                # (a rare colour's name only spelled right: 'zorp' isn't zomp)
        if slip:
            return slip
    return None


def place_of(words):
    import difflib
    for w in words:
        low = w.lower().strip(".,!?;:")
        if low in SIDES:
            return low
        if near(low, SIDES):
            return near(low, SIDES)
        close = difflib.get_close_matches(low, SIDES, n=1, cutoff=0.8)
        if close:
            return close[0]
    return None


ALL_BRICKS = ["write", "set", "wait", "clear", "put", "window write in window", "window window open plan",
              "window window phrase"]


def key_part(brick, words, roles):
    """Does the direction have what this brick is about (a time for a wait, a colour for a set ...)?"""
    present = set(roles)
    verbs = {w.lower().strip(".,!?;:") for w, r in zip(words, roles) if r == "V"}
    return {"write": "W" in present, "put": "K" in present or "W" in present, "place": "W" in present,
            "set": "C" in present and bool(present & {"B", "Q"}),
            "wait": "T" in present and "W" not in present,
            "clear": bool({near(v, {"clear", "wipe", "cls", "erase"}) for v in verbs} - {None}),
            "window write in window": "G" in present and "W" in present,
            "window window open plan": "G" in present, "window window phrase": "G" in present}.get(brick, True)


USES = {"write": set("WFCPNTV"), "console": set("KPVF"), "set": set("CBQVF"), "clear": set("V"), "wait": set("TV"),
        "window write": set("WFCPNTGV"), "window open": set("GMFSCV"), "window phrase": set("GMFSCV")}


def say_again(brick, words, roles):
    """The direction in brick's words, from its brick and each word's role -> text, or None if a part it needs is
    missing, or a part was read that it doesn't use (a time can't vanish into 'clear the screen')."""
    text = say_again_parts(brick, words, roles)
    if text is None:
        return None
    verbs = {w.lower().strip(".,!?;:") for w, r in zip(words, roles) if r == "V"} - {"up"}
    shown = [w for w, r in zip(words, roles) if r == "W"]
    if shown and shown[-1].endswith("?") and not shown[-1].endswith("??"):
        return None                                  # 'what is 7 x 6?': the question mark may be meant
    own = {"wait": {"wait", "pause", "sleep", "hold"}, "clear": {"clear", "wipe", "cls", "erase"},
           "set": {"make", "turn", "set", "paint", "colour", "color", "change"}, "console": {"put", "move", "place"}}
    kind = ("window write" if text.split(" in the window")[0] != text else "window open" if text.startswith("open")
            else "window phrase" if "the window" in text else "console" if "console" in text
            else "set" if text.startswith("set") else "clear" if text.startswith("clear")
            else "wait" if text.startswith("wait") else "write")
    found = {r for r in roles if r != "X"}
    def one_of(v, choices):                          # the word, or a slip of it ('print' is no slip of paint)
        return v in choices or v not in VERBS and v not in action_words() and near(v, choices) is not None
    if kind in own and verbs and not all(one_of(v, own[kind]) for v in verbs):
        return None                                  # the action word doesn't suit it ('after 3 secs SHADOW')
    if kind == "write" and any(one_of(v, set().union(*own.values()) - {"put", "make"}) for v in verbs):
        return None
    return text if found <= USES[kind] else None


def say_again_parts(brick, words, roles):
    part = {r: [w for w, x in zip(words, roles) if x == r] for r in ROLES}
    later = number(part["T"]) if part["T"] else None
    if part["G"]:
        return say_window(brick, part, later)
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


def sizes(words):
    found = [int(n) for w in words for n in re.findall(r"\d+", w)]
    return found if len(found) == 2 else None


def say_window(brick, part, later):
    """A direction about brick's own window, from its brick and the words' roles -> text, or None."""
    verbs = {w.lower().strip(".,!?;:") for w in part["V"]}
    colour = colour_of(part["C"]) if part["C"] else None
    name = " ".join(part["M"]).strip(".,!?;:")
    size = sizes(part["S"]) if part["S"] else None
    if part["C"] and not colour or part["S"] and not size:
        return None
    if part["W"] and brick in ("write", "put", "place", "window write in window"):
        shown = " ".join(w.strip('"') for w in part["W"]).strip(".,!?;:")
        place, count = place_of(part["P"]), number(part["N"]) if part["N"] else None
        if part["P"] and not place:
            return None
        text = f"write {shown} in the window" + (f" at the {place}" if place else "") + \
            (f" in {colour}" if colour else "") + (f" {count} times" if count else "")
        return (f"after {later} seconds " if later else "") + text
    if part["W"] or part["P"] or part["N"] or later:
        return None                                  # (brick's own window can't move: the desktop places it)
    verbs -= {"up", "a", "new"}

    def said_with(*allowed):                         # the action word is said, and fits ('write' doesn't open
        return bool(verbs) and all(near(v, set(allowed)) for v in verbs)      # a window)
    if brick == "window window open plan" and said_with("open", "make", "create", "start", "pop", "show"):
        return "open a window" + (f" called {name}" if name else "") + (f" {size[0]} by {size[1]}" if size else "") + \
            (f" in {colour}" if colour else "")
    if brick == "window window phrase":
        if size and not (name or colour) and (not verbs or said_with("make", "resize", "set", "size")):
            return f"make the window {size[0]} by {size[1]}"
        if name and not (size or colour) and said_with("name", "call", "title", "rename", "label", "set"):
            return f"name the window {name}"
        if colour and not (size or name) and said_with("make", "paint", "colour", "color", "turn", "set"):
            return f"make the window {colour}"
        if not (size or name or colour) and verbs and said_with("close", "shut"):
            return "close the window"
    return None


FILLER = {"secs", "sec", "seconds", "second", "s", "times", "time", "screen", "side", "color", "colour", "of", "the",
          "a", "with", "at", "to", "on", "in", "rn", "thx", "thanks", "pls", "please", "lol", "yeah", "ok", "so", "um",
          "hey", "just", "now", "right", "and", "then", "can", "you", "u", "real", "quick", "also", "&", "x", "for",
          "my", "new", "that", "up", "by", "size", "wide", "high", "inside", "background"}


def filler(word):
    """Chatter and small words a direction can leave out without losing anything ('ok so', 'side of screen')."""
    from trainer import picker, scrambler
    low = word.lower().strip(".,!?;:-")
    if not low or not re.search(r"[a-z0-9]", low):
        return True
    chatter = {w for c in scrambler.CHATTER_BEFORE + scrambler.CHATTER_AFTER for w in c.split()}
    if low in FILLER or low in chatter or low in picker.SMALL or picker.respell([low])[0] in FILLER:
        return True
    import difflib                                   # a slip of one ('wiht', 'insside', 'affter')
    pool = [w for w in FILLER | chatter | TIME_MARKS if len(w) > 2]
    return len(low) > 3 and (near(low, set(pool)) is not None or
                             bool(difflib.get_close_matches(low, pool, n=1, cutoff=0.75)))


SLIP_VERBS = {"write", "show", "print", "display", "type", "clear", "wait", "pause", "sleep", "open", "close",
              "resize", "rename"}         # action words whose slips are common ('tpe', 'wirte', 'claer')
TIME_MARKS = {"after", "sec", "secs", "second", "seconds", "s", "minute", "minutes", "min", "mins"}
JOINERS = {"but", "and", "or", "so", "then", "that", "which"}
_actions = None


def action_words():
    """brick's own action words and the dictionary's words for them (scribble, inquire, exhibit ...)."""
    global _actions
    import dictionary
    if _actions is None:
        from actions import STARTERS
        _actions = set(STARTERS) | set(dictionary.first_words("action", "loop"))
    return _actions


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
    for one in ("W", "M"):                             # the words to show, or a name, are one stretch
        found = [i for i, r in enumerate(roles) if r == one]
        if found and found != list(range(found[0], found[-1] + 1)):
            return False
    shown = [i for i, r in enumerate(roles) if r == "W"]
    if shown and (low[shown[0]] in _actions and not words[shown[0]].startswith('"') or low[shown[-1]] in JOINERS):
        return False
    for i in (shown[:1] + shown[-1:]) if shown else []:      # the words to show start or end on a colour or a
        if words[i].startswith('"'):                          # number ('orange juice', 'paint everything black',
            continue                                          # 'hello three times'): is it shown, or a colour?
        from actions import colour_words
        if low[i] in colour_words() and "C" not in roles or low[i] in NUMBER_WORDS and i + 1 < len(low) and \
                low[i + 1] in ("times", "time"):       # (spelled exactly: 'back' isn't black; 'nice one' is shown;
            return False                               # 'write yellow in yellow': the colour is said on its own)
    if any(not words[i].startswith('"') and any(fits(words[i], r) for r in "BKGQ") and
           low[i] not in ("text", "writing") for i in shown):
        return False                                   # shown words that could be a thing ('backcloth crimson')
    if shown and shown[0] and roles[shown[0] - 1] == "C":    # a colour right before them that makes a phrase
        from trainer import conceptnet                        # with them ('orange juice', 'red wine'): shown,
        if conceptnet.web().together(low[shown[0] - 1], low[shown[0]]) > 0:   # or a colour? not sure
            return False
    if shown and any(not words[i].startswith('"') and low[i] not in VERBS and near(low[i], SLIP_VERBS)
                     for i in (shown[0], shown[-1])):
        return False                                   # 'welcome back tpe': a slip of an action word isn't shown
    verbs = {picker.respell([low[i]])[0] for i, r in enumerate(roles) if r == "V"} - {"up"}   # 'open up', 'pop up'
    if len(verbs) > 1:
        return False
    marks = set(low)
    for i, r in enumerate(roles):
        if r == "T" and re.search(r"\d", low[i]) and not (marks & TIME_MARKS or re.fullmatch(r"\d+s", low[i])):
            return False
        if r == "N" and re.fullmatch(r"\d+", low[i]) and not marks & {"times", "time"}:
            return False
    return True


def colour_words_set():
    from actions import colour_words
    return set(colour_words())


def stands_for(word, things):
    """Does ConceptNet say this word is the same as one of these ('backcloth' = backdrop)?"""
    from trainer import conceptnet
    if not os.path.isfile(conceptnet.PATH) or len(word) < 4:
        return False
    return any(other in things for other, _ in conceptnet.web().stands_for(word))


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
        return bool(re.search(r"\d", low)) or low in ("half", "a") or low in NUMBER_WORDS or filler(word)
    slip = globals()["near"]
    if role == "B":
        return {low, near} & {"bg", "background"} != set() or slip(low, {"background"}) is not None or \
            stands_for(low, {"background", "backdrop"})
    if role == "K":
        return "console" in (low, near) or slip(low, {"console"}) is not None
    if role == "Q":
        return {low, near} & {"text", "writing"} != set() or slip(low, {"text", "writing"}) is not None or \
            low in ("tet", "txt")
    if role == "F":
        return {low, near} & {"text", "that", "says", "the", "words", "saying", "called", "named", "titled", "title",
                              "with", "to"} != set()
    if role == "G":
        return {low, near} & WINDOW_WORDS != set() or slip(low, {"window"}) is not None
    if role == "S":
        return bool(re.search(r"\d", low))
    if role == "M":
        return True
    if role == "V":
        return low in VERBS or near in VERBS or globals()["near"](low, SLIP_VERBS) is not None or \
            low in action_words()
    return True


_details = None


def read(line):
    """The brain's whole reading of a line: cut into directions (the cutter), each one's brick (the picker), each
    word's role, said again in brick's words -> text, or None where it can't say one direction. When a cut piece
    can't be read, the line read whole, as one direction ('write yellow in yellow on a window')."""
    return read_cut(line) or read_cut(line, whole=True)


def read_cut(line, whole=False):
    global _details
    from trainer import cutter, picker, universe as u
    if _details is None:
        _details = Details()
    if picker._picker is None:
        picker._picker = picker.Picker()
    frames = None if whole else cutter.cut(u.tokens_of(line, gaps=True))
    pieces = [" ".join(t.text for t in fr) for fr in frames] if frames else [line]
    said = []
    for piece in pieces:
        picked = picker._picker.pick(piece)
        words = re.findall(r'"[^"]*"|\S+', piece)
        if (not picked or picked[0][0] == "none") and all(filler(w) for w in words):
            continue                             # chatter (other words: their roles say which brick it is)
        roles = _details.roles(words)
        if all(r == "X" for r in roles) and all(filler(w) for w in words):
            continue                             # only chatter
        again = reading(picked, words, roles)
        for other in ([] if again else _details.other_roles(words, roles)):
            again = reading(picked, words, other)
            if again:
                break
        if again is None:
            return None
        said.append(again)
    return " then ".join(said) if said else None


def reading(picked, words, roles):
    """One direction said again from its words' roles, with the surest brick that fits them -> text, or None."""
    has_window = "G" in roles
    if any(r == "X" and not filler(w) and not (has_window and (fits(w, "G") or fits(w, "B")))
           for w, r in zip(words, roles)) or not all(fits(w, r) for w, r in zip(words, roles)):
        return None                              # a word it has no role for, or one that doesn't fit its role
    if not consistent(words, roles):
        return None                              # the roles don't hang together: it isn't sure
    ranked = [b for b, share in picked if b != "none" and share >= 0.02]
    ranked += [b for b in ALL_BRICKS if b not in ranked]
    for brick in ranked:                         # the surest brick whose direction uses every role read and has
        if not key_part(brick, words, roles):      # its own key part ('pls hold on 4 secs' is a wait: a time)
            continue
        again = say_again(brick, words, roles)
        if again:
            return again
    return None


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


def paragraph(window_share=0.5):
    """A broken paragraph like the scrambler's, with directions about brick's own window mixed in."""
    from trainer import scrambler
    parts = [random.choice(WINDOW_MAKERS if random.random() < window_share else scrambler.MAKERS)()
             for _ in range(random.randint(2, 4))]
    mess_text = ""
    for k, p in enumerate(parts):
        said = scrambler.say(p)
        if random.random() < 0.25:
            said = f"{random.choice(scrambler.CHATTER_BEFORE)} {said}"
        if random.random() < 0.15:
            said = f"{said} {random.choice(scrambler.CHATTER_AFTER)}"
        mess_text += (random.choice(scrambler.JOINS) if k else "") + said
    if random.random() < 0.3:
        mess_text = mess_text.lower()
    return mess_text + random.choice(["", "", ".", "!", "...", " lol", "??"]), [p.tidy for p in parts]


def score(count=300, seed=777, workers=10):
    from multiprocessing import Pool
    from trainer import scrambler
    os.environ.setdefault("BRICK_NEURAL", "off")
    random.seed(seed)
    fresh = []
    for _ in range(count):
        mess_text, _, tidy = scrambler.paragraph()
        fresh.append((mess_text, " then ".join(tidy)))
    windows = []
    for _ in range(count):
        mess_text, tidy = paragraph()
        windows.append((mess_text, " then ".join(tidy)))
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
        for name, jobs in (("fresh scrambled paragraphs", fresh), ("with window directions", windows),
                           ("bench paragraphs", bench)):
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
