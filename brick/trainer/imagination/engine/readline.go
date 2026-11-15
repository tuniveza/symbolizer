package engine

// readLine(words, steps) (sim.js): every word of a line read in one run. The line is felt together, and each
// word's senses are judged only by what reached them from the other words, with the same 16 energies and the same
// verdict as read(). The per-neuron intake and each word's judging only read the network, so they run in parallel;
// every sum is still added in sim.js's order.

import (
	"math"
	"slices"
)

type LineAnswer struct {
	Word   string
	Result *ReadResult
}

var pairEnergies = [7]int{eRest, eMagnetic, eGravity, eElastic, eNuclear, eElectric, eOscillation}

type lineIntake struct {
	imp, heat, thermal, lit, work []float64 // what came from each word of the line
	allIn                         float64
	pair                          [7][]float64
	fsimO                         []float64
	base                          float64
	tImp, tHeat, tThermal         float64
	tLit, tWork                   float64
	tPair                         [7]float64
}

func fsOf(a, b *Neuron) float64 {
	q := (a.Omega - b.Omega) / cSIGMA
	return Exp(-(q * q))
}

func addSum(m *OMap[int, *energySum], id, k int, v float64) {
	if !(v > 0) {
		return
	}
	t, ok := m.Get(id)
	if !ok {
		t = &energySum{}
		m.Set(id, t)
	}
	t.v[k] += v
}

