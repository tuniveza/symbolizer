"""Every word its own universe: all of English, each word linked to every word it can mean, and its brick worked
out from those links.

A word's universe holds:
  meaning      its commonest meanings (Wiktionary), and its parts of speech
  same as      words that can mean the same (Wiktionary's synonyms, WordNet's same-meaning sets, ConceptNet)
  kind of      what it is a kind of (WordNet, ConceptNet)
  goes with    words it is linked to in everyday English (ConceptNet)
  form of      the word it is a form, spelling or slip of ('placed', 'colour', 'pirnt')
  brick        what it is to brick: an action (write, wait, clear ...), a colour, a place, a time, a count,
               chatter, a joining word, or words to show; and how sure, and why
  links        its universal links: WordNet's meaning ids, ConceptNet's /c/en/<word>, its Wiktionary page

The bricks come from brick's own words (its actions, colours, places, the dictionary, the lexicon), then spread
along the links: a word whose same-meaning words are mostly 'write' bricks ('place', 'position', 'put',
'display' ...) is read as write too, if it can be a verb and the links clearly agree. A word's universe can't
overrule brick's own words or the words you teach; the reader asks it only about words nothing else knows.

All of it is one SQLite file, trainer/data/words.sqlite (made from the sources, not kept in git):

    python3 -m trainer.words build        make every word's universe (a few minutes)
    python3 -m trainer.words <word>       show one word's universe, in the style of trainer/data/universe.txt
"""
import json
import os
import re
import sqlite3
import sys
from collections import defaultdict

HERE = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
DB = os.path.join(HERE, "trainer", "data", "words.sqlite")
CONCEPTNET = os.path.join(HERE, "trainer", "data", "sources", "conceptnet.tsv")
WORD = re.compile(r"[a-z][a-z'-]*( [a-z][a-z'-]*){0,2}")
# how much each kind of link counts when bricks spread (a slip or form is the same word; a synonym nearly)
WEIGHT = {"form": 4.0, "same": 2.0, "kind": 1.0, "similar": 1.0, "goes": 0.25}
SURE = 4.0                      # a brick is taken when its links add up to this much, and twice any other
POS_FOR = {"action": {"verb"}, "colour": {"adj", "noun"}, "place": {"adv", "prep", "adj", "noun"},
           "time": {"noun"}, "count": {"adv"}}

_db = None


# ---------------- building ----------------

def seeds():
    """word -> (brick, why) from what brick knows itself: its actions, the dictionaries, the lexicon, the universe."""
    import actions
    import dictionary
    from trainer import universe
    found = {}
    for w in actions.STARTERS:
        found[w] = ("action:" + w, "brick's own action")
    for name in actions.COLOURS:
        found[name] = ("colour", "brick's own colour")
    for e in dictionary.entries():
        kind = {"action": "action:" + e.meaning.split()[0].lower(), "colour": "colour", "place": "place",
                "time": "time", "loop": "count"}.get(e.kind)
        if kind and e.key not in found:
            found[e.key] = (kind, "the lexicon" if getattr(e, "lexicon", False) else "brick's dictionary")
    world = universe.universe()
    for w, ideas in world.words.items():
        if w in found:
            continue
        for idea, kind in (("show", "action:write"), ("wait", "action:wait"), ("clear", "action:clear"),
                           ("colour", "colour"), ("place", "place"), ("time unit", "time"), ("chatter", "chatter"),
                           ("filler", "chatter"), ("saying", "saying"), ("window", "thing"), ("background", "thing"),
                           ("screen", "thing"), ("text", "thing"), ("glue", "joining")):
            if idea in ideas:
                found[w] = (kind, "brick's universe")
                break
    return found


