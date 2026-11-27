package main

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// 窓の大きさ（画面の単位。拡大率は Windows 側で掛ける）。どれも 1280×800 の高さの 3 分の 2 まで
const (
	panelW = 400
	panelH = 520
	askW   = 440
	askH   = 540
)

type App struct {
	ctx    context.Context
	mu     sync.Mutex
	ask    *pending
	others int
	askAt  time.Time
	panel  bool // スイッチの窓を出している（頼みが終わったらそこへ戻る）
	shown  string
	msg    string
	msgAt  time.Time
	tray   string // 通知領域に今出している色と文字
	quit   bool
}

func newApp() *App { return &App{} }

func (a *App) startup(ctx context.Context) { a.ctx = ctx }

func (a *App) domReady(ctx context.Context) {
	winPrepare()
	trayStart(trayHandlers{open: a.ShowPanel, allOff: func() { a.AllOff() }, quit: a.Quit})
	if !hasArg("/tray") && os.Getenv("HANDS_SWITCH_TEST") == "" {
		a.ShowPanel()
	}
	go a.loop()
}

func (a *App) beforeClose(ctx context.Context) bool {
	a.mu.Lock()
	q := a.quit
	a.mu.Unlock()
	if q {
		return false
	}
	a.HidePanel() // Alt+F4 などで閉じても、通知領域に残る
	return true
}

func hasArg(s string) bool {
	for _, x := range os.Args[1:] {
		if x == s {
			return true
		}
	}
	return false
}

func (a *App) loop() {
	// 試験（HANDS_SWITCH_TEST）：5 秒で窓の様子を書き、12 秒で終わる（その間に頼みの時間切れも起きる）
	test := os.Getenv("HANDS_SWITCH_TEST")
	start, reported := time.Now(), false
	for {
		a.tick()
		if test != "" && !reported && time.Since(start) > 5*time.Second {
			a.mu.Lock()
			shown := a.shown
			a.mu.Unlock()
			winReport(test, shown)
			reported = true
		}
		if test != "" && time.Since(start) > 12*time.Second {
			trayStop()
			runtime.Quit(a.ctx)
			return
		}
		time.Sleep(500 * time.Millisecond)
	}
}

func (a *App) tick() {
	a.mu.Lock()
	if a.ask == nil {
		if p, others := nextRequest(); p != nil {
			a.ask, a.others, a.askAt = p, others, time.Now()
			a.mu.Unlock()
			a.place()
			playSound()
			a.mu.Lock()
		}
	} else {
		a.others = countRequests()
		if time.Now().After(a.ask.Deadline) {
			p := a.ask
			a.ask = nil
			a.mu.Unlock()
			logAsk(p, "timeout")
			writeAnswer(p.Kind, "timeout")
			a.place()
			a.mu.Lock()
		}
	}
	a.mu.Unlock()
	devs := devViews()
	allow := allowLeft() > 0
	t := strongest(devs, allow) + "|" + trayTip(devs, allow)
	if t != a.tray {
		a.tray = t
		traySet(trayTip(devs, allow), strongest(devs, allow))
	}
}

// place：今出すべき形（頼み・スイッチ・隠す）に窓を合わせる
func (a *App) place() {
	a.mu.Lock()
	want := "hide"
	switch {
	case a.ask != nil:
		want = "ask"
	case a.panel:
		want = "panel"
	}
	if want == a.shown {
		a.mu.Unlock()
		return
	}
	a.shown = want
	a.mu.Unlock()
	switch want {
	case "ask":
		// 真ん中・いちばん前。フォーカスは取らない（打っているキーが窓に入らないように）
		winPlace(askW, askH, true)
		winTopmost(true)
		winShow(false)
	case "panel":
		winTopmost(false)
		winPlace(panelW, panelH, false)
		winShow(true)
	default:
		winTopmost(false)
		winHide()
	}
}

// ---- 画面から呼ぶもの ----

// State：画面が 0.5 秒ごとに読む
func (a *App) State() View {
	devs := devViews()
	v := View{Devs: devs, AllowSecs: int(allowLeft().Seconds()), Last: lastAction(), Version: version}
	a.mu.Lock()
	v.Ask = askView(a.ask, a.others)
	if a.msg != "" && time.Since(a.msgAt) < 8*time.Second {
		v.Msg = a.msg
	}
	a.mu.Unlock()
	return v
}

