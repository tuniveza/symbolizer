package main

// ---- dropping files and text on the board -------------------------------------------
//
// In kitty (0.47 and later) the board takes drops through kitty's drag and
// drop protocol (OSC 72, https://sw.kovidgoyal.net/kitty/dnd-protocol/):
// while something is dragged over it, the structure under the pointer lights
// up (or a ✚ shows where a new one would go), and on the drop the files go
// where they were let go. A picture goes into the octagon it is dropped on,
// or into a new octagon there; a text file goes into the page or note it is
// dropped on, into the cabinet as a page, or into a new page there. Dragged
// text (from a browser, say) goes the same way as a text file. Several
// files at once each get a place of their own, side by side.
//
// Other terminals paste a dropped file as its path; designPaste handles
// that, one file or several.

import (
	"encoding/base64"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	uv "github.com/charmbracelet/ultraviolet"
)

// dndTerminal: the terminal speaks kitty's drag and drop protocol. Sending
// the codes elsewhere does no harm (unknown codes are ignored), but kitty
// is the one that answers.
var dndTerminal = os.Getenv("KITTY_WINDOW_ID") != "" || os.Getenv("TERM") == "xterm-kitty"

const (
	oscDnd    = "\x1b]72;"
	oscEnd    = "\x1b\\"
	dndAccept = oscDnd + "t=a" + oscEnd // take drops
	dndStop   = oscDnd + "t=A" + oscEnd // take no more drops
)

// dropState is a drag over the board, or a drop being read.
type dropState struct {
	over       bool     // something is being dragged over the window
	x, y       int      // the cell under it (screen cells)
	offered    []string // the MIME types it offers
	reading    int      // 1-based index of the MIME type being read, or 0
	mime       string   // that type
	data       strings.Builder
	atX, atY   int // where it was dropped
	lastAnswer string
}

// dropsCmd tells the terminal to send drops to sy while the board is shown,
// and to stop when it is not (so in Text mode a drop pastes the path, as
// before).
func (m *Model) dropsCmd() tea.Cmd {
	want := dndTerminal && m.inDesign() && !m.help && m.picker == nil
	if want == m.dropsOn {
		return nil
	}
	m.dropsOn = want
	if want {
		return tea.Raw(dndAccept)
	}
	if m.design != nil {
		m.design.drop = dropState{}
		m.design.dropOn = -1
	}
	return tea.Raw(dndStop)
}

// dndMsg handles one of kitty's drag and drop codes, if msg is one.
func (m *Model) dndMsg(msg tea.Msg) (tea.Cmd, bool) {
	e, ok := msg.(uv.UnknownOscEvent)
	if !ok || !strings.HasPrefix(string(e), oscDnd) {
		return nil, false
	}
	meta, payload := parseDnd(string(e))
	if !m.inDesign() {
		if meta["t"] == "M" { // dropped after the board went: decline it
			return tea.Raw(oscDnd + "t=r:o=0" + oscEnd), true
		}
		return nil, true
	}
	return m.designDnd(meta, payload), true
}

// parseDnd splits "ESC ] 72 ; k=v:k=v ; payload ST" into its keys and payload.
func parseDnd(s string) (map[string]string, string) {
	s = strings.TrimPrefix(s, oscDnd)
	s = strings.TrimSuffix(strings.TrimSuffix(s, oscEnd), "\a")
	head, payload, _ := strings.Cut(s, ";")
	meta := map[string]string{}
	for _, kv := range strings.Split(head, ":") {
		if k, v, ok := strings.Cut(kv, "="); ok {
			meta[k] = v
		}
	}
	return meta, payload
}

func atoi(s string) int { n, _ := strconv.Atoi(s); return n }

// dropTypes are the kinds of data the board takes, best first.
var dropTypes = []string{"text/uri-list", "text/plain;charset=utf-8", "text/plain", "UTF8_STRING"}

