"""The digital brain reads a line: the only part of brick that decides how messy English is read.

Everything it knows comes from its own imagination: the 48 archetypes, each with its own personality, voice
directions in their own way (trainer/voices.py), and the brain practises on those (each keeps a known meaning).
Reading a line is the brain's functions working together:

  Se  takes in the line: its words, where each stands, its neighbours, its shape
  Ni  sees what it is: which brick each direction is (trainer/picker.py) and what each word is for: the words
      to show, a colour, a place, a count, a time, a file, the window ... (trainer/details.py)
  Ne  tries other ways when the first doesn't hold: the next brick, a word's other role, the line whole
  Ti  checks the reading holds together: every word has a role it fits, one action word, one stretch of
      words to show, the action word suits the brick
  Te  checks it in the world: brick builds the reading (brick's builder only turns a reading into bytes)

It practises hardest where it fails on your own English: each of your lines it reads wrong (real-lines.tsv;
every 'Not what I meant' answer lands there too) is said again many ways with exactly your meaning (mistakes).

When the brain isn't sure, it says nothing, and the line is read as brick's own language, as written.

    python3 -m trainer.mind "a line"        what the brain reads it as
    python3 -m trainer.mind learn           practise: the archetypes' voiced directions -> the brain's links
    python3 -m trainer.mind real            the brain against your own lines (trainer/data/real-lines.tsv)
    python3 -m trainer.mind mistakes [N]    practise on mistakes: for each of your lines brick reads wrong, your line
                                            and N new ways the archetypes say what you meant (100)
    python3 -m trainer.mind daily [N]       the brain's day: practise on mistakes, N new voiced paragraphs (3000),
                                            then it learns again
                                            (each night at 3am: systemctl --user status brick-imagination.timer)
"""

import os
import sys
import tempfile

HERE = os.path.dirname(os.path.abspath(__file__))
sys.path.insert(0, os.path.dirname(HERE))

REAL = os.path.join(HERE, "data", "real-lines.tsv")


def off():
    """Off with BRICK_MIND=off (or BRICK_PICKER=off, or all learned memory off: BRICK_MEMORY=off, as in tests)."""
    from trainer import picker
    return os.environ.get("BRICK_MIND") == "off" or picker.off()


def say(piece):
    """(Ni, Ne) one direction: its readings, best linked first, and the first that can be said in brick's words
    -> (text, how far ahead it is of the best other reading), or (None, 0)."""
    from trainer import details, links
    words = links.words_of(piece)
    found = links.links().read(words)
    if not found:
        return None, 0.0
    said = []
    for score, brick, roles in found[:12]:
        text = details.say_again_parts(brick, words, roles)
        if text and details.accounts_for(text, roles):        # every linked word accounted for
            said.append((score, text))
    if not said:
        return None, 0.0
    best = said[0]
    other = next((sc for sc, t in said[1:] if t != best[1]), best[0] - 99)
    return best[1], best[0] - other


def read_line(line):
    """The whole line: cut into directions (the cutter), each said again -> (text, how sure: the smallest gap)."""
    from trainer import cutter, details, universe as u
    frames = cutter.cut(u.tokens_of(line, gaps=True))
    pieces = [" ".join(t.text for t in fr) for fr in frames] if frames else [line]
    said, sure = [], 99.0
    for piece in pieces:
        if all(details.filler(w) for w in piece.split()):
            continue
        text, gap = say(piece)
        if not text:
            return None, 0.0
        said.append(text)
        sure = min(sure, gap)
    return (" then ".join(said), sure) if said else (None, 0.0)


def read(line, words_file=None):
    """What the digital brain reads this line as, in brick's own words (Te: checked by building it), or None
    when it isn't sure."""
    from trainer import links
    if off() or not line.strip():
        return None
    try:
        way, sure = read_line(line)
    except Exception:
        return None
    if not way or sure < links.SURE:
        return None
    from compiler import failing_lines
    folder = None
    if words_file is None or not os.path.isfile(words_file):
        folder = tempfile.mkdtemp(prefix="mind-")
        words_file = os.path.join(folder, "mywords.txt")
    try:
        return None if failing_lines(way, words_file) else way
    finally:
        if folder:
            import shutil
            shutil.rmtree(folder, ignore_errors=True)


def learn():
    """The brain practises on its archetypes' voiced directions: the universal linkage (trainer/links.py)."""
    from trainer import links
    links.learn()


