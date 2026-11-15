"""The imagination neurons learning by themselves (trainer/intuition.py, trainer/imagination/sim.js)."""
import json
import os
import shutil
import subprocess
import sys
import tempfile

HERE = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
sys.path.insert(0, HERE)
os.environ.update(BRICK_TRAINER="off", BRICK_MEMORY="off", BRICK_NEURAL="off")

from trainer import intuition  # noqa: E402

SIM = os.path.join(HERE, "trainer", "imagination", "sim.js")


def node(script):
    return subprocess.run(["node", "-e", script], capture_output=True, text=True, timeout=300, cwd=os.path.dirname(SIM))


def test_learns_from_raw_text():
    # nothing given but sentences: Ni's guesses must get better, and Ne must propose lines
    got = node("""
const S = require('./sim.js');
const net = new S.Network(S.dataSpec(S.SAMPLE_DATA, 'raw', 1), 1, null, null, { intuition: true });
let early = 0;
for (let k = 0; k < 3000; k++) { if (k % 20 === 0) net.train(); net.step(); if (k === 500) early = net.niRate(); }
const saved = net.save(), back = new S.Network({ neurons: [] }, 1, null, null, { intuition: true }).load(saved);
console.log(JSON.stringify({ early, late: net.niRate(), proposed: net.proposed, n: net.N.length, links: net.links.size,
  backN: back.N.length, backLinks: back.links.size }));
""")
    assert got.returncode == 0, got.stderr
    r = json.loads(got.stdout)
    assert r["late"] > r["early"], r
    assert r["proposed"] > 0, r
    assert (r["backN"], r["backLinks"]) == (r["n"], r["links"]), r


def test_neutral_network_unchanged():
    # the first version (groups given, no archetype, no intuition) must behave exactly as before
    got = node("""
const S = require('./sim.js');
const n = new S.Network(S.wordSpec(1.2), 1);
for (let k = 0; k < 3000; k++) { if (k % 20 === 0) n.train(); n.step(); }
console.log(n.N.filter(x => x.label.includes('·')).map(x => x.label).sort().join(','));
""")
    assert got.returncode == 0, got.stderr
    assert got.stdout.strip() == "crap·1,crap·2,test·1,test·2", got.stdout


def test_word_parts():
    assert ("write", "meaning") in intuition.word_parts("writes")
    assert ("paint", "meaning") in intuition.word_parts("painted")


def test_brick_reads_first():
    # a line brick builds by itself keeps brick's own bits: the brain never overrides it
    got = intuition.bitstream("write hello in red")
    assert got["by"] == "brick" and got["precise"] and got["bits"] == intuition.builds("write hello in red")


def test_inflection_is_precise():
    got = intuition.bitstream("writes hello in red")
    if got["by"] == "brick":                     # brick learned the form itself meanwhile: also fine
        return
    assert got["bits"] == intuition.builds("write hello in red") and got["precise"], got


def test_check_cuts_a_different_role():
    ok, why, level, built = intuition.check({"a": "left", "b": "red", "original": "write hello at the left",
                                             "swapped": "write hello at the red"})
    assert ok is False, why


if __name__ == "__main__":
    if not shutil.which("node"):
        print("skipped: node isn't installed")
        sys.exit(0)
    for name, fn in list(globals().items()):
        if name.startswith("test_"):
            fn()
            print("ok", name)