func (m *Model) designDnd(meta map[string]string, payload string) tea.Cmd {
	d := m.ds()
	dr := &d.drop
	switch meta["t"] {
	case "m": // dragged over, or away
		x, y := atoi(meta["x"]), atoi(meta["y"])
		if payload != "" {
			dr.offered = strings.Fields(payload)
		}
		if x < 0 || y < 0 {
			dr.over, d.dropOn = false, -1
			m.say("")
			return nil
		}
		dr.over, dr.x, dr.y = true, x, y
		want := m.dropWants(dr.offered)
		op := "1"
		if want == "" || y < textTop || y >= textTop+m.textHeight() {
			op = "0" // off the board, or nothing it can take
		}
		m.dropHint(x, y-textTop, op == "1")
		answer := "t=m:o=" + op
		if answer == dr.lastAnswer {
			return nil
		}
		dr.lastAnswer = answer
		return tea.Raw(oscDnd + answer + ";" + want + oscEnd)
	case "M": // dropped: ask for the best kind of data
		dr.over, dr.lastAnswer, d.dropOn = false, "", -1
		offered := strings.Fields(payload)
		want := m.dropWants(offered)
		y := atoi(meta["y"])
		if want == "" || y < textTop || y >= textTop+m.textHeight() {
			return tea.Raw(oscDnd + "t=r:o=0" + oscEnd)
		}
		idx := 0
		for i, t := range offered {
			if t == want {
				idx = i + 1
			}
		}
		dr.reading, dr.mime, dr.atX, dr.atY = idx, want, atoi(meta["x"]), y-textTop
		dr.data.Reset()
		return tea.Raw(fmt.Sprintf("%st=r:x=%d%s", oscDnd, idx, oscEnd))
	case "r": // the data, in pieces; an empty last piece ends it
		if dr.reading == 0 || atoi(meta["x"]) != dr.reading {
			return nil
		}
		dr.data.WriteString(payload)
		if meta["m"] == "1" || payload != "" {
			return nil
		}
		raw, err := base64.RawStdEncoding.DecodeString(strings.TrimRight(dr.data.String(), "="))
		mime, x, y := dr.mime, dr.atX, dr.atY
		dr.reading = 0
		dr.data.Reset()
		done := tea.Raw(oscDnd + "t=r:o=1" + oscEnd)
		if err != nil {
			m.sayErr("The drop could not be read: " + err.Error())
			return tea.Raw(oscDnd + "t=r:o=0" + oscEnd)
		}
		return tea.Batch(done, m.designDropped(mime, string(raw), x, y))
	case "R": // the terminal could not give the data
		dr.reading = 0
		dr.data.Reset()
		m.sayErr("The drop could not be read: " + payload)
	}
	return nil
}

// dropWants is the best of the offered types the board takes, or "".
func (m *Model) dropWants(offered []string) string {
	for _, t := range dropTypes {
		for _, o := range offered {
			if o == t {
				return t
			}
		}
	}
	return ""
}

// dropHint lights up where a drop at board cell (sx, sy) would go, and says so.
func (m *Model) dropHint(sx, sy int, ok bool) {
	d := m.ds()
	d.dropOn = -1
	if !ok {
		m.say("Drop on the board.")
		return
	}
	o := d.hit(sx, sy).o
	if o == nil {
		m.say("Drop here: each file gets a new structure where you let go.")
		return
	}
	d.dropOn = o.ID
	switch o.Kind {
	case kOct:
		m.say("Drop on " + octName(o) + ": a picture goes into it, text into a new page beside it.")
	case kCabinet:
		m.say("Drop on " + octName(o) + ": text files are filed in it as pages.")
	default:
		m.say("Drop on " + octName(o) + ": text goes at its end.")
	}
}

// designDropped places what was dropped at board cell (sx, sy).
func (m *Model) designDropped(mime, data string, sx, sy int) tea.Cmd {
	d := m.ds()
	defer d.save()
	target := d.hit(sx, sy).o
	wx, wy := d.world(sx, sy)
	if mime != "text/uri-list" {
		text := textLike(data)
		if paths := droppedPaths(data); len(paths) > 0 {
			return m.dropFiles(paths, target, wx, wy) // text that names files
		}
		if target != nil && target.Kind != kOct && target.Kind != kCabinet {
			m.designAddText(target, text)
			return nil
		}
		o := d.newKind(kPage, int(wx), int(wy))
		d.editing = partNone
		m.designAddText(o, text)
		return nil
	}
	var paths []string
	for _, line := range strings.Split(data, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if u, err := url.Parse(line); err == nil && u.Scheme == "file" {
			line = u.Path
		}
		paths = append(paths, line)
	}
	return m.dropFiles(paths, target, wx, wy)
}

// dropFiles puts each file in place: the first on target (or at wx, wy),
// the rest beside it, each in a structure of its own.
func (m *Model) dropFiles(paths []string, target *dOct, wx, wy float64) tea.Cmd {
	var cmds []tea.Cmd
	var skipped []string
	n := 0
	for _, p := range paths {
		info, err := os.Stat(p)
		if err != nil || info.IsDir() {
			skipped = append(skipped, filepath.Base(p))
			continue
		}
		// Side by side, four to a row: room for an octagon with a picture.
		at := [2]float64{wx + float64(n%4*38), wy + float64(n/4*20)}
		into := target
		if n > 0 && (into == nil || into.Kind == kOct) {
			into = nil // one picture per octagon: the next ones get their own
		}
		cmds = append(cmds, m.designUseFileAt(p, into, &at))
		n++
	}
	if len(skipped) > 0 {
		m.sayErr("Not put in (folders, or not found): " + strings.Join(skipped, ", ") + ". Drop the files inside a folder.")
	}
	return tea.Batch(cmds...)
}

// droppedPaths is the files a paste names, when every line of it is one
// existing file (terminals paste dropped files as their paths, one a line,
// sometimes quoted or as file:// addresses). Otherwise nil.
func droppedPaths(s string) []string {
	var paths []string
	for _, line := range strings.Split(strings.TrimSpace(s), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		p := droppedPath(line)
		if p == "" {
			return nil
		}
		paths = append(paths, p)
	}
	return paths
}
