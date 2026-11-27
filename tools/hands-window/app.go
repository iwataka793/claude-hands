package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// 窓の大きさ（画面の単位。拡大率は Windows 側で掛ける）
const (
	liveW  = 320
	liveH  = 76
	smallW = 112
	smallH = 40
)

const (
	busyFor = 15 * time.Second  // 動いてからこれだけは広げておく
	showFor = 120 * time.Second // 作業が開いていなくても、動いてからこれだけは出しておく
)

type App struct {
	ctx     context.Context
	mu      sync.Mutex
	hover   bool
	stopped time.Time // ［止める］を押した時刻
	shown   bool
	compact bool
}

func newApp() *App { return &App{} }

func home() string {
	if h := os.Getenv("HANDS_HOME"); h != "" {
		return h
	}
	if h := os.Getenv("USERPROFILE"); h != "" {
		return h
	}
	h, _ := os.UserHomeDir()
	return h
}

func handsDir() string { return filepath.Join(home(), "devkit", "hands") }

// スイッチ（mode.txt）の場所：iPhone・PC・ブラウザ
func switchFiles() map[string]string {
	d := filepath.Join(home(), "devkit")
	return map[string]string{
		"iPhone": filepath.Join(d, "iphone-mcp", "mode.txt"),
		"PC":     filepath.Join(d, "pc-agent", "mode.txt"),
		"WEB":    filepath.Join(d, "browser", "mode.txt"),
	}
}

func readMode(p string) string {
	b, err := os.ReadFile(p)
	if err != nil {
		return "off"
	}
	m := strings.ToLower(strings.TrimSpace(string(b)))
	if m == "" {
		return "off"
	}
	return m
}

type entry struct {
	T    time.Time `json:"t"`
	Hand string    `json:"hand"`
	Tool string    `json:"tool"`
	OK   bool      `json:"ok"`
}

// lastEntry：今日の TIMELINE の最後の行（ファイルの最後だけ読む）
func lastEntry() (entry, bool) {
	p := filepath.Join(handsDir(), "timeline", time.Now().Format("2006-01-02")+".jsonl")
	f, err := os.Open(p)
	if err != nil {
		return entry{}, false
	}
	defer f.Close()
	st, _ := f.Stat()
	off := st.Size() - 16<<10
	if off < 0 {
		off = 0
	}
	_, _ = f.Seek(off, 0)
	var last entry
	ok := false
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		var e entry
		if json.Unmarshal(sc.Bytes(), &e) == nil && !e.T.IsZero() {
			last, ok = e, true
		}
	}
	return last, ok
}

type task struct {
	ID    string    `json:"id"`
	Title string    `json:"title"`
	Start time.Time `json:"start"`
}

func currentTask() *task {
	b, err := os.ReadFile(filepath.Join(handsDir(), "task.json"))
	if err != nil {
		return nil
	}
	var t task
	if json.Unmarshal(b, &t) != nil || t.ID == "" {
		return nil
	}
	return &t
}

// 道具の名前 → ふつうの言葉（ここに無いものは名前のまま）
var toolWords = map[string]string{
	"pc_powershell": "PowerShell を動かした", "pc_trash": "消した（取り置きへ）", "pc_open": "開いた", "pc_ui_do": "窓の部品を押した",
	"pc_keys": "キーを打った", "pc_ui": "窓の部品を読んだ", "pc_screenshot": "画面を撮った", "pc_files": "ファイルを読んだ",
	"pc_disk": "容量を調べた", "pc_problems": "不具合の記録を読んだ", "pc_processes": "動いているものを見た", "pc_apps": "アプリを見た",
	"pc_windows": "窓の一覧を見た", "hands": "作業の区切り",
	"browser_tabs": "タブ", "browser_read": "ページを読んだ", "browser_do": "ページを操作した", "browser_js": "ページで台本を動かした", "browser_screenshot": "ページを撮った",
	"iphone_tap": "押した", "iphone_tap_text": "押した", "iphone_type": "打った", "iphone_swipe": "なぞった", "iphone_steps": "まとめて操作した",
	"iphone_app": "アプリを開いた・閉じた", "iphone_screenshot": "画面を撮った", "iphone_elements": "画面の部品を読んだ", "iphone_info": "中の様子を見た",
	"iphone_files": "ファイル", "iphone_clipboard": "クリップボード", "iphone_logs": "ログを読んだ", "iphone_scroll_to": "スクロールした",
	"iphone_button": "ボタンを押した", "iphone_alert": "確認の窓", "iphone_web": "ページ", "iphone_location": "仮の位置",
}

