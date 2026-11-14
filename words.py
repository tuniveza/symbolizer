"""brick teach: give brick a new word for every program, in one line.

  brick teach zap means write     zap now works like write, in every program (zap hello in red)
  brick teach zap = write         the same
  brick teach                     the words you taught this way
  brick unteach zap               forget it again

The word goes into texts/dictionary.txt, under '# Your words'. Before it's kept, brick checks it really
works: a small program with your word must build exactly like the same program with its meaning.
"""
import os
import re
import tempfile

import dictionary
import factory

HEADING = "# Your words\n# Taught with brick teach. Each one was checked to build exactly like its meaning.\n"


def command(args):
    text = " ".join(args).strip()
    if not text:
        return show()
    found = split(text)
    if not found:
        print("Say it like this:  brick teach zap means write")
        return 1
    ok, message, _ = teach(*found)
    print(message)
    return 0 if ok else 1


def split(text):
    """'zap means write' or 'zap = write' -> ('zap', 'write'), or None."""
    found = re.fullmatch(r"(.+?)\s*(?:\bmeans\b|=)\s*(.+)", text.strip(), re.IGNORECASE)
    if not found:
        return None
    return found.group(1).strip().strip('"').lower(), found.group(2).strip().strip('"')


def teach(word, meaning):
    """Teach brick the word for every program. -> (ok, message, an example line that uses it)."""
    word, meaning = " ".join(word.lower().split()), " ".join(meaning.split())
    if not word or not meaning:
        return False, "Say the new word and what it means.", None
    if "=" in word or "=" in meaning or "#" in word:
        return False, "A word can't have = or # in it.", None
    if word == meaning.lower():
        return False, f"'{word}' already means itself.", None
    probe = works_like(word, meaning, before=True)
    if probe is None:
        return False, (f"brick doesn't know '{meaning}' yet, so it can't make '{word}' mean it. "
                       f"Pick something brick knows, like write, wait or red."), None
    line = f"{word} = {meaning}"
    if any(e.key == word and e.meaning.lower() == meaning.lower() for e in dictionary.entries()):
        return True, f"brick already knows that: {word} means {meaning}.", probe
    old = factory.same_key(dictionary.PATH, line)
    with factory.Writing():
        for o in old:
            factory.remove_line_now(dictionary.PATH, o)
        add(line)
    dictionary.clear()
    if not works_like(word, meaning):
        with factory.Writing():
            factory.remove_line_now(dictionary.PATH, line)
            for o in old:
                add(o)
        dictionary.clear()
        return False, f"brick couldn't make '{word}' work like '{meaning}', so it isn't kept.", None
    was = f" (It used to mean {old[0].split('=', 1)[1].strip()}.)" if old else ""
    return True, f"Taught: {word} means {meaning}. It works in every program now, for example: {probe}{was}", probe


def add(line):
    """Put the line at the end of the '# Your words' part (made at the end of the file if it isn't there)."""
    with open(dictionary.PATH, encoding="utf-8") as f:
        lines = f.read().rstrip("\n").split("\n")
    if "# Your words" not in lines:
        lines += [""] + HEADING.rstrip("\n").split("\n")
    k = lines.index("# Your words") + 1
    while k < len(lines) and lines[k].strip():
        k += 1
    lines.insert(k, line)
    factory.write_whole(dictionary.PATH, "\n".join(lines) + "\n")


def works_like(word, meaning, before=False):
    """A small program using the meaning that builds; with before=False, the word must build the same.
    Returns that program with the word in it, or None."""
    probes = ["{} hello", "write hello in {}", "set the background colour to {}", "{}", "write 2 {} 3"]
    saved = {k: os.environ.get(k) for k in ("BRICK_MEMORY", "BRICK_NEURAL", "BRICK_FACTORY")}
    os.environ.update(BRICK_MEMORY="off", BRICK_NEURAL="off", BRICK_FACTORY="off")
    try:
        with tempfile.TemporaryDirectory() as where:
            words = os.path.join(where, "mywords.txt")
            for probe in probes:
                meant = build(probe.format(meaning), words)
                if meant is None or probe.format(meaning) == probe.format(""):
                    continue
                if before:
                    return probe.format(word)
                return probe.format(word) if build(probe.format(word), words) == meant else None
    finally:
        for k, v in saved.items():
            if v is None:
                os.environ.pop(k, None)
            else:
                os.environ[k] = v
    return None


