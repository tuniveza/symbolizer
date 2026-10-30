package emmet

// Path auto-correct and path completion for src/href/url()/import paths.

import (
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type actPathKind int

const (
	actPathURL   actPathKind = iota // HTML attributes, CSS url() and @import: the extension must be there
	actPathJS                       // JS/TS imports: the extension and "/index" may be left out
	actPathPlain                    // a plain quoted string that looks like a relative path
)

var actURLAttrs = map[string]bool{"src": true, "href": true, "srcset": true, "poster": true, "data": true, "action": true}

var actJSExts = []string{".js", ".ts", ".jsx", ".tsx", ".mjs", ".cjs", ".mts", ".cts", ".json", ".vue", ".svelte"}

var actNodeBuiltins = map[string]bool{
	"fs": true, "path": true, "os": true, "http": true, "https": true, "url": true, "util": true,
	"crypto": true, "events": true, "stream": true, "child_process": true, "assert": true,
	"buffer": true, "zlib": true, "net": true, "tls": true, "dns": true, "readline": true,
	"process": true, "worker_threads": true, "module": true, "vm": true, "querystring": true,
	"timers": true, "cluster": true, "http2": true, "perf_hooks": true,
}

// CorrectPathAt fixes the path the user just finished: call it after the
// closing quote or ')' was typed, or Enter was pressed after the path (pos is
// the cursor). It looks at HTML src, href, srcset (each URL), poster, data
// and action; CSS url(…) and @import "…"; JS/TS import … from '…',
// import('…') and require('…'); and any quoted string starting with ./ ../ or /.
//
// URLs with a scheme (http:, data:, mailto:, //…), #anchors and template
// placeholders ({{, ${, <?) are left alone; a ?query or #fragment is kept.
// A path that exists (relative to baseDir; a leading "/" is relative to the
// nearest folder at or above baseDir holding index.html or a project marker
// such as .git or package.json, else the filesystem root) is not changed.
// Otherwise each part is corrected to the one clearly best existing match:
// letter case (Images/Logo.PNG → images/logo.png), one or two letter typos,
// a wrong or missing extension (only with exactly one candidate), and for JS
// imports a missing "./". JS imports may leave out .js/.ts/… or point at a
// folder with an index file; an import written without extension stays so.
// Folders are never invented: when any part has no clear match, no change.
// Note says e.g. "path fixed: imgs/logo.png → images/logo.png".
func CorrectPathAt(text []rune, pos int, baseDir string) (Change, bool) {
	refs, kind := actFindPathBefore(text, pos)
	if len(refs) == 0 {
		return Change{}, false
	}
	var edits []Edit
	var notes []string
	for _, r := range refs {
		old := string(text[r.Start:r.End])
		fixed, ok := actFixPath(old, kind, baseDir)
		if !ok || fixed == old {
			continue
		}
		edits = append(edits, Edit{r.Start, r.End, fixed})
		notes = append(notes, old+" → "+fixed)
	}
	if len(edits) == 0 {
		return Change{}, false
	}
	p := actMapPos(edits, pos)
	return Change{Edits: edits, Sel: Range{p, p}, Note: "path fixed: " + strings.Join(notes, ", ")}, true
}

// actFindPathBefore finds the path string(s) ending at or just before pos.
// The ranges leave out quotes, ?query and #fragment.
func actFindPathBefore(text []rune, pos int) ([]Range, actPathKind) {
	if pos > len(text) {
		pos = len(text)
	}
	j := pos
	skipWS := func() {
		for j > 0 && actIsSpace(text[j-1]) {
			j--
		}
	}
	skipWS()
	if j > 0 && (text[j-1] == ';' || text[j-1] == ',') {
		j--
		skipWS()
	}
	if j > 0 && text[j-1] == '>' {
		j--
		if j > 0 && text[j-1] == '/' {
			j--
		}
		skipWS()
	}
	if j > 0 && text[j-1] == ')' && j > 1 && (text[j-2] == '"' || text[j-2] == '\'' || text[j-2] == '`') {
		j-- // require('…') / import('…') / url("…")
	}
	if j == 0 {
		return nil, 0
	}
	var s, e int
	kind := actPathPlain
	srcset := false
	switch q := text[j-1]; q {
	case '"', '\'', '`':
		e = j - 1
		s = e
		for s > 0 && text[s-1] != q && text[s-1] != '\n' {
			s--
		}
		if s == 0 || text[s-1] != q {
			return nil, 0
		}
		oq := s - 1
		b := oq
		for b > 0 && actIsSpace(text[b-1]) {
			b--
		}
		word := func(at int) string {
			k := at
			for k > 0 && (actIsNameChar(text[k-1]) || text[k-1] == '@') {
				k--
			}
			return string(text[k:at])
		}
		switch {
		case b > 0 && text[b-1] == '=':
			k := b - 1
			for k > 0 && actIsSpace(text[k-1]) {
				k--
			}
			name := strings.ToLower(word(k))
			if i := strings.LastIndexByte(name, ':'); i >= 0 { // xlink:href
				name = name[i+1:]
			}
			if actURLAttrs[name] {
				kind = actPathURL
				srcset = name == "srcset"
			}
		case b > 0 && text[b-1] == '(':
			k := b - 1
			for k > 0 && actIsSpace(text[k-1]) {
				k--
			}
			switch strings.ToLower(word(k)) {
			case "url":
				kind = actPathURL
			case "import", "require":
				kind = actPathJS
			}
		default:
			switch word(b) {
			case "from", "import":
				kind = actPathJS
			case "@import":
				kind = actPathURL
			}
		}
	case ')':
		e = j - 1
		s = e
		for s > 0 && text[s-1] != '(' && text[s-1] != '\n' && text[s-1] != ')' {
			s--
		}
		if s < 4 || text[s-1] != '(' || !strings.EqualFold(string(text[s-4:s-1]), "url") {
			return nil, 0
		}
		for s < e && actIsSpace(text[s]) {
			s++
		}
		for e > s && actIsSpace(text[e-1]) {
			e--
		}
		kind = actPathURL
	default:
		return nil, 0
	}
	var parts []Range
	if srcset {
		parts = actSplitSrcset(text, s, e)
	} else {
		parts = []Range{{s, e}}
	}
	var out []Range
	for _, r := range parts {
		p := string(text[r.Start:r.End])
		if actSkipPath(p) {
			continue
		}
		if kind == actPathPlain && !strings.HasPrefix(p, "./") && !strings.HasPrefix(p, "../") && !strings.HasPrefix(p, "/") {
			continue
		}
		if i := strings.IndexAny(p, "?#"); i >= 0 {
			r.End = r.Start + len([]rune(p[:i]))
		}
		if r.End > r.Start {
			out = append(out, r)
		}
	}
	return out, kind
}

// actSplitSrcset gives the URL of each "url 2x, url 300w" candidate.
func actSplitSrcset(text []rune, s, e int) []Range {
	var out []Range
	i := s
	for i < e {
		for i < e && (actIsSpace(text[i]) || text[i] == ',') {
			i++
		}
		st := i
		for i < e && !actIsSpace(text[i]) {
			i++
		}
		en := i
		if en > st && text[en-1] == ',' {
			en--
		}
		if en > st {
			out = append(out, Range{st, en})
		}
		for i < e && text[i] != ',' {
			i++
		}
	}
	return out
}

// actSkipPath: URLs with a scheme, protocol-relative URLs, anchors, template
// placeholders and empty strings are not paths to check.
func actSkipPath(p string) bool {
	switch {
	case p == "", strings.HasPrefix(p, "#"), strings.HasPrefix(p, "//"), strings.HasPrefix(p, "?"),
		actHasScheme(p), strings.Contains(p, "{{"), strings.Contains(p, "${"),
		strings.Contains(p, "<?"), strings.Contains(p, "<%"), strings.Contains(p, "\n"),
		strings.HasPrefix(p, "~"), strings.HasPrefix(p, "@/"):
		return true
	}
	return false
}

// actFixPath corrects one path. ok is false when it cannot be resolved.
func actFixPath(p string, kind actPathKind, baseDir string) (string, bool) {
	js := kind == actPathJS
	if js && !strings.HasPrefix(p, ".") && !strings.HasPrefix(p, "/") {
		// a bare name: a package, unless it is a local file missing its "./"
		first := strings.SplitN(p, "/", 2)[0]
		if strings.HasPrefix(first, "@") || actNodeBuiltins[first] || actHasNodeModule(baseDir, first) {
			return p, false
		}
		fixed, ok := actFixPath("./"+p, kind, baseDir)
		if !ok {
			return p, false
		}
		return fixed, true
	}
	segs := strings.Split(p, "/")
	cur := baseDir
	if strings.HasPrefix(p, "/") {
		cur = actSiteRoot(baseDir)
	}
	out := make([]string, len(segs))
	copy(out, segs)
	for i, raw := range segs {
		last := i == len(segs)-1
		seg := raw
		if strings.Contains(raw, "%") {
			if u, err := url.PathUnescape(raw); err == nil {
				seg = u
			}
		}
		switch seg {
		case "":
			continue
		case ".":
			continue
		case "..":
			cur = filepath.Dir(cur)
			continue
		}
		full := filepath.Join(cur, seg)
		if !last {
			if actIsDir(full) {
				cur = full
				continue
			}
			name, ok := actBestName(seg, actListDir(cur, true), false, false)
			if !ok {
				return p, false
			}
			out[i] = actEscapeLike(raw, name)
			cur = filepath.Join(cur, name)
			continue
		}
		if js {
			if actJSResolves(cur, seg) {
				continue
			}
		} else if _, err := os.Stat(full); err == nil {
			continue
		}
		name, ok := actBestName(seg, actListDir(cur, false), true, js && !actHasJSExt(seg))
		if !ok && js && !actHasJSExt(seg) && filepath.Ext(seg) != "" {
			name, ok = actBestName(seg, actListDir(cur, false), true, false)
		}
		if !ok {
			return p, false
		}
		out[i] = actEscapeLike(raw, name)
	}
	return strings.Join(out, "/"), true
}

func actEscapeLike(raw, name string) string {
	if strings.Contains(raw, "%") {
		return url.PathEscape(name)
	}
	return name
}

func actIsDir(p string) bool {
	st, err := os.Stat(p)
	return err == nil && st.IsDir()
}

func actHasNodeModule(baseDir, name string) bool {
	dir, err := filepath.Abs(baseDir)
	if err != nil {
		return false
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "node_modules", name)); err == nil {
			return true
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return false
		}
		dir = parent
	}
}

