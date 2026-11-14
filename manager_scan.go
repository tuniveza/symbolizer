package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// The Manager's scan: one walk over ~/an (and ~/Projects) that sorts every
// folder into its category, names it from ~/an/ca/sy/manager/names.tsv (the
// same list the manager program uses), finds the projects in it and adds up sizes, file counts and the
// last change. It runs in the background; the last result is kept in
// ~/.cache/symbolizer/manager.json so the Manager opens at once.

// mgItem is one row of the Manager's list: a project, a folder or a file.
type mgItem struct {
	Rel     string   `json:"rel"`  // path from ~/an ("~/Projects/x" outside it)
	Name    string   `json:"name"` // from names.tsv, or the folder's name
	About   string   `json:"about,omitempty"`
	Kind    string   `json:"kind,omitempty"` // Go, Godot, C, PDF, folder …
	Dir     bool     `json:"dir,omitempty"`
	Group   bool     `json:"group,omitempty"`   // a folder straight inside the category
	Project bool     `json:"project,omitempty"` // has an executable make.sh
	Reload  bool     `json:"reload,omitempty"`  // has reload.sh
	Depth   int      `json:"depth,omitempty"`   // how far it is drawn in
	Size    int64    `json:"size"`
	Files   int      `json:"files"`
	Changed int64    `json:"changed,omitempty"` // unix seconds: the newest file
	Git     string   `json:"git,omitempty"`     // branch
	Commit  string   `json:"commit,omitempty"`  // the last commit's message
	When    int64    `json:"when,omitempty"`    // the last commit's time
	Readme  string   `json:"readme,omitempty"`  // the README's first sentence
	Inside  []string `json:"inside,omitempty"`  // the most common file types
}

// mgCat is one category: a top folder of ~/an.
type mgCat struct {
	Key      string   `json:"key"` // co, pj …, "~/Projects", "?" not named, "!" leftovers
	Name     string   `json:"name"`
	About    string   `json:"about,omitempty"`
	Size     int64    `json:"size"`
	Files    int      `json:"files"`
	Changed  int64    `json:"changed,omitempty"`
	Projects int      `json:"projects"`
	Items    []mgItem `json:"items"`
}

type mgData struct {
	Root    string  `json:"root"`
	At      int64   `json:"at"`   // when the scan finished
	Took    float64 `json:"took"` // seconds
	Size    int64   `json:"size"`
	Files   int     `json:"files"`
	Cats    []mgCat `json:"cats"`
	Missing bool    `json:"missing,omitempty"` // names.tsv was not found
}

// ---- names.tsv ------------------------------------------------------------------------

type mgName struct{ name, about string }

// mgNames reads names.tsv: "path<TAB>name<TAB>about". Paths are from ~/an,
// or start with ~/; "*/fs" names a folder called fs anywhere.
func mgNames(path string) (map[string]mgName, map[string]mgName, []string, bool) {
	exact, wild := map[string]mgName{}, map[string]mgName{}
	var order []string // top-level keys in the order the file gives them
	f, err := os.Open(path)
	if err != nil {
		return exact, wild, order, false
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		if strings.HasPrefix(strings.TrimSpace(line), "#") || strings.TrimSpace(line) == "" {
			continue
		}
		parts := strings.Split(line, "\t")
		if len(parts) < 2 {
			continue
		}
		p := strings.TrimRight(strings.TrimSpace(parts[0]), "/")
		n := mgName{name: strings.TrimSpace(parts[1])}
		if len(parts) > 2 {
			n.about = strings.TrimSpace(parts[2])
		}
		if strings.HasPrefix(p, "*/") {
			wild[p[2:]] = n
			continue
		}
		exact[p] = n
		if !strings.Contains(p, "/") || p == "~/Projects" {
			order = append(order, p)
		}
	}
	return exact, wild, order, true
}

// mgIgnores reads the IGNORE list from manager.conf: folders of copies,
// backups and other people's code. Their insides are not listed (only what
// names.tsv names); the folder itself is one row. The Manager adds the
// fixer-jobs copies wherever they are.
func mgIgnores(conf string) []string {
	out := []string{"*fixer-jobs*", "*backup*", "*/old", "*/archive"}
	b, err := os.ReadFile(conf)
	if err != nil {
		return out
	}
	s := string(b)
	i := strings.Index(s, "IGNORE=(")
	if i < 0 {
		return out
	}
	s = s[i+len("IGNORE=("):]
	if j := strings.IndexByte(s, ')'); j >= 0 {
		s = s[:j]
	}
	for _, l := range strings.Split(s, "\n") {
		if k := strings.IndexByte(l, '#'); k >= 0 {
			l = l[:k]
		}
		for _, f := range strings.Fields(l) {
			out = append(out, strings.Trim(f, `"'`))
		}
	}
	return out
}

