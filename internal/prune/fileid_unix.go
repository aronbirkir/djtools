//go:build unix

package prune

import (
	"io/fs"
	"syscall"
)

// Supported reports whether the file-identity mechanism this package depends on
// exists on the current platform.
//
// It deliberately says nothing about whether files can be deleted. Planning
// works on any POSIX system, while moving files to a recoverable trash is
// checked separately, immediately before anything is trashed.
func Supported() error { return nil }

// fileIDFromInfo extracts the identity pair from a stat result. It reports false
// when the platform provides no underlying stat data, which callers must treat
// as "unknown", never as "not in the library".
func fileIDFromInfo(info fs.FileInfo) (FileID, bool) {
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return FileID{}, false
	}
	return FileID{Dev: uint64(st.Dev), Ino: uint64(st.Ino)}, true
}
