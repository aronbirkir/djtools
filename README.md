# djtools

CLI tools for maintaining a rekordbox-backed DJ collection. The examples assume a
collection folder holding `rekordbox.xml` and a `music/` folder; this repo holds only
code.

```
dj prune       remove files from the music folder that are no longer in the rekordbox library
dj rename      rename MP3 files from their ID3 tags
```

Planned:

```
dj playlist    generate playlists from criteria (genre, BPM, key, energy)
dj enrich      fill missing tags, replace generic genres with specific ones
```

No tool in this repo ever writes to `rekordbox.xml`. The XML is read-only input; problems
found in it are reported for you to fix inside rekordbox.

## Usage

Build from this repo — the module is local, so `go install <path>@latest` will not
work:

```sh
cd ~/dev/experiment/go/djtools
go install ./cmd/dj          # -> $(go env GOPATH)/bin/dj
```

`$(go env GOPATH)/bin` is not on `PATH` by default. Either add it:

```sh
export PATH="$PATH:$(go env GOPATH)/bin"
```

or call the binary by its full path. Then run from the collection directory, so the
default `--xml` and `--music` values resolve:

```sh
cd /path/to/collection
dj prune --dry-run                        # scan and report, change nothing
dj prune --dry-run --report /tmp/p.txt    # plus full lists to grep
dj prune --dry-run --list                 # every orphan path, to eyeball or grep
dj prune                                  # summary, confirm, move to Trash
```

Flags:

| Flag | Default | Meaning |
| --- | --- | --- |
| `--xml` | `rekordbox.xml` | rekordbox XML export |
| `--music` | `music` | folder to prune |
| `--ext` | `.mp3,.wav,.aiff,.flac,.m4a` | audio extensions |
| `--dry-run` | off | scan and report, never trash |
| `--yes` | off | skip the confirmation prompt |
| `--force` | off | proceed despite abort-level findings |
| `--max-orphan-pct` | `60` | abort above this orphan share |
| `--list` | off | print every orphan path |
| `--report` | none | write full lists to a file |
| `--keep-empty-dirs` | off | leave emptied directories in place |

Exit codes: `0` success, `1` stopped (guard abort, declined, or usage error),
`2` files were trashed but some batches failed. If rekordbox.xml is rewritten
while the confirmation prompt is open, the run stops with exit `1`, moving nothing.

### Platform support

Pruning runs on macOS. The XML parsing and planning logic is portable and the binary
compiles for Windows and Linux, but both refuse at startup: file identity needs
platform-specific syscalls and there is no verified trash mechanism there. Since this
collection's Windows machine is retired, an untested deletion path was left unwritten
rather than shipped unverified.

### Re-running after trimming more tracks

1. Remove tracks from the rekordbox library.
2. Export the collection to `rekordbox.xml` in your collection folder.
3. `cd /path/to/collection && dj prune`.

The tool holds no state between runs, so it is safe to run as often as you like.
Every run recomputes from the current XML and the current disk. If a run fails
part way through, just run it again.

### Recovering a mistake

Files go to the macOS Trash rather than being unlinked, so nothing is destroyed
outright — but **Finder's "Put Back" does not work on them.** Put Back requires
Finder to have recorded the original location, and `/usr/bin/trash` does not do
that. This was claimed here before it was tested; it is not true.

To restore, use `tools/restore-from-trash.py`, which reconstructs each file's
original folder from the rekordbox exports and any saved `--report` file:

```sh
# list what the run moved (adjust the window to when you ran it)
find ~/.Trash -maxdepth 1 -newerct '2026-08-28 22:20:00' \
     ! -newerct '2026-08-28 22:35:00' > /tmp/trashed.txt

tools/restore-from-trash.py --trash-list /tmp/trashed.txt            # dry run
tools/restore-from-trash.py --trash-list /tmp/trashed.txt --apply    # restore
```

