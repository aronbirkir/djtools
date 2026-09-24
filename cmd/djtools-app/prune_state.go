package main

import (
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/aronbirkir/djtools/internal/prune"
)

type phase int

const (
	phaseIdle phase = iota
	phaseScanning
	phaseScanned
	phaseConfirming
	phaseTrashing
	phaseDone
	phaseError
)

// maxFindingLines caps how many findings the screen lists. An unresolved
// library produces one finding per failed stat, which could be thousands.
const maxFindingLines = 6

// pruneState is the Rekordbox Prune screen minus its widgets. Every rule that
// decides whether files may be trashed lives here, where it can be tested
// without a window. prune.Apply re-checks the same gates regardless.
type pruneState struct {
	phase    phase
	result   *prune.Result
	applied  *prune.ApplyResult
	err      error
	override bool

	// musicHome is ~/Music. It usually holds more than the rekordbox
	// collection -- an Apple Music library, GarageBand projects -- all of which
	// would count as orphans, so overrides are never offered there.
	musicHome string
}

func (s *pruneState) busy() bool {
	return s.phase == phaseScanning || s.phase == phaseTrashing
}

func (s *pruneState) reset() {
	s.result, s.applied, s.err, s.override = nil, nil, nil, false
}

// inputsChanged discards a scan that no longer describes what is on screen.
func (s *pruneState) inputsChanged() {
	if s.busy() {
		return
	}
	s.reset()
	s.phase = phaseIdle
}

// fail shows err in place of any result.
func (s *pruneState) fail(err error) {
	if s.busy() {
		return
	}
	s.reset()
	s.phase, s.err = phaseError, err
}

// notice shows err beside the current scan without discarding it, for
// problems that moved nothing, such as not finding a place for the report.
func (s *pruneState) notice(err error) {
	if s.busy() {
		return
	}
	s.err = err
}

func (s *pruneState) startScan() bool {
	if s.busy() {
		return false
	}
	s.reset()
	s.phase = phaseScanning
	return true
}

func (s *pruneState) scanDone(r *prune.Result, err error) {
	if err != nil {
		s.phase, s.err = phaseError, err
		return
	}
	s.phase, s.result = phaseScanned, r
}

