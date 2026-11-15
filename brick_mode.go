package main

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
)

// Brick mode: brick, the plain-English compiler (brick/), inside Symbolizer.
// The editor works as in Text mode, with every word of a brick program
// coloured by what brick reads it as. Under it, a panel with brick's whole
// system (it replaces brick's old web window):
//
//	Bricks       is the program right? problems, size, how long it took; the line under the cursor as bricks
//	Words        words brick doesn't know in this file, and the words you taught (type "zap means write")
//	Decisions    what brick's trainer couldn't settle by itself: Left/Right pick a reading, Enter keeps it
//	Imagination  what brick imagined, with its pictures, and what its eye saw
//	Neurons      the imagination neurons read the cursor's line (Left/Right pick an archetype's brain)
//	Learning     the training daemon: on/off, the diary switch, what it learned
//
//	F6  run the program (a kitty window beside this one)    F7  check it now
//	F8  keys to the panel / back to the text                F9  next panel
//	F12 (in every mode) brick learning on / off
//
// brick runs as `brick.py serve` in the background (brick_client.go); all
// the work is brick's own.

const brickMode = 7 // its place in modes
const brPanelH = 10 // rows the panel takes under the text

var brTabs = []string{"Bricks", "Words", "Decisions", "Imagination", "Neurons", "Learning"}

type brState struct {
	tab, sel, pick, arch int
	focus                bool   // the panel has the keys
	typed                string // what's typed in the Words panel
	watch, words, decisions, imagination, neurons, learning, daemon map[string]any
	archetypes           []any
	cursorLine           map[string]any // the cursor's line as bricks
	repair               map[string]any // a line the brain judged not precise, and its fixes (F7)
	watched, coloured    int            // buffer versions
	busy                 map[string]bool
	note                 string
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
		m.brick = &brState{busy: map[string]bool{}, watched: -1}
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
		what := "check" // the light check while typing; F7 asks for the full report (watch)
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
		}
		m.hl.running = false
		if v != m.buf.Version() && m.inBrick() {
			return tea.Batch(next, m.brickColour())
		}
	case "watch", "check":
		s.watch = d
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
		s.repair = obj(d["repair"])
		if s.repair == nil {
			m.say("brick: every line is precise (the brain expected what brick built).")
		} else {
			s.tab = 0
			m.say("brick: line " + fmt.Sprint(int(num(s.repair, "number"))) + " isn't precise: see the Bricks panel.")
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
	case "f6":
		m.say("Building and running ...")
		return m.brickAsk("run", map[string]any{"text": m.buf.Text(), "path": m.buf.Path}), true
	case "f7":
		s.watched, s.tab, s.repair = -1, 0, nil
		m.say("Checking, and looking for anything missing or misread (a few seconds) ...")
		return tea.Batch(m.brickColour(), m.brickAsk("repair", map[string]any{"text": m.buf.Text()})), true
	case "f8":
		s.focus = !s.focus
		if s.focus {
			m.say("The panel has the keys: Left/Right, Up/Down, Enter. F8 or Esc gives them back to the text.")
		}
		return m.brickTabOpened(), true
	case "f9":
		s.tab = (s.tab + 1) % len(brTabs)
		s.sel, s.pick = 0, 0
		return m.brickTabOpened(), true
	}
	if !s.focus {
		return nil, false
	}
	switch k {
	case "esc":
		s.focus = false
		return nil, true
	case "tab":
		s.tab = (s.tab + 1) % len(brTabs)
		s.sel, s.pick = 0, 0
		return m.brickTabOpened(), true
	case "shift+tab":
		s.tab = (s.tab + len(brTabs) - 1) % len(brTabs)
		s.sel, s.pick = 0, 0
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
			s.repair = nil
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
	keys := "  F6 run · F7 check · F8 panel · F9 next"
	if s.focus {
		keys = "  panel has the keys · F8/Esc back"
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
	bad := st(cError, cBG).Bold(true)
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
		out := []string{}
		if s.watch == nil {
			return []string{dim.Render("  Checking the program ...")}
		}
		if yes(s.watch, "ok") {
			lay := obj(s.watch["layout"])
			out = append(out, hot.Render(fmt.Sprintf("  ✓ It builds: %d bytes in %.0f ms.", int(num(lay, "file")), num(s.watch, "took"))))
		} else {
			out = append(out, bad.Render("  ✗ It doesn't build yet:"))
			for _, p := range list(s.watch, "problems") {
				ps, _ := p.(string)
				out = append(out, text.Render("    "+clip(ps)))
			}
		}
		if r := s.repair; r != nil {
			out = append(out, bad.Render(fmt.Sprintf("  ⚠ Line %d isn't precise: %s.", int(num(r, "number")), clip(str(r, "why")))))
			fixes := list(r, "fixes")
			if len(fixes) == 0 {
				out = append(out, text.Render("    No fix built what the brain expected: brick may not have this action yet."))
			}
			label := "Fix (precise)"
			if !yes(r, "precise") {
				label = "Not sure; best fixes (Left/Right pick)"
			}
			for i, f := range fixes {
				o := obj(f)
				var ch []string
				for _, c := range list(o, "changes") {
					cs, _ := c.(string)
					ch = append(ch, cs)
				}
				mark := "    "
				if i == min(s.pick, len(fixes)-1) {
					mark = "  ▸ "
				}
				if i == 0 {
					out = append(out, hot.Render("  "+label+":   F8 then Enter applies it"))
				}
				out = append(out, text.Render(mark+clip(strings.Join(ch, "; "))))
			}
		}
		for _, n := range list(s.watch, "notes") {
			ns, _ := n.(string)
			out = append(out, dim.Render("  "+clip(ns)))
		}
		out = append(out, dim.Render("  Colours: ")+st(m.styleColour(sKeyword), cBG).Render("do ")+st(m.styleColour(sFunction), cBG).Render("thing ")+
			st(m.styleColour(sConstant), cBG).Render("colour ")+st(m.styleColour(sType), cBG).Render("place ")+st(m.styleColour(sNumber), cBG).Render("time/count ")+
			st(m.styleColour(sString), cBG).Render("words to show ")+dim.Render("· Enter (F8 first): the brain reads this line into bits"))
		return out
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