def build(text, words):
    from compiler import Compiler
    try:
        return Compiler(words, ask_user=lambda question: "no").compile_text(text)
    except Exception:
        return None


def taught():
    """The lines under '# Your words' in dictionary.txt (up to the next blank line)."""
    with open(dictionary.PATH, encoding="utf-8") as f:
        lines = f.read().split("\n")
    if "# Your words" not in lines:
        return []
    found = []
    for line in lines[lines.index("# Your words") + 1:]:
        if not line.strip():
            break
        if "=" in line and not line.startswith("#"):
            found.append(line.strip())
    return found


def show():
    found = taught()
    if not found:
        print("You haven't taught brick any words yet. Try:  brick teach zap means write")
    else:
        print("Words you taught brick (brick unteach zap forgets one):")
        print("\n".join("  " + line.replace(" = ", " means ") for line in found))
    return 0


def unteach(args):
    ok, message = forget(" ".join(args))
    print(message)
    return 0 if ok else 1


def forget(word):
    """Forget a word taught with brick teach. -> (ok, message)."""
    word = " ".join(word.strip().strip('"').lower().split())
    lines = [line for line in taught() if line.split("=", 1)[0].strip().lower() == word]
    if not word or not lines:
        return False, f"'{word}' isn't one of the words you taught. brick teach lists them."
    with factory.Writing():
        for line in lines:
            factory.remove_line_now(dictionary.PATH, line)
        if not taught():                  # the last one: take the empty heading away too
            with open(dictionary.PATH, encoding="utf-8") as f:
                text = f.read()
            factory.write_whole(dictionary.PATH, text.replace("\n\n" + HEADING, "\n").rstrip("\n") + "\n")
    dictionary.clear()
    return True, f"Forgot: {word}."


# ---------------- for brick kiln's Words tab ----------------

UNKNOWN = re.compile(r"I don't know how to '([^']+)'\.(?: Did you mean '([^']+)'\?)?")
SGR = re.compile(r"\x1b\[([0-9;]*)m")
OTHER_ESCAPE = re.compile(r"\x1b\[[0-9;?]*[A-Za-ln-z]|\x1b\][^\x07]*\x07")


def unknown(text, words_file):
    """The words in this program brick doesn't know as a direction -> [{word, line, number, guess}]."""
    from compiler import Compiler
    from expressions import Problem
    saved = os.environ.get("BRICK_NEURAL")
    os.environ["BRICK_NEURAL"] = "off"
    try:
        Compiler(words_file, ask_user=lambda question: "no").compile_text(text)
        return []
    except Problem as problem:
        report = str(problem)
    except Exception:
        return []
    finally:
        if saved is None:
            os.environ.pop("BRICK_NEURAL", None)
        else:
            os.environ["BRICK_NEURAL"] = saved
    found, lines = {}, text.split("\n")
    for number, message in re.findall(r"^Line (\d+): .*\n((?:    .*\n?)*)", report, re.MULTILINE):
        for word, guess in UNKNOWN.findall(message):
            word = word.lower()
            if word not in found and 0 < int(number) <= len(lines):
                found[word] = {"word": word, "number": int(number), "line": lines[int(number) - 1].strip(),
                               "guess": guess or None}
    return list(found.values())


