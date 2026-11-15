"""The sandbox: run a program brick made where it can't do any harm, and see exactly what it did.

Used by the inventor (trainer/inventor.py) to check that a new kind of program does what the English asked.

  run(program, files={"notes.txt": "a\\nb\\n"}, stdin="", seconds=1)
    -> {"out", "err", "exit", "files": {name: text}, "timed_out"}

The program runs under bubblewrap (bwrap): no network, no other processes, no home folder, nothing writable but
a fresh folder holding only the files given; read-only system folders; at most `seconds` of CPU, 256 MB of
memory, 1 MB per file it writes (other processes: none it can see). What comes back is everything it can be judged by: what it
printed, what files it left, and how it ended.
"""
import os
import resource
import shutil
import subprocess
import tempfile

LIMITS = [(resource.RLIMIT_AS, 256 << 20), (resource.RLIMIT_FSIZE, 1 << 20),
          (resource.RLIMIT_CORE, 0)]


def available():
    return shutil.which("bwrap") is not None


def run(program, files=None, stdin="", seconds=1):
    if not available():
        raise RuntimeError("the sandbox needs bubblewrap (bwrap), and it isn't installed")
    with tempfile.TemporaryDirectory(prefix="brick-sandbox-") as work:
        for name, text in (files or {}).items():
            path = os.path.join(work, os.path.basename(name))
            with open(path, "w", encoding="utf-8") as f:
                f.write(text)
        shutil.copy(program, os.path.join(work, ".program"))
        os.chmod(os.path.join(work, ".program"), 0o755)
        cmd = ["bwrap", "--unshare-all", "--die-with-parent", "--new-session", "--cap-drop", "ALL",
               "--ro-bind", "/usr", "/usr", "--symlink", "usr/lib", "/lib", "--symlink", "usr/lib64", "/lib64",
               "--symlink", "usr/bin", "/bin", "--proc", "/proc", "--dev", "/dev", "--tmpfs", "/tmp",
               "--bind", work, "/work", "--chdir", "/work", "--clearenv", "--setenv", "PATH", "/usr/bin",
               "--setenv", "TERM", "dumb", "/work/.program"]

        def limit():
            cpu = max(1, int(seconds))
            resource.setrlimit(resource.RLIMIT_CPU, (cpu, cpu))
            for what, value in LIMITS:
                resource.setrlimit(what, (value, value))
        timed_out = False
        try:
            done = subprocess.run(cmd, input=stdin.encode(), capture_output=True, timeout=seconds + 2, preexec_fn=limit)
            out, err, code = done.stdout, done.stderr, done.returncode
        except subprocess.TimeoutExpired as e:
            out, err, code, timed_out = e.stdout or b"", e.stderr or b"", None, True
        left = {}
        for name in sorted(os.listdir(work)):
            path = os.path.join(work, name)
            if name == ".program" or not os.path.isfile(path):
                continue
            with open(path, "rb") as f:
                left[name] = f.read(1 << 20).decode("utf-8", "replace")
    return {"out": out.decode("utf-8", "replace"), "err": err.decode("utf-8", "replace"), "exit": code,
            "files": left, "timed_out": timed_out}
