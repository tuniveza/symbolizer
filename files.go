package main

import (
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
)

// Opening, naming and closing files. Saving itself happens on its own
// (autosave.go); Ctrl+S only names a session or saves at once.

// save is Ctrl+S.
func (m *Model) save() tea.Cmd {
	b := m.buf
	if b.Draft {
		return m.askName()
	}
	if m.savePaused {
		return m.confirm("Another program changed this file. Save yours over it?  y = yes   n = no", "yn",
			func(m *Model, k rune) tea.Cmd {
				if k != 'y' {
					m.say("Not saved. Alt+L can load their version.")
					return nil
				}
				m.savePaused = false
				return m.saveAs(m.buf.Path)
			})
	}
	if m.saveNow() {
		m.say(fmt.Sprintf("Saved %s (%d lines). It also saves by itself after every change.",
			filepath.Base(b.Path), len(b.Lines)))
	}
	return nil
}

// askName asks what to call a session. The ending (.cpp, .c, .py ...) comes
// from what the text looks like, and is added if the name has none.
func (m *Model) askName() tea.Cmd {
	lang, ext := guessLanguage(m.sample(32 << 10))
	m.nameExt = ext
	label := fmt.Sprintf("Name it (%s, %s is added): ", lang, ext)
	if lang == "Text" {
		label = "Name it (.txt is added): "
	}
	cmd := m.ask(pName, label, "")
	m.nameSuggest, _ = m.buf.suggestName()
	m.input.Placeholder = m.nameSuggest + "   (Enter takes this name)"
	return cmd
}

// nameEntered turns what was typed into a path in the project or start folder.
func (m *Model) nameEntered(value string) tea.Cmd {
	value = strings.TrimSpace(value)
	if value == "" {
		value = m.nameSuggest // Enter on its own takes the suggested name
	}
	if value == "" {
		m.say("Not named. It is still kept as a session.")
		return nil
	}
	if filepath.Ext(filepath.Base(value)) == "" {
		value += m.nameExt
	}
	path := value
	if !filepath.IsAbs(path) && !strings.HasPrefix(path, "~") {
		path = filepath.Join(m.baseDir(), path)
	}
	path = expandPath(path)
	if fileExists(path) {
		return m.confirm(filepath.Base(path)+" already exists. Replace it?  y = yes   n = no", "yn",
			func(m *Model, k rune) tea.Cmd {
				if k == 'y' {
					return m.saveAs(path)
				}
				return m.askName()
			})
	}
	return m.saveAs(path)
}

// baseDir is where new names go: the project folder, else where sy started.
func (m *Model) baseDir() string {
	if m.tree != nil {
		return m.tree.root
	}
	if m.buf.Path != "" && !m.buf.Draft {
		return filepath.Dir(m.buf.Path)
	}
	return m.startDir
}

// saveAs writes the text to path and keeps editing it there. A session that
// gets a name moves into the named file.
func (m *Model) saveAs(path string) tea.Cmd {
	b := m.buf
	old, wasDraft := b.Path, b.Draft
	if dir := filepath.Dir(path); !fileExists(dir) {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			m.sayErr("Could not make the folder: " + err.Error())
			return nil
		}
	}
	if err := b.Save(path); err != nil {
		m.sayErr("Could not save: " + err.Error())
		return nil
	}
	b.Draft = isSession(path)
	if wasDraft && old != "" && old != path {
		os.Remove(old)
	}
	m.savePaused, m.saveErr = false, ""
	m.resetHighlighter()
	m.refreshTree()
	m.say(fmt.Sprintf("Saved as %s in %s. It keeps saving by itself.", filepath.Base(path), shortPath(filepath.Dir(path))))
	return nil
}

