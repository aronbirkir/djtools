# Design: `dj prune` — rekordbox-driven music folder cleanup

Date: 2026-08-27
Status: approved

## Problem

The DJ collection at `/Users/aron/DJ` has drifted out of sync with rekordbox. Tracks removed
from the rekordbox library leave their files behind on disk. The library is trimmed
repeatedly and re-exported, so this is a recurring operation, not a one-off cleanup.

Measured state as of 2026-08-27, using the inode-matching algorithm this spec describes:

| Quantity | Value |
| --- | --- |
| Audio files under `music/` | 15,012 (136 GB), all `.mp3` |
| `<TRACK>` entries in `rekordbox.xml` | 8,617 |
| — resolving under `music/` | 8,367 |
| — stale Windows paths (`e:/`, `E:/`, `C:/`) | 240 |
| — outside `music/` (rekordbox Sampler `.wav`) | 8 |
| — not files at all (`tidal:`) | 2 |
| Library entries that stat successfully | 8,363 |
| Distinct files those entries point to | 8,355 (8 pairs are the same file, cased differently) |
| Library entries whose file is gone | 4 (all under `A2026-05` … `A2026-07`) |
| On-disk `.mp3` filenames stored non-NFC | 499 (155 of them in the library) |
| **Files on disk with no library entry** | **6,657 — 44.3% of on-disk audio, 35.9 GB (33.4 GiB)** |
| Non-audio files under `music/` | 35 (`.jpg`, `.webp`, `.txt`, `.xml`, `.DS_Store`) |
| Symlinks under `music/` | 0 |
| Top-level folders fully orphaned | 31 of 166 |

## Scope

This spec covers **only** the prune tool. Two further tools are planned and will each get
their own spec:

- `dj playlist` — generate playlists from criteria (genre, BPM, key, energy).
- `dj enrich` — fill missing tags and replace generic genres with specific ones. 1,798
  tracks have an empty genre, 781 are `Dance`, 415 are `Other`, 257 are `Music`.

The prune tool's XML-parsing layer is built to be reused by both. Nothing else is shared.

`dj prune` **never modifies `rekordbox.xml`.** It reads the XML and acts only on the
filesystem. Problematic library entries are reported for the user to clean up inside
rekordbox.

## Findings that drive the design

Five properties of the real data were verified before designing. Each one invalidates an
otherwise obvious implementation choice.

### 1. The disk stores NFD filenames; rekordbox writes NFC

`music/` is on APFS, case-insensitive and normalization-insensitive. `os.Stat` resolves a
path whether it is spelled NFC or NFD, but a directory walk returns each filename in
**whatever form was stored**.

Measured, not assumed:

- All 8,367 local `Location` values are NFC. **Zero** are NFD. (One NFD path exists in the
  export — `Zoë Johnston`, escaped `Zoe%cc%88` — but it is a stale `e:/` entry that is never
  statted, so it does not matter.)
- **499 on-disk `.mp3` filenames are stored non-NFC.** 155 of them are in the library; the
  other 344 are orphans regardless.

So the mismatch is **one-directional**: disk NFD versus XML NFC. Comparing decoded path
strings marks those **155 in-library files as orphans** — everything with accented or
Icelandic characters (`Loredana Bertè`, `Þú komst við hjartað í mér`, `Pépé Bradock`,
`Te Amo Corazón`). String matching yields 6,812 orphans; inode matching yields 6,657. That
155-file gap is the bug this design exists to prevent.

Note that high-byte percent-escapes are written in **lowercase** hex (`%cc`, `%ce`, `%8a`).
`url.PathUnescape` accepts either case, but test fixtures must use the real form.

### 2. The library contains case-only duplicate entries

Eight pairs of `<TRACK>` entries point at the same physical file with different casing —
``Jeu D`Amour`` / ``Jeu D`amour``, `Oliver Koletzki'S` / `Oliver Koletzki's`,
`Blackball Main Mixx` / `Blackball Main MixX`, `STEVE WATSON` / `Steve Watson`,
`WAND7R` / `Wand7r`.

