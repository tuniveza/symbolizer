"""The whole English dictionary as bricks: every word Wiktionary and WordNet know, given the brick it means.

Two dictionaries, both free (trainer/data/sources):
  WordNet     the English dictionary sorted by meaning: 'shout' is a kind of 'utter'
  Wiktionary  the biggest English dictionary (over a million entries, slang, misspellings, abbreviations,
              grammar words and punctuation), from kaikki.org's extract; fetched once (`fetch`)

From each word's meaning a brick is formed:
  actions     a verb whose meaning leads to something brick can do: WordNet's tree ('shout' -> 'utter' ->
              write, from one exact meaning of each brick, never a word that only looks the same), then
              Wiktionary's definitions ('holler: to shout loudly' -> shout -> write), again and again until
              nothing new is found. Each one is test-built: 'holler hello' must build exactly what
              'write hello' builds, or it is left out.
  colours     'cerulean: a deep blue colour' -> blue
  places      'atop: on top of' -> top
  times       'fortnight: a period of two weeks'
  counts      'thrice: three times'
  slips       a misspelling, other spelling or abbreviation of a word brick knows reads as that word
              ('pirnt' -> print, 'bg' -> background, 'secs' -> seconds)
  chatter     interjections of hesitation and laughter ('erm', 'lmao'): never shown
  joining     conjunctions and punctuation that join clauses

Everything brick knows already comes first (its own words, the hand-made dictionary, your words), and a word is
given only the one brick its commonest meaning leads to. Words that are mostly words to show ('welcome',
'level', 'game') are never made orders. The rest (most nouns and describing words) stay words to show, which
is what they are in a line.

The results:
  texts/dictionary-lexicon.txt     actions, colours, places, times, counts, slips (read after dictionary.txt)
  trainer/data/lexicon.tsv         chatter, joining words and slips of the universe's words (bg -> background)
  trainer/data/lacking-actions.txt verbs whose meaning leads to nothing brick can do yet

    python3 -m trainer.lexicon fetch      download Wiktionary's English (about 3.4 GB)
    python3 -m trainer.lexicon            make the lexicon again, and say how it went
"""
import json
import os
import re
import sys
import tempfile

HERE = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
OUT = os.path.join(HERE, "texts", "dictionary-lexicon.txt")
KINDS = os.path.join(HERE, "trainer", "data", "lexicon.tsv")
LACKING = os.path.join(HERE, "trainer", "data", "lacking-actions.txt")
WIKI = os.path.join(HERE, "trainer", "data", "sources", "wiktionary")
WIKI_URL = "https://kaikki.org/dictionary/English/kaikki.org-dictionary-English.jsonl"
DIGEST = os.path.join(WIKI, "english.tsv")        # the parts of Wiktionary the lexicon reads, one line per entry

# The brick actions a verb can be taken to, and a line that shows each working (the verb goes first).
TRIES = {"write": "{} hello", "wait": "{} 2 seconds", "clear": "{} the screen", "ask": "{} your name",
         "stop": "{}", "beep": "{}", "save": "{} score to best.txt", "load": "{} best.txt as best",
         "delete": "{} best.txt", "repeat": "{} 3 times: write hi", "count": "{} to 10"}
# Each brick action as exact WordNet meanings (synset offsets), so 'ring' the sound leads to beep, never
# 'ring' the circle, and 'clear' a desk leads to clear, never 'clear' a manuscript for printing.
SENSES = {
    "write": [1009240, 940384, 941990, 983824, 2140033, 2148788, 1745722, 1746715, 953216, 945255, 1703454,
              974367, 965871, 977336, 1001008, 993014, 2137132],
    "wait": [2637938, 2641035, 779061, 779360, 14742, 2058590, 2011560, 2417504],
    "clear": [181664, 1548718, 1392237],
    "ask": [784342, 784727, 785008, 788184, 1069809],
    "stop": [2680814, 1860795, 352826],
    "beep": [2183175, 2182109, 2182342, 2180898],
    "save": [2225492, 2281093, 2282506],
    "load": [2231910, 625119, 627520],
    "delete": [1549187, 999815, 478830],
    "repeat": [1734502],
    "count": [948071, 947857, 946105],
}
# Words that are much more often the words to show than an order ('welcome back', 'level up', 'see you soon').
SHOWN = {"welcome", "level", "try", "see", "go", "let", "nice", "happy", "hello", "hi", "well", "done", "game",
         "good", "morning", "win", "ready", "soon", "over", "back", "up", "days", "one", "there", "lets", "bye",
         "thank", "thanks", "love", "like", "hope", "wish", "please", "sorry", "play", "run", "jump", "yes", "no",
         "great", "cool", "wow", "yay", "hooray", "oops", "ok", "okay", "hey", "goodbye", "cheers", "congrats"}
