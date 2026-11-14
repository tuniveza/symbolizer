"""capture — a picture and a clip for every project that has none, made by manager index.

Two ways, best first:
  run    the project runs on an invisible screen (Xvfb, 1280x800) and 7 seconds of it are recorded:
         windowed programs as they are, terminal programs inside a kitty window, web pages in Chromium.
         Sound is muted and nothing appears on the real desktop. Nova projects (pj/ns) are never run,
         only their static pages are shown. A `.no-capture` file in a project stops it being run.
  code   the project's main source file, highlighted in Emberline colours: a picture of the top of it
         and a clip scrolling slowly through it. Used when a project can't be run (or Xvfb is missing).

Every result is checked: a frame that is one flat colour (nothing drawn) is thrown away.
"""
import html
import os
import re
import shutil
import signal
import subprocess
import tempfile
import threading
import time
from pathlib import Path

W, H = 1280, 800
CW, CH = 960, 600                            # code clips are drawn at exactly this size, so text stays sharp
RECORD = 7                                   # seconds recorded per project
NEVER_RUN = ("/an/pj/ns/",                   # Nova: talks to live services; static pages only
             "/an/co/ar/ps1/")               # runs DuckStation, which only shows its first-run wizard
# a terminal program that only printed one of these didn't really run here (no GPU, no WebGPU…): show its code
FAILED = re.compile(r"failed|error|traceback|segmentation fault|core dumped|not found|cannot open|unable to|panic", re.I)
RUN_TYPES = {"make", "cmake", "c", "cpp", "nasm", "odin", "crystal", "love", "lua", "lime", "dotnet", "gradle",
             "dragonruby", "py", "godot", "v", "gleam", "go", "rust"}
SRC_EXT = {".c", ".h", ".cpp", ".hpp", ".cc", ".py", ".ts", ".tsx", ".js", ".mjs", ".html", ".lua", ".rb", ".hx",
           ".cs", ".java", ".kt", ".odin", ".cr", ".gleam", ".v", ".asm", ".s", ".gd", ".gml", ".go", ".rs", ".brick"}
MAIN_NAMES = ("main", "game", "app", "index", "brick", "program", "lt", "bitstream")

# the Emberline palette (~/.config/hypr/emberline/colors.lua)
BG, BG_ALT, SURFACE, ASH, DIM, BURNT, BASE, ORANGE, GOLD = ("#161616", "#1c1c1c", "#2e150c", "#5e2c12", "#8e3c1a",
                                                           "#d84a00", "#ff5414", "#ff6600", "#ff8a1a")


def _run(cmd, timeout=60, **kw):
    try:
        return subprocess.run(cmd, capture_output=True, text=True, errors="replace", timeout=timeout, **kw)
    except (subprocess.TimeoutExpired, OSError):
        return None


def flat(png, limit=0.03):
    """True when a picture is (nearly) one colour, i.e. nothing was drawn."""
    r = _run(["magick", str(png), "-colorspace", "gray", "-format", "%[fx:standard_deviation]", "info:"], 30)
    try:
        return float(r.stdout) < limit
    except (AttributeError, ValueError):
        return True


def tone(img):
    """the picture's average colour, darkened: the backdrop behind it when it doesn't fill the frame."""
    r = _run(["magick", str(img) + "[0]", "-resize", "1x1!", "-modulate", "38,90", "-format",
              "#%[hex:u.p{0,0}]", "info:"], 20)
    out = (r.stdout.strip() if r and r.returncode == 0 else "")[:7]
    return out if re.fullmatch(r"#[0-9A-Fa-f]{6}", out) else None


