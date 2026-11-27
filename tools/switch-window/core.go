package main

// 中身（Wails・Win32 を使わない部分）：スイッチの読み書き・頼みの受け付け・返事・記録。
// 決まりは今までの devkit-switch.ps1 と同じ（mode.txt を書く・許可は UAC を通って allow.txt を書く）。

import (
	"bufio"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const (
	idleFor   = 30 * time.Minute // 使わないまま、これだけたつとオフ（オフにするのは道具の側）
	allowFor  = 10 * time.Minute // ALLOW の長さ
	askSecs   = 55               // 返事を待つ秒数（スイッチの頼み）
	allowSecs = 80               // 返事を待つ秒数（ALLOW の頼み）
	staleReq  = 2 * time.Minute  // これより古い頼みは捨てる（Claude がもう待っていない）
)

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

func devkit() string     { return filepath.Join(home(), "devkit") }
func requestDir() string { return filepath.Join(devkit(), "requests") }
func handsDir() string   { return filepath.Join(devkit(), "hands") }

func allowDir() string {
	if d := os.Getenv("HANDS_ALLOW_DIR"); d != "" {
		return d
	}
	p := os.Getenv("ProgramData")
	if p == "" {
		p = `C:\ProgramData`
	}
	return filepath.Join(p, "pc-agent")
}

// 機器：iPhone・PC・WEB（ブラウザ）
type device struct{ Key, Name, JP, Dir string }

func devices() []device {
	return []device{
		{"iphone", "iPhone", "スマホ", filepath.Join(devkit(), "iphone-mcp")},
		{"pc", "PC", "パソコン", filepath.Join(devkit(), "pc-agent")},
		{"web", "WEB", "ブラウザ（Chrome）", filepath.Join(devkit(), "browser")},
	}
}

func deviceOf(key string) (device, bool) {
	for _, d := range devices() {
		if d.Key == key {
			return d, true
		}
	}
	return device{}, false
}

func readMode(dir string) string {
	b, err := os.ReadFile(filepath.Join(dir, "mode.txt"))
	if err != nil {
		return "off"
	}
	m := strings.ToLower(strings.TrimSpace(strings.TrimPrefix(string(b), "\ufeff")))
	switch m {
	case "view", "operate":
		return m
	}
	return "off"
}

func writeMode(dir, mode string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "mode.txt"), []byte(mode+"\r\n"), 0o644)
}

func mtime(p string) time.Time {
	st, err := os.Stat(p)
	if err != nil {
		return time.Time{}
	}
	return st.ModTime()
}

// minutesLeft：最後の動き（切り替えた時刻か道具を使った時刻の新しい方）から 30 分の残り
func minutesLeft(dir string) int {
	last := mtime(filepath.Join(dir, "mode.txt"))
	if t := mtime(filepath.Join(dir, "last-used.txt")); t.After(last) {
		last = t
	}
	if last.IsZero() {
		return 0
	}
	l := idleFor - time.Since(last)
	if l <= 0 {
		return 0
	}
	return int((l + time.Minute - 1) / time.Minute)
}

// allowLeft：ALLOW の残り（無ければ 0）。PC かブラウザを切り替えたら取り消し（道具の側と同じ決まり）
func allowLeft() time.Duration {
	at := mtime(filepath.Join(allowDir(), "allow.txt"))
	if at.IsZero() {
		return 0
	}
	for _, k := range []string{"pc", "web"} {
		d, _ := deviceOf(k)
		if mtime(filepath.Join(d.Dir, "mode.txt")).After(at) {
			return 0
		}
	}
	l := allowFor - time.Since(at)
	if l < 0 {
		return 0
	}
	return l
}

// cancelAllow：ALLOW を取り消す（PC の mode.txt を同じ中身で書き直す。新しくなると道具は許可を取り消しと見る）
func cancelAllow() error {
	d, _ := deviceOf("pc")
	return writeMode(d.Dir, readMode(d.Dir))
}

// ---- 頼み ----

type askRequest struct {
	Kind string    `json:"kind"`
	What string    `json:"what"`
	Why  string    `json:"why"`
	Task string    `json:"task,omitempty"`
	Tool string    `json:"tool,omitempty"`
	From string    `json:"from"`
	At   time.Time `json:"at"`
	Sig  string    `json:"sig,omitempty"`

	Unsigned bool `json:"-"` // 合言葉が無くて署名を確かめられなかった
}