func (a *App) say(s string) {
	a.mu.Lock()
	a.msg, a.msgAt = s, time.Now()
	a.mu.Unlock()
}

// ShowPanel：スイッチの窓を出す（通知領域を押した・ショートカットを開いた）
func (a *App) ShowPanel() {
	a.mu.Lock()
	a.panel = true
	a.shown = "" // 出し直す（隠れていた・後ろに回っていた）
	a.mu.Unlock()
	a.place()
}

func (a *App) HidePanel() {
	a.mu.Lock()
	a.panel = false
	a.mu.Unlock()
	a.place()
}

func (a *App) Minimise() { runtime.WindowMinimise(a.ctx) }

func (a *App) Quit() {
	a.mu.Lock()
	a.quit = true
	a.mu.Unlock()
	trayStop()
	runtime.Quit(a.ctx)
}

// SetMode：スイッチを切り替える（カオルが押した）
func (a *App) SetMode(key, mode string) string {
	if err := setMode(key, mode); err != nil {
		a.say(err.Error())
		return err.Error()
	}
	return ""
}

// setMode：mode.txt を書く。PC とブラウザは同じ常駐役（pc-agent.exe）なので、オンなら動かす（両方オフで常駐役は自分で終わる）
func setMode(key, mode string) error {
	d, ok := deviceOf(key)
	if !ok || (mode != "off" && mode != "view" && mode != "operate") {
		return errorf("知らない切り替え（%s・%s）", key, mode)
	}
	exe := filepath.Join(devkit(), "pc-agent", "pc-agent.exe")
	if key != "iphone" && mode != "off" {
		if _, err := os.Stat(exe); err != nil {
			return errorf("pc-agent.exe がありません。先に pc-setup.bat を実行してください")
		}
	}
	if err := writeMode(d.Dir, mode); err != nil {
		return errorf("切り替えられませんでした（%v）", err)
	}
	if key != "iphone" && mode != "off" && !agentRunning() {
		if err := startAgent(exe); err != nil {
			return errorf("常駐役を起動できませんでした（%v）", err)
		}
	}
	return nil
}

// AllOff：全部オフ
func (a *App) AllOff() string {
	for _, d := range devices() {
		if _, err := os.Stat(d.Dir); err == nil {
			_ = writeMode(d.Dir, "off")
		}
	}
	a.say("全部のスイッチをオフにしました")
	return ""
}

// AllowNow：ALLOW を 10 分（UAC を通る。Claude には押せない）
func (a *App) AllowNow() {
	go func() {
		if ok, why := grantAllow(); !ok {
			a.say("許可しませんでした（" + why + "）")
		}
	}()
}

// AllowCancel：ALLOW を取り消す
func (a *App) AllowCancel() {
	if err := cancelAllow(); err != nil {
		a.say("取り消せませんでした（" + err.Error() + "）")
		return
	}
	a.say("ALLOW を取り消しました")
}

// Answer：頼みの窓の［DENY］［ALLOW］
func (a *App) Answer(kind string, allow bool) string {
	a.mu.Lock()
	p := a.ask
	if p == nil || p.Kind != kind {
		a.mu.Unlock()
		return ""
	}
	if time.Since(a.askAt) < time.Second {
		a.mu.Unlock()
		return "" // 出た直後の押しまちがいは受け付けない
	}
	a.ask = nil
	a.mu.Unlock()
	defer a.place()
	if !allow {
		logAsk(p, "deny")
		writeAnswer(kind, "deny")
		return "断りました"
	}
	k := kinds[kind]
	if k.Mode == "allow" {
		logAsk(p, "allow")
		writeAnswer(kind, "allow")
		go func() {
			if ok, why := grantAllow(); !ok {
				logAsk(p, "deny（"+why+"）")
				writeAnswer(kind, "deny")
				a.say("許可しませんでした（" + why + "）")
			}
		}()
		return "許可しました（UAC が出ます）"
	}
	if err := setMode(k.Device, k.Mode); err != nil {
		logAsk(p, "deny（"+err.Error()+"）")
		writeAnswer(kind, "deny")
		a.say(err.Error())
		return err.Error()
	}
	logAsk(p, "allow")
	writeAnswer(kind, "allow")
	return "許可しました"
}