HELPERS = {"be", "become", "make", "cause", "have", "get", "give", "do", "put", "take", "go", "come", "use",
           "provide", "act", "engage", "perform", "subject", "allow", "let", "keep", "set", "bring", "begin",
           "try", "attempt", "continue", "start", "one's", "oneself", "someone", "something", "or", "and", "a",
           "an", "the", "in", "into", "with", "as", "by", "of", "to", "up", "out", "off", "very", "repeatedly"}
NUMBERS = {"one": 1, "two": 2, "three": 3, "four": 4, "five": 5, "six": 6, "seven": 7, "eight": 8, "nine": 9,
           "ten": 10, "twelve": 12, "fourteen": 14, "twenty": 20, "thirty": 30, "sixty": 60, "hundred": 100}
UNITS = {"second": "seconds", "seconds": "seconds", "minute": "minutes", "minutes": "minutes",
         "hour": "hours", "hours": "hours", "day": "days", "days": "days", "week": "weeks", "weeks": "weeks"}
SIDE_WORDS = {"top": "top", "above": "top", "upper": "top", "uppermost": "top", "topmost": "top", "highest": "top",
              "bottom": "bottom", "below": "bottom", "beneath": "bottom", "under": "bottom", "underneath": "bottom",
              "lowest": "bottom", "bottommost": "bottom", "left": "left", "leftward": "left", "right": "right",
              "rightward": "right", "middle": "middle", "centre": "middle", "center": "middle"}
CHATTER_GLOSS = re.compile(r"\b(hesitat|filler|pause|uncertain|laugh|amusement|boredom|indifferen|"
                           r"thinking|unsure|doubt|gap in speech)", re.I)
JOINING_GLOSS = re.compile(r"\b(and then|afterwards?|next|subsequently|in addition)\b", re.I)
SLIP_TAGS = {"misspelling", "alternative", "abbreviation", "clipping", "contraction", "nonstandard",
             "eye-dialect", "pronunciation-spelling", "informal", "acronym", "initialism", "obsolete-spelling"}


# ---------------- Wiktionary ----------------

def fetch(say=print):
    """Download Wiktionary's English from kaikki.org (once), and digest it."""
    import urllib.request
    os.makedirs(WIKI, exist_ok=True)
    whole = os.path.join(WIKI, "english.jsonl")
    if not os.path.isfile(whole):
        say("Downloading Wiktionary's English (about 3.4 GB) ...")
        request = urllib.request.Request(WIKI_URL, headers={"User-Agent": "brick-lexicon"})
        with urllib.request.urlopen(request) as answer, open(whole + ".part", "wb") as f:
            while True:
                block = answer.read(1 << 20)
                if not block:
                    break
                f.write(block)
        os.replace(whole + ".part", whole)
    digest(say)


