package engine

// Bit-exact ports of the math routines V8 (node) uses for Math.exp, Math.log, Math.log10, Math.sin and
// Math.cos: V8's src/base/ieee754.cc, itself a port of fdlibm. Go's math package uses other algorithms (and
// assembly on amd64), which differ by an ulp here and there; the simulation is chaotic, so one ulp is enough to
// make two runs drift apart. Checked against node on millions of inputs (see fdlibm_test.go).
//
// Every constant below is a variable on purpose where it takes part in arithmetic with another constant: Go
// folds constant expressions exactly (no rounding per step), which would differ from C's double arithmetic.

import "math"

func hiWord(x float64) int32  { return int32(math.Float64bits(x) >> 32) }
func loWord(x float64) uint32 { return uint32(math.Float64bits(x)) }
func fromWords(hi int32, lo uint32) float64 {
	return math.Float64frombits(uint64(uint32(hi))<<32 | uint64(lo))
}
func setHigh(x float64, hi int32) float64 { return fromWords(hi, loWord(x)) }
func setLow(x float64, lo uint32) float64 { return fromWords(hiWord(x), lo) }

var (
	expHalF     = [2]float64{0.5, -0.5}
	expOThresh  = 7.09782712893383973096e+02
	expUThresh  = -7.45133219101941108420e+02
	expLn2HI    = [2]float64{6.93147180369123816490e-01, -6.93147180369123816490e-01}
	expLn2LO    = [2]float64{1.90821492927058770002e-10, -1.90821492927058770002e-10}
	expInvLn2   = 1.44269504088896338700e+00
	expP1       = 1.66666666666666019037e-01
	expP2       = -2.77777777770155933842e-03
	expP3       = 6.61375632143793436117e-05
	expP4       = -1.65339022054652515390e-06
	expP5       = 4.13813679705723846039e-08
	expE        = 2.718281828459045
	expHuge     = 1.0e+300
	expTwom1000 = 9.33263618503218878990e-302
	expTwo1023  = 8.988465674311579539e307
)

// Exp is V8's ieee754::exp.
func Exp(x float64) float64 {
	var hi, lo, c, t, y, twopk float64
	k := int32(0)
	hx := uint32(hiWord(x))
	xsb := int((hx >> 31) & 1)
	hx &= 0x7FFFFFFF
	if hx >= 0x40862E42 {
		if hx >= 0x7FF00000 {
			if ((hx & 0xFFFFF) | loWord(x)) != 0 {
				return x + x
			}
			if xsb == 0 {
				return x
			}
			return 0.0
		}
		if x > expOThresh {
			return expHuge * expHuge
		}
		if x < expUThresh {
			return expTwom1000 * expTwom1000
		}
	}
	if hx > 0x3FD62E42 {
		if hx < 0x3FF0A2B2 {
			if x == 1.0 {
				return expE
			}
			hi = x - expLn2HI[xsb]
			lo = expLn2LO[xsb]
			k = int32(1 - xsb - xsb)
		} else {
			k = int32(expInvLn2*x + expHalF[xsb])
			t = float64(k)
			hi = x - t*expLn2HI[0]
			lo = t * expLn2LO[0]
		}
		x = hi - lo
	} else if hx < 0x3E300000 {
		if expHuge+x > 1.0 {
			return 1.0 + x
		}
	} else {
		k = 0
	}
	t = x * x
	if k >= -1021 {
		twopk = fromWords(0x3FF00000+(k<<20), 0)
	} else {
		twopk = fromWords(0x3FF00000+((k+1000)<<20), 0)
	}
	c = x - t*(expP1+t*(expP2+t*(expP3+t*(expP4+t*expP5))))
	if k == 0 {
		return 1.0 - ((x*c)/(c-2.0) - x)
	}
	y = 1.0 - ((lo - (x*c)/(2.0-c)) - hi)
	if k >= -1021 {
		if k == 1024 {
			return y * 2.0 * expTwo1023
		}
		return y * twopk
	}
	return y * twopk * expTwom1000
}

var (
	logLn2Hi = 6.93147180369123816490e-01
	logLn2Lo = 1.90821492927058770002e-10
	logTwo54 = 1.80143985094819840000e+16
	logLg1   = 6.666666666666735130e-01
	logLg2   = 3.999999999940941908e-01
	logLg3   = 2.857142874366239149e-01
	logLg4   = 2.222219843214978396e-01
	logLg5   = 1.818357216161805012e-01
	logLg6   = 1.531383769920937332e-01
	logLg7   = 1.479819860511658591e-01
	logThird = 0.33333333333333333
	zeroVar  = 0.0
)

