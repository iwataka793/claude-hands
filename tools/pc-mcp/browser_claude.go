package main

// Claude 専用の Chrome（ASK-DESIGN.md の 4）：
//   ログインの要らない作業は、カオルのログインが入っていない Claude 用の Chrome（別の User Data）でやる。
//   ここではログインしない（ログイン・お金・パスワードのページは、許可があっても開かない・動かさない）。
//   カオルのいつもの Chrome（ログインしたまま）は、頼まれたとき（profile=kaoru）だけ使う。
// Claude 用の Chrome は道具が自分で起動する（--user-data-dir と --remote-debugging-port=0。PC の中 127.0.0.1 だけ）。
// カオルの Chrome と違って「リモートデバッグを許可しますか？」は出ない。ブラウザのスイッチがオフ・30 分使わないと閉じる。

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	profClaude = "claude"
	profKaoru  = "kaoru"
)

// browserArg：ブラウザの道具の引数「どの Chrome か」
type browserArg struct {
	Profile string `json:"profile,omitempty" jsonschema:"どの Chrome か：claude（Claude 専用・既定。カオルのログインが入っていない。ここではログインしない）・kaoru（カオルのいつもの Chrome。カオルに頼まれたときだけ）"`
}

type profileKey struct{}

// withProfile：この呼び出しで使う Chrome を ctx に入れる（省くと既定。既定は claude、試験などで PC_BROWSER_PROFILE）
func withProfile(ctx context.Context, p string) (context.Context, error) {
	p = strings.ToLower(strings.TrimSpace(p))
	if p == "" {
		p = env("PC_BROWSER_PROFILE", profClaude)
	}
	if p != profClaude && p != profKaoru {
		return ctx, fmt.Errorf("profile は claude（Claude 専用の Chrome）か kaoru（カオルの Chrome）です（%q）", p)
	}
	return context.WithValue(ctx, profileKey{}, p), nil
}

func profileOf(ctx context.Context) string {
	if p, ok := ctx.Value(profileKey{}).(string); ok {
		return p
	}
	return env("PC_BROWSER_PROFILE", profClaude)
}

// claudeChromeDir：Claude 用の Chrome の User Data（カオルの Chrome とは別。ログインは入れない）
func claudeChromeDir() string {
	if v := env("PC_CLAUDE_CHROME_DIR", ""); v != "" {
		return v
	}
	return filepath.Join(os.Getenv("LOCALAPPDATA"), "claude-hands", "chrome-claude")
}

func chromeExe() string {
	for _, c := range []string{env("PC_CHROME_EXE", ""),
		filepath.Join(os.Getenv("ProgramFiles"), `Google\Chrome\Application\chrome.exe`),
		filepath.Join(os.Getenv("ProgramFiles(x86)"), `Google\Chrome\Application\chrome.exe`),
		filepath.Join(os.Getenv("LOCALAPPDATA"), `Google\Chrome\Application\chrome.exe`)} {
		if c != "" {
			if st, err := os.Stat(c); err == nil && !st.IsDir() {
				return c
			}
		}
	}
	return ""
}

