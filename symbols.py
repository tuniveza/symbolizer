"""A universe for each symbol: brick's neurons.

Every symbol brick knows something about (apple, round, fruit ...) has a plain-text file of its own,
texts/universes/<symbol>.txt, in the style of texts/universe.txt ("idea: word, word"). It holds the symbol's
links to other symbols, grouped by facet (what it is, its shape, colour, mathematics, biology, what it goes
with ...), each with a strength and where it came from:

    # apple: brick's neuron for this symbol ...
    is: fruit (4.2, ConceptNet, WordNet), edible fruit (3, WordNet)
    shape: round (6, seen in apple.png, ConceptNet), sphere (5, yours)
    colour: red (2, seen in apple.png), green (1.6, ConceptNet)
    biology: plant (2.5, WordNet), seed (0.3, via fruit)
    not: company
    seen in: apple.png
    looked up: WordNet, ConceptNet

  a link with no brackets   was written by hand: it's "yours" (strength 5), and brick never fades or deletes it
  not: word, word           words brick must never add back
  apple is a fruit          a line like that counts as "is: fruit" (yours), like texts/universe.txt
Every symbol inside a universe can have a universe of its own (abstract on abstract): made on demand
(brick universe round), or born by itself when a link to it gets strong enough.

Each file is a neuron that learns by itself, only while brick runs:
  experience(symbols, source)   symbols that turned up together fire together, and their links grow
                                (a symbol with no file yet waits in texts/universes/_waiting.txt)
  practise(budget)              one small round with no input (the factory calls it every few seconds):
                                a few neurons, least recently practised first,
                                  spread activation along their strongest links and gain weak links to what
                                  answers strongly (apple is a fruit, a fruit has seeds: apple gets
                                  "parts: seed (0.2, via fruit)"; at most 2 new ones a round, kept weak),
                                  look themselves up in WordNet and ConceptNet once, for facets they lack,
                                  fade links unused for a long while (never the ones you wrote),
                                and at most MAX_BIRTHS new neurons are born for links that got strong.
                                Returns one line saying what changed: "apple learned seed (via fruit)".
  links_for(name)               for the brain: [(symbol, facet, weight 0..1)], e.g. ("round", "shape", 1.0);
                                weight = strength / 5, at most 1 (5 is a hand-written link's strength)

Little words are symbols too ('the', 'in'): broad ones. Each file's header says its breadth (how many
symbols it links to: "# breadth: 340 (a broad symbol)"), and in spreading and in recognising pictures a link
counts 1 / (1 + floor(log2(breadth))), breadth counted leaving out the symbols being compared (as brain.py).
Letters have neurons of their own, letter:a in letter-a.txt (never the word 'a'): the letters before and
after them, where they sit in words, the words they're in, vowel or consonant (spell()); a word's neuron gets
"letters:" and "spelled:" lines. A context is a symbol of itself: context:in _ at (context-in-_-at.txt) is
what fills 'in _ at' in directions, and a scene the eye sees ('sky above ground') is what is seen in it
(experience() and contextualise() make them; they're born like any neuron, from _waiting.txt).

When each link was last used is brick's own bookkeeping, in texts/universes/_times.json (delete it any time).

Commands (main(args); handles(args) says whether a brick command line is one of these):
    brick show apple.png as apple        look at a picture and learn what it shows (also: this is an apple:
                                         apple.png / apple.png is an apple)
    brick look photo.jpg                 the eye names it itself: memory first, then the vision model (also:
                                         what is photo.jpg / look at photo.jpg)
    brick universe apple                 print apple's universe, filled from WordNet and ConceptNet if new
    brick universe apple forget          delete it
    brick universes                      list them
    brick sight                          is the vision model there?   brick sight setup: download it (once)
"""
import json
import os
import re
import time

HERE = os.path.dirname(os.path.abspath(__file__))
FOLDER = os.path.join(HERE, "texts", "universes")
WORDNET = os.path.join(HERE, "texts", "sources", "wordnet")
CONCEPTNET = os.path.join(HERE, "texts", "sources", "conceptnet.tsv")

YOURS = 5.0                 # a hand-written link's strength
BIRTH = 4.0                 # a link this strong gives the symbol it points to a neuron of its own
MAX_BIRTHS = 2              # new neuron files per round, at most
MAX_NEURONS = 2000          # and in all
PER_ROUND = 3               # neurons practised per round
SPREAD = 6                  # strongest links a firing neuron sends activation along
DECAY = 0.8                 # activation lost at each step
ANSWERS = 0.15              # activation that counts as a strong answer (after the breadth weights)
NEW_PER_ROUND = 2           # indirect links a neuron may gain in one round
FADE_AFTER = 2 * 86400      # a link unused this long (seconds) fades a little each time its neuron practises
FADE = 0.9
GONE = 0.2                  # a learned link weaker than this is let go
BROAD, NARROW = 40, 8       # breadth that makes a symbol broad / specific (said in its header)
IN_WORDS = 20               # words kept on a letter's "in words" line (the strongest)
VOWELS = set("aeiou")
FORM = {"letters", "spelled"}   # how a word is written, not what it's linked to: left out of its breadth
WAITING_MOST = 3000         # links kept for symbols with no neuron yet
META = ("not", "seen in", "looked up")
ORDER = ("is", "same as", "shape", "size", "colour", "texture", "taste", "mathematics", "biology", "parts",
         "part of", "made of", "uses", "kinds", "like", "opposite", "goes with")
ALIASES = {"color": "colour", "colors": "colour", "colours": "colour", "related": "goes with",
           "related to": "goes with", "goes": "goes with", "with": "goes with", "kind of": "is", "is a": "is",
           "a kind of": "is", "part": "parts", "has": "parts", "has parts": "parts", "used for": "uses",
           "use": "uses", "math": "mathematics", "maths": "mathematics", "synonyms": "same as",
           "seen": "seen in", "never": "not"}
LOOSE = {"goes with", "same as", "like"}              # a symbol already filed elsewhere stays there
TRANSITIVE = {"is", "biology", "parts", "made of"}     # what fruit is, apple is too (spread keeps the facet)
SHAPES = {"round", "circular", "circle", "sphere", "spherical", "oval", "square", "rectangular", "rectangle",
          "triangle", "triangular", "flat", "long", "curved", "pointed", "cylindrical", "cylinder", "cone",
          "cube", "ball", "disc", "disk", "ring", "oblong", "spiral", "straight", "roundish", "elliptical"}