// Log is V8's ieee754::log (fdlibm e_log.c).
func Log(x float64) float64 {
	var hfsq, f, s, z, R, w, t1, t2, dk float64
	hx := hiWord(x)
	lx := loWord(x)
	k := int32(0)
	if hx < 0x00100000 {
		if ((hx & 0x7FFFFFFF) | int32(lx)) == 0 {
			return math.Inf(-1)
		}
		if hx < 0 {
			return math.NaN()
		}
		k -= 54
		x *= logTwo54
		hx = hiWord(x)
	}
	if hx >= 0x7FF00000 {
		return x + x
	}
	k += (hx >> 20) - 1023
	hx &= 0x000FFFFF
	i := (hx + 0x95F64) & 0x100000
	x = setHigh(x, hx|(i^0x3FF00000))
	k += i >> 20
	f = x - 1.0
	if (0x000FFFFF & (2 + hx)) < 3 {
		if f == zeroVar {
			if k == 0 {
				return 0
			}
			dk = float64(k)
			return dk*logLn2Hi + dk*logLn2Lo
		}
		R = f * f * (0.5 - logThird*f)
		if k == 0 {
			return f - R
		}
		dk = float64(k)
		return dk*logLn2Hi - ((R - dk*logLn2Lo) - f)
	}
	s = f / (2.0 + f)
	dk = float64(k)
	z = s * s
	i = hx - 0x6147A
	w = z * z
	j := 0x6B851 - hx
	t1 = w * (logLg2 + w*(logLg4+w*logLg6))
	t2 = z * (logLg1 + w*(logLg3+w*(logLg5+w*logLg7)))
	i |= j
	R = t2 + t1
	if i > 0 {
		hfsq = 0.5 * f * f
		if k == 0 {
			return f - (hfsq - s*(hfsq+R))
		}
		return dk*logLn2Hi - ((hfsq - (s*(hfsq+R) + dk*logLn2Lo)) - f)
	}
	if k == 0 {
		return f - s*(f-R)
	}
	return dk*logLn2Hi - ((s*(f-R) - dk*logLn2Lo) - f)
}

var (
	l10Two54  = 1.80143985094819840000e+16
	l10Ivln10 = 4.34294481903251816668e-01
	l10Log2Hi = 3.01029995663611771306e-01
	l10Log2Lo = 3.69423907715893078616e-13
)

// Log10 is V8's ieee754::log10 (fdlibm e_log10.c, built on log).
func Log10(x float64) float64 {
	hx := hiWord(x)
	lx := loWord(x)
	k := int32(0)
	if hx < 0x00100000 {
		if ((hx & 0x7FFFFFFF) | int32(lx)) == 0 {
			return math.Inf(-1)
		}
		if hx < 0 {
			return math.NaN()
		}
		k -= 54
		x *= l10Two54
		hx = hiWord(x)
		lx = loWord(x)
	}
	if hx >= 0x7FF00000 {
		return x + x
	}
	if hx == 0x3FF00000 && lx == 0 {
		return 0
	}
	k += (hx >> 20) - 1023
	i := int32((uint32(k) & 0x80000000) >> 31)
	hx = (hx & 0x000FFFFF) | ((0x3FF - i) << 20)
	y := float64(k + i)
	x = fromWords(hx, lx)
	z := y*l10Log2Lo + l10Ivln10*Log(x)
	return z + y*l10Log2Hi
}

// ---------- sin and cos ----------

var (
	ksS1   = -1.66666666666666324348e-01
	ksS2   = 8.33333333332248946124e-03
	ksS3   = -1.98412698298579493134e-04
	ksS4   = 2.75573137070700676789e-06
	ksS5   = -2.50507602534068634195e-08
	ksS6   = 1.58969099521155010221e-10
	kcC1   = 4.16666666666666019037e-02
	kcC2   = -1.38888888888741095749e-03
	kcC3   = 2.48015872894767294178e-05
	kcC4   = -2.75573143513906633035e-07
	kcC5   = 2.08757232129817482790e-09
	kcC6   = -1.13596475577881948265e-11
	halfV  = 0.5
	oneV   = 1.0
	q28125 = 0.28125
)

