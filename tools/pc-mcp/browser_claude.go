package main

// Claude 専用の Chrome（ASK-DESIGN.md の 4）：
//   ログインの要らない作業は、カオルのログインが入っていない Claude 用の Chrome（別の User Data）でやる。
//   ここではログインしない（ログイン・お金・パスワードのページは、許可があっても開かない・動かさない）。
//   カオルのいつもの Chrome（ログインしたまま）は、頼まれたとき（profile=kaoru）だけ使う。
// Claude 用の Chrome は道具が自分で起動する（--user-data-dir と --remote-debugging-port=0。PC の中 127.0.0.1 だけ）。
// カオルの Chrome と違って「リモートデバッグを許可しますか？」は出ない。ブラウザのスイッチがオフ・30 分使わないと閉じる。

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
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
func launchClaudeChrome(ctx context.Context) error {
	exe := chromeExe()
	if exe == "" {
		return errors.New("Chrome（chrome.exe）が見つかりません")
	}
	dir := claudeChromeDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	port := filepath.Join(dir, "DevToolsActivePort")
	_ = os.Remove(port) // 前に閉じたときの古い番号
	args := []string{"--user-data-dir=" + dir, "--remote-debugging-port=0", "--remote-debugging-address=127.0.0.1",
		"--no-first-run", "--no-default-browser-check", "--disable-sync", "--new-window"}
	if extra := env("PC_CLAUDE_CHROME_ARGS", ""); extra != "" {
		args = append(args, strings.Fields(extra)...)
	}
	args = append(args, "about:blank")
	cmd := exec.Command(exe, args...)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("Claude 用の Chrome を起動できません：%v", err)
	}
	go func() { _ = cmd.Wait() }()
	end := time.Now().Add(25 * time.Second)
	for time.Now().Before(end) {
		if b, err := os.ReadFile(port); err == nil && strings.TrimSpace(strings.SplitN(string(b), "\n", 2)[0]) != "" {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(200 * time.Millisecond):
		}
	}
	return errors.New("Claude 用の Chrome が起動しましたが、つなぐ番号（DevToolsActivePort）が出ませんでした")
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
