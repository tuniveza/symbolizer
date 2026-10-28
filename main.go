// Symbolizer: a text editor for any terminal, in the Emberistal theme.
//
//	sy                 start with a new file
//	sy notes.txt       open (or create) notes.txt
//	sy notes.txt +42   open notes.txt at line 42
//
// Press F1 inside for every key.
package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
)

const version = "0.1.0"

func main() {
	path, line, startMode := "", 0, 0
	for i, a := range os.Args[1:] {
		// The first word can be a mode: sy design, sy slide, sy diary.
		// (A file with one of these names opens with ./design.)
		if n, ok := modeWords[strings.ToLower(a)]; ok && i == 0 {
			startMode = n
			continue
		}
		switch {
		case a == "-h" || a == "--help":
			fmt.Println("Symbolizer " + version + ", a text editor in the Emberistal theme.\n\n" +
				"  sy                 Text mode: carry on with today's session (a draft with no name)\n" +
				"  sy design          Design mode   (zsh: de)\n" +
				"  sy slide           Slides mode   (zsh: sli)\n" +
				"  sy diary           Diary mode    (zsh: syd, di)\n" +
				"  sy book           Book mode     (zsh: bo)\n" +
				"  sy draw           Draw mode     (zsh: dr)\n" +
				"  sy FILE            open (or create) FILE\n" +
				"  sy FOLDER          open FOLDER as a project, with the file manager\n" +
				"  sy FILE +LINE      open FILE at line LINE\n" +
				"  sy --keys          check which keys reach Symbolizer\n\n" +
				"Inside, press F1 to see every key. Alt+Q quits.")
			return
		case a == "--keys":
			if _, err := tea.NewProgram(&keyCheck{releases: "waiting for the terminal to answer…"}).Run(); err != nil {
				fmt.Fprintln(os.Stderr, "sy: "+err.Error())
				os.Exit(1)
			}
			return
		case a == "-v" || a == "--version":
			fmt.Println("Symbolizer " + version)
			return
		case strings.HasPrefix(a, "+"):
			line, _ = strconv.Atoi(a[1:])
		default:
			path = a
		}
	}
	m, err := NewModel(path, line)
	if err != nil {
		fmt.Fprintln(os.Stderr, "sy: "+err.Error())
		os.Exit(1)
	}
	if startMode != 0 {
		m.mode = startMode
		m.say(modes[startMode].Icon() + "  " + modes[startMode].name + " mode. Alt+M switches modes.")
	}
	p := tea.NewProgram(m)
	_, err = p.Run()
	if !m.quitClean {
		m.saveNow()
		if saved := rescue(m.buf); saved != "" {
			fmt.Fprintln(os.Stderr, "sy: closed with unsaved changes. They are in "+saved)
		}
	}
	m.storeTab()
	for _, t := range m.tabs {
		if t.buf.Draft && fileExists(t.buf.Path) {
			fmt.Println("Session kept: " + shortPath(t.buf.Path) + "   (sy carries on with it for 24 hours; Alt+L lists all sessions)")
		}
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "sy: "+err.Error())
		os.Exit(1)
	}
}
