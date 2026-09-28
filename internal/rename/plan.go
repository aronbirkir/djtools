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
		if newName == e.Name() {
			plan.Unchanged++
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
	return true, os.SameFile(oi, ni), nil
}

func foldName(name string) string {
	return strings.ToLower(norm.NFC.String(name))
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
// point the rename into a folder that does not exist.
func clean(s string) string {
	return strings.TrimSpace(strings.NewReplacer("/", "-", ":", "-").Replace(s))
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
