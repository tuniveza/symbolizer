package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"math"
	"math/cmplx"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// Tests never play real sound: the music is off, and the player is one
// that does not start, unless fakeSound puts a fake one in.
func init() {
	soundOn = false
	soundStart = func() (soundSink, error) { return nil, errNoPlayer }
}

// fakeSink is a player that only notes what it is given.
type fakeSink struct {
	mu      sync.Mutex
	got     bytes.Buffer
	stopped chan struct{}
}

func (f *fakeSink) Write(b []byte) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.got.Write(b)
}

func (f *fakeSink) Stop() { close(f.stopped) }

// fakeSound turns the music on with short pieces and a fake player; each
// player started is sent on the channel it gives, and each seed a piece
// was made for on the other.
func fakeSound(t *testing.T) (chan *fakeSink, chan uint64) {
	t.Helper()
	started, made := make(chan *fakeSink, 8), make(chan uint64, 16)
	oldOn, oldMuted, oldRoom, oldData, oldStart := soundOn, soundMuted, roomSound, soundData, soundStart
	t.Cleanup(func() {
		roomSound.quiet()
		roomSound.mu.Lock()
		next := roomSound.next
		roomSound.mu.Unlock()
		if next != nil {
			<-next.ready // the piece made ahead, done
		}
		soundOn, soundMuted, roomSound, soundData, soundStart = oldOn, oldMuted, oldRoom, oldData, oldStart
	})
	soundOn, soundMuted, roomSound = true, false, &roomMusic{}
	soundData = func(seed uint64) []byte {
		made <- seed
		return make([]byte, 4*sndRate/10)
	}
	soundStart = func() (soundSink, error) {
		f := &fakeSink{stopped: make(chan struct{})}
		started <- f
		return f, nil
	}
	return started, made
}

func waitStart(t *testing.T, started chan *fakeSink, what string) *fakeSink {
	t.Helper()
	select {
	case f := <-started:
		return f
	case <-time.After(3 * time.Second):
		t.Fatalf("%s: want the music started", what)
		return nil
	}
}

func waitStop(t *testing.T, f *fakeSink, what string) {
	t.Helper()
	select {
	case <-f.stopped:
	case <-time.After(3 * time.Second):
		t.Fatalf("%s: want the music stopped", what)
	}
}

func noStart(t *testing.T, started chan *fakeSink, what string) {
	t.Helper()
	select {
	case <-started:
		t.Fatalf("%s: want no music", what)
	case <-time.After(150 * time.Millisecond):
	}
}

func waitMade(t *testing.T, made chan uint64, what string) uint64 {
	t.Helper()
	select {
	case s := <-made:
		return s
	case <-time.After(3 * time.Second):
		t.Fatalf("%s: want a piece made", what)
		return 0
	}
}

// playing is the seed of the piece playing now.
func playing() uint64 {
	roomSound.mu.Lock()
	defer roomSound.mu.Unlock()
	return roomSound.last
}

// The music starts as the room goes 3D and stops as it goes flat, as
// design mode is left and as sy ends. Each time it starts it is another
// piece, unlike the one before, and the next is made while one plays.
func TestDesign3DSoundStartsStops(t *testing.T) {
	started, made := fakeSound(t)
	m := room3DModel(t, 120, 40)
	m.soundCmd()
	noStart(t, started, "the flat board")
	press(m, "ctrl+f")
	f := waitStart(t, started, "Ctrl+F: 3D")
	first := waitMade(t, made, "Ctrl+F: 3D")
	ahead := waitMade(t, made, "the next piece, made while the first plays")
	press(m, "esc")
	waitStop(t, f, "Esc: the flat board")
	if f.got.Len() == 0 {
		t.Fatal("the player was given no sound")
	}
	if playing() != first {
		t.Fatalf("playing %x, want the first piece %x", playing(), first)
	}
	press(m, "ctrl+f")
	f = waitStart(t, started, "3D again")
	if second := playing(); second != ahead || second == first || !sndUnlike(sndStyleOf(first), sndStyleOf(second)) {
		t.Fatalf("3D again: want the piece made ahead (%x), unlike the first (%x), got %x", ahead, first, second)
	}
	m.setMode(0)
	m.soundCmd()
	waitStop(t, f, "design mode left")
	before := playing()
	m.setMode(designMode)
	m.soundCmd()
	f = waitStart(t, started, "back in design mode, the room still 3D")
	if playing() == before {
		t.Fatal("back in 3D: want another piece")
	}
	roomSound.quiet() // sy ending
	waitStop(t, f, "sy ending")
}