On case-insensitive APFS these resolve to one file. Case-sensitive string matching treats
one spelling as valid and the other as a dead link, which is what inflated an earlier
estimate of dead links from 4 to 12. Case-folding strings would fix this particular
symptom, but only by re-deriving a rule the filesystem already enforces.

### 3. `+` appears literally in paths

28 locations contain an unescaped `+` (`Put Your Hands Up for Detroit + Pump Up the Volume`).
`%2B` never appears. Decoding with `url.QueryUnescape` converts `+` to a space and
wrongly orphans those 28 files. `url.PathUnescape` is required.

### 4. Windows locations have no separator after the host

Mac entries are `file://localhost/Users/aron/DJ/music/...`. Windows entries are
`file://localhoste:/music/...` — no slash between `localhost` and the drive letter.
`url.Parse` yields host `localhoste` and an empty path. Stripping the literal
`file://localhost` prefix handles both forms.

### 5. `%25` is present

Literal `%` characters are escaped in the XML, so decoding must happen exactly once.
Double-decoding corrupts paths.

## Approach

Identity is decided by the filesystem, not by string comparison. Each library location and
each file on disk is statted, and the `(device, inode)` pair is the identity key. APFS
resolves Unicode form and case itself, so the comparison is immune to normalization form,
casing, symlinks, and hard links — findings 1 and 2 both dissolve rather than needing
separate handling. That matters because they are independent failures: normalization
folding alone would still misreport the 8 case-duplicate entries, and case folding alone
would still orphan the 155 NFD files.

This needs no third-party dependency. The alternative
(`golang.org/x/text/unicode/norm` plus case folding) re-implements what the filesystem
already knows, and every normalization case not anticipated becomes a wrongly deleted file.

Cost is roughly 23,400 stat calls, negligible on APFS/SSD.

**Core invariant: uncertainty means keep.** A file is trashed only if it was statted
successfully *and* its `FileID` is proven absent from a library set that resolved without
error. Any stat failure other than `ENOENT` aborts the run rather than risking a deletion
based on an incomplete library set.

## Portability

The collection began on Windows and moved to the Mac, and the tools should not be gratuitously
macOS-only. Two pieces genuinely are platform-bound, so they are isolated behind build tags
rather than allowed to leak into the core:

| Concern | POSIX (darwin, linux) | Windows |
| --- | --- | --- |
| File identity | `(dev, ino)` from `syscall.Stat_t` | Would need `GetFileInformationByHandle` (volume serial + file index), which opens a handle per file rather than statting. **Not implemented.** |
| Trash | `/usr/bin/trash` | Would need `SHFileOperation` with `FOF_ALLOWUNDO`. **Not implemented.** |

Everything else — XML parsing, Location classification, planning, guards, reporting, batching,
empty-directory removal — is portable and tested on any host.

On Windows the tool **refuses to run with a clear message** rather than falling back to
anything weaker. That is deliberate: the Windows machine is retired, so a Windows code path
could not be tested on real hardware, and an untested deletion path is worse than no path.
`Supported()` is checked before any work, so the failure is immediate and unambiguous.

Note that the NFD/NFC problem is a macOS artifact. NTFS is case-insensitive but
normalization-*sensitive*, so on Windows two normalization forms are genuinely two files.
File-ID matching remains the right approach there — it is simply solving a problem that only
exists on one of the two platforms.

## Repository layout

Module `github.com/aronbirkir/djtools` at `~/dev/experiment/go/djtools`, stdlib only.

The layout follows the neighbouring `picoclaw` project (`cmd/<name>/internal/<subcommand>/`
with `command.go` and colocated tests) rather than `mp3renamer`'s flat `main` package,
because three subcommands will share a parser.

