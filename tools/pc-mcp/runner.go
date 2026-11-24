package main

// PowerShell を開きっぱなしにして、1 回ごとの起動（0.5〜1 秒）を省く。
// 1 回の頼みは「base64 にした台本を 1 行で送る → 終わりの印が出るまで読む」。
// 時間切れになったら PowerShell ごと終わらせて、次の頼みで起動し直す。

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"regexp"
	"strings"
	"sync"
	"time"
)

type Runner struct {
	mu     sync.Mutex
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	lines  chan string
	exe    string
	starts int
}

func NewRunner() *Runner { return &Runner{exe: env("PC_AGENT_POWERSHELL", "powershell.exe")} }

func (r *Runner) startLocked() error {
	cmd := exec.Command(r.exe, "-NoLogo", "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-Command", "-")
	hideWindow(cmd)
	in, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	out, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	cmd.Stderr = io.Discard
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("PowerShell を起動できません：%v", err)
	}
	lines := make(chan string, 4096)
	go func() {
		sc := bufio.NewScanner(out)
		sc.Buffer(make([]byte, 1024*1024), 64*1024*1024)
		for sc.Scan() {
			lines <- sc.Text()
		}
		close(lines)
	}()
	r.cmd, r.stdin, r.lines = cmd, in, lines
	r.starts++
	// 出力を UTF-8 に、進み具合の表示を止める
	_, err = r.sendLocked(context.Background(), "[Console]::OutputEncoding=[Text.Encoding]::UTF8; $OutputEncoding=[Text.Encoding]::UTF8; $ProgressPreference='SilentlyContinue'; $ErrorActionPreference='Continue'; if ($PSStyle) { $PSStyle.OutputRendering='PlainText' }; 'ready'", 30*time.Second)
	return err
}

func (r *Runner) killLocked() {
	if r.cmd != nil && r.cmd.Process != nil {
		_ = r.cmd.Process.Kill()
		_ = r.cmd.Wait()
	}
	r.cmd, r.stdin, r.lines = nil, nil, nil
}

// 画面の色づけなどの制御文字（PowerShell 7 が出すことがある）
var ansiRe = regexp.MustCompile(`\x1b\[[0-9;?]*[A-Za-z]`)

func marker() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return "<<PCEND-" + hex.EncodeToString(b) + ">>"
}

// sendLocked は台本 src を実行して、出力（標準出力とエラーをまとめた文字）を返す。
func (r *Runner) sendLocked(ctx context.Context, src string, timeout time.Duration) (string, error) {
	mk := marker()
	b64 := base64.StdEncoding.EncodeToString([]byte(src))
	// 送る 1 行は ASCII だけにする（PowerShell の標準入力の文字コードに左右されないように）
	line := "$__s=[Text.Encoding]::UTF8.GetString([Convert]::FromBase64String('" + b64 + "')); " +
		"$__o = try { (& ([ScriptBlock]::Create($__s))) 2>&1 | Out-String -Width 400 } catch { 'ERROR: ' + $_.Exception.Message }; " +
		"[Console]::Out.Write($__o); [Console]::Out.WriteLine(''); [Console]::Out.WriteLine('" + mk + "'); [Console]::Out.Flush()\n"
	if _, err := io.WriteString(r.stdin, line); err != nil {
		return "", fmt.Errorf("PowerShell に送れません：%v", err)
	}
	var sb strings.Builder
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	for {
		select {
		case l, ok := <-r.lines:
			if !ok {
				return sb.String(), errors.New("PowerShell が途中で終わりました")
			}
			l = ansiRe.ReplaceAllString(l, "")
			if l == mk {
				return strings.TrimRight(sb.String(), "\r\n "), nil
			}
			sb.WriteString(strings.TrimRight(l, "\r"))
			sb.WriteString("\n")
		case <-deadline.C:
			return sb.String(), fmt.Errorf("時間切れ（%.0f 秒）", timeout.Seconds())
		case <-ctx.Done():
			return sb.String(), ctx.Err()
		}
	}
}

// Run は台本を実行する。args は $A として台本から使える（JSON から作る）。
func (r *Runner) Run(ctx context.Context, script string, args any, timeout time.Duration) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.cmd == nil {
		if err := r.startLocked(); err != nil {
			r.killLocked()
			return "", err
		}
	}
	src := script
	if args != nil {
		j, _ := json.Marshal(args)
		src = "$A = '" + strings.ReplaceAll(string(j), "'", "''") + "' | ConvertFrom-Json\n" + script
	}
	out, err := r.sendLocked(ctx, src, timeout)
	if err != nil {
		// 状態が分からなくなったので、次は起動し直す
		r.killLocked()
	}
	return out, err
}

func (r *Runner) Close() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.killLocked()
}
