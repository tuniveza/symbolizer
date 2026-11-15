"""The teacher: the language model teaches brick's own brain, so the brain reads the next line like it by itself.

The model is the teacher, not the reader. Every line you write in Symbolizer's brick mode is read by the model in
the background (neural.readings, remembered in trainer/data/neural.json). The teacher goes through those readings
(the daemon calls it between its rounds, with the model switched off) and, for each line brick's own brain doesn't
already build the same way:

  1. builds the model's reading by brick alone: no program, no lesson (the model made something up);
  2. checks the reading keeps the line's meaning (changes_meaning): building isn't proof, the model drops and swaps
     words, so every place, colour, number, count, time and window word must still be there, and none added;
  3. matches the line's pieces (brick's split) to the reading's directions (align), and for each piece teaches the
     brain the reading that makes the same program (Trainer.steer: the memory and the imagination neurons), looks
     up words brick doesn't know (Trainer.learn_toward: WordNet, ConceptNet), or else learns a dictionary pattern
     with gaps (pattern), kept only when the piece then builds exactly the model's program and no line brick
     already reads builds differently; check() (brick's tests and examples) runs once a batch, only when
     something was added, and takes back anything that broke one;
  4. builds the line again by brick alone: when it now makes the model's program, the brain has learned it.

The model is never asked here (BRICK_NEURAL is off while teaching): only its cached readings are used.

    teach(most=5, say=print) -> {"looked": n, "learned": [lines], "already": n, "not yet": [lines],
                                 "pieces": pieces of long lines learned, "refused": readings refused}
"""
import json
import os
import re
import time

from trainer import neural

FOLDER = os.path.join(os.path.dirname(os.path.dirname(os.path.abspath(__file__))), "trainer", "data")
TAUGHT = os.path.join(FOLDER, "taught.json")
_trainer = None


def _load():
    try:
        with open(TAUGHT, encoding="utf-8") as f:
            return json.load(f)
    except (OSError, ValueError):
        return {}


def _save(done):
    os.makedirs(FOLDER, exist_ok=True)
    with open(TAUGHT + ".new", "w", encoding="utf-8") as f:
        json.dump(done, f)
    os.replace(TAUGHT + ".new", TAUGHT)


def lessons():
    """(line, the model's best reading) for every line the model read, newest last; each line once (its newest
    reading: the same line read with different lines before it is one lesson)."""
    out = {}
    for key, readings in list(neural.cache().items()):
        if key.startswith(("choose|", "invent|")) or not readings or not isinstance(readings, list):
            continue
        parts = key.split("|", 3)
        if len(parts) != 4:
            continue
        line, best = parts[3].strip(), readings[0]
        if line and isinstance(best, str) and best.strip().lower() != line.lower():
            out.pop(line, None)
            out[line] = best.strip()
    lines = sorted(out)                                  # 'write yellow on con' while typing 'console': not a lesson
    typing = {a for a, b in zip(lines, lines[1:]) if b.startswith(a) and b[len(a)].isalnum() and a[-1].isalnum()}
    return [(line, best) for line, best in out.items() if line not in typing]


FUNCTION_WORDS = {"a", "an", "the", "in", "on", "at", "to", "of", "it", "and", "with", "for", "is", "be"}
STYLES = {"bold", "italic", "italics", "underlined", "big", "small"}


def colour_words():
    """Every word or name brick reads as a colour: the dictionary's colours, actions.COLOURS, and the words the
    universe files under colour."""
    import dictionary
    from actions import COLOURS
    found = set(dictionary.colour_table()) | set(COLOURS)
    found |= {e.key.lower() for e in dictionary.of_kind("colour")}
    try:
        from trainer import universe
        found |= {w for w, ideas in universe.universe().words.items() if "colour" in ideas}
    except Exception:                                    # the universe should never stop a lesson
        pass
    return found


