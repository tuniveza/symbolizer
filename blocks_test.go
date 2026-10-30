package main

import (
	"reflect"
	"strings"
	"testing"
)

// The lines that start a block, as their text, for each kind of file.
func startTexts(lang, text string) []string {
	var lines [][]rune
	for _, l := range strings.Split(text, "\n") {
		lines = append(lines, []rune(l))
	}
	var out []string
	for r, s := range blockStarts(lines, lang) {
		if s {
			out = append(out, strings.TrimSpace(string(lines[r])))
		}
	}
	return out
}

func TestBlockStarts(t *testing.T) {
	cases := []struct {
		lang, text string
		want       []string
	}{
		{"plaintext", "one\ntwo\n\nthree", []string{"one", "three"}},
		{"Go", `package main

// main says hi.
func main() {
	x := 1
	y := 2
	if x > 0 {
		println(x)
	} else {
		println(y)
	}
	z := "{ not a brace"
	println(z)
}
func b() {}`, []string{"package main", "// main says hi.", "if x > 0 {", "} else {", "z := \"{ not a brace\"", "func b() {}"}},
		{"Python", `import os

def f(x):
    if x:
        return 1
    y = 2
    return y
class A:
    """A thing.

    notes:
      more
    """
    def g(self):
        for a in b:
            for c in d:
                pass
        return 1`, []string{"import os", "def f(x):", "y = 2", "class A:", "def g(self):", "return 1"}},
		{"Markdown", "# Title\nSome words\nmore words\n- one\n- two\n  still two\n```go\nx := 1\n\ny := 2\n```\nAfter the code.\n## Next\n> quoted",
			[]string{"# Title", "- one", "```go", "After the code.", "## Next", "> quoted"}},
		{"HTML", `<!DOCTYPE html>
<html>
<head>
  <meta charset="utf-8">
  <title>x</title>
</head>
<body>
  <ul>
    <li>a</li>
    <li>b</li>
  </ul>
  <p>after</p>
</body>
</html>`, []string{"<!DOCTYPE html>", "<html>", "<head>", "<body>", "<ul>", "<p>after</p>"}},
	}
	for _, c := range cases {
		if got := startTexts(c.lang, c.text); !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s:\n got %q\nwant %q", c.lang, got, c.want)
		}
	}
}

// Each { ( [ pairs with its own partner: brackets in strings and comments,
// and pairs that close on the same line, do not count.
func TestBracketSpans(t *testing.T) {
	text := "func f() {\n\ts := \"} ) ]\" // }\n\t/* {\n\t*/ x := []int{1, 2}\n\tg(a,\n\t\tb)\n}\nr := `{\n`"
	var lines [][]rune
	for _, l := range strings.Split(text, "\n") {
		lines = append(lines, []rune(l))
	}
	want := []blockSpan{{4, 5}, {0, 6}}
	if got := bracketSpans(lines, "Go"); !reflect.DeepEqual(got, want) {
		t.Errorf("got %v want %v", got, want)
	}
}

// The block Alt+B deletes, as its first and last line (1-based).
func TestBlockAround(t *testing.T) {
	goText := `package main

// f does it.
func f() {
	x := 1
	if x > 0 {
		println(x)
	} else {
		println(0)
	}
	g(a,
		b)
}`
	pyText := `import os

@cache
def f(x):
    if x:
        return 1

    return 2
y = 3`
	cases := []struct {
		lang, text string
		row        int // 1-based
		want       [2]int
	}{
		{"Go", goText, 1, [2]int{1, 1}},    // outside any bracket: its paragraph
		{"Go", goText, 5, [2]int{3, 13}},   // in f: all of f, with its comment
		{"Go", goText, 7, [2]int{6, 10}},   // in the if: if and else together
		{"Go", goText, 9, [2]int{6, 10}},   // in the else: the same
		{"Go", goText, 12, [2]int{11, 12}}, // a call over two lines
		{"Go", goText, 4, [2]int{3, 13}},   // on the opening line
		{"Python", pyText, 6, [2]int{5, 6}},
		{"Python", pyText, 8, [2]int{3, 8}}, // with its decorator, past the empty line
		{"Python", pyText, 9, [2]int{9, 9}},
		{"Markdown", "# A\nwords\n\n- one\n- two", 5, [2]int{4, 5}},
	}
	for _, c := range cases {
		var lines [][]rune
		for _, l := range strings.Split(c.text, "\n") {
			lines = append(lines, []rune(l))
		}
		sp, ok := blockAround(lines, c.lang, c.row-1)
		if got := [2]int{sp.top + 1, sp.bottom + 1}; !ok || got != c.want {
			t.Errorf("%s line %d: got %v (%v), want %v", c.lang, c.row, got, ok, c.want)
		}
	}
}
