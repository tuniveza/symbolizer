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
// coloured by what brick reads it as. Under it, a panel with brick's whole
// system (it replaces brick's old web window):
//
//	Bricks       is the program right? what the brain thinks of it; every brick in use, live as you type
//	Words        words brick doesn't know in this file, and the words you taught (type "zap means write")
//	Decisions    what brick's trainer couldn't settle by itself: Left/Right pick a reading, Enter keeps it
//	Imagination  what brick imagined, with its pictures, and what its eye saw
//	Neurons      the imagination neurons read the cursor's line (Left/Right pick an archetype's brain)
//	Learning     the training daemon: on/off, the diary switch, what it learned
//
//	Alt+R  run the program (a kitty window beside this one)   Alt+C  check it now, and look for fixes
//	Alt+P  keys to the panel / back to the text               Alt+N  next panel
//	F12 (in every mode) brick learning on / off.  Alt tapped twice, then B (in every mode): brick mode.
//
// The brain checks the program by itself while you type (once you pause):
// does every line build what you meant? When it has a precise fix, Alt+R
// writes that fix in (Ctrl+Z takes it back) before building, so the
// program's bits do what the words meant.
//
// brick runs as `brick.py serve` in the background (brick_client.go); all
// the work is brick's own.

const brickMode = 7 // its place in modes
const brPanelH = 10 // rows the panel takes under the text

var brTabs = []string{"Bricks", "Words", "Decisions", "Imagination", "Neurons", "Learning"}

type brState struct {
	tab, sel, pick, arch                                            int
	focus                                                           bool   // the panel has the keys
	typed                                                           string // what's typed in the Words panel
	watch, words, decisions, imagination, neurons, learning, daemon map[string]any
	archetypes                                                      []any
	cursorLine                                                      map[string]any // the cursor's line as bricks
	repair                                                          map[string]any // a line the brain judged not precise, and its fixes
	lines                                                           []any          // every line as bricks (brick's latest reading): the wall of bricks in use
	watched, coloured                                               int            // buffer versions
	brainAsked, brainV                                              int            // the version the brain was asked about / its verdict (repair) is for
	manual, runAfter                                                bool           // Alt+C asked (say what it found) / Alt+R waits for the brain, then builds
	busy                                                            map[string]bool
	note                                                            string
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
		m.brick = &brState{busy: map[string]bool{}, watched: -1, brainAsked: -1, brainV: -1}
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
	s := m.br()
	m.hl.brick = m.brickFile()
	m.hl.done = -1 // colour it brick's way now
	cmds := []tea.Cmd{m.brickAsk("decisions", nil), m.brickAsk("daemon", nil), m.brickColour()}
	if s.archetypes == nil {
		cmds = append(cmds, m.brickAsk("archetypes", nil))
	}
	return tea.Batch(cmds...)
}

func (m *Model) leaveBrick() {
	m.hl.brick = false
	m.hl.done = -1
}

