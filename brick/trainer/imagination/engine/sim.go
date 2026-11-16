// Package engine is a Go port of brick's imagination-neuron engine (../sim.js). It gives the same results as the
// JavaScript, bit for bit, on the same seed: same random numbers in the same order, same floating-point
// operations in the same order (with V8's own exp/log/sin/cos, see fdlibm.go), same Map and object orders.
// Comments name the sim.js function each part mirrors; read sim.js for what the physics means.
package engine

import (
	"math"
	"slices"
	"sort"
	"strings"
)

var nan = math.NaN()

// Energies, in sim.js's ENERGIES order (also PROOF's order).
const (
	eMechanical = iota
	eHeat
	eNuclear
	eRadiant
	eRest
	eThermal
	eGravity
	eIonization
	ePotential
	eKinetic
	eChemical
	eElastic
	eElectric
	eMagnetic
	eWork
	eOscillation
	nEnergies
)

var EnergyKeys = [nEnergies]string{"mechanical", "heat", "nuclear", "radiant", "rest", "thermal", "gravity", "ionization",
	"potential", "kinetic", "chemical", "elastic", "electric", "magnetic", "work", "oscillation"}

func energyIndex(k string) int {
	for i, x := range EnergyKeys {
		if x == k {
			return i
		}
	}
	return -1
}

// The constants of sim.js's C. Variables, not Go constants: Go would fold constant expressions exactly.
var (
	cDt          = 0.05
	cSIGMA       = 0.6
	cSYNC        = 0.45
	cTHOT        = 1.0
	cTCOLD       = 0.5
	cTREST       = 0.2
	cK           = 1.6
	cRELAY       = 1.2
	cDECAY       = 1.6
	cABSORB      = 0.5
	cREST        = 0.25
	cHEAT        = 0.8
	cTHERMAL     = 0.6
	cRADIANT     = 0.6
	cGRAVITY     = 0.15
	cION         = 0.12
	cGAP         = 0.18
	cELECTRIC    = 0.6
	cCHEM        = 0.04
	cELASTIC     = 0.02
	cFIELD       = 0.08
	cWORK        = 0.35
	cOSC         = 0.5
	cSHARE       = 0.25
	cSPLITGAP    = 0.4
	cMEANGAP     = 1.0
	cCOMIN       = 0.15
	cCOLINK      = 0.1
	cMINGROUP    = 2
	cNOISE       = 0.6
	cLITDECAY    = 0.4
	cINBOXDECAY  = 0.015
	cFANOUT      = 2.0
	cIMAGINE     = 20.0
	TrainEvery   = 20.0
	piVar        = math.Pi
	ninetyPctVar = 0.9
)

var orderWords = map[string]bool{"then": true, "after": true, "before": true, "next": true, "first": true, "finally": true,
	"until": true, "while": true, "when": true, "once": true, "afterwards": true, "later": true}
var flipWords = map[string]bool{"not": true, "don't": true, "dont": true, "never": true, "unless": true, "no": true,
	"without": true, "stop": true, "nor": true}

func special(w string) bool { return len(w) > 0 && (w[0] == '=' || w[0] == '@' || w[0] == '#') }

// ostr is a value that is a string, null or undefined (group, truth).
type ostr struct {
	kind uint8 // 0 undefined, 1 null, 2 string
	s    string
}

func (o ostr) truthy() bool { return o.kind == 2 && o.s != "" }
func strOrNull(s string) ostr {
	if s == "" {
		return ostr{kind: 1}
	}
	return ostr{kind: 2, s: s}
}
func ostrFromJV(v *JV) ostr {
	if v == nil {
		return ostr{}
	}
	if v.Kind == JNull {
		return ostr{kind: 1}
	}
	return ostr{kind: 2, s: v.String()}
}

// orNull is `v || null` for a JSON value.
func orNullJV(v *JV) ostr {
	if !v.Truthy() {
		return ostr{kind: 1}
	}
	return ostr{kind: 2, s: v.String()}
}

type Neuron struct {
	id                        int
	Label, Word               string
	group, truth              ostr
	Omega, Omega0, Theta, Amp float64
	E, T, Lit                 float64
	inbox, co, next, out      OMap[int, float64]
	stepIn, stepHeat          OMap[int, float64]
	stepThermal, stepLit      OMap[int, float64]
	Fixed, Hidden             bool
	Seen                      float64
	litBy                     int // -1: null
	refined                   int
	X, Y                      float64
	relayed                   float64
	Role                      string // "" = none
	parts                     []string
	phrase                    bool
	parentLabel               string
	adj                       []*Link // this neuron's links, in the links Map's order (linksOf)
}

func (n *Neuron) ID() int { return n.id }

type Link struct {
	A, B          int
	W, W0         float64
	Carried, Idle float64
	Tent          float64
	HasTent       bool
	Ne            bool
	Proved        *JV // nil: not proved
	Bits          bool
}

func (l *Link) other(id int) int {
	if l.A == id {
		return l.B
	}
	return l.A
}

// Sent is a sentence (a JS array of words that may carry .out).
type Sent struct {
	W      []string
	Out    []string
	HasOut bool
}

func (s *Sent) includes(w string) bool {
	for _, x := range s.W {
		if x == w {
			return true
		}
	}
	return false
}

type LogEntry struct {
	T    float64
	Text string
}

type Decisions struct {
	Strict, Split, Patience, Radiant, Replay, React float64
}

var Neutral = Decisions{Strict: 0.5, Split: 0.4, Patience: 40, Radiant: 0.5, Replay: 0, React: 0}

func pkey(a, b int) uint64 {
	if a < b {
		return uint64(a)<<32 | uint64(uint32(b))
	}
	return uint64(b)<<32 | uint64(uint32(a))
}

type Network struct {
	intuition                     bool
	ni                            []int8
	proposed, confirmed, rejected float64
	lastSentence                  *Sent
	rand                          *Rng
	Brain                         *Archetype
	e, base                       [nEnergies]float64
	D                             Decisions
	Warmth, OrderPull             float64
	T                             float64
	Steps                         float64
	nextID                        int
	Log                           []LogEntry
	N                             []*Neuron
	links                         OMap[uint64, *Link]
	byID                          []*Neuron
	wordIdx                       map[string][]*Neuron
	Sentences                     []*Sent
	senseCount                    NumObj
	banned, checked, phrases      StrSet
	bits                          []int8
	Batch                         float64
	predCache                     *predSnap
	alikeMarks, nearMarks         marks
	alikeCache                    alikeX
	normPass, normEnded           bool
	Encoded                       *JV // run.js's job.omegas: the encoder's starting frequencies, by word
	nearList                      []int
	rarity                        []float64
	rarityOK                      bool

	// scratch for step()
	dTheta, inc, dOmega []float64
	fan, share          []float64
	sb                  stepBuf
	structVer           uint64 // bumped whenever N changes (add, remove, load)
	items               []spreadItem
}

type SpecNeuron struct {
	Label, Word  string
	Group, Truth string
	Omega        float64
	X, Y         float64
	HasX, HasY   bool
	Fixed        bool
	Hidden       bool
}

type SpecLink struct {
	A, B string
	W    float64
}

type Spec struct {
	Neurons   []SpecNeuron
	Links     []SpecLink
	Sentences []*Sent
}

// NewNetwork is sim.js's `new Network(spec, seed, energies, brain, { intuition })`.
func NewNetwork(spec *Spec, seed float64, energies map[string]float64, brain *Archetype, intuition bool) *Network {
	net := &Network{intuition: intuition, rand: NewRng(seed), Brain: brain, wordIdx: map[string][]*Neuron{}}
	for i := 0; i < nEnergies; i++ {
		lean := 1.0
		if brain != nil {
			lean = brain.Personality.Energies[i]
		}
		m := 1.0
		if v, ok := energies[EnergyKeys[i]]; ok {
			m = v
		}
		net.e[i] = lean * m
	}
	net.base = net.e
	net.D = Neutral
	if brain != nil {
		net.D = brain.Decisions
	}
	net.Warmth, net.OrderPull = 1, 1
	for i := range spec.Neurons {
		net.addSpec(&spec.Neurons[i])
	}
	for _, l := range spec.Links {
		net.connect(net.byLabel(l.A), net.byLabel(l.B), l.W)
	}
	net.Sentences = spec.Sentences
	return net
}