func actHasJSExt(name string) bool {
	ext := strings.ToLower(filepath.Ext(name))
	for _, e := range actJSExts {
		if ext == e {
			return true
		}
	}
	return false
}

// actJSResolves tells whether an import of dir/seg finds a file, as bundlers do.
func actJSResolves(dir, seg string) bool {
	full := filepath.Join(dir, seg)
	if st, err := os.Stat(full); err == nil && !st.IsDir() {
		return true
	}
	for _, e := range actJSExts {
		if actIsFile(full + e) {
			return true
		}
	}
	if actIsDir(full) && actHasIndex(full) {
		return true
	}
	// TypeScript: "./a.js" may mean a.ts
	ext := strings.ToLower(filepath.Ext(seg))
	if ext == ".js" || ext == ".jsx" || ext == ".mjs" || ext == ".cjs" {
		stem := strings.TrimSuffix(full, filepath.Ext(seg))
		for _, e := range []string{".ts", ".tsx", ".mts", ".cts"} {
			if actIsFile(stem + e) {
				return true
			}
		}
	}
	return false
}

func actIsFile(p string) bool {
	st, err := os.Stat(p)
	return err == nil && !st.IsDir()
}

func actHasIndex(dir string) bool {
	if actIsFile(filepath.Join(dir, "package.json")) {
		return true
	}
	for _, e := range actJSExts {
		if actIsFile(filepath.Join(dir, "index"+e)) {
			return true
		}
	}
	return false
}

