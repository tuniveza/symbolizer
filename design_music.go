package main

// Music on the board: a chiptune plays while design mode is open, if it
// is turned on (M on the board, or AltGr+M... see designMusicToggle). It
// starts off; the choice is kept in the house file, so it stays on or off
// the next time. Nothing is recorded: every piece is worked out here
// (design_chip.go), off Bubble Tea's loop, and played round and round by
// the desktop's own player (pw-play, else paplay, else aplay), fed raw
// samples on its standard input. With no player there is no sound and
// nothing is said.
//
// Each time the music starts a different piece plays (a new seed). The
// next piece is made in the background while one plays, so the next start
// is at once. SY_MUSIC=0 (or the old SY_3D_SOUND=0) turns it off for good.
// It stops, with a short fade, as design mode is left and as sy ends.

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
	sndAhead   = 100 * time.Millisecond // fed this far ahead of the player
	sndChunk   = sndRate / 50           // fed 20 ms at a time
	sndFadeIn  = sndRate * 3 / 2        // samples: 1.5 s
	sndFadeOut = sndRate * 3 / 50       // samples: 60 ms
)

var (
	// soundOn is whether the board may play music at all (tests set it).
	soundOn   = soundAllowed(os.Getenv)
	roomSound = &roomMusic{}
	// soundData makes the piece for a seed as 16-bit stereo samples (tests set it).
	soundData = chipPiece
	// soundStart starts a player that reads such samples (tests set it).
	soundStart = startSoundPlayer
)

// soundAllowed is false when SY_MUSIC=0 (or SY_3D_SOUND=0, its old name).
func soundAllowed(getenv func(string) string) bool {
	return getenv("SY_MUSIC") != "0" && getenv("SY_3D_SOUND") != "0"
}

// soundWanted is whether the music should play now: on the board, with
// the music turned on.
func (m *Model) soundWanted() bool {
	return soundOn && m.inDesign() && m.design != nil && m.hs().Music
}

// soundCmd runs after every message: it starts the music as the board
// opens (with music on) and stops it as design mode is left.
func (m *Model) soundCmd() tea.Cmd {
	roomSound.set(m.soundWanted())
	return nil
}

// designMusicToggle is M on the board: the chiptune on, or off. The choice
// is kept in the house file.
func (m *Model) designMusicToggle() {
	if !soundOn {
		m.say("Music is off for good: SY_MUSIC=0.")
		return
	}
	h := m.hs()
	h.Music = !h.Music
	_ = h.save()
	if h.Music {
		m.say("Music on (M: off).")
	} else {
		m.say("Music off (M: on).")
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

// sndNewSeed is a new piece's seed, not the last one's.
func sndNewSeed(last uint64) uint64 {
	for {
		if seed := rand.Uint64() | 1; seed != last {
			return seed
		}
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
