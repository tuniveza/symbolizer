"""Outliers: what brick couldn't settle by itself, waiting for a person to decide (the Decisions tab in
brick gui). brick keeps learning on its own meanwhile; these are only the lines and words it gave up on.

texts/factory/unsure.txt holds them, one block each, separated by a blank line:
    line: wait 2 secs then wirte hi
    said: I don't know how to 'wirte'. Did you mean 'write'?
    meant: wait 2 seconds then write hi            (when brick knows what it should mean: the scrambler)
    could be: restate = repeat | restate = say      (when a word could mean more than one thing)
    from: the scrambler

What a person decides goes in texts/decisions.txt, in plain words.

Grouped by root (groups()): the factory makes thousands of lines, but they fail for a few reasons, mostly
one word brick doesn't know ('carefully', 'pick', 'every') or one kind of missing detail ('Set it to what?').
One decision about the root settles every line that grew from it (decide_group()), so the desk shows a few
questions instead of a thousand lines, and stops keeping more made-up lines for a root once it has a few.
"""
import hashlib
import os
import re
import time

HERE = os.path.dirname(os.path.abspath(__file__))
PATH = os.path.join(HERE, "texts", "factory", "unsure.txt")
FOR_DIRECTOR = os.path.join(HERE, "texts", "factory", "for-director.txt")
DECISIONS = os.path.join(HERE, "texts", "decisions.txt")
FIELDS = ("line", "said", "meant", "could be", "from")
MADE_UP = ("the sentence maker", "the scrambler")   # lines brick made itself (not from your programs)
KEEP_PER_ROOT = 6                                   # made-up lines kept per root: enough examples to decide
UNKNOWN = re.compile(r"^I don't know how to '([^']+)'(?:\. Did you mean '([^']+)'\?)?")
MISUSED = re.compile(r"^What should '([^']+)' do here\?")


def entries(path=None):
    """Every outlier waiting -> [{"id", "line", "said", "meant", "could be": [...], "from"}]."""
    path = path or PATH
    if not os.path.isfile(path):
        return []
    with open(path, encoding="utf-8", errors="replace") as f:
        blocks = f.read().split("\n\n")
    found = []
    for block in blocks:
        entry = {}
        for row in block.strip().split("\n"):
            name, _, value = row.partition(": ")
            if name in FIELDS and value:
                entry[name] = value.strip()
        if entry.get("line"):
            entry["could be"] = [c.strip() for c in entry["could be"].split(" | ")] if entry.get("could be") else []
            entry["id"] = hashlib.sha1(entry["line"].encode()).hexdigest()[:12]
            found.append(entry)
    return found


def root(entry):
    """What a line's trouble grows from -> a key. Lines with the same key are settled by one decision."""
    said = entry.get("said", "")
    found = UNKNOWN.match(said)
    if found:
        return "word:" + found.group(1).lower()
    found = MISUSED.match(said)
    if found:
        return "misused:" + found.group(1).lower()
    if "should be a name of one to three words" in said:
        return "said:a name of one to three words"
    if entry.get("meant") and said == "brick read it as something else":
        return "said:read differently by the scrambler"
    return "said:" + (re.sub(r"'[^']*'", "'…'", said) or "no reason given")


def add(line, said="", meant=None, could_be=(), source="", path=None):
    """A new outlier, unless that line is waiting already, or brick made it up itself and its root
    already has KEEP_PER_ROOT lines waiting (one decision settles them all; more of them don't help)."""
    path = path or PATH
    waiting = entries(path)
    if any(e["line"] == line for e in waiting):
        return
    if source in MADE_UP and path == PATH:          # only the desk is capped, never the Director's list
        key = root({"said": said, "meant": meant})
        if sum(1 for e in waiting if root(e) == key) >= KEEP_PER_ROOT:
            return
    rows = [f"line: {line}"] + ([f"said: {said}"] if said else []) + ([f"meant: {meant}"] if meant else []) + \
        ([f"could be: {' | '.join(could_be)}"] if could_be else []) + ([f"from: {source}"] if source else [])
    os.makedirs(os.path.dirname(path), exist_ok=True)
    with open(path, "a", encoding="utf-8") as f:
        f.write("\n".join(rows) + "\n\n")


def remove(id, path=None):
    """Take an outlier off the list -> the entry, or None."""
    gone = remove_many([id], path)
    return gone[0] if gone else None


def remove_many(ids, path=None):
    """Take several outliers off the list in one go -> the entries taken."""
    path = path or PATH
    ids = set(ids)
    found = entries(path)
    gone = [e for e in found if e["id"] in ids]
    if gone:
        keep = [e for e in found if e["id"] not in ids]
        with open(path + ".new", "w", encoding="utf-8") as f:
            for e in keep:
                rows = [f"line: {e['line']}"] + [f"{name}: {e[name]}" for name in ("said", "meant") if e.get(name)] + \
                    ([f"could be: {' | '.join(e['could be'])}"] if e["could be"] else []) + \
                    ([f"from: {e['from']}"] if e.get("from") else [])
                f.write("\n".join(rows) + "\n\n")
        os.replace(path + ".new", path)
    return gone


def note(what):
    """Write a decision down, in plain words."""
    with open(DECISIONS, "a", encoding="utf-8") as f:
        f.write(f"{time.strftime('%Y-%m-%d %H:%M')}  {what}\n")


# ---------------- a person decides ----------------

