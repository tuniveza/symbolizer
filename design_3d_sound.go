package main

// Music in the 3D room: while the room is in 3D a soft lounge loop plays,
// with the sound of an office under it. Nothing is recorded: every sound
// is worked out here, once, the first time it is wanted (soundLoop), and
// played round and round by the desktop's own player (pw-play, else
// paplay, else aplay), fed raw samples on its standard input. With no
// player there is no sound and nothing is said.
//
// The loop is 16 bars at 90 beats a minute (about 43 seconds): a deep,
// warm bass, soft electric-piano chords (ii-V-I in C, then ii-V-i in A
// minor) and brushed drums, over air conditioning, far-off voices that
// are never words, typing, a page now and then and a mug put down. Where
// the office sounds fall is chosen by a random generator with a fixed
// seed, so the loop does not repeat inside itself and is the same every
// time.
//
// M in 3D turns the music off and on for the session; SY_3D_SOUND=0 turns
// it off for good. It stops, with a short fade, as the room goes flat, as
// design mode is left and as sy ends.

import (
	"encoding/binary"
	"errors"
	"io"
	"math"
	"math/rand/v2"
	"os"
	"os/exec"
	"sync"
	"time"

	tea "charm.land/bubbletea/v2"
)

const (
	sndRate    = 48000                  // samples a second, each channel
	sndBeat    = sndRate * 60 / 90      // samples a beat: 90 beats a minute
	sndBar     = 4 * sndBeat            // samples a bar
	sndBars    = 16                     // bars in the loop
	sndFrames  = sndBars * sndBar       // the loop's length: samples a channel
	sndPeak    = 0.5                    // the loudest sample: -6 dBFS
	sndAhead   = 100 * time.Millisecond // fed this far ahead of the player
	sndChunk   = sndRate / 50           // fed 20 ms at a time
	sndFadeIn  = sndRate                // samples: 1 s
	sndFadeOut = sndRate * 3 / 50       // samples: 60 ms
)

var (
	// soundOn is whether the 3D room may play music (tests set it).
	soundOn = soundAllowed(os.Getenv)
	// soundMuted is M in 3D: the music off for the rest of the session.
	soundMuted bool
	roomSound  = &roomMusic{}
	// soundData gives the loop as 16-bit sndStereo samples (tests set it).
	soundData = soundLoop
	// soundStart starts a player that reads such samples (tests set it).
	soundStart = startSoundPlayer
)

// soundAllowed is false when SY_3D_SOUND=0.
func soundAllowed(getenv func(string) string) bool { return getenv("SY_3D_SOUND") != "0" }

// soundWanted is whether the music should play now.
func (m *Model) soundWanted() bool {
	return soundOn && !soundMuted && m.inDesign() && m.design != nil && m.design.room.on()
}

// soundCmd runs after every message: it starts the music as the room goes
// 3D and stops it as the room goes flat or design mode is left.
func (m *Model) soundCmd() tea.Cmd {
	roomSound.set(m.soundWanted())
	return nil
}

// soundMute is M in 3D: the music off, or on again.
func (m *Model) soundMute() {
	if !soundOn {
		m.say("Music is off: SY_3D_SOUND=0.")
		return
	}
	soundMuted = !soundMuted
	if soundMuted {
		m.say("Music off.")
	} else {
		m.say("Music on.")
	}
}

// soundSink is a running player.
type soundSink interface {
	io.Writer
	Stop() // its input closed, and it killed if it does not end soon
}

// roomMusic starts and stops the player, off Bubble Tea's loop.
type roomMusic struct {
	mu   sync.Mutex
	want bool
	stop chan struct{} // closed to stop the one playing
	done chan struct{} // closed once it has stopped
	pos  int           // where in the loop it got to (bytes): it goes on from there
}

// set asks for the music on or off.
func (s *roomMusic) set(play bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.want == play {
		return
	}
	s.want = play
	if !play {
		close(s.stop)
		return
	}
	stop, done, last := make(chan struct{}), make(chan struct{}), s.done
	s.stop, s.done = stop, done
	go s.play(last, stop, done)
}

// quiet stops the music and waits, a second at most, until it has (sy ending).
func (s *roomMusic) quiet() {
	s.set(false)
	s.mu.Lock()
	done := s.done
	s.mu.Unlock()
	if done == nil {
		return
	}
	select {
	case <-done:
	case <-time.After(time.Second):
	}
}

