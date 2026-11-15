package engine

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// equivCase is one run, done by both engines (testdata/equiv.js and runCase) and compared byte for byte.
type equivCase struct {
	Kind       string             `json:"kind"`
	Gap        float64            `json:"gap,omitempty"`
	Data       string             `json:"data,omitempty"`
	Mode       string             `json:"mode,omitempty"`
	Seed       float64            `json:"seed,omitempty"`
	Brain      string             `json:"brain,omitempty"`
	Intuition  bool               `json:"intuition,omitempty"`
	Batch      float64            `json:"batch,omitempty"`
	Ticks      int                `json:"ticks,omitempty"`
	Questions  string             `json:"questions,omitempty"`
	State      string             `json:"state,omitempty"`
	Candidates float64            `json:"candidates,omitempty"`
	Bricks     []string           `json:"bricks,omitempty"`
	Readline   []string           `json:"readline,omitempty"`
	Omegas     map[string]float64 `json:"omegas,omitempty"`
}

func runCase(t testing.TB, c equivCase) []byte {
	seed := c.Seed
	if seed == 0 {
		seed = 1
	}
	var brain *Archetype
	if c.Brain != "" {
		brain = FindArchetype(c.Brain)
	}
	var net *Network
	switch c.Kind {
	case "word":
		net = NewNetwork(WordSpec(c.Gap), seed, nil, brain, c.Intuition)
	case "data":
		net = NewNetwork(DataSpec(c.Data, c.Mode, seed), seed, nil, brain, c.Intuition)
	case "load":
		b, err := os.ReadFile(c.State)
		if err != nil {
			t.Fatal(err)
		}
		st, err := ParseJSON(b)
		if err != nil {
			t.Fatal(err)
		}
		net = NewNetwork(&Spec{}, seed, nil, brain, true).Load(st)
		spec := DataSpec(c.Data, "raw", seed)
		om, _ := json.Marshal(c.Omegas)
		if c.Omegas == nil {
			om = []byte("{}")
		}
		net.Encoded, _ = ParseJSON(om)
		net.Sentences = spec.Sentences
		var all []string
		for _, s := range spec.Sentences {
			all = append(all, s.W...)
		}
		net.AddWords(uniqueWords(all))
	}
	if c.Batch != 0 {
		net.Batch = c.Batch
	}
	for k := 0; k < c.Ticks; k++ {
		net.Tick()
	}
	w := &jw{}
	w.raw("[")
	for i, q := range ParseQuestions(c.Questions) {
		if i > 0 {
			w.raw(",")
		}
		for k := 0; k < 30; k++ {
			net.Tick()
		}
		r := net.Read(q.Word, q.Context, 40)
		if r == nil {
			w.raw(`{"word":`)
			w.str(q.Word)
			w.raw(`,"found":false}`)
			continue
		}
		WriteAnswer(w, net, q.Word, r)
	}
	w.raw("]\n[")
	if c.Candidates != 0 {
		for i, cd := range net.Candidates(c.Candidates) {
			if i > 0 {
				w.raw(",")
			}
			w.raw(`{"a":`)
			w.str(cd.A)
			w.raw(`,"b":`)
			w.str(cd.B)
			w.raw(`,"alike":`)
			w.num(cd.Alike)
			w.raw(`,"original":`)
			w.str(cd.Original)
			w.raw(`,"swapped":`)
			w.str(cd.Swapped)
			w.raw("}")
		}
	}
	w.raw("]\n")
	st := net.Stats()
	w.raw(`{"warmth":`)
	w.num(net.Warmth)
	w.raw(`,"orderPull":`)
	w.num(net.OrderPull)
	w.raw(`,"steps":`)
	w.num(net.Steps)
	w.raw(`,"t":`)
	w.num(net.T)
	w.raw(`,"log":[`)
	for i, l := range net.Log {
		if i > 0 {
			w.raw(",")
		}
		w.str(l.Text)
	}
	w.raw(`],"stats":{"order":`)
	w.num(st.Order)
	w.raw(`,"heat":`)
	w.num(st.Heat)
	for _, kv := range []struct {
		k string
		v int
	}{{"hot", st.Hot}, {"cold", st.Cold}, {"neurons", st.Neurons}, {"links", st.Links}, {"lit", st.Lit}} {
		w.raw(`,"` + kv.k + `":`)
		w.num(float64(kv.v))
	}
	w.raw(`},"bricks":[`)
	for i, word := range c.Bricks {
		if i > 0 {
			w.raw(",")
		}
		g := net.BrickOf(word)
		if g == nil {
			w.raw("null")
			continue
		}
		w.raw(`{"output":`)
		w.str(g.Output)
		w.raw(`,"share":`)
		w.num(g.Share)
		w.raw(`,"second":`)
		if g.HasSecond {
			w.str(g.Second)
		} else {
			w.raw("null")
		}
		w.raw("}")
	}
	w.raw(`],"readline":[`)
	if c.Readline != nil {
		for i, a := range net.ReadLine(c.Readline, 40) {
			if i > 0 {
				w.raw(",")
			}
			w.raw(`{"word":`)
			w.str(a.Word)
			r := a.Result
			if r == nil {
				w.raw(`,"found":false}`)
				continue
			}
			best := r.Ranked[0]
			role := net.RoleOf(best.S)
			w.raw(`,"verdict":`)
			w.str(r.Verdict)
			w.raw(`,"clear":`)
			w.boolean(r.Clear)
			w.raw(`,"meaning":`)
			w.ostrVal(r.Meaning)
			w.raw(`,"sense":`)
			w.str(best.S.Label)
			w.raw(`,"matched":`)
			w.num(float64(best.Matched))
			w.raw(`,"carried":`)
			w.num(float64(r.Carried))
			w.raw(`,"role":`)
			if role != nil {
				w.str(role.Role)
			} else {
				w.raw("null")
			}
			w.raw(`,"senses":[`)
			for k, x := range r.Ranked {
				if k > 0 {
					w.raw(",")
				}
				w.raw(`{"label":`)
				w.str(x.S.Label)
				w.raw(`,"matched":`)
				w.num(float64(x.Matched))
				w.raw(`,"profile":{`)
				for q, p := range x.Profile {
					if q > 0 {
						w.raw(",")
					}
					w.key(p.Key)
					w.num(p.Pct)
				}
				w.raw("}}")
			}
			w.raw("]}")
		}
	}
	w.raw("]}\n")
	w.b = append(w.b, net.Save()...)
	w.raw("\n")
	return w.b
}

