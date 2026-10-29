package main

import (
	"os"
	"path/filepath"
	"strings"
)

// ---- remembered settings (~/.config/symbolizer/settings) --------------------------

func settingsPath() string {
	base := os.Getenv("XDG_CONFIG_HOME")
	if base == "" {
		home, _ := os.UserHomeDir()
		base = filepath.Join(home, ".config")
	}
	return filepath.Join(base, "symbolizer", "settings")
}

func loadSettings() map[string]string {
	out := map[string]string{}
	data, err := os.ReadFile(settingsPath())
	if err != nil {
		return out
	}
	for _, line := range strings.Split(string(data), "\n") {
		if k, v, ok := strings.Cut(line, "="); ok {
			out[strings.TrimSpace(k)] = strings.TrimSpace(v)
		}
	}
	return out
}

func saveSetting(key, value string) {
	s := loadSettings()
	s[key] = value
	var sb strings.Builder
	sb.WriteString("# Symbolizer settings (written by sy)\n")
	for k, v := range s {
		sb.WriteString(k + " = " + v + "\n")
	}
	os.MkdirAll(filepath.Dir(settingsPath()), 0o755)
	os.WriteFile(settingsPath(), []byte(sb.String()), 0o644)
}
