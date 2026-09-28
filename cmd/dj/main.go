// Command dj maintains a rekordbox-backed DJ collection.
package main

import (
	"fmt"
	"io"
	"os"

	"github.com/aronbirkir/djtools/internal/prune"
	"github.com/aronbirkir/djtools/internal/rename"
)

func usage(w io.Writer) {
	fmt.Fprint(w, `dj maintains a rekordbox-backed DJ collection.

usage: dj <command> [flags]

commands:
  prune   move audio files that are no longer in the rekordbox library to the Trash
  rename  rename MP3 files from their ID3 tags

Run "dj <command> --help" for a command's flags.
`)
}

func main() {
	if len(os.Args) < 2 {
		usage(os.Stderr)
		os.Exit(1)
	}
	switch os.Args[1] {
	case "prune":
		os.Exit(prune.Run(os.Args[2:], os.Stdout, os.Stderr, os.Stdin))
	case "rename":
		os.Exit(rename.Run(os.Args[2:], os.Stdout, os.Stderr, os.Stdin))
	case "help", "-h", "--help":
		usage(os.Stdout)
	default:
		fmt.Fprintf(os.Stderr, "dj: unknown command %q\n\n", os.Args[1])
		usage(os.Stderr)
		os.Exit(1)
	}
}