// brickColour asks brick to read every line (the colours), and to check the program.
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
	if s.watched != v && !s.busy["watch"] && !s.busy["check"] {
		what := "check" // the light check while typing; Alt+C asks for the full report (watch)
		if s.watched == -1 {
			what = "watch"
		}
		s.watched = v
		cmds = append(cmds, m.brickAsk(what, map[string]any{"text": text, "path": m.buf.Path}))
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
	case "watch", "check":
		s.watch = d
		return tea.Batch(next, m.brickBrain())
	case "words":
		if _, isList := d["here"]; isList {
			s.words = d
		} else {
			m.say("brick: " + str(d, "message"))
			return tea.Batch(next, m.brickAsk("words", map[string]any{"what": "list", "text": m.buf.Text(), "path": m.buf.Path}))
		}
	case "decisions":
		s.decisions = d
	case "decide", "decide_group":
		m.say("brick: " + str(d, "message"))
		return tea.Batch(next, m.brickAsk("decisions", nil))
	case "imagination":
		s.imagination = d
	case "archetypes":
		s.archetypes = list(d, "archetypes")
	case "neurons":
		s.neurons = d
		if e := str(d, "error"); e != "" {
			m.sayErr("neurons: " + e)
		}
	case "learning":
		s.learning = d
	case "daemon":
		s.daemon = d
		m.brickLearningLamp = yes(d, "running")
	case "run":
		if p := str(d, "program"); p != "" {
			return tea.Batch(next, runProgram(p, str(d, "folder")))
		}
		s.watch = map[string]any{"ok": false, "problems": list(d, "problems")}
		s.tab = 0
		m.sayErr("It didn't build: see the Bricks panel.")
	case "repair":
		v := int(num(d, "version"))
		if v != m.buf.Version() {
			return tea.Batch(next, m.brickBrain()) // you typed since: the brain reads it again
		}
		s.repair, s.brainV, s.pick = obj(d["repair"]), v, 0
		if s.manual {
			s.manual = false
			if s.repair == nil {
				m.say("brick: every line is precise (the brain expected what brick built).")
			} else {
				s.tab = 0
				m.say("brick: line " + fmt.Sprint(int(num(s.repair, "number"))) + " isn't precise: see the Bricks panel.")
			}
		}
		if s.runAfter {
			s.runAfter = false
			return tea.Batch(next, m.brickRunChecked())
		}
	case "bitstream":
		m.say(fmt.Sprintf("brick: %d bytes from \"%s\" (%s)", int(num(d, "bytes")), str(d, "line"), str(d, "why")))
	}
	return next
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
			return m.brickRun("as written (not a brick file, so the brain didn't check it)"), true
		}
		if s.brainV == m.buf.Version() {
			return m.brickRunChecked(), true
		}
		s.runAfter = true // the brain checks the newest words first, then it builds
		m.say("The brain is checking what you wrote first (a few seconds) ...")
		return m.brickBrain(), true
	case "alt+c":
		s.watched, s.tab, s.manual = -1, 0, true
		s.brainAsked = -1
		m.say("Checking, and looking for anything missing or misread (a few seconds) ...")
		return tea.Batch(m.brickColour(), m.brickBrain()), true
	case "alt+p":
		s.focus = !s.focus
		if s.focus {
			m.say("The panel has the keys: " + brFocusKeys[brTabs[s.tab]] + ". Alt+P or Esc gives them back to the text.")
		} else {
			m.say("The text has the keys again.")
		}
		return m.brickTabOpened(), true
	case "alt+n":
		s.tab = (s.tab + 1) % len(brTabs)
		s.sel, s.pick = 0, 0
		m.say(brTabHelp[brTabs[s.tab]])
		return m.brickTabOpened(), true
	}
	if !s.focus {
		return nil, false
	}
	switch k {
	case "esc":
		s.focus = false
		m.say("The text has the keys again.")
		return nil, true
	case "tab":
		s.tab = (s.tab + 1) % len(brTabs)
		s.sel, s.pick = 0, 0
		m.say(brTabHelp[brTabs[s.tab]])
		return m.brickTabOpened(), true
	case "shift+tab":
		s.tab = (s.tab + len(brTabs) - 1) % len(brTabs)
		s.sel, s.pick = 0, 0
		m.say(brTabHelp[brTabs[s.tab]])
		return m.brickTabOpened(), true
	case "up":
		s.sel = max(0, s.sel-1)
		s.pick = 0
		return nil, true
	case "down":
		s.sel++
		s.pick = 0
		return nil, true
	case "left":
		s.pick = max(0, s.pick-1)
		if brTabs[s.tab] == "Neurons" && len(s.archetypes) > 0 {
			s.arch = (s.arch + len(s.archetypes)) % (len(s.archetypes) + 1)
		}
		return nil, true
	case "right":
		s.pick++
		if brTabs[s.tab] == "Neurons" && len(s.archetypes) > 0 {
			s.arch = (s.arch + 1) % (len(s.archetypes) + 1)
		}
		return nil, true
	case "enter":
		return m.brickEnter(), true
	case "backspace":
		if s.typed != "" {
			_, n := utf8.DecodeLastRuneInString(s.typed)
			s.typed = s.typed[:len(s.typed)-n]
		}
		return nil, true
	}
	if brTabs[s.tab] == "Words" && text != "" {
		s.typed += text
		return nil, true
	}
	if brTabs[s.tab] == "Decisions" && (k == "d" || k == "delete") {
		if it := m.brickDecision(); it != nil {
			return m.brickAsk("decide", map[string]any{"id": str(it, "id"), "action": "dismiss"}), true
		}
	}
	if brTabs[s.tab] == "Learning" && k == "d" {
		return m.brickDiarySwitch(), true
	}
	return nil, true
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

