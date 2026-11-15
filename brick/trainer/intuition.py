"""The imagination neurons learning by themselves (the owner's design: Ni and Ne are the basis of intuition).

Input is whatever brick already has: the sentence pool, the examples, the programs in wa/, and the sentences the
neurons made themselves that passed the check. Nothing is labelled: no meaning groups, no lines, random frequencies.

One round:
  Se takes in sentences, Si recalls similar ones, Ni guesses each next word (a wrong guess teaches it), Ne proposes
  lines between words used in the same places; Ti, Te, Fi and Fe keep what holds. Then Ne makes its own sentences:
  a known sentence with one word swapped for a word it thinks is alike.
  The reality check: brick's compiler reads both. Same role and it still builds -> the line is proved and the new
  sentence becomes practice (so a little data multiplies). A different role -> the line is cut and never proposed
  again (so it can't keep reinforcing its own mistakes). A word brick doesn't know is checked against WordNet.
  A proved pair where brick knows one word and not the other becomes a suggested new brick ("exhibit = show"),
  tried with brick's own word check first.

The network is saved in trainer/data/neurons/, so it keeps learning from where it stopped.

    brick trainer neurons learn [rounds]     learn for that many rounds (default 5, about 10 s each)
    brick trainer neurons learned            what it has learned so far
    brick trainer neurons forget             start again from nothing
"""
import json
import os
import re
import shutil
import tempfile
from collections import Counter

from trainer import neurons

HERE = neurons.HERE
FOLDER = os.environ.get("BRICK_NEURONS_FOLDER") or os.path.join(HERE, "trainer", "data", "neurons")   # tests use their own
# Learning from its own proved sentences: measured on 15 Nov (8 rounds) it did not raise precision (top-100 pairs 69%
# with, 72% without), so by the owner's rule it is off unless BRICK_NEURONS_OWN=on. Public text did (85%), so that's on.
OWN = os.environ.get("BRICK_NEURONS_OWN", "off") == "on"
EXTRA = os.environ.get("BRICK_NEURONS_PUBLIC_DIR") or os.path.join(HERE, "trainer", "data", "sources", "public")   # public text (see public_text())
STATE = os.path.join(FOLDER, "network.json")
IMAGINED = os.path.join(FOLDER, "imagined.txt")          # Ne's own sentences that passed the compiler
CHECKED = os.path.join(FOLDER, "checked.txt")            # every verdict, newest last
BRICKS = os.path.join(FOLDER, "new-bricks.txt")          # suggested new words: "exhibit = show"
WAREHOUSE = os.path.join(HERE, "wa")
CAP = 1500                                               # words it keeps neurons for (the most used ones)
PENDING = os.path.join(FOLDER, "pending.json")          # the last verdicts, applied at the start of the next round
BATCH = int(os.environ.get("BRICK_NEURONS_BATCH", "20"))                                                # sentences Ni and Ne read per training moment
STEPS = int(os.environ.get("BRICK_NEURONS_STEPS", "1200"))                                              # one round: 60 s of simulated time
UNKNOWN = {None, "words"}                                # roles that mean brick doesn't know the word


def lines_of(path):
    try:
        with open(path, encoding="utf-8", errors="replace") as f:
            return [l for l in f.read().splitlines() if l.strip() and not l.lstrip().startswith("#")]
    except OSError:
        return []


NUMBER_WORDS = {"zero": 0, "one": 1, "two": 2, "three": 3, "four": 4, "five": 5, "six": 6, "seven": 7, "eight": 8, "nine": 9,
                "ten": 10, "eleven": 11, "twelve": 12, "twenty": 20, "thirty": 30, "forty": 40, "fifty": 50, "hundred": 100,
                "thousand": 1000}


class Sentence(list):
    """A sentence's tokens, and what brick built from it (out: its bitstream neurons, like '=write')."""
    out = ()


def number_token(value):
    """Number neurons: exact up to 12 and for round tens, else one per size ("#~200" for 150..249)."""
    v = float(value)
    if v == int(v) and (v <= 12 or (v <= 100 and v % 10 == 0) or v in (1000, 10000)):
        return f"#{int(v)}"
    size = 10 ** max(0, len(str(int(v))) - 1)
    return f"#~{int(round(v / size) * size)}"


def tokens(text):
    """Words, with numbers (digits or number words) as number neurons: 'wait two seconds' -> wait #2 seconds."""
    out = []
    for t in re.findall(r"[a-z']+|\d+(?:\.\d+)?", text.lower()):
        if t[0].isdigit():
            out.append(number_token(t))
        elif t in NUMBER_WORDS:
            out.append(number_token(NUMBER_WORDS[t]))
        else:
            out.append(t)
    return out


def untokens(words):
    """Back to text brick can build: #2 -> 2, #~200 -> 200."""
    return " ".join(w.lstrip("#~") if w.startswith("#") else w for w in words)


TRACES = os.path.join(FOLDER, "traces-2.json")      # (2: window routines are traced too)          # what brick builds from each sentence (its output pieces)
_traces = None