// play makes the loop (the first time), starts a player and feeds it
// until stop is closed. The one before (last) has ended first.
func (s *roomMusic) play(last, stop, done chan struct{}) {
	defer close(done)
	if last != nil {
		<-last
	}
	pcm := soundData()
	select {
	case <-stop:
		return
	default:
	}
	if len(pcm) < 4 {
		return
	}
	out, err := soundStart()
	if err != nil {
		return
	}
	defer out.Stop()
	s.mu.Lock()
	pos := s.pos % len(pcm) &^ 3
	s.mu.Unlock()
	pos = feedSound(out, pcm, pos, stop)
	s.mu.Lock()
	s.pos = pos
	s.mu.Unlock()
}

// feedSound writes the loop from byte pos, round and round, about sndAhead
// ahead of the player, fading in at first. Once stop is closed it fades
// out, and gives where it got to.
func feedSound(out io.Writer, pcm []byte, pos int, stop chan struct{}) int {
	buf := make([]byte, sndChunk*4)
	start, sent := time.Now(), 0
	gain, step, stopping := 0.0, 1.0/sndFadeIn, false
	for {
		if !stopping {
			select {
			case <-stop:
				stopping, step = true, -1.0/sndFadeOut
			default:
			}
		}
		if ahead := time.Duration(sent)*time.Second/sndRate - time.Since(start); !stopping && ahead > sndAhead {
			select {
			case <-stop:
			case <-time.After(ahead - sndAhead):
			}
			continue
		}
		for i := 0; i < len(buf); i += 4 {
			gain = min(max(gain+step, 0), 1)
			l := float64(int16(binary.LittleEndian.Uint16(pcm[pos:])))
			r := float64(int16(binary.LittleEndian.Uint16(pcm[pos+2:])))
			binary.LittleEndian.PutUint16(buf[i:], uint16(int16(math.Round(l*gain))))
			binary.LittleEndian.PutUint16(buf[i+2:], uint16(int16(math.Round(r*gain))))
			pos = (pos + 4) % len(pcm)
		}
		if _, err := out.Write(buf); err != nil {
			return pos
		}
		sent += sndChunk
		if stopping && gain == 0 {
			return pos
		}
	}
}

// soundPlayers are the players tried, in order, each reading 48 kHz
// 16-bit sndStereo samples on its input.
var soundPlayers = [][]string{
	{"pw-play", "--raw", "--format", "s16", "--rate", "48000", "--channels", "2", "--media-role", "Music", "-"},
	{"paplay", "--raw", "--format=s16le", "--rate=48000", "--channels=2"},
	{"aplay", "-q", "-t", "raw", "-f", "S16_LE", "-r", "48000", "-c", "2"},
}

var errNoPlayer = errors.New("no sound player")

// startSoundPlayer starts the first player there is, quiet on the screen.
func startSoundPlayer() (soundSink, error) {
	for _, p := range soundPlayers {
		path, err := exec.LookPath(p[0])
		if err != nil {
			continue
		}
		c := exec.Command(path, p[1:]...)
		in, err := c.StdinPipe()
		if err != nil {
			continue
		}
		if c.Start() != nil {
			in.Close()
			continue
		}
		return &soundProc{c: c, in: in}, nil
	}
	return nil, errNoPlayer
}

// soundProc is a player program.
type soundProc struct {
	c  *exec.Cmd
	in io.WriteCloser
}

func (p *soundProc) Write(b []byte) (int, error) { return p.in.Write(b) }

// Stop closes the player's input, so it plays what it has and ends; one
// that has not ended soon after is killed.
func (p *soundProc) Stop() {
	p.in.Close()
	done := make(chan struct{})
	go func() {
		p.c.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(400 * time.Millisecond):
		p.c.Process.Kill()
		<-done
	}
}

var (
	loopOnce sync.Once
	loopPCM  []byte
)

// soundLoop is the loop as 16-bit little-endian sndStereo samples, made the
// first time it is asked for (about a second's work).
func soundLoop() []byte {
	loopOnce.Do(func() { loopPCM = makeLoop().pcm16() })
	return loopPCM
}

// sndStereo is sound as two channels of samples.
type sndStereo struct{ l, r []float32 }

func newSndStereo(n int) sndStereo { return sndStereo{make([]float32, n), make([]float32, n)} }

// sndPanG is how much of a sound goes left and how much right.
type sndPanG struct{ l, r float64 }

