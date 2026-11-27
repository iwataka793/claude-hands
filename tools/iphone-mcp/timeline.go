package main

// TIMELINE：iPhone の道具の呼び出しを、PC・ブラウザと同じ 1 本の記録に残す（tools/pc-mcp/hands.go と同じ形）。
// 置き場所 %USERPROFILE%\devkit\hands\timeline\日付.jsonl。今の作業（task.json）があれば、その番号も付ける。

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

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
		res, err := next(ctx, method, req)
		p, ok := req.GetParams().(*mcp.CallToolParamsRaw)
		if !ok || p.Name == "iphone_status" {
			return res, err
		}
		e := tlEntry{T: start, Hand: "iphone", Tool: p.Name, Args: oneLine(string(p.Arguments), 300), OK: err == nil, Ms: time.Since(start).Milliseconds()}
		if r, ok := res.(*mcp.CallToolResult); ok && r != nil {
			e.OK = e.OK && !r.IsError
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