type actEntry struct {
	name string
	dir  bool
	path string
}

func actListDir(dir string, dirsOnly bool) []actEntry {
	ents, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var out []actEntry
	for _, e := range ents {
		p := filepath.Join(dir, e.Name())
		isDir := e.IsDir()
		if e.Type()&os.ModeSymlink != 0 {
			isDir = actIsDir(p)
		}
		if dirsOnly && !isDir {
			continue
		}
		out = append(out, actEntry{e.Name(), isDir, p})
	}
	return out
}

// actBestName picks the one clearly best entry for want. Costs: same name in
// other letter case 0; another extension (or a missing one) 2, only for the
// last part; a typo of d letters 1+d. The best must beat the second best.
// jsBare: want is a JS import without extension, so it is compared with file
// names without their JS extension and with folders holding an index file;
// the answer is then also written without extension.
func actBestName(want string, ents []actEntry, last, jsBare bool) (string, bool) {
	best, second := 1<<30, 1<<30
	bestName := ""
	lw := strings.ToLower(want)
	wstem, wext := actSplitExt(lw)
	for _, e := range ents {
		if strings.HasPrefix(e.name, ".") && !strings.HasPrefix(want, ".") {
			continue
		}
		name := e.name
		cmp := strings.ToLower(name)
		if jsBare {
			if e.dir {
				if !actHasIndex(e.path) {
					continue
				}
			} else if actHasJSExt(name) {
				name = strings.TrimSuffix(name, filepath.Ext(name))
				cmp = strings.ToLower(name)
			} else {
				continue
			}
		}
		cost := -1
		cstem, cext := actSplitExt(cmp)
		switch {
		case cmp == lw:
			cost = 0
		case last && !jsBare && !e.dir && cstem == wstem && cext != wext:
			cost = 2
		default:
			var d int
			if !e.dir && !jsBare && cext == wext && wext != "" {
				d = actTypoDist(wstem, cstem)
			} else {
				d = actTypoDist(lw, cmp)
			}
			if d > 0 {
				cost = 1 + d
			}
		}
		if cost < 0 {
			continue
		}
		switch {
		case cost < best:
			second = best
			best, bestName = cost, name
		case cost < second:
			second = cost
		}
	}
	if bestName == "" || best >= second {
		return "", false
	}
	return bestName, true
}

