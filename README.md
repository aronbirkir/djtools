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

Files go to the macOS Trash with their original locations recorded, so select
them in Finder and use **Put Back**. Nothing is unlinked directly.

## Design

See `docs/superpowers/specs/` — the specs record the measured state of the collection and
the filesystem quirks (APFS case- and normalization-insensitivity, rekordbox's mixed
NFC/NFD output) that the implementation depends on.
