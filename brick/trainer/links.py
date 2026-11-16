"""The digital brain's universal linkage: one network of links, and a reading is right when its links hold together.

Everything is a node: each word, each role a word can play (W words to show, C colour, P place, N count, T time,
F frame, B background, K console, Q the text, G window, M name, S size, D file, A amount, H button, V action word,
X chatter), each brick, a word's neighbours ('before:in', 'after:times'), a word's shape ('shape:num').
The links between them come from the brain's own practice: its archetypes voice directions in their personalities
(trainer/voices.py), and every word of every voiced direction links to the role it played, its neighbours to that
role, each role to the next, each word to the brick, and the words that stay together ('orange' and 'juice' shown
together) link as together, the ones split apart as apart. What the world knows links in too: ConceptNet's words
that belong together ('orange juice', 'red wine').

How strong a link is: how much more often the two turn up together than by chance (log odds, smoothed).

A reading of a direction (a role for every word, and its brick) is scored by how strongly all its links hold:
each word to its role, its neighbours to its role, each role to the next, each pair of neighbours together (same
stretch) or apart (different ones), every word and role to the brick. The best reading wins (Viterbi over roles,
for each brick). How sure it is: how far ahead the best reading is of the best other one (another brick, or the
same words grouped another way). Too close: it says it isn't sure.

    python3 -m trainer.links learn          the links from the archetypes' practice (trainer/data/links.txt)
    python3 -m trainer.links "a line"       the best reading, how sure, and the links that decided it
"""

import math
import os
import re
import sys
from collections import defaultdict

HERE = os.path.dirname(os.path.abspath(__file__))
sys.path.insert(0, os.path.dirname(HERE))

PATH = os.path.join(HERE, "data", "links.txt")
ROLES = ["W", "F", "C", "P", "N", "T", "B", "K", "Q", "G", "M", "S", "D", "A", "H", "V", "X"]
STRETCH = {"W", "M"}          # roles whose words stay together as one stretch
DICT_ON = 2.5                 # how strongly a dictionary role holds a word the practice hasn't met
DICT_OFF = 0.0                # the dictionary never rules a role out: a real word can be a slip ("pint" for print)
MEANING = {"C", "P", "T", "N", "V"}   # roles the dictionary can rule out (a word that is no colour is never one)
SURE = 1.0                    # how far ahead the best reading must be (on unseen practice: reads ~88%, ~85% right)


def shape(word):
    w = word.lower().strip(".,!?;:")
    if word.startswith('"'):
        return "quote"
    if re.fullmatch(r"\d+", w):
        return "num"
    if re.fullmatch(r"x\d+|\d+x", w):
        return "xnum"
    if re.fullmatch(r"[\w-]+\.(txt|csv|md|log|json)", w):
        return "file"
    if re.fullmatch(r"\d+x\d+", w):
        return "size"
    return "upper" if word.isupper() and len(word) > 1 else "word"


def norm(word):
    from trainer import picker
    w = word.lower().strip(".,!?;:")
    if word.startswith('"'):
        return "(quoted)"
    return picker.respell([w])[0] if w.isalpha() and len(w) > 3 else w


def words_of(line):
    return re.findall(r'"[^"]*"|\S+', line)


