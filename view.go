package main

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/mattn/go-runewidth"
)

// runeWidth is how many screen columns a letter takes. x is where it starts,
// so a tab can reach the next tab stop.
func runeWidth(r rune, x int) int {
	if r == '\t' {
		return tabWidth - x%tabWidth
	}
	if r < 0x300 {
		if r < 0x20 || r == 0x7f {
			return 1 // shown as a symbol
		}
		return 1
	}
	return max(runewidth.RuneWidth(r), 0)
}

func (m *Model) View() tea.View {
	var sb strings.Builder
	sb.Grow(m.width * m.height * 4)
	switch {
	case m.help:
		m.drawHelp(&sb)
	case m.picker != nil:
		m.drawPicker(&sb)
	case m.diaryViewer():
		sb.WriteString(paintRow(m.topPanel(), 0, m.width))
		sb.WriteByte('\n')
		sb.WriteString(paintRow(m.tabBar(), 1, m.width))
		sb.WriteByte('\n')
		m.diaryRows(&sb)
		sb.WriteString(paintRow(m.infoLine(), m.height-3, m.width))
		sb.WriteByte('\n')
		sb.WriteString(paintRow(m.statusBar(), m.height-2, m.width))
		sb.WriteByte('\n')
		sb.WriteString(m.bottomLine())
	case !modes[m.mode].ready:
		sb.WriteString(paintRow(m.topPanel(), 0, m.width))
		sb.WriteByte('\n')
		sb.WriteString(paintRow(m.tabBar(), 1, m.width))
		sb.WriteByte('\n')
		m.emptyModeRows(&sb)
		sb.WriteString(paintRow(m.infoLine(), m.height-3, m.width))
		sb.WriteByte('\n')
		sb.WriteString(paintRow(m.statusBar(), m.height-2, m.width))
		sb.WriteByte('\n')
		sb.WriteString(m.bottomLine())
	default:
		sb.WriteString(paintRow(m.topPanel(), 0, m.width))
		sb.WriteByte('\n')
		sb.WriteString(paintRow(m.tabBar(), 1, m.width))
		sb.WriteByte('\n')
		var text strings.Builder
		m.drawText(&text)
		rows := m.withMargins(strings.Split(strings.TrimSuffix(text.String(), "\n"), "\n"))
		for _, r := range rows {
			sb.WriteString(r)
			sb.WriteByte('\n')
		}
		sb.WriteString(paintRow(m.infoLine(), m.height-3, m.width))
		sb.WriteByte('\n')
		sb.WriteString(paintRow(m.statusBar(), m.height-2, m.width))
		sb.WriteByte('\n')
		sb.WriteString(m.bottomLine())
	}
	content := sb.String()
	if m.tabMode {
		content = m.overlayTabRows(content)
	}
	if m.modesOpen {
		content = m.overlay(content, m.modesBox())
	}
	if m.showTree && m.tree != nil && !m.help && m.picker == nil {
		content = m.overlay(content, m.fileManagerBox())
	}
	if m.themeOpen {
		content = m.overlay(content, m.themesBox())
	}
	content = m.overlayPlate(content)
	v := tea.NewView(content)
	v.AltScreen = true
	v.MouseMode = tea.MouseModeCellMotion
	v.ReportFocus = true // the lock plate only shows in the window being typed in
	v.KeyboardEnhancements.ReportEventTypes = true
	// Every key as a code (with its text), so AltGr on its own can be seen.
	v.KeyboardEnhancements.ReportAllKeysAsEscapeCodes = true
	v.KeyboardEnhancements.ReportAssociatedText = true
	v.BackgroundColor = hex(cBG)
	v.ForegroundColor = hex(cText)
	name := m.tabName(m.cur) + " — Symbolizer"
	v.WindowTitle = name
	if !m.help && m.picker == nil && !m.modesOpen && !m.themeOpen && !m.tabMode && modes[m.mode].ready && !m.diaryViewer() && !(m.treeFocus && m.showTree && m.prompt == pNone) {
		v.Cursor = m.cursor()
	}
	return v
}

func (m *Model) cursor() *tea.Cursor {
	if m.prompt == pConfirm || m.prompt == pReplaceAsk {
		return nil
	}
	if m.prompt != pNone {
		c := m.input.Cursor()
		if c == nil {
			return nil
		}
		c.Y = m.height - 1
		return c
	}
	b := m.buf
	y := b.Cursor.Row - m.top + textTop
	left, _, _ := m.zone()
	start := m.treeWidth() + left + m.gutterWidth()
	x := colToX(m.line(b.Cursor.Row), b.Cursor.Col) - m.left + start + m.lineOffset(b.Cursor.Row)
	if y < textTop || y >= textTop+m.textHeight() || x < start || x >= start+m.textWidth() {
		return nil
	}
	c := tea.NewCursor(x, y)
	c.Shape = tea.CursorBar
	c.Blink = false
	c.Color = hex(cEmber)
	return c
}

