package prune

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/aronbirkir/djtools/cmd/dj/internal/rekordbox"
)

// Plan is the complete picture of one prune run, computed before anything is
// deleted. Build never mutates the filesystem, so a Plan can always be
// inspected and discarded.
type Plan struct {
	MusicDir string

	// Library maps each resolved library file to every Location spelling that
	// pointed at it. The value is a slice rather than a single string so that
	// duplicate rekordbox entries are surfaced instead of silently collapsed.
	Library map[FileID][]string

	// Orphans is audio on disk with no library entry: exactly what gets
	// trashed. Sorted.
	Orphans    []string
	OrphanSize int64
	Keepers    int

	// RecentlyAdded is audio with no library entry whose mtime is newer than
	// the export. Such a file cannot possibly appear in that XML, so its
	// absence proves nothing and it is never trashed -- it is reported and
	// skipped. This is not hypothetical: 7 tracks were added to the reference
	// collection 40 minutes after its export, and without this they would have
	// been trashed. If one really was removed from the library, the next run
	// against a fresh export prunes it.
	RecentlyAdded []string

	DeadLinks  []string       // library entries under MusicDir whose file is gone
	Stale      []string       // unique Windows leftover paths
	StaleDupes map[string]int // stale path -> entry count, only where count > 1
	CaseDupes  [][]string     // one file, several Location spellings
	Leftovers  []string       // non-audio files under MusicDir, reported only
	Symlinks   []string       // never followed, never trashed
	Foreign    int            // real paths outside MusicDir
	NonFile    int            // streaming entries

	// Unresolved holds stat failures that were not ENOENT. Any entry here means
	// the library set is incomplete and nothing may be deleted.
	Unresolved []error

	// NewestAudio is the newest mod time seen on disk, used to warn about a
	// stale export.
	NewestAudio time.Time

	FolderTotals  map[string]int   // top-level folder -> on-disk audio count
	FolderOrphans map[string]int   // top-level folder -> orphan count
	FolderBytes   map[string]int64 // top-level folder -> reclaimable bytes
}

// OnDiskAudio is every audio file walked, and the figure to report as the
// collection's total footprint.
func (p *Plan) OnDiskAudio() int {
	return p.Keepers + len(p.Orphans) + len(p.RecentlyAdded)
}

// EligibleAudio is the audio this export could plausibly account for: keepers
// plus orphans. Files added after the export was written are excluded, since it
// could not have described them either way.
func (p *Plan) EligibleAudio() int {
	return p.Keepers + len(p.Orphans)
}

// OrphanPct is the share of eligible audio that would be trashed, and the
// quantity the orphan-share guard thresholds on.
//
// RecentlyAdded is deliberately absent from the denominator. Including it would
// dilute the percentage in proportion to how much un-exported new music happened
// to be sitting on disk, making a run look more trustworthy precisely when the
// collection is most in flux. With 500 tracks added after the export, this
// collection's 6,645 orphans would read 42.8% rather than 44.2% -- movement that
// says nothing about whether the export is trustworthy.
func (p *Plan) OrphanPct() float64 {
	total := p.EligibleAudio()
	if total == 0 {
		return 0
	}
	return float64(len(p.Orphans)) / float64(total) * 100
}

// Build resolves the library against the filesystem and computes what would be
// removed. It never modifies anything.
//
// Files are matched by (device, inode), not by path string. rekordbox writes NFC
// while APFS returns whatever form was stored, and the volume is
// case-insensitive, so string comparison reports in-library files as orphans.
// exportedAt is when the XML was written. A zero value means unknown, which
// disables the added-after-export skip.
func Build(c *rekordbox.Collection, musicDir string, exts []string, exportedAt time.Time) (*Plan, error) {
	// Resolved absolute, not merely cleaned. Every path in the Plan derives from
	// this one, and Trash refuses a relative path outright -- so a relative
	// --music, which is the documented default, would otherwise leave the tool
	// refusing its own orphan list.
	musicDir, err := filepath.Abs(musicDir)
	if err != nil {
		return nil, fmt.Errorf("resolving music directory %q: %w", musicDir, err)
	}
	p := &Plan{
		MusicDir:      musicDir,
		Library:       make(map[FileID][]string),
		StaleDupes:    make(map[string]int),
		FolderTotals:  make(map[string]int),
		FolderOrphans: make(map[string]int),
		FolderBytes:   make(map[string]int64),
	}
	p.resolveLibrary(c)
	if err := p.walkDisk(exts, exportedAt); err != nil {
		return nil, err
	}
	p.finalize()
	return p, nil
}

// resolveLibrary stats every local Location and records the identity of each
// file that exists.
func (p *Plan) resolveLibrary(c *rekordbox.Collection) {
	seenLocal := make(map[string]bool)
	seenStale := make(map[string]bool)

	// Resolved once: which paths are reachable depends on the running OS, and
	// it cannot change mid-run.
	host := rekordbox.CurrentHost()

	for _, t := range c.Tracks {
		loc := rekordbox.ClassifyLocation(t.Location, p.MusicDir, host)
		switch loc.Kind {
		case rekordbox.KindStale:
			p.StaleDupes[loc.Path]++
			if !seenStale[loc.Path] {
				seenStale[loc.Path] = true
				p.Stale = append(p.Stale, loc.Path)
			}
		case rekordbox.KindForeign:
			p.Foreign++
		case rekordbox.KindNonFile:
			p.NonFile++
		case rekordbox.KindLocal:
			if seenLocal[loc.Path] {
				continue // byte-identical duplicate entry, already statted
			}
			seenLocal[loc.Path] = true
			p.resolveLocal(loc.Path)
		}
	}
}