def digest(say=print):
    """english.jsonl -> english.tsv: word, part of speech, tags, the first meanings, the word it's a form of."""
    whole = os.path.join(WIKI, "english.jsonl")
    files = [whole] if os.path.isfile(whole) else \
        [os.path.join(WIKI, n) for n in ("verbs.jsonl", "adverbs.jsonl") if os.path.isfile(os.path.join(WIKI, n))]
    n = 0

    def lines():
        for name in files:
            with open(name, encoding="utf-8") as f:
                yield from f
    with open(DIGEST + ".tmp", "w", encoding="utf-8") as out:
        for line in lines():
            try:
                entry = json.loads(line)
            except ValueError:
                continue
            word, pos = entry.get("word", ""), entry.get("pos", "")
            if not word or "\t" in word or "\n" in word or len(word) > 40:
                continue
            glosses, tags, of = [], set(), ""
            for sense in entry.get("senses", [])[:6]:
                st = set(sense.get("tags", []))
                if st & {"obsolete", "archaic", "rare", "dated", "historical"} and glosses:
                    continue
                for g in sense.get("glosses", [])[:1]:
                    glosses.append(g.replace("\t", " ").replace("\n", " ")[:200])
                tags |= st
                for key in ("alt_of", "form_of"):
                    for target in sense.get(key, [])[:1]:
                        of = of or target.get("word", "")
                if len(glosses) >= 3:
                    break
            if not glosses and not of:
                continue
            out.write("\t".join([word, pos, ",".join(sorted(tags)), " || ".join(glosses), of]) + "\n")
            n += 1
    os.replace(DIGEST + ".tmp", DIGEST)
    say(f"Wiktionary digested: {n} entries")


def wiki_entries():
    """[(word, pos, tags, [meanings], the word it's a form of)] from the digest."""
    if not os.path.isfile(DIGEST):
        return []
    out = []
    with open(DIGEST, encoding="utf-8") as f:
        for line in f:
            parts = line.rstrip("\n").split("\t")
            if len(parts) == 5:
                out.append((parts[0], parts[1], set(filter(None, parts[2].split(","))), parts[3].split(" || "),
                            parts[4]))
    return out


# ---------------- what brick knows already ----------------

def brick_words():
    """word -> what it is to brick now ('action:write', 'colour', 'place', ...), from brick's own words, the
    hand-made dictionary (not the lexicon's) and the universe."""
    import actions
    import dictionary
    from trainer import universe
    have = {}
    for w in actions.STARTERS:
        have[w] = "action:" + w
    for e in dictionary.read(dictionary.PATH):
        if e.kind == "action":
            have.setdefault(e.key, "action:" + e.meaning.split()[0].lower())
        else:
            have.setdefault(e.key, e.kind)
    for w, ideas in universe.Universe(universe.PATH).words.items():
        have.setdefault(w, "universe:" + ",".join(sorted(ideas)))
    return have


def colour_names():
    import dictionary
    from actions import COLOURS
    names = set(COLOURS) | {e.key for e in dictionary.read(dictionary.PATH) if e.kind == "colour"}
    return {n for n in names if " " not in n}


# ---------------- WordNet: verbs by exact meaning ----------------

def wordnet_actions(have):
    """verb -> brick action, for WordNet's single-word verbs whose commonest meanings lead to one of brick's."""
    from trainer import wordnet
    exact = {off: action for action, offs in SENSES.items() for off in offs}
    found = {}
    with open(os.path.join(wordnet.WORDNET, "index.verb"), encoding="utf-8") as f:
        verbs = [l.split(" ", 1)[0] for l in f if not l.startswith(" ")]
    for verb in verbs:
        if not re.fullmatch(r"[a-z]{3,}", verb) or verb in have or verb in SHOWN:
            continue
        best = None
        for rank, offset in enumerate(wordnet.senses(verb, "v")[:2]):
            todo, seen = [(offset, 0)], set()
            while todo:
                at, steps = todo.pop(0)
                if at in seen:
                    continue
                seen.add(at)
                if int(at) in exact and (rank == 0 or steps == 0):
                    if best is None or (rank + steps) < best[0]:
                        best = (rank + steps, exact[int(at)])
                    break
                if steps < 2:
                    todo += [(t, steps + 1) for sym, t, pos in wordnet.synset(at, "v")[1] if sym in ("@", "@i") and pos == "v"]
        if best:
            found[verb] = best[1]
    return found


# ---------------- Wiktionary: bricks from meanings ----------------

ASPECT = {"stop", "end", "cease", "quit", "finish", "begin", "start", "keep", "continue", "resume", "wait"}


