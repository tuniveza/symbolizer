#!/usr/bin/env python3
"""manager index — gather what manager gui shows: every project's name, facts, pictures and videos.

  manager index            refresh now (only projects that changed since last time are looked at again)
  manager index --if-due   refresh only if the last refresh is 3 days old (what the timer runs every day)
  manager index --force    look at every project again and remake every thumbnail

Pictures and videos come from, in order:
  1. the Application Organizer (~/an/ca/ap/heat-map), which has hand-picked media for many projects
  2. screenshots, previews and videos found inside the project folder
  3. for web projects with nothing else, a screenshot of the page taken by headless Chromium

Everything lands in ~/.cache/an-manager/gui: data.json (what the page reads) and t/ (small webp thumbnails
and short muted mp4 previews). Thumbnails are named after the source file's path, size and time,
so nothing is ever made twice. Runs at idle priority; one run at a time.
"""
import fcntl
import hashlib
import http.server
import json
import os
import re
import shutil
import socket
import subprocess
import sys
import tempfile
import threading
import time
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))
import capture  # noqa: E402

HOME = Path.home()
AN = HOME / "an"
MANAGER = Path(os.environ.get("AN_MANAGER", Path(__file__).resolve().parent.parent))
CACHE = HOME / ".cache" / "an-manager" / "gui"
THUMBS = CACHE / "t"
DATA = CACHE / "data.json"
STATE = CACHE / "state.json"
ORGANIZER = AN / "ca" / "ap" / "heat-map" / "index.html"
DUE_AFTER = 3 * 24 * 3600
VERSION = 11                                  # bump to remake every entry

PRUNE = {"node_modules", ".git", ".hg", "venv", ".venv", "__pycache__", ".godot", ".gradle", ".cache",
         ".wrangler", ".idea", ".vscode", "build", "out", "obj", "target", ".dragonruby", ".manager-trash",
         "vendor", "vendored", "deps", "third_party", "external", "Libs", "bin", "dist", ".svelte-kit"}
IMG = {".png", ".jpg", ".jpeg", ".webp", ".gif"}
VID = {".mp4", ".webm", ".mov", ".mkv"}
LANGS = {".c": "C", ".h": "C", ".cpp": "C++", ".hpp": "C++", ".cc": "C++", ".py": "Python", ".ts": "TypeScript",
         ".tsx": "TypeScript", ".js": "JavaScript", ".mjs": "JavaScript", ".html": "HTML", ".css": "CSS",
         ".lua": "Lua", ".rb": "Ruby", ".hx": "Haxe", ".cs": "C#", ".java": "Java", ".kt": "Kotlin", ".odin": "Odin",
         ".cr": "Crystal", ".gleam": "Gleam", ".v": "V", ".asm": "Assembly", ".s": "Assembly", ".gd": "GDScript",
         ".glsl": "GLSL", ".frag": "GLSL", ".vert": "GLSL", ".wgsl": "WGSL", ".gml": "GML", ".go": "Go",
         ".rs": "Rust", ".brick": "brick", ".sh": "Shell"}
GOOD_NAME = re.compile(r"screen|shot|preview|cover|demo|capture|promo|banner|hero|title|showcase|gameplay|thumb|poster", re.I)
GOOD_DIR = {"screenshots", "screenshot", "media", "promo", "docs", ".itch", "img", "images", "press"}
ART_DIR = {"art", "sprites", "sprite", "tiles", "tilesets", "tileset", "fonts", "font", "icons", "textures",
           "texture", "assets", "content", "res", "sfx", "audio", "music", "shader", "shaders", "dataset", "datafiles"}
# folders that hold downloaded art, engine files or vendored docs, never pictures of the project itself
JUNK_DIR = re.compile(r"pack|asset|sprite|tile|kenney|premium|texture|model|sample|example|vendor|font|icon|sound|"
                      r"sfx|music|anim|^ui$|^doc$|^\d+$|metadata|template|addons|plugins|thirdparty|\bkit\b", re.I)
