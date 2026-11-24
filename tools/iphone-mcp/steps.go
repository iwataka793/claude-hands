package main

// iphone_steps：いくつかの操作を 1 回の呼び出しで順に行う。

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// stepArgs は 1 つの操作の引数（道具ごとの引数をまとめたもの。名前は各道具と同じ）。
type stepArgs struct {
	Do        string  `json:"do"`
	X         float64 `json:"x"`
	Y         float64 `json:"y"`
	FromX     float64 `json:"from_x"`
	FromY     float64 `json:"from_y"`
	ToX       float64 `json:"to_x"`
	ToY       float64 `json:"to_y"`
	Duration  float64 `json:"duration"`
	Count     int     `json:"count"`
	Hold      float64 `json:"hold"`
	Text      string  `json:"text"`
	Index     int     `json:"index"`
	Partial   bool    `json:"partial"`
	Field     string  `json:"field"`
	Clear     bool    `json:"clear"`
	Enter     bool    `json:"enter"`
	Name      string  `json:"name"`
	Scale     float64 `json:"scale"`
	Velocity  float64 `json:"velocity"`
	Action    string  `json:"action"`
	Button    string  `json:"button"`
	Gone      bool    `json:"gone"`
	Seconds   float64 `json:"seconds"`
	Direction string  `json:"direction"`
	Max       int     `json:"max"`
	Tap       bool    `json:"tap"`
	Bundle    string  `json:"bundle"`
	URL       string  `json:"url"`
}

func (a *app) runStep(ctx context.Context, s stepArgs) (string, error) {
	switch s.Do {
	case "tap":
		return a.opTap(ctx, s.X, s.Y, s.Count, s.Hold)
	case "swipe":
		return a.opSwipe(ctx, s.FromX, s.FromY, s.ToX, s.ToY, s.Duration)
	case "tap_text":
		return a.opTapText(ctx, s.Text, s.Index, s.Partial, s.Count, s.Hold)
	case "type":
		return a.opType(ctx, s.Text, s.Field, s.Partial, s.Clear, s.Enter)
	case "button":
		return a.opButton(ctx, s.Name)
	case "pinch":
		return a.opPinch(ctx, s.Scale, s.Velocity)
	case "alert":
		return a.opAlert(ctx, s.Action, s.Button)
	case "wait":
		return a.opWait(ctx, s.Text, s.Partial, s.Gone, s.Seconds)
	case "scroll_to":
		return a.opScrollTo(ctx, s.Text, s.Partial, s.Direction, s.Max, s.Tap)
	case "app":
		return a.opApp(ctx, s.Action, s.Bundle, s.URL)
	case "sleep":
		if s.Seconds <= 0 || s.Seconds > 60 {
			return "", fmt.Errorf("sleep の seconds は 0〜60")
		}
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(time.Duration(s.Seconds * float64(time.Second))):
		}
		return fmt.Sprintf("%.1f 秒待った", s.Seconds), nil
	}
	return "", fmt.Errorf("do「%s」は知らない種類（tap / swipe / tap_text / type / button / pinch / alert / wait / scroll_to / app / sleep）", s.Do)
}

func (a *app) runSteps(ctx context.Context, in stepsIn) *mcp.CallToolResult {
	if len(in.Steps) == 0 {
		return fail("steps が空です")
	}
	if len(in.Steps) > 50 {
		return fail("steps は 50 個まで")
	}
	pause := in.Pause
	if pause <= 0 {
		pause = 0.3
	}
	show := in.Show
	if show == "" {
		show = "always"
	}
	start := time.Now()
	var lines []string
	for i, raw := range in.Steps {
		b, _ := json.Marshal(raw)
		var s stepArgs
		if err := json.Unmarshal(b, &s); err != nil {
			msg := fmt.Sprintf("%d 番目の書き方がおかしい：%v", i+1, err)
			return a.after(ctx, afterOpts{Show: "always", Log: in.Log, Settle: 0.5}, start, nil, joinLines(lines, msg), true)
		}
		t0 := time.Now()
		m, err := a.runStep(ctx, s)
		if err != nil {
			msg := fmt.Sprintf("%d 番目（%s）で止めた：%v", i+1, s.Do, err)
			return a.after(ctx, afterOpts{Show: "always", Log: in.Log, Settle: 1}, start, nil, joinLines(lines, msg), true)
		}
		lines = append(lines, fmt.Sprintf("%d. %s（%.1f 秒）", i+1, m, time.Since(t0).Seconds()))
		if i < len(in.Steps)-1 {
			time.Sleep(time.Duration(pause * float64(time.Second)))
		}
	}
	lines = append(lines, fmt.Sprintf("全部できた（%.1f 秒）", time.Since(start).Seconds()))
	return a.after(ctx, afterOpts{Show: show, Log: in.Log}, start, nil, joinLines(lines, ""), false)
}

func joinLines(lines []string, last string) string {
	out := ""
	for _, l := range lines {
		out += l + "\n"
	}
	return out + last
}
