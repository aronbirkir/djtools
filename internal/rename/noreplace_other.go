//go:build !darwin

package rename

import (
	"fmt"
	"os"
	"path/filepath"
)

// renameNoReplace renames oldPath to newPath unless newPath already exists.
//
// Without an atomic no-replace rename, this checks and then renames, which
// leaves a small window in which a file created at newPath would be replaced.
// The tool is used on macOS, where noreplace_darwin.go closes that window.
func renameNoReplace(oldPath, newPath string) error {
	exists, same, err := occupant(oldPath, newPath)
	if err != nil {
		return err
	}
	if exists && !same {
		return fmt.Errorf("%s already exists", filepath.Base(newPath))
	}
	return os.Rename(oldPath, newPath)
}