// sndPan is -1 left, 0 the middle, 1 right, as loud wherever it is.
func sndPan(p float64) sndPanG {
	a := (p + 1) * math.Pi / 4
	return sndPanG{math.Cos(a) * math.Sqrt2, math.Sin(a) * math.Sqrt2}
}

var sndMid = sndPan(0)

// add adds v at sample i, which runs round the loop's end to its start.
func (s sndStereo) add(i int, v float64, g sndPanG) {
	n := len(s.l)
	i %= n
	if i < 0 {
		i += n
	}
	s.l[i] += float32(v * g.l)
	s.r[i] += float32(v * g.r)
}

func (s sndStereo) zero() {
	clear(s.l)
	clear(s.r)
}

func (s sndStereo) peak() float64 {
	p := 0.0
	for i := range s.l {
		p = max(p, math.Abs(float64(s.l[i])), math.Abs(float64(s.r[i])))
	}
	return p
}

func (s sndStereo) rms() float64 {
	sum := 0.0
	for i := range s.l {
		sum += float64(s.l[i])*float64(s.l[i]) + float64(s.r[i])*float64(s.r[i])
	}
	return math.Sqrt(sum / float64(2*len(s.l)))
}

// pcm16 is s as 16-bit little-endian samples, left and right in turn.
func (s sndStereo) pcm16() []byte {
	b := make([]byte, 4*len(s.l))
	to := func(v float32) uint16 {
		return uint16(int16(math.Round(min(max(float64(v), -1), 1) * 32767)))
	}
	for i := range s.l {
		binary.LittleEndian.PutUint16(b[4*i:], to(s.l[i]))
		binary.LittleEndian.PutUint16(b[4*i+2:], to(s.r[i]))
	}
	return b
}

// makeLoop makes the loop: each part made on its own, set to its level
// (by its loudness, or by its loudest moment for the ones that come and
// go), and added in; then the whole set to peak at sndPeak.
func makeLoop() sndStereo {
	mix, part := newSndStereo(sndFrames), newSndStereo(sndFrames)
	for _, p := range []struct {
		make   func(sndStereo)
		byPeak bool
		level  float64
	}{
		{loopBass, false, 1},
		{loopKeys, false, 0.42},
		{loopKick, true, 0.9},
		{loopRim, true, 0.3},
		{loopShaker, true, 0.2},
		{loopBrush, true, 0.12},
		{loopRoom, false, 0.07},
		{loopVoices, false, 0.07},
		{loopTyping, true, 0.2},
		{loopPaper, true, 0.12},
		{loopMug, true, 0.15},
	} {
		part.zero()
		p.make(part)
		g := part.rms()
		if p.byPeak {
			g = part.peak()
		}
		if g == 0 {
			continue
		}
		g = p.level / g
		for i := range mix.l {
			mix.l[i] += part.l[i] * float32(g)
			mix.r[i] += part.r[i] * float32(g)
		}
	}
	g := float32(sndPeak / mix.peak())
	for i := range mix.l {
		mix.l[i] *= g
		mix.r[i] *= g
	}
	return mix
}

// sndRand is a random generator with a fixed seed: the loop is the same every time.
func sndRand(part uint64) *rand.Rand { return rand.New(rand.NewPCG(0x5359_3344, part)) }

// sndNoise is white noise, -1 to 1.
func sndNoise(rng *rand.Rand) float64 { return 2*rng.Float64() - 1 }

// sndHz is a note's pitch (MIDI numbers: 69 is A 440).
func sndHz(note int) float64 { return 440 * math.Pow(2, float64(note-69)/12) }

// sndAt is the sample beat beats into bar falls on: off-beat eighths swing a
// little late.
func sndAt(bar int, beat float64) int {
	if f := beat - math.Floor(beat); math.Abs(f-0.5) < 1e-9 {
		beat += 0.08
	}
	return bar*sndBar + int(beat*sndBeat)
}

// sndBiquad is a two-pole filter (the audio EQ cookbook's).
type sndBiquad struct{ b0, b1, b2, a1, a2, x1, x2, y1, y2 float64 }

// sndFilter is a new biquad: 'l' low pass, 'h' high pass, 'b' band pass.
func sndFilter(kind byte, f, q float64) *sndBiquad {
	b := &sndBiquad{}
	b.set(kind, f, q)
	return b
}

