# djtools Desktop App Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a Gio desktop app (`cmd/djtools-app`) whose first tool, Rekordbox Prune, drives the existing prune logic through the same safety path as the `dj prune` CLI.

**Architecture:** Move `cmd/dj/internal/{prune,rekordbox}` to `internal/`. Add a UI-neutral `prune.Scan` / `prune.Apply` pair that owns every pre-flight check and safety gate, and put the CLI on top of it. The app is a Gio window with a tool sidebar. Every rule about whether files may be trashed lives in `prune_state.go`, which has no Gio imports and is unit-tested; the Gio view is thin and checked by hand.

**Tech Stack:** Go 1.27, Gio `gioui.org v0.10.2`, `github.com/ncruces/zenity v0.10.15` (native pickers), `gogio v0.10.0` (for the `.app` bundle). Both versions match `~/dev/experiment/go/mp3renamer`.

**Spec:** `docs/superpowers/specs/2026-09-24-djtools-app-design.md`

**Conventions to follow** (from the existing code): comments explain *why*, errors print as `dj prune: <err>`, tests use real temp dirs through the `fixture()` helper in `command_test.go`, and there's a `fakeRunner` in `apply_test.go`. Run commands from the repo root `/Users/aron/dev/experiment/go/djtools`.

**Note:** `cmd/dj/internal/prune/.claude/` is an untracked directory created by a hook. `git mv` leaves it behind. Leave it alone, since Go ignores dot-directories.

---

## File map

| File | Responsibility |
| --- | --- |
| `internal/rekordbox/*` | moved, unchanged |
| `internal/prune/session.go` | NEW: `Options`, `ParseExtensions`, `Result`, `Scan`, `Blocked`/`Forceable`/`CanApply`, `Apply`, sentinel errors |
| `internal/prune/session_test.go` | NEW: tests for the above |
| `internal/prune/report.go` | gains `WriteReportFile`; `humanBytes` becomes `HumanBytes` |
| `internal/prune/command.go` | CLI flags + printing on top of `Scan`/`Apply` |
| `cmd/dj/main.go` | import path only |
| `cmd/djtools-app/main.go` | window, event loop, sidebar, theme switch |
| `cmd/djtools-app/theme.go` | palette (copied from mp3renamer, plus `Warn`), theme mode, OS dark-mode watcher |
| `cmd/djtools-app/widgets.go` | drawing helpers and styled widgets copied from mp3renamer, plus `segmented` |
| `cmd/djtools-app/config.go` + `config_test.go` | settings file, defaults, `~` expansion, report paths |
| `cmd/djtools-app/prune_state.go` + `prune_state_test.go` | Rekordbox Prune state machine and pure formatting helpers |
| `cmd/djtools-app/prune_view.go` | Rekordbox Prune screen widgets and layout |
| `cmd/djtools-app/icon.go`, `icon.png`, `genicon/main.go` | app icon |
| `cmd/djtools-app/dockicon_darwin.go`, `dockicon_other.go` | Dock icon when running unbundled |
| `Makefile` | run / build / test / icon / macos targets |
| `README.md`, `.gitignore` | docs, `dist/` |

---

### Task 1: Move the packages to `internal/`

**Files:**
- Move: `cmd/dj/internal/prune/` → `internal/prune/`
- Move: `cmd/dj/internal/rekordbox/` → `internal/rekordbox/`
- Modify: every `.go` file importing `djtools/cmd/dj/internal/...`

- [ ] **Step 1: Move with git**

```bash
mkdir -p internal
git mv cmd/dj/internal/prune internal/prune
git mv cmd/dj/internal/rekordbox internal/rekordbox
```

- [ ] **Step 2: Rewrite the import paths and the realdata run hint**

```bash
grep -rl 'djtools/cmd/dj/internal/' --include='*.go' . | xargs sed -i '' 's#djtools/cmd/dj/internal/#djtools/internal/#g'
sed -i '' 's#go test -tags realdata ./cmd/dj/internal/prune#go test -tags realdata ./internal/prune#' internal/prune/realdata_test.go
grep -rn 'cmd/dj/internal' --include='*.go' . || echo "no stale imports"
```

Expected: `no stale imports`

- [ ] **Step 3: Build and test**

Run: `go build ./... && go test ./...`
Expected: every package `ok`, including `internal/prune` and `internal/rekordbox`.

- [ ] **Step 4: Commit**

```bash
git add -A internal cmd/dj
git commit -m "Move prune and rekordbox packages to internal/

The desktop app needs them too, and Go only lets cmd/dj import cmd/dj/internal."
```

---

### Task 2: Export what the app needs from `prune`

This is a pure refactor with no behaviour change. The app formats sizes, knows the default extensions, and the session code writes report files.

**Files:**
- Modify: `internal/prune/report.go`, `internal/prune/command.go`, any file using `humanBytes` or `defaultExtensions`

- [ ] **Step 1: Rename `humanBytes` and `defaultExtensions` everywhere in the package**

```bash
perl -pi -e 's/\bhumanBytes\b/HumanBytes/g; s/\bdefaultExtensions\b/DefaultExtensions/g' internal/prune/*.go
grep -rn 'humanBytes\|defaultExtensions' internal/prune || echo "renamed"
```

Expected: `renamed`

- [ ] **Step 2: Fix the doc comments of the renamed identifiers**

In `internal/prune/report.go`, the comment above `func HumanBytes` should begin `// HumanBytes formats a byte count ...`. In `internal/prune/command.go`, the comment above the const should begin `// DefaultExtensions covers what rekordbox can hold. ...`. Check both after the perl run and fix them if needed.

- [ ] **Step 3: Move `writeReportFile` into `report.go` as `WriteReportFile`**

Delete this function from the bottom half of `internal/prune/command.go`:

```go
func writeReportFile(path string, p *Plan) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	if err := Report(f, p); err != nil {
		return err
	}
	return f.Close()
}
```

Change its one call site in `command.go` from `writeReportFile(opts.reportPath, plan)` to `WriteReportFile(opts.reportPath, plan)`.

Add `"os"` to the imports of `internal/prune/report.go` and append:

```go
// WriteReportFile writes Report to path, creating or truncating it. The file is
// closed and checked before returning, so a nil error means the report is on
// disk -- callers that trash afterwards depend on that.
func WriteReportFile(path string, p *Plan) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	if err := Report(f, p); err != nil {
		return err
	}
	return f.Close()
}
```

`command.go` still uses `os` (for `os.Stat`/`os.Open`), so leave its imports alone.

- [ ] **Step 4: Build and test**

Run: `go vet ./... && go test ./...`
Expected: all `ok`.

- [ ] **Step 5: Commit**

```bash
git add internal/prune
git commit -m "Export HumanBytes, DefaultExtensions and WriteReportFile

The desktop app formats sizes and fills in defaults, and the shared session
code writes reports."
```

---

### Task 3: `prune.Scan` and the apply gates

**Files:**
- Create: `internal/prune/session.go`
- Test: `internal/prune/session_test.go`

- [ ] **Step 1: Write the failing tests**

Create `internal/prune/session_test.go`:

```go
package prune

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
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
```

- [ ] **Step 2: Run the tests to confirm they fail**

Run: `go test ./internal/prune -run 'TestParseExtensions|TestScan|TestCanApply'`
Expected: build failure (`undefined: Options`, `undefined: Scan`, ...).

- [ ] **Step 3: Implement `session.go` (without `Apply` yet)**

Create `internal/prune/session.go`:

```go
package prune

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/aronbirkir/djtools/internal/rekordbox"
)

// Options configures one prune run. It is the UI-neutral form of the CLI
// flags, shared by the dj command and the desktop app so that both reach the
// Trash through the same checks.
type Options struct {
	XMLPath       string
	MusicDir      string
	Extensions    []string // dotted, e.g. ".mp3"; see ParseExtensions
	MaxOrphanPct  float64
	KeepEmptyDirs bool
}

// ParseExtensions turns a comma-separated list such as "mp3, .wav" into dotted
// extensions, dropping empty items.
func ParseExtensions(list string) []string {
	var exts []string
	for _, e := range strings.Split(list, ",") {
		if e = strings.TrimSpace(e); e != "" {
			if !strings.HasPrefix(e, ".") {
				e = "." + e
			}
			exts = append(exts, e)
		}
	}
	return exts
}

// Result is a completed scan: what would be trashed and whether it may be.
// Nothing in it has touched the filesystem.
type Result struct {
	Options    Options
	Plan       *Plan
	Findings   []Finding
	Collection *rekordbox.Collection
	// XMLModTime is the export's modification time when it was read. Apply
	// compares against it so a plan built from one export is never carried out
	// after rekordbox has written another.
	XMLModTime time.Time
}

var (
	// ErrBlocked means a finding no override may reach, such as an unresolved
	// library path.
	ErrBlocked = errors.New("these findings cannot be overridden")
	// ErrNeedsForce means abort-level findings the user may override, but has
	// not.
	ErrNeedsForce = errors.New("these findings stop the run unless overridden")
	// ErrXMLChanged means the export was rewritten after the scan.
	ErrXMLChanged = errors.New("the rekordbox XML changed after the scan; scan again")
)

// Scan reads the export, walks the music folder and evaluates every guard. It
// never modifies anything, and deliberately does not require the trash utility,
// so a dry run works on a machine that could not delete.
func Scan(opts Options) (*Result, error) {
	// Refuse before reading anything. The plan itself depends on file
	// identity, so without it there is nothing meaningful to report.
	if err := Supported(); err != nil {
		return nil, err
	}
	info, err := os.Stat(opts.XMLPath)
	if err != nil {
		return nil, err
	}
	f, err := os.Open(opts.XMLPath)
	if err != nil {
		return nil, err
	}
	collection, err := rekordbox.Parse(f)
	f.Close()
	if err != nil {
		return nil, err
	}

	// A zero export time would silently disable the added-after-export skip,
	// quietly removing the protection that keeps freshly downloaded music out of
	// the Trash. Refuse rather than proceed with it weakened.
	exportedAt := info.ModTime()
	if exportedAt.IsZero() {
		return nil, fmt.Errorf("%s has no modification time, so files added after "+
			"the export cannot be identified; refusing to run", opts.XMLPath)
	}

	plan, err := Build(collection, opts.MusicDir, opts.Extensions, exportedAt)
	if err != nil {
		return nil, err
	}
	findings := Check(plan, collection, GuardOptions{
		MaxOrphanPct: opts.MaxOrphanPct,
		MinLibrary:   DefaultMinLibrary,
	})
	return &Result{
		Options:    opts,
		Plan:       plan,
		Findings:   findings,
		Collection: collection,
		XMLModTime: exportedAt,
	}, nil
}

// Blocked returns the abort-level findings that no override may reach.
func (r *Result) Blocked() []Finding {
	var out []Finding
	for _, f := range r.Findings {
		if f.Level == LevelAbort && f.Unforceable() {
			out = append(out, f)
		}
	}
	return out
}

// Forceable returns the abort-level findings an explicit override may reach.
func (r *Result) Forceable() []Finding {
	var out []Finding
	for _, f := range r.Findings {
		if f.Level == LevelAbort && !f.Unforceable() {
			out = append(out, f)
		}
	}
	return out
}

// CanApply reports whether this result may be trashed, given whether the user
// has explicitly overridden the forceable findings.
func (r *Result) CanApply(force bool) error {
	if b := r.Blocked(); len(b) > 0 {
		return fmt.Errorf("%w: %s", ErrBlocked, findingMessages(b))
	}
	if f := r.Forceable(); len(f) > 0 && !force {
		return fmt.Errorf("%w: %s", ErrNeedsForce, findingMessages(f))
	}
	return nil
}

func findingMessages(findings []Finding) string {
	msgs := make([]string, len(findings))
	for i, f := range findings {
		msgs[i] = f.Message
	}
	return strings.Join(msgs, "; ")
}
```