def real_lines(path=REAL):
    """Your own lines and what each plainly means -> [(line, meant)]."""
    if not os.path.isfile(path):
        return []
    with open(path, encoding="utf-8") as f:
        return [tuple(r.rstrip("\n").split("\t", 1)) for r in f if "\t" in r and not r.startswith("#")]


def own_parts(line, tidy):
    """Your line cut into one piece per direction it means (the cutter), or None when the pieces don't match."""
    if len(tidy) == 1:
        return [line]
    from trainer import cutter, details, universe as u
    try:
        frames = cutter.cut(u.tokens_of(line, gaps=True))
    except Exception:
        return None
    pieces = [" ".join(t.text for t in fr) for fr in frames] if frames else [line]
    pieces = [p for p in pieces if not all(details.filler(w) for w in p.split())]
    return pieces if len(pieces) == len(tidy) else None


def mistakes(per_line=100, path=REAL, say=print):
    """Practise on mistakes: where brick reads one of your own lines wrong (it builds something other than what
    you meant, or nothing), the brain practises hardest on exactly that meaning. Your line itself goes into its
    practice (once each run, when every piece keeps the meaning), and the archetypes say the same meaning
    per_line other ways: half through the language model (trainer/voices.py), half through their own networks
    (trainer/speech.py). Only sayings that keep the meaning exactly are kept. -> (mistakes, practice rows added)"""
    import json
    import random
    import shutil
    import time
    from compiler import Compiler
    from trainer import scrambler, voices
    try:
        from trainer import speech
    except Exception:
        speech = None
    rng = random.Random(int(time.time()))

    found = []                                     # (line, its tidy meaning), where brick reads it wrong
    for line, meant in real_lines(path):
        folder = tempfile.mkdtemp(prefix="mind-")
        words = os.path.join(folder, "mywords.txt")
        try:
            try:
                want = Compiler(words, ask_user=lambda q: "no").compile_text(meant)
            except Exception:
                want = None                        # the meaning itself doesn't build: nothing to practise on
            try:
                got = Compiler(words, ask_user=lambda q: "no").compile(line)
            except Exception:
                got = None
        finally:
            shutil.rmtree(folder, ignore_errors=True)
        if want is not None and got != want:
            found.append((line, [t.strip() for t in meant.split(" then ") if t.strip()]))
    say(f"{len(found)} of your lines brick reads wrong")
    if not found:
        return 0, 0

    turns = []

    def next_voice():
        """Every archetype takes its turn (yours twice a round)."""
        if not turns:
            turns.extend(voices.archetypes() + voices.yours())
            rng.shuffle(turns)
        return turns.pop() if turns else None

    added, seen = 0, set()
    with open(voices.PRACTICE, "a", encoding="utf-8") as out:
        def write(mess, parts, tidy, voice):
            nonlocal added
            if mess in seen:
                return False
            seen.add(mess)
            out.write(json.dumps({"mess": mess, "parts": parts, "tidy": tidy, "voice": voice}) + "\n")
            out.flush()
            added += 1
            return True

        for line, tidy in found:
            before = added
            parts = own_parts(line, tidy)          # (a) your own line, as you said it
            if parts and all(voices.problem(p, t) is None for p, t in zip(parts, tidy)):
                write(line, parts, tidy, "your line")
            by_network = per_line // 2 if speech and hasattr(speech, "speak") else 0
            made, tries = 0, 0                     # (b) the archetypes' networks say it their way
            while made < by_network and tries < by_network * 3:
                chosen = next_voice()
                if not chosen:
                    break
                batch = min(5, by_network - made)  # one archetype says it a few times in one go
                tries += batch
                try:
                    said = speech.speak(tidy * batch, chosen["name"], seed=rng.randint(1, 10 ** 6))
                except Exception:
                    said = []
                if len(said) != len(tidy) * batch:
                    continue
                for k in range(batch):
                    bits = said[k * len(tidy):(k + 1) * len(tidy)]
                    if all(isinstance(b, str) for b in bits) and all(voices.problem(b, t) is None
                                                                     for b, t in zip(bits, tidy)):
                        mess = "".join((rng.choice(scrambler.JOINS) if j else "") + b for j, b in enumerate(bits))
                        made += write(mess, bits, tidy, chosen["name"] + " (neurons, your mistake)")
            wanted = per_line - made               # (c) the language model says the rest, in each one's voice
            made, tries = 0, 0
            while made < wanted and tries < wanted * 3:
                tries += 1
                got = voices.attempt(tidy, next_voice())
                if got is None:                    # no model, or no archetypes
                    say("  the language model isn't here, so the archetypes can't speak through it")
                    break
                mess, bits, name, why = got
                if not why:
                    made += write(mess, bits, tidy, f"{name} (your mistake)")
            say(f"  {added - before} practice rows for: {line}")
    note = f"practised on {len(found)} of your lines brick reads wrong: {added} practice rows"
    say(note)
    return len(found), added