def trace(text):
    """The pieces of output brick builds from a line: the names of the compiler's action routines it ran, in order
    ('write hello then wait 2 seconds' -> =write =wait). None when it doesn't build. Cached."""
    global _traces
    if _traces is None:
        try:
            with open(TRACES, encoding="utf-8") as f:
                _traces = json.load(f)
        except (OSError, ValueError):
            _traces = {}
    if text in _traces:
        return _traces[text]
    from compiler import Compiler
    ran = []
    saved = {k: os.environ.get(k) for k in ("BRICK_MEMORY", "BRICK_NEURAL", "BRICK_TRAINER")}
    os.environ.update(BRICK_MEMORY="off", BRICK_NEURAL="off", BRICK_TRAINER="off")
    try:
        with tempfile.TemporaryDirectory() as where:
            c = Compiler(os.path.join(where, "mywords.txt"), ask_user=lambda q: "no")
            for name in [n for n in dir(c) if is_action(n)]:
                real = getattr(c, name)
                def spy(*a, _real=real, _name=name, **k):
                    ran.append(piece_name(_name))
                    return _real(*a, **k)
                setattr(c, name, spy)
            try:
                got = c.compile_text(text)
            except Exception:
                got = None
    finally:
        for k, v in saved.items():
            if v is None:
                os.environ.pop(k, None)
            else:
                os.environ[k] = v
    _traces[text] = list(dict.fromkeys(ran)) if got is not None and ran else None
    return _traces[text]


# brick's action routines: the do_ actions, its window's (bitwin) element and window routines, and the console's
WINDOW_ROUTINES = {"gui_make", "gui_window_settings", "gui_draw", "gui_event", "gui_when", "gui_every", "window_action",
                   "move_to_spot", "set_title", "make_disappear", "nudge", "write_placed", "blank_out"}


def is_action(name):
    return name.startswith("do_") or name in WINDOW_ROUTINES


def piece_name(name):
    return "=" + (name[3:] if name.startswith("do_") else name)


def save_traces():
    if _traces is not None:
        os.makedirs(FOLDER, exist_ok=True)
        with open(TRACES, "w", encoding="utf-8") as f:
            json.dump(_traces, f)


def pieces(texts, traced=False):
    """Sentences as token lists (split at . ; ! ?; order words like 'then' stay in, for the order neurons).
    traced: also what brick builds from each (bitstream neurons)."""
    out = []
    for text in texts:
        for piece in re.split(r"[.;!?]", text):
            ws = tokens(piece)
            if len(ws) >= 2:
                sent = Sentence(ws)
                if traced:
                    sent.out = tuple(trace(piece.strip()) or ())
                out.append(sent)
    return out


SY = os.path.expanduser("~/.local/share/symbolizer")
READING = os.path.join(SY, "brick", "reading.list")     # every file you save in Symbolizer (it writes this list)
SETTINGS = os.path.join(SY, "brick", "settings.json")    # {"diary": "on"/"off"}: Symbolizer's brick switches
WRITING_CAP = 3_000_000                                  # characters of your writing per round (newest first)


def diary_on():
    try:
        with open(SETTINGS, encoding="utf-8") as f:
            return json.load(f).get("diary") == "on"
    except (OSError, ValueError):
        return False


def your_writing():
    """Everything you write in Symbolizer, as lines: every file you saved (code and prose alike), your sessions,
    and your diary only when its switch is on. Newest first, up to WRITING_CAP characters. Never leaves your PC."""
    paths = []
    for line in lines_of(READING):
        p = line.strip()
        if p and os.path.isfile(p) and "/diary/" not in p:
            paths.append(p)
    for folder in [os.path.join(SY, "sessions")] + ([os.path.join(SY, "diary")] if diary_on() else []):
        for root, _, names in os.walk(folder):
            paths += [os.path.join(root, n) for n in names if n.endswith(".txt")]
    paths = sorted(set(paths), key=lambda p: -os.path.getmtime(p))
    texts, size = [], 0
    for p in paths:
        try:
            if os.path.getsize(p) > 1_000_000:
                continue
            with open(p, encoding="utf-8") as f:
                text = f.read()
        except (OSError, UnicodeDecodeError):
            continue                                       # not text
        text = re.sub(r"([a-z])([A-Z])", r"\1 \2", text).replace("_", " ")   # code: drawTile, max_size -> words
        texts += [l for l in text.splitlines() if len(l.split()) >= 2]
        size += len(text)
        if size > WRITING_CAP:
            break
    return texts


def public_text():
    """Public sentences in trainer/data/sources/public/*.txt (one per line), e.g. Tatoeba's English sentences."""
    texts = []
    if os.path.isdir(EXTRA) and os.environ.get("BRICK_NEURONS_PUBLIC", "on") != "off":
        for name in sorted(os.listdir(EXTRA)):
            if name.endswith(".txt"):
                texts += lines_of(os.path.join(EXTRA, name))
    return texts