def build(say=print):
    from trainer import lexicon, wordnet
    words = defaultdict(lambda: {"pos": set(), "meaning": [], "same": {}, "kind": {}, "goes": {}, "form": "",
                                 "similar": {}, "ids": set(), "verb": set()})   # verb: links from its verb meanings

    def link(a, b, kind, weight=1.0):
        if a != b and WORD.fullmatch(a) and WORD.fullmatch(b):
            words[a][kind][b] = max(words[a][kind].get(b, 0), weight)

    # Wiktionary: meanings, parts of speech, synonyms, forms
    n = 0
    for word, pos, tags, glosses, of in lexicon.wiki_entries():
        w = word.lower()
        if not WORD.fullmatch(w):
            continue
        u = words[w]
        u["pos"].add(pos)
        if glosses and len(u["meaning"]) < 3:
            u["meaning"].append(f"({pos}) {glosses[0][:120]}")
        if of and WORD.fullmatch(of.lower()) and of.lower() != w:
            u["form"] = u["form"] or of.lower()
        n += 1
    with open(lexicon.DIGEST, encoding="utf-8") as f:
        for line in f:
            parts = line.rstrip("\n").split("\t")
            if len(parts) >= 6 and parts[5]:
                w = parts[0].lower()
                for other in parts[5].split("|"):
                    link(w, other.lower(), "same")
                    link(other.lower(), w, "same")
                    if parts[1] == "verb":
                        words[w]["verb"].add(other.lower())
                        words[other.lower()]["verb"].add(w)
    say(f"words: {n} Wiktionary entries read")
    # WordNet: same-meaning sets and what each is a kind of, for every word in it
    for pos in "nvar":
        path = os.path.join(wordnet.WORDNET, "index." + wordnet.FILES[pos])
        with open(path, encoding="utf-8") as f:
            lemmas = [l.split(" ", 1)[0] for l in f if not l.startswith(" ")]
        for lemma in lemmas:
            w = lemma.replace("_", " ").lower()
            if not WORD.fullmatch(w):
                continue
            words[w]["pos"].add({"n": "noun", "v": "verb", "a": "adj", "r": "adv"}[pos])
            for rank, offset in enumerate(wordnet.senses(lemma, pos)[:3]):
                words[w]["ids"].add(f"{pos}{offset}")
                names, pointers = wordnet.synset(offset, pos)
                for other in names:
                    link(w, other.lower(), "same", 1.0 if rank == 0 else 0.6)
                    if pos == "v":
                        words[w]["verb"].add(other.lower())
                if rank == 0:
                    for symbol, target, tpos in pointers:
                        if symbol in ("@", "@i") and tpos == pos:
                            for other in wordnet.synset(target, tpos)[0][:3]:
                                link(w, other.lower(), "kind")
                                if pos == "v":
                                    words[w]["verb"].add(other.lower())
                        elif symbol == "&" and tpos in "as":
                            for other in wordnet.synset(target, tpos)[0][:3]:
                                link(w, other.lower(), "similar")
    say(f"words: WordNet read; {len(words)} words so far")
    # ConceptNet: synonyms, forms, kinds, and the words each goes with
    if os.path.isfile(CONCEPTNET):
        with open(CONCEPTNET, encoding="utf-8") as f:
            for line in f:
                if line.startswith("#"):
                    continue
                parts = line.rstrip("\n").split("\t")
                if len(parts) != 4:
                    continue
                a, rel, b, weight = parts[0].lower(), parts[1], parts[2].lower(), float(parts[3] or 1)
                if rel == "Synonym":
                    link(a, b, "same", min(1.0, weight))
                    link(b, a, "same", min(1.0, weight))
                elif rel == "FormOf":
                    if WORD.fullmatch(b) and not words[a]["form"]:
                        words[a]["form"] = b
                elif rel == "IsA":
                    link(a, b, "kind", min(1.0, weight / 2))
                elif rel == "SimilarTo":
                    link(a, b, "similar", min(1.0, weight))
                elif rel in ("RelatedTo", "UsedFor", "PartOf", "HasA"):
                    link(a, b, "goes", min(1.0, weight / 2))
    say(f"words: ConceptNet read; {len(words)} words in all")
    # the brain's own links: pairs it proved mean the same (as strong as a synonym), words it saw doing the
    # same job in a line, and the words its neurons hold together (trainer/intuition.py, trainer/symbols.py)
    from trainer import intuition
    proved = 0
    for a, links in intuition.evidence().items():
        for b, level, weight in links:
            a_, b_ = a.lower(), b.lower()
            if level == "meaning":
                link(a_, b_, "same", 1.0)
                words[a_]["verb"].add(b_)
                proved += 1
            elif level == "role":
                link(a_, b_, "similar", 1.0)
            elif weight >= 0.5:
                link(a_, b_, "goes", min(1.0, weight) * 0.5)
    try:
        from trainer import symbols
        folder = os.path.dirname(symbols.path_of("x"))
        for name in os.listdir(folder):
            if name.endswith(".txt") and not name.startswith(("_", "context-")):
                w = name[:-4]
                for value, facet, weight in symbols.links_for(w):
                    if isinstance(value, str) and not value.startswith("context-"):
                        link(w, value.lower(), "same" if facet in ("same as", "like") else "kind" if facet == "is"
                             else "goes", weight)
    except Exception:
        pass
    say(f"words: the brain's links added ({proved} proved to mean the same)")
    bricks = spread(words, seeds(), say)
    write(words, bricks, say)
    return len(words)


