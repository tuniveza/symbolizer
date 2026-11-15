"""Repair: when brick builds a line but misreads it, or can't build it, find what's missing and fix it, judged by
the digital brain and checked by the compiler. For anything, not one kind of line.

How it knows a line went wrong (the brain's own judgement):
  the brain's bitstream neurons predict which pieces of output the line's action words make ("put" -> place);
  brick's real build is traced (which action routines ran for each line, and how brick re-read the line).
  A line whose build is missing a predicted piece, or that failed, or that brick re-read into another action
  ("put window at left" read as "write window ...") is not precise.

What it tries (every one is built and traced for real):
  1. a missing step before the line: what Ni learned usually comes before the predicted action, said the
     shortest way brick knows to make exactly that piece ("open a window" before placing things in it);
  2. the action word swapped for words the brain proved alike (put -> place, move, position ...);
  3. the line split at "and" / "then" into separate lines;
  4. the brick phrasings brick's own sentences use with the same action and content words (from the pool).
  Steps can combine (a missing step and a swap), up to two changes.

What it keeps: the fix whose build has every piece the brain predicted, keeps the line's content words out of
the "words to show" (so nothing meant as an action is just printed), and changes the least. Precise when it's the
only best fix, or every best fix builds the same bits; otherwise it offers the best few and says it isn't sure.
"""
import json
import os
import re
import tempfile
from collections import Counter

from trainer import intuition, neural, neurons

ORDER = os.path.join(intuition.FOLDER, "order.json")     # Ni's learned order of output pieces (from programs)


# ---- brick's real build, traced ----

def traced_build(text, words_file=None):
    """Build like brick does (the universe helping, as in brick mode) and note, per line, which action routines
    ran and how brick re-read the line. -> {"ok", "bits", "lines": {n: [pieces]}, "read": {n: "read as"}, "problems"}"""
    from compiler import Compiler
    from expressions import Problem
    saved = {k: os.environ.get(k) for k in ("BRICK_MEMORY", "BRICK_NEURAL", "BRICK_TRAINER")}
    os.environ.update(BRICK_MEMORY="off", BRICK_NEURAL="off", BRICK_TRAINER="off")
    ran = {}
    try:
        with tempfile.TemporaryDirectory() as where:
            mine = os.path.join(where, "mywords.txt")
            if words_file and os.path.isfile(words_file):
                with open(words_file, encoding="utf-8") as f, open(mine, "w", encoding="utf-8") as g:
                    g.write(f.read())
            c = Compiler(mine, ask_user=lambda q: "yes")
            for name in [n for n in dir(c) if intuition.is_action(n)]:
                real = getattr(c, name)

                def spy(*a, _real=real, _name=name, **k):
                    ran.setdefault(c.line_number, []).append(intuition.piece_name(_name))
                    return _real(*a, **k)
                setattr(c, name, spy)
            problems, bits = [], None
            try:
                bits = c.compile(text)
            except Problem as p:
                problems = str(p).split("\n")
            except Exception as e:
                problems = [f"brick tripped up: {type(e).__name__}: {e}"]
            read = {}
            for n in c.notes:
                m = re.match(r"Note: line (\d+) read as: (.*)", n)
                if m:
                    read[int(m.group(1))] = m.group(2)
    finally:
        for k, v in saved.items():
            if v is None:
                os.environ.pop(k, None)
            else:
                os.environ[k] = v
    ran = {k: [p for p in v if p != "=gui_event"] for k, v in ran.items()}   # gui_event runs for every line
    return {"ok": bits is not None, "bits": bits, "lines": {k: list(dict.fromkeys(v)) for k, v in ran.items()},
            "read": read, "problems": problems}


# ---- what the brain expects ----

def expected(line):
    """The output pieces the brain predicts the line's action words make: {piece: word}."""
    roles = intuition.roles_in(line)
    words = [w for w, r in roles if r == "do"] or [w for w in neurons.words_of(line)[:1]]
    got = intuition.brain_bricks(set(words)) if words else {}
    out = {}
    for w in words:
        p = (got.get(w) or {})
        if p.get("output") and p.get("share", 0) >= 0.3:
            out[p["output"]] = w
    return out


# ---- Ni's order of pieces, and the shortest English for each piece ----

