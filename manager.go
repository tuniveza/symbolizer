package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/mattn/go-runewidth"
)

// Manager mode: the whole of ~/an on one screen, sorted into its categories
// (the top folders, named by ~/an/lb/names.tsv), with every project and what
// it is, its size, its last change and its git state. The categories are on
// the left, what is in the chosen one in the middle, and everything known
// about the chosen row on the right.
//
//	Up / Down        choose a row (PgUp, PgDn, Home, End)
//	Left / Right     the category before / after
//	type             search everything; Backspace, Esc clear it
//	Tab              sort: by folder, biggest first, newest first
//	Enter            open it in Text mode (a folder: the file manager on it)
//	Shift + Enter    open it in the desktop's own app (Dolphin, a PDF viewer …)
//	Alt + C          copy its path
//	F5               look at the folders again
//
// The scan runs in the background (manager_scan.go); the last one is kept,
// so the screen is there at once.

const managerMode = 6 // its place in modes

type mgState struct {
	data      *mgData
	scanning  bool
	cat       int // the chosen category
	sel, top  int // the chosen row and the first row shown
	query     string
	sort      int // 0 by folder, 1 biggest first, 2 newest first
	lastClick time.Time

	// where the last frame put things, for the mouse
	catY0, catX1, listY0, listX0, listX1, listH int
	narrow                                      bool
}

type mgRow struct {
	it  *mgItem
	cat int
}

type mgScanMsg struct {
	data *mgData
	err  string
}

var mgSorts = []string{"by folder", "biggest first", "newest first"}

// mgRoot is the folder the Manager shows: ~/an (SY_AN changes it, for tests).
func mgRoot() string {
	if r := os.Getenv("SY_AN"); r != "" {
		return r
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "an")
}

func (m *Model) inManager() bool { return m.mode == managerMode }

// enterManager shows the kept scan at once and starts a fresh one.
func (m *Model) enterManager() tea.Cmd {
	if m.mgr == nil {
		m.mgr = &mgState{data: mgLoad(mgCachePath())}
		if d := m.mgr.data; d != nil && d.Root != mgRoot() {
			m.mgr.data = nil // kept from another folder
		}
	}
	m.showTree, m.treeFocus = false, false
	return m.mgScanCmd()
}

func (m *Model) mgScanCmd() tea.Cmd {
	if m.mgr == nil || m.mgr.scanning {
		return nil
	}
	m.mgr.scanning = true
	root := mgRoot()
	home, _ := os.UserHomeDir()
	extra := []string{filepath.Join(home, "Projects")}
	if os.Getenv("SY_AN") != "" {
		extra = nil
	}
	return func() (msg tea.Msg) {
		defer func() {
			if r := recover(); r != nil { // a scan that goes wrong never takes the editor down
				msg = mgScanMsg{err: fmt.Sprint(r)}
			}
		}()
		d := mgScan(root, extra, filepath.Join(root, "lb", "names.tsv"))
		mgSave(mgCachePath(), d)
		return mgScanMsg{data: d}
	}
}

// mgScanned takes a finished scan, keeping the same row chosen.
func (m *Model) mgScanned(d *mgData, failed string) {
	s := m.mgr
	if s == nil {
		return
	}
	s.scanning = false
	if d == nil {
		m.sayErr("The Manager could not look at the folders: " + failed)
		return
	}
	keepCat, keepRow := "", ""
	if s.data != nil && s.cat < len(s.data.Cats) {
		keepCat = s.data.Cats[s.cat].Key
		if rows := s.rows(); s.sel < len(rows) {
			keepRow = rows[s.sel].it.Rel
		}
	}
	s.data = d
	s.cat, s.sel = 0, 0
	for i, c := range d.Cats {
		if c.Key == keepCat {
			s.cat = i
		}
	}
	for i, r := range s.rows() {
		if r.it.Rel == keepRow {
			s.sel = i
		}
	}
	if m.inManager() {
		m.say(fmt.Sprintf("%s  Looked at %s files in %.1f seconds.", modes[managerMode].Icon(), mgCount(d.Files), d.Took))
	}
}