type LiveState struct {
	Show    bool     `json:"show"`
	Compact bool     `json:"compact"`
	Title   string   `json:"title"`
	Last    string   `json:"last"`
	Ago     string   `json:"ago"`
	On      []string `json:"on"` // 入っているスイッチ
	Stopped bool     `json:"stopped"`
	NG      bool     `json:"ng"`
	Version string   `json:"version"`
}

func (a *App) compute() LiveState {
	st := LiveState{Version: version}
	for _, h := range []string{"iPhone", "PC", "WEB"} {
		if m := readMode(switchFiles()[h]); m != "off" {
			st.On = append(st.On, h)
		}
	}
	t := currentTask()
	e, ok := lastEntry()
	var ago time.Duration = 1 << 62
	if ok {
		ago = time.Since(e.T)
		w := toolWords[e.Tool]
		if w == "" {
			w = e.Tool
		}
		hand := map[string]string{"pc": "PC", "browser": "WEB", "iphone": "iPhone"}[e.Hand]
		st.Last = hand + "：" + w
		st.NG = !e.OK
		switch {
		case ago < time.Minute:
			st.Ago = fmt.Sprintf("%d 秒前", int(ago.Seconds()))
		case ago < time.Hour:
			st.Ago = fmt.Sprintf("%d 分前", int(ago.Minutes()))
		default:
			st.Ago = e.T.Format("15:04")
		}
	}
	if t != nil {
		st.Title = t.Title
	} else {
		st.Title = "（作業の区切りなし）"
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if !a.stopped.IsZero() && time.Since(a.stopped) < 10*time.Second {
		st.Stopped, st.Show, st.Compact = true, true, false
		return st
	}
	st.Show = len(st.On) > 0 && (t != nil || ago < showFor)
	st.Compact = !(ago < busyFor || a.hover)
	return st
}

func (a *App) startup(ctx context.Context) { a.ctx = ctx }

func (a *App) domReady(ctx context.Context) {
	winPrepare()
	go a.loop()
}

func (a *App) loop() {
	test := os.Getenv("HANDS_LIVE_TEST") != ""
	start := time.Now()
	for {
		st := a.compute()
		a.apply(st)
		runtime.EventsEmit(a.ctx, "state", st)
		if test && time.Since(start) > 6*time.Second {
			winReport(os.Getenv("HANDS_LIVE_TEST"), a.shown, a.compact)
			runtime.Quit(a.ctx)
			return
		}
		time.Sleep(time.Second)
	}
}

// apply：出す・消す・広げる・たたむ（フォーカスは取らない）
func (a *App) apply(st LiveState) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if st.Show != a.shown || (st.Show && st.Compact != a.compact) {
		if st.Show {
			w, h := liveW, liveH
			if st.Compact {
				w, h = smallW, smallH
			}
			winPlace(w, h)
			winShow(true)
		} else {
			winShow(false)
		}
		a.shown, a.compact = st.Show, st.Compact
	}
}

// State：画面から最初に呼ぶ
func (a *App) State() LiveState { return a.compute() }

// Hover：マウスが乗ったら広げる
func (a *App) Hover(on bool) {
	a.mu.Lock()
	a.hover = on
	a.mu.Unlock()
}

// Stop：全部のスイッチをオフにする（［止める］）
func (a *App) Stop() string {
	var ng []string
	for h, p := range switchFiles() {
		if _, err := os.Stat(filepath.Dir(p)); err != nil {
			continue
		}
		if err := os.WriteFile(p, []byte("off\r\n"), 0o644); err != nil {
			ng = append(ng, h)
		}
	}
	a.mu.Lock()
	a.stopped = time.Now()
	a.mu.Unlock()
	if len(ng) > 0 {
		return "止められなかった：" + strings.Join(ng, "・")
	}
	return "全部のスイッチをオフにしました"
}
