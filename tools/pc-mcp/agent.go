package main

// 常駐役：カオルの .bat で起動する。PC の中（127.0.0.1）だけで受け付け、合言葉（token.txt）がないと答えない。
// スイッチがオフになる／使わない時間が続くと、自分で終わる。

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

// 道具の答え（拡張機能へ返す形）
type Reply struct {
	Text    string  `json:"text"`
	Images  []Image `json:"images,omitempty"`
	IsError bool    `json:"isError,omitempty"`
}

type Image struct {
	Data []byte `json:"data"`
	MIME string `json:"mime"`
}

func okText(s string) Reply            { return Reply{Text: s} }
func errText(f string, a ...any) Reply { return Reply{Text: fmt.Sprintf(f, a...), IsError: true} }
func okTextf(f string, a ...any) Reply { return Reply{Text: fmt.Sprintf(f, a...)} }
func (r Reply) withImage(b []byte, m string) Reply {
	r.Images = append(r.Images, Image{b, m})
	return r
}

type Agent struct {
	st    *State
	ps    *Runner
	token string
	br    *Browser // Chrome とのつながり（ブラウザの道具を最初に使ったときにつなぐ）
	bst   *State   // ブラウザのスイッチ（PC とは別。%USERPROFILE%\devkit\browser）
}

func isBrowserTool(t string) bool { return strings.HasPrefix(t, "browser_") }

// 道具ごとに要るスイッチ
var toolNeed = map[string]Mode{
	"pc_status": ModeOff,
	"pc_disk":   ModeView, "pc_problems": ModeView, "pc_processes": ModeView, "pc_apps": ModeView,
	"pc_files": ModeView, "pc_screenshot": ModeView, "pc_windows": ModeView, "pc_ui": ModeView,
	"pc_powershell": ModeView, // 中で「見るだけ」か「操作」かを分ける
	"pc_open":       ModeOperate, "pc_ui_do": ModeOperate, "pc_keys": ModeOperate, "pc_trash": ModeOperate,
	// ブラウザ（Chrome）
	"browser_tabs": ModeView, // 一覧は見るだけ、開く・前に出す・閉じるは中で「操作も可」を確かめる
	"browser_read": ModeView, "browser_screenshot": ModeView,
	"browser_do": ModeOperate, "browser_js": ModeOperate,
}

func runAgent() {
	dir := stateDir()
	_ = os.MkdirAll(dir, 0o755)
	if f, err := os.OpenFile(dir+string(os.PathSeparator)+"agent.log", os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644); err == nil {
		slog.SetDefault(slog.New(slog.NewTextHandler(f, nil)))
	}
	ln, err := net.Listen("tcp", agentAddr)
	if err != nil {
		slog.Info("already running or port busy", "err", err)
		return // もう動いている
	}
	idleMin, _ := strconv.Atoi(env("PC_AGENT_IDLE_MINUTES", "30"))
	if idleMin <= 0 {
		idleMin = 30
	}
	b := make([]byte, 24)
	_, _ = rand.Read(b)
	a := &Agent{st: NewState(dir, time.Duration(idleMin)*time.Minute), ps: NewRunner(), token: hex.EncodeToString(b)}
	a.bst = NewState(browserDir(), time.Duration(idleMin)*time.Minute)
	a.st.revokers = []string{a.bst.path("mode.txt")}
	if err := os.WriteFile(a.st.path("token.txt"), []byte(a.token), 0o600); err != nil {
		slog.Error("token", "err", err)
		return
	}
	slog.Info("agent start", "version", version, "dir", dir, "pid", os.Getpid())
	a.st.Audit("（起動）", "版 "+version, "ok", 0)

	srv := &http.Server{Handler: http.HandlerFunc(a.handle), ReadHeaderTimeout: 10 * time.Second}
	go func() {
		// PC もブラウザもオフ（か使わない時間が続いた）→ 終わる。ブラウザだけオフ → Chrome とのつながりを切る
		for range time.Tick(2 * time.Second) {
			pcOn := !a.st.IdleExpired() && a.st.Mode() != ModeOff
			brOn := !a.bst.IdleExpired() && a.bst.Mode() != ModeOff
			if !brOn && a.br != nil && a.br.Connected() {
				a.br.Close()
				a.st.Audit("（ブラウザ）", "ブラウザのスイッチがオフなので Chrome とのつながりを切った", "ok", 0)
			}
			if !pcOn && !brOn {
				a.st.Audit("（終了）", "スイッチがオフ", "ok", 0)
				ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
				_ = srv.Shutdown(ctx)
				cancel()
				return
			}
		}
	}()
	_ = srv.Serve(ln)
	a.ps.Close()
	if a.br != nil {
		a.br.Close()
	}
	_ = os.Remove(a.st.path("token.txt"))
	slog.Info("agent stop")
}

