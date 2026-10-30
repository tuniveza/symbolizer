package emmet

// Emmet's "update image size": width and height from the image file.

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"image"
	_ "image/gif" // registers GIF for image.DecodeConfig
	_ "image/jpeg"
	_ "image/png"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// UpdateImageSize sets the size of the image at pos.
//
// Markup: the cursor is on an <img src="…">; width and height attributes are
// set or replaced (jsx: width={N}). CSS: the cursor is in a rule with a
// url(…) declaration; "width: Npx; height: Npx;" are added after it or their
// values replaced. Reads PNG, JPEG, GIF, WebP and SVG. Paths are relative to
// baseDir (a leading "/" means the site root, found as for CorrectPathAt).
// A name like "logo@2x.png" halves the size. http(s) and data URLs give an
// error, as do missing or unreadable files.
func UpdateImageSize(text []rune, pos int, opt Options, baseDir string) (Change, error) {
	if IsStylesheet(opt.Syntax) {
		return actImageSizeCSS(text, pos, baseDir)
	}
	syn := actMarkupSyntax(opt.Syntax)
	d := actParse(text, syn)
	var img *actTok
	for i := range d.toks {
		t := &d.toks[i]
		if t.kind == actTokOpen && strings.EqualFold(t.name, "img") && t.start <= pos && pos <= t.end {
			img = t
		}
	}
	if img == nil {
		return Change{}, errors.New("no <img> at the cursor")
	}
	src := img.attr("src")
	if src == nil || !src.hasValue() || src.quote == '{' {
		return Change{}, errors.New("the <img> has no src to read")
	}
	w, h, err := actImageFileSize(string(text[src.valStart:src.valEnd]), baseDir)
	if err != nil {
		return Change{}, err
	}
	q := "\""
	if src.quote == '\'' {
		q = "'"
	}
	val := func(n int) string {
		if syn == "jsx" {
			return "{" + strconv.Itoa(n) + "}"
		}
		return q + strconv.Itoa(n) + q
	}
	var edits []Edit
	var add strings.Builder
	for _, wh := range []struct {
		name string
		n    int
	}{{"width", w}, {"height", h}} {
		a := img.attr(wh.name)
		switch {
		case a == nil:
			add.WriteString(" " + wh.name + "=" + val(wh.n))
		case !a.hasValue():
			edits = append(edits, Edit{a.nameEnd, a.end, "=" + val(wh.n)})
		case a.quote == '"' || a.quote == '\'':
			edits = append(edits, Edit{a.valStart, a.valEnd, strconv.Itoa(wh.n)})
		case a.quote == '{':
			edits = append(edits, Edit{a.valStart, a.valEnd, strconv.Itoa(wh.n)})
		default:
			edits = append(edits, Edit{a.valStart, a.valEnd, val(wh.n)})
		}
	}
	if add.Len() > 0 {
		at := img.attrs[len(img.attrs)-1].end
		edits = append(edits, Edit{at, at, add.String()})
	}
	edits = actNormalize(edits)
	p := actMapPos(edits, pos)
	return Change{Edits: edits, Sel: Range{p, p}, Note: fmt.Sprintf("image size: %d×%d", w, h)}, nil
}

var actURLRe = regexp.MustCompile(`url\(\s*(?:"([^"]*)"|'([^']*)'|([^)'"\s]*))\s*\)`)

