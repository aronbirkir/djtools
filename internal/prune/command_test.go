package prune

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// fixture builds a music dir plus an XML export naming only the keepers.
func fixture(t *testing.T, keepers, orphans int) (musicDir, xmlPath string) {
	t.Helper()
	root := t.TempDir()
	musicDir = filepath.Join(root, "music")

	var locations []string
	for i := range keepers {
		p := filepath.Join(musicDir, "House", filepath.Base(pad("keep", i)))
		writeFile(t, p, 100)
		locations = append(locations, "    <TRACK TrackID=\""+pad("k", i)+"\" Location=\""+locationFor(p)+"\"/>")
	}
	for i := range orphans {
		writeFile(t, filepath.Join(musicDir, "Disco", filepath.Base(pad("orphan", i))), 200)
	}

	xmlPath = filepath.Join(root, "rekordbox.xml")
	doc := `<?xml version="1.0" encoding="UTF-8"?>
<DJ_PLAYLISTS Version="1.0.0">
  <PRODUCT Name="rekordbox" Version="7.2.18"/>
  <COLLECTION Entries="` + itoa(keepers) + `">
` + strings.Join(locations, "\n") + `
  </COLLECTION>
  <PLAYLISTS><NODE Type="0" Name="ROOT" Count="1"/></PLAYLISTS>
</DJ_PLAYLISTS>`
	if err := os.WriteFile(xmlPath, []byte(doc), 0o644); err != nil {
		t.Fatal(err)
	}
	return musicDir, xmlPath
}

func pad(prefix string, i int) string { return prefix + "-" + itoa(i) + ".mp3" }
func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var b []byte
	for i > 0 {
		b = append([]byte{byte('0' + i%10)}, b...)
		i /= 10
	}
	return string(b)
}

func TestRunDryRunTrashesNothing(t *testing.T) {
	music, xml := fixture(t, 600, 5)
	var out, errOut strings.Builder

	code := Run([]string{"--xml", xml, "--music", music, "--dry-run"},
		&out, &errOut, strings.NewReader(""))

	if code != 0 {
		t.Errorf("exit = %d, want 0. stderr:\n%s", code, errOut.String())
	}
	if !strings.Contains(out.String(), "dry run") {
		t.Errorf("output should say it is a dry run:\n%s", out.String())
	}
	if _, err := os.Stat(filepath.Join(music, "Disco", "orphan-0.mp3")); err != nil {
		t.Errorf("dry run removed a file: %v", err)
	}
}

func TestRunDeclinedConfirmationTrashesNothing(t *testing.T) {
	music, xml := fixture(t, 600, 5)
	var out, errOut strings.Builder

	code := Run([]string{"--xml", xml, "--music", music},
		&out, &errOut, strings.NewReader("n\n"))

	if code != 1 {
		t.Errorf("exit = %d, want 1 when the user declines", code)
	}
	if _, err := os.Stat(filepath.Join(music, "Disco", "orphan-0.mp3")); err != nil {
		t.Errorf("declining removed a file: %v", err)
	}
}

func TestRunEmptyAnswerIsNo(t *testing.T) {
	music, xml := fixture(t, 600, 5)
	var out, errOut strings.Builder
	if code := Run([]string{"--xml", xml, "--music", music},
		&out, &errOut, strings.NewReader("\n")); code != 1 {
		t.Errorf("exit = %d, want 1: a bare newline must not confirm a deletion", code)
	}
}

func TestRunGuardAbortStopsBeforePrompting(t *testing.T) {
	// 600 keepers but only 5 named in the XML: the library is far too small.
	music, xml := fixture(t, 5, 600)
	var out, errOut strings.Builder

	// Reading "y" would confirm, so if the guard works this is never consumed.
	code := Run([]string{"--xml", xml, "--music", music},
		&out, &errOut, strings.NewReader("y\n"))

	if code != 1 {
		t.Errorf("exit = %d, want 1", code)
	}
	if !strings.Contains(out.String()+errOut.String(), "playlist export") {
		t.Errorf("expected the small-library guard to fire:\n%s\n%s", out.String(), errOut.String())
	}
	if _, err := os.Stat(filepath.Join(music, "Disco", "orphan-0.mp3")); err != nil {
		t.Errorf("guard abort still removed a file: %v", err)
	}
}

func TestRunReportFileIsWritten(t *testing.T) {
	music, xml := fixture(t, 600, 5)
	reportPath := filepath.Join(t.TempDir(), "report.txt")
	var out, errOut strings.Builder

	code := Run([]string{"--xml", xml, "--music", music, "--dry-run", "--report", reportPath},
		&out, &errOut, strings.NewReader(""))
	if code != 0 {
		t.Fatalf("exit = %d, want 0. stderr:\n%s", code, errOut.String())
	}

	body, err := os.ReadFile(reportPath)
	if err != nil {
		t.Fatalf("reading report: %v", err)
	}
	if !strings.Contains(string(body), "# ORPHANS (5)") {
		t.Errorf("report missing orphan section:\n%s", body)
	}
}

