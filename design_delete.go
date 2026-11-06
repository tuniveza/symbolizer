package main

// ---- deleting ---------------------------------------------------------------------
//
// Everything on the board can be deleted: a structure of any kind (with what
// its drawer holds), a link, a thing in a drawer or a cabinet, an octagon's
// picture. Delete (or Backspace) deletes what is chosen and the ✕ on it does
// the same with the mouse; Ctrl+Z brings it back, like any other step
// (design_undo.go). Pen ink has its own rubber (E, or a right-drag).

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
)

// deleteSpot is where the chosen structure's ✕ is: on its top edge, near the
// right corner (left of nothing else drawn there). Small ones have none:
// Delete still deletes them.
func (d *designState) deleteSpot(o *dOct, x, y, w, h int) (int, int, bool) {
	if d.sel != o.ID || d.linkMode || w < 8 || h < 3 {
		return 0, 0, false
	}
	return x + w - 4, y, true
}

// pictureDeleteSpot is where the ✕ on the chosen octagon's picture is: its
// top right corner.
func (d *designState) pictureDeleteSpot(o *dOct, x, y int) (int, int, bool) {
	b, ok := d.picBox[o.ID]
	if !ok || d.sel != o.ID || d.linkMode || o.Image == "" || b[2] < 3 || b[3] < 2 {
		return 0, 0, false
	}
	return x + b[0] + b[2] - 1, y + b[1], true
}

// deleteStruct takes structure o off the board, with its links.
func (d *designState) deleteStruct(o *dOct) {
	d.remove(o.ID)
}

// designRemove deletes a structure; when it holds things, it asks first.
func (m *Model) designRemove(o *dOct) tea.Cmd {
	d := m.ds()
	n := countItems(o)
	if n == 0 {
		d.deleteStruct(o)
		m.say("Deleted " + octName(o) + " (and its links). Ctrl+Z brings it back.")
		return nil
	}
	things := fmt.Sprintf("the %d things", n)
	if n == 1 {
		things = "the 1 thing"
	}
	return m.confirm(fmt.Sprintf("Delete %s and %s in it? y = yes, n = no", octName(o), things), "yn", func(m *Model, k rune) tea.Cmd {
		if k == 'y' {
			m.ds().deleteStruct(o)
			m.ds().save()
			m.say("Deleted " + octName(o) + ". Ctrl+Z brings it back.")
		} else {
			m.say("Kept.")
		}
		return nil
	})
}

// designDeleteLink deletes link i.
func (m *Model) designDeleteLink(i int) {
	d := m.ds()
	if i < 0 || i >= len(d.links) {
		return
	}
	l := d.links[i]
	d.links = append(d.links[:i:i], d.links[i+1:]...)
	d.selLink, d.dirty = -1, true
	name := octName(d.byID(l.A)) + " – " + octName(d.byID(l.B))
	m.say("Deleted the link " + name + ". Ctrl+Z brings it back.")
}

// designDeletePicture takes the picture out of octagon o (its file stays,
// so Ctrl+Z can put it back).
func (m *Model) designDeletePicture(o *dOct) {
	d := m.ds()
	if o.Image == "" {
		return
	}
	o.Image, d.dirty = "", true
	m.say("Deleted the picture of " + octName(o) + ". Ctrl+Z brings it back · I puts in another.")
}

// designDeleteItem deletes thing i of o's drawer (or cabinet).
func (m *Model) designDeleteItem(o *dOct, i int) {
	d := m.ds()
	if i < 0 || i >= len(o.Items) || o.Items[i] == nil {
		return
	}
	it := o.Items[i]
	o.Items = append(o.Items[:i:i], o.Items[i+1:]...)
	d.dirty = true
	m.say("Deleted " + kindIcons[it.Kind] + " " + itemTitle(it) + " from " + octName(o) + ". Ctrl+Z brings it back.")
}

// contains is whether p is on the board: a structure, or in a drawer or
// cabinet of one, however deep.
func (d *designState) contains(p *dOct) bool {
	var in func([]*dOct) bool
	in = func(os []*dOct) bool {
		for _, o := range os {
			if o == p || (o != nil && in(o.Items)) {
				return true
			}
		}
		return false
	}
	return in(d.octs)
}

// linked is whether structures a and b have a link.
func (d *designState) linked(a, b int) bool {
	for _, l := range d.links {
		if l.A == a && l.B == b || l.A == b && l.B == a {
			return true
		}
	}
	return false
}

// itemDeleteAct turns a click on a drawer or cabinet row into its delete
// when it is on the row's ✕ (the cell before its last): col counts from
// the row's first cell, w is the row's width.
func itemDeleteAct(act string, col, w int) string {
	if strings.HasPrefix(act, "item:") && col == w-2 {
		return "del:" + strings.TrimPrefix(act, "item:")
	}
	return act
}
