package main

import (
	"strings"
	"testing"
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