JUNK_NAME = re.compile(r"^(icon|logo|favicon|splash|default|controller)|dragonruby|godot|unity|texture|pack|anim|"
                       r"example|sample|low_poly|lowpoly|sprite|tileset", re.I)
MAX_WALK = 25000                             # files looked at per project, at most
MAX_IMG, MAX_VID = 6, 2
SCREEN = capture.Screen()                    # the invisible screen projects are recorded on (Xvfb)


def log(*a):
    print(*a, flush=True)


def save_json(path, obj):
    tmp = path.with_suffix(".tmp")
    tmp.write_text(json.dumps(obj, separators=(",", ":")))
    tmp.replace(path)


def set_state(**kw):
    try:
        st = json.loads(STATE.read_text())
    except (OSError, ValueError):
        st = {}
    st.update(kw)
    save_json(STATE, st)


def run(cmd, timeout=120, **kw):
    try:
        return subprocess.run(cmd, capture_output=True, text=True, errors="replace", timeout=timeout, **kw)
    except (subprocess.TimeoutExpired, OSError):
        return None


# ---- projects ---------------------------------------------------------------------------------

def mgr_projects():
    r = run([str(MANAGER / "bin" / "manager"), "projects", "--tsv"], timeout=300)
    out = []
    for line in (r.stdout if r else "").splitlines():
        f = line.split("\t")
        if len(f) >= 6:
            out.append(dict(path=f[0], type=f[1], name=f[2], about=f[3], trail=f[4], editor=f[5]))
    return out


def organizer_media():
    """path -> [media] from the Application Organizer page (its media is already web-sized)."""
    try:
        html = ORGANIZER.read_text()
        start = html.index('<script id="data" type="application/json">') + len('<script id="data" type="application/json">')
        apps, _ = json.JSONDecoder().raw_decode(html[start:])
    except (OSError, ValueError):
        return {}
    base = ORGANIZER.parent
    out = {}
    for a in apps:
        p = os.path.expanduser(a.get("path", ""))
        media = []
        for m in a.get("media", []):
            src = base / m["src"]
            if not src.is_file():
                continue
            thumb = base / (m.get("thumb") or m.get("poster") or m["src"])
            media.append({"kind": "video" if m.get("type") == "video" else "img", "src": str(src),
                          "thumb": str(thumb if thumb.is_file() else src), "caption": m.get("caption", ""),
                          "from": "organizer"})
        if media and "*" not in p:
            out[p.rstrip("/")] = {"media": media, "what": a.get("what", ""), "status": a.get("status", "")}
    return out


def walk(root, nested):
    """newest mtime, files, bytes, language counts and media candidates, skipping nested projects."""
    newest, files, size = 0.0, 0, 0
    langs, media = {}, []
    stack = [root]
    while stack and files < MAX_WALK:
        d = stack.pop()
        try:
            it = os.scandir(d)
        except OSError:
            continue
        with it:
            for e in it:
                try:
                    if e.is_dir(follow_symlinks=False):
                        if e.name not in PRUNE and e.path not in nested:
                            stack.append(e.path)
                        continue
                    if not e.is_file(follow_symlinks=False):
                        continue
                    st = e.stat(follow_symlinks=False)
                except OSError:
                    continue
                files += 1
                size += st.st_size
                newest = max(newest, st.st_mtime)
                ext = os.path.splitext(e.name)[1].lower()
                if ext in LANGS:
                    langs[LANGS[ext]] = langs.get(LANGS[ext], 0) + 1
                if ext in IMG or ext in VID:
                    media.append((e.path, ext, st.st_size, st.st_mtime_ns))
    return newest, files, size, langs, media


def wanted(path, root):
    """only things that look like pictures of the project: named like a screenshot, kept in a
    screenshots/media/docs folder, or sitting near the top, and never inside downloaded art."""
    rel = Path(path).relative_to(root)
    dirs = rel.parts[:-1]
    if any(JUNK_DIR.search(d) for d in dirs) or {d.lower() for d in dirs} & ART_DIR:
        return False
    if JUNK_NAME.search(rel.name) and not GOOD_NAME.search(rel.name):
        return False
    if GOOD_NAME.search(rel.name) or {d.lower() for d in dirs} & GOOD_DIR:
        return True
    return len(dirs) <= 1 and not JUNK_NAME.search(rel.name)


