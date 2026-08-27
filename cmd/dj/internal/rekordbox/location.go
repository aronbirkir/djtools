// Package rekordbox reads rekordbox XML collection exports.
package rekordbox

import (
	"net/url"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
)

// Kind classifies what a rekordbox Location attribute points at. The four kinds
// need different handling, so this is a closed enum rather than a bool.
type Kind int

const (
	// KindLocal is a file under the music directory: a prune candidate.
	KindLocal Kind = iota
	// KindStale is a path shaped for a different operating system than the one
	// running, so it cannot be reached from here. On the Mac these are the
	// Windows paths left over from the original export; run on that Windows
	// machine, the mac paths would be the stale ones instead.
	KindStale
	// KindForeign is a valid path for this host that lies outside the music
	// directory, such as rekordbox's own Sampler content. Never touched.
	KindForeign
	// KindNonFile is a streaming entry or anything with no usable path.
	KindNonFile
)

func (k Kind) String() string {
	switch k {
	case KindLocal:
		return "local"
	case KindStale:
		return "stale"
	case KindForeign:
		return "foreign"
	case KindNonFile:
		return "nonfile"
	}
	return "unknown"
}

// Host is the operating system whose path conventions apply. It is a parameter
// rather than a direct runtime.GOOS read so that both branches are testable
// from either platform, which is the only coverage the Windows path can get now
// that the machine holding this collection has been retired.
type Host int

const (
	// HostPOSIX is macOS or Linux. Zero value so tests default to it.
	HostPOSIX Host = iota
	HostWindows
)

// CurrentHost is the host this process is running on.
func CurrentHost() Host {
	if runtime.GOOS == "windows" {
		return HostWindows
	}
	return HostPOSIX
}

// Location is a decoded and classified Location attribute.
type Location struct {
	// Raw is the attribute value exactly as it appeared in the XML.
	Raw string
	// Path is the decoded filesystem path, empty when Kind is KindNonFile.
	Path string
	Kind Kind
}

// localhostPrefix is what rekordbox writes ahead of every path. Windows entries
// continue straight into a drive letter with no separator, which is why this is
// stripped literally instead of going through url.Parse.
const localhostPrefix = "file://localhost"

var windowsDrive = regexp.MustCompile(`^[A-Za-z]:[/\\]`)

// ClassifyLocation decodes a rekordbox Location and decides how prune should
// treat it, according to the path conventions of host.
func ClassifyLocation(raw, musicDir string, host Host) Location {
	rest, ok := strings.CutPrefix(raw, localhostPrefix)
	if !ok {
		return Location{Raw: raw, Kind: KindNonFile}
	}

	// Decode exactly once. A literal '%' arrives as %25, so a second pass would
	// corrupt the path. PathUnescape rather than QueryUnescape because '+' is a
	// real filename character in this collection, not an encoded space.
	path, err := url.PathUnescape(rest)
	if err != nil {
		return Location{Raw: raw, Kind: KindNonFile}
	}

	drive := windowsDrive.MatchString(path)
	posixAbs := strings.HasPrefix(path, "/")

	switch host {
	case HostWindows:
		if drive {
			break // reachable here
		}
		if posixAbs {
			// A mac path, unreachable from Windows.
			return Location{Raw: raw, Path: path, Kind: KindStale}
		}
		// Streaming entries such as "tidal:tracks:62303032".
		return Location{Raw: raw, Kind: KindNonFile}
	default: // HostPOSIX
		if drive {
			// A Windows path, unreachable from here. These are the leftovers
			// from the original Windows export of this collection.
			return Location{Raw: raw, Path: path, Kind: KindStale}
		}
		if !posixAbs {
			return Location{Raw: raw, Kind: KindNonFile}
		}
		// Clean only POSIX paths. Cleaning a drive path would rewrite its
		// separators differently depending on which OS the binary was built
		// for, making Path unstable across platforms.
		path = filepath.Clean(path)
	}

	if underMusicDir(path, musicDir) {
		return Location{Raw: raw, Path: path, Kind: KindLocal}
	}
	return Location{Raw: raw, Path: path, Kind: KindForeign}
}

// underMusicDir reports whether path is musicDir or sits inside it.
//
// Comparison folds case and treats the two separators as equivalent, because
// both APFS and NTFS are case-insensitive by default. Being liberal is safe: a
// false positive here costs one wasted os.Stat, never a wrong deletion, because
// identity is settled afterwards by FileID. The trailing separator matters --
// without it "/music-archive" would match a music dir of "/music".
func underMusicDir(path, musicDir string) bool {
	p, m := foldPath(path), foldPath(musicDir)
	if p == m {
		return true
	}
	if !strings.HasSuffix(m, "/") {
		m += "/"
	}
	return strings.HasPrefix(p, m)
}

func foldPath(p string) string {
	return strings.ToLower(strings.ReplaceAll(p, `\`, "/"))
}
