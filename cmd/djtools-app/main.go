// Command djtools-app is the desktop front end for djtools.
package main

import (
	"image/color"
	"log"
	"os"

	"gioui.org/app"
	"gioui.org/font"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
)

func main() {
	go func() {
		window := new(app.Window)
		window.Option(
			app.Title("djtools"),
			app.Size(unit.Dp(1100), unit.Dp(760)),
			app.MinSize(unit.Dp(820), unit.Dp(520)),
		)
		if err := run(window); err != nil {
			log.Fatal(err)
		}
		os.Exit(0)
	}()
	setAppIcon()
	app.Main()
}

type appUI struct {
	window       *app.Window
	cfg          Config
	themeMode    ThemeMode
	themeButtons [3]widget.Clickable
	sysTheme     systemTheme
	prune        *pruneView
}

func run(window *app.Window) error {
	th := newTheme()
	var ops op.Ops
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	ui := &appUI{window: window, cfg: loadConfig()}
	ui.themeMode = ui.cfg.Theme
	if ui.themeMode == "" {
		ui.themeMode = ThemeSystem
	}
	ui.prune = newPruneView(window, ui.cfg, home, ui.saveConfig)
	ui.sysTheme.watch(window)

	for {
		switch e := window.Event().(type) {
		case app.DestroyEvent:
			// Returning exits the process, which would stop prune.Apply
			// between batches; let a trash run finish first.
			if ui.prune.state.phase == phaseTrashing {
				log.Print("finishing the trash run before exiting")
			}
			ui.prune.wg.Wait()
			return e.Err
		case app.FrameEvent:
			gtx := app.NewContext(&ops, e)
			ui.update(gtx)
			applyTheme(th, ui.themeMode, ui.sysTheme.dark.Load())
			ui.Layout(gtx, th)
			e.Frame(gtx.Ops)
		}
	}
}

func (ui *appUI) saveConfig() {
	ui.prune.fillConfig(&ui.cfg)
	ui.cfg.Theme = ui.themeMode
	if err := saveConfig(ui.cfg); err != nil {
		log.Printf("saving config: %v", err)
	}
}

func (ui *appUI) update(gtx C) {
	for i := range ui.themeButtons {
		if ui.themeButtons[i].Clicked(gtx) && ui.themeMode != themeModes[i] {
			ui.themeMode = themeModes[i]
			ui.saveConfig()
		}
	}
	ui.prune.update(gtx)
}

func (ui *appUI) Layout(gtx C, th *material.Theme) D {
	fill(gtx, pal.Bg)
	gtx.Constraints.Min = gtx.Constraints.Max
	layout.Flex{}.Layout(gtx,
		layout.Rigid(func(gtx C) D { return ui.layoutSidebar(gtx, th) }),
		layout.Flexed(1, func(gtx C) D {
			return layout.UniformInset(unit.Dp(24)).Layout(gtx, func(gtx C) D {
				return ui.prune.Layout(gtx, th)
			})
		}),
	)
	ui.prune.layoutModal(gtx, th)
	return D{Size: gtx.Constraints.Max}
}

func (ui *appUI) layoutSidebar(gtx C, th *material.Theme) D {
	width := gtx.Dp(unit.Dp(232))
	gtx.Constraints.Min.X, gtx.Constraints.Max.X = width, width
	gtx.Constraints.Min.Y = gtx.Constraints.Max.Y
	return layout.Background{}.Layout(gtx, rounded(pal.Surface, 0), func(gtx C) D {
		return layout.UniformInset(unit.Dp(14)).Layout(gtx, func(gtx C) D {
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
				layout.Rigid(func(gtx C) D {
					l := material.H6(th, "djtools")
					l.Font.Weight = font.Bold
					return layout.Inset{Top: 6, Left: 10, Bottom: 18}.Layout(gtx, l.Layout)
				}),
				layout.Rigid(func(gtx C) D { return navItem(gtx, th, "Rekordbox Prune", true) }),
				layout.Flexed(1, layout.Spacer{}.Layout),
				layout.Rigid(func(gtx C) D {
					return segmented(gtx, th, ui.themeButtons[:], themeLabels(), themeIndex(ui.themeMode))
				}),
			)
		})
	})
}

// navItem is one tool in the sidebar. There is only one tool so far, so it is
// always selected and not yet clickable.
func navItem(gtx C, th *material.Theme, text string, selected bool) D {
	gtx.Constraints.Min.X = gtx.Constraints.Max.X
	bg := color.NRGBA{}
	if selected {
		bg = withAlpha(pal.Accent, 0x26)
	}
	return layout.Background{}.Layout(gtx, rounded(bg, 8), func(gtx C) D {
		return layout.Inset{Top: 10, Bottom: 8, Left: 10, Right: 10}.Layout(gtx, func(gtx C) D {
			l := material.Body1(th, text)
			l.Font.Weight = font.SemiBold
			if selected {
				l.Color = pal.Accent
			}
			return l.Layout(gtx)
		})
	})
}
