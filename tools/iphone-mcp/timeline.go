package main

// TIMELINE：iPhone の道具の呼び出しを、PC・ブラウザと同じ 1 本の記録に残す（tools/pc-mcp/hands.go と同じ形）。
// 置き場所 %USERPROFILE%\devkit\hands\timeline\日付.jsonl。今の作業（task.json）があれば、その番号も付ける。

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type tlEntry struct {
	T    time.Time `json:"t"`
	Hand string    `json:"hand"`
	Tool string    `json:"tool"`
	Args string    `json:"args,omitempty"`
	OK   bool      `json:"ok"`
	Res  string    `json:"res,omitempty"`
	Ms   int64     `json:"ms"`
	Task string    `json:"task,omitempty"`
	Lane string    `json:"lane,omitempty"` // 作業の道 A〜E（tools/pc-mcp/lane.go と同じ）
}

var tlMu sync.Mutex

func handsUserDir() string {
	return env("PC_HANDS_USERDIR", filepath.Join(os.Getenv("USERPROFILE"), "devkit", "hands"))
}

func oneLine(s string, n int) string {
	s = strings.ReplaceAll(strings.ReplaceAll(s, "\r", " "), "\n", " ⏎ ")
	if r := []rune(s); len(r) > n {
		return string(r[:n]) + "…"
	}
	return s
}

func appendTimeline(e tlEntry) {
	tlMu.Lock()
	defer tlMu.Unlock()
	if b, err := os.ReadFile(filepath.Join(handsUserDir(), "task.json")); err == nil {
		var t struct {
			ID string `json:"id"`
		}
		if json.Unmarshal(b, &t) == nil {
			e.Task = t.ID
		}
	}
	dir := filepath.Join(handsUserDir(), "timeline")
	_ = os.MkdirAll(dir, 0o755)
	f, err := os.OpenFile(filepath.Join(dir, e.T.Format("2006-01-02")+".jsonl"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	b, _ := json.Marshal(e)
	_, _ = f.Write(append(b, '\r', '\n'))
}

// timelineMiddleware：道具の呼び出し（tools/call）を記録する。iphone_status は記録しない
func timelineMiddleware(next mcp.MethodHandler) mcp.MethodHandler {
	return func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
		if method != "tools/call" {
			return next(ctx, method, req)
		}
		start := time.Now()
		if p, ok := req.GetParams().(*mcp.CallToolParamsRaw); ok {
			curTool.Store(p.Name)
			var w struct {
				Why string `json:"why"`
			}
			_ = json.Unmarshal(p.Arguments, &w)
			curWhy.Store(w.Why)
		}
		res, err := next(ctx, method, req)
		p, ok := req.GetParams().(*mcp.CallToolParamsRaw)
		if !ok || p.Name == "iphone_status" {
			return res, err
		}
		e := tlEntry{T: start, Hand: "iphone", Tool: p.Name, Args: oneLine(string(p.Arguments), 300), OK: err == nil, Ms: time.Since(start).Milliseconds()}
		e.Lane = "D"
		if iphoneViewTools[p.Name] {
			e.Lane = "A"
		}
		if r, ok := res.(*mcp.CallToolResult); ok && r != nil {
			e.OK = e.OK && !r.IsError
			if r.IsError && len(r.Content) > 0 {
				if t, ok := r.Content[0].(*mcp.TextContent); ok && strings.HasPrefix(t.Text, "E（") {
					e.Lane = "E"
				}
			}
			for _, c := range r.Content {
				if t, ok := c.(*mcp.TextContent); ok {
					e.Res = oneLine(strings.SplitN(t.Text, "\n", 2)[0], 160)
					break
				}
			}
		}
		if err != nil {
			e.Res = oneLine(err.Error(), 160)
		}
		appendTimeline(e)
		return res, err
	}
}

// iphoneViewTools：見るだけの道具（道 A）。ほかは D
var iphoneViewTools = map[string]bool{"iphone_screenshot": true, "iphone_elements": true, "iphone_info": true, "iphone_logs": true, "iphone_wait": true}

// finalRe：戻せない操作の最後のボタンの名前（tools/pc-mcp/lane.go と同じ）
var finalRe = regexp.MustCompile(`(?i)(送信|送る|購入|注文を確定|注文する|ご注文|支払う|支払い|お支払い|決済|ログイン|サインイン|投稿|公開する|ツイート|ポストする|\bsend\b|\bsubmit\b|\bbuy\b|\bpurchase\b|place (your )?order|\bpay\b|checkout|check out|\blog ?in\b|\bsign ?in\b|\bpost\b|\bpublish\b|\btweet\b)`)

func finalWord(name string) string {
	name = strings.TrimSpace(name)
	if name == "" || utf8.RuneCountInString(name) > 30 {
		return ""
	}
	return finalRe.FindString(name)
}
