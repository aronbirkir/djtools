//go:build !unix

package prune

import (
	"errors"
	"io/fs"
	"runtime"
)

// Supported reports whether the file-identity mechanism this package depends on
// exists on the current platform. Deletion capability is a separate check, made
// immediately before anything is trashed.
//
// Only POSIX platforms are implemented. On Windows, file identity would require
// GetFileInformationByHandle (volume serial plus file index), which opens a
// handle per file rather than statting, and trashing would require
// SHFileOperation with FOF_ALLOWUNDO. Neither is implemented, and the machine
// that would have exercised them is retired, so this refuses rather than
// falling back to path-string comparison -- which is exactly the approach that
// would delete in-library files.
func Supported() error {
	return errors.New("dj prune does not run on " + runtime.GOOS +
		": deciding whether two paths name the same file needs platform support " +
		"that is not implemented here")
}

// fileIDFromInfo always reports false here. Callers must treat that as
// "identity unknown" and refuse to delete, never as "not in the library".
func fileIDFromInfo(fs.FileInfo) (FileID, bool) { return FileID{}, false }
