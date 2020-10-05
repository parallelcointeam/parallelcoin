package p9

import (
	"image"
	"image/color"
	"math"

	"gioui.org/f32"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/text"
	"gioui.org/unit"
	"gioui.org/widget"

	w "github.com/p9c/pod/pkg/gui/widget"

	"github.com/p9c/pod/pkg/gui/f32color"
)

type ButtonStyle struct {
	Text string
	// Color is the text color.
	Color        color.RGBA
	Font         text.Font
	TextSize     unit.Value
	Background   color.RGBA
	CornerRadius unit.Value
	Inset        layout.Inset
	Button       *w.Clickable
	shaper       text.Shaper
}

type ButtonLayoutStyle struct {
	Background   color.RGBA
	CornerRadius unit.Value
	Button       *w.Clickable
}

type IconButtonStyle struct {
	Background color.RGBA
	// Color is the icon color.
	Color color.RGBA
	Icon  *widget.Icon
	// Size is the icon size.
	Size   unit.Value
	Inset  layout.Inset
	Button *w.Clickable
}

// Clickable lays out a rectangular clickable widget without further decoration.
func Clickable(gtx layout.Context, button *w.Clickable, w layout.Widget) layout.Dimensions {
	return layout.Stack{}.Layout(gtx,
		layout.Expanded(button.Fn),
		layout.Expanded(func(gtx layout.Context) layout.Dimensions {
			clip.RRect{
				Rect: f32.Rectangle{Max: f32.Point{
					X: float32(gtx.Constraints.Min.X),
					Y: float32(gtx.Constraints.Min.Y),
				}},
			}.Add(gtx.Ops)
			for _, c := range button.History() {
				drawInk(gtx, widget.Press(c))
			}
			return layout.Dimensions{Size: gtx.Constraints.Min}
		}),
		layout.Stacked(w),
	)
}

func drawInk(gtx layout.Context, c widget.Press) {
	// duration is the number of seconds for the completed animation: expand while fading in, then out.
	const (
		expandDuration = float32(0.5)
		fadeDuration   = float32(0.9)
	)
	now := gtx.Now
	t := float32(now.Sub(c.Start).Seconds())
	end := c.End
	if end.IsZero() {
		// If the press hasn't ended, don't fade-out.
		end = now
	}
	endt := float32(end.Sub(c.Start).Seconds())
	// Compute the fade-in/out position in [0;1].
	var alphat float32
	{
		var haste float32
		if c.Cancelled {
			// If the press was cancelled before the inkwell was fully faded in, fast forward the animation to match the
			// fade-out.
			if h := 0.5 - endt/fadeDuration; h > 0 {
				haste = h
			}
		}
		// Fade in.
		half1 := t/fadeDuration + haste
		if half1 > 0.5 {
			half1 = 0.5
		}
		// Fade out.
		half2 := float32(now.Sub(end).Seconds())
		half2 /= fadeDuration
		half2 += haste
		if half2 > 0.5 {
			// Too old.
			return
		}

		alphat = half1 + half2
	}
	// Compute the expand position in [0;1].
	sizet := t
	if c.Cancelled {
		// Freeze expansion of cancelled presses.
		sizet = endt
	}
	sizet /= expandDuration

	// Animate only ended presses, and presses that are fading in.
	if !c.End.IsZero() || sizet <= 1.0 {
		op.InvalidateOp{}.Add(gtx.Ops)
	}

	if sizet > 1.0 {
		sizet = 1.0
	}

	if alphat > .5 {
		// Start fadeout after half the animation.
		alphat = 1.0 - alphat
	}
	// Twice the speed to attain fully faded in at 0.5.
	t2 := alphat * 2
	// Beziér ease-in curve.
	alphaBezier := t2 * t2 * (3.0 - 2.0*t2)
	sizeBezier := sizet * sizet * (3.0 - 2.0*sizet)
	size := float32(gtx.Constraints.Min.X)
	if h := float32(gtx.Constraints.Min.Y); h > size {
		size = h
	}
	// Cover the entire constraints min rectangle.
	size *= 2 * float32(math.Sqrt(2))
	// Apply curve values to size and color.
	size *= sizeBezier
	alpha := 0.7 * alphaBezier
	const col = 0.8
	ba, bc := byte(alpha*0xff), byte(col*0xff)
	defer op.Push(gtx.Ops).Pop()
	rgba := f32color.MulAlpha(color.RGBA{A: 0xff, R: bc, G: bc, B: bc}, ba)
	ink := paint.ColorOp{Color: rgba}
	ink.Add(gtx.Ops)
	rr := size * .5
	op.Offset(c.Position.Add(f32.Point{
		X: -rr,
		Y: -rr,
	})).Add(gtx.Ops)
	clip.RRect{
		Rect: f32.Rectangle{Max: f32.Point{
			X: float32(size),
			Y: float32(size),
		}},
		NE: rr, NW: rr, SE: rr, SW: rr,
	}.Add(gtx.Ops)
	paint.PaintOp{Rect: f32.Rectangle{Max: f32.Point{X: float32(size), Y: float32(size)}}}.Add(gtx.Ops)
}

