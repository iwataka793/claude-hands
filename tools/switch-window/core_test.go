package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
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

func TestSign(t *testing.T) {
	h := t.TempDir()
	os.Setenv("HANDS_HOME", h)
	kf := filepath.Join(h, "request.key")
	os.Setenv("HANDS_KEY_FILE", kf)
	rd := requestDir()
	os.MkdirAll(rd, 0o755)
	put := func(name string, r askRequest) {
		b, _ := json.Marshal(r)
		os.WriteFile(filepath.Join(rd, name), b, 0o644)
	}
	r := askRequest{Kind: "pc-operate", What: "PC を操作する", Why: "説明", Task: "作業", Tool: "pc_ui_do", From: "t", At: time.Now()}
	// 合言葉なし：受け付けて「署名なし」
	put("pc-operate.txt", r)
	p, _ := nextRequest()
	if p == nil || !p.Unsigned {
		t.Fatalf("nokey: %+v", p)
	}
	os.WriteFile(kf, []byte("abc123\r\n"), 0o644)
	// 正しい署名
	r.Sig = signWith("abc123", r)
	put("pc-operate.txt", r)
	if p, _ := nextRequest(); p == nil || p.Unsigned || p.Why != "説明" {
		t.Fatalf("ok: %+v", p)
	}
	// 書き換えた頼み（署名が合わない）
	r2 := r
	r2.Why = "ちがう説明"
	put("pc-operate.txt", r2)
	if p, _ := nextRequest(); p != nil {
		t.Fatalf("bad: %+v", p)
	}
	if b, _ := os.ReadFile(filepath.Join(rd, "pc-operate.answer.json")); !strings.Contains(string(b), "unsigned") {
		t.Fatalf("answer %s", b)
	}
	// 署名なし（前の形）
	os.WriteFile(filepath.Join(rd, "pc-allow.txt"), []byte("理由だけ"), 0o644)
	if p, _ := nextRequest(); p != nil {
		t.Fatal("plain accepted with key")
	}
	// 名前と中身の種類が違う
	put("pc-allow.txt", r)
	if p, _ := nextRequest(); p != nil {
		t.Fatal("mismatch accepted")
	}
}

// 署名の並びは道具（tools/pc-mcp・iphone-mcp）と同じ。どちらも同じ答えになること
func TestSignSame(t *testing.T) {
	r := askRequest{Kind: "pc-allow", What: "インターネットとやりとりする（落とす・送る）", Why: "説明", Task: "作業", Tool: "pc_powershell", From: "pc-mcp t", At: time.Date(2026, 11, 27, 21, 0, 0, 0, time.FixedZone("JST", 9*3600))}
	if s := signWith("abc123", r); s != "b1eaa42805d6556ec4d703d1eaed5f7f80e0d0b459c0e5f932a59bf02bf6e6e6" {
		t.Fatal(s)
	}
}
