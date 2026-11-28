package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// ②：本物で変える台本は、控え（Git の記録）が取れなければ動かさない
func TestRealNeedsSnapshot(t *testing.T) {
	home := t.TempDir()
	t.Setenv("USERPROFILE", home)
	t.Setenv("HOME", home)
	hu := filepath.Join(home, "devkit", "hands")
	t.Setenv("PC_HANDS_USERDIR", hu)
	t.Setenv("PC_HANDS_DIR", t.TempDir())
	_ = os.MkdirAll(hu, 0o755)
	// 記録するフォルダが無い ＝ 控えが取れない
	_ = os.WriteFile(filepath.Join(hu, "git-folders.txt"), []byte("# なし\r\n"), 0o644)
	a := &Agent{st: NewState(t.TempDir(), time.Minute), ps: NewRunner()}
	_ = os.WriteFile(a.st.path("mode.txt"), []byte("operate"), 0o644)
	a.insp = func(string) (scriptInfo, error) { return scriptInfo{Cmds: []string{"Set-Content"}}, nil }
	out := filepath.Join(home, "x.txt")
	b, _ := json.Marshal(map[string]any{"script": "Set-Content " + out + " x", "real": true})
	r := a.powershell(context.Background(), b)
	if !r.IsError || !strings.Contains(r.Text, "控え") || r.Lane != "C" {
		t.Fatalf("控えなしで進んだ：%+v", r)
	}
	if _, err := os.Stat(out); err == nil {
		t.Errorf("本物が変わった")
	}
}
