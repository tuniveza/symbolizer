#!/usr/bin/env python3
"""manager gui — every project in ~/an with its pictures and videos, in a window of its own.

  manager gui              open the window (starts the little server; it stops ~3 min after the window goes)
  manager gui --serve      just serve http://localhost:8767 (no window, never stops by itself)

The page is gui/www/. What it shows comes from ~/.cache/an-manager/gui/data.json, which `manager index` writes
(a systemd timer runs it every 3 days; the ⟳ button runs it now). The server only listens on this
computer, and every button press must carry a token that only the page itself knows, so other web
pages can't press them.
"""
import json
import mimetypes
import os
import secrets
import shutil
import subprocess
import sys
import threading
import time
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from pathlib import Path
from urllib.parse import parse_qs, urlparse

HERE = Path(__file__).resolve().parent
WWW = HERE / "www"
MANAGER = HERE.parent
HOME = Path.home()
CACHE = HOME / ".cache" / "an-manager" / "gui"
THUMBS = CACHE / "t"
DATA = CACHE / "data.json"
STATE = CACHE / "state.json"
RECENT = MANAGER / ".state" / "recent"
PORT = 8767
HOSTS = {f"localhost:{PORT}", f"127.0.0.1:{PORT}"}
ADDRESS = f"http://localhost:{PORT}/"           # "localhost" gives the window its own class (brick uses 127.0.0.1)
IDLE = 180
TOKEN = secrets.token_urlsafe(24)
last_seen = [time.monotonic()]

mimetypes.add_type("image/webp", ".webp")
mimetypes.add_type("video/mp4", ".mp4")
mimetypes.add_type("text/javascript", ".js")


class Data:
    """data.json, re-read only when it changes; plus which files and folders the page may touch."""
    mtime, data, files, dirs = 0.0, {"projects": []}, set(), {}

    @classmethod
    def get(cls):
        try:
            m = DATA.stat().st_mtime
        except OSError:
            return cls.data
        if m != cls.mtime:
            try:
                cls.data = json.loads(DATA.read_text())
                cls.mtime = m
            except ValueError:
                return cls.data
            cls.files = {v for p in cls.data["projects"] for md in p.get("media", [])
                         for k, v in md.items() if k in ("src", "thumbSrc") and v}
            cls.dirs = {p["path"]: p for p in cls.data["projects"]}
        return cls.data


def detach(cmd, cwd=None):
    subprocess.Popen(cmd, cwd=cwd, stdin=subprocess.DEVNULL, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL,
                     start_new_session=True, env={**os.environ, "PATH": f"{HOME}/.local/bin:{os.environ.get('PATH', '/usr/bin:/bin')}"})


def in_terminal(project, script, label):
    """run a project script in a kitty window that stays open afterwards, so its output can be read."""
    hold = f'./{script}; s=$?; echo; read -rsn1 -p "[{label} finished ($s) — any key closes this]"'
    term = shutil.which("kitty")
    if term:
        detach([term, "--class", "manager-run", "--title", f"{project['name']} — {label}", "--directory", project["path"],
                "bash", "-c", hold])
    else:
        detach(["x-terminal-emulator", "-e", "bash", "-c", f'cd "{project["path"]}" && {hold}'])


def mgr_recent_add(path):
    try:
        lines = [l for l in RECENT.read_text().splitlines() if l and l != path] if RECENT.exists() else []
        RECENT.parent.mkdir(parents=True, exist_ok=True)
        RECENT.write_text("\n".join((lines + [path])[-30:]) + "\n")
    except OSError:
        pass


def act(project, action):
    d = project["path"]
    if action == "run":
        mgr_recent_add(d); in_terminal(project, "make.sh", "run")
    elif action == "watch":
        if not os.access(os.path.join(d, "reload.sh"), os.X_OK):
            return "this project has no reload.sh"
        mgr_recent_add(d); in_terminal(project, "reload.sh", "watch")
    elif action == "build":
        in_terminal(project, "make.sh build", "build")
    elif action == "open":
        detach([str(MANAGER / "bin" / "manager"), "open", d])
    elif action == "folder":
        fm = shutil.which("dolphin") or shutil.which("xdg-open")
        detach([fm, d])
    else:
        return "unknown action"
    return None


def refresh(force):
    """run manager index now, at idle priority, unless it's already running (it locks itself)."""
    cmd = ["nice", "-n", "19", "ionice", "-c", "3", sys.executable, str(HERE / "index.py")] + (["--force"] if force else [])
    detach(cmd)


