package main

// 3 段のスイッチ（オフ・見るだけ・操作も可）。
// 切り替えるのはカオルだけ：PC のファイル mode.txt を、デスクトップに置いた .bat が書き換える。
// この MCP サーバーには切り替える道具を作らない（Claude からは変えられない）。
// 最後の動き（.bat で切り替えた時刻か、道具が呼ばれた時刻の新しい方）から一定時間（既定 30 分）たつと自動でオフに戻す。
// 時刻はファイル（mode.txt と last-used.txt の更新時刻）で持つ。Claude デスクトップはこのサーバーを
// 同時に 2 つ動かしたり、途中で起動し直したりするため（2026-11-24 の server.log で確認）、
// 起動のたびに「オフ」を書くと .bat で入れたスイッチが消えてしまう。

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
	path string // mode.txt
	used string // last-used.txt（道具が呼ばれた時刻。中身は使わず更新時刻だけ見る）
	idle time.Duration
	mu   sync.Mutex
}

func NewSwitch(dir string, idle time.Duration) *Switch {
	_ = os.MkdirAll(dir, 0o755)
	s := &Switch{path: filepath.Join(dir, "mode.txt"), used: filepath.Join(dir, "last-used.txt"), idle: idle}
	if _, err := os.Stat(s.path); err != nil {
		s.write("off") // まだ無ければオフで作る
	}
	s.IdleExpired() // PC を起動し直したときなど、しばらく使っていなければここでオフになる
	return s
}

func (s *Switch) write(v string) { _ = os.WriteFile(s.path, []byte(v+"\r\n"), 0o644) }

// Current は今のスイッチ。ファイルを毎回読む（カオルが切り替えたらすぐ効く）。
func (s *Switch) Current() Mode {
	// 読みそこね（.bat やほかのサーバーが同時に触っている瞬間）でオフと取り違えないよう、少し待って読み直す
	// （2026-11-25 に 1 回だけ「オフです」が出て、すぐ後は「操作も可」だった）
	var b []byte
	var err error
	for i := 0; i < 5; i++ {
		if b, err = os.ReadFile(s.path); err == nil && len(strings.TrimSpace(string(b))) > 0 {
			break
		}
		time.Sleep(60 * time.Millisecond)
	}
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

// Touch は道具が呼ばれた時刻を記録する（ほかのサーバーとも共有するためファイルに）。
func (s *Switch) Touch() {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	if err := os.Chtimes(s.used, now, now); err != nil {
		_ = os.WriteFile(s.used, []byte("used\r\n"), 0o644)
	}
}

// lastActivity は最後の動きの時刻（スイッチを切り替えた時刻と道具が呼ばれた時刻の新しい方）。
func (s *Switch) lastActivity() time.Time {
	var t time.Time
	for _, f := range []string{s.path, s.used} {
		if st, err := os.Stat(f); err == nil && st.ModTime().After(t) {
			t = st.ModTime()
		}
	}
	return t
}

// IdleExpired は、しばらく使われていなければ true を返し、スイッチをオフに戻す。
func (s *Switch) IdleExpired() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.Current() == ModeOff {
		return false
	}
	if time.Since(s.lastActivity()) >= s.idle {
		s.write("off")
		return true
	}
	return false
}

func (s *Switch) Path() string { return s.path }
