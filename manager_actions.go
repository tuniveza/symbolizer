package main

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// Manager actions: Alt+A opens a list of what the manager program
// (~/an/ca/sy/manager/bin/manager) can do. For the chosen row: run, build,
// clean or watch its project, open it in its editor. For all of ~/an: a new
// project, tidy, duplicates, set up the tools and more. F6 runs and F7
// builds the chosen project straight away.
//
// Each one runs in the terminal itself (sy steps aside), so what it prints
// shows. Then it waits for a key and sy comes back.

// mgAction is one line of the list.
type mgAction struct {
	label  string   // what the list says
	key    string   // its own key, if it has one
	args   []string // the manager's subcommand and its words
	dir    string   // the folder it runs in
	rescan bool     // look at the folders again after
	next   int      // not 0: opens this step instead of running
	after  int      // not 0: opens this step when it is done
	head   bool     // a heading: never chosen
}

// the steps of the list
const (
	mgStepList = iota
	mgStepLang // New project: which language
	mgStepName // New project: its name
	mgStepTidy // Tidy: do it for real?
)

type mgMenu struct {
	step   int
	items  []mgAction
	sel    int
	title  string // what it is for
	about  string // a line under the title
	lang   string // New project: the language chosen
	name   string // New project: the name typed
	catDir string // New project: where it goes
}

// mgDoneMsg comes when an action has finished and sy is back.
type mgDoneMsg struct {
	a   mgAction
	err error
}

// mgManagerDir is the manager's folder: names.tsv, manager.conf, bin/manager.
// Under SY_AN (tests) it is $SY_AN/ca/sy/manager; else $AN_MANAGER, else
// ~/an/ca/sy/manager.
func mgManagerDir() string {
	if os.Getenv("SY_AN") == "" {
		if d := os.Getenv("AN_MANAGER"); d != "" {
			return d
		}
	}
	return filepath.Join(mgRoot(), "ca", "sy", "manager")
}

func mgManagerBin() string { return filepath.Join(mgManagerDir(), "bin", "manager") }

// mgProjectDir is the project p is in: the nearest folder (p or one above
// it) with an executable make.sh. "" when there is none.
func mgProjectDir(p string) string {
	if info, err := os.Stat(p); err == nil && !info.IsDir() {
		p = filepath.Dir(p)
	}
	for d := p; ; d = filepath.Dir(d) {
		if info, err := os.Stat(filepath.Join(d, "make.sh")); err == nil && !info.IsDir() && info.Mode()&0o111 != 0 {
			return d
		}
		if filepath.Dir(d) == d {
			return ""
		}
	}
}

// mgCatDir is the folder of a category, or root for the made-up ones
// (Not named yet, Leftovers).
func mgCatDir(root, key string) string {
	d := root
	switch {
	case strings.HasPrefix(key, "~/"):
		home, _ := os.UserHomeDir()
		d = filepath.Join(home, key[2:])
	case key != "" && key != "?" && key != "!":
		d = filepath.Join(root, key)
	}
	if info, err := os.Stat(d); err != nil || !info.IsDir() {
		return root
	}
	return d
}

// mgActions is the full list for the chosen row and category.
func (m *Model) mgActions() (items []mgAction, title, about string) {
	s := m.mgr
	root := mgRoot()
	title, about = "Actions", "for all of "+shortPath(root)
	if r := s.picked(); r != nil {
		p := mgPath(root, r.it)
		title, about = r.it.Name, shortPath(p)
		items = append(items, mgAction{label: "For " + r.it.Name, head: true})
		if pd := mgProjectDir(p); pd != "" {
			items = append(items,
				mgAction{label: "Run", key: "F6", args: []string{"run"}, dir: pd},
				mgAction{label: "Build", key: "F7", args: []string{"build"}, dir: pd},
				mgAction{label: "Clean (remove what the build made)", args: []string{"clean"}, dir: pd})
			if fileExists(filepath.Join(pd, "reload.sh")) {
				items = append(items, mgAction{label: "Watch (build and run on every save)", args: []string{"watch"}, dir: pd})
			}
		} else if r.it.Dir && r.it.Kind != "" && r.it.Kind != "copies" && r.it.Kind != "leftover" {
			items = append(items, mgAction{label: "Adopt (give it a make.sh)", args: []string{"adopt", p}, dir: p, rescan: true})
		}
		dir := p
		if !r.it.Dir {
			dir = filepath.Dir(p)
		}
		items = append(items, mgAction{label: "Open in its editor", args: []string{"open", p}, dir: dir})
	}
	catDir, catName := root, ""
	if s.data != nil && s.query == "" && s.cat < len(s.data.Cats) {
		catDir, catName = mgCatDir(root, s.data.Cats[s.cat].Key), s.data.Cats[s.cat].Name
	}
	newLabel := "New project…"
	if catName != "" {
		newLabel = "New project in " + catName + "…"
	}
	items = append(items,
		mgAction{label: "For all of " + shortPath(root), head: true},
		mgAction{label: newLabel, dir: catDir, next: mgStepLang},
		mgAction{label: "Find folders without make.sh", args: []string{"scan"}, dir: root},
		mgAction{label: "Adopt them all (give each a make.sh)", args: []string{"adopt"}, dir: root, rescan: true},
		mgAction{label: "Check all builds", args: []string{"check"}, dir: root},
		mgAction{label: "Tidy (shows the list first)", args: []string{"tidy", "-n"}, dir: root, after: mgStepTidy},
		mgAction{label: "Find duplicates (only lists them)", args: []string{"dupes", "-n"}, dir: root},
		mgAction{label: "Set up / repair tools", args: []string{"setup"}, dir: root},
		mgAction{label: "Status", args: []string{"status"}, dir: root},
		mgAction{label: "Libraries", args: []string{"libs"}, dir: root},
		mgAction{label: "Refresh pictures", args: []string{"index"}, dir: root},
		mgAction{label: "Card window", args: []string{"gui"}, dir: root},
	)
	return items, title, about
}