def stem(path):
    """frames of one animation (walk-000.png, walk 1-005.png …) count once."""
    return re.sub(r"[\d_\- .]+(?=\.[^.]+$)", "", path.lower())


def score(path, root, ext, size):
    rel = Path(path).relative_to(root)
    parts = {p.lower() for p in rel.parts[:-1]}
    s = 0
    if GOOD_NAME.search(rel.name):
        s += 50
    if parts & GOOD_DIR:
        s += 20
    if parts & ART_DIR:
        s -= 45
    if ext in VID:
        s += 15
    if ext in IMG and size < 25_000:
        s -= 40                               # icons and sprites
    s -= 4 * (len(rel.parts) - 1)             # shallower is likelier to be a showcase
    return s


def image_size(path):
    """width, height from the file header (png, gif, jpeg, webp), without decoding the picture."""
    try:
        with open(path, "rb") as f:
            head = f.read(64)
            if head[:8] == b"\x89PNG\r\n\x1a\n":
                return int.from_bytes(head[16:20], "big"), int.from_bytes(head[20:24], "big")
            if head[:6] in (b"GIF87a", b"GIF89a"):
                return int.from_bytes(head[6:8], "little"), int.from_bytes(head[8:10], "little")
            if head[:4] == b"RIFF" and head[8:12] == b"WEBP":
                c = head[12:16]
                if c == b"VP8X":
                    return int.from_bytes(head[24:27], "little") + 1, int.from_bytes(head[27:30], "little") + 1
                if c == b"VP8L":
                    b = int.from_bytes(head[21:25], "little")
                    return (b & 0x3FFF) + 1, ((b >> 14) & 0x3FFF) + 1
                if c == b"VP8 ":
                    return int.from_bytes(head[26:28], "little") & 0x3FFF, int.from_bytes(head[28:30], "little") & 0x3FFF
            if head[:2] == b"\xff\xd8":                      # jpeg: walk the markers to the frame header
                f.seek(2)
                while True:
                    m = f.read(4)
                    if len(m) < 4 or m[0] != 0xFF:
                        break
                    n = int.from_bytes(m[2:4], "big")
                    if m[1] in (0xC0, 0xC1, 0xC2, 0xC3, 0xC5, 0xC6, 0xC7, 0xC9, 0xCA, 0xCB, 0xCD, 0xCE, 0xCF):
                        sof = f.read(5)
                        return int.from_bytes(sof[3:5], "big"), int.from_bytes(sof[1:3], "big")
                    f.seek(n - 2, 1)
    except OSError:
        pass
    return 0, 0


def pick_media(root, cands, room_img=MAX_IMG, room_vid=MAX_VID):
    imgs, vids, probed, stems = [], [], 0, set()
    cands = [c for c in cands if wanted(c[0], root)]
    for path, ext, size, mt in sorted(cands, key=lambda c: -score(c[0], root, c[1], c[2])):
        sc = score(path, root, ext, size)
        if stem(path) in stems:
            continue
        if ext in IMG and (probed >= 200 or sc <= -20):
            continue
        probed += ext in IMG
        if ext in VID:
            if len(vids) < room_vid and 50_000 < size < 2_000_000_000:
                vids.append((path, ext, size, mt)); stems.add(stem(path))
        elif len(imgs) < room_img and sc > -20:
            w, h = image_size(path)
            # a real picture of the thing, not a sprite sheet strip or a tiny tile
            if w >= 480 and h >= 270 and 0.4 < w / max(h, 1) < 3.2:
                imgs.append((path, ext, size, mt)); stems.add(stem(path))
        if len(imgs) >= room_img and len(vids) >= room_vid:
            break
    return vids + imgs


# ---- thumbnails -------------------------------------------------------------------------------

