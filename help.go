package main

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

type helpLine struct{ keys, what string }
type helpGroup struct {
	title string
	added bool // added by Claude for you to try, not from your list
	lines []helpLine
}

var helpGroups = []helpGroup{
	{"Moving the cursor", false, []helpLine{
		{"Home / Alt + B", "start of the line"},
		{"End", "end of the line"},
		{"Left / Right", "one step: a word, part of a name (MAX _ PARTS), number, bracket, operator"},
		{"Up / Down", "up or down one line"},
		{"Ctrl + arrows", "one character (Up / Down: one line)"},
		{"Ctrl + Left + Up", "up a line and left a letter (hold both)"},
		{"Ctrl + Right + Up", "up a line and right a letter"},
		{"Ctrl + Left + Down", "down a line and left a letter"},
		{"Ctrl + Right + Down", "down a line and right a letter"},
		{"Alt + S / Alt + E", "start / end of the file (also Ctrl + Home / End)"},
	}},
	{"Moving text", false, []helpLine{
		{"Alt + Left / Alt + Right", "move the highlighted text, or else the word at the cursor"},
		{"Alt + Up / Alt + Down", "move the highlighted text up or down a line (whole lines, or nothing highlighted: the line)"},
	}},
	{"Key mode (status bar)", false, []helpLine{
		{"no key held", "text: typing"},
		{"Shift held", "highlighter: the arrows highlight"},
		{"Ctrl held", "position: the arrows move the cursor one character"},
		{"Alt held", "movement: the arrows move the highlighted text, or the word"},
	}},
	{"Quitting", false, []helpLine{
		{"Alt + Q", "quit to the terminal (everything is already saved)"},
	}},
	{"Highlighting (selecting)", false, []helpLine{
		{"Shift + Left / Right", "highlight one step (the same steps Left / Right move)"},
		{"Ctrl + Shift + arrows", "highlight one character (Up / Down: one line)"},
		{"Shift + Up / Down", "highlight one line at a time"},
	}},
	{"Copy and paste", false, []helpLine{
		{"Ctrl + C", "copy (nothing highlighted = the whole line)"},
		{"Ctrl + V / Ctrl + P", "paste"},
	}},
	{"Find, save and load", false, []helpLine{
		{"Ctrl + F", "find (Enter or Down = next, Up = back, Esc = close)"},
		{"(every change)", "saves by itself, a moment after you type"},
		{"Ctrl + S  or ß", "save now; a session is asked for a name (.cpp, .c, .py ... added)"},
		{"Alt + L", "load: every session, and this file again from disk"},
		{"ø  or Alt + O", "open a file: the file manager"},
	}},
	{"Sessions (drafts with no name)", false, []helpLine{
		{"sy", "carry on with today's session, or start a new one"},
		{"24 hours", "a session with no change for 24 hours is concluded:"},
		{"", "  sy starts a new one; the old one stays in Alt+L"},
	}},
	{"File manager (a box over the editor)", false, []helpLine{
		{"AltGr, AltGr", "tap AltGr twice: open it (or close it), with or without a file open"},
		{"Alt + O  or AltGr + O (ø)", "open it"},
		{"Alt + E  or AltGr + E", "exit it (Esc and Ctrl + B do too)"},
		{"sy FOLDER", "open a folder as a project"},
		{"AltGr + P  or Alt + P", "make the chosen folder the project"},
		{"type letters", "find a file anywhere in the project"},
		{"Enter / Right", "open a file, or open / close a folder"},
		{"Shift + Enter", "open every text file in this folder and its folders as tabs, code first"},
		{"sy -t FOLDER", "the same from the terminal (sy -t alone: this folder)"},
		{"Left", "close a folder (on the top row: go up a folder)"},
		{"Ctrl + N / F7", "new file / new folder here"},
		{"F2 / Delete", "rename / move to the Trash (asks first)"},
		{"Esc / click outside", "close it"},
	}},
	{"The central zone", false, []helpLine{
		{"drag a margin line", "the text sits in a centred column; drag either side's thin line"},
		{"", "  to make it wider or narrower (remembered)"},
	}},
	{"Centred lines", false, []helpLine{
		{"(prose)", "plain text and Markdown lines sit in the middle, like a page"},
		{"Alt + J", "centre the lines of this file, or put them back on the left"},
	}},
	{"Colours", false, []helpLine{
		{"ŧ  (Right Alt + T) or Alt + T", "colour themes: Emberistal, Emberline, Molten, Ash, Cinder, Daybreak"},
		{"", "  moving shows it · Enter keeps it · Esc puts it back"},
	}},
	{"Modes", false, []helpLine{
		{"Alt + M", "switch modes: Text, Design, Slides, Diary, Book, Draw (Design, Slides, Book, Draw are empty for now)"},
		{"sy design / slide / diary", "start in that mode (zsh: de, sli, syd or di)"},
		{"sy book / draw", "start in Book or Draw mode (zsh: bo, dr)"},
	}},
	{"Diary mode", false, []helpLine{
		{"(chapters)", "a chapter a day; its pages in ~/.local/share/symbolizer/diary"},
		{"Left / Right", "previous / next page (on into the chapter before or after)"},
		{"Up / Down", "previous / next chapter"},
		{"Alt + N or AltGr + N", "a new page in today's chapter, ready to write on"},
		{"Alt + A or AltGr + A", "all pages at once (same arrows; Enter shows the page)"},
		{"Enter / Esc", "write on the page in the editor / back to the pages"},
	}},
	{"Open files (tabs)", false, []helpLine{
		{"Alt + N", "new file in a new tab"},
		{"Alt + C", "close this file (it is already saved)"},
		{"Tab", "next file tab in the common direction"},
		{"Tab, then Left / Right", "hold Tab: switch file tabs that way"},
		{"Alt + Tab", "a real Tab: indent the line or the highlighted lines"},
		{"Shift + Alt + Tab", "un-indent (Shift + Tab does too)"},
		{"Ctrl + Tab, then Left / Right", "keep Ctrl held: set the common direction"},
		{"Alt + 1 … 9", "go to open file 1 … 9"},
		{"Ctrl + PgUp / PgDn", "previous / next open file"},
		{"Click a tab", "go to that file"},
	}},
	{"Moving text sideways", true, []helpLine{
		{"Alt + Shift + Left / Right", "move the highlighted text (or one letter) sideways"},
	}},
	{"Editing", true, []helpLine{
		{"Ctrl + Z / Ctrl + Y", "undo / redo"},
		{"Ctrl + X", "cut (nothing highlighted = the whole line)"},
		{"Ctrl + A", "highlight everything"},
		{"Ctrl + D", "duplicate the line or the highlighted text"},
		{"Ctrl + K", "delete the line"},
		{"Ctrl + /", "turn lines into comments, or back"},
		{"Alt + Tab / Shift + Alt + Tab", "indent / un-indent the line or highlighted lines"},
		{"Alt + Backspace", "delete the word before the cursor"},
	}},
	{"Finding and files", true, []helpLine{
		{"Ctrl + R", "find and replace (y = yes, n = skip, a = all)"},
		{"Ctrl + G", "go to a line number (42, or 42:7 for a column)"},
		{"F3 / Shift + F3", "next / previous match after Find is closed"},
		{"Alt + A", "save under a new name"},
		{"Ctrl + N", "new session in a new tab (same as Alt + N)"},
	}},
	{"Mouse", true, []helpLine{
		{"Click / drag", "place the cursor / highlight"},
		{"Double click", "highlight a word (triple click: the line)"},
		{"Wheel", "scroll"},
		{"Shift + drag", "the terminal's own text selection (kitty)"},
	}},
}

