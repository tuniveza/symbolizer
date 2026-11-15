package engine

// JavaScript semantics the engine depends on: the mulberry32 RNG, Map insertion order, object key order,
// Number.prototype.toFixed, Math.round, UTF-16 string order and the number format of JSON.stringify.

import (
	"math"
	"math/big"
	"strconv"
	"unicode/utf16"
	"unicode/utf8"
)

// ---------- numbers ----------

// toUint32 is ECMAScript ToUint32 (what `x >>> 0` does).
func toUint32(x float64) uint32 {
	if x != x || math.IsInf(x, 0) {
		return 0
	}
	t := math.Trunc(x)
	m := math.Mod(t, 4294967296)
	if m < 0 {
		m += 4294967296
	}
	return uint32(m)
}

// Rng is mulberry32, exactly as sim.js's rng(seed).
type Rng struct{ a uint32 }

func NewRng(seed float64) *Rng { return &Rng{a: toUint32(seed)} }

func (r *Rng) Next() float64 {
	r.a += 0x6d2b79f5
	a := r.a
	t := (a ^ (a >> 15)) * (1 | a)
	t = (t + (t^(t>>7))*(61|t)) ^ t
	return float64(t^(t>>14)) / 4294967296
}

func jsMin(a, b float64) float64 {
	if a != a || b != b {
		return math.NaN()
	}
	if a < b {
		return a
	}
	if b < a {
		return b
	}
	if a == 0 && b == 0 && math.Signbit(a) {
		return a
	}
	return b
}

func jsMax(a, b float64) float64 {
	if a != a || b != b {
		return math.NaN()
	}
	if a > b {
		return a
	}
	if b > a {
		return b
	}
	if a == 0 && b == 0 && !math.Signbit(a) {
		return a
	}
	return b
}

func jsSign(x float64) float64 {
	if x != x || x == 0 {
		return x
	}
	if x > 0 {
		return 1
	}
	return -1
}

// orOne is `x || 1` for a number.
func orOne(x float64) float64 {
	if x == 0 || x != x {
		return 1
	}
	return x
}

// jsRound is Math.round: halves go up (toward +Infinity).
func jsRound(x float64) float64 {
	if x != x || math.IsInf(x, 0) {
		return x
	}
	if x == 0 {
		return x
	}
	if x > 0 && x < 0.5 {
		return 0
	}
	if x < 0 && x >= -0.5 {
		return math.Copysign(0, -1)
	}
	r := math.Floor(x)
	if x-r >= 0.5 {
		r++
	}
	return r
}

var pow10 = [...]float64{1, 10, 100, 1000, 1e4, 1e5, 1e6, 1e7, 1e8, 1e9, 1e10}

// toFixedNum is `+x.toFixed(d)`: x rounded to d decimals (ties on the exact binary value go away from zero),
// read back as the nearest double.
func toFixedNum(x float64, d int) float64 {
	if x != x || math.IsInf(x, 0) || math.Abs(x) >= 1e21 {
		return x
	}
	neg := x < 0
	ax := math.Abs(x)
	p := pow10[d]
	y := ax * p
	var n float64
	fl := math.Floor(y)
	frac := y - fl
	if y < 1e15 && math.Abs(frac-0.5) > y*4e-16+1e-12 {
		n = fl
		if frac > 0.5 {
			n++
		}
	} else {
		// exact: n = floor(ax*10^d + 1/2) on the exact value of ax
		r := new(big.Rat).SetFloat64(ax)
		r.Mul(r, new(big.Rat).SetInt64(int64(p)))
		r.Add(r, big.NewRat(1, 2))
		q := new(big.Int).Quo(r.Num(), r.Denom())
		n, _ = new(big.Float).SetInt(q).Float64()
		if n >= 1<<53 {
			// too many digits to divide exactly: fall back to the decimal string
			s := q.String()
			for len(s) <= d {
				s = "0" + s
			}
			v, _ := strconv.ParseFloat(s[:len(s)-d]+"."+s[len(s)-d:], 64)
			if neg {
				return -v
			}
			return v
		}
	}
	v := n / p
	if neg {
		return -v
	}
	return v
}

