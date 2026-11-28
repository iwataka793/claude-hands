package main

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// 箱で試す：箱の中身の読み替え・消したことの印・勝手にできるもの・本物に入れるときの署名と確かめ
func TestTry(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows では本物の Sandboxie で試す（scenario.py）")
	}
	home := t.TempDir()
	box := t.TempDir()
	keyf := filepath.Join(t.TempDir(), "request.key")
	_ = os.WriteFile(keyf, []byte("k1"), 0o600)
	fake := filepath.Join(t.TempDir(), "fake.sh")
	_ = os.WriteFile(fake, []byte("#!/bin/sh\nexit 0\n"), 0o755)
	t.Setenv("USERPROFILE", home)
	t.Setenv("HANDS_BOX_ROOT", box)
	t.Setenv("HANDS_SBIE_START", fake)
	t.Setenv("HANDS_MPCMDRUN", fake)
	t.Setenv("HANDS_KEY_FILE", keyf)
	t.Setenv("PC_HANDS_USERDIR", filepath.Join(home, "devkit", "hands"))
	dl := filepath.Join(home, "devkit", "downloads")
	_ = os.MkdirAll(dl, 0o755)
	w := func(p, s string) { _ = os.MkdirAll(filepath.Dir(p), 0o755); _ = os.WriteFile(p, []byte(s), 0o644) }
	w(filepath.Join(dl, "keep.txt"), "same")
	w(filepath.Join(dl, "change.txt"), "before")
	w(filepath.Join(dl, "remove.txt"), "real")
	ub := filepath.Join(box, "user", "current", "devkit", "downloads")
	w(filepath.Join(ub, "keep.txt"), "same")     // 開いただけ：数えない
	w(filepath.Join(ub, "change.txt"), "after!") // 変わった
	w(filepath.Join(ub, "new.txt"), "new")       // 増えた
	w(filepath.Join(ub, "remove.txt"), "")       // 消えた（印）
	old := time.Date(1986, 5, 24, 0, 47, 2, 0, time.Local)
	_ = os.Chtimes(filepath.Join(ub, "remove.txt"), old, old)
	w(filepath.Join(box, "user", "current", "AppData", "Local", "Temp", "x.tmp"), "n") // 勝手にできるもの
	w(filepath.Join(box, "RegHive"), "r")
	items, noise, reg, err := scanBox()
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, it := range items {
		got[filepath.Base(it.Real)] = it.Kind
	}
	want := map[string]string{"change.txt": "changed", "new.txt": "new", "remove.txt": "deleted"}
	if len(got) != len(want) {
		t.Fatalf("items %v want %v", got, want)
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s: %q want %q", k, got[k], v)
		}
	}
	if noise != 1 || !reg {
		t.Errorf("noise=%d reg=%v", noise, reg)
	}
	if rb := realToBox(box, filepath.Join(dl, "new.txt")); rb != filepath.Join(ub, "new.txt") {
		t.Errorf("realToBox %q", rb)
	}

	a := &Agent{st: NewState(t.TempDir(), time.Minute), ps: NewRunner()}
	_ = os.WriteFile(a.st.path("mode.txt"), []byte("operate"), 0o644)
	ctx := context.Background()
	// 試しの記録を作る（run の後半と同じ）
	s := &tryState{ID: "t1", Path: filepath.Join(dl, "x.exe"), Started: time.Now().Add(-time.Minute), Status: "waiting", Items: items}
	if err := saveTry(s); err != nil {
		t.Fatal(err)
	}
	// Claude からは本物に入れられない
	call := func(m map[string]any) Reply { b, _ := json.Marshal(m); return a.try(ctx, b) }
	if r := call(map[string]any{"action": "apply", "id": "t1"}); !r.IsError || !strings.Contains(r.Text, "TRY の窓") {
		t.Errorf("署名なしで入れられた：%s", r.Text)
	}
	if r := call(map[string]any{"action": "apply", "id": "t1", "sig": "00"}); !r.IsError {
		t.Errorf("偽の署名で入れられた")
	}
	m := hmac.New(sha256.New, []byte("k1"))
	m.Write([]byte(trySignText("t1", "apply")))
	sig := hex.EncodeToString(m.Sum(nil))
	// 窓に出したものと箱の中身が違えば入れない（一覧を新しくする）
	w(filepath.Join(ub, "later.txt"), "x")
	if r := call(map[string]any{"action": "apply", "id": "t1", "sig": sig}); !r.IsError || !strings.Contains(r.Text, "まだ何も入れていない") {
		t.Errorf("中身が変わったのに入れた：%s", r.Text)
	}
	if b, _ := os.ReadFile(filepath.Join(dl, "change.txt")); string(b) != "before" {
		t.Errorf("止めたのに本物が変わった")
	}
	// 新しくした一覧で、消えたもの・変わったものの元は取り置きへ送る（試験では取り置きの台本が動かないので、送れずに止まる）
	r := call(map[string]any{"action": "apply", "id": "t1", "sig": sig})
	if !r.IsError || !strings.Contains(r.Text, "何も入れずに止めました") {
		t.Errorf("取り置きへ送れないのに入れた：%s", r.Text)
	}
	if _, err := os.Stat(filepath.Join(dl, "new.txt")); err == nil {
		t.Errorf("止めたのに増えたものが本物にできた")
	}
	for _, e := range []string{"change.txt.claude-hands-try", "new.txt.claude-hands-try"} {
		if _, err := os.Stat(filepath.Join(dl, e)); err == nil {
			t.Errorf("一時のファイルが残った：%s", e)
		}
	}
	// 捨てる
	if r := call(map[string]any{"action": "discard"}); r.IsError {
		t.Errorf("捨てられない：%s", r.Text)
	}
	if st, _ := loadTry(); st.Status != "discarded" {
		t.Errorf("status %q", st.Status)
	}
	if _, err := os.Stat(box); err == nil {
		t.Errorf("箱が空にならない")
	}
	// 作業場：Claude の台本・downloads の外のファイルは、カオルに見せずに続けて試す
	tool := filepath.Join(home, "tools", "x.bat")
	w(tool, "@echo off")
	r = call(map[string]any{"action": "run", "path": tool})
	if r.IsError || !strings.Contains(r.Text, "まだカオルには見せていない") || !strings.Contains(r.Text, "終わりのコード 0") {
		t.Errorf("作業場の run：%s", r.Text)
	}
	r = call(map[string]any{"action": "run", "script": "Write-Output hi"})
	if r.IsError || !strings.Contains(r.Text, "2 回") {
		t.Errorf("続けて試せない：%s", r.Text)
	}
	if _, err := os.Stat(filepath.Join(home, "devkit", "boxwork", "run-2.ps1")); err != nil {
		t.Errorf("台本が作業用のフォルダに無い")
	}
	if r := call(map[string]any{"action": "run", "path": tool, "script": "x"}); !r.IsError {
		t.Errorf("path と script の両方を受けた")
	}
	if r := call(map[string]any{"action": "offer"}); !r.IsError || !strings.Contains(r.Text, "入れるファイルがありません") {
		t.Errorf("入れるものが無いのに見せた：%s", r.Text)
	}
	// 箱の中で増えたものがあれば見せられる（TRY の窓）
	_ = os.MkdirAll(box, 0o755)
	w(filepath.Join(box, "user", "current", "tools", "out.txt"), "a\nb\n")
	if r := call(map[string]any{"action": "offer"}); r.IsError || !strings.Contains(r.Text, "TRY の窓") || !strings.Contains(r.Text, "+ a") {
		t.Errorf("見せられない：%s", r.Text)
	}
	if r := call(map[string]any{"action": "run", "script": "x"}); !r.IsError || !strings.Contains(r.Text, "返事待ち") {
		t.Errorf("返事待ちなのに続けた：%s", r.Text)
	}
	if r := call(map[string]any{"action": "discard"}); r.IsError {
		t.Errorf("捨てられない：%s", r.Text)
	}
	if _, err := os.Stat(filepath.Join(home, "devkit", "boxwork")); err == nil {
		t.Errorf("捨てたのに作業用のフォルダが残った")
	}
	// 鍵の場所のものは箱でも動かさない
	key := filepath.Join(home, ".ssh", "x.bat")
	w(key, "x")
	if r := call(map[string]any{"action": "run", "path": key}); !r.IsError {
		t.Errorf("鍵の場所のものを動かした")
	}
}