TASTES = {"sweet", "sour", "bitter", "salty", "tart", "savory", "savoury", "spicy", "tangy", "bland", "acidic"}
TEXTURES = {"crisp", "crunchy", "soft", "hard", "smooth", "rough", "juicy", "fluffy", "furry", "hairy", "sticky",
            "waxy", "shiny", "fuzzy", "slimy", "wet", "dry", "firm", "tender", "creamy"}
STOP = {"the", "a", "an", "of", "and", "or", "to", "in", "on", "at", "by", "for", "with", "it", "is", "be", "as",
        "this", "that", "then", "than", "so", "but", "if", "i", "you", "me", "my", "we", "he", "she", "they",
        "them", "his", "her", "its", "are", "was", "were", "do", "does", "did", "not", "no", "yes", "into",
        "onto", "from", "after", "before", "there", "here", "some", "any", "all", "can", "will", "please"}
TOO_WIDE = {"entity", "physical entity", "abstraction", "abstract entity", "object", "physical object", "whole",
            "unit", "matter", "thing", "causal agent", "psychological feature", "attribute", "relation", "group",
            "grouping", "measure", "quantity", "solid", "part", "portion", "living thing", "being", "organism"}
LIVING = {"organism", "living thing", "plant", "animal", "fungus", "person", "bacteria", "microorganism"}
now = time.time             # a function, so the tests can move the clock


# ---------------- names and files ----------------

def clean(name):
    """'An Apple!' -> 'apple'; the words of a symbol, lower case, no articles in front.
    'letter:a' (the letter, not the word 'a') and 'context:in _ at' (a context) keep their kind in front."""
    kind = re.match(r"^\s*(letter|context)\s*:\s*(.*)$", str(name).lower())
    if kind:
        rest = kind.group(2)
        if kind.group(1) == "letter":
            rest = re.sub(r"[^a-z0-9]", "", rest)[:1]
        else:
            rest = re.sub(r"\s+", " ", re.sub(r"[^a-z0-9 _'\-]+", " ", rest)).strip()
        return f"{kind.group(1)}:{rest}" if rest else ""
    name = re.sub(r"[^a-z0-9 '\-.]+", " ", str(name).lower().replace("_", " "))
    name = re.sub(r"\s+", " ", name).strip(" .'-")
    return re.sub(r"^(?:a|an|the) ", "", name)


def slug(name):
    """'apple tree' -> apple-tree, 'letter:a' -> letter-a, 'context:in _ at' -> context-in-_-at."""
    return re.sub(r"[^a-z0-9_]+", "-", clean(name)).strip("-")


def target(facet, value):
    """The neuron a link points to: letters (and the letters before / after one) are 'letter:x', contexts are
    'context:...', everything else is the symbol itself."""
    if facet in ("letters", "before", "after"):
        return f"letter:{value}"
    if facet == "contexts":
        return f"context:{value}"
    return value


_breadths = {}


def breadth(name, leave_out=()):
    """1 + how many distinct symbols a neuron links to, leaving out the ones being compared (leave_out): broad
    symbols ('the', 'red') link to many. A symbol with no neuron has breadth 1."""
    path = path_of(name) if slug(name) else ""
    try:
        when = os.path.getmtime(path)
    except OSError:
        return 1
    if _breadths.get(path, (None,))[0] != when:
        _breadths[path] = (when, {target(f, v) for v, f, _ in Neuron(name).links() if f not in FORM})
    linked = _breadths[path][1]
    return 1 + len(linked) - len(linked & {clean(n) for n in leave_out})


def weight(name, leave_out=()):
    """How much a link to this symbol counts, in spreading and in recognising: 1 / (1 + floor(log2(breadth))),
    with breadth counted leaving out the symbols being compared (the same form as brain.py and the compiled
    programs). A symbol linked to nothing else counts 1; to 3 others, 1/2; to 1,000, 1/10."""
    return 1.0 / breadth(name, leave_out).bit_length()


def path_of(name):
    return os.path.join(FOLDER, slug(name) + ".txt")


def exists(name):
    return bool(slug(name)) and os.path.isfile(path_of(name))


def names():
    """Every symbol that has a universe, by the name inside its file."""
    if not os.path.isdir(FOLDER):
        return []
    found = []
    for file in sorted(os.listdir(FOLDER)):
        if file.endswith(".txt") and not file.startswith("_"):
            found.append(Neuron.read_name(os.path.join(FOLDER, file)) or file[:-4].replace("-", " "))
    return found


def tidy(word):
    """A value as it can go in a file: no commas, brackets or #; 'pear-shaped' and 'pear shaped' the same."""
    word = re.sub(r"(?<=[a-z])-(?=[a-z])", " ", str(word).lower())
    return re.sub(r"\s+", " ", re.sub(r"[,()#:\n]", " ", word)).strip()


def plain(word):
    """A source or a file name as it can go in a file: case kept, no commas, brackets, colons or #."""
    return re.sub(r"\s+", " ", re.sub(r"[,()#:\n]", " ", str(word))).strip()


def number(text):
    return f"{round(text, 2):g}"


def split_items(text):
    """'round (5, seen in a.png), sphere' -> ['round (5, seen in a.png)', 'sphere'] (commas in brackets kept)."""
    items, depth, word = [], 0, ""
    for ch in text:
        if ch == "(":
            depth += 1
        elif ch == ")":
            depth = max(0, depth - 1)
        if ch == "," and depth == 0:
            items.append(word)
            word = ""
        else:
            word += ch
    items.append(word)
    return [item.strip() for item in items if item.strip()]


class Link:
    def __init__(self, strength, sources=(), yours=False):
        self.strength, self.sources, self.yours = float(strength), list(sources), yours

    def text(self, value):
        inside = [number(self.strength)] + (["yours"] if self.yours else []) + self.sources
        return f"{value} ({', '.join(inside)})"