# brick actions that are one action in different words: links to any of them count for the same brick
SAME_ACTION = {"say": "write", "show": "write", "print": "write", "display": "write", "output": "write",
               "tell": "write", "type": "write", "place": "put", "position": "put", "pause": "wait",
               "sleep": "wait", "erase": "clear", "empty": "clear", "quit": "stop", "end": "stop", "exit": "stop",
               "leave": "stop", "create": "make", "store": "save", "keep": "save", "push": "add",
               "append": "add", "increase": "add", "subtract": "take", "decrease": "take", "go": "move"}


# only actions and colours spread along links: places, times, counts and joining words are a small closed set
# ('up', 'back', 'in' must stay words, or 'level up' and 'welcome back' lose their words)
SPREADS = re.compile(r"action:|colour$")


def canon(brick):
    if brick.startswith("action:"):
        return "action:" + SAME_ACTION.get(brick[7:], brick[7:])
    return brick


def fits(brick, pos):
    """Can a word with these parts of speech be this kind of brick? (an action must be able to be a verb)"""
    head = brick.split(":")[0]
    need = POS_FOR.get(head)
    return not need or not pos or bool(need & pos)


def spread(words, seeded, say=print):
    """Bricks for every word: brick's own first, then twice along the links, a word taking the brick its links
    clearly agree on. -> {word: (brick, how sure, why)}"""
    bricks = {w: (b, 9.0, why) for w, (b, why) in seeded.items()}
    for round_ in range(2):
        new = {}
        for w, u in words.items():
            if w in bricks:
                continue
            votes, why = defaultdict(float), defaultdict(list)
            if u["form"] and u["form"] in bricks:
                b = canon(bricks[u["form"]][0])
                votes[b] += WEIGHT["form"]
                why[b].append(f"a form of {u['form']}")
            for kind in ("same", "kind", "similar", "goes"):
                for other, weight in u[kind].items():
                    if other in bricks and bricks[other][1] >= SURE:
                        b = canon(bricks[other][0])
                        if b.startswith("action:") and other not in u["verb"]:
                            continue          # an action counts only through the word's verb meanings
                        votes[b] += WEIGHT[kind] * weight * (1.0 if round_ == 0 else 0.6)
                        if len(why[b]) < 4:
                            why[b].append(f"{'same as' if kind == 'same' else kind} {other}")
            ranked = sorted(((v, b) for b, v in votes.items() if fits(b, u["pos"]) and SPREADS.match(b)), reverse=True)
            if not ranked:
                continue
            top, b = ranked[0]
            second = ranked[1][0] if len(ranked) > 1 else 0
            if top >= SURE and top >= 1.5 * second:
                new[w] = (b, round(top, 2), "; ".join(why[b]))
        bricks.update(new)
        say(f"words: round {round_ + 1}: {len(new)} more words given a brick by their links")
    return bricks


