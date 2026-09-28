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
// RENAME_EXCL makes the kernel refuse atomically, so a file created between
// planning and renaming is never replaced. A case-only rename is the one
// exception: on a case-insensitive volume the "existing" target is the file
// itself, which RENAME_EXCL would refuse, so that case uses a plain rename.
func renameNoReplace(oldPath, newPath string) error {
	exists, same, err := occupant(oldPath, newPath)
	if err != nil {
		return err
	}
	if exists && same {
		return os.Rename(oldPath, newPath)
	}
	if err := unix.RenamexNp(oldPath, newPath, unix.RENAME_EXCL); err != nil {
		if errors.Is(err, unix.EEXIST) {
			return fmt.Errorf("%s already exists", filepath.Base(newPath))
		}
		return &os.LinkError{Op: "rename", Old: oldPath, New: newPath, Err: err}
	}
	return nil
}
