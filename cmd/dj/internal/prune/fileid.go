// Package prune removes audio files that are no longer in the rekordbox library.
package prune

// FileID identifies a file the way the kernel does. It exists because path
// strings are not a reliable identity here: the music volume is APFS, which is
// both case-insensitive and normalization-insensitive, so two different strings
// routinely name one file. rekordbox writes NFC while the disk stores NFD for
// 499 filenames, and the library additionally contains entries that differ only
// in letter case. Comparing the identity pair makes all of that irrelevant.
//
// The field names follow the POSIX meaning. On other platforms they hold
// whatever pair uniquely identifies a file there; only equality is meaningful.
type FileID struct {
	Dev uint64
	Ino uint64
}
