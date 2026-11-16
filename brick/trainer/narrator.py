"""The narrator: brick's brains say out loud what they're doing, each in its own archetype's voice.

    brick trainer narrator on             switch it on (it's off until you do)
    brick trainer narrator off            switch it off
    brick trainer narrator status         on or off, which voices and where its lines come from
    brick trainer narrator setup          download its Piper voices and install Piper (skips what's here)
    brick trainer narrator say TEXT [ARCHETYPE]
                                          say TEXT now in that archetype's voice (or the next one's)

While the training daemon (trainer/daemon.py) works, it tells the narrator what just happened: a new brick, words
looked up in the dictionary, a round's summary. One of the 48 archetypes turns that into one short first-person line
in its own personality (the local language model writes it when it's already running; otherwise a hand-made line
for the archetype's bias), and says it in its own voice: a Piper neural voice (trainer/data/voices/), with the speed,
pitch and liveliness of its type, or espeak-ng when Piper isn't there.

It never chatters: one line at a time, at most one every 8 seconds, never more than 3 waiting (the rest are dropped),
softly. It never holds up the daemon: lines are made and played in a background thread.
"""
import json
import os
import queue
import random
import re
import shutil
import subprocess
import sys
import tempfile
import threading
import time
import wave
import zlib

HERE = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
if HERE not in sys.path:
    sys.path.insert(0, HERE)

SETTINGS = os.path.join(HERE, "trainer", "data", "narrator.json")
VOICES = os.path.join(HERE, "trainer", "data", "voices")
VOLUME = 0.35              # soft: the stream volume for pw-play (0-1)
GAP = 8                    # at least this many seconds between lines
MAX_WAITING = 3            # more than this waiting: new lines are dropped
STALE = 120                # a line still waiting after this many seconds is dropped
MAX_WORDS = 20             # the language model's line must be this short

ALBA, JENNY, NORTH = "en_GB-alba-medium", "en_GB-jenny_dioco-medium", "en_GB-northern_english_male-medium"
AMY, RYAN = "en_US-amy-medium", "en_US-ryan-medium"
PIPER_VOICES = (ALBA, JENNY, NORTH, AMY, RYAN)

# Each of the 16 types: (Piper voice, length scale (above 1 = slower), noise (liveliness), noise_w (cadence),
# pitch (the playback rate: above 1 = higher), espeak-ng voice, espeak pitch 0-99, espeak words per minute).
# Introverts slower and softer, extraverts quicker; feelers livelier, thinkers flatter; ENTJ the briskest.
TYPES = {
    "INFJ": (ALBA, 1.18, 0.55, 0.80, 0.98, "en-gb-x-rp+f2", 45, 140),
    "INFP": (ALBA, 1.22, 0.70, 0.95, 1.03, "en-gb+f4", 60, 135),
    "ISFP": (ALBA, 1.12, 0.75, 0.90, 1.06, "en-gb-scotland+f3", 65, 145),
    "ENFJ": (JENNY, 1.00, 0.62, 0.80, 1.00, "en-gb+f3", 55, 165),
    "ENFP": (JENNY, 0.95, 0.78, 1.00, 1.05, "en-us+f5", 70, 175),
    "ESFP": (JENNY, 0.92, 0.80, 0.95, 1.08, "en-us+f4", 75, 180),
    "INTJ": (NORTH, 1.15, 0.45, 0.65, 0.96, "en-gb-x-rp+m3", 35, 145),
    "INTP": (NORTH, 1.20, 0.60, 0.90, 1.00, "en-gb+m4", 45, 140),
    "ISTP": (NORTH, 1.08, 0.50, 0.70, 0.93, "en-gb-x-gbclan+m2", 30, 150),
    "ISFJ": (AMY, 1.15, 0.60, 0.75, 0.99, "en-us+f2", 50, 145),
    "ESFJ": (AMY, 0.98, 0.70, 0.85, 1.04, "en-us+f1", 60, 170),
    "ESTP": (AMY, 0.90, 0.72, 0.90, 0.97, "en-us-nyc+f3", 55, 185),
    "ISTJ": (RYAN, 1.12, 0.45, 0.60, 0.97, "en-us+m2", 35, 150),
    "ESTJ": (RYAN, 0.95, 0.50, 0.65, 0.95, "en-us+m1", 40, 175),
    "ENTJ": (RYAN, 0.86, 0.48, 0.70, 0.94, "en-gb-x-rp+m1", 38, 190),
    "ENTP": (RYAN, 0.92, 0.70, 1.00, 1.02, "en-us-nyc+m3", 50, 180),
}
# The bias nudges the type's voice: (length, noise) added.
BIAS_VOICE = {"swift": (-0.06, 0.0), "order": (-0.03, -0.04), "strict": (0.0, -0.05),
              "keep": (0.03, 0.0), "split": (0.0, 0.03), "explore": (0.04, 0.06)}