- [ ] **Step 4: Run the tests**

Run: `go test ./internal/prune`
Expected: `ok`, with the new tests and every existing test passing.

- [ ] **Step 5: Commit**

```bash
git add internal/prune/session.go internal/prune/session_test.go
git commit -m "Add prune.Scan and the apply gates as a UI-neutral session API"
```

---

### Task 4: `prune.Apply`

**Files:**
- Modify: `internal/prune/session.go`
- Test: `internal/prune/session_test.go`

- [ ] **Step 1: Write the failing tests**

Add `"time"` to the imports of `session_test.go` and append:

```go
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

	if _, err := Apply(r, runner, false, report); err == nil {
		t.Error("Apply succeeded with an unwritable report path")
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
```

- [ ] **Step 2: Run the tests to confirm they fail**

Run: `go test ./internal/prune -run TestApply`
Expected: build failure (`undefined: Apply`).

- [ ] **Step 3: Implement `Apply`**

Append to `internal/prune/session.go`:

```go
// ApplyResult is what a completed Apply did. A partial trash failure is not an
// error: files already moved stay moved, and the failures are listed here.
type ApplyResult struct {
	Moved, Total int
	TrashErrs    []error
	RemovedDirs  []string
	DirErrs      []error
	ReportPath   string
}

// Apply moves the result's orphans to the Trash.
//
// It re-checks every gate itself rather than trusting its caller: a UI that
// forgot to disable a button, or a CLI path that skipped a check, must still
// be unable to trash what the guards refused. If reportPath is set, the report
// is written first and a failure to write it stops the run before anything
// moves, since the report is what makes a restore precise.
func Apply(r *Result, runner Runner, force bool, reportPath string) (*ApplyResult, error) {
	if err := TrashAvailable(); err != nil {
		return nil, err
	}
	if err := r.CanApply(force); err != nil {
		return nil, err
	}
	info, err := os.Stat(r.Options.XMLPath)
	if err != nil {
		return nil, fmt.Errorf("re-checking %s: %w", r.Options.XMLPath, err)
	}
	if !info.ModTime().Equal(r.XMLModTime) {
		return nil, ErrXMLChanged
	}
	if reportPath != "" {
		if err := WriteReportFile(reportPath, r.Plan); err != nil {
			return nil, fmt.Errorf("writing the report, so nothing was moved: %w", err)
		}
	}

	res := &ApplyResult{Total: len(r.Plan.Orphans), ReportPath: reportPath}
	res.Moved, res.TrashErrs = Trash(runner, r.Plan.MusicDir, r.Plan.Orphans)
	if !r.Options.KeepEmptyDirs {
		res.RemovedDirs, res.DirErrs = RemoveEmptyDirs(r.Plan.MusicDir)
	}
	return res, nil
}
```

- [ ] **Step 4: Run the tests**

Run: `go test ./internal/prune`
Expected: `ok`.

- [ ] **Step 5: Commit**

```bash
git add internal/prune/session.go internal/prune/session_test.go
git commit -m "Add prune.Apply, which re-checks every gate and writes the report first"
```

---

### Task 5: Put the CLI on `Scan`/`Apply`

**Files:**
- Modify: `internal/prune/command.go` (full replacement below)
- Test: `internal/prune/command_test.go` must pass **unchanged**

- [ ] **Step 1: Replace `internal/prune/command.go`**

```go
package prune

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"
)

// Exit codes. These are the tool's contract with any script wrapping it.
const (
	exitOK      = 0
	exitStopped = 1 // guard abort, declined confirmation, or a usage error
	exitPartial = 2 // files were trashed but some batches failed
)

// DefaultExtensions covers what rekordbox can hold. Only .mp3 exists in the
// reference collection, but the Sampler entries are .wav.
const DefaultExtensions = ".mp3,.wav,.aiff,.flac,.m4a"

type cliOptions struct {
	xmlPath       string
	musicDir      string
	extensions    string
	dryRun        bool
	assumeYes     bool
	force         bool
	maxOrphanPct  float64
	list          bool
	reportPath    string
	keepEmptyDirs bool
}

// Run executes the prune subcommand and returns a process exit code.
func Run(args []string, stdout, stderr io.Writer, stdin io.Reader) int {
	var opts cliOptions
	fs := flag.NewFlagSet("prune", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.StringVar(&opts.xmlPath, "xml", "rekordbox.xml", "rekordbox XML export")
	fs.StringVar(&opts.musicDir, "music", "music", "music folder to prune")
	fs.StringVar(&opts.extensions, "ext", DefaultExtensions, "comma-separated audio extensions")
	fs.BoolVar(&opts.dryRun, "dry-run", false, "scan and report, never trash")
	fs.BoolVar(&opts.assumeYes, "yes", false, "skip the confirmation prompt")
	fs.BoolVar(&opts.force, "force", false, "proceed despite abort-level guard findings")
	fs.Float64Var(&opts.maxOrphanPct, "max-orphan-pct", DefaultMaxOrphanPct,
		"abort when orphans exceed this share of on-disk audio")
	fs.BoolVar(&opts.list, "list", false, "print every orphan path")
	fs.StringVar(&opts.reportPath, "report", "", "write the full lists to this file")
	fs.BoolVar(&opts.keepEmptyDirs, "keep-empty-dirs", false, "do not remove emptied directories")

	if err := fs.Parse(args); err != nil {
		// -h/--help is a successful request for help, not a failure: the flag
		// package has already printed usage and returns ErrHelp.
		if errors.Is(err, flag.ErrHelp) {
			return exitOK
		}
		return exitStopped
	}

	// Scan checks this too; checking here first keeps the platform refusal
	// ahead of the trash-utility one on unsupported systems.
	if err := Supported(); err != nil {
		fmt.Fprintf(stderr, "dj prune: %v\n", err)
		return exitStopped
	}

	// Fail before doing any work, and well before prompting, if we cannot
	// actually move files to the Trash.
	if !opts.dryRun {
		if err := TrashAvailable(); err != nil {
			fmt.Fprintf(stderr, "dj prune: %v\n", err)
			return exitStopped
		}
	}

	result, err := Scan(Options{
		XMLPath:       opts.xmlPath,
		MusicDir:      opts.musicDir,
		Extensions:    ParseExtensions(opts.extensions),
		MaxOrphanPct:  opts.maxOrphanPct,
		KeepEmptyDirs: opts.keepEmptyDirs,
	})
	if err != nil {
		fmt.Fprintf(stderr, "dj prune: %v\n", err)
		return exitStopped
	}
	plan := result.Plan

	if err := Summary(stdout, plan, result.Findings); err != nil {
		fmt.Fprintf(stderr, "dj prune: %v\n", err)
		return exitStopped
	}
	if opts.list {
		for _, p := range plan.Orphans {
			fmt.Fprintln(stdout, p)
		}
	}
	if opts.reportPath != "" {
		if err := WriteReportFile(opts.reportPath, plan); err != nil {
			fmt.Fprintf(stderr, "dj prune: %v\n", err)
			return exitStopped
		}
		fmt.Fprintf(stdout, "\nfull lists written to %s\n", opts.reportPath)
	}

	// --force covers findings the user may legitimately know to be wrong, such
	// as an unusually high orphan share. It deliberately does not reach an
	// unresolved library path: that means we could not establish what is in the
	// library, and deleting on that basis is the exact failure this tool exists
	// to prevent.
	if blocked := result.Blocked(); len(blocked) > 0 {
		fmt.Fprintln(stdout, "\nStopping. These cannot be overridden with --force:")
		for _, f := range blocked {
			fmt.Fprintf(stdout, "  %s\n", f.Message)
		}
		return exitStopped
	}
	if len(result.Forceable()) > 0 && !opts.force {
		fmt.Fprintln(stdout, "\nStopping. Re-run with --force to override, or re-export from rekordbox.")
		return exitStopped
	}

	if len(plan.Orphans) == 0 {
		return exitOK
	}
	if opts.dryRun {
		fmt.Fprintf(stdout, "\nThis was a dry run. Nothing was moved.\n")
		return exitOK
	}
	if !opts.assumeYes && !confirm(stdin, stdout, len(plan.Orphans), plan.OrphanSize) {
		fmt.Fprintln(stdout, "Nothing was moved.")
		return exitStopped
	}

	// Apply re-checks the gates above rather than trusting this function, and
	// refuses if the export was rewritten while the prompt was open. The
	// report, if requested, was already written above.
	res, err := Apply(result, ExecRunner{}, opts.force, "")
	if err != nil {
		fmt.Fprintf(stderr, "dj prune: %v\n", err)
		return exitStopped
	}
	fmt.Fprintf(stdout, "\nMoved %d of %d files to the Trash.\n", res.Moved, res.Total)
	for _, err := range res.TrashErrs {
		fmt.Fprintf(stderr, "dj prune: %v\n", err)
	}
	if !opts.keepEmptyDirs {
		fmt.Fprintf(stdout, "Removed %d empty directories.\n", len(res.RemovedDirs))
		for _, err := range res.DirErrs {
			fmt.Fprintf(stderr, "dj prune: %v\n", err)
		}
	}

	if len(res.TrashErrs) > 0 {
		return exitPartial
	}
	return exitOK
}

// confirm asks once. Anything other than an explicit yes means no, so a stray
// newline or a closed stdin can never authorise a deletion.
func confirm(in io.Reader, out io.Writer, count int, size int64) bool {
	fmt.Fprintf(out, "\nMove %d files (%s) to the Trash? [y/N] ", count, HumanBytes(size))
	line, err := bufio.NewReader(in).ReadString('\n')
	if err != nil && line == "" {
		fmt.Fprintln(out)
		return false
	}
	switch strings.ToLower(strings.TrimSpace(line)) {
	case "y", "yes":
		return true
	}
	return false
}
```

- [ ] **Step 2: Confirm `command_test.go` is untouched and everything passes**

Run: `git diff --stat internal/prune/command_test.go && go vet ./... && go test ./...`
Expected: no diff for `command_test.go`; all packages `ok`.

- [ ] **Step 3: Smoke-test the CLI against the real collection (dry run only)**

Run: `cd ~/DJ && go run ~/dev/experiment/go/djtools/cmd/dj prune --dry-run | tail -5; cd -`
Expected: the same summary as before the refactor, ending `This was a dry run. Nothing was moved.` (or a guard stop). If `~/DJ` doesn't exist, skip this step.

- [ ] **Step 4: Commit**

