package engine

// save() and load(): the same network.json format as sim.js, byte for byte (JSON.stringify(net.save())).

import "strings"

func (net *Network) label(id int) (string, bool) {
	if n := net.Get(id); n != nil {
		return n.Label, true
	}
	return "", false
}

func (w *jw) ostrVal(o ostr) {
	if o.kind == 2 {
		w.str(o.s)
	} else {
		w.raw("null")
	}
}

func (net *Network) writeMap(w *jw, m *OMap[int, float64]) {
	var o NumObj
	for i := range m.e {
		en := &m.e[i]
		if en.dead {
			continue
		}
		if lab, ok := net.label(en.k); ok {
			o.Set(lab, toFixedNum(en.v, 4))
		}
	}
	w.raw("{")
	for i, p := range o.Order() {
		if i > 0 {
			w.raw(",")
		}
		w.key(o.keys[p])
		w.num(o.vals[p])
	}
	w.raw("}")
}

func writeNumObj(w *jw, o *NumObj) {
	w.raw("{")
	for i, p := range o.Order() {
		if i > 0 {
			w.raw(",")
		}
		w.key(o.keys[p])
		w.num(o.vals[p])
	}
	w.raw("}")
}

func writeStrList(w *jw, s []string) {
	w.raw("[")
	for i, x := range s {
		if i > 0 {
			w.raw(",")
		}
		w.str(x)
	}
	w.raw("]")
}

// Save is JSON.stringify(net.save()).
func (net *Network) Save() []byte {
	w := &jw{b: make([]byte, 0, 1<<20)}
	w.raw(`{"version":1,"t":`)
	w.num(net.T)
	w.raw(`,"steps":`)
	w.num(net.Steps)
	w.raw(`,"senseCount":`)
	writeNumObj(w, &net.senseCount)
	w.raw(`,"intuition":`)
	w.boolean(net.intuition)
	w.raw(`,"neurons":[`)
	for i, n := range net.N {
		if i > 0 {
			w.raw(",")
		}
		w.raw(`{"label":`)
		w.str(n.Label)
		w.raw(`,"word":`)
		w.str(n.Word)
		if n.group.kind != 0 {
			w.raw(`,"group":`)
			w.ostrVal(n.group)
		}
		if n.truth.kind != 0 {
			w.raw(`,"truth":`)
			w.ostrVal(n.truth)
		}
		w.raw(`,"omega":`)
		w.num(toFixedNum(n.Omega, 4))
		w.raw(`,"omega0":`)
		w.num(toFixedNum(n.Omega0, 4))
		w.raw(`,"T":`)
		w.num(toFixedNum(n.T, 3))
		w.raw(`,"amp":`)
		w.num(toFixedNum(n.Amp, 3))
		w.raw(`,"seen":`)
		w.num(n.Seen)
		w.raw(`,"role":`)
		if n.Role != "" {
			w.str(n.Role)
		} else {
			w.raw("null")
		}
		w.raw(`,"x":`)
		w.num(toFixedNum(n.X, 3))
		w.raw(`,"y":`)
		w.num(toFixedNum(n.Y, 3))
		w.raw(`,"next":`)
		net.writeMap(w, &n.next)
		w.raw(`,"co":`)
		net.writeMap(w, &n.co)
		w.raw(`,"out":`)
		net.writeMap(w, &n.out)
		if n.Fixed {
			w.raw(`,"fixed":true`)
		}
		w.raw("}")
	}
	w.raw(`],"links":[`)
	first := true
	for i := range net.links.e {
		en := &net.links.e[i]
		if en.dead {
			continue
		}
		l := en.v
		if !first {
			w.raw(",")
		}
		first = false
		w.raw("[")
		if s, ok := net.label(l.A); ok {
			w.str(s)
		} else {
			w.raw("null")
		}
		w.raw(",")
		if s, ok := net.label(l.B); ok {
			w.str(s)
		} else {
			w.raw("null")
		}
		w.raw(",")
		w.num(toFixedNum(l.W, 4))
		w.raw(",")
		w.num(toFixedNum(l.W0, 4))
		if l.HasTent {
			w.raw(",0")
		} else {
			w.raw(",null")
		}
		if l.Ne {
			w.raw(",1,")
		} else {
			w.raw(",0,")
		}
		if l.Proved.Truthy() {
			l.Proved.write(w)
		} else {
			w.raw("0")
		}
		w.raw("]")
	}
	w.raw(`],"banned":`)
	writeStrList(w, net.banned.list)
	w.raw(`,"checked":`)
	writeStrList(w, net.checked.list)
	w.raw(`,"phrases":`)
	writeStrList(w, net.phrases.list)
	w.raw(`,"stats":{"proposed":`)
	w.num(net.proposed)
	w.raw(`,"confirmed":`)
	w.num(net.confirmed)
	w.raw(`,"rejected":`)
	w.num(net.rejected)
	w.raw(`,"ni":`)
	w.num(toFixedNum(net.NiRate(), 3))
	w.raw(`,"bits":`)
	w.num(toFixedNum(net.BitRate(), 3))
	w.raw("}}")
	return w.b
}

