package rename

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestExternalVolume exercises renameNoReplace against a real mounted
// filesystem, gated behind RENAME_TEST_DIR since it needs an actual volume --
// in particular exFAT, which unlike APFS has no atomic no-overwrite rename
// primitive and so exercises the ENOTSUP/EINVAL and checked-fallback paths
// that a temp dir on the build machine's APFS volume cannot reach.
//
// To run it against a throwaway exFAT volume:
//
//	hdiutil create -size 20m -fs ExFAT -volname DJTEST /tmp/djtest.dmg
//	hdiutil attach /tmp/djtest.dmg
//	RENAME_TEST_DIR=/Volumes/DJTEST go test ./internal/rename -run TestExternalVolume -v
//	hdiutil detach /Volumes/DJTEST
//	rm /tmp/djtest.dmg
func TestExternalVolume(t *testing.T) {
	root := os.Getenv("RENAME_TEST_DIR")
	if root == "" {
		t.Skip("set RENAME_TEST_DIR to an empty folder on the volume under test to run this")
	}
	dir, err := os.MkdirTemp(root, "djtools-rename-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })

	t.Run("normal rename", func(t *testing.T) {
		old := filepath.Join(dir, "old.mp3")
		if err := os.WriteFile(old, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
		newp := filepath.Join(dir, "new.mp3")

		checked, err := renameNoReplace(old, newp)
		t.Logf("this volume's normal-rename result: checked=%v err=%v", checked, err)

		// A volume with no RENAME_EXCL-equivalent (exFAT, observed on macOS)
		// still succeeds here, via checkedRename's plain Lstat-then-rename;
		// it just cannot do it atomically, so checked comes back true.
		if err != nil {
			t.Fatalf("renameNoReplace: %v", err)
		}
		if _, err := os.Stat(newp); err != nil {
			t.Errorf("new name missing: %v", err)
		}
		if _, err := os.Stat(old); !os.IsNotExist(err) {
			t.Errorf("old name still present (err %v)", err)
		}
	})

	t.Run("refuses an existing target", func(t *testing.T) {
		old := filepath.Join(dir, "a.mp3")
		if err := os.WriteFile(old, []byte("a"), 0o644); err != nil {
			t.Fatal(err)
		}
		target := filepath.Join(dir, "taken.mp3")
		if err := os.WriteFile(target, []byte("taken"), 0o644); err != nil {
			t.Fatal(err)
		}

		checked, err := renameNoReplace(old, target)

		if err == nil {
			t.Fatal("renameNoReplace succeeded, want a refusal")
		}
		t.Logf("this volume's refusal: checked=%v err=%v", checked, err)
		if body, rerr := os.ReadFile(target); rerr != nil || string(body) != "taken" {
			t.Errorf("target was changed: %q, %v", body, rerr)
		}
		if _, err := os.Stat(old); err != nil {
			t.Errorf("source vanished: %v", err)
		}
	})

	t.Run("case-only rename", func(t *testing.T) {
		old := filepath.Join(dir, "lower.mp3")
		if err := os.WriteFile(old, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
		newp := filepath.Join(dir, "LOWER.mp3")

		checked, err := renameNoReplace(old, newp)
		t.Logf("this volume's case-only rename result: checked=%v err=%v", checked, err)

		entries, rerr := os.ReadDir(dir)
		if rerr != nil {
			t.Fatal(rerr)
		}
		var names []string
		for _, e := range entries {
			names = append(names, e.Name())
		}
		t.Logf("directory entries after: %s", strings.Join(names, ", "))
	})
}