// rows is what the middle column lists: the chosen category, or the
// search's matches in every category, in the chosen order.
func (s *mgState) rows() []mgRow {
	if s.data == nil || len(s.data.Cats) == 0 {
		return nil
	}
	var out []mgRow
	if q := strings.ToLower(strings.TrimSpace(s.query)); q != "" {
		for ci := range s.data.Cats {
			c := &s.data.Cats[ci]
			for i := range c.Items {
				it := &c.Items[i]
				hay := strings.ToLower(it.Name + " " + it.About + " " + it.Rel + " " + it.Kind + " " + it.Readme + " " + c.Name)
				if mgMatch(hay, q) {
					out = append(out, mgRow{it, ci})
				}
			}
		}
	} else {
		s.cat = max(0, min(s.cat, len(s.data.Cats)-1))
		c := &s.data.Cats[s.cat]
		for i := range c.Items {
			out = append(out, mgRow{&c.Items[i], s.cat})
		}
	}
	switch s.sort {
	case 1:
		sort.SliceStable(out, func(i, j int) bool { return out[i].it.Size > out[j].it.Size })
	case 2:
		sort.SliceStable(out, func(i, j int) bool { return out[i].it.Changed > out[j].it.Changed })
	}
	return out
}

// mgMatch: every word of the search is somewhere in hay.
func mgMatch(hay, q string) bool {
	for _, w := range strings.Fields(q) {
		if !strings.Contains(hay, w) {
			return false
		}
	}
	return true
}

func (s *mgState) chosen() *mgRow {
	rows := s.rows()
	if s.sel >= 0 && s.sel < len(rows) {
		return &rows[s.sel]
	}
	return nil
}

// mgPath is where an item is on disk.
func mgPath(root string, it *mgItem) string {
	if strings.HasPrefix(it.Rel, "~/") {
		home, _ := os.UserHomeDir()
		return filepath.Join(home, it.Rel[2:])
	}
	if filepath.IsAbs(it.Rel) {
		return it.Rel
	}
	return filepath.Join(root, it.Rel)
}

// ---- keys -------------------------------------------------------------------------------

func (m *Model) managerKey(k tea.Key) tea.Cmd {
	if m.mgr == nil {
		if cmd := m.enterManager(); cmd != nil {
			return cmd
		}
	}
	s := m.mgr
	name := keyName(k)
	n := len(s.rows())
	page := max(s.listH-1, 1)
	m.msg = ""
	switch name {
	case "up":
		s.sel = max(s.sel-1, 0)
	case "down":
		s.sel = min(s.sel+1, max(n-1, 0))
	case "pgup":
		s.sel = max(s.sel-page, 0)
	case "pgdown":
		s.sel = min(s.sel+page, max(n-1, 0))
	case "home":
		s.sel = 0
	case "end":
		s.sel = max(n-1, 0)
	case "left", "shift+tab":
		m.mgCategory(-1)
	case "right":
		m.mgCategory(1)
	case "tab":
		s.sort = (s.sort + 1) % len(mgSorts)
		s.sel, s.top = 0, 0
		m.say("Sorted " + mgSorts[s.sort] + ".")
	case "enter":
		return m.mgOpen(false)
	case "shift+enter":
		return m.mgOpen(true)
	case "alt+c":
		if r := s.chosen(); r != nil {
			p := mgPath(mgRoot(), r.it)
			m.say("Copied " + shortPath(p))
			return writeClipboard(p)
		}
	case "f5", "ctrl+r":
		m.say("Looking at the folders again…")
		return m.mgScanCmd()
	case "f1":
		m.help = !m.help
	case "esc":
		if s.query != "" {
			s.query, s.sel, s.top = "", 0, 0
		} else {
			m.say("Alt+M switches modes · Alt+Q quits")
		}
	case "backspace":
		if r := []rune(s.query); len(r) > 0 {
			s.query = string(r[:len(r)-1])
			s.sel, s.top = 0, 0
		}
	case "ctrl+u", "alt+backspace":
		s.query, s.sel, s.top = "", 0, 0
	default:
		if k.Text != "" && !k.Mod.Contains(tea.ModCtrl) && !k.Mod.Contains(tea.ModAlt) && (name == "space" || len([]rune(name)) == 1 || strings.HasPrefix(name, "shift+")) {
			s.query += k.Text
			s.sel, s.top = 0, 0
		}
	}
	return nil
}

