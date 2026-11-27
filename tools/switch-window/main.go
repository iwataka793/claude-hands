// claude-hands-switch：スイッチと「許可しますか？」の窓（ASK-DESIGN.md の「窓」の SWITCH・ALLOW）。
//   - スイッチ：iPhone・PC・WEB を OFF・VIEW・OPERATE に切り替える。ALLOW（パスワード・セキュリティ・お金を 10 分）は UAC を通る
//   - 頼み：Claude の道具が devkit\requests に頼みを置くと、真ん中に WHAT（しようとしていること）・WHY（Claude の説明）・
//     IF ALLOWED（許可すると）を出す。決めるのはカオル。返事は requests\<種類>.answer.json（断ったら道具はすぐ Claude に返す）
//   - 通知領域（右下）に常駐する。窓を閉じても残る（終わるのは通知領域のメニューの「終わる」）
//
// 今までの PowerShell の窓（tools/switch-app/devkit-switch.ps1）と同じことをする。あちらは予備に残す。
package main

import (
	"embed"
	"io/fs"
	"log"
	"os"
	"path/filepath"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/windows"
)

//go:embed frontend
var assets embed.FS

var version = "dev"

const title = "claude-hands SWITCH"

func main() {
	winRemember() // 起動する前にいちばん前だった窓（Wails が取るので、あとで返す）
	app := newApp()
	data := os.Getenv("LOCALAPPDATA")
	if data == "" {
		data = os.TempDir()
	}
	front, err := fs.Sub(assets, "frontend")
	if err != nil {
		panic(err)
	}
	err = wails.Run(&options.App{
		Title:             title,
		Width:             panelW,
		Height:            panelH,
		Frameless:         true,
		StartHidden:       true,
		DisableResize:     true,
		HideWindowOnClose: false,
		BackgroundColour:  &options.RGBA{R: 0, G: 0, B: 0, A: 0},
		AssetServer:       &assetserver.Options{Assets: front},
		OnStartup:         app.startup,
		OnDomReady:        app.domReady,
		OnBeforeClose:     app.beforeClose,
		Bind:              []interface{}{app},
		SingleInstanceLock: &options.SingleInstanceLock{
			UniqueId: "claude-hands-switch-3f9a",
			OnSecondInstanceLaunch: func(d options.SecondInstanceData) {
				for _, a := range d.Args {
					if a == "/tray" {
						return // 道具が起こしただけ（動いていれば何もしない）
					}
				}
				app.ShowPanel()
			},
		},
		Windows: &windows.Options{
			WebviewIsTransparent:              true,
			DisableFramelessWindowDecorations: true,
			WebviewUserDataPath:               filepath.Join(data, "claude-hands", "switch-webview"),
		},
	})
	if err != nil {
		log.Println(err)
	}
}
