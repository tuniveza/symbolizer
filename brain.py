"""brick's brain: symbols linked by weighted associations, and how alike two symbols are.

A symbol is a word ('red', 'zorp'), an idea in square brackets ('[colour]'), or a feature of how a word
is used: the word before it ('before:in'), the word after it ('after:at', 'after:(end)'), the ideas
around it ('before:[by means of]'), or a facet from a symbol's own universe ('shape:round').

The brain links symbols by itself, with nobody telling it, from:
  every direction the factory proves by building it (the words, how they're used, the idea each was read as)
  your saved .brick files and the sentence pool (texts/sentences.txt): how each word is used
  texts/universe.txt (word -> idea, idea -> wider idea) and the dictionary (word -> what it means)
  ConceptNet (texts/sources/conceptnet.tsv): words that belong together
  texts/universes/*.txt, if there are any: one file per symbol, lines like 'shape: round, sphere'

How alike two symbols are (0.0 to 1.0) is the weighted overlap of their link patterns:
  score = sum of min(wa, wb) / sum of max(wa, wb), over everything either is linked to,
leaving out the two symbols themselves (that 'a is linked to b' is not a reason for them to be alike).
Each pattern is first taken as shares of the symbol's links (each weight over the symbol's total), so a
word seen 3 times and one seen 3,000 times compare by how they are used, not by how often.
The same symbol is 1.0; a symbol with no links is 0.0 to everything.

The universe asks it about a word nothing else knows (not the universe, the dictionary, spelling, WordNet
or ConceptNet): which word brick knows is used most like it? If that reaches the threshold, the unknown
word borrows that word's ideas ('vermilion' used like 'red' is a colour), and that reading is built and
checked like every other.

What it learns is kept in texts/factory/brain.txt, plain text: one link per line, which you can read,
change or delete (delete the file to start again). Old links fade: when the weights add up to FADE_AT,
every weight halves and the weakest go. Settings (on or off, the threshold) are in
texts/factory/brain-settings.txt. The factory teaches it, only while brick runs.

    python3 brain.py                 status
    python3 brain.py like WORD       the words used most like WORD
    python3 brain.py links WORD      WORD's strongest links
    python3 brain.py learn           learn from everything now
"""
import glob
import os
import re
import sys
import time

HERE = os.path.dirname(os.path.abspath(__file__))
PATH = os.path.join(HERE, "texts", "factory", "brain.txt")
SETTINGS = os.path.join(HERE, "texts", "factory", "brain-settings.txt")
UNIVERSES = os.path.join(HERE, "texts", "universes")
POOL = os.path.join(HERE, "texts", "sentences.txt")
THRESHOLD = 0.65             # how alike a word must be used to stand for another: measured with measure() (brick brain measure)
CLOSE = 0.1                  # a runner-up meaning something else this close to the best: too close to call.
# How a word is used says what kind of thing it is (a colour) far better than which one (fawn is used much
# like aquamarine, chartreuse and cerulean, 0.78 to 0.74): only a word that stands out is taken.
FADE_AT = 4_000_000          # when the weights add up to this, every weight halves
LEND = {"colour", "place", "window", "background", "screen", "size", "style", "time unit"}
# what a word used like another may borrow: what a thing is (a colour, a place ...), never an action, so
# an unknown verb is never quietly read as 'write' or 'clear'
IDEA_WEIGHT = 3.0            # word -> its idea, from universe.txt: one line there is worth a few sightings
MEANS_WEIGHT = 2.0           # dictionary: word <-> what it means


def overlap(pa, pb):
    """The weighted overlap of two patterns ({symbol: weight}): sum of min / sum of max, 0.0 to 1.0."""
    keys = set(pa) | set(pb)
    most = sum(max(pa.get(k, 0.0), pb.get(k, 0.0)) for k in keys)
    return sum(min(pa.get(k, 0.0), pb.get(k, 0.0)) for k in keys) / most if most else 0.0


def shares(pattern):
    total = sum(pattern.values())
    return {k: w / total for k, w in pattern.items()} if total else {}


def usage(symbol):
    """Is this link about how a word is used (not one of its own ideas, '[colour]')?"""
    return not symbol.startswith("[")


def words_of(line):
    """A line as the brain reads it: words, numbers as '(number)', quoted text as '(quotes)', and
    punctuation that ends a direction as '(stop)'."""
    found = []
    for w in re.findall(r'"[^"]*"|[A-Za-z]+(?:\'[A-Za-z]+)?|\d+(?:\.\d+)?|[,;.!?]', line):
        found.append("(quotes)" if w.startswith('"') else "(number)" if w[0].isdigit() else
                     "(stop)" if w in ",;.!?" else w.lower())
    return found