func (m *Model) mgCategory(step int) {
	s := m.mgr
	if s.data == nil || len(s.data.Cats) == 0 {
		return
	}
	if s.query != "" {
		s.query = "" // leave the search for the categories
	} else {
		s.cat = (s.cat + step + len(s.data.Cats)) % len(s.data.Cats)
	}
	s.sel, s.top = 0, 0
}

// mgOpen opens the chosen row: a folder in Text mode with the file manager on
// it (and its README), a text file as a tab; or, with desktop, in the
// desktop's own app.
func (m *Model) mgOpen(desktop bool) tea.Cmd {
	r := m.mgr.chosen()
	if r == nil {
		return nil
	}
	p := mgPath(mgRoot(), r.it)
	info, err := os.Stat(p)
	if err != nil {
		m.sayErr("It is not there any more: " + shortPath(p) + ". F5 looks again.")
		return nil
	}
	if desktop || (!info.IsDir() && !looksText(p)) {
		if !have("xdg-open") {
			m.sayErr("No xdg-open here to open " + shortPath(p) + ".")
			return nil
		}
		c := exec.Command("xdg-open", p)
		c.Stdout, c.Stderr = nil, nil
		if err := c.Start(); err != nil {
			m.sayErr("Could not open: " + err.Error())
			return nil
		}
		go c.Wait()
		m.say("Opened " + shortPath(p) + " in its own app.")
		return nil
	}
	m.setMode(0)
	if !info.IsDir() {
		return m.openPath(p)
	}
	var cmd tea.Cmd
	for _, rd := range []string{"README.md", "README", "README.txt", "readme.md"} {
		if fileExists(filepath.Join(p, rd)) {
			cmd = m.openPath(filepath.Join(p, rd))
			break
		}
	}
	m.setRoot(p)
	m.openTree(false)
	m.say(r.it.Name + ": the file manager is on " + shortPath(p) + ". Alt+M then 7 goes back to the Manager.")
	return cmd
}

// ---- the mouse --------------------------------------------------------------------------

func (m *Model) managerClick(x, y int) tea.Cmd {
	s := m.mgr
	if s == nil || m.help {
		return nil
	}
	if !s.narrow && x < s.catX1 && y >= s.catY0 && s.data != nil {
		if i := y - s.catY0; i >= 0 && i < len(s.data.Cats) {
			s.query = ""
			s.cat, s.sel, s.top = i, 0, 0
		}
		return nil
	}
	if x >= s.listX0 && x < s.listX1 && y >= s.listY0 && y < s.listY0+s.listH {
		i := s.top + y - s.listY0
		if i < len(s.rows()) {
			again := i == s.sel && time.Since(s.lastClick) < 400*time.Millisecond
			s.sel, s.lastClick = i, time.Now()
			if again {
				return m.mgOpen(false)
			}
		}
	}
	return nil
}

func (m *Model) managerWheel(n int) {
	if s := m.mgr; s != nil {
		s.sel = max(0, min(s.sel+n, len(s.rows())-1))
	}
}

// ---- words ------------------------------------------------------------------------------