func kernelSin(x, y float64, iy int) float64 {
	ix := hiWord(x) & 0x7FFFFFFF
	if ix < 0x3E400000 {
		if int32(x) == 0 {
			return x
		}
	}
	z := x * x
	v := z * x
	r := ksS2 + z*(ksS3+z*(ksS4+z*(ksS5+z*ksS6)))
	if iy == 0 {
		return x + v*(ksS1+z*r)
	}
	return x - ((z*(halfV*y-v*r) - y) - v*ksS1)
}

func kernelCos(x, y float64) float64 {
	ix := hiWord(x) & 0x7FFFFFFF
	if ix < 0x3E400000 {
		if int32(x) == 0 {
			return oneV
		}
	}
	z := x * x
	r := z * (kcC1 + z*(kcC2+z*(kcC3+z*(kcC4+z*(kcC5+z*kcC6)))))
	if ix < 0x3FD33333 {
		return oneV - (halfV*z - (z*r - x*y))
	}
	var qx float64
	if ix > 0x3FE90000 {
		qx = q28125
	} else {
		qx = fromWords(ix-0x00200000, 0)
	}
	hz := halfV*z - qx
	a := oneV - qx
	return a - (hz - (z*r - x*y))
}

var twoOverPi = [66]int32{
	0xA2F983, 0x6E4E44, 0x1529FC, 0x2757D1, 0xF534DD, 0xC0DB62,
	0x95993C, 0x439041, 0xFE5163, 0xABDEBB, 0xC561B7, 0x246E3A,
	0x424DD2, 0xE00649, 0x2EEA09, 0xD1921C, 0xFE1DEB, 0x1CB129,
	0xA73EE8, 0x8235F5, 0x2EBB44, 0x84E99C, 0x7026B4, 0x5F7E41,
	0x3991D6, 0x398353, 0x39F49C, 0x845F8B, 0xBDF928, 0x3B1FF8,
	0x97FFDE, 0x05980F, 0xEF2F11, 0x8B5A0A, 0x6D1F6D, 0x367ECF,
	0x27CB09, 0xB74F46, 0x3F669E, 0x5FEA2D, 0x7527BA, 0xC7EBE5,
	0xF17B3D, 0x0739F7, 0x8A5292, 0xEA6BFB, 0x5FB11F, 0x8D5D08,
	0x560330, 0x46FC7B, 0x6BABF0, 0xCFBC20, 0x9AF436, 0x1DA9E3,
	0x91615E, 0xE61B08, 0x659985, 0x5F14A0, 0x68408D, 0xFFD880,
	0x4D7327, 0x310606, 0x1556CA, 0x73A8C9, 0x60E27B, 0xC08C6B,
}

var npio2HW = [32]int32{
	0x3FF921FB, 0x400921FB, 0x4012D97C, 0x401921FB, 0x401F6A7A, 0x4022D97C,
	0x4025FDBB, 0x402921FB, 0x402C463A, 0x402F6A7A, 0x4031475C, 0x4032D97C,
	0x40346B9C, 0x4035FDBB, 0x40378FDB, 0x403921FB, 0x403AB41B, 0x403C463A,
	0x403DD85A, 0x403F6A7A, 0x40407E4C, 0x4041475C, 0x4042106C, 0x4042D97C,
	0x4043A28C, 0x40446B9C, 0x404534AC, 0x4045FDBB, 0x4046C6CB, 0x40478FDB,
	0x404858EB, 0x404921FB,
}

var (
	rpTwo24   = 1.67772160000000000000e+07
	rpInvpio2 = 6.36619772367581382433e-01
	rpPio21   = 1.57079632673412561417e+00
	rpPio21t  = 6.07710050650619224932e-11
	rpPio22   = 6.07710050630396597660e-11
	rpPio22t  = 2.02226624879595063154e-21
	rpPio23   = 2.02226624871116645580e-21
	rpPio23t  = 8.47842766036889956997e-32
)

