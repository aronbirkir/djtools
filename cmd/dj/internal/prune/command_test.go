package prune

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
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
