package main

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// Brick mode: brick, the plain-English compiler (brick/), inside Symbolizer.
// The editor works as in Text mode, with every word of a brick program
// coloured by what brick reads it as. Under it, one panel: the brain's
// reading of the program, line by line, live as you type:
//
//	✓  the line, read as you wrote it          (how many bytes it builds)
//	→  what the brain understood it as, when it read it differently
//	✗  what brick can't build yet, and why
//	?  the brain isn't sure the line does what you meant: its fixes under it
//
// The keys, kept to three while you write:
//
//	Alt+R  run it (it is built in the background while you type, so it starts at once)
//	Alt+C  check it again now          Alt+P  the brain's fixes: Left/Right pick, Enter writes it in (Esc back)
//	F12 (in every mode) brick learning on / off.  AltGr+D your diary, AltGr+V the brains' voices.
//	Alt tapped twice, then B (in every mode): brick mode.
//
// brick runs as `brick.py serve` in the background (brick_client.go); all
// the work is brick's own.

const brickMode = 7 // its place in modes
const brPanelH = 10 // rows the panel takes under the text

type brState struct {
	pick               int
	focus              bool           // the panel has the keys (to pick one of the brain's fixes)
	watch              map[string]any // the latest check: does it build, and each line as brick understood it
	voices             bool           // the brains speak what they learn (trainer/narrator.py)
	repair             map[string]any // a line the brain judged not precise, and its fixes
	lines              []any          // every line as bricks (the colours)
	watched, coloured  int            // buffer versions
	checkedV           int            // the version the latest check (watch) is for
	brainAsked, brainV int            // the version the brain was asked about / its verdict (repair) is for
	manual, runAfter   bool           // Alt+C asked (say what it found) / Alt+R waits for the brain, then builds
	fixed              []string       // what the brain fixed during this Alt+R (it fixes line after line)
	fixedSeen          map[string]bool
	busy               map[string]bool
}

func (m *Model) inBrick() bool { return m.mode == brickMode }

// brickFile: a file brick reads (not Go, Python ...): .txt, .brick, or a session.
func (m *Model) brickFile() bool {
	if m.buf.Draft || m.buf.Path == "" {
		return true
	}
	ext := strings.ToLower(filepath.Ext(m.buf.Path))
	return ext == ".txt" || ext == ".brick" || ext == ""
}

func (m *Model) br() *brState {
	if m.brick == nil {
		m.brick = &brState{busy: map[string]bool{}, watched: -1, brainAsked: -1, brainV: -1, checkedV: -1}
	}
	return m.brick
}

// brickAsk asks brick something, starting it first if needed.
func (m *Model) brickAsk(what string, fields map[string]any) tea.Cmd {
	var listen tea.Cmd
	if m.bk == nil {
		m.bk = startBrick()
		listen = m.bk.listen()
	}
	if m.bk.ask(what, fields) == 0 {
		m.sayErr("brick isn't answering: " + m.bk.dead)
		return listen
	}
	m.br().busy[what] = true
	return listen
}

func (m *Model) enterBrick() tea.Cmd {
	m.hl.brick = m.brickFile()
	m.hl.done = -1 // colour it brick's way now
	return tea.Batch(m.brickAsk("daemon", nil), m.brickAsk("narrator", nil), m.brickColour())
}

func (m *Model) leaveBrick() {
	m.hl.brick = false
	m.hl.done = -1
}

// brickColour asks brick to read every line (the colours), and to check the
// program: each line as brick understood it, and the program built, ready to run.
func (m *Model) brickColour() tea.Cmd {
	if !m.hl.brick {
		return nil
	}
	s := m.br()
	v := m.buf.Version()
	s.coloured = v
	m.hl.running = true
	text := m.buf.Text()
	cmds := []tea.Cmd{m.brickAsk("lines", map[string]any{"text": text, "version": v})}
	if s.watched != v && !s.busy["check"] {
		s.watched = v
		cmds = append(cmds, m.brickAsk("check", map[string]any{"text": text, "path": m.buf.Path, "version": v}))
	}
	return tea.Batch(cmds...)
}

