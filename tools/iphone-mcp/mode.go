package main

// 3 段のスイッチ（オフ・見るだけ・操作も可）。
// 切り替えるのはカオルだけ：PC のファイル mode.txt を、デスクトップに置いた .bat が書き換える。
// この MCP サーバーには切り替える道具を作らない（Claude からは変えられない）。
// 起動時はオフ。最後の呼び出しから一定時間（既定 30 分）たつと自動でオフに戻す。

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type Mode int

const (
	ModeOff Mode = iota
	ModeView
	ModeOperate
)

func (m Mode) String() string {
	switch m {
	case ModeView:
		return "見るだけ"
	case ModeOperate:
		return "操作も可"
	default:
		return "オフ"
	}
}

type Switch struct {
	path     string
	idle     time.Duration
	mu       sync.Mutex
	lastUsed time.Time
}

func NewSwitch(dir string, idle time.Duration) *Switch {
	_ = os.MkdirAll(dir, 0o755)
	s := &Switch{path: filepath.Join(dir, "mode.txt"), idle: idle, lastUsed: time.Now()}
	s.write("off") // 起動時はオフ
	return s
}

func (s *Switch) write(v string) { _ = os.WriteFile(s.path, []byte(v+"\r\n"), 0o644) }

// Current は今のスイッチ。ファイルを毎回読む（カオルが切り替えたらすぐ効く）。
func (s *Switch) Current() Mode {
	b, err := os.ReadFile(s.path)
	if err != nil {
		return ModeOff
	}
	switch strings.ToLower(strings.TrimSpace(string(b))) {
	case "view":
		return ModeView
	case "operate":
		return ModeOperate
	default:
		return ModeOff
	}
}

// Touch は道具が呼ばれた時刻を記録する。
func (s *Switch) Touch() {
	s.mu.Lock()
	s.lastUsed = time.Now()
	s.mu.Unlock()
}

// IdleExpired は、しばらく使われていなければ true を返し、スイッチをオフに戻す。
func (s *Switch) IdleExpired() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.Current() == ModeOff {
		return false
	}
	if time.Since(s.lastUsed) >= s.idle {
		s.write("off")
		return true
	}
	return false
}

func (s *Switch) Path() string { return s.path }