// With no player, a piece is made once and kept, not made again each time.
func TestDesign3DSoundNoPlayer(t *testing.T) {
	_, made := fakeSound(t)
	soundStart = func() (soundSink, error) { return nil, errNoPlayer }
	m := room3DModel(t, 120, 40)
	for range 3 {
		press(m, "ctrl+f")
		time.Sleep(50 * time.Millisecond)
		press(m, "esc")
		roomSound.quiet()
	}
	waitMade(t, made, "the first 3D")
	select {
	case <-made:
		t.Fatal("no player: want only one piece made")
	case <-time.After(150 * time.Millisecond):
	}
}

// M in 3D turns the music off and on, and says so; off stays off for the
// session, even after leaving 3D and coming back.
func TestDesign3DSoundMute(t *testing.T) {
	started, _ := fakeSound(t)
	m := room3DModel(t, 120, 40)
	press(m, "ctrl+f")
	f := waitStart(t, started, "Ctrl+F: 3D")
	press(m, "m")
	waitStop(t, f, "M")
	if m.msg != "Music off." || !soundMuted {
		t.Fatalf("M: want \"Music off.\", got %q", m.msg)
	}
	press(m, "esc")
	press(m, "ctrl+f")
	noStart(t, started, "3D again with the music off")
	press(m, "m")
	waitStart(t, started, "M again")
	if m.msg != "Music on." || soundMuted {
		t.Fatalf("M again: want \"Music on.\", got %q", m.msg)
	}
}

// SY_3D_SOUND=0 turns the music off for good, and M says so.
func TestDesign3DSoundEnv(t *testing.T) {
	for v, want := range map[string]bool{"": true, "1": true, "0": false} {
		if got := soundAllowed(func(string) string { return v }); got != want {
			t.Fatalf("SY_3D_SOUND=%q: want %v, got %v", v, want, got)
		}
	}
	started, _ := fakeSound(t)
	soundOn = false
	m := room3DModel(t, 120, 40)
	press(m, "ctrl+f")
	press(m, "m")
	noStart(t, started, "SY_3D_SOUND=0")
	if m.msg != "Music is off: SY_3D_SOUND=0." || soundMuted {
		t.Fatalf("M with SY_3D_SOUND=0: got %q", m.msg)
	}
}

// The music fades in from silence and fades out to silence when stopped.
func TestSoundFeedFades(t *testing.T) {
	pcm := make([]byte, 4*sndRate)
	for i := 0; i < len(pcm); i += 2 {
		binary.LittleEndian.PutUint16(pcm[i:], uint16(10000))
	}
	stop := make(chan struct{})
	time.AfterFunc(80*time.Millisecond, func() { close(stop) })
	var out bytes.Buffer
	pos := feedSound(&out, pcm, 0, stop)
	b := out.Bytes()
	first := int16(binary.LittleEndian.Uint16(b))
	last := int16(binary.LittleEndian.Uint16(b[len(b)-4:]))
	if first > 20 || last != 0 || len(b) < 4*sndFadeOut {
		t.Fatalf("want a fade in from silence and out to it: first %d, last %d, %d bytes", first, last, len(b))
	}
	if pos%4 != 0 || pos != len(b)%len(pcm) {
		t.Fatalf("where it got to: %d, want %d", pos, len(b)%len(pcm))
	}
}

