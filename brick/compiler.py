"""The compiler: turns lines of loose English into a runnable Linux program.

A line splits into parts at commas, full stops and "then"; each part is one or more
actions joined by "and".
  "N times", "forever" or "until CHECK" in a part repeats that whole part;
  "while CHECK ..." repeats the rest of its part for as long as the check is true.
  "for each X in L ..." repeats the rest of its part once per item.
  "if CHECK ..." runs the rest of its part (or, if nothing follows, the next part) only when
  the check is true; an "otherwise ..." part right after runs when it isn't.
A loop or "if" with nothing after it ("repeat 3 times:", "if x is 3,") takes the next part;
at the end of a line, it takes the lines indented under it (a block), and an "otherwise"
line straight after belongs to the "if". Blocks can hold blocks.
Polite extra words at the start of an action ("please", "can you", "computer,") are skipped.
"""
import difflib
import os
import re
import shutil

import dictionary
import runtime
import teach
from actions import BRAIN_WORDS, EXAMPLES, RESET, STARTERS, Actions, case_at_end, colour_words, file_name, plain_name, split_at
from window import WINDOW_HELP, Window
from bitwords import BitWords
from asm import Asm, EQUAL, GREATER, LESS, NOT_GREATER, NOT_LESS, R8, RAX, RCX, RDX
from elf import wrap
from expressions import Emitter, Parser, Problem, item_kind, read_condition, read_value
from reader import Word, group_parts, split_line
from runtime import LIST_SIZE
from spelling import slip_of

FILLER = {"please", "pls", "now", "also", "just", "then", "and", "so", "ok", "okay", "computer",
          "can", "could", "would", "will", "you", "kindly", "ahead", "i", "want", "need", "to",
          "lets", "let's", "first", "next", "finally", "after", "that", "hey", "well", "right",
          "alright", "and then", "we"}
TIME_UNITS = {"seconds", "second", "secs", "sec", "s", "milliseconds", "millisecond", "ms",
              "minutes", "minute", "mins", "min"}
STAND_ALONE = {"beep", "stop", "end", "quit", "exit", "leave", "break", "clear", "wait", "pause", "sleep",
               "press", "blank", "new", "repeat", "forever"}      # actions that need nothing after them
CONTROL = {"if", "when", "whenever", "unless", "while", "until", "repeat", "for", "each", "every", "forever",
           "otherwise", "else", "means", "mean", "call", "called", "define", "teach", "loop", "do"}
DESCRIBED = re.compile(r"\b(?:text|words?|message|msg|note|label|sign|caption|writing|letters)\s+"
                       r"(?:that says|which says|saying|that reads|which reads|reading|says)\b")
VANISHING = re.compile(r"\b(disappear|vanish|go away|fade|evaporate|hide|rub out)\b", re.IGNORECASE)
INDENT_HINT = ". Or put it on the lines below, each one indented with spaces."
CURLY = str.maketrans({"“": '"', "”": '"', "„": '"', "″": '"',
                       "‘": "'", "’": "'"})


def failing_lines(text, words_file):
    """The numbers of the lines brick can't read in this text (none if it builds)."""
    try:
        Compiler(words_file, ask_user=lambda question: "no").compile_text(text)
        return set()
    except Problem as problem:
        return {int(n) for n in re.findall(r"^Line (\d+): ", str(problem), re.MULTILINE)}
    except Exception:
        return {0}


def built(text, words_file):
    """The program, built by brick's own language as written, or None."""
    try:
        return Compiler(words_file, ask_user=lambda question: "no").compile_text(text)
    except Exception:
        return None




class Var:
    def __init__(self, name, kind, place):
        self.name, self.kind, self.place = name, kind, place


class LonelyOtherwise(Problem):
    """An 'otherwise' with no 'if' to belong to."""


def read_blocks(text):
    """The program's lines as (line number, words, the lines indented under it or None).
    Blank lines and # notes are left out."""
    lines = []
    for number, raw in enumerate(text.splitlines(), 1):
        raw = raw.expandtabs(4).translate(CURLY).rstrip()
        if raw.strip() and not raw.strip().startswith("#"):
            lines.append((number, raw))
    return gather(lines, 0, -1)[0]


def gather(lines, i, outer):
    """The lines from i on that are indented more than outer -> (items, where they stop)."""
    items = []
    while i < len(lines) and indent(lines[i][1]) > outer:
        number, raw = lines[i]
        i += 1
        children = None
        if i < len(lines) and indent(lines[i][1]) > indent(raw):
            children, i = gather(lines, i, indent(raw))
        items.append((number, raw.strip(), children))
    return items, i


def indent(line):
    return len(line) - len(line.lstrip())


def otherwise_line(line):
    """'otherwise write no' -> 'write no'; 'otherwise:' -> ''; None if the line isn't an otherwise."""
    found = re.fullmatch(r"(?:otherwise|else)\b[\s,:;.-]*(.*)", line, re.IGNORECASE)
    return found.group(1) if found else None


