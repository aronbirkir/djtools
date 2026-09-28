package main

import (
	"errors"
	"strings"
	"testing"

	"github.com/aronbirkir/djtools/internal/rename"
)

func planWith(renames int) *rename.RenamePlan {
	p := &rename.RenamePlan{Dir: "/m"}
	for i := range renames {
		p.Renames = append(p.Renames, rename.Rename{Old: string(rune('a'+i)) + ".mp3", New: string(rune('A'+i)) + ".mp3"})
	}
	return p
}

// scannedRename is a renameState immediately after a successful scan of p.
// Named to avoid colliding with the renameScanned phase constant.
func scannedRename(p *rename.RenamePlan) *renameState {
	s := &renameState{}
	s.setPattern(rename.DefaultPattern)
	s.startScan()
	s.scanDone(p, nil)
	return s
}

func TestRenameCanRenameAfterScan(t *testing.T) {
	s := scannedRename(planWith(2))
	if !s.canRename() {
		t.Error("a scan with renames should be renamable")
	}
	if scannedRename(planWith(0)).canRename() {
		t.Error("nothing to rename should not be renamable")
	}
}

func TestRenameBadPatternBlocksScanAndRename(t *testing.T) {
	s := scannedRename(planWith(2))
	s.setPattern("{album}")
	if s.patternErr == nil {
		t.Fatal("bad pattern not reported")
	}
	if s.canScan() || s.canRename() {
		t.Error("scan/rename allowed with a bad pattern")
	}
	if s.plan != nil {
		t.Error("changing the pattern should discard the preview")
	}
	s.setPattern("{title}")
	if s.patternErr != nil || !s.canScan() {
		t.Errorf("good pattern: err %v, canScan %v", s.patternErr, s.canScan())
	}
}

func TestRenameInputsChangedDiscardsPreview(t *testing.T) {
	s := scannedRename(planWith(2))
	s.lastApplied = &rename.ApplyResult{Renamed: 1}
	s.inputsChanged()
	if s.phase != renameIdle || s.plan != nil || s.lastApplied != nil || s.canRename() {
		t.Errorf("after inputsChanged: phase %v plan %v applied %v", s.phase, s.plan, s.lastApplied)
	}
}

func TestRenameBusyIgnoresChanges(t *testing.T) {
	for name, setup := range map[string]func(*renameState){
		"scanning": func(s *renameState) { s.startScan() },
		"applying": func(s *renameState) { s.askConfirm(); s.startApply() },
	} {
		s := scannedRename(planWith(2))
		setup(s)
		before := s.phase
		s.inputsChanged()
		s.setPattern("{title}")
		if s.phase != before {
			t.Errorf("%s: phase changed to %v", name, s.phase)
		}
		if s.startScan() {
			t.Errorf("%s: a scan started while busy", name)
		}
	}
}

func TestRenameConfirmFlow(t *testing.T) {
	s := scannedRename(planWith(2))
	s.askConfirm()
	if s.phase != renameConfirming {
		t.Fatalf("phase = %v, want confirming", s.phase)
	}
	s.cancelConfirm()
	if s.phase != renameScanned {
		t.Fatalf("phase = %v after cancel", s.phase)
	}
	s.askConfirm()
	p, ok := s.startApply()
	if !ok || p == nil || s.phase != renameApplying {
		t.Fatalf("startApply = %v %v, phase %v", p, ok, s.phase)
	}
	res := &rename.ApplyResult{Renamed: 2}
	s.applyDone(res)
	if s.phase != renameIdle || s.plan != nil || s.lastApplied != res {
		t.Errorf("after applyDone: phase %v plan %v applied %v", s.phase, s.plan, s.lastApplied)
	}
	// The automatic rescan keeps the result on screen.
	s.startScan()
	if s.lastApplied != res {
		t.Error("rescan dropped the last result")
	}
}

// TestRenameLastAppliedSurvivesOnlyTheAutomaticRescan pins down that the
// result only stays on screen through the one rescan applyDone itself
// triggers: any scan after that -- automatic or the user pressing Scan --
// finds a plan from arbitrarily long ago and must not keep describing it.
func TestRenameLastAppliedSurvivesOnlyTheAutomaticRescan(t *testing.T) {
	s := scannedRename(planWith(2))
	s.askConfirm()
	s.startApply()
	res := &rename.ApplyResult{Renamed: 2}
	s.applyDone(res)

	if !s.startScan() {
		t.Fatal("the automatic rescan did not start")
	}
	if s.lastApplied != res {
		t.Error("the automatic rescan should keep lastApplied")
	}
	s.scanDone(planWith(0), nil)

	if !s.startScan() {
		t.Fatal("the next scan did not start")
	}
	if s.lastApplied != nil {
		t.Error("a scan after the automatic rescan should clear lastApplied")
	}
}

// TestRenameManualScanClearsLastApplied covers pressing Scan (rather than
// relying on the automatic rescan) right after applying: it must not show a
// stale result next to a plan it has nothing to do with.
func TestRenameManualScanClearsLastApplied(t *testing.T) {
	s := scannedRename(planWith(2))
	s.askConfirm()
	s.startApply()
	s.applyDone(&rename.ApplyResult{Renamed: 2})

	// Simulate the user clicking Scan again before or instead of the
	// automatic one by calling startScan twice.
	s.startScan()
	s.scanDone(planWith(0), nil)
	s.startScan()

	if s.lastApplied != nil {
		t.Error("a manual scan should clear lastApplied")
	}
}

func TestRenameApplyPanicked(t *testing.T) {
	s := scannedRename(planWith(2))
	s.askConfirm()
	s.startApply()

	s.applyPanicked(errors.New("boom"))

	if s.phase != renameError {
		t.Fatalf("phase = %v, want renameError", s.phase)
	}
	if s.err == nil || !strings.Contains(s.err.Error(), "Renaming stopped part way: boom") {
		t.Errorf("err = %v, want it to mention stopping part way", s.err)
	}
	if !strings.Contains(s.err.Error(), "Rescan to see what is left.") {
		t.Errorf("err = %v, want it to suggest rescanning", s.err)
	}
	if s.lastApplied != nil {
		t.Errorf("lastApplied = %v, want nil after a panic", s.lastApplied)
	}
	if s.plan != nil {
		t.Errorf("plan = %v, want nil after a panic", s.plan)
	}
}

func TestRenameStartApplyNeedsConfirm(t *testing.T) {
	s := scannedRename(planWith(2))
	if _, ok := s.startApply(); ok {
		t.Error("startApply without confirming")
	}
	s.setPattern("{album}")
	s.askConfirm()
	if s.phase == renameConfirming {
		t.Error("confirm opened with a bad pattern")
	}
}

func TestRenameScanFailure(t *testing.T) {
	s := &renameState{}
	s.startScan()
	s.scanDone(nil, errors.New("boom"))
	if s.phase != renameError || s.err == nil {
		t.Errorf("phase %v err %v", s.phase, s.err)
	}
}

func TestRenameFolder(t *testing.T) {
	if _, err := renameFolder("  ", "/Users/me"); err == nil {
		t.Error("empty folder accepted")
	}
	if got, err := renameFolder("~/Downloads", "/Users/me"); err != nil || got != "/Users/me/Downloads" {
		t.Errorf("renameFolder = %q, %v", got, err)
	}
}
