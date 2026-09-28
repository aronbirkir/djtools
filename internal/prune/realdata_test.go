//go:build realdata

// Run with: go test -tags realdata ./internal/prune -run TestReal -v
//
// These tests assert invariants, not absolute counts. The collection is live:
// tracks get added and trimmed between runs, and 7 arrived during this
// project's own implementation. Pinning `orphans == 6645` would fail on every
// download and train whoever runs it to ignore red. The measured figures are in
// docs/superpowers/specs/2026-08-27-rekordbox-prune-design.md and are logged
// here for comparison rather than asserted.
package prune

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/aronbirkir/djtools/internal/rekordbox"
)

const (
	realXML   = "/Users/aron/DJ/rekordbox.xml"
	realMusic = "/Users/aron/DJ/music"
)

func loadReal(t *testing.T) (*rekordbox.Collection, *Plan, time.Time) {
	t.Helper()

	info, err := os.Stat(realXML)
	if err != nil {
		t.Skipf("real collection unavailable: %v", err)
	}
	f, err := os.Open(realXML)
	if err != nil {
		t.Skipf("real collection unavailable: %v", err)
	}
	defer f.Close()

	c, err := rekordbox.Parse(f)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	p, err := Build(c, realMusic,
		[]string{".mp3", ".wav", ".aiff", ".flac", ".m4a"}, info.ModTime())
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	return c, p, info.ModTime()
}

func TestRealCollectionInvariants(t *testing.T) {
	c, p, exportedAt := loadReal(t)

	// A truncated export is the failure most likely to delete the library.
	if c.DeclaredEntries != len(c.Tracks) {
		t.Errorf("COLLECTION declares %d entries but %d TRACK elements parsed: truncated export",
			c.DeclaredEntries, len(c.Tracks))
	}

	// Nothing may be deleted from an incompletely resolved library.
	if len(p.Unresolved) != 0 {
		t.Errorf("Unresolved = %v, want none", p.Unresolved)
	}

	// Every walked audio file is exactly one of keeper, orphan, or skipped.
	if sum := p.Keepers + len(p.Orphans) + len(p.RecentlyAdded); sum != p.OnDiskAudio() {
		t.Errorf("keepers+orphans+skipped = %d, but OnDiskAudio() = %d", sum, p.OnDiskAudio())
	}

	// Deliberately NOT asserted: Keepers == len(Library). Keepers counts walked
	// paths while Library counts inodes, so a hard link makes two paths share one
	// inode, and a library entry whose extension --ext excludes is resolved but
	// never walked as audio. It holds on this collection today, which is exactly
	// why asserting it would be a trap.
	if p.Keepers > p.OnDiskAudio() {
		t.Errorf("Keepers = %d exceeds OnDiskAudio() = %d", p.Keepers, p.OnDiskAudio())
	}
	if p.Keepers != len(p.Library) {
		t.Logf("note: Keepers = %d, len(Library) = %d (hard links or excluded extensions)",
			p.Keepers, len(p.Library))
	}

	// The whole point of RecentlyAdded: nothing newer than the export is trashable.
	for _, o := range p.Orphans {
		st, err := os.Stat(o)
		if err != nil {
			t.Errorf("orphan %s vanished mid-run: %v", o, err)
			continue
		}
		if st.ModTime().After(exportedAt) {
			t.Errorf("orphan is newer than the export and would be trashed: %s", o)
		}
	}

	// Every path queued for deletion must lie inside the music directory.
	for _, o := range p.Orphans {
		if err := checkUnder(filepath.Clean(realMusic), o); err != nil {
			t.Errorf("orphan outside the music dir: %v", err)
		}
	}

	if len(p.Library) < DefaultMinLibrary {
		t.Errorf("only %d library files resolved, below the %d floor", len(p.Library), DefaultMinLibrary)
	}

	t.Logf("declared=%d parsed=%d library=%d keepers=%d orphans=%d skipped=%d eligible=%d "+
		"deadlinks=%d stalepaths=%d staledupes=%d casedupes=%d foreign=%d nonfile=%d "+
		"leftovers=%d symlinks=%d ondisk=%d pct=%.1f%% reclaim=%.1fGB",
		c.DeclaredEntries, len(c.Tracks), len(p.Library), p.Keepers, len(p.Orphans),
		len(p.RecentlyAdded), p.EligibleAudio(), len(p.DeadLinks), len(p.Stale),
		len(p.StaleDupes), len(p.CaseDupes), p.Foreign, p.NonFile, len(p.Leftovers),
		len(p.Symlinks), p.OnDiskAudio(), p.OrphanPct(), float64(p.OrphanSize)/1e9)
	t.Logf("spec baseline 2026-08-27 22:42 export: library=8374 keepers=8374 orphans=6645 " +
		"skipped=0 deadlinks=4 stalepaths=94 staledupes=30 casedupes=8 foreign=8 nonfile=2 " +
		"leftovers=35 symlinks=0 ondisk=15019 pct=44.2%% reclaim=35.7GB")
}