def key(path, size, mt):
    return hashlib.sha1(f"{path}|{size}|{mt}|{VERSION}".encode()).hexdigest()[:16]


def thumb_image(src, k, used):
    out = THUMBS / f"{k}.webp"
    used.add(out.name)
    if not out.exists():
        r = run(["vipsthumbnail", src, "--size", "720x450", "-o", f"{out}[Q=74,strip]"], timeout=60)
        if not (r and r.returncode == 0 and out.exists()):
            r = run(["magick", src + "[0]", "-thumbnail", "720x450", "-quality", "74", str(out)], timeout=60)
    return out.name if out.exists() else None


def big_image(src, ext, size, k, used):
    """the full view: the original when the browser shows it well, else a 1920px webp."""
    if ext in {".png", ".jpg", ".jpeg", ".webp", ".gif"} and size < 3_000_000:
        return None
    out = THUMBS / f"{k}.big.webp"
    used.add(out.name)
    if not out.exists():
        run(["vipsthumbnail", src, "--size", "1920x1200>", "-o", f"{out}[Q=82,strip]"], timeout=90)
    return out.name if out.exists() else None


def ffmpeg(args, timeout=300):
    return run(["ffmpeg", "-nostdin", "-hide_banner", "-loglevel", "error", "-y", "-threads", "2", *args], timeout=timeout)


def video_bits(src, ext, size, k, used):
    """poster (webp), hover preview (short muted mp4) and, for mov/mkv, a playable mp4."""
    poster, prev, play = THUMBS / f"{k}.webp", THUMBS / f"{k}.p.mp4", None
    used.update({poster.name, prev.name})
    if not poster.exists():
        ffmpeg(["-ss", "1", "-i", src, "-frames:v", "1", "-vf", "scale=720:-2", "-quality", "74", str(poster)], 60)
        if not poster.exists():
            ffmpeg(["-i", src, "-frames:v", "1", "-vf", "scale=720:-2", str(poster)], 60)
    if not prev.exists():
        ffmpeg(["-ss", "0.5", "-t", "6", "-i", src, "-an", "-vf", "scale=640:-2,fps=24", "-c:v", "libx264",
                "-preset", "veryfast", "-crf", "31", "-pix_fmt", "yuv420p", "-movflags", "+faststart", str(prev)])
    if ext in {".mov", ".mkv"}:
        p = THUMBS / f"{k}.full.mp4"
        used.add(p.name)
        if not p.exists():
            ffmpeg(["-i", src, "-t", "180", "-vf", "scale='min(1280,iw)':-2", "-c:v", "libx264", "-preset", "veryfast",
                    "-crf", "26", "-pix_fmt", "yuv420p", "-c:a", "aac", "-b:a", "96k", "-movflags", "+faststart", str(p)], 900)
        play = p.name if p.exists() else None
    return (poster.name if poster.exists() else None), (prev.name if prev.exists() else None), play


# ---- screenshots of web projects -------------------------------------------------------------

def web_root(p):
    d = Path(p["path"])
    if p["type"] == "web" and (d / "index.html").exists():
        return d
    if p["type"] == "js-vite" and (d / "dist" / "index.html").exists():
        return d / "dist"
    if p["type"] in ("js-worker", "js-node") and (d / "public" / "index.html").exists():
        return d / "public"                       # the static page only; the worker itself is never started
    return None


class Quiet(http.server.SimpleHTTPRequestHandler):
    extensions_map = {**http.server.SimpleHTTPRequestHandler.extensions_map,
                      ".html": "text/html; charset=utf-8", ".js": "text/javascript; charset=utf-8",
                      ".mjs": "text/javascript; charset=utf-8", ".css": "text/css; charset=utf-8",
                      ".json": "application/json", ".wasm": "application/wasm"}

    def log_message(self, *a):
        pass


