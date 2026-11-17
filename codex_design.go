package main

// ---- a codex on the Design board ----------------------------------------------------------
//
// A codex structure (kCodex) shows its codex's cover, as the codex is now.
// Its From is "codex:<id>"; its Header keeps the title, for containers'
// rows. It goes into drawers, racks, boards and boxes like anything else.
//
//	Alt+N, then K   a new codex, here on the board
//	Enter           open it in Codex mode (Esc there comes back)
//	E               open it in Codex mode, to edit
//	double-click    open it
//
// In Codex mode's library, P puts the chosen codex on the board.

import (
	"bytes"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

const kCodex = "codex"

const codexBoardW, codexBoardH = 30, 13

func init() {
	kindIcons[kCodex] = "▥"
	kindNames[kCodex] = "codex"
	for _, k := range []string{"k", "K", "shift+k"} {
		placeKeys[k] = kCodex
	}
}

// codexIDOf is the codex structure o shows ("" none).
func codexIDOf(o *dOct) string {
	if o == nil || o.Kind != kCodex {
		return ""
	}
	return strings.TrimPrefix(o.From, "codex:")
}

// codexLayers draws codex structure o: its codex's cover.
func (d *designState) codexLayers(o *dOct) []*lipgloss.Layer {
	x, y, w, h := d.rect(o)
	cf, err := codexPeek(codexIDOf(o))
	if err != nil {
		cf = &codexFile{Title: itemTitle(o) + " (not found)", Color: "#555555"}
	}
	cv := codexCover(cf, w, h, 0, d.sel == o.ID)
	l := lipgloss.NewLayer("").X(x).Y(y)
	d.preCells(l, cv)
	return []*lipgloss.Layer{l}
}

// codexNewOnBoard makes a new codex for structure o, just placed.
func (m *Model) codexNewOnBoard(o *dOct) {
	id, cf, err := createCodex("")
	if err != nil {
		m.sayErr("Could not make the codex: " + err.Error())
		return
	}
	o.From = "codex:" + id
	o.Header = codexName(cf.Title)
	o.Color = codexOr(cf.Color, codexThemeCover())
	m.say("New codex on the board. Enter opens it · E edits it (its title first).")
}

// codexFromDesign opens codex structure o in Codex mode (edit: to edit it).
func (m *Model) codexFromDesign(o *dOct, edit bool) tea.Cmd {
	id := codexIDOf(o)
	if id == "" {
		return nil
	}
	if _, err := codexPeek(id); err != nil {
		m.sayErr("Its codex is not there any more (" + id + ").")
		return nil
	}
	d := m.ds()
	d.save()
	cmd := m.setMode(codexMode)
	c := m.cx()
	if !m.codexOpen(id, edit) {
		return cmd
	}
	c.fromDesign, c.designID = true, o.ID
	if edit && strings.TrimSpace(c.cf.Title) == "" {
		m.codexStartWrite(codexWriteTitle)
		m.say("Type the codex's title, then Enter (Tab: a subtitle) · Esc twice: back to the board.")
	}
	return tea.Batch(cmd, m.codexTickCmd())
}

// codexOnBoard is whether any Design board (in any room, on any wall)
// shows codex id: the board open now, and the boards' files.
func (m *Model) codexOnBoard(id string) bool {
	if id == "" {
		return false
	}
	if m.design != nil {
		for _, o := range m.design.octs {
			if codexIDOf(o) == id {
				return true
			}
		}
	}
	want := []byte(`"codex:` + id + `"`)
	found := false
	filepath.WalkDir(filepath.Dir(designPath()), func(p string, de fs.DirEntry, err error) error {
		if err != nil || de.IsDir() || !strings.HasSuffix(p, ".json") {
			return nil
		}
		if b, err := os.ReadFile(p); err == nil && bytes.Contains(b, want) {
			found = true
			return filepath.SkipAll
		}
		return nil
	})
	return found
}

// codexToBoard puts codex id on the Design board, in the middle of the view.
func (m *Model) codexToBoard(id string) tea.Cmd {
	cf, err := codexPeek(id)
	if err != nil {
		m.sayErr("Could not read that codex: " + err.Error())
		return nil
	}
	cmd := m.setMode(designMode)
	d := m.ds()
	wx, wy := d.world(m.width/2, m.boardH()/2)
	o := d.newKind(kCodex, int(wx), int(wy))
	o.From, o.Header, o.Color = "codex:"+id, codexName(cf.Title), codexOr(cf.Color, codexThemeCover())
	d.clearSpot(o)
	d.sel, d.editing = o.ID, partNone
	m.centreOn(o)
	d.save()
	m.say("“" + codexName(cf.Title) + "” is on the board. Enter opens it.")
	return cmd
}
