package prune

import (
	"fmt"
	"io"
	"os"
	"sort"
	"text/tabwriter"
)

// maxFolderRows caps the per-folder table. The reference collection has 110
// folders holding orphans, and the tail is long and uninformative: 54 of them
// account for 88 files between them, while the top 25 cover 91% of the orphans
// and 88% of the reclaimable bytes. Printing every row buries the folders that
// actually matter. The remainder collapses into one row carrying its own totals,
// so nothing is concealed -- and --report still lists every path.
const maxFolderRows = 25

// FoldersByOrphans returns the plan's top-level folders with orphans, most
// affected first, ties broken by name so output is stable.
func FoldersByOrphans(p *Plan) []string {
	folders := make([]string, 0, len(p.FolderOrphans))
	for name := range p.FolderOrphans {
		folders = append(folders, name)
	}
	sort.Slice(folders, func(i, j int) bool {
		a, b := folders[i], folders[j]
		if p.FolderOrphans[a] != p.FolderOrphans[b] {
			return p.FolderOrphans[a] > p.FolderOrphans[b]
		}
		return a < b
	})
	return folders
}

// Summary writes the human-facing report: a per-folder table, totals, and every
// guard finding. It is what the user reads before confirming.
//
// Leftovers and Symlinks are printed here rather than surfaced as Findings.
// Findings are judgments about whether the export can be trusted; these two are
// inventory -- facts about what is on disk, which no guard acts on. The
// consequence worth knowing: Check alone is not a complete account of what to
// tell the user, so a future machine-readable output would need to include these
// explicitly.
func Summary(w io.Writer, p *Plan, findings []Finding) error {
	if len(p.Orphans) == 0 {
		if _, err := fmt.Fprintf(w,
			"%d files on disk, all present in the library: nothing to remove.\n",
			p.OnDiskAudio()); err != nil {
			return err
		}
		return writeFindings(w, findings)
	}

	folders := FoldersByOrphans(p)

	shown := folders
	if len(shown) > maxFolderRows {
		shown = folders[:maxFolderRows]
	}

	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "FOLDER\tORPHANS\tTOTAL\tRECLAIM")
	for _, name := range shown {
		fmt.Fprintf(tw, "%s\t%d\t%d\t%s\n",
			name, p.FolderOrphans[name], p.FolderTotals[name], HumanBytes(p.FolderBytes[name]))
	}
	if rest := folders[len(shown):]; len(rest) > 0 {
		var orphans, total int
		var bytes int64
		for _, name := range rest {
			orphans += p.FolderOrphans[name]
			total += p.FolderTotals[name]
			bytes += p.FolderBytes[name]
		}
		fmt.Fprintf(tw, "(+%d more folders)\t%d\t%d\t%s\n",
			len(rest), orphans, total, HumanBytes(bytes))
	}
	fmt.Fprintf(tw, "TOTAL\t%d\t%d\t%s\n", len(p.Orphans), p.OnDiskAudio(), HumanBytes(p.OrphanSize))
	if err := tw.Flush(); err != nil {
		return err
	}

	if _, err := fmt.Fprintf(w,
		"\n%d files kept, %d orphaned (%.1f%% of the %d files this export accounts for)\n",
		p.Keepers, len(p.Orphans), p.OrphanPct(), p.EligibleAudio()); err != nil {
		return err
	}

	// Without this the TOTAL row (collection-wide) cannot be reconciled against
	// the rows above it, which only cover folders containing orphans.
	if untouched := len(p.FolderTotals) - len(folders); untouched > 0 {
		fmt.Fprintf(w, "%d folders contain no orphans and are untouched\n", untouched)
	}

	if n := len(p.RecentlyAdded); n > 0 {
		fmt.Fprintf(w, "%d files newer than the export, skipped rather than trashed\n", n)
	}
	if n := len(p.Leftovers); n > 0 {
		fmt.Fprintf(w, "%d non-audio files present, reported but never trashed\n", n)
	}
	if n := len(p.Symlinks); n > 0 {
		fmt.Fprintf(w, "%d symlinks present, never followed or trashed\n", n)
	}

	return writeFindings(w, findings)
}