def parse_item(item):
    """'round (5, seen in a.png)' -> ('round', Link); 'sphere' (no brackets) -> yours, strength 5."""
    found = re.match(r"^(.*?)\s*\(([^()]*)\)\s*$", item)
    if not found:
        return tidy(item), Link(YOURS, [], True)
    value, parts = tidy(found.group(1)), [p.strip() for p in found.group(2).split(",") if p.strip()]
    strength = None
    if parts and re.fullmatch(r"\d+(?:\.\d+)?", parts[0]):
        strength = float(parts.pop(0))
    yours = any(p.lower() == "yours" for p in parts)
    sources = [p for p in parts if p.lower() != "yours"]
    return value, Link(strength if strength is not None else (YOURS if yours else 1.0), sources, yours)


# ---------------- a neuron: one symbol's file ----------------

class Neuron:
    def __init__(self, name):
        self.name = clean(name)
        self.path = path_of(self.name)
        self.lines = []          # in file order: ("text", line) or ("facet", facet) or ("meta", facet)
        self.facets = {}         # facet -> {value: Link}
        self.notes = {}          # facet -> the owner's # note after that line
        self.meta = {}           # "not" / "seen in" / "looked up" -> [words]
        self.said_is = set()     # "apple is a fruit" lines: fruit
        self.changed = False
        if os.path.isfile(self.path):
            with open(self.path, encoding="utf-8", errors="replace") as f:
                self.parse(f.read())

    @staticmethod
    def read_name(path):
        try:
            with open(path, encoding="utf-8", errors="replace") as f:
                first = f.readline()
        except OSError:
            return None
        found = re.match(r"#\s*(.+?): brick's neuron", first)
        return found.group(1).strip() if found else None

    def header(self, with_breadth=True):
        wide = len({target(f, v) for v, f, _ in self.links() if f not in FORM})
        kind = "a broad symbol" if wide >= BROAD else "a specific symbol" if wide <= NARROW else \
            "between broad and specific"
        return [f"# {self.name}: brick's neuron for this symbol: what it's linked to, how strongly, and where "
                "each link came from."] + ([f"# breadth: {wide} ({kind})"] if with_breadth else []) + [
                "# round (3.5, ConceptNet) means: linked to round with strength 3.5, learned from ConceptNet. "
                "Links grow when",
                "# they turn up together and fade when unused. Anything you write by hand (no brackets, or "
                "\"yours\" in them)",
                "# is kept: brick never fades or deletes it. Put a word on the \"not:\" line and brick won't "
                "add it back."]

    def parse(self, text):
        standard = {line.strip() for line in self.header(False)[1:]}
        for line in text.splitlines():
            stripped = line.strip()
            if stripped.startswith("#"):
                if stripped in standard or re.match(r"#\s*.+?: brick's neuron|#\s*breadth:", stripped):
                    continue             # the header is written fresh each time
                self.lines.append(("text", line))
                continue
            body, _, note = line.partition(" #")
            if ":" in body:
                facet, words = (part.strip() for part in body.split(":", 1))
                facet = ALIASES.get(facet.lower(), facet.lower())
                if facet in META:
                    kept = self.meta.setdefault(facet, [])
                    for word in words.split(","):
                        word = tidy(word) if facet == "not" else plain(word)
                        if word and word.lower() not in [k.lower() for k in kept]:
                            kept.append(word)
                    if ("meta", facet) not in self.lines:
                        self.lines.append(("meta", facet))
                    continue
                links = self.facets.setdefault(facet, {})
                for item in split_items(words):
                    value, link = parse_item(item)
                    if value and value not in links:
                        links[value] = link
                    elif value and link.yours and not links[value].yours:     # written again by hand
                        links[value].yours, links[value].strength = True, max(link.strength,
                                                                              links[value].strength)
                if note.strip():
                    self.notes[facet] = note.strip()
                if ("facet", facet) not in self.lines:
                    self.lines.append(("facet", facet))
                continue
            said = re.match(r"^(.+?) is (?:an? )?(.+)$", body.strip())
            if said and clean(said.group(1)) == self.name:
                self.said_is.add(tidy(said.group(2)))
            self.lines.append(("text", line))
        for word in self.meta.get("not", ()):          # "not:" words go, unless written by hand
            for links in self.facets.values():
                if word in links and not links[word].yours:
                    del links[word]

    def render(self):
        out = self.header()
        placed = set()
        for kind, what in self.lines:
            if kind == "text":
                out.append(what)
            elif kind == "facet" and self.facets.get(what):
                out.append(self.facet_line(what))
            if kind != "meta":
                placed.add(what)
        new = [f for f in self.facets if f not in placed and self.facets[f]]
        for facet in sorted(new, key=lambda f: ORDER.index(f) if f in ORDER else len(ORDER)):
            out.append(self.facet_line(facet))
        for facet in META:                  # the notes about the neuron itself go last
            if self.meta.get(facet):
                out.append(f"{facet}: {', '.join(self.meta[facet])}")
        return "\n".join(out) + "\n"

    def facet_line(self, facet):
        links = sorted(self.facets[facet].items(), key=lambda kv: (-kv[1].yours, -kv[1].strength))
        line = f"{facet}: " + ", ".join(link.text(value) for value, link in links)
        return line + (f"   # {self.notes[facet]}" if facet in self.notes else "")

    def save(self):
        os.makedirs(FOLDER, exist_ok=True)
        with open(self.path + ".new", "w", encoding="utf-8") as f:
            f.write(self.render())
        os.replace(self.path + ".new", self.path)
        self.changed = False

    # ---- links ----

    def find(self, value):
        """The facet this neuron already files value under, or None."""
        for facet, links in self.facets.items():
            if value in links:
                return facet
        return None

    def links(self):
        """[(value, facet, Link)] for every link (not the meta lines)."""
        return [(value, facet, link) for facet, links in self.facets.items() for value, link in links.items()]

    def strongest(self, most):
        return sorted(self.links(), key=lambda t: -t[2].strength)[:most]

    def add(self, facet, value, amount, source, ceiling=None, times=None):
        """Strengthen the link to value (filed under facet) by amount; -> 'new', 'grew' or None.
        ceiling: never past this strength by this route (a weak, indirect hint stays weak)."""
        facet = ALIASES.get(tidy(facet), tidy(facet)) or "goes with"
        if facet in META:
            value = tidy(value) if facet == "not" else plain(value)
            kept = self.meta.setdefault(facet, [])
            if value and value.lower() not in [k.lower() for k in kept]:
                kept.append(value)
                self.changed = True
            return None
        value = tidy(value)
        if not value or value == self.name or len(value) > 60 or value in self.meta.get("not", ()):
            return None
        if facet == "is" and value in self.said_is:
            return None
        source = plain(source) if source else ""
        if facet in LOOSE and value not in self.facets.get(facet, {}):
            facet = self.find(value) or facet       # 'goes with: core' when parts has core: strengthen that
        links = self.facets.setdefault(facet, {})
        if facet not in LOOSE and value not in links:
            for loose in LOOSE:                     # 'shape: circle' moves 'goes with: circle' to its facet
                moved = self.facets.get(loose, {}).pop(value, None)
                if moved is not None and not moved.yours:
                    links[value] = moved
                    break
                if moved is not None:
                    self.facets[loose][value] = moved
        link = links.get(value)
        if link is None:
            if ceiling is not None:
                amount = min(amount, ceiling)
            links[value] = Link(round(amount, 2), [source] if source else [])
            result = "new"
        else:
            before = link.strength
            grown = before + amount
            if ceiling is not None:
                grown = max(before, min(grown, ceiling))
            link.strength = round(grown, 2)
            if source and source not in link.sources:
                link.sources = (link.sources + [source])[-6:]
            result = "grew" if link.strength > before else None
        if times is not None:
            times.used(self.name, facet, value)
        self.changed = self.changed or result is not None
        return result


