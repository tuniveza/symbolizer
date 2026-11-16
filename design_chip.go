package main

// Chiptune background-music generator.
//
// chipPiece(seed) renders one loopable piece of 8-bit console style music as
// raw PCM: 48000 Hz, signed 16-bit little-endian, stereo interleaved (L,R).
// Channels: two pulse waves (lead + harmony/arpeggio), a triangle bass and a
// noise channel for drums. Everything is rendered into a circular buffer so
// note tails and echoes that run past the end wrap round to the start, and
// the filters are run twice round the loop, so the end joins the start with
// no click. Only the standard library is used; the same seed always gives
// the same bytes.

import (
	"encoding/binary"
	"math"
	"math/rand/v2"
	"strconv"
)

const (
	chipRate     = 48000
	chipPeak     = 0.47   // final peak, fraction of full scale (about -6.6 dBFS)
	chipLowPass  = 7000.0 // Hz, softens the square edges
	chipDCCut    = 20.0   // Hz, removes pulse-wave DC drift
	chipArpFrame = 800    // samples per arpeggio step (one 60 Hz console frame)
)

// Harmony channel styles.
const (
	chipHarmArp    = iota // fast frame-rate chord arpeggios
	chipHarmBroken        // 16th-note broken chords
	chipHarmThirds        // follows the lead a third below
	chipHarmEcho          // delayed, quieter copy of the lead
)

func chipHarmName(s int) string {
	switch s {
	case chipHarmArp:
		return "fast arps"
	case chipHarmBroken:
		return "broken chords"
	case chipHarmThirds:
		return "harmony thirds"
	default:
		return "echo lead"
	}
}

func chipKeyName(pc int) string {
	return [12]string{"C", "C#", "D", "Eb", "E", "F", "F#", "G", "Ab", "A", "Bb", "B"}[pc%12]
}

// chipPlan holds every choice that depends on the seed.
type chipPlan struct {
	rng       *rand.Rand
	root      int // tonic pitch class 0..11
	minor     bool
	majorV    bool // minor key: play chord v as major V
	bpm       int
	sps       int // samples per 16th note
	phrases   int // 4-bar phrases in the piece
	prog      [4]int
	harmA     int // harmony style, first half
	harmB     int // harmony style, second half
	leadDuty  float64
	harmDuty  float64
	sweep     bool // duty sweeps on long lead notes
	bassPat   int
	drumPat   int
	noiseKick bool
	arpRhythm int
	arpFrames int
	fill      int
	echoDelay int
}

func chipNewPlan(seed uint64) *chipPlan {
	r := rand.New(rand.NewPCG(seed, 0x9e3779b97f4a7c15^seed<<1))
	p := &chipPlan{rng: r}
	p.root = r.IntN(12)
	p.minor = r.IntN(2) == 0
	p.majorV = p.minor && r.IntN(2) == 0
	p.bpm = 110 + r.IntN(41)
	p.sps = int(math.Round(chipRate * 60.0 / float64(p.bpm) / 4))
	// Pick 4 or 6 phrases: whichever keeps the length in 30..50 s, nearest 40 s.
	best, bestErr := 4, math.MaxFloat64
	for _, n := range []int{4, 6, 8} {
		sec := float64(n*64*p.sps) / chipRate
		if sec < 30 || sec > 50 {
			continue
		}
		if e := math.Abs(sec - 40); e < bestErr {
			best, bestErr = n, e
		}
	}
	p.phrases = best
	majorProgs := [][4]int{{0, 4, 5, 3}, {0, 5, 3, 4}, {0, 3, 4, 3}, {5, 3, 0, 4}, {0, 3, 5, 4}, {0, 2, 3, 4}}
	minorProgs := [][4]int{{0, 5, 2, 6}, {0, 3, 4, 0}, {0, 6, 5, 6}, {0, 5, 3, 4}, {0, 3, 6, 2}, {5, 6, 0, 0}}
	if p.minor {
		p.prog = minorProgs[r.IntN(len(minorProgs))]
	} else {
		p.prog = majorProgs[r.IntN(len(majorProgs))]
	}
	pairs := [][2]int{
		{chipHarmArp, chipHarmThirds}, {chipHarmArp, chipHarmEcho}, {chipHarmBroken, chipHarmArp},
		{chipHarmThirds, chipHarmArp}, {chipHarmEcho, chipHarmArp}, {chipHarmArp, chipHarmBroken},
	}
	pr := pairs[r.IntN(len(pairs))]
	p.harmA, p.harmB = pr[0], pr[1]
	p.leadDuty = []float64{0.25, 0.5, 0.25}[r.IntN(3)]
	p.harmDuty = []float64{0.125, 0.25}[r.IntN(2)]
	p.sweep = r.IntN(3) > 0
	p.bassPat = r.IntN(4)
	p.drumPat = r.IntN(4)
	p.noiseKick = r.IntN(3) == 0
	p.arpRhythm = r.IntN(3)
	p.arpFrames = []int{1, 1, 2}[r.IntN(3)]
	p.fill = r.IntN(3)
	p.echoDelay = 2 + r.IntN(2)
	return p
}

