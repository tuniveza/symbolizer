package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

// a small pretend ~/an: categories from names.tsv, projects, copies, leftovers
func TestManagerScan(t *testing.T) {
	root := t.TempDir()
	put := func(p, text string, mode os.FileMode) {
		p = filepath.Join(root, p)
		os.MkdirAll(filepath.Dir(p), 0o755)
		if err := os.WriteFile(p, []byte(text), mode); err != nil {
			t.Fatal(err)
		}
	}
	put("ca/sy/manager/names.tsv", "co\tCode\tby language\npj\tProjects\tbigger ones\nno\tNotes\nca\tApps\nbr\tbrick\nco/c/game\tMy Game\ta game in C\n", 0o644)
	put("ca/sy/manager/manager.conf", "IGNORE=(\n  \"*backup*\" pj/keepout ca/sy/manager\n)\n", 0o644)
	put("ca/sy/make.sh", "#!/bin/sh\n", 0o755)
	put("ca/sy/manager/templates/c/make.sh", "#!/bin/sh\n", 0o755) // a template, not a project
	put("ca/sy/manager/bin/manager", "#!/bin/sh\n", 0o755)
	put("pj/keepout/inner/package.json", "{}", 0o644)
	put("co/c/game/make.sh", "#!/bin/sh\n", 0o755)
	put("co/c/game/main.c", "int main(){}\n", 0o644)
	put("co/c/game/README.md", "# Game\n\nA small game about embers.\n", 0o644)
	put("pj/web/package.json", "{}", 0o644)
	put("pj/web-backup/package.json", "{}", 0o644)
	put("pj/web-backup/inner/package.json", "{}", 0o644)
	put("no/list.txt", "eggs", 0o644)
	put("no/paper.pdf", "%PDF", 0o644)
	os.MkdirAll(filepath.Join(root, "s1/run/pulse"), 0o755)
	put("odd/thing.sh", "echo", 0o644)
	put("ca/tool/go.mod", "module tool\n", 0o644)   // a project without make.sh (as sy was)
	put("br/make.sh", "#!/bin/sh\n", 0o755)         // a project outside pj, ca, co
	put("br/inner/make.sh", "#!/bin/sh\n", 0o755)   // inside it: not listed again
	put("no/side/Cargo.toml", "[package]\n", 0o644) // deeper down in another folder

	d := mgScan(root, nil, filepath.Join(root, "ca/sy/manager/names.tsv"))
	cats := map[string]mgCat{}
	var order []string
	for _, c := range d.Cats {
		cats[c.Key] = c
		order = append(order, c.Key)
	}
	if got := strings.Join(order, " "); got != "pj ca co + no br ? !" {
		t.Fatalf("category order = %q", got)
	}
	find := func(key, rel string) *mgItem {
		for i, it := range cats[key].Items {
			if it.Rel == rel {
				return &cats[key].Items[i]
			}
		}
		return nil
	}
	g := find("co", "co/c/game")
	if g == nil || g.Name != "My Game" || !g.Project || g.Kind != "C" || g.Readme != "A small game about embers." || g.Depth != 1 {
		t.Fatalf("game = %+v", g)
	}
	if w := find("pj", "pj/web"); w == nil || w.Kind != "JavaScript" {
		t.Fatalf("web = %+v", w)
	}
	if b := find("pj", "pj/web-backup"); b == nil || b.Kind != "copies" || b.Project {
		t.Fatalf("backup = %+v", b)
	}
	if k := find("pj", "pj/keepout"); k == nil || k.Kind != "copies" || find("pj", "pj/keepout/inner") != nil {
		t.Fatalf("manager.conf's IGNORE is not used: %+v", k)
	}
	if find("ca", "ca/sy") == nil {
		t.Fatalf("ca/sy is not listed: %+v", cats["ca"].Items)
	}
	for _, it := range cats["ca"].Items {
		if strings.HasPrefix(it.Rel, "ca/sy/manager") {
			t.Fatalf("the manager's own folder is listed: %+v", it)
		}
	}
	if tl := find("ca", "ca/tool"); tl == nil || !tl.Project || tl.Kind != "Go" {
		t.Fatalf("a go.mod folder is a project: %+v", tl)
	}
	var other []string
	for _, it := range cats["+"].Items {
		other = append(other, it.Rel)
		if !it.Project {
			t.Fatalf("other project not marked: %+v", it)
		}
	}
	if got := strings.Join(other, " "); got != "br no/side" || cats["+"].Projects != 2 {
		t.Fatalf("other projects = %q", got)
	}
	if find("pj", "pj/web-backup/inner") != nil {
		t.Fatal("a project inside copies is listed")
	}
	if p := find("no", "no/paper.pdf"); p == nil || p.Kind != "PDF" {
		t.Fatalf("pdf = %+v", p)
	}
	if l := find("!", "s1"); l == nil || l.Kind != "leftover" {
		t.Fatalf("leftover = %+v", cats["!"].Items)
	}
	if o := find("?", "odd"); o == nil {
		t.Fatalf("not named = %+v", cats["?"].Items)
	}
	if cats["co"].Projects != 1 || d.Files < 10 {
		t.Fatalf("co projects %d, files %d", cats["co"].Projects, d.Files)
	}

	// kept and read back
	keep := filepath.Join(root, "cache.json")
	mgSave(keep, d)
	if back := mgLoad(keep); back == nil || len(back.Cats) != len(d.Cats) || back.Files != d.Files {
		t.Fatal("the kept scan did not read back")
	}
}

