package main

import (
	"image"
	"image/color"
	"strings"

	"gioui.org/font"
	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
)

type (
	C = layout.Context
	D = layout.Dimensions
)

func rgb(c uint32) color.NRGBA {
	return color.NRGBA{R: uint8(c >> 16), G: uint8(c >> 8), B: uint8(c), A: 0xff}
}

func withAlpha(c color.NRGBA, a uint8) color.NRGBA {
	c.A = a
	return c
}

// rounded returns a widget that fills its minimum constraints with a rounded rectangle.
// Use it as the background of a layout.Background.
func rounded(c color.NRGBA, radius unit.Dp) layout.Widget {
	return func(gtx C) D {
		size := gtx.Constraints.Min
		defer clip.UniformRRect(image.Rectangle{Max: size}, gtx.Dp(radius)).Push(gtx.Ops).Pop()
		paint.Fill(gtx.Ops, c)
		return D{Size: size}
	}
}

// fill paints the whole available area.
func fill(gtx C, c color.NRGBA) D {
	size := gtx.Constraints.Max
	defer clip.Rect{Max: size}.Push(gtx.Ops).Pop()
	paint.Fill(gtx.Ops, c)
	return D{Size: size}
}

// drainFuncs runs every func currently queued on ch, in order, without
// blocking once it's empty. pruneView and renameView both use it to run
// results a worker goroutine sent back on their own done channel; each is
// drained every frame, including while the other tool is showing, so a
// finished scan or apply run is never left waiting.
func drainFuncs(ch chan func()) {
	for {
		select {
		case f := <-ch:
			f()
		default:
			return
		}
	}
}

// divider draws a 1dp horizontal line across the available width.
func divider(gtx C) D {
	size := image.Pt(gtx.Constraints.Max.X, gtx.Dp(1))
	defer clip.Rect{Max: size}.Push(gtx.Ops).Pop()
	paint.Fill(gtx.Ops, pal.Border)
	return D{Size: size}
}

// modal draws a confirmation dialog over the whole window. scrim covers
// everything behind it and cancels when clicked; sink covers the dialog
// itself so clicks on its body do not fall through to the scrim. Draw it
// last, so it is on top for both painting and pointer input.
func modal(gtx C, scrim, sink *widget.Clickable, body layout.Widget) D {
	gtx.Constraints.Min = gtx.Constraints.Max
	return layout.Stack{Alignment: layout.Center}.Layout(gtx,
		layout.Expanded(func(gtx C) D {
			return scrim.Layout(gtx, func(gtx C) D { return fill(gtx, withAlpha(rgb(0x000000), 0x99)) })
		}),
		layout.Stacked(func(gtx C) D {
			width := min(gtx.Constraints.Max.X, gtx.Dp(unit.Dp(520)))
			gtx.Constraints.Min.X, gtx.Constraints.Max.X = width, width
			return sink.Layout(gtx, func(gtx C) D { return card(gtx, body) })
		}),
	)
}

func vspace(dp unit.Dp) layout.FlexChild {
	return layout.Rigid(layout.Spacer{Height: dp}.Layout)
}

func hspace(dp unit.Dp) layout.FlexChild {
	return layout.Rigid(layout.Spacer{Width: dp}.Layout)
}

func card(gtx C, w layout.Widget) D {
	return widget.Border{Color: pal.Border, CornerRadius: unit.Dp(12), Width: unit.Dp(1)}.Layout(gtx, func(gtx C) D {
		return layout.Background{}.Layout(gtx, rounded(pal.Surface, 12), func(gtx C) D {
			return layout.UniformInset(unit.Dp(18)).Layout(gtx, w)
		})
	})
}

func fieldLabel(th *material.Theme, s string) layout.Widget {
	l := material.Caption(th, strings.ToUpper(s))
	l.Color = pal.Muted
	l.Font.Weight = font.SemiBold
	return l.Layout
}

// label is a Body2 text in the given colour.
func label(th *material.Theme, s string, c color.NRGBA) layout.Widget {
	l := material.Body2(th, s)
	l.Color = c
	return l.Layout
}

func input(gtx C, th *material.Theme, ed *widget.Editor, hint string) D {
	border := pal.Border
	if gtx.Source.Focused(ed) {
		border = pal.Accent
	}
	return widget.Border{Color: border, CornerRadius: unit.Dp(8), Width: unit.Dp(1)}.Layout(gtx, func(gtx C) D {
		return layout.Background{}.Layout(gtx, rounded(pal.Surface2, 8), func(gtx C) D {
			return layout.Inset{Top: 10, Bottom: 10, Left: 12, Right: 12}.Layout(gtx, func(gtx C) D {
				gtx.Constraints.Min.X = gtx.Constraints.Max.X
				e := material.Editor(th, ed, hint)
				e.HintColor = pal.Muted
				e.SelectionColor = withAlpha(pal.Accent, 0x60)
				return e.Layout(gtx)
			})
		})
	})
}

// styleButton applies the app's button look.
func styleButton(b *material.ButtonStyle) {
	b.CornerRadius = unit.Dp(8)
	// The label's line box reserves room for descenders, so button text looks
	// high when centered. Shift the padding down to center it visually.
	b.Inset = layout.Inset{Top: 13, Bottom: 7, Left: 18, Right: 18}
	b.Font.Weight = font.SemiBold
	b.TextSize = unit.Sp(14)
}

func button(th *material.Theme, c *widget.Clickable, text string, primary bool) layout.Widget {
	b := material.Button(th, c, text)
	styleButton(&b)
	if !primary {
		b.Background = pal.Surface2
		b.Color = pal.Fg
	}
	return b.Layout
}

// segmented draws a row of mutually exclusive options: the theme switch and
// the orphan/details tabs.
func segmented(gtx C, th *material.Theme, clicks []widget.Clickable, labels []string, selected int) D {
	segments := make([]layout.FlexChild, len(labels))
	for i, text := range labels {
		segments[i] = layout.Rigid(func(gtx C) D {
			on := i == selected
			return material.Clickable(gtx, &clicks[i], func(gtx C) D {
				bg := pal.Surface
				if on {
					bg = pal.Accent
				}
				return layout.Background{}.Layout(gtx, rounded(bg, 6), func(gtx C) D {
					return layout.Inset{Top: 9, Bottom: 3, Left: 12, Right: 12}.Layout(gtx, func(gtx C) D {
						l := material.Body2(th, text)
						l.TextSize = unit.Sp(13)
						l.Font.Weight = font.SemiBold
						l.Color = pal.Muted
						if on {
							l.Color = rgb(0xffffff)
						}
						return l.Layout(gtx)
					})
				})
			})
		})
	}
	return widget.Border{Color: pal.Border, CornerRadius: unit.Dp(9), Width: unit.Dp(1)}.Layout(gtx, func(gtx C) D {
		return layout.Background{}.Layout(gtx, rounded(pal.Surface, 9), func(gtx C) D {
			return layout.UniformInset(unit.Dp(3)).Layout(gtx, func(gtx C) D {
				return layout.Flex{}.Layout(gtx, segments...)
			})
		})
	})
}
