package duoui

import (
	"github.com/p9c/pod/pkg/gio/app"
	"github.com/p9c/pod/pkg/gio/layout"
	"github.com/p9c/pod/pkg/gio/unit"
	"github.com/p9c/pod/pkg/gio/widget"
	"github.com/p9c/pod/pkg/gio/widget/material"
	"github.com/p9c/pod/cmd/gui/ico"
	"github.com/p9c/pod/cmd/gui/models"
	"github.com/p9c/pod/pkg/conte"
	"github.com/p9c/pod/pkg/log"
	"github.com/p9c/pod/pkg/fonts"
	"golang.org/x/exp/shiny/materialdesign/icons"
	"image/color"
)

func DuOuI(cx *conte.Xt) (duo *DuoUI) {
	//opts := &app.Options{
	//	Width:  unit.Dp(800),
	//	Height: unit.Dp(600),
	//	Title:  "Gio",
	//}
	duo = &DuoUI{
		cx: cx,
		rc: RcInit(),
		ww: app.NewWindow(),
	}
	fonts.Register()
//duo.ww.Width =  unit.Dp(800)
//	duo.ww.Height=  unit.Dp(600)
//		duo.ww.Title =  "ParallelCoin - True Story"

	duo.gc = layout.NewContext(duo.ww.Queue())
	duo.cs = &duo.gc.Constraints

	// Layouts
	view := models.DuoUIcomponent{
		Layout: layout.Flex{Axis: layout.Vertical},
	}
	header := models.DuoUIcomponent{
		Layout: layout.Flex{Axis: layout.Horizontal, Spacing: layout.SpaceBetween},
	}
	logo := models.DuoUIcomponent{
		Layout: layout.Flex{Axis: layout.Vertical},
	}
	body := models.DuoUIcomponent{
		Layout: layout.Flex{Axis: layout.Horizontal},
	}
	sidebar := models.DuoUIcomponent{
		Layout: layout.Flex{Axis: layout.Vertical},
		Inset: layout.UniformInset(unit.Dp(8)),
	}
	content := models.DuoUIcomponent{
		Layout: layout.Flex{Axis: layout.Vertical},
		Inset: layout.UniformInset(unit.Dp(30)),
	}
	overview := models.DuoUIcomponent{
		Layout: layout.Flex{Axis: layout.Vertical},
	}
	overviewTop := models.DuoUIcomponent{
		Layout: layout.Flex{Axis: layout.Horizontal},
	}
	sendReceive := models.DuoUIcomponent{
		Layout: layout.Flex{Axis: layout.Vertical},
		Inset: layout.UniformInset(unit.Dp(15)),
	}
	sendReceiveButtons := models.DuoUIcomponent{
		Layout: layout.Flex{Axis: layout.Horizontal},
	}
	overviewBottom := models.DuoUIcomponent{
		Layout: layout.Flex{Axis: layout.Horizontal},
	}
	status := models.DuoUIcomponent{
		Layout: layout.Flex{Axis: layout.Vertical},
		Inset: layout.UniformInset(unit.Dp(15)),
	}
	menu := models.DuoUIcomponent{
		Layout: layout.Flex{Axis: layout.Vertical, Alignment: layout.Middle},
	}

	duo.comp = &models.DuoUIcomponents{
		View:               view,
		Header:             header,
		Logo:               logo,
		Body:               body,
		Sidebar:            sidebar,
		Content:            content,
		Overview:           overview,
		OverviewTop:        overviewTop,
		SendReceive:        sendReceive,
		SendReceiveButtons: sendReceiveButtons,
		OverviewBottom:     overviewBottom,
		Status:             status,
		Menu:               menu,
	}

	// Navigation
	duo.menu = &models.DuoUInav{
		Current: "overview",
		//icoBackground: color.RGBA{A: 0xff, R: 0xcf, G: 0xcf, B: 0xcf},
		IcoColor:    color.RGBA{A: 0xff, R: 0x30, G: 0x30, B: 0x30},
		IcoPadding:  unit.Dp(8),
		IcoSize:     unit.Dp(64),
		Overview:    *new(widget.Button),
		History:     *new(widget.Button),
		AddressBook: *new(widget.Button),
		Explorer:    *new(widget.Button),
		Settings:    *new(widget.Button),
	}

	// Icons

	var err error
	ics := &models.DuoUIicons{}

	ics.Logo, err = material.NewIcon(ico.ParallelCoin)
	if err != nil {
		log.FATAL(err)
	}
	ics.Overview, err = material.NewIcon(icons.ActionHome)
	if err != nil {
		log.FATAL(err)
	}
	ics.History, err = material.NewIcon(icons.ActionHistory)
	if err != nil {
		log.FATAL(err)
	}
	ics.AddressBook, err = material.NewIcon(icons.ActionBook)
	if err != nil {
		log.FATAL(err)
	}
	ics.Explorer, err = material.NewIcon(icons.ActionExplore)
	if err != nil {
		log.FATAL(err)
	}
	ics.Network, err = material.NewIcon(icons.ActionFingerprint)
	if err != nil {
		log.FATAL(err)
	}
	ics.Settings, err = material.NewIcon(icons.ActionSettings)
	if err != nil {
		log.FATAL(err)
	}
	duo.ico = ics

	duo.th = material.NewTheme()
	return
}