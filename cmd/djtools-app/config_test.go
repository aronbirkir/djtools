package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

var testDefaults = Config{
	XMLPath:      "/d/rekordbox.xml",
	MusicDir:     "/d/music",
	Extensions:   ".mp3",
	MaxOrphanPct: 60,
	Theme:        ThemeSystem,
}

func TestDefaultXMLPath(t *testing.T) {
	for _, tt := range []struct{ goos, want string }{
		{"darwin", filepath.Join("/Users/me", "Library", "Pioneer", "rekordbox", "rekordbox.xml")},
		{"windows", filepath.Join(`C:\Users\me\AppData\Roaming`, "Pioneer", "rekordbox", "rekordbox.xml")},
		{"linux", ""},
	} {
		if got := defaultXMLPath(tt.goos, "/Users/me", `C:\Users\me\AppData\Roaming`); got != tt.want {
			t.Errorf("%s: got %q, want %q", tt.goos, got, tt.want)
		}
	}
}

func TestExpandHome(t *testing.T) {
	for in, want := range map[string]string{
		"~":                 "/Users/me",
		"~/Music/rekordbox": "/Users/me/Music/rekordbox",
		"  /abs/path  ":     "/abs/path",
		"~other/x":          "~other/x",
		"":                  "",
	} {
		if got := expandHome(in, "/Users/me"); got != want {
			t.Errorf("expandHome(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestReportPath(t *testing.T) {
	at := time.Date(2026, 9, 24, 15, 30, 12, 0, time.Local)
	if got, want := reportPath("/r", at), filepath.Join("/r", "prune-20260924-153012.txt"); got != want {
		t.Errorf("reportPath = %q, want %q", got, want)
	}
}

func TestLoadConfigMissingFileUsesDefaults(t *testing.T) {
	got := loadConfigFrom(filepath.Join(t.TempDir(), "none.json"), testDefaults)
	if got != testDefaults {
		t.Errorf("got %+v, want defaults", got)
	}
}

func TestLoadConfigKeepsDefaultsForMissingFields(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(`{"xml": "/mine.xml"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	got := loadConfigFrom(path, testDefaults)
	want := testDefaults
	want.XMLPath = "/mine.xml"
	if got != want {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

func TestLoadConfigCorruptFileUsesDefaults(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(`{"xml": `), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := loadConfigFrom(path, testDefaults); got != testDefaults {
		t.Errorf("got %+v, want defaults", got)
	}
}

func TestSaveConfigRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", "config.json")
	cfg := Config{XMLPath: "/x.xml", MusicDir: "/m", Extensions: ".mp3,.wav",
		MaxOrphanPct: 45.5, KeepEmptyDirs: true, Theme: ThemeDark}
	if err := saveConfigTo(path, cfg); err != nil {
		t.Fatal(err)
	}
	if got := loadConfigFrom(path, testDefaults); got != cfg {
		t.Errorf("got %+v, want %+v", got, cfg)
	}
}

func TestUniqueReportPath(t *testing.T) {
	dir := t.TempDir()
	at := time.Date(2026, 9, 24, 15, 30, 12, 0, time.Local)
	base := filepath.Join(dir, "prune-20260924-153012")
	if got := uniqueReportPath(dir, at); got != base+".txt" {
		t.Fatalf("first = %q", got)
	}
	for _, want := range []string{base + "-2.txt", base + "-3.txt"} {
		prev := uniqueReportPath(dir, at)
		if err := os.WriteFile(prev, nil, 0o644); err != nil {
			t.Fatal(err)
		}
		if got := uniqueReportPath(dir, at); got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	}
}