// chipStyleName gives a short human description of the piece for a seed,
// for example "C minor, 132 bpm, fast arps + echo lead".
func chipStyleName(seed uint64) string {
	p := chipNewPlan(seed)
	mode := "major"
	if p.minor {
		mode = "minor"
	}
	return chipKeyName(p.root) + " " + mode + ", " + strconv.Itoa(p.bpm) + " bpm, " +
		chipHarmName(p.harmA) + " + " + chipHarmName(p.harmB)
}

func (p *chipPlan) scale() [7]int {
	if p.minor {
		return [7]int{0, 2, 3, 5, 7, 8, 10}
	}
	return [7]int{0, 2, 4, 5, 7, 9, 11}
}

func chipFloorDiv(a, b int) int {
	q := a / b
	if a%b != 0 && (a < 0) != (b < 0) {
		q--
	}
	return q
}

// semis converts a scale degree (0 = tonic, may be negative or above 6) to
// semitones above the tonic.
func (p *chipPlan) semis(deg int) int {
	sc := p.scale()
	o := chipFloorDiv(deg, 7)
	return sc[deg-7*o] + 12*o
}

// chordSemis returns root, third, fifth of the diatonic triad on degree c,
// in semitones above the tonic.
func (p *chipPlan) chordSemis(c int) [3]int {
	t := [3]int{p.semis(c), p.semis(c + 2), p.semis(c + 4)}
	if p.majorV && c == 4 {
		t[1]++
	}
	return t
}

func chipIsChordTone(deg, c int) bool {
	m := ((deg-c)%7 + 7) % 7
	return m == 0 || m == 2 || m == 4
}

// chipVoice is one pulse or triangle note.
type chipVoice struct {
	start, gate  int // samples
	midi         float64
	arp          []float64 // semitone offsets cycled every arpStep samples
	arpStep      int
	tri          bool
	duty0, duty1 float64
	vol          float64
	attack       int
	decay        int
	sustain      float64
	release      int
	vib          bool
	panL, panR   float64
}

func chipFreq(midi float64) float64 { return 440 * math.Pow(2, (midi-69)/12) }

func chipBlep(t, dt float64) float64 {
	if t < dt {
		t /= dt
		return t + t - t*t - 1
	}
	if t > 1-dt {
		t = (t - 1) / dt
		return t*t + t + t + 1
	}
	return 0
}