// mgIgnored: is rel (a path from ~/an) one of the ignored folders?
func mgIgnored(rel string, ignores []string) bool {
	for _, g := range ignores {
		if strings.ContainsAny(g, "*?") {
			// like find -path: * matches across slashes too
			re := "^" + strings.NewReplacer(`\*`, ".*", `\?`, ".").Replace(regexp.QuoteMeta(g)) + "$"
			if ok, _ := regexp.MatchString(re, rel); ok {
				return true
			}
			continue
		}
		if rel == g {
			return true
		}
	}
	return false
}

// ---- the walk -------------------------------------------------------------------------

// marker bits: what a folder holds straight inside it.
const (
	mkMake uint32 = 1 << iota
	mkReload
	mkGodot
	mkDotnet
	mkLime
	mkGleam
	mkCrystal
	mkGo
	mkRust
	mkGradle
	mkPackage
	mkWrangler
	mkToc
	mkOdin
	mkV
	mkMainLua
	mkMakefile
	mkCMake
	mkAsm
	mkCpp
	mkC
	mkPy
	mkIndex
	mkGit
	mkAndroid
	mkMainRb
	mkHaxe
)

// strong markers make a folder a project of its own, even without make.sh.
const mkStrong = mkGodot | mkDotnet | mkLime | mkGleam | mkCrystal | mkGo | mkRust | mkGradle | mkPackage

func mgMarker(name string, d fs.DirEntry) uint32 {
	switch name {
	case "make.sh":
		if info, err := d.Info(); err == nil && info.Mode()&0o100 != 0 {
			return mkMake
		}
	case "reload.sh":
		return mkReload
	case "project.godot":
		return mkGodot
	case "Project.xml":
		return mkLime
	case "gleam.toml":
		return mkGleam
	case "shard.yml":
		return mkCrystal
	case "go.mod":
		return mkGo
	case "Cargo.toml":
		return mkRust
	case "settings.gradle", "settings.gradle.kts", "build.gradle", "build.gradle.kts":
		return mkGradle
	case "AndroidManifest.xml":
		return mkAndroid
	case "package.json":
		return mkPackage
	case "main.lua":
		return mkMainLua
	case "Makefile", "makefile", "GNUmakefile":
		return mkMakefile
	case "CMakeLists.txt":
		return mkCMake
	case "index.html":
		return mkIndex
	case "v.mod":
		return mkV
	case "main.rb":
		return mkMainRb
	}
	switch filepath.Ext(name) {
	case ".csproj":
		return mkDotnet
	case ".toc":
		return mkToc
	case ".odin":
		return mkOdin
	case ".asm":
		return mkAsm
	case ".cpp", ".cc", ".cxx":
		return mkCpp
	case ".c":
		return mkC
	case ".py":
		return mkPy
	case ".hx":
		return mkHaxe
	case ".v":
		return mkV
	}
	if strings.HasPrefix(name, "wrangler.") {
		return mkWrangler
	}
	return 0
}

// mgSkip: folders whose files are counted in sizes but are tools' output,
// so they never make "last changed" newer and never hold listed projects.
var mgSkip = map[string]bool{".git": true, "node_modules": true, "__pycache__": true, ".godot": true,
	".gradle": true, ".cache": true, "target": true, ".venv": true, "venv": true, "dist": true,
	"build": true, ".idea": true, "deps": true, "vendor": true, "vendored": true, ".import": true,
	"bin": true, "obj": true, "out": true, ".dart_tool": true, ".next": true}

type mgDir struct {
	path    string
	parent  int // index into dirs, -1 for a root
	skip    bool
	size    int64
	files   int
	newest  int64
	own     uint32 // markers straight inside
	deep    uint32 // C/C++/Python anywhere below (for the kind)
	readme  string // path of a README straight inside
	topKey  string // which top child it is under, for the file-type count
	topSelf bool   // it is a top child
	subKey  string // which child of a top child it is under
	subSelf bool
}