# ---------------- when links were last used (brick's own bookkeeping) ----------------

class Times:
    PATH = "_times.json"

    def __init__(self):
        self.file = os.path.join(FOLDER, self.PATH)
        try:
            with open(self.file, encoding="utf-8") as f:
                self.data = json.load(f)
        except (OSError, ValueError):
            self.data = {}
        self.dirty = False

    def of(self, name):
        return self.data.setdefault(slug(name), {"practised": 0, "used": {}})

    def used(self, name, facet, value):
        self.of(name)["used"][f"{facet}|{value}"] = now()
        self.dirty = True

    def last_used(self, name, facet, value):
        key = f"{facet}|{value}"
        used = self.of(name)["used"]
        if key not in used:                  # never seen before: its clock starts now
            used[key] = now()
            self.dirty = True
        return used[key]

    def practised(self, name):
        self.of(name)["practised"] = now()
        self.dirty = True

    def forget(self, name):
        if self.data.pop(slug(name), None) is not None:
            self.dirty = True

    def save(self):
        if self.dirty:
            os.makedirs(FOLDER, exist_ok=True)
            with open(self.file + ".new", "w", encoding="utf-8") as f:
                json.dump(self.data, f)
            os.replace(self.file + ".new", self.file)
            self.dirty = False


# ---------------- symbols with no neuron yet ----------------

WAITING = "_waiting.txt"


def waiting_path():
    return os.path.join(FOLDER, WAITING)


def read_waiting():
    """{symbol: {facet: {value: Link}}} from texts/universes/_waiting.txt."""
    found = {}
    try:
        with open(waiting_path(), encoding="utf-8", errors="replace") as f:
            lines = f.read().splitlines()
    except OSError:
        return found
    for line in lines:
        head = re.match(r"^([^#/][^/]*?) / ([^:]+): (.*)$", line)
        if head:
            links = found.setdefault(clean(head.group(1)), {}).setdefault(head.group(2).strip(), {})
            for item in split_items(head.group(3)):
                value, link = parse_item(item)
                if value:
                    links[value] = link
    return found


def write_waiting(found):
    every = [(link.strength, symbol, facet, value) for symbol, facets in found.items()
             for facet, links in facets.items() for value, link in links.items()]
    if len(every) > WAITING_MOST:                     # keep the strongest
        keep = {(s, f, v) for _, s, f, v in sorted(every, reverse=True)[:WAITING_MOST]}
        for symbol, facets in found.items():
            for facet, links in facets.items():
                for value in [v for v in links if (symbol, facet, v) not in keep]:
                    del links[value]
    out = ["# Symbols that turned up together but have no neuron of their own yet. When one of these links",
           f"# reaches strength {number(BIRTH)}, the symbol gets its own file in this folder. Safe to delete."]
    for symbol in sorted(found):
        for facet, links in sorted(found[symbol].items()):
            if links:
                ranked = sorted(links.items(), key=lambda kv: -kv[1].strength)
                out.append(f"{symbol} / {facet}: " + ", ".join(link.text(v) for v, link in ranked))
    os.makedirs(FOLDER, exist_ok=True)
    with open(waiting_path() + ".new", "w", encoding="utf-8") as f:
        f.write("\n".join(out) + "\n")
    os.replace(waiting_path() + ".new", waiting_path())


# ---------------- what a word is about, guessed ----------------

_colours = None


def colour_names():
    global _colours
    if _colours is None:
        try:
            from actions import COLOURS
            import dictionary
            _colours = set(COLOURS) | {name for name, (_, thing) in dictionary.colour_table().items()
                                       if not thing}        # not salmon or snow: things first
        except Exception:
            _colours = {"red", "green", "blue", "yellow", "orange", "purple", "pink", "brown", "black", "white",
                        "grey", "gray"}
    return _colours


def guess_facet(value):
    value = tidy(value)
    if value in colour_names() or re.fullmatch(r"(?:light|dark|bright|pale|deep) \w+", value) and \
            value.split()[1] in colour_names():
        return "colour"
    if value in SHAPES:
        return "shape"
    if value in TASTES:
        return "taste"
    if value in TEXTURES:
        return "texture"
    return None


# ---------------- knowledge: WordNet and ConceptNet ----------------

_files = {}


def _bytes(path):
    if path not in _files:
        try:
            with open(path, "rb") as f:
                _files[path] = b"\n" + f.read()
        except OSError:
            _files[path] = b""
    return _files[path]


