// pc-mcp：Claude が Windows の PC を見て・動かすための道具。
//
// 2 つの役で動く（同じソースから 2 つの exe を作る）：
//   - pc-mcp.exe（引数なし）：Claude デスクトップの拡張機能。MCP の受け答えだけをして、中身は常駐役に頼む
//   - pc-agent.exe agent：常駐役。カオルのスイッチ（.bat）で起動・停止する。PowerShell・ファイル・画面を実際に扱う
//
// 常駐役を分けるのは、ストア版の Claude から動くプログラムは AppData への書き込みが
// 別の場所に振り分けられるため（2026-11-24 に iPhone の道具で実物を確認）。常駐役はカオルが
// エクスプローラーから起動するので、振り分けを受けない。
//
// スイッチ（オフ・見るだけ・操作も可）と許可（ALLOW、10 分）はカオルだけが .bat で切り替える。
// この道具には切り替える機能を作らない。
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

var version = "dev"

func env(k, def string) string {
	if v := strings.TrimSpace(os.Getenv(k)); v != "" && !strings.HasPrefix(v, "${") {
		return v
	}
	return def
}

// stateDir は常駐役と拡張機能が共有する置き場所（%USERPROFILE%\devkit\pc-agent）。
// AppData の中には置かない（ストア版の Claude で振り分けられるため）。
func stateDir() string {
	home := os.Getenv("USERPROFILE")
	if home == "" {
		home, _ = os.UserHomeDir()
	}
	return env("PC_AGENT_DIR", filepath.Join(home, "devkit", "pc-agent"))
}

// browserDir はブラウザのスイッチの置き場所（%USERPROFILE%\devkit\browser の mode.txt・last-used.txt）。
// 書くのはカオルのスイッチの窓だけ。
func browserDir() string {
	home := os.Getenv("USERPROFILE")
	if home == "" {
		home, _ = os.UserHomeDir()
	}
	return env("PC_BROWSER_DIR", filepath.Join(home, "devkit", "browser"))
}

const agentAddr = "127.0.0.1:60210"

func main() {
	args := os.Args[1:]
	if len(args) == 0 {
		runMCP()
		return
	}
	switch args[0] {
	case "agent":
		runAgent()
	case "version":
		fmt.Println(version)
	default:
		fmt.Fprintln(os.Stderr, "使い方：pc-mcp（拡張機能として）／pc-agent agent（常駐役）")
		os.Exit(2)
	}
}
