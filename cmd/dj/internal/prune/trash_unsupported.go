//go:build !darwin

package prune

import "errors"

// TrashPath is unused on platforms without a supported trash mechanism. It is
// declared so the portable batching code and its tests still build.
const TrashPath = "trash"

// TrashAvailable always fails here: moving files to a recoverable trash is the
// only deletion this tool will perform, and there is no verified way to do it
// on this platform.
func TrashAvailable() error {
	return errors.New("no supported trash mechanism on this platform; dj prune only runs on macOS")
}
