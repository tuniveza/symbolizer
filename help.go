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
		{"Alt + Tab", "in Design mode and in sy term: the other split (the editor ↔ sy term; kitty does it). In the other modes: tab switch mode"},
		{"Alt + R, then arrows", "resize mode: each arrow moves the line between the panes that way (Up: up …); any other key leaves (kitty does it; not in Text mode, where Alt + R is Emmet's remove tag)"},
		{"", "  moving shows it · Enter keeps it · Esc puts it back"},
	}},
	{"Modes", false, []helpLine{
		{"Alt + M", "switch modes: Text, Design, Slides, Diary, Book, Draw, Manager, Brick, Codex (Slides, Book, Draw are empty for now)"},
		{"sy manager", "start in Manager mode: all of ~/an at a glance (zsh: ma)"},
		{"sy design / slide / diary", "start in that mode (zsh: de, sli, syd or di)"},
		{"sy book / draw", "start in Book or Draw mode (zsh: bo, dr)"},
	}},
	{"Design mode", false, []helpLine{
		{"(the board)", "kept in ~/.local/share/symbolizer/design/ (board.json, a file per structure in parts/, the ink in marks.json)"},
		{"Double-click the board", "a new octagon structure (or what the toolbar's new chooses): type the header"},
		{"  Tab · Enter · Esc", "  header → text → footer · next part (a new line in the text) · done (or Alt tapped twice)"},
		{"  While writing", "  Text mode's keys, with a cursor: arrows (Ctrl: a letter, Alt: line start / end, a block), Shift highlights, Home · End · Ctrl + A, Ctrl + C · X · V, Backspace · Delete (Ctrl: a word), Ctrl + Z · Y undo the typing, Alt + Shift + arrows, Ctrl + D, Ctrl + K"},
		{"Drag · its ◢ corner", "move a structure · resize it (double-click one to write in it)"},
		{"Middle-drag · wheel", "move the board (left-drag on empty board too) · zoom in and out, toward the pointer"},
		{"⛓ link on the toolbar", "link mode: click a structure, then each one to attach (click again to detach)"},
		{"Arrows", "jump the view to the closest structure that way and choose it (held: on to the next one); nothing that way: a step"},
		{"E", "write in the chosen structure's text (a cabinet: its name); the view centres on it whenever writing starts"},
		{"AltGr + M  (µ)", "the mini map: every structure in its own outline and colour with its symbol and title, links as dotted lines, a frame for the screen; again: the next corner, then hidden"},
		{"Alt tapped twice", "Esc: stop writing, leave link mode, put the pen down, or let go of the chosen structure"},
		{"Space held + arrows", "move the view freely, a step at a time (held: it glides); let go of Space and the view snaps to the structure nearest the middle"},
		{"Shift + arrows", "move the view freely (held: it glides on at once, no pause); the structure nearest the middle is chosen as you go"},
		{"Two arrows held (Up + Right …)", "move the view diagonally (with Alt: the chosen structure)"},
		{"Alt + arrows (Alt + Shift)", "move the chosen structure (further); near the closest one it snaps a small gap away from it and stops there"},
		{"Tab · Shift + Tab", "switch areas: write in the chosen structure's header, then its text, then its footer (back), and the view moves to it; on a container: its name, then each of its sections (drawers, shelves, rows, pins, layers and the things in them), each written in by itself, round to its name; an arrow leaves, Esc stops writing"},
		{"+ · − · 0", "zoom in · out · back to 100%, around the chosen structure (it stays in the middle); the keypad's + and − too"},
		{"AltGr + R · AltGr + Shift + R", "turn to the next wall of the room · the one before, with a spin. Every room has 4 walls (more if added), each a board of its own"},
		{"AltGr + F", "look down at the room's floor (the place for what is not finished yet), and up again"},
		{"M", "chiptune music on the board, on or off (kept for next time; SY_MUSIC=0: never)"},
		{"Windows", "each room has 1 to 4 windows on its walls, with the town outside and its people"},
		{"1–8", "highlighter pen colours (press the same one again to put it down)"},
		{"  ⌫ · over/under on the toolbar", "  rubber · pen under the text or over it; right-drag rubs out too"},
		{"Alt + N, then P · N · S", "place a new page · note · shape in the middle of the screen"},
		{"AltGr + N, then R · W · L", "a new room on this level, with a door to it · a new wall after this one · a new level above the top one, with stairs up to it"},
		{"Enter or Space on a door or stairs", "go through to its room. The door back is chosen there, so Enter again goes back. A door's title is its room's name"},
		{"Alt + N, then K", "place a codex: its cover on the board; Enter opens it in Codex mode"},
		{"Notepads", "every .txt saved with a name in Text mode is a note, 16 to a notepad; each notepad looks different. Enter or double-click on one opens it"},
		{"  In an open notepad", "  its notes by day, newest first · ↑ ↓ choose · Enter: open in Text mode · F: sticker · / or S: only one sticker's notes (again: the next, then all) · X X: take out of the notepad (the .txt file stays) · PgUp · PgDn: other notepads · Esc: close"},
		{"Alt + N, then C · D · R · B · O", "place a container: cabinet (two doors, a shelf for each kind) · drawer (one or more drawers, each with a handle; inside, everything small) · rack (a stack: click a row to open it inside the rack; Tab · Shift+Tab: its rows) · board (things pinned on it as cards with their title and text) · box (things in it on three layers: top, middle, bottom; a container in it lies folded, and opens out while chosen)"},
		{"  Cabinet · drawer", "  click the doors · a drawer's front (or Space, with it chosen) to open or shut it; click a thing inside to take it out"},
		{"  [ · ]", "  with a drawer unit chosen: one drawer fewer · one more (or its [−] · [+] on its bottom edge)"},
		{"  C · Shift+C", "  with a drawer unit chosen: the open drawer's next colour · next texture (or click the dot · swatch on its front)"},
		{"  Box", "  drop things in (top layer, where let go); click a thing to choose it, drag it to take it out; Tab steps through its layers and things; PgUp · PgDn: its layer above · below (none chosen: look at one layer, the ones above see-through); Alt + arrows move it; Enter takes it out; X X takes it away"},
		{"  D · Shift + D", "  with a box chosen: its next · last design (cardboard, crate, tin, gift, hat box, hexagon; or its [◇])"},
		{"  Board", "  drop things on it to pin them; click a pin to open it; drag it to move it; Tab steps to a pin (writing its title; Esc: stop writing), then R turn · S size · W H stretch · K J skew · F V flip · Alt + arrows move · T type its transform (rotate 30 scale 1.5 skew 0.2 …) · X X take it off"},
		{"Alt + L", "label mode (again, Esc or Alt tapped twice: off). On the chosen structure:"},
		{"  L · C · I", "  label: icon + title → title + icon → icon → title → none · its colour · its icon (symbols, then pictures in design/images)"},
		{"  T · G", "  write the label's own words · its name plate, on the board where it shows (Enter: done); all with the same name plate are a group"},
		{"Space", "on (or with the mouse over) a cabinet, drawer, rack, board or box: open it or shut it (a box: its lid); "},
		{"Alt + Space", "grab: the chosen structure, moved by the arrows. Alt + Space again (or a click) puts it down, Esc or Alt twice puts it back. On a thing inside a container: shows it big (Alt + Space, Esc or Alt twice: back; Tab: the next one)"},
		{"Ctrl + drag · Ctrl + click", "a shaded box chooses what it touches · choose one more (or one less); then drag one or Alt + arrows move them all, X X deletes them, Esc lets go"},
		{"Alt + S", "the structure the arrows chose joins those chosen (again: leaves them). Two or more share one name plate at once (a new “Group N”, or one they had). The only key that chooses"},
		{"Alt + Shift + S", "rename the chosen group's name plate, written on the board (Enter: done)"},
		{"AltGr + P  (þ)", "the list of name plates and what has each"},
		{"X X  (X twice, quickly)", "delete what is chosen: a structure or a link (✕ on it does the same)"},
		{"Ctrl+Z · Ctrl+Y (or Ctrl+Shift+Z)", "undo · redo any change: a move, words, a deletion, ink (also ↶ ↷ on the toolbar)"},
		{"Click a link", "choose the link (X X deletes it)"},
		{"✕ on a container's row", "delete that thing"},
		{"✕ on a picture", "take the picture out of its shape (the structure stays)"},
		{"  ◆ ≡ ✎ ≣ on the toolbar", "  what a double-click on the empty board places"},
		{"img on the toolbar", "a picture (any format) for the chosen shape, or a text file for the chosen page; type its path"},
		{"Drag files onto the board", "in kitty: each goes where you let go (into what is under it, or a new structure there)"},
		{"AltGr + L  (ł) · [¶] on the top edge", "the chosen shape's next layout: text · picture above · left · right · picture only · two columns"},
		{"Cabinet: click a stack", "pull out Pages, Books, Slides, Notes or Drawings; drag a thing out onto the board"},
		{"  Drop a page or note", "  on a cabinet: it is filed in its stack"},
		{"Drawer · rack", "drop anything on it (a page, note or shape); drag a row out to take it out"},
		{"  Rack: click a row", "  opens it in the middle of the view; Alt tapped twice (or a click on the board) puts it back"},
		{"  ⇣ rows in Pages", "  bring in the Diary's pages, or the files open in Text mode"},
		{"Wheel on a page", "scroll it (Shift + wheel)"},
		{"(height)", "no structure is drawn taller than the screen, at any zoom: its text scrolls, its picture shrinks to fit"},
		{"(text)", "all text in Design mode is centred: on the board, and the bars above and below it"},
	}},
	{"Codex mode", false, []helpLine{
		{"(what it is)", "books you can play, like HyperCard: pages of text, pictures, drawings, moving symbols and buttons behind a cover; kept in ~/.local/share/symbolizer/codex/ (sy codex starts here)"},
		{"Library", "arrows choose · Enter read · E edit · Alt + N new codex · P put it on the Design board · X X delete"},
		{"Reading", "→ PgDn Space next page (the cover opens) · ← PgUp back · Home cover · End last · Tab the next thing to press · Enter or a click presses it · E edit · Esc close (back to the board, if opened there)"},
		{"Editing", "arrows choose a thing · PgUp PgDn pages · Alt + N, then T text · B button · I picture · S symbol · D drawing · Enter writes it (a picture: its file; a drawing: draw)"},
		{"  L · Shift + L", "  the page's layout: 16 of them (one area, title and text, columns, grids, sidebars, comic strip …)"},
		{"  F C A + − S J", "  font (16) · colour · animation (16) · size (4) · symbol · left / middle / right; Shift: back"},
		{"  K · G", "  a button's action (next page, page before, go to page, activate a thing) · what it aims at"},
		{"  T", "  the activation table: what activating each thing does (flash, hide, show, animate, colour, font, size, swap its words, inside out)"},
		{"  Alt + ← → · Alt + ↑ ↓", "  move it to the slot before / after · up / down in its slot"},
		{"  W · O · Alt + P", "  paper colour · pages wide or upright · a new page; X X deletes the thing (none chosen: the page); Space tries it; Ctrl + Z · Y"},
		{"  The cover", "  Enter title · Tab subtitle · C colour · S symbol · W the pages' paper"},
		{"  Drawing", "  arrows move the brush · Space pen down / up · 1–8, 9 white, 0 dark · R rubber · B brush size · the mouse draws · Esc done"},
		{"On the Design board", "Alt + N, then K: a new codex there (its cover); Enter opens it, E edits it, double-click opens it; it goes into drawers, racks, boards and boxes"},
	}},
	{"Manager mode", false, []helpLine{
		{"(what it shows)", "all of ~/an sorted into its top folders, named from ~/an/ca/sy/manager/names.tsv; ◆ is a project (make.sh, go.mod …); Other projects: those outside pj, ca, co; Recovered files and Downloaded source: code outside ~/an"},
		{"Up / Down", "choose a category; once in one, a row (PgUp, PgDn, Home, End too)"},
		{"Right / Enter", "go into the chosen category: Up / Down then choose its rows"},
		{"Left / Esc", "back out to the categories"},
		{"type", "search everything at once; Backspace takes a letter off, Esc or Left clears it"},
		{"Tab", "sort: by folder, biggest first, newest first"},
		{"Enter (in a category)", "open it in Text mode: a folder with the file manager on it (and its README), a file as a tab"},
		{"Shift + Enter", "open it in the desktop's own app (the file manager, a PDF viewer …)"},
		{"Alt + C / F5", "copy its path / look at the folders again (it also looks each time the mode opens)"},
		{"Alt + A", "actions: run, build, clean, watch or open the chosen project; for all of ~/an: new project, tidy, duplicates, set up tools, status …"},
		{"  ↑↓ · Enter · Esc", "  choose · do it (it runs in the terminal, then any key comes back) · close"},
		{"F6 / F7", "run / build the chosen project (its make.sh)"},
	}},
	{"Brick mode", false, []helpLine{
		{"How it works", "write what the program should do in plain English, one line at a time: write hello in red"},
		{"Colours", "each word turns into a coloured brick: do, thing, colour, place, time, count, words to show"},
		{"The panel", "the brain's reading of each line, live as you type: ✓ read as written · → understood as something else · ✗ can't build yet, and why · ? may not do what you meant, with its fix under it"},
		{"Alt + R", "run it: it is built while you type, so it starts at once (a fix of the brain's is written in first; Ctrl + Z takes it back)"},
		{"Alt + C", "check every line again now"},
		{"Alt + P", "the panel takes the keys: Left / Right pick a fix, Enter writes it in, B bytes of the cursor's line, S save the bits · Esc gives the keys back"},
		{"Alt, Alt, then B", "tap Left Alt twice, then press B: brick mode, from any mode"},
		{"F12", "brick learning on / off (in every mode)"},
		{"AltGr + D / V", "brick learns from your diary on / off (it stays on your PC) · the brains' voices on / off"},
		{"Lines that build", "write hello in red · wait 2 seconds · write goodbye in blue at the bottom · repeat 3 times: write hi"},
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
