package prune

import (
	"fmt"

	"github.com/aronbirkir/djtools/cmd/dj/internal/rekordbox"
)

// Level says whether a finding stops the run.
type Level int

const (
	LevelWarn Level = iota
	LevelAbort
)

// Finding is one guard result.
type Finding struct {
	Level   Level
	Message string
}

// GuardOptions holds the tunable thresholds.
type GuardOptions struct {
	// MaxOrphanPct is the share of on-disk audio above which the run stops.
	MaxOrphanPct float64
	// MinLibrary is the smallest resolvable library that still looks like a
	// full collection rather than a single playlist.
	MinLibrary int
}

// Note there is no export timestamp here. Build already consumes it to compute
// Plan.RecentlyAdded, and Check reads that consequence instead. Passing the same
// time to both would create two places for it to be wrong.

const (
	// DefaultMaxOrphanPct bounds how much of the disk one run may remove. The
	// first run on the reference collection is 44.3%, so this leaves headroom
	// while still catching a wrong export.
	DefaultMaxOrphanPct = 60.0
	// DefaultMinLibrary distinguishes a collection export from a playlist
	// export, which would orphan almost everything.
	DefaultMinLibrary = 500
	// maxNonFileEntries is how many undecodable Locations are plausible as real
	// streaming tracks. The reference export has 2. Set high enough that adding
	// Tidal tracks never trips it, low enough that a systemic decoding failure
	// does.
	maxNonFileEntries = 50
)

// Check evaluates every guard. An abort-level finding means the XML is not a
// trustworthy picture of the library and nothing should be deleted.
func Check(p *Plan, c *rekordbox.Collection, opts GuardOptions) []Finding {
	var findings []Finding
	abort := func(format string, a ...any) {
		findings = append(findings, Finding{LevelAbort, fmt.Sprintf(format, a...)})
	}
	warn := func(format string, a ...any) {
		findings = append(findings, Finding{LevelWarn, fmt.Sprintf(format, a...)})
	}

	if !c.HasCollection {
		abort("no COLLECTION element: this is not a rekordbox collection export")
	}
	if len(c.Tracks) == 0 {
		abort("the collection contains zero tracks")
	}
	if c.HasCollection && len(c.Tracks) > 0 && c.DeclaredEntries != len(c.Tracks) {
		abort("truncated export: COLLECTION declares %d entries but %d TRACK elements parsed",
			c.DeclaredEntries, len(c.Tracks))
	}
	if !c.HasPlaylists {
		abort("no PLAYLISTS element: the export looks incomplete")
	}
	if n := len(p.Library); n < opts.MinLibrary {
		abort("only %d library files resolved under %s (minimum %d): this looks like a playlist export, not a collection",
			n, p.MusicDir, opts.MinLibrary)
	}
	if pct := p.OrphanPct(); pct > opts.MaxOrphanPct {
		abort("%d of the %d files this export accounts for (%.1f%%) have no library entry, "+
			"above the %.0f%% limit",
			len(p.Orphans), p.EligibleAudio(), pct, opts.MaxOrphanPct)
	}
	for _, err := range p.Unresolved {
		abort("the library set is incomplete, so nothing can be deleted: %v", err)
	}
	// A Location that yields no usable path is normally a streaming entry, and
	// the reference export has exactly 2. A sudden crop of them means rekordbox
	// changed the Location format and decoding is failing systemically, which
	// would quietly shrink the library and inflate the orphan list. The
	// small-library and orphan-share guards would catch a total failure, but not
	// reliably a partial one, and neither would say what actually went wrong.
	if n := p.NonFile; n > maxNonFileEntries {
		abort("%d library entries yielded no usable path (expected a handful of streaming tracks): the Location format may have changed and decoding is failing", n)
	}

	if n := len(p.RecentlyAdded); n > 0 {
		warn("%d audio files are newer than this export and were skipped rather than trashed, "+
			"since they could not have appeared in it; re-export from rekordbox to have them considered", n)
	}
	if n := len(p.CaseDupes); n > 0 {
		warn("%d files have more than one library entry differing only in case; worth merging in rekordbox", n)
	}
	if n := len(p.StaleDupes); n > 0 {
		warn("%d stale Windows paths appear under more than one TrackID", n)
	}
	if n := len(p.DeadLinks); n > 0 {
		warn("%d library entries point at files that no longer exist under %s", n, p.MusicDir)
	}

	return findings
}

// Aborts reports whether any finding is abort-level.
func Aborts(findings []Finding) bool {
	for _, f := range findings {
		if f.Level == LevelAbort {
			return true
		}
	}
	return false
}