func (m *Model) helpText() []string {
	var out []string
	out = append(out, glowText("◆ Symbolizer", cBG)+stHelpText.Render("  ·  keys"))
	out = append(out, stHelpNew.Render("Groups marked  ✦ try me  were added by Claude. Keep the ones you like."))
	out = append(out, "")
	for _, g := range helpGroups {
		title := stHelpHead.Render(g.title)
		if g.added {
			title += stHelpNew.Render("  ✦ try me")
		}
		out = append(out, title)
		for _, l := range g.lines {
			k := stHelpKey.Render(l.keys)
			pad := strings.Repeat(" ", max(30-lipgloss.Width(k), 1))
			out = append(out, "  "+k+pad+stHelpText.Render(l.what))
		}
		out = append(out, "")
	}
	out = append(out, stHelpNew.Render("Esc, F1 or Q closes this page.  Up / Down scrolls."))
	return out
}

func (m *Model) drawHelp(sb *strings.Builder) {
	lines := m.helpText()
	room := max(m.height-2, 1)
	m.helpTop = max(0, min(m.helpTop, len(lines)-room))
	end := min(len(lines), m.helpTop+room)
	box := stHelpBox.Render(strings.Join(lines[m.helpTop:end], "\n"))
	sb.WriteString(lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, box,
		lipgloss.WithWhitespaceStyle(lipgloss.NewStyle().Background(hex(cBG)))))
}

func (m *Model) helpKey(name string) tea.Cmd {
	switch name {
	case "esc", "f1", "q", "alt+h", "enter":
		m.help = false
	case "up", "alt+up":
		m.helpTop = max(0, m.helpTop-1)
	case "down", "alt+down":
		m.helpTop++
	case "pgup":
		m.helpTop = max(0, m.helpTop-10)
	case "pgdown", "space":
		m.helpTop += 10
	}
	return nil
}
