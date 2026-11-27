// claude-hands-live：右下の LIVE の窓（ASK-DESIGN.md の「窓」）。
// Claude が PC・ブラウザ・iPhone を動かしている間だけ、画面の右下に小さく出る。フォーカスは取らない（全画面の動画の上でも）。
// 動いた直後は広げて「何をしているか」、15 秒動きがなければ小さくたたむ。2 分なにもなく作業も開いていなければ消える。
// ［止める］：iPhone・PC・ブラウザのスイッチを全部オフにする（カオルの権限で mode.txt を書く。Claude の道具からは押せない作り）。
// 中身は記録（TIMELINE・task.json・スイッチ）から作る。Claude の説明は使わない。
package main

import (
	"embed"
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

const title = "claude-hands LIVE"

func main() {
	app := newApp()
	data := os.Getenv("LOCALAPPDATA")
	if data == "" {
		data = os.TempDir()
	}
	err := wails.Run(&options.App{
		Title:             title,
		Width:             liveW,
		Height:            liveH,
		Frameless:         true,
		AlwaysOnTop:       true,
		StartHidden:       true,
		DisableResize:     true,
		HideWindowOnClose: false,
		BackgroundColour:  &options.RGBA{R: 0, G: 0, B: 0, A: 0},
		AssetServer:       &assetserver.Options{Assets: sub(assets)},
		OnStartup:         app.startup,
		OnDomReady:        app.domReady,
		Bind:              []interface{}{app},
		SingleInstanceLock: &options.SingleInstanceLock{
			UniqueId: "claude-hands-live-7c1e",
		},
		Windows: &windows.Options{
			WebviewIsTransparent:              true,
			DisableWindowIcon:                 true,
			DisableFramelessWindowDecorations: true,
			WebviewUserDataPath:               filepath.Join(data, "claude-hands", "live-webview"),
		},
	})
	if err != nil {
		log.Println(err)
	}
}
