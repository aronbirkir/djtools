# djtools

CLI tools for maintaining a rekordbox-backed DJ collection. The library lives at
`/Users/aron/DJ`; this repo holds only code.

```
dj prune       remove files from the music folder that are no longer in the rekordbox library
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
cd ~/DJ
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
`2` files were trashed but some batches failed.

### Platform support

Pruning runs on macOS. The XML parsing and planning logic is portable and the binary
compiles for Windows and Linux, but both refuse at startup: file identity needs
platform-specific syscalls and there is no verified trash mechanism there. Since this
collection's Windows machine is retired, an untested deletion path was left unwritten
rather than shipped unverified.

### Re-running after trimming more tracks

1. Remove tracks from the rekordbox library.
2. Export the collection to `~/DJ/rekordbox.xml`.
3. `cd ~/DJ && dj prune`.

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
dj prune --report ~/DJ/prune-$(date +%Y%m%d-%H%M).txt
```

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

## Design

See `docs/superpowers/specs/` — the specs record the measured state of the collection and
the filesystem quirks (APFS case- and normalization-insensitivity, rekordbox's mixed
NFC/NFD output) that the implementation depends on.
