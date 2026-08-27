package prune

import (
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/aronbirkir/djtools/cmd/dj/internal/rekordbox"
)

var defaultExts = []string{".mp3", ".wav", ".aiff", ".flac", ".m4a"}

// locationFor builds the Location attribute rekordbox would write for a path.
// EscapedPath escapes spaces as %20 and leaves '+' literal, matching the real
// export.
func locationFor(path string) string {
	u := url.URL{Path: path}
	return "file://localhost" + u.EscapedPath()
}

func collectionOf(paths ...string) *rekordbox.Collection {
	c := &rekordbox.Collection{HasCollection: true, HasPlaylists: true}
	for _, p := range paths {
		c.Tracks = append(c.Tracks, rekordbox.Track{Location: locationFor(p)})
	}
	c.DeclaredEntries = len(c.Tracks)
	return c
}

func writeFile(t *testing.T, path string, size int) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, make([]byte, size), 0o644); err != nil {
		t.Fatal(err)
	}
}

// requireNormInsensitiveFS skips a test unless the filesystem resolves a path
// spelled in a different normalization form, which is what inode matching
// relies on. Without this a failure here would look like a logic bug.
func requireNormInsensitiveFS(t *testing.T, dir string) {
	t.Helper()
	probeNFD := filepath.Join(dir, ".probe-e\u0301")
	if err := os.WriteFile(probeNFD, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	defer os.Remove(probeNFD)
	if _, err := os.Stat(filepath.Join(dir, ".probe-\u00e9")); err != nil {
		t.Skipf("filesystem at %s is normalization-sensitive, inode matching is untestable here", dir)
	}
}

// requireDistinctSpellings fails when two names meant to be different
// normalization forms are in fact the same string.
//
// Spelled with literal accented characters the two forms are indistinguishable
// in an editor, so a copy-paste slip leaves the test comparing a file to itself:
// passing while proving nothing. That mistake has already happened once in this
// package, in fileid_test.go, and it passed silently. Always write the names as
// \u escapes and assert here before relying on them.
func requireDistinctSpellings(t *testing.T, a, b string) {
	t.Helper()
	if a == b {
		t.Fatalf("the two spellings are byte-identical (% x), so this test would prove nothing", a)
	}
}

// buildPlan calls Build with no known export time, which disables the
// added-after-export skip. Only TestBuildSkipsAudioAddedAfterExport needs that
// behaviour, and it calls Build directly.
func buildPlan(c *rekordbox.Collection, musicDir string, exts []string) (*Plan, error) {
	return Build(c, musicDir, exts, time.Time{})
}

// THE regression test. On disk the filename is stored NFD; rekordbox spells it
// NFC. This is the real-world direction and it accounts for 155 in-library
// files in the reference collection. String comparison reports them as orphans.
func TestBuildNFDOnDiskNFCInCollection(t *testing.T) {
	music := t.TempDir()
	requireNormInsensitiveFS(t, music)

	// "Loredana Bertè": NFD stores 'e' + U+0300, NFC stores U+00E8.
	nfdName := "08A 109 In alto mare - Loredana Berte\u0300.mp3"
	nfcName := "08A 109 In alto mare - Loredana Bert\u00e8.mp3"
	requireDistinctSpellings(t, nfdName, nfcName)

	writeFile(t, filepath.Join(music, "Easy", nfdName), 1024)

	p, err := buildPlan(collectionOf(filepath.Join(music, "Easy", nfcName)), music, defaultExts)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if len(p.Orphans) != 0 {
		t.Errorf("Orphans = %v, want none: the NFD file on disk is the NFC entry in the collection", p.Orphans)
	}
	if p.Keepers != 1 {
		t.Errorf("Keepers = %d, want 1", p.Keepers)
	}
	if len(p.DeadLinks) != 0 {
		t.Errorf("DeadLinks = %v, want none", p.DeadLinks)
	}
}

// The reverse direction does not occur in the current export, but nothing
// guarantees rekordbox will not start emitting NFD.
func TestBuildNFCOnDiskNFDInCollection(t *testing.T) {
	music := t.TempDir()
	requireNormInsensitiveFS(t, music)

	nfcName := "06A 108 Te Amo Coraz\u00f3n - Prince.mp3"
	nfdName := "06A 108 Te Amo Corazo\u0301n - Prince.mp3"
	requireDistinctSpellings(t, nfdName, nfcName)

	writeFile(t, filepath.Join(music, "Pop", nfcName), 512)

	p, err := buildPlan(collectionOf(filepath.Join(music, "Pop", nfdName)), music, defaultExts)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if len(p.Orphans) != 0 {
		t.Errorf("Orphans = %v, want none", p.Orphans)
	}
	if p.Keepers != 1 {
		t.Errorf("Keepers = %d, want 1", p.Keepers)
	}
}

// Two entries differing only in case name one file on a case-insensitive
// volume. They must collapse to one keeper and be surfaced as a duplicate.
func TestBuildCaseOnlyDuplicateEntries(t *testing.T) {
	music := t.TempDir()

	onDisk := filepath.Join(music, "A2025-07", "112 04A Oliver Koletzki - A Tribe Called Kotori.mp3")
	writeFile(t, onDisk, 2048)

	upper := filepath.Join(music, "A2025-07", "112 04A OLIVER KOLETZKI - A Tribe Called Kotori.mp3")
	if _, err := os.Stat(upper); err != nil {
		t.Skipf("filesystem at %s is case-sensitive: %v", music, err)
	}

	p, err := buildPlan(collectionOf(onDisk, upper), music, defaultExts)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if len(p.Orphans) != 0 {
		t.Errorf("Orphans = %v, want none", p.Orphans)
	}
	if p.Keepers != 1 {
		t.Errorf("Keepers = %d, want 1", p.Keepers)
	}
	if len(p.CaseDupes) != 1 {
		t.Fatalf("len(CaseDupes) = %d, want 1", len(p.CaseDupes))
	}
	if len(p.CaseDupes[0]) != 2 {
		t.Errorf("CaseDupes[0] = %v, want two spellings", p.CaseDupes[0])
	}
}

func TestBuildOrphansAndKeepers(t *testing.T) {
	music := t.TempDir()

	keep := filepath.Join(music, "House", "keep.mp3")
	orphan := filepath.Join(music, "House", "orphan.mp3")
	otherOrphan := filepath.Join(music, "Disco", "gone.mp3")
	writeFile(t, keep, 100)
	writeFile(t, orphan, 250)
	writeFile(t, otherOrphan, 400)

	p, err := buildPlan(collectionOf(keep), music, defaultExts)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if p.Keepers != 1 {
		t.Errorf("Keepers = %d, want 1", p.Keepers)
	}
	want := []string{otherOrphan, orphan}
	if len(p.Orphans) != len(want) {
		t.Fatalf("Orphans = %v, want %v", p.Orphans, want)
	}
	for i := range want {
		if p.Orphans[i] != want[i] {
			t.Errorf("Orphans[%d] = %q, want %q (must be sorted)", i, p.Orphans[i], want[i])
		}
	}
	if p.OrphanSize != 650 {
		t.Errorf("OrphanSize = %d, want 650", p.OrphanSize)
	}
	if p.OnDiskAudio() != 3 {
		t.Errorf("OnDiskAudio() = %d, want 3", p.OnDiskAudio())
	}
	if got := p.OrphanPct(); got < 66.6 || got > 66.7 {
		t.Errorf("OrphanPct() = %.2f, want ~66.67", got)
	}
	if p.FolderOrphans["Disco"] != 1 || p.FolderOrphans["House"] != 1 {
		t.Errorf("FolderOrphans = %v, want Disco:1 House:1", p.FolderOrphans)
	}
	if p.FolderTotals["House"] != 2 {
		t.Errorf("FolderTotals[House] = %d, want 2", p.FolderTotals["House"])
	}
	if p.FolderBytes["Disco"] != 400 {
		t.Errorf("FolderBytes[Disco] = %d, want 400", p.FolderBytes["Disco"])
	}
}

func TestBuildClassifiesNonLocalEntries(t *testing.T) {
	music := t.TempDir()
	writeFile(t, filepath.Join(music, "House", "keep.mp3"), 10)

	c := collectionOf(filepath.Join(music, "House", "keep.mp3"))
	c.Tracks = append(c.Tracks,
		rekordbox.Track{Location: "file://localhoste:/music/Funky/a.mp3"},
		rekordbox.Track{Location: "file://localhoste:/music/Funky/a.mp3"},
		rekordbox.Track{Location: "file://localhoste:/music/Funky/b.mp3"},
		rekordbox.Track{Location: "file://localhosttidal:tracks:62303032"},
		rekordbox.Track{Location: "file://localhost/Users/aron/Music/rekordbox/Sampler/House1.wav"},
	)
	c.DeclaredEntries = len(c.Tracks)

	p, err := buildPlan(c, music, defaultExts)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if len(p.Stale) != 2 {
		t.Errorf("Stale = %v, want 2 unique paths", p.Stale)
	}
	// Only paths seen more than once are duplicates.
	if len(p.StaleDupes) != 1 {
		t.Errorf("StaleDupes = %v, want 1 entry", p.StaleDupes)
	}
	if p.StaleDupes["e:/music/Funky/a.mp3"] != 2 {
		t.Errorf("StaleDupes[a.mp3] = %d, want 2", p.StaleDupes["e:/music/Funky/a.mp3"])
	}
	if p.NonFile != 1 {
		t.Errorf("NonFile = %d, want 1", p.NonFile)
	}
	if p.Foreign != 1 {
		t.Errorf("Foreign = %d, want 1", p.Foreign)
	}
}

func TestBuildDeadLinks(t *testing.T) {
	music := t.TempDir()
	writeFile(t, filepath.Join(music, "House", "present.mp3"), 10)

	missing := filepath.Join(music, "A2026-05", "gone.mp3")
	p, err := buildPlan(collectionOf(filepath.Join(music, "House", "present.mp3"), missing), music, defaultExts)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if len(p.DeadLinks) != 1 || p.DeadLinks[0] != missing {
		t.Errorf("DeadLinks = %v, want [%s]", p.DeadLinks, missing)
	}
	if len(p.Unresolved) != 0 {
		t.Errorf("Unresolved = %v, want none: a missing file is a dead link, not an error", p.Unresolved)
	}
}

func TestBuildNonAudioIsLeftoverNotOrphan(t *testing.T) {
	music := t.TempDir()
	writeFile(t, filepath.Join(music, "House", "cover.jpg"), 10)
	writeFile(t, filepath.Join(music, "House", "notes.txt"), 10)
	writeFile(t, filepath.Join(music, ".DS_Store"), 10)

	p, err := buildPlan(collectionOf(), music, defaultExts)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if len(p.Orphans) != 0 {
		t.Errorf("Orphans = %v, want none: non-audio files are never trashed", p.Orphans)
	}
	if len(p.Leftovers) != 3 {
		t.Errorf("Leftovers = %v, want 3", p.Leftovers)
	}
}

func TestBuildHonoursExtensionList(t *testing.T) {
	music := t.TempDir()
	writeFile(t, filepath.Join(music, "Kit", "loop.WAV"), 10)
	writeFile(t, filepath.Join(music, "Kit", "track.mp3"), 10)

	p, err := buildPlan(collectionOf(), music, []string{".mp3"})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if len(p.Orphans) != 1 || filepath.Base(p.Orphans[0]) != "track.mp3" {
		t.Errorf("Orphans = %v, want only track.mp3", p.Orphans)
	}
	if len(p.Leftovers) != 1 {
		t.Errorf("Leftovers = %v, want loop.WAV", p.Leftovers)
	}

	// Extension matching is case-insensitive.
	p2, err := buildPlan(collectionOf(), music, []string{".wav"})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if len(p2.Orphans) != 1 || filepath.Base(p2.Orphans[0]) != "loop.WAV" {
		t.Errorf("Orphans = %v, want only loop.WAV", p2.Orphans)
	}
}

func TestBuildSymlinksAreNeverOrphans(t *testing.T) {
	music := t.TempDir()
	real := filepath.Join(music, "House", "real.mp3")
	writeFile(t, real, 10)

	link := filepath.Join(music, "House", "link.mp3")
	if err := os.Symlink(real, link); err != nil {
		t.Skipf("cannot create symlink: %v", err)
	}

	p, err := buildPlan(collectionOf(real), music, defaultExts)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if len(p.Orphans) != 0 {
		t.Errorf("Orphans = %v, want none: a symlink must never be trashed", p.Orphans)
	}
	if len(p.Symlinks) != 1 || p.Symlinks[0] != link {
		t.Errorf("Symlinks = %v, want [%s]", p.Symlinks, link)
	}
}

func TestBuildTracksNewestAudioModTime(t *testing.T) {
	music := t.TempDir()
	writeFile(t, filepath.Join(music, "House", "a.mp3"), 10)
	p, err := buildPlan(collectionOf(), music, defaultExts)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if p.NewestAudio.IsZero() {
		t.Error("NewestAudio is zero, want the mod time of the newest audio file")
	}
}

func TestBuildMissingMusicDirIsAnError(t *testing.T) {
	if _, err := buildPlan(collectionOf(), filepath.Join(t.TempDir(), "nope"), defaultExts); err == nil {
		t.Fatal("Build succeeded on a missing music directory, want error")
	}
}

// A file added after the export cannot appear in it, so its absence from the
// library proves nothing. This happened for real: 7 tracks landed in the
// reference collection 40 minutes after its export, and would have been
// trashed.
func TestBuildSkipsAudioAddedAfterExport(t *testing.T) {
	music := t.TempDir()

	old := filepath.Join(music, "House", "removed-from-library.mp3")
	fresh := filepath.Join(music, "A2026-08", "just-downloaded.mp3")
	writeFile(t, old, 100)
	writeFile(t, fresh, 200)

	exportedAt := time.Now()

	// Backdate the genuinely old orphan to before the export, and make the new
	// one newer. Explicit times beat sleeping.
	if err := os.Chtimes(old, exportedAt.Add(-time.Hour), exportedAt.Add(-time.Hour)); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(fresh, exportedAt.Add(time.Minute), exportedAt.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}

	p, err := Build(collectionOf(), music, defaultExts, exportedAt)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	if len(p.Orphans) != 1 || p.Orphans[0] != old {
		t.Errorf("Orphans = %v, want only the pre-export file %q", p.Orphans, old)
	}
	if len(p.RecentlyAdded) != 1 || p.RecentlyAdded[0] != fresh {
		t.Errorf("RecentlyAdded = %v, want only the post-export file %q", p.RecentlyAdded, fresh)
	}
	if p.OrphanSize != 100 {
		t.Errorf("OrphanSize = %d, want 100: the skipped file must not be counted as reclaimable", p.OrphanSize)
	}
	if p.OnDiskAudio() != 2 {
		t.Errorf("OnDiskAudio() = %d, want 2: skipped files are still on disk", p.OnDiskAudio())
	}

	// With no export time, the same layout yields two orphans -- proving the
	// skip is driven by the timestamp and not by anything else.
	p2, err := buildPlan(collectionOf(), music, defaultExts)
	if err != nil {
		t.Fatalf("buildPlan: %v", err)
	}
	if len(p2.Orphans) != 2 {
		t.Errorf("Orphans without an export time = %v, want both files", p2.Orphans)
	}
	if len(p2.RecentlyAdded) != 0 {
		t.Errorf("RecentlyAdded without an export time = %v, want none", p2.RecentlyAdded)
	}
}