// jsNumber formats a number as JavaScript's Number.prototype.toString does (and JSON.stringify for finite numbers).
func jsNumber(x float64) string {
	if x != x {
		return "NaN"
	}
	if math.IsInf(x, 1) {
		return "Infinity"
	}
	if math.IsInf(x, -1) {
		return "-Infinity"
	}
	if x == 0 {
		return "0"
	}
	var buf [32]byte
	b := strconv.AppendFloat(buf[:0], x, 'e', -1, 64)
	sign := ""
	if b[0] == '-' {
		sign = "-"
		b = b[1:]
	}
	// b = d[.ddd]e±XX
	ei := 0
	for ei < len(b) && b[ei] != 'e' {
		ei++
	}
	mant := b[:ei]
	exp, _ := strconv.Atoi(string(b[ei+1:]))
	digits := make([]byte, 0, 20)
	for _, c := range mant {
		if c != '.' {
			digits = append(digits, c)
		}
	}
	k := len(digits)
	n := exp + 1 // value = 0.digits * 10^n
	out := make([]byte, 0, 32)
	out = append(out, sign...)
	switch {
	case k <= n && n <= 21:
		out = append(out, digits...)
		for i := 0; i < n-k; i++ {
			out = append(out, '0')
		}
	case 0 < n && n <= 21:
		out = append(out, digits[:n]...)
		out = append(out, '.')
		out = append(out, digits[n:]...)
	case -6 < n && n <= 0:
		out = append(out, '0', '.')
		for i := 0; i < -n; i++ {
			out = append(out, '0')
		}
		out = append(out, digits...)
	default:
		out = append(out, digits[0])
		if k > 1 {
			out = append(out, '.')
			out = append(out, digits[1:]...)
		}
		out = append(out, 'e')
		if n-1 >= 0 {
			out = append(out, '+')
		}
		out = strconv.AppendInt(out, int64(n-1), 10)
	}
	return string(out)
}

// ---------- strings ----------

// lessUTF16 is JavaScript's `a < b` for strings (UTF-16 code unit order).
func lessUTF16(a, b string) bool { return cmpUTF16(a, b) < 0 }

func cmpUTF16(a, b string) int {
	for i, j := 0, 0; ; {
		if i >= len(a) {
			if j >= len(b) {
				return 0
			}
			return -1
		}
		if j >= len(b) {
			return 1
		}
		ra, na := utf8.DecodeRuneInString(a[i:])
		rb, nb := utf8.DecodeRuneInString(b[j:])
		if ra == rb {
			i += na
			j += nb
			continue
		}
		ua, ub := unitOf(ra), unitOf(rb)
		if ua != ub {
			if ua < ub {
				return -1
			}
			return 1
		}
		// same first unit (both astral with the same high surrogate): compare the low units
		_, la := utf16.EncodeRune(ra)
		_, lb := utf16.EncodeRune(rb)
		if la < lb {
			return -1
		}
		return 1
	}
}

func unitOf(r rune) rune {
	if r >= 0x10000 {
		h, _ := utf16.EncodeRune(r)
		return h
	}
	return r
}

// pairKey is sim.js's pairKey for words.
func pairKey(a, b string) string {
	if lessUTF16(a, b) {
		return a + "|" + b
	}
	return b + "|" + a
}

// isArrayIndex reports whether a property key is an array index (those come first, ascending, in JS objects).
func isArrayIndex(k string) (uint32, bool) {
	if k == "" || len(k) > 10 {
		return 0, false
	}
	if k == "0" {
		return 0, true
	}
	if k[0] < '1' || k[0] > '9' {
		return 0, false
	}
	var v uint64
	for i := 0; i < len(k); i++ {
		c := k[i]
		if c < '0' || c > '9' {
			return 0, false
		}
		v = v*10 + uint64(c-'0')
	}
	if v >= 4294967295 {
		return 0, false
	}
	return uint32(v), true
}

// ---------- ordered map (a JS Map) ----------

type omEntry[K comparable, V any] struct {
	k    K
	v    V
	dead bool
}

// OMap keeps insertion order like a JS Map: setting an existing key keeps its place, deleting then setting
// moves it to the end, and entries added while iterating are visited.
type OMap[K comparable, V any] struct {
	e    []omEntry[K, V]
	idx  map[K]int32
	live int
	iter int
}