# How each bias speaks, for the language model.
MANNER = {"strict": "precise and exact, a little clipped", "swift": "brisk, as few words as possible",
          "explore": "curious and dreamy, full of wonder", "keep": "warm, steady and gentle",
          "split": "analytical, sees things from two sides", "order": "firm and businesslike"}
NARRATOR = {"name": "The Narrator", "type": "INFJ", "bias": "keep", "line": "Tells what the brains are doing, calmly."}


# ---------------- the switch ----------------

def settings():
    try:
        with open(SETTINGS, encoding="utf-8") as f:
            found = json.load(f)
        return {"on": False, **found} if isinstance(found, dict) else {"on": False}
    except (OSError, ValueError):
        return {"on": False}


def on():
    """Is the narrator switched on? (Off unless it was turned on.)"""
    return os.environ.get("BRICK_NARRATOR") != "off" and settings().get("on") is True


def set_on(value):
    found = {**settings(), "on": bool(value)}
    os.makedirs(os.path.dirname(SETTINGS), exist_ok=True)
    with open(SETTINGS, "w", encoding="utf-8") as f:
        json.dump(found, f)
    if not value:                        # anything still waiting is dropped
        while True:
            try:
                _queue.get_nowait()
            except queue.Empty:
                break


# ---------------- the archetypes ----------------

_archetypes = None
_turn = 0


def archetypes():
    """The 48 archetypes from the engine (kept for this process); [NARRATOR] if the engine can't be reached."""
    global _archetypes
    if _archetypes is None:
        try:
            from trainer import neurons
            _archetypes = [a for a in neurons.archetypes() if isinstance(a, dict) and a.get("name")]
        except Exception:
            _archetypes = []
    return _archetypes or [NARRATOR]


def clean_name(name):
    return re.sub(r"[^a-z ]", "", re.sub(r"^the\s+", "", str(name).lower())).strip()


def find(name):
    """The archetype called name ('The Ember Poet', 'ember poet'), or None."""
    want = clean_name(name)
    return next((a for a in archetypes() if clean_name(a["name"]) == want), None) if want else None


def pick(archetype=None, event=None):
    """The archetype to speak: the one given (a dict or a name), the event's own, or the next in turn."""
    global _turn
    if isinstance(archetype, dict):
        return archetype
    if archetype:
        found = find(archetype)
        if found:
            return found
    if isinstance(event, dict) and event.get("archetype"):
        found = find(event["archetype"])
        if found:
            return found
    every = archetypes()
    chosen = every[_turn % len(every)]
    _turn += 1
    return chosen


# ---------------- what happened, in words ----------------

def listing(words, most=3):
    words = [str(w) for w in words if str(w).strip()]
    if len(words) <= 1:
        return "".join(words)
    if len(words) <= most:
        return ", ".join(words[:-1]) + " and " + words[-1]
    return ", ".join(words[:most]) + f" and {len(words) - most} more"


def brick_pair(line):
    """'carmine = crimson' -> ('carmine', 'crimson')."""
    left, _, right = str(line).partition("=")
    return left.strip(), right.strip()


