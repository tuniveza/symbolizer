package main

import (
	"reflect"
	"testing"
)

// The text of each step on a line, in order.
func stepTexts(line string) []string {
	l := []rune(line)
	var out []string
	for _, s := range steps(l) {
		out = append(out, string(l[s[0]:s[1]]))
	}
	return out
}

func TestSteps(t *testing.T) {
	cases := map[string][]string{
		"char *parts[MAX_PARTS];":        {"char", "*", "parts", "[", "MAX", "_", "PARTS", "]", ";"},
		"maxParts = parseHTTPRequest(x)": {"max", "Parts", "=", "parse", "HTTP", "Request", "(", "x", ")"},
		"def __init__(self):":            {"def", "__", "init", "__", "(", "self", ")", ":"},
		"snake_case_name x86_64 IOError": {"snake", "_", "case", "_", "name", "x86", "_", "64", "IO", "Error"},
		"x += foo(bar)  baz":             {"x", "+=", "foo", "(", "bar", ")", "baz"},
		"std::vector<int> v;":            {"std", "::", "vector", "<", "int", ">", "v", ";"},
		"p->next = q;":                   {"p", "->", "next", "=", "q", ";"},
		"if (a == b && c != d) {":        {"if", "(", "a", "==", "b", "&&", "c", "!=", "d", ")", "{"},
		"x = 3.14 + 0xFF - 1e-5;":        {"x", "=", "3.14", "+", "0xFF", "-", "1e-5", ";"},
		"cout << \"hi\" << endl;":        {"cout", "<<", "\"", "hi", "\"", "<<", "endl", ";"},
		"obj.method(a, b)":               {"obj", ".", "method", "(", "a", ",", "b", ")"},
		"// don't stop here":             {"//", "don't", "stop", "here"},
		"const f = (x) => x * 2;":        {"const", "f", "=", "(", "x", ")", "=>", "x", "*", "2", ";"},
		"width: 10px;":                   {"width", ":", "10px", ";"},
		"    return arr[i][j];":          {"return", "arr", "[", "i", "]", "[", "j", "]", ";"},
		"x := <-ch":                      {"x", ":=", "<-", "ch"},
		"I'm writing plain words, too.":  {"I'm", "writing", "plain", "words", ",", "too", "."},
	}
	for line, want := range cases {
		if got := stepTexts(line); !reflect.DeepEqual(got, want) {
			t.Errorf("%q\n got %q\nwant %q", line, got, want)
		}
	}
}

// The line from the screenshot, key by key.
func TestStepsOnScreenshotLine(t *testing.T) {
	m := newTest(t, "char *parts[MAX_PARTS];")
	for _, c := range []int{4, 6, 11, 12, 15, 16, 21, 22, 23} {
		press(m, "right")
		want(t, m, m.buf.Text(), Pos{0, c})
	}
	for _, c := range []int{22, 21, 16, 15, 12, 11, 6, 5, 0} {
		press(m, "left")
		want(t, m, m.buf.Text(), Pos{0, c})
	}
}
