package main

import (
	"hash/fnv"
	"math/rand/v2"
	"regexp"
	"strings"
	"time"
	"unicode"
)

// File-like names for sessions (drafts with no name yet), shown on their tab
// and offered by Ctrl+S. With text in it, the name comes from the text: the
// first function or class, else a heading, else the first few words. An
// empty session gets an ember-flavoured random name that stays the same for
// that session. The ending (.go, .md, .txt ...) comes from what the text
// looks like.

var (
	defName  = regexp.MustCompile(`(?m)^\s*(?:pub\s+)?(?:def|class|func|fn|function|struct|interface|enum|trait)\s+(?:\([^)]*\)\s*)?([A-Za-z_][A-Za-z0-9_]{2,})`)
	cFunc    = regexp.MustCompile(`(?m)^\s*(?:static\s+)?(?:int|void|char|float|double|bool|auto)\s+\*?([A-Za-z_][A-Za-z0-9_]{2,})\s*\(`)
	heading  = regexp.MustCompile(`(?m)^#{1,6}\s+(.+)$`)
	skipLine = regexp.MustCompile(`^\s*(#include|#!|import\b|package\b|using\b|from\b|<!doctype|<html|<\?php|---)`)
)

var emberAdjectives = strings.Fields("amber ashen blazing bright cinder copper crimson drifting ember flickering " +
	"glowing golden kindled molten quiet russet scarlet smoky sparking sunset warm wild")
var emberNouns = strings.Fields("anvil beacon brand coal comet draft ember flame forge furnace glow hearth " +
	"kiln lantern phoenix sketch spark torch wick willow")

// suggestName gives a name (without the ending) and the ending.
func (b *Buffer) suggestName() (string, string) {
	if b.nameVersion == b.version+1 && b.nameCache != "" {
		return b.nameCache, b.nameExt
	}
	var sb strings.Builder
	for _, l := range b.Lines {
		sb.WriteString(string(l))
		sb.WriteByte('\n')
		if sb.Len() > 4096 {
			break
		}
	}
	text := sb.String()
	// Guessing is ~20 pattern searches: only redo it when the start of the
	// text changed, and at most every 0.3 s while typing.
	if b.nameCache != "" && (text == b.nameText || time.Since(b.nameAt) < 300*time.Millisecond) {
		b.nameVersion = b.version + 1
		return b.nameCache, b.nameExt
	}
	b.nameText, b.nameAt = text, time.Now()
	_, ext := guessLanguage(text)
	name := nameFromText(text, ext)
	if name == "" {
		name = b.randomName()
	}
	b.nameCache, b.nameExt, b.nameVersion = name, ext, b.version+1
	return name, ext
}

func nameFromText(text, ext string) string {
	if m := defName.FindStringSubmatch(text); m != nil {
		return slug(m[1])
	}
	if m := cFunc.FindStringSubmatch(text); m != nil {
		return slug(m[1])
	}
	if ext == ".md" || ext == ".txt" {
		if m := heading.FindStringSubmatch(text); m != nil {
			return slug(m[1])
		}
	}
	for _, line := range strings.Split(text, "\n") {
		if strings.TrimSpace(line) == "" || skipLine.MatchString(strings.ToLower(line)) {
			continue
		}
		if s := slug(line); s != "" {
			return s
		}
	}
	return ""
}

// slug keeps the first few words: "Hello, my old friend!" -> hello-my-old.
func slug(s string) string {
	var words []string
	word := []rune{}
	flush := func() {
		if len(word) > 0 {
			words = append(words, strings.ToLower(string(word)))
			word = word[:0]
		}
	}
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' {
			word = append(word, r)
		} else {
			flush()
		}
		if len(words) == 3 {
			break
		}
	}
	flush()
	if len(words) > 3 {
		words = words[:3]
	}
	out := strings.Join(words, "-")
	if len([]rune(out)) > 32 {
		out = string([]rune(out)[:32])
	}
	if len([]rune(out)) < 2 {
		return ""
	}
	return out
}

// randomName is the same for a session every time: it comes from the
// session's file name, or from a number picked when the session began.
func (b *Buffer) randomName() string {
	var seed uint64
	if b.Path != "" {
		h := fnv.New64a()
		h.Write([]byte(b.Path))
		seed = h.Sum64()
	} else {
		if b.nameSeed == 0 {
			b.nameSeed = rand.Uint64() | 1
		}
		seed = b.nameSeed
	}
	r := rand.New(rand.NewPCG(seed, seed>>17))
	return emberAdjectives[r.IntN(len(emberAdjectives))] + "-" + emberNouns[r.IntN(len(emberNouns))]
}