def daily(count=3000):
    """The brain's day: first it practises on your lines it reads wrong (mistakes), then its archetypes voice
    count new practice paragraphs in their own personalities (your own archetypes in your way), then the brain
    learns from its practice again: the bricks, the words' roles, and where directions end. Run each night by
    brick-imagination.timer."""
    import time
    from trainer import cutter, speech, voices
    started = time.time()
    wrong, extra = mistakes()
    made = speech.practice(int(count * 0.8), seed=int(started))          # the archetypes' own networks speak
    helped = voices.practice(count - int(count * 0.8), seed=int(started) + 1, workers=4)   # the model complements
    learn()
    cutter.learn(4000)
    note = (f"{extra} practice rows for the {wrong} of your lines it read wrong, the archetypes' networks said {made} "
            f"new paragraphs (the language model added {helped}), and the brain learned from them "
            f"({time.time() - started:.0f}s)")
    with open(os.path.join(HERE, "data", "log.txt"), "a", encoding="utf-8") as f:
        f.write(time.strftime("%Y-%m-%d %H:%M:%S") + "  daily: " + note + "\n")
    print(note)


def check(limit=None):
    """The brain on the voiced directions it was never taught (links.held_out): how often its reading builds exactly
    what the archetype meant, and how that depends on how sure it was (to set links.SURE)."""
    import json
    import shutil
    from compiler import Compiler
    from trainer import links, voices
    with open(voices.PRACTICE, encoding="utf-8") as f:
        rows = [json.loads(r) for r in f if r.strip()]
    tests = [(p, t) for k, d in enumerate(rows) if links.held_out(k) for p, t in zip(d["parts"], d["tidy"])]
    tests = tests[:limit] if limit else tests
    folder = tempfile.mkdtemp(prefix="mind-")
    words = os.path.join(folder, "mywords.txt")

    def build(text):
        try:
            return Compiler(words, ask_user=lambda q: "no").compile_text(text)
        except Exception:
            return None
    results = []
    try:
        for part, tidy in tests:
            text, gap = say(part)
            want = build(tidy)
            results.append((gap, text is not None and want is not None and build(text) == want))
    finally:
        shutil.rmtree(folder, ignore_errors=True)
    n = len(results)
    print(f"{n} voiced directions it never learned from: {sum(ok for _, ok in results)} read exactly right "
          f"({100 * sum(ok for _, ok in results) / max(n, 1):.1f}%)")
    for bar in (0, 0.5, 1, 1.5, 2, 3, 4):
        kept = [ok for gap, ok in results if gap >= bar]
        if kept:
            print(f"  when at least {bar} ahead: it reads {len(kept)} ({100 * len(kept) / n:.0f}%), "
                  f"{100 * sum(kept) / len(kept):.1f}% of them right")
    return results


def real(path=REAL):
    """The brain, and brick as a whole, against your own lines (line <tab> what it plainly means)."""
    import shutil
    from compiler import Compiler
    rows = [r.rstrip("\n").split("\t") for r in open(path, encoding="utf-8") if "\t" in r and not r.startswith("#")]
    right = 0
    for line, meant in rows:
        folder = tempfile.mkdtemp(prefix="mind-")
        words = os.path.join(folder, "mywords.txt")
        try:
            want = Compiler(words, ask_user=lambda q: "no").compile_text(meant)
            try:
                c = Compiler(words, ask_user=lambda q: "no")
                got = c.compile(line)
            except Exception:
                got = None
            ok = got == want
            right += ok
            print(("right  " if ok else "WRONG  ") + line + ("" if ok else f"\n         brain: {read(line)}"))
        finally:
            shutil.rmtree(folder, ignore_errors=True)
    print(f"{right} of {len(rows)} of your lines right")
    return right, len(rows)


def command(args):
    if args[:1] == ["learn"]:
        return learn()
    if args[:1] == ["real"]:
        return real()
    if args[:1] == ["check"]:
        return check(int(args[1]) if args[1:] else None)
    if args[:1] == ["mistakes"]:
        return mistakes(int(args[1]) if args[1:] else 100)
    if args[:1] == ["daily"]:
        return daily(int(args[1]) if args[1:] else 3000)
    if args:
        print(read(" ".join(args)))
        return
    print(__doc__)


if __name__ == "__main__":
    command(sys.argv[1:])