// mgScan walks root (~/an) and extra roots (~/Projects) and builds the
// categories. namesPath is ~/an/ca/sy/manager/names.tsv; manager.conf is next to it.
func mgScan(root string, extra []string, namesPath string) *mgData {
	start := time.Now()
	exact, wild, order, found := mgNames(namesPath)
	mgrDir := filepath.Dir(namesPath) // ~/an/ca/sy/manager: never listed
	ignores := mgIgnores(filepath.Join(mgrDir, "manager.conf"))
	home, _ := os.UserHomeDir()

	var dirs []mgDir
	index := map[string]int{}
	exts := map[string]map[string]int{} // top child → file type → count
	topFiles := map[string][]fs.DirEntry{}

	walk := func(base string) {
		_ = filepath.WalkDir(base, func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				if d != nil && d.IsDir() {
					return fs.SkipDir
				}
				return nil
			}
			if d.IsDir() {
				parent, ok := index[filepath.Dir(p)]
				if !ok || p == base {
					parent = -1
				}
				md := mgDir{path: p, parent: parent}
				if parent >= 0 {
					pd := dirs[parent]
					md.skip = pd.skip || mgSkip[d.Name()] || p == mgrDir // the manager's own folder: its templates are not projects
					md.topKey, md.subKey = pd.topKey, pd.subKey
					if pd.parent == -1 {
						md.topKey, md.topSelf = p, true
					} else if pd.topSelf {
						md.subKey, md.subSelf = p, true
					}
				}
				index[p] = len(dirs)
				dirs = append(dirs, md)
				return nil
			}
			i, ok := index[filepath.Dir(p)]
			if !ok {
				return nil
			}
			md := &dirs[i]
			name := d.Name()
			md.own |= mgMarker(name, d)
			if md.readme == "" && strings.HasPrefix(strings.ToLower(name), "readme") {
				md.readme = p
			}
			if md.parent == -1 {
				topFiles[md.path] = append(topFiles[md.path], d)
			}
			if d.Type()&fs.ModeSymlink != 0 {
				return nil
			}
			info, err := d.Info()
			if err != nil {
				return nil
			}
			md.size += info.Size()
			md.files++
			if !md.skip {
				if t := info.ModTime().Unix(); t > md.newest {
					md.newest = t
				}
			}
			if !md.skip {
				k := mgFileKind(name)
				for _, key := range []string{md.topKey, md.subKey} {
					if key == "" {
						continue
					}
					if exts[key] == nil {
						exts[key] = map[string]int{}
					}
					exts[key][k]++
				}
			}
			return nil
		})
	}
	walk(root)
	for _, e := range extra {
		if st, err := os.Stat(e); err == nil && st.IsDir() {
			walk(e)
		}
	}
	// .git folders are skipped above for markers; note them on their parent.
	for i := range dirs {
		if filepath.Base(dirs[i].path) == ".git" && dirs[i].parent >= 0 {
			dirs[dirs[i].parent].own |= mkGit
		}
	}
	// add each folder into its parent: walk order puts parents first.
	for i := len(dirs) - 1; i >= 0; i-- {
		d := &dirs[i]
		if d.own&(mkC|mkCpp|mkPy) != 0 {
			d.deep |= d.own & (mkC | mkCpp | mkPy)
		}
		if d.parent >= 0 {
			p := &dirs[d.parent]
			p.size += d.size
			p.files += d.files
			p.newest = max(p.newest, d.newest)
			if !d.skip {
				p.deep |= d.deep
				if d.own&mkAndroid != 0 {
					p.own |= mkAndroid
				}
			}
		}
	}

	rel := func(p string) string {
		if r, err := filepath.Rel(root, p); err == nil && !strings.HasPrefix(r, "..") {
			return r
		}
		if home != "" && strings.HasPrefix(p, home+"/") {
			return "~/" + p[len(home)+1:]
		}
		return p
	}
	nameOf := func(r string) (mgName, bool) {
		if n, ok := exact[r]; ok {
			return n, true
		}
		if n, ok := wild[filepath.Base(r)]; ok {
			return n, false
		}
		return mgName{name: filepath.Base(r)}, false
	}
	item := func(d *mgDir) mgItem {
		r := rel(d.path)
		n, _ := nameOf(r)
		it := mgItem{Rel: r, Name: n.name, About: n.about, Dir: true, Size: d.size, Files: d.files,
			Changed: d.newest, Project: d.own&mkMake != 0, Reload: d.own&mkReload != 0, Kind: mgKind(d)}
		if d.own&mkGit != 0 {
			it.Git, it.Commit, it.When = mgGit(d.path)
		}
		if d.readme != "" {
			it.Readme = mgReadme(d.readme)
		}
		if d.topSelf || d.subSelf {
			it.Inside = mgTop(exts[d.path], 3)
		}
		return it
	}

	data := &mgData{Root: root, Missing: !found}
	// the category roots: each top folder of ~/an, and the extra roots
	var tops []int
	for i := range dirs {
		if dirs[i].parent == -1 && dirs[i].path != root {
			tops = append(tops, i) // an extra root (~/Projects)
		}
		if dirs[i].parent >= 0 && dirs[dirs[i].parent].path == root {
			tops = append(tops, i)
		}
	}
	if i, ok := index[root]; ok {
		data.Size, data.Files = dirs[i].size, dirs[i].files
		for _, e := range extra {
			if j, ok := index[e]; ok {
				data.Size += dirs[j].size
				data.Files += dirs[j].files
			}
		}
	}

	// projectRoots are the categories whose projects are listed (the ROOTS in manager.conf).
	projectRoots := map[string]bool{"co": true, "pj": true, "ca": true}
	for _, e := range extra {
		projectRoots[rel(e)] = true
	}

	var notNamed, leftovers []mgItem
	for _, ti := range tops {
		td := &dirs[ti]
		key := rel(td.path)
		if strings.HasPrefix(filepath.Base(key), ".") {
			continue
		}
		n, named := exact[key]
		if !named {
			it := item(td)
			if mgLeftover(td.path) {
				it.About = "holds only a run folder (sockets for cinder, pulse …): a program made it, not you"
				it.Kind = "leftover"
				leftovers = append(leftovers, it)
			} else {
				it.About = "not in names.tsv yet"
				notNamed = append(notNamed, it)
			}
			continue
		}
		cat := mgCat{Key: key, Name: n.name, About: n.about, Size: td.size, Files: td.files, Changed: td.newest}
		if projectRoots[key] {
			cat.Items = mgProjects(dirs, ti, rel, exact, item, ignores)
		} else {
			cat.Items = mgChildren(dirs, ti, index, td.path, item, nameOf)
		}
		for _, it := range cat.Items {
			if it.Project {
				cat.Projects++
			}
		}
		data.Cats = append(data.Cats, cat)
	}
	// projects outside the project roots (brick in br …): one more category
	var others []mgItem
	for _, ti := range tops {
		if projectRoots[rel(dirs[ti].path)] || strings.HasPrefix(filepath.Base(dirs[ti].path), ".") {
			continue
		}
		others = append(others, mgOthers(dirs, ti, rel, item, ignores)...)
	}
	if len(others) > 0 {
		c := mgCat{Key: "+", Name: "Other projects", About: "projects outside pj, ca and co: each is also in its own category"}
		for _, it := range others {
			c.Size += it.Size
			c.Files += it.Files
			c.Changed = max(c.Changed, it.Changed)
			c.Projects++
		}
		c.Items = others
		data.Cats = append(data.Cats, c)
	}
	// loose files straight inside ~/an
	if ri, ok := index[root]; ok {
		for _, f := range topFiles[dirs[ri].path] {
			if strings.HasPrefix(f.Name(), ".") {
				continue
			}
			it := mgItem{Rel: f.Name(), Name: f.Name(), Kind: mgFileKind(f.Name()), Files: 1, About: "a loose file straight in ~/an"}
			if info, err := f.Info(); err == nil {
				it.Size, it.Changed = info.Size(), info.ModTime().Unix()
			}
			notNamed = append(notNamed, it)
		}
	}
	// the order names.tsv gives, then any others
	rank := map[string]int{}
	for i, k := range order {
		rank[k] = i
	}
	sort.SliceStable(data.Cats, func(i, j int) bool {
		ri, iok := rank[data.Cats[i].Key]
		rj, jok := rank[data.Cats[j].Key]
		if iok != jok {
			return iok
		}
		return ri < rj
	})
	// projects first: pj, ca, co, then the rest in names.tsv's order
	first := map[string]int{"pj": 0, "ca": 1, "co": 2, "~/Projects": 3, "+": 4}
	sort.SliceStable(data.Cats, func(i, j int) bool {
		fi, iok := first[data.Cats[i].Key]
		fj, jok := first[data.Cats[j].Key]
		if iok != jok {
			return iok
		}
		return iok && fi < fj
	})
	add := func(key, name, about string, items []mgItem) {
		if len(items) == 0 {
			return
		}
		c := mgCat{Key: key, Name: name, About: about, Items: items}
		for _, it := range items {
			c.Size += it.Size
			c.Files += it.Files
			c.Changed = max(c.Changed, it.Changed)
		}
		data.Cats = append(data.Cats, c)
	}
	add("?", "Not named yet", "top folders of ~/an that names.tsv does not name", notNamed)
	add("!", "Leftovers", "folders a program made by mistake in ~/an; nothing of yours is in them", leftovers)
	data.At = time.Now().Unix()
	data.Took = time.Since(start).Seconds()
	return data
}

