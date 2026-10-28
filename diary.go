package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// Diary mode: writing kept by date. Each day is a chapter; a chapter holds
// pages. They are plain text files:
//
//	~/.local/share/symbolizer/diary/2026-10-26/001.txt, 002.txt ...
//
// The page viewer shows one page at a time.
//
//	Left / Right     previous / next page (on into the chapter before or after)
//	Up / Down        previous / next chapter
//	Alt+N or AltGr+N (”)   a new page in today's chapter, ready to write on
//	Alt+A or AltGr+A (æ)   all pages at once (the same arrows move; Enter shows one)
//	Enter            write on the page (the normal editor); Esc comes back

const diaryMode = 3 // its place in modes

type diaryChapter struct {
	day   string   // 2026-10-26
	pages []string // page files, in order
}

type diaryState struct {
	chapters []diaryChapter
	ch, pg   int  // the page shown
	all      bool // all pages at once
	writing  bool // the page is open in the editor
}

func diaryDir() string { return filepath.Join(filepath.Dir(sessionDir()), "diary") }

// loadDiary reads the chapters and pages from disk.
func loadDiary() []diaryChapter {
	var out []diaryChapter
	days, _ := os.ReadDir(diaryDir())
	for _, d := range days {
		if !d.IsDir() {
			continue
		}
		if _, err := time.Parse("2006-01-02", d.Name()); err != nil {
			continue
		}
		files, _ := os.ReadDir(filepath.Join(diaryDir(), d.Name()))
		var pages []string
		for _, f := range files {
			if !f.IsDir() && strings.HasSuffix(f.Name(), ".txt") {
				pages = append(pages, filepath.Join(diaryDir(), d.Name(), f.Name()))
			}
		}
		if len(pages) > 0 {
			sort.Strings(pages)
			out = append(out, diaryChapter{d.Name(), pages})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].day < out[j].day })
	return out
}

func (m *Model) inDiary() bool { return m.mode == diaryMode }

// diaryViewer is true while the page viewer (or all pages) is on screen.
func (m *Model) diaryViewer() bool { return m.inDiary() && (m.diary == nil || !m.diary.writing) }

// enterDiary sets the diary up when the mode is switched to: the newest page.
func (m *Model) enterDiary() {
	d := &diaryState{chapters: loadDiary()}
	if n := len(d.chapters); n > 0 {
		d.ch = n - 1
		d.pg = len(d.chapters[n-1].pages) - 1
	}
	m.diary = d
}

// refreshDiary reads the disk again but keeps the same page on screen.
func (m *Model) refreshDiary() {
	d := m.diary
	path := d.page()
	d.chapters = loadDiary()
	d.goTo(path)
}

func (d *diaryState) empty() bool { return len(d.chapters) == 0 }

func (d *diaryState) page() string {
	if d.empty() {
		return ""
	}
	return d.chapters[d.ch].pages[d.pg]
}

func (d *diaryState) goTo(path string) {
	for c, ch := range d.chapters {
		for p, pg := range ch.pages {
			if pg == path {
				d.ch, d.pg = c, p
				return
			}
		}
	}
	d.ch = max(0, min(d.ch, len(d.chapters)-1))
	if !d.empty() {
		d.pg = max(0, min(d.pg, len(d.chapters[d.ch].pages)-1))
	}
}

// turn moves one page, on into the next or previous chapter at its ends.
func (d *diaryState) turn(step int) bool {
	if d.empty() {
		return false
	}
	pg, ch := d.pg+step, d.ch
	if pg < 0 {
		if ch == 0 {
			return false
		}
		ch--
		pg = len(d.chapters[ch].pages) - 1
	} else if pg >= len(d.chapters[ch].pages) {
		if ch == len(d.chapters)-1 {
			return false
		}
		ch++
		pg = 0
	}
	d.ch, d.pg = ch, pg
	return true
}

// chapter moves to the first page of the chapter before or after.
func (d *diaryState) chapter(step int) bool {
	ch := d.ch + step
	if d.empty() || ch < 0 || ch >= len(d.chapters) {
		return false
	}
	d.ch, d.pg = ch, 0
	return true
}

