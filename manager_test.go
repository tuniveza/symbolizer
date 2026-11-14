package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
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
	put("lb/names.tsv", "co\tCode\tby language\npj\tProjects\tbigger ones\nno\tNotes\nlb\tlb\nco/c/game\tMy Game\ta game in C\n", 0o644)
	put("lb/lb.conf", "IGNORE=(\n  \"*backup*\"\n)\n", 0o644)
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

	d := mgScan(root, nil, filepath.Join(root, "lb/names.tsv"))
	cats := map[string]mgCat{}
	var order []string
	for _, c := range d.Cats {
		cats[c.Key] = c
		order = append(order, c.Key)
	}
	if got := strings.Join(order, " "); got != "pj co no lb ? !" {
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
	m.mgr.data = mgScan(root, nil, filepath.Join(root, "lb/names.tsv"))
	for _, size := range [][2]int{{140, 40}, {80, 24}, {40, 12}} {
		m.width, m.height = size[0], size[1]
		_ = m.View()
	}
}
