"""The digital brain's Ni, second half: what each word of a direction is for, and the direction said in brick's words.

trainer/picker.py picks the brick (write, set, wait ...). This reads the rest: for every word, its role
  W  words to show        C  a colour        P  a place          N  how many times    T  a time
  F  the frame of words to show or a name ('text that says', 'the words', 'called', 'as')
  B  the background       K  the console     Q  the text itself ('make the writing red')
  G  brick's own window   M  a name ('called My Game')   S  a size ('800 by 600')   D  a file ('notes.txt')
  A  an amount or value ('set score to 5', 'count to 10')               H  a button
  V  the action word      X  chatter and small words
then (Ti) checks the roles hold together and (Ne) tries others when they don't, and says the direction again
exactly ('after 2 seconds write hi in red at the top 3 times'). trainer/mind.py is where brick asks the brain.

It learns strictly from its own imagination: the archetypes voice directions in their personalities
(trainer/voices.py), each keeping its meaning, so each word's role can be read off the meaning. A structured
perceptron: each word's role is weighed by the word, its neighbours, its shape and the role before it; the best
roles for the whole direction win (Viterbi), each word only in a role it can fit. It learns from its own mistakes.
The links are kept in trainer/data/brain-details.txt.

    python3 -m trainer.details learn             learn from the archetypes' practice
    python3 -m trainer.details try "a line"      each word's role, and the direction said again
"""

import os
import random
import re
import sys
import tempfile

HERE = os.path.dirname(os.path.abspath(__file__))
sys.path.insert(0, os.path.dirname(HERE))

LINKS = os.path.join(HERE, "data", "brain-details.txt")
ROLES = ["W", "F", "C", "P", "N", "T", "B", "K", "Q", "G", "M", "S", "D", "A", "H", "V", "X"]
FILE_NAME = re.compile(r"[\w-]+\.(?:txt|csv|md|log|json|dat)$", re.IGNORECASE)
WINDOW_WORDS = {"window", "windows", "win", "windw"}
VERBS = {"write", "say", "show", "print", "display", "type", "put", "make", "turn", "set", "paint", "move", "clear",
         "wipe", "cls", "wait", "pause", "sleep", "hold", "add", "open", "create", "start", "pop", "resize", "size",
         "name", "call", "title", "rename", "label", "colour", "color", "close", "shut", "up", "save", "store",
         "record", "keep", "stash", "away", "append", "load", "read", "get", "delete", "remove", "erase", "ask",
         "question", "prompt", "let", "plus", "increase", "take", "subtract", "minus", "decrease", "create", "start",
         "count", "countdown", "beep", "ding", "stop", "quit", "end", "exit", "change"}
SIDES = {"left", "right", "top", "bottom", "middle", "centre", "center", "top-left", "top-right", "bottom-left",
         "bottom-right"}


# ---------------- meanings the archetypes are handed to voice (brick's own window, files) ----------------

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


# ---------------- files: practice directions with known parts ----------------

FILES = ["notes.txt", "todo.txt", "scores.txt", "names.txt", "log.txt", "diary.txt", "shopping.txt", "best.txt",
         "words.txt", "results.txt", "save.txt", "list.txt"]
LOAD_NAMES = ["notes", "names", "best", "words", "lines", "todo"]


def said_words():
    from trainer import scrambler
    w = random.choice(scrambler.WORDS)
    return w, [w, f'"{w}"']


def file_save():
    from trainer import scrambler
    w, ways = said_words()
    f = random.choice(FILES)
    return scrambler.Part(f"save {w} to {f}", {"verb": ["save", "store", "record", "keep", "stash away", "put"],
                                               "words": ways, "file": [f"to {f}", f"in {f}", f"into {f}"]},
                          keep=["words"])


def file_add():
    from trainer import scrambler
    w, ways = said_words()
    f = random.choice(FILES)
    return scrambler.Part(f"add {w} to the end of {f}", {"verb": ["add", "append"], "words": ways,
                          "file": [f"to the end of {f}", f"to {f}", f"onto {f}", f"at the end of {f}"]},
                          keep=["words"])


def file_write():
    from trainer import scrambler
    w, ways = said_words()
    f = random.choice(FILES)
    return scrambler.Part(f"write {w} to {f}", {"verb": ["write"], "words": ways, "file": [f"to {f}"]},
                          keep=["words"])


