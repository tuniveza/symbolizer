"""brick serve: brick for Symbolizer's brick mode (and any other front end), over stdin and stdout.

One request per line, as JSON: {"id": 7, "do": "lines", "text": "write hello in red"}
One answer per line, as JSON:  {"id": 7, "ok": true, ...}   (or {"id": 7, "ok": false, "error": "..."})
Requests run side by side (a slow one, like reading with the neurons, never holds up colouring), so answers can
come back in any order: match them by id.

    python3 brick.py serve

What it can do ("do"):
  hello                          brick's version stamp and where it lives
  lines      text                every line as bricks: each word and what brick reads it as (do, thing, colour ...)
  watch      text path           build it the way brick would: problems, notes, size, how long it took
  check      text path version   the light check while typing: each line as understood, its bytes, and the program ready to run
  run        text path           build it next to its file, ready to run -> program (the file to run)
  keep       text path           as run, when sy already started the program a check had ready
  bits       text path           write the program's bits and materials next to the file
  words      what text path [word meaning where key]   list / teach / forget / try (as the Words tab)
  decisions                      what the trainer couldn't settle by itself, and what it did lately
  decide     id action meaning   decide one; decide_group key action meaning: decide a whole group
  imagination origin most        what brick imagined: pictures (full paths) and what its eye saw
  archetypes                     the imagination neurons' archetypes
  neurons    line archetype long read a line with the imagination neurons
  bitstream  line                English -> bits by the brain, judged by itself
  learning                       what the neurons have learned so far
  daemon     what                status / always / off: the training daemon
  narrator   what                status / on / off: the brains say out loud what they're learning
  help                           brick's phrases and dictionary
"""
import json
import os
import sys
import threading
from collections.abc import MutableMapping

HERE = os.path.dirname(os.path.abspath(__file__))
if HERE not in sys.path:
    sys.path.insert(0, HERE)
os.chdir(HERE)

out_lock = threading.Lock()


class ThreadEnv(MutableMapping):
    """os.environ, except that brick's own switches (BRICK_MEMORY, BRICK_NEURAL, BRICK_TRAINER) set while answering
    one request stay with that request's thread. brick turns the brain off for a moment in places (the repair's
    trial builds, the intuition's traces); with requests answered side by side, that must never switch the brain
    off for a build running at the same time."""
    KEYS = {"BRICK_MEMORY", "BRICK_NEURAL", "BRICK_TRAINER"}

    def __init__(self, real):
        self.real, self.here = real, threading.local()

    def _mine(self):
        if not hasattr(self.here, "keys"):
            self.here.keys = {}
        return self.here.keys

    def __getitem__(self, key):
        mine = self._mine()
        if key in self.KEYS and key in mine:
            if mine[key] is None:
                raise KeyError(key)
            return mine[key]
        return self.real[key]

    def __setitem__(self, key, value):
        if key in self.KEYS:
            self._mine()[key] = value
        else:
            self.real[key] = value

    def __delitem__(self, key):
        if key in self.KEYS:
            if key not in self:
                raise KeyError(key)
            self._mine()[key] = None
        else:
            del self.real[key]

    def __iter__(self):
        mine = self._mine()
        seen = set()
        for key in self.real:
            if mine.get(key, "") is not None:
                seen.add(key)
                yield key
        for key, value in mine.items():
            if value is not None and key not in seen:
                yield key

    def __len__(self):
        return sum(1 for _ in self)

    def copy(self):
        return dict(self)


def say(answer):
    line = json.dumps(answer, default=str)
    with out_lock:
        sys.stdout.write(line + "\n")
        sys.stdout.flush()


def folder_of(asked):
    path = asked.get("path")
    return os.path.dirname(os.path.abspath(os.path.expanduser(path))) if path else os.getcwd()


def do_hello(asked):
    from trainer import core
    return {"stamp": core.code_stamp(), "home": HERE}


def do_lines(asked):
    import bricks
    from teach import read_definition
    lines = []
    for number, raw in enumerate(asked.get("text", "").split("\n"), 1):
        stripped = raw.strip()
        kind = "blank" if not stripped else "note" if stripped.startswith("#") else \
            "definition" if read_definition(stripped) else "line"     # 'to shout something means ...': a new word
        lines.append({"number": number, "kind": kind,
                      "directions": bricks.read_line(raw) if kind in ("line", "definition") else []})
    return {"lines": lines}


def do_watch(asked):
    import kiln as gui
    got = gui.watch(asked.get("text", ""), folder_of(asked))
    keep = ("ok", "problems", "notes", "took", "layout", "counts", "uses_window")
    lines = [{k: l.get(k) for k in ("number", "kind", "text", "tidied", "read") if k in l} for l in got.get("lines", [])]
    return {**{k: got.get(k) for k in keep}, "lines": lines}


