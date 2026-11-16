"""Archetype voices: brick's brain makes its own messy practice sentences, in the voices of its 48 archetypes.

The scrambler (trainer/scrambler.py) makes tidy directions with a known meaning. Here the language model on this
computer (trainer/neural.py) rewrites them the way one archetype would type them: its type, bias and personality
steer the style (typos, slang, word order, punctuation, polite or blunt, terse or chatty). Every direction, its
order, colours, numbers and places stay, and the words to show stay exactly as they are. An answer that drops
or changes any of them, or adds new ones, is thrown away, so the meaning is always known:

  'write hello in red at the top then wait 2 seconds'
  -> 'ok so top of the screen, hello in red pls... then like 2 secs wait'    (voice: The Soft Rebel)

The trainer practises on these some of the time (trainer/core.py practise()), and
    brick trainer voices 10      makes 10, checks how brick reads each, and keeps them in trainer/data/voiced.txt
"""
import json
import os
import random
import re
import sys

HERE = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
sys.path.insert(0, HERE)

from trainer import neural, scrambler  # noqa: E402

VOICED = os.path.join(HERE, "trainer", "data", "voiced.txt")
COLOURS = set(scrambler.COLOURS) | {"black", "grey", "gray", "brown", "violet", "silver", "maroon", "olive",
                                    "indigo", "turquoise", "beige", "aqua"}
PLACES = set(scrambler.SIDES) | {"middle", "centre", "center"}
QUOTED = re.compile(r'"([^"]+)"|“([^”]+)”|‘([^’]+)’|(?<![A-Za-z])\'([^\']+)\'(?![A-Za-z])')

STYLE = {   # the bias: how the archetype types
    "strict": "careful about meaning but rushed: all lowercase, clipped words, hardly any punctuation",
    "swift": "very terse and fast: short forms (u, pls, bg, secs, thx), drops little words, no punctuation",
    "explore": "chatty and rambling: filler like 'ok so', 'um', 'maybe', 'lol', and a typo or two",
    "keep": "polite and steady: please and thanks, but sloppy spelling and odd commas",
    "split": "jumps around: odd word order, the details before the verb, run-on sentences",
    "order": "blunt commands: semicolons, a word in CAPS for emphasis, no please",
}

GUIDE = """You rewrite tidy directions for a computer the way one particular person would type them: messy and casual.
Keep every direction, in the same order. Keep every colour, number and place (left, right, top, bottom, middle).
Numbers stay as digits. The words to show must be copied EXACTLY, letter for letter, same case, never changed,
never translated, never misspelled; you may put them in double quotes. Do not add any new words to show,
colours, numbers or directions. Anything else may be messy: typos, slang, short forms, word order, punctuation.
Answer in JSON: {"parts": ["the first direction, rewritten", "the second direction, rewritten", ...]},
one part for each direction."""

EXAMPLE = (["write well done in green at the top", "wait 3 seconds"],
           {"parts": ["ok top of screen well done in green pls", "then like 3 secs wait"]})

_archetypes = None


def archetypes():
    """The 48 archetypes from the engine (kept for this run only); [] when the engine can't be reached."""
    global _archetypes
    if _archetypes is None:
        try:
            from trainer import neurons
            _archetypes = [a for a in neurons.archetypes() if isinstance(a, dict) and a.get("name")]
        except Exception:
            _archetypes = []
    return _archetypes


def pick(archetype=None):
    """An archetype dict: the one given (a dict, or a name), or a random one. None if there are none."""
    if isinstance(archetype, dict):
        return archetype
    found = archetypes()
    if archetype:
        want = archetype.lower().removeprefix("the ").strip()
        return next((a for a in found if a["name"].lower().removeprefix("the ").strip() == want), None)
    return random.choice(found) if found else None


YOUR_WAY = ("you often say where it goes first ('at the left side of the screen put ...', 'on the top side of the "
            "screen write ...'); you name what to show with 'the word X', 'the text X' or 'with the words X'; you say "
            "colours as 'in the color X' or 'put red text ...'; places as 'to the bottom right of screen', 'at left side "
            "of window', 'on that window'; you ask politely ('can you write X', 'can you write X on the screen'); the "
            "thing to show can itself be a colour word; lower case, few commas, the odd slip ('wirte', 'winodw')")