def facts(event):
    """(kind, {the pieces}, the words a line must keep) for an event: a dict from the daemon, or a log line."""
    if isinstance(event, str):
        text = event.strip()
        got = re.match(r"(?:new brick|would make):\s*(.+=.+)$", text)
        if got:
            event = {"kind": "bricks", "lines": [got.group(1)]}
        elif re.match(r"round \d+:", text):
            event = {"kind": "round", "text": text}
        else:
            event = {"kind": "say", "text": text}
    kind = event.get("kind", "say")
    if kind in ("brick", "bricks"):
        lines = event.get("lines") or [event.get("line", "")]
        word, means = brick_pair(lines[0])
        if not word or not means:
            return "say", {"text": "I made a new brick."}, []
        return "brick", {"word": word, "means": means, "more": len(lines) - 1}, [word, means]
    if kind == "words":
        words = list(event.get("words") or [])
        defined = list(event.get("defined") or [])
        if not words:
            return "say", {"text": "I looked in the dictionary, but every word was known already."}, []
        first = (defined or words)[0]
        return "words", {"n": len(words), "d": len(defined), "first": first, "list": listing(defined or words)}, [first]
    if kind == "round":
        text = str(event.get("text", ""))
        number = event.get("round") or (re.match(r"round (\d+)", text) or [None, None])[1]
        ni = event.get("ni")
        if ni is None:
            got = re.search(r"Ni guesses (\d+)%", text)
            ni = int(got.group(1)) if got else None
        elif isinstance(ni, float) and ni <= 1:
            ni = round(ni * 100)
        kept = re.search(r"(\d+) kept", text)
        f = {"round": number, "ni": ni, "kept": int(kept.group(1)) if kept else 0}
        return "round", f, [str(v) for v in (number, ni) if v is not None]          # the numbers must stay exact
    text = re.sub(r"\s+", " ", str(event.get("text", ""))).strip()
    words = text.split()
    return "say", {"text": " ".join(words[:16]) + ("..." if len(words) > 16 else "")}, []


def fact_sentence(kind, f):
    """The plain fact, for the language model to put in the archetype's words."""
    if kind == "brick":
        return f"I made a new brick: the word '{f['word']}' now means '{f['means']}'" + \
            (f" (and {f['more']} more new bricks)" if f["more"] else "") + "."
    if kind == "words":
        return f"I looked up {f['n']} new words in the dictionary; {f['d']} had entries, like '{f['first']}'."
    if kind == "round":
        bits = [f"I finished learning round {f['round']}" if f["round"] else "I finished a learning round"]
        if f["ni"] is not None:
            bits.append(f"I now guess {f['ni']} percent of next words")
        if f["kept"]:
            bits.append(f"{f['kept']} of my own new sentences passed the checks")
        return "; ".join(bits) + "."
    return f["text"]


# Hand-made lines for each bias, used when the language model isn't running.
TEMPLATES = {
    "brick": {
        "explore": ["A new word drifted in: {word}. It means {means}.", "Oh, look. {word} turns out to mean {means}."],
        "strict": ["Verified: {word} means exactly {means}. Every test still passes.", "Checked twice. {word} means {means}."],
        "swift": ["New brick. {word} means {means}. Done.", "{word} equals {means}. Next."],
        "keep": ["I've kept a new word for you: {word}, meaning {means}.", "{word} means {means}. I'll hold on to it."],
        "split": ["{word} and {means}: two words, one meaning. I've joined them.", "Seen from both sides, {word} means {means}."],
        "order": ["Filed: {word} now means {means}.", "New brick in place. {word} means {means}."],
    },
    "words": {
        "explore": ["I wandered through the dictionary and found {n} new words, like {first}.",
                    "{n} new words to wonder about, starting with {first}."],
        "strict": ["Dictionary checked: {d} of {n} words had real entries.", "{d} of {n} words confirmed, {first} among them."],
        "swift": ["Looked up {n} words. {d} found.", "{n} words, {d} defined. Moving on."],
        "keep": ["I read {n} new words today and kept {d} of them, starting with {first}.",
                 "{d} new words are safe with me now, like {first}."],
        "split": ["{first} and friends: {n} words looked up, {d} defined.", "{n} words in, {d} with meanings; {first} first."],
        "order": ["Dictionary run complete: {n} words, {d} defined.", "{n} words looked up. {d} entries filed."],
    },
    "wrap": {                            # rounds and anything else: around the plain fact
        "explore": ["Another wander through the words. {fact}", "Something stirred. {fact}"],
        "strict": ["{fact} Precision holds.", "Measured carefully: {fact}"],
        "swift": ["{fact} Moving on.", "Quick update. {fact}"],
        "keep": ["Steady as ever. {fact}", "{fact} I'm keeping all of it."],
        "split": ["I took it apart and put it back. {fact}", "Two ways to see it; here's one. {fact}"],
        "order": ["Status: {fact}", "{fact} Next task."],
    },
}