func actSplitExt(name string) (string, string) {
	ext := filepath.Ext(name)
	if ext == name { // ".hidden"
		return name, ""
	}
	return strings.TrimSuffix(name, ext), ext
}

// actTypoDist gives the Damerau-Levenshtein distance when it is small enough
// for the names' length (1 letter for 3-5 letters, 2 for longer), else -1.
func actTypoDist(a, b string) int {
	ra, rb := []rune(a), []rune(b)
	long, short := len(ra), len(rb)
	if short > long {
		long, short = short, long
	}
	limit := 0
	switch {
	case long >= 6:
		limit = 2
	case long >= 3:
		limit = 1
	}
	if limit == 0 || long-short > limit {
		return -1
	}
	d := actDamerau(ra, rb)
	if d > limit || d >= short {
		return -1
	}
	return d
}

// actDamerau is the optimal-string-alignment distance (one swap of two
// neighbouring letters counts as one edit).
func actDamerau(a, b []rune) int {
	n, m := len(a), len(b)
	d := make([][]int, n+1)
	for i := range d {
		d[i] = make([]int, m+1)
		d[i][0] = i
	}
	for j := 0; j <= m; j++ {
		d[0][j] = j
	}
	for i := 1; i <= n; i++ {
		for j := 1; j <= m; j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			v := min(d[i-1][j]+1, d[i][j-1]+1, d[i-1][j-1]+cost)
			if i > 1 && j > 1 && a[i-1] == b[j-2] && a[i-2] == b[j-1] {
				v = min(v, d[i-2][j-2]+1)
			}
			d[i][j] = v
		}
	}
	return d[n][m]
}

// PathCandidates lists completions for the partial path that ends at pos
// inside a quoted string or url(…): the entries of the folder typed so far
// whose names start with the typed rest (ignoring letter case). Each entry is
// the whole path as it reads after completing (the typed folder part plus
// the entry's name), so the editor replaces from the path's start to pos.
// Folders end in "/". Dot-files only when a "." was typed. At most 50,
// sorted. nil when pos is not in a path or the folder does not exist.
func PathCandidates(text []rune, pos int, baseDir string) []string {
	if pos > len(text) || pos < 0 {
		return nil
	}
	s := pos
	for s > 0 {
		r := text[s-1]
		if r == '\n' || r == '"' || r == '\'' || r == '`' || r == '(' {
			break
		}
		s--
	}
	if s == 0 || text[s-1] == '\n' {
		return nil
	}
	if text[s-1] == '(' {
		for i := s; i < pos; i++ {
			if actIsSpace(text[i]) {
				return nil
			}
		}
	} else {
		// the quote must open a string: an even number of the same quote before it on the line
		q := text[s-1]
		cnt := 0
		for i := actLineStart(text, s-1); i < s-1; i++ {
			if text[i] == q {
				cnt++
			}
		}
		if cnt%2 != 0 {
			return nil
		}
	}
	typed := string(text[s:pos])
	if typed != "" && actSkipPath(typed) || strings.ContainsAny(typed, "?#") {
		return nil
	}
	dirPart, prefix := "", typed
	if i := strings.LastIndexByte(typed, '/'); i >= 0 {
		dirPart, prefix = typed[:i+1], typed[i+1:]
	}
	fsDir := dirPart
	if u, err := url.PathUnescape(fsDir); err == nil {
		fsDir = u
	}
	var dir string
	switch {
	case fsDir == "":
		dir = baseDir
	case strings.HasPrefix(fsDir, "/"):
		dir = filepath.Join(actSiteRoot(baseDir), filepath.FromSlash(fsDir))
	default:
		dir = filepath.Join(baseDir, filepath.FromSlash(fsDir))
	}
	if !actIsDir(dir) {
		return nil
	}
	lp := strings.ToLower(prefix)
	var out []string
	for _, e := range actListDir(dir, false) {
		if strings.HasPrefix(e.name, ".") && !strings.HasPrefix(prefix, ".") {
			continue
		}
		if !strings.HasPrefix(strings.ToLower(e.name), lp) {
			continue
		}
		c := dirPart + e.name
		if e.dir {
			c += "/"
		}
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool {
		a, b := strings.ToLower(out[i]), strings.ToLower(out[j])
		if a != b {
			return a < b
		}
		return out[i] < out[j]
	})
	if len(out) > 50 {
		out = out[:50]
	}
	return out
}
