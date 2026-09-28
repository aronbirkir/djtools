package main

import (
	"fmt"

	"gioui.org/font"
	"gioui.org/layout"
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
			return listRow(gtx, th, i, s.Name+"  —  "+s.Reason, pal.Fg, 1)
		})
	}
	if len(p.Renames) == 0 {
		return centered(gtx, th, "Nothing to rename.")
	}
	return material.List(th, &v.list).Layout(gtx, len(p.Renames), func(gtx C, i int) D {
		r := p.Renames[i]
		return listRow(gtx, th, i, r.Old+"   →   "+r.New, pal.Fg, 1)
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
			layout.Rigid(label(th, "In "+p.Dir+". Files are renamed in place; no existing file is ever overwritten.", pal.Fg)),
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
