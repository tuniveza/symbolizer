"""brick trainer neurons: the imagination neurons (the owner's design) reading brick's own language. View only.

The neurons are trained on brick's own words: the meaning groups come from brick's universe (its own words by idea:
put and place are "placing", left and top are "place") and the dictionary's sections, and the practice comes from
directions in the sentence pool and the examples. Then each word of your line is read in the context of the rest of the line, and the 16
energy percentages decide which meaning it has, or that none is relevant.

It never changes how brick builds anything: brick still reads the line its own way. This is for seeing how the
design plays out on real brick language. The engine is trainer/imagination/sim.js (the same one as the page), run
with node.

    brick trainer neurons "put window at left side of screen and place text in brown at the top right of the window"
    brick trainer neurons --long "..."     train twice as long (300 s instead of 150 s of simulated time)
    brick trainer neurons --as "mirror oracle" "..."     read it with an archetype's brain (personality + decisions)
    brick trainer neurons --archetypes     list the archetypes
"""
import json
import os
import random
import re
import shutil
import subprocess
from collections import Counter

HERE = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
DICTIONARY = os.path.join(HERE, "texts", "dictionary.txt")
POOL = os.path.join(HERE, "trainer", "data", "sentences.txt")
EXAMPLES = os.path.join(HERE, "texts", "examples")
RUN = os.path.join(HERE, "trainer", "imagination", "run.js")

SECTIONS = [("built in", "colour"), ("need the word colour", "colour"), ("showing", "show"), ("positions", "place"),
            ("the window", "window"), ("the console", "console"), ("comparisons", "check"), ("maths", "maths"),
            ("lists", "list"), ("loops", "loop"), ("waiting", "wait"), ("adding and taking", "add"),
            ("changing things", "change"), ("time", "time"), ("asking", "ask"), ("stopping", "stop"), ("files", "file"),
            ("remembering", "set")]
SMALL = {"the", "a", "an", "of", "and", "to", "with", "at", "in", "on", "it", "its", "is", "then", "that", "this", "for",
         "as", "by", "be", "words", "word"}
PER_GROUP = 8
SENTENCES = 450


def words_of(text):
    return re.findall(r"[a-z']+", text.lower())


def dictionary_groups():
    """{group: [words]} from the dictionary's sections: each section's single-word keys and what they mean."""
    groups, group = {}, None
    with open(DICTIONARY, encoding="utf-8") as f:
        for line in f:
            if line.startswith("# ") and line[2:3].isupper():
                heading = line[2:].lower()
                group = next((name for key, name in SECTIONS if heading.startswith(key) or key in heading), None)
                if group == "colour" and ":" in heading:            # "# Built in: black, red, ..." names colours itself
                    groups.setdefault("colour", []).extend(w for w in words_of(heading.split(":", 1)[1]) if w != "gray")
                continue
            if group and " = " in line and not line.startswith("#"):
                key, meaning = (part.strip().lower() for part in line.split(" = ", 1))
                if re.fullmatch(r"[a-z]+", key):
                    groups.setdefault(group, []).append(key)
                if re.fullmatch(r"[a-z]+", meaning) and group != "colour":
                    groups.setdefault(group, []).append(meaning)
    return {g: list(dict.fromkeys(ws)) for g, ws in groups.items()}


NOT_GROUPS = {"chatter", "filler", "glue", "other action", "it", "by means of", "look word"}


def universe_groups():
    """{idea: [words]} from brick's universe: its own words grouped by idea (put and place are placing ...)."""
    from trainer import universe
    groups = {}
    for word, ideas in universe.universe().words.items():
        if not re.fullmatch(r"[a-z]+", word):
            continue
        for idea in ideas:
            if idea not in NOT_GROUPS:
                groups.setdefault(re.sub(r"\W+", "-", idea), []).append(word)
    return groups


def all_groups():
    """The universe's idea groups first (brick's own words), then the dictionary's sections it doesn't cover."""
    groups = universe_groups()
    for name, words in dictionary_groups().items():
        if name in groups:
            groups[name] = list(dict.fromkeys(groups[name] + words))
        else:
            groups[name] = words
    return groups


def directions():
    """Short directions to practise on: the sentence pool and the examples, split where one direction ends."""
    texts = []
    if os.path.isfile(POOL):
        with open(POOL, encoding="utf-8", errors="replace") as f:
            texts += f.read().splitlines()
    for name in sorted(os.listdir(EXAMPLES)) if os.path.isdir(EXAMPLES) else []:
        if name.endswith(".txt"):
            with open(os.path.join(EXAMPLES, name), encoding="utf-8", errors="replace") as f:
                texts += f.read().splitlines()
    out = []
    for text in texts:
        for piece in re.split(r"[.;,!?:]|\b(?:then|after that|next|finally|first|now|once that is done|straight after)\b",
                              text.lower()):
            ws = words_of(piece)
            if len(ws) >= 2:
                out.append(ws)
    return out