class Links:
    def __init__(self, path=PATH):
        self.path = path
        self.count = defaultdict(float)      # (a, b) -> how often together
        self.total = defaultdict(float)      # a -> how often at all
        self.n = 0.0
        self.strength = {}
        self.words = defaultdict(int)        # word -> how often met (for how rare it is)
        self.adjust = defaultdict(float)     # (a, b) -> how much the line grew or faded from its own mistakes
        if os.path.isfile(path):
            self.load()

    # ---------------- learning ----------------

    def see(self, a, b, weight=1.0):
        self.count[(a, b)] += weight
        self.total[a] += weight
        self.total[b] += weight
        self.n += weight

    def rarity(self, w):
        """How often the brain has met the word: 'rare' words are mostly words to show, names and slips."""
        n = self.words.get(w, 0)
        return "seen:rare" if n < 3 else "seen:some" if n < 30 else "seen:often"

    def observe(self, words, roles, brick):
        """One voiced direction: every link it shows."""
        ws = [norm(w) for w in words]
        for i, (w, r) in enumerate(zip(ws, roles)):
            role = "r:" + r
            self.see("w:" + w, role)
            self.see(self.rarity(w), role)
            self.see("before:" + (ws[i - 1] if i else "(start)"), role)
            self.see("after:" + (ws[i + 1] if i + 1 < len(ws) else "(end)"), role)
            self.see("shape:" + shape(words[i]), role)
            self.see("r:" + (roles[i - 1] if i else "(start)"), "next:" + r)
            if r != "X":
                self.see("r:" + r, "b:" + brick)
                self.see("w:" + w, "b:" + brick)
            if i:
                same = roles[i - 1] == r and r in STRETCH
                self.see("pair:" + ws[i - 1] + "|" + w, "together" if same else "apart")

    def know(self):
        """What the world knows links in: ConceptNet's words that belong together, as if seen shown together."""
        from trainer import conceptnet, scrambler
        if not os.path.isfile(conceptnet.PATH):
            return
        web = conceptnet.web()
        colours = set(scrambler.COLOURS) | {"black", "brown", "grey", "white", "silver"}
        for (a, b) in list(self.count):
            if a.startswith("pair:"):
                left, right = a[5:].split("|", 1)
                if left in colours or right in colours:
                    t = web.together(left, right)
                    if t > 0:
                        self.see(a, "together", 2.0 * t)

    def finish(self):
        """Each link's strength: log odds of the two together against chance (smoothed)."""
        self.strength = {}
        for (a, b), c in self.count.items():
            expected = self.total[a] * self.total[b] / max(self.n, 1.0)
            self.strength[(a, b)] = math.log((c + 0.5) / (expected + 0.5))

    def save(self):
        with open(self.path + ".new", "w", encoding="utf-8") as f:
            f.write("# brick's digital brain: the universal linkage. One link per line: a, b, how often together.\n"
                    "# Learned from the archetypes' practice (python3 -m trainer.links learn). Delete it to start again.\n")
            for (a, b), c in sorted(self.count.items()):
                f.write(f"{a}\t{b}\t{c:.3f}\n")
            for (a, b), g in sorted(self.adjust.items()):
                if abs(g) > 0.001:
                    f.write(f"grown\t{a}\t{b}\t{g:.3f}\n")
        os.replace(self.path + ".new", self.path)

    def load(self):
        with open(self.path, encoding="utf-8") as f:
            for row in f:
                parts = row.rstrip("\n").split("\t")
                if len(parts) == 4 and parts[0] == "grown":
                    self.adjust[(parts[1], parts[2])] = float(parts[3])
                elif len(parts) == 3 and not row.startswith("#"):
                    self.see(parts[0], parts[1], float(parts[2]))
        self.words = defaultdict(int, {a[2:]: int(t) for a, t in self.total.items() if a.startswith("w:")})
        self.finish()

    # ---------------- reading ----------------

    def s(self, a, b):
        """How strongly a and b are linked. Never seen together though both are common: that counts against."""
        grown = self.adjust.get((a, b), 0.0)
        if (a, b) in self.strength:
            return self.strength[(a, b)] + grown
        if self.total.get(a) and self.total.get(b):
            expected = self.total[a] * self.total[b] / max(self.n, 1.0)
            return math.log(0.5 / (expected + 0.5)) + grown
        return grown

    def met(self, w):
        """Has the brain met the word often enough in practice to know it from there?"""
        return self.words.get(w, 0) >= 3

    def known(self, w, node):
        """How strongly word w links to a role or brick node: from practice when the brain has met the word; else
        from the dictionary (trainer/dictbrain.py): a form or slip borrows its own word's links ('waited' -> wait),
        any other word gets the roles and brick its dictionary entry allows."""
        if self.met(w):
            return self.s("w:" + w, node)
        got = dictionary_entry(w)
        if not got:
            return self.s("w:" + w, node)
        roles, brick, base = got
        if base and self.met(base):
            return self.s("w:" + base, node)
        if node.startswith("b:"):
            return (DICT_ON if node[2:] == brick else -DICT_OFF) if brick else 0.0
        r = node[2:]
        if r in roles:
            return DICT_ON * roles[r]
        if r == "M" and "W" in roles:                   # a name is words too
            return DICT_ON * 0.5 * roles["W"]
        return -DICT_OFF if r in MEANING else 0.0

    def word_role(self, words, ws, i, r):
        role = "r:" + r
        rare = 0.0 if not self.met(ws[i]) and dictionary_entry(ws[i]) else 0.5   # the dictionary knows it: not rare
        return (self.known(ws[i], role) + rare * self.s(self.rarity(ws[i]), role) + 0.5 * self.s("before:" + (ws[i - 1] if i else "(start)"), role) +
                0.5 * self.s("after:" + (ws[i + 1] if i + 1 < len(ws) else "(end)"), role) +
                0.5 * self.s("shape:" + shape(words[i]), role))

    def pair(self, a, b, together):
        """How strongly two neighbours link as one stretch (together) or as two (apart). A pair the practice never
        met: what the world knows (ConceptNet: 'orange juice' belongs together)."""
        node = "pair:" + a + "|" + b
        if self.total.get(node):
            return self.s(node, "together" if together else "apart")
        known = known_together(a, b)
        return (1.0 + known if together else -1.0 - known) if known else 0.0

    def best(self, words, brick, forbid=None):
        """(score, roles): the reading whose links hold together best, for this brick."""
        ws = [norm(w) for w in words]
        if not ws:
            return 0.0, []
        b = "b:" + brick

        def to_brick(i, r):                  # a word links to the brick through itself and its role; chatter doesn't
            if r == "X":
                return 0.0
            return self.s("r:" + r, b) + self.known(ws[i], b) * (2.0 if r == "V" else 1.0)
        table = [{}]
        for r in ROLES:
            if forbid and r in forbid.get(0, ()):
                continue
            table[0][r] = (self.word_role(words, ws, 0, r) + self.s("r:(start)", "next:" + r) + to_brick(0, r), None)
        for i in range(1, len(ws)):
            col = {}
            for r in ROLES:
                if forbid and r in forbid.get(i, ()):
                    continue
                here = self.word_role(words, ws, i, r) + to_brick(i, r)
                col[r] = max(((table[-1][q][0] + here + self.s("r:" + q, "next:" + r) +
                               self.pair(ws[i - 1], ws[i], q == r and r in STRETCH), q) for q in table[-1]),
                             key=lambda x: x[0])
            table.append(col)
        r = max(table[-1], key=lambda x: table[-1][x][0])
        score, path = table[-1][r][0], [r]
        for i in range(len(ws) - 1, 0, -1):
            r = table[i][r][1]
            path.append(r)
        return score, path[::-1]

    def brick_pull(self, ws, roles, brick):
        """How strongly the words, in these roles, pull toward the brick."""
        b = "b:" + brick
        return sum(self.s("r:" + r, b) + self.known(w, b) * (2.0 if r == "V" else 1.0)
                   for w, r in zip(ws, roles) if r != "X")

    def practise(self, examples, rounds=4):
        """Lines that lead to the right brick grow, lines that led to a wrong one fade (each direction read with its
        own words' roles): the linkage learns from its own mistakes."""
        bricks = self.bricks()
        for k in range(rounds):
            wrong = 0
            for words, roles, brick in examples:
                ws = [norm(w) for w in words]
                pick = max(bricks, key=lambda x: self.brick_pull(ws, roles, x))
                if pick == brick:
                    continue
                wrong += 1
                for w, r in zip(ws, roles):
                    if r == "X":
                        continue
                    for node in ("r:" + r, "w:" + w):
                        self.adjust[(node, "b:" + brick)] += 0.3
                        self.adjust[(node, "b:" + pick)] -= 0.3
            print(f"  practice round {k + 1}: {wrong} of {len(examples)} pulled to the wrong brick", flush=True)

    def bricks(self):
        return sorted({b[2:] for (a, b) in self.count if b.startswith("b:")})

    def read(self, words):
        """[(score, brick, roles)], best first: the best reading for each brick, and for the best brick the
        best ones with a word in another role (the same words grouped another way)."""
        found = [(score, brick, roles) for brick in self.bricks() for score, roles in [self.best(words, brick)]]
        found.sort(key=lambda x: -x[0])
        if found:
            score, brick, roles = found[0]
            for i, r in enumerate(roles):               # each word in its next best role
                other_score, other = self.best(words, brick, {i: {r}})
                if other != roles:
                    found.append((other_score, brick, other))
        found.sort(key=lambda x: -x[0])
        return found

    def why(self, words, roles):
        """The links that held the reading together most, and the weakest."""
        ws = [norm(w) for w in words]
        parts = [(f"{words[i]} as {r}", self.word_role(words, ws, i, r)) for i, r in enumerate(roles)]
        parts += [(f"{words[i - 1]} + {words[i]} {'together' if roles[i - 1] == roles[i] and roles[i] in STRETCH else 'apart'}",
                   self.pair(ws[i - 1], ws[i], roles[i - 1] == roles[i] and roles[i] in STRETCH))
                  for i in range(1, len(ws))]
        parts.sort(key=lambda x: -x[1])
        return parts[:3], parts[-2:]


