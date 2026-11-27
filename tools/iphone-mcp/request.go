package main

// Claude からの頼み：スイッチが足りないとき、カオルの PC に「許可しますか？」の窓を出してもらう。
// 道具は mode.txt を書かない。%USERPROFILE%\devkit\requests\<kind>.txt に頼み（JSON）を置くだけで、
// 窓を出して決めるのはスイッチの窓（claude-hands-switch.exe。無ければ予備の devkit-switch.ps1。カオルが押す）。
// 中身の決まりは tools/pc-mcp/request.go と同じ（what は道具が作る・why は今の作業の名前・返事は <kind>.answer.json）。

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
	"sync/atomic"
	"time"
	"unicode/utf8"
)

func requestDir() string { return filepath.Join(os.Getenv("USERPROFILE"), "devkit", "requests") }

type askRequest struct {
	Kind string    `json:"kind"`
	What string    `json:"what"`
	Why  string    `json:"why"`            // Claude の説明（道具の引数 why）
	Task string    `json:"task,omitempty"` // 今の作業の名前
	Tool string    `json:"tool,omitempty"`
	From string    `json:"from"`
	At   time.Time `json:"at"`
	Sig  string    `json:"sig,omitempty"` // 署名（合言葉の HMAC-SHA256）
}

// curTool：いま動いている道具の名前（timeline.go の受け口で入れる。窓の WHAT に使う）
var curTool atomic.Value

func currentTool() string {
	if s, ok := curTool.Load().(string); ok {
		return s
	}
	return ""
}

func canAsk() bool { return os.Getenv("USERPROFILE") != "" && os.Getenv("DEVKIT_NO_ASK") == "" }

// curWhy：いま動いている道具の引数 why（timeline.go の受け口で入れる）
var curWhy atomic.Value

func currentWhy() string {
	if s, ok := curWhy.Load().(string); ok {
		return s
	}
	return ""
}

// whyArg：道具の引数に足す「説明」（tools/pc-mcp/what.go と同じ決まり）
type whyArg struct {
	Why string `json:"why,omitempty" jsonschema:"カオルの許可の窓（スイッチが足りない）が出るときだけ要る、Claude の説明。この許可が続く間に何をするかを、ふつうの言葉で 100 字まで（記号の多い文・命令・URL は受け付けない）。窓に紫で出る"`
}

// checkWhy：説明として受け付けるか（tools/pc-mcp/what.go と同じ決まり）。だめなら Claude に返す文
func checkWhy(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return "この操作にはカオルの許可の窓が出ます。引数 why に、この許可で何をするかを、ふつうの言葉で 100 字までで書いて、呼び直してください（窓に「Claude の説明」として出ます）"
	}
	if n := utf8.RuneCountInString(s); n > 100 {
		return "why が長すぎます（" + strconv.Itoa(n) + " 字）。100 字までに縮めて呼び直してください"
	}
	if strings.ContainsAny(s, "\r\n\t") {
		return "why は 1 行で書いてください"
	}
	l := strings.ToLower(s)
	if strings.Contains(l, "http") || strings.Contains(l, "://") || strings.Contains(l, "www.") {
		return "why に URL は書けません。ふつうの言葉で書き直してください"
	}
	sym := 0
	for _, r := range s {
		if strings.ContainsRune("$%&*|\\/<>{}[]=;~^`\"#@", r) {
			sym++
		}
	}
	if sym >= 3 {
		return "why に記号が多すぎます（命令のように見える）。ふつうの言葉で書き直してください"
	}
	return ""
}

// askTask：今の作業の名前（%USERPROFILE%\devkit\hands\task.json の題名。無ければ空）
func askTask() string {
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
	if !canAsk() {
		return false
	}
	_ = os.MkdirAll(requestDir(), 0o755)
	_ = os.Remove(filepath.Join(requestDir(), kind+".answer.json"))
	r := askRequest{Kind: kind, What: what, Why: strings.TrimSpace(currentWhy()), Task: askTask(), Tool: currentTool(), From: "iphone-mcp " + version, At: time.Now()}
	r.sign()
	b, _ := json.Marshal(r)
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
		case "unsigned":
			return false, "頼みの署名が確かめられず、窓が頼みを捨てました（道具の版が古いか、合言葉 request.key が読めない）。カオルに伝えてください"
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