// newDiaryPage makes an empty page at the end of today's chapter and opens it for writing.
func (m *Model) newDiaryPage() tea.Cmd {
	if m.diary == nil {
		m.enterDiary()
	}
	day := time.Now().Format("2006-01-02")
	dir := filepath.Join(diaryDir(), day)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		m.sayErr("Could not make the page: " + err.Error())
		return nil
	}
	n := 1
	for fileExists(filepath.Join(dir, fmt.Sprintf("%03d.txt", n))) {
		n++
	}
	path := filepath.Join(dir, fmt.Sprintf("%03d.txt", n))
	if err := os.WriteFile(path, nil, 0o600); err != nil {
		m.sayErr("Could not make the page: " + err.Error())
		return nil
	}
	d := m.diary
	d.chapters = loadDiary()
	d.goTo(path)
	d.all = false
	cmd := m.writeDiaryPage()
	m.say(fmt.Sprintf("New page %d of today. Write away; Esc shows the pages.", n))
	return cmd
}

// writeDiaryPage opens the page on screen in the editor.
func (m *Model) writeDiaryPage() tea.Cmd {
	d := m.diary
	if d == nil || d.empty() {
		m.say("No pages yet. Alt+N (or AltGr+N) makes the first one.")
		return nil
	}
	cmd := m.openPath(d.page())
	m.buf.Cursor = m.buf.Clamp(Pos{len(m.buf.Lines) - 1, len(m.buf.Lines[len(m.buf.Lines)-1])})
	m.ensureVisible()
	d.writing = true
	return cmd
}

// leaveDiaryPage saves the page being written, closes its tab and goes back to the viewer.
func (m *Model) leaveDiaryPage() {
	m.closeDiaryTab()
	m.diary.writing = false
	m.refreshDiary()
}

// closeDiaryTab saves the diary page on screen and closes its tab. If it can't
// be saved, the tab stays open so nothing is lost.
func (m *Model) closeDiaryTab() {
	m.say("")
	if !strings.HasPrefix(m.buf.Path, diaryDir()+string(filepath.Separator)) {
		return
	}
	if !m.saveNow() || m.buf.Modified() {
		m.sayErr("Could not save this page yet, so its tab stays open.")
		return
	}
	m.tabs = append(m.tabs[:m.cur], m.tabs[m.cur+1:]...)
	if len(m.tabs) == 0 {
		m.cur = 0
		m.addTab(newDraft())
	} else {
		m.showTab(min(m.cur, len(m.tabs)-1))
	}
}

// diaryKey handles a key in diary mode. done is false when the key is not the
// diary's own (it then goes to the editor, while a page is being written).
func (m *Model) diaryKey(name string) (tea.Cmd, bool) {
	if m.diary == nil {
		m.enterDiary()
	}
	d := m.diary
	switch name {
	case "alt+n", "”":
		if d.writing {
			m.closeDiaryTab()
		}
		return m.newDiaryPage(), true
	case "alt+a", "æ":
		if d.writing {
			m.leaveDiaryPage()
		}
		d.all = !d.all
		return nil, true
	}
	if d.writing {
		if name == "esc" && m.prompt == pNone && m.picker == nil && !m.help && !(m.showTree && m.treeFocus) {
			m.leaveDiaryPage()
			return nil, true
		}
		return nil, false
	}
	m.msg = ""
	switch name {
	case "left":
		if !d.turn(-1) && !d.empty() {
			m.say("This is the first page.")
		}
	case "right":
		if !d.turn(1) && !d.empty() {
			m.say("This is the last page. Alt+N makes a new one.")
		}
	case "up":
		if !d.chapter(-1) && !d.empty() {
			m.say("This is the first chapter.")
		}
	case "down":
		if !d.chapter(1) && !d.empty() {
			m.say("This is the last chapter.")
		}
	case "enter":
		if d.all {
			d.all = false
			return nil, true
		}
		return m.writeDiaryPage(), true
	case "esc":
		d.all = false
	case "f1":
		m.help = !m.help
	}
	return nil, true
}

// ---- drawing ----------------------------------------------------------------------------

// chapterTitle: Monday the twenty-sixth of October.
func chapterTitle(day string) string {
	t, err := time.Parse("2006-01-02", day)
	if err != nil {
		return day
	}
	title := t.Weekday().String() + " the " + ordinalWord(t.Day()) + " of " + t.Month().String()
	if t.Year() != time.Now().Year() {
		title += fmt.Sprintf(" %d", t.Year())
	}
	return title
}

func readPage(path string) []string {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	return strings.Split(strings.TrimRight(strings.ReplaceAll(string(b), "\r\n", "\n"), "\n"), "\n")
}

