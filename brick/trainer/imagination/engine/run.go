package engine

// RunJob is run.js: one JSON job in, one JSON answer out (the same contract, so brick can call either).

import (
	"errors"
	"math"
	"os"
	"sort"
	"time"
)

// Clock lets tests pin the timing fields ("ms") that run.js fills from Date.now().
var Clock = func() time.Time { return time.Now() }

type jobRunner struct {
	job   *JV
	brain *Archetype
	seed  float64
}

func (j *jobRunner) network(withState bool) (*Network, bool, error) {
	var saved *JV
	if withState {
		if p := j.job.Get("state"); p.Truthy() {
			if b, err := os.ReadFile(p.String()); err == nil {
				v, err := ParseJSON(b)
				if err != nil {
					return nil, false, err
				}
				saved = v
			} else if !os.IsNotExist(err) {
				return nil, false, err
			}
		}
	}
	data := ""
	if d := j.job.Get("data"); d.Truthy() {
		data = d.String()
	}
	spec := DataSpec(data, "raw", j.seed)
	enc := j.job.Get("omegas") // the encoder's starting frequencies (trainer/encoder.py)
	if enc == nil || !enc.Truthy() || enc.Kind != JObj {
		enc = &JV{Kind: JObj}
	}
	for i := range spec.Neurons {
		n := &spec.Neurons[i]
		if v := enc.Get(n.Label); !v.IsNullish() && !n.Fixed {
			n.Omega = v.Num()
		}
	}
	var net *Network
	if saved != nil {
		net = NewNetwork(&Spec{}, j.seed, nil, j.brain, true)
	} else {
		net = NewNetwork(spec, j.seed, nil, j.brain, true)
	}
	net.Encoded = enc
	if saved != nil {
		net.Load(saved)
	}
	net.Sentences = spec.Sentences
	var all []string
	for _, s := range spec.Sentences {
		all = append(all, s.W...)
	}
	net.AddWords(uniqueWords(all))
	roles := j.job.Get("roles")
	if roles != nil && roles.Kind == JObj {
		for i, w := range roles.Keys {
			for _, n := range net.N {
				if n.Word == w {
					n.Role = ""
					if r := roles.Vals[i]; r.Truthy() {
						n.Role = r.String()
					}
				}
			}
		}
	}
	if roles.Truthy() {
		var vals []string
		if roles.Kind == JObj {
			for _, v := range roles.Vals {
				vals = append(vals, v.String())
			}
		}
		net.Slots(vals)
	}
	return net, saved == nil, nil
}

func numOr(v *JV, def float64) float64 { // `v || def`
	if v.Truthy() {
		return v.Num()
	}
	return def
}

func splitWords(net *Network) []string {
	var out []string
	seen := map[string]bool{}
	for _, n := range net.N {
		if containsMiddot(n.Label) && !seen[n.Word] {
			seen[n.Word] = true
			out = append(out, n.Word)
		}
	}
	return out
}

func containsMiddot(s string) bool {
	for _, r := range s {
		if r == '·' {
			return true
		}
	}
	return false
}

