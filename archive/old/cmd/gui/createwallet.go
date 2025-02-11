package gui

import (
	"fmt"
	"github.com/p9c/pod/pkg/chaincfg"
	"github.com/p9c/pod/pkg/fork"
	"github.com/p9c/pod/pkg/gui"
	"github.com/p9c/pod/pkg/podcfg"
	"github.com/p9c/pod/pkg/util/interrupt"
	"github.com/p9c/pod/pkg/util/qu"
	"golang.org/x/exp/shiny/materialdesign/icons"
	"os"
	"time"
	
	l "gioui.org/layout"
	"github.com/urfave/cli"
	
	"github.com/p9c/pod/pkg/wallet"
)

const slash = string(os.PathSeparator)

func (wg *WalletGUI) CreateWalletPage(gtx l.Context) l.Dimensions {
	walletForm := wg.createWalletFormWidgets()
	le := func(gtx l.Context, index int) l.Dimensions {
		return wg.Inset(0.25, walletForm[index]).Fn(gtx)
	}
	return func(gtx l.Context) l.Dimensions {
		return wg.Fill(
			"DocBg", l.Center, 0, 0,
			// wg.Inset(
			// 	0.5,
			wg.VFlex().
				Flexed(
					1,
					wg.lists["createWallet"].Vertical().Start().Length(len(walletForm)).ListElement(le).Fn,
				).
				Rigid(
					wg.createConfirmExitBar(),
				).Fn,
			// ).Fn,
		).Fn(gtx)
	}(gtx)
}

func (wg *WalletGUI) createConfirmExitBar() l.Widget {
	return wg.VFlex().
		// Rigid(
		// 	wg.Inset(
		// ,
		// 			).Fn,
		// 		).
		Rigid(
			
			wg.Flex().
				Rigid(
					func(gtx l.Context) l.Dimensions {
						return wg.Flex().
							Rigid(
								wg.ButtonLayout(
									wg.clickables["quit"].SetClick(
										func() {
											interrupt.Request()
										},
									),
								).
									CornerRadius(0.5).
									Corners(0).
									Background("PanelBg").
									Embed(
										wg.Inset(
											0.25,
											wg.Flex().AlignMiddle().
												Rigid(
													wg.Icon().
														Scale(
															gui.
																Scales["H4"],
														).
														Color("DocText").
														Src(
															&icons.
																MapsDirectionsRun,
														).Fn,
												).
												Rigid(
													wg.Inset(
														0.5,
														gui.EmptySpace(
															0,
															0,
														),
													).Fn,
												).
												Rigid(
													wg.H6("exit").Color("DocText").Fn,
												).
												Rigid(
													wg.Inset(
														0.5,
														gui.EmptySpace(
															0,
															0,
														),
													).Fn,
												).
												Fn,
										).Fn,
									).Fn,
							).
							Fn(gtx)
					},
				).
				Flexed(
					1,
					gui.EmptyMaxWidth(),
				).
				Rigid(
					func(gtx l.Context) l.Dimensions {
						if !wg.createWalletInputsAreValid() {
							gtx = gtx.Disabled()
						}
						return wg.Flex().
							Rigid(
								wg.ButtonLayout(
									wg.clickables["createWallet"].SetClick(
										func() {
											go wg.createWalletAction()
										},
									),
								).
									CornerRadius(0).
									Corners(0).
									Background("Primary").
									Embed(
										// wg.Fill("DocText",
										wg.Inset(
											0.25,
											wg.Flex().AlignMiddle().
												Rigid(
													wg.Icon().
														Scale(
															gui.
																Scales["H4"],
														).
														Color("DocText").
														Src(
															&icons.
																ContentCreate,
														).Fn,
												).
												Rigid(
													wg.Inset(
														0.5,
														gui.EmptySpace(
															0,
															0,
														),
													).Fn,
												).
												Rigid(
													wg.H6("create").Color("DocText").Fn,
												).
												Rigid(
													wg.Inset(
														0.5,
														gui.EmptySpace(
															0,
															0,
														),
													).Fn,
												).
												Fn,
										).Fn,
									).Fn,
							).
							Fn(gtx)
					},
				).
				Fn,
		).
		Fn
}

func (wg *WalletGUI) createWalletPasswordsMatch() bool {
	return wg.passwords["passEditor"].GetPassword() != "" &&
		wg.passwords["confirmPassEditor"].GetPassword() != "" &&
		len(wg.passwords["passEditor"].GetPassword()) >= 8 &&
		wg.passwords["passEditor"].GetPassword() ==
			wg.passwords["confirmPassEditor"].GetPassword()
}