def _lines_starting(path, key):
    """Every line of a big file that starts with key (bytes), found at C speed."""
    data, found, at = _bytes(path), [], 0
    key = b"\n" + key
    while True:
        at = data.find(key, at)
        if at < 0:
            return found
        end = data.find(b"\n", at + 1)
        found.append(data[at + 1:end if end >= 0 else len(data)].decode("utf-8", "replace"))
        at += 1


POS_FILES = {"n": "noun", "a": "adj", "v": "verb", "r": "adv"}


def wn_index(word, pos):
    """-> (tag-sense count, [synset offsets]) or None."""
    lines = _lines_starting(os.path.join(WORDNET, "index." + POS_FILES[pos]),
                            (word.replace(" ", "_").lower() + " ").encode())
    if not lines:
        return None
    fields = lines[0].split()
    pointers = int(fields[3])
    return int(fields[5 + pointers]), fields[6 + pointers:]


def wn_synset(offset, pos):
    """-> (words, [(pointer, offset, pos)], meaning)."""
    if pos == "s":
        pos = "a"
    data = _bytes(os.path.join(WORDNET, "data." + POS_FILES[pos]))
    start = int(offset) + 1                       # +1: the newline put in front
    line = data[start:data.find(b"\n", start)].decode("utf-8", "replace")
    head, _, meaning = line.partition(" | ")
    fields = head.split()
    count = int(fields[3], 16)
    words = [re.sub(r"\(.*?\)$", "", fields[4 + 2 * k]).replace("_", " ") for k in range(count)]
    at = 4 + 2 * count
    pointers = [tuple(fields[at + 1 + 4 * k: at + 4 + 4 * k]) for k in range(int(fields[at]))]
    return words, pointers, meaning.strip()


def wordnet_facets(name):
    """What WordNet says about a symbol -> {facet: [(value, strength)]}."""
    found = {}

    def put(facet, value, strength):
        value = tidy(value)
        if value and value != name and value not in TOO_WIDE:
            found.setdefault(facet, [])
            if value not in [v for v, _ in found[facet]]:
                found[facet].append((value, strength))

    def first(offset, pos="n"):
        return wn_synset(offset, pos)[0][0].lower()

    if not os.path.isdir(WORDNET):
        return found
    known = {pos: wn_index(name, pos) for pos in "na"}
    main = max((pos for pos in known if known[pos]), key=lambda p: known[p][0], default=None)
    if known["a"] and guess_facet(name) in ("shape", "taste", "texture"):
        main, known["n"] = "a", None  # 'round' is a shape, not a round of ammunition
    for pos in "na":
        if not known[pos] or pos != main and known[pos][0] == 0:
            continue
        for k, offset in enumerate(known[pos][1][:2 if pos == main else 1]):
            words, pointers, meaning = wn_synset(offset, pos)
            strong = 1.0 if k == 0 and pos == main else 0.5       # the main meaning counts most
            latin = [w for w in words if re.fullmatch(r"[A-Z][a-z]+ [a-z]+", w)]
            synonyms = [w.lower() for w in words if w.lower() != name and w not in latin]
            for word in re.findall(r"[a-z]+", meaning.split(";")[0].lower()):
                facet = guess_facet(word)
                if facet:
                    put(facet, word, round(1.5 * strong, 1))
            if pos == "a":
                for w in synonyms:
                    put("same as", w, round(2.5 * strong, 1))
                for symbol, target, target_pos in pointers:
                    if symbol == "&" and len(found.get("like", ())) < 6:
                        put("like", first(target, "a"), round(1.5 * strong, 1))
                    elif symbol == "!":
                        put("opposite", first(target, "a"), 2 * strong)
                    elif symbol in ("+", "=") and target_pos == "n":
                        put("goes with", first(target), round(1.5 * strong, 1))
                continue
            chain, at = [], pointers
            for _ in range(10):
                up = [p for p in at if p[0] in ("@", "@i")]
                if not up:
                    break
                words_up, at, _ = wn_synset(up[0][1], "n")
                chain.append((words_up[0].lower(), up[0][1], at))
            living = any(w in (LIVING if k == 0 else {"plant", "animal", "fungus"}) for w, _, _ in chain)
            for w in synonyms:
                put("biology" if living and k > 0 else "same as", w, round(2.5 * strong, 1))
            if k == 0 or not living:
                for depth, (w, _, _) in enumerate(chain[:3 if k == 0 else 1]):
                    put("is", w, round(max(1.0, 3 - 0.5 * depth) * strong, 1))
            if living:
                for depth, (w, _, _) in enumerate(chain[:8]):
                    if w in LIVING or depth < 4:
                        put("biology", w, round(max(1.0, 2.5 - 0.25 * depth) * strong, 1))
                for w in latin:
                    put("biology", w.lower(), 2 * strong)
            for symbol, target, _ in pointers + [p for p in (chain[0][2] if chain else []) if p[0] == "%p"]:
                if symbol == "#p":
                    put("part of", first(target), 2.5 * strong)
                elif symbol == "%p":
                    put("parts", first(target), 2 * strong)
                elif symbol == "%s":
                    put("made of", first(target), 2 * strong)
                elif symbol == "#m":
                    genus = first(target)
                    put("biology" if living else "part of", genus.replace("genus ", ""), 2 * strong)
            if k == 0:
                for symbol, target, _ in [p for p in pointers if p[0] == "~"][:5]:
                    put("kinds", first(target), 1.0)
    return found


CONCEPTNET_FACETS = {"IsA": "is", "PartOf": "part of", "HasA": "parts", "MadeOf": "made of", "UsedFor": "uses",
                     "Synonym": "same as", "SimilarTo": "same as", "RelatedTo": "goes with"}


def conceptnet_facets(name):
    """What ConceptNet says about a symbol -> {facet: [(value, strength)]}, strongest first."""
    best = {}
    describes = guess_facet(name) in ("shape", "taste", "texture", "colour")
    for line in _lines_starting(CONCEPTNET, (name + "\t").encode()):
        parts = line.split("\t")
        if len(parts) != 4 or parts[1] not in CONCEPTNET_FACETS or describes and parts[1] == "IsA":
            continue                     # 'round' is a shape; its other senses (ammunition, applause) aren't
        other, weight = tidy(parts[2]), float(parts[3])
        if not other or other == name or len(other.split()) > 3 or weight < 0.5:
            continue
        facet = CONCEPTNET_FACETS[parts[1]]
        if facet == "goes with":
            facet = guess_facet(other) or facet
        best[(facet, other)] = max(best.get((facet, other), 0), weight)
    found = {}
    for (facet, other), weight in sorted(best.items(), key=lambda kv: -kv[1]):
        if len(found.setdefault(facet, [])) < (10 if facet == "goes with" else 8):
            found[facet].append((other, round(min(3.0, 0.5 + weight / 4), 1)))
    return found


