package main

// iPhone のログを PC のファイルにも残す（覚えておける 3 万行からあふれた前のことも確かめられるように）。
// 置き場所：%USERPROFILE%\devkit\iphone-mcp\logs\syslog.txt（200 MB で syslog.1.txt に回す。2 つだけ残す）。
// 中身は PC の中だけ。repo には入れない。

import (
	"bufio"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

const (
	logFileMax   = 200 << 20
	logTimeStamp = "2006-01-02 15:04:05.000"
)

type logFile struct {
	dir  string
	f    *os.File
	w    *bufio.Writer
	size int64
	last time.Time // 最後に書き出した時刻（溜めすぎない）
}

func newLogFile(dir string) *logFile {
	_ = os.MkdirAll(dir, 0o755)
	l := &logFile{dir: dir}
	l.open()
	return l
}

func (l *logFile) path() string { return filepath.Join(l.dir, "syslog.txt") }

func (l *logFile) open() {
	f, err := os.OpenFile(l.path(), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	st, _ := f.Stat()
	l.f, l.w = f, bufio.NewWriterSize(f, 256<<10)
	if st != nil {
		l.size = st.Size()
	}
}

// write は 1 行書く（logMu を持って呼ぶ）。
func (l *logFile) write(at time.Time, msg string) {
	if l == nil || l.w == nil {
		return
	}
	line := at.Format(logTimeStamp) + " " + strings.ReplaceAll(msg, "\n", " ") + "\n"
	n, _ := l.w.WriteString(line)
	l.size += int64(n)
	if at.Sub(l.last) > time.Second {
		_ = l.w.Flush()
		l.last = at
	}
	if l.size > logFileMax {
		_ = l.w.Flush()
		_ = l.f.Close()
		_ = os.Rename(l.path(), filepath.Join(l.dir, "syslog.1.txt"))
		l.size = 0
		l.open()
	}
}

func (l *logFile) flush() {
	if l != nil && l.w != nil {
		_ = l.w.Flush()
	}
}

// searchLogFiles は PC のファイルから、from〜to の時刻で正規表現に合う行を探す（古い方のファイルから）。
func searchLogFiles(dir string, re *regexp.Regexp, from, to time.Time, max int) []string {
	var out []string
	for _, name := range []string{"syslog.1.txt", "syslog.txt"} {
		f, err := os.Open(filepath.Join(dir, name))
		if err != nil {
			continue
		}
		sc := bufio.NewScanner(f)
		sc.Buffer(make([]byte, 1<<20), 1<<20)
		for sc.Scan() {
			s := sc.Text()
			if len(s) <= len(logTimeStamp) {
				continue
			}
			at, err := time.ParseInLocation(logTimeStamp, s[:len(logTimeStamp)], time.Local)
			if err != nil || at.Before(from) || !at.Before(to) {
				continue
			}
			text := s[len(logTimeStamp)+1:]
			if !re.MatchString(text) {
				continue
			}
			out = append(out, at.Format("01/02 15:04:05.000")+" "+text)
			if len(out) > max*4 {
				out = out[len(out)-max:]
			}
		}
		f.Close()
	}
	if len(out) > max {
		out = out[len(out)-max:]
	}
	return out
}