```bash
git add internal/prune/command.go
git commit -m "Run dj prune through Scan and Apply

The CLI's flags, output and exit codes are unchanged; command_test.go passes
as-is. Apply now also refuses if the export is rewritten while the prompt is
open."
```

---

### Task 6: App scaffold — deps, theme, widgets, config, icon, window

**Files:**
- Create: `cmd/djtools-app/{main.go,theme.go,widgets.go,config.go,config_test.go,icon.go,dockicon_darwin.go,dockicon_other.go,genicon/main.go}`
- Generate: `cmd/djtools-app/icon.png`
- Modify: `go.mod`, `go.sum`

- [ ] **Step 1: Write the failing config tests**

Create `cmd/djtools-app/config_test.go`:

```go
package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

var testDefaults = Config{
	XMLPath:      "/d/rekordbox.xml",
	MusicDir:     "/d/music",
	Extensions:   ".mp3",
	MaxOrphanPct: 60,
	Theme:        ThemeSystem,
}

func TestDefaultXMLPath(t *testing.T) {
	for _, tt := range []struct{ goos, want string }{
		{"darwin", filepath.Join("/Users/me", "Library", "Pioneer", "rekordbox", "rekordbox.xml")},
		{"windows", filepath.Join(`C:\Users\me\AppData\Roaming`, "Pioneer", "rekordbox", "rekordbox.xml")},
		{"linux", ""},
	} {
		if got := defaultXMLPath(tt.goos, "/Users/me", `C:\Users\me\AppData\Roaming`); got != tt.want {
			t.Errorf("%s: got %q, want %q", tt.goos, got, tt.want)
		}
	}
}

func TestExpandHome(t *testing.T) {
	for in, want := range map[string]string{
		"~":                  "/Users/me",
		"~/Music/rekordbox":  "/Users/me/Music/rekordbox",
		"  /abs/path  ":      "/abs/path",
		"~other/x":           "~other/x",
		"":                   "",
	} {
		if got := expandHome(in, "/Users/me"); got != want {
			t.Errorf("expandHome(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestReportPath(t *testing.T) {
	at := time.Date(2026, 9, 24, 15, 30, 12, 0, time.Local)
	if got, want := reportPath("/r", at), filepath.Join("/r", "prune-20260924-153012.txt"); got != want {
		t.Errorf("reportPath = %q, want %q", got, want)
	}
}

func TestLoadConfigMissingFileUsesDefaults(t *testing.T) {
	got := loadConfigFrom(filepath.Join(t.TempDir(), "none.json"), testDefaults)
	if got != testDefaults {
		t.Errorf("got %+v, want defaults", got)
	}
}

func TestLoadConfigKeepsDefaultsForMissingFields(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(`{"xml": "/mine.xml"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	got := loadConfigFrom(path, testDefaults)
	want := testDefaults
	want.XMLPath = "/mine.xml"
	if got != want {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

func TestLoadConfigCorruptFileUsesDefaults(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(`{"xml": `), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := loadConfigFrom(path, testDefaults); got != testDefaults {
		t.Errorf("got %+v, want defaults", got)
	}
}

func TestSaveConfigRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", "config.json")
	cfg := Config{XMLPath: "/x.xml", MusicDir: "/m", Extensions: ".mp3,.wav",
		MaxOrphanPct: 45.5, KeepEmptyDirs: true, Theme: ThemeDark}
	if err := saveConfigTo(path, cfg); err != nil {
		t.Fatal(err)
	}
	if got := loadConfigFrom(path, testDefaults); got != cfg {
		t.Errorf("got %+v, want %+v", got, cfg)
	}
}
```

- [ ] **Step 2: Add the Gio and zenity dependencies**

```bash
go get gioui.org@v0.10.2 github.com/ncruces/zenity@v0.10.15
```

- [ ] **Step 3: Create `cmd/djtools-app/theme.go`**

This is copied from mp3renamer's `theme.go` with a `Warn` colour added and `applyTheme` turned into a function.

```go
package main

import (
	"image/color"
	"os/exec"
	"runtime"
	"strings"
	"sync/atomic"
	"time"

	"gioui.org/app"
	"gioui.org/unit"
	"gioui.org/widget/material"
)

type Palette struct {
	Bg       color.NRGBA
	Surface  color.NRGBA
	Surface2 color.NRGBA
	Border   color.NRGBA
	Fg       color.NRGBA
	Muted    color.NRGBA
	Accent   color.NRGBA
	Success  color.NRGBA
	Warn     color.NRGBA
	Error    color.NRGBA
}

var darkPalette = Palette{
	Bg:       rgb(0x121418),
	Surface:  rgb(0x1b1e25),
	Surface2: rgb(0x242832),
	Border:   rgb(0x2f3440),
	Fg:       rgb(0xe6e8eb),
	Muted:    rgb(0x8b93a1),
	Accent:   rgb(0x7c5cff),
	Success:  rgb(0x3ddc97),
	Warn:     rgb(0xf5b94a),
	Error:    rgb(0xff6b6b),
}

var lightPalette = Palette{
	Bg:       rgb(0xf4f5f8),
	Surface:  rgb(0xffffff),
	Surface2: rgb(0xf1f2f6),
	Border:   rgb(0xdfe2e8),
	Fg:       rgb(0x1a1d23),
	Muted:    rgb(0x6b7280),
	Accent:   rgb(0x6a4cf5),
	Success:  rgb(0x12a36b),
	Warn:     rgb(0xb7791f),
	Error:    rgb(0xd93b3b),
}

// pal is the palette used for the current frame.
var pal = &darkPalette

// ThemeMode is the user's theme preference, as stored in the config.
type ThemeMode string

const (
	ThemeSystem ThemeMode = "system"
	ThemeLight  ThemeMode = "light"
	ThemeDark   ThemeMode = "dark"
)

var themeModes = []ThemeMode{ThemeSystem, ThemeLight, ThemeDark}

func (m ThemeMode) Label() string {
	switch m {
	case ThemeLight:
		return "Light"
	case ThemeDark:
		return "Dark"
	default:
		return "System"
	}
}

func themeLabels() []string {
	out := make([]string, len(themeModes))
	for i, m := range themeModes {
		out[i] = m.Label()
	}
	return out
}

func themeIndex(m ThemeMode) int {
	for i, mode := range themeModes {
		if mode == m {
			return i
		}
	}
	return 0
}

func newTheme() *material.Theme {
	th := material.NewTheme()
	th.TextSize = unit.Sp(15)
	return th
}

// applyTheme picks the palette for this frame from the user's preference.
func applyTheme(th *material.Theme, mode ThemeMode, systemDark bool) {
	if mode == ThemeDark || (mode != ThemeLight && systemDark) {
		pal = &darkPalette
	} else {
		pal = &lightPalette
	}
	th.Palette = material.Palette{
		Bg:         pal.Bg,
		Fg:         pal.Fg,
		ContrastBg: pal.Accent,
		ContrastFg: rgb(0xffffff),
	}
}

// systemTheme tracks whether the OS is in dark mode.
type systemTheme struct {
	dark atomic.Bool
}

// watch polls the OS appearance and invalidates the window when it changes.
// Gio has no appearance-change event, so polling is the portable option.
func (s *systemTheme) watch(w *app.Window) {
	s.dark.Store(systemIsDark())
	go func() {
		for range time.Tick(3 * time.Second) {
			if d := systemIsDark(); d != s.dark.Load() {
				s.dark.Store(d)
				w.Invalidate()
			}
		}
	}()
}

func systemIsDark() bool {
	switch runtime.GOOS {
	case "darwin":
		// Prints "Dark" in dark mode; the key is absent in light mode.
		out, err := exec.Command("defaults", "read", "-g", "AppleInterfaceStyle").Output()
		return err == nil && strings.TrimSpace(string(out)) == "Dark"
	case "windows":
		out, err := exec.Command("reg", "query",
			`HKCU\Software\Microsoft\Windows\CurrentVersion\Themes\Personalize`,
			"/v", "AppsUseLightTheme").Output()
		return err == nil && strings.Contains(string(out), "0x0")
	case "linux":
		out, err := exec.Command("gsettings", "get", "org.gnome.desktop.interface", "color-scheme").Output()
		return err == nil && strings.Contains(string(out), "dark")
	}
	return true
}
```

- [ ] **Step 4: Create `cmd/djtools-app/widgets.go`**

This is mp3renamer's `utils.go` plus its `card`/`fieldLabel`/`input`/`button`, with the theme switch generalised into `segmented`.

```go
package main

import (
	"image"
	"image/color"
	"strings"

	"gioui.org/font"
	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
)

type (
	C = layout.Context
	D = layout.Dimensions
)

func rgb(c uint32) color.NRGBA {
	return color.NRGBA{R: uint8(c >> 16), G: uint8(c >> 8), B: uint8(c), A: 0xff}
}

func withAlpha(c color.NRGBA, a uint8) color.NRGBA {
	c.A = a
	return c
}

// rounded returns a widget that fills its minimum constraints with a rounded rectangle.
// Use it as the background of a layout.Background.
func rounded(c color.NRGBA, radius unit.Dp) layout.Widget {
	return func(gtx C) D {
		size := gtx.Constraints.Min
		defer clip.UniformRRect(image.Rectangle{Max: size}, gtx.Dp(radius)).Push(gtx.Ops).Pop()
		paint.Fill(gtx.Ops, c)
		return D{Size: size}
	}
}

// fill paints the whole available area.
func fill(gtx C, c color.NRGBA) D {
	size := gtx.Constraints.Max
	defer clip.Rect{Max: size}.Push(gtx.Ops).Pop()
	paint.Fill(gtx.Ops, c)
	return D{Size: size}
}

// divider draws a 1dp horizontal line across the available width.
func divider(gtx C) D {
	size := image.Pt(gtx.Constraints.Max.X, gtx.Dp(1))
	defer clip.Rect{Max: size}.Push(gtx.Ops).Pop()
	paint.Fill(gtx.Ops, pal.Border)
	return D{Size: size}
}

func vspace(dp unit.Dp) layout.FlexChild {
	return layout.Rigid(layout.Spacer{Height: dp}.Layout)
}

func hspace(dp unit.Dp) layout.FlexChild {
	return layout.Rigid(layout.Spacer{Width: dp}.Layout)
}

func card(gtx C, w layout.Widget) D {
	return widget.Border{Color: pal.Border, CornerRadius: unit.Dp(12), Width: unit.Dp(1)}.Layout(gtx, func(gtx C) D {
		return layout.Background{}.Layout(gtx, rounded(pal.Surface, 12), func(gtx C) D {
			return layout.UniformInset(unit.Dp(18)).Layout(gtx, w)
		})
	})
}

func fieldLabel(th *material.Theme, s string) layout.Widget {
	l := material.Caption(th, strings.ToUpper(s))
	l.Color = pal.Muted
	l.Font.Weight = font.SemiBold
	return l.Layout
}

// label is a Body2 text in the given colour.
func label(th *material.Theme, s string, c color.NRGBA) layout.Widget {
	l := material.Body2(th, s)
	l.Color = c
	return l.Layout
}

func input(gtx C, th *material.Theme, ed *widget.Editor, hint string) D {
	border := pal.Border
	if gtx.Source.Focused(ed) {
		border = pal.Accent
	}
	return widget.Border{Color: border, CornerRadius: unit.Dp(8), Width: unit.Dp(1)}.Layout(gtx, func(gtx C) D {
		return layout.Background{}.Layout(gtx, rounded(pal.Surface2, 8), func(gtx C) D {
			return layout.Inset{Top: 10, Bottom: 10, Left: 12, Right: 12}.Layout(gtx, func(gtx C) D {
				gtx.Constraints.Min.X = gtx.Constraints.Max.X
				e := material.Editor(th, ed, hint)
				e.HintColor = pal.Muted
				e.SelectionColor = withAlpha(pal.Accent, 0x60)
				return e.Layout(gtx)
			})
		})
	})
}

// styleButton applies the app's button look.
func styleButton(b *material.ButtonStyle) {
	b.CornerRadius = unit.Dp(8)
	// The label's line box reserves room for descenders, so button text looks
	// high when centered. Shift the padding down to center it visually.
	b.Inset = layout.Inset{Top: 13, Bottom: 7, Left: 18, Right: 18}
	b.Font.Weight = font.SemiBold
	b.TextSize = unit.Sp(14)
}

func button(th *material.Theme, c *widget.Clickable, text string, primary bool) layout.Widget {
	b := material.Button(th, c, text)
	styleButton(&b)
	if !primary {
		b.Background = pal.Surface2
		b.Color = pal.Fg
	}
	return b.Layout
}

// segmented draws a row of mutually exclusive options: the theme switch and
// the orphan/details tabs.
func segmented(gtx C, th *material.Theme, clicks []widget.Clickable, labels []string, selected int) D {
	segments := make([]layout.FlexChild, len(labels))
	for i, text := range labels {
		segments[i] = layout.Rigid(func(gtx C) D {
			on := i == selected
			return material.Clickable(gtx, &clicks[i], func(gtx C) D {
				bg := pal.Surface
				if on {
					bg = pal.Accent
				}
				return layout.Background{}.Layout(gtx, rounded(bg, 6), func(gtx C) D {
					return layout.Inset{Top: 9, Bottom: 3, Left: 12, Right: 12}.Layout(gtx, func(gtx C) D {
						l := material.Body2(th, text)
						l.TextSize = unit.Sp(13)
						l.Font.Weight = font.SemiBold
						l.Color = pal.Muted
						if on {
							l.Color = rgb(0xffffff)
						}
						return l.Layout(gtx)
					})
				})
			})
		})
	}
	return widget.Border{Color: pal.Border, CornerRadius: unit.Dp(9), Width: unit.Dp(1)}.Layout(gtx, func(gtx C) D {
		return layout.Background{}.Layout(gtx, rounded(pal.Surface, 9), func(gtx C) D {
			return layout.UniformInset(unit.Dp(3)).Layout(gtx, func(gtx C) D {
				return layout.Flex{}.Layout(gtx, segments...)
			})
		})
	})
}
```

- [ ] **Step 5: Create `cmd/djtools-app/config.go`**

```go
package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/aronbirkir/djtools/internal/prune"
)