// mgProjects lists a project root (co, pj, ca, ~/Projects): every folder
// straight inside it as a heading, and under those every project (a
// make.sh, a strong build file, or a line in names.tsv).
func mgProjects(dirs []mgDir, top int, rel func(string) string, exact map[string]mgName, item func(*mgDir) mgItem, ignores []string) []mgItem {
	var out []mgItem
	ignored := map[int]bool{} // folders of copies: inside them only named folders are listed
	listed := map[int]int{}   // dir index → depth it was listed at
	listed[top] = -1
	topPath := dirs[top].path
	for i := top + 1; i < len(dirs); i++ {
		d := &dirs[i]
		if !inFolder(topPath, d.path) {
			break // walk order: a folder's insides come straight after it
		}
		if d.skip || strings.HasPrefix(filepath.Base(d.path), ".") {
			ignored[i] = ignored[d.parent] // so nothing under a hidden folder of copies shows
			continue
		}
		// the nearest listed folder above it, and whether that is a project
		depth, inProject := 0, false
		for a := d.parent; a >= 0; a = dirs[a].parent {
			if dd, ok := listed[a]; ok {
				depth = dd + 1
				break
			}
		}
		for a := d.parent; a >= 0 && a != top; a = dirs[a].parent {
			if _, ok := listed[a]; ok && dirs[a].own&(mkMake|mkStrong) != 0 {
				inProject = true
				break
			}
		}
		r := rel(d.path)
		_, named := exact[r]
		group := d.parent == top
		if ignored[d.parent] {
			ignored[i] = true
			if !named {
				continue
			}
		}
		copies := !ignored[i] && mgIgnored(r, ignores)
		if copies {
			ignored[i] = true
		}
		project := d.own&mkMake != 0 && !ignored[i]
		strong := d.own&mkStrong != 0 && !inProject && !ignored[i]
		if !(group || named || project || strong || copies) {
			continue
		}
		listed[i] = depth
		it := item(d)
		it.Group, it.Depth = group, depth
		if strong {
			it.Project = true // a project of its own without make.sh (go.mod, Cargo.toml …): sy is one
		}
		if copies {
			it.Project, it.Kind = false, "copies"
			if it.About == "" {
				it.About = "copies, backups or someone else's code: what is inside is not listed"
			}
		}
		out = append(out, it)
	}
	return out
}

