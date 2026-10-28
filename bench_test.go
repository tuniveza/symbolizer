package main

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func bigModel(b *testing.B) *Model {
	m, _ := NewModel("", 0)
	m.Update(tea.WindowSizeMsg{Width: 200, Height: 60})
	var sb strings.Builder
	for i := 0; i < 200000; i++ {
		sb.WriteString("func handler(w http.ResponseWriter, r *http.Request) { fmt.Println(\"value\", 42) } // note\n")
	}
	m.buf.Insert(sb.String(), "")
	m.buf.Cursor = Pos{100000, 10}
	m.ensureVisible()
	m.hl = NewHighlighter("x.go", "")
	m.buf.OnSplice = m.hl.Splice
	return m
}

func BenchmarkKeystroke(b *testing.B) {
	m := bigModel(b)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m.Update(tea.KeyPressMsg(tea.Key{Code: 'a', Text: "a"}))
		_ = m.View()
	}
}

func BenchmarkHighlightAroundScreen(b *testing.B) {
	m := bigModel(b)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m.hl.running = true
		cmd := m.hl.Start(m.buf, m.top, m.textHeight())
		m.hl.Finish(cmd().(highlightMsg))
	}
	if m.hl.Line(m.top) == nil {
		b.Fatal("screen not coloured")
	}
}

// What the screen's own thread spends per autosave: copying the line list.
func BenchmarkAutosaveScreenCost(b *testing.B) {
	m := bigModel(b)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = m.buf.Snapshot()
	}
}
