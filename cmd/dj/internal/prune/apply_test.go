package prune

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fakeRunner records calls instead of executing them, so trashing is testable
// without touching the real Trash.
type fakeRunner struct {
	calls   [][]string
	failOn  int // 1-based call index to fail, 0 for none
	callNum int
}

func (f *fakeRunner) Run(name string, args ...string) error {
	f.callNum++
	f.calls = append(f.calls, append([]string{name}, args...))
	if f.failOn == f.callNum {
		return errors.New("simulated trash failure")
	}
	return nil
}

func paths(n int) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = fmt.Sprintf("/m/track%04d.mp3", i)
	}
	return out
}

func TestTrashBatching(t *testing.T) {
	for _, tt := range []struct {
		n         int
		wantCalls int
		wantFirst int
		wantLast  int
	}{
		{n: 1, wantCalls: 1, wantFirst: 1, wantLast: 1},
		{n: 199, wantCalls: 1, wantFirst: 199, wantLast: 199},
		{n: 200, wantCalls: 1, wantFirst: 200, wantLast: 200},
		{n: 201, wantCalls: 2, wantFirst: 200, wantLast: 1},
		{n: 400, wantCalls: 2, wantFirst: 200, wantLast: 200},
		{n: 6657, wantCalls: 34, wantFirst: 200, wantLast: 57},
	} {
		t.Run(fmt.Sprintf("%d paths", tt.n), func(t *testing.T) {
			r := &fakeRunner{}
			done, errs := Trash(r, "/m", paths(tt.n))
			if len(errs) != 0 {
				t.Fatalf("errors = %v, want none", errs)
			}
			if done != tt.n {
				t.Errorf("done = %d, want %d", done, tt.n)
			}
			if len(r.calls) != tt.wantCalls {
				t.Fatalf("calls = %d, want %d", len(r.calls), tt.wantCalls)
			}
			// calls[i][0] is the binary, so subtract it from the arg count.
			if got := len(r.calls[0]) - 1; got != tt.wantFirst {
				t.Errorf("first batch = %d paths, want %d", got, tt.wantFirst)
			}
			if got := len(r.calls[len(r.calls)-1]) - 1; got != tt.wantLast {
				t.Errorf("last batch = %d paths, want %d", got, tt.wantLast)
			}
			if r.calls[0][0] != TrashPath {
				t.Errorf("binary = %q, want %q", r.calls[0][0], TrashPath)
			}
		})
	}
}

func TestTrashEmptyInputRunsNothing(t *testing.T) {
	r := &fakeRunner{}
	done, errs := Trash(r, "/m", nil)
	if done != 0 || len(errs) != 0 || len(r.calls) != 0 {
		t.Errorf("done=%d errs=%v calls=%d, want 0/nil/0", done, errs, len(r.calls))
	}
}

// A failing batch must not strand the batches after it.
func TestTrashContinuesPastAFailedBatch(t *testing.T) {
	r := &fakeRunner{failOn: 1}
	done, errs := Trash(r, "/m", paths(400))
	if len(errs) != 1 {
		t.Fatalf("errs = %v, want exactly 1", errs)
	}
	if done != 200 {
		t.Errorf("done = %d, want 200 (second batch still ran)", done)
	}
	if len(r.calls) != 2 {
		t.Errorf("calls = %d, want 2", len(r.calls))
	}
}

func TestTrashPassesExactPaths(t *testing.T) {
	r := &fakeRunner{}
	want := []string{
		"/m/Easy/11A 108 Bonga (Original Mix) - Dj Pantelis.mp3",
		"/m/Pop/08B 122 Þú komst við hjartað í mér - Hjaltalín.mp3",
	}
	if _, errs := Trash(r, "/m", want); len(errs) != 0 {
		t.Fatalf("errors = %v", errs)
	}
	got := r.calls[0][1:]
	if len(got) != len(want) {
		t.Fatalf("got %d paths, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("path %d = %q, want %q", i, got[i], want[i])
		}
	}
}

