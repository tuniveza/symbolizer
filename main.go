// Symbolizer: a text editor for any terminal, in the Emberistal theme.
//
//	sy                 open again what was open when sy last closed
//	sy --new           start with today's session instead
//	sy notes.txt       open (or create) notes.txt
//	sy notes.txt +42   open notes.txt at line 42
//	sy -t FOLDER       open the text files in a folder and its folders as tabs
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
	path, line, startMode, tabs, fresh := "", 0, 0, false, false
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
				"  sy                 open again what was open when sy last closed\n" +
				"  sy --new           Text mode: carry on with today's session (a draft with no name)\n" +
				"  sy design          Design mode   (zsh: de)\n" +
				"  sy slide           Slides mode   (zsh: sli)\n" +
				"  sy diary           Diary mode    (zsh: syd, di)\n" +
				"  sy book           Book mode     (zsh: bo)\n" +
				"  sy draw           Draw mode     (zsh: dr)\n" +
				"  sy FILE            open (or create) FILE\n" +
				"  sy FOLDER          open FOLDER as a project, with the file manager\n" +
				"  sy -t [FOLDER]     open the text files in FOLDER (or here), subfolders too, as tabs\n" +
				"  sy FILE +LINE      open FILE at line LINE\n" +
				"  sy --keys          check which keys reach Symbolizer\n" +
				"  sy --font          install the shape letters font for Design mode's 3D room (and tell kitty)\n\n" +
				"Inside, press F1 to see every key. Alt+Q quits.")
			return
		case a == "--keys":
			if _, err := tea.NewProgram(&keyCheck{releases: "waiting for the terminal to answer…"}).Run(); err != nil {
				fmt.Fprintln(os.Stderr, "sy: "+err.Error())
				os.Exit(1)
			}
			return
		case a == "--font":
			if err := geoInstall(os.Stdout); err != nil {
				fmt.Fprintln(os.Stderr, "sy: "+err.Error())
				os.Exit(1)
			}
			return
		case a == "-n" || a == "--new":
			fresh = true
		case a == "-t" || a == "--tabs":
			tabs = true
		case a == "-v" || a == "--version":
			fmt.Println(strings.TrimSpace("Symbolizer " + version + " " + buildNumber))
			return
		case strings.HasPrefix(a, "+"):
			line, _ = strconv.Atoi(a[1:])
		default:
			path = a
		}
	}
	if tabs && path == "" {
		path = "."
	}
	m, err := NewModel(path, line)
	if err != nil {
		fmt.Fprintln(os.Stderr, "sy: "+err.Error())
		os.Exit(1)
	}
	if tabs && m.tree != nil && m.openProject(m.tree.root, pickText) {
		m.closeTree()
	}
	if path == "" && startMode == 0 && !fresh {
		m.reopenLast()
	}
	if startMode != 0 {
		m.mode = startMode
		m.say(modes[startMode].Icon() + "  " + modes[startMode].name + " mode. Alt+M switches modes.")
	}
	designFrames, designSaveLater, designAnims, room3DPace, room3DGPU = true, true, true, true, true
	out := &termOut{File: os.Stdout}                              // Bubble Tea's frames and the 3D room's pictures share it (design_3d_kitty.go)
	p := tea.NewProgram(m, tea.WithFPS(120), tea.WithOutput(out)) // 120: the most Bubble Tea allows, smoother scrolling
	if room3DPixels = kittyWanted(os.Getenv); room3DPixels {
		// The 3D room as a picture: written to kitty between Bubble Tea's frames (design_3d_kitty.go).
		room3DRemote = kittyRemote(os.Getenv)
		startKitty(out.WriteString, func() { p.Send(room3DFrameMsg{}) })
	}
	_, err = p.Run()
	kittyPic.stop(os.Stdout) // the 3D room's picture deleted
	hyprPtr.show()           // the pointer shown again (design_3d_pointer.go)
	if room3DPixels {
		fmt.Print(pixelMouseOff) // the mouse back to cells (design_3d_mouse.go)
	}
	if m.design != nil {
		m.design.flushSave() // the board's last changes
	}
	if m.dropsOn {
		fmt.Print(dndStop) // drops go back to being pasted
	}
	if !m.quitClean {
		m.saveNow()
		if saved := rescue(m.buf); saved != "" {
			fmt.Fprintln(os.Stderr, "sy: closed with unsaved changes. They are in "+saved)
		}
	}
	m.saveLastOpen()
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