var brKindStyle = map[string]uint8{
	"do": sKeyword, "thing": sFunction, "colour": sConstant, "place": sType, "time": sNumber, "count": sNumber,
	"saying": sAttribute, "words": sString, "glue": sOperator,
}

// brickSpans turns brick's reading of each line into colours for the editor.
func (m *Model) brickSpans(lines []any) [][]span {
	out := make([][]span, len(m.buf.Lines))
	for i := range out {
		if i >= len(lines) {
			break
		}
		l := obj(lines[i])
		text := string(m.buf.Lines[i])
		low := strings.ToLower(text)
		if str(l, "kind") == "note" {
			out[i] = []span{{From: 0, To: utf8.RuneCountInString(text), Style: sComment}}
			continue
		}
		pos := 0
		for _, d := range list(l, "directions") {
			for _, t := range d.([]any) {
				tok := obj(t)
				w := strings.ToLower(str(tok, "text"))
				if w == "" {
					continue
				}
				at := strings.Index(low[pos:], w)
				if at < 0 {
					continue
				}
				from := utf8.RuneCountInString(text[:pos+at])
				to := from + utf8.RuneCountInString(text[pos+at:pos+at+len(w)])
				if st, ok := brKindStyle[str(tok, "kind")]; ok {
					out[i] = append(out[i], span{From: from, To: to, Style: st})
				}
				pos += at + len(w)
			}
		}
	}
	return out
}

// brickAnswer takes one of brick's answers.
func (m *Model) brickAnswer(a brickMsg) tea.Cmd {
	s := m.br()
	delete(s.busy, a.what)
	next := m.bk.listen()
	if a.what == "dead" {
		m.sayErr("brick stopped. It starts again with the next thing you ask it.")
		m.bk = nil
		return nil
	}
	if a.err != "" {
		if a.what == "lines" {
			m.hl.running = false
		}
		if a.what == "repair" {
			s.brainV, s.repair = s.brainAsked, nil
			if s.runAfter {
				s.runAfter = false
				return tea.Batch(next, m.brickRun("the brain couldn't check it: built as written"))
			}
		}
		m.sayErr("brick: " + a.err)
		return next
	}
	d := a.data
	switch a.what {
	case "lines":
		v := int(num(d, "version"))
		if v == 0 {
			v = s.coloured
		}
		if m.hl.brick {
			m.hl.Finish(highlightMsg{version: v, base: 0, whole: true, lines: m.brickSpans(list(d, "lines"))})
			s.lines = list(d, "lines")
		}
		m.hl.running = false
		if v != m.buf.Version() && m.inBrick() {
			return tea.Batch(next, m.brickColour())
		}
	case "check":
		s.watch, s.checkedV = d, int(num(d, "version"))
		if s.checkedV != m.buf.Version() && m.inBrick() {
			return tea.Batch(next, m.brickColour()) // you typed while it was checking: check again
		}
		if s.runAfter && s.brainV == m.buf.Version() { // Alt+R was waiting for this check
			s.runAfter = false
			return tea.Batch(next, m.brickRunChecked())
		}
		return tea.Batch(next, m.brickBrain())
	case "narrator":
		s.voices = yes(d, "on")
	case "daemon":
		m.brickLearningLamp = yes(d, "running")
	case "run":
		if p := str(d, "program"); p != "" {
			return tea.Batch(next, runProgram(p, str(d, "folder")))
		}
		s.watch = map[string]any{"ok": false, "problems": list(d, "problems")}
		s.checkedV = m.buf.Version()
		m.sayErr("It didn't build: the panel shows why.")
	case "keep":
		if str(d, "program") == "" && len(list(d, "problems")) > 0 {
			m.sayErr("It ran, but writing it next to the file failed: " + firstText(list(d, "problems")))
		}
	case "repair":
		v := int(num(d, "version"))
		if v != m.buf.Version() {
			return tea.Batch(next, m.brickBrain()) // you typed since: the brain reads it again
		}
		s.repair, s.brainV, s.pick = obj(d["repair"]), v, 0
		if s.manual {
			s.manual = false
			if s.repair == nil {
				m.say("Checked: the brain read every line precisely.")
			} else {
				m.say("Line " + fmt.Sprint(int(num(s.repair, "number"))) + ": the brain isn't sure it does what you meant. Its fixes are in the panel.")
			}
		}
		if s.runAfter {
			s.runAfter = false
			return tea.Batch(next, m.brickRunChecked())
		}
	case "bits":
		var names []string
		for _, p := range list(d, "paths") {
			ps, _ := p.(string)
			names = append(names, filepath.Base(ps))
		}
		m.say("brick wrote the bits: " + strings.Join(names, ", ") + " (next to the file).")
	case "bitstream":
		m.say(fmt.Sprintf("Line %d: %d bytes. %s.", m.buf.Cursor.Row+1, int(num(d, "bytes")), str(d, "why")))
	}
	return next
}

