package prune

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/aronbirkir/djtools/internal/rekordbox"
)

// Options configures one prune run. It is the UI-neutral form of the CLI
// flags, shared by the dj command and the desktop app so that both reach the
// Trash through the same checks.
type Options struct {
	XMLPath       string
	MusicDir      string
	Extensions    []string // dotted, e.g. ".mp3"; see ParseExtensions
	MaxOrphanPct  float64
	KeepEmptyDirs bool
}

// ParseExtensions turns a comma-separated list such as "mp3, .wav" into dotted
// extensions, dropping empty items.
func ParseExtensions(list string) []string {
	var exts []string
	for _, e := range strings.Split(list, ",") {
		if e = strings.TrimSpace(e); e != "" {
			if !strings.HasPrefix(e, ".") {
				e = "." + e
			}
			exts = append(exts, e)
		}
	}
	return exts
}

// Result is a completed scan: what would be trashed and whether it may be.
// Nothing in it has touched the filesystem.
type Result struct {
	Options    Options
	Plan       *Plan
	Findings   []Finding
	Collection *rekordbox.Collection
	// XMLModTime is the export's modification time when it was read. Apply
	// compares against it so a plan built from one export is never carried out
	// after rekordbox has written another.
	XMLModTime time.Time
}

var (
	// ErrBlocked means a finding no override may reach, such as an unresolved
	// library path.
	ErrBlocked = errors.New("these findings cannot be overridden")
	// ErrNeedsForce means abort-level findings the user may override, but has
	// not.
	ErrNeedsForce = errors.New("these findings stop the run unless overridden")
	// ErrXMLChanged means the export was rewritten after the scan.
	ErrXMLChanged = errors.New("the rekordbox XML changed after the scan; scan again")
)

// Scan reads the export, walks the music folder and evaluates every guard. It
// never modifies anything, and deliberately does not require the trash utility,
// so a dry run works on a machine that could not delete.
func Scan(opts Options) (*Result, error) {
	// Refuse before reading anything. The plan itself depends on file
	// identity, so without it there is nothing meaningful to report.
	if err := Supported(); err != nil {
		return nil, err
	}
	info, err := os.Stat(opts.XMLPath)
	if err != nil {
		return nil, err
	}
	f, err := os.Open(opts.XMLPath)
	if err != nil {
		return nil, err
	}
	collection, err := rekordbox.Parse(f)
	f.Close()
	if err != nil {
		return nil, err
	}

	// A zero export time would silently disable the added-after-export skip,
	// quietly removing the protection that keeps freshly downloaded music out of
	// the Trash. Refuse rather than proceed with it weakened.
	exportedAt := info.ModTime()
	if exportedAt.IsZero() {
		return nil, fmt.Errorf("%s has no modification time, so files added after "+
			"the export cannot be identified; refusing to run", opts.XMLPath)
	}

	plan, err := Build(collection, opts.MusicDir, opts.Extensions, exportedAt)
	if err != nil {
		return nil, err
	}
	findings := Check(plan, collection, GuardOptions{
		MaxOrphanPct: opts.MaxOrphanPct,
		MinLibrary:   DefaultMinLibrary,
	})
	return &Result{
		Options:    opts,
		Plan:       plan,
		Findings:   findings,
		Collection: collection,
		XMLModTime: exportedAt,
	}, nil
}

// Blocked returns the abort-level findings that no override may reach.
func (r *Result) Blocked() []Finding {
	var out []Finding
	for _, f := range r.Findings {
		if f.Level == LevelAbort && f.Unforceable() {
			out = append(out, f)
		}
	}
	return out
}

// Forceable returns the abort-level findings an explicit override may reach.
func (r *Result) Forceable() []Finding {
	var out []Finding
	for _, f := range r.Findings {
		if f.Level == LevelAbort && !f.Unforceable() {
			out = append(out, f)
		}
	}
	return out
}

// CanApply reports whether this result may be trashed, given whether the user
// has explicitly overridden the forceable findings.
func (r *Result) CanApply(force bool) error {
	if b := r.Blocked(); len(b) > 0 {
		return fmt.Errorf("%w: %s", ErrBlocked, findingMessages(b))
	}
	if f := r.Forceable(); len(f) > 0 && !force {
		return fmt.Errorf("%w: %s", ErrNeedsForce, findingMessages(f))
	}
	return nil
}

func findingMessages(findings []Finding) string {
	msgs := make([]string, len(findings))
	for i, f := range findings {
		msgs[i] = f.Message
	}
	return strings.Join(msgs, "; ")
}

// ApplyResult is what a completed Apply did. A partial trash failure is not an
// error: files already moved stay moved, and the failures are listed here.
type ApplyResult struct {
	Moved, Total int
	TrashErrs    []error
	RemovedDirs  []string
	DirErrs      []error
	ReportPath   string
}

// Apply moves the result's orphans to the Trash.
//
// It re-checks every gate itself rather than trusting its caller: a UI that
// forgot to disable a button, or a CLI path that skipped a check, must still
// be unable to trash what the guards refused. It re-evaluates the guards from
// the plan itself rather than trusting r.Findings, so a caller that altered
// Findings still cannot get past them. If reportPath is set, the report is
// written first and a failure to write it stops the run before anything
// moves, since the report is what makes a restore precise.
func Apply(r *Result, runner Runner, force bool, reportPath string) (*ApplyResult, error) {
	if err := TrashAvailable(); err != nil {
		return nil, err
	}
	if r == nil || r.Plan == nil || r.Collection == nil {
		return nil, errors.New("no completed scan to apply")
	}
	fresh := *r
	fresh.Findings = Check(r.Plan, r.Collection, GuardOptions{
		MaxOrphanPct: r.Options.MaxOrphanPct,
		MinLibrary:   DefaultMinLibrary,
	})
	if err := fresh.CanApply(force); err != nil {
		return nil, err
	}
	info, err := os.Stat(r.Options.XMLPath)
	if err != nil {
		return nil, fmt.Errorf("re-checking %s: %w", r.Options.XMLPath, err)
	}
	if !info.ModTime().Equal(r.XMLModTime) {
		return nil, ErrXMLChanged
	}
	if reportPath != "" {
		if err := WriteReportFile(reportPath, r.Plan); err != nil {
			return nil, fmt.Errorf("writing the report, so nothing was moved: %w", err)
		}
	}

	res := &ApplyResult{Total: len(r.Plan.Orphans), ReportPath: reportPath}
	res.Moved, res.TrashErrs = Trash(runner, r.Plan.MusicDir, r.Plan.Orphans)
	if !r.Options.KeepEmptyDirs {
		res.RemovedDirs, res.DirErrs = RemoveEmptyDirs(r.Plan.MusicDir)
	}
	return res, nil
}