func runNode(t testing.TB, node, script string, input []byte) []byte {
	cmd := exec.Command(node, script)
	cmd.Stdin = bytes.NewReader(input)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("node %s: %v\n%s", script, err, stderr.String())
	}
	return out
}

// compareOutputs fails with the first difference, named as precisely as it can (which line, which neuron).
func compareOutputs(t testing.TB, name string, js, gob []byte) {
	if bytes.Equal(js, gob) {
		return
	}
	jl := strings.Split(string(js), "\n")
	gl := strings.Split(string(gob), "\n")
	parts := []string{"answers", "candidates", "extra", "save"}
	for i := 0; i < len(jl) && i < len(gl); i++ {
		if jl[i] == gl[i] {
			continue
		}
		part := "line " + strconv.Itoa(i)
		if i < len(parts) {
			part = parts[i]
		}
		a, b := jl[i], gl[i]
		k := 0
		for k < len(a) && k < len(b) && a[k] == b[k] {
			k++
		}
		lo := max(0, k-300)
		where := ""
		if part == "save" {
			// name the first neuron that differs
			var ja, ga struct {
				Neurons []json.RawMessage `json:"neurons"`
			}
			json.Unmarshal([]byte(a), &ja)
			json.Unmarshal([]byte(b), &ga)
			for n := 0; n < len(ja.Neurons) && n < len(ga.Neurons); n++ {
				if !bytes.Equal(ja.Neurons[n], ga.Neurons[n]) {
					where = fmt.Sprintf("\nfirst differing neuron #%d:\n js: %.600s\n go: %.600s", n, ja.Neurons[n], ga.Neurons[n])
					break
				}
			}
		}
		t.Fatalf("%s: %s differs at byte %d\n js: ...%s\n go: ...%s%s", name, part, k, a[lo:min(len(a), k+200)], b[lo:min(len(b), k+200)], where)
	}
	t.Fatalf("%s: outputs differ in length (%d vs %d lines)", name, len(jl), len(gl))
}