func (net *Network) newNeuron(label, word string, omega float64) *Neuron {
	r := net.rand
	n := &Neuron{id: net.nextID, Label: label, Word: word, Omega: omega, Omega0: omega, Amp: 0.6, T: cTREST, litBy: -1}
	net.nextID++
	n.Theta = r.Next() * piVar * 2
	return n
}

func (net *Network) push(n *Neuron) {
	net.N = append(net.N, n)
	for len(net.byID) <= n.id {
		net.byID = append(net.byID, nil)
	}
	net.byID[n.id] = n
	net.wordIdx[n.Word] = append(net.wordIdx[n.Word], n)
	net.structVer++
	net.dropCaches() // a new neuron: the cached tables are out of date
}

// addSpec is add(n) for a neuron described in Go.
func (net *Network) addSpec(s *SpecNeuron) *Neuron {
	word := s.Word
	if word == "" {
		word = s.Label
	}
	n := net.newNeuron(s.Label, word, s.Omega)
	n.group = strOrNull(s.Group)
	n.Fixed = s.Fixed
	n.truth = strOrNull(s.Truth)
	if s.HasX {
		n.X = s.X
	} else {
		n.X = net.rand.Next()
	}
	if s.HasY {
		n.Y = s.Y
	} else {
		n.Y = net.rand.Next()
	}
	n.Hidden = s.Hidden
	net.push(n)
	return n
}

// addJV is add(n) for a neuron from a saved file (load()).
func (net *Network) addJV(v *JV) *Neuron {
	label := v.Get("label").String()
	word := label
	if w := v.Get("word"); w.Truthy() {
		word = w.String()
	}
	n := net.newNeuron(label, word, v.Get("omega").Num())
	n.group = orNullJV(v.Get("group"))
	n.Fixed = v.Get("fixed").Truthy()
	n.truth = orNullJV(v.Get("truth"))
	if s := v.Get("seen"); s.Truthy() {
		n.Seen = s.Num()
	}
	if x := v.Get("x"); !x.IsNullish() {
		n.X = x.Num()
	} else {
		n.X = net.rand.Next()
	}
	if y := v.Get("y"); !y.IsNullish() {
		n.Y = y.Num()
	} else {
		n.Y = net.rand.Next()
	}
	n.Hidden = v.Get("hidden").Truthy()
	net.push(n)
	return n
}

func (net *Network) byLabel(label string) *Neuron {
	for _, n := range net.N {
		if n.Label == label {
			return n
		}
	}
	return nil
}

func (net *Network) Get(id int) *Neuron {
	if id >= 0 && id < len(net.byID) {
		return net.byID[id]
	}
	return nil
}

func (net *Network) alive(n *Neuron) bool { return n != nil && net.Get(n.id) == n }

func (net *Network) link(a, b *Neuron) *Link {
	if a == nil || b == nil {
		return nil
	}
	x, other := a, b.id
	if len(b.adj) < len(a.adj) {
		x, other = b, a.id
	}
	if a == b {
		return nil
	}
	for _, l := range x.adj {
		if l.A == other || l.B == other {
			return l
		}
	}
	return nil
}

func removeLink(s []*Link, l *Link) []*Link {
	for i, x := range s {
		if x == l {
			copy(s[i:], s[i+1:])
			s[len(s)-1] = nil
			return s[:len(s)-1]
		}
	}
	return s
}

func replaceLink(s []*Link, old, nu *Link) {
	for i, x := range s {
		if x == old {
			s[i] = nu
			return
		}
	}
}

func (net *Network) connect(a, b *Neuron, w float64) {
	if a == nil || b == nil || a == b {
		return
	}
	k := pkey(a.id, b.id)
	l := &Link{A: a.id, B: b.id, W: w, W0: w}
	if old, ok := net.links.Get(k); ok {
		net.links.Set(k, l) // a JS Map keeps the old place
		replaceLink(a.adj, old, l)
		replaceLink(b.adj, old, l)
		return
	}
	net.links.Set(k, l)
	a.adj = append(a.adj, l)
	b.adj = append(b.adj, l)
}

func (net *Network) deleteLink(l *Link) {
	net.links.Delete(pkey(l.A, l.B))
	if a := net.Get(l.A); a != nil {
		a.adj = removeLink(a.adj, l)
	}
	if b := net.Get(l.B); b != nil {
		b.adj = removeLink(b.adj, l)
	}
}

func (net *Network) say(text string) {
	net.Log = append(net.Log, LogEntry{T: net.T, Text: text})
	if len(net.Log) > 200 {
		net.Log = net.Log[1:]
	}
}

// byWord lists the neurons of each word in N order (N is always in id order).
func (net *Network) unionWords(words []string, skip string) []*Neuron {
	var out []*Neuron
	seen := map[string]bool{}
	for _, w := range words {
		if w == skip || seen[w] {
			continue
		}
		seen[w] = true
		out = append(out, net.wordIdx[w]...)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].id < out[j].id })
	return out
}

func uniqueWords(words []string) []string {
	seen := make(map[string]bool, len(words))
	out := make([]string, 0, len(words))
	for _, w := range words {
		if !seen[w] {
			seen[w] = true
			out = append(out, w)
		}
	}
	return out
}

// Stimulate is stimulate(words, strength).
func (net *Network) Stimulate(words []string, strength float64) {
	set := uniqueWords(words)
	hit := net.unionWords(set, "\x00none")
	for _, n := range hit {
		n.E += strength
	}
	var chosen []*Neuron
	for _, word := range set {
		senses := net.wordIdx[word]
		if len(senses) < 2 {
			chosen = append(chosen, senses...)
			continue
		}
		others := make([]*Neuron, 0, len(hit))
		for _, o := range hit {
			if o.Word != word {
				others = append(others, o)
			}
		}
		fit := func(sn *Neuron) float64 {
			t := 0.0
			for _, o := range others {
				w := 0.0
				if l := net.link(sn, o); l != nil {
					w = l.W
				}
				_, fsim, _ := syncOf(sn, o)
				t = t + w + fsim
			}
			return t
		}
		best := senses[0]
		for _, sn := range senses[1:] {
			if fit(sn) > fit(best) {
				best = sn
			}
		}
		chosen = append(chosen, best)
	}
	for _, a := range chosen {
		for _, b := range chosen {
			if a.Word != b.Word {
				v, _ := a.co.Get(b.id)
				a.co.Set(b.id, v+strength)
			}
		}
	}
}

// cmpDiff turns a JS comparator's difference into a sort order (NaN and zero: equal).
func cmpDiff(d float64) int {
	if d < 0 {
		return -1
	}
	if d > 0 {
		return 1
	}
	return 0
}

func syncOf(a, b *Neuron) (df, fsim, s float64) {
	df = a.Omega - b.Omega
	q := df / cSIGMA
	fsim = Exp(-(q * q))
	s = fsim * (1 + Cos(b.Theta-a.Theta)) / 2
	return
}

func decayMap(m *OMap[int, float64], decay float64) {
	m.iter++
	for i := 0; i < len(m.e); i++ {
		if m.e[i].dead {
			continue
		}
		nv := m.e[i].v * decay
		if nv < 1e-4 {
			m.e[i].dead = true
			m.e[i].v = 0
			if m.idx != nil {
				delete(m.idx, m.e[i].k)
			}
			m.live--
		} else {
			m.e[i].v = nv
		}
	}
	m.iter--
	m.maybeCompact()
}

// linkList is [...this.links.values()] (a snapshot, so links deleted on the way are still visited, like
// iterating a copy; callers that delete while iterating use the Map itself).
func (net *Network) linkList() []*Link {
	out := make([]*Link, 0, net.links.Len())
	for i := range net.links.e {
		if !net.links.e[i].dead {
			out = append(out, net.links.e[i].v)
		}
	}
	return out
}