// Config holds settings persisted between runs.
type Config struct {
	XMLPath       string    `json:"xml"`
	MusicDir      string    `json:"music"`
	Extensions    string    `json:"extensions"`
	MaxOrphanPct  float64   `json:"maxOrphanPct"`
	KeepEmptyDirs bool      `json:"keepEmptyDirs"`
	Theme         ThemeMode `json:"theme"`
}

// appDir is where the app keeps its settings and prune reports.
func appDir() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "djtools"), nil
}

// reportsDir holds one report per real run. It is ours, unlike the folder
// holding rekordbox.xml, which belongs to rekordbox.
func reportsDir() (string, error) {
	dir, err := appDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "reports"), nil
}

func reportPath(dir string, now time.Time) string {
	return filepath.Join(dir, "prune-"+now.Format("20060102-150405")+".txt")
}

func defaultConfig() Config {
	home, _ := os.UserHomeDir()
	configDir, _ := os.UserConfigDir()
	return Config{
		XMLPath:      defaultXMLPath(runtime.GOOS, home, configDir),
		MusicDir:     filepath.Join(home, "Music", "rekordbox"),
		Extensions:   prune.DefaultExtensions,
		MaxOrphanPct: prune.DefaultMaxOrphanPct,
		Theme:        ThemeSystem,
	}
}

// defaultXMLPath is where rekordbox writes its XML export unless told
// otherwise. configDir is %AppData% on Windows.
func defaultXMLPath(goos, home, configDir string) string {
	switch goos {
	case "darwin":
		return filepath.Join(home, "Library", "Pioneer", "rekordbox", "rekordbox.xml")
	case "windows":
		return filepath.Join(configDir, "Pioneer", "rekordbox", "rekordbox.xml")
	}
	return ""
}

// expandHome resolves a leading ~ so typed paths behave as in a shell.
func expandHome(path, home string) string {
	path = strings.TrimSpace(path)
	if path == "~" {
		return home
	}
	if strings.HasPrefix(path, "~/") {
		return filepath.Join(home, path[2:])
	}
	return path
}

func configPath() (string, error) {
	dir, err := appDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "config.json"), nil
}

func loadConfig() Config {
	defaults := defaultConfig()
	path, err := configPath()
	if err != nil {
		return defaults
	}
	return loadConfigFrom(path, defaults)
}

// loadConfigFrom overlays the saved settings on defaults, so a setting missing
// from an older file keeps its default rather than becoming zero. A corrupt
// file yields the defaults rather than a half-applied mix.
func loadConfigFrom(path string, defaults Config) Config {
	data, err := os.ReadFile(path)
	if err != nil {
		return defaults
	}
	cfg := defaults
	if err := json.Unmarshal(data, &cfg); err != nil {
		return defaults
	}
	return cfg
}

func saveConfig(cfg Config) error {
	path, err := configPath()
	if err != nil {
		return err
	}
	return saveConfigTo(path, cfg)
}

func saveConfigTo(path string, cfg Config) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}
```

- [ ] **Step 6: Create the icon files**

`cmd/djtools-app/icon.go`:

```go
package main

import _ "embed"

//go:generate go run ./genicon icon.png

//go:embed icon.png
var iconPNG []byte
```

`cmd/djtools-app/dockicon_darwin.go`, copied verbatim from mp3renamer except for the comment:

```go
package main

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Cocoa
#import <Cocoa/Cocoa.h>

static void setDockIcon(const void *data, int length) {
	NSData *d = [NSData dataWithBytes:data length:length];
	dispatch_async(dispatch_get_main_queue(), ^{
		NSImage *img = [[NSImage alloc] initWithData:d];
		if (img != nil) {
			[[NSApplication sharedApplication] setApplicationIconImage:img];
		}
	});
}
*/
import "C"

import "unsafe"

// setAppIcon replaces the generic executable icon in the Dock. It's needed
// when running the bare binary (e.g. `make run`) rather than an .app bundle.
func setAppIcon() {
	C.setDockIcon(unsafe.Pointer(&iconPNG[0]), C.int(len(iconPNG)))
}
```

`cmd/djtools-app/dockicon_other.go`:

```go
//go:build !darwin

package main

func setAppIcon() {}
```

`cmd/djtools-app/genicon/main.go`:

```go
// Command genicon renders the djtools app icon to icon.png: a vinyl record on
// the same violet macOS-style rounded square as MP3 Renamer's icon.
package main

import (
	"image"
	"image/color"
	"image/png"
	"log"
	"math"
	"os"
)

const size = 1024

var (
	white  = [3]float64{255, 255, 255}
	disc   = [3]float64{0x1b, 0x1e, 0x25}
	groove = [3]float64{0x2c, 0x30, 0x3b}
)

func main() {
	out := "icon.png"
	if len(os.Args) > 1 {
		out = os.Args[1]
	}
	img := image.NewNRGBA(image.Rect(0, 0, size, size))
	const ss = 4 // supersampling per axis
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			var r, g, b, a float64
			for sy := 0; sy < ss; sy++ {
				for sx := 0; sx < ss; sx++ {
					px := float64(x) + (float64(sx)+0.5)/ss
					py := float64(y) + (float64(sy)+0.5)/ss
					if c, ok := sample(px, py); ok {
						r, g, b, a = r+c[0], g+c[1], b+c[2], a+1
					}
				}
			}
			if a == 0 {
				continue
			}
			img.SetNRGBA(x, y, color.NRGBA{
				R: uint8(r / a), G: uint8(g / a), B: uint8(b / a),
				A: uint8(a / (ss * ss) * 255),
			})
		}
	}
	f, err := os.Create(out)
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		log.Fatal(err)
	}
}

// sample returns the colour at p, or false if p is outside the icon.
func sample(x, y float64) ([3]float64, bool) {
	if !inRoundRect(x, y, 100, 100, 924, 924, 185) {
		return [3]float64{}, false
	}
	// Diagonal violet gradient, as in MP3 Renamer.
	t := (x + y) / (2 * size)
	bg := [3]float64{lerp(0x9a, 0x55, t), lerp(0x7c, 0x33, t), lerp(0xff, 0xe0, t)}

	r := math.Hypot(x-512, y-512)
	switch {
	case r <= 22: // spindle hole
		return bg, true
	case r <= 118: // label
		return white, true
	case r <= 136:
		return disc, true
	case r <= 330:
		if int((r-136)/18)%2 == 1 {
			return groove, true
		}
		return disc, true
	}
	return bg, true
}

func lerp(a, b, t float64) float64 { return a + (b-a)*t }

func inRoundRect(x, y, x0, y0, x1, y1, rad float64) bool {
	if x < x0 || x > x1 || y < y0 || y > y1 {
		return false
	}
	cx := math.Max(math.Max(x0+rad-x, x-(x1-rad)), 0)
	cy := math.Max(math.Max(y0+rad-y, y-(y1-rad)), 0)
	return cx*cx+cy*cy <= rad*rad
}
```

Then generate it:

Run: `go generate ./cmd/djtools-app && ls -la cmd/djtools-app/icon.png`
Expected: `icon.png` exists (a few tens of kB). Open it with `open cmd/djtools-app/icon.png` and check it's a charcoal record with a white label on violet.

- [ ] **Step 7: Create the scaffold `cmd/djtools-app/main.go`**

This version draws the sidebar and an empty content area. Task 8 replaces it.

```go
// Command djtools-app is the desktop front end for djtools.
package main

import (
	"image/color"
	"log"
	"os"

	"gioui.org/app"
	"gioui.org/font"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
)

func main() {
	go func() {
		window := new(app.Window)
		window.Option(
			app.Title("djtools"),
			app.Size(unit.Dp(1100), unit.Dp(760)),
			app.MinSize(unit.Dp(820), unit.Dp(520)),
		)
		if err := run(window); err != nil {
			log.Fatal(err)
		}
		os.Exit(0)
	}()
	setAppIcon()
	app.Main()
}

type appUI struct {
	window       *app.Window
	cfg          Config
	themeMode    ThemeMode
	themeButtons [3]widget.Clickable
	sysTheme     systemTheme
}