def manner(archetype):
    """The archetype's way of typing, in words for the model. Your own archetypes type the way you do."""
    kind = archetype.get("type", "")
    letters = [("chatty, says more than needed" if kind[:1] == "E" else "terse, says as little as it can"),
               ("blunt" if kind[2:3] == "T" else "friendly"),
               ("in a hurry" if kind[3:4] == "P" else "methodical")] if len(kind) == 4 else []
    if archetype.get("yours"):
        return YOUR_WAY + "; " + "; ".join(letters)
    return "; ".join([STYLE.get(archetype.get("bias"), "casual")] + letters)


def yours():
    """The archetypes that are yours (the owner's own types)."""
    return [a for a in archetypes() if a.get("yours")]


# ---------------- what a tidy line means ----------------

def shows(tidy):
    """The words to show in one tidy line ('write hello there in red at the top 3 times' -> ['hello there'])."""
    colour = "|".join(sorted(COLOURS, key=len, reverse=True))
    found = [m.group(1) for m in re.finditer(rf"\bwrite (.+?)(?= in the window\b| to [\w-]+\.txt\b| in (?:{colour})\b"
                                             rf"| at the | \d+ times\b|$)", tidy)]
    found += [m.group(1) for m in re.finditer(r"^(?:save|add) (.+?) to (?:the end of )?[\w-]+\.txt$", tidy)]
    found += [m.group(1) for m in re.finditer(r'^ask "(.+?)"', tidy)]
    found += [m.group(1) for m in re.finditer(rf"^make a button called \w+ saying (.+?)(?= in (?:{colour})\b| at the |$)",
                                              tidy)]
    return found


def names(tidy):
    """Names and items a tidy line uses, which every voicing must keep: a value's, a list's, a button's, a question's
    answer, a list's items."""
    found = re.findall(r"(?:called|^set|^ask \"[^\"]*\" as) (\w+)", tidy)
    found += re.findall(r"(?:to|from) (\w+)$", tidy) if re.match(r"^(add|take) \d+ ", tidy) else []
    items = re.match(r"^make a list called \w+ with (.+)$", tidy)
    if items:
        found += [w for w in re.split(r",\s*|\s+and\s+", items.group(1)) if w]
    return found


def files(tidy):
    return re.findall(r"[\w-]+\.txt", tidy)


def without_shows(text, words):
    for w in sorted(words, key=len, reverse=True):
        text = re.sub(rf"(?<![A-Za-z]){re.escape(w)}(?![A-Za-z])", " ", text)
    return text


def found_words(text, choices):
    return sorted(w for w in re.findall(r"[a-z]+", text.lower()) if w in choices)


def problem(said, tidy):
    """Why a messy rewrite doesn't mean exactly what the tidy line means, or None if it does."""
    if not isinstance(said, str) or not said.strip() or "\n" in said.strip() or len(said) > 300:
        return "not one short line"
    if re.search(r"\b(keep|show exactly|show|colour|color|number|place)\s*:", said, re.IGNORECASE):
        return "it copied the notes it was given"
    words = shows(tidy)
    for w in words:
        if not re.search(rf"(?<![A-Za-z]){re.escape(w)}(?![A-Za-z])", said):
            return f"the words to show '{w}' are missing or changed"
    rest, plain = without_shows(said, words), without_shows(tidy, words)
    extra = [next(g for g in m.groups() if g is not None) for m in QUOTED.finditer(rest)]
    extra = [e for e in extra if e.strip(" .,!?")]
    if extra:
        return f"new words to show: '{extra[0]}'"
    for name, choices in (("colours", COLOURS), ("places", PLACES)):
        if found_words(rest, choices) != found_words(plain, choices):
            if set(found_words(plain, choices)) - set(found_words(rest, choices)):
                return f"{name} missing: {', '.join(sorted(set(found_words(plain, choices)) - set(found_words(rest, choices))))}"
            if set(found_words(rest, choices)) - set(found_words(plain, choices)):
                return f"new {name}: {', '.join(sorted(set(found_words(rest, choices)) - set(found_words(plain, choices))))}"
    if sorted(re.findall(r"\d+", rest)) != sorted(re.findall(r"\d+", plain)):
        return f"numbers changed: {' '.join(re.findall(r'[0-9]+', rest)) or 'none'} for {' '.join(re.findall(r'[0-9]+', plain))}"
    for n in names(tidy):                                   # names and items stay
        if not re.search(rf"(?<![A-Za-z]){re.escape(n)}(?![A-Za-z])", said, re.IGNORECASE):
            return f"the name '{n}' is missing or changed"
    for f in files(tidy):                                   # file names stay exactly as they are
        if f not in said:
            return f"the file {f} is missing or changed"
    if files(said) and set(files(said)) != set(files(tidy)):
        return "new files"
    if ("window" in tidy) != bool(re.search(r"win+do?w", said, re.IGNORECASE)):
        return "the window is missing" if "window" in tidy else "a new window"
    others = [w for w in scrambler.WORDS if w not in words and re.search(rf"\b{re.escape(w)}\b", rest)
              and not any(w in k for k in words)]
    if others:
        return f"new words to show: '{others[0]}'"
    return None