def decide(id, action, meaning=""):
    """action: 'means' (the line means this, in brick's words), 'word' (this way of teaching the word),
    'director' (leave it for the English Director) or 'dismiss'. -> (ok, a message to show)."""
    entry = next((e for e in entries() if e["id"] == id), None)
    if not entry:
        return False, "That one has been settled already."
    line = entry["line"]
    if action == "means":
        import review
        problem = review.correct(line, 1, meaning)
        if problem:
            return False, problem
        remove(id)
        note(f"'{line}' means: {meaning}")
        return True, f"Kept: brick reads '{line}' as '{meaning}' from now on."
    if action == "word":
        import factory
        ok, message = factory.teach(meaning)
        if ok:
            remove(id)
            note(f"taught: {meaning}  (for '{line}')")
        return ok, message
    if action == "director":
        remove(id)
        add(line, entry.get("said", ""), entry.get("meant"), entry["could be"], entry.get("from", ""), path=FOR_DIRECTOR)
        note(f"left for the English Director: '{line}'")
        return True, "Left for the English Director (texts/factory/for-director.txt)."
    if action == "dismiss":
        remove(id)
        note(f"dismissed: '{line}'")
        return True, "Dismissed."
    return False, "I don't know that choice."


# ---------------- decide a root: one decision for every line that grew from it ----------------

def groups(path=None):
    """The waiting lines grouped by root, biggest first -> [{"key", "kind", "word", "question", "count",
    "examples", "ids", "guess"}]. kind: "word" (a word brick doesn't know: teach it once),
    "misused" (a word brick knows, used in a way it doesn't), "said" (the same kind of trouble)."""
    found = {}
    for e in entries(path):
        found.setdefault(root(e), []).append(e)
    made = []
    for key, items in found.items():
        kind, _, word = key.partition(":")
        guess = None
        if kind == "word":
            guesses = [UNKNOWN.match(e.get("said", "")) for e in items]
            guess = next((g.group(2) for g in guesses if g and g.group(2)), None)
            question = f"What does '{word}' mean at the start of a direction?"
        elif kind == "misused":
            question = f"'{word}' is used in a way brick doesn't know yet."
        else:
            question = {"read differently by the scrambler": "brick read these differently from what they should mean.",
                        "a name of one to three words": "brick took too many words as a name in these."}.get(
                word, f"brick said: {items[0].get('said') or 'nothing (no reason given)'}")
        made.append({"key": key, "kind": kind, "word": word, "question": question, "count": len(items),
                     "guess": guess, "ids": [e["id"] for e in items], "examples": items[:KEEP_PER_ROOT]})
    made.sort(key=lambda g: (-g["count"], g["key"]))
    return made


def trouble(line):
    """What stops this line building now -> brick's message, or None if it builds. (After a word was taught.)"""
    import tempfile
    from compiler import Compiler
    with tempfile.TemporaryDirectory() as folder:
        try:
            Compiler(os.path.join(folder, "mywords.txt"), ask_user=lambda question: "no").compile(line)
            return None
        except Exception as problem:
            return str(problem).split("\n")[0] or "brick refused it"


def builds(line):
    """Does this line build now?"""
    return trouble(line) is None


def resay(changed, path=None):
    """Give waiting lines a new reason ({id: message}), so they move to the root that stops them now."""
    path = path or PATH
    found = entries(path)
    with open(path + ".new", "w", encoding="utf-8") as f:
        for e in found:
            said = changed.get(e["id"], e.get("said"))
            rows = [f"line: {e['line']}"] + ([f"said: {said}"] if said else []) + \
                ([f"meant: {e['meant']}"] if e.get("meant") else []) + \
                ([f"could be: {' | '.join(e['could be'])}"] if e["could be"] else []) + \
                ([f"from: {e['from']}"] if e.get("from") else [])
            f.write("\n".join(rows) + "\n\n")
    os.replace(path + ".new", path)


def decide_group(key, action, meaning=""):
    """One decision for a whole root. action: 'word' (the word means this: 'write', or a whole
    'carefully something = something'), 'filler' (the word adds nothing: skip it), 'director' or 'dismiss'
    (every line of the root). -> (ok, a message to show)."""
    group = next((g for g in groups() if g["key"] == key), None)
    if not group:
        return False, "That one has been settled already."
    lines, word, count = [e["line"] for e in group["examples"]], group["word"], group["count"]
    if action in ("word", "filler"):
        if group["kind"] != "word":
            return False, f"brick already knows '{word}', so it can't be taught as a new word."
        if action == "filler":
            entry = f"{word} something = something"
        elif "=" in meaning:
            entry = meaning
        elif meaning.strip():
            entry = f"{word} = {meaning.strip()}"
        else:
            return False, f"Say what '{word}' means first, in brick's words (like: write)."
        import factory
        ok, message = factory.teach(entry)
        if not ok:
            return False, message
        every = [e for e in entries() if root(e) == key]
        now = {e["id"]: trouble(e["line"]) for e in every}
        settled = [id for id, said in now.items() if said is None]
        remove_many(settled)
        resay({id: said for id, said in now.items() if said})    # the rest move to what stops them now
        note(f"taught: {entry}  (settles '{word}', {len(settled)} of {len(every)} lines)")
        left = len(every) - len(settled)
        return True, f"Taught: {entry}. {len(settled)} of {len(every)} lines build now" + \
            (f"; {left} still need something else, so they moved to those questions." if left else ".")
    if action == "director":
        taken = remove_many(group["ids"])
        for e in taken:
            add(e["line"], e.get("said", ""), e.get("meant"), e["could be"], e.get("from", ""), path=FOR_DIRECTOR)
        note(f"left for the English Director: {len(taken)} lines about {key}  (like '{lines[0]}')")
        return True, f"Left {len(taken)} lines for the English Director."
    if action == "dismiss":
        taken = remove_many(group["ids"])
        note(f"dismissed {len(taken)} lines about {key}  (like '{lines[0]}')")
        return True, f"Dismissed {len(taken)} lines."
    return False, "I don't know that choice."
