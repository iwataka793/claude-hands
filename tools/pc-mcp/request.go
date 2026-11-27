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
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
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
	Why  string    `json:"why"`            // Claude の説明（道具の引数 why）
	Task string    `json:"task,omitempty"` // 今の作業の名前（hands start の題名）
	Tool string    `json:"tool,omitempty"`
	From string    `json:"from"`
	At   time.Time `json:"at"`
	Sig  string    `json:"sig,omitempty"` // 署名（合言葉の HMAC-SHA256）
}

type askAnswer struct {
	Answer string    `json:"answer"` // allow・deny・timeout
	At     time.Time `json:"at"`
}

// canAsk：窓を出してもらえるか（Windows でない・試験で DEVKIT_NO_ASK が付いているときは頼まない）
func canAsk() bool { return os.Getenv("USERPROFILE") != "" && os.Getenv("DEVKIT_NO_ASK") == "" }

// askFor：窓を出す前に Claude の説明（why）を確かめてから頼む。
// 返り値：頼んだか・頼まなかった理由（説明が無い・だめ。Claude に「書いて呼び直して」と返す）
func askFor(kind, tool, what string, in any) (bool, string) {
	if !canAsk() {
		return false, ""
	}
	why := whyOf(in)
	if msg := checkWhy(why); msg != "" {
		return false, msg
	}
	return askKaoru(kind, tool, what, strings.TrimSpace(why)), ""
}

// whyOf：道具の引数から why を取り出す
func whyOf(in any) string {
	b, _ := json.Marshal(in)
	var m struct {
		Why string `json:"why"`
	}
	_ = json.Unmarshal(b, &m)
	return m.Why
}

// allowWhat：許可の窓の WHAT。常駐役が言い換えの表で作ったもの（無ければ道具の短い言い方）
func allowWhat(tool string, rep Reply) string {
	if rep.What != "" {
		return rep.What
	}
	return actionOf(tool) + "（許可が要るもの）"
}

// askTask：今の作業の名前（無ければ空）
func askTask() string {
	if t := currentTask(); t != nil {
		return t.Title
	}
	return ""
}

// askKaoru は頼みを置き、スイッチの窓が動いていなければ起動する（動いていれば何もしないで終わる）。
// 頼めないとき（Windows でない・試験で DEVKIT_NO_ASK が付いている）は false。
func askKaoru(kind, tool, what, why string) bool {
	if !canAsk() {
		return false
	}
	_ = os.MkdirAll(requestDir(), 0o755)
	_ = os.Remove(filepath.Join(requestDir(), kind+".answer.json")) // 前の返事は捨てる
	r := askRequest{Kind: kind, What: what, Why: why, Task: askTask(), Tool: tool, From: "pc-mcp " + version, At: time.Now()}
	r.sign()
	b, _ := json.Marshal(r)
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
		case "unsigned":
			return false, "頼みの署名が確かめられず、窓が頼みを捨てました（道具の版が古いか、合言葉 request.key が読めない）。カオルに伝えてください"
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

// ---- 頼みの署名（偽の頼みを窓に出させない） ----
// 合言葉は %ProgramData%\claude-hands\keys\request.key（管理者が作る。カオルの権限では読むだけ。文字の守りで Claude の台本からは読ませない）。
// 窓（tools/switch-window）は合言葉があれば、署名の合わない頼みを捨てる。

func requestKeyFile() string {
	if f := os.Getenv("HANDS_KEY_FILE"); f != "" {
		return f
	}
	p := os.Getenv("ProgramData")
	if p == "" {
		p = `C:\ProgramData`
	}
	return filepath.Join(p, "claude-hands", "keys", "request.key")
}

// signText：署名する中身（窓の側と同じ並び）
func (r askRequest) signText() string {
	return strings.Join([]string{"claude-hands-request-v1", r.Kind, r.What, r.Why, r.Task, r.Tool, r.From, r.At.UTC().Format(time.RFC3339Nano)}, "\n")
}

func (r *askRequest) sign() {
	key, err := os.ReadFile(requestKeyFile())
	k := strings.TrimSpace(string(key))
	if err != nil || k == "" {
		return
	}
	m := hmac.New(sha256.New, []byte(k))
	m.Write([]byte(r.signText()))
	r.Sig = hex.EncodeToString(m.Sum(nil))
}