Restoring a file that is still absent from your rekordbox library means the next
run will trash it again. Add it back to the library first, or hold off on
pruning until you have decided.

**Save a report every run.** `--report` is what makes restoration precise:

```sh
dj prune --report prune-$(date +%Y%m%d-%H%M).txt
```

## dj rename

Renames the `.mp3` files directly inside a folder from their ID3 tags. It works the
same way as MP3 Renamer: `track01.mp3` becomes `123 04A Daft Punk - One More Time.mp3`.

```sh
dj rename --dry-run ~/Downloads/new-music          # preview, change nothing
dj rename ~/Downloads/new-music                    # preview, confirm, rename
dj rename --pattern '{artist} - {title}' --yes .   # custom pattern, no prompt
```

| Token | ID3 frame | Notes |
| --- | --- | --- |
| `{artist}` | TPE1 | Title Case |
| `{title}` | TIT2 | Title Case |
| `{bpm}` | TBPM | |
| `{key}` | TKEY | |

`{bpm:03}` zero-pads the value to at least 3 characters. The default pattern is
`{bpm:03} {key:03} {artist} - {title}`, and `.mp3` is added automatically.

Differences from MP3 Renamer:

- Title Case no longer capitalises after an apostrophe: `It's`, not `It'S`.
- It never overwrites a file. A file is skipped and listed if its new name is shared
  with another file, or is already taken in the folder.
- On volumes without atomic no-overwrite renames, such as exFAT USB sticks, each file
  is checked just before it is renamed.
- `/` and `:` in tags are replaced with `-`, so `AC/DC` no longer breaks the rename.
- A file missing a tag that the pattern uses is skipped, rather than being renamed to
  ` - .mp3`.
- Every occurrence of a token is filled. An unknown token such as `{album}` is an
  error.

Exit codes: `0` success, `1` stopped (declined, usage error or bad pattern), `2` some
renames failed.

Renaming files that rekordbox already knows makes them show as missing in rekordbox,
and the next `dj prune` will treat the renamed files as orphans. Relocate them in
rekordbox and re-export before pruning.

## Desktop app

`cmd/djtools-app` is a Gio desktop app with the same look as MP3 Renamer. Its first
tool, **Rekordbox Prune**, is `dj prune` with a window: pick the export and the music
folder, scan, read the findings and orphan list, and confirm. It runs the same checks
as the CLI (the shared `prune.Scan` / `prune.Apply`), so nothing the CLI would refuse
can be trashed from the app.

The second tool, **MP3 Rename**, is `dj rename` with a window. It works the same way
as MP3 Renamer: pick a folder, adjust the pattern, check the preview (with a Skipped
tab listing what was left alone and why), and confirm. Its folder and pattern are
remembered separately from the prune settings.

```sh
make run      # run from source
make macos    # dist/macos/djtools.app (universal)
```

Differences from the CLI:

- The defaults are rekordbox's own export location,
  `~/Library/Pioneer/rekordbox/rekordbox.xml`, and `~/Music/rekordbox`. After the
  first run it uses whatever you last chose.
- An overridable finding (the CLI's `--force`) needs the "I understand" checkbox.
  It is never offered when the music folder is `~/Music` or a folder containing it.
- Every real run saves a report to
  `~/Library/Application Support/djtools/reports/` before moving anything, and
  won't move anything if the report can't be written.
- To undo a run, copy its report to `/tmp/prune-report.txt` and follow
  [Recovering a mistake](#recovering-a-mistake). The restore script currently
  has its collection paths hard-coded; set `DJ` and `MUSIC` at the top of
  `tools/restore-from-trash.py` to your collection folder first.
- Settings live in `~/Library/Application Support/djtools/config.json`. Delete it
  to reset.

## Design

See `docs/superpowers/specs/` — the specs record the measured state of the collection and
the filesystem quirks (APFS case- and normalization-insensitivity, rekordbox's mixed
NFC/NFD output) that the implementation depends on.
