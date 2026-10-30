package emmet

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// actSite makes a small project:
//
//	.git/  index.html  images/{logo.png,photo.jpeg,icon.png,icon.svg}
//	css/style.css  docs/  My Files/report.pdf  sub/page.html
//	src/{app.ts, utils/helper.js, components/{index.ts,Button.tsx}}
func actSite(t *testing.T) string {
	root := t.TempDir()
	for _, f := range []string{
		"index.html", "images/logo.png", "images/photo.jpeg", "images/icon.png", "images/icon.svg",
		"css/style.css", "My Files/report.pdf", "sub/page.html",
		"src/app.ts", "src/utils/helper.js", "src/components/index.ts", "src/components/Button.tsx",
	} {
		actWriteFile(t, filepath.Join(root, filepath.FromSlash(f)), []byte("x"))
	}
	for _, d := range []string{".git", "docs"} {
		if err := os.MkdirAll(filepath.Join(root, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func TestActCorrectPathAt(t *testing.T) {
	root := actSite(t)
	src := filepath.Join(root, "src")
	sub := filepath.Join(root, "sub")
	cases := []struct {
		base, in, want string // want "" = no change
	}{
		// HTML attributes
		{root, `<img src="imgs/logo.png"|>`, `<img src="images/logo.png"|>`},
		{root, `<img src="Images/Logo.PNG"|`, `<img src="images/logo.png"|`},
		{root, `<img src="images/logo.png"|>`, ""},
		{root, `<img src="images/logo.jpg"|>`, `<img src="images/logo.png"|>`},
		{root, `<img src="images/photo.jpg"|>`, `<img src="images/photo.jpeg"|>`},
		{root, `<img src="images/icon.jpg"|>`, ""}, // icon.png or icon.svg: not clear
		{root, `<img src="images/lgoo.png"|>`, `<img src="images/logo.png"|>`},
		{root, `<img src="images/logo"|>`, `<img src="images/logo.png"|>`},
		{root, `<img src="images/zzz.png"|>`, ""},
		{root, `<img src="nothere/logo.png"|>`, ""},
		{root, `<img src="https://x.com/a.png"|>`, ""},
		{root, `<a href="#top"|>`, ""},
		{root, `<a href="mailto:a@b.c"|>`, ""},
		{root, `<img src="data:image/png;base64,AA"|>`, ""},
		{root, `<script src="//cdn.x.org/a.js"|>`, ""},
		{root, `<img src="{{ url }}"|>`, ""},
		{root, `<img src="${base}/a.png"|>`, ""},
		{root, `<img src="imgs/logo.png?v=2#x"|>`, `<img src="images/logo.png?v=2#x"|>`},
		{sub, `<img src="/images/logo.png"|>`, ""},
		{sub, `<img src="/Images/logo.png"|>`, `<img src="/images/logo.png"|>`},
		{sub, `<img src="../imags/logo.png"|>`, `<img src="../images/logo.png"|>`},
		{root, `<a href="docs"|>`, ""},
		{root, `<a href="Docs/"|>`, `<a href="docs/"|>`},
		{root, `<video poster="images/Photo.jpeg"|>`, `<video poster="images/photo.jpeg"|>`},
		{root, `<object data='images/icon.SVG'|>`, `<object data='images/icon.svg'|>`},
		{root, `<form action="sub/pgae.html"|>`, `<form action="sub/page.html"|>`},
		{root, `<a title="imgs/logo.png"|>`, ""}, // not a path attribute
		{root, `<img srcset="imgs/logo.png 1x, images/Photo.jpeg 2x"|>`, `<img srcset="images/logo.png 1x, images/photo.jpeg 2x"|>`},
		{root, "<img src=\"imgs/logo.png\">\n|", "<img src=\"images/logo.png\">\n|"},
		{root, `<a href="My%20Files/Report.pdf"|>`, `<a href="My%20Files/report.pdf"|>`},
		{root, `<a href="my files/report.pdf"|>`, `<a href="My Files/report.pdf"|>`},
		// CSS
		{root, `a { background: url(imgs/logo.png)| }`, `a { background: url(images/logo.png)| }`},
		{root, `a { background: url("Images/logo.png")| }`, `a { background: url("images/logo.png")| }`},
		{root, `a { background: url(images/logo.png)| }`, ""},
		{root, `@import "css/styel.css";|`, `@import "css/style.css";|`},
		{root, `@import "css/style.css";|`, ""},
		// JS / TS
		{src, `import { h } from './utils/helpr'|`, `import { h } from './utils/helper'|`},
		{src, `import h from './utils/helper'|`, ""},
		{src, `import h from './utils/helper.js'|`, ""},
		{src, `import h from './utils/Helper.js';` + "\n|", `import h from './utils/helper.js';` + "\n|"},
		{src, `import B from './components'|`, ""},
		{src, `import B from './components/button'|`, `import B from './components/Button'|`},
		{src, `import B from './Components'|`, `import B from './components'|`},
		{src, `import a from './app.js'|`, ""},
		{src, `import x from 'react'|`, ""},
		{src, `import fs from 'fs'|`, ""},
		{src, `import h from 'utils/helper'|`, `import h from './utils/helper'|`},
		{src, `const h = require('./utils/helpr')|`, `const h = require('./utils/helper')|`},
		{src, `const m = import('./utils/Helper')|`, `const m = import('./utils/helper')|`},
		{src, `import './utils/helper'|`, ""},
		// plain strings
		{root, `const p = "./images/logo.PNG"|`, `const p = "./images/logo.png"|`},
		{root, `const p = "hello"|`, ""},
		{root, `const p = "imgs/logo.png"|`, ""}, // does not start with ./ ../ or /
		{root, `x|`, ""},
	}
	for _, c := range cases {
		text, sel := actMarks(c.in)
		ch, ok := CorrectPathAt(text, sel.Start, c.base)
		got := actResult(t, text, ch, ok)
		if got != c.want {
			t.Errorf("CorrectPathAt(%q) = %q, want %q", c.in, got, c.want)
			continue
		}
		if ok && !strings.HasPrefix(ch.Note, "path fixed: ") {
			t.Errorf("note %q", ch.Note)
		}
	}
	text, sel := actMarks(`<img src="imgs/logo.png"|>`)
	if ch, _ := CorrectPathAt(text, sel.Start, root); ch.Note != "path fixed: imgs/logo.png → images/logo.png" {
		t.Errorf("note %q", ch.Note)
	}
}

func TestActPathCandidates(t *testing.T) {
	root := actSite(t)
	cases := []struct {
		in   string
		want []string
	}{
		{`<img src="images/|`, []string{"images/icon.png", "images/icon.svg", "images/logo.png", "images/photo.jpeg"}},
		{`<img src="images/l|`, []string{"images/logo.png"}},
		{`<img src="images/I|`, []string{"images/icon.png", "images/icon.svg"}},
		{`<img src="|`, []string{"css/", "docs/", "images/", "index.html", "My Files/", "src/", "sub/"}},
		{`<img src=".|`, []string{".git/"}},
		{`a { b: url(ima|`, []string{"images/"}},
		{`<img src="/sub/|`, []string{"/sub/page.html"}},
		{`<a href="My%20Files/|`, []string{"My%20Files/report.pdf"}},
		{`<img src="nope/|`, nil},
		{`abc|`, nil},
		{`<a href="https://|`, nil},
		{`<a href="x.html?|`, nil},
		{`"a" + b|`, nil},
	}
	for _, c := range cases {
		text, sel := actMarks(c.in)
		got := PathCandidates(text, sel.Start, root)
		if !reflect.DeepEqual(got, c.want) {
			t.Errorf("PathCandidates(%q) = %q, want %q", c.in, got, c.want)
		}
	}
	// at most 50
	many := t.TempDir()
	for i := 0; i < 60; i++ {
		actWriteFile(t, filepath.Join(many, "f"+string(rune('A'+i%26))+string(rune('a'+i/26))), nil)
	}
	if got := PathCandidates([]rune(`"f`), 2, many); len(got) != 50 {
		t.Errorf("got %d candidates, want 50", len(got))
	}
}

func TestActTypoDist(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"imgs", "images", 2},
		{"logo", "lgoo", 1},
		{"style", "styel", 1},
		{"css", "js", -1},
		{"ab", "ac", -1},
		{"report", "rapirt", 2},
		{"abc", "xyz", -1},
	}
	for _, c := range cases {
		if got := actTypoDist(c.a, c.b); got != c.want {
			t.Errorf("actTypoDist(%q, %q) = %d, want %d", c.a, c.b, got, c.want)
		}
	}
}