func actImageSizeCSS(text []rune, pos int, baseDir string) (Change, error) {
	sh := actParseCSS(text)
	r := sh.ruleAt(pos)
	if r == nil {
		return Change{}, errors.New("no CSS rule at the cursor")
	}
	// the url() declaration: the one at the cursor, else the first in the rule
	pick := -1
	var src string
	for i, dc := range r.decls {
		m := actURLRe.FindStringSubmatch(string(text[dc.valStart:dc.valEnd]))
		if m == nil {
			continue
		}
		if pick < 0 || dc.start <= pos && pos <= dc.end {
			pick, src = i, m[1]+m[2]+m[3]
		}
	}
	if pick < 0 {
		return Change{}, errors.New("no url(…) in this rule")
	}
	w, h, err := actImageFileSize(src, baseDir)
	if err != nil {
		return Change{}, err
	}
	dc := r.decls[pick]
	var edits []Edit
	// how declarations are separated: the text before this one
	before := " "
	prevEnd := r.open + 1
	if pick > 0 {
		prevEnd = r.decls[pick-1].end
	}
	if prevEnd <= dc.start {
		before = string(text[prevEnd:dc.start])
		if strings.TrimSpace(before) != "" {
			before = "\n" + actLineIndent(text, dc.start)
		}
	}
	if before == "" {
		before = " "
	}
	sep := ": "
	if dc.nameEnd < dc.valStart {
		sep = string(text[dc.nameEnd:dc.valStart])
	}
	insertAt := dc.end
	if dc.semi < 0 {
		edits = append(edits, Edit{dc.end, dc.end, ";"})
	}
	var add strings.Builder
	for _, wh := range []struct {
		name string
		n    int
	}{{"width", w}, {"height", h}} {
		found := false
		for _, o := range r.decls {
			if strings.EqualFold(string(text[o.nameStart:o.nameEnd]), wh.name) {
				found = true
				edits = append(edits, Edit{o.valStart, o.valEnd, strconv.Itoa(wh.n) + "px"})
			}
		}
		if !found {
			add.WriteString(before + wh.name + sep + strconv.Itoa(wh.n) + "px;")
		}
	}
	if add.Len() > 0 {
		edits = append(edits, Edit{insertAt, insertAt, add.String()})
	}
	edits = actNormalize(edits)
	p := actMapPos(edits, pos)
	return Change{Edits: edits, Sel: Range{p, p}, Note: fmt.Sprintf("image size: %d×%d", w, h)}, nil
}

var actDensityRe = regexp.MustCompile(`@(\d+(?:\.\d+)?)x\.[A-Za-z0-9]+$`)

// actImageFileSize resolves src and reads the image's size in CSS pixels.
func actImageFileSize(src, baseDir string) (int, int, error) {
	src = strings.TrimSpace(src)
	if src == "" {
		return 0, 0, errors.New("the image path is empty")
	}
	if actHasScheme(src) || strings.HasPrefix(src, "//") {
		return 0, 0, fmt.Errorf("cannot read the size of a web or data image (%s)", actShort(src))
	}
	if i := strings.IndexAny(src, "?#"); i >= 0 {
		src = src[:i]
	}
	if u, err := url.PathUnescape(src); err == nil {
		src = u
	}
	p := actResolvePath(src, baseDir)
	f, err := os.Open(p)
	if err != nil {
		return 0, 0, fmt.Errorf("image not found: %s", src)
	}
	defer f.Close()
	w, h, err := actReadImageSize(f)
	if err != nil {
		return 0, 0, fmt.Errorf("%s: %v", src, err)
	}
	if m := actDensityRe.FindStringSubmatch(src); m != nil {
		if s, err := strconv.ParseFloat(m[1], 64); err == nil && s > 0 {
			w, h = int(float64(w)/s), int(float64(h)/s)
		}
	}
	return w, h, nil
}

func actShort(s string) string {
	if r := []rune(s); len(r) > 40 {
		return string(r[:37]) + "…"
	}
	return s
}

// actReadImageSize reads the size of a PNG, JPEG, GIF, WebP or SVG image.
func actReadImageSize(f io.Reader) (int, int, error) {
	data, err := io.ReadAll(io.LimitReader(f, 4<<20))
	if err != nil {
		return 0, 0, err
	}
	if w, h, ok := actWebPSize(data); ok {
		return w, h, nil
	}
	if cfg, _, err := image.DecodeConfig(bytes.NewReader(data)); err == nil {
		return cfg.Width, cfg.Height, nil
	}
	if w, h, ok := actSVGSize(data); ok {
		return w, h, nil
	}
	return 0, 0, errors.New("not a PNG, JPEG, GIF, WebP or SVG image (or no size in it)")
}

