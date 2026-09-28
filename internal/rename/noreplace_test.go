package rename

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestCheckedRenameRefusesExistingDifferentFile pins down checkedRename's own
// contract, independent of which platform file calls it.
func TestCheckedRenameRefusesExistingDifferentFile(t *testing.T) {
	dir := t.TempDir()
	old := filepath.Join(dir, "old.mp3")
	if err := os.WriteFile(old, []byte("a"), 0o644); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(dir, "new.mp3")
	if err := os.WriteFile(target, []byte("b"), 0o644); err != nil {
		t.Fatal(err)
	}

	err := checkedRename(old, target)

	if err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Errorf("checkedRename = %v, want an already-exists error", err)
	}
	if _, err := os.Stat(old); err != nil {
		t.Errorf("old file vanished: %v", err)
	}
	if body, err := os.ReadFile(target); err != nil || string(body) != "b" {
		t.Errorf("target was changed: %q, %v", body, err)
	}
}

func TestCheckedRenameRenamesOtherwise(t *testing.T) {
	dir := t.TempDir()
	old := filepath.Join(dir, "old.mp3")
	if err := os.WriteFile(old, []byte("a"), 0o644); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(dir, "new.mp3")

	if err := checkedRename(old, target); err != nil {
		t.Fatalf("checkedRename: %v", err)
	}
	if _, err := os.Stat(target); err != nil {
		t.Errorf("new name missing: %v", err)
	}
	if _, err := os.Stat(old); !os.IsNotExist(err) {
		t.Errorf("old name still present (err %v)", err)
	}
}