class Brain:
    def __init__(self, path=None):
        self.path = PATH if path is None else path
        self.links = {}          # symbol -> {symbol it's linked to: weight}
        self.learned = {}        # what it has learned from already: source -> marker (a time, a line count)
        self.total = 0.0
        self.changed = False
        self.saved_at = time.time()
        self._index = None
        if self.path and os.path.isfile(self.path):
            with open(self.path, encoding="utf-8") as f:
                for line in f:
                    parts = line.rstrip("\n").split("\t")
                    if parts[0] == "link" and len(parts) == 4:
                        try:
                            weight = float(parts[3])
                        except ValueError:
                            continue
                        self.links.setdefault(parts[1], {})[parts[2]] = weight
                        self.total += weight
                    elif parts[0] == "learned" and len(parts) == 3:
                        self.learned[parts[1]] = parts[2]

    # ---------------- linking ----------------

    def associate(self, a, b, weight=1.0, both_ways=False):
        """Link a to b (and b to a). Linking again makes the link stronger."""
        if not a or not b or a == b or weight <= 0:
            return
        for x, y in ((a, b), (b, a)) if both_ways else ((a, b),):
            self.links.setdefault(x, {})[y] = self.links.get(x, {}).get(y, 0.0) + weight
            self.total += weight
        self.changed, self._index = True, None
        if self.total > FADE_AT:
            self.fade()

    def fade(self):
        """Every weight halves; links weaker than 1 before that go (what was said long ago counts less)."""
        kept = {}
        for a, linked in self.links.items():
            left = {b: w / 2 for b, w in linked.items() if w >= 1}
            if left:
                kept[a] = left
        self.links = kept
        self.total = sum(w for linked in kept.values() for w in linked.values())
        self.changed, self._index = True, None

    def learn_line(self, line, world=None):
        """How each word in a line is used: the words on either side of it, and the ideas around it."""
        if world is None:
            import universe
            world = universe.universe()
        words = words_of(line)
        for k, word in enumerate(words):
            if word.startswith("("):
                continue
            before = words[k - 1] if k else "(start)"
            after = words[k + 1] if k + 1 < len(words) else "(end)"
            self.associate(word, f"before:{before}")
            self.associate(word, f"after:{after}")
            for idea in sorted(world.words.get(before, ())):
                self.associate(word, f"before:[{idea}]")
            for idea in sorted(world.words.get(after, ())):
                self.associate(word, f"after:[{idea}]")

    def learn_proved(self, line, roles, world=None):
        """A direction the factory proved by building it: how its words are used, and the idea each was read as."""
        self.learn_line(line, world)
        for word, idea, _ in roles:
            if idea != "words" and re.fullmatch(r"[a-z]+", word or ""):
                self.associate(word, f"[{idea}]")

    def learn_ideas(self, world=None):
        """texts/universe.txt (with brick's colours and the dictionary's places and times): word -> idea,
        idea -> wider idea."""
        if world is None:
            import universe
            world = universe.universe()
        for word, ideas in world.words.items():
            if re.fullmatch(r"[a-z]+", word):
                for idea in ideas:
                    self.associate(word, f"[{idea}]", IDEA_WEIGHT)
        for idea, wider in world.wider.items():
            for w in wider:
                self.associate(f"[{idea}]", f"[{w}]", IDEA_WEIGHT)

    def learn_dictionary(self):
        """The dictionary: a word and what it means are linked ('scribble' <-> 'write'), and the word to its
        kind ('[colour]')."""
        import dictionary
        for entry in dictionary.entries():
            key, meaning = entry.key.lower(), entry.meaning.lower()
            if not re.fullmatch(r"[a-z]+", key):
                continue
            if re.fullmatch(r"[a-z]+", meaning):
                self.associate(key, meaning, MEANS_WEIGHT, both_ways=True)
            if entry.kind:
                self.associate(key, f"[{entry.kind}]", MEANS_WEIGHT)

    def learn_conceptnet(self, path=None, world=None):
        """ConceptNet links between single words where one of them is a word brick knows."""
        import conceptnet
        import dictionary
        if world is None:
            import universe
            world = universe.universe()
        path = conceptnet.PATH if path is None else path
        if not os.path.isfile(path):
            return 0
        known = {w for w in world.words if re.fullmatch(r"[a-z]+", w)} | \
            {e.key.lower() for e in dictionary.entries() if re.fullmatch(r"[a-z]+", e.key.lower())}
        count = 0
        with open(path, encoding="utf-8", errors="replace") as f:
            for line in f:
                if line.startswith("#"):
                    continue
                parts = line.rstrip("\n").split("\t")
                if len(parts) != 4 or parts[1] not in conceptnet.KEPT:
                    continue
                a, _, b, weight = parts
                if not (a.isalpha() and b.isalpha()) or a not in known and b not in known:
                    continue
                try:
                    self.associate(a.lower(), b.lower(), float(weight), both_ways=True)
                    count += 1
                except ValueError:
                    continue
        return count

    def learn_universe(self, name, links):
        """A symbol's own universe: links = [(symbol, facet, weight)], e.g. ('round', 'shape', 1.0).
        name <-> symbol are linked, and name gets the facet as a feature ('shape:round'), so two symbols
        with alike universes come out alike."""
        name = name.strip().lower()
        for symbol, facet, weight in links:
            symbol, facet = str(symbol).strip().lower(), str(facet).strip().lower()
            if not symbol:
                continue
            self.associate(name, symbol, float(weight), both_ways=True)
            if facet:
                self.associate(name, f"{facet}:{symbol}", float(weight))

    def learn_universe_file(self, path):
        """texts/universes/<symbol>.txt: lines like 'shape: round, sphere'; '#' lines are notes."""
        links = []
        with open(path, encoding="utf-8", errors="replace") as f:
            for line in f:
                line = line.strip()
                if not line or line.startswith("#") or ":" not in line:
                    continue
                facet, words = line.split(":", 1)
                links += [(w.strip(), facet.strip(), 1.0) for w in words.split(",") if w.strip()]
        self.learn_universe(os.path.splitext(os.path.basename(path))[0], links)
        return len(links)

    def catch_up(self, folders=()):
        """Learn from whatever is new or changed since last time: universe.txt, the dictionary, ConceptNet,
        texts/universes, new lines in the sentence pool, and new or changed .brick files in these folders.
        -> {what: how many things learned from it}."""
        import conceptnet
        import dictionary
        import universe
        world = universe.universe()
        done = {}

        def changed(source, path):
            when = f"{os.path.getmtime(path):.0f}" if os.path.isfile(path) else None
            if when is None or self.learned.get(source) == when:
                return False
            self.learned[source] = when
            self.changed = True
            return True

        if changed("universe.txt", universe.PATH):
            self.learn_ideas(world)
            done["universe.txt"] = 1
        if changed("dictionary.txt", dictionary.PATH):
            self.learn_dictionary()
            done["dictionary.txt"] = 1
        if changed("conceptnet.tsv", conceptnet.PATH):
            done["conceptnet.tsv"] = self.learn_conceptnet(world=world)
        for path in sorted(glob.glob(os.path.join(UNIVERSES, "*.txt"))):
            if changed("universes/" + os.path.basename(path), path):
                done["universes/" + os.path.basename(path)] = self.learn_universe_file(path)
        if os.path.isfile(POOL):
            start = int(self.learned.get("sentences.txt", "0") or 0)
            with open(POOL, encoding="utf-8", errors="replace") as f:
                lines = f.readlines()
            if len(lines) < start:              # the pool was started again
                start = 0
            for line in lines[start:]:
                if line.strip() and not line.startswith("#"):
                    self.learn_line(line.strip(), world)
            if len(lines) != start:
                done["sentences.txt"] = len(lines) - start
                self.learned["sentences.txt"] = str(len(lines))
                self.changed = True
        for folder in dict.fromkeys(folders):
            for path in sorted(glob.glob(os.path.join(folder, "*.brick"))):
                if changed("brick:" + os.path.abspath(path), path):
                    import factory
                    lines = [line.strip() for line in factory.source_of(path).splitlines()]
                    for line in lines:
                        if line and not line.startswith("#"):
                            self.learn_line(line, world)
                    done[os.path.basename(path)] = len(lines)
        return done

    # ---------------- comparing ----------------

    def pattern(self, symbol, only_usage=False, leave_out=(), extra=None):
        """The symbol's links as shares of all its links (leaving out these symbols)."""
        raw = dict(self.links.get(symbol, {}))
        for k, w in (extra or {}).items():
            raw[k] = raw.get(k, 0.0) + w
        return shares({k: w for k, w in raw.items() if k not in leave_out and k != symbol and
                       (not only_usage or usage(k))})

    def similarity(self, a, b, only_usage=False, extra=None):
        """0.0 to 1.0: how alike the link patterns of a and b are (the weighted overlap). extra: more links
        for a, just for this question (how it's used in the line being read)."""
        if a == b:
            return 1.0
        return overlap(self.pattern(a, only_usage, {a, b}, extra), self.pattern(b, only_usage, {a, b}))

    def index(self):
        """symbol -> the words linked to it, to find quickly which words share anything with one."""
        if self._index is None:
            self._index = {}
            for a, linked in self.links.items():
                if re.fullmatch(r"[a-z]+", a):
                    for b in linked:
                        self._index.setdefault(b, []).append(a)
        return self._index

    def like(self, word, top=8, among=None, only_usage=False, extra=None):
        """The words most like this one -> [(word, score)], best first, scores above 0."""
        mine = self.pattern(word, only_usage, (), extra)
        if among is None:
            found = set()
            for k in mine:
                found.update(self.index().get(k, ()))
            among = found
        # The same sum as similarity(), worked out faster: with both patterns as shares (each adding up to
        # 1), sum of max = 2 - sum of min, so the score is m / (2 - m), and m needs only the shared links.
        raw = dict(self.links.get(word, {}))
        for k, w in (extra or {}).items():
            raw[k] = raw.get(k, 0.0) + w
        raw = {k: w for k, w in raw.items() if k != word and (not only_usage or usage(k))}
        total = sum(raw.values())
        scores = []
        for other in among:
            if other == word or other not in self.links or not total:
                continue
            theirs = self.links[other]
            if only_usage:
                theirs = {k: w for k, w in theirs.items() if usage(k)}
            mine_total = total - raw.get(other, 0.0)
            their_total = sum(theirs.values()) - theirs.get(word, 0.0) - theirs.get(other, 0.0)
            if mine_total <= 0 or their_total <= 0:
                continue
            small, big = (raw, theirs) if len(raw) < len(theirs) else (theirs, raw)
            m = sum(min(raw[k] / mine_total, theirs[k] / their_total)
                    for k in small if k in big and k != word and k != other)
            score = m / (2 - m) if m < 2 else 1.0
            if score > 0:
                scores.append((other, score))
        scores.sort(key=lambda x: (-x[1], x[0]))
        return scores[:top]

    def understand(self, word, before=None, after=None, world=None, threshold=None):
        """A word brick doesn't know: the known word it's used most like, if that reaches the threshold and
        no word with other ideas comes close -> (known word, score), or None.
        before, after: the words on either side of it in the line being read."""
        if world is None:
            import universe
            world = universe.universe()
        threshold = settings()["threshold"] if threshold is None else threshold
        extra = {}
        for side, other in (("before", before), ("after", after)):
            other = (words_of(other) or ["(" + ("start" if side == "before" else "end") + ")"])[0] \
                if other else "(" + ("start" if side == "before" else "end") + ")"
            extra[f"{side}:{other}"] = 1.0
            for idea in world.words.get(other, ()):
                extra[f"{side}:[{idea}]"] = 1.0
        known = [w for w, ideas in world.words.items() if ideas & LEND and re.fullmatch(r"[a-z]+", w)
                 and w != word and w in self.links]
        return pick(self.like(word, top=len(known), among=known, only_usage=True, extra=extra), world, threshold)

    def strongest(self, symbol, top=12):
        return sorted(self.links.get(symbol, {}).items(), key=lambda x: (-x[1], x[0]))[:top]

    # ---------------- saving ----------------

    def save(self, every=None):
        """Write it out (if anything changed). every: only if that many seconds have gone by since the last save."""
        if not self.changed or not self.path or every and time.time() - self.saved_at < every:
            return
        os.makedirs(os.path.dirname(self.path) or ".", exist_ok=True)
        lines = ["# brick's brain: which symbols are linked, and how strongly. Learned by itself from what the",
                 "# factory proves, your .brick files, the sentence pool, universe.txt, the dictionary and ConceptNet.",
                 "# One link per line: link, a symbol, what it's linked to, how strongly. [colour] is an idea;",
                 "# before:in means 'comes after the word in'. Change or delete lines freely; delete the file to",
                 "# start again. Old links fade."]
        lines += [f"learned\t{k}\t{v}" for k, v in sorted(self.learned.items())]
        lines += [f"link\t{a}\t{b}\t{w:g}" for a in sorted(self.links) for b, w in sorted(self.links[a].items())]
        with open(self.path + ".new", "w", encoding="utf-8") as f:
            f.write("\n".join(lines) + "\n")
        os.replace(self.path + ".new", self.path)
        self.changed, self.saved_at = False, time.time()