def encode(src, out, width, seconds=None, start=0.0, crf=30):
    args = ["ffmpeg", "-nostdin", "-hide_banner", "-loglevel", "error", "-y", "-threads", "2"]
    if start:
        args += ["-ss", str(start)]
    args += ["-i", str(src)]
    if seconds:
        args += ["-t", str(seconds)]
    args += ["-an", "-vf", f"scale={width}:-2", "-c:v", "libx264", "-preset", "veryfast", "-crf", str(crf),
             "-pix_fmt", "yuv420p", "-movflags", "+faststart", str(out)]
    r = _run(args, 300)
    return r is not None and r.returncode == 0 and Path(out).exists()


def finish(raw_mp4, shot_png, thumbs, k, used, label, final=False):
    """turn a recording + screenshot into: thumbnail, full picture, hover preview, playable clip.
    final=True: the recording is already web-ready and becomes the playable clip as it is."""
    names = {n: f"{k}.{n}" for n in ("cap.webp", "cap.big.webp", "cap.p.mp4", "cap.full.mp4")}
    used.update(names.values())
    _run(["vipsthumbnail", str(shot_png), "--size", "720x450", "-o", f"{thumbs / names['cap.webp']}[Q=74,strip]"], 30)
    _run(["vipsthumbnail", str(shot_png), "--size", f"{W}x{H}", "-o", f"{thumbs / names['cap.big.webp']}[Q=82,strip]"], 30)
    encode(raw_mp4, thumbs / names["cap.p.mp4"], 640, seconds=6, crf=31)
    if final:
        shutil.copyfile(raw_mp4, thumbs / names["cap.full.mp4"])
    else:
        encode(raw_mp4, thumbs / names["cap.full.mp4"], W, crf=28)
    if not (thumbs / names["cap.webp"]).exists():
        return []
    t = tone(shot_png)
    img = {"kind": "img", "thumb": names["cap.webp"], "big": names["cap.big.webp"], "caption": label, "from": "capture", "tone": t}
    out = [img]
    if (thumbs / names["cap.full.mp4"]).exists():
        out.insert(0, {"kind": "video", "thumb": names["cap.webp"], "preview": names["cap.p.mp4"] if (thumbs / names["cap.p.mp4"]).exists() else None,
                       "play": names["cap.full.mp4"], "caption": label + " (recorded)", "from": "capture", "tone": t})
    return out


# ---- code pictures + scrolling clips ------------------------------------------------------------

def main_file(d, ptype, deep=False):
    """the file that best shows what the project is."""
    d = Path(d)
    try:
        m = re.search(r'^MAIN="?([^"\s$]+)', (d / "make.sh").read_text(), re.M)
        if m and (d / m.group(1)).is_file():
            return d / m.group(1)
    except OSError:
        pass
    found = []
    skip = {"node_modules", ".git", "build", "dist", "out", "obj", "bin", "vendor", "deps", "third_party", "Libs",
            ".godot", "__pycache__", "venv", ".venv", "addons", "test", "tests", "old"} | (set() if deep else {"lib"})
    for root, dirs, files in os.walk(d):
        dirs[:] = [x for x in dirs if x not in skip and not x.startswith(".")]
        depth = len(Path(root).relative_to(d).parts)
        if depth > (7 if deep else 3):
            dirs[:] = []
        for f in files:
            p = Path(root) / f
            if p.suffix.lower() in SRC_EXT:
                try:
                    size = p.stat().st_size
                except OSError:
                    continue
                stem = p.stem.lower()
                score = (40 if stem in MAIN_NAMES else 0) - 6 * depth + min(size, 40_000) / 2000
                if size > 300_000 or p.suffix == ".html" and ptype not in ("web", "js-vite"):
                    score -= 30
                found.append((score, p))
    if not found and not deep:
        return main_file(d, ptype, deep=True)
    return max(found, key=lambda f: f[0])[1] if found else None