const omSmall = 8

func (m *OMap[K, V]) find(k K) int {
	if m.idx != nil {
		if p, ok := m.idx[k]; ok {
			return int(p)
		}
		return -1
	}
	for i := range m.e {
		if !m.e[i].dead && m.e[i].k == k {
			return i
		}
	}
	return -1
}

func (m *OMap[K, V]) Len() int { return m.live }

func (m *OMap[K, V]) Get(k K) (V, bool) {
	if p := m.find(k); p >= 0 {
		return m.e[p].v, true
	}
	var z V
	return z, false
}

func (m *OMap[K, V]) Has(k K) bool { return m.find(k) >= 0 }

func (m *OMap[K, V]) Set(k K, v V) {
	if p := m.find(k); p >= 0 {
		m.e[p].v = v
		return
	}
	m.e = append(m.e, omEntry[K, V]{k: k, v: v})
	m.live++
	if m.idx != nil {
		m.idx[k] = int32(len(m.e) - 1)
	} else if len(m.e) > omSmall {
		m.buildIdx()
	}
}

func (m *OMap[K, V]) buildIdx() {
	m.idx = make(map[K]int32, len(m.e)*2)
	for i := range m.e {
		if !m.e[i].dead {
			m.idx[m.e[i].k] = int32(i)
		}
	}
}

func (m *OMap[K, V]) Delete(k K) bool {
	p := m.find(k)
	if p < 0 {
		return false
	}
	m.deleteAt(p)
	return true
}

func (m *OMap[K, V]) deleteAt(p int) {
	m.e[p].dead = true
	var z V
	m.e[p].v = z
	if m.idx != nil {
		delete(m.idx, m.e[p].k)
	}
	m.live--
	m.maybeCompact()
}

func (m *OMap[K, V]) maybeCompact() {
	if m.iter > 0 {
		return
	}
	dead := len(m.e) - m.live
	if m.live == 0 {
		m.e = m.e[:0]
		if m.idx != nil {
			clear(m.idx)
		}
		return
	}
	if dead > 16 && dead > m.live {
		j := 0
		for i := range m.e {
			if !m.e[i].dead {
				m.e[j] = m.e[i]
				j++
			}
		}
		clear(m.e[j:])
		m.e = m.e[:j]
		if m.idx != nil {
			if j <= omSmall {
				m.idx = nil
			} else {
				m.buildIdx()
			}
		}
	}
}

// appendNew adds a key the caller knows is not in the map (the per-step maps of step(): each pair is visited once,
// so a source is never set twice in one step). It keeps no index: these maps are only iterated, never searched.
func (m *OMap[K, V]) appendNew(k K, v V) {
	if m.idx != nil {
		m.idx[k] = int32(len(m.e))
	}
	m.e = append(m.e, omEntry[K, V]{k: k, v: v})
	m.live++
}

// Clear empties the map, keeping its memory.
func (m *OMap[K, V]) Clear() {
	clear(m.e)
	m.e = m.e[:0]
	m.live = 0
	if m.idx != nil {
		clear(m.idx)
	}
}

// All iterates in insertion order (JS for-of over a Map).
func (m *OMap[K, V]) All() func(yield func(K, V) bool) {
	return func(yield func(K, V) bool) {
		m.iter++
		defer func() { m.iter--; m.maybeCompact() }()
		for i := 0; i < len(m.e); i++ {
			if m.e[i].dead {
				continue
			}
			if !yield(m.e[i].k, m.e[i].v) {
				return
			}
		}
	}
}

// Keys returns a snapshot of the keys in order.
func (m *OMap[K, V]) Keys() []K {
	out := make([]K, 0, m.live)
	for i := range m.e {
		if !m.e[i].dead {
			out = append(out, m.e[i].k)
		}
	}
	return out
}

// Clone copies the map (new Map(m)).
func (m *OMap[K, V]) Clone() OMap[K, V] {
	var c OMap[K, V]
	c.e = make([]omEntry[K, V], 0, m.live)
	for i := range m.e {
		if !m.e[i].dead {
			c.e = append(c.e, m.e[i])
		}
	}
	c.live = len(c.e)
	if c.live > omSmall {
		c.buildIdx()
	}
	return c
}