func chipRender(buf []float32, n int, v *chipVoice) {
	total := v.gate + v.release
	if total <= 0 {
		return
	}
	j := ((v.start % n) + n) % n
	phase := 0.0
	base := chipFreq(v.midi)
	f := base
	arpIdx := -1
	vibDelay := chipRate / 6
	for i := 0; i < total; i++ {
		var e float64
		switch {
		case i < v.attack:
			e = float64(i) / float64(v.attack)
		case i < v.attack+v.decay:
			e = 1 - (1-v.sustain)*float64(i-v.attack)/float64(v.decay)
		default:
			e = v.sustain
		}
		if i >= v.gate {
			e *= 1 - float64(i-v.gate)/float64(v.release)
		}
		if v.arp != nil {
			if k := (i / v.arpStep) % len(v.arp); k != arpIdx {
				arpIdx = k
				f = chipFreq(v.midi + v.arp[k])
			}
		}
		ff := f
		if v.vib && i > vibDelay {
			depth := math.Min(1, float64(i-vibDelay)/float64(vibDelay)) * 0.22
			ff = f * (1 + 0.0578*depth*math.Sin(2*math.Pi*5.5*float64(i)/chipRate))
		}
		dt := ff / chipRate
		var y float64
		if v.tri {
			// 32-step triangle like the console's 4-bit sequencer.
			q := math.Floor(phase*32) / 32
			y = 4*math.Abs(q-0.5) - 1
		} else {
			d := v.duty0 + (v.duty1-v.duty0)*float64(i)/float64(total)
			y = -1.0
			if phase < d {
				y = 1
			}
			y += chipBlep(phase, dt)
			t2 := phase - d
			if t2 < 0 {
				t2++
			}
			y -= chipBlep(t2, dt)
		}
		phase += dt
		if phase >= 1 {
			phase--
		}
		s := float32(y * e * v.vol)
		buf[2*j] += s * float32(v.panL)
		buf[2*j+1] += s * float32(v.panR)
		j++
		if j == n {
			j = 0
		}
	}
}

// chipNoise is a 15-bit LFSR like the console noise channel.
type chipNoise struct {
	reg uint16
	acc float64
}

func (z *chipNoise) next(rate float64) float64 {
	z.acc += rate / chipRate
	for z.acc >= 1 {
		z.acc--
		bit := (z.reg ^ z.reg>>1) & 1
		z.reg = z.reg>>1 | bit<<14
	}
	if z.reg&1 == 0 {
		return 1
	}
	return -1
}

// chipDrum renders one drum hit (centre panned). kind: 0 kick, 1 noise kick,
// 2 snare, 3 closed hat, 4 open hat.
func chipDrum(buf []float32, n, start, kind int, vol float64) {
	var length int
	var tau, rate float64
	switch kind {
	case 0:
		length, tau = chipRate*15/100, 0.07
	case 1:
		length, tau, rate = chipRate*8/100, 0.025, 2200
	case 2:
		length, tau, rate = chipRate*18/100, 0.055, 13000
	case 3:
		length, tau, rate = chipRate*3/100, 0.008, 48000
	default:
		length, tau, rate = chipRate*12/100, 0.04, 48000
	}
	z := chipNoise{reg: 1}
	decay := math.Exp(-1 / (tau * chipRate))
	env := 1.0
	phase := 0.0
	bodyEnv := 1.0
	bodyDecay := math.Exp(-1 / (0.03 * chipRate))
	j := ((start % n) + n) % n
	for i := 0; i < length; i++ {
		atk := math.Min(1, float64(i)/48) // 1 ms attack, no click
		fade := 1.0
		if rem := length - i; rem < 240 {
			fade = float64(rem) / 240
		}
		var y float64
		switch kind {
		case 0:
			t := float64(i) / chipRate
			f := 45 + 115*math.Exp(-t/0.03)
			phase += f / chipRate
			if phase >= 1 {
				phase--
			}
			q := math.Floor(phase*32) / 32
			y = (4*math.Abs(q-0.5) - 1) * env
		case 2:
			f := 150 + 40*bodyEnv
			phase += f / chipRate
			if phase >= 1 {
				phase--
			}
			body := (4*math.Abs(phase-0.5) - 1) * bodyEnv * 0.6
			y = z.next(rate)*env + body
			bodyEnv *= bodyDecay
		default:
			y = z.next(rate) * env
		}
		env *= decay
		s := float32(y * vol * atk * fade)
		buf[2*j] += s * 0.5
		buf[2*j+1] += s * 0.5
		j++
		if j == n {
			j = 0
		}
	}
}