// mgOpenMenu opens the list of actions, on its first line that can be chosen.
func (m *Model) mgOpenMenu() {
	items, title, about := m.mgActions()
	mn := &mgMenu{items: items, title: title, about: about}
	mn.move(0)
	m.mgr.menu = mn
}

// move steps the choice by n, over the headings.
func (mn *mgMenu) move(n int) {
	if len(mn.items) == 0 {
		return
	}
	step := 1
	if n < 0 {
		step = -1
	}
	i := max(0, min(mn.sel+n, len(mn.items)-1))
	for i >= 0 && i < len(mn.items) && mn.items[i].head {
		i += step
	}
	if i < 0 || i >= len(mn.items) { // only headings that way: look the other way
		i = max(0, min(mn.sel+n, len(mn.items)-1))
		for i >= 0 && i < len(mn.items) && mn.items[i].head {
			i -= step
		}
	}
	if i >= 0 && i < len(mn.items) {
		mn.sel = i
	}
}

// jump goes to the next line that starts with the letter t.
func (mn *mgMenu) jump(t string) {
	t = strings.ToLower(t)
	for k := 1; k <= len(mn.items); k++ {
		i := (mn.sel + k) % len(mn.items)
		if !mn.items[i].head && strings.HasPrefix(strings.ToLower(mn.items[i].label), t) {
			mn.sel = i
			return
		}
	}
}

// mgLangs lists the languages `manager langs` knows ("  c   about" lines),
// or the template folders if that does not answer. A var, so tests can
// change it.
var mgLangs = func() []string {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var out []string
	if b, err := exec.CommandContext(ctx, mgManagerBin(), "langs").Output(); err == nil {
		for _, l := range strings.Split(string(b), "\n") {
			if f := strings.Fields(l); len(f) > 0 {
				out = append(out, f[0])
			}
		}
	}
	if len(out) == 0 {
		es, _ := os.ReadDir(filepath.Join(mgManagerDir(), "templates"))
		for _, e := range es {
			if e.IsDir() {
				out = append(out, e.Name())
			}
		}
	}
	return out
}

// mgMenuKey is a key while the list is open.
func (m *Model) mgMenuKey(k tea.Key) tea.Cmd {
	mn := m.mgr.menu
	name := keyName(k)
	switch name {
	case "esc", "alt+a":
		switch mn.step {
		case mgStepLang:
			m.mgOpenMenu()
		case mgStepName:
			mn.step, mn.name = mgStepLang, ""
		default:
			m.mgr.menu = nil
		}
		return nil
	case "up":
		mn.move(-1)
	case "down":
		mn.move(1)
	case "home":
		mn.sel = len(mn.items)
		mn.move(-len(mn.items))
	case "end":
		mn.sel = -1
		mn.move(len(mn.items))
	case "pgup":
		mn.move(-8)
	case "pgdown":
		mn.move(8)
	case "enter":
		return m.mgMenuChoose()
	case "backspace":
		if r := []rune(mn.name); mn.step == mgStepName && len(r) > 0 {
			mn.name = string(r[:len(r)-1])
		}
	default:
		t := k.Text
		if t == "" || k.Mod.Contains(tea.ModCtrl) || k.Mod.Contains(tea.ModAlt) {
			return nil
		}
		if mn.step == mgStepName {
			if !strings.ContainsAny(t, "/\\") && !(mn.name == "" && t == ".") {
				mn.name += t
			}
		} else {
			mn.jump(t)
		}
	}
	return nil
}