def pick(found, world, threshold):
    """From [(known word, score)], best first: the best, if it reaches the threshold and no word meaning
    something else comes within CLOSE of it -> (word, score), or None."""
    if not found or found[0][1] < threshold:
        return None
    best, score = found[0]
    if any(s >= score - CLOSE and not same_thing(best, other, world) for other, s in found[1:]):
        return None                             # used about as much like another word: too close to call
    return best, score


def same_thing(a, b, world):
    """Do two words brick knows mean the same here: the same colour ('grey', 'gray'), or one means the other
    in the dictionary ('centre', 'center')?"""
    if world.words.get(a, set()) & LEND != world.words.get(b, set()) & LEND:
        return False
    import dictionary
    from actions import COLOURS
    table = dictionary.colour_table()
    codes = [COLOURS.get(w) or table.get(w, (None,))[0] for w in (a, b)]
    if codes[0] and codes[0] == codes[1]:
        return True
    means = {e.key.lower(): e.meaning.lower() for e in dictionary.entries()}
    return means.get(a, a) == means.get(b, b)


# ---------------- on or off, and the brain brick uses ----------------

def settings():
    """{'on': bool, 'threshold': float} from texts/factory/brain-settings.txt (plain text)."""
    found = {"on": True, "threshold": THRESHOLD}
    try:
        with open(SETTINGS, encoding="utf-8") as f:
            for line in f:
                key, _, value = line.split("#", 1)[0].partition(":")
                key, value = key.strip().lower(), value.strip().lower()
                if key == "on":
                    found["on"] = value not in ("no", "off", "false", "0")
                elif key == "threshold":
                    try:
                        found["threshold"] = min(1.0, max(0.0, float(value)))
                    except ValueError:
                        pass
    except OSError:
        pass
    if os.environ.get("BRICK_BRAIN_THRESHOLD"):
        found["threshold"] = float(os.environ["BRICK_BRAIN_THRESHOLD"])
    return found


