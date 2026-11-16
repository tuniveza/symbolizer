package main

// ---- writing a label or a name plate on the board itself ----------------------------
//
// A label's words (label mode, T) and a name plate (label mode, G; Shift+G
// in group mode, then Enter; Alt+A on several chosen) are written where
// they show, on the board, with a cursor, not at a prompt in the status
// bar. Text mode's keys work (the arrows, Ctrl + arrows, Shift to
// highlight, Backspace, Ctrl+Z …). Enter, Tab or Esc (or Alt tapped twice)
// is done. Empty words: the label shows the structure's title again; an
// empty name plate is no name plate.
//
// A name plate written for several structures at once (Alt+A, group mode)
// is written on the chosen one, and each letter goes on all of them.

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	uv "github.com/charmbracelet/ultraviolet"
)

// The two parts written on the board rather than in a structure (see the
// parts in design.go and partThing, design_sections.go).
const (
	partLabel = 10 // the label's own words
	partPlate = 11 // the name plate
)

// isTagPart is whether p is a label's words or a name plate.
func isTagPart(p int) bool { return p == partLabel || p == partPlate }

// startTagEdit starts writing part p (partLabel or partPlate) of o on the
// board. group are the others that get the same name plate as it is
// written (nil: o alone).
func (m *Model) startTagEdit(o *dOct, p int, group []int) {
	d := m.ds()
	if o == nil {
		return
	}
	d.sel, d.selLink = o.ID, -1
	if p == partLabel {
		if o.LabelStyle == labelNone || o.LabelStyle == labelIcon {
			o.LabelStyle = labelIconTitle
		}
		if o.LabelIcon == "" {
			o.LabelIcon = labelIcons[0]
		}
		if o.LabelColor == "" {
			o.LabelColor = labelColors[0]
		}
	}
	d.save() // what came before is a step of its own
	d.editing = p
	d.tagGroup = nil
	for _, id := range group {
		if id != o.ID && d.byID(id) != nil {
			d.tagGroup = append(d.tagGroup, id)
		}
	}
	if p == partPlate && len(d.tagGroup) > 0 {
		m.tagSpread(o) // they all have its name plate from the start
	}
	d.field = fieldEd{} // the cursor at the end of the words
	d.dirty = true
	m.keepInView(o, false)
	switch {
	case p == partLabel:
		m.say("Writing the label on " + octName(o) + "  ·  Enter: done  ·  empty: its title")
	case len(d.tagGroup) > 0:
		m.say("Writing the name plate for " + plural(len(d.tagGroup)+1, "structure") + "  ·  Enter: done  ·  empty: none")
	default:
		m.say("Writing the name plate on " + octName(o) + "  ·  Enter: done  ·  empty: none")
	}
}

// tagKey is a key while a label or a name plate is being written.
func (m *Model) tagKey(k tea.Key, name string) tea.Cmd {
	d := m.ds()
	o := d.byID(d.sel)
	if o == nil {
		d.editing, d.tagGroup = partNone, nil
		return nil
	}
	switch name {
	case "enter", "ctrl+j", "tab", "shift+tab", "esc":
		m.endTagEdit(o)
		return nil
	}
	return m.designWrite(d, o, k, name) // Text mode's keys on its words (design_field.go)
}

// tagSpread gives the others being named with o (tagGroup) o's name plate.
func (m *Model) tagSpread(o *dOct) {
	d := m.ds()
	for _, id := range d.tagGroup {
		if p := d.byID(id); p != nil {
			p.Plate = o.Plate
		}
	}
}

// endTagEdit stops writing the label or name plate, tidies the words and
// says what it is now.
func (m *Model) endTagEdit(o *dOct) {
	d := m.ds()
	p := d.editing
	d.editing = partNone
	group := d.tagGroup
	d.tagGroup = nil
	d.dirty = true
	if p == partLabel {
		o.LabelTitle = strings.TrimSpace(o.LabelTitle)
		if o.LabelTitle == "" {
			m.say("The label on " + octName(o) + " shows its title.")
		} else {
			m.say("Label “" + o.LabelTitle + "” on " + octName(o) + ".")
		}
		d.save()
		return
	}
	o.Plate = strings.TrimSpace(o.Plate)
	n := 1
	for _, id := range group {
		if q := d.byID(id); q != nil {
			q.Plate = o.Plate
			n++
		}
	}
	d.save()
	switch {
	case o.Plate == "" && n > 1:
		m.say("Name plate taken off " + plural(n, "structure") + ".")
	case o.Plate == "":
		m.say(octName(o) + " has no name plate now.")
	default:
		m.say("Name plate “" + o.Plate + "” on " + plural(n, "structure") + " (" + itoa(len(d.plates()[o.Plate])) + " in the group).")
	}
}

// designAttach is Alt+Shift+S (or Alt+A): the structures chosen together
// (two or more) get one name plate, written on the board at once. With
// fewer chosen, the chosen one's group (its name plate) is renamed.
func (m *Model) designAttach() {
	d := m.ds()
	ids := d.multiIDs()
	if len(ids) < 2 {
		o := d.byID(d.sel)
		if o == nil || o.Plate == "" {
			m.say("Alt+S on two or more (the arrows choose each) gives them one name plate; Alt+Shift+S renames it.")
			return
		}
		ids = nil
		for _, p := range d.octs {
			if p != nil && p.Plate == o.Plate {
				ids = append(ids, p.ID)
			}
		}
		m.startTagEdit(o, partPlate, ids)
		return
	}
	o := d.byID(d.sel)
	if o == nil || !d.multi[o.ID] {
		o = d.byID(ids[0])
	}
	// Offer the name they already share, if they all have the same one.
	same := o.Plate
	for _, id := range ids {
		if d.byID(id).Plate != same {
			same = ""
			break
		}
	}
	o.Plate = same
	m.startTagEdit(o, partPlate, ids)
}

// tagCells draws the words of a label or name plate being written, from
// (x, y), with the cursor and the highlight; fg on bg. It gives the cell
// after the last.
func (d *designState) tagCells(o *dOct, p int, words string, x, y int, fg, bg string, put func(x, y int, cell uv.Cell)) int {
	cur, s, e, ok := d.fieldMarks(o, p)
	rs := []rune(words)
	st := uv.Style{Fg: rgbaOf(fg), Bg: rgbaOf(bg), Attrs: uv.AttrBold}
	hi := uv.Style{Fg: rgbaOf(bg), Bg: rgbaOf(fg), Attrs: uv.AttrBold}
	for i, r := range rs {
		c := st
		if ok && (i == cur || (s != e && i >= s && i < e)) {
			c = hi
		}
		put(x, y, uv.Cell{Content: string(r), Width: 1, Style: c})
		x++
	}
	if ok && cur >= len(rs) { // the cursor after the last letter
		put(x, y, uv.Cell{Content: " ", Width: 1, Style: hi})
		x++
	}
	return x
}
