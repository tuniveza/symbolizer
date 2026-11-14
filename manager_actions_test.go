package main

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

// a pretend ~/an with one project (make.sh + reload.sh), a folder with no
// make.sh, and a fake manager program; the Manager open on the project
func mgActionsTest(t *testing.T) (*Model, string, *[]mgAction) {
	t.Helper()
	root := t.TempDir()
	t.Setenv("SY_AN", root)
	t.Setenv("AN_MANAGER", "/somewhere/else") // SY_AN wins over it
	put := func(p, text string, mode os.FileMode) {
		p = filepath.Join(root, p)
		os.MkdirAll(filepath.Dir(p), 0o755)
		if err := os.WriteFile(p, []byte(text), mode); err != nil {
			t.Fatal(err)
		}
	}
	put("ca/sy/manager/names.tsv", "co\tCode\tby language\nca\tApps\nco/c/game\tMy Game\ta game in C\n", 0o644)
	put("ca/sy/manager/bin/manager", "#!/bin/sh\n", 0o755)
	put("co/c/game/make.sh", "#!/bin/sh\n", 0o755)
	put("co/c/game/reload.sh", "#!/bin/sh\n", 0o755)
	put("co/c/game/main.c", "int main(){}\n", 0o644)
	put("co/go/tool/main.go", "package main\n", 0o644)
	put("co/go/tool/go.mod", "module tool\n", 0o644)

	m := newTest(t, "")
	m.mode = managerMode
	m.width, m.height = 140, 40
	m.mgr = &mgState{data: mgScan(root, nil, filepath.Join(root, "ca/sy/manager/names.tsv"))}
	for i, c := range m.mgr.data.Cats {
		if c.Key == "co" {
			m.mgr.cat = i
		}
	}
	mgChoose(t, m, "co/c/game")

	var ran []mgAction
	old := mgExec
	mgExec = func(bin string, a mgAction) tea.Cmd {
		if bin != filepath.Join(root, "ca/sy/manager/bin/manager") {
			t.Errorf("bin = %s", bin)
		}
		ran = append(ran, a)
		return func() tea.Msg { return nil }
	}
	t.Cleanup(func() { mgExec = old })
	return m, root, &ran
}

// mgChoose puts the Manager on the row with this path.
func mgChoose(t *testing.T, m *Model, rel string) {
	t.Helper()
	for i, r := range m.mgr.rows() {
		if r.it.Rel == rel {
			m.mgr.sel = i
			return
		}
	}
	t.Fatalf("no row %s", rel)
}

func mgLabels(mn *mgMenu) []string {
	var out []string
	for _, a := range mn.items {
		out = append(out, a.label)
	}
	return out
}

func TestManagerMenuOpensAndCloses(t *testing.T) {
	m, _, ran := mgActionsTest(t)
	press(m, "alt+a")
	mn := m.mgr.menu
	if mn == nil {
		t.Fatal("Alt+A did not open the actions")
	}
	labels := mgLabels(mn)
	for _, l := range []string{"Run", "Build", "Clean (remove what the build made)", "Watch (build and run on every save)", "Open in its editor",
		"New project in Code…", "Find folders without make.sh", "Adopt them all (give each a make.sh)", "Check all builds",
		"Tidy (shows the list first)", "Find duplicates (only lists them)", "Set up / repair tools", "Status", "Libraries", "Refresh pictures", "Card window"} {
		if !slices.Contains(labels, l) {
			t.Errorf("no %q in %q", l, labels)
		}
	}
	if mn.items[mn.sel].label != "Run" {
		t.Fatalf("starts on %q, not Run (a heading is never chosen)", mn.items[mn.sel].label)
	}
	// letters jump in the list, they do not search
	typeStr(m, "s")
	if m.mgr.query != "" || mn.items[mn.sel].label != "Set up / repair tools" {
		t.Fatalf("s: query %q, on %q", m.mgr.query, mn.items[mn.sel].label)
	}
	press(m, "home")
	if mn.items[mn.sel].label != "Run" {
		t.Fatalf("Home: on %q", mn.items[mn.sel].label)
	}
	press(m, "up") // nothing above Run but a heading: it stays
	if mn.items[mn.sel].label != "Run" {
		t.Fatalf("Up from the top: on %q", mn.items[mn.sel].label)
	}
	press(m, "end")
	if mn.items[mn.sel].label != "Card window" {
		t.Fatalf("End: on %q", mn.items[mn.sel].label)
	}
	for _, size := range [][2]int{{140, 40}, {80, 24}, {40, 12}, {30, 8}} { // it draws at any size
		m.width, m.height = size[0], size[1]
		_ = m.View()
	}
	press(m, "esc")
	if m.mgr.menu != nil || len(*ran) != 0 {
		t.Fatal("Esc did not close it, or something ran")
	}
}

