//go:build !darwin

package prune

import "errors"

// TrashPath is deliberately not a resolvable command. Nothing should reach it --
// TrashAvailable refuses first -- but if that gate were ever skipped, a bare
// name like "trash" would resolve through PATH and could invoke an unrelated
// binary instead of failing closed.
const TrashPath = ""

// TrashAvailable always fails here: moving files to a recoverable trash is the
// only deletion this tool will perform, and there is no verified way to do it
// on this platform.
func TrashAvailable() error {
	return errors.New("no supported trash mechanism on this platform; dj prune only runs on macOS")
}