// fit cuts or pads a styled string to exactly w columns.
func fit(s string, w int, pad lipgloss.Style) string {
	sw := lipgloss.Width(s)
	if sw > w {
		return lipgloss.NewStyle().MaxWidth(w).Render(s)
	}
	return s + pad.Render(strings.Repeat(" ", w-sw))
}

func (m *Model) statusBar() string {
	b := m.buf
	where := m.tabName(m.cur)
	if b.Draft {
		where = sessionText(b) // the tab shows the made-up file name
	}
	left := m.syBlock(cStatusBG) + stStatus.Render(" ") + stStatus.Render(where)
	syRight := stStatus.Render(" ") + m.syBlock(cStatusBG)
	state, short := "", ""
	switch {
	case m.saveErr != "":
		state = lipgloss.NewStyle().Background(hex(cStatusBG)).Foreground(hex(cError)).Bold(true).Render("  ✗ not saved")
		short = state
	case m.savePaused:
		state = lipgloss.NewStyle().Background(hex(cStatusBG)).Foreground(hex(cWarning)).Bold(true).Render("  ⏸ saving paused")
		short = state
	case b.Draft:
		state = stStatusDim.Render("  ✓ kept as a session · Ctrl+S names it")
		short = stStatusDim.Render("  ✓")
	case b.Path != "":
		state = stStatusDim.Render("  ✓ saves by itself")
		short = stStatusDim.Render("  ✓")
	}

	var parts []string
	if s, e, ok := b.Selection(); ok {
		n := len([]rune(b.Get(s, e)))
		if e.Row > s.Row {
			parts = append(parts, fmt.Sprintf("%d letters, %d lines selected", n, e.Row-s.Row+1))
		} else {
			parts = append(parts, fmt.Sprintf("%d selected", n))
		}
	}
	parts = append(parts, m.hl.Name)
	parts = append(parts, fmt.Sprintf("Ln %d, Col %d", b.Cursor.Row+1, b.Cursor.Col+1))
	parts = append(parts, fmt.Sprintf("%d lines", len(b.Lines)))
	// On a narrow window, shorten in this order: the saving words, the line
	// count, the language, the key mode word (its mark stays). The cursor
	// position always stays.
	full, small := m.keyModeSign(false), m.keyModeSign(true)
	tries := []struct {
		state string
		sign  string
		parts []string
	}{
		{state, full, parts}, {short, full, parts}, {short, full, parts[:len(parts)-1]},
		{short, full, parts[len(parts)-2 : len(parts)-1]}, {short, small, parts[len(parts)-2 : len(parts)-1]},
	}
	for _, t := range tries {
		l := left + t.state
		right := t.sign + m.lampSign() + stStatus.Render("  ") + stStatusDim.Render(strings.Join(t.parts, "  ·  ")) + syRight
		if gap := m.width - lipgloss.Width(l) - lipgloss.Width(right); gap >= 1 {
			return l + stStatus.Render(strings.Repeat(" ", gap)) + right
		}
	}
	signs := small
	return fit(left+short, m.width-lipgloss.Width(signs)-lipgloss.Width(syRight), stStatus) + signs + syRight
}

func (m *Model) bottomLine() string {
	switch m.prompt {
	case pNone:
		if m.msg == "" {
			return stMessage.Render(strings.Repeat(" ", m.width))
		}
		st := stMessage
		if m.msgErr {
			st = stError
		}
		return fit(st.Render(" "+m.msg), m.width, stMessage)
	case pConfirm:
		return fit(stPrompt.Render(" "+m.confirmText), m.width, stMessage)
	case pReplaceAsk:
		return fit(stPrompt.Render(" Replace this one?  y = yes   n = skip   a = all   Esc = stop"),
			m.width, stMessage)
	}
	in := m.input.View()
	if m.msg != "" {
		st := stStatusDim.Background(hex(cBG))
		if m.msgErr {
			st = stError
		}
		in = fit(in, max(m.width/2, lipgloss.Width(in)+2), stMessage) + st.Render(m.msg)
	}
	return fit(in, m.width, stMessage)
}

// ---- the text ----------------------------------------------------------------------

type cell struct {
	fg, bg                  string
	bold, italic, underline bool
}