// chipNote is one melody note on the 16th grid of a 4-bar phrase.
type chipNote struct{ step, dur, deg int }

type chipMelody struct {
	p      *chipPlan
	lo, hi int // allowed lead degrees
	tonic  int // MIDI of degree 0
}

func (m *chipMelody) clamp(d int) int {
	for d < m.lo {
		d += 2
	}
	for d > m.hi {
		d -= 2
	}
	return d
}

func (m *chipMelody) nearestChordTone(prev, c int, avoidTonic bool) int {
	best, bestDist := prev, 99
	for off := 0; off <= 4; off++ {
		for _, s := range []int{1, -1} {
			d := prev + s*off
			if d < m.lo || d > m.hi || !chipIsChordTone(d, c) {
				continue
			}
			if avoidTonic && ((d%7)+7)%7 == 0 {
				continue
			}
			if off < bestDist {
				best, bestDist = d, off
			}
		}
	}
	return best
}

func (m *chipMelody) rhythmBar(final bool) []int {
	r := m.p.rng
	one := [][]int{{4}, {2, 2}, {2, 2}, {2, 1, 1}, {1, 1, 2}, {3, 1}, {3, 1}, {1, 1, 1, 1}, {-2, 2}, {2, -1, 1}}
	two := [][]int{{6, 2}, {8}, {3, 3, 2}}
	var out []int
	beats := 4
	if final {
		beats = 2
	}
	for b := 0; b < beats; {
		if beats-b >= 2 && r.IntN(4) == 0 {
			out = append(out, two[r.IntN(len(two))]...)
			b += 2
			continue
		}
		out = append(out, one[r.IntN(len(one))]...)
		b++
	}
	if final {
		if r.IntN(2) == 0 {
			out = append(out, 8)
		} else {
			out = append(out, 6, -2)
		}
	}
	return out
}

func (m *chipMelody) pitches(rhythm []int, barStep, c, prev int) []chipNote {
	r := m.p.rng
	center := (m.lo + m.hi) / 2
	var out []chipNote
	pos := 0
	for _, d := range rhythm {
		if d < 0 {
			pos -= d
			continue
		}
		var deg int
		if pos%4 == 0 || d >= 4 {
			deg = m.nearestChordTone(prev, c, false)
			if r.IntN(4) == 0 { // leap to the next chord tone towards the centre
				dir := 1
				if deg > center {
					dir = -1
				}
				deg = m.nearestChordTone(deg+2*dir, c, false)
			}
		} else {
			moves := []int{-1, 1, -1, 1, 0, 2, -2}
			mv := moves[r.IntN(len(moves))]
			if prev > center+3 && mv > 0 {
				mv = -mv
			}
			if prev < center-3 && mv < 0 {
				mv = -mv
			}
			deg = prev + mv
		}
		deg = m.clamp(deg)
		out = append(out, chipNote{barStep + pos, d, deg})
		prev = deg
		pos += d
	}
	return out
}