// mgOthers finds the projects under a top folder that is not a project
// root: the top folder itself or the first folders down that have make.sh or
// a strong build file. Nothing inside a project found is looked at again.
func mgOthers(dirs []mgDir, top int, rel func(string) string, item func(*mgDir) mgItem, ignores []string) []mgItem {
	var out []mgItem
	topPath := dirs[top].path
	stop := map[int]bool{} // found projects and ignored folders: their insides are not looked at
	for i := top; i < len(dirs); i++ {
		d := &dirs[i]
		if i > top && !inFolder(topPath, d.path) {
			break // walk order: a folder's insides come straight after it
		}
		if i > top && (stop[d.parent] || d.skip || strings.HasPrefix(filepath.Base(d.path), ".")) {
			stop[i] = true
			continue
		}
		if mgIgnored(rel(d.path), ignores) {
			stop[i] = true
			continue
		}
		if d.own&(mkMake|mkStrong) == 0 {
			continue
		}
		it := item(d)
		it.Project = true
		out = append(out, it)
		stop[i] = true
	}
	return out
}

// mgChildren lists any other category: what is straight inside it.
func mgChildren(dirs []mgDir, top int, index map[string]int, topPath string, item func(*mgDir) mgItem, nameOf func(string) (mgName, bool)) []mgItem {
	entries, err := os.ReadDir(topPath)
	if err != nil {
		return nil
	}
	var out []mgItem
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".") {
			continue
		}
		p := filepath.Join(topPath, e.Name())
		if i, ok := index[p]; ok && e.IsDir() {
			it := item(&dirs[i])
			it.Group = true
			out = append(out, it)
			continue
		}
		r, _ := filepath.Rel(filepath.Dir(dirs[top].path), p)
		n, _ := nameOf(r)
		it := mgItem{Rel: r, Name: n.name, About: n.about, Kind: mgFileKind(e.Name()), Files: 1}
		if info, err := os.Stat(p); err == nil {
			it.Size, it.Changed = info.Size(), info.ModTime().Unix()
			if info.IsDir() {
				it.Dir, it.Kind = true, "link"
			}
		}
		out = append(out, it)
	}
	return out
}

