package main

// Chrome の DevTools（CDP）とのやりとり。
// つなぎ方：Chrome 144 からの「自動接続」。カオルが chrome://inspect/#remote-debugging でオンにしておくと、
// Chrome が User Data の DevToolsActivePort に番号を書く。そこへつなぐと、Chrome がカオルに「許可しますか？」を出す。

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type cdpResp struct {
	ID     int64           `json:"id"`
	Result json.RawMessage `json:"result"`
	Error  *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
	Method    string `json:"method"`
	SessionID string `json:"sessionId"`
}

type CDP struct {
	ws      *wsConn
	next    atomic.Int64
	mu      sync.Mutex
	pending map[int64]chan cdpResp
	done    chan struct{}
	err     error
}

func newCDP(ws *wsConn) *CDP {
	c := &CDP{ws: ws, pending: map[int64]chan cdpResp{}, done: make(chan struct{})}
	go c.readLoop()
	return c
}

func (c *CDP) readLoop() {
	for {
		b, err := c.ws.ReadMessage()
		if err != nil {
			c.mu.Lock()
			c.err = err
			for _, ch := range c.pending {
				close(ch)
			}
			c.pending = map[int64]chan cdpResp{}
			c.mu.Unlock()
			close(c.done)
			return
		}
		var r cdpResp
		if json.Unmarshal(b, &r) != nil || r.ID == 0 {
			continue // 出来事（event）は使わない
		}
		c.mu.Lock()
		ch := c.pending[r.ID]
		delete(c.pending, r.ID)
		c.mu.Unlock()
		if ch != nil {
			ch <- r
		}
	}
}

func (c *CDP) Alive() bool {
	select {
	case <-c.done:
		return false
	default:
		return true
	}
}

func (c *CDP) Close() { _ = c.ws.Close() }

// Call は CDP の命令を 1 つ送って返事を待つ。session は空ならブラウザ全体。
func (c *CDP) Call(ctx context.Context, session, method string, params any, out any) error {
	id := c.next.Add(1)
	m := map[string]any{"id": id, "method": method}
	if params != nil {
		m["params"] = params
	}
	if session != "" {
		m["sessionId"] = session
	}
	b, _ := json.Marshal(m)
	ch := make(chan cdpResp, 1)
	c.mu.Lock()
	if c.pending == nil || !c.Alive() {
		c.mu.Unlock()
		return errBrowserGone
	}
	c.pending[id] = ch
	c.mu.Unlock()
	if err := c.ws.WriteText(b); err != nil {
		return errBrowserGone
	}
	select {
	case r, ok := <-ch:
		if !ok {
			return errBrowserGone
		}
		if r.Error != nil {
			return fmt.Errorf("%s：%s", method, r.Error.Message)
		}
		if out != nil && len(r.Result) > 0 {
			return json.Unmarshal(r.Result, out)
		}
		return nil
	case <-ctx.Done():
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
		return fmt.Errorf("%s：時間切れ", method)
	}
}

var errBrowserGone = errors.New("Chrome とのつながりが切れました（Chrome を閉じた・許可を取り消した）")

// chromeUserData は Chrome の User Data の場所（PC_BROWSER_USERDATA で変えられる。試験用）。
func chromeUserData() string {
	if v := env("PC_BROWSER_USERDATA", ""); v != "" {
		return v
	}
	return filepath.Join(os.Getenv("LOCALAPPDATA"), "Google", "Chrome", "User Data")
}