func firstText(l []any) string {
	if len(l) == 0 {
		return ""
	}
	s, _ := l[0].(string)
	return s
}

// runProgram runs a built program in a kitty window beside Symbolizer (or a new kitty when not inside one).
func runProgram(program, folder string) tea.Cmd {
	return func() tea.Msg {
		args := []string{"@", "launch", "--type=window", "--cwd=" + folder, "--hold", "--title", filepath.Base(program), program}
		if err := exec.Command("kitten", args...).Run(); err != nil {
			if err := exec.Command("kitty", "--hold", "--directory", folder, program).Start(); err != nil {
				return syTermMsg("Couldn't open a window to run it: " + err.Error())
			}
		}
		return nil
	}
}

// brickKey: keys in brick mode. done=false lets the editor have the key.
func (m *Model) brickKey(k string, text string) (tea.Cmd, bool) {
	s := m.br()
	switch k {
	case "alt+r":
		if strings.TrimSpace(m.buf.Text()) == "" {
			m.sayErr("Nothing to run yet. Type a line first, like: write hello in red")
			return nil, true
		}
		if !m.hl.brick {
			return m.brickRun("as written (not a brick file, so the brain didn't read it)"), true
		}
		if v := m.buf.Version(); s.brainV == v && s.checkedV == v {
			return m.brickRunChecked(), true // read and built while you typed: it starts at once
		}
		s.runAfter = true // the newest words are read first, then it runs
		m.say("Reading what you just wrote, then running it ...")
		return tea.Batch(m.brickColour(), m.brickBrain()), true
	case "alt+c":
		s.watched, s.manual, s.brainAsked = -1, true, -1
		m.say("Checking every line again ...")
		return tea.Batch(m.brickColour(), m.brickBrain()), true
	case "alt+p":
		s.focus = !s.focus
		if s.focus {
			m.say("The panel has the keys: " + brFocusKeys + ". Esc gives them back to the text.")
		} else {
			m.say("The text has the keys again.")
		}
		return nil, true
	case "altgr+d", "ð":
		return m.brickDiarySwitch(), true // brick learns from your diary too, or stops
	case "altgr+v", "“":
		return m.brickVoicesSwitch(), true
	}
	if !s.focus {
		return nil, false
	}
	switch k {
	case "esc", "alt+p":
		s.focus = false
		m.say("The text has the keys again.")
	case "left":
		s.pick = max(0, s.pick-1)
	case "right":
		s.pick = min(s.pick+1, max(0, len(list(s.repair, "fixes"))-1))
	case "enter":
		return m.brickWriteFix(), true
	case "b":
		return m.brickBytes(), true
	case "s":
		return m.brickSaveBits(), true
	}
	return nil, true
}

