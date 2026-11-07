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
		{"Left / Right", "one step: a word, part of a name (MAX _ PARTS), one digit of a number (1 0, 3 . 1 4), bracket, operator"},
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
		{"Alt + Shift + Left / Right", "indent the line (or the highlighted lines) left / right"},
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
		{"Enter", "open a file, or open / close a folder"},
		{"Right", "open a file. On a folder: Right once opens it here, Right twice opens it as a project (its files as tabs)"},
		{"  Left straight after", "  undo it: its tabs close and you are back where you were"},
		{"  Left twice on its folder", "  close the project. One project is open at a time"},
		{"Shift + Right", "the same, but only text with something in it: no pictures, sound, video or empty files"},
		{"Shift + Enter", "open every text file in this folder and its folders as a project, code first"},
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
		{"Alt + Z", "word wrap on / off: long lines carry on in the rows under them (on to start with; remembered)"},
	}},
	{"Learn", false, []helpLine{
		{"AltGr + L  (ł)", "the recommended way to write this file's language, open at what the cursor is on (16px → px)"},
		{"  Up / Down · Left / Right", "  the line before / after · the section before / after"},
		{"  Tab · Esc", "  another language · close"},
	}},
	{"Colours", false, []helpLine{
		{"µ  (Right Alt + M) or Alt + T", "colour themes: Emberistal, Emberline, Molten, Ash, Cinder, Daybreak"},
		{"ŧ  (Right Alt + T)", "sy term: show or hide a real kitty terminal under the editor (it keeps running while hidden)"},
		{"Ctrl + Alt + arrows", "go to the pane that way: Up the editor, Down sy term (kitty does it: ~/.config/kitty/panes.conf)"},
		{"Alt + R, then an arrow", "resize the split once: Up / Down make the pane you are in taller / shorter (kitty does it; not in Text mode, where Alt + R is Emmet's remove tag)"},
		{"", "  moving shows it · Enter keeps it · Esc puts it back"},
	}},
	{"Modes", false, []helpLine{
		{"Alt + M", "switch modes: Text, Design, Slides, Diary, Book, Draw (Slides, Book, Draw are empty for now)"},
		{"sy design / slide / diary", "start in that mode (zsh: de, sli, syd or di)"},
		{"sy book / draw", "start in Book or Draw mode (zsh: bo, dr)"},
	}},
	{"Design mode", false, []helpLine{
		{"(the board)", "kept in ~/.local/share/symbolizer/design/ (board.json, a file per structure in parts/, the ink in marks.json)"},
		{"Double-click the board", "a new octagon structure (or what the toolbar's new chooses): type the header"},
		{"  Tab · Enter · Esc", "  header → text → footer · next part (a new line in the text) · done"},
		{"Drag · its ◢ corner", "move a structure · resize it (double-click one to write in it)"},
		{"Middle-drag · wheel", "move the board (left-drag on empty board too) · zoom in and out, toward the pointer"},
		{"⛓ link on the toolbar", "link mode: click a structure, then each one to attach (click again to detach)"},
		{"Arrows", "jump the view to the closest structure that way and choose it (held: on to the next one); nothing that way: a step"},
		{"Shift + arrows", "move the view freely (held: it glides on at once, no pause); the structure nearest the middle is chosen as you go"},
		{"Two arrows held (Up + Right …)", "move the view diagonally (with Alt: the chosen structure)"},
		{"Alt + arrows (Alt + Shift)", "move the chosen structure (further); near the closest one it snaps a small gap away from it and stops there"},
		{"Tab · Shift + Tab", "switch areas: write in the chosen structure's header, then its text, then its footer (back), and the view moves to it; on a cabinet: the next stack"},
		{"+ · − · 0", "zoom the view in · out · back to 100% (the keypad's + and − too)"},
		{"1–8", "highlighter pen colours (press the same one again to put it down)"},
		{"  ⌫ · over/under on the toolbar", "  rubber · pen under the text or over it; right-drag rubs out too"},
		{"Alt + N, then P · N · C · S", "place a new page · note · cabinet · shape in the middle of the screen"},
		{"Alt + L (Alt + Shift + L)", "change the chosen shape's layout: text · picture above · left · right · picture only · two columns (back)"},
		{"X X  (X twice, quickly)", "delete what is chosen: a structure or a link (✕ on it does the same)"},
		{"Ctrl+Z · Ctrl+Y (or Ctrl+Shift+Z)", "undo · redo any change: a move, words, a deletion, ink (also ↶ ↷ on the toolbar)"},
		{"Click a link", "choose the link (X X deletes it)"},
		{"✕ on a drawer or cabinet row", "delete that page, note, book, slide or drawing"},
		{"✕ on a picture", "take the picture out of its shape (the structure stays)"},
		{"  ◆ ≡ ✎ ≣ on the toolbar", "  what a double-click on the empty board places"},
		{"img on the toolbar", "a picture (any format) for the chosen shape, or a text file for the chosen page; type its path"},
		{"Drag files onto the board", "in kitty: each goes where you let go (into what is under it, or a new structure there)"},
		{"[¶] on the top edge", "the shape's layout (Alt + L does the same)"},
		{"▾ on the bottom edge", "pull the structure's drawer out (a small cabinet) or put it back"},
		{"Cabinet: click a stack", "pull out Pages, Books, Slides, Notes or Drawings; drag a thing out onto the board"},
		{"  Drop a page or note", "  on a cabinet, an open drawer or a ▾: it is filed in its stack"},
		{"  ⇣ rows in Pages", "  bring in the Diary's pages, or the files open in Text mode"},
		{"Wheel on a page", "scroll it (Shift + wheel)"},
		{"(height)", "no structure is drawn taller than the screen, at any zoom: its text scrolls, its picture shrinks to fit"},
		{"(text)", "all text on the board is centred, like prose in Text mode"},
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
		{"Alt + Tab", "tab mode: the tabs open out into rows"},
		{"  Left / Right or A / D", "  the file before / after"},
		{"  Up / Down or W / S", "  the file in the row above / below"},
		{"  Enter, Down twice or Alt", "  keep this file (Enter twice works too; Alt: tap it on its own)"},
		{"  Esc", "  go back to where you were"},
		{"Tab", "a real Tab: indent the line or the highlighted lines (web files: Tab expands Emmet first)"},
		{"Shift + Tab", "un-indent (Shift + Alt + Tab does too)"},
		{"Alt + 1 … 9", "go to open file 1 … 9"},
		{"Ctrl + PgUp / PgDn", "previous / next open file"},
		{"Click a tab", "go to that file"},
	}},
	{"Brackets and suggestions (code files)", false, []helpLine{
		{"Typing ( [ { \" ' `", "the closer appears after the cursor; with text highlighted: wraps it"},
		{"Typing ) ] } \" ' `", "in front of the same closer: steps over it"},
		{"Backspace in ( )", "deletes both halves of an empty pair"},
		{"Enter in ( ) [ ] { }", "the inside gets its own line"},
		{"Typing a word", "a list shows ways to finish it: CSS properties and values, HTML tags, words in the file"},
		{"  Tab / Up, Down / Esc", "  take the chosen one / choose / close the list"},
		{"Typing : (CSS)", "after a property, the ; appears after the cursor; typing ; steps over it"},
		{"Dim shadow (CSS)", "the recommended unit: font-size: 1 shows rem; Tab takes it; the message says why"},
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
	{"Indent", true, []helpLine{
		{"Alt + Shift + Left / Right", "dedent / indent the line (or the highlighted lines)"},
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

// helpIntro is how many lines come before the first group (helpText).
const helpIntro = 4

func (m *Model) helpText() []string {
	var out []string
	out = append(out, glowText("◆ Symbolizer", cBG)+stHelpText.Render("  ·  keys"))
	out = append(out, stHelpText.Render("F1 or AltGr + K opens this page; AltGr + K opens it at the keys of the mode you are in."))
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
	out = append(out, stHelpNew.Render("Esc, F1, AltGr + K or Q closes this page.  Up / Down scrolls."))
	return out
}

// helpLineOf is the line of helpText where the group called title starts, or 0.
func helpLineOf(title string) int {
	n := helpIntro
	for _, g := range helpGroups {
		if g.title == title {
			return n
		}
		n += 1 + len(g.lines) + 1
	}
	return 0
}

// openKeys opens the keys page at the keys of the mode in use (AltGr + K),
// or closes it when it is open.
func (m *Model) openKeys() tea.Cmd {
	if m.help {
		m.help = false
		return nil
	}
	m.help, m.helpTop = true, 0
	if m.mode > 0 && m.mode < len(modes) {
		m.helpTop = helpLineOf(modes[m.mode].name + " mode")
	}
	return nil
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