// mgNewName makes a typed name fit for a folder: spaces become -.
func mgNewName(s string) string { return strings.Join(strings.Fields(s), "-") }

// mgMenuChoose is Enter in the list.
func (m *Model) mgMenuChoose() tea.Cmd {
	s := m.mgr
	mn := s.menu
	switch mn.step {
	case mgStepName:
		name := mgNewName(mn.name)
		if name == "" {
			m.sayErr("Type a name first.")
			return nil
		}
		if fileExists(filepath.Join(mn.catDir, name)) {
			m.sayErr("There is already a " + name + " in " + shortPath(mn.catDir) + ".")
			return nil
		}
		s.menu = nil
		return m.mgRun(mgAction{label: "New " + mn.lang + " project", args: []string{"new", mn.lang, name, mn.catDir}, dir: mn.catDir, rescan: true})
	}
	if mn.sel < 0 || mn.sel >= len(mn.items) || mn.items[mn.sel].head {
		return nil
	}
	a := mn.items[mn.sel]
	switch {
	case mn.step == mgStepLang:
		mn.lang, mn.step, mn.name = a.label, mgStepName, ""
		return nil
	case a.next == mgStepLang:
		langs := mgLangs()
		if len(langs) == 0 {
			m.sayErr("No languages: " + shortPath(mgManagerBin()) + " langs gave none.")
			return nil
		}
		var items []mgAction
		for _, l := range langs {
			items = append(items, mgAction{label: l})
		}
		s.menu = &mgMenu{step: mgStepLang, items: items, title: "New project", about: "in " + shortPath(a.dir), catDir: a.dir}
		return nil
	case a.args == nil: // No, on the Tidy question
		s.menu = nil
		return nil
	}
	s.menu = nil
	return m.mgRun(a)
}

// mgRowAction is F6 / F7: run or build the chosen row's project at once.
func (m *Model) mgRowAction(sub string) tea.Cmd {
	r := m.mgr.picked()
	if r == nil {
		m.say("Right goes into the category first; then choose a project.")
		return nil
	}
	pd := mgProjectDir(mgPath(mgRoot(), r.it))
	if pd == "" {
		m.sayErr(r.it.Name + " has no make.sh. Alt+A → Adopt gives it one.")
		return nil
	}
	label := map[string]string{"run": "Run", "build": "Build"}[sub]
	return m.mgRun(mgAction{label: label, args: []string{sub}, dir: pd})
}

// mgCommand is what runs an action: bash prints a title, runs the manager
// in a.dir, says how it went and waits for a key. Ctrl+C stops the manager
// (watch runs until then) but not the wait.
func mgCommand(bin string, a mgAction) *exec.Cmd {
	const script = `trap : INT
printf '\n\033[1m▶ %s\033[0m\n\n' "$1"; shift
"$@"; s=$?
echo
case $s in
0) printf '\033[32mDone.\033[0m' ;;
130) printf 'Stopped.' ;;
*) printf '\033[31mIt failed (error %s).\033[0m' "$s" ;;
esac
printf ' Press any key to go back.'
read -rsn1 _ 2>/dev/null || read -r _
echo
exit $s`
	title := "manager " + strings.Join(a.args, " ") + "   in " + shortPath(a.dir)
	c := exec.Command("bash", append([]string{"-c", script, "sy", title, bin}, a.args...)...)
	c.Dir = a.dir
	c.Env = os.Environ()
	if os.Getenv("AN_MANAGER") == "" {
		c.Env = append(c.Env, "AN_MANAGER="+mgManagerDir())
	}
	return c
}

// mgRun runs an action in the terminal and comes back with mgDoneMsg.
func (m *Model) mgRun(a mgAction) tea.Cmd {
	bin := mgManagerBin()
	if info, err := os.Stat(bin); err != nil || info.IsDir() {
		m.sayErr("The manager program is not there: " + shortPath(bin))
		return nil
	}
	if info, err := os.Stat(a.dir); err != nil || !info.IsDir() {
		m.sayErr("It is not there any more: " + shortPath(a.dir) + ". F5 looks again.")
		return nil
	}
	return mgExec(bin, a)
}

