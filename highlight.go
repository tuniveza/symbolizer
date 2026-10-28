package main

import (
	"path/filepath"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/lexers"
)

// span colours letters From..To (not including To) on one line.
type span struct {
	From, To int
	Style    uint8
}

// Highlighter colours the text with Chroma (the library Charm's own tools
// use, 250+ languages). The work runs in the background after each change,
// so typing never waits for it.
//
// Small files are coloured whole. In files over wholeLimit lines only the
// part on screen is coloured, starting a few hundred lines above it, so a
// huge file costs the same as a small one.
type Highlighter struct {
	lexer   chroma.Lexer
	Name    string
	lines   [][]span // colours of lines base, base+1, ...; nil = not known yet
	base    int
	whole   bool // lines covers the whole file
	running bool
	done    int // buffer version the colours match
}

type highlightMsg struct {
	version int
	base    int
	whole   bool
	lines   [][]span
}

type highlightTick struct{}

const (
	wholeLimit = 3000 // lines; bigger files are coloured around the screen
	lookBehind = 500  // lines coloured above the screen, so comments and strings that started earlier still show right
	lookAhead  = 200
	textLimit  = 64 << 20 // files over 64 MB are not coloured
)

func NewHighlighter(path, text string) *Highlighter {
	h := &Highlighter{done: -1}
	var lx chroma.Lexer
	if path != "" {
		lx = lexers.Match(filepath.Base(path))
		if lx == nil {
			// kitty.conf.bak-20261017 and the like: try without the backup part.
			base := filepath.Base(path)
			if i := strings.Index(base, ".bak"); i > 0 {
				lx = lexers.Match(base[:i])
			}
		}
	}
	if lx == nil && text != "" {
		// No name to go by: guess from what the text looks like.
		if name, _ := guessLanguage(text); name != "Text" {
			lx = lexers.Get(name)
		}
	}
	if lx == nil {
		lx = lexers.Fallback
	}
	h.lexer = chroma.Coalesce(lx)
	h.Name = lx.Config().Name
	if h.Name == "fallback" || h.Name == "plaintext" {
		h.Name = "Text"
	}
	if r := []rune(h.Name); len(r) > 0 {
		h.Name = strings.ToUpper(string(r[0])) + string(r[1:])
	}
	return h
}

// Splice moves the known colours along when lines are added or removed.
// A line that only changed keeps its old colours until the new ones arrive,
// so the line being typed on never flashes plain.
func (h *Highlighter) Splice(row, removed, added int) {
	if removed == added || h.lines == nil {
		return
	}
	i := row - h.base
	switch {
	case i+removed <= 0: // above the coloured part
		h.base += added - removed
	case i < 0: // overlaps its top: start again
		h.lines, h.whole = nil, false
	case i >= len(h.lines):
	default:
		end := min(i+removed, len(h.lines))
		fresh := make([][]span, added)
		h.lines = append(h.lines[:i], append(fresh, h.lines[end:]...)...)
	}
}

func (h *Highlighter) Line(row int) []span {
	if i := row - h.base; i >= 0 && i < len(h.lines) {
		return h.lines[i]
	}
	return nil
}

// covers says whether the coloured part holds every line on screen.
func (h *Highlighter) covers(top, height int) bool {
	return h.whole || (top >= h.base && top+height <= h.base+len(h.lines))
}

// Want asks for new colours when the text changed or the screen moved past
// the coloured part. It waits a moment so a burst of typing becomes one run.
func (h *Highlighter) Want(version, top, height int) tea.Cmd {
	if h.running || h.Name == "Text" || (h.done == version && h.covers(top, height)) {
		return nil
	}
	h.running = true
	return tea.Tick(12*time.Millisecond, func(time.Time) tea.Msg { return highlightTick{} })
}

// Start copies the lines to colour and colours them in the background.
func (h *Highlighter) Start(b *Buffer, top, height int) tea.Cmd {
	n := len(b.Lines)
	from, to, whole := 0, n, n <= wholeLimit
	if !whole {
		from = max(0, top-lookBehind)
		// Begin at a line with no indent: usually outside any comment or string.
		for i := from; i > max(0, from-2000); i-- {
			if l := b.Lines[i]; len(l) > 0 && l[0] != ' ' && l[0] != '\t' {
				from = i
				break
			}
		}
		to = min(n, top+height+lookAhead)
	}
	var sb strings.Builder
	for r := from; r < to; r++ {
		if r > from {
			sb.WriteByte('\n')
		}
		sb.WriteString(string(b.Lines[r]))
		if sb.Len() > textLimit {
			h.running, h.lines, h.done = false, nil, b.Version()
			return nil
		}
	}
	text, lexer, version := sb.String(), h.lexer, b.Version()
	return func() tea.Msg {
		return highlightMsg{version: version, base: from, whole: whole, lines: tokenise(lexer, text)}
	}
}

// Finish stores a finished run.
func (h *Highlighter) Finish(msg highlightMsg) {
	h.running = false
	h.lines, h.base, h.whole, h.done = msg.lines, msg.base, msg.whole, msg.version
}

