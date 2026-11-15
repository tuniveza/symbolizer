"""The inventor: when brick has no action for what a line asks, the brain invents one.

A new action is a definition made only of what brick can already do ("to splash something in a word means set
the background colour to word then write something"), so it compiles to real machine code with no new routines.
The language model drafts a few definitions; each is kept only if teach can read it, it uses nothing but brick's
own words and the person's, and (in repair) the program builds with it and its build makes what the brain
expected the line to make. The definition goes into the program at the top, where anyone can read and change it.

    invent(line, before="") -> ["to VERB ... means ...", ...]   (drafts, best first; [] when the model is off)
"""
import json
import re

from trainer import intuition, neural

ASK = """brick is a tiny programming language written in plain English. It has no action for a word in a line
someone wrote. Invent that action as a brick definition, built ONLY from brick's own directions:
  write WORDS · write WORDS in COLOUR · write WORDS at the top|bottom|left|right|middle · write WORDS N times
  wait N seconds · after N seconds write WORDS · clear the screen · beep · stop
  set the background colour to COLOUR · set the colour to COLOUR · make it say WORDS · set the title to WORDS
  put the console on the left|right|top|bottom side of the screen · make the console bigger|smaller|full screen
A definition looks like:  to VERB [something] [in a word] [a number] means DIRECTION then DIRECTION
Holes: "something" = the words to show, "a word" = one word such as a colour, "a number" = a number.
In the directions after "means", use the hole's name (something, word, number) where its value goes.
Examples:
  line: flash the screen          -> to flash the screen means set the background colour to white then wait 1 seconds then set the background colour to black
  line: shout hello               -> to shout something means write something in bold then beep
  line: splash hi in red          -> to splash something in a word means set the background colour to word then write something
Reply with JSON only: {"definitions": ["best", "second best"]}"""

HOLE_NAMES = "something someone somebody number word"


def unknown_action(line):
    """The word brick has no action for: the line's first word, when brick doesn't read it as an action."""
    words = re.findall(r"[a-z']+", line.lower())
    if not words:
        return None
    first = words[0]
    if intuition.role_of(first) == "do" or first in ("write", "say", "show", "put", "set", "make", "wait", "clear"):
        return None
    from trainer import wordnet
    if not wordnet.senses(first, "v"):
        return None                                      # not a doing word ('hello world' is words to show)
    return first


def invent(line, before="", verb=None):
    """Draft definitions for the action this line needs (best first). [] when there's nothing to invent."""
    from teach import read_definition
    verb = verb or unknown_action(line)
    if not verb or neural.off() or not neural.ready():
        return []
    asked = (f"(the lines before it: {before.strip()[-200:]})\n" if before.strip() else "") + \
        f"line: {line.strip()}\nThe word brick has no action for: {verb}"
    try:
        answer = neural.call("/api/chat", {"model": neural.settings()["model"], "stream": False, "format": "json",
                                           "messages": [{"role": "system", "content": ASK},
                                                        {"role": "user", "content": asked}],
                                           "options": {"temperature": 0.2, "num_predict": 300}})
        found = json.loads(answer.get("message", {}).get("content", "{}")).get("definitions", [])
    except Exception:
        return []
    out = []
    for d in found:
        if not isinstance(d, str):
            continue
        d = re.sub(r"\s+", " ", d).strip().rstrip(".")
        if not d.lower().startswith("to "):
            d = "to " + d
        got = read_definition(d)
        if not got or got.first_word() != verb:
            continue                                     # not a definition of this word
        if not neural.only_known_words(got.body, line + " " + HOLE_NAMES):
            continue                                     # it made up words: brick couldn't build what it means
        if d not in out:
            out.append(d)
    return out[:3]
