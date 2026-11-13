package main

import (
	"bytes"
	"encoding/binary"
	"math"
	"os"
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

// fakeSound turns the music on with a short loop and a fake player; each
// player started is sent on the channel it gives.
func fakeSound(t *testing.T) chan *fakeSink {
	t.Helper()
	started := make(chan *fakeSink, 8)
	oldOn, oldMuted, oldRoom, oldData, oldStart := soundOn, soundMuted, roomSound, soundData, soundStart
	t.Cleanup(func() {
		roomSound.quiet()
		soundOn, soundMuted, roomSound, soundData, soundStart = oldOn, oldMuted, oldRoom, oldData, oldStart
	})
	soundOn, soundMuted, roomSound = true, false, &roomMusic{}
	pcm := make([]byte, 4*sndRate/10)
	soundData = func() []byte { return pcm }
	soundStart = func() (soundSink, error) {
		f := &fakeSink{stopped: make(chan struct{})}
		started <- f
		return f, nil
	}
	return started
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

// The music starts as the room goes 3D and stops as it goes flat, as
// design mode is left and as sy ends.
func TestDesign3DSoundStartsStops(t *testing.T) {
	started := fakeSound(t)
	m := room3DModel(t, 120, 40)
	m.soundCmd()
	noStart(t, started, "the flat board")
	press(m, "ctrl+f")
	f := waitStart(t, started, "Ctrl+F: 3D")
	press(m, "esc")
	waitStop(t, f, "Esc: the flat board")
	if f.got.Len() == 0 {
		t.Fatal("the player was given no sound")
	}
	press(m, "ctrl+f")
	f = waitStart(t, started, "3D again")
	m.setMode(0)
	m.soundCmd()
	waitStop(t, f, "design mode left")
	m.setMode(designMode)
	m.soundCmd()
	f = waitStart(t, started, "back in design mode, the room still 3D")
	roomSound.quiet() // sy ending
	waitStop(t, f, "sy ending")
}

// M in 3D turns the music off and on, and says so; off stays off for the
// session, even after leaving 3D and coming back.
func TestDesign3DSoundMute(t *testing.T) {
	started := fakeSound(t)
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
	started := fakeSound(t)
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

// loopSamples is the loop's left and right channels, -1 to 1.
func loopSamples(t *testing.T) (l, r []float64) {
	t.Helper()
	pcm := soundLoop()
	if len(pcm) != 4*sndFrames {
		t.Fatalf("the loop: want %d bytes (%.1f s), got %d", 4*sndFrames, float64(sndFrames)/sndRate, len(pcm))
	}
	l, r = make([]float64, sndFrames), make([]float64, sndFrames)
	for i := range sndFrames {
		l[i] = float64(int16(binary.LittleEndian.Uint16(pcm[4*i:]))) / 32768
		r[i] = float64(int16(binary.LittleEndian.Uint16(pcm[4*i+2:]))) / 32768
	}
	return l, r
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

// The loop is 16 bars at 90 beats a minute, peaks at about -6 dBFS with no
// sample clipped, is neither faint nor loud, is wider than mono, joins end
// to start without a jump, and its bass is the strongest of its low end.
func TestSoundLoopLevels(t *testing.T) {
	if testing.Short() {
		t.Skip("makes the whole loop")
	}
	l, r := loopSamples(t)
	if secs := float64(len(l)) / sndRate; math.Abs(secs-128.0/3) > 0.01 {
		t.Fatalf("the loop: want 42.7 s, got %.2f", secs)
	}
	var peak, sum, diff float64
	for i := range l {
		peak = max(peak, math.Abs(l[i]), math.Abs(r[i]))
		sum += l[i]*l[i] + r[i]*r[i]
		diff += (l[i] - r[i]) * (l[i] - r[i])
	}
	rms := math.Sqrt(sum / float64(2*len(l)))
	if peak >= 32767.0/32768 || sndDB(peak) < -6.5 || sndDB(peak) > -5.5 {
		t.Fatalf("peak %.2f dBFS: want about -6, and nothing clipped", sndDB(peak))
	}
	if sndDB(rms) < -30 || sndDB(rms) > -12 {
		t.Fatalf("loudness %.1f dBFS RMS: want -30 to -12", sndDB(rms))
	}
	if side := math.Sqrt(diff / float64(len(l))); side < rms/30 || side > rms {
		t.Fatalf("stereo: the sides differ by %.1f dB under the whole: want a little width", sndDB(rms/side))
	}
	n := len(l)
	if jump := math.Abs(l[0]-l[n-1]) + math.Abs(r[0]-r[n-1]); jump > 0.05 {
		t.Fatalf("the loop's end jumps %.3f to its start", jump)
	}
	mono := make([]float64, n)
	for i := range mono {
		mono[i] = (l[i] + r[i]) / 2
	}
	band := func(fs ...float64) float64 {
		p := 0.0
		for _, f := range fs {
			p += sndGoertzel(mono, f)
		}
		return p / float64(len(fs))
	}
	bass := band(sndHz(31), sndHz(33), sndHz(36), sndHz(38), sndHz(40), sndHz(43), sndHz(45)) // the bass's roots and fifths
	mids := band(400, 630, 800, 1000)
	highs := band(2500, 4000, 6000, 9000)
	if bass < 10*mids || bass < 100*highs {
		t.Fatalf("the bass should lead the low end: bass %.3g, mids %.3g, highs %.3g", bass, mids, highs)
	}
}

// SY_SOUND_WAV=file writes the loop to a WAV file, to listen to.
func TestSoundLoopWAV(t *testing.T) {
	path := os.Getenv("SY_SOUND_WAV")
	if path == "" {
		t.Skip("SY_SOUND_WAV=file writes the loop there")
	}
	pcm := soundLoop()
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
	if err := os.WriteFile(path, append(h.Bytes(), pcm...), 0o644); err != nil {
		t.Fatal(err)
	}
}