func (b *sndBiquad) set(kind byte, f, q float64) {
	w := 2 * math.Pi * f / sndRate
	c, al := math.Cos(w), math.Sin(w)/(2*q)
	a0 := 1 + al
	switch kind {
	case 'l':
		b.b0, b.b1, b.b2 = (1-c)/2, 1-c, (1-c)/2
	case 'h':
		b.b0, b.b1, b.b2 = (1+c)/2, -1-c, (1+c)/2
	default:
		b.b0, b.b1, b.b2 = al, 0, -al
	}
	b.b0, b.b1, b.b2, b.a1, b.a2 = b.b0/a0, b.b1/a0, b.b2/a0, -2*c/a0, (1-al)/a0
}

func (b *sndBiquad) step(x float64) float64 {
	y := b.b0*x + b.b1*b.x1 + b.b2*b.x2 - b.a1*b.y1 - b.a2*b.y2
	b.x2, b.x1, b.y2, b.y1 = b.x1, x, b.y1, y
	return y
}

// sndChord is a bar's chord: the bass's root and the piano's notes.
type sndChord struct {
	root         int // the bass's root, a MIDI note low down
	third, fifth int // above the root: 3 or 4, 6 or 7
	seventh      int // 10 or 11
	keys         []int
}

var (
	chDm9   = sndChord{38, 3, 7, 10, []int{53, 57, 60, 64}}
	chG13   = sndChord{31, 4, 7, 10, []int{53, 57, 59, 64}}
	chCmaj9 = sndChord{36, 4, 7, 11, []int{52, 55, 59, 62}}
	chC69   = sndChord{36, 4, 7, 11, []int{55, 57, 62, 64}}
	chBm7b5 = sndChord{35, 3, 6, 10, []int{53, 57, 59, 62}}
	chE7b9  = sndChord{40, 4, 7, 10, []int{50, 53, 56, 59}}
	chAm9   = sndChord{33, 3, 7, 10, []int{55, 59, 60, 64}}
	chA7b9  = sndChord{33, 4, 7, 10, []int{55, 58, 61, 64}}
	// loopChords: ii-V-I in C, then ii-V-i in A minor, twice; the last
	// bar leads back to the first.
	loopChords = [sndBars]sndChord{
		chDm9, chG13, chCmaj9, chC69, chBm7b5, chE7b9, chAm9, chAm9,
		chDm9, chG13, chCmaj9, chC69, chBm7b5, chE7b9, chAm9, chA7b9,
	}
)

// sndBassNote is a note of a bass pattern: its beat, which note of the chord
// (R root, 3, 5, 7, 8 the octave, A a step from the next bar's root), how
// many beats it lasts and how hard it is played.
type sndBassNote struct {
	beat     float64
	deg      byte
	dur, vel float64
}

var bassPatterns = [][]sndBassNote{
	{{0, 'R', 1.3, 1}, {1.5, '5', 0.4, 0.7}, {2.5, 'R', 0.4, 0.85}, {3, '8', 0.35, 0.6}, {3.5, 'A', 0.45, 0.75}}, // syncopated
	{{0, 'R', 0.9, 1}, {1, '3', 0.9, 0.8}, {2, '5', 0.9, 0.85}, {3, 'A', 0.9, 0.8}},                              // walking
	{{0, 'R', 1.6, 1}, {1.75, 'R', 0.2, 0.5}, {2.5, '5', 0.9, 0.85}, {3.5, '7', 0.45, 0.7}},                      // sparse
}

// loopBassPattern is the pattern each bar plays.
var loopBassPattern = [sndBars]int{0, 0, 2, 0, 0, 0, 2, 0, 1, 0, 1, 2, 1, 0, 1, 0}

// loopBass is the bass: a sine with a little of its overtones, softly
// saturated, deep (49 to 165 Hz), playing a groove round the chords.
func loopBass(s sndStereo) {
	rng := sndRand(1)
	for bar := range sndBars {
		c, next := loopChords[bar], loopChords[(bar+1)%sndBars]
		for _, b := range bassPatterns[loopBassPattern[bar]] {
			n := c.root
			switch b.deg {
			case '3':
				n += c.third
			case '5':
				n += c.fifth
			case '7':
				n += c.seventh - 12
			case '8':
				n += 12
			case 'A':
				n = next.root + 1
				if rng.IntN(2) == 0 || n > 45 {
					n = next.root - 1
				}
			}
			jitter := int((rng.Float64() - 0.5) * 0.006 * sndRate)
			bassTone(s, sndAt(bar, b.beat)+jitter, sndHz(n), b.dur*sndBeat, b.vel*(0.92+0.08*rng.Float64()))
		}
	}
}