```
djtools/
  go.mod                                       module github.com/aronbirkir/djtools
  .gitignore
  README.md
  docs/superpowers/specs/
  cmd/dj/main.go                               subcommand dispatch, usage
  cmd/dj/internal/rekordbox/
    location.go                                Location string -> classified path
    location_test.go
    xml.go                                     XML structs, Parse()
    xml_test.go
  cmd/dj/internal/prune/
    command.go                                 flag parsing, orchestration
    fileid.go                                  FileID type, portable
    fileid_unix.go                             (dev, ino) via syscall.Stat_t
    fileid_unsupported.go                      stub that refuses early (!unix)
    fileid_test.go
    plan.go                                    file-id index -> Plan
    plan_test.go
    guard.go                                   sanity checks and thresholds
    guard_test.go
    apply.go                                   batching, empty-dir removal (portable)
    trash_darwin.go                            /usr/bin/trash
    trash_unsupported.go                       refuses on other platforms
    apply_test.go
    report.go                                  summary rendering
    report_test.go
```

Data stays at `/Users/aron/DJ`; only code lives in the repo. Default flag values are
relative to the working directory (`./rekordbox.xml`, `./music`) so that
`cd ~/DJ && dj prune` works.

## Components

### `internal/rekordbox`

`Parse(io.Reader) (*Collection, error)` decodes the `DJ_PLAYLISTS` document with
`encoding/xml`. The 17 MB file is read whole; streaming is unnecessary at this size.

`Collection` exposes the declared `Entries` attribute alongside the actual track count so a
truncated export can be detected. Playlist nodes are parsed for presence checking now, and
for `dj playlist` later.

`ClassifyLocation(raw, musicDir string, host Host) Location` returns a closed enum, because
the four kinds need different handling:

| Kind | Meaning | Count on macOS today | Prune behaviour |
| --- | --- | --- | --- |
| `KindLocal` | resolves under `--music` | 8,367 | participates in keep/orphan matching |
| `KindStale` | path shaped for a *different* host OS | 240 | reported for rekordbox cleanup |
| `KindForeign` | valid path for this host, outside `--music` | 8 | ignored; not a problem |
| `KindNonFile` | `tidal:` or no decodable path | 2 | ignored |

Decoding: strip the literal `file://localhost` prefix, then `url.PathUnescape` exactly
once, then classify.

`KindStale` is deliberately **not** defined as "has a drive letter". This collection was
exported from a Windows machine and imported on the Mac, which is where the 240 `e:/` and
`C:/` entries come from — but the same tool run on that Windows machine against
`--music e:/music` would find those entries live and the 8,367 `/Users/aron/...` entries
unreachable. So the rule is *host-relative*:

- On a POSIX host, a Windows drive path (`^[A-Za-z]:[/\\]`) is stale.
- On a Windows host, a POSIX absolute path (`/...`) is stale.
- Anything else absolute is `KindLocal` or `KindForeign` depending on whether it is under
  `--music`.

The host is a parameter rather than a direct `runtime.GOOS` read, so both branches are
testable from either platform. This matters because the Windows machine is retired and its
behaviour cannot be verified on real hardware.

The "under `--music`" test compares case-insensitively and treats `\` and `/` as
equivalent. Being liberal here is safe: classification only decides whether a path is worth
statting, and identity is still settled by the file ID. A false `KindLocal` costs one
wasted `os.Stat`, never a wrong deletion.

### `internal/prune` — `plan.go`

```go
type FileID struct{ Dev, Ino uint64 }