// Styles: every seed gives a relaxed tempo and a whole style; all the
// feels, progressions and instruments come up; the same seed always gives
// the same style; and a new seed is always unlike the last.
func TestSoundStyles(t *testing.T) {
	feels, progs, keys, leads, tonics := map[int]bool{}, map[int]bool{}, map[int]bool{}, map[int]bool{}, map[int]bool{}
	for seed := uint64(1); seed <= 400; seed++ {
		st := sndStyleOf(seed)
		if st != sndStyleOf(seed) {
			t.Fatalf("seed %d: two styles", seed)
		}
		if st.bpm < 80 || st.bpm > 100 {
			t.Fatalf("seed %d: %.0f bpm, want 80 to 100", seed, st.bpm)
		}
		if st.keys > sndPluckI || st.lead >= sndInstruments || st.prog >= len(sndProgs) || st.bass >= len(sndGrooves) {
			t.Fatalf("seed %d: a style out of range: %+v", seed, st)
		}
		feels[st.feel], progs[st.prog], keys[st.keys], leads[st.lead], tonics[st.key] = true, true, true, true, true
	}
	if len(feels) != sndFeels || len(progs) != len(sndProgs) || len(keys) != 4 || len(leads) != 4 || len(tonics) != 12 {
		t.Fatalf("want every feel, progression, instrument and key: %d %d %d %d %d", len(feels), len(progs), len(keys), len(leads), len(tonics))
	}
	last := uint64(0)
	for range 300 {
		next := sndNewSeed(last)
		if next == last || last != 0 && !sndUnlike(sndStyleOf(last), sndStyleOf(next)) {
			t.Fatalf("seed %x after %x: want a piece unlike the last", next, last)
		}
		last = next
	}
}

// Chords: each bar's notes sit in the middle of the keyboard, the bass's
// roots low down, and the chords move smoothly from one to the next.
func TestSoundChords(t *testing.T) {
	for seed := uint64(1); seed <= 60; seed++ {
		p := newSndPiece(seed)
		for b, c := range p.chords {
			if c.root < 28 || c.root > 39 || c.keys[0] < 50 || c.keys[len(c.keys)-1] > 76 {
				t.Fatalf("seed %d bar %d: notes out of range: root %d, keys %v", seed, b, c.root, c.keys)
			}
			if b > 0 {
				move := 0
				for i := range c.keys {
					move += max(c.keys[i]-p.chords[b-1].keys[i], p.chords[b-1].keys[i]-c.keys[i])
				}
				if move > 16 {
					t.Fatalf("seed %d bar %d: the chord jumps %d semitones", seed, b, move)
				}
			}
		}
		if got, want := p.frames, int(math.Round(sndBars*4*60*sndRate/p.bpm)); got != want {
			t.Fatalf("seed %d: %d samples, want %d", seed, got, want)
		}
	}
}

// The same seed makes the same piece, sample for sample.
func TestSoundSameSeedSamePiece(t *testing.T) {
	part := func(seed uint64) sndStereo {
		p := newSndPiece(seed)
		s := newSndStereo(p.frames)
		p.compPart(s)
		p.tunePart(s)
		p.typingPart(s)
		return s
	}
	a, b := part(7), part(7)
	if !slices.Equal(a.l, b.l) || !slices.Equal(a.r, b.r) {
		t.Fatal("seed 7 made two different pieces")
	}
}

// ---- whole pieces, looked at as sound ----

// sndChain is n seeds in a row as the room would play them: each unlike
// the one before, and always the same (for the tests).
func sndChain(n int) []uint64 {
	seeds := []uint64{1}
	for c := uint64(2); len(seeds) < n; c++ {
		if sndUnlike(sndStyleOf(seeds[len(seeds)-1]), sndStyleOf(c)) {
			seeds = append(seeds, c)
		}
	}
	return seeds
}

// sndTake is a rendered piece, as mono and stereo samples.
type sndTake struct {
	seed                uint64
	style               sndStyle
	mix, music, office  sndStereo
	musicMono, officeMo []float64
}

var (
	takesMu sync.Mutex
	takes   = map[uint64]*sndTake{}
)

