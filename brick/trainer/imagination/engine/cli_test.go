package engine

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"testing"
)

// The drop-in check: run.js (node) and RunJob (Go) get the same jobs and must answer the same, byte for byte,
// and leave byte-identical network.json files. Only the fields run.js fills from the clock ("ms") or from
// Math.random() (score's "chance") are left out of the comparison.

var volatile = regexp.MustCompile(`,"(ms|chance)":-?[0-9.e+-]+`)

func runBoth(t *testing.T, name string, job map[string]any, stateJS, stateGo string) {
	node := needNode(t)
	mk := func(state string) []byte {
		j := map[string]any{}
		for k, v := range job {
			j[k] = v
		}
		if state != "" {
			j["state"] = state
		}
		b, _ := json.Marshal(j)
		return b
	}
	js := runNode(t, node, "../run.js", mk(stateJS))
	gob, err := RunJob(mk(stateGo))
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	js, gob = volatile.ReplaceAll(js, nil), volatile.ReplaceAll(gob, nil)
	if !bytes.Equal(js, gob) {
		k := 0
		for k < len(js) && k < len(gob) && js[k] == gob[k] {
			k++
		}
		lo := max(0, k-200)
		t.Fatalf("%s: output differs at byte %d\n js: ...%s\n go: ...%s", name, k, js[lo:min(len(js), k+200)], gob[lo:min(len(gob), k+200)])
	}
	if stateJS != "" {
		a, errA := os.ReadFile(stateJS)
		b, errB := os.ReadFile(stateGo)
		if (errA == nil) != (errB == nil) {
			t.Fatalf("%s: only one engine wrote its state file", name)
		}
		if errA == nil && !bytes.Equal(a, b) {
			compareOutputs(t, name+" state", append([]byte("\n\n\n"), a...), append([]byte("\n\n\n"), b...))
		}
	}
	t.Logf("%s: identical (%d bytes)", name, len(js))
}

func TestCLIEquivalence(t *testing.T) {
	needNode(t)
	dir := t.TempDir()
	sjs, sgo := filepath.Join(dir, "js.json"), filepath.Join(dir, "go.json")
	data, _ := os.ReadFile("testdata/sentences.txt")
	steps := 600
	if testing.Short() {
		steps = 150
	}
	roles := map[string]string{"display": "do", "write": "do", "add": "do", "colour": "thing", "ruby": "colour", "times": "count", "score": "thing", "1": "count"}

	runBoth(t, "list", map[string]any{"list": true}, "", "")
	runBoth(t, "unknown brain", map[string]any{"task": "learn", "brain": "The Nobody"}, "", "")

	// a fresh network grown from the sentences, then a second round that continues the saved one with verdicts
	learn := map[string]any{"task": "learn", "data": string(data), "roles": roles, "steps": steps, "candidates": 24, "batch": 3, "seed": 2}
	runBoth(t, "learn (fresh)", learn, sjs, sgo)
	learn2 := map[string]any{"task": "learn", "data": string(data), "roles": roles, "steps": steps, "candidates": 24, "batch": 3, "seed": 3,
		"verdicts": []map[string]any{{"a": "display", "b": "write", "ok": true, "level": "meaning", "why": "same brick"},
			{"a": "ruby", "b": "score", "ok": false}, {"a": "add", "b": "echo", "ok": nil}}}
	runBoth(t, "learn (saved, verdicts)", learn2, sjs, sgo)
	runBoth(t, "learn (archetype)", map[string]any{"task": "learn", "data": string(data), "steps": steps / 2, "brain": "Mirror Oracle", "seed": 4}, sjs, sgo)

	runBoth(t, "bricks", map[string]any{"task": "bricks", "words": []string{"display", "write", "colour", "nothing", "1"}}, sjs, sgo)
	runBoth(t, "score", map[string]any{"task": "score", "groups": map[string][]string{"do": {"display", "write", "add", "echo"}, "things": {"score", "colour", "name"}}}, sjs, sgo)
	runBoth(t, "read (saved)", map[string]any{"data": string(data), "questions": "display @ the score\ncolour @ ruby\nzzz @ display", "steps": 100}, sjs, sgo)
	runBoth(t, "read (fresh, groups)", map[string]any{"data": SampleData, "questions": SampleQuestions, "steps": steps}, "", "")
	runBoth(t, "read (fresh, brain)", map[string]any{"data": SampleData, "questions": SampleQuestions, "steps": steps, "brain": "Iron Conductor", "mode": "use"}, "", "")

	// brick's own saved network (BRICK_NETWORK or the usual place), one learn round on a copy
	path := os.Getenv("BRICK_NETWORK")
	if path == "" {
		path = filepath.Join("..", "..", "data", "neurons", "network.json")
	}
	if b, err := os.ReadFile(path); err == nil {
		os.WriteFile(sjs, b, 0o644)
		os.WriteFile(sgo, b, 0o644)
		runBoth(t, "learn (brick's network)", map[string]any{"task": "learn", "data": string(data), "roles": roles, "steps": steps / 3, "candidates": 24, "batch": 3, "seed": 5}, sjs, sgo)
	} else {
		t.Log("no saved network at " + path + "; set BRICK_NETWORK to include it")
	}
}
