"""The whole English dictionary inside the digital brain: every word Wiktionary knows (about 1.4 million).

The brain's practice (trainer/links.py) meets only the words its archetypes happen to say. Every other word is
known from the dictionary: what kind of word it is (its parts of speech), which brick it leads to (the lexicon's
test-built actions, colours, places, times, counts) and the word it is a form or slip of ('waited' -> wait,
'pirnt' -> print). When the brain reads a word it has rarely met in practice, it reads it the dictionary's way:
a form or slip borrows the links of its own word; any other word gets the roles its dictionary entry allows.

Sources (trainer/data/sources/wiktionary/english.tsv, made by `python3 -m trainer.lexicon`), texts/dictionary.txt,
texts/dictionary-lexicon.txt, trainer/data/lexicon.tsv. Result: trainer/data/dictionary-brain.db (sqlite, one row per
word, read a word at a time so the brain doesn't load 1.4 million words to read one line).

    python3 -m trainer.dictbrain build       make the database again (about a minute)
    python3 -m trainer.dictbrain WORD ...    what the brain knows about each word from the dictionary
"""
import os
import sqlite3
import sys
from collections import defaultdict
from functools import lru_cache

HERE = os.path.dirname(os.path.abspath(__file__))
ROOT = os.path.dirname(HERE)
DB = os.path.join(HERE, "data", "dictionary-brain.db")
DIGEST = os.path.join(HERE, "data", "sources", "wiktionary", "english.tsv")
KINDS = os.path.join(HERE, "data", "lexicon.tsv")

# part of speech -> the roles a word of that kind plays in a direction (W words to show, X filler)
POS_ROLES = {
    "noun": "W", "name": "W", "adj": "W", "pron": "W", "phrase": "W", "proverb": "W", "intj": "W",
    "num": "N", "verb": "W", "adv": "X", "prep": "X", "prep_phrase": "X", "conj": "X", "det": "X",
    "article": "X", "particle": "X", "contraction": "W",
}
ODD = {"archaic", "obsolete", "dialectal", "historical", "rare", "Early", "Middle"}   # forms the brain ignores
KIND_ROLE = {"colour": "C", "place": "P", "time": "T", "loop": "N", "action": "V"}


def _bricks():
    """brick action word -> the brain's brick ('show' -> write), from the brain's practice links."""
    from trainer import links
    lk = links.Links()
    names = lk.bricks()

    def brick_of(word):
        if word in names:
            return word
        if lk.words.get(word, 0) >= 3:
            best = max(names, key=lambda b: lk.s("w:" + word, "b:" + b))
            return best if lk.s("w:" + word, "b:" + best) > 0 else ""
        return ""
    return brick_of


def build(say=print):
    import dictionary
    roles = defaultdict(lambda: defaultdict(float))     # word -> role -> how many entries say so
    base, brick = {}, {}
    forms = defaultdict(lambda: defaultdict(float))     # word -> word it's a form of -> entries that say so
    entries = defaultdict(float)
    n = 0
    with open(DIGEST, encoding="utf-8") as f:
        for line in f:
            parts = line.rstrip("\n").split("\t")
            if len(parts) < 5:
                continue
            word = parts[0].lower()
            if " " in word or len(word) > 30:
                continue
            role = POS_ROLES.get(parts[1])
            if role:
                roles[word][role] += 1.0
            entries[word] += 1.0
            of = parts[4].lower().strip()
            tags = set(parts[2].split(","))
            if of and of != word and " " not in of and not tags & ODD:
                forms[word][of] += 1.0
            n += 1
    for word, targets in forms.items():                 # a form only when most of the word's entries say so
        of, k = max(targets.items(), key=lambda x: x[1])
        if k / entries[word] >= 0.5:
            base[word] = of
    from actions import COLOURS
    for c in COLOURS:
        if " " not in c:
            roles[c.lower()]["C"] = max(roles[c.lower()].values(), default=1.0) * 2
    brick_of = _bricks()
    # brick's own dictionary first, then the lexicon: what each word is to brick (overrides the part of speech)
    for path in (dictionary.EXTRA, dictionary.PATH):        # PATH last: the hand-made words win
        for e in dictionary.read(path):
            key = e.key.lower()
            if " " in key:
                continue
            role = KIND_ROLE.get(e.kind)
            if not role:
                continue
            roles[key] = defaultdict(float, {role: 3.0})
            if role == "V":
                b = brick_of(e.meaning.split()[0].lower())
                if b:
                    brick[key] = b
            elif role != "C" and e.meaning.lower() != key and " " not in e.meaning:
                base.setdefault(key, e.meaning.lower())
    from actions import STARTERS                          # brick's own action words: always actions
    for w in STARTERS:
        roles[w.lower()]["V"] = max(roles[w.lower()].values(), default=1.0) * 2
        b = brick_of(w.lower())
        if b:
            brick[w.lower()] = b
    if os.path.isfile(KINDS):
        with open(KINDS, encoding="utf-8") as f:
            for line in f:
                p = line.rstrip("\n").split("\t")
                if len(p) == 2 and not line.startswith("#") and p[1] in ("chatter", "joining"):
                    roles[p[0].lower()] = defaultdict(float, {"X": 3.0})
    tmp = DB + ".new"
    if os.path.exists(tmp):
        os.remove(tmp)
    con = sqlite3.connect(tmp)
    con.execute("CREATE TABLE words (word TEXT PRIMARY KEY, roles TEXT, brick TEXT, base TEXT)")
    rows = []
    for word in set(roles) | set(base):
        r = roles.get(word, {})
        top = max(r.values()) if r else 0
        text = ",".join(f"{k}:{v / top:.2f}" for k, v in sorted(r.items(), key=lambda x: -x[1])) if top else ""
        rows.append((word, text, brick.get(word, ""), base.get(word, "")))
    con.executemany("INSERT INTO words VALUES (?,?,?,?)", rows)
    con.commit()
    con.close()
    os.replace(tmp, DB)
    _con.cache_clear()
    entry.cache_clear()
    say(f"The brain's dictionary: {len(rows):,} words from {n:,} Wiktionary entries "
        f"({len(brick):,} lead to a brick, {len(base):,} are forms or slips of another word) -> {DB}")
    return len(rows)


@lru_cache(maxsize=1)
def _con():
    if not os.path.isfile(DB):
        return None
    return sqlite3.connect(f"file:{DB}?mode=ro", uri=True, check_same_thread=False)


@lru_cache(maxsize=50000)
def entry(word):
    """word -> ({role: weight 0..1}, brick, base word) from the dictionary, or None if the dictionary hasn't it."""
    con = _con()
    if con is None:
        return None
    row = con.execute("SELECT roles, brick, base FROM words WHERE word = ?", (word.lower(),)).fetchone()
    if not row:
        return None
    roles = {}
    for part in filter(None, row[0].split(",")):
        k, v = part.split(":")
        roles[k] = float(v)
    return roles, row[1], row[2]


def size():
    con = _con()
    return con.execute("SELECT COUNT(*) FROM words").fetchone()[0] if con else 0


if __name__ == "__main__":
    args = sys.argv[1:]
    if args[:1] == ["build"]:
        build()
    elif args:
        for w in args:
            print(w, entry(w))
    else:
        print(__doc__)