const brFocusKeys = "Left/Right pick a fix · Enter writes it in · B bytes of this line · S save the bits"

// brickWriteFix writes the chosen fix of the brain's into the text (Enter, with the panel's keys).
func (m *Model) brickWriteFix() tea.Cmd {
	s := m.br()
	fixes := list(s.repair, "fixes")
	if len(fixes) == 0 {
		m.say("The brain has no fix to write in right now.")
		return nil
	}
	text := str(obj(fixes[min(s.pick, len(fixes)-1)]), "text")
	last := len(m.buf.Lines) - 1
	m.buf.Change(Pos{0, 0}, Pos{last, len(m.buf.Lines[last])}, text, "the brain's fix")
	s.repair, s.focus = nil, false
	m.say("Fixed. Ctrl+Z takes it back.")
	return tea.Batch(m.brickColour(), m.brickBrain())
}

// brickReady: the program the latest check built, when it is for the text as it is now (it starts at once).
func (m *Model) brickReady() string {
	s := m.br()
	if s.watch == nil || s.checkedV != m.buf.Version() || !yes(s.watch, "ok") {
		return ""
	}
	return str(s.watch, "ready")
}

// brickRunChecked runs with the brain's verdict in hand: a precise fix is
// written in first (Ctrl+Z takes it back), so the bits do what was meant.
func (m *Model) brickRunChecked() tea.Cmd {
	s := m.br()
	r := s.repair
	fixes := list(r, "fixes")
	said := func() string {
		if len(s.fixed) == 0 {
			return ""
		}
		x := "The brain fixed " + strings.Join(s.fixed, "; ") + " (Ctrl+Z takes it back). "
		s.fixed, s.fixedSeen = nil, nil
		return x
	}
	if r == nil {
		if p := m.brickReady(); p != "" { // built while you typed: it starts at once
			folder := filepath.Dir(p)
			if m.buf.Path != "" {
				folder = filepath.Dir(m.buf.Path)
			}
			m.say(said() + "Running it.")
			return tea.Batch(runProgram(p, folder), m.brickAsk("keep", map[string]any{"text": m.buf.Text(), "path": m.buf.Path}))
		}
		return m.brickRun(strings.TrimSpace(said() + " every line read"))
	}
	if (yes(r, "precise") || yes(r, "picked")) && len(fixes) > 0 && len(s.fixed) < 8 && !s.fixedSeen[str(obj(fixes[0]), "text")] {
		f := obj(fixes[0])
		var ch []string
		for _, c := range list(f, "changes") {
			cs, _ := c.(string)
			ch = append(ch, cs)
		}
		if s.fixedSeen == nil {
			s.fixedSeen = map[string]bool{}
		}
		s.fixedSeen[str(f, "text")] = true
		guess := ""
		if !yes(r, "precise") {
			guess = " (its best guess)"
		}
		s.fixed = append(s.fixed, fmt.Sprintf("line %d: %s%s", int(num(r, "number")), strings.Join(ch, "; "), guess))
		last := len(m.buf.Lines) - 1
		m.buf.Change(Pos{0, 0}, Pos{last, len(m.buf.Lines[last])}, str(f, "text"), "the brain's fix")
		s.repair, s.runAfter = nil, true // read the fixed program again: the next line may need a fix too
		m.say(fmt.Sprintf("The brain fixed line %d; reading the rest ...", int(num(r, "number"))))
		return tea.Batch(m.brickColour(), m.brickBrain())
	}
	return m.brickRun(strings.TrimSpace(said() + fmt.Sprintf(" the brain isn't sure line %d does what you meant (its fixes are in the panel)", int(num(r, "number")))))
}

// brickBytes: how many bytes the cursor's line builds to, and how brick read it (B, with the panel's keys).
func (m *Model) brickBytes() tea.Cmd {
	line := strings.TrimSpace(string(m.buf.Lines[m.buf.Cursor.Row]))
	if line == "" {
		m.sayErr("Put the cursor on a line first.")
		return nil
	}
	return m.brickAsk("bitstream", map[string]any{"line": line})
}