func run(window *app.Window) error {
	th := newTheme()
	var ops op.Ops
	ui := &appUI{window: window, cfg: loadConfig()}
	ui.themeMode = ui.cfg.Theme
	if ui.themeMode == "" {
		ui.themeMode = ThemeSystem
	}
	ui.sysTheme.watch(window)

	for {
		switch e := window.Event().(type) {
		case app.DestroyEvent:
			return e.Err
		case app.FrameEvent:
			gtx := app.NewContext(&ops, e)
			ui.update(gtx)
			applyTheme(th, ui.themeMode, ui.sysTheme.dark.Load())
			ui.Layout(gtx, th)
			e.Frame(gtx.Ops)
		}
	}
}

func (ui *appUI) saveConfig() {
	ui.cfg.Theme = ui.themeMode
	if err := saveConfig(ui.cfg); err != nil {
		log.Printf("saving config: %v", err)
	}
}

func (ui *appUI) update(gtx C) {
	for i := range ui.themeButtons {
		if ui.themeButtons[i].Clicked(gtx) && ui.themeMode != themeModes[i] {
			ui.themeMode = themeModes[i]
			ui.saveConfig()
		}
	}
}

func (ui *appUI) Layout(gtx C, th *material.Theme) D {
	fill(gtx, pal.Bg)
	gtx.Constraints.Min = gtx.Constraints.Max
	return layout.Flex{}.Layout(gtx,
		layout.Rigid(func(gtx C) D { return ui.layoutSidebar(gtx, th) }),
		layout.Flexed(1, func(gtx C) D { return D{Size: gtx.Constraints.Max} }),
	)
}

func (ui *appUI) layoutSidebar(gtx C, th *material.Theme) D {
	width := gtx.Dp(unit.Dp(232))
	gtx.Constraints.Min.X, gtx.Constraints.Max.X = width, width
	gtx.Constraints.Min.Y = gtx.Constraints.Max.Y
	return layout.Background{}.Layout(gtx, rounded(pal.Surface, 0), func(gtx C) D {
		return layout.UniformInset(unit.Dp(14)).Layout(gtx, func(gtx C) D {
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
				layout.Rigid(func(gtx C) D {
					l := material.H6(th, "djtools")
					l.Font.Weight = font.Bold
					return layout.Inset{Top: 6, Left: 10, Bottom: 18}.Layout(gtx, l.Layout)
				}),
				layout.Rigid(func(gtx C) D { return navItem(gtx, th, "Rekordbox Prune", true) }),
				layout.Flexed(1, layout.Spacer{}.Layout),
				layout.Rigid(func(gtx C) D {
					return segmented(gtx, th, ui.themeButtons[:], themeLabels(), themeIndex(ui.themeMode))
				}),
			)
		})
	})
}

// navItem is one tool in the sidebar. There is only one tool so far, so it is
// always selected and not yet clickable.
func navItem(gtx C, th *material.Theme, text string, selected bool) D {
	gtx.Constraints.Min.X = gtx.Constraints.Max.X
	bg := color.NRGBA{}
	if selected {
		bg = withAlpha(pal.Accent, 0x26)
	}
	return layout.Background{}.Layout(gtx, rounded(bg, 8), func(gtx C) D {
		return layout.Inset{Top: 10, Bottom: 8, Left: 10, Right: 10}.Layout(gtx, func(gtx C) D {
			l := material.Body1(th, text)
			l.Font.Weight = font.SemiBold
			if selected {
				l.Color = pal.Accent
			}
			return l.Layout(gtx)
		})
	})
}
```

- [ ] **Step 8: Tidy, test, and look at it**

```bash
go mod tidy
go vet ./... && go test ./...
go run ./cmd/djtools-app
```

Expected: tests `ok` (including `cmd/djtools-app`). A window titled "djtools" opens with a sidebar showing "djtools", a highlighted "Rekordbox Prune" entry, and a System/Light/Dark switch at the bottom that changes the theme. The rest of the window is empty. Close the window.

- [ ] **Step 9: Commit**

```bash
git add go.mod go.sum cmd/djtools-app
git commit -m "Scaffold the djtools Gio app: theme, widgets, config, icon, sidebar

Theme, widgets and the dock icon are copied from MP3 Renamer so both apps look
alike."
```

---

### Task 7: Rekordbox Prune state machine

**Files:**
- Create: `cmd/djtools-app/prune_state.go`
- Test: `cmd/djtools-app/prune_state_test.go`

- [ ] **Step 1: Write the failing tests**

Create `cmd/djtools-app/prune_state_test.go`:

```go
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
	home     = "/Users/me"
	rbMusic  = "/Users/me/Music/rekordbox"
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
```

- [ ] **Step 2: Run the tests to confirm they fail**

Run: `go test ./cmd/djtools-app`
Expected: build failure (`undefined: pruneState`, ...).

- [ ] **Step 3: Implement `cmd/djtools-app/prune_state.go`**

```go
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

func (s *pruneState) isMusicHome() bool {
	if s.result == nil || s.result.Plan == nil || s.musicHome == "" {
		return false
	}
	return strings.EqualFold(filepath.Clean(s.result.Plan.MusicDir), filepath.Clean(s.musicHome))
}

// overrideAllowed reports whether the forceable findings may be overridden at
// all: there must be some, nothing unforceable, and Music must not be ~/Music.
func (s *pruneState) overrideAllowed() bool {
	r := s.result
	return r != nil && len(r.Forceable()) > 0 && len(r.Blocked()) == 0 && !s.isMusicHome()
}

// overrideRefusedForHome reports whether the only thing withholding the
// override is the ~/Music rule, so the screen can say so.
func (s *pruneState) overrideRefusedForHome() bool {
	r := s.result
	return r != nil && len(r.Forceable()) > 0 && len(r.Blocked()) == 0 && s.isMusicHome()
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
```

- [ ] **Step 4: Run the tests**

Run: `gofmt -l cmd/djtools-app; go vet ./cmd/djtools-app && go test ./cmd/djtools-app`
Expected: `gofmt -l` prints nothing (run `gofmt -w cmd/djtools-app` if it lists files), and tests pass with `ok`.

- [ ] **Step 5: Commit**

```bash
git add cmd/djtools-app/prune_state.go cmd/djtools-app/prune_state_test.go
git commit -m "Add the Rekordbox Prune state machine, with the override rules tested"
```

---

### Task 8: Rekordbox Prune screen and wiring

**Files:**
- Create: `cmd/djtools-app/prune_view.go`
- Modify: `cmd/djtools-app/main.go` (full replacement below)

- [ ] **Step 1: Create `cmd/djtools-app/prune_view.go`**

```go
package main

import (
	"errors"
	"fmt"
	"image/color"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"gioui.org/app"
	"gioui.org/font"
	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/text"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
	"github.com/ncruces/zenity"

	"github.com/aronbirkir/djtools/internal/prune"
)

const (
	tabOrphans = iota
	tabDetails
)

const restoreHint = "Finder's Put Back does not work for files moved this way. To restore them, " +
	"run tools/restore-from-trash.py from the djtools repo (see \"Recovering a mistake\" in its README)."

// pruneView is the Rekordbox Prune screen: its widgets, plus the pruneState
// that decides what they may do.
type pruneView struct {
	window *app.Window
	home   string
	save   func()
	state  pruneState

	xml, music, exts, maxPct, filter widget.Editor
	keepDirs, override               widget.Bool

	browseXML, browseMusic, scanBtn, advancedBtn widget.Clickable
	trashBtn, confirmBtn, cancelBtn, finderBtn   widget.Clickable
	// scrim covers the window behind the confirmation dialog and cancels it
	// when clicked; sink covers the dialog itself so clicks on its body do not
	// fall through to the scrim.
	scrim, sink widget.Clickable
	tabs        [2]widget.Clickable

	showAdvanced bool
	tab          int
	filtered     []int
	details      []detailRow
	list         widget.List
	reportPath   string

	// done carries results from worker goroutines, which must not touch UI
	// state themselves; each func runs on the UI goroutine at the next frame.
	done     chan func()
	browsing bool
}

func newPruneView(w *app.Window, cfg Config, home string, save func()) *pruneView {
	v := &pruneView{
		window: w,
		home:   home,
		save:   save,
		state:  pruneState{musicHome: filepath.Join(home, "Music")},
		xml:    widget.Editor{SingleLine: true, Submit: true},
		music:  widget.Editor{SingleLine: true, Submit: true},
		exts:   widget.Editor{SingleLine: true, Submit: true},
		maxPct: widget.Editor{SingleLine: true, Submit: true},
		filter: widget.Editor{SingleLine: true},
		list:   widget.List{List: layout.List{Axis: layout.Vertical}},
		done:   make(chan func(), 4),
	}
	v.xml.SetText(cfg.XMLPath)
	v.music.SetText(cfg.MusicDir)
	v.exts.SetText(cfg.Extensions)
	v.maxPct.SetText(strconv.FormatFloat(cfg.MaxOrphanPct, 'f', -1, 64))
	v.keepDirs.Value = cfg.KeepEmptyDirs
	return v
}

// fillConfig copies the screen's settings into cfg for saving. An unparsable
// max orphan % keeps the previous value rather than saving garbage.
func (v *pruneView) fillConfig(cfg *Config) {
	cfg.XMLPath = strings.TrimSpace(v.xml.Text())
	cfg.MusicDir = strings.TrimSpace(v.music.Text())
	cfg.Extensions = strings.TrimSpace(v.exts.Text())
	if pct, err := strconv.ParseFloat(strings.TrimSpace(v.maxPct.Text()), 64); err == nil {
		cfg.MaxOrphanPct = pct
	}
	cfg.KeepEmptyDirs = v.keepDirs.Value
}

func (v *pruneView) inputs() pruneInputs {
	return pruneInputs{
		xml:           v.xml.Text(),
		music:         v.music.Text(),
		exts:          v.exts.Text(),
		maxPct:        v.maxPct.Text(),
		keepEmptyDirs: v.keepDirs.Value,
	}
}

func (v *pruneView) update(gtx C) {
drain:
	for {
		select {
		case f := <-v.done:
			f()
		default:
			break drain
		}
	}

	for _, ed := range []*widget.Editor{&v.xml, &v.music, &v.exts, &v.maxPct} {
		for {
			ev, ok := ed.Update(gtx)
			if !ok {
				break
			}
			switch ev.(type) {
			case widget.ChangeEvent:
				v.state.inputsChanged()
			case widget.SubmitEvent:
				v.scan()
			}
		}
	}
	for {
		ev, ok := v.filter.Update(gtx)
		if !ok {
			break
		}
		if _, ok := ev.(widget.ChangeEvent); ok {
			v.refreshList()
		}
	}
	if v.keepDirs.Update(gtx) {
		v.state.inputsChanged()
	}
	if v.override.Update(gtx) {
		v.state.override = v.override.Value
	}

	if v.browseXML.Clicked(gtx) && !v.browsing {
		v.browse(&v.xml, false)
	}
	if v.browseMusic.Clicked(gtx) && !v.browsing {
		v.browse(&v.music, true)
	}
	if v.scanBtn.Clicked(gtx) {
		v.scan()
	}
	if v.advancedBtn.Clicked(gtx) {
		v.showAdvanced = !v.showAdvanced
	}
	for i := range v.tabs {
		if v.tabs[i].Clicked(gtx) && v.tab != i {
			v.tab = i
			v.list.Position = layout.Position{}
		}
	}
	if v.trashBtn.Clicked(gtx) {
		v.askConfirm()
	}
	if v.cancelBtn.Clicked(gtx) || v.scrim.Clicked(gtx) {
		v.state.cancelConfirm()
	}
	v.sink.Clicked(gtx) // swallowed on purpose
	if v.confirmBtn.Clicked(gtx) {
		v.trash()
	}
	if v.finderBtn.Clicked(gtx) {
		v.showReport()
	}
	for {
		ev, ok := gtx.Event(key.Filter{Name: key.NameEscape})
		if !ok {
			break
		}
		if e, ok := ev.(key.Event); ok && e.State == key.Press {
			v.state.cancelConfirm()
		}
	}

	// The state can clear the override (new scan, changed inputs); keep the
	// checkbox showing what the state will actually use.
	v.override.Value = v.state.override
}

func (v *pruneView) scan() {
	if v.state.busy() {
		return
	}
	opts, err := buildOptions(v.inputs(), v.home)
	if err != nil {
		v.state.fail(err)
		return
	}
	if !v.state.startScan() {
		return
	}
	v.save()
	go func() {
		r, err := safely(func() (*prune.Result, error) { return prune.Scan(opts) })
		v.done <- func() {
			v.state.scanDone(r, err)
			v.refreshList()
		}
		v.window.Invalidate()
	}()
}

// browse opens a native picker without blocking the UI.
func (v *pruneView) browse(ed *widget.Editor, dir bool) {
	v.browsing = true
	start := expandHome(ed.Text(), v.home)
	go func() {
		var opts []zenity.Option
		if dir {
			opts = append(opts, zenity.Directory(), zenity.Title("Choose the music folder"))
			if start != "" {
				opts = append(opts, zenity.Filename(start+string(filepath.Separator)))
			}
		} else {
			opts = append(opts, zenity.Title("Choose the rekordbox XML export"),
				zenity.FileFilters{{Name: "rekordbox XML", Patterns: []string{"*.xml"}}})
			if start != "" {
				opts = append(opts, zenity.Filename(start))
			}
		}
		path, err := zenity.SelectFile(opts...)
		if err != nil && !errors.Is(err, zenity.ErrCanceled) {
			log.Printf("file dialog: %v", err)
		}
		v.done <- func() {
			v.browsing = false
			if path != "" && !v.state.busy() {
				ed.SetText(path)
				v.state.inputsChanged()
				v.save()
			}
		}
		v.window.Invalidate()
	}()
}

// askConfirm fixes the report path before the dialog opens, so the dialog can
// show exactly where the report will be written.
func (v *pruneView) askConfirm() {
	if !v.state.canTrash() {
		return
	}
	dir, err := reportsDir()
	if err != nil {
		v.state.fail(fmt.Errorf("finding a place for the report: %w", err))
		return
	}
	v.reportPath = reportPath(dir, time.Now())
	v.state.askConfirm()
}

func (v *pruneView) trash() {
	r, force, ok := v.state.startTrash()
	if !ok {
		return
	}
	path := v.reportPath
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		v.state.trashDone(nil, fmt.Errorf("creating the reports folder, so nothing was moved: %w", err))
		return
	}
	go func() {
		ar, err := safely(func() (*prune.ApplyResult, error) {
			return prune.Apply(r, prune.ExecRunner{}, force, path)
		})
		v.done <- func() {
			v.state.trashDone(ar, err)
			v.list.Position = layout.Position{}
		}
		v.window.Invalidate()
	}()
}

