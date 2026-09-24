package main

import (
	"errors"
	"fmt"
	"image/color"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"gioui.org/app"
	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/widget"
	"github.com/ncruces/zenity"

	"github.com/aronbirkir/djtools/internal/prune"
)

const (
	tabOrphans = iota
	tabDetails
)

const restoreHint = "Finder's Put Back does not work for files moved this way. To restore them, " +
	"run tools/restore-from-trash.py from the djtools repo (see \"Recovering a mistake\" in its README)."

// pruneView is the Rekordbox Prune screen: its widgets, plus the pruneState
// that decides what they may do.
type pruneView struct {
	window *app.Window
	home   string
	save   func()
	state  pruneState

	xml, music, exts, maxPct, filter widget.Editor
	keepDirs, override               widget.Bool

	browseXML, browseMusic, scanBtn, advancedBtn widget.Clickable
	trashBtn, confirmBtn, cancelBtn, finderBtn   widget.Clickable
	// scrim covers the window behind the confirmation dialog and cancels it
	// when clicked; sink covers the dialog itself so clicks on its body do not
	// fall through to the scrim.
	scrim, sink widget.Clickable
	tabs        [2]widget.Clickable

	showAdvanced bool
	tab          int
	filtered     []int
	details      []detailRow
	list         widget.List
	reportPath   string

	// done carries results from worker goroutines, which must not touch UI
	// state themselves; each func runs on the UI goroutine at the next frame.
	done     chan func()
	browsing bool

	// focusCancel asks the next update to move keyboard focus to the dialog's
	// Cancel button, which only exists once the dialog has been laid out.
	// With Cancel focused, Enter and Space cancel rather than confirm.
	focusCancel bool

	// wg counts trash runs in progress, so closing the window can wait for
	// prune.Apply to finish instead of killing it between batches.
	wg sync.WaitGroup
}

func newPruneView(w *app.Window, cfg Config, home string, save func()) *pruneView {
	v := &pruneView{
		window: w,
		home:   home,
		save:   save,
		state:  pruneState{musicHome: filepath.Join(home, "Music")},
		xml:    widget.Editor{SingleLine: true, Submit: true},
		music:  widget.Editor{SingleLine: true, Submit: true},
		exts:   widget.Editor{SingleLine: true, Submit: true},
		maxPct: widget.Editor{SingleLine: true, Submit: true},
		filter: widget.Editor{SingleLine: true},
		list:   widget.List{List: layout.List{Axis: layout.Vertical}},
		done:   make(chan func(), 4),
	}
	v.xml.SetText(cfg.XMLPath)
	v.music.SetText(cfg.MusicDir)
	v.exts.SetText(cfg.Extensions)
	v.maxPct.SetText(strconv.FormatFloat(cfg.MaxOrphanPct, 'f', -1, 64))
	v.keepDirs.Value = cfg.KeepEmptyDirs
	return v
}

// fillConfig copies the screen's settings into cfg for saving. An unparsable
// max orphan % keeps the previous value rather than saving garbage.
func (v *pruneView) fillConfig(cfg *Config) {
	cfg.XMLPath = strings.TrimSpace(v.xml.Text())
	cfg.MusicDir = strings.TrimSpace(v.music.Text())
	cfg.Extensions = strings.TrimSpace(v.exts.Text())
	if pct, err := strconv.ParseFloat(strings.TrimSpace(v.maxPct.Text()), 64); err == nil {
		cfg.MaxOrphanPct = pct
	}
	cfg.KeepEmptyDirs = v.keepDirs.Value
}

func (v *pruneView) inputs() pruneInputs {
	return pruneInputs{
		xml:           v.xml.Text(),
		music:         v.music.Text(),
		exts:          v.exts.Text(),
		maxPct:        v.maxPct.Text(),
		keepEmptyDirs: v.keepDirs.Value,
	}
}

func (v *pruneView) update(gtx C) {
drain:
	for {
		select {
		case f := <-v.done:
			f()
		default:
			break drain
		}
	}

	// gtx.Disabled() in layoutInputs only greys the inputs out: update reads
	// events with the enabled root gtx, so a focused editor would still take
	// typing mid-scan. Skipping Update while busy also drops the editors'
	// focus filters, so Gio unfocuses them until the work is done.
	inputs := []*widget.Editor{&v.xml, &v.music, &v.exts, &v.maxPct}
	if v.state.busy() {
		inputs = nil
	}
	for _, ed := range inputs {
		for {
			ev, ok := ed.Update(gtx)
			if !ok {
				break
			}
			switch ev.(type) {
			case widget.ChangeEvent:
				v.state.inputsChanged()
				v.save()
			case widget.SubmitEvent:
				v.scan()
			}
		}
	}
	for {
		ev, ok := v.filter.Update(gtx)
		if !ok {
			break
		}
		if _, ok := ev.(widget.ChangeEvent); ok {
			v.refreshList()
		}
	}
	if !v.state.busy() && v.keepDirs.Update(gtx) {
		v.state.inputsChanged()
		v.save()
	}
	if !v.state.busy() && v.override.Update(gtx) {
		v.state.override = v.override.Value
	}

	if v.browseXML.Clicked(gtx) && !v.browsing && !v.state.busy() {
		v.browse(&v.xml, false)
	}
	if v.browseMusic.Clicked(gtx) && !v.browsing && !v.state.busy() {
		v.browse(&v.music, true)
	}
	if v.scanBtn.Clicked(gtx) {
		v.scan()
	}
	if v.advancedBtn.Clicked(gtx) {
		v.showAdvanced = !v.showAdvanced
	}
	for i := range v.tabs {
		if v.tabs[i].Clicked(gtx) && v.tab != i {
			v.tab = i
			v.list.Position = layout.Position{}
		}
	}
	if v.focusCancel {
		v.focusCancel = false
		if v.state.phase == phaseConfirming {
			gtx.Execute(key.FocusCmd{Tag: &v.cancelBtn})
		}
	}
	if v.trashBtn.Clicked(gtx) {
		v.askConfirm()
	}
	if v.cancelBtn.Clicked(gtx) || v.scrim.Clicked(gtx) {
		v.state.cancelConfirm()
	}
	v.sink.Clicked(gtx) // swallowed on purpose
	if v.confirmBtn.Clicked(gtx) {
		v.trash()
	}
	if v.finderBtn.Clicked(gtx) {
		v.showReport()
	}
	for {
		ev, ok := gtx.Event(key.Filter{Name: key.NameEscape})
		if !ok {
			break
		}
		if e, ok := ev.(key.Event); ok && e.State == key.Press {
			v.state.cancelConfirm()
		}
	}

	// The state can clear the override (new scan, changed inputs); keep the
	// checkbox showing what the state will actually use.
	v.override.Value = v.state.override
}

