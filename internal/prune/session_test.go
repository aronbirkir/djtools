package prune

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func scanOptions(music, xml string) Options {
	return Options{
		XMLPath:      xml,
		MusicDir:     music,
		Extensions:   ParseExtensions(DefaultExtensions),
		MaxOrphanPct: DefaultMaxOrphanPct,
	}
}

func mustScan(t *testing.T, opts Options) *Result {
	t.Helper()
	r, err := Scan(opts)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	return r
}

// addUnresolvableEntry adds a library entry that points through a regular
// file. Statting it fails with ENOTDIR rather than ENOENT, which is what
// produces the unforceable unresolved-library finding.
func addUnresolvableEntry(t *testing.T, music, xml string) {
	t.Helper()
	through := filepath.Join(music, "House", "keep-0.mp3", "inner.mp3")
	body, err := os.ReadFile(xml)
	if err != nil {
		t.Fatal(err)
	}
	extra := "    <TRACK TrackID=\"unresolvable\" Location=\"" +
		locationFor(through) + "\"/>\n  </COLLECTION>"
	if err := os.WriteFile(xml,
		[]byte(strings.Replace(string(body), "  </COLLECTION>", extra, 1)), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestParseExtensions(t *testing.T) {
	got := strings.Join(ParseExtensions(" mp3, .wav,, FLAC "), "|")
	if want := ".mp3|.wav|.FLAC"; got != want {
		t.Errorf("ParseExtensions = %q, want %q", got, want)
	}
	if got := ParseExtensions(" , "); len(got) != 0 {
		t.Errorf("ParseExtensions of blanks = %q, want none", got)
	}
}

func TestScanHealthyCollection(t *testing.T) {
	music, xml := fixture(t, 600, 5)

	r := mustScan(t, scanOptions(music, xml))

	if got := len(r.Plan.Orphans); got != 5 {
		t.Errorf("orphans = %d, want 5", got)
	}
	if Aborts(r.Findings) {
		t.Errorf("healthy collection produced aborts: %v", r.Findings)
	}
	if err := r.CanApply(false); err != nil {
		t.Errorf("CanApply(false) = %v, want nil", err)
	}
	info, err := os.Stat(xml)
	if err != nil {
		t.Fatal(err)
	}
	if !r.XMLModTime.Equal(info.ModTime()) {
		t.Errorf("XMLModTime = %v, want %v", r.XMLModTime, info.ModTime())
	}
}

func TestScanMissingXMLIsAnError(t *testing.T) {
	music, _ := fixture(t, 600, 1)
	_, err := Scan(scanOptions(music, filepath.Join(music, "nope.xml")))
	if !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("Scan error = %v, want a not-exist error", err)
	}
}

func TestCanApplyForceableFindingNeedsForce(t *testing.T) {
	music, xml := fixture(t, 600, 5)
	opts := scanOptions(music, xml)
	opts.MaxOrphanPct = 0.1 // 5 of 605 is 0.8%, over the limit

	r := mustScan(t, opts)

	if len(r.Forceable()) == 0 {
		t.Fatalf("expected a forceable finding, got %v", r.Findings)
	}
	if len(r.Blocked()) != 0 {
		t.Fatalf("expected no blocked findings, got %v", r.Blocked())
	}
	if err := r.CanApply(false); !errors.Is(err, ErrNeedsForce) {
		t.Errorf("CanApply(false) = %v, want ErrNeedsForce", err)
	}
	if err := r.CanApply(true); err != nil {
		t.Errorf("CanApply(true) = %v, want nil", err)
	}
}

func TestCanApplyBlockedEvenWithForce(t *testing.T) {
	music, xml := fixture(t, 600, 1)
	addUnresolvableEntry(t, music, xml)

	r := mustScan(t, scanOptions(music, xml))

	if len(r.Blocked()) == 0 {
		t.Fatalf("expected a blocked finding, got %v", r.Findings)
	}
	for _, force := range []bool{false, true} {
		if err := r.CanApply(force); !errors.Is(err, ErrBlocked) {
			t.Errorf("CanApply(%v) = %v, want ErrBlocked", force, err)
		}
	}
}

func requireTrash(t *testing.T) {
	t.Helper()
	if err := TrashAvailable(); err != nil {
		t.Skip(err)
	}
}

// reportCheckingRunner records whether the report already existed when the
// first trash batch ran.
type reportCheckingRunner struct {
	report    string
	calls     int
	sawReport bool
}

func (r *reportCheckingRunner) Run(string, ...string) error {
	if r.calls == 0 {
		_, err := os.Stat(r.report)
		r.sawReport = err == nil
	}
	r.calls++
	return nil
}

func TestApplyRefusesBlockedEvenWithForce(t *testing.T) {
	requireTrash(t)
	music, xml := fixture(t, 600, 1)
	addUnresolvableEntry(t, music, xml)
	r := mustScan(t, scanOptions(music, xml))
	runner := &fakeRunner{}
	report := filepath.Join(t.TempDir(), "report.txt")

	if _, err := Apply(r, runner, true, report); !errors.Is(err, ErrBlocked) {
		t.Errorf("Apply = %v, want ErrBlocked", err)
	}
	if len(runner.calls) != 0 {
		t.Errorf("trash ran %d times despite the refusal", len(runner.calls))
	}
	if _, err := os.Stat(report); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("a refused apply wrote a report (stat err %v)", err)
	}
}

