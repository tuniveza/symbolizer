package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"unicode"

	tea "charm.land/bubbletea/v2"
)

// ask opens a one-line question at the bottom with a Bubbles text input.
func (m *Model) ask(kind promptKind, label, value string) tea.Cmd {
	m.prompt = kind
	m.input.Prompt = label
	m.input.SetValue(value)
	m.input.CursorEnd()
	m.input.SetWidth(m.inputWidth())
	return m.input.Focus()
}

// inputWidth leaves the right half of the line for messages such as
// "Match 2 of 5".
func (m *Model) inputWidth() int {
	return max(m.width/2-len([]rune(m.input.Prompt))-2, 10)
}

// confirm asks a question answered with one key, such as y or n.
func (m *Model) confirm(text, keys string, fn func(m *Model, k rune) tea.Cmd) tea.Cmd {
	m.prompt = pConfirm
	m.confirmText, m.confirmKeys, m.onConfirm = text, keys, fn
	return nil
}

func (m *Model) closePrompt() {
	m.prompt = pNone
	m.input.Placeholder = ""
	m.input.Blur()
}

func (m *Model) promptKey(msg tea.KeyPressMsg) tea.Cmd {
	name := keyName(msg.Key())
	switch m.prompt {
	case pConfirm:
		if name == "esc" || name == "ctrl+c" {
			m.closePrompt()
			m.say("Cancelled.")
			return nil
		}
		k := unicode.ToLower([]rune(msg.Key().Text + " ")[0])
		if strings.ContainsRune(m.confirmKeys, k) {
			fn := m.onConfirm
			m.closePrompt()
			return fn(m, k)
		}
		return nil
	case pReplaceAsk:
		return m.replaceAnswer(name, msg.Key().Text)
	}

	switch name {
	case "esc":
		kind := m.prompt
		m.closePrompt()
		if kind == pFind {
			m.say("")
		} else {
			m.say("Cancelled.")
		}
		return nil
	case "enter":
		return m.promptEnter(false)
	case "shift+enter":
		if m.prompt == pFind {
			m.findNext(-1, true)
			return nil
		}
		return m.promptEnter(false)
	case "down", "ctrl+f", "f3":
		if m.prompt == pFind {
			m.findNext(1, true)
			return nil
		}
	case "up", "shift+f3":
		if m.prompt == pFind {
			m.findNext(-1, true)
			return nil
		}
	case "tab":
		if m.prompt == pOpen || m.prompt == pSaveAs || m.prompt == pName {
			m.completePath()
			return nil
		}
	case "ctrl+v", "ctrl+p":
		return readClipboard(m.clip)
	}
	before := m.input.Value()
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	if m.input.Value() != before {
		return tea.Batch(cmd, m.promptChanged())
	}
	return cmd
}

// promptChanged runs while typing: Find jumps to the first match at once.
func (m *Model) promptChanged() tea.Cmd {
	if m.prompt == pFind {
		m.query = m.input.Value()
		m.showMatches = m.query != ""
		m.buf.Cursor = m.findFrom
		m.buf.ClearSelect()
		if m.query != "" {
			m.findNext(1, true)
		} else {
			m.say("")
		}
	}
	return nil
}

func (m *Model) promptEnter(_ bool) tea.Cmd {
	value := m.input.Value()
	kind := m.prompt
	m.closePrompt()
	switch kind {
	case pFind:
		m.query = value
		m.showMatches = value != ""
		return nil
	case pReplaceFind:
		if value == "" {
			m.say("Cancelled.")
			return nil
		}
		m.query = value
		return m.ask(pReplaceWith, "Replace \""+value+"\" with: ", m.replaceWith)
	case pReplaceWith:
		m.replaceWith = value
		m.showMatches = true
		m.findFrom = m.buf.Cursor
		if !m.findNext(1, false) {
			m.showMatches = false
			return nil
		}
		m.prompt = pReplaceAsk
		return nil
	case pGoto:
		return m.gotoLine(value)
	case pOpen:
		path := expandPath(value)
		if path == "" {
			return nil
		}
		if info, err := os.Stat(path); err == nil && info.IsDir() {
			m.setRoot(path)
			return m.openTree(false)
		}
		return m.openPath(path)
	case pSaveAs:
		path := expandPath(value)
		if path == "" {
			m.say("Not saved under a new name.")
			return nil
		}
		if fileExists(path) && path != m.buf.Path {
			return m.confirm(filepath.Base(path)+" already exists. Replace it?  y = yes   n = no", "yn",
				func(m *Model, k rune) tea.Cmd {
					if k == 'y' {
						return m.saveAs(path)
					}
					m.say("Not saved under a new name.")
					return nil
				})
		}
		return m.saveAs(path)
	case pName:
		return m.nameEntered(value)
	case pRename, pNewFile, pNewFolder:
		return m.treeAnswer(kind, value)
	}
	return nil
}

