//go:build !darwin

package rename

// renameNoReplace renames oldPath to newPath unless newPath already exists.
// It reports whether it had to fall back to checkedRename's check-then-rename
// rather than an atomic no-replace rename -- always true here, since this
// platform has no such primitive at all; noreplace_darwin.go's equivalent
// can report false when the volume supports one.
//
// The tool is used on macOS, where noreplace_darwin.go closes the small
// window checkedRename's own doc comment explains, whenever the volume lets
// it.
func renameNoReplace(oldPath, newPath string) (checked bool, err error) {
	return true, checkedRename(oldPath, newPath)
}