func (v *pruneView) showReport() {
	if v.state.applied == nil {
		return
	}
	path := v.state.applied.ReportPath
	go func() {
		if err := exec.Command("open", "-R", path).Run(); err != nil {
			log.Printf("show report: %v", err)
		}
	}()
}

func (v *pruneView) refreshList() {
	v.list.Position = layout.Position{}
	if v.state.result == nil {
		v.filtered, v.details = nil, nil
		return
	}
	p := v.state.result.Plan
	v.filtered = filterOrphans(p.Orphans, p.MusicDir, v.filter.Text())
	v.details = detailRows(p)
}

func (v *pruneView) status() (string, color.NRGBA) {
	s := &v.state
	switch s.phase {
	case phaseScanning:
		return "Reading the export and walking the music folder…", pal.Muted
	case phaseTrashing:
		return "Writing the report, then moving files to the Trash…", pal.Muted
	case phaseDone:
		if n := len(s.applied.TrashErrs); n > 0 {
			return fmt.Sprintf("%d trash batches failed; the files listed below may still be on disk. Scan again to see what is left.", n), pal.Error
		}
		return "Done.", pal.Success
	}
	if s.err != nil {
		return s.err.Error(), pal.Error
	}
	return "", pal.Muted
}

// ---- Layout ----

func (v *pruneView) Layout(gtx C, th *material.Theme) D {
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx C) D { return v.layoutHeader(gtx, th) }),
		vspace(18),
		layout.Rigid(func(gtx C) D { return v.layoutInputs(gtx, th) }),
		vspace(16),
		layout.Flexed(1, func(gtx C) D { return v.layoutResults(gtx, th) }),
		layout.Rigid(func(gtx C) D {
			msg, c := v.status()
			if msg == "" {
				return D{}
			}
			return layout.Inset{Top: 10}.Layout(gtx, func(gtx C) D {
				l := material.Body2(th, msg)
				l.Color = c
				l.MaxLines = 3
				return l.Layout(gtx)
			})
		}),
	)
}

func (v *pruneView) layoutHeader(gtx C, th *material.Theme) D {
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx C) D {
			l := material.H5(th, "Rekordbox Prune")
			l.Font.Weight = font.Bold
			return l.Layout(gtx)
		}),
		vspace(2),
		layout.Rigid(label(th, "Move audio files that are no longer in your rekordbox library to the Trash.", pal.Muted)),
	)
}

func (v *pruneView) layoutInputs(gtx C, th *material.Theme) D {
	if v.state.busy() {
		gtx = gtx.Disabled()
	}
	scan := func(gtx C) D {
		text := "Scan"
		if v.state.phase == phaseScanning {
			text = "Scanning…"
		}
		return button(th, &v.scanBtn, text, true)(gtx)
	}
	return card(gtx, func(gtx C) D {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(fieldLabel(th, "rekordbox XML export")),
			vspace(6),
			layout.Rigid(func(gtx C) D {
				return v.pathRow(gtx, th, &v.xml, &v.browseXML, "~/Library/Pioneer/rekordbox/rekordbox.xml", nil)
			}),
			vspace(14),
			layout.Rigid(fieldLabel(th, "Music folder")),
			vspace(6),
			layout.Rigid(func(gtx C) D {
				return v.pathRow(gtx, th, &v.music, &v.browseMusic, "~/Music/rekordbox", scan)
			}),
			vspace(10),
			layout.Rigid(func(gtx C) D { return v.layoutAdvanced(gtx, th) }),
		)
	})
}

func (v *pruneView) pathRow(gtx C, th *material.Theme, ed *widget.Editor, browse *widget.Clickable, hint string, extra layout.Widget) D {
	children := []layout.FlexChild{
		layout.Flexed(1, func(gtx C) D { return input(gtx, th, ed, hint) }),
		hspace(8),
		layout.Rigid(func(gtx C) D {
			if v.browsing {
				gtx = gtx.Disabled()
			}
			return button(th, browse, "Browse…", false)(gtx)
		}),
	}
	if extra != nil {
		children = append(children, hspace(8), layout.Rigid(extra))
	}
	return layout.Flex{Alignment: layout.Middle}.Layout(gtx, children...)
}

func (v *pruneView) layoutAdvanced(gtx C, th *material.Theme) D {
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx C) D {
			text := "Show advanced options"
			if v.showAdvanced {
				text = "Hide advanced options"
			}
			return material.Clickable(gtx, &v.advancedBtn, func(gtx C) D {
				return layout.Inset{Top: 4, Bottom: 4}.Layout(gtx, label(th, text, pal.Accent))
			})
		}),
		layout.Rigid(func(gtx C) D {
			if !v.showAdvanced {
				return D{}
			}
			return layout.Inset{Top: 10}.Layout(gtx, func(gtx C) D {
				return layout.Flex{Alignment: layout.End}.Layout(gtx,
					layout.Flexed(2, labeled(th, "Audio extensions", func(gtx C) D {
						return input(gtx, th, &v.exts, prune.DefaultExtensions)
					})),
					hspace(12),
					layout.Flexed(1, labeled(th, "Max orphan %", func(gtx C) D {
						return input(gtx, th, &v.maxPct, "60")
					})),
					hspace(12),
					layout.Rigid(func(gtx C) D {
						cb := material.CheckBox(th, &v.keepDirs, "Keep empty folders")
						cb.Color, cb.IconColor = pal.Fg, pal.Accent
						return layout.Inset{Bottom: 8}.Layout(gtx, cb.Layout)
					}),
				)
			})
		}),
	)
}

func labeled(th *material.Theme, title string, w layout.Widget) layout.Widget {
	return func(gtx C) D {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(fieldLabel(th, title)),
			vspace(6),
			layout.Rigid(w),
		)
	}
}

func (v *pruneView) layoutResults(gtx C, th *material.Theme) D {
	gtx.Constraints.Min = gtx.Constraints.Max
	return card(gtx, func(gtx C) D {
		gtx.Constraints.Min = gtx.Constraints.Max
		switch v.state.phase {
		case phaseScanned, phaseConfirming, phaseTrashing:
			return v.layoutPlan(gtx, th)
		case phaseDone:
			return v.layoutDone(gtx, th)
		case phaseScanning:
			return centered(gtx, th, "Scanning…")
		}
		return centered(gtx, th, "Press Scan to compare the music folder with the rekordbox export.\nNothing is moved until you confirm.")
	})
}

func (v *pruneView) layoutPlan(gtx C, th *material.Theme) D {
	p := v.state.result.Plan
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx C) D {
			return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
				layout.Flexed(1, func(gtx C) D {
					l := material.H6(th, summaryLine(p))
					l.Font.Weight = font.SemiBold
					return l.Layout(gtx)
				}),
				hspace(12),
				layout.Rigid(func(gtx C) D {
					text := "Move to Trash"
					if n := len(p.Orphans); n > 0 {
						text = fmt.Sprintf("Move %d files to Trash", n)
					}
					if !v.state.canTrash() {
						gtx = gtx.Disabled()
					}
					return button(th, &v.trashBtn, text, true)(gtx)
				}),
			)
		}),
		layout.Rigid(func(gtx C) D { return v.layoutFindings(gtx, th) }),
		layout.Rigid(func(gtx C) D { return v.layoutOverride(gtx, th) }),
		layout.Rigid(func(gtx C) D {
			line := folderLine(p, 5)
			if line == "" {
				return D{}
			}
			return layout.Inset{Top: 10}.Layout(gtx, func(gtx C) D {
				l := material.Body2(th, line)
				l.Color = pal.Muted
				l.MaxLines = 2
				return l.Layout(gtx)
			})
		}),
		vspace(14),
		layout.Rigid(func(gtx C) D {
			labels := []string{fmt.Sprintf("Orphans (%d)", len(p.Orphans)), "Details"}
			return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
				layout.Rigid(func(gtx C) D { return segmented(gtx, th, v.tabs[:], labels, v.tab) }),
				hspace(12),
				layout.Flexed(1, func(gtx C) D {
					if v.tab != tabOrphans {
						return D{}
					}
					return input(gtx, th, &v.filter, "Filter orphans")
				}),
			)
		}),
		vspace(10),
		layout.Rigid(divider),
		layout.Flexed(1, func(gtx C) D { return v.layoutList(gtx, th) }),
	)
}