// phrases builds the main 4-bar phrase A and its answer A'.
func (m *chipMelody) phrases() (a, b []chipNote) {
	p := m.p
	r := p.rng
	center := (m.lo + m.hi) / 2
	r1, r2 := m.rhythmBar(false), m.rhythmBar(false)
	start := m.nearestChordTone(center+r.IntN(5)-2, p.prog[0], false)
	bar1 := m.pitches(r1, 0, p.prog[0], start)
	bar2 := m.pitches(r2, 16, p.prog[1], bar1[len(bar1)-1].deg)
	// Bar 3 repeats bar 1's rhythm and shape, moved to the third chord.
	shift := ((p.prog[2]-p.prog[0])%7 + 7) % 7
	if shift > 3 {
		shift -= 7
	}
	var bar3 []chipNote
	for _, nt := range bar1 {
		d := nt.deg + shift
		if (nt.step%4 == 0 || nt.dur >= 4) && !chipIsChordTone(d, p.prog[2]) {
			d = m.nearestChordTone(d, p.prog[2], false)
		}
		bar3 = append(bar3, chipNote{nt.step + 32, nt.dur, m.clamp(d)})
	}
	last := bar3[len(bar3)-1].deg
	endA := m.pitches(m.rhythmBar(true), 48, p.prog[3], last)
	endB := m.pitches(m.rhythmBar(true), 48, p.prog[3], last)
	// A ends open (not on the tonic); A' ends on the chord root, nearest the tonic.
	la := &endA[len(endA)-1]
	la.deg = m.nearestChordTone(la.deg, p.prog[3], true)
	lb := &endB[len(endB)-1]
	rootDeg := p.prog[3]
	for rootDeg+7 <= m.hi && math.Abs(float64(rootDeg+7-lb.deg)) < math.Abs(float64(rootDeg-lb.deg)) {
		rootDeg += 7
	}
	for rootDeg-7 >= m.lo && math.Abs(float64(rootDeg-7-lb.deg)) < math.Abs(float64(rootDeg-lb.deg)) {
		rootDeg -= 7
	}
	if rootDeg >= m.lo && rootDeg <= m.hi {
		lb.deg = rootDeg
	}
	a = append(append(append(a, bar1...), bar2...), bar3...)
	a = append(a, endA...)
	// A': same bars 1 and 3, bar 2 gets small changes on weak notes.
	b = append(b, bar1...)
	changed := 0
	for _, nt := range bar2 {
		if nt.step%4 != 0 && nt.dur < 4 && changed < 2 && r.IntN(2) == 0 {
			nt.deg = m.clamp(nt.deg + []int{-1, 1}[r.IntN(2)])
			changed++
		}
		b = append(b, nt)
	}
	b = append(b, bar3...)
	b = append(b, endB...)
	return a, b
}