func TestApplyForceableNeedsForce(t *testing.T) {
	requireTrash(t)
	music, xml := fixture(t, 600, 5)
	opts := scanOptions(music, xml)
	opts.MaxOrphanPct = 0.1
	r := mustScan(t, opts)
	runner := &fakeRunner{}

	if _, err := Apply(r, runner, false, ""); !errors.Is(err, ErrNeedsForce) {
		t.Errorf("Apply(force=false) = %v, want ErrNeedsForce", err)
	}
	if len(runner.calls) != 0 {
		t.Fatalf("trash ran without an override")
	}

	res, err := Apply(r, runner, true, "")
	if err != nil {
		t.Fatalf("Apply(force=true) = %v", err)
	}
	if res.Moved != 5 || res.Total != 5 {
		t.Errorf("moved %d of %d, want 5 of 5", res.Moved, res.Total)
	}
	if len(runner.calls) != 1 {
		t.Errorf("trash calls = %d, want 1", len(runner.calls))
	}
}

func TestApplyWritesReportBeforeTrashing(t *testing.T) {
	requireTrash(t)
	music, xml := fixture(t, 600, 5)
	r := mustScan(t, scanOptions(music, xml))
	report := filepath.Join(t.TempDir(), "report.txt")
	runner := &reportCheckingRunner{report: report}

	res, err := Apply(r, runner, false, report)
	if err != nil {
		t.Fatalf("Apply = %v", err)
	}
	if runner.calls == 0 || !runner.sawReport {
		t.Errorf("the report must exist before the first trash batch (calls %d, saw %v)",
			runner.calls, runner.sawReport)
	}
	if res.ReportPath != report {
		t.Errorf("ReportPath = %q, want %q", res.ReportPath, report)
	}
	body, err := os.ReadFile(report)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), "# ORPHANS (5)") {
		t.Errorf("report missing orphan section:\n%s", body)
	}
}

func TestApplyUnwritableReportTrashesNothing(t *testing.T) {
	requireTrash(t)
	music, xml := fixture(t, 600, 5)
	r := mustScan(t, scanOptions(music, xml))
	// The parent is a regular file, so this path can never be created.
	report := filepath.Join(xml, "report.txt")
	runner := &fakeRunner{}

	_, err := Apply(r, runner, false, report)
	if err == nil {
		t.Fatal("Apply succeeded with an unwritable report path")
	}
	if !strings.Contains(err.Error(), "writing the report") {
		t.Errorf("Apply err = %v, want it to mention writing the report", err)
	}
	if len(runner.calls) != 0 {
		t.Errorf("trash ran %d times although the report could not be written", len(runner.calls))
	}
}