// ifUnsaved runs fn once the text is safely saved. Only if saving fails
// (or is paused because another program changed the file) does it ask.
func (m *Model) ifUnsaved(what string, fn func(m *Model) tea.Cmd) tea.Cmd {
	if m.saveNow() {
		return fn(m)
	}
	if m.savePaused {
		return m.confirm("Another program changed this file. "+what+"  k = keep mine (save over theirs)   d = drop mine   Esc = cancel",
			"kd", func(m *Model, k rune) tea.Cmd {
				if k == 'k' {
					m.savePaused = false
					if err := m.buf.Save(""); err != nil {
						m.sayErr("Could not save: " + err.Error())
						return nil
					}
				}
				return fn(m)
			})
	}
	return m.confirm("Could not save ("+m.saveErr+"). "+what+"  d = drop the changes   Esc = cancel",
		"d", func(m *Model, k rune) tea.Cmd { return fn(m) })
}

// openPath opens a file in a tab (or shows its tab if already open).
func (m *Model) openPath(path string) tea.Cmd {
	if i := m.findTab(path); i >= 0 {
		return m.switchTab(i)
	}
	nb := NewBuffer()
	if fileExists(path) {
		if err := nb.Load(path); err != nil {
			m.sayErr("Could not open: " + err.Error())
			return nil
		}
		m.say(fmt.Sprintf("Opened %s (%d lines).", filepath.Base(path), len(nb.Lines)))
	} else {
		nb.Path = path
		m.say("New file " + filepath.Base(path) + ". It saves by itself as you type.")
	}
	nb.Draft = isSession(path)
	m.saveNow()
	if m.blankDraft() {
		m.buf = nb
		m.top, m.left, m.savePaused, m.saveErr = 0, 0, false, ""
		m.resetHighlighter()
		m.storeTab()
		return nil
	}
	m.addTab(nb)
	return nil
}

// maxFolderTabs is the most files one folder opens as tabs; folderScan is
// how many it looks at to choose them.
const maxFolderTabs = 50
const folderScan = 2000

// openFolder opens the text files in dir and its subfolders as tabs, the
// most code-like kinds first (codeRank). Within a kind, each folder's own
// files come first, then its subfolders, A to Z. It shows the first one and
// says whether any opened.
func (m *Model) openFolder(dir string) bool {
	hidden := m.tree != nil && m.tree.showHidden
	var paths []string
	skipped, full := 0, false
	var walk func(d string) error
	walk = func(d string) error {
		entries, err := os.ReadDir(d)
		if err != nil {
			return err
		}
		sort.Slice(entries, func(i, j int) bool {
			return strings.ToLower(entries[i].Name()) < strings.ToLower(entries[j].Name())
		})
		var subs []string
		for _, e := range entries {
			name := e.Name()
			if !hidden && strings.HasPrefix(name, ".") {
				continue
			}
			p := filepath.Join(d, name)
			info, err := os.Stat(p) // follows links
			switch {
			case err != nil:
			case info.IsDir():
				// Linked folders are left out, so a link to a parent cannot loop.
				if !skipDirs[name] && e.Type()&fs.ModeSymlink == 0 {
					subs = append(subs, p)
				}
			case !textFile(p, info.Size()):
				skipped++
			case len(paths) == folderScan:
				full = true
				return nil
			default:
				paths = append(paths, p)
			}
		}
		for _, sub := range subs {
			if full {
				break
			}
			walk(sub) // an unreadable subfolder is left out
		}
		return nil
	}
	if err := walk(dir); err != nil {
		m.sayErr("Could not read the folder: " + err.Error())
		return false
	}
	if len(paths) == 0 {
		m.say("No text files in " + shortPath(dir) + ", so no tabs were opened.")
		return false
	}
	sort.SliceStable(paths, func(i, j int) bool {
		return codeRank(filepath.Base(paths[i])) < codeRank(filepath.Base(paths[j]))
	})
	if len(paths) > maxFolderTabs {
		paths, full = paths[:maxFolderTabs], true
	}
	for _, p := range paths {
		m.openPath(p)
	}
	m.switchTab(m.findTab(paths[0]))
	msg := fmt.Sprintf("Opened %d files from %s as tabs.", len(paths), filepath.Base(dir))
	if len(paths) == 1 {
		msg = "Opened 1 file from " + filepath.Base(dir) + " as a tab."
	}
	if full {
		msg += fmt.Sprintf(" Stopped at %d, the most at once (code first).", maxFolderTabs)
	}
	if skipped > 0 {
		msg += fmt.Sprintf(" Skipped %d that are not text or too big.", skipped)
	}
	m.say(msg)
	return true
}

