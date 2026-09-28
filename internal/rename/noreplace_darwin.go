//go:build darwin

package rename

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"golang.org/x/sys/unix"
)

// renameNoReplace renames oldPath to newPath unless newPath already exists.
//
// unix.RenamexNp with RENAME_EXCL is tried first, so the kernel refuses
// atomically: a file created between planning and renaming is never
// replaced. On APFS, RENAME_EXCL itself succeeds for a case-only rename --
// the "existing" name at newPath is oldPath, just spelled differently. On a
// volume where RENAME_EXCL rejects that as EEXIST instead (HFS+, exFAT),
// this falls back to a plain os.Rename, but only once occupant confirms the
// existing name really is oldPath under a new case, not a hard link with an
// unrelated name, which stays refused. ENOTSUP or EINVAL means the volume
// has no safe no-overwrite rename at all; rather than silently fall back to
// a replacing rename in that case, it is reported as an error.
func renameNoReplace(oldPath, newPath string) error {
	err := unix.RenamexNp(oldPath, newPath, unix.RENAME_EXCL)
	switch {
	case err == nil:
		return verifyRenamed(filepath.Dir(oldPath), filepath.Base(oldPath), filepath.Base(newPath))
	case errors.Is(err, unix.ENOTSUP), errors.Is(err, unix.EINVAL):
		return errors.New("this volume does not support safe (no-overwrite) renames")
	case !errors.Is(err, unix.EEXIST):
		return &os.LinkError{Op: "rename", Old: oldPath, New: newPath, Err: err}
	}

	exists, same, serr := occupant(oldPath, newPath)
	if serr != nil {
		return serr
	}
	if !exists || !same {
		return fmt.Errorf("%s already exists", filepath.Base(newPath))
	}
	if err := os.Rename(oldPath, newPath); err != nil {
		return err
	}
	return verifyRenamed(filepath.Dir(oldPath), filepath.Base(oldPath), filepath.Base(newPath))
}