def build(line, seed=1):
    """The data and questions for the neurons, in the engine's own text format."""
    asked = [w for w in dict.fromkeys(words_of(line)) if w not in SMALL]
    pool = directions()
    count = Counter(w for d in pool for w in set(d))
    groups = {}
    for group, members in all_groups().items():
        ranked = sorted(members, key=lambda w: -count[w])
        keep = [w for w in ranked if count[w]][:PER_GROUP] + [w for w in members if w in asked]
        keep = list(dict.fromkeys(keep))
        if len(keep) >= 2:
            groups[group] = keep
    vocab = {w for ws in groups.values() for w in ws} | set(asked)
    practice = [[w for w in d if w in vocab] for d in pool]
    practice = [d for d in practice if len(set(d)) >= 2]
    rnd = random.Random(seed)
    with_asked = [d for d in practice if set(d) & set(asked)]
    rnd.shuffle(with_asked)
    rest = [d for d in practice if not set(d) & set(asked)]
    rnd.shuffle(rest)
    chosen = with_asked[:SENTENCES // 2]
    chosen += rest[:SENTENCES - len(chosen)]
    data = "\n".join([f"group {g}: {', '.join(ws)}" for g, ws in groups.items()] + [" ".join(d) for d in chosen])
    questions = "\n".join(f"{w} @ {' '.join(x for x in asked if x != w)}" for w in asked)
    unseen = [w for w in asked if not any(w in d for d in chosen) and not any(w in ws for ws in groups.values())]
    return data, questions, asked, groups, len(chosen), unseen


def run_node(job):
    done = subprocess.run([shutil.which("node"), RUN], input=json.dumps(job), capture_output=True, text=True, timeout=600)
    if done.returncode != 0:
        raise RuntimeError(done.stderr.strip()[-400:])
    return json.loads(done.stdout)


def archetypes():
    """[{name, type, bias, yours, line}] from the engine."""
    return run_node({"list": True})


def find_archetype(name, names=None):
    """The archetype's full name, matched the way the engine matches it (case, "the" and spaces don't matter)."""
    clean = lambda x: re.sub(r"[^a-z ]", "", re.sub(r"^the\s+", "", str(x).lower())).strip()
    names = names if names is not None else [a["name"] for a in archetypes()]
    want = clean(name)
    return next((n for n in names if clean(n) == want), None) or next((n for n in names if want and want in clean(n)), None)


def split_args(args):
    """(line, long, archetype name or None, list wanted) from the command's words."""
    args, brain, rest = list(args), None, []
    k = 0
    while k < len(args):
        a = args[k]
        if a in ("--as", "--archetype") and k + 1 < len(args):
            brain, k = args[k + 1], k + 2
            continue
        if a.startswith("--as="):
            brain = a.split("=", 1)[1]
        elif a not in ("--long", "--archetypes"):
            rest.append(a)
        k += 1
    return " ".join(rest).strip(), "--long" in args, brain, "--archetypes" in args


def read_line(line, brain=None, long=False):
    """Train the neurons and read every word of the line. Returns a dict; "error" is set when it couldn't.
    brain: an archetype name (any spelling find_archetype accepts) or None for the neutral network."""
    if not shutil.which("node"):
        return {"error": "The imagination neurons run with node, and node isn't installed here."}
    if brain:
        found = find_archetype(brain)
        if not found:
            return {"error": f"There's no archetype called \"{brain}\". See them all with: brick trainer neurons --archetypes"}
        brain = found
    from trainer import intuition
    # the learned network, read in one run: on with BRICK_NEURONS_READ=learned (default once the Go engine lands:
    # in JavaScript it is 6.7 s against 1.3 s for the small fresh network)
    if os.environ.get("BRICK_NEURONS_READ") == "learned" and os.path.isfile(intuition.STATE) and not long:
        asked = [w for w in dict.fromkeys(words_of(line)) if w not in SMALL]
        if len(asked) < 2:
            return {"error": "Give it a line with at least two words that aren't small joining words."}
        try:
            out = run_node({"task": "readline", "state": intuition.STATE, "words": asked, "brain": brain})
        except (RuntimeError, subprocess.TimeoutExpired, ValueError) as error:
            return {"error": f"The neurons stopped with an error: {error}"}
        return _rows(line, brain, out, set(), 0, 0, 0)
    data, questions, asked, groups, practised, unseen = build(line)
    if len(asked) < 2:
        return {"error": "Give it a line with at least two words that aren't small joining words."}
    job = {"data": data, "questions": questions, "mode": "groups", "seed": 1, "steps": 6000 if long else 3000, "brain": brain}
    try:
        out = run_node(job)
    except (RuntimeError, subprocess.TimeoutExpired, ValueError) as error:
        return {"error": f"The neurons stopped with an error: {error}"}
    if out.get("error"):
        return {"error": out["error"]}
    return _rows(line, brain, out, unseen, practised, len(groups), 300 if long else 150)


def _rows(line, brain, out, unseen, practised, groups, seconds):
    if out.get("error"):
        return {"error": out["error"]}
    rows = []
    for a in out["answers"]:
        w = a["word"]
        if not a.get("found"):
            rows.append({"word": w, "meaning": "(no neuron)", "sure": "never used in practice" if w in unseen else "not in the network",
                         "senses": "", "verdict": "none"})
            continue
        if a["verdict"] == "accurate" and a["clear"]:
            meaning, sure = a["meaning"] or a["sense"], f"accurate: {a['matched']} of {a['carried']} energies"
        elif a["verdict"] == "accurate":
            meaning, sure = "too close to call", f"two senses tie on {a['matched']} of {a['carried']}"
        else:
            meaning, sure = a["verdict"], f"best {a['matched']} of {a['carried']} energies"
        senses = "; ".join(f"{s['label']} (near {', '.join(s['near'])})" for s in a["senses"]) if len(a["senses"]) > 1 \
            else f"1 sense (near {', '.join(a['senses'][0]['near'])})" if a["senses"][0]["near"] else "1 sense (no lines yet)"
        rows.append({"word": w, "meaning": str(meaning), "sure": sure, "senses": senses,
                     "verdict": a["verdict"] if a["clear"] or a["verdict"] != "accurate" else "tie"})
    return {"line": line, "brain": brain, "seconds": seconds, "practised": practised, "groups": groups, "learned": bool(out.get("learned")),
            "neurons": out["neurons"], "links": out["links"], "archetype": out.get("archetype"), "rows": rows,
            "split": out["split"], "small": [w for w in dict.fromkeys(words_of(line)) if w in SMALL]}


def describe(archetype):
    """One plain line about the archetype's brain as it ran."""
    a, d = archetype, archetype["rules"]
    return (f"{a['name']} ({a['type']}, leads with {' > '.join(a['stack'])}): a meaning must win {round(d['strict'] * 100)}% "
            f"of the energies; unused lines last {d['patience']} s; replays {round(d['replay'] * 100)}% of practice. "
            f"It ended at {a['warmth']}x warmth, {a['orderPull']}x order pull.")


def command(args):
    if args[:1] and args[0] in ("always", "off", "status"):
        from trainer import daemon                       # the training daemon (a systemd user service)
        return daemon.command(args[0])
    if args[:1] == ["repair"]:
        from trainer import repair                       # find what's missing in a program and fix it
        return repair.command(args[1:])
    if args[:1] == ["words"]:
        from trainer import wordfeed                     # new words from the internet dictionary, made into bricks
        got = wordfeed.feed(int(args[1]) if len(args) > 1 and args[1].isdigit() else 20, say=lambda s: print(s, flush=True))
        return 0 if got else 1
    if args[:1] and args[0] in ("learn", "learned", "forget", "score", "build", "bricks"):
        from trainer import intuition                    # learning by itself (Ni + Ne), checked by brick's compiler
        return intuition.command(args)
    line, long, brain, listing = split_args(args)
    if listing:
        if not shutil.which("node"):
            print("The imagination neurons run with node, and node isn't installed here.")
            return 1
        for a in archetypes():
            print(f"{a['name'].ljust(26)}{a['type']}  {a['bias'].ljust(8)}{'yours' if a['yours'] else ''}")
        return 0
    if not line:
        print(__doc__)
        return 1
    print("Imagination neurons (view only: brick still builds the line its own way)")
    print(f"Training on brick's sentence pool and examples{' with ' + brain if brain else ''} "
          f"({300 if long else 150} s of simulated time)...", flush=True)
    out = read_line(line, brain, long)
    if out.get("error"):
        print(out["error"])
        return 1
    print(f"{out['practised']} directions, {out['groups']} meaning groups from brick's universe and dictionary: "
          f"{out['neurons']} neurons, {out['links']} lines.")
    if out["archetype"]:
        print(describe(out["archetype"]))
    print()
    width = max(len(r["word"]) for r in out["rows"]) + 2
    print(f"{'word'.ljust(width)}{'meaning it found'.ljust(26)}{'how sure'.ljust(30)}senses")
    for r in out["rows"]:
        print(f"{r['word'].ljust(width)}{r['meaning'].ljust(26)}{r['sure'].ljust(30)}{r['senses']}")
    if out["split"]:
        print(f"\nWords that split into separate meanings while training: {', '.join(out['split'])}")
    if out["small"]:
        print(f"Left to brick's grammar (small joining words, no neuron): {', '.join(out['small'])}")
    return 0