def raw_input():
    """Every direction brick has, as word lists: the pool, your lines from real builds, the examples, wa/'s programs,
    Ne's proved sentences (only when BRICK_NEURONS_OWN=on), and
    public text. brick's own words get neurons first; public text fills the rest up to CAP and adds practice."""
    heard = os.path.join(HERE, "trainer", "data", "heard.txt")     # your own lines from real builds (trainer/core.py)
    texts = lines_of(neurons.POOL) + lines_of(heard) + (lines_of(IMAGINED) if OWN else [])
    mine_written = your_writing()                           # your own writing counts as brick's own input
    for folder in (neurons.EXAMPLES, WAREHOUSE):
        if os.path.isdir(folder):
            for name in sorted(os.listdir(folder)):
                if name.endswith(".txt"):
                    texts += lines_of(os.path.join(folder, name))
    own, public = pieces(texts, traced=True) + pieces(mine_written), pieces(public_text())
    save_traces()
    mine, theirs = Counter(w for d in own for w in d), Counter(w for d in public for w in d)
    keep = [w for w, n in mine.most_common() if n >= 2][:CAP]
    keep += [w for w, n in theirs.most_common() if n >= 3 and w not in mine][:max(0, CAP - len(keep))]
    keep = set(keep)
    if os.environ.get("BRICK_NEURONS_DEFINITIONS", "off") == "on":   # rare words: their dictionary definitions too
        everyone = mine + theirs
        for w in [w for w in keep if everyone[w] < 6 and w.isalpha()]:
            for d in pieces(gloss_text(w)):
                public.append(d)
    out = []
    for d in own + public:
        kept = Sentence(w for w in d if w in keep)
        kept.out = d.out
        if len(kept) >= 2:
            out.append(kept)
    return out, keep


_encoded = {}


def encoded(vocab):
    """Starting frequencies from the encoder (trainer/encoder.py: a stand-in dictionary encoder until the owner's
    encoder engine exists; BRICK_ENCODER=none turns it off). Worked out once per vocabulary."""
    from trainer import encoder
    key = (os.environ.get("BRICK_ENCODER", encoder.DEFAULT), len(vocab), hash(frozenset(vocab)))
    if key not in _encoded:
        _encoded.clear()
        _encoded[key] = encoder.frequencies(sorted(vocab), max(4, len(vocab) / 25))
    return _encoded[key]


def gloss_text(word):
    """WordNet's first definitions of a word, as sentences the neurons can read ("flicker means ...")."""
    from trainer import wordnet
    out = []
    for pos in "nva":
        try:
            offsets = wordnet.senses(word, pos)[:1]
        except OSError:
            return out
        for offset in offsets:
            with open(os.path.join(wordnet.WORDNET, "data." + wordnet.FILES[pos])) as data:
                data.seek(int(offset))
                text = data.readline().split(" | ", 1)[-1].split(";")[0].split('"')[0].strip()
            if text:
                out.append(f"{word} means {text}")
    return out


def data_text(sentences):
    """The engine's input: one sentence a line, with what brick built after '||'."""
    return "\n".join(" ".join(d) + (" || " + " ".join(d.out) if d.out else "") for d in sentences)


_roles = {}


def roles_in(line):
    """[(word, role)] as brick's reader reads the line (do, thing, place, colour, time, count, glue, words ...)."""
    if line not in _roles:
        import bricks
        try:
            _roles[line] = [(b["text"].lower(), b["kind"]) for d in bricks.read_line(line) for b in d]
        except Exception:
            _roles[line] = []
    return _roles[line]


def role_of(word):
    found = roles_in(word)
    return found[0][1] if len(found) == 1 else None


def builds(text):
    """brick's compiler builds the line on its own -> the program's bytes, else None."""
    import words
    saved = {k: os.environ.get(k) for k in ("BRICK_MEMORY", "BRICK_NEURAL", "BRICK_TRAINER")}
    os.environ.update(BRICK_MEMORY="off", BRICK_NEURAL="off", BRICK_TRAINER="off")
    try:
        with tempfile.TemporaryDirectory() as where:
            return words.build(text, os.path.join(where, "mywords.txt"))
    finally:
        for k, v in saved.items():
            if v is None:
                os.environ.pop(k, None)
            else:
                os.environ[k] = v


def same_meaning(a, b):
    from trainer import wordnet
    try:
        return b in wordnet.synonyms(a) or a in wordnet.synonyms(b)
    except OSError:
        return False


CONTENT = {"do", "thing", "colour", "place", "time", "count"}   # roles a new brick can stand in for


def check(c):
    """The reality check for one of Ne's sentences -> (ok True/False/None, why, level, builds).
    level: "meaning" (WordNet agrees, or the program builds exactly the same) or "role" (same job, maybe another
    meaning: left and right). builds: the swapped sentence builds on its own (only those become practice)."""
    return _check(c)