// mgAgo says how long ago, in words: 3 days ago.
func mgAgo(unix int64) string {
	if unix == 0 {
		return ""
	}
	d := time.Since(time.Unix(unix, 0))
	unit := func(n int, one string) string {
		if n == 1 {
			return "1 " + one + " ago"
		}
		return fmt.Sprintf("%d %ss ago", n, one)
	}
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return unit(int(d.Minutes()), "minute")
	case d < 24*time.Hour:
		return unit(int(d.Hours()), "hour")
	case d < 14*24*time.Hour:
		return unit(int(d.Hours()/24), "day")
	case d < 60*24*time.Hour:
		return unit(int(d.Hours()/24/7), "week")
	case d < 365*24*time.Hour:
		return unit(int(d.Hours()/24/30), "month")
	}
	return unit(int(d.Hours()/24/365), "year")
}

// mgCount: 589311 → 589,311.
func mgCount(n int) string {
	s := fmt.Sprint(n)
	for i := len(s) - 3; i > 0; i -= 3 {
		s = s[:i] + "," + s[i:]
	}
	return s
}

func mgFit(s string, w int) string {
	if w <= 0 {
		return ""
	}
	if runewidth.StringWidth(s) <= w {
		return s
	}
	return runewidth.Truncate(s, w, "…")
}

// ---- drawing ----------------------------------------------------------------------------

// seg is a piece of a row: its text, colours and width (0: as wide as the text).
type seg struct {
	text, fg, bg string
	w            int
	bold, italic bool
	right        bool
}

func mgRender(segs ...seg) string {
	var sb strings.Builder
	for _, s := range segs {
		t := s.text
		if s.w > 0 {
			t = mgFit(t, s.w)
		}
		st := lipgloss.NewStyle().Foreground(hex(s.fg)).Background(hex(s.bg)).Bold(s.bold).Italic(s.italic)
		if s.w > 0 {
			st = st.Width(s.w)
			if s.right {
				st = st.Align(lipgloss.Right)
			}
		}
		sb.WriteString(st.Render(t))
	}
	return sb.String()
}

// mgPad makes a styled line exactly w wide on bg.
func mgPad(line string, w int, bg string) string {
	lw := lipgloss.Width(line)
	if lw >= w {
		return line
	}
	return line + lipgloss.NewStyle().Background(hex(bg)).Render(strings.Repeat(" ", w-lw))
}

// managerRows draws the Manager in the text area: textHeight rows.
func (m *Model) managerRows(sb *strings.Builder) {
	if m.mgr == nil {
		m.mgr = &mgState{data: mgLoad(mgCachePath())}
	}
	s := m.mgr
	h := m.textHeight()
	var lines []string
	if s.data == nil {
		lines = m.mgWaiting(h)
	} else if m.width >= 96 {
		lines = m.mgWide(h)
	} else {
		lines = m.mgNarrow(h)
	}
	for i := 0; i < h; i++ {
		l := ""
		if i < len(lines) {
			l = lines[i]
		}
		sb.WriteString(mgPad(l, m.width, cBG))
		sb.WriteByte('\n')
	}
}

func (m *Model) mgWaiting(h int) []string {
	body := []string{
		mgRender(seg{text: modes[managerMode].Icon() + "  Manager", fg: cEmber, bg: cBG, bold: true}),
		"",
		mgRender(seg{text: "Looking at every folder in " + shortPath(mgRoot()) + " …", fg: cStatusText, bg: cBG}),
		mgRender(seg{text: "The first time takes a few seconds. After that it shows at once.", fg: cDim, bg: cBG, italic: true}),
	}
	block := lipgloss.Place(m.width, h, lipgloss.Center, lipgloss.Center, strings.Join(body, "\n"),
		lipgloss.WithWhitespaceStyle(lipgloss.NewStyle().Background(hex(cBG))))
	return strings.Split(block, "\n")
}

