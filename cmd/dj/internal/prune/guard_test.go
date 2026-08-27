package prune

import (
	"errors"
	"strings"
	"testing"

	"github.com/aronbirkir/djtools/cmd/dj/internal/rekordbox"
)

// healthyPlan is a plan that trips no guard, so each test can break exactly one
// thing.
func healthyPlan() *Plan {
	p := &Plan{
		MusicDir:   "/Users/aron/DJ/music",
		Library:    make(map[FileID][]string),
		StaleDupes: make(map[string]int),
		Keepers:    8355,
		Orphans:    make([]string, 6657),
	}
	for i := range 8355 {
		p.Library[FileID{Dev: 1, Ino: uint64(i + 1)}] = []string{"/x"}
	}
	return p
}

func healthyCollection() *rekordbox.Collection {
	return &rekordbox.Collection{
		HasCollection:   true,
		HasPlaylists:    true,
		DeclaredEntries: 8617,
		Tracks:          make([]rekordbox.Track, 8617),
	}
}

func defaultGuardOptions() GuardOptions {
	return GuardOptions{
		MaxOrphanPct: DefaultMaxOrphanPct,
		MinLibrary:   DefaultMinLibrary,
	}
}

func findingsText(f []Finding) string {
	var b strings.Builder
	for _, x := range f {
		b.WriteString(x.Message)
		b.WriteString("\n")
	}
	return b.String()
}

func TestCheckHealthyPlanHasNoAborts(t *testing.T) {
	f := Check(healthyPlan(), healthyCollection(), defaultGuardOptions())
	if Aborts(f) {
		t.Errorf("Aborts = true for a healthy plan:\n%s", findingsText(f))
	}
}

func TestCheckAborts(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*Plan, *rekordbox.Collection, *GuardOptions)
		wantSub string
	}{
		{
			name: "no collection element",
			mutate: func(_ *Plan, c *rekordbox.Collection, _ *GuardOptions) {
				c.HasCollection = false
			},
			wantSub: "no COLLECTION element",
		},
		{
			name: "zero tracks",
			mutate: func(_ *Plan, c *rekordbox.Collection, _ *GuardOptions) {
				c.Tracks = nil
				c.DeclaredEntries = 0
			},
			wantSub: "zero tracks",
		},
		{
			name: "truncated export",
			mutate: func(_ *Plan, c *rekordbox.Collection, _ *GuardOptions) {
				c.Tracks = make([]rekordbox.Track, 400)
			},
			wantSub: "truncated export",
		},
		{
			name: "no playlists element",
			mutate: func(_ *Plan, c *rekordbox.Collection, _ *GuardOptions) {
				c.HasPlaylists = false
			},
			wantSub: "no PLAYLISTS element",
		},
		{
			name: "library too small",
			mutate: func(p *Plan, _ *rekordbox.Collection, _ *GuardOptions) {
				p.Library = map[FileID][]string{{Dev: 1, Ino: 1}: {"/x"}}
				p.Keepers = 1
			},
			wantSub: "looks like a playlist export",
		},
		{
			name: "orphan share above limit",
			mutate: func(p *Plan, _ *rekordbox.Collection, _ *GuardOptions) {
				p.Keepers = 1000
				p.Orphans = make([]string, 9000)
			},
			wantSub: "above the 60% limit",
		},
		{
			name: "unresolved stat error",
			mutate: func(p *Plan, _ *rekordbox.Collection, _ *GuardOptions) {
				p.Unresolved = []error{errors.New("permission denied")}
			},
			wantSub: "library set is incomplete",
		},
		{
			// A systemic decoding failure would quietly shrink the library and
			// inflate the orphan list rather than erroring anywhere.
			name: "too many undecodable locations",
			mutate: func(p *Plan, _ *rekordbox.Collection, _ *GuardOptions) {
				p.NonFile = maxNonFileEntries + 1
			},
			wantSub: "Location format may have changed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p, c, opts := healthyPlan(), healthyCollection(), defaultGuardOptions()
			tt.mutate(p, c, &opts)

			f := Check(p, c, opts)
			if !Aborts(f) {
				t.Fatalf("Aborts = false, want true:\n%s", findingsText(f))
			}
			if got := findingsText(f); !strings.Contains(got, tt.wantSub) {
				t.Errorf("findings do not mention %q:\n%s", tt.wantSub, got)
			}
		})
	}
}