// brickRunChecked builds with the brain's verdict in hand: a precise fix is
// written in first (Ctrl+Z takes it back), so the bits do what was meant.
func (m *Model) brickRunChecked() tea.Cmd {
	s := m.br()
	r := s.repair
	fixes := list(r, "fixes")
	if r == nil {
		return m.brickRun("the brain checked every line")
	}
	if yes(r, "precise") && len(fixes) > 0 {
		f := obj(fixes[0])
		var ch []string
		for _, c := range list(f, "changes") {
			cs, _ := c.(string)
			ch = append(ch, cs)
		}
		last := len(m.buf.Lines) - 1
		m.buf.Change(Pos{0, 0}, Pos{last, len(m.buf.Lines[last])}, str(f, "text"), "the brain's fix")
		s.repair, s.brainV = nil, m.buf.Version() // what the brain picked is now the text
		s.brainAsked = s.brainV
		return tea.Batch(m.brickColour(), m.brickRun(fmt.Sprintf("the brain fixed line %d first: %s (Ctrl+Z takes it back)", int(num(r, "number")), strings.Join(ch, "; "))))
	}
	s.tab = 0
	return m.brickRun(fmt.Sprintf("the brain isn't sure line %d does what you meant: see the Bricks panel", int(num(r, "number"))))
}

// brickRun builds the text as it is and runs it; why says what the brain made of it.
func (m *Model) brickRun(why string) tea.Cmd {
	m.say("Building and running: " + why + ".")
	return m.brickAsk("run", map[string]any{"text": m.buf.Text(), "path": m.buf.Path})
}

// What each panel is for (said when it opens), and its keys while it has them.
var brTabHelp = map[string]string{
	"Bricks":      "Bricks: does it build, what the brain thinks, and every brick you've used. Alt+R runs it.",
	"Words":       "Words: words brick doesn't know yet. Alt+P, then type: zap means write, then Enter.",
	"Decisions":   "Decisions: readings brick couldn't settle. Alt+P, Up/Down choose, Left/Right pick, Enter keeps it.",
	"Imagination": "Imagination: pictures brick imagined while learning, and what its eye saw. Alt+P, then Up/Down.",
	"Neurons":     "Neurons: how the brain reads the cursor's line. Alt+P, Left/Right pick a brain, Enter reads.",
	"Learning":    "Learning: brick learning in the background. Alt+P, Enter turns it on/off, d for your diary.",
}

var brFocusKeys = map[string]string{
	"Bricks":      "Left/Right pick a fix · Enter writes it in",
	"Words":       "type: zap means write · Enter teaches it",
	"Decisions":   "Up/Down choose · Left/Right pick · Enter keeps · d dismisses",
	"Imagination": "Up/Down look",
	"Neurons":     "Left/Right pick a brain · Enter reads the line",
	"Learning":    "Enter on/off · d diary",
}

// brickTabOpened fetches what the chosen panel shows.
func (m *Model) brickTabOpened() tea.Cmd {
	s := m.br()
	switch brTabs[s.tab] {
	case "Words":
		return m.brickAsk("words", map[string]any{"what": "list", "text": m.buf.Text(), "path": m.buf.Path})
	case "Decisions":
		return m.brickAsk("decisions", nil)
	case "Imagination":
		return m.brickAsk("imagination", map[string]any{"most": 30})
	case "Learning":
		return tea.Batch(m.brickAsk("daemon", nil), m.brickAsk("learning", nil))
	}
	return nil
}

func (m *Model) brickDecision() map[string]any {
	items := list(m.br().decisions, "items")
	if len(items) == 0 {
		return nil
	}
	return obj(items[min(m.br().sel, len(items)-1)])
}

