package main

import (
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// The key mode is a word in the status bar that says what the arrows do
// right now, from the key held down:
//
//	nothing   text          typing and moving by word
//	Shift     highlighter   the arrows highlight
//	Ctrl      position      the arrows move the cursor one character
//	Alt       movement      the arrows move the highlighted text, or the word
//
// kitty reports Shift, Ctrl and Alt going down and up on their own (see
// altgr.go), so the word changes the moment the key is pressed. Other
// terminals only send them with another key, so there the word follows the
// last key pressed. AltGr (Right Alt) types letters, so it is not Alt here.

type keyMode struct {
	shift, ctrl, alt bool
}

func (k *keyMode) press(key tea.Key, altGr bool) {
	if isAltGr(key) {
		return
	}
	k.fromMods(key, altGr)
	switch key.Code {
	case tea.KeyLeftShift, tea.KeyRightShift:
		k.shift = true
	case tea.KeyLeftCtrl, tea.KeyRightCtrl:
		k.ctrl = true
	case tea.KeyLeftAlt:
		k.alt = true
	}
}

// release: kitty sends the keys still held with every key let go.
func (k *keyMode) release(key tea.Key, altGr bool) {
	if isAltGr(key) {
		return
	}
	k.fromMods(key, altGr)
	switch key.Code {
	case tea.KeyLeftShift, tea.KeyRightShift:
		k.shift = false
	case tea.KeyLeftCtrl, tea.KeyRightCtrl:
		k.ctrl = false
	case tea.KeyLeftAlt:
		k.alt = false
	}
}

func (k *keyMode) fromMods(key tea.Key, altGr bool) {
	k.shift = key.Mod.Contains(tea.ModShift)
	k.ctrl = key.Mod.Contains(tea.ModCtrl) && !altGr
	k.alt = key.Mod.Contains(tea.ModAlt) && !altGr
}

// name is the mode word. Alt wins, then Shift, then Ctrl: Alt+Shift+arrows
// move text, and Ctrl+Shift+arrows highlight.
func (k keyMode) name() string {
	switch {
	case k.alt:
		return "movement"
	case k.shift:
		return "highlighter"
	case k.ctrl:
		return "position"
	}
	return "text"
}

// keyModeSign draws the mode word for the status bar; short is the mark
// alone, for a narrow window.
func (m *Model) keyModeSign(short bool) string {
	name := m.keys.name()
	var mark string
	switch name {
	case "movement":
		mark = icon("\U000f0041", "✥")
	case "highlighter":
		mark = icon("\U000f0652", "▍")
	case "position":
		mark = icon("\U000f05e7", "⌶")
	default:
		mark = icon("\U000f03eb", "✎")
	}
	text := " " + mark + " " + name + " "
	if short {
		text = " " + mark + " "
	}
	if name == "text" {
		return lipgloss.NewStyle().Background(hex(cStatusBG)).Foreground(hex(cDim)).Render(text)
	}
	return lipgloss.NewStyle().Background(hex(cEmber)).Foreground(hex(cBG)).Bold(true).Render(text)
}