func (a *Agent) handle(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost || r.URL.Path != "/call" || r.Header.Get("Authorization") != "Bearer "+a.token {
		http.Error(w, "no", http.StatusForbidden)
		return
	}
	var req struct {
		Tool string          `json:"tool"`
		Args json.RawMessage `json:"args"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<20)).Decode(&req); err != nil {
		http.Error(w, "bad", http.StatusBadRequest)
		return
	}
	start := time.Now()
	rep := a.call(r.Context(), req.Tool, req.Args)
	res := "ok"
	if rep.IsError {
		first := strings.SplitN(rep.Text, "\n", 2)[0]
		res = "NG: " + first
	}
	a.st.Audit(req.Tool, string(req.Args), res, time.Since(start))
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(rep)
}

func (a *Agent) call(ctx context.Context, tool string, raw json.RawMessage) Reply {
	need, ok := toolNeed[tool]
	if !ok {
		return errText("知らない道具：%s", tool)
	}
	if isBrowserTool(tool) {
		m := a.bst.Mode()
		if m == ModeOff {
			return errText("ブラウザのスイッチがオフです。カオルにスイッチの窓で「ブラウザ」を「見るだけ」か「操作も可」にしてもらってください（切り替えはカオルだけ）")
		}
		if need == ModeOperate && m != ModeOperate {
			return errText("ブラウザのスイッチが「見るだけ」なので操作できません。要るならカオルにスイッチの窓で「ブラウザ」を「操作も可」にしてもらってください")
		}
		a.bst.Touch()
	} else {
		m := a.st.Mode()
		if need > ModeOff {
			if m == ModeOff {
				return errText("PC のスイッチがオフです。カオルに PC-VIEW.bat か PC-OPERATE.bat を押してもらってください（切り替えはカオルだけ）")
			}
			if need == ModeOperate && m != ModeOperate {
				return errText("スイッチが「見るだけ」なので操作できません。要るならカオルに PC-OPERATE.bat を押してもらってください")
			}
			a.st.Touch()
		}
	}
	if len(raw) == 0 || string(raw) == "null" {
		raw = []byte("{}")
	}
	switch tool {
	case "pc_status":
		return a.status()
	case "pc_disk":
		return a.disk(ctx, raw)
	case "pc_problems":
		return a.problems(ctx, raw)
	case "pc_processes":
		return a.processes(ctx, raw)
	case "pc_apps":
		return a.apps(ctx, raw)
	case "pc_files":
		return a.files(ctx, raw)
	case "pc_screenshot":
		return a.screenshot(ctx, raw)
	case "pc_windows":
		return a.windows(ctx, raw)
	case "pc_ui":
		return a.ui(ctx, raw)
	case "pc_powershell":
		return a.powershell(ctx, raw)
	case "pc_open":
		return a.open(ctx, raw)
	case "pc_ui_do":
		return a.uiDo(ctx, raw)
	case "pc_keys":
		return a.keys(ctx, raw)
	case "pc_trash":
		return a.trash(ctx, raw)
	case "browser_tabs":
		return a.brTabs(ctx, raw)
	case "browser_read":
		return a.brRead(ctx, raw)
	case "browser_do":
		return a.brDo(ctx, raw)
	case "browser_js":
		return a.brJS(ctx, raw)
	case "browser_screenshot":
		return a.brShot(ctx, raw)
	}
	return errText("知らない道具：%s", tool)
}

func (a *Agent) chromeState() string {
	if a.br != nil && a.br.Connected() {
		return "つながっている"
	}
	return "つながっていない"
}

func (a *Agent) status() Reply {
	allow, left := a.st.Allowed()
	al := "なし"
	if allow {
		al = fmt.Sprintf("あり（残り %d 分）", int(left.Minutes())+1)
	}
	lines := []string{
		"常駐役：動いている（版 " + version + "）",
		"スイッチ：" + a.st.Mode().String(),
		"ブラウザのスイッチ：" + a.bst.Mode().String() + "（Chrome：" + a.chromeState() + "）",
		"許可（ALLOW）：" + al + "（" + allowFile() + "）",
		fmt.Sprintf("自動オフまで（使わない時間）：%d 分", int(a.st.idle.Minutes())),
		"記録：" + a.st.path("audit.log"),
		"最近の記録：",
	}
	for _, l := range a.st.AuditTail(8) {
		lines = append(lines, "  "+l)
	}
	return okText(strings.Join(lines, "\n"))
}

// needAllowFor は、場所の種類から「止める／許可が要る」を決める。modify=true は変える操作。
func (a *Agent) needAllowFor(path string, modify bool) *Reply {
	cls, why := pathClass(path)
	switch cls {
	case "switch":
		r := errText("%s（%s）は触れません", path, why)
		return &r
	case "sensitive":
		if ok, _ := a.st.Allowed(); !ok {
			r := errText("%s：%s", path, allowMsg(why))
			return &r
		}
	case "system":
		if modify {
			if ok, _ := a.st.Allowed(); !ok {
				r := errText("%s：%s", path, allowMsg(why))
				return &r
			}
		}
	}
	return nil
}

func decode[T any](raw json.RawMessage) (T, error) {
	var v T
	err := json.Unmarshal(raw, &v)
	return v, err
}
