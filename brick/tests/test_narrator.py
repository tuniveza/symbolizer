"""The narrator: off by default, silent when off, short checked lines, a voice for every archetype. No sound here:
the speech engines, the player and the language model are all stubbed."""
import time

import pytest

from trainer import narrator

ARCHETYPES = [{"name": f"The {t} {b.title()}", "type": t, "bias": b, "line": "A test personality."}
              for t in narrator.TYPES for b in ("strict", "explore", "order")]


@pytest.fixture(autouse=True)
def quiet(monkeypatch, tmp_path):
    """A switch file of the test's own, no engines, no player, no model, and no waiting."""
    monkeypatch.setattr(narrator, "SETTINGS", str(tmp_path / "narrator.json"))
    monkeypatch.delenv("BRICK_NARRATOR", raising=False)
    monkeypatch.setattr(narrator, "render", lambda *a: pytest.fail("no sound in tests"))
    monkeypatch.setattr(narrator, "play", lambda *a: pytest.fail("no sound in tests"))
    monkeypatch.setattr(narrator, "model_ready", lambda: False)
    monkeypatch.setattr(narrator, "_archetypes", list(ARCHETYPES))
    monkeypatch.setattr(narrator, "GAP", 0)
    said = []
    monkeypatch.setattr(narrator, "say_now", lambda text, archetype: said.append((archetype["name"], text)) or "stub")
    return said


def wait_for(said, count, seconds=5):
    end = time.time() + seconds
    while len(said) < count and time.time() < end:
        time.sleep(0.02)
    return said


def test_switch_is_off_by_default():
    assert narrator.on() is False
    assert narrator.settings() == {"on": False}


def test_switch_turns_on_and_off():
    narrator.set_on(True)
    assert narrator.on() is True
    narrator.set_on(False)
    assert narrator.on() is False


def test_environment_keeps_it_off(monkeypatch):
    narrator.set_on(True)
    monkeypatch.setenv("BRICK_NARRATOR", "off")
    assert narrator.on() is False


def test_speak_does_nothing_when_off(quiet):
    assert narrator.speak({"kind": "bricks", "lines": ["carmine = crimson"]}) is False
    time.sleep(0.1)
    assert quiet == []
    assert narrator._queue.qsize() == 0


def test_speak_says_a_line_in_the_background_when_on(quiet):
    narrator.set_on(True)
    assert narrator.speak({"kind": "bricks", "lines": ["carmine = crimson"]}, "The INFP Explore") is True
    wait_for(quiet, 1)
    assert quiet and quiet[0][0] == "The INFP Explore"
    assert "carmine" in quiet[0][1] and "crimson" in quiet[0][1]
    narrator.set_on(False)


def test_never_more_than_three_waiting(monkeypatch):
    monkeypatch.setattr(narrator, "_worker", None)
    monkeypatch.setattr(narrator, "_queue", narrator.queue.Queue())
    monkeypatch.setattr(narrator.threading, "Thread", lambda **k: type("T", (), {"start": lambda s: None,
                                                                                 "is_alive": lambda s: True})())
    narrator.set_on(True)
    taken = [narrator.speak(f"line {k}") for k in range(6)]
    assert taken == [True, True, True, False, False, False]
    narrator.set_on(False)                       # switching off drops what's waiting
    assert narrator._queue.qsize() == 0


def test_line_length_and_empty_rejection():
    assert narrator.clean_line("") is None
    assert narrator.clean_line("   \n  ") is None
    assert narrator.clean_line(None) is None
    assert narrator.clean_line(" ".join(["word"] * 21)) is None
    assert narrator.clean_line(" ".join(["word"] * 20)) == " ".join(["word"] * 20)
    assert narrator.clean_line('"A new word drifted in: cls."\nSecond line') == "A new word drifted in: cls."
    assert narrator.clean_line("I made a new brick.", keep=["carmine"]) is None
    assert narrator.clean_line("Carmine means crimson now.", keep=["carmine", "crimson"]) == "Carmine means crimson now."


