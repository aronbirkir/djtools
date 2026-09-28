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

func TestTitleCaseMatchesMP3Renamer(t *testing.T) {
	// strings.Title's word boundaries, including its apostrophe quirk, so
	// names come out exactly as MP3 Renamer made them.
	for in, want := range map[string]string{
		"DAFT PUNK":     "Daft Punk",
		"one more time": "One More Time",
		"don't stop":    "Don'T Stop",
		"ac/dc":         "Ac/Dc",
		"sigur rós":     "Sigur Rós",
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