def _check(c):
    c = dict(c, original=untokens(c["original"].split()), swapped=untokens(c["swapped"].split()))
    a, b = c["a"], c["b"]
    ra = dict(roles_in(c["original"])).get(a)
    rb = dict(roles_in(c["swapped"])).get(b)
    if ra in UNKNOWN or rb in UNKNOWN:                    # brick can't read one of them: ask WordNet
        if same_meaning(a, b):
            return True, "WordNet: same meaning", "meaning", builds(c["swapped"]) is not None
        return None, "brick doesn't know one of them, and WordNet doesn't say", None, False
    if ra != rb:
        return False, f"{a} is {ra}, {b} is {rb}", None, False
    if same_meaning(a, b):
        return True, "WordNet: same meaning", "meaning", builds(c["swapped"]) is not None
    before = builds(c["original"])
    if before is None:
        return True, f"both {ra} in brick's reader", "role", False
    after = builds(c["swapped"])
    if after is None:
        return False, f"'{c['swapped']}' doesn't build", None, False
    if after == before:
        return True, "builds exactly the same", "meaning", True
    return True, f"both {ra}, and it still builds", "role", True


def suggest_brick(a, b):
    """A pair proved to mean the same, where brick knows one word (as an action, thing, colour, place, time or count)
    and not the other -> 'unknown = known', if brick's own word check agrees."""
    import words
    ra, rb = role_of(a), role_of(b)
    if (ra in UNKNOWN) == (rb in UNKNOWN):
        return None
    new, known = (a, b) if ra in UNKNOWN else (b, a)
    if (ra if known == a else rb) not in CONTENT or new in neurons.SMALL or len(new) < 3:
        return None
    if not words.works_like(new, known, before=True):
        return None
    return f"{new} = {known}"


def append(path, lines):
    if lines:
        os.makedirs(FOLDER, exist_ok=True)
        with open(path, "a", encoding="utf-8") as f:
            f.write("".join(l + "\n" for l in lines))


_pool = None


def pmap(fn, items):
    """fn over items side by side in worker processes (brick's compiler checks don't depend on each other), in the
    same order, so the results are the same as one after another. BRICK_PARALLEL=off runs them one by one."""
    items = list(items)
    if len(items) < 3 or os.environ.get("BRICK_PARALLEL", "on") == "off":
        return [fn(x) for x in items]
    global _pool
    try:
        if _pool is None:
            import concurrent.futures
            import multiprocessing
            _pool = concurrent.futures.ProcessPoolExecutor(max_workers=min(12, os.cpu_count() or 4),
                                                           mp_context=multiprocessing.get_context("forkserver"))
        return list(_pool.map(fn, items))
    except Exception:                                      # no workers here: one by one
        _pool = None
        return [fn(x) for x in items]


class Lock:
    """One learner at a time (the daemon, brick gui's trainer, a command): the saved network has one writer."""

    def __init__(self, wait=True):
        self.wait = wait

    def __enter__(self):
        import fcntl
        os.makedirs(FOLDER, exist_ok=True)
        self.file = open(os.path.join(FOLDER, "learn.lock"), "w")
        try:
            fcntl.flock(self.file, fcntl.LOCK_EX | (0 if self.wait else fcntl.LOCK_NB))
        except BlockingIOError:
            self.file.close()
            return False
        return True

    def __exit__(self, *problem):
        import fcntl
        if not self.file.closed:
            fcntl.flock(self.file, fcntl.LOCK_UN)
            self.file.close()


def learn(rounds=5, steps=STEPS, brain=None, say=print, wait=True):
    """Learn for some rounds, checking Ne's own sentences with brick's compiler. -> the last round's numbers.
    wait=False: if another learner is busy, do nothing (-> None)."""
    with Lock(wait) as got:
        if not got:
            return None
        return _learn(rounds, steps, brain, say)