type Plan struct {
    Library    map[FileID][]string // resolved library files -> every path spelling
    Orphans    []string            // audio on disk, absent from Library
    Keepers    int
    DeadLinks  []string            // KindLocal, ENOENT
    Stale      []string            // KindStale
    StaleDupes map[string]int      // stale paths repeated across TrackIDs
    CaseDupes  [][]string          // one file, multiple cased entries
    Leftovers  []string            // non-audio files under music/
    Symlinks   []string            // never followed, never trashed
    Unresolved []error             // stat failures other than ENOENT
}
```

1. For each `KindLocal` location: `os.Stat`. Success appends the path under its `FileID`.
   `ENOENT` records a dead link. Any other error appends to `Unresolved`.
2. Any `FileID` with more than one path spelling becomes a `CaseDupes` entry.
3. Walk `--music` with `filepath.WalkDir`. For each entry, `os.Lstat`:
   - symlink → `Symlinks`, never trashed
   - audio extension (`--ext`, default `.mp3,.wav,.aiff,.flac,.m4a`) → `FileID` present in
     `Library` means keep, absent means `Orphans`
   - anything else → `Leftovers`, reported only

Reporting `Library` as `FileID -> []string` rather than `FileID -> string` is what surfaces
finding 2 instead of silently collapsing it.

### `internal/prune` — `guard.go`

`Check(plan, collection, opts) []Finding`, each finding either `LevelAbort` or `LevelWarn`.
Aborts are overridable with `--force`.

Abort conditions, each meaning the XML is not a trustworthy picture of the library:

- `COLLECTION` element missing, or zero tracks parsed
- declared `Entries` attribute does not match the actual `<TRACK>` count (truncated export)
- `PLAYLISTS` element missing
- fewer than 500 resolvable `KindLocal` tracks — indicates a playlist export, not a collection
- orphans exceed `--max-orphan-pct` of on-disk audio (default 60; the first real run is
  44.3%, which clears with headroom)
- `Unresolved` is non-empty
- more than 50 entries yield no usable path. Normally these are streaming tracks and the
  export has exactly 2; a sudden crop means rekordbox changed the `Location` format and
  decoding is failing systemically. A *total* failure would already be caught by the
  small-library and orphan-share guards, but a partial one might slip past both, and
  neither would report the real cause.

Warnings, which do not stop the run:

- XML mtime older than the newest audio file on disk — "tracks were added since this
  export; re-export first". Quiet on current data: 0 audio files are newer than the XML.
- `CaseDupes` present (8 today) — duplicate library entries worth merging in rekordbox
- `StaleDupes` present (30 today) — stale Windows paths repeated up to 6 times each
- `DeadLinks` present (4 today)

No state file is kept between runs; every check derives from the current XML and disk.

### `internal/prune` — `report.go`

Prints a per-top-level-folder table of `orphans / total`, sorted by orphan count, followed
by totals, reclaimable bytes, and the guard findings. `--list` prints every orphan path.
`--report <file>` writes the full orphan, dead-link, stale, leftover, symlink, and
duplicate lists for grepping and spot-checking.

Grouping is by top-level folder because that is how the collection is organised: genre
folders (`Dance`, `Pop`, `Rock`) and dated import batches (`A2026-01`).

### `internal/prune` — `apply.go`

After a single `y/N` confirmation, orphans are passed to the platform's trash mechanism in
batches of 200 paths to stay clear of `ARG_MAX`. On macOS that is `/usr/bin/trash`, which
ships with the OS, handles the accented and Icelandic filenames correctly (verified), and
records original paths so Finder's **Put Back** restores files individually. The batching
and error-collection logic is portable; only the binary and its availability check sit
behind a build tag.

Batch failures are collected and reported rather than aborting the run, and set a non-zero
exit code. A partially completed run needs no cleanup: re-running recomputes from scratch.

Then, unless `--keep-empty-dirs` is set, directories under `music/` are removed
deepest-first, and **only when they contain no entries at all**. A directory still holding a
`.DS_Store` or cover art is left alone and counted in the report as skipped, because
leftovers are reported rather than trashed and removing them would exceed what the user
confirmed. 31 folders are fully orphaned today, so at most 31 are removable; the actual
number depends on how many still hold a leftover. `music/` itself is never removed.

The command runner is an injectable interface so trashing is testable without touching the
real Trash.

## CLI

```
dj prune [flags]

  --xml path             rekordbox XML export        (default ./rekordbox.xml)
  --music path           music folder to prune       (default ./music)
  --ext list             audio extensions            (default .mp3,.wav,.aiff,.flac,.m4a)
  --dry-run              scan and report, never trash
  --yes                  skip the confirmation prompt
  --force                proceed despite abort-level guard findings
  --max-orphan-pct N     abort above this orphan share (default 60)
  --list                 print every orphan path
  --report file          write full lists to file
  --keep-empty-dirs      do not remove emptied directories