def test_model_line_used_when_good_else_template(monkeypatch):
    archetype = ARCHETYPES[0]
    monkeypatch.setattr(narrator, "model_ready", lambda: True)
    monkeypatch.setattr(narrator, "model_line", lambda *a: "Carmine now means crimson, I checked.")
    assert narrator.line_for({"kind": "bricks", "lines": ["carmine = crimson"]}, archetype) == \
        "Carmine now means crimson, I checked."
    monkeypatch.setattr(narrator, "model_line", lambda *a: None)          # refused: too long, empty or lost a word
    line = narrator.line_for({"kind": "bricks", "lines": ["carmine = crimson"]}, archetype)
    assert "carmine" in line and "crimson" in line


def test_model_reply_is_checked(monkeypatch):
    from trainer import neural
    archetype = ARCHETYPES[0]
    kind, f, keep = narrator.facts({"kind": "bricks", "lines": ["carmine = crimson"]})
    for reply, wanted in [("Carmine is crimson now.", "Carmine is crimson now."),
                          (" ".join(["very"] * 30) + " carmine crimson", None), ("", None)]:
        monkeypatch.setattr(neural, "call", lambda path, body, seconds=60, r=reply: {"message": {"content": r}})
        monkeypatch.setattr(neural, "settings", lambda: {"model": "stub", "on": True})
        assert narrator.model_line(kind, f, keep, archetype) == wanted


def test_templates_for_every_bias_and_event():
    events = [{"kind": "bricks", "lines": ["cls = clear the screen"]},
              {"kind": "words", "words": ["ecru", "cerise"], "defined": ["ecru"]},
              {"kind": "round", "round": 10, "text": "round 1: 2197 neurons ..., Ni guesses 35% of next words; "
                                                      "checked 24 of Ne's sentences: 2 kept, 0 cut"},
              "something happened here"]
    for bias in narrator.MANNER:
        for event in events:
            kind, f, _ = narrator.facts(event)
            line = narrator.template_line(kind, f, {"name": "x", "type": "INFJ", "bias": bias})
            assert line and "{" not in line and len(line.split()) <= 25
    kind, f, _ = narrator.facts(events[2])
    assert f == {"round": 10, "ni": 35, "kept": 2}


def test_log_lines_become_events():
    assert narrator.facts("new brick: carmine = crimson")[:2] == ("brick", {"word": "carmine", "means": "crimson",
                                                                             "more": 0})
    assert narrator.facts("round 1: 2197 neurons, Ni guesses 35% of next words")[0] == "round"


def test_voice_mapping_covers_all_48_archetypes():
    try:
        from trainer import neurons
        real = [a for a in neurons.archetypes() if isinstance(a, dict) and a.get("name")]
    except Exception:
        real = []
    every = real or ARCHETYPES
    assert len(every) == 48
    assert {a["type"] for a in every} <= set(narrator.TYPES)
    sounds = set()
    for a in every:
        v = narrator.voice_for(a)
        assert v["piper"] in narrator.PIPER_VOICES
        assert 0.75 <= v["length"] <= 1.35 and 0.3 <= v["noise"] <= 0.95 and 0.9 <= v["pitch"] <= 1.1
        assert 0 <= v["espeak_pitch"] <= 99 and 100 <= v["wpm"] <= 220 and 0 < v["volume"] <= 0.5
        sounds.add((v["piper"], v["length"], v["noise"], v["pitch"]))
    assert len(sounds) == len(every)                     # every archetype sounds a little different
    types = {t: narrator.voice_for({"type": t, "bias": "keep", "name": "same"}) for t in narrator.TYPES}
    assert len({(v["piper"], v["length"], v["noise"], v["pitch"]) for v in types.values()}) == 16
    assert types["ENTJ"]["length"] == min(v["length"] for v in types.values())          # ENTJ the briskest
    assert all(types[t]["volume"] < types["E" + t[1:]]["volume"] for t in types if t[0] == "I")


def test_round_robin_and_named_archetype():
    first, second = narrator.pick(), narrator.pick()
    assert first != second
    assert narrator.pick("infp explore")["name"] == "The INFP Explore"
    assert narrator.split_say(["hello", "there", "The", "INFP", "Explore"]) == ("hello there", ARCHETYPES[4])
    assert narrator.split_say(["hello there"]) == ("hello there", None)


def test_failures_never_escape_the_worker(monkeypatch, quiet):
    monkeypatch.setattr(narrator, "line_for", lambda *a: 1 / 0)
    narrator.set_on(True)
    assert narrator.speak("anything") is True
    time.sleep(0.2)
    assert quiet == []
    narrator.set_on(False)