// coversMusicHome reports whether the scanned folder is ~/Music or contains
// it (home, /Users, /), so that everything in ~/Music would count as orphans.
func (s *pruneState) coversMusicHome() bool {
	if s.result == nil || s.result.Plan == nil || s.musicHome == "" {
		return false
	}
	dir := strings.ToLower(filepath.Clean(s.result.Plan.MusicDir))
	home := strings.ToLower(filepath.Clean(s.musicHome))
	if dir == home {
		return true
	}
	rel, err := filepath.Rel(dir, home)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// overrideAllowed reports whether the forceable findings may be overridden at
// all: there must be some, nothing unforceable, and Music must not be ~/Music
// or a folder containing it.
func (s *pruneState) overrideAllowed() bool {
	r := s.result
	return r != nil && len(r.Forceable()) > 0 && len(r.Blocked()) == 0 && !s.coversMusicHome()
}

// overrideRefusedForHome reports whether the only thing withholding the
// override is the ~/Music rule (~/Music or a folder containing it), so the
// screen can say so.
func (s *pruneState) overrideRefusedForHome() bool {
	r := s.result
	return r != nil && len(r.Forceable()) > 0 && len(r.Blocked()) == 0 && s.coversMusicHome()
}

func (s *pruneState) showOverride() bool {
	return s.phase == phaseScanned && s.overrideAllowed()
}

// force is the override as prune.Apply should see it: ticked, and allowed.
func (s *pruneState) force() bool {
	return s.override && s.overrideAllowed()
}

func (s *pruneState) canTrash() bool {
	return s.phase == phaseScanned && s.result != nil && len(s.result.Plan.Orphans) > 0 &&
		s.result.CanApply(s.force()) == nil
}

func (s *pruneState) askConfirm() {
	if s.canTrash() {
		s.phase = phaseConfirming
	}
}

func (s *pruneState) cancelConfirm() {
	if s.phase == phaseConfirming {
		s.phase = phaseScanned
	}
}

// startTrash moves from the confirmation dialog to trashing, returning what to
// apply and with which override.
func (s *pruneState) startTrash() (*prune.Result, bool, bool) {
	if s.phase != phaseConfirming {
		return nil, false, false
	}
	force := s.force()
	if s.result.CanApply(force) != nil {
		s.phase = phaseScanned
		return nil, false, false
	}
	s.phase, s.err = phaseTrashing, nil
	return s.result, force, true
}

// trashDone records the outcome. A refusal or report failure moved nothing, so
// the scan stays on screen with the error beside it.
func (s *pruneState) trashDone(ar *prune.ApplyResult, err error) {
	if err != nil {
		s.phase, s.err = phaseScanned, err
		return
	}
	s.phase, s.applied = phaseDone, ar
}

// pruneInputs is the text of the screen's fields.
type pruneInputs struct {
	xml, music, exts, maxPct string
	keepEmptyDirs            bool
}

func buildOptions(in pruneInputs, home string) (prune.Options, error) {
	xml := expandHome(in.xml, home)
	music := expandHome(in.music, home)
	if xml == "" {
		return prune.Options{}, errors.New("choose the rekordbox XML export")
	}
	if music == "" {
		return prune.Options{}, errors.New("choose the music folder")
	}
	pct, err := strconv.ParseFloat(strings.TrimSpace(in.maxPct), 64)
	if err != nil || pct <= 0 || pct > 100 {
		return prune.Options{}, fmt.Errorf("max orphan %% must be a number above 0 and at most 100, not %q", in.maxPct)
	}
	exts := prune.ParseExtensions(in.exts)
	if len(exts) == 0 {
		return prune.Options{}, errors.New("list at least one audio extension")
	}
	return prune.Options{
		XMLPath:       xml,
		MusicDir:      music,
		Extensions:    exts,
		MaxOrphanPct:  pct,
		KeepEmptyDirs: in.keepEmptyDirs,
	}, nil
}

func relPath(root, path string) string {
	return strings.TrimPrefix(path, root+string(filepath.Separator))
}

// filterOrphans returns the indexes of orphans whose path below root contains
// query, ignoring case. It narrows the display only; what gets trashed is
// always the whole plan.
func filterOrphans(orphans []string, root, query string) []int {
	q := strings.ToLower(strings.TrimSpace(query))
	idx := make([]int, 0, len(orphans))
	for i, p := range orphans {
		if q == "" || strings.Contains(strings.ToLower(relPath(root, p)), q) {
			idx = append(idx, i)
		}
	}
	return idx
}

type detailRow struct {
	heading bool
	text    string
}

// detailRows flattens the plan's inventory lists into one scrollable list.
func detailRows(p *prune.Plan) []detailRow {
	var rows []detailRow
	section := func(title string, items []string) {
		if len(items) == 0 {
			return
		}
		rows = append(rows, detailRow{heading: true, text: fmt.Sprintf("%s (%d)", title, len(items))})
		for _, item := range items {
			rows = append(rows, detailRow{text: item})
		}
	}
	section("Skipped: newer than the export", p.RecentlyAdded)
	section("Dead links: in the library, missing on disk", p.DeadLinks)
	section("Stale Windows paths in the library", p.Stale)
	var dupes []string
	for _, group := range p.CaseDupes {
		dupes = append(dupes, strings.Join(group, "  |  "))
	}
	section("One file, several library spellings", dupes)
	section("Non-audio files (never trashed)", p.Leftovers)
	section("Symlinks (never followed)", p.Symlinks)
	return rows
}

// folderLine summarises the n folders with the most orphans, ordered as the
// CLI's table is: most orphans first, ties by name.
func folderLine(p *prune.Plan, n int) string {
	names := make([]string, 0, len(p.FolderOrphans))
	for name := range p.FolderOrphans {
		names = append(names, name)
	}
	if len(names) == 0 {
		return ""
	}
	sort.Slice(names, func(i, j int) bool {
		a, b := names[i], names[j]
		if p.FolderOrphans[a] != p.FolderOrphans[b] {
			return p.FolderOrphans[a] > p.FolderOrphans[b]
		}
		return a < b
	})
	shown := names[:min(n, len(names))]
	parts := make([]string, len(shown))
	for i, name := range shown {
		parts[i] = fmt.Sprintf("%s %d (%s)", name, p.FolderOrphans[name], prune.HumanBytes(p.FolderBytes[name]))
	}
	line := "By folder:  " + strings.Join(parts, "  ·  ")
	if rest := len(names) - len(shown); rest > 0 {
		line += fmt.Sprintf("  ·  +%d more", rest)
	}
	return line
}

func summaryLine(p *prune.Plan) string {
	if len(p.Orphans) == 0 {
		return fmt.Sprintf("Nothing to remove: all %d files this export accounts for are in the library.",
			p.EligibleAudio())
	}
	return fmt.Sprintf("%d orphans · %s · %.1f%% of the %d files this export accounts for",
		len(p.Orphans), prune.HumanBytes(p.OrphanSize), p.OrphanPct(), p.EligibleAudio())
}

type findingLine struct {
	text  string
	abort bool
	more  bool // the "+N more" line
}

// findingLines orders aborts before warnings and caps the list at max.
func findingLines(findings []prune.Finding, max int) []findingLine {
	var out []findingLine
	for _, level := range []prune.Level{prune.LevelAbort, prune.LevelWarn} {
		for _, f := range findings {
			if f.Level == level {
				out = append(out, findingLine{text: f.Message, abort: level == prune.LevelAbort})
			}
		}
	}
	if len(out) > max {
		rest := len(out) - max
		noun := "findings"
		if rest == 1 {
			noun = "finding"
		}
		out = append(out[:max:max], findingLine{
			text: fmt.Sprintf("(+%d more %s in the report)", rest, noun), more: true,
		})
	}
	return out
}

func findingMessages(findings []prune.Finding) string {
	msgs := make([]string, len(findings))
	for i, f := range findings {
		msgs[i] = f.Message
	}
	return strings.Join(msgs, "; ")
}

// safely runs f, turning a panic into an error so a bug in a worker goroutine
// shows up in the window instead of killing the app.
func safely[T any](f func() (T, error)) (v T, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("internal error: %v", r)
		}
	}()
	return f()
}