def file_load():
    from trainer import scrambler
    f, n = random.choice(FILES), random.choice(LOAD_NAMES + [None, None])
    pieces = {"verb": ["load", "open", "read", "get"], "file": [f]}
    if n:
        pieces["as"] = [f"as {n}"]
    return scrambler.Part(f"load {f}" + (f" as {n}" if n else ""), pieces)


def file_delete():
    from trainer import scrambler
    f = random.choice(FILES)
    return scrambler.Part(f"delete {f}", {"verb": ["delete", "remove", "erase"], "file": [f, f"the file {f}"]})


def file_show():
    from trainer import scrambler
    f = random.choice(FILES)
    return scrambler.Part(f"show {f}", {"verb": ["show", "display"], "file": [f, f"the file {f}"]})


FILE_MAKERS = [file_save, file_save, file_add, file_write, file_load, file_delete, file_show]


# ---------------- questions, values, lists, counting, buttons, beeps ----------------

QUESTIONS = ["What is your name?", "How old are you?", "Your favourite colour?", "Pick a number", "Ready?",
             "Where do you live?"]
ASK_NAMES = ["name", "age", "colour", "guess", "answer", "place"]
VALUES = ["score", "lives", "level", "speed", "points", "count"]
LISTS = ["fruits", "names", "pets", "jobs", "colours", "games"]
ITEMS = ["apple", "pear", "plum", "cat", "dog", "fish", "red", "blue", "Bob", "Ann", "chess", "tennis"]
BUTTONS = [("go", "Go"), ("start", "Start"), ("quit", "Quit"), ("play", "Play"), ("ok", "OK"), ("next", "Next")]


def question():
    from trainer import scrambler
    q, n = random.choice(QUESTIONS), random.choice(ASK_NAMES)
    return scrambler.Part(f'ask "{q}" as {n}', {"verb": ["ask", "ask me", "ask the user"], "words": [f'"{q}"'],
                                                "as": [f"as {n}"]}, keep=["words"])


def value():
    from trainer import scrambler
    v, n = random.choice(VALUES), random.randint(0, 20)
    return scrambler.Part(f"set {v} to {n}", {"verb": ["set", "make", "let"], "name": [v, f"the {v}"],
                                              "to": [f"to {n}", f"= {n}", f"be {n}"]}, keep=["name"])


def change():
    """'add 3 to score' (after 'set score to 0': it needs the value first)."""
    from trainer import scrambler
    v, n = random.choice(VALUES), random.randint(1, 9)
    up = random.random() < 0.6
    part = scrambler.Part(f"add {n} to {v}" if up else f"take {n} from {v}",
                          {"verb": (["add", "plus", "increase"] if up else ["take", "subtract", "minus"]),
                           "to": [f"{n} to {v}" if up else f"{n} from {v}", f"{n} {'to' if up else 'from'} the {v}"]})
    part.before = [f"set {v} to {random.randint(10, 20)}"]
    return part


def a_list():
    from trainer import scrambler
    n, items = random.choice(LISTS), random.sample(ITEMS, random.randint(0, 3))
    said = ", ".join(items[:-1]) + (" and " if len(items) > 1 else "") + (items[-1] if items else "")
    pieces = {"verb": ["make", "create", "start"], "list": [f"a list called {n}", f"a list named {n}", f"list {n}"]}
    if items:
        pieces["with"] = [f"with {said}", f"of {said}", f"holding {said}"]
    return scrambler.Part(f"make a list called {n}" + (f" with {said}" if items else ""), pieces, keep=["list", "with"])


def counting():
    from trainer import scrambler
    a = random.randint(2, 20)
    kind = random.choice(["to", "from", "down"])
    if kind == "to":
        return scrambler.Part(f"count to {a}", {"verb": ["count", "count up"], "to": [f"to {a}", f"up to {a}"]})
    if kind == "from":
        b = a + random.randint(3, 10)
        return scrambler.Part(f"count from {a} to {b}", {"verb": ["count"], "to": [f"from {a} to {b}"]})
    return scrambler.Part(f"count down from {a}", {"verb": ["count down", "countdown"], "to": [f"from {a}"]})


