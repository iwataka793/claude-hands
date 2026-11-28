package main

// 箱のネットの中継（次の段 1。2026-11-28）。
//
// 箱（ClaudeBox）からは、Sandboxie の通信の決まり（NetworkAccess。Windows の中核 WFP で強制）で
// 127.0.0.1 のこの中継の口にしかつながらない。中継は「許可したサイト」への HTTPS（CONNECT・443）だけを通す。
// 許可したサイトの一覧は %ProgramData%\claude-hands\net-allow.txt（pc-setup.bat が作る。カオルの権限では読むだけ＝Claude は足せない）。
// 一覧が無い・読めないときは何も通さない。通した・止めたは devkit\hands\try\net.log に残す（pc_try の返事に出る）。
// 中継は常駐役が動いている間だけ動く（スイッチがオフなら箱のネットも無い）。

import (
	"bufio"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const boxProxyAddr = "127.0.0.1:60230"

func netAllowFile() string {
	if v := os.Getenv("HANDS_NET_ALLOW"); v != "" {
		return v
	}
	return filepath.Join(programData(), "claude-hands", "net-allow.txt")
}

func netLogFile() string { return filepath.Join(tryDir(), "net.log") }

// netAllowList：一覧（1 行 1 つ。# から後は説明。「github.com」は github.com とその下の名前を通す）
func netAllowList() []string {
	b, err := os.ReadFile(netAllowFile())
	if err != nil {
		return nil
	}
	var out []string
	for _, l := range strings.Split(strings.ReplaceAll(string(b), "\r\n", "\n"), "\n") {
		if i := strings.Index(l, "#"); i >= 0 {
			l = l[:i]
		}
		l = strings.ToLower(strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(l), "*.")))
		if l != "" && !strings.ContainsAny(l, " /:\\") {
			out = append(out, l)
		}
	}
	return out
}

// hostAllowed：名前（IP の数字は通さない）が一覧のどれかか、その下か
func hostAllowed(host string, list []string) bool {
	h := strings.ToLower(strings.TrimSuffix(host, "."))
	if h == "" || net.ParseIP(strings.Trim(h, "[]")) != nil {
		return false
	}
	for _, a := range list {
		if h == a || strings.HasSuffix(h, "."+a) {
			return true
		}
	}
	return false
}

var netLogMu sync.Mutex

func netLog(host, verdict string) {
	netLogMu.Lock()
	defer netLogMu.Unlock()
	_ = os.MkdirAll(tryDir(), 0o755)
	p := netLogFile()
	if fi, err := os.Stat(p); err == nil && fi.Size() > 2<<20 { // 2 MB で前の分を 1 つだけ残す
		_ = os.Rename(p, p+".1")
	}
	f, err := os.OpenFile(p, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	fmt.Fprintf(f, "%s\t%s\t%s\n", time.Now().Format(time.RFC3339), verdict, host)
}

// netSince：その時刻からあとに通した・止めたもの（同じ名前はまとめる）
func netSince(t time.Time) (allowed, blocked []string) {
	b, err := os.ReadFile(netLogFile())
	if err != nil {
		return nil, nil
	}
	seenA, seenB := map[string]bool{}, map[string]bool{}
	for _, l := range strings.Split(string(b), "\n") {
		f := strings.Split(l, "\t")
		if len(f) != 3 {
			continue
		}
		at, err := time.Parse(time.RFC3339, f[0])
		if err != nil || at.Before(t.Add(-time.Second)) {
			continue
		}
		switch f[1] {
		case "通した":
			if !seenA[f[2]] {
				seenA[f[2]] = true
				allowed = append(allowed, f[2])
			}
		default:
			if !seenB[f[2]] {
				seenB[f[2]] = true
				blocked = append(blocked, f[2])
			}
		}
	}
	return
}

// boxProxy：中継を動かす（常駐役の中で。口が使えなければ記録だけ残して何もしない）
func boxProxy() {
	ln, err := net.Listen("tcp", boxProxyAddr)
	if err != nil {
		slog.Info("box proxy not started", "err", err)
		return
	}
	srv := &http.Server{Handler: http.HandlerFunc(proxyHandle), ReadHeaderTimeout: 15 * time.Second}
	_ = srv.Serve(ln)
}

func proxyHandle(w http.ResponseWriter, r *http.Request) {
	host, port := r.Host, "443"
	if h, p, err := net.SplitHostPort(r.Host); err == nil {
		host, port = h, p
	}
	if r.Method != http.MethodConnect {
		netLog(host, "止めた（HTTPS 以外）")
		http.Error(w, "claude-hands：箱からは HTTPS（CONNECT）だけ通します", http.StatusForbidden)
		return
	}
	if port != "443" {
		netLog(host+":"+port, "止めた（443 以外）")
		http.Error(w, "claude-hands：箱からは 443 番だけ通します", http.StatusForbidden)
		return
	}
	if !hostAllowed(host, netAllowList()) {
		netLog(host, "止めた（許可した一覧に無い）")
		http.Error(w, "claude-hands：許可したサイトではありません（"+host+"）", http.StatusForbidden)
		return
	}
	up, err := net.DialTimeout("tcp", net.JoinHostPort(host, "443"), 20*time.Second)
	if err != nil {
		netLog(host, "止めた（つながらない："+oneLine(err.Error(), 80)+"）")
		http.Error(w, "claude-hands：つながりません", http.StatusBadGateway)
		return
	}
	hj, ok := w.(http.Hijacker)
	if !ok {
		up.Close()
		http.Error(w, "claude-hands：中継できません", http.StatusInternalServerError)
		return
	}
	c, rw, err := hj.Hijack()
	if err != nil {
		up.Close()
		return
	}
	netLog(host, "通した")
	_, _ = c.Write([]byte("HTTP/1.1 200 Connection established\r\n\r\n"))
	go pipe(up, rw.Reader, c)
	pipe(c, up, up)
}

// pipe：片方向に流す（終わったら両方閉じる）
func pipe(dst io.WriteCloser, src io.Reader, closeAlso io.Closer) {
	if br, ok := src.(*bufio.Reader); ok && br.Buffered() > 0 {
		b, _ := br.Peek(br.Buffered())
		_, _ = dst.Write(b)
		_, _ = br.Discard(len(b))
	}
	_, _ = io.Copy(dst, src)
	dst.Close()
	closeAlso.Close()
}
