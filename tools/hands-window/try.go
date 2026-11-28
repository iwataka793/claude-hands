package main

// TRY の窓（箱で試した結果。docs/ASK-DESIGN.md の 3「窓」・docs/PC.md「箱」）。
// 常駐役（pc_try）が devkit\hands\try\current.json に書いた試しを出し、カオルが［箱の中で開く］［本物に入れる］［捨てる］を押す。
// ［本物に入れる］は、窓が合言葉（request.key）で署名して常駐役に頼む（Claude からは頼めない）。

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	tryW      = 420
	tryH      = 300
	tryResFor = 20 * time.Second // 押したあとの結果を出しておく時間
)

type tryItem struct {
	Kind  string   `json:"kind"`
	Real  string   `json:"real"`
	Size  int64    `json:"size"`
	Lines []string `json:"lines"`
}

type tryRunRec struct {
	Exit    *int `json:"exit"`
	Running bool `json:"running"`
}

// TryRow：一覧の 1 行（［見る］で箱の中で開ける・変わった行）
type TryRow struct {
	Text  string   `json:"text"`
	Index int      `json:"index"` // ［見る］で開くもの（-1 は開けない）
	Sub   []string `json:"sub,omitempty"`
}

type tryState struct {
	ID       string      `json:"id"`
	Path     string      `json:"path"`
	Defender string      `json:"defender"`
	Running  bool        `json:"running"`
	Exit     string      `json:"exit"`
	Items    []tryItem   `json:"items"`
	Noise    int         `json:"noise"`
	Registry bool        `json:"registry"`
	Status   string      `json:"status"`
	Result   string      `json:"result"`
	Runs     []tryRunRec `json:"runs"`
}

// TryView：画面に出す中身
type TryView struct {
	ID       string   `json:"id"`
	File     string   `json:"file"`
	Defender string   `json:"defender"`
	Counts   string   `json:"counts"`
	Rows     []TryRow `json:"rows"`
	Notes    []string `json:"notes"`
	Armed    string   `json:"armed,omitempty"` // apply・discard（1 回押した）
	Msg      string   `json:"msg,omitempty"`
	Done     bool     `json:"done"`     // 押し終わった（結果だけ出す）
	Openable bool     `json:"openable"` // ［箱の中で開く］が使える（試したものがファイル）
}

func tryFile() string { return filepath.Join(handsDir(), "try", "current.json") }

func loadTry() *tryState {
	b, err := os.ReadFile(tryFile())
	if err != nil {
		return nil
	}
	var s tryState
	if json.Unmarshal(b, &s) != nil || s.ID == "" {
		return nil
	}
	return &s
}

