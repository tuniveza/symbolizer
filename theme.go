package main

import (
	"fmt"
	"image/color"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// Colours. Emberistal comes from
// ~/.local/share/org.kde.syntax-highlighting/themes/Emberistal.theme and
// ~/.local/share/konsole/Emberistal.colorscheme, so Symbolizer matches Kate,
// Konsole, kitty and Android Studio. The other themes are ember variants of
// it, chosen with ŧ (themes.go); they change Symbolizer only. Bubble Tea
// lowers colours to 256 or 16 by itself on terminals that need it.

type palette struct {
	Name, What                                             string
	BG, Text, CurrentLine, LineNumber, CurrentNum          string
	Selection, SelectedText, Search, SearchNow, Bracket    string
	GutterLine, BarBG, StatusBG, StatusText, Accent, Ember string
	Dim, Error, Warning, Glow                              string
	Gradient                                               [3]string
	Keyword, Function, Variable, Operator, Type, String    string
	Number, Comment, Escape, Heading, Emphasis, Strong     string
}

var themes = []palette{
	{
		Name: "Emberistal", What: "the original: embers on charcoal",
		BG: "#161616", Text: "#ff4518", CurrentLine: "#24120b", LineNumber: "#8e3c1a", CurrentNum: "#ff4518",
		Selection: "#5a2214", SelectedText: "#fff0d6", Search: "#5a2c12", SearchNow: "#8a3a12", Bracket: "#6a3010",
		GutterLine: "#2e150c", BarBG: "#1b1614", StatusBG: "#2a1610", StatusText: "#ffa66b", Accent: "#ffb400", Ember: "#ff6600",
		Dim: "#8e3c1a", Error: "#ff1f4b", Warning: "#ffc14d", Glow: "#fff0d6",
		Gradient: [3]string{"#ff3704", "#ff6600", "#ffb400"},
		Keyword:  "#ff3704", Function: "#ff6600", Variable: "#ff5125", Operator: "#d84017", Type: "#d74a00", String: "#ff8a1a",
		Number: "#ffb400", Comment: "#983818", Escape: "#ffb400", Heading: "#ff6600", Emphasis: "#ffb870", Strong: "#ff9a3c",
	},
	{
		Name: "Emberline", What: "the desktop's colours: a little lighter charcoal",
		BG: "#1c1c1c", Text: "#ff4518", CurrentLine: "#2a1610", LineNumber: "#8a3020", CurrentNum: "#ff6600",
		Selection: "#5a2214", SelectedText: "#fff0d6", Search: "#5a2c12", SearchNow: "#8a3a12", Bracket: "#6a3010",
		GutterLine: "#2e1a14", BarBG: "#222020", StatusBG: "#2a1610", StatusText: "#ffa66b", Accent: "#ffb400", Ember: "#ff6600",
		Dim: "#8a3020", Error: "#ff1f4b", Warning: "#ffc14d", Glow: "#fff0d6",
		Gradient: [3]string{"#c4552c", "#ff4518", "#ff6600"},
		Keyword:  "#ff6600", Function: "#ff4518", Variable: "#ff5a2a", Operator: "#c4552c", Type: "#e85a10", String: "#ff8a3c",
		Number: "#ffb400", Comment: "#8a3020", Escape: "#ffb400", Heading: "#ff6600", Emphasis: "#ffb870", Strong: "#ff9a3c",
	},
	{
		Name: "Molten", What: "hotter: bright orange and gold on dark rust",
		BG: "#1a0c07", Text: "#ff7a1a", CurrentLine: "#2b140a", LineNumber: "#a04a1c", CurrentNum: "#ffb400",
		Selection: "#6e2a10", SelectedText: "#fff4dc", Search: "#5a2a0e", SearchNow: "#8e4210", Bracket: "#7a3410",
		GutterLine: "#33180c", BarBG: "#200f08", StatusBG: "#34170b", StatusText: "#ffc07a", Accent: "#ffd23c", Ember: "#ff8a1a",
		Dim: "#9a4a22", Error: "#ff3a4b", Warning: "#ffd23c", Glow: "#fff4dc",
		Gradient: [3]string{"#ff6a00", "#ffa400", "#ffe08a"},
		Keyword:  "#ffcf3c", Function: "#ff9a2a", Variable: "#ff8a3a", Operator: "#e8661a", Type: "#ffb340", String: "#ffe08a",
		Number: "#fff0a0", Comment: "#a8582a", Escape: "#fff0a0", Heading: "#ffb340", Emphasis: "#ffe0a0", Strong: "#ffc070",
	},
	{
		Name: "Ash", What: "cooled embers: soft greys with a warm glow",
		BG: "#1b1a19", Text: "#d8b4a0", CurrentLine: "#24211f", LineNumber: "#6e625c", CurrentNum: "#ff7a4a",
		Selection: "#3e3430", SelectedText: "#fff0e6", Search: "#4a3a30", SearchNow: "#6a4a36", Bracket: "#4e3c34",
		GutterLine: "#2c2826", BarBG: "#201e1d", StatusBG: "#2a2624", StatusText: "#c9a898", Accent: "#ff9a5a", Ember: "#ff7a4a",
		Dim: "#7a6c64", Error: "#ff4a5a", Warning: "#ffb070", Glow: "#fff0e6",
		Gradient: [3]string{"#b06a50", "#ff7a4a", "#ffb08a"},
		Keyword:  "#ff7a4a", Function: "#e8a080", Variable: "#d8b4a0", Operator: "#a88878", Type: "#f0a070", String: "#c8b090",
		Number: "#ffb08a", Comment: "#6e625c", Escape: "#ffb08a", Heading: "#ff9a5a", Emphasis: "#e8c8b8", Strong: "#f0b090",
	},
	{
		Name: "Cinder", What: "deep red coals",
		BG: "#120808", Text: "#e33a2a", CurrentLine: "#1f0c0c", LineNumber: "#7a2420", CurrentNum: "#ff5a40",
		Selection: "#4a1414", SelectedText: "#ffe6dc", Search: "#4a1a14", SearchNow: "#6e2418", Bracket: "#5a1a14",
		GutterLine: "#261010", BarBG: "#170a0a", StatusBG: "#241010", StatusText: "#ff8a70", Accent: "#ff6a3a", Ember: "#ff3a2a",
		Dim: "#7a2a24", Error: "#ff2a6a", Warning: "#ff9a4a", Glow: "#ffe6dc",
		Gradient: [3]string{"#a01010", "#ff2a2a", "#ff8a5a"},
		Keyword:  "#ff2a4a", Function: "#ff5a3a", Variable: "#e8483a", Operator: "#c0302a", Type: "#ff6a4a", String: "#ff8a5a",
		Number: "#ffaa6a", Comment: "#7a2a24", Escape: "#ffaa6a", Heading: "#ff5a3a", Emphasis: "#ffb0a0", Strong: "#ff7a5a",
	},
	{
		Name: "Daybreak", What: "light: embers on warm paper, for daylight",
		BG: "#fff4e6", Text: "#9a2a08", CurrentLine: "#ffe8d0", LineNumber: "#c08a6a", CurrentNum: "#c0300a",
		Selection: "#ffcfa0", SelectedText: "#4a1404", Search: "#ffd8a8", SearchNow: "#ffb070", Bracket: "#ffc890",
		GutterLine: "#f0d8c0", BarBG: "#f6e2cc", StatusBG: "#f0d2b2", StatusText: "#8a3a10", Accent: "#b05a00", Ember: "#d84a00",
		Dim: "#b08060", Error: "#c8102e", Warning: "#a06a00", Glow: "#4a1404",
		Gradient: [3]string{"#c0300a", "#e05a00", "#c88000"},
		Keyword:  "#c0300a", Function: "#d04a00", Variable: "#a03a10", Operator: "#b05a30", Type: "#b04400", String: "#8a5a00",
		Number: "#a06a00", Comment: "#b08a70", Escape: "#a06a00", Heading: "#d04a00", Emphasis: "#8a4a20", Strong: "#a04010",
	},
}

// The colours in use. applyTheme sets them.
var (
	cBG, cText, cCurrentLine, cLineNumber, cCurrentNum           string
	cSelection, cSelectedText, cSearch, cSearchNow, cBracket     string
	cGutterLine, cBarBG, cStatusBG, cStatusText, cAccent, cEmber string
	cDim, cError, cWarning, cGlow                                string
	cGradient                                                    [3]string
	currentTheme                                                 int
)

func hex(s string) color.Color { return lipgloss.Color(s) }

// Syntax styles, by name.
type synStyle struct {
	fg                      string
	bold, italic, underline bool
}

var syntaxStyles []synStyle

const (
	sNormal uint8 = iota
	sKeyword
	sFunction
	sVariable
	sOperator
	sBuiltin
	sType
	sPreproc
	sAttribute
	sImport
	sString
	sEscape
	sNumber
	sConstant
	sComment
	sSpecial
	sHeading
	sEmphasis
	sStrong
	sTag
	sKey
	sInserted
	sError
)

// Lip Gloss styles for the bars, prompts and help page (set by applyTheme).
var (
	stBar, stBarName, stStatus, stBadge, stModified, stStatusDim lipgloss.Style
	stMessage, stError, stPrompt, stPromptTxt                    lipgloss.Style
	stHelpHead, stHelpKey, stHelpText, stHelpNew, stHelpBox      lipgloss.Style
)

func init() { applyTheme(0) }

// applyTheme switches every colour to theme i.
func applyTheme(i int) {
	if i < 0 || i >= len(themes) {
		i = 0
	}
	currentTheme = i
	p := themes[i]
	cBG, cText, cCurrentLine, cLineNumber, cCurrentNum = p.BG, p.Text, p.CurrentLine, p.LineNumber, p.CurrentNum
	cSelection, cSelectedText, cSearch, cSearchNow, cBracket = p.Selection, p.SelectedText, p.Search, p.SearchNow, p.Bracket
	cGutterLine, cBarBG, cStatusBG, cStatusText, cAccent, cEmber = p.GutterLine, p.BarBG, p.StatusBG, p.StatusText, p.Accent, p.Ember
	cDim, cError, cWarning, cGlow, cGradient = p.Dim, p.Error, p.Warning, p.Glow, p.Gradient

	syntaxStyles = []synStyle{
		sNormal:    {p.Text, false, false, false},
		sKeyword:   {p.Keyword, true, false, false},
		sFunction:  {p.Function, false, false, false},
		sVariable:  {p.Variable, false, false, false},
		sOperator:  {p.Operator, false, false, false},
		sBuiltin:   {p.Type, true, false, false},
		sType:      {p.Type, true, false, false},
		sPreproc:   {p.Keyword, true, false, false},
		sAttribute: {p.Function, false, true, false},
		sImport:    {p.Function, false, false, false},
		sString:    {p.String, false, false, false},
		sEscape:    {p.Escape, true, false, false},
		sNumber:    {p.Number, false, false, false},
		sConstant:  {p.Text, true, false, false},
		sComment:   {p.Comment, false, true, false},
		sSpecial:   {p.Escape, true, true, false},
		sHeading:   {p.Heading, true, false, false},
		sEmphasis:  {p.Emphasis, false, true, false},
		sStrong:    {p.Strong, true, false, false},
		sTag:       {p.Keyword, false, false, false},
		sKey:       {p.Function, false, false, false},
		sInserted:  {p.Number, false, false, false},
		sError:     {p.Error, false, false, true},
	}

	stBar = lipgloss.NewStyle().Background(hex(cBarBG)).Foreground(hex(cDim))
	stBarName = lipgloss.NewStyle().Background(hex(cBarBG)).Foreground(hex(cStatusText))
	stStatus = lipgloss.NewStyle().Background(hex(cStatusBG)).Foreground(hex(cStatusText))
	stBadge = lipgloss.NewStyle().Background(hex(cEmber)).Foreground(hex(cBG)).Bold(true).Padding(0, 1)
	stModified = lipgloss.NewStyle().Background(hex(cStatusBG)).Foreground(hex(cAccent)).Bold(true)
	stStatusDim = lipgloss.NewStyle().Background(hex(cStatusBG)).Foreground(hex(cDim))
	stMessage = lipgloss.NewStyle().Background(hex(cBG)).Foreground(hex(cStatusText))
	stError = lipgloss.NewStyle().Background(hex(cBG)).Foreground(hex(cError)).Bold(true)
	stPrompt = lipgloss.NewStyle().Background(hex(cBG)).Foreground(hex(cAccent)).Bold(true)
	stPromptTxt = lipgloss.NewStyle().Background(hex(cBG)).Foreground(hex(cGlow))
	stHelpHead = lipgloss.NewStyle().Foreground(hex(cEmber)).Bold(true)
	stHelpKey = lipgloss.NewStyle().Foreground(hex(cAccent)).Bold(true)
	stHelpText = lipgloss.NewStyle().Foreground(hex(cStatusText))
	stHelpNew = lipgloss.NewStyle().Foreground(hex(cDim)).Italic(true)
	stHelpBox = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(hex(cEmber)).Padding(0, 2)
}

// sgr builds the escape code for a text style. Results are kept, since the
// same few styles are drawn thousands of times a second.
var sgrCache = map[[5]string]string{}

func sgr(fg, bg string, bold, italic, underline bool) string {
	k := [5]string{fg, bg, flag(bold), flag(italic), flag(underline)}
	if s, ok := sgrCache[k]; ok {
		return s
	}
	st := ansi.Style{}.ForegroundColor(hex(fg)).BackgroundColor(hex(bg))
	if bold {
		st = st.Bold()
	}
	if italic {
		st = st.Italic(true)
	}
	if underline {
		st = st.Underline(true)
	}
	s := ansi.ResetStyle + st.String()
	sgrCache[k] = s
	return s
}

func flag(b bool) string {
	if b {
		return "1"
	}
	return ""
}

// glowText paints text in the theme's ember gradient.
func glowText(s string, bg string) string {
	runes := []rune(s)
	if len(runes) == 0 {
		return ""
	}
	var sb strings.Builder
	for i, r := range runes {
		t := 0.0
		if len(runes) > 1 {
			t = float64(i) / float64(len(runes)-1)
		}
		sb.WriteString(lipgloss.NewStyle().Foreground(gradientAt(t)).Background(hex(bg)).Bold(true).Render(string(r)))
	}
	return sb.String()
}

// gradientAt is the theme's ember gradient at t (0 to 1), mixed straight
// between the colours so it never strays outside them. (Lip Gloss's own
// blending went pink half way between two oranges.)
func gradientAt(t float64) color.Color {
	a, b := cGradient[0], cGradient[1]
	if t >= 0.5 {
		a, b, t = cGradient[1], cGradient[2], t-0.5
	}
	t *= 2
	ar, ag, ab := rgb(a)
	br, bg, bb := rgb(b)
	mix := func(x, y int) int { return int(float64(x) + (float64(y)-float64(x))*t + 0.5) }
	return lipgloss.Color(fmt.Sprintf("#%02x%02x%02x", mix(ar, br), mix(ag, bg), mix(ab, bb)))
}

func rgb(h string) (int, int, int) {
	var r, g, b int
	fmt.Sscanf(strings.TrimPrefix(h, "#"), "%02x%02x%02x", &r, &g, &b)
	return r, g, b
}
