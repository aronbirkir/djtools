package rename

import (
	"errors"
	"os"
)

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