// brickSaveBits writes the program's bits next to its file (S, with the panel's keys): the program itself, and its parts.
func (m *Model) brickSaveBits() tea.Cmd {
	if m.buf.Draft || m.buf.Path == "" {
		m.sayErr("Save the file first: the bits go next to it.")
		return nil
	}
	m.say("Writing the bits next to the file ...")
	return m.brickAsk("bits", map[string]any{"text": m.buf.Text(), "path": m.buf.Path})
}

// brickBrain asks the brain to check the program as it is now (in the
// background, one check at a time): does every line build what was meant?
func (m *Model) brickBrain() tea.Cmd {
	s := m.br()
	v := m.buf.Version()
	if !m.inBrick() || !m.hl.brick || s.busy["repair"] || s.brainAsked == v {
		return nil
	}
	s.brainAsked = v
	if strings.TrimSpace(m.buf.Text()) == "" {
		s.brainV, s.repair = v, nil
		return nil
	}
	return m.brickAsk("repair", map[string]any{"text": m.buf.Text(), "version": v})
}

// brickRun builds the text as it is and runs it; why says what the brain made of it.
func (m *Model) brickRun(why string) tea.Cmd {
	m.say("Building and running: " + why + ".")
	return m.brickAsk("run", map[string]any{"text": m.buf.Text(), "path": m.buf.Path})
}

// brickLearningSwitch turns the training daemon on or off (F12 in every mode).
func (m *Model) brickLearningSwitch() tea.Cmd {
	what := "always"
	if m.brickLearningLamp {
		what = "off"
	}
	m.brickLearningLamp = !m.brickLearningLamp
	if m.brickLearningLamp {
		m.say("brick learning: on. It learns from everything you write, quietly, in every mode.")
	} else {
		m.say("brick learning: off.")
	}
	return m.brickAsk("daemon", map[string]any{"what": what})
}

// brickVoicesSwitch turns the brains' voices on or off (AltGr+V in brick mode; V in the Learning panel).
func (m *Model) brickVoicesSwitch() tea.Cmd {
	s := m.br()
	s.voices = !s.voices
	what := "off"
	if s.voices {
		what = "on"
		m.say("The brains' voices: on. Each archetype says what it's learning, in its own voice, now and then. AltGr+V turns them off.")
	} else {
		m.say("The brains' voices: off. AltGr+V turns them on.")
	}
	return m.brickAsk("narrator", map[string]any{"what": what})
}

func (m *Model) brickDiarySwitch() tea.Cmd {
	on := !brickDiaryOn()
	setBrickDiary(on)
	if on {
		m.say("brick learns from your diary too now (it never leaves your PC). AltGr+D turns it off.")
	} else {
		m.say("brick no longer reads your diary. AltGr+D turns it on.")
	}
	return nil
}

// brStarter: lines brick builds as they are, for an empty program.
var brStarter = []string{"write hello in red", "wait 2 seconds", "write goodbye in blue at the bottom"}

// brickRows draws the panel under the text: one top line (how the program
// stands, and the keys), then the brain's reading of each line.
func (m *Model) brickRows(sb *strings.Builder) {
	w := m.width
	base := st(cText, cBG)
	rows := m.brickPanel(w)
	for len(rows) < brPanelH {
		rows = append(rows, "")
	}
	for _, r := range rows[:brPanelH] {
		sb.WriteString(fit(r, w, base))
		sb.WriteByte('\n')
	}
}

// brickSame: the brain read the line as it was written (case, spaces and quotes aside).
func brickSame(a, b string) bool {
	norm := func(x string) string {
		x = strings.ToLower(strings.NewReplacer(`"`, "", "'", "", ",", "", ".", "", ":", "").Replace(x))
		return strings.Join(strings.Fields(x), " ")
	}
	return norm(a) == norm(b)
}