// advisoryCodes are warnings that describe the state of the rekordbox library
// rather than anything about this run. They repeat identically on every run
// until the user edits the library by hand, so they are grouped separately --
// otherwise they become familiar noise, and the warning that does matter
// (files skipped because they are newer than the export) scrolls past inside it.
var advisoryCodes = map[Code]bool{
	CodeCaseDupes:  true,
	CodeStaleDupes: true,
	CodeDeadLinks:  true,
}

func writeFindings(w io.Writer, findings []Finding) error {
	if len(findings) == 0 {
		return nil
	}
	if _, err := fmt.Fprintln(w); err != nil {
		return err
	}
	// Aborts first: they are the reason the run will stop.
	for _, want := range []Level{LevelAbort, LevelWarn} {
		for _, f := range findings {
			if f.Level != want {
				continue
			}
			if f.Level == LevelWarn && advisoryCodes[f.Code] {
				continue // printed below, under their own heading
			}
			label := "WARN"
			if f.Level == LevelAbort {
				label = "ABORT"
			}
			if _, err := fmt.Fprintf(w, "%-5s %s\n", label, f.Message); err != nil {
				return err
			}
		}
	}

	var advisories []Finding
	for _, f := range findings {
		if f.Level == LevelWarn && advisoryCodes[f.Code] {
			advisories = append(advisories, f)
		}
	}
	if len(advisories) > 0 {
		if _, err := fmt.Fprintf(w,
			"\nTo clean up in rekordbox (unchanged until you do, reported every run):\n"); err != nil {
			return err
		}
		for _, f := range advisories {
			if _, err := fmt.Fprintf(w, "  - %s\n", f.Message); err != nil {
				return err
			}
		}
	}
	return nil
}

// Report writes the full lists for --report, one path per line under a section
// header, so the output can be grepped and spot-checked.
func Report(w io.Writer, p *Plan) error {
	sections := []struct {
		title string
		paths []string
	}{
		{"ORPHANS", p.Orphans},
		{"SKIPPED, NEWER THAN EXPORT", p.RecentlyAdded},
		{"DEAD LINKS", p.DeadLinks},
		{"STALE ENTRIES", p.Stale},
		{"LEFTOVERS", p.Leftovers},
		{"SYMLINKS", p.Symlinks},
	}
	for _, s := range sections {
		if _, err := fmt.Fprintf(w, "# %s (%d)\n", s.title, len(s.paths)); err != nil {
			return err
		}
		for _, path := range s.paths {
			if _, err := fmt.Fprintln(w, path); err != nil {
				return err
			}
		}
		fmt.Fprintln(w)
	}

	if _, err := fmt.Fprintf(w, "# CASE DUPLICATES (%d)\n", len(p.CaseDupes)); err != nil {
		return err
	}
	for _, group := range p.CaseDupes {
		for _, path := range group {
			fmt.Fprintln(w, path)
		}
		fmt.Fprintln(w)
	}
	return nil
}

// HumanBytes formats a byte count in decimal units, matching how Finder reports
// disk space.
func HumanBytes(n int64) string {
	const unit = 1000
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	value := float64(n)
	for _, suffix := range []string{"kB", "MB", "GB", "TB"} {
		value /= unit
		if value < unit {
			return fmt.Sprintf("%.1f %s", value, suffix)
		}
	}
	return fmt.Sprintf("%.1f PB", value/unit)
}

// WriteReportFile writes Report to path, creating or truncating it. The file is
// closed and checked before returning, so a nil error means the report is on
// disk -- callers that trash afterwards depend on that.
func WriteReportFile(path string, p *Plan) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	if err := Report(f, p); err != nil {
		return err
	}
	return f.Close()
}
