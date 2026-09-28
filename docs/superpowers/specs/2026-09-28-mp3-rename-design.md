# Design: MP3 Rename as a djtools tool

Date: 2026-09-28
Status: draft

## Problem

MP3 Renamer (`~/dev/experiment/go/mp3renamer`) renames MP3 files from their ID3 tags,
for example `track01.mp3` → `123 04A Daft Punk - One More Time.mp3`. It's a separate
app. djtools is becoming the one place for DJ collection tools, so renaming should
live there too, both as a `dj rename` command and as a second tool in the desktop app.

djtools is a collection of independent tools. Rename doesn't know about rekordbox and
doesn't refuse library files. How the tools are combined is left to the DJ.

## Scope

- A new `internal/rename` package holds the pattern, plan and apply logic, shared by
  the CLI and the app.
- `dj rename [--pattern P] [--dry-run] [--yes] <folder>`.
- The desktop app sidebar becomes clickable with two tools, **Rekordbox Prune** and
  **MP3 Rename**.
- The pattern syntax, Title Case and default pattern are kept from MP3 Renamer. Its
  known bugs are fixed (see "Changes from MP3 Renamer").
- The mp3renamer repo is left untouched.

Out of scope: subfolders, new tokens, file types other than `.mp3`, reordering chained
renames, and any change to rekordbox.

## Changes from MP3 Renamer

| MP3 Renamer | djtools Rename |
| --- | --- |
| `os.Rename` silently overwrites when two files map to one name, or when the new name already exists | Never overwrites; such files are skipped and listed |
| `/` in a tag (e.g. `AC/DC`) makes the rename fail | `/` and `:` in tag values are replaced with `-` |
| A file with no tags becomes ` - .mp3` | A file with an empty tag that the pattern uses is skipped and listed |
| Each token is replaced at its first occurrence only; unknown tokens stay in the name | Every occurrence is replaced; an unknown token is a pattern error |

## Layout

```
internal/rename/
  pattern.go      ParsePattern(s) (Pattern, error); Pattern.Expand(tags) string
  plan.go         Plan(dir string, p Pattern) (*RenamePlan, error)
  apply.go        Apply(plan) *ApplyResult
  noreplace_darwin.go / noreplace_other.go   renameNoReplace(old, new) error
  command.go      Run(args, stdout, stderr, stdin) int   (dj rename)
cmd/dj/main.go    + "rename" subcommand and usage line
cmd/djtools-app/
  rename_state.go (+_test.go)   screen rules, no Gio imports
  rename_view.go, rename_layout.go
  main.go         clickable sidebar; each tool keeps its own state
  config.go       + RenameFolder, RenamePattern
```

New dependencies: `github.com/bogem/id3v2/v2 v2.1.4` (as in mp3renamer), plus
`golang.org/x/sys` and `golang.org/x/text` (for `unicode/norm`) as direct dependencies.

## `internal/rename`

### Pattern

- The tokens are `{artist}` (TPE1), `{title}` (TIT2), `{bpm}` (TBPM) and `{key}`
  (TKEY).
- `{token:N}` left-pads the value with zeros to at least N characters, as in MP3
  Renamer: `{bpm:03}` turns `95` into `095`.
