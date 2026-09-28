package main

import (
	"fmt"
	"image/color"
	"strings"

	"gioui.org/font"
	"gioui.org/layout"
	"gioui.org/text"
	"gioui.org/widget"
	"gioui.org/widget/material"

	"github.com/aronbirkir/djtools/internal/prune"
)

// Layout of the Rekordbox Prune screen. The widgets and what they may do live
// in prune_view.go; the rules in prune_state.go.

func (v *pruneView) Layout(gtx C, th *material.Theme) D {
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx C) D { return v.layoutHeader(gtx, th) }),
		vspace(18),
		layout.Rigid(func(gtx C) D { return v.layoutInputs(gtx, th) }),
		vspace(16),
		layout.Flexed(1, func(gtx C) D { return v.layoutResults(gtx, th) }),
		layout.Rigid(func(gtx C) D {
			msg, c := v.status()
			if msg == "" {
				return D{}
			}
			return layout.Inset{Top: 10}.Layout(gtx, func(gtx C) D {
				l := material.Body2(th, msg)
				l.Color = c
				l.MaxLines = 3
				return l.Layout(gtx)
			})
		}),
	)
}

func (v *pruneView) layoutHeader(gtx C, th *material.Theme) D {
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx C) D {
			l := material.H5(th, "Rekordbox Prune")
			l.Font.Weight = font.Bold
			return l.Layout(gtx)
		}),
		vspace(2),
		layout.Rigid(label(th, "Move audio files that are no longer in your rekordbox library to the Trash.", pal.Muted)),
	)
}

func (v *pruneView) layoutInputs(gtx C, th *material.Theme) D {
	if v.state.busy() {
		gtx = gtx.Disabled()
	}
	scan := func(gtx C) D {
		text := "Scan"
		if v.state.phase == phaseScanning {
			text = "Scanning…"
		}
		return button(th, &v.scanBtn, text, true)(gtx)
	}
	return card(gtx, func(gtx C) D {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(fieldLabel(th, "rekordbox XML export")),
			vspace(6),
			layout.Rigid(func(gtx C) D {
				return v.pathRow(gtx, th, &v.xml, &v.browseXML, "~/Library/Pioneer/rekordbox/rekordbox.xml", nil)
			}),
			vspace(14),
			layout.Rigid(fieldLabel(th, "Music folder")),
			vspace(6),
			layout.Rigid(func(gtx C) D {
				return v.pathRow(gtx, th, &v.music, &v.browseMusic, "~/Music/rekordbox", scan)
			}),
			vspace(10),
			layout.Rigid(func(gtx C) D { return v.layoutAdvanced(gtx, th) }),
		)
	})
}

func (v *pruneView) pathRow(gtx C, th *material.Theme, ed *widget.Editor, browse *widget.Clickable, hint string, extra layout.Widget) D {
	children := []layout.FlexChild{
		layout.Flexed(1, func(gtx C) D { return input(gtx, th, ed, hint) }),
		hspace(8),
		layout.Rigid(func(gtx C) D {
			if v.browsing {
				gtx = gtx.Disabled()
			}
			return button(th, browse, "Browse…", false)(gtx)
		}),
	}
	if extra != nil {
		children = append(children, hspace(8), layout.Rigid(extra))
	}
	return layout.Flex{Alignment: layout.Middle}.Layout(gtx, children...)
}

func (v *pruneView) layoutAdvanced(gtx C, th *material.Theme) D {
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx C) D {
			text := "Show advanced options"
			if v.showAdvanced {
				text = "Hide advanced options"
			}
			return material.Clickable(gtx, &v.advancedBtn, func(gtx C) D {
				return layout.Inset{Top: 4, Bottom: 4}.Layout(gtx, label(th, text, pal.Accent))
			})
		}),
		layout.Rigid(func(gtx C) D {
			if !v.showAdvanced {
				return D{}
			}
			return layout.Inset{Top: 10}.Layout(gtx, func(gtx C) D {
				return layout.Flex{Alignment: layout.End}.Layout(gtx,
					layout.Flexed(2, labeled(th, "Audio extensions", func(gtx C) D {
						return input(gtx, th, &v.exts, prune.DefaultExtensions)
					})),
					hspace(12),
					layout.Flexed(1, labeled(th, "Max orphan %", func(gtx C) D {
						return input(gtx, th, &v.maxPct, "60")
					})),
					hspace(12),
					layout.Rigid(func(gtx C) D {
						cb := material.CheckBox(th, &v.keepDirs, "Keep empty folders")
						cb.Color, cb.IconColor = pal.Fg, pal.Accent
						return layout.Inset{Bottom: 8}.Layout(gtx, cb.Layout)
					}),
				)
			})
		}),
	)
}

func labeled(th *material.Theme, title string, w layout.Widget) layout.Widget {
	return func(gtx C) D {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(fieldLabel(th, title)),
			vspace(6),
			layout.Rigid(w),
		)
	}
}

