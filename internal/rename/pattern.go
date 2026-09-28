// Package rename renames MP3 files from their ID3 tags. It is the logic behind
// `dj rename` and the desktop app's MP3 Rename tool.
package rename

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"
)

// Tokens a pattern may use, each filled from one ID3 frame.
const (
	TokenArtist = "artist" // TPE1
	TokenTitle  = "title"  // TIT2
	TokenBPM    = "bpm"    // TBPM
	TokenKey    = "key"    // TKEY
)

var knownTokens = map[string]bool{TokenArtist: true, TokenTitle: true, TokenBPM: true, TokenKey: true}

// DefaultPattern is MP3 Renamer's default, which produces names such as
// "123 04A Daft Punk - One More Time.mp3".
const DefaultPattern = "{bpm:03} {key:03} {artist} - {title}"

type segment struct {
	literal string
	token   string // empty for a literal segment
	width   int    // zero-pad to at least this many characters
}

// Pattern is a parsed name pattern. The zero value expands to "".
type Pattern struct {
	segs []segment
}

// ParsePattern parses a pattern such as "{bpm:03} {artist} - {title}".
//
// Unlike MP3 Renamer, which left anything it did not recognise in the name, an
// unknown or malformed token is an error: a typo should be caught before any
// file is renamed, not discovered in the Finder afterwards. A pattern without
// tokens is refused too, since every file would get the same name.
func ParsePattern(s string) (Pattern, error) {
	var (
		p   Pattern
		lit strings.Builder
	)
	flush := func() {
		if lit.Len() > 0 {
			p.segs = append(p.segs, segment{literal: lit.String()})
			lit.Reset()
		}
	}
	for i := 0; i < len(s); {
		if s[i] != '{' {
			// "/" would move a rename's target into another folder (or one
			// that doesn't exist); ":" shows as "/" in the Finder, which
			// hides the same problem. Neither is allowed in literal text --
			// clean() strips both from tag values, but the pattern's own
			// literal characters are not run through clean.
			if s[i] == '/' || s[i] == ':' {
				return Pattern{}, fmt.Errorf("%q is not allowed in a pattern: it would move files into another folder", string(s[i]))
			}
			lit.WriteByte(s[i])
			i++
			continue
		}
		end := strings.IndexByte(s[i:], '}')
		if end < 0 {
			return Pattern{}, fmt.Errorf("unclosed { at position %d", i+1)
		}
		body := s[i+1 : i+end]
		name, width, hasWidth := strings.Cut(body, ":")
		if !knownTokens[name] {
			return Pattern{}, fmt.Errorf("unknown token {%s}: use {artist}, {title}, {bpm} or {key}", body)
		}
		w := 0
		if hasWidth {
			n, err := strconv.Atoi(width)
			if err != nil || n < 0 {
				return Pattern{}, fmt.Errorf("bad width in {%s}: want a number, as in {%s:03}", body, name)
			}
			w = n
		}
		flush()
		p.segs = append(p.segs, segment{token: name, width: w})
		i += end + 1
	}
	flush()
	if len(p.Tokens()) == 0 {
		return Pattern{}, errors.New("the pattern has no tokens, so every file would get the same name")
	}
	return p, nil
}

// Tokens returns the distinct tokens the pattern uses, in order of first use.
func (p Pattern) Tokens() []string {
	var out []string
	seen := map[string]bool{}
	for _, s := range p.segs {
		if s.token != "" && !seen[s.token] {
			seen[s.token] = true
			out = append(out, s.token)
		}
	}
	return out
}

// Expand fills every token from values, zero-padding where a width is given.
// It does not add the .mp3 extension.
func (p Pattern) Expand(values map[string]string) string {
	var b strings.Builder
	for _, s := range p.segs {
		if s.token == "" {
			b.WriteString(s.literal)
			continue
		}
		v := values[s.token]
		if n := utf8.RuneCountInString(v); n < s.width {
			b.WriteString(strings.Repeat("0", s.width-n))
		}
		b.WriteString(v)
	}
	return b.String()
}