func TestRunMissingXMLIsAnError(t *testing.T) {
	music, _ := fixture(t, 600, 1)
	var out, errOut strings.Builder
	if code := Run([]string{"--xml", filepath.Join(music, "nope.xml"), "--music", music},
		&out, &errOut, strings.NewReader("")); code == 0 {
		t.Error("exit = 0, want non-zero for a missing XML file")
	}
}

func TestRunUnknownFlagIsAnError(t *testing.T) {
	var out, errOut strings.Builder
	if code := Run([]string{"--nonsense"}, &out, &errOut, strings.NewReader("")); code == 0 {
		t.Error("exit = 0, want non-zero for an unknown flag")
	}
}

// --force covers findings a user may know to be wrong, but must not reach an
// unresolved library path: that means we could not establish what is in the
// library, so no deletion is justified. Dropping that gate left the whole suite
// green, because nothing produced such a finding.
//
// ENOTDIR is provoked by pointing a Location through a regular file. Unlike an
// unreadable directory it does not also break the walk, so Build still returns a
// plan and the finding is reachable.
func TestRunForceCannotOverrideUnresolved(t *testing.T) {
	music, xml := fixture(t, 600, 1)

	blocker := filepath.Join(music, "House", "keep-0.mp3")
	through := filepath.Join(blocker, "inner.mp3")

	body, err := os.ReadFile(xml)
	if err != nil {
		t.Fatal(err)
	}
	extra := "    <TRACK TrackID=\"unresolvable\" Location=\"" +
		locationFor(through) + "\"/>\n  </COLLECTION>"
	if err := os.WriteFile(xml,
		[]byte(strings.Replace(string(body), "  </COLLECTION>", extra, 1)), 0o644); err != nil {
		t.Fatal(err)
	}

	orphan := filepath.Join(music, "Disco", "orphan-0.mp3")

	var out, errOut strings.Builder
	// The most dangerous combination available: force past the guards and skip
	// the prompt.
	code := Run([]string{"--xml", xml, "--music", music, "--force", "--yes"},
		&out, &errOut, strings.NewReader("y\n"))

	if code != 1 {
		t.Fatalf("exit = %d, want 1: --force must not override an unresolved library path.\nstdout:\n%s\nstderr:\n%s",
			code, out.String(), errOut.String())
	}
	if !strings.Contains(out.String(), "cannot be overridden with --force") {
		t.Errorf("expected the blocked-findings message:\n%s", out.String())
	}
	if _, err := os.Stat(orphan); err != nil {
		t.Errorf("a file was removed despite the refusal: %v", err)
	}
}

// The export time must actually reach Build, or the protection keeping freshly
// downloaded music out of the Trash silently disappears. Removing the zero-time
// refusal in Run left the whole suite green, so this exercises the plumbing
// end to end rather than the guard clause.
func TestRunSkipsAudioNewerThanTheExport(t *testing.T) {
	music, xml := fixture(t, 600, 0)

	xi, err := os.Stat(xml)
	if err != nil {
		t.Fatal(err)
	}

	fresh := filepath.Join(music, "A2026-08", "just-downloaded.mp3")
	writeFile(t, fresh, 200)
	later := xi.ModTime().Add(time.Hour)
	if err := os.Chtimes(fresh, later, later); err != nil {
		t.Fatal(err)
	}

	var out, errOut strings.Builder
	code := Run([]string{"--xml", xml, "--music", music, "--dry-run"},
		&out, &errOut, strings.NewReader(""))
	if code != 0 {
		t.Fatalf("exit = %d, want 0. stderr:\n%s", code, errOut.String())
	}
	if !strings.Contains(out.String(), "skipped rather than trashed") {
		t.Errorf("a file newer than the export should have been skipped:\n%s", out.String())
	}
	if _, err := os.Stat(fresh); err != nil {
		t.Errorf("the newer file was removed: %v", err)
	}
}

func TestRunHelpExitsZero(t *testing.T) {
	var out, errOut strings.Builder
	if code := Run([]string{"--help"}, &out, &errOut, strings.NewReader("")); code != 0 {
		t.Errorf("exit = %d, want 0: --help is a successful request, not a failure", code)
	}
	if !strings.Contains(errOut.String(), "-dry-run") {
		t.Errorf("usage should list the flags:\n%s", errOut.String())
	}
}
