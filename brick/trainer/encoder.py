"""Encoders: where a new neuron's starting frequency comes from. The slot for the owner's encoder engine.

The neurons can run with or without an encoder. Without one (BRICK_ENCODER=none), a new word starts at a random
frequency and has to find its place by learning alone. With one, related words start close together, which is
what an encoder is for.

    frequencies(words, band) -> {word: frequency}     (frequencies in 2 .. 2+band, as the engine uses)

Which encoder (BRICK_ENCODER):
  dictionary   (the default, a stand-in until the owner's encoder engine exists) educated guesses from three
               sources, none of them brick's own word groups (those are the answer key precision is scored
               against): WordNet (synonyms 1.0, sharing a wider word 0.5: "red" and "blue" are both a colour),
               ConceptNet (Synonym, FormOf 1.0, SimilarTo 0.8, IsA 0.5, RelatedTo 0.3), and word parts (writes,
               writing, write: 1.0). A spectral layout then puts strongly linked words at nearby frequencies.
  none         random starting frequencies.
  a file path  a JSON file {word: frequency} (or a Python file with frequencies(words, band)): how a future
               encoder engine plugs in, without touching anything else.
"""
import json
import os
import random

import numpy as np

from trainer import wordnet


DEFAULT = "none"      # until measured: set to "dictionary" once the 12-run test shows it helps


def frequencies(words, band, seed=1):
    which = os.environ.get("BRICK_ENCODER", DEFAULT)
    words = list(dict.fromkeys(words))
    if which == "none" or not words:
        return {}
    if which != "dictionary" and os.path.isfile(which):
        if which.endswith(".py"):
            import importlib.util
            spec = importlib.util.spec_from_file_location("brick_encoder", which)
            module = importlib.util.module_from_spec(spec)
            spec.loader.exec_module(module)
            return module.frequencies(words, band)
        with open(which, encoding="utf-8") as f:
            given = json.load(f)
        return {w: float(given[w]) for w in words if w in given}
    return dictionary(words, band, seed)


def _related(word):
    """The WordNet entries a word belongs to (its two commonest senses per part of speech) and the wider entries
    above them, as ids: two words sharing an id are related."""
    ids, wider = set(), set()
    for pos in "nva":
        try:
            offsets = wordnet.senses(word, pos)[:2]
        except OSError:
            return ids, wider
        for off in offsets:
            ids.add(pos + off)
            try:
                _, pointers = wordnet.synset(off, pos)
            except (OSError, ValueError, IndexError):
                continue
            wider |= {p[2] + p[1] for p in pointers if p[0] in ("@", "@i")}
    return ids, wider


CONCEPTNET = os.path.join(os.path.dirname(wordnet.WORDNET), "conceptnet.tsv")
CN_WEIGHT = {"Synonym": 1.0, "FormOf": 1.0, "SimilarTo": 0.8, "IsA": 0.5, "RelatedTo": 0.3}
_cn_cache = {}


def conceptnet_links(words):
    """ConceptNet links between two of these words -> {(a, b): weight} (read once per set of words)."""
    key = hash(frozenset(words))
    if key in _cn_cache:
        return _cn_cache[key]
    want, out = set(words), {}
    try:
        with open(CONCEPTNET, encoding="utf-8") as f:
            for line in f:
                if line.startswith("#"):
                    continue
                parts = line.rstrip("\n").split("\t")
                if len(parts) < 3 or parts[0] not in want or parts[2] not in want or parts[0] == parts[2]:
                    continue
                w = CN_WEIGHT.get(parts[1])
                if w:
                    a, b = parts[0], parts[2]
                    out[(a, b)] = out[(b, a)] = max(out.get((a, b), 0), w)
    except OSError:
        pass
    _cn_cache.clear()
    _cn_cache[key] = out
    return out


ENDINGS = ("ing", "ed", "es", "s", "er", "ly")


def stem(word):
    """A word without a common ending, for word parts: writes, writing -> writ(e)."""
    for end in ENDINGS:
        if word.endswith(end) and len(word) - len(end) >= 3:
            word = word[: -len(end)]
            break
    return word.rstrip("e")


def dictionary(words, band, seed=1):
    """Linked words (same entry: 1.0; sharing a wider entry: 0.5) laid out on a line by the graph's spectral order
    (the Fiedler vector of each connected group), each group in its own stretch of the band. Words WordNet doesn't
    link to anything get no frequency here (the engine then gives them a random one)."""
    rel = {w: _related(w) for w in words if w.isalpha() and len(w) > 1}
    by_id, by_wider = {}, {}
    for w, (ids, wider) in rel.items():
        for i in ids:
            by_id.setdefault(i, []).append(w)
        for i in wider:
            by_wider.setdefault(i, []).append(w)
    index = {w: k for k, w in enumerate(rel)}
    n = len(index)
    weight = np.zeros((n, n))
    for groups, strength in ((by_id, 1.0), (by_wider, 0.5)):
        for members in groups.values():
            if 1 < len(members) <= 40:                     # very wide entries ("entity") say nothing
                for a in members:
                    for b in members:
                        if a != b:
                            weight[index[a], index[b]] = max(weight[index[a], index[b]], strength)
    for (a, b), w in conceptnet_links(list(index)).items():   # ConceptNet's links
        weight[index[a], index[b]] = max(weight[index[a], index[b]], w)
    by_stem = {}
    for w in index:                                        # word parts
        by_stem.setdefault(stem(w), []).append(w)
    for members in by_stem.values():
        if 1 < len(members) <= 8:
            for a in members:
                for b in members:
                    if a != b:
                        weight[index[a], index[b]] = 1.0
    names = list(index)
    comp = [-1] * n
    c = 0
    for s in range(n):
        if comp[s] != -1 or weight[s].sum() == 0:
            continue
        stack = [s]
        comp[s] = c
        while stack:
            x = stack.pop()
            for y in np.nonzero(weight[x])[0]:
                if comp[y] == -1:
                    comp[y] = c
                    stack.append(y)
        c += 1
    order = []                                             # each component in spectral order, components one after another
    rnd = random.Random(seed)
    comps = list(range(c))
    rnd.shuffle(comps)
    for k in comps:
        members = [i for i in range(n) if comp[i] == k]
        if len(members) == 1:
            order += members
            continue
        sub = weight[np.ix_(members, members)]
        degree = sub.sum(axis=1)
        lap = np.diag(degree) - sub
        d = np.diag(1 / np.sqrt(degree))
        vals, vecs = np.linalg.eigh(d @ lap @ d)
        fiedler = vecs[:, 1] if len(members) > 2 else np.array([0.0, 1.0])
        order += [members[i] for i in np.argsort(fiedler)]
        order.append(None)                                 # a gap between components
    slots = len(order)
    out = {}
    for k, i in enumerate(order):
        if i is not None:
            out[names[i]] = 2 + band * (k + 0.5) / max(1, slots)
    return out