def write(words, bricks, say=print):
    tmp = DB + ".tmp"
    if os.path.exists(tmp):
        os.remove(tmp)
    con = sqlite3.connect(tmp)
    con.execute("create table words (word text primary key, pos text, brick text, sure real, why text, "
                "meaning text, same text, kind text, goes text, form text, ids text)")
    top = lambda d, n: [k for k, _ in sorted(d.items(), key=lambda kv: -kv[1])[:n]]
    rows = []
    for w, u in words.items():
        b = bricks.get(w, ("words", 0.0, "nothing links it to a brick: words to show"))
        rows.append((w, ",".join(sorted(u["pos"])), b[0], b[1], b[2], json.dumps(u["meaning"]),
                     json.dumps(top(u["same"], 16)), json.dumps(top(u["kind"], 8)), json.dumps(top(u["goes"], 10)),
                     u["form"], json.dumps(sorted(u["ids"])[:12])))
    con.executemany("insert or replace into words values (?,?,?,?,?,?,?,?,?,?,?)", rows)
    con.commit()
    con.close()
    os.replace(tmp, DB)
    counted = defaultdict(int)
    for r in rows:
        counted[r[2].split(":")[0]] += 1
    say(f"words: {len(rows)} universes written to {os.path.relpath(DB, HERE)}; bricks: {dict(counted)}")


# ---------------- reading ----------------

def universe_of(word):
    """One word's universe as a dict, or None."""
    global _db
    if _db is None:
        if not os.path.isfile(DB):
            return None
        _db = sqlite3.connect(f"file:{DB}?mode=ro", uri=True, check_same_thread=False)
    row = _db.execute("select * from words where word = ?", (word.lower(),)).fetchone()
    if not row:
        return None
    keys = ("word", "pos", "brick", "sure", "why", "meaning", "same", "kind", "goes", "form", "ids")
    u = dict(zip(keys, row))
    for k in ("meaning", "same", "kind", "goes", "ids"):
        u[k] = json.loads(u[k])
    u["links"] = {"wordnet": u["ids"], "conceptnet": "/c/en/" + u["word"].replace(" ", "_"),
                  "wiktionary": "https://en.wiktionary.org/wiki/" + u["word"].replace(" ", "_")}
    return u


def brick_of(word):
    """(brick, how sure) for a word, from its universe; None when there is none or it's words to show."""
    if os.environ.get("BRICK_WORDS") == "off":
        return None
    u = universe_of(word)
    if not u or u["brick"] == "words":
        return None
    return u["brick"], u["sure"]


def shown(word):
    u = universe_of(word)
    if not u:
        return f"# {word}: no universe (not a word in WordNet, Wiktionary or ConceptNet)"
    lines = [f"# {u['word']}: its universe (made by `python3 -m trainer.words build`)",
             f"brick: {u['brick']}" + (f" (sure {u['sure']}: {u['why']})" if u['why'] else ""),
             f"parts of speech: {u['pos']}"]
    lines += [f"meaning: {m}" for m in u["meaning"]]
    for key, name in (("same", "same as"), ("kind", "kind of"), ("goes", "goes with")):
        if u[key]:
            lines.append(f"{name}: {', '.join(u[key])}")
    if u["form"]:
        lines.append(f"form of: {u['form']}")
    lines.append(f"links: WordNet {' '.join(u['links']['wordnet']) or '-'} · ConceptNet {u['links']['conceptnet']} · "
                 f"{u['links']['wiktionary']}")
    return "\n".join(lines)


if __name__ == "__main__":
    if len(sys.argv) > 1 and sys.argv[1] == "build":
        build()
    elif len(sys.argv) > 1:
        print(shown(" ".join(sys.argv[1:])))
    else:
        print(__doc__)
