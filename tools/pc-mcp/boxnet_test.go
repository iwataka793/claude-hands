package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestBoxNet(t *testing.T) {
	f := filepath.Join(t.TempDir(), "net-allow.txt")
	_ = os.WriteFile(f, []byte("# 説明\r\ngithub.com\r\n*.pypi.org  # 説明\r\nbad entry\r\nhttp://x.com\r\n"), 0o644)
	t.Setenv("HANDS_NET_ALLOW", f)
	t.Setenv("PC_HANDS_USERDIR", t.TempDir())
	list := netAllowList()
	if strings.Join(list, ",") != "github.com,pypi.org" {
		t.Fatalf("一覧 %v", list)
	}
	for h, want := range map[string]bool{
		"github.com": true, "api.github.com": true, "GitHub.com.": true, "pypi.org": true, "files.pypi.org": true,
		"evilgithub.com": false, "github.com.evil.net": false, "example.com": false, "140.82.112.3": false, "": false,
	} {
		if got := hostAllowed(h, list); got != want {
			t.Errorf("%q：%v（%v のはず）", h, got, want)
		}
	}
	// 一覧が無ければ何も通さない
	t.Setenv("HANDS_NET_ALLOW", filepath.Join(t.TempDir(), "none.txt"))
	if hostAllowed("github.com", netAllowList()) {
		t.Errorf("一覧が無いのに通した")
	}
	t.Setenv("HANDS_NET_ALLOW", f)
	start := time.Now()
	for _, c := range []struct {
		method, host string
		code         int
	}{
		{http.MethodGet, "github.com", http.StatusForbidden},          // HTTPS 以外
		{http.MethodConnect, "example.com:443", http.StatusForbidden}, // 一覧に無い
		{http.MethodConnect, "github.com:22", http.StatusForbidden},   // 443 以外
	} {
		r := httptest.NewRequest(c.method, "http://"+c.host+"/", nil)
		r.Host = c.host
		w := httptest.NewRecorder()
		proxyHandle(w, r)
		if w.Code != c.code {
			t.Errorf("%s %s：%d", c.method, c.host, w.Code)
		}
	}
	al, bl := netSince(start)
	if len(al) != 0 || len(bl) != 3 {
		t.Errorf("記録 通した %v・止めた %v", al, bl)
	}
}