func (m *Model) brickPanel(w int) []string {
	s := m.br()
	text := st(cText, cBG)
	dim := st(cDim, cBG)
	hot := st(cEmber, cBG).Bold(true)
	bad := st(cError, cBG).Bold(true)
	v := m.buf.Version()
	cut := func(x string, room int) string {
		if room < 2 {
			return ""
		}
		if utf8.RuneCountInString(x) > room {
			return string([]rune(x)[:room-1]) + "…"
		}
		return x
	}

	// the top line: how the program stands, then the keys
	title := st(cBG, cEmber).Bold(true).Render(" " + icon("\U000f09d1", "◉") + " brick's brain ")
	var status string
	lines := list(s.watch, "lines")
	fresh := s.watch != nil && s.checkedV == v
	r := s.repair
	switch {
	case !m.hl.brick:
		status = dim.Render("this isn't a brick file")
	case strings.TrimSpace(m.buf.Text()) == "":
		status = dim.Render("write a line to start")
	case !fresh:
		status = dim.Render("reading ...")
	case !yes(s.watch, "ok"):
		status = bad.Render("✗ It doesn't build yet") + dim.Render(": the line marked ✗ says why")
	case s.brainV != v:
		status = text.Render(fmt.Sprintf("✓ It builds: %d bytes", int(num(obj(s.watch["layout"]), "file")))) + dim.Render(" · the brain is checking ...")
	case r != nil:
		status = hot.Render(fmt.Sprintf("? Line %d may not do what you meant", int(num(r, "number")))) + dim.Render(": see under it")
	default:
		status = hot.Render(fmt.Sprintf("✓ Ready: %d bytes", int(num(obj(s.watch["layout"]), "file")))) + dim.Render(" · Alt+R runs it")
	}
	keys := "Alt+R run · Alt+C check again · Alt+P fixes · F1 keys"
	if s.focus {
		keys = brFocusKeys + " · Esc back"
	}
	top := title + "  " + status
	if lipgloss.Width(top)+lipgloss.Width(keys)+3 > w && !s.focus {
		keys = "F1 keys" // too narrow for every key: the line about the program comes first
	}
	if gap := w - lipgloss.Width(top) - lipgloss.Width(keys) - 1; gap >= 2 {
		top += strings.Repeat(" ", gap) + dim.Render(keys)
	} else {
		top += "  " + dim.Render(keys)
	}
	out := []string{top}
	h := brPanelH - 1

	if !m.hl.brick {
		return append(out, dim.Render("  brick reads .txt and .brick files, and sessions."))
	}
	if strings.TrimSpace(m.buf.Text()) == "" {
		out = append(out, text.Render("  Write what the program should do, one plain line at a time. For example:"))
		for _, l := range brStarter {
			out = append(out, st(m.styleColour(sString), cBG).Render("      "+l))
		}
		return append(out,
			text.Render("  Here, each line shows what the brain understood, and how many bytes it builds."),
			text.Render("  It is built while you type, so ")+hot.Render("Alt+R")+text.Render(" runs it at once."))
	}

	// the brain's reading, one row per line (blank lines and notes left out), around the cursor's line
	type row struct {
		line int // 0-based; -1 for a fix under the line before
		s    string
	}
	info := map[int]map[string]any{}
	for _, l := range lines {
		o := obj(l)
		info[int(num(o, "number"))-1] = o
	}
	kinds := map[int]string{}
	for i, l := range s.lines {
		kinds[i] = str(obj(l), "kind")
	}
	var rows []row
	cursorAt := 0
	for i, raw := range m.buf.Lines {
		typed := strings.TrimSpace(string(raw))
		if typed == "" || strings.HasPrefix(typed, "#") {
			continue
		}
		if i <= m.buf.Cursor.Row {
			cursorAt = len(rows)
		}
		mark := "  "
		if i == m.buf.Cursor.Row {
			mark = hot.Render("▸ ")
		}
		no := dim.Render(fmt.Sprintf("%3d ", i+1))
		o := info[i]
		var meant []string
		for _, x := range list(o, "meant") {
			xs, _ := x.(string)
			meant = append(meant, xs)
		}
		bytes := ""
		if b := int(num(o, "bytes")); b > 0 && fresh {
			bytes = fmt.Sprintf("%d bytes", b)
		}
		room := w - 8 - len(bytes) - 2
		var body string
		switch {
		case !fresh || o == nil:
			body = dim.Render("… " + cut(typed, room-2))
		case str(o, "problem") != "":
			body = bad.Render("✗ ") + text.Render(cut(str(o, "problem"), room-2))
		case r != nil && int(num(r, "number")) == i+1:
			why := str(r, "why")
			if why == "" {
				why = "it may not do what you meant"
			}
			body = hot.Render("? ") + text.Render(cut(why, room-2))
		case len(meant) == 0 && kinds[i] == "definition":
			body = text.Render("✓ ") + dim.Render(cut("teaches brick a new word: "+typed, room-2))
		case len(meant) == 0:
			body = dim.Render("· " + cut(typed, room-2))
		case brickSame(strings.Join(meant, " "), typed):
			body = text.Render("✓ " + cut(strings.Join(meant, " · "), room-2))
		default:
			body = hot.Render("→ ") + dim.Render("understood as: ") + hot.Render(cut(strings.Join(meant, " · then "), room-17))
		}
		left := mark + no + body
		if bytes != "" {
			if gap := w - lipgloss.Width(left) - len(bytes) - 1; gap > 0 {
				left += strings.Repeat(" ", gap) + dim.Render(bytes)
			}
		}
		rows = append(rows, row{i, left})
		if r != nil && int(num(r, "number")) == i+1 && fresh { // the brain's fixes, under its line
			fixes := list(r, "fixes")
			if len(fixes) == 0 {
				rows = append(rows, row{-1, dim.Render("        No fix built what you meant. Try other words for it.")})
			}
			for k, f := range fixes {
				if k == 3 {
					break
				}
				var ch []string
				for _, c := range list(obj(f), "changes") {
					cs, _ := c.(string)
					ch = append(ch, cs)
				}
				label := fmt.Sprintf("fix %d: ", k+1)
				if k == 0 && (yes(r, "precise") || yes(r, "picked")) {
					label = "brain's fix: "
				}
				line := "        " + label + cut(strings.Join(ch, "; "), w-12-len(label)-30)
				chosen := k == min(s.pick, len(fixes)-1)
				switch {
				case chosen && s.focus:
					line = st(cBG, cEmber).Render(line+" ") + dim.Render("  Enter writes it in")
				case chosen && k == 0 && (yes(r, "precise") || yes(r, "picked")):
					line = text.Render(line) + dim.Render("  Alt+R writes it in and runs")
				case chosen:
					line = text.Render(line) + dim.Render("  Alt+P, then Enter writes it in")
				default:
					line = dim.Render(line)
				}
				rows = append(rows, row{-1, line})
			}
		}
	}
	top0 := max(0, min(cursorAt-h/2, len(rows)-h))
	for i := top0; i < len(rows) && len(out) < brPanelH; i++ {
		out = append(out, rows[i].s)
	}
	return out
}

// brickTile draws one brick: its words on its kind's colour.
func (m *Model) brickTile(kind, text string, count int, here bool) string {
	style, ok := brKindStyle[kind]
	if !ok {
		style = sString
	}
	label := " " + text + " "
	if count > 1 {
		label = fmt.Sprintf(" %s ×%d ", text, count)
	}
	t := st(cBG, m.styleColour(style))
	if here {
		t = t.Bold(true).Underline(true) // a brick on the cursor's line
	}
	return t.Render(label)
}
