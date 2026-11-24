package main

// スイッチ（mode.txt）・許可（allow.txt）・使った時刻（last-used.txt）・記録（audit.log）。
// どれも %USERPROFILE%\devkit\pc-agent に置く。mode.txt と allow.txt を書くのはカオルの .bat だけ。

import (
	"fmt"
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

const allowWindow = 10 * time.Minute

type State struct {
	dir  string
	idle time.Duration
	mu   sync.Mutex
}

func NewState(dir string, idle time.Duration) *State {
	_ = os.MkdirAll(dir, 0o755)
	return &State{dir: dir, idle: idle}
}

func (s *State) path(name string) string { return filepath.Join(s.dir, name) }

func (s *State) Mode() Mode {
	b, err := os.ReadFile(s.path("mode.txt"))
	if err != nil {
		return ModeOff
	}
	switch strings.ToLower(strings.TrimSpace(string(b))) {
	case "view":
		return ModeView
	case "operate":
		return ModeOperate
	}
	return ModeOff
}

func (s *State) setOff() { _ = os.WriteFile(s.path("mode.txt"), []byte("off\r\n"), 0o644) }

// allowDir は許可のファイルの置き場所（%ProgramData%\pc-agent）。
// PC-ALLOW.bat が管理者の確認（UAC の「はい」）を通ってから書く。ここは管理者しか書けないようにしてあるので、
// 普通の権限で動く常駐役・PowerShell（＝Claude）からは書けない。
func allowDir() string {
	if v := env("PC_AGENT_ALLOW_DIR", ""); v != "" {
		return v
	}
	if pd := os.Getenv("ProgramData"); pd != "" {
		return filepath.Join(pd, "pc-agent")
	}
	return stateDir()
}

func allowFile() string { return filepath.Join(allowDir(), "allow.txt") }

// Allowed は、カオルが PC-ALLOW.bat を押してから 10 分以内か。残り時間も返す。
// スイッチを切り替えた（mode.txt を書いた）あとは、許可は取り消し。
func (s *State) Allowed() (bool, time.Duration) {
	st, err := os.Stat(allowFile())
	if err != nil {
		return false, 0
	}
	if ms, err := os.Stat(s.path("mode.txt")); err == nil && ms.ModTime().After(st.ModTime()) {
		return false, 0
	}
	left := allowWindow - time.Since(st.ModTime())
	if left <= 0 {
		return false, 0
	}
	return true, left
}

func (s *State) Touch() {
	now := time.Now()
	if err := os.Chtimes(s.path("last-used.txt"), now, now); err != nil {
		_ = os.WriteFile(s.path("last-used.txt"), []byte("used\r\n"), 0o644)
	}
}

// IdleExpired：最後の動き（スイッチを切り替えた時刻か、道具を使った時刻）から idle たったらオフにする。
func (s *State) IdleExpired() bool {
	if s.Mode() == ModeOff {
		return false
	}
	var last time.Time
	for _, f := range []string{"mode.txt", "last-used.txt"} {
		if st, err := os.Stat(s.path(f)); err == nil && st.ModTime().After(last) {
			last = st.ModTime()
		}
	}
	if time.Since(last) >= s.idle {
		s.setOff()
		return true
	}
	return false
}

// Audit は、Claude が頼んだことを 1 行ずつ残す（あとでカオルと Claude が見返す）。
func (s *State) Audit(tool, detail, result string, d time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	f, err := os.OpenFile(s.path("audit.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	detail = strings.ReplaceAll(strings.ReplaceAll(detail, "\r", " "), "\n", " ⏎ ")
	if len([]rune(detail)) > 300 {
		detail = string([]rune(detail)[:300]) + "…"
	}
	allow, _ := s.Allowed()
	a := ""
	if allow {
		a = " 許可中"
	}
	fmt.Fprintf(f, "%s [%s%s] %s %s → %s（%.1f 秒）\r\n", time.Now().Format("2006-01-02 15:04:05"), s.Mode(), a, tool, detail, result, d.Seconds())
}

// AuditTail は記録の最後の n 行。
func (s *State) AuditTail(n int) []string {
	b, err := os.ReadFile(s.path("audit.log"))
	if err != nil {
		return nil
	}
	lines := strings.Split(strings.TrimRight(strings.ReplaceAll(string(b), "\r\n", "\n"), "\n"), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return lines
}