def button():
    from trainer import scrambler
    (n, label), colour = random.choice(BUTTONS), random.choice(scrambler.COLOURS + [None, None])
    side = random.choice(WINDOW_SIDES + [None, None])
    tidy = f"make a button called {n} saying {label}" + (f" in {colour}" if colour else "") + \
        (f" at the {side}" if side else "")
    pieces = {"verb": ["make", "add", "put", "create"], "button": [f"a button called {n}", f"a button named {n}",
                                                                 f"button {n}"],
              "words": [f"saying {label}", f'that says "{label}"', f"with {label} on it", f'"{label}"']}
    if colour:
        pieces["colour"] = [f"in {colour}", colour, f"{colour} coloured"]
    if side:
        pieces["place"] = [f"at the {side}", side, f"on the {side}"]
    return scrambler.Part(tidy, pieces, keep=["button", "words"])


def plain():
    from trainer import scrambler
    tidy, ways = random.choice([("beep", ["beep", "ding", "make a beep", "go ding"]),
                                ("stop", ["stop", "quit", "end", "exit", "stop the program"]),
                                ("wait for a key", ["wait for a key", "wait till i press a key", "wait for any key"])])
    return scrambler.Part(tidy, {"all": ways})


PHRASES = [("orange", "juice"), ("red", "wine"), ("black", "hole"), ("green", "tea"), ("blue", "sky"),
           ("white", "board"), ("gold", "fish"), ("pink", "panther"), ("yellow", "submarine"), ("red", "alert"),
           ("green", "light"), ("blue", "moon")]


def look_alike():
    """A colour word and a word it makes a known thing with: shown together ('write orange juice at the top'), or
    the colour of a word ('write juice in orange'): the same words, two meanings, told apart by what links them."""
    from trainer import scrambler
    (colour, noun), side = random.choice(PHRASES), random.choice(scrambler.SIDES + [None])
    where = f" at the {side}" if side else ""
    if random.random() < 0.5:
        other = random.choice([c for c in scrambler.COLOURS if c != colour] + [None, None])
        return scrambler.Part(f"write {colour} {noun}" + (f" in {other}" if other else "") + where,
                              {"verb": ["write", "show", "type", "put"], "words": [f"{colour} {noun}", f'"{colour} {noun}"',
                                                                                   f"the words {colour} {noun}"],
                               **({"colour": [f"in {other}", f"with {other} color"]} if other else {}),
                               **({"place": [f"at the {side}", side]} if side else {})}, keep=["words"])
    return scrambler.Part(f"write {noun} in {colour}" + where,
                          {"verb": ["write", "show", "type", "put"], "words": [noun, f'"{noun}"'],
                           "colour": [f"in {colour}", f"with {colour} color", f"in the color {colour}"],
                           **({"place": [f"at the {side}", side]} if side else {})}, keep=["words"])


def colour_as_words():
    """A colour word that is itself the thing to show ('can you write purple', 'write pink in pink')."""
    from trainer import scrambler
    word, colour = random.choice(scrambler.COLOURS), random.choice(scrambler.COLOURS + [None, None])
    side = random.choice(scrambler.SIDES + [None, None])
    tidy = f"write {word}" + (f" in {colour}" if colour else "") + (f" at the {side}" if side else "")
    pieces = {"verb": ["write", "say", "show", "type", "can you write", "put"],
              "words": [word, f"the word {word}", f'"{word}"', f"the text {word}"]}
    if colour:
        pieces["colour"] = [f"in {colour}", f"in the color {colour}", f"with {colour} color"]
    if side:
        pieces["place"] = [f"at the {side}", f"on the {side} side of the screen", f"at {side} side of screen"]
    return scrambler.Part(tidy, pieces, keep=["words"])


OTHER_MAKERS = [question, question, value, change, change, a_list, counting, button, button, plain,
                look_alike, look_alike, look_alike, colour_as_words, colour_as_words, colour_as_words]


def makers():
    from trainer import scrambler
    return scrambler.MAKERS + WINDOW_MAKERS + FILE_MAKERS + OTHER_MAKERS


# ---------------- the digital brain's own practice: the archetypes' voiced directions ----------------