def do_check(asked):
    """The light check while typing: does it build, and if not, what's wrong (no disassembly; F7 does the full one)."""
    import shutil
    import tempfile
    import time
    from expressions import Problem
    from kiln import Watching
    from trainer import neural
    words_file = os.path.join(folder_of(asked), "mywords.txt")
    with tempfile.TemporaryDirectory() as scratch:
        mine = os.path.join(scratch, "mywords.txt")
        if os.path.isfile(words_file):
            shutil.copy(words_file, mine)
        started = time.perf_counter()

        def build():
            compiler = Watching(mine, ask_user=lambda question: "yes")   # writes down what each line meant
            try:
                return compiler, compiler.compile(asked.get("text", "")), []
            except Problem as problem:
                return compiler, None, str(problem).split("\n")
            except Exception as error:
                return compiler, None, [f"Line {compiler.line_number}: (brick itself tripped up here)",
                                        f"    {type(error).__name__}: {error}"]
        with neural.impatient():
            compiler, program, problems = build()   # at once, never waiting for the language model ...
        waited = False
        if neural._asking:                          # ... which was asked about new lines in the background:
            until = time.time() + 15                # wait for its answers, then build again with them, so the
            while neural._asking and time.time() < until:   # check shows what brick will really build
                time.sleep(0.1)
            compiler, program, problems = build()
            waited = True
    return {"ok": program is not None, "problems": problems, "notes": compiler.notes, "waited": waited,
            "took": round((time.perf_counter() - started) * 1000, 1), "layout": {"file": len(program) if program else 0}, "light": True,
            "lines": understood(compiler, asked.get("text", ""), problems), "version": asked.get("version"),
            "ready": ready(program, asked.get("path"))}


READY = []


def ready(program, path):
    """Keep the program the check just built, ready to run at once (Alt+R in sy starts it with no wait).
    Each build gets its own file, so one already running is never written over."""
    import hashlib
    import tempfile
    if not program:
        return None
    if not READY:
        READY.append(tempfile.mkdtemp(prefix="brick-ready-"))
    name = os.path.splitext(os.path.basename(path))[0] if path else "program"
    folder = os.path.join(READY[0], hashlib.sha1(program).hexdigest()[:12])
    output = os.path.join(folder, name or "program")
    if not os.path.isfile(output):
        os.makedirs(folder, exist_ok=True)
        with open(output + ".part", "wb") as f:
            f.write(program)
        os.chmod(output + ".part", 0o755)
        os.replace(output + ".part", output)
    return output


def understood(compiler, text, problems):
    """What brick understood each line as (its directions, as it meant them), how many bytes of code each made,
    and the problem brick has with it, if any: the one panel's view of the program."""
    import re
    from kiln import owners
    code = compiler.a.code
    start, end = compiler.program_at or 0, compiler.program_end if compiler.program_end is not None else len(code)
    made = {}
    for at, number in enumerate(owners(compiler.spans, len(code))):
        if number is not None and start <= at < end:
            made[number] = made.get(number, 0) + 1
    trouble, current = {}, None
    for line in problems:
        m = re.match(r"Line (\d+): (.*)", line)
        if m:
            current = int(m[1])
            trouble.setdefault(current, []).append(m[2].strip())
        elif current is not None and line.strip():
            trouble[current].append(line.strip())
    lines = text.split("\n")

    return [{"number": n, "meant": [b for _, b in compiler.tidied.get(n, [])], "bytes": made.get(n, 0),
             "problem": " ".join(trouble.get(n, [])) or None, "left out": []} for n in range(1, len(lines) + 1)]


def do_run(asked):
    import kiln as gui
    program, folder, problems, notes = gui.build_for_running(asked.get("text", ""), asked.get("path"))
    return {"program": program, "folder": folder, "problems": problems, "notes": notes}


def do_keep(asked):
    """As run, after sy already started the program the check had ready: the program is also written next to its
    file, and brick learns from the build. sy doesn't start it a second time."""
    return do_run(asked)


def do_bits(asked):
    import kiln as gui
    written = gui.write_materials(asked.get("text", ""), asked.get("path"), folder_of(asked))
    if written is None:
        raise ValueError("It didn't build, so there are no bits yet.")
    return {"paths": written}


def do_words(asked):
    import words
    mine = os.path.join(folder_of(asked), "mywords.txt")
    what = asked.get("what", "list")
    if what == "list":
        return {**words.listing(mine), "unknown": words.unknown(asked.get("text", ""), mine)}
    if what == "teach":
        ok, message, example = (words.teach_here if asked.get("where") == "here" else
                                lambda w, m, _=None: words.teach(w, m))(asked.get("word", ""), asked.get("meaning", ""), mine)
        return {"done": ok, "message": message, "example": example}
    if what == "forget":
        ok, message = words.forget_here(asked.get("key", ""), mine) if asked.get("where") == "here" \
            else words.forget(asked.get("word", ""))
        return {"done": ok, "message": message}
    if what == "try":
        return words.try_text(asked.get("text", ""), mine)
    raise ValueError(f"words can't '{what}'")


def do_decisions(asked):
    from trainer import core, outliers
    log = os.path.join(core.FOLDER, "log.txt")
    activity = []
    if os.path.isfile(log):
        with open(log, encoding="utf-8", errors="replace") as f:
            activity = [line.rstrip() for line in f][-40:][::-1]
    return {"running": core.running(), "activity": activity, "items": outliers.entries(), "groups": outliers.groups()}