def head_verb(gloss):
    """'To utter (something) loudly.' -> 'utter'; None when the meaning doesn't start with a plain verb
    ('to cause ...', 'to stop ignoring', 'to wait for' are about another action, not this one)."""
    g = re.sub(r"\([^)]*\)", " ", gloss).lower()
    g = re.split(r"[;:,.]", g)[0]
    m = re.match(r"\s*to\s+(.*)", g)
    if not m:
        return None
    words = re.findall(r"[a-z'-]+", m.group(1))
    if not words or words[0] in HELPERS:
        return None
    if words[0] in ASPECT and len(words) > 1 and (words[1].endswith("ing") or words[1] in ("for", "on", "upon", "being")):
        return None
    return words[0]


def in_wordnet(word):
    from trainer import wordnet
    return bool(wordnet.senses(word, "v"))


def mostly_verb(word, senses):
    """Is the word mostly a verb? 'tripod', 'ammo', 'garage' are things that can also be verbs: never orders."""
    from trainer import wordnet
    v, n = len(wordnet.senses(word, "v")), len(wordnet.senses(word, "n"))
    if v or n:
        return v >= n
    return not any(s[0] == "noun" for s in senses)


def colour_of(gloss, colours, short_adj=False):
    """'A deep blue colour.' -> blue: the meaning is about a colour, and names one in its first few words
    ('a lesion with red ...' is not a colour)."""
    g = gloss.lower()
    if re.match(r"\s*(fear|not|without|lacking|other than|any|absence)\b", g):
        return None                                   # 'unred: not red', 'porphyrophobia: fear of purple'
    first = re.findall(r"[a-z]+", g)[:6]
    named = [w for w in first if w in colours]
    if not named:
        return None
    if re.search(r"\b(colou?r(ed)?|shade|hue|tint|tinge|tinged)\b", " ".join(first + re.findall(r"[a-z]+", g)[6:9])) or \
            short_adj and len(first) <= 4:
        return named[0]
    return None


def wiki_bricks(have, actions_found, say=print):
    """From Wiktionary's meanings: actions (chained), colours, places, times, counts, slips, chatter, joining."""
    colours = colour_names()
    entries = [e for e in wiki_entries() if re.fullmatch(r"[a-z][a-z'-]*[a-z]|[a-z]", e[0])]
    say(f"lexicon: reading {len(entries)} Wiktionary entries")
    out = {"colour": {}, "place": {}, "time": {}, "count": {}, "slip": {}, "uslip": {}, "chatter": set(),
           "joining": set()}
    by_word = {}
    for word, pos, tags, glosses, of in entries:
        by_word.setdefault(word, []).append((pos, tags, glosses, of))
    known = lambda w: w in have or w in actions_found
    # actions: a verb's first meaning starts with a verb that leads to a brick by itself (brick's own words or
    # WordNet's exact meanings): one step only, as every further step drifts ('lathe' -> shape -> ... -> ask)
    base = dict(actions_found)
    for _ in range(1):
        new = {}
        for word, senses in by_word.items():
            if known(word) or word in SHOWN or len(word) < 3:
                continue
            verbs = [s for s in senses if s[0] == "verb" and not s[3]]
            if not verbs or not verbs[0][2] or not mostly_verb(word, senses) or in_wordnet(word):
                continue                            # WordNet decides the verbs it knows; Wiktionary the rest
            head = head_verb(verbs[0][2][0])
            if not head or head == word:
                continue
            target = base.get(head) or (have.get(head, "")[7:] if have.get(head, "").startswith("action:") else None)
            if target in TRIES:
                new[word] = target
        if not new:
            break
        actions_found.update(new)
    for word, senses in by_word.items():
        if known(word) or word in SHOWN:
            continue
        first = senses[0]
        pos, tags, glosses, of = first
        # slips: a misspelling, other spelling or abbreviation of a word brick knows
        slipped = [s for s in senses if s[3] and (s[1] & SLIP_TAGS or s[0] in ("abbrev", "symbol"))]
        if slipped:
            target = slipped[0][3].lower()
            what = have.get(target)                 # only brick's own words: a slip of a slip drifts
            from trainer import universe as u
            if what and target != word and " " not in target and not u.real_word(word):   # 'member' is a word
                if what.startswith(("action:", "colour", "place", "time")):
                    out["slip"][word] = target
                elif what.startswith("universe:"):
                    out["uslip"][word] = target
                continue
        gloss = glosses[0] if glosses else ""
        if pos == "intj" and CHATTER_GLOSS.search(" ".join(glosses[:2])):
            out["chatter"].add(word)
            continue
        if pos in ("adj", "noun") and not re.match(r"(un|non|anti)", word):
            c = colour_of(gloss, colours, short_adj=pos == "adj")
            if c and c != word:
                out["colour"][word] = (c, any(s[0] == "noun" and not colour_of(s[2][0] if s[2] else "", colours)
                                              for s in senses))
                continue
        if pos in ("adv", "prep") and len(gloss.split()) <= 6 and len(word) <= 12 and \
                re.search(r"top|bottom|left|right|mid|cent|up|down|high|low|above|below|under|neath|over", word):
            m = re.match(r"\s*(?:on |at |to |towards? |in |at or )?(?:the |a )?(\w+)\b", gloss.lower())
            if m and m.group(1) in SIDE_WORDS:
                out["place"][word] = SIDE_WORDS[m.group(1)]
                continue
        if pos == "adv":
            m = re.match(r"\s*(\w+)\s+times\b", gloss.lower())
            if m and m.group(1) in NUMBERS:
                out["count"][word] = NUMBERS[m.group(1)]
                continue
            if JOINING_GLOSS.match(gloss.strip()) and len(gloss.split()) <= 4:
                out["joining"].add(word)
                continue
        if pos == "noun":
            m = re.search(r"\b(?:period|unit|space|interval) of (?:time )?(?:equal to |of )?(\w+) (\w+)", gloss.lower())
            if m and m.group(1) in NUMBERS and m.group(2) in UNITS:
                out["time"][word] = f"{NUMBERS[m.group(1)]} {UNITS[m.group(2)]}"
                continue
        if pos == "conj" and JOINING_GLOSS.search(gloss):
            out["joining"].add(word)
    return out