def save_settings(**changes):
    found = {**settings(), **changes}
    os.makedirs(os.path.dirname(SETTINGS), exist_ok=True)
    with open(SETTINGS, "w", encoding="utf-8") as f:
        f.write("# brick's brain. on: yes or no. threshold: how alike (0 to 1) an unknown word must be used to a\n"
                "# word brick knows before it's read as that word.\n"
                f"on: {'yes' if found['on'] else 'no'}\nthreshold: {found['threshold']:g}\n")


def path():
    chosen = os.environ.get("BRICK_BRAIN")
    return chosen if chosen and chosen != "off" else PATH


def off():
    """Not used: in the tests (BRICK_MEMORY=off, unless BRICK_BRAIN names a brain file), with
    BRICK_BRAIN=off, or when switched off (brick brain off)."""
    chosen = os.environ.get("BRICK_BRAIN")
    if chosen == "off":
        return True
    if chosen:
        return False                    # a brain file named on purpose: on
    return os.environ.get("BRICK_MEMORY") == "off" or not settings()["on"]


_brain, _read_when = None, None


def brain():
    """The brain as saved (read again when the file changes)."""
    global _brain, _read_when
    where = path()
    when = (where, os.path.getmtime(where) if os.path.isfile(where) else None)
    if _brain is None or when != _read_when:
        _brain, _read_when = Brain(where), when
    return _brain