// mgHeader is the first row: everything in a line.
func (m *Model) mgHeader() string {
	d := m.mgr.data
	projects := 0
	for _, c := range d.Cats {
		projects += c.Projects
	}
	when := "looked " + mgAgo(d.At)
	if m.mgr.scanning {
		when = "looking again…"
	}
	dot := seg{text: "  ·  ", fg: cGutterLine, bg: cBG}
	parts := []seg{
		{text: " " + modes[managerMode].Icon() + "  ", fg: cEmber, bg: cBG, bold: true},
		{text: shortPath(d.Root), fg: cEmber, bg: cBG, bold: true}, dot,
		{text: fmt.Sprintf("%d projects", projects), fg: cAccent, bg: cBG, bold: true}, dot,
		{text: mgSize(d.Size), fg: cStatusText, bg: cBG}, dot,
		{text: mgCount(d.Files) + " files", fg: cStatusText, bg: cBG}, dot,
		{text: when, fg: cDim, bg: cBG, italic: true},
	}
	if d.Missing {
		parts = append(parts, dot, seg{text: "no lb/names.tsv: folders keep their own names", fg: cWarning, bg: cBG})
	}
	return mgFit2(mgRender(parts...), m.width)
}

// mgFit2 cuts a styled line to w cells.
func mgFit2(line string, w int) string {
	if lipgloss.Width(line) <= w {
		return line
	}
	return lipgloss.NewStyle().MaxWidth(w).Render(line)
}

func (m *Model) mgHint() string {
	s := m.mgr
	sortWord := mgSorts[s.sort]
	full := "↑↓ choose · ←→ category · type to search · Tab sort: " + sortWord + " · Enter open · Shift+Enter its own app · Alt+C copy path · F5 look again"
	mid := "↑↓ · ←→ category · type: search · Tab: " + sortWord + " · Enter open · F5 again"
	short := "↑↓ ←→ · type · Tab · Enter · F5"
	t := full
	for _, c := range []string{full, mid, short} {
		t = c
		if runewidth.StringWidth(c)+2 <= m.width {
			break
		}
	}
	return lipgloss.NewStyle().Width(m.width).Align(lipgloss.Center).Background(hex(cBG)).
		Foreground(hex(cDim)).Italic(true).Render(mgFit(t, m.width))
}

// mgWide: categories | list | what the chosen row is.
func (m *Model) mgWide(h int) []string {
	s := m.mgr
	s.narrow = false
	catW := 26
	detW := max(min(m.width*31/100, 56), 34)
	listW := m.width - catW - detW - 6
	colH := max(h-3, 3)
	s.catY0, s.catX1 = textTop+2, 1+catW
	s.listX0, s.listX1 = 1+catW+2, 1+catW+2+listW
	cats := m.mgCats(catW, colH)
	list := m.mgList(listW, colH, textTop+2)
	det := m.mgDetail(detW, colH)
	gap := lipgloss.NewStyle().Background(hex(cBG)).Render("  ")
	edge := lipgloss.NewStyle().Background(hex(cBG)).Render(" ")
	out := []string{m.mgHeader(), ""}
	for i := 0; i < colH; i++ {
		out = append(out, edge+cats[i]+gap+list[i]+gap+det[i]+edge)
	}
	return append(out, m.mgHint())
}

// mgNarrow: a category line, the list, and a few lines about the chosen row.
func (m *Model) mgNarrow(h int) []string {
	s := m.mgr
	s.narrow = true
	w := m.width - 2
	detH := 5
	if h < 14 {
		detH = 2
	}
	listH := max(h-3-detH, 2)
	c := s.data.Cats[max(0, min(s.cat, len(s.data.Cats)-1))]
	label := fmt.Sprintf("◀  %s  %d/%d  ▶", c.Name, s.cat+1, len(s.data.Cats))
	if s.query != "" {
		label = "⌕  search: " + s.query + "▏"
	}
	catLine := lipgloss.NewStyle().Width(m.width).Align(lipgloss.Center).Background(hex(cBG)).
		Foreground(hex(cEmber)).Bold(true).Render(mgFit(label, m.width))
	s.listX0, s.listX1 = 1, 1+w
	list := m.mgList(w, listH, textTop+2)
	edge := lipgloss.NewStyle().Background(hex(cBG)).Render(" ")
	out := []string{m.mgHeader(), catLine}
	for _, l := range list {
		out = append(out, edge+l+edge)
	}
	det := m.mgDetail(w, detH+1)
	for _, l := range det[:detH] {
		out = append(out, edge+l+edge)
	}
	return append(out, m.mgHint())
}