func (v *pruneView) layoutFindings(gtx C, th *material.Theme) D {
	lines := findingLines(v.state.result.Findings, maxFindingLines)
	children := make([]layout.FlexChild, len(lines))
	for i, fl := range lines {
		c, prefix := pal.Warn, "Warning: "
		switch {
		case fl.more:
			c, prefix = pal.Muted, ""
		case fl.abort:
			c, prefix = pal.Error, "Stops the run: "
		}
		children[i] = layout.Rigid(func(gtx C) D {
			return layout.Inset{Top: 6}.Layout(gtx, label(th, prefix+fl.text, c))
		})
	}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
}

func (v *pruneView) layoutOverride(gtx C, th *material.Theme) D {
	s := &v.state
	var w layout.Widget
	switch {
	case len(s.result.Blocked()) > 0:
		w = label(th, "These findings cannot be overridden. Fix them in rekordbox, export again and rescan.", pal.Error)
	case s.overrideRefusedForHome():
		w = label(th, "Overrides are not offered when Music is ~/Music itself. Point it at your rekordbox folder.", pal.Error)
	case s.showOverride():
		cb := material.CheckBox(th, &v.override, "I understand. Override these findings.")
		cb.Color, cb.IconColor = pal.Fg, pal.Error
		w = cb.Layout
	default:
		return D{}
	}
	return layout.Inset{Top: 10}.Layout(gtx, w)
}

func (v *pruneView) layoutList(gtx C, th *material.Theme) D {
	p := v.state.result.Plan
	if v.tab == tabDetails {
		if len(v.details) == 0 {
			return centered(gtx, th, "Nothing else to report.")
		}
		return material.List(th, &v.list).Layout(gtx, len(v.details), func(gtx C, i int) D {
			row := v.details[i]
			if row.heading {
				return layout.Inset{Top: 12, Bottom: 4, Left: 10}.Layout(gtx, fieldLabel(th, row.text))
			}
			return listRow(gtx, th, i, row.text, pal.Fg, 1)
		})
	}
	if len(p.Orphans) == 0 {
		return centered(gtx, th, "Nothing to remove.")
	}
	if len(v.filtered) == 0 {
		return centered(gtx, th, "No orphans match the filter.")
	}
	return material.List(th, &v.list).Layout(gtx, len(v.filtered), func(gtx C, i int) D {
		return listRow(gtx, th, i, relPath(p.MusicDir, p.Orphans[v.filtered[i]]), pal.Fg, 1)
	})
}

func (v *pruneView) layoutDone(gtx C, th *material.Theme) D {
	ar := v.state.applied
	var errs []string
	for _, err := range ar.TrashErrs {
		errs = append(errs, err.Error())
	}
	for _, err := range ar.DirErrs {
		errs = append(errs, err.Error())
	}
	head := pal.Success
	if len(ar.TrashErrs) > 0 {
		head = pal.Error
	}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx C) D {
			l := material.H6(th, fmt.Sprintf("Moved %d of %d files to the Trash.", ar.Moved, ar.Total))
			l.Color = head
			l.Font.Weight = font.SemiBold
			return l.Layout(gtx)
		}),
		layout.Rigid(func(gtx C) D {
			if v.state.result.Options.KeepEmptyDirs {
				return D{}
			}
			return layout.Inset{Top: 4}.Layout(gtx,
				label(th, fmt.Sprintf("Removed %d empty folders.", len(ar.RemovedDirs)), pal.Fg))
		}),
		vspace(14),
		layout.Rigid(func(gtx C) D {
			return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
				layout.Flexed(1, label(th, "Report: "+ar.ReportPath, pal.Fg)),
				hspace(8),
				layout.Rigid(button(th, &v.finderBtn, "Show in Finder", false)),
			)
		}),
		vspace(8),
		layout.Rigid(label(th, restoreHint, pal.Muted)),
		vspace(14),
		layout.Flexed(1, func(gtx C) D {
			if len(errs) == 0 {
				return D{}
			}
			return material.List(th, &v.list).Layout(gtx, len(errs), func(gtx C, i int) D {
				return listRow(gtx, th, i, errs[i], pal.Error, 0)
			})
		}),
	)
}

// layoutModal draws the confirmation dialog over the whole window. It is drawn
// last, so it is on top for both painting and pointer input.
func (v *pruneView) layoutModal(gtx C, th *material.Theme) D {
	if v.state.phase != phaseConfirming {
		return D{}
	}
	p := v.state.result.Plan
	gtx.Constraints.Min = gtx.Constraints.Max
	return layout.Stack{Alignment: layout.Center}.Layout(gtx,
		layout.Expanded(func(gtx C) D {
			return v.scrim.Layout(gtx, func(gtx C) D { return fill(gtx, withAlpha(rgb(0x000000), 0x99)) })
		}),
		layout.Stacked(func(gtx C) D {
			width := min(gtx.Constraints.Max.X, gtx.Dp(unit.Dp(520)))
			gtx.Constraints.Min.X, gtx.Constraints.Max.X = width, width
			return v.sink.Layout(gtx, func(gtx C) D {
				return card(gtx, func(gtx C) D {
					return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
						layout.Rigid(func(gtx C) D {
							l := material.H6(th, fmt.Sprintf("Move %d files to the Trash?", len(p.Orphans)))
							l.Font.Weight = font.SemiBold
							return l.Layout(gtx)
						}),
						vspace(10),
						layout.Rigid(label(th, fmt.Sprintf("%s will be moved out of %s.",
							prune.HumanBytes(p.OrphanSize), p.MusicDir), pal.Fg)),
						vspace(8),
						layout.Rigid(label(th, "A report listing every file is saved first, to:\n"+v.reportPath, pal.Muted)),
						layout.Rigid(func(gtx C) D {
							if !v.state.force() {
								return D{}
							}
							return layout.Inset{Top: 10}.Layout(gtx, label(th,
								"You are overriding: "+findingMessages(v.state.result.Forceable()), pal.Error))
						}),
						vspace(18),
						layout.Rigid(func(gtx C) D {
							return layout.Flex{}.Layout(gtx,
								layout.Flexed(1, layout.Spacer{}.Layout),
								layout.Rigid(button(th, &v.cancelBtn, "Cancel", false)),
								hspace(8),
								layout.Rigid(func(gtx C) D {
									b := material.Button(th, &v.confirmBtn, "Move to Trash")
									styleButton(&b)
									b.Background = pal.Error
									return b.Layout(gtx)
								}),
							)
						}),
					)
				})
			})
		}),
	)
}

func centered(gtx C, th *material.Theme, msg string) D {
	return layout.Center.Layout(gtx, func(gtx C) D {
		l := material.Body1(th, msg)
		l.Color = pal.Muted
		l.Alignment = text.Middle
		return l.Layout(gtx)
	})
}

// listRow draws one striped row. maxLines 0 lets the text wrap.
func listRow(gtx C, th *material.Theme, i int, s string, c color.NRGBA, maxLines int) D {
	bg := pal.Surface
	if i%2 == 1 {
		bg = pal.Surface2
	}
	return layout.Background{}.Layout(gtx, rounded(bg, 6), func(gtx C) D {
		return layout.Inset{Top: 7, Bottom: 7, Left: 10, Right: 10}.Layout(gtx, func(gtx C) D {
			gtx.Constraints.Min.X = gtx.Constraints.Max.X
			l := material.Body2(th, s)
			l.Color = c
			l.MaxLines = maxLines
			return l.Layout(gtx)
		})
	})
}
```

- [ ] **Step 2: Replace `cmd/djtools-app/main.go` with the wired version**

This matches the Task 6 scaffold, plus the `prune` field, a real content area, and the modal.

```go
// Command djtools-app is the desktop front end for djtools.
package main

import (
	"image/color"
	"log"
	"os"

	"gioui.org/app"
	"gioui.org/font"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
)

func main() {
	go func() {
		window := new(app.Window)
		window.Option(
			app.Title("djtools"),
			app.Size(unit.Dp(1100), unit.Dp(760)),
			app.MinSize(unit.Dp(820), unit.Dp(520)),
		)
		if err := run(window); err != nil {
			log.Fatal(err)
		}
		os.Exit(0)
	}()
	setAppIcon()
	app.Main()
}

type appUI struct {
	window       *app.Window
	cfg          Config
	themeMode    ThemeMode
	themeButtons [3]widget.Clickable
	sysTheme     systemTheme
	prune        *pruneView
}

func run(window *app.Window) error {
	th := newTheme()
	var ops op.Ops
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	ui := &appUI{window: window, cfg: loadConfig()}
	ui.themeMode = ui.cfg.Theme
	if ui.themeMode == "" {
		ui.themeMode = ThemeSystem
	}
	ui.prune = newPruneView(window, ui.cfg, home, ui.saveConfig)
	ui.sysTheme.watch(window)

	for {
		switch e := window.Event().(type) {
		case app.DestroyEvent:
			return e.Err
		case app.FrameEvent:
			gtx := app.NewContext(&ops, e)
			ui.update(gtx)
			applyTheme(th, ui.themeMode, ui.sysTheme.dark.Load())
			ui.Layout(gtx, th)
			e.Frame(gtx.Ops)
		}
	}
}

func (ui *appUI) saveConfig() {
	ui.prune.fillConfig(&ui.cfg)
	ui.cfg.Theme = ui.themeMode
	if err := saveConfig(ui.cfg); err != nil {
		log.Printf("saving config: %v", err)
	}
}

func (ui *appUI) update(gtx C) {
	for i := range ui.themeButtons {
		if ui.themeButtons[i].Clicked(gtx) && ui.themeMode != themeModes[i] {
			ui.themeMode = themeModes[i]
			ui.saveConfig()
		}
	}
	ui.prune.update(gtx)
}

func (ui *appUI) Layout(gtx C, th *material.Theme) D {
	fill(gtx, pal.Bg)
	gtx.Constraints.Min = gtx.Constraints.Max
	layout.Flex{}.Layout(gtx,
		layout.Rigid(func(gtx C) D { return ui.layoutSidebar(gtx, th) }),
		layout.Flexed(1, func(gtx C) D {
			return layout.UniformInset(unit.Dp(24)).Layout(gtx, func(gtx C) D {
				return ui.prune.Layout(gtx, th)
			})
		}),
	)
	ui.prune.layoutModal(gtx, th)
	return D{Size: gtx.Constraints.Max}
}