// ---------- ordered string set (a JS Set of strings) ----------

type StrSet struct {
	list []string
	has  map[string]bool
}

func (s *StrSet) Add(v string) {
	if s.has == nil {
		s.has = map[string]bool{}
	}
	if !s.has[v] {
		s.has[v] = true
		s.list = append(s.list, v)
	}
}
func (s *StrSet) Has(v string) bool { return s.has[v] }
func (s *StrSet) Len() int          { return len(s.list) }

// ---------- JS object with number values (property order: array indices ascending, then insertion) ----------

type NumObj struct {
	keys []string
	vals []float64
	idx  map[string]int
}

func (o *NumObj) Get(k string) (float64, bool) {
	if o.idx == nil {
		return 0, false
	}
	p, ok := o.idx[k]
	if !ok {
		return 0, false
	}
	return o.vals[p], true
}

func (o *NumObj) Set(k string, v float64) {
	if o.idx == nil {
		o.idx = map[string]int{}
	}
	if p, ok := o.idx[k]; ok {
		o.vals[p] = v
		return
	}
	o.idx[k] = len(o.keys)
	o.keys = append(o.keys, k)
	o.vals = append(o.vals, v)
}

// Order returns positions in JS property order.
func (o *NumObj) Order() []int { return jsKeyOrder(o.keys) }

func jsKeyOrder(keys []string) []int {
	var ints []int
	var rest []int
	for i, k := range keys {
		if _, ok := isArrayIndex(k); ok {
			ints = append(ints, i)
		} else {
			rest = append(rest, i)
		}
	}
	if len(ints) == 0 {
		return rest
	}
	// insertion sort by numeric value (few of them)
	for i := 1; i < len(ints); i++ {
		for j := i; j > 0; j-- {
			a, _ := isArrayIndex(keys[ints[j-1]])
			b, _ := isArrayIndex(keys[ints[j]])
			if a > b {
				ints[j-1], ints[j] = ints[j], ints[j-1]
			} else {
				break
			}
		}
	}
	return append(ints, rest...)
}

// ---------- JSON writer (JSON.stringify's output) ----------

type jw struct{ b []byte }

func (w *jw) raw(s string) { w.b = append(w.b, s...) }
func (w *jw) num(x float64) {
	if x != x || math.IsInf(x, 0) {
		w.b = append(w.b, "null"...)
		return
	}
	w.b = append(w.b, jsNumber(x)...)
}
func (w *jw) boolean(v bool) {
	if v {
		w.raw("true")
	} else {
		w.raw("false")
	}
}

func (w *jw) str(s string) {
	w.b = append(w.b, '"')
	for i := 0; i < len(s); {
		c := s[i]
		if c < 0x80 {
			switch c {
			case '"':
				w.b = append(w.b, '\\', '"')
			case '\\':
				w.b = append(w.b, '\\', '\\')
			case '\b':
				w.b = append(w.b, '\\', 'b')
			case '\f':
				w.b = append(w.b, '\\', 'f')
			case '\n':
				w.b = append(w.b, '\\', 'n')
			case '\r':
				w.b = append(w.b, '\\', 'r')
			case '\t':
				w.b = append(w.b, '\\', 't')
			default:
				if c < 0x20 {
					const hex = "0123456789abcdef"
					w.b = append(w.b, '\\', 'u', '0', '0', hex[c>>4], hex[c&15])
				} else {
					w.b = append(w.b, c)
				}
			}
			i++
			continue
		}
		r, n := utf8.DecodeRuneInString(s[i:])
		if r >= 0xD800 && r <= 0xDFFF || r == utf8.RuneError && n == 1 {
			// not representable in valid UTF-8 output; JSON.stringify would escape a lone surrogate
			w.b = append(w.b, "\\ufffd"...)
		} else {
			w.b = append(w.b, s[i:i+n]...)
		}
		i += n
	}
	w.b = append(w.b, '"')
}

func (w *jw) key(k string) { w.str(k); w.b = append(w.b, ':') }
