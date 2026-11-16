"""The archetypes speak with their own neuronal brains: practice for the digital brain, made by the network itself.

A meaning (brick's tidy words, handed over by the makers: the scrambler, the window, file and other makers) goes
to the imagination network (trainer/imagination/sim.js, Network.speak) with one archetype's brain: its personality
(which of Se, Si, Ni, Ne, Ti, Te, Fi, Fe lead) and its decisions (how strict, how warm). The network's own
movements say it: the meaning's words are stimulated, heat spreads along lines, radiant lights up words used
alike, Ni's predictions set the order, cold small words drop out, hot chatter gets said, heat makes slips. The words
that carry the meaning are kept exactly, and every saying is checked to keep the meaning (voices.problem), so the
meaning is always known. The language model only complements this (trainer/voices.py), now and then.

    python3 -m trainer.speech "write hello in red at the top"     how each of a few archetypes says it
    python3 -m trainer.speech practice 1000                       1000 paragraphs into the brain's practice
"""

import atexit
import json
import os
import random
import re
import shutil
import subprocess
import sys

HERE = os.path.dirname(os.path.abspath(__file__))
sys.path.insert(0, os.path.dirname(HERE))

from trainer import voices  # noqa: E402

NETWORK = os.path.join(HERE, "data", "neurons", "network.json")
RUN = os.path.join(HERE, "imagination", "run.js")
SMALL = ["the", "a", "of", "at", "in", "to", "on", "that", "side", "screen", "colour", "color"]
CHATTER = ["pls", "ok", "so", "lol", "can", "u", "thx", "yeah", "hey", "um", "just", "now", "real", "quick"]
UNIT = re.compile(r"(?i)^(ms|millisecond|second|sec|minute|min|hour|hr|day|time|pixel|px|step|beat|frame)s?$")
BOUNDARY = {"in", "at", "after", "to", "as", "called", "saying", "with", "from", "on"}


def pieces(tidy):
    """The meaning in pieces, each word {text, keep}: the words that carry the meaning are kept."""
    from trainer import details
    pairs = details.voiced_roles(tidy, tidy)
    keep = {"W", "C", "P", "N", "T", "D", "M", "A", "S", "H", "G", "B", "K", "Q"}
    out, current, in_shown = [], [], False
    for k, (w, r) in enumerate(pairs):
        starts = r == "V" or (w.lower() in BOUNDARY and not in_shown) or (r == "W" and not in_shown and
                                                                           not (current and current[-1]["text"].lower() in ("the", "words", "saying", "says")))
        if starts and current:
            out.append(current)
            current = []
        unit = k and pairs[k - 1][1] in ("N", "T", "A") and UNIT.match(w)   # "1 second": the unit is the amount too
        current.append({"text": w, "keep": r in keep or bool(unit)})
        in_shown = r == "W" and k + 1 < len(pairs) and pairs[k + 1][1] == "W"
    if current:
        out.append(current)
    return out


_NODE = None                                         # one node kept running (run.js --serve): it reads the network once


def _ask(node, job):
    """One job for the kept node; a fresh node run if it isn't there or fails."""
    global _NODE
    line = json.dumps(job)
    try:
        if _NODE is None or _NODE.poll() is not None:
            _NODE = subprocess.Popen([node, RUN, "--serve"], stdin=subprocess.PIPE, stdout=subprocess.PIPE,
                                     stderr=subprocess.DEVNULL, text=True, bufsize=1)
            atexit.register(_NODE.kill)
        _NODE.stdin.write(line + "\n")
        _NODE.stdin.flush()
        answer = json.loads(_NODE.stdout.readline())
        if "error" not in answer:
            return answer
    except (OSError, ValueError):
        pass
    if _NODE is not None:
        _NODE.kill()
        _NODE = None
    done = subprocess.run([node, RUN], input=line, capture_output=True, text=True, timeout=600)
    try:
        return json.loads(done.stdout)
    except ValueError:
        return {}


def speak(tidy_lines, archetype, seed=1):
    """How this archetype's network says each tidy line -> [said]."""
    node = shutil.which("node")
    if not node or not os.path.isfile(NETWORK):
        return []
    job = {"task": "speak", "state": NETWORK, "brain": archetype, "seed": seed, "small": SMALL, "chatter": CHATTER,
           "meanings": [pieces(t) for t in tidy_lines]}
    return _ask(node, job).get("said", [])


def practice(count, seed=None, say=print):
    """count paragraphs said by the archetypes' own networks, each archetype in turn -> the brain's practice."""
    from trainer import details, scrambler
    rng = random.Random(seed)
    names = [a["name"] for a in voices.archetypes()] + [a["name"] for a in voices.yours()]
    made, tried, turn = 0, 0, []
    while made < count and tried < count * 4:
        if not turn:
            turn = names[:]
            rng.shuffle(turn)
        name = turn.pop()
        batch = []                                   # one archetype says a few paragraphs in one go
        for _ in range(10):
            kinds = rng.choice([scrambler.MAKERS, details.WINDOW_MAKERS, details.FILE_MAKERS, details.OTHER_MAKERS])
            tidy = []
            for _ in range(rng.randint(1, 3)):
                part = rng.choice(kinds)()
                tidy += getattr(part, "before", []) + [part.tidy]
            batch.append(tidy)
        flat = [t for tidy in batch for t in tidy]
        said = speak(flat, name, seed=rng.randint(1, 10 ** 6))
        if len(said) != len(flat):
            tried += len(batch)
            continue
        k = 0
        for tidy in batch:
            parts = said[k:k + len(tidy)]
            k += len(tidy)
            tried += 1
            if any(voices.problem(p, t) for p, t in zip(parts, tidy)):
                continue
            mess = ""
            for j, p in enumerate(parts):
                mess += (rng.choice(scrambler.JOINS) if j else "") + p
            with open(voices.PRACTICE, "a", encoding="utf-8") as f:
                f.write(json.dumps({"mess": mess, "parts": parts, "tidy": tidy, "voice": name + " (neurons)"}) + "\n")
            made += 1
        if made and made % 100 < 10:
            say(f"  {made} said by the archetypes' networks ({tried} tried)", flush=True)
    say(f"The archetypes' networks said {made} paragraphs ({tried} tried) -> {voices.PRACTICE}")
    return made


def command(args):
    if args[:1] == ["practice"]:
        return practice(int(args[1]) if args[1:] else 500)
    if args:
        line = " ".join(args)
        for a in (voices.yours()[:1] + random.sample(voices.archetypes(), 4)):
            print(f"{a['name']:28} {speak([line], a['name'], seed=random.randint(1, 999))}")
        return
    print(__doc__)


if __name__ == "__main__":
    command(sys.argv[1:])
