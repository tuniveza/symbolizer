"""New words from the internet dictionary, learned by the imagination neurons, turned into bricks.

    brick trainer neurons words [count]      take that many new words (default 20) through the whole way

1. New words: the commonest English words (a public list of 10,000) that brick doesn't know yet.
2. The internet dictionary (dictionaryapi.dev, free, no key): each word's definitions, examples and synonyms.
   Each word is fetched once and kept in trainer/data/sources/dictionary-api/.
3. The neurons learn it: the definitions and examples become input (trainer/data/sources/public/definitions.txt),
   so Ni and Ne learn the word from how it is used, like any other sentence.
4. The brick: the neurons rank the bricks it could be (their lines, the role they learned, the dictionary's
   synonyms), and trainer/intuition.py's make_bricks keeps one only if it works precisely (its 6 checks).
5. Or a new one: when a definition is two of brick's actions joined ("to open and close quickly"), it makes a new
   action out of them ("to blink something means open something then close something"), checks it builds, and
   puts it in your Decisions list: a guess from a definition is never kept without you.

Only the word itself is sent to the dictionary; nothing about you or your programs.
"""
import json
import os
import re
import time
import urllib.parse
import urllib.request

from trainer import intuition, neurons

HERE = neurons.HERE
SOURCES = os.path.join(HERE, "trainer", "data", "sources")
CACHE = os.path.join(SOURCES, "dictionary-api")
WORDLIST = os.path.join(SOURCES, "public", "words-10000.list")      # .list, so it isn't read as sentences
DEFINITIONS = os.path.join(intuition.EXTRA, "definitions.txt")
LIST_URL = "https://raw.githubusercontent.com/first20hours/google-10000-english/master/google-10000-english-usa-no-swears.txt"
API = "https://api.dictionaryapi.dev/api/v2/entries/en/"
AGENT = {"User-Agent": "brick-trainer"}
PAUSE = 0.6                                                          # seconds between dictionary requests


def fetch(url):
    with urllib.request.urlopen(urllib.request.Request(url, headers=AGENT), timeout=30) as r:
        return r.read().decode("utf-8", "replace")


def word_list():
    if not os.path.isfile(WORDLIST):
        os.makedirs(os.path.dirname(WORDLIST), exist_ok=True)
        text = fetch(LIST_URL)
        with open(WORDLIST, "w", encoding="utf-8") as f:
            f.write(text)
    with open(WORDLIST, encoding="utf-8") as f:
        return [w.strip().lower() for w in f if re.fullmatch(r"[a-z]+", w.strip().lower())]


def entry(word):
    """The dictionary's entry for the word -> {"meanings": [{"pos", "definitions": [...], "examples": [...],
    "synonyms": [...]}]}, or None when it has none. Cached, so each word is asked once."""
    os.makedirs(CACHE, exist_ok=True)
    path = os.path.join(CACHE, word + ".json")
    if os.path.isfile(path):
        with open(path, encoding="utf-8") as f:
            return json.load(f) or None
    try:
        raw = json.loads(fetch(API + urllib.parse.quote(word)))
        time.sleep(PAUSE)
    except urllib.error.HTTPError as error:
        raw = [] if error.code == 404 else None
    except (urllib.error.URLError, OSError, ValueError):
        return None                                                  # offline: try again another time
    if raw is None:
        return None
    meanings = []
    for e in raw if isinstance(raw, list) else []:
        for m in e.get("meanings", []):
            defs = m.get("definitions", [])
            meanings.append({"pos": m.get("partOfSpeech", ""), "definitions": [d.get("definition", "") for d in defs],
                             "examples": [d["example"] for d in defs if d.get("example")],
                             "synonyms": list(dict.fromkeys(m.get("synonyms", []) + [s for d in defs for s in d.get("synonyms", [])]))})
    got = {"meanings": meanings} if meanings else {}
    with open(path, "w", encoding="utf-8") as f:
        json.dump(got, f)
    return got or None


def sentences_for(word, got):
    """The entry as plain sentences the neurons can read: definitions (said with the word) and examples."""
    out = []
    for m in got["meanings"]:
        for d in m["definitions"][:3]:
            d = d.rstrip(".").strip()
            if d:
                out.append(f"{word} means {d[0].lower() + d[1:]}")
        out += [x.strip() for x in m["examples"][:3]]
    return out


def synonyms(word):
    got = entry(word)
    return {s.lower() for m in (got or {}).get("meanings", []) for s in m["synonyms"] if " " not in s}


def agrees(new, known):
    """General English says they mean the same: WordNet both ways, or the internet dictionary both ways."""
    if intuition.same_meaning(new, known) and new in __import__("trainer.wordnet", fromlist=["synonyms"]).synonyms(known) \
            and known in __import__("trainer.wordnet", fromlist=["synonyms"]).synonyms(new):
        return True
    return known in synonyms(new) and new in synonyms(known)


def actions_in(definition):
    """brick actions in a definition, in order: 'to open and close quickly' -> ['open', 'close']."""
    return [w for w in neurons.words_of(definition) if intuition.role_of(w) == "do" and w not in neurons.SMALL]


def new_actions(word, got):
    """A definition made of two or more of brick's actions -> a new action built from them, if it builds."""
    made = []
    for m in got["meanings"]:
        if m["pos"] != "verb":
            continue
        for d in m["definitions"][:3]:
            acts = list(dict.fromkeys(actions_in(d)))
            if 2 <= len(acts) <= 3:
                line = f"to {word} something means " + " then ".join(f"{a} something" for a in acts)
                if intuition.builds(f"{line}\n{word} hello") is not None:
                    made.append((line, d))
    return made


def feed(count=20, rounds=2, say=print):
    """Take `count` new words the whole way. -> {"words", "defined", "bricks", "actions"}"""
    from trainer import outliers
    known = set()
    for name in os.listdir(CACHE) if os.path.isdir(CACHE) else []:
        known.add(name[:-5])
    try:
        words = word_list()
    except OSError as error:
        say(f"Couldn't get the word list: {error}")
        return None
    fresh = [w for w in words if w not in known and len(w) >= 3 and w not in neurons.SMALL and not intuition.brick_knows(w)][:count]
    if not fresh:
        say("Every word on the list has been looked up already.")
        return None
    say(f"Looking up {len(fresh)} new words in the internet dictionary: {', '.join(fresh)}")
    added, defined, actions = [], [], []
    for w in fresh:
        got = entry(w)
        if not got:
            continue
        defined.append(w)
        added += sentences_for(w, got)
        for line, why in new_actions(w, got):
            actions.append(line)
            outliers.add(w, f"the dictionary says '{why}': a new action made of brick's own", could_be=[line], source="the internet dictionary")
    if added:
        os.makedirs(os.path.dirname(DEFINITIONS), exist_ok=True)
        with open(DEFINITIONS, "a", encoding="utf-8") as f:
            f.write("".join(s + "\n" for s in added))
    say(f"{len(defined)} had entries: {len(added)} definitions and examples are now input for the neurons.")
    for w in defined:                                            # the dictionary's synonyms become suggestions to check
        for k in synonyms(w):
            if intuition.brick_knows(k) and intuition.role_of(k) in intuition.CONTENT:
                intuition.append(intuition.BRICKS, [f"{w} = {k}"])
    if rounds:
        intuition.learn(rounds, say=say)
    bricks = intuition.make_bricks(say=say, only=set(defined))
    if actions:
        say(f"New actions put in your Decisions list: {'; '.join(actions)}")
    return {"words": fresh, "defined": defined, "bricks": bricks, "actions": actions}