// mgLeftover: a folder that holds nothing but "run" (a runtime folder a
// program made in the wrong place).
func mgLeftover(p string) bool {
	es, err := os.ReadDir(p)
	return err == nil && len(es) == 1 && es[0].Name() == "run" && es[0].IsDir()
}

// mgKind says what kind of project a folder is, the way the manager program does.
func mgKind(d *mgDir) string {
	o := d.own
	switch {
	case o&mkGodot != 0:
		return "Godot"
	case o&mkDotnet != 0:
		return "C# (.NET)"
	case o&mkLime != 0:
		return "Haxe"
	case o&mkGleam != 0:
		return "Gleam"
	case o&mkCrystal != 0:
		return "Crystal"
	case o&mkGo != 0:
		return "Go"
	case o&mkRust != 0:
		return "Rust"
	case o&mkGradle != 0:
		if o&mkAndroid != 0 {
			return "Android"
		}
		return "Java (Gradle)"
	case o&mkPackage != 0:
		if o&mkWrangler != 0 {
			return "Web worker"
		}
		return "JavaScript"
	case o&mkToc != 0:
		return "WoW add-on"
	case o&mkOdin != 0:
		return "Odin"
	case o&mkV != 0 && o&mkMainLua == 0:
		return "V"
	case o&mkMainLua != 0:
		return "Lua"
	case o&mkHaxe != 0:
		return "Haxe"
	case o&mkMainRb != 0:
		return "Ruby"
	case o&mkAsm != 0:
		return "Assembly"
	}
	// C, C++ and Python files further down only name a project's kind
	deep := o & (mkC | mkCpp | mkPy)
	if o&(mkMake|mkMakefile|mkCMake) != 0 {
		deep |= d.deep
	}
	switch {
	case o&mkCMake != 0 || deep&mkCpp != 0:
		return "C++"
	case deep&mkC != 0 || o&mkMakefile != 0:
		return "C"
	case deep&mkPy != 0:
		return "Python"
	case o&mkIndex != 0:
		return "Web page"
	}
	return ""
}

// mgFileKind names a file by what it is, in a word.
func mgFileKind(name string) string {
	ext := strings.ToLower(filepath.Ext(name))
	switch ext {
	case ".png", ".jpg", ".jpeg", ".gif", ".webp", ".svg", ".bmp", ".aseprite", ".ase", ".kra", ".psd":
		return "picture"
	case ".mp4", ".mkv", ".webm", ".mov", ".avi":
		return "video"
	case ".mp3", ".wav", ".ogg", ".flac", ".opus", ".m4a":
		return "sound"
	case ".pdf":
		return "PDF"
	case ".zip", ".tar", ".gz", ".xz", ".7z", ".rar", ".zst", ".bz2":
		return "archive"
	case ".iso", ".img":
		return "disk image"
	case ".txt", ".md", ".org", ".rst":
		return "text"
	case ".gltf", ".glb", ".obj", ".fbx", ".blend":
		return "3D model"
	case ".ttf", ".otf", ".woff", ".woff2":
		return "font"
	case ".json", ".toml", ".yaml", ".yml", ".ini", ".conf", ".xml", ".tsv", ".csv":
		return "data"
	case ".so", ".a", ".o", ".dll", ".exe", ".appimage", ".x86_64", ".pck", ".bin":
		return "program"
	case ".sh", ".py", ".c", ".h", ".cpp", ".hpp", ".go", ".js", ".ts", ".lua", ".rs", ".java", ".kt",
		".cs", ".hx", ".rb", ".asm", ".s", ".odin", ".v", ".cr", ".gleam", ".scm", ".html", ".css", ".gd":
		return "code"
	}
	if ext == "" {
		return "other"
	}
	e := ext[1:]
	if strings.HasPrefix(name, ".") || len(e) > 5 || strings.ContainsAny(e, "0123456789-_") {
		return "other"
	}
	return e
}

