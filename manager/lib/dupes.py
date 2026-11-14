#!/usr/bin/env python3
"""manager dupes: find redundant copies. Prints TSV lines: kind<TAB>bytes<TAB>path<TAB>why

kinds:
  extracted  an archive whose contents already exist unpacked next to it (or in a sibling folder)
  copy       "name (1).ext", "name copy.ext", *.bak / *.orig / *~ that are byte-identical to the original
  same       big files with identical content elsewhere (the newest copies are listed; the oldest is kept)
"""
import hashlib, os, sys, tarfile, zipfile, re
from collections import defaultdict

PRUNE = {"node_modules", ".git", ".hg", "venv", ".venv", "__pycache__", ".godot", ".gradle", ".cache",
         ".wrangler", ".dragonruby", ".dl", ".trash", "build", "dist", "obj", "out", "export", "target", "drive_c"}
BIG = int(os.environ.get("MGR_DUPES_MIN_MB", "5")) * 1024 * 1024
SKIP_ROOTS = [os.path.realpath(p) for p in os.environ.get("MGR_DUPES_SKIP", "").split(":") if p]
# installed apps / SDKs (manager.conf IGNORE): identical files there are normal, so only look for
# unpacked archives and "(1)" copies in them, not for same-content files
import fnmatch
NO_SAME = [p for p in os.environ.get("MGR_DUPES_NO_SAME", "").split(":") if p]
BUILD_OUT = re.compile(r"/(bin/(Debug|Release)|Content/bin)/")
def no_same(p):
    return BUILD_OUT.search(p) or any(fnmatch.fnmatch(p + "/", pat + "/*") for pat in NO_SAME)

def walk(roots):
    seen = set()
    for root in roots:
        for d, dirs, files in os.walk(root):
            rd = os.path.realpath(d)
            if rd in seen or any(rd == s or rd.startswith(s + "/") for s in SKIP_ROOTS):
                dirs[:] = []; continue
            seen.add(rd)
            dirs[:] = [x for x in dirs if x not in PRUNE and not os.path.islink(os.path.join(d, x))]
            for f in files:
                p = os.path.join(d, f)
                if not os.path.islink(p):
                    yield p

def same(a, b):
    try:
        if os.path.getsize(a) != os.path.getsize(b): return False
        with open(a, "rb") as fa, open(b, "rb") as fb:
            while True:
                x, y = fa.read(1 << 20), fb.read(1 << 20)
                if x != y: return False
                if not x: return True
    except OSError:
        return False

def members(path):
    """(name, size) of regular files inside an archive, or None if unreadable/unsupported"""
    try:
        if path.endswith(".zip"):
            with zipfile.ZipFile(path) as z:
                return [(i.filename, i.file_size) for i in z.infolist() if not i.is_dir()]
        if re.search(r"\.(tar(\.(gz|xz|bz2|zst))?|tgz|txz)$", path):
            with tarfile.open(path) as t:
                return [(m.name, m.size) for m in t.getmembers() if m.isfile()]
    except Exception:
        return None
    return None

def extracted_at(path):
    """where (if anywhere) the archive's contents already exist with matching sizes"""
    ms = members(path)
    if not ms: return None
    ms = [(n.lstrip("./"), s) for n, s in ms if "__MACOSX" not in n]
    if not ms: return None
    here = os.path.dirname(path)
    stem = re.sub(r"( \(\d+\))?\.(zip|tar(\.\w+)?|tgz|txz)$", "", os.path.basename(path))
    bases = [here, os.path.join(here, stem), os.path.join(os.path.expanduser("~/AppImages")),
             os.path.join(os.path.expanduser("~/Applications"))]
    for base in bases:
        if not os.path.isdir(base): continue
        hit = sum(1 for n, s in ms if os.path.isfile(os.path.join(base, n)) and os.path.getsize(os.path.join(base, n)) == s)
        if hit >= max(1, int(len(ms) * 0.98)):
            return os.path.join(base, ms[0][0].split("/")[0]) if base == here else base
    return None

COPY_RE = re.compile(r"^(?P<stem>.+?)(?: \(\d+\)| copy| - Copy)(?P<ext>\.[^.]+)?$")
BAK_RE = re.compile(r"^(?P<orig>.+?)(?:\.bak(?:-[\w.-]+)?|\.orig|~)$")

def main(roots):
    out = []
    by_size = defaultdict(list)
    for p in walk(roots):
        b = os.path.basename(p)
        try: size = os.path.getsize(p)
        except OSError: continue
        # archives already unpacked
        if re.search(r"\.(zip|tar(\.(gz|xz|bz2|zst))?|tgz|txz)$", b):
            where = extracted_at(p)
            if where:
                out.append(("extracted", size, p, "already unpacked at " + where))
                continue
        # "x (1).ext", "x copy.ext"
        m = COPY_RE.match(b)
        if m:
            orig = os.path.join(os.path.dirname(p), m["stem"] + (m["ext"] or ""))
            if os.path.isfile(orig) and same(p, orig):
                out.append(("copy", size, p, "identical to " + os.path.basename(orig))); continue
        m = BAK_RE.match(b)
        if m and "/data/" not in p:   # apps keep their own .bak of state files in data/ — leave those
            orig = os.path.join(os.path.dirname(p), m["orig"])
            if os.path.isfile(orig) and same(p, orig):
                out.append(("copy", size, p, "identical to " + os.path.basename(orig))); continue
        if size >= BIG and not no_same(p):
            by_size[size].append(p)
    listed = {o[2] for o in out}
    for size, ps in by_size.items():
        ps = [p for p in ps if p not in listed]
        if len(ps) < 2: continue
        groups = defaultdict(list)
        for p in ps:
            h = hashlib.sha1()
            try:
                with open(p, "rb") as f:
                    for chunk in iter(lambda: f.read(1 << 20), b""): h.update(chunk)
            except OSError: continue
            groups[h.hexdigest()].append(p)
        for g in groups.values():
            if len(g) < 2: continue
            g.sort(key=lambda p: (os.path.getmtime(p), len(p)))
            for p in g[1:]:
                out.append(("same", size, p, "same content as " + g[0]))
    for kind, size, p, why in out:
        print(f"{kind}\t{size}\t{p}\t{why}")

if __name__ == "__main__":
    main([os.path.expanduser(a) for a in sys.argv[1:]] or [os.path.expanduser("~/an")])