// bassTone is one bass note: f Hz, dur samples long, then let go.
func bassTone(s sndStereo, start int, f, dur, vel float64) {
	w := 2 * math.Pi * f / sndRate
	n := int(dur) + sndRate/5
	sat := math.Tanh(1.6)
	for i := range n {
		t := float64(i) / sndRate
		ph := w * float64(i)
		pluck := 0.3 * math.Exp(-t/0.08)
		v := math.Sin(ph) + (0.2+pluck)*math.Sin(2*ph) + 0.06*math.Sin(3*ph)
		v = math.Tanh(1.6*v) / sat
		e := min(t/0.006, 1) * (0.6 + 0.4*math.Exp(-t/0.25))
		if float64(i) > dur {
			e *= math.Exp(-(float64(i) - dur) / sndRate / 0.03)
		}
		s.add(start+i, v*e*vel, sndMid)
	}
}

// sndKeyHit is a chord played: when (beats into the bar), for how many beats, how hard.
type sndKeyHit struct{ beat, dur, vel float64 }

var keyPatterns = [][]sndKeyHit{
	{{0, 2.3, 0.7}, {2.5, 1.3, 0.55}},
	{{0, 1.3, 0.65}, {1.5, 2.2, 0.6}},
	{{0, 3.7, 0.7}},
	{{0, 0.4, 0.6}, {0.5, 1.4, 0.55}, {2.5, 1.3, 0.5}},
}

// loopKeys is the electric piano: the chords, a little strummed, gently
// panned to and fro, and a short run of notes every fourth bar.
func loopKeys(s sndStereo) {
	rng := sndRand(2)
	for bar := range sndBars {
		c := loopChords[bar]
		for _, h := range keyPatterns[rng.IntN(len(keyPatterns))] {
			start := sndAt(bar, h.beat) + int((rng.Float64()-0.5)*0.008*sndRate)
			for k, n := range c.keys {
				strum := k * int((0.006+0.008*rng.Float64())*sndRate)
				sndEPiano(s, start+strum, sndHz(n), h.dur*sndBeat, h.vel*(0.85+0.15*rng.Float64()))
			}
		}
		if bar%4 == 3 { // a run down the chord, an octave up
			for i := range 4 {
				n := c.keys[len(c.keys)-1-i] + 12
				sndEPiano(s, sndAt(bar, 2+0.5*float64(i)), sndHz(n), 0.45*sndBeat, 0.5)
			}
		}
	}
	// A slow tremolo across the two sides, a whole number of times round the loop.
	k := math.Round(3.6 * sndFrames / sndRate)
	for i := range s.l {
		g := 0.25 * math.Sin(2*math.Pi*k*float64(i)/sndFrames)
		s.l[i] *= float32(1 + g)
		s.r[i] *= float32(1 - g)
	}
}

// sndEPiano is one electric-piano note (a soft FM tone with a short bell at
// its start): f Hz, held dur samples, then let go.
func sndEPiano(s sndStereo, start int, f, dur, vel float64) {
	w := 2 * math.Pi * f / sndRate
	n := int(dur) + sndRate*7/10
	tau := 1.8 * math.Pow(220/f, 0.3) // higher notes die away sooner
	for i := range n {
		t := float64(i) / sndRate
		ph := w * float64(i)
		index := 0.15 + 1.1*vel*math.Exp(-t/0.35)
		v := math.Sin(ph+index*math.Sin(ph)) + 0.08*vel*math.Sin(4*ph)*math.Exp(-t/0.05)
		e := min(t/0.004, 1) * math.Exp(-t/tau)
		if float64(i) > dur {
			e *= math.Exp(-(float64(i) - dur) / sndRate / 0.12)
		}
		s.add(start+i, 0.3*v*e*vel, sndMid)
	}
}

// loopKick is a soft kick on the first beat and the off-beat of the
// third, and now and then a quiet one before the third.
func loopKick(s sndStereo) {
	rng := sndRand(3)
	for bar := range sndBars {
		sndKick(s, sndAt(bar, 0), 1)
		sndKick(s, sndAt(bar, 2.5), 0.8)
		if rng.Float64() < 0.3 {
			sndKick(s, sndAt(bar, 1.75), 0.45)
		}
	}
}