func (th *Theme) Button(button *w.Clickable, font, txt string) ButtonStyle {
	var f text.Font
	for i := range th.Collection {
		// Debug(th.Collection[i].Font)
		if th.Collection[i].Font.Typeface == text.Typeface(font) {
			f = th.Collection[i].Font
		}
	}
	// run handler
	go func(button *w.Clickable) {
		// handler(button)
		for {
			select {
			case <-th.quit:
				// so the handler stops it listens to a quit channel
				return
			}
		}
	}(button)
	return ButtonStyle{
		Text:  txt,
		Font:  f,
		Color: rgb(0xffffff),
		// CornerRadius: unit.Dp(4),
		Background: th.Color.Primary,
		TextSize:   th.TextSize,
		Inset: layout.Inset{
			Top: unit.Dp(10), Bottom: unit.Dp(10),
			Left: unit.Dp(10), Right: unit.Dp(12),
		},
		Button: button,
		shaper: th.Shaper,
	}
}

func (b ButtonStyle) Fn(gtx layout.Context) layout.Dimensions {
	return ButtonLayoutStyle{
		Background:   b.Background,
		CornerRadius: b.CornerRadius,
		Button:       b.Button,
	}.Fn(gtx, func(gtx layout.Context) layout.Dimensions {
		return b.Inset.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			paint.ColorOp{Color: b.Color}.Add(gtx.Ops)
			return widget.Label{Alignment: text.Middle}.Layout(gtx, b.shaper, b.Font, b.TextSize, b.Text)
		})
	})
}

func ButtonLayout(th *Theme, button *w.Clickable) ButtonLayoutStyle {
	return ButtonLayoutStyle{
		Button:       button,
		Background:   th.Color.Primary,
		CornerRadius: unit.Dp(4),
	}
}

func (b ButtonLayoutStyle) Fn(gtx layout.Context, w layout.Widget) layout.Dimensions {
	min := gtx.Constraints.Min
	return layout.Stack{Alignment: layout.Center}.Layout(gtx,
		layout.Expanded(func(gtx layout.Context) layout.Dimensions {
			rr := float32(gtx.Px(b.CornerRadius))
			clip.RRect{
				Rect: f32.Rectangle{Max: f32.Point{
					X: float32(gtx.Constraints.Min.X),
					Y: float32(gtx.Constraints.Min.Y),
				}},
				NE: rr, NW: rr, SE: rr, SW: rr,
			}.Add(gtx.Ops)
			background := b.Background
			if gtx.Queue == nil {
				background = f32color.MulAlpha(b.Background, 150)
			}
			dims := fill(gtx, background)
			for _, c := range b.Button.History() {
				drawInk(gtx, widget.Press(c))
			}
			return dims
		}),
		layout.Stacked(func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Min = min
			return layout.Center.Layout(gtx, w)
		}),
		layout.Expanded(b.Button.Fn),
	)
}

func IconButton(th *Theme, button *w.Clickable, icon *widget.Icon) IconButtonStyle {
	return IconButtonStyle{
		Background: th.Color.Primary,
		Color:      th.Color.InvText,
		Icon:       icon,
		Size:       unit.Dp(24),
		Inset:      layout.UniformInset(unit.Dp(12)),
		Button:     button,
	}
}

func (b IconButtonStyle) Fn(gtx layout.Context) layout.Dimensions {
	return layout.Stack{Alignment: layout.Center}.Layout(gtx,
		layout.Expanded(func(gtx layout.Context) layout.Dimensions {
			sizex, sizey := gtx.Constraints.Min.X, gtx.Constraints.Min.Y
			sizexf, sizeyf := float32(sizex), float32(sizey)
			rr := (sizexf + sizeyf) * .25
			clip.RRect{
				Rect: f32.Rectangle{Max: f32.Point{X: sizexf, Y: sizeyf}},
				NE:   rr, NW: rr, SE: rr, SW: rr,
			}.Add(gtx.Ops)
			background := b.Background
			if gtx.Queue == nil {
				background = f32color.MulAlpha(b.Background, 150)
			}
			dims := fill(gtx, background)
			for _, c := range b.Button.History() {
				drawInk(gtx, widget.Press(c))
			}
			return dims
		}),
		layout.Stacked(func(gtx layout.Context) layout.Dimensions {
			return b.Inset.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				size := gtx.Px(b.Size)
				if b.Icon != nil {
					b.Icon.Color = b.Color
					b.Icon.Layout(gtx, unit.Px(float32(size)))
				}
				return layout.Dimensions{
					Size: image.Point{X: size, Y: size},
				}
			})
		}),
		layout.Expanded(func(gtx layout.Context) layout.Dimensions {
			pointer.Ellipse(image.Rectangle{Max: gtx.Constraints.Min}).Add(gtx.Ops)
			return b.Button.Fn(gtx)
		}),
	)
}