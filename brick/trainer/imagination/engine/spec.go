package engine

// Specs (wordSpec, dataSpec, fieldSpec), the data parsers and the archetypes, as in sim.js.

import (
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// ---------- numbers ----------

var numberRe = regexp.MustCompile(`^#~?([0-9]+(?:\.[0-9]+)?)$`)

func numberValue(w string) (float64, bool) {
	m := numberRe.FindStringSubmatch(w)
	if m == nil {
		return 0, false
	}
	v, _ := strconv.ParseFloat(m[1], 64)
	return v, true
}

var (
	numLo    = 0.4
	numSpan  = 1.3
	num10001 = 10001.0
)

func numberOmega(v float64) float64 { return numLo + numSpan*Log10(1+v)/Log10(num10001) }

// ---------- JS string helpers ----------

// JS \s and String.prototype.trim's white space.
const jsSpace = `\t\n\v\f\r \x{a0}\x{1680}\x{2000}-\x{200a}\x{2028}\x{2029}\x{202f}\x{205f}\x{3000}\x{feff}`
const jsDot = `[^\n\r\x{2028}\x{2029}]` // JS `.` without the s flag

func isJSSpace(r rune) bool {
	switch r {
	case '\t', '\n', '\v', '\f', '\r', ' ', 0xa0, 0x1680, 0x2028, 0x2029, 0x202f, 0x205f, 0x3000, 0xfeff:
		return true
	}
	return r >= 0x2000 && r <= 0x200a
}

func jsTrim(s string) string { return strings.TrimFunc(s, isJSSpace) }

// jsLower is String.prototype.toLowerCase (Go's simple mapping plus the one unconditional special case).
func jsLower(s string) string {
	if strings.ContainsRune(s, 0x130) {
		s = strings.ReplaceAll(s, "İ", "i̇")
	}
	return strings.ToLower(s)
}

var (
	commentRe  = regexp.MustCompile(`^[` + jsSpace + `]*#` + jsDot + `*|[` + jsSpace + `]#(?:[` + jsSpace + `]` + jsDot + `*)?$`)
	groupRe    = regexp.MustCompile(`^group[` + jsSpace + `]+([a-z0-9_-]+)[` + jsSpace + `]*:[` + jsSpace + `]*(` + jsDot + `+)$`)
	groupWord  = regexp.MustCompile(`[a-z0-9']+`)
	saidWord   = regexp.MustCompile(`[#~a-z0-9'.]+`)
	madeWord   = regexp.MustCompile(`[=a-z0-9_:-]+`)
	questionRe = regexp.MustCompile(`^([a-z0-9']+)[` + jsSpace + `]*@[` + jsSpace + `]*([^=]+?)[` + jsSpace + `]*(?:=[` + jsSpace + `]*(` + jsDot + `+))?$`)
)

func cleanLine(raw string) string {
	if loc := commentRe.FindStringIndex(raw); loc != nil {
		raw = raw[:loc[0]] + raw[loc[1]:]
	}
	return jsLower(jsTrim(raw))
}

// ---------- parseData / parseQuestions ----------

// StrListObj is a JS object of word lists (groups), in JS property order.
type StrListObj struct {
	keys []string
	vals [][]string
}

func (o *StrListObj) set(k string, v []string) {
	for i, x := range o.keys {
		if x == k {
			o.vals[i] = v
			return
		}
	}
	o.keys = append(o.keys, k)
	o.vals = append(o.vals, v)
}

// ordered returns the keys and values in JS property order.
func (o *StrListObj) ordered() ([]string, [][]string) {
	ord := jsKeyOrder(o.keys)
	ks := make([]string, len(ord))
	vs := make([][]string, len(ord))
	for i, p := range ord {
		ks[i], vs[i] = o.keys[p], o.vals[p]
	}
	return ks, vs
}

func ParseData(text string) (StrListObj, []*Sent) {
	var groups StrListObj
	var sentences []*Sent
	for _, raw := range strings.Split(text, "\n") {
		line := cleanLine(raw)
		if line == "" {
			continue
		}
		if g := groupRe.FindStringSubmatch(line); g != nil {
			groups.set(g[1], nonNil(groupWord.FindAllString(g[2], -1)))
			continue
		}
		parts := strings.Split(line, "||")
		said := parts[0]
		words := saidWord.FindAllString(said, -1)
		if len(words) > 0 {
			s := &Sent{W: words}
			if len(parts) > 1 && parts[1] != "" {
				s.Out, s.HasOut = nonNil(madeWord.FindAllString(parts[1], -1)), true
			}
			sentences = append(sentences, s)
		}
	}
	return groups, sentences
}

func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

type Question struct {
	Word    string
	Context []string
	Want    ostr
	Text    string
}

func ParseQuestions(text string) []Question {
	var out []Question
	for _, raw := range strings.Split(text, "\n") {
		line := cleanLine(raw)
		m := questionRe.FindStringSubmatchIndex(line)
		if m == nil {
			continue
		}
		q := Question{Word: line[m[2]:m[3]], Context: nonNil(groupWord.FindAllString(line[m[4]:m[5]], -1)), Want: ostr{kind: 1}, Text: line}
		if m[6] >= 0 && m[7] > m[6] {
			q.Want = ostr{kind: 2, s: jsTrim(line[m[6]:m[7]])}
		}
		out = append(out, q)
	}
	return out
}

// ---------- dataSpec ----------

type strNumObjMap struct {
	keys []string
	vals []*NumObj
	idx  map[string]int
}

func (m *strNumObjMap) get(k string) *NumObj {
	if m.idx == nil {
		m.idx = map[string]int{}
	}
	if p, ok := m.idx[k]; ok {
		return m.vals[p]
	}
	o := &NumObj{}
	m.idx[k] = len(m.keys)
	m.keys = append(m.keys, k)
	m.vals = append(m.vals, o)
	return o
}

func (m *strNumObjMap) lookup(k string) *NumObj {
	if p, ok := m.idx[k]; ok {
		return m.vals[p]
	}
	return nil
}

// DataSpec is dataSpec(text, mode, seed).
func DataSpec(text, mode string, seed float64) *Spec {
	groupsObj, sentences := ParseData(text)
	r := NewRng(seed*7919 + 3)
	omega := map[string]float64{}
	groupOf := map[string]string{}
	var words []string
	seen := map[string]bool{}
	addWord := func(w string) {
		if !seen[w] {
			seen[w] = true
			words = append(words, w)
		}
	}
	names, gvals := groupsObj.ordered()
	for _, g := range gvals {
		for _, w := range g {
			addWord(w)
		}
	}
	for _, st := range sentences {
		for _, w := range st.W {
			addWord(w)
		}
	}
	var co strNumObjMap
	for _, st := range sentences {
		for _, a := range st.W {
			for _, b := range st.W {
				if a != b {
					o := co.get(a)
					v, _ := o.Get(b)
					o.Set(b, v+1)
				}
			}
		}
	}
	for i, g := range names {
		ws := gvals[i]
		for k, w := range ws {
			if _, ok := groupOf[w]; !ok || groupOf[w] == "" {
				groupOf[w] = g
			}
			if mode == "groups" {
				if _, ok := omega[w]; !ok {
					omega[w] = 2 + float64(i)*1.2 + (float64(k)-(float64(len(ws))-1)/2)*0.12
				}
			}
		}
	}
	band := jsMax(4, float64(len(words))/25)
	if mode == "random" || mode == "raw" {
		for _, w := range words {
			omega[w] = 2 + r.Next()*band
		}
	} else {
		var free []string
		for _, w := range words {
			if _, ok := omega[w]; !ok {
				free = append(free, w)
			}
		}
		fixed := len(words) - len(free)
		for _, w := range free {
			omega[w] = 2 + r.Next()*4
		}
		for it := 0; it < 60; it++ {
			next := map[string]float64{}
			for _, w := range free {
				t, n := 0.0, 0.0
				if o := co.lookup(w); o != nil {
					for _, p := range o.Order() {
						c := o.vals[p]
						t += c * omega[o.keys[p]]
						n += c
					}
				}
				if truthyNum(n) {
					next[w] = 0.5*omega[w] + 0.5*t/n
				} else {
					next[w] = omega[w]
				}
			}
			if fixed != 0 {
				for _, w := range free {
					omega[w] = next[w]
				}
			} else {
				srt := append([]string(nil), free...)
				sort.SliceStable(srt, func(i, j int) bool { return next[srt[i]]-next[srt[j]] < 0 })
				den := jsMax(1, float64(len(free)-1))
				for k, w := range srt {
					omega[w] = 2 + 4*float64(k)/den
				}
			}
		}
	}
	raw := mode == "raw"
	spec := &Spec{Sentences: sentences}
	for _, w := range words {
		a := ((omega[w] - 1.6) / 5) * piVar * 2
		rad := 0.3 + 0.1*r.Next()
		n := SpecNeuron{Label: w, Truth: groupOf[w], Omega: omega[w], X: 0.5 + rad*Cos(a), HasX: true, Y: 0.5 + rad*0.95*Sin(a), HasY: true}
		if !raw {
			n.Group = groupOf[w]
		}
		spec.Neurons = append(spec.Neurons, n)
	}
	if raw {
		var outs []string
		oseen := map[string]bool{}
		for _, st := range sentences {
			for _, o := range st.Out {
				if !oseen[o] {
					oseen[o] = true
					outs = append(outs, o)
				}
			}
		}
		no := float64(len(outs))
		for k, o := range outs {
			kk := float64(k)
			spec.Neurons = append(spec.Neurons, SpecNeuron{Label: o, Omega: 2 + band*(kk+0.5)/no, X: 0.05 + ninetyPctVar*(kk+0.5)/no, HasX: true, Y: 0.04, HasY: true, Fixed: true})
		}
		for i := range spec.Neurons {
			if v, ok := numberValue(spec.Neurons[i].Label); ok {
				spec.Neurons[i].Omega = numberOmega(v)
				spec.Neurons[i].Fixed = true
			}
		}
		return spec
	}
	linked := map[string]bool{}
	for _, g := range gvals {
		for i := 0; i < len(g); i++ {
			for j := i + 1; j < len(g); j++ {
				k := pairKey(g[i], g[j])
				if !linked[k] {
					linked[k] = true
					spec.Links = append(spec.Links, SpecLink{g[i], g[j], 0.5})
				}
			}
		}
	}
	for _, p := range jsKeyOrder(co.keys) {
		a := co.keys[p]
		o := co.vals[p]
		for _, q := range o.Order() {
			b := o.keys[q]
			k := pairKey(a, b)
			if !linked[k] {
				linked[k] = true
				spec.Links = append(spec.Links, SpecLink{a, b, 0.4})
			}
		}
	}
	return spec
}

// WordSpec is wordSpec(gap): the word preset.
func WordSpec(gap float64) *Spec {
	two := 2.0
	g := map[string]float64{"remove": two, "show": two + gap, "check": two + 2*gap, "place": two + 3*gap}
	groupNames := []string{"remove", "show", "place", "check"}
	groups := map[string][]string{
		"remove": {"clear", "wipe", "remove", "junk"},
		"show":   {"write", "show", "text", "quote"},
		"place":  {"window", "put", "place", "position"},
		"check":  {"check", "if", "compare", "equals"},
	}
	pos := map[string][2]float64{"remove": {0.16, 0.3}, "show": {0.5, 0.2}, "check": {0.7, 0.68}, "place": {0.26, 0.76}}
	spec := &Spec{}
	for _, grp := range groupNames {
		ws := groups[grp]
		for k, w := range ws {
			c := pos[grp]
			ang := (float64(k) / float64(len(ws))) * piVar * 2
			spec.Neurons = append(spec.Neurons, SpecNeuron{Label: w, Group: grp, Omega: g[grp] + (float64(k)-1.5)*0.12,
				X: c[0] + 0.11*Cos(ang), HasX: true, Y: c[1] + 0.11*Sin(ang), HasY: true})
		}
	}
	spec.Neurons = append(spec.Neurons,
		SpecNeuron{Label: "crap", Omega: (g["remove"] + g["show"]) / 2, X: 0.4, HasX: true, Y: 0.25, HasY: true},
		SpecNeuron{Label: "test", Omega: (g["show"]+g["check"])/2 + 0.05, X: 0.46, HasX: true, Y: 0.5, HasY: true},
		SpecNeuron{Label: "zorp", Omega: g["place"] + 0.35, X: 0.92, HasX: true, Y: 0.42, HasY: true, Hidden: true})
	for _, grp := range groupNames {
		ws := groups[grp]
		for i := 0; i < len(ws); i++ {
			for j := i + 1; j < len(ws); j++ {
				spec.Links = append(spec.Links, SpecLink{ws[i], ws[j], 0.5})
			}
		}
	}
	for _, w := range []string{"clear", "wipe", "junk", "write", "text", "quote"} {
		spec.Links = append(spec.Links, SpecLink{"crap", w, 0.4})
	}
	for _, w := range []string{"text", "write", "check", "compare"} {
		spec.Links = append(spec.Links, SpecLink{"test", w, 0.4})
	}
	for _, s := range [][]string{
		{"clear", "crap"}, {"wipe", "crap"}, {"crap", "junk"}, {"remove", "crap"},
		{"write", "crap"}, {"show", "crap"}, {"crap", "text"}, {"quote", "crap"},
		{"write", "test"}, {"test", "text"}, {"check", "test"}, {"if", "test"}, {"compare", "test"},
		{"put", "window"}, {"place", "position"}, {"clear", "junk"}, {"write", "text"}, {"check", "equals"},
	} {
		spec.Sentences = append(spec.Sentences, &Sent{W: s})
	}
	return spec
}

// FieldSpec is fieldSpec(n, seed): a random field.
func FieldSpec(n int, seed float64) *Spec {
	r := NewRng(seed)
	spec := &Spec{}
	for i := 0; i < n; i++ {
		om := 2 + r.Next()*3
		x := 0.1 + 0.8*r.Next()
		y := 0.1 + 0.8*r.Next()
		spec.Neurons = append(spec.Neurons, SpecNeuron{Label: "n" + strconv.Itoa(i+1), Omega: om, X: x, HasX: true, Y: y, HasY: true})
	}
	for i := 0; i < n; i++ {
		for j := i + 1; j < n; j++ {
			if r.Next() < 0.18 {
				spec.Links = append(spec.Links, SpecLink{"n" + strconv.Itoa(i+1), "n" + strconv.Itoa(j+1), 0.3 + 0.4*r.Next()})
			}
		}
	}
	for _, x := range spec.Neurons {
		spec.Sentences = append(spec.Sentences, &Sent{W: []string{x.Label}})
	}
	return spec
}

const SampleData = `# Meaning groups: "group name: words". Every other line is a sentence it practises on.
group remove: clear, wipe, remove, junk
group show: write, show, text, quote
group check: check, if, compare, equals
group place: window, put, place, position
clear crap
wipe crap
crap junk
remove crap
write crap
show crap
crap text
quote crap
write test
test text
check test
if test
compare test
put window
place position
clear junk
write text
check equals`

const SampleQuestions = `# word @ context = what you expect (a group, a word it should be linked to, or "not relevant")
crap @ clear = remove
crap @ write = show
crap @ quote = show
test @ check = check
test @ text = show
crap @ put = not relevant
test @ window = not relevant`

// ---------- archetypes ----------

var stacks = map[string][4]string{
	"INTJ": {"Ni", "Te", "Fi", "Se"}, "INTP": {"Ti", "Ne", "Si", "Fe"}, "ENTJ": {"Te", "Ni", "Se", "Fi"}, "ENTP": {"Ne", "Ti", "Fe", "Si"},
	"INFJ": {"Ni", "Fe", "Ti", "Se"}, "INFP": {"Fi", "Ne", "Si", "Te"}, "ENFJ": {"Fe", "Ni", "Se", "Ti"}, "ENFP": {"Ne", "Fi", "Te", "Si"},
	"ISTJ": {"Si", "Te", "Fi", "Ne"}, "ISFJ": {"Si", "Fe", "Ti", "Ne"}, "ESTJ": {"Te", "Si", "Ne", "Fi"}, "ESFJ": {"Fe", "Si", "Ne", "Ti"},
	"ISTP": {"Ti", "Se", "Ni", "Fe"}, "ISFP": {"Fi", "Se", "Ni", "Te"}, "ESTP": {"Se", "Ti", "Fe", "Ni"}, "ESFP": {"Se", "Fi", "Te", "Ni"},
}

var stackWeight = [4]float64{1.6, 1.3, 0.9, 0.6}
var shadowWeight = 0.5

type Target struct{ Heat, Order float64 }

type Personality struct {
	Type     string
	Stack    [4]string
	Energies [nEnergies]float64
	Target   Target
}

type Archetype struct {
	Name, Type, Bias, Line string
	Yours                  bool
	Personality            Personality
	Decisions              Decisions
}

func pick(c bool, a, b float64) float64 {
	if c {
		return a
	}
	return b
}

func personalityOf(typ string) Personality {
	stack := stacks[typ]
	p := Personality{Type: typ, Stack: stack}
	for _, fn := range functionOrder {
		w := shadowWeight
		for i, s := range stack {
			if s == fn {
				w = stackWeight[i]
				break
			}
		}
		k := functions[fn]
		p.Energies[k[0]] = w
		p.Energies[k[1]] = w
	}
	base := 0.75
	p.Target.Heat = toFixedNum(base+pick(typ[0] == 'E', 0.25, 0)+pick(typ[3] == 'P', 0.1, 0), 2)
	p.Target.Order = pick(typ[3] == 'J', 0.86, 0.76)
	return p
}

func decisionsOf(typ, bias string) Decisions {
	T, J, N := typ[2] == 'T', typ[3] == 'J', typ[1] == 'N'
	S, F, P := !N, !T, !J
	var d Decisions
	d.Strict = 0.5 + pick(T, 0.08, -0.04) + pick(J, 0.07, -0.03) + pick(bias == "strict", 0.1, 0)
	d.Split = 0.4 - pick(N, 0.08, 0) + pick(J, 0.04, -0.04) - pick(bias == "split", 0.12, 0)
	d.Patience = jsRound(40 * pick(S, 1.6, 1) * pick(F, 1.3, 1) * pick(P, 1.1, 0.9) * pick(bias == "keep", 2, 1))
	d.Radiant = 0.5 - pick(N, 0.1, 0) - pick(P, 0.08, 0) - pick(bias == "explore", 0.12, 0)
	d.Replay = toFixedNum(pick(S, 0.25, 0.05)+pick(F, 0.1, 0)+pick(bias == "keep", 0.2, 0), 2)
	d.React = pick(bias == "swift", 0.08, pick(bias == "order", 0.03, 0.05))
	d.Strict = toFixedNum(jsMin(0.8, d.Strict), 2)
	d.Split = toFixedNum(jsMax(0.15, d.Split), 2)
	d.Radiant = toFixedNum(jsMax(0.2, d.Radiant), 2)
	return d
}

var archetypeRows = [][5]string{
	{"The Mirror Oracle", "INFJ", "strict", "Sees itself in others and the future in both: it reflects every context back until the true meaning shows.", "1"},
	{"The Veiled Seer", "INFJ", "keep", "Holds quiet visions for a long time and lets nothing meaningful fade.", ""},
	{"The Dream Keeper", "INFJ", "explore", "Walks the edges of sleep, lighting up meanings no one else has noticed.", ""},
	{"The Sensory Mythweaver", "ISFP", "explore", "Weaves myths out of colour, texture and feeling: every sensation becomes a story.", "1"},
	{"The Colour Drifter", "ISFP", "keep", "Moves gently through hues and moods, keeping each one it has loved.", ""},
	{"The Wild Gardener", "ISFP", "split", "Lets meanings grow in every direction, then prunes them by feel.", ""},
	{"The Silent Architect", "INTJ", "strict", "Plans the whole structure before laying a single brick.", ""},
	{"The Midnight Strategist", "INTJ", "order", "Works alone in the dark hours, ordering every possibility into one path.", ""},
	{"The Crystal Cartographer", "INTJ", "split", "Maps meanings with cold precision, splitting each region cleanly.", ""},
	{"The Paradox Engineer", "INTP", "split", "Takes every idea apart to see whether its opposite also works.", ""},
	{"The Quiet Logician", "INTP", "strict", "Accepts nothing that isn't proven, and says so softly.", ""},
	{"The Fractal Hermit", "INTP", "explore", "Follows patterns inward forever, finding worlds inside words.", ""},
	{"The Iron Conductor", "ENTJ", "order", "Sets the tempo for the whole network and keeps it in line.", ""},
	{"The Sovereign Engine", "ENTJ", "swift", "Decides fast, drives hard, and turns every impact into motion.", ""},
	{"The Storm Marshal", "ENTJ", "strict", "Commands chaos into formation, keeping only what holds.", ""},
	{"The Spark Trickster", "ENTP", "explore", "Throws sparks at every idea to see which ones catch fire.", ""},
	{"The Idea Smith", "ENTP", "split", "Hammers one word into many shapes and tests each blade.", ""},
	{"The Chaos Cartographer", "ENTP", "swift", "Charts what's changing while it changes.", ""},
	{"The Ember Poet", "INFP", "keep", "Keeps small fires of meaning burning long after others have gone out.", ""},
	{"The Wandering Lantern", "INFP", "explore", "Carries a soft light into forgotten corners.", ""},
	{"The Soft Rebel", "INFP", "split", "Refuses the obvious meaning and finds the personal one.", ""},
	{"The Warm Beacon", "ENFJ", "order", "Draws every neuron toward one warm centre.", ""},
	{"The Chorus Shepherd", "ENFJ", "keep", "Keeps every voice in the choir, even the quietest.", ""},
	{"The Sun Herald", "ENFJ", "swift", "Announces meaning the moment it rises.", ""},
	{"The Wildfire Muse", "ENFP", "explore", "Sets imagination alight and follows wherever it spreads.", ""},
	{"The Starlight Vagabond", "ENFP", "split", "Collects meanings like stars, one for every road.", ""},
	{"The Kaleidoscope Child", "ENFP", "swift", "Turns every context and sees a new pattern each time.", ""},
	{"The Stone Keeper", "ISTJ", "keep", "Remembers every brick ever laid, exactly where it was.", ""},
	{"The Ledger Monk", "ISTJ", "strict", "Records only what balances, and checks it twice.", ""},
	{"The Clockwork Sentinel", "ISTJ", "order", "Keeps perfect time and lets nothing slip out of rhythm.", ""},
	{"The Hearth Guardian", "ISFJ", "keep", "Tends the fire so the oldest meanings stay warm.", ""},
	{"The Gentle Archivist", "ISFJ", "order", "Files every memory with care so it can be found again.", ""},
	{"The Lantern Nurse", "ISFJ", "strict", "Heals only what it understands, one careful step at a time.", ""},
	{"The Steel Steward", "ESTJ", "order", "Runs the network like a well-kept workshop.", ""},
	{"The Bridge Builder", "ESTJ", "swift", "Connects what should be connected, fast and solid.", ""},
	{"The Order Captain", "ESTJ", "strict", "Keeps the ranks straight and the rules clear.", ""},
	{"The Village Heart", "ESFJ", "keep", "Knows everyone's story and keeps them all together.", ""},
	{"The Table Host", "ESFJ", "order", "Seats every meaning where it belongs.", ""},
	{"The Golden Thread", "ESFJ", "swift", "Ties people and meanings together before they drift apart.", ""},
	{"The Lone Mechanic", "ISTP", "strict", "Strips each brick to its working parts and rebuilds it right.", ""},
	{"The Edge Walker", "ISTP", "explore", "Tests the limits by standing on them.", ""},
	{"The Silent Blade", "ISTP", "split", "Cuts one meaning cleanly into two.", ""},
	{"The Thunder Runner", "ESTP", "swift", "Acts first, learns from the impact, acts again.", ""},
	{"The Street Alchemist", "ESTP", "explore", "Turns whatever is lying around into something that works.", ""},
	{"The Daredevil Spark", "ESTP", "split", "Tries the risky meaning just to see it fly.", ""},
	{"The Stage Flame", "ESFP", "swift", "Lights up the moment and plays to every impact.", ""},
	{"The Festival Spirit", "ESFP", "explore", "Brings every neuron to the party.", ""},
	{"The Velvet Performer", "ESFP", "keep", "Remembers how every moment felt, and replays the best ones.", ""},
}

var Archetypes = func() []*Archetype {
	out := make([]*Archetype, len(archetypeRows))
	for i, r := range archetypeRows {
		out[i] = &Archetype{Name: r[0], Type: r[1], Bias: r[2], Line: r[3], Yours: r[4] != "", Personality: personalityOf(r[1]), Decisions: decisionsOf(r[1], r[2])}
	}
	return out
}()

var (
	theRe    = regexp.MustCompile(`^the[` + jsSpace + `]+`)
	nonAlpha = regexp.MustCompile(`[^a-z ]`)
)

func cleanName(x string) string {
	s := jsLower(x)
	s = theRe.ReplaceAllString(s, "") // anchored, so it replaces once like replace() without g
	s = nonAlpha.ReplaceAllString(s, "")
	return jsTrim(s)
}

// FindArchetype is findArchetype(name).
func FindArchetype(name string) *Archetype {
	want := cleanName(name)
	for _, a := range Archetypes {
		if cleanName(a.Name) == want {
			return a
		}
	}
	for _, a := range Archetypes {
		if strings.Contains(cleanName(a.Name), want) {
			return a
		}
	}
	return nil
}
