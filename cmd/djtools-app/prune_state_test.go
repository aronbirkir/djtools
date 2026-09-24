package main

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aronbirkir/djtools/internal/prune"
)

const (
	home      = "/Users/me"
	rbMusic   = "/Users/me/Music/rekordbox"
	homeMusic = "/Users/me/Music"
)

var (
	forceable = prune.Finding{Code: prune.CodeOrphanShare, Level: prune.LevelAbort, Message: "orphan share too high"}
	blocked   = prune.Finding{Code: prune.CodeUnresolved, Level: prune.LevelAbort, Message: "library unresolved"}
	warning   = prune.Finding{Code: prune.CodeDeadLinks, Level: prune.LevelWarn, Message: "3 dead links"}
)

func resultWith(music string, orphans int, findings ...prune.Finding) *prune.Result {
	p := &prune.Plan{MusicDir: music}
	for i := range orphans {
		p.Orphans = append(p.Orphans, filepath.Join(music, fmt.Sprintf("t%d.mp3", i)))
	}
	return &prune.Result{Plan: p, Findings: findings}
}

func scanned(r *prune.Result) *pruneState {
	s := &pruneState{musicHome: homeMusic}
	s.startScan()
	s.scanDone(r, nil)
	return s
}

func TestCanTrashHealthy(t *testing.T) {
	s := scanned(resultWith(rbMusic, 3, warning))
	if !s.canTrash() {
		t.Error("a healthy scan with orphans should be trashable")
	}
	if s.showOverride() {
		t.Error("no override should be offered without forceable findings")
	}
}

func TestCanTrashNothingToRemove(t *testing.T) {
	if scanned(resultWith(rbMusic, 0)).canTrash() {
		t.Error("nothing to remove should not be trashable")
	}
}

func TestForceableNeedsOverride(t *testing.T) {
	s := scanned(resultWith(rbMusic, 3, forceable))
	if s.canTrash() {
		t.Error("trashable without the override")
	}
	if !s.showOverride() {
		t.Fatal("override checkbox should be offered")
	}
	s.override = true
	if !s.canTrash() {
		t.Error("override ticked but still not trashable")
	}
}

func TestBlockedNeverTrashable(t *testing.T) {
	s := scanned(resultWith(rbMusic, 3, forceable, blocked))
	if s.showOverride() {
		t.Error("override must not be offered alongside an unforceable finding")
	}
	s.override = true
	if s.canTrash() {
		t.Error("an unforceable finding must block trashing even with override set")
	}
}

func TestNoOverrideForMusicHome(t *testing.T) {
	for _, dir := range []string{homeMusic, homeMusic + "/", "/users/me/music"} {
		s := scanned(resultWith(dir, 3, forceable))
		if s.showOverride() {
			t.Errorf("%q: override offered for ~/Music itself", dir)
		}
		if !s.overrideRefusedForHome() {
			t.Errorf("%q: the ~/Music refusal should be explained", dir)
		}
		s.override = true
		if s.canTrash() {
			t.Errorf("%q: trashable through an override on ~/Music", dir)
		}
	}
}

func TestMusicHomeWithoutAbortsIsTrashable(t *testing.T) {
	// The rule only withholds overrides; a clean scan of ~/Music is the user's call.
	if !scanned(resultWith(homeMusic, 3)).canTrash() {
		t.Error("a clean scan of ~/Music should be trashable")
	}
}

func TestInputsChangedDiscardsResult(t *testing.T) {
	s := scanned(resultWith(rbMusic, 3, forceable))
	s.override = true
	s.inputsChanged()
	if s.phase != phaseIdle || s.result != nil || s.override {
		t.Errorf("after inputsChanged: phase %v, result %v, override %v", s.phase, s.result, s.override)
	}
	if s.canTrash() {
		t.Error("trashable after the inputs changed")
	}
}

func TestInputsChangedIgnoredWhileBusy(t *testing.T) {
	s := &pruneState{}
	if !s.startScan() {
		t.Fatal("startScan refused from idle")
	}
	s.inputsChanged()
	if s.phase != phaseScanning {
		t.Errorf("phase = %v, want scanning", s.phase)
	}
	if s.startScan() {
		t.Error("a second scan started while one was running")
	}
}

func TestScanFailure(t *testing.T) {
	s := &pruneState{}
	s.startScan()
	s.scanDone(nil, errors.New("boom"))
	if s.phase != phaseError || s.err == nil {
		t.Errorf("phase %v err %v, want error phase", s.phase, s.err)
	}
}