// sndTakeOf renders seed's piece once for all the tests.
func sndTakeOf(seed uint64) *sndTake {
	takesMu.Lock()
	defer takesMu.Unlock()
	if tk := takes[seed]; tk != nil {
		return tk
	}
	mix, music, office := sndRender(seed, true)
	tk := &sndTake{seed: seed, style: sndStyleOf(seed), mix: mix, music: music, office: office,
		musicMono: music.mono(), officeMo: office.mono()}
	takes[seed] = tk
	return tk
}

func (s sndStereo) mono() []float64 {
	m := make([]float64, len(s.l))
	for i := range m {
		m[i] = (float64(s.l[i]) + float64(s.r[i])) / 2
	}
	return m
}

func sndDB(v float64) float64 { return 20 * math.Log10(v) }

// sndGoertzel is how strong frequency f is in x.
func sndGoertzel(x []float64, f float64) float64 {
	c := 2 * math.Cos(2*math.Pi*f/sndRate)
	var s1, s2 float64
	for _, v := range x {
		s1, s2 = v+c*s1-s2, s1
	}
	return (s1*s1 + s2*s2 - c*s1*s2) / float64(len(x)*len(x))
}

// sndFFT is the discrete Fourier transform of x (its length a power of two).
func sndFFT(x []complex128) {
	n := len(x)
	for i, j := 1, 0; i < n; i++ {
		bit := n >> 1
		for ; j&bit != 0; bit >>= 1 {
			j ^= bit
		}
		j ^= bit
		if i < j {
			x[i], x[j] = x[j], x[i]
		}
	}
	for size := 2; size <= n; size <<= 1 {
		w := cmplx.Exp(complex(0, -2*math.Pi/float64(size)))
		for start := 0; start < n; start += size {
			wk := complex(1, 0)
			for k := range size / 2 {
				a, b := x[start+k], x[start+k+size/2]*wk
				x[start+k], x[start+k+size/2] = a+b, a-b
				wk *= w
			}
		}
	}
}

// sndSpectrum is x's average power spectrum, windows of n samples
// overlapping by half (Welch's way); bin k is k*sndRate/n Hz.
func sndSpectrum(x []float64, n int) []float64 {
	p := make([]float64, n/2)
	win := make([]float64, n)
	for i := range win {
		win[i] = 0.5 - 0.5*math.Cos(2*math.Pi*float64(i)/float64(n))
	}
	buf := make([]complex128, n)
	count := 0
	for at := 0; at+n <= len(x); at += n / 2 {
		for i := range buf {
			buf[i] = complex(x[at+i]*win[i], 0)
		}
		sndFFT(buf)
		for k := range p {
			p[k] += real(buf[k])*real(buf[k]) + imag(buf[k])*imag(buf[k])
		}
		count++
	}
	for k := range p {
		p[k] /= float64(count)
	}
	return p
}

// sndCentroid is a spectrum's centre of weight (Hz): the brightness.
func sndCentroid(p []float64, n int) float64 {
	var num, den float64
	for k, v := range p {
		num += float64(k) * float64(sndRate) / float64(n) * v
		den += v
	}
	return num / den
}

// sndTonal is the most a single narrow line in the spectrum stands above
// its neighbourhood (dB), from 60 Hz to 12 kHz, and where: a steady tone,
// a whine or a whistle stands well above; noise does not.
func sndTonal(p []float64, n int) (excess, at float64) {
	hz := float64(sndRate) / float64(n)
	db := make([]float64, len(p))
	for k, v := range p {
		db[k] = 10 * math.Log10(v+1e-30)
	}
	excess = -100
	for k := int(60 / hz); k < int(12000/hz); k++ {
		w := max(12, int(0.08*float64(k)))
		var near []float64
		for j := max(k-w, 1); j <= min(k+w, len(db)-1); j++ {
			if j < k-3 || j > k+3 {
				near = append(near, db[j])
			}
		}
		sort.Float64s(near)
		if e := db[k] - near[len(near)/2]; e > excess {
			excess, at = e, float64(k)*hz
		}
	}
	return excess, at
}