def code_html(path, title, lines=240):
    from pygments import highlight
    from pygments.formatters import HtmlFormatter
    from pygments.lexers import TextLexer, guess_lexer_for_filename
    from pygments.style import Style
    from pygments.token import Comment, Error, Keyword, Name, Number, Operator, Punctuation, String, Text, Token

    class Ember(Style):
        background_color = BG
        styles = {Token: BASE, Text: BASE, Comment: f"italic {DIM}", Keyword: f"bold {GOLD}", Keyword.Type: ORANGE,
                  Name.Function: GOLD, Name.Class: f"bold {GOLD}", Name.Builtin: ORANGE, Name.Decorator: ORANGE,
                  String: ORANGE, Number: GOLD, Operator: BURNT, Punctuation: BURNT, Error: BASE}

    text = path.read_text(errors="replace").replace("\t", "    ")
    text = "\n".join(text.splitlines()[:lines])
    try:
        lexer = guess_lexer_for_filename(path.name, text)
    except Exception:
        lexer = TextLexer()
    body = highlight(text, lexer, HtmlFormatter(style=Ember, linenos="inline", nowrap=False))
    css = HtmlFormatter(style=Ember).get_style_defs(".highlight")
    return f"""<!doctype html><meta charset="utf-8"><style>
{css}
html,body{{margin:0;background:{BG};}}
body{{font:15px/1.5 "Agave Nerd Font Mono","Agave",monospace;color:{BASE};}}
.bar{{display:flex;gap:14px;align-items:center;padding:11px 20px;background:{BG_ALT};border-bottom:1px solid {ASH};}}
.bar b{{color:{GOLD};font-size:16px}} .bar span{{color:{BURNT};font-size:13px}}
.dots i{{display:inline-block;width:11px;height:11px;border-radius:50%;margin-right:7px;background:{ASH}}}
.dots i:first-child{{background:{BURNT}}}
.highlight{{padding:10px 20px 30px;}} .highlight pre{{margin:0;white-space:pre;}}
.highlight .linenos{{color:{ASH};padding-right:18px;}}
</style><div class="bar"><span class="dots"><i></i><i></i><i></i></span><b>{html.escape(title)}</b><span>{html.escape(path.name)}</span></div>{body}"""


