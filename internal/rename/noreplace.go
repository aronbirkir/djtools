package rename

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// checkedRename renames oldPath to newPath after checking that newPath is
// either free or the same file as oldPath -- a case-only rename, by the same
// same-file rule occupant uses elsewhere.
//
// Unlike an atomic no-replace rename, this leaves a small window between the
// check and the rename in which a file created at newPath would be
// overwritten. It is what both platform files fall back to when the volume
// has no atomic no-overwrite primitive (every platform but darwin, and
// darwin itself on a volume such as exFAT that RENAME_EXCL refuses or does
// not support).
func checkedRename(oldPath, newPath string) error {
	exists, same, err := occupant(oldPath, newPath)
	if err != nil {
		return err
	}
	if exists && !same {
		return fmt.Errorf("%s already exists", filepath.Base(newPath))
	}
	if err := os.Rename(oldPath, newPath); err != nil {
		return err
	}
	return verifyRenamed(filepath.Dir(oldPath), filepath.Base(oldPath), filepath.Base(newPath))
}

// verifyRenamed guards against a rename call that reported success but left
// the old name in place -- for example a rename between two names that are
// hard links of each other under names that are not case-only variants of
// one another, which some rename implementations silently no-op rather than
// error on. It is deliberately paranoid: a rename that actually happened
// must make oldBase disappear from dir's listing.
func verifyRenamed(dir, oldBase, newBase string) error {
	if oldBase == newBase {
		return nil
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if e.Name() == oldBase {
			return errors.New("rename had no effect")
		}
	}
	return nil
}
