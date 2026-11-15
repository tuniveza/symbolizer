"""The teacher: the language model teaches brick's own brain, so the brain reads the next line like it by itself.

The model is the teacher, not the reader. Every line you write in Symbolizer's brick mode is read by the model in
the background (neural.readings, remembered in trainer/data/neural.json). The teacher goes through those readings
(the daemon calls it between its rounds, with the model switched off) and, for each line brick's own brain doesn't
already build the same way:

  1. builds the model's reading by brick alone: no program, no lesson (the model made something up);
  2. teaches the brain the reading of each messy direction that makes the same program (Trainer.steer: the
     memory and the imagination neurons), and looks up words brick doesn't know in the dictionaries
     (Trainer.learn_toward: WordNet, ConceptNet; kept only when the line then builds exactly the right program);
  3. builds the line again by brick alone: when it now makes the model's program, the brain has learned it.

    teach(most=5, say=print) -> {"looked": n, "learned": [lines], "already": n, "not yet": [lines]}
"""
import json
import os
import re

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
    """(line, the model's best reading) for every line the model read, newest last."""
    out = []
    for key, readings in list(neural.cache().items()):
        if key.startswith("choose|") or not readings or not isinstance(readings, list):
            continue
        parts = key.split("|", 3)
        if len(parts) != 4:
            continue
        line, best = parts[3].strip(), readings[0]
        if line and isinstance(best, str) and best.strip().lower() != line.lower():
            out.append((line, best.strip()))
    return out


def pattern(messy, meant):
    """Generalise one lesson into a dictionary line with gaps, so the brain reads every line like it:
    ('set background to purple', 'set the background colour to purple')
        -> 'set background to a colour = set the background colour to a colour'.
    Colours become 'a colour', numbers 'a number', the words to show (in both, in the same order) 'something'.
    None when it can't be said safely (a gap twice, or nothing fixed left to recognise it by)."""
    import dictionary
    from actions import COLOURS
    colours = set(dictionary.colour_table()) | set(COLOURS)
    m = re.findall(r"[a-z0-9']+", messy.lower())
    w = re.findall(r"[a-z0-9']+", meant.lower())
    if not m or not w or len(m) > 9:
        return None
    gaps = {}
    for word in set(m) & set(w):
        if word in colours:
            gaps[word] = "a colour"
        elif word.isdigit():
            gaps[word] = "a number"
    if len(set(gaps.values())) != len(gaps):
        return None                                      # two colours (or numbers): which gap is which?
    said = ""                                            # the words to show, from brick's own form of the meaning:
    got = re.match(r"(?:after \S+ seconds? )?(?:write|make it say) (.+?)(?: in [a-z ]+| at the \S+| on the \S+|"
                   r" \d+ times| in bold)*$", " ".join(w))    # write WORDS [in COLOUR] [at PLACE] [N times]
    if got:
        said = " ".join(x for x in got.group(1).split() if x not in gaps)
    if said and f" {said} " in f" {' '.join(m)} ":
        gaps[said] = "something"
    left, right = " ".join(m), " ".join(w)
    for value, gap in sorted(gaps.items(), key=lambda kv: -len(kv[0])):
        if left.count(value) != 1 or right.count(value) != 1:
            return None
        left = re.sub(rf"\b{re.escape(value)}\b", gap, left)
        right = re.sub(rf"\b{re.escape(value)}\b", gap, right)
    fixed = [x for x in left.split() if x not in ("a", "colour", "number", "something")]
    if not fixed or left.split()[0] in ("a", "something") or left == right:
        return None                                      # nothing left to recognise it by
    return f"{left} = {right}"


def trainer():
    global _trainer
    if _trainer is None:
        from trainer.core import Trainer
        _trainer = Trainer(quiet=True)
    return _trainer


def teach(most=5, say=print):
    done = _load()
    todo = [(line, best) for line, best in lessons() if line not in done][-most:]
    report = {"looked": len(todo), "learned": [], "already": 0, "not yet": []}
    if not todo:
        return report
    t = trainer()
    for line, best in todo:
        meant = "\n".join(p.strip() for p in re.split(r"\bthen\b", best) if p.strip())
        want = t.builder.build(meant, by_itself=True)
        if not isinstance(want, bytes):
            done[line] = "the model's reading doesn't build"
            continue
        if t.builder.build(line) == want:                # brick's normal build (the universe helping; no model)
            done[line] = "known"
            report["already"] += 1
            continue
        from trainer import universe
        pieces = universe.analyse(line)
        wanted = meant.split("\n")
        pairs = [(" ".join(tok.text for tok in piece), w) for piece, w in zip(pieces, wanted)] \
            if len(pieces) == len(wanted) else [(line, meant)]
        for text, w in pairs:
            piece_want = t.builder.build(w, by_itself=True)
            if not isinstance(piece_want, bytes) or t.builder.build(text) == piece_want:
                continue
            t.learn_toward(text, w) or t.steer(text, w, times=3)
            if t.builder.build(text) == piece_want:
                continue
            # the brain couldn't reach it by itself: learn the pattern (a dictionary line with gaps), kept only
            # when the piece then builds exactly the model's program; t.check() takes it back if it breaks anything
            rule = pattern(text, w) if "\n" not in w else None
            if rule:
                import dictionary
                from trainer.core import add_line
                with t.builder.trying(dictionary.PATH, rule) as trial:
                    ok = trial.build(text) == piece_want
                if ok:
                    add_line(dictionary.PATH, rule)
                    t.added.append((dictionary.PATH, rule))
                    say(f"learned the pattern: {rule}    (from the model's reading of '{text}')")
        t.check()                                        # anything learned that broke a test or example comes out
        t.save()
        if t.builder.build(line) == want:
            done[line] = "learned"
            report["learned"].append(line)
            say(f"learned from the model: '{line}' -> {best}")
        else:
            done[line] = "not yet"
            report["not yet"].append(line)
    _save(done)
    return report


def command(args):
    most = int(args[0]) if args and args[0].isdigit() else 20
    got = teach(most)
    print(f"The model's readings looked at: {got['looked']} · learned by the brain: {len(got['learned'])} · "
          f"known already: {got['already']} · not yet: {len(got['not yet'])}")
    for line in got["not yet"][:10]:
        print(f"  not yet: {line}")
    return 0
