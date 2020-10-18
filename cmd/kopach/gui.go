package kopach

import (
	"fmt"
	"image"
	"runtime"
	"time"

	"gioui.org/app"
	l "gioui.org/layout"
	"gioui.org/text"

	"github.com/p9c/pod/app/conte"
	"github.com/p9c/pod/app/save"
	"github.com/p9c/pod/pkg/gui/f"
	"github.com/p9c/pod/pkg/gui/fonts/p9fonts"
	icons "github.com/p9c/pod/pkg/gui/ico/svg"
	"github.com/p9c/pod/pkg/gui/p9"
	"github.com/p9c/pod/pkg/util/interrupt"
)

var maxThreads = float32(runtime.NumCPU())

type MinerModel struct {
	*p9.Theme
	Cx                     *conte.Xt
	worker                 *Worker
	DarkTheme              bool
	logoButton             *p9.Clickable
	mineToggle             *p9.Bool
	nCores                 int
	solButtons             []*p9.Clickable
	lists                  map[string]*p9.List
	solutionCount          int
	modalWidget            l.Widget
	modalOn                bool
	modalScrim, modalClose *p9.Clickable
	password               *p9.Password
	threadSlider           *p9.IntSlider
}

func (w *Worker) Run() {
	th := p9.NewTheme(p9fonts.Collection(), w.quit)
	solButtons := make([]*p9.Clickable, 201)
	for i := range solButtons {
		solButtons[i] = th.Clickable()
	}
	lists := map[string]*p9.List{
		"found": th.List().Vertical().Start(),
	}
	minerModel := &MinerModel{
		Cx:        w.cx,
		worker:    w,
		Theme:     th,
		DarkTheme: *w.cx.Config.DarkTheme,
		logoButton: th.Clickable().SetClick(func() {
			Debug("clicked logo button")
		}),
		mineToggle: th.Bool(*w.cx.Config.Generate),
		solButtons: solButtons,
		lists:      lists,
		modalScrim: th.Clickable(),
		modalClose: th.Clickable(),
		password: th.Password(w.cx.Config.MinerPass, func(pass string) {
			Debug("changed password")
			*w.cx.Config.MinerPass = pass
			save.Pod(w.cx.Config)
		}),
		threadSlider: th.IntSlider().Min(0).Max(maxThreads).Value(*w.cx.Config.GenThreads).Hook(func(v int){
			w.SetThreads <- v
		}),
	}
	minerModel.SetTheme(minerModel.DarkTheme)
	for i := 0; i < 201; i++ {
		minerModel.solButtons[i] = th.Clickable()
	}
	minerModel.logoButton.SetClick(
		func() {
			minerModel.FlipTheme()
			Info("clicked logo button")
		})
	win := f.NewWindow()
	go func() {
		if err := win.
			Size(640, 480).
			Title("kopach").
			Open().
			Run(
				minerModel.Widget,
				func() {
					Debug("quitting miner")
					close(w.quit)
					interrupt.Request()
				}); Check(err) {
		}
	}()
	go func() {
		for {
			select {
			case <-minerModel.worker.Update:
				win.Window.Invalidate()
			}
		}
	}()
	app.Main()
}

func (m *MinerModel) Widget(gtx l.Context) l.Dimensions {
	return m.Stack().Stacked(
		m.Flex().Flexed(1,
			m.Flex().Vertical().
				Rigid(m.Header).
				Flexed(1,
					m.Fill("DocBg").Embed(
						m.Inset(0.5).Embed(
							m.Flex().Vertical().
								Rigid(m.H5("miner settings").Fn).
								Rigid(m.RunControl).
								Rigid(m.SetThreads).
								Rigid(m.PreSharedKey).
								Rigid(m.VSpacer).
								Rigid(m.H5("found blocks").Fn).
								Flexed(1,
									m.Fill("PanelBg").Embed(
										m.FoundBlocks,
									).Fn,
								).Fn,
						).Fn,
					).Fn,
				).Fn,
		).Fn,
	).
		Stacked(func(gtx l.Context) l.Dimensions {
			if m.modalOn {
				return m.Fill("scrim").Embed(
					m.Flex().
						Vertical().
						Flexed(0.1,
							m.Flex().
								Rigid(
									func(gtx l.Context) l.Dimensions {
										return l.Dimensions{
											Size: image.Point{
												X: gtx.Constraints.Max.X,
												Y: gtx.Constraints.Max.Y,
											},
											Baseline: 0,
										}
									}).Fn,
						).AlignMiddle().
						Rigid(m.modalWidget).
						Flexed(0.1,
							m.Flex().
								Rigid(
									func(gtx l.Context) l.Dimensions {
										return l.Dimensions{
											Size: image.Point{
												X: gtx.Constraints.Max.X,
												Y: gtx.Constraints.Max.Y,
											},
											Baseline: 0,
										}
									},
								).Fn,
						).Fn,
				).Fn(gtx)
			} else {
				return l.Dimensions{}
			}
		}).
		Fn(gtx)
}