func (v *pruneView) scan() {
	if v.state.busy() {
		return
	}
	opts, err := buildOptions(v.inputs(), v.home)
	if err != nil {
		v.state.fail(err)
		return
	}
	if !v.state.startScan() {
		return
	}
	v.save()
	go func() {
		r, err := safely(func() (*prune.Result, error) { return prune.Scan(opts) })
		v.done <- func() {
			v.state.scanDone(r, err)
			v.refreshList()
		}
		v.window.Invalidate()
	}()
}

// browse opens a native picker without blocking the UI.
func (v *pruneView) browse(ed *widget.Editor, dir bool) {
	v.browsing = true
	start := expandHome(ed.Text(), v.home)
	go func() {
		var opts []zenity.Option
		if dir {
			opts = append(opts, zenity.Directory(), zenity.Title("Choose the music folder"))
			if start != "" {
				opts = append(opts, zenity.Filename(start+string(filepath.Separator)))
			}
		} else {
			opts = append(opts, zenity.Title("Choose the rekordbox XML export"),
				zenity.FileFilters{{Name: "rekordbox XML", Patterns: []string{"*.xml"}}})
			if start != "" {
				opts = append(opts, zenity.Filename(start))
			}
		}
		path, err := zenity.SelectFile(opts...)
		if err != nil && !errors.Is(err, zenity.ErrCanceled) {
			log.Printf("file dialog: %v", err)
		}
		v.done <- func() {
			v.browsing = false
			if path != "" && !v.state.busy() {
				ed.SetText(path)
				v.state.inputsChanged()
				v.save()
			}
		}
		v.window.Invalidate()
	}()
}

// askConfirm fixes the report path before the dialog opens, so the dialog can
// show exactly where the report will be written.
func (v *pruneView) askConfirm() {
	if !v.state.canTrash() {
		return
	}
	dir, err := reportsDir()
	if err != nil {
		v.state.notice(fmt.Errorf("finding a place for the report: %w", err))
		return
	}
	v.reportPath = uniqueReportPath(dir, time.Now())
	v.state.askConfirm()
	v.focusCancel = true
	v.window.Invalidate()
}

func (v *pruneView) trash() {
	r, force, ok := v.state.startTrash()
	if !ok {
		return
	}
	path := v.reportPath
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		v.state.trashDone(nil, fmt.Errorf("creating the reports folder, so nothing was moved: %w", err))
		return
	}
	v.wg.Add(1)
	go func() {
		ar, err := safely(func() (*prune.ApplyResult, error) {
			return prune.Apply(r, prune.ExecRunner{}, force, path)
		})
		// Done before the send: once the window is gone nothing drains done,
		// and the exit path is waiting on wg.
		v.wg.Done()
		v.done <- func() {
			v.state.trashDone(ar, err)
			v.list.Position = layout.Position{}
		}
		v.window.Invalidate()
	}()
}

func (v *pruneView) showReport() {
	if v.state.applied == nil {
		return
	}
	path := v.state.applied.ReportPath
	go func() {
		if err := exec.Command("open", "-R", path).Run(); err != nil {
			log.Printf("show report: %v", err)
		}
	}()
}

func (v *pruneView) refreshList() {
	v.list.Position = layout.Position{}
	if v.state.result == nil {
		v.filtered, v.details = nil, nil
		return
	}
	p := v.state.result.Plan
	v.filtered = filterOrphans(p.Orphans, p.MusicDir, v.filter.Text())
	v.details = detailRows(p)
}

func (v *pruneView) status() (string, color.NRGBA) {
	s := &v.state
	switch s.phase {
	case phaseScanning:
		return "Reading the export and walking the music folder…", pal.Muted
	case phaseTrashing:
		return "Writing the report, then moving files to the Trash…", pal.Muted
	case phaseDone:
		if n := len(s.applied.TrashErrs); n > 0 {
			return fmt.Sprintf("%d trash batches failed; see the errors below. Scan again to see what is left.", n), pal.Error
		}
		return "Done.", pal.Success
	}
	if s.err != nil {
		return s.err.Error(), pal.Error
	}
	return "", pal.Muted
}