func (m *Model) gotoLine(v string) tea.Cmd {
	parts := strings.SplitN(strings.TrimSpace(v), ":", 2)
	n, err := strconv.Atoi(strings.TrimSpace(parts[0]))
	if err != nil {
		m.sayErr("Type a line number, like 42 or 42:7.")
		return nil
	}
	col := 0
	if len(parts) == 2 {
		if c, err := strconv.Atoi(parts[1]); err == nil {
			col = c - 1
		}
	}
	if n < 0 {
		n = len(m.buf.Lines) + n + 1 // -1 = last line
	}
	m.moveTo(Pos{n - 1, col}, false)
	m.top = max(0, m.buf.Cursor.Row-m.textHeight()/2)
	return nil
}

// ---- find ---------------------------------------------------------------------------------

func (m *Model) openFind(kind promptKind) tea.Cmd {
	m.findFrom = m.buf.Cursor
	start := ""
	if s, e, ok := m.buf.Selection(); ok && s.Row == e.Row {
		start = m.buf.Get(s, e)
		m.findFrom = s
	} else if kind == pFind || kind == pReplaceFind {
		start = m.query
	}
	label := "Find: "
	if kind == pReplaceFind {
		label = "Replace, find: "
	}
	cmd := m.ask(kind, label, start)
	if kind == pFind && start != "" {
		m.query = start
		m.showMatches = true
		m.say(m.matchCount())
	}
	return cmd
}

// smartCase: small letters find any case, a capital letter means exact case.
func foldFor(q string) bool {
	for _, r := range q {
		if unicode.IsUpper(r) {
			return false
		}
	}
	return true
}

func indexRunes(l, q []rune, from int, fold bool) int {
	if len(q) == 0 {
		return -1
	}
outer:
	for i := max(from, 0); i+len(q) <= len(l); i++ {
		for j, r := range q {
			c := l[i+j]
			if c != r && !(fold && unicode.ToLower(c) == r) {
				continue outer
			}
		}
		return i
	}
	return -1
}

func lastIndexRunes(l, q []rune, before int, fold bool) int {
	for i := min(before, len(l)-len(q)); i >= 0; i-- {
		if indexRunes(l[:i+len(q)], q, i, fold) == i {
			return i
		}
	}
	return -1
}

func (m *Model) queryRunes() ([]rune, bool) {
	fold := foldFor(m.query)
	q := []rune(m.query)
	if fold {
		for i := range q {
			q[i] = unicode.ToLower(q[i])
		}
	}
	return q, fold
}

// findNext selects the next (d=1) or previous (d=-1) match. It wraps round
// the end of the file. Returns false when there is none.
func (m *Model) findNext(d int, inPrompt bool) bool {
	if m.query == "" {
		m.say("Nothing to find. Ctrl+F to search.")
		return false
	}
	q, fold := m.queryRunes()
	b := m.buf
	n := len(b.Lines)
	start := b.Cursor
	if s, e, ok := b.Selection(); ok {
		if d > 0 {
			start = e
		} else {
			start = s
		}
	}
	for i := 0; i <= n; i++ {
		var r, c int
		if d > 0 {
			r = (start.Row + i) % n
			from := 0
			if i == 0 {
				from = start.Col
			}
			c = indexRunes(b.Lines[r], q, from, fold)
		} else {
			r = ((start.Row-i)%n + n) % n
			before := len(b.Lines[r])
			if i == 0 {
				before = start.Col - 1
			}
			c = lastIndexRunes(b.Lines[r], q, before, fold)
		}
		if c >= 0 {
			a := Pos{r, c}
			b.Anchor, b.Cursor = &a, Pos{r, c + len(q)}
			b.WantCol = -1
			m.ensureVisible()
			m.say(m.matchCount())
			return true
		}
	}
	m.sayErr("\"" + m.query + "\" is not in this file.")
	return false
}