def dictionary_entry(w):
    if os.environ.get("BRICK_DICTBRAIN") == "off":
        return None
    from trainer import dictbrain
    return dictbrain.entry(w)


_known = {}


def known_together(a, b):
    """How strongly ConceptNet says two words belong together (0 if it doesn't link them)."""
    if (a, b) not in _known:
        from trainer import conceptnet
        _known[(a, b)] = conceptnet.web().together(a, b) if os.path.isfile(conceptnet.PATH) and a.isalpha() and \
            b.isalpha() and len(a) > 2 and len(b) > 2 else 0.0
    return _known[(a, b)]


_links = None


def links():
    global _links
    if _links is None:
        _links = Links()
    return _links


def reading(piece):
    """The direction's best reading (brick, roles) and how sure (the gap to the best other), or None."""
    words = words_of(piece)
    found = links().read(words)
    if not found:
        return None
    best = found[0]
    gap = best[0] - next((f[0] for f in found[1:] if (f[1], f[2]) != (best[1], best[2])), best[0] - 99)
    return best[1], best[2], gap, words


def held_out(k):
    """Every tenth-ish voiced paragraph is kept back: the brain is checked on those, never learns from them."""
    return (k * 2654435761) % 1000 < 100


def learn():
    """The links, strictly from the archetypes' voiced practice (brick's builder only says which brick a meaning
    builds as)."""
    import json
    import shutil
    import tempfile
    from trainer import details, picker, voices
    lk = Links(path=PATH)
    lk.count.clear(); lk.total.clear(); lk.n = 0.0; lk.words.clear(); lk.adjust.clear()
    folder = tempfile.mkdtemp(prefix="links-")
    words_file, built, seen, examples = os.path.join(folder, "mywords.txt"), {}, 0, []
    try:
        with open(voices.PRACTICE, encoding="utf-8") as f:
            rows = [json.loads(r) for r in f if r.strip()][-picker.NEWEST:]
        rows = [d for k, d in enumerate(rows) if not held_out(k)]   # some kept back to check it on
        for d in rows:                                   # first: how often the brain meets each word
            for part in d["parts"]:
                for w in words_of(part):
                    lk.words[norm(w)] += 1
        for d in rows:
            for part, tidy in zip(d["parts"], d["tidy"]):
                if tidy not in built:
                    b = picker.all_bricks_of(tidy, words_file)     # (waits too: 'wait 4 seconds' is a wait)
                    built[tidy] = picker.main(b)[0] if b and len(picker.main(b)) == 1 else None
                if not built[tidy]:
                    continue
                pairs = details.voiced_roles(part, tidy)
                lk.observe([w for w, _ in pairs], [r for _, r in pairs], built[tidy])
                examples.append(([w for w, _ in pairs], [r for _, r in pairs], built[tidy]))
                seen += 1
    finally:
        shutil.rmtree(folder, ignore_errors=True)
    lk.know()
    lk.finish()
    lk.practise(examples)
    lk.save()
    print(f"Linked {seen} voiced directions: {len(lk.count)} links -> {PATH}")


def command(args):
    if args[:1] == ["learn"]:
        return learn()
    if args:
        found = reading(" ".join(args))
        if not found:
            print("nothing linked yet")
            return
        brick, roles, gap, words = found
        print(f"{brick}  (ahead by {gap:.1f}{'' if gap >= SURE else ': not sure'})")
        print("  ".join(f"{w}/{r}" for w, r in zip(words, roles)))
        strong, weak = links().why(words, roles)
        print("held together most by:", "; ".join(f"{p} {v:+.1f}" for p, v in strong))
        print("weakest:", "; ".join(f"{p} {v:+.1f}" for p, v in weak))
        return
    print(__doc__)


if __name__ == "__main__":
    command(sys.argv[1:])
