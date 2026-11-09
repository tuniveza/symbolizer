package main

import (
	"os"
	"path/filepath"
	"testing"

	tea "charm.land/bubbletea/v2"
)

// The tabs sit in the middle of the window whichever one is chosen, and
// tab mode (Alt+Tab) puts them in the same place.
func TestTabsStayCentred(t *testing.T) {
	dir := t.TempDir()
	var paths []string
	for _, n := range []string{"a.txt", "longer-name.go", "c.md"} {
		p := filepath.Join(dir, n)
		os.WriteFile(p, []byte("x\n"), 0o644)
		paths = append(paths, p)
	}
	m, _ := NewModel(paths[0], 0)
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m.openPath(paths[1])
	m.openPath(paths[2])
	where := func() [][3]int {
		m.tabBar()
		return append([][3]int(nil), m.tabHits...)
	}
	first := where()
	if mid := (first[0][0] + first[len(first)-1][1] - 1) / 2; mid < 49 || mid > 50 {
		t.Fatalf("tabs %v: middle at %d, want the window's middle (49-50)", first, mid)
	}
	for i := range m.tabs {
		m.switchTab(i)
		if got := where(); got[0] != first[0] || got[2] != first[2] {
			t.Fatalf("with tab %d chosen the tabs moved: %v, was %v", i+1, got, first)
		}
	}
	// Tab mode: the same places.
	rows := m.tabLayout()
	for c, cell := range rows[0] {
		if cell.x != first[c][0] || cell.x+cell.w != first[c][1] {
			t.Fatalf("tab mode puts tab %d at %d-%d, the tab bar at %d-%d", c+1, cell.x, cell.x+cell.w, first[c][0], first[c][1])
		}
	}
}