func (net *Network) heatNow() float64 {
	t := 0.0
	for _, n := range net.N {
		t += n.T
	}
	return t / math.Max(1, float64(len(net.N)))
}

func (net *Network) orderNow() float64 {
	s, c := 0.0, 0
	for i := range net.links.e {
		if net.links.e[i].dead {
			continue
		}
		l := net.links.e[i].v
		a, b := net.Get(l.A), net.Get(l.B)
		if a != nil && b != nil {
			_, _, ss := syncOf(a, b)
			s += ss
			c++
		}
	}
	if c == 0 {
		return 0
	}
	return s / float64(c)
}

func truthyNum(x float64) bool { return x != 0 && x == x }

func (net *Network) steer() {
	if net.Brain == nil || !truthyNum(net.D.React) {
		return
	}
	tg := net.Brain.Personality.Target
	k := net.D.React
	clamp := func(v float64) float64 { return jsMin(2, jsMax(0.5, v)) }
	net.Warmth = clamp(net.Warmth + k*(tg.Heat-net.heatNow()))
	net.OrderPull = clamp(net.OrderPull + 4*k*(tg.Order-net.orderNow()))
	e, b := &net.e, &net.base
	if truthyNum(b[eMechanical]) {
		e[eMechanical] = b[eMechanical] * net.Warmth
	}
	if truthyNum(b[eRest]) {
		e[eRest] = b[eRest] / net.Warmth
	}
	if truthyNum(b[eGravity]) {
		e[eGravity] = b[eGravity] * net.OrderPull
	}
	if truthyNum(b[eChemical]) {
		e[eChemical] = b[eChemical] / net.OrderPull
	}
}

type spreadItem struct {
	s *Neuron
	v float64
}

type spreadResult struct {
	ok, bimodal bool
	total, mean float64
}

// spread(a) on the inbox: only total, mean and bimodal are used.
func (net *Network) spread(a *Neuron) spreadResult {
	items := net.items[:0]
	for i := range a.inbox.e {
		en := &a.inbox.e[i]
		if en.dead {
			continue
		}
		if s := net.Get(en.k); s != nil && s.Word != a.Word {
			items = append(items, spreadItem{s, en.v})
		}
	}
	net.items = items
	if len(items) == 0 {
		return spreadResult{}
	}
	slices.SortStableFunc(items, func(x, y spreadItem) int { return cmpDiff(x.s.Omega - y.s.Omega) })
	total := 0.0
	for _, x := range items {
		total += x.v
	}
	mt := 0.0
	for _, x := range items {
		mt += x.v * x.s.Omega
	}
	mean := mt / total
	bestK, bestGap, bestShare := -1, 0.0, 0.0
	left := 0.0
	splitGap := cSPLITGAP * cSIGMA
	for k := 0; k < len(items)-1; k++ {
		left += items[k].v
		gap := items[k+1].s.Omega - items[k].s.Omega
		share := jsMin(left, total-left) / total
		if share > cSHARE && gap > splitGap && (bestK < 0 || gap*share > bestGap*bestShare) {
			bestK, bestGap, bestShare = k, gap, share
		}
	}
	if bestK < 0 {
		return spreadResult{ok: true, total: total, mean: mean}
	}
	m := func(xs []spreadItem) float64 {
		t := 0.0
		for _, x := range xs {
			t += x.v * x.s.Omega
		}
		w := 0.0
		for _, x := range xs {
			w += x.v
		}
		return t / w
	}
	loMean, hiMean := m(items[:bestK+1]), m(items[bestK+1:])
	if hiMean-loMean < cMEANGAP*cSIGMA {
		return spreadResult{ok: true, total: total, mean: mean}
	}
	return spreadResult{ok: true, bimodal: true, total: total, mean: mean}
}

type ctxResult struct {
	ok, bimodal bool
	total       float64
	lo, hi      []spreadItem
	loMean      float64
	hiMean      float64
	share       float64
}

// contexts(a).
func (net *Network) contexts(a *Neuron) ctxResult {
	var partners []spreadItem
	top := 0.0
	for id, v := range a.co.All() {
		if s := net.Get(id); s != nil && s.Word != a.Word {
			partners = append(partners, spreadItem{s, v})
			top = jsMax(top, v)
		}
	}
	var strong []spreadItem
	lim := cCOMIN * top
	for _, p := range partners {
		if p.v > lim {
			strong = append(strong, p)
		}
	}
	if len(strong) < 2 {
		return ctxResult{}
	}
	seen := map[int]bool{}
	var groups [][]spreadItem
	for _, p := range strong {
		if seen[p.s.id] {
			continue
		}
		var group []spreadItem
		todo := []spreadItem{p}
		seen[p.s.id] = true
		for len(todo) > 0 {
			x := todo[len(todo)-1]
			todo = todo[:len(todo)-1]
			group = append(group, x)
			for _, q := range strong {
				if seen[q.s.id] {
					continue
				}
				v1, _ := x.s.co.Get(q.s.id)
				v2, _ := q.s.co.Get(x.s.id)
				together := jsMax(v1, v2)
				line := net.link(x.s, q.s)
				if together > cCOLINK*jsMin(x.v, q.v) || (line != nil && line.W > 0.2) {
					seen[q.s.id] = true
					todo = append(todo, q)
				}
			}
		}
		groups = append(groups, group)
	}
	total := 0.0
	for _, x := range strong {
		total += x.v
	}
	weight := func(g []spreadItem) float64 {
		t := 0.0
		for _, x := range g {
			t += x.v
		}
		return t
	}
	sort.SliceStable(groups, func(x, y int) bool { return weight(groups[y])-weight(groups[x]) < 0 })
	big := groups[:0]
	for _, g := range groups {
		if len(g) >= cMINGROUP {
			big = append(big, g)
		}
	}
	groups = big
	if len(groups) < 2 || weight(groups[1])/total < cSHARE {
		return ctxResult{ok: true, total: total}
	}
	m := func(xs []spreadItem) float64 {
		t := 0.0
		for _, x := range xs {
			t += x.v * x.s.Omega
		}
		return t / weight(xs)
	}
	lo, hi := groups[0], groups[1]
	if m(lo) > m(hi) {
		lo, hi = hi, lo
	}
	return ctxResult{ok: true, bimodal: true, total: total, lo: lo, hi: hi, loMean: m(lo), hiMean: m(hi), share: weight(groups[1]) / total}
}

// imagine(): the imagination layer.
func (net *Network) imagine() {
	e := &net.e
	if net.intuition {
		net.decide()
	}
	for _, a := range append([]*Neuron(nil), net.N...) {
		if a.T >= cTCOLD || a.Fixed {
			continue
		}
		sp := net.contexts(a)
		if !sp.ok || !sp.bimodal || sp.total < 0.05 {
			continue
		}
		score := e[eNuclear]*sp.share*2*(0.5+0.5*e[eElectric]) - 0.25*e[eChemical]
		if score > net.D.Split {
			net.fission(a, &sp)
		}
	}
	if e[eElectric] < 0.5 {
		for i := 0; i < len(net.N); i++ {
			for j := i + 1; j < len(net.N); j++ {
				a, b := net.N[i], net.N[j]
				if a.Word == b.Word && math.Abs(a.Omega-b.Omega) < 0.12 && e[eNuclear]*(1-e[eElectric]) > 0.5 {
					net.fusion(a, b)
					return
				}
			}
		}
	}
	lim := net.D.Patience / jsMax(0.2, e[eWork])
	for _, l := range net.links.All() {
		if l.Carried > 0.01 {
			l.W = jsMin(1, l.W+0.02*e[eGravity])
		}
		if l.Idle > lim {
			l.W -= 0.05
			if l.W <= 0.03 {
				net.deleteLink(l)
				a, b := net.Get(l.A), net.Get(l.B)
				net.say("refinement: the unused line " + a.Label + "—" + b.Label + " faded away")
			}
		}
	}
	for i := 0; i < len(net.N); i++ {
		a := net.N[i]
		if a.Lit < net.D.Radiant || a.litBy < 0 || len(a.adj) > 0 {
			continue
		}
		src := net.Get(a.litBy)
		if src == nil || e[eRadiant] <= 0 {
			continue
		}
		a.Omega = a.Omega + 0.6*(src.Omega-a.Omega)
		a.Omega0 = a.Omega
		net.connect(a, src, 0.3)
		a.Hidden = false
		net.say("radiant: hidden " + a.Label + " was lit up by " + src.Label + "; the imagination layer tuned it closer and joined them")
	}
}

