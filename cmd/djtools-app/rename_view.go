package main

import (
	"errors"
	"fmt"
	"image/color"
	"log"
	"path/filepath"
	"strings"
	"sync"

	"gioui.org/app"
	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/widget"
	"github.com/ncruces/zenity"

	"github.com/aronbirkir/djtools/internal/rename"
)

const (
	tabRenames = iota
	tabSkipped
)

// renameView is the MP3 Rename screen: its widgets, plus the renameState
// that decides what they may do.
type renameView struct {
	window *app.Window
	home   string
	save   func()
	state  renameState

	folder, pattern                    widget.Editor
	browseBtn, scanBtn, renameBtn      widget.Clickable
	confirmBtn, cancelBtn, scrim, sink widget.Clickable
	tabs                               [2]widget.Clickable
	tab                                int
	list                               widget.List

	done        chan func()
	browsing    bool
	focusCancel bool
	// wg counts rename runs in progress, so closing the window waits for
	// Apply rather than stopping it part way through.
	wg sync.WaitGroup
}

func newRenameView(w *app.Window, cfg Config, home string, save func()) *renameView {
	v := &renameView{
		window:  w,
		home:    home,
		save:    save,
		folder:  widget.Editor{SingleLine: true, Submit: true},
		pattern: widget.Editor{SingleLine: true, Submit: true},
		list:    widget.List{List: layout.List{Axis: layout.Vertical}},
		done:    make(chan func(), 4),
	}
	v.folder.SetText(cfg.RenameFolder)
	v.pattern.SetText(cfg.RenamePattern)
	v.state.setPattern(cfg.RenamePattern)
	return v
}

func (v *renameView) fillConfig(cfg *Config) {
	cfg.RenameFolder = strings.TrimSpace(v.folder.Text())
	cfg.RenamePattern = v.pattern.Text()
}

// drain runs results that worker goroutines sent back; see drainFuncs.
func (v *renameView) drain() {
	drainFuncs(v.done)
}

func (v *renameView) update(gtx C) {
	v.drain()

	// As in pruneView.update: disabled layout does not stop events read here,
	// so inputs are skipped entirely while busy.
	if !v.state.busy() {
		for {
			ev, ok := v.folder.Update(gtx)
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
		for {
			ev, ok := v.pattern.Update(gtx)
			if !ok {
				break
			}
			switch ev.(type) {
			case widget.ChangeEvent:
				v.state.setPattern(v.pattern.Text())
				v.save()
			case widget.SubmitEvent:
				v.scan()
			}
		}
		if v.browseBtn.Clicked(gtx) && !v.browsing {
			v.browse()
		}
	}
	if v.scanBtn.Clicked(gtx) {
		v.scan()
	}
	for i := range v.tabs {
		if v.tabs[i].Clicked(gtx) && v.tab != i {
			v.tab = i
			v.list.Position = layout.Position{}
		}
	}
	if v.focusCancel {
		v.focusCancel = false
		if v.state.phase == renameConfirming {
			gtx.Execute(key.FocusCmd{Tag: &v.cancelBtn})
		}
	}
	if v.renameBtn.Clicked(gtx) && v.state.canRename() {
		v.state.askConfirm()
		v.focusCancel = true
		v.window.Invalidate()
	}
	if v.cancelBtn.Clicked(gtx) || v.scrim.Clicked(gtx) {
		v.state.cancelConfirm()
	}
	v.sink.Clicked(gtx) // swallowed on purpose
	if v.confirmBtn.Clicked(gtx) {
		v.apply()
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
}

func (v *renameView) scan() {
	if !v.state.canScan() {
		return
	}
	dir, err := renameFolder(v.folder.Text(), v.home)
	if err != nil {
		v.state.fail(err)
		return
	}
	p := v.state.pattern
	if !v.state.startScan() {
		return
	}
	v.save()
	go func() {
		plan, err := safely(func() (*rename.RenamePlan, error) { return rename.Plan(dir, p) })
		v.done <- func() {
			v.state.scanDone(plan, err)
			v.list.Position = layout.Position{}
		}
		v.window.Invalidate()
	}()
}

func (v *renameView) apply() {
	plan, ok := v.state.startApply()
	if !ok {
		return
	}
	v.wg.Add(1)
	go func() {
		res, err := safely(func() (*rename.ApplyResult, error) { return rename.Apply(plan), nil })
		v.wg.Done()
		v.done <- func() {
			if err != nil {
				// No automatic rescan here: applyPanicked's message tells
				// the user to rescan themselves, which would be pointless
				// if a rescan had already run and overwritten it before
				// they could read it.
				v.state.applyPanicked(err)
				return
			}
			v.state.applyDone(res)
			v.scan()
		}
		v.window.Invalidate()
	}()
}

func (v *renameView) browse() {
	v.browsing = true
	start := expandHome(v.folder.Text(), v.home)
	go func() {
		opts := []zenity.Option{zenity.Directory(), zenity.Title("Choose a folder of MP3 files")}
		if start != "" {
			opts = append(opts, zenity.Filename(start+string(filepath.Separator)))
		}
		dir, err := zenity.SelectFile(opts...)
		if err != nil && !errors.Is(err, zenity.ErrCanceled) {
			log.Printf("folder dialog: %v", err)
		}
		v.done <- func() {
			v.browsing = false
			if dir != "" && !v.state.busy() {
				v.folder.SetText(dir)
				v.state.inputsChanged()
				v.save()
				v.scan()
			}
		}
		v.window.Invalidate()
	}()
}

func (v *renameView) status() (string, color.NRGBA) {
	s := &v.state
	switch s.phase {
	case renameScanning:
		return "Reading tags…", pal.Muted
	case renameApplying:
		return "Renaming…", pal.Muted
	case renameError:
		// lastApplied can still be set here: applying succeeded, but the
		// automatic rescan straight after it failed. Losing the apply
		// result from the screen would make a successful rename look like
		// it never happened.
		if a := s.lastApplied; a != nil {
			return fmt.Sprintf("Renamed %d files. Rescan failed: %v", a.Renamed, s.err), pal.Error
		}
		return s.err.Error(), pal.Error
	}
	if a := s.lastApplied; a != nil {
		msg := fmt.Sprintf("Renamed %d files.", a.Renamed)
		if a.Checked > 0 {
			msg += fmt.Sprintf(" %d were renamed after a check rather than atomically (e.g. on exFAT, or case-only renames on HFS+).", a.Checked)
		}
		if len(a.Failures) == 0 {
			return msg, pal.Success
		}
		var fails []string
		for _, f := range a.Failures {
			fails = append(fails, f.Name+": "+f.Reason)
		}
		return fmt.Sprintf("%s %d failed: %s", msg, len(a.Failures), strings.Join(fails, "; ")), pal.Error
	}
	if p := s.plan; p != nil {
		if len(p.Renames) == 0 {
			return fmt.Sprintf("Nothing to rename: %d already match, %d skipped.", p.Unchanged, len(p.Skipped)), pal.Muted
		}
		return fmt.Sprintf("%d to rename, %d skipped, %d already match.", len(p.Renames), len(p.Skipped), p.Unchanged), pal.Muted
	}
	return "", pal.Muted
}
