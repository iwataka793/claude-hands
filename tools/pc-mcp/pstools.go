package main

// PowerShell の台本で動く道具。台本は scripts/*.ps1（exe に埋め込む）。

import (
	"context"
	"embed"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"
)

//go:embed scripts/*.ps1
var scriptFS embed.FS

func script(name string) string {
	b, err := scriptFS.ReadFile("scripts/" + name)
	if err != nil {
		panic(err)
	}
	return string(b)
}

// 画面や入力を使う台本は、共通の部品（common.ps1）を先に読む
func uiScript(name string) string { return script("common.ps1") + "\n" + script(name) }

var scriptDrives = script("drives.ps1")

func b64(b []byte) string { return base64.StdEncoding.EncodeToString(b) }

// psReply は台本の出力を答えにする（"ERROR: " で始まる行があればエラー扱い）。
func psReply(out string, err error) Reply {
	if err != nil {
		if out != "" {
			return errText("%v\n%s", err, out)
		}
		return errText("%v", err)
	}
	if strings.HasPrefix(strings.TrimSpace(out), "ERROR: ") {
		return errText("%s", strings.TrimPrefix(strings.TrimSpace(out), "ERROR: "))
	}
	if strings.TrimSpace(out) == "" {
		out = "（出力なし）"
	}
	return okText(out)
}

func (a *Agent) problems(ctx context.Context, raw json.RawMessage) Reply {
	var in map[string]any
	_ = json.Unmarshal(raw, &in)
	return psReply(a.ps.Run(ctx, script("problems.ps1"), in, 180*time.Second))
}

func (a *Agent) processes(ctx context.Context, raw json.RawMessage) Reply {
	var in map[string]any
	_ = json.Unmarshal(raw, &in)
	return psReply(a.ps.Run(ctx, script("processes.ps1"), in, 60*time.Second))
}

func (a *Agent) apps(ctx context.Context, raw json.RawMessage) Reply {
	var in map[string]any
	_ = json.Unmarshal(raw, &in)
	return psReply(a.ps.Run(ctx, script("apps.ps1"), in, 90*time.Second))
}

func (a *Agent) windows(ctx context.Context, raw json.RawMessage) Reply {
	var in map[string]any
	_ = json.Unmarshal(raw, &in)
	return psReply(a.ps.Run(ctx, uiScript("windows.ps1"), in, 60*time.Second))
}

func (a *Agent) ui(ctx context.Context, raw json.RawMessage) Reply {
	var in map[string]any
	_ = json.Unmarshal(raw, &in)
	if w, _ := in["window"].(string); w != "" && sensitiveWindowRe.MatchString(w) {
		if ok, _ := a.st.Allowed(); !ok {
			return errText("窓「%s」：%s", w, allowMsg("パスワード・セキュリティ・お金にかかわりそうな窓"))
		}
	}
	return psReply(a.ps.Run(ctx, uiScript("ui.ps1"), in, 90*time.Second))
}

// ---- 画面 ----

type shotInfo struct {
	Left, Top, Width, Height int
	Factor                   int
}

var (
	lastShotMu sync.Mutex
	lastShot   shotInfo
)

func (a *Agent) screenshot(ctx context.Context, raw json.RawMessage) Reply {
	var in struct {
		Window   string `json:"window"`
		Activate bool   `json:"activate"`
		Full     bool   `json:"full"`
		MaxWidth int    `json:"max_width"`
	}
	_ = json.Unmarshal(raw, &in)
	out, err := a.ps.Run(ctx, uiScript("screenshot.ps1"), in, 60*time.Second)
	if err != nil || strings.HasPrefix(strings.TrimSpace(out), "ERROR: ") {
		return psReply(out, err)
	}
	var s struct {
		Left, Top, Width, Height int
		Title                    string
		PNG                      string `json:"png"`
	}
	if err := json.Unmarshal([]byte(strings.ReplaceAll(strings.ReplaceAll(out, "\n", ""), "\r", "")), &s); err != nil {
		return errText("画面の結果が読めません：%v", err)
	}
	png, err := base64.StdEncoding.DecodeString(s.PNG)
	if err != nil {
		return errText("画面の結果が読めません：%v", err)
	}
	if in.MaxWidth <= 0 {
		in.MaxWidth = 1280
	}
	factor := 1
	for s.Width/factor > in.MaxWidth {
		factor++
	}
	if in.Full {
		factor = 1
	}
	img, mime := png, "image/png"
	if factor > 1 || !in.Full {
		if j, _, _, err := shrink(png, factor, 80); err == nil {
			img, mime = j, "image/jpeg"
		}
	}
	lastShotMu.Lock()
	lastShot = shotInfo{s.Left, s.Top, s.Width, s.Height, factor}
	lastShotMu.Unlock()
	what := "画面全体"
	if s.Title != "" {
		what = "窓「" + s.Title + "」"
	}
	msg := fmt.Sprintf("%s（%dx%d を 1/%d に縮めた）。画像の (x, y) は画面の (%d + x×%d, %d + y×%d)。pc_ui_do の click_xy で from_image: true にすると、画像の座標のまま押せる",
		what, s.Width, s.Height, factor, s.Left, factor, s.Top, factor)
	return okText(msg).withImage(img, mime)
}

// ---- PowerShell ----

type psIn struct {
	Script  string `json:"script"`
	Seconds int    `json:"seconds"`
	Max     int    `json:"max_chars"`
}

