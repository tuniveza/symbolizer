package main

import (
	"os"
	"os/exec"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
)

type pasteMsg string
type clipErrMsg string

// The system clipboard: Wayland, then X11, then macOS. Over SSH, or when none
// of these tools exist, copying uses the terminal itself (OSC 52), and
// pasting falls back to the last text copied inside Symbolizer.
func copyTool() []string {
	switch {
	case os.Getenv("SSH_TTY") != "":
		return nil
	case os.Getenv("WAYLAND_DISPLAY") != "" && have("wl-copy"):
		return []string{"wl-copy"}
	case os.Getenv("DISPLAY") != "" && have("xclip"):
		return []string{"xclip", "-selection", "clipboard", "-in"}
	case os.Getenv("DISPLAY") != "" && have("xsel"):
		return []string{"xsel", "--clipboard", "--input"}
	case have("pbcopy"):
		return []string{"pbcopy"}
	}
	return nil
}

func pasteTool() []string {
	switch {
	case os.Getenv("SSH_TTY") != "":
		return nil
	case os.Getenv("WAYLAND_DISPLAY") != "" && have("wl-paste"):
		return []string{"wl-paste", "--no-newline"}
	case os.Getenv("DISPLAY") != "" && have("xclip"):
		return []string{"xclip", "-selection", "clipboard", "-out"}
	case os.Getenv("DISPLAY") != "" && have("xsel"):
		return []string{"xsel", "--clipboard", "--output"}
	case have("pbpaste"):
		return []string{"pbpaste"}
	}
	return nil
}

func have(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

func writeClipboard(text string) tea.Cmd {
	tool := copyTool()
	if tool == nil {
		return tea.SetClipboard(text)
	}
	return func() tea.Msg {
		c := exec.Command(tool[0], tool[1:]...)
		c.Stdin = strings.NewReader(text)
		if err := runWithin(c, 2*time.Second); err != nil {
			return clipErrMsg("Copied inside Symbolizer only. The system clipboard did not answer.")
		}
		return nil
	}
}

func readClipboard(fallback string) tea.Cmd {
	tool := pasteTool()
	if tool == nil {
		return func() tea.Msg { return pasteMsg(fallback) }
	}
	return func() tea.Msg {
		c := exec.Command(tool[0], tool[1:]...)
		out := &strings.Builder{}
		c.Stdout = out
		if err := runWithin(c, 2*time.Second); err != nil {
			return pasteMsg(fallback)
		}
		return pasteMsg(out.String())
	}
}

// runWithin runs a command but gives up after d, so a stuck clipboard tool
// can never freeze the editor.
func runWithin(c *exec.Cmd, d time.Duration) error {
	if err := c.Start(); err != nil {
		return err
	}
	done := make(chan error, 1)
	go func() { done <- c.Wait() }()
	select {
	case err := <-done:
		return err
	case <-time.After(d):
		c.Process.Kill()
		return exec.ErrWaitDelay
	}
}