func tryView(s *tryState) *TryView {
	v := &TryView{ID: s.ID, File: filepath.Base(s.Path), Defender: "Defender：" + s.Defender}
	if fi, err := os.Stat(s.Path); err == nil && !fi.IsDir() {
		v.Openable = true
	}
	n := map[string]int{}
	kw := map[string]string{"new": "増えた", "changed": "変わった", "deleted": "消えた", "admin": "管理者の場所（入れない）"}
	for _, it := range s.Items {
		n[it.Kind]++
	}
	v.Counts = fmt.Sprintf("増えた %d・変わった %d・消えた %d", n["new"], n["changed"], n["deleted"])
	if n["admin"] > 0 {
		v.Counts += fmt.Sprintf("・管理者の場所 %d（入れない）", n["admin"])
	}
	hd := strings.ToLower(home())
	for i, it := range s.Items {
		if i >= 8 {
			v.Rows = append(v.Rows, TryRow{Text: fmt.Sprintf("ほか %d", len(s.Items)-8), Index: -1})
			break
		}
		p := it.Real
		if strings.HasPrefix(strings.ToLower(p), hd+`\`) {
			p = "~" + p[len(hd):]
		}
		idx := i
		if it.Kind == "deleted" || it.Kind == "admin" {
			idx = -1
		}
		sub := it.Lines
		if len(sub) > 4 {
			sub = append(append([]string{}, sub[:4]...), "…")
		}
		v.Rows = append(v.Rows, TryRow{Text: kw[it.Kind] + "：" + p, Index: idx, Sub: sub})
	}
	if len(s.Items) == 0 {
		v.Rows = append(v.Rows, TryRow{Text: "本物に入れるファイルはありません", Index: -1})
	}
	if n := len(s.Runs); n > 0 {
		r := s.Runs[n-1]
		switch {
		case r.Running:
			v.Notes = append(v.Notes, fmt.Sprintf("Claude が箱で %d 回試した（最後の回はまだ動いている）", n))
		case r.Exit != nil && *r.Exit == 0:
			v.Notes = append(v.Notes, fmt.Sprintf("Claude が箱で %d 回試した（最後の回は成功で終わった）", n))
		case r.Exit != nil:
			v.Notes = append(v.Notes, fmt.Sprintf("Claude が箱で %d 回試した（最後の回は終わりのコード %d）", n, *r.Exit))
		}
	}
	if s.Running {
		v.Notes = append(v.Notes, "まだ箱の中で動いています")
	}
	if s.Registry {
		v.Notes = append(v.Notes, "レジストリの変更は箱の中だけ（本物には入れない）")
	}
	v.Notes = append(v.Notes, "ネットは切ってあります（ネットありでは試していません）")
	return v
}

// computeTry：TRY を出すか（返事待ちの試しがある・押したあとの結果を出している）
func (a *App) computeTry(st *LiveState) bool {
	if !a.tryResAt.IsZero() && time.Since(a.tryResAt) < tryResFor {
		st.Mode, st.Show, st.Compact = "try", true, false
		st.Try = &TryView{ID: a.tryID, File: a.tryFile, Msg: a.tryMsg, Done: true}
		return true
	}
	s := loadTry()
	if s == nil || s.Status != "waiting" {
		return false
	}
	if s.ID != a.trySeen {
		if a.trySeen != "" || a.started { // 出たときに 1 回だけ鳴らす
			winBeep()
		}
		a.trySeen, a.tryArmed, a.tryMsg = s.ID, "", ""
	}
	v := tryView(s)
	v.Armed, v.Msg = a.tryArmed, a.tryMsg
	a.tryID, a.tryFile = s.ID, v.File
	st.Mode, st.Show, st.Compact, st.Try = "try", true, false, v
	return true
}

func trySig(id, action string) string {
	p := os.Getenv("ProgramData")
	if p == "" {
		p = `C:\ProgramData`
	}
	kf := filepath.Join(p, "claude-hands", "keys", "request.key")
	if f := os.Getenv("HANDS_KEY_FILE"); f != "" {
		kf = f
	}
	b, err := os.ReadFile(kf)
	k := strings.TrimSpace(string(b))
	if err != nil || k == "" {
		return ""
	}
	m := hmac.New(sha256.New, []byte(k))
	m.Write([]byte(strings.Join([]string{"claude-hands-try-v1", id, action}, "\n")))
	return hex.EncodeToString(m.Sum(nil))
}

// TryOpen：［箱の中で開く］（ネットなしの箱で、カオルが使ってみる）
func (a *App) TryOpen() string {
	s := loadTry()
	if s == nil || s.Status != "waiting" {
		return ""
	}
	if fi, err := os.Stat(s.Path); err != nil || fi.IsDir() {
		return "Claude の台本の試しなので、開くものはありません（一覧の［見る］で中身を見られます）"
	}
	pf := os.Getenv("ProgramFiles")
	if pf == "" {
		pf = `C:\Program Files`
	}
	if err := startProc(filepath.Join(pf, "Sandboxie-Plus", "Start.exe"), "/box:ClaudeBox", s.Path); err != nil {
		return "箱の中で開けませんでした：" + err.Error()
	}
	msg := "箱の中で開きました（ネットなし・窓のふちが水色）。使い終わったら閉じて、［本物に入れる］か［捨てる］を押してください"
	a.mu.Lock()
	a.tryMsg, a.tryArmed = msg, ""
	a.mu.Unlock()
	return msg
}

// TryView：一覧の［見る］：そのファイルを箱の中で開く（箱の中では本物の場所が箱の中身に見える）
func (a *App) TryView(i int) string {
	s := loadTry()
	if s == nil || s.Status != "waiting" || i < 0 || i >= len(s.Items) {
		return ""
	}
	it := s.Items[i]
	if it.Kind == "deleted" || it.Kind == "admin" {
		return ""
	}
	pf := os.Getenv("ProgramFiles")
	if pf == "" {
		pf = `C:\Program Files`
	}
	if err := startProc(filepath.Join(pf, "Sandboxie-Plus", "Start.exe"), "/box:ClaudeBox", it.Real); err != nil {
		return "箱の中で開けませんでした：" + err.Error()
	}
	msg := filepath.Base(it.Real) + " を箱の中で開きました（ふちが水色の窓。ネットなし）"
	a.mu.Lock()
	a.tryMsg = msg
	a.mu.Unlock()
	return msg
}

// TryApply・TryDiscard：1 回目は「本当に？」にするだけ、2 回目で常駐役に頼む（カオルが押したと記録に残る）
func (a *App) TryApply() string   { return a.tryAct("apply") }
func (a *App) TryDiscard() string { return a.tryAct("discard") }

func (a *App) tryAct(action string) string {
	s := loadTry()
	if s == nil || s.Status != "waiting" {
		return ""
	}
	a.mu.Lock()
	if a.tryArmed != action {
		a.tryArmed = action
		msg := map[string]string{"apply": "もう一度押すと本物に入れます", "discard": "もう一度押すと捨てます（箱を空にする）"}[action]
		a.tryMsg = msg
		a.mu.Unlock()
		return msg
	}
	a.tryArmed, a.tryMsg = "", "…"
	a.mu.Unlock()
	sig := trySig(s.ID, action)
	if sig == "" {
		return a.tryResult(s, "合言葉（request.key）が読めないので頼めません。pc-setup.bat を動かしてください", false)
	}
	txt, ok := callAgent("pc_try", map[string]any{"action": action, "id": s.ID, "sig": sig, "by": "カオル（TRY の窓）"})
	return a.tryResult(s, txt, ok)
}

// tryResult：うまくいったら結果を少し出して閉じる。だめなら TRY のまま理由を出す
func (a *App) tryResult(s *tryState, msg string, ok bool) string {
	a.mu.Lock()
	defer a.mu.Unlock()
	if ok {
		a.tryResAt, a.tryID, a.tryFile, a.tryMsg = time.Now(), s.ID, filepath.Base(s.Path), msg
	} else {
		a.tryMsg = msg
	}
	return msg
}

// CloseTry：結果を閉じる（返事待ちの TRY は閉じられない）
func (a *App) CloseTry() {
	a.mu.Lock()
	a.tryResAt = time.Time{}
	a.mu.Unlock()
}

// FitTry：画面が TRY の中身の高さを知らせる
func (a *App) FitTry(h int) {
	a.mu.Lock()
	a.tryFit = h
	a.mu.Unlock()
}

func (a *App) tryHeight() int {
	if a.tryFit >= 120 && a.tryFit <= 520 {
		return a.tryFit
	}
	return tryH
}
