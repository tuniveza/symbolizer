package engine

// predIndex, alike and propose (sim.js), on flat arrays. Same sums in the same order as the JavaScript Maps:
// every predecessor list is in N order (ascending id), which is the order sim.js's predIndex inserts them in.

import (
	"math"
	"slices"
	"sort"
)

// predSnap is predIndex()'s cache: who comes before each neuron, and how strongly, as it was when it was built
// (sim.js keeps using a stale cache until intuit() drops it; so does this).
type predSnap struct {
	start []int32 // row id is keys/vals[start[id]:start[id+1]]
	keys  []int32
	vals  []float64
}

func (p *predSnap) row(id int) ([]int32, []float64) {
	if id < 0 || id+1 >= len(p.start) {
		return nil, nil
	}
	a, b := p.start[id], p.start[id+1]
	return p.keys[a:b], p.vals[a:b]
}

func (net *Network) predIndex() *predSnap {
	if net.predCache != nil {
		return net.predCache
	}
	n := net.nextID
	p := &predSnap{start: make([]int32, n+2)}
	for _, a := range net.N {
		for i := range a.next.e {
			if en := &a.next.e[i]; !en.dead {
				p.start[en.k+2]++
			}
		}
	}
	for i := 2; i < len(p.start); i++ {
		p.start[i] += p.start[i-1]
	}
	total := p.start[len(p.start)-1]
	p.keys = make([]int32, total)
	p.vals = make([]float64, total)
	// start[id+1] is the fill position of row id while filling; afterwards it is the end of row id
	for _, a := range net.N {
		for i := range a.next.e {
			if en := &a.next.e[i]; !en.dead {
				at := p.start[en.k+1]
				p.keys[at], p.vals[at] = int32(a.id), en.v
				p.start[en.k+1]++
			}
		}
	}
	p.start = p.start[:n+1]
	net.predCache = p
	return p
}

func (net *Network) rare(k int) float64 {
	if uint(k) < uint(len(net.rarity)) {
		return net.rarity[k] // already `|| 1`
	}
	return 1
}

// scratch marks for alike and propose (sequential use only)
type marks struct {
	stamp uint32
	mark  []uint32
	val   []float64
}

func (m *marks) next(n int) {
	if len(m.mark) < n {
		m.mark = make([]uint32, n+n/4+16)
		m.val = make([]float64, len(m.mark))
		m.stamp = 0
	}
	m.stamp++
	if m.stamp == 0 {
		clear(m.mark)
		m.stamp = 1
	}
}

// AlikeZeroOutsidePass mirrors sim.js as of brick-mode a743999: alike()'s norm cache (this._norm) starts
// undefined, is a Map inside propose() and decide(), and is set to null when each of them ends. Once it is null,
// `const memo = this._norm && this._norm.get(x.id)` is null, which `memo !== undefined` takes as a cached norm of
// null, so alike() returns 0 outside those passes: candidates() after any training sees every likeness as 0
// (a network that has not run propose/decide yet, like run.js's score task, is not affected). When sim.js is
// fixed (e.g. `if (memo != null) return memo`), set this to false and the Go engine follows.
var AlikeZeroOutsidePass = false // sim.js fixed on 15 Nov: alike() works out the size when no pass is running