def _learn(rounds, steps, brain, say):
    if not shutil.which("node"):
        say("The imagination neurons run with node, and node isn't installed here.")
        return None
    os.makedirs(FOLDER, exist_ok=True)
    out = None
    try:
        with open(PENDING, encoding="utf-8") as f:
            verdicts = json.load(f)
    except (OSError, ValueError):
        verdicts = []
    for k in range(1, rounds + 1):
        sentences, vocab = raw_input()
        roles = {w: r for w in vocab if (r := role_of(w)) not in UNKNOWN}
        out = neurons.run_node({"task": "learn", "state": STATE, "data": data_text(sentences),
                                "roles": roles, "verdicts": verdicts, "steps": steps, "candidates": 24, "batch": BATCH, "omegas": encoded(vocab), "seed": k + 100 * int(os.environ.get("BRICK_NEURONS_SEED", "0")), "brain": brain})
        if out.get("error"):
            say(out["error"])
            return None
        verdicts, proved, cut, new, bricks = [], 0, 0, [], []
        have = set(lines_of(IMAGINED))
        known_bricks = set(lines_of(BRICKS))
        checked = pmap(check, out["candidates"])          # the compiler checks, side by side
        for c, (ok, why, level, built) in zip(out["candidates"], checked):
            verdicts.append({"a": c["a"], "b": c["b"], "ok": ok, "why": why, "level": level})
            if ok:
                proved += 1
                if built and c["swapped"] not in have:          # only sentences brick can build become practice
                    have.add(c["swapped"]); new.append(c["swapped"])
            elif ok is False:
                cut += 1
            brick = suggest_brick(c["a"], c["b"]) if level == "meaning" else None
            if brick and brick not in known_bricks:
                known_bricks.add(brick); bricks.append(brick)
        append(IMAGINED, new)
        append(BRICKS, bricks)
        append(CHECKED, [f"{'yes' if v['ok'] else 'no' if v['ok'] is False else '?'}\t{v['a']}\t{v['b']}\t{v['why']}" for v in verdicts])
        say(f"round {k}: {out['neurons']} neurons ({out.get('phrases', 0)} phrases), {out['settled']} lines ({out['proved']} proved), "
            f"Ni guesses {round(out['ni'] * 100)}% of next words and {round(out.get('bits', 0) * 100)}% of brick's output pieces; "
            f"checked {len(verdicts)} of Ne's sentences: {proved} kept, {cut} cut"
            + (f"; {len(new)} new practice sentences" if OWN else "") + (f"; suggested: {', '.join(bricks)}" if bricks else ""))
        with open(PENDING, "w", encoding="utf-8") as f:     # the network takes these in at the next round
            json.dump(verdicts, f)
    return out


def score(state=STATE):
    """Precision of what it learned, against brick's own word groups (used only for scoring, never for learning):
    p100 = of the 100 word pairs it finds most alike, the share that really are in one group; chance = the same for
    random pairs; placed = words whose proved/settled lines mostly lead into their own group."""
    groups = neurons.all_groups()
    return neurons.run_node({"task": "score", "state": state, "groups": groups})


EVERY = 600                                              # the trainer runs one round at most every 10 minutes


def background_round():
    """One quiet learning round for the trainer, at most every EVERY seconds (BRICK_NEURONS=off stops it).
    -> a short line about it, or None."""
    import time
    if os.environ.get("BRICK_NEURONS", "on") == "off" or not shutil.which("node"):
        return None
    stamp = os.path.join(FOLDER, "last-round")
    try:
        if time.time() - os.path.getmtime(stamp) < EVERY:
            return None
    except OSError:
        pass
    os.makedirs(FOLDER, exist_ok=True)
    with open(stamp, "w") as f:
        f.write(str(time.time()))
    said = []
    try:
        learn(1, say=said.append, wait=False)               # the daemon may be learning: then skip
    except Exception as error:                            # never stop the trainer over this
        return f"stopped: {error}"
    return said[-1] if said else None


def brick_knows(word):
    from trainer import universe
    return word in universe.universe().words


def evidence(state=STATE):
    """{word: [(other word, level)]} from the saved network: its proved lines (level "meaning" or "role") and the
    lines it settled by itself (level "lines")."""
    try:
        with open(STATE if state is None else state, encoding="utf-8") as f:
            saved = json.load(f)
    except (OSError, ValueError):
        return {}
    word = {n["label"]: n["word"] for n in saved["neurons"]}
    out = {}
    rank = {"meaning": 0, "role": 1, "lines": 2}
    for a, b, w, w0, tent, *flags in saved["links"]:
        if tent is not None:
            continue
        level = flags[1] if len(flags) > 1 and flags[1] else "lines"
        level = "role" if level == 1 else level
        wa, wb = word.get(a), word.get(b)
        if wa and wb and wa != wb:
            out.setdefault(wa, []).append((wb, level, w))
            out.setdefault(wb, []).append((wa, level, w))
    for k in out:
        out[k].sort(key=lambda x: (rank.get(x[1], 3), -x[2]))
    return out


INFLECTIONS = ("ing", "ed", "es", "s", "d")                # same meaning, another form: writes, writing, painted
PREFIXES = ("re", "un", "pre", "over", "out")             # word parts that change the meaning a little


def word_parts(word):
    """A word brick doesn't know, from the parts of it that brick does: [(root, level)].
    Inflections keep the meaning ("meaning"); prefixes change it ("role": the same job, not proved the same)."""
    out = []
    for end in INFLECTIONS:
        if word.endswith(end) and len(word) - len(end) >= 3:
            stem = word[: -len(end)]
            for root in (stem, stem + "e", stem[:-1] if len(stem) > 3 and stem[-1] == stem[-2] else None):
                if root and brick_knows(root):
                    out.append((root, "meaning"))
    for start in PREFIXES:
        if word.startswith(start) and len(word) - len(start) >= 3 and brick_knows(word[len(start):]):
            out.append((word[len(start):], "role"))
    return list(dict.fromkeys(out))