func (wg *WalletGUI) createWalletInputsAreValid() bool {
	return wg.createWalletPasswordsMatch() && wg.bools["ihaveread"].GetValue() && wg.createWords == wg.createMatch
}

func (wg *WalletGUI) createWalletAction() {
	// wg.NodeRunCommandChan <- "stop"
	D.Ln("clicked submit wallet")
	*wg.cx.Config.WalletFile = *wg.cx.Config.DataDir +
		string(os.PathSeparator) + wg.cx.ActiveNet.Name +
		string(os.PathSeparator) + wallet.DbName
	dbDir := *wg.cx.Config.WalletFile
	loader := wallet.NewLoader(wg.cx.ActiveNet, dbDir, 250)
	// seed, _ := hex.DecodeString(wg.inputs["walletSeed"].GetText())
	seed := wg.createSeed
	pass := []byte(wg.passwords["passEditor"].GetPassword())
	*wg.cx.Config.WalletPass = string(pass)
	D.Ln("password", string(pass))
	podcfg.Save(wg.cx.Config)
	w, e := loader.CreateNewWallet(
		pass,
		pass,
		seed,
		time.Now(),
		false,
		wg.cx.Config,
		qu.T(),
	)
	D.Ln("*** created wallet")
	if E.Chk(e) {
		// return
	}
	w.Stop()
	D.Ln("shutting down wallet", w.ShuttingDown())
	w.WaitForShutdown()
	D.Ln("starting main app")
	*wg.cx.Config.Generate = true
	*wg.cx.Config.GenThreads = 1
	*wg.cx.Config.NodeOff = false
	*wg.cx.Config.WalletOff = false
	podcfg.Save(wg.cx.Config)
	// // we are going to assume the config is not manually misedited
	// if apputil.FileExists(*wg.cx.Config.ConfigFile) {
	// 	b, e := ioutil.ReadFile(*wg.cx.Config.ConfigFile)
	// 	if e == nil {
	// 		wg.cx.Config, wg.cx.ConfigMap = pod.EmptyConfig()
	// 		e = json.Unmarshal(b, wg.cx.Config)
	// 		if e != nil {
	// 			E.Ln("error unmarshalling config", e)
	// 			// os.Exit(1)
	// 			panic(e)
	// 		}
	// 	} else {
	// 		F.Ln("unexpected error reading configuration file:", e)
	// 		// os.Exit(1)
	// 		// return e
	// 		panic(e)
	// 	}
	// }
	*wg.noWallet = false
	// interrupt.Request()
	// wg.wallet.Stop()
	// wg.wallet.Start()
	// wg.node.Start()
	// wg.miner.Start()
	wg.unlockPassword.Editor().SetText(string(pass))
	wg.unlockWallet(string(pass))
	interrupt.RequestRestart()
}

func (wg *WalletGUI) createWalletTestnetToggle(b bool) {
	D.Ln("testnet on?", b)
	// if the password has been entered, we need to copy it to the variable
	if wg.passwords["passEditor"].GetPassword() != "" ||
		wg.passwords["confirmPassEditor"].GetPassword() != "" ||
		len(wg.passwords["passEditor"].GetPassword()) >= 8 ||
		wg.passwords["passEditor"].GetPassword() ==
			wg.passwords["confirmPassEditor"].GetPassword() {
		*wg.cx.Config.WalletPass = wg.passwords["confirmPassEditor"].GetPassword()
		D.Ln("wallet pass", *wg.cx.Config.WalletPass)
	}
	if b {
		wg.cx.ActiveNet = &chaincfg.TestNet3Params
		fork.IsTestnet = true
	} else {
		wg.cx.ActiveNet = &chaincfg.MainNetParams
		fork.IsTestnet = false
	}
	I.Ln("activenet:", wg.cx.ActiveNet.Name)
	D.Ln("setting ports to match network")
	*wg.cx.Config.Network = wg.cx.ActiveNet.Name
	*wg.cx.Config.P2PListeners = cli.StringSlice{
		fmt.Sprintf(
			"0.0.0.0:" + wg.cx.ActiveNet.DefaultPort,
		),
	}
	address := fmt.Sprintf(
		"127.0.0.1:%s",
		wg.cx.ActiveNet.RPCClientPort,
	)
	*wg.cx.Config.RPCListeners = cli.StringSlice{address}
	*wg.cx.Config.RPCConnect = address
	address = fmt.Sprintf("127.0.0.1:" + wg.cx.ActiveNet.WalletRPCServerPort)
	*wg.cx.Config.WalletRPCListeners = cli.StringSlice{address}
	*wg.cx.Config.WalletServer = address
	*wg.cx.Config.NodeOff = false
	podcfg.Save(wg.cx.Config)
}
