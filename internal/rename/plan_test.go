package rename

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bogem/id3v2/v2"
)

// writeMP3 creates a file named name in dir with the given ID3 tags. The body
// is not audio; id3v2 only needs a file to prepend the tag to. nil tags leave
// the file untagged.
func writeMP3(t *testing.T, dir, name string, tags map[string]string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte("not really audio data"), 0o644); err != nil {
		t.Fatal(err)
	}
	if tags == nil {
		return path
	}
	tag, err := id3v2.Open(path, id3v2.Options{Parse: true})
	if err != nil {
		t.Fatal(err)
	}
	defer tag.Close()
	tag.SetDefaultEncoding(id3v2.EncodingUTF8)
	if v := tags[TokenArtist]; v != "" {
		tag.SetArtist(v)
	}
	if v := tags[TokenTitle]; v != "" {
		tag.SetTitle(v)
	}
	if v := tags[TokenBPM]; v != "" {
		tag.AddTextFrame("TBPM", id3v2.EncodingUTF8, v)
	}
	if v := tags[TokenKey]; v != "" {
		tag.AddTextFrame("TKEY", id3v2.EncodingUTF8, v)
	}
	if err := tag.Save(); err != nil {
		t.Fatal(err)
	}
	return path
}

func mustPattern(t *testing.T, s string) Pattern {
	t.Helper()
	p, err := ParsePattern(s)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func mustPlan(t *testing.T, dir, pattern string) *RenamePlan {
	t.Helper()
	plan, err := Plan(dir, mustPattern(t, pattern))
	if err != nil {
		t.Fatal(err)
	}
	return plan
}

// requireCaseInsensitive skips on case-sensitive volumes, where the case-only
// behaviour under test cannot arise.
func requireCaseInsensitive(t *testing.T, dir string) {
	t.Helper()
	probe := filepath.Join(dir, "CaseProbe")
	if err := os.WriteFile(probe, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	defer os.Remove(probe)
	if _, err := os.Stat(filepath.Join(dir, "caseprobe")); err != nil {
		t.Skip("volume is case-sensitive")
	}
}

var fullTags = map[string]string{
	TokenArtist: "DAFT PUNK",
	TokenTitle:  "one more time",
	TokenBPM:    "123",
	TokenKey:    "4A",
}

func TestPlanRenamesFromTags(t *testing.T) {
	dir := t.TempDir()
	writeMP3(t, dir, "track01.mp3", fullTags)

	plan := mustPlan(t, dir, DefaultPattern)

	want := []Rename{{Old: "track01.mp3", New: "123 04A Daft Punk - One More Time.mp3"}}
	if len(plan.Renames) != 1 || plan.Renames[0] != want[0] {
		t.Errorf("Renames = %+v, want %+v", plan.Renames, want)
	}
	if len(plan.Skipped) != 0 || plan.Unchanged != 0 {
		t.Errorf("Skipped = %+v, Unchanged = %d", plan.Skipped, plan.Unchanged)
	}
}

func TestTitleCase(t *testing.T) {
	// strings.Title's word boundaries, as MP3 Renamer used, except that an
	// apostrophe does not start a new word.
	for in, want := range map[string]string{
		"DAFT PUNK":                "Daft Punk",
		"one more time":            "One More Time",
		"don't stop":               "Don't Stop",
		"i swear, it's a daydream": "I Swear, It's A Daydream",
		"that’s real":              "That’s Real",
		"'til dawn":                "'Til Dawn",
		"ac/dc":                    "Ac/Dc",
		"sigur rós":                "Sigur Rós",
	} {
		if got := titleCase(in); got != want {
			t.Errorf("titleCase(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestPlanReplacesSlashAndColon(t *testing.T) {
	dir := t.TempDir()
	writeMP3(t, dir, "x.mp3", map[string]string{TokenArtist: "AC/DC", TokenTitle: "Live: 1991"})

	plan := mustPlan(t, dir, "{artist} - {title}")

	if len(plan.Renames) != 1 || plan.Renames[0].New != "Ac-Dc - Live- 1991.mp3" {
		t.Errorf("Renames = %+v, want Ac-Dc - Live- 1991.mp3", plan.Renames)
	}
}

func TestPlanSkipsMissingTag(t *testing.T) {
	dir := t.TempDir()
	writeMP3(t, dir, "x.mp3", map[string]string{TokenArtist: "A", TokenTitle: "B"})

	plan := mustPlan(t, dir, DefaultPattern)

	if len(plan.Renames) != 0 || len(plan.Skipped) != 1 || plan.Skipped[0].Reason != "no {bpm} tag" {
		t.Errorf("Renames %+v, Skipped %+v, want x.mp3 skipped for no {bpm} tag", plan.Renames, plan.Skipped)
	}
}

func TestPlanSkipsUntaggedFile(t *testing.T) {
	dir := t.TempDir()
	writeMP3(t, dir, "x.mp3", nil)

	plan := mustPlan(t, dir, "{artist} - {title}")

	if len(plan.Skipped) != 1 || plan.Skipped[0].Reason != "no {artist} tag" {
		t.Errorf("Skipped = %+v, want no {artist} tag", plan.Skipped)
	}
}

func TestPlanSkipsUnreadableFile(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("root can read anything")
	}
	dir := t.TempDir()
	path := writeMP3(t, dir, "x.mp3", fullTags)
	if err := os.Chmod(path, 0o000); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(path, 0o644)

	plan := mustPlan(t, dir, DefaultPattern)

	if len(plan.Skipped) != 1 || !strings.HasPrefix(plan.Skipped[0].Reason, "cannot read tags") {
		t.Errorf("Skipped = %+v, want cannot read tags", plan.Skipped)
	}
}

func TestPlanSkipsFilesSharingANewName(t *testing.T) {
	dir := t.TempDir()
	writeMP3(t, dir, "a.mp3", fullTags)
	writeMP3(t, dir, "b.mp3", fullTags)

	plan := mustPlan(t, dir, DefaultPattern)

	if len(plan.Renames) != 0 || len(plan.Skipped) != 2 {
		t.Fatalf("Renames %+v, Skipped %+v, want both skipped", plan.Renames, plan.Skipped)
	}
	if !strings.Contains(plan.Skipped[0].Reason, "same new name as b.mp3") ||
		!strings.Contains(plan.Skipped[1].Reason, "same new name as a.mp3") {
		t.Errorf("reasons = %q / %q", plan.Skipped[0].Reason, plan.Skipped[1].Reason)
	}
}

func TestPlanCollisionIgnoresCase(t *testing.T) {
	// {key} is not title-cased, so the two names differ only in case. On
	// APFS they would be the same file.
	dir := t.TempDir()
	writeMP3(t, dir, "a.mp3", map[string]string{TokenKey: "4a"})
	writeMP3(t, dir, "b.mp3", map[string]string{TokenKey: "4A"})

	plan := mustPlan(t, dir, "{key}")

	if len(plan.Renames) != 0 || len(plan.Skipped) != 2 {
		t.Errorf("Renames %+v, Skipped %+v, want both skipped", plan.Renames, plan.Skipped)
	}
}

func TestPlanSkipsExistingTarget(t *testing.T) {
	dir := t.TempDir()
	writeMP3(t, dir, "x.mp3", map[string]string{TokenArtist: "a", TokenTitle: "b"})
	writeMP3(t, dir, "A - B.mp3", nil) // untagged, so itself skipped

	plan := mustPlan(t, dir, "{artist} - {title}")

	if len(plan.Renames) != 0 {
		t.Fatalf("Renames = %+v, want none", plan.Renames)
	}
	var reason string
	for _, s := range plan.Skipped {
		if s.Name == "x.mp3" {
			reason = s.Reason
		}
	}
	if reason != "A - B.mp3 already exists" {
		t.Errorf("x.mp3 reason = %q, want A - B.mp3 already exists", reason)
	}
}

func TestPlanSkipsChain(t *testing.T) {
	// a.mp3 wants B.mp3, which exists and itself wants C.mp3. v1 does not
	// reorder, so a is skipped and B still renames.
	dir := t.TempDir()
	writeMP3(t, dir, "a.mp3", map[string]string{TokenTitle: "b"})
	writeMP3(t, dir, "B.mp3", map[string]string{TokenTitle: "c"})

	plan := mustPlan(t, dir, "{title}")

	if len(plan.Renames) != 1 || plan.Renames[0] != (Rename{Old: "B.mp3", New: "C.mp3"}) {
		t.Errorf("Renames = %+v, want only B.mp3 -> C.mp3", plan.Renames)
	}
	if len(plan.Skipped) != 1 || plan.Skipped[0].Name != "a.mp3" {
		t.Errorf("Skipped = %+v, want a.mp3", plan.Skipped)
	}
}

func TestPlanAllowsCaseOnlyRename(t *testing.T) {
	dir := t.TempDir()
	writeMP3(t, dir, "daft punk - one.mp3", map[string]string{TokenArtist: "daft punk", TokenTitle: "one"})

	plan := mustPlan(t, dir, "{artist} - {title}")

	want := Rename{Old: "daft punk - one.mp3", New: "Daft Punk - One.mp3"}
	if len(plan.Renames) != 1 || plan.Renames[0] != want {
		t.Errorf("Renames = %+v, want %+v (Skipped %+v)", plan.Renames, want, plan.Skipped)
	}
}

func TestPlanCountsUnchanged(t *testing.T) {
	dir := t.TempDir()
	writeMP3(t, dir, "Daft Punk - One.mp3", map[string]string{TokenArtist: "daft punk", TokenTitle: "one"})

	plan := mustPlan(t, dir, "{artist} - {title}")

	if plan.Unchanged != 1 || len(plan.Renames) != 0 || len(plan.Skipped) != 0 {
		t.Errorf("Unchanged %d, Renames %+v, Skipped %+v", plan.Unchanged, plan.Renames, plan.Skipped)
	}
}

func TestPlanOnlyTopLevelMP3s(t *testing.T) {
	dir := t.TempDir()
	writeMP3(t, dir, "LOUD.MP3", fullTags)
	if err := os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dir, "folder.mp3"), 0o755); err != nil {
		t.Fatal(err)
	}
	sub := filepath.Join(dir, "sub")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	writeMP3(t, sub, "deep.mp3", fullTags)

	plan := mustPlan(t, dir, DefaultPattern)

	if len(plan.Renames) != 1 || plan.Renames[0].Old != "LOUD.MP3" {
		t.Errorf("Renames = %+v, want only LOUD.MP3", plan.Renames)
	}
	if len(plan.Skipped) != 0 {
		t.Errorf("Skipped = %+v, want none", plan.Skipped)
	}
}

func TestPlanMissingDirIsAnError(t *testing.T) {
	if _, err := Plan(filepath.Join(t.TempDir(), "nope"), mustPattern(t, DefaultPattern)); err == nil {
		t.Error("Plan of a missing folder succeeded")
	}
}

// TestValidName exercises the plain-file-name guard directly: ParsePattern
// already refuses "/" and ":" in a pattern's literal text, and clean strips
// them from tag values, so Plan itself cannot be driven to produce a name
// that fails this check. It exists as defence in depth against a future
// token or bug that stops honouring that invariant.
func TestValidName(t *testing.T) {
	for name, wantErr := range map[string]bool{
		"track.mp3":     false,
		".":             true,
		"..":            true,
		"sub/track.mp3": true,
		"/etc/passwd":   true,
	} {
		err := validName(name)
		if (err != nil) != wantErr {
			t.Errorf("validName(%q) = %v, wantErr %v", name, err, wantErr)
		}
	}
}

func TestPlanSkipsHardLinkTarget(t *testing.T) {
	dir := t.TempDir()
	writeMP3(t, dir, "a.mp3", map[string]string{TokenArtist: "x", TokenTitle: "y"})
	// A hard link under a different name is the same inode as a.mp3 but is
	// not a case-only variant of its name, so it must not be treated as
	// "the same file" the way a case-only rename target is.
	if err := os.Link(filepath.Join(dir, "a.mp3"), filepath.Join(dir, "X - Y.mp3")); err != nil {
		t.Skipf("hard links unsupported here: %v", err)
	}

	plan := mustPlan(t, dir, "{artist} - {title}")

	if len(plan.Renames) != 0 {
		t.Fatalf("Renames = %+v, want none", plan.Renames)
	}
	var reason string
	for _, s := range plan.Skipped {
		if s.Name == "a.mp3" {
			reason = s.Reason
		}
	}
	if reason != "X - Y.mp3 already exists" {
		t.Errorf("a.mp3 reason = %q, want X - Y.mp3 already exists", reason)
	}
}

func TestCleanReplacesNullSeparatorAndDropsControlChars(t *testing.T) {
	if got, want := clean("A\x00B"), "A, B"; got != want {
		t.Errorf("clean(%q) = %q, want %q", "A\x00B", got, want)
	}
	if got, want := clean("A\x01B"), "AB"; got != want {
		t.Errorf("clean with a control char = %q, want %q", got, want)
	}
}

func TestPlanJoinsMultiValueArtistTag(t *testing.T) {
	// ID3v2.4 separates multiple values in one text frame with "\x00"; MP3
	// Renamer left it in the file name verbatim.
	dir := t.TempDir()
	path := writeMP3(t, dir, "x.mp3", nil)
	tag, err := id3v2.Open(path, id3v2.Options{Parse: true})
	if err != nil {
		t.Fatal(err)
	}
	tag.SetDefaultEncoding(id3v2.EncodingUTF8)
	tag.AddTextFrame("TPE1", id3v2.EncodingUTF8, "a\x00b")
	tag.SetTitle("t")
	if err := tag.Save(); err != nil {
		t.Fatal(err)
	}
	tag.Close()

	plan := mustPlan(t, dir, "{artist} - {title}")

	if len(plan.Renames) != 1 || plan.Renames[0].New != "A, B - T.mp3" {
		t.Errorf("Renames = %+v, want A, B - T.mp3", plan.Renames)
	}
}

func TestPlanSkipsHiddenFileName(t *testing.T) {
	dir := t.TempDir()
	writeMP3(t, dir, "x.mp3", map[string]string{TokenTitle: "y"})

	plan := mustPlan(t, dir, ".{title}")

	if len(plan.Renames) != 0 || len(plan.Skipped) != 1 || plan.Skipped[0].Reason != "new name would be a hidden file" {
		t.Errorf("Renames %+v, Skipped %+v, want a hidden-file skip", plan.Renames, plan.Skipped)
	}
}

func TestPlanSkipsNameTooLong(t *testing.T) {
	dir := t.TempDir()
	writeMP3(t, dir, "a.mp3", map[string]string{TokenTitle: strings.Repeat("x", 300)})

	plan := mustPlan(t, dir, "{title}")

	if len(plan.Renames) != 0 || len(plan.Skipped) != 1 {
		t.Fatalf("Renames %+v, Skipped %+v, want one skipped for length", plan.Renames, plan.Skipped)
	}
	if !strings.Contains(plan.Skipped[0].Reason, "new name is too long") {
		t.Errorf("reason = %q, want it to mention the name being too long", plan.Skipped[0].Reason)
	}
}

func TestPlanCountsNormalizationOnlyDifferenceAsUnchanged(t *testing.T) {
	const (
		precomposedE = "é"  // é as one code point (NFC)
		decomposedE  = "é" // é as "e" + combining acute accent (NFD)
	)
	dir := t.TempDir()
	// The file on disk is named with the decomposed form; the tag produces
	// the composed form. They are the same text, just different Unicode
	// normalisation, so nothing should be renamed.
	diskName := "Caf" + decomposedE + ".mp3"
	writeMP3(t, dir, diskName, map[string]string{TokenTitle: "caf" + precomposedE})

	plan := mustPlan(t, dir, "{title}")

	if plan.Unchanged != 1 || len(plan.Renames) != 0 || len(plan.Skipped) != 0 {
		t.Errorf("Unchanged %d, Renames %+v, Skipped %+v", plan.Unchanged, plan.Renames, plan.Skipped)
	}
}

// TestPlanIgnoresDotFiles covers AppleDouble shadow files (exFAT, FAT, SMB
// all give a file such as "a.mp3" a "._a.mp3" sidecar for the attributes the
// filesystem can't store natively) and hidden files generally: they are not
// real tracks, so Plan should act as though they are not there at all --
// not renamed, not skipped, not counted as Unchanged.
func TestPlanIgnoresDotFiles(t *testing.T) {
	dir := t.TempDir()
	writeMP3(t, dir, "a.mp3", fullTags)
	// Plain bytes, not a real AppleDouble file or valid ID3 -- Plan must
	// never even try to read it.
	if err := os.WriteFile(filepath.Join(dir, "._a.mp3"), []byte("not an appledouble file"), 0o644); err != nil {
		t.Fatal(err)
	}

	plan := mustPlan(t, dir, DefaultPattern)

	if len(plan.Renames) != 1 || plan.Renames[0].Old != "a.mp3" {
		t.Errorf("Renames = %+v, want only a.mp3", plan.Renames)
	}
	if len(plan.Skipped) != 0 {
		t.Errorf("Skipped = %+v, want none", plan.Skipped)
	}
	if plan.Unchanged != 0 {
		t.Errorf("Unchanged = %d, want 0", plan.Unchanged)
	}
}
