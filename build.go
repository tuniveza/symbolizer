package main

import (
	"os"
	"runtime/debug"
)

// buildNumber names the build that is running, for the status bar and
// sy --version: the commit it was built from (Go stamps it in when sy is
// built in its git folder). Built with changes not yet committed, the
// commit gets a + and the time the program file was made, so each new
// build shows a new number. Empty when Go stamped nothing (go test).
var buildNumber = func() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return ""
	}
	rev, modified := "", false
	for _, s := range info.Settings {
		switch s.Key {
		case "vcs.revision":
			rev = s.Value
		case "vcs.modified":
			modified = s.Value == "true"
		}
	}
	if rev == "" {
		return ""
	}
	n := "build " + rev[:min(7, len(rev))]
	if modified {
		n += "+"
		if exe, err := os.Executable(); err == nil {
			if st, err := os.Stat(exe); err == nil {
				n += " " + st.ModTime().Format("2 Jan 15:04")
			}
		}
	}
	return n
}()