# ---------------- checking the actions ----------------

def builds_same(pairs):
    """[(verb, action)] -> the ones where '<verb> ...' builds exactly what '<action> ...' builds, with the verb
    taught as that action (in a scratch copy of the dictionary)."""
    import dictionary
    from compiler import Compiler
    from expressions import Problem
    keep = []
    saved_env = {k: os.environ.get(k) for k in ("BRICK_NEURAL", "BRICK_MEMORY", "BRICK_TRAINER")}
    with tempfile.TemporaryDirectory() as scratch:
        mine = os.path.join(scratch, "mywords.txt")
        saved, saved_extra = dictionary.PATH, dictionary.EXTRA
        trial = os.path.join(scratch, "dictionary.txt")
        with open(saved, encoding="utf-8") as f, open(trial, "w", encoding="utf-8") as g:
            g.write(f.read() + "\n# Trial\n" + "".join(f"{v} = {a}\n" for v, a in pairs))
        try:
            dictionary.PATH, dictionary.EXTRA = trial, None
            dictionary.clear()
            os.environ.update(BRICK_NEURAL="off", BRICK_MEMORY="off", BRICK_TRAINER="off")

            def build(text):
                try:
                    return Compiler(mine, ask_user=lambda q: "no").compile(text)
                except (Problem, Exception):
                    return None
            want = {a: build(t.format(a)) for a, t in TRIES.items()}
            for verb, action in pairs:
                got = build(TRIES[action].format(verb))
                if got is not None and got == want[action]:
                    keep.append((verb, action))
        finally:
            dictionary.PATH, dictionary.EXTRA = saved, saved_extra
            dictionary.clear()
            for k, v in saved_env.items():
                os.environ.pop(k, None) if v is None else os.environ.__setitem__(k, v)
    return keep


# ---------------- making it ----------------

