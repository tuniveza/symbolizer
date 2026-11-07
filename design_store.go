package main

// ---- the board on disk ------------------------------------------------------------
//
// The board is kept in parts, so a save writes only what changed: typing in
// one page of a large project writes that page, not every page.
//
//	design/board.json        the index: which structures, in what order, the
//	                         links, where the board was looked at, the zoom
//	design/parts/<id>.json   one structure each, with what its drawer holds
//	design/marks.json        the ink
//	design/board.v1.json     the board as it was before it was kept in parts
//
// Each file is written whole to a .tmp beside it and then renamed over the
// old one, so a file is always either the old one or the new one. Parts are
// written first and the index last, so the index never names a part that is
// not there yet. A part is removed only after an index without it is
// written, and only when this sy deleted that structure: a part no index
// names is left alone.

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
)

// designIndex is board.json, kept in parts (version 2).
type designIndex struct {
	Version int     `json:"version"`
	Order   []int   `json:"order"` // the structures' IDs, the last on top
	Links   []dLink `json:"links"`
	CamX    float64 `json:"camX"`
	CamY    float64 `json:"camY"`
	Zoom    int     `json:"zoom"`
	NextID  int     `json:"nextID"`
}

const designDiskVersion = 2

func partsDir(boardPath string) string { return filepath.Join(filepath.Dir(boardPath), "parts") }

func partPath(boardPath string, id int) string {
	return filepath.Join(partsDir(boardPath), strconv.Itoa(id)+".json")
}

func marksPath(boardPath string) string { return filepath.Join(filepath.Dir(boardPath), "marks.json") }

// writeBoardFile writes data to path through a .tmp file renamed over it.
func writeBoardFile(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// readDesignFile reads the board at path, kept whole (version 1) or in
// parts (version 2), as one designFile. inParts says which it was.
func readDesignFile(path string) (f designFile, inParts bool, err error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return f, false, err
	}
	var probe struct {
		Version int `json:"version"`
	}
	if err := json.Unmarshal(data, &probe); err != nil {
		return f, false, err
	}
	if probe.Version < designDiskVersion {
		err = json.Unmarshal(data, &f)
		return f, false, err
	}
	var idx designIndex
	if err := json.Unmarshal(data, &idx); err != nil {
		return f, true, err
	}
	f = designFile{Links: idx.Links, CamX: idx.CamX, CamY: idx.CamY, Zoom: idx.Zoom, NextID: idx.NextID}
	var missing []int
	for _, id := range idx.Order {
		var o dOct
		pd, err := os.ReadFile(partPath(path, id))
		if err == nil {
			err = json.Unmarshal(pd, &o)
		}
		if err != nil {
			missing = append(missing, id)
			continue
		}
		f.Octs = append(f.Octs, &o)
	}
	if md, err := os.ReadFile(marksPath(path)); err == nil {
		json.Unmarshal(md, &f.Marks)
	}
	if len(missing) > 0 {
		return f, true, fmt.Errorf("parts missing or unreadable: %v", missing)
	}
	return f, true, nil
}

// diskState is what is on disk now, so a save can leave alone what has not
// changed. It belongs to writeSnapshot (under designDisk).
type diskState struct {
	inParts  bool
	parts    map[int]*dOct // each structure as last written
	marks    map[[2]int]dMark
	marksVer int // the ink's version last written
	index    []byte
}

// remember notes f as what is on disk (just read, or just written).
func (s *diskState) remember(f designFile, index []byte) {
	s.inParts = true
	s.parts = make(map[int]*dOct, len(f.Octs))
	for _, o := range copyOcts(f.Octs) {
		if o != nil {
			s.parts[o.ID] = o
		}
	}
	s.marks = marksOf(f.Marks)
	s.index = index
}

// marksOfFile is f's ink as a map.
func marksOfFile(f designFile) map[[2]int]dMark {
	if f.marksMap != nil {
		return f.marksMap
	}
	return marksOf(f.Marks)
}