// alike is alike(a, b, pred): cosine of how the two are used, each context neuron weighted by its rarity.
// Its three sums (dot, na, nb) are kept apart, each added up in sim.js's order.
func (net *Network) alike(a, b *Neuron, pred *predSnap) float64 {
	if !net.rarityOK {
		net.rarityOf()
	}
	if AlikeZeroOutsidePass && net.normEnded && !net.normPass {
		return 0
	}
	pak, pav := pred.row(a.id)
	pbk, pbv := pred.row(b.id)
	m := &net.alikeMarks
	m.next(net.nextID)
	dot, na, nb := 0.0, 0.0, 0.0
	for i := range b.next.e {
		en := &b.next.e[i]
		if en.dead {
			continue
		}
		q := en.v * net.rare(en.k)
		nb += q * q
		m.mark[en.k], m.val[en.k] = m.stamp, en.v
	}
	for i := range a.next.e {
		en := &a.next.e[i]
		if en.dead {
			continue
		}
		k := en.k
		q := en.v * net.rare(k)
		na += q * q
		if k != a.id && k != b.id && m.mark[k] == m.stamp {
			dot += q * m.val[k] * net.rare(k)
		}
	}
	j := 0
	for i, k32 := range pak {
		k := int(k32)
		q := pav[i] * net.rare(k)
		na += q * q
		if k != a.id && k != b.id {
			for j < len(pbk) && pbk[j] < k32 {
				j++
			}
			if j < len(pbk) && pbk[j] == k32 {
				dot += q * pbv[j] * net.rare(k)
			}
		}
	}
	for i, k32 := range pbk {
		q := pbv[i] * net.rare(int(k32))
		nb += q * q
	}
	if truthyNum(na) && truthyNum(nb) {
		return dot / math.Sqrt(na*nb)
	}
	return 0
}

// alikeX is alike(x, ·) for one x against many: x's own part (na, and where each of x's keys sits) is worked out
// once. with(y) gives exactly alike(x, y, pred): the matching terms of dot are added in x's order, as sim.js does.
type alikeX struct {
	net   *Network
	x     *Neuron
	pred  *predSnap
	na    float64
	pak   []int32
	pav   []float64
	pos   []int32 // position of key k in x.next (live order), -1 when absent; by id
	xv    []float64
	terms []dotTerm
}

type dotTerm struct {
	pos int32
	v   float64
}

func (net *Network) alikeFrom(x *Neuron, pred *predSnap) *alikeX {
	if !net.rarityOK {
		net.rarityOf()
	}
	ax := &net.alikeCache
	ax.net, ax.x, ax.pred = net, x, pred
	ax.pak, ax.pav = pred.row(x.id)
	n := net.nextID
	if len(ax.pos) < n {
		ax.pos = make([]int32, n+n/4+16)
		ax.xv = make([]float64, len(ax.pos))
		for i := range ax.pos {
			ax.pos[i] = -1
		}
	}
	na := 0.0
	p := int32(0)
	for i := range x.next.e {
		en := &x.next.e[i]
		if en.dead {
			continue
		}
		q := en.v * net.rare(en.k)
		na += q * q
		ax.pos[en.k], ax.xv[en.k] = p, en.v
		p++
	}
	for i, k := range ax.pak {
		q := ax.pav[i] * net.rare(int(k))
		na += q * q
	}
	ax.na = na
	return ax
}

// done clears the position marks (call before x.next changes or another alikeFrom).
func (ax *alikeX) done() {
	for i := range ax.x.next.e {
		if en := &ax.x.next.e[i]; !en.dead {
			ax.pos[en.k] = -1
		}
	}
}