// sndTempo is the tempo heard in x (beats a minute, 70 to 115): the beat
// that best lines up with where the sound grows louder.
func sndTempo(x []float64) float64 {
	const n, hop = 1024, 256
	var onset []float64
	prev := make([]float64, n/2)
	buf := make([]complex128, n)
	for at := 0; at+n <= len(x); at += hop {
		for i := range buf {
			buf[i] = complex(x[at+i]*(0.5-0.5*math.Cos(2*math.Pi*float64(i)/n)), 0)
		}
		sndFFT(buf)
		flux := 0.0
		for k := range prev {
			m := math.Log1p(1000 * cmplx.Abs(buf[k]))
			flux += max(m-prev[k], 0)
			prev[k] = m
		}
		onset = append(onset, flux)
	}
	mean := 0.0
	for _, v := range onset {
		mean += v
	}
	mean /= float64(len(onset))
	for i := range onset {
		onset[i] -= mean
	}
	ac := func(lag float64) float64 { // at a fractional lag: between the two nearest
		l := int(lag)
		f := lag - float64(l)
		sum := func(l int) float64 {
			s := 0.0
			for i := 0; i+l < len(onset); i++ {
				s += onset[i] * onset[i+l]
			}
			return s
		}
		return (1-f)*sum(l) + f*sum(l+1)
	}
	fps := float64(sndRate) / hop
	best, bestScore := 0.0, math.Inf(-1)
	for bpm := 70.0; bpm <= 115; bpm += 0.25 {
		l := 60 / bpm * fps
		if s := ac(l) + 0.5*ac(2*l) + 0.5*ac(l/2) + 0.25*ac(4*l); s > bestScore {
			best, bestScore = bpm, s
		}
	}
	return best
}

// sndChroma is how strong each of the 12 notes is (150 Hz to 2 kHz),
// summed over octaves.
func sndChroma(p []float64, n int) [12]float64 {
	var c [12]float64
	for k := 1; k < len(p); k++ {
		f := float64(k) * sndRate / float64(n)
		if f < 150 || f > 2000 {
			continue
		}
		note := int(math.Round(12*math.Log2(f/440))) + 69
		c[(note%12+12)%12] += math.Sqrt(p[k])
	}
	return c
}

func sndCorr(a, b []float64) float64 {
	var ma, mb float64
	for i := range a {
		ma += a[i]
		mb += b[i]
	}
	ma /= float64(len(a))
	mb /= float64(len(b))
	var ab, aa, bb float64
	for i := range a {
		ab += (a[i] - ma) * (b[i] - mb)
		aa += (a[i] - ma) * (a[i] - ma)
		bb += (b[i] - mb) * (b[i] - mb)
	}
	return ab / math.Sqrt(aa*bb)
}

// sndTopNotes is the n strongest notes in a chroma, strongest first.
func sndTopNotes(c [12]float64, n int) string {
	idx := []int{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11}
	sort.Slice(idx, func(i, j int) bool { return c[idx[i]] > c[idx[j]] })
	var names []string
	for _, k := range idx[:n] {
		names = append(names, sndNoteNames[k])
	}
	return strings.Join(names, " ")
}

// sndLevels is a sound's peak and loudness (dBFS).
func sndLevels(s sndStereo) (peak, rms float64) { return sndDB(s.peak()), sndDB(s.rms()) }

// sndReport is what a piece measures: its numbers, one line each part.
func sndReport(tk *sndTake) string {
	const n = 8192
	var b strings.Builder
	st := tk.style
	fmt.Fprintf(&b, "seed %x: %s\n", tk.seed, st)
	mp := sndSpectrum(tk.musicMono, n)
	fmt.Fprintf(&b, "  %.1f s; tempo set %.0f, heard %.1f bpm; key set %s, strongest notes heard %s\n",
		float64(len(tk.mix.l))/sndRate, st.bpm, sndTempo(tk.musicMono), sndKeyName(st.key, st.minor), sndTopNotes(sndChroma(mp, n), 5))
	for _, part := range []struct {
		name string
		s    sndStereo
		mono []float64
	}{{"mix", tk.mix, nil}, {"music", tk.music, tk.musicMono}, {"office", tk.office, tk.officeMo}} {
		mono := part.mono
		if mono == nil {
			mono = part.s.mono()
		}
		p := sndSpectrum(mono, n)
		pk, rms := sndLevels(part.s)
		ex, at := sndTonal(p, n)
		fmt.Fprintf(&b, "  %-6s peak %6.2f dBFS, RMS %6.1f dBFS, centroid %5.0f Hz, strongest narrow line %4.1f dB over its neighbours (%.0f Hz)\n",
			part.name, pk, rms, sndCentroid(p, n), ex, at)
	}
	return b.String()
}

