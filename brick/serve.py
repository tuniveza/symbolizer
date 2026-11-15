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
  run        text path           build it next to its file, ready to run -> program (the file to run)
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
  help                           brick's phrases and dictionary
"""
import json
import os
import sys
import threading

HERE = os.path.dirname(os.path.abspath(__file__))
if HERE not in sys.path:
    sys.path.insert(0, HERE)
os.chdir(HERE)

out_lock = threading.Lock()


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
    lines = []
    for number, raw in enumerate(asked.get("text", "").split("\n"), 1):
        stripped = raw.strip()
        kind = "blank" if not stripped else "note" if stripped.startswith("#") else "line"
        lines.append({"number": number, "kind": kind,
                      "directions": bricks.read_line(raw) if kind == "line" else []})
    return {"lines": lines}


def do_watch(asked):
    import kiln as gui
    got = gui.watch(asked.get("text", ""), folder_of(asked))
    keep = ("ok", "problems", "notes", "took", "layout", "counts", "uses_window")
    lines = [{k: l.get(k) for k in ("number", "kind", "text", "tidied", "read") if k in l} for l in got.get("lines", [])]
    return {**{k: got.get(k) for k in keep}, "lines": lines}


def do_run(asked):
    import kiln as gui
    program, folder, problems, notes = gui.build_for_running(asked.get("text", ""), asked.get("path"))
    return {"program": program, "folder": folder, "problems": problems, "notes": notes}


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
    from trainer import repair
    got = repair.repair(asked.get("text", ""), asked.get("number"))
    return {"repair": got}


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