# ---------------- asking the model ----------------

def ask(tidy, archetype):
    """The model's messy parts for the tidy lines, in this archetype's voice ([] if it couldn't answer)."""
    def listing(lines):
        out = []
        for k, line in enumerate(lines, 1):
            keep = [f"show exactly: {w}" for w in shows(line)]
            keep += [f"colour: {c}" for c in found_words(without_shows(line, shows(line)), COLOURS)]
            keep += [f"number: {n}" for n in re.findall(r"\d+", without_shows(line, shows(line)))]
            keep += [f"place: {p}" for p in found_words(without_shows(line, shows(line)), PLACES)]
            keep += [f"file: {f}" for f in files(line)] + (["it's about the window"] if "window" in line else [])
            keep += [f"name: {n}" for n in names(line)]
            out.append(f"{k}. {line}" + (f"   (keep {'; '.join(keep)})" if keep else ""))
        return "\n".join(out)

    persona = (f"You are {archetype['name']} ({archetype.get('type', '')}): {archetype.get('line', '')}\n"
               f"How you type: {manner(archetype)}.")
    messages = [{"role": "system", "content": GUIDE + "\n\n" + persona},
                {"role": "user", "content": listing(EXAMPLE[0])},
                {"role": "assistant", "content": json.dumps(EXAMPLE[1])},
                {"role": "user", "content": listing(tidy)}]
    try:
        answer = neural.call("/api/chat", {"model": neural.settings()["model"], "messages": messages, "stream": False,
                                           "format": "json", "options": {"temperature": 0.9, "num_predict": 300}},
                             seconds=90)
        parts = json.loads(answer.get("message", {}).get("content", "{}")).get("parts", [])
    except Exception:
        return []
    return [re.sub(r"\s+", " ", p).strip() for p in parts if isinstance(p, str) and p.strip()] \
        if isinstance(parts, list) else []


def attempt(tidy, archetype=None):
    """-> (mess, its parts, the archetype's name, why it was refused or None). None if no model or archetype."""
    if neural.off() or not neural.ready():
        return None
    chosen = pick(archetype)
    if not chosen:
        return None
    parts = ask(tidy, chosen)
    if len(parts) != len(tidy):
        return " ".join(parts), parts, chosen["name"], f"{len(parts)} parts for {len(tidy)} directions"
    for said, line in zip(parts, tidy):
        why = problem(said, line)
        if why:
            return " ".join(parts), parts, chosen["name"], why
    mess = ""
    for k, said in enumerate(parts):
        mess += (random.choice(scrambler.JOINS) if k else "") + said
    return mess, parts, chosen["name"], None


def voice(tidy, archetype=None):
    """-> (mess, its messy parts, the archetype's name), or None when the model is off or its answer doesn't fit."""
    found = attempt(tidy, archetype)
    if not found or found[3]:
        return None
    return found[:3]


def voiced(tidy_lines, archetype=None):
    """The tidy brick lines as one messy line in an archetype's voice -> (mess, archetype name), or None
    (no model, or an answer that lost or added a word to show, colour, number or place: use the scrambler then)."""
    found = voice(tidy_lines, archetype)
    return (found[0], found[2]) if found else None


# ---------------- practice for the brain: many voiced directions, each with its meaning ----------------

