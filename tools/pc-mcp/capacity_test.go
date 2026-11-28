package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCapacity(t *testing.T) {
	root := t.TempDir()
	t.Setenv("PC_HANDS_USERDIR", filepath.Join(root, "hands"))
	t.Setenv("PC_HANDS_LOCALDIR", filepath.Join(root, "local", "claude-hands"))
	t.Setenv("PC_HANDS_DIR", filepath.Join(root, "pd"))
	t.Setenv("LOCALAPPDATA", filepath.Join(root, "local"))
	t.Setenv("PC_HANDS_FREE_GB", "300")
	now := time.Now()
	old := now.AddDate(0, 0, -100)
	_ = os.MkdirAll(timelineDir(), 0o755)
	_ = os.MkdirAll(doneDir(), 0o755)
	write := func(p, s string, when time.Time) {
		_ = os.MkdirAll(filepath.Dir(p), 0o755)
		if err := os.WriteFile(p, []byte(s), 0o644); err != nil {
			t.Fatal(err)
		}
		_ = os.Chtimes(p, when, when)
	}
	write(filepath.Join(timelineDir(), old.Format("2006-01-02")+".jsonl"), "{}\n", now)
	write(filepath.Join(timelineDir(), now.Format("2006-01-02")+".jsonl"), "{}\n", now)
	write(filepath.Join(doneDir(), "old.json"), "{}", old)
	write(filepath.Join(doneDir(), "new.json"), "{}", now)
	asks := filepath.Join(handsUserDir(), "asks.jsonl")
	write(asks, `{"t":"`+old.Format(time.RFC3339)+`","kind":"pc-allow"}`+"\n"+`{"t":"`+now.Format(time.RFC3339)+`","kind":"pc-allow"}`+"\n", now)
	// Claude 用の Chrome：キャッシュだけ消え、ほかは残る
	cd := claudeChromeDir()
	write(filepath.Join(cd, "Default", "Cache", "Cache_Data", "f_0001"), strings.Repeat("x", 1000), now)
	write(filepath.Join(cd, "Default", "Preferences"), "{}", now)
	// 道具の一時ファイル：1 日より古いものだけ消える
	oldTmp := filepath.Join(os.TempDir(), "claude-hands-test-old-"+now.Format("150405.000"))
	newTmp := filepath.Join(os.TempDir(), "claude-hands-test-new-"+now.Format("150405.000"))
	write(oldTmp, "x", now.Add(-48*time.Hour))
	write(newTmp, "x", now)
	defer os.Remove(newTmp)

	out := (&Agent{}).capacityRun(context.Background())
	text := strings.Join(out, "\n")
	t.Log(text)
	if _, err := os.Stat(filepath.Join(timelineDir(), old.Format("2006-01-02")+".jsonl")); err == nil {
		t.Error("100 日前の TIMELINE が消えていない")
	}
	if _, err := os.Stat(filepath.Join(timelineDir(), now.Format("2006-01-02")+".jsonl")); err != nil {
		t.Error("今日の TIMELINE が消えた")
	}
	if _, err := os.Stat(filepath.Join(doneDir(), "old.json")); err == nil {
		t.Error("古い DONE が消えていない")
	}
	if _, err := os.Stat(filepath.Join(doneDir(), "new.json")); err != nil {
		t.Error("新しい DONE が消えた")
	}
	if b, _ := os.ReadFile(asks); strings.Count(string(b), "pc-allow") != 1 {
		t.Errorf("頼みの記録の古い行が落ちていない：%q", b)
	}
	if _, err := os.Stat(filepath.Join(cd, "Default", "Cache")); err == nil {
		t.Error("Chrome のキャッシュが消えていない")
	}
	if _, err := os.Stat(filepath.Join(cd, "Default", "Preferences")); err != nil {
		t.Error("Chrome の設定まで消えた")
	}
	if _, err := os.Stat(oldTmp); err == nil {
		os.Remove(oldTmp)
		t.Error("古い一時ファイルが消えていない")
	}
	if _, err := os.Stat(newTmp); err != nil {
		t.Error("新しい一時ファイルが消えた")
	}
	if !strings.Contains(text, "知らせ：なし") || !strings.Contains(text, "大きさ：Claude 用の Chrome") {
		t.Error("結果の書き方が違う")
	}
	notes, rest := capacityLines()
	if len(notes) != 0 || len(rest) == 0 || !strings.HasPrefix(rest[0], "容量の点検：") {
		t.Errorf("pc_status の行が違う：%v %v", notes, rest)
	}
	if capacityDue() {
		t.Error("点検したばかりなのに、まだ点検が要ると言う")
	}
	// C の空きが少ないと知らせる
	t.Setenv("PC_HANDS_FREE_GB", "80")
	(&Agent{}).capacityRun(context.Background())
	if notes, _ := capacityLines(); len(notes) == 0 || !strings.Contains(notes[0], "100 GB を切った") {
		t.Errorf("C の空きの知らせが出ない：%v", notes)
	}
}
