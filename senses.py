"""brick's senses: it learns from any symbol, wherever it comes from.

    brick learn notes.txt             learn from a file
    brick learn ~/Pictures            learn from every file in a folder (and the folders inside it)
    brick learn "red apples are sweet" learn from these words
    brick learn watch ~/Pictures      while brick runs, learn by itself from new or changed files there
    brick learn unwatch ~/Pictures    stop that
    brick learn watching              which folders are watched

What happens to each thing:
  text (any file that reads as text: .txt, .md, .brick, code ...)
      each line teaches the brain how its words are used (brain.py), and the words of each line fire
      together in their neurons (symbols.py, texts/universes): letters, words and contexts
  pictures (.png .jpg .jpeg .webp .gif .bmp .tif .heic ...)
      the eye looks (sight.py): it filters the picture, names what it sees, and the neurons of what it saw
      fire together
  anything else is skipped, and counted

Watched folders are in texts/factory/watching.txt, one per line (change it freely). Which files were already
learned from, and when they last changed, is in texts/factory/senses.json, so nothing is learned twice.
Watching only happens while brick runs (the factory's round), a few files at a time. Nothing leaves this
computer.
"""
import json
import os
import re
import sys
import time

HERE = os.path.dirname(os.path.abspath(__file__))
FOLDER = os.path.join(HERE, "texts", "factory")
WATCHING = os.path.join(FOLDER, "watching.txt")
SEEN = os.path.join(FOLDER, "senses.json")
PICTURES = {".png", ".jpg", ".jpeg", ".webp", ".gif", ".bmp", ".tif", ".tiff", ".heic", ".heif", ".ppm", ".avif"}
SKIP_FOLDERS = {".git", "__pycache__", "node_modules", ".cache", ".venv", "venv", ".local", ".Trash"}
BIGGEST_TEXT = 2 * 1024 * 1024        # bytes: a bigger text file is read only this far
BIGGEST_PICTURE = 60 * 1024 * 1024    # bytes: a bigger picture is skipped
PER_ROUND = 8                         # files learned from per factory round while watching
ROUND_BUDGET = 2.0                    # seconds per factory round, at most


# ---------------- what a thing is ----------------

def kind(path):
    """'picture', 'text' or None (skip)."""
    ext = os.path.splitext(path)[1].lower()
    if ext in PICTURES:
        return "picture" if os.path.getsize(path) <= BIGGEST_PICTURE else None
    try:
        with open(path, "rb") as f:
            start = f.read(4096)
    except OSError:
        return None
    if b"\0" in start:
        return None
    try:
        start.decode("utf-8")
    except UnicodeDecodeError as e:
        if e.start < len(start) - 4:           # not just a character cut off at the end of the sample
            return None
    return "text"


def files_in(target):
    """Every file at target: the file itself, or every file under a folder (hidden ones and caches left out)."""
    target = os.path.expanduser(target)
    if os.path.isfile(target):
        yield target
        return
    for root, folders, files in os.walk(target):
        folders[:] = sorted(f for f in folders if f not in SKIP_FOLDERS and not f.startswith("."))
        for name in sorted(files):
            if not name.startswith("."):
                yield os.path.join(root, name)


# ---------------- learning ----------------

class Senses:
    def __init__(self, mind=None):
        import brain
        self.brain = mind if mind is not None else brain.Brain(brain.path())
        self.world = None

    def text(self, text, source):
        """Each line: how its words are used (the brain), and its words firing together (the neurons)."""
        import brain
        import symbols
        if self.world is None:
            import universe
            self.world = universe.universe()
        lines = 0
        for line in re.split(r"[\n.!?;]+", text):
            words = brain.words_of(line)
            if not words:
                continue
            self.brain.learn_line(line, self.world)
            for k in range(0, len(words), 12):             # a neuron round takes up to 12 symbols at once
                symbols.experience(words[k:k + 12], source)
            lines += 1
        return lines

    def picture(self, path, name=None):
        """The eye looks; it feeds the neurons itself. -> what it said, in a few words."""
        import inspect
        import sight
        parameters = inspect.signature(sight.look).parameters
        if "learn" in parameters:
            seen = sight.look(path, name=name, learn=True)
            if seen.get("errors") and not seen.get("symbols"):
                return None, "; ".join(seen["errors"])
            what = seen.get("name") or "something"
            return f"{what} ({seen.get('how', '?')}, {seen.get('sure', 0):.2f} sure)", None
        if name is None:
            return None, "this version of the eye needs to be told what the picture shows"
        import symbols
        return ("taught" if symbols.show(path, name) == 0 else None), None

    def learn(self, target, source=None):
        """Learn from a file, a folder or plain words. -> {'text': n, 'picture': n, 'skipped': n, 'said': [...]}."""
        done = {"text": 0, "picture": 0, "skipped": 0, "said": []}
        expanded = os.path.expanduser(target)
        if not os.path.exists(expanded):
            self.text(target, source or "words you gave")
            done["text"] += 1
            return done
        for path in files_in(expanded):
            self.one(path, done, source)
        return done

    def one(self, path, done, source=None):
        try:
            what = kind(path)
            if what == "text":
                with open(path, encoding="utf-8", errors="replace") as f:
                    self.text(f.read(BIGGEST_TEXT), source or os.path.basename(path))
                done["text"] += 1
            elif what == "picture":
                said, problem = self.picture(path)
                if said:
                    done["picture"] += 1
                    done["said"].append(f"{os.path.basename(path)}: {said}")
                else:
                    done["skipped"] += 1
                    done["said"].append(f"{os.path.basename(path)}: skipped ({problem})")
            else:
                done["skipped"] += 1
        except Exception as problem:                       # one odd file never stops the rest
            done["skipped"] += 1
            done["said"].append(f"{os.path.basename(path)}: skipped ({type(problem).__name__}: {problem})")

    def save(self):
        self.brain.save()