// browserWSURL は、DevToolsActivePort から ws の行き先を作る（PC_BROWSER_WS があればそれ）。
func browserWSURL(userData string) (string, error) {
	if v := env("PC_BROWSER_WS", ""); v != "" && userData == chromeUserData() {
		return v, nil
	}
	p := filepath.Join(userData, "DevToolsActivePort")
	b, err := os.ReadFile(p)
	if err != nil {
		return "", errNoRemoteDebug
	}
	lines := strings.Split(strings.ReplaceAll(string(b), "\r", ""), "\n")
	if len(lines) < 2 || strings.TrimSpace(lines[0]) == "" {
		return "", errNoRemoteDebug
	}
	path := strings.TrimSpace(lines[1])
	if path == "" {
		path = "/devtools/browser"
	}
	return "ws://127.0.0.1:" + strings.TrimSpace(lines[0]) + path, nil
}

var errNoRemoteDebug = errors.New("Chrome のリモートデバッグがオフです（DevToolsActivePort がない）。カオルに Chrome で chrome://inspect/#remote-debugging を開いて「リモートデバッグを許可」をオンにしてもらってください（1 回だけ。Chrome が起動していることも必要）")

// Browser は、常駐役が持ち続ける Chrome とのつながり（タブごとのセッションも覚える）。
type Browser struct {
	profile  string // claude（Claude 用の Chrome。道具が起動する）・kaoru（カオルの Chrome）
	mu       sync.Mutex
	cdp      *CDP
	sessions map[string]string // targetId → sessionId
	current  string            // 最後に使ったタブ
	opened   map[string]bool   // Claude が開いたタブ（閉じてよいのはこれだけ）
}

func (b *Browser) conn(ctx context.Context) (*CDP, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.cdp != nil && b.cdp.Alive() {
		return b.cdp, nil
	}
	if b.profile == profClaude {
		return b.connClaudeLocked(ctx)
	}
	u, err := browserWSURL(chromeUserData())
	if err != nil {
		return nil, err
	}
	ws, err := wsDial(u, 75*time.Second)
	if errors.Is(err, errConsentTimeout) {
		return nil, errors.New("Chrome に「リモートデバッグを許可しますか？」が出ています。75 秒で返事がありませんでした。カオルに Chrome の窓で「許可」を押してもらってから、もう一度呼んでください")
	}
	if err != nil {
		return nil, fmt.Errorf("Chrome につなげません（%v）。Chrome が起動しているか、chrome://inspect/#remote-debugging がオンかを見てください", err)
	}
	b.cdp = newCDP(ws)
	b.sessions = map[string]string{}
	return b.cdp, nil
}

// connClaudeLocked：Claude 用の Chrome につなぐ。動いていなければ起動する（許可の問い合わせは出ない）
func (b *Browser) connClaudeLocked(ctx context.Context) (*CDP, error) {
	try := func() (*wsConn, error) {
		u, err := browserWSURL(claudeChromeDir())
		if err != nil {
			return nil, err
		}
		return wsDial(u, 10*time.Second)
	}
	ws, err := try()
	if err != nil {
		if lerr := launchClaudeChrome(ctx); lerr != nil {
			return nil, lerr
		}
		if ws, err = try(); err != nil {
			return nil, fmt.Errorf("Claude 用の Chrome につなげません（%v）", err)
		}
	}
	b.cdp = newCDP(ws)
	b.sessions = map[string]string{}
	return b.cdp, nil
}

// Quit：Claude 用の Chrome を閉じる（カオルの Chrome はつながりを切るだけ）
func (b *Browser) Quit() {
	b.mu.Lock()
	c := b.cdp
	b.mu.Unlock()
	if b.profile == profClaude && c != nil && c.Alive() {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		_ = c.Call(ctx, "", "Browser.close", nil, nil)
		cancel()
	}
	b.Close()
}

func (b *Browser) Connected() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.cdp != nil && b.cdp.Alive()
}

func (b *Browser) Close() {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.cdp != nil {
		b.cdp.Close()
		b.cdp = nil
	}
	b.sessions = map[string]string{}
}

type targetInfo struct {
	TargetID string `json:"targetId"`
	Type     string `json:"type"`
	Title    string `json:"title"`
	URL      string `json:"url"`
	Attached bool   `json:"attached"`
}