// mgTop: the n most common file types, biggest first.
func mgTop(counts map[string]int, n int) []string {
	type kc struct {
		k string
		c int
	}
	var all []kc
	for k, c := range counts {
		all = append(all, kc{k, c})
	}
	sort.Slice(all, func(i, j int) bool {
		if all[i].c != all[j].c {
			return all[i].c > all[j].c
		}
		return all[i].k < all[j].k
	})
	var out []string
	for i := 0; i < len(all) && i < n; i++ {
		out = append(out, all[i].k)
	}
	return out
}

// mgGit reads the branch and the last commit straight from .git, with no
// git program.
func mgGit(dir string) (branch, msg string, when int64) {
	head, err := os.ReadFile(filepath.Join(dir, ".git", "HEAD"))
	if err != nil {
		return "", "", 0
	}
	h := strings.TrimSpace(string(head))
	branch = strings.TrimPrefix(h, "ref: refs/heads/")
	if branch == h && len(h) >= 7 {
		branch = h[:7]
	}
	log, err := os.ReadFile(filepath.Join(dir, ".git", "logs", "HEAD"))
	if err != nil {
		return branch, "", 0
	}
	lines := strings.Split(strings.TrimRight(string(log), "\n"), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		// "<old> <new> Name <mail> 1731600000 +0000\tcommit: message"
		l := lines[i]
		tab := strings.IndexByte(l, '\t')
		if tab < 0 {
			continue
		}
		head, what := l[:tab], l[tab+1:]
		if !strings.HasPrefix(what, "commit") && !strings.HasPrefix(what, "merge") {
			continue
		}
		f := strings.Fields(head)
		if len(f) >= 2 {
			when, _ = strconv.ParseInt(f[len(f)-2], 10, 64)
		}
		if c := strings.Index(what, ": "); c >= 0 {
			what = what[c+2:]
		}
		return branch, what, when
	}
	return branch, "", 0
}

// mgReadme: the first sentence of a README that is words, not a heading,
// a picture or a badge.
func mgReadme(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	buf := make([]byte, 8192)
	n, _ := f.Read(buf)
	for _, l := range strings.Split(string(buf[:n]), "\n") {
		l = strings.TrimSpace(l)
		if l == "" || strings.ContainsAny(l[:1], "#<![`|>-=*_~") || strings.HasPrefix(l, "```") {
			continue
		}
		l = strings.NewReplacer("**", "", "__", "", "`", "").Replace(l)
		if len([]rune(l)) > 160 {
			l = string([]rune(l)[:157]) + "…"
		}
		return l
	}
	return ""
}

// ---- the kept copy ----------------------------------------------------------------------

func mgCachePath() string {
	dir, err := os.UserCacheDir()
	if err != nil {
		return ""
	}
	return filepath.Join(dir, "symbolizer", "manager.json")
}

func mgLoad(path string) *mgData {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var d mgData
	if json.Unmarshal(b, &d) != nil {
		return nil
	}
	return &d
}

func mgSave(path string, d *mgData) {
	if path == "" || d == nil {
		return
	}
	b, err := json.Marshal(d)
	if err != nil {
		return
	}
	_ = os.MkdirAll(filepath.Dir(path), 0o755)
	tmp := path + ".tmp"
	if os.WriteFile(tmp, b, 0o644) == nil {
		_ = os.Rename(tmp, path)
	}
}

// mgSize: a size in words people read: 3.1 GB, 820 KB.
func mgSize(n int64) string {
	const k = 1024
	switch {
	case n >= k*k*k:
		return fmt.Sprintf("%.1f GB", float64(n)/(k*k*k))
	case n >= k*k:
		return fmt.Sprintf("%.0f MB", float64(n)/(k*k))
	case n >= k:
		return fmt.Sprintf("%.0f KB", float64(n)/k)
	}
	return fmt.Sprintf("%d B", n)
}
