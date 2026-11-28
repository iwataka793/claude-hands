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
	liveW  = 340
	liveH  = 84
	smallW = 112
	smallH = 40
	doneW  = 380
	doneH  = 172
	openH  = 340 // DONE を「詳しく」で開いたとき
)

const doneFor = 3 * time.Minute // DONE は閉じなければこれだけ出しておく（マウスが乗っている間は消さない）

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
	size    [2]int
	started bool

	// DONE
	seenDone  string    // もう見せた（か、起動したときにもうあった）いちばん新しい作業の番号
	doneID    string    // いま見せている DONE
	doneAt    time.Time // 見せ始めた時刻
	doneOpen  bool      // 詳しく
	undoArmed bool      // ［まとめて戻す］を 1 回押した（もう 1 回で戻す）
	undoMsg   string
	doneFit   int // DONE（たたんだ形）の中身の高さ（画面から。題名の行数で変わる）

	// TRY（箱で試した結果。try.go）
	trySeen  string    // もう鳴らした試しの番号
	tryArmed string    // apply・discard を 1 回押した
	tryMsg   string    // 窓に出す返事
	tryResAt time.Time // 押し終わった時刻（結果を少し出す）
	tryID    string
	tryFile  string
	tryFit   int
	checkFit int // CHECK の中身の高さ（画面から）
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

type Chip struct {
	Name string `json:"name"`
	Mode string `json:"mode"`
}

type LiveState struct {
	Show    bool     `json:"show"`
	Compact bool     `json:"compact"`
	Title   string   `json:"title"`
	Last    string   `json:"last"`
	Ago     string   `json:"ago"`
	On      []string `json:"on"`    // 入っているスイッチ
	Chips   []Chip   `json:"chips"` // 入っているスイッチと、その強さ（色の札）
	Stopped bool     `json:"stopped"`
	NG      bool     `json:"ng"`
	Version string   `json:"version"`

	Mode      string   `json:"mode"`                // live・done・check
	Check     []string `json:"check,omitempty"`     // 容量の点検の知らせ（CHECK）
	CheckTime string   `json:"checkTime,omitempty"` // その点検の時刻
	DoneTitle string   `json:"doneTitle,omitempty"`
	DoneTime  string   `json:"doneTime,omitempty"`
	Summary   []string `json:"summary,omitempty"`
	Report    string   `json:"report,omitempty"`
	Open      bool     `json:"open"`
	UndoArmed bool     `json:"undoArmed"`
	UndoMsg   string   `json:"undoMsg,omitempty"`
	Try       *TryView `json:"try,omitempty"`
}