func (m *MinerModel) FillSpace(gtx l.Context) l.Dimensions {
	return l.Dimensions{
		Size: image.Point{
			X: gtx.Constraints.Min.X,
			Y: gtx.Constraints.Min.Y,
		},
		Baseline: 0,
	}
}

func (m *MinerModel) VSpacer(gtx l.Context) l.Dimensions {
	return l.Dimensions{
		Size: image.Point{
			X: int(m.TextSize.Scale(2).V),
			Y: int(m.TextSize.Scale(2).V),
		},
		Baseline: 0,
	}
}

func (m *MinerModel) Header(gtx l.Context) l.Dimensions {
	return m.Fill("Primary").Embed(
		m.Flex().Rigid(
			m.Inset(0.25).Embed(
				m.IconButton(m.logoButton).
					Color("Light").
					Background("Dark").
					Scale(p9.Scales["H4"]).
					Icon(icons.ParallelCoin).
					Fn,
			).Fn,
		).Rigid(
			m.Inset(0.5).Embed(
				m.H5("kopach").
					Color("DocBg").
					Fn,
			).Fn,
		).Flexed(1,
			m.Inset(0.5).Embed(
				m.Body1(fmt.Sprintf("%d hash/s", int(m.worker.hashrate))).
					Color("DocBg").
					Alignment(text.End).
					Fn,
			).Fn,
		).Fn,
	).Fn(gtx)
}

func (m *MinerModel) RunControl(gtx l.Context) l.Dimensions {
	return m.Inset(0.25).Embed(
		m.Flex().Flexed(0.5,
			m.Body1("enable mining").
				Color("DocText").
				Fn,
		).Flexed(0.5,
			m.Switch(m.mineToggle.SetOnChange(
				func(b bool) {
					if b {
						Debug("start mining")
						m.worker.StartChan <- struct{}{}
					} else {
						Debug("stop mining")
						m.worker.StopChan <- struct{}{}
					}
				})).
				Fn,
		).Fn,
	).Fn(gtx)
}

func (m *MinerModel) SetThreads(gtx l.Context) l.Dimensions {
	return m.Flex().Rigid(
		m.Inset(0.25).Embed(
			m.Flex().
				Flexed(0.5,
					m.Body1("number of mining threads"+
						fmt.Sprintf("%3v", int(m.threadSlider.GetValue()))).
						Fn,
				).
				Flexed(0.5,
					m.threadSlider.Fn,
				).
				Fn,
		).Fn,
	).Fn(gtx)
}

func (m *MinerModel) PreSharedKey(gtx l.Context) l.Dimensions {
	return m.Inset(0.25).Embed(
		m.Flex().Flexed(0.5,
			m.Body1("cluster preshared key").
				Color("DocText").
				Fn,
		).Flexed(0.5,
			m.password.Fn,
		).Fn,
	).Fn(gtx)
}

func (m *MinerModel) BlockInfoModalCloser(gtx l.Context) l.Dimensions {
	return m.Button(m.modalScrim.SetClick(func() {
		m.modalOn = false
	})).Background("Primary").Text("close").Fn(gtx)
}

var currentBlock SolutionData