// ---- 頼みの署名 ----
// 道具（tools/pc-mcp・iphone-mcp の request.go）が合言葉で署名する。合言葉があれば、署名の合わない頼みは捨てる（偽の頼み）。
// 合言葉が無いとき（守りの場所をまだ作り直していない）は、署名を見ずに受け付けて、窓に「署名なし」と出す。

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

func requestKey() string {
	b, err := os.ReadFile(requestKeyFile())
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

func (r askRequest) signText() string {
	return strings.Join([]string{"claude-hands-request-v1", r.Kind, r.What, r.Why, r.Task, r.Tool, r.From, r.At.UTC().Format(time.RFC3339Nano)}, "\n")
}

func signWith(key string, r askRequest) string {
	m := hmac.New(sha256.New, []byte(key))
	m.Write([]byte(r.signText()))
	return hex.EncodeToString(m.Sum(nil))
}

// verify：署名が合うか。合言葉が無ければ "nokey"
func verify(r askRequest) string {
	key := requestKey()
	if key == "" {
		return "nokey"
	}
	if r.Sig == "" || !hmac.Equal([]byte(signWith(key, r)), []byte(r.Sig)) {
		return "bad"
	}
	if d := time.Since(r.At); d > staleReq || d < -time.Minute {
		return "bad"
	}
	return "ok"
}

// 頼みの種類：どの機器を・どこまで
type kindInfo struct {
	Device string // iphone・pc・web（ALLOW は pc）
	Mode   string // view・operate・allow
}

var kinds = map[string]kindInfo{
	"iphone-view": {"iphone", "view"}, "iphone-operate": {"iphone", "operate"},
	"pc-view": {"pc", "view"}, "pc-operate": {"pc", "operate"},
	"browser-view": {"web", "view"}, "browser-operate": {"web", "operate"},
	"pc-allow": {"pc", "allow"},
}

// ifAllowed：「許可すると」の文（窓の側が持つ。頼みからは受け取らない）
func ifAllowed(kind string) string {
	k := kinds[kind]
	if k.Mode == "allow" {
		return "10 分間、パスワード・セキュリティ・お金にかかわる操作が全部できるようになります。次に Windows の確認（UAC）が出ます。PC かブラウザのスイッチを切り替えると取り消されます。"
	}
	d, _ := deviceOf(k.Device)
	name := d.Name + " "
	if k.Device == "web" {
		name = "ブラウザ（Chrome）"
	}
	if k.Mode == "view" {
		return name + "が「見るだけ」になります（見る・読む・撮るだけ）。30 分使わなければ OFF に戻ります。"
	}
	return name + "が「操作も可」になります（押す・打つ・開くなど、ふつうの操作）。30 分使わなければ OFF に戻ります。"
}

type pending struct {
	askRequest
	Deadline time.Time
	Total    int
}

// nextRequest：いちばん古い頼みを取り出す（ファイルは消す）。古い・知らない種類は捨てる
func nextRequest() (*pending, int) {
	es, err := os.ReadDir(requestDir())
	if err != nil {
		return nil, 0
	}
	type f struct {
		name string
		t    time.Time
	}
	var fs []f
	for _, e := range es {
		if n := e.Name(); strings.HasSuffix(strings.ToLower(n), ".txt") && !e.IsDir() {
			if inf, err := e.Info(); err == nil {
				fs = append(fs, f{n, inf.ModTime()})
			}
		}
	}
	sort.Slice(fs, func(i, j int) bool { return fs[i].t.Before(fs[j].t) })
	for i, x := range fs {
		p := filepath.Join(requestDir(), x.name)
		b, _ := os.ReadFile(p)
		_ = os.Remove(p)
		kind := strings.ToLower(strings.TrimSuffix(x.name, filepath.Ext(x.name)))
		if _, ok := kinds[kind]; !ok {
			continue
		}
		age := time.Since(x.t)
		if age > staleReq {
			continue
		}
		var r askRequest
		txt := strings.TrimSpace(strings.TrimPrefix(string(b), "\ufeff"))
		if json.Unmarshal([]byte(txt), &r) != nil {
			r = askRequest{Why: txt} // 前の形（理由 1 行だけ）
		}
		mismatch := r.Kind != "" && r.Kind != kind // ファイルの名前と中身の種類が違う
		if r.Kind == "" {
			r.Kind = kind
		}
		switch v := verify(r); {
		case v == "bad" || mismatch:
			r.Kind = kind
			logAsk(&pending{askRequest: r}, "捨てた（署名が合わない）")
			writeAnswer(kind, "unsigned")
			continue
		case v == "nokey":
			r.Unsigned = true
		}
		r.Kind = kind
		total := askSecs
		if kinds[kind].Mode == "allow" {
			total = allowSecs
		}
		left := total - int(age.Seconds())
		if left <= 5 {
			writeAnswer(kind, "timeout")
			continue
		}
		if r.What == "" {
			r.What = defaultWhat(kind)
		}
		return &pending{askRequest: r, Deadline: time.Now().Add(time.Duration(left) * time.Second), Total: total}, len(fs) - i - 1
	}
	return nil, 0
}

func defaultWhat(kind string) string {
	k := kinds[kind]
	d, _ := deviceOf(k.Device)
	switch k.Mode {
	case "allow":
		return "パスワード・セキュリティ・お金にかかわる操作"
	case "view":
		return d.Name + " を見る"
	}
	return d.Name + " を操作する"
}

// countRequests：待っている頼みの数
func countRequests() int {
	es, _ := os.ReadDir(requestDir())
	n := 0
	for _, e := range es {
		if strings.HasSuffix(strings.ToLower(e.Name()), ".txt") {
			n++
		}
	}
	return n
}

func writeAnswer(kind, answer string) {
	_ = os.MkdirAll(requestDir(), 0o755)
	b, _ := json.Marshal(map[string]any{"answer": answer, "at": time.Now()})
	_ = os.WriteFile(filepath.Join(requestDir(), kind+".answer.json"), b, 0o644)
}

// logAsk：許可・断る・時間切れを記録に残す（devkit\hands\asks.jsonl）
func logAsk(p *pending, answer string) {
	_ = os.MkdirAll(handsDir(), 0o755)
	b, _ := json.Marshal(map[string]any{"t": time.Now(), "kind": p.Kind, "what": p.What, "why": p.Why, "task": p.Task, "tool": p.Tool, "from": p.From, "answer": answer, "by": "カオル（スイッチの窓）"})
	f, err := os.OpenFile(filepath.Join(handsDir(), "asks.jsonl"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	_, _ = f.Write(append(b, '\n'))
}

// ---- 最後の動き（窓の下の LOG） ----

var toolWords = map[string]string{
	"pc_powershell": "PowerShell を動かした", "pc_trash": "ごみ箱へ送った", "pc_open": "開いた", "pc_ui_do": "窓の部品を押した",
	"pc_keys": "キーを打った", "pc_ui": "窓の部品を読んだ", "pc_screenshot": "画面を撮った", "pc_files": "ファイルを読んだ",
	"pc_disk": "容量を調べた", "pc_problems": "不具合の記録を読んだ", "pc_processes": "動いているものを見た", "pc_apps": "アプリを見た",
	"pc_windows": "窓の一覧を見た", "hands": "作業の区切り",
	"browser_tabs": "タブ", "browser_read": "ページを読んだ", "browser_do": "ページを操作した", "browser_js": "ページで台本を動かした", "browser_screenshot": "ページを撮った",
	"iphone_tap": "押した", "iphone_tap_text": "押した", "iphone_type": "打った", "iphone_swipe": "なぞった", "iphone_steps": "まとめて操作した",
	"iphone_app": "アプリを開いた・閉じた", "iphone_screenshot": "画面を撮った", "iphone_elements": "画面の部品を読んだ", "iphone_info": "中の様子を見た",
	"iphone_files": "ファイル", "iphone_clipboard": "クリップボード", "iphone_logs": "ログを読んだ", "iphone_scroll_to": "スクロールした",
	"iphone_button": "ボタンを押した", "iphone_alert": "確認の窓", "iphone_web": "ページ", "iphone_location": "仮の位置",
}

func lastAction() string { t, _ := lastActionAt(); return t }

// lastActionAt：今日の TIMELINE の最後の行（ふつうの言葉と、その時刻）
func lastActionAt() (string, time.Time) {
	p := filepath.Join(handsDir(), "timeline", time.Now().Format("2006-01-02")+".jsonl")
	f, err := os.Open(p)
	if err != nil {
		return "", time.Time{}
	}
	defer f.Close()
	if st, err := f.Stat(); err == nil && st.Size() > 16<<10 {
		_, _ = f.Seek(st.Size()-16<<10, 0)
	}
	var last struct {
		T    time.Time `json:"t"`
		Hand string    `json:"hand"`
		Tool string    `json:"tool"`
	}
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		var e struct {
			T    time.Time `json:"t"`
			Hand string    `json:"hand"`
			Tool string    `json:"tool"`
		}
		if json.Unmarshal(sc.Bytes(), &e) == nil && !e.T.IsZero() {
			last = e
		}
	}
	if last.T.IsZero() {
		return "", time.Time{}
	}
	w := toolWords[last.Tool]
	if w == "" {
		w = last.Tool
	}
	hand := map[string]string{"pc": "PC", "browser": "WEB", "iphone": "iPhone"}[last.Hand]
	ago := time.Since(last.T)
	when := last.T.Format("15:04")
	switch {
	case ago < time.Minute:
		when = "いま"
	case ago < time.Hour:
		when = fmt.Sprintf("%d 分前", int(ago.Minutes()))
	}
	return hand + "：" + w + " · " + when, last.T
}

// ---- 下の帯（LIVE・IDLE）。LIVE の窓と同じ決まり：どれかのスイッチが入っていて、作業が開いているか 2 分以内に動いた ----

type LiveView struct {
	Live  bool   `json:"live"`
	Title string `json:"title"`
	Last  string `json:"last"`
}

func liveView(devs []DevView) LiveView {
	last, at := lastActionAt()
	v := LiveView{Last: last}
	var t struct {
		ID    string `json:"id"`
		Title string `json:"title"`
	}
	if b, err := os.ReadFile(filepath.Join(handsDir(), "task.json")); err == nil {
		_ = json.Unmarshal(b, &t)
	}
	on := false
	for _, d := range devs {
		if d.Mode != "off" {
			on = true
		}
	}
	v.Title = t.Title
	if v.Title == "" {
		v.Title = "（作業の区切りなし）"
	}
	v.Live = on && (t.ID != "" || (!at.IsZero() && time.Since(at) < 2*time.Minute))
	return v
}

// ---- 画面に渡す形 ----

type DevView struct {
	Key  string `json:"key"`
	Name string `json:"name"`
	JP   string `json:"jp"`
	Mode string `json:"mode"`
	Left int    `json:"left"` // 自動オフまでの分
}

type AskView struct {
	Kind   string `json:"kind"`
	Head   string `json:"head"`   // VIEW・OPERATE・ALLOW
	Target string `json:"target"` // iPhone・PC・WEB
	What   string `json:"what"`
	Why    string `json:"why"`
	Task   string `json:"task"`
	If     string `json:"if"`
	Secs   int    `json:"secs"`
	Total  int    `json:"total"`
	Others int    `json:"others"`
	NoSig  bool   `json:"noSig"` // 署名を確かめられなかった（合言葉が無い）
}

type View struct {
	Devs      []DevView `json:"devs"`
	AllowSecs int       `json:"allowSecs"`
	Ask       *AskView  `json:"ask"`
	Last      string    `json:"last"`
	Live      LiveView  `json:"live"`
	Msg       string    `json:"msg"`
	Version   string    `json:"version"`
}

func devViews() []DevView {
	var out []DevView
	for _, d := range devices() {
		m := readMode(d.Dir)
		v := DevView{Key: d.Key, Name: d.Name, JP: d.JP, Mode: m}
		if m != "off" {
			v.Left = minutesLeft(d.Dir)
		}
		out = append(out, v)
	}
	return out
}

func askView(p *pending, others int) *AskView {
	if p == nil {
		return nil
	}
	k := kinds[p.Kind]
	d, _ := deviceOf(k.Device)
	secs := int(time.Until(p.Deadline).Seconds())
	if secs < 0 {
		secs = 0
	}
	return &AskView{Kind: p.Kind, Head: strings.ToUpper(k.Mode), Target: d.Name, What: p.What, Why: p.Why, Task: p.Task, If: ifAllowed(p.Kind),
		Secs: secs, Total: p.Total, Others: others, NoSig: p.Unsigned}
}

// trayTip：通知領域の文字（63 字まで）
func trayTip(devs []DevView, allow bool) string {
	var parts []string
	for _, d := range devs {
		parts = append(parts, d.Name+"："+strings.ToUpper(d.Mode))
	}
	s := strings.Join(parts, " / ")
	if allow {
		s += " · ALLOW"
	}
	r := []rune(s)
	if len(r) > 63 {
		r = r[:63]
	}
	return string(r)
}

// strongest：通知領域の丸の色（いちばん強いスイッチ）
func strongest(devs []DevView, allow bool) string {
	if allow {
		return "allow"
	}
	best := "off"
	for _, d := range devs {
		if d.Mode == "operate" {
			return "operate"
		}
		if d.Mode == "view" {
			best = "view"
		}
	}
	return best
}

func errorf(f string, a ...any) error { return fmt.Errorf(f, a...) }