// Load is load(state).
func (net *Network) Load(state *JV) *Network {
	for _, n := range net.N {
		n.adj = nil
	}
	net.N = nil
	net.links = OMap[uint64, *Link]{}
	net.nextID = 0
	net.byID = nil
	net.wordIdx = map[string][]*Neuron{}
	neurons := state.Get("neurons")
	var list []*JV
	if neurons != nil && neurons.Kind == JArr {
		list = neurons.A
	}
	for _, v := range list {
		x := net.addJV(v)
		x.Omega0 = v.Get("omega0").Num()
		x.T = v.Get("T").Num()
		x.Amp = v.Get("amp").Num()
		x.group = ostrFromJV(v.Get("group"))
		x.truth = ostrFromJV(v.Get("truth"))
		x.Seen = 0
		if s := v.Get("seen"); s.Truthy() {
			x.Seen = s.Num()
		}
		x.Role = ""
		if r := v.Get("role"); r.Truthy() {
			x.Role = r.String()
		}
	}
	byLabel := make(map[string]*Neuron, len(net.N))
	for _, n := range net.N {
		byLabel[n.Label] = n
	}
	for i, v := range list {
		x := net.N[i]
		for _, f := range [3]struct {
			key string
			m   *OMap[int, float64]
		}{{"next", &x.next}, {"co", &x.co}, {"out", &x.out}} {
			o := v.Get(f.key)
			if o == nil || o.Kind != JObj {
				continue
			}
			for j, k := range o.Keys {
				if n, ok := byLabel[k]; ok {
					f.m.Set(n.id, o.Vals[j].Num())
				}
			}
		}
		x.Fixed = v.Get("fixed").Truthy()
	}
	if links := state.Get("links"); links != nil && links.Kind == JArr {
		for _, row := range links.A {
			at := func(i int) *JV {
				if row != nil && row.Kind == JArr && i < len(row.A) {
					return row.A[i]
				}
				return nil
			}
			A, B := byLabel[at(0).String()], byLabel[at(1).String()]
			if A == nil || B == nil || at(0) == nil || at(1) == nil {
				continue
			}
			w0 := 0.5
			if v := at(3); v != nil {
				w0 = v.Num()
			}
			net.connect(A, B, w0)
			l := net.link(A, B)
			if l == nil {
				continue
			}
			l.W = at(2).Num()
			if !at(4).IsNullish() {
				l.Tent, l.HasTent = net.T, true
			}
			if at(5).Truthy() {
				l.Ne = true
			}
			if p := at(6); p.Truthy() {
				if p.Kind == JNum && p.N == 1 {
					l.Proved = &JV{Kind: JStr, S: "role"}
				} else {
					l.Proved = p
				}
			}
		}
	}
	net.banned, net.checked, net.phrases = StrSet{}, StrSet{}, StrSet{}
	for _, f := range [3]struct {
		key string
		s   *StrSet
	}{{"banned", &net.banned}, {"checked", &net.checked}, {"phrases", &net.phrases}} {
		if a := state.Get(f.key); a != nil && a.Kind == JArr {
			for _, x := range a.A {
				f.s.Add(x.String())
			}
		}
	}
	for _, n := range net.N {
		if net.phrases.Has(n.Word) {
			n.phrase = true
			n.parts = strings.Split(n.Word, " ")
		}
	}
	net.senseCount = NumObj{}
	if sc := state.Get("senseCount"); sc != nil && sc.Kind == JObj {
		for i, k := range sc.Keys {
			net.senseCount.Set(k, sc.Vals[i].Num())
		}
	}
	net.T = 0
	if t := state.Get("t"); t.Truthy() {
		net.T = t.Num()
	}
	net.Steps = 0
	if s := state.Get("steps"); s.Truthy() {
		net.Steps = s.Num()
	}
	if st := state.Get("stats"); st.Truthy() {
		net.proposed = st.Get("proposed").Num()
		net.confirmed = st.Get("confirmed").Num()
		net.rejected = st.Get("rejected").Num()
	}
	net.predCache = nil
	return net
}