// codeKinds lists file kinds by how much they matter when writing code, most
// first: endings, then whole file names (a name wins over its ending). The
// empty kind is where files of a kind not listed go.
var codeKinds = []struct{ endings, names string }{
	// web pages and page templates
	{"html htm xhtml shtml vue svelte astro marko ejs hbs handlebars mustache pug jade njk jinja jinja2 j2 " +
		"twig erb liquid haml slim cshtml razor eex heex leex mjml", ""},
	// scripts for the web
	{"js mjs cjs jsx ts tsx mts cts es6 coffee litcoffee elm res resi purs ls", ""},
	// styles
	{"css scss sass less styl stylus pcss postcss", ""},
	// other code
	{"go py pyw pyi pyx pxd rs c h cpp cc cxx c++ hpp hh hxx ipp inl tpp m mm java kt kts scala sc groovy gvy " +
		"swift rb rake php phtml cs csx fs fsi fsx vb lua luau dart zig ex exs erl hrl gleam hs lhs ml mli " +
		"re rei clj cljs cljc nim nims cr jl r rmd pl pm raku rakumod sql psql plsql glsl vert frag comp geom " +
		"hlsl wgsl metal cu cuh vim el lisp lsp scm ss rkt fnl janet gd gdshader asm s nasm v sv svh vhd vhdl " +
		"ada adb ads f f90 f95 f03 for pas pp lpr d odin hx sol move cairo vy mojo vala qml ino pde tcl awk " +
		"proto graphql gql thrift prisma smithy wat applescript ahk au3 bas", ""},
	// shell scripts
	{"sh bash zsh fish ksh csh tcsh nu xsh ps1 psm1 psd1 bat cmd", ""},
	// building, packaging and running
	{"mk mak mod work gradle cmake nix bzl bazel ebuild spec tf tfvars hcl dockerfile",
		"makefile gnumakefile dockerfile containerfile justfile cmakelists.txt meson.build meson_options.txt " +
			"build build.bazel workspace module.bazel vagrantfile procfile gemfile rakefile pipfile brewfile " +
			"podfile cargo.toml pyproject.toml setup.cfg setup.py requirements.txt requirements-dev.txt go.mod " +
			"go.work pom.xml build.gradle settings.gradle build.sbt pkgbuild jenkinsfile tiltfile earthfile " +
			"docker-compose.yml docker-compose.yaml compose.yml compose.yaml flake.nix default.nix shell.nix"},
	// kinds not listed
	{"", ""},
	// notes and writing
	{"md markdown mdx mdown rst txt text adoc asciidoc org tex ltx sty cls bib typ pod rdoc textile wiki",
		"readme license licence copying changelog changes authors contributors notice todo news history " +
			"contributing code_of_conduct security"},
	// settings
	{"toml yaml yml ini cfg conf config env properties desktop service socket timer mount rasi kdl ron " +
		"cue dhall xresources gitignore gitattributes gitmodules dockerignore editorconfig prettierrc " +
		"eslintrc stylelintrc babelrc npmrc nvmrc envrc tool-versions", ""},
	// XML and SVG
	{"xml svg plist xsd xsl xslt wsdl rss atom kml gpx csproj fsproj vbproj props targets resx xaml sln", ""},
	// data, logs and patches
	{"csv tsv psv log out dat srt vtt ass ics vcf diff patch po pot", ""},
	// JSON
	{"json jsonc json5 jsonl ndjson geojson webmanifest har map ipynb", ""},
	// lock files, last
	{"lock sum", "go.sum cargo.lock yarn.lock pnpm-lock.yaml poetry.lock uv.lock flake.lock gemfile.lock " +
		"composer.lock pipfile.lock package-lock.json bun.lockb mix.lock"},
}