func remPio2(x float64) (int32, float64, float64) {
	var z, w, t, r, fn, y0, y1 float64
	hx := hiWord(x)
	ix := hx & 0x7FFFFFFF
	if ix <= 0x3FE921FB {
		return 0, x, 0
	}
	if ix < 0x4002D97C {
		if hx > 0 {
			z = x - rpPio21
			if ix != 0x3FF921FB {
				y0 = z - rpPio21t
				y1 = (z - y0) - rpPio21t
			} else {
				z -= rpPio22
				y0 = z - rpPio22t
				y1 = (z - y0) - rpPio22t
			}
			return 1, y0, y1
		}
		z = x + rpPio21
		if ix != 0x3FF921FB {
			y0 = z + rpPio21t
			y1 = (z - y0) + rpPio21t
		} else {
			z += rpPio22
			y0 = z + rpPio22t
			y1 = (z - y0) + rpPio22t
		}
		return -1, y0, y1
	}
	if ix <= 0x413921FB {
		t = math.Abs(x)
		n := int32(t*rpInvpio2 + halfV)
		fn = float64(n)
		r = t - fn*rpPio21
		w = fn * rpPio21t
		if n < 32 && ix != npio2HW[n-1] {
			y0 = r - w
		} else {
			j := ix >> 20
			y0 = r - w
			i := j - ((hiWord(y0) >> 20) & 0x7FF)
			if i > 16 {
				t = r
				w = fn * rpPio22
				r = t - w
				w = fn*rpPio22t - ((t - r) - w)
				y0 = r - w
				i = j - ((hiWord(y0) >> 20) & 0x7FF)
				if i > 49 {
					t = r
					w = fn * rpPio23
					r = t - w
					w = fn*rpPio23t - ((t - r) - w)
					y0 = r - w
				}
			}
		}
		y1 = (r - y0) - w
		if hx < 0 {
			return -n, -y0, -y1
		}
		return n, y0, y1
	}
	if ix >= 0x7FF00000 {
		return 0, x - x, x - x
	}
	var tx [3]float64
	z = fromWords(0, loWord(x))
	e0 := (ix >> 20) - 1046
	z = fromWords(ix-(e0<<20), loWord(x))
	for i := 0; i < 2; i++ {
		tx[i] = float64(int32(z))
		z = (z - tx[i]) * rpTwo24
	}
	tx[2] = z
	nx := 3
	for tx[nx-1] == 0 {
		nx--
	}
	n, a, b := kernelRemPio2(tx[:nx], int(e0), nx)
	if hx < 0 {
		return -n, -a, -b
	}
	return n, a, b
}

var pio2Parts = [8]float64{
	1.57079625129699707031e+00,
	7.54978941586159635335e-08,
	5.39030252995776476554e-15,
	3.28200341580791294123e-22,
	1.27065575308067607349e-29,
	1.22933308981111328932e-36,
	2.73370053816464559624e-44,
	2.16741683877804819444e-51,
}

var (
	krTwo24  = 1.67772160000000000000e+07
	krTwon24 = 5.96046447753906250000e-08
	krEight  = 8.0
	krEighth = 0.125
)