func (v *pruneView) layoutResults(gtx C, th *material.Theme) D {
	gtx.Constraints.Min = gtx.Constraints.Max
	return card(gtx, func(gtx C) D {
		gtx.Constraints.Min = gtx.Constraints.Max
		switch v.state.phase {
		case phaseScanned, phaseConfirming, phaseTrashing:
			return v.layoutPlan(gtx, th)
		case phaseDone:
			return v.layoutDone(gtx, th)
		case phaseScanning:
			return centered(gtx, th, "Scanning…")
		}
		return centered(gtx, th, "Press Scan to compare the music folder with the rekordbox export.\nNothing is moved until you confirm.")
	})
}

func (v *pruneView) layoutPlan(gtx C, th *material.Theme) D {
	p := v.state.result.Plan
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx C) D {
			return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
				layout.Flexed(1, func(gtx C) D {
					l := material.H6(th, summaryLine(p))
					l.Font.Weight = font.SemiBold
					return l.Layout(gtx)
				}),
				hspace(12),
				layout.Rigid(func(gtx C) D {
					text := "Move to Trash"
					if n := len(p.Orphans); n > 0 {
						text = fmt.Sprintf("Move %d files to Trash", n)
					}
					if !v.state.canTrash() {
						gtx = gtx.Disabled()
					}
					return button(th, &v.trashBtn, text, true)(gtx)
				}),
			)
		}),
		layout.Rigid(func(gtx C) D { return v.layoutFindings(gtx, th) }),
		layout.Rigid(func(gtx C) D { return v.layoutOverride(gtx, th) }),
		layout.Rigid(func(gtx C) D {
			line := folderLine(p, 5)
			if line == "" {
				return D{}
			}
			return layout.Inset{Top: 10}.Layout(gtx, func(gtx C) D {
				l := material.Body2(th, line)
				l.Color = pal.Muted
				l.MaxLines = 2
				return l.Layout(gtx)
			})
		}),
		vspace(14),
		layout.Rigid(func(gtx C) D {
			labels := []string{fmt.Sprintf("Orphans (%d)", len(p.Orphans)), "Details"}
			return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
				layout.Rigid(func(gtx C) D { return segmented(gtx, th, v.tabs[:], labels, v.tab) }),
				hspace(12),
				layout.Flexed(1, func(gtx C) D {
					if v.tab != tabOrphans {
						return D{}
					}
					return input(gtx, th, &v.filter, "Filter orphans")
				}),
			)
		}),
		vspace(10),
		layout.Rigid(divider),
		layout.Flexed(1, func(gtx C) D { return v.layoutList(gtx, th) }),
	)
}

func (v *pruneView) layoutFindings(gtx C, th *material.Theme) D {
	lines := findingLines(v.state.result.Findings, maxFindingLines)
	children := make([]layout.FlexChild, len(lines))
	for i, fl := range lines {
		c, prefix := pal.Warn, "Warning: "
		switch {
		case fl.more:
			c, prefix = pal.Muted, ""
		case fl.abort:
			c, prefix = pal.Error, "Stops the run: "
		}
		children[i] = layout.Rigid(func(gtx C) D {
			return layout.Inset{Top: 6}.Layout(gtx, label(th, prefix+fl.text, c))
		})
	}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
}

func (v *pruneView) layoutOverride(gtx C, th *material.Theme) D {
	s := &v.state
	var w layout.Widget
	switch {
	case len(s.result.Blocked()) > 0:
		w = label(th, "These findings cannot be overridden. Fix them in rekordbox, export again and rescan.", pal.Error)
	case s.overrideRefusedForHome():
		w = label(th, "Overrides are not offered for ~/Music or a folder containing it. Point Music at your rekordbox folder.", pal.Error)
	case s.showOverride():
		cb := material.CheckBox(th, &v.override, "I understand. Override these findings.")
		cb.Color, cb.IconColor = pal.Fg, pal.Error
		w = cb.Layout
	default:
		return D{}
	}
	return layout.Inset{Top: 10}.Layout(gtx, w)
}

func (v *pruneView) layoutList(gtx C, th *material.Theme) D {
	p := v.state.result.Plan
	if v.tab == tabDetails {
		if len(v.details) == 0 {
			return centered(gtx, th, "Nothing else to report.")
		}
		return material.List(th, &v.list).Layout(gtx, len(v.details), func(gtx C, i int) D {
			row := v.details[i]
			if row.heading {
				return layout.Inset{Top: 12, Bottom: 4, Left: 10}.Layout(gtx, fieldLabel(th, row.text))
			}
			return listRow(gtx, th, i, row.text, pal.Fg, 1)
		})
	}
	if len(p.Orphans) == 0 {
		return centered(gtx, th, "Nothing to remove.")
	}
	if len(v.filtered) == 0 {
		return centered(gtx, th, "No orphans match the filter.")
	}
	return material.List(th, &v.list).Layout(gtx, len(v.filtered), func(gtx C, i int) D {
		return listRow(gtx, th, i, relPath(p.MusicDir, p.Orphans[v.filtered[i]]), pal.Fg, 1)
	})
}