func TestApplyRefusesWhenXMLChangedSinceScan(t *testing.T) {
	requireTrash(t)
	music, xml := fixture(t, 600, 5)
	r := mustScan(t, scanOptions(music, xml))
	later := r.XMLModTime.Add(time.Minute)
	if err := os.Chtimes(xml, later, later); err != nil {
		t.Fatal(err)
	}
	runner := &fakeRunner{}

	if _, err := Apply(r, runner, false, ""); !errors.Is(err, ErrXMLChanged) {
		t.Errorf("Apply = %v, want ErrXMLChanged", err)
	}
	if len(runner.calls) != 0 {
		t.Errorf("trash ran against a stale plan")
	}
}

func TestApplyEmptyDirs(t *testing.T) {
	requireTrash(t)
	for _, keep := range []bool{false, true} {
		music, xml := fixture(t, 600, 5)
		// The fake runner moves nothing, so provide an empty directory to find.
		empty := filepath.Join(music, "Empty")
		if err := os.Mkdir(empty, 0o755); err != nil {
			t.Fatal(err)
		}
		opts := scanOptions(music, xml)
		opts.KeepEmptyDirs = keep
		r := mustScan(t, opts)

		res, err := Apply(r, &fakeRunner{}, false, "")
		if err != nil {
			t.Fatalf("keep=%v: Apply = %v", keep, err)
		}
		_, statErr := os.Stat(empty)
		if keep && (len(res.RemovedDirs) != 0 || statErr != nil) {
			t.Errorf("keep=true removed %v (stat err %v)", res.RemovedDirs, statErr)
		}
		if !keep && (len(res.RemovedDirs) != 1 || statErr == nil) {
			t.Errorf("keep=false removed %v, want the one empty directory", res.RemovedDirs)
		}
	}
}

func TestApplyIgnoresTamperedFindings(t *testing.T) {
	requireTrash(t)

	t.Run("blocked", func(t *testing.T) {
		music, xml := fixture(t, 600, 1)
		addUnresolvableEntry(t, music, xml)
		r := mustScan(t, scanOptions(music, xml))
		r.Findings = nil
		runner := &fakeRunner{}

		if _, err := Apply(r, runner, true, ""); !errors.Is(err, ErrBlocked) {
			t.Errorf("Apply = %v, want ErrBlocked", err)
		}
		if len(runner.calls) != 0 {
			t.Errorf("trash ran %d times despite the blocked finding", len(runner.calls))
		}
	})

	t.Run("forceable", func(t *testing.T) {
		music, xml := fixture(t, 600, 5)
		opts := scanOptions(music, xml)
		opts.MaxOrphanPct = 0.1
		r := mustScan(t, opts)
		r.Findings = nil
		runner := &fakeRunner{}

		if _, err := Apply(r, runner, false, ""); !errors.Is(err, ErrNeedsForce) {
			t.Errorf("Apply = %v, want ErrNeedsForce", err)
		}
		if len(runner.calls) != 0 {
			t.Errorf("trash ran %d times without an override", len(runner.calls))
		}
	})
}

func TestApplyRejectsIncompleteResult(t *testing.T) {
	requireTrash(t)
	runner := &fakeRunner{}

	if _, err := Apply(nil, runner, true, ""); err == nil {
		t.Error("Apply(nil, ...) = nil error, want one")
	}
	if _, err := Apply(&Result{}, runner, true, ""); err == nil {
		t.Error("Apply(&Result{}, ...) = nil error, want one")
	}
	if len(runner.calls) != 0 {
		t.Errorf("trash ran %d times against an incomplete result", len(runner.calls))
	}
}

func TestFindingMessages(t *testing.T) {
	got := FindingMessages([]Finding{{Message: "a"}, {Message: "b"}})
	if got != "a; b" {
		t.Errorf("FindingMessages = %q", got)
	}
}
