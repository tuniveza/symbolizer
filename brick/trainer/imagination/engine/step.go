package engine

// step(): one tick of the physics, as sim.js does it, arranged so it can use every core and still give the same
// bits. During the pair loop no neuron's frequency or phase changes (those move only afterwards) and no line is
// added or removed, so each pair's sync, its sine and its line are worked out first, in parallel. The pair loop
// itself (temperature, amplitude, light and inbox changes, which depend on order) then runs one pair after another
// in exactly sim.js's order, on compact arrays instead of the neurons themselves (same numbers, fewer cache misses).

import (
	"math"
	"runtime"
	"slices"
	"sync"
)

type stepBuf struct {
	pi, pj    []int32
	df, fs, s []float64
	sn        []float64
	lk        []*Link
	order     []int32
	orderVer  uint64
	orderN    int
	rank      []int32
	pos       []int32
	links     []*Link
	om, th    []float64 // frequency and phase of N[i] (fixed during the pair loop)
	tt, amp   []float64 // temperature and amplitude of N[i], changed by the pair loop
	lit, en   []float64
	litBy     []int
	rowStart  []int32 // lines by index: row i is rowOther/rowLink[rowStart[i]:rowStart[i+1]]
	rowOther  []int32
	rowLink   []*Link
	rowFill   []int32
	nWindow   int
}

// parallelMin is the number of pairs below which the parallel phase runs on one core (goroutines cost more).
var parallelMin = 6000

// Workers caps the goroutines of the parallel phase (0: one per core).
var Workers = 0

func growF(s []float64, n int) []float64 {
	if cap(s) < n {
		return make([]float64, n, n+n/4)
	}
	return s[:n]
}

func growI32(s []int32, n int) []int32 {
	if cap(s) < n {
		return make([]int32, n, n+n/4)
	}
	return s[:n]
}

func (net *Network) pairOrder(n int) []int32 {
	sb := &net.sb
	om := sb.om
	if sb.orderN == n && sb.orderVer == net.structVer && len(sb.order) == n {
		// frequencies move a little each step: an insertion sort from the last order is nearly linear.
		// (omega, index) is exactly the order a stable sort by omega gives.
		o := sb.order
		for i := 1; i < n; i++ {
			x := o[i]
			ox := om[x]
			j := i
			for j > 0 {
				y := o[j-1]
				oy := om[y]
				if ox-oy < 0 || (!(oy-ox < 0) && x < y) {
					o[j] = y
					j--
					continue
				}
				break
			}
			o[j] = x
		}
		return o
	}
	o := sb.order[:0]
	for k := 0; k < n; k++ {
		o = append(o, int32(k))
	}
	slices.SortStableFunc(o, func(p, q int32) int {
		d := om[p] - om[q]
		if d < 0 {
			return -1
		}
		if d > 0 {
			return 1
		}
		return 0
	})
	sb.order, sb.orderN, sb.orderVer = o, n, net.structVer
	return o
}

func (net *Network) buildPairs() {
	sb := &net.sb
	N := net.N
	n := len(N)
	om := sb.om
	sb.pi, sb.pj, sb.lk = sb.pi[:0], sb.pj[:0], sb.lk[:0]
	// each id's index in N, and the lines by index
	sb.pos = growI32(sb.pos, net.nextID)
	for k := range sb.pos {
		sb.pos[k] = -1
	}
	for k, x := range N {
		sb.pos[x.id] = int32(k)
	}
	sb.rowStart = growI32(sb.rowStart, n+1)
	clear(sb.rowStart)
	for _, l := range sb.links {
		sb.rowStart[sb.pos[l.A]+1]++
		sb.rowStart[sb.pos[l.B]+1]++
	}
	for i := 0; i < n; i++ {
		sb.rowStart[i+1] += sb.rowStart[i]
	}
	m := int(sb.rowStart[n])
	sb.rowOther = growI32(sb.rowOther, m)
	if cap(sb.rowLink) < m {
		sb.rowLink = make([]*Link, m, m+m/4)
	}
	sb.rowLink = sb.rowLink[:m]
	sb.rowFill = growI32(sb.rowFill, n)
	copy(sb.rowFill, sb.rowStart[:n])
	for _, l := range sb.links {
		p, q := sb.pos[l.A], sb.pos[l.B]
		sb.rowOther[sb.rowFill[p]], sb.rowLink[sb.rowFill[p]] = q, l
		sb.rowFill[p]++
		sb.rowOther[sb.rowFill[q]], sb.rowLink[sb.rowFill[q]] = p, l
		sb.rowFill[q]++
	}
	if n <= 120 {
		for i := 0; i < n; i++ {
			for j := i + 1; j < n; j++ {
				sb.pi = append(sb.pi, int32(i))
				sb.pj = append(sb.pj, int32(j))
			}
		}
		sb.nWindow = len(sb.pi)
		sb.lk = growLinks(sb.lk, sb.nWindow)
		return
	}
	order := net.pairOrder(n)
	reach := 1.8 * cSIGMA
	for u := 0; u < n; u++ {
		ou := om[order[u]]
		for v := u + 1; v < n && om[order[v]]-ou < reach; v++ {
			p, q := order[u], order[v]
			if p > q {
				p, q = q, p
			}
			sb.pi = append(sb.pi, p)
			sb.pj = append(sb.pj, q)
		}
	}
	sb.nWindow = len(sb.pi)
	sb.rank = growI32(sb.rank, n)
	rank := sb.rank
	for k, p := range order {
		rank[p] = int32(k)
	}
	sb.lk = growLinks(sb.lk, sb.nWindow)
	for _, l := range sb.links {
		p, q := sb.pos[l.A], sb.pos[l.B]
		if p > q {
			p, q = q, p
		}
		lo, hi := rank[p], rank[q]
		if lo > hi {
			lo, hi = hi, lo
		}
		if om[order[hi]]-om[order[lo]] < reach {
			continue // already a pair of the frequency window
		}
		sb.pi = append(sb.pi, p)
		sb.pj = append(sb.pj, q)
		sb.lk = append(sb.lk, l) // a linked pair's line is that line
	}
}