def gloss(word):
    """WordNet's definitions of the word (verbs first): its words that brick knows, in order -> [str]."""
    from trainer import wordnet
    found = []
    for pos in "vna":
        try:
            offsets = wordnet.senses(word, pos)[:2]
        except OSError:
            return []
        for offset in offsets:
            with open(os.path.join(wordnet.WORDNET, "data." + wordnet.FILES[pos])) as data:
                data.seek(int(offset))
                text = data.readline().split(" | ", 1)[-1].split(";")[0].split('"')[0]
            found += [w for w in neurons.words_of(text) if w not in neurons.SMALL and w != word and brick_knows(w)]
    return list(dict.fromkeys(found))


def bitstream(line):
    """English -> the program's bits, judged by the brain itself.
    brick reads the line itself first; if it builds, those bits stand (the brain never overrides brick).
    Otherwise every word brick doesn't know is swapped for words the brain proved to mean the same (then WordNet's),
    every rewrite is built, and the brain approves only when it is precise: every swap is proved to mean the same,
    and every rewrite that builds gives the very same bits. Two different programs -> it isn't sure, and says so.
    -> {"bits", "line", "by", "precise", "why", "tried"}"""
    from itertools import product
    from trainer import wordnet
    first = builds(line)
    if first is not None:
        return {"bits": first, "line": line, "by": "brick", "precise": True, "why": "brick reads it by itself", "tried": []}
    words = neurons.words_of(line)
    unknown = [w for w in dict.fromkeys(words) if not brick_knows(w) and w not in neurons.SMALL]
    if not unknown:
        return {"bits": None, "line": line, "by": None, "precise": False, "why": "brick knows every word but can't build the line", "tried": []}
    ev = evidence()
    options = {}
    for w in unknown:
        mine = [(o, lvl) for o, lvl, _ in ev.get(w, []) if brick_knows(o)]
        try:
            net = [(o, "meaning") for o in wordnet.synonyms(w) if " " not in o and brick_knows(o)]
        except OSError:
            net = []
        parts = word_parts(w)                             # writes -> write; redraw -> draw
        defined = [(o, "role") for o in gloss(w)[:2]] if not (mine or net or parts) else []   # only in a dictionary definition
        options[w] = list(dict.fromkeys(parts + mine + net + defined))[:3] + [(w, "kept")]   # kept: it may be words to show, like hello
    tried = []
    for pick in list(product(*[options[w] for w in unknown]))[:27]:
        swap = dict(zip(unknown, pick))
        text = " ".join(swap[w][0] if w in swap else w for w in words)
        bits = builds(text)
        tried.append({"line": text, "bits": bits, "levels": [p[1] for p in pick]})
    good = [t for t in tried if t["bits"] is not None]
    if not good:
        return {"bits": None, "line": None, "by": None, "precise": False, "why": "no rewrite builds", "tried": tried}
    proved = [t for t in good if all(l in ("meaning", "kept") for l in t["levels"])]
    pool = proved or good
    same = len({t["bits"] for t in pool}) == 1
    best = pool[0]
    precise = bool(proved) and same
    why = ("every swap is proved to mean the same, and every rewrite that builds gives the same bits" if precise else
           "the rewrites that build give different programs" if not same else "a swap is only proved to do the same job, not to mean the same")
    return {"bits": best["bits"], "line": best["line"], "by": "brain", "precise": precise, "why": why, "tried": tried}


MADE = os.path.join(FOLDER, "made-bricks.txt")           # bricks it made and kept, with the evidence


def brain_role(word, ev):
    """The role the neurons learned for a word brick doesn't know: what its settled lines' known words mostly are."""
    vote = Counter()
    for other, level, w in ev.get(word, []):
        r = role_of(other)
        if r not in UNKNOWN:
            vote[r] += w * (2 if level == "meaning" else 1)
    return vote.most_common(1)[0][0] if vote else None


def neurons_read(pairs):
    """The neurons' own verdict (16 energy percentages) on each new word in a real sentence it came from.
    pairs: [(new, sentence words)] -> {new: verdict}"""
    if not pairs or not os.path.isfile(STATE):
        return {}
    sentences, _ = raw_input()
    questions = "\n".join(f"{w} @ {' '.join(x for x in ctx if x != w)}" for w, ctx in pairs)
    out = neurons.run_node({"state": STATE, "data": data_text(sentences), "questions": questions, "steps": 200})
    return {a["word"]: (a.get("verdict") if a.get("found") else "no neuron") for a in out.get("answers", [])}


def brain_bricks(words):
    """{word: {"output", "share", "second"}}: the piece of brick's output the brain predicts each word makes."""
    if not words or not os.path.isfile(STATE):
        return {}
    return neurons.run_node({"task": "bricks", "state": STATE, "words": sorted(words)}) or {}