// mgCats is the category column.
func (m *Model) mgCats(w, h int) []string {
	s := m.mgr
	var out []string
	for i, c := range s.data.Cats {
		bg, fg, mark := cBG, cText, "  "
		if i == s.cat && s.query == "" {
			bg, fg, mark = cSelection, cSelectedText, "▸ "
		}
		size := mgSize(c.Size)
		out = append(out, mgRender(
			seg{text: mark, fg: cEmber, bg: bg, bold: true},
			seg{text: c.Name, fg: fg, bg: bg, w: w - 2 - 9, bold: i == s.cat},
			seg{text: size, fg: cDim, bg: bg, w: 9, right: true},
		))
	}
	out = append(out, "")
	if s.query != "" {
		out = append(out, mgRender(seg{text: "⌕ " + s.query + "▏", fg: cEmber, bg: cBG, w: w, bold: true}),
			mgRender(seg{text: fmt.Sprintf("%d found in everything", len(s.rows())), fg: cDim, bg: cBG, w: w, italic: true}))
	} else {
		out = append(out, mgRender(seg{text: "⌕ type to search", fg: cDim, bg: cBG, w: w, italic: true}))
	}
	if s.cat < len(s.data.Cats) && s.query == "" {
		c := s.data.Cats[s.cat]
		out = append(out, "")
		for _, l := range wrapWords(c.About, w) {
			out = append(out, mgRender(seg{text: l, fg: cStatusText, bg: cBG, w: w, italic: true}))
		}
		info := fmt.Sprintf("%d rows", len(c.Items))
		if c.Projects > 0 {
			info = fmt.Sprintf("%d projects · %d rows", c.Projects, len(c.Items))
		}
		out = append(out, mgRender(seg{text: info, fg: cDim, bg: cBG, w: w}))
		if c.Changed > 0 {
			out = append(out, mgRender(seg{text: "changed " + mgAgo(c.Changed), fg: cDim, bg: cBG, w: w}))
		}
	}
	return mgColumn(out, w, h, cBG)
}

// mgColumn makes lines exactly w wide and h tall.
func mgColumn(lines []string, w, h int, bg string) []string {
	out := make([]string, h)
	for i := range out {
		l := ""
		if i < len(lines) {
			l = mgFit2(lines[i], w)
		}
		out[i] = mgPad(l, w, bg)
	}
	return out
}

