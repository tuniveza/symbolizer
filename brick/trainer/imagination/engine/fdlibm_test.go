package engine

import (
	"bufio"
	"bytes"
	"math"
	"os/exec"
	"strconv"
	"strings"
	"testing"
)

func needNode(t testing.TB) string {
	p, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is not installed: the equivalence tests compare against the JavaScript engine and need node")
	}
	return p
}

// TestMathMatchesV8 compares Exp, Log, Log10, Sin, Cos and x*x (for x ** 2) with node on many inputs, bit for bit.
func TestMathMatchesV8(t *testing.T) {
	node := needNode(t)
	n := "60000"
	if testing.Short() {
		n = "5000"
	}
	out, err := exec.Command(node, "testdata/mathvec.js", n).Output()
	if err != nil {
		t.Fatal(err)
	}
	fns := map[string]func(float64) float64{"sin": Sin, "cos": Cos, "exp": Exp, "log": Log, "log10": Log10, "sq": func(x float64) float64 { return x * x },
		"fx2": func(x float64) float64 { return toFixedNum(x, 2) }, "fx3": func(x float64) float64 { return toFixedNum(x, 3) },
		"fx4": func(x float64) float64 { return toFixedNum(x, 4) }}
	bad := map[string]int{}
	total := map[string]int{}
	sc := bufio.NewScanner(bytes.NewReader(out))
	for sc.Scan() {
		f := strings.Fields(sc.Text())
		if len(f) < 3 {
			continue
		}
		xb, _ := strconv.ParseUint(f[1], 16, 64)
		x := math.Float64frombits(xb)
		if f[0] == "str" { // String(x): JavaScript's number printing (JSON.stringify uses the same)
			total["str"]++
			if got := jsNumber(x); got != f[2] {
				if bad["str"] < 3 {
					t.Errorf("String(%v): Go %q, node %q", x, got, f[2])
				}
				bad["str"]++
			}
			continue
		}
		yb, _ := strconv.ParseUint(f[2], 16, 64)
		got := math.Float64bits(fns[f[0]](x))
		total[f[0]]++
		if got != yb {
			if bad[f[0]] < 3 {
				t.Errorf("%s(%v) = %v, node says %v", f[0], x, math.Float64frombits(got), math.Float64frombits(yb))
			}
			bad[f[0]]++
		}
	}
	for k, v := range total {
		t.Logf("%s: %d inputs, %d differ", k, v, bad[k])
	}
}
