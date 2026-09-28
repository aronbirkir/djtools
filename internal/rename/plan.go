package rename

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode"

	"github.com/bogem/id3v2/v2"
	"golang.org/x/text/unicode/norm"
)

// Rename is one file's current and new base name within RenamePlan.Dir.
type Rename struct{ Old, New string }

// Skip is a file left alone, and why.
type Skip struct{ Name, Reason string }

// maxNameBytes is the file name length most filesystems this tool targets
// enforce (APFS, HFS+, exFAT), in bytes of the encoded name.
const maxNameBytes = 255

// RenamePlan is everything one run would do, worked out before anything is
// renamed.
type RenamePlan struct {
	Dir       string
	Renames   []Rename // sorted by Old
	Skipped   []Skip   // sorted by Name
	Unchanged int      // files already named as the pattern says
}

// Plan works out the new name of every .mp3 directly inside dir. It never
// modifies anything.
//
// MP3 Renamer renamed with os.Rename, which silently replaces an existing
// file: two copies of a track with identical tags left one of them destroyed.
// Plan refuses every rename that could land on another file -- a shared new
// name, or a name already taken on disk -- and lists it instead.
func Plan(dir string, p Pattern) (*RenamePlan, error) {
	dir, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	plan := &RenamePlan{Dir: dir}
	skip := func(name, format string, a ...any) {
		plan.Skipped = append(plan.Skipped, Skip{Name: name, Reason: fmt.Sprintf(format, a...)})
	}

	var candidates []Rename
	for _, e := range entries {
		if !e.Type().IsRegular() || !strings.EqualFold(filepath.Ext(e.Name()), ".mp3") {
			continue
		}
		values, err := readTags(filepath.Join(dir, e.Name()))
		if err != nil {
			skip(e.Name(), "cannot read tags: %v", err)
			continue
		}
		if missing := firstMissing(p, values); missing != "" {
			skip(e.Name(), "no {%s} tag", missing)
			continue
		}
		newName := p.Expand(values) + ".mp3"
		// A difference that is purely Unicode normalisation (an NFD-named
		// file whose tags produce the NFC form, or vice versa) is not a
		// real rename: it is the same text, and APFS itself treats the two
		// forms as the same name for lookups.
		if norm.NFC.String(newName) == norm.NFC.String(e.Name()) {
			plan.Unchanged++
			continue
		}
		if err := validName(newName); err != nil {
			skip(e.Name(), "new name is not a plain file name")
			continue
		}
		if strings.HasPrefix(newName, ".") {
			skip(e.Name(), "new name would be a hidden file")
			continue
		}
		if n := len(newName); n > maxNameBytes {
			skip(e.Name(), "new name is too long (%d bytes, the limit is %d)", n, maxNameBytes)
			continue
		}
		candidates = append(candidates, Rename{Old: e.Name(), New: newName})
	}

	// APFS treats names differing only in case or Unicode normalisation as
	// the same file, so compare new names the same way.
	groups := map[string][]string{}
	for _, c := range candidates {
		k := foldName(c.New)
		groups[k] = append(groups[k], c.Old)
	}
	for _, c := range candidates {
		if group := groups[foldName(c.New)]; len(group) > 1 {
			var others []string
			for _, old := range group {
				if old != c.Old {
					others = append(others, old)
				}
			}
			skip(c.Old, "same new name as %s: %s", strings.Join(others, ", "), c.New)
			continue
		}
		exists, same, err := occupant(filepath.Join(dir, c.Old), filepath.Join(dir, c.New))
		switch {
		case err != nil:
			skip(c.Old, "cannot check %s: %v", c.New, err)
			continue
		case exists && !same:
			// Includes another file's current name: chains and swaps are
			// skipped rather than reordered.
			skip(c.Old, "%s already exists", c.New)
			continue
		}
		plan.Renames = append(plan.Renames, c)
	}

	sort.Slice(plan.Renames, func(i, j int) bool { return plan.Renames[i].Old < plan.Renames[j].Old })
	sort.Slice(plan.Skipped, func(i, j int) bool { return plan.Skipped[i].Name < plan.Skipped[j].Name })
	return plan, nil
}