def make(say=print):
    import dictionary
    os.environ["BRICK_LEXICON"] = "off"             # what brick knows by itself, without the last lexicon
    dictionary.clear()
    have = brick_words()
    acts = wordnet_actions(have)
    say(f"lexicon: WordNet: {len(acts)} verbs lead to a brick by exact meaning")
    wiki = wiki_bricks(have, acts, say)
    working = dict(builds_same(sorted(acts.items())))
    say(f"lexicon: {len(working)} of {len(acts)} actions build exactly what their brick builds")
    lines = ["# The whole English dictionary as bricks: made by `python3 -m trainer.lexicon` from WordNet and",
             "# Wiktionary. Every action here was test-built. Don't edit this file by hand: put your own words in",
             "# dictionary.txt (it comes first) or mywords.txt."]
    by_action = {}
    for v, a in working.items():
        by_action.setdefault(a, []).append(v)
    for a in sorted(by_action):
        lines += ["", f"# {a.capitalize()}: from the dictionaries"] + [f"{v} = {a}" for v in sorted(by_action[a])]
    plain = {w: c for w, (c, thing) in wiki["colour"].items() if not thing}
    things = {w: c for w, (c, thing) in wiki["colour"].items() if thing}
    if plain:
        lines += ["", "# Colours: from the dictionaries"] + [f"{w} = {c}" for w, c in sorted(plain.items())]
    if things:
        lines += ["", "# Colours that are also everyday things: from the dictionaries", "# (they need the word colour)"] + \
            [f"{w} = {c}" for w, c in sorted(things.items())]
    if wiki["place"]:
        lines += ["", "# Places: from the dictionaries"] + [f"{w} = {p}" for w, p in sorted(wiki["place"].items())]
    if wiki["time"]:
        lines += ["", "# Times: from the dictionaries"] + [f"{w} = {t}" for w, t in sorted(wiki["time"].items())
                                                             if t.split()[1] in ("seconds", "minutes")]
    if wiki["count"]:
        lines += ["", "# Counts: from the dictionaries"] + [f"{w} = {n} times" for w, n in sorted(wiki["count"].items())]
    slips = {w: t for w, t in wiki["slip"].items() if w not in working}
    if slips:
        lines += ["", "# Misspellings, other spellings and abbreviations: from Wiktionary"] + \
            [f"{w} = {t}" for w, t in sorted(slips.items())]
    with open(OUT + ".tmp", "w", encoding="utf-8") as f:
        f.write("\n".join(lines) + "\n")
    os.replace(OUT + ".tmp", OUT)
    with open(KINDS + ".tmp", "w", encoding="utf-8") as f:
        f.write("# word\twhat it is (made by `python3 -m trainer.lexicon`; read by the universe for words it doesn't know)\n")
        for w in sorted(wiki["chatter"]):
            f.write(f"{w}\tchatter\n")
        for w in sorted(wiki["joining"]):
            f.write(f"{w}\tjoin\n")
        for w, t in sorted(wiki["uslip"].items()):
            f.write(f"{w}\tsame:{t}\n")
    os.replace(KINDS + ".tmp", KINDS)
    lacking = sorted({w for w, senses in {e[0]: 1 for e in wiki_entries() if e[1] == "verb" and not e[4]}.items()
                      if w not in have and w not in working and re.fullmatch(r"[a-z]{3,}", w)})
    with open(LACKING, "w", encoding="utf-8") as f:
        f.write("# Verbs whose meaning leads to nothing brick can do yet: actions brick still lacks.\n")
        f.write("\n".join(lacking) + "\n")
    os.environ.pop("BRICK_LEXICON", None)
    dictionary.clear()
    report = {"actions": len(working), "by_action": {a: len(v) for a, v in by_action.items()},
              "colours": len(wiki["colour"]), "places": len(wiki["place"]), "times": len(wiki["time"]),
              "counts": len(wiki["count"]), "slips": len(slips) + len(wiki["uslip"]), "chatter": len(wiki["chatter"]),
              "joining": len(wiki["joining"]), "lacking": len(lacking)}
    say("lexicon: " + json.dumps(report))
    return report


if __name__ == "__main__":
    if len(sys.argv) > 1 and sys.argv[1] == "fetch":
        fetch()
    elif len(sys.argv) > 1 and sys.argv[1] == "digest":
        digest()
    else:
        make()