// launchClaudeChrome：Claude 用の Chrome を起動して、つなぐ番号（DevToolsActivePort）ができるまで待つ
// 窓は出さずに起動し（--no-startup-window）、つないでから窓を「後ろで」作る（newClaudeWindow）。
// カオルの PC で確かめた（2026-11-27）：ふつうに起動すると、最小化の指定でも Chrome が前を取った。
// 窓なしで起動して Target.createTarget（newWindow・background）で作ると、カオルの窓が前のまま。
func launchClaudeChrome(ctx context.Context) (int, error) {
	exe := chromeExe()
	if exe == "" {
		return 0, errors.New("Chrome（chrome.exe）が見つかりません")
	}
	dir := claudeChromeDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return 0, err
	}
	port := filepath.Join(dir, "DevToolsActivePort")
	_ = os.Remove(port) // 前に閉じたときの古い番号
	setClaudeChromePrefs(dir)
	args := []string{"--user-data-dir=" + dir, "--remote-debugging-port=0", "--remote-debugging-address=127.0.0.1",
		"--no-first-run", "--no-default-browser-check", "--disable-sync", "--new-window",
		"--disable-features=Translate", "--hide-crash-restore-bubble", "--no-startup-window"}
	if extra := env("PC_CLAUDE_CHROME_ARGS", ""); extra != "" {
		args = append(args, strings.Fields(extra)...)
	}
	pid, err := startQuiet(exe, args)
	if err != nil {
		return 0, fmt.Errorf("Claude 用の Chrome を起動できません：%v", err)
	}
	end := time.Now().Add(25 * time.Second)
	for time.Now().Before(end) {
		if b, err := os.ReadFile(port); err == nil && strings.TrimSpace(strings.SplitN(string(b), "\n", 2)[0]) != "" {
			return pid, nil
		}
		select {
		case <-ctx.Done():
			return 0, ctx.Err()
		case <-time.After(200 * time.Millisecond):
		}
	}
	return 0, errors.New("Claude 用の Chrome が起動しましたが、つなぐ番号（DevToolsActivePort）が出ませんでした")
}

// newClaudeWindow：Claude 用の Chrome に窓が無ければ、後ろで作る（フォーカスを取らない）。窓はほかの窓の後ろに置く
func newClaudeWindow(ctx context.Context, c *CDP, pid int) {
	var r struct {
		TargetInfos []targetInfo `json:"targetInfos"`
	}
	if c.Call(ctx, "", "Target.getTargets", nil, &r) == nil {
		for _, t := range r.TargetInfos {
			if t.Type == "page" {
				return
			}
		}
	}
	_ = c.Call(ctx, "", "Target.createTarget", map[string]any{"url": "about:blank", "newWindow": true, "background": true}, nil)
	if pid != 0 {
		go sendBack(pid)
	}
}

// claudeBlocked：Claude 用の Chrome では開かない・動かさないページ（ログイン・お金・パスワード）。許可があってもだめ
func claudeBlocked(ctx context.Context, t targetInfo) *Reply {
	if profileOf(ctx) != profClaude {
		return nil
	}
	if m := sensitivePageRe.FindString(t.Title + " " + t.URL); m != "" {
		r := errText("Claude 用の Chrome では、ログイン・お金・パスワードにかかわるページ（「%s」）は開かない・動かさない（ログインがたまらないように）。要るなら、カオルに聞いてから profile=kaoru（カオルの Chrome）で", m)
		return &r
	}
	return nil
}

func profileName(p string) string {
	if p == profKaoru {
		return "カオルの Chrome"
	}
	return "Claude 用の Chrome"
}

// setClaudeChromePrefs：Claude 用の Chrome の見た目と設定（起動の前に Preferences に入れる。ほかの中身はそのまま）
//
//	ダーク＋紫（カオルの Chrome とひと目で見分ける。2026-11-27 カオルの PC で見て決めた）・名前「Claude」・翻訳の吹き出しを出さない。
//	Chrome 158 の名前：browser.theme.user_color2（ARGB の色）・color_variant2（3＝はっきりした色）・color_scheme2（2＝ダーク）
func setClaudeChromePrefs(dir string) {
	f := filepath.Join(dir, "Default", "Preferences")
	m := map[string]any{}
	if b, err := os.ReadFile(f); err == nil {
		if json.Unmarshal(b, &m) != nil {
			return // 読めない形なら触らない
		}
	}
	sub := func(parent map[string]any, k string) map[string]any {
		if v, ok := parent[k].(map[string]any); ok {
			return v
		}
		v := map[string]any{}
		parent[k] = v
		return v
	}
	theme := sub(sub(m, "browser"), "theme")
	theme["user_color2"] = int32(-8635667) // 0xFF7C3AED（紫）
	theme["color_variant2"] = 3
	theme["color_scheme2"] = 2
	sub(m, "profile")["name"] = "Claude"
	sub(m, "translate")["enabled"] = false
	b, err := json.Marshal(m)
	if err != nil {
		return
	}
	_ = os.MkdirAll(filepath.Dir(f), 0o755)
	_ = os.WriteFile(f, b, 0o644)
}