// occupant reports whether newPath exists and, if so, whether it is the same
// file as oldPath -- as it is for a case-only rename on a case-insensitive
// volume.
//
// os.SameFile alone is not enough: a hard link at newPath pointing at
// oldPath's inode under a wholly different name is also "the same file" by
// that test, but renaming oldPath onto it would silently make the link's own
// name the survivor while discarding oldPath's name -- not what a case-only
// rename does. "Same file" here additionally requires the two base names to
// be equal once case- and normalisation-folded, i.e. this is a rename that
// only changes case or Unicode form.
func occupant(oldPath, newPath string) (exists, same bool, err error) {
	ni, err := os.Lstat(newPath)
	if errors.Is(err, fs.ErrNotExist) {
		return false, false, nil
	}
	if err != nil {
		return false, false, err
	}
	oi, err := os.Lstat(oldPath)
	if err != nil {
		return true, false, err
	}
	same = os.SameFile(oi, ni) && foldName(filepath.Base(oldPath)) == foldName(filepath.Base(newPath))
	return true, same, nil
}

func foldName(name string) string {
	return strings.ToLower(norm.NFC.String(name))
}

// validName reports whether name is safe to use as a file's own base name
// within its current directory -- defence in depth. ParsePattern already
// rejects "/" and ":" in a pattern's literal text, and clean strips both
// from tag values, so in practice Plan cannot produce a name that fails
// this check; it exists in case a future token or a bug in either of those
// stops holding that invariant.
func validName(name string) error {
	if name == "." || name == ".." || filepath.Base(name) != name {
		return fmt.Errorf("%q is not a plain file name", name)
	}
	return nil
}

func firstMissing(p Pattern, values map[string]string) string {
	for _, tok := range p.Tokens() {
		if values[tok] == "" {
			return tok
		}
	}
	return ""
}

// readTags returns the pattern values for one file, cleaned for use in a name.
func readTags(path string) (map[string]string, error) {
	tag, err := id3v2.Open(path, id3v2.Options{Parse: true})
	if err != nil {
		return nil, err
	}
	defer tag.Close()
	return map[string]string{
		TokenArtist: clean(titleCase(tag.Artist())),
		TokenTitle:  clean(titleCase(tag.Title())),
		TokenBPM:    clean(tag.GetTextFrame("TBPM").Text),
		TokenKey:    clean(tag.GetTextFrame("TKEY").Text),
	}, nil
}

// clean makes a tag value safe inside a file name. "/" is a path separator and
// ":" shows as "/" in the Finder, so an artist such as AC/DC would otherwise
// point the rename into a folder that does not exist. ID3v2.4 joins a
// multi-value frame (for example two artists) with "\x00"; that is turned
// into ", " before other control characters, which have no place in a file
// name, are dropped.
func clean(s string) string {
	s = strings.ReplaceAll(s, "\x00", ", ")
	s = strings.NewReplacer("/", "-", ":", "-").Replace(s)
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if unicode.IsControl(r) {
			continue
		}
		b.WriteRune(r)
	}
	return strings.TrimSpace(b.String())
}

// titleCase reproduces strings.Title(strings.ToLower(s)), which MP3 Renamer
// used, without the deprecated function: names must come out as they always
// have, quirks included ("don't" becomes "Don'T").
func titleCase(s string) string {
	var b strings.Builder
	prev := ' '
	for _, r := range strings.ToLower(s) {
		if isSeparator(prev) {
			b.WriteRune(unicode.ToTitle(r))
		} else {
			b.WriteRune(r)
		}
		prev = r
	}
	return b.String()
}

// isSeparator is strings.Title's word-boundary test.
func isSeparator(r rune) bool {
	if r <= 0x7F {
		switch {
		case '0' <= r && r <= '9', 'a' <= r && r <= 'z', 'A' <= r && r <= 'Z', r == '_':
			return false
		}
		return true
	}
	if unicode.IsLetter(r) || unicode.IsDigit(r) {
		return false
	}
	return unicode.IsSpace(r)
}
