package engine

import (
	"bytes"
	"math"
	"os/exec"
	"strconv"
	"testing"
	"time"
)

// scaleNet is results.js's scale network: n neurons, ~5 random lines each, frequencies 2..2+max(4, n/25).
func scaleNet(n int) *Network {
	r := NewRng(9)
	spec := &Spec{}
	for i := 0; i < n; i++ {
		om := 2 + r.Next()*math.Max(4, float64(n)/25)
		x := r.Next()
		y := r.Next()
		spec.Neurons = append(spec.Neurons, SpecNeuron{Label: "n" + strconv.Itoa(i), Omega: om, X: x, HasX: true, Y: y, HasY: true})
	}
	for i := 0; i < n; i++ {
		for q := 0; q < 5; q++ {
			j := int(math.Floor(r.Next() * float64(n)))
			if j != i {
				spec.Links = append(spec.Links, SpecLink{"n" + strconv.Itoa(i), "n" + strconv.Itoa(j), 0.4})
			}
		}
	}
	return NewNetwork(spec, 5, nil, nil, false)
}

// TestScale runs the scale networks in both engines: same saved state after the steps, and how long a step takes.
func TestScale(t *testing.T) {
	node := needNode(t)
	sizes := []struct{ n, k int }{{1000, 40}, {10000, 10}}
	if testing.Short() {
		sizes = sizes[:1]
	}
	for _, sz := range sizes {
		out, err := exec.Command(node, "testdata/scale.js", strconv.Itoa(sz.n), strconv.Itoa(sz.k)).Output()
		if err != nil {
			t.Fatal(err)
		}
		nl := bytes.IndexByte(out, '\n')
		jsMs, _ := strconv.ParseFloat(string(out[:nl]), 64)
		jsSave := bytes.TrimSuffix(out[nl+1:], []byte("\n"))
		net := scaleNet(sz.n)
		t0 := time.Now()
		for i := 0; i < sz.k; i++ {
			net.Step()
		}
		goMs := float64(time.Since(t0).Microseconds()) / 1000 / float64(sz.k)
		compareOutputs(t, "scale "+strconv.Itoa(sz.n), append([]byte("\n\n\n"), jsSave...), append([]byte("\n\n\n"), net.Save()...))
		t.Logf("%d neurons, %d links: node %.2f ms/step, Go %.2f ms/step (%.1fx); saved states identical", sz.n, net.links.Len(), jsMs, goMs, jsMs/goMs)
	}
}

func BenchmarkStep1000(b *testing.B)  { benchStep(b, 1000) }
func BenchmarkStep10000(b *testing.B) { benchStep(b, 10000) }

func benchStep(b *testing.B, n int) {
	net := scaleNet(n)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		net.Step()
	}
}