// ---------- intuition ----------

var functions = map[string][2]int{
	"Ni": {eMechanical, eWork}, "Ne": {eRadiant, eOscillation}, "Ti": {eNuclear, eGravity}, "Te": {eKinetic, eIonization},
	"Fi": {eElectric, eChemical}, "Fe": {eHeat, eMagnetic}, "Si": {eRest, ePotential}, "Se": {eThermal, eElastic},
}
var functionOrder = []string{"Ni", "Ne", "Ti", "Te", "Fi", "Fe", "Si", "Se"}

func (net *Network) fnPower(fn string) float64 {
	k := functions[fn]
	return (net.e[k[0]] + net.e[k[1]]) / 2
}

func (net *Network) senseIn(word string, words []string) *Neuron {
	senses := net.wordIdx[word]
	if len(senses) < 2 {
		if len(senses) == 0 {
			return nil
		}
		return senses[0]
	}
	others := net.unionWords(words, word)
	fit := func(sn *Neuron) float64 {
		t := 0.0
		for _, o := range others {
			w := 0.0
			if l := net.link(sn, o); l != nil {
				w = l.W
			}
			_, fsim, _ := syncOf(sn, o)
			t = t + w + fsim
		}
		return t
	}
	best := senses[0]
	for _, sn := range senses[1:] {
		if fit(sn) > fit(best) {
			best = sn
		}
	}
	return best
}

func (net *Network) intuit(s *Sent) {
	words := s.W
	var seq []*Neuron
	for _, w := range words {
		if len(w) > 0 && (w[0] == '=' || w[0] == '@') {
			continue
		}
		if x := net.senseIn(w, words); x != nil {
			seq = append(seq, x)
		}
	}
	for _, x := range seq {
		x.Seen++
	}
	start := 0
	ni, ne, te := net.fnPower("Ni"), net.fnPower("Ne"), net.fnPower("Te")
	R := 0.5
	var score OMap[int, float64]
	for i := 1; i < len(seq); i++ {
		x := seq[i]
		score.Clear()
		if orderWords[seq[i-1].Word] {
			start = i - 1
		}
		total := 0.0
		for j, r := i-1, 1.0; j >= start && r > 0.1; j, r = j-1, r*R {
			for id, v := range seq[j].next.All() {
				old, _ := score.Get(id)
				score.Set(id, old+r*v)
				total += r * v
			}
		}
		best, top := -1, 0.0
		for id, v := range score.All() {
			if v > top {
				top = v
				best = id
			}
		}
		p := 0.0
		if total > 0 {
			sv, _ := score.Get(x.id)
			if sv != sv {
				sv = 0
			}
			p = sv / (total + 0.2)
		}
		if best == x.id {
			net.ni = append(net.ni, 1)
		} else {
			net.ni = append(net.ni, 0)
		}
		if len(net.ni) > 300 {
			net.ni = net.ni[1:]
		}
		surprise := 1 - p
		eta := 0.25 * ni
		lim := 0.35 / jsMax(0.3, te)
		for j, r := i-1, 1.0; j >= start && r > 0.1; j, r = j-1, r*R {
			c := seq[j]
			k := eta * r
			if c.Word == x.Word {
				continue
			}
			keep := 1 - 0.1*k
			for id, v := range c.next.All() {
				if id != x.id {
					nv := v * keep
					if nv < 1e-3 {
						c.next.Delete(id)
					} else {
						c.next.Set(id, nv)
					}
				}
			}
			old, _ := c.next.Get(x.id)
			nv := old + k*surprise*(1-old)
			c.next.Set(x.id, nv)
			if nv > lim && net.link(c, x) == nil {
				net.connect(c, x, 0.3)
			}
		}
		x.T += cABSORB * net.e[eMechanical] * surprise * 0.4
		if p > 0.5 && x.T < cTCOLD {
			x.Amp = jsMin(1, x.Amp+0.05*net.e[eWork])
		}
		if surprise > 0.6 && net.rand.Next() < 0.5*ne {
			net.propose(x)
		}
	}
	net.predCache = nil
	net.rarityOK = false
}

func (net *Network) rarityOf() {
	total := 0.0
	for _, n := range net.N {
		total += n.Seen
	}
	total = orOne(total)
	out := make([]float64, net.nextID)
	for i := range out {
		out[i] = 1 // `rare.get(k) || 1` for a neuron the snapshot does not have
	}
	den := Log(1 + total)
	for _, n := range net.N {
		out[n.id] = orOne(Log(1+total/(1+n.Seen)) / den)
	}
	net.rarity = out
	net.rarityOK = true
}

func (net *Network) decide() {
	pred := net.predIndex()
	te, fe, fi := net.fnPower("Te"), net.fnPower("Fe"), net.fnPower("Fi")
	net.normPass = true // sim.js: this._norm = new Map() (see AlikeZeroOutsidePass)
	for _, l := range net.links.All() {
		a, b := net.Get(l.A), net.Get(l.B)
		if a == nil || b == nil {
			continue
		}
		if l.HasTent && net.T-l.Tent > 8 {
			if net.alike(a, b, pred) > 0.35*fi {
				l.W = jsMin(1, l.W+0.08*te)
				if l.W >= 0.3 {
					l.HasTent = false
					l.W0 = l.W
					l.Ne = true
					net.confirmed++
					net.say("Ne proposed, Te kept: " + a.Label + "—" + b.Label + " (used in the same places)")
				}
			} else {
				l.W -= 0.05 * jsMax(0.2, te)
				if l.W <= 0.03 {
					net.deleteLink(l)
					net.rejected++
					continue
				}
			}
		}
		if !l.HasTent {
			pull := fe * 0.02 * l.W * (b.Omega - a.Omega)
			if a.Fixed && b.Fixed {
				continue
			}
			if a.Fixed {
				b.Omega -= 2 * pull
			} else if b.Fixed {
				a.Omega += 2 * pull
			} else {
				a.Omega += pull
				b.Omega -= pull
			}
		}
	}
	net.normPass, net.normEnded = false, true // sim.js: this._norm = null
	for _, n := range net.N {
		if !n.Fixed {
			n.Omega0 += 0.05 * (n.Omega - n.Omega0)
		}
	}
	net.fusePhrases(pred)
	if math.Mod(net.Steps, cIMAGINE*10) == 0 {
		net.assignSlots()
	}
}

// ---------- bitstream neurons ----------

func (net *Network) firstOfWord(w string) *Neuron {
	if l := net.wordIdx[w]; len(l) > 0 {
		return l[0]
	}
	return nil
}

