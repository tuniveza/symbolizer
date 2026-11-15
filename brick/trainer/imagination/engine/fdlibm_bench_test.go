package engine

import "testing"

var sinkF float64

func BenchmarkExp(b *testing.B) {
	x := -0.3
	for i := 0; i < b.N; i++ {
		sinkF += Exp(x)
		x -= 1e-7
	}
}

func BenchmarkSin(b *testing.B) {
	x := 3000.3
	for i := 0; i < b.N; i++ {
		sinkF += Sin(x)
		x += 1e-3
	}
}

func BenchmarkCos(b *testing.B) {
	x := 0.3
	for i := 0; i < b.N; i++ {
		sinkF += Cos(x)
		x += 1e-3
	}
}
