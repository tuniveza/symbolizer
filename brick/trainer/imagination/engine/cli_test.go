package engine

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// The drop-in check: run.js (node) and RunJob (Go) get the same jobs and must answer the same, byte for byte,
// and leave byte-identical network.json files. Only the field run.js fills from the clock ("ms") is left out.

var volatile = regexp.MustCompile(`,"ms":-?[0-9.e+-]+`)

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
	omegas := map[string]float64{"display": 3.25, "colour": 9.5, "ruby": 2.75, "#5": 1.0, "=write": 4.0, "nosuchword": 6}
	learn := map[string]any{"task": "learn", "data": string(data), "roles": roles, "steps": steps, "candidates": 24, "batch": 3, "seed": 2, "omegas": omegas}
	runBoth(t, "learn (fresh)", learn, sjs, sgo)
	learn2 := map[string]any{"task": "learn", "data": string(data), "roles": roles, "steps": steps, "candidates": 24, "batch": 3, "seed": 3,
		"verdicts": []map[string]any{{"a": "display", "b": "write", "ok": true, "level": "meaning", "why": "same brick"},
			{"a": "ruby", "b": "score", "ok": false}, {"a": "add", "b": "echo", "ok": nil},
			{"a": "display", "b": "display", "ok": true}, {"a": "score", "b": "score", "ok": false}}} // a word and itself: skipped (crashed node before)
	runBoth(t, "learn (saved, verdicts)", learn2, sjs, sgo)
	more := string(data) + "\nquibblefrost and zentangle wobble\n"
	runBoth(t, "learn (saved, new words with omegas)", map[string]any{"task": "learn", "data": more, "steps": steps / 3, "seed": 6,
		"omegas": map[string]float64{"quibblefrost": 5.5, "zentangle": 0.25}}, sjs, sgo)
	// a saved line from a neuron to itself (crashed node's load before): skipped
	if b, err := os.ReadFile(sjs); err == nil {
		st := map[string]any{}
		json.Unmarshal(b, &st)
		st["links"] = append(st["links"].([]any), []any{"display", "display", 0.5, 0.5, nil, 0, 0})
		nb, _ := json.Marshal(st)
		os.WriteFile(sjs, nb, 0o644)
		os.WriteFile(sgo, nb, 0o644)
		runBoth(t, "learn (saved line to itself)", map[string]any{"task": "learn", "data": string(data), "steps": steps / 3, "seed": 7}, sjs, sgo)
	}
	runBoth(t, "readline", map[string]any{"task": "readline", "words": []string{"display", "the", "score", "colour", "ruby", "zzz", "display"}}, sjs, sgo)
	runBoth(t, "readline (archetype)", map[string]any{"task": "readline", "brain": "Stone Keeper", "words": []string{"add", "score", "times", "1"}}, sjs, sgo)
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
		runBoth(t, "readline (brick's network)", map[string]any{"task": "readline", "words": strings.Fields("put window left side screen place text brown top right")}, sjs, sgo)
		runBoth(t, "learn (brick's network)", map[string]any{"task": "learn", "data": string(data), "roles": roles, "steps": steps / 3, "candidates": 24, "batch": 3, "seed": 5}, sjs, sgo)
	} else {
		t.Log("no saved network at " + path + "; set BRICK_NETWORK to include it")
	}
}

// --serve: several jobs through one long-lived process each (node run.js --serve and imagine --serve), one answer
// per line; the network is read from its file for every job, so the learn job's second round sees the first's.
func TestServeEquivalence(t *testing.T) {
	node := needNode(t)
	dir := t.TempDir()
	bin := filepath.Join(dir, "imagine")
	if out, err := exec.Command("go", "build", "-o", bin, "./cmd/imagine").CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}
	data, _ := os.ReadFile("testdata/sentences.txt")
	jobs := func(state string) []byte {
		var b bytes.Buffer
		for _, j := range []map[string]any{
			{"list": true},
			{"task": "learn", "state": state, "data": string(data), "steps": 100, "batch": 2, "seed": 2, "omegas": map[string]float64{"display": 3.5}},
			{"task": "learn", "state": state, "data": string(data), "steps": 100, "batch": 2, "seed": 3},
			{"task": "readline", "state": state, "words": []string{"display", "score", "colour"}},
			{"task": "bricks", "state": state, "words": []string{"display"}},
			{"task": "readline", "state": filepath.Join(dir, "missing.json"), "words": []string{"x"}},
		} {
			line, _ := json.Marshal(j)
			b.Write(line)
			b.WriteString("\n\n") // a blank line is skipped
		}
		return b.Bytes()
	}
	run := func(name string, args ...string) []byte {
		cmd := exec.Command(name, args...)
		state := filepath.Join(dir, filepath.Base(name)+".json")
		cmd.Stdin = bytes.NewReader(jobs(state))
		out, err := cmd.Output()
		if err != nil {
			t.Fatalf("%s --serve: %v", name, err)
		}
		return bytes.ReplaceAll(out, []byte(state), []byte("STATE"))
	}
	js := strings.Split(string(volatile.ReplaceAll(run(node, "../run.js", "--serve"), nil)), "\n")
	gob := strings.Split(string(volatile.ReplaceAll(run(bin, "--serve"), nil)), "\n")
	if len(js) != len(gob) {
		t.Fatalf("--serve: node gave %d lines, Go %d", len(js), len(gob))
	}
	for i := range js {
		if strings.HasPrefix(js[i], `{"error":`) && strings.HasPrefix(gob[i], `{"error":`) {
			continue // both failed (the messages differ: a JS stack vs a Go error)
		}
		if js[i] != gob[i] {
			t.Fatalf("--serve answer %d differs\n js: %.300s\n go: %.300s", i, js[i], gob[i])
		}
	}
	a, _ := os.ReadFile(filepath.Join(dir, "node.json"))
	b, _ := os.ReadFile(filepath.Join(dir, "imagine.json"))
	if !bytes.Equal(a, b) {
		t.Fatal("--serve: the saved networks differ")
	}
	t.Logf("--serve: %d answers identical, saved networks identical", len(js)-1)
}
