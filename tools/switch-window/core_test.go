package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestRequests(t *testing.T) {
	h := t.TempDir()
	os.Setenv("HANDS_HOME", h)
	os.Setenv("HANDS_ALLOW_DIR", filepath.Join(h, "allow"))
	rd := requestDir()
	os.MkdirAll(rd, 0o755)
	os.WriteFile(filepath.Join(rd, "bogus.txt"), []byte("x"), 0o644)
	os.WriteFile(filepath.Join(rd, "pc-view.txt"), []byte("old"), 0o644)
	old := time.Now().Add(-3 * time.Minute)
	os.Chtimes(filepath.Join(rd, "pc-view.txt"), old, old)
	os.WriteFile(filepath.Join(rd, "iphone-operate.txt"), []byte(`{"kind":"iphone-operate","what":"iPhone の画面を押す","why":"作業A"}`), 0o644)
	os.WriteFile(filepath.Join(rd, "pc-allow.txt"), []byte("理由だけ\r\n"), 0o644)
	n := time.Now().Add(time.Second)
	os.Chtimes(filepath.Join(rd, "pc-allow.txt"), n, n)
	p, others := nextRequest()
	if p == nil || p.Kind != "iphone-operate" || p.What != "iPhone の画面を押す" || p.Why != "作業A" || others != 1 || p.Total != 55 {
		t.Fatalf("1: %+v %d", p, others)
	}
	v := askView(p, others)
	t.Logf("%+v", v)
	p, others = nextRequest()
	if p == nil || p.Kind != "pc-allow" || p.Why != "理由だけ" || p.Total != 80 || p.What == "" {
		t.Fatalf("2: %+v", p)
	}
	t.Logf("%+v %s", askView(p, 0), ifAllowed("browser-view"))
	if p, _ := nextRequest(); p != nil {
		t.Fatal("3")
	}
	if es, _ := os.ReadDir(rd); len(es) != 0 {
		t.Fatalf("left %v", es)
	}
	writeAnswer("pc-allow", "deny")
	logAsk(p2(), "deny")
	// allow
	os.MkdirAll(allowDir(), 0o755)
	os.WriteFile(filepath.Join(allowDir(), "allow.txt"), []byte("allow"), 0o644)
	if allowLeft() <= 0 {
		t.Fatal("allow")
	}
	time.Sleep(20 * time.Millisecond)
	cancelAllow()
	if allowLeft() != 0 {
		t.Fatal("cancel")
	}
	writeMode(filepath.Join(devkit(), "iphone-mcp"), "operate")
	dv := devViews()
	t.Logf("%+v tip=%s strong=%s", dv, trayTip(dv, false), strongest(dv, false))
	if dv[0].Mode != "operate" || dv[0].Left != 30 {
		t.Fatal("dev")
	}
}

func p2() *pending { return &pending{askRequest: askRequest{Kind: "pc-allow"}} }