def learn_order():
    """From every program brick has (the examples and your programs in wa/), the pieces each line makes, in
    order: which piece usually comes before which (Ni over output pieces), and the shortest line brick knows
    that makes each piece on its own. Saved in order.json."""
    files = []
    for folder in (neurons.EXAMPLES, intuition.WAREHOUSE):
        if os.path.isdir(folder):
            files += [os.path.join(folder, n) for n in sorted(os.listdir(folder)) if n.endswith(".txt")]
    before, seen = Counter(), Counter()
    for path in files:
        with open(path, encoding="utf-8", errors="replace") as f:
            text = f.read()
        got = traced_build(text)
        seq = [p for n in sorted(got["lines"]) for p in got["lines"][n]]
        for i, b in enumerate(seq):
            seen[b] += 1
            for a in set(seq[:i]):
                if a != b:
                    before[a + " " + b] += 1
    alone = {}
    traces = intuition._traces if intuition._traces is not None else {}
    if not traces and os.path.isfile(intuition.TRACES):
        with open(intuition.TRACES, encoding="utf-8") as f:
            traces = json.load(f)
    for line, pieces in traces.items():
        if pieces and len(pieces) == 1 and 2 <= len(line.split()) <= 6 and line.isascii():
            p = pieces[0]
            if p not in alone or len(line) < len(alone[p]):
                alone[p] = line.lower().strip()
    data = {"before": dict(before), "seen": dict(seen), "alone": alone}
    os.makedirs(intuition.FOLDER, exist_ok=True)
    with open(ORDER, "w", encoding="utf-8") as f:
        json.dump(data, f)
    return data


def order():
    try:
        with open(ORDER, encoding="utf-8") as f:
            return json.load(f)
    except (OSError, ValueError):
        return learn_order()


def comes_before(piece, data, most=3):
    """Pieces Ni saw come before this one in most programs that have it, strongest first."""
    seen = data["seen"].get(piece, 0)
    if not seen:
        return []
    scored = [(n / seen, a.split(" ")[0]) for a, n in data["before"].items() if a.endswith(" " + piece)]
    return [a for s, a in sorted(scored, reverse=True) if s >= 0.5][:most]


# ---- the repair ----

def judge(text, number, want, base_line):
    """How good a try is: (it does an expected action, content kept). want: the pieces the brain expects from the
    tried line's own action words (after a swap, the new word's)."""
    got = traced_build(text)
    if not got["ok"]:
        return None, got
    made = set(got["lines"].get(number, []))               # what the repaired line itself made
    covered = any(p in made for p in want) if want else bool(made - {"=write"})
    content = [w for w, r in intuition.roles_in(base_line) if r in ("thing", "place")]
    read = " ".join(got["read"].values()).lower()
    shown = re.findall(r"\bwrite ([a-z ]+?)(?: in | at | on |$)", read)
    lost = [w for w in content if any(w in s.split() for s in shown)]
    return (covered, not lost), got


def _judge(job):
    text, n, want, line = job
    score, got = judge(text, n, want, line)
    return score, {"bits": got["bits"], "made": got["lines"].get(n, [])} if score is not None else None


