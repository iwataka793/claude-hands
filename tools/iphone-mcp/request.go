package main

// Claude からの頼み：スイッチが足りないとき、カオルの PC に「許可しますか？」の窓を出してもらう。
// 道具は mode.txt を書かない。%USERPROFILE%\devkit\requests\<kind>.txt に頼みを置くだけで、
// 窓を出して決めるのはスイッチのアプリ（tools/switch-app/devkit-switch.ps1。カオルが押す）。

import (
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

func requestDir() string { return filepath.Join(os.Getenv("USERPROFILE"), "devkit", "requests") }

// askKaoru は頼みを置き、スイッチのアプリが動いていなければ起動する（動いていれば何もしないで終わる）。
// 頼めないとき（Windows でない・試験で DEVKIT_NO_ASK が付いている）は false。
func askKaoru(kind, why string) bool {
	if os.Getenv("USERPROFILE") == "" || os.Getenv("DEVKIT_NO_ASK") != "" {
		return false
	}
	_ = os.MkdirAll(requestDir(), 0o755)
	_ = os.WriteFile(filepath.Join(requestDir(), kind+".txt"), []byte(why+"\r\n"), 0o644)
	vbs := filepath.Join(os.Getenv("USERPROFILE"), "devkit", "switch-app", "devkit-switch.vbs")
	if _, err := os.Stat(vbs); err == nil {
		_ = exec.Command("wscript.exe", vbs, "/tray").Start()
	}
	return true
}

// waitFor は ok() が true になるまで最大 d 待つ。
func waitFor(d time.Duration, ok func() bool) bool {
	end := time.Now().Add(d)
	for time.Now().Before(end) {
		if ok() {
			return true
		}
		time.Sleep(time.Second)
	}
	return ok()
}