// mgExec hands the action to the terminal. A var, so tests see what would
// run without running it.
var mgExec = func(bin string, a mgAction) tea.Cmd {
	if !have("bash") {
		return func() tea.Msg { return mgDoneMsg{a: a, err: errors.New("no bash here to run it")} }
	}
	return tea.ExecProcess(mgCommand(bin, a), func(err error) tea.Msg { return mgDoneMsg{a: a, err: err} })
}

// mgDone is back from an action: say how it went, look again if it changed
// folders, and ask the Tidy question after the list.
func (m *Model) mgDone(msg mgDoneMsg) tea.Cmd {
	a := msg.a
	what := "manager " + strings.Join(a.args, " ")
	if len(a.args) > 0 && (a.args[0] == "open" || a.args[0] == "adopt" || a.args[0] == "new") && len(a.args) > 1 {
		what = "manager " + a.args[0]
	}
	if msg.err != nil {
		m.sayErr(what + " did not finish well: " + msg.err.Error())
	} else {
		m.say(what + ": done.")
	}
	if m.mgr == nil {
		return nil
	}
	if a.after == mgStepTidy && msg.err == nil {
		m.mgr.menu = &mgMenu{step: mgStepTidy, title: "Tidy", about: "That was only the list. Nothing is changed yet.", items: []mgAction{
			{label: "Yes: tidy for real", args: []string{"tidy"}, dir: a.dir, rescan: true},
			{label: "No: leave it all"},
		}}
		return nil
	}
	if a.rescan {
		return m.mgScanCmd()
	}
	return nil
}

// ---- drawing ----------------------------------------------------------------------------

// mgMenuLines draws the open list in the middle of the text area.
func (m *Model) mgMenuLines(h int) []string {
	mn := m.mgr.menu
	bg := cBarBG
	w := min(max(m.width-4, 20), 58)
	in := w - 4
	var out []string
	line := func(segs ...seg) {
		out = append(out, mgPad(mgRender(append([]seg{{text: "  ", bg: bg}}, segs...)...), w, bg))
	}
	out = append(out, mgPad("", w, bg))
	line(seg{text: modes[managerMode].Icon() + "  " + mn.title, fg: cEmber, bg: bg, bold: true, w: in})
	line(seg{text: mn.about, fg: cDim, bg: bg, w: in, italic: true})
	out = append(out, mgPad("", w, bg))
	hint := "↑↓ choose · Enter do it · Esc close"
	switch mn.step {
	case mgStepLang:
		line(seg{text: "Which language?", fg: cStatusText, bg: bg, w: in})
		hint = "↑↓ choose · Enter next · Esc back"
	case mgStepName:
		line(seg{text: mn.lang + " project. Its name:", fg: cStatusText, bg: bg, w: in})
		line(seg{text: "› " + mn.name + "▏", fg: cEmber, bg: bg, bold: true, w: in})
		where := "…"
		if n := mgNewName(mn.name); n != "" {
			where = shortPath(filepath.Join(mn.catDir, n))
		}
		out = append(out, mgPad("", w, bg))
		line(seg{text: "It goes in " + where, fg: cDim, bg: bg, w: in})
		hint = "type the name · Enter make it · Esc back"
	case mgStepTidy:
		hint = "↑↓ choose · Enter · Esc no"
	}
	if mn.step != mgStepName {
		room := max(h-len(out)-4, 3)
		top := 0
		if len(mn.items) > room {
			top = max(0, min(mn.sel-room/2, len(mn.items)-room))
		}
		for i := top; i < min(len(mn.items), top+room); i++ {
			a := mn.items[i]
			if a.head {
				if i > top {
					out = append(out, mgPad("", w, bg))
				}
				line(seg{text: a.label, fg: cDim, bg: bg, w: in, italic: true})
				continue
			}
			rbg, fg, mark := bg, cText, "  "
			if i == mn.sel {
				rbg, fg, mark = cSelection, cSelectedText, "▸ "
			}
			line(seg{text: mark, fg: cEmber, bg: rbg, bold: true},
				seg{text: a.label, fg: fg, bg: rbg, w: in - 2 - 4, bold: i == mn.sel},
				seg{text: a.key, fg: cAccent, bg: rbg, w: 4, right: true})
		}
	}
	out = append(out, mgPad("", w, bg))
	line(seg{text: hint, fg: cDim, bg: bg, w: in, italic: true})
	out = append(out, mgPad("", w, bg))
	block := lipgloss.Place(m.width, h, lipgloss.Center, lipgloss.Center, strings.Join(out, "\n"),
		lipgloss.WithWhitespaceStyle(lipgloss.NewStyle().Background(hex(cBG))))
	return strings.Split(block, "\n")
}