func TestManagerSearchAndSort(t *testing.T) {
	s := &mgState{data: &mgData{Cats: []mgCat{
		{Key: "co", Name: "Code", Items: []mgItem{{Rel: "co/a", Name: "Alpha", Size: 5, Changed: 1}, {Rel: "co/b", Name: "Farm Simulator", Size: 9, Changed: 3}}},
		{Key: "pj", Name: "Projects", Items: []mgItem{{Rel: "pj/f", Name: "Farm Simulator (Godot)", Kind: "Godot", Size: 1, Changed: 2}}},
	}}}
	s.query = "farm"
	if r := s.rows(); len(r) != 2 {
		t.Fatalf("search farm found %d", len(r))
	}
	s.query = "farm godot"
	if r := s.rows(); len(r) != 1 || r[0].cat != 1 {
		t.Fatalf("search farm godot = %+v", r)
	}
	s.query, s.sort = "", 1
	if r := s.rows(); r[0].it.Name != "Farm Simulator" {
		t.Fatalf("biggest first = %s", r[0].it.Name)
	}
	if mgCount(587571) != "587,571" || mgSize(3*1024*1024*1024) != "3.0 GB" {
		t.Fatal("number words")
	}
}

// the mode draws at any size without a panic, with and without a scan
func TestManagerDraws(t *testing.T) {
	m := newTest(t, "")
	m.mode = managerMode
	m.mgr = &mgState{}
	for _, size := range [][2]int{{140, 40}, {80, 24}, {40, 12}, {200, 60}} {
		m.width, m.height = size[0], size[1]
		_ = m.View()
	}
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, "co/c"), 0o755)
	m.mgr.data = mgScan(root, nil, filepath.Join(root, "ca/sy/manager/names.tsv"))
	for _, size := range [][2]int{{140, 40}, {80, 24}, {40, 12}} {
		m.width, m.height = size[0], size[1]
		_ = m.View()
	}
}

