# Design: djtools desktop app (Gio)

Date: 2026-09-24
Status: draft

## Problem

`dj prune` is safe but terminal-only. Reading findings, eyeballing the orphan list
and deciding whether to override a guard is easier in a window. MP3 Renamer
(`~/dev/experiment/go/mp3renamer`) already set a Gio look and build pipeline; djtools
should match it.

## Scope

- A separate desktop app, `cmd/djtools-app`, built as a macOS `.app` bundle.
- The `dj` CLI stays, with unchanged flags, output and exit codes.
- The app has a tool sidebar with **Rekordbox Prune** (the UI for `dj prune`) as its only entry. `playlist` and
  `enrich` get added to the sidebar when their logic exists. There are no
  placeholder screens.
- MP3 Renamer's theme, config and dock-icon code is **copied and adapted**, not
  shared through a module.

Out of scope: Windows/Linux pruning (the app inherits `Supported()` and refuses),
restoring from the Trash inside the app, and editing `rekordbox.xml`, which stays
read-only.

## Layout

```
internal/rekordbox/        moved from cmd/dj/internal/rekordbox, unchanged
internal/prune/            moved from cmd/dj/internal/prune
  session.go   NEW  Scan / Apply: the single safety path for CLI and app
  command.go        flag parsing + printing on top of Scan / Apply
cmd/dj/                    CLI
cmd/djtools-app/
  main.go          window, event loop, sidebar, theme switch
  prune_view.go    Prune screen layout
  prune_state.go   Prune screen state machine, no Gio imports
  theme.go, config.go, dockicon_darwin.go, dockicon_other.go, icon.go
Makefile                   run / build / macos targets, modelled on mp3renamer's
```

The packages move because Go only allows `cmd/dj/...` to import `cmd/dj/internal`.
Gio and zenity join `go.mod`. The `dj` binary does not import them, so it doesn't
link them.

## `internal/prune` session API

`Run` currently mixes pre-flight checks, sequencing and printing. The checks and
sequencing move into:

```go
type Options struct {
    XMLPath, MusicDir string
    Extensions        []string // parsed; ParseExtensions handles the flag form
    MaxOrphanPct      float64
    KeepEmptyDirs     bool
}

type Result struct {
    Options    Options
    Plan       *Plan
    Findings   []Finding
    Collection *rekordbox.Collection
    XMLModTime time.Time
}

func Scan(opts Options) (*Result, error)
func (r *Result) Blocked() []Finding    // abort-level and Unforceable
func (r *Result) Forceable() []Finding  // abort-level and not Unforceable
func (r *Result) CanApply(force bool) error

type ApplyResult struct {
    Moved, Total int
    TrashErrs    []error
    RemovedDirs  []string
    DirErrs      []error
    ReportPath   string
}

func Apply(r *Result, runner Runner, force bool, reportPath string) (*ApplyResult, error)
```

`Scan` runs, in order: `Supported()`, stat and parse the XML, refuse a zero
modification time, `Build`, then `Check`. `TrashAvailable()` is not part of
`Scan`, so dry runs still work without `/usr/bin/trash`.

`Apply` enforces the rules itself and does not trust its caller:

1. `TrashAvailable()`.
2. `CanApply(force)`. Any unforceable finding refuses, **even with `force=true`**.
   Forceable findings refuse unless `force`.
3. Re-stat the XML. If its modification time differs from `XMLModTime`, refuse
   with "rekordbox.xml changed since the scan; scan again".
4. If `reportPath != ""`, write the report. If that fails, return the error
   with nothing trashed.
5. `Trash`, then `RemoveEmptyDirs` unless `KeepEmptyDirs`.

A partial trash failure is not a Go error. It comes back in `TrashErrs` so the
caller can report it; the CLI maps it to exit code 2.

The CLI keeps its own `--list`, `--report` (written after the summary, even on a
dry run), the `[y/N]` prompt and its exit codes. `command_test.go` must pass
unchanged.

## App

### Defaults

These apply only when no config file exists yet:

| Setting | Default |
| --- | --- |
| XML | macOS `~/Library/Pioneer/rekordbox/rekordbox.xml`; Windows `%AppData%\Pioneer\rekordbox\rekordbox.xml` |
| Music | `~/Music/rekordbox` |
| Extensions | the CLI's `defaultExtensions` |
| Max orphan % | `DefaultMaxOrphanPct` |
| Keep empty dirs | off |
| Theme | System |

The config lives at `~/Library/Application Support/djtools/config.json` (and its
Windows/Linux equivalents, as in mp3renamer). It is saved whenever a path, option or
theme changes.

Reports go to `~/Library/Application Support/djtools/reports/prune-YYYYMMDD-HHMMSS.txt`.
They don't go next to the XML because that directory belongs to rekordbox.

### Rekordbox Prune screen