def voiced_roles(part, tidy):
    """One direction an archetype voiced (part) and its meaning (tidy) -> [(word, role)], each word's role read off
    what the meaning keeps: the words to show, the file, colours, places, numbers stay exactly as they were."""
    from trainer import voices
    from actions import colour_words
    words = re.findall(r'"[^"]*"|\S+', part)
    low = [w.lower().strip(".,!?;:") for w in words]
    roles = ["X"] * len(words)
    for shown in voices.shows(tidy):                   # the words to show: quoted, or the same words in a row
        target = shown.lower().split()
        for i, w in enumerate(words):
            if w.startswith('"') and w.strip('"').lower().strip(".,!?;:") == shown.lower():
                roles[i] = "W"
                break
            if low[i:i + len(target)] == target and all(r == "X" for r in roles[i:i + len(target)]):
                roles[i:i + len(target)] = ["W"] * len(target)
                break
    name = re.search(r"(?:called|name the window|^set|^ask \"[^\"]*\" as) (.+?)(?= saying | with | to \d| \d+ by \d+"
                     r"| in \w+| at the |$)", tidy)
    if not name and re.match(r"^(add|take) \d+ (to|from) (\w+)$", tidy):
        name = re.match(r"^(?:add|take) \d+ (?:to|from) (\w+)()$", tidy)
    load_as = re.search(r" as (\w+)$", tidy)
    items = re.match(r"^make a list called \w+ with (.+)$", tidy)
    item_words = {w.lower() for w in re.split(r",\s*|\s+and\s+", items.group(1))} if items else set()
    tidy_low = tidy.lower()
    colours, verbs_ = set(colour_words()), VERBS | action_words()
    for i, w in enumerate(low):
        if roles[i] != "X":
            continue
        if FILE_NAME.match(w) and w in tidy_low:
            roles[i] = "D"
        elif w in item_words:
            roles[i] = "W"
        elif w in ("button", "btn") and "button" in tidy:
            roles[i] = "H"
        elif w in colours and re.search(rf"\b{re.escape(w)}\b", tidy_low):
            roles[i] = "C"
        elif w in SIDES and re.search(rf"\b{re.escape(w)}\b", tidy_low):
            roles[i] = "P"
        elif re.search(r"\d", w):
            n = re.search(r"\d+", w).group()
            if re.search(rf"\b{n} by \d+|\d+ by {n}\b", tidy):
                roles[i] = "S"
            elif re.search(rf"\b{n} times\b", tidy):
                roles[i] = "N"
            elif re.search(rf"(after|wait) {n} second", tidy):
                roles[i] = "T"
            elif re.search(rf"\b{n}\b", tidy):
                roles[i] = "A"
        elif name and w in name.group(1).lower().split() or load_as and w == load_as.group(1):
            roles[i] = "M"
        elif (w in ("bg", "bkg", "backdrop") or fits(w, "B")) and "background" in tidy:
            roles[i] = "B"
        elif (w == "terminal" or fits(w, "K")) and "console" in tidy:
            roles[i] = "K"
        elif (w in ("font", "txt") or fits(w, "Q")) and tidy.startswith("set the colour"):
            roles[i] = "Q"
        elif fits(w, "G") and "window" in tidy:
            roles[i] = "G"
        elif w in ("saying", "says", "that", "words", "called", "named", "titled", "as", "with", "of") and \
                (i + 1 < len(roles) and (low[i + 1] in ("says", "that", "words") or i + 1 < len(words))):
            roles[i] = "F" if any(r == "W" or r == "M" for r in roles[i + 1:i + 4]) else "X"
        elif w in verbs_ or near(w, SLIP_VERBS) and not filler(w):
            roles[i] = "V"
        elif w == "as" and load_as:
            roles[i] = "F"
    return list(zip(words, roles))


def voiced_examples(limit=None):
    """[(pairs of (word, role), tidy)] from trainer/data/voiced-practice.jsonl (the archetypes' directions)."""
    import json
    from trainer import voices
    found = []
    if not os.path.isfile(voices.PRACTICE):
        return found
    from trainer.picker import NEWEST
    with open(voices.PRACTICE, encoding="utf-8") as f:
        for row in [r for r in f if r.strip()][-NEWEST:]:
            d = json.loads(row)
            for part, tidy in zip(d["parts"], d["tidy"]):
                found.append((voiced_roles(part, tidy), tidy))
    return found[:limit] if limit else found


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
              "window window phrase", "save", "add", "load", "delete", "ask", "take", "make", "count", "window make",
              "beep", "stop"]