def repair(text, number=None):
    """Find a precise fix for the first line that isn't precise (or line `number`).
    -> {"line", "number", "why", "fixes": [{"text", "changes", "bytes"}], "precise", "judged"} or None when
    every line is precise already."""
    data = order()
    lines = text.split("\n")
    base = traced_build(text)
    targets = [number] if number else range(1, len(lines) + 1)
    for n in targets:
        line = lines[n - 1].strip()
        from teach import read_definition, opens_block
        if not line or line.startswith("#") or read_definition(line) or opens_block(line):
            continue                                       # a new word being taught is the person's own: never "fixed"
        want = expected(line)
        made = set(base["lines"].get(n, []))
        reread = base["read"].get(n, "")
        missing = [p for p in want if p not in made]
        verbs = [w for w, r in intuition.roles_in(line) if r == "do"]
        changed_verb = reread and verbs and not any(v in reread.lower().split() for v in verbs)
        from trainer import inventor
        lacking = inventor.unknown_action(line) if not base["ok"] or made <= {"=write"} else None
        if base["ok"] and not missing and not changed_verb and not lacking:
            continue
        why = ("brick can't build it" if not base["ok"] else
               f"brick read it as '{reread}'" if changed_verb else
               f"its build has no {', '.join(missing)}" if missing else
               f"brick has no action for '{lacking}': it would only show the words")
        tries = []
        indent = re.match(r"\s*", lines[n - 1]).group(0)
        # 1. a missing step before it
        for piece in list(want) or list(made):
            for prior in comes_before(piece, data):
                say = data["alone"].get(prior)
                if say and prior not in made:
                    tries.append((lines[:n - 1] + [indent + say] + lines[n - 1:], [f"added '{say}' before it"]))
        # 2. the action word swapped for proved-alike words that make the expected piece
        ev = intuition.evidence()
        swaps = []
        for v in verbs:
            for other, level, w in ev.get(v, []):
                if level in ("meaning", "role") and intuition.role_of(other) == "do":
                    swaps.append((v, other))
        for v, other in swaps[:6]:
            new = re.sub(rf"\b{re.escape(v)}\b", other, line, count=1, flags=re.IGNORECASE)
            tries.append((lines[:n - 1] + [indent + new] + lines[n:], [f"'{v}' -> '{other}'"]))
        # 2b. a thing swapped for a proved-alike thing (window -> console): it changes the meaning, never precise
        for w, r in intuition.roles_in(line):
            if r in ("thing", "words"):
                for other, level, wt in ev.get(w, [])[:4]:
                    if intuition.role_of(other) == "thing":
                        new = re.sub(rf"\b{re.escape(w)}\b", other, line, flags=re.IGNORECASE)
                        tries.append((lines[:n - 1] + [indent + new] + lines[n:], [f"'{w}' -> '{other}' (another thing)"]))
        # 3. split at and / then
        parts = [p.strip() for p in re.split(r"\b(?:and then|then|and)\b", line) if p.strip()]
        if len(parts) > 1:
            tries.append((lines[:n - 1] + [indent + p for p in parts] + lines[n:], ["split into separate lines"]))
        # combinations: a missing step together with each swap or split
        firsts = [t for t in tries if t[1][0].startswith("added")]
        for f_lines, f_change in firsts[:2]:
            for t_lines, t_change in [t for t in tries if not t[1][0].startswith("added")][:4]:
                tries.append((f_lines[:n - 1 + 1] + t_lines[n - 1:], f_change + t_change))
        # 4. the language model's reading of the line, in brick's own words (each direction its own line)
        before = "\n".join(lines[max(0, n - 3):n - 1])
        ways = [neural.read_pieces(line)] + neural.readings(line)[:1]
        for way in dict.fromkeys(w for w in ways if w):
            parts = [p.strip() for p in re.split(r"\bthen\b", way) if p.strip()]
            if parts and [p.lower() for p in parts] != [line.lower()]:
                tries.append((lines[:n - 1] + [indent + p for p in parts] + lines[n:], [f"read as '{way}'"]))

        def judge_all(tries):
            jobs = []
            for t_lines, change in tries:                   # what the brain expects first (quick), then build them all side by side
                at = n - 1 + sum(1 for c in change if c.startswith(("added", "new action")))   # your line moves down past added lines
                tried_line = t_lines[at] if at < len(t_lines) else line
                jobs.append(("\n".join(t_lines), at + 1, expected(tried_line) or want, line))
            judged = []
            for (t_lines, change), (score, got) in zip(tries, intuition.pmap(_judge, jobs)):
                if score is None:
                    continue
                covered, kept = score
                if lacking and set(got["made"]) <= {"=write"}:
                    continue                                # it asked for an action: a fix that only shows words isn't one
                judged.append({"text": "\n".join(t_lines), "changes": change, "covered": covered, "kept": kept,
                               "bits": got["bits"], "bytes": len(got["bits"])})
            return judged
        judged = judge_all(tries)
        good = [j for j in judged if j["covered"] and j["kept"]] or [j for j in judged if j["covered"]]
        if not good:
            # 5. brick has no action for it: the brain invents one from what brick can already do
            from trainer import inventor
            verb = inventor.unknown_action(line) or (verbs[0] if verbs else None)
            made = [(lines[:n - 1] + [indent + d] + lines[n - 1:], [f"new action: {d}"])
                    for d in inventor.invent(line, before, verb)]
            # a definition goes before the first line that uses it; at the top keeps it out of the way
            made = [([t_lines[n - 1]] + t_lines[:n - 1] + t_lines[n:], change) if n > 1 else (t_lines, change)
                    for t_lines, change in made]
            for j in judge_all([(t, c) for t, c in made]):
                judged.append(j)
            good = [j for j in judged if j["covered"] and j["kept"]] or [j for j in judged if j["covered"]]
        good.sort(key=lambda j: len(j["changes"]))
        best = [j for j in good if len(j["changes"]) == len(good[0]["changes"])] if good else []
        precise = bool(best) and len({j["bits"] for j in best}) == 1 and \
            not any("another thing" in c or c.startswith("new action") for j in best for c in j["changes"])
        return {"line": line, "number": n, "why": why, "precise": precise, "judged": len(judged),
                "expected": list(want),
                "fixes": [{"text": j["text"], "changes": j["changes"], "bytes": j["bytes"]} for j in (best or good)[:3]]}
    return None


def command(args):
    path = args[0] if args else None
    if not path or not os.path.isfile(path):
        print("brick trainer neurons repair FILE: find what's missing in a program and fix it, checked by the compiler.")
        return 1
    with open(path, encoding="utf-8") as f:
        got = repair(f.read())
    if not got:
        print("Every line is precise: the brain expected what brick built.")
        return 0
    print(f"Line {got['number']}: '{got['line']}': {got['why']}.")
    if not got["fixes"]:
        print(f"Tried {got['judged']} fixes; none built what the brain expected ({', '.join(got['expected']) or 'nothing predicted'}).")
        return 1
    print(("Precise fix" if got["precise"] else "Not sure; the best fixes") + f" (tried {got['judged']}):")
    for f in got["fixes"]:
        print(f"  - {'; '.join(f['changes'])} ({f['bytes']} bytes)")
    return 0