# ---------------- watched folders ----------------

def watched():
    try:
        with open(WATCHING, encoding="utf-8") as f:
            return [line.strip() for line in f if line.strip() and not line.startswith("#")]
    except OSError:
        return []


def set_watched(folders):
    os.makedirs(FOLDER, exist_ok=True)
    with open(WATCHING, "w", encoding="utf-8") as f:
        f.write("# Folders brick learns from by itself while it runs, one per line. brick learn watch / unwatch.\n")
        f.writelines(folder + "\n" for folder in folders)


def seen():
    try:
        with open(SEEN, encoding="utf-8") as f:
            return json.load(f)
    except (OSError, ValueError):
        return {}


def watch_round(mind, budget=ROUND_BUDGET, most=PER_ROUND):
    """One factory round: a few new or changed files from the watched folders. -> a line saying what, or ''."""
    folders = watched()
    if not folders:
        return ""
    started, known, senses = time.monotonic(), seen(), Senses(mind)
    done = {"text": 0, "picture": 0, "skipped": 0, "said": []}
    count = 0
    for folder in folders:
        for path in files_in(folder):
            if count >= most or time.monotonic() - started > budget:
                break
            try:
                when = f"{os.path.getmtime(path):.0f}"
            except OSError:
                continue
            if known.get(path) == when:
                continue
            known[path] = when
            senses.one(path, done)
            count += 1
    if not count:
        return ""
    with open(SEEN + ".new", "w", encoding="utf-8") as f:
        json.dump(known, f)
    os.replace(SEEN + ".new", SEEN)
    return summary(done)


def summary(done):
    parts = [f"{done['text']} text" + ("" if done["text"] == 1 else "s"),
             f"{done['picture']} picture" + ("" if done["picture"] == 1 else "s")]
    if done["skipped"]:
        parts.append(f"{done['skipped']} skipped")
    return "learned from " + ", ".join(parts)


# ---------------- brick learn ----------------

def command(args):
    if not args:
        print(__doc__)
        return 1
    if args[0] in ("watch", "unwatch"):
        if len(args) < 2:
            print(f"Which folder? For example: brick learn {args[0]} ~/Pictures")
            return 1
        folder = os.path.abspath(os.path.expanduser(" ".join(args[1:])))
        folders = [f for f in watched() if f != folder]
        if args[0] == "watch":
            if not os.path.isdir(folder):
                print(f"There's no folder {folder}.")
                return 1
            set_watched(folders + [folder])
            print(f"Watching {folder}: while brick runs, it learns from new or changed files there by itself.")
        else:
            set_watched(folders)
            print(f"Stopped watching {folder}.")
        return 0
    if args == ["watching"]:
        folders = watched()
        print("Watching:\n  " + "\n  ".join(folders) if folders else "No folders are watched. brick learn watch FOLDER")
        return 0
    import brain
    if brain.off():
        print("The brain is off (brick brain on), so it isn't learning.")
        return 1
    senses = Senses()
    started = time.monotonic()
    targets = args if all(os.path.exists(os.path.expanduser(a)) for a in args) else [" ".join(args)]
    total = {"text": 0, "picture": 0, "skipped": 0, "said": []}
    for target in targets:
        done = senses.learn(target)
        for key in ("text", "picture", "skipped"):
            total[key] += done[key]
        total["said"] += done["said"]
    senses.save()
    for line in total["said"][:20]:
        print("  " + line)
    if len(total["said"]) > 20:
        print(f"  ... and {len(total['said']) - 20} more")
    print(summary(total).capitalize() + f", in {time.monotonic() - started:.1f}s.")
    return 0


if __name__ == "__main__":
    sys.exit(command(sys.argv[1:]))