def lookup(name):
    """[(source, {facet: [(value, strength)]})] from the knowledge on this computer."""
    name = clean(name)
    if name.startswith("letter:"):
        return [("WordNet", letter_facets(name[-1])), ("ConceptNet", {})]
    if name.startswith("context:"):
        return [("WordNet", {}), ("ConceptNet", {})]      # a context is known by what fills it, nothing else
    return [("WordNet", wordnet_facets(name)), ("ConceptNet", conceptnet_facets(name))]


# ---------------- learning ----------------

def learn(name, facets, source, times=None):
    """Add {facet: [(value, strength)] or [value]} to name's neuron, from source -> [(facet, value)] new."""
    neuron = Neuron(name)
    mine = times is None
    times = times or Times()
    new = []
    for facet, values in facets.items():
        for item in values:
            value, amount = item if isinstance(item, tuple) else (item, 1.0)
            if neuron.add(facet, value, amount, source, times=times) == "new":
                new.append((facet, tidy(value)))
    if neuron.changed or not os.path.isfile(neuron.path):
        neuron.save()
    if mine:
        times.save()
    return new


def fill(name, only_missing=False, times=None):
    """Look name up in WordNet and ConceptNet once (noted on its "looked up:" line) -> [(facet, value)] new."""
    neuron = Neuron(name)
    done = [word.lower() for word in neuron.meta.get("looked up", [])]
    new = []
    for source, facets in lookup(name):
        if source.lower() in done:
            continue
        if only_missing:
            facets = {f: v for f, v in facets.items() if not neuron.facets.get(f)}
        new += learn(name, facets, source, times)
        neuron = Neuron(name)
        neuron.add("looked up", source, 0, "")
        neuron.save()
    return new


class Linker:
    """Strengthens links on neurons, or, for symbols with no neuron yet, in _waiting.txt; saves at the end."""
    def __init__(self, times=None):
        self.times = times or Times()
        self.neurons, self.waiting, self.new = {}, None, []

    def neuron(self, name):
        if name not in self.neurons:
            self.neurons[name] = Neuron(name)
        return self.neurons[name]

    def link(self, owner, facet, value, amount, source, given_facet=False):
        owner = clean(owner)
        if not owner or owner == clean(value):
            return
        if exists(owner) or owner in self.neurons:
            neuron = self.neuron(owner)
            if not given_facet:
                facet = neuron.find(tidy(value)) or facet
            if neuron.add(facet, value, amount, source, times=self.times) == "new":
                self.new.append((owner, facet, tidy(value)))
            return
        if self.waiting is None:
            self.waiting = read_waiting()
        links = self.waiting.setdefault(owner, {}).setdefault(facet, {})
        link = links.setdefault(tidy(value), Link(0, []))
        link.strength = round(link.strength + amount, 2)
        if source and plain(source) not in link.sources:
            link.sources = (link.sources + [plain(source)])[-6:]

    def save(self):
        for neuron in self.neurons.values():
            if neuron.changed or not os.path.isfile(neuron.path):
                neuron.save()
        if self.waiting is not None:
            write_waiting(self.waiting)
        self.times.save()
        return self.new


def experience(symbols, source="together", amount=1.0, centre=None, centre_learns=True, spelling=True):
    """Symbols that turned up together fire together, and their links grow ("fire together, wire together").
    symbols: words in order (a direction, a line), or (word, facet) pairs when the facet is known (an image's
    ("round", "shape")). Little words fire too: they're symbols like any other, just broad ones.
    centre: only links between it and the others (a picture of an apple: apple <-> round, red ...);
    centre_learns=False: only the others learn (the centre was taught already).
    Words in order also make contexts: in 'write hi in red at the top', 'red' fills the context 'in _ at',
    which is a symbol of its own (context:in _ at). Each word's letters are learned too (spell).
    A symbol with no neuron yet keeps its links in _waiting.txt until practise() gives it one.
    -> [(symbol, facet, other)] links that are new."""
    ordered = [clean(item) for item in symbols if isinstance(item, str)]
    ordered = [w for w in ordered if w][:24]
    items, seen = [], set()
    for item in symbols:
        word, facet = item if isinstance(item, tuple) else (item, None)
        word = clean(word)
        if not word or word in seen or len(word) < 1 or re.fullmatch(r"[\d.]+", word):
            continue
        seen.add(word)
        items.append((word, facet))
    items = items[:12]
    centre = clean(centre) if centre else None
    linker = Linker()
    for word, _ in items:
        others = [(o, f) for o, f in items if o != word and (centre is None or centre in (word, o))]
        if not others or re.search(r"\d", word) or word == centre and not centre_learns:
            continue                     # 'ratio 1.04' can be linked to, but needs no neuron of its own
        for other, facet in others:
            given = facet if word == centre or centre is None else None
            linker.link(word, given or guess_facet(other) or "goes with", other, amount, source,
                        given_facet=bool(given))
    if len(ordered) >= 3 and len(ordered) == len(symbols):          # a line of words: its contexts
        for k in range(1, len(ordered) - 1):
            frame = f"{ordered[k - 1]} _ {ordered[k + 1]}"
            if re.search(r"\d", frame) or ordered[k] in ordered[k - 1:k] + ordered[k + 1:k + 2]:
                continue
            linker.link(f"context:{frame}", "fills", ordered[k], amount, source, given_facet=True)
            linker.link(ordered[k], "contexts", frame, amount, source, given_facet=True)
    new = linker.save()
    if spelling:
        spell([w for w in ordered if re.fullmatch(r"[a-z]+", w)] if ordered else
              [w for w, _ in items if re.fullmatch(r"[a-z]+", w)], source)
    return new


