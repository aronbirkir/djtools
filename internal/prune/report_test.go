package prune

import (
	"fmt"
	"strings"
	"testing"
)

func reportPlan() *Plan {
	return &Plan{
		MusicDir:      "/Users/aron/DJ/music",
		Keepers:       3,
		Orphans:       []string{"/m/Dance/a.mp3", "/m/Dance/b.mp3", "/m/Pop/c.mp3"},
		OrphanSize:    1500,
		DeadLinks:     []string{"/m/A2026-05/gone.mp3"},
		Stale:         []string{"e:/music/x.mp3"},
		Leftovers:     []string{"/m/Dance/cover.jpg"},
		CaseDupes:     [][]string{{"/m/Pop/A.mp3", "/m/Pop/a.mp3"}},
		FolderTotals:  map[string]int{"Dance": 4, "Pop": 2},
		FolderOrphans: map[string]int{"Dance": 2, "Pop": 1},
		FolderBytes:   map[string]int64{"Dance": 1000, "Pop": 500},
	}
}

func TestSummaryGroupsByFolderSortedByOrphanCount(t *testing.T) {
	var b strings.Builder
	if err := Summary(&b, reportPlan(), nil); err != nil {
		t.Fatalf("Summary: %v", err)
	}
	out := b.String()

	dance, pop := strings.Index(out, "Dance"), strings.Index(out, "Pop")
	if dance < 0 || pop < 0 {
		t.Fatalf("summary is missing folder rows:\n%s", out)
	}
	if dance > pop {
		t.Errorf("Dance (2 orphans) must sort above Pop (1):\n%s", out)
	}
	for _, want := range []string{"TOTAL", "3", "6"} {
		if !strings.Contains(out, want) {
			t.Errorf("summary missing %q:\n%s", want, out)
		}
	}
}

// Findings are grouped three ways: aborts first because they are why the run
// stopped, then warnings about this run, then advisories that describe the
// library and repeat every run until the user edits rekordbox.
func TestSummaryIncludesFindings(t *testing.T) {
	findings := []Finding{
		{Code: CodeDeadLinks, Level: LevelWarn,
			Message: "4 library entries point at files that no longer exist"},
		{Code: CodeRecentlyAdded, Level: LevelWarn,
			Message: "7 audio files are newer than this export and were skipped"},
		{Code: CodeTruncatedExport, Level: LevelAbort,
			Message: "truncated export: declares 8617 but parsed 400"},
	}
	var b strings.Builder
	if err := Summary(&b, reportPlan(), findings); err != nil {
		t.Fatalf("Summary: %v", err)
	}
	out := b.String()

	if !strings.Contains(out, "ABORT") || !strings.Contains(out, "truncated export") {
		t.Errorf("summary missing the abort:\n%s", out)
	}
	if !strings.Contains(out, "WARN") || !strings.Contains(out, "newer than this export") {
		t.Errorf("summary missing the run warning:\n%s", out)
	}
	if !strings.Contains(out, "To clean up in rekordbox") {
		t.Errorf("summary missing the advisory heading:\n%s", out)
	}
	if !strings.Contains(out, "no longer exist") {
		t.Errorf("summary missing the advisory itself:\n%s", out)
	}

	// The advisory must sit under its own heading, not among the run warnings.
	heading := strings.Index(out, "To clean up in rekordbox")
	if dead := strings.Index(out, "no longer exist"); dead < heading {
		t.Errorf("dead-links advisory appears above its heading:\n%s", out)
	}
	// And the abort must precede everything.
	if strings.Index(out, "ABORT") > strings.Index(out, "WARN") {
		t.Errorf("abort should print before warnings:\n%s", out)
	}
}

func TestSummaryEmptyPlanSaysNothingToDo(t *testing.T) {
	p := &Plan{
		MusicDir:      "/m",
		Keepers:       10,
		FolderTotals:  map[string]int{},
		FolderOrphans: map[string]int{},
		FolderBytes:   map[string]int64{},
	}
	var b strings.Builder
	if err := Summary(&b, p, nil); err != nil {
		t.Fatalf("Summary: %v", err)
	}
	if out := b.String(); !strings.Contains(out, "nothing to remove") {
		t.Errorf("summary should say nothing to remove:\n%s", out)
	}
}

func TestReportListsEverySection(t *testing.T) {
	var b strings.Builder
	if err := Report(&b, reportPlan()); err != nil {
		t.Fatalf("Report: %v", err)
	}
	out := b.String()
	for _, want := range []string{
		"# ORPHANS (3)", "/m/Dance/a.mp3",
		"# DEAD LINKS (1)", "/m/A2026-05/gone.mp3",
		"# STALE ENTRIES (1)", "e:/music/x.mp3",
		"# LEFTOVERS (1)", "/m/Dance/cover.jpg",
		"# CASE DUPLICATES (1)", "/m/Pop/A.mp3",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("report missing %q:\n%s", want, out)
		}
	}
}

// The real collection has 110 folders holding orphans, most contributing a
// handful of files. Printing all of them buries the folders that matter, so the
// tail collapses into a single row that still carries its own totals.
func TestSummaryCollapsesLongFolderTail(t *testing.T) {
	p := &Plan{
		MusicDir:      "/m",
		FolderTotals:  map[string]int{},
		FolderOrphans: map[string]int{},
		FolderBytes:   map[string]int64{},
	}
	// Five more folders than the cap, each with a distinct orphan count so the
	// ordering is unambiguous: f00 is largest, f29 smallest.
	const extra = 5
	for i := 0; i < maxFolderRows+extra; i++ {
		name := fmt.Sprintf("f%02d", i)
		n := maxFolderRows + extra - i
		p.FolderOrphans[name] = n
		p.FolderTotals[name] = n
		p.FolderBytes[name] = int64(n) * 1000
		p.OrphanSize += int64(n) * 1000
		for j := 0; j < n; j++ {
			p.Orphans = append(p.Orphans, name+"/x.mp3")
		}
	}

	var b strings.Builder
	if err := Summary(&b, p, nil); err != nil {
		t.Fatalf("Summary: %v", err)
	}
	out := b.String()

	if !strings.Contains(out, fmt.Sprintf("(+%d more folders)", extra)) {
		t.Errorf("expected the tail collapsed into one row:\n%s", out)
	}
	if !strings.Contains(out, "f00") {
		t.Errorf("the largest folder must still be listed:\n%s", out)
	}
	if strings.Contains(out, "f29") {
		t.Errorf("the smallest folder should be collapsed, not listed:\n%s", out)
	}
	// The collapsed row must not swallow anything: its orphan count plus the
	// listed rows' must equal the total.
	if !strings.Contains(out, "TOTAL") {
		t.Errorf("missing TOTAL row:\n%s", out)
	}
}

// A table with fewer folders than the cap must not print a collapsed row at all.
func TestSummaryNoCollapseRowWhenShort(t *testing.T) {
	var b strings.Builder
	if err := Summary(&b, reportPlan(), nil); err != nil {
		t.Fatalf("Summary: %v", err)
	}
	if out := b.String(); strings.Contains(out, "more folders") {
		t.Errorf("unexpected collapsed row for a two-folder plan:\n%s", out)
	}
}

func TestHumanBytes(t *testing.T) {
	for _, tt := range []struct {
		in   int64
		want string
	}{
		{0, "0 B"},
		{999, "999 B"},
		{1500, "1.5 kB"},
		{35_900_000_000, "35.9 GB"},
	} {
		if got := HumanBytes(tt.in); got != tt.want {
			t.Errorf("HumanBytes(%d) = %q, want %q", tt.in, got, tt.want)
		}
	}
}