func (m *MinerModel) BlockDetails(gtx l.Context) l.Dimensions {
	return m.Fill("DocBg").Embed(
		m.Flex().Vertical().AlignMiddle().Rigid(
			m.Inset(0.5).Embed(
				m.H5("Block Information").Alignment(text.Middle).Color("DocText").Fn,
			).Fn,
		).Rigid(
			m.Inset(0.5).Embed(
				m.Flex().Rigid(
					m.Flex().Vertical().
						Rigid(
							m.H6("Height").Font("bariol bold").Fn,
						).
						Rigid(
							m.H6("PoW Hash").Font("bariol bold").Fn,
						).
						Rigid(
							m.H6("Algorithm").Font("bariol bold").Fn,
						).
						Rigid(
							m.H6("Version").Font("bariol bold").Fn,
						).
						Rigid(
							m.H6("Index Hash").Font("bariol bold").Fn,
						).
						Rigid(
							m.H6("Prev Block").Font("bariol bold").Fn,
						).
						Rigid(
							m.H6("Merkle Root").Font("bariol bold").Fn,
						).
						Rigid(
							m.H6("Timestamp").Font("bariol bold").Fn,
						).
						Rigid(
							m.H6("Bits").Font("bariol bold").Fn,
						).
						Rigid(
							m.H6("Nonce").Font("bariol bold").Fn,
						).
						Fn,

				).Rigid(
					m.Flex().Vertical().
						Rigid(
							m.Flex().
								AlignBaseline().
								Rigid(
									m.H6(" ").Font("bariol bold").Fn,
								).
								Rigid(
									m.Body1(fmt.Sprintf("%d", currentBlock.height)).
										Fn,
								).
								Fn,
						).
						Rigid(
							m.Flex().
								AlignBaseline().
								Rigid(
									m.H6(" ").Font("bariol bold").Fn,
								).
								Rigid(
									m.Caption(fmt.Sprintf("%s", currentBlock.hash)).
										Font("go regular").
										Fn,
								).Fn,
						).
						Rigid(
							m.Flex().
								AlignBaseline().
								Rigid(
									m.H6(" ").Font("bariol bold").Fn,
								).
								Rigid(
									m.Body1(currentBlock.algo).
										Fn,
								).
								Fn,
						).
						Rigid(
							m.Flex().
								AlignBaseline().
								Rigid(
									m.H6(" ").Font("bariol bold").Fn,
								).
								Rigid(
									m.Body1(fmt.Sprintf("%d", currentBlock.version)).
										Fn,
								).
								Fn,
						).
						Rigid(
							m.Flex().
								AlignBaseline().
								Rigid(
									m.H6(" ").Font("bariol bold").Fn,
								).
								Rigid(
									m.Caption(fmt.Sprintf("%s", currentBlock.indexHash)).
										Font("go regular").
										Fn,
								).
								Fn,
						).
						Rigid(
							m.Flex().
								AlignBaseline().
								Rigid(
									m.H6(" ").Font("bariol bold").Fn,
								).
								Rigid(
									m.Caption(fmt.Sprintf("%s", currentBlock.prevBlock)).
										Font("go regular").
										Fn,
								).Fn,
						).
						Rigid(
							m.Flex().
								AlignBaseline().
								Rigid(
									m.H6(" ").Font("bariol bold").Fn,
								).
								Rigid(
									m.Caption(fmt.Sprintf("%s", currentBlock.merkleRoot)).
										Font("go regular").
										Fn,
								).Fn,
						).
						Rigid(
							m.Flex().
								AlignBaseline().
								Rigid(
									m.H6(" ").Font("bariol bold").Fn,
								).
								Rigid(
									m.Body1(currentBlock.timestamp.Format(time.RFC3339)).
										Fn,
								).Fn,
						).
						Rigid(
							m.Flex().
								AlignBaseline().
								Rigid(
									m.H6(" ").Font("bariol bold").Fn,
								).
								Rigid(
									m.Body1(fmt.Sprintf("%x", currentBlock.bits)).
										Fn,
								).Fn,
						).
						Rigid(
							m.Flex().
								AlignBaseline().
								Rigid(
									m.H6(" ").Font("bariol bold").Fn,
								).
								Rigid(
									m.Body1(fmt.Sprintf("%d", currentBlock.nonce)).
										Fn,
								).Fn,
						).Fn,
				).Fn,

			).Fn,
		).Rigid(
			m.Inset(0.5).Embed(
				m.BlockInfoModalCloser,
			).Fn,
		).Fn,
	).Fn(gtx)
}

func (m *MinerModel) FoundBlocks(gtx l.Context) l.Dimensions {
	return m.Inset(0.25).Embed(
		m.Flex().Flexed(1, func(gtx l.Context) l.Dimensions {
			return m.lists["found"].
				End().
				ScrollWidth(int(m.Theme.TextSize.V)).
				ScrollToEnd().
				Length(m.worker.solutionCount).
				ListElement(
					func(gtx l.Context, i int) l.Dimensions {
						return m.Flex().
							Rigid(
								m.Button(m.solButtons[i].SetClick(func() {
									currentBlock = m.worker.solutions[i]
									Debug("clicked for block", currentBlock.height)
									m.modalWidget = m.BlockDetails
									m.modalOn = true
								})).Text(fmt.Sprint(m.worker.solutions[i].height)).Inset(0.5).Fn,
							).Flexed(1,
							m.Inset(0.25).Embed(
								m.Flex().Vertical().Rigid(
									m.Flex().Rigid(
										m.Body1(m.worker.solutions[i].algo).Font("plan9").Fn,
									).Flexed(1,
										m.Flex().Vertical().Rigid(
											m.Body1(m.worker.solutions[i].hash).
												Font("go regular").
												TextScale(0.75).
												Alignment(text.End).
												Fn,
										).Rigid(
											m.Caption(fmt.Sprint(
												m.worker.solutions[i].time.Format(time.RFC3339))).
												Alignment(text.End).
												Fn,
										).Fn,
									).Fn,
								).Fn,
							).Fn,
						).Fn(gtx)
					}).Fn(gtx)
		}).Fn,
	).Fn(gtx)
}