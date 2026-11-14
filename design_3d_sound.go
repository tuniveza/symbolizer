package main

// Music in the 3D room: while the room is in 3D a soft lounge piece plays,
// with the sound of an office under it. Nothing is recorded: every sound
// is worked out here, off Bubble Tea's loop, and played round and round
// by the desktop's own player (pw-play, else paplay, else aplay), fed raw
// samples on its standard input. With no player there is no sound and
// nothing is said.
//
// Each time the room goes 3D a different piece plays. A piece is made
// from a seed (a random number picked each time), and the same seed
// always makes the same piece. From it come the feel (swing, bossa, a
// laid-back neo-soul groove or a slow ballad), the tempo (80 to 100 beats
// a minute), the key, the chords (one of several jazz and lounge
// progressions), the bass's groove, the drums, the instrument on the
// chords (an electric piano, a reedier one, a soft pad or a muted guitar)
// and the one with the tune (short phrases and runs up the chords). A
// piece is 16 bars, about 40 to 48 seconds round. The next piece is made
// in the background while one plays, so the next 3D starts at once, and
// it is always clearly unlike the one before (sndUnlike).
//
// Under it is an office, made the way the real sounds are made: air
// conditioning (soft rumble and broad hiss, no whine), people talking
// further off (voices that rise and fall a syllable at a time, muffled by
// distance and blurred by the room, never words), typing (key down and up,
// in words and bursts, a space bar now and then), and once in a while a
// page, a mug put down, a chair's creak or someone walking past. None of
// it holds a steady pitch, so none of it whistles or hums.
//
// M in 3D turns the music off and on for the session; SY_3D_SOUND=0 turns
// it off for good. It stops, with a short fade, as the room goes flat, as
// design mode is left and as sy ends.

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"math/rand/v2"
	"os"
	"os/exec"
	"slices"
	"sync"
	"time"

	tea "charm.land/bubbletea/v2"
)

const (
	sndRate    = 48000                  // samples a second, each channel
	sndBars    = 16                     // bars in a piece
	sndPeak    = 0.5                    // the loudest sample: -6 dBFS
	sndAhead   = 100 * time.Millisecond // fed this far ahead of the player
	sndChunk   = sndRate / 50           // fed 20 ms at a time
	sndFadeIn  = sndRate * 3 / 2        // samples: 1.5 s
	sndFadeOut = sndRate * 3 / 50       // samples: 60 ms
)

var (
	// soundOn is whether the 3D room may play music (tests set it).
	soundOn = soundAllowed(os.Getenv)
	// soundMuted is M in 3D: the music off for the rest of the session.
	soundMuted bool
	roomSound  = &roomMusic{}
	// soundData makes the piece for a seed as 16-bit stereo samples (tests set it).
	soundData = soundPiece
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
	last uint64        // the seed of the piece played last (0: none yet)
	next *sndNext      // the next piece, made in the background
}

