package prune

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFileIDFromInfo(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "track.mp3")
	if err := os.WriteFile(path, []byte("audio"), 0o644); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	id, ok := fileIDFromInfo(info)
	if !ok {
		t.Fatal("fileIDFromInfo returned ok = false for a real file")
	}
	if id.Ino == 0 {
		t.Error("Ino = 0, want a real inode number")
	}
}

// The premise of the whole matching strategy: differently spelled paths that
// resolve to the same file produce the same FileID.
func TestFileIDIsStableAcrossPathSpellings(t *testing.T) {
	dir := t.TempDir()
	// "é" as a combining sequence: 'e' + U+0301.
	nfd := filepath.Join(dir, "Café.mp3")
	// The same name precomposed: U+00E9.
	nfc := filepath.Join(dir, "Café.mp3")

	if err := os.WriteFile(nfd, []byte("audio"), 0o644); err != nil {
		t.Fatal(err)
	}

	nfdInfo, err := os.Stat(nfd)
	if err != nil {
		t.Fatal(err)
	}
	nfcInfo, err := os.Stat(nfc)
	if err != nil {
		t.Skipf("filesystem at %s is normalization-sensitive: %v", dir, err)
	}

	nfdID, ok := fileIDFromInfo(nfdInfo)
	if !ok {
		t.Fatal("no FileID for NFD spelling")
	}
	nfcID, ok := fileIDFromInfo(nfcInfo)
	if !ok {
		t.Fatal("no FileID for NFC spelling")
	}
	if nfdID != nfcID {
		t.Errorf("FileID differs by spelling: NFD %+v, NFC %+v", nfdID, nfcID)
	}
}

func TestFileIDDistinguishesDifferentFiles(t *testing.T) {
	dir := t.TempDir()
	var ids []FileID
	for _, name := range []string{"a.mp3", "b.mp3"} {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte(name), 0o644); err != nil {
			t.Fatal(err)
		}
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		id, ok := fileIDFromInfo(info)
		if !ok {
			t.Fatalf("no FileID for %s", name)
		}
		ids = append(ids, id)
	}
	if ids[0] == ids[1] {
		t.Errorf("distinct files share a FileID: %+v", ids[0])
	}
}