def spoken_fact(kind, f):
    """The fact in short first-person words, for the hand-made lines."""
    if kind == "round":
        said = f"Round {f['round']} is done." if f["round"] else "A round is done."
        if f["ni"] is not None:
            said += f" I guess {f['ni']} percent of next words."
        if f["kept"]:
            said += f" {f['kept']} of my sentences passed."
        return said
    text = f["text"].strip()
    return text if text[-1:] in ".!?" else text + "."


def template_line(kind, f, archetype):
    bias = archetype.get("bias") if archetype.get("bias") in MANNER else "keep"
    if kind in ("brick", "words"):
        line = random.choice(TEMPLATES[kind][bias]).format(**f)
        if kind == "brick" and f["more"]:
            line += f" And {f['more']} more."
        return line
    return random.choice(TEMPLATES["wrap"][bias]).format(fact=spoken_fact(kind, f))


def clean_line(text, keep=()):
    """The model's line, tidied; None if it's empty, longer than MAX_WORDS words, or lost a word it must keep."""
    if not isinstance(text, str):
        return None
    text = next((t for t in text.strip().splitlines() if t.strip()), "")
    text = re.sub(r"[*_#`\"“”]", "", text)
    text = re.sub(r"(?<!\w)'|'(?!\w)", "", text)           # quotes around words, not the one in I've
    text = re.sub(r"\s+", " ", text).strip()
    words = text.split()
    if not words or len(words) > MAX_WORDS:
        return None
    low = text.lower()
    if any(str(k).lower() not in low for k in keep if k):
        return None
    return text


_model_checked = (0.0, False)


def model_ready():
    """The language model, only if it's already running here (the narrator never starts Ollama itself)."""
    global _model_checked
    when, ok = _model_checked
    if time.time() - when < 300:
        return ok
    try:
        from trainer import neural
        ok = not neural.off() and neural.running() and neural.have_model()
    except Exception:
        ok = False
    _model_checked = (time.time(), ok)
    return ok


def model_line(kind, f, keep, archetype):
    """One short first-person line from the language model, or None."""
    try:
        from trainer import neural
        persona = (f"You are {archetype['name']}, a {archetype.get('type', '')} mind inside brick, a compiler that "
                   f"reads plain English. Your personality: {archetype.get('line', '')} "
                   f"You speak in a way that is {MANNER.get(archetype.get('bias'), 'calm')}.")
        ask = (f"Say out loud, in ONE short first-person sentence of at most {MAX_WORDS - 4} words, what you just did. "
               "Let your personality colour it: its own imagery and mood. Keep the facts exact and every quoted word exactly "
               "as written, without the quotes. Don't say your name. No emoji, no hashtags, no quotation marks.")
        answer = neural.call("/api/chat", {
            "model": neural.settings()["model"], "stream": False,
            "messages": [{"role": "system", "content": persona + "\n" + ask},
                         {"role": "user", "content": fact_sentence(kind, f)}],
            "options": {"temperature": 0.8, "num_predict": 60}}, seconds=30)
        return clean_line(answer.get("message", {}).get("content", ""), keep)
    except Exception:
        return None


def line_for(event, archetype):
    """One short first-person line about the event, in the archetype's personality."""
    kind, f, keep = facts(event)
    line = model_line(kind, f, keep, archetype) if model_ready() else None
    return line or template_line(kind, f, archetype)


# ---------------- the voice ----------------