// actWebPSize reads the RIFF header of a WebP file (VP8, VP8L and VP8X).
func actWebPSize(b []byte) (int, int, bool) {
	if len(b) < 30 || string(b[0:4]) != "RIFF" || string(b[8:12]) != "WEBP" {
		return 0, 0, false
	}
	switch string(b[12:16]) {
	case "VP8 ":
		// frame tag (3 bytes), start code 9d 01 2a, then 14-bit width and height
		if b[23] != 0x9d || b[24] != 0x01 || b[25] != 0x2a {
			return 0, 0, false
		}
		w := int(binary.LittleEndian.Uint16(b[26:28]) & 0x3fff)
		h := int(binary.LittleEndian.Uint16(b[28:30]) & 0x3fff)
		return w, h, true
	case "VP8L":
		if b[20] != 0x2f {
			return 0, 0, false
		}
		bits := binary.LittleEndian.Uint32(b[21:25])
		return int(bits&0x3fff) + 1, int(bits>>14&0x3fff) + 1, true
	case "VP8X":
		w := int(b[24]) | int(b[25])<<8 | int(b[26])<<16
		h := int(b[27]) | int(b[28])<<8 | int(b[29])<<16
		return w + 1, h + 1, true
	}
	return 0, 0, false
}

// actSVGSize reads width/height (plain or px numbers) of the root <svg>, or
// its viewBox when they are missing or relative.
func actSVGSize(b []byte) (int, int, bool) {
	text := []rune(string(b))
	var svg *actTok
	toks := actScan(text, "xml")
	for i := range toks {
		if toks[i].kind == actTokOpen && strings.EqualFold(toks[i].name, "svg") {
			svg = &toks[i]
			break
		}
	}
	if svg == nil {
		return 0, 0, false
	}
	get := func(name string) string {
		if a := svg.attr(name); a != nil && a.hasValue() {
			return strings.TrimSpace(string(text[a.valStart:a.valEnd]))
		}
		return ""
	}
	num := func(s string) (float64, bool) {
		s = strings.TrimSuffix(s, "px")
		v, err := strconv.ParseFloat(s, 64)
		return v, err == nil && v > 0
	}
	w, wok := num(get("width"))
	h, hok := num(get("height"))
	var vw, vh float64
	vok := false
	if f := strings.FieldsFunc(get("viewBox"), func(r rune) bool { return r == ' ' || r == ',' }); len(f) == 4 {
		a, e1 := strconv.ParseFloat(f[2], 64)
		c, e2 := strconv.ParseFloat(f[3], 64)
		vw, vh, vok = a, c, e1 == nil && e2 == nil && a > 0 && c > 0
	}
	switch {
	case wok && hok:
	case wok && vok:
		h = w * vh / vw
	case hok && vok:
		w = h * vw / vh
	case vok:
		w, h = vw, vh
	default:
		return 0, 0, false
	}
	return int(w + 0.5), int(h + 0.5), true
}

// actHasScheme: "http:", "data:", "mailto:" … (but not "C:" style drive letters).
func actHasScheme(s string) bool {
	for i, r := range s {
		switch {
		case r == ':':
			return i > 1
		case actIsNameStart(r) && r < 128:
		case i > 0 && (actIsDigit(r) || r == '+' || r == '-' || r == '.'):
		default:
			return false
		}
	}
	return false
}

// actResolvePath turns a written path into a file path: relative to baseDir,
// or for a leading "/" relative to the site root (see actSiteRoot).
func actResolvePath(p, baseDir string) string {
	if strings.HasPrefix(p, "/") {
		return filepath.Join(actSiteRoot(baseDir), filepath.FromSlash(p))
	}
	return filepath.Join(baseDir, filepath.FromSlash(p))
}

var actRootMarkers = []string{"index.html", ".git", "package.json", "go.mod", "Cargo.toml", "composer.json", "pyproject.toml", "deno.json"}

// actSiteRoot gives the nearest folder at or above baseDir holding index.html
// or a project root marker, else the filesystem root.
func actSiteRoot(baseDir string) string {
	dir, err := filepath.Abs(baseDir)
	if err != nil {
		dir = baseDir
	}
	for {
		for _, m := range actRootMarkers {
			if _, err := os.Stat(filepath.Join(dir, m)); err == nil {
				return dir
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return string(filepath.Separator)
		}
		dir = parent
	}
}