// mgList is the middle column: a heading and the rows.
func (m *Model) mgList(w, h, y0 int) []string {
	s := m.mgr
	rows := s.rows()
	bodyH := max(h-1, 1)
	s.listY0, s.listH = y0+1, bodyH
	s.sel = max(0, min(s.sel, len(rows)-1))
	if s.sel < s.top {
		s.top = s.sel
	}
	if s.sel >= s.top+bodyH {
		s.top = s.sel - bodyH + 1
	}
	s.top = max(0, min(s.top, max(len(rows)-bodyH, 0)))

	kindW, sizeW, agoW := 11, 9, 14
	if w < 56 {
		agoW = 0
	}
	if w < 44 {
		kindW = 0
	}
	nameW := w - kindW - sizeW - agoW
	head := []seg{{text: "  Name", fg: cDim, bg: cBG, w: nameW}}
	if kindW > 0 {
		head = append(head, seg{text: "Kind", fg: cDim, bg: cBG, w: kindW})
	}
	head = append(head, seg{text: "Size", fg: cDim, bg: cBG, w: sizeW, right: true})
	if agoW > 0 {
		head = append(head, seg{text: "Changed", fg: cDim, bg: cBG, w: agoW, right: true})
	}
	if s.query == "" && s.sort != 0 {
		head[0].text = "  Name · " + mgSorts[s.sort]
	}
	out := []string{mgRender(head...)}
	if len(rows) == 0 {
		msg := "Nothing in here."
		if s.query != "" {
			msg = "Nothing matches “" + s.query + "”. Esc clears the search."
		}
		out = append(out, "", mgRender(seg{text: "  " + msg, fg: cDim, bg: cBG, w: w, italic: true}))
		return mgColumn(out, w, h, cBG)
	}
	flat := s.query != "" || s.sort != 0
	for i := s.top; i < min(len(rows), s.top+bodyH); i++ {
		r := rows[i]
		it := r.it
		bg := cBG
		if i == s.sel {
			bg = cSelection
		}
		mark, mfg := "· ", cDim
		fg := cText
		switch {
		case it.Project:
			mark, mfg, fg = "◆ ", cEmber, cEmber
		case it.Dir && it.Group:
			mark, mfg, fg = "▸ ", cAccent, cAccent
		case it.Dir:
			mark, mfg = "▹ ", cAccent
		}
		if i == s.sel {
			fg = cSelectedText
		}
		indent := ""
		if !flat {
			indent = strings.Repeat("  ", min(it.Depth, 6))
		}
		name := it.Name
		if s.query != "" {
			name += "  · " + s.data.Cats[r.cat].Name
		}
		kind := mgKindWord(it)
		segs := []seg{
			{text: " " + indent, fg: cDim, bg: bg},
			{text: mark, fg: mfg, bg: bg, bold: true},
			{text: name, fg: fg, bg: bg, w: max(nameW-3-len(indent), 1), bold: it.Group || it.Project},
		}
		if kindW > 0 {
			segs = append(segs, seg{text: kind, fg: cDim, bg: bg, w: kindW})
		}
		segs = append(segs, seg{text: mgSize(it.Size), fg: cStatusText, bg: bg, w: sizeW, right: true})
		if agoW > 0 {
			segs = append(segs, seg{text: mgAgo(it.Changed), fg: cDim, bg: bg, w: agoW, right: true})
		}
		out = append(out, mgPad(mgRender(segs...), w, bg))
	}
	if len(rows) > bodyH {
		// a small "3/40" in the heading's corner says there is more
		out[0] = mgRender(append(head[:len(head)-1:len(head)-1], seg{text: fmt.Sprintf("%d/%d", s.sel+1, len(rows)), fg: cAccent, bg: cBG, w: head[len(head)-1].w, right: true})...)
	}
	return mgColumn(out, w, h, cBG)
}