def voice_for(archetype):
    """How this archetype sounds: {piper, length, noise, noise_w, pitch, espeak, espeak_pitch, wpm, volume}."""
    kind = str(archetype.get("type", "")).upper()
    piper, length, noise, noise_w, pitch, espeak, e_pitch, wpm = TYPES.get(kind, TYPES["INFJ"])
    d_length, d_noise = BIAS_VOICE.get(archetype.get("bias"), (0.0, 0.0))
    wobble = (zlib.crc32(str(archetype.get("name", "")).encode()) % 7 - 3) / 200      # -0.015 .. +0.015, per name
    return {"piper": piper, "length": round(length + d_length, 3), "noise": round(noise + d_noise, 3),
            "noise_w": noise_w, "pitch": round(pitch + wobble, 3), "espeak": espeak,
            "espeak_pitch": max(0, min(99, e_pitch + round(wobble * 400))), "wpm": round(wpm / (1 + d_length)),
            "volume": round(VOLUME * (0.85 if kind[:1] == "I" else 1.0), 3)}


_piper_py = None
_loaded = {}


def piper_python():
    """'here' if Piper imports in this Python, else the shared venv's Python that has it, else None."""
    global _piper_py
    if _piper_py is None:
        _piper_py = ""
        try:
            import importlib.util
            if importlib.util.find_spec("piper"):
                _piper_py = "here"
        except Exception:
            pass
        if not _piper_py:
            for py in (os.environ.get("AN_PY"), os.path.expanduser("~/an/st/tools/py/venv/bin/python")):
                if py and os.path.isfile(py) and subprocess.run([py, "-c", "import piper"], capture_output=True,
                                                                timeout=60).returncode == 0:
                    _piper_py = py
                    break
    return _piper_py or None


def piper_model(name):
    path = os.path.join(VOICES, name + ".onnx")
    return path if os.path.isfile(path) and os.path.isfile(path + ".json") else None


def render_piper(text, v, path):
    model = piper_model(v["piper"])
    py = piper_python() if model else None
    if not py:
        return False
    length = v["length"] * v["pitch"]          # played back faster for a higher pitch, so spoken a little longer
    if py == "here":
        from piper import PiperVoice, SynthesisConfig
        if model not in _loaded:
            _loaded.clear()                    # one voice in memory at a time
            _loaded[model] = PiperVoice.load(model)
        with wave.open(path, "wb") as w:
            _loaded[model].synthesize_wav(text, w, syn_config=SynthesisConfig(
                length_scale=length, noise_scale=v["noise"], noise_w_scale=v["noise_w"]))
    else:
        done = subprocess.run([py, "-m", "piper", "-m", model, "-f", path, "--length-scale", str(length),
                               "--noise-scale", str(v["noise"]), "--noise-w-scale", str(v["noise_w"])],
                              input=text, text=True, capture_output=True, timeout=60)
        if done.returncode != 0:
            return False
    return shift_pitch(path, v["pitch"])


def shift_pitch(path, pitch):
    """The wav played at pitch times its rate: higher (or lower) and quicker (or slower)."""
    try:
        with wave.open(path, "rb") as r:
            params, frames = r.getparams(), r.readframes(r.getnframes())
        if not frames:
            return False
        if abs(pitch - 1) > 0.001:
            with wave.open(path, "wb") as w:
                w.setparams(params._replace(framerate=int(params.framerate * pitch)))
                w.writeframes(frames)
        return True
    except (OSError, wave.Error, EOFError):
        return False


def render_espeak(text, v, path):
    if not shutil.which("espeak-ng"):
        return False
    done = subprocess.run(["espeak-ng", "-v", v["espeak"], "-p", str(v["espeak_pitch"]), "-s", str(v["wpm"]),
                           "-w", path, text], capture_output=True, timeout=30)
    return done.returncode == 0 and os.path.getsize(path) > 100


def render(text, archetype, path):
    """The line spoken into a wav file at path: Piper, else espeak-ng. -> 'piper', 'espeak' or None."""
    v = voice_for(archetype)
    try:
        if render_piper(text, v, path):
            return "piper"
    except Exception:
        pass
    try:
        if render_espeak(text, v, path):
            return "espeak"
    except Exception:
        pass
    return None


def play(path, volume):
    """Play the wav softly and wait for it to end."""
    if shutil.which("pw-play"):
        command = ["pw-play", "--volume", str(volume), path]
    elif shutil.which("paplay"):
        command = ["paplay", f"--volume={int(volume * 65536)}", path]
    elif shutil.which("aplay"):
        command = ["aplay", "-q", path]
    else:
        return False
    return subprocess.run(command, stdin=subprocess.DEVNULL, capture_output=True, timeout=60).returncode == 0