```

Exit codes: `0` success, `1` guard abort or user declined, `2` trash failures occurred.

## Error handling

| Situation | Behaviour |
| --- | --- |
| Unsupported platform | refuse immediately, before reading anything |
| XML unreadable or malformed | abort before any filesystem work |
| trash mechanism unavailable | abort before the confirmation prompt |
| Library location `ENOENT` | record dead link, continue |
| Library location other stat error | abort; the library set is incomplete |
| Walk error on a directory | abort; an unreadable directory could hide library files |
| Trash batch failure | report, continue remaining batches, exit 2 |
| Directory not empty at removal | skip silently |

## Testing

`plan_test.go` carries the regression test that matters: a temp-directory fixture with
files created in NFD and a collection referencing them in NFC, asserting **zero orphans**.

Every such test writes the two names as `\\u` escapes and first asserts they really are
different byte sequences. That is not defensive padding: the equivalent test in
`fileid_test.go` was first written with two byte-identical literals, so it statted one path
twice, compared a `FileID` to itself, and **passed while proving nothing**. Spelled as
literal accented characters the two forms are indistinguishable in an editor. A vacuous test
here would conceal precisely the failure that deletes 155 in-library files.
That is the observed real-world direction and the guard against the 155-file bug. The
reverse direction (NFD in the collection, NFC on disk) is also tested, defensively — it
does not occur in the current export, but nothing guarantees rekordbox will not emit it.

This is why matching is tested against real temp-directory fixtures rather than in-memory
strings: the behaviour under test belongs to the filesystem. On a normalization-sensitive
volume the test would be meaningless, so it probes the fixture directory first and skips
with an explanation rather than failing confusingly.

- `location_test.go` — table-driven over the real samples: unescaped `+`, `%25`, `%27`,
  `%26`, lowercase high-byte escapes, `file://localhoste:/`, `file://localhostC:/`,
  `tidal:tracks:`, and Sampler `.wav`. Each classification case runs against **both** host
  values, asserting that a drive path is stale on POSIX and local on Windows, and that a
  `/Users/...` path is the reverse. This is the only way the retired Windows machine's
  behaviour gets covered at all.
- `xml_test.go` — declared `Entries` vs actual count, missing `COLLECTION`, missing
  `PLAYLISTS`.
- `plan_test.go` — normalization fixtures both directions, case-variant entries collapsing
  to one `FileID` and populating `CaseDupes`, symlinks not followed, non-audio to
  `Leftovers`, dead links, stat-error propagation.
- `guard_test.go` — each abort and warn condition, and `--force` override behaviour.
- `apply_test.go` — batch sizing at boundaries (199/200/201 paths), partial batch failure
  handling, empty-dir removal only when empty, injected runner asserting exact arguments.
- `report_test.go` — golden output for the summary table.

## Verification on real data

Before the first destructive run, `--dry-run` output is checked against the measured table
at the top of this spec: 6,657 orphans, 8,355 keepers, 4 dead links, 240 stale entries, 8
case-duplicate pairs, 35 non-audio leftovers, 31 fully orphaned folders, 35.9 GB
reclaimable. A discrepancy means a parsing bug, not a changed collection.

A useful cross-check during development: implementing naive string matching alongside the
inode matcher should reproduce exactly 6,812 orphans. The 155-file difference confirms the
inode path is doing the work it was chosen for.
