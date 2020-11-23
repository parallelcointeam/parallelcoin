package main

import (
	l "gioui.org/layout"
	"github.com/p9c/pod/pkg/util/logi"

	"github.com/p9c/pod/pkg/gui/f"
	"github.com/p9c/pod/pkg/gui/fonts/p9fonts"
	"github.com/p9c/pod/pkg/gui/p9"
)

type App struct {
	th   *p9.Theme
	quit chan struct{}
}

func main() {
	logi.L.SetLevel("trace", false, "pod")
	quit := make(chan struct{})
	th := p9.NewTheme(p9fonts.Collection(), quit)
	model := App{
		th: th,
	}
	go func() {
		if err := f.NewWindow().
			Size(800, 600).
			Title("table example").
			Open().
			Run(model.mainWidget, func(l.Context) {}, func() {
				close(quit)
			}, quit); Check(err) {
		}
	}()
	<-quit
}

func (m *App) mainWidget(gtx l.Context) l.Dimensions {
	th := m.th
	gtx.Constraints.Max = gtx.Constraints.Min
	dims := th.Flex().AlignStart().Rigid(
		m.th.Body1("test").Fn,
	).Fn(gtx)
	Infos(dims)
	return dims
}