func equiv(t *testing.T, name string, c equivCase) {
	node := needNode(t)
	in, _ := json.Marshal(c)
	js := runNode(t, node, "testdata/equiv.js", in)
	gob := runCase(t, c)
	compareOutputs(t, name, js, gob)
	t.Logf("%s: identical (%d bytes); %s", name, len(js), summary(js))
}

// summary says what a run went through, so a passing test shows it exercised the engine.
func summary(out []byte) string {
	lines := strings.Split(string(out), "\n")
	if len(lines) < 4 {
		return ""
	}
	var st struct {
		Neurons    []struct{ Label string }
		Links      [][]any
		Phrases    []string
		SenseCount map[string]float64
		Stats      map[string]float64
	}
	json.Unmarshal([]byte(lines[3]), &st)
	var ex struct{ Log []string }
	json.Unmarshal([]byte(lines[2]), &ex)
	count := func(sub string) int {
		c := 0
		for _, l := range ex.Log {
			if strings.Contains(l, sub) {
				c++
			}
		}
		return c
	}
	tent := 0
	for _, l := range st.Links {
		if len(l) > 4 && l[4] != nil {
			tent++
		}
	}
	return fmt.Sprintf("neurons %d, links %d (tentative %d), phrases %d, split words %d, log: fission %d, fused %d, kept %d, faded %d, radiant %d; stats %v",
		len(st.Neurons), len(st.Links), tent, len(st.Phrases), len(st.SenseCount), count("fission"), count("fused"), count("Te kept"), count("faded"), count("radiant"), st.Stats)
}

// genData makes a raw training text with a vocabulary of about n words: sentences of 3 to 7 words, some with
// what brick built (|| =...), some with flip and order words and numbers, so every part of the engine runs.
func genData(n, lines int, seed uint32) string {
	s := seed
	next := func() uint32 { s = s*1664525 + 1013904223; return s >> 8 }
	outs := []string{"=say", "=wait", "=clear", "=move", "=ask", "=colour"}
	extra := []string{"then", "not", "don't", "after", "#3", "#250", "#~7", "the", "a", "and"}
	var b strings.Builder
	b.WriteString("# generated\ngroup g1: w1, w2, w3\n")
	for i := 0; i < lines; i++ {
		k := 3 + int(next()%5)
		for j := 0; j < k; j++ {
			if j > 0 {
				b.WriteByte(' ')
			}
			if next()%6 == 0 {
				b.WriteString(extra[next()%uint32(len(extra))])
			} else {
				// skewed: low numbers more often, so some words are seen often enough to fuse and split
				r := next() % uint32(n)
				r = r * (next() % uint32(n)) / uint32(n)
				b.WriteString("w" + strconv.Itoa(int(r)))
			}
		}
		if next()%3 == 0 {
			b.WriteString(" || " + outs[next()%uint32(len(outs))])
			if next()%2 == 0 {
				b.WriteString(" " + outs[next()%uint32(len(outs))])
			}
		}
		b.WriteByte('\n')
	}
	return b.String()
}

func ticks(full int) int {
	if testing.Short() {
		return full / 6
	}
	return full
}

// (a) the word preset, neutral, seed 1: training, then reading.
func TestEquivWordSpec(t *testing.T) {
	equiv(t, "wordSpec", equivCase{Kind: "word", Gap: 1.2, Seed: 1, Ticks: ticks(3000), Questions: SampleQuestions})
}

// (b) raw sample data, learning by itself (intuition), batch 5.
func TestEquivRawIntuition(t *testing.T) {
	equiv(t, "raw", equivCase{Kind: "data", Data: SampleData, Mode: "raw", Seed: 1, Intuition: true, Batch: 5, Ticks: ticks(3000), Candidates: 20})
}