// diaryRows fills the text area with the page viewer or with all pages.
func (m *Model) diaryRows(sb *strings.Builder) {
	if m.diary == nil {
		m.enterDiary()
	}
	h := m.textHeight()
	var block string
	switch {
	case m.diary.empty():
		block = m.diaryEmpty()
	case m.diary.all:
		block = m.diaryAll(h)
	default:
		block = m.diaryPage(h)
	}
	block = lipgloss.Place(m.width, h, lipgloss.Center, lipgloss.Center, block,
		lipgloss.WithWhitespaceStyle(lipgloss.NewStyle().Background(hex(cBG))))
	sb.WriteString(lipgloss.NewStyle().Background(hex(cBG)).Render(block))
	sb.WriteByte('\n')
}

func (m *Model) diaryEmpty() string {
	st := func(fg string) lipgloss.Style { return lipgloss.NewStyle().Foreground(hex(fg)).Background(hex(cBG)) }
	return strings.Join([]string{
		st(cEmber).Bold(true).Render(modes[diaryMode].Icon() + "  Diary"),
		"",
		st(cStatusText).Render("No pages yet."),
		st(cAccent).Render("Alt+N (or AltGr+N) makes the first page of today."),
		"",
		st(cDim).Italic(true).Render("Alt+M switches modes · Alt+Q quits"),
	}, "\n")
}

// diaryPage draws the one page in focus as a sheet: the chapter, the page
// number, the text, and the keys underneath.
func (m *Model) diaryPage(h int) string {
	d := m.diary
	ch := d.chapters[d.ch]
	pageW := max(min(m.width-6, 74), 20) // inside the sheet
	textW := pageW - 4
	bodyH := max(h-8, 3) // sheet border 2, header 3, footer 2, a row free
	paper := cBarBG
	st := func(fg string) lipgloss.Style { return lipgloss.NewStyle().Foreground(hex(fg)).Background(hex(paper)) }
	row := func(s string, align lipgloss.Position) string {
		return lipgloss.NewStyle().Width(pageW).Align(align).Background(hex(paper)).Render(s)
	}

	var lines []string
	for _, l := range readPage(d.page()) {
		if l == "" {
			lines = append(lines, "")
			continue
		}
		lines = append(lines, wrapWords(l, textW)...)
	}
	more := false
	if len(lines) > bodyH {
		lines, more = lines[:bodyH-1], true
	}
	var body []string
	if len(lines) == 0 || (len(lines) == 1 && lines[0] == "") {
		body = append(body, row(st(cDim).Italic(true).Render("an empty page — Enter to write on it"), lipgloss.Center))
	} else {
		for _, l := range lines {
			body = append(body, row("  "+st(cText).Render(l), lipgloss.Left))
		}
	}
	if more {
		body = append(body, row(st(cDim).Italic(true).Render("… more — Enter shows it all"), lipgloss.Center))
	}
	for len(body) < bodyH {
		body = append(body, row("", lipgloss.Left))
	}

	pageWord := fmt.Sprintf("page %s of %s", numberWord(min(d.pg+1, 99)), numberWord(min(len(ch.pages), 99)))
	if d.pg+1 >= 100 || len(ch.pages) >= 100 {
		pageWord = fmt.Sprintf("page %d of %d", d.pg+1, len(ch.pages))
	}
	chapWord := fmt.Sprintf("chapter %d of %d", d.ch+1, len(d.chapters))
	header := []string{
		row(glowText(chapterTitle(ch.day), paper), lipgloss.Center),
		row(st(cAccent).Render(pageWord)+st(cDim).Render("  ·  "+chapWord), lipgloss.Center),
		row(st(cGutterLine).Render(strings.Repeat("─", pageW-4)), lipgloss.Center),
	}
	keys := "← → pages  ·  ↑ ↓ chapters  ·  Enter write  ·  Alt+N new page  ·  Alt+A all pages"
	if lipgloss.Width(keys) > pageW {
		keys = "← → pages · ↑ ↓ chapters · Enter write"
	}
	footer := []string{
		row(st(cGutterLine).Render(strings.Repeat("─", pageW-4)), lipgloss.Center),
		row(st(cDim).Italic(true).Render(keys), lipgloss.Center),
	}
	inside := strings.Join(append(append(header, body...), footer...), "\n")
	sheet := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(hex(cEmber)).
		BorderBackground(hex(cBG)).Background(hex(paper)).Render(inside)
	return m.diaryEdges(sheet)
}

