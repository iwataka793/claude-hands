package main

// Claude からの頼み：スイッチが足りないとき、カオルの PC に「許可しますか？」の窓を出してもらう。
// 道具は mode.txt を書かない。%USERPROFILE%\devkit\requests\<kind>.txt に頼み（JSON）を置くだけで、
// 窓を出して決めるのはスイッチの窓（claude-hands-switch.exe。無ければ予備の devkit-switch.ps1。カオルが押す）。
//
// 頼みの中身（ASK-DESIGN.md の「窓」）：
//   what：しようとしていること。道具が作る（Claude には書かせない）
//   why ：Claude の説明。今は作業の名前（hands start の題名）
//   「許可すると」の文は窓の側が種類ごとに持つ（頼みからは受け取らない）
// 窓の返事は requests\<kind>.answer.json（allow・deny・timeout）。断られたら待たずに Claude に返す。

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
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

type askAnswer struct {
	Answer string    `json:"answer"` // allow・deny・timeout
	At     time.Time `json:"at"`
}

// askWhy：Claude の説明。今の作業の名前（無ければ空）
func askWhy() string {
	if t := currentTask(); t != nil {
		return t.Title
	}
	return ""
}

// askKaoru は頼みを置き、スイッチの窓が動いていなければ起動する（動いていれば何もしないで終わる）。
// 頼めないとき（Windows でない・試験で DEVKIT_NO_ASK が付いている）は false。
func askKaoru(kind, tool, what string) bool {
	if os.Getenv("USERPROFILE") == "" || os.Getenv("DEVKIT_NO_ASK") != "" {
		return false
	}
	_ = os.MkdirAll(requestDir(), 0o755)
	_ = os.Remove(filepath.Join(requestDir(), kind+".answer.json")) // 前の返事は捨てる
	b, _ := json.Marshal(askRequest{Kind: kind, What: what, Why: askWhy(), Tool: tool, From: "pc-mcp " + version, At: time.Now()})
	_ = os.WriteFile(filepath.Join(requestDir(), kind+".txt"), b, 0o644)
	startSwitchWindow()
	return true
}

// startSwitchWindow：新しい窓（claude-hands-switch.exe）があればそれ、無ければ予備の PowerShell の窓を、通知領域に起こす
func startSwitchWindow() {
	dk := filepath.Join(os.Getenv("USERPROFILE"), "devkit")
	if exe := filepath.Join(dk, "hands", "claude-hands-switch.exe"); fileExists(exe) {
		_ = exec.Command(exe, "/tray").Start()
		return
	}
	if vbs := filepath.Join(dk, "switch-app", "devkit-switch.vbs"); fileExists(vbs) {
		_ = exec.Command("wscript.exe", vbs, "/tray").Start()
	}
}

func fileExists(p string) bool { _, err := os.Stat(p); return err == nil }

// readAnswer：窓の返事（まだ無ければ ""）
func readAnswer(kind string) string {
	b, err := os.ReadFile(filepath.Join(requestDir(), kind+".answer.json"))
	if err != nil {
		return ""
	}
	var a askAnswer
	if json.Unmarshal(b, &a) != nil {
		return ""
	}
	return a.Answer
}

const deniedMsg = "カオルが断りました。同じ操作をやり直さず、チャットでカオルに聞いてください"

// waitKaoru は ok() が true になるまで最大 d 待つ。窓で断られた・時間切れになったら、そこで終わる。
// 返り値：通ったか、通らなかったときの理由（断った・返事が無い）
func waitKaoru(kind string, d time.Duration, ok func() bool) (bool, string) {
	end := time.Now().Add(d)
	for time.Now().Before(end) {
		if ok() {
			return true, ""
		}
		switch readAnswer(kind) {
		case "deny":
			return false, deniedMsg
		case "timeout":
			return false, "カオルの PC に「許可しますか？」の窓を出しましたが、返事がありませんでした（時間切れ）。チャットでカオルに聞いてください"
		case "allow":
			// 許可された：スイッチ・許可のファイルが変わるまで少し待つ（UAC を通る ALLOW は時間がかかる）
		}
		time.Sleep(time.Second)
	}
	if ok() {
		return true, ""
	}
	return false, "カオルの PC に「許可しますか？」の窓を出しましたが、" + secs(d) + " 秒で返事がありませんでした。チャットでカオルに聞いてください"
}

func secs(d time.Duration) string { return strconv.Itoa(int(d.Seconds())) }

// waitFor は ok() が true になるまで最大 d 待つ。
func waitFor(d time.Duration, ok func() bool) bool {
	end := time.Now().Add(d)
	for time.Now().Before(end) {
		if ok() {
			return true
		}
		time.Sleep(time.Second)
	}
	return ok()
}

// toolAction：道具が「何をしようとしているか」の短い言い方（窓の WHAT。言い換えの表はこの先で育てる）
var toolAction = map[string]string{
	"pc_status": "PC の様子を見る", "pc_screenshot": "PC の画面を撮る", "pc_windows": "PC の窓の一覧を見る",
	"pc_ui": "PC の窓の部品を読む", "pc_ui_do": "PC の窓の部品を押す・文字を入れる", "pc_keys": "PC でキーを打つ",
	"pc_open": "PC でファイル・アプリ・ページを開く", "pc_files": "PC のファイルを読む・探す", "pc_disk": "PC の容量を調べる",
	"pc_problems": "PC の不具合の記録を読む", "pc_processes": "PC で動いているものを見る", "pc_apps": "PC のアプリの一覧を見る",
	"pc_powershell": "PC で PowerShell の命令を動かす", "pc_trash": "PC のファイルをごみ箱へ送る（取り置きあり）",
	"hands":        "作業の区切り・まとめて戻す",
	"browser_tabs": "Chrome のタブを見る・開く・閉じる", "browser_read": "Chrome のページを読む", "browser_do": "Chrome のページを押す・文字を入れる",
	"browser_js": "Chrome のページで台本を動かす", "browser_screenshot": "Chrome のページを撮る",
}

func actionOf(tool string) string {
	if s, ok := toolAction[tool]; ok {
		return s
	}
	return "よく分からない操作（" + tool + "）"
}

// allowWhy：守りが止めた理由（「カオルの許可が要ります（ここ）」の中身。無ければ 1 行目）
func allowWhy(t string) string {
	const key = "カオルの許可が要ります（"
	if i := strings.Index(t, key); i >= 0 {
		r := t[i+len(key):]
		if j := strings.Index(r, "）。カオルに"); j > 0 { // 中に（ ）が入ることがある
			return r[:j]
		}
	}
	return firstLine(t)
}