func (m *Model) brickEnter() tea.Cmd {
	s := m.br()
	switch brTabs[s.tab] {
	case "Words":
		t := strings.TrimSpace(s.typed)
		parts := strings.SplitN(strings.Replace(t, " = ", " means ", 1), " means ", 2)
		if len(parts) != 2 {
			m.sayErr(`Type it like this: zap means write`)
			return nil
		}
		s.typed = ""
		return m.brickAsk("words", map[string]any{"what": "teach", "word": strings.TrimSpace(parts[0]), "meaning": strings.TrimSpace(parts[1])})
	case "Decisions":
		it := m.brickDecision()
		if it == nil {
			return nil
		}
		ways := list(it, "could be")
		if len(ways) == 0 {
			return nil
		}
		way, _ := ways[min(s.pick, len(ways)-1)].(string)
		action := "means"
		if strings.Contains(way, " = ") || strings.Contains(way, ": ") || strings.HasPrefix(way, "to ") {
			action = "word"
		}
		return m.brickAsk("decide", map[string]any{"id": str(it, "id"), "action": action, "meaning": way})
	case "Neurons":
		line := strings.TrimSpace(string(m.buf.Lines[m.buf.Cursor.Row]))
		if line == "" {
			m.sayErr("Put the cursor on a line to read.")
			return nil
		}
		arch := ""
		if s.arch > 0 && s.arch <= len(s.archetypes) {
			arch = str(obj(s.archetypes[s.arch-1]), "name")
		}
		m.say("The neurons are reading it (a few seconds) ...")
		return m.brickAsk("neurons", map[string]any{"line": line, "archetype": arch})
	case "Learning":
		return m.brickLearningSwitch()
	case "Bricks":
		if fixes := list(s.repair, "fixes"); len(fixes) > 0 {
			text := str(obj(fixes[min(s.pick, len(fixes)-1)]), "text")
			last := len(m.buf.Lines) - 1
			m.buf.Change(Pos{0, 0}, Pos{last, len(m.buf.Lines[last])}, text, "brick repair")
			s.repair, s.brainV = nil, m.buf.Version()
			s.brainAsked = s.brainV
			m.say("Fixed. Ctrl+Z takes it back.")
			return m.brickColour()
		}
		line := strings.TrimSpace(string(m.buf.Lines[m.buf.Cursor.Row]))
		return m.brickAsk("bitstream", map[string]any{"line": line})
	}
	return nil
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

func (m *Model) brickDiarySwitch() tea.Cmd {
	on := !brickDiaryOn()
	setBrickDiary(on)
	if on {
		m.say("brick learns from your diary too now (it never leaves your PC).")
	} else {
		m.say("brick no longer reads your diary.")
	}
	return nil
}

// brickRows draws the panel under the text.
func (m *Model) brickRows(sb *strings.Builder) {
	s := m.br()
	w := m.width
	base := st(cText, cBG)
	dim := st(cDim, cBG)
	hot := st(cEmber, cBG).Bold(true)
	var tabs strings.Builder
	tabs.WriteString(hot.Render(" " + modes[brickMode].Icon() + " brick  "))
	for i, t := range brTabs {
		if i == s.tab {
			tabs.WriteString(st(cBG, cEmber).Bold(true).Render(" " + t + " "))
		} else {
			tabs.WriteString(dim.Render(" " + t + " "))
		}
	}
	keys := "  Alt+R run · Alt+C check · Alt+P panel · Alt+N next · F1 help"
	if s.focus {
		keys = "  " + brFocusKeys[brTabs[s.tab]] + " · Alt+P/Esc back"
	}
	tabs.WriteString(dim.Render(keys))
	rows := []string{tabs.String()}
	rows = append(rows, m.brickPanel(w)...)
	for len(rows) < brPanelH {
		rows = append(rows, "")
	}
	for _, r := range rows[:brPanelH] {
		sb.WriteString(fit(r, w, base))
		sb.WriteByte('\n')
	}
}

func (m *Model) brickPanel(w int) []string {
	s := m.br()
	text := st(cText, cBG)
	dim := st(cDim, cBG)
	hot := st(cEmber, cBG).Bold(true)
	h := brPanelH - 1
	clip := func(x string) string {
		if utf8.RuneCountInString(x) > w-2 {
			return string([]rune(x)[:max(0, w-3)]) + "…"
		}
		return x
	}
	choose := func(items []string) []string { // a window of rows around the chosen one
		s.sel = min(s.sel, max(0, len(items)-1))
		top := max(0, min(s.sel-h/2, len(items)-h))
		out := []string{}
		for i := top; i < len(items) && len(out) < h; i++ {
			mark := "  "
			if i == s.sel && s.focus {
				mark = hot.Render("▸ ")
			}
			out = append(out, mark+text.Render(clip(items[i])))
		}
		return out
	}
	switch brTabs[s.tab] {
	case "Bricks":
		return m.brickBricksPanel(w, h)
	case "Words":
		out := []string{text.Render("  Teach: ") + hot.Render(s.typed+"▏") + dim.Render("   (type: zap means write, then Enter)")}
		if s.words == nil {
			return append(out, dim.Render("  Loading ..."))
		}
		var items []string
		for _, u := range list(s.words, "unknown") {
			us, _ := u.(string)
			items = append(items, "not known yet: "+us)
		}
		for _, e := range append(list(s.words, "here"), list(s.words, "every")...) {
			items = append(items, "taught: "+str(obj(e), "line"))
		}
		if len(items) == 0 {
			items = []string{"No new words in this file, and none taught yet."}
		}
		h--
		out = append(out, choose(items)...)
		return out
	case "Decisions":
		items := list(s.decisions, "items")
		if s.decisions == nil {
			return []string{dim.Render("  Loading ...")}
		}
		if len(items) == 0 {
			return []string{text.Render("  Nothing to decide: brick settled everything by itself."), dim.Render("  " + str(obj(map[string]any{"a": firstString(list(s.decisions, "activity"))}), "a"))}
		}
		var rows []string
		for _, it := range items {
			o := obj(it)
			rows = append(rows, fmt.Sprintf("'%s' — %s", str(o, "line"), str(o, "said")))
		}
		out := choose(rows)
		if it := m.brickDecision(); it != nil && s.focus {
			ways := list(it, "could be")
			var line strings.Builder
			line.WriteString(dim.Render("  could be: "))
			for i, wv := range ways {
				ws, _ := wv.(string)
				if i == min(s.pick, len(ways)-1) {
					line.WriteString(st(cBG, cEmber).Render(" " + ws + " "))
				} else {
					line.WriteString(text.Render(" " + ws + " "))
				}
			}
			out = append(out[:min(len(out), h-1)], line.String()+dim.Render("   Enter keeps it · d dismisses"))
		}
		return out
	case "Imagination":
		items := list(s.imagination, "items")
		if s.imagination == nil {
			return []string{dim.Render("  Loading ...")}
		}
		if len(items) == 0 {
			return []string{text.Render("  Nothing imagined yet. brick dreams while it learns.")}
		}
		var rows []string
		for _, it := range items {
			o := obj(it)
			rows = append(rows, fmt.Sprintf("%s · %s · eye saw: %s", str(o, "label"), str(o, "recipe"), str(o, "eye")))
		}
		listW := w * 3 / 5
		left := choose(rows)
		sel := obj(items[min(s.sel, len(items)-1)])
		pic := ""
		if p := str(sel, "picture"); p != "" {
			if block, ok := pictureBlock(p, w-listW-4, h, st(cText, cBG).Render(" ")); ok {
				pic = block
			}
		}
		picRows := strings.Split(pic, "\n")
		out := []string{}
		for i := 0; i < h; i++ {
			l := ""
			if i < len(left) {
				l = left[i]
			}
			r := ""
			if i < len(picRows) {
				r = picRows[i]
			}
			out = append(out, fit(l, listW, text)+"  "+r)
		}
		return out
	case "Neurons":
		name := "no archetype (neutral)"
		if s.arch > 0 && s.arch <= len(s.archetypes) {
			a := obj(s.archetypes[s.arch-1])
			name = str(a, "name") + " · " + str(a, "type")
		}
		out := []string{text.Render("  Brain: ") + hot.Render(name) + dim.Render("   (Left/Right change it · Enter reads the cursor's line)")}
		if s.neurons == nil {
			return out
		}
		for _, r := range list(s.neurons, "rows") {
			o := obj(r)
			out = append(out, text.Render(clip(fmt.Sprintf("  %-12s %-22s %-28s %s", str(o, "word"), str(o, "meaning"), str(o, "sure"), str(o, "senses")))))
		}
		return out
	case "Learning":
		d := s.daemon
		state := "off"
		if yes(d, "running") {
			state = "on (learning now, at the lowest priority)"
		}
		diary := "off"
		if brickDiaryOn() {
			diary = "on"
		}
		out := []string{
			text.Render("  Learning: ") + hot.Render(state) + dim.Render("   Enter (or F12 anywhere) turns it on/off"),
			text.Render("  Your diary: ") + hot.Render(diary) + dim.Render("   d turns it on/off (all writing stays on your PC)"),
		}
		if d != nil && num(d, "rounds") > 0 {
			out = append(out, text.Render(fmt.Sprintf("  %d rounds · %d neurons (%d phrases) · %d lines, %d proved · Ni guesses %d%% of next words, %d%% of brick's output",
				int(num(d, "rounds")), int(num(d, "neurons")), int(num(d, "phrases")), int(num(d, "lines")), int(num(d, "proved")),
				int(num(d, "ni")*100), int(num(d, "bits")*100))))
			out = append(out, text.Render(fmt.Sprintf("  New bricks made: %d", int(num(d, "bricks_made")))))
		}
		if l := s.learning; l != nil {
			var same []string
			for _, p := range list(l, "proved") {
				pp := p.([]any)
				if len(pp) == 3 && pp[2] == "meaning" {
					same = append(same, fmt.Sprintf("%v~%v", pp[0], pp[1]))
				}
			}
			if len(same) > 0 {
				out = append(out, dim.Render("  Proved to mean the same: "+clip(strings.Join(same, ", "))))
			}
			var bricks []string
			for _, b := range list(l, "bricks") {
				bs, _ := b.(string)
				bricks = append(bricks, bs)
			}
			if len(bricks) > 0 {
				out = append(out, dim.Render("  Suggested bricks: "+clip(strings.Join(bricks, ", "))))
			}
		}
		return out
	}
	return nil
}

func firstString(l []any) string {
	if len(l) == 0 {
		return ""
	}
	s, _ := l[0].(string)
	return s
}

// brKinds: the kinds of brick, in the order the wall shows them.
var brKinds = []struct{ kind, label string }{
	{"do", "do"}, {"thing", "things"}, {"colour", "colours"}, {"place", "places"}, {"time", "time"},
	{"count", "count"}, {"saying", "saying"}, {"glue", "joining"}, {"words", "words to show"},
}

// brStarter: lines brick builds as they are, for an empty program.
var brStarter = []string{"write hello in red", "wait 2 seconds", "write goodbye in blue at the bottom", "repeat 3 times: write hi"}

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

// flow lays pieces left to right in rows of width w, at most rows rows;
// what doesn't fit is counted at the end.
func flow(pieces []string, w, rows int, dim lipgloss.Style) []string {
	var out []string
	line, lw := "  ", 2
	for i, p := range pieces {
		pw := lipgloss.Width(p)
		if lw+pw > w-1 && lw > 2 {
			if len(out) == rows-1 {
				return append(out, line+dim.Render(fmt.Sprintf("… +%d more", len(pieces)-i)))
			}
			out = append(out, line)
			line, lw = "  ", 2
		}
		line += p + " "
		lw += pw + 1
	}
	if lw > 2 {
		out = append(out, line)
	}
	return out
}

// brickBricksPanel: does it build, what the brain thinks, and the wall of
// every brick in use (live as you type), with the cursor's line under it.
func (m *Model) brickBricksPanel(w, h int) []string {
	s := m.br()
	text := st(cText, cBG)
	dim := st(cDim, cBG)
	hot := st(cEmber, cBG).Bold(true)
	bad := st(cError, cBG).Bold(true)
	clip := func(x string) string {
		if utf8.RuneCountInString(x) > w-6 {
			return string([]rune(x)[:max(0, w-7)]) + "…"
		}
		return x
	}
	if !m.hl.brick {
		return []string{dim.Render("  This file isn't a brick program. brick reads .txt and .brick files, and sessions.")}
	}
	if strings.TrimSpace(m.buf.Text()) == "" {
		out := []string{hot.Render("  Start here: ") + text.Render("write what the program should do, one plain-English line at a time. For example:")}
		for _, l := range brStarter {
			out = append(out, st(m.styleColour(sString), cBG).Render("      "+l))
		}
		return append(out,
			text.Render("  Each word becomes a coloured brick here as you type: the colour says what brick reads it as."),
			text.Render("  The brain checks every line while you type. ")+hot.Render("Alt+R")+text.Render(" builds it and runs it."),
			dim.Render("  Stuck on a word? Alt+N to Words teaches brick a new one. F1 shows every brick key."))
	}
	var out []string
	// 1. does it build?
	switch {
	case s.watch == nil:
		out = append(out, dim.Render("  Checking the program ..."))
	case yes(s.watch, "ok"):
		lay := obj(s.watch["layout"])
		out = append(out, hot.Render(fmt.Sprintf("  ✓ It builds: %d bytes in %.0f ms.", int(num(lay, "file")), num(s.watch, "took")))+dim.Render("  Alt+R runs it."))
	default:
		out = append(out, bad.Render("  ✗ It doesn't build yet.")+dim.Render("  Change the line it names, or Alt+C looks for a fix."))
		for i, p := range list(s.watch, "problems") {
			if i == 2 {
				break
			}
			ps, _ := p.(string)
			out = append(out, text.Render("    "+clip(ps)))
		}
	}
	// 2. what the brain thinks: does every line build what was meant?
	brain := icon("\U000f09d1", "◉") + " "
	r := s.repair
	switch {
	case s.brainV != m.buf.Version():
		out = append(out, dim.Render("  "+brain+"The brain is checking that every line builds what you meant ..."))
	case r == nil:
		out = append(out, hot.Render("  "+brain)+text.Render("The brain checked every line: each builds what the words mean."))
	default:
		out = append(out, bad.Render(fmt.Sprintf("  %sLine %d isn't precise: %s.", brain, int(num(r, "number")), clip(str(r, "why")))))
		fixes := list(r, "fixes")
		if len(fixes) == 0 {
			out = append(out, text.Render("    No fix built what you meant: brick may not have this action yet. Try other words for it."))
		}
		for i, f := range fixes {
			o := obj(f)
			var ch []string
			for _, c := range list(o, "changes") {
				cs, _ := c.(string)
				ch = append(ch, cs)
			}
			chosen := i == min(s.pick, len(fixes)-1)
			mark := "    "
			if chosen {
				mark = "  ▸ "
			}
			label := ""
			switch {
			case i == 0 && yes(r, "precise"):
				label = "brain's pick (precise): "
			case i == 0 && yes(r, "picked"):
				label = "brain's best guess: "
			}
			row := text.Render(mark + label + clip(strings.Join(ch, "; ")))
			if chosen {
				if yes(r, "precise") && i == 0 {
					row += dim.Render("   Alt+R writes it in and builds")
				} else {
					row += dim.Render("   Alt+P, Enter writes it in")
				}
			}
			out = append(out, row)
			if len(out) >= h-4 {
				break
			}
		}
	}
	// 3. the wall: every brick in use, by kind, live as you type.
	type brick struct {
		kind, text string
		count      int
		here       bool
	}
	seen := map[string]*brick{}
	var order []*brick
	total := 0
	for row, l := range s.lines {
		for _, d := range list(obj(l), "directions") {
			ds, _ := d.([]any)
			for _, t := range ds {
				tok := obj(t)
				word := strings.ToLower(strings.TrimSpace(str(tok, "text")))
				if word == "" {
					continue
				}
				key := str(tok, "kind") + "\x00" + word
				b := seen[key]
				if b == nil {
					b = &brick{kind: str(tok, "kind"), text: word}
					seen[key] = b
					order = append(order, b)
				}
				b.count++
				total++
				if row == m.buf.Cursor.Row {
					b.here = true
				}
			}
		}
	}
	room := h - len(out) - 2 // the title row, and the cursor's line at the bottom
	if room > 0 && total > 0 {
		out = append(out, text.Render(fmt.Sprintf("  Bricks in use: %d different, %d in all", len(order), total))+
			dim.Render("  (underlined: on the cursor's line)"))
		var pieces []string
		for _, k := range brKinds {
			first := true
			for _, b := range order {
				if b.kind != k.kind {
					continue
				}
				if first {
					pieces = append(pieces, dim.Render(k.label+":"))
					first = false
				}
				pieces = append(pieces, m.brickTile(b.kind, b.text, b.count, b.here))
			}
		}
		out = append(out, flow(pieces, w, room, dim)...)
	}
	// 4. the cursor's line as bricks, in order: where brick thinks each direction ends.
	if row := m.buf.Cursor.Row; row < len(s.lines) && len(out) < h {
		var line []string
		for i, d := range list(obj(s.lines[row]), "directions") {
			if i > 0 {
				line = append(line, dim.Render("│"))
			}
			ds, _ := d.([]any)
			for _, t := range ds {
				tok := obj(t)
				line = append(line, m.brickTile(str(tok, "kind"), str(tok, "text"), 1, false))
			}
		}
		if len(line) > 0 {
			out = append(out, dim.Render(fmt.Sprintf("  line %d: ", row+1))+strings.Join(line, " "))
		}
	}
	return out
}
