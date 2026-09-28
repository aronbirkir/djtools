package rename

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const renamedName = "123 04A Daft Punk - One More Time.mp3"

func commandFixture(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	writeMP3(t, dir, "track01.mp3", fullTags)
	writeMP3(t, dir, "untagged.mp3", nil)
	return dir
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func TestRunDryRunRenamesNothing(t *testing.T) {
	dir := commandFixture(t)
	var out, errOut strings.Builder

	code := Run([]string{"--dry-run", dir}, &out, &errOut, strings.NewReader(""))

	if code != 0 {
		t.Fatalf("exit = %d, stderr:\n%s", code, errOut.String())
	}
	for _, want := range []string{"track01.mp3", renamedName, "SKIPPED", "untagged.mp3: no {bpm} tag",
		"1 to rename, 1 skipped, 0 already match", "dry run"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("output missing %q:\n%s", want, out.String())
		}
	}
	if !exists(filepath.Join(dir, "track01.mp3")) {
		t.Error("dry run renamed a file")
	}
}

func TestRunFlagsAfterFolder(t *testing.T) {
	dir := commandFixture(t)
	var out, errOut strings.Builder
	if code := Run([]string{dir, "--dry-run"}, &out, &errOut, strings.NewReader("")); code != 0 {
		t.Fatalf("exit = %d, stderr:\n%s", code, errOut.String())
	}
	if !exists(filepath.Join(dir, "track01.mp3")) {
		t.Error("--dry-run after the folder was ignored")
	}
}

func TestRunDeclineRenamesNothing(t *testing.T) {
	for _, answer := range []string{"n\n", "\n", ""} {
		dir := commandFixture(t)
		var out, errOut strings.Builder
		if code := Run([]string{dir}, &out, &errOut, strings.NewReader(answer)); code != 1 {
			t.Errorf("answer %q: exit = %d, want 1", answer, code)
		}
		if !exists(filepath.Join(dir, "track01.mp3")) {
			t.Errorf("answer %q renamed a file", answer)
		}
	}
}

func TestRunConfirmRenames(t *testing.T) {
	dir := commandFixture(t)
	var out, errOut strings.Builder

	code := Run([]string{dir}, &out, &errOut, strings.NewReader("y\n"))

	if code != 0 {
		t.Fatalf("exit = %d, stderr:\n%s", code, errOut.String())
	}
	if !exists(filepath.Join(dir, renamedName)) {
		t.Error("file was not renamed")
	}
	if !strings.Contains(out.String(), "Renamed 1 of 1 files.") {
		t.Errorf("output:\n%s", out.String())
	}
}

func TestRunYesSkipsPrompt(t *testing.T) {
	dir := commandFixture(t)
	var out, errOut strings.Builder
	if code := Run([]string{"--yes", dir}, &out, &errOut, strings.NewReader("")); code != 0 {
		t.Fatalf("exit = %d, stderr:\n%s", code, errOut.String())
	}
	if !exists(filepath.Join(dir, renamedName)) {
		t.Error("--yes did not rename")
	}
}

func TestRunPatternFlag(t *testing.T) {
	dir := commandFixture(t)
	var out, errOut strings.Builder
	if code := Run([]string{"--yes", "--pattern", "{artist} - {title}", dir}, &out, &errOut, strings.NewReader("")); code != 0 {
		t.Fatalf("exit = %d, stderr:\n%s", code, errOut.String())
	}
	if !exists(filepath.Join(dir, "Daft Punk - One More Time.mp3")) {
		t.Error("--pattern was not used")
	}
}

func TestRunNothingToRenameDoesNotPrompt(t *testing.T) {
	dir := t.TempDir()
	writeMP3(t, dir, "untagged.mp3", nil)
	var out, errOut strings.Builder
	if code := Run([]string{dir}, &out, &errOut, strings.NewReader("")); code != 0 {
		t.Errorf("exit = %d, want 0", code)
	}
	if strings.Contains(out.String(), "[y/N]") {
		t.Errorf("prompted with nothing to rename:\n%s", out.String())
	}
}

func TestRunErrors(t *testing.T) {
	dir := commandFixture(t)
	for name, args := range map[string][]string{
		"bad pattern":    {"--pattern", "{album}", dir},
		"no folder":      {},
		"two folders":    {dir, dir},
		"missing folder": {filepath.Join(dir, "nope")},
		"unknown flag":   {"--nonsense", dir},
	} {
		var out, errOut strings.Builder
		if code := Run(args, &out, &errOut, strings.NewReader("y\n")); code != 1 {
			t.Errorf("%s: exit = %d, want 1", name, code)
		}
	}
	if !exists(filepath.Join(dir, "track01.mp3")) {
		t.Error("an erroring run renamed a file")
	}
}

func TestRunHelpExitsZero(t *testing.T) {
	var out, errOut strings.Builder
	if code := Run([]string{"--help"}, &out, &errOut, strings.NewReader("")); code != 0 {
		t.Errorf("exit = %d, want 0", code)
	}
	if !strings.Contains(errOut.String(), "-pattern") {
		t.Errorf("usage should list the flags:\n%s", errOut.String())
	}
}
