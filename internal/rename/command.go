package rename

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"
)

// Exit codes, matching dj prune's.
const (
	exitOK      = 0
	exitStopped = 1 // declined, usage error, bad pattern or unreadable folder
	exitPartial = 2 // some renames failed
)

// Run executes the rename subcommand and returns a process exit code.
func Run(args []string, stdout, stderr io.Writer, stdin io.Reader) int {
	fs := flag.NewFlagSet("rename", flag.ContinueOnError)
	fs.SetOutput(stderr)
	pattern := fs.String("pattern", DefaultPattern,
		"name pattern: {artist} {title} {bpm} {key}; {bpm:03} zero-pads to 3")
	dryRun := fs.Bool("dry-run", false, "show the renames, change nothing")
	yes := fs.Bool("yes", false, "skip the confirmation prompt")
	fs.Usage = func() {
		fmt.Fprintln(stderr, "usage: dj rename [flags] <folder>")
		fs.PrintDefaults()
	}

	// "--" ends flag parsing for good: everything after it is a folder
	// argument, never a flag, even one that looks like "--yes". Split it out
	// up front, because the loop below re-parses each remaining chunk from
	// scratch (to support flags after the folder) and would otherwise forget
	// that "--" had already been seen and parse a later chunk as real flags.
	head := args
	var tail []string
	for i, a := range args {
		if a == "--" {
			head = args[:i]
			tail = args[i+1:]
			break
		}
	}

	// The flag package stops at the first non-flag, so parse again after the
	// folder: "dj rename ~/Downloads --dry-run" must not rename anything.
	var folders []string
	for rest := head; ; {
		if err := fs.Parse(rest); err != nil {
			if errors.Is(err, flag.ErrHelp) {
				return exitOK
			}
			return exitStopped
		}
		if fs.NArg() == 0 {
			break
		}
		folders = append(folders, fs.Arg(0))
		rest = fs.Args()[1:]
	}
	folders = append(folders, tail...)
	if len(folders) != 1 {
		fs.Usage()
		return exitStopped
	}

	p, err := ParsePattern(*pattern)
	if err != nil {
		fmt.Fprintf(stderr, "dj rename: %v\n", err)
		return exitStopped
	}
	plan, err := Plan(folders[0], p)
	if err != nil {
		fmt.Fprintf(stderr, "dj rename: %v\n", err)
		return exitStopped
	}
	if err := Summary(stdout, plan); err != nil {
		fmt.Fprintf(stderr, "dj rename: %v\n", err)
		return exitStopped
	}

	if len(plan.Renames) == 0 {
		return exitOK
	}
	if *dryRun {
		fmt.Fprintln(stdout, "\nThis was a dry run. Nothing was renamed.")
		return exitOK
	}
	if !*yes && !confirm(stdin, stdout, len(plan.Renames)) {
		fmt.Fprintln(stdout, "Nothing was renamed.")
		return exitStopped
	}

	res := Apply(plan)
	fmt.Fprintf(stdout, "\nRenamed %d of %d files.\n", res.Renamed, len(plan.Renames))
	if res.Checked > 0 {
		fmt.Fprintf(stdout, "%d were renamed after a check rather than atomically (e.g. on exFAT, or case-only renames on HFS+).\n", res.Checked)
	}
	for _, f := range res.Failures {
		fmt.Fprintf(stderr, "dj rename: %s: %s\n", f.Name, f.Reason)
	}
	if len(res.Failures) > 0 {
		return exitPartial
	}
	return exitOK
}

// Summary writes the old -> new table, the skipped files and the totals.
func Summary(w io.Writer, p *RenamePlan) error {
	if len(p.Renames) > 0 {
		tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
		fmt.Fprintln(tw, "OLD\t\tNEW")
		for _, r := range p.Renames {
			fmt.Fprintf(tw, "%s\t→\t%s\n", r.Old, r.New)
		}
		if err := tw.Flush(); err != nil {
			return err
		}
	}
	if len(p.Skipped) > 0 {
		fmt.Fprintln(w, "\nSKIPPED")
		for _, s := range p.Skipped {
			fmt.Fprintf(w, "  %s: %s\n", s.Name, s.Reason)
		}
	}
	_, err := fmt.Fprintf(w, "\n%d to rename, %d skipped, %d already match\n",
		len(p.Renames), len(p.Skipped), p.Unchanged)
	return err
}

// confirm asks once. Anything other than an explicit yes means no.
func confirm(in io.Reader, out io.Writer, count int) bool {
	fmt.Fprintf(out, "\nRename %d files? [y/N] ", count)
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
