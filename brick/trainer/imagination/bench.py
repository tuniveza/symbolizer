"""Timings for the imagination neurons, before and after each speed-up. Runs on a frozen copy of the learned network
so the running daemon is never disturbed and every run starts from the same place.

    python3 trainer/imagination/bench.py SNAPSHOT_FOLDER [item ...]

items: score (precision of the snapshot: p20, p100, placed, Ni, bits), read (Neurons panel: one line), start (node
start + loading network.json), round (one learning round), check (brick mode's check while typing), repair (one
repair), learn3 (3 rounds, then precision again: the check that a learning change keeps precision).
"""
import json
import os
import shutil
import sys
import tempfile
import time

HERE = os.path.dirname(os.path.dirname(os.path.dirname(os.path.abspath(__file__))))
sys.path.insert(0, HERE)

LINE = "put window at left side of screen and place text with the words of shit in brown at the top right of the window"
PROGRAM = "write hello in red\nwait 2 seconds\nwrite goodbye in teal\nclear the screen\n"


def timed(fn, *a, **k):
    t = time.perf_counter()
    out = fn(*a, **k)
    return time.perf_counter() - t, out


def main():
    snap = sys.argv[1]
    items = sys.argv[2:] or ["score", "read", "start", "check", "repair", "round"]
    work = tempfile.mkdtemp(prefix="brick-bench-")
    for name in os.listdir(snap):
        if os.path.isfile(os.path.join(snap, name)):
            shutil.copy(os.path.join(snap, name), work)
    os.environ["BRICK_NEURONS_FOLDER"] = work
    os.environ.setdefault("BRICK_TRAINER", "off")
    from trainer import intuition, neurons
    out = {}

    def precision():
        got = intuition.score()
        res = {k: got[k] for k in ("p20", "p100", "placed", "ni")}
        with open(intuition.STATE, encoding="utf-8") as f:
            res["bits"] = json.load(f).get("stats", {}).get("bits")
        return res
    if "score" in items:
        out["score"] = precision()
    if "read" in items:
        s, got = timed(neurons.read_line, LINE)
        out["read"] = round(s, 2)
    if "start" in items:
        s, got = timed(neurons.run_node, {"task": "bricks", "state": intuition.STATE, "words": ["write"]})
        out["start"] = round(s, 2)
    if "check" in items:
        import kiln
        kiln.watch("write hi", ".")                        # imports warm, as in brick serve
        s, got = timed(kiln.watch, PROGRAM, ".")
        out["check"] = round(s, 3)
    if "repair" in items:
        from trainer import repair
        s, got = timed(repair.repair, LINE)
        out["repair"] = round(s, 2)
    if "round" in items:
        intuition.raw_input()                              # warm the traces cache, as a running daemon has it
        s, got = timed(intuition.learn, 1, say=lambda x: None)
        out["round"] = round(s, 2)
    if "learn3" in items:                                  # precision after 3 more rounds from the snapshot
        intuition.raw_input()
        s, got = timed(intuition.learn, 3, say=lambda x: None)
        out["learn3"] = {"seconds": round(s, 1), **precision()}
    shutil.rmtree(work, ignore_errors=True)
    print(json.dumps(out))


if __name__ == "__main__":
    main()
