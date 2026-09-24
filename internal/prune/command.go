package prune

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/aronbirkir/djtools/internal/rekordbox"
)

// Exit codes. These are the tool's contract with any script wrapping it.
const (
	exitOK      = 0
	exitStopped = 1 // guard abort, declined confirmation, or a usage error
	exitPartial = 2 // files were trashed but some batches failed
)

// defaultExtensions covers what rekordbox can hold. Only .mp3 exists in the
// reference collection, but the Sampler entries are .wav.
const defaultExtensions = ".mp3,.wav,.aiff,.flac,.m4a"

type options struct {
	xmlPath       string
	musicDir      string
	extensions    string
	dryRun        bool
	assumeYes     bool
	force         bool
	maxOrphanPct  float64
	list          bool
	reportPath    string
	keepEmptyDirs bool
}

// Run executes the prune subcommand and returns a process exit code.
func Run(args []string, stdout, stderr io.Writer, stdin io.Reader) int {
	var opts options
	fs := flag.NewFlagSet("prune", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.StringVar(&opts.xmlPath, "xml", "rekordbox.xml", "rekordbox XML export")
	fs.StringVar(&opts.musicDir, "music", "music", "music folder to prune")
	fs.StringVar(&opts.extensions, "ext", defaultExtensions, "comma-separated audio extensions")
	fs.BoolVar(&opts.dryRun, "dry-run", false, "scan and report, never trash")
	fs.BoolVar(&opts.assumeYes, "yes", false, "skip the confirmation prompt")
	fs.BoolVar(&opts.force, "force", false, "proceed despite abort-level guard findings")
	fs.Float64Var(&opts.maxOrphanPct, "max-orphan-pct", DefaultMaxOrphanPct,
		"abort when orphans exceed this share of on-disk audio")
	fs.BoolVar(&opts.list, "list", false, "print every orphan path")
	fs.StringVar(&opts.reportPath, "report", "", "write the full lists to this file")
	fs.BoolVar(&opts.keepEmptyDirs, "keep-empty-dirs", false, "do not remove emptied directories")

	if err := fs.Parse(args); err != nil {
		// -h/--help is a successful request for help, not a failure: the flag
		// package has already printed usage and returns ErrHelp.
		if errors.Is(err, flag.ErrHelp) {
			return exitOK
		}
		return exitStopped
	}

	// Refuse before reading anything. This gates --dry-run too: the plan itself
	// depends on file identity, so without it there is nothing meaningful to
	// report, let alone delete.
	if err := Supported(); err != nil {
		fmt.Fprintf(stderr, "dj prune: %v\n", err)
		return exitStopped
	}

	// Fail before doing any work, and well before prompting, if we cannot
	// actually move files to the Trash.
	if !opts.dryRun {
		if err := TrashAvailable(); err != nil {
			fmt.Fprintf(stderr, "dj prune: %v\n", err)
			return exitStopped
		}
	}

	xmlInfo, err := os.Stat(opts.xmlPath)
	if err != nil {
		fmt.Fprintf(stderr, "dj prune: %v\n", err)
		return exitStopped
	}
	f, err := os.Open(opts.xmlPath)
	if err != nil {
		fmt.Fprintf(stderr, "dj prune: %v\n", err)
		return exitStopped
	}
	collection, err := rekordbox.Parse(f)
	f.Close()
	if err != nil {
		fmt.Fprintf(stderr, "dj prune: %v\n", err)
		return exitStopped
	}

	var exts []string
	for _, e := range strings.Split(opts.extensions, ",") {
		if e = strings.TrimSpace(e); e != "" {
			if !strings.HasPrefix(e, ".") {
				e = "." + e
			}
			exts = append(exts, e)
		}
	}

	// A zero export time would silently disable the added-after-export skip,
	// quietly removing the protection that keeps freshly downloaded music out of
	// the Trash. Refuse rather than proceed with it weakened.
	exportedAt := xmlInfo.ModTime()
	if exportedAt.IsZero() {
		fmt.Fprintf(stderr, "dj prune: %s has no modification time, so files added after "+
			"the export cannot be identified; refusing to run\n", opts.xmlPath)
		return exitStopped
	}

	plan, err := Build(collection, opts.musicDir, exts, exportedAt)
	if err != nil {
		fmt.Fprintf(stderr, "dj prune: %v\n", err)
		return exitStopped
	}

	findings := Check(plan, collection, GuardOptions{
		MaxOrphanPct: opts.maxOrphanPct,
		MinLibrary:   DefaultMinLibrary,
	})

	if err := Summary(stdout, plan, findings); err != nil {
		fmt.Fprintf(stderr, "dj prune: %v\n", err)
		return exitStopped
	}
	if opts.list {
		for _, p := range plan.Orphans {
			fmt.Fprintln(stdout, p)
		}
	}
	if opts.reportPath != "" {
		if err := writeReportFile(opts.reportPath, plan); err != nil {
			fmt.Fprintf(stderr, "dj prune: %v\n", err)
			return exitStopped
		}
		fmt.Fprintf(stdout, "\nfull lists written to %s\n", opts.reportPath)
	}

	if Aborts(findings) {
		// --force covers findings the user may legitimately know to be wrong,
		// such as an unusually high orphan share. It deliberately does not reach
		// an unresolved library path: that means we could not establish what is
		// in the library, and deleting on that basis is the exact failure this
		// tool exists to prevent.
		var blocked []Finding
		for _, f := range findings {
			if f.Level == LevelAbort && f.Unforceable() {
				blocked = append(blocked, f)
			}
		}
		switch {
		case len(blocked) > 0:
			fmt.Fprintln(stdout, "\nStopping. These cannot be overridden with --force:")
			for _, f := range blocked {
				fmt.Fprintf(stdout, "  %s\n", f.Message)
			}
			return exitStopped
		case !opts.force:
			fmt.Fprintln(stdout, "\nStopping. Re-run with --force to override, or re-export from rekordbox.")
			return exitStopped
		}
	}

	if len(plan.Orphans) == 0 {
		return exitOK
	}
	if opts.dryRun {
		fmt.Fprintf(stdout, "\nThis was a dry run. Nothing was moved.\n")
		return exitOK
	}
	if !opts.assumeYes && !confirm(stdin, stdout, len(plan.Orphans), plan.OrphanSize) {
		fmt.Fprintln(stdout, "Nothing was moved.")
		return exitStopped
	}

	done, trashErrs := Trash(ExecRunner{}, plan.MusicDir, plan.Orphans)
	fmt.Fprintf(stdout, "\nMoved %d of %d files to the Trash.\n", done, len(plan.Orphans))
	for _, err := range trashErrs {
		fmt.Fprintf(stderr, "dj prune: %v\n", err)
	}

	if !opts.keepEmptyDirs {
		removed, dirErrs := RemoveEmptyDirs(plan.MusicDir)
		fmt.Fprintf(stdout, "Removed %d empty directories.\n", len(removed))
		for _, err := range dirErrs {
			fmt.Fprintf(stderr, "dj prune: %v\n", err)
		}
	}

	if len(trashErrs) > 0 {
		return exitPartial
	}
	return exitOK
}

func writeReportFile(path string, p *Plan) error {
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

// confirm asks once. Anything other than an explicit yes means no, so a stray
// newline or a closed stdin can never authorise a deletion.
func confirm(in io.Reader, out io.Writer, count int, size int64) bool {
	fmt.Fprintf(out, "\nMove %d files (%s) to the Trash? [y/N] ", count, humanBytes(size))
	line, err := bufio.NewReader(in).ReadString('\n')
	if err != nil && line == "" {
		fmt.Fprintln(out)
		return false
	}
	switch strings.ToLower(strings.TrimSpace(line)) {
	case "y", "yes":
		return true
	}
	return false
}
