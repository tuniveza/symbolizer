package main

import (
	"encoding/json"
	"regexp"
	"strings"
)

// Guessing the language from the text itself, for files with no name yet.
// Each language has telltale patterns with points; the most points wins.
// Used for the colours while typing, and for the file ending (.cpp, .c, .py
// ...) when Ctrl+S asks for a name.

type language struct {
	Name string // Chroma's name for it
	Ext  string
	cues []cue
}

type cue struct {
	rx     *regexp.Regexp
	points int
}

func cues(pairs ...any) []cue {
	var out []cue
	for i := 0; i < len(pairs); i += 2 {
		out = append(out, cue{regexp.MustCompile("(?m)" + pairs[i].(string)), pairs[i+1].(int)})
	}
	return out
}

var languages = []language{
	{"C++", ".cpp", cues(
		`#include\s*<(iostream|vector|string|map|unordered_map|memory|algorithm|fstream|sstream|array)>`, 5,
		`\bstd::`, 4, `\b(cout|cerr)\s*<<|\bcin\s*>>`, 4, `\busing\s+namespace\s+std\b`, 5,
		`\btemplate\s*<`, 4, `\bnullptr\b`, 3, `\bnamespace\s+\w+\s*\{`, 3,
		`\bclass\s+\w+\s*(:\s*(public|private)\s+\w+\s*)?\{`, 2, `#include\s*[<"]`, 1, `\bint\s+main\s*\(`, 1)},
	{"C", ".c", cues(
		`#include\s*<(stdio|stdlib|string|math|unistd|stdint|stdbool|ctype|time|errno)\.h>`, 5,
		`\bprintf\s*\(`, 2, `\b(malloc|calloc|free)\s*\(`, 2, `\bint\s+main\s*\(`, 2,
		`#include\s*[<"]`, 1, `\bstruct\s+\w+\s*\{`, 1, `^#define\s`, 1, `\btypedef\b`, 1)},
	{"Go", ".go", cues(
		`^package\s+\w+\s*$`, 6, `^import\s+(\(|")`, 3, `\bfunc\s+(\([^)]*\)\s*)?\w+\s*\(`, 3,
		`\bfmt\.\w+`, 4, `:=`, 1, `\bgo\s+func\b|\bchan\b|\bdefer\b`, 3)},
	{"Rust", ".rs", cues(
		`\bfn\s+\w+\s*[(<]`, 3, `\blet\s+mut\b`, 4, `\b(println|print|format|vec|panic)!\(`, 4,
		`^\s*use\s+(std|crate)::`, 4, `\bimpl\b`, 2, `\bpub\s+(fn|struct|enum)\b`, 3, `&mut\b|&self\b`, 3)},
	{"Python", ".py", cues(
		`^\s*def\s+\w+\s*\(.*\)\s*(->.*)?:\s*$`, 5, `^\s*(from\s+[\w.]+\s+import\s|import\s+[\w.]+(\s+as\s+\w+)?\s*$)`, 3,
		`^\s*class\s+\w+(\(.*\))?:\s*$`, 4, `^\s*(if|elif|else|for|while|with|try|except|finally)\b.*:\s*$`, 2,
		`\bself\.\w+`, 3, `__name__|__init__`, 4, `\bprint\s*\(`, 1, `\b(True|False|None)\b`, 1, `^\s*#`, 1)},
	{"JavaScript", ".js", cues(
		`\bconsole\.(log|error|warn)\s*\(`, 3, `\b(const|let|var)\s+\w+\s*=`, 1, `\bfunction\s*\w*\s*\(`, 2,
		`=>`, 1, `\b(document|window)\.\w+`, 3, `\brequire\s*\(|\bmodule\.exports\b`, 3,
		`^\s*import\s+.+\s+from\s+['"]`, 3, `===|!==`, 2)},
	{"TypeScript", ".ts", cues(
		`\w\s*:\s*(string|number|boolean|any|void|unknown|never)\b`, 4, `^\s*(export\s+)?interface\s+\w+`, 4,
		`^\s*(export\s+)?type\s+\w+\s*=`, 4, `\bconsole\.log\s*\(`, 1, `\b(const|let)\s+\w+\s*:\s*\w+`, 3)},
	{"Java", ".java", cues(
		`\bpublic\s+static\s+void\s+main\b`, 6, `\bSystem\.out\.print`, 5, `^\s*import\s+java\.`, 6,
		`\bpublic\s+(final\s+)?class\b`, 4, `^package\s+[\w.]+;`, 4, `@Override\b`, 3)},
	{"C#", ".cs", cues(
		`^\s*using\s+System`, 6, `\bConsole\.Write(Line)?\s*\(`, 5, `\bnamespace\s+[\w.]+`, 1,
		`\bstatic\s+void\s+Main\b`, 5)},
	{"Kotlin", ".kt", cues(`\bfun\s+main\s*\(`, 5, `\bval\s+\w+\s*=`, 2, `\bfun\s+\w+\s*\(`, 2, `\bprintln\(`, 1)},
	{"Bash", ".sh", cues(
		`^\s*if\s+\[|^\s*fi\s*$|^\s*then\s*$|^\s*done\s*$|^\s*esac\s*$`, 4, `^\s*echo\s`, 2,
		`\$\(`, 2, `\$\{?\w+\}?`, 1, `^\s*export\s+\w+=`, 3, `^\s*\w+=\S`, 1)},
	{"Lua", ".lua", cues(
		`^\s*local\s+\w+`, 3, `^\s*end\s*[,)]?\s*$`, 3, `\bthen\s*$`, 2, `~=`, 2,
		`\bfunction\s+[\w.:]+\s*\(`, 2, `\bpairs\(|\bipairs\(`, 3, `\.\.`, 1)},
	{"PHP", ".php", cues(`<\?php`, 12, `\$\w+\s*=`, 1, `\becho\s`, 1)},
	{"HTML", ".html", cues(
		`(?i)<!DOCTYPE\s+html`, 8, `(?i)<html\b`, 6, `(?i)</(div|p|span|body|head|ul|li|a|section)>`, 3)},
	{"CSS", ".css", cues(
		`^\s*[.#]?[\w-]+([\s,>+~]+[.#:]?[\w-]+)*\s*\{\s*$`, 2, `^\s*[\w-]+\s*:\s*[^;{}]+;\s*$`, 2,
		`@media\b|@import\b|@keyframes\b`, 4, `\b\d+(px|em|rem|vh|vw)\b`, 3, `#[0-9a-fA-F]{3,6}\b`, 1)},
	{"SQL", ".sql", cues(`(?i)^\s*(select\s.+\sfrom|insert\s+into|create\s+table|update\s+\w+\s+set|delete\s+from)\b`, 6)},
	{"Markdown", ".md", cues(
		"^#{1,6}\\s+\\S", 3, "^```", 3, `\[[^\]]+\]\([^)]+\)`, 2, `^\s*[-*]\s+\S`, 1, `\*\*[^*]+\*\*`, 2)},
	{"YAML", ".yaml", cues(`^---\s*$`, 2, `^[\w-]+:\s*$`, 1, `^\s+-\s+\w`, 1, `^[\w-]+:\s+\S`, 1)},
}

// guessLanguage returns Chroma's name for the language of text and the file
// ending to use. Plain text gives ("Text", ".txt").
func guessLanguage(text string) (string, string) {
	if len(text) > 32<<10 {
		text = text[:32<<10]
	}
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return "Text", ".txt"
	}
	if strings.HasPrefix(trimmed, "#!") {
		first := strings.SplitN(trimmed, "\n", 2)[0]
		switch {
		case strings.Contains(first, "python"):
			return "Python", ".py"
		case strings.Contains(first, "node"):
			return "JavaScript", ".js"
		case strings.Contains(first, "lua"):
			return "Lua", ".lua"
		case strings.Contains(first, "sh"):
			return "Bash", ".sh"
		}
	}
	if (trimmed[0] == '{' || trimmed[0] == '[') && json.Valid([]byte(trimmed)) {
		return "JSON", ".json"
	}
	best, bestScore := -1, 0
	for i, l := range languages {
		score := 0
		for _, c := range l.cues {
			if c.rx.MatchString(text) {
				score += c.points
			}
		}
		if score > bestScore {
			best, bestScore = i, score
		}
	}
	if best < 0 || bestScore < 3 {
		return "Text", ".txt"
	}
	return languages[best].Name, languages[best].Ext
}