// The real collection must clear every guard without --force. If it does not,
// the thresholds are wrong rather than the collection.
func TestRealCollectionClearsGuards(t *testing.T) {
	c, p, _ := loadReal(t)
	findings := Check(p, c, GuardOptions{
		MaxOrphanPct: DefaultMaxOrphanPct,
		MinLibrary:   DefaultMinLibrary,
	})
	for _, f := range findings {
		if f.Level == LevelAbort {
			t.Errorf("unexpected abort [%s]: %s", f.Code, f.Message)
		} else {
			t.Logf("warning (expected) [%s]: %s", f.Code, f.Message)
		}
	}
}

// Proves file-identity matching is doing real work rather than being ceremony.
//
// Naive path-string comparison reports more orphans, and every extra one is a
// file that IS in the library -- all with accented or Icelandic filenames.
// Counts are not asserted because the collection is live; what must hold is the
// direction and the explanation.
func TestRealCollectionStringMatchingIsWrong(t *testing.T) {
	_, p, _ := loadReal(t)

	inLibrary := make(map[string]bool)
	for _, spellings := range p.Library {
		for _, s := range spellings {
			inLibrary[s] = true
		}
	}

	stringOrphans, err := orphansByString(p.MusicDir, inLibrary)
	if err != nil {
		t.Fatalf("orphansByString: %v", err)
	}

	inodeOrphans := make(map[string]bool, len(p.Orphans))
	for _, o := range p.Orphans {
		inodeOrphans[o] = true
	}

	var wouldDelete int
	for _, s := range stringOrphans {
		if inodeOrphans[s] {
			continue // both methods agree it is an orphan
		}
		// String matching calls this an orphan; identity matching does not. It
		// must genuinely be in the library, or the premise is wrong.
		info, err := os.Stat(s)
		if err != nil {
			t.Errorf("cannot stat %s: %v", s, err)
			continue
		}
		id, ok := fileIDFromInfo(info)
		if !ok {
			t.Errorf("no FileID for %s", s)
			continue
		}
		if _, in := p.Library[id]; !in {
			t.Errorf("%s is not an orphan by identity yet is not in the library either", s)
			continue
		}
		wouldDelete++
	}

	if wouldDelete == 0 {
		t.Error("string matching over-reported nothing: the premise behind identity matching is unverified against this collection")
	}
	t.Logf("identity: %d orphans; string: %d; %d in-library files string matching would delete",
		len(p.Orphans), len(stringOrphans), wouldDelete)
}

// orphansByString lists orphans the naive way, comparing decoded path strings.
// It exists only to demonstrate how many in-library files that approach deletes.
func orphansByString(musicDir string, inLibrary map[string]bool) ([]string, error) {
	var orphans []string
	err := filepath.WalkDir(musicDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || strings.ToLower(filepath.Ext(path)) != ".mp3" {
			return nil
		}
		if !inLibrary[path] {
			orphans = append(orphans, path)
		}
		return nil
	})
	return orphans, err
}