def do_decide(asked):
    from trainer import outliers
    ok, message = outliers.decide(asked.get("id", ""), asked.get("action", ""), asked.get("meaning", "").strip())
    return {"done": ok, "message": message}


def do_decide_group(asked):
    from trainer import outliers
    ok, message = outliers.decide_group(asked.get("key", ""), asked.get("action", ""), asked.get("meaning", "").strip())
    return {"done": ok, "message": message}


def do_imagination(asked):
    from trainer import core, imagine
    origin = asked.get("origin") if asked.get("origin") in imagine.ORIGINS else None
    items = imagine.entries(origin, most=min(200, int(asked.get("most") or 40)))
    for item in items:
        item["picture"] = os.path.join(imagine.PICTURES, item["picture"]) if item.get("exists") else None
        item.pop("path", None)
    tally = imagine.Tally()
    return {"items": items, "eye": tally.summary(), "totals": dict(tally.totals), "running": core.running()}


def do_archetypes(asked):
    from trainer import neurons
    return {"archetypes": neurons.archetypes()}


def do_neurons(asked):
    from trainer import neurons
    return neurons.read_line(asked.get("line", ""), asked.get("archetype") or None, bool(asked.get("long")))


def do_bitstream(asked):
    from trainer import intuition
    got = intuition.bitstream(asked.get("line", ""))
    return {"bytes": len(got["bits"]) if got["bits"] else 0, "line": got["line"], "by": got["by"],
            "precise": got["precise"], "why": got["why"],
            "tried": [{"line": t["line"], "builds": t["bits"] is not None} for t in got["tried"][:8]]}


def do_repair(asked):
    """The brain's check: does every line build what was meant? A precise fix is the brain's pick. When it isn't
    sure between a few fixes, the language model picks the one the person most likely meant (put first, "picked")."""
    from trainer import repair
    text = asked.get("text", "")
    got = repair.repair(text, asked.get("number"))
    if got and not got.get("precise") and len(got.get("fixes") or []) == 1:
        got["picked"] = True                          # the only fix that passed the brain's checks: its pick
    elif got and not got.get("precise") and len(got.get("fixes") or []) > 1:
        from trainer import neural
        mine = set(text.split("\n"))
        ways = [" / ".join(l.strip() for l in f["text"].split("\n") if l not in mine) or f["text"] for f in got["fixes"]]
        try:
            pick = neural.choose(got["line"], ways, "\n".join(text.split("\n")[:max(0, got["number"] - 1)]))
        except Exception:
            pick = None
        if isinstance(pick, int) and 0 <= pick < len(got["fixes"]):
            got["fixes"].insert(0, got["fixes"].pop(pick))
            got["picked"] = True
    if got and got.get("picked"):                    # sy writes a pick in by itself only when it comes from the
        first = (got["fixes"][0].get("changes") or [""])[0]   # line's own meaning: the model's reading, a new action
        got["picked"] = first.startswith(("read as", "new action", "split"))   # or a split, never an unrelated added step
    return {"repair": got, "version": asked.get("version")}


def do_learning(asked):
    from trainer import intuition
    got = intuition.learned() or {}
    got["proved"] = [list(p) for p in got.get("proved", [])][:40]
    return got


def do_daemon(asked):
    from trainer import daemon
    what = asked.get("what", "status")
    if what in ("always", "off"):
        import io
        import contextlib
        said = io.StringIO()
        with contextlib.redirect_stdout(said):
            code = daemon.command(what)
        if code:
            raise ValueError(said.getvalue().strip() or "systemd said no")
    return daemon.state()


def do_narrator(asked):
    """The brains' voices: status / on / off (trainer/narrator.py; the daemon speaks when it's on)."""
    from trainer import narrator
    what = asked.get("what", "status")
    if what in ("on", "off"):
        narrator.set_on(what == "on")
    return {"on": narrator.on()}


def do_help(asked):
    import actions
    import dictionary
    return {"phrases": actions.help_text(), "dictionary": dictionary.listing(None)}


def handle(request):
    rid = request.get("id")
    try:
        fn = globals().get("do_" + str(request.get("do", "")).replace("-", "_"))
        if not fn:
            raise ValueError(f"brick serve can't '{request.get('do')}'")
        say({"id": rid, "ok": True, **fn(request)})
    except Exception as error:                        # an answer, never a crash
        say({"id": rid, "ok": False, "error": f"{type(error).__name__}: {error}" if not isinstance(error, ValueError) else str(error)})


def main():
    os.environ.setdefault("BRICK_TRAINER", "off")     # the daemon does the learning; serving never starts a trainer
    os.environ = ThreadEnv(os.environ)                # brick's switches set by one request stay with it
    for line in sys.stdin:
        line = line.strip()
        if not line:
            continue
        try:
            request = json.loads(line)
        except ValueError:
            say({"id": None, "ok": False, "error": "that wasn't JSON"})
            continue
        threading.Thread(target=handle, args=(request,), daemon=True).start()


if __name__ == "__main__":
    main()