def try_text(text, words_file):
    """Build these lines the way a real build does and run them for a moment.
    -> {ok, reads: [how brick read lines], problem, output: [{text, colour, bold}], ran, why}."""
    import subprocess
    from compiler import Compiler
    from expressions import Problem
    compiler = Compiler(words_file, ask_user=lambda question: "no")
    try:
        program = compiler.compile(text)
    except Problem as problem:
        return {"ok": False, "problem": str(problem), "reads": reads(compiler), "output": []}
    except Exception as problem:
        return {"ok": False, "problem": f"brick got stuck on it: {problem}", "reads": [], "output": []}
    result = {"ok": True, "reads": reads(compiler), "output": [], "ran": False, "why": ""}
    if compiler.gui_lines or getattr(compiler, "uses_window", False):
        result["why"] = "It opens a window, so it isn't run here. Put it in your program and press Run."
        return result
    with tempfile.TemporaryDirectory() as where:
        path = os.path.join(where, "try")
        with open(path, "wb") as f:
            f.write(program)
        os.chmod(path, 0o755)
        env = {k: v for k, v in os.environ.items() if not k.startswith("BRICK_WINDOW")}
        try:
            done = subprocess.run([path], cwd=where, stdin=subprocess.DEVNULL, capture_output=True,
                                  timeout=4, env=env)
            out = done.stdout
        except subprocess.TimeoutExpired as late:
            out = late.stdout or b""
            result["why"] = "(It was still going after 4 seconds, so it was stopped.)"
    result["ran"] = True
    result["output"] = painted(out.decode("utf-8", errors="replace"))
    return result


def reads(compiler):
    return [n.replace("Note: ", "", 1) for n in compiler.notes if " read as: " in n]


def painted(text):
    """Terminal text -> [{text, colour, bold}]: colours and bold kept, other escapes taken out."""
    text = OTHER_ESCAPE.sub("", text).replace("\a", "(beep)").replace("\r", "")
    parts, colour, bold, at = [], None, False, 0
    for found in SGR.finditer(text):
        if found.start() > at:
            parts.append({"text": text[at:found.start()], "colour": colour, "bold": bold})
        codes = [c for c in found.group(1).split(";")] or ["0"]
        k = 0
        while k < len(codes):
            c = codes[k]
            if c in ("", "0"):
                colour, bold = None, False
            elif c == "1":
                bold = True
            elif c == "38" and k + 4 < len(codes) + 1 and codes[k + 1:k + 2] == ["2"]:
                colour = "rgb({},{},{})".format(*codes[k + 2:k + 5])
                k += 4
            elif c.isdigit() and 30 <= int(c) <= 37:
                colour = ["black", "#e5484d", "#46a758", "#f5d90a", "#3e63dd", "#ab4aba", "#12a594", "white"][int(c) - 30]
            k += 1
        at = found.end()
    if at < len(text):
        parts.append({"text": text[at:], "colour": colour, "bold": bold})
    return [p for p in parts if p["text"]]


def listing(words_file):
    """Everything the Words tab lists: words taught for every program, and this folder's own words."""
    import teach as own
    every = [{"word": line.split("=", 1)[0].strip(), "meaning": line.split("=", 1)[1].strip()} for line in taught()]
    here = [{"line": d.line, "key": d.key} for d in own.load(words_file)]
    return {"every": every, "here": here}


def forget_here(key, words_file):
    """Forget one of this folder's own words (from mywords.txt). -> (ok, message)."""
    import teach as own
    found = own.load(words_file)
    kept = [d for d in found if d.key != key]
    if len(kept) == len(found):
        return False, "That word isn't in this folder's words."
    own.save(words_file, kept)
    return True, f"Forgot: {key.replace('_', '…')}."


def teach_here(word, meaning, words_file):
    """Teach a word for the programs in this folder only (mywords.txt). -> (ok, message, example)."""
    import teach as own
    word, meaning = " ".join(word.lower().split()), " ".join(meaning.split())
    if not word or not meaning:
        return False, "Say the new word and what it means.", None
    definition = own.read_definition(f"{word} means {meaning}")
    if definition is None:
        return False, "Say it like: zap means write", None
    probe = works_like(word, meaning, before=True)
    test = os.path.join(tempfile.mkdtemp(), "mywords.txt")
    try:
        own.save(test, [d for d in own.load(words_file) if d.key != definition.key] + [definition])
        if probe and build(probe, test) is None:
            return False, f"brick couldn't make '{word}' work like '{meaning}', so it isn't kept.", None
    finally:
        import shutil
        shutil.rmtree(os.path.dirname(test), ignore_errors=True)
    own.save(words_file, [d for d in own.load(words_file) if d.key != definition.key] + [definition])
    return True, f"Taught for this folder: {word} means {meaning}." + \
        (f" For example: {probe}" if probe else ""), probe
