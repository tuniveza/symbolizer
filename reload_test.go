package main

import (
	"os"
	"testing"
	"time"
)

// changeOnDisk writes text the way another program would, with a new time.
func changeOnDisk(t *testing.T, path, text string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
	later := time.Now().Add(2 * time.Second)
	os.Chtimes(path, later, later)
}

func TestReloadWhenChangedOnDisk(t *testing.T) {
	m := newWebTest(t, "a.go", "one\ntwo\nthree\nfour\n")
	m.buf.Cursor = Pos{3, 2} // in "four"
	changeOnDisk(t, m.buf.Path, "one\nTWO\n2b\nthree\nfour\n")
	m.Update(diskTick{})
	want(t, m, "one\nTWO\n2b\nthree\nfour", Pos{4, 2}) // the cursor moved down with its line
	if m.buf.Modified() {
		t.Fatalf("reloaded text should count as saved")
	}
	press(m, "ctrl+z") // takes the outside change back
	want(t, m, "one\ntwo\nthree\nfour", Pos{3, 2})

	// nothing changed: nothing happens
	m = newWebTest(t, "b.go", "x\n")
	v := m.buf.Version()
	m.Update(diskTick{})
	if m.buf.Version() != v {
		t.Fatalf("reloaded with no change")
	}
}

func TestReloadEdgeLines(t *testing.T) {
	for _, c := range []struct{ from, to string }{
		{"a\nb\nc", "a\nb"}, {"a\nb", "a\nb\nc"}, {"a\nb", "z\nb"}, {"a", ""}, {"", "a\nb"},
		{"a\nb\nc", "a\nc"}, {"a\nc", "a\nb\nc"}, {"a\nb\nc", "x"},
	} {
		b := NewBuffer()
		b.Insert(c.from, "")
		b.replaceLines(NewBufferFrom(c.to).Lines)
		if b.Text() != c.to {
			t.Fatalf("%q → %q gave %q", c.from, c.to, b.Text())
		}
	}
}

func NewBufferFrom(s string) *Buffer {
	b := NewBuffer()
	b.Insert(s, "")
	return b
}

func TestNoReloadOverUnsavedText(t *testing.T) {
	m := newWebTest(t, "a.go", "one\n")
	m.buf.Insert("mine ", "") // typed, not saved yet
	changeOnDisk(t, m.buf.Path, "theirs\n")
	m.Update(diskTick{})
	if m.buf.Text() != "onemine " {
		t.Fatalf("unsaved text was overwritten: %q", m.buf.Text())
	}
}

func TestReloadOtherTab(t *testing.T) {
	m := newWebTest(t, "a.go", "first\n")
	first := m.buf
	other := newWebTest(t, "b.go", "second\n")
	m.addTab(other.buf)
	changeOnDisk(t, first.Path, "first changed\n")
	m.Update(diskTick{})
	if first.Text() != "first changed" {
		t.Fatalf("the other tab was not reloaded: %q", first.Text())
	}
}