class Compiler(Actions, Window, BitWords):
    def __init__(self, words_file="mywords.txt", ask_user=input):
        self.a = Asm()
        self.emitter = Emitter(self)
        self.scopes = [{}]
        self.loop_ends = []            # where "leave the loop" jumps to, innermost last
        self.last_timer = None
        self.coloured = False          # put colours back to normal at the end?
        self.uses_window = False       # does it move its own window? Then it needs one of its own.
        self.bricks_on = {}            # line number -> the bricks its directions became, in order
        self.erases = False            # does it make text disappear? Then text put on the screen is remembered.
        self.written = False           # has anything been shown yet? Until then a background colours the whole screen
        self.uses_brain = False        # does it link symbols? Then it carries the brain's routines too
        self.worked_out = set()        # lines where write fills in a value it works out ("how many in pets")
        self.actions_on = {}           # line number -> how many actions brick read on it
        self.texts_on = {}             # line number -> the texts its own directions used (not the toolbox's)
        self.last_shown = None         # what "make it disappear" means: "placed" text, a "line", or nothing yet
        self.known_names = set()       # for "did you mean ...?"
        self.words_file = words_file
        self.ask_user = ask_user
        self.definitions = teach.load(words_file)
        self.compiled_words = {}       # (word, kinds of what fills its holes) -> (label, holes)
        self.made_just_now = None      # a name 'make a variable called x' just made, for "set it to 5"
        self.made_before = None
        self.notes = []                # "Note: line 3: 'scroe' looks like 'score'." for likely typos
        self.line_number = 0
        self.dictionary_starts = dictionary.first_words("action", "loop")    # scribble, inquire, turn ...
        self.gui_setup()               # brick's own windows (bitwords.py): off until the text talks about them

    # ---- the whole program ----

    def compile(self, text):
        """Return the program's bytes. brick's own language builds as it's written; how a messy line is read is
        decided by the digital brain alone (trainer/mind.py: the imagination neurons and the archetypes' practice).
        Raises Problem listing every line that needs fixing."""
        from trainer import mind
        lines, notes = text.split("\n"), []
        for number, raw in enumerate(lines, 1):
            line = raw.strip()
            if not line or line.startswith("#") or line.endswith(":") or teach.read_definition(line) or \
                    self.uses_definition(line):
                continue
            way = mind.read(line, self.words_file)
            if way and way.lower() != line.lower():
                lines[number - 1] = raw[:len(raw) - len(raw.lstrip())] + way
                notes.append(f"Note: line {number} read as: {way}")
        try:
            return self.compile_text("\n".join(lines))
        finally:
            self.notes[:0] = notes

    def uses_definition(self, line):
        """Does this line use one of the words taught (in mywords.txt or earlier in the program)?"""
        from reader import words_of
        words = words_of(line)
        return any(teach.match(d, words) is not None for d in self.definitions)

    def compile_text(self, text):
        runtime.start(self.a)
        self.erases = bool(VANISHING.search(text))
        self.gui_setup(text)
        problems = []
        self.compile_items(read_blocks(text), problems)
        if problems:
            raise Problem("\n".join(problems))
        self.gui_finish()                      # a window it opened stays open until it's closed
        if self.coloured:
            self.show(RESET)
        runtime.finish(self.a)
        runtime.add_all(self.a)
        if self.uses_brain:
            runtime.add_brain(self.a)
        self.gui_routines()
        return wrap(self.a)

    def compile_items(self, items, problems):
        k = 0
        while k < len(items):
            k = self.compile_item(items, k, problems)

    def compile_item(self, items, k, problems, line=None):
        """Compile items[k], with the indented lines under it, and any 'otherwise' lines straight after it.
        line: the words to compile, if not the whole line ('otherwise if x is 2:' -> 'if x is 2:').
        Each line's problems go in problems. Returns where the next item is."""
        number, whole, children = items[k]
        line = re.sub(r"\s+:$", ":", whole if line is None else line)     # "if x is 5 :"
        if children and teach.opens_block(line):        # "to greet someone means:" with its actions below
            lines, children = [child[1] for child in children], [child for child in children if child[2]]
            if children:
                problems.append(f"Line {number}: {whole}\n    The lines of a new word can't have their own "
                                "indented lines yet. Put each one on a single line, like: if someone is Bob, write hi")
                return k + 1
            line = teach.with_body(line, lines)
        after = k + 1
        otherwise, used = None, []              # used: did the 'otherwise' line and the block find a home?
        if after < len(items) and otherwise_line(items[after][1]) is not None:
            def otherwise():
                used.append("otherwise")
                rest = otherwise_line(items[k + 1][1])
                if not rest and not items[k + 1][2]:
                    problems.append(f"Line {items[k + 1][0]}: {items[k + 1][1]}\n    "
                                    "Otherwise what? Like: otherwise write no")
                    return
                self.compile_item(items, k + 1, problems, rest)
            while after < len(items) and otherwise_line(items[after][1]) is not None:
                after += 1                      # 'otherwise if ...' lines take the next one in turn

        def block():
            used.append("block")
            self.compile_items(children, problems)
        self.line_number = number
        try:
            if line.count('"') % 2:
                raise Problem('A quote mark " is missing its partner. Quotes go in pairs: write "hello"')
            if line:
                start = len(self.a.text_log)
                self.compile_line(line, block if children else None, otherwise)
                self.texts_on.setdefault(number, set()).update(self.a.text_log[start:])
            if otherwise and "otherwise" not in used:
                raise LonelyOtherwise("'otherwise' needs an 'if' just before it.")
        except LonelyOtherwise as problem:
            if otherwise and "otherwise" not in used:       # it's the next line's 'otherwise' that's alone
                problems.append(f"Line {items[k + 1][0]}: {items[k + 1][1]}\n    {problem} "
                                "Put it straight after its 'if', lined up with it.")
            else:
                problems.append(f"Line {number}: {whole}\n    {problem}")
        except Problem as problem:
            problems.append(f"Line {number}: {whole}\n    {self.with_suggestion(str(problem))}")
        if children and "block" not in used:
            self.compile_items(children, problems)          # lines under a line that isn't a loop or 'if'
        return after

    def with_suggestion(self, message):
        """Add 'Did you mean ...?' (or how to make it) right after "I don't know what 'x' is yet."."""
        unknown = re.search(r"I don't know (?:what )?'([^']+)'(?: is)? yet\.?", message)
        if not unknown:
            return message
        word = unknown.group(1).lower()
        close = difflib.get_close_matches(word, [n.replace("_", " ") for n in self.known_names], n=1)
        if close == [word]:                     # a 'for each' name, used after its loop
            hint = f" ('{word}' only exists inside the loop or new word that made it.)"
        elif close:
            hint = f" Did you mean '{close[0]}'?"
        elif re.fullmatch(r"[a-z]\w*", word) and not re.search(r"(it|list) first", message):
            hint = f" Set it first, like: set {word} to 0"
        else:
            return message
        return message[:unknown.end()] + hint + message[unknown.end():]

    def compile_line(self, line, block=None, otherwise=None):
        """block: compiles the lines indented under this one; otherwise: compiles the 'otherwise' line after it."""
        if self.gui_event(line, block):          # "when start is clicked, ...", "every 2 seconds, ..."
            return None
        definition = teach.read_definition(line)
        if definition:
            return self.learn(definition)
        whole = self.gui_whole_line(line)        # "make a menu called file with New, Open and Quit": one phrase
        if whole:
            return self.compile_action(whole)
        first = line.split()[0].lower()
        if teach.unfinished(line) and (first == "to" or first not in STARTERS):    # not "write it means"
            head = teach.unfinished(line)
            example = head if head.split()[0].lower() in ("a", "an", "the") else "to " + head
            raise Problem(f"What does '{head}' mean? Put the actions after 'means', on the same line: "
                          f"{example} means write hi")
        self.compile_parts(group_parts(split_line(line, self.starts_action)), block, otherwise)

    # ---- names ----

    def find(self, name):
        if name == "last timer":
            return self.last_timer
        for scope in reversed(self.scopes):
            if name in scope:
                return scope[name]
        return None

    def own_names(self):
        return {name for name in self.known_names if "_" not in name and name != "it"}

    def note_slip(self, typed, choices):
        """If typed looks like a slip of one of the choices, say so when it's built (it still builds)."""
        meant = slip_of(typed.lower(), choices)
        note = f"Note: line {self.line_number}: '{typed}' looks like '{meant}'."
        if meant and note not in self.notes:
            self.notes.append(note)

    def kind_of(self, name):
        var = self.find(name)
        return var.kind if var else None

    def new_var(self, name, kind, scope=None):
        if scope is None:
            self.note_slip(name, self.own_names())          # "set scroe to 5" when there's a score
        place = self.a.reserve(LIST_SIZE if kind.startswith("list") else 8, "name")
        var = Var(name, kind, place)
        (scope if scope is not None else self.scopes[0])[name] = var
        self.known_names.add(name)
        return var

    def store(self, name, kind):
        """Save the value in rax under name (made if it's new). rax is kept."""
        a = self.a
        var = self.find(name)
        if var is None:
            var = self.new_var(name, kind)
        a.push(RAX)
        if var.kind.startswith("list"):
            if not kind.startswith("list"):
                said = name.replace("_", " ")
                raise Problem(f"'{said}' is a list. To put something in it, say: add it to {said}")
            a.address(RCX, var.place)
            a.call("list_copy")
            if var.kind == "list":
                var.kind = kind
        else:
            self.emitter.convert(kind, var.kind)
            a.address(RCX, var.place)
            a.store(RCX, 0, RAX)
        a.pop(RAX)

    def set_it(self, kind):
        """'it' always holds the latest answer or item, whatever kind it is."""
        if kind.startswith("list"):
            return
        it = self.scopes[0].get("it") or self.new_var("it", kind)
        it.kind = kind
        self.a.address(RCX, it.place)
        self.a.store(RCX, 0, RAX)

    def value(self, words):
        """Code for a value; returns its kind."""
        return self.emitter.emit(read_value(words, self.kind_of))

    def value_or_text(self, words):
        """A value, or, if the words don't make one, the words as text: 'add apple to fruits'.
        A word that looks like a mistyped name is still an error, so typos don't hide."""
        try:
            return read_value(words, self.kind_of)
        except Problem:
            near = [w.word for w in words if w.word and
                    difflib.get_close_matches(w.word, list(self.known_names), n=1, cutoff=0.8)]
            if near or len(words) > 4:
                raise
            return ("text", " ".join(w.text + w.after if w.quoted else w.raw for w in words))

    # ---- where actions start ----

    def starts_here(self, words, i):
        if i >= len(words) or words[i].word is None:
            return False
        w = words[i].word
        if w in ("name", "call") and not self.names_something(words[i + 1:]):
            return False                        # "write hello, name" is just a name
        if w in BRAIN_WORDS and i and not self.brain_word_fits(words, i):
            return False                        # "write live, laugh, learn" is just words
        if w in STARTERS or any(d.first_word() == w for d in self.definitions):
            return True
        if self.dictionary_action(words, i):
            return True
        if self.delay_at_start(words[i:]):
            return True
        if self.gui_starts(w):
            return True
        for n in range(1, 4):                   # "score = 5", "high score is now 10"
            joiner = [x.word for x in words[i + n:i + n + 2]]
            if joiner[:1] in (["="], ["becomes"]) or joiner == ["is", "now"]:
                return all(x.word and x.word.isidentifier() for x in words[i:i + n])
        return False

    def names_something(self, words):
        """After 'name' or 'call': 'it guess', 'the console My Game' or 'the window My Game'."""
        return len(words) > 1 and (words[0].word in ("it", "that", "this") or
                                   [w.word for w in words[:2]] in (["the", "window"], ["the", "console"],
                                                                   ["the", "terminal"], ["this", "window"],
                                                                   ["the", "game"]))

    def starts_action(self, words, i):
        """Does an action start at words[i]? Polite extra words in front are fine."""
        while i < len(words) and not self.starts_here(words, i) and words[i].word in FILLER:
            i += 1
        return self.starts_here(words, i)

    def delay_at_start(self, words):
        """'after 5 seconds ...', 'in 2 minutes ...', '5 seconds later ...'
        -> (a wait, the rest, which may be empty), else None."""
        said = [w.word for w in words]
        units = TIME_UNITS | dictionary.first_words("time")            # "after 2 hours ..."
        if said[:1] in (["after"], ["in"]):
            for k in range(2, min(len(words), 7)):
                if said[k] in units:
                    return [Word("wait")] + words[1:k + 1], words[k + 1:]
        for k in range(1, min(len(words) - 1, 6)):
            if said[k] in units and said[k + 1] == "later":
                return [Word("wait")] + words[:k + 1], words[k + 2:]
        return None

    def delay_at_end(self, words):
        """'write hi after 5 seconds' -> (a wait, the rest), else None. Waiting words keep their own times."""
        if not words or words[0].word in ("wait", "pause", "sleep", "press") or len(words) < 4:
            return None
        for j in range(len(words) - 2, 0, -1):
            if words[j].word == "after":                     # not "in": "write it ends in 5 minutes"
                delay = self.delay_at_start(words[j:])
                if delay and not delay[1]:
                    rest = words[:j]
                    while rest and rest[-1].word in ("and", "then"):
                        rest = rest[:-1]
                    return (delay[0], rest) if rest else None
        return None

    def vanishes(self, words):
        """'make it disappear', 'make the text vanish', 'let it go away', 'hide it', 'rub it out'."""
        said = [w.word for w in words]
        things = (["it"], ["that"], ["this"], ["them"], ["the", "text"], ["the", "writing"], ["the", "words"],
                  ["the", "message"], ["the", "note"])
        for thing in things:
            n = len(thing)
            if said[:1] in (["make"], ["let"]) and said[1:1 + n] == thing and \
                    said[1 + n:] in (["disappear"], ["vanish"], ["go", "away"], ["fade"], ["fade", "away"], ["evaporate"]):
                return True
            if said[:1] == ["hide"] and said[1:] == thing:
                return True
            if said[:2] == ["rub", "out"] and said[2:] == thing or said[:1] == ["rub"] and said[1:] == thing + ["out"]:
                return True
        return False

    def drop_filler(self, words):
        """'please write hi' -> 'write hi'; 'can you make it say hi' -> 'say hi'."""
        i = 0
        while i < len(words) - 1 and not self.starts_here(words, i) and words[i].word in FILLER:
            i += 1
        if self.gui_keeps(words[i:]):           # "close the window" isn't the dictionary's "stop"
            return words[i:]
        if i + 2 < len(words) and words[i].word in ("make", "have", "let", "get", "tell") \
                and words[i + 1].word in ("it", "me", "us", "the", "them"):
            j = i + 2
            while j < len(words) and words[j].word in ("computer", "program", "to"):
                j += 1
            if self.starts_here(words, j):
                i = j
        if not self.starts_here(words, i):
            return words
        return self.dictionary_action(words, i) or words[i:]

    def dictionary_action(self, words, i=0):
        """The dictionary's meaning for an action starting at words[i]: 'scribble hi' -> the words 'write hi',
        'turn green' -> 'set the colour to green'. None if there isn't one, or your own word or a name comes first."""
        w = words[i].word if i < len(words) else None
        if w not in self.dictionary_starts or self.find(w) or getattr(words[i], "meant", False):
            return None
        after = [x.word for x in words[i + 1:i + 3]]
        if after[:1] in (["="], ["becomes"]) or after == ["is", "now"]:
            return None                 # "delay = 2" makes a name called delay
        if any(teach.match(d, words[i:]) is not None for d in self.definitions):
            return None
        found = dictionary.respell(words[i:], ("action", "loop"), fits=lambda gap, gap_words: self.is_colour(gap_words))
        if not found or found[0][0].word not in STARTERS:
            return None
        alone = i + 1 == len(words) or words[i].after[-1:] in tuple(",;:.!?") or \
            words[i + 1].word in ("and", "then", "or", "otherwise", "else")
        if alone and found[0][0].word not in STAND_ALONE and any(w.word not in FILLER for w in words[:i]):
            return None                 # "write stop, drop and roll": a word in a sentence, with nothing to act on
        if found[0][0].word in ("if", "while", "until"):          # "write call me, when you can" isn't a check
            try:
                self.split_check(found[0][1:])
            except Problem:
                return None
        return found[0]

    # ---- parts, loops and choices ----

    def tidy(self, actions):
        """Drop polite extra words, and actions that were nothing but those ("computer,")."""
        return [self.drop_filler(words) for words in actions
                if not all(w.word in FILLER for w in words)]

    def compile_parts(self, parts, block=None, otherwise_after=None):
        """block: the lines indented under this one, for a loop or 'if' at its end.
        otherwise_after: an 'otherwise' line after this one, for an 'if' in its last part."""
        parts = [(is_otherwise, self.tidy(actions)) for is_otherwise, actions in parts]
        parts = [(is_otherwise, actions) for is_otherwise, actions in parts if actions or is_otherwise]
        if block and parts and len(parts[-1][1]) > 1 and self.opens(parts[-1][1][-1]):
            parts[-1:] = [(parts[-1][0], parts[-1][1][:-1]), (False, parts[-1][1][-1:])]   # "set x to 1 and repeat:"
        k = 0
        while k + 1 < len(parts):               # "add 1 to x and if x is 3, leave the loop": the 'if' takes what's after the comma
            is_otherwise, actions = parts[k]
            last = actions[-1]
            if len(actions) > 1 and last[0].word == "if" and not parts[k + 1][0] and \
                    not any(self.starts_action(last, n) for n in range(2, len(last))):
                parts[k:k + 2] = [(is_otherwise, actions + parts[k + 1][1])]
            else:
                k += 1
        def last_part():
            """Does an 'if' with its actions ('if x is 1, write a') end the line? Then it takes the lines
            indented under it too, unless the line gave it an 'otherwise' already."""
            return i == len(parts) and block and (otherwise is None or otherwise is otherwise_after)

        i = 0
        while i < len(parts):
            is_otherwise, actions = parts[i]
            if is_otherwise:
                raise Problem("'otherwise' needs an 'if' just before it.")
            header = self.loop_header(actions)
            each = self.read_for_each(actions[0]) if len(actions) == 1 else None
            each = each if each and each[2] is None else None
            if header or each or self.is_bare_if(actions):
                if i + 1 == len(parts) and block:
                    body, block = block, None
                    i += 1
                else:
                    if (i + 1 >= len(parts) or parts[i + 1][0]) and header:
                        raise Problem("Repeat what? Put it on the same line, after the colon: "
                                      "repeat 3 times: write hi and beep" + INDENT_HINT)
                    if i + 1 >= len(parts) or parts[i + 1][0]:
                        if each and each[1][0] == "list":
                            self.known_list(each[1][1])
                        raise Problem("What should happen? Put it after a comma, like: if x is 3, write yes"
                                      + INDENT_HINT)
                    body = parts[i + 1][1]
                    i += 2
            else:
                body = None
                i += 1
            otherwise = None
            if i < len(parts) and parts[i][0]:
                otherwise = parts[i][1]
                i += 1
                if not otherwise and i == len(parts) and block:
                    otherwise, block = block, None          # "otherwise:" with lines under it
                elif not otherwise:
                    raise Problem("Otherwise what? Like: otherwise write no")
            if i == len(parts) and otherwise_after:
                if otherwise is not None:
                    raise LonelyOtherwise("That 'if' already has an 'otherwise'.")
                otherwise = otherwise_after
            if header:
                self.run_loop(header, lambda: self.compile_part(body, otherwise))
            elif each:
                self.for_each(each[0], each[1], lambda: self.compile_part(body, otherwise))
            elif body is not None:
                condition = self.split_check(actions[0][1:])[0]
                more = block if last_part() else None
                self.choose(condition, lambda: (self.compile_part(body), more and more()), otherwise)
            else:
                self.compile_part(actions, otherwise, block if last_part() else None)

    def opens(self, words):
        """Is this action a loop, 'if' or 'for each' waiting for what it does?"""
        each = self.read_for_each(words)
        try:
            return bool(self.loop_header([words]) or each and each[2] is None or self.is_bare_if([words]))
        except Problem:
            return False

    def compile_part(self, actions, otherwise=None, block=None):
        """block: the lines indented under a line ending in an 'if' with its actions ('if x is 1, write a')."""
        if callable(actions):                   # the lines indented under a loop or 'if'
            if otherwise is not None:
                raise LonelyOtherwise("'otherwise' needs an 'if' just before it.")
            return actions()
        if not actions:
            raise Problem("Something is missing here.")
        loop, actions = self.take_loop(actions)
        if loop:
            self.run_loop(loop, lambda: self.compile_actions(actions, otherwise))
        else:
            self.compile_actions(actions, otherwise, block)

    def compile_actions(self, actions, otherwise=None, block=None):
        """Plain actions one by one; an 'if' or 'for each' takes in everything after it."""
        for k, words in enumerate(actions):
            if words[0].word == "if" or self.read_for_each(words):
                return self.compile_choice_or_loop(actions[k:], otherwise, block)
            self.compile_action(words)
        if otherwise is not None:
            raise LonelyOtherwise("'otherwise' needs an 'if' just before it.")

    def compile_choice_or_loop(self, actions, otherwise, block=None):
        first = actions[0]
        loop = self.read_for_each(first)
        if loop:
            name, source, rest = loop
            body = ([rest] if rest else []) + actions[1:]
            if not body:
                raise Problem(f"What should happen for each {name}? Like: for each {name} in list write {name}")
            return self.for_each(name, source, lambda: self.compile_actions(body, otherwise))
        if first[0].word == "if":
            condition, rest = self.split_check(first[1:])
            body = ([rest] if rest else []) + actions[1:]
            if not body:
                raise Problem("What should happen? Put it after a comma, like: if x is 3, write yes")
        return self.choose(condition, lambda: (self.compile_actions(body), block and block()), otherwise)

    def known_list(self, words):
        """'for each name in names' when there's no names yet: say so."""
        try:
            read_value(words, self.kind_of)
        except Problem as problem:
            unknown = re.match(r"I don't know what '([^']+)' is yet", str(problem))
            if unknown:
                raise Problem(f"{unknown.group(0)}. Make the list first, like: make a list called {unknown.group(1)}")
            raise

    def is_bare_if(self, actions):
        if len(actions) == 1 and [w.word for w in actions[0]] == ["if"]:
            raise Problem("If what? Put a check after 'if', like: if score is 5, write hi")
        return (len(actions) == 1 and actions[0][0].word == "if"
                and self.split_check(actions[0][1:])[1] is None)

    def split_check(self, words):
        """Split 'CHECK ACTION...' into the check and the action (None if there's no action)."""
        if len(words) > 1 and words[-1].word == "then":
            words = words[:-1]                  # "if score is 5 then:"
        weak = None                             # a split that leaves no real check: "if lives | drop below 1"
        for k in range(1, len(words)):
            if self.starts_action(words, k):
                try:
                    check = read_condition(words[:k], self.kind_of)
                except Problem:
                    continue
                if not self.weak_check(check):
                    return check, self.drop_filler(words[k:])
                weak = weak or (check, k)
        try:
            check = read_condition(words, self.kind_of)
        except Problem:
            if weak:
                return weak[0], self.drop_filler(words[weak[1]:])
            raise
        if weak and self.weak_check(check):
            return weak[0], self.drop_filler(words[weak[1]:])
        return check, None

    def weak_check(self, check):
        """A 'check' that's only a value, like 'lives': it can't be true or false unless it holds yes or no."""
        return check[0] == "truth" and not (check[1][0] == "name" and self.kind_of(check[1][1]) == "yes/no")

    def read_for_each(self, words):
        """'for each X in L ...' or 'for each X from A to B ...'
        -> (X, ("list", words of L) or ("range", A, B), the rest or None). None if it isn't one."""
        lead = [w.word for w in words[:3]]
        if lead[:2] in (["for", "each"], ["for", "every"]):
            words = words[2:]
        elif lead[:1] in (["each"], ["every"]):
            words = words[1:]
        else:
            return None
        if len(words) >= 4 and words[1].word == "from":
            return self.read_range(words[0].word, words[2:])
        if len(words) < 3 or words[1].word not in ("in", "of", "on"):
            return None
        name, rest = words[0].word, words[2:]
        if file_name(rest[:1]):                 # "for each line in shopping.txt"
            rest = [Word("the"), Word("file")] + rest
        for k in range(1, len(rest)):
            if self.starts_action(rest, k):
                try:
                    read_value(rest[:k], self.kind_of)
                    return name, ("list", rest[:k]), self.drop_filler(rest[k:])
                except Problem:
                    pass
        return name, ("list", rest), None

    def read_range(self, name, words):
        """'A to B ...' -> (name, ("range", A, B), the rest or None)."""
        p = Parser(words, self.kind_of)
        try:
            low = p.sum()
            p.need("to")
            high = p.sum()
        except Problem:
            return None
        rest = p.words[p.at:]
        if rest and not self.starts_action(rest, 0):
            return None
        return name, ("range", low, high), self.drop_filler(rest) if rest else None

    # ---- loops ----

    def loop_header(self, actions):
        """A loop on its own, like 'repeat 3 times' or 'repeat until x is 5'; else None."""
        if len(actions) != 1:
            return None
        loop, rest = self.loop_at_start(actions[0])
        return loop if loop and not rest else None

    def take_loop(self, actions):
        """Find a loop at the start of the first action or the end of any action.
        Returns (loop, actions without the loop words)."""
        loop, rest = self.loop_at_start(actions[0])
        if loop:
            return loop, [rest] + actions[1:]
        for n, words in enumerate(actions):
            loop, rest = self.loop_at_end(words)
            if loop and rest:
                return loop, actions[:n] + [rest] + actions[n + 1:]
        return None, actions

    def loop_at_start(self, words):
        doing = self.keep_doing(words)
        if doing:                           # "keep asking for a number until it is 5"
            loop, rest = self.loop_at_end(doing)
            return (loop, rest) if loop else (("forever",), doing)
        keep = [w.word for w in words[:2]] in (["keep", "going"], ["keep", "doing"], ["keep", "repeating"],
                                               ["do", "this"], ["do", "it"], ["do", "that"])
        repeat = keep or bool(words) and words[0].word in ("repeat", "loop")
        w = words[2:] if keep else words[1:] if repeat else words
        if w[:1] and w[0].word in ("this", "that", "it") and repeat:
            w = w[1:]                       # "repeat this", "keep doing this"
        if not w:
            return (("forever",), []) if repeat else (None, words)
        if w[0].word == "forever" or [x.word for x in w[:2]] == ["for", "ever"]:
            return ("forever",), w[1 if w[0].word == "forever" else 2:]
        if w[0].word == "until":
            try:
                check, rest = self.split_check(w[1:])
            except Problem:                 # "repeat until guess is 7:" when the loop asks for guess
                k = next((k for k in range(1, len(w) - 1) if self.starts_action(w[1:], k)), len(w) - 1)
                check, rest = ("later", w[1:k + 1]), self.drop_filler(w[k + 1:]) if k + 1 < len(w) else None
            return ("until", check), rest or []
        if w[0].word == "while":
            check, rest = self.split_check(w[1:])
            return (w[0].word, check), rest or []
        for k in range(1, min(len(w), 6)):
            if w[k].word in ("times", "time") or self.means(w[k:k + 1], "times"):   # "repeat 3 rounds"
                try:
                    return ("times", read_value(w[:k], self.kind_of)), w[k + 1:]
                except Problem:
                    break
        if repeat:
            raise Problem("Repeat how? Try: repeat 3 times, write hi — or: repeat until x is 5, add 1 to x")
        return None, words

    def keep_doing(self, words):
        """'keep asking for a number' -> the words 'ask for a number'; None if it isn't 'keep' + '...ing'."""
        if len(words) < 2 or words[0].word != "keep" or words[1].word in ("going", "doing", "repeating") \
                or not (words[1].word or "").endswith("ing"):
            return None
        stem = words[1].word[:-3]
        for verb in (stem, stem + "e", stem[:-1]):          # asking, writing, stopping
            if (verb in STARTERS or verb in self.dictionary_starts) and verb != "keep":
                return [Word(verb)] + words[2:]
        return None

    def made_by(self, words):
        """The name 'ask ... as guess' will make, so a check after it can use guess already."""
        before, after = split_at(words, "as", "into", "called", "named", last=True)
        if not (words and words[0].word == "ask" and after):
            return None
        try:
            return plain_name(after)
        except Problem:
            return None

    def means(self, words, meaning):
        """Are these words a dictionary loop word with this meaning ('rounds' -> times)?"""
        found = dictionary.respell(words, "loop", whole=True)
        return bool(found) and " ".join(w.word for w in found[0]) == meaning

    def loop_at_end(self, words):
        end = dictionary.ending(words, "loop") if len(words) > 1 else None     # "beep over and over", "thrice"
        if end and [w.word for w in end[1]] == ["forever"]:
            return ("forever",), words[:-end[0]]
        if end and len(end[1]) > 1 and end[1][-1].word == "times":
            count, rest = self.count_at_end(words[:-end[0]] + end[1])
            if count is not None and rest:
                return ("times", count), rest
        if words and words[-1].word == "forever":
            return ("forever",), words[:-1]
        spots = []
        for i in range(1, len(words)):                  # "until", or "till", "up until", "as long as"
            found = dictionary.respell(words, "loop", start=i)
            if found and found[1].meaning in ("until", "while"):
                spots.append((i, len(words) - len(found[0]) + 1, found[1].meaning))
            elif words[i].word in ("until", "while") and not (spots and spots[-1][0] + spots[-1][1] > i):
                spots.append((i, 1, words[i].word))
        if spots:
            i, size, how = max(spots)
            made = self.made_by(words[:i])
            try:
                return (how, read_condition(
                    words[i + size:], lambda name: self.kind_of(name) or ("text" if name == made else None))), words[:i]
            except Problem:
                pass
        count, rest = self.count_at_end(words)
        return (("times", count), rest) if count is not None else (None, words)

    def count_at_end(self, words):
        if words and words[-1].word in ("once", "twice"):
            return ("number", 1 if words[-1].word == "once" else 2), words[:-1]
        if len(words) >= 2 and words[-1].word in ("times", "time"):
            for start in range(len(words) - 1):
                try:
                    count = read_value(words[start:-1], self.kind_of)
                except Problem:
                    continue
                if start == 0 or not self.ends_value(words[:start]):
                    return count, words[:start]
        case = case_at_end(words)                               # "write hello 3 times in capitals"
        if case:
            count, rest = self.count_at_end(words[:-case[1]])
            if count is not None and rest:
                return count, rest + words[-case[1]:]
        for i in range(max(1, len(words) - 6), len(words)):    # "write hello 3 times in red"
            found = self.style_at(words, i)
            if found and i + found[1] == len(words):
                count, rest = self.count_at_end(words[:i])
                if count is not None and rest:
                    return count, rest + words[i:]
        return None, words

    def ends_value(self, words):
        """True if these words end with something that wants a value after it, like 'plus'."""
        return words[-1].word in ("plus", "minus", "times", "by", "+", "-", "*", "/", "to", "from")

    def run_loop(self, loop, body):
        a = self.a
        top, end = a.new_label("loop"), a.new_label("loop_end")
        if loop[0] == "times":
            return self.repeat(loop[1], body)
        self.loop_ends.append(end)
        a.label(top)
        if loop[0] == "while":
            self.check_or_jump(loop[1], end)
        body()
        if loop[0] == "until":
            check = loop[1]
            if check[0] == "later":         # read now that the loop has made its names
                check = read_condition(check[1], self.kind_of)
            self.check_or_jump(check, top)
            self.loop_ends.pop()
            return a.label(end)
        a.jump(top)
        self.loop_ends.pop()
        a.label(end)

    def check_or_jump(self, check, where):
        """Go to where if the check is not true."""
        if self.emitter.emit(check) != "yes/no":
            raise Problem("I need a check here, like: until x is 5")
        self.a.op("test", RAX, RAX)
        self.a.jump_if(EQUAL, where)

    def repeat(self, count, body):
        a = self.a
        counter = a.reserve(8, "counter")
        top, end = a.new_label("repeat"), a.new_label("repeat_end")
        self.emitter.whole(self.emitter.emit(count))
        a.address(RCX, counter)
        a.store(RCX, 0, RAX)
        a.label(top)
        a.address(RCX, counter)
        a.load(RAX, RCX)
        a.op_n("sub", RAX, 1)
        a.jump_if(LESS, end)
        a.store(RCX, 0, RAX)
        self.loop_ends.append(end)
        body()
        self.loop_ends.pop()
        a.jump(top)
        a.label(end)

    def leave_loop(self):
        if not self.loop_ends:
            raise Problem("There's no loop to leave here. To end the whole program, say: stop")
        self.a.jump(self.loop_ends[-1])

    def for_each(self, name, source, body):
        if source[0] == "range":
            return self.count_through(name, source[1], source[2], body)
        a = self.a
        kind = self.value(source[1])
        if kind == "text":
            how = "letters" if name in ("letter", "character", "char") else \
                "lines" if name == "line" else "words"
            place = a.reserve(LIST_SIZE, "split_list")
            a.address(RCX, place)
            if how in ("words", "lines"):
                a.set(RDX, ord(" ") if how == "words" else ord("\n"))
                a.set(R8, 0)
                a.call("split")
            else:
                a.call("letters")
            a.address(RAX, place)
            kind = "list of text"
        if not kind.startswith("list"):
            raise Problem("'for each' needs a list or some text to go through.")
        the_list, position = a.reserve(8, "loop_list"), a.reserve(8, "loop_position")
        scope = {}
        var = self.new_var(name, item_kind(kind), scope)
        top, end = a.new_label("each"), a.new_label("each_end")
        a.address(RCX, the_list)
        a.store(RCX, 0, RAX)
        a.set(RAX, 0)
        a.address(RCX, position)
        a.store(RCX, 0, RAX)
        a.label(top)
        a.address(RCX, position)
        a.load(RCX, RCX)
        a.address(RAX, the_list)
        a.load(RAX, RAX)
        a.load(R8, RAX)
        a.op("cmp", RCX, R8)
        a.jump_if(NOT_LESS, end)
        a.shift_left(RCX, 3)
        a.op("add", RAX, RCX)
        a.load(RAX, RAX, 8)
        a.address(RCX, var.place)
        a.store(RCX, 0, RAX)
        self.scopes.append(scope)
        self.loop_ends.append(end)
        try:
            body()
        finally:
            self.scopes.pop()
            self.loop_ends.pop()
        a.address(RCX, position)
        a.load(RAX, RCX)
        a.op_n("add", RAX, 1)
        a.store(RCX, 0, RAX)
        a.jump(top)
        a.label(end)

    def count_through(self, name, low, high, body):
        """name goes from low to high (or down, if high is smaller), one step at a time."""
        a = self.a
        scope = {}
        var = self.new_var(name, "number", scope)
        last, step = a.reserve(8, "count_to"), a.reserve(8, "count_step")
        top, end = a.new_label("count"), a.new_label("count_end")
        self.emitter.whole(self.emitter.emit(high))
        a.address(RCX, last)
        a.store(RCX, 0, RAX)
        self.emitter.whole(self.emitter.emit(low))
        a.address(RCX, var.place)
        a.store(RCX, 0, RAX)
        a.address(RCX, last)
        a.load(RCX, RCX)
        a.set(RDX, 1)
        a.op("cmp", RAX, RCX)
        up = a.new_label("up")
        a.jump_if(NOT_GREATER, up)
        a.set(RDX, -1)
        a.label(up)
        a.address(RCX, step)
        a.store(RCX, 0, RDX)
        a.label(top)                            # stop once we've gone past the end
        a.address(RAX, var.place)
        a.load(RAX, RAX)
        a.address(RCX, last)
        a.load(RCX, RCX)
        a.op("sub", RAX, RCX)
        a.address(RDX, step)
        a.load(RDX, RDX)
        a.multiply(RAX, RDX)
        a.op_n("cmp", RAX, 0)
        a.jump_if(GREATER, end)
        self.scopes.append(scope)
        self.loop_ends.append(end)
        try:
            body()
        finally:
            self.scopes.pop()
            self.loop_ends.pop()
        a.address(RCX, var.place)
        a.load(RAX, RCX)
        a.address(RDX, step)
        a.load(RDX, RDX)
        a.op("add", RAX, RDX)
        a.store(RCX, 0, RAX)
        a.jump(top)
        a.label(end)

    def choose(self, condition, body, otherwise=None):
        a = self.a
        if self.emitter.emit(condition) != "yes/no":
            raise Problem("I need a check here, like: if score is more than 10")
        no, end = a.new_label("no"), a.new_label("end_if")
        a.op("test", RAX, RAX)
        a.jump_if(EQUAL, no)
        body()
        a.jump(end)
        a.label(no)
        if otherwise:
            self.compile_part(otherwise)
        a.label(end)

    # ---- one action ----

    def compile_action(self, words):
        self.made_before, self.made_just_now = self.made_just_now, None
        self.actions_on[self.line_number] = self.actions_on.get(self.line_number, 0) + 1
        delay = self.delay_at_start(words)
        if delay:
            self.brick("wait", lambda: self.do_wait(delay[0]))
            return self.compile_action(delay[1]) if delay[1] else None
        delay = self.delay_at_end(words)
        if delay:                               # "make it disappear after 5 seconds"
            self.brick("wait", lambda: self.do_wait(delay[0]))
            return self.compile_action(delay[1])
        words = self.drop_filler(words)
        if self.vanishes(words):
            return self.brick("disappear", self.make_disappear)
        words = self.same_as(words)
        for definition in reversed(self.definitions):
            filled = teach.match(definition, words)
            if filled is not None:
                return self.brick("your word", lambda: self.use_word(definition, filled))
        plan = self.gui_read(words)              # brick's own window: buttons, canvases, menus ...
        if plan:
            maker = plan.__qualname__.split(".<locals>")[0].split(".")[-1]
            return self.brick("window " + maker.replace("read_", "").replace("gui_", "").replace("_", " "), plan)
        verb = words[0].word
        handler = self.handler_for(words)
        if handler is None:
            for i in range(1, min(len(words) - 1, 4)):
                joiner = [w.word for w in words[i:i + 2]]
                if joiner[0] in ("=", "becomes") or joiner == ["is", "now"]:
                    return self.brick("set", lambda: self.assign(plain_name(words[:i]),
                                                                 words[i + (2 if joiner[0] == "is" else 1):]))
            try:
                node = read_value(words, self.kind_of)
            except Problem:
                node = None
            if node and node[0] not in ("number", "decimal", "text"):
                return self.brick("write", lambda: self.do_write([Word("write")] + words))  # "how many in names"
        if handler is None:
            yours = [d for d in self.definitions if d.first_word() == verb]
            if yours:
                raise Problem(f"Your word '{verb}' needs more after it: {teach.how_to_use(yours[-1])}")
            if verb in self.dictionary_starts and verb not in STARTERS:
                colours = colour_words()
                close = [difflib.get_close_matches(w.word, colours, n=1, cutoff=0.8) for w in words[1:] if w.word]
                close = next((c[0] for c in close if c), None)          # "turn the text crimsom"
                hint = f" Did you mean '{close}'?" if close else ""
                raise Problem(f"What should '{words[0].text}' do here?{hint} You can say: "
                              + "  |  ".join(dictionary.examples_for(verb)[:6]))
            first = words[0].text if verb else words[0].raw
            own = [d.first_word() for d in self.definitions if d.first_word()]
            close = difflib.get_close_matches(first.lower(), own + STARTERS +
                                              sorted(self.dictionary_starts - dictionary.lexicon_keys()), n=1)
            hint = f" Did you mean '{close[0]}'?" if close and close[0] != first.lower() else ""
            if first.lower() in ("each", "every", "for"):          # "each turn hi"
                hint = " To go through a list, say: for each name in names write name."
            raise Problem(f"I don't know how to '{first}'.{hint} "
                          "Actions start with a word like write, ask, set, add, wait, if or for each."
                          + self.more_help(words, None))
        try:
            self.brick(handler.__name__[3:], lambda: handler(words))
        except Problem as problem:
            extra = "" if str(problem).startswith("What should the") else self.more_help(words, verb)
            raise Problem(str(problem) + (extra if extra.strip() not in str(problem) else ""))

    def brick(self, name, build):
        """Build one brick (the action a direction became: write, wait, set, a window phrase, your word ...) and
        note it for the line, for the brain's check of which bricks a line should be (trainer/picker.py). A brick
        built inside another (a write inside a make) is part of that one."""
        if getattr(self, "in_brick", False):
            return build()
        self.bricks_on.setdefault(self.line_number, []).append(name)
        self.in_brick = True
        try:
            return build()
        finally:
            self.in_brick = False

    def more_help(self, words, verb):
        """Extra lines for a problem: how to talk about the window, or examples for this verb."""
        said = {w.word for w in words}
        if said & {"console", "terminal"}:
            return "\n    " + WINDOW_HELP
        if self.gui_on and said & {"window", "button", "canvas", "menu", "tabs", "tab"}:
            return self.gui_help(words)
        if verb in EXAMPLES:
            return "\n    You can say: " + "  |  ".join(EXAMPLES[verb][:3])
        return ""

    # ---- your own words ----

    def same_as(self, words):
        """'zap means write' makes zap another word for write: 'zap hello in red' -> 'write hello in red'.
        Only for a word (or words) with no holes that mean one word brick already knows as a direction."""
        for definition in reversed(self.definitions):
            head = [part for kind, part in definition.pattern]
            body = definition.body.strip().lower()
            if any(kind != "word" for kind, _ in definition.pattern) or " " in body or \
                    body not in STARTERS and body not in self.dictionary_starts:
                continue
            if len(words) > len(head) and [w.word for w in words[:len(head)]] == head:
                return [Word(body)] + words[len(head):]
        return words

    def learn(self, definition):
        first = definition.first_word()
        if first is None:
            raise Problem("A new word needs at least one real word, like: to greet someone means ...")
        known = [d for d in self.definitions if d.key == definition.key]
        if first in STARTERS and not known:
            answer = self.ask_user(f"'{first}' is already a built-in word. Replace it with yours? (yes/no) ")
            if not answer.strip().lower().startswith("y"):
                print(f"OK, I kept the built-in '{first}'.")
                return
        self.definitions = [d for d in self.definitions if d.key != definition.key] + [definition]
        teach.save(self.words_file, self.definitions)

    def use_word(self, definition, filled):
        a = self.a
        holes = list(filled)
        kinds = []
        for hole in holes:
            kinds.append(self.hole_value(filled[hole]))
            a.push(RAX)
        key = (definition.key, tuple(kinds))
        if key not in self.compiled_words:
            self.compile_word(definition, key, holes, kinds)
        label, places = self.compiled_words[key]
        for place in reversed(places):
            a.pop(RAX)
            a.address(RCX, place)
            a.store(RCX, 0, RAX)
        a.call(label)

    def hole_value(self, words):
        """What fills a hole: a value if the words make one, else the words themselves as text."""
        try:
            node = read_value(words, self.kind_of)
        except Problem:
            node = ("text", " ".join(w.text + w.after if w.quoted else w.raw for w in words))
        return self.emitter.emit(node)

    def compile_word(self, definition, key, holes, kinds):
        a = self.a
        label, skip = a.new_label("word"), a.new_label("after_word")
        scope = {}
        places = [self.new_var(hole, kind, scope).place for hole, kind in zip(holes, kinds)]
        self.compiled_words[key] = (label, places)
        saved = self.scopes, self.loop_ends
        self.scopes, self.loop_ends = [self.scopes[0], scope], []
        a.jump(skip)
        a.label(label)
        try:
            self.compile_line(definition.body)
        except Problem as problem:
            raise Problem(f"In your word '{definition.key}': {problem}")
        finally:
            self.scopes, self.loop_ends = saved
        a.ret()
        a.label(skip)