func TestManagerMenuRunsAction(t *testing.T) {
	m, root, ran := mgActionsTest(t)
	game := filepath.Join(root, "co/c/game")
	press(m, "alt+a")
	press(m, "down")
	press(m, "enter")
	if m.mgr.menu != nil || len(*ran) != 1 {
		t.Fatalf("Enter: menu %v, ran %d", m.mgr.menu, len(*ran))
	}
	if a := (*ran)[0]; !slices.Equal(a.args, []string{"build"}) || a.dir != game || a.rescan {
		t.Fatalf("Build = %+v", a)
	}

	// Open goes to the manager with the path; global ones run in ~/an
	press(m, "alt+a")
	typeStr(m, "o")
	press(m, "enter")
	if a := (*ran)[1]; !slices.Equal(a.args, []string{"open", game}) || a.dir != game {
		t.Fatalf("Open = %+v", a)
	}
	press(m, "alt+a")
	typeStr(m, "f")
	press(m, "enter")
	if a := (*ran)[2]; !slices.Equal(a.args, []string{"scan"}) || a.dir != root {
		t.Fatalf("Find folders = %+v", a)
	}
	press(m, "alt+a")
	typeStr(m, "ff") // Find folders, then Find duplicates
	press(m, "enter")
	if a := (*ran)[3]; !slices.Equal(a.args, []string{"dupes", "-n"}) {
		t.Fatalf("Find duplicates = %+v", a)
	}

	// F6 runs and F7 builds the chosen project at once
	m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyF6}))
	m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyF7}))
	if len(*ran) != 6 || !slices.Equal((*ran)[4].args, []string{"run"}) || !slices.Equal((*ran)[5].args, []string{"build"}) || (*ran)[4].dir != game {
		t.Fatalf("F6 F7 = %+v", (*ran)[4:])
	}

	// a folder with no make.sh: F6 says why, the list offers Adopt and no Run
	mgChoose(t, m, "co/go/tool")
	m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyF6}))
	if len(*ran) != 6 || !m.msgErr || !strings.Contains(m.msg, "no make.sh") {
		t.Fatalf("F6 on no project: ran %d, msg %q", len(*ran), m.msg)
	}
	press(m, "alt+a")
	labels := mgLabels(m.mgr.menu)
	if slices.Contains(labels, "Run") || !slices.Contains(labels, "Adopt (give it a make.sh)") {
		t.Fatalf("no project: %q", labels)
	}
	press(m, "enter")
	tool := filepath.Join(root, "co/go/tool")
	if a := (*ran)[6]; !slices.Equal(a.args, []string{"adopt", tool}) || a.dir != tool || !a.rescan {
		t.Fatalf("Adopt = %+v", a)
	}
	// coming back from adopt looks at the folders again
	m.mgr.scanning = false
	_, cmd := m.Update(mgDoneMsg{a: (*ran)[6]})
	if cmd == nil || !m.mgr.scanning {
		t.Fatal("no new look after adopt")
	}
}