// RunJob runs one job (run.js's stdin) and returns run.js's stdout.
func RunJob(input []byte) ([]byte, error) {
	job, err := ParseJSON(input)
	if err != nil {
		return nil, err
	}
	w := &jw{}
	if job.Get("list").Truthy() {
		w.raw("[")
		for i, a := range Archetypes {
			if i > 0 {
				w.raw(",")
			}
			w.raw(`{"name":`)
			w.str(a.Name)
			w.raw(`,"type":`)
			w.str(a.Type)
			w.raw(`,"bias":`)
			w.str(a.Bias)
			w.raw(`,"yours":`)
			w.boolean(a.Yours)
			w.raw(`,"line":`)
			w.str(a.Line)
			w.raw("}")
		}
		w.raw("]")
		return w.b, nil
	}
	j := &jobRunner{job: job}
	if b := job.Get("brain"); b.Truthy() {
		j.brain = FindArchetype(b.String())
		if j.brain == nil {
			w.raw(`{"error":`)
			w.str("no archetype called " + b.String())
			w.raw(`,"names":[`)
			for i, a := range Archetypes {
				if i > 0 {
					w.raw(",")
				}
				w.str(a.Name)
			}
			w.raw("]}")
			return w.b, nil
		}
	}
	j.seed = numOr(job.Get("seed"), 1)
	task := ""
	if t := job.Get("task"); t != nil && t.Kind == JStr {
		task = t.S
	}

	if task == "learn" {
		net, fresh, err := j.network(true)
		if err != nil {
			return nil, err
		}
		net.ApplyVerdicts(job.Get("verdicts"))
		net.Batch = numOr(job.Get("batch"), 1)
		t0 := Clock()
		steps := numOr(job.Get("steps"), 1200)
		for k := 0.0; k < steps; k++ {
			net.Tick()
		}
		k := 20.0
		if c := job.Get("candidates"); !c.IsNullish() {
			k = c.Num()
		}
		cands := net.Candidates(k)
		if p := job.Get("state"); p.Truthy() {
			if err := os.WriteFile(p.String(), net.Save(), 0o644); err != nil {
				return nil, err
			}
		}
		links := net.linkList()
		settled, proved := 0, 0
		for _, l := range links {
			if !l.HasTent {
				settled++
			}
			if l.Proved.Truthy() {
				proved++
			}
		}
		slotted := 0
		for _, n := range net.N {
			if n.Role == "" {
				if r := net.RoleOf(n); r != nil && r.From == "its slot neuron" {
					slotted++
				}
			}
		}
		w.raw(`{"fresh":`)
		w.boolean(fresh)
		w.raw(`,"candidates":[`)
		for i, c := range cands {
			if i > 0 {
				w.raw(",")
			}
			w.raw(`{"a":`)
			w.str(c.A)
			w.raw(`,"b":`)
			w.str(c.B)
			w.raw(`,"alike":`)
			w.num(c.Alike)
			w.raw(`,"original":`)
			w.str(c.Original)
			w.raw(`,"swapped":`)
			w.str(c.Swapped)
			w.raw("}")
		}
		w.raw(`],"neurons":`)
		w.num(float64(len(net.N)))
		w.raw(`,"links":`)
		w.num(float64(len(links)))
		w.raw(`,"settled":`)
		w.num(float64(settled))
		w.raw(`,"proved":`)
		w.num(float64(proved))
		w.raw(`,"banned":`)
		w.num(float64(net.banned.Len()))
		w.raw(`,"ni":`)
		w.num(toFixedNum(net.NiRate(), 3))
		w.raw(`,"bits":`)
		w.num(toFixedNum(net.BitRate(), 3))
		w.raw(`,"phrases":`)
		w.num(float64(net.phrases.Len()))
		w.raw(`,"slotted":`)
		w.num(float64(slotted))
		w.raw(`,"proposed":`)
		w.num(net.proposed)
		w.raw(`,"kept":`)
		w.num(net.confirmed)
		w.raw(`,"cut":`)
		w.num(net.rejected)
		w.raw(`,"split":`)
		writeStrList(w, nonNil(splitWords(net)))
		w.raw(`,"seconds":`)
		w.num(jsRound(net.T))
		w.raw(`,"ms":`)
		w.num(float64(Clock().Sub(t0).Milliseconds()))
		w.raw("}")
		return w.b, nil
	}

	loadState := func(brain *Archetype) (*Network, *JV, error) {
		p := job.Get("state")
		b, err := os.ReadFile(p.String())
		if err != nil {
			return nil, nil, err
		}
		saved, err := ParseJSON(b)
		if err != nil {
			return nil, nil, err
		}
		net := NewNetwork(&Spec{}, j.seed, nil, brain, true)
		net.Load(saved)
		return net, saved, nil
	}

	if task == "bricks" {
		net, _, err := loadState(nil)
		if err != nil {
			return nil, err
		}
		var out NumObjAny
		if ws := job.Get("words"); ws != nil && ws.Kind == JArr {
			for _, x := range ws.A {
				out.set(x.String(), net.BrickOf(x.String()))
			}
		}
		w.raw("{")
		for i, p := range jsKeyOrder(out.keys) {
			if i > 0 {
				w.raw(",")
			}
			w.key(out.keys[p])
			g := out.vals[p]
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
		w.raw("}")
		return w.b, nil
	}

	// Read a whole line with the saved, learned network (no training first): one run for the line.
	if task == "readline" {
		net, _, err := loadState(j.brain)
		if err != nil {
			return nil, err
		}
		var words []string
		if ws := job.Get("words"); ws.Truthy() && ws.Kind == JArr {
			for _, x := range ws.A {
				words = append(words, x.String())
			}
		}
		answers := net.ReadLine(words, 40)
		w.raw(`{"answers":[`)
		for i, a := range answers {
			if i > 0 {
				w.raw(",")
			}
			if a.Result == nil {
				w.raw(`{"word":`)
				w.str(a.Word)
				w.raw(`,"found":false}`)
				continue
			}
			r := a.Result
			best := r.Ranked[0]
			role := net.RoleOf(best.S)
			w.raw(`{"word":`)
			w.str(a.Word)
			w.raw(`,"found":true,"verdict":`)
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
			w.raw(`,"senses":`)
			writeSenses(w, net, r)
			w.raw("}")
		}
		n := len(answers)
		for _, word := range words { // asked for but not in the network
			found := false
			for _, a := range answers {
				if a.Word == word {
					found = true
					break
				}
			}
			if !found {
				if n > 0 {
					w.raw(",")
				}
				n++
				w.raw(`{"word":`)
				w.str(word)
				w.raw(`,"found":false}`)
			}
		}
		w.raw(`],"split":[],"neurons":`)
		w.num(float64(len(net.N)))
		w.raw(`,"links":`)
		w.num(float64(net.links.Len()))
		w.raw(`,"learned":true}`)
		return w.b, nil
	}

	if task == "score" {
		net, saved, err := loadState(nil)
		if err != nil {
			return nil, err
		}
		groups := job.Get("groups")
		if groups == nil || groups.Kind != JObj {
			return nil, errors.New("score needs groups")
		}
		groupsOf := map[string]map[string]bool{}
		for i, g := range groups.Keys {
			if ws := groups.Vals[i]; ws != nil && ws.Kind == JArr {
				for _, x := range ws.A {
					wd := x.String()
					if groupsOf[wd] == nil {
						groupsOf[wd] = map[string]bool{}
					}
					groupsOf[wd][g] = true
				}
			}
		}
		share := func(a, b string) bool {
			for g := range groupsOf[a] {
				if groupsOf[b][g] {
					return true
				}
			}
			return false
		}
		var gold []*Neuron
		for _, n := range net.N {
			if groupsOf[n.Word] != nil && n.Seen >= 3 {
				gold = append(gold, n)
			}
		}
		pred := net.predIndex()
		type pr struct {
			sim  float64
			a, b string
		}
		var pairs []pr
		for i := 0; i < len(gold); i++ {
			for k := i + 1; k < len(gold); k++ {
				if gold[i].Word != gold[k].Word {
					pairs = append(pairs, pr{net.alike(gold[i], gold[k], pred), gold[i].Word, gold[k].Word})
				}
			}
		}
		sort.SliceStable(pairs, func(x, y int) bool { return pairs[y].sim-pairs[x].sim < 0 })
		at := func(k int) (float64, bool) {
			top := pairs
			if len(top) > k {
				top = top[:k]
			}
			if len(top) == 0 {
				return 0, false
			}
			c := 0
			for _, p := range top {
				if share(p.a, p.b) {
					c++
				}
			}
			return jsRound(100 * float64(c) / float64(len(top))), true
		}
		placed, alikePlaced := 0, 0
		for _, n := range gold { // only the lines Ne proposed and Te kept, or the compiler proved
			var vote OMap[string, float64]
			for _, l := range n.adj {
				if l.HasTent || !(l.Ne || l.Proved.Truthy()) {
					continue
				}
				o := net.Get(l.other(n.id))
				if o == nil || o.Word == n.Word || groupsOf[o.Word] == nil {
					continue
				}
				for _, g := range groups.Keys {
					if groupsOf[o.Word][g] {
						v, _ := vote.Get(g)
						vote.Set(g, v+l.W)
					}
				}
			}
			if best, ok := topVote(&vote); ok && groupsOf[n.Word][best] {
				alikePlaced++
			}
		}
		for _, n := range gold {
			var vote OMap[string, float64]
			for _, l := range n.adj {
				if l.HasTent {
					continue
				}
				o := net.Get(l.other(n.id))
				if o == nil || o.Word == n.Word || groupsOf[o.Word] == nil {
					continue
				}
				// a JS Set of group names, in insertion order (the order of the groups object)
				for _, g := range groups.Keys {
					if groupsOf[o.Word][g] {
						v, _ := vote.Get(g)
						vote.Set(g, v+l.W)
					}
				}
			}
			type kv struct {
				k string
				v float64
			}
			var list []kv
			for k, v := range vote.All() {
				list = append(list, kv{k, v})
			}
			sort.SliceStable(list, func(x, y int) bool { return list[y].v-list[x].v < 0 })
			if len(list) > 0 && groupsOf[n.Word][list[0].k] {
				placed++
			}
		}
		chance, cn := 0, 0
		rand := NewRng(j.seed) // seeded: the same answer every time
		for k := 0; k < len(pairs) && k < 4000; k++ {
			p := pairs[int(math.Floor(rand.Next()*float64(len(pairs))))]
			cn++
			if share(p.a, p.b) {
				chance++
			}
		}
		w.raw(`{"gold":`)
		w.num(float64(len(gold)))
		for _, kk := range []struct {
			name string
			k    int
		}{{"p20", 20}, {"p100", 100}} {
			w.raw(`,"` + kk.name + `":`)
			if v, ok := at(kk.k); ok {
				w.num(v)
			} else {
				w.raw("null")
			}
		}
		w.raw(`,"placed":`)
		w.num(jsRound(100 * float64(placed) / math.Max(1, float64(len(gold)))))
		w.raw(`,"placedAlike":`)
		w.num(jsRound(100 * float64(alikePlaced) / math.Max(1, float64(len(gold)))))
		w.raw(`,"chance":`)
		w.num(jsRound(100 * float64(chance) / math.Max(1, float64(cn))))
		w.raw(`,"ni":`)
		ni := 0.0
		if st := saved.Get("stats"); st.Truthy() {
			if v := st.Get("ni"); v.Truthy() {
				ni = v.Num()
			}
		}
		w.num(ni)
		w.raw(`,"top":[`)
		for i := 0; i < len(pairs) && i < 12; i++ {
			if i > 0 {
				w.raw(",")
			}
			w.str(pairs[i].a + "~" + pairs[i].b)
		}
		w.raw("]}")
		return w.b, nil
	}

	// Reading.
	var net *Network
	if job.Get("state").Truthy() {
		net, _, err = j.network(true)
		if err != nil {
			return nil, err
		}
		steps := numOr(job.Get("steps"), 600)
		for k := 0.0; k < steps; k++ {
			net.Tick()
		}
	} else {
		mode := "groups"
		if m := job.Get("mode"); m.Truthy() {
			mode = m.String()
		}
		net = NewNetwork(DataSpec(job.Get("data").String(), mode, j.seed), j.seed, nil, j.brain, false)
		steps := numOr(job.Get("steps"), 3000)
		for k := 0.0; k < steps; k++ {
			net.Tick()
		}
	}
	// The archetype part is written after reading: its warmth and orderPull are read after the questions.
	w.raw(`{"archetype":`)
	answers := &jw{}
	answers.raw("[")
	for i, q := range ParseQuestions(job.Get("questions").String()) {
		if i > 0 {
			answers.raw(",")
		}
		for k := 0; k < 30; k++ {
			net.Tick()
		}
		r := net.Read(q.Word, q.Context, 40)
		if r == nil {
			answers.raw(`{"word":`)
			answers.str(q.Word)
			answers.raw(`,"found":false}`)
			continue
		}
		WriteAnswer(answers, net, q.Word, r)
	}
	answers.raw("]")
	if j.brain != nil {
		w.raw(`{"name":`)
		w.str(j.brain.Name)
		w.raw(`,"type":`)
		w.str(j.brain.Type)
		w.raw(`,"stack":`)
		writeStrList(w, j.brain.Personality.Stack[:])
		w.raw(`,"rules":`)
		writeDecisions(w, net.D)
		w.raw(`,"warmth":`)
		w.num(toFixedNum(net.Warmth, 2))
		w.raw(`,"orderPull":`)
		w.num(toFixedNum(net.OrderPull, 2))
		w.raw("}")
	} else {
		w.raw("null")
	}
	w.raw(`,"answers":`)
	w.b = append(w.b, answers.b...)
	w.raw(`,"split":`)
	writeStrList(w, nonNil(splitWords(net)))
	w.raw(`,"neurons":`)
	w.num(float64(len(net.N)))
	w.raw(`,"links":`)
	w.num(float64(net.links.Len()))
	w.raw(`,"seconds":`)
	w.num(jsRound(net.T))
	w.raw(`,"learned":`)
	w.boolean(job.Get("state").Truthy())
	w.raw(`,"events":[`)
	first := true
	for _, l := range net.Log {
		if containsAny(l.Text, "fission", "fusion") {
			if !first {
				w.raw(",")
			}
			first = false
			w.str(l.Text)
		}
	}
	w.raw("]}")
	return w.b, nil
}

func containsAny(s string, subs ...string) bool {
	for _, x := range subs {
		for i := 0; i+len(x) <= len(s); i++ {
			if s[i:i+len(x)] == x {
				return true
			}
		}
	}
	return false
}

func writeDecisions(w *jw, d Decisions) {
	w.raw(`{"strict":`)
	w.num(d.Strict)
	w.raw(`,"split":`)
	w.num(d.Split)
	w.raw(`,"patience":`)
	w.num(d.Patience)
	w.raw(`,"radiant":`)
	w.num(d.Radiant)
	w.raw(`,"replay":`)
	w.num(d.Replay)
	w.raw(`,"react":`)
	w.num(d.React)
	w.raw("}")
}

// WriteAnswer writes one reading answer the way run.js does.
func WriteAnswer(w *jw, net *Network, word string, r *ReadResult) {
	best := r.Ranked[0]
	role := net.RoleOf(best.S)
	w.raw(`{"word":`)
	w.str(word)
	w.raw(`,"found":true,"verdict":`)
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
	w.raw(`,"roleFrom":`)
	if role != nil {
		w.str(role.From)
	} else {
		w.raw("null")
	}
	w.raw(`,"senses":[`)
	for i, x := range r.Ranked {
		if i > 0 {
			w.raw(",")
		}
		w.raw(`{"label":`)
		w.str(x.S.Label)
		w.raw(`,"matched":`)
		w.num(float64(x.Matched))
		w.raw(`,"near":`)
		near := net.Neighbours(x.S)
		if len(near) > 3 {
			near = near[:3]
		}
		writeStrList(w, near)
		w.raw("}")
	}
	w.raw(`],"profile":{`)
	for i, p := range best.Profile {
		if i > 0 {
			w.raw(",")
		}
		w.key(p.Key)
		w.num(p.Pct)
	}
	w.raw("}}")
}

// NumObjAny is a JS object from word to brick guess (property order: array indices first).
type NumObjAny struct {
	keys []string
	vals []*BrickGuess
}

func (o *NumObjAny) set(k string, v *BrickGuess) {
	for i, x := range o.keys {
		if x == k {
			o.vals[i] = v
			return
		}
	}
	o.keys = append(o.keys, k)
	o.vals = append(o.vals, v)
}

// topVote is [...vote].sort((x, y) => y[1] - x[1])[0][0]: the first of the highest.
func topVote(vote *OMap[string, float64]) (string, bool) {
	type kv struct {
		k string
		v float64
	}
	var list []kv
	for k, v := range vote.All() {
		list = append(list, kv{k, v})
	}
	sort.SliceStable(list, func(x, y int) bool { return list[y].v-list[x].v < 0 })
	if len(list) == 0 {
		return "", false
	}
	return list[0].k, true
}

func writeSenses(w *jw, net *Network, r *ReadResult) {
	w.raw("[")
	for i, x := range r.Ranked {
		if i > 0 {
			w.raw(",")
		}
		w.raw(`{"label":`)
		w.str(x.S.Label)
		w.raw(`,"matched":`)
		w.num(float64(x.Matched))
		w.raw(`,"near":`)
		near := net.Neighbours(x.S)
		if len(near) > 3 {
			near = near[:3]
		}
		writeStrList(w, near)
		w.raw("}")
	}
	w.raw("]")
}
