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
		{"Alt + Left / Right", "start / end of the line"},
		{"Left / Right", "one step: a word, part of a name (MAX _ PARTS), part of a number (1 234 567, 3 . 14), bracket, operator"},
		{"Up / Down", "up or down one line"},
		{"Alt + Up / Down", "up / down a block: a paragraph, heading, list, element, function or if / for (follows the language). On a line that opens { ( [ or a tag, Down goes to its closing line; Up goes back"},
		{"Ctrl + arrows", "one character, or a whole number (Up / Down: one line)"},
		{"Ctrl + Left + Up", "up a line and left a letter (hold both)"},
		{"Ctrl + Right + Up", "up a line and right a letter"},
		{"Ctrl + Left + Down", "down a line and left a letter"},
		{"Ctrl + Right + Down", "down a line and right a letter"},
		{"Alt + S / Alt + E", "start / end of the file (also Ctrl + Home / End)"},
	}},
	{"Moving text", false, []helpLine{
		{"Alt + Shift + Left / Right", "move the highlighted text, or else the word at the cursor"},
		{"Alt + Shift + Up / Down", "move the highlighted text up or down a line (whole lines, or nothing highlighted: the line)"},
	}},
	{"Key mode (status bar)", false, []helpLine{
		{"no key held", "text: typing"},
		{"Shift held", "highlighter: the arrows highlight"},
		{"Ctrl held", "position: the arrows move the cursor one character (a number: all of it)"},
		{"Alt held", "line: Left / Right go to the start / end of the line, Up / Down a block"},
		{"Alt + Shift held", "movement: the arrows move the highlighted text, or the word"},
	}},
	{"Quitting", false, []helpLine{
		{"Alt + Q", "quit to the terminal (everything is already saved)"},
	}},
	{"Highlighting (selecting)", false, []helpLine{
		{"Shift + Left / Right", "highlight one step (the same steps Left / Right move)"},
		{"Ctrl + Shift + arrows", "highlight one character, or a whole number (Up / Down: one line)"},
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
		{"sy", "open again what was open when sy last closed (files, project, places)"},
		{"sy --new", "carry on with today's session, or start a new one"},
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
		{"µ  (Right Alt + M) or Alt + T", "colour themes: Emberistal, Emberline, Molten, Ash, Cinder, Daybreak"},
		{"ŧ  (Right Alt + T)", "sy term: show or hide a real kitty terminal under the editor (it keeps running while hidden)"},
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
		{"AltGr + C", "close this file (it is already saved)"},
		{"Alt + Tab", "tab mode: the tabs open out into rows"},
		{"  Left / Right or A / D", "  the file before / after"},
		{"  W / S", "  the file in the row above / below"},
		{"  Up / Down", "  keep this file and move the cursor"},
		{"  Enter or Alt / Esc", "  keep this file (Alt: tap it on its own) / go back to where you were"},
		{"Tab", "a real Tab: indent the line or the highlighted lines (web files: Tab expands Emmet first)"},
		{"Shift + Tab", "un-indent (Shift + Alt + Tab does too)"},
		{"Alt + 1 … 9", "go to open file 1 … 9"},
		{"Ctrl + PgUp / PgDn", "previous / next open file"},
		{"Click a tab", "go to that file"},
	}},
	{"Web files: Emmet and auto-correct (HTML, CSS, JSX, XML)", false, []helpLine{
		{"Tab", "expand the abbreviation before the cursor (ul>li*3, m10, d:f); Tab again: the next empty place"},
		{"! / html / html:5 + Tab", "on an empty file: a whole HTML5 page (works in a new tab or .txt too)"},
		{"Ctrl + E", "expand any word; with text highlighted: wrap it (ul>li* puts each line in an li)"},
		{"Alt + W", "a list of every web action"},
		{"Alt + U / Alt + D", "highlight the tag around / inside"},
		{"Alt + G", "go to the matching tag"},
		{"Alt + R / Alt + K", "remove the tag (keep what is inside) / split or join it"},
		{"Alt + Y", "update the tag: a new name, .class, #id or [attr=value]"},
		{"Alt + . / Alt + ,", "next / previous empty place to fill in"},
		{"Alt + ] / Alt + [", "highlight the next / previous item (tag, attribute, value)"},
		{"Alt + X", "work out the sum before the cursor (10*3+2 → 32)"},
		{"Alt + = / Alt + -", "the number at the cursor up / down 1 (with Shift: 10)"},
		{"Alt + I", "set an image's width and height from the file"},
		{"Alt + P", "list the files that fit the path being typed"},
		{"Ctrl + /", "comment out with <!-- --> or /* */"},
		{"Typing >", "closes the tag: <div> gets </div>; typing </ finishes the closing tag"},
		{"Editing a tag name", "the partner tag is renamed too"},
		{"Typing ; or : (CSS)", "fixes slips: colour → color, dispaly: flx → display: flex, 10xp → 10px"},
		{"Closing a path's quote", "a path to no file is fixed to the file it meant (Ctrl + Z puts it back)"},
		{"Enter between <p></p>", "the inside gets its own line (the same for { })"},
	}},
	{"Moving text sideways", true, []helpLine{
		{"Alt + Shift + Left / Right", "move the highlighted text (or one letter) sideways"},
	}},
	{"Editing", true, []helpLine{
		{"Ctrl + Z / Ctrl + Y", "undo / redo"},
		{"Ctrl + X", "cut (nothing highlighted = the whole line)"},
		{"Ctrl + A", "highlight everything"},
		{"Ctrl + D", "duplicate the line or the highlighted text"},
		{"Ctrl + K or Alt + D", "delete the line"},
		{"Alt + B", "delete the block the cursor is in: bracket to its partner bracket (or an element, a Python block, a paragraph)"},
		{"Ctrl + /", "turn lines into comments, or back"},
		{"Tab / Shift + Alt + Tab", "indent / un-indent the line or highlighted lines"},
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
