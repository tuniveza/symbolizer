"""The brick training daemon: the imagination neurons learning your English all the time, quietly.

    brick trainer neurons always      start it now and with every login (a systemd user service)
    brick trainer neurons off         stop it, and don't start it at login
    brick trainer neurons status      is it running, and what has it learned lately

What it does, over and over:
  - a learning round (trainer/intuition.py): Ni, Ne and the other functions read brick's input (the sentence pool,
    your programs in wa/, the examples, public text) and the compiler checks what Ne proposes;
  - every 6th round it makes new bricks (only ones that pass every check: the brain's bitstream neurons, general
    English, the exact bits, and all tests still passing);
  - once a day it looks up 20 new words in the internet dictionary (trainer/wordfeed.py).

It stays out of your way: lowest CPU and disk priority, a pause between rounds, much longer pauses on battery,
and it stops cleanly between steps. Only one learner runs at a time (a lock), so brick gui's trainer waits for it.
"""
import json
import os
import re
import signal
import subprocess
import sys
import time

HERE = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
if HERE not in sys.path:
    sys.path.insert(0, HERE)

from trainer import intuition  # noqa: E402

SERVICE = "brick-neurons.service"
UNIT = """[Unit]
Description=brick's imagination neurons, learning your English all the time

[Service]
WorkingDirectory={here}
Environment=PYTHONPATH={here}
ExecStart={python} {here}/trainer/daemon.py
Restart=on-failure
RestartSec=60
Nice=19
IOSchedulingClass=idle
CPUSchedulingPolicy=idle
CPUWeight=1

[Install]
WantedBy=default.target
"""
STATUS = os.path.join(intuition.FOLDER, "daemon-status.json")
LOG = os.path.join(intuition.FOLDER, "daemon.log")
PAUSE, BATTERY_PAUSE = 30, 600          # seconds between rounds (on mains, on battery)
BRICKS_EVERY, WORDS_EVERY = 6, 86400    # rounds between brick making; seconds between dictionary lookups
TEACH_EVERY = 3                         # rounds between lessons from the language model's readings (trainer/teacher.py)
SPEAK_EVERY = 10                        # rounds between the narrator's round summaries (when it's switched on)
stopping = False


def on_battery():
    base = "/sys/class/power_supply"
    try:
        mains = [d for d in os.listdir(base) if open(os.path.join(base, d, "type")).read().strip() == "Mains"]
        return bool(mains) and not any(open(os.path.join(base, d, "online")).read().strip() == "1" for d in mains)
    except OSError:
        return False


def log(line):
    os.makedirs(intuition.FOLDER, exist_ok=True)
    stamp = time.strftime("%Y-%m-%d %H:%M")
    with open(LOG, "a", encoding="utf-8") as f:
        f.write(f"{stamp}  {line}\n")
    try:                                 # keep the log short
        if os.path.getsize(LOG) > 400_000:
            with open(LOG, encoding="utf-8") as f:
                lines = f.readlines()[-2000:]
            with open(LOG, "w", encoding="utf-8") as f:
                f.writelines(lines)
    except OSError:
        pass


def status(**fields):
    old = read_status()
    old.update(fields, updated=time.strftime("%Y-%m-%d %H:%M"))
    os.makedirs(intuition.FOLDER, exist_ok=True)
    with open(STATUS, "w", encoding="utf-8") as f:
        json.dump(old, f)


def read_status():
    try:
        with open(STATUS, encoding="utf-8") as f:
            return json.load(f)
    except (OSError, ValueError):
        return {}


def hear(event):
    """Tell the narrator (trainer/narrator.py) what just happened: it speaks only when switched on, in the background,
    and nothing it does can stop the daemon."""
    try:
        from trainer import narrator
        if narrator.on():
            narrator.speak(event)
    except Exception:
        pass


def nap(seconds):
    end = time.time() + seconds
    while not stopping and time.time() < end:
        time.sleep(min(5, end - time.time()))


