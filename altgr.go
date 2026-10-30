package main

import (
	"time"

	tea "charm.land/bubbletea/v2"
)

// Tapping AltGr twice, on its own, opens the file manager (or closes it),
// with or without a file open. Terminals normally send nothing for a
// modifier key on its own; kitty does once it is asked to report every key
// (View: ReportAllKeysAsEscapeCodes, with ReportAssociatedText so typing
// still carries its letters). A tap is AltGr going down and up with no
// other key in between, so AltGr+S (ß) or AltGr+O (ø) never count.

const (
	tapMax    = 400 * time.Millisecond // a tap is shorter than this
	doubleGap = 450 * time.Millisecond // the second tap must start within this
)

type altGrState struct {
	down    bool
	clean   bool // no other key while it was held
	pressed time.Time
	lastTap time.Time
}

func isAltGr(k tea.Key) bool {
	return k.Code == tea.KeyIsoLevel3Shift || k.Code == tea.KeyRightAlt
}

// isModifierOnly is a modifier key pressed or let go on its own.
func isModifierOnly(k tea.Key) bool {
	switch k.Code {
	case tea.KeyLeftShift, tea.KeyRightShift, tea.KeyLeftCtrl, tea.KeyRightCtrl,
		tea.KeyLeftAlt, tea.KeyRightAlt, tea.KeyLeftSuper, tea.KeyRightSuper,
		tea.KeyLeftHyper, tea.KeyRightHyper, tea.KeyLeftMeta, tea.KeyRightMeta,
		tea.KeyIsoLevel3Shift, tea.KeyIsoLevel5Shift:
		return true
	}
	return false
}

// altGrName is keyName, except that C, M or T with AltGr held is "altgr+c",
// "altgr+m" or "altgr+t" (kitty says when AltGr is held; the key itself
// comes with no modifier).
func (m *Model) altGrName(k tea.Key) string {
	if m.altGr.down && k.Mod == 0 {
		switch k.Code {
		case 'c', 'm', 't':
			return "altgr+" + string(k.Code)
		}
	}
	return keyName(k)
}

// altGrPress watches every key press. It returns true when the key was
// AltGr itself (nothing else should happen for it).
func (m *Model) altGrPress(k tea.Key) bool {
	a := &m.altGr
	if isAltGr(k) {
		if !k.IsRepeat && !a.down {
			a.down, a.clean, a.pressed = true, true, time.Now()
		}
		return true
	}
	if a.down {
		a.clean = false // AltGr is being used to type something (ß, ø, @ ...)
	}
	if !isModifierOnly(k) {
		a.lastTap = time.Time{} // any other key breaks a double tap
	}
	return false
}

// altGrRelease watches key releases. It returns a command when AltGr was
// tapped twice.
func (m *Model) altGrRelease(k tea.Key) (tea.Cmd, bool) {
	a := &m.altGr
	if !isAltGr(k) || !a.down {
		return nil, false
	}
	a.down = false
	now := time.Now()
	if !a.clean || now.Sub(a.pressed) > tapMax {
		a.lastTap = time.Time{}
		return nil, true
	}
	if !a.lastTap.IsZero() && a.pressed.Sub(a.lastTap) < doubleGap {
		a.lastTap = time.Time{}
		return m.altGrDouble(), true
	}
	a.lastTap = now
	return nil, true
}

// altGrDouble opens the file manager and gives it the keys, or closes it.
func (m *Model) altGrDouble() tea.Cmd {
	if m.showTree {
		m.closeTree()
		return nil
	}
	m.help, m.picker, m.modesOpen = false, nil, false
	if m.themeOpen {
		m.themeKey("esc")
	}
	m.closePrompt()
	return m.openTree(false)
}