func (m *Model) drawText(sb *strings.Builder) {
	b := m.buf
	h := m.textHeight()
	gw := m.gutterWidth()
	fullTW := m.textWidth()
	selS, selE, hasSel := b.Selection()
	bracketA, bracketB, hasBracket := m.matchBracket()
	var q []rune
	var fold bool
	if m.showMatches && m.query != "" {
		q, fold = m.queryRunes()
	}
	numFmt := fmt.Sprintf(" %%%dd ", gw-2)
	blankGutter := strings.Repeat(" ", gw)

	for y := 0; y < h; y++ {
		tw := fullTW
		r := m.top + y
		if r >= len(b.Lines) {
			sb.WriteString(sgr(cGutterLine, cBG, false, false, false))
			sb.WriteString(blankGutter)
			sb.WriteString(strings.Repeat(" ", max(tw, 0)))
			sb.WriteString(ansiReset)
			sb.WriteByte('\n')
			continue
		}
		line := b.Lines[r]
		isCur := r == b.Cursor.Row
		rowBG := cBG
		if isCur && !hasSel {
			rowBG = cCurrentLine
		}
		// line number
		if isCur {
			sb.WriteString(sgr(cCurrentNum, rowBG, true, false, false))
		} else {
			sb.WriteString(sgr(cLineNumber, cBG, false, false, false))
		}
		sb.WriteString(fmt.Sprintf(numFmt, r+1))
		if off := m.lineOffset(r); off > 0 { // a centred line: space in front of it
			sb.WriteString(sgr(cText, rowBG, false, false, false))
			sb.WriteString(strings.Repeat(" ", off))
			tw -= off
		}

		// search matches on this line
		var matches [][2]int
		if q != nil {
			for c := indexRunes(line, q, 0, fold); c >= 0; c = indexRunes(line, q, c+len(q), fold) {
				matches = append(matches, [2]int{c, c + len(q)})
				if len(matches) > 500 {
					break
				}
			}
		}
		spans := m.hl.Line(r)
		si, mi := 0, 0
		x := 0
		last := ""
		for c := 0; c <= len(line) && x < m.left+tw; c++ {
			if c == len(line) {
				break
			}
			ch := line[c]
			w := runeWidth(ch, x)
			// colour from the syntax
			for si < len(spans) && spans[si].To <= c {
				si++
			}
			st := syntaxStyles[sNormal]
			if si < len(spans) && spans[si].From <= c {
				st = syntaxStyles[spans[si].Style]
			}
			cl := cell{st.fg, rowBG, st.bold, st.italic, st.underline}
			for mi < len(matches) && matches[mi][1] <= c {
				mi++
			}
			p := Pos{r, c}
			inSel := hasSel && !p.Less(selS) && p.Less(selE)
			switch {
			case inSel:
				cl.bg, cl.fg = cSelection, cSelectedText
			case mi < len(matches) && matches[mi][0] <= c:
				cl.bg = cSearch
				if hasSel && selS.Row == r && selS.Col == matches[mi][0] {
					cl.bg = cSearchNow
				}
			case hasBracket && (p == bracketA || p == bracketB):
				cl.bg, cl.bold = cBracket, true
			}
			// what to draw
			glyph := string(ch)
			switch {
			case ch == '\t':
				glyph = strings.Repeat(" ", w)
			case ch < 0x20 || ch == 0x7f:
				glyph = "·"
				cl.fg = cLineNumber
			}
			if x+w <= m.left {
				x += w
				continue
			}
			if x < m.left { // a wide letter cut by the left edge
				glyph = strings.Repeat(" ", x+w-m.left)
			}
			if x+w > m.left+tw { // cut by the right edge
				glyph = strings.Repeat(" ", m.left+tw-x)
			}
			code := sgr(cl.fg, cl.bg, cl.bold, cl.italic, cl.underline)
			if code != last {
				sb.WriteString(code)
				last = code
			}
			sb.WriteString(glyph)
			x += w
		}
		// selection reaching past the end of the line shows one cell
		used := max(x-m.left, 0)
		if used < tw && hasSel && selS.Row <= r && r < selE.Row && len(line) >= 0 {
			if colToX(line, len(line)) >= m.left {
				sb.WriteString(sgr(cSelectedText, cSelection, false, false, false))
				sb.WriteByte(' ')
				used++
			}
		}
		if used < tw {
			sb.WriteString(sgr(cText, rowBG, false, false, false))
			sb.WriteString(strings.Repeat(" ", tw-used))
		}
		sb.WriteString(ansiReset)
		sb.WriteByte('\n')
	}
}

const ansiReset = "\x1b[m"

var pairs = map[rune]rune{'(': ')', '[': ']', '{': '}', ')': '(', ']': '[', '}': '{'}

// matchBracket finds the bracket that pairs with the one at (or just before)
// the cursor. It looks at most 20 000 lines away.
func (m *Model) matchBracket() (Pos, Pos, bool) {
	b := m.buf
	l := b.Lines[b.Cursor.Row]
	c := b.Cursor.Col
	at := -1
	if c < len(l) && pairs[l[c]] != 0 {
		at = c
	} else if c > 0 && pairs[l[c-1]] != 0 {
		at = c - 1
	}
	if at < 0 {
		return Pos{}, Pos{}, false
	}
	open := l[at]
	close := pairs[open]
	dir := 1
	if open == ')' || open == ']' || open == '}' {
		dir = -1
	}
	depth := 0
	r, col := b.Cursor.Row, at
	for steps := 0; steps < 20000; {
		line := b.Lines[r]
		for col >= 0 && col < len(line) {
			switch line[col] {
			case open:
				depth++
			case close:
				depth--
				if depth == 0 {
					return Pos{b.Cursor.Row, at}, Pos{r, col}, true
				}
			}
			col += dir
		}
		r += dir
		steps++
		if r < 0 || r >= len(b.Lines) {
			break
		}
		if dir > 0 {
			col = 0
		} else {
			col = len(b.Lines[r]) - 1
		}
	}
	return Pos{}, Pos{}, false
}