// kernelRemPio2 is fdlibm's __kernel_rem_pio2 with prec = 2 (the only precision rem_pio2 asks for).
func kernelRemPio2(x []float64, e0, nx int) (int32, float64, float64) {
	var iq [20]int32
	var f, fq, q [20]float64
	var z, fw float64
	const jk = 4 // init_jk[2]
	jp := jk
	jx := nx - 1
	jv := (e0 - 3) / 24
	if jv < 0 {
		jv = 0
	}
	q0 := e0 - 24*(jv+1)
	j := jv - jx
	m := jx + jk
	for i := 0; i <= m; i, j = i+1, j+1 {
		if j < 0 {
			f[i] = 0
		} else {
			f[i] = float64(twoOverPi[j])
		}
	}
	for i := 0; i <= jk; i++ {
		fw = 0.0
		for j := 0; j <= jx; j++ {
			fw += x[j] * f[jx+i-j]
		}
		q[i] = fw
	}
	jz := jk
	var n, ih int32
recompute:
	{
		i := 0
		j := jz
		z = q[jz]
		for ; j > 0; i, j = i+1, j-1 {
			fw = float64(int32(krTwon24 * z))
			iq[i] = int32(z - krTwo24*fw)
			z = q[j-1] + fw
		}
	}
	z = math.Ldexp(z, q0)
	z -= krEight * math.Floor(z*krEighth)
	n = int32(z)
	z -= float64(n)
	ih = 0
	if q0 > 0 {
		i := iq[jz-1] >> (24 - q0)
		n += i
		iq[jz-1] -= i << (24 - q0)
		ih = iq[jz-1] >> (23 - q0)
	} else if q0 == 0 {
		ih = iq[jz-1] >> 23
	} else if z >= 0.5 {
		ih = 2
	}
	if ih > 0 {
		n++
		carry := int32(0)
		for i := 0; i < jz; i++ {
			j := iq[i]
			if carry == 0 {
				if j != 0 {
					carry = 1
					iq[i] = 0x1000000 - j
				}
			} else {
				iq[i] = 0xFFFFFF - j
			}
		}
		if q0 > 0 {
			switch q0 {
			case 1:
				iq[jz-1] &= 0x7FFFFF
			case 2:
				iq[jz-1] &= 0x3FFFFF
			}
		}
		if ih == 2 {
			z = oneV - z
			if carry != 0 {
				z -= math.Ldexp(oneV, q0)
			}
		}
	}
	if z == 0 {
		j := int32(0)
		for i := jz - 1; i >= jk; i-- {
			j |= iq[i]
		}
		if j == 0 {
			k := 1
			for iq[jk-k] == 0 {
				k++
			}
			for i := jz + 1; i <= jz+k; i++ {
				f[jx+i] = float64(twoOverPi[jv+i])
				fw = 0.0
				for j := 0; j <= jx; j++ {
					fw += x[j] * f[jx+i-j]
				}
				q[i] = fw
			}
			jz += k
			goto recompute
		}
	}
	if z == 0.0 {
		jz--
		q0 -= 24
		for iq[jz] == 0 {
			jz--
			q0 -= 24
		}
	} else {
		z = math.Ldexp(z, -q0)
		if z >= krTwo24 {
			fw = float64(int32(krTwon24 * z))
			iq[jz] = int32(z - krTwo24*fw)
			jz++
			q0 += 24
			iq[jz] = int32(fw)
		} else {
			iq[jz] = int32(z)
		}
	}
	fw = math.Ldexp(oneV, q0)
	for i := jz; i >= 0; i-- {
		q[i] = fw * float64(iq[i])
		fw *= krTwon24
	}
	for i := jz; i >= 0; i-- {
		fw = 0.0
		for k := 0; k <= jp && k <= jz-i; k++ {
			fw += pio2Parts[k] * q[i+k]
		}
		fq[jz-i] = fw
	}
	fw = 0.0
	for i := jz; i >= 0; i-- {
		fw += fq[i]
	}
	var y0, y1 float64
	if ih == 0 {
		y0 = fw
	} else {
		y0 = -fw
	}
	fw = fq[0] - fw
	for i := 1; i <= jz; i++ {
		fw += fq[i]
	}
	if ih == 0 {
		y1 = fw
	} else {
		y1 = -fw
	}
	return n & 7, y0, y1
}

// Sin is V8's ieee754::sin (fdlibm).
func Sin(x float64) float64 {
	ix := hiWord(x) & 0x7FFFFFFF
	if ix <= 0x3FE921FB {
		return kernelSin(x, 0, 0)
	}
	if ix >= 0x7FF00000 {
		return x - x
	}
	n, y0, y1 := remPio2(x)
	switch n & 3 {
	case 0:
		return kernelSin(y0, y1, 1)
	case 1:
		return kernelCos(y0, y1)
	case 2:
		return -kernelSin(y0, y1, 1)
	default:
		return -kernelCos(y0, y1)
	}
}

// Cos is V8's ieee754::cos (fdlibm).
func Cos(x float64) float64 {
	ix := hiWord(x) & 0x7FFFFFFF
	if ix <= 0x3FE921FB {
		return kernelCos(x, 0)
	}
	if ix >= 0x7FF00000 {
		return x - x
	}
	n, y0, y1 := remPio2(x)
	switch n & 3 {
	case 0:
		return kernelCos(y0, y1)
	case 1:
		return -kernelSin(y0, y1, 1)
	case 2:
		return -kernelCos(y0, y1)
	default:
		return kernelSin(y0, y1, 1)
	}
}

// Hypot is V8's Math.hypot (builtins-math: largest magnitude first, then a Kahan sum of the scaled squares).
func Hypot(xs ...float64) float64 {
	max := 0.0
	nan := false
	abs := make([]float64, len(xs))
	for i, x := range xs {
		v := math.Abs(x)
		abs[i] = v
		if v != v {
			nan = true
		} else if v > max {
			max = v
		}
	}
	if math.IsInf(max, 1) {
		return max
	}
	if nan {
		return math.NaN()
	}
	if max == 0 {
		return 0
	}
	sum, comp := 0.0, 0.0
	for _, v := range abs {
		n := v / max
		summand := n*n - comp
		pre := sum + summand
		comp = (pre - sum) - summand
		sum = pre
	}
	return math.Sqrt(sum) * max
}