def shown_words(meant, colours):
    """The words brick's own form of the meaning writes: 'after 2 seconds write hi there in red at the top 3 times'
    -> 'hi there'. Only real colours, places, counts and styles come off the end ('write hi in mocha' shows
    'hi in mocha'). '' when it doesn't write."""
    got = re.match(r"(?:after \S+ seconds? )?(?:write|make it say) (.+)$", meant)
    if not got:
        return ""
    words = got.group(1).split()
    while words:
        tail = " ".join(words[-3:])
        for n in (3, 2, 1):                              # ... in COLOUR (one to three words)
            if len(words) > n and words[-n - 1] == "in" and " ".join(words[-n:]) in colours:
                words = words[:-n - 1]
                break
        else:
            if re.search(r" (?:at|on|to|in) the \S+$", " " + tail) and len(words) >= 3:
                words = words[:-3]                       # at the top, on the left
            elif len(words) > 2 and words[-1] == "times" and words[-2].isdigit():
                words = words[:-2]                       # 3 times
            elif len(words) > 2 and words[-2] == "in" and words[-1] in STYLES:
                words = words[:-2]                       # in bold
            else:
                break
    return " ".join(words)


KEPT = {"place", "colour", "number", "count", "time unit", "window", "background"}
SAME = {"centre": "middle", "center": "middle", "once": "1", "twice": "2", "thrice": "3", "one": "1", "two": "2",
        "three": "3", "four": "4", "five": "5", "six": "6", "seven": "7", "eight": "8", "nine": "9", "ten": "10"}


def key_words(text, every=False):
    """The words that carry the meaning, by brick's own ideas: [(ideas, {the word, its spelled-right forms, the
    number of a count like 'x5'})] for every place, colour, number, count, time unit, window and background
    (every=True: for every word, a slip like 'tal' for 'teal' too)."""
    from trainer import universe
    out = []
    for piece in universe.analyse(text):
        for tok in piece:
            ideas = set(tok.ideas) & KEPT
            if not (ideas or every) or tok.quoted or (tok.word or "").lower() in ("side", "sides"):
                continue
            forms = {tok.text.lower(), (tok.word or "").lower()} | {x.lower() for x in tok.spelled or []}
            forms |= {re.sub(r"\D", "", x) for x in forms if re.fullmatch(r"x?\d+x?", x)}
            out.append((ideas, {SAME.get(x, x) for x in forms} - {""}))
    return out


def same_colour(a, b):
    import dictionary
    table = dictionary.colour_table()
    codes = {table[x][0] for x in a if x in table}
    return bool(codes & {table[x][0] for x in b if x in table})


def changes_meaning(messy, meant):
    """Does the model's reading change what the line says? Building isn't proof: the model drops or swaps words.
    Every place, colour, number, count, time and window or background word of the line must still be in the
    reading (the same word, its spelled-right form, or a colour of the same shade), and the reading mustn't add a
    place, colour or number the line didn't have. -> why not ('corner' left out), or None when it keeps it."""
    have, said = key_words(messy), key_words(meant)
    have_forms = set().union(*(f for _, f in key_words(messy, every=True)))
    said_forms = set().union(*(f for _, f in said)) if said else set()
    for ideas, forms in have:
        if ideas & {"place", "colour", "number", "count"}:
            if not forms & said_forms and not ("colour" in ideas and same_colour(forms, said_forms)):
                return f"'{min(forms, key=len)}' left out"
        elif not any(ideas & other for other, _ in said):
            return f"'{min(forms, key=len)}' left out"
    for ideas, forms in said:
        if ideas & {"place", "colour", "number", "count"} and not forms & have_forms and \
                not ("colour" in ideas and same_colour(forms, have_forms)) and \
                not (ideas & {"place", "colour"} and any(slip(x, y) for x in forms for y in have_forms)):
            return f"'{min(forms, key=len)}' made up"
    return None