func tokenise(lexer chroma.Lexer, text string) [][]span {
	lines := make([][]span, 1, strings.Count(text, "\n")+1)
	it, err := lexer.Tokenise(nil, text+"\n")
	if err != nil {
		return nil
	}
	row, col := 0, 0
	for t := it(); t != chroma.EOF; t = it() {
		style := styleFor(t.Type)
		v := t.Value
		for {
			nl := strings.IndexByte(v, '\n')
			part := v
			if nl >= 0 {
				part = v[:nl]
			}
			n := len([]rune(part))
			if n > 0 && style != sNormal {
				lines[row] = append(lines[row], span{col, col + n, style})
			}
			col += n
			if nl < 0 {
				break
			}
			v = v[nl+1:]
			lines = append(lines, nil)
			row++
			col = 0
		}
	}
	return lines
}

var tokenStyles = map[chroma.TokenType]uint8{
	chroma.Keyword:               sKeyword,
	chroma.KeywordType:           sType,
	chroma.KeywordConstant:       sConstant,
	chroma.KeywordNamespace:      sImport,
	chroma.KeywordPseudo:         sBuiltin,
	chroma.Name:                  sNormal,
	chroma.NameFunction:          sFunction,
	chroma.NameFunctionMagic:     sFunction,
	chroma.NameClass:             sType,
	chroma.NameException:         sType,
	chroma.NameBuiltin:           sBuiltin,
	chroma.NameBuiltinPseudo:     sBuiltin,
	chroma.NameDecorator:         sAttribute,
	chroma.NameAttribute:         sAttribute,
	chroma.NameTag:               sTag,
	chroma.NameVariable:          sVariable,
	chroma.NameConstant:          sConstant,
	chroma.NameNamespace:         sImport,
	chroma.NameEntity:            sEscape,
	chroma.NameLabel:             sAttribute,
	chroma.NameProperty:          sKey,
	chroma.NameOther:             sNormal,
	chroma.LiteralString:         sString,
	chroma.LiteralStringEscape:   sEscape,
	chroma.LiteralStringInterpol: sEscape,
	chroma.LiteralStringAffix:    sKeyword,
	chroma.LiteralStringDoc:      sComment,
	chroma.LiteralStringRegex:    sEscape,
	chroma.LiteralNumber:         sNumber,
	chroma.Literal:               sString,
	chroma.LiteralDate:           sNumber,
	chroma.Operator:              sOperator,
	chroma.OperatorWord:          sKeyword,
	chroma.Punctuation:           sNormal,
	chroma.Comment:               sComment,
	chroma.CommentPreproc:        sPreproc,
	chroma.CommentPreprocFile:    sString,
	chroma.CommentSpecial:        sSpecial,
	chroma.GenericHeading:        sHeading,
	chroma.GenericSubheading:     sHeading,
	chroma.GenericEmph:           sEmphasis,
	chroma.GenericStrong:         sStrong,
	chroma.GenericInserted:       sInserted,
	chroma.GenericDeleted:        sError,
	chroma.GenericError:          sError,
	chroma.GenericPrompt:         sKeyword,
	chroma.GenericUnderline:      sEmphasis,
	chroma.Error:                 sNormal,
	chroma.Text:                  sNormal,
}

// styleFor finds the closest known style: the exact token type, then its
// sub-group, then its main group.
func styleFor(t chroma.TokenType) uint8 {
	if s, ok := tokenStyles[t]; ok {
		return s
	}
	if s, ok := tokenStyles[t.SubCategory()]; ok {
		return s
	}
	if s, ok := tokenStyles[t.Category()]; ok {
		return s
	}
	return sNormal
}

// commentToken is what Ctrl+/ puts in front of a line for each language.
// Languages not listed use //.
var commentTokens = map[string]string{}

func init() {
	for _, n := range strings.Split("python|python 2|bash|bash session|fish|zsh|yaml|toml|ruby|"+
		"perl|makefile|cmake|docker|nim|r|elixir|powershell|nix|nginx configuration nginx|"+
		"systemd|hcl|terraform|awk|crystal|julia|tcl|coffeescript|gdscript|properties|"+
		"apacheconf|hyprlang|desktop|meson|gnuplot", "|") {
		commentTokens[n] = "#"
	}
	for _, n := range strings.Split("lua|sql|mysql|postgresql sql dialect|plpgsql|haskell|elm|ada|vhdl", "|") {
		commentTokens[n] = "--"
	}
	for _, n := range strings.Split("ini|common lisp|emacslisp|clojure|scheme|racket|nasm|fennel", "|") {
		commentTokens[n] = ";"
	}
	for _, n := range strings.Split("tex|erlang|matlab|prolog", "|") {
		commentTokens[n] = "%"
	}
	for _, n := range strings.Split("text|markdown|html|xml|json|css|scss|svg|plaintext", "|") {
		commentTokens[n] = ""
	}
	commentTokens["vim"] = "\""
}

func commentToken(lang string) string {
	if t, ok := commentTokens[strings.ToLower(lang)]; ok {
		return t
	}
	return "//"
}