def screenshot(p, k, used):
    root = web_root(p)
    browser = shutil.which("chromium") or shutil.which("google-chrome-stable")
    if not root or not browser:
        return None
    out = THUMBS / f"{k}.shot.webp"
    used.add(out.name)
    if out.exists():
        return out.name
    handler = lambda *a, **kw: Quiet(*a, directory=str(root), **kw)
    srv = http.server.ThreadingHTTPServer(("127.0.0.1", 0), handler)
    threading.Thread(target=srv.serve_forever, daemon=True).start()
    try:
        with tempfile.TemporaryDirectory(prefix="manager-shot-") as tmp:
            png = Path(tmp) / "shot.png"
            run([browser, "--headless=new", "--use-angle=swiftshader", "--enable-unsafe-swiftshader", "--hide-scrollbars", "--mute-audio", "--no-first-run",
                 "--disable-extensions", "--force-device-scale-factor=1", "--window-size=1280,800",
                 "--virtual-time-budget=5000", f"--user-data-dir={tmp}/profile", f"--screenshot={png}",
                 f"http://127.0.0.1:{srv.server_address[1]}/"], timeout=45)
            # a page that is one flat colour (still loading, or needs a click) says nothing: leave it out
            dev = run(["magick", str(png), "-colorspace", "gray", "-format", "%[fx:standard_deviation]", "info:"], timeout=30)
            flat = dev is not None and dev.returncode == 0 and float(dev.stdout or 0) < 0.03
            if png.exists() and not flat:
                run(["vipsthumbnail", str(png), "--size", "1280x800", "-o", f"{out}[Q=80,strip]"], timeout=30)
    finally:
        srv.shutdown()
        srv.server_close()
    return out.name if out.exists() else None


# ---- one project ------------------------------------------------------------------------------

def git_info(d):
    if not (Path(d) / ".git").exists():
        return None
    r = run(["git", "-C", d, "log", "-1", "--format=%ct%x09%s"], timeout=15)
    if not r or r.returncode or "\t" not in r.stdout:
        return None
    t, subject = r.stdout.strip().split("\t", 1)
    return {"time": int(t), "subject": subject[:140]}


def describe(p, nested, curated, old, force, used):
    d = p["path"]
    newest, files, size, langs, cands = walk(d, nested)
    git = git_info(d)
    finger = (f"{VERSION}|{newest:.0f}|{files}|{size}|{(git or {}).get('time')}|{len(curated.get('media', []))}"
              f"|{'x' if SCREEN.available() else '-'}")
    if not force and old and old.get("finger") == finger:
        for m in old.get("media", []):           # keep its thumbnails alive
            used.update(v for kk, v in m.items() if kk in ("thumb", "preview", "big", "play") and v and "/" not in v)
        old.update({k: p[k] for k in ("name", "about", "trail", "editor", "type")})
        return old, False

    media = []
    for m in curated.get("media", []):
        media.append({"kind": m["kind"], "src": m["src"], "thumbSrc": m["thumb"], "caption": m["caption"], "from": "organizer"})
    # hand-picked media wins; found media only fills up what's missing
    own = [] if Path(d) == ORGANIZER.parent or len(media) >= 3 else cands   # the organizer's m/ is other projects'
    room_img = max(0, MAX_IMG - sum(m["kind"] == "img" for m in media))
    room_vid = max(0, MAX_VID - sum(m["kind"] == "video" for m in media))
    for path, ext, sz, mt in pick_media(Path(d), own, room_img, room_vid):
        if any(m.get("src") == path for m in media):
            continue
        k = key(path, sz, mt)
        rel = os.path.relpath(path, d)
        if ext in VID:
            poster, prev, play = video_bits(path, ext, sz, k, used)
            if poster:
                media.append({"kind": "video", "src": path, "thumb": poster, "preview": prev, "play": play, "caption": rel})
        else:
            t = thumb_image(path, k, used)
            if t:
                media.append({"kind": "img", "src": path, "thumb": t, "big": big_image(path, ext, sz, k, used), "caption": rel})
    # every project gets a picture and a clip: record it running, else show its code
    k = key(d, size, int(newest))
    need = lambda kind: not any(m["kind"] == kind for m in media)
    if need("img") or need("video"):
        got = capture.run_capture(p, web_root, SCREEN, THUMBS, k, used) if SCREEN.available() else []
        if not got and need("img"):
            shot = screenshot(p, k, used)             # no Xvfb: at least a still of web pages
            if shot:
                got = [{"kind": "img", "thumb": shot, "big": shot, "caption": "the page, as it opens", "from": "screenshot"}]
        media += [m for m in got if need(m["kind"])]
    if need("img") or need("video"):
        media += [m for m in capture.code_tour(p, THUMBS, k + "c", used) if need(m["kind"])]
    for m in media:                                   # backdrop colour for pictures that don't fill the frame
        if "tone" not in m:
            m["tone"] = capture.tone(THUMBS / m["thumb"] if m.get("thumb") else m.get("thumbSrc") or m.get("src"))

    entry = dict(p)
    entry.update({
        "id": hashlib.sha1(d.encode()).hexdigest()[:10],
        "finger": finger, "modified": int(newest), "files": files, "bytes": size,
        "langs": [k for k, _ in sorted(langs.items(), key=lambda kv: -kv[1])[:4]],
        "git": git, "media": media,
        "what": curated.get("what", ""), "status": curated.get("status", ""),
        "reload": os.access(os.path.join(d, "reload.sh"), os.X_OK),
    })
    return entry, True