def contextualise(thing, contexts, source, appears=(), amount=1.0):
    """A thing seen in a context ('sky above ground', 'a picture with red and round'): the thing links to the
    context, and the context links back to what fills it and what it appears with."""
    thing = clean(thing)
    linker = Linker()
    for context in contexts:
        context = clean(f"context:{context}")
        if not context:
            continue
        frame = context.split(":", 1)[1]
        if thing:
            linker.link(thing, "contexts", frame, amount, source, given_facet=True)
            linker.link(context, "fills", thing, amount, source, given_facet=True)
        for other in appears:
            linker.link(context, "appears with", other, amount / 2, source)
    return linker.save()


def letter_facets(letter):
    """What brick knows about a letter by itself, and what WordNet says -> {facet: [(value, strength)]}."""
    found = {"is": [("letter", 3.0), ("vowel" if letter in VOWELS else "consonant", 3.0)]}
    if letter == "y":
        found["is"].append(("sometimes a vowel", 1.5))
    known = wn_index(letter, "n") if os.path.isdir(WORDNET) else None
    for offset in (known[1] if known else [])[:12]:
        words, pointers, meaning = wn_synset(offset, "n")
        if "letter" not in meaning.lower():
            continue
        for w in words:
            if len(w) > 1 and w.lower() != letter:
                found.setdefault("same as", []).append((w.lower(), 2.0))
        for symbol, up, _ in pointers:
            if symbol == "@":
                found["is"].append((wn_synset(up, "n")[0][0].lower(), 2.0))
        break
    return found


def spell(words, source="spelling", amount=1.0):
    """Letters in context: each letter's neuron (letter:a) learns the letters before and after it, where it
    sits in words (start, middle, end) and the words it's in; a word's neuron learns its letters and how it
    is spelled (starts with, ends with). Letters get a neuron the first time they're seen (26 at most)."""
    words = [w for w in (clean(w) for w in words) if re.fullmatch(r"[a-z]+", w)][:40]
    if not words:
        return
    linker = Linker()
    for word in words:
        for k, ch in enumerate(word):
            name = f"letter:{ch}"
            fresh = not exists(name) and name not in linker.neurons
            neuron = linker.neuron(name)
            if fresh:
                for facet, values in letter_facets(ch).items():
                    for value, strength in values:
                        neuron.add(facet, value, strength, "brick" if facet == "is" and value in
                                   ("letter", "vowel", "consonant", "sometimes a vowel") else "WordNet",
                                   times=linker.times)
                neuron.add("looked up", "WordNet", 0, "")
                neuron.add("looked up", "ConceptNet", 0, "")
            place = "alone" if len(word) == 1 else "start" if k == 0 else "end" if k == len(word) - 1 else "middle"
            neuron.add("place", place, amount, source, times=linker.times)
            if k:
                neuron.add("before", word[k - 1], amount, source, times=linker.times)
            if k < len(word) - 1:
                neuron.add("after", word[k + 1], amount, source, times=linker.times)
            if len(word) > 1:
                neuron.add("in words", word, amount, source, times=linker.times)
                kept = neuron.facets["in words"]
                if len(kept) > IN_WORDS:              # keep the strongest words (and any you wrote)
                    for value, link in sorted(kept.items(), key=lambda kv: kv[1].strength)[:len(kept) - IN_WORDS]:
                        if not link.yours:
                            del kept[value]
        if exists(word) and len(word) > 1:
            neuron = linker.neuron(word)
            if not neuron.facets.get("spelled"):
                for ch in dict.fromkeys(word):
                    neuron.add("letters", ch, 1.0, "spelling", times=linker.times)
                neuron.add("spelled", f"starts with {word[0]}", 1.0, "spelling", times=linker.times)
                neuron.add("spelled", f"ends with {word[-1]}", 1.0, "spelling", times=linker.times)
    linker.save()


def real_word(word):
    """Is this a word WordNet (or brick's little words) knows? For words read off a picture."""
    word = word.lower()
    return word in STOP or os.path.isdir(WORDNET) and any(wn_index(word, pos) for pos in "nvar")


def born(name, links, reason, times):
    """A new neuron file for name, with these links: [(facet, value, Link)]."""
    neuron = Neuron(name)
    for facet, value, link in links:
        neuron.add(facet, value, link.strength, (link.sources or [reason])[-1], times=times)
    neuron.save()