func growLinks(s []*Link, n int) []*Link {
	if cap(s) < n {
		ns := make([]*Link, n, n+n/4)
		return ns
	}
	return s[:n]
}

func (net *Network) appendLinks(out []*Link) []*Link {
	for i := range net.links.e {
		if !net.links.e[i].dead {
			out = append(out, net.links.e[i].v)
		}
	}
	return out
}

// lineByIndex is link(N[i], N[j]) from the per-step rows.
func (sb *stepBuf) lineByIndex(i, j int32) *Link {
	a0, a1 := sb.rowStart[i], sb.rowStart[i+1]
	b0, b1 := sb.rowStart[j], sb.rowStart[j+1]
	if b1-b0 < a1-a0 {
		a0, a1, j = b0, b1, i
	}
	for k := a0; k < a1; k++ {
		if sb.rowOther[k] == j {
			return sb.rowLink[k]
		}
	}
	return nil
}

// pairMath fills sync, sine and line for pairs [lo, hi) (lines only for frequency-window pairs).
func (net *Network) pairMath(lo, hi int) {
	sb := &net.sb
	om, th := sb.om, sb.th
	for k := lo; k < hi; k++ {
		i, j := sb.pi[k], sb.pj[k]
		df := om[i] - om[j]
		q := df / cSIGMA
		fsim := Exp(-(q * q))
		d := th[j] - th[i]
		sb.df[k], sb.fs[k] = df, fsim
		sb.s[k] = fsim * (1 + Cos(d)) / 2
		sb.sn[k] = Sin(d)
		if k < sb.nWindow {
			sb.lk[k] = sb.lineByIndex(i, j)
		}
	}
}

