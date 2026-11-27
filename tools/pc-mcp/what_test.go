package main

import (
	"os"
	"strings"
	"testing"
	"time"
)

func TestScriptWhat(t *testing.T) {
	cases := []struct {
		info scriptInfo
		src  string
		want string
	}{
		{scriptInfo{Cmds: []string{"Invoke-WebRequest"}}, "Invoke-WebRequest https://x", "インターネットとやりとりする（落とす・送る）"},
		{scriptInfo{Cmds: []string{"Remove-Item"}}, "Remove-Item C:\\x", "ファイルを完全に消す（ごみ箱を通さない）"},
		{scriptInfo{Cmds: []string{"Get-Content"}}, `Get-Content "$env:LOCALAPPDATA\Google\Chrome\User Data\Default\Login Data"`, "ブラウザのパスワード・クッキーのファイルを読む・変える"},
		{scriptInfo{Cmds: []string{"Get-ItemProperty"}}, `Get-ItemProperty HKLM:\SOFTWARE\x`, "Windows 全体の設定（レジストリ）を読む・変える"},
		{scriptInfo{Members: []string{"DownloadString"}}, `(New-Object Net.WebClient).DownloadString('x')`, "インターネットとやりとりする（落とす・送る）"},
	}
	for _, c := range cases {
		_, need, _, what := checkScriptWhat(c.info, c.src)
		if need == "" || what != c.want {
			t.Errorf("%q: need=%q what=%q want %q", c.src, need, what, c.want)
		}
		if strings.Contains(what, "Invoke") || strings.Contains(what, "HKLM") || strings.Contains(what, "Login Data") {
			t.Errorf("raw text in what: %q", what)
		}
	}
	// 種類の表に抜けが無いこと
	for _, v := range allowCmds {
		if cmdWhat[v] == "" {
			t.Errorf("cmdWhat に無い：%s", v)
		}
	}
}

func TestPathWhat(t *testing.T) {
	if w := pathWhat("sensitive", `\.ssh\`, false); w != "開発の鍵・トークンの場所を読む・開く" {
		t.Error(w)
	}
	if w := pathWhat("system", "", true); !strings.HasPrefix(w, "Windows 本体の場所") {
		t.Error(w)
	}
}

func TestCheckWhy(t *testing.T) {
	ok := []string{"Chrome から claude-hands の新しい版を落として、手元の exe を入れ替えます", "銀行のページの残高を読むだけ（押さない）"}
	ng := []string{"", "  ", strings.Repeat("あ", 101), "a\nb", "https://example.com を開く", "$x = Get-Item {a}; rm"}
	for _, s := range ok {
		if m := checkWhy(s); m != "" {
			t.Errorf("ok %q: %s", s, m)
		}
	}
	for _, s := range ng {
		if m := checkWhy(s); m == "" {
			t.Errorf("ng %q を受け付けた", s)
		}
	}
	if whyOf(psIn2{whyArg: whyArg{Why: "せつめい"}}) != "せつめい" {
		t.Error("whyOf")
	}
}

// 署名の並びは窓（tools/switch-window）と同じ。どちらも同じ答えになること
func TestSignSame(t *testing.T) {
	f := t.TempDir() + "/request.key"
	os.WriteFile(f, []byte("abc123\r\n"), 0o644)
	os.Setenv("HANDS_KEY_FILE", f)
	r := askRequest{Kind: "pc-allow", What: "インターネットとやりとりする（落とす・送る）", Why: "説明", Task: "作業", Tool: "pc_powershell", From: "pc-mcp t", At: time.Date(2026, 11, 27, 21, 0, 0, 0, time.FixedZone("JST", 9*3600))}
	r.sign()
	if r.Sig != "b1eaa42805d6556ec4d703d1eaed5f7f80e0d0b459c0e5f932a59bf02bf6e6e6" {
		t.Fatal(r.Sig)
	}
}

func TestLane(t *testing.T) {
	yes := []string{"送信", "送信する", "購入手続きへ", "ログイン", "Sign in", "Log in", "Submit", "Place your order", "Post", "投稿する", "支払う"}
	no := []string{"検索", "次へ", "キャンセル", "Postal code", "保存", "閉じる", "このページについてのとても長い説明文で、ログインの話も出てくるけれどボタンではないもの"}
	for _, s := range yes {
		if finalWord(s) == "" {
			t.Errorf("E にならない：%q", s)
		}
	}
	for _, s := range no {
		if finalWord(s) != "" {
			t.Errorf("E になった：%q", s)
		}
	}
	if !sendKeys("{ENTER}", "", false) || !sendKeys("", "a\nb", false) || !sendKeys("", "", true) || sendKeys("^s", "abc", false) {
		t.Error("sendKeys")
	}
	if !sendAppRe.MatchString("「Claude」 claude（pid 1）") || sendAppRe.MatchString("「メモ帳」 notepad（pid 2）") {
		t.Error("sendAppRe")
	}
	if laneOf("pc_files", nil) != "A" || laneOf("pc_trash", nil) != "D" || laneOf("browser_tabs", map[string]any{"action": "new"}) != "D" {
		t.Error("laneOf")
	}
}