// Each piece is 16 bars at its tempo, peaks at about -6 dBFS with no
// sample clipped, is neither faint nor loud, is wider than mono, joins end
// to start without a jump, and its bass is the strongest of its low end.
func TestSoundPieceLevels(t *testing.T) {
	if testing.Short() {
		t.Skip("makes whole pieces")
	}
	for _, seed := range sndChain(3) {
		tk := sndTakeOf(seed)
		l, r := tk.mix.l, tk.mix.r
		if secs, want := float64(len(l))/sndRate, sndBars*4*60/tk.style.bpm; math.Abs(secs-want) > 0.01 {
			t.Fatalf("seed %x: %.2f s, want %.2f", seed, secs, want)
		}
		pcm := tk.mix.pcm16()
		peak, sum, diff := 0.0, 0.0, 0.0
		for i := range l {
			a := float64(int16(binary.LittleEndian.Uint16(pcm[4*i:]))) / 32768
			b := float64(int16(binary.LittleEndian.Uint16(pcm[4*i+2:]))) / 32768
			peak = max(peak, math.Abs(a), math.Abs(b))
			sum += a*a + b*b
			diff += (a - b) * (a - b)
		}
		rms := math.Sqrt(sum / float64(2*len(l)))
		if peak >= 32767.0/32768 || sndDB(peak) < -6.5 || sndDB(peak) > -5.5 {
			t.Fatalf("seed %x: peak %.2f dBFS: want about -6, and nothing clipped", seed, sndDB(peak))
		}
		if sndDB(rms) < -30 || sndDB(rms) > -12 {
			t.Fatalf("seed %x: loudness %.1f dBFS RMS: want -30 to -12", seed, sndDB(rms))
		}
		if side := math.Sqrt(diff / float64(len(l))); side < rms/30 || side > rms {
			t.Fatalf("seed %x: stereo: the sides differ by %.1f dB under the whole: want a little width", seed, sndDB(rms/side))
		}
		n := len(l)
		if jump := math.Abs(float64(l[0]-l[n-1])) + math.Abs(float64(r[0]-r[n-1])); jump > 0.05 {
			t.Fatalf("seed %x: the end jumps %.3f to the start", seed, jump)
		}
		mono := tk.mix.mono()
		p := newSndPiece(seed)
		band := func(fs ...float64) float64 {
			s := 0.0
			for _, f := range fs {
				s += sndGoertzel(mono, f)
			}
			return s / float64(len(fs))
		}
		var roots []float64
		for _, c := range p.chords {
			roots = append(roots, sndHz(c.root))
		}
		bass := band(roots...)
		mids := band(400, 630, 800, 1000)
		highs := band(2500, 4000, 6000, 9000)
		if bass < 10*mids || bass < 100*highs {
			t.Fatalf("seed %x: the bass should lead the low end: bass %.3g, mids %.3g, highs %.3g", seed, bass, mids, highs)
		}
	}
}