def main():
    global stopping

    def stop(*_):
        global stopping
        stopping = True
    signal.signal(signal.SIGTERM, stop)
    signal.signal(signal.SIGINT, stop)
    os.environ.setdefault("BRICK_TRAINER", "off")         # builds inside never start another trainer
    log("started")
    rounds = read_status().get("rounds", 0)
    while not stopping:
        said = []
        try:
            got = intuition.learn(1, say=said.append)
        except Exception as error:                        # log it and carry on after a pause
            log(f"round failed: {error}")
            nap(PAUSE * 4)
            continue
        rounds += 1
        if said:
            log(said[-1])
            if rounds % SPEAK_EVERY == 0 or re.search(r": [1-9]\d* kept", said[-1]):
                hear({"kind": "round", "round": rounds, "text": said[-1]})
        if got:
            status(rounds=rounds, neurons=got.get("neurons"), lines=got.get("settled"), proved=got.get("proved"),
                   ni=got.get("ni"), bits=got.get("bits"), phrases=got.get("phrases"))
        if stopping:
            break
        if rounds % BRICKS_EVERY == 0 and not on_battery():
            made = intuition.make_bricks(limit=5, say=lambda s: log(s.strip()))
            status(last_bricks=made, bricks_made=read_status().get("bricks_made", 0) + len(made))
            if made:
                hear({"kind": "bricks", "lines": made})
        if rounds % TEACH_EVERY == 0 and not on_battery():
            try:                                          # the model's readings of your lines, taught to the brain
                from trainer import teacher
                got = teacher.teach(5, say=lambda s: log(s.strip()))
                status(taught=read_status().get("taught", 0) + len(got["learned"]))
                if got["learned"]:
                    hear({"kind": "say", "text": f"The language model taught me to read '{got['learned'][0]}' by myself."})
            except Exception as error:
                log(f"teaching failed: {error}")
        last_words = read_status().get("words_at", 0)
        if time.time() - last_words > WORDS_EVERY and not on_battery():
            try:
                from trainer import wordfeed
                fed = wordfeed.feed(20, rounds=0, say=lambda s: log(s.strip()))
                status(words_at=time.time(), last_words=(fed or {}).get("defined", []))
                if fed:
                    hear({"kind": "words", **fed})
                    if fed.get("bricks"):
                        hear({"kind": "bricks", "lines": fed["bricks"]})
            except Exception as error:
                log(f"dictionary lookup failed: {error}")
                status(words_at=time.time())
        nap(BATTERY_PAUSE if on_battery() else PAUSE)
    log("stopped")


def state():
    """The daemon as data: {"running", "enabled", ...the last status fields}."""
    running = subprocess.run(["systemctl", "--user", "is-active", "--quiet", SERVICE]).returncode == 0
    enabled = subprocess.run(["systemctl", "--user", "is-enabled", "--quiet", SERVICE]).returncode == 0
    return {"running": running, "enabled": enabled, **read_status(), "log": LOG}


def command(what):
    unit = os.path.expanduser(f"~/.config/systemd/user/{SERVICE}")
    if what in ("always", "off"):
        if what == "always":
            os.makedirs(os.path.dirname(unit), exist_ok=True)
            with open(unit, "w") as f:
                f.write(UNIT.format(here=HERE, python=sys.executable))
            subprocess.run(["systemctl", "--user", "daemon-reload"])
        done = subprocess.run(["systemctl", "--user", "enable" if what == "always" else "disable", "--now", SERVICE],
                              capture_output=True, text=True)
        if done.returncode != 0:
            print("systemd said:", done.stderr.strip())
            return 1
        print("The training daemon is on: it learns all the time, at the lowest priority, and starts at every login."
              if what == "always" else "The training daemon is off.")
        return 0
    running = subprocess.run(["systemctl", "--user", "is-active", "--quiet", SERVICE]).returncode == 0
    st = read_status()
    print(f"The training daemon is {'running' if running else 'not running'}" +
          (f" (last round {st['updated']})." if st.get("updated") else "."))
    if st:
        print(f"{st.get('rounds', 0)} rounds: {st.get('neurons')} neurons ({st.get('phrases') or 0} phrases), {st.get('lines')} lines, "
              f"{st.get('proved')} proved. Ni guesses {round((st.get('ni') or 0) * 100)}% of next words and "
              f"{round((st.get('bits') or 0) * 100)}% of brick's output pieces.")
        print(f"New bricks made so far: {st.get('bricks_made', 0)}" + (f" (last: {', '.join(st['last_bricks'])})" if st.get("last_bricks") else ""))
    print("Its log: " + LOG)
    return 0


if __name__ == "__main__":
    main()
