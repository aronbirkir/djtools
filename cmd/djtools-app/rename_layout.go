package main

import (
	"fmt"
	"image"
	"image/color"

	"gioui.org/font"
	"gioui.org/layout"
	"gioui.org/unit"
	"gioui.org/widget/material"

	"github.com/aronbirkir/djtools/internal/rename"
)

const renameTokenHint = "Tokens: {artist}  {title}  {bpm}  {key}   ·   zero-pad with {bpm:03}   ·   press Enter to rescan"

func (v *renameView) Layout(gtx C, th *material.Theme) D {
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx C) D {
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
				layout.Rigid(func(gtx C) D {
					l := material.H5(th, "MP3 Rename")
					l.Font.Weight = font.Bold
					return l.Layout(gtx)
				}),
				vspace(2),
				layout.Rigid(label(th, "Rename MP3 files from their ID3 tags, e.g. 123 04A Daft Punk - One More Time.mp3", pal.Muted)),
			)
		}),
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

func (v *renameView) layoutInputs(gtx C, th *material.Theme) D {
	if v.state.busy() {
		gtx = gtx.Disabled()
	}
	return card(gtx, func(gtx C) D {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(fieldLabel(th, "Music folder")),
			vspace(6),
			layout.Rigid(func(gtx C) D {
				return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
					layout.Flexed(1, func(gtx C) D { return input(gtx, th, &v.folder, "/path/to/music") }),
					hspace(8),
					layout.Rigid(func(gtx C) D {
						if v.browsing {
							gtx = gtx.Disabled()
						}
						return button(th, &v.browseBtn, "Browse…", false)(gtx)
					}),
					hspace(8),
					layout.Rigid(func(gtx C) D {
						text := "Scan"
						if v.state.phase == renameScanning {
							text = "Scanning…"
						}
						if !v.state.canScan() {
							gtx = gtx.Disabled()
						}
						return button(th, &v.scanBtn, text, true)(gtx)
					}),
				)
			}),
			vspace(14),
			layout.Rigid(fieldLabel(th, "Rename pattern")),
			vspace(6),
			layout.Rigid(func(gtx C) D { return input(gtx, th, &v.pattern, "{bpm:03} {key:03} {artist} - {title}") }),
			vspace(6),
			layout.Rigid(func(gtx C) D {
				if err := v.state.patternErr; err != nil {
					return label(th, err.Error(), pal.Error)(gtx)
				}
				l := material.Caption(th, renameTokenHint)
				l.Color = pal.Muted
				return l.Layout(gtx)
			}),
		)
	})
}

func (v *renameView) layoutResults(gtx C, th *material.Theme) D {
	gtx.Constraints.Min = gtx.Constraints.Max
	return card(gtx, func(gtx C) D {
		gtx.Constraints.Min = gtx.Constraints.Max
		p := v.state.plan
		if p == nil {
			msg := "Choose a folder and press Scan to preview the new names.\nNothing is renamed until you confirm."
			if v.state.phase == renameScanning {
				msg = "Reading tags…"
			}
			return centered(gtx, th, msg)
		}
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(func(gtx C) D {
				return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
					layout.Rigid(func(gtx C) D {
						labels := []string{fmt.Sprintf("To rename (%d)", len(p.Renames)), fmt.Sprintf("Skipped (%d)", len(p.Skipped))}
						return segmented(gtx, th, v.tabs[:], labels, v.tab)
					}),
					layout.Flexed(1, layout.Spacer{}.Layout),
					layout.Rigid(func(gtx C) D {
						text := "Rename files"
						if n := len(p.Renames); n > 0 {
							text = fmt.Sprintf("Rename %d files", n)
						}
						if !v.state.canRename() {
							gtx = gtx.Disabled()
						}
						return button(th, &v.renameBtn, text, true)(gtx)
					}),
				)
			}),
			vspace(12),
			layout.Rigid(func(gtx C) D {
				left, right := "Current name", "New name"
				if v.tab == tabSkipped {
					left, right = "File", "Reason"
				}
				return layout.Inset{Left: 10, Right: 10, Bottom: 8}.Layout(gtx, func(gtx C) D {
					return columnsRow(gtx, th, fieldLabel(th, left), fieldLabel(th, right), false)
				})
			}),
			layout.Rigid(divider),
			layout.Flexed(1, func(gtx C) D { return v.layoutList(gtx, th, p) }),
		)
	})
}