// sndNext is a piece being made: pcm is ready once ready is closed.
type sndNext struct {
	seed  uint64
	ready chan struct{}
	pcm   []byte
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

// prepare starts making a piece unlike the last one played. s.mu is held.
func (s *roomMusic) prepare() *sndNext {
	n := &sndNext{seed: sndNewSeed(s.last), ready: make(chan struct{})}
	data := soundData
	go func() {
		n.pcm = data(n.seed)
		close(n.ready)
	}()
	return n
}

// play takes the next piece (making it now if it is not on its way),
// starts a player and feeds it until stop is closed; meanwhile the piece
// after is made. The one before (last) has ended first.
func (s *roomMusic) play(last, stop, done chan struct{}) {
	defer close(done)
	if last != nil {
		<-last
	}
	s.mu.Lock()
	n := s.next
	if n == nil {
		n = s.prepare()
	}
	s.next = nil
	s.mu.Unlock()
	keep := func() { // not heard: kept for next time
		s.mu.Lock()
		if s.next == nil {
			s.next = n
		}
		s.mu.Unlock()
	}
	select {
	case <-stop:
		keep()
		return
	case <-n.ready:
	}
	if len(n.pcm) < 4 {
		return
	}
	out, err := soundStart()
	if err != nil {
		keep()
		return
	}
	defer out.Stop()
	s.mu.Lock()
	s.last = n.seed
	s.next = s.prepare()
	s.mu.Unlock()
	feedSound(out, n.pcm, 0, stop)
}

// feedSound writes the piece from byte pos, round and round, about
// sndAhead ahead of the player, fading in at first. Once stop is closed
// it fades out, and gives where it got to.
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
// 16-bit stereo samples on its input.
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

// soundPiece is the piece for seed, music and office together, as 16-bit
// little-endian stereo samples (a second or two's work).
func soundPiece(seed uint64) []byte {
	mix, _, _ := sndRender(seed, false)
	return mix.pcm16()
}

// ---- choosing a piece ----

// The feels a piece can have.
const (
	sndSwing  = iota // brushed swing
	sndBossa         // bossa nova, straight eighths
	sndNeo           // a laid-back neo-soul groove
	sndBallad        // a slow ballad on brushes
	sndFeels
)

var sndFeelNames = [sndFeels]string{"swing", "bossa", "neo-soul", "ballad"}

// The instruments that play the chords or the tune.
const (
	sndRhodes = iota // an electric piano with a bell in its tone
	sndWurli         // an electric piano with a reed's bark
	sndPadI          // a soft pad of slow strings
	sndPluckI        // a muted guitar
	sndVibesI        // a vibraphone (the tune only)
	sndFluteI        // a breathy flute (the tune only)
	sndInstruments
)

var sndInstrumentNames = [sndInstruments]string{"Rhodes", "Wurli", "pad", "muted guitar", "vibes", "flute"}

// sndStyle is what a piece is made of, all drawn from its seed.
type sndStyle struct {
	feel  int     // sndSwing ...
	bpm   float64 // beats a minute: 80 to 100
	key   int     // the tonic: 0 C to 11 B
	minor bool    // a minor key (set by the progression)
	prog  int     // which of sndProgs
	bass  int     // which of sndGrooves
	keys  int     // the chords' instrument: sndRhodes to sndPluckI
	lead  int     // the tune's instrument
	swing float64 // how late off-beat eighths fall, in beats
}

// sndStyleOf is the style seed gives.
func sndStyleOf(seed uint64) sndStyle {
	rng := rand.New(rand.NewPCG(seed, 0x5359_3344))
	st := sndStyle{feel: rng.IntN(sndFeels)}
	tempo := [sndFeels][2]float64{{84, 100}, {86, 100}, {80, 92}, {80, 88}}[st.feel]
	st.bpm = math.Round(tempo[0] + (tempo[1]-tempo[0])*rng.Float64())
	st.key = rng.IntN(12)
	st.prog = rng.IntN(len(sndProgs))
	st.minor = sndProgs[st.prog].minor
	grooves := [sndFeels][]int{{0, 1, 3}, {2}, {4, 0}, {3, 0}}[st.feel]
	st.bass = grooves[rng.IntN(len(grooves))]
	st.keys = rng.IntN(4)
	if st.feel == sndBossa && rng.IntN(2) == 0 {
		st.keys = sndPluckI
	}
	leads := []int{sndRhodes, sndVibesI, sndPluckI, sndFluteI}
	st.lead = leads[rng.IntN(len(leads))]
	switch st.feel {
	case sndSwing:
		st.swing = 0.06 + 0.06*rng.Float64()
	case sndNeo:
		st.swing = 0.09 + 0.06*rng.Float64()
	case sndBallad:
		st.swing = 0.05 + 0.05*rng.Float64()
	}
	return st
}

var sndNoteNames = [12]string{"C", "Db", "D", "Eb", "E", "F", "F#", "G", "Ab", "A", "Bb", "B"}

// sndKeyName is a key's name, such as "Eb major".
func sndKeyName(key int, minor bool) string {
	if minor {
		return sndNoteNames[key%12] + " minor"
	}
	return sndNoteNames[key%12] + " major"
}

func (st sndStyle) String() string {
	return fmt.Sprintf("%s, %.0f bpm, %s, %s, chords on %s, tune on %s",
		sndFeelNames[st.feel], st.bpm, sndKeyName(st.key, st.minor), sndProgs[st.prog].name,
		sndInstrumentNames[st.keys], sndInstrumentNames[st.lead])
}

// sndFifths is how far apart two keys are round the circle of fifths
// (0 the same notes, 6 the furthest): a minor key counts as its relative major.
func sndFifths(a, b sndStyle) int {
	pos := func(st sndStyle) int {
		k := st.key
		if st.minor {
			k += 3
		}
		return k * 7 % 12
	}
	d := (pos(a) - pos(b) + 12) % 12
	return min(d, 12-d)
}

// sndUnlike is whether b is clearly unlike a: another tempo, a key with
// other notes, other chords, and another feel or chord instrument.
func sndUnlike(a, b sndStyle) bool {
	return math.Abs(a.bpm-b.bpm) >= 4 && sndFifths(a, b) >= 2 && a.prog != b.prog &&
		(a.feel != b.feel || a.keys != b.keys)
}

// sndNewSeed picks a random seed whose piece is unlike last's (0: no last).
func sndNewSeed(last uint64) uint64 {
	a := sndStyleOf(last)
	for {
		seed := rand.Uint64() | 1
		if seed != last && (last == 0 || sndUnlike(a, sndStyleOf(seed))) {
			return seed
		}
	}
}

// ---- chords ----

// sndQual is a kind of chord: its third, fifth and seventh over the root
// for the bass, and the notes over the root the chord instrument plays.
type sndQual struct {
	third, fifth, seventh int
	voice                 []int
}

var sndQuals = map[string]sndQual{
	"maj9":  {4, 7, 11, []int{4, 7, 11, 14}},
	"69":    {4, 7, 9, []int{4, 9, 14, 19}},
	"m9":    {3, 7, 10, []int{3, 7, 10, 14}},
	"m11":   {3, 7, 10, []int{3, 10, 14, 17}},
	"m6":    {3, 7, 9, []int{3, 7, 9, 14}},
	"9":     {4, 7, 10, []int{4, 7, 10, 14}},
	"13":    {4, 7, 10, []int{4, 10, 14, 21}},
	"13sus": {5, 7, 10, []int{5, 10, 14, 21}},
	"7b9":   {4, 7, 10, []int{4, 7, 10, 13}},
	"m7b5":  {3, 6, 10, []int{3, 6, 10, 17}},
}

// sndStep is a bar's chord in a progression: its root over the key's
// tonic (semitones) and its kind.
type sndStep struct {
	deg  int
	qual string
}

// sndProgs are the progressions, 8 bars each, played twice; the last bar
// of the piece leads back to the first.
var sndProgs = []struct {
	name  string
	minor bool
	bars  [8]sndStep
}{
	{"ii-V-I, then ii-V-i in the relative minor", false,
		[8]sndStep{{2, "m9"}, {7, "13"}, {0, "maj9"}, {0, "69"}, {11, "m7b5"}, {4, "7b9"}, {9, "m9"}, {9, "m9"}}},
	{"I-vi-ii-V turnarounds", false,
		[8]sndStep{{0, "maj9"}, {9, "m9"}, {2, "m11"}, {7, "13"}, {4, "m9"}, {9, "7b9"}, {2, "m9"}, {7, "13sus"}}},
	{"I-II7-ii-bII7, as in the bossa standards", false,
		[8]sndStep{{0, "maj9"}, {0, "maj9"}, {2, "9"}, {2, "9"}, {2, "m9"}, {1, "9"}, {0, "maj9"}, {0, "69"}}},
	{"neo-soul: I-IV-iii-VI7", false,
		[8]sndStep{{0, "maj9"}, {5, "maj9"}, {4, "m9"}, {9, "9"}, {2, "m9"}, {7, "13sus"}, {0, "maj9"}, {7, "13"}}},
	{"minor round the circle of fifths", true,
		[8]sndStep{{5, "m9"}, {10, "13"}, {3, "maj9"}, {8, "maj9"}, {2, "m7b5"}, {7, "7b9"}, {0, "m9"}, {0, "m9"}}},
	{"minor modal: i, iv, bVI, V", true,
		[8]sndStep{{0, "m9"}, {0, "m11"}, {5, "m9"}, {5, "m9"}, {8, "maj9"}, {7, "7b9"}, {0, "m9"}, {0, "m6"}}},
	{"dorian: i-IV7, then home through ii-V", true,
		[8]sndStep{{0, "m9"}, {5, "9"}, {0, "m9"}, {5, "9"}, {3, "maj9"}, {2, "m7b5"}, {7, "7b9"}, {0, "m11"}}},
}

// sndChord is a bar's chord: the bass's root and the notes played over it.
type sndChord struct {
	root                  int // the bass's root, a MIDI note low down
	third, fifth, seventh int // over the root
	keys                  []int
	tones                 []int // pitch classes the tune may use (0 to 11)
}

// sndVoice is the chord's notes for the chord instrument: of the ways to
// stack them, the one that moves least from the chord before.
func sndVoice(root int, iv, prev []int) []int {
	pcs := make([]int, len(iv))
	for i, v := range iv {
		pcs[i] = (root + v) % 12
	}
	slices.Sort(pcs)
	var best []int
	bestCost := math.MaxFloat64
	for r := range pcs {
		v := make([]int, len(pcs))
		n := 50 + (pcs[r]-50%12+12)%12 // the lowest note: from 50 up
		for i := range pcs {
			p := pcs[(r+i)%len(pcs)]
			if i > 0 {
				n = v[i-1] + 1 + (p-(v[i-1]+1)%12+12)%12
			}
			v[i] = n
		}
		cost := 0.0
		if prev == nil {
			mid := 0.0
			for _, n := range v {
				mid += float64(n)
			}
			cost = math.Abs(mid/float64(len(v)) - 60)
		} else {
			for i := range v {
				cost += math.Abs(float64(v[i] - prev[i]))
			}
		}
		if v[len(v)-1] > 76 {
			cost += 100
		}
		if cost < bestCost {
			best, bestCost = v, cost
		}
	}
	return best
}

// ---- a piece ----

// sndPiece is a piece being made: its style, its timing and its chords.
type sndPiece struct {
	sndStyle
	seed   uint64
	beat   float64 // samples a beat
	bar    float64 // samples a bar
	frames int     // the piece's length, samples a channel
	chords [sndBars]sndChord
}

func newSndPiece(seed uint64) *sndPiece {
	p := &sndPiece{sndStyle: sndStyleOf(seed), seed: seed}
	p.beat = sndRate * 60 / p.bpm
	p.bar = 4 * p.beat
	p.frames = int(math.Round(sndBars * p.bar))
	prog := sndProgs[p.prog].bars
	var prev []int
	for b := range sndBars {
		st := prog[b%8]
		if b == sndBars-1 && !sndDominant(st.qual) { // back to the start
			st = sndStep{prog[0].deg + 7, "7b9"}
		}
		q := sndQuals[st.qual]
		pc := (p.key + st.deg) % 12
		c := sndChord{root: 28 + (pc-28%12+12)%12, third: q.third, fifth: q.fifth, seventh: q.seventh}
		c.keys = sndVoice(pc, q.voice, prev)
		prev = c.keys
		c.tones = []int{pc}
		for _, v := range q.voice {
			c.tones = append(c.tones, (pc+v)%12)
		}
		p.chords[b] = c
	}
	return p
}

func sndDominant(q string) bool { return q == "9" || q == "13" || q == "13sus" || q == "7b9" }

// rng is the piece's random generator for one part.
func (p *sndPiece) rng(part uint64) *rand.Rand { return rand.New(rand.NewPCG(p.seed, part)) }

// at is the sample beat beats into bar falls on: off-beat eighths swing late.
func (p *sndPiece) at(bar int, beat float64) int {
	if f := beat - math.Floor(beat); math.Abs(f-0.5) < 1e-9 {
		beat += p.swing
	}
	return int(float64(bar)*p.bar + beat*p.beat)
}

// sndRender makes seed's piece: each part made on its own, set to its
// level (by its loudness, or by its loudest moment for the ones that come
// and go), and added to the music or the office; then the whole set to
// peak at sndPeak. With split the music and the office come back too, on
// their own at the same levels (to look at); without, they are empty.
func sndRender(seed uint64, split bool) (mix, music, office sndStereo) {
	p := newSndPiece(seed)
	music, office = newSndStereo(p.frames), newSndStereo(p.frames)
	part := newSndStereo(p.frames)
	brush := 0.12
	if p.feel == sndBallad {
		brush = 0.22
	}
	for _, pt := range []struct {
		make   func(sndStereo)
		byPeak bool
		level  float64
		office bool
	}{
		{p.bassPart, false, 1, false},
		{p.compPart, false, 0.42, false},
		{p.tunePart, false, 0.3, false},
		{p.kickPart, true, 0.9, false},
		{p.snarePart, true, 0.3, false},
		{p.shakerPart, true, 0.2, false},
		{p.brushPart, true, brush, false},
		{p.hvacPart, false, 0.06, true},
		{p.voicesPart, false, 0.04, true},
		{p.typingPart, true, 0.2, true},
		{p.paperPart, true, 0.09, true},
		{p.mugPart, true, 0.09, true},
		{p.creakPart, true, 0.05, true},
		{p.stepsPart, true, 0.1, true},
	} {
		part.zero()
		pt.make(part)
		g := part.rms()
		if pt.byPeak {
			g = part.peak()
		}
		if g == 0 {
			continue
		}
		g = pt.level / g
		to := music
		if pt.office {
			to = office
		}
		for i := range to.l {
			to.l[i] += part.l[i] * float32(g)
			to.r[i] += part.r[i] * float32(g)
		}
	}
	mix = part
	for i := range mix.l {
		mix.l[i] = music.l[i] + office.l[i]
		mix.r[i] = music.r[i] + office.r[i]
	}
	g := float32(sndPeak / mix.peak())
	for _, s := range []sndStereo{mix, music, office} {
		for i := range s.l {
			s.l[i] *= g
			s.r[i] *= g
		}
	}
	if !split {
		return mix, sndStereo{}, sndStereo{}
	}
	return mix, music, office
}

// ---- sound as samples ----

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

// add adds v at sample i, which runs round the piece's end to its start.
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

// sndNoise is white noise, -1 to 1.
func sndNoise(rng *rand.Rand) float64 { return 2*rng.Float64() - 1 }

// sndHz is a note's pitch (MIDI numbers: 69 is A 440).
func sndHz(note int) float64 { return 440 * math.Pow(2, float64(note-69)/12) }

// sndBiquad is a two-pole filter (the audio EQ cookbook's).
type sndBiquad struct{ b0, b1, b2, a1, a2, x1, x2, y1, y2 float64 }

// sndFilter is a new biquad: 'l' low pass, 'h' high pass, 'b' band pass
// (its peak at 0 dB).
func sndFilter(kind byte, f, q float64) *sndBiquad {
	b := &sndBiquad{}
	b.set(kind, f, q)
	return b
}

func (b *sndBiquad) set(kind byte, f, q float64) {
	w := 2 * math.Pi * min(f, 0.45*sndRate) / sndRate
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

// sndPink makes pink noise (as loud in every octave) from white (Paul
// Kellet's filter).
type sndPink struct{ b0, b1, b2, b3, b4, b5, b6 float64 }

func (p *sndPink) step(w float64) float64 {
	p.b0 = 0.99886*p.b0 + w*0.0555179
	p.b1 = 0.99332*p.b1 + w*0.0750759
	p.b2 = 0.96900*p.b2 + w*0.1538520
	p.b3 = 0.86650*p.b3 + w*0.3104856
	p.b4 = 0.55000*p.b4 + w*0.5329522
	p.b5 = -0.7616*p.b5 - w*0.0168980
	out := p.b0 + p.b1 + p.b2 + p.b3 + p.b4 + p.b5 + p.b6 + w*0.5362
	p.b6 = w * 0.115926
	return out * 0.11
}

// sndBed fills s with a steady sound, next giving a sample at a time (i is
// where in the piece it is). It runs a little past the end and fades that
// over the start, so there is no seam where the piece comes round.
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

// whole is a slow wobble's rate (Hz) rounded to go a whole number of times
// round the piece, so the piece joins up; as cycles a sample.
func (p *sndPiece) whole(hz float64) float64 {
	return math.Max(1, math.Round(hz*float64(p.frames)/sndRate)) / float64(p.frames)
}

// ---- the band ----

// sndBassNote is a note of a bass groove: its beat, which note of the chord
// (R root, 3, 5, 7, 8 the octave, A a step from the next bar's root, N the
// next bar's root early), how many beats it lasts and how hard it is played.
type sndBassNote struct {
	beat     float64
	deg      byte
	dur, vel float64
}

// sndGrooves are the bass's grooves, each a few bar patterns; the first of
// each is played most.
var sndGrooves = [][][]sndBassNote{
	{ // syncopated
		{{0, 'R', 1.3, 1}, {1.5, '5', 0.4, 0.7}, {2.5, 'R', 0.4, 0.85}, {3, '8', 0.35, 0.6}, {3.5, 'A', 0.45, 0.75}},
		{{0, 'R', 1.6, 1}, {1.75, 'R', 0.2, 0.5}, {2.5, '5', 0.9, 0.85}, {3.5, '7', 0.45, 0.7}},
	},
	{ // walking
		{{0, 'R', 0.9, 1}, {1, '3', 0.9, 0.8}, {2, '5', 0.9, 0.85}, {3, 'A', 0.9, 0.8}},
		{{0, 'R', 0.9, 1}, {1, '5', 0.9, 0.8}, {2, '8', 0.9, 0.8}, {3, 'A', 0.9, 0.8}},
		{{0, 'R', 0.9, 1}, {1, '3', 0.9, 0.8}, {2, '5', 0.9, 0.85}, {3, '7', 0.9, 0.8}},
	},
	{ // bossa
		{{0, 'R', 1.4, 1}, {1.5, '5', 0.45, 0.7}, {2, '5', 1.4, 0.85}, {3.5, 'N', 0.45, 0.7}},
		{{0, 'R', 1.4, 1}, {1.5, '5', 0.45, 0.7}, {2, '5', 0.9, 0.85}, {3, '8', 0.45, 0.6}, {3.5, 'N', 0.45, 0.7}},
	},
	{ // two to a bar
		{{0, 'R', 1.9, 1}, {2, '5', 1.4, 0.85}, {3.5, 'A', 0.45, 0.7}},
		{{0, 'R', 1.9, 1}, {2, '5', 1.9, 0.85}},
	},
	{ // neo-soul
		{{0, 'R', 0.7, 1}, {0.75, 'R', 0.2, 0.5}, {1.5, '8', 0.3, 0.6}, {2.5, '5', 0.8, 0.85}, {3.5, 'A', 0.4, 0.7}},
		{{0, 'R', 1.6, 1}, {1.75, 'R', 0.2, 0.5}, {2.5, '5', 0.9, 0.85}, {3.5, '7', 0.45, 0.7}},
	},
}

// bassPart is the bass: a sine with a little of its overtones, softly
// saturated, deep (41 to 117 Hz roots), playing its groove round the chords.
func (p *sndPiece) bassPart(s sndStereo) {
	rng := p.rng(1)
	groove := sndGrooves[p.bass]
	pluck := 0.15 + 0.3*rng.Float64() // how much of a finger's snap it has
	for bar := range sndBars {
		c, next := p.chords[bar], p.chords[(bar+1)%sndBars]
		pat := groove[0]
		if rng.Float64() > 0.6 {
			pat = groove[rng.IntN(len(groove))]
		}
		for _, b := range pat {
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
			case 'N':
				n = next.root
			case 'A':
				n = next.root + 1
				if rng.IntN(2) == 0 || n > 45 {
					n = next.root - 1
				}
			}
			jitter := int((rng.Float64() - 0.5) * 0.006 * sndRate)
			bassTone(s, p.at(bar, b.beat)+jitter, sndHz(n), b.dur*p.beat, b.vel*(0.92+0.08*rng.Float64()), pluck)
		}
	}
}

// bassTone is one bass note: f Hz, dur samples long, then let go.
func bassTone(s sndStereo, start int, f, dur, vel, pluckAmt float64) {
	w := 2 * math.Pi * f / sndRate
	n := int(dur) + sndRate/5
	sat := math.Tanh(1.6)
	for i := range n {
		t := float64(i) / sndRate
		ph := w * float64(i)
		pluck := pluckAmt * math.Exp(-t/0.08)
		v := math.Sin(ph) + (0.2+pluck)*math.Sin(2*ph) + 0.06*math.Sin(3*ph)
		v = math.Tanh(1.6*v) / sat
		e := min(t/0.006, 1) * (0.6 + 0.4*math.Exp(-t/0.25))
		if float64(i) > dur {
			e *= math.Exp(-(float64(i) - dur) / sndRate / 0.03)
		}
		s.add(start+i, v*e*vel, sndMid)
	}
}

// sndNoteFn plays one note: f Hz from sample start, held dur samples.
type sndNoteFn func(s sndStereo, start int, f, dur, vel float64, g sndPanG, rng *rand.Rand)

var sndPlayers = [sndInstruments]sndNoteFn{sndEPiano, sndWurliNote, sndPad, sndPluck, sndVibes, sndFlute}

// sndKeyHit is a chord played: when (beats into the bar), for how many beats, how hard.
type sndKeyHit struct{ beat, dur, vel float64 }

var keyPatterns = [][]sndKeyHit{
	{{0, 2.3, 0.7}, {2.5, 1.3, 0.55}},
	{{0, 1.3, 0.65}, {1.5, 2.2, 0.6}},
	{{0, 3.7, 0.7}},
	{{0, 0.4, 0.6}, {0.5, 1.4, 0.55}, {2.5, 1.3, 0.5}},
}

// guitarPatterns are a muted guitar's comping, short chords on and off the beat.
var guitarPatterns = [][]sndKeyHit{
	{{0, 0.4, 0.7}, {1.5, 0.4, 0.55}, {2.5, 0.4, 0.6}, {3.5, 0.3, 0.5}},
	{{0, 0.6, 0.7}, {1, 0.3, 0.45}, {1.5, 0.4, 0.55}, {3, 0.4, 0.6}},
	{{0.5, 0.4, 0.6}, {1.5, 0.4, 0.55}, {2.5, 0.4, 0.6}, {3.5, 0.4, 0.5}},
}

// compPart is the chords: an electric piano a little strummed, a pad held
// through each bar or a muted guitar strummed short; then each moved
// about as such an instrument is (the Rhodes from side to side, the
// Wurli's tremolo, the pad slowly drifting).
func (p *sndPiece) compPart(s sndStereo) {
	rng := p.rng(2)
	play := sndPlayers[p.keys]
	for bar := range sndBars {
		c := p.chords[bar]
		switch p.keys {
		case sndPadI:
			for k, n := range c.keys {
				play(s, p.at(bar, 0)+k*sndRate/200, sndHz(n), 4*p.beat, 0.6, sndPan(0.5*float64(2*(k%2)-1)), rng)
			}
		case sndPluckI:
			for _, h := range guitarPatterns[rng.IntN(len(guitarPatterns))] {
				start := p.at(bar, h.beat) + int((rng.Float64()-0.5)*0.008*sndRate)
				up := h.beat-math.Floor(h.beat) > 0.25 && rng.IntN(2) == 0 // an up-strum on some off-beats
				for k := range c.keys {
					n := c.keys[k]
					if up {
						n = c.keys[len(c.keys)-1-k]
					}
					play(s, start+k*int((0.008+0.006*rng.Float64())*sndRate), sndHz(n), h.dur*p.beat, h.vel*(0.85+0.15*rng.Float64()), sndPan(0.3), rng)
				}
			}
		default:
			for _, h := range keyPatterns[rng.IntN(len(keyPatterns))] {
				start := p.at(bar, h.beat) + int((rng.Float64()-0.5)*0.008*sndRate)
				for k, n := range c.keys {
					strum := k * int((0.006+0.008*rng.Float64())*sndRate)
					play(s, start+strum, sndHz(n), h.dur*p.beat, h.vel*(0.85+0.15*rng.Float64()), sndMid, rng)
				}
			}
		}
	}
	switch p.keys {
	case sndRhodes, sndPadI: // from side to side
		depth, k := 0.25, p.whole(3.6)
		if p.keys == sndPadI {
			depth, k = 0.2, p.whole(0.07)
		}
		for i := range s.l {
			g := depth * math.Sin(2*math.Pi*k*float64(i))
			s.l[i] *= float32(1 + g)
			s.r[i] *= float32(1 - g)
		}
	case sndWurli: // its tremolo, both sides together
		k := p.whole(5.5)
		for i := range s.l {
			g := float32(1 - 0.18*(1+math.Sin(2*math.Pi*k*float64(i))))
			s.l[i] *= g
			s.r[i] *= g
		}
	}
}

// sndRhythms are where a phrase's notes fall in its bar (beats).
var sndRhythms = [][]float64{
	{0.5, 1, 1.5, 2.5},
	{0, 0.5, 1, 2},
	{1.5, 2, 2.5, 3, 3.5},
	{0, 0.75, 1.5, 3},
	{0.5, 1.5, 2, 2.5, 3.5},
	{0, 1, 1.5, 2},
	{2, 2.5, 3},
}

// tunePart is the tune: in most four-bar phrases a short motif, played
// over the second bar's chord and again, a little changed, over the
// third's, and sometimes a run up the chord in the fourth. The motif is
// steps through the chord's notes, so it fits each chord it meets.
func (p *sndPiece) tunePart(s sndStereo) {
	rng := p.rng(20)
	play := sndPlayers[p.lead]
	g := sndPan(0.5 * (2*rng.Float64() - 1))
	centre := 70 + rng.IntN(6)
	if p.lead == sndPluckI {
		centre -= 5
	}
	var rhythm []float64
	var steps []int
	newMotif := func() {
		rhythm = sndRhythms[rng.IntN(len(sndRhythms))]
		steps = make([]int, len(rhythm))
		k := rng.IntN(3) - 1
		for i := range steps {
			steps[i] = k
			k += []int{-2, -1, -1, 1, 1, 2}[rng.IntN(6)]
			k = min(max(k, -4), 4)
		}
	}
	newMotif()
	phrase := func(bar int, rh []float64, st []int) {
		c := p.chords[bar]
		for i, b := range rh {
			dur := 0.8
			if i+1 < len(rh) {
				dur = 0.9 * (rh[i+1] - b)
			}
			n := sndTuneNote(c, centre, st[i])
			play(s, p.at(bar, b), sndHz(n), dur*p.beat, (0.55+0.25*rng.Float64())*[]float64{1, 0.8}[i%2], g, rng)
		}
	}
	for ph := 0; ph < sndBars/4; ph++ {
		if rng.Float64() < 0.2 {
			continue // a phrase left to the chords
		}
		if rng.Float64() < 0.4 {
			newMotif()
		}
		phrase(ph*4+1, rhythm, steps)
		changed := append([]int(nil), steps...)
		changed[len(changed)-1] += rng.IntN(3) - 1
		phrase(ph*4+2, rhythm, changed)
		if rng.Float64() < 0.5 { // a run up the chord
			from := 1 + float64(rng.IntN(3))*0.5
			var rh []float64
			var st []int
			for b, k := from, rng.IntN(2)-2; b < 4; b, k = b+0.5, k+1 {
				rh, st = append(rh, b), append(st, k)
			}
			phrase(ph*4+3, rh, st)
		}
	}
}

// sndTuneNote is the note step chord notes from the one nearest centre.
func sndTuneNote(c sndChord, centre, step int) int {
	var notes []int
	at := 0
	for n := centre - 14; n <= centre+16; n++ {
		for _, t := range c.tones {
			if n%12 == t {
				if n <= centre {
					at = len(notes)
				}
				notes = append(notes, n)
				break
			}
		}
	}
	return notes[min(max(at+step, 0), len(notes)-1)]
}

// sndEPiano is one electric-piano note (a soft FM tone with a short bell at
// its start): f Hz, held dur samples, then let go.
func sndEPiano(s sndStereo, start int, f, dur, vel float64, g sndPanG, _ *rand.Rand) {
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
		s.add(start+i, 0.3*v*e*vel, g)
	}
}

