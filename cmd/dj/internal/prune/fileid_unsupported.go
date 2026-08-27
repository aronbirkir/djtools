//go:build !unix

package prune

import (
	"errors"
	"io/fs"
	"runtime"
)

// Supported reports whether pruning can run on this platform.
//
// Only POSIX platforms are implemented. On Windows, file identity would require
// GetFileInformationByHandle (volume serial plus file index), which opens a
// handle per file rather than statting, and trashing would require
// SHFileOperation with FOF_ALLOWUNDO. Neither is implemented, and the machine
// that would have exercised them is retired, so this refuses rather than
// falling back to path-string comparison -- which is exactly the approach that
// would delete in-library files.
func Supported() error {
	return errors.New("dj prune is not implemented on " + runtime.GOOS +
		": file identity and a recoverable trash are both missing")
}

// fileIDFromInfo always reports false here. Callers must treat that as
// "identity unknown" and refuse to delete, never as "not in the library".
func fileIDFromInfo(fs.FileInfo) (FileID, bool) { return FileID{}, false }