// (c) a ~300-neuron raw network: the frequency-window path of step(), phrases, splits, bitstream neurons.
func TestEquivBigRaw(t *testing.T) {
	data := genData(260, 500, 7)
	equiv(t, "bigRaw", equivCase{Kind: "data", Data: data, Mode: "raw", Seed: 3, Intuition: true, Batch: 3, Ticks: ticks(1200),
		Questions: "w5 @ w1 w2\nw9 @ w3 then\nw40 @ w7", Candidates: 24, Bricks: []string{"w1", "w2", "w5", "then", "nothing"}})
}

// (c') brick's own saved network, if there is one (BRICK_NETWORK, or the usual place), for 200 steps.
func TestEquivSavedNetwork(t *testing.T) {
	path := os.Getenv("BRICK_NETWORK")
	if path == "" {
		path = filepath.Join("..", "..", "data", "neurons", "network.json")
	}
	if _, err := os.Stat(path); err != nil {
		t.Skip("no saved network at " + path + " (set BRICK_NETWORK to a network.json)")
	}
	tmp := filepath.Join(t.TempDir(), "network.json")
	b, _ := os.ReadFile(path)
	os.WriteFile(tmp, b, 0o644)
	data, _ := os.ReadFile("testdata/sentences.txt")
	equiv(t, "savedNetwork", equivCase{Kind: "load", State: tmp, Data: string(data), Seed: 1, Batch: 3, Ticks: ticks(200), Candidates: 24})
}

// (d) an archetype brain: The Mirror Oracle, groups given.
func TestEquivMirrorOracle(t *testing.T) {
	equiv(t, "mirrorOracle", equivCase{Kind: "data", Data: SampleData, Mode: "groups", Seed: 1, Brain: "The Mirror Oracle", Ticks: ticks(3000), Questions: SampleQuestions})
}

// More archetypes, short runs: different energies and rules exercise steer() and the other branches.
func TestEquivArchetypes(t *testing.T) {
	for _, name := range []string{"The Sensory Mythweaver", "The Sovereign Engine", "The Stone Keeper", "The Wild Gardener"} {
		equiv(t, name, equivCase{Kind: "data", Data: SampleData, Mode: "raw", Seed: 2, Brain: name, Intuition: true, Batch: 2, Ticks: ticks(1500), Questions: "crap @ clear\ntest @ check", Candidates: 10})
	}
}

// A random field and the "use" encoder.
func TestEquivFieldAndUse(t *testing.T) {
	equiv(t, "use", equivCase{Kind: "data", Data: SampleData, Mode: "use", Seed: 4, Ticks: ticks(1000), Questions: SampleQuestions})
	equiv(t, "random", equivCase{Kind: "data", Data: SampleData, Mode: "random", Seed: 5, Ticks: ticks(1000), Questions: SampleQuestions})
}

// readLine: a whole line in one run (small network: all-pairs pool; the 260-neuron one: the filtered pool).
func TestEquivReadLine(t *testing.T) {
	equiv(t, "readLine small", equivCase{Kind: "word", Gap: 1.2, Seed: 1, Ticks: ticks(600), Readline: []string{"crap", "clear", "write", "test", "check", "crap", "nothing"}})
	equiv(t, "readLine big", equivCase{Kind: "data", Data: genData(260, 500, 7), Mode: "raw", Seed: 3, Intuition: true, Batch: 3, Ticks: ticks(600),
		Readline: []string{"w5", "w1", "w2", "then", "w9", "w40", "#3", "zzz"}})
}

// readLine and omegas on brick's own saved network (BRICK_NETWORK or the usual place).
func TestEquivSavedReadLine(t *testing.T) {
	path := os.Getenv("BRICK_NETWORK")
	if path == "" {
		path = filepath.Join("..", "..", "data", "neurons", "network.json")
	}
	if _, err := os.Stat(path); err != nil {
		t.Skip("no saved network at " + path + " (set BRICK_NETWORK to a network.json)")
	}
	data := "put the window on the left side of the screen\nplace brown text at the top right || =write\nzorbleword and flimflam appear"
	equiv(t, "saved readLine + omegas", equivCase{Kind: "load", State: path, Data: data, Seed: 1, Batch: 3, Ticks: ticks(60),
		Omegas:   map[string]float64{"zorbleword": 7.25, "flimflam": 3.5, "window": 99},
		Readline: strings.Fields("put window left side screen place text brown top right")})
}