// chipPiece renders one loopable chiptune piece for the seed as raw PCM:
// 48000 Hz, signed 16-bit little-endian, stereo interleaved. Length is
// 30..50 s, peak about -6.6 dBFS, and the end joins the start seamlessly.
func chipPiece(seed uint64) []byte {
	p := chipNewPlan(seed)
	r := p.rng
	sps := p.sps
	n := p.phrases * 64 * sps // frames
	buf := make([]float32, 2*n)

	tonic := 60 + p.root
	if tonic > 66 {
		tonic -= 12
	}
	m := &chipMelody{p: p, tonic: tonic, lo: 99, hi: -99}
	for d := -14; d <= 21; d++ {
		midi := tonic + p.semis(d)
		if midi >= 62 && midi <= 81 {
			m.lo = min(m.lo, d)
			m.hi = max(m.hi, d)
		}
	}
	melA, melB := m.phrases()

	// Sweep choice per long note, decided once so A repeats exactly.
	sweepA := make([]bool, len(melA))
	sweepB := make([]bool, len(melB))
	for i := range sweepA {
		sweepA[i] = p.sweep && r.IntN(2) == 0
	}
	for i := range sweepB {
		sweepB[i] = p.sweep && r.IntN(2) == 0
	}
	altDuty := 0.125
	if p.leadDuty == 0.125 {
		altDuty = 0.5
	}
	half := p.phrases / 2
	otherDuty := 0.5
	if p.leadDuty == 0.5 {
		otherDuty = 0.25
	}

	leadMidi := func(deg int) float64 {
		v := tonic + p.semis(deg)
		for v > 84 {
			v -= 12
		}
		for v < 60 {
			v += 12
		}
		return float64(v)
	}

	for ph := 0; ph < p.phrases; ph++ {
		base := ph * 64 * sps
		mel, sw := melA, sweepA
		if ph%2 == 1 {
			mel, sw = melB, sweepB
		}
		second := ph >= half
		duty := p.leadDuty
		if second {
			duty = otherDuty
		}
		harm := p.harmA
		if second {
			harm = p.harmB
		}

		// Lead.
		for i, nt := range mel {
			gate := nt.dur*sps - sps/4
			v := chipVoice{
				start: base + nt.step*sps, gate: gate, midi: leadMidi(nt.deg),
				duty0: duty, duty1: duty, vol: 0.22,
				attack: 96, decay: chipRate / 12, sustain: 0.7, release: chipRate / 50,
				panL: 0.62, panR: 0.38,
			}
			if nt.dur >= 6 {
				v.vib = true
				if sw[i] {
					v.duty0, v.duty1 = duty, altDuty
				}
			}
			chipRender(buf, n, &v)

			// Harmony that follows the lead.
			switch harm {
			case chipHarmThirds:
				h := v
				h.midi = leadMidi(nt.deg - 2)
				if h.midi > v.midi {
					h.midi -= 12
				}
				h.duty0, h.duty1 = 0.25, 0.25
				h.vol = 0.11
				h.vib = false
				h.panL, h.panR = 0.38, 0.62
				chipRender(buf, n, &h)
			case chipHarmEcho:
				h := v
				h.start += p.echoDelay * sps
				h.vol = 0.09
				h.duty0, h.duty1 = p.harmDuty, p.harmDuty
				h.panL, h.panR = 0.3, 0.7
				chipRender(buf, n, &h)
			}
		}

		for bar := 0; bar < 4; bar++ {
			c := p.prog[bar]
			cs := p.chordSemis(c)
			barStart := base + bar*16*sps
			lastBar := bar == 3

			// Harmony chords.
			hRoot := 57 + (p.root+cs[0])%12
			if hRoot > 64 {
				hRoot -= 12
			}
			hm := func(k int) float64 { // chord tone k (0..3+) as MIDI
				return float64(hRoot + cs[k%3] - cs[0] + 12*(k/3))
			}
			switch harm {
			case chipHarmArp:
				rhythms := [][]int{{2, 2, 2, 2, 2, 2, 2, 2}, {3, 3, 2, 3, 3, 2}, {4, 2, 2, 4, 2, 2}}
				pos := 0
				for _, d := range rhythms[p.arpRhythm] {
					arp := []float64{0, float64(cs[1] - cs[0]), float64(cs[2] - cs[0])}
					if p.arpFrames == 1 {
						arp = append(arp, 12)
					}
					d0, d1 := p.harmDuty, p.harmDuty
					if lastBar {
						d0, d1 = 0.125, 0.375
					}
					v := chipVoice{
						start: barStart + pos*sps, gate: d*sps - sps/3, midi: hm(0),
						arp: arp, arpStep: chipArpFrame * p.arpFrames,
						duty0: d0, duty1: d1, vol: 0.11,
						attack: 48, decay: chipRate / 10, sustain: 0.55, release: chipRate / 80,
						panL: 0.38, panR: 0.62,
					}
					chipRender(buf, n, &v)
					pos += d
				}
			case chipHarmBroken:
				pat := []int{0, 1, 2, 3, 2, 1, 2, 3, 0, 1, 2, 3, 4, 3, 2, 1}
				for s := 0; s < 16; s++ {
					v := chipVoice{
						start: barStart + s*sps, gate: sps * 3 / 4, midi: hm(pat[s]),
						duty0: p.harmDuty, duty1: p.harmDuty, vol: 0.1,
						attack: 48, decay: sps / 2, sustain: 0.5, release: chipRate / 100,
						panL: 0.38, panR: 0.62,
					}
					chipRender(buf, n, &v)
				}
			}

			// Triangle bass, chord tones by eighth note (-1 = rest).
			bassPats := [][8]int{
				{0, 3, 0, 3, 0, 3, 0, 3},
				{0, 0, 0, 0, 0, 0, 2, 3},
				{0, -1, 0, 2, 3, 2, 0, 1},
				{0, -1, 0, 3, -1, 0, 2, 3},
			}
			bRoot := 36 + (p.root+cs[0])%12
			if bRoot > 43 {
				bRoot -= 12
			}
			if bRoot < 33 {
				bRoot += 12
			}
			for e, k := range bassPats[p.bassPat] {
				if k < 0 {
					continue
				}
				midi := bRoot + cs[k%3] - cs[0] + 12*(k/3)
				v := chipVoice{
					start: barStart + e*2*sps, gate: 2 * sps * 85 / 100, midi: float64(midi), tri: true,
					vol: 0.34, attack: 48, decay: 1, sustain: 1, release: chipRate / 200,
					panL: 0.5, panR: 0.5,
				}
				chipRender(buf, n, &v)
			}

			// Drums.
			kicks := []string{"x.......x.......", "x.....x...x.....", "x.x.....x.......", "x.......x.x....."}
			snares := []string{"....x.......x...", "....x.......x...", "....x..x....x...", "....x.......x..x"}
			hats := []string{"x.x.x.x.x.x.x.x.", "..x...x...x...x.", "xxxxxxxxxxxxxxxx", "x.xxx.xxx.xxx.xx"}
			kp, sp, hp := kicks[p.drumPat], snares[p.drumPat], hats[p.drumPat]
			kickKind := 0
			if p.noiseKick {
				kickKind = 1
			}
			for s := 0; s < 16; s++ {
				at := barStart + s*sps
				if lastBar && s >= 12 {
					// Fill on the last beat of every phrase.
					fills := []string{"xxxx", "x.xx", "xx.x"}
					if fills[p.fill][s-12] == 'x' {
						chipDrum(buf, n, at, 2, 0.13+0.03*float64(s-12))
					}
					if s == 12 {
						chipDrum(buf, n, at, kickKind, 0.38)
					}
					continue
				}
				snare := sp[s] == 'x'
				if kp[s] == 'x' {
					chipDrum(buf, n, at, kickKind, 0.38)
				}
				if snare {
					chipDrum(buf, n, at, 2, 0.2)
				}
				hat := hp[s] == 'x' || (second && s%2 == 1 && p.drumPat != 1)
				if hat && !snare {
					vol := 0.045
					if s%4 == 0 {
						vol = 0.07
					} else if s%2 == 0 {
						vol = 0.058
					}
					kind := 3
					if p.drumPat == 1 && s == 14 && bar%2 == 1 {
						kind = 4
					}
					chipDrum(buf, n, at, kind, vol)
				}
			}
		}
	}

	chipFilter(buf, n)

	peak := float32(0)
	for _, s := range buf {
		if s < 0 {
			s = -s
		}
		peak = max(peak, s)
	}
	gain := float32(0)
	if peak > 0 {
		gain = float32(chipPeak) * 32767 / peak
	}
	out := make([]byte, 4*n)
	for i, s := range buf {
		v := math.Round(float64(s * gain))
		v = math.Max(-32768, math.Min(32767, v))
		binary.LittleEndian.PutUint16(out[2*i:], uint16(int16(v)))
	}
	return out
}

// chipFilter runs a gentle one-pole low-pass and a DC blocker over each
// channel. The first pass round the loop only warms the filter state, so the
// second (written) pass starts exactly where the end leaves off.
func chipFilter(buf []float32, n int) {
	a := float32(1 - math.Exp(-2*math.Pi*chipLowPass/chipRate))
	rc := float32(1 - 2*math.Pi*chipDCCut/chipRate)
	for ch := 0; ch < 2; ch++ {
		var lp, xPrev, hp float32
		for pass := 0; pass < 2; pass++ {
			for i := ch; i < 2*n; i += 2 {
				lp += a * (buf[i] - lp)
				hp = lp - xPrev + rc*hp
				xPrev = lp
				if pass == 1 {
					buf[i] = hp
				}
			}
		}
	}
}
