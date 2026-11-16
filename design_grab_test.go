package main

import (
	"testing"

	tea "charm.land/bubbletea/v2"
)

// Alt+Space is named "alt+space" (the grab key).
func TestGrabKeyName(t *testing.T) {
	if n := keyName(tea.Key{Code: tea.KeySpace, Mod: tea.ModAlt}); !isGrabKey(n) {
		t.Fatalf("Alt+Space is named %q", n)
	}
	if !isGrabKey("alt+ ") {
		t.Fatal(`"alt+ " should count as Alt+Space too`)
	}
}
