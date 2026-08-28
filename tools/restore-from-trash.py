#!/usr/bin/env python3
"""Restore files from the macOS Trash back into the DJ collection.

Finder's "Put Back" is unavailable for files moved by a command-line tool, so
this reconstructs each file's original folder instead. It builds a map of
filename -> original path from every source that records where a file lived:

  - the current rekordbox export
  - any older export still on disk
  - a saved `dj prune --report` file, which lists orphan paths verbatim

Dry-run by default. Nothing moves without --apply.

Usage:
    restore-from-trash.py --trash-list /tmp/run3-trash.txt
    restore-from-trash.py --trash-list /tmp/run3-trash.txt --apply

    # restrict to particular folders
    restore-from-trash.py --trash-list ... --only "A2025-12" --only Jazz
"""

import argparse
import os
import re
import shutil
import sys
import urllib.parse
from collections import defaultdict

MUSIC = "/Users/aron/DJ/music"
DJ = "/Users/aron/DJ"


def paths_from_xml(path):
    """Every music/ path a rekordbox export references."""
    try:
        with open(path, encoding="utf-8", errors="replace") as f:
            xml = f.read()
    except OSError:
        return set()
    out = set()
    for loc in re.findall(r'Location="([^"]*)"', xml):
        if not loc.startswith("file://localhost"):
            continue
        p = urllib.parse.unquote(loc[len("file://localhost"):])
        if p.startswith(MUSIC + "/"):
            out.add(p)
    return out


def paths_from_report(path):
    """Every music/ path a saved `dj prune --report` file lists."""
    try:
        with open(path, encoding="utf-8") as f:
            return {l.strip() for l in f if l.startswith(MUSIC + "/")}
    except OSError:
        return set()


def build_map(sources):
    """filename -> set of original paths, plus a count of what each source gave."""
    by_name = defaultdict(set)
    stats = []
    for label, paths in sources:
        stats.append((label, len(paths)))
        for p in paths:
            by_name[os.path.basename(p)].add(p)
    return by_name, stats


def main():
    ap = argparse.ArgumentParser(description=__doc__,
                                 formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--trash-list", required=True,
                    help="file listing the Trash paths to restore, one per line")
    ap.add_argument("--apply", action="store_true",
                    help="actually move files (default is a dry run)")
    ap.add_argument("--only", action="append", default=[],
                    help="restore only files whose original top-level folder matches "
                         "this (repeatable, case-insensitive substring)")
    ap.add_argument("--report", help="write the full resolution table to this file")
    args = ap.parse_args()

    by_name, stats = build_map([
        ("current export", paths_from_xml(os.path.join(DJ, "rekordbox.xml"))),
        ("rekordbox.old.xml", paths_from_xml(os.path.join(DJ, "rekordbox.old.xml"))),
        ("saved prune report", paths_from_report("/tmp/prune-report.txt")),
    ])

    print("Original-path sources:")
    for label, n in stats:
        print(f"  {label:22s} {n:6d} paths")
    print(f"  {'distinct filenames':22s} {len(by_name):6d}")
    print()

    try:
        with open(args.trash_list, encoding="utf-8") as f:
            trashed = [l.rstrip("\n") for l in f if l.strip()]
    except OSError as e:
        sys.exit(f"cannot read --trash-list: {e}")

    resolved, ambiguous, unknown, occupied, filtered = [], [], [], [], []

    for src in trashed:
        if not os.path.exists(src):
            unknown.append((src, "not present in the Trash"))
            continue
        if os.path.isdir(src):
            unknown.append((src, "is a directory; restore it by hand"))
            continue

        name = os.path.basename(src)
        candidates = by_name.get(name, set())

        if not candidates:
            unknown.append((src, "no original path known for this filename"))
            continue
        if len(candidates) > 1:
            ambiguous.append((src, sorted(candidates)))
            continue

        dest = next(iter(candidates))

        if args.only:
            rel = os.path.relpath(dest, MUSIC)
            top = rel.split(os.sep)[0]
            if not any(o.lower() in top.lower() for o in args.only):
                filtered.append((src, dest))
                continue

        if os.path.exists(dest):
            occupied.append((src, dest))
            continue

        resolved.append((src, dest))

    print(f"Trash entries considered:        {len(trashed)}")
    print(f"  restorable (one known origin): {len(resolved)}")
    print(f"  already present at origin:     {len(occupied)}  (skipped, nothing overwritten)")
    print(f"  ambiguous filename:            {len(ambiguous)}  (needs your choice)")
    print(f"  origin unknown:                {len(unknown)}")
    if args.only:
        print(f"  excluded by --only:            {len(filtered)}")
    print()

    if resolved:
        by_folder = defaultdict(int)
        for _, dest in resolved:
            by_folder[os.path.relpath(dest, MUSIC).split(os.sep)[0]] += 1
        print("Restorable, by original folder:")
        for folder, n in sorted(by_folder.items(), key=lambda kv: -kv[1])[:30]:
            print(f"  {n:5d}  {folder}")
        if len(by_folder) > 30:
            print(f"  ... and {len(by_folder) - 30} more folders")
        print()

    if ambiguous:
        print("Ambiguous (same filename in more than one folder) — not restored:")
        for src, cands in ambiguous[:10]:
            print(f"  {os.path.basename(src)}")
            for c in cands:
                print(f"      -> {os.path.relpath(c, MUSIC)}")
        if len(ambiguous) > 10:
            print(f"  ... and {len(ambiguous) - 10} more")
        print()

    if args.report:
        with open(args.report, "w", encoding="utf-8") as f:
            f.write(f"# RESTORABLE ({len(resolved)})\n")
            for src, dest in sorted(resolved, key=lambda x: x[1]):
                f.write(f"{dest}\n")
            f.write(f"\n# ALREADY PRESENT ({len(occupied)})\n")
            for src, dest in occupied:
                f.write(f"{dest}\n")
            f.write(f"\n# AMBIGUOUS ({len(ambiguous)})\n")
            for src, cands in ambiguous:
                f.write(f"{os.path.basename(src)} -> {' | '.join(cands)}\n")
            f.write(f"\n# UNKNOWN ORIGIN ({len(unknown)})\n")
            for src, why in unknown:
                f.write(f"{os.path.basename(src)}  ({why})\n")
        print(f"Full resolution table written to {args.report}")
        print()

    if not args.apply:
        print("Dry run. Nothing was moved. Re-run with --apply to restore.")
        return 0

    if not resolved:
        print("Nothing to restore.")
        return 0

    moved, failed = 0, []
    for src, dest in resolved:
        try:
            os.makedirs(os.path.dirname(dest), exist_ok=True)
            if os.path.exists(dest):        # re-check: never overwrite
                failed.append((src, dest, "appeared at destination during the run"))
                continue
            shutil.move(src, dest)
            moved += 1
        except OSError as e:
            failed.append((src, dest, str(e)))

    print(f"Restored {moved} of {len(resolved)} files.")
    for src, dest, why in failed:
        print(f"  FAILED {os.path.basename(src)}: {why}")
    return 2 if failed else 0


if __name__ == "__main__":
    sys.exit(main())