func TestConfirmFlow(t *testing.T) {
	s := scanned(resultWith(rbMusic, 3))
	s.askConfirm()
	if s.phase != phaseConfirming {
		t.Fatalf("phase = %v, want confirming", s.phase)
	}
	s.cancelConfirm()
	if s.phase != phaseScanned {
		t.Fatalf("phase = %v after cancel, want scanned", s.phase)
	}
	s.askConfirm()
	r, force, ok := s.startTrash()
	if !ok || r == nil || force || s.phase != phaseTrashing {
		t.Fatalf("startTrash = %v %v %v, phase %v", r, force, ok, s.phase)
	}
	s.trashDone(&prune.ApplyResult{Moved: 3, Total: 3}, nil)
	if s.phase != phaseDone || s.applied == nil {
		t.Errorf("phase = %v, applied %v", s.phase, s.applied)
	}
}

func TestConfirmRefusedWithoutOverride(t *testing.T) {
	s := scanned(resultWith(rbMusic, 3, forceable))
	s.askConfirm()
	if s.phase != phaseScanned {
		t.Errorf("phase = %v, want scanned: confirm must not open without the override", s.phase)
	}
	if _, _, ok := s.startTrash(); ok {
		t.Error("startTrash succeeded without confirming")
	}
}

func TestStartTrashPassesForce(t *testing.T) {
	s := scanned(resultWith(rbMusic, 3, forceable))
	s.override = true
	s.askConfirm()
	_, force, ok := s.startTrash()
	if !ok || !force {
		t.Errorf("startTrash = force %v ok %v, want both true", force, ok)
	}
}

func TestTrashFailureReturnsToScanned(t *testing.T) {
	s := scanned(resultWith(rbMusic, 3))
	s.askConfirm()
	s.startTrash()
	s.trashDone(nil, errors.New("report unwritable"))
	if s.phase != phaseScanned || s.err == nil || s.result == nil {
		t.Errorf("phase %v err %v result %v", s.phase, s.err, s.result)
	}
}