func sndKick(s sndStereo, start int, vel float64) {
	ph := 0.0
	for i := range sndRate / 2 {
		t := float64(i) / sndRate
		ph += 2 * math.Pi * (46 + 70*math.Exp(-t/0.03)) / sndRate
		s.add(start+i, vel*min(t/0.002, 1)*math.Exp(-t/0.2)*math.Sin(ph), sndMid)
	}
}

// loopRim is a rim click on the second and fourth beats, a little left,
// with a few quiet ones before every eighth bar's end.
func loopRim(s sndStereo) {
	rng := sndRand(4)
	g := sndPan(-0.25)
	for bar := range sndBars {
		for _, b := range []float64{1, 3} {
			sndRim(s, sndAt(bar, b)+int((rng.Float64()-0.5)*0.006*sndRate), 0.85+0.15*rng.Float64(), g, rng)
		}
		if bar%8 == 7 {
			for _, b := range []float64{3.25, 3.5, 3.75} {
				sndRim(s, sndAt(bar, b), 0.35, g, rng)
			}
		}
	}
}

func sndRim(s sndStereo, start int, vel float64, g sndPanG, rng *rand.Rand) {
	bp := sndFilter('b', 2800, 1.5)
	for i := range sndRate * 12 / 100 {
		t := float64(i) / sndRate
		v := 0.6*math.Sin(2*math.Pi*1700*t)*math.Exp(-t/0.015) +
			0.45*math.Sin(2*math.Pi*820*t)*math.Exp(-t/0.028) +
			0.8*bp.step(sndNoise(rng))*math.Exp(-t/0.006)
		s.add(start+i, v*vel, g)
	}
}

// loopShaker is a shaker on every eighth, a little right, the off-beats
// louder.
func loopShaker(s sndStereo) {
	rng := sndRand(5)
	g := sndPan(0.35)
	for bar := range sndBars {
		for e := range 8 {
			vel := 0.5
			if e%2 == 1 {
				vel = 0.85
			}
			sndShake(s, sndAt(bar, float64(e)/2)+int((rng.Float64()-0.5)*0.006*sndRate), vel*(0.85+0.15*rng.Float64()), g, rng)
		}
	}
}

func sndShake(s sndStereo, start int, vel float64, g sndPanG, rng *rand.Rand) {
	hp := sndFilter('h', 6500, 0.7)
	for i := range sndRate * 14 / 100 {
		t := float64(i) / sndRate
		e := min(t/0.006, 1) * math.Exp(-max(t-0.006, 0)/0.03)
		s.add(start+i, vel*e*hp.step(sndNoise(rng)), g)
	}
}

// loopBrush is a soft brush swish with the rim clicks.
func loopBrush(s sndStereo) {
	rng := sndRand(6)
	g := sndPan(-0.1)
	for bar := range sndBars {
		for _, b := range []float64{1, 3} {
			bp := sndFilter('b', 3500, 0.6)
			start := sndAt(bar, b) - sndRate/50
			for i := range sndRate * 4 / 10 {
				t := float64(i) / sndRate
				e := math.Pow(math.Sin(math.Min(t/0.04, 1)*math.Pi/2), 2) * math.Exp(-max(t-0.04, 0)/0.1)
				s.add(start+i, e*bp.step(sndNoise(rng)), g)
			}
		}
	}
}

// sndBed fills s with a steady sound, next giving a sample at a time (i is
// where in the loop it is). It runs a little past the loop's end and fades
// that over the start, so there is no seam where the loop comes round.
func sndBed(s sndStereo, next func(i int) (l, r float64)) {
	n, fade := len(s.l), sndRate/2
	for i := n - fade; i < n; i++ {
		next(i) // the filters settled first
	}
	for i := range n + fade {
		l, r := next(i % n)
		if i < n {
			s.l[i] += float32(l)
			s.r[i] += float32(r)
			continue
		}
		j := i - n
		x := float64(j) / float64(fade) * math.Pi / 2
		a, b := math.Cos(x), math.Sin(x) // the run on fades out as the start fades in
		s.l[j] = s.l[j]*float32(b) + float32(l*a)
		s.r[j] = s.r[j]*float32(b) + float32(r*a)
	}
}