func (a *App) compute() LiveState {
	st := LiveState{Version: version}
	for _, h := range []string{"iPhone", "PC", "WEB"} {
		if m := readMode(switchFiles()[h]); m != "off" {
			st.On = append(st.On, h)
			st.Chips = append(st.Chips, Chip{h, m})
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
	st.Mode = "live"
	if id := newestDone(); id != "" && id != a.seenDone {
		if !a.started {
			a.seenDone = id // 起動したときにもうあったものは見せない
		} else {
			a.seenDone, a.doneID, a.doneAt, a.doneOpen, a.undoArmed, a.undoMsg = id, id, time.Now(), false, false, ""
		}
	}
	if a.computeTry(&st) {
		a.started = true
		return st
	}
	a.started = true
	if a.doneID != "" && (time.Since(a.doneAt) < doneFor || a.hover) {
		d := loadDone(a.doneID)
		st.Mode, st.Show, st.Compact = "done", true, false
		st.DoneTitle, st.DoneTime, st.Summary, st.Report = d.title, d.when, d.summary, d.report
		st.Open, st.UndoArmed, st.UndoMsg = a.doneOpen, a.undoArmed, a.undoMsg
		return st
	}
	a.doneID = ""
	if !a.stopped.IsZero() && time.Since(a.stopped) < 10*time.Second {
		st.Stopped, st.Show, st.Compact = true, true, false
		return st
	}
	st.Show = len(st.On) > 0 && (t != nil || ago < showFor)
	st.Compact = !(ago < busyFor || a.hover)
	// CHECK：容量の点検に知らせがあり、今日まだ閉じていなければ出す（Claude が動いている間は LIVE を優先）
	if !st.Show {
		if notes, when := checkNotes(); len(notes) > 0 && checkClosedOn() != time.Now().Format("2006-01-02") {
			st.Mode, st.Show, st.Compact, st.Check, st.CheckTime = "check", true, false, notes, when
		}
	}
	return st
}

// ---- CHECK（容量の点検の知らせ。docs/ASK-DESIGN.md「容量の決まり」） ----

func checkFile() string       { return filepath.Join(handsDir(), "status", "capacity.txt") }
func checkClosedFile() string { return filepath.Join(handsDir(), "status", "check-closed.txt") }

// checkNotes：常駐役が書いた点検の結果から「知らせ：」の行（なしは除く）と時刻
func checkNotes() ([]string, string) {
	b, err := os.ReadFile(checkFile())
	if err != nil {
		return nil, ""
	}
	var notes []string
	when := ""
	for _, l := range strings.Split(strings.ReplaceAll(strings.TrimPrefix(string(b), "\ufeff"), "\r\n", "\n"), "\n") {
		if v, ok := strings.CutPrefix(l, "時刻："); ok {
			when = v
		}
		if v, ok := strings.CutPrefix(l, "知らせ："); ok && v != "なし" {
			notes = append(notes, v)
		}
	}
	return notes, when
}

// checkClosedOn：カオルが CHECK を閉じた日（yyyy-mm-dd）
func checkClosedOn() string {
	b, _ := os.ReadFile(checkClosedFile())
	return strings.TrimSpace(string(b))
}

// CloseCheck：CHECK を閉じる（今日はもう出さない。知らせが続いていれば次の日にまた出す）
func (a *App) CloseCheck() {
	_ = os.MkdirAll(filepath.Dir(checkClosedFile()), 0o755)
	_ = os.WriteFile(checkClosedFile(), []byte(time.Now().Format("2006-01-02")), 0o644)
}

// FitCheck：画面が CHECK の中身の高さを知らせる
func (a *App) FitCheck(h int) {
	a.mu.Lock()
	a.checkFit = h
	a.mu.Unlock()
}

func (a *App) checkHeight() int {
	if a.checkFit >= 100 && a.checkFit <= 400 {
		return a.checkFit
	}
	return doneH
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
		if winSwitchOpen() {
			// スイッチの窓を開いている間は隠れる（あちらの下の帯が LIVE の役をする）。DONE は閉じるまで取っておく
			st.Show = false
			a.mu.Lock()
			if a.doneID != "" {
				a.doneAt = time.Now()
			}
			a.mu.Unlock()
		}
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
	w, h := liveW, liveH
	switch {
	case st.Mode == "try":
		w, h = tryW, a.tryHeight()
	case st.Mode == "done" && st.Open:
		w, h = doneW, openH
	case st.Mode == "done":
		w, h = doneW, a.doneHeight()
	case st.Mode == "check":
		w, h = doneW, a.checkHeight()
	case st.Compact:
		w, h = smallW, smallH
	}
	if st.Show != a.shown || (st.Show && a.size != [2]int{w, h}) {
		if st.Show {
			winPlace(w, h)
			winShow(true)
		} else {
			winShow(false)
		}
		a.shown, a.compact, a.size = st.Show, st.Compact, [2]int{w, h}
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

// ---- DONE ----

func doneDir() string { return filepath.Join(handsDir(), "done") }

// newestDone：いちばん新しい DONE の作業の番号（done\<番号>.txt）
func newestDone() string {
	es, err := os.ReadDir(doneDir())
	if err != nil {
		return ""
	}
	best := ""
	for _, e := range es {
		if n := e.Name(); strings.HasSuffix(n, ".txt") && strings.TrimSuffix(n, ".txt") > best {
			best = strings.TrimSuffix(n, ".txt")
		}
	}
	return best
}

type doneInfo struct {
	title, when, report string
	summary             []string
}

// loadDone：DONE の報告（常駐役が記録から作ったもの）を読む
func loadDone(id string) doneInfo {
	var d doneInfo
	b, err := os.ReadFile(filepath.Join(doneDir(), id+".txt"))
	if err != nil {
		d.title = id
		return d
	}
	txt := strings.ReplaceAll(strings.TrimPrefix(string(b), "\ufeff"), "\r\n", "\n")
	var rest []string
	for i, l := range strings.Split(txt, "\n") {
		if i == 0 && strings.HasPrefix(l, "DONE：") {
			t := strings.TrimPrefix(l, "DONE：")
			if k := strings.LastIndex(t, "（"); k > 0 {
				d.when = strings.Trim(t[k:], "（）")
				t = t[:k]
			}
			d.title = t
			continue
		}
		if strings.HasPrefix(l, "戻すとき：") {
			continue
		}
		if strings.HasPrefix(l, "手の数：") || strings.HasPrefix(l, "変えたファイル") || strings.HasPrefix(l, "消したもの") {
			d.summary = append(d.summary, l)
		}
		rest = append(rest, l)
	}
	d.report = strings.TrimSpace(strings.Join(rest, "\n"))
	return d
}

// CloseDone：DONE を閉じる（LIVE に戻る）
func (a *App) CloseDone() {
	a.mu.Lock()
	a.doneID, a.undoArmed = "", false
	a.mu.Unlock()
}

// OpenDone：詳しく／たたむ
func (a *App) OpenDone(open bool) {
	a.mu.Lock()
	a.doneOpen = open
	a.doneAt = time.Now()
	a.mu.Unlock()
}

// Undo：［まとめて戻す］。1 回目は「本当に？」にするだけ、2 回目で常駐役に頼む（カオルが押したと記録に残す）
func (a *App) Undo() string {
	a.mu.Lock()
	id, armed := a.doneID, a.undoArmed
	if id == "" {
		a.mu.Unlock()
		return ""
	}
	if !armed {
		a.undoArmed, a.doneAt = true, time.Now()
		a.mu.Unlock()
		return "もう一度押すと戻します"
	}
	a.undoArmed = false
	a.mu.Unlock()
	msg := callUndo(id)
	a.mu.Lock()
	a.undoMsg, a.doneAt, a.doneOpen = msg, time.Now(), true
	a.mu.Unlock()
	return msg
}

func (a *App) doneHeight() int {
	if a.doneFit >= 120 && a.doneFit <= 400 {
		return a.doneFit
	}
	return doneH
}

// FitDone：画面が DONE の中身の高さを知らせる（題名が長いと折り返して高くなる）
func (a *App) FitDone(h int) {
	a.mu.Lock()
	a.doneFit = h
	a.mu.Unlock()
}