func TestManagerMenuNewProject(t *testing.T) {
	m, root, ran := mgActionsTest(t)
	old := mgLangs
	mgLangs = func() []string { return []string{"c", "py", "godot"} }
	t.Cleanup(func() { mgLangs = old })
	press(m, "alt+a")
	typeStr(m, "n")
	press(m, "enter")
	mn := m.mgr.menu
	if mn == nil || mn.step != mgStepLang || !slices.Equal(mgLabels(mn), []string{"c", "py", "godot"}) {
		t.Fatalf("languages = %+v", mn)
	}
	press(m, "down")
	press(m, "enter")
	if mn.step != mgStepName || mn.lang != "py" {
		t.Fatalf("after the language: step %d lang %q", mn.step, mn.lang)
	}
	press(m, "enter") // no name yet: nothing runs
	if len(*ran) != 0 || m.mgr.menu == nil {
		t.Fatal("ran with no name")
	}
	typeStr(m, "my/ game")
	press(m, "backspace")
	typeStr(m, "e")
	if mn.name != "my game" {
		t.Fatalf("name = %q", mn.name)
	}
	_ = m.View()
	press(m, "esc") // back to the languages, and on again
	if mn.step != mgStepLang {
		t.Fatalf("Esc from the name: step %d", mn.step)
	}
	press(m, "enter")
	typeStr(m, "my game")
	press(m, "enter")
	co := filepath.Join(root, "co")
	if len(*ran) != 1 {
		t.Fatalf("ran %d", len(*ran))
	}
	if a := (*ran)[0]; !slices.Equal(a.args, []string{"new", "py", "my-game", co}) || a.dir != co || !a.rescan {
		t.Fatalf("New = %+v", a)
	}
	// a name that is taken is refused
	press(m, "alt+a")
	typeStr(m, "n")
	press(m, "enter")
	press(m, "enter")
	typeStr(m, "c")
	press(m, "enter")
	if len(*ran) != 1 || !m.msgErr || m.mgr.menu == nil {
		t.Fatalf("taken name: ran %d msg %q", len(*ran), m.msg)
	}
}

func TestManagerMenuTidyAsksAfterList(t *testing.T) {
	m, root, ran := mgActionsTest(t)
	press(m, "alt+a")
	typeStr(m, "t")
	press(m, "enter")
	if a := (*ran)[0]; !slices.Equal(a.args, []string{"tidy", "-n"}) || a.dir != root || a.after != mgStepTidy {
		t.Fatalf("Tidy = %+v", a)
	}
	m.Update(mgDoneMsg{a: (*ran)[0]})
	mn := m.mgr.menu
	if mn == nil || mn.step != mgStepTidy {
		t.Fatal("no question after the tidy list")
	}
	press(m, "down") // No
	press(m, "enter")
	if m.mgr.menu != nil || len(*ran) != 1 {
		t.Fatal("No still ran something")
	}
	m.Update(mgDoneMsg{a: (*ran)[0]})
	press(m, "enter") // Yes
	if len(*ran) != 2 || !slices.Equal((*ran)[1].args, []string{"tidy"}) || !(*ran)[1].rescan {
		t.Fatalf("Yes = %+v", *ran)
	}
	// a failed list does not ask
	m.mgr.menu = nil
	m.Update(mgDoneMsg{a: (*ran)[0], err: os.ErrInvalid})
	if m.mgr.menu != nil || !m.msgErr {
		t.Fatal("asked after a failed list")
	}
}

// the command for an action: bash around the manager, in the action's folder
func TestManagerActionCommand(t *testing.T) {
	root := t.TempDir()
	t.Setenv("SY_AN", root)
	t.Setenv("AN_MANAGER", "")
	if got := mgManagerDir(); got != filepath.Join(root, "ca/sy/manager") {
		t.Fatalf("manager dir = %s", got)
	}
	a := mgAction{args: []string{"new", "c", "game", root}, dir: root}
	c := mgCommand("/x/bin/manager", a)
	if c.Dir != root || filepath.Base(c.Path) != "bash" && c.Args[0] != "bash" {
		t.Fatalf("dir %s path %s", c.Dir, c.Path)
	}
	if tail := c.Args[len(c.Args)-5:]; !slices.Equal(tail, []string{"/x/bin/manager", "new", "c", "game", root}) {
		t.Fatalf("argv tail = %q", tail)
	}
	if !strings.Contains(c.Args[2], "Press any key to go back") {
		t.Fatal("it does not wait for a key")
	}
	if !slices.Contains(c.Env, "AN_MANAGER="+filepath.Join(root, "ca/sy/manager")) {
		t.Fatal("AN_MANAGER is not passed on")
	}
	// no manager program: it says so and runs nothing
	m := newTest(t, "")
	m.mode = managerMode
	m.mgr = &mgState{}
	if cmd := m.mgRun(mgAction{args: []string{"status"}, dir: root}); cmd != nil || !strings.Contains(m.msg, "not there") {
		t.Fatalf("no program: msg %q", m.msg)
	}
	// outside tests, AN_MANAGER is used
	t.Setenv("SY_AN", "")
	t.Setenv("AN_MANAGER", "/opt/manager")
	if mgManagerBin() != "/opt/manager/bin/manager" {
		t.Fatalf("bin = %s", mgManagerBin())
	}
}