// loopRoom is the room's own sound: the air conditioning's low rumble and
// soft hiss, a different hiss each side, and a faint hum.
func loopRoom(s sndStereo) {
	rng := sndRand(7)
	rumble := sndFilter('l', 140, 0.7)
	fanL, fanR := sndFilter('b', 450, 0.8), sndFilter('b', 470, 0.8)
	hissL, hissR := sndFilter('l', 2500, 0.7), sndFilter('l', 2400, 0.7)
	sndBed(s, func(int) (float64, float64) {
		low := 5 * rumble.step(sndNoise(rng))
		return low + 0.5*fanL.step(sndNoise(rng)) + 0.12*hissL.step(sndNoise(rng)),
			low + 0.5*fanR.step(sndNoise(rng)) + 0.12*hissR.step(sndNoise(rng))
	})
	hum := 0.1 * s.rms()
	k := math.Round(100.0 * sndFrames / sndRate) // 100 Hz, a whole number of times round the loop
	for i := range s.l {
		v := float32(hum * math.Sin(2*math.Pi*k*float64(i)/sndFrames))
		s.l[i] += v
		s.r[i] += v
	}
}

// sndTalker is one far-off voice: a buzz and breath through two formants that
// wander at random, a syllable at a time, so it is never words.
type sndTalker struct {
	g          sndPanG
	scale      float64  // the voice's size: its pitch and formants
	segs       [][2]int // when it talks (samples: may run past the loop's end)
	pitch, ph  float64
	left       int // samples to the next syllable
	amp, ampTo float64
	f1, f2     float64
	f1To, f2To float64
	gate       float64
	b1, b2     *sndBiquad
}

// talking is whether the voice talks at sample i of the loop.
func (k *sndTalker) talking(i int) bool {
	for _, sg := range k.segs {
		if i >= sg[0] && i < sg[1] || i+sndFrames >= sg[0] && i+sndFrames < sg[1] {
			return true
		}
	}
	return false
}

func (k *sndTalker) next(i int, rng *rand.Rand) float64 {
	if k.left--; k.left <= 0 {
		k.left = int((0.1 + 0.22*rng.Float64()) * sndRate)
		k.ampTo = 0.25 + 0.75*rng.Float64()
		if rng.Float64() < 0.15 {
			k.ampTo = 0.03 // a gap between words
		}
		k.f1To, k.f2To = 300+500*rng.Float64(), 900+1400*rng.Float64()
		k.pitch = 120 * k.scale * (0.85 + 0.3*rng.Float64())
	}
	k.amp += (k.ampTo - k.amp) * 0.0015
	k.f1 += (k.f1To - k.f1) * 0.0004
	k.f2 += (k.f2To - k.f2) * 0.0004
	on := 0.0
	if k.talking(i) {
		on = 1
	}
	k.gate += (on - k.gate) * 0.0001 // fades in and out over a fifth of a second
	if i%64 == 0 {
		k.b1.set('b', k.f1*k.scale, 4)
		k.b2.set('b', k.f2*k.scale, 5)
	}
	if k.ph += k.pitch / sndRate; k.ph >= 1 {
		k.ph--
	}
	src := 0.5*(1-2*k.ph) + 0.5*sndNoise(rng)
	return (k.b1.step(src) + 0.6*k.b2.step(src)) * k.amp * k.gate
}

// loopVoices is people talking further off: four voices, each on and off,
// muffled by the distance and blurred by the room.
func loopVoices(s sndStereo) {
	rng := sndRand(8)
	var ks []*sndTalker
	for _, p := range []float64{-0.7, -0.2, 0.3, 0.75} {
		k := &sndTalker{g: sndPan(p), scale: 0.8 + 0.6*rng.Float64(), f1: 500, f2: 1500,
			b1: sndFilter('b', 500, 4), b2: sndFilter('b', 1500, 5)}
		for t := int(rng.Float64() * 4 * sndRate); t < sndFrames; {
			d := int((1.5 + 5.5*rng.Float64()) * sndRate)
			k.segs = append(k.segs, [2]int{t, t + d})
			t += d + int((1+5*rng.Float64())*sndRate)
		}
		ks = append(ks, k)
	}
	lpL, lpR := sndFilter('l', 1100, 0.7), sndFilter('l', 1100, 0.7)
	dl, dr := make([]float64, sndRate*17/1000), make([]float64, sndRate*29/1000)
	j := 0
	sndBed(s, func(i int) (float64, float64) {
		var l, r float64
		for _, k := range ks {
			v := k.next(i, rng)
			l += v * k.g.l
			r += v * k.g.r
		}
		l, r = lpL.step(l), lpR.step(r)
		el, er := dl[j%len(dl)], dr[j%len(dr)] // the room: each side heard again from the other, a moment later
		dl[j%len(dl)], dr[j%len(dr)] = l, r
		j++
		return l + 0.35*er, r + 0.35*el
	})
}

