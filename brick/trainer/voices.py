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


def manner(archetype):
    """The archetype's way of typing, in words for the model."""
    kind = archetype.get("type", "")
    letters = [("chatty, says more than needed" if kind[:1] == "E" else "terse, says as little as it can"),
               ("blunt" if kind[2:3] == "T" else "friendly"),
               ("in a hurry" if kind[3:4] == "P" else "methodical")] if len(kind) == 4 else []
    return "; ".join([STYLE.get(archetype.get("bias"), "casual")] + letters)


# ---------------- what a tidy line means ----------------

def shows(tidy):
    """The words to show in one tidy line ('write hello there in red at the top 3 times' -> ['hello there'])."""
    colour = "|".join(sorted(COLOURS, key=len, reverse=True))
    return [m.group(1) for m in re.finditer(rf"\bwrite (.+?)(?= in (?:{colour})\b| at the | \d+ times\b|$)", tidy)]


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
    return [re.sub(r"\s+", " ", p).strip() for p in parts if isinstance(p, str)] if isinstance(parts, list) else []


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


# ---------------- brick trainer voices N ----------------

def main(args):
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
