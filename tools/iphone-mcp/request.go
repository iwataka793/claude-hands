package main

// Claude からの頼み：スイッチが足りないとき、カオルの PC に「許可しますか？」の窓を出してもらう。
// 道具は mode.txt を書かない。%USERPROFILE%\devkit\requests\<kind>.txt に頼み（JSON）を置くだけで、
// 窓を出して決めるのはスイッチの窓（claude-hands-switch.exe。無ければ予備の devkit-switch.ps1。カオルが押す）。
// 中身の決まりは tools/pc-mcp/request.go と同じ（what は道具が作る・why は今の作業の名前・返事は <kind>.answer.json）。

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"sync/atomic"
	"time"
)

func requestDir() string { return filepath.Join(os.Getenv("USERPROFILE"), "devkit", "requests") }

type askRequest struct {
	Kind string    `json:"kind"`
	What string    `json:"what"`
	Why  string    `json:"why"`
	Tool string    `json:"tool,omitempty"`
	From string    `json:"from"`
	At   time.Time `json:"at"`
}

// curTool：いま動いている道具の名前（timeline.go の受け口で入れる。窓の WHAT に使う）
var curTool atomic.Value

func currentTool() string {
	if s, ok := curTool.Load().(string); ok {
		return s
	}
	return ""
}

// askWhy：Claude の説明。今の作業の名前（%USERPROFILE%\devkit\hands\task.json の題名。無ければ空）
func askWhy() string {
	b, err := os.ReadFile(filepath.Join(os.Getenv("USERPROFILE"), "devkit", "hands", "task.json"))
	if err != nil {
		return ""
	}
	var t struct {
		Title string `json:"title"`
	}
	_ = json.Unmarshal(b, &t)
	return t.Title
}

var iphoneAction = map[string]string{
	"iphone_screenshot": "iPhone の画面を撮る", "iphone_elements": "iPhone の画面の部品を読む", "iphone_info": "iPhone の中の様子を見る",
	"iphone_logs": "iPhone のログを読む", "iphone_files": "iPhone のアプリのファイルを読む・置く", "iphone_clipboard": "iPhone のクリップボードを読む・書く",
	"iphone_tap": "iPhone の画面を押す", "iphone_tap_text": "iPhone の画面の文字を押す", "iphone_type": "iPhone で文字を打つ",
	"iphone_swipe": "iPhone の画面をなぞる", "iphone_steps": "iPhone をまとめて操作する", "iphone_app": "iPhone のアプリを開く・閉じる",
	"iphone_scroll_to": "iPhone の画面をスクロールする", "iphone_button": "iPhone のボタン（ホーム・音量など）を押す",
	"iphone_alert": "iPhone の確認の窓に答える", "iphone_web": "iPhone でページを開く", "iphone_location": "iPhone の位置を仮に変える",
	"iphone_pinch": "iPhone の画面を広げる・縮める", "iphone_wait": "iPhone の画面を待つ", "iphone_devkit": "iPhone の開発の道具を動かす",
	"iphone_lc_install": "iPhone にアプリを入れる",
}

func iphoneWhat(need Mode) string {
	t := currentTool()
	if s, ok := iphoneAction[t]; ok {
		return s
	}
	if t != "" {
		return "よく分からない操作（" + t + "）"
	}
	if need == ModeOperate {
		return "iPhone を操作する"
	}
	return "iPhone の画面を見る"
}

// askKaoru は頼みを置き、スイッチの窓が動いていなければ起動する（動いていれば何もしないで終わる）。
// 頼めないとき（Windows でない・試験で DEVKIT_NO_ASK が付いている）は false。
func askKaoru(kind, what string) bool {
	if os.Getenv("USERPROFILE") == "" || os.Getenv("DEVKIT_NO_ASK") != "" {
		return false
	}
	_ = os.MkdirAll(requestDir(), 0o755)
	_ = os.Remove(filepath.Join(requestDir(), kind+".answer.json"))
	b, _ := json.Marshal(askRequest{Kind: kind, What: what, Why: askWhy(), Tool: currentTool(), From: "iphone-mcp " + version, At: time.Now()})
	_ = os.WriteFile(filepath.Join(requestDir(), kind+".txt"), b, 0o644)
	dk := filepath.Join(os.Getenv("USERPROFILE"), "devkit")
	if exe := filepath.Join(dk, "hands", "claude-hands-switch.exe"); exists(exe) {
		_ = exec.Command(exe, "/tray").Start()
	} else if vbs := filepath.Join(dk, "switch-app", "devkit-switch.vbs"); exists(vbs) {
		_ = exec.Command("wscript.exe", vbs, "/tray").Start()
	}
	return true
}

func exists(p string) bool { _, err := os.Stat(p); return err == nil }

func readAnswer(kind string) string {
	b, err := os.ReadFile(filepath.Join(requestDir(), kind+".answer.json"))
	if err != nil {
		return ""
	}
	var a struct {
		Answer string `json:"answer"`
	}
	_ = json.Unmarshal(b, &a)
	return a.Answer
}

// waitKaoru は ok() が true になるまで最大 d 待つ。窓で断られた・時間切れになったら、そこで終わる。
func waitKaoru(kind string, d time.Duration, ok func() bool) (bool, string) {
	end := time.Now().Add(d)
	for time.Now().Before(end) {
		if ok() {
			return true, ""
		}
		switch readAnswer(kind) {
		case "deny":
			return false, "カオルが断りました。同じ操作をやり直さず、チャットでカオルに聞いてください"
		case "timeout":
			return false, "カオルの PC に「許可しますか？」の窓を出しましたが、返事がありませんでした（時間切れ）。チャットでカオルに聞いてください"
		}
		time.Sleep(time.Second)
	}
	if ok() {
		return true, ""
	}
	return false, "カオルの PC に「許可しますか？」の窓を出しましたが、" + strconv.Itoa(int(d.Seconds())) + " 秒で返事がありませんでした。チャットでカオルに聞いてください"
}