func TestBuildOptions(t *testing.T) {
	good := pruneInputs{xml: "~/x.xml", music: " ~/Music/rekordbox ", exts: ".mp3, wav",
		maxPct: "60", keepEmptyDirs: true}
	opts, err := buildOptions(good, home)
	if err != nil {
		t.Fatal(err)
	}
	if opts.XMLPath != "/Users/me/x.xml" || opts.MusicDir != rbMusic ||
		strings.Join(opts.Extensions, "|") != ".mp3|.wav" || opts.MaxOrphanPct != 60 || !opts.KeepEmptyDirs {
		t.Errorf("buildOptions = %+v", opts)
	}

	for name, mutate := range map[string]func(*pruneInputs){
		"no xml":       func(in *pruneInputs) { in.xml = " " },
		"no music":     func(in *pruneInputs) { in.music = "" },
		"pct not num":  func(in *pruneInputs) { in.maxPct = "abc" },
		"pct zero":     func(in *pruneInputs) { in.maxPct = "0" },
		"pct over 100": func(in *pruneInputs) { in.maxPct = "101" },
		"no exts":      func(in *pruneInputs) { in.exts = " , " },
	} {
		in := good
		mutate(&in)
		if _, err := buildOptions(in, home); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
}

func TestFilterOrphans(t *testing.T) {
	orphans := []string{rbMusic + "/House/A.mp3", rbMusic + "/Techno/b.mp3", rbMusic + "/house2/c.mp3"}
	if got := filterOrphans(orphans, rbMusic, ""); len(got) != 3 {
		t.Errorf("empty filter kept %d, want 3", len(got))
	}
	got := filterOrphans(orphans, rbMusic, " HOUSE ")
	if len(got) != 2 || got[0] != 0 || got[1] != 2 {
		t.Errorf("filter house = %v, want [0 2]", got)
	}
	// The root itself must not match: every path contains it.
	if got := filterOrphans(orphans, rbMusic, "rekordbox"); len(got) != 0 {
		t.Errorf("filter matched the music root: %v", got)
	}
}

func TestDetailRows(t *testing.T) {
	p := &prune.Plan{
		DeadLinks: []string{"/d1"},
		CaseDupes: [][]string{{"/a/X.mp3", "/a/x.mp3"}},
	}
	rows := detailRows(p)
	want := []detailRow{
		{heading: true, text: "Dead links: in the library, missing on disk (1)"},
		{text: "/d1"},
		{heading: true, text: "One file, several library spellings (1)"},
		{text: "/a/X.mp3  |  /a/x.mp3"},
	}
	if fmt.Sprint(rows) != fmt.Sprint(want) {
		t.Errorf("detailRows =\n%v\nwant\n%v", rows, want)
	}
}

func TestFolderLine(t *testing.T) {
	p := &prune.Plan{
		FolderOrphans: map[string]int{"House": 120, "Techno": 88, "Disco": 88},
		FolderBytes:   map[string]int64{"House": 1_600_000_000, "Techno": 900_000_000, "Disco": 1_000},
	}
	got := folderLine(p, 2)
	want := "By folder:  House 120 (1.6 GB)  ·  Disco 88 (1.0 kB)  ·  +1 more"
	if got != want {
		t.Errorf("folderLine =\n%q\nwant\n%q", got, want)
	}
	if got := folderLine(&prune.Plan{}, 5); got != "" {
		t.Errorf("empty plan folderLine = %q", got)
	}
}

func TestSummaryLine(t *testing.T) {
	p := &prune.Plan{Keepers: 95, Orphans: make([]string, 5), OrphanSize: 4_100_000_000}
	if got, want := summaryLine(p), "5 orphans · 4.1 GB · 5.0% of the 100 files this export accounts for"; got != want {
		t.Errorf("summaryLine = %q, want %q", got, want)
	}
	if got, want := summaryLine(&prune.Plan{Keepers: 10}), "Nothing to remove: all 10 files this export accounts for are in the library."; got != want {
		t.Errorf("summaryLine = %q, want %q", got, want)
	}
}

func TestFindingLines(t *testing.T) {
	findings := []prune.Finding{warning, forceable, blocked}
	lines := findingLines(findings, 10)
	if len(lines) != 3 || !lines[0].abort || !lines[1].abort || lines[2].abort {
		t.Errorf("aborts should come first: %+v", lines)
	}
	capped := findingLines(findings, 2)
	if len(capped) != 3 || !capped[2].more || capped[2].text != "(+1 more finding in the report)" {
		t.Errorf("capped = %+v", capped)
	}
	if got := findingMessages([]prune.Finding{forceable, blocked}); got != "orphan share too high; library unresolved" {
		t.Errorf("findingMessages = %q", got)
	}
}

func TestSafelyRecoversPanic(t *testing.T) {
	_, err := safely(func() (int, error) { panic("kaboom") })
	if err == nil || !strings.Contains(err.Error(), "kaboom") {
		t.Errorf("safely = %v, want an error mentioning the panic", err)
	}
	v, err := safely(func() (int, error) { return 7, nil })
	if v != 7 || err != nil {
		t.Errorf("safely = %v, %v", v, err)
	}
}

func TestTrashingIgnoresInputsFailAndScan(t *testing.T) {
	s := scanned(resultWith(rbMusic, 3))
	s.askConfirm()
	if _, _, ok := s.startTrash(); !ok {
		t.Fatal("startTrash refused a confirmed healthy scan")
	}
	s.inputsChanged()
	s.fail(errors.New("x"))
	if s.startScan() {
		t.Error("startScan accepted while trashing")
	}
	if s.phase != phaseTrashing || s.result == nil {
		t.Errorf("while trashing: phase %v, result %v", s.phase, s.result)
	}
}

func TestStartTrashRechecksOverride(t *testing.T) {
	s := scanned(resultWith(rbMusic, 3, forceable))
	s.override = true
	s.askConfirm()
	if s.phase != phaseConfirming {
		t.Fatalf("phase %v, want confirming", s.phase)
	}
	s.override = false
	if _, _, ok := s.startTrash(); ok {
		t.Error("startTrash went ahead after the override was cleared")
	}
	if s.phase != phaseScanned {
		t.Errorf("phase %v, want scanned", s.phase)
	}
}

func TestForceFalseOnMusicHome(t *testing.T) {
	s := scanned(resultWith(homeMusic, 3, forceable))
	s.override = true
	if s.force() {
		t.Error("force() true for ~/Music")
	}
}

func TestNoOverrideForAncestorsOfMusicHome(t *testing.T) {
	for _, dir := range []string{"/Users/me", "/Users", "/", "/users/ME/"} {
		s := scanned(resultWith(dir, 3, forceable))
		if s.showOverride() || !s.overrideRefusedForHome() {
			t.Errorf("%q: override should be refused for a folder containing ~/Music", dir)
		}
		s.override = true
		if s.force() || s.canTrash() {
			t.Errorf("%q: trashable through an override", dir)
		}
	}
	for _, dir := range []string{rbMusic, "/Users/me/Musical"} {
		s := scanned(resultWith(dir, 3, forceable))
		if !s.showOverride() || s.overrideRefusedForHome() {
			t.Errorf("%q: override should be offered", dir)
		}
	}
}

func TestNoticeKeepsScan(t *testing.T) {
	r := resultWith(rbMusic, 3)
	s := scanned(r)
	err := errors.New("no reports folder")
	s.notice(err)
	if s.phase != phaseScanned || s.result != r || s.err != err {
		t.Errorf("after notice: phase %v, result %v, err %v", s.phase, s.result, s.err)
	}
	if !s.canTrash() {
		t.Error("a notice should not stop a later attempt")
	}
}