def practise(budget=0.5, neurons=PER_ROUND):
    """One small round of learning with no input. -> one line saying what changed ('' if nothing did)."""
    started = now()
    clock = time.monotonic()
    if not os.path.isdir(FOLDER):
        return ""
    times, said = Times(), []
    files = [f[:-4] for f in os.listdir(FOLDER) if f.endswith(".txt") and not f.startswith("_")]
    births = 0
    room = MAX_NEURONS - len(files)

    # new neurons for symbols waiting with a strong link
    waiting = read_waiting()
    ready = []
    for symbol, facets in waiting.items():
        strengths = [link.strength for links in facets.values() for link in links.values()]
        if strengths and not re.search(r"\d", symbol.split(":")[-1]):
            many = sum(strengths) / 2 if symbol.startswith("context:") else 0      # a context filled by many
            ready.append((max(max(strengths), many), symbol))
    for strength, symbol in sorted(ready, reverse=True):
        if strength < BIRTH or births >= MAX_BIRTHS or births >= room:
            break
        if not exists(symbol):
            born(symbol, [(f, v, l) for f, links in waiting[symbol].items() for v, l in links.items()],
                 "together", times)
            said.append(f"{symbol} got a neuron (strength {number(strength)})")
            births += 1
        del waiting[symbol]
    if births:
        write_waiting(waiting)

    picked = sorted(files, key=lambda s: times.of(s)["practised"])[:neurons]
    looked_up = False
    for file in picked:
        if time.monotonic() - clock > budget:
            break
        name = Neuron.read_name(os.path.join(FOLDER, file + ".txt")) or file.replace("-", " ")
        neuron = Neuron(name)
        # (b) look itself up once, for the facets it hasn't got yet
        if not looked_up and not {"wordnet", "conceptnet"} <= {w.lower() for w in neuron.meta.get("looked up", [])}:
            looked_up = True
            new = fill(name, only_missing=True, times=times)
            if new:
                said.append(f"{name} looked itself up ({len(new)} links)")
            neuron = Neuron(name)
        # (a) fire: spread activation along the strongest links, and link to what answers strongly
        # (letters learn only from spelling: activation doesn't spread from or through them)
        answers = {}
        for value, facet, link in ([] if neuron.name.startswith("letter:") else neuron.strongest(SPREAD)):
            via = target(facet, value)
            if not exists(via) or via.startswith("letter:"):
                continue
            first = min(1.0, link.strength / YOURS) * weight(via, {neuron.name})
            for further, further_facet, further_link in Neuron(via).strongest(SPREAD):
                if target(further_facet, further) == neuron.name or further_facet in ("letters", "before", "after"):
                    continue
                activation = first * min(1.0, further_link.strength / YOURS) * DECAY * \
                    weight(target(further_facet, further), {neuron.name})
                if activation >= ANSWERS and activation > answers.get(further, (0,))[0]:
                    # what fruit is and has, apple is and has (apple is a fruit); red's 'is' isn't apple's
                    keep = further_facet in TRANSITIVE and facet in ("is", further_facet)
                    answers[further] = (activation, via, further_facet if keep else "goes with")
        gained = 0
        for further, (activation, via, further_facet) in sorted(answers.items(), key=lambda kv: -kv[1][0]):
            known = neuron.find(further)
            if not known and gained >= NEW_PER_ROUND:
                continue
            facet = known or further_facet
            if neuron.add(facet, further, 0.3 * activation, f"via {via}", ceiling=2 * activation,
                          times=times) == "new":
                said.append(f"{neuron.name} learned {further} (via {via})")
                gained += 1
        # (c) fade what hasn't been used for a long while (never what you wrote)
        faded = 0
        for value, facet, link in neuron.links():
            if link.yours or started - times.last_used(neuron.name, facet, value) < FADE_AFTER:
                continue
            link.strength = round(link.strength * FADE, 2)
            neuron.changed = True
            faded += 1
            if link.strength < GONE:
                del neuron.facets[facet][value]
                said.append(f"{neuron.name} let go of {value}")
        if faded:
            said.append(f"{neuron.name}: {faded} unused link{'s' * (faded > 1)} faded")
        # strong links to symbols with no neuron: give them one (a few per round)
        for value, facet, link in ([] if neuron.name.startswith("letter:") else neuron.strongest(SPREAD)):
            new = target(facet, value)          # (a letter's links are how words are written: no births)
            if births >= MAX_BIRTHS or births >= room or link.strength < BIRTH or exists(new) or \
                    re.search(r"\d", value) or new.startswith("letter:"):
                continue
            back = ("fills" if new.startswith("context:") else
                    "contexts" if neuron.name.startswith("context:") else "goes with")
            mine = neuron.name.split(":", 1)[1] if back == "contexts" else neuron.name
            born(new, [(back, mine, Link(link.strength / 2, [f"born from {neuron.name}"]))], neuron.name, times)
            said.append(f"{new} got a neuron (from {neuron.name})")
            births += 1
        if neuron.changed:
            neuron.save()
        times.practised(file)
    times.save()
    return "; ".join(said)


def links_for(name):
    """For the brain: [(symbol, facet, weight 0..1)] from name's universe ('round', 'shape', 1.0)."""
    if not exists(name):
        return []
    neuron = Neuron(name)
    found = [(target(facet, value), facet, round(min(1.0, link.strength / YOURS), 3))
             for value, facet, link in neuron.links()]
    found += [(value, "is", 1.0) for value in neuron.said_is if value not in neuron.facets.get("is", {})]
    return sorted(found, key=lambda t: -t[2])


def forget(name):
    """Delete name's universe. -> True if there was one."""
    if not exists(name):
        return False
    os.remove(path_of(name))
    times = Times()
    times.forget(name)
    times.save()
    return True


# ---------------- brick show / brick universe ----------------

def handles(args):
    """Is this brick command line one of these commands?"""
    import sight
    return bool(args) and (args[0] in ("universe", "universes", "sight") or
                           sight.show_request(" ".join(args)) is not None)


def main(args):
    import sight
    if not args:
        print(__doc__)
        return 1
    if args[0] == "sight":
        return sight.command(args[1:])
    if args[0] == "universes":
        found = names()
        print(f"{len(found)} universes in {FOLDER}:" if found else f"No universes yet ({FOLDER}).")
        if found:
            print("  " + ", ".join(found))
        return 0
    if args[0] == "universe":
        words = args[1:]
        drop = words and words[-1].lower() in ("forget", "delete", "remove")
        name = clean(" ".join(words[:-1] if drop else words))
        if not slug(name):
            print("Which symbol? For example: brick universe apple")
            return 1
        if drop:
            if forget(name):
                print(f"Forgot {name}: deleted {path_of(name)}.")
            else:
                print(f"There's no universe for {name}, so there was nothing to forget.")
            return 0
        if not exists(name):
            new = fill(name)
            if not new:
                print(f"WordNet and ConceptNet don't know {name}, so its universe starts empty.")
                Neuron(name).save()
        with open(path_of(name), encoding="utf-8") as f:
            print(f.read(), end="")
        print(f"({path_of(name)})")
        return 0
    request = sight.show_request(" ".join(args))
    if request is None:
        print(__doc__)
        return 1
    return show(*request)


def show(path, name=""):
    """brick show apple.png as apple (teach it), or brick look photo.jpg (the eye names it): look, learn, say."""
    import sight
    name = clean(name) if name else ""
    if not os.path.isfile(path):
        print(f"I can't find {path} in {os.getcwd()}.")
        return 1
    seen = sight.look(path, name or None, learn=True)
    print(seen["summary"])
    for line in seen["learned"]:
        print(f"  learned: {line}")
    for problem in seen["errors"]:
        print(f"  problem: {problem}")
    if seen["name"]:
        print(f"brick universe {seen['name']} shows what brick knows about it ({path_of(seen['name'])}).")
    elif not seen["errors"]:
        print(f"Teach me what it is: brick show {path} as NAME")
    return 1 if seen["errors"] and not seen["symbols"] else 0