func (v *pruneView) layoutDone(gtx C, th *material.Theme) D {
	ar := v.state.applied
	var errs []string
	for _, err := range ar.TrashErrs {
		errs = append(errs, err.Error())
	}
	for _, err := range ar.DirErrs {
		errs = append(errs, err.Error())
	}
	head := pal.Success
	if len(ar.TrashErrs) > 0 {
		head = pal.Error
	}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx C) D {
			l := material.H6(th, fmt.Sprintf("Moved %d of %d files to the Trash.", ar.Moved, ar.Total))
			l.Color = head
			l.Font.Weight = font.SemiBold
			return l.Layout(gtx)
		}),
		layout.Rigid(func(gtx C) D {
			if v.state.result.Options.KeepEmptyDirs {
				return D{}
			}
			return layout.Inset{Top: 4}.Layout(gtx,
				label(th, fmt.Sprintf("Removed %d empty folders.", len(ar.RemovedDirs)), pal.Fg))
		}),
		vspace(14),
		layout.Rigid(func(gtx C) D {
			return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
				layout.Flexed(1, label(th, "Report: "+ar.ReportPath, pal.Fg)),
				hspace(8),
				layout.Rigid(button(th, &v.finderBtn, "Show in Finder", false)),
			)
		}),
		vspace(8),
		layout.Rigid(label(th, restoreHint, pal.Muted)),
		vspace(14),
		layout.Flexed(1, func(gtx C) D {
			if len(errs) == 0 {
				return D{}
			}
			return material.List(th, &v.list).Layout(gtx, len(errs), func(gtx C, i int) D {
				return listRow(gtx, th, i, errs[i], pal.Error, 0)
			})
		}),
	)
}

// layoutModal draws the confirmation dialog over the whole window. It is drawn
// last, so it is on top for both painting and pointer input.
func (v *pruneView) layoutModal(gtx C, th *material.Theme) D {
	if v.state.phase != phaseConfirming {
		return D{}
	}
	p := v.state.result.Plan
	return modal(gtx, &v.scrim, &v.sink, func(gtx C) D {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(func(gtx C) D {
				l := material.H6(th, fmt.Sprintf("Move %d files to the Trash?", len(p.Orphans)))
				l.Font.Weight = font.SemiBold
				return l.Layout(gtx)
			}),
			vspace(10),
			layout.Rigid(label(th, fmt.Sprintf("%s will be moved out of %s.",
				prune.HumanBytes(p.OrphanSize), p.MusicDir), pal.Fg)),
			vspace(8),
			layout.Rigid(label(th, "A report listing every file is saved first, to:\n"+v.reportPath, pal.Muted)),
			layout.Rigid(func(gtx C) D {
				if strings.TrimSpace(v.filter.Text()) == "" {
					return D{}
				}
				return layout.Inset{Top: 8}.Layout(gtx, label(th, fmt.Sprintf(
					"The filter only narrows the list; all %d files will be moved.", len(p.Orphans)), pal.Muted))
			}),
			layout.Rigid(func(gtx C) D {
				if !v.state.force() {
					return D{}
				}
				return layout.Inset{Top: 10}.Layout(gtx, label(th,
					"You are overriding: "+prune.FindingMessages(v.state.result.Forceable()), pal.Error))
			}),
			vspace(18),
			layout.Rigid(func(gtx C) D {
				return layout.Flex{}.Layout(gtx,
					layout.Flexed(1, layout.Spacer{}.Layout),
					layout.Rigid(button(th, &v.cancelBtn, "Cancel", false)),
					hspace(8),
					layout.Rigid(func(gtx C) D {
						b := material.Button(th, &v.confirmBtn, "Move to Trash")
						styleButton(&b)
						b.Background = pal.Error
						return b.Layout(gtx)
					}),
				)
			}),
		)
	})
}

func centered(gtx C, th *material.Theme, msg string) D {
	return layout.Center.Layout(gtx, func(gtx C) D {
		l := material.Body1(th, msg)
		l.Color = pal.Muted
		l.Alignment = text.Middle
		return l.Layout(gtx)
	})
}

// listRow draws one striped row. maxLines 0 lets the text wrap.
func listRow(gtx C, th *material.Theme, i int, s string, c color.NRGBA, maxLines int) D {
	bg := pal.Surface
	if i%2 == 1 {
		bg = pal.Surface2
	}
	return layout.Background{}.Layout(gtx, rounded(bg, 6), func(gtx C) D {
		return layout.Inset{Top: 7, Bottom: 7, Left: 10, Right: 10}.Layout(gtx, func(gtx C) D {
			gtx.Constraints.Min.X = gtx.Constraints.Max.X
			l := material.Body2(th, s)
			l.Color = c
			l.MaxLines = maxLines
			return l.Layout(gtx)
		})
	})
}
