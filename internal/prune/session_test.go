package prune

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func scanOptions(music, xml string) Options {
	return Options{
		XMLPath:      xml,
		MusicDir:     music,
		Extensions:   ParseExtensions(DefaultExtensions),
		MaxOrphanPct: DefaultMaxOrphanPct,
	}
}

func mustScan(t *testing.T, opts Options) *Result {
	t.Helper()
	r, err := Scan(opts)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	return r
}

// addUnresolvableEntry adds a library entry that points through a regular
// file. Statting it fails with ENOTDIR rather than ENOENT, which is what
// produces the unforceable unresolved-library finding.
func addUnresolvableEntry(t *testing.T, music, xml string) {
	t.Helper()
	through := filepath.Join(music, "House", "keep-0.mp3", "inner.mp3")
	body, err := os.ReadFile(xml)
	if err != nil {
		t.Fatal(err)
	}
	extra := "    <TRACK TrackID=\"unresolvable\" Location=\"" +
		locationFor(through) + "\"/>\n  </COLLECTION>"
	if err := os.WriteFile(xml,
		[]byte(strings.Replace(string(body), "  </COLLECTION>", extra, 1)), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestParseExtensions(t *testing.T) {
	got := strings.Join(ParseExtensions(" mp3, .wav,, FLAC "), "|")
	if want := ".mp3|.wav|.FLAC"; got != want {
		t.Errorf("ParseExtensions = %q, want %q", got, want)
	}
	if got := ParseExtensions(" , "); len(got) != 0 {
		t.Errorf("ParseExtensions of blanks = %q, want none", got)
	}
}

func TestScanHealthyCollection(t *testing.T) {
	music, xml := fixture(t, 600, 5)

	r := mustScan(t, scanOptions(music, xml))

	if got := len(r.Plan.Orphans); got != 5 {
		t.Errorf("orphans = %d, want 5", got)
	}
	if Aborts(r.Findings) {
		t.Errorf("healthy collection produced aborts: %v", r.Findings)
	}
	if err := r.CanApply(false); err != nil {
		t.Errorf("CanApply(false) = %v, want nil", err)
	}
	info, err := os.Stat(xml)
	if err != nil {
		t.Fatal(err)
	}
	if !r.XMLModTime.Equal(info.ModTime()) {
		t.Errorf("XMLModTime = %v, want %v", r.XMLModTime, info.ModTime())
	}
}

func TestScanMissingXMLIsAnError(t *testing.T) {
	music, _ := fixture(t, 600, 1)
	_, err := Scan(scanOptions(music, filepath.Join(music, "nope.xml")))
	if !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("Scan error = %v, want a not-exist error", err)
	}
}

func TestCanApplyForceableFindingNeedsForce(t *testing.T) {
	music, xml := fixture(t, 600, 5)
	opts := scanOptions(music, xml)
	opts.MaxOrphanPct = 0.1 // 5 of 605 is 0.8%, over the limit

	r := mustScan(t, opts)

	if len(r.Forceable()) == 0 {
		t.Fatalf("expected a forceable finding, got %v", r.Findings)
	}
	if len(r.Blocked()) != 0 {
		t.Fatalf("expected no blocked findings, got %v", r.Blocked())
	}
	if err := r.CanApply(false); !errors.Is(err, ErrNeedsForce) {
		t.Errorf("CanApply(false) = %v, want ErrNeedsForce", err)
	}
	if err := r.CanApply(true); err != nil {
		t.Errorf("CanApply(true) = %v, want nil", err)
	}
}

func TestCanApplyBlockedEvenWithForce(t *testing.T) {
	music, xml := fixture(t, 600, 1)
	addUnresolvableEntry(t, music, xml)

	r := mustScan(t, scanOptions(music, xml))

	if len(r.Blocked()) == 0 {
		t.Fatalf("expected a blocked finding, got %v", r.Findings)
	}
	for _, force := range []bool{false, true} {
		if err := r.CanApply(force); !errors.Is(err, ErrBlocked) {
			t.Errorf("CanApply(%v) = %v, want ErrBlocked", force, err)
		}
	}
}