def slip(right, typed):
    """'tal' typed for 'teal', 'lefft' for 'left': the same first letter and nearly the same letters."""
    import difflib
    return len(typed) > 2 and typed[0] == right[0] and not typed.isdigit() and \
        difflib.SequenceMatcher(None, right, typed).ratio() >= 0.75


def pattern(messy, meant):
    """Generalise one lesson into a dictionary line with gaps, so the brain reads every line like it:
    ('set background to purple', 'set the background colour to purple')
        -> 'set background to a colour = set the background colour to a colour'.
    Colours become 'a colour', numbers 'a number', the words to show (in both, in the same order) 'something'.
    None when it can't be said safely: a gap twice, nothing fixed left to recognise it by, no gap at all (one line
    word for word is the brain's job, not the dictionary's), or the reading makes things up or drops them (a
    colour, a number or words to show that aren't in the line, or ones in the line it leaves out)."""
    colours = colour_words()
    m = re.findall(r"[a-z0-9']+", messy.lower())
    w = re.findall(r"[a-z0-9']+", meant.lower())
    if not m or not w or len(m) > 9:
        return None
    left, right = " ".join(m), " ".join(w)
    if any(re.search(r"\d", x) and not x.isdigit() for x in m):
        return None                                      # 'x5': a count it can't make a gap of
    places = {"top", "bottom", "left", "right", "middle", "centre", "center", "corner"}
    if set(w) & places - set(m):
        return None                                      # a place the line doesn't say
    if any(a == "in" and b not in w and b not in colours for a, b in zip(m, m[1:])):
        return None                                      # 'in canary' left out of the reading
    named = set()                                        # colour names (longest first) and numbers in each side
    for side in (left, right):
        words, found, k = side.split(), set(), 0
        while k < len(words):
            for n in (3, 2, 1):
                if " ".join(words[k:k + n]) in colours or (n == 1 and words[k].isdigit()):
                    found.add(" ".join(words[k:k + n]))
                    k += n
                    break
            else:
                k += 1
        named.add(frozenset(found))
    if len(named) != 1:
        return None                                      # a colour or number made up, or one left out
    gaps = {value: "a number" if value.isdigit() else "a colour" for value in next(iter(named))}
    if len(set(gaps.values())) != len(gaps):
        return None                                      # two colours (or numbers): which gap is which?
    said = " ".join(x for x in shown_words(right, colours).split() if x not in gaps)
    if said:
        if f" {said} " not in f" {left} " or not set(said.split()) - FUNCTION_WORDS:
            return None                                  # it shows words the line doesn't say (or just 'in')
        gaps[said] = "something"
    for value, gap in sorted(gaps.items(), key=lambda kv: -len(kv[0])):
        if left.count(value) != 1 or right.count(value) != 1:
            return None
        left = re.sub(rf"\b{re.escape(value)}\b", gap, left)
        right = re.sub(rf"\b{re.escape(value)}\b", gap, right)
    fixed = [x for x in left.split() if x not in ("a", "colour", "number", "something")]
    if not gaps or not fixed or left.split()[0] in ("a", "something") or left == right:
        return None                                      # nothing to generalise, or nothing left to recognise it by
    if re.search(r"\b(?:the|a|an) something\b", left) and not re.search(r"\b(?:the|a|an) something\b", right):
        return None                                      # 'save the links' read as 'write links': a word dropped
    if len(right.split()) > len(left.split()) + 4:
        return None                                      # far more in the reading than in the line: made up
    return f"{left} = {right}"


def trainer():
    global _trainer
    if _trainer is None:
        from trainer.core import Trainer
        _trainer = Trainer(quiet=True)
    return _trainer


def words_of(text):
    return {x for x in re.findall(r"[a-z0-9']+", text.lower()) if x not in FUNCTION_WORDS}