def key_part(brick, words, roles):
    """Does the direction have what this brick is about (a time for a wait, a colour for a set ...)?"""
    present = set(roles)
    verbs = {w.lower().strip(".,!?;:") for w, r in zip(words, roles) if r == "V"}
    return {"write": "W" in present or "D" in present, "put": "K" in present or "W" in present,
            "place": "W" in present,
            "set": "C" in present and bool(present & {"B", "Q"}),
            "wait": ("T" in present or any(w.lower().strip(".,!?;:") == "key" for w in words)) and "W" not in present,
            "clear": bool({near(v, {"clear", "wipe", "cls"}) for v in verbs} - {None}),
            "window write in window": "G" in present and "W" in present,
            "window window open plan": "G" in present, "window window phrase": "G" in present,
            "save": "D" in present and "W" in present,
            "add": "D" in present and "W" in present or "M" in present and "A" in present,
            "load": "D" in present and "W" not in present, "delete": "D" in present and "W" not in present,
            "remove": "D" in present and "W" not in present,
            "ask": "M" in present, "take": "M" in present and "A" in present,
            "make": "H" in present or any(w.lower().strip(".,!?;:") == "list" for w in words),
            "count": "A" in present, "window make": "H" in present,
            "beep": bool(verbs & {"beep", "ding"}), "stop": bool(verbs & {"stop", "quit", "end", "exit"})}.get(brick, True)


USES = {"write": set("WFCPNTV"), "console": set("KPVF"), "set": set("CBQVF"), "clear": set("V"), "wait": set("TV"),
        "window write": set("WFCPNTGV"), "window open": set("GMFSCV"), "window phrase": set("GMFSCV"),
        "file save": set("WDVF"), "file add": set("WDVF"), "file write": set("WDVF"), "file load": set("DMFV"),
        "file delete": set("DV"), "file show": set("DV"),
        "ask": set("WFMV"), "value": set("MAVF"), "change": set("MAVF"), "list": set("MWVF"), "count": set("AV"),
        "button": set("MWFCPHV"), "plain": set("V"), "key": set("V")}


def say_again(brick, words, roles):
    """The direction in brick's words, from its brick and each word's role -> text, or None if a part it needs is
    missing, or a part was read that it doesn't use (a time can't vanish into 'clear the screen')."""
    text = say_again_parts(brick, words, roles)
    if text is None:
        return None
    verbs = {w.lower().strip(".,!?;:") for w, r in zip(words, roles) if r == "V"} - {"up", "away"}
    shown = [w for w, r in zip(words, roles) if r == "W"]
    if shown and shown[-1].endswith("?") and not shown[-1].endswith("??"):
        return None                                  # 'what is 7 x 6?': the question mark may be meant
    own = {"wait": {"wait", "pause", "sleep", "hold"}, "clear": {"clear", "wipe", "cls"},
           "set": {"make", "turn", "set", "paint", "colour", "color", "change"}, "console": {"put", "move", "place"},
           "file save": {"save", "store", "record", "keep", "stash", "put"}, "file add": {"add", "append"},
           "file write": {"write"}, "file load": {"load", "open", "read", "get"},
           "file delete": {"delete", "remove", "erase"}, "file show": {"show", "display"},
           "ask": {"ask", "question", "prompt"}, "value": {"set", "make", "let", "change"},
           "change": {"add", "plus", "increase", "take", "subtract", "minus", "decrease"},
           "list": {"make", "create", "start"}, "count": {"count", "countdown"},
           "button": {"make", "add", "put", "create"}, "plain": {"beep", "ding", "stop", "quit", "end", "exit", "make", "go"},
           "key": {"wait", "press"}}
    file_kind = {"save": "file save", "add": "file add", "write": "file write", "load": "file load",
                 "delete": "file delete", "show": "file show"}
    if any(r == "D" for r in roles):
        kind = file_kind[text.split()[0]]
    else:
        kind = ("ask" if text.startswith("ask ") else "list" if text.startswith("make a list") else
                "button" if text.startswith("make a button") else "count" if text.startswith("count ") else
                "key" if text == "wait for a key" else "plain" if text in ("beep", "stop") else
                "change" if re.match(r"^(add|take) \S+ (to|from) ", text) else
                "value" if re.match(r"^set (?!the )\S+ to ", text) else None)
    kind = kind or ("window write" if text.split(" in the window")[0] != text else "window open" if text.startswith("open")
            else "window phrase" if "the window" in text else "console" if "console" in text
            else "set" if text.startswith("set") else "clear" if text.startswith("clear")
            else "wait" if text.startswith("wait") else "write")
    found = {r for r in roles if r != "X"}
    def one_of(v, choices):                          # the word, or a slip of it ('print' is no slip of paint)
        return v in choices or v not in VERBS and v not in action_words() and near(v, choices) is not None
    if kind in own and verbs and not all(one_of(v, own[kind]) for v in verbs):
        return None                                  # the action word doesn't suit it ('after 3 secs SHADOW')
    if kind == "write" and any(one_of(v, set().union(*(own[k] for k in ("wait", "clear", "set", "console"))) -
                                      {"put", "make"}) for v in verbs):
        return None
    return text if found <= USES[kind] else None