func TestCheckWarnings(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*Plan, *GuardOptions)
		wantSub string
	}{
		{
			name: "audio added after the export was skipped",
			mutate: func(p *Plan, _ *GuardOptions) {
				p.RecentlyAdded = []string{"/m/A2026-08/just-downloaded.mp3"}
			},
			wantSub: "skipped rather than trashed",
		},
		{
			name: "case duplicates",
			mutate: func(p *Plan, _ *GuardOptions) {
				p.CaseDupes = [][]string{{"/a.mp3", "/A.mp3"}}
			},
			wantSub: "differing only in case",
		},
		{
			name: "stale duplicates",
			mutate: func(p *Plan, _ *GuardOptions) {
				p.StaleDupes = map[string]int{"e:/music/a.mp3": 6}
			},
			wantSub: "stale Windows paths",
		},
		{
			name: "dead links",
			mutate: func(p *Plan, _ *GuardOptions) {
				p.DeadLinks = []string{"/Users/aron/DJ/music/A2026-05/gone.mp3"}
			},
			wantSub: "no longer exist",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p, opts := healthyPlan(), defaultGuardOptions()
			tt.mutate(p, &opts)

			f := Check(p, healthyCollection(), opts)
			if Aborts(f) {
				t.Errorf("Aborts = true, want warning only:\n%s", findingsText(f))
			}
			if got := findingsText(f); !strings.Contains(got, tt.wantSub) {
				t.Errorf("findings do not mention %q:\n%s", tt.wantSub, got)
			}
		})
	}
}

// The reference collection's first run is 44.3% orphaned, which must clear the
// 60% default rather than needing --force.
func TestCheckReferenceCollectionClearsOrphanLimit(t *testing.T) {
	f := Check(healthyPlan(), healthyCollection(), defaultGuardOptions())
	if got := findingsText(f); strings.Contains(got, "limit") {
		t.Errorf("orphan limit tripped at 44.3%%:\n%s", got)
	}
}

func TestCheckNoAudioOnDiskDoesNotDivideByZero(t *testing.T) {
	p := healthyPlan()
	p.Keepers = 0
	p.Orphans = nil
	if got := p.OrphanPct(); got != 0 {
		t.Errorf("OrphanPct() = %v, want 0", got)
	}
	Check(p, healthyCollection(), defaultGuardOptions()) // must not panic
}

func TestFindingUnforceable(t *testing.T) {
	if !(Finding{Code: CodeUnresolved}).Unforceable() {
		t.Error("an unresolved library path must not be forceable")
	}
	for _, c := range []Code{CodeOrphanShare, CodeTruncatedExport, CodeDeadLinks} {
		if (Finding{Code: c}).Unforceable() {
			t.Errorf("%s should be forceable", c)
		}
	}
}

func TestNonFileLimitScalesWithCollectionSize(t *testing.T) {
	for _, tt := range []struct {
		entries int
		want    int
	}{
		{0, 5},      // floor
		{100, 5},    // floor
		{500, 5},    // 1% = 5
		{2000, 20},  // proportional
		{8636, 50},  // ceiling, the reference collection
		{50000, 50}, // ceiling
	} {
		if got := nonFileLimit(tt.entries); got != tt.want {
			t.Errorf("nonFileLimit(%d) = %d, want %d", tt.entries, got, tt.want)
		}
	}
}

func TestAbortsWithNoFindings(t *testing.T) {
	if Aborts(nil) {
		t.Error("Aborts(nil) = true, want false")
	}
	if Aborts([]Finding{}) {
		t.Error("Aborts(empty) = true, want false")
	}
}

// Check must collect every abort rather than returning at the first. A future
// early return would pass every single-condition test above while hiding the
// rest of what is wrong with an export.
func TestCheckReportsAllSimultaneousAborts(t *testing.T) {
	p, c, opts := healthyPlan(), healthyCollection(), defaultGuardOptions()
	c.HasPlaylists = false
	p.Unresolved = []error{errors.New("permission denied")}

	var aborts int
	for _, f := range Check(p, c, opts) {
		if f.Level == LevelAbort {
			aborts++
		}
	}
	if aborts < 2 {
		t.Fatalf("got %d aborts, want at least 2: Check must not short-circuit", aborts)
	}
}

// Aborts are the reason a run stops, so they must be readable first.
func TestCheckAbortsPrecedeWarnings(t *testing.T) {
	p, c, opts := healthyPlan(), healthyCollection(), defaultGuardOptions()
	c.HasPlaylists = false
	p.DeadLinks = []string{"/m/A2026-05/gone.mp3"}

	var seenWarn bool
	for _, f := range Check(p, c, opts) {
		if f.Level == LevelWarn {
			seenWarn = true
		}
		if f.Level == LevelAbort && seenWarn {
			t.Fatal("an abort finding appeared after a warning finding")
		}
	}
}