def align(pieces, wanted):
    """The messy line's pieces (brick's split) matched to the model's directions, in order, by the words they share:
    -> [(the pieces' text, the direction)]. Neighbouring pieces can go together ('wipe' 'everything off' -> 'clear
    the screen'); a piece the model left out, or a direction it made up, is matched with nothing."""
    n, m = len(pieces), len(wanted)
    best = {(0, 0): (0.0, [])}
    for i in range(n + 1):
        for j in range(m + 1):
            if (i, j) not in best:
                continue
            score, pairs = best[(i, j)]
            steps = [((i + 1, j), score, pairs), ((i, j + 1), score, pairs)]      # skip a piece / a direction
            if j < m:
                for k in range(i + 1, n + 1):
                    text = " ".join(pieces[i:k])
                    shared = len(words_of(text) & words_of(wanted[j]))
                    late = shared and not words_of(pieces[i]) & words_of(wanted[j])   # its words only further on:
                    gain = shared + 0.1 + 0.01 * (k - i) - 0.05 * bool(late)          # the extra pieces belong to
                    steps.append(((k, j + 1), score + gain, pairs + [(text, wanted[j])]))   # the direction before
            for at, sc, pr in steps:
                if at[0] <= n and at[1] <= m and (at not in best or sc > best[at][0]):
                    best[at] = (sc, pr)
    return best[(n, m)][1]


class _no_model:
    """Lessons are learned by brick alone: the model's cached readings are what's taught, never what's built with,
    and the model itself is never asked (no graphics card in the background)."""
    def __enter__(self):
        self.was = os.environ.get("BRICK_NEURAL")
        os.environ["BRICK_NEURAL"] = "off"

    def __exit__(self, *problem):
        if self.was is None:
            os.environ.pop("BRICK_NEURAL", None)
        else:
            os.environ["BRICK_NEURAL"] = self.was


def probes(word, done):
    """Lines brick already reads right that hold this word: the lessons it knew, and the dictionary's own lines
    without gaps. A new pattern mustn't change how any of them builds."""
    import dictionary
    found = [line for line, how in done.items() if how == "known"]
    for e in dictionary.entries():
        for text in (e.key, e.meaning):
            if not any(gap in text for gap in dictionary.GAPS) and not text.startswith("#"):
                found.append(text)
    return [x for x in dict.fromkeys(found) if word in x.lower().split()][:40]


_already = None                 # (when, what it was checked against, the tests and examples failing without the lessons)


def _stamp():
    from trainer import core
    newest = core.code_stamp()
    for folder in (os.path.join(core.HERE, "tests"), core.EXAMPLES):
        for name in os.listdir(folder) if os.path.isdir(folder) else []:
            newest = max(newest, os.path.getmtime(os.path.join(folder, name)))
    return newest


def check(t, say=print):
    """Trainer.check(), at half the cost: brick's tests and examples run once with the new lines; only when something
    fails that wasn't failing at the last check (or that was over six hours ago, or the code or tests changed since)
    do they run again without them, to see whether the new lines are to blame. -> the lines taken back."""
    global _already
    from trainer import core
    if not t.added:
        return []
    found = core.broken(t.builder)
    taken = []
    known = _already and time.time() - _already[0] < 6 * 3600 and _already[1] == _stamp()
    if not (known and found <= _already[2]):
        for path, line in t.added:
            core.remove_line(path, line)
        without = core.broken(t.builder)
        _already = (time.time(), _stamp(), without)
        if found - without:                              # the new lines broke something: they stay out
            taken = [line for _, line in t.added]
            say("took back " + ", ".join(taken) + " (broke " +
                ", ".join(sorted(os.path.basename(b) for b in found - without)) + ")")
        else:
            for path, line in t.added:
                core.add_line(path, line)
    t.added = []
    t.save()
    return taken


def teach(most=5, say=print):
    with _no_model():
        return _teach(most, say)