def code_tour(p, thumbs, k, used):
    """a picture of the main source file and a clip scrolling through it."""
    browser = shutil.which("chromium") or shutil.which("google-chrome-stable")
    src = main_file(p["path"], p["type"])
    if not browser or not src:
        return []
    try:
        page = code_html(src, p["name"])
    except Exception:
        return []
    n_lines = min(240, len(src.read_text(errors="replace").splitlines()))
    height = max(CH, min(5600, 46 + 23 * n_lines + 60))
    with tempfile.TemporaryDirectory(prefix="manager-code-") as tmp:
        tmp = Path(tmp)
        (tmp / "code.html").write_text(page)
        tall = tmp / "tall.png"
        _run([browser, "--headless=new", "--disable-gpu", "--hide-scrollbars", "--force-device-scale-factor=1",
              f"--window-size={CW},{height}", f"--user-data-dir={tmp}/profile", f"--screenshot={tall}",
              (tmp / "code.html").as_uri()], 60)
        if not tall.exists():
            return []
        shot = tmp / "shot.png"
        _run(["magick", str(tall), "-crop", f"{CW}x{CH}+0+0", "+repage", str(shot)], 30)
        raw = tmp / "raw.mp4"
        # page by page: each screenful holds still (sharp, and costs almost nothing to store),
        # then glides to the next one
        travel, fps, hold, glide = height - CH, 20, 1.5, 0.45
        if travel > 40:
            pages = min(5, -(-travel // 480))
            step = travel / pages
            period = hold + glide
            secs = pages * period + hold
            f = f"clip((t-floor(t/{period})*{period}-{hold})/{glide},0,1)"
            y = f"'min({travel:.0f},(floor(t/{period})+{f}*{f}*(3-2*{f}))*{step:.1f})'"
        else:
            secs, y = 4, "0"
        # the picture is decoded once and repeated by the loop filter (not re-read every frame)
        vf = f"loop=loop=-1:size=1:start=0,setpts=N/{fps}/TB,crop={CW}:{CH}:0:{y}"
        _run(["ffmpeg", "-nostdin", "-hide_banner", "-loglevel", "error", "-y", "-threads", "2", "-i", str(tall),
              "-vf", vf, "-frames:v", str(int(secs * fps)), "-r", str(fps), "-c:v", "libx264", "-preset", "veryfast",
              "-crf", "26", "-tune", "stillimage", "-pix_fmt", "yuv420p", "-movflags", "+faststart", str(raw)], 300)
        if not raw.exists():
            return []
        out = finish(raw, shot, thumbs, k, used, f"the code: {src.relative_to(p['path'])}", final=True)
        for m in out:
            m["from"] = "code"
            m["caption"] = m["caption"].replace(" (recorded)", " (scrolling)")
        return out


# ---- running projects on an invisible screen ---------------------------------------------------

class Screen:
    """one Xvfb for the whole index run, started only if something needs it."""

    def __init__(self):
        self.proc, self.display, self.shims = None, None, None

    def available(self):
        return shutil.which("Xvfb") is not None

    def start(self):
        if self.proc:
            return True
        if not self.available():
            return False
        n = next(i for i in range(91, 200) if not Path(f"/tmp/.X11-unix/X{i}").exists() and not Path(f"/tmp/.X{i}-lock").exists())
        self.proc = subprocess.Popen(["Xvfb", f":{n}", "-screen", "0", f"{W}x{H}x24", "-nolisten", "tcp", "-noreset"],
                                     stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL, start_new_session=True)
        self.display = f":{n}"
        for _ in range(50):
            if Path(f"/tmp/.X11-unix/X{n}").exists():
                break
            time.sleep(0.1)
        # programs that would open the real browser or editor get a do-nothing stand-in
        self.shims = Path(tempfile.mkdtemp(prefix="manager-shims-"))
        for name in ("xdg-open", "emberline-helium", "helium", "codium", "code", "notify-send", "paplay", "aplay"):
            (self.shims / name).write_text("#!/bin/sh\nexit 0\n")
            (self.shims / name).chmod(0o755)
        return True

    def stop(self):
        if self.proc:
            os.killpg(self.proc.pid, signal.SIGTERM)
            self.proc.wait(timeout=10)
            self.proc = None
        if self.shims:
            shutil.rmtree(self.shims, ignore_errors=True)

    def env(self):
        e = {k: v for k, v in os.environ.items() if k not in ("WAYLAND_DISPLAY", "HYPRLAND_INSTANCE_SIGNATURE", "XDG_SESSION_TYPE")}
        e.update({"DISPLAY": self.display, "PATH": f"{self.shims}:{e.get('PATH', '/usr/bin:/bin')}",
                  "SDL_VIDEODRIVER": "x11", "SDL_AUDIODRIVER": "dummy", "ALSOFT_DRIVERS": "null",
                  "PULSE_SERVER": "unix:/nonexistent", "PIPEWIRE_REMOTE": "/nonexistent", "GDK_BACKEND": "x11",
                  "QT_QPA_PLATFORM": "xcb", "LIBGL_ALWAYS_SOFTWARE": "1", "BROWSER": "true", "XDG_SESSION_TYPE": "x11",
                  "WINEDLLOVERRIDES": "winepulse.drv,winealsa.drv=d", "WINEDEBUG": "-all", "AUDIODEV": "null"})
        return e

    def grab(self, out, seconds=None):
        args = ["ffmpeg", "-nostdin", "-hide_banner", "-loglevel", "error", "-y", "-f", "x11grab", "-draw_mouse", "0",
                "-video_size", f"{W}x{H}", "-framerate", "30", "-i", f"{self.display}.0"]
        if seconds:
            args += ["-t", str(seconds), "-c:v", "libx264", "-preset", "ultrafast", "-crf", "18", "-pix_fmt", "yuv420p", str(out)]
        else:
            args += ["-frames:v", "1", str(out)]
        return _run(args, (seconds or 2) + 20, env=self.env())


def _kill(proc):
    if proc and proc.poll() is None:
        try:
            os.killpg(proc.pid, signal.SIGTERM)
            proc.wait(timeout=3)
        except (ProcessLookupError, subprocess.TimeoutExpired):
            try:
                os.killpg(proc.pid, signal.SIGKILL)
            except ProcessLookupError:
                pass


def _record_while(screen, start_cmd, cwd, tmp, wait=14, limit=0.03, fit=None):
    """start something on the invisible screen, wait until it draws, record it, stop it."""
    proc = subprocess.Popen(start_cmd, cwd=cwd, env=screen.env(), stdin=subprocess.DEVNULL, stdout=subprocess.DEVNULL,
                            stderr=subprocess.DEVNULL, start_new_session=True)
    try:
        probe = tmp / "probe.png"
        drawn = False
        for _ in range(wait):
            time.sleep(1)
            probe.unlink(missing_ok=True)
            screen.grab(probe)
            if probe.exists() and not flat(probe, limit):
                drawn = True
                break
            if proc.poll() is not None:
                break
        if not drawn:
            return None
        time.sleep(0.8)                           # let the first frames settle
        raw = tmp / "raw.mp4"
        screen.grab(raw, RECORD)
        shot = tmp / "shot.png"
        _run(["ffmpeg", "-nostdin", "-loglevel", "error", "-y", "-sseof", "-2", "-i", str(raw), "-frames:v", "1", str(shot)], 30)
        if not shot.exists() or flat(shot, limit):
            return None
        return fit_to_window(raw, shot, tmp, **(fit or {}))
    finally:
        _kill(proc)


def fit_to_window(raw, shot, tmp, bg="black", pad=0, least=(320, 200)):
    """with no window manager, a program's window sits in a corner of the black screen: keep just the window.
    For terminals (bg = the terminal's colour, pad > 0): keep the text plus a margin, shaped 16:10."""
    r = _run(["magick", str(shot), "-bordercolor", bg, "-border", "1", "-fuzz", "4%", "-trim", "-format", "%w %h %X %Y", "info:"], 30)
    try:
        w, h, x, y = (int(v.lstrip("+")) for v in r.stdout.split())
    except (AttributeError, ValueError):
        return raw, shot
    x, y = max(0, x - 1 - pad), max(0, y - 1 - pad)
    w, h = w + 2 * pad, h + 2 * pad
    if pad:
        w, h = max(w, least[0], int(h * 1.6)), max(h, least[1])
        h = max(h, int(w / 1.6))
    w, h = min(w, W - x) // 2 * 2, min(h, H - y) // 2 * 2
    if w * h > 0.9 * W * H or w < least[0] or h < least[1]:
        return raw, shot
    raw2, shot2 = tmp / "raw2.mp4", tmp / "shot2.png"
    _run(["ffmpeg", "-nostdin", "-loglevel", "error", "-y", "-i", str(raw), "-vf", f"crop={w}:{h}:{x}:{y}", "-c:v", "libx264",
          "-preset", "ultrafast", "-crf", "18", "-pix_fmt", "yuv420p", str(raw2)], 120)
    _run(["magick", str(shot), "-crop", f"{w}x{h}+{x}+{y}", "+repage", str(shot2)], 30)
    return (raw2, shot2) if raw2.exists() and shot2.exists() else (raw, shot)



def godot_tidy(since):
    """a Godot game stopped from outside leaves a recovery lock, and the editor would then offer recovery mode."""
    for lock in Path.home().glob(".local/share/godot/app_userdata/*/.recovery_mode_lock"):
        try:
            if lock.stat().st_mtime >= since - 1:
                lock.unlink()
        except OSError:
            pass


def run_capture(p, web_root, screen, thumbs, k, used):
    started = time.time()
    try:
        return _run_capture(p, web_root, screen, thumbs, k, used)
    finally:
        if p["type"] == "godot":
            godot_tidy(started)


def _run_capture(p, web_root, screen, thumbs, k, used):
    d = p["path"]
    if (Path(d) / ".no-capture").exists() or not screen.start():
        return []
    with tempfile.TemporaryDirectory(prefix="manager-cap-") as tmp:
        tmp = Path(tmp)
        root = web_root(p)
        if root:                                  # web page: a static server + chromium on the invisible screen
            import http.server
            from index import Quiet  # noqa: the same quiet, utf-8 file server the stills use
            srv = http.server.ThreadingHTTPServer(("127.0.0.1", 0), lambda *a, **kw: Quiet(*a, directory=str(root), **kw))
            threading.Thread(target=srv.serve_forever, daemon=True).start()
            browser = shutil.which("chromium") or shutil.which("google-chrome-stable")
            try:
                got = _record_while(screen, [browser, f"--app=http://127.0.0.1:{srv.server_address[1]}/", "--no-first-run",
                                             f"--user-data-dir={tmp}/profile", f"--window-size={W},{H}", "--window-position=0,0",
                                             "--mute-audio", "--disable-extensions", "--use-angle=swiftshader",
                                             "--enable-unsafe-swiftshader", "--hide-scrollbars"], d, tmp)
            finally:
                srv.shutdown(); srv.server_close()
            return finish(*got, thumbs, k, used, "the page") if got else []

        try:
            script = (Path(d) / "make.sh").read_text()
        except OSError:
            return []
        # "unknown" projects run too, unless their make.sh only points at an outside editor
        editor_only = re.search(r"xdg-open|GameMaker|Construct|editor\.", script)
        if any(x in d + "/" for x in NEVER_RUN) or (p["type"] not in RUN_TYPES and (p["type"] != "unknown" or editor_only)):
            return []
        b = _run(["./make.sh", "build"], 600, cwd=d, env=screen.env())
        if not b or b.returncode != 0:
            return []
        got = _record_while(screen, ["./make.sh"], d, tmp, limit=0.006)   # a program with a window of its own
        # (the empty screen is pure black, so even a dark window with a little in it counts)
        if not got:                                                    # a terminal program: show it in kitty
            kitty = shutil.which("kitty")
            if not kitty:
                return []
            # only worth showing if it prints something (a file of definitions prints nothing)
            said = _run(["timeout", "8", "./make.sh"], 15, cwd=d, env=screen.env(), stdin=subprocess.DEVNULL)
            text = (said.stdout + said.stderr).strip() if said else ""
            if not text or FAILED.search(text):
                return []
            hold = './make.sh; echo; echo "[finished]"; sleep 60'
            got = _record_while(screen, [kitty, "--config", "NONE", "-o", f"initial_window_width={W}",
                                         "-o", f"initial_window_height={H}", "-o", "remember_window_size=no",
                                         "-o", "placement_strategy=top-left",
                                         "-o", "font_family=Agave", "-o", "font_size=21", "-o", f"background={BG}",
                                         "-o", f"foreground={BASE}", "-o", f"cursor={GOLD}", "-o", "window_padding_width=18",
                                         "-o", f"color1={BURNT}", "-o", f"color2={ORANGE}", "-o", f"color3={GOLD}",
                                         "-o", f"color4={BURNT}", "-o", f"color5={ORANGE}", "-o", f"color6={GOLD}",
                                         "-o", f"color7={BASE}", "-o", f"color8={DIM}", "-o", "linux_display_server=x11",
                                         "bash", "-c", hold], d, tmp, wait=10, limit=0.004,   # a few lines of text is enough
                                   fit={"bg": BG, "pad": 40, "least": (640, 400)})
            label = "in a terminal"
        else:
            label = "running"
        return finish(*got, thumbs, k, used, label) if got else []
