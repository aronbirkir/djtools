package main

import (
	"errors"

	"github.com/aronbirkir/djtools/internal/rename"
)

type renamePhase int

const (
	renameIdle renamePhase = iota
	renameScanning
	renameScanned
	renameConfirming
	renameApplying
	renameError
)

// renameState is the MP3 Rename screen minus its widgets. The rules about
// what may be renamed live in internal/rename; this decides only when the
// screen lets the user ask.
type renameState struct {
	phase      renamePhase
	pattern    rename.Pattern
	patternErr error
	plan       *rename.RenamePlan
	err        error
	// lastApplied survives the automatic rescan after renaming, so the
	// outcome stays on screen next to the fresh preview.
	lastApplied *rename.ApplyResult
}

func (s *renameState) busy() bool {
	return s.phase == renameScanning || s.phase == renameApplying
}

// setPattern parses the pattern as it is typed and discards a preview made
// with the old one.
func (s *renameState) setPattern(text string) {
	if s.busy() {
		return
	}
	s.pattern, s.patternErr = rename.ParsePattern(text)
	s.inputsChanged()
}

func (s *renameState) inputsChanged() {
	if s.busy() {
		return
	}
	s.plan, s.err, s.lastApplied = nil, nil, nil
	s.phase = renameIdle
}

func (s *renameState) fail(err error) {
	if s.busy() {
		return
	}
	s.plan = nil
	s.phase, s.err = renameError, err
}

func (s *renameState) canScan() bool {
	return !s.busy() && s.phase != renameConfirming && s.patternErr == nil
}

func (s *renameState) startScan() bool {
	if !s.canScan() {
		return false
	}
	s.plan, s.err = nil, nil
	s.phase = renameScanning
	return true
}

func (s *renameState) scanDone(p *rename.RenamePlan, err error) {
	if err != nil {
		s.phase, s.err = renameError, err
		return
	}
	s.phase, s.plan = renameScanned, p
}

func (s *renameState) canRename() bool {
	return s.phase == renameScanned && s.patternErr == nil && s.plan != nil && len(s.plan.Renames) > 0
}

func (s *renameState) askConfirm() {
	if s.canRename() {
		s.phase = renameConfirming
	}
}

func (s *renameState) cancelConfirm() {
	if s.phase == renameConfirming {
		s.phase = renameScanned
	}
}

func (s *renameState) startApply() (*rename.RenamePlan, bool) {
	if s.phase != renameConfirming || s.plan == nil {
		return nil, false
	}
	s.phase = renameApplying
	return s.plan, true
}

// applyDone records the outcome. The preview is stale once files have moved,
// so the view rescans straight away.
func (s *renameState) applyDone(res *rename.ApplyResult) {
	s.lastApplied = res
	s.plan = nil
	s.phase = renameIdle
}

func renameFolder(text, home string) (string, error) {
	dir := expandHome(text, home)
	if dir == "" {
		return "", errors.New("choose a folder")
	}
	return dir, nil
}