def _teach(most, say):
    done = _load()
    todo = [(line, best) for line, best in lessons() if line not in done][-most:]
    report = {"looked": len(todo), "learned": [], "already": 0, "not yet": [], "pieces": 0, "refused": 0}
    if not todo:
        return report
    t = trainer()
    before = {}                                          # probe line -> its build before any pattern this batch

    def build(text):
        if text not in before:
            before[text] = t.builder.build(text)
        return before[text]

    import dictionary
    from trainer import universe
    from trainer.core import add_line
    wants = {}
    for line, best in todo:
        meant = "\n".join(p.strip() for p in re.split(r"\bthen\b", best) if p.strip())
        want = t.builder.build(meant, by_itself=True)
        if not isinstance(want, bytes):
            done[line] = "the model's reading doesn't build"
            report["refused"] += 1
            continue
        if t.builder.build(line) == want:                # brick's normal build (the universe helping; no model)
            done[line] = "known"
            report["already"] += 1
            continue
        changed = changes_meaning(line, best)            # the model dropped or swapped a word that matters:
        if changed and "\n" not in meant:               # nothing to learn (a long line's pieces may still be right)
            done[line] = f"the model's reading changes the meaning ({changed})"
            report["refused"] += 1
            continue
        wanted = meant.split("\n")
        pieces = [" ".join(tok.text for tok in piece) for piece in universe.analyse(line)]
        pairs = align(pieces, wanted) if len(wanted) > 1 and len(pieces) > 1 else [(line, meant)]
        taught = 0
        for text, w in pairs:
            piece_want = t.builder.build(w, by_itself=True)
            if not isinstance(piece_want, bytes) or t.builder.build(text) == piece_want or changes_meaning(text, w):
                continue
            t.learn_toward(text, w) or t.steer(text, w, times=3)
            if t.builder.build(text) == piece_want:
                taught += 1
                continue
            # the brain couldn't reach it by itself: learn the pattern (a dictionary line with gaps), kept only
            # when the piece then builds exactly the model's program and nothing brick already reads changes;
            # t.check() takes it back if it breaks a test or an example
            rule = pattern(text, w) if "\n" not in w else None
            if not rule:
                continue
            first = next(x for x in rule.split(" = ")[0].split() if x not in ("a", "colour", "number", "something"))
            looked = probes(first, done)
            olds = [build(x) for x in looked]
            with t.builder.trying(dictionary.PATH, rule) as trial:
                ok = trial.build(text) == piece_want
                moved = [x for x, old in zip(looked, olds) if ok and trial.build(x) != old]
            if ok and not moved:
                add_line(dictionary.PATH, rule)
                t.added.append((dictionary.PATH, rule))
                before.clear()
                taught += 1
                say(f"learned the pattern: {rule}    (from the model's reading of '{text}')")
            elif moved:
                say(f"not kept the pattern {rule}: it would change how brick reads '{moved[0]}'")
        wants[line] = (want, best, taught, len(pairs), changed)
    if t.added:
        check(t, say)                                    # anything learned that broke a test or example comes out
    t.save()
    for line, (want, best, taught, count, changed) in wants.items():
        if changed:
            done[line] = f"the model's reading changes the meaning ({changed}" + \
                (f"; {taught} of {count} pieces learned)" if count > 1 else ")")
            report["refused"] += 1
        elif t.builder.build(line) == want:
            done[line] = "learned"
            report["learned"].append(line)
            say(f"learned from the model: '{line}' -> {best}")
        else:
            done[line] = f"not yet ({taught} of {count} pieces learned)" if count > 1 and taught else "not yet"
            report["not yet"].append(line)
        report["pieces"] += taught if count > 1 else 0
    _save(done)
    return report


def command(args):
    most = int(args[0]) if args and args[0].isdigit() else 20
    got = teach(most)
    print(f"The model's readings looked at: {got['looked']} · learned by the brain: {len(got['learned'])} · "
          f"known already: {got['already']} · not yet: {len(got['not yet'])} (pieces of long lines learned: "
          f"{got['pieces']}) · readings refused (they don't build, or change the meaning): {got['refused']}")
    for line in got["not yet"][:10]:
        print(f"  not yet: {line}")
    return 0