func (ui *appUI) layoutSidebar(gtx C, th *material.Theme) D {
	width := gtx.Dp(unit.Dp(232))
	gtx.Constraints.Min.X, gtx.Constraints.Max.X = width, width
	gtx.Constraints.Min.Y = gtx.Constraints.Max.Y
	return layout.Background{}.Layout(gtx, rounded(pal.Surface, 0), func(gtx C) D {
		return layout.UniformInset(unit.Dp(14)).Layout(gtx, func(gtx C) D {
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
				layout.Rigid(func(gtx C) D {
					l := material.H6(th, "djtools")
					l.Font.Weight = font.Bold
					return layout.Inset{Top: 6, Left: 10, Bottom: 18}.Layout(gtx, l.Layout)
				}),
				layout.Rigid(func(gtx C) D { return navItem(gtx, th, "Rekordbox Prune", true) }),
				layout.Flexed(1, layout.Spacer{}.Layout),
				layout.Rigid(func(gtx C) D {
					return segmented(gtx, th, ui.themeButtons[:], themeLabels(), themeIndex(ui.themeMode))
				}),
			)
		})
	})
}

// navItem is one tool in the sidebar. There is only one tool so far, so it is
// always selected and not yet clickable.
func navItem(gtx C, th *material.Theme, text string, selected bool) D {
	gtx.Constraints.Min.X = gtx.Constraints.Max.X
	bg := color.NRGBA{}
	if selected {
		bg = withAlpha(pal.Accent, 0x26)
	}
	return layout.Background{}.Layout(gtx, rounded(bg, 8), func(gtx C) D {
		return layout.Inset{Top: 10, Bottom: 8, Left: 10, Right: 10}.Layout(gtx, func(gtx C) D {
			l := material.Body1(th, text)
			l.Font.Weight = font.SemiBold
			if selected {
				l.Color = pal.Accent
			}
			return l.Layout(gtx)
		})
	})
}
```

- [ ] **Step 3: Build, vet, test**

Run: `gofmt -l cmd/djtools-app; go vet ./... && go test ./...`
Expected: no gofmt output; all `ok`. If the compiler flags a Gio API mismatch (v0.10 names), fix it against `~/go/pkg/mod/gioui.org@v0.10.2`. Don't change behaviour while doing so.

- [ ] **Step 4: Commit**

```bash
git add cmd/djtools-app
git commit -m "Add the Rekordbox Prune screen: scan, findings, override, confirm, trash"
```

---

### Task 9: Makefile, .gitignore, README

**Files:**
- Create: `Makefile`
- Modify: `.gitignore`, `README.md`

- [ ] **Step 1: Create `Makefile`** (recipe lines are indented with tabs)

```make
APP_NAME := djtools
APP_ID   := is.ankeri.djtools
APP_PKG  := ./cmd/djtools-app
DIST     := dist

GOGIO := go run gioui.org/cmd/gogio@v0.10.0

MAC_APP := $(DIST)/macos/$(APP_NAME).app

.PHONY: help run build test icon macos macos-arm64 macos-amd64 clean

help:
	@echo "Targets:"
	@echo "  run            Run the desktop app from source"
	@echo "  build          Build dj and djtools-app for this platform into $(DIST)/"
	@echo "  test           Run all tests"
	@echo "  icon           Regenerate cmd/djtools-app/icon.png"
	@echo "  macos          Universal (Apple Silicon + Intel) .app bundle"
	@echo "  macos-arm64    Apple Silicon .app bundle"
	@echo "  macos-amd64    Intel .app bundle"
	@echo "  clean          Remove $(DIST)/"

run:
	go run $(APP_PKG)

build:
	go build -o $(DIST)/dj ./cmd/dj
	go build -o $(DIST)/djtools-app $(APP_PKG)

test:
	go test ./...

icon:
	go generate $(APP_PKG)

# gogio marks the bundle as a generic bundle (BNDL); patch it to an
# application, then re-sign ad hoc since editing Info.plist breaks the signature.
define finish_mac_app
	plutil -replace CFBundlePackageType -string APPL "$(1)/Contents/Info.plist"
	plutil -replace CFBundleName -string "$(APP_NAME)" "$(1)/Contents/Info.plist"
	codesign --force --deep --sign - "$(1)"
endef

macos-arm64 macos-amd64: macos-%:
	rm -rf "$(DIST)/macos-$*"
	mkdir -p "$(DIST)/macos-$*"
	$(GOGIO) -target macos -arch $* -appid $(APP_ID) -icon $(APP_PKG)/icon.png -o "$(DIST)/macos-$*/$(APP_NAME).app" $(APP_PKG)
	$(call finish_mac_app,$(DIST)/macos-$*/$(APP_NAME).app)

# Build both architectures and merge the executables with lipo.
macos: macos-arm64 macos-amd64
	rm -rf "$(DIST)/macos"
	mkdir -p "$(DIST)/macos"
	cp -R "$(DIST)/macos-arm64/$(APP_NAME).app" "$(MAC_APP)"
	lipo -create \
		"$(DIST)/macos-arm64/$(APP_NAME).app/Contents/MacOS/$(APP_NAME)" \
		"$(DIST)/macos-amd64/$(APP_NAME).app/Contents/MacOS/$(APP_NAME)" \
		-output "$(MAC_APP)/Contents/MacOS/$(APP_NAME)"
	$(call finish_mac_app,$(MAC_APP))
	@echo "Built $(MAC_APP)"

clean:
	rm -rf $(DIST)
```

- [ ] **Step 2: Add build output to `.gitignore`**

Append:

```
# App build output
dist/
*.app/
*.syso
```

- [ ] **Step 3: Add a README section**

In `README.md`, insert this section directly before `## Design`:

````markdown
## Desktop app

`cmd/djtools-app` is a Gio desktop app with the same look as MP3 Renamer. Its first
tool, **Rekordbox Prune**, is `dj prune` with a window: pick the export and the music
folder, scan, read the findings and orphan list, and confirm. It runs the same checks
as the CLI (the shared `prune.Scan` / `prune.Apply`), so nothing the CLI would refuse
can be trashed from the app.

```sh
make run      # run from source
make macos    # dist/macos/djtools.app (universal)
```

Differences from the CLI:

- The defaults are rekordbox's own export location,
  `~/Library/Pioneer/rekordbox/rekordbox.xml`, and `~/Music/rekordbox`. After the
  first run it uses whatever you last chose.
- An overridable finding (the CLI's `--force`) needs the "I understand" checkbox.
  It is never offered when the music folder is `~/Music` itself.
- Every real run saves a report to
  `~/Library/Application Support/djtools/reports/` before moving anything, and
  won't move anything if the report can't be written.
- Settings live in `~/Library/Application Support/djtools/config.json`. Delete it
  to reset.
````

- [ ] **Step 4: Verify build targets**

Run: `make build && ls dist && make test`
Expected: `dist/dj` and `dist/djtools-app` exist; tests `ok`.

- [ ] **Step 5: Commit**

```bash
git add Makefile .gitignore README.md
git commit -m "Add Makefile for the desktop app and document it"
```

---

### Task 10: Manual verification

There's no Gio test harness, so check the screen by hand. **Do not trash anything from the real collection in this task.**

- [ ] **Step 1: Scan the real collection**

Run: `make run`. In the window, set XML to `~/DJ/rekordbox.xml` and Music to `~/DJ/music` (if `~/DJ` exists), then press Scan. Check:
- Inputs are disabled and the status says "Reading the export…" while it scans.
- The summary line matches `cd ~/DJ && dj prune --dry-run` (orphan count and size).
- Findings show in red/amber. The folder line lists the top folders. The Orphans tab lists relative paths, and the filter narrows them. The Details tab lists dead links, case duplicates and the rest.
- **Close the app without clicking Move.** Reopen it: the paths are remembered.

- [ ] **Step 2: Check the gates with a throwaway fixture**

```bash
T=$(mktemp -d) && mkdir -p "$T/music/Keep" "$T/music/Drop"
for i in $(seq 1 3); do head -c 1000 /dev/urandom > "$T/music/Drop/d$i.mp3"; done
echo "$T"
```

Point the app at `$T/music` with the real XML. Almost everything is an orphan, so the library-too-small and orphan-share aborts fire. Check:
- The Trash button is disabled, and the "I understand" checkbox appears.
- Ticking it enables the button. Clicking the button opens the dialog, which names the report path and lists what you are overriding. Esc, Cancel, and clicking the dimmed area all close it without moving anything.
- Changing any input while scanned clears the result.
- Set Music to `~` + `/Music` exactly: after scanning, the checkbox is replaced by the "~/Music itself" message. **Cancel before confirming anything there.**

- [ ] **Step 3: Real trash against a throwaway copy only**

Build a small self-consistent collection: 600 keepers listed in the XML, plus 5 orphans that aren't. There are no spaces in the paths, so no URL escaping is needed.

```bash
T=$(mktemp -d); M="$T/music"; mkdir -p "$M/Keep" "$M/Drop"
{
  echo '<?xml version="1.0" encoding="UTF-8"?>'
  echo '<DJ_PLAYLISTS Version="1.0.0"><PRODUCT Name="rekordbox" Version="7.2.18"/>'
  echo '<COLLECTION Entries="600">'
  for i in $(seq 1 600); do
    head -c 100 /dev/urandom > "$M/Keep/k$i.mp3"
    echo "<TRACK TrackID=\"$i\" Location=\"file://localhost$M/Keep/k$i.mp3\"/>"
  done
  echo '</COLLECTION><PLAYLISTS><NODE Type="0" Name="ROOT" Count="0"/></PLAYLISTS></DJ_PLAYLISTS>'
} > "$T/rekordbox.xml"
sleep 1   # orphans must predate the export, or they are skipped as newly added
touch -t 202001010000 "$M"/Keep/*.mp3
for i in 1 2 3 4 5; do head -c 200 /dev/urandom > "$M/Drop/d$i.mp3"; touch -t 202001010000 "$M/Drop/d$i.mp3"; done
echo "XML: $T/rekordbox.xml   Music: $M"
```

Check it with the CLI first: `go run ./cmd/dj prune --xml "$T/rekordbox.xml" --music "$M" --dry-run` should report 5 orphans and no ABORT lines. Then scan the same paths in the app, click **Move 5 files to Trash**, and confirm. Check:
- The Done view says "Moved 5 of 5 files to the Trash." and "Removed 1 empty folders." (`Drop`). **Show in Finder** reveals the report in `~/Library/Application Support/djtools/reports/`, and the restore hint is shown.
- `d1.mp3`…`d5.mp3` are in `~/.Trash`, and every `Keep/` file is still there. Afterwards, delete the five from the Trash and remove `$T`.

- [ ] **Step 4: Build the bundle**

Run: `make macos && open dist/macos/djtools.app`
Expected: the app launches with the vinyl icon in the Dock.

- [ ] **Step 5: Record anything that needed fixing**

If any of the checks needed code changes, commit them with a message saying what was wrong. If Esc did not close the dialog, note it in the commit. Cancel and the scrim are the primary ways to close it.