// loopTyping is two people typing in bursts, one each side, with pauses.
func loopTyping(s sndStereo) {
	rng := sndRand(9)
	for k, side := range []float64{-0.55, 0.5} {
		g, loud := sndPan(side), 1-0.3*float64(k)
		for t := rng.Float64() * 8 * sndRate; t < sndFrames; t += (4 + 12*rng.Float64()) * sndRate {
			for range 5 + rng.IntN(30) {
				sndKey(s, int(t), rng.Float64() < 0.15, loud*(0.7+0.3*rng.Float64()), g, rng)
				t += (0.07 + 0.15*rng.Float64()) * sndRate
				if rng.Float64() < 0.08 {
					t += (0.3 + 0.5*rng.Float64()) * sndRate // a pause to think
				}
			}
		}
	}
}

// keystroke is a key pressed (a click, and a second as it bottoms out),
// or the space bar, lower.
func sndKey(s sndStereo, start int, space bool, vel float64, g sndPanG, rng *rand.Rand) {
	f, thud, decay := 2200+2000*rng.Float64(), 160+60*rng.Float64(), 0.012
	if space {
		f, thud, decay, vel = 1200, 110, 0.02, vel*1.2
	}
	bp := sndFilter('b', f, 1.4)
	bottom := int((0.008 + 0.008*rng.Float64()) * sndRate)
	for i := range sndRate * 6 / 100 {
		t := float64(i) / sndRate
		e := math.Exp(-t / 0.0035)
		if i >= bottom {
			e += 0.6 * math.Exp(-float64(i-bottom)/sndRate/0.003)
		}
		v := bp.step(sndNoise(rng))*e + 0.5*math.Sin(2*math.Pi*thud*t)*math.Exp(-t/decay)
		s.add(start+i, v*vel, g)
	}
}

// loopPaper is a page turned or shuffled now and then.
func loopPaper(s sndStereo) {
	rng := sndRand(10)
	for t := (6 + 10*rng.Float64()) * sndRate; t < sndFrames; t += (14 + 18*rng.Float64()) * sndRate {
		g := sndPan(1.4*rng.Float64() - 0.7)
		bp, hp := sndFilter('b', 3000, 0.5), sndFilter('h', 900, 0.7)
		n := int((0.5 + 0.9*rng.Float64()) * sndRate)
		crackle := 0.0
		for i := range n {
			crackle *= 0.997
			if rng.Float64() < 0.004 {
				crackle += rng.Float64()
			}
			w := math.Sin(math.Pi * float64(i) / float64(n))
			s.add(int(t)+i, hp.step(bp.step(sndNoise(rng)))*(0.25+crackle)*w*w, g)
		}
	}
}

// loopMug is a mug put down now and then: a soft knock and a short ring,
// and sometimes a spoon's clink after.
func loopMug(s sndStereo) {
	rng := sndRand(11)
	for t := (10 + 8*rng.Float64()) * sndRate; t < sndFrames; t += (18 + 22*rng.Float64()) * sndRate {
		g := sndPan(1.2*rng.Float64() - 0.6)
		sndClink(s, int(t), 2300+600*rng.Float64(), 1, true, g, rng)
		if rng.Float64() < 0.5 {
			sndClink(s, int(t)+int((0.12+0.08*rng.Float64())*sndRate), 3000+800*rng.Float64(), 0.35, false, g, rng)
		}
	}
}

func sndClink(s sndStereo, start int, f, vel float64, knock bool, g sndPanG, rng *rand.Rand) {
	ratios := []float64{1, 1.53, 2.27, 3.05}
	amps := []float64{1, 0.55, 0.35, 0.2}
	taus := []float64{0.3, 0.2, 0.12, 0.08}
	lp := sndFilter('l', 400, 0.7)
	for i := range sndRate * 6 / 10 {
		t := float64(i) / sndRate
		v := 0.0
		for p, r := range ratios {
			v += amps[p] * math.Sin(2*math.Pi*f*r*t) * math.Exp(-t/taus[p])
		}
		v *= 0.6 * min(t/0.0005, 1)
		if knock {
			v += (0.5*math.Sin(2*math.Pi*95*t) + 2*lp.step(sndNoise(rng))) * math.Exp(-t/0.03)
		}
		s.add(start+i, v*vel, g)
	}
}