def accounts_for(text, roles):
    """Does the reading use every word that isn't chatter (a time can't vanish into 'clear the screen')?"""
    file_kind = {"save": "file save", "add": "file add", "write": "file write", "load": "file load",
                 "delete": "file delete", "show": "file show"}
    if any(r == "D" for r in roles):
        kind = file_kind.get(text.split()[0])
    else:
        kind = ("ask" if text.startswith("ask ") else "list" if text.startswith("make a list") else
                "button" if text.startswith("make a button") else "count" if text.startswith("count ") else
                "key" if text == "wait for a key" else "plain" if text in ("beep", "stop") else
                "change" if re.match(r"^(add|take) \S+ (to|from) ", text) else
                "value" if re.match(r"^set (?!the )\S+ to ", text) else
                "window write" if " in the window" in text else "window open" if text.startswith("open") else
                "window phrase" if "the window" in text else "console" if "console" in text else
                "set" if text.startswith("set") else "clear" if text.startswith("clear") else
                "wait" if text.startswith("wait") else "write")
    return kind in USES and {r for r in roles if r != "X"} <= USES[kind]


def say_again_parts(brick, words, roles):
    part = {r: [w for w, x in zip(words, roles) if x == r] for r in ROLES}
    later = number(part["T"]) if part["T"] else None
    if part["D"]:
        return say_file(brick, part)
    other = say_other(brick, part, words)
    if other:
        return other
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


def say_other(brick, part, words):
    """Questions, values, lists, counting, buttons, beeps -> text, or None."""
    low = {w.lower().strip(".,!?;:") for w in words}
    name = " ".join(part["M"]).strip(".,!?;:")
    shown = " ".join(part["W"]).strip(".,!?;:").strip('"')
    amounts = [number([w]) for w in part["A"]]
    loose = part["C"] or part["P"] or part["N"] or part["T"] or part["G"] or part["K"] or part["B"] or part["S"]
    if part["H"]:
        if not (name and shown) or part["N"] or part["T"] or part["G"] or part["S"]:
            return None
        colour, place = colour_of(part["C"]) if part["C"] else None, place_of(part["P"]) if part["P"] else None
        if part["C"] and not colour or part["P"] and not place:
            return None
        return f"make a button called {name} saying {shown}" + (f" in {colour}" if colour else "") + \
            (f" at the {place}" if place else "")
    if loose:
        return None
    if brick == "ask" and name and len(part["M"]) == 1:
        return f'ask "{shown}" as {name}' if shown else None
    if brick == "set" and name and len(part["M"]) == 1 and len(amounts) == 1 and not shown:
        return f"set {name} to {amounts[0]}"
    if brick in ("add", "take") and name and len(part["M"]) == 1 and len(amounts) == 1 and not shown:
        return f"add {amounts[0]} to {name}" if brick == "add" else f"take {amounts[0]} from {name}"
    if brick == "make" and "list" in low and name and len(part["M"]) == 1:
        items = [w.strip(",.") for w in part["W"] if w.lower().strip(",.") != "and"]
        said = ", ".join(items[:-1]) + (" and " if len(items) > 1 else "") + (items[-1] if items else "")
        return f"make a list called {name}" + (f" with {said}" if items else "")
    if brick == "count" and amounts and not shown and not name:
        if len(amounts) == 2:
            return f"count from {amounts[0]} to {amounts[1]}"
        if len(amounts) == 1:
            return f"count down from {amounts[0]}" if low & {"down", "countdown"} else f"count to {amounts[0]}"
    if not (shown or name or amounts):
        if brick == "beep":
            return "beep"
        if brick == "stop":
            return "stop"
        if brick == "wait" and "key" in low:
            return "wait for a key"
    return None


