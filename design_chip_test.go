package main

import (
	"bytes"
	"encoding/binary"
	"math"
	"os"
	"testing"
	"time"
)

func chipSamples(pcm []byte) []int16 {
	s := make([]int16, len(pcm)/2)
	for i := range s {
		s[i] = int16(binary.LittleEndian.Uint16(pcm[2*i:]))
	}
	return s
}

func TestChipLength(t *testing.T) {
	for seed := uint64(0); seed < 24; seed++ {
		pcm := chipPiece(seed)
		if len(pcm)%4 != 0 {
			t.Fatalf("seed %d: %d bytes is not whole stereo frames", seed, len(pcm))
		}
		sec := float64(len(pcm)/4) / chipRate
		if sec < 30 || sec > 50 {
			t.Errorf("seed %d: length %.1f s out of 30..50", seed, sec)
		}
	}
}

func TestChipDeterministic(t *testing.T) {
	for _, seed := range []uint64{1, 42, 1 << 40} {
		if !bytes.Equal(chipPiece(seed), chipPiece(seed)) {
			t.Errorf("seed %d: two renders differ", seed)
		}
		if chipStyleName(seed) != chipStyleName(seed) {
			t.Errorf("seed %d: style name differs", seed)
		}
	}
}

func TestChipSeedsDiffer(t *testing.T) {
	if bytes.Equal(chipPiece(1), chipPiece(2)) {
		t.Fatal("seeds 1 and 2 give the same piece")
	}
	names := map[string]bool{}
	for seed := uint64(0); seed < 12; seed++ {
		names[chipStyleName(seed)] = true
	}
	if len(names) < 10 {
		t.Errorf("only %d distinct styles in 12 seeds", len(names))
	}
}

func TestChipLevels(t *testing.T) {
	for seed := uint64(0); seed < 12; seed++ {
		s := chipSamples(chipPiece(seed))
		peak, sum := 0, 0.0
		for _, v := range s {
			a := int(v)
			if a < 0 {
				a = -a
			}
			peak = max(peak, a)
			sum += float64(v) * float64(v)
		}
		rms := math.Sqrt(sum/float64(len(s))) / 32768
		if peak > 16384 {
			t.Errorf("seed %d: peak %d above 0.5 FS", seed, peak)
		}
		if rms < 0.03 {
			t.Errorf("seed %d: RMS %.4f FS is nearly silent", seed, rms)
		}
	}
}

func TestChipLoopJoint(t *testing.T) {
	for seed := uint64(0); seed < 24; seed++ {
		s := chipSamples(chipPiece(seed))
		frames := len(s) / 2
		for ch := 0; ch < 2; ch++ {
			maxStep := 0.0
			for i := 1; i < frames; i++ {
				maxStep = math.Max(maxStep, math.Abs(float64(s[2*i+ch])-float64(s[2*(i-1)+ch])))
			}
			joint := math.Abs(float64(s[ch]) - float64(s[2*(frames-1)+ch]))
			if joint > maxStep || joint > 0.05*32768 {
				t.Errorf("seed %d ch %d: loop joint jump %.0f (largest normal step %.0f)", seed, ch, joint, maxStep)
			}
		}
	}
}

func TestChipRenderTime(t *testing.T) {
	var worst time.Duration
	for seed := uint64(100); seed < 108; seed++ {
		t0 := time.Now()
		chipPiece(seed)
		worst = max(worst, time.Since(t0))
	}
	t.Logf("slowest render of 8 seeds: %v", worst)
	if worst > time.Second {
		t.Errorf("render took %v, want well under 1 s", worst)
	}
}

func BenchmarkChipPiece(b *testing.B) {
	for i := 0; b.Loop(); i++ {
		chipPiece(uint64(i))
	}
}

// chipWriteWav writes PCM from chipPiece as a 48 kHz 16-bit stereo WAV file.
func chipWriteWav(path string, pcm []byte) error {
	h := make([]byte, 44)
	copy(h[0:], "RIFF")
	binary.LittleEndian.PutUint32(h[4:], uint32(36+len(pcm)))
	copy(h[8:], "WAVEfmt ")
	binary.LittleEndian.PutUint32(h[16:], 16)
	binary.LittleEndian.PutUint16(h[20:], 1)
	binary.LittleEndian.PutUint16(h[22:], 2)
	binary.LittleEndian.PutUint32(h[24:], chipRate)
	binary.LittleEndian.PutUint32(h[28:], chipRate*4)
	binary.LittleEndian.PutUint16(h[32:], 4)
	binary.LittleEndian.PutUint16(h[34:], 16)
	copy(h[36:], "data")
	binary.LittleEndian.PutUint32(h[40:], uint32(len(pcm)))
	return os.WriteFile(path, append(h, pcm...), 0o644)
}

// Writes a sample when CHIP_SAMPLE_WAV is set, e.g.
// CHIP_SAMPLE_WAV=sample.wav CHIP_SAMPLE_SEED=7 go test -run Sample
func TestChipSample(t *testing.T) {
	path := os.Getenv("CHIP_SAMPLE_WAV")
	if path == "" {
		t.Skip("set CHIP_SAMPLE_WAV to write a sample")
	}
	seed := uint64(7)
	if v := os.Getenv("CHIP_SAMPLE_SEED"); v != "" {
		var x uint64
		for _, c := range v {
			x = x*10 + uint64(c-'0')
		}
		seed = x
	}
	if err := chipWriteWav(path, chipPiece(seed)); err != nil {
		t.Fatal(err)
	}
	t.Logf("wrote %s: seed %d, %s", path, seed, chipStyleName(seed))
}