// sndWurliNote is a reedier electric piano: a brighter bark at the start,
// a little clipped, dying away sooner.
func sndWurliNote(s sndStereo, start int, f, dur, vel float64, g sndPanG, _ *rand.Rand) {
	w := 2 * math.Pi * f / sndRate
	n := int(dur) + sndRate/2
	tau := 1.1 * math.Pow(220/f, 0.3)
	sat := math.Tanh(1.5)
	for i := range n {
		t := float64(i) / sndRate
		ph := w * float64(i)
		index := 0.5 + 1.4*vel*math.Exp(-t/0.12)
		v := math.Sin(ph + index*math.Sin(ph))
		v = math.Tanh(1.5*v)/sat + 0.15*v*v // the reed: odd overtones, and a little even
		e := min(t/0.003, 1) * math.Exp(-t/tau)
		if float64(i) > dur {
			e *= math.Exp(-(float64(i) - dur) / sndRate / 0.08)
		}
		s.add(start+i, 0.25*v*e*vel, g)
	}
}

// sndBlep smooths a sawtooth's jump (t its phase, dt its step), so it has
// no harsh edges.
func sndBlep(t, dt float64) float64 {
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

// sndPad is one pad note: three sawtooths a little out of tune with each
// other, softened, swelling in and dying slowly.
func sndPad(s sndStereo, start int, f, dur, vel float64, g sndPanG, rng *rand.Rand) {
	n := int(dur) + sndRate
	lp := sndFilter('l', min(2.5*f+500, 3000), 0.6)
	var ph [3]float64
	for k := range ph {
		ph[k] = rng.Float64()
	}
	det := [3]float64{1, 1.0035, 0.9968}
	for i := range n {
		t := float64(i) / sndRate
		v := 0.0
		for k := range ph {
			dt := f * det[k] / sndRate
			v += 2*ph[k] - 1 - sndBlep(ph[k], dt)
			if ph[k] += dt; ph[k] >= 1 {
				ph[k]--
			}
		}
		x := min(t/0.5, 1)
		e := x * x * (3 - 2*x)
		if float64(i) > dur {
			e *= math.Exp(-(float64(i) - dur) / sndRate / 0.3)
		}
		s.add(start+i, 0.12*lp.step(v)*e*vel, g)
	}
}

// sndPluck is a muted guitar string (a plucked string as a short delay
// fed back on itself, a little duller each time round), damped soon.
func sndPluck(s sndStereo, start int, f, dur, vel float64, g sndPanG, rng *rand.Rand) {
	period := sndRate / f
	l := max(int(period-0.5), 2)
	line := make([]float64, l)
	lp := 0.0
	for i := range line { // the pick: a short burst of soft noise
		lp += (sndNoise(rng) - lp) * (0.3 + 0.4*vel)
		line[i] = lp
	}
	n := int(dur) + sndRate*15/100
	pos, dc, damp := 0, 0.0, 0.0
	for i := range n {
		tau := 0.35 // the palm on the strings
		if float64(i) > dur {
			tau = 0.05
		}
		loss := math.Exp(-period / sndRate / tau)
		a, b := line[pos], line[(pos+1)%l]
		v := loss * 0.5 * (a + b)
		damp += (v - damp) * 0.6 // duller still
		line[pos] = damp
		pos = (pos + 1) % l
		dc += (a - dc) * 0.002 // no creep
		s.add(start+i, 0.5*(a-dc)*vel, g)
	}
}

// sndVibes is a vibraphone note: a soft mallet on a bar, its overtones
// fading fast, the fan's slow tremolo, damped when let go.
func sndVibes(s sndStereo, start int, f, dur, vel float64, g sndPanG, _ *rand.Rand) {
	n := int(dur) + sndRate/4
	for i := range n {
		t := float64(i) / sndRate
		ph := 2 * math.Pi * f * t
		v := math.Sin(ph)*math.Exp(-t/1.4) + 0.25*math.Sin(4*ph)*math.Exp(-t/0.25) + 0.06*math.Sin(10*ph)*math.Exp(-t/0.04)
		e := min(t/0.001, 1) * (1 - 0.25*(1-math.Cos(2*math.Pi*5*t))/2)
		if float64(i) > dur {
			e *= math.Exp(-(float64(i) - dur) / sndRate / 0.12)
		}
		s.add(start+i, 0.3*v*e*vel, g)
	}
}

// sndFlute is a breathy flute note: a soft tone with breath in it and a
// gentle vibrato once it is held.
func sndFlute(s sndStereo, start int, f, dur, vel float64, g sndPanG, rng *rand.Rand) {
	n := int(dur) + sndRate/10
	breath := sndFilter('b', 2*f, 1)
	ph := 0.0
	for i := range n {
		t := float64(i) / sndRate
		vib := 1 + 0.004*min(max(t-0.25, 0)/0.3, 1)*math.Sin(2*math.Pi*5*t)
		ph += 2 * math.Pi * f * vib / sndRate
		v := math.Sin(ph) + 0.22*math.Sin(2*ph) + 0.06*math.Sin(3*ph) + 0.5*breath.step(sndNoise(rng))
		e := min(t/0.06, 1)
		if float64(i) > dur {
			e *= math.Exp(-(float64(i) - dur) / sndRate / 0.06)
		}
		s.add(start+i, 0.25*v*e*vel, g)
	}
}

// ---- the drums ----

// kickPart is a soft kick, where the feel puts it.
func (p *sndPiece) kickPart(s sndStereo) {
	rng := p.rng(3)
	for bar := range sndBars {
		hits := [][2]float64{{0, 1}, {2.5, 0.8}}
		switch p.feel {
		case sndSwing:
			if rng.Float64() < 0.3 {
				hits = append(hits, [2]float64{1.75, 0.45})
			}
		case sndBossa:
			hits = [][2]float64{{0, 0.9}, {1.5, 0.5}, {2, 0.85}, {3.5, 0.5}}
		case sndNeo:
			if rng.Float64() < 0.3 {
				hits = append(hits, [2]float64{0.75, 0.5})
			}
			if rng.Float64() < 0.4 {
				hits = append(hits, [2]float64{1.75, 0.55})
			}
		case sndBallad:
			hits = [][2]float64{{0, 0.8}}
			if rng.Float64() < 0.5 {
				hits = append(hits, [2]float64{2.5, 0.5})
			}
		}
		for _, h := range hits {
			sndKick(s, p.at(bar, h[0]), h[1])
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

// snarePart is the rim or the snare: a rim click on two and four (swing),
// a cross-stick playing the bossa's clave, or a soft, late snare (neo-soul).
func (p *sndPiece) snarePart(s sndStereo) {
	rng := p.rng(4)
	g := sndPan(-0.25)
	jit := func() int { return int((rng.Float64() - 0.5) * 0.006 * sndRate) }
	for bar := range sndBars {
		switch p.feel {
		case sndSwing:
			for _, b := range []float64{1, 3} {
				sndRim(s, p.at(bar, b)+jit(), 0.85+0.15*rng.Float64(), g, rng)
			}
			if bar%8 == 7 {
				for _, b := range []float64{3.25, 3.5, 3.75} {
					sndRim(s, p.at(bar, b), 0.35, g, rng)
				}
			}
		case sndBossa:
			beats := []float64{0, 1.5, 3}
			if bar%2 == 1 {
				beats = []float64{1, 2.5}
			}
			for _, b := range beats {
				sndRim(s, p.at(bar, b)+jit(), 0.7+0.2*rng.Float64(), g, rng)
			}
		case sndNeo:
			late := int(0.025 * sndRate)
			for _, b := range []float64{1, 3} {
				sndSnare(s, p.at(bar, b)+late+jit(), 0.85+0.15*rng.Float64(), g, rng)
			}
			if rng.Float64() < 0.25 {
				sndSnare(s, p.at(bar, 3.75), 0.25, g, rng)
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

// sndSnare is a soft snare: the snares' rattle and a short knock of the drum.
func sndSnare(s sndStereo, start int, vel float64, g sndPanG, rng *rand.Rand) {
	bp, body := sndFilter('b', 2200, 0.6), sndFilter('l', 400, 0.7)
	for i := range sndRate / 5 {
		t := float64(i) / sndRate
		w := sndNoise(rng)
		v := bp.step(w)*math.Exp(-t/0.07) + 2.5*body.step(w)*math.Exp(-t/0.02)
		s.add(start+i, v*vel*min(t/0.001, 1), g)
	}
}

// shakerPart is a shaker on the eighths (swing), on the sixteenths (bossa),
// a closed hi-hat on the eighths (neo-soul) or on the beats (ballad).
func (p *sndPiece) shakerPart(s sndStereo) {
	rng := p.rng(5)
	g := sndPan(0.35)
	for bar := range sndBars {
		var hits [][2]float64
		switch p.feel {
		case sndBossa:
			for e := range 16 {
				hits = append(hits, [2]float64{float64(e) / 4, []float64{0.8, 0.4, 0.55, 0.4}[e%4]})
			}
		case sndBallad:
			for b := range 4 {
				hits = append(hits, [2]float64{float64(b), 0.5})
			}
		default:
			for e := range 8 {
				hits = append(hits, [2]float64{float64(e) / 2, []float64{0.5, 0.85}[e%2]})
			}
		}
		for _, h := range hits {
			at := p.at(bar, h[0]) + int((rng.Float64()-0.5)*0.006*sndRate)
			vel := h[1] * (0.85 + 0.15*rng.Float64())
			if p.feel == sndNeo {
				sndShake(s, at, vel, 0.012, 8000, g, rng)
			} else {
				sndShake(s, at, vel, 0.03, 6500, g, rng)
			}
		}
	}
}

func sndShake(s sndStereo, start int, vel, decay, cut float64, g sndPanG, rng *rand.Rand) {
	hp := sndFilter('h', cut, 0.7)
	for i := range sndRate * 14 / 100 {
		t := float64(i) / sndRate
		e := min(t/0.004, 1) * math.Exp(-max(t-0.004, 0)/decay)
		s.add(start+i, vel*e*hp.step(sndNoise(rng)), g)
	}
}

// brushPart is a brush's swish on two and four (swing and ballad; the
// ballad's brush stirs on every beat too).
func (p *sndPiece) brushPart(s sndStereo) {
	if p.feel != sndSwing && p.feel != sndBallad {
		return
	}
	rng := p.rng(6)
	g := sndPan(-0.1)
	for bar := range sndBars {
		beats := [][2]float64{{1, 1}, {3, 1}}
		if p.feel == sndBallad {
			beats = append(beats, [2]float64{0, 0.45}, [2]float64{2, 0.45})
		}
		for _, b := range beats {
			bp := sndFilter('b', 3500, 0.6)
			start := p.at(bar, b[0]) - sndRate/50
			for i := range sndRate * 4 / 10 {
				t := float64(i) / sndRate
				e := math.Pow(math.Sin(math.Min(t/0.04, 1)*math.Pi/2), 2) * math.Exp(-max(t-0.04, 0)/0.1)
				s.add(start+i, b[1]*e*bp.step(sndNoise(rng)), g)
			}
		}
	}
}

// ---- the office ----

// hvacPart is the air conditioning: broad hiss (pink noise, a little
// duller at the top), a low rumble under it, each side its own, slowly
// rising and falling a little. No filter here rings, so nothing whistles.
func (p *sndPiece) hvacPart(s sndStereo) {
	rng := p.rng(30)
	var pinkL, pinkR sndPink
	lpL, lpR := sndFilter('l', 3200, 0.5), sndFilter('l', 2900, 0.5)
	hpL, hpR := sndFilter('h', 35, 0.5), sndFilter('h', 35, 0.5)
	rum1, rum2 := sndFilter('l', 120, 0.5), sndFilter('l', 120, 0.5)
	drift, driftTo := 1.0, 1.0
	sndBed(s, func(i int) (float64, float64) {
		if i%(sndRate/4) == 0 {
			driftTo = 0.85 + 0.3*rng.Float64()
		}
		drift += (driftTo - drift) * 0.00003
		low := 9 * rum2.step(rum1.step(sndNoise(rng)))
		l := hpL.step(lpL.step(pinkL.step(sndNoise(rng))) + low)
		r := hpR.step(lpR.step(pinkR.step(sndNoise(rng))) + low)
		return l * drift, r * drift
	})
}

// sndVowels are vowels' first three formants (Hz, a man's voice).
var sndVowels = [][3]float64{
	{730, 1090, 2440}, {530, 1840, 2480}, {270, 2290, 3010}, {570, 840, 2410},
	{300, 870, 2240}, {500, 1500, 2500}, {660, 1720, 2410}, {440, 1020, 2240},
}

// sndTalker is one voice further off. It talks in syllables of a fifth of
// a second or so, each with its own vowel and loudness; between them it
// glides, never jumps. Its pitch falls through each phrase, as speech
// does, rises on stressed syllables and wavers a little all the time.
type sndTalker struct {
	g          sndPanG
	base, size float64 // the voice's usual pitch (Hz), and its formants' scale
	on         bool
	phrase, at int // the phrase's length and how far into it (samples)
	syl, sylN  int // samples into the syllable, and its length
	tick       int // samples, to move the formants every 32
	cons       int // samples of consonant at the syllable's start
	amp, env   float64
	f0, f0To   float64
	tone       float64 // the phrase's pitch: each phrase a little higher or lower
	jit, shim  float64 // this vocal-fold period's waver in pitch and loudness
	ph         float64
	f, fTo     [3]float64
	bp         [3]*sndBiquad
	hiss       *sndBiquad
}

func newSndTalker(rng *rand.Rand, female bool, pan float64) *sndTalker {
	k := &sndTalker{g: sndPan(pan), base: 95 + 45*rng.Float64(), size: 1}
	if female {
		k.base, k.size = 175+60*rng.Float64(), 1.15+0.05*rng.Float64()
	}
	k.f0, k.f0To, k.tone, k.jit, k.shim = k.base, k.base, k.base, 1, 1
	k.f, k.fTo = sndVowels[5], sndVowels[5]
	for j := range k.bp {
		k.bp[j] = sndFilter('b', k.f[j], 4)
	}
	k.hiss = sndFilter('h', 2500, 0.7)
	return k
}

// say starts a phrase n samples long.
func (k *sndTalker) say(n int, rng *rand.Rand) {
	k.on, k.phrase, k.at, k.syl, k.sylN = true, n, 0, 0, 0
	k.tone = k.base * (0.85 + 0.3*rng.Float64())
}

func (k *sndTalker) newSyllable(rng *rand.Rand) {
	k.syl, k.sylN = 0, int((0.13+0.17*rng.Float64())*sndRate) // 3 to 8 a second
	k.amp = 0.35 + 0.45*rng.Float64()
	k.cons = 0
	if rng.Float64() < 0.55 {
		k.cons = int((0.015 + 0.04*rng.Float64()) * sndRate)
	}
	fall := 1.12 - 0.27*float64(k.at)/float64(max(k.phrase, 1)) // the phrase's pitch falls
	k.f0To = k.tone * fall * (0.86 + 0.28*rng.Float64())
	switch r := rng.Float64(); {
	case r < 0.22: // stressed
		k.amp, k.f0To = 1, k.f0To*1.12
	case r < 0.32: // a gap between words
		k.amp, k.cons = 0.02, 0
	}
	v := sndVowels[rng.IntN(len(sndVowels))]
	for j := range k.fTo {
		k.fTo[j] = v[j] * k.size
	}
}

func (k *sndTalker) next(rng *rand.Rand) float64 {
	target := 0.0
	if k.on {
		if k.syl >= k.sylN {
			k.newSyllable(rng)
		}
		target = k.amp * math.Sin(math.Pi*float64(k.syl)/float64(k.sylN))
		k.syl++
		if k.at++; k.at >= k.phrase {
			k.on = false
		}
	}
	k.env += (target - k.env) * 0.003
	if k.tick++; k.tick%32 == 0 {
		for j := range k.f {
			k.f[j] += (k.fTo[j] - k.f[j]) * 0.04
			k.bp[j].set('b', k.f[j], k.f[j]/[3]float64{110, 150, 220}[j])
		}
	}
	k.f0 += (k.f0To - k.f0) * 0.0006
	if k.ph += k.f0 * k.jit / sndRate; k.ph >= 1 {
		k.ph--
		k.jit, k.shim = 1+0.025*sndNoise(rng), 1+0.15*sndNoise(rng)
	}
	w := sndNoise(rng)
	if k.syl < k.cons { // a consonant: breath only
		return 0.5 * k.hiss.step(w) * k.env
	}
	src := 0.8*(1-2*k.ph)*k.shim + 0.35*w
	return (k.bp[0].step(src) + 0.5*k.bp[1].step(src) + 0.25*k.bp[2].step(src)) * k.env
}

// sndVerb is a room's echo: eight delay lines (37 to 79 ms) feeding each
// other through a mixing matrix, each time round a little quieter and
// duller, so it blurs without ringing.
type sndVerb struct {
	lines [8][]float64
	pos   [8]int
	loss  [8]float64
	lp    [8]float64
	damp  float64
}

func newSndVerb(rt60, damp float64) *sndVerb {
	v := &sndVerb{damp: damp}
	for i, ms := range []float64{37.1, 41.3, 47.9, 53.3, 59.7, 66.1, 71.9, 79.3} {
		n := int(ms * sndRate / 1000)
		v.lines[i] = make([]float64, n)
		v.loss[i] = math.Pow(10, -3*float64(n)/(rt60*sndRate))
	}
	return v
}

func (v *sndVerb) step(l, r float64) (float64, float64) {
	var o [8]float64
	sum := 0.0
	for i := range o {
		o[i] = v.lines[i][v.pos[i]]
		sum += o[i]
	}
	sum *= 2.0 / 8
	for i := range o {
		x := (o[i] - sum) * v.loss[i]
		v.lp[i] += (x - v.lp[i]) * v.damp
		in := l
		if i%2 == 1 {
			in = r
		}
		v.lines[i][v.pos[i]] = v.lp[i] + in
		v.pos[i] = (v.pos[i] + 1) % len(v.lines[i])
	}
	return (o[0] + o[2] + o[4] + o[6]) / 4, (o[1] + o[3] + o[5] + o[7]) / 4
}

// voicesPart is people talking further off: three pairs, each a
// conversation in turns (now and then both quiet), muffled by the
// distance and heard mostly through the room's echo.
func (p *sndPiece) voicesPart(s sndStereo) {
	rng := p.rng(31)
	type pair struct {
		k     [2]*sndTalker
		who   int
		pause int // samples until the next turn
	}
	var pairs []*pair
	for i, pan := range []float64{-0.6, 0.1, 0.65} {
		c := &pair{who: rng.IntN(2), pause: int(rng.Float64() * 3 * sndRate)}
		c.k[0] = newSndTalker(rng, rng.IntN(2) == 0, pan-0.12)
		c.k[1] = newSndTalker(rng, i%2 == 0, pan+0.12)
		pairs = append(pairs, c)
	}
	hpL, hpR := sndFilter('h', 280, 0.6), sndFilter('h', 280, 0.6)
	hp2L, hp2R := sndFilter('h', 280, 0.6), sndFilter('h', 280, 0.6)
	lp1L, lp1R := sndFilter('l', 2000, 0.6), sndFilter('l', 2000, 0.6)
	lp2L, lp2R := sndFilter('l', 2600, 0.6), sndFilter('l', 2600, 0.6)
	verb := newSndVerb(0.7, 0.35)
	sndBed(s, func(int) (float64, float64) {
		var l, r float64
		for _, c := range pairs {
			if !c.k[0].on && !c.k[1].on {
				if c.pause--; c.pause <= 0 {
					c.who = 1 - c.who
					if rng.Float64() < 0.25 {
						c.who = 1 - c.who // the same one goes on
					}
					c.k[c.who].say(int((1.2+4.5*rng.Float64())*sndRate), rng)
					c.pause = int((0.15 + 0.7*rng.Float64()) * sndRate)
					if rng.Float64() < 0.12 {
						c.pause = int((3 + 6*rng.Float64()) * sndRate) // both quiet a while
					}
				}
			}
			for _, k := range c.k {
				v := k.next(rng)
				l += v * k.g.l
				r += v * k.g.r
			}
		}
		l = lp2L.step(lp1L.step(hp2L.step(hpL.step(l))))
		r = lp2R.step(lp1R.step(hp2R.step(hpR.step(r))))
		el, er := verb.step(l, r)
		return 0.35*l + 1.2*el, 0.35*r + 1.2*er
	})
}

// typingPart is two or three people typing at their desks, each with a
// keyboard of its own: words of a few letters with a space between, in
// bursts, with pauses to think and now and then a few backspaces or Enter.
func (p *sndPiece) typingPart(s sndStereo) {
	rng := p.rng(32)
	for range 2 + rng.IntN(2) {
		g := sndPan(1.6*rng.Float64() - 0.8)
		near := 0.3 + 0.7*rng.Float64()
		kb := sndKeyboard{tone: 0.8 + 0.4*rng.Float64(), far: 2500 + 5000*near, vel: 0.5 + 0.5*near}
		gap := func() float64 { // between keys: mostly quick, sometimes slow
			r := rng.Float64()
			return (0.1 + 0.08*r + 0.12*r*r*r) * sndRate
		}
		t := rng.Float64() * 8 * sndRate
		for t < float64(p.frames) {
			end := t + (2+10*rng.Float64())*sndRate
			for t < end {
				for range 2 + rng.IntN(8) {
					kb.key(s, int(t), 0, g, rng)
					t += gap()
				}
				kb.key(s, int(t), 1, g, rng) // space
				t += gap()
				switch r := rng.Float64(); {
				case r < 0.1:
					t += (0.5 + 1.5*rng.Float64()) * sndRate // thinking
				case r < 0.15:
					for range 2 + rng.IntN(5) {
						kb.key(s, int(t), 0, g, rng) // backspaces
						t += (0.08 + 0.03*rng.Float64()) * sndRate
					}
				}
			}
			if rng.Float64() < 0.4 {
				kb.key(s, int(t), 2, g, rng) // Enter
			}
			t += (4 + 16*rng.Float64()) * sndRate
		}
	}
}

// sndKeyboard is one person's keyboard: its tone, how far off (a low-pass
// cut, Hz) and how loud.
type sndKeyboard struct{ tone, far, vel float64 }

// key is a key pressed: a sharp click as it goes, the cap knocking the
// board a moment later, and a softer click as it comes up. kind 1 is the
// space bar (bigger, duller, with a rattle), 2 Enter. All noise, no pitch.
func (kb sndKeyboard) key(s sndStereo, start, kind int, g sndPanG, rng *rand.Rand) {
	j := 0.85 + 0.3*rng.Float64() // no two keys alike
	hp := sndFilter('h', 1800*kb.tone*j, 0.7)
	body := sndFilter('l', 900*kb.tone*j, 0.7)
	far := sndFilter('l', kb.far, 0.7)
	bottom := int((0.003 + 0.004*rng.Float64()) * sndRate)
	up := int((0.06 + 0.08*rng.Float64()) * sndRate)
	knockT, bodyG, vel := 0.006, 1.5, kb.vel*(0.7+0.3*rng.Float64())
	switch kind {
	case 1:
		body = sndFilter('l', 380*kb.tone, 0.7)
		knockT, bodyG, vel = 0.016, 3.5, vel*1.3
		up += int(0.03 * sndRate)
	case 2:
		body = sndFilter('l', 600*kb.tone, 0.7)
		knockT, bodyG, vel = 0.01, 2.5, vel*1.15
	}
	for i := range up + sndRate/40 {
		t := float64(i) / sndRate
		click := math.Exp(-t / 0.0015)
		knock, rel := 0.0, 0.0
		if i >= bottom {
			knock = math.Exp(-float64(i-bottom) / sndRate / knockT)
			if kind == 1 && (i-bottom == sndRate/200 || i-bottom == sndRate*11/1000) {
				click += 0.4 // the bar's rattle
			}
		}
		if i >= up {
			rel = 0.35 * math.Exp(-float64(i-up)/sndRate/0.0015)
		}
		w := sndNoise(rng)
		v := hp.step(w)*(click+0.4*knock+rel) + bodyG*body.step(w)*(knock+0.3*rel)
		s.add(start+i, far.step(v)*vel, g)
	}
}

// events calls at for random moments, one every lo to hi seconds.
func (p *sndPiece) events(rng *rand.Rand, first, lo, hi float64, at func(int)) {
	for t := first * rng.Float64() * sndRate; t < float64(p.frames); t += (lo + (hi-lo)*rng.Float64()) * sndRate {
		at(int(t))
	}
}

// paperPart is a page turned or shuffled now and then.
func (p *sndPiece) paperPart(s sndStereo) {
	rng := p.rng(33)
	p.events(rng, 16, 14, 32, func(t int) {
		g := sndPan(1.4*rng.Float64() - 0.7)
		bp, hp, far := sndFilter('b', 3000, 0.5), sndFilter('h', 900, 0.7), sndFilter('l', 6000, 0.7)
		n := int((0.4 + 0.9*rng.Float64()) * sndRate)
		crackle := 0.0
		for i := range n {
			crackle *= 0.997
			if rng.Float64() < 0.004 {
				crackle += rng.Float64()
			}
			w := math.Sin(math.Pi * float64(i) / float64(n))
			s.add(t+i, far.step(hp.step(bp.step(sndNoise(rng))))*(0.25+crackle)*w*w, g)
		}
	})
}

// mugPart is a mug put down now and then: mostly a dull knock on the desk,
// with a short, quickly damped ring, and sometimes a spoon's tiny clink.
func (p *sndPiece) mugPart(s sndStereo) {
	rng := p.rng(34)
	p.events(rng, 30, 22, 45, func(t int) {
		g := sndPan(1.2*rng.Float64() - 0.6)
		sndClink(s, t, 1800+900*rng.Float64(), 1, true, g, rng)
		if rng.Float64() < 0.4 {
			sndClink(s, t+int((0.12+0.08*rng.Float64())*sndRate), 3200+900*rng.Float64(), 0.25, false, g, rng)
		}
	})
}

func sndClink(s sndStereo, start int, f, vel float64, knock bool, g sndPanG, rng *rand.Rand) {
	ratios := []float64{1, 1.53, 2.27, 3.05}
	amps := []float64{1, 0.55, 0.35, 0.2}
	taus := []float64{0.06, 0.045, 0.03, 0.02}
	lp, far := sndFilter('l', 500, 0.7), sndFilter('l', 5000, 0.7)
	ring := 0.25
	if !knock {
		ring = 0.5
	}
	for i := range sndRate * 3 / 10 {
		t := float64(i) / sndRate
		v := 0.0
		for k, r := range ratios {
			v += amps[k] * math.Sin(2*math.Pi*f*r*t) * math.Exp(-t/taus[k])
		}
		v *= ring * min(t/0.0005, 1)
		if knock {
			v += 2.5 * lp.step(sndNoise(rng)) * math.Exp(-t/0.025)
		}
		s.add(start+i, far.step(v)*vel, g)
	}
}

// creakPart is a chair creaking now and then as someone leans back: the
// seat's joint sticking and slipping, a click at a time, faster as it
// goes, through the wood (no steady pitch).
func (p *sndPiece) creakPart(s sndStereo) {
	rng := p.rng(35)
	p.events(rng, 25, 18, 40, func(t int) {
		g := sndPan(1.4*rng.Float64() - 0.7)
		wood1 := sndFilter('b', 700+500*rng.Float64(), 3)
		wood2 := sndFilter('b', 250+150*rng.Float64(), 2)
		far := sndFilter('l', 4000, 0.7)
		n := int((0.25 + 0.45*rng.Float64()) * sndRate)
		rate0, rate1 := 25+30*rng.Float64(), 80+80*rng.Float64()
		next := 0.0
		for i := range n {
			x := float64(i) / float64(n)
			imp := 0.0
			if float64(i) >= next {
				imp = 0.6 + 0.4*rng.Float64()
				next += sndRate / (rate0 + (rate1-rate0)*x) * (0.7 + 0.6*rng.Float64())
			}
			v := wood1.step(imp) + 0.7*wood2.step(imp)
			s.add(t+i, far.step(v)*math.Sin(math.Pi*x), g)
		}
	})
}

// stepsPart is now and then someone walking past on the carpet: soft,
// dull footfalls coming nearer and going, across the room.
func (p *sndPiece) stepsPart(s sndStereo) {
	rng := p.rng(36)
	p.events(rng, 30, 20, 45, func(t int) {
		from := 1.6*rng.Float64() - 0.8
		steps := 6 + rng.IntN(7)
		pace := (0.5 + 0.1*rng.Float64()) * sndRate
		for k := range steps {
			x := float64(k) / float64(steps-1)
			g := sndPan(from * (1 - 2*x))
			loud := 0.4 + 0.6*math.Sin(math.Pi*x)
			lp := sndFilter('l', 180+60*rng.Float64(), 0.7)
			scuff := sndFilter('b', 1500, 0.7)
			at := t + int(float64(k)*pace+(rng.Float64()-0.5)*0.04*sndRate)
			for i := range sndRate / 8 {
				tt := float64(i) / sndRate
				w := sndNoise(rng)
				v := 6*lp.step(w)*math.Exp(-tt/0.03) + 0.15*scuff.step(w)*math.Exp(-tt/0.05)
				s.add(at+i, v*loud*min(tt/0.004, 1), g)
			}
		}
	})
}