// Up / Down choose a category; Right goes into it, and then Up / Down
// choose a row; Left (or Esc) comes back out to the categories.
func TestManagerArrows(t *testing.T) {
	m := newTest(t, "")
	m.mode = managerMode
	m.width, m.height = 140, 40
	m.mgr = &mgState{data: &mgData{Cats: []mgCat{
		{Key: "pj", Name: "Projects", Items: []mgItem{{Rel: "pj/a", Name: "A"}, {Rel: "pj/b", Name: "B"}}},
		{Key: "ca", Name: "Apps", Items: []mgItem{{Rel: "ca/sy", Name: "Symbolizer"}, {Rel: "ca/x", Name: "X"}, {Rel: "ca/y", Name: "Y"}}},
		{Key: "co", Name: "Code"},
	}}}
	s := m.mgr
	key := func(k string) {
		t.Helper()
		press(m, k)
		_ = m.View()
	}
	key("down")
	key("down")
	key("down") // stops on the last
	if s.cat != 2 || s.inList {
		t.Fatalf("down x3: category %d (in rows %v), want 2", s.cat, s.inList)
	}
	key("right") // an empty category: stays out
	if s.inList {
		t.Fatal("went into an empty category")
	}
	key("up")
	key("right")
	if s.cat != 1 || !s.inList || s.sel != 0 {
		t.Fatalf("up, right: category %d, in rows %v, row %d", s.cat, s.inList, s.sel)
	}
	key("down")
	key("down")
	if s.cat != 1 || s.sel != 2 {
		t.Fatalf("in the rows, down x2: category %d row %d, want 1 and 2", s.cat, s.sel)
	}
	if r := s.picked(); r == nil || r.it.Rel != "ca/y" {
		t.Fatalf("picked = %+v", r)
	}
	key("left")
	if s.inList || s.cat != 1 || s.picked() != nil {
		t.Fatalf("left: in rows %v, category %d", s.inList, s.cat)
	}
	key("enter") // Enter on a category goes in too
	if !s.inList {
		t.Fatal("Enter on a category did not go in")
	}
	key("esc")
	if s.inList {
		t.Fatal("Esc did not come back out")
	}
	for _, r := range "sy" { // a search: the arrows are on what it found
		m.Update(tea.KeyPressMsg(tea.Key{Code: r, Text: string(r)}))
	}
	if s.query != "sy" || s.picked() == nil || s.picked().it.Rel != "ca/sy" {
		t.Fatalf("search %q picked %+v", s.query, s.picked())
	}
	key("left")
	if s.query != "" || s.inList || s.cat != 1 {
		t.Fatalf("left after a search: query %q, in rows %v, category %d", s.query, s.inList, s.cat)
	}
	for _, size := range [][2]int{{140, 40}, {60, 20}} { // both layouts draw both ways
		m.width, m.height = size[0], size[1]
		key("right")
		key("left")
	}
}

// Folders outside ~/an with other people's code (~/.local/src) or an old
// copy of home (~/recovered_files): each is a category, listing what is
// straight inside and the git repositories and projects below, but never
// the hundreds of go.mod files in a Go cache.
func TestManagerOutside(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	put := func(p, text string) {
		p = filepath.Join(home, p)
		os.MkdirAll(filepath.Dir(p), 0o755)
		if err := os.WriteFile(p, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	put("an/ca/sy/manager/names.tsv", "ca\tApps\n~/.local/src\tDownloaded source\n~/recovered_files\tRecovered files\n")
	put("an/ca/x.txt", "x")
	put(".local/src/zathura/meson.build", "")
	put(".local/src/zathura/src/inner/CMakeLists.txt", "") // inside a project: not again
	put(".local/src/tarballs/lib-1.0/CMakeLists.txt", "")
	put(".local/src/tarballs/lib-1.0.tar.gz", "")
	put("recovered_files/Hyprland-Dots/.git/HEAD", "ref: refs/heads/main\n")
	put("recovered_files/go/pkg/mod/github.com/x/go.mod", "module x\n") // a cache, too far down
	put("recovered_files/Music/song.mp3", "")
	var extra []string
	for _, r := range mgOutsideRoots {
		extra = append(extra, filepath.Join(home, r))
	}
	d := mgScan(filepath.Join(home, "an"), extra, filepath.Join(home, "an/ca/sy/manager/names.tsv"))
	got := map[string]string{}
	var order []string
	for _, c := range d.Cats {
		order = append(order, c.Key)
		var rows []string
		for _, it := range c.Items {
			mark := ""
			if it.Project {
				mark = "◆"
			}
			rows = append(rows, mark+it.Rel)
		}
		got[c.Key] = strings.Join(rows, " ")
	}
	if o := strings.Join(order, " "); o != "ca ~/recovered_files ~/.local/src" {
		t.Fatalf("category order = %q", o)
	}
	if g := got["~/.local/src"]; g != "◆~/.local/src/tarballs/lib-1.0 ~/.local/src/tarballs ◆~/.local/src/zathura" && g != "~/.local/src/tarballs ◆~/.local/src/tarballs/lib-1.0 ◆~/.local/src/zathura" {
		t.Fatalf("downloaded source = %q", g)
	}
	if g := got["~/recovered_files"]; g != "◆~/recovered_files/Hyprland-Dots ~/recovered_files/Music ~/recovered_files/go" {
		t.Fatalf("recovered files = %q", g)
	}
}