def main(argv):
    force = "--force" in argv
    CACHE.mkdir(parents=True, exist_ok=True)
    THUMBS.mkdir(exist_ok=True)
    lock = open(CACHE / ".lock", "w")
    try:
        fcntl.flock(lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
    except OSError:
        log("manager index is already running")
        return 0
    try:
        prev = json.loads(DATA.read_text())
    except (OSError, ValueError):
        prev = {}
    if "--if-due" in argv and time.time() - prev.get("generated", 0) < DUE_AFTER:
        log("manager index: last refresh was less than 3 days ago, nothing to do")
        return 0

    started = time.time()
    if force:                                            # remake every thumbnail too
        shutil.rmtree(THUMBS, ignore_errors=True)
        THUMBS.mkdir()
    set_state(running=True, started=int(started), done=0, total=0, current="finding projects", error=None)
    try:
        projects = mgr_projects()
        paths = {p["path"] for p in projects}
        curated = organizer_media()
        # organizer media filed under a folder inside a project (pj/ge/ct/BMG) belongs to that project
        for cp in sorted(curated, key=len):
            if cp in paths:
                continue
            owner = max((q for q in paths if cp.startswith(q + "/")), key=len, default=None)
            if owner and owner not in curated:
                curated[owner] = curated[cp]
        olds = {e["path"]: e for e in prev.get("projects", [])}
        used, out, changed = set(), [], 0
        set_state(total=len(projects))
        for i, p in enumerate(projects):
            set_state(done=i, current=p["name"])
            nested = {q for q in paths if q != p["path"] and q.startswith(p["path"] + "/")}
            try:
                e, did = describe(p, nested, curated.get(p["path"], {}), olds.get(p["path"]), force, used)
            except Exception as ex:                      # one odd project never stops the rest
                log(f"  !! {p['path']}: {ex}")
                e, did = dict(p, id=hashlib.sha1(p["path"].encode()).hexdigest()[:10], media=[], finger=""), True
            changed += did
            out.append(e)
            if did:
                log(f"  {'ok':4} {p['name']}  ({len(e.get('media', []))} media)")
        for f in THUMBS.iterdir():                       # thumbnails nothing points at any more
            if f.name not in used:
                f.unlink(missing_ok=True)
        save_json(DATA, {"generated": int(time.time()), "took": round(time.time() - started, 1), "projects": out})
        set_state(running=False, done=len(projects), current="", finished=int(time.time()))
        log(f"manager index: {len(out)} projects, {changed} looked at again, {time.time() - started:.1f}s")
    except Exception as ex:
        set_state(running=False, error=str(ex))
        raise
    finally:
        SCREEN.stop()
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
