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

Run from the collection directory so the default paths resolve:

```sh
cd ~/DJ
dj prune --dry-run          # scan and report, change nothing
dj prune                    # summary, confirm, then move orphans to Trash
```

## Design

See `docs/superpowers/specs/` — the specs record the measured state of the collection and
the filesystem quirks (APFS case- and normalization-insensitivity, rekordbox's mixed
NFC/NFD output) that the implementation depends on.