func (b *Browser) pages(ctx context.Context) ([]targetInfo, error) {
	c, err := b.conn(ctx)
	if err != nil {
		return nil, err
	}
	var r struct {
		TargetInfos []targetInfo `json:"targetInfos"`
	}
	if err := c.Call(ctx, "", "Target.getTargets", map[string]any{"filter": []map[string]any{{"type": "page"}}}, &r); err != nil {
		// 古い形（filter なし）
		if err2 := c.Call(ctx, "", "Target.getTargets", nil, &r); err2 != nil {
			return nil, err
		}
	}
	var out []targetInfo
	for _, t := range r.TargetInfos {
		if t.Type == "page" && !strings.HasPrefix(t.URL, "devtools://") {
			out = append(out, t)
		}
	}
	return out, nil
}

// session は、そのタブにつないだ（使い回す）セッションを返す。
func (b *Browser) session(ctx context.Context, targetID string) (*CDP, string, error) {
	c, err := b.conn(ctx)
	if err != nil {
		return nil, "", err
	}
	b.mu.Lock()
	sid := b.sessions[targetID]
	b.mu.Unlock()
	if sid != "" {
		// 生きているか軽く確かめる
		var r any
		cctx, cancel := context.WithTimeout(ctx, 5*time.Second)
		err := c.Call(cctx, sid, "Runtime.evaluate", map[string]any{"expression": "1", "returnByValue": true}, &r)
		cancel()
		if err == nil {
			return c, sid, nil
		}
	}
	var r struct {
		SessionID string `json:"sessionId"`
	}
	if err := c.Call(ctx, "", "Target.attachToTarget", map[string]any{"targetId": targetID, "flatten": true}, &r); err != nil {
		return nil, "", fmt.Errorf("タブにつなげません（%v）", err)
	}
	b.mu.Lock()
	b.sessions[targetID] = r.SessionID
	b.current = targetID
	b.mu.Unlock()
	return c, r.SessionID, nil
}

// evalIn は、ページの中で JavaScript を動かして、値（JSON にできるもの）を返す。
func evalIn(ctx context.Context, c *CDP, sid, expr string, out any) error {
	return evalOpt(ctx, c, sid, expr, false, out)
}

// evalRepl は、コンソールと同じ書き方（文を並べてよい・一番上で await を使える・最後の値を返す）で動かす。
func evalRepl(ctx context.Context, c *CDP, sid, code string, out any) error {
	return evalOpt(ctx, c, sid, code, true, out)
}

func evalOpt(ctx context.Context, c *CDP, sid, expr string, repl bool, out any) error {
	var r struct {
		Result struct {
			Type  string          `json:"type"`
			Value json.RawMessage `json:"value"`
			Desc  string          `json:"description"`
		} `json:"result"`
		Exc *struct {
			Text      string `json:"text"`
			Exception *struct {
				Description string `json:"description"`
			} `json:"exception"`
		} `json:"exceptionDetails"`
	}
	err := c.Call(ctx, sid, "Runtime.evaluate", map[string]any{
		"expression": expr, "returnByValue": true, "awaitPromise": true, "userGesture": true, "allowUnsafeEvalBlockedByCSP": true, "replMode": repl,
	}, &r)
	if err != nil {
		return err
	}
	if r.Exc != nil {
		msg := r.Exc.Text
		if r.Exc.Exception != nil && r.Exc.Exception.Description != "" {
			msg = r.Exc.Exception.Description
		}
		return fmt.Errorf("ページの中でエラー：%s", firstLine(msg))
	}
	if out == nil {
		return nil
	}
	if len(r.Result.Value) == 0 {
		if r.Result.Desc != "" {
			b, _ := json.Marshal(r.Result.Desc)
			return json.Unmarshal(b, out)
		}
		return json.Unmarshal([]byte("null"), out)
	}
	return json.Unmarshal(r.Result.Value, out)
}

func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}