def stamp():
    """What the brain's answers depend on, for the universe to know when to read a line again."""
    if off():
        return None
    where = path()
    return where, os.path.getmtime(where) if os.path.isfile(where) else None, settings()["threshold"]


# ---------------- how good is the threshold? ----------------

def measure(mind=None):
    """Two checks of the threshold, on what the brain has learned -> lines of plain English.
    1. Each word brick knows (a colour, a place ...) is taken as unknown: the word the brain finds used most
       like it (how it's used only, not its ideas) should carry the same ideas.
    2. Ordinary words (no ideas, seen in use 5+ times): how many would pass for a word brick knows?"""
    import universe
    mind = brain() if mind is None else mind
    world = universe.universe()
    known = [w for w, ideas in world.words.items() if ideas & LEND and re.fullmatch(r"[a-z]+", w) and w in mind.links]

    def best(word, among):
        return pick(mind.like(word, top=len(among), among=among, only_usage=True), world, 0.0)

    held = [(w, best(w, [k for k in known if k != w])) for w in known]
    held = [(w, b[0], b[1]) for w, b in held if b]
    plain = [w for w in mind.links if re.fullmatch(r"[a-z]+", w) and w not in world.words and
             sum(v for k, v in mind.links[w].items() if ":" in k and usage(k)) >= 5]
    passing = [(w, b[0], b[1]) for w, b in ((w, best(w, known)) for w in plain) if b]
    lines = [f"{len(known)} words brick knows, each taken as unknown; {len(plain)} ordinary words seen in use."]
    for t in (0.4, 0.5, 0.6, 0.65, 0.7, 0.8):
        above = [h for h in held if h[2] >= t]
        right = sum(world.words[w] & LEND == world.words[o] & LEND for w, o, _ in above)
        through = [p for p in passing if p[2] >= t]
        lines.append(f"  threshold {t:.2f}: known words matched {len(above)}, right {right}; "
                     f"ordinary words passing {len(through)}: " +
                     ", ".join(f"{w}~{o}" for w, o, _ in sorted(through, key=lambda p: p[2])[:4]))
    return lines