// Step is step(): one tick of the physics.
func (net *Network) Step() {
	N := net.N
	n := len(N)
	e := &net.e
	dt := cDt
	r := net.rand
	sb := &net.sb
	if cap(net.dTheta) < n {
		net.dTheta = make([]float64, n, n+n/4)
		net.inc = make([]float64, n, n+n/4)
		net.dOmega = make([]float64, n, n+n/4)
	}
	dTheta, inc, dOmega := net.dTheta[:n], net.inc[:n], net.dOmega[:n]
	clear(dTheta)
	clear(inc)
	clear(dOmega)
	sb.om, sb.th, sb.tt, sb.amp, sb.lit, sb.en = growF(sb.om, n), growF(sb.th, n), growF(sb.tt, n), growF(sb.amp, n), growF(sb.lit, n), growF(sb.en, n)
	if cap(sb.litBy) < n {
		sb.litBy = make([]int, n, n+n/4)
	}
	sb.litBy = sb.litBy[:n]
	om, th, T, Amp, Lit, En, litBy := sb.om, sb.th, sb.tt, sb.amp, sb.lit, sb.en, sb.litBy
	for i, x := range N {
		x.stepIn.Clear()
		x.stepHeat.Clear()
		x.stepThermal.Clear()
		x.stepLit.Clear()
		om[i], th[i], T[i], Amp[i], Lit[i], En[i], litBy[i] = x.Omega, x.Theta, x.T, x.Amp, x.Lit, x.E, x.litBy
	}

	// phase 1: the pairs and what stays fixed about them during the pair loop
	sb.links = net.appendLinks(sb.links[:0])
	net.buildPairs()
	np := len(sb.pi)
	sb.df, sb.fs, sb.s, sb.sn = growF(sb.df, np), growF(sb.fs, np), growF(sb.s, np), growF(sb.sn, np)
	workers := Workers
	if workers <= 0 {
		workers = runtime.GOMAXPROCS(0)
	}
	if np < parallelMin || workers < 2 {
		net.pairMath(0, np)
	} else {
		chunks := workers * 4
		size := (np + chunks - 1) / chunks
		var wg sync.WaitGroup
		for w := 0; w < workers; w++ {
			wg.Add(1)
			go func(w int) {
				defer wg.Done()
				for c := w; c < chunks; c += workers {
					lo := c * size
					if lo < np {
						net.pairMath(lo, min(np, lo+size))
					}
				}
			}(w)
		}
		wg.Wait()
	}

	// fan-out (sim.js): what a neuron sends out is shared among the neurons it reaches, past cFANOUT
	potential := e[ePotential]
	fieldMag := cFIELD * e[eMagnetic]
	kinetic := e[eKinetic]
	net.fan, net.share = growF(net.fan, n), growF(net.share, n)
	fan, share := net.fan, net.share
	clear(fan)
	for k := 0; k < np; k++ {
		w := 0.0
		if l := sb.lk[k]; l != nil {
			w = l.W
		}
		if c := w + fieldMag*sb.fs[k]; c > 0 {
			i, j := int(sb.pi[k]), int(sb.pj[k])
			fan[i] += c * sb.s[k]
			fan[j] += c * sb.s[k]
		}
	}
	for i := 0; i < n; i++ {
		share[i] = 1
		if fan[i] > cFANOUT {
			share[i] = cFANOUT / fan[i]
		}
	}

	// phase 2: the pair loop, in sim.js's order
	for k := 0; k < np; k++ {
		i, j := int(sb.pi[k]), int(sb.pj[k])
		df, fsim, s := sb.df[k], sb.fs[k], sb.s[k]
		l := sb.lk[k]
		w := 0.0
		if l != nil {
			w = l.W
		}
		c := w + fieldMag*fsim
		if c > 0 {
			pull := cK * c * fsim * sb.sn[k]
			dTheta[i] += pull
			dTheta[j] -= pull
			if En[i] > 0.01 && s > cSYNC-0.12*potential*jsMin(T[j], 1.5)/1.5 { // both tests are pure: order is free
				a, b := N[i], N[j]
				imp := cRELAY * kinetic * c * Amp[i] * En[i] * s * dt * share[i]
				inc[j] += imp
				v, _ := b.inbox.Get(a.id)
				b.inbox.Set(a.id, v+imp)
				b.stepIn.appendNew(a.id, imp)
				a.relayed += imp
				if l != nil {
					l.Carried += imp
				}
			}
			if En[j] > 0.01 && s > cSYNC-0.12*potential*jsMin(T[i], 1.5)/1.5 {
				a, b := N[i], N[j]
				imp := cRELAY * kinetic * c * Amp[j] * En[j] * s * dt * share[j]
				inc[i] += imp
				v, _ := a.inbox.Get(b.id)
				a.inbox.Set(b.id, v+imp)
				a.stepIn.appendNew(b.id, imp)
				b.relayed += imp
				if l != nil {
					l.Carried += imp
				}
			}
		}
		if l != nil && s > cSYNC {
			g := e[eGravity] * cGRAVITY * (om[j] - om[i]) * s
			dOmega[i] += g
			dOmega[j] -= g
		} else if l == nil && fsim > 0.12 && fsim < 0.5 {
			d := df
			if d == 0 || d != d {
				d = 1
			}
			p := e[eIonization] * cION * jsSign(d) * fsim
			dOmega[i] += p
			dOmega[j] -= p
		}
		if math.Abs(df) < cGAP {
			d := df
			if d == 0 || d != d {
				d = r.Next() - 0.5
			}
			p := e[eElectric] * cELECTRIC * jsSign(d) * (cGAP - math.Abs(df)) / cGAP
			dOmega[i] += p
			dOmega[j] -= p
		}
		if s > cSYNC {
			f := e[eThermal] * cTHERMAL * s * (T[i] - T[j]) * dt
			T[i] -= f
			T[j] += f
			if f > 0 {
				N[j].stepThermal.appendNew(N[i].id, f)
			} else if f < 0 {
				N[i].stepThermal.appendNew(N[j].id, -f)
			}
		}
		if l != nil {
			hot, cold := j, i
			if T[i] >= T[j] {
				hot, cold = i, j
			}
			if T[hot] > cTHOT {
				f := e[eHeat] * cHEAT * l.W * (T[hot] - T[cold]) * dt
				T[hot] -= f
				T[cold] += f
				N[cold].stepHeat.appendNew(N[hot].id, f)
			}
		}
		if T[i] > cTHOT {
			add := e[eRadiant] * cRADIANT * (T[i] - cTHOT) * fsim * dt
			if add > 0.002 && Lit[j] < 1 {
				Lit[j] += add
				litBy[j] = N[i].id
			}
			if add > 0 {
				N[j].stepLit.appendNew(N[i].id, add)
			}
		}
		if T[j] > cTHOT {
			add := e[eRadiant] * cRADIANT * (T[j] - cTHOT) * fsim * dt
			if add > 0.002 && Lit[i] < 1 {
				Lit[i] += add
				litBy[i] = N[j].id
			}
			if add > 0 {
				N[i].stepLit.appendNew(N[j].id, add)
			}
		}
		if l != nil && s > cSYNC {
			f := e[eOscillation] * cOSC * s * (Amp[i] - Amp[j]) * dt
			Amp[i] -= f
			Amp[j] += f
		}
	}
	for i, x := range N {
		x.T, x.Amp, x.Lit, x.litBy = T[i], Amp[i], Lit[i], litBy[i]
	}

	// each neuron, in order
	noise := cNOISE / (1 + 2*e[eRest])
	sqdt := math.Sqrt(dt)
	decay := 1 - cINBOXDECAY*dt
	litKeep := 1 - cLITDECAY*dt
	for i := 0; i < n; i++ {
		a := N[i]
		a.Theta += (a.Omega+dTheta[i])*dt + noise*sqdt*(r.Next()-0.5)
		a.E += inc[i] - a.E*cDECAY*dt
		a.T += e[eMechanical] * cABSORB * inc[i]
		a.T += (cTREST - a.T) * e[eRest] * cREST * dt
		if a.T > cTHOT {
			a.Amp -= 0.15 * (a.T - cTHOT) * dt
		}
		a.refined = 0
		if a.T < cTCOLD && a.inbox.Len() > 0 {
			sp := net.spread(a)
			if sp.ok && !sp.bimodal && sp.total > 0.05 {
				dOmega[i] += e[eWork] * cWORK * (sp.mean - a.Omega)
				a.Amp += e[eWork] * 0.2 * (1 - a.Amp) * dt
				a.refined = 1
			}
		}
		dOmega[i] += e[eChemical] * cCHEM * (a.Omega0 - a.Omega)
		a.Omega += dOmega[i] * dt
		if a.Fixed {
			a.Omega = a.Omega0
		}
		a.Amp = jsMin(1, jsMax(0.15, a.Amp))
		a.T = jsMax(0, a.T)
		a.Lit = jsMax(0, jsMin(1, a.Lit*litKeep))
		decayMap(&a.inbox, decay)
		decayMap(&a.co, decay)
	}
	elastic := net.e[eElastic]
	for _, l := range sb.links {
		l.W += elastic * cELASTIC * (l.W0 - l.W) * dt
		if l.Carried > 0.001 {
			l.Idle = 0
		} else {
			l.Idle = l.Idle + dt
		}
		l.Carried *= ninetyPctVar
	}
	clear(sb.lk) // let removed lines be collected
	clear(sb.rowLink)
	net.T += dt
	net.Steps++
	if math.Mod(net.Steps, cIMAGINE) == 0 {
		net.steer()
		net.imagine()
	}
}

// parallelFor runs f over [0, n) in chunks on every core (one call on this goroutine when n is small).
func parallelFor(n, minPer int, f func(lo, hi int)) {
	workers := Workers
	if workers <= 0 {
		workers = runtime.GOMAXPROCS(0)
	}
	if n < 2*minPer || workers < 2 {
		f(0, n)
		return
	}
	chunks := min(workers*4, (n+minPer-1)/minPer)
	size := (n + chunks - 1) / chunks
	var wg sync.WaitGroup
	for w := 0; w < min(workers, chunks); w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			for c := w; c < chunks; c += workers {
				if lo := c * size; lo < n {
					f(lo, min(n, lo+size))
				}
			}
		}(w)
	}
	wg.Wait()
}