- Every occurrence of a token is replaced.
- `ParsePattern` returns an error for an unknown token, an unclosed `{`, a non-numeric
  or negative `N`, a pattern with no tokens, or a literal `/` or `:` outside a token
  (either would move a rename's target into another folder).
- `.mp3` is appended to the expanded name.
- The default pattern is `{bpm:03} {key:03} {artist} - {title}`.

### Plan

```go
type Rename struct{ Old, New string }            // base names within Dir
type Skip   struct{ Name, Reason string }
type RenamePlan struct {
    Dir       string
    Renames   []Rename  // sorted by Old
    Skipped   []Skip    // sorted by Name
    Unchanged int       // already match the pattern
}
```

- `Plan` reads the top level of `dir` only. It ignores directories and anything that
  isn't `.mp3` (case-insensitive).
- For each file it reads the ID3 tags with `id3v2.Open(path, {Parse: true})`, closing
  the file afterwards. Artist and title go through Title Case, using the same result
  as MP3 Renamer's `strings.Title(strings.ToLower(s))` but implemented without the
  deprecated function. Then `/` and `:` in every value are replaced with `-`, and
  surrounding whitespace is trimmed.
- A file is skipped, with its reason, when:
  - its tags can't be read (`"cannot read tags: …"`);
  - a tag the pattern uses is empty (`"no {bpm} tag"`);
  - its new name, once both are NFC-normalised, equals its current name (counted as
    `Unchanged`, not skipped -- this also covers a purely Unicode-normalisation
    difference, such as an NFD-named file whose tags produce the NFC form);
  - its new name isn't a plain file name (`"… is not a plain file name"`), would be a
    hidden file starting with `.` (`"new name would be a hidden file"`), or is longer
    than 255 bytes (`"new name is too long (N bytes, the limit is 255)"`). In practice
    `ParsePattern` and `clean` already rule most of this out; the checks are defence in
    depth;
  - its new name collides, case-insensitively after NFC normalisation, with another
    file's new name. All files in that group are skipped (`"same new name as …"`);
  - its new name matches an existing file in the folder that isn't the same file
    (`"… already exists"`). "Same file" means `os.SameFile` **and** the two base names
    fold to the same value (case- and NFC-insensitively) -- so a hard link at the new
    name, under an unrelated name, is treated as a different file and refused, not as a
    case-only rename. This also covers another file's current name, so chains and
    swaps are skipped rather than reordered.
- A **case-only** rename (for example `daft punk - one.mp3` → `Daft Punk - One.mp3`)
  is allowed. The existing file is the same file under the same name save for case, so
  the rename isn't a collision.

### Apply

```go
type ApplyResult struct {
    Renamed  int
    Checked  int    // of Renamed, how many used check-then-rename, not an atomic rename
    Failures []Skip // Name = old name, Reason = error text
}
```

- `Apply` renames each entry through `renameNoReplace`, which reports whether that
  rename used the atomic path or had to fall back to a check-then-rename. One failure
  doesn't stop the others.
- `checkedRename` (`noreplace.go`) is the shared check-then-rename fallback: it uses the
  same same-file-and-matching-base-name rule `Plan`'s `occupant` uses to refuse an
  existing different file (never for a hard link under an unrelated name, which is
  always refused) or to allow a case-only rename, then calls `os.Rename` and confirms
  the old name is gone from the directory listing, to catch a rename that silently
  no-oped. This is not atomic: there is a small window between the check and the
  rename in which a file created at the new name would be overwritten.
- On darwin, `renameNoReplace` tries `unix.RenamexNp(old, new, unix.RENAME_EXCL)`
  first, so on a volume that supports it the kernel refuses atomically, closing that
  window: a file created after the plan was made can never be overwritten. On APFS this
  itself succeeds for a case-only rename. On a volume that instead refuses that with
  `EEXIST` (HFS+), or reports `ENOTSUP`/`EINVAL` because it has no such primitive at all
  (exFAT, observed even for an otherwise uncontested rename with no real collision),
  `renameNoReplace` falls back to `checkedRename`. (Verified against a real exFAT
  volume: with the fallback, a normal rename and a case-only rename both succeed there,
  and a genuine collision is still refused as `"… already exists"` -- confirmed both via
  `go test -run TestExternalVolume` and a real `dj rename --yes` run against a mounted
  exFAT image.)
- On other platforms `renameNoReplace` always uses `checkedRename` -- there is no
  atomic alternative to try first.

## `dj rename`

```
dj rename [--pattern P] [--dry-run] [--yes] <folder>
```

- The output is a two-column `OLD → NEW` table, then a `SKIPPED` section with reasons,
  then a totals line (`N to rename, M skipped, K already match`).
- `--dry-run` stops there. Otherwise the command asks
  `Rename N files? [y/N]`. Only `y` or `yes` renames, and `--yes` skips the prompt.
  Nothing is asked when there's nothing to rename.
- After `Renamed N of M files.`, if any of those used `checkedRename` (`ApplyResult.
  Checked > 0`), a line explains it: `"N of these were on a volume without atomic
  no-overwrite renames (e.g. exFAT); each was checked just before renaming."`
- Exit codes are the same as prune's: `0` ok, `1` stopped (declined, usage error,
  invalid pattern, unreadable folder), `2` some renames failed.

## App: MP3 Rename tool

- **Sidebar:** the sidebar becomes a list of clickable tools, Rekordbox Prune and
  MP3 Rename. The selected tool is highlighted. Each tool keeps its own state when you
  switch, and the selected tool is saved in the config.
- **Screen:** it follows MP3 Renamer's layout.
  - A folder field with Browse (zenity directory picker) and Scan.
  - A pattern field. Pressing Enter rescans.
  - The token hint line.
  - A preview list with current and new names side by side, and a second tab listing
    skipped files with reasons.
- **Renaming:** the **Rename N files** button opens the same kind of confirm dialog as
  prune, with Cancel focused. Only Rename in the dialog runs `Apply`.
- **Background work:** scan and apply run in goroutines, with results passed back
  through a `done` channel as in prune. Inputs are ignored while busy, and changing
  the folder or pattern discards the preview.
- **Errors:** an invalid pattern shows its error under the pattern field and disables
  Scan and Rename. After applying, the screen shows `Renamed N files` -- with
  `" (this volume lacks atomic renames; each file was checked just before renaming)"`
  appended when any of them used `checkedRename` -- lists any failures, and rescans
  automatically.
- **Config:** `RenameFolder` (default empty) and `RenamePattern` (default above) are
  added to `config.json`, independent of the prune paths. A new `Tool` field stores the
  last selected tool.

The rules live in `rename_state.go` and are unit-tested: when Scan and Rename are
enabled, discarding on input change, ignoring input while busy, and the confirm flow.

## Docs

The README gets a `dj rename` section (usage, flags, exit codes, the changes from MP3
Renamer) and a line in the Desktop app section. It also gets this note:

> Renaming files that rekordbox already knows makes them show as missing in rekordbox,
> and the next `dj prune` will treat the renamed files as orphans. Relocate them in
> rekordbox and re-export before pruning.

## Testing

- `pattern_test.go`: each token, `:N` padding, a value longer than N, repeated tokens,
  unknown or unclosed tokens, bad N, and a pattern with no tokens.
- `plan_test.go`: real temp dirs with MP3s tagged by `id3v2` in the test. It covers:
  - Title Case, and `/` and `:` being replaced;
  - an empty tag being skipped;
  - an unreadable file being skipped;
  - two files producing one name, both skipped;
  - a new name that already exists, skipped;
  - a case-only rename, allowed;
  - a chain (A→B while B exists), skipped;
  - `.txt` and subdirectories ignored;
  - `Unchanged` counted.
- `apply_test.go`:
  - a target created after `Plan` is not overwritten and is reported as a failure;
  - one failure doesn't stop the others;
  - a case-only rename works on the (case-insensitive) temp volume.
- `command_test.go` in the style of prune's: dry run renames nothing, declining or an
  empty answer renames nothing, `--yes` renames, an invalid pattern exits 1, and
  `--help` exits 0.
- `rename_state_test.go`: the enablement rules and the confirm flow.
- The Gio layout is checked by hand with `make run` against a throwaway folder of
  tagged MP3s.