// matchCount says "3 of 12" (counts stop at 10 000 to stay quick).
func (m *Model) matchCount() string {
	q, fold := m.queryRunes()
	total, current := 0, 0
	cur, _, hasSel := m.buf.Selection()
	for r, l := range m.buf.Lines {
		for c := indexRunes(l, q, 0, fold); c >= 0; c = indexRunes(l, q, c+max(len(q), 1), fold) {
			total++
			if hasSel && cur == (Pos{r, c}) {
				current = total
			}
			if total >= 10000 {
				return "10 000+ matches"
			}
		}
	}
	switch {
	case total == 0:
		return "No matches."
	case current > 0:
		return fmt.Sprintf("Match %d of %d.   Enter/Down = next   Up = back   Esc = close", current, total)
	}
	return fmt.Sprintf("%d matches.", total)
}

// ---- replace -------------------------------------------------------------------------------

func (m *Model) replaceAnswer(name, text string) tea.Cmd {
	b := m.buf
	k := strings.ToLower(text)
	switch {
	case name == "esc" || k == "q":
		m.closePrompt()
		m.say("Stopped replacing.")
		return nil
	case k == "y" || name == "enter":
		s, e, ok := b.Selection()
		if ok {
			b.Change(s, e, m.replaceWith, "")
		}
		if !m.findNext(1, false) {
			m.closePrompt()
		}
	case k == "n" || name == "down":
		if !m.findNext(1, false) {
			m.closePrompt()
		}
	case k == "a":
		count := 0
		q, fold := m.queryRunes()
		b.Group(func() {
			for r := 0; r < len(b.Lines); r++ {
				for c := indexRunes(b.Lines[r], q, 0, fold); c >= 0; {
					end := b.Change(Pos{r, c}, Pos{r, c + len(q)}, m.replaceWith, "")
					count++
					r = end.Row
					c = indexRunes(b.Lines[r], q, end.Col, fold)
				}
			}
		})
		m.closePrompt()
		m.showMatches = false
		m.say(fmt.Sprintf("Replaced %d.", count))
		m.ensureVisible()
	}
	return nil
}

// ---- file name completion ----------------------------------------------------------------

func (m *Model) completePath() {
	typed := m.input.Value()
	full := expandPath(typed)
	dir, part := filepath.Split(full)
	if strings.HasSuffix(typed, "/") {
		dir, part = full+"/", ""
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		m.sayErr("No such folder.")
		return
	}
	var names []string
	for _, e := range entries {
		n := e.Name()
		if strings.HasPrefix(n, part) && (part != "" || !strings.HasPrefix(n, ".")) {
			if e.IsDir() {
				n += "/"
			}
			names = append(names, n)
		}
	}
	sort.Strings(names)
	if len(names) == 0 {
		m.sayErr("Nothing starts with that.")
		return
	}
	common := names[0]
	for _, n := range names[1:] {
		for !strings.HasPrefix(n, common) {
			common = common[:len(common)-1]
		}
	}
	base := typed[:len(typed)-len(part)]
	if strings.HasSuffix(typed, "/") {
		base = typed
	}
	m.input.SetValue(base + common)
	m.input.CursorEnd()
	if len(names) == 1 {
		m.say("")
		return
	}
	shown := names
	if len(shown) > 12 {
		shown = append(shown[:12:12], fmt.Sprintf("… %d more", len(names)-12))
	}
	m.say(strings.Join(shown, "   "))
}