The sidebar entry and screen title are both "Rekordbox Prune" (shortened in the sketch below).

```
┌──────────┬───────────────────────────────────────────────┐
│ djtools  │ XML   [~/Library/Pioneer/…/rekordbox.xml][Browse]│
│          │ Music [~/Music/rekordbox               ][Browse]│
│ ▸ Prune  │ ▸ Advanced (extensions, max orphan %, keep dirs)│
│          │                                   [Scan]       │
│          │ ─────────────────────────────────────────────  │
│          │ 312 orphans · 4.1 GB · 8.2% of 3,804 on disk   │
│          │ ⛔ orphan share 72% > 60%                       │
│          │    ☐ I understand — override these findings    │
│          │ ⚠ 3 dead links · 2 case duplicates  [details]  │
│          │ By folder:  House 120 (1.6 GB) · Techno 88 …    │
│          │ Orphans  [filter……]                            │
│          │   House/Old/track.mp3                          │
│          │   …                                            │
│          │                        [Move 312 files to Trash]│
│ ◐ theme  │ status line                                    │
└──────────┴───────────────────────────────────────────────┘
```

- The Browse buttons open zenity pickers (a file picker for the XML, a directory
  picker for Music) off the UI goroutine, the same way mp3renamer does.
- Abort findings are shown in the error colour and warnings in amber. Clicking
  "details" expands the full lists (dead links, case dupes, stale paths, recently
  added, leftovers, symlinks).
- The orphan list is virtualised with `widget.List`. Paths are shown relative to
  Music. There are no per-file sizes because `Plan` records only totals. A
  substring filter narrows the list for display only. It never changes what gets
  trashed.

### States

`Idle → Scanning → Scanned → Confirming → Trashing → Done`. Any failure goes to
`Error`, which keeps the last inputs.

- **Scanning** and **Trashing** run in goroutines. Results come back over a channel
  and then `window.Invalidate()` is called. Inputs, Scan and Trash are disabled
  while either runs.
- Changing XML, Music or any advanced option discards the current `Result` and
  returns to Idle.
- **The Trash button is enabled only when** the state is Scanned, there is at least
  one orphan, and `Result.CanApply(override)` returns nil.
- **The override checkbox is shown only when** there are forceable findings, there
  are no blocked findings, and Music is not exactly `~/Music` (compared after
  `filepath.Clean` and expanding `~`). An unforceable finding shows "cannot be
  overridden". For the `~/Music` case the app says "point Music at your rekordbox
  folder, not ~/Music". Both of those leave no way to trash from the app.
- **Confirming** is an in-window modal: "Move N files (size) to the Trash? A report
  will be saved to <path>." The buttons are Cancel (default, Esc) and Move to Trash.
- **Done** shows moved M of N, directories removed, any trash or directory errors,
  and the report path with **Show in Finder** (`open -R`). It also notes that
  Finder's Put Back does not work and points to `tools/restore-from-trash.py`.
  Scanning again starts over.

The rules above live in `prune_state.go` as plain Go, with no Gio types, so they
can be unit tested.

### Errors

- `Scan` errors (unsupported platform, unreadable or unparsable XML, zero
  modification time, missing Music folder) appear in the status line in the error
  colour, with no plan shown. They are the same strings as the CLI's.
- `Apply` errors (Trash unavailable, refused, XML changed, report unwritable) return
  to Scanned with the error shown and nothing trashed.
- Worker goroutines recover panics and report them as errors instead of taking
  down the window.

## Build

The Makefile copies mp3renamer's targets: `run` (`go run ./cmd/djtools-app`),
`build`, `macos` / `macos-arm64` / `macos-amd64` using gogio with appid
`is.ankeri.djtools`, the Info.plist patch and ad hoc re-signing, `icon` and
`clean`. There is no Windows target, since the app refuses to prune there. The
icon comes from a `tools/genicon` copied from mp3renamer.

## Testing

- `internal/prune/session_test.go`:
  - an unforceable finding refuses `Apply` with `force=true`;
  - a forceable finding refuses without `force` and proceeds with it;
  - the report is written before any trash call (a fake `Runner` records order),
    and an unwritable report path means zero trash calls;
  - touching the XML after `Scan` makes `Apply` refuse.
  `Scan` not calling `TrashAvailable` is a structural property, stated in its doc
  comment. It can't be tested on a Mac, where `/usr/bin/trash` always exists.
- The existing `internal/prune` and `internal/rekordbox` tests pass after the move,
  with `command_test.go` unchanged.
- `cmd/djtools-app/prune_state_test.go`: Trash button enablement, checkbox
  visibility (including the `~/Music` rule), and inputs changing to discard the
  result.
- The Gio layout is checked by hand with `make run` against the real collection,
  scanning only (no trashing), plus one run against a throwaway copy that moves
  files to the Trash.