def say_file(brick, part):
    """A direction about a file, from its brick and the words' roles -> text, or None."""
    f = part["D"][0].strip(",!?;:\"'")
    f = f[:-1] if f.endswith(".") and not FILE_NAME.match(f) else f
    shown = " ".join(part["W"]).strip(".,!?;:")
    if part["C"] or part["P"] or part["N"] or part["T"] or part["G"] or part["K"] or part["B"]:
        return None
    if shown and shown.startswith('"') != shown.endswith('"'):
        return None
    if brick in ("save", "put") and shown:
        return f"save {shown.strip(chr(34))} to {f}"
    if brick == "add" and shown:
        return f"add {shown.strip(chr(34))} to the end of {f}"
    if brick == "write" and shown:
        return f"write {shown.strip(chr(34))} to {f}"
    if shown:
        return None
    name = " ".join(part["M"]).strip(".,!?;:")
    if brick == "load":
        return f"load {f}" + (f" as {name}" if name else "")
    if part["M"]:
        return None
    if brick in ("delete", "remove"):
        return f"delete {f}"
    if brick == "write":
        return f"show {f}"
    return None


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
    framed = {w.lower() for w in part["F"]} & {"called", "named", "titled", "title"}
    if name and not framed and not said_with("name", "call", "title", "rename", "label", "set"):
        return None                                  # a name needs its frame ('called My Game'): 'half the screen'
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
          "my", "new", "that", "up", "by", "size", "wide", "high", "inside", "background", "end", "into", "onto",
          "away", "file", "down", "list", "key", "any", "program", "till", "until", "i", "press", "me", "user",
          "plus", "minus", "sound", "go", "make"}


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
              "resize", "rename", "save", "store", "record", "append", "load", "delete", "remove", "erase"}         # action words whose slips are common ('tpe', 'wirte', 'claer')
TIME_MARKS = {"after", "sec", "secs", "second", "seconds", "s", "minute", "minutes", "min", "mins"}
JOINERS = {"but", "and", "or", "so", "then", "that", "which"}
_actions = None


def action_words():
    """brick's own action words and the dictionary's words for them (scribble, inquire, exhibit ...)."""
    global _actions
    import dictionary
    if _actions is None:
        from actions import STARTERS
        _actions = (set(STARTERS) | set(dictionary.first_words("action", "loop"))) - \
            {"as", "file", "end", "can", "the", "a", "to", "in", "on", "at"}     # small words aren't actions here
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
    if len({low[i] for i, r in enumerate(roles) if r == "D"}) > 1:
        return False                                   # two different files
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
    verbs = {picker.respell([low[i]])[0] for i, r in enumerate(roles) if r == "V"} - {"up", "away"}   # 'open up', 'pop up'
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
                              "with", "to", "as", "of", "holding", "on", "it"} != set()
    if role == "G":
        return {low, near} & WINDOW_WORDS != set() or slip(low, {"window"}) is not None
    if role == "S":
        return bool(re.search(r"\d", low))
    if role == "M":
        return not filler(word) or low in ("name", "count", "colour")
    if role == "A":
        return bool(re.search(r"\d", low)) or low in NUMBER_WORDS
    if role == "H":
        return {low, near} & {"button", "btn", "buttons"} != set() or slip(low, {"button"}) is not None
    if role == "D":
        return bool(FILE_NAME.match(word.strip(".,!?;:\"'") if not FILE_NAME.match(word) else word))
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


# ---------------- learning ----------------

def learn(count=None, seed=4242):
    """The brain learns each word's role strictly from its archetypes' voiced directions."""
    random.seed(seed)
    examples = [pairs for pairs, _ in voiced_examples(count) if pairs]
    d = Details(path=LINKS)
    d.w = {}
    print(f"Learning the roles of words from {len(examples)} directions the archetypes voiced")
    d.learn(examples)
    d.save()
    print(f"Saved {sum(len(v) for v in d.w.values())} links -> {LINKS}")


def command(args):
    if args[:1] == ["learn"]:
        return learn(int(args[1]) if args[1:] else None)
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