func (net *Network) learnBits(s *Sent) {
	want := map[string]bool{}
	for _, o := range s.Out {
		want[o] = true
	}
	var outs []*Neuron
	for _, o := range s.Out {
		if n := net.firstOfWord(o); n != nil {
			outs = append(outs, n)
		}
	}
	if len(outs) == 0 {
		return
	}
	var seq []*Neuron
	for _, w := range s.W {
		if x := net.senseIn(w, s.W); x != nil && !special(x.Word) {
			seq = append(seq, x)
		}
	}
	flipped := map[*Neuron]bool{}
	for i, x := range seq {
		if flipWords[x.Word] && i+1 < len(seq) {
			flipped[seq[i+1]] = true
		}
	}
	var kept []*Neuron
	for _, x := range seq {
		if !flipped[x] {
			kept = append(kept, x)
		}
	}
	guess := net.guessBits(kept)
	if len(guess) > 0 {
		if want[guess[0].o] {
			net.bits = append(net.bits, 1)
		} else {
			net.bits = append(net.bits, 0)
		}
		if len(net.bits) > 300 {
			net.bits = net.bits[1:]
		}
	}
	eta, te := 0.2*net.fnPower("Ni"), net.fnPower("Te")
	keep := 1 - 0.15*eta
	lim := 0.5 / jsMax(0.3, te)
	for _, x := range seq {
		if flipped[x] {
			continue
		}
		for id, v := range x.out.All() {
			isOut := false
			for _, o := range outs {
				if o.id == id {
					isOut = true
					break
				}
			}
			if !isOut {
				nv := v * keep
				if nv < 1e-3 {
					x.out.Delete(id)
				} else {
					x.out.Set(id, nv)
				}
			}
		}
		for _, o := range outs {
			old, _ := x.out.Get(o.id)
			old2, _ := x.out.Get(o.id)
			nv := old + eta*(1-old2)
			x.out.Set(o.id, nv)
			if nv > lim && x.Seen >= 3 && net.link(x, o) == nil {
				net.connect(x, o, 0.4)
				if l := net.link(x, o); l != nil {
					l.Bits = true
				}
			}
		}
	}
}

type guess struct {
	o     string
	share float64
}

func (net *Network) guessBits(seq []*Neuron) []guess {
	var score OMap[int, float64]
	for _, x := range seq {
		m := &x.out
		if m.Len() == 0 {
			m = &OMap[int, float64]{}
			for _, l := range x.adj {
				if l.HasTent {
					continue
				}
				o := net.Get(l.other(x.id))
				if o != nil && !special(o.Word) {
					for id, v := range o.out.All() {
						old, _ := m.Get(id)
						m.Set(id, old+v*l.W)
					}
				}
			}
		}
		for id, v := range m.All() {
			old, _ := score.Get(id)
			score.Set(id, old+v)
		}
	}
	total := 0.0
	for _, v := range score.All() {
		total += v
	}
	var out []guess
	for id, v := range score.All() {
		n := net.Get(id)
		if n == nil || n.Word == "" {
			continue
		}
		sh := 0.0
		if truthyNum(total) {
			sh = v / total
		}
		out = append(out, guess{n.Word, sh})
	}
	sort.SliceStable(out, func(i, j int) bool { return out[j].share-out[i].share < 0 })
	return out
}

// BrickGuess is brickOf()'s answer.
type BrickGuess struct {
	Output    string
	Share     float64
	Second    string
	HasSecond bool
}

func (net *Network) BrickOf(word string) *BrickGuess {
	cands := append([]*Neuron(nil), net.wordIdx[word]...)
	if len(cands) == 0 {
		return nil
	}
	sort.SliceStable(cands, func(i, j int) bool { return cands[j].Seen-cands[i].Seen < 0 })
	g := net.guessBits(cands[:1])
	if len(g) == 0 {
		return nil
	}
	b := &BrickGuess{Output: g[0].o, Share: toFixedNum(g[0].share, 3)}
	if len(g) > 1 {
		b.Second, b.HasSecond = g[1].o, true
	}
	return b
}

func (net *Network) BitRate() float64 {
	if len(net.bits) == 0 {
		return 0
	}
	s := 0.0
	for _, b := range net.bits {
		s += float64(b)
	}
	return s / float64(len(net.bits))
}

// ---------- slot neurons ----------

// Slots is slots(roles): roles are the values of the roles object (order does not matter).
func (net *Network) Slots(roleValues []string) {
	band := jsMax(4, float64(len(net.N))/25)
	kinds := uniqueWords(roleValues)
	sort.SliceStable(kinds, func(i, j int) bool { return lessUTF16(kinds[i], kinds[j]) })
	for k, r := range kinds {
		w := "@" + r
		if len(net.wordIdx[w]) == 0 {
			kk := float64(k)
			nk := float64(len(kinds))
			net.addSpec(&SpecNeuron{Label: w, Omega: 2 + band*(kk+0.5)/nk, X: 0.05 + ninetyPctVar*(kk+0.5)/nk, HasX: true, Y: 0.97, HasY: true, Fixed: true})
		}
	}
	for _, n := range net.N {
		if n.Role == "" || special(n.Word) {
			continue
		}
		slot := net.firstOfWord("@" + n.Role)
		if slot != nil && net.link(n, slot) == nil {
			net.connect(n, slot, 0.3)
			if l := net.link(n, slot); l != nil {
				l.Proved = &JV{Kind: JStr, S: "role"}
			}
		}
	}
}

