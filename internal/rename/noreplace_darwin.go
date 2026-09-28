//go:build darwin

package rename

import (
	"errors"
	"os"
	"path/filepath"

	"golang.org/x/sys/unix"
)

// renameNoReplace renames oldPath to newPath unless newPath already exists.
// It reports whether it had to fall back to checkedRename's check-then-rename
// rather than an atomic no-replace rename.
//
// unix.RenamexNp with RENAME_EXCL is tried first: on a volume that supports
// it, the kernel refuses atomically, so a file created between planning and
// renaming is never replaced. On APFS, RENAME_EXCL itself succeeds for a
// case-only rename -- the "existing" name at newPath is oldPath, just
// spelled differently. On a volume that instead refuses that as EEXIST
// (HFS+), or has no such primitive at all and reports ENOTSUP or EINVAL
// (exFAT, observed even for an otherwise uncontested rename with no real
// collision), this falls back to checkedRename, whose own doc comment
// explains the small window that leaves.
func renameNoReplace(oldPath, newPath string) (checked bool, err error) {
	err = unix.RenamexNp(oldPath, newPath, unix.RENAME_EXCL)
	switch {
	case err == nil:
		return false, verifyRenamed(filepath.Dir(oldPath), filepath.Base(oldPath), filepath.Base(newPath))
	case errors.Is(err, unix.EEXIST), errors.Is(err, unix.ENOTSUP), errors.Is(err, unix.EINVAL):
		return true, checkedRename(oldPath, newPath)
	default:
		return false, &os.LinkError{Op: "rename", Old: oldPath, New: newPath, Err: err}
	}
}