// ReadLine is readLine(words, steps).
func (net *Network) ReadLine(words []string, steps int) []LineAnswer {
	var ws []string
	for _, w := range uniqueWords(words) {
		if len(net.wordIdx[w]) > 0 {
			ws = append(ws, w)
		}
	}
	W := len(ws)
	wordOf := make(map[string]int, W)
	for i, w := range ws {
		wordOf[w] = i
	}
	dt := cDt
	sumsW := make([]OMap[int, *energySum], W)
	reach := 5.3 * cSIGMA
	near := 1.8 * cSIGMA
	for step := 0; step < steps; step++ {
		if step%8 == 0 {
			net.Stimulate(ws, 1.0)
		}
		net.Step()
		var cx []*Neuron
		var cxWord []int
		for _, n := range net.N {
			if i, ok := wordOf[n.Word]; ok {
				cx = append(cx, n)
				cxWord = append(cxWord, i)
			}
		}
		if W < 2 || len(cx) == 0 {
			continue
		}
		idWord := make(map[int]int, len(cx))
		for i, n := range cx {
			idWord[n.id] = cxWord[i]
		}
		var pool []*Neuron
		small := len(net.N) <= 120
		for _, r := range net.N {
			if _, ok := wordOf[r.Word]; ok {
				continue
			}
			in := small || r.stepIn.Len() > 0 || r.stepLit.Len() > 0
			if !in {
				for _, o := range cx {
					if math.Abs(o.Omega-r.Omega) < near || net.link(r, o) != nil {
						in = true
						break
					}
				}
			}
			if in {
				pool = append(pool, r)
			}
		}
		all := append(append([]*Neuron(nil), pool...), cx...)
		A, P := len(all), len(pool)
		sorted := make([]int, P)
		for j := range sorted {
			sorted[j] = j
		}
		slices.SortStableFunc(sorted, func(a, b int) int { return cmpDiff(pool[a].Omega - pool[b].Omega) })
		sortedOm := make([]float64, P)
		for q, j := range sorted {
			sortedOm[q] = pool[j].Omega
		}
		lower := func(x float64) int {
			lo, hi := 0, P
			for lo < hi {
				mid := (lo + hi) >> 1
				if sortedOm[mid] < x {
					lo = mid + 1
				} else {
					hi = mid
				}
			}
			return lo
		}
		per := make([]lineIntake, A)
		parallelFor(A, 32, func(lo, hi int) {
			for ri := lo; ri < hi; ri++ {
				r := all[ri]
				p := &per[ri]
				byWord := func(m *OMap[int, float64], scale float64) []float64 {
					v := make([]float64, W)
					for i := range m.e {
						en := &m.e[i]
						if en.dead {
							continue
						}
						if wi, ok := idWord[en.k]; ok {
							v[wi] += en.v * scale
						}
					}
					return v
				}
				for k := range p.pair {
					p.pair[k] = make([]float64, W)
				}
				p.fsimO = make([]float64, len(cx))
				for oi, o := range cx {
					if o == r {
						continue
					}
					_, fsim, s := syncOf(r, o)
					l := net.link(r, o)
					i := cxWord[oi]
					p.fsimO[oi] = fsim
					lw, lw0 := 0.0, 0.0
					if l != nil {
						lw, lw0 = l.W, l.W0
					}
					cv, _ := r.co.Get(o.id)
					if cv != cv {
						cv = 0
					}
					p.pair[0][i] += s * dt
					p.pair[1][i] += fsim * dt
					p.pair[2][i] += lw * dt
					p.pair[3][i] += lw0 * dt
					p.pair[4][i] += cv * dt
					p.pair[5][i] += r.E * s * dt
					p.pair[6][i] += s * jsMin(r.Amp, o.Amp) * dt
				}
				base := 0.0
				for q := lower(r.Omega - reach); q < P && sortedOm[q] <= r.Omega+reach; q++ {
					if j := sorted[q]; j != ri {
						base += fsOf(r, pool[j])
					}
				}
				p.base = base
				tot := func(v []float64) float64 {
					t := 0.0
					for i := 0; i < W; i++ {
						t += v[i]
					}
					return t
				}
				p.imp, p.heat, p.thermal, p.lit, p.work = byWord(&r.stepIn, 1), byWord(&r.stepHeat, 1), byWord(&r.stepThermal, 1), byWord(&r.stepLit, 1), byWord(&r.inbox, dt)
				allIn := 0.0
				for i := range r.stepIn.e {
					if en := &r.stepIn.e[i]; !en.dead {
						allIn += en.v
					}
				}
				p.allIn = allIn
				p.tImp, p.tHeat, p.tThermal, p.tLit, p.tWork = tot(p.imp), tot(p.heat), tot(p.thermal), tot(p.lit), tot(p.work)
				for k := range p.pair {
					p.tPair[k] = tot(p.pair[k])
				}
			}
		})
		but := func(t float64, v []float64, w int) float64 { return jsMax(0, t-v[w]) }
		// each word writes only its own sums, so the words are judged in parallel
		parallelFor(W, 1, func(lo, hi int) {
			for w := lo; w < hi; w++ {
				var ctxN, own []*Neuron
				for i, n := range cx {
					if cxWord[i] != w {
						ctxN = append(ctxN, n)
					} else {
						own = append(own, n)
					}
				}
				if len(ctxN) == 0 {
					continue
				}
				o0 := 0.0
				for _, n := range ctxN {
					o0 += n.Omega0
				}
				mean0 := o0 / float64(len(ctxN))
				m := &sumsW[w]
				for ri := 0; ri < A; ri++ {
					r := all[ri]
					if ri >= P && cxWord[ri-P] != w {
						continue
					}
					p := &per[ri]
					imp := but(p.tImp, p.imp, w)
					addSum(m, r.id, eKinetic, imp)
					addSum(m, r.id, eMechanical, imp*cABSORB)
					addSum(m, r.id, eHeat, but(p.tHeat, p.heat, w))
					addSum(m, r.id, eThermal, but(p.tThermal, p.thermal, w))
					addSum(m, r.id, eRadiant, but(p.tLit, p.lit, w))
					if p.allIn > 0 {
						addSum(m, r.id, ePotential, r.E*imp/p.allIn*dt)
					}
					addSum(m, r.id, eWork, but(p.tWork, p.work, w))
					q := (r.Omega0 - mean0) / cSIGMA
					addSum(m, r.id, eChemical, Exp(-(q*q))*dt)
					for ki, k := range pairEnergies {
						addSum(m, r.id, k, but(p.tPair[ki], p.pair[ki], w))
					}
					fsAll := p.base
					for _, s := range own {
						if s != r {
							fsAll += fsOf(r, s)
						}
					}
					ion := 0.0
					for oi, o := range cx {
						if cxWord[oi] != w && o != r {
							ion += p.fsimO[oi] / (p.fsimO[oi] + fsAll + 1e-9) * dt
						}
					}
					addSum(m, r.id, eIonization, ion)
				}
			}
		})
	}
	out := make([]LineAnswer, W)
	for i, w := range ws {
		out[i] = LineAnswer{Word: w, Result: net.verdictFrom(append([]*Neuron(nil), net.wordIdx[w]...), &sumsW[i])}
	}
	return out
}