func (ax *alikeX) with(y *Neuron, buf *[]dotTerm) float64 {
	net, x := ax.net, ax.x
	nb := 0.0
	terms := (*buf)[:0]
	for i := range y.next.e {
		en := &y.next.e[i]
		if en.dead {
			continue
		}
		k := en.k
		q := en.v * net.rare(k)
		nb += q * q
		if k != x.id && k != y.id && k < len(ax.pos) && ax.pos[k] >= 0 {
			// sim.js: q = x's value * rare; dot += q * y's value * rare
			terms = append(terms, dotTerm{ax.pos[k], ax.xv[k] * net.rare(k) * en.v * net.rare(k)})
		}
	}
	if len(terms) > 1 {
		slices.SortFunc(terms, func(a, b dotTerm) int { return int(a.pos - b.pos) }) // positions are distinct
	}
	dot := 0.0
	for _, t := range terms {
		dot += t.v
	}
	*buf = terms
	pbk, pbv := ax.pred.row(y.id)
	// matches between pred rows, in x's (ascending) order: walk the shorter row, search the longer
	if len(pbk)*8 >= len(ax.pak) && len(ax.pak)*8 >= len(pbk) {
		// similar lengths: walk both (both ascending)
		j := 0
		for i, k32 := range ax.pak {
			k := int(k32)
			if k == x.id || k == y.id {
				continue
			}
			for j < len(pbk) && pbk[j] < k32 {
				j++
			}
			if j < len(pbk) && pbk[j] == k32 {
				q := ax.pav[i] * net.rare(k)
				dot += q * pbv[j] * net.rare(k)
			}
		}
	} else if len(pbk) <= len(ax.pak) {
		for j, k32 := range pbk {
			k := int(k32)
			if k == x.id || k == y.id {
				continue
			}
			if i, ok := slices.BinarySearch(ax.pak, k32); ok {
				q := ax.pav[i] * net.rare(k)
				dot += q * pbv[j] * net.rare(k)
			}
		}
	} else {
		for i, k32 := range ax.pak {
			k := int(k32)
			if k == x.id || k == y.id {
				continue
			}
			if j, ok := slices.BinarySearch(pbk, k32); ok {
				q := ax.pav[i] * net.rare(k)
				dot += q * pbv[j] * net.rare(k)
			}
		}
	}
	for i, k32 := range pbk {
		q := pbv[i] * net.rare(int(k32))
		nb += q * q
	}
	if truthyNum(ax.na) && truthyNum(nb) {
		return dot / math.Sqrt(ax.na*nb)
	}
	return 0
}

// propose is propose(x): Ne proposes lines to neurons used in the same places as x.
func (net *Network) propose(x *Neuron) {
	pred := net.predIndex()
	if x.Seen < 3 || special(x.Word) {
		return
	}
	net.normEnded = true // sim.js sets this._norm = null at the end of this pass
	nm := &net.nearMarks
	nm.next(net.nextID)
	near := net.nearList[:0]
	addNear := func(id int) {
		if id >= len(nm.mark) {
			nm.mark = append(nm.mark, make([]uint32, id+1-len(nm.mark))...)
			nm.val = append(nm.val, make([]float64, id+1-len(nm.val))...)
		}
		if nm.mark[id] != nm.stamp {
			nm.mark[id] = nm.stamp
			near = append(near, id)
		}
	}
	for i := range x.next.e {
		if en := &x.next.e[i]; !en.dead {
			ks, _ := pred.row(en.k)
			for _, p := range ks {
				addNear(int(p))
			}
		}
	}
	ps, _ := pred.row(x.id)
	for _, p := range ps {
		if q := net.Get(int(p)); q != nil {
			for i := range q.next.e {
				if en := &q.next.e[i]; !en.dead {
					addNear(en.k)
				}
			}
		}
	}
	net.nearList = near
	type cand struct {
		y   *Neuron
		sim float64
	}
	var cands []cand
	// each candidate's likeness only reads the network, so they are worked out in parallel, then taken in order
	ys := make([]*Neuron, 0, len(near))
	for _, id := range near {
		y := net.Get(id)
		if y == nil || y == x || y.Word == x.Word || y.Seen < 3 || special(y.Word) || net.link(x, y) != nil || net.banned.Has(pairKey(x.Word, y.Word)) {
			continue
		}
		ys = append(ys, y)
	}
	if len(ys) == 0 {
		return // (sim.js would not have called alike, so the rarity cache is left as it is)
	}
	ax := net.alikeFrom(x, pred) // builds the rarity cache if needed, as sim.js's first alike() call would
	sims := make([]float64, len(ys))
	parallelFor(len(ys), 48, func(lo, hi int) {
		var buf []dotTerm
		for i := lo; i < hi; i++ {
			sims[i] = ax.with(ys[i], &buf)
		}
	})
	for i, y := range ys {
		if sims[i] > 0.25 {
			cands = append(cands, cand{y, sims[i]})
		}
	}
	ax.done()
	sort.SliceStable(cands, func(i, j int) bool { return cands[j].sim-cands[i].sim < 0 })
	for i := 0; i < len(cands) && i < 2; i++ {
		y := cands[i].y
		net.connect(x, y, 0.15)
		l := net.link(x, y)
		l.Tent, l.HasTent = net.T, true
		net.proposed++
	}
}