var codeEndingRank, codeNameRank = map[string]int{}, map[string]int{}
var codeUnknownRank int

func init() {
	for rank, k := range codeKinds {
		if k.endings == "" && k.names == "" {
			codeUnknownRank = rank
		}
		for _, e := range strings.Fields(k.endings) {
			codeEndingRank[e] = rank
		}
		for _, n := range strings.Fields(k.names) {
			codeNameRank[n] = rank
		}
	}
}

// codeRank is where a file's kind comes in codeKinds.
func codeRank(name string) int {
	lower := strings.ToLower(name)
	if r, ok := codeNameRank[lower]; ok {
		return r
	}
	if r, ok := codeEndingRank[strings.TrimPrefix(filepath.Ext(lower), ".")]; ok {
		return r
	}
	return codeUnknownRank
}

// textFile says whether a file looks like text worth a tab: not too big,
// and its start has no zero bytes and is UTF-8.
func textFile(path string, size int64) bool {
	if size > 8<<20 {
		return false
	}
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()
	head := make([]byte, 8192)
	n, _ := f.Read(head)
	full := n == len(head)
	head = head[:n]
	if bytes.IndexByte(head, 0) >= 0 {
		return false
	}
	if full {
		// The read may have cut the last character in two.
		for i := 0; i < utf8.UTFMax && len(head) > 0 && !utf8.Valid(head); i++ {
			head = head[:len(head)-1]
		}
	}
	return utf8.Valid(head)
}

// reload loads the file on screen again from disk (Alt+L list).
func (m *Model) reload() tea.Cmd {
	path := m.buf.Path
	if path == "" || !fileExists(path) {
		m.sayErr("Not on disk yet, so there is nothing to load.")
		return nil
	}
	if m.savePaused {
		// Their version wins: that is what loading it again means.
		m.savePaused = false
		m.buf.MarkSaved(m.buf.Version(), m.buf.ModTime)
	}
	return m.ifUnsaved("Load it from disk?", func(m *Model) tea.Cmd {
		nb := NewBuffer()
		if err := nb.Load(path); err != nil {
			m.sayErr("Could not load: " + err.Error())
			return nil
		}
		nb.Draft = m.buf.Draft
		cur, top := m.buf.Cursor, m.top
		m.buf = nb
		m.resetHighlighter()
		m.buf.Cursor, m.top = nb.Clamp(cur), min(top, len(nb.Lines)-1)
		m.saveErr = ""
		m.storeTab()
		m.say("Loaded " + filepath.Base(path) + " from disk.")
		return nil
	})
}

// newFile is Ctrl+N: a new session in a new tab.
func (m *Model) newFile() tea.Cmd {
	m.saveNow()
	if m.blankDraft() {
		m.say("This is already a new session. Type, and it saves by itself.")
		return nil
	}
	m.addTab(newDraft())
	m.say("New session. It saves by itself. Ctrl+S gives it a name.")
	return nil
}

func (m *Model) quit() tea.Cmd {
	return m.ifUnsaved("Quit anyway?", func(m *Model) tea.Cmd {
		m.storeTab()
		for _, t := range m.tabs {
			dropEmptySession(t.buf)
		}
		m.quitClean = true
		return tea.Quit
	})
}

// shortPath writes the home folder as ~.
func shortPath(p string) string {
	if home, err := os.UserHomeDir(); err == nil && (p == home || strings.HasPrefix(p, home+"/")) {
		return "~" + p[len(home):]
	}
	return p
}

// rescue writes unsaved text somewhere safe if Symbolizer crashes.
func rescue(b *Buffer) string {
	if b == nil || !b.Modified() {
		return ""
	}
	path := b.Path + ".sy-rescue"
	if b.Path == "" {
		home, _ := os.UserHomeDir()
		path = filepath.Join(home, "symbolizer-rescue.txt")
	}
	if err := os.WriteFile(path, []byte(b.Text()+"\n"), 0o600); err != nil {
		return ""
	}
	return path
}
