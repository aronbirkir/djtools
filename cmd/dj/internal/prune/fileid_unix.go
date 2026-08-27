//go:build unix

package prune

import (
	"io/fs"
	"syscall"
)

// Supported reports whether pruning can run on this platform.
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