// Pieces one after another are clearly different: heard at another tempo,
// with other notes (their chromas unalike) and another brightness or key.
func TestSoundPiecesDiffer(t *testing.T) {
	if testing.Short() {
		t.Skip("makes whole pieces")
	}
	const n = 8192
	seeds := sndChain(3)
	type heard struct {
		tempo, bright float64
		chroma        []float64
	}
	var hs []heard
	for _, seed := range seeds {
		tk := sndTakeOf(seed)
		p := sndSpectrum(tk.musicMono, n)
		c := sndChroma(p, n)
		h := heard{sndTempo(tk.musicMono), sndCentroid(p, n), c[:]}
		if math.Abs(h.tempo-tk.style.bpm) > 2 {
			t.Errorf("seed %x: tempo heard %.1f, set %.0f", seed, h.tempo, tk.style.bpm)
		}
		hs = append(hs, h)
	}
	for i := 1; i < len(hs); i++ {
		a, b := hs[i-1], hs[i]
		if math.Abs(a.tempo-b.tempo) < 3 {
			t.Errorf("pieces %d and %d: tempos %.1f and %.1f too close", i, i+1, a.tempo, b.tempo)
		}
		if r := sndCorr(a.chroma, b.chroma); r > 0.8 {
			t.Errorf("pieces %d and %d: their notes alike (chroma correlation %.2f)", i, i+1, r)
		}
	}
}

// The office has no steady tone in it (no hum, whine or whistle: no
// narrow line in its spectrum stands 12 dB over its neighbours), and it
// sits well under the music.
func TestSoundOfficeNoTones(t *testing.T) {
	if testing.Short() {
		t.Skip("makes whole pieces")
	}
	const n = 8192
	for _, seed := range sndChain(3) {
		tk := sndTakeOf(seed)
		ex, at := sndTonal(sndSpectrum(tk.officeMo, n), n)
		if ex > 12 {
			t.Errorf("seed %x: the office has a tone at %.0f Hz, %.1f dB over its neighbours", seed, at, ex)
		}
		if _, o := sndLevels(tk.office); o > sndDB(tk.music.rms())-12 {
			t.Errorf("seed %x: the office (%.1f dBFS) should be 12 dB under the music (%.1f)", seed, o, sndDB(tk.music.rms()))
		}
	}
}

// SY_SOUND_WAV=dir writes pieces there to listen to (never played here):
// for each, the mix, the music alone and the office alone, at the levels
// they have in the mix, and report.txt with each one's numbers.
// SY_SOUND_N sets how many (4).
func TestSoundPreview(t *testing.T) {
	dir := os.Getenv("SY_SOUND_WAV")
	if dir == "" {
		t.Skip("SY_SOUND_WAV=dir writes pieces there")
	}
	n := 4
	if v, err := strconv.Atoi(os.Getenv("SY_SOUND_N")); err == nil && v > 0 {
		n = v
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	var report strings.Builder
	for i, seed := range sndChain(n) {
		start := time.Now()
		tk := sndTakeOf(seed)
		made := time.Since(start)
		for name, s := range map[string]sndStereo{"mix": tk.mix, "music": tk.music, "office": tk.office} {
			if err := writeWAV(filepath.Join(dir, fmt.Sprintf("piece-%d-%s.wav", i+1, name)), s.pcm16()); err != nil {
				t.Fatal(err)
			}
		}
		r := fmt.Sprintf("piece %d (made in %.1f s) %s", i+1, made.Seconds(), sndReport(tk))
		report.WriteString(r)
		t.Log(r)
		takesMu.Lock()
		delete(takes, seed)
		takesMu.Unlock()
	}
	if err := os.WriteFile(filepath.Join(dir, "report.txt"), []byte(report.String()), 0o644); err != nil {
		t.Fatal(err)
	}
}

// writeWAV writes 48 kHz 16-bit stereo samples as a WAV file.
func writeWAV(path string, pcm []byte) error {
	var h bytes.Buffer
	put := func(v any) { binary.Write(&h, binary.LittleEndian, v) }
	h.WriteString("RIFF")
	put(uint32(36 + len(pcm)))
	h.WriteString("WAVEfmt ")
	put(uint32(16))
	put(uint16(1))
	put(uint16(2))
	put(uint32(sndRate))
	put(uint32(sndRate * 4))
	put(uint16(4))
	put(uint16(16))
	h.WriteString("data")
	put(uint32(len(pcm)))
	return os.WriteFile(path, append(h.Bytes(), pcm...), 0o644)
}