def make_bricks(limit=12, say=print, dry=False, only=None):
    """New bricks from the neurons' own findings, kept only when they work precisely:
      1. the neurons found it: Ne proposed the pair and it was kept, or a proved line, or the role they learned
         for the new word is the known word's role;
      2. the brain's own judgement: its bitstream neurons predict the same output for the new word as for the brick
         (or, for a word with no output learned yet, its 16 energy percentages call it "accurate" in a real sentence);
      3. general English agrees: each word is a synonym of the other both ways, in WordNet (commonest senses) or in
         the internet dictionary (trainer/wordfeed.py);
      4. the exact bits: a real sentence with the new word builds exactly the same program as with the known word;
      5. only one meaning: if two known words would give different programs, it isn't sure and makes nothing;
      6. nothing breaks: every test and example that passed before still passes (else they're all taken out again).
    -> the lines added."""
    from trainer import core, wordnet
    import dictionary
    ev = evidence()
    if not ev:
        say("Nothing learned yet, so nothing to make bricks from. Run: brick trainer neurons learn")
        return []
    sentences, _ = raw_input()
    used = Counter(w for d in sentences for w in set(d))
    made = {l.split("\t")[0] for l in lines_of(MADE)}
    suggested = [l.split(" = ") for l in lines_of(BRICKS) if " = " in l]
    found = [n for n, k in suggested] + [w for w in ev if not brick_knows(w)]
    seen, candidates = set(), []
    if only is not None:
        found = [w for w in found if w in only] + [w for w in only if w not in found]
    from trainer import wordfeed
    for new in found:
        if new in seen or new[:1] in "#=@" or " " in new or new in made or brick_knows(new) or new in core.STARTERS or new in core.UNWANTED or new in neurons.SMALL \
                or len(new) < 3 or used[new] < 2 or dictionary.by_first_word().get(new):
            continue
        seen.add(new)
        role = brain_role(new, ev)
        mine = {o: lvl for o, lvl, _ in ev.get(new, [])}
        try:
            pool = set(wordnet.synonyms(new)) | (wordfeed.synonyms(new) if os.path.isfile(os.path.join(wordfeed.CACHE, new + ".json")) else set())
            mates = [k for k in pool if " " not in k and brick_knows(k) and wordfeed.agrees(new, k)]
        except OSError:
            mates = []
        mates = [k for k in mates if role_of(k) in CONTENT and (mine.get(k) in ("meaning", "role", "lines") or role == role_of(k))]
        if mates:
            candidates.append((new, mates, role))
    if not candidates:
        say("No new brick passed the neurons' and WordNet's checks this time.")
        return []
    context = {}
    for new, _, _ in candidates:
        host = next((d for d in sentences if new in d and len(d) >= 3), None) or next(d for d in sentences if new in d)
        context[new] = host
    # the brain judges first: its bitstream neurons must predict the same piece of output for the new word as for the
    # brick it would stand for; a word with no output learned yet is judged by its 16 energies in a real sentence
    predicted = brain_bricks({w for n, mates, _ in candidates for w in [n] + mates})
    verdict = neurons_read([(n, context[n] + mates[:1]) for n, mates, _ in candidates
                            if not (predicted.get(n) or {}).get("output")])
    builder, keep = core.Builder(), []
    for new, mates, role in candidates:
        mine = predicted.get(new) or {}
        if mine.get("output"):
            agree = [k for k in mates if (predicted.get(k) or {}).get("output") == mine["output"]]
            if not agree or mine["share"] < 0.4:
                say(f"  not sure about '{new}': its bitstream neurons say {mine['output']} ({round(mine['share'] * 100)}%), "
                    f"not what {' or '.join(mates)} make")
                continue
            mates = agree
        elif verdict.get(new) != "accurate":
            say(f"  not sure about '{new}': the neurons' 16 energies say {verdict.get(new, 'nothing')}")
            continue
        texts = [" ".join(d) for d in sentences if new in d][:8]
        results = []
        for known in mates:
            line, path = core.link_for(new, known)
            if not line:
                continue
            same = 0
            for text in texts:
                swapped = builder.build(text.replace(new, known))
                if not isinstance(swapped, bytes):
                    continue
                with builder.trying(path, line) as trial:
                    if trial.build(text) == swapped:
                        same += 1
                    else:
                        same = -99
            if same <= 0:                                  # no real sentence built, or one built differently
                probe = __import__("words").works_like(new, known, before=True)
                if probe and same == 0:
                    swapped = builder.build(probe.replace(new, known))
                    with builder.trying(path, line) as trial:
                        same = 1 if isinstance(swapped, bytes) and trial.build(probe) == swapped else 0
            if same > 0:
                with builder.trying(path, line) as trial:
                    results.append((line, path, known, trial.build(texts[0] if texts else new)))
        if not results:
            say(f"  not kept '{new}': it didn't build exactly the same as {' or '.join(mates)}")
            continue
        if len({r[3] if isinstance(r[3], bytes) else r[2] for r in results}) > 1:
            say(f"  not sure about '{new}': it could mean {' or '.join(r[2] for r in results)}, and they build different programs")
            continue
        keep.append((new, results[0], role))
        if len(keep) >= limit:
            break
    if not keep or dry:
        for new, (line, _, _, _), role in keep:
            say(f"  would make: {line}")
        return [] if not keep else [line for _, (line, _, _, _), _ in keep]
    before = core.broken(builder)                          # what already failed, so only new failures count
    for _, (line, path, _, _), _ in keep:
        core.add_line(path, line)
    after = core.broken(builder) - before
    if after:                                              # something broke: take them all out, keep none
        for _, (line, path, _, _), _ in keep:
            core.remove_line(path, line)
        say(f"Took them out again: {', '.join(sorted(os.path.basename(b) for b in after))} stopped passing.")
        return []
    append(MADE, [f"{line}\tneurons: role {role or '-'}, 16 energies accurate; WordNet both ways; builds exactly the same; tests pass"
                  for _, (line, _, _, _), role in keep])
    for _, (line, _, _, _), _ in keep:
        say(f"  new brick: {line}")
    return [line for _, (line, _, _, _), _ in keep]