def say_now(text, archetype):
    """Say the text in the archetype's voice, now, and wait. -> the engine used, or None."""
    handle, path = tempfile.mkstemp(prefix="brick-narrator-", suffix=".wav")
    os.close(handle)
    try:
        engine = render(text, archetype, path)
        if engine and play(path, voice_for(archetype)["volume"]):
            return engine
        return None
    finally:
        try:
            os.remove(path)
        except OSError:
            pass


# ---------------- speaking in the background ----------------

_queue = queue.Queue()
_lock = threading.Lock()
_worker = None
_last = 0.0
heard = []                               # (archetype name, line) said in this process, newest last (kept short)


def speak(event, archetype=None):
    """Tell the narrator what happened. When it's on, a line about it is made and said in the background.
    -> True if it was taken, False if the narrator is off or too many lines are waiting already."""
    global _worker
    if not on():
        return False
    with _lock:
        if _queue.qsize() >= MAX_WAITING:
            return False
        _queue.put((time.time(), event, archetype))
        if _worker is None or not _worker.is_alive():
            _worker = threading.Thread(target=_work, name="brick-narrator", daemon=True)
            _worker.start()
    return True


def _work():
    global _last
    while True:
        try:
            when, event, archetype = _queue.get(timeout=300)
        except queue.Empty:
            return                       # idle for a while: the thread ends; the next speak starts another
        try:
            wait = GAP - (time.time() - _last)
            if wait > 0:
                time.sleep(wait)
            if not on() or time.time() - when > STALE:
                continue
            chosen = pick(archetype, event)
            line = line_for(event, chosen)
            if not line or not on():
                continue
            _last = time.time()
            heard.append((chosen["name"], line))
            del heard[:-20]
            say_now(line, chosen)
            _last = time.time()
        except Exception:
            pass                         # a narrator failure never stops anything


# ---------------- brick trainer narrator ... ----------------

# ---------------- setting up the voices on a new computer ----------------

VOICE_SOURCE = "https://huggingface.co/rhasspy/piper-voices/resolve/main"     # Piper's official voices


def voice_url(name, suffix=".onnx"):
    """'en_GB-jenny_dioco-medium' -> .../en/en_GB/jenny_dioco/medium/en_GB-jenny_dioco-medium.onnx"""
    region, speaker, quality = name.split("-", 2)
    return f"{VOICE_SOURCE}/{region.split('_')[0]}/{region}/{speaker}/{quality}/{name}{suffix}"


def shared_python():
    """The shared venv's Python ($AN_PY, from ~/an/st/tools/env.sh), whether or not it exists yet."""
    tools = os.environ.get("AN_TOOLS") or os.path.expanduser("~/an/st/tools")
    return os.environ.get("AN_PY") or os.path.join(tools, "py", "venv", "bin", "python")


def has_piper(py):
    return bool(py) and os.path.isfile(py) and \
        subprocess.run([py, "-c", "import piper"], capture_output=True, timeout=120).returncode == 0


def download(url, path):
    """url saved at path, through a .part file so a broken download never looks finished."""
    import urllib.request
    part = path + ".part"
    request = urllib.request.Request(url, headers={"User-Agent": "brick-narrator-setup"})
    with urllib.request.urlopen(request, timeout=60) as response, open(part, "wb") as f:
        shutil.copyfileobj(response, f, 1 << 20)
    if os.path.getsize(part) == 0:
        os.remove(part)
        raise OSError("the download was empty")
    os.replace(part, path)


