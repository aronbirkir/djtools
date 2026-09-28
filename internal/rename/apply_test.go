package rename

import (
	"os"
	"path/filepath"
	"testing"
)

func TestApplyRenames(t *testing.T) {
	dir := t.TempDir()
	writeMP3(t, dir, "track01.mp3", fullTags)
	plan := mustPlan(t, dir, DefaultPattern)

	res := Apply(plan)

	if res.Renamed != 1 || len(res.Failures) != 0 {
		t.Fatalf("Apply = %+v", res)
	}
	if _, err := os.Stat(filepath.Join(dir, "123 04A Daft Punk - One More Time.mp3")); err != nil {
		t.Errorf("new name missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "track01.mp3")); !os.IsNotExist(err) {
		t.Errorf("old name still present (err %v)", err)
	}
}

func TestApplyNeverOverwritesATargetCreatedAfterPlanning(t *testing.T) {
	dir := t.TempDir()
	writeMP3(t, dir, "x.mp3", map[string]string{TokenArtist: "a", TokenTitle: "b"})
	plan := mustPlan(t, dir, "{artist} - {title}")
	target := filepath.Join(dir, "A - B.mp3")
	if err := os.WriteFile(target, []byte("precious"), 0o644); err != nil {
		t.Fatal(err)
	}

	res := Apply(plan)

	if res.Renamed != 0 || len(res.Failures) != 1 || res.Failures[0].Name != "x.mp3" {
		t.Fatalf("Apply = %+v, want x.mp3 to fail", res)
	}
	if body, err := os.ReadFile(target); err != nil || string(body) != "precious" {
		t.Errorf("target was changed: %q, %v", body, err)
	}
	if _, err := os.Stat(filepath.Join(dir, "x.mp3")); err != nil {
		t.Errorf("source vanished: %v", err)
	}
}

func TestApplyContinuesPastAFailure(t *testing.T) {
	dir := t.TempDir()
	writeMP3(t, dir, "a.mp3", map[string]string{TokenTitle: "one"})
	writeMP3(t, dir, "b.mp3", map[string]string{TokenTitle: "two"})
	plan := mustPlan(t, dir, "{title}")
	if err := os.WriteFile(filepath.Join(dir, "One.mp3"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	res := Apply(plan)

	if res.Renamed != 1 || len(res.Failures) != 1 {
		t.Errorf("Apply = %+v, want one renamed and one failure", res)
	}
	if _, err := os.Stat(filepath.Join(dir, "Two.mp3")); err != nil {
		t.Errorf("b.mp3 was not renamed: %v", err)
	}
}

func TestApplyCaseOnlyRename(t *testing.T) {
	dir := t.TempDir()
	requireCaseInsensitive(t, dir)
	writeMP3(t, dir, "daft punk - one.mp3", map[string]string{TokenArtist: "daft punk", TokenTitle: "one"})
	plan := mustPlan(t, dir, "{artist} - {title}")

	res := Apply(plan)

	if res.Renamed != 1 || len(res.Failures) != 0 {
		t.Fatalf("Apply = %+v", res)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != "Daft Punk - One.mp3" {
		t.Errorf("directory holds %v, want only Daft Punk - One.mp3", entries)
	}
}