func marksOf(ms []dMarkAt) map[[2]int]dMark {
	out := make(map[[2]int]dMark, len(ms))
	for _, m := range ms {
		out[[2]int{m.X, m.Y}] = m.dMark
	}
	return out
}

func indexOf(f designFile) designIndex {
	idx := designIndex{Version: designDiskVersion, Order: []int{}, Links: f.Links, CamX: f.CamX, CamY: f.CamY, Zoom: f.Zoom, NextID: f.NextID}
	if idx.Links == nil {
		idx.Links = []dLink{}
	}
	for _, o := range f.Octs {
		if o != nil {
			idx.Order = append(idx.Order, o.ID)
		}
	}
	return idx
}

// writeParts writes snapshot f in parts: the structures that changed, the
// ink if it changed, then the index, then removes the parts of structures
// that are gone. It returns the error, or "".
func (d *designState) writeParts(f designFile) string {
	s := &d.disk
	path := d.loadedFrom
	if !s.inParts {
		// The first save in parts: keep the whole board as it was, once.
		if data, err := os.ReadFile(path); err == nil {
			backup := filepath.Join(filepath.Dir(path), "board.v1.json")
			if _, err := os.Stat(backup); os.IsNotExist(err) {
				if err := writeBoardFile(backup, data); err != nil {
					return err.Error()
				}
			}
		}
		s.parts, s.marks = map[int]*dOct{}, nil
	}
	ids := map[int]bool{}
	for _, o := range f.Octs {
		if o == nil {
			continue
		}
		ids[o.ID] = true
		if old, ok := s.parts[o.ID]; ok && (old == o || sameSaved(old, o)) {
			continue
		}
		data, err := json.MarshalIndent(o, "", " ")
		if err == nil {
			err = writeBoardFile(partPath(path, o.ID), data)
		}
		if err != nil {
			return err.Error()
		}
		s.parts[o.ID] = o
	}
	if f.marksMap != nil && s.inParts && s.marksVer == f.marksVer && s.marks != nil {
		// The ink is the version already written.
	} else if marks := marksOfFile(f); s.marks == nil || !maps2Equal(marks, s.marks) {
		sorted := make([]dMarkAt, 0, len(marks))
		for k, v := range marks {
			sorted = append(sorted, dMarkAt{k[0], k[1], v})
		}
		slices.SortFunc(sorted, func(a, b dMarkAt) int {
			if a.Y != b.Y {
				return a.Y - b.Y
			}
			return a.X - b.X
		})
		if sorted == nil {
			sorted = []dMarkAt{}
		}
		data, err := json.Marshal(sorted)
		if err == nil {
			err = writeBoardFile(marksPath(path), data)
		}
		if err != nil {
			return err.Error()
		}
		s.marks = marks
		s.marksVer = f.marksVer
	} else {
		s.marksVer = f.marksVer
	}
	index, err := json.MarshalIndent(indexOf(f), "", " ")
	if err != nil {
		return err.Error()
	}
	if !s.inParts || !bytes.Equal(index, s.index) {
		if err := writeBoardFile(path, index); err != nil {
			return err.Error()
		}
		s.index, s.inParts = index, true
	}
	for id := range s.parts { // gone from the board, and from the index now
		if !ids[id] {
			if err := os.Remove(partPath(path, id)); err != nil && !os.IsNotExist(err) {
				return err.Error()
			}
			delete(s.parts, id)
		}
	}
	return ""
}

func maps2Equal(a, b map[[2]int]dMark) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if w, ok := b[k]; !ok || w != v {
			return false
		}
	}
	return true
}

// sameSaved is whether a and b would be written the same: every saved
// field, and what their drawers hold.
func sameSaved(a, b *dOct) bool {
	return sameOct(a, b) && a.Scroll == b.Scroll && a.DrawerOpen == b.DrawerOpen &&
		a.OpenStack == b.OpenStack && sameSavedItems(a.Items, b.Items)
}

func sameSavedItems(a, b []*dOct) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if (a[i] == nil) != (b[i] == nil) || (a[i] != nil && !sameSaved(a[i], b[i])) {
			return false
		}
	}
	return true
}