// diaryEdges puts a faint arrow either side of the sheet when there is a page that way.
func (m *Model) diaryEdges(sheet string) string {
	d := m.diary
	back, on := d.ch > 0 || d.pg > 0, d.ch < len(d.chapters)-1 || d.pg < len(d.chapters[d.ch].pages)-1
	arrow := func(s string, ok bool) string {
		if !ok {
			s = " "
		}
		return lipgloss.NewStyle().Foreground(hex(cEmber)).Background(hex(cBG)).Render(" " + s + " ")
	}
	if m.width < lipgloss.Width(sheet)+6 {
		return sheet
	}
	return lipgloss.JoinHorizontal(lipgloss.Center, arrow("‹", back), sheet, arrow("›", on))
}

// diaryAll draws every page as a small card, a row of cards per chapter. The
// page in focus glows; the list scrolls to keep it on screen.
func (m *Model) diaryAll(h int) string {
	d := m.diary
	cardW := 20
	perRow := max(1, min((m.width-4)/(cardW+3), 6))
	var rows []string
	focusRow := 0
	bg := cBG
	st := func(fg, b string) lipgloss.Style { return lipgloss.NewStyle().Foreground(hex(fg)).Background(hex(b)) }
	total := 0
	for _, ch := range d.chapters {
		total += len(ch.pages)
	}
	rows = append(rows, st(cEmber, bg).Bold(true).Render(fmt.Sprintf("%s  All pages · %d in %d chapters", modes[diaryMode].Icon(), total, len(d.chapters))), "")
	for c, ch := range d.chapters {
		rows = append(rows, st(cAccent, bg).Bold(true).Render(chapterTitle(ch.day)))
		for start := 0; start < len(ch.pages); start += perRow {
			var cards []string
			for p := start; p < min(start+perRow, len(ch.pages)); p++ {
				focus := c == d.ch && p == d.pg
				cards = append(cards, m.diaryCard(ch.pages[p], p+1, cardW, focus))
				if focus {
					focusRow = len(rows)
				}
			}
			rows = append(rows, lipgloss.JoinHorizontal(lipgloss.Top, cards...))
		}
		rows = append(rows, "")
	}
	rows = append(rows, st(cDim, bg).Italic(true).Render("← → pages · ↑ ↓ chapters · Enter show this page · Alt+A or Esc back"))
	// keep the page in focus on screen: rows are blocks of several lines
	var lines []string
	focusLine := 0
	for i, r := range rows {
		if i == focusRow {
			focusLine = len(lines)
		}
		lines = append(lines, strings.Split(r, "\n")...)
	}
	if len(lines) > h {
		top := max(0, min(focusLine-h/3, len(lines)-h))
		lines = lines[top : top+h]
	}
	return strings.Join(lines, "\n")
}

func (m *Model) diaryCard(path string, n, w int, focus bool) string {
	paper, border, numFG := cBarBG, cGutterLine, cDim
	if focus {
		paper, border, numFG = cSelection, cEmber, cAccent
	}
	st := func(fg string) lipgloss.Style {
		return lipgloss.NewStyle().Foreground(hex(fg)).Background(hex(paper)).Width(w)
	}
	first := ""
	for _, l := range readPage(path) {
		if strings.TrimSpace(l) != "" {
			first = strings.TrimSpace(l)
			break
		}
	}
	preview := st(cDim).Italic(true).Render("empty")
	if first != "" {
		pw := wrapWords(first, w-1)
		if len(pw) > 2 {
			pw = pw[:2]
			pw[1] = fit(pw[1]+"…", w-1, lipgloss.NewStyle())
		}
		for len(pw) < 2 {
			pw = append(pw, "")
		}
		preview = st(cText).Render(pw[0]) + "\n" + st(cText).Render(pw[1])
	} else {
		preview += "\n" + st(cText).Render("")
	}
	inside := st(numFG).Bold(focus).Render(fmt.Sprintf("page %d", n)) + "\n" + preview
	card := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(hex(border)).
		BorderBackground(hex(cBG)).Background(hex(paper)).Render(inside)
	gap := lipgloss.NewStyle().Background(hex(cBG)).Render(" ")
	out := strings.Split(card, "\n")
	for i := range out {
		out[i] += gap
	}
	return strings.Join(out, "\n")
}