func (v *renameView) layoutList(gtx C, th *material.Theme, p *rename.RenamePlan) D {
	if v.tab == tabSkipped {
		if len(p.Skipped) == 0 {
			return centered(gtx, th, "Nothing was skipped.")
		}
		return material.List(th, &v.list).Layout(gtx, len(p.Skipped), func(gtx C, i int) D {
			s := p.Skipped[i]
			return stripedRow(gtx, i, func(gtx C) D {
				return columnsRow(gtx, th, cell(th, s.Name, pal.Fg), cell(th, s.Reason, pal.Muted), false)
			})
		})
	}
	if len(p.Renames) == 0 {
		return centered(gtx, th, "Nothing to rename.")
	}
	return material.List(th, &v.list).Layout(gtx, len(p.Renames), func(gtx C, i int) D {
		r := p.Renames[i]
		return stripedRow(gtx, i, func(gtx C) D {
			return columnsRow(gtx, th, cell(th, r.Old, pal.Muted), cell(th, r.New, pal.Fg), true)
		})
	})
}

// columnsRow lays out two equal-width columns, as MP3 Renamer did, so long
// names line up and are cut off within their own column. With arrow set, a
// narrow "→" column sits between them; otherwise an empty one of the same
// width keeps headings aligned with rows.
func columnsRow(gtx C, th *material.Theme, left, right layout.Widget, arrow bool) D {
	return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
		layout.Flexed(1, left),
		layout.Rigid(func(gtx C) D {
			gtx.Constraints.Min.X = gtx.Dp(unit.Dp(36))
			gtx.Constraints.Max.X = gtx.Constraints.Min.X
			if !arrow {
				return D{Size: image.Pt(gtx.Constraints.Min.X, 0)}
			}
			return layout.Center.Layout(gtx, label(th, "→", pal.Accent))
		}),
		layout.Flexed(1, right),
	)
}

// cell is one column's text, kept to a single line.
func cell(th *material.Theme, s string, c color.NRGBA) layout.Widget {
	return func(gtx C) D {
		l := material.Body2(th, s)
		l.Color = c
		l.MaxLines = 1
		return l.Layout(gtx)
	}
}

// stripedRow draws a list row with alternating backgrounds, like listRow.
func stripedRow(gtx C, i int, w layout.Widget) D {
	bg := pal.Surface
	if i%2 == 1 {
		bg = pal.Surface2
	}
	return layout.Background{}.Layout(gtx, rounded(bg, 6), func(gtx C) D {
		return layout.Inset{Top: 7, Bottom: 7, Left: 10, Right: 10}.Layout(gtx, func(gtx C) D {
			gtx.Constraints.Min.X = gtx.Constraints.Max.X
			return w(gtx)
		})
	})
}

func (v *renameView) layoutModal(gtx C, th *material.Theme) D {
	if v.state.phase != renameConfirming {
		return D{}
	}
	p := v.state.plan
	return modal(gtx, &v.scrim, &v.sink, func(gtx C) D {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(func(gtx C) D {
				l := material.H6(th, fmt.Sprintf("Rename %d files?", len(p.Renames)))
				l.Font.Weight = font.SemiBold
				return l.Layout(gtx)
			}),
			vspace(10),
			layout.Rigid(label(th, "In "+p.Dir+". Files are renamed in place and never replace an existing file.", pal.Fg)),
			vspace(18),
			layout.Rigid(func(gtx C) D {
				return layout.Flex{}.Layout(gtx,
					layout.Flexed(1, layout.Spacer{}.Layout),
					layout.Rigid(button(th, &v.cancelBtn, "Cancel", false)),
					hspace(8),
					layout.Rigid(button(th, &v.confirmBtn, "Rename", true)),
				)
			}),
		)
	})
}