def learned():
    """What it has learned so far, in plain numbers and examples."""
    if not os.path.isfile(STATE):
        return None
    with open(STATE, encoding="utf-8") as f:
        state = json.load(f)
    label = {n["label"]: n for n in state["neurons"]}
    proved = [(a, b, flags[1]) for a, b, w, w0, tent, *flags in state["links"] if len(flags) > 1 and flags[1]]
    checked = [l.split("\t") for l in lines_of(CHECKED)]
    return {"words": len(state["neurons"]), "lines": len(state["links"]), "proved": proved,
            "ni": state.get("stats", {}).get("ni", 0), "seconds": round(state.get("t", 0)),
            "split": sorted({n["word"] for n in state["neurons"] if "·" in n["label"]}),
            "yes": sum(1 for c in checked if c[0] == "yes"), "no": sum(1 for c in checked if c[0] == "no"),
            "imagined": len(lines_of(IMAGINED)), "bricks": lines_of(BRICKS),
            "roles": sum(1 for n in label.values() if n.get("role"))}


def command(args):
    what = args[0] if args else "learned"
    if what == "learn":
        rounds = int(args[1]) if len(args) > 1 and args[1].isdigit() else 5
        print(f"Learning by itself for {rounds} rounds from brick's own input, checked by brick's compiler...", flush=True)
        return 0 if learn(rounds, say=lambda s: print(s, flush=True)) else 1
    if what == "build":
        line = " ".join(args[1:]).strip()
        got = bitstream(line)
        if got["bits"] is None:
            print(f"No bits: {got['why']}.")
        else:
            print(f"{len(got['bits'])} bytes, built from \"{got['line']}\" ({'brick read it' if got['by'] == 'brick' else 'the brain rewrote it'}).")
            print(("Precise" if got["precise"] else "Not sure") + f": {got['why']}.")
        for t in got["tried"][:6]:
            print(f"  tried \"{t['line']}\": {'builds' if t['bits'] is not None else 'does not build'} ({', '.join(str(l) for l in t['levels'])})")
        return 0 if got["bits"] is not None else 1
    if what == "bricks":
        print("Making new bricks from what the neurons found (each one is checked; this takes a few minutes)...", flush=True)
        made = make_bricks(say=lambda x: print(x, flush=True))
        print(f"Kept {len(made)} new bricks." if made else "Kept no new bricks this time.")
        return 0
    if what == "score":
        got = score()
        print(f"precision: {got['p20']}% of its top 20 alike pairs and {got['p100']}% of its top 100 really share a group "
              f"(random pairs: {got['chance']}%); {got['placed']}% of {got['gold']} words placed in their own group "
              f"({got.get('placedAlike')}% by alike lines only); "
              f"Ni guesses {round(got['ni'] * 100)}%.\nTop pairs: {', '.join(got['top'])}")
        return 0
    if what == "forget":
        for path in (STATE, IMAGINED, CHECKED, BRICKS, PENDING):
            if os.path.isfile(path):
                os.remove(path)
        print("Forgotten: the neurons start again from nothing next time.")
        return 0
    got = learned()
    if not got:
        print("Nothing learned yet. Start it with: brick trainer neurons learn")
        return 0
    print(f"{got['words']} words, {got['lines']} lines after {got['seconds']} s of learning. Ni guesses {round(got['ni'] * 100)}% of next words.")
    print(f"The compiler checked Ne's sentences: {got['yes']} kept, {got['no']} cut. {got['imagined']} of its own sentences became practice.")
    if got["proved"]:
        same = [f"{a}~{b}" for a, b, level in got["proved"] if level == "meaning"]
        role = [f"{a}~{b}" for a, b, level in got["proved"] if level != "meaning"]
        if same:
            print("Proved to mean the same:", ", ".join(same[:20]))
        if role:
            print("Proved to do the same job:", ", ".join(role[:20]))
    if got["split"]:
        print("Words it split into separate meanings:", ", ".join(got["split"]))
    if got["bricks"]:
        print("Suggested new bricks (teach one with: brick teach NEW means KNOWN):", ", ".join(got["bricks"]))
    return 0