class Handler(BaseHTTPRequestHandler):
    protocol_version = "HTTP/1.1"

    def log_message(self, *a):
        pass

    def send(self, body, kind="application/json", status=200, cache="no-store"):
        last_seen[0] = time.monotonic()
        if not isinstance(body, (bytes, bytearray)):
            body = json.dumps(body, separators=(",", ":")).encode()
        self.send_response(status)
        self.send_header("Content-Type", kind)
        self.send_header("Content-Length", str(len(body)))
        self.send_header("Cache-Control", cache)
        self.send_header("X-Content-Type-Options", "nosniff")
        self.end_headers()
        if self.command != "HEAD":
            self.wfile.write(body)

    def file(self, path, cache="max-age=86400"):
        """a file, with Range support so videos can seek."""
        try:
            f = open(path, "rb")
        except OSError:
            return self.send({"error": "gone"}, status=404)
        last_seen[0] = time.monotonic()
        with f:
            size = os.fstat(f.fileno()).st_size
            kind = mimetypes.guess_type(str(path))[0] or "application/octet-stream"
            start, end = 0, size - 1
            rng = self.headers.get("Range", "")
            if rng.startswith("bytes="):
                a, _, b = rng[6:].split(",")[0].partition("-")
                try:
                    if a:
                        start, end = int(a), (int(b) if b else size - 1)
                    else:
                        start = max(0, size - int(b))
                except ValueError:
                    start, end = 0, size - 1
                end = min(end, size - 1)
                if start > end:
                    self.send_response(416); self.send_header("Content-Range", f"bytes */{size}")
                    self.send_header("Content-Length", "0"); self.end_headers(); return
                self.send_response(206)
                self.send_header("Content-Range", f"bytes {start}-{end}/{size}")
            else:
                self.send_response(200)
            self.send_header("Content-Type", kind)
            self.send_header("Accept-Ranges", "bytes")
            self.send_header("Content-Length", str(end - start + 1))
            self.send_header("Cache-Control", cache)
            self.end_headers()
            if self.command == "HEAD":
                return
            f.seek(start)
            left = end - start + 1
            try:
                while left > 0:
                    chunk = f.read(min(1 << 16, left))
                    if not chunk:
                        break
                    self.wfile.write(chunk)
                    left -= len(chunk)
            except (BrokenPipeError, ConnectionResetError):
                pass

    def host_ok(self):
        return self.headers.get("Host", "") in HOSTS      # no DNS-rebinding tricks

    def do_HEAD(self):
        self.do_GET()

    def do_GET(self):
        if not self.host_ok():
            return self.send({"error": "wrong host"}, status=403)
        url = urlparse(self.path)
        q = {k: v[0] for k, v in parse_qs(url.query).items()}
        p = url.path
        if p == "/api/alive":
            return self.send({"ok": True})
        if p in ("/", "/index.html"):
            html = (WWW / "index.html").read_text().replace("__MANAGER_TOKEN__", TOKEN)
            return self.send(html.encode(), "text/html; charset=utf-8")
        if p in ("/app.js", "/style.css", "/icon.svg"):
            kind = {"js": "text/javascript", "css": "text/css", "svg": "image/svg+xml"}[p.rsplit(".", 1)[1]]
            return self.send((WWW / p[1:]).read_bytes(), kind + "; charset=utf-8", cache="no-cache")
        if p == "/api/data":
            d = Data.get()
            try:
                recent = [l for l in RECENT.read_text().splitlines() if l]
            except OSError:
                recent = []
            return self.send({**d, "recent": recent, "state": self.state()})
        if p == "/api/state":
            return self.send(self.state())
        if p.startswith("/t/"):                            # thumbnails + previews the indexer made
            name = p[3:]
            if "/" in name or name.startswith("."):
                return self.send({"error": "no"}, status=404)
            return self.file(THUMBS / name, cache="max-age=31536000, immutable")
        if p == "/f":                                      # media inside projects, only ones data.json lists
            Data.get()
            path = q.get("p", "")
            if path not in Data.files:
                return self.send({"error": "not listed"}, status=404)
            return self.file(path)
        return self.send({"error": "not here"}, status=404)

    def state(self):
        try:
            st = json.loads(STATE.read_text())
        except (OSError, ValueError):
            st = {}
        st["generated"] = Data.get().get("generated")
        return st

    def do_POST(self):
        if not self.host_ok() or self.headers.get("X-Manager-Token") != TOKEN:
            return self.send({"error": "no"}, status=403)
        try:
            body = json.loads(self.rfile.read(int(self.headers.get("Content-Length") or 0)) or b"{}")
        except ValueError:
            return self.send({"error": "bad json"}, status=400)
        if self.path == "/api/do":
            Data.get()
            project = Data.dirs.get(body.get("path"))
            if not project:
                return self.send({"error": "not a known project"}, status=404)
            err = act(project, body.get("action"))
            return self.send({"ok": not err, "error": err})
        if self.path == "/api/refresh":
            refresh(bool(body.get("force")))
            return self.send({"ok": True})
        return self.send({"error": "not here"}, status=404)


def open_window():
    for b in (str(HOME / ".local/bin/emberline-helium"), "helium", "chromium", "google-chrome-stable", "brave"):
        found = b if os.access(b, os.X_OK) and os.path.isfile(b) else shutil.which(b)
        if found:
            return detach([found, f"--app={ADDRESS}"])
    import webbrowser
    webbrowser.open(ADDRESS)


def close_when_idle(server):
    while True:
        time.sleep(15)
        if time.monotonic() - last_seen[0] > IDLE:
            server.shutdown()
            return


def main(argv):
    serve_only = "--serve" in argv
    try:
        server = ThreadingHTTPServer(("127.0.0.1", PORT), Handler)
    except OSError:                                    # already running: just open another window onto it
        if not serve_only:
            open_window()
        print(f"manager gui is already running: {ADDRESS}")
        return 0
    server.daemon_threads = True
    if not DATA.exists():                              # first time: gather everything (the page shows progress)
        refresh(False)
    print(f"manager gui: {ADDRESS}" + ("" if serve_only else "  (closes a few minutes after its window does)"))
    if not serve_only:
        threading.Timer(0.2, open_window).start()
        threading.Thread(target=close_when_idle, args=(server,), daemon=True).start()
    try:
        server.serve_forever()
    except KeyboardInterrupt:
        pass
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