# ---------------- brick brain ----------------

def command(args):
    if args[:1] in (["off"], ["on"]):
        save_settings(on=args[0] == "on")
        print(f"The brain is {args[0]}." + (" brick won't use it to read words it doesn't know, and the factory "
                                            "won't teach it." if args[0] == "off" else ""))
        return
    if args[:1] == ["threshold"] and len(args) == 2:
        try:
            save_settings(threshold=min(1.0, max(0.0, float(args[1]))))
        except ValueError:
            print("The threshold is a number from 0 to 1, like 0.6.")
            return
        print(f"The brain's threshold is now {settings()['threshold']:g}.")
        return
    if args[:1] == ["measure"]:
        print("\n".join(measure()))
        return
    if args[:1] == ["learn"]:
        import factory
        if factory.running():
            print("(The factory is running alongside brick, and teaches the brain too.)")
        mind = Brain(path())
        print("Learning from everything...", flush=True)
        started = time.time()
        done = mind.catch_up([HERE, factory.EXAMPLES, os.getcwd()])
        mind.save()
        if done:
            for what, count in done.items():
                print(f"  {what}: {count:,}")
        else:
            print("  nothing new since last time")
        print(f"Done in {time.time() - started:.1f}s. {len(mind.links):,} symbols, "
              f"{sum(len(v) for v in mind.links.values()):,} links, in {mind.path}")
        return
    if args[:1] in (["like"], ["links"]) and len(args) == 2:
        mind, word = brain(), args[1].lower()
        if word not in mind.links:
            print(f"The brain hasn't seen '{word}' yet.")
            return
        if args[0] == "links":
            print(f"'{word}' is most strongly linked to:")
            for other, weight in mind.strongest(word):
                print(f"  {other:<28} {weight:g}")
            return
        import universe
        world = universe.universe()
        print(f"Words used most like '{word}' (threshold {settings()['threshold']:g}):")
        for other, score in mind.like(word, only_usage=True):     # how they are used: what brick compares
            ideas = ", ".join(sorted(world.words.get(other, set()) & LEND))
            print(f"  {other:<20} {score:.2f}" + (f"   ({ideas})" if ideas else ""))
        return
    if args:
        print("brick brain           is it on, and how much it knows\n"
              "brick brain like red  the words used most like red\n"
              "brick brain links red red's strongest links\n"
              "brick brain learn     learn from everything now\n"
              "brick brain off / on  stop using it, or start again\n"
              "brick brain measure   how well the threshold tells words apart, on what it has learned\n"
              "brick brain threshold 0.6   how alike a word must be used to stand for another")
        return
    found = settings()
    mind = brain()
    print(f"The brain is {'on' if found['on'] else 'off'}" +
          (" (but not used now: BRICK_MEMORY=off or BRICK_BRAIN=off)." if found["on"] and off() else "."))
    print(f"It knows {len(mind.links):,} symbols and {sum(len(v) for v in mind.links.values()):,} links, "
          f"in {mind.path}" + ("" if os.path.isfile(mind.path) else " (not made yet: brick brain learn)"))
    print(f"Threshold: {found['threshold']:g}. A word brick doesn't know is read as a word it does when it's "
          "used at least that much alike.")


if __name__ == "__main__":
    sys.path.insert(0, HERE)
    command(sys.argv[1:])
