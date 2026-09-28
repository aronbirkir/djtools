package main

import (
	"errors"
	"strings"
	"testing"

	"github.com/aronbirkir/djtools/internal/rename"
)

// TestRenameStatusCombinesLastAppliedWithFailedRescan covers applying
// successfully and then having the automatic rescan itself fail: both the
// outcome of the apply and the rescan's error need to stay visible, since
// otherwise the successful rename would vanish from the screen.
func TestRenameStatusCombinesLastAppliedWithFailedRescan(t *testing.T) {
	v := &renameView{}
	v.state.lastApplied = &rename.ApplyResult{Renamed: 3}
	v.state.phase = renameError
	v.state.err = errors.New("boom")

	msg, c := v.status()

	if !strings.Contains(msg, "Renamed 3 files.") {
		t.Errorf("status = %q, want it to mention the apply result", msg)
	}
	if !strings.Contains(msg, "Rescan failed: boom") {
		t.Errorf("status = %q, want it to mention the rescan failure", msg)
	}
	if c != pal.Error {
		t.Errorf("color = %v, want pal.Error", c)
	}
}

// TestRenameStatusPlainErrorWithoutLastApplied covers a scan failure (or an
// applyPanicked) with no prior apply result to preserve: just the error.
func TestRenameStatusPlainErrorWithoutLastApplied(t *testing.T) {
	v := &renameView{}
	v.state.phase = renameError
	v.state.err = errors.New("boom")

	msg, c := v.status()

	if msg != "boom" {
		t.Errorf("status = %q, want %q", msg, "boom")
	}
	if c != pal.Error {
		t.Errorf("color = %v, want pal.Error", c)
	}
}

func TestRenameStatusChecked(t *testing.T) {
	v := &renameView{}
	v.state.lastApplied = &rename.ApplyResult{Renamed: 3, Checked: 2}

	msg, c := v.status()

	if !strings.Contains(msg, "Renamed 3 files.") {
		t.Errorf("status = %q, want it to mention the apply result", msg)
	}
	if !strings.Contains(msg, "2 were renamed after a check rather than atomically (e.g. on exFAT, or case-only renames on HFS+).") {
		t.Errorf("status = %q, want the checked wording", msg)
	}
	if c != pal.Success {
		t.Errorf("color = %v, want pal.Success", c)
	}
}
