//go:build darwin

package prune

import (
	"fmt"
	"os"
)

// TrashPath is the trash utility that ships with macOS. It is used rather than
// os.Remove because it records each file's original location, so anything moved
// can be restored individually with Put Back in Finder.
const TrashPath = "/usr/bin/trash"

// TrashAvailable reports whether the trash utility can be used. Callers check
// this before prompting, so a run fails before the user confirms rather than
// after.
func TrashAvailable() error {
	info, err := os.Stat(TrashPath)
	if err != nil {
		return fmt.Errorf("%s is required to move files to the Trash: %w", TrashPath, err)
	}
	if info.Mode()&0o111 == 0 {
		return fmt.Errorf("%s is not executable", TrashPath)
	}
	return nil
}