func (net *Network) assignSlots() {
	for _, n := range net.N {
		if n.Role != "" || special(n.Word) || n.Seen < 3 {
			continue
		}
		hasSlot := false
		for _, l := range n.adj {
			if o := net.Get(l.other(n.id)); o != nil && len(o.Word) > 0 && o.Word[0] == '@' {
				hasSlot = true
				break
			}
		}
		if hasSlot {
			continue
		}
		var vote OMap[string, float64]
		total := 0.0
		for _, l := range n.adj {
			if l.HasTent {
				continue
			}
			o := net.Get(l.other(n.id))
			if o != nil && o.Role != "" && !special(o.Word) {
				v, _ := vote.Get(o.Role)
				vote.Set(o.Role, v+1)
				total++
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
		sort.SliceStable(list, func(i, j int) bool { return list[j].v-list[i].v < 0 })
		if len(list) > 0 && list[0].v >= 2 && list[0].v/total >= 0.6 {
			if slot := net.firstOfWord("@" + list[0].k); slot != nil {
				net.connect(n, slot, 0.2)
			}
		}
	}
}

// ---------- phrase neurons ----------

func wordCount(w string) int { return strings.Count(w, " ") + 1 }

func (net *Network) fusePhrases(pred *predSnap) {
	ti := net.fnPower("Ti")
	cp := int(jsMax(20, jsRound(float64(len(net.N))*0.3)))
	if net.phrases.Len() >= cp || ti <= 0 {
		return
	}
	type made struct {
		c, x *Neuron
		word string
	}
	var list []made
	lim := 0.6 / jsMin(1.5, ti)
	for _, c := range net.N {
		if c.Seen < 5 || wordCount(c.Word) >= 4 || special(c.Word) || orderWords[c.Word] {
			continue
		}
		for id, v := range c.next.All() {
			x := net.Get(id)
			if x == nil || x.Seen < 5 || v < lim || x.Word == c.Word || special(x.Word) || orderWords[x.Word] {
				continue
			}
			word := c.Word + " " + x.Word
			if wordCount(word) > 4 || net.phrases.Has(word) {
				continue
			}
			total := 0.0
			_, vs := pred.row(x.id)
			for _, u := range vs {
				total += u
			}
			if truthyNum(total) && v/total > 0.5 {
				list = append(list, made{c, x, word})
			}
		}
	}
	for i := 0; i < len(list) && i < 5; i++ {
		if net.phrases.Len() >= cp {
			break
		}
		c, x, word := list[i].c, list[i].x, list[i].word
		p := net.addSpec(&SpecNeuron{Label: word, Word: word, Omega: (c.Omega + x.Omega) / 2, X: (c.X + x.X) / 2, HasX: true, Y: (c.Y+x.Y)/2 + 0.03, HasY: true})
		p.parts = []string{c.Word, x.Word}
		p.phrase = true
		net.connect(p, c, 0.5)
		net.connect(p, x, 0.5)
		net.phrases.Add(word)
		net.say("Ti fused \"" + word + "\" into one phrase neuron (Ni nearly always sees " + x.Word + " right after " + c.Word + ")")
	}
}

// Chunk is chunk(words): a new sentence when there are phrases, else the same one.
func (net *Network) Chunk(s *Sent) *Sent {
	if net.phrases.Len() == 0 {
		return s
	}
	words := s.W
	var out []string
	for i := 0; i < len(words); {
		took := 1
		for k := min(4, len(words)-i); k > 1; k-- {
			if net.phrases.Has(strings.Join(words[i:i+k], " ")) {
				took = k
				break
			}
		}
		out = append(out, strings.Join(words[i:i+took], " "))
		i += took
	}
	return &Sent{W: out}
}

func (net *Network) NiRate() float64 {
	if len(net.ni) == 0 {
		return 0
	}
	s := 0.0
	for _, b := range net.ni {
		s += float64(b)
	}
	return s / float64(len(net.ni))
}

// ---------- the reality check ----------

type Candidate struct {
	A, B, Original, Swapped string
	Alike                   float64
}

func (net *Network) Candidates(k float64) []Candidate {
	pred := net.predIndex()
	var out []Candidate
	asked := map[string]bool{}
	type line struct {
		a, b *Neuron
		sim  float64
	}
	var lines []line
	for _, l := range net.linkList() {
		a, b := net.Get(l.A), net.Get(l.B)
		if a == nil || b == nil || a.Word == b.Word || special(a.Word) || special(b.Word) || net.checked.Has(pairKey(a.Word, b.Word)) || !(l.HasTent || l.Ne) {
			continue
		}
		lines = append(lines, line{a: a, b: b})
	}
	for i := range lines {
		lines[i].sim = net.alike(lines[i].a, lines[i].b, pred)
	}
	sort.SliceStable(lines, func(i, j int) bool { return lines[j].sim-lines[i].sim < 0 })
	var chunked []*Sent
	for _, ln := range lines {
		if float64(len(out)) >= k {
			break
		}
		key := pairKey(ln.a.Word, ln.b.Word)
		if asked[key] {
			continue
		}
		from, to := ln.b, ln.a
		if net.rand.Next() < 0.5 {
			from, to = ln.a, ln.b
		}
		if chunked == nil {
			chunked = make([]*Sent, len(net.Sentences))
			for i, st := range net.Sentences {
				chunked[i] = net.Chunk(st)
			}
		}
		var hosts []*Sent
		for _, st := range chunked {
			if st.includes(from.Word) && !st.includes(to.Word) {
				hosts = append(hosts, st)
			}
		}
		if len(hosts) == 0 {
			continue
		}
		host := hosts[int(math.Floor(net.rand.Next()*float64(len(hosts))))]
		asked[key] = true
		sw := make([]string, len(host.W))
		for i, w := range host.W {
			if w == from.Word {
				sw[i] = to.Word
			} else {
				sw[i] = w
			}
		}
		out = append(out, Candidate{A: from.Word, B: to.Word, Alike: toFixedNum(ln.sim, 2), Original: strings.Join(host.W, " "), Swapped: strings.Join(sw, " ")})
	}
	return out
}

// ApplyVerdicts is applyVerdicts(verdicts) with the verdicts as parsed JSON.
func (net *Network) ApplyVerdicts(verdicts *JV) {
	if verdicts != nil && verdicts.Kind == JArr {
		for _, v := range verdicts.A {
			va, vb := v.Get("a"), v.Get("b")
			if strictEqual(va, vb) { // a word and itself: nothing to check
				continue
			}
			key := pairKey(va.String(), vb.String())
			net.checked.Add(key)
			ok := v.Get("ok")
			if ok.IsNullish() {
				continue
			}
			level := v.Get("level")
			if !level.Truthy() {
				level = &JV{Kind: JStr, S: "role"}
			}
			w := 0.5
			if level.Kind == JStr && level.S == "meaning" {
				w = 0.7
			}
			if va != nil && va.Kind == JStr && vb != nil && vb.Kind == JStr {
				for _, x := range append([]*Neuron(nil), net.wordIdx[va.S]...) {
					for _, y := range append([]*Neuron(nil), net.wordIdx[vb.S]...) {
						l := net.link(x, y)
						if ok.Truthy() {
							if l != nil {
								l.W = jsMax(l.W, w)
								l.W0 = l.W
								l.HasTent = false
								l.Proved = level
							} else {
								net.connect(x, y, w)
								if l := net.link(x, y); l != nil {
									l.Proved = level
								}
							}
						} else if l != nil {
							net.deleteLink(l)
						}
					}
				}
			}
			if ok.Kind == JBool && !ok.B {
				net.banned.Add(key)
			}
			if ok.Truthy() {
				why := "same role"
				if x := v.Get("why"); x.Truthy() {
					why = x.String()
				}
				net.say("the check agreed: " + va.String() + "—" + vb.String() + " (" + why + ")")
			} else {
				why := "different role"
				if x := v.Get("why"); x.Truthy() {
					why = x.String()
				}
				net.say("the compiler disagreed: " + va.String() + "—" + vb.String() + " cut (" + why + ")")
			}
		}
	}
	net.predCache = nil
}

type RoleAnswer struct{ Role, From string }

func (net *Network) RoleOf(n *Neuron) *RoleAnswer {
	if n.Role != "" {
		return &RoleAnswer{n.Role, "brick's reader"}
	}
	for _, l := range n.adj {
		if o := net.Get(l.other(n.id)); o != nil && len(o.Word) > 0 && o.Word[0] == '@' {
			return &RoleAnswer{o.Word[1:], "its slot neuron"}
		}
	}
	var vote NumObj
	for _, l := range n.adj {
		if l.HasTent {
			continue
		}
		o := net.Get(l.other(n.id))
		if o != nil && o.Role != "" && o.Word != n.Word {
			m := 1.0
			if l.Proved.Truthy() {
				m = 2
			}
			v, _ := vote.Get(o.Role)
			vote.Set(o.Role, v+l.W*m)
		}
	}
	best := bestEntry(&vote)
	if best < 0 {
		return nil
	}
	return &RoleAnswer{vote.keys[best], "learned from its lines"}
}

// bestEntry is Object.entries(o).sort((a, b) => b[1] - a[1])[0]: the position of the winner, or -1.
func bestEntry(o *NumObj) int {
	order := o.Order()
	sort.SliceStable(order, func(i, j int) bool { return o.vals[order[j]]-o.vals[order[i]] < 0 })
	if len(order) == 0 {
		return -1
	}
	return order[0]
}

// AddWords is addWords(words).
func (net *Network) AddWords(words []string) {
	have := map[string]bool{}
	for _, n := range net.N {
		have[n.Word] = true
	}
	band := jsMax(4, float64(len(net.N))/25)
	for _, w := range words {
		if have[w] {
			continue
		}
		have[w] = true
		if v, ok := numberValue(w); ok {
			net.addSpec(&SpecNeuron{Label: w, Omega: numberOmega(v), Fixed: true})
		} else {
			var om float64
			if enc := net.Encoded.Get(w); !enc.IsNullish() { // the encoder's frequency, when it has one
				om = enc.Num()
			} else {
				om = 2 + net.rand.Next()*band
			}
			net.addSpec(&SpecNeuron{Label: w, Omega: om, Fixed: len(w) > 0 && w[0] == '='})
		}
	}
}

func (net *Network) fission(a *Neuron, sp *ctxResult) {
	base := a.Word
	mk := func(part []spreadItem, mean float64, side float64) *Neuron {
		v, _ := net.senseCount.Get(base)
		if v != v {
			v = 0
		}
		suffix := v + 1
		net.senseCount.Set(base, suffix)
		child := net.addSpec(&SpecNeuron{Label: base + "·" + jsNumber(suffix), Word: base, Omega: mean, X: a.X + side*0.05, HasX: true, Y: a.Y + 0.05, HasY: true})
		child.T, child.Amp, child.Theta, child.parentLabel, child.truth = a.T, a.Amp, a.Theta, a.Label, a.truth
		child.next = a.next.Clone()
		for _, x := range net.N {
			if v, ok := x.next.Get(a.id); ok {
				x.next.Set(child.id, v)
			}
		}
		for _, p := range part {
			w := 0.3
			if old := net.link(a, p.s); old != nil {
				w = old.W
			}
			net.connect(child, p.s, w)
		}
		return child
	}
	c1 := mk(sp.lo, sp.loMean, -1)
	c2 := mk(sp.hi, sp.hiMean, 1)
	net.dropCaches() // the children took over the parent's next maps
	for _, l := range append([]*Link(nil), a.adj...) {
		other := net.Get(l.other(a.id))
		if net.link(c1, other) != nil || net.link(c2, other) != nil {
			continue
		}
		near := c2
		if math.Abs(other.Omega-c1.Omega) < math.Abs(other.Omega-c2.Omega) {
			near = c1
		}
		net.connect(near, other, l.W)
	}
	net.remove(a)
	names := func(xs []spreadItem) string {
		c := append([]spreadItem(nil), xs...)
		sort.SliceStable(c, func(i, j int) bool { return c[j].v-c[i].v < 0 })
		var s []string
		for i := 0; i < len(c) && i < 3; i++ {
			s = append(s, c[i].s.Label)
		}
		return strings.Join(s, ", ")
	}
	net.say("nuclear fission: " + a.Label + " held two meanings, so it split into " + c1.Label + " (near " + names(sp.lo) + ") and " + c2.Label + " (near " + names(sp.hi) + ")")
}

func (net *Network) fusion(a, b *Neuron) {
	a.Omega = (a.Omega + b.Omega) / 2
	a.Omega0 = a.Omega
	for _, l := range append([]*Link(nil), b.adj...) {
		other := net.Get(l.other(b.id))
		if other != a && net.link(a, other) == nil {
			net.connect(a, other, l.W)
		}
	}
	net.remove(b)
	net.say("nuclear fusion: " + b.Label + " merged into " + a.Label + " (electric was too weak to keep them apart)")
}

func (net *Network) remove(a *Neuron) {
	for _, l := range append([]*Link(nil), a.adj...) {
		net.deleteLink(l)
	}
	k := 0
	for _, x := range net.N {
		if x != a {
			net.N[k] = x
			k++
		}
	}
	clear(net.N[k:])
	net.N = net.N[:k]
	net.byID[a.id] = nil
	net.structVer++
	ws := net.wordIdx[a.Word]
	for i, x := range ws {
		if x == a {
			ws = append(ws[:i:i], ws[i+1:]...)
			break
		}
	}
	if len(ws) == 0 {
		delete(net.wordIdx, a.Word)
	} else {
		net.wordIdx[a.Word] = ws
	}
	for _, x := range net.N {
		x.inbox.Delete(a.id)
		x.co.Delete(a.id)
		x.next.Delete(a.id)
	}
	net.dropCaches() // gone, and every next map changed
}

// dropCaches forgets predIndex()'s and rarityOf()'s tables (sim.js: this.predCache = null; this.rarity = null).
func (net *Network) dropCaches() {
	net.predCache = nil
	net.rarityOK = false
}

// Train is train(): one training moment.
func (net *Network) Train() *Sent {
	if len(net.Sentences) == 0 {
		return nil
	}
	var s *Sent
	if truthyNum(net.D.Replay) && net.rand.Next() < net.D.Replay {
		var worst *Link
		for _, l := range net.linkList() {
			if l.Idle > net.D.Patience*0.25 && (worst == nil || l.Idle > worst.Idle) {
				worst = l
			}
		}
		if worst != nil {
			a, b := net.Get(worst.A), net.Get(worst.B)
			var both, one []*Sent
			for _, x := range net.Sentences {
				if x.includes(a.Word) && x.includes(b.Word) {
					both = append(both, x)
				}
			}
			one = both
			if len(both) == 0 {
				for _, x := range net.Sentences {
					if x.includes(a.Word) || x.includes(b.Word) {
						one = append(one, x)
					}
				}
			}
			if len(one) > 0 {
				s = one[int(math.Floor(net.rand.Next()*float64(len(one))))]
			}
		}
	}
	if s == nil && net.intuition && net.lastSentence != nil && net.rand.Next() < 0.2*net.fnPower("Si") {
		var like []*Sent
		for _, x := range net.Sentences {
			if x == net.lastSentence {
				continue
			}
			for _, w := range x.W {
				if net.lastSentence.includes(w) {
					like = append(like, x)
					break
				}
			}
		}
		if len(like) > 0 {
			s = like[int(math.Floor(net.rand.Next()*float64(len(like))))]
		}
	}
	if s == nil {
		s = net.Sentences[int(math.Floor(net.rand.Next()*float64(len(net.Sentences))))]
	}
	out, hasOut := s.Out, s.HasOut
	if net.intuition {
		s = net.Chunk(s)
		if hasOut {
			s.Out, s.HasOut = out, true
		}
	}
	strength := 1.2
	if net.intuition {
		strength = 1.2 * jsMin(1.5, net.fnPower("Se"))
	}
	net.Stimulate(s.W, strength)
	if net.intuition {
		net.intuit(s)
		if s.HasOut {
			net.learnBits(s)
		}
		batch := net.Batch
		if !truthyNum(batch) {
			batch = 1
		}
		for k := 1.0; k < batch; k++ {
			raw := net.Sentences[int(math.Floor(net.rand.Next()*float64(len(net.Sentences))))]
			c := net.Chunk(raw)
			if raw.HasOut {
				c.Out, c.HasOut = raw.Out, true
			}
			net.intuit(c)
			if c.HasOut {
				net.learnBits(c)
			}
		}
	}
	net.lastSentence = s
	return s
}

// Tick is run.js's tick: train every TRAIN_EVERY steps, then step.
func (net *Network) Tick() {
	if math.Mod(net.Steps, TrainEvery) == 0 {
		net.Train()
	}
	net.Step()
}

// ---------- reading ----------

type Ranked struct {
	S       *Neuron
	Matched int
	Pct     float64
	Profile []ProfileEntry
}

type ProfileEntry struct {
	Key string
	Pct float64
}

type ReadResult struct {
	Winner  *Neuron
	Ranked  []Ranked
	Carried int
	Verdict string
	Clear   bool
	Meaning ostr
}

// Read is read(word, context, steps).
func (net *Network) Read(word string, context []string, steps int) *ReadResult {
	senses := append([]*Neuron(nil), net.wordIdx[word]...)
	if len(senses) == 0 {
		return nil
	}
	ctx := map[string]bool{}
	for _, w := range context {
		ctx[w] = true
	}
	isCtx := func(n *Neuron) bool { return ctx[n.Word] }
	isSense := map[*Neuron]bool{}
	for _, s := range senses {
		isSense[s] = true
	}
	var sums OMap[int, *energySum]
	add := func(x *Neuron, k int, v float64) {
		if !(v > 0) {
			return
		}
		t, ok := sums.Get(x.id)
		if !ok {
			t = &energySum{}
			sums.Set(x.id, t)
		}
		t.v[k] += v
	}
	dt := cDt
	reach := 1.8 * cSIGMA
	take := func(m *OMap[int, float64]) float64 {
		v := 0.0
		for id, a := range m.All() {
			if src := net.Get(id); src != nil && isCtx(src) {
				v += a
			}
		}
		return v
	}
	for k := 0; k < steps; k++ {
		if k%8 == 0 {
			net.Stimulate(context, 1.0)
			net.Stimulate([]string{word}, 0.4)
		}
		net.Step()
		var cx []*Neuron
		for _, n := range net.N {
			if isCtx(n) {
				cx = append(cx, n)
			}
		}
		if len(cx) == 0 {
			continue
		}
		o0 := 0.0
		for _, n := range cx {
			o0 += n.Omega0
		}
		ctxOmega0 := o0 / float64(len(cx))
		pool := net.N
		if len(net.N) > 120 {
			pool = nil
			for _, r := range net.N {
				if isCtx(r) {
					continue
				}
				in := isSense[r] || r.stepIn.Len() > 0 || r.stepLit.Len() > 0
				if !in {
					for _, o := range cx {
						if math.Abs(o.Omega-r.Omega) < reach || net.link(r, o) != nil {
							in = true
							break
						}
					}
				}
				if in {
					pool = append(pool, r)
				}
			}
		}
		// how much every other reachable neuron overlaps each one: independent per neuron, so worked out in parallel
		// (nothing below changes a frequency or a phase); each sum still runs in pool order
		fsAlls := make([]float64, len(pool))
		parallelFor(len(pool), 64, func(lo, hi int) {
			for pi := lo; pi < hi; pi++ {
				r := pool[pi]
				if isCtx(r) {
					continue
				}
				fsAll := 0.0
				for _, o := range pool {
					if o != r && !isCtx(o) {
						_, f, _ := syncOf(r, o)
						fsAll += f
					}
				}
				fsAlls[pi] = fsAll
			}
		})
		for pi, r := range pool {
			if isCtx(r) {
				continue
			}
			imp := take(&r.stepIn)
			allIn := 0.0
			for _, v := range r.stepIn.All() {
				allIn += v
			}
			add(r, eKinetic, imp)
			add(r, eMechanical, imp*cABSORB)
			add(r, eHeat, take(&r.stepHeat))
			add(r, eThermal, take(&r.stepThermal))
			add(r, eRadiant, take(&r.stepLit))
			if allIn > 0 {
				add(r, ePotential, r.E*imp/allIn*dt)
			} else {
				add(r, ePotential, 0)
			}
			add(r, eWork, take(&r.inbox)*dt)
			q := (r.Omega0 - ctxOmega0) / cSIGMA
			add(r, eChemical, Exp(-(q*q))*dt)
			fsAll := fsAlls[pi]
			for _, o := range cx {
				_, fsim, s := syncOf(r, o)
				l := net.link(r, o)
				add(r, eRest, s*dt)
				add(r, eMagnetic, fsim*dt)
				lw, lw0 := 0.0, 0.0
				if l != nil {
					lw, lw0 = l.W, l.W0
				}
				add(r, eGravity, lw*dt)
				add(r, eElastic, lw0*dt)
				cv, _ := r.co.Get(o.id)
				if cv != cv {
					cv = 0
				}
				add(r, eNuclear, cv*dt)
				add(r, eElectric, r.E*s*dt)
				add(r, eOscillation, s*jsMin(r.Amp, o.Amp)*dt)
				add(r, eIonization, fsim/(fsim+fsAll+1e-9)*dt)
			}
		}
	}
	return net.verdictFrom(senses, &sums)
}

// energySum is what one neuron took in from a context, per energy.
type energySum struct{ v [nEnergies]float64 }

// verdictFrom is verdictFrom(senses, sums, keys): the 16-percentage verdict (read and readLine share it).
func (net *Network) verdictFrom(senses []*Neuron, sums *OMap[int, *energySum]) *ReadResult {
	var alive []*Neuron
	for _, x := range senses {
		if net.alive(x) {
			alive = append(alive, x)
		}
	}
	if len(alive) == 0 {
		return nil
	}
	profile := make([][]ProfileEntry, len(alive))
	matched := make([]int, len(alive))
	carried := 0
	for k := 0; k < nEnergies; k++ {
		if net.e[k] <= 0 {
			continue
		}
		total, reached := 0.0, 0
		for _, t := range sums.All() {
			if t.v[k] > 0 {
				total += t.v[k]
				reached++
			}
		}
		if total <= 1e-12 {
			continue
		}
		carried++
		fair := 100 / float64(reached)
		for i, x := range alive {
			v := 0.0
			if t, ok := sums.Get(x.id); ok {
				v = t.v[k]
			}
			pct := 100 * v / total
			profile[i] = append(profile[i], ProfileEntry{EnergyKeys[k], pct})
			if pct > fair {
				matched[i]++
			}
		}
	}
	ranked := make([]Ranked, len(alive))
	for i, x := range alive {
		avg := 0.0
		if len(profile[i]) > 0 {
			s := 0.0
			for _, p := range profile[i] {
				s += p.Pct
			}
			avg = s / float64(len(profile[i]))
		}
		ranked[i] = Ranked{S: x, Matched: matched[i], Pct: avg, Profile: profile[i]}
	}
	sort.SliceStable(ranked, func(i, j int) bool {
		a, b := ranked[i], ranked[j]
		c := float64(b.Matched - a.Matched)
		if c == 0 {
			c = b.Pct - a.Pct
		}
		return c < 0
	})
	best := ranked[0]
	bar := net.D.Strict
	cf := float64(carried)
	verdict := "not relevant"
	if carried != 0 {
		if float64(best.Matched) > cf*bar {
			verdict = "accurate"
		} else if float64(best.Matched) >= cf*bar/2 {
			verdict = "partly"
		}
	}
	clr := len(ranked) < 2 || best.Matched > ranked[1].Matched
	res := &ReadResult{Winner: best.S, Ranked: ranked, Carried: carried, Verdict: verdict, Clear: clr, Meaning: ostr{kind: 1}}
	if verdict == "accurate" && clr {
		res.Meaning = net.meaningOf(best.S)
	}
	return res
}

func (net *Network) meaningOf(n *Neuron) ostr {
	if n.group.truthy() {
		return n.group
	}
	var count NumObj
	for _, l := range n.adj {
		o := net.Get(l.other(n.id))
		if o == nil {
			continue
		}
		g := o.group
		if !g.truthy() {
			g = o.truth
		}
		if g.truthy() && o.Word != n.Word && !l.HasTent {
			v, _ := count.Get(g.s)
			count.Set(g.s, v+l.W)
		}
	}
	if b := bestEntry(&count); b >= 0 {
		return ostr{kind: 2, s: count.keys[b]}
	}
	near := net.Neighbours(n)
	if len(near) > 2 {
		near = near[:2]
	}
	if len(near) > 0 {
		return ostr{kind: 2, s: "near " + strings.Join(near, ", ")}
	}
	return ostr{kind: 1}
}

func (net *Network) Neighbours(n *Neuron) []string {
	type ow struct {
		o *Neuron
		w float64
	}
	var list []ow
	for _, l := range n.adj {
		o := net.Get(l.other(n.id))
		if o != nil && o.Word != n.Word {
			list = append(list, ow{o, l.W})
		}
	}
	sort.SliceStable(list, func(i, j int) bool { return list[j].w-list[i].w < 0 })
	out := make([]string, len(list))
	for i, x := range list {
		out[i] = x.o.Word
	}
	return out
}

// Stats is stats() (for the page; the CLI does not use it).
type Stats struct {
	Order, Heat                    float64
	Hot, Cold, Neurons, Links, Lit int
}

func (net *Network) Stats() Stats {
	re, im := 0.0, 0.0
	for _, n := range net.N {
		re += Cos(n.Theta)
		im += Sin(n.Theta)
	}
	st := Stats{Order: Hypot(re, im) / math.Max(1, float64(len(net.N))), Neurons: len(net.N), Links: net.links.Len()}
	for _, n := range net.N {
		st.Heat += n.T
		if n.T > cTHOT {
			st.Hot++
		}
		if n.T < cTCOLD {
			st.Cold++
		}
		if n.Lit > 0.3 {
			st.Lit++
		}
	}
	return st
}

// Accessors used by the command line.
func (net *Network) Links() []*Link                 { return net.linkList() }
func (net *Network) LinksOf(n *Neuron) []*Link      { return n.adj }
func (net *Network) Banned() int                    { return net.banned.Len() }
func (net *Network) Phrases() int                   { return net.phrases.Len() }
func (net *Network) Proposed() float64              { return net.proposed }
func (net *Network) Confirmed() float64             { return net.confirmed }
func (net *Network) Rejected() float64              { return net.rejected }
func (net *Network) PredAlike(a, b *Neuron) float64 { return net.alike(a, b, net.predIndex()) }

// strictEqual is `a === b` for the simple JSON kinds (undefined, null, booleans, numbers, strings).
func strictEqual(a, b *JV) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	if a.Kind != b.Kind {
		return false
	}
	switch a.Kind {
	case JNull:
		return true
	case JBool:
		return a.B == b.B
	case JNum:
		return a.N == b.N
	case JStr:
		return a.S == b.S
	}
	return a == b
}
