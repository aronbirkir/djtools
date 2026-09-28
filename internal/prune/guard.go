package prune

import (
	"fmt"

	"github.com/aronbirkir/djtools/internal/rekordbox"
)

// Code identifies a finding independently of its wording, so callers can act on
// a specific finding without matching prose. Task 8's --force needs this to
// distinguish a finding it may override from one it must not.
type Code string

const (
	CodeNoCollection    Code = "no-collection"
	CodeZeroTracks      Code = "zero-tracks"
	CodeTruncatedExport Code = "truncated-export"
	CodeNoPlaylists     Code = "no-playlists"
	CodeLibraryTooSmall Code = "library-too-small"
	CodeOrphanShare     Code = "orphan-share"
	CodeUnresolved      Code = "unresolved"
	CodeUndecodable     Code = "undecodable-locations"
	CodeRecentlyAdded   Code = "recently-added"
	CodeCaseDupes       Code = "case-duplicates"
	CodeStaleDupes      Code = "stale-duplicates"
	CodeDeadLinks       Code = "dead-links"
)

// Level says whether a finding stops the run.
type Level int

const (
	LevelWarn Level = iota
	LevelAbort
)

// Finding is one guard result.
type Finding struct {
	Code    Code
	Level   Level
	Message string
}

// Unforceable reports whether --force must not be allowed to override this
// finding.
//
// An unresolved library path means we could not establish what is in the
// library. Deleting on that basis is the precise failure this design exists to
// prevent, so no flag reaches past it -- unlike, say, an unusually high orphan
// share, which a user may legitimately know to be correct.
func (f Finding) Unforceable() bool {
	return f.Code == CodeUnresolved
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
	// maxNonFileEntries caps how many undecodable Locations are plausible as
	// genuine streaming entries. The reference export has 2.
	maxNonFileEntries = 50
	// minNonFileEntries floors the proportional threshold so a small library
	// does not abort over a couple of legitimately-added Tidal tracks.
	minNonFileEntries = 5
)

// nonFileLimit is how many undecodable Locations to tolerate before treating
// them as a decoding failure rather than streaming entries.
//
// Proportional with a floor and a ceiling, because a fixed count inverts with
// library size: 50 is ample headroom against 8,636 entries but would let 10% of
// a 500-entry library fail silently. One percent of the collection, clamped to
// [5, 50], keeps sensitivity roughly constant in relative terms.
func nonFileLimit(declaredEntries int) int {
	return max(minNonFileEntries, min(maxNonFileEntries, declaredEntries/100))
}

// Check evaluates every guard. An abort-level finding means the XML is not a
// trustworthy picture of the library and nothing should be deleted.
func Check(p *Plan, c *rekordbox.Collection, opts GuardOptions) []Finding {
	var findings []Finding
	abort := func(code Code, format string, a ...any) {
		findings = append(findings, Finding{code, LevelAbort, fmt.Sprintf(format, a...)})
	}
	warn := func(code Code, format string, a ...any) {
		findings = append(findings, Finding{code, LevelWarn, fmt.Sprintf(format, a...)})
	}

	if !c.HasCollection {
		abort(CodeNoCollection, "no COLLECTION element: this is not a rekordbox collection export")
	}
	if len(c.Tracks) == 0 {
		abort(CodeZeroTracks, "the collection contains zero tracks")
	}
	if c.HasCollection && len(c.Tracks) > 0 && c.DeclaredEntries != len(c.Tracks) {
		abort(CodeTruncatedExport, "truncated export: COLLECTION declares %d entries but %d TRACK elements parsed",
			c.DeclaredEntries, len(c.Tracks))
	}
	if !c.HasPlaylists {
		abort(CodeNoPlaylists, "no PLAYLISTS element: the export looks incomplete")
	}
	if n := len(p.Library); n < opts.MinLibrary {
		abort(CodeLibraryTooSmall, "only %d library files resolved under %s (minimum %d): this looks like a playlist export, not a collection",
			n, p.MusicDir, opts.MinLibrary)
	}
	if pct := p.OrphanPct(); pct > opts.MaxOrphanPct {
		abort(CodeOrphanShare, "%d of the %d files this export accounts for (%.1f%%) have no library entry, "+
			"above the %.0f%% limit",
			len(p.Orphans), p.EligibleAudio(), pct, opts.MaxOrphanPct)
	}
	for _, err := range p.Unresolved {
		abort(CodeUnresolved, "the library set is incomplete, so nothing can be deleted: %v", err)
	}
	// A Location that yields no usable path is normally a streaming entry, and
	// the reference export has exactly 2. A sudden crop of them means rekordbox
	// changed the Location format and decoding is failing systemically, which
	// would quietly shrink the library and inflate the orphan list. The
	// small-library and orphan-share guards would catch a total failure, but not
	// reliably a partial one, and neither would say what actually went wrong.
	if limit := nonFileLimit(c.DeclaredEntries); p.NonFile > limit {
		abort(CodeUndecodable,
			"%d library entries yielded no usable path, above the %d tolerated for a "+
				"collection of %d (expected a handful of streaming tracks): the Location "+
				"format may have changed and decoding is failing",
			p.NonFile, limit, c.DeclaredEntries)
	}

	if n := len(p.RecentlyAdded); n > 0 {
		share := float64(n) / float64(max(1, p.OnDiskAudio())) * 100
		warn(CodeRecentlyAdded,
			"%d audio files (%.1f%% of the collection) are newer than this export and were "+
				"skipped rather than trashed, since they could not have appeared in it; "+
				"re-export from rekordbox to have them considered", n, share)
	}
	if n := len(p.CaseDupes); n > 0 {
		warn(CodeCaseDupes, "%d files have more than one library entry differing only in case; worth merging in rekordbox", n)
	}
	if n := len(p.StaleDupes); n > 0 {
		warn(CodeStaleDupes, "%d stale Windows paths appear under more than one TrackID", n)
	}
	if n := len(p.DeadLinks); n > 0 {
		warn(CodeDeadLinks, "%d library entries point at files that no longer exist under %s", n, p.MusicDir)
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
