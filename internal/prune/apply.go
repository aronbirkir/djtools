package prune

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

// Runner executes an external command. It is an interface so tests can verify
// trashing without moving real files.
type Runner interface {
	Run(name string, args ...string) error
}

// ExecRunner runs commands for real.
type ExecRunner struct{}

func (ExecRunner) Run(name string, args ...string) error {
	out, err := exec.Command(name, args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s: %w: %s", name, err, strings.TrimSpace(string(out)))
	}
	return nil
}

// batchSize keeps each argv comfortably under ARG_MAX while still making few
// enough calls to be fast over several thousand files.
const batchSize = 200

// Trash moves paths to the system trash in batches. A failed batch is recorded
// and the remaining batches still run, so one bad path cannot strand the rest.
// It returns the number of files handed to a batch that succeeded.
//
// Every path is verified to lie under root before anything is moved, and one
// path outside it refuses the entire operation without trashing any of them.
// This is the only function in the package that can destroy data, so it does not
// trust its caller: a mis-parsed --music, a planner regression or a mis-wired
// flag would otherwise be carried out faithfully. The check costs one string
// comparison per file; the failure it prevents is unrecoverable.
func Trash(r Runner, root string, paths []string) (int, []error) {
	root = filepath.Clean(root)
	for _, path := range paths {
		if err := checkUnder(root, path); err != nil {
			return 0, []error{fmt.Errorf("refusing to trash anything: %w", err)}
		}
	}

	var (
		done int
		errs []error
	)
	for start := 0; start < len(paths); start += batchSize {
		end := min(start+batchSize, len(paths))
		batch := paths[start:end]
		if err := r.Run(TrashPath, batch...); err != nil {
			// The trash binary reports one exit status for the whole batch but
			// moves what it can: one bad path among 200 can leave 199 files
			// genuinely gone while the batch still fails. Counting the batch as
			// zero would tell the user nothing moved when almost everything did,
			// so ask the filesystem rather than trusting the exit code.
			moved := countMissing(batch)
			done += moved
			errs = append(errs, fmt.Errorf(
				"trashing files %d-%d (%s ... %s): %d of %d moved anyway: %w",
				start+1, end, filepath.Base(batch[0]), filepath.Base(batch[len(batch)-1]),
				moved, len(batch), err))
			continue
		}
		done += len(batch)
	}
	return done, errs
}

// countMissing reports how many of paths no longer exist, which after a failed
// batch is how many the trash binary actually moved.
//
// A stat failure other than "does not exist" counts the file as still present.
// Over-reporting what remains is the safe direction: the user re-runs and it
// gets picked up, whereas over-reporting what moved would send them looking in
// the Trash for something still on disk.
func countMissing(paths []string) int {
	var n int
	for _, path := range paths {
		if _, err := os.Lstat(path); errors.Is(err, fs.ErrNotExist) {
			n++
		}
	}
	return n
}

// checkUnder reports whether path is a file strictly inside root.
//
// Matching is exact, deliberately unlike rekordbox.underMusicDir which folds
// case and separators. That one decides whether a path is worth statting, where
// being liberal costs a wasted syscall. This one decides whether to destroy a
// file, so it errs the other way: anything it cannot prove is inside root is
// refused.
//
// It checks the path lexically only. It assumes paths come from a filesystem
// walk that never traverses symlinks and never yields directories, which is what
// plan.walkDisk guarantees; it is not a general-purpose sandbox check.
func checkUnder(root, path string) error {
	if !filepath.IsAbs(path) {
		return fmt.Errorf("path is not absolute: %q", path)
	}
	clean := filepath.Clean(path)
	if clean == root {
		return fmt.Errorf("path is the music directory itself: %q", clean)
	}
	if !strings.HasPrefix(clean, root+string(filepath.Separator)) {
		return fmt.Errorf("path lies outside %s: %q", root, clean)
	}
	return nil
}

// RemoveEmptyDirs deletes directories under root that contain no entries at
// all, deepest first so a parent emptied by its children is also removed.
//
// A directory still holding a leftover such as .DS_Store or cover art is left
// alone: leftovers are reported rather than trashed, and removing them would
// exceed what the user confirmed. root itself is never removed.
func RemoveEmptyDirs(root string) ([]string, []error) {
	root = filepath.Clean(root)
	var dirs []string
	if err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && path != root {
			dirs = append(dirs, path)
		}
		return nil
	}); err != nil {
		return nil, []error{fmt.Errorf("scanning for empty directories: %w", err)}
	}

	// Deepest first, so children are gone before their parent is considered.
	sort.Slice(dirs, func(i, j int) bool {
		return strings.Count(dirs[i], string(filepath.Separator)) >
			strings.Count(dirs[j], string(filepath.Separator))
	})

	var (
		removed []string
		errs    []error
	)
	for _, dir := range dirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			errs = append(errs, fmt.Errorf("reading %s: %w", dir, err))
			continue
		}
		if len(entries) > 0 {
			continue
		}
		if err := os.Remove(dir); err != nil {
			errs = append(errs, fmt.Errorf("removing %s: %w", dir, err))
			continue
		}
		removed = append(removed, dir)
	}
	return removed, errs
}
