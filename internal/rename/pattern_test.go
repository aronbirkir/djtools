package rename

import (
	"strings"
	"testing"
)

var sampleValues = map[string]string{
	TokenArtist: "Daft Punk",
	TokenTitle:  "One More Time",
	TokenBPM:    "123",
	TokenKey:    "4A",
}

func TestExpandDefaultPattern(t *testing.T) {
	p, err := ParsePattern(DefaultPattern)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := p.Expand(sampleValues), "123 04A Daft Punk - One More Time"; got != want {
		t.Errorf("Expand = %q, want %q", got, want)
	}
}

func TestExpandPadding(t *testing.T) {
	for _, tt := range []struct{ pattern, bpm, want string }{
		{"{bpm:03}", "95", "095"},
		{"{bpm:03}", "128", "128"},
		{"{bpm:02}", "128", "128"}, // longer than the width is left alone
		{"{bpm:0}", "95", "95"},
		{"{bpm}", "95", "95"},
	} {
		p, err := ParsePattern(tt.pattern)
		if err != nil {
			t.Fatalf("%s: %v", tt.pattern, err)
		}
		if got := p.Expand(map[string]string{TokenBPM: tt.bpm}); got != tt.want {
			t.Errorf("%s with %q = %q, want %q", tt.pattern, tt.bpm, got, tt.want)
		}
	}
}

func TestExpandRepeatedToken(t *testing.T) {
	p, err := ParsePattern("{artist} - {title} ({artist})")
	if err != nil {
		t.Fatal(err)
	}
	if got, want := p.Expand(sampleValues), "Daft Punk - One More Time (Daft Punk)"; got != want {
		t.Errorf("Expand = %q, want %q", got, want)
	}
}

func TestExpandKeepsLiteralBraceClose(t *testing.T) {
	p, err := ParsePattern("{title} }")
	if err != nil {
		t.Fatal(err)
	}
	if got, want := p.Expand(sampleValues), "One More Time }"; got != want {
		t.Errorf("Expand = %q, want %q", got, want)
	}
}

func TestTokensInOrderWithoutDuplicates(t *testing.T) {
	p, err := ParsePattern("{title} {artist} {title}")
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(p.Tokens(), ","); got != "title,artist" {
		t.Errorf("Tokens = %q, want title,artist", got)
	}
}

func TestParsePatternRejectsPathSeparators(t *testing.T) {
	// A literal "/" or ":" in the pattern text (outside a token) would move
	// a rename's target into another folder, or (on some volumes) be shown
	// as "/" by the Finder in a way that hides the real separator. Both are
	// refused before any file is touched.
	for pattern, wantInErr := range map[string]string{
		"../{title}":       `"/" is not allowed in a pattern`,
		"{artist}/{title}": `"/" is not allowed in a pattern`,
		"{artist}:{title}": `":" is not allowed in a pattern`,
	} {
		_, err := ParsePattern(pattern)
		if err == nil || !strings.Contains(err.Error(), wantInErr) {
			t.Errorf("ParsePattern(%q) = %v, want an error containing %q", pattern, err, wantInErr)
		}
	}
}

func TestParsePatternErrors(t *testing.T) {
	for pattern, wantInErr := range map[string]string{
		"{album}":        "unknown token {album}",
		"{artist":        "unclosed {",
		"{bpm:x}":        "bad width",
		"{bpm:-1}":       "bad width",
		"just text":      "no tokens",
		"":               "no tokens",
		"{artist} {Key}": "unknown token {Key}",
	} {
		_, err := ParsePattern(pattern)
		if err == nil || !strings.Contains(err.Error(), wantInErr) {
			t.Errorf("ParsePattern(%q) = %v, want an error containing %q", pattern, err, wantInErr)
		}
	}
}