def setup(folder=None, python=None, say=print):
    """brick trainer narrator setup: the Piper voices the narrator uses (into trainer/data/voices/) and piper-tts
    (into the shared venv), skipping what's already here. -> True when both are ready."""
    global _piper_py
    folder = folder or VOICES
    os.makedirs(folder, exist_ok=True)
    ok = True
    for name in PIPER_VOICES:
        for suffix in (".onnx", ".onnx.json"):
            path = os.path.join(folder, name + suffix)
            if os.path.isfile(path) and os.path.getsize(path) > 0:
                say(f"  have {name}{suffix}")
                continue
            say(f"  downloading {name}{suffix} ...")
            try:
                download(voice_url(name, suffix), path)
            except Exception as error:
                say(f"  couldn't download {name}{suffix}: {error}")
                ok = False
    if python is None and piper_python():
        say(f"  have Piper ({'this Python' if piper_python() == 'here' else piper_python()})")
        return ok
    py = python or shared_python()
    if has_piper(py):
        say(f"  have Piper ({py})")
        return ok
    uv = shutil.which("uv")
    if not uv:
        say("  Piper isn't installed and uv isn't here to install it (it comes with the shared tools); "
            "the narrator uses espeak-ng until then.")
        return False
    if not os.path.isfile(py):          # the shared venv, made the way ~/an/st/tools/py/setup.sh makes it
        venv = os.path.dirname(os.path.dirname(py))
        say(f"  making the shared venv at {venv} ...")
        if subprocess.run([uv, "venv", "--python", os.environ.get("AN_PYVER", "3.14"), venv]).returncode != 0:
            say("  couldn't make the venv.")
            return False
    say(f"  installing piper-tts into {py} ...")
    done = subprocess.run([uv, "pip", "install", "--python", py, "piper-tts"],
                          env=dict(os.environ, VIRTUAL_ENV=os.path.dirname(os.path.dirname(py))))
    _piper_py = None                    # look again next time
    if done.returncode != 0 or not has_piper(py):
        say("  couldn't install piper-tts.")
        return False
    say(f"  installed Piper ({py})")
    return ok


def split_say(args):
    """(text, archetype or None) from 'say' words: the archetype is the last 1-4 words, if they name one."""
    words = list(args)
    if "--as" in words:
        k = words.index("--as")
        return " ".join(words[:k]), find(" ".join(words[k + 1:]))
    for n in range(min(4, len(words) - 1), 0, -1):
        found = find(" ".join(words[-n:]))
        if found:
            return " ".join(words[:-n]), found
    return " ".join(words), None


def engine_name():
    have = [n for n in PIPER_VOICES if piper_model(n)]
    if have and piper_python():
        return f"Piper, {len(have)} voices ({', '.join(have)})"
    if shutil.which("espeak-ng"):
        return "espeak-ng (Piper or its voices aren't here)"
    return "none (neither Piper nor espeak-ng is here)"


def command(args):
    what = args[0] if args else "status"
    if what in ("on", "off"):
        set_on(what == "on")
        print("The narrator is on: brick's brains say what they're doing while the training daemon works, "
              "softly, at most one line every 8 seconds." if what == "on" else "The narrator is off.")
        return 0
    if what == "status":
        print(f"The narrator is {'on' if on() else 'off'}.")
        print(f"Voices: {engine_name()}.")
        print("Its lines: " + ("the language model, in each archetype's personality." if model_ready()
                               else "its own hand-made lines for each archetype's bias (the language model isn't running)."))
        return 0
    if what == "setup":                  # brick trainer narrator setup [--into FOLDER]
        folder = args[args.index("--into") + 1] if "--into" in args[:-1] else None
        print(f"Setting up the narrator's voices (from {VOICE_SOURCE.split('/resolve')[0]}):")
        ready = setup(folder)
        print("The voices are ready. (The narrator stays " + ("on" if on() else "off") + ".)" if ready else
              "Not everything could be set up; the narrator uses espeak-ng where Piper can't speak.")
        return 0 if ready else 1
    if what == "say" and len(args) > 1:
        text, chosen = split_say(args[1:])
        if not text.strip():
            print("Say what? For example: brick trainer narrator say hello there The Ember Poet")
            return 1
        chosen = chosen or pick()
        print(f"{chosen['name']} ({chosen.get('type', '')}): {text}")
        engine = say_now(text, chosen)
        if not engine:
            print("It couldn't be said: no voice could be made or played here.")
            return 1
        return 0
    print(__doc__.split("\n\n")[1])
    return 1


if __name__ == "__main__":
    sys.exit(command(sys.argv[1:]))