func TestDiffLines(t *testing.T) {
	got := strings.Join(diffLines("a\nb\nc\nd", "a\nB\nc\nd\ne", 10), "|")
	if got != "+ B|- b|+ e" && got != "- b|+ B|+ e" {
		t.Errorf("diff %q", got)
	}
	if l := diffLines("same", "same", 5); len(l) != 0 {
		t.Errorf("同じなのに %v", l)
	}
	if l := headLines("1\n2\n3\n4\n5\n6", 4); len(l) != 5 || l[4] != "…（ほか 2 行）" {
		t.Errorf("head %v", l)
	}
}

func TestTryGuard(t *testing.T) {
	block := []string{
		`Start-Process "C:\Users\iwata\devkit\downloads\setup.exe"`,
		`& 'C:\Users\iwata\devkit\downloads\x\install.bat'`,
		`msiexec /i C:\Users\iwata\devkit\downloads\app.msi`,
		`Invoke-Item C:\Users\iwata\devkit\downloads\report.docm`,
		`Copy-Item C:\Sandbox\iwata\ClaudeBox\user\current\a.txt C:\Users\iwata\a.txt`,
	}
	cmds := [][]string{{"Start-Process"}, {`C:\Users\iwata\devkit\downloads\x\install.bat`}, {"msiexec"}, {"Invoke-Item"}, {"Copy-Item"}}
	for i, src := range block {
		if b, _, _, _ := checkScriptWhat(scriptInfo{Cmds: cmds[i]}, src); b == "" {
			t.Errorf("止まらない：%s", src)
		}
	}
	pass := []string{
		`& 'C:\ProgramData\Microsoft\Windows Defender\Platform\CurrentVersion\MpCmdRun.exe' -Scan -ScanType 3 -File C:\Users\iwata\devkit\downloads\setup.exe`,
		`Expand-Archive -LiteralPath C:\Users\iwata\devkit\downloads\pc-switch.zip -DestinationPath C:\Users\iwata\devkit\downloads\x`,
		`Get-Item -LiteralPath C:\Users\iwata\devkit\downloads\setup.exe -Stream *`,
		`Start-Process 'C:\Program Files\Sandboxie-Plus\Start.exe' -ArgumentList '/box:ClaudeBox','/wait','C:\Users\iwata\devkit\downloads\setup.exe'`,
		`Get-ChildItem D:\sandbox-projects`,
	}
	pcmds := [][]string{{`C:\ProgramData\Microsoft\Windows Defender\Platform\CurrentVersion\MpCmdRun.exe`}, {"Expand-Archive"}, {"Get-Item"}, {"Start-Process"}, {"Get-ChildItem"}}
	for i, src := range pass {
		if b, _, _, _ := checkScriptWhat(scriptInfo{Cmds: pcmds[i]}, src); b != "" {
			t.Errorf("止めすぎ：%s → %s", src, b)
		}
	}
	if openRuns(`C:\Users\iwata\devkit\downloads\setup.exe`, "") == "" || openRuns("explorer", `C:\Users\iwata\devkit\downloads\setup.exe`) == "" {
		t.Errorf("pc_open で落としたものを開ける")
	}
	if openRuns(`C:\Sandbox\iwata\ClaudeBox\drive\C\x.exe`, "") == "" {
		t.Errorf("pc_open で箱の中のものを開ける")
	}
	if openRuns("explorer", `C:\Users\iwata\devkit\downloads\pc-switch-53`) != "" {
		t.Errorf("フォルダを開くのまで止めた")
	}
}