// resolveLocal stats one library path and records what it found. It is separate
// from resolveLibrary because this is where "uncertainty means keep" is actually
// enforced, and that decision is worth reading on its own.
//
// A missing file is a dead link: the library simply points at something the user
// deleted. Any other failure means we could not determine what is in the
// library, which must stop the run rather than licensing a deletion.
func (p *Plan) resolveLocal(path string) {
	info, err := os.Stat(path)
	switch {
	case err == nil:
		id, ok := fileIDFromInfo(info)
		if !ok {
			p.Unresolved = append(p.Unresolved,
				fmt.Errorf("no stat data for library file %s", path))
			return
		}
		p.Library[id] = append(p.Library[id], path)
	case errors.Is(err, fs.ErrNotExist):
		p.DeadLinks = append(p.DeadLinks, path)
	default:
		// Permissions, I/O errors, a detached volume, a path component that is
		// not a directory: we cannot prove what is in the library.
		p.Unresolved = append(p.Unresolved,
			fmt.Errorf("stat library file %s: %w", path, err))
	}
}

// walkDisk classifies every file under MusicDir against the resolved library.
func (p *Plan) walkDisk(exts []string, exportedAt time.Time) error {
	audio := make(map[string]bool, len(exts))
	for _, e := range exts {
		audio[strings.ToLower(e)] = true
	}

	return filepath.WalkDir(p.MusicDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			// An unreadable directory would make everything under it look
			// orphaned. Refuse to guess.
			return fmt.Errorf("walk %s: %w", path, err)
		}
		if d.IsDir() {
			return nil
		}

		// WalkDir does not follow symlinks, so Info is lstat data.
		info, err := d.Info()
		if err != nil {
			return fmt.Errorf("stat %s: %w", path, err)
		}
		if info.Mode()&fs.ModeSymlink != 0 {
			p.Symlinks = append(p.Symlinks, path)
			return nil
		}
		// Sockets, FIFOs and device files are ignored entirely rather than
		// reported as leftovers. Nothing under a music folder should be one, and
		// they are never candidates for trashing.
		if !info.Mode().IsRegular() {
			return nil
		}
		if !audio[strings.ToLower(filepath.Ext(path))] {
			p.Leftovers = append(p.Leftovers, path)
			return nil
		}

		if mod := info.ModTime(); mod.After(p.NewestAudio) {
			p.NewestAudio = mod
		}

		folder := topFolder(p.MusicDir, path)
		p.FolderTotals[folder]++

		id, ok := fileIDFromInfo(info)
		if !ok {
			return fmt.Errorf("no stat data for %s", path)
		}
		if _, inLibrary := p.Library[id]; inLibrary {
			p.Keepers++
			return nil
		}

		// A file newer than the export could not have been in it, so its
		// absence from the library is not evidence. Never trash it.
		if !exportedAt.IsZero() && info.ModTime().After(exportedAt) {
			p.RecentlyAdded = append(p.RecentlyAdded, path)
			return nil
		}

		p.Orphans = append(p.Orphans, path)
		p.OrphanSize += info.Size()
		p.FolderOrphans[folder]++
		p.FolderBytes[folder] += info.Size()
		return nil
	})
}

// finalize derives the duplicate lists and sorts every reported slice so output
// is stable between runs.
func (p *Plan) finalize() {
	// Sorting here reorders the slices stored in p.Library itself, not copies.
	// That is intended -- CaseDupes must be deterministic -- but it means
	// Library's order does not reflect which Location the XML declared first.
	for _, spellings := range p.Library {
		if len(spellings) > 1 {
			sort.Strings(spellings)
			p.CaseDupes = append(p.CaseDupes, spellings)
		}
	}
	sort.Slice(p.CaseDupes, func(i, j int) bool {
		return p.CaseDupes[i][0] < p.CaseDupes[j][0]
	})

	// Only repeated stale paths are interesting.
	for path, n := range p.StaleDupes {
		if n < 2 {
			delete(p.StaleDupes, path)
		}
	}

	sort.Strings(p.Orphans)
	sort.Strings(p.RecentlyAdded)
	sort.Strings(p.DeadLinks)
	sort.Strings(p.Stale)
	sort.Strings(p.Leftovers)
	sort.Strings(p.Symlinks)
}

// topFolder is the first path element below musicDir. That is the unit the
// collection is organised in: genre folders such as "Dance" and dated import
// batches such as "A2026-01".
func topFolder(musicDir, path string) string {
	rel, err := filepath.Rel(musicDir, path)
	if err != nil {
		return "."
	}
	if i := strings.IndexByte(rel, filepath.Separator); i >= 0 {
		return rel[:i]
	}
	return "."
}
