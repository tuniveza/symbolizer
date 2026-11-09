package main

// Every line of every card fits on screens from 80 × 24 up: the list gives way.
import (
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"testing"
)

func TestLearnFitsSmallScreens(t *testing.T) {
	worst := 0
	for _, h := range []int{24, 30, 40} {
		m := newWebTest(t, "a.css", "x\n")
		m.Update(tea.WindowSizeMsg{Width: 80, Height: h})
		pressLearn(m)
		for ti := range learnTopics {
			for si, s := range learnTopics[ti].Sections {
				for ii := range s.Items {
					m.learn.topic, m.learn.sec, m.learn.item = ti, si, ii
					if bh := lipgloss.Height(m.learnBox()); bh > h-2 {
						t.Errorf("h=%d %s/%s/%d: box %d rows", h, learnTopics[ti].Name, s.Title, ii, bh)
					} else if bh > worst {
						worst = bh
					}
				}
			}
		}
	}

}