// mgDetail is the right column: everything about the chosen row.
func (m *Model) mgDetail(w, h int) []string {
	s := m.mgr
	bg := cBarBG
	in := w - 4
	var out []string
	line := func(segs ...seg) { out = append(out, mgRender(append([]seg{{text: "  ", bg: bg}}, segs...)...)) }
	r := s.chosen()
	if r == nil {
		line(seg{text: "Nothing chosen.", fg: cDim, bg: bg, italic: true})
		return mgColumn(append([]string{""}, out...), w, h, bg)
	}
	it := r.it
	out = append(out, "")
	for _, l := range wrapWords(it.Name, in) {
		line(seg{text: l, fg: cEmber, bg: bg, bold: true, w: in})
	}
	about := it.About
	if about == "" {
		about = it.Readme
	}
	for i, l := range wrapWords(about, in) {
		if i == 3 {
			break
		}
		line(seg{text: l, fg: cStatusText, bg: bg, w: in})
	}
	out = append(out, "")
	labelW := 9
	field := func(label, value, fg string) {
		if value == "" {
			return
		}
		for i, l := range wrapWords(value, max(in-labelW, 8)) {
			lab := ""
			if i == 0 {
				lab = label
			}
			line(seg{text: lab, fg: cDim, bg: bg, w: labelW}, seg{text: l, fg: fg, bg: bg, w: in - labelW})
		}
	}
	p := mgPath(s.data.Root, it)
	field("Where", shortPath(p), cAccent)
	field("Kind", mgKindWord(it), cText)
	switch {
	case it.Project && it.Reload:
		field("Builds", "./make.sh ✓  ./reload.sh ✓", cText)
	case it.Project:
		field("Builds", "./make.sh ✓", cText)
	case it.Kind != "" && it.Dir && r.cat < len(s.data.Cats) && mgProjectCat(s.data.Cats[r.cat].Key):
		field("Builds", "no make.sh yet (lb adopt gives it one)", cDim)
	}
	if it.Dir {
		field("Size", mgSize(it.Size)+" in "+mgCount(it.Files)+" files", cText)
	} else {
		field("Size", mgSize(it.Size), cText)
	}
	if it.Changed > 0 {
		field("Changed", mgAgo(it.Changed)+" · "+time.Unix(it.Changed, 0).Format("Mon 2 Jan 2006"), cText)
	}
	if len(it.Inside) > 0 {
		field("Inside", "mostly "+strings.Join(it.Inside, ", "), cText)
	}
	if it.Git != "" {
		g := it.Git
		if it.When > 0 {
			g += " · last commit " + mgAgo(it.When)
		}
		field("Git", g, cText)
		if it.Commit != "" {
			field("", "“"+it.Commit+"”", cDim)
		}
	}
	if it.Readme != "" && it.About != "" {
		out = append(out, "")
		for i, l := range wrapWords(it.Readme, in) {
			if i == 3 {
				break
			}
			line(seg{text: l, fg: cDim, bg: bg, w: in, italic: true})
		}
	}
	// the keys, at the bottom
	keys := [][2]string{{"Enter", "open in Text mode"}, {"Shift+Enter", "open in its own app"}, {"Alt+C", "copy the path"}}
	if !it.Dir && !looksTextName(it.Rel) {
		keys[0][1] = "open in its own app"
	}
	var foot []string
	for _, k := range keys {
		foot = append(foot, mgRender(seg{text: "  ", bg: bg}, seg{text: k[0], fg: cAccent, bg: bg, w: 13, bold: true}, seg{text: k[1], fg: cDim, bg: bg, w: in - 13}))
	}
	if len(out)+len(foot)+1 <= h {
		for len(out) < h-len(foot)-1 {
			out = append(out, "")
		}
		out = append(out, foot...)
	}
	return mgColumn(out, w, h, bg)
}

func mgProjectCat(key string) bool {
	return key == "co" || key == "pj" || key == "ca" || key == "~/Projects"
}

// looksTextName guesses from the name alone (the detail panel draws often,
// so it never opens the file).
func looksTextName(name string) bool {
	switch mgFileKind(name) {
	case "text", "code", "data":
		return true
	}
	return false
}

// managerStatus is the status bar in Manager mode: the chosen row's path.
func (m *Model) managerStatus() string {
	left := m.syBlock(cStatusBG) + stStatus.Render(" ") + stStatus.Bold(true).Render("Manager")
	where := ""
	if m.mgr != nil && m.mgr.data != nil {
		if r := m.mgr.chosen(); r != nil {
			where = shortPath(mgPath(m.mgr.data.Root, r.it))
		}
	}
	right := stStatusDim.Render("Alt+M modes · F1 keys ") + m.syBlock(cStatusBG)
	room := m.width - lipgloss.Width(left) - lipgloss.Width(right) - 4
	mid := stStatus.Render("  " + mgFit(where, max(room, 0)))
	return mgPad(left+mid, m.width-lipgloss.Width(right), cStatusBG) + right
}

// mgKindWord is the Kind column: the project's language, or what it is.
func mgKindWord(it *mgItem) string {
	switch {
	case it.Kind != "":
		return it.Kind
	case it.Project:
		return "project"
	case it.Dir:
		return "folder"
	}
	return ""
}