PRACTICE = os.path.join(HERE, "trainer", "data", "voiced-practice.jsonl")


def practice(count, seed=None, workers=3, say=print):
    """The archetypes voice count paragraphs (meanings handed over by the scrambler and the window and file
    makers) -> appended to trainer/data/voiced-practice.jsonl: {mess, parts, tidy, voice}, one per line. Only
    voicings that keep the meaning exactly are kept."""
    import json as _json
    import threading
    from trainer import details
    if neural.off() or not neural.ready():
        say("The language model isn't here or is switched off, so the archetypes can't speak.")
        return 0
    rng = random.Random(seed)
    lock, made, tried = threading.Lock(), [0], [0]
    turns = []

    def next_voice():
        """The archetypes take turns: every one practises equally, in its own way (yours twice a round)."""
        with lock:
            if not turns:
                turns.extend(archetypes() + yours())
                rng.shuffle(turns)
            return turns.pop() if turns else None

    def one():
        while True:
            with lock:
                if made[0] >= count or tried[0] >= count * 4:
                    return
                tried[0] += 1
                kinds = rng.choice([scrambler.MAKERS, details.WINDOW_MAKERS, details.FILE_MAKERS,
                                    details.OTHER_MAKERS])
                tidy = []
                for _ in range(rng.randint(1, 3)):
                    part = rng.choice(kinds)()
                    tidy += getattr(part, "before", []) + [part.tidy]   # ('set score to 10' before 'add 3 to score')
            found = attempt(tidy, next_voice())       # every archetype in turn, each in its own way
            if found is None or found[3] and found[3].startswith("0 parts"):   # no answer: the model is down just
                found = None                                                  # now: wait for it, don't count
                neural.ready()
            if found is None:
                import time
                with lock:
                    tried[0] -= 1
                time.sleep(3)
                continue
            if found[3]:
                continue
            with lock:
                with open(PRACTICE, "a", encoding="utf-8") as f:
                    f.write(_json.dumps({"mess": found[0], "parts": found[1], "tidy": tidy, "voice": found[2]}) + "\n")
                made[0] += 1
                if made[0] % 50 == 0:
                    say(f"  {made[0]} voiced ({tried[0]} asked)", flush=True)
    threads = [threading.Thread(target=one) for _ in range(workers)]
    for t in threads:
        t.start()
    for t in threads:
        t.join()
    say(f"The archetypes voiced {made[0]} paragraphs ({tried[0]} asked) -> {PRACTICE}")
    return made[0]


# ---------------- brick trainer voices N ----------------

def main(args):
    if args[:1] == ["practice"]:
        return practice(int(args[1]) if args[1:] else 500)
    count = int(args[0]) if args and args[0].isdigit() else 10
    if neural.off() or not neural.ready():
        print("The language model isn't here or is switched off (brick trainer neural setup), so there are no voices.")
        return 1
    if not archetypes():
        print("The archetypes couldn't be read (brick trainer neurons --archetypes).")
        return 1
    checker, tally, refused, made = scrambler.Checker(), {"right": 0, "wrong": 0, "refused": 0}, 0, 0
    try:
        for _ in range(count * 3):
            if made >= count:
                break
            _, _, tidy = scrambler.paragraph()
            found = attempt(tidy)
            if found is None:
                continue
            mess, said, name, why = found
            if why:
                refused += 1
                print(f"(thrown away, {why}: '{mess}'  voice: {name})")
                continue
            how, missed = checker.check(mess, said, tidy)
            if how is None:
                continue
            made += 1
            tally[how] += 1
            entry = f"{'✓' if how == 'right' else '✗'} {mess}\n    meant: {' then '.join(tidy)}\n    voice: {name}\n"
            print(entry, end="")
            for messy, meant, what in missed:
                print(f"    {what}: '{messy}'  (meant: {meant})")
            os.makedirs(os.path.dirname(VOICED), exist_ok=True)
            with open(VOICED, "a", encoding="utf-8") as f:
                f.write(entry)
    finally:
        checker.done()
    print(f"\n{made} voiced paragraphs ({refused} more thrown away for changing the meaning). "
          f"brick read {tally['right']} right; {tally['wrong']} misread, {tally['refused']} refused. "
          f"Kept in {VOICED}")
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