// The containment gate: one path outside the root refuses the whole operation.
// All-or-nothing matters more than partial progress here -- if the path list is
// wrong, the right response is to move nothing and let a human look.
func TestTrashRefusesPathsOutsideRoot(t *testing.T) {
	for _, tt := range []struct {
		name    string
		root    string
		paths   []string
		wantSub string
	}{
		{
			name:    "path outside root",
			root:    "/m",
			paths:   []string{"/m/ok.mp3", "/etc/passwd"},
			wantSub: "lies outside",
		},
		{
			name:    "the root itself",
			root:    "/m",
			paths:   []string{"/m"},
			wantSub: "music directory itself",
		},
		{
			name:    "relative path",
			root:    "/m",
			paths:   []string{"ok.mp3"},
			wantSub: "not absolute",
		},
		{
			// A sibling whose name merely starts with the root's name.
			name:    "sibling directory prefix",
			root:    "/m",
			paths:   []string{"/m-archive/old.mp3"},
			wantSub: "lies outside",
		},
		{
			// Traversal that resolves outside root after cleaning.
			name:    "dot-dot escape",
			root:    "/m",
			paths:   []string{"/m/../etc/passwd"},
			wantSub: "lies outside",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			r := &fakeRunner{}
			done, errs := Trash(r, tt.root, tt.paths)
			if done != 0 {
				t.Errorf("done = %d, want 0: nothing may be trashed", done)
			}
			if len(errs) != 1 {
				t.Fatalf("errs = %v, want exactly one refusal", errs)
			}
			if !strings.Contains(errs[0].Error(), tt.wantSub) {
				t.Errorf("error %q does not mention %q", errs[0], tt.wantSub)
			}
			if len(r.calls) != 0 {
				t.Errorf("the trash binary was invoked %d times, want 0", len(r.calls))
			}
		})
	}
}

// One bad path among many good ones must still trash nothing, rather than
// getting most of the way through and then stopping.
func TestTrashRefusalIsAllOrNothing(t *testing.T) {
	good := paths(300)
	withBad := append(append([]string{}, good...), "/elsewhere/evil.mp3")

	r := &fakeRunner{}
	done, errs := Trash(r, "/m", withBad)
	if done != 0 || len(errs) != 1 || len(r.calls) != 0 {
		t.Errorf("done=%d errs=%v calls=%d, want 0/one refusal/0 -- a single bad path must stop everything",
			done, errs, len(r.calls))
	}
}

func TestRemoveEmptyDirsDeepestFirst(t *testing.T) {
	root := t.TempDir()
	deep := filepath.Join(root, "A2026-01", "sub")
	if err := os.MkdirAll(deep, 0o755); err != nil {
		t.Fatal(err)
	}

	removed, errs := RemoveEmptyDirs(root)
	if len(errs) != 0 {
		t.Fatalf("errors = %v", errs)
	}
	if len(removed) != 2 {
		t.Fatalf("removed = %v, want both directories", removed)
	}
	// The child must be removed before the parent, otherwise the parent is not
	// empty yet.
	if removed[0] != deep {
		t.Errorf("removed[0] = %q, want the deepest directory %q", removed[0], deep)
	}
	if _, err := os.Stat(filepath.Join(root, "A2026-01")); !os.IsNotExist(err) {
		t.Error("parent directory still exists")
	}
}

func TestRemoveEmptyDirsKeepsNonEmpty(t *testing.T) {
	root := t.TempDir()
	withAudio := filepath.Join(root, "House")
	withLeftover := filepath.Join(root, "Disco")
	for _, d := range []string{withAudio, withLeftover} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(withAudio, "keep.mp3"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	// A leftover is still an entry, so the directory is not empty and must
	// survive: removing it would exceed what the user confirmed.
	if err := os.WriteFile(filepath.Join(withLeftover, ".DS_Store"), nil, 0o644); err != nil {
		t.Fatal(err)
	}

	removed, errs := RemoveEmptyDirs(root)
	if len(errs) != 0 {
		t.Fatalf("errors = %v", errs)
	}
	if len(removed) != 0 {
		t.Errorf("removed = %v, want none", removed)
	}
}

func TestRemoveEmptyDirsNeverRemovesRoot(t *testing.T) {
	root := t.TempDir()
	removed, errs := RemoveEmptyDirs(root)
	if len(errs) != 0 {
		t.Fatalf("errors = %v", errs)
	}
	if len(removed) != 0 {
		t.Errorf("removed = %v, want none: the music dir itself must survive", removed)
	}
	if _, err := os.Stat(root); err != nil {
		t.Errorf("music dir was removed: %v", err)
	}
}