func (a *Agent) inspect(ctx context.Context, src string) (scriptInfo, error) {
	out, err := a.ps.Run(ctx, script("astinfo.ps1"), map[string]string{"src": src}, 30*time.Second)
	if err != nil {
		return scriptInfo{}, err
	}
	var info scriptInfo
	if err := json.Unmarshal([]byte(strings.TrimSpace(strings.ReplaceAll(out, "\n", ""))), &info); err != nil {
		return scriptInfo{}, fmt.Errorf("台本を調べた結果が読めません：%v（%s）", err, clip(out, 200))
	}
	return info, nil
}

func (a *Agent) powershell(ctx context.Context, raw json.RawMessage) Reply {
	in, err := decode[psIn](raw)
	if err != nil || strings.TrimSpace(in.Script) == "" {
		return errText("script（PowerShell の台本）を指定してください")
	}
	info, err := a.inspect(ctx, in.Script)
	if err != nil {
		return errText("%v", err)
	}
	block, needAllow, viewNG := checkScript(info, in.Script)
	if block != "" {
		return errText("%s", block)
	}
	if a.st.Mode() != ModeOperate && viewNG != "" {
		return errText("スイッチが「見るだけ」なので、この台本は使えません（%s）。見るだけで使えるのは Get- / Test- / Measure- / Select- などの読む命令だけ。要るならカオルに PC-OPERATE.bat を押してもらう", viewNG)
	}
	if needAllow != "" {
		if ok, _ := a.st.Allowed(); !ok {
			return errText("%s", allowMsg(needAllow))
		}
	}
	if in.Seconds <= 0 {
		in.Seconds = 60
	}
	if in.Seconds > 600 {
		in.Seconds = 600
	}
	if in.Max <= 0 {
		in.Max = 30000
	}
	out, err := a.ps.Run(ctx, in.Script, nil, time.Duration(in.Seconds)*time.Second)
	if r := []rune(out); len(r) > in.Max {
		out = string(r[:in.Max]) + fmt.Sprintf("\n（出力が長いので %d 文字で切った。全部で %d 文字）", in.Max, len(r))
	}
	if err != nil {
		return errText("%v\n%s", err, out)
	}
	if strings.TrimSpace(out) == "" {
		out = "（出力なし）"
	}
	return okText(out)
}

// ---- 動かす ----

func (a *Agent) open(ctx context.Context, raw json.RawMessage) Reply {
	var in struct {
		Target string `json:"target"`
		Args   string `json:"args"`
	}
	_ = json.Unmarshal(raw, &in)
	if in.Target == "" {
		return errText("target（開くもの：アプリ名・ファイル・フォルダ・URL）を指定してください")
	}
	if !strings.Contains(in.Target, "://") {
		if r := a.needAllowFor(in.Target, false); r != nil {
			return *r
		}
	}
	if m := sensitiveWindowRe.FindString(in.Target + " " + in.Args); m != "" || strings.Contains(strings.ToLower(in.Args), "runas") {
		if ok, _ := a.st.Allowed(); !ok {
			return errText("%s", allowMsg("パスワード・セキュリティ・お金にかかわりそうなもの（"+m+"）"))
		}
	}
	return psReply(a.ps.Run(ctx, script("open.ps1"), in, 60*time.Second))
}

func (a *Agent) checkWindow(ctx context.Context, window string) *Reply {
	// 指定が無いときは、いま前にある窓を調べる
	name := window
	if name == "" {
		out, err := a.ps.Run(ctx, uiScript("windows.ps1"), map[string]any{}, 30*time.Second)
		if err == nil {
			for _, l := range strings.Split(out, "\n") {
				if strings.Contains(l, "← いま前") {
					name = l
				}
			}
		}
	}
	if m := sensitiveWindowRe.FindString(name); m != "" {
		if ok, _ := a.st.Allowed(); !ok {
			r := errText("窓「%s」：%s", strings.TrimSpace(name), allowMsg("パスワード・セキュリティ・お金にかかわりそうな窓（"+m+"）"))
			return &r
		}
	}
	return nil
}

func (a *Agent) uiDo(ctx context.Context, raw json.RawMessage) Reply {
	var in map[string]any
	_ = json.Unmarshal(raw, &in)
	w, _ := in["window"].(string)
	if r := a.checkWindow(ctx, w); r != nil {
		return *r
	}
	if fi, _ := in["from_image"].(bool); fi {
		lastShotMu.Lock()
		s := lastShot
		lastShotMu.Unlock()
		if s.Factor == 0 {
			return errText("先に pc_screenshot で画面を撮ってください（from_image は最後に撮った画像の座標）")
		}
		x, _ := in["x"].(float64)
		y, _ := in["y"].(float64)
		in["x"] = float64(s.Left) + x*float64(s.Factor)
		in["y"] = float64(s.Top) + y*float64(s.Factor)
	}
	return psReply(a.ps.Run(ctx, uiScript("uido.ps1"), in, 60*time.Second))
}

func (a *Agent) keys(ctx context.Context, raw json.RawMessage) Reply {
	var in map[string]any
	_ = json.Unmarshal(raw, &in)
	w, _ := in["window"].(string)
	if r := a.checkWindow(ctx, w); r != nil {
		return *r
	}
	return psReply(a.ps.Run(ctx, uiScript("keys.ps1"), in, 60*time.Second))
}

func (a *Agent) trash(ctx context.Context, raw json.RawMessage) Reply {
	var in struct {
		Paths []string `json:"paths"`
	}
	_ = json.Unmarshal(raw, &in)
	if len(in.Paths) == 0 {
		return errText("paths（ごみ箱に送るファイル・フォルダ）を指定してください")
	}
	var ps []string
	for _, p := range in.Paths {
		n := normPath(p)
		if r := a.needAllowFor(n, true); r != nil {
			return *r
		}
		ps = append(ps, n)
	}
	return psReply(a.ps.Run(ctx, script("trash.ps1"), map[string]any{"paths": ps}, 300*time.Second))
}
