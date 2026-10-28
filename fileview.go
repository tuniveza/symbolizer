package main

import (
	"path/filepath"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// The file manager is a box floating over the editor (Alt+O, ø, Ctrl+B, or
// AltGr tapped twice). It takes the keys while it is open; Esc, opening a
// file or a click outside it closes it.

// treeWidth was the side panel's width; the file manager floats now, so
// the text always has the whole width.
func (m *Model) treeWidth() int { return 0 }

// fileIcon gives a file (or folder) its icon and colour. The icons come from
// the Nerd Font symbols kitty carries; elsewhere plain marks are used.
func fileIcon(name string, dir, open bool) (string, string) {
	if dir {
		if open {
			return icon("", "▾"), "#ffb400"
		}
		return icon("", "▸"), "#ff6600"
	}
	lower := strings.ToLower(name)
	switch lower {
	case "readme", "readme.md", "readme.txt":
		return icon("", "•"), "#ffe08a"
	case "license", "licence", "license.md", "copying":
		return icon("", "•"), "#ffc14d"
	case "makefile", "gnumakefile":
		return icon("", "•"), "#d84a00"
	case "go.mod", "go.sum":
		return icon("", "•"), "#ff9a3c"
	case ".gitignore", ".gitattributes", ".gitmodules":
		return icon("", "•"), "#ff4518"
	}
	switch strings.TrimPrefix(filepath.Ext(lower), ".") {
	case "go":
		return icon("", "•"), "#ff9a3c"
	case "py", "pyw", "pyi":
		return icon("", "•"), "#ffc14d"
	case "js", "mjs", "cjs", "jsx":
		return icon("", "•"), "#ffd23c"
	case "ts", "tsx", "mts":
		return icon("", "•"), "#ff8a1a"
	case "rs":
		return icon("", "•"), "#ff5a2a"
	case "c", "h":
		return icon("", "•"), "#ff7a4a"
	case "cpp", "cc", "cxx", "hpp", "hh":
		return icon("", "•"), "#ff7a4a"
	case "java":
		return icon("", "•"), "#ff8533"
	case "kt", "kts":
		return icon("", "•"), "#ff8533"
	case "md", "markdown", "mdx":
		return icon("", "•"), "#ffe08a"
	case "json", "jsonc":
		return icon("", "•"), "#ffb400"
	case "html", "htm", "xml", "svg":
		return icon("", "•"), "#ff6a3a"
	case "css", "scss", "less", "rasi":
		return icon("", "•"), "#ff9a5a"
	case "sh", "bash", "zsh", "fish":
		return icon("", "•"), "#ffa66b"
	case "lua":
		return icon("", "•"), "#ff8533"
	case "toml", "yaml", "yml", "conf", "cfg", "ini", "desktop", "service":
		return icon("", "•"), "#d84a00"
	case "vim":
		return icon("", "•"), "#ffb870"
	case "csv", "tsv":
		return icon("", "•"), "#ffc14d"
	case "png", "jpg", "jpeg", "gif", "webp", "bmp", "ico":
		return icon("", "•"), "#ff6a8a"
	case "pdf":
		return icon("", "•"), "#ff3a2a"
	case "zip", "gz", "xz", "zst", "tar", "7z", "rar":
		return icon("", "•"), "#c83800"
	case "lock":
		return icon("", "•"), "#ffb400"
	case "txt", "log", "rst":
		return icon("", "•"), "#ffa66b"
	case "":
		return icon("", "•"), "#d84a00" // no ending: often a program
	}
	return icon("", "•"), cStatusText
}

// fileManagerBox draws the floating file manager and remembers where its
// list is, for mouse clicks.
func (m *Model) fileManagerBox() string {
	t := m.tree
	bg := cBarBG
	w := max(30, min(84, m.width-6))
	on := func(fg, back string) lipgloss.Style {
		return lipgloss.NewStyle().Foreground(hex(fg)).Background(hex(back))
	}
	line := func(s, back string) string {
		return fit(s, w, lipgloss.NewStyle().Background(hex(back)))
	}
	openTabs := map[string]bool{}
	for i := range m.tabs {
		openTabs[m.tabBuf(i).Path] = true
	}
	cur := m.buf.Path

	var top []string
	name := filepath.Base(t.root)
	if t.root == "/" {
		name = "/"
	}
	folderIcon, _ := fileIcon("", true, true)
	top = append(top, line(glowText(" "+folderIcon+"  "+name, bg)+on(cDim, bg).Render("   "+shortPath(filepath.Dir(t.root))), bg))
	if t.query != "" {
		top = append(top, line(on(cAccent, bg).Bold(true).Render(" find: ")+on(cGlow, bg).Render(t.query+"▏"), bg))
	} else {
		top = append(top, line(on(cDim, bg).Italic(true).Render(" type letters to find a file anywhere in "+name), bg))
	}
	top = append(top, line(on(cGutterLine, bg).Render(" "+strings.Repeat("─", w-2)), bg))

	hint := " Enter open · ← up · Ctrl+N new file · F7 folder · F2 rename · Del trash · Esc close"
	if lipgloss.Width(hint) > w {
		hint = " Enter open · ← up · Esc close · F1 more"
	}
	bottom := []string{line(on(cGutterLine, bg).Render(" "+strings.Repeat("─", w-2)), bg), line(on(cDim, bg).Italic(true).Render(hint), bg)}

	n := len(t.rows)
	if t.query != "" {
		n = len(t.found)
	}
	listH := max(3, min(max(n, 6), m.height-4-2-len(top)-len(bottom)))
	if t.sel < t.scroll {
		t.scroll = t.sel
	}
	if t.sel >= t.scroll+listH {
		t.scroll = t.sel - listH + 1
	}
	var list []string
	for i := t.scroll; i < t.scroll+listH; i++ {
		if i >= n {
			list = append(list, line("", bg))
			continue
		}
		rowBG := bg
		if i == t.sel {
			rowBG = cSelection
		}
		var s string
		if t.query != "" {
			p := t.found[i]
			rel, _ := filepath.Rel(t.root, p)
			ic, col := fileIcon(filepath.Base(p), false, false)
			s = on(col, rowBG).Render(" "+ic+"  ") + m.fileName(filepath.Base(p), p, cur, openTabs, rowBG, false)
			if d := filepath.Dir(rel); d != "." {
				s += on(cDim, rowBG).Render("   " + d)
			}
		} else {
			nd := t.rows[i]
			if nd == t.top {
				up := icon("", "‹")
				s = on(cDim, rowBG).Render(" " + up + "  up a folder")
				if len(t.rows) == 1 {
					s = on(cDim, rowBG).Render(" (empty folder)")
				}
			} else {
				indent := strings.Repeat("   ", nd.depth-1)
				ic, col := fileIcon(nd.name, nd.dir, nd.open)
				s = on(col, rowBG).Render(" "+indent+ic+"  ") + m.fileName(nd.name, nd.path, cur, openTabs, rowBG, nd.dir)
			}
		}
		list = append(list, line(s, rowBG))
	}

	body := strings.Join(append(append(top, list...), bottom...), "\n")
	box := stHelpBox.Background(hex(bg)).BorderBackground(hex(bg)).Padding(0, 1).Render(body)
	// where the list sits on screen, for mouse clicks (the box is centred by overlay)
	bw, bh := lipgloss.Width(box), lipgloss.Height(box)
	m.fmBox = [4]int{max(0, (m.width-bw)/2), max(1, (m.height-bh)/2), bw, bh}
	m.fmListTop = m.fmBox[1] + 1 + len(top)
	m.fmListH = listH
	return box
}

// fileName draws a name: open files are bright, the one on screen gold.
func (m *Model) fileName(name, path, cur string, openTabs map[string]bool, back string, dir bool) string {
	st := lipgloss.NewStyle().Background(hex(back)).Foreground(hex(cStatusText))
	switch {
	case path == cur:
		st = st.Foreground(hex(cAccent)).Bold(true)
	case openTabs[path]:
		st = st.Foreground(hex(cGlow))
	case dir:
		st = st.Foreground(hex(cEmber)).Bold(true)
	case strings.HasPrefix(name, "."):
		st = st.Foreground(hex(cDim))
	}
	if dir {
		name += "/"
	}
	mark := ""
	if path == cur {
		mark = lipgloss.NewStyle().Background(hex(back)).Foreground(hex(cAccent)).Render("  ◆")
	} else if openTabs[path] {
		mark = lipgloss.NewStyle().Background(hex(back)).Foreground(hex(cDim)).Render("  ●")
	}
	return st.Render(name) + mark
}

// treeMouse handles a click while the file manager is open: inside picks a
// row (a second click on it opens it), outside closes the box.
func (m *Model) treeMouse(x, y int) tea.Cmd {
	b := m.fmBox
	if x < b[0] || x >= b[0]+b[2] || y < b[1] || y >= b[1]+b[3] {
		m.closeTree()
		return nil
	}
	t := m.tree
	if y < m.fmListTop || y >= m.fmListTop+m.fmListH {
		return nil
	}
	i := t.scroll + y - m.fmListTop
	n := len(t.rows)
	if t.query != "" {
		n = len(t.found)
	}
	if i >= n {
		return nil
	}
	again := i == t.sel
	t.sel = i
	if again {
		return m.treeKey(tea.Key{Code: tea.KeyEnter})
	}
	return nil
}

func (m *Model) closeTree() {
	m.showTree, m.treeFocus = false, false
}